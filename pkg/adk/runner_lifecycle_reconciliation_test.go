package adk

import (
	"context"
	"strings"
	"testing"
)

func TestReconcileStaleRunsPreservesRecoverableWorkflowStateAndFailsOrphans(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()

	parent := mustSaveRun(t, runtime, Run{
		ID: "reconcile-parent", SessionID: "reconcile-session", AgentID: "reconcile-agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		ChildRunIDs:  []string{"dormant-child"},
		WorkflowPlan: []WorkflowStepState{{TaskID: "dormant-task", ChildRunID: "dormant-child", Status: "IN_PROGRESS"}},
		CreatedAt:    now, UpdatedAt: now, Usage: &RunUsage{},
	})
	dormantChild := mustSaveRun(t, runtime, Run{
		ID: "dormant-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeChat, CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	pendingApproval := mustSaveRun(t, runtime, Run{
		ID: "recoverable-approval", SessionID: parent.SessionID, AgentID: parent.AgentID,
		Status: RunStatusPending, WorkMode: WorkModeChat,
		PendingApprovals: []Approval{{ID: "recoverable-approval-id", Status: ApprovalStatusPending, FunctionCallID: "call", ConfirmationCallID: "confirmation"}},
		CreatedAt:        now, UpdatedAt: now, Usage: &RunUsage{},
	})
	pendingInput := mustSaveRun(t, runtime, Run{
		ID: "recoverable-input", SessionID: parent.SessionID, AgentID: parent.AgentID,
		Status: RunStatusPendingInput, WorkMode: WorkModeChat,
		InputRequest: &InputRequest{ID: "input", Status: InputRequestStatusPending},
		CreatedAt:    now, UpdatedAt: now, Usage: &RunUsage{},
	})
	orphan := mustSaveRun(t, runtime, Run{
		ID: "orphaned-running", SessionID: parent.SessionID, AgentID: parent.AgentID,
		Status: RunStatusRunning, WorkMode: WorkModeChat, CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})

	if err := runtime.reconcileStaleRuns(ctx); err != nil {
		t.Fatalf("reconcileStaleRuns: %v", err)
	}
	for _, expected := range []struct {
		id     string
		status string
	}{
		{dormantChild.ID, RunStatusRunning},
		{pendingApproval.ID, RunStatusPending},
		{pendingInput.ID, RunStatusPendingInput},
	} {
		run, ok, err := runtime.Store().Run(ctx, expected.id)
		if err != nil || !ok || run.Status != expected.status {
			t.Fatalf("reconciled %s = %+v, ok=%v err=%v", expected.id, run, ok, err)
		}
	}
	failed, ok, err := runtime.Store().Run(ctx, orphan.ID)
	if err != nil || !ok || failed.Status != RunStatusFailed || failed.ErrorCode != "RUN_ORPHANED" || failed.ResumeState != "restart_unrecoverable" {
		t.Fatalf("orphan reconciliation = %+v, ok=%v err=%v", failed, ok, err)
	}
	if !runtime.isDormantWorkflowChildRun(ctx, dormantChild) {
		t.Fatal("unstarted child referenced by a live parent should remain dormant, not orphaned")
	}
	if runtime.isDormantWorkflowChildRun(ctx, Run{ID: "no-parent", Status: RunStatusRunning}) {
		t.Fatal("ordinary run should not be considered a dormant workflow child")
	}
}

func TestReconcileTerminalWorkflowCancelsChildrenAndClearsStaleApprovals(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "terminal-workflow-parent", SessionID: "terminal-session", AgentID: "terminal-agent",
		Status: RunStatusFailed, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusFailed,
		ChildRunIDs:      []string{"terminal-workflow-child"},
		PendingApprovals: []Approval{{ID: "terminal-approval", RunID: "terminal-workflow-parent", Status: ApprovalStatusPending}},
		CreatedAt:        now, UpdatedAt: now, Usage: &RunUsage{},
	})
	child := mustSaveRun(t, runtime, Run{
		ID: "terminal-workflow-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeChat, CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})

	if err := runtime.reconcileStaleRun(ctx, runtime.workflowExecutor(), parent); err != nil {
		t.Fatalf("reconcile terminal parent: %v", err)
	}
	updatedParent, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok || len(updatedParent.PendingApprovals) != 0 {
		t.Fatalf("terminal parent reconciliation = %+v, ok=%v err=%v", updatedParent, ok, err)
	}
	updatedChild, ok, err := runtime.Store().Run(ctx, child.ID)
	if err != nil || !ok || updatedChild.Status != RunStatusCancelled || updatedChild.ErrorCode != "PARENT_RUN_TERMINATED" {
		t.Fatalf("terminal child reconciliation = %+v, ok=%v err=%v", updatedChild, ok, err)
	}
	cancelled, err := runtime.cancelChildOfTerminalParent(ctx, Run{ID: "missing-child", ParentRunID: parent.ID, Status: RunStatusRunning})
	if err != nil {
		t.Fatalf("cancel child of terminal parent: %v", err)
	}
	if !cancelled {
		t.Fatal("child under a terminal workflow parent should be identified for cancellation")
	}
}

func TestRepairWorkflowSelfReferenceResetsTheTaskAndPausesTheParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := Run{
		ID: "self-reference-parent", SessionID: "self-reference-session", AgentID: "self-reference-agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		ChildRunIDs: []string{"self-reference-parent", "real-child"}, CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	}
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "self-reference-task", Title: "Recover execution", Status: "IN_PROGRESS", AgentID: parent.AgentID,
		RunID: parent.ID, Executor: workflowTaskExecutorChild, ResultSummary: "stale child output",
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	parent.WorkflowPlan = []WorkflowStepState{{
		TaskID: task.ID, Status: "IN_PROGRESS", ChildRunID: parent.ID, Executor: workflowTaskExecutorChild, ResultSummary: "stale child output",
	}}
	mustSaveRun(t, runtime, parent)

	repaired, err := runtime.repairWorkflowSelfReference(ctx, &parent)
	if err != nil || !repaired {
		t.Fatalf("repairWorkflowSelfReference repaired=%v err=%v", repaired, err)
	}
	if parent.Status != RunStatusPaused || parent.WorkflowStatus != workflowStatusPaused || parent.ResumeState != "self_reference_recovered" || parent.PausedAt == nil {
		t.Fatalf("repaired parent = %+v", parent)
	}
	if len(parent.ChildRunIDs) != 1 || parent.ChildRunIDs[0] != "real-child" || parent.WorkflowPlan[0].ChildRunID != "" || parent.WorkflowPlan[0].Status != "TODO" {
		t.Fatalf("repaired parent references = %+v", parent)
	}
	updatedTask, ok, err := runtime.Store().Task(ctx, task.ID)
	if err != nil || !ok || updatedTask.Status != "TODO" || updatedTask.Executor != "" || updatedTask.ResultSummary != "" {
		t.Fatalf("repaired task = %+v, ok=%v err=%v", updatedTask, ok, err)
	}
	if repaired, err := runtime.repairWorkflowSelfReference(ctx, &parent); err != nil || repaired {
		t.Fatalf("second repair repaired=%v err=%v", repaired, err)
	}
}

func TestWorkflowParentReferenceAndReconcileHelpersBoundaries(t *testing.T) {
	parent := Run{ChildRunIDs: []string{" child-a "}, WorkflowPlan: []WorkflowStepState{{ChildRunID: "child-b"}}}
	for _, tc := range []struct {
		child string
		want  bool
	}{
		{"child-a", true},
		{"child-b", true},
		{"missing", false},
		{" ", false},
	} {
		if got := workflowParentReferencesChild(parent, tc.child); got != tc.want {
			t.Fatalf("workflowParentReferencesChild(%q) = %v, want %v", tc.child, got, tc.want)
		}
	}
	if !workflowChildRunHasNoExecutionActivity(Run{ParentRunID: "parent", Status: RunStatusRunning}) {
		t.Fatal("fresh child run should have no execution activity")
	}
	if workflowChildRunHasNoExecutionActivity(Run{ParentRunID: "parent", Status: RunStatusRunning, PreToolContent: "tool context"}) {
		t.Fatal("child with tool context must not be treated as dormant")
	}
	if isRecoverableReconcileStatus("unknown") || !isRecoverableReconcileStatus(RunStatusPaused) || !isTerminalLifecycleRunStatus(" completed ") || isTerminalLifecycleRunStatus(RunStatusRunning) {
		t.Fatal("lifecycle status classification mismatch")
	}
	_, model := (&Runtime{}).runModelSnapshot(context.Background(), Agent{Model: "model"})
	if !strings.Contains(model, "model") {
		t.Fatal("run model snapshot should retain an explicitly selected model without a store")
	}
}
