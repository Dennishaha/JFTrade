package adk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/genai"
)

func TestSaveAgentNormalizesWorkflowDefaults(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-defaults", Name: "Workflow Defaults", Status: AgentStatusEnabled,
		LoopMaxIterations: 99,
	})
	if agent.WorkMode != WorkModeChat {
		t.Fatalf("work mode = %q, want %q", agent.WorkMode, WorkModeChat)
	}
	if agent.LoopMaxIterations != MaxLoopIterations {
		t.Fatalf("loop max iterations = %d, want %d", agent.LoopMaxIterations, MaxLoopIterations)
	}
	sequential := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-hidden-sequential", Name: "Workflow Hidden Sequential", Status: AgentStatusEnabled,
		WorkMode: "sequential",
	})
	if sequential.WorkMode != WorkModeChat {
		t.Fatalf("sequential default work mode = %q, want %q", sequential.WorkMode, WorkModeChat)
	}
	parallel := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-hidden-parallel", Name: "Workflow Hidden Parallel", Status: AgentStatusEnabled,
		WorkMode: "parallel",
	})
	if parallel.WorkMode != WorkModeChat {
		t.Fatalf("parallel default work mode = %q, want %q", parallel.WorkMode, WorkModeChat)
	}
	taskAgent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-task-agent", Name: "Workflow Task", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	if taskAgent.WorkMode != WorkModeTask {
		t.Fatalf("task default work mode = %q, want %q", taskAgent.WorkMode, WorkModeTask)
	}
}

func TestSequentialParallelWorkModesAreRejectedForRuns(t *testing.T) {
	if got := normalizeWorkMode("sequential"); got != WorkModeChat {
		t.Fatalf("normalizeWorkMode(sequential) = %q, want %q", got, WorkModeChat)
	}
	if got := normalizeWorkMode("parallel"); got != WorkModeChat {
		t.Fatalf("normalizeWorkMode(parallel) = %q, want %q", got, WorkModeChat)
	}
	for _, mode := range []string{"sequential", "parallel"} {
		if validWorkMode(mode) {
			t.Fatalf("validWorkMode(%q) = true, want false", mode)
		}
	}
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "invalid-work-mode-agent", Name: "Invalid Work Mode", Status: AgentStatusEnabled,
	})
	if _, err := runtime.Chat(context.Background(), ChatRequest{
		AgentID: agent.ID, Message: "hello", WorkModeOverride: "sequential",
	}); err == nil || !strings.Contains(err.Error(), "invalid work mode") {
		t.Fatalf("Chat invalid sequential err = %v, want invalid work mode", err)
	}
}

func TestTaskWorkflowCanCompleteTODOWithoutChildRun(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "task-agent-self", Name: "Task Agent", Status: AgentStatusEnabled, WorkMode: WorkModeTask,
	})
	response, err := runtime.Chat(context.Background(), ChatRequest{
		AgentID:          agent.ID,
		Message:          "整理一个执行清单",
		WorkModeOverride: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("Chat task workflow: %v", err)
	}
	if response.Run.WorkMode != WorkModeTask || response.Run.WorkflowStatus != workflowStatusComplete {
		t.Fatalf("parent run = %+v, want completed task workflow", response.Run)
	}
	if len(response.Run.ChildRunIDs) != 0 {
		t.Fatalf("child run ids = %+v, want none for self-completed task", response.Run.ChildRunIDs)
	}
	if len(response.Run.WorkflowPlan) == 0 {
		t.Fatalf("workflow plan is empty")
	}
	step := response.Run.WorkflowPlan[0]
	if step.Status != "DONE" || step.Executor != workflowTaskExecutorSelf || step.ChildRunID != "" {
		t.Fatalf("workflow step = %+v, want self DONE without child run", step)
	}
}

func TestTaskWorkflowDelegatesChildRunOnlyWhenToolCalled(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "task-agent-child", Name: "Task Agent", Status: AgentStatusEnabled, WorkMode: WorkModeTask,
	})
	response, err := runtime.Chat(context.Background(), ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体完成行情分析",
		WorkModeOverride: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("Chat delegated task workflow: %v", err)
	}
	if response.Run.WorkMode != WorkModeTask || response.Run.WorkflowStatus != workflowStatusComplete {
		t.Fatalf("parent run = %+v, want completed task workflow", response.Run)
	}
	if len(response.Run.ChildRunIDs) != 1 {
		t.Fatalf("child run ids = %+v, want delegated child run", response.Run.ChildRunIDs)
	}
	if len(response.Run.WorkflowPlan) == 0 || response.Run.WorkflowPlan[0].ChildRunID != response.Run.ChildRunIDs[0] {
		t.Fatalf("workflow plan = %+v child ids=%+v, want child mapped to task", response.Run.WorkflowPlan, response.Run.ChildRunIDs)
	}
	if response.Run.WorkflowPlan[0].Executor != workflowTaskExecutorChild || response.Run.WorkflowPlan[0].Status != "DONE" {
		t.Fatalf("workflow step = %+v, want child DONE", response.Run.WorkflowPlan[0])
	}
	hasDelegateCall := false
	for _, call := range response.Run.ToolCalls {
		if call.ToolName == workflowTaskDelegateTool {
			hasDelegateCall = true
		}
		if call.RunID != response.Run.ID {
			t.Fatalf("parent tool call = %+v, want only parent-owned task tool calls", call)
		}
	}
	if !hasDelegateCall {
		t.Fatalf("parent tool calls = %+v, want %s", response.Run.ToolCalls, workflowTaskDelegateTool)
	}
	child, ok, err := runtime.Store().Run(context.Background(), response.Run.ChildRunIDs[0])
	if err != nil || !ok {
		t.Fatalf("child run lookup err=%v ok=%v", err, ok)
	}
	if child.ParentRunID != response.Run.ID || child.Status != RunStatusCompleted {
		t.Fatalf("child run = %+v, want completed child owned by parent", child)
	}
}

func TestTaskWorkflowStreamEmitsParentAfterChildRunCreated(t *testing.T) {
	ctx := context.Background()
	runtime, _ := newWorkflowApprovalRuntime(t, WorkModeTask)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "task-agent-child-stream", Name: "Task Agent Child Stream", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask, Tools: []string{"strategy.save_draft"}, PermissionMode: PermissionModeApproval,
	})

	var runDeltas []Run
	response, err := runtime.ChatStream(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体并 @strategy.save_draft 保存策略",
		WorkModeOverride: WorkModeTask,
	}, func(delta ChatDelta) error {
		if delta.Run != nil {
			runDeltas = append(runDeltas, *delta.Run)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream delegated task workflow: %v", err)
	}
	if len(response.PendingApprovals) != 1 {
		t.Fatalf("pending approvals = %+v, want child approval", response.PendingApprovals)
	}

	parentIndex := -1
	for index, delta := range runDeltas {
		if delta.ID != response.Run.ID {
			continue
		}
		if len(delta.ChildRunIDs) == 1 && len(delta.WorkflowPlan) > 0 && delta.WorkflowPlan[0].ChildRunID == delta.ChildRunIDs[0] {
			parentIndex = index
			break
		}
	}
	if parentIndex < 0 {
		t.Fatalf("run deltas = %+v, want parent delta with child run id before approval resolution", runDeltas)
	}
	if childID := response.PendingApprovals[0].RunID; childID == "" || runDeltas[parentIndex].ChildRunIDs[0] != childID {
		t.Fatalf("parent delta child ids = %+v approval=%+v, want pending child run", runDeltas[parentIndex].ChildRunIDs, response.PendingApprovals[0])
	}
}

func TestWorkflowPlannerCreatesDynamicStrategySteps(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "planner-agent", Name: "Planner", Status: AgentStatusEnabled, WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "planner")
	objective := "编写个tme的策略，要求不可以过度拟合，而且年收益20%+，交易频次年30内"
	executor := &WorkflowExecutor{runtime: runtime}
	steps, warnings, err := compileWorkflowPlanDraft(workflowPlanDraft{
		Finished: true,
		Steps: []workflowPlanDraftStep{
			{Order: 1, Title: "收集约束", Message: "收集约束", AgentRole: "数据与约束收集子 Agent"},
			{Order: 2, Title: "定义策略", Message: "定义策略", AgentRole: "策略定义子 Agent", DependsOn: []string{"1"}},
			{Order: 3, Title: "验证策略", Message: "验证策略", AgentRole: "验证与风控子 Agent", DependsOn: []string{"2"}},
		},
	}, WorkModeTask, objective, objective, RunOptions{})
	if err != nil {
		t.Fatalf("compile planner draft: %v", err)
	}
	steps = applyWorkflowStepPlanningMetadata(steps, WorkModeTask, objective, warnings)
	parent := mustSaveRun(t, runtime, Run{
		ID: "planner-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
		Objective: objective, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	tasks, err := executor.persistWorkflowTasks(ctx, parent, agent, steps)
	if err != nil {
		t.Fatalf("persistWorkflowTasks: %v", err)
	}
	plan := workflowPlanFromSteps(steps, tasks)
	if len(plan) < 3 {
		t.Fatalf("workflow plan = %+v, want dynamic multi-step plan", plan)
	}
	if got := plan[0].Title; got != "收集约束" {
		t.Fatalf("first planner step title = %q, want 收集约束", got)
	}
	if got := plan[0].Description; !strings.Contains(got, "Agent role:") {
		t.Fatalf("planner step description = %q, want agent role projection", got)
	}
	firstStep := plan[0]
	if firstStep.Order != 1 || firstStep.PlanSource != workflowPlanSourcePlanner || firstStep.WorkflowMode != WorkModeTask || firstStep.AgentRole == "" || firstStep.PlannerStepID == "" {
		t.Fatalf("first planner step metadata = %+v, want ADK planner projection", firstStep)
	}
	firstTask, ok, err := runtime.Store().Task(ctx, firstStep.TaskID)
	if err != nil || !ok {
		t.Fatalf("first planner task lookup ok=%v err=%v", ok, err)
	}
	if firstTask.Order != firstStep.Order || firstTask.PlanSource != firstStep.PlanSource || firstTask.AgentRole != firstStep.AgentRole || firstTask.PlannerStepID != firstStep.PlannerStepID || firstTask.Objective == "" {
		t.Fatalf("planner task metadata = %+v, step=%+v, want mirrored metadata", firstTask, firstStep)
	}
	if len(plan) > 1 {
		secondTask, ok, err := runtime.Store().Task(ctx, plan[1].TaskID)
		if err != nil || !ok {
			t.Fatalf("second planner task lookup ok=%v err=%v", ok, err)
		}
		if len(secondTask.DependsOn) != 1 || secondTask.DependsOn[0] != firstTask.ID {
			t.Fatalf("second task dependsOn = %+v, want first task id %q", secondTask.DependsOn, firstTask.ID)
		}
	}
}

func TestCompileWorkflowPlanDraftValidationAndLimits(t *testing.T) {
	_, _, err := compileWorkflowPlanDraft(workflowPlanDraft{
		Steps: []workflowPlanDraftStep{{Title: "未完成", Message: "执行"}},
	}, WorkModeTask, "执行", "执行", RunOptions{})
	if err == nil {
		t.Fatal("unfinished planner draft unexpectedly compiled")
	}

	loopSteps, warnings, err := compileWorkflowPlanDraft(workflowPlanDraft{
		Finished: true,
		Steps: []workflowPlanDraftStep{
			{Title: "观察", Message: "观察"},
			{Title: "多余", Message: "多余"},
		},
	}, WorkModeLoop, "循环", "循环", RunOptions{})
	if err != nil {
		t.Fatalf("compile loop draft: %v", err)
	}
	if len(loopSteps) != 1 || len(warnings) == 0 {
		t.Fatalf("loop steps=%+v warnings=%+v, want first step warning", loopSteps, warnings)
	}
}

func TestCompileWorkflowPlanDraftOrdersAndMapsTaskDAG(t *testing.T) {
	steps, warnings, err := compileWorkflowPlanDraft(workflowPlanDraft{
		Finished: true,
		Steps: []workflowPlanDraftStep{
			{Order: 2, Title: "实现", Message: "实现功能", DependsOn: []string{"设计"}},
			{Order: 1, Title: "设计", Message: "设计方案"},
			{Order: 3, Title: "验证", Message: "验证结果", DependsOn: []string{"2"}},
		},
	}, WorkModeTask, "完成目标", "完成目标", RunOptions{})
	if err != nil {
		t.Fatalf("compile ordered DAG: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %+v, want none", warnings)
	}
	if got := []string{steps[0].Title, steps[1].Title, steps[2].Title}; strings.Join(got, ",") != "设计,实现,验证" {
		t.Fatalf("ordered titles = %+v, want 设计,实现,验证", got)
	}
	if steps[1].DependsOn[0] != steps[0].DependencyID {
		t.Fatalf("step 2 dependsOn = %+v, want %q", steps[1].DependsOn, steps[0].DependencyID)
	}
	if steps[2].DependsOn[0] != steps[1].DependencyID {
		t.Fatalf("step 3 dependsOn = %+v, want %q", steps[2].DependsOn, steps[1].DependencyID)
	}
}

func TestCompileWorkflowPlanDraftRejectsInvalidDAG(t *testing.T) {
	cases := []struct {
		name  string
		mode  string
		steps []workflowPlanDraftStep
	}{
		{
			name:  "task future dependency",
			mode:  WorkModeTask,
			steps: []workflowPlanDraftStep{{Order: 1, Title: "先做", Message: "先做", DependsOn: []string{"后做"}}, {Order: 2, Title: "后做", Message: "后做"}},
		},
		{
			name:  "task unknown dependency",
			mode:  WorkModeTask,
			steps: []workflowPlanDraftStep{{Order: 1, Title: "先做", Message: "先做"}, {Order: 2, Title: "后做", Message: "后做", DependsOn: []string{"不存在"}}},
		},
		{
			name:  "duplicate title dependency alias",
			mode:  WorkModeTask,
			steps: []workflowPlanDraftStep{{Order: 1, Title: "重复", Message: "一"}, {Order: 2, Title: "重复", Message: "二"}},
		},
		{
			name:  "duplicate order",
			mode:  WorkModeTask,
			steps: []workflowPlanDraftStep{{Order: 1, Title: "步骤一", Message: "一"}, {Order: 1, Title: "步骤二", Message: "二"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := compileWorkflowPlanDraft(workflowPlanDraft{Finished: true, Steps: tc.steps}, tc.mode, "目标", "目标", RunOptions{})
			if err == nil {
				t.Fatal("compile invalid DAG err = nil, want error")
			}
		})
	}
}

func TestRuntimeWorkflowTaskAddRules(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "runtime-task-agent", Name: "Runtime Task", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "runtime tasks")
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-runtime-tasks", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
		Objective: "动态任务", CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	base, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		ID: "runtime-base", Title: "基础任务", Message: "先做基础任务", Status: "IN_PROGRESS",
		AgentID: agent.ID, RunID: parent.ID, Order: 1, WorkflowMode: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("SaveTask base: %v", err)
	}
	parent.WorkflowPlan = workflowPlanFromTasks([]Task{base}, nil)
	executor := &WorkflowExecutor{runtime: runtime}

	added, err := executor.addRuntimeWorkflowTask(ctx, parent, base, workflowRuntimeTaskRequest{
		Title: "追加验证", Message: "验证新增条件",
	})
	if err != nil {
		t.Fatalf("addRuntimeWorkflowTask: %v", err)
	}
	if added.PlanSource != workflowPlanSourceRuntime || added.PlannerStepID != "runtime-1" || len(added.DependsOn) != 0 {
		t.Fatalf("runtime task = %+v, want runtime task without implicit dependency", added)
	}

	if _, err := executor.addRuntimeWorkflowTask(ctx, parent, base, workflowRuntimeTaskRequest{Title: "坏依赖", DependsOn: []string{"missing-task"}}); err == nil {
		t.Fatal("addRuntimeWorkflowTask unknown dependency err = nil, want error")
	}
	for index := 2; index <= maxRuntimeWorkflowTasks; index++ {
		parent.WorkflowPlan = workflowPlanFromTasks(mustWorkflowTasks(t, runtime, parent), parent.WorkflowPlan)
		if _, err := executor.addRuntimeWorkflowTask(ctx, parent, base, workflowRuntimeTaskRequest{Title: fmt.Sprintf("追加 %d", index)}); err != nil {
			t.Fatalf("addRuntimeWorkflowTask %d: %v", index, err)
		}
	}
	parent.WorkflowPlan = workflowPlanFromTasks(mustWorkflowTasks(t, runtime, parent), parent.WorkflowPlan)
	if _, err := executor.addRuntimeWorkflowTask(ctx, parent, base, workflowRuntimeTaskRequest{Title: "超过上限"}); err == nil {
		t.Fatal("addRuntimeWorkflowTask over limit err = nil, want error")
	}
}

func mustWorkflowTasks(t *testing.T, runtime *Runtime, parent Run) []Task {
	t.Helper()
	tasks, err := (&WorkflowExecutor{runtime: runtime}).workflowTasks(context.Background(), parent, nil)
	if err != nil {
		t.Fatalf("workflowTasks: %v", err)
	}
	return tasks
}

func TestWorkflowChildInstructionTaskIsSelfContained(t *testing.T) {
	task := workflowChildInstructionTask(workflowStep{
		Objective:    "完成 TME 策略",
		Message:      "验证收益与频次",
		Description:  "检查年化收益、交易频次和过拟合风险。",
		AgentRole:    "验证与风控子 Agent",
		DependencyID: "__planner_step_3",
	})
	for _, want := range []string{"总体目标：完成 TME 策略", "当前子任务：验证收益与频次", "子任务说明：检查年化收益、交易频次和过拟合风险。", "子 Agent 角色：验证与风控子 Agent", "不要假设自己能看到父对话"} {
		if !strings.Contains(task, want) {
			t.Fatalf("workflow child task = %q, want to contain %q", task, want)
		}
	}
	restored := workflowStepFromState(WorkflowStepState{
		Title:       "验证",
		Description: "检查年化收益。\n\nAgent role: 验证与风控子 Agent",
		Message:     "验证收益",
		AgentRole:   "验证与风控子 Agent",
	})
	restoredTask := workflowChildInstructionTask(restored)
	if strings.Count(restoredTask, "验证与风控子 Agent") != 1 {
		t.Fatalf("restored workflow child task = %q, want role once", restoredTask)
	}
}

func TestWorkflowFinalSynthesisCompletesToolOnlyChildRun(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-final-synth-agent", Name: "Workflow Final Synth", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "final synth")
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-final-synth", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask,
		WorkflowPlan: []WorkflowStepState{{
			TaskID: "step-final-synth", Title: "读取数据后总结", Status: "IN_PROGRESS",
		}},
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	child := mustSaveRun(t, runtime, Run{
		ID: "child-final-synth", SessionID: session.ID, AgentID: agent.ID,
		ParentRunID: parent.ID, Status: RunStatusRunning, UserMessage: "读取数据后总结",
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	steps := []workflowStep{{Title: "读取数据后总结", Message: "读取数据后总结"}}
	execution, err := runtime.newGoogleADKWorkflowExecution(ctx, agent, session, parent, []Run{child}, steps, WorkModeTask, RunOptions{}, nil)
	if err != nil {
		t.Fatalf("newGoogleADKWorkflowExecution: %v", err)
	}
	call := execution.ensureCallForRun("call-final-synth", ToolDescriptor{Name: "market.candles", Permission: "read"}, map[string]any{"symbol": "TME"}, child.ID)
	execution.finishCall(call.ID, map[string]any{"symbol": "TME", "close": 10.2}, nil)
	execution.consumeFunctionResponse(&genai.FunctionResponse{
		ID:       "call-final-synth",
		Name:     "market.candles",
		Response: map[string]any{"symbol": "TME", "close": 10.2},
	})

	if !execution.runNeedsFinalSynthesis(child.ID) {
		t.Fatal("child run does not need final synthesis before post-tool text")
	}
	executor := WorkflowExecutor{runtime: runtime}
	err = executor.ensureWorkflowChildrenFinalReplies(ctx, workflowRequest{
		Agent: agent, Session: session, Message: child.UserMessage,
	}, execution, []Run{child}, nil)
	if err != nil {
		t.Fatalf("ensureWorkflowChildrenFinalReplies: %v", err)
	}
	if execution.runNeedsFinalSynthesis(child.ID) || !execution.runHasPostToolText(child.ID) {
		t.Fatal("child run still lacks post-tool final text after synthesis")
	}
	if reply := execution.resultForRun(child.ID).Reply; strings.TrimSpace(reply) == "" || !strings.Contains(reply, "读取数据后总结") {
		t.Fatalf("synthesized reply = %q, want local final reply with child task", reply)
	}
}

func TestWorkflowFinalSynthesisSkipsChildrenWithPendingApproval(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "workflow-final-synth-approval-agent", Name: "Workflow Final Synth Approval", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "final synth approval")
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-final-synth-approval", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask,
		WorkflowPlan: []WorkflowStepState{{
			TaskID: "step-final-synth-approval", Title: "读取数据后审批", Status: "IN_PROGRESS",
		}},
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	child := mustSaveRun(t, runtime, Run{
		ID: "child-final-synth-approval", SessionID: session.ID, AgentID: agent.ID,
		ParentRunID: parent.ID, Status: RunStatusRunning, UserMessage: "读取数据后审批",
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	steps := []workflowStep{{Title: "读取数据后审批", Message: "读取数据后审批"}}
	execution, err := runtime.newGoogleADKWorkflowExecution(ctx, agent, session, parent, []Run{child}, steps, WorkModeTask, RunOptions{}, nil)
	if err != nil {
		t.Fatalf("newGoogleADKWorkflowExecution: %v", err)
	}
	call := execution.ensureCallForRun("call-final-synth-approval", ToolDescriptor{Name: "market.candles", Permission: "read"}, map[string]any{"symbol": "TME"}, child.ID)
	execution.finishCall(call.ID, map[string]any{"symbol": "TME", "close": 10.2}, nil)
	execution.consumeFunctionResponse(&genai.FunctionResponse{
		ID:       "call-final-synth-approval",
		Name:     "market.candles",
		Response: map[string]any{"symbol": "TME", "close": 10.2},
	})
	approval := Approval{
		ID: "approval-final-synth-child", RunID: child.ID, AgentID: agent.ID,
		ToolName: "strategy.save_draft", Status: ApprovalStatusPending,
		CreatedAt: nowString(), UpdatedAt: nowString(),
	}

	if !execution.runNeedsFinalSynthesis(child.ID) {
		t.Fatal("child run does not need final synthesis before approval")
	}
	executor := WorkflowExecutor{runtime: runtime}
	if err := executor.ensureWorkflowChildrenFinalReplies(ctx, workflowRequest{
		Agent: agent, Session: session, Message: child.UserMessage,
	}, execution, []Run{child}, []Approval{approval}); err != nil {
		t.Fatalf("ensureWorkflowChildrenFinalReplies: %v", err)
	}
	if !execution.runNeedsFinalSynthesis(child.ID) {
		t.Fatal("pending approval child unexpectedly synthesized a final reply")
	}
	responses, err := executor.completeWorkflowChildrenFromADK(ctx, workflowRequest{
		Agent: agent, Session: session, Message: child.UserMessage,
	}, execution, []Run{child}, []Approval{approval})
	if err != nil {
		t.Fatalf("completeWorkflowChildrenFromADK: %v", err)
	}
	if len(responses) != 1 || responses[0].Run.Status != RunStatusPending {
		t.Fatalf("child response = %+v, want pending approval", responses)
	}
}

func TestLoopWorkflowCanBeSelectedPerRun(t *testing.T) {
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "loop-agent", Name: "Loop", Status: AgentStatusEnabled, WorkMode: WorkModeChat,
	})
	response, err := runtime.Chat(context.Background(), ChatRequest{
		AgentID:          agent.ID,
		Message:          "完成一次目标推进",
		WorkModeOverride: WorkModeLoop,
		RunOptions:       &RunOptions{LoopMaxIterations: 3},
	})
	if err != nil {
		t.Fatalf("Chat loop workflow: %v", err)
	}
	if response.Run.WorkMode != WorkModeLoop || response.Run.Iteration != 1 || response.Run.Status != RunStatusCompleted {
		t.Fatalf("parent run = %+v, want completed loop workflow at iteration 1", response.Run)
	}
	if response.Run.Objective != "完成一次目标推进" {
		t.Fatalf("objective = %q", response.Run.Objective)
	}
	if len(response.Run.WorkflowPlan) != 1 || response.Run.WorkflowPlan[0].PlanSource != workflowPlanSourceRuntime {
		t.Fatalf("workflow plan = %+v, want runtime initial goal task", response.Run.WorkflowPlan)
	}
	for _, call := range response.Run.ToolCalls {
		if strings.HasPrefix(call.ToolName, "workflow.plan.") {
			t.Fatalf("tool calls = %+v, loop goal mode must not call planner tools", response.Run.ToolCalls)
		}
	}
	if !runHasToolCall(response.Run, workflowGoalCompleteTool) {
		t.Fatalf("tool calls = %+v, want goal completion decision", response.Run.ToolCalls)
	}
}

func TestGoalWorkflowRequiresDecisionTool(t *testing.T) {
	runtime := newTestRuntime(t)
	providerID := saveGoalWorkflowProvider(t, runtime, "goal-no-decision-provider", func(req openAIChatRequest) openAIChatMessage {
		if calls := testGoalWorkflowTaskProgressCalls(req); len(calls) > 0 {
			return openAIChatMessage{Role: "assistant", ToolCalls: calls}
		}
		return openAIChatMessage{Role: "assistant", Content: "目标推进了一步，但没有裁决。"}
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-no-decision-agent", Name: "Goal No Decision", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeChat,
	})
	response, err := runtime.Chat(context.Background(), ChatRequest{
		AgentID:          agent.ID,
		Message:          "推进一个必须裁决的目标",
		WorkModeOverride: WorkModeLoop,
		RunOptions:       &RunOptions{LoopMaxIterations: 2},
	})
	if err != nil {
		t.Fatalf("Chat goal workflow missing decision: %v", err)
	}
	if response.Run.Status != RunStatusFailed || response.Run.ErrorCode != workflowGoalDecisionErr {
		t.Fatalf("run = %+v, want failed %s", response.Run, workflowGoalDecisionErr)
	}
}

func TestGoalWorkflowContinueRespectsMaxIterations(t *testing.T) {
	runtime := newTestRuntime(t)
	providerID := saveGoalWorkflowProvider(t, runtime, "goal-continue-provider", func(req openAIChatRequest) openAIChatMessage {
		lastUser := testGoalWorkflowLastUserMessage(req)
		if strings.Contains(lastUser, "是否完成目标") && containsTool(testProviderToolNames(req), workflowGoalContinueTool) {
			seen := testGoalWorkflowToolResponsesSinceLastUser(req.Messages)
			if !seen[workflowGoalContinueTool] {
				return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
					testProviderToolCall("call-goal-continue", workflowGoalContinueTool, map[string]any{
						"reason": "还需要继续推进。",
					}),
				}}
			}
			return openAIChatMessage{Role: "assistant", Content: "继续推进中。"}
		}
		if calls := testGoalWorkflowTaskProgressCalls(req); len(calls) > 0 {
			return openAIChatMessage{Role: "assistant", ToolCalls: calls}
		}
		return openAIChatMessage{Role: "assistant", Content: "本轮推进完成，等待裁决。"}
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-continue-agent", Name: "Goal Continue", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeChat,
	})
	response, err := runtime.Chat(context.Background(), ChatRequest{
		AgentID:          agent.ID,
		Message:          "持续推进直到超过上限",
		WorkModeOverride: WorkModeLoop,
		RunOptions:       &RunOptions{LoopMaxIterations: 2},
	})
	if err != nil {
		t.Fatalf("Chat goal workflow continue: %v", err)
	}
	if response.Run.Status != RunStatusFailed || response.Run.ErrorCode != workflowGoalMaxLoopErr {
		t.Fatalf("run = %+v, want failed %s", response.Run, workflowGoalMaxLoopErr)
	}
}

func TestGoalWorkflowPauseAfterContinueAndResume(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	var mu sync.Mutex
	pauseRequested := false
	resumeNudgeSeen := false
	providerID := saveGoalWorkflowProvider(t, runtime, "goal-pause-resume-provider", func(req openAIChatRequest) openAIChatMessage {
		lastUser := testGoalWorkflowLastUserMessage(req)
		if strings.Contains(lastUser, "用户继续运行目标") {
			mu.Lock()
			resumeNudgeSeen = true
			mu.Unlock()
		}
		if strings.Contains(lastUser, "是否完成目标") && containsTool(testProviderToolNames(req), workflowGoalContinueTool) {
			seen := testGoalWorkflowToolResponsesSinceLastUser(req.Messages)
			if seen[workflowGoalContinueTool] || seen[workflowGoalCompleteTool] {
				return openAIChatMessage{Role: "assistant", Content: "目标裁决已记录。"}
			}
			mu.Lock()
			shouldPause := !pauseRequested
			if shouldPause {
				pauseRequested = true
			}
			shouldComplete := resumeNudgeSeen
			mu.Unlock()
			if shouldPause {
				runs, _ := runtime.Store().ListRuns(ctx)
				for _, run := range runs {
					if normalizeWorkMode(run.WorkMode) == WorkModeLoop && run.ParentRunID == "" && run.Status == RunStatusRunning {
						_, _ = runtime.PauseGoalRun(ctx, run.ID)
						break
					}
				}
				return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
					testProviderToolCall("call-goal-continue", workflowGoalContinueTool, map[string]any{
						"reason": "用户请求本轮后暂停。",
					}),
				}}
			}
			if shouldComplete {
				return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
					testProviderToolCall("call-goal-complete", workflowGoalCompleteTool, map[string]any{
						"summary": "恢复后目标完成。",
					}),
				}}
			}
			return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
				testProviderToolCall("call-goal-continue-after-resume", workflowGoalContinueTool, map[string]any{
					"reason": "继续推进。",
				}),
			}}
		}
		if calls := testGoalWorkflowTaskProgressCalls(req); len(calls) > 0 {
			return openAIChatMessage{Role: "assistant", ToolCalls: calls}
		}
		return openAIChatMessage{Role: "assistant", Content: "本轮推进完成。"}
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-pause-resume-agent", Name: "Goal Pause Resume", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeChat, LoopMaxIterations: 3,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "推进一个可暂停目标",
		WorkModeOverride: WorkModeLoop,
		RunOptions:       &RunOptions{LoopMaxIterations: 3},
	})
	if err != nil {
		t.Fatalf("Chat goal workflow pause: %v", err)
	}
	if response.Run.Status != RunStatusPaused || response.Run.WorkflowStatus != workflowStatusPaused || response.Run.ResumeState != "user_paused" {
		t.Fatalf("paused run = %+v, want user-paused goal", response.Run)
	}
	if response.Run.PauseRequestedAt == nil || response.Run.PausedAt == nil || response.Run.PausedReason != "user" {
		t.Fatalf("paused fields = pauseRequestedAt:%v pausedAt:%v pausedReason:%q", response.Run.PauseRequestedAt, response.Run.PausedAt, response.Run.PausedReason)
	}
	resumed, err := runtime.ResumeGoalRun(ctx, response.Run.ID)
	if err != nil {
		t.Fatalf("ResumeGoalRun: %v", err)
	}
	if resumed.Status != RunStatusRunning || resumed.PauseRequestedAt != nil || resumed.PausedAt != nil || resumed.PausedReason != "" {
		t.Fatalf("resumed run = %+v, want running with pause fields cleared", resumed)
	}
	completed := waitForRunStatus(t, runtime, response.Run.ID, RunStatusCompleted)
	if completed.Message != "goal completed" {
		t.Fatalf("completed run = %+v, want goal completed", completed)
	}
	mu.Lock()
	defer mu.Unlock()
	if !resumeNudgeSeen {
		t.Fatal("resume nudge was not sent to the existing goal run")
	}
}

func TestGoalWorkflowPauseRequestedBeforeCompleteDecisionPausesInsteadOfCompleting(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	var pauseRequested atomic.Bool
	providerID := saveGoalWorkflowProvider(t, runtime, "goal-pause-before-complete-provider", func(req openAIChatRequest) openAIChatMessage {
		lastUser := testGoalWorkflowLastUserMessage(req)
		if strings.Contains(lastUser, "是否完成目标") && containsTool(testProviderToolNames(req), workflowGoalCompleteTool) {
			seen := testGoalWorkflowToolResponsesSinceLastUser(req.Messages)
			if seen[workflowGoalCompleteTool] {
				return openAIChatMessage{Role: "assistant", Content: "goal.complete 已被中断。"}
			}
			if !pauseRequested.Swap(true) {
				runs, _ := runtime.Store().ListRuns(ctx)
				for _, run := range runs {
					if normalizeWorkMode(run.WorkMode) == WorkModeLoop && run.ParentRunID == "" && run.Status == RunStatusRunning {
						_, _ = runtime.PauseGoalRun(ctx, run.ID)
						break
					}
				}
			}
			return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
				testProviderToolCall("call-goal-complete-after-pause", workflowGoalCompleteTool, map[string]any{
					"summary": "这一轮已经完成。",
				}),
			}}
		}
		if calls := testGoalWorkflowTaskProgressCalls(req); len(calls) > 0 {
			return openAIChatMessage{Role: "assistant", ToolCalls: calls}
		}
		return openAIChatMessage{Role: "assistant", Content: "本轮推进完成。"}
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-pause-before-complete-agent", Name: "Goal Pause Before Complete", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeChat, LoopMaxIterations: 3,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "推进一个在完成前暂停的目标",
		WorkModeOverride: WorkModeLoop,
		RunOptions:       &RunOptions{LoopMaxIterations: 3},
	})
	if err != nil {
		t.Fatalf("Chat goal workflow pause before complete: %v", err)
	}
	if response.Run.Status != RunStatusPaused || response.Run.WorkflowStatus != workflowStatusPaused || response.Run.ResumeState != "user_paused" {
		t.Fatalf("paused run = %+v, want user-paused goal instead of completion", response.Run)
	}
	if response.Run.CompletedAt != nil {
		t.Fatalf("completedAt = %v, want nil for paused goal", response.Run.CompletedAt)
	}
	if response.Run.PauseRequestedAt == nil || response.Run.PausedAt == nil || response.Run.PausedReason != "user" {
		t.Fatalf("paused fields = pauseRequestedAt:%v pausedAt:%v pausedReason:%q", response.Run.PauseRequestedAt, response.Run.PausedAt, response.Run.PausedReason)
	}
	if runHasToolCall(response.Run, workflowGoalCompleteTool) {
		t.Fatalf("tool calls = %+v, want interrupted goal.complete pruned from paused snapshot", response.Run.ToolCalls)
	}
}

func TestGoalWorkflowPauseRequestBeforeNextTurnDoesNotCallModel(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	providerID := saveGoalWorkflowProvider(t, runtime, "goal-pause-before-next-turn-provider", func(req openAIChatRequest) openAIChatMessage {
		t.Fatalf("provider called after pause request: last user=%q", testGoalWorkflowLastUserMessage(req))
		return openAIChatMessage{}
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-pause-before-next-turn-agent", Name: "Goal Pause Before Next Turn", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeLoop, LoopMaxIterations: 3,
	})
	session := mustCreateSession(t, runtime, agent.ID, "pause before next turn")
	now := nowString()
	run := mustSaveRun(t, runtime, Run{
		ID: "run-goal-pause-before-next-turn", SessionID: session.ID, AgentID: agent.ID, ProviderID: providerID,
		Status: RunStatusRunning, Message: "goal continues", UserMessage: "继续目标", WorkMode: WorkModeLoop,
		Objective: "继续目标", Iteration: 1, WorkflowStatus: workflowStatusRunning,
		PauseRequestedAt: &now, CreatedAt: now, StartedAt: now, UpdatedAt: now,
		ToolCalls: []ToolCall{}, PendingApprovals: []Approval{}, Usage: &RunUsage{},
	})
	task, err := runtime.Store().SaveTask(ctx, TaskWriteRequest{
		Title: "继续目标", Status: "DONE", AgentID: agent.ID, RunID: run.ID,
		Order: 1, ModeHint: WorkModeTask, PlanSource: workflowPlanSourceRuntime, WorkflowMode: WorkModeLoop,
		Objective: run.Objective, Message: run.UserMessage,
	})
	if err != nil {
		t.Fatalf("SaveTask: %v", err)
	}
	run.WorkflowPlan = workflowPlanFromTasks([]Task{task}, run.WorkflowPlan)
	if err := runtime.Store().SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun with plan: %v", err)
	}
	response, err := (&WorkflowExecutor{runtime: runtime}).continueADKGoalWorkflow(ctx, workflowRequest{
		Agent: agent, Session: session, Message: run.UserMessage, Mode: WorkModeLoop, Objective: run.Objective,
		RunOptions: RunOptions{LoopMaxIterations: 3},
	}, run, []Task{task}, goalOrchestratorContinueNudge(run, "继续推进。"), 2, 3)
	if err != nil {
		t.Fatalf("continueADKGoalWorkflow: %v", err)
	}
	if response.Run.Status != RunStatusPaused || response.Run.ResumeState != "user_paused" || response.Run.PausedReason != "user" {
		t.Fatalf("run = %+v, want user-paused without another model call", response.Run)
	}
}

func TestGoalWorkflowPauseRequestBlocksChildCompletionContinuation(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-pause-child-agent", Name: "Goal Pause Child", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "pause child continuation")
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-goal-pause-child-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, Message: "等待子运行", UserMessage: "推进目标", WorkMode: WorkModeLoop,
		Objective: "推进目标", Iteration: 1, WorkflowStatus: workflowStatusRunning,
		PauseRequestedAt: &now, ChildRunIDs: []string{"run-goal-pause-child"},
		WorkflowPlan: []WorkflowStepState{{
			Title: "子步骤", Message: "执行子步骤", Status: "TODO", ChildRunID: "run-goal-pause-child",
		}},
		CreatedAt: now, StartedAt: now, UpdatedAt: now, ToolCalls: []ToolCall{}, PendingApprovals: []Approval{}, Usage: &RunUsage{},
	})
	child := mustSaveRun(t, runtime, Run{
		ID: "run-goal-pause-child", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
		Status: RunStatusRunning, Message: "子运行仍在执行", UserMessage: "执行子步骤",
		CreatedAt: now, StartedAt: now, UpdatedAt: now, ToolCalls: []ToolCall{}, PendingApprovals: []Approval{}, Usage: &RunUsage{},
	})
	runningParent, err := runtime.continueParentWorkflowAfterChild(ctx, child)
	if err != nil {
		t.Fatalf("continueParentWorkflowAfterChild running child: %v", err)
	}
	if runningParent == nil || runningParent.Status != RunStatusRunning || runningParent.PauseRequestedAt == nil {
		t.Fatalf("running child parent = %+v, want still running with pause requested", runningParent)
	}

	completedAt := nowString()
	child.Status = RunStatusCompleted
	child.Message = "子运行完成"
	child.CompletedAt = &completedAt
	child.UpdatedAt = completedAt
	if err := runtime.Store().SaveRun(ctx, child); err != nil {
		t.Fatalf("Save child completed: %v", err)
	}
	pausedParent, err := runtime.continueParentWorkflowAfterChild(ctx, child)
	if err != nil {
		t.Fatalf("continueParentWorkflowAfterChild completed child: %v", err)
	}
	if pausedParent == nil || pausedParent.Status != RunStatusPaused || pausedParent.WorkflowStatus != workflowStatusPaused || pausedParent.ResumeState != "user_paused" {
		t.Fatalf("completed child parent = %+v, want user-paused parent", pausedParent)
	}
	if pausedParent.CompletedAt != nil {
		t.Fatalf("parent completedAt = %v, want nil while user-paused", *pausedParent.CompletedAt)
	}
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("stored parent lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusPaused || stored.PausedReason != "user" {
		t.Fatalf("stored parent = %+v, want user-paused", stored)
	}
}

func TestGoalWorkflowActivitySnapshotDoesNotDowngradeUserPausedParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-paused-snapshot-agent", Name: "Goal Paused Snapshot", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "paused snapshot")
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-goal-paused-snapshot-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusPaused, Message: "目标已暂停。", UserMessage: "推进目标", WorkMode: WorkModeLoop,
		Objective: "推进目标", Iteration: 1, WorkflowStatus: workflowStatusPaused,
		PauseRequestedAt: &now, PausedAt: &now, PausedReason: "user", ResumeState: "user_paused",
		WorkflowPlan: []WorkflowStepState{{
			Title: "已暂停步骤", Message: "等待继续", Status: "TODO",
		}},
		CreatedAt: now, StartedAt: now, UpdatedAt: now, ToolCalls: []ToolCall{}, PendingApprovals: []Approval{}, Usage: &RunUsage{},
	})
	snapshot := parent
	snapshot.Status = RunStatusRunning
	snapshot.WorkflowStatus = workflowStatusRunning
	snapshot.Message = "goal running"
	snapshot.PauseRequestedAt = nil
	snapshot.PausedAt = nil
	snapshot.PausedReason = ""
	snapshot.ResumeState = ""
	snapshot.ToolCalls = []ToolCall{{
		ID: "tool-paused-stale", RunID: parent.ID, ToolName: workflowGoalContinueTool, Status: "RUNNING",
		CreatedAt: now, UpdatedAt: now,
	}}

	if _, err := runtime.persistRunActivitySnapshot(ctx, snapshot); err != nil {
		t.Fatalf("persistRunActivitySnapshot: %v", err)
	}
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("stored parent lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusPaused || stored.WorkflowStatus != workflowStatusPaused || stored.PausedReason != "user" || stored.ResumeState != "user_paused" {
		t.Fatalf("stored parent = %+v, want lifecycle to remain user-paused", stored)
	}
	if len(stored.ToolCalls) != 1 || stored.ToolCalls[0].ID != "tool-paused-stale" {
		t.Fatalf("stored tool calls = %+v, want activity snapshot merged", stored.ToolCalls)
	}
}

func TestGoalWorkflowDecisionPromptUsesUpdatedObjective(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	var mu sync.Mutex
	var decisionPrompts []string
	updated := false
	providerID := saveGoalWorkflowProvider(t, runtime, "goal-updated-objective-provider", func(req openAIChatRequest) openAIChatMessage {
		lastUser := testGoalWorkflowLastUserMessage(req)
		if strings.Contains(lastUser, "是否完成目标") {
			seen := testGoalWorkflowToolResponsesSinceLastUser(req.Messages)
			if seen[workflowGoalCompleteTool] {
				return openAIChatMessage{Role: "assistant", Content: "目标已按新描述完成。"}
			}
			mu.Lock()
			decisionPrompts = append(decisionPrompts, lastUser)
			mu.Unlock()
			return openAIChatMessage{Role: "assistant", ToolCalls: []openAIToolCall{
				testProviderToolCall("call-goal-complete", workflowGoalCompleteTool, map[string]any{
					"summary": "目标已按新描述完成。",
				}),
			}}
		}
		if calls := testGoalWorkflowTaskProgressCalls(req); len(calls) > 0 {
			return openAIChatMessage{Role: "assistant", ToolCalls: calls}
		}
		if !updated && testProviderToolResponseNames(req.Messages)[workflowTaskCompleteTool] {
			updated = true
			runs, err := runtime.Store().ListRuns(ctx)
			if err != nil {
				return openAIChatMessage{Role: "assistant", Content: "读取 run 失败。"}
			}
			for _, run := range runs {
				if normalizeWorkMode(run.WorkMode) == WorkModeLoop && run.ParentRunID == "" && run.Status == RunStatusRunning {
					_, _ = runtime.UpdateRunObjective(ctx, run.ID, "更新后的目标")
					break
				}
			}
		}
		return openAIChatMessage{Role: "assistant", Content: "已根据当前目标推进。"}
	})
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-updated-objective-agent", Name: "Goal Updated Objective", ProviderID: providerID,
		Status: AgentStatusEnabled, WorkMode: WorkModeChat,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "旧目标",
		Objective:        "旧目标",
		WorkModeOverride: WorkModeLoop,
		RunOptions:       &RunOptions{LoopMaxIterations: 2},
	})
	if err != nil {
		t.Fatalf("Chat goal workflow updated objective: %v", err)
	}
	if response.Run.Status != RunStatusCompleted {
		t.Fatalf("run = %+v, want completed", response.Run)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(decisionPrompts) == 0 {
		t.Fatal("decision prompts empty")
	}
	if !strings.Contains(decisionPrompts[0], "更新后的目标") {
		t.Fatalf("decision prompt = %q, want updated objective", decisionPrompts[0])
	}
}

func TestWorkflowResponseUsesAuthoritativePauseRequestedParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-response-pause-agent", Name: "Goal Response Pause", Status: AgentStatusEnabled,
		WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "pause response")
	now := nowString()
	parent := mustSaveRun(t, runtime, Run{
		ID: "run-goal-response-pause-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, Message: "目标将在当前轮结束后暂停。", UserMessage: "推进目标", WorkMode: WorkModeLoop,
		Objective: "推进目标", WorkflowStatus: workflowStatusRunning, PauseRequestedAt: &now, ResumeState: "user_pause_requested",
		CreatedAt: now, StartedAt: now, UpdatedAt: now, ToolCalls: []ToolCall{}, PendingApprovals: []Approval{}, Usage: &RunUsage{},
	})
	stale := parent
	stale.Message = "goal running"
	stale.PauseRequestedAt = nil
	stale.ResumeState = ""

	response := (&WorkflowExecutor{runtime: runtime}).workflowResponse(
		ctx,
		session,
		stale,
		openAIChatResult{Reply: "still running"},
	)

	if response.Run.PauseRequestedAt == nil || response.Run.ResumeState != "user_pause_requested" {
		t.Fatalf("response run = %+v, want authoritative pause request fields", response.Run)
	}
}

func TestUpdateRunObjectiveOnlyAllowsActiveGoalParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "goal-objective-agent", Name: "Goal Objective", Status: AgentStatusEnabled, WorkMode: WorkModeLoop,
	})
	session := mustCreateSession(t, runtime, agent.ID, "goal objective")
	active := mustSaveRun(t, runtime, Run{
		ID: "run-goal-active", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, Objective: "旧目标",
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	updated, err := runtime.UpdateRunObjective(ctx, active.ID, " 新目标 ")
	if err != nil {
		t.Fatalf("UpdateRunObjective active goal: %v", err)
	}
	if updated.Objective != "新目标" {
		t.Fatalf("objective = %q, want updated", updated.Objective)
	}
	if _, err := runtime.UpdateRunObjective(ctx, active.ID, " "); err == nil {
		t.Fatal("UpdateRunObjective empty err = nil, want error")
	}
	chat := mustSaveRun(t, runtime, Run{
		ID: "run-chat-active", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeChat, Objective: "chat",
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.UpdateRunObjective(ctx, chat.ID, "新目标"); err == nil {
		t.Fatal("UpdateRunObjective chat run err = nil, want error")
	}
	child := mustSaveRun(t, runtime, Run{
		ID: "run-goal-child", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, Objective: "child", ParentRunID: active.ID,
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.UpdateRunObjective(ctx, child.ID, "新目标"); err == nil {
		t.Fatal("UpdateRunObjective child run err = nil, want error")
	}
	completed := mustSaveRun(t, runtime, Run{
		ID: "run-goal-complete", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusCompleted, WorkMode: WorkModeLoop, Objective: "done",
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	if _, err := runtime.UpdateRunObjective(ctx, completed.ID, "新目标"); err == nil {
		t.Fatal("UpdateRunObjective terminal run err = nil, want error")
	}
}

func TestTaskWorkflowApprovalContinuesParentWorkflow(t *testing.T) {
	ctx := context.Background()
	runtime, executions := newWorkflowApprovalRuntime(t, WorkModeTask)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-approval-agent", Name: "Task Approval", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask, Tools: []string{"strategy.save_draft"}, PermissionMode: PermissionModeApproval,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体并 @strategy.save_draft 保存策略",
		Objective:        "完成审批续跑测试",
		WorkModeOverride: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("Chat task approval workflow: %v", err)
	}
	if response.Run.Status != RunStatusPending || response.Run.WorkflowStatus != workflowStatusPaused {
		t.Fatalf("parent run = %+v, want paused pending workflow", response.Run)
	}
	if len(response.PendingApprovals) != 1 || response.PendingApprovals[0].RunID == response.Run.ID {
		t.Fatalf("pending approvals = %+v, want child-run approval", response.PendingApprovals)
	}

	resolution, err := runtime.ResolveApproval(ctx, response.PendingApprovals[0].ID, true)
	if err != nil {
		t.Fatalf("ResolveApproval: %v", err)
	}
	if resolution.Run == nil || resolution.Run.ParentRunID != response.Run.ID || resolution.Run.Status != RunStatusCompleted {
		t.Fatalf("child resolution run = %+v, want completed child", resolution.Run)
	}
	if resolution.ParentRun == nil || resolution.ParentRun.ID != response.Run.ID || resolution.ParentRun.Status != RunStatusCompleted {
		t.Fatalf("parent resolution run = %+v, want completed parent workflow", resolution.ParentRun)
	}
	if len(resolution.ParentRun.ChildRunIDs) != 1 {
		t.Fatalf("child run ids = %+v, want approved child", resolution.ParentRun.ChildRunIDs)
	}
	if executions.Load() != 1 {
		t.Fatalf("tool executions = %d, want 1", executions.Load())
	}
}

func TestTaskWorkflowApprovalDeniedTerminatesParentWorkflow(t *testing.T) {
	ctx := context.Background()
	runtime, _ := newWorkflowApprovalRuntime(t, WorkModeTask)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-deny-agent", Name: "Task Deny", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask, Tools: []string{"strategy.save_draft"}, PermissionMode: PermissionModeApproval,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体并 @strategy.save_draft 保存策略",
		WorkModeOverride: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("Chat task denial workflow: %v", err)
	}
	resolution, err := runtime.ResolveApproval(ctx, response.PendingApprovals[0].ID, false)
	if err != nil {
		t.Fatalf("ResolveApproval deny: %v", err)
	}
	if resolution.ParentRun == nil || resolution.ParentRun.Status != RunStatusDenied || resolution.ParentRun.WorkflowStatus != workflowStatusFailed {
		t.Fatalf("parent resolution run = %+v, want denied failed workflow", resolution.ParentRun)
	}
}

func TestTaskResumeUsesStoredPendingChildBeforeCompletingParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-stale-child-agent", Name: "Task Stale Child", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "stale child")
	approval := Approval{
		ID: "approval-stale-child", RunID: "child-stale-pending", AgentID: agent.ID,
		ToolName: "strategy.save_draft", Status: ApprovalStatusPending,
		CreatedAt: nowString(), UpdatedAt: nowString(),
	}
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-stale-plan", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
		Objective: "等待子审批", ChildRunIDs: []string{"child-stale-pending"},
		WorkflowPlan: []WorkflowStepState{{
			Title: "需要审批的步骤", Message: "保存策略", Status: "DONE", ChildRunID: "child-stale-pending",
		}},
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	mustSaveRun(t, runtime, Run{
		ID: "child-stale-pending", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
		Status: RunStatusPending, Message: "等待用户审批后继续执行。", UserMessage: "保存策略",
		PendingApprovals: []Approval{approval},
		CreatedAt:        nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	if err := runtime.Store().SaveApproval(ctx, approval); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).reconcileWorkflowChildren(ctx, parent)
	if err != nil {
		t.Fatalf("reconcileWorkflowChildren: %v", err)
	}
	if !blocked {
		t.Fatal("reconcileWorkflowChildren blocked = false, want true")
	}
	if updated.Status != RunStatusPending || updated.WorkflowStatus != workflowStatusPaused {
		t.Fatalf("parent run = %+v, want paused pending workflow", updated)
	}
	if len(updated.PendingApprovals) != 1 || updated.PendingApprovals[0].ID != approval.ID {
		t.Fatalf("parent pending approvals = %+v, want child approval", updated.PendingApprovals)
	}
	if got := updated.WorkflowPlan[0].Status; got != "BLOCKED" {
		t.Fatalf("workflow step status = %q, want BLOCKED", got)
	}
	if updated.CompletedAt != nil {
		t.Fatalf("parent completed at = %v, want nil while child waits approval", *updated.CompletedAt)
	}
	stored, ok, err := runtime.Store().Run(ctx, parent.ID)
	if err != nil || !ok {
		t.Fatalf("stored parent lookup ok=%v err=%v", ok, err)
	}
	if stored.Status != RunStatusPending || stored.WorkflowStatus != workflowStatusPaused {
		t.Fatalf("stored parent = %+v, want paused pending workflow", stored)
	}
}

func TestTaskResumeUsesStoredRunningChildBeforeCompletingParent(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-running-child-agent", Name: "Task Running Child", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask,
	})
	session := mustCreateSession(t, runtime, agent.ID, "running child")
	parent := mustSaveRun(t, runtime, Run{
		ID: "parent-running-plan", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
		Objective: "等待子运行", ChildRunIDs: []string{"child-still-running"},
		WorkflowPlan: []WorkflowStepState{{
			Title: "仍在运行的步骤", Message: "继续运行", Status: "DONE", ChildRunID: "child-still-running",
		}},
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	mustSaveRun(t, runtime, Run{
		ID: "child-still-running", SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
		Status: RunStatusRunning, Message: "子运行仍在执行。", UserMessage: "继续运行",
		CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})

	updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).reconcileWorkflowChildren(ctx, parent)
	if err != nil {
		t.Fatalf("reconcileWorkflowChildren: %v", err)
	}
	if !blocked {
		t.Fatal("reconcileWorkflowChildren blocked = false, want true")
	}
	if updated.Status != RunStatusRunning || updated.WorkflowStatus != workflowStatusRunning {
		t.Fatalf("parent run = %+v, want running workflow", updated)
	}
	if got := updated.WorkflowPlan[0].Status; got != "IN_PROGRESS" {
		t.Fatalf("workflow step status = %q, want IN_PROGRESS", got)
	}
	if updated.CompletedAt != nil {
		t.Fatalf("parent completed at = %v, want nil while child is running", *updated.CompletedAt)
	}
}

func TestTaskResumeTerminatesParentForStoredTerminalChild(t *testing.T) {
	cases := []struct {
		name   string
		status string
	}{
		{name: "failed", status: RunStatusFailed},
		{name: "denied", status: RunStatusDenied},
		{name: "cancelled", status: RunStatusCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			runtime := newTestRuntime(t)
			agent := mustSaveAgent(t, runtime, AgentWriteRequest{
				ID: "seq-terminal-child-agent-" + tc.name, Name: "Task Terminal Child", Status: AgentStatusEnabled,
				WorkMode: WorkModeTask,
			})
			session := mustCreateSession(t, runtime, agent.ID, "terminal child "+tc.name)
			childID := "child-terminal-" + tc.name
			parent := mustSaveRun(t, runtime, Run{
				ID: "parent-terminal-plan-" + tc.name, SessionID: session.ID, AgentID: agent.ID,
				Status: RunStatusRunning, WorkMode: WorkModeTask, WorkflowStatus: workflowStatusRunning,
				Objective: "处理终止子运行", ChildRunIDs: []string{childID},
				WorkflowPlan: []WorkflowStepState{{
					Title: "终止步骤", Message: "终止", Status: "DONE", ChildRunID: childID,
				}},
				CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
			})
			mustSaveRun(t, runtime, Run{
				ID: childID, SessionID: session.ID, AgentID: agent.ID, ParentRunID: parent.ID,
				Status: tc.status, Message: "child terminal", FailureReason: "child terminal failure",
				CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
			})

			updated, blocked, err := (&WorkflowExecutor{runtime: runtime}).reconcileWorkflowChildren(ctx, parent)
			if err != nil {
				t.Fatalf("reconcileWorkflowChildren: %v", err)
			}
			if !blocked {
				t.Fatal("reconcileWorkflowChildren blocked = false, want true")
			}
			if updated.Status != tc.status || updated.WorkflowStatus != workflowStatusFailed {
				t.Fatalf("parent run = %+v, want status %q failed workflow", updated, tc.status)
			}
			if updated.CompletedAt == nil {
				t.Fatal("parent completed at is nil, want terminal timestamp")
			}
			if got := updated.WorkflowPlan[0].Status; got != "BLOCKED" {
				t.Fatalf("workflow step status = %q, want BLOCKED", got)
			}
		})
	}
}

func TestWorkflowParentReconcilesResolvedChildApproval(t *testing.T) {
	ctx := context.Background()
	runtime, executions := newWorkflowApprovalRuntime(t, WorkModeTask)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "seq-reconcile-agent", Name: "Task Reconcile", Status: AgentStatusEnabled,
		WorkMode: WorkModeTask, Tools: []string{"strategy.save_draft"}, PermissionMode: PermissionModeApproval,
	})
	response, err := runtime.Chat(ctx, ChatRequest{
		AgentID:          agent.ID,
		Message:          "请创建子智能体并 @strategy.save_draft 保存策略",
		Objective:        "完成审批恢复测试",
		WorkModeOverride: WorkModeTask,
	})
	if err != nil {
		t.Fatalf("Chat task approval workflow: %v", err)
	}
	if _, changed, err := runtime.Store().ResolvePendingApproval(ctx, response.PendingApprovals[0].ID, ApprovalStatusApproved); err != nil || !changed {
		t.Fatalf("ResolvePendingApproval changed=%v err=%v", changed, err)
	}
	runtime.ReconcileResolvedApprovals(ctx)
	parent := waitForRunStatus(t, runtime, response.Run.ID, RunStatusCompleted)
	if parent.WorkflowStatus != workflowStatusComplete {
		t.Fatalf("parent workflow status = %q, want %q", parent.WorkflowStatus, workflowStatusComplete)
	}
	if executions.Load() != 1 {
		t.Fatalf("tool executions = %d, want 1", executions.Load())
	}
}

func newWorkflowApprovalRuntime(t *testing.T, mode string) (*Runtime, *atomic.Int32) {
	t.Helper()
	base := newTestRuntime(t)
	executions := &atomic.Int32{}
	registry := NewToolRegistry()
	registry.Register(ToolDescriptor{
		Name: "strategy.save_draft", Permission: "write_strategy",
		AllowedModes: []string{PermissionModeApproval},
	}, func(context.Context, map[string]any) (any, error) {
		executions.Add(1)
		return map[string]any{"saved": true, "mode": mode}, nil
	})
	return newRuntimeWithRegistry(t, base.Store(), registry), executions
}

func waitForRunStatus(t *testing.T, runtime *Runtime, runID string, status string) Run {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		run, ok, err := runtime.Store().Run(context.Background(), runID)
		if err != nil || !ok {
			t.Fatalf("Run lookup err=%v ok=%v", err, ok)
		}
		if run.Status == status {
			return run
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %s status = %q, want %q; run=%+v", runID, run.Status, status, run)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func runHasToolCall(run Run, toolName string) bool {
	for _, call := range run.ToolCalls {
		if call.ToolName == toolName {
			return true
		}
	}
	return false
}

func saveGoalWorkflowProvider(t *testing.T, runtime *Runtime, id string, message func(openAIChatRequest) openAIChatMessage) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		defer r.Body.Close()
		var req openAIChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(openAIChatResponse{
			Choices: []struct {
				Message openAIChatMessage `json:"message"`
			}{{Message: message(req)}},
		})
	}))
	t.Cleanup(server.Close)
	provider := mustSaveProvider(t, runtime, ProviderWriteRequest{
		ID: id, DisplayName: id, BaseURL: server.URL, Model: "test-model", APIKey: "sk-test", Enabled: true,
	})
	return provider.ID
}

func testGoalWorkflowTaskProgressCalls(req openAIChatRequest) []openAIToolCall {
	seen := testProviderToolResponseNames(req.Messages)
	tools := testProviderToolNames(req)
	if containsTool(tools, workflowTasksListTool) && !seen[workflowTasksListTool] {
		return []openAIToolCall{testProviderToolCall("call-task-list", workflowTasksListTool, map[string]any{})}
	}
	if containsTool(tools, workflowTaskCompleteTool) && !seen[workflowTaskCompleteTool] {
		text := strings.ToLower(testProviderConversationText(req.Messages))
		return []openAIToolCall{testProviderToolCall("call-task-complete", workflowTaskCompleteTool, map[string]any{
			"taskId": testProviderTaskIDFromText(text), "resultSummary": "已完成一次目标推进。",
		})}
	}
	return nil
}

func testGoalWorkflowLastUserMessage(req openAIChatRequest) string {
	for index := len(req.Messages) - 1; index >= 0; index-- {
		if req.Messages[index].Role == "user" {
			return req.Messages[index].Content
		}
	}
	return ""
}

func testGoalWorkflowToolResponsesSinceLastUser(messages []openAIChatMessage) map[string]bool {
	names := map[string]bool{}
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role == "user" {
			break
		}
		if message.Role != "tool" {
			continue
		}
		name := restoreToolNameFromOpenAI(message.Name)
		if name == "" {
			name = message.Name
		}
		names[name] = true
	}
	return names
}
