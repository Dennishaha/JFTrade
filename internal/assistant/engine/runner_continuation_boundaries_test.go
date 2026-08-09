package adk

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestContinuationClaimGuardsAndClosingRuntime(t *testing.T) {
	var nilRuntime *Runtime
	if nilRuntime.claimApprovalContinuation("run") || nilRuntime.claimInputContinuation("run") {
		t.Fatal("nil runtime claimed a continuation")
	}
	nilRuntime.releaseApprovalContinuation("run")
	nilRuntime.releaseInputContinuation("run")
	nilRuntime.enqueueResolvedApprovalContinuation("run")
	nilRuntime.enqueueResolvedInputContinuation("run")

	runtime := newTestRuntime(t)
	if runtime.claimApprovalContinuation(" ") || runtime.claimInputContinuation(" ") {
		t.Fatal("blank run id claimed a continuation")
	}
	if !runtime.claimApprovalContinuation("run-approval-claim") || runtime.claimApprovalContinuation("run-approval-claim") {
		t.Fatal("approval continuation claim was not exclusive")
	}
	runtime.releaseApprovalContinuation("run-approval-claim")
	if !runtime.claimInputContinuation("run-input-claim") || runtime.claimInputContinuation("run-input-claim") {
		t.Fatal("input continuation claim was not exclusive")
	}
	runtime.releaseInputContinuation("run-input-claim")

	runtime.approvalMu.Lock()
	runtime.closing = true
	runtime.approvalMu.Unlock()
	runtime.enqueueResolvedApprovalContinuation("run-closing-approval")
	runtime.enqueueResolvedInputContinuation("run-closing-input")
	runtime.approvalMu.Lock()
	approvalClaims := len(runtime.approvalRuns)
	inputClaims := len(runtime.inputRuns)
	runtime.approvalMu.Unlock()
	if approvalClaims != 0 || inputClaims != 0 {
		t.Fatalf("closing runtime retained claims: approvals=%d inputs=%d", approvalClaims, inputClaims)
	}

	if !runtime.claimInputContinuation("run-finished-error") {
		t.Fatal("claim input before failed finish")
	}
	runtime.finishInputContinuation(t.Context(), "run-finished-error", errors.New("continuation failed"))
	if !runtime.claimInputContinuation("run-finished-cancel") {
		t.Fatal("claim input before cancelled finish")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	runtime.finishInputContinuation(cancelled, "run-finished-cancel", nil)
	if !runtime.claimInputContinuation("run-finished-nil-context") {
		t.Fatal("claim input before nil-context finish")
	}
	runtime.finishInputContinuation(nil, "run-finished-nil-context", nil) //nolint:staticcheck // Exercise the nil-context recovery boundary.
}

func TestContinuationQueuesInitializeWithoutRuntimeBackgroundContext(t *testing.T) {
	store := newExecutionClaimTestStore(t)
	runtime := &Runtime{store: store, executorID: "continuation-boundary"}
	if !runtime.claimApprovalContinuation("initialize-approval-map") {
		t.Fatal("initialize approval claim map")
	}
	runtime.releaseApprovalContinuation("initialize-approval-map")
	if !runtime.claimInputContinuation("initialize-input-map") {
		t.Fatal("initialize input claim map")
	}
	runtime.releaseInputContinuation("initialize-input-map")

	runtime.enqueueResolvedApprovalContinuation("missing-approval-run")
	runtime.enqueueResolvedInputContinuation("missing-input-run")
	runtime.approvalWG.Wait()
	runtime.approvalMu.Lock()
	approvalClaims := len(runtime.approvalRuns)
	inputClaims := len(runtime.inputRuns)
	runtime.approvalMu.Unlock()
	if approvalClaims != 0 || inputClaims != 0 {
		t.Fatalf("completed missing-run continuations retained claims: %d/%d", approvalClaims, inputClaims)
	}
}

func TestResolvedContinuationsHonorForeignLeasesAndEmptyState(t *testing.T) {
	runtime := newTestRuntime(t)
	if err := runtime.continueResolvedApprovalRun(t.Context(), "missing-run"); err != nil {
		t.Fatalf("missing approval continuation: %v", err)
	}
	empty := mustSaveRun(t, runtime, Run{
		ID: "approval-empty-run", SessionID: "approval-empty-session", AgentID: "approval-empty-agent",
		Status: RunStatusRunning, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	if err := runtime.continueResolvedApprovalRun(t.Context(), empty.ID); err != nil {
		t.Fatalf("empty approval continuation: %v", err)
	}
	staged, shouldContinue, err := runtime.stageResolvedApproval(t.Context(), Approval{ID: "missing-approval", RunID: "missing-run"}, true)
	if err != nil || shouldContinue || staged.Approval.ID != "missing-approval" {
		t.Fatalf("stage missing approval = %+v, %v, err=%v", staged, shouldContinue, err)
	}
	if err := runtime.markApprovalContinuationFailed(t.Context(), "missing-run", errors.New("ignored")); err != nil {
		t.Fatalf("mark missing continuation failed: %v", err)
	}
	if _, err := runtime.attachParentWorkflowResolution(t.Context(), ApprovalResolution{}); err != nil {
		t.Fatalf("attach empty parent resolution: %v", err)
	}
	if !runHasPendingApproval([]Approval{{Status: ApprovalStatusPending}}) || runHasPendingApproval([]Approval{{Status: ApprovalStatusApproved}}) {
		t.Fatal("pending approval classification mismatch")
	}
	if runHasRecoverableApprovalContext(Run{PendingApprovals: []Approval{{Status: ApprovalStatusApproved, FunctionCallID: "call", ConfirmationCallID: "confirmation"}}}) {
		t.Fatal("resolved approval was treated as pending-recoverable")
	}
	if runHasRecoverableResolvedApprovalContext(Run{ResumeState: "waiting_approval", PendingApprovals: []Approval{{Status: ApprovalStatusApproved, FunctionCallID: "call", ConfirmationCallID: "confirmation"}}}) {
		t.Fatal("resolved approval outside resumption was recoverable")
	}
	nilRuntime := (*Runtime)(nil)
	nilRuntime.ReconcileResolvedApprovals(t.Context())

	inputRuntime, _, _, answered := newAnsweredInputRun(t, "foreign-continuation")
	foreign, err := inputRuntime.Store().ClaimRunLease(t.Context(), answered.ID, "executor-foreign", time.Now().UTC(), time.Minute)
	if err != nil {
		t.Fatalf("claim foreign input lease: %v", err)
	}
	defer func() { _ = inputRuntime.Store().ReleaseRunLease(context.Background(), foreign) }()
	if err := inputRuntime.continueResolvedInput(t.Context(), answered.ID); err != nil {
		t.Fatalf("foreign-owned input continuation: %v", err)
	}
	stored, ok, err := inputRuntime.Store().Run(t.Context(), answered.ID)
	if err != nil || !ok || !runHasRecoverableAnsweredInputContext(stored) {
		t.Fatalf("foreign-owned input changed = %+v, %v, %v", stored, ok, err)
	}
}

func TestResolvedApprovalContinuationKeepsSiblingStateAtomic(t *testing.T) {
	t.Run("approved action waits for pending sibling", func(t *testing.T) {
		runtime, run, approvals := newPendingApprovalRun(t, "approval-sibling-remains", 2)
		approved := approvals[0]
		approved.Status = ApprovalStatusApproved
		resolution, err := runtime.continueResolvedApproval(t.Context(), approved, true)
		if err != nil || resolution.Run == nil || resolution.Run.Status != RunStatusPending || resolution.Run.PendingApprovals[1].Status != ApprovalStatusPending {
			t.Fatalf("pending sibling resolution = %+v, err=%v", resolution, err)
		}
		stored, ok, err := runtime.Store().Run(t.Context(), run.ID)
		if err != nil || !ok || stored.PendingApprovals[1].Status != ApprovalStatusPending {
			t.Fatalf("stored pending sibling = %+v, %v, %v", stored, ok, err)
		}
	})

	t.Run("denial closes pending siblings before resume", func(t *testing.T) {
		runtime, run, approvals := newPendingApprovalRun(t, "approval-deny-siblings-direct", 2)
		denied := approvals[0]
		denied.Status = ApprovalStatusDenied
		if _, err := runtime.continueResolvedApproval(t.Context(), denied, false); err == nil {
			t.Fatal("denied continuation unexpectedly resumed without durable execution context")
		}
		storedSibling, ok, err := runtime.Store().Approval(t.Context(), approvals[1].ID)
		if err != nil || !ok || storedSibling.Status != ApprovalStatusDenied {
			t.Fatalf("durable sibling denial = %+v, %v, %v (run %s)", storedSibling, ok, err, run.ID)
		}
	})

	t.Run("unrelated approval cannot replace embedded state", func(t *testing.T) {
		runtime, _, approvals := newPendingApprovalRun(t, "approval-unrelated", 1)
		unrelated := approvals[0]
		unrelated.ID = "approval-not-embedded"
		unrelated.Status = ApprovalStatusApproved
		resolution, err := runtime.continueResolvedApproval(t.Context(), unrelated, true)
		if err != nil || resolution.Run == nil || resolution.Run.Status != RunStatusPending {
			t.Fatalf("unrelated approval resolution = %+v, err=%v", resolution, err)
		}
	})

	t.Run("terminal run cannot restart from old approval", func(t *testing.T) {
		runtime, run, approvals := newPendingApprovalRun(t, "approval-terminal", 1)
		run.Status = RunStatusCompleted
		if err := runtime.Store().SaveRun(t.Context(), run); err != nil {
			t.Fatalf("save terminal run: %v", err)
		}
		resolution, err := runtime.continueResolvedApproval(t.Context(), approvals[0], true)
		if err != nil || resolution.Run != nil {
			t.Fatalf("terminal approval resolution = %+v, err=%v", resolution, err)
		}
	})
}

func TestStartRunPersistsExecutionLeaseClaimFailure(t *testing.T) {
	runtime := newTestRuntime(t)
	ensureTestProvider(t, runtime)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "lease-claim-failure-agent", Name: "Lease Claim Failure", ProviderID: testProviderID, Status: AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "lease claim failure")
	if _, err := runtime.Store().DB().ExecContext(t.Context(), `CREATE TRIGGER reject_run_lease_claim BEFORE INSERT ON `+tableRunLeases+` BEGIN SELECT RAISE(FAIL, 'forced lease claim failure'); END`); err != nil {
		t.Fatalf("create lease trigger: %v", err)
	}
	if _, _, _, err := runtime.startRun(t.Context(), session.ID, agent, "must fail closed"); err == nil || !strings.Contains(err.Error(), "forced lease claim failure") {
		t.Fatalf("startRun lease error = %v", err)
	}
	runs, err := runtime.Store().ListRuns(t.Context())
	if err != nil || len(runs) != 1 || runs[0].Status != RunStatusFailed || runs[0].ErrorCode != "RUN_LEASE_CLAIM_FAILED" {
		t.Fatalf("failed run persistence = %+v, err=%v", runs, err)
	}
}

func TestResolvedApprovalDoesNotStealForeignExecutionLease(t *testing.T) {
	t.Run("synchronous resolution returns durable staged state", func(t *testing.T) {
		runtime, run, approvals := newPendingApprovalRun(t, "approval-sync-foreign-lease", 1)
		lease, err := runtime.Store().ClaimRunLease(t.Context(), run.ID, "executor-foreign", time.Now().UTC(), time.Minute)
		if err != nil {
			t.Fatalf("claim foreign lease: %v", err)
		}
		defer func() { _ = runtime.Store().ReleaseRunLease(context.Background(), lease) }()
		resolution, err := runtime.ResolveApproval(t.Context(), approvals[0].ID, true)
		if err != nil || resolution.Run == nil || resolution.Run.ResumeState != "approval_resuming" {
			t.Fatalf("foreign-owned approval resolution = %+v, err=%v", resolution, err)
		}
	})

	t.Run("background continuation leaves foreign owner untouched", func(t *testing.T) {
		runtime, run, approvals := newPendingApprovalRun(t, "approval-background-foreign-lease", 1)
		resolved, _, staged, shouldContinue, err := runtime.Store().ResolveAndStageApproval(t.Context(), approvals[0].ID, ApprovalStatusApproved)
		if err != nil || !shouldContinue || staged == nil {
			t.Fatalf("stage approval = %+v/%+v/%v, err=%v", resolved, staged, shouldContinue, err)
		}
		lease, err := runtime.Store().ClaimRunLease(t.Context(), run.ID, "executor-foreign", time.Now().UTC(), time.Minute)
		if err != nil {
			t.Fatalf("claim foreign lease: %v", err)
		}
		defer func() { _ = runtime.Store().ReleaseRunLease(context.Background(), lease) }()
		if err := runtime.continueResolvedApprovalRun(t.Context(), run.ID); err != nil {
			t.Fatalf("foreign-owned background continuation: %v", err)
		}
		stored, ok, err := runtime.Store().Run(t.Context(), run.ID)
		if err != nil || !ok || stored.ResumeState != "approval_resuming" {
			t.Fatalf("foreign-owned staged run = %+v, %v, %v", stored, ok, err)
		}
	})
}

func TestLifecycleReportsLeaseStorageFailures(t *testing.T) {
	runtime := newTestRuntime(t)
	now := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	run := mustSaveRun(t, runtime, Run{
		ID: "run-missing-lease-schema", SessionID: "session-missing-lease-schema", AgentID: "agent-missing-lease-schema",
		Status: RunStatusRunning, CreatedAt: now, StartedAt: now, UpdatedAt: now, MaxDurationMs: 1, Usage: &RunUsage{},
	})
	if _, err := runtime.Store().DB().ExecContext(t.Context(), `DROP TABLE `+tableRunLeases); err != nil {
		t.Fatalf("drop run lease table: %v", err)
	}
	if err := runtime.ReconcileExpiredRuns(t.Context()); err == nil || !strings.Contains(err.Error(), tableRunLeases) {
		t.Fatalf("expired-run lease inspection error = %v", err)
	}
	if err := runtime.reconcileStaleRun(t.Context(), mustWorkflowExecutor(t, runtime), run); err == nil || !strings.Contains(err.Error(), tableRunLeases) {
		t.Fatalf("stale-run lease inspection error = %v", err)
	}

	closedStore := newExecutionClaimTestStore(t)
	if err := closedStore.Close(); err != nil {
		t.Fatalf("close lifecycle store: %v", err)
	}
	closedRuntime := &Runtime{store: closedStore}
	if _, err := closedRuntime.PauseGoalRun(t.Context(), "run"); err == nil {
		t.Fatal("closed store paused goal run")
	}
	if _, err := closedRuntime.ResumeGoalRun(t.Context(), "run"); err == nil {
		t.Fatal("closed store resumed goal run")
	}
	if _, err := closedRuntime.UpdateRunObjective(t.Context(), "run", "objective"); err == nil {
		t.Fatal("closed store updated goal objective")
	}
}

func TestGoalResumeFailsClosedWhenExecutionLeaseCannotBeClaimed(t *testing.T) {
	t.Run("fresh foreign lease leaves resumed goal for its owner", func(t *testing.T) {
		runtime := newTestRuntime(t)
		run := mustSaveRun(t, runtime, Run{
			ID: "goal-resume-foreign", SessionID: "goal-resume-session", AgentID: "goal-resume-agent",
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), StartedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		lease, err := runtime.Store().ClaimRunLease(t.Context(), run.ID, "executor-foreign", time.Now().UTC(), time.Minute)
		if err != nil {
			t.Fatalf("claim foreign goal lease: %v", err)
		}
		defer func() { _ = runtime.Store().ReleaseRunLease(context.Background(), lease) }()
		runtime.resumeUserPausedGoalRun(run)
		time.Sleep(50 * time.Millisecond)
		stored, ok, err := runtime.Store().Run(t.Context(), run.ID)
		if err != nil || !ok || stored.Status != RunStatusRunning {
			t.Fatalf("foreign-owned resumed goal = %+v, %v, %v", stored, ok, err)
		}
	})

	t.Run("lease storage error terminates goal truthfully", func(t *testing.T) {
		runtime := newTestRuntime(t)
		run := mustSaveRun(t, runtime, Run{
			ID: "goal-resume-lease-failure", SessionID: "goal-resume-failure-session", AgentID: "goal-resume-failure-agent",
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), StartedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `CREATE TRIGGER reject_goal_resume_lease BEFORE INSERT ON `+tableRunLeases+` BEGIN SELECT RAISE(FAIL, 'forced goal resume lease failure'); END`); err != nil {
			t.Fatalf("create goal lease trigger: %v", err)
		}
		runtime.resumeUserPausedGoalRun(run)
		failed := waitForRunStatus(t, runtime, run.ID, RunStatusFailed)
		if !strings.Contains(failed.FailureReason, "forced goal resume lease failure") {
			t.Fatalf("goal lease failure = %+v", failed)
		}
	})
}

func TestGoBackgroundNilGuardsAndClosingState(t *testing.T) {
	var nilRuntime *Runtime
	if nilRuntime.goBackground(func(ctx context.Context) {}) {
		t.Fatal("nil runtime accepted background work")
	}

	runtime := newTestRuntime(t)
	if runtime.goBackground(nil) {
		t.Fatal("runtime accepted nil function")
	}

	executed := make(chan bool, 1)
	if !runtime.goBackground(func(ctx context.Context) {
		executed <- true
	}) {
		t.Fatal("valid background work was rejected")
	}
	select {
	case <-executed:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("background work did not execute")
	}

	runtime.approvalMu.Lock()
	runtime.closing = true
	runtime.approvalMu.Unlock()
	if runtime.goBackground(func(ctx context.Context) {
		t.Fatal("closing runtime executed background work")
	}) {
		t.Fatal("closing runtime accepted background work")
	}
}

func TestRuntimeCloseWaitsForInFlightBackgroundWorkAndRejectsNewWork(t *testing.T) {
	runtime := newTestRuntime(t)
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	if !runtime.goBackground(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(canceled)
		<-release
		close(finished)
	}) {
		t.Fatal("runtime rejected background work before closing")
	}
	<-started

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- runtime.Close()
	}()

	canceledBeforeTimeout := false
	select {
	case <-canceled:
		canceledBeforeTimeout = true
	case <-time.After(time.Second):
	}
	closeReturnedEarly := false
	select {
	case <-closeDone:
		closeReturnedEarly = true
	default:
	}
	acceptedWhileClosing := runtime.goBackground(func(ctx context.Context) {
		t.Error("closing runtime executed newly admitted background work")
	})
	close(release)
	<-finished
	if !closeReturnedEarly {
		if err := <-closeDone; err != nil {
			t.Fatalf("close runtime: %v", err)
		}
	}

	if !canceledBeforeTimeout {
		t.Fatal("Close did not cancel in-flight background work")
	}
	if closeReturnedEarly {
		t.Fatal("Close returned before in-flight background work finished")
	}
	if acceptedWhileClosing {
		t.Fatal("closing runtime accepted new background work")
	}
	if runtime.goBackground(func(ctx context.Context) {}) {
		t.Fatal("closed runtime accepted new background work")
	}
}

func TestGoBackgroundUsesBackgroundCtxOrFallback(t *testing.T) {
	runtime := newTestRuntime(t)
	runtime.backgroundCtx = nil

	receivedCtx := make(chan context.Context, 1)
	if !runtime.goBackground(func(ctx context.Context) {
		receivedCtx <- ctx
	}) {
		t.Fatal("runtime rejected background work with nil backgroundCtx")
	}

	select {
	case ctx := <-receivedCtx:
		if ctx == nil {
			t.Fatal("background work received nil context")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("background work did not execute")
	}
}

func TestGoalResumeExecutionErrorPaths(t *testing.T) {
	t.Run("workflowResumeContext session not found marks run as failed", func(t *testing.T) {
		runtime := newTestRuntime(t)
		run := mustSaveRun(t, runtime, Run{
			ID: "goal-resume-no-session", SessionID: "session-does-not-exist", AgentID: "agent-no-session",
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), StartedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		runtime.resumeUserPausedGoalRun(run)
		failed := waitForRunStatus(t, runtime, run.ID, RunStatusFailed)
		if !strings.Contains(failed.FailureReason, "session not found") {
			t.Fatalf("workflowResumeContext failure reason = %q, want containing %q", failed.FailureReason, "session not found")
		}
	})

	t.Run("resumeADKGoalWorkflow save failure marks run as failed", func(t *testing.T) {
		runtime := newTestRuntime(t)
		ensureTestProvider(t, runtime)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{ID: "agent-goal-wf-fail", Name: "Agent Goal WF Fail"})
		session := mustCreateSession(t, runtime, agent.ID, "goal-wf-fail")
		run := mustSaveRun(t, runtime, Run{
			ID: "goal-resume-wf-save-fail", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), StartedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `CREATE TRIGGER reject_wf_resume BEFORE UPDATE ON `+tableRuns+` WHEN NEW.id = 'goal-resume-wf-save-fail' AND json_extract(NEW.payload_json, '$.resumeState') = 'user_resuming' BEGIN SELECT RAISE(FAIL, 'forced workflow save failure'); END`); err != nil {
			t.Fatalf("create workflow trigger: %v", err)
		}
		runtime.resumeUserPausedGoalRun(run)
		failed := waitForRunStatus(t, runtime, run.ID, RunStatusFailed)
		if !strings.Contains(failed.FailureReason, "forced workflow save failure") {
			t.Fatalf("resumeADKGoalWorkflow failure reason = %q, want containing %q", failed.FailureReason, "forced workflow save failure")
		}
	})
}
