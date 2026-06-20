package adk

import (
	"context"
	"errors"
	"testing"

	adksession "google.golang.org/adk/session"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
)

func TestPendingApprovalsOnlyClaimsConfirmationCallsOwnedByExecution(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	service := adksession.InMemoryService()
	created, err := service.Create(ctx, &adksession.CreateRequest{AppName: "app", UserID: googleADKUserID, SessionID: "session-approval-owner"})
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	appendConfirmation := func(invocationID, confirmationID, functionCallID string) {
		t.Helper()
		event := adksession.NewEvent(invocationID)
		event.Author = "agent"
		event.Content = genai.NewContentFromParts([]*genai.Part{{FunctionCall: &genai.FunctionCall{
			ID: confirmationID, Name: toolconfirmation.FunctionCallName,
			Args: map[string]any{
				"originalFunctionCall": &genai.FunctionCall{ID: functionCallID, Name: "strategy.research_backtest", Args: map[string]any{"symbol": "TME"}},
				"toolConfirmation":     toolconfirmation.ToolConfirmation{Hint: "approve"},
			},
		}}}, genai.RoleModel)
		if err := service.AppendEvent(ctx, created.Session, event); err != nil {
			t.Fatalf("Append confirmation: %v", err)
		}
	}
	appendConfirmation("inv-foreign", "confirmation-foreign", "call-foreign")
	appendConfirmation("inv-owned", "confirmation-owned", "call-owned")

	execution := &googleADKExecution{
		sessionID: "session-approval-owner", appName: "app", sessionService: service,
		agent: Agent{ID: "agent-1"}, runID: "run-owned",
	}
	execution.ensureCall("call-owned", ToolDescriptor{Name: "strategy.research_backtest"}, map[string]any{"symbol": "TME"})
	approvals, err := execution.pendingApprovals(ctx, runtime.Store())
	if err != nil {
		t.Fatalf("pendingApprovals: %v", err)
	}
	if len(approvals) != 1 || approvals[0].ConfirmationCallID != "confirmation-owned" || approvals[0].RunID != "run-owned" {
		t.Fatalf("approvals = %+v, want only owned confirmation", approvals)
	}
	again, err := execution.pendingApprovals(ctx, runtime.Store())
	if err != nil {
		t.Fatalf("pendingApprovals second pass: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("second pass approvals = %+v, want none", again)
	}

	recovery := &googleADKExecution{
		sessionID: "session-approval-owner", appName: "app", sessionService: service,
		agent: Agent{ID: "agent-1"}, runID: "run-recovery",
	}
	recovery.ensureCall("call-owned", ToolDescriptor{Name: "strategy.research_backtest"}, map[string]any{"symbol": "TME"})
	recovered, err := recovery.pendingApprovals(ctx, runtime.Store())
	if err != nil {
		t.Fatalf("pendingApprovals recovery pass: %v", err)
	}
	if len(recovered) != 0 {
		t.Fatalf("recovery approvals = %+v, want globally idempotent confirmation", recovered)
	}
	all, err := runtime.Store().ListApprovals(ctx)
	if err != nil {
		t.Fatalf("ListApprovals: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("stored approvals = %d, want 1", len(all))
	}
}

func TestGoogleADKExecutionBuffersTextUntilToolsFinish(t *testing.T) {
	t.Parallel()

	var replies []string
	execution := &googleADKExecution{
		sessionID: "session-1",
		agent:     Agent{ID: "agent-1"},
		runID:     "run-1",
		onDelta: func(delta ChatDelta) error {
			if delta.Reply != "" {
				replies = append(replies, delta.Reply)
			}
			return nil
		},
	}

	if err := execution.appendVisibleText("先给前置结论。", ""); err != nil {
		t.Fatalf("append pre-tool text: %v", err)
	}
	call := execution.ensureCall("call-1", ToolDescriptor{Name: "portfolio.summary", Permission: "read"}, map[string]any{"scope": "all"})
	if call == nil {
		t.Fatal("expected tool call to be created")
	}
	if err := execution.appendVisibleText("这段应该等工具结束后再出来。", ""); err != nil {
		t.Fatalf("append buffered text: %v", err)
	}

	if len(replies) != 1 || replies[0] != "先给前置结论。" {
		t.Fatalf("replies before finish = %#v, want only pre-tool text", replies)
	}

	execution.finishCall(call.ID, map[string]any{"ok": true}, nil)

	if len(replies) != 2 {
		t.Fatalf("replies after finish = %#v, want buffered text flushed", replies)
	}
	if replies[1] != "这段应该等工具结束后再出来。" {
		t.Fatalf("flushed reply = %q, want buffered post-tool text", replies[1])
	}

	preToolContent, preToolReasoning := execution.preToolState()
	if preToolContent != "先给前置结论。" || preToolReasoning != "" {
		t.Fatalf("preToolState = (%q, %q)", preToolContent, preToolReasoning)
	}
}

func TestGoogleADKExecutionRequiresPostToolTextBeforeCompletedStatus(t *testing.T) {
	t.Parallel()

	var snapshots []*Run
	execution := &googleADKExecution{
		sessionID: "session-1",
		agent:     Agent{ID: "agent-1"},
		runID:     "run-1",
		onDelta: func(delta ChatDelta) error {
			if delta.Run != nil {
				snapshots = append(snapshots, new(*delta.Run))
			}
			return nil
		},
	}

	if err := execution.appendVisibleText("先检查数据。", ""); err != nil {
		t.Fatalf("append pre-tool text: %v", err)
	}
	call := execution.ensureCall("call-1", ToolDescriptor{Name: "portfolio.summary", Permission: "read"}, map[string]any{"scope": "all"})
	execution.finishCall(call.ID, map[string]any{"ok": true}, nil)

	if len(snapshots) < 2 {
		t.Fatalf("snapshots = %d, want at least start + finish", len(snapshots))
	}
	if got := snapshots[len(snapshots)-1].Status; got != RunStatusRunning {
		t.Fatalf("tool-only snapshot status = %q, want %q", got, RunStatusRunning)
	}
	execution.consumeFunctionResponse(&genai.FunctionResponse{
		ID:       "call-1",
		Name:     "portfolio.summary",
		Response: map[string]any{"ok": true},
	})
	execution.mu.Lock()
	statusAfterResponse := execution.derivedRunStatusForRunLocked("run-1")
	execution.mu.Unlock()
	if statusAfterResponse != RunStatusRunning {
		t.Fatalf("status after function response = %q, want %q", statusAfterResponse, RunStatusRunning)
	}
	if err := execution.appendVisibleText("基于数据，最终结论如下。", ""); err != nil {
		t.Fatalf("append post-tool text: %v", err)
	}
	execution.mu.Lock()
	finalStatus := execution.derivedRunStatusForRunLocked("run-1")
	execution.mu.Unlock()
	if finalStatus != RunStatusCompleted {
		t.Fatalf("status after post-tool text = %q, want %q", finalStatus, RunStatusCompleted)
	}
}

func TestGoogleADKExecutionRequiresPostToolTextAfterLatestToolResponse(t *testing.T) {
	t.Parallel()

	execution := &googleADKExecution{
		sessionID: "session-1",
		agent:     Agent{ID: "agent-1"},
		runID:     "run-1",
		onDelta:   func(ChatDelta) error { return nil },
	}

	first := execution.ensureCall("call-1", ToolDescriptor{Name: "market.candles", Permission: "read"}, map[string]any{"symbol": "TME"})
	execution.finishCall(first.ID, map[string]any{"ok": true}, nil)
	execution.consumeFunctionResponse(&genai.FunctionResponse{
		ID:       "call-1",
		Name:     "market.candles",
		Response: map[string]any{"ok": true},
	})
	if err := execution.appendVisibleText("第一轮工具后的分析。", ""); err != nil {
		t.Fatalf("append first post-tool text: %v", err)
	}
	second := execution.ensureCall("call-2", ToolDescriptor{Name: "strategy.definitions", Permission: "read"}, map[string]any{"query": "TME"})
	execution.finishCall(second.ID, map[string]any{"ok": true}, nil)
	execution.consumeFunctionResponse(&genai.FunctionResponse{
		ID:       "call-2",
		Name:     "strategy.definitions",
		Response: map[string]any{"ok": true},
	})

	execution.mu.Lock()
	statusAfterSecondTool := execution.derivedRunStatusForRunLocked("run-1")
	execution.mu.Unlock()
	if statusAfterSecondTool != RunStatusRunning {
		t.Fatalf("status after second tool response = %q, want %q", statusAfterSecondTool, RunStatusRunning)
	}
	if !execution.runNeedsFinalSynthesis("run-1") {
		t.Fatal("run should need final synthesis after latest tool response")
	}

	if err := execution.appendVisibleText("第二轮工具后的最终结论。", ""); err != nil {
		t.Fatalf("append final post-tool text: %v", err)
	}
	execution.mu.Lock()
	finalStatus := execution.derivedRunStatusForRunLocked("run-1")
	execution.mu.Unlock()
	if finalStatus != RunStatusCompleted {
		t.Fatalf("status after latest post-tool text = %q, want %q", finalStatus, RunStatusCompleted)
	}
}

func TestGoogleADKExecutionFlushBufferedTextWithoutDeadlock(t *testing.T) {
	t.Parallel()

	execution := &googleADKExecution{
		sessionID: "session-1",
		agent:     Agent{ID: "agent-1"},
		runID:     "run-1",
		onDelta:   func(ChatDelta) error { return nil },
	}

	call := execution.ensureCall("call-1", ToolDescriptor{Name: "portfolio.summary", Permission: "read"}, map[string]any{"scope": "all"})
	if err := execution.appendVisibleText("buffered", ""); err != nil {
		t.Fatalf("appendVisibleText: %v", err)
	}
	execution.finishCall(call.ID, map[string]any{"ok": true}, nil)

	if err := execution.flushBufferedTextIfReady(); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("flushBufferedTextIfReady: %v", err)
	}
}

func TestGoogleADKExecutionMarksToolsetFunctionResponseAsSucceeded(t *testing.T) {
	t.Parallel()

	execution := &googleADKExecution{
		sessionID: "session-1",
		agent:     Agent{ID: "agent-1"},
		runID:     "run-1",
		onDelta:   func(ChatDelta) error { return nil },
	}

	call := execution.ensureCall("call-1", ToolDescriptor{Name: "load_skill", Permission: "read"}, map[string]any{"skill_name": "portfolio"})
	if call == nil {
		t.Fatal("expected tool call to be created")
	}

	execution.consumeFunctionResponse(&genai.FunctionResponse{
		ID:       "call-1",
		Name:     "load_skill",
		Response: map[string]any{"result": "ok"},
	})

	toolContext := execution.toolContext()
	if len(toolContext.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(toolContext.calls))
	}
	if toolContext.calls[0].Status != "SUCCEEDED" {
		t.Fatalf("tool call status = %q, want SUCCEEDED", toolContext.calls[0].Status)
	}
	if toolContext.calls[0].CompletedAt == nil {
		t.Fatal("expected completed timestamp to be recorded")
	}
}

func TestGoogleADKExecutionPersistsTimedOutToolFailureOnRunningSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime := newTestRuntime(t)
	run := mustSaveRun(t, runtime, Run{
		ID:        "run-tool-timeout",
		SessionID: "session-1",
		AgentID:   "agent-1",
		Status:    RunStatusRunning,
		Message:   "running",
		CreatedAt: nowString(),
		UpdatedAt: nowString(),
		StartedAt: nowString(),
		Usage:     &RunUsage{},
	})
	execution := &googleADKExecution{
		sessionID: "session-1",
		agent:     Agent{ID: "agent-1"},
		runID:     run.ID,
		persistRunSnapshot: func(snapshot Run) (Run, error) {
			return runtime.persistRunActivitySnapshot(ctx, snapshot)
		},
	}

	call := execution.ensureCall("call-timeout", ToolDescriptor{
		Name:       "portfolio.summary",
		Permission: "read",
	}, map[string]any{"scope": "all"})
	execution.finishCall(call.ID, nil, context.DeadlineExceeded)

	stored, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run lookup err=%v ok=%v", err, ok)
	}
	if stored.Status != RunStatusRunning {
		t.Fatalf("stored status = %q, want %q", stored.Status, RunStatusRunning)
	}
	if stored.ErrorCode != "" {
		t.Fatalf("stored error code = %q, want empty for activity snapshot", stored.ErrorCode)
	}
	if len(stored.ToolCalls) != 1 || stored.ToolCalls[0].Status != "TIMED_OUT" {
		t.Fatalf("stored tool calls = %+v, want timed out call", stored.ToolCalls)
	}
	if stored.ToolCalls[0].Error == nil || *stored.ToolCalls[0].Error != "tool execution timed out: context deadline exceeded" {
		t.Fatalf("stored tool error = %#v, want explicit timeout message", stored.ToolCalls[0].Error)
	}
	if stored.FailureReason != "" {
		t.Fatalf("stored failure reason = %q, want empty for activity snapshot", stored.FailureReason)
	}
	if stored.Degraded {
		t.Fatalf("stored degraded = %v, want false for activity snapshot", stored.Degraded)
	}
}

func TestGoogleADKExecutionDoesNotPersistCompletedActivitySnapshot(t *testing.T) {
	t.Parallel()
	execution := &googleADKExecution{
		runID:                   "run-activity",
		calls:                   []ToolCall{{RunID: "run-activity", ToolName: "strategy.inspect", Status: "SUCCEEDED"}},
		toolResponseSeenByRunID: map[string]bool{"run-activity": true},
		postToolTextByRunID:     map[string]bool{"run-activity": true},
		toolResponseSeqByRunID:  map[string]int{"run-activity": 1},
		postToolTextSeqByRunID:  map[string]int{"run-activity": 1},
	}
	if status := execution.derivedRunStatusForRunLocked("run-activity"); status != RunStatusCompleted {
		t.Fatalf("derived display status = %q, want completed after post-tool text", status)
	}
	if status := execution.persistedRunStatusForRunLocked("run-activity"); status != RunStatusRunning {
		t.Fatalf("persisted activity status = %q, want running until invocation returns", status)
	}
}

func TestGoogleADKExecutionEmitsAuthoritativePauseRequestedSnapshot(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	storedRun := mustSaveRun(t, runtime, Run{
		ID:               "run-goal-pause-stream",
		SessionID:        "session-1",
		AgentID:          "agent-1",
		Status:           RunStatusRunning,
		Message:          "目标将在当前轮结束后暂停。",
		WorkMode:         WorkModeLoop,
		Objective:        "推进目标",
		WorkflowStatus:   workflowStatusRunning,
		PauseRequestedAt: &now,
		ResumeState:      "user_pause_requested",
		CreatedAt:        now,
		UpdatedAt:        now,
		StartedAt:        now,
		Usage:            &RunUsage{},
	})
	staleSnapshot := storedRun
	staleSnapshot.Message = "goal running"
	staleSnapshot.PauseRequestedAt = nil
	staleSnapshot.ResumeState = ""

	var emitted Run
	execution := &googleADKExecution{
		sessionID:           storedRun.SessionID,
		agent:               Agent{ID: storedRun.AgentID},
		runID:               storedRun.ID,
		runSnapshotBaseByID: map[string]Run{storedRun.ID: staleSnapshot},
		persistRunSnapshot: func(snapshot Run) (Run, error) {
			return runtime.persistRunActivitySnapshot(ctx, snapshot)
		},
		onDelta: func(delta ChatDelta) error {
			if delta.Run != nil {
				emitted = *delta.Run
			}
			return nil
		},
	}

	execution.emitRunSnapshotLocked()

	if emitted.PauseRequestedAt == nil || emitted.ResumeState != "user_pause_requested" {
		t.Fatalf("emitted run = %+v, want authoritative pause request fields", emitted)
	}
}
