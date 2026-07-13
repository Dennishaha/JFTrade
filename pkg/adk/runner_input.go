package adk

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

func (r *Runtime) ResolveInputAsync(ctx context.Context, runID string, payload InputResponseRequest) (InputResolution, error) {
	run, changed, err := r.store.ResolveRunInput(ctx, runID, payload)
	if err != nil {
		return InputResolution{}, err
	}
	resolution := InputResolution{Run: &run}
	for index := range run.InputRequests {
		if run.InputRequests[index].ID == strings.TrimSpace(payload.RequestID) {
			resolution.Request = *normalizeInputRequest(&run.InputRequests[index])
			break
		}
	}
	if parent, err := r.syncParentWorkflowFromChild(ctx, run); err != nil {
		return InputResolution{}, err
	} else if parent != nil {
		resolution.ParentRun = parent
	}
	if !changed {
		return resolution, nil
	}
	r.audit(ctx, "input.resolved", run.InputRequest.ID, "Agent input request resolved.", map[string]any{
		"runId": run.ID, "requestId": run.InputRequest.ID, "answers": len(run.InputRequest.Answers),
	})
	go r.continueResolvedInput(context.Background(), run.ID)
	return resolution, nil
}

func (r *Runtime) continueResolvedInput(ctx context.Context, runID string) {
	run, ok, err := r.store.Run(ctx, runID)
	if err != nil || !ok || run.InputRequest == nil || run.InputRequest.Status != InputRequestStatusAnswered || run.Status != RunStatusRunning {
		return
	}
	execution, err := r.resumeAnsweredInput(ctx, run)
	if err != nil {
		r.failInputContinuation(ctx, run, err)
		return
	}
	if paused, err := r.pauseForNextInput(ctx, run, execution); err != nil {
		r.failInputContinuation(ctx, run, err)
		return
	} else if paused {
		return
	}
	if paused, err := r.pauseForApprovalAfterInput(ctx, run, execution); err != nil {
		r.failInputContinuation(ctx, run, err)
		return
	} else if paused {
		return
	}
	if err := r.completeInputContinuation(ctx, run, execution); err != nil {
		r.failInputContinuation(ctx, run, err)
	}
}

func (r *Runtime) resumeAnsweredInput(ctx context.Context, run Run) (*googleADKExecution, error) {
	execution, handled, err := r.loadResumedExecution(ctx, run)
	if err != nil || !handled || execution == nil {
		return nil, defaultError(err, "GO-ADK input context could not be recovered")
	}
	seedResumedExecutionState(execution, run)
	if err := r.prepareResumedExecution(ctx, run, execution); err != nil {
		return nil, err
	}
	content := &genai.Content{Role: genai.RoleUser, Parts: []*genai.Part{{FunctionResponse: &genai.FunctionResponse{
		ID: run.InputRequest.FunctionCallID, Name: interactionRequestUserTool, Response: inputResponsePayload(*run.InputRequest),
	}}}}
	if err := execution.run(ctx, content); err != nil {
		return nil, err
	}
	return execution, nil
}

func (r *Runtime) pauseForNextInput(ctx context.Context, run Run, execution *googleADKExecution) (bool, error) {
	inputRequests, err := r.pendingInputRequests(ctx, execution)
	if err != nil {
		return false, err
	}
	nextRequest := inputRequests[run.ID]
	if nextRequest == nil {
		return false, nil
	}
	execution.setInputRequests(inputRequests)
	toolContext := execution.toolContextForRun(run.ID)
	run.ToolCalls = toolContext.calls
	run.ToolSummaries = toolContext.summaries
	run.PreToolContent, run.PreToolReasoning = execution.preToolState()
	if _, err := r.finishPendingInputRun(ctx, Session{ID: run.SessionID, AgentID: run.AgentID}, run, nextRequest); err != nil {
		return false, err
	}
	if refreshed, ok, _ := r.store.Run(ctx, run.ID); ok {
		_, _ = r.syncParentWorkflowFromChild(ctx, refreshed)
	}
	return true, nil
}

func (r *Runtime) pauseForApprovalAfterInput(ctx context.Context, run Run, execution *googleADKExecution) (bool, error) {
	approvals, err := execution.pendingApprovals(ctx, r.store)
	if err != nil || len(approvals) == 0 {
		return false, err
	}
	run = hydrateResumedRunWithApprovals(run, execution, approvals)
	run.InputRequest = normalizeInputRequest(run.InputRequest)
	run.Status = RunStatusPending
	run.ResumeState = "waiting_approval"
	run.Message = "等待用户审批后继续执行。"
	if err := r.store.SaveRun(ctx, run); err != nil {
		return false, err
	}
	_, _ = r.syncParentWorkflowFromChild(ctx, run)
	return true, nil
}

func (r *Runtime) completeInputContinuation(ctx context.Context, run Run, execution *googleADKExecution) error {
	if err := r.ensureGoogleADKFinalReply(ctx, execution.agent, Session{ID: run.SessionID, AgentID: run.AgentID}, execution, run.ID, run.UserMessage); err != nil {
		return err
	}
	toolContext := execution.toolContextForRun(run.ID)
	run.ToolCalls = toolContext.calls
	run.ToolSummaries = toolContext.summaries
	run.PreToolContent, run.PreToolReasoning = execution.preToolState()
	run.OptimizationTaskID = optimizationTaskID(toolContext.calls)
	run.Status = RunStatusCompleted
	run.ResumeState = "input_resolved"
	run.Message = "completed"
	run.CompletedAt = new(nowString())
	run.FailureReason = ""
	run.ErrorCode = ""
	run.Degraded = firstToolCallFailure(&run) != ""
	finalizeRunUsage(&run)
	result := execution.resultForRun(run.ID)
	if strings.TrimSpace(result.Reply) == "" {
		result.Reply = "已根据你的选择继续执行。"
	}
	message, err := r.ensureAssistantMessage(ctx, Session{ID: run.SessionID, AgentID: run.AgentID}, run, result)
	if err != nil {
		return err
	}
	run.FinalMessageID = message.ID
	if err := r.store.SaveRun(ctx, run); err != nil {
		return err
	}
	r.deleteADKRun(run.ID)
	r.audit(ctx, "run.input_resolved", run.ID, "Agent run completed after user input.", map[string]any{"runId": run.ID})
	_, _ = r.continueParentWorkflowAfterChild(ctx, run)
	return nil
}

func (r *Runtime) failInputContinuation(ctx context.Context, run Run, cause error) {
	if cause == nil {
		cause = fmt.Errorf("input continuation failed")
	}
	run = markFailedChatRun(ctx, run, cause)
	run.ResumeState = "input_resume_failed"
	_ = r.persistRunTerminalState(ctx, run)
	r.deleteADKRun(run.ID)
	_, _ = r.continueParentWorkflowAfterChild(ctx, run)
}

func defaultError(err error, message string) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("%s", message)
}
