package adk

import (
	"context"
	"testing"

	"google.golang.org/genai"
)

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

func TestGoogleADKExecutionDerivesCompletedStatusFromFinishedToolCalls(t *testing.T) {
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

	call := execution.ensureCall("call-1", ToolDescriptor{Name: "portfolio.summary", Permission: "read"}, map[string]any{"scope": "all"})
	execution.finishCall(call.ID, map[string]any{"ok": true}, nil)

	if len(snapshots) < 2 {
		t.Fatalf("snapshots = %d, want at least start + finish", len(snapshots))
	}
	if got := snapshots[len(snapshots)-1].Status; got != RunStatusCompleted {
		t.Fatalf("final snapshot status = %q, want %q", got, RunStatusCompleted)
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

	if err := execution.flushBufferedTextIfReady(); err != nil && err != context.Canceled {
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
		persistRunSnapshot: func(snapshot Run) error {
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
