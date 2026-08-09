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
	workflowTasksListTool     = jfadkmodel.WorkflowTasksListTool
	workflowTaskAddTool       = jfadkmodel.WorkflowTaskAddTool
	workflowTaskClaimTool     = jfadkmodel.WorkflowTaskClaimTool
	workflowTaskCompleteTool  = jfadkmodel.WorkflowTaskCompleteTool
	workflowTaskBlockTool     = jfadkmodel.WorkflowTaskBlockTool
	workflowTaskDelegateTool  = jfadkmodel.WorkflowTaskDelegateTool
	workflowModelsListTool    = jfadkmodel.WorkflowModelsListTool
	workflowTaskIncompleteErr = jfadkmodel.WorkflowTaskIncompleteErr

	workflowGoalCompleteTool = jfadkmodel.WorkflowGoalCompleteTool
	workflowGoalContinueTool = jfadkmodel.WorkflowGoalContinueTool
)

type workflowGoalDecision = jfadkmodel.WorkflowGoalDecision

type workflowGoalDecisionSnapshot = jfadkmodel.WorkflowGoalDecisionSnapshot

func (e *WorkflowExecutor) RunADKGoalWorkflow(ctx context.Context, req workflowRequest, parent Run, tasks []Task) (ChatResponse, error) {
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = jfadkmodel.WorkflowEngineForMode(WorkModeLoop)
	}
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.WorkflowPlan = jfadkmodel.WorkflowPlanFromTasks(tasks, parent.WorkflowPlan)
	if _, err := e.runtime.SaveRunPreservingUserGoalPause(ctx, parent); err != nil {
		parent, persistErr := e.FailParent(ctx, parent, err)
		if persistErr != nil {
			return ChatResponse{}, persistErr
		}
		return e.WorkflowResponse(ctx, req.Session, parent, jfadkmodel.AssistantExecutionResult{Reply: parent.FailureReason}), nil
	}
	limit := jfadkmodel.NormalizeLoopMaxIterations(req.RunOptions.LoopMaxIterations)
	return e.ContinueADKGoalWorkflow(ctx, req, parent, tasks, jfadkmodel.GoalOrchestratorUserMessage(parent), 1, limit)
}

func (e *WorkflowExecutor) ContinueADKGoalWorkflow(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	tasks []Task,
	nextPrompt string,
	startIteration int,
	limit int,
) (ChatResponse, error) {
	if strings.TrimSpace(parent.WorkflowEngine) == "" {
		parent.WorkflowEngine = jfadkmodel.WorkflowEngineForMode(WorkModeLoop)
	}
	decision := &workflowGoalDecision{}
	req.GoalDecision = decision
	taskTools := NewWorkflowTaskToolset(e, parent.ID, "")
	taskTools.Req = req
	execution, err := e.runtime.NewGoogleADKTaskExecution(ctx, req.Agent, req.Session, parent, req, taskTools, req.OnDelta)
	if err != nil {
		parent, persistErr := e.FailParent(ctx, parent, err)
		if persistErr != nil {
			return ChatResponse{}, persistErr
		}
		return e.WorkflowResponse(ctx, req.Session, parent, jfadkmodel.AssistantExecutionResult{Reply: parent.FailureReason}), nil
	}
	if startIteration < 1 {
		startIteration = 1
	}
	for iteration := startIteration; iteration <= limit; iteration++ {
		var response ChatResponse
		var paused bool
		var pauseErr error
		parent, response, paused, pauseErr = e.PauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration-1, "")
		if pauseErr != nil {
			return ChatResponse{}, pauseErr
		}
		if paused {
			return response, nil
		}
		decision.Reset()
		adkErr := execution.Run(ctx, genai.NewContentFromText(nextPrompt, genai.RoleUser))
		parent, response, done, prompt, turnErr := e.FinishADKGoalWorkflowTurn(ctx, req, parent, tasks, execution, decision, adkErr, iteration, false)
		if turnErr != nil {
			return ChatResponse{}, turnErr
		}
		if done {
			return response, nil
		}
		if prompt == "" {
			prompt = jfadkmodel.GoalOrchestratorContinueNudge(parent, "")
		}
		nextPrompt = prompt
	}
	pausedAt := jfadkmodel.NowString()
	parent.Status = RunStatusPaused
	parent.WorkflowStatus = workflowStatusPaused
	parent.Message = "目标达到本轮运行上限，已暂停。"
	parent.ResumeState = "iteration_limit"
	parent.PausedReason = "iteration_limit"
	parent.PausedAt = &pausedAt
	parent.Iteration = limit
	parent.PendingApprovals = jfadkmodel.PendingApprovalsOnly(parent.PendingApprovals)
	parent, saveErr := e.runtime.SaveRunPreservingUserGoalPause(ctx, parent)
	if saveErr != nil {
		return ChatResponse{}, fmt.Errorf("persist goal iteration-limit pause: %w", saveErr)
	}
	return e.WorkflowResponse(ctx, req.Session, parent, jfadkmodel.AssistantExecutionResult{Reply: parent.Message}), nil
}

func (e *WorkflowExecutor) PauseADKGoalWorkflowIfRequested(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	completedIteration int,
	reply string,
) (Run, ChatResponse, bool, error) {
	latest := parent
	if refreshed, ok, err := e.runtime.WorkflowStore().Run(ctx, parent.ID); err == nil && ok {
		latest = refreshed
	}
	if latest.PauseRequestedAt == nil {
		return parent, ChatResponse{}, false, nil
	}
	parent = latest
	if parent.Status == RunStatusPaused && parent.PausedReason == "user" {
		if cleaned, changed := jfadkmodel.PruneInterruptedGoalWorkflowToolCalls(parent); changed {
			parent = cleaned
			updatedParent, err := e.runtime.SaveRunPreservingUserGoalPause(ctx, parent)
			if err != nil {
				return Run{}, ChatResponse{}, false, fmt.Errorf("persist cleaned paused goal state: %w", err)
			}
			parent = updatedParent
		}
		return parent, e.WorkflowResponse(ctx, req.Session, parent, jfadkmodel.AssistantExecutionResult{Reply: jfadkmodel.DefaultString(reply, parent.Message)}), true, nil
	}
	pausedAt := jfadkmodel.NowString()
	parent.Status = RunStatusPaused
	parent.WorkflowStatus = workflowStatusPaused
	if parent.PausedAt == nil {
		parent.PausedAt = &pausedAt
	}
	parent.PausedReason = "user"
	parent.ResumeState = "user_paused"
	parent.Message = "目标已暂停。"
	if completedIteration > parent.Iteration {
		parent.Iteration = completedIteration
	}
	parent.PendingApprovals = jfadkmodel.PendingApprovalsOnly(parent.PendingApprovals)
	parent, _ = jfadkmodel.PruneInterruptedGoalWorkflowToolCalls(parent)
	parent, err := e.runtime.SaveRunPreservingUserGoalPause(ctx, parent)
	if err != nil {
		return Run{}, ChatResponse{}, false, fmt.Errorf("persist user-paused goal state: %w", err)
	}
	return parent, e.WorkflowResponse(ctx, req.Session, parent, jfadkmodel.AssistantExecutionResult{Reply: jfadkmodel.DefaultString(reply, parent.Message)}), true, nil
}

func (e *WorkflowExecutor) ResumeADKGoalWorkflow(ctx context.Context, session Session, agent Agent, parent Run) (Run, error) {
	parent, blocked, err := e.ReconcileWorkflowChildren(ctx, parent)
	if err != nil {
		return Run{}, err
	}
	if blocked {
		return parent, nil
	}
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.Message = "goal resumed"
	parent.ResumeState = "user_resuming"
	parent.PendingApprovals = jfadkmodel.PendingApprovalsOnly(parent.PendingApprovals)
	if _, err := e.runtime.SaveRunPreservingUserGoalPause(ctx, parent); err != nil {
		return Run{}, err
	}
	tasks, err := e.WorkflowTasks(ctx, parent, nil)
	if err != nil {
		return Run{}, err
	}
	limit := parent.Iteration + jfadkmodel.NormalizeLoopMaxIterations(agent.LoopMaxIterations)
	response, err := e.ContinueADKGoalWorkflow(ctx, workflowRequest{
		Agent: agent, Session: session, Message: parent.UserMessage, Mode: parent.WorkMode, Objective: parent.Objective,
		RunOptions: RunOptions{
			LoopMaxIterations: limit,
		},
	}, parent, tasks, jfadkmodel.GoalOrchestratorContinueNudge(parent, "用户继续运行目标。"), parent.Iteration+1, limit)
	if err != nil {
		return Run{}, err
	}
	return response.Run, nil
}

func (e *WorkflowExecutor) FinishADKGoalWorkflowTurn(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	known []Task,
	execution jfadkmodel.WorkflowExecutionHandle,
	decision *workflowGoalDecision,
	adkErr error,
	iteration int,
	decisionRetry bool,
) (Run, ChatResponse, bool, string, error) {
	parent, replyResult, done, prompt, err := e.PrepareGoalWorkflowTurn(ctx, req, parent, known, execution, adkErr, iteration)
	if err != nil {
		return Run{}, ChatResponse{}, false, "", err
	}
	if done {
		return parent, e.WorkflowResponse(ctx, req.Session, parent, replyResult), true, "", nil
	}
	visibleReply := strings.TrimSpace(replyResult.Reply)
	parent, replyResult, snapshot, done, response, prompt, err := e.ResolveGoalWorkflowDecision(
		ctx, req, parent, known, execution, decision, replyResult, visibleReply, prompt, iteration, decisionRetry,
	)
	if err != nil {
		return Run{}, ChatResponse{}, false, "", err
	}
	if done {
		return parent, response, true, prompt, nil
	}
	switch snapshot.Status {
	case "complete":
		return e.FinishCompleteGoalWorkflow(ctx, req, parent, known, replyResult, snapshot, visibleReply, iteration)
	case "continue":
		return e.FinishContinueGoalWorkflow(ctx, req, parent, replyResult, snapshot, visibleReply, iteration)
	default:
		return parent, ChatResponse{}, false, prompt, nil
	}
}

func (e *WorkflowExecutor) ResolveGoalWorkflowDecision(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	known []Task,
	execution jfadkmodel.WorkflowExecutionHandle,
	decision *workflowGoalDecision,
	replyResult jfadkmodel.AssistantExecutionResult,
	visibleReply string,
	prompt string,
	iteration int,
	decisionRetry bool,
) (Run, jfadkmodel.AssistantExecutionResult, workflowGoalDecisionSnapshot, bool, ChatResponse, string, error) {
	snapshot := decision.Snapshot()
	if snapshot.Status != "" {
		return parent, replyResult, snapshot, false, ChatResponse{}, prompt, nil
	}
	parent, response, paused, err := e.PauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, visibleReply)
	if err != nil {
		return Run{}, jfadkmodel.AssistantExecutionResult{}, workflowGoalDecisionSnapshot{}, false, ChatResponse{}, "", err
	}
	if paused {
		replyResult.Reply = jfadkmodel.DefaultString(visibleReply, replyResult.Reply)
		return parent, replyResult, snapshot, true, response, "", nil
	}
	latest := parent
	if refreshed, ok, err := e.runtime.WorkflowStore().Run(ctx, parent.ID); err == nil && ok {
		latest = refreshed
		parent = refreshed
	}
	if !execution.HasFinalReplyForRun(parent.ID, visibleReply) {
		return parent, replyResult, snapshot, false, ChatResponse{}, jfadkmodel.GoalFinalReplyPrompt(parent), nil
	}
	return e.RunGoalWorkflowDecision(ctx, req, parent, known, execution, decision, latest, visibleReply, iteration, decisionRetry)
}

func (e *WorkflowExecutor) RunGoalWorkflowDecision(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	known []Task,
	execution jfadkmodel.WorkflowExecutionHandle,
	decision *workflowGoalDecision,
	latest Run,
	visibleReply string,
	iteration int,
	decisionRetry bool,
) (Run, jfadkmodel.AssistantExecutionResult, workflowGoalDecisionSnapshot, bool, ChatResponse, string, error) {
	decision.BeginDecision()
	decisionErr := execution.Run(ctx, genai.NewContentFromText(jfadkmodel.GoalDecisionPrompt(latest, visibleReply, decisionRetry), genai.RoleUser))
	parent, replyResult, done, prompt, err := e.PrepareGoalWorkflowTurn(ctx, req, parent, known, execution, decisionErr, iteration)
	if err != nil {
		return Run{}, jfadkmodel.AssistantExecutionResult{}, workflowGoalDecisionSnapshot{}, false, ChatResponse{}, "", err
	}
	if done {
		return parent, replyResult, decision.Snapshot(), true, e.WorkflowResponse(ctx, req.Session, parent, replyResult), "", nil
	}
	snapshot := decision.Snapshot()
	parent, replyResult, done, response, err := e.PauseAfterMissingGoalDecision(ctx, req, parent, replyResult, visibleReply, snapshot, iteration)
	if err != nil {
		return Run{}, jfadkmodel.AssistantExecutionResult{}, workflowGoalDecisionSnapshot{}, false, ChatResponse{}, "", err
	}
	if done {
		return parent, replyResult, snapshot, true, response, "", nil
	}
	if snapshot.Status == "" && !decisionRetry {
		parent, response, done, prompt, err = e.FinishADKGoalWorkflowTurn(ctx, req, parent, known, execution, decision, nil, iteration, true)
		return parent, replyResult, snapshot, done, response, prompt, err
	}
	if snapshot.Status == "" {
		decision.SetContinue("目标裁决未按要求调用工具，安全地继续目标。")
		snapshot = decision.Snapshot()
	}
	return parent, replyResult, snapshot, false, ChatResponse{}, prompt, nil
}

func (e *WorkflowExecutor) PauseAfterMissingGoalDecision(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	replyResult jfadkmodel.AssistantExecutionResult,
	visibleReply string,
	snapshot workflowGoalDecisionSnapshot,
	iteration int,
) (Run, jfadkmodel.AssistantExecutionResult, bool, ChatResponse, error) {
	if snapshot.Status != "" {
		return parent, replyResult, false, ChatResponse{}, nil
	}
	reply := strings.TrimSpace(replyResult.Reply)
	if reply == "" {
		reply = visibleReply
	}
	parent, response, paused, err := e.PauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, reply)
	if err != nil {
		return Run{}, jfadkmodel.AssistantExecutionResult{}, false, ChatResponse{}, err
	}
	if paused {
		replyResult.Reply = jfadkmodel.DefaultString(reply, replyResult.Reply)
		return parent, replyResult, true, response, nil
	}
	return parent, replyResult, false, ChatResponse{}, nil
}

func (e *WorkflowExecutor) FinishCompleteGoalWorkflow(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	known []Task,
	replyResult jfadkmodel.AssistantExecutionResult,
	snapshot workflowGoalDecisionSnapshot,
	visibleReply string,
	iteration int,
) (Run, ChatResponse, bool, string, error) {
	reply := e.CompleteGoalReply(ctx, parent, known, snapshot, visibleReply)
	replyResult.Reply = reply
	parent, response, paused, err := e.PauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, reply)
	if err != nil {
		return Run{}, ChatResponse{}, false, "", err
	}
	if paused {
		return parent, response, true, "", nil
	}
	parent.Status = RunStatusCompleted
	parent.WorkflowStatus = workflowStatusComplete
	parent.Message = "goal completed"
	parent.Iteration = iteration
	parent.PendingApprovals = jfadkmodel.PendingApprovalsOnly(parent.PendingApprovals)
	parent.CompletedAt = new(jfadkmodel.NowString())
	jfadkmodel.FinalizeRunUsage(&parent)
	if saved, err := e.runtime.AttachFinalAssistantMessage(ctx, req.Session, parent, replyResult); err == nil {
		parent = saved
	} else {
		if saveErr := e.runtime.WorkflowStore().SaveRun(ctx, parent); saveErr != nil {
			return Run{}, ChatResponse{}, false, "", fmt.Errorf("persist completed goal state: %w", saveErr)
		}
	}
	return parent, e.WorkflowResponse(ctx, req.Session, parent, replyResult), true, "", nil
}

func (e *WorkflowExecutor) CompleteGoalReply(
	ctx context.Context,
	parent Run,
	known []Task,
	snapshot workflowGoalDecisionSnapshot,
	visibleReply string,
) string {
	reply := strings.TrimSpace(snapshot.Summary)
	if reply != "" {
		return reply
	}
	if visibleReply != "" {
		return visibleReply
	}
	tasks, jftradeErr10 := e.WorkflowTasks(ctx, parent, known)
	besteffort.LogError(jftradeErr10)
	return jfadkmodel.WorkflowSummary(parent, jfadkmodel.WorkflowTaskResultSummaries(tasks))
}

func (e *WorkflowExecutor) FinishContinueGoalWorkflow(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	replyResult jfadkmodel.AssistantExecutionResult,
	snapshot workflowGoalDecisionSnapshot,
	visibleReply string,
	iteration int,
) (Run, ChatResponse, bool, string, error) {
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.Message = jfadkmodel.DefaultString(snapshot.Reason, "goal continues")
	parent.Iteration = iteration
	reply := jfadkmodel.DefaultString(visibleReply, jfadkmodel.DefaultString(snapshot.Reason, "目标已暂停。"))
	parent, response, paused, err := e.PauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, reply)
	if err != nil {
		return Run{}, ChatResponse{}, false, "", err
	}
	if paused {
		replyResult.Reply = reply
		return parent, response, true, "", nil
	}
	parent, err = e.runtime.SaveRunPreservingUserGoalPause(ctx, parent)
	if err != nil {
		return Run{}, ChatResponse{}, false, "", fmt.Errorf("persist continued goal state: %w", err)
	}
	return parent, ChatResponse{}, false, jfadkmodel.GoalOrchestratorContinueNudge(parent, snapshot.Reason), nil
}

func (e *WorkflowExecutor) PrepareGoalWorkflowTurn(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	known []Task,
	execution jfadkmodel.WorkflowExecutionHandle,
	adkErr error,
	iteration int,
) (Run, jfadkmodel.AssistantExecutionResult, bool, string, error) {
	if latest, ok, err := e.runtime.WorkflowStore().Run(ctx, parent.ID); err == nil && ok {
		parent = latest
	}
	toolContext := execution.ToolContextForRun(parent.ID)
	replyResult := execution.ResultForRun(parent.ID)
	parent = jfadkmodel.HydrateRunExecutionResult(parent, toolContext, nil, "", "")
	parent.Iteration = iteration
	tasks, err := e.WorkflowTasks(ctx, parent, known)
	if err != nil {
		parent, persistErr := e.FailParent(ctx, parent, err)
		if persistErr != nil {
			return Run{}, jfadkmodel.AssistantExecutionResult{}, false, "", persistErr
		}
		return parent, jfadkmodel.AssistantExecutionResult{Reply: parent.FailureReason}, true, "", nil
	}
	parent.WorkflowPlan = jfadkmodel.WorkflowPlanFromTasks(tasks, parent.WorkflowPlan)
	if errors.Is(adkErr, jfadkmodel.ErrUserGoalPauseRequested) {
		reply := strings.TrimSpace(replyResult.Reply)
		var paused bool
		parent, _, paused, err = e.PauseADKGoalWorkflowIfRequested(ctx, req, parent, iteration, reply)
		if err != nil {
			return Run{}, jfadkmodel.AssistantExecutionResult{}, false, "", err
		}
		if paused {
			return parent, jfadkmodel.AssistantExecutionResult{Reply: jfadkmodel.DefaultString(reply, parent.Message)}, true, "", nil
		}
	}
	if adkErr != nil {
		parent, persistErr := e.FailParent(ctx, parent, adkErr)
		if persistErr != nil {
			return Run{}, jfadkmodel.AssistantExecutionResult{}, false, "", persistErr
		}
		return parent, jfadkmodel.AssistantExecutionResult{Reply: parent.FailureReason}, true, "", nil
	}
	if child, index, ok := e.FirstBlockingTaskChild(ctx, parent); ok {
		if child.Status == RunStatusPending || child.Status == RunStatusPendingInput || child.Status == RunStatusRunning {
			parent = jfadkmodel.PauseParentForChild(parent, child, index)
			parent.Iteration = iteration
			parent, err = e.runtime.SaveRunPreservingUserGoalPause(ctx, parent)
			if err != nil {
				return Run{}, jfadkmodel.AssistantExecutionResult{}, false, "", fmt.Errorf("persist goal blocked by child: %w", err)
			}
			return parent, jfadkmodel.AssistantExecutionResult{Reply: jfadkmodel.WorkflowPendingReply(parent)}, true, "", nil
		}
		parent, err = e.runtime.TerminateParentWorkflowFromChild(ctx, parent, child)
		if err != nil {
			return Run{}, jfadkmodel.AssistantExecutionResult{}, false, "", err
		}
		return parent, jfadkmodel.AssistantExecutionResult{Reply: parent.FailureReason}, true, "", nil
	}
	if blockedTask, ok := jfadkmodel.FirstTerminalWorkflowTask(tasks); ok {
		parent.Status = RunStatusFailed
		parent.WorkflowStatus = workflowStatusFailed
		parent.Message = jfadkmodel.DefaultString(blockedTask.ResultSummary, blockedTask.Description)
		parent.FailureReason = parent.Message
		parent.ErrorCode = "WORKFLOW_TASK_BLOCKED"
		parent.Degraded = true
		parent.CompletedAt = new(jfadkmodel.NowString())
		jfadkmodel.FinalizeRunUsage(&parent)
		if err := e.runtime.WorkflowStore().SaveRun(ctx, parent); err != nil {
			return Run{}, jfadkmodel.AssistantExecutionResult{}, false, "", fmt.Errorf("persist blocked goal state: %w", err)
		}
		return parent, jfadkmodel.AssistantExecutionResult{Reply: parent.FailureReason}, true, "", nil
	}
	parent.Status = RunStatusRunning
	parent.WorkflowStatus = workflowStatusRunning
	parent.Message = "goal running"
	parent, err = e.runtime.SaveRunPreservingUserGoalPause(ctx, parent)
	if err != nil {
		return Run{}, jfadkmodel.AssistantExecutionResult{}, false, "", fmt.Errorf("persist running goal state: %w", err)
	}
	return parent, replyResult, false, "", nil
}

func (e *WorkflowExecutor) FirstBlockingTaskChild(ctx context.Context, parent Run) (Run, int, bool) {
	for index, state := range parent.WorkflowPlan {
		childRunID := strings.TrimSpace(state.ChildRunID)
		if childRunID == "" {
			continue
		}
		child, ok, err := e.runtime.WorkflowStore().Run(ctx, childRunID)
		if err != nil || !ok {
			continue
		}
		if !jfadkmodel.IsDirectWorkflowChild(parent, child) {
			continue
		}
		switch child.Status {
		case RunStatusPending, RunStatusPendingInput, RunStatusRunning, RunStatusFailed, RunStatusDenied, RunStatusCancelled, RunStatusTimedOut:
			return child, index, true
		}
	}
	return Run{}, -1, false
}
