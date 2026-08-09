package workflowexec

import (
	"context"
	"errors"
	"fmt"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"google.golang.org/genai"
)

const (
	workflowStatusRunning  = jfadkmodel.WorkflowStatusRunning
	workflowStatusPaused   = jfadkmodel.WorkflowStatusPaused
	workflowStatusComplete = jfadkmodel.WorkflowStatusComplete
	workflowStatusFailed   = jfadkmodel.WorkflowStatusFailed

	workflowPlanSourcePlanner = jfadkmodel.WorkflowPlanSourcePlanner
	workflowPlanSourceRuntime = jfadkmodel.WorkflowPlanSourceRuntime
	workflowPlanSourceCanvas  = jfadkmodel.WorkflowPlanSourceCanvas

	workflowTaskExecutorSelf  = jfadkmodel.WorkflowTaskExecutorSelf
	workflowTaskExecutorChild = jfadkmodel.WorkflowTaskExecutorChild

	maxRuntimeWorkflowTasks = jfadkmodel.MaxRuntimeWorkflowTasks
)

type WorkflowExecutor struct {
	runtime jfadkmodel.WorkflowExecutorRuntime
}

type workflowRequest = jfadkmodel.WorkflowRequest

type workflowStep = jfadkmodel.WorkflowStep

type workflowChildResult struct {
	Index    int
	TaskID   string
	Response ChatResponse
	Err      error
}

// NewWorkflowExecutor constructs the workflow execution implementation used by
// the engine-root composition seam.
func NewWorkflowExecutor(runtime jfadkmodel.WorkflowExecutorRuntime) *WorkflowExecutor {
	return &WorkflowExecutor{runtime: runtime}
}

func (e *WorkflowExecutor) Run(ctx context.Context, req workflowRequest) (ChatResponse, error) {
	if e == nil || e.runtime == nil {
		return ChatResponse{}, fmt.Errorf("adk runtime is unavailable")
	}
	mode := jfadkmodel.NormalizeWorkMode(req.Mode)
	if mode == WorkModeChat {
		return ChatResponse{}, fmt.Errorf("workflow mode is required")
	}
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		objective = req.Message
	}
	parent, parentCtx, finishParent, err := e.runtime.StartRunWithOptions(ctx, req.Session.ID, req.Agent, req.Message, jfadkmodel.RunStartOptions{
		WorkMode:           mode,
		Objective:          objective,
		ClientRequestID:    req.ClientRequestID,
		RequestFingerprint: req.RequestFingerprint,
		WorkflowStatus:     workflowStatusRunning,
		WorkflowEngine:     jfadkmodel.WorkflowEngineForMode(mode),
	})
	if err != nil {
		var reused *jfadkmodel.ReusedChatRequestError
		if errors.As(err, &reused) {
			return e.runtime.ChatResponseForExistingRun(ctx, reused.Run)
		}
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
		parent, persistErr := e.FailParent(parentCtx, parent, err)
		if persistErr != nil {
			return ChatResponse{}, persistErr
		}
		return e.WorkflowResponse(parentCtx, req.Session, parent, jfadkmodel.AssistantExecutionResult{Reply: parent.FailureReason}), nil
	}
	parent.WorkflowPlan = jfadkmodel.WorkflowPlanFromTasks([]Task{task}, parent.WorkflowPlan)
	parent, err = e.runtime.SaveRunPreservingUserGoalPause(parentCtx, parent)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("persist initial goal workflow state: %w", err)
	}
	return e.RunADKGoalWorkflow(parentCtx, req, parent, []Task{task})
}

func (e *WorkflowExecutor) createInitialGoalTask(ctx context.Context, parent Run, agent Agent, objective string, message string) (Task, error) {
	return e.runtime.WorkflowStore().SaveTask(ctx, TaskWriteRequest{
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

func (e *WorkflowExecutor) PlanWorkflowSteps(ctx context.Context, req workflowRequest, mode string, objective string) ([]workflowStep, []string, error) {
	steps, warnings, err := e.runtime.PlanWorkflowWithADK(ctx, req.Agent, req.Session, mode, req.Message, objective, req.RunOptions)
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

func (e *WorkflowExecutor) PersistWorkflowTasks(ctx context.Context, parent Run, agent Agent, steps []workflowStep) ([]Task, error) {
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
		task, err := e.runtime.WorkflowStore().SaveTask(ctx, TaskWriteRequest{
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
	execution     jfadkmodel.WorkflowExecutionHandle
	approvals     []Approval
	inputRequests map[string]*InputRequest
}

func (e *WorkflowExecutor) RunNativeTaskGraphWorkflow(ctx context.Context, req workflowRequest, parent Run, steps []workflowStep, tasks []Task) (ChatResponse, error) {
	childRuns, finishes, err := e.StartWorkflowChildRuns(ctx, req, parent, steps, tasks)
	defer finishWorkflowChildren(finishes)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	parent, err = e.PrepareWorkflowParent(ctx, req, parent, childRuns)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	execution, err := e.runtime.NewGoogleADKWorkflowExecution(ctx, req.Agent, req.Session, parent, childRuns, steps, parent.WorkMode, req.RunOptions, req.OnDelta)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	executionResult, parent, err := e.ExecuteStartedWorkflowGraph(ctx, req, parent, childRuns, steps, execution)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	if len(executionResult.inputRequests) > 0 {
		return e.finishWorkflowPendingInputs(ctx, req, parent, tasks, childRuns, executionResult)
	}
	if err := e.EnsureWorkflowChildrenFinalReplies(ctx, req, executionResult.execution, childRuns, steps, executionResult.approvals); err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	responses, err := e.CompleteWorkflowChildrenFromADK(ctx, req, executionResult.execution, childRuns, executionResult.approvals)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	return e.FinalizePlannedWorkflow(ctx, req, parent, tasks, responses, executionResult.approvals)
}

func (e *WorkflowExecutor) RunPlannedGoogleADKWorkflow(ctx context.Context, req workflowRequest, parent Run, steps []workflowStep, tasks []Task) (ChatResponse, error) {
	childRuns, finishes, err := e.StartWorkflowChildRuns(ctx, req, parent, steps, tasks)
	defer finishWorkflowChildren(finishes)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	parent, err = e.PrepareWorkflowParent(ctx, req, parent, childRuns)
	if err != nil {
		return ChatResponse{}, err
	}
	executionResult, parent, err := e.RunWorkflowExecution(ctx, req, parent, childRuns, steps)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	if len(executionResult.inputRequests) > 0 {
		return e.finishWorkflowPendingInputs(ctx, req, parent, tasks, childRuns, executionResult)
	}
	if err := e.EnsureWorkflowChildrenFinalReplies(ctx, req, executionResult.execution, childRuns, steps, executionResult.approvals); err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	responses, err := e.CompleteWorkflowChildrenFromADK(ctx, req, executionResult.execution, childRuns, executionResult.approvals)
	if err != nil {
		return e.failedWorkflowResponse(ctx, req, parent, err)
	}
	return e.FinalizePlannedWorkflow(ctx, req, parent, tasks, responses, executionResult.approvals)
}

func (e *WorkflowExecutor) failedWorkflowResponse(ctx context.Context, req workflowRequest, parent Run, cause error) (ChatResponse, error) {
	parent, err := e.FailParent(ctx, parent, cause)
	if err != nil {
		return ChatResponse{}, err
	}
	return e.WorkflowResponse(ctx, req.Session, parent, jfadkmodel.AssistantExecutionResult{Reply: parent.FailureReason}), nil
}

func (e *WorkflowExecutor) PrepareWorkflowParent(ctx context.Context, req workflowRequest, parent Run, childRuns []Run) (Run, error) {
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = WorkflowEngineADK2Loop
	}
	parent.ChildRunIDs = jfadkmodel.ChildRunIDs(childRuns)
	for index, child := range childRuns {
		if index < len(parent.WorkflowPlan) {
			jfadkmodel.ApplyWorkflowChildState(&parent.WorkflowPlan[index], child)
			if strings.TrimSpace(parent.WorkflowPlan[index].NodeName) == "" {
				parent.WorkflowPlan[index].NodeName = jfadkmodel.GoogleADKWorkflowChildName(parent.ID, index)
			}
		}
	}
	if err := e.runtime.WorkflowStore().SaveRun(ctx, parent); err != nil {
		return parent, err
	}
	if err := emitWorkflowRunSnapshot(ctx, e.runtime, req, parent); err != nil {
		return parent, err
	}
	if err := e.runtime.MaybeAutoCompactSessionDuringWorkflow(ctx, req.Session, req.Agent, req.Message, req.OnDelta); err != nil {
		return parent, err
	}
	return parent, nil
}

func (e *WorkflowExecutor) RunWorkflowExecution(ctx context.Context, req workflowRequest, parent Run, childRuns []Run, steps []workflowStep) (workflowExecutionResult, Run, error) {
	execution, err := e.runtime.NewGoogleADKWorkflowExecution(ctx, req.Agent, req.Session, parent, childRuns, steps, parent.WorkMode, req.RunOptions, req.OnDelta)
	if err != nil {
		return workflowExecutionResult{}, parent, err
	}
	return e.ExecuteStartedWorkflowGraph(ctx, req, parent, childRuns, steps, execution)
}

func (e *WorkflowExecutor) ExecuteStartedWorkflowGraph(ctx context.Context, req workflowRequest, parent Run, childRuns []Run, steps []workflowStep, execution jfadkmodel.WorkflowExecutionHandle) (workflowExecutionResult, Run, error) {
	approvals, inputRequests, err := e.executeWorkflowRun(ctx, req.Message, parent, childRuns, execution)
	if err != nil {
		return workflowExecutionResult{}, parent, err
	}
	return workflowExecutionResult{execution: execution, approvals: approvals, inputRequests: inputRequests}, parent, nil
}

func (e *WorkflowExecutor) executeWorkflowRun(ctx context.Context, message string, parent Run, childRuns []Run, execution jfadkmodel.WorkflowExecutionHandle) ([]Approval, map[string]*InputRequest, error) {
	var approvals []Approval
	var inputRequests map[string]*InputRequest
	var adkErr error
	_ = e.runtime.WithWorkflowChildLock(ctx, func() error {
		adkErr = execution.Run(ctx, genai.NewContentFromText(message, genai.RoleUser))
		if adkErr == nil {
			approvals, adkErr = execution.PendingApprovals(ctx, e.runtime.WorkflowStore())
		}
		if adkErr == nil {
			inputRequests, adkErr = e.runtime.PendingInputRequests(ctx, execution)
			execution.SetInputRequests(inputRequests)
		}
		return nil
	})
	if adkErr != nil {
		return nil, nil, adkErr
	}
	if len(approvals) > 0 || len(inputRequests) > 0 {
		e.runtime.RegisterWorkflowExecution(parent.ID, jfadkmodel.ChildRunIDs(childRuns), execution)
	}
	return approvals, inputRequests, nil
}

func (e *WorkflowExecutor) finishWorkflowPendingInputs(ctx context.Context, req workflowRequest, parent Run, tasks []Task, childRuns []Run, result workflowExecutionResult) (ChatResponse, error) {
	responses := make([]ChatResponse, 0, len(childRuns))
	for _, child := range childRuns {
		request := result.inputRequests[child.ID]
		if request == nil {
			responses = append(responses, e.WorkflowResponse(ctx, req.Session, child, jfadkmodel.AssistantExecutionResult{}))
			continue
		}
		toolContext := result.execution.ToolContextForRun(child.ID)
		child = jfadkmodel.HydrateRunExecutionResult(child, toolContext, nil, child.PreToolContent, child.PreToolReasoning)
		childCtx, err := e.runtime.ActiveRunExecutionContext(ctx, child.ID)
		if err != nil {
			return e.failedWorkflowResponse(ctx, req, parent, err)
		}
		response, err := e.runtime.FinishPendingInputRun(childCtx, req.Session, child, request)
		if err != nil {
			return e.failedWorkflowResponse(ctx, req, parent, err)
		}
		responses = append(responses, response)
	}
	return e.FinalizePlannedWorkflow(ctx, req, parent, tasks, responses, nil)
}

func (e *WorkflowExecutor) FinalizePlannedWorkflow(ctx context.Context, req workflowRequest, parent Run, tasks []Task, responses []ChatResponse, approvals []Approval) (ChatResponse, error) {
	replies, blockingChild, parent, err := e.applyWorkflowChildResponses(ctx, parent, tasks, responses, approvals)
	if err != nil {
		return ChatResponse{}, err
	}
	if blockingChild != nil {
		return e.WorkflowResponse(ctx, req.Session, parent, jfadkmodel.AssistantExecutionResult{Reply: jfadkmodel.WorkflowPendingReply(parent)}), nil
	}
	if !e.WorkflowTasksFinished(ctx, parent, tasks) {
		parent, err = e.FailParent(ctx, parent, fmt.Errorf("workflow task scheduler incomplete"))
		if err != nil {
			return ChatResponse{}, err
		}
		parent.ErrorCode = workflowTaskIncompleteErr
		if err := e.runtime.WorkflowStore().SaveRun(ctx, parent); err != nil {
			return ChatResponse{}, fmt.Errorf("persist incomplete workflow state: %w", err)
		}
		return e.WorkflowResponse(ctx, req.Session, parent, jfadkmodel.AssistantExecutionResult{Reply: parent.FailureReason}), nil
	}
	parent.Status = RunStatusCompleted
	parent.Message = "workflow completed"
	parent.WorkflowStatus = workflowStatusComplete
	if parent.WorkMode == WorkModeLoop && parent.Iteration == 0 {
		parent.Iteration = 1
	}
	parent.PendingApprovals = nil
	parent.CompletedAt = new(jfadkmodel.NowString())
	jfadkmodel.FinalizeRunUsage(&parent)
	replyResult := jfadkmodel.AssistantExecutionResult{Reply: jfadkmodel.WorkflowSummary(parent, replies), SyntheticKind: "workflow_summary"}
	if saved, err := e.runtime.AttachFinalAssistantMessage(ctx, req.Session, parent, replyResult); err == nil {
		parent = saved
	} else {
		if saveErr := e.runtime.WorkflowStore().SaveRun(ctx, parent); saveErr != nil {
			return ChatResponse{}, fmt.Errorf("persist completed workflow state: %w", saveErr)
		}
	}
	return e.WorkflowResponse(ctx, req.Session, parent, replyResult), nil
}

func (e *WorkflowExecutor) WorkflowTasksFinished(ctx context.Context, parent Run, known []Task) bool {
	tasks, err := e.WorkflowTasks(ctx, parent, known)
	if err != nil {
		return false
	}
	return jfadkmodel.WorkflowTasksComplete(tasks)
}

func (e *WorkflowExecutor) applyWorkflowChildResponses(ctx context.Context, parent Run, tasks []Task, responses []ChatResponse, approvals []Approval) ([]string, *Run, Run, error) {
	replies := make([]string, 0, len(responses))
	var blockingChild *Run
	pendingApprovals := append([]Approval(nil), approvals...)
	for responseIndex, response := range responses {
		child := response.Run
		index := jfadkmodel.WorkflowResponsePlanIndex(responseIndex, child)
		parent.ChildRunIDs = jfadkmodel.AppendUniqueString(parent.ChildRunIDs, child.ID)
		parent = jfadkmodel.UpdateWorkflowPlanForChildAt(parent, child, index)
		if index >= 0 && index < len(parent.WorkflowPlan) {
			parent.WorkflowPlan[index].OutputSummary = strings.TrimSpace(response.Reply)
		}
		e.UpdateWorkflowTaskResult(ctx, tasks, index, child, response.Reply)
		pendingApprovals = append(pendingApprovals, child.PendingApprovals...)
		if strings.TrimSpace(response.Reply) != "" {
			replies = append(replies, response.Reply)
		}
		if jfadkmodel.IsWorkflowBlockingStatus(child.Status) && blockingChild == nil {
			childCopy := child
			blockingChild = &childCopy
		}
	}
	if blockingChild != nil {
		parent = finalizeBlockedWorkflowParent(parent, *blockingChild, pendingApprovals)
		if err := e.runtime.WorkflowStore().SaveRun(ctx, parent); err != nil {
			return nil, nil, Run{}, fmt.Errorf("persist blocked workflow state: %w", err)
		}
	}
	return replies, blockingChild, parent, nil
}

func (e *WorkflowExecutor) UpdateWorkflowTaskResult(ctx context.Context, tasks []Task, index int, child Run, reply string) {
	if index >= len(tasks) {
		return
	}
	status := "DONE"
	if child.Status != RunStatusCompleted {
		status = "BLOCKED"
	}
	summary := strings.TrimSpace(reply)
	_, jftradeErr := e.runtime.WorkflowStore().UpdateTask(ctx, tasks[index].ID, TaskPatchRequest{Status: &status, RunID: &child.ID, ResultSummary: &summary})
	besteffort.LogError(jftradeErr)
}

func finalizeBlockedWorkflowParent(parent Run, child Run, approvals []Approval) Run {
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = jfadkmodel.WorkflowEngineForMode(parent.WorkMode)
	}
	if parent.WorkMode == WorkModeLoop && parent.Iteration == 0 {
		parent.Iteration = 1
	}
	parent.Status = child.Status
	parent.Message = child.Message
	parent.WorkflowStatus = workflowStatusPaused
	parent.PendingApprovals = jfadkmodel.PendingApprovalsOnly(approvals)
	parent.InputRequest = jfadkmodel.NormalizeInputRequest(child.InputRequest)
	if parent.Status != RunStatusPending && parent.Status != RunStatusPendingInput {
		parent.WorkflowStatus = workflowStatusFailed
		parent.FailureReason = child.FailureReason
		parent.ErrorCode = child.ErrorCode
		parent.Degraded = true
		parent.CompletedAt = new(jfadkmodel.NowString())
		jfadkmodel.FinalizeRunUsage(&parent)
	}
	return parent
}

func (e *WorkflowExecutor) FailParent(ctx context.Context, parent Run, cause error) (Run, error) {
	persistCtx := context.WithoutCancel(ctx)
	if tasks, taskErr := e.WorkflowTasks(persistCtx, parent, nil); taskErr == nil && len(tasks) > 0 {
		parent.WorkflowPlan = jfadkmodel.WorkflowPlanFromTasks(tasks, parent.WorkflowPlan)
	}
	parent.Status = jfadkmodel.RunStatusForContext(ctx, cause)
	parent.Message = jfadkmodel.UserFacingADKError(cause)
	parent.FailureReason = jfadkmodel.UserFacingADKError(cause)
	parent.ErrorCode = jfadkmodel.RunErrorCode(parent.Status, cause)
	parent.WorkflowStatus = workflowStatusFailed
	parent.Degraded = true
	parent.PendingApprovals = nil
	parent.CompletedAt = new(jfadkmodel.NowString())
	if parent.Status == RunStatusCancelled {
		parent.CancelledAt = parent.CompletedAt
	}
	jfadkmodel.FinalizeRunUsage(&parent)
	if err := e.runtime.WorkflowStore().SaveRunAndDenyPendingApprovals(persistCtx, parent); err != nil {
		return parent, fmt.Errorf("persist failed workflow state: %w", err)
	}
	e.runtime.CancelUnfinishedWorkflowChildren(context.Background(), parent)
	return parent, nil
}

func (e *WorkflowExecutor) WorkflowResponse(ctx context.Context, session Session, parent Run, replyResult jfadkmodel.AssistantExecutionResult) ChatResponse {
	return e.runtime.ProjectedChatResponse(ctx, session, parent, replyResult)
}
