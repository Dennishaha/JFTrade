package adk

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/genai"
)

func TestWorkflowFinalSynthesisFailureMarksChildRunFailed(t *testing.T) {
	ctx := context.Background()

	buildExecution := func(t *testing.T, runtime *Runtime, agent Agent, session Session, parent Run, child Run) *googleADKExecution {
		t.Helper()
		steps := []workflowStep{{Title: child.UserMessage, Message: child.UserMessage}}
		execution, err := runtime.newGoogleADKWorkflowExecution(ctx, agent, session, parent, []Run{child}, steps, WorkModeTask, RunOptions{}, nil)
		if err != nil {
			t.Fatalf("newGoogleADKWorkflowExecution: %v", err)
		}
		call := execution.ensureCallForRun("call-final-synth-failure", ToolDescriptor{Name: "market.candles", Permission: "read"}, map[string]any{"symbol": "TME"}, child.ID)
		execution.finishCall(call.ID, map[string]any{"symbol": "TME", "close": 10.2}, nil)
		execution.consumeFunctionResponse(&genai.FunctionResponse{
			ID:       "call-final-synth-failure",
			Name:     "market.candles",
			Response: map[string]any{"symbol": "TME", "close": 10.2},
		})
		if !execution.runNeedsFinalSynthesis(child.ID) {
			t.Fatal("child run should need final synthesis before failure path")
		}
		return execution
	}

	t.Run("provider setup error fails child terminally", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-final-synth-fail-agent", Name: "Workflow Final Synth Fail", Status: AgentStatusEnabled,
			WorkMode: WorkModeTask,
		})
		session := mustCreateSession(t, runtime, agent.ID, "final synth fail")
		parent := mustSaveRun(t, runtime, Run{
			ID: "parent-final-synth-fail", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeTask,
			WorkflowPlan: []WorkflowStepState{{TaskID: "step-final-synth-fail", Title: "读取数据后总结", Status: "IN_PROGRESS"}},
			CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		child := mustSaveRun(t, runtime, Run{
			ID: "child-final-synth-fail", SessionID: session.ID, AgentID: agent.ID,
			ParentRunID: parent.ID, Status: RunStatusRunning, UserMessage: "读取数据后总结",
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		execution := buildExecution(t, runtime, agent, session, parent, child)
		badAgent := agent
		badAgent.ProviderID = "missing-final-synth-provider"

		executor := &WorkflowExecutor{runtime: runtime}
		err := executor.ensureWorkflowChildrenFinalReplies(ctx, workflowRequest{
			Agent: badAgent, Session: session, Message: child.UserMessage,
		}, execution, []Run{child}, []workflowStep{{Title: child.UserMessage, Message: child.UserMessage}}, nil)
		if err == nil || err.Error() != "agent provider is unavailable" {
			t.Fatalf("ensureWorkflowChildrenFinalReplies err = %v, want provider unavailable", err)
		}
		stored, ok, getErr := runtime.Store().Run(ctx, child.ID)
		if getErr != nil || !ok {
			t.Fatalf("child run lookup ok=%v err=%v", ok, getErr)
		}
		if stored.Status != RunStatusFailed || stored.FailureReason != "agent provider is unavailable" {
			t.Fatalf("stored child = %+v, want failed child with provider error", stored)
		}
	})

	t.Run("missing post-tool final reply fails child terminally", func(t *testing.T) {
		runtime := newTestRuntime(t)
		providerID := saveGoalWorkflowProvider(t, runtime, "workflow-final-synth-empty-provider", func(req openAIChatRequest) openAIChatMessage {
			return openAIChatMessage{Role: "assistant", Content: "   "}
		})
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-final-synth-empty-agent", Name: "Workflow Final Synth Empty", ProviderID: providerID,
			Status: AgentStatusEnabled, WorkMode: WorkModeTask,
		})
		session := mustCreateSession(t, runtime, agent.ID, "final synth empty")
		parent := mustSaveRun(t, runtime, Run{
			ID: "parent-final-synth-empty", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeTask,
			WorkflowPlan: []WorkflowStepState{{TaskID: "step-final-synth-empty", Title: "读取数据后总结", Status: "IN_PROGRESS"}},
			CreatedAt:    nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		child := mustSaveRun(t, runtime, Run{
			ID: "child-final-synth-empty", SessionID: session.ID, AgentID: agent.ID,
			ParentRunID: parent.ID, Status: RunStatusRunning, UserMessage: "读取数据后总结",
			CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		execution := buildExecution(t, runtime, agent, session, parent, child)

		executor := &WorkflowExecutor{runtime: runtime}
		err := executor.ensureWorkflowChildrenFinalReplies(ctx, workflowRequest{
			Agent: agent, Session: session, Message: child.UserMessage,
		}, execution, []Run{child}, []workflowStep{{Title: child.UserMessage, Message: child.UserMessage}}, nil)
		if err == nil || !strings.Contains(err.Error(), "最终回复") {
			t.Fatalf("ensureWorkflowChildrenFinalReplies err = %v, want missing final reply", err)
		}
		stored, ok, getErr := runtime.Store().Run(ctx, child.ID)
		if getErr != nil || !ok {
			t.Fatalf("child run lookup ok=%v err=%v", ok, getErr)
		}
		if stored.Status != RunStatusFailed || !strings.Contains(stored.FailureReason, "最终回复") {
			t.Fatalf("stored child = %+v, want failed child with missing final reply", stored)
		}
	})
}

func TestRunGoogleADKWorkflowCompletesAndPausesForChildApprovals(t *testing.T) {
	ctx := context.Background()

	t.Run("planned child workflow completes end to end", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "planned-workflow-complete-agent", Name: "Planned Workflow Complete", Status: AgentStatusEnabled,
			WorkMode: WorkModeTask,
		})
		session := mustCreateSession(t, runtime, agent.ID, "planned workflow complete")
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID:             "run-planned-workflow-complete",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeTask,
			WorkflowStatus: workflowStatusRunning,
			Objective:      "完成计划工作流",
			UserMessage:    "执行计划工作流",
			CreatedAt:      now,
			StartedAt:      now,
			UpdatedAt:      now,
			Usage:          &RunUsage{},
		})
		steps := []workflowStep{{
			Title: "分析子任务", Message: "分析子任务并给出结论", Description: "读取输入并给出总结。",
		}}
		task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-planned-workflow-complete", Title: "分析子任务", Status: "TODO", AgentID: agent.ID,
			RunID: parent.ID, Order: 1, WorkflowMode: WorkModeTask, Objective: parent.Objective, Message: steps[0].Message,
		})
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		parent.WorkflowPlan = workflowPlanFromSteps(steps, []Task{task})
		if err := runtime.Store().SaveRun(ctx, parent); err != nil {
			t.Fatalf("SaveRun parent: %v", err)
		}

		response, err := (&WorkflowExecutor{runtime: runtime}).runGoogleADKWorkflow(ctx, workflowRequest{
			Agent: agent, Session: session, Message: "执行计划工作流", Mode: WorkModeTask, Objective: parent.Objective,
		}, parent, steps, []Task{task})
		if err != nil {
			t.Fatalf("runGoogleADKWorkflow: %v", err)
		}
		if response.Run.Status != RunStatusCompleted || response.Run.WorkflowStatus != workflowStatusComplete {
			t.Fatalf("response run = %+v, want completed workflow", response.Run)
		}
		if len(response.Run.ChildRunIDs) != 1 {
			t.Fatalf("child run ids = %+v, want one child run", response.Run.ChildRunIDs)
		}
		if strings.TrimSpace(response.Reply) == "" || !strings.Contains(response.Reply, "tools.search") {
			t.Fatalf("reply = %q, want aggregated child reply content", response.Reply)
		}
		storedTask, ok, err := runtime.Store().Task(ctx, task.ID)
		if err != nil || !ok {
			t.Fatalf("stored task lookup ok=%v err=%v", ok, err)
		}
		if storedTask.Status != "DONE" || storedTask.RunID == "" {
			t.Fatalf("stored task = %+v, want DONE child task", storedTask)
		}
		child, ok, err := runtime.Store().Run(ctx, storedTask.RunID)
		if err != nil || !ok {
			t.Fatalf("child run lookup ok=%v err=%v", ok, err)
		}
		if child.ParentRunID != parent.ID || child.Status != RunStatusCompleted || child.FinalMessageID == "" {
			t.Fatalf("stored child = %+v, want completed child with final message", child)
		}
	})

	t.Run("planned child workflow pauses parent for approval", func(t *testing.T) {
		runtime, executions := newWorkflowApprovalRuntime(t, WorkModeTask)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "planned-workflow-approval-agent", Name: "Planned Workflow Approval", Status: AgentStatusEnabled,
			WorkMode: WorkModeTask, Tools: []string{"approval.required"}, PermissionMode: PermissionModeApproval,
		})
		session := mustCreateSession(t, runtime, agent.ID, "planned workflow approval")
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID:             "run-planned-workflow-approval",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeTask,
			WorkflowStatus: workflowStatusRunning,
			Objective:      "等待子审批",
			UserMessage:    "执行需要审批的计划",
			CreatedAt:      now,
			StartedAt:      now,
			UpdatedAt:      now,
			Usage:          &RunUsage{},
		})
		steps := []workflowStep{{
			Title: "保存策略草稿", Message: "请 @approval.required 保存策略", Description: "需要审批后保存策略。",
		}}
		task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-planned-workflow-approval", Title: "保存策略草稿", Status: "TODO", AgentID: agent.ID,
			RunID: parent.ID, Order: 1, WorkflowMode: WorkModeTask, Objective: parent.Objective, Message: steps[0].Message,
		})
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		parent.WorkflowPlan = workflowPlanFromSteps(steps, []Task{task})
		if err := runtime.Store().SaveRun(ctx, parent); err != nil {
			t.Fatalf("SaveRun parent: %v", err)
		}

		response, err := (&WorkflowExecutor{runtime: runtime}).runGoogleADKWorkflow(ctx, workflowRequest{
			Agent: agent, Session: session, Message: "执行需要审批的计划", Mode: WorkModeTask, Objective: parent.Objective,
		}, parent, steps, []Task{task})
		if err != nil {
			t.Fatalf("runGoogleADKWorkflow approval: %v", err)
		}
		if response.Run.Status != RunStatusPending || response.Run.WorkflowStatus != workflowStatusPaused {
			t.Fatalf("response run = %+v, want paused pending workflow", response.Run)
		}
		if len(response.PendingApprovals) != 1 {
			t.Fatalf("pending approvals = %+v, want one child approval", response.PendingApprovals)
		}
		if executions.Load() != 0 {
			t.Fatalf("tool executions = %d, want 0 before approval", executions.Load())
		}
		storedTask, ok, err := runtime.Store().Task(ctx, task.ID)
		if err != nil || !ok {
			t.Fatalf("stored task lookup ok=%v err=%v", ok, err)
		}
		if storedTask.Status != "BLOCKED" || storedTask.RunID == "" {
			t.Fatalf("stored task = %+v, want blocked child task", storedTask)
		}
		child, ok, err := runtime.Store().Run(ctx, storedTask.RunID)
		if err != nil || !ok {
			t.Fatalf("child run lookup ok=%v err=%v", ok, err)
		}
		if child.Status != RunStatusPending || child.ParentRunID != parent.ID || len(child.PendingApprovals) != 1 {
			t.Fatalf("stored child = %+v, want pending child approval run", child)
		}
		runtime.adkMu.Lock()
		_, parentTracked := runtime.adkRuns[parent.ID]
		_, childTracked := runtime.adkRuns[child.ID]
		runtime.adkMu.Unlock()
		if !parentTracked || !childTracked {
			t.Fatalf("adk runs tracked parent=%v child=%v, want both tracked for approval resume", parentTracked, childTracked)
		}
	})

	t.Run("planned workflow without steps fails safely", func(t *testing.T) {
		runtime := newTestRuntime(t)
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "planned-workflow-empty-agent", Name: "Planned Workflow Empty", Status: AgentStatusEnabled,
			WorkMode: WorkModeTask,
		})
		session := mustCreateSession(t, runtime, agent.ID, "planned workflow empty")
		now := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID:             "run-planned-workflow-empty",
			SessionID:      session.ID,
			AgentID:        agent.ID,
			Status:         RunStatusRunning,
			WorkMode:       WorkModeTask,
			WorkflowStatus: workflowStatusRunning,
			Objective:      "空计划应失败",
			UserMessage:    "执行空计划",
			CreatedAt:      now,
			StartedAt:      now,
			UpdatedAt:      now,
			Usage:          &RunUsage{},
		})

		response, err := (&WorkflowExecutor{runtime: runtime}).runPlannedGoogleADKWorkflow(ctx, workflowRequest{
			Agent: agent, Session: session, Message: "执行空计划", Mode: WorkModeTask, Objective: parent.Objective,
		}, parent, nil, nil)
		if err != nil {
			t.Fatalf("runPlannedGoogleADKWorkflow empty: %v", err)
		}
		if response.Run.Status != RunStatusFailed || response.Run.WorkflowStatus != workflowStatusFailed {
			t.Fatalf("response run = %+v, want failed workflow for empty plan", response.Run)
		}
		if !strings.Contains(response.Reply, "workflow requires at least one sub-agent") {
			t.Fatalf("reply = %q, want empty-plan failure", response.Reply)
		}
	})
}

func TestFinishWorkflowChildrenInvokesEveryCleanup(t *testing.T) {
	var order []int
	finishWorkflowChildren([]func(){
		func() { order = append(order, 1) },
		func() { order = append(order, 2) },
	})
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("finish order = %+v, want [1 2]", order)
	}
}
