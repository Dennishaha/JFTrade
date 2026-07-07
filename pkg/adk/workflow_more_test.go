package adk

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWorkflowExecutorRunAndFinalizeAdditionalCoverage(t *testing.T) {
	ctx := t.Context()

	t.Run("run surfaces start-run and initial task persistence failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		session := mustCreateSession(t, runtime, "workflow-run-extra-agent", "workflow run extra")

		if _, err := executor.Run(ctx, workflowRequest{
			Agent:   Agent{ID: session.AgentID, Name: "Missing Provider", ProviderID: "missing-provider", Status: AgentStatusEnabled},
			Session: session,
			Message: "start run should fail",
			Mode:    WorkModeLoop,
		}); err == nil {
			t.Fatal("WorkflowExecutor.Run accepted missing provider")
		}

		loopRuntime := newTestRuntime(t)
		loopExecutor := &WorkflowExecutor{runtime: loopRuntime}
		loopAgent := mustSaveAgent(t, loopRuntime, AgentWriteRequest{
			ID: "workflow-loop-save-fail-agent", Name: "Workflow Loop Save Fail", ProviderID: testProviderID,
			Status: AgentStatusEnabled, WorkMode: WorkModeChat,
		})
		loopSession := mustCreateSession(t, loopRuntime, loopAgent.ID, "workflow loop save fail")
		if _, err := loopRuntime.Store().db.ExecContext(ctx, `DROP TABLE `+tableTasks); err != nil {
			t.Fatalf("drop tasks table: %v", err)
		}
		response, err := loopExecutor.Run(ctx, workflowRequest{
			Agent:   loopAgent,
			Session: loopSession,
			Message: "start a goal workflow",
			Mode:    WorkModeLoop,
		})
		if err != nil {
			t.Fatalf("WorkflowExecutor.Run loop save failure: %v", err)
		}
		if response.Run.Status != RunStatusFailed || response.Run.WorkflowStatus != workflowStatusFailed || response.Reply == "" {
			t.Fatalf("loop save failure response = %+v", response)
		}
	})

	t.Run("run surfaces task persistence failures after planning", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-task-save-fail-agent", Name: "Workflow Task Save Fail", ProviderID: testProviderID,
			Status: AgentStatusEnabled, WorkMode: WorkModeChat,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow task save fail")
		if _, err := runtime.Store().db.ExecContext(ctx, `DROP TABLE `+tableTasks); err != nil {
			t.Fatalf("drop tasks table: %v", err)
		}
		response, err := executor.Run(ctx, workflowRequest{
			Agent:   agent,
			Session: session,
			Message: "plan a task workflow",
			Mode:    WorkModeLoop,
		})
		if err != nil {
			t.Fatalf("WorkflowExecutor.Run task save failure: %v", err)
		}
		if response.Run.Status != RunStatusFailed || response.Run.WorkflowStatus != workflowStatusFailed || response.Reply == "" {
			t.Fatalf("task save failure response = %+v", response)
		}
	})

	t.Run("sanitizeWorkflowPlanStep rewrites echoed messages and descriptions", func(t *testing.T) {
		step := sanitizeWorkflowPlanStep(workflowStep{
			Title: "same request", Description: "same request", Message: "same request",
		}, "same request", 1)
		if step.Title != "执行计划步骤 2" || step.Description != "" || step.Message != "推进计划中的第 2 步。" {
			t.Fatalf("sanitizeWorkflowPlanStep = %+v", step)
		}

		step = sanitizeWorkflowPlanStep(workflowStep{
			Message: "same request", Description: "custom description",
		}, "same request", 0)
		if step.Message != "custom description" {
			t.Fatalf("sanitizeWorkflowPlanStep message = %+v, want description fallback", step)
		}
	})

	t.Run("finalize planned workflow sets loop iteration and falls back when final message persistence fails", func(t *testing.T) {
		runtime := newTestRuntime(t)
		runtime.rawSessionService = createErrorSessionService{err: errors.New("create failed")}
		executor := &WorkflowExecutor{runtime: runtime}
		session := mustCreateSession(t, runtime, "workflow-finalize-fallback-agent", "workflow finalize fallback")
		parent := mustSaveRun(t, runtime, Run{
			ID: "workflow-finalize-fallback-parent", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "workflow-finalize-fallback-task", Title: "Done", Status: "DONE", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode, ResultSummary: "done",
		})
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		parent.WorkflowPlan = workflowPlanFromTasks([]Task{task}, nil)
		mustSaveRun(t, runtime, parent)

		response := executor.finalizePlannedWorkflow(ctx, workflowRequest{
			Agent:   Agent{ID: parent.AgentID, Status: AgentStatusEnabled},
			Session: session,
			Mode:    WorkModeLoop,
		}, parent, []Task{task}, nil, nil)
		if response.Run.Status != RunStatusCompleted || response.Run.WorkflowStatus != workflowStatusComplete || response.Run.Iteration != 1 {
			t.Fatalf("finalizePlannedWorkflow response = %+v", response)
		}
		if response.Run.FinalMessageID != "" || strings.TrimSpace(response.Reply) == "" {
			t.Fatalf("finalizePlannedWorkflow response = %+v", response)
		}
		stored, ok, err := runtime.Store().Run(ctx, parent.ID)
		if err != nil || !ok {
			t.Fatalf("stored run lookup ok=%v err=%v", ok, err)
		}
		if stored.Status != RunStatusCompleted || stored.FinalMessageID != "" || stored.Iteration != 1 {
			t.Fatalf("stored finalized run = %+v", stored)
		}
	})
}

func TestWorkflowExecutorAdditionalSaveAndCancelBranches(t *testing.T) {
	ctx := t.Context()

	t.Run("prepare workflow parent surfaces save failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-prepare-save-error-agent", Name: "Workflow Prepare Save Error", ProviderID: testProviderID, Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow prepare save error")
		now := nowString()
		parent := Run{
			ID:             "workflow-prepare-save-error-parent",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeLoop,
			WorkflowStatus: workflowStatusRunning,
			WorkflowPlan:   []WorkflowStepState{{TaskID: "task-1", Title: "Step 1"}},
			ToolCalls:      []ToolCall{{ID: "bad-call", Output: make(chan int)}},
			CreatedAt:      now,
			UpdatedAt:      now,
			Usage:          &RunUsage{},
		}
		child := Run{
			ID: "workflow-prepare-save-error-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
			Status: RunStatusRunning, CreatedAt: now, UpdatedAt: now, Usage: &RunUsage{},
		}

		if _, err := executor.prepareWorkflowParent(ctx, workflowRequest{
			Agent: agent, Session: session, Message: "prepare workflow parent",
		}, parent, []Run{child}); err == nil || !strings.Contains(err.Error(), "unsupported type") {
			t.Fatalf("prepareWorkflowParent err = %v, want unsupported type", err)
		}
	})

	t.Run("blocked workflow loop parents advance iteration and cancelled parents record cancelledAt", func(t *testing.T) {
		blocked := finalizeBlockedWorkflowParent(Run{WorkMode: WorkModeLoop}, Run{
			Status: RunStatusFailed, FailureReason: "failed child", ErrorCode: "CHILD_FAILED",
		}, nil)
		if blocked.Iteration != 1 {
			t.Fatalf("finalizeBlockedWorkflowParent iteration = %d, want 1", blocked.Iteration)
		}

		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-fail-parent-agent", Name: "Workflow Fail Parent", ProviderID: testProviderID, Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow fail parent")
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()
		parent := Run{
			ID:             "workflow-fail-parent-run",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeLoop,
			WorkflowStatus: workflowStatusRunning,
			CreatedAt:      nowString(),
			UpdatedAt:      nowString(),
			Usage:          &RunUsage{},
		}
		failed := executor.failParent(cancelledCtx, parent, errors.New("cancelled"))
		if failed.Status != RunStatusCancelled || failed.CancelledAt == nil {
			t.Fatalf("failParent(cancelled) = %+v", failed)
		}
	})

	t.Run("native workflow reports parent preparation and graph compilation failures", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-native-error-agent", Name: "Workflow Native Error", ProviderID: testProviderID, Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow native error")
		now := nowString()

		prepareParent := Run{
			ID:             "workflow-native-prepare-parent",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeLoop,
			WorkflowStatus: workflowStatusRunning,
			ToolCalls:      []ToolCall{{ID: "bad-call", Output: make(chan int)}},
			CreatedAt:      now,
			UpdatedAt:      now,
			Usage:          &RunUsage{},
		}
		prepareStep := workflowStep{Order: 1, DependencyID: "step-1", Title: "Prepare child", Message: "run prepare child"}
		prepareTask, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "workflow-native-prepare-task", Title: prepareStep.Title, Status: "TODO", AgentID: agent.ID,
			RunID: prepareParent.ID, Order: 1, WorkflowMode: WorkModeLoop, Message: prepareStep.Message,
		})
		if err != nil {
			t.Fatalf("SaveTask(prepare): %v", err)
		}
		response, err := executor.runNativeTaskGraphWorkflow(ctx, workflowRequest{
			Agent: agent, Session: session, Message: "run native workflow prepare failure",
		}, prepareParent, []workflowStep{prepareStep}, []Task{prepareTask})
		if err != nil {
			t.Fatalf("runNativeTaskGraphWorkflow prepare err = %v", err)
		}
		if response.Run.Status != RunStatusFailed || !strings.Contains(response.Reply, "unsupported type") {
			t.Fatalf("prepare failure response = %+v", response)
		}

		compileParent := Run{
			ID:             "workflow-native-compile-parent",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeLoop,
			WorkflowStatus: workflowStatusRunning,
			CreatedAt:      now,
			UpdatedAt:      now,
			Usage:          &RunUsage{},
		}
		compileStep := workflowStep{
			Order: 1, DependencyID: "step-1", Title: "Compile child", Message: "run compile child", DependsOn: []string{"missing-step"},
		}
		compileTask, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "workflow-native-compile-task", Title: compileStep.Title, Status: "TODO", AgentID: agent.ID,
			RunID: compileParent.ID, Order: 1, WorkflowMode: WorkModeLoop, Message: compileStep.Message,
		})
		if err != nil {
			t.Fatalf("SaveTask(compile): %v", err)
		}
		response, err = executor.runNativeTaskGraphWorkflow(ctx, workflowRequest{
			Agent: agent, Session: session, Message: "run native workflow compile failure",
		}, compileParent, []workflowStep{compileStep}, []Task{compileTask})
		if err != nil {
			t.Fatalf("runNativeTaskGraphWorkflow compile err = %v", err)
		}
		if response.Run.Status != RunStatusFailed || !strings.Contains(response.Reply, "unknown dependency") {
			t.Fatalf("compile failure response = %+v", response)
		}
	})
}
