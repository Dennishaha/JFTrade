package adk

import (
	"context"
	"testing"
	"time"
)

func TestSaveRunProtectsClaimedApprovalContinuationFromStaleSnapshot(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	approval := Approval{
		ID:                 "approval-claimed-round-one",
		RunID:              "run-claimed-approval",
		AgentID:            "agent-claimed-approval",
		ToolName:           "strategy.save_draft",
		Status:             ApprovalStatusPending,
		FunctionCallID:     "function-round-one",
		ConfirmationCallID: "confirmation-round-one",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	stale := mustSaveRun(t, runtime, Run{
		ID:          approval.RunID,
		SessionID:   "session-claimed-approval",
		AgentID:     approval.AgentID,
		Status:      RunStatusPending,
		ResumeState: "waiting_approval",
		PendingApprovals: []Approval{
			approval,
		},
		ToolCalls: []ToolCall{{
			ID: "function-round-one", RunID: approval.RunID, ToolName: approval.ToolName,
			Status: "PENDING_APPROVAL", RequiresUser: true, CreatedAt: now, UpdatedAt: now,
		}},
		CreatedAt: now,
		StartedAt: now,
		UpdatedAt: now,
		Usage:     &RunUsage{},
	})
	if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	resolved, changed, claimed, shouldContinue, err := runtime.Store().resolveAndStageApproval(ctx, approval.ID, ApprovalStatusApproved)
	if err != nil || !changed || !shouldContinue || claimed == nil {
		t.Fatalf("resolveAndStageApproval = %+v/%v/%+v/%v/%v", resolved, changed, claimed, shouldContinue, err)
	}
	if claimed.Status != RunStatusRunning || claimed.ResumeState != "approval_resuming" || claimed.PendingApprovals[0].Status != ApprovalStatusApproved {
		t.Fatalf("claimed run = %+v", claimed)
	}
	claimedStartedAt := claimed.StartedAt

	stale.Message = "stale waiting snapshot"
	if err := runtime.Store().SaveRun(ctx, stale); err != nil {
		t.Fatalf("SaveRun stale approval snapshot: %v", err)
	}
	stored, ok, err := runtime.Store().Run(ctx, stale.ID)
	if err != nil || !ok {
		t.Fatalf("Run after stale save ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusRunning || stored.ResumeState != "approval_resuming" || stored.PendingApprovals[0].Status != ApprovalStatusApproved || stored.StartedAt != claimedStartedAt {
		t.Fatalf("claimed approval run regressed = %+v", stored)
	}

	nextApproval := Approval{
		ID:                 "approval-claimed-round-two",
		RunID:              stored.ID,
		AgentID:            stored.AgentID,
		ToolName:           "strategy.run_backtest",
		Status:             ApprovalStatusPending,
		FunctionCallID:     "function-round-two",
		ConfirmationCallID: "confirmation-round-two",
		CreatedAt:          nowString(),
		UpdatedAt:          nowString(),
	}
	nextRound := stored
	nextRound.Status = RunStatusPending
	nextRound.ResumeState = "waiting_approval"
	nextRound.Message = "waiting for a new approval round"
	nextRound.PendingApprovals = []Approval{nextApproval}
	nextRound.ToolCalls = []ToolCall{{
		ID: nextApproval.FunctionCallID, RunID: nextApproval.RunID, ToolName: nextApproval.ToolName,
		Status: "PENDING_APPROVAL", RequiresUser: true, CreatedAt: nextApproval.CreatedAt, UpdatedAt: nextApproval.UpdatedAt,
	}}
	if err := runtime.Store().SaveRun(ctx, nextRound); err != nil {
		t.Fatalf("SaveRun JSON-only approval round: %v", err)
	}
	stored, ok, err = runtime.Store().Run(ctx, nextRound.ID)
	if err != nil || !ok {
		t.Fatalf("Run after JSON-only round ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusRunning || stored.ResumeState != "approval_resuming" {
		t.Fatalf("non-durable approval round reopened claimed run = %+v", stored)
	}

	if err := runtime.Store().SaveApproval(ctx, nextApproval); err != nil {
		t.Fatalf("SaveApproval next round: %v", err)
	}
	if err := runtime.Store().SaveRun(ctx, nextRound); err != nil {
		t.Fatalf("SaveRun next approval round: %v", err)
	}
	stored, ok, err = runtime.Store().Run(ctx, nextRound.ID)
	if err != nil || !ok {
		t.Fatalf("Run after next round ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusPending || stored.ResumeState != "waiting_approval" || len(stored.PendingApprovals) != 1 || stored.PendingApprovals[0].ID != nextApproval.ID {
		t.Fatalf("new approval round was blocked = %+v", stored)
	}
}

func TestApprovalContinuationGetsFreshTimeoutWindow(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	oldStartedAt := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	approval := Approval{
		ID:                 "approval-fresh-timeout",
		RunID:              "run-fresh-approval-timeout",
		AgentID:            "agent-fresh-approval-timeout",
		ToolName:           "strategy.save_draft",
		Status:             ApprovalStatusPending,
		FunctionCallID:     "function-fresh-timeout",
		ConfirmationCallID: "confirmation-fresh-timeout",
		CreatedAt:          oldStartedAt,
		UpdatedAt:          oldStartedAt,
	}
	mustSaveRun(t, runtime, Run{
		ID:            approval.RunID,
		SessionID:     "session-fresh-approval-timeout",
		AgentID:       approval.AgentID,
		Status:        RunStatusPending,
		ResumeState:   "waiting_approval",
		MaxDurationMs: (5 * time.Minute).Milliseconds(),
		PendingApprovals: []Approval{
			approval,
		},
		CreatedAt: oldStartedAt,
		StartedAt: oldStartedAt,
		UpdatedAt: oldStartedAt,
		Usage:     &RunUsage{},
	})
	if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	// Keep the durable approval_resuming state observable without launching a
	// real continuation goroutine.
	runtime.approvalMu.Lock()
	runtime.closing = true
	runtime.approvalMu.Unlock()
	resolution, err := runtime.ResolveApprovalAsync(ctx, approval.ID, true)
	if err != nil || resolution.Run == nil {
		t.Fatalf("ResolveApprovalAsync = %+v/%v", resolution, err)
	}
	refreshedAt, err := time.Parse(time.RFC3339Nano, resolution.Run.StartedAt)
	if err != nil {
		t.Fatalf("parse refreshed StartedAt %q: %v", resolution.Run.StartedAt, err)
	}
	if !refreshedAt.After(time.Now().UTC().Add(-time.Minute)) {
		t.Fatalf("approval continuation StartedAt = %s, want a fresh timeout window", resolution.Run.StartedAt)
	}

	if err := runtime.ReconcileExpiredRuns(ctx); err != nil {
		t.Fatalf("ReconcileExpiredRuns: %v", err)
	}
	stored, ok, err := runtime.Store().Run(ctx, approval.RunID)
	if err != nil || !ok {
		t.Fatalf("Run after expiration reconciliation ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusRunning || stored.ResumeState != "approval_resuming" {
		t.Fatalf("fresh approval continuation was timed out = %+v", stored)
	}
}
