package adk

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunnerLifecycleProtectsDormantChildrenAndObjectiveBoundaries(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	parent := mustSaveRun(t, runtime, Run{
		ID: "dormant-parent", SessionID: "dormant-session", AgentID: "dormant-agent", Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, ChildRunIDs: []string{"dormant-child"},
		WorkflowPlan: []WorkflowStepState{{ChildRunID: "dormant-child", Status: "IN_PROGRESS"}},
		StartedAt:    now, CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	dormant := mustSaveRun(t, runtime, Run{
		ID: "dormant-child", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeChat, StartedAt: old, CreatedAt: old, UpdatedAt: old, MaxDurationMs: 1, Usage: &RunUsage{},
	})
	if !runtime.isDormantWorkflowChildRun(ctx, dormant) {
		t.Fatal("unstarted workflow child referenced by an active parent should be protected from expiry")
	}
	if err := runtime.ReconcileExpiredRuns(ctx); err != nil {
		t.Fatalf("ReconcileExpiredRuns: %v", err)
	}
	storedDormant, ok, err := runtime.Store().Run(ctx, dormant.ID)
	if err != nil || !ok || storedDormant.Status != RunStatusRunning {
		t.Fatalf("dormant child after expiry reconciliation = %+v ok=%v err=%v", storedDormant, ok, err)
	}
	dormant.ToolCalls = []ToolCall{{ID: "active", Status: "RUNNING"}}
	if runtime.isDormantWorkflowChildRun(ctx, dormant) {
		t.Fatal("a child with actual tool activity must no longer be treated as dormant")
	}

	goal := mustSaveRun(t, runtime, Run{
		ID: "objective-goal", SessionID: parent.SessionID, AgentID: parent.AgentID, Status: RunStatusRunning,
		WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, Objective: "old", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	updated, err := runtime.UpdateRunObjective(ctx, goal.ID, "  verified new objective  ")
	if err != nil || updated.Objective != "verified new objective" {
		t.Fatalf("UpdateRunObjective = %+v err=%v", updated, err)
	}
	if _, err := runtime.UpdateRunObjective(ctx, goal.ID, " "); err == nil || !strings.Contains(err.Error(), "objective is required") {
		t.Fatalf("blank objective err = %v", err)
	}
	chat := mustSaveRun(t, runtime, Run{ID: "objective-chat", Status: RunStatusRunning, WorkMode: WorkModeChat, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{}})
	if _, err := runtime.UpdateRunObjective(ctx, chat.ID, "x"); err == nil || !strings.Contains(err.Error(), "goal runs") {
		t.Fatalf("chat objective err = %v", err)
	}
	childGoal := mustSaveRun(t, runtime, Run{ID: "objective-child", ParentRunID: goal.ID, Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{}})
	if _, err := runtime.UpdateRunObjective(ctx, childGoal.ID, "x"); err == nil || !strings.Contains(err.Error(), "child run") {
		t.Fatalf("child objective err = %v", err)
	}
	goal.Status = RunStatusCompleted
	mustSaveRun(t, runtime, goal)
	if _, err := runtime.UpdateRunObjective(ctx, goal.ID, "x"); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("terminal objective err = %v", err)
	}
}

func TestApprovalRecoveryRecognizesBusySQLiteAndResolvableContexts(t *testing.T) {
	busy := errors.New("append event to SessionService failed: database is locked")
	if !isRetryableADKSessionBusy(busy) || !isRetryableADKSessionBusy(errors.New("Append Event To SessionService: SQLITE_BUSY")) {
		t.Fatal("SQLite session busy errors should be recognized for bounded retry")
	}
	if isRetryableADKSessionBusy(errors.New("database is locked")) || isRetryableADKSessionBusy(nil) {
		t.Fatal("unrelated or nil errors must not be retried as ADK session contention")
	}
	pending := Approval{Status: ApprovalStatusPending, FunctionCallID: "call", ConfirmationCallID: "confirmation"}
	if !runHasRecoverableApprovalContext(Run{PendingApprovals: []Approval{pending}}) {
		t.Fatal("pending approval with both call IDs should be recoverable")
	}
	if runHasRecoverableApprovalContext(Run{PendingApprovals: []Approval{{Status: ApprovalStatusPending, FunctionCallID: "call"}}}) {
		t.Fatal("approval without a confirmation call cannot be rehydrated")
	}
	if !runCanContinueResolvedApproval(Run{Status: RunStatusRunning, ResumeState: "approval_resuming", PendingApprovals: []Approval{{Status: ApprovalStatusApproved, FunctionCallID: "call", ConfirmationCallID: "confirmation"}}}) {
		t.Fatal("running resolved approval context should remain continuable")
	}
	if runCanContinueResolvedApproval(Run{Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning}) {
		t.Fatal("workflow parents must be continued through their child workflow, not direct approval resume")
	}
}
