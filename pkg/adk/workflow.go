package adk

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/genai"
)

const (
	workflowStatusRunning  = "RUNNING"
	workflowStatusPaused   = "PAUSED"
	workflowStatusComplete = "COMPLETED"
	workflowStatusFailed   = "FAILED"

	workflowPlanSourcePlanner = "planner"
	workflowPlanSourceRuntime = "runtime"

	workflowTaskExecutorSelf  = "self"
	workflowTaskExecutorChild = "child"

	maxRuntimeWorkflowTasks = 10
)

type WorkflowExecutor struct {
	runtime *Runtime
}

type workflowRequest struct {
	Agent      Agent
	Session    Session
	Message    string
	Mode       string
	Objective  string
	RunOptions RunOptions
	OnDelta    func(ChatDelta) error
	EmitRun    bool

	GoalDecision *workflowGoalDecision
}

type workflowStep struct {
	Order           int
	DependencyID    string
	Title           string
	Description     string
	Message         string
	DependsOn       []string
	AgentRole       string
	ModeHint        string
	Objective       string
	PlanSource      string
	WorkflowMode    string
	PlannerWarnings []string
}

type workflowChildResult struct {
	Index    int
	TaskID   string
	Response ChatResponse
	Err      error
}

func (r *Runtime) workflowExecutor() *WorkflowExecutor {
	return &WorkflowExecutor{runtime: r}
}

func (e *WorkflowExecutor) Run(ctx context.Context, req workflowRequest) (ChatResponse, error) {
	if e == nil || e.runtime == nil {
		return ChatResponse{}, fmt.Errorf("adk runtime is unavailable")
	}
	mode := normalizeWorkMode(req.Mode)
	if mode == WorkModeChat {
		return ChatResponse{}, fmt.Errorf("workflow mode is required")
	}
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		objective = req.Message
	}
	parent, parentCtx, finishParent, err := e.runtime.startRunWithOptions(ctx, req.Session.ID, req.Agent, req.Message, runStartOptions{
		WorkMode:       mode,
		Objective:      objective,
		WorkflowStatus: workflowStatusRunning,
	})
	if err != nil {
		return ChatResponse{}, err
	}
	defer finishParent()
	if req.EmitRun && req.OnDelta != nil {
		if err := req.OnDelta(ChatDelta{Run: &parent}); err != nil {
			return ChatResponse{}, err
		}
	}
	if mode == WorkModeLoop {
		task, err := e.createInitialGoalTask(parentCtx, parent, req.Agent, objective, req.Message)
		if err != nil {
			parent = e.failParent(parentCtx, parent, err)
			return e.workflowResponse(parentCtx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
		}
		parent.WorkflowPlan = workflowPlanFromTasks([]Task{task}, parent.WorkflowPlan)
		_ = e.runtime.store.SaveRun(parentCtx, parent)
		return e.runADKGoalWorkflow(parentCtx, req, parent, []Task{task})
	}
	steps, planningWarnings, err := e.planWorkflowSteps(parentCtx, req, mode, objective)
	if err != nil {
		parent = e.failParent(parentCtx, parent, err)
		return e.workflowResponse(parentCtx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	steps = applyWorkflowStepPlanningMetadata(steps, mode, objective, planningWarnings)
	tasks, err := e.persistWorkflowTasks(parentCtx, parent, req.Agent, steps)
	if err != nil {
		parent = e.failParent(parentCtx, parent, err)
		return e.workflowResponse(parentCtx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	parent.WorkflowPlan = workflowPlanFromSteps(steps, tasks)
	if len(planningWarnings) > 0 {
		parent.Message = strings.Join(planningWarnings, "; ")
	}
	_ = e.runtime.store.SaveRun(parentCtx, parent)
	if mode == WorkModeTask {
		return e.runADKTaskWorkflow(parentCtx, req, parent, tasks)
	}
	return e.runGoogleADKWorkflow(parentCtx, req, parent, steps, tasks)
}

func (e *WorkflowExecutor) createInitialGoalTask(ctx context.Context, parent Run, agent Agent, objective string, message string) (Task, error) {
	taskMessage := strings.TrimSpace(message)
	if taskMessage == "" {
		taskMessage = strings.TrimSpace(objective)
	}
	title := strings.TrimSpace(objective)
	if title == "" {
		title = taskMessage
	}
	if title == "" {
		title = "推进目标"
	}
	if len([]rune(title)) > 80 {
		title = string([]rune(title)[:80])
	}
	return e.runtime.store.SaveTask(ctx, TaskWriteRequest{
		Title:        title,
		Description:  "目标模式初始任务",
		Status:       "TODO",
		AgentID:      agent.ID,
		RunID:        parent.ID,
		Order:        1,
		ModeHint:     WorkModeTask,
		PlanSource:   workflowPlanSourceRuntime,
		WorkflowMode: WorkModeLoop,
		Objective:    strings.TrimSpace(objective),
		Message:      taskMessage,
	})
}

func (e *WorkflowExecutor) planWorkflowSteps(ctx context.Context, req workflowRequest, mode string, objective string) ([]workflowStep, []string, error) {
	steps, warnings, err := e.runtime.planWorkflowWithADK(ctx, req.Agent, req.Session, mode, req.Message, objective, req.RunOptions)
	if err == nil && len(steps) > 0 {
		for index := range steps {
			steps[index].PlanSource = workflowPlanSourcePlanner
		}
		return steps, warnings, nil
	}
	if err != nil {
		return nil, warnings, fmt.Errorf("workflow planner failed: %w", err)
	}
	return nil, warnings, fmt.Errorf("workflow planner returned no steps")
}

func applyWorkflowStepPlanningMetadata(steps []workflowStep, mode string, objective string, warnings []string) []workflowStep {
	normalizedWarnings := normalizeStringSlice(warnings)
	normalizedMode := normalizeWorkMode(mode)
	normalizedObjective := strings.TrimSpace(objective)
	for index := range steps {
		if steps[index].Order <= 0 {
			steps[index].Order = index + 1
		}
		if strings.TrimSpace(steps[index].WorkflowMode) == "" {
			steps[index].WorkflowMode = normalizedMode
		}
		if strings.TrimSpace(steps[index].Objective) == "" {
			steps[index].Objective = normalizedObjective
		}
		if strings.TrimSpace(steps[index].PlanSource) == "" {
			steps[index].PlanSource = workflowPlanSourcePlanner
		}
		if len(normalizedWarnings) > 0 {
			steps[index].PlannerWarnings = append([]string(nil), normalizedWarnings...)
		}
	}
	return steps
}

func (e *WorkflowExecutor) persistWorkflowTasks(ctx context.Context, parent Run, agent Agent, steps []workflowStep) ([]Task, error) {
	tasks := make([]Task, 0, len(steps))
	taskIDByDependencyID := make(map[string]string, len(steps))
	for index, step := range steps {
		dependsOn := append([]string(nil), step.DependsOn...)
		for depIndex, dep := range dependsOn {
			if taskID, ok := taskIDByDependencyID[dep]; ok {
				dependsOn[depIndex] = taskID
				continue
			}
			if strings.HasPrefix(dep, "__previous_step_") && len(tasks) > 0 {
				dependsOn[depIndex] = tasks[len(tasks)-1].ID
			}
		}
		description := step.Description
		if strings.TrimSpace(step.AgentRole) != "" {
			if strings.TrimSpace(description) != "" {
				description += "\n\n"
			}
			description += "Agent role: " + strings.TrimSpace(step.AgentRole)
		}
		task, err := e.runtime.store.SaveTask(ctx, TaskWriteRequest{
			Title:           step.Title,
			Description:     description,
			Status:          "TODO",
			AgentID:         agent.ID,
			RunID:           parent.ID,
			DependsOn:       dependsOn,
			Order:           step.Order,
			ModeHint:        step.ModeHint,
			AgentRole:       step.AgentRole,
			PlannerStepID:   step.DependencyID,
			PlanSource:      step.PlanSource,
			WorkflowMode:    step.WorkflowMode,
			Objective:       step.Objective,
			Message:         step.Message,
			PlannerWarnings: step.PlannerWarnings,
		})
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
		if strings.TrimSpace(step.DependencyID) != "" {
			taskIDByDependencyID[strings.TrimSpace(step.DependencyID)] = task.ID
		}
		_ = index
	}
	return tasks, nil
}

func (e *WorkflowExecutor) runGoogleADKWorkflow(ctx context.Context, req workflowRequest, parent Run, steps []workflowStep, tasks []Task) (ChatResponse, error) {
	return e.runPlannedGoogleADKWorkflow(ctx, req, parent, steps, tasks)
}

func (e *WorkflowExecutor) runWorkflowOrchestrator(ctx context.Context, req workflowRequest, parent Run, tasks []Task) (ChatResponse, error) {
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.PendingApprovals = pendingApprovalsOnly(parent.PendingApprovals)
	parent.WorkflowPlan = workflowPlanFromTasks(tasks, parent.WorkflowPlan)
	if err := e.runtime.store.SaveRun(ctx, parent); err != nil {
		parent = e.failParent(ctx, parent, err)
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}

	replies := make([]string, 0, len(tasks))
	iterations := 0
	for {
		iterations++
		if iterations > len(tasks)+maxRuntimeWorkflowTasks {
			parent = e.failParent(ctx, parent, fmt.Errorf("workflow scheduler incomplete"))
			parent.ErrorCode = "WORKFLOW_SCHEDULER_INCOMPLETE"
			_ = e.runtime.store.SaveRun(ctx, parent)
			return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
		}

		currentTasks, err := e.workflowTasks(ctx, parent, tasks)
		if err != nil {
			parent = e.failParent(ctx, parent, err)
			return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
		}
		tasks = currentTasks
		parent.WorkflowPlan = workflowPlanFromTasks(tasks, parent.WorkflowPlan)
		if err := e.runtime.store.SaveRun(ctx, parent); err != nil {
			parent = e.failParent(ctx, parent, err)
			return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
		}

		if workflowTasksComplete(tasks) {
			reply := workflowSummary(parent, replies)
			parent.Status = RunStatusCompleted
			parent.Message = "workflow completed"
			parent.WorkflowStatus = workflowStatusComplete
			if parent.WorkMode == WorkModeLoop && parent.Iteration == 0 {
				parent.Iteration = 1
			}
			parent.PendingApprovals = pendingApprovalsOnly(parent.PendingApprovals)
			parent.CompletedAt = new(nowString())
			finalizeRunUsage(&parent)
			if saved, msgErr := e.runtime.attachFinalAssistantMessage(ctx, req.Session, parent, openAIChatResult{Reply: reply}); msgErr == nil {
				parent = saved
			} else {
				_ = e.runtime.store.SaveRun(ctx, parent)
			}
			return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: reply}), nil
		}

		if blockedTask, ok := firstTerminalWorkflowTask(tasks); ok {
			parent.Status = RunStatusFailed
			parent.WorkflowStatus = workflowStatusFailed
			parent.Message = defaultString(blockedTask.ResultSummary, blockedTask.Description)
			parent.FailureReason = parent.Message
			parent.ErrorCode = "WORKFLOW_TASK_BLOCKED"
			parent.Degraded = true
			parent.CompletedAt = new(nowString())
			finalizeRunUsage(&parent)
			_ = e.runtime.store.SaveRun(ctx, parent)
			return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
		}

		executable := executableWorkflowTasks(tasks, parent.WorkMode)
		if len(executable) == 0 {
			parent = e.failParent(ctx, parent, fmt.Errorf("workflow scheduler incomplete"))
			parent.ErrorCode = "WORKFLOW_SCHEDULER_INCOMPLETE"
			_ = e.runtime.store.SaveRun(ctx, parent)
			return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
		}

		if len(executable) > 1 {
			executable = executable[:1]
		}
		madeProgress := false
		for _, task := range executable {
			if workflowTaskShouldDelegate(task, req) {
				result := e.runChild(ctx, req, parent, workflowStepFromTask(task), task, workflowTaskIteration(task))
				if result.Err != nil {
					parent = e.failParent(ctx, parent, result.Err)
					return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
				}
				parent = e.mergeChildResultAt(ctx, parent, result.Response.Run, workflowPlanIndexForTask(parent.WorkflowPlan, task.ID))
				_, _ = e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{
					Executor:      stringPtr(workflowTaskExecutorChild),
					ResultSummary: stringPtr(strings.TrimSpace(result.Response.Reply)),
				})
				if result.Response.Run.Status == RunStatusPending {
					parent = pauseParentForChild(parent, result.Response.Run, workflowPlanIndexForTask(parent.WorkflowPlan, task.ID))
					_ = e.runtime.store.SaveRun(ctx, parent)
					return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: workflowPendingReply(parent)}), nil
				}
				if isWorkflowBlockingStatus(result.Response.Run.Status) {
					parent = e.runtime.terminateParentWorkflowFromChild(ctx, parent, result.Response.Run)
					return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
				}
				if strings.TrimSpace(result.Response.Reply) != "" {
					replies = append(replies, result.Response.Reply)
				}
				madeProgress = true
				continue
			}
			updatedParent, summary, paused, err := e.runSelfTask(ctx, req, parent, task)
			if err != nil {
				parent = e.failParent(ctx, parent, err)
				return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
			}
			parent = updatedParent
			updatedTask, ok, taskErr := e.runtime.store.Task(ctx, task.ID)
			if taskErr != nil {
				parent = e.failParent(ctx, parent, taskErr)
				return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
			}
			if ok {
				parent.WorkflowPlan = updateWorkflowPlanForTask(parent.WorkflowPlan, updatedTask)
			}
			replies = append(replies, summary)
			if paused {
				_ = e.runtime.store.SaveRun(ctx, parent)
				return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: workflowPendingReply(parent)}), nil
			}
			madeProgress = true
		}
		if !madeProgress {
			parent = e.failParent(ctx, parent, fmt.Errorf("workflow scheduler incomplete"))
			parent.ErrorCode = "WORKFLOW_SCHEDULER_INCOMPLETE"
			_ = e.runtime.store.SaveRun(ctx, parent)
			return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
		}
	}
}

func (e *WorkflowExecutor) runPlannedGoogleADKWorkflow(ctx context.Context, req workflowRequest, parent Run, steps []workflowStep, tasks []Task) (ChatResponse, error) {
	childRuns, finishes, err := e.startWorkflowChildRuns(ctx, req, parent, steps, tasks)
	for _, finish := range finishes {
		defer finish()
	}
	if err != nil {
		parent = e.failParent(ctx, parent, err)
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	parent.ChildRunIDs = childRunIDs(childRuns)
	for index, child := range childRuns {
		if index < len(parent.WorkflowPlan) {
			applyWorkflowChildState(&parent.WorkflowPlan[index], child)
		}
	}
	if err := e.runtime.store.SaveRun(ctx, parent); err != nil {
		parent = e.failParent(ctx, parent, err)
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	if err := emitWorkflowRunSnapshot(req, parent); err != nil {
		return ChatResponse{}, err
	}
	execution, err := e.runtime.newGoogleADKWorkflowExecution(ctx, req.Agent, req.Session, parent, childRuns, steps, parent.WorkMode, req.RunOptions, req.OnDelta)
	if err != nil {
		parent = e.failParent(ctx, parent, err)
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	e.runtime.workflowChildMu.Lock()
	adkErr := execution.run(ctx, genai.NewContentFromText(req.Message, genai.RoleUser))
	var approvals []Approval
	if adkErr == nil {
		approvals, err = execution.pendingApprovals(ctx, e.runtime.store)
		if err != nil {
			adkErr = err
		}
	}
	e.runtime.workflowChildMu.Unlock()
	if adkErr != nil {
		parent = e.failParent(ctx, parent, adkErr)
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	if len(approvals) > 0 {
		execution.detachDeltaSink()
		e.runtime.adkMu.Lock()
		e.runtime.adkRuns[parent.ID] = execution
		for _, child := range childRuns {
			e.runtime.adkRuns[child.ID] = execution
		}
		e.runtime.adkMu.Unlock()
	}
	if err := e.ensureWorkflowChildrenFinalReplies(ctx, req, execution, childRuns, approvals); err != nil {
		parent = e.failParent(ctx, parent, err)
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	responses, err := e.completeWorkflowChildrenFromADK(ctx, req, execution, childRuns, approvals)
	if err != nil {
		parent = e.failParent(ctx, parent, err)
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	replies := make([]string, 0, len(responses))
	var blockingChild *Run
	for index, response := range responses {
		child := response.Run
		parent.ChildRunIDs = appendUniqueString(parent.ChildRunIDs, child.ID)
		parent = updateWorkflowPlanForChildAt(parent, child, index)
		if index < len(tasks) {
			status := "DONE"
			if child.Status != RunStatusCompleted {
				status = "BLOCKED"
			}
			_, _ = e.runtime.store.UpdateTask(ctx, tasks[index].ID, TaskPatchRequest{Status: &status, RunID: &child.ID})
		}
		if strings.TrimSpace(response.Reply) != "" {
			replies = append(replies, response.Reply)
		}
		if isWorkflowBlockingStatus(child.Status) && blockingChild == nil {
			blockingChild = &child
		}
	}
	if blockingChild != nil {
		if parent.WorkMode == WorkModeLoop && parent.Iteration == 0 {
			parent.Iteration = 1
		}
		parent.Status = blockingChild.Status
		parent.Message = blockingChild.Message
		parent.WorkflowStatus = workflowStatusPaused
		parent.PendingApprovals = pendingApprovalsOnly(approvals)
		if parent.Status != RunStatusPending {
			parent.WorkflowStatus = workflowStatusFailed
			parent.FailureReason = blockingChild.FailureReason
			parent.ErrorCode = blockingChild.ErrorCode
			parent.Degraded = true
			parent.CompletedAt = new(nowString())
			finalizeRunUsage(&parent)
		}
		_ = e.runtime.store.SaveRun(ctx, parent)
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: workflowPendingReply(parent)}), nil
	}
	parent.Status = RunStatusCompleted
	parent.Message = "workflow completed"
	parent.WorkflowStatus = workflowStatusComplete
	if parent.WorkMode == WorkModeLoop && parent.Iteration == 0 {
		parent.Iteration = 1
	}
	parent.PendingApprovals = pendingApprovalsOnly(parent.PendingApprovals)
	parent.CompletedAt = new(nowString())
	finalizeRunUsage(&parent)
	_ = e.runtime.store.SaveRun(ctx, parent)
	return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: workflowSummary(parent, replies)}), nil
}

func (e *WorkflowExecutor) startWorkflowChildRuns(ctx context.Context, req workflowRequest, parent Run, steps []workflowStep, tasks []Task) ([]Run, []func(), error) {
	childRuns := make([]Run, 0, len(steps))
	finishes := make([]func(), 0, len(steps))
	for index, step := range steps {
		if index < len(tasks) {
			_, _ = e.runtime.store.UpdateTask(ctx, tasks[index].ID, TaskPatchRequest{Status: stringPtr("IN_PROGRESS")})
		}
		childAgent := req.Agent
		childAgent.WorkMode = WorkModeChat
		child, _, finishChild, err := e.runtime.startRunWithOptions(ctx, req.Session.ID, childAgent, step.Message, runStartOptions{
			WorkMode:    WorkModeChat,
			Objective:   req.Objective,
			ParentRunID: parent.ID,
			Iteration:   index + 1,
		})
		if err != nil {
			for _, finish := range finishes {
				finish()
			}
			return nil, nil, err
		}
		childRuns = append(childRuns, child)
		finishes = append(finishes, finishChild)
		if index < len(tasks) {
			_, _ = e.runtime.store.UpdateTask(ctx, tasks[index].ID, TaskPatchRequest{RunID: &child.ID})
		}
	}
	return childRuns, finishes, nil
}

func (e *WorkflowExecutor) completeWorkflowChildrenFromADK(
	ctx context.Context,
	req workflowRequest,
	execution *googleADKExecution,
	childRuns []Run,
	approvals []Approval,
) ([]ChatResponse, error) {
	responses := make([]ChatResponse, 0, len(childRuns))
	for _, child := range childRuns {
		childApprovals := approvalsForRun(approvals, child.ID)
		toolContext := execution.toolContextForRun(child.ID)
		replyResult := execution.resultForRun(child.ID)
		child = hydrateRunExecutionResult(child, toolContext, childApprovals, "", "")
		response, err := e.runtime.completeChatRun(ctx, req.Session, child, child.UserMessage, toolContext, childApprovals, replyResult, nil)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func (e *WorkflowExecutor) ensureWorkflowChildrenFinalReplies(
	ctx context.Context,
	req workflowRequest,
	execution *googleADKExecution,
	childRuns []Run,
	approvals []Approval,
) error {
	for _, child := range childRuns {
		if len(approvalsForRun(approvals, child.ID)) > 0 {
			continue
		}
		if !execution.runNeedsFinalSynthesis(child.ID) {
			continue
		}
		if err := e.runtime.runGoogleADKWorkflowChildFinalSynthesis(ctx, req.Agent, req.Session, execution, child); err != nil {
			return e.failWorkflowChildAfterMissingFinal(ctx, child, execution, err)
		}
		if execution.runNeedsFinalSynthesis(child.ID) || !execution.runHasPostToolText(child.ID) {
			return e.failWorkflowChildAfterMissingFinal(ctx, child, execution, errADKMissingFinalReply())
		}
	}
	return nil
}

func (e *WorkflowExecutor) failWorkflowChildAfterMissingFinal(
	ctx context.Context,
	child Run,
	execution *googleADKExecution,
	cause error,
) error {
	toolContext := execution.toolContextForRun(child.ID)
	child = hydrateRunExecutionResult(child, toolContext, nil, "", "")
	child = markFailedChatRun(ctx, child, cause)
	_ = e.runtime.persistRunTerminalState(context.Background(), child)
	return cause
}

func (e *WorkflowExecutor) runChild(ctx context.Context, req workflowRequest, parent Run, step workflowStep, task Task, iteration int) workflowChildResult {
	_, _ = e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{Status: stringPtr("IN_PROGRESS"), Executor: stringPtr(workflowTaskExecutorChild)})
	childAgent := req.Agent
	childAgent.WorkMode = WorkModeChat
	child, childCtx, finishChild, err := e.runtime.startRunWithOptions(ctx, req.Session.ID, childAgent, step.Message, runStartOptions{
		WorkMode:    WorkModeChat,
		Objective:   req.Objective,
		ParentRunID: parent.ID,
		Iteration:   iteration,
	})
	if err != nil {
		_, _ = e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{Status: stringPtr("BLOCKED"), RunID: stringPtr(parent.ID)})
		return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Err: err}
	}
	defer finishChild()
	_, _ = e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{RunID: &child.ID})
	parent.ChildRunIDs = appendUniqueString(parent.ChildRunIDs, child.ID)
	parent = updateWorkflowPlanForChildAt(parent, child, workflowPlanIndexForTask(parent.WorkflowPlan, task.ID))
	if err := e.runtime.store.SaveRun(ctx, parent); err != nil {
		return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Err: err}
	}
	if err := emitWorkflowRunSnapshot(req, parent); err != nil {
		return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Err: err}
	}
	e.runtime.workflowChildMu.Lock()
	childSession := req.Session
	if refreshed, ok, refreshErr := e.runtime.store.Session(ctx, req.Session.ID); refreshErr == nil && ok {
		childSession = refreshed
	}
	toolContext, approvals, replyResult, preToolContent, preToolReasoning, adkErr := e.runtime.executeGoogleADK(childCtx, childAgent, childSession, child.ID, step.Message, req.OnDelta)
	child = hydrateRunExecutionResult(child, toolContext, approvals, preToolContent, preToolReasoning)
	response, err := e.runtime.completeChatRun(ctx, childSession, child, step.Message, toolContext, approvals, replyResult, adkErr)
	e.runtime.workflowChildMu.Unlock()
	if err != nil {
		_, _ = e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{Status: stringPtr("BLOCKED")})
		return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Response: response, Err: err}
	}
	status := "DONE"
	if response.Run.Status != RunStatusCompleted {
		status = "BLOCKED"
	}
	_, _ = e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{
		Status:        &status,
		RunID:         &response.Run.ID,
		Executor:      stringPtr(workflowTaskExecutorChild),
		ResultSummary: stringPtr(strings.TrimSpace(response.Reply)),
	})
	return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Response: response}
}

func (e *WorkflowExecutor) runSelfTask(ctx context.Context, req workflowRequest, parent Run, task Task) (Run, string, bool, error) {
	_, _ = e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{
		Status:   stringPtr("IN_PROGRESS"),
		Executor: stringPtr(workflowTaskExecutorSelf),
	})
	if strings.Contains(req.Message, "@") {
		summary := workflowSelfTaskSummary(task)
		status := "DONE"
		_, err := e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{
			Status:        &status,
			Executor:      stringPtr(workflowTaskExecutorSelf),
			ResultSummary: stringPtr(summary),
		})
		if err != nil {
			return parent, "", false, err
		}
		parent.Status = RunStatusRunning
		parent.WorkflowStatus = workflowStatusRunning
		parent.Message = "workflow running"
		if saved, msgErr := e.runtime.attachFinalAssistantMessage(ctx, req.Session, parent, openAIChatResult{Reply: summary}); msgErr == nil {
			parent = saved
		} else {
			return parent, "", false, msgErr
		}
		return parent, summary, false, nil
	}
	selfAgent := req.Agent
	selfAgent.WorkMode = WorkModeChat
	if strings.Contains(req.Message, "@") && !workflowTaskShouldDelegate(task, req) {
		selfAgent.Tools = []string{"__workflow_no_tools__"}
		selfAgent.Skills = nil
	}
	prompt := workflowSelfTaskPrompt(parent, task)
	e.runtime.workflowChildMu.Lock()
	toolContext, approvals, replyResult, preToolContent, preToolReasoning, adkErr := e.runtime.executeGoogleADK(ctx, selfAgent, req.Session, parent.ID, prompt, req.OnDelta)
	e.runtime.workflowChildMu.Unlock()
	parent = hydrateRunExecutionResult(parent, toolContext, approvals, preToolContent, preToolReasoning)
	if len(approvals) > 0 {
		parent.Status = RunStatusPending
		parent.WorkflowStatus = workflowStatusPaused
		parent.Message = "等待用户审批后继续执行。"
		parent.PendingApprovals = pendingApprovalsOnly(approvals)
		_, _ = e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{
			Status:        stringPtr("BLOCKED"),
			Executor:      stringPtr(workflowTaskExecutorSelf),
			ResultSummary: stringPtr(parent.Message),
		})
		return parent, parent.Message, true, nil
	}
	if adkErr != nil {
		_, _ = e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{
			Status:        stringPtr("BLOCKED"),
			Executor:      stringPtr(workflowTaskExecutorSelf),
			ResultSummary: stringPtr(adkErr.Error()),
		})
		return parent, "", false, adkErr
	}
	reply := strings.TrimSpace(replyResult.Reply)
	if reply == "" {
		return parent, "", false, errADKMissingFinalReply()
	}
	status := "DONE"
	_, err := e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{
		Status:        &status,
		Executor:      stringPtr(workflowTaskExecutorSelf),
		ResultSummary: stringPtr(reply),
	})
	if err != nil {
		return parent, "", false, err
	}
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.Message = "workflow running"
	parent.PendingApprovals = pendingApprovalsOnly(parent.PendingApprovals)
	if saved, msgErr := e.runtime.attachFinalAssistantMessage(ctx, req.Session, parent, replyResult); msgErr == nil {
		parent = saved
	} else {
		return parent, "", false, msgErr
	}
	return parent, reply, false, nil
}

func emitWorkflowRunSnapshot(req workflowRequest, run Run) error {
	if !req.EmitRun || req.OnDelta == nil {
		return nil
	}
	snapshot := NormalizeRun(run)
	return req.OnDelta(ChatDelta{Run: &snapshot})
}

func (e *WorkflowExecutor) mergeChildResult(ctx context.Context, parent Run, child Run) Run {
	return e.mergeChildResultAt(ctx, parent, child, -1)
}

func (e *WorkflowExecutor) mergeChildResultAt(ctx context.Context, parent Run, child Run, index int) Run {
	if strings.TrimSpace(child.ID) != "" {
		parent.ChildRunIDs = appendUniqueString(parent.ChildRunIDs, child.ID)
	}
	parent = updateWorkflowPlanForChildAt(parent, child, index)
	if child.Status == RunStatusPending {
		parent.Status = RunStatusPending
		parent.PendingApprovals = append([]Approval(nil), child.PendingApprovals...)
	}
	parent.ToolCalls = append(parent.ToolCalls, child.ToolCalls...)
	parent.ToolSummaries = toolSummariesForRun(parent)
	parent.OptimizationTaskID = optimizationTaskID(parent.ToolCalls)
	_ = e.runtime.store.SaveRun(ctx, parent)
	return parent
}

func workflowPlanFromSteps(steps []workflowStep, tasks []Task) []WorkflowStepState {
	plan := make([]WorkflowStepState, 0, len(steps))
	for index, step := range steps {
		state := WorkflowStepState{
			Title:           step.Title,
			Description:     workflowStepDescription(step),
			Message:         step.Message,
			Status:          "TODO",
			DependsOn:       append([]string(nil), step.DependsOn...),
			Iteration:       index + 1,
			Order:           step.Order,
			ModeHint:        step.ModeHint,
			AgentRole:       step.AgentRole,
			PlannerStepID:   step.DependencyID,
			PlanSource:      step.PlanSource,
			WorkflowMode:    step.WorkflowMode,
			Objective:       step.Objective,
			PlannerWarnings: append([]string(nil), step.PlannerWarnings...),
		}
		if index < len(tasks) {
			state.TaskID = tasks[index].ID
			state.Message = defaultString(tasks[index].Message, state.Message)
			state.Status = defaultString(tasks[index].Status, state.Status)
			state.DependsOn = append([]string(nil), tasks[index].DependsOn...)
			state.Order = tasks[index].Order
			state.ModeHint = tasks[index].ModeHint
			state.AgentRole = tasks[index].AgentRole
			state.PlannerStepID = tasks[index].PlannerStepID
			state.PlanSource = tasks[index].PlanSource
			state.WorkflowMode = tasks[index].WorkflowMode
			state.Objective = tasks[index].Objective
			state.Executor = tasks[index].Executor
			state.ResultSummary = tasks[index].ResultSummary
			state.PlannerWarnings = append([]string(nil), tasks[index].PlannerWarnings...)
		}
		plan = append(plan, state)
	}
	return plan
}

func workflowPlanFromTasks(tasks []Task, existing []WorkflowStepState) []WorkflowStepState {
	existingByTaskID := make(map[string]WorkflowStepState, len(existing))
	for _, state := range existing {
		if strings.TrimSpace(state.TaskID) != "" {
			existingByTaskID[state.TaskID] = state
		}
	}
	ordered := append([]Task(nil), tasks...)
	sortWorkflowTasks(ordered)
	plan := make([]WorkflowStepState, 0, len(ordered))
	for index, task := range ordered {
		prior := existingByTaskID[task.ID]
		state := WorkflowStepState{
			TaskID:          task.ID,
			Title:           task.Title,
			Description:     task.Description,
			Message:         task.Message,
			Status:          defaultString(task.Status, "TODO"),
			DependsOn:       append([]string(nil), task.DependsOn...),
			Iteration:       index + 1,
			Order:           task.Order,
			ModeHint:        task.ModeHint,
			AgentRole:       task.AgentRole,
			PlannerStepID:   task.PlannerStepID,
			PlanSource:      task.PlanSource,
			WorkflowMode:    task.WorkflowMode,
			Objective:       task.Objective,
			Executor:        task.Executor,
			ResultSummary:   task.ResultSummary,
			PlannerWarnings: append([]string(nil), task.PlannerWarnings...),
		}
		if strings.TrimSpace(state.Title) == "" {
			state.Title = prior.Title
		}
		if strings.TrimSpace(state.Description) == "" {
			state.Description = prior.Description
		}
		if strings.TrimSpace(state.Message) == "" {
			state.Message = prior.Message
		}
		if strings.TrimSpace(state.PlanSource) == "" {
			state.PlanSource = prior.PlanSource
		}
		if strings.TrimSpace(state.WorkflowMode) == "" {
			state.WorkflowMode = prior.WorkflowMode
		}
		if strings.TrimSpace(state.Objective) == "" {
			state.Objective = prior.Objective
		}
		state.ChildRunID = prior.ChildRunID
		if task.Executor == workflowTaskExecutorChild && strings.TrimSpace(task.RunID) != "" {
			state.ChildRunID = task.RunID
		}
		plan = append(plan, state)
	}
	return plan
}

func (e *WorkflowExecutor) workflowTasks(ctx context.Context, parent Run, known []Task) ([]Task, error) {
	byID := make(map[string]Task, len(known)+len(parent.WorkflowPlan))
	for _, task := range known {
		if strings.TrimSpace(task.ID) != "" {
			byID[task.ID] = task
		}
	}
	for _, state := range parent.WorkflowPlan {
		if strings.TrimSpace(state.TaskID) == "" {
			continue
		}
		task, ok, err := e.runtime.store.Task(ctx, state.TaskID)
		if err != nil {
			return nil, err
		}
		if ok {
			byID[task.ID] = task
		}
	}
	parentTasks, _, err := e.runtime.store.ListTasksPage(ctx, "", "", parent.ID, 1000, 0)
	if err != nil {
		return nil, err
	}
	for _, task := range parentTasks {
		byID[task.ID] = task
	}
	tasks := make([]Task, 0, len(byID))
	for _, task := range byID {
		tasks = append(tasks, task)
	}
	sortWorkflowTasks(tasks)
	return tasks, nil
}

func workflowTasksComplete(tasks []Task) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, task := range tasks {
		if task.Status != "DONE" {
			return false
		}
	}
	return true
}

func firstTerminalWorkflowTask(tasks []Task) (Task, bool) {
	for _, task := range tasks {
		if task.Status == "BLOCKED" || task.Status == "CANCELLED" {
			return task, true
		}
	}
	return Task{}, false
}

func executableWorkflowTasks(tasks []Task, _ string) []Task {
	taskByID := make(map[string]Task, len(tasks))
	for _, task := range tasks {
		taskByID[task.ID] = task
	}
	ready := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if task.Status != "TODO" {
			continue
		}
		depsDone := true
		for _, dep := range task.DependsOn {
			depTask, ok := taskByID[dep]
			if !ok || depTask.Status != "DONE" {
				depsDone = false
				break
			}
		}
		if depsDone {
			ready = append(ready, task)
		}
	}
	sortWorkflowTasks(ready)
	if len(ready) > 1 {
		return ready[:1]
	}
	return ready
}

func workflowTaskShouldDelegate(task Task, req workflowRequest) bool {
	if strings.EqualFold(strings.TrimSpace(task.Executor), workflowTaskExecutorChild) {
		return true
	}
	hint := strings.ToLower(strings.TrimSpace(task.ModeHint + " " + task.AgentRole))
	if strings.Contains(hint, "child") || strings.Contains(hint, "delegate") || strings.Contains(hint, "子智能体") || strings.Contains(hint, "子agent") {
		return true
	}
	text := strings.ToLower(task.Message + "\n" + task.Description + "\n" + task.Title)
	if len(req.Agent.Tools) > 0 && workflowTaskLooksMutating(text) {
		return true
	}
	return strings.Contains(text, "@") || strings.Contains(text, "调用工具") || strings.Contains(text, "使用工具")
}

func workflowTaskLooksMutating(text string) bool {
	for _, marker := range []string{
		"保存", "提交", "写入", "创建", "新增", "删除", "更新", "修改", "下单", "交易",
		"save", "submit", "write", "create", "delete", "update", "modify", "order", "trade",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func workflowStepFromTask(task Task) workflowStep {
	message := strings.TrimSpace(task.Message)
	if message == "" {
		message = defaultString(task.Description, task.Title)
	}
	return workflowStep{
		Order:           task.Order,
		DependencyID:    task.PlannerStepID,
		Title:           task.Title,
		Description:     workflowDescriptionWithoutAgentRole(task.Description),
		Message:         message,
		DependsOn:       append([]string(nil), task.DependsOn...),
		AgentRole:       task.AgentRole,
		ModeHint:        task.ModeHint,
		Objective:       task.Objective,
		PlanSource:      task.PlanSource,
		WorkflowMode:    task.WorkflowMode,
		PlannerWarnings: append([]string(nil), task.PlannerWarnings...),
	}
}

func workflowTaskIteration(task Task) int {
	if task.Order > 0 {
		return task.Order
	}
	return 1
}

func workflowPlanIndexForTask(plan []WorkflowStepState, taskID string) int {
	for index, state := range plan {
		if state.TaskID == taskID {
			return index
		}
	}
	return -1
}

func workflowSelfTaskSummary(task Task) string {
	if strings.TrimSpace(task.ResultSummary) != "" {
		return task.ResultSummary
	}
	subject := strings.TrimSpace(task.Title)
	if subject == "" {
		subject = "任务"
	}
	detail := strings.TrimSpace(task.Description)
	if detail == "" {
		detail = strings.TrimSpace(task.Message)
	}
	if detail == "" {
		return subject + " 已由父智能体完成。"
	}
	if len([]rune(detail)) > 120 {
		detail = string([]rune(detail)[:120]) + "..."
	}
	return fmt.Sprintf("%s 已由父智能体完成：%s", subject, detail)
}

func workflowSelfTaskPrompt(parent Run, task Task) string {
	var builder strings.Builder
	builder.WriteString("你是当前 ADK workflow 的父智能体调度员。请你亲自完成下面这个 TODO，不要创建子智能体。\n\n")
	if strings.TrimSpace(parent.Objective) != "" {
		builder.WriteString("总体目标：")
		builder.WriteString(strings.TrimSpace(parent.Objective))
		builder.WriteString("\n\n")
	}
	if strings.TrimSpace(task.Title) != "" {
		builder.WriteString("当前 TODO：")
		builder.WriteString(strings.TrimSpace(task.Title))
		builder.WriteString("\n")
	}
	if strings.TrimSpace(task.Description) != "" {
		builder.WriteString("任务说明：")
		builder.WriteString(strings.TrimSpace(task.Description))
		builder.WriteString("\n")
	}
	if strings.TrimSpace(task.Message) != "" {
		builder.WriteString("执行提示：")
		builder.WriteString(strings.TrimSpace(task.Message))
		builder.WriteString("\n")
	}
	if len(task.DependsOn) > 0 {
		builder.WriteString("前置任务 ID：")
		builder.WriteString(strings.Join(task.DependsOn, ", "))
		builder.WriteString("\n")
	}
	builder.WriteString("\n请输出这个 TODO 的可见结果。若需要工具且当前 agent 具备工具，可直接调用工具；若确实需要后续 TODO，请在回复中说明建议新增的任务。")
	return strings.TrimSpace(builder.String())
}

func updateWorkflowPlanForTask(plan []WorkflowStepState, task Task) []WorkflowStepState {
	index := workflowPlanIndexForTask(plan, task.ID)
	if index < 0 {
		return workflowPlanFromTasks([]Task{task}, plan)
	}
	plan[index].Title = task.Title
	plan[index].Description = task.Description
	plan[index].Message = task.Message
	plan[index].Status = task.Status
	plan[index].DependsOn = append([]string(nil), task.DependsOn...)
	plan[index].Order = task.Order
	plan[index].ModeHint = task.ModeHint
	plan[index].AgentRole = task.AgentRole
	plan[index].PlannerStepID = task.PlannerStepID
	plan[index].PlanSource = task.PlanSource
	plan[index].WorkflowMode = task.WorkflowMode
	plan[index].Objective = task.Objective
	plan[index].Executor = task.Executor
	plan[index].ResultSummary = task.ResultSummary
	plan[index].PlannerWarnings = append([]string(nil), task.PlannerWarnings...)
	if task.Executor == workflowTaskExecutorChild && strings.TrimSpace(task.RunID) != "" {
		plan[index].ChildRunID = task.RunID
	}
	return plan
}

type workflowRuntimeTaskRequest struct {
	Title       string
	Message     string
	Description string
	DependsOn   []string
	AgentRole   string
	ModeHint    string
}

func (e *WorkflowExecutor) addRuntimeWorkflowTask(ctx context.Context, parent Run, current Task, req workflowRuntimeTaskRequest) (Task, error) {
	tasks, err := e.workflowTasks(ctx, parent, nil)
	if err != nil {
		return Task{}, err
	}
	runtimeCount := 0
	maxOrder := 0
	taskIDs := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		taskIDs[task.ID] = true
		if task.PlanSource == workflowPlanSourceRuntime {
			runtimeCount++
		}
		if task.Order > maxOrder {
			maxOrder = task.Order
		}
	}
	if runtimeCount >= maxRuntimeWorkflowTasks {
		return Task{}, fmt.Errorf("runtime workflow task limit reached")
	}
	title := strings.TrimSpace(req.Title)
	message := strings.TrimSpace(req.Message)
	description := strings.TrimSpace(req.Description)
	if title == "" {
		title = message
	}
	if title == "" {
		return Task{}, fmt.Errorf("runtime task title is required")
	}
	if message == "" {
		message = defaultString(description, title)
	}
	dependsOn := normalizeStringSlice(req.DependsOn)
	for _, dep := range dependsOn {
		if !taskIDs[dep] {
			return Task{}, fmt.Errorf("runtime task dependency not found: %s", dep)
		}
	}
	nextRuntime := runtimeCount + 1
	task, err := e.runtime.store.SaveTask(ctx, TaskWriteRequest{
		Title:         title,
		Description:   description,
		Message:       message,
		Status:        "TODO",
		AgentID:       parent.AgentID,
		RunID:         parent.ID,
		DependsOn:     dependsOn,
		Order:         maxOrder + 1,
		ModeHint:      req.ModeHint,
		AgentRole:     req.AgentRole,
		PlannerStepID: fmt.Sprintf("runtime-%d", nextRuntime),
		PlanSource:    workflowPlanSourceRuntime,
		WorkflowMode:  parent.WorkMode,
		Objective:     parent.Objective,
	})
	if err != nil {
		return Task{}, err
	}
	tasks = append(tasks, task)
	if workflowTasksHaveCycle(tasks) {
		_ = e.runtime.store.DeleteTask(ctx, task.ID)
		return Task{}, fmt.Errorf("runtime task dependencies contain a cycle")
	}
	return task, nil
}

func workflowTasksHaveCycle(tasks []Task) bool {
	graph := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		graph[task.ID] = append([]string(nil), task.DependsOn...)
	}
	visiting := make(map[string]bool, len(tasks))
	visited := make(map[string]bool, len(tasks))
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, dep := range graph[id] {
			if graph[dep] == nil {
				continue
			}
			if visit(dep) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for id := range graph {
		if visit(id) {
			return true
		}
	}
	return false
}

func sortWorkflowTasks(tasks []Task) {
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Order != tasks[j].Order {
			if tasks[i].Order == 0 {
				return false
			}
			if tasks[j].Order == 0 {
				return true
			}
			return tasks[i].Order < tasks[j].Order
		}
		if tasks[i].CreatedAt != tasks[j].CreatedAt {
			return tasks[i].CreatedAt < tasks[j].CreatedAt
		}
		return tasks[i].ID < tasks[j].ID
	})
}

func workflowStepDescription(step workflowStep) string {
	description := strings.TrimSpace(step.Description)
	if strings.TrimSpace(step.AgentRole) == "" {
		return description
	}
	role := "Agent role: " + strings.TrimSpace(step.AgentRole)
	if description == "" {
		return role
	}
	return description + "\n\n" + role
}

func workflowDescriptionWithoutAgentRole(description string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return ""
	}
	if index := strings.LastIndex(description, "\n\nAgent role:"); index >= 0 {
		return strings.TrimSpace(description[:index])
	}
	if strings.HasPrefix(description, "Agent role:") {
		return ""
	}
	return description
}

func pauseParentForChild(parent Run, child Run, cursor int) Run {
	parent.Status = child.Status
	parent.Message = child.Message
	parent.PendingApprovals = pendingApprovalsOnly(child.PendingApprovals)
	parent.WorkflowStatus = workflowStatusPaused
	parent.WorkflowCursor = cursor
	parent = updateWorkflowPlanForChildAt(parent, child, cursor)
	return parent
}

func childRunIDs(children []Run) []string {
	ids := make([]string, 0, len(children))
	for _, child := range children {
		ids = appendUniqueString(ids, child.ID)
	}
	return ids
}

func approvalsForRun(approvals []Approval, runID string) []Approval {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil
	}
	filtered := make([]Approval, 0, len(approvals))
	for _, approval := range approvals {
		if approval.RunID == runID {
			filtered = append(filtered, approval)
		}
	}
	return pendingApprovalsOnly(filtered)
}

func workflowPendingReply(parent Run) string {
	if parent.Status != RunStatusPending {
		if strings.TrimSpace(parent.FailureReason) != "" {
			return parent.FailureReason
		}
		return parent.Message
	}
	switch parent.WorkMode {
	case WorkModeTask:
		return "任务编排正在等待审批。"
	case WorkModeLoop:
		return "目标模式正在等待审批。"
	default:
		return "工作流正在等待审批。"
	}
}

func updateWorkflowPlanForChild(parent Run, child Run) Run {
	return updateWorkflowPlanForChildAt(parent, child, -1)
}

func updateWorkflowPlanForChildAt(parent Run, child Run, stepIndex int) Run {
	if len(parent.WorkflowPlan) == 0 || strings.TrimSpace(child.ID) == "" {
		return parent
	}
	if stepIndex >= 0 && stepIndex < len(parent.WorkflowPlan) {
		applyWorkflowChildState(&parent.WorkflowPlan[stepIndex], child)
		parent.WorkflowCursor = stepIndex
		return parent
	}
	matched := false
	for index := range parent.WorkflowPlan {
		step := &parent.WorkflowPlan[index]
		if step.ChildRunID == child.ID {
			matched = true
		}
	}
	for index := range parent.WorkflowPlan {
		step := &parent.WorkflowPlan[index]
		if matched && step.ChildRunID != child.ID {
			continue
		}
		if !matched {
			break
		}
		applyWorkflowChildState(step, child)
		parent.WorkflowCursor = index
		break
	}
	return parent
}

func applyWorkflowChildState(step *WorkflowStepState, child Run) {
	if step == nil {
		return
	}
	step.ChildRunID = child.ID
	step.Executor = workflowTaskExecutorChild
	switch child.Status {
	case RunStatusCompleted:
		step.Status = "DONE"
		step.ResultSummary = strings.TrimSpace(child.Message)
	case RunStatusPending:
		step.Status = "BLOCKED"
	case RunStatusRunning:
		step.Status = "IN_PROGRESS"
	default:
		step.Status = "BLOCKED"
	}
	if child.Iteration > 0 {
		step.Iteration = child.Iteration
	}
}

func workflowStepFromState(state WorkflowStepState) workflowStep {
	return workflowStep{
		Order:           state.Order,
		DependencyID:    state.PlannerStepID,
		Title:           state.Title,
		Description:     workflowDescriptionWithoutAgentRole(state.Description),
		Message:         state.Message,
		DependsOn:       append([]string(nil), state.DependsOn...),
		AgentRole:       state.AgentRole,
		ModeHint:        state.ModeHint,
		Objective:       state.Objective,
		PlanSource:      state.PlanSource,
		WorkflowMode:    state.WorkflowMode,
		PlannerWarnings: append([]string(nil), state.PlannerWarnings...),
	}
}

func taskFromWorkflowStepState(state WorkflowStepState, parent Run) Task {
	return Task{
		ID:              state.TaskID,
		Title:           state.Title,
		Description:     state.Description,
		Message:         state.Message,
		Status:          state.Status,
		AgentID:         parent.AgentID,
		RunID:           parent.ID,
		DependsOn:       append([]string(nil), state.DependsOn...),
		Order:           state.Order,
		ModeHint:        state.ModeHint,
		AgentRole:       state.AgentRole,
		PlannerStepID:   state.PlannerStepID,
		PlanSource:      state.PlanSource,
		WorkflowMode:    state.WorkflowMode,
		Objective:       state.Objective,
		Executor:        state.Executor,
		ResultSummary:   state.ResultSummary,
		PlannerWarnings: append([]string(nil), state.PlannerWarnings...),
		CreatedAt:       parent.CreatedAt,
		UpdatedAt:       parent.UpdatedAt,
	}
}

func (e *WorkflowExecutor) failParent(ctx context.Context, parent Run, cause error) Run {
	parent.Status = runStatusForContext(ctx, cause)
	parent.Message = userFacingADKError(cause)
	parent.FailureReason = userFacingADKError(cause)
	parent.ErrorCode = runErrorCode(parent.Status)
	parent.WorkflowStatus = workflowStatusFailed
	parent.Degraded = true
	parent.CompletedAt = new(nowString())
	if parent.Status == RunStatusCancelled {
		parent.CancelledAt = parent.CompletedAt
	}
	finalizeRunUsage(&parent)
	_ = e.runtime.store.SaveRun(context.Background(), parent)
	return parent
}

func (e *WorkflowExecutor) workflowResponse(ctx context.Context, session Session, parent Run, replyResult openAIChatResult) ChatResponse {
	response := e.runtime.projectedChatResponse(ctx, session, parent, parent.PendingApprovals, replyResult)
	response.Run = NormalizeRun(parent)
	response.PendingApprovals = pendingApprovalsOnly(parent.PendingApprovals)
	return NormalizeChatResponse(response)
}

func workflowSummary(parent Run, replies []string) string {
	var builder strings.Builder
	switch parent.WorkMode {
	case WorkModeTask:
		builder.WriteString("任务编排已完成。")
	case WorkModeLoop:
		builder.WriteString("目标模式已完成。")
	default:
		builder.WriteString("工作流已完成。")
	}
	if strings.TrimSpace(parent.Objective) != "" {
		builder.WriteString("\n\n目标：")
		builder.WriteString(parent.Objective)
	}
	if len(parent.ChildRunIDs) > 0 {
		builder.WriteString(fmt.Sprintf("\n\n子运行：%d 个", len(parent.ChildRunIDs)))
	}
	if len(replies) > 0 {
		builder.WriteString("\n\n结果摘要：")
		for _, reply := range replies {
			reply = strings.TrimSpace(reply)
			if reply == "" {
				continue
			}
			builder.WriteString("\n- ")
			if len([]rune(reply)) > 180 {
				reply = string([]rune(reply)[:180]) + "..."
			}
			builder.WriteString(reply)
		}
	}
	return builder.String()
}

func workflowReplyLooksComplete(reply string) bool {
	lower := strings.ToLower(reply)
	return strings.Contains(lower, "workflow_done") || strings.Contains(reply, "目标已完成") || strings.Contains(reply, "已完成")
}

func errADKMissingFinalReply() error {
	return fmt.Errorf("工具调用完成后模型未返回最终回复")
}

func isWorkflowBlockingStatus(status string) bool {
	switch status {
	case RunStatusPending, RunStatusFailed, RunStatusTimedOut, RunStatusCancelled, RunStatusDenied:
		return true
	default:
		return false
	}
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func stringPtr(value string) *string {
	return &value
}
