package adk

import (
	"errors"
	assistantmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"strings"
	"testing"

	workflowexec "github.com/jftrade/jftrade-main/internal/assistant/engine/workflowexec"
)

func TestCanvasWorkflowPersistenceFailuresDoNotReportSuccess(t *testing.T) {
	newRequest := func(agentID string) WorkflowCanvasRunRequest {
		return WorkflowCanvasRunRequest{
			Workflow: WorkflowDefinition{
				ID: "canvas-persistence-boundary", Name: "Canvas Persistence Boundary", AgentID: agentID,
				CanvasGraph: &WorkflowCanvasGraph{
					Nodes: []WorkflowCanvasNode{
						canvasNode("start", "start", nil),
						canvasNode("review", "agent", map[string]any{"title": "Review", "message": "Review the durable state."}),
					},
					Edges: []WorkflowCanvasEdge{{ID: "start-review", Source: "start", Target: "review"}},
				},
			},
			Message: "Run the persistence boundary workflow.",
		}
	}

	t.Run("task write failure becomes a durable failed run", func(t *testing.T) {
		runtime := newTestRuntime(t)
		ensureTestProvider(t, runtime)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "canvas-task-write-agent", Name: "Canvas Task Write", Status: AgentStatusEnabled,
		})
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `
			CREATE TRIGGER reject_canvas_task_write
			BEFORE INSERT ON `+tableTasks+`
			BEGIN SELECT RAISE(FAIL, 'canvas task write rejected'); END
		`); err != nil {
			t.Fatalf("create task-write trigger: %v", err)
		}

		response, err := runtime.RunCanvasWorkflow(t.Context(), newRequest(agent.ID))
		if err != nil {
			t.Fatalf("RunCanvasWorkflow should return its durably failed run: %v", err)
		}
		if response.Run.Status != RunStatusFailed || !strings.Contains(response.Run.FailureReason, "canvas task write rejected") {
			t.Fatalf("canvas task-write response = %+v", response)
		}
		stored, ok, loadErr := runtime.Store().Run(t.Context(), response.Run.ID)
		if loadErr != nil || !ok || stored.Status != RunStatusFailed {
			t.Fatalf("stored failed canvas run = %+v ok=%v err=%v", stored, ok, loadErr)
		}
	})

	t.Run("task and terminal writes failing together return an error", func(t *testing.T) {
		runtime := newTestRuntime(t)
		ensureTestProvider(t, runtime)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "canvas-double-write-agent", Name: "Canvas Double Write", Status: AgentStatusEnabled,
		})
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `
			CREATE TRIGGER reject_canvas_task_and_terminal_task
			BEFORE INSERT ON `+tableTasks+`
			BEGIN SELECT RAISE(FAIL, 'canvas task write rejected'); END
		`); err != nil {
			t.Fatalf("create task-write trigger: %v", err)
		}
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `
			CREATE TRIGGER reject_canvas_task_and_terminal_run
			BEFORE UPDATE ON `+tableRuns+`
			BEGIN SELECT RAISE(FAIL, 'canvas terminal write rejected'); END
		`); err != nil {
			t.Fatalf("create run-write trigger: %v", err)
		}

		response, err := runtime.RunCanvasWorkflow(t.Context(), newRequest(agent.ID))
		if err == nil || !strings.Contains(err.Error(), "canvas terminal write rejected") {
			t.Fatalf("RunCanvasWorkflow error = %v, want terminal persistence failure", err)
		}
		if response.Run.ID != "" {
			t.Fatalf("canvas response = %+v, want no successful response", response)
		}
	})

	t.Run("plan write failure is persisted as a failed run", func(t *testing.T) {
		runtime := newTestRuntime(t)
		ensureTestProvider(t, runtime)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "canvas-plan-write-agent", Name: "Canvas Plan Write", Status: AgentStatusEnabled,
		})
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `
			CREATE TRIGGER reject_canvas_running_plan_write
			BEFORE UPDATE ON `+tableRuns+`
			WHEN NEW.status = '`+RunStatusRunning+`'
			BEGIN SELECT RAISE(FAIL, 'canvas plan write rejected'); END
		`); err != nil {
			t.Fatalf("create plan-write trigger: %v", err)
		}

		response, err := runtime.RunCanvasWorkflow(t.Context(), newRequest(agent.ID))
		if err != nil {
			t.Fatalf("RunCanvasWorkflow should return its durably failed run: %v", err)
		}
		if response.Run.Status != RunStatusFailed || !strings.Contains(response.Run.FailureReason, "canvas plan write rejected") {
			t.Fatalf("canvas plan-write response = %+v", response)
		}
	})
}

func TestNativeTaskGraphPersistsCompletedAndPendingInputOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		message    string
		permission string
		wantStatus string
	}{
		{name: "completed", message: "Summarize the durable workflow result.", permission: PermissionModeLessApproval, wantStatus: RunStatusCompleted},
		{name: "pending input", message: "@input.required choose a durable workflow option", permission: PermissionModeAll, wantStatus: RunStatusPendingInput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime := newTestRuntime(t)
			ensureTestProvider(t, runtime)
			agent := mustSaveAgent(t, runtime, AgentWriteRequest{
				ID: "native-outcome-agent-" + strings.ReplaceAll(tc.name, " ", "-"), Name: "Native Outcome",
				Status: AgentStatusEnabled, WorkMode: WorkModeLoop, PermissionMode: tc.permission,
			})
			session := mustCreateSession(t, runtime, agent.ID, "native task graph "+tc.name)
			parent := mustSaveRun(t, runtime, Run{
				ID: "native-outcome-parent-" + strings.ReplaceAll(tc.name, " ", "-"), SessionID: session.ID, AgentID: agent.ID,
				Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
				CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
			})
			step := workflowStep{Order: 1, DependencyID: "native-step", Title: "Native step", Message: tc.message, WorkflowMode: WorkModeLoop}
			task, err := runtime.Store().SaveTask(t.Context(), TaskWriteRequest{
				ID: "native-outcome-task-" + strings.ReplaceAll(tc.name, " ", "-"), Title: step.Title, Message: step.Message,
				Status: "TODO", AgentID: agent.ID, RunID: parent.ID, Order: 1, WorkflowMode: WorkModeLoop,
			})
			if err != nil {
				t.Fatalf("SaveTask: %v", err)
			}
			parent.WorkflowPlan = assistantmodel.WorkflowPlanFromTasks([]Task{task}, nil)
			mustSaveRun(t, runtime, parent)

			response, err := workflowexec.NewWorkflowExecutor(runtime).RunPlannedGoogleADKWorkflow(
				t.Context(), workflowRequest{Agent: agent, Session: session, Message: tc.message, Mode: WorkModeLoop},
				parent, []workflowStep{step}, []Task{task},
			)
			if err != nil {
				t.Fatalf("RunPlannedGoogleADKWorkflow: %v", err)
			}
			if response.Run.Status != tc.wantStatus || len(response.Run.ChildRunIDs) != 1 {
				t.Fatalf("native task graph response = %+v, want status %s", response, tc.wantStatus)
			}
			if tc.wantStatus == RunStatusPendingInput && response.InputRequest == nil {
				t.Fatalf("pending-input native response = %+v", response)
			}
		})
	}
}

func TestStaleRunRecoveryAggregatesPersistenceFailures(t *testing.T) {
	t.Run("terminal reconciliation failure reaches startup supervisor", func(t *testing.T) {
		runtime := newTestRuntime(t)
		run := mustSaveRun(t, runtime, Run{
			ID: "stale-terminal-write-run", Status: RunStatusFailed, WorkMode: WorkModeChat,
			PendingApprovals: []Approval{{ID: "stale-terminal-approval", Status: ApprovalStatusPending}},
			CreatedAt:        nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, run.ID, "reject_stale_terminal_write")

		err := runtime.reconcileStaleRuns(t.Context())
		if err == nil || !strings.Contains(err.Error(), "persist reconciled terminal run "+run.ID) {
			t.Fatalf("reconcileStaleRuns error = %v, want terminal persistence failure", err)
		}
	})

	t.Run("orphan recovery cannot claim an unpersisted failure", func(t *testing.T) {
		runtime := newTestRuntime(t)
		run := mustSaveRun(t, runtime, Run{
			ID: "stale-orphan-write-run", Status: RunStatusRunning, WorkMode: WorkModeChat,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, run.ID, "reject_stale_orphan_write")

		err := runtime.reconcileStaleRun(t.Context(), mustWorkflowExecutor(t, runtime), run)
		if err == nil || !strings.Contains(err.Error(), "persist unrecoverable run "+run.ID) {
			t.Fatalf("reconcileStaleRun error = %v, want orphan persistence failure", err)
		}
		stored, ok, loadErr := runtime.Store().Run(t.Context(), run.ID)
		if loadErr != nil || !ok || stored.Status != RunStatusRunning {
			t.Fatalf("orphan recovery changed durable state: %+v ok=%v err=%v", stored, ok, loadErr)
		}
	})

	t.Run("run listing and parent lookup failures are returned", func(t *testing.T) {
		listRuntime := newTestRuntime(t)
		if _, err := listRuntime.Store().DB().ExecContext(t.Context(), `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop run table: %v", err)
		}
		if err := listRuntime.reconcileStaleRuns(t.Context()); err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("reconcileStaleRuns list error = %v", err)
		}

		parentRuntime := newTestRuntime(t)
		if _, err := parentRuntime.Store().DB().ExecContext(t.Context(), `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop parent run table: %v", err)
		}
		cancelled, err := parentRuntime.cancelChildOfTerminalParent(t.Context(), Run{ID: "child", ParentRunID: "parent", Status: RunStatusRunning})
		if err == nil || cancelled || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("cancelChildOfTerminalParent = cancelled:%v err:%v", cancelled, err)
		}
	})
}

func TestApprovalContinuationPersistenceFailuresRemainRetryable(t *testing.T) {
	t.Run("terminal continuation failure is returned before messaging", func(t *testing.T) {
		runtime := newTestRuntime(t)
		run := mustSaveRun(t, runtime, Run{
			ID: "approval-continuation-terminal-write", SessionID: "approval-continuation-session", AgentID: "approval-continuation-agent",
			Status: RunStatusRunning, ResumeState: "approval_resuming",
			PendingApprovals: []Approval{{ID: "resolved-approval", Status: ApprovalStatusApproved, FunctionCallID: "call", ConfirmationCallID: "confirmation"}},
			CreatedAt:        nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		installRunUpdateRejectTrigger(t, runtime, run.ID, "reject_approval_continuation_terminal")

		err := runtime.markApprovalContinuationFailed(t.Context(), run.ID, errors.New("continuation provider unavailable"))
		if err == nil || !strings.Contains(err.Error(), "persist approval continuation failure") {
			t.Fatalf("markApprovalContinuationFailed error = %v, want terminal persistence failure", err)
		}
		stored, ok, loadErr := runtime.Store().Run(t.Context(), run.ID)
		if loadErr != nil || !ok || stored.Status != RunStatusRunning {
			t.Fatalf("approval continuation changed durable state: %+v ok=%v err=%v", stored, ok, loadErr)
		}
	})

	t.Run("final message reference write failure is returned", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "approval-message-write-agent", "approval message write failure")
		run := mustSaveRun(t, runtime, Run{
			ID: "approval-message-write-run", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, ResumeState: "approval_resuming",
			PendingApprovals: []Approval{{ID: "resolved-message-approval", Status: ApprovalStatusApproved, FunctionCallID: "call", ConfirmationCallID: "confirmation"}},
			CreatedAt:        nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `
			CREATE TRIGGER reject_approval_final_message_reference
			BEFORE UPDATE ON `+tableRuns+`
			WHEN NEW.id = '`+run.ID+`' AND COALESCE(json_extract(NEW.payload_json, '$.finalMessageId'), '') <> ''
			BEGIN SELECT RAISE(FAIL, 'approval message reference rejected'); END
		`); err != nil {
			t.Fatalf("create final-message trigger: %v", err)
		}

		err := runtime.markApprovalContinuationFailed(t.Context(), run.ID, errors.New("continuation provider unavailable"))
		if err == nil || !strings.Contains(err.Error(), "persist approval failure message reference") {
			t.Fatalf("markApprovalContinuationFailed error = %v, want message-reference persistence failure", err)
		}
		stored, ok, loadErr := runtime.Store().Run(t.Context(), run.ID)
		if loadErr != nil || !ok || stored.Status != RunStatusFailed || stored.FinalMessageID != "" {
			t.Fatalf("stored approval failure = %+v ok=%v err=%v", stored, ok, loadErr)
		}
	})
}

func TestWorkflowChildContinuationPersistsRecoveryDecisions(t *testing.T) {
	t.Run("missing resume session fails the parent durably", func(t *testing.T) {
		runtime := newTestRuntime(t)
		parent := mustSaveRun(t, runtime, Run{
			ID: "missing-resume-session-parent", SessionID: "missing-resume-session", AgentID: "missing-resume-agent",
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		child := Run{ID: "missing-resume-session-child", ParentRunID: parent.ID, Status: RunStatusCompleted, Message: "child completed"}

		updated, err := runtime.continueParentWorkflowAfterChild(t.Context(), child)
		if err != nil || updated == nil || updated.Status != RunStatusFailed || !strings.Contains(updated.FailureReason, "session not found") {
			t.Fatalf("continueParentWorkflowAfterChild = %+v err=%v", updated, err)
		}
		stored, ok, loadErr := runtime.Store().Run(t.Context(), parent.ID)
		if loadErr != nil || !ok || stored.Status != RunStatusFailed {
			t.Fatalf("stored failed parent = %+v ok=%v err=%v", stored, ok, loadErr)
		}
	})

	t.Run("failed recovery write is returned instead of a synthetic parent", func(t *testing.T) {
		runtime := newTestRuntime(t)
		parent := mustSaveRun(t, runtime, Run{
			ID: "missing-resume-write-parent", SessionID: "missing-resume-write-session", AgentID: "missing-resume-write-agent",
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `
			CREATE TRIGGER reject_missing_resume_terminal_write
			BEFORE UPDATE ON `+tableRuns+`
			WHEN NEW.id = '`+parent.ID+`' AND NEW.status = '`+RunStatusFailed+`'
			BEGIN SELECT RAISE(FAIL, 'missing resume terminal write rejected'); END
		`); err != nil {
			t.Fatalf("create recovery-write trigger: %v", err)
		}

		updated, err := runtime.continueParentWorkflowAfterChild(t.Context(), Run{
			ID: "missing-resume-write-child", ParentRunID: parent.ID, Status: RunStatusCompleted,
		})
		if err == nil || updated != nil || !strings.Contains(err.Error(), "missing resume terminal write rejected") {
			t.Fatalf("continueParentWorkflowAfterChild = %+v err=%v", updated, err)
		}
	})
}

func TestStaleRunRecoveryHandlesReadRepairAndParentTermination(t *testing.T) {
	t.Run("stale run reload failure is returned", func(t *testing.T) {
		runtime := newTestRuntime(t)
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `DROP TABLE `+tableRuns); err != nil {
			t.Fatalf("drop run table: %v", err)
		}
		err := runtime.reconcileStaleRun(t.Context(), mustWorkflowExecutor(t, runtime), Run{ID: "unreadable-stale-run"})
		if err == nil || !strings.Contains(err.Error(), tableRuns) {
			t.Fatalf("reconcileStaleRun reload error = %v", err)
		}
	})

	t.Run("self-reference repair storage failure is returned", func(t *testing.T) {
		runtime := newTestRuntime(t)
		parent := mustSaveRun(t, runtime, Run{
			ID: "stale-self-reference-read-parent", Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "stale-self-reference-read-task", Executor: workflowTaskExecutorChild, Status: "IN_PROGRESS"}},
			CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `DROP TABLE `+tableTasks); err != nil {
			t.Fatalf("drop task table: %v", err)
		}
		err := runtime.reconcileStaleRun(t.Context(), mustWorkflowExecutor(t, runtime), parent)
		if err == nil || !strings.Contains(err.Error(), tableTasks) {
			t.Fatalf("reconcileStaleRun repair error = %v", err)
		}
	})

	t.Run("self-reference is repaired before generic orphan handling", func(t *testing.T) {
		runtime := newTestRuntime(t)
		parent := mustSaveRun(t, runtime, Run{
			ID: "stale-self-reference-parent", Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "stale-self-reference-task", Executor: workflowTaskExecutorChild, Status: "IN_PROGRESS"}},
			CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if _, err := runtime.Store().SaveTask(t.Context(), TaskWriteRequest{
			ID: "stale-self-reference-task", Title: "Repair self reference", Status: "IN_PROGRESS",
			RunID: parent.ID, Executor: workflowTaskExecutorChild,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		if err := runtime.reconcileStaleRun(t.Context(), mustWorkflowExecutor(t, runtime), parent); err != nil {
			t.Fatalf("reconcileStaleRun self-reference: %v", err)
		}
		stored, ok, loadErr := runtime.Store().Run(t.Context(), parent.ID)
		if loadErr != nil || !ok || stored.Status != RunStatusPaused || stored.WorkflowPlan[0].Executor != "" {
			t.Fatalf("repaired self-reference = %+v ok=%v err=%v", stored, ok, loadErr)
		}
	})

	t.Run("child of a terminal parent is cancelled", func(t *testing.T) {
		runtime := newTestRuntime(t)
		parent := mustSaveRun(t, runtime, Run{
			ID: "stale-terminal-parent", Status: RunStatusFailed, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusFailed,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		child := mustSaveRun(t, runtime, Run{
			ID: "stale-terminal-child", ParentRunID: parent.ID, Status: RunStatusRunning, WorkMode: WorkModeChat,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		if err := runtime.reconcileStaleRun(t.Context(), mustWorkflowExecutor(t, runtime), child); err != nil {
			t.Fatalf("reconcileStaleRun terminal parent: %v", err)
		}
		stored, ok, loadErr := runtime.Store().Run(t.Context(), child.ID)
		if loadErr != nil || !ok || stored.Status != RunStatusCancelled || stored.ErrorCode != "PARENT_RUN_TERMINATED" {
			t.Fatalf("cancelled stale child = %+v ok=%v err=%v", stored, ok, loadErr)
		}
	})
}

func TestCanvasWorkflowSetupFailuresRemainPreRunErrors(t *testing.T) {
	t.Run("missing provider override", func(t *testing.T) {
		runtime := newTestRuntime(t)
		ensureTestProvider(t, runtime)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{ID: "canvas-provider-override-agent", Name: "Canvas Provider Override", Status: AgentStatusEnabled})
		_, err := runtime.RunCanvasWorkflow(t.Context(), WorkflowCanvasRunRequest{
			Workflow: WorkflowDefinition{
				AgentID: agent.ID, ProviderID: "missing-canvas-provider",
				CanvasGraph: &WorkflowCanvasGraph{
					Nodes: []WorkflowCanvasNode{canvasNode("start", "start", nil), canvasNode("review", "agent", nil)},
					Edges: []WorkflowCanvasEdge{{ID: "start-review", Source: "start", Target: "review"}},
				},
			},
			Message: "run with an unavailable provider override",
		})
		if err == nil || !strings.Contains(err.Error(), "provider") {
			t.Fatalf("RunCanvasWorkflow provider error = %v", err)
		}
	})

	t.Run("run row insert failure", func(t *testing.T) {
		runtime := newTestRuntime(t)
		ensureTestProvider(t, runtime)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{ID: "canvas-run-insert-agent", Name: "Canvas Run Insert", Status: AgentStatusEnabled})
		if _, err := runtime.Store().DB().ExecContext(t.Context(), `
			CREATE TRIGGER reject_canvas_run_insert
			BEFORE INSERT ON `+tableRuns+`
			BEGIN SELECT RAISE(FAIL, 'canvas run insert rejected'); END
		`); err != nil {
			t.Fatalf("create run-insert trigger: %v", err)
		}
		_, err := runtime.RunCanvasWorkflow(t.Context(), WorkflowCanvasRunRequest{
			Workflow: WorkflowDefinition{
				AgentID: agent.ID,
				CanvasGraph: &WorkflowCanvasGraph{
					Nodes: []WorkflowCanvasNode{canvasNode("start", "start", nil), canvasNode("review", "agent", nil)},
					Edges: []WorkflowCanvasEdge{{ID: "start-review", Source: "start", Target: "review"}},
				},
			},
			Message: "run with unavailable durable storage",
		})
		if err == nil || !strings.Contains(err.Error(), "canvas run insert rejected") {
			t.Fatalf("RunCanvasWorkflow run insert error = %v", err)
		}
	})
}

func installRunUpdateRejectTrigger(t *testing.T, runtime *Runtime, runID string, triggerName string) {
	t.Helper()
	if _, err := runtime.Store().DB().ExecContext(t.Context(), `
		CREATE TRIGGER `+triggerName+`
		BEFORE UPDATE ON `+tableRuns+`
		WHEN NEW.id = '`+runID+`'
		BEGIN SELECT RAISE(FAIL, '`+triggerName+`'); END
	`); err != nil {
		t.Fatalf("create %s trigger: %v", triggerName, err)
	}
}
