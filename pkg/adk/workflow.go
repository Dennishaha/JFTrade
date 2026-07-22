package adk

import (
	"context"
	"fmt"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"google.golang.org/genai"
)

const (
	workflowStatusRunning  = "RUNNING"
	workflowStatusPaused   = "PAUSED"
	workflowStatusComplete = "COMPLETED"
	workflowStatusFailed   = "FAILED"

	workflowPlanSourcePlanner = "planner"
	workflowPlanSourceRuntime = "runtime"
	workflowPlanSourceCanvas  = "canvas"

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
	Order               int
	DependencyID        string
	Title               string
	Description         string
	Message             string
	DependsOn           []string
	AgentRole           string
	ChildAgentID        string
	ChildProviderID     string
	ChildModel          string
	ChildPermissionMode string
	ModeHint            string
	Objective           string
	PlanSource          string
	WorkflowMode        string
	PlannerWarnings     []string
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

func workflowEngineForMode(mode string) string {
	switch normalizeWorkMode(mode) {
	case WorkModeLoop:
		return WorkflowEngineADK2Loop
	default:
		return ""
	}
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
		WorkflowEngine: workflowEngineForMode(mode),
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
	task, err := e.createInitialGoalTask(parentCtx, parent, req.Agent, objective, req.Message)
	if err != nil {
		parent, persistErr := e.failParent(parentCtx, parent, err)
		if persistErr != nil {
			return ChatResponse{}, persistErr
		}
		return e.workflowResponse(parentCtx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
	}
	parent.WorkflowPlan = workflowPlanFromTasks([]Task{task}, parent.WorkflowPlan)
	parent, err = e.runtime.saveRunPreservingUserGoalPause(parentCtx, parent)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("persist initial goal workflow state: %w", err)
	}
	return e.runADKGoalWorkflow(parentCtx, req, parent, []Task{task})
}

func (e *WorkflowExecutor) createInitialGoalTask(ctx context.Context, parent Run, agent Agent, objective string, message string) (Task, error) {
	return e.runtime.store.SaveTask(ctx, TaskWriteRequest{
		Title:        "推进当前目标",
		Description:  "目标模式初始任务",
		Status:       "TODO",
		AgentID:      agent.ID,
		RunID:        parent.ID,
		Order:        1,
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
			Title:               step.Title,
			Description:         description,
			Status:              "TODO",
			AgentID:             agent.ID,
			RunID:               parent.ID,
			DependsOn:           dependsOn,
			Order:               step.Order,
			ModeHint:            step.ModeHint,
			AgentRole:           step.AgentRole,
			ChildAgentID:        step.ChildAgentID,
			ChildProviderID:     step.ChildProviderID,
			ChildModel:          step.ChildModel,
			ChildPermissionMode: step.ChildPermissionMode,
			PlannerStepID:       step.DependencyID,
			PlanSource:          step.PlanSource,
			WorkflowMode:        step.WorkflowMode,
			Objective:           step.Objective,
			Message:             step.Message,
			PlannerWarnings:     step.PlannerWarnings,
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

type workflowExecutionResult struct {
	execution     *googleADKExecution
	approvals     []Approval
	inputRequests map[string]*InputRequest
}

func (e *WorkflowExecutor) runNativeTaskGraphWorkflow(ctx context.Context, req workflowRequest, parent Run, steps []workflowStep, tasks []Task) (ChatResponse, error) {
	childRuns, finishes, err := e.startWorkflowChildRuns(ctx, req, parent, steps, tasks)
	defer finishWorkflowChildren(finishes)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	parent, err = e.prepareWorkflowParent(ctx, req, parent, childRuns)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	execution, err := e.runtime.newGoogleADKWorkflowExecution(ctx, req.Agent, req.Session, parent, childRuns, steps, parent.WorkMode, req.RunOptions, req.OnDelta)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	executionResult, parent, err := e.executeStartedWorkflowGraph(ctx, req, parent, childRuns, steps, execution)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	if len(executionResult.inputRequests) > 0 {
		return e.finishWorkflowPendingInputs(ctx, req, parent, tasks, childRuns, executionResult)
	}
	if err := e.ensureWorkflowChildrenFinalReplies(ctx, req, executionResult.execution, childRuns, steps, executionResult.approvals); err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	responses, err := e.completeWorkflowChildrenFromADK(ctx, req, executionResult.execution, childRuns, executionResult.approvals)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	return e.finalizePlannedWorkflow(ctx, req, parent, tasks, responses, executionResult.approvals)
}

func (e *WorkflowExecutor) runPlannedGoogleADKWorkflow(ctx context.Context, req workflowRequest, parent Run, steps []workflowStep, tasks []Task) (ChatResponse, error) {
	childRuns, finishes, err := e.startWorkflowChildRuns(ctx, req, parent, steps, tasks)
	defer finishWorkflowChildren(finishes)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	parent, err = e.prepareWorkflowParent(ctx, req, parent, childRuns)
	if err != nil {
		return ChatResponse{}, err
	}
	executionResult, parent, err := e.runWorkflowExecution(ctx, req, parent, childRuns, steps)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	if len(executionResult.inputRequests) > 0 {
		return e.finishWorkflowPendingInputs(ctx, req, parent, tasks, childRuns, executionResult)
	}
	if err := e.ensureWorkflowChildrenFinalReplies(ctx, req, executionResult.execution, childRuns, steps, executionResult.approvals); err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	responses, err := e.completeWorkflowChildrenFromADK(ctx, req, executionResult.execution, childRuns, executionResult.approvals)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	return e.finalizePlannedWorkflow(ctx, req, parent, tasks, responses, executionResult.approvals)
}

func (e *WorkflowExecutor) failedWorkflowResponse(ctx context.Context, req workflowRequest, parent Run, cause error) (ChatResponse, error) {
	parent, err := e.failParent(ctx, parent, cause)
	if err != nil {
		return ChatResponse{}, err
	}
	return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
}

func (e *WorkflowExecutor) prepareWorkflowParent(ctx context.Context, req workflowRequest, parent Run, childRuns []Run) (Run, error) {
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = WorkflowEngineADK2Loop
	}
	parent.ChildRunIDs = childRunIDs(childRuns)
	for index, child := range childRuns {
		if index < len(parent.WorkflowPlan) {
			applyWorkflowChildState(&parent.WorkflowPlan[index], child)
			if strings.TrimSpace(parent.WorkflowPlan[index].NodeName) == "" {
				parent.WorkflowPlan[index].NodeName = googleADKWorkflowChildName(parent.ID, index)
			}
		}
	}
	if err := e.runtime.store.SaveRun(ctx, parent); err != nil {
		return parent, err
	}
	if err := emitWorkflowRunSnapshot(ctx, e.runtime, req, parent); err != nil {
		return parent, err
	}
	if err := e.runtime.maybeAutoCompactSessionDuringWorkflow(ctx, req.Session, req.Agent, req.Message, req.OnDelta); err != nil {
		return parent, err
	}
	return parent, nil
}

func (e *WorkflowExecutor) runWorkflowExecution(ctx context.Context, req workflowRequest, parent Run, childRuns []Run, steps []workflowStep) (workflowExecutionResult, Run, error) {
	execution, err := e.runtime.newGoogleADKWorkflowExecution(ctx, req.Agent, req.Session, parent, childRuns, steps, parent.WorkMode, req.RunOptions, req.OnDelta)
	if err != nil {
		return workflowExecutionResult{}, parent, err
	}
	return e.executeStartedWorkflowGraph(ctx, req, parent, childRuns, steps, execution)
}

func (e *WorkflowExecutor) executeStartedWorkflowGraph(ctx context.Context, req workflowRequest, parent Run, childRuns []Run, steps []workflowStep, execution *googleADKExecution) (workflowExecutionResult, Run, error) {
	approvals, inputRequests, err := e.executeWorkflowRun(ctx, req.Message, parent, childRuns, execution)
	if err != nil {
		return workflowExecutionResult{}, parent, err
	}
	return workflowExecutionResult{execution: execution, approvals: approvals, inputRequests: inputRequests}, parent, nil
}

func (e *WorkflowExecutor) executeWorkflowRun(ctx context.Context, message string, parent Run, childRuns []Run, execution *googleADKExecution) ([]Approval, map[string]*InputRequest, error) {
	e.runtime.workflowChildMu.Lock()
	adkErr := execution.run(ctx, genai.NewContentFromText(message, genai.RoleUser))
	var approvals []Approval
	if adkErr == nil {
		approvals, adkErr = execution.pendingApprovals(ctx, e.runtime.store)
	}
	var inputRequests map[string]*InputRequest
	if adkErr == nil {
		inputRequests, adkErr = e.runtime.pendingInputRequests(ctx, execution)
		execution.setInputRequests(inputRequests)
	}
	e.runtime.workflowChildMu.Unlock()
	if adkErr != nil {
		return nil, nil, adkErr
	}
	if len(approvals) > 0 || len(inputRequests) > 0 {
		e.registerWorkflowExecution(parent, childRuns, execution)
	}
	return approvals, inputRequests, nil
}

func (e *WorkflowExecutor) finishWorkflowPendingInputs(ctx context.Context, req workflowRequest, parent Run, tasks []Task, childRuns []Run, result workflowExecutionResult) (ChatResponse, error) {
	responses := make([]ChatResponse, 0, len(childRuns))
	for _, child := range childRuns {
		request := result.inputRequests[child.ID]
		if request == nil {
			responses = append(responses, e.workflowResponse(ctx, req.Session, child, openAIChatResult{}))
			continue
		}
		toolContext := result.execution.toolContextForRun(child.ID)
		child = hydrateRunExecutionResult(child, toolContext, nil, child.PreToolContent, child.PreToolReasoning)
		childCtx, err := e.runtime.activeRunExecutionContext(ctx, child.ID)
		if err != nil {
			return e.failedWorkflowResponse(ctx, req, parent, err)
		}
		response, err := e.runtime.finishPendingInputRun(childCtx, req.Session, child, request)
		if err != nil {
			return e.failedWorkflowResponse(ctx, req, parent, err)
		}
		responses = append(responses, response)
	}
	return e.finalizePlannedWorkflow(ctx, req, parent, tasks, responses, nil)
}

func (e *WorkflowExecutor) registerWorkflowExecution(parent Run, childRuns []Run, execution *googleADKExecution) {
	execution.detachDeltaSink()
	e.runtime.adkMu.Lock()
	defer e.runtime.adkMu.Unlock()
	e.runtime.adkRuns[parent.ID] = execution
	for _, child := range childRuns {
		e.runtime.adkRuns[child.ID] = execution
	}
}

func (e *WorkflowExecutor) finalizePlannedWorkflow(ctx context.Context, req workflowRequest, parent Run, tasks []Task, responses []ChatResponse, approvals []Approval) (ChatResponse, error) {
	replies, blockingChild, parent, err := e.applyWorkflowChildResponses(ctx, parent, tasks, responses, approvals)
	if err != nil {
		return ChatResponse{}, err
	}
	if blockingChild != nil {
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: workflowPendingReply(parent)}), nil
	}
	if !e.workflowTasksFinished(ctx, parent, tasks) {
		parent, err = e.failParent(ctx, parent, fmt.Errorf("workflow task scheduler incomplete"))
		if err != nil {
			return ChatResponse{}, err
		}
		parent.ErrorCode = workflowTaskIncompleteErr
		if err := e.runtime.store.SaveRun(ctx, parent); err != nil {
			return ChatResponse{}, fmt.Errorf("persist incomplete workflow state: %w", err)
		}
		return e.workflowResponse(ctx, req.Session, parent, openAIChatResult{Reply: parent.FailureReason}), nil
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
	replyResult := openAIChatResult{Reply: workflowSummary(parent, replies)}
	if saved, err := e.runtime.attachFinalAssistantMessage(ctx, req.Session, parent, replyResult); err == nil {
		parent = saved
	} else {
		if saveErr := e.runtime.store.SaveRun(ctx, parent); saveErr != nil {
			return ChatResponse{}, fmt.Errorf("persist completed workflow state: %w", saveErr)
		}
	}
	return e.workflowResponse(ctx, req.Session, parent, replyResult), nil
}

func (e *WorkflowExecutor) workflowTasksFinished(ctx context.Context, parent Run, known []Task) bool {
	tasks, err := e.workflowTasks(ctx, parent, known)
	if err != nil {
		return false
	}
	return workflowTasksComplete(tasks)
}

func (e *WorkflowExecutor) applyWorkflowChildResponses(ctx context.Context, parent Run, tasks []Task, responses []ChatResponse, approvals []Approval) ([]string, *Run, Run, error) {
	replies := make([]string, 0, len(responses))
	var blockingChild *Run
	pendingApprovals := append([]Approval(nil), approvals...)
	for responseIndex, response := range responses {
		child := response.Run
		index := workflowResponsePlanIndex(responseIndex, child)
		parent.ChildRunIDs = appendUniqueString(parent.ChildRunIDs, child.ID)
		parent = updateWorkflowPlanForChildAt(parent, child, index)
		if index >= 0 && index < len(parent.WorkflowPlan) {
			parent.WorkflowPlan[index].OutputSummary = strings.TrimSpace(response.Reply)
		}
		e.updateWorkflowTaskResult(ctx, tasks, index, child, response.Reply)
		pendingApprovals = append(pendingApprovals, child.PendingApprovals...)
		if strings.TrimSpace(response.Reply) != "" {
			replies = append(replies, response.Reply)
		}
		if isWorkflowBlockingStatus(child.Status) && blockingChild == nil {
			childCopy := child
			blockingChild = &childCopy
		}
	}
	if blockingChild != nil {
		parent = finalizeBlockedWorkflowParent(parent, *blockingChild, pendingApprovals)
		if err := e.runtime.store.SaveRun(ctx, parent); err != nil {
			return nil, nil, Run{}, fmt.Errorf("persist blocked workflow state: %w", err)
		}
	}
	return replies, blockingChild, parent, nil
}

func workflowResponsePlanIndex(fallback int, child Run) int {
	if child.Iteration > 0 {
		return child.Iteration - 1
	}
	return fallback
}

func (e *WorkflowExecutor) updateWorkflowTaskResult(ctx context.Context, tasks []Task, index int, child Run, reply string) {
	if index >= len(tasks) {
		return
	}
	status := "DONE"
	if child.Status != RunStatusCompleted {
		status = "BLOCKED"
	}
	summary := strings.TrimSpace(reply)
	_, jftradeErr := e.runtime.store.UpdateTask(ctx, tasks[index].ID, TaskPatchRequest{Status: &status, RunID: &child.ID, ResultSummary: &summary})
	besteffort.LogError(jftradeErr)
}

func finalizeBlockedWorkflowParent(parent Run, child Run, approvals []Approval) Run {
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = workflowEngineForMode(parent.WorkMode)
	}
	if parent.WorkMode == WorkModeLoop && parent.Iteration == 0 {
		parent.Iteration = 1
	}
	parent.Status = child.Status
	parent.Message = child.Message
	parent.WorkflowStatus = workflowStatusPaused
	parent.PendingApprovals = pendingApprovalsOnly(approvals)
	parent.InputRequest = normalizeInputRequest(child.InputRequest)
	if parent.Status != RunStatusPending && parent.Status != RunStatusPendingInput {
		parent.WorkflowStatus = workflowStatusFailed
		parent.FailureReason = child.FailureReason
		parent.ErrorCode = child.ErrorCode
		parent.Degraded = true
		parent.CompletedAt = new(nowString())
		finalizeRunUsage(&parent)
	}
	return parent
}

func (e *WorkflowExecutor) failParent(ctx context.Context, parent Run, cause error) (Run, error) {
	persistCtx := context.WithoutCancel(ctx)
	if tasks, taskErr := e.workflowTasks(persistCtx, parent, nil); taskErr == nil && len(tasks) > 0 {
		parent.WorkflowPlan = workflowPlanFromTasks(tasks, parent.WorkflowPlan)
	}
	parent.Status = runStatusForContext(ctx, cause)
	parent.Message = userFacingADKError(cause)
	parent.FailureReason = userFacingADKError(cause)
	parent.ErrorCode = runErrorCode(parent.Status, cause)
	parent.WorkflowStatus = workflowStatusFailed
	parent.Degraded = true
	parent.PendingApprovals = nil
	parent.CompletedAt = new(nowString())
	if parent.Status == RunStatusCancelled {
		parent.CancelledAt = parent.CompletedAt
	}
	finalizeRunUsage(&parent)
	if err := e.runtime.store.SaveRunAndDenyPendingApprovals(persistCtx, parent); err != nil {
		return parent, fmt.Errorf("persist failed workflow state: %w", err)
	}
	e.runtime.cancelUnfinishedWorkflowChildren(context.Background(), parent)
	return parent, nil
}

func (e *WorkflowExecutor) workflowResponse(ctx context.Context, session Session, parent Run, replyResult openAIChatResult) ChatResponse {
	return e.runtime.projectedChatResponse(ctx, session, parent, replyResult)
}
