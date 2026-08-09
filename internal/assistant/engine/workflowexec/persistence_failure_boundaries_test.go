package workflowexec

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	jfadk "github.com/jftrade/jftrade-main/internal/assistant/engine"
	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

func TestGoalTurnPersistenceFailuresBubbleThroughTheOrchestrator(t *testing.T) {
	t.Run("model failure cannot hide a failed terminal write", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "goal-model-write-agent", "goal model write failure")
		parent := mustSaveRun(t, runtime, Run{
			ID: "goal-model-write-parent", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_goal_model_terminal_write")

		updated, response, done, prompt, err := (&WorkflowExecutor{runtime: runtime}).FinishADKGoalWorkflowTurn(
			t.Context(), workflowRequest{Session: session}, parent, nil, &fakeWorkflowExecutionHandle{},
			&workflowGoalDecision{}, errors.New("goal model unavailable"), 1, false,
		)
		if err == nil || !strings.Contains(err.Error(), "reject_goal_model_terminal_write") {
			t.Fatalf("FinishADKGoalWorkflowTurn error = %v, want terminal persistence failure", err)
		}
		if updated.ID != "" || response.Run.ID != "" || done || prompt != "" {
			t.Fatalf("failed goal turn leaked success: run=%+v response=%+v done=%v prompt=%q", updated, response, done, prompt)
		}
	})

	t.Run("running state write failure stops decision processing", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "goal-running-write-agent", "goal running write failure")
		parent := mustSaveRun(t, runtime, Run{
			ID: "goal-running-write-parent", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_goal_running_write")

		_, _, _, _, err := (&WorkflowExecutor{runtime: runtime}).FinishADKGoalWorkflowTurn(
			t.Context(), workflowRequest{Session: session}, parent, nil, &fakeWorkflowExecutionHandle{},
			&workflowGoalDecision{}, nil, 1, false,
		)
		if err == nil || !strings.Contains(err.Error(), "persist running goal state") || !strings.Contains(err.Error(), "reject_goal_running_write") {
			t.Fatalf("FinishADKGoalWorkflowTurn error = %v, want running-state persistence failure", err)
		}
	})

	t.Run("completed state fallback write failure remains observable", func(t *testing.T) {
		dir := t.TempDir()
		sessionService, err := enginepersistence.NewSQLiteSessionService(filepath.Join(dir, "adk-session.db"))
		if err != nil {
			t.Fatalf("NewSQLiteSessionService: %v", err)
		}
		if err := enginepersistence.ValidateSQLiteSessionService(sessionService); err != nil {
			t.Fatalf("ValidateSQLiteSessionService: %v", err)
		}
		service := &failAfterSessionService{Service: sessionService}
		runtime := newTestRuntimeWithSessionService(t, service)
		session := mustCreateSession(t, runtime, "goal-complete-write-agent", "goal complete write failure")
		parent := mustSaveRun(t, runtime, Run{
			ID: "goal-complete-write-parent", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_goal_complete_write")
		service.fail = true

		_, response, done, prompt, err := (&WorkflowExecutor{runtime: runtime}).FinishCompleteGoalWorkflow(
			t.Context(), workflowRequest{Session: session}, parent, nil, jfadk.AssistantExecutionResult{},
			workflowGoalDecisionSnapshot{Summary: "durable result"}, "", 1,
		)
		if err == nil || !strings.Contains(err.Error(), "persist completed goal state") || !strings.Contains(err.Error(), "reject_goal_complete_write") {
			t.Fatalf("FinishCompleteGoalWorkflow error = %v, want completed-state persistence failure", err)
		}
		if response.Run.ID != "" || done || prompt != "" {
			t.Fatalf("completed goal failure leaked success: response=%+v done=%v prompt=%q", response, done, prompt)
		}
	})

	t.Run("continued state write failure prevents another model turn", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "goal-continue-write-agent", "goal continue write failure")
		parent := mustSaveRun(t, runtime, Run{
			ID: "goal-continue-write-parent", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_goal_continue_write")

		_, response, done, prompt, err := (&WorkflowExecutor{runtime: runtime}).FinishContinueGoalWorkflow(
			t.Context(), workflowRequest{Session: session}, parent, jfadk.AssistantExecutionResult{},
			workflowGoalDecisionSnapshot{Reason: "continue after review"}, "", 1,
		)
		if err == nil || !strings.Contains(err.Error(), "persist continued goal state") || !strings.Contains(err.Error(), "reject_goal_continue_write") {
			t.Fatalf("FinishContinueGoalWorkflow error = %v, want continued-state persistence failure", err)
		}
		if response.Run.ID != "" || done || prompt != "" {
			t.Fatalf("continued goal failure leaked success: response=%+v done=%v prompt=%q", response, done, prompt)
		}
	})
}

func TestWorkflowTerminalProjectionPersistenceFailuresDoNotDisappear(t *testing.T) {
	t.Run("blocked child cannot produce an unpersisted parent response", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "blocked-parent-write-agent", "blocked parent write failure")
		parent := mustSaveRun(t, runtime, Run{
			ID: "blocked-parent-write-run", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "blocked-parent-write-task", Status: "IN_PROGRESS"}},
			CreatedAt:    jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_blocked_parent_write")
		child := Run{
			ID: "blocked-parent-write-child", ParentRunID: parent.ID, Status: RunStatusFailed,
			Message: "child failed", FailureReason: "child failed", ErrorCode: "CHILD_FAILED", Iteration: 1,
		}

		response, err := (&WorkflowExecutor{runtime: runtime}).FinalizePlannedWorkflow(
			t.Context(), workflowRequest{Session: session}, parent, nil, []ChatResponse{{Run: child}}, nil,
		)
		if err == nil || !strings.Contains(err.Error(), "persist blocked workflow state") || !strings.Contains(err.Error(), "reject_blocked_parent_write") {
			t.Fatalf("FinalizePlannedWorkflow error = %v, want blocked-parent persistence failure", err)
		}
		if response.Run.ID != "" {
			t.Fatalf("blocked workflow response = %+v, want no successful response", response)
		}
	})

	t.Run("scheduler failure itself must be persisted", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "scheduler-failure-write-agent", "scheduler failure write")
		parent := mustSaveRun(t, runtime, Run{
			ID: "scheduler-failure-write-run", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		task, err := runtime.Store().SaveTask(t.Context(), TaskWriteRequest{
			ID: "scheduler-failure-write-task", Title: "Still pending", Status: "TODO", RunID: parent.ID,
		})
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_scheduler_failure_write")

		response, err := (&WorkflowExecutor{runtime: runtime}).FinalizePlannedWorkflow(
			t.Context(), workflowRequest{Session: session}, parent, []Task{task}, nil, nil,
		)
		if err == nil || !strings.Contains(err.Error(), "persist failed workflow state") {
			t.Fatalf("FinalizePlannedWorkflow error = %v, want scheduler failure persistence error", err)
		}
		if response.Run.ID != "" {
			t.Fatalf("scheduler failure response = %+v, want no successful response", response)
		}
	})

	t.Run("scheduler error code write cannot be silently dropped", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "scheduler-code-write-agent", "scheduler code write")
		parent := mustSaveRun(t, runtime, Run{
			ID: "scheduler-code-write-run", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		task, err := runtime.Store().SaveTask(t.Context(), TaskWriteRequest{
			ID: "scheduler-code-write-task", Title: "Still pending", Status: "TODO", RunID: parent.ID,
		})
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `
			CREATE TRIGGER reject_scheduler_error_code_write
			BEFORE UPDATE ON `+enginepersistence.TableRuns+`
			WHEN NEW.id = '`+parent.ID+`' AND json_extract(NEW.payload_json, '$.errorCode') = '`+workflowTaskIncompleteErr+`'
			BEGIN SELECT RAISE(FAIL, 'scheduler error code write rejected'); END
		`); err != nil {
			t.Fatalf("create scheduler-code trigger: %v", err)
		}

		response, err := (&WorkflowExecutor{runtime: runtime}).FinalizePlannedWorkflow(
			t.Context(), workflowRequest{Session: session}, parent, []Task{task}, nil, nil,
		)
		if err == nil || !strings.Contains(err.Error(), "persist incomplete workflow state") || !strings.Contains(err.Error(), "scheduler error code write rejected") {
			t.Fatalf("FinalizePlannedWorkflow error = %v, want scheduler error-code persistence failure", err)
		}
		if response.Run.ID != "" {
			t.Fatalf("scheduler code response = %+v, want no successful response", response)
		}
	})

	t.Run("missing child final reply cannot hide its failed terminal write", func(t *testing.T) {
		runtime := newTestRuntime(t)
		child := mustSaveRun(t, runtime, Run{
			ID: "missing-final-write-child", SessionID: "missing-final-session", AgentID: "missing-final-agent",
			Status: RunStatusRunning, ParentRunID: "missing-final-parent",
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, child.ID, "reject_missing_final_child_write")

		err := (&WorkflowExecutor{runtime: runtime}).FailWorkflowChildAfterMissingFinal(
			t.Context(), child, &fakeWorkflowExecutionHandle{}, errors.New("final response missing"),
		)
		if err == nil || !strings.Contains(err.Error(), "persist failed workflow child state") || !strings.Contains(err.Error(), "reject_missing_final_child_write") {
			t.Fatalf("FailWorkflowChildAfterMissingFinal error = %v, want terminal persistence failure", err)
		}
	})
}

func TestGoalProjectionPersistenceFailuresStopAtTheirBoundary(t *testing.T) {
	t.Run("pause cleanup failure is returned", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "paused-cleanup-write-agent", "paused cleanup write")
		pauseRequestedAt := jfadkmodel.NowString()
		pauseError := jfadkmodel.ErrUserGoalPauseRequested.Error()
		parent := mustSaveRun(t, runtime, Run{
			ID: "paused-cleanup-write-run", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusPaused, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusPaused,
			PausedReason: "user", PauseRequestedAt: &pauseRequestedAt,
			ToolCalls: []ToolCall{{ID: "interrupted-task-call", ToolName: workflowTaskClaimTool, Status: "FAILED", Error: &pauseError}},
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_paused_cleanup_write")

		_, _, paused, err := (&WorkflowExecutor{runtime: runtime}).PauseADKGoalWorkflowIfRequested(
			t.Context(), workflowRequest{Session: session}, parent, 1, "paused",
		)
		if err == nil || paused || !strings.Contains(err.Error(), "persist cleaned paused goal state") {
			t.Fatalf("PauseADKGoalWorkflowIfRequested = paused:%v err:%v", paused, err)
		}
	})

	t.Run("pause failure bubbles through final turn resolution", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "turn-pause-write-agent", "turn pause write")
		pauseRequestedAt := jfadkmodel.NowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "turn-pause-write-run", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			PauseRequestedAt: &pauseRequestedAt, CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_turn_pause_write")

		_, _, _, _, err := (&WorkflowExecutor{runtime: runtime}).FinishADKGoalWorkflowTurn(
			t.Context(), workflowRequest{Session: session}, parent, nil, &fakeWorkflowExecutionHandle{},
			&workflowGoalDecision{}, jfadkmodel.ErrUserGoalPauseRequested, 1, false,
		)
		if err == nil || !strings.Contains(err.Error(), "persist user-paused goal state") {
			t.Fatalf("FinishADKGoalWorkflowTurn error = %v, want pause persistence failure", err)
		}
	})

	t.Run("active child pause write failure is returned", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "child-pause-write-agent", "child pause write")
		child := mustSaveRun(t, runtime, Run{
			ID: "child-pause-write-child", SessionID: session.ID, AgentID: session.AgentID,
			ParentRunID: "child-pause-write-parent", Status: RunStatusRunning,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		parent := mustSaveRun(t, runtime, Run{
			ID: child.ParentRunID, SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "child-pause-write-task", ChildRunID: child.ID, Status: "IN_PROGRESS"}},
			CreatedAt:    jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().SaveTask(t.Context(), TaskWriteRequest{
			ID: "child-pause-write-task", Title: "Active child", Status: "IN_PROGRESS", RunID: parent.ID,
			Executor: workflowTaskExecutorChild,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_child_pause_write")

		_, _, _, _, err := (&WorkflowExecutor{runtime: runtime}).PrepareGoalWorkflowTurn(
			t.Context(), workflowRequest{Session: session}, parent, nil, &fakeWorkflowExecutionHandle{}, nil, 1,
		)
		if err == nil || !strings.Contains(err.Error(), "persist goal blocked by child") {
			t.Fatalf("PrepareGoalWorkflowTurn error = %v, want child-pause persistence failure", err)
		}
	})

	t.Run("blocked task terminal write failure is returned", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "blocked-task-write-agent", "blocked task write")
		parent := mustSaveRun(t, runtime, Run{
			ID: "blocked-task-write-parent", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: jfadkmodel.NowString(), UpdatedAt: jfadkmodel.NowString(), Usage: &RunUsage{},
		})
		task, err := runtime.Store().SaveTask(t.Context(), TaskWriteRequest{
			ID: "blocked-task-write-task", Title: "Blocked task", Status: "BLOCKED", RunID: parent.ID, ResultSummary: "dependency failed",
		})
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		installRunUpdateRejectTrigger(t, runtime, parent.ID, "reject_blocked_task_write")

		_, _, _, _, err = (&WorkflowExecutor{runtime: runtime}).PrepareGoalWorkflowTurn(
			t.Context(), workflowRequest{Session: session}, parent, []Task{task}, &fakeWorkflowExecutionHandle{}, nil, 1,
		)
		if err == nil || !strings.Contains(err.Error(), "persist blocked goal state") {
			t.Fatalf("PrepareGoalWorkflowTurn error = %v, want blocked-task persistence failure", err)
		}
	})
}
