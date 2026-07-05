package adk

import (
	"context"
	"fmt"
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
	ChildProviderID string
	ChildModel      string
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
		parent, jftradeErr7 := e.runtime.saveRunPreservingUserGoalPause(parentCtx, parent)
		jftradeLogError(jftradeErr7)
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
	jftradeErr4 := e.runtime.store.SaveRun(parentCtx, parent)
	jftradeLogError(jftradeErr4)
	if mode == WorkModeTask {
		return e.runADKTaskWorkflow(parentCtx, req, parent, tasks)
	}
	return e.runGoogleADKWorkflow(parentCtx, req, parent, steps, tasks)
}

func (e *WorkflowExecutor) createInitialGoalTask(ctx context.Context, parent Run, agent Agent, objective string, message string) (Task, error) {
	return e.runtime.store.SaveTask(ctx, TaskWriteRequest{
		Title:        "推进当前目标",
		Description:  "目标模式初始任务",
		Status:       "TODO",
		AgentID:      agent.ID,
		RunID:        parent.ID,
		Order:        1,
		ModeHint:     WorkModeTask,
		PlanSource:   workflowPlanSourceRuntime,
		WorkflowMode: WorkModeLoop,
		Message:      "分析当前目标并维护后续执行步骤。",
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
	for index := range steps {
		if steps[index].Order <= 0 {
			steps[index].Order = index + 1
		}
		if strings.TrimSpace(steps[index].WorkflowMode) == "" {
			steps[index].WorkflowMode = normalizedMode
		}
		steps[index].Objective = ""
		if strings.TrimSpace(steps[index].PlanSource) == "" {
			steps[index].PlanSource = workflowPlanSourcePlanner
		}
		if len(normalizedWarnings) > 0 {
			steps[index].PlannerWarnings = append([]string(nil), normalizedWarnings...)
		}
		steps[index] = sanitizeWorkflowPlanStep(steps[index], objective, index)
	}
	return steps
}

func sanitizeWorkflowPlanStep(step workflowStep, userRequest string, index int) workflowStep {
	original := strings.TrimSpace(userRequest)
	if original == "" {
		return step
	}
	if strings.TrimSpace(step.Title) == original {
		step.Title = fmt.Sprintf("执行计划步骤 %d", index+1)
	}
	if strings.TrimSpace(step.Description) == original {
		step.Description = ""
	}
	if strings.TrimSpace(step.Message) == original {
		if description := strings.TrimSpace(step.Description); description != "" {
			step.Message = description
		} else {
			step.Message = fmt.Sprintf("推进计划中的第 %d 步。", index+1)
		}
	}
	return step
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
			ChildProviderID: step.ChildProviderID,
			ChildModel:      step.ChildModel,
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

func (e *WorkflowExecutor) runPlannedGoogleADKWorkflow(ctx context.Context, req workflowRequest, parent Run, steps []workflowStep, tasks []Task) (ChatResponse, error) {
	childRuns, finishes, err := e.startWorkflowChildRuns(ctx, req, parent, steps, tasks)
	defer finishWorkflowChildren(finishes)
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
	if err := emitWorkflowRunSnapshot(ctx, e.runtime, req, parent); err != nil {
		return ChatResponse{}, err
	}
	if err := e.runtime.maybeAutoCompactSessionDuringWorkflow(ctx, req.Session, req.Agent, req.Message, req.OnDelta); err != nil {
		parent = e.failParent(ctx, parent, err)
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
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
	if err := e.ensureWorkflowChildrenFinalReplies(ctx, req, execution, childRuns, steps, approvals); err != nil {
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
			_, jftradeErr12 := e.runtime.store.UpdateTask(ctx, tasks[index].ID, TaskPatchRequest{Status: &status, RunID: &child.ID})
			jftradeLogError(jftradeErr12)
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
		jftradeErr2 := e.runtime.store.SaveRun(ctx, parent)
		jftradeLogError(jftradeErr2)
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: workflowPendingReply(parent)}), nil
	}
	parent.Status = RunStatusCompleted
	parent.Message = "workflow completed"
	parent.WorkflowStatus = workflowStatusComplete
	if parent.WorkMode == WorkModeLoop && parent.Iteration == 0 {
		parent.Iteration = 1
	}
	parent.PendingApprovals = nil
	parent.CompletedAt = new(nowString())
	finalizeRunUsage(&parent)
	jftradeErr1 := e.runtime.store.SaveRun(ctx, parent)
	jftradeLogError(jftradeErr1)
	return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: workflowSummary(parent, replies)}), nil
}

func (e *WorkflowExecutor) failParent(ctx context.Context, parent Run, cause error) Run {
	if tasks, taskErr := e.workflowTasks(context.Background(), parent, nil); taskErr == nil && len(tasks) > 0 {
		parent.WorkflowPlan = workflowPlanFromTasks(tasks, parent.WorkflowPlan)
	}
	parent.Status = runStatusForContext(ctx, cause)
	parent.Message = userFacingADKError(cause)
	parent.FailureReason = userFacingADKError(cause)
	parent.ErrorCode = runErrorCode(parent.Status)
	parent.WorkflowStatus = workflowStatusFailed
	parent.Degraded = true
	parent.PendingApprovals = nil
	parent.CompletedAt = new(nowString())
	if parent.Status == RunStatusCancelled {
		parent.CancelledAt = parent.CompletedAt
	}
	finalizeRunUsage(&parent)
	if err := e.runtime.store.SaveRunAndDenyPendingApprovals(context.Background(), parent); err != nil {
		jftradeLogError(err)
	} else {
		e.runtime.cancelUnfinishedWorkflowChildren(context.Background(), parent)
	}
	return parent
}

func (e *WorkflowExecutor) workflowResponse(ctx context.Context, session Session, parent Run, replyResult openAIChatResult) ChatResponse {
	return e.runtime.projectedChatResponse(ctx, session, parent, replyResult)
}
