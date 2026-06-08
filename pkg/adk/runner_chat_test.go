package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestPrepareChatRequestValidationAndConcurrency(t *testing.T) {
	ctx := context.Background()

	var nilRuntime *Runtime
	if _, err := nilRuntime.prepareChatRequest(ctx, ChatRequest{Message: "hello"}); err == nil || err.Error() != "adk runtime is unavailable" {
		t.Fatalf("nil runtime error = %v, want adk runtime is unavailable", err)
	}

	runtime := newTestRuntime(t)
	if _, err := runtime.prepareChatRequest(ctx, ChatRequest{Message: "   "}); err == nil || err.Error() != "message is required" {
		t.Fatalf("blank message error = %v, want message is required", err)
	}

	longMessage := strings.Repeat("a", MaxMessageLength+1)
	if _, err := runtime.prepareChatRequest(ctx, ChatRequest{Message: longMessage}); err == nil || !strings.Contains(err.Error(), "message exceeds maximum length") {
		t.Fatalf("long message error = %v, want maximum length error", err)
	}

	for i := 0; i < MaxConcurrentRuns; i++ {
		runtime.runSem <- struct{}{}
	}
	if _, err := runtime.prepareChatRequest(ctx, ChatRequest{Message: "hello"}); err == nil || !strings.Contains(err.Error(), "maximum concurrent runs") {
		t.Fatalf("concurrency error = %v, want maximum concurrent runs", err)
	}
	for i := 0; i < MaxConcurrentRuns; i++ {
		<-runtime.runSem
	}

	text, err := runtime.prepareChatRequest(ctx, ChatRequest{Message: "  hello world  "})
	if err != nil {
		t.Fatalf("prepareChatRequest: %v", err)
	}
	if text != "hello world" {
		t.Fatalf("text = %q, want trimmed message", text)
	}
	<-runtime.runSem
}

func TestHydrateRunExecutionResultPopulatesRunFields(t *testing.T) {
	run := Run{ID: "run-1", Usage: &RunUsage{}}
	errText := "disk full"
	result := hydrateRunExecutionResult(
		run,
		toolExecutionContext{
			calls: []ToolCall{
				{ToolName: "strategy.save_draft", Status: "FAILED", Error: &errText},
				{ToolName: "strategy.optimize", Status: "SUCCEEDED", Output: map[string]any{"taskId": "opt-123"}},
			},
			summaries: []string{"saved draft", "optimization started"},
		},
		[]Approval{{ID: "approval-1", Status: ApprovalStatusPending}},
		"pre content",
		"pre reasoning",
	)

	if len(result.ToolCalls) != 2 || len(result.ToolSummaries) != 2 {
		t.Fatalf("hydrated result = %+v, want calls and summaries", result)
	}
	if result.PreToolContent != "pre content" || result.PreToolReasoning != "pre reasoning" {
		t.Fatalf("pre-tool fields = %q/%q", result.PreToolContent, result.PreToolReasoning)
	}
	if result.OptimizationTaskID != "opt-123" {
		t.Fatalf("optimization task id = %q, want opt-123", result.OptimizationTaskID)
	}
	if len(result.PendingApprovals) != 1 || result.PendingApprovals[0].ID != "approval-1" {
		t.Fatalf("pending approvals = %+v, want approval-1", result.PendingApprovals)
	}
	if result.Usage == nil || result.Usage.ToolCallsTotal != 2 {
		t.Fatalf("usage = %+v, want tool call total 2", result.Usage)
	}
}

func TestMarkFailedChatRunMapsContextToTerminalState(t *testing.T) {
	startedAt := time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano)

	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelled := markFailedChatRun(cancelCtx, Run{ID: "run-cancelled", StartedAt: startedAt, Usage: &RunUsage{}}, cancelCtx.Err())
	if cancelled.Status != RunStatusCancelled || cancelled.ErrorCode != "RUN_CANCELLED" || cancelled.CancelledAt == nil || !cancelled.Degraded {
		t.Fatalf("cancelled run = %+v", cancelled)
	}

	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer timeoutCancel()
	<-timeoutCtx.Done()
	timedOut := markFailedChatRun(timeoutCtx, Run{ID: "run-timeout", StartedAt: startedAt, Usage: &RunUsage{}}, timeoutCtx.Err())
	if timedOut.Status != RunStatusTimedOut || timedOut.ErrorCode != "RUN_TIMED_OUT" || timedOut.CompletedAt == nil || timedOut.CancelledAt != nil {
		t.Fatalf("timed out run = %+v", timedOut)
	}

	otherErr := errors.New("model exploded")
	failed := markFailedChatRun(context.Background(), Run{ID: "run-failed", StartedAt: startedAt, Usage: &RunUsage{}}, otherErr)
	if failed.Status != RunStatusFailed || failed.ErrorCode != "MODEL_CALL_FAILED" || failed.FailureReason != "model exploded" || failed.CompletedAt == nil {
		t.Fatalf("failed run = %+v", failed)
	}
}

func TestPersistRunTerminalStateWritesRunAndAudit(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	run := mustSaveRun(t, runtime, Run{
		ID:            "run-terminal",
		SessionID:     "session-1",
		AgentID:       "agent-1",
		Status:        RunStatusFailed,
		ErrorCode:     "MODEL_CALL_FAILED",
		FailureReason: "boom",
		Message:       "boom",
		CreatedAt:     nowString(),
		UpdatedAt:     nowString(),
	})
	if err := runtime.persistRunTerminalState(ctx, run); err != nil {
		t.Fatalf("persistRunTerminalState: %v", err)
	}

	stored, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	if stored.Status != RunStatusFailed || stored.ErrorCode != "MODEL_CALL_FAILED" {
		t.Fatalf("stored run = %+v", stored)
	}

	events := mustAuditEvents(t, runtime)
	var found *AuditEvent
	for i := range events {
		if events[i].SubjectID == run.ID && events[i].Kind == "run.failed" {
			found = &events[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("audit events = %+v, want run.failed", events)
	}
	if found.Metadata["errorCode"] != "MODEL_CALL_FAILED" || found.Metadata["failureReason"] != "boom" {
		t.Fatalf("audit metadata = %+v", found.Metadata)
	}
}

func TestAttachFinalAssistantMessagePersistsMessageAndRunLink(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	session := mustCreateSession(t, runtime, "agent-1", "chat")
	run := mustSaveRun(t, runtime, Run{
		ID:        "run-final-message",
		SessionID: session.ID,
		AgentID:   "agent-1",
		Status:    RunStatusCompleted,
		CreatedAt: nowString(),
		UpdatedAt: nowString(),
	})

	updated, err := runtime.attachFinalAssistantMessage(ctx, session, run, openAIChatResult{
		Reply:            "all set",
		ReasoningContent: "internal reasoning",
	})
	if err != nil {
		t.Fatalf("attachFinalAssistantMessage: %v", err)
	}
	if updated.FinalMessageID == "" {
		t.Fatalf("updated run = %+v, want final message id", updated)
	}

	messages := mustMessages(t, runtime, session.ID)
	if len(messages) != 1 {
		t.Fatalf("messages len = %d, want 1", len(messages))
	}
	if messages[0].ID != updated.FinalMessageID || messages[0].Content != "all set" || messages[0].ReasoningContent != "internal reasoning" {
		t.Fatalf("message = %+v, updated run = %+v", messages[0], updated)
	}

	stored, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	if stored.FinalMessageID != updated.FinalMessageID {
		t.Fatalf("stored final message id = %q, want %q", stored.FinalMessageID, updated.FinalMessageID)
	}
}

func TestFinishPendingApprovalRunPersistsPendingStateAndAssistantPrompt(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	session := mustCreateSession(t, runtime, "agent-1", "pending")
	run := mustSaveRun(t, runtime, Run{
		ID:        "run-pending-approval",
		SessionID: session.ID,
		AgentID:   "agent-1",
		Status:    RunStatusRunning,
		CreatedAt: nowString(),
		UpdatedAt: nowString(),
	})
	approvals := []Approval{{ID: "approval-1", Status: ApprovalStatusPending}}

	response, err := runtime.finishPendingApprovalRun(ctx, session, run, approvals)
	if err != nil {
		t.Fatalf("finishPendingApprovalRun: %v", err)
	}
	if response.Run.Status != RunStatusPending || response.Run.ResumeState != "waiting_approval" {
		t.Fatalf("response run = %+v, want pending waiting_approval", response.Run)
	}
	if len(response.PendingApprovals) != 1 || response.PendingApprovals[0].ID != "approval-1" {
		t.Fatalf("response approvals = %+v", response.PendingApprovals)
	}
	if !strings.Contains(response.Reply, "审批队列") {
		t.Fatalf("reply = %q, want approval prompt", response.Reply)
	}

	stored, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	if stored.Status != RunStatusPending || stored.Message != "等待用户审批后继续执行。" {
		t.Fatalf("stored run = %+v", stored)
	}

	messages := mustMessages(t, runtime, session.ID)
	if len(messages) != 1 || !strings.Contains(messages[0].Content, "审批队列") {
		t.Fatalf("messages = %+v, want assistant approval message", messages)
	}

	events := mustAuditEvents(t, runtime)
	var found *AuditEvent
	for i := range events {
		if events[i].SubjectID == run.ID && events[i].Kind == "run.awaiting_approval" {
			found = &events[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("audit event = %+v, want run.awaiting_approval", found)
	}
	if got := strings.TrimSpace(toString(found.Metadata["pendingApprovals"])); got != "1" {
		t.Fatalf("pendingApprovals metadata = %q, want 1; event=%+v", got, found)
	}
}

func TestCompleteChatRunFailureUsesFallbackReplyAndPersistsTerminalState(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	session := mustCreateSession(t, runtime, "agent-1", "failed")
	run := mustSaveRun(t, runtime, Run{
		ID:            "run-complete-failed",
		SessionID:     session.ID,
		AgentID:       "agent-1",
		Status:        RunStatusRunning,
		ToolSummaries: []string{"portfolio.summary: ok"},
		StartedAt:     time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
		CreatedAt:     nowString(),
		UpdatedAt:     nowString(),
		Usage:         &RunUsage{},
	})

	response, err := runtime.completeChatRun(
		ctx,
		session,
		run,
		"账户现在怎么样",
		toolExecutionContext{summaries: run.ToolSummaries},
		nil,
		openAIChatResult{},
		errors.New("provider down"),
	)
	if err != nil {
		t.Fatalf("completeChatRun failed: %v", err)
	}
	if response.Run.Status != RunStatusFailed || response.Run.ErrorCode != "MODEL_CALL_FAILED" || response.Run.FinalMessageID == "" {
		t.Fatalf("response run = %+v", response.Run)
	}
	if !strings.Contains(response.Reply, "本地兜底回复") || strings.Contains(response.Reply, "provider down") {
		t.Fatalf("reply = %q, want user-friendly fallback reply without raw cause", response.Reply)
	}

	stored, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	if stored.Status != RunStatusFailed || stored.FinalMessageID == "" {
		t.Fatalf("stored run = %+v", stored)
	}

	messages := mustMessages(t, runtime, session.ID)
	if len(messages) != 1 || !strings.Contains(messages[0].Content, "本地兜底回复") {
		t.Fatalf("messages = %+v, want fallback assistant message", messages)
	}
}

func TestCompleteChatRunSuccessPersistsCompletedRunAndAssistantReply(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	session := mustCreateSession(t, runtime, "agent-1", "success")
	run := mustSaveRun(t, runtime, Run{
		ID:        "run-complete-success",
		SessionID: session.ID,
		AgentID:   "agent-1",
		Status:    RunStatusRunning,
		StartedAt: time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
		CreatedAt: nowString(),
		UpdatedAt: nowString(),
		Usage:     &RunUsage{},
	})

	response, err := runtime.completeChatRun(
		ctx,
		session,
		run,
		"hello",
		toolExecutionContext{},
		nil,
		openAIChatResult{Reply: "final answer", ReasoningContent: "because data"},
		nil,
	)
	if err != nil {
		t.Fatalf("completeChatRun success: %v", err)
	}
	if response.Run.Status != RunStatusCompleted || response.Run.FinalMessageID == "" || response.Reply != "final answer" {
		t.Fatalf("response = %+v", response)
	}

	stored, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	if stored.Status != RunStatusCompleted || stored.Message != "completed" || stored.FinalMessageID == "" {
		t.Fatalf("stored run = %+v", stored)
	}

	messages := mustMessages(t, runtime, session.ID)
	if len(messages) != 1 || messages[0].Content != "final answer" || messages[0].ReasoningContent != "because data" {
		t.Fatalf("messages = %+v", messages)
	}
}

func toString(value any) string {
	return fmt.Sprint(value)
}

func TestResolveAgentCoversDefaultAndProviderValidation(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	defaultAgent, err := runtime.resolveAgent(ctx, "")
	if err != nil {
		t.Fatalf("resolveAgent default: %v", err)
	}
	if defaultAgent.ID == "" || defaultAgent.Status != AgentStatusEnabled {
		t.Fatalf("default agent = %+v, want enabled default agent", defaultAgent)
	}

	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "provider-disabled", DisplayName: "Disabled", Enabled: false,
	})
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "provider-no-key", DisplayName: "No Key", Enabled: true,
	})
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: "provider-ok", DisplayName: "OK", APIKey: "sk-ok", Enabled: true,
	})
	disabledAgent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent-disabled", Name: "Disabled", Status: AgentStatusDisabled,
	})
	deletedAgent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent-deleted", Name: "Deleted", Status: AgentStatusEnabled,
	})
	if err := runtime.Store().DeleteAgent(ctx, deletedAgent.ID); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
	agentDisabledProvider := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent-provider-disabled", Name: "Provider Disabled", ProviderID: "provider-disabled", Status: AgentStatusEnabled,
	})
	agentNoKey := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent-no-key", Name: "No Key", ProviderID: "provider-no-key", Status: AgentStatusEnabled,
	})
	agentOK := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "agent-ok", Name: "OK", ProviderID: "provider-ok", Status: AgentStatusEnabled,
	})

	if _, err := runtime.resolveAgent(ctx, "missing-agent"); err == nil || err.Error() != "agent not found" {
		t.Fatalf("missing agent error = %v, want agent not found", err)
	}
	if _, err := runtime.resolveAgent(ctx, disabledAgent.ID); err == nil || err.Error() != "agent is disabled" {
		t.Fatalf("disabled agent error = %v, want agent is disabled", err)
	}
	if _, err := runtime.resolveAgent(ctx, deletedAgent.ID); err == nil || err.Error() != "agent is disabled" {
		t.Fatalf("deleted agent error = %v, want agent is disabled due to soft delete", err)
	}
	if _, err := runtime.resolveAgent(ctx, agentDisabledProvider.ID); err == nil || err.Error() != "agent provider is unavailable" {
		t.Fatalf("provider disabled error = %v, want agent provider is unavailable", err)
	}
	if _, err := runtime.resolveAgent(ctx, agentNoKey.ID); err == nil || err.Error() != "agent provider API key is not configured" {
		t.Fatalf("provider no key error = %v, want provider API key not configured", err)
	}

	resolved, err := runtime.resolveAgent(ctx, agentOK.ID)
	if err != nil {
		t.Fatalf("resolveAgent ok: %v", err)
	}
	if resolved.ID != agentOK.ID {
		t.Fatalf("resolved agent = %+v, want %s", resolved, agentOK.ID)
	}
}

func TestResolveSessionReusesExistingRejectsMismatchAndCreatesTrimmedSession(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	agentA := Agent{ID: "agent-a"}
	agentB := Agent{ID: "agent-b"}
	existing := mustCreateSession(t, runtime, agentA.ID, "Existing")

	resolved, err := runtime.resolveSession(ctx, existing.ID, agentA, "hello")
	if err != nil {
		t.Fatalf("resolveSession existing: %v", err)
	}
	if resolved.ID != existing.ID {
		t.Fatalf("resolved session = %+v, want %s", resolved, existing.ID)
	}

	if _, err := runtime.resolveSession(ctx, existing.ID, agentB, "hello"); err == nil || err.Error() != "session belongs to a different agent" {
		t.Fatalf("mismatch session error = %v, want different agent", err)
	}
	if _, err := runtime.resolveSession(ctx, "missing-session", agentA, "hello"); err == nil || err.Error() != "session not found" {
		t.Fatalf("missing session error = %v, want session not found", err)
	}

	longText := strings.Repeat("会话标题", 10)
	created, err := runtime.resolveSession(ctx, "", agentA, longText)
	if err != nil {
		t.Fatalf("resolveSession create: %v", err)
	}
	if created.AgentID != agentA.ID {
		t.Fatalf("created session = %+v, want agent %s", created, agentA.ID)
	}
	if len([]rune(created.Title)) != 28 {
		t.Fatalf("created title len = %d, want 28", len([]rune(created.Title)))
	}
}

func TestStartRunPersistsRunAndFinishRemovesActiveHandle(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	run, runCtx, finish, err := runtime.startRun(ctx, "session-1", Agent{ID: "agent-1", ProviderID: "provider-1"}, "hello")
	if err != nil {
		t.Fatalf("startRun: %v", err)
	}
	if run.Status != RunStatusRunning || run.SessionID != "session-1" || run.AgentID != "agent-1" || run.ProviderID != "provider-1" {
		t.Fatalf("run = %+v", run)
	}
	if runCtx == nil {
		t.Fatal("expected run context")
	}

	stored, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	if stored.Status != RunStatusRunning || stored.UserMessage != "hello" {
		t.Fatalf("stored run = %+v", stored)
	}

	runtime.activeMu.Lock()
	_, active := runtime.activeRuns[run.ID]
	runtime.activeMu.Unlock()
	if !active {
		t.Fatalf("active run %s not registered", run.ID)
	}

	finish()
	select {
	case <-runCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("run context was not cancelled by finish")
	}

	runtime.activeMu.Lock()
	_, active = runtime.activeRuns[run.ID]
	runtime.activeMu.Unlock()
	if active {
		t.Fatalf("active run %s still registered after finish", run.ID)
	}
}

func TestCancelRunOnTerminalStateIsNoop(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	completedAt := nowString()
	run := mustSaveRun(t, runtime, Run{
		ID:          "run-terminal-noop",
		SessionID:   "session-1",
		AgentID:     "agent-1",
		Status:      RunStatusCompleted,
		Message:     "completed",
		CreatedAt:   nowString(),
		UpdatedAt:   nowString(),
		CompletedAt: &completedAt,
	})

	cancelled, err := runtime.CancelRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("CancelRun terminal: %v", err)
	}
	if cancelled.Status != RunStatusCompleted || cancelled.Message != "completed" {
		t.Fatalf("cancelled run = %+v, want unchanged completed run", cancelled)
	}

	events := mustAuditEvents(t, runtime)
	for _, event := range events {
		if event.SubjectID == run.ID && event.Kind == "run.cancelled" {
			t.Fatalf("unexpected run.cancelled audit for terminal run: %+v", event)
		}
	}
}
