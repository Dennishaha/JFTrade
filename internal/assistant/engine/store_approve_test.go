package adk

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestResolveApprovalAsyncIsIdempotentForMissingAndResolvedApprovals(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	resolution, err := runtime.ResolveApprovalAsync(ctx, "approval-missing", true)
	if err != nil {
		t.Fatalf("ResolveApprovalAsync missing: %v", err)
	}
	if resolution.Run != nil || resolution.Message != nil || resolution.Approval.ID != "" || resolution.Approval.Status != "" {
		t.Fatalf("missing async resolution = %+v, want zero-value idempotent result", resolution)
	}

	stored := Approval{
		ID: "approval-async-approved", RunID: "run-async-1", AgentID: "agent-async", ToolName: "strategy.save_draft",
		Status: ApprovalStatusApproved, CreatedAt: nowString(), UpdatedAt: nowString(),
	}
	if err := runtime.Store().SaveApproval(ctx, stored); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}
	resolution, err = runtime.ResolveApprovalAsync(ctx, stored.ID, true)
	if err != nil {
		t.Fatalf("ResolveApprovalAsync resolved: %v", err)
	}
	if resolution.Run != nil || resolution.Message != nil {
		t.Fatalf("resolved async resolution = %+v, want no resumed run or message", resolution)
	}
	if resolution.Approval.ID != stored.ID || resolution.Approval.Status != ApprovalStatusApproved {
		t.Fatalf("resolved async approval = %+v, want original approved approval", resolution.Approval)
	}
}

func TestSaveRunAndDenyPendingApprovalsDeniesOnlyPendingRecords(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	run := mustSaveRun(t, runtime, Run{
		ID:        "run-deny-pending",
		SessionID: "session-deny",
		AgentID:   "agent-deny",
		Status:    RunStatusRunning,
		CreatedAt: nowString(),
		UpdatedAt: nowString(),
	})
	pendingA := Approval{ID: "approval-pending-a", RunID: run.ID, AgentID: run.AgentID, ToolName: "strategy.save_definition", Status: ApprovalStatusPending, CreatedAt: nowString(), UpdatedAt: nowString()}
	pendingB := Approval{ID: "approval-pending-b", RunID: run.ID, AgentID: run.AgentID, ToolName: "tasks.update", Status: ApprovalStatusPending, CreatedAt: nowString(), UpdatedAt: nowString()}
	approved := Approval{ID: "approval-approved", RunID: run.ID, AgentID: run.AgentID, ToolName: "memory.remember", Status: ApprovalStatusApproved, CreatedAt: nowString(), UpdatedAt: nowString()}
	for _, approval := range []Approval{pendingA, pendingB, approved} {
		if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
			t.Fatalf("SaveApproval(%s): %v", approval.ID, err)
		}
	}

	cancelledAt := nowString()
	run.Status = RunStatusCancelled
	run.CancelledAt = &cancelledAt
	run.CompletedAt = &cancelledAt
	run.Message = "cancelled"
	run.PendingApprovals = nil
	if err := runtime.Store().SaveRunAndDenyPendingApprovals(ctx, run); err != nil {
		t.Fatalf("SaveRunAndDenyPendingApprovals: %v", err)
	}

	storedRun, ok, err := runtime.Store().Run(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("Run(%s) ok=%v err=%v", run.ID, ok, err)
	}
	if storedRun.Status != RunStatusCancelled || storedRun.CancelledAt == nil || storedRun.CompletedAt == nil {
		t.Fatalf("stored run = %+v, want cancelled terminal run", storedRun)
	}

	for _, tc := range []struct {
		id         string
		wantStatus string
	}{
		{id: pendingA.ID, wantStatus: ApprovalStatusDenied},
		{id: pendingB.ID, wantStatus: ApprovalStatusDenied},
		{id: approved.ID, wantStatus: ApprovalStatusApproved},
	} {
		approval, ok, err := runtime.Store().Approval(ctx, tc.id)
		if err != nil || !ok {
			t.Fatalf("Approval(%s) ok=%v err=%v", tc.id, ok, err)
		}
		if approval.Status != tc.wantStatus {
			t.Fatalf("approval %s status=%q want=%q", tc.id, approval.Status, tc.wantStatus)
		}
	}
}

func TestStoreDeleteSessionContextRemovesLiveState(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)

	if err := runtime.Store().DeleteSessionContext(ctx, "   "); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("DeleteSessionContext(blank) err=%v, want os.ErrNotExist", err)
	}

	saved, err := runtime.Store().SaveSessionContext(ctx, SessionContextState{
		SessionID:         "session-context-delete",
		ContextRevisionID: "ctx-delete-1",
		RecentUserWindow:  3,
	})
	if err != nil {
		t.Fatalf("SaveSessionContext: %v", err)
	}
	if saved.CreatedAt == "" || saved.UpdatedAt == "" {
		t.Fatalf("saved state = %+v, want timestamps", saved)
	}
	if err := runtime.Store().DeleteSessionContext(ctx, saved.SessionID); err != nil {
		t.Fatalf("DeleteSessionContext: %v", err)
	}
	state, ok, err := runtime.Store().SessionContext(ctx, saved.SessionID)
	if err != nil {
		t.Fatalf("SessionContext after delete: %v", err)
	}
	if ok {
		t.Fatalf("SessionContext after delete = %+v ok=%v, want missing", state, ok)
	}
}
