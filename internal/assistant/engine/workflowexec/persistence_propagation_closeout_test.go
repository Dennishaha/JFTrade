package workflowexec

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestGoalPausePersistenceErrorsPropagateAcrossDecisionBoundaries(t *testing.T) {
	t.Run("decision resolution", func(t *testing.T) {
		runtime, session, parent := newGoalPausePersistenceFixture(t, "resolve")
		_, _, _, _, _, _, err := (&WorkflowExecutor{runtime: runtime}).ResolveGoalWorkflowDecision(
			t.Context(), workflowRequest{Session: session}, parent, nil, &fakeWorkflowExecutionHandle{},
			&workflowGoalDecision{}, jfadk.AssistantExecutionResult{}, "visible progress", "", 1, false,
		)
		assertGoalPausePersistenceError(t, err)
	})

	t.Run("missing decision fallback", func(t *testing.T) {
		runtime, session, parent := newGoalPausePersistenceFixture(t, "missing-decision")
		_, _, _, _, err := (&WorkflowExecutor{runtime: runtime}).PauseAfterMissingGoalDecision(
			t.Context(), workflowRequest{Session: session}, parent, jfadk.AssistantExecutionResult{}, "visible progress", workflowGoalDecisionSnapshot{}, 1,
		)
		assertGoalPausePersistenceError(t, err)
	})

	t.Run("complete decision", func(t *testing.T) {
		runtime, session, parent := newGoalPausePersistenceFixture(t, "complete")
		_, _, _, _, err := (&WorkflowExecutor{runtime: runtime}).FinishCompleteGoalWorkflow(
			t.Context(), workflowRequest{Session: session}, parent, nil, jfadk.AssistantExecutionResult{},
			workflowGoalDecisionSnapshot{Summary: "complete after persistence"}, "", 1,
		)
		assertGoalPausePersistenceError(t, err)
	})

	t.Run("continue decision", func(t *testing.T) {
		runtime, session, parent := newGoalPausePersistenceFixture(t, "continue")
		_, _, _, _, err := (&WorkflowExecutor{runtime: runtime}).FinishContinueGoalWorkflow(
			t.Context(), workflowRequest{Session: session}, parent, jfadk.AssistantExecutionResult{},
			workflowGoalDecisionSnapshot{Reason: "continue after persistence"}, "", 1,
		)
		assertGoalPausePersistenceError(t, err)
	})
}

func TestGoalDecisionAndChildTerminationWritesFailClosed(t *testing.T) {
	t.Run("terminal child projection", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "goal-terminal-child-agent", "goal terminal child write")
		parent := mustSaveRun(t, runtime, Run{
			ID: "goal-terminal-child-parent", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "goal-terminal-child-task", ChildRunID: "goal-terminal-child", Status: "IN_PROGRESS"}},
			CreatedAt:    jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().SaveTask(t.Context(), TaskWriteRequest{
			ID: "goal-terminal-child-task", Title: "Terminal child", Status: "IN_PROGRESS", RunID: parent.ID,
			Executor: workflowTaskExecutorChild,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		mustSaveRun(t, runtime, Run{
			ID: "goal-terminal-child", ParentRunID: parent.ID, SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusFailed, FailureReason: "child execution failed", ErrorCode: "CHILD_FAILED",
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_goal_terminal_child_projection")

		_, _, _, _, err := (&WorkflowExecutor{runtime: runtime}).PrepareGoalWorkflowTurn(
			t.Context(), workflowRequest{Session: session}, parent, nil, &fakeWorkflowExecutionHandle{}, nil, 1,
		)
		if err == nil || !strings.Contains(err.Error(), "persist terminal parent workflow state") {
			t.Fatalf("PrepareGoalWorkflowTurn error = %v, want terminal parent persistence failure", err)
		}
	})

	t.Run("model bootstrap failure", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "goal-bootstrap-write-agent", Name: "Goal Bootstrap Write", ProviderID: "missing-goal-provider",
			Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "goal bootstrap write")
		parent := mustSaveRun(t, runtime, Run{
			ID: "goal-bootstrap-write-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_goal_bootstrap_terminal_write")

		response, err := (&WorkflowExecutor{runtime: runtime}).ContinueADKGoalWorkflow(
			t.Context(), workflowRequest{Agent: agent, Session: session, Mode: WorkModeLoop}, parent, nil, "continue", 1, 1,
		)
		if err == nil || !strings.Contains(err.Error(), "reject_goal_bootstrap_terminal_write") || response.Run.ID != "" {
			t.Fatalf("ContinueADKGoalWorkflow response=%+v err=%v", response, err)
		}
	})

	t.Run("decision provider failure", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "goal-decision-write-agent", "goal decision write")
		parent := mustSaveRun(t, runtime, Run{
			ID: "goal-decision-write-parent", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_goal_decision_terminal_write")
		execution := &fakeWorkflowExecutionHandle{runErr: errors.New("decision provider unavailable")}

		_, _, _, _, _, _, err := (&WorkflowExecutor{runtime: runtime}).RunGoalWorkflowDecision(
			t.Context(), workflowRequest{Session: session}, parent, nil, execution, &workflowGoalDecision{}, parent, "visible progress", 1, false,
		)
		if err == nil || !strings.Contains(err.Error(), "reject_goal_decision_terminal_write") {
			t.Fatalf("RunGoalWorkflowDecision error = %v, want terminal persistence failure", err)
		}
	})
}

func TestNativeTaskGraphProviderFailurePersistsTheParent(t *testing.T) {
	runtime := newTestRuntime(t)
	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "provider unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(unavailable.Close)
	providerID := "native-unavailable-provider"
	mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: providerID, DisplayName: "Native Unavailable", BaseURL: unavailable.URL,
		Model: "test-model", APIKey: "sk-test", Enabled: true,
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "native-unavailable-agent", Name: "Native Unavailable", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "native provider outage")
	parent := mustSaveRun(t, runtime, Run{
		ID: "native-unavailable-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	step := workflowStep{Order: 1, DependencyID: "native-outage-step", Title: "Provider outage", Message: "Fetch provider response", WorkflowMode: WorkModeLoop}
	task, err := runtime.Store().SaveTask(t.Context(), TaskWriteRequest{
		ID: "native-unavailable-task", Title: step.Title, Message: step.Message, Status: "TODO",
		AgentID: agent.ID, RunID: parent.ID, Order: 1, WorkflowMode: WorkModeLoop,
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	parent.WorkflowPlan = jfadkmodel.WorkflowPlanFromTasks([]Task{task}, nil)
	mustSaveRun(t, runtime, parent)

	response, err := (&WorkflowExecutor{runtime: runtime}).RunNativeTaskGraphWorkflow(
		t.Context(), workflowRequest{Agent: agent, Session: session, Message: step.Message, Mode: WorkModeLoop},
		parent, []workflowStep{step}, []Task{task},
	)
	if err != nil || response.Run.Status != RunStatusFailed || !strings.Contains(response.Run.FailureReason, "503") {
		t.Fatalf("native provider outage response=%+v err=%v", response, err)
	}
}

func TestWorkflowTaskToolsetNameAndModelsListContract(t *testing.T) {
	toolset := NewWorkflowTaskToolset(nil, "", "")
	if toolset.Name() != "jftrade-workflow-task-tools" {
		t.Fatalf("workflow task toolset name = %q", toolset.Name())
	}
	if _, err := toolset.ModelsList(nil); err == nil || !strings.Contains(err.Error(), "runtime is unavailable") {
		t.Fatalf("modelsList nil runtime error = %v", err)
	}
	execution := &fakeWorkflowExecutionHandle{reply: "visible reply", postTool: true}
	if !execution.HasFinalReplyForRun("final-reply-run", "visible reply") {
		t.Fatal("a visible reply without tool calls should be final")
	}
}

func TestWorkflowResumePausedPersistenceFailureRemainsObservable(t *testing.T) {
	runtime := newTestRuntime(t)
	parent := mustSaveRun(t, runtime, Run{
		ID: "resume-paused-write-parent", Status: RunStatusPaused, WorkMode: WorkModeLoop,
		WorkflowStatus: workflowStatusPaused, PausedReason: "user",
		CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_resume_paused_write")
	_, blocked, err := (&WorkflowExecutor{runtime: runtime}).ReconcileWorkflowChildren(t.Context(), parent)
	if err == nil || blocked || !strings.Contains(err.Error(), "reject_resume_paused_write") {
		t.Fatalf("reconcileWorkflowChildren = blocked:%v err:%v", blocked, err)
	}
}

func TestWorkflowCompletionBlockerMissingChildSemantics(t *testing.T) {
	runtime := newTestRuntime(t)
	parent := Run{ID: "missing-child-blocker-parent", ChildRunIDs: []string{"orphan-child"}}
	toolset := NewWorkflowTaskToolset(&WorkflowExecutor{runtime: runtime}, "", "")
	blockers, err := toolset.WorkflowCompletionBlockers(t.Context(), parent, []Task{{
		ID: "missing-child-blocker-task", Status: "DONE", Executor: workflowTaskExecutorChild, RunID: "missing-child",
	}})
	if err != nil {
		t.Fatalf("workflowCompletionBlockers: %v", err)
	}
	if len(blockers) != 2 || blockers[0]["status"] != "MISSING" || blockers[1]["status"] != "MISSING" {
		t.Fatalf("workflow completion blockers = %+v", blockers)
	}
}

func newGoalPausePersistenceFixture(t *testing.T, suffix string) (*Runtime, Session, Run) {
	t.Helper()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "pause-boundary-agent-"+suffix, "pause boundary "+suffix)
	pauseRequestedAt := jfadkmodel.NowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "pause-boundary-parent-" + suffix, SessionID: session.ID, AgentID: session.AgentID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		PauseRequestedAt: &pauseRequestedAt, CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
	})
	installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_pause_boundary_"+strings.ReplaceAll(suffix, "-", "_"))
	return runtime, session, parent
}

func assertGoalPausePersistenceError(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "persist user-paused goal state") {
		t.Fatalf("goal pause error = %v, want durable pause persistence failure", err)
	}
}
