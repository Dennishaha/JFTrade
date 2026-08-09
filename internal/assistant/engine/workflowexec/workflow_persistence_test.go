package workflowexec

import (
	"errors"
	"strings"
	"testing"

	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestGoalWorkflowFailsWhenInitialStateCannotBePersisted(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-persist-initial-agent", Name: "Goal Persistence", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "persist initial goal state")
	if _, err := runtime.Store().DB().ExecContext(t.Context(), `
		CREATE TRIGGER fail_initial_goal_state
		BEFORE UPDATE ON `+enginepersistence.TableRuns+`
		BEGIN SELECT RAISE(FAIL, 'initial goal state unavailable'); END
	`); err != nil {
		t.Fatalf("create initial-state trigger: %v", err)
	}

	_, err := (&WorkflowExecutor{runtime: runtime}).Run(t.Context(), workflowRequest{
		Agent: agent, Session: session, Message: "advance goal", Mode: WorkModeLoop, Objective: "advance goal",
	})
	if err == nil || !strings.Contains(err.Error(), "persist initial goal workflow state") {
		t.Fatalf("Run error = %v, want explicit initial persistence failure", err)
	}
}

func TestCompletedWorkflowFailsWhenTerminalStateCannotBePersisted(t *testing.T) {
	runtime := newTestRuntimeWithSessionService(t, failGetSessionService{err: errors.New("assistant message unavailable")})
	session := mustCreateSession(t, runtime, "workflow-terminal-persist-agent", "persist terminal workflow")
	parent := mustSaveRun(t, runtime, Run{
		ID: "workflow-terminal-persist-run", SessionID: session.ID, AgentID: session.AgentID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	task, err := runtime.Store().SaveTask(t.Context(), TaskWriteRequest{
		ID: "workflow-terminal-persist-task", Title: "done", Status: "DONE", RunID: parent.ID,
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	if _, err := runtime.Store().DB().ExecContext(t.Context(), `
		CREATE TRIGGER fail_completed_workflow_state
		BEFORE UPDATE ON `+enginepersistence.TableRuns+`
		WHEN NEW.id = 'workflow-terminal-persist-run'
		BEGIN SELECT RAISE(FAIL, 'terminal workflow state unavailable'); END
	`); err != nil {
		t.Fatalf("create terminal-state trigger: %v", err)
	}

	response, err := (&WorkflowExecutor{runtime: runtime}).FinalizePlannedWorkflow(t.Context(), workflowRequest{Session: session}, parent, []Task{task}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "persist completed workflow state") {
		t.Fatalf("FinalizePlannedWorkflow error = %v, want terminal persistence failure", err)
	}
	if response.Run.ID != "" {
		t.Fatalf("response = %+v, want no successful terminal response", response)
	}
}

func TestUserPauseFailsWhenPausedStateCannotBePersisted(t *testing.T) {
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "goal-pause-persist-agent", "persist user pause")
	pauseRequestedAt := jfadkmodel.NowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "goal-pause-persist-run", SessionID: session.ID, AgentID: session.AgentID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		PauseRequestedAt: &pauseRequestedAt, CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.Store().DB().ExecContext(t.Context(), `
		CREATE TRIGGER fail_user_pause_state
		BEFORE UPDATE ON `+enginepersistence.TableRuns+`
		WHEN NEW.id = 'goal-pause-persist-run'
		BEGIN SELECT RAISE(FAIL, 'user pause state unavailable'); END
	`); err != nil {
		t.Fatalf("create user-pause trigger: %v", err)
	}

	_, _, paused, err := (&WorkflowExecutor{runtime: runtime}).PauseADKGoalWorkflowIfRequested(
		t.Context(), workflowRequest{Session: session}, parent, 1, "pause",
	)
	if err == nil || !strings.Contains(err.Error(), "persist user-paused goal state") {
		t.Fatalf("pause error = %v, want explicit persistence failure", err)
	}
	if paused {
		t.Fatal("pause reported success without durable state")
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
		ID: "goal-persist-limit-run", SessionID: session.ID, AgentID: session.AgentID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.Store().DB().ExecContext(t.Context(), `
		CREATE TRIGGER fail_iteration_limit_pause
		BEFORE UPDATE ON `+enginepersistence.TableRuns+`
		WHEN NEW.id = 'goal-persist-limit-run'
		BEGIN SELECT RAISE(FAIL, 'iteration pause unavailable'); END
	`); err != nil {
		t.Fatalf("create iteration-limit trigger: %v", err)
	}

	response, err := (&WorkflowExecutor{runtime: runtime}).ContinueADKGoalWorkflow(t.Context(), workflowRequest{
		Agent: agent, Session: session, Mode: WorkModeLoop,
	}, parent, nil, "continue", 1, 0)
	if err == nil || !strings.Contains(err.Error(), "persist goal iteration-limit pause") {
		t.Fatalf("ContinueADKGoalWorkflow error = %v, want explicit pause persistence failure", err)
	}
	if response.Run.ID != "" {
		t.Fatalf("response = %+v, want no successful run response", response)
	}
}
