package adk

import (
	"context"
	"errors"
	"strings"

	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

func (r *Runtime) resumeGoogleADK(ctx context.Context, run Run) (Run, *Message, bool, error) {
	execution, handled, err := r.loadResumedExecution(ctx, run)
	if err != nil || !handled {
		return run, nil, handled, err
	}
	parts := approvalResolutionParts(run.PendingApprovals)
	if len(parts) == 0 {
		return run, nil, false, nil
	}
	if err := r.prepareResumedExecution(ctx, run, execution); err != nil {
		return run, nil, true, err
	}
	if err := execution.run(ctx, genai.NewContentFromParts(parts, genai.RoleUser)); err != nil {
		if errors.Is(err, adkworkflow.ErrNothingToResume) && strings.TrimSpace(run.ParentRunID) != "" {
			return r.resumeGoogleADKDirect(ctx, run)
		}
		return run, nil, true, err
	}
	seedResumedConfirmationIDs(execution, run.PendingApprovals)
	if updatedRun, waiting, err := r.handleResumedApprovals(ctx, run, execution); err != nil {
		return updatedRun, nil, true, err
	} else if waiting {
		return updatedRun, nil, true, nil
	}
	if !runHasDeniedApproval(run.PendingApprovals) && len(run.PendingApprovals) > 0 {
		jftradeErr := execution.appendVisibleTextForRun(run.ID, approvalResolutionSummary(run, run.PendingApprovals[0], true), "")
		jftradeLogError(jftradeErr)
	}
	return r.completeResumedExecution(ctx, run, execution)
}

func (r *Runtime) resumeGoogleADKDirect(ctx context.Context, run Run) (Run, *Message, bool, error) {
	execution, err := r.rehydrateGoogleADKExecution(ctx, run)
	if err != nil {
		return run, nil, true, err
	}
	if execution == nil {
		return run, nil, false, nil
	}
	parts := approvalResolutionParts(run.PendingApprovals)
	if len(parts) == 0 {
		return run, nil, false, nil
	}
	if err := r.prepareResumedExecution(ctx, run, execution); err != nil {
		return run, nil, true, err
	}
	if err := execution.run(ctx, genai.NewContentFromParts(parts, genai.RoleUser)); err != nil {
		if !isIgnorableDirectApprovalResumeError(err, execution.toolContextForRun(run.ID)) {
			return run, nil, true, err
		}
	}
	seedResumedConfirmationIDs(execution, run.PendingApprovals)
	if updatedRun, waiting, err := r.handleResumedApprovals(ctx, run, execution); err != nil {
		return updatedRun, nil, true, err
	} else if waiting {
		return updatedRun, nil, true, nil
	}
	return r.completeDirectResumedExecution(ctx, run, execution)
}

func isIgnorableDirectApprovalResumeError(err error, toolContext toolExecutionContext) bool {
	if err == nil || !strings.Contains(err.Error(), "no function call event found for function responses ids") {
		return false
	}
	if len(toolContext.calls) == 0 {
		return false
	}
	for _, call := range toolContext.calls {
		switch strings.ToUpper(strings.TrimSpace(call.Status)) {
		case "SUCCEEDED", "COMPLETED", "FAILED", "TIMED_OUT", "DENIED", "CANCELLED":
			continue
		default:
			return false
		}
	}
	return true
}

func (r *Runtime) loadResumedExecution(ctx context.Context, run Run) (*googleADKExecution, bool, error) {
	r.adkMu.Lock()
	execution := r.adkRuns[run.ID]
	r.adkMu.Unlock()
	if execution != nil {
		return execution, true, nil
	}
	execution, err := r.rehydrateGoogleADKExecution(ctx, run)
	if err != nil {
		return nil, true, err
	}
	if execution == nil {
		return nil, false, nil
	}
	return execution, true, nil
}

func (r *Runtime) prepareResumedExecution(ctx context.Context, run Run, execution *googleADKExecution) error {
	execution.detachDeltaSink()
	// Only reset reply buffers when this is a fresh (rehydrated) execution.
	// When the execution carries over from a previous approval round we
	// keep accumulating text so the final assistant message contains the
	// full conversation.
	if execution.reply.Len() == 0 && execution.reasoning.Len() == 0 {
		execution.reply.Reset()
		execution.reasoning.Reset()
	}
	return r.maybeAutoCompactSessionDuringWorkflow(ctx, Session{ID: run.SessionID, AgentID: run.AgentID}, execution.agent, run.UserMessage, nil)
}

func seedResumedConfirmationIDs(execution *googleADKExecution, approvals []Approval) {
	if execution.processedConfirmationIDs == nil {
		execution.processedConfirmationIDs = make(map[string]struct{})
	}
	for _, approval := range approvals {
		if approval.ConfirmationCallID != "" {
			execution.processedConfirmationIDs[approval.ConfirmationCallID] = struct{}{}
		}
	}
}

func (r *Runtime) handleResumedApprovals(ctx context.Context, run Run, execution *googleADKExecution) (Run, bool, error) {
	newApprovals, err := execution.pendingApprovals(ctx, r.store)
	if err != nil {
		return run, false, err
	}
	if len(newApprovals) == 0 {
		return run, false, nil
	}
	run = persistResumedApprovalMessage(ctx, r, run, execution)
	run = hydrateResumedRunWithApprovals(run, execution, newApprovals)
	run.Status = RunStatusPending
	run.ResumeState = "waiting_approval"
	run.Message = "等待用户审批后继续执行。"
	if err := r.store.SaveRun(ctx, run); err != nil {
		return run, false, err
	}
	r.audit(ctx, "run.awaiting_approval", run.ID, "Agent run is waiting for approval.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "pendingApprovals": len(run.PendingApprovals),
	})
	return run, true, nil
}

func persistResumedApprovalMessage(ctx context.Context, r *Runtime, run Run, execution *googleADKExecution) Run {
	// Save the assistant message accumulated so far so the timeline
	// renders the LLM's analysis between approval rounds.
	result := execution.resultForRun(run.ID)
	if strings.TrimSpace(result.Reply) == "" && strings.TrimSpace(result.ReasoningContent) == "" {
		return run
	}
	message, err := r.ensureAssistantMessage(ctx, Session{ID: run.SessionID, AgentID: run.AgentID}, run, result)
	if err == nil {
		run.FinalMessageID = message.ID
	}
	return run
}

func (r *Runtime) completeResumedExecution(ctx context.Context, run Run, execution *googleADKExecution) (Run, *Message, bool, error) {
	initialDenied := runHasDeniedApproval(run.PendingApprovals)
	if !initialDenied {
		if err := r.ensureGoogleADKFinalReply(ctx, execution.agent, Session{ID: run.SessionID, AgentID: run.AgentID}, execution, run.ID, run.UserMessage); err != nil {
			return r.failResumedExecution(ctx, run, execution, err)
		}
	}
	run, denied := hydrateResumedRun(run, execution)
	result := finalizeResumedResult(run, execution.resultForRun(run.ID), denied)
	run.CompletedAt = new(nowString())
	message, err := r.persistResumedRunResult(ctx, run, result)
	if err != nil {
		return run, nil, true, err
	}
	r.auditResumedRun(ctx, run)
	r.deleteADKRun(run.ID)
	return run, message, true, nil
}

func (r *Runtime) completeDirectResumedExecution(ctx context.Context, run Run, execution *googleADKExecution) (Run, *Message, bool, error) {
	run, denied := hydrateResumedRun(run, execution)
	result := finalizeResumedResult(run, execution.resultForRun(run.ID), denied)
	run.CompletedAt = new(nowString())
	message, err := r.persistResumedRunResult(ctx, run, result)
	if err != nil {
		return run, nil, true, err
	}
	r.auditResumedRun(ctx, run)
	r.deleteADKRun(run.ID)
	return run, message, true, nil
}

func (r *Runtime) failResumedExecution(ctx context.Context, run Run, execution *googleADKExecution, cause error) (Run, *Message, bool, error) {
	run, _ = hydrateResumedRun(run, execution)
	run = markFailedChatRun(ctx, run, cause)
	if err := r.persistRunTerminalState(ctx, run); err != nil {
		return run, nil, true, err
	}
	r.deleteADKRun(run.ID)
	return run, nil, true, nil
}

func finalizeResumedResult(run Run, result openAIChatResult, denied bool) openAIChatResult {
	if denied {
		result.Reply = approvalResolutionSummary(run, run.PendingApprovals[0], false)
		result.ReasoningContent = ""
	} else if result.Reply == "" {
		result.Reply = approvalResolutionSummary(run, run.PendingApprovals[0], true)
	}
	return result
}

func (r *Runtime) deleteADKRun(runID string) {
	r.adkMu.Lock()
	defer r.adkMu.Unlock()
	delete(r.adkRuns, runID)
}

func approvalResolutionParts(approvals []Approval) []*genai.Part {
	parts := make([]*genai.Part, 0, len(approvals))
	for _, approval := range approvals {
		if approval.ConfirmationCallID == "" {
			continue
		}
		parts = append(parts, &genai.Part{FunctionResponse: &genai.FunctionResponse{
			Name: toolconfirmation.FunctionCallName,
			ID:   approval.ConfirmationCallID,
			Response: map[string]any{
				"confirmed": approval.Status == ApprovalStatusApproved,
			},
		}})
	}
	return parts
}

func hydrateResumedRun(run Run, execution *googleADKExecution) (Run, bool) {
	toolContext := execution.toolContextForRun(run.ID)
	run.ToolCalls = toolContext.calls
	run.ToolSummaries = toolContext.summaries
	run.PreToolContent, run.PreToolReasoning = execution.preToolState()
	run.OptimizationTaskID = optimizationTaskID(toolContext.calls)
	run.ResumeState = "adk_confirmation_resolved"
	run.Status = RunStatusCompleted
	run.Message = "completed"

	denied := runHasDeniedApproval(run.PendingApprovals)
	if denied {
		run = markDeniedResumedRun(run)
	} else {
		run = markFailedResumedRunIfNeeded(run)
	}
	return run, denied
}

// hydrateResumedRunWithApprovals updates the run with the execution state
// while keeping the run in a pending-approval state so further confirmation
// rounds can be processed.
func hydrateResumedRunWithApprovals(run Run, execution *googleADKExecution, newApprovals []Approval) Run {
	toolContext := execution.toolContextForRun(run.ID)
	run.ToolCalls = toolContext.calls
	run.ToolSummaries = toolContext.summaries
	run.PreToolContent, run.PreToolReasoning = execution.preToolState()
	run.OptimizationTaskID = optimizationTaskID(toolContext.calls)
	run.PendingApprovals = newApprovals
	run.ResumeState = "waiting_approval"
	return run
}

func runHasDeniedApproval(approvals []Approval) bool {
	for _, approval := range approvals {
		if approval.Status == ApprovalStatusDenied {
			return true
		}
	}
	return false
}

func markDeniedResumedRun(run Run) Run {
	run.Status = RunStatusDenied
	run.ResumeState = "approval_denied"
	run.Message = "approval denied"
	run.ErrorCode = ""
	run.FailureReason = ""
	for index := range run.ToolCalls {
		call := &run.ToolCalls[index]
		if call.Status == "FAILED" && call.Error != nil &&
			strings.Contains(*call.Error, adktool.ErrConfirmationRejected.Error()) {
			call.Status = "DENIED"
			call.Error = nil
			call.RequiresUser = false
		}
	}
	return run
}

func markFailedResumedRunIfNeeded(run Run) Run {
	run.FailureReason = ""
	run.ErrorCode = ""
	run.Degraded = firstToolCallFailure(&run) != ""
	return run
}

func (r *Runtime) persistResumedRunResult(ctx context.Context, run Run, result openAIChatResult) (*Message, error) {
	message, err := r.ensureAssistantMessage(ctx, Session{ID: run.SessionID, AgentID: run.AgentID}, run, result)
	if err != nil {
		return nil, err
	}
	run.FinalMessageID = message.ID
	if err := r.store.SaveRun(ctx, run); err != nil {
		return nil, err
	}
	return &message, nil
}

func (r *Runtime) auditResumedRun(ctx context.Context, run Run) {
	r.audit(ctx, "run.resumed", run.ID, "Agent run resumed after approval resolution.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "resumeState": run.ResumeState,
	})
	r.audit(ctx, runLifecycleAuditKind(run.Status), run.ID, "Agent run reached a terminal state after approval resolution.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "resumeState": run.ResumeState, "failureReason": run.FailureReason,
	})
}
