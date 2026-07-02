package adk

import (
	"strings"
	"testing"
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

func withLifecycleRun(run Run, mutate func(*Run)) Run {
	mutate(&run)
	return run
}
