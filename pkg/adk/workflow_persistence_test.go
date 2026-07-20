package adk

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGoalWorkflowFailsWhenInitialStateCannotBePersisted(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-persist-initial-agent", Name: "Goal Persistence", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "persist initial goal state")
	if _, err := runtime.store.db.ExecContext(t.Context(), `
		CREATE TRIGGER fail_initial_goal_state
		BEFORE UPDATE ON `+tableRuns+`
		BEGIN SELECT RAISE(FAIL, 'initial goal state unavailable'); END
	`); err != nil {
		t.Fatalf("create initial-state trigger: %v", err)
	}

	_, err := runtime.workflowExecutor().Run(t.Context(), workflowRequest{
		Agent: agent, Session: session, Message: "advance goal", Mode: WorkModeLoop, Objective: "advance goal",
	})
	if err == nil || !strings.Contains(err.Error(), "persist initial goal workflow state") {
		t.Fatalf("Run error = %v, want explicit initial persistence failure", err)
	}
}

func TestCompletedWorkflowFailsWhenTerminalStateCannotBePersisted(t *testing.T) {
	runtime := newTestRuntime(t)
	runtime.rawSessionService = createErrorSessionService{err: errors.New("assistant message unavailable")}
	session := mustCreateSession(t, runtime, "workflow-terminal-persist-agent", "persist terminal workflow")
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-terminal-persist-run", SessionID: session.ID, AgentID: session.AgentID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	task, err := runtime.Store().SaveTask(t.Context(), TaskWriteRequest{
		ID: "workflow-terminal-persist-task", Title: "done", Status: "DONE", RunID: parent.ID,
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	if _, err := runtime.store.db.ExecContext(t.Context(), `
		CREATE TRIGGER fail_completed_workflow_state
		BEFORE UPDATE ON `+tableRuns+`
		WHEN NEW.id = 'workflow-terminal-persist-run'
		BEGIN SELECT RAISE(FAIL, 'terminal workflow state unavailable'); END
	`); err != nil {
		t.Fatalf("create terminal-state trigger: %v", err)
	}

	response, err := runtime.workflowExecutor().finalizePlannedWorkflow(t.Context(), workflowRequest{Session: session}, parent, []Task{task}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "persist completed workflow state") {
		t.Fatalf("finalizePlannedWorkflow error = %v, want terminal persistence failure", err)
	}
	if response.Run.ID != "" {
		t.Fatalf("response = %+v, want no successful terminal response", response)
	}
}

func TestUserPauseFailsWhenPausedStateCannotBePersisted(t *testing.T) {
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "goal-pause-persist-agent", "persist user pause")
	pauseRequestedAt := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "goal-pause-persist-run", SessionID: session.ID, AgentID: session.AgentID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		PauseRequestedAt: &pauseRequestedAt, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.store.db.ExecContext(t.Context(), `
		CREATE TRIGGER fail_user_pause_state
		BEFORE UPDATE ON `+tableRuns+`
		WHEN NEW.id = 'goal-pause-persist-run'
		BEGIN SELECT RAISE(FAIL, 'user pause state unavailable'); END
	`); err != nil {
		t.Fatalf("create user-pause trigger: %v", err)
	}

	_, _, paused, err := runtime.workflowExecutor().pauseADKGoalWorkflowIfRequested(
		t.Context(), workflowRequest{Session: session}, parent, 1, "pause",
	)
	if err == nil || !strings.Contains(err.Error(), "persist user-paused goal state") {
		t.Fatalf("pause error = %v, want explicit persistence failure", err)
	}
	if paused {
		t.Fatal("pause reported success without durable state")
	}
}

func TestExpiredRunReconciliationReturnsTerminalPersistenceFailure(t *testing.T) {
	runtime := newTestRuntime(t)
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	run := mustSaveRun(t, runtime, Run{
		ID: "expired-persist-failure", Status: RunStatusRunning, WorkMode: WorkModeChat,
		StartedAt: old, CreatedAt: old, UpdatedAt: old, MaxDurationMs: 1, Usage: &RunUsage{},
	})
	if _, err := runtime.store.db.ExecContext(t.Context(), `
		CREATE TRIGGER fail_expired_terminal_state
		BEFORE UPDATE ON `+tableRuns+`
		WHEN NEW.id = 'expired-persist-failure'
		BEGIN SELECT RAISE(FAIL, 'timeout state unavailable'); END
	`); err != nil {
		t.Fatalf("create timeout-state trigger: %v", err)
	}

	err := runtime.ReconcileExpiredRuns(t.Context())
	if err == nil || !strings.Contains(err.Error(), "persist timed-out run "+run.ID) {
		t.Fatalf("ReconcileExpiredRuns error = %v, want terminal persistence failure", err)
	}
	stored, ok, loadErr := runtime.Store().Run(t.Context(), run.ID)
	if loadErr != nil || !ok || stored.Status != RunStatusRunning {
		t.Fatalf("stored run = %+v ok=%v err=%v, want unchanged running state", stored, ok, loadErr)
	}
}

func TestGoalWorkflowFailsWhenIterationLimitPauseCannotBePersisted(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-persist-limit-agent", Name: "Goal Persistence", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "persist iteration pause")
	parent := mustSaveRun(t, runtime, Run{
		ID: "goal-persist-limit-run", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.store.db.ExecContext(t.Context(), `
		CREATE TRIGGER fail_iteration_limit_pause
		BEFORE UPDATE ON `+tableRuns+`
		WHEN NEW.id = 'goal-persist-limit-run'
		BEGIN SELECT RAISE(FAIL, 'iteration pause unavailable'); END
	`); err != nil {
		t.Fatalf("create iteration-limit trigger: %v", err)
	}

	response, err := runtime.workflowExecutor().continueADKGoalWorkflow(t.Context(), workflowRequest{
		Agent: agent, Session: session, Mode: WorkModeLoop,
	}, parent, nil, "continue", 1, 0)
	if err == nil || !strings.Contains(err.Error(), "persist goal iteration-limit pause") {
		t.Fatalf("continueADKGoalWorkflow error = %v, want explicit pause persistence failure", err)
	}
	if response.Run.ID != "" {
		t.Fatalf("response = %+v, want no successful run response", response)
	}
}
