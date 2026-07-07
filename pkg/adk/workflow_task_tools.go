package adk

import (
	"context"
	"fmt"
	"strings"
	"sync"

	adkagent "google.golang.org/adk/v2/agent"
	adktool "google.golang.org/adk/v2/tool"
)

type workflowTaskToolset struct {
	mu            sync.Mutex
	executor      *WorkflowExecutor
	req           workflowRequest
	parentID      string
	currentTaskID string
}

func (t *workflowTaskToolset) Name() string { return "jftrade-workflow-task-tools" }

func (t *workflowTaskToolset) Tools(adkagent.ReadonlyContext) ([]adktool.Tool, error) {
	if normalizeWorkMode(t.req.Mode) == WorkModeLoop && t.req.GoalDecision != nil && t.req.GoalDecision.decisionPhase() {
		return newWorkflowMapFunctionTools(
			workflowMapToolSpec{name: workflowGoalCompleteTool, description: "Declare that the current objective is complete and finish the goal loop.", schema: workflowGoalCompleteSchema(), run: t.goalComplete},
			workflowMapToolSpec{name: workflowGoalContinueTool, description: "Declare that the current objective is not complete yet and continue orchestration.", schema: workflowGoalContinueSchema(), run: t.goalContinue},
		)
	}
	modelsListTool, err := t.modelsListTool()
	if err != nil {
		return nil, err
	}
	tools, err := newWorkflowMapFunctionTools(
		workflowMapToolSpec{name: workflowTasksListTool, description: "List current workflow TODO DAG, ready tasks, completed results and blocked state.", schema: emptyObjectSchema(), run: t.list},
		workflowMapToolSpec{name: workflowTaskAddTool, description: "Add a runtime TODO to the current ADK task workflow.", schema: workflowTaskAddSchema(), run: t.add},
		workflowMapToolSpec{name: workflowTaskClaimTool, description: "Claim a ready TODO for the orchestrator itself or a child agent.", schema: workflowTaskClaimSchema(), run: t.claim},
		workflowMapToolSpec{name: workflowTaskCompleteTool, description: "Mark a claimed or ready TODO as DONE with a result summary.", schema: workflowTaskCompleteSchema(), run: t.complete},
		workflowMapToolSpec{name: workflowTaskBlockTool, description: "Mark a TODO as BLOCKED with a blocking reason.", schema: workflowTaskBlockSchema(), run: t.block},
		workflowMapToolSpec{name: workflowTaskDelegateTool, description: "Delegate a ready TODO to an ADK child agent. This creates a JFTrade child run only when called.", schema: workflowTaskDelegateSchema(), run: t.delegate},
	)
	if err != nil {
		return nil, err
	}
	return append(tools, modelsListTool), nil
}

func (t *workflowTaskToolset) modelsListTool() (adktool.Tool, error) {
	return newWorkflowMapFunctionTool(workflowMapToolSpec{
		name:        workflowModelsListTool,
		description: "List callable ADK models that can be selected for delegated child agents.",
		schema:      workflowModelsListSchema(),
		run: func(args map[string]any) (map[string]any, error) {
			return t.modelsList(args)
		},
	})
}

func (t *workflowTaskToolset) modelsList(args map[string]any) (map[string]any, error) {
	if t == nil || t.executor == nil || t.executor.runtime == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	output, err := t.executor.runtime.modelsListTool(context.Background(), args)
	if err != nil {
		return nil, err
	}
	mapped, ok := output.(map[string]any)
	if !ok {
		return map[string]any{"result": output}, nil
	}
	return mapped, nil
}

func (t *workflowTaskToolset) list(map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	jftradeErr15 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr15)
	return map[string]any{"success": true, "tasks": taskToolTaskSummaries(tasks), "readyTasks": taskToolTaskSummaries(executableWorkflowTasks(tasks, parent.WorkMode))}, nil
}

func (t *workflowTaskToolset) add(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, _, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	current, _ := t.taskByID(context.Background(), t.currentTaskID)
	task, err := t.executor.addRuntimeWorkflowTask(context.Background(), parent, current, workflowRuntimeTaskRequest{
		Title: plannerStringArg(args, "title"), Message: plannerStringArg(args, "message"), Description: plannerStringArg(args, "description"),
		DependsOn: plannerStringSliceArg(args, "dependsOn"), AgentRole: plannerStringArg(args, "agentRole"), ModeHint: plannerStringArg(args, "modeHint"),
		ChildProviderID: plannerStringArg(args, "childProviderId"), ChildModel: plannerStringArg(args, "childModel"),
	})
	if err != nil {
		return nil, err
	}
	parent, tasks, jftradeErr22 := t.parentAndTasks(context.Background())
	jftradeLogError(jftradeErr22)
	jftradeErr13 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr13)
	return map[string]any{"success": true, "task": taskToolTaskSummary(task)}, nil
}

func (t *workflowTaskToolset) claim(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	task, err := t.resolveTask(context.Background(), parent, tasks, plannerStringArg(args, "taskId"), true)
	if err != nil {
		return nil, err
	}
	executor := plannerStringArg(args, "executor")
	if executor != workflowTaskExecutorChild {
		executor = workflowTaskExecutorSelf
	}
	updated, err := t.executor.runtime.store.UpdateTask(context.Background(), task.ID, TaskPatchRequest{Status: new("IN_PROGRESS"), Executor: new(executor)})
	if err != nil {
		return nil, err
	}
	t.currentTaskID = updated.ID
	parent, tasks, jftradeErr16 := t.parentAndTasks(context.Background())
	jftradeLogError(jftradeErr16)
	jftradeErr11 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr11)
	return map[string]any{"success": true, "task": taskToolTaskSummary(updated)}, nil
}

func (t *workflowTaskToolset) complete(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	task, err := t.resolveTask(context.Background(), parent, tasks, plannerStringArg(args, "taskId"), false)
	if err != nil {
		return nil, err
	}
	switch strings.ToUpper(strings.TrimSpace(task.Status)) {
	case "DONE", "CANCELLED", "BLOCKED":
		return map[string]any{
			"success": false, "message": "task is not completable in its current status",
			"taskId": task.ID, "status": task.Status,
		}, nil
	}
	if task.Executor == workflowTaskExecutorChild && strings.TrimSpace(task.RunID) != "" {
		child, ok, childErr := t.executor.runtime.store.Run(context.Background(), task.RunID)
		if childErr != nil {
			return nil, childErr
		}
		if !ok || !isDirectWorkflowChild(parent, child) {
			return map[string]any{"success": false, "message": "delegated child run is unavailable", "taskId": task.ID}, nil
		}
		if child.Status != RunStatusCompleted {
			return map[string]any{
				"success": false, "message": "delegated task cannot be completed before its child run succeeds",
				"taskId": task.ID, "childRunId": child.ID, "childStatus": child.Status,
			}, nil
		}
	}
	summary := plannerStringArg(args, "resultSummary")
	if summary == "" {
		summary = plannerStringArg(args, "summary")
	}
	if summary == "" {
		summary = workflowSelfTaskSummary(task)
	}
	updated, err := t.executor.runtime.store.UpdateTask(context.Background(), task.ID, TaskPatchRequest{
		Status: new("DONE"), Executor: new(defaultString(task.Executor, workflowTaskExecutorSelf)), ResultSummary: new(summary),
	})
	if err != nil {
		return nil, err
	}
	t.currentTaskID = ""
	parent, tasks, jftradeErr12 := t.parentAndTasks(context.Background())
	jftradeLogError(jftradeErr12)
	jftradeErr14 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr14)
	return map[string]any{"success": true, "task": taskToolTaskSummary(updated)}, nil
}

func (t *workflowTaskToolset) block(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	task, err := t.resolveTask(context.Background(), parent, tasks, plannerStringArg(args, "taskId"), false)
	if err != nil {
		return nil, err
	}
	reason := plannerStringArg(args, "reason")
	if reason == "" {
		reason = "任务被阻塞。"
	}
	updated, err := t.executor.runtime.store.UpdateTask(context.Background(), task.ID, TaskPatchRequest{
		Status: new("BLOCKED"), ResultSummary: new(reason),
	})
	if err != nil {
		return nil, err
	}
	parent, tasks, jftradeErr20 := t.parentAndTasks(context.Background())
	jftradeLogError(jftradeErr20)
	jftradeErr18 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr18)
	return map[string]any{"success": true, "task": taskToolTaskSummary(updated)}, nil
}

func (t *workflowTaskToolset) delegate(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	task, err := t.resolveTask(context.Background(), parent, tasks, plannerStringArg(args, "taskId"), true)
	if err != nil {
		return nil, err
	}
	if task.Executor == workflowTaskExecutorChild && strings.TrimSpace(task.RunID) != "" {
		child, ok, childErr := t.executor.runtime.store.Run(context.Background(), task.RunID)
		if childErr != nil {
			return nil, childErr
		}
		if ok && isDirectWorkflowChild(parent, child) && (child.Status == RunStatusPending || child.Status == RunStatusRunning || child.Status == RunStatusCompleted) {
			return map[string]any{
				"success": true, "taskId": task.ID, "childRunId": child.ID, "status": child.Status,
				"pendingApproval": child.Status == RunStatusPending, "result": strings.TrimSpace(child.Message),
				"reused": true,
			}, nil
		}
	}
	step := workflowStepFromTask(task)
	if prompt := plannerStringArg(args, "prompt"); prompt != "" {
		step.Message = prompt
	}
	if role := plannerStringArg(args, "agentRole"); role != "" {
		step.AgentRole = role
	}
	if providerID := plannerStringArg(args, "childProviderId"); providerID != "" {
		step.ChildProviderID = providerID
	}
	if modelName := plannerStringArg(args, "childModel"); modelName != "" {
		step.ChildModel = modelName
	}
	_, jftradeErr31 := t.executor.runtime.store.UpdateTask(context.Background(), task.ID, TaskPatchRequest{
		Executor: new(workflowTaskExecutorChild), ChildProviderID: &step.ChildProviderID, ChildModel: &step.ChildModel,
	})
	jftradeLogError(jftradeErr31)
	result := t.executor.runChild(context.Background(), t.req, parent, step, task, workflowTaskIteration(task))
	if result.Err != nil {
		return map[string]any{"success": false, "message": result.Err.Error()}, nil
	}
	parent, ok, err := t.executor.runtime.store.Run(context.Background(), parent.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("parent run not found")
	}
	parent = t.executor.mergeTaskChildProjectionAt(context.Background(), parent, result.Response.Run, workflowPlanIndexForTask(parent.WorkflowPlan, task.ID))
	if result.Response.Run.Status == RunStatusPending {
		parent = pauseParentForChild(parent, result.Response.Run, workflowPlanIndexForTask(parent.WorkflowPlan, task.ID))
		_, jftradeErr30 := t.executor.runtime.saveRunPreservingUserGoalPause(context.Background(), parent)
		jftradeLogError(jftradeErr30)
	}
	t.currentTaskID = ""
	return map[string]any{
		"success": true, "taskId": task.ID, "childRunId": result.Response.Run.ID, "status": result.Response.Run.Status,
		"pendingApproval": result.Response.Run.Status == RunStatusPending, "result": strings.TrimSpace(result.Response.Reply),
	}, nil
}

func (t *workflowTaskToolset) goalComplete(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.parentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	if blockers := t.workflowCompletionBlockers(context.Background(), parent, tasks); len(blockers) > 0 {
		return map[string]any{
			"success":  false,
			"status":   "blocked",
			"message":  "goal cannot complete while workflow tasks or child runs are unfinished",
			"blockers": blockers,
		}, nil
	}
	summary := plannerStringArg(args, "summary")
	if summary == "" {
		summary = plannerStringArg(args, "resultSummary")
	}
	t.req.GoalDecision.setComplete(summary)
	jftradeErr17 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr17)
	return map[string]any{"success": true, "status": "complete", "summary": summary}, nil
}

func (t *workflowTaskToolset) workflowCompletionBlockers(ctx context.Context, parent Run, tasks []Task) []map[string]any {
	blockers := make([]map[string]any, 0)
	pendingApprovalRuns := map[string]struct{}{}
	if approvals, err := t.executor.runtime.store.ListApprovals(ctx); err == nil {
		for _, approval := range approvals {
			if approval.Status == ApprovalStatusPending {
				pendingApprovalRuns[strings.TrimSpace(approval.RunID)] = struct{}{}
			}
		}
	}
	for _, task := range tasks {
		status := strings.ToUpper(strings.TrimSpace(task.Status))
		if status != "DONE" && status != "CANCELLED" {
			blockers = append(blockers, map[string]any{"type": "task", "id": task.ID, "status": status})
		}
		if task.Executor != workflowTaskExecutorChild || strings.TrimSpace(task.RunID) == "" {
			continue
		}
		child, ok, err := t.executor.runtime.store.Run(ctx, task.RunID)
		if err != nil || !ok || !isDirectWorkflowChild(parent, child) {
			blockers = append(blockers, map[string]any{"type": "child_run", "id": task.RunID, "status": "MISSING"})
			continue
		}
		if child.Status != RunStatusCompleted {
			blockers = append(blockers, map[string]any{"type": "child_run", "id": child.ID, "status": child.Status})
			continue
		}
		if _, pending := pendingApprovalRuns[child.ID]; pending || t.executor.runtime.runExecutionInFlight(child.ID) {
			blockers = append(blockers, map[string]any{"type": "child_run", "id": child.ID, "status": "STILL_ACTIVE"})
		}
	}
	known := make(map[string]struct{}, len(tasks))
	for _, task := range tasks {
		if strings.TrimSpace(task.RunID) != "" {
			known[task.RunID] = struct{}{}
		}
	}
	for _, childRunID := range parent.ChildRunIDs {
		childRunID = strings.TrimSpace(childRunID)
		if childRunID == "" || childRunID == parent.ID {
			continue
		}
		if _, ok := known[childRunID]; ok {
			continue
		}
		child, ok, err := t.executor.runtime.store.Run(ctx, childRunID)
		_, pending := pendingApprovalRuns[childRunID]
		if err != nil || !ok || !isDirectWorkflowChild(parent, child) || child.Status != RunStatusCompleted || pending || t.executor.runtime.runExecutionInFlight(childRunID) {
			status := "MISSING"
			if ok {
				status = child.Status
			}
			blockers = append(blockers, map[string]any{"type": "child_run", "id": childRunID, "status": status})
		}
	}
	return blockers
}

func (t *workflowTaskToolset) goalContinue(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	reason := plannerStringArg(args, "reason")
	if reason == "" {
		reason = "目标尚未完成。"
	}
	t.req.GoalDecision.setContinue(reason)
	parent, tasks, jftradeErr21 := t.parentAndTasks(context.Background())
	jftradeLogError(jftradeErr21)
	jftradeErr19 := t.saveParentPlan(context.Background(), parent, tasks)
	jftradeLogError(jftradeErr19)
	return map[string]any{"success": true, "status": "continue", "reason": reason}, nil
}

func (t *workflowTaskToolset) parentAndTasks(ctx context.Context) (Run, []Task, error) {
	parent, ok, err := t.executor.runtime.store.Run(ctx, t.parentID)
	if err != nil {
		return Run{}, nil, err
	}
	if !ok {
		return Run{}, nil, fmt.Errorf("parent run not found")
	}
	tasks, err := t.executor.workflowTasks(ctx, parent, nil)
	if err != nil {
		return Run{}, nil, err
	}
	return parent, tasks, nil
}

func (t *workflowTaskToolset) saveParentPlan(ctx context.Context, parent Run, tasks []Task) error {
	parent.WorkflowPlan = workflowPlanFromTasks(tasks, parent.WorkflowPlan)
	_, err := t.executor.runtime.saveRunPreservingUserGoalPause(ctx, parent)
	return err
}

func (t *workflowTaskToolset) taskByID(ctx context.Context, id string) (Task, bool) {
	if strings.TrimSpace(id) == "" {
		return Task{}, false
	}
	task, ok, err := t.executor.runtime.store.Task(ctx, id)
	if err != nil {
		return Task{}, false
	}
	return task, ok
}

func (t *workflowTaskToolset) resolveTask(ctx context.Context, parent Run, tasks []Task, id string, allowReady bool) (Task, error) {
	if strings.TrimSpace(id) != "" {
		task, ok := t.taskByID(ctx, id)
		if !ok {
			return Task{}, fmt.Errorf("task not found: %s", id)
		}
		return task, nil
	}
	if task, ok := t.taskByID(ctx, t.currentTaskID); ok && task.Status != "DONE" && task.Status != "CANCELLED" {
		return task, nil
	}
	for _, task := range tasks {
		if task.Status == "IN_PROGRESS" {
			return task, nil
		}
	}
	if allowReady {
		ready := executableWorkflowTasks(tasks, parent.WorkMode)
		if len(ready) > 0 {
			return ready[0], nil
		}
	}
	return Task{}, fmt.Errorf("no executable workflow task")
}

func (e *WorkflowExecutor) mergeTaskChildProjectionAt(ctx context.Context, parent Run, child Run, index int) Run {
	if strings.TrimSpace(child.ID) != "" {
		parent.ChildRunIDs = appendUniqueString(parent.ChildRunIDs, child.ID)
	}
	parent = updateWorkflowPlanForChildAt(parent, child, index)
	if child.Status == RunStatusPending {
		parent.Status = RunStatusPending
		parent.PendingApprovals = append([]Approval(nil), child.PendingApprovals...)
	}
	parent, jftradeErr28 := e.runtime.saveRunPreservingUserGoalPause(ctx, parent)
	jftradeLogError(jftradeErr28)
	return parent
}

var _ adktool.Toolset = (*workflowTaskToolset)(nil)
