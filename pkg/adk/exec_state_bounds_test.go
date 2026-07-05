package adk

import (
	"errors"
	"strings"
	"testing"
)

func TestGoogleADKExecutionRunScopedStateProjectionBoundaries(t *testing.T) {
	toolErr := "feed unavailable"
	execution := &googleADKExecution{
		sessionID: "session-state",
		agent:     Agent{ID: "agent-state"},
		runID:     "root-run",
		calls: []ToolCall{
			{
				ID:             "root-call",
				RunID:          "root-run",
				ToolName:       "portfolio.summary",
				Status:         "SUCCEEDED",
				Output:         map[string]any{"cash": 1200},
				IdempotencyKey: "function-root",
			},
			{
				ID:             "child-call",
				RunID:          "child-run",
				ToolName:       "market.candles",
				Status:         "FAILED",
				Error:          &toolErr,
				IdempotencyKey: "function-child",
			},
			{
				ID:             "active-call",
				RunID:          "child-run",
				ToolName:       "strategy.backtest",
				Status:         "PENDING",
				IdempotencyKey: "function-active",
			},
			{
				ID:             "unscoped-call",
				ToolName:       "internal.cleanup",
				Status:         "RUNNING",
				IdempotencyKey: "function-unscoped",
			},
		},
		summaries: []string{"legacy global summary"},
	}
	execution.reply.WriteString(" root reply ")
	execution.reasoning.WriteString(" root reasoning ")
	execution.preToolContent.WriteString(" before tools ")
	execution.preToolReasoning.WriteString(" inspect inputs ")
	execution.ensureTextMaps()
	execution.builderForRun(execution.replyByRunID, "child-run").WriteString(" child reply ")
	execution.builderForRun(execution.reasoningByRunID, "child-run").WriteString(" child reasoning ")

	root := execution.toolContextForRun(" root-run ")
	if len(root.calls) != 1 || root.calls[0].ID != "root-call" {
		t.Fatalf("root tool context calls = %+v, want only root call", root.calls)
	}
	if len(root.summaries) != 1 || !strings.Contains(root.summaries[0], "portfolio.summary") {
		t.Fatalf("root summaries = %+v, want output summary", root.summaries)
	}
	child := execution.toolContextForRun("child-run")
	if len(child.calls) != 2 {
		t.Fatalf("child tool context calls = %+v, want failed and pending child calls", child.calls)
	}
	if len(child.summaries) != 1 || child.summaries[0] != "market.candles: feed unavailable" {
		t.Fatalf("child summaries = %+v, want failed tool summary only", child.summaries)
	}
	global := execution.toolContextForRun("")
	if len(global.calls) != 4 || len(global.summaries) != 1 || global.summaries[0] != "legacy global summary" {
		t.Fatalf("global context = calls:%d summaries:%+v", len(global.calls), global.summaries)
	}

	if got, ok := execution.trackedRunIDForFunctionCall("function-child"); !ok || got != "child-run" {
		t.Fatalf("tracked child run = %q/%v", got, ok)
	}
	if got, ok := execution.trackedRunIDForFunctionCall("function-unscoped"); ok || got != "" {
		t.Fatalf("tracked unscoped call = %q/%v, want missing", got, ok)
	}
	if got := execution.resultForRun(""); got.Reply != "root reply" || got.ReasoningContent != "root reasoning" {
		t.Fatalf("root result = %#v", got)
	}
	if got := execution.resultForRun("child-run"); got.Reply != "child reply" || got.ReasoningContent != "child reasoning" {
		t.Fatalf("child result = %#v", got)
	}
	if reply, reasoning := execution.preToolState(); reply != "before tools" || reasoning != "inspect inputs" {
		t.Fatalf("pre tool state = %q/%q", reply, reasoning)
	}
	if got := execution.activeToolCallCountForRun(" child-run "); got != 1 {
		t.Fatalf("active child tools = %d, want 1", got)
	}
	if got := execution.activeToolCallCountForRun(""); got != 2 {
		t.Fatalf("active global tools = %d, want pending child + running unscoped", got)
	}
	if got := summarizeToolCall(ToolCall{ToolName: "noop"}); got != "" {
		t.Fatalf("empty summary = %q", got)
	}
}

func TestGoogleADKExecutionBufferedTextAndDeltaBoundaries(t *testing.T) {
	var deltas []ChatDelta
	execution := &googleADKExecution{
		sessionID: "session-buffer",
		agent:     Agent{ID: "agent-buffer"},
		runID:     "root-run",
		onDelta: func(delta ChatDelta) error {
			deltas = append(deltas, delta)
			return nil
		},
		calls: []ToolCall{
			{ID: "child-tool", RunID: "child-run", ToolName: "market.candles", Status: "RUNNING"},
			{ID: "root-tool", RunID: "root-run", ToolName: "portfolio.summary", Status: "RUNNING"},
		},
	}

	if err := execution.appendVisibleTextForRun("child-run", " child answer ", " child reasoning "); err != nil {
		t.Fatalf("append child buffered text: %v", err)
	}
	if err := execution.flushBufferedTextForRunIfReady("child-run"); err != nil {
		t.Fatalf("flush active child: %v", err)
	}
	if got := execution.resultForRun("child-run"); got.Reply != "" || got.ReasoningContent != "" {
		t.Fatalf("active child result = %#v, want buffered only", got)
	}
	if len(deltas) != 0 {
		t.Fatalf("child buffered deltas = %+v, want none for non-root run", deltas)
	}

	execution.calls[0].Status = "SUCCEEDED"
	execution.markToolResponseSeenForRun("child-run")
	if err := execution.flushBufferedTextForRunIfReady(" child-run "); err != nil {
		t.Fatalf("flush completed child: %v", err)
	}
	if got := execution.resultForRun("child-run"); got.Reply != "child answer" || got.ReasoningContent != "child reasoning" {
		t.Fatalf("flushed child result = %#v", got)
	}
	if !execution.runHasPostToolText("child-run") {
		t.Fatal("child run should record post-tool text after flushing buffered content")
	}
	if len(deltas) != 0 {
		t.Fatalf("child flush deltas = %+v, want none for non-root run", deltas)
	}

	if err := execution.appendVisibleTextForRun("root-run", " root answer ", " root reasoning "); err != nil {
		t.Fatalf("append root buffered text: %v", err)
	}
	execution.calls[1].Status = "SUCCEEDED"
	execution.markToolResponseSeenForRun("root-run")
	if err := execution.flushBufferedTextIfReady(); err != nil {
		t.Fatalf("flush root: %v", err)
	}
	if got := execution.resultForRun("root-run"); got.Reply != "root answer" || got.ReasoningContent != "root reasoning" {
		t.Fatalf("flushed root result = %#v", got)
	}
	if len(deltas) != 1 || strings.TrimSpace(deltas[0].Reply) != "root answer" || strings.TrimSpace(deltas[0].ReasoningContent) != "root reasoning" {
		t.Fatalf("root deltas = %+v, want flushed root reply and reasoning", deltas)
	}

	execution.emitToolProgress("root-tool", "portfolio.summary")
	if len(deltas) != 2 || !strings.Contains(deltas[1].ToolProgress, "portfolio.summary") {
		t.Fatalf("tool progress delta = %+v", deltas)
	}
	execution.detachDeltaSink()
	execution.emitToolProgress("root-tool", "market.candles")
	if len(deltas) != 2 {
		t.Fatalf("delta sink was not detached: %+v", deltas)
	}
}

func TestGoogleADKExecutionRunSnapshotPersistenceBoundary(t *testing.T) {
	now := nowString()
	var persisted Run
	var emitted []Run
	execution := &googleADKExecution{
		sessionID: "session-snapshot",
		agent:     Agent{ID: "agent-snapshot"},
		runID:     "run-snapshot",
		runSnapshotBaseByID: map[string]Run{
			"run-snapshot": {
				ID:            "run-snapshot",
				SessionID:     "session-snapshot",
				AgentID:       "agent-snapshot",
				Status:        RunStatusFailed,
				Message:       "stale terminal message",
				FailureReason: "stale failure",
				ErrorCode:     "stale_code",
				CompletedAt:   &now,
				CancelledAt:   &now,
				Degraded:      true,
			},
		},
		calls: []ToolCall{{
			ID:       "call-running",
			RunID:    "run-snapshot",
			ToolName: "portfolio.summary",
			Status:   "RUNNING",
		}},
		persistRunSnapshot: func(snapshot Run) (Run, error) {
			persisted = snapshot
			return Run{}, errors.New("store temporarily unavailable")
		},
		onDelta: func(delta ChatDelta) error {
			if delta.Run != nil {
				emitted = append(emitted, *delta.Run)
			}
			return nil
		},
	}

	execution.emitRunSnapshotLocked()

	if persisted.Status != RunStatusRunning {
		t.Fatalf("persisted status = %q, want running activity snapshot", persisted.Status)
	}
	if persisted.CompletedAt != nil || persisted.CancelledAt != nil || persisted.Degraded {
		t.Fatalf("persisted terminal fields = completed:%v cancelled:%v degraded:%v", persisted.CompletedAt, persisted.CancelledAt, persisted.Degraded)
	}
	if persisted.Message != "" || persisted.FailureReason != "" || persisted.ErrorCode != "" {
		t.Fatalf("persisted stale failure fields = message:%q reason:%q code:%q", persisted.Message, persisted.FailureReason, persisted.ErrorCode)
	}
	if len(emitted) != 1 {
		t.Fatalf("emitted snapshots = %+v, want one root snapshot", emitted)
	}
	if emitted[0].Status != RunStatusRunning || len(emitted[0].ToolCalls) != 1 || emitted[0].ToolCalls[0].ID != "call-running" {
		t.Fatalf("emitted snapshot = %+v, want sanitized running tool snapshot", emitted[0])
	}
}
