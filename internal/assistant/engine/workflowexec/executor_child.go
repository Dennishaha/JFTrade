package workflowexec

import (
	"context"
	"errors"
	"fmt"
	"strings"

	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

func finishWorkflowChildren(finishes []func()) {
	for _, finish := range finishes {
		finish()
	}
}

func (e *WorkflowExecutor) StartWorkflowChildRuns(ctx context.Context, req workflowRequest, parent Run, steps []workflowStep, tasks []Task) ([]Run, []func(), error) {
	childRuns := make([]Run, 0, len(steps))
	finishes := make([]func(), 0, len(steps))
	for index, step := range steps {
		if index < len(tasks) {
			_, jftradeErr11 := e.runtime.WorkflowStore().UpdateTask(ctx, tasks[index].ID, TaskPatchRequest{Status: new("IN_PROGRESS")})
			besteffort.LogError(jftradeErr11)
		}
		childAgent, err := e.runtime.WorkflowChildAgentForStep(ctx, req.Agent, step)
		if err != nil {
			for _, finish := range finishes {
				finish()
			}
			return nil, nil, err
		}
		if _, err := e.runtime.GoogleADKModelForAgent(ctx, childAgent); err != nil {
			for _, finish := range finishes {
				finish()
			}
			return nil, nil, err
		}
		child, _, finishChild, err := e.runtime.StartRunWithOptions(ctx, req.Session.ID, childAgent, step.Message, jfadkmodel.RunStartOptions{
			WorkMode:       WorkModeChat,
			Objective:      req.Objective,
			ParentRunID:    parent.ID,
			Iteration:      index + 1,
			WorkflowEngine: jfadkmodel.DefaultString(parent.WorkflowEngine, WorkflowEngineADK2Loop),
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
			_, jftradeErr10 := e.runtime.WorkflowStore().UpdateTask(ctx, tasks[index].ID, TaskPatchRequest{RunID: &child.ID})
			besteffort.LogError(jftradeErr10)
		}
	}
	return childRuns, finishes, nil
}

func (e *WorkflowExecutor) CompleteWorkflowChildrenFromADK(
	ctx context.Context,
	req workflowRequest,
	execution jfadkmodel.WorkflowExecutionHandle,
	childRuns []Run,
	approvals []Approval,
) ([]ChatResponse, error) {
	responses := make([]ChatResponse, 0, len(childRuns))
	for _, child := range childRuns {
		childApprovals := jfadkmodel.ApprovalsForRun(approvals, child.ID)
		toolContext := execution.ToolContextForRun(child.ID)
		replyResult := execution.ResultForRun(child.ID)
		if !workflowChildHasExecutionActivity(execution, child, childApprovals, replyResult) {
			continue
		}
		child = jfadkmodel.HydrateRunExecutionResult(child, toolContext, childApprovals, "", "")
		childCtx, err := e.runtime.ActiveRunExecutionContext(ctx, child.ID)
		if err != nil {
			return nil, err
		}
		response, err := e.runtime.CompleteChatRun(childCtx, req.Session, child, child.UserMessage, toolContext, childApprovals, replyResult, nil)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func workflowChildHasExecutionActivity(
	execution jfadkmodel.WorkflowExecutionHandle,
	child Run,
	approvals []Approval,
	replyResult jfadkmodel.AssistantExecutionResult,
) bool {
	if len(approvals) > 0 || execution != nil && execution.HasToolCallsForRun(child.ID) {
		return true
	}
	if strings.TrimSpace(replyResult.Reply) != "" || strings.TrimSpace(replyResult.ReasoningContent) != "" {
		return true
	}
	return execution != nil && execution.WorkflowRunObserved(child.ID)
}

func (e *WorkflowExecutor) EnsureWorkflowChildrenFinalReplies(
	ctx context.Context,
	req workflowRequest,
	execution jfadkmodel.WorkflowExecutionHandle,
	childRuns []Run,
	steps []workflowStep,
	approvals []Approval,
) error {
	for index, child := range childRuns {
		if len(jfadkmodel.ApprovalsForRun(approvals, child.ID)) > 0 {
			continue
		}
		if !execution.RunNeedsFinalSynthesis(child.ID) {
			continue
		}
		childAgent := req.Agent
		if index < len(steps) {
			resolved, err := e.runtime.WorkflowChildAgentForStep(ctx, req.Agent, steps[index])
			if err != nil {
				return err
			}
			childAgent = resolved
		}
		if err := execution.RunGoogleADKWorkflowChildFinalSynthesis(ctx, childAgent, req.Session, child); err != nil {
			return e.FailWorkflowChildAfterMissingFinal(ctx, child, execution, err)
		}
		if execution.RunNeedsFinalSynthesis(child.ID) || !execution.RunHasPostToolText(child.ID) {
			return e.FailWorkflowChildAfterMissingFinal(ctx, child, execution, errADKMissingFinalReply())
		}
	}
	return nil
}

func (e *WorkflowExecutor) FailWorkflowChildAfterMissingFinal(
	ctx context.Context,
	child Run,
	execution jfadkmodel.WorkflowExecutionHandle,
	cause error,
) error {
	toolContext := execution.ToolContextForRun(child.ID)
	child = jfadkmodel.HydrateRunExecutionResult(child, toolContext, nil, "", "")
	child = jfadkmodel.MarkFailedChatRun(ctx, child, cause)
	childCtx, err := e.runtime.ActiveRunExecutionContext(ctx, child.ID)
	if err != nil {
		return errors.Join(cause, err)
	}
	if err := e.runtime.PersistRunTerminalState(context.WithoutCancel(childCtx), child); err != nil {
		return fmt.Errorf("persist failed workflow child state: %w", err)
	}
	return cause
}

func (e *WorkflowExecutor) BlockedWorkflowChildResult(
	ctx context.Context,
	req workflowRequest,
	parent Run,
	task Task,
	iteration int,
	childAgent Agent,
	fallbackAgentID string,
	reason string,
) workflowChildResult {
	_, jftradeErr13 := e.runtime.WorkflowStore().UpdateTask(ctx, task.ID, TaskPatchRequest{Status: new("BLOCKED"), RunID: new(parent.ID), ResultSummary: &reason})
	besteffort.LogError(jftradeErr13)
	agentID := strings.TrimSpace(childAgent.ID)
	if agentID == "" {
		agentID = strings.TrimSpace(fallbackAgentID)
	}
	failed := Run{
		ID:              parent.ID,
		SessionID:       req.Session.ID,
		AgentID:         agentID,
		ProviderID:      childAgent.ProviderID,
		Model:           childAgent.Model,
		ReasoningEffort: childAgent.ReasoningEffort,
		ParentRunID:     parent.ID,
		Status:          RunStatusFailed,
		Message:         reason,
		FailureReason:   reason,
		ErrorCode:       jfadkmodel.RunErrorCode(RunStatusFailed),
		WorkMode:        WorkModeChat,
		WorkflowEngine:  jfadkmodel.DefaultString(parent.WorkflowEngine, jfadkmodel.WorkflowEngineForMode(parent.WorkMode)),
		CreatedAt:       jfadkmodel.NowString(),
		UpdatedAt:       jfadkmodel.NowString(),
		Usage:           &RunUsage{},
	}
	return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Response: ChatResponse{Reply: reason, Session: req.Session, Run: failed}}
}

func (e *WorkflowExecutor) RunChild(ctx context.Context, req workflowRequest, parent Run, step workflowStep, task Task, iteration int) workflowChildResult {
	_, jftradeErr14 := e.runtime.WorkflowStore().UpdateTask(ctx, task.ID, TaskPatchRequest{Status: new("IN_PROGRESS"), Executor: new(workflowTaskExecutorChild)})
	besteffort.LogError(jftradeErr14)
	childAgent, err := e.runtime.WorkflowChildAgentForStep(ctx, req.Agent, step)
	if err != nil {
		return e.BlockedWorkflowChildResult(ctx, req, parent, task, iteration, Agent{}, step.ChildAgentID, err.Error())
	}
	if _, err := e.runtime.GoogleADKModelForAgent(ctx, childAgent); err != nil {
		return e.BlockedWorkflowChildResult(ctx, req, parent, task, iteration, childAgent, "", err.Error())
	}
	child, childCtx, finishChild, err := e.runtime.StartRunWithOptions(ctx, req.Session.ID, childAgent, step.Message, jfadkmodel.RunStartOptions{
		WorkMode:       WorkModeChat,
		Objective:      req.Objective,
		ParentRunID:    parent.ID,
		Iteration:      iteration,
		WorkflowEngine: jfadkmodel.DefaultString(parent.WorkflowEngine, jfadkmodel.WorkflowEngineForMode(parent.WorkMode)),
	})
	if err != nil {
		_, jftradeErr13 := e.runtime.WorkflowStore().UpdateTask(ctx, task.ID, TaskPatchRequest{Status: new("BLOCKED"), RunID: new(parent.ID)})
		besteffort.LogError(jftradeErr13)
		return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Err: err}
	}
	defer finishChild()
	_, jftradeErr8 := e.runtime.WorkflowStore().UpdateTask(ctx, task.ID, TaskPatchRequest{RunID: &child.ID})
	besteffort.LogError(jftradeErr8)
	parent.ChildRunIDs = jfadkmodel.AppendUniqueString(parent.ChildRunIDs, child.ID)
	parent = jfadkmodel.UpdateWorkflowPlanForChildAt(parent, child, jfadkmodel.WorkflowPlanIndexForTask(parent.WorkflowPlan, task.ID))
	if err := e.runtime.WorkflowStore().SaveRun(ctx, parent); err != nil {
		return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Err: err}
	}
	if err := emitWorkflowRunSnapshot(ctx, e.runtime, req, parent); err != nil {
		return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Err: err}
	}
	childSession := req.Session
	var (
		toolContext      jfadkmodel.ToolExecutionContext
		approvals        []Approval
		replyResult      jfadkmodel.AssistantExecutionResult
		preToolContent   string
		preToolReasoning string
		response         ChatResponse
	)
	lockErr := e.runtime.WithWorkflowChildLock(ctx, func() error {
		if err := e.runtime.MaybeAutoCompactSessionDuringWorkflow(ctx, req.Session, childAgent, step.Message, req.OnDelta); err != nil {
			return err
		}
		if refreshed, ok, refreshErr := e.runtime.WorkflowStore().Session(ctx, req.Session.ID); refreshErr == nil && ok {
			childSession = refreshed
		}
		var adkErr error
		toolContext, approvals, replyResult, preToolContent, preToolReasoning, adkErr = e.runtime.ExecuteGoogleADK(childCtx, childAgent, childSession, child.ID, step.Message, req.OnDelta)
		child = jfadkmodel.HydrateRunExecutionResult(child, toolContext, approvals, preToolContent, preToolReasoning)
		var err error
		response, err = e.runtime.CompleteChatRun(childCtx, childSession, child, step.Message, toolContext, approvals, replyResult, adkErr)
		return err
	})
	if lockErr != nil {
		_, jftradeErr9 := e.runtime.WorkflowStore().UpdateTask(ctx, task.ID, TaskPatchRequest{Status: new("BLOCKED")})
		besteffort.LogError(jftradeErr9)
		return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Response: response, Err: lockErr}
	}
	status := "DONE"
	if response.Run.Status != RunStatusCompleted {
		status = "BLOCKED"
	}
	_, jftradeErr15 := e.runtime.WorkflowStore().UpdateTask(ctx, task.ID, TaskPatchRequest{
		Status:        &status,
		RunID:         &response.Run.ID,
		Executor:      new(workflowTaskExecutorChild),
		ResultSummary: new(strings.TrimSpace(response.Reply)),
	})
	besteffort.LogError(jftradeErr15)
	return workflowChildResult{Index: iteration - 1, TaskID: task.ID, Response: response}
}

func emitWorkflowRunSnapshot(ctx context.Context, runtime jfadkmodel.WorkflowExecutorRuntime, req workflowRequest, run Run) error {
	if !req.EmitRun || req.OnDelta == nil {
		return nil
	}
	if runtime != nil {
		run = runtime.AuthoritativeRunSnapshot(ctx, run)
	}
	run = jfadkmodel.NormalizeRun(run)
	return req.OnDelta(ChatDelta{Run: &run})
}
