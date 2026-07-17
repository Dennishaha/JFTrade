package adk

import (
	"strings"
	"testing"
)

func TestCoverage98GoalTurnPersistsUserPauseAndTerminatesFailedChildren(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	executor := runtime.workflowExecutor()
	session := mustCreateSession(t, runtime, "coverage-goal-turn-agent", "goal turn recovery")
	now := nowString()

	t.Run("a model pause request honors an already persisted user pause", func(t *testing.T) {
		pauseRequestedAt := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "coverage-goal-turn-user-pause", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			PauseRequestedAt: &pauseRequestedAt, Message: "model was interrupted",
			CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
		})

		paused, reply, done, prompt := executor.prepareGoalWorkflowTurn(
			ctx,
			workflowRequest{Session: session},
			parent,
			nil,
			newBareGoogleADKExecution(parent.ID),
			errUserGoalPauseRequested,
			3,
		)
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

		terminated, reply, done, prompt := executor.prepareGoalWorkflowTurn(
			ctx,
			workflowRequest{Session: session},
			parent,
			nil,
			newBareGoogleADKExecution(parent.ID),
			nil,
			2,
		)
		if !done || prompt != "" || terminated.Status != RunStatusFailed || terminated.ErrorCode != "CHILD_RETRY_EXHAUSTED" || !strings.Contains(reply.Reply, "retry budget") {
			t.Fatalf("terminal child handling = %+v reply=%+v done=%v prompt=%q", terminated, reply, done, prompt)
		}
	})
}

func TestCoverage98GoalTurnFailsClosedWhenTaskStateCannotBeRead(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	executor := runtime.workflowExecutor()
	session := mustCreateSession(t, runtime, "coverage-goal-store-agent", "goal turn storage failure")
	parent := mustSaveRun(t, runtime, Run{
		ID: "coverage-goal-turn-task-store-failure", SessionID: session.ID, AgentID: session.AgentID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableTasks); err != nil {
		t.Fatalf("drop task table: %v", err)
	}

	failed, reply, done, prompt := executor.prepareGoalWorkflowTurn(
		ctx,
		workflowRequest{Session: session},
		parent,
		nil,
		newBareGoogleADKExecution(parent.ID),
		nil,
		1,
	)
	if !done || prompt != "" || failed.Status != RunStatusFailed || !strings.Contains(failed.FailureReason, tableTasks) || !strings.Contains(reply.Reply, tableTasks) {
		t.Fatalf("task-store failure = %+v reply=%+v done=%v prompt=%q", failed, reply, done, prompt)
	}
}

func TestCoverage98GoalWorkflowSaveFailureReturnsFailedResponseWithoutRunningModel(t *testing.T) {
	ctx := t.Context()
	runtime := newTestRuntime(t)
	executor := runtime.workflowExecutor()
	session := mustCreateSession(t, runtime, "coverage-goal-save-agent", "goal workflow save failure")
	parent := Run{
		ID: "coverage-goal-initial-save-failure", SessionID: session.ID, AgentID: session.AgentID,
		Status: RunStatusPending, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	}
	if _, err := runtime.Store().db.ExecContext(ctx, `CREATE TRIGGER coverage_fail_goal_initial_save BEFORE INSERT ON `+tableRuns+` BEGIN SELECT RAISE(FAIL, 'goal initial save failed'); END`); err != nil {
		t.Fatalf("create run-save failure trigger: %v", err)
	}

	response, err := executor.runADKGoalWorkflow(ctx, workflowRequest{Session: session, RunOptions: RunOptions{LoopMaxIterations: 1}}, parent, nil)
	if err != nil {
		t.Fatalf("runADKGoalWorkflow returned transport error: %v", err)
	}
	if response.Run.Status != RunStatusFailed || !strings.Contains(response.Reply, "goal initial save failed") || !strings.Contains(response.Run.FailureReason, "goal initial save failed") {
		t.Fatalf("initial save failure response = %+v", response)
	}
}
