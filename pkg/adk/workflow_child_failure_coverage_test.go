package adk

import (
	"context"
	"errors"
	"testing"
)

func TestWorkflowChildFailurePersistsTerminalStateAndFallbackAgent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executor := runtime.workflowExecutor()
	parent := mustSaveRun(t, runtime, Run{
		ID: "child-failure-parent", SessionID: "child-failure-session", AgentID: "parent-agent", Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	child := mustSaveRun(t, runtime, Run{
		ID: "child-failure-run", SessionID: parent.SessionID, AgentID: "child-agent", ParentRunID: parent.ID,
		Status: RunStatusRunning, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	execution := newBareGoogleADKExecution(parent.ID)
	execution.calls = []ToolCall{{ID: "child-tool", RunID: child.ID, ToolName: "market.candles", Status: "SUCCEEDED", Output: map[string]any{"count": 10}}}
	cause := errors.New("final assistant response was missing")
	if err := executor.failWorkflowChildAfterMissingFinal(ctx, child, execution, cause); !errors.Is(err, cause) {
		t.Fatalf("failWorkflowChildAfterMissingFinal err = %v, want cause", err)
	}
	stored, ok, err := runtime.Store().Run(ctx, child.ID)
	if err != nil || !ok || stored.Status != RunStatusFailed || stored.FailureReason != cause.Error() || len(stored.ToolCalls) != 1 {
		t.Fatalf("stored failed child = %+v ok=%v err=%v", stored, ok, err)
	}
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "child-failure-task", Title: "Fallback child", Status: "TODO", RunID: parent.ID})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	blocked := executor.blockedWorkflowChildResult(ctx, workflowRequest{Session: Session{ID: parent.SessionID}}, parent, task, 1, Agent{}, "fallback-agent", "child agent is unavailable")
	if blocked.Response.Run.AgentID != "fallback-agent" || blocked.Response.Run.Status != RunStatusFailed || blocked.Response.Reply != "child agent is unavailable" {
		t.Fatalf("blocked child result = %+v", blocked)
	}
}
