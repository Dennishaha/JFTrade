package adk

import (
	"context"
	"testing"
)

func TestResolveAndStageApprovalBoundaryStates(t *testing.T) {
	ctx := context.Background()
	var nilStore *Store
	if approval, changed, run, resumed, err := nilStore.resolveAndStageApproval(ctx, "missing", ApprovalStatusApproved); err != nil || changed || run != nil || resumed || approval.ID != "" {
		t.Fatalf("nil store resolution = %+v/%v/%+v/%v/%v", approval, changed, run, resumed, err)
	}

	t.Run("missing approval", func(t *testing.T) {
		store := newTestRuntime(t).Store()
		if approval, changed, run, resumed, err := store.resolveAndStageApproval(ctx, "missing", ApprovalStatusApproved); err != nil || changed || run != nil || resumed || approval.ID != "" {
			t.Fatalf("missing approval resolution = %+v/%v/%+v/%v/%v", approval, changed, run, resumed, err)
		}
	})

	t.Run("already resolved to another status", func(t *testing.T) {
		store := newTestRuntime(t).Store()
		approval := boundaryApproval("already-resolved", ApprovalStatusApproved)
		if err := store.SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval: %v", err)
		}
		resolved, changed, run, resumed, err := store.resolveAndStageApproval(ctx, approval.ID, ApprovalStatusDenied)
		if err != nil || changed || run != nil || resumed || resolved.Status != ApprovalStatusApproved {
			t.Fatalf("already-resolved resolution = %+v/%v/%+v/%v/%v", resolved, changed, run, resumed, err)
		}
	})

	t.Run("approval without run", func(t *testing.T) {
		store := newTestRuntime(t).Store()
		approval := boundaryApproval("missing-run", ApprovalStatusPending)
		if err := store.SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval: %v", err)
		}
		_, changed, run, resumed, err := store.resolveAndStageApproval(ctx, approval.ID, ApprovalStatusApproved)
		if err != nil || !changed || run != nil || resumed {
			t.Fatalf("missing-run resolution = %v/%+v/%v/%v", changed, run, resumed, err)
		}
	})

	t.Run("run is not pending", func(t *testing.T) {
		runtime := newTestRuntime(t)
		store := runtime.Store()
		approval := boundaryApproval("completed-run", ApprovalStatusPending)
		mustSaveRun(t, runtime, Run{ID: approval.RunID, AgentID: approval.AgentID, Status: RunStatusCompleted, Usage: &RunUsage{}})
		if err := store.SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval: %v", err)
		}
		_, changed, run, resumed, err := store.resolveAndStageApproval(ctx, approval.ID, ApprovalStatusApproved)
		if err != nil || !changed || run != nil || resumed {
			t.Fatalf("completed-run resolution = %v/%+v/%v/%v", changed, run, resumed, err)
		}
	})

	t.Run("approval is absent from run snapshot", func(t *testing.T) {
		runtime := newTestRuntime(t)
		store := runtime.Store()
		approval := boundaryApproval("not-embedded", ApprovalStatusPending)
		mustSaveRun(t, runtime, Run{ID: approval.RunID, AgentID: approval.AgentID, Status: RunStatusPending, Usage: &RunUsage{}})
		if err := store.SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval: %v", err)
		}
		_, changed, run, resumed, err := store.resolveAndStageApproval(ctx, approval.ID, ApprovalStatusApproved)
		if err != nil || !changed || run == nil || resumed {
			t.Fatalf("not-embedded resolution = %v/%+v/%v/%v", changed, run, resumed, err)
		}
	})
}

func TestResolveAndStageApprovalRejectsCorruptDurablePayloads(t *testing.T) {
	ctx := context.Background()
	t.Run("approval payload", func(t *testing.T) {
		store := newTestRuntime(t).Store()
		approval := boundaryApproval("corrupt-approval", ApprovalStatusPending)
		if err := store.SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval: %v", err)
		}
		if _, err := store.db.ExecContext(ctx, `UPDATE `+tableApprovals+` SET payload_json = '{"id":[],"status":"PENDING"}' WHERE id = ?`, approval.ID); err != nil {
			t.Fatalf("corrupt approval: %v", err)
		}
		if _, _, _, _, err := store.resolveAndStageApproval(ctx, approval.ID, ApprovalStatusApproved); err == nil {
			t.Fatal("corrupt approval payload was accepted")
		}
	})

	t.Run("run payload", func(t *testing.T) {
		runtime := newTestRuntime(t)
		store := runtime.Store()
		approval := boundaryApproval("corrupt-run", ApprovalStatusPending)
		mustSaveRun(t, runtime, boundaryPendingRun(approval))
		if err := store.SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval: %v", err)
		}
		if _, err := store.db.ExecContext(ctx, `UPDATE `+tableRuns+` SET payload_json = '{' WHERE id = ?`, approval.RunID); err != nil {
			t.Fatalf("corrupt run: %v", err)
		}
		if _, _, _, _, err := store.resolveAndStageApproval(ctx, approval.ID, ApprovalStatusApproved); err == nil {
			t.Fatal("corrupt run payload was accepted")
		}
	})

	t.Run("sibling approval payload", func(t *testing.T) {
		runtime := newTestRuntime(t)
		store := runtime.Store()
		approval := boundaryApproval("resolved-sibling", ApprovalStatusPending)
		sibling := boundaryApproval("corrupt-sibling", ApprovalStatusPending)
		sibling.RunID = approval.RunID
		run := boundaryPendingRun(approval)
		run.PendingApprovals = append(run.PendingApprovals, sibling)
		mustSaveRun(t, runtime, run)
		for _, item := range []Approval{approval, sibling} {
			if err := store.SaveApproval(ctx, item); err != nil {
				t.Fatalf("SaveApproval(%s): %v", item.ID, err)
			}
		}
		if _, err := store.db.ExecContext(ctx, `UPDATE `+tableApprovals+` SET payload_json = '{"id":[],"status":"PENDING"}' WHERE id = ?`, sibling.ID); err != nil {
			t.Fatalf("corrupt sibling: %v", err)
		}
		if _, _, _, _, err := store.resolveAndStageApproval(ctx, approval.ID, ApprovalStatusApproved); err == nil {
			t.Fatal("corrupt sibling approval payload was accepted")
		}
	})
}

func boundaryApproval(id, status string) Approval {
	now := nowString()
	return Approval{
		ID: id, RunID: "run-" + id, AgentID: "agent-boundary", ToolName: "strategy.save_draft",
		Status: status, FunctionCallID: "call-" + id, ConfirmationCallID: "confirmation-" + id,
		CreatedAt: now, UpdatedAt: now,
	}
}

func boundaryPendingRun(approval Approval) Run {
	return Run{
		ID: approval.RunID, AgentID: approval.AgentID, Status: RunStatusPending,
		PendingApprovals: []Approval{approval}, Usage: &RunUsage{},
	}
}
