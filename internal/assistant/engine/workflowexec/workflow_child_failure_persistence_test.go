package workflowexec

import (
	"context"
	"errors"
	"testing"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestWorkflowChildFailurePersistsTerminalStateAndFallbackAgent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executor := (&WorkflowExecutor{runtime: runtime})
	parent := mustSaveRun(t, runtime, Run{
		ID: "child-failure-parent", SessionID: "child-failure-session", AgentID: "parent-agent", Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	child := mustSaveRun(t, runtime, Run{
		ID: "child-failure-run", SessionID: parent.SessionID, AgentID: "child-agent", ParentRunID: parent.ID,
		Status: RunStatusRunning, CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	execution := &fakeWorkflowExecutionHandle{calls: []ToolCall{{ID: "child-tool", RunID: child.ID, ToolName: "market.candles", Status: "SUCCEEDED", Output: map[string]any{"count": 10}}}}
	cause := errors.New("final assistant response was missing")
	if err := executor.FailWorkflowChildAfterMissingFinal(ctx, child, execution, cause); !errors.Is(err, cause) {
		t.Fatalf("FailWorkflowChildAfterMissingFinal err = %v, want cause", err)
	}
	stored, ok, err := runtime.Store().Run(ctx, child.ID)
	if err != nil || !ok || stored.Status != RunStatusFailed || stored.FailureReason != cause.Error() || len(stored.ToolCalls) != 1 {
		t.Fatalf("stored failed child = %+v ok=%v err=%v", stored, ok, err)
	}
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{ID: "child-failure-task", Title: "Fallback child", Status: "TODO", RunID: parent.ID})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	blocked := executor.BlockedWorkflowChildResult(ctx, workflowRequest{Session: Session{ID: parent.SessionID}}, parent, task, 1, Agent{}, "fallback-agent", "child agent is unavailable")
	if blocked.Response.Run.AgentID != "fallback-agent" || blocked.Response.Run.Status != RunStatusFailed || blocked.Response.Reply != "child agent is unavailable" {
		t.Fatalf("blocked child result = %+v", blocked)
	}
}
