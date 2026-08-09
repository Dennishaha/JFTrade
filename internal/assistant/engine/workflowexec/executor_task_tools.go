package workflowexec

import (
	"context"
	"fmt"
	"strings"
	"sync"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	adkagent "google.golang.org/adk/v2/agent"
	adktool "google.golang.org/adk/v2/tool"
)

type WorkflowTaskToolset struct {
	mu            sync.Mutex
	Executor      *WorkflowExecutor
	Req           WorkflowRequest
	ParentID      string
	CurrentTaskID string
}

// NewWorkflowTaskToolset constructs the workflow task toolset used by the
// executor and exposed to white-box tests in the engine-root package.
func NewWorkflowTaskToolset(executor *WorkflowExecutor, parentID string, currentTaskID string) *WorkflowTaskToolset {
	return &WorkflowTaskToolset{
		Executor:      executor,
		ParentID:      parentID,
		CurrentTaskID: currentTaskID,
	}
}

func (t *WorkflowTaskToolset) Name() string { return "jftrade-workflow-task-tools" }

func (t *WorkflowTaskToolset) Tools(adkagent.ReadonlyContext) ([]adktool.Tool, error) {
	if jfadkmodel.NormalizeWorkMode(t.Req.Mode) == WorkModeLoop && t.Req.GoalDecision != nil && t.Req.GoalDecision.DecisionPhase() {
		return jfadkmodel.NewWorkflowMapFunctionTools(
			jfadkmodel.WorkflowMapToolSpec{Name: workflowGoalCompleteTool, Description: "Declare that the current objective is complete and finish the goal loop.", Schema: jfadkmodel.WorkflowGoalCompleteSchema(), Run: t.GoalComplete},
			jfadkmodel.WorkflowMapToolSpec{Name: workflowGoalContinueTool, Description: "Declare that the current objective is not complete yet and continue orchestration.", Schema: jfadkmodel.WorkflowGoalContinueSchema(), Run: t.GoalContinue},
		)
	}
	modelsListTool, err := t.ModelsListTool()
	if err != nil {
		return nil, err
	}
	tools, err := jfadkmodel.NewWorkflowMapFunctionTools(
		jfadkmodel.WorkflowMapToolSpec{Name: workflowTasksListTool, Description: "List current workflow TODO DAG, ready tasks, completed results and blocked state.", Schema: jfadkmodel.EmptyObjectSchema(), Run: t.List},
		jfadkmodel.WorkflowMapToolSpec{Name: workflowTaskAddTool, Description: "Add a runtime TODO to the current ADK task workflow.", Schema: jfadkmodel.WorkflowTaskAddSchema(), Run: t.Add},
		jfadkmodel.WorkflowMapToolSpec{Name: workflowTaskClaimTool, Description: "Claim a ready TODO for the orchestrator itself or a child agent.", Schema: jfadkmodel.WorkflowTaskClaimSchema(), Run: t.Claim},
		jfadkmodel.WorkflowMapToolSpec{Name: workflowTaskCompleteTool, Description: "Mark a claimed or ready TODO as DONE with a result summary.", Schema: jfadkmodel.WorkflowTaskCompleteSchema(), Run: t.Complete},
		jfadkmodel.WorkflowMapToolSpec{Name: workflowTaskBlockTool, Description: "Mark a TODO as BLOCKED with a blocking reason.", Schema: jfadkmodel.WorkflowTaskBlockSchema(), Run: t.Block},
		jfadkmodel.WorkflowMapToolSpec{Name: workflowTaskDelegateTool, Description: "Delegate a ready TODO to an ADK child agent. This creates a JFTrade child run only when called.", Schema: jfadkmodel.WorkflowTaskDelegateSchema(), Run: t.Delegate},
	)
	if err != nil {
		return nil, err
	}
	return append(tools, modelsListTool), nil
}

func (t *WorkflowTaskToolset) ModelsListTool() (adktool.Tool, error) {
	return jfadkmodel.NewWorkflowMapFunctionTool(jfadkmodel.WorkflowMapToolSpec{
		Name:        workflowModelsListTool,
		Description: "List callable ADK models that can be selected for delegated child agents.",
		Schema:      jfadkmodel.WorkflowModelsListSchema(),
		Run: func(args map[string]any) (map[string]any, error) {
			return t.ModelsList(args)
		},
	})
}

func (t *WorkflowTaskToolset) ModelsList(args map[string]any) (map[string]any, error) {
	if t == nil || t.Executor == nil || t.Executor.runtime == nil {
		return nil, fmt.Errorf("adk runtime is unavailable")
	}
	output, err := t.Executor.runtime.ModelsListTool(context.Background(), args)
	if err != nil {
		return nil, err
	}
	mapped, ok := output.(map[string]any)
	if !ok {
		return map[string]any{"result": output}, nil
	}
	return mapped, nil
}

func (t *WorkflowTaskToolset) List(map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.syncParentPlan(context.Background())
	if err != nil {
		return nil, err
	}
	return map[string]any{"success": true, "tasks": jfadkmodel.TaskToolTaskSummaries(tasks), "readyTasks": jfadkmodel.TaskToolTaskSummaries(jfadkmodel.ExecutableWorkflowTasks(tasks, parent.WorkMode))}, nil
}

func (t *WorkflowTaskToolset) Add(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, _, err := t.ParentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	current, _, err := t.TaskByID(context.Background(), t.CurrentTaskID)
	if err != nil {
		return nil, err
	}
	task, err := t.Executor.AddRuntimeWorkflowTask(context.Background(), parent, current, workflowRuntimeTaskRequest{
		Title: jfadkmodel.PlannerStringArg(args, "title"), Message: jfadkmodel.PlannerStringArg(args, "message"), Description: jfadkmodel.PlannerStringArg(args, "description"),
		DependsOn: jfadkmodel.PlannerStringListArg(args, "dependsOn"), AgentRole: jfadkmodel.PlannerStringArg(args, "agentRole"), ModeHint: jfadkmodel.PlannerStringArg(args, "modeHint"),
		ChildProviderID: jfadkmodel.PlannerStringArg(args, "childProviderId"), ChildModel: jfadkmodel.PlannerStringArg(args, "childModel"),
	})
	if err != nil {
		return nil, err
	}
	if _, _, err := t.syncParentPlan(context.Background()); err != nil {
		return taskMutationProjectionFailure(task, err)
	}
	return map[string]any{"success": true, "task": jfadkmodel.TaskToolTaskSummary(task)}, nil
}

func (t *WorkflowTaskToolset) Claim(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.ParentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	task, err := t.ResolveTask(context.Background(), parent, tasks, jfadkmodel.PlannerStringArg(args, "taskId"), true)
	if err != nil {
		return nil, err
	}
	executor := jfadkmodel.PlannerStringArg(args, "executor")
	if executor != workflowTaskExecutorChild {
		executor = workflowTaskExecutorSelf
	}
	updated, err := t.Executor.runtime.WorkflowStore().UpdateTask(context.Background(), task.ID, TaskPatchRequest{Status: new("IN_PROGRESS"), Executor: new(executor)})
	if err != nil {
		return nil, err
	}
	if _, _, err := t.syncParentPlan(context.Background()); err != nil {
		t.CurrentTaskID = updated.ID
		return taskMutationProjectionFailure(updated, err)
	}
	t.CurrentTaskID = updated.ID
	return map[string]any{"success": true, "task": jfadkmodel.TaskToolTaskSummary(updated)}, nil
}

func (t *WorkflowTaskToolset) Complete(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.ParentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	task, err := t.ResolveTask(context.Background(), parent, tasks, jfadkmodel.PlannerStringArg(args, "taskId"), false)
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
		child, ok, childErr := t.Executor.runtime.WorkflowStore().Run(context.Background(), task.RunID)
		if childErr != nil {
			return nil, childErr
		}
		if !ok || !jfadkmodel.IsDirectWorkflowChild(parent, child) {
			return map[string]any{"success": false, "message": "delegated child run is unavailable", "taskId": task.ID}, nil
		}
		if child.Status != RunStatusCompleted {
			return map[string]any{
				"success": false, "message": "delegated task cannot be completed before its child run succeeds",
				"taskId": task.ID, "childRunId": child.ID, "childStatus": child.Status,
			}, nil
		}
	}
	summary := jfadkmodel.PlannerStringArg(args, "resultSummary")
	if summary == "" {
		summary = jfadkmodel.PlannerStringArg(args, "summary")
	}
	if summary == "" {
		summary = jfadkmodel.WorkflowSelfTaskSummary(task)
	}
	updated, err := t.Executor.runtime.WorkflowStore().UpdateTask(context.Background(), task.ID, TaskPatchRequest{
		Status: new("DONE"), Executor: new(jfadkmodel.DefaultString(task.Executor, workflowTaskExecutorSelf)), ResultSummary: new(summary),
	})
	if err != nil {
		return nil, err
	}
	if _, _, err := t.syncParentPlan(context.Background()); err != nil {
		t.CurrentTaskID = ""
		return taskMutationProjectionFailure(updated, err)
	}
	t.CurrentTaskID = ""
	return map[string]any{"success": true, "task": jfadkmodel.TaskToolTaskSummary(updated)}, nil
}

func (t *WorkflowTaskToolset) Block(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.ParentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	task, err := t.ResolveTask(context.Background(), parent, tasks, jfadkmodel.PlannerStringArg(args, "taskId"), false)
	if err != nil {
		return nil, err
	}
	reason := jfadkmodel.PlannerStringArg(args, "reason")
	if reason == "" {
		reason = "任务被阻塞。"
	}
	updated, err := t.Executor.runtime.WorkflowStore().UpdateTask(context.Background(), task.ID, TaskPatchRequest{
		Status: new("BLOCKED"), ResultSummary: new(reason),
	})
	if err != nil {
		return nil, err
	}
	if _, _, err := t.syncParentPlan(context.Background()); err != nil {
		return taskMutationProjectionFailure(updated, err)
	}
	return map[string]any{"success": true, "task": jfadkmodel.TaskToolTaskSummary(updated)}, nil
}

func (t *WorkflowTaskToolset) Delegate(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.ParentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	task, err := t.ResolveTask(context.Background(), parent, tasks, jfadkmodel.PlannerStringArg(args, "taskId"), true)
	if err != nil {
		return nil, err
	}
	if task.Executor == workflowTaskExecutorChild && strings.TrimSpace(task.RunID) != "" {
		child, ok, childErr := t.Executor.runtime.WorkflowStore().Run(context.Background(), task.RunID)
		if childErr != nil {
			return nil, childErr
		}
		if ok && jfadkmodel.IsDirectWorkflowChild(parent, child) && (child.Status == RunStatusPending || child.Status == RunStatusPendingInput || child.Status == RunStatusRunning || child.Status == RunStatusCompleted) {
			return map[string]any{
				"success": true, "taskId": task.ID, "childRunId": child.ID, "status": child.Status,
				"pendingApproval": child.Status == RunStatusPending, "result": strings.TrimSpace(child.Message),
				"pendingInput": child.Status == RunStatusPendingInput,
				"reused":       true,
			}, nil
		}
	}
	step := jfadkmodel.WorkflowStepFromTask(task)
	if prompt := jfadkmodel.PlannerStringArg(args, "prompt"); prompt != "" {
		step.Message = prompt
	}
	if role := jfadkmodel.PlannerStringArg(args, "agentRole"); role != "" {
		step.AgentRole = role
	}
	if providerID := jfadkmodel.PlannerStringArg(args, "childProviderId"); providerID != "" {
		step.ChildProviderID = providerID
	}
	if modelName := jfadkmodel.PlannerStringArg(args, "childModel"); modelName != "" {
		step.ChildModel = modelName
	}
	if _, err := t.Executor.runtime.WorkflowStore().UpdateTask(context.Background(), task.ID, TaskPatchRequest{
		Executor: new(workflowTaskExecutorChild), ChildProviderID: &step.ChildProviderID, ChildModel: &step.ChildModel,
	}); err != nil {
		return nil, err
	}
	result := t.Executor.RunChild(context.Background(), t.Req, parent, step, task, jfadkmodel.WorkflowTaskIteration(task))
	if result.Err != nil {
		return map[string]any{"success": false, "message": result.Err.Error()}, nil
	}
	parent, ok, err := t.Executor.runtime.WorkflowStore().Run(context.Background(), parent.ID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("parent run not found")
	}
	parent, err = t.Executor.MergeTaskChildProjectionAt(context.Background(), parent, result.Response.Run, jfadkmodel.WorkflowPlanIndexForTask(parent.WorkflowPlan, task.ID))
	if err != nil {
		return nil, err
	}
	if result.Response.Run.Status == RunStatusPending || result.Response.Run.Status == RunStatusPendingInput {
		parent = jfadkmodel.PauseParentForChild(parent, result.Response.Run, jfadkmodel.WorkflowPlanIndexForTask(parent.WorkflowPlan, task.ID))
		if _, err := t.Executor.runtime.SaveRunPreservingUserGoalPause(context.Background(), parent); err != nil {
			return nil, err
		}
	}
	t.CurrentTaskID = ""
	return map[string]any{
		"success": true, "taskId": task.ID, "childRunId": result.Response.Run.ID, "status": result.Response.Run.Status,
		"pendingApproval": result.Response.Run.Status == RunStatusPending, "result": strings.TrimSpace(result.Response.Reply),
		"pendingInput": result.Response.Run.Status == RunStatusPendingInput,
	}, nil
}

func (t *WorkflowTaskToolset) GoalComplete(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, tasks, err := t.ParentAndTasks(context.Background())
	if err != nil {
		return nil, err
	}
	blockers, err := t.WorkflowCompletionBlockers(context.Background(), parent, tasks)
	if err != nil {
		return nil, err
	}
	if len(blockers) > 0 {
		return map[string]any{
			"success":  false,
			"status":   "blocked",
			"message":  "goal cannot complete while workflow tasks or child runs are unfinished",
			"blockers": blockers,
		}, nil
	}
	summary := jfadkmodel.PlannerStringArg(args, "summary")
	if summary == "" {
		summary = jfadkmodel.PlannerStringArg(args, "resultSummary")
	}
	if err := t.SaveParentPlan(context.Background(), parent, tasks); err != nil {
		return nil, err
	}
	t.Req.GoalDecision.SetComplete(summary)
	return map[string]any{"success": true, "status": "complete", "summary": summary}, nil
}

func (t *WorkflowTaskToolset) WorkflowCompletionBlockers(ctx context.Context, parent Run, tasks []Task) ([]map[string]any, error) {
	blockers := make([]map[string]any, 0)
	pendingApprovalRuns := map[string]struct{}{}
	approvals, err := t.Executor.runtime.WorkflowStore().ListApprovals(ctx)
	if err != nil {
		return nil, err
	}
	for _, approval := range approvals {
		if approval.Status == ApprovalStatusPending {
			pendingApprovalRuns[strings.TrimSpace(approval.RunID)] = struct{}{}
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
		child, ok, err := t.Executor.runtime.WorkflowStore().Run(ctx, task.RunID)
		if err != nil || !ok || !jfadkmodel.IsDirectWorkflowChild(parent, child) {
			blockers = append(blockers, map[string]any{"type": "child_run", "id": task.RunID, "status": "MISSING"})
			continue
		}
		if child.Status != RunStatusCompleted {
			blockers = append(blockers, map[string]any{"type": "child_run", "id": child.ID, "status": child.Status})
			continue
		}
		if _, pending := pendingApprovalRuns[child.ID]; pending || t.Executor.runtime.RunExecutionInFlight(child.ID) {
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
		child, ok, err := t.Executor.runtime.WorkflowStore().Run(ctx, childRunID)
		_, pending := pendingApprovalRuns[childRunID]
		if err != nil || !ok || !jfadkmodel.IsDirectWorkflowChild(parent, child) || child.Status != RunStatusCompleted || pending || t.Executor.runtime.RunExecutionInFlight(childRunID) {
			status := "MISSING"
			if ok {
				status = child.Status
			}
			blockers = append(blockers, map[string]any{"type": "child_run", "id": childRunID, "status": status})
		}
	}
	return blockers, nil
}

func (t *WorkflowTaskToolset) GoalContinue(args map[string]any) (map[string]any, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	reason := jfadkmodel.PlannerStringArg(args, "reason")
	if reason == "" {
		reason = "目标尚未完成。"
	}
	if _, _, err := t.syncParentPlan(context.Background()); err != nil {
		return nil, err
	}
	t.Req.GoalDecision.SetContinue(reason)
	return map[string]any{"success": true, "status": "continue", "reason": reason}, nil
}

func (t *WorkflowTaskToolset) ParentAndTasks(ctx context.Context) (Run, []Task, error) {
	parent, ok, err := t.Executor.runtime.WorkflowStore().Run(ctx, t.ParentID)
	if err != nil {
		return Run{}, nil, err
	}
	if !ok {
		return Run{}, nil, fmt.Errorf("parent run not found")
	}
	tasks, err := t.Executor.WorkflowTasks(ctx, parent, nil)
	if err != nil {
		return Run{}, nil, err
	}
	return parent, tasks, nil
}

func (t *WorkflowTaskToolset) SaveParentPlan(ctx context.Context, parent Run, tasks []Task) error {
	parent.WorkflowPlan = jfadkmodel.WorkflowPlanFromTasks(tasks, parent.WorkflowPlan)
	_, err := t.Executor.runtime.SaveRunPreservingUserGoalPause(ctx, parent)
	return err
}

func (t *WorkflowTaskToolset) syncParentPlan(ctx context.Context) (Run, []Task, error) {
	parent, tasks, err := t.ParentAndTasks(ctx)
	if err != nil {
		return Run{}, nil, err
	}
	if err := t.SaveParentPlan(ctx, parent, tasks); err != nil {
		return Run{}, nil, err
	}
	return parent, tasks, nil
}

func (t *WorkflowTaskToolset) TaskByID(ctx context.Context, id string) (Task, bool, error) {
	if strings.TrimSpace(id) == "" {
		return Task{}, false, nil
	}
	return t.Executor.runtime.WorkflowStore().Task(ctx, id)
}

func taskMutationProjectionFailure(task Task, err error) (map[string]any, error) {
	return jfadkmodel.TaskMutationProjectionFailure(task, err)
}

func (t *WorkflowTaskToolset) ResolveTask(ctx context.Context, parent Run, tasks []Task, id string, allowReady bool) (Task, error) {
	if strings.TrimSpace(id) != "" {
		task, ok, err := t.TaskByID(ctx, id)
		if err != nil {
			return Task{}, err
		}
		if !ok {
			return Task{}, fmt.Errorf("task not found: %s", id)
		}
		return task, nil
	}
	if task, ok, err := t.TaskByID(ctx, t.CurrentTaskID); err != nil {
		return Task{}, err
	} else if ok && task.Status != "DONE" && task.Status != "CANCELLED" {
		return task, nil
	}
	for _, task := range tasks {
		if task.Status == "IN_PROGRESS" {
			return task, nil
		}
	}
	if allowReady {
		ready := jfadkmodel.ExecutableWorkflowTasks(tasks, parent.WorkMode)
		if len(ready) > 0 {
			return ready[0], nil
		}
	}
	return Task{}, fmt.Errorf("no executable workflow task")
}

func (e *WorkflowExecutor) MergeTaskChildProjectionAt(ctx context.Context, parent Run, child Run, index int) (Run, error) {
	if strings.TrimSpace(child.ID) != "" {
		parent.ChildRunIDs = jfadkmodel.AppendUniqueString(parent.ChildRunIDs, child.ID)
	}
	parent = jfadkmodel.UpdateWorkflowPlanForChildAt(parent, child, index)
	if child.Status == RunStatusPending || child.Status == RunStatusPendingInput {
		parent.Status = child.Status
		parent.PendingApprovals = append([]Approval(nil), child.PendingApprovals...)
		parent.InputRequest = jfadkmodel.NormalizeInputRequest(child.InputRequest)
	}
	parent, err := e.runtime.SaveRunPreservingUserGoalPause(ctx, parent)
	if err != nil {
		return Run{}, err
	}
	return parent, nil
}

var _ adktool.Toolset = (*WorkflowTaskToolset)(nil)
