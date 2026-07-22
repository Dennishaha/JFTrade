package adk

import (
	"context"
	"strings"
	"testing"
)

func TestCoverage98ApprovalPersistenceFailuresRemainObservable(t *testing.T) {
	ctx := context.Background()

	t.Run("async approval rolls back resolution when run staging fails", func(t *testing.T) {
		runtime, run, approvals := newCoverage98PendingApprovalRun(t, "approval-stage-pending", 2)
		installCoverage98RunUpdateRejectTrigger(t, runtime, "reject_stage_pending")

		_, err := runtime.ResolveApprovalAsync(ctx, approvals[0].ID, true)
		if err == nil || !strings.Contains(err.Error(), "reject_stage_pending") {
			t.Fatalf("ResolveApprovalAsync stage save error = %v", err)
		}
		storedApproval, ok, approvalErr := runtime.Store().Approval(ctx, approvals[0].ID)
		if approvalErr != nil || !ok || storedApproval.Status != ApprovalStatusPending {
			t.Fatalf("approval after failed atomic stage = %+v/%v/%v, want pending", storedApproval, ok, approvalErr)
		}
		storedRun, ok, runErr := runtime.Store().Run(ctx, run.ID)
		if runErr != nil || !ok || storedRun.PendingApprovals[0].Status != ApprovalStatusPending || storedRun.PendingApprovals[1].Status != ApprovalStatusPending {
			t.Fatalf("run must remain retryable after failed stage save = %+v/%v/%v", storedRun, ok, runErr)
		}
	})

	t.Run("async approval surfaces final transition save failure", func(t *testing.T) {
		runtime, _, approvals := newCoverage98PendingApprovalRun(t, "approval-stage-final", 1)
		installCoverage98RunUpdateRejectTrigger(t, runtime, "reject_stage_final")

		_, err := runtime.ResolveApprovalAsync(ctx, approvals[0].ID, true)
		if err == nil || !strings.Contains(err.Error(), "reject_stage_final") {
			t.Fatalf("ResolveApprovalAsync final stage error = %v", err)
		}
	})

	t.Run("synchronous approval does not hide continuation state save failure", func(t *testing.T) {
		runtime, _, approvals := newCoverage98PendingApprovalRun(t, "approval-sync-save", 1)
		installCoverage98RunUpdateRejectTrigger(t, runtime, "reject_sync_continue")

		_, err := runtime.ResolveApproval(ctx, approvals[0].ID, true)
		if err == nil || !strings.Contains(err.Error(), "reject_sync_continue") {
			t.Fatalf("ResolveApproval continuation save error = %v", err)
		}
	})

	t.Run("background continuation returns persistence failures to its supervisor", func(t *testing.T) {
		runtime := newTestRuntime(t)
		run := mustSaveRun(t, runtime, Run{
			ID:        "approval-background-save",
			SessionID: "session-approval-background",
			AgentID:   "agent-approval-background",
			Status:    RunStatusPending,
			PendingApprovals: []Approval{{
				ID: "approval-background-save-id", RunID: "approval-background-save", Status: ApprovalStatusApproved,
				FunctionCallID: "call", ConfirmationCallID: "confirmation",
			}},
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		installCoverage98RunUpdateRejectTrigger(t, runtime, "reject_background_continue")

		if err := runtime.continueResolvedApprovalRun(ctx, run.ID); err == nil || !strings.Contains(err.Error(), "reject_background_continue") {
			t.Fatalf("continueResolvedApprovalRun error = %v", err)
		}
		resolution, err := runtime.continueResolvedApproval(ctx, Approval{ID: "missing", RunID: "missing-run", Status: ApprovalStatusApproved}, true)
		if err != nil || resolution.Run != nil || resolution.Approval.ID != "missing" {
			t.Fatalf("missing continuation resolution = %+v/%v", resolution, err)
		}
	})
}

func TestCoverage98WorkflowApprovalReconcilerRestagesPersistedResolution(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "approval-reconcile-parent", SessionID: "approval-reconcile-session", AgentID: "approval-reconcile-agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{{TaskID: "approval-child-task", ChildRunID: "approval-reconcile-child", Status: "IN_PROGRESS"}},
		CreatedAt:    now, UpdatedAt: now, Usage: &RunUsage{},
	})
	approval := Approval{
		ID: "approval-reconcile-id", RunID: "approval-reconcile-child", AgentID: parent.AgentID,
		ToolName: "write", Status: ApprovalStatusApproved, CreatedAt: now, UpdatedAt: now,
	}
	child := mustSaveRun(t, runtime, Run{
		ID: "approval-reconcile-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusPending, WorkMode: WorkModeChat, PendingApprovals: []Approval{{
			ID: approval.ID, RunID: approval.RunID, AgentID: approval.AgentID, ToolName: approval.ToolName,
			Status: ApprovalStatusPending, CreatedAt: now, UpdatedAt: now,
		}},
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval resolved record: %v", err)
	}

	// The reconciler should stage the durable resolution and synchronize the
	// parent immediately.  Disable only the later goroutine; the parent/child
	// state transition itself remains synchronous and observable here.
	runtime.closing = true
	runtime.reconcileWorkflowParent(ctx, parent)

	storedChild, ok, err := runtime.Store().Run(ctx, child.ID)
	if err != nil || !ok || storedChild.Status != RunStatusRunning || storedChild.ResumeState != "approval_resuming" || storedChild.PendingApprovals[0].Status != ApprovalStatusApproved {
		t.Fatalf("reconciled child = %+v/%v/%v", storedChild, ok, err)
	}
	storedParent, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok || storedParent.Status != RunStatusRunning || storedParent.WorkflowStatus != workflowStatusRunning || len(storedParent.ChildRunIDs) != 1 || storedParent.ChildRunIDs[0] != child.ID {
		t.Fatalf("reconciled parent = %+v/%v/%v", storedParent, ok, err)
	}
}

func newCoverage98PendingApprovalRun(t *testing.T, runID string, approvalCount int) (*Runtime, Run, []Approval) {
	t.Helper()
	runtime := newTestRuntime(t)
	now := nowString()
	approvals := make([]Approval, 0, approvalCount)
	toolCalls := make([]ToolCall, 0, approvalCount)
	for index := range approvalCount {
		approval := Approval{
			ID: runID + "-approval-" + string(rune('a'+index)), RunID: runID, AgentID: "agent-" + runID,
			ToolName: "write", Status: ApprovalStatusPending, CreatedAt: now, UpdatedAt: now,
		}
		approvals = append(approvals, approval)
		toolCalls = append(toolCalls, ToolCall{ID: runID + "-call-" + string(rune('a'+index)), RunID: runID, ToolName: "write", Status: "PENDING_APPROVAL", RequiresUser: true, CreatedAt: now, UpdatedAt: now})
	}
	run := mustSaveRun(t, runtime, Run{
		ID: runID, SessionID: "session-" + runID, AgentID: "agent-" + runID, Status: RunStatusPending,
		ResumeState: "waiting_approval", PendingApprovals: approvals, ToolCalls: toolCalls,
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	for _, approval := range approvals {
		if err := runtime.Store().SaveApproval(context.Background(), approval); err != nil {
			t.Fatalf("SaveApproval(%s): %v", approval.ID, err)
		}
	}
	return runtime, run, approvals
}

func installCoverage98RunUpdateRejectTrigger(t *testing.T, runtime *Runtime, triggerName string) {
	t.Helper()
	if _, err := runtime.Store().db.Exec(`CREATE TRIGGER ` + triggerName + ` BEFORE UPDATE ON ` + tableRuns + ` BEGIN SELECT RAISE(ABORT, '` + triggerName + `'); END`); err != nil {
		t.Fatalf("create %s trigger: %v", triggerName, err)
	}
}
