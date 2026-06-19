package adk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/genai"
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
	result := hydrateRunExecutionResult(
		run,
		toolExecutionContext{
			calls: []ToolCall{
				{ToolName: "strategy.save_draft", Status: "FAILED", Error: new("disk full")},
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

func TestChatToolOnlyADKRunSynthesizesFinalReply(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "tool-final-agent", Name: "Tool Final", Status: AgentStatusEnabled,
		Tools: []string{"tools.search"},
	})

	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID: agent.ID,
		Message: "@tools.search 查找工具",
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if response.Run.Status != RunStatusCompleted {
		t.Fatalf("run status = %q, want completed", response.Run.Status)
	}
	if len(response.Run.ToolCalls) != 1 || response.Run.ToolCalls[0].Status != "SUCCEEDED" {
		t.Fatalf("tool calls = %+v, want one succeeded call", response.Run.ToolCalls)
	}
	if strings.TrimSpace(response.Reply) == "" {
		t.Fatal("reply is empty, want synthesized final reply after tool result")
	}
	if !strings.Contains(response.Reply, "tools.search") {
		t.Fatalf("reply = %q, want tool result summary", response.Reply)
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

	messages := mustAssistantMessages(t, runtime, session.ID)
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
	if len(messages) != 0 {
		t.Fatalf("messages = %+v, want no persisted assistant placeholder", messages)
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

func TestCompleteChatRunFailurePersistsUserFacingErrorReply(t *testing.T) {
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
	if response.Reply != "provider down" {
		t.Fatalf("reply = %q, want user-facing error reply", response.Reply)
	}

	stored, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	if stored.Status != RunStatusFailed || stored.FinalMessageID == "" {
		t.Fatalf("stored run = %+v", stored)
	}

	messages := mustAssistantMessages(t, runtime, session.ID)
	if len(messages) != 1 || messages[0].Content != "provider down" {
		t.Fatalf("messages = %+v, want user-facing error assistant message", messages)
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

	messages := mustAssistantMessages(t, runtime, session.ID)
	if len(messages) != 1 || messages[0].Content != "final answer" || messages[0].ReasoningContent != "because data" {
		t.Fatalf("messages = %+v", messages)
	}
}

func TestCompleteChatRunKeepsFailedToolCallsVisibleWithoutFailingRun(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	session := mustCreateSession(t, runtime, "agent-1", "tool failure")
	run := mustSaveRun(t, runtime, Run{
		ID:        "run-complete-tool-failed",
		SessionID: session.ID,
		AgentID:   "agent-1",
		Status:    RunStatusRunning,
		StartedAt: time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano),
		CreatedAt: nowString(),
		UpdatedAt: nowString(),
		ToolCalls: []ToolCall{{
			ID:         "tool-1",
			RunID:      "run-complete-tool-failed",
			ToolName:   "strategy.save_draft",
			Permission: "write_strategy",
			Status:     "FAILED",
			Error:      new("disk full"),
			CreatedAt:  nowString(),
			UpdatedAt:  nowString(),
		}},
		Usage: &RunUsage{},
	})

	response, err := runtime.completeChatRun(
		ctx,
		session,
		run,
		"save this strategy",
		toolExecutionContext{calls: run.ToolCalls},
		nil,
		openAIChatResult{},
		nil,
	)
	if err != nil {
		t.Fatalf("completeChatRun tool failure: %v", err)
	}
	if response.Run.Status != RunStatusCompleted {
		t.Fatalf("response status = %q, want %q", response.Run.Status, RunStatusCompleted)
	}
	if response.Run.ErrorCode != "" {
		t.Fatalf("response error code = %q, want empty", response.Run.ErrorCode)
	}
	if response.Run.FailureReason != "" {
		t.Fatalf("response failure reason = %q, want empty", response.Run.FailureReason)
	}
	if !response.Run.Degraded {
		t.Fatalf("response degraded = %v, want true", response.Run.Degraded)
	}

	stored, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	if stored.Status != RunStatusCompleted || stored.ErrorCode != "" {
		t.Fatalf("stored run = %+v", stored)
	}
	if stored.FailureReason != "" {
		t.Fatalf("stored failure reason = %q, want empty", stored.FailureReason)
	}
	if !stored.Degraded {
		t.Fatalf("stored degraded = %v, want true", stored.Degraded)
	}
}

func TestProjectedChatResponseAppliesProjectionToRunFields(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:             "agent-projection-run",
		Name:           "Agent",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "projection run")
	run := mustSaveRun(t, runtime, Run{
		ID:        "run-projection-run",
		SessionID: session.ID,
		AgentID:   agent.ID,
		Status:    RunStatusCompleted,
		CreatedAt: nowString(),
		UpdatedAt: nowString(),
		Usage:     &RunUsage{},
	})
	appendADKEvent(t, runtime, agent.ID, session.ID, newAssistantEvent(run.ID, []*genai.Part{{Text: "先说明一下。"}}, time.Unix(30, 0)))
	appendADKEvent(t, runtime, agent.ID, session.ID, newToolCallEvent(run.ID, "call-opt", "strategy.optimize", time.Unix(31, 0)))
	appendADKEvent(t, runtime, agent.ID, session.ID, newToolResponseEvent(run.ID, "call-opt", "strategy.optimize", map[string]any{"taskId": "opt-999", "status": "started"}, time.Unix(32, 0)))
	appendADKEvent(t, runtime, agent.ID, session.ID, newAssistantEvent(run.ID, []*genai.Part{{Text: "优化已启动。"}}, time.Unix(33, 0)))

	response := runtime.projectedChatResponse(ctx, session, run, openAIChatResult{Reply: "projected reply"})
	if response.Reply != "先说明一下。优化已启动。" {
		t.Fatalf("reply = %q, want 先说明一下。优化已启动。", response.Reply)
	}
	if response.Run.PreToolContent != "先说明一下。" {
		t.Fatalf("preToolContent = %q, want 先说明一下。", response.Run.PreToolContent)
	}
	if len(response.Run.ToolCalls) != 1 || response.Run.ToolCalls[0].ToolName != "strategy.optimize" {
		t.Fatalf("tool calls = %+v, want projected optimize call", response.Run.ToolCalls)
	}
	if len(response.Run.ToolSummaries) != 1 || !strings.Contains(response.Run.ToolSummaries[0], "strategy.optimize") {
		t.Fatalf("tool summaries = %+v, want projected optimize summary", response.Run.ToolSummaries)
	}
	if response.Run.OptimizationTaskID != "opt-999" {
		t.Fatalf("optimizationTaskID = %q, want opt-999", response.Run.OptimizationTaskID)
	}
	if response.Run.Usage == nil || response.Run.Usage.ToolCallsTotal != 1 {
		t.Fatalf("usage = %+v, want tool call total 1", response.Run.Usage)
	}
	if response.Run.FinalMessageID == "" {
		t.Fatalf("finalMessageID = %q, want projected final message id", response.Run.FinalMessageID)
	}
	if len(response.Timeline) == 0 {
		t.Fatalf("timeline = %+v, want projected timeline entries", response.Timeline)
	}
}

func TestRunStoresResolvedModelSnapshot(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID:          "snapshot-provider",
		DisplayName: "Snapshot Provider",
		BaseURL:     "https://example.test/v1",
		Model:       "snapshot-model-v1",
		APIKey:      "sk-test",
		Enabled:     true,
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:             "agent-model-snapshot",
		Name:           "Agent",
		ProviderID:     "snapshot-provider",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "model snapshot")
	run, _, finish, err := runtime.startRun(ctx, session.ID, agent, "hello")
	if err != nil {
		t.Fatalf("startRun: %v", err)
	}
	finish()
	if run.Model != "snapshot-model-v1" {
		t.Fatalf("run model = %q, want snapshot-model-v1", run.Model)
	}
	if run.ProviderName != "Snapshot Provider" {
		t.Fatalf("run providerName = %q, want Snapshot Provider", run.ProviderName)
	}

	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID:          "snapshot-provider",
		DisplayName: "Snapshot Provider Renamed",
		BaseURL:     "https://example.test/v1",
		Model:       "snapshot-model-v2",
		Enabled:     true,
	})
	stored, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	if stored.Model != "snapshot-model-v1" {
		t.Fatalf("stored model = %q, want snapshot-model-v1", stored.Model)
	}
	runs, err := runtime.Store().SessionRuns(ctx, session.ID)
	if err != nil {
		t.Fatalf("SessionRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].Model != "snapshot-model-v1" {
		t.Fatalf("session runs = %+v, want model snapshot", runs)
	}
}

func TestProjectedChatResponseDoesNotExposeResolvedApprovals(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID:             "agent-approved-response",
		Name:           "Agent",
		PermissionMode: PermissionModeApproval,
		Status:         AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "projection approvals")
	run := mustSaveRun(t, runtime, Run{
		ID:        "run-approved-response",
		SessionID: session.ID,
		AgentID:   agent.ID,
		Status:    RunStatusCompleted,
		PendingApprovals: []Approval{{
			ID:        "approval-approved",
			RunID:     "run-approved-response",
			AgentID:   agent.ID,
			ToolName:  "strategy.save_draft",
			Status:    ApprovalStatusApproved,
			Reason:    "resolved",
			CreatedAt: nowString(),
			UpdatedAt: nowString(),
		}},
		CreatedAt: nowString(),
		UpdatedAt: nowString(),
		Usage:     &RunUsage{},
	})
	appendADKEvent(t, runtime, agent.ID, session.ID, newAssistantEvent(run.ID, []*genai.Part{{Text: "done"}}, time.Unix(41, 0)))

	response := runtime.projectedChatResponse(ctx, session, run, openAIChatResult{Reply: "projected reply"})
	if len(response.PendingApprovals) != 0 {
		t.Fatalf("response pending approvals = %+v, want none", response.PendingApprovals)
	}
	if len(response.Run.PendingApprovals) != 0 {
		t.Fatalf("response run pending approvals = %+v, want none", response.Run.PendingApprovals)
	}
	for _, entry := range response.Timeline {
		if entry.Kind == TimelineKindApprovalGroup {
			t.Fatalf("timeline approval group = %+v, want none", entry)
		}
	}
}

func TestResolveApprovalAsyncDetachesClosedStreamBeforeBackgroundResume(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executions := 0
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "strategy.save_draft", Permission: "write_strategy",
		AllowedModes: []string{PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		executions++
		return map[string]any{"saved": true}, nil
	})
	runtime = newRuntimeWithRegistry(t, runtime.Store(), registry)
	agent, err := runtime.Store().SaveAgent(ctx, AgentWriteRequest{
		ID: "agent", Name: "Agent", ProviderID: testProviderID, Tools: []string{"strategy.save_draft"},
		PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled,
	})
	if err != nil {
		t.Fatalf("SaveAgent: %v", err)
	}

	var streamClosed atomic.Bool
	var lateDeltaCalls atomic.Int32
	response, err := runtime.ChatStream(ctx, ChatRequest{
		AgentID: agent.ID, Message: "@strategy.save_draft save",
	}, func(delta ChatDelta) error {
		if streamClosed.Load() {
			lateDeltaCalls.Add(1)
			return errors.New("stale stream callback invoked after request completed")
		}
		_ = delta
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if len(response.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %d, want 1", len(response.PendingApprovals))
	}

	streamClosed.Store(true)
	resolution, err := runtime.ResolveApprovalAsync(ctx, response.PendingApprovals[0].ID, true)
	if err != nil {
		t.Fatalf("ResolveApprovalAsync: %v", err)
	}
	if resolution.Run == nil || resolution.Run.Status != RunStatusRunning || resolution.Run.ResumeState != "approval_resuming" {
		t.Fatalf("initial async resolution = %+v, want running approval_resuming", resolution.Run)
	}
	if len(resolution.Run.ToolCalls) != 1 || resolution.Run.ToolCalls[0].Status != "RUNNING" {
		t.Fatalf("resolution tool calls = %+v, want approved tool resuming as running", resolution.Run.ToolCalls)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		stored, ok, err := runtime.Store().Run(ctx, response.Run.ID)
		if err != nil || !ok {
			t.Fatalf("Run lookup err=%v ok=%v", err, ok)
		}
		if stored.Status != RunStatusRunning {
			if stored.Status != RunStatusCompleted {
				t.Fatalf("final run status = %q, want %q; run=%+v", stored.Status, RunStatusCompleted, stored)
			}
			if stored.ResumeState != "adk_confirmation_resolved" {
				t.Fatalf("resume state = %q, want adk_confirmation_resolved", stored.ResumeState)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run stayed pending after async approval: %+v", stored)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if executions != 1 {
		t.Fatalf("executions = %d, want 1", executions)
	}
	if lateDeltaCalls.Load() != 0 {
		t.Fatalf("late delta callback count = %d, want 0", lateDeltaCalls.Load())
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

	run := mustSaveRun(t, runtime, Run{
		ID:          "run-terminal-noop",
		SessionID:   "session-1",
		AgentID:     "agent-1",
		Status:      RunStatusCompleted,
		Message:     "completed",
		CreatedAt:   nowString(),
		UpdatedAt:   nowString(),
		CompletedAt: new(nowString()),
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
