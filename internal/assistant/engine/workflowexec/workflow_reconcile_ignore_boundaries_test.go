package workflowexec

import (
	"context"
	"testing"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestReconcileWorkflowChildrenIgnoresMissingAndForeignRuns(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "reconcile-ignore-agent", Name: "Reconcile Ignore", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "reconcile ignore")
	now := jfadkmodel.NowString()
	parent := mustSaveRun(t, runtime, Run{
		ID:             "run-reconcile-ignore",
		SessionID:      session.ID,
		AgentID:        agent.ID,
		Status:         RunStatusRunning,
		WorkMode:       WorkModeLoop,
		WorkflowStatus: workflowStatusRunning,
		ChildRunIDs:    []string{"child-missing-ignore", "child-foreign-ignore"},
		WorkflowPlan: []WorkflowStepState{
			{TaskID: "task-missing-ignore", Title: "缺失子步骤", Status: "IN_PROGRESS", ChildRunID: "child-missing-ignore"},
			{TaskID: "task-foreign-ignore", Title: "串错子步骤", Status: "IN_PROGRESS", ChildRunID: "child-foreign-ignore"},
		},
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	mustSaveRun(t, runtime, Run{
		ID:          "child-foreign-ignore",
		SessionID:   session.ID,
		AgentID:     agent.ID,
		ParentRunID: "different-parent",
		Status:      RunStatusCompleted,
		Message:     "不属于这个父工作流",
		CreatedAt:   now, UpdatedAt: now, Usage: &RunUsage{},
	})

	updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).ReconcileWorkflowChildren(ctx, parent)
	if err != nil {
		t.Fatalf("reconcileWorkflowChildren ignore stale children: %v", err)
	}
	if blocked {
		t.Fatal("reconcileWorkflowChildren blocked = true, want false when only missing/foreign children remain")
	}
	if updated.Status != RunStatusRunning || updated.WorkflowStatus != workflowStatusRunning {
		t.Fatalf("updated parent = %+v, want unchanged running workflow", updated)
	}
	if got := updated.WorkflowPlan[0].Status; got != "IN_PROGRESS" {
		t.Fatalf("missing child step status = %q, want unchanged IN_PROGRESS", got)
	}
	if got := updated.WorkflowPlan[1].Status; got != "IN_PROGRESS" {
		t.Fatalf("foreign child step status = %q, want unchanged IN_PROGRESS", got)
	}
}
