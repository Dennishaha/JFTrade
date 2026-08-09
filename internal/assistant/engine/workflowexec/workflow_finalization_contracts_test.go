package workflowexec

import (
	"context"
	"strings"
	"testing"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestWorkflowExecutorPersistsFinalizedAndIncompletePlans(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{ID: "workflow-final-agent", Name: "Workflow Final Agent", Status: AgentStatusEnabled})
	session := mustCreateSession(t, runtime, agent.ID, "workflow finalization")
	executor := (&WorkflowExecutor{runtime: runtime})

	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-final-parent", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	doneTask, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "workflow-final-task", Title: "Publish conclusion", Status: "DONE", AgentID: agent.ID, RunID: parent.ID, Order: 1,
	})
	if err != nil {
		t.Fatalf("SaveTask done: %v", err)
	}
	parent.WorkflowPlan = jfadkmodel.WorkflowPlanFromTasks([]Task{doneTask}, nil)
	mustSaveRun(t, runtime, parent)
	child := mustSaveRun(t, runtime, Run{
		ID: "workflow-final-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID, Iteration: 1,
		Status: RunStatusCompleted, Message: "child complete", CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	completed, err := executor.FinalizePlannedWorkflow(ctx, workflowRequest{Session: session}, parent, []Task{doneTask}, []ChatResponse{{Reply: "verified conclusion", Run: child}}, nil)
	if err != nil {
		t.Fatalf("finalize completed workflow: %v", err)
	}
	if completed.Run.Status != RunStatusCompleted || completed.Run.WorkflowStatus != workflowStatusComplete || completed.Run.Iteration != 1 || completed.Run.FinalMessageID == "" {
		t.Fatalf("completed workflow response = %+v", completed)
	}
	if !strings.Contains(completed.Reply, "verified conclusion") || len(completed.Run.WorkflowPlan) != 1 || completed.Run.WorkflowPlan[0].OutputSummary != "verified conclusion" {
		t.Fatalf("completed workflow summary/plan = %+v", completed)
	}

	incompleteParent := mustSaveRun(t, runtime, Run{
		ID: "workflow-incomplete-parent", SessionID: session.ID, AgentID: agent.ID, Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	incompleteTask, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "workflow-incomplete-task", Title: "Still pending", Status: "TODO", AgentID: agent.ID, RunID: incompleteParent.ID,
	})
	if err != nil {
		t.Fatalf("SaveTask incomplete: %v", err)
	}
	incompleteParent.WorkflowPlan = jfadkmodel.WorkflowPlanFromTasks([]Task{incompleteTask}, nil)
	mustSaveRun(t, runtime, incompleteParent)
	incomplete, err := executor.FinalizePlannedWorkflow(ctx, workflowRequest{Session: session}, incompleteParent, []Task{incompleteTask}, nil, nil)
	if err != nil {
		t.Fatalf("finalize incomplete workflow: %v", err)
	}
	if incomplete.Run.Status != RunStatusFailed || incomplete.Run.ErrorCode != workflowTaskIncompleteErr || !strings.Contains(incomplete.Run.FailureReason, "scheduler incomplete") {
		t.Fatalf("incomplete workflow response = %+v", incomplete)
	}
}

func TestWorkflowExecutorPreparesParentPlanAndEmitsAuthoritativeSnapshot(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "workflow-prepare-agent", "workflow preparation")
	executor := (&WorkflowExecutor{runtime: runtime})
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-prepare-parent", SessionID: session.ID, AgentID: session.AgentID, Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{{TaskID: "prepare-task", Title: "Prepare", Status: "TODO"}},
		CreatedAt:    jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	child := Run{ID: "workflow-prepare-child", SessionID: session.ID, AgentID: session.AgentID, ParentRunID: parent.ID, Status: RunStatusRunning}
	var snapshots []Run
	prepared, err := executor.PrepareWorkflowParent(ctx, workflowRequest{
		Agent: sessionAgent(session), Session: session, Message: "prepare plan", EmitRun: true,
		OnDelta: func(delta ChatDelta) error {
			if delta.Run != nil {
				snapshots = append(snapshots, *delta.Run)
			}
			return nil
		},
	}, parent, []Run{child})
	if err != nil {
		t.Fatalf("PrepareWorkflowParent: %v", err)
	}
	if prepared.WorkflowEngine != WorkflowEngineADK2Loop || len(prepared.ChildRunIDs) != 1 || prepared.ChildRunIDs[0] != child.ID || prepared.WorkflowPlan[0].NodeName != jfadkmodel.GoogleADKWorkflowChildName(parent.ID, 0) {
		t.Fatalf("prepared parent = %+v", prepared)
	}
	if len(snapshots) != 1 || snapshots[0].ID != parent.ID {
		t.Fatalf("emitted workflow snapshots = %+v", snapshots)
	}
}

func sessionAgent(session Session) Agent {
	return Agent{ID: session.AgentID, Name: "workflow prepare agent", WorkMode: WorkModeLoop, PermissionMode: PermissionModeApproval}
}
