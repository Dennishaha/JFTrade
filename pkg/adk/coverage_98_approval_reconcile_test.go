package adk

import (
	"context"
	"strings"
	"testing"
)

func TestCoverage98WorkflowApprovalReconcilerProtectsParentAndChildLifecycle(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "coverage-reconcile-agent", Name: "Coverage Reconcile", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "approval reconciliation")
	now := nowString()

	completedParent := Run{
		ID: "coverage-reconcile-completed", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusCompleted, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusComplete,
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	}
	runtime.reconcileWorkflowParent(ctx, completedParent)

	activeParent := mustSaveRun(t, runtime, Run{
		ID: "coverage-reconcile-active", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{
			{TaskID: "blank-child"},
			{TaskID: "missing-child", ChildRunID: "missing-child"},
			{TaskID: "running-child", ChildRunID: "coverage-reconcile-running-child"},
		},
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	mustSaveRun(t, runtime, Run{
		ID: "coverage-reconcile-running-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: activeParent.ID,
		Status: RunStatusRunning, CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	runtime.reconcileWorkflowParent(ctx, activeParent)
	stored, ok, err := runtime.Store().Run(ctx, activeParent.ID)
	if err != nil || !ok || stored.Status != RunStatusRunning {
		t.Fatalf("active parent after incomplete child reconciliation = %+v, ok=%v, err=%v", stored, ok, err)
	}

	terminalParent := mustSaveRun(t, runtime, Run{
		ID: "coverage-reconcile-terminal-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{{TaskID: "failed-child", ChildRunID: "coverage-reconcile-failed-child", Status: "IN_PROGRESS"}},
		ChildRunIDs:  []string{"coverage-reconcile-failed-child"},
		CreatedAt:    now, UpdatedAt: now, Usage: &RunUsage{},
	})
	mustSaveRun(t, runtime, Run{
		ID: "coverage-reconcile-failed-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: terminalParent.ID,
		Status: RunStatusFailed, FailureReason: "child provider rejected the task", CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	runtime.reconcileWorkflowParent(ctx, terminalParent)
	stored, ok, err = runtime.Store().Run(ctx, terminalParent.ID)
	if err != nil || !ok || stored.Status != RunStatusFailed || stored.CompletedAt == nil || !strings.Contains(stored.FailureReason, "child provider rejected") {
		t.Fatalf("terminal child did not terminate parent safely: %+v, ok=%v, err=%v", stored, ok, err)
	}
}

func TestCoverage98ApprovalContinuationEligibilityDistinguishesRecoverableState(t *testing.T) {
	pending := Approval{Status: ApprovalStatusPending, FunctionCallID: "call", ConfirmationCallID: "confirmation"}
	resolved := pending
	resolved.Status = ApprovalStatusApproved

	if !runHasRecoverableApprovalContext(Run{PendingApprovals: []Approval{pending}}) {
		t.Fatal("pending approval with both call ids should be recoverable")
	}
	if runHasRecoverableApprovalContext(Run{PendingApprovals: []Approval{{Status: ApprovalStatusPending, FunctionCallID: "call"}}}) {
		t.Fatal("approval without confirmation id should not be recoverable")
	}
	if !runHasRecoverableResolvedApprovalContext(Run{ResumeState: "approval_resuming", PendingApprovals: []Approval{resolved}}) {
		t.Fatal("resolved approval should be recoverable during approval resumption")
	}
	if runHasRecoverableResolvedApprovalContext(Run{ResumeState: "approval_resuming", PendingApprovals: []Approval{pending}}) {
		t.Fatal("still-pending approval should not be treated as resolved")
	}
	if !runCanContinueResolvedApproval(Run{Status: RunStatusPending}) {
		t.Fatal("pending leaf run should allow continuation")
	}
	if runCanContinueResolvedApproval(Run{Status: RunStatusRunning}) {
		t.Fatal("running leaf run without recoverable state should not continue")
	}
	if runCanContinueResolvedApproval(Run{Status: RunStatusPending, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning}) {
		t.Fatal("workflow parent must not be treated as a resumable leaf")
	}
}
