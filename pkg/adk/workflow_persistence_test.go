package adk

import (
	"strings"
	"testing"
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
