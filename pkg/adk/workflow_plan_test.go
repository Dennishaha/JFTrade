package adk

import (
	"context"
	"strings"
	"testing"
)

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
	if firstTask.Order != firstStep.Order || firstTask.PlanSource != firstStep.PlanSource || firstTask.AgentRole != firstStep.AgentRole || firstTask.PlannerStepID != firstStep.PlannerStepID || firstTask.Objective != "" || firstStep.Objective != "" {
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

func TestWorkflowPlanDoesNotCopyOriginalUserRequest(t *testing.T) {
	ctx := context.Background()
	runtime := newTestRuntime(t)
	agent := mustSaveAgent(t, runtime, AgentWriteRequest{
		ID: "plan-redaction-agent", Name: "Plan Redaction", Status: AgentStatusEnabled,
	})
	session := mustCreateSession(t, runtime, agent.ID, "plan redaction")
	original := "请完整照抄这一条用户请求作为计划"
	parent := mustSaveRun(t, runtime, Run{
		ID: "plan-redaction-parent", SessionID: session.ID, AgentID: agent.ID,
		Status: RunStatusRunning, WorkMode: WorkModeLoop, WorkflowStatus: workflowStatusRunning,
		UserMessage: original, Objective: original, CreatedAt: nowString(), UpdatedAt: nowString(), Usage: &RunUsage{},
	})
	executor := &WorkflowExecutor{runtime: runtime}
	initial, err := executor.createInitialGoalTask(ctx, parent, agent, original, original)
	if err != nil {
		t.Fatalf("createInitialGoalTask: %v", err)
	}
	if initial.Title == original || initial.Message == original || initial.Objective != "" {
		t.Fatalf("initial goal task copied original request: %+v", initial)
	}
	if parent.UserMessage != original || parent.Objective != original {
		t.Fatalf("root run lost original goal: %+v", parent)
	}

	steps, _, err := compileWorkflowPlanDraft(workflowPlanDraft{
		Finished: true,
		Steps: []workflowPlanDraftStep{{
			Order: 1, Title: original, Description: original, Message: original,
		}},
	}, WorkModeTask, original, original, RunOptions{})
	if err != nil {
		t.Fatalf("compileWorkflowPlanDraft: %v", err)
	}
	steps = applyWorkflowStepPlanningMetadata(steps, WorkModeTask, original, nil)
	if len(steps) != 1 {
		t.Fatalf("steps = %+v, want one", steps)
	}
	if steps[0].Title == original || steps[0].Description == original || steps[0].Message == original || steps[0].Objective != "" {
		t.Fatalf("planned step copied original request: %+v", steps[0])
	}
	tasks, err := executor.persistWorkflowTasks(ctx, parent, agent, steps)
	if err != nil {
		t.Fatalf("persistWorkflowTasks: %v", err)
	}
	plan := workflowPlanFromSteps(steps, tasks)
	if len(plan) != 1 || plan[0].Title == original || plan[0].Description == original || plan[0].Message == original || plan[0].Objective != "" {
		t.Fatalf("persisted workflow plan copied original request: %+v", plan)
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

func TestCompileWorkflowPlanDraftNormalizesDuplicateOrders(t *testing.T) {
	steps, warnings, err := compileWorkflowPlanDraft(workflowPlanDraft{
		Finished: true,
		Steps: []workflowPlanDraftStep{
			{Order: 1, Title: "步骤一", Message: "一"},
			{Order: 1, Title: "步骤二", Message: "二"},
			{Order: 3, Title: "步骤三", Message: "三"},
		},
	}, WorkModeTask, "目标", "目标", RunOptions{})
	if err != nil {
		t.Fatalf("compile duplicate order draft: %v", err)
	}
	if len(warnings) != 1 || warnings[0] != "planner step orders were duplicated and normalized" {
		t.Fatalf("warnings = %+v, want duplicate order normalization warning", warnings)
	}
	if got := []int{steps[0].Order, steps[1].Order, steps[2].Order}; got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Fatalf("orders = %+v, want normalized 1,2,3", got)
	}
	if got := []string{steps[0].Title, steps[1].Title, steps[2].Title}; strings.Join(got, ",") != "步骤一,步骤二,步骤三" {
		t.Fatalf("titles = %+v, want stable order", got)
	}
	if steps[1].DependsOn[0] != steps[0].DependencyID || steps[2].DependsOn[0] != steps[1].DependencyID {
		t.Fatalf("dependsOn = %+v / %+v, want normalized sequential dependencies", steps[1].DependsOn, steps[2].DependsOn)
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
