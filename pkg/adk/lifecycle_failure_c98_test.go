package adk

import (
	"context"
	"strings"
	"testing"
)

func TestCoverage98StaleRunReconciliationCoversTerminalPlanAndSelfReferenceRecovery(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executor := runtime.workflowExecutor()
	now := nowString()

	// A stale row can disappear between ListRuns and Run; recovery must simply
	// skip it rather than turning an already-deleted run into an error.
	runtime.reconcileStaleRun(ctx, executor, Run{ID: "deleted-before-reconcile"})

	unsupported := mustSaveRun(t, runtime, Run{
		ID: "unsupported-stale-status", Status: "NOT_A_LIFECYCLE_STATE", WorkMode: WorkModeChat,
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	runtime.reconcileStaleRun(ctx, executor, unsupported)
	storedUnsupported, ok, err := runtime.Store().Run(ctx, unsupported.ID)
	if err != nil || !ok || storedUnsupported.Status != unsupported.Status {
		t.Fatalf("unsupported stale status = %+v, ok=%v err=%v", storedUnsupported, ok, err)
	}

	pending := mustSaveRun(t, runtime, Run{
		ID: "pending-without-recoverable-context", SessionID: "reconcile-session", AgentID: "agent",
		Status: RunStatusPending, WorkMode: WorkModeChat, CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	runtime.reconcileStaleRun(ctx, executor, pending)
	failedPending, ok, err := runtime.Store().Run(ctx, pending.ID)
	if err != nil || !ok || failedPending.Status != RunStatusFailed || failedPending.ResumeState != "approval_context_missing" {
		t.Fatalf("unrecoverable pending run = %+v, ok=%v err=%v", failedPending, ok, err)
	}

	completedParent := mustSaveRun(t, runtime, Run{
		ID: "completed-parent-awaiting-reconcile", SessionID: "reconcile-session", AgentID: "agent",
		Status: RunStatusCompleted, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
	})
	if !runtime.reconcileCompletedWorkflowParent(ctx, executor, completedParent) {
		t.Fatal("completed workflow parent should be consumed by child reconciliation")
	}

	terminalParent := Run{
		ID: "terminal-plan-refresh", SessionID: "reconcile-session", AgentID: "agent",
		Status: RunStatusFailed, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusFailed,
		WorkflowPlan: []WorkflowStepState{{TaskID: "terminal-plan-task", Status: "TODO"}},
		CreatedAt:    now, UpdatedAt: now, Usage: &RunUsage{},
	}
	if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "terminal-plan-task", Title: "Persist final result", Status: "DONE", RunID: terminalParent.ID,
	}); err != nil {
		t.Fatalf("SaveTask terminal plan: %v", err)
	}
	mustSaveRun(t, runtime, terminalParent)
	if !runtime.reconcileTerminalStaleRun(ctx, executor, terminalParent) {
		t.Fatal("terminal workflow run should be reconciled")
	}
	refreshedTerminal, ok, err := runtime.Store().Run(ctx, terminalParent.ID)
	if err != nil || !ok || len(refreshedTerminal.WorkflowPlan) != 1 || refreshedTerminal.WorkflowPlan[0].Status != "DONE" {
		t.Fatalf("terminal workflow plan refresh = %+v, ok=%v err=%v", refreshedTerminal, ok, err)
	}

	selfReferenced := Run{
		ID: "self-reference-via-task", SessionID: "reconcile-session", AgentID: "agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		WorkflowPlan: []WorkflowStepState{{TaskID: "self-reference-via-task-task", Executor: workflowTaskExecutorChild, Status: "IN_PROGRESS"}},
		CreatedAt:    now, UpdatedAt: now, Usage: &RunUsage{},
	}
	if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "self-reference-via-task-task", Title: "Delegate", Status: "IN_PROGRESS", RunID: selfReferenced.ID, Executor: workflowTaskExecutorChild,
	}); err != nil {
		t.Fatalf("SaveTask self reference: %v", err)
	}
	mustSaveRun(t, runtime, selfReferenced)
	if repaired, err := runtime.repairWorkflowSelfReference(ctx, &selfReferenced); err != nil || !repaired {
		t.Fatalf("task-backed self reference repaired=%v err=%v", repaired, err)
	}
	if selfReferenced.Status != RunStatusPaused || selfReferenced.WorkflowPlan[0].Status != "TODO" || selfReferenced.WorkflowPlan[0].Executor != "" {
		t.Fatalf("task-backed self-reference recovery = %+v", selfReferenced)
	}
}

func TestCoverage98LifecycleStoreFailuresAreReturnedToTheCaller(t *testing.T) {
	ctx := context.Background()

	t.Run("goal control writes do not report success when the run row rejects updates", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			run  Run
			call func(*Runtime, string) error
		}{
			{
				name: "pause",
				run:  Run{Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning},
				call: func(runtime *Runtime, id string) error {
					_, err := runtime.PauseGoalRun(ctx, id)
					return err
				},
			},
			{
				name: "resume",
				run:  Run{Status: RunStatusPaused, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused, PausedReason: "user"},
				call: func(runtime *Runtime, id string) error {
					_, err := runtime.ResumeGoalRun(ctx, id)
					return err
				},
			},
			{
				name: "objective",
				run:  Run{Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning, Objective: "before"},
				call: func(runtime *Runtime, id string) error {
					_, err := runtime.UpdateRunObjective(ctx, id, "after")
					return err
				},
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				runtime := newTestRuntime(t)
				run := tc.run
				run.ID = "goal-write-failure-" + tc.name
				run.SessionID = "goal-write-failure-session"
				run.AgentID = "agent"
				run.CreatedAt = nowString()
				run.UpdatedAt = nowString()
				run.Usage = &RunUsage{}
				mustSaveRun(t, runtime, run)
				if _, err := runtime.Store().db.ExecContext(ctx, `CREATE TRIGGER reject_`+tc.name+`_run_update BEFORE UPDATE ON `+tableRuns+` WHEN NEW.id = '`+run.ID+`' BEGIN SELECT RAISE(FAIL, 'run write rejected'); END`); err != nil {
					t.Fatalf("create write rejection trigger: %v", err)
				}
				if err := tc.call(runtime, run.ID); err == nil || !strings.Contains(err.Error(), "run write rejected") {
					t.Fatalf("%s write failure = %v", tc.name, err)
				}
			})
		}
	})

	t.Run("self-reference repair surfaces task storage errors", func(t *testing.T) {
		runtime := newTestRuntime(t)
		parent := Run{
			ID: "self-reference-task-error", Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "missing-after-schema-fault", Executor: workflowTaskExecutorChild, Status: "IN_PROGRESS"}},
			CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		}
		mustSaveRun(t, runtime, parent)
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableTasks); err != nil {
			t.Fatalf("drop task table: %v", err)
		}
		if repaired, err := runtime.repairWorkflowSelfReference(ctx, &parent); err == nil || repaired {
			t.Fatalf("repair after task-store failure = repaired:%v err:%v", repaired, err)
		}
	})
}
