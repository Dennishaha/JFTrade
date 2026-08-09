package workflowexec

import (
	"strings"
	"testing"

	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestGoalTurnPersistsUserPauseAndTerminatesFailedChildren(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	executor := (&WorkflowExecutor{runtime: runtime})
	session := mustCreateSession(t, runtime, "coverage-goal-turn-agent", "goal turn recovery")
	now := jfadkmodel.NowString()

	t.Run("a model pause request honors an already persisted user pause", func(t *testing.T) {
		pauseRequestedAt := jfadkmodel.NowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage-goal-turn-user-pause", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			PauseRequestedAt: &pauseRequestedAt, Message: "model was interrupted",
			CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
		})

		paused, reply, done, prompt, err := executor.PrepareGoalWorkflowTurn(
			ctx,
			workflowRequest{Session: session},
			parent,
			nil,
			&fakeWorkflowExecutionHandle{},
			jfadkmodel.ErrUserGoalPauseRequested,
			3,
		)
		if err != nil {
			t.Fatalf("prepare paused goal turn: %v", err)
		}
		if !done || prompt != "" || paused.Status != RunStatusPaused || paused.PausedReason != "user" || paused.Iteration != 3 {
			t.Fatalf("paused goal turn = %+v reply=%+v done=%v prompt=%q", paused, reply, done, prompt)
		}
		if reply.Reply != "目标已暂停。" {
			t.Fatalf("paused goal reply = %q, want user-facing pause acknowledgement", reply.Reply)
		}
	})

	t.Run("failed direct child ends its parent rather than scheduling another turn", func(t *testing.T) {
		terminalTask, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "coverage-goal-turn-terminal-task", Title: "terminal child", Status: "IN_PROGRESS", RunID: "coverage-goal-turn-terminal-parent",
		})
		if err != nil {
			t.Fatalf("SaveTask terminal child: %v", err)
		}
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage-goal-turn-terminal-parent", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: terminalTask.ID, ChildRunID: "coverage-goal-turn-terminal-child", Status: "IN_PROGRESS"}},
			CreatedAt:    now, UpdatedAt: now, Usage: &RunUsage{},
		})
		mustSaveRun(t, runtime, Run{
			ID: "coverage-goal-turn-terminal-child", SessionID: session.ID, AgentID: session.AgentID, ParentRunID: parent.ID,
			Status: RunStatusFailed, Message: "worker exhausted retry budget", FailureReason: "worker exhausted retry budget", ErrorCode: "CHILD_RETRY_EXHAUSTED",
			CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
		})

		terminated, reply, done, prompt, err := executor.PrepareGoalWorkflowTurn(
			ctx,
			workflowRequest{Session: session},
			parent,
			nil,
			&fakeWorkflowExecutionHandle{},
			nil,
			2,
		)
		if err != nil {
			t.Fatalf("prepare terminal-child goal turn: %v", err)
		}
		if !done || prompt != "" || terminated.Status != RunStatusFailed || terminated.ErrorCode != "CHILD_RETRY_EXHAUSTED" || !strings.Contains(reply.Reply, "retry budget") {
			t.Fatalf("terminal child handling = %+v reply=%+v done=%v prompt=%q", terminated, reply, done, prompt)
		}
	})
}

func TestGoalTurnFailsClosedWhenTaskStateCannotBeRead(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	executor := (&WorkflowExecutor{runtime: runtime})
	session := mustCreateSession(t, runtime, "coverage-goal-store-agent", "goal turn storage failure")
	parent := mustSaveRun(t, runtime, Run{
		ID: "coverage-goal-turn-task-store-failure", SessionID: session.ID, AgentID: session.AgentID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.Store().DB().ExecContext(ctx, `DROP TABLE `+enginepersistence.TableTasks); err != nil {
		t.Fatalf("drop task table: %v", err)
	}

	failed, reply, done, prompt, err := executor.PrepareGoalWorkflowTurn(
		ctx,
		workflowRequest{Session: session},
		parent,
		nil,
		&fakeWorkflowExecutionHandle{},
		nil,
		1,
	)
	if err != nil {
		t.Fatalf("prepare task-store failure turn: %v", err)
	}
	if !done || prompt != "" || failed.Status != RunStatusFailed || !strings.Contains(failed.FailureReason, enginepersistence.TableTasks) || !strings.Contains(reply.Reply, enginepersistence.TableTasks) {
		t.Fatalf("task-store failure = %+v reply=%+v done=%v prompt=%q", failed, reply, done, prompt)
	}
}

func TestGoalWorkflowSaveFailureReturnsFailedResponseWithoutRunningModel(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	executor := (&WorkflowExecutor{runtime: runtime})
	session := mustCreateSession(t, runtime, "coverage-goal-save-agent", "goal workflow save failure")
	parent := Run{
		ID: "coverage-goal-initial-save-failure", SessionID: session.ID, AgentID: session.AgentID,
		Status: RunStatusPending, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	}
	if _, err := runtime.Store().DB().ExecContext(ctx, `CREATE TRIGGER coverage_fail_goal_initial_save BEFORE INSERT ON `+enginepersistence.TableRuns+` BEGIN SELECT RAISE(FAIL, 'goal initial save failed'); END`); err != nil {
		t.Fatalf("create run-save failure trigger: %v", err)
	}

	response, err := executor.RunADKGoalWorkflow(ctx, workflowRequest{Session: session, RunOptions: RunOptions{LoopMaxIterations: 1}}, parent, nil)
	if err == nil || !strings.Contains(err.Error(), "persist failed workflow state") {
		t.Fatalf("RunADKGoalWorkflow error = %v, want durable failure-state error", err)
	}
	if response.Run.ID != "" {
		t.Fatalf("initial save failure response = %+v, want no successful response", response)
	}
}
