package adk

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestWorkflowTaskToolsetAddsClaimsAndBlocksRuntimeTasks(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-parent-runtime-tools", SessionID: "session-runtime-tools", AgentID: "agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		Objective: "完成运行时任务编排", CreatedAt: now, UpdatedAt: now,
	})
	seed, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-seed-done", Title: "收集基础数据", Status: "DONE", AgentID: parent.AgentID,
		RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode, Objective: parent.Objective,
		ResultSummary: "已完成基础数据准备",
	})
	if err != nil {
		t.Fatalf("SaveTask seed: %v", err)
	}
	toolset := &workflowTaskToolset{
		executor: &WorkflowExecutor{runtime: runtime}, parentID: parent.ID,
		req: workflowRequest{Mode: WorkModeLoop, GoalDecision: &workflowGoalDecision{}},
	}

	addedResult, err := toolset.add(map[string]any{
		"title":       "分析波动率",
		"description": "结合最近五日的回测结果分析波动率变化",
		"dependsOn":   []any{seed.ID},
		"agentRole":   "风险分析 Agent",
		"modeHint":    WorkModeTask,
	})
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	addedPayload, _ := addedResult["task"].(map[string]any)
	addedID, _ := addedPayload["id"].(string)
	added, ok, err := runtime.Store().Task(ctx, addedID)
	if err != nil || !ok {
		t.Fatalf("added task lookup ok=%v err=%v", ok, err)
	}
	if added.Status != "TODO" || added.Order != 2 || added.PlanSource != workflowPlanSourceRuntime || added.PlannerStepID != "runtime-1" {
		t.Fatalf("added task = %+v, want runtime TODO with runtime planner step", added)
	}
	if len(added.DependsOn) != 1 || added.DependsOn[0] != seed.ID || added.AgentRole != "风险分析 Agent" || added.ModeHint != WorkModeTask {
		t.Fatalf("added task = %+v, want dependency and runtime metadata", added)
	}

	claimedResult, err := toolset.claim(map[string]any{"taskId": added.ID, "executor": workflowTaskExecutorChild})
	if err != nil {
		t.Fatalf("claim child: %v", err)
	}
	claimedPayload, _ := claimedResult["task"].(map[string]any)
	if claimedPayload["executor"] != workflowTaskExecutorChild {
		t.Fatalf("claimed payload = %#v, want child executor", claimedPayload)
	}
	claimed, ok, err := runtime.Store().Task(ctx, added.ID)
	if err != nil || !ok {
		t.Fatalf("claimed task lookup ok=%v err=%v", ok, err)
	}
	if claimed.Status != "IN_PROGRESS" || claimed.Executor != workflowTaskExecutorChild {
		t.Fatalf("claimed task = %+v, want in progress child executor", claimed)
	}

	blockedResult, err := toolset.block(map[string]any{"taskId": added.ID})
	if err != nil {
		t.Fatalf("block: %v", err)
	}
	blockedPayload, _ := blockedResult["task"].(map[string]any)
	if blockedPayload["status"] != "BLOCKED" || blockedPayload["resultSummary"] != "任务被阻塞。" {
		t.Fatalf("blocked payload = %#v, want default blocked summary", blockedPayload)
	}
	blocked, ok, err := runtime.Store().Task(ctx, added.ID)
	if err != nil || !ok {
		t.Fatalf("blocked task lookup ok=%v err=%v", ok, err)
	}
	if blocked.Status != "BLOCKED" || blocked.ResultSummary != "任务被阻塞。" {
		t.Fatalf("blocked task = %+v, want blocked with default reason", blocked)
	}

	reloadedParent, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("parent run lookup ok=%v err=%v", ok, err)
	}
	if len(reloadedParent.WorkflowPlan) < 2 {
		t.Fatalf("parent workflow plan = %+v, want persisted runtime plan", reloadedParent.WorkflowPlan)
	}
}

func TestWorkflowTaskToolsetClaimDefaultsToReadySelfAndAddRejectsMissingDependency(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-parent-ready-claim", SessionID: "session-ready-claim", AgentID: "agent",
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		Objective: "推进 ready 任务", CreatedAt: now, UpdatedAt: now,
	})
	ready, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "task-ready-self", Title: "整理研究结论", Status: "TODO", AgentID: parent.AgentID,
		RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode, Objective: parent.Objective,
	})
	if err != nil {
		t.Fatalf("SaveTask ready: %v", err)
	}
	toolset := &workflowTaskToolset{
		executor: &WorkflowExecutor{runtime: runtime}, parentID: parent.ID,
		req: workflowRequest{Mode: WorkModeLoop, GoalDecision: &workflowGoalDecision{}},
	}

	if _, err := toolset.add(map[string]any{"title": "坏依赖", "dependsOn": []any{"missing-task"}}); err == nil || !strings.Contains(err.Error(), "dependency not found") {
		t.Fatalf("add missing dependency err = %v, want dependency not found", err)
	}

	claimedResult, err := toolset.claim(nil)
	if err != nil {
		t.Fatalf("claim ready self: %v", err)
	}
	claimedPayload, _ := claimedResult["task"].(map[string]any)
	if claimedPayload["id"] != ready.ID || claimedPayload["executor"] != workflowTaskExecutorSelf {
		t.Fatalf("claimed payload = %#v, want ready task claimed by self", claimedPayload)
	}
	claimed, ok, err := runtime.Store().Task(ctx, ready.ID)
	if err != nil || !ok {
		t.Fatalf("claimed ready task lookup ok=%v err=%v", ok, err)
	}
	if claimed.Status != "IN_PROGRESS" || claimed.Executor != workflowTaskExecutorSelf {
		t.Fatalf("claimed ready task = %+v, want in-progress self executor", claimed)
	}
}

func TestWorkflowExecutorRunValidatesModeAndEmitsInitialRun(t *testing.T) {
	ctx := context.Background()

	t.Run("unavailable runtime and chat mode rejected", func(t *testing.T) {
		if _, err := (*WorkflowExecutor)(nil).Run(ctx, workflowRequest{Mode: WorkModeTask}); err == nil || err.Error() != "adk runtime is unavailable" {
			t.Fatalf("nil executor error = %v, want runtime unavailable", err)
		}
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		if _, err := executor.Run(ctx, workflowRequest{Mode: WorkModeChat}); err == nil || err.Error() != "workflow mode is required" {
			t.Fatalf("chat mode error = %v, want workflow mode required", err)
		}
	})

	t.Run("task mode emits initial run delta and completes", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-run-task-agent", Name: "Workflow Run Task", ProviderID: testProviderID,
			Status: AgentStatusEnabled, WorkMode: WorkModeChat,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow run task")
		var deltas []ChatDelta

		response, err := executor.Run(ctx, workflowRequest{
			Agent:   agent,
			Session: session,
			Message: "整理一个执行清单",
			Mode:    WorkModeTask,
			EmitRun: true,
			OnDelta: func(delta ChatDelta) error {
				deltas = append(deltas, delta)
				return nil
			},
		})
		if err != nil {
			t.Fatalf("WorkflowExecutor.Run task mode: %v", err)
		}
		runningDeltaSeen := false
		for _, delta := range deltas {
			if delta.Run != nil && delta.Run.Status == RunStatusRunning {
				runningDeltaSeen = true
				break
			}
		}
		if !runningDeltaSeen {
			t.Fatalf("initial deltas = %+v, want at least one running run delta", deltas)
		}
		if response.Run.Status != RunStatusCompleted || response.Run.WorkflowStatus != workflowStatusComplete || response.Run.FinalMessageID == "" {
			t.Fatalf("response run = %+v, want completed workflow with final message", response.Run)
		}
		if !strings.Contains(response.Reply, "workflow.tasks.list") || !strings.Contains(response.Reply, "workflow.task.complete") {
			t.Fatalf("reply = %q, want tool-based task workflow completion summary", response.Reply)
		}
		if len(response.Run.WorkflowPlan) == 0 {
			t.Fatalf("workflow plan = %+v, want persisted planned steps", response.Run.WorkflowPlan)
		}
	})

	t.Run("emit run callback error stops execution", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-run-delta-error-agent", Name: "Workflow Run Delta Error", ProviderID: testProviderID,
			Status: AgentStatusEnabled, WorkMode: WorkModeChat,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow run delta error")
		wantErr := fmt.Errorf("delta sink closed")

		if _, err := executor.Run(ctx, workflowRequest{
			Agent:   agent,
			Session: session,
			Message: "整理一个执行清单",
			Mode:    WorkModeTask,
			EmitRun: true,
			OnDelta: func(ChatDelta) error { return wantErr },
		}); err == nil || err.Error() != wantErr.Error() {
			t.Fatalf("WorkflowExecutor.Run delta error = %v, want %v", err, wantErr)
		}
	})

	t.Run("planner unfinished draft fails task workflow run", func(t *testing.T) {
		runtime := newTestRuntime(t)
		executor := &WorkflowExecutor{runtime: runtime}
		providerID := saveGoalWorkflowProvider(t, runtime, "workflow-planner-incomplete-provider", func(req openAIChatRequest) openAIChatMessage {
			return openAIChatMessage{Role: "assistant", Content: "我先想一下，但不调用 planner tools。"}
		})
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "workflow-planner-incomplete-agent", Name: "Workflow Planner Incomplete", ProviderID: providerID,
			Status: AgentStatusEnabled, WorkMode: WorkModeChat,
		})
		session := mustCreateSession(t, runtime, agent.ID, "workflow planner incomplete")

		response, err := executor.Run(ctx, workflowRequest{
			Agent:   agent,
			Session: session,
			Message: "整理一个执行清单",
			Mode:    WorkModeTask,
		})
		if err != nil {
			t.Fatalf("WorkflowExecutor.Run planner incomplete: %v", err)
		}
		if response.Run.Status != RunStatusFailed || response.Run.WorkflowStatus != workflowStatusFailed {
			t.Fatalf("response run = %+v, want failed workflow", response.Run)
		}
		if !strings.Contains(response.Reply, "workflow planner failed: planner did not finish") {
			t.Fatalf("reply = %q, want planner unfinished failure", response.Reply)
		}
	})
}

func testWorkflowExecution(runID string, reply string) *googleADKExecution {
	execution := &googleADKExecution{
		runID:            runID,
		replyByRunID:     map[string]*strings.Builder{},
		reasoningByRunID: map[string]*strings.Builder{},
	}
	if reply != "" {
		execution.reply.WriteString(reply)
	}
	return execution
}

func TestPrepareGoalWorkflowTurnHandlesPendingChildAndBlockedTask(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executor := &WorkflowExecutor{runtime: runtime}
	now := nowString()

	t.Run("pending child pauses parent workflow", func(t *testing.T) {
		session := mustCreateSession(t, runtime, "agent-goal-pending-child", "goal pending child")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-goal-pending-child", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "task-goal-pending-child", Title: "等待子任务", ChildRunID: "run-goal-child-pending"}},
			CreatedAt:    now, UpdatedAt: now,
		})
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-goal-pending-child", Title: "等待子任务", Status: "DONE", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode, Objective: parent.Objective,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		child := mustSaveRun(t, runtime, Run{
			ID: "run-goal-child-pending", SessionID: session.ID, AgentID: parent.AgentID,
			ParentRunID: parent.ID, Status: RunStatusPending, Message: "waiting approval",
			CreatedAt: now, UpdatedAt: now,
		})

		updated, reply, done, prompt := executor.prepareGoalWorkflowTurn(ctx, workflowRequest{Mode: WorkModeLoop}, parent, nil, testWorkflowExecution(parent.ID, ""), nil, 2)
		if !done || prompt != "" {
			t.Fatalf("prepareGoalWorkflowTurn done=%v prompt=%q, want done with empty prompt", done, prompt)
		}
		if updated.Status != child.Status || updated.WorkflowStatus != workflowStatusPaused || updated.WorkflowCursor != 0 || updated.Iteration != 2 {
			t.Fatalf("updated parent = %+v, want paused at child cursor", updated)
		}
		if reply.Reply != "目标模式正在等待审批。" {
			t.Fatalf("reply = %#v, want workflow pending reply", reply)
		}
	})

	t.Run("blocked task fails parent", func(t *testing.T) {
		session := mustCreateSession(t, runtime, "agent-goal-blocked-task", "goal blocked task")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-goal-blocked-task", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "task-goal-blocked-task", Title: "缺数据任务"}},
			CreatedAt:    now, UpdatedAt: now,
		})
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-goal-blocked-task", Title: "缺数据任务", Status: "BLOCKED", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode, Objective: parent.Objective,
			ResultSummary: "缺少行情数据",
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}

		updated, reply, done, prompt := executor.prepareGoalWorkflowTurn(ctx, workflowRequest{Mode: WorkModeLoop}, parent, nil, testWorkflowExecution(parent.ID, ""), nil, 1)
		if !done || prompt != "" {
			t.Fatalf("prepareGoalWorkflowTurn done=%v prompt=%q, want terminal blocked result", done, prompt)
		}
		if updated.Status != RunStatusFailed || updated.WorkflowStatus != workflowStatusFailed || updated.ErrorCode != "WORKFLOW_TASK_BLOCKED" {
			t.Fatalf("updated parent = %+v, want blocked workflow failure", updated)
		}
		if reply.Reply != "缺少行情数据" || updated.FailureReason != "缺少行情数据" {
			t.Fatalf("blocked reply=%#v failure=%q, want blocked summary", reply, updated.FailureReason)
		}
	})

	t.Run("pause requested adk error pauses goal workflow", func(t *testing.T) {
		session := mustCreateSession(t, runtime, "agent-goal-pause-requested", "goal pause requested")
		pauseAt := nowString()
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-goal-pause-requested", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			PauseRequestedAt: &pauseAt, WorkflowPlan: []WorkflowStepState{{TaskID: "task-goal-pause-requested", Title: "暂停中的目标"}},
			CreatedAt: now, UpdatedAt: now,
		})
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-goal-pause-requested", Title: "暂停中的目标", Status: "DONE", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		execution := testWorkflowExecution(parent.ID, "本轮已有可见回复")

		updated, reply, done, prompt := executor.prepareGoalWorkflowTurn(ctx, workflowRequest{Session: session, Mode: WorkModeLoop}, parent, nil, execution, errUserGoalPauseRequested, 3)
		if !done || prompt != "" {
			t.Fatalf("prepareGoalWorkflowTurn done=%v prompt=%q, want paused terminal result", done, prompt)
		}
		if updated.Status != RunStatusPaused || updated.WorkflowStatus != workflowStatusPaused || updated.ResumeState != "user_paused" || updated.PausedReason != "user" {
			t.Fatalf("updated parent = %+v, want user-paused goal workflow", updated)
		}
		if reply.Reply != "本轮已有可见回复" {
			t.Fatalf("reply = %#v, want visible reply preserved while pausing", reply)
		}
	})

	t.Run("generic adk error fails goal workflow", func(t *testing.T) {
		session := mustCreateSession(t, runtime, "agent-goal-adk-error", "goal adk error")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-goal-adk-error", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "task-goal-adk-error", Title: "失败中的目标"}},
			CreatedAt:    now, UpdatedAt: now,
		})
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-goal-adk-error", Title: "失败中的目标", Status: "IN_PROGRESS", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		adkErr := fmt.Errorf("model provider timed out")

		updated, reply, done, prompt := executor.prepareGoalWorkflowTurn(ctx, workflowRequest{Session: session, Mode: WorkModeLoop}, parent, nil, testWorkflowExecution(parent.ID, ""), adkErr, 2)
		if !done || prompt != "" {
			t.Fatalf("prepareGoalWorkflowTurn done=%v prompt=%q, want failed terminal result", done, prompt)
		}
		if updated.Status != RunStatusFailed || updated.WorkflowStatus != workflowStatusFailed || updated.FailureReason != adkErr.Error() {
			t.Fatalf("updated parent = %+v, want failed goal workflow with adk error", updated)
		}
		if reply.Reply != adkErr.Error() {
			t.Fatalf("reply = %#v, want adk error reply", reply)
		}
	})
}

func TestFinishADKTaskWorkflowAttemptHandlesIncompleteAndSummaryFallback(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	executor := &WorkflowExecutor{runtime: runtime}
	now := nowString()

	t.Run("incomplete final attempt fails scheduler", func(t *testing.T) {
		session := mustCreateSession(t, runtime, "agent-task-incomplete", "task incomplete")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-task-incomplete", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "task-todo-incomplete", Title: "待完成任务"}},
			UserMessage:  "整理执行清单", CreatedAt: now, UpdatedAt: now,
		})
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-todo-incomplete", Title: "待完成任务", Status: "TODO", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode, Objective: parent.Objective,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}

		updated, response, done := executor.finishADKTaskWorkflowAttempt(ctx, workflowRequest{Session: session, Mode: WorkModeTask}, parent, nil, testWorkflowExecution(parent.ID, ""), nil, true)
		if !done {
			t.Fatal("finishADKTaskWorkflowAttempt finalAttempt should terminate incomplete scheduler")
		}
		if updated.Status != RunStatusFailed || updated.ErrorCode != workflowTaskIncompleteErr {
			t.Fatalf("updated parent = %+v, want scheduler incomplete failure", updated)
		}
		if !strings.Contains(response.Reply, "workflow task scheduler incomplete") {
			t.Fatalf("response = %+v, want scheduler incomplete reply", response)
		}
	})

	t.Run("incomplete non-final attempt keeps workflow running", func(t *testing.T) {
		session := mustCreateSession(t, runtime, "agent-task-incomplete-continue", "task incomplete continue")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-task-incomplete-continue", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "task-todo-continue", Title: "待后续完成任务"}},
			UserMessage:  "整理一个执行清单", CreatedAt: now, UpdatedAt: now,
		})
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-todo-continue", Title: "待后续完成任务", Status: "TODO", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode, Objective: parent.Objective,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}

		updated, response, done := executor.finishADKTaskWorkflowAttempt(ctx, workflowRequest{Session: session, Mode: WorkModeTask}, parent, nil, testWorkflowExecution(parent.ID, ""), nil, false)
		if done {
			t.Fatal("finishADKTaskWorkflowAttempt non-final incomplete turn should continue")
		}
		if response.Reply != "" || response.Run.ID != "" || response.Session.ID != "" {
			t.Fatalf("response = %+v, want empty response while scheduler continues", response)
		}
		if updated.Status != RunStatusRunning || updated.WorkflowStatus != workflowStatusRunning || updated.Message != "workflow running" {
			t.Fatalf("updated parent = %+v, want running workflow state", updated)
		}
	})

	t.Run("adk execution error fails parent workflow", func(t *testing.T) {
		session := mustCreateSession(t, runtime, "agent-task-adk-error", "task adk error")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-task-adk-error", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "task-adk-error", Title: "执行任务"}},
			UserMessage:  "执行一个会报错的任务", CreatedAt: now, UpdatedAt: now,
		})
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-adk-error", Title: "执行任务", Status: "IN_PROGRESS", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}

		adkErr := fmt.Errorf("provider returned 502")
		updated, response, done := executor.finishADKTaskWorkflowAttempt(ctx, workflowRequest{Session: session, Mode: WorkModeTask}, parent, nil, testWorkflowExecution(parent.ID, ""), adkErr, false)
		if !done {
			t.Fatal("finishADKTaskWorkflowAttempt should terminate on adk error")
		}
		if updated.Status != RunStatusFailed || updated.WorkflowStatus != workflowStatusFailed || updated.FailureReason != adkErr.Error() {
			t.Fatalf("updated parent = %+v, want failed parent with adk error", updated)
		}
		if response.Reply != adkErr.Error() {
			t.Fatalf("response = %+v, want adk error reply", response)
		}
	})

	t.Run("completed tasks use summary fallback when model reply empty", func(t *testing.T) {
		session := mustCreateSession(t, runtime, "agent-task-summary", "task summary")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-task-summary", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{
				{TaskID: "task-summary-a", Title: "研究", Status: "DONE"},
				{TaskID: "task-summary-b", Title: "结论", Status: "DONE"},
			},
			UserMessage: "完成一个任务编排", Objective: "输出任务编排总结", CreatedAt: now, UpdatedAt: now,
		})
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-summary-a", Title: "研究", Status: "DONE", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode, Objective: parent.Objective,
			ResultSummary: "完成市场研究",
		}); err != nil {
			t.Fatalf("SaveTask A: %v", err)
		}
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-summary-b", Title: "结论", Status: "DONE", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 2, WorkflowMode: parent.WorkMode, Objective: parent.Objective,
			ResultSummary: "形成最终结论",
		}); err != nil {
			t.Fatalf("SaveTask B: %v", err)
		}

		updated, response, done := executor.finishADKTaskWorkflowAttempt(ctx, workflowRequest{Session: session, Mode: WorkModeTask}, parent, nil, testWorkflowExecution(parent.ID, ""), nil, false)
		if !done {
			t.Fatal("finishADKTaskWorkflowAttempt should complete when all tasks are done")
		}
		if updated.Status != RunStatusCompleted || updated.WorkflowStatus != workflowStatusComplete || updated.FinalMessageID == "" {
			t.Fatalf("updated parent = %+v, want completed workflow with final message", updated)
		}
		if !strings.Contains(response.Reply, "任务编排已完成。") || !strings.Contains(response.Reply, "完成市场研究") || !strings.Contains(response.Reply, "形成最终结论") {
			t.Fatalf("response = %+v, want workflow summary fallback", response)
		}
	})

	t.Run("pending child pauses parent workflow", func(t *testing.T) {
		session := mustCreateSession(t, runtime, "agent-task-child-pending", "task child pending")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-task-child-pending", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "task-child-pending", Title: "委派子任务", ChildRunID: "run-child-pending", Status: "BLOCKED"}},
			UserMessage:  "委派一个子任务", CreatedAt: now, UpdatedAt: now,
		})
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-child-pending", Title: "委派子任务", Status: "DONE", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		mustSaveRun(t, runtime, Run{
			ID: "run-child-pending", SessionID: session.ID, AgentID: parent.AgentID,
			ParentRunID: parent.ID, Status: RunStatusPending, Message: "waiting approval",
			CreatedAt: now, UpdatedAt: now,
		})

		updated, response, done := executor.finishADKTaskWorkflowAttempt(ctx, workflowRequest{Session: session, Mode: WorkModeTask}, parent, nil, testWorkflowExecution(parent.ID, ""), nil, false)
		if !done {
			t.Fatal("finishADKTaskWorkflowAttempt should stop when child is pending")
		}
		if updated.Status != RunStatusPending || updated.WorkflowStatus != workflowStatusPaused || updated.WorkflowCursor != 0 {
			t.Fatalf("updated parent = %+v, want paused pending parent", updated)
		}
		if response.Reply != "任务编排正在等待审批。" {
			t.Fatalf("response = %+v, want pending workflow reply", response)
		}
	})

	t.Run("terminal child failure terminates parent workflow", func(t *testing.T) {
		session := mustCreateSession(t, runtime, "agent-task-child-denied", "task child denied")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-task-child-denied", SessionID: session.ID, AgentID: session.AgentID,
			Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
			WorkflowPlan: []WorkflowStepState{{TaskID: "task-child-denied", Title: "需要审批的子任务", ChildRunID: "run-child-denied", Status: "BLOCKED"}},
			UserMessage:  "处理一个会被拒绝的子任务", CreatedAt: now, UpdatedAt: now,
		})
		if _, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-child-denied", Title: "需要审批的子任务", Status: "DONE", AgentID: parent.AgentID,
			RunID: parent.ID, Order: 1, WorkflowMode: parent.WorkMode,
		}); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		mustSaveRun(t, runtime, Run{
			ID: "run-child-denied", SessionID: session.ID, AgentID: parent.AgentID,
			ParentRunID: parent.ID, Status: RunStatusDenied, Message: "child denied",
			CreatedAt: now, UpdatedAt: now,
		})

		updated, response, done := executor.finishADKTaskWorkflowAttempt(ctx, workflowRequest{Session: session, Mode: WorkModeTask}, parent, nil, testWorkflowExecution(parent.ID, ""), nil, false)
		if !done {
			t.Fatal("finishADKTaskWorkflowAttempt should stop when child is terminal")
		}
		if updated.Status != RunStatusDenied || updated.WorkflowStatus != workflowStatusFailed || updated.ErrorCode != "APPROVAL_DENIED" {
			t.Fatalf("updated parent = %+v, want denied parent workflow", updated)
		}
		if updated.FailureReason != "workflow stopped because a child approval was denied" {
			t.Fatalf("failureReason = %q, want derived denial message", updated.FailureReason)
		}
		if response.Reply != updated.FailureReason {
			t.Fatalf("response = %+v, want denial failure reply", response)
		}
	})
}

func TestRunADKTaskWorkflowUsesNudgeAndHandlesExecutionInitFailure(t *testing.T) {
	ctx := context.Background()

	t.Run("nudge second attempt completes remaining todo", func(t *testing.T) {
		runtime := newTestRuntime(t)
		var firstPromptSeen, nudgeSeen bool
		var listCount, completeCount int
		providerID := saveGoalWorkflowProvider(t, runtime, "task-nudge-provider", func(req openAIChatRequest) openAIChatMessage {
			lastUser := testGoalWorkflowLastUserMessage(req)
			seen := testGoalWorkflowToolResponsesSinceLastUser(req.Messages)
			switch {
			case strings.Contains(lastUser, "仍有未完成 TODO"):
				nudgeSeen = true
				if !seen[workflowTaskCompleteTool] {
					completeCount++
					return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
						testProviderToolCall("call-task-complete-nudge", workflowTaskCompleteTool, map[string]any{
							"taskId": "task-runadk-nudge", "resultSummary": "收到 nudge 后完成任务。",
						}),
					}}
				}
				return openAIChatMessage{Role: "assistant", Content: "第二轮已完成任务。"}
			case strings.Contains(lastUser, "请推进这个任务编排"):
				firstPromptSeen = true
				if !seen[workflowTasksListTool] {
					listCount++
					return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
						testProviderToolCall("call-task-list-only", workflowTasksListTool, map[string]any{}),
					}}
				}
				return openAIChatMessage{Role: "assistant", Content: "我先查看任务状态。"}
			default:
				return openAIChatMessage{Role: "assistant", Content: "ignored"}
			}
		})
		agent := mustSaveAgent(t, runtime, AgentWriteRequest{
			ID: "runadk-task-agent", Name: "RunADK Task", ProviderID: providerID,
			Status: AgentStatusEnabled, WorkMode: WorkModeChat,
		})
		session := mustCreateSession(t, runtime, agent.ID, "runadk task")
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-parent-task-nudge", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
			UserMessage: "整理一个执行清单", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-runadk-nudge", Title: "整理执行清单", Status: "TODO", AgentID: agent.ID,
			RunID: parent.ID, Order: 1, WorkflowMode: WorkModeTask, Objective: "整理一个执行清单",
		})
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}

		response, err := (&WorkflowExecutor{runtime: runtime}).runADKTaskWorkflow(ctx, workflowRequest{
			Agent: agent, Session: session, Message: "整理一个执行清单", Mode: WorkModeTask,
		}, parent, []Task{task})
		if err != nil {
			t.Fatalf("runADKTaskWorkflow: %v", err)
		}
		if !firstPromptSeen || !nudgeSeen || listCount != 1 || completeCount != 1 {
			t.Fatalf("prompt/list/complete state = first:%v nudge:%v list:%d complete:%d", firstPromptSeen, nudgeSeen, listCount, completeCount)
		}
		if response.Run.Status != RunStatusCompleted || response.Run.WorkflowStatus != workflowStatusComplete {
			t.Fatalf("response run = %+v, want completed workflow", response.Run)
		}
		if !strings.Contains(response.Reply, "第二轮已完成任务") {
			t.Fatalf("reply = %q, want second-turn completion reply", response.Reply)
		}
		saved, ok, err := runtime.Store().Task(ctx, task.ID)
		if err != nil || !ok {
			t.Fatalf("saved task lookup ok=%v err=%v", ok, err)
		}
		if saved.Status != "DONE" || saved.ResultSummary != "收到 nudge 后完成任务。" {
			t.Fatalf("saved task = %+v, want done after nudge", saved)
		}
	})

	t.Run("execution init failure returns failed workflow response", func(t *testing.T) {
		runtime := newTestRuntime(t)
		session := mustCreateSession(t, runtime, "agent-missing-provider", "missing provider")
		agent := Agent{ID: "agent-missing-provider", Name: "Missing Provider", Status: AgentStatusEnabled}
		parent := mustSaveRun(t, runtime, Run{
			ID: "run-parent-missing-provider", SessionID: session.ID, AgentID: agent.ID,
			Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
			UserMessage: "整理一个执行清单", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
		})
		task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
			ID: "task-missing-provider", Title: "整理执行清单", Status: "TODO", AgentID: agent.ID,
			RunID: parent.ID, Order: 1, WorkflowMode: WorkModeTask,
		})
		if err != nil {
			t.Fatalf("SaveTask: %v", err)
		}

		response, err := (&WorkflowExecutor{runtime: runtime}).runADKTaskWorkflow(ctx, workflowRequest{
			Agent: agent, Session: session, Message: "整理一个执行清单", Mode: WorkModeTask,
		}, parent, []Task{task})
		if err != nil {
			t.Fatalf("runADKTaskWorkflow missing provider: %v", err)
		}
		if response.Run.Status != RunStatusFailed || response.Run.WorkflowStatus != workflowStatusFailed {
			t.Fatalf("response run = %+v, want failed workflow", response.Run)
		}
		if response.Reply != "agent provider is required" {
			t.Fatalf("reply = %q, want provider-required failure", response.Reply)
		}
	})
}
