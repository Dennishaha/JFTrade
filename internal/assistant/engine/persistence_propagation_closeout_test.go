package adk

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/genai"
)

func TestGoalPausePersistenceErrorsPropagateAcrossDecisionBoundaries(t *testing.T) {
	t.Run("decision resolution", func(t *testing.T) {
		runtime, session, parent := newGoalPausePersistenceFixture(t, "resolve")
		_, _, _, _, _, _, err := runtime.workflowExecutor().resolveGoalWorkflowDecision(
			t.Context(), workflowRequest{Session: session}, parent, nil, newBareGoogleADKExecution(parent.ID),
			&workflowGoalDecision{}, openAIChatResult{}, "visible progress", "", 1, false,
		)
		assertGoalPausePersistenceError(t, err)
	})

	t.Run("missing decision fallback", func(t *testing.T) {
		runtime, session, parent := newGoalPausePersistenceFixture(t, "missing-decision")
		_, _, _, _, err := runtime.workflowExecutor().pauseAfterMissingGoalDecision(
			t.Context(), workflowRequest{Session: session}, parent, openAIChatResult{}, "visible progress", workflowGoalDecisionSnapshot{}, 1,
		)
		assertGoalPausePersistenceError(t, err)
	})

	t.Run("complete decision", func(t *testing.T) {
		runtime, session, parent := newGoalPausePersistenceFixture(t, "complete")
		_, _, _, _, err := runtime.workflowExecutor().finishCompleteGoalWorkflow(
			t.Context(), workflowRequest{Session: session}, parent, nil, openAIChatResult{},
			workflowGoalDecisionSnapshot{summary: "complete after persistence"}, "", 1,
		)
		assertGoalPausePersistenceError(t, err)
	})

	t.Run("continue decision", func(t *testing.T) {
		runtime, session, parent := newGoalPausePersistenceFixture(t, "continue")
		_, _, _, _, err := runtime.workflowExecutor().finishContinueGoalWorkflow(
			t.Context(), workflowRequest{Session: session}, parent, openAIChatResult{},
			workflowGoalDecisionSnapshot{reason: "continue after persistence"}, "", 1,
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
			CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
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
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_goal_terminal_child_projection")

		_, _, _, _, err := runtime.workflowExecutor().prepareGoalWorkflowTurn(
			t.Context(), workflowRequest{Session: session}, parent, nil, newBareGoogleADKExecution(parent.ID), nil, 1,
		)
		if err == nil || !strings.Contains(err.Error(), "persist terminal parent workflow state") {
			t.Fatalf("prepareGoalWorkflowTurn error = %v, want terminal parent persistence failure", err)
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
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_goal_bootstrap_terminal_write")

		response, err := runtime.workflowExecutor().continueADKGoalWorkflow(
			t.Context(), workflowRequest{Agent: agent, Session: session, Mode: WorkModeLoop}, parent, nil, "continue", 1, 1,
		)
		if err == nil || !strings.Contains(err.Error(), "reject_goal_bootstrap_terminal_write") || response.Run.ID != "" {
			t.Fatalf("continueADKGoalWorkflow response=%+v err=%v", response, err)
		}
	})

	t.Run("decision provider failure", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "goal-decision-write-agent", "goal decision write")
		parent := mustSaveRun(t, runtime, Run{
			ID: "goal-decision-write-parent", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_goal_decision_terminal_write")
		execution := newBareGoogleADKExecution(parent.ID)
		execution.runBlocking = func(context.Context, *genai.Content) error { return errors.New("decision provider unavailable") }

		_, _, _, _, _, _, err := runtime.workflowExecutor().runGoalWorkflowDecision(
			t.Context(), workflowRequest{Session: session}, parent, nil, execution, &workflowGoalDecision{}, parent, "visible progress", 1, false,
		)
		if err == nil || !strings.Contains(err.Error(), "reject_goal_decision_terminal_write") {
			t.Fatalf("runGoalWorkflowDecision error = %v, want terminal persistence failure", err)
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
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	step := workflowStep{Order: 1, DependencyID: "native-outage-step", Title: "Provider outage", Message: "Fetch provider response", WorkflowMode: WorkModeLoop}
	task, err := runtime.Store().SaveTask(t.Context(), TaskWriteRequest{
		ID: "native-unavailable-task", Title: step.Title, Message: step.Message, Status: "TODO",
		AgentID: agent.ID, RunID: parent.ID, Order: 1, WorkflowMode: WorkModeLoop,
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	parent.WorkflowPlan = workflowPlanFromTasks([]Task{task}, nil)
	mustSaveRun(t, runtime, parent)

	response, err := runtime.workflowExecutor().runNativeTaskGraphWorkflow(
		t.Context(), workflowRequest{Agent: agent, Session: session, Message: step.Message, Mode: WorkModeLoop},
		parent, []workflowStep{step}, []Task{task},
	)
	if err != nil || response.Run.Status != RunStatusFailed || !strings.Contains(response.Run.FailureReason, "503") {
		t.Fatalf("native provider outage response=%+v err=%v", response, err)
	}
}

func TestWorkflowTaskToolContractAndLifecycleReadFailures(t *testing.T) {
	toolset := &workflowTaskToolset{}
	if toolset.Name() != "jftrade-workflow-task-tools" {
		t.Fatalf("workflow task toolset name = %q", toolset.Name())
	}
	if _, err := toolset.modelsList(nil); err == nil || !strings.Contains(err.Error(), "runtime is unavailable") {
		t.Fatalf("modelsList nil runtime error = %v", err)
	}
	execution := newBareGoogleADKExecution("final-reply-run")
	if !goalTurnHasFinalReply(execution, "final-reply-run", "visible reply") {
		t.Fatal("a visible reply without tool calls should be final")
	}
	if plannerStringSliceArg(nil, "dependsOn") != nil || plannerStringSliceArg(map[string]any{"dependsOn": "invalid"}, "dependsOn") != nil {
		t.Fatal("plannerStringSliceArg accepted an absent or malformed dependency list")
	}

	readRuntime := newTestRuntime(t)
	if _, err := readRuntime.Store().db.ExecContext(t.Context(), `DROP TABLE `+tableRuns); err != nil {
		t.Fatalf("drop run table: %v", err)
	}
	if err := readRuntime.markApprovalContinuationFailed(t.Context(), "missing", errors.New("continuation failed")); err == nil {
		t.Fatal("markApprovalContinuationFailed hid a run lookup failure")
	}

	timeoutRuntime := newTestRuntime(t)
	old := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	run := mustSaveRun(t, timeoutRuntime, Run{
		ID: "cancel-reconcile-write-run", Status: RunStatusRunning, WorkMode: WorkModeChat,
		StartedAt: old, CreatedAt: old, UpdatedAt: old, MaxDurationMs: 1, Usage: &RunUsage{},
	})
	installRunUpdateRejectTrigger(t, timeoutRuntime, run.ID, "reject_cancel_reconcile_write")
	if _, err := timeoutRuntime.CancelRun(t.Context(), "another-run"); err == nil || !strings.Contains(err.Error(), "persist timed-out run "+run.ID) {
		t.Fatalf("CancelRun reconciliation error = %v", err)
	}
}

func TestWorkflowResumePersistenceFailuresRemainObservable(t *testing.T) {
	t.Run("already paused parent", func(t *testing.T) {
		runtime := newTestRuntime(t)
		parent := mustSaveRun(t, runtime, Run{
			ID: "resume-paused-write-parent", Status: RunStatusPaused, WorkMode: WorkModeLoop,
			WorkflowStatus: workflowStatusPaused, PausedReason: "user",
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_resume_paused_write")
		_, blocked, err := runtime.workflowExecutor().reconcileWorkflowChildren(t.Context(), parent)
		if err == nil || blocked || !strings.Contains(err.Error(), "reject_resume_paused_write") {
			t.Fatalf("reconcileWorkflowChildren = blocked:%v err:%v", blocked, err)
		}
	})

	t.Run("completion save failure becomes a durable parent failure", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "resume-complete-write-agent", Name: "Resume Complete Write", Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "resume complete write")
		parent := mustSaveRun(t, runtime, Run{
			ID: "resume-complete-write-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(t.Context(), `
			CREATE TRIGGER reject_resumed_completion_write
			BEFORE UPDATE ON `+tableRuns+`
			WHEN NEW.id = '`+parent.ID+`' AND NEW.status = '`+RunStatusCompleted+`'
			BEGIN SELECT RAISE(FAIL, 'resumed completion write rejected'); END
		`); err != nil {
			t.Fatalf("create completion trigger: %v", err)
		}

		updated, err := runtime.continueParentWorkflowAfterChild(t.Context(), Run{
			ID: "resume-complete-write-child", ParentRunID: parent.ID, Status: RunStatusCompleted, Message: "child complete",
		})
		if err != nil || updated == nil || updated.Status != RunStatusFailed || !strings.Contains(updated.FailureReason, "resumed completion write rejected") {
			t.Fatalf("continueParentWorkflowAfterChild = %+v err=%v", updated, err)
		}
	})

	t.Run("completion and failure writes both failing return the storage error", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "resume-double-write-agent", Name: "Resume Double Write", Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "resume double write")
		parent := mustSaveRun(t, runtime, Run{
			ID: "resume-double-write-parent", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().db.ExecContext(t.Context(), `
			CREATE TRIGGER reject_resumed_terminal_writes
			BEFORE UPDATE ON `+tableRuns+`
			WHEN NEW.id = '`+parent.ID+`' AND NEW.status IN ('`+RunStatusCompleted+`', '`+RunStatusFailed+`')
			BEGIN SELECT RAISE(FAIL, 'resumed terminal write rejected'); END
		`); err != nil {
			t.Fatalf("create terminal trigger: %v", err)
		}

		updated, err := runtime.continueParentWorkflowAfterChild(t.Context(), Run{
			ID: "resume-double-write-child", ParentRunID: parent.ID, Status: RunStatusCompleted, Message: "child complete",
		})
		if err == nil || updated != nil || !strings.Contains(err.Error(), "resumed terminal write rejected") {
			t.Fatalf("continueParentWorkflowAfterChild = %+v err=%v", updated, err)
		}
	})
}

func TestWorkflowBlockerAndRuntimeInitializationFailureSemantics(t *testing.T) {
	runtime := newTestRuntime(t)
	parent := Run{ID: "missing-child-blocker-parent", ChildRunIDs: []string{"orphan-child"}}
	toolset := &workflowTaskToolset{executor: runtime.workflowExecutor()}
	blockers := toolset.workflowCompletionBlockers(t.Context(), parent, []Task{{
		ID: "missing-child-blocker-task", Status: "DONE", Executor: workflowTaskExecutorChild, RunID: "missing-child",
	}})
	if len(blockers) != 2 || blockers[0]["status"] != "MISSING" || blockers[1]["status"] != "MISSING" {
		t.Fatalf("workflow completion blockers = %+v", blockers)
	}

	originalBuiltinSpecs := builtinSkillSpecs
	builtinSkillSpecs = []builtinSkillSpec{{Name: "startup-failure", BuildBundle: func() (map[string]string, error) {
		return nil, errors.New("builtin skill initialization rejected")
	}}}
	t.Cleanup(func() { builtinSkillSpecs = originalBuiltinSpecs })
	root := t.TempDir()
	store, err := NewStore(
		filepath.Join(root, "adk.db"), filepath.Join(root, "secrets", "adk.json"), filepath.Join(root, "skills"),
	)
	if err == nil || store != nil || !strings.Contains(err.Error(), "builtin skill initialization rejected") {
		t.Fatalf("NewStore = store:%v err:%v", store, err)
	}
}

func newGoalPausePersistenceFixture(t *testing.T, suffix string) (*Runtime, Session, Run) {
	t.Helper()
	runtime := newTestRuntime(t)
	session := mustCreateSession(t, runtime, "pause-boundary-agent-"+suffix, "pause boundary "+suffix)
	pauseRequestedAt := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "pause-boundary-parent-" + suffix, SessionID: session.ID, AgentID: session.AgentID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		PauseRequestedAt: &pauseRequestedAt, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
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
