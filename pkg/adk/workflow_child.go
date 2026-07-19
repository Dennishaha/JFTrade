package adk

import (
	"context"
	"strings"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

func finishWorkflowChildren(finishes []func()) {
	for _, finish := range finishes {
		finish()
	}
}

func (e *WorkflowExecutor) startWorkflowChildRuns(ctx context.Context, req workflowRequest, parent Run, steps []workflowStep, tasks []Task) ([]Run, []func(), error) {
	childRuns := make([]Run, 0, len(steps))
	finishes := make([]func(), 0, len(steps))
	for index, step := range steps {
		if index < len(tasks) {
			_, jftradeErr11 := e.runtime.store.UpdateTask(ctx, tasks[index].ID, TaskPatchRequest{Status: new("IN_PROGRESS")})
			besteffort.LogError(jftradeErr11)
		}
		childAgent, err := e.runtime.workflowChildAgentForStep(ctx, req.Agent, step)
		if err != nil {
			for _, finish := range finishes {
				finish()
			}
			return nil, nil, err
		}
		if _, err := e.runtime.googleADKModelForAgent(ctx, childAgent); err != nil {
			for _, finish := range finishes {
				finish()
			}
			return nil, nil, err
		}
		child, _, finishChild, err := e.runtime.startRunWithOptions(ctx, req.Session.ID, childAgent, step.Message, runStartOptions{
			WorkMode:       WorkModeChat,
			Objective:      req.Objective,
			ParentRunID:    parent.ID,
			Iteration:      index + 1,
			WorkflowEngine: defaultString(parent.WorkflowEngine, WorkflowEngineADK2Loop),
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
			_, jftradeErr10 := e.runtime.store.UpdateTask(ctx, tasks[index].ID, TaskPatchRequest{RunID: &child.ID})
			besteffort.LogError(jftradeErr10)
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
		if !workflowChildHasExecutionActivity(execution, child, toolContext, childApprovals, replyResult) {
			continue
		}
		child = hydrateRunExecutionResult(child, toolContext, childApprovals, "", "")
		response, err := e.runtime.completeChatRun(ctx, req.Session, child, child.UserMessage, toolContext, childApprovals, replyResult, nil)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func workflowChildHasExecutionActivity(
	execution *googleADKExecution,
	child Run,
	toolContext toolExecutionContext,
	approvals []Approval,
	replyResult openAIChatResult,
) bool {
	if len(approvals) > 0 || len(toolContext.calls) > 0 {
		return true
	}
	if strings.TrimSpace(replyResult.Reply) != "" || strings.TrimSpace(replyResult.ReasoningContent) != "" {
		return true
	}
	return execution != nil && execution.workflowRunObserved(child.ID)
}

func (e *WorkflowExecutor) ensureWorkflowChildrenFinalReplies(
	ctx context.Context,
	req workflowRequest,
	execution *googleADKExecution,
	childRuns []Run,
	steps []workflowStep,
	approvals []Approval,
) error {
	for index, child := range childRuns {
		if len(approvalsForRun(approvals, child.ID)) > 0 {
			continue
		}
		if !execution.runNeedsFinalSynthesis(child.ID) {
			continue
		}
		childAgent := req.Agent
		if index < len(steps) {
			resolved, err := e.runtime.workflowChildAgentForStep(ctx, req.Agent, steps[index])
			if err != nil {
				return err
			}
			childAgent = resolved
		}
		if err := e.runtime.runGoogleADKWorkflowChildFinalSynthesis(ctx, childAgent, req.Session, execution, child); err != nil {
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
	jftradeErr6 := e.runtime.persistRunTerminalState(context.Background(), child)
	besteffort.LogError(jftradeErr6)
	return cause
}

func (e *WorkflowExecutor) blockedWorkflowChildResult(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	task Task,
	iteration int,
	childAgent Agent,
	fallbackAgentID string,
	reason string,
) workflowChildResult {
	_, jftradeErr13 := e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{Status: new("BLOCKED"), RunID: new(parent.ID), ResultSummary: &reason})
	besteffort.LogError(jftradeErr13)
	agentID := strings.TrimSpace(childAgent.ID)
	if agentID == "" {
		agentID = strings.TrimSpace(fallbackAgentID)
	}
	failed := Run{
		ID:             parent.ID,
		SessionID:      req.Session.ID,
		AgentID:        agentID,
		ProviderID:     childAgent.ProviderID,
		Model:          childAgent.Model,
		ParentRunID:    parent.ID,
		Status:         RunStatusFailed,
		Message:        reason,
		FailureReason:  reason,
		ErrorCode:      runErrorCode(RunStatusFailed),
		WorkMode:       WorkModeChat,
		WorkflowEngine: defaultString(parent.WorkflowEngine, workflowEngineForMode(parent.WorkMode)),
		CreatedAt:      nowString(),
		UpdatedAt:      nowString(),
		Usage:          &RunUsage{},
	}
	return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Response: ChatResponse{Reply: reason, Session: req.Session, Run: failed}}
}

func (e *WorkflowExecutor) runChild(ctx context.Context, req workflowRequest, parent Run, step workflowStep, task Task, iteration int) workflowChildResult {
	_, jftradeErr14 := e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{Status: new("IN_PROGRESS"), Executor: new(workflowTaskExecutorChild)})
	besteffort.LogError(jftradeErr14)
	childAgent, err := e.runtime.workflowChildAgentForStep(ctx, req.Agent, step)
	if err != nil {
		return e.blockedWorkflowChildResult(ctx, req, parent, task, iteration, Agent{}, step.ChildAgentID, err.Error())
	}
	if _, err := e.runtime.googleADKModelForAgent(ctx, childAgent); err != nil {
		return e.blockedWorkflowChildResult(ctx, req, parent, task, iteration, childAgent, "", err.Error())
	}
	child, childCtx, finishChild, err := e.runtime.startRunWithOptions(ctx, req.Session.ID, childAgent, step.Message, runStartOptions{
		WorkMode:       WorkModeChat,
		Objective:      req.Objective,
		ParentRunID:    parent.ID,
		Iteration:      iteration,
		WorkflowEngine: defaultString(parent.WorkflowEngine, workflowEngineForMode(parent.WorkMode)),
	})
	if err != nil {
		_, jftradeErr13 := e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{Status: new("BLOCKED"), RunID: new(parent.ID)})
		besteffort.LogError(jftradeErr13)
		return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Err: err}
	}
	defer finishChild()
	_, jftradeErr8 := e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{RunID: &child.ID})
	besteffort.LogError(jftradeErr8)
	parent.ChildRunIDs = appendUniqueString(parent.ChildRunIDs, child.ID)
	parent = updateWorkflowPlanForChildAt(parent, child, workflowPlanIndexForTask(parent.WorkflowPlan, task.ID))
	if err := e.runtime.store.SaveRun(ctx, parent); err != nil {
		return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Err: err}
	}
	if err := emitWorkflowRunSnapshot(ctx, e.runtime, req, parent); err != nil {
		return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Err: err}
	}
	e.runtime.workflowChildMu.Lock()
	if err := e.runtime.maybeAutoCompactSessionDuringWorkflow(ctx, req.Session, childAgent, step.Message, req.OnDelta); err != nil {
		e.runtime.workflowChildMu.Unlock()
		return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Err: err}
	}
	childSession := req.Session
	if refreshed, ok, refreshErr := e.runtime.store.Session(ctx, req.Session.ID); refreshErr == nil && ok {
		childSession = refreshed
	}
	toolContext, approvals, replyResult, preToolContent, preToolReasoning, adkErr := e.runtime.executeGoogleADK(childCtx, childAgent, childSession, child.ID, step.Message, req.OnDelta)
	child = hydrateRunExecutionResult(child, toolContext, approvals, preToolContent, preToolReasoning)
	response, err := e.runtime.completeChatRun(ctx, childSession, child, step.Message, toolContext, approvals, replyResult, adkErr)
	e.runtime.workflowChildMu.Unlock()
	if err != nil {
		_, jftradeErr9 := e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{Status: new("BLOCKED")})
		besteffort.LogError(jftradeErr9)
		return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Response: response, Err: err}
	}
	status := "DONE"
	if response.Run.Status != RunStatusCompleted {
		status = "BLOCKED"
	}
	_, jftradeErr15 := e.runtime.store.UpdateTask(ctx, task.ID, TaskPatchRequest{
		Status:        &status,
		RunID:         &response.Run.ID,
		Executor:      new(workflowTaskExecutorChild),
		ResultSummary: new(strings.TrimSpace(response.Reply)),
	})
	besteffort.LogError(jftradeErr15)
	return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Response: response}
}

func emitWorkflowRunSnapshot(ctx context.Context, runtime *Runtime, req workflowRequest, run Run) error {
	if !req.EmitRun || req.OnDelta == nil {
		return nil
	}
	if runtime != nil {
		run = runtime.authoritativeRunSnapshot(ctx, run)
	}
	run = NormalizeRun(run)
	return req.OnDelta(ChatDelta{Run: &run})
}
