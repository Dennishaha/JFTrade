package adk

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPauseGoalRunEnforcesRootActiveGoalBoundaries(t *testing.T) {
	ctx := t.Context()
	var nilRuntime *Runtime
	if _, err := nilRuntime.PauseGoalRun(ctx, "run"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil runtime pause err = %v", err)
	}

	runtime := newTestRuntime(t)
	if _, err := runtime.PauseGoalRun(ctx, "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing run pause err = %v", err)
	}
	now := nowString()
	base := Run{
		SessionID: "session-pause-boundaries", AgentID: "agent-pause-boundaries",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: now, UpdatedAt: now,
	}
	cases := []struct {
		name string
		run  Run
		want string
	}{
		{name: "child goal", run: withLifecycleRun(base, func(run *Run) { run.ParentRunID = "parent" }), want: "only root goal runs"},
		{name: "chat run", run: withLifecycleRun(base, func(run *Run) { run.WorkMode = WorkModeChat; run.WorkflowStatus = "" }), want: "only loop goal runs"},
		{name: "terminal goal", run: withLifecycleRun(base, func(run *Run) { run.Status = RunStatusCompleted }), want: "terminal runs cannot be paused"},
		{name: "system paused goal", run: withLifecycleRun(base, func(run *Run) { run.Status = RunStatusPaused; run.PausedReason = "iteration_limit" }), want: "system-paused runs"},
		{name: "pending goal", run: withLifecycleRun(base, func(run *Run) { run.Status = RunStatusPending }), want: "only running goal runs"},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := tc.run
			run.ID = "pause-boundary-" + string(rune('a'+index))
			mustSaveRun(t, runtime, run)
			if _, err := runtime.PauseGoalRun(ctx, run.ID); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("PauseGoalRun err = %v, want containing %q", err, tc.want)
			}
		})
	}

	requested := base
	requested.ID = "pause-requested"
	mustSaveRun(t, runtime, requested)
	paused, err := runtime.PauseGoalRun(ctx, requested.ID)
	if err != nil {
		t.Fatalf("PauseGoalRun running goal: %v", err)
	}
	if paused.PauseRequestedAt == nil || paused.ResumeState != "user_pause_requested" || paused.Status != RunStatusRunning {
		t.Fatalf("pause-requested run = %#v", paused)
	}

	alreadyPaused := base
	alreadyPaused.ID = "pause-idempotent"
	alreadyPaused.Status = RunStatusPaused
	alreadyPaused.PausedReason = "user"
	mustSaveRun(t, runtime, alreadyPaused)
	got, err := runtime.PauseGoalRun(ctx, alreadyPaused.ID)
	if err != nil || got.Status != RunStatusPaused || got.PausedReason != "user" {
		t.Fatalf("idempotent pause = %#v, %v", got, err)
	}
}

func TestResumeGoalRunRejectsNonResumableStates(t *testing.T) {
	ctx := t.Context()
	var nilRuntime *Runtime
	if _, err := nilRuntime.ResumeGoalRun(ctx, "run"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil runtime resume err = %v", err)
	}

	runtime := newTestRuntime(t)
	if _, err := runtime.ResumeGoalRun(ctx, "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing run resume err = %v", err)
	}
	now := nowString()
	base := Run{
		SessionID: "session-resume-boundaries", AgentID: "agent-resume-boundaries",
		Status: RunStatusPaused, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused,
		PausedReason: "user", CreatedAt: now, UpdatedAt: now,
	}
	cases := []struct {
		name string
		run  Run
		want string
	}{
		{name: "child goal", run: withLifecycleRun(base, func(run *Run) { run.ParentRunID = "parent" }), want: "only root goal runs"},
		{name: "chat run", run: withLifecycleRun(base, func(run *Run) { run.WorkMode = WorkModeChat; run.WorkflowStatus = "" }), want: "only loop goal runs"},
		{name: "running goal", run: withLifecycleRun(base, func(run *Run) { run.Status = RunStatusRunning; run.PausedReason = "" }), want: "only resumable paused goal runs"},
		{name: "unsupported pause reason", run: withLifecycleRun(base, func(run *Run) { run.PausedReason = "system_failure" }), want: "only resumable paused goal runs"},
	}
	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := tc.run
			run.ID = "resume-boundary-" + string(rune('a'+index))
			mustSaveRun(t, runtime, run)
			if _, err := runtime.ResumeGoalRun(ctx, run.ID); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ResumeGoalRun err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestReconcileExpiredRunsCancelsTimedOutRuns(t *testing.T) {
	ctx := t.Context()
	var nilRuntime *Runtime
	nilRuntime.ReconcileExpiredRuns(ctx)

	runtime := newTestRuntime(t)
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	recent := time.Now().UTC().Format(time.RFC3339Nano)
	timeoutRun := mustSaveRun(t, runtime, Run{
		ID: "expired-run", SessionID: "session-expired", AgentID: "agent-expired",
		Status: RunStatusRunning, WorkMode: WorkModeChat,
		StartedAt: old, CreatedAt: old, UpdatedAt: old, MaxDurationMs: 1,
		ToolCalls: []ToolCall{
			{ID: "call-running", ToolName: "market.snapshot", Status: "RUNNING"},
			{ID: "call-done", ToolName: "market.candles", Status: "SUCCEEDED"},
		},
		Usage: &RunUsage{},
	})
	cancelled := false
	runtime.activeMu.Lock()
	runtime.activeRuns[timeoutRun.ID] = func() { cancelled = true }
	runtime.activeMu.Unlock()

	freshRun := mustSaveRun(t, runtime, Run{
		ID: "fresh-running-run", SessionID: "session-expired", AgentID: "agent-expired",
		Status: RunStatusRunning, WorkMode: WorkModeChat,
		StartedAt: recent, CreatedAt: recent, UpdatedAt: recent, MaxDurationMs: int64(time.Hour / time.Millisecond),
	})
	badStartedRun := mustSaveRun(t, runtime, Run{
		ID: "bad-started-run", SessionID: "session-expired", AgentID: "agent-expired",
		Status: RunStatusRunning, WorkMode: WorkModeChat,
		StartedAt: "not-time", CreatedAt: "also-not-time", UpdatedAt: recent, MaxDurationMs: 1,
	})
	runtime.ReconcileExpiredRuns(ctx)
	if !cancelled {
		t.Fatal("expired active run was not cancelled")
	}
	expired, ok, err := runtime.Store().Run(ctx, timeoutRun.ID)
	if err != nil || !ok {
		t.Fatalf("expired Run ok=%v err=%v", ok, err)
	}
	if expired.Status != RunStatusTimedOut || expired.ToolCalls[0].Status != "FAILED" || expired.ToolCalls[0].Error == nil || expired.CompletedAt == nil || expired.Usage.DurationMs == 0 {
		t.Fatalf("expired run after reconcile = %+v", expired)
	}
	fresh, _, err := runtime.Store().Run(ctx, freshRun.ID)
	if err != nil || fresh.Status != RunStatusRunning {
		t.Fatalf("fresh run after reconcile = %+v err=%v", fresh, err)
	}
	badStarted, _, err := runtime.Store().Run(ctx, badStartedRun.ID)
	if err != nil || badStarted.Status != RunStatusRunning {
		t.Fatalf("bad started run after reconcile = %+v err=%v", badStarted, err)
	}

	closedStore := newClosedStoreForLifecycle(t)
	(&Runtime{store: closedStore}).ReconcileExpiredRuns(ctx)
}

func TestCancelRunTreeAndRunLifecycleHelpers(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "cancel-parent", SessionID: "session-cancel", AgentID: "agent-cancel",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		ChildRunIDs: []string{"", "cancel-parent", "cancel-child-explicit"},
		WorkflowPlan: []WorkflowStepState{
			{TaskID: "done-step", Title: "Done", Status: "DONE"},
			{TaskID: "todo-step", Title: "Todo", Status: "TODO"},
		},
		ToolCalls:        []ToolCall{{ID: "parent-call", ToolName: "tool", Status: "PENDING_APPROVAL", RequiresUser: true}},
		PendingApprovals: []Approval{{ID: "approval-cancel", RunID: "cancel-parent", Status: ApprovalStatusPending}},
		CreatedAt:        now, UpdatedAt: now, Usage: &RunUsage{},
	})
	explicitChild := mustSaveRun(t, runtime, Run{
		ID: "cancel-child-explicit", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusPending, WorkMode: WorkModeChat, CreatedAt: now, UpdatedAt: now,
		ToolCalls: []ToolCall{{ID: "child-call", ToolName: "tool", Status: "RUNNING"}},
	})
	discoveredChild := mustSaveRun(t, runtime, Run{
		ID: "cancel-child-discovered", SessionID: parent.SessionID, AgentID: parent.AgentID, ParentRunID: parent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeChat, CreatedAt: now, UpdatedAt: now,
	})
	cancelled, err := runtime.CancelRun(ctx, parent.ID)
	if err != nil {
		t.Fatalf("CancelRun parent: %v", err)
	}
	if cancelled.Status != RunStatusCancelled || cancelled.WorkflowStatus != workflowStatusFailed || cancelled.WorkflowPlan[1].Status != "BLOCKED" || cancelled.ToolCalls[0].Status != "CANCELLED" || cancelled.ToolCalls[0].RequiresUser {
		t.Fatalf("cancelled parent = %+v", cancelled)
	}
	for _, childID := range []string{explicitChild.ID, discoveredChild.ID} {
		child, ok, err := runtime.Store().Run(ctx, childID)
		if err != nil || !ok || child.Status != RunStatusCancelled {
			t.Fatalf("child %s after cancel = %+v ok=%v err=%v", childID, child, ok, err)
		}
	}
	if terminal, err := runtime.CancelRun(ctx, cancelled.ID); err != nil || terminal.Status != RunStatusCancelled {
		t.Fatalf("CancelRun terminal = %+v err=%v", terminal, err)
	}
	if _, err := runtime.CancelRun(ctx, "missing-run"); err == nil || !strings.Contains(err.Error(), "run not found") {
		t.Fatalf("CancelRun missing err = %v", err)
	}

	activeRuntime := &Runtime{activeRuns: map[string]context.CancelFunc{}, approvalRuns: map[string]struct{}{}}
	if activeRuntime.runExecutionInFlight(" ") {
		t.Fatal("blank run should not be in-flight")
	}
	activeRuntime.activeRuns["active"] = func() {}
	if !activeRuntime.runExecutionInFlight(" active ") {
		t.Fatal("active run was not detected")
	}
	delete(activeRuntime.activeRuns, "active")
	activeRuntime.approvalRuns["approval"] = struct{}{}
	if !activeRuntime.runExecutionInFlight("approval") {
		t.Fatal("approval run was not detected")
	}
	if (*Runtime)(nil).runExecutionInFlight("run") {
		t.Fatal("nil runtime should not report in-flight")
	}

	if got := runTimeoutForRun(Run{MaxDurationMs: 25}); got != 25*time.Millisecond {
		t.Fatalf("runTimeoutForRun = %v, want 25ms", got)
	}
	if got := runStatusForContext(context.Background(), errors.New("failed")); got != RunStatusFailed {
		t.Fatalf("runStatusForContext failed = %q", got)
	}
	deadlineCtx, cancelDeadline := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancelDeadline()
	<-deadlineCtx.Done()
	if got := runStatusForContext(deadlineCtx, deadlineCtx.Err()); got != RunStatusTimedOut {
		t.Fatalf("deadline status = %q", got)
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := runStatusForContext(cancelCtx, cancelCtx.Err()); got != RunStatusCancelled {
		t.Fatalf("canceled status = %q", got)
	}
	if kind := runLifecycleAuditKind(RunStatusDenied); kind != "run.denied" {
		t.Fatalf("denied audit kind = %q", kind)
	}
	if kind := runLifecycleAuditKind(RunStatusCompleted); kind != "run.completed" {
		t.Fatalf("completed audit kind = %q", kind)
	}
	if code := runErrorCode("other"); code != "MODEL_CALL_FAILED" {
		t.Fatalf("default error code = %q", code)
	}
}

func TestUpdateRunObjectiveAndRecentMessageBranches(t *testing.T) {
	ctx := t.Context()
	var nilRuntime *Runtime
	if _, err := nilRuntime.UpdateRunObjective(ctx, "run", "objective"); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil UpdateRunObjective err = %v", err)
	}
	runtime := newTestRuntime(t)
	if _, err := runtime.UpdateRunObjective(ctx, "run", " "); err == nil || !strings.Contains(err.Error(), "objective is required") {
		t.Fatalf("blank UpdateRunObjective err = %v", err)
	}
	if _, err := runtime.UpdateRunObjective(ctx, "missing", "new objective"); err == nil || !strings.Contains(err.Error(), "run not found") {
		t.Fatalf("missing UpdateRunObjective err = %v", err)
	}
	now := nowString()
	chat := mustSaveRun(t, runtime, Run{ID: "objective-chat", Status: RunStatusRunning, WorkMode: WorkModeChat, CreatedAt: now, UpdatedAt: now})
	if _, err := runtime.UpdateRunObjective(ctx, chat.ID, "new objective"); err == nil || !strings.Contains(err.Error(), "goal runs") {
		t.Fatalf("chat objective err = %v", err)
	}
	child := mustSaveRun(t, runtime, Run{ID: "objective-child", Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, ParentRunID: "parent", CreatedAt: now, UpdatedAt: now})
	if _, err := runtime.UpdateRunObjective(ctx, child.ID, "new objective"); err == nil || !strings.Contains(err.Error(), "child run") {
		t.Fatalf("child objective err = %v", err)
	}
	terminal := mustSaveRun(t, runtime, Run{ID: "objective-terminal", Status: RunStatusCompleted, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: now, UpdatedAt: now})
	if _, err := runtime.UpdateRunObjective(ctx, terminal.ID, "new objective"); err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("terminal objective err = %v", err)
	}
	goal := mustSaveRun(t, runtime, Run{ID: "objective-goal", Status: RunStatusPending, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, CreatedAt: now, UpdatedAt: now})
	updated, err := runtime.UpdateRunObjective(ctx, goal.ID, "  sharper objective  ")
	if err != nil || updated.Objective != "sharper objective" {
		t.Fatalf("UpdateRunObjective success = %+v err=%v", updated, err)
	}

	if recent := recentOpenAIMessages(nil, 10, 100); recent != nil {
		t.Fatalf("recent nil = %#v, want nil", recent)
	}
	messages := []Message{
		{Role: "assistant", Content: "  "},
		{Role: "assistant", Content: "绛夊緟鐢ㄦ埛瀹℃壒"},
		{Role: "user", Content: "first user"},
		{Role: "assistant", Content: "assistant answer"},
		{Role: "tool", Content: "tool says hello"},
		{Role: "user", Content: strings.Repeat("x", 20)},
	}
	recent := recentOpenAIMessages(messages, 4, 18)
	if len(recent) != 2 || recent[0].Role != "user" || recent[0].Content != "first user" || recent[1].Role != "assistant" || recent[1].Content != "assistan" {
		t.Fatalf("recent messages = %#v", recent)
	}
}

func withLifecycleRun(run Run, mutate func(*Run)) Run {
	mutate(&run)
	return run
}

func newClosedStoreForLifecycle(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(dir+"/adk.db", dir+"/secrets/adk.json", dir+"/skills")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	jftradeCheckTestError(t, store.Close())
	return store
}
