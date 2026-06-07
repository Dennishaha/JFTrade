package adk

import (
	"context"
	"fmt"
	"strings"
)

func (r *Runtime) runChat(ctx context.Context, req ChatRequest, onDelta func(ChatDelta) error, emitRun bool) (ChatResponse, error) {
	text, err := r.prepareChatRequest(ctx, req)
	if err != nil {
		return ChatResponse{}, err
	}
	defer func() { <-r.runSem }()
	agent, err := r.resolveAgent(ctx, req.AgentID)
	if err != nil {
		return ChatResponse{}, err
	}
	agent, err = r.prepareAgent(ctx, agent)
	if err != nil {
		return ChatResponse{}, err
	}
	session, err := r.resolveSession(ctx, req.SessionID, agent, text)
	if err != nil {
		return ChatResponse{}, err
	}
	run, runCtx, finishRun, err := r.startRun(ctx, session.ID, agent, text)
	if err != nil {
		return ChatResponse{}, err
	}
	defer finishRun()
	if emitRun && onDelta != nil {
		if err := onDelta(ChatDelta{Run: &run}); err != nil {
			return ChatResponse{}, err
		}
	}
	toolContext, approvals, replyResult, preToolContent, preToolReasoning, adkErr := r.executeGoogleADK(runCtx, agent, session, run.ID, text, onDelta)
	if _, err := r.store.AddMessage(ctx, session.ID, "user", text, ""); err != nil {
		return ChatResponse{}, err
	}
	run = hydrateRunExecutionResult(run, toolContext, approvals, preToolContent, preToolReasoning)
	return r.completeChatRun(ctx, session, run, text, toolContext, approvals, replyResult, adkErr)
}

func (r *Runtime) completeChatRun(
	ctx context.Context,
	session Session,
	run Run,
	text string,
	toolContext toolExecutionContext,
	approvals []Approval,
	replyResult openAIChatResult,
	adkErr error,
) (ChatResponse, error) {
	if len(approvals) > 0 {
		return r.finishPendingApprovalRun(ctx, session, run, approvals)
	}
	if adkErr != nil {
		run = markFailedChatRun(ctx, run, adkErr)
		if err := r.persistRunTerminalState(ctx, run); err != nil {
			return ChatResponse{}, err
		}
		replyResult = openAIChatResult{Reply: localReply(text, toolContext.summaries, adkErr)}
	} else {
		run = markCompletedChatRun(run)
		if err := r.persistRunTerminalState(ctx, run); err != nil {
			return ChatResponse{}, err
		}
	}
	var err error
	run, err = r.attachFinalAssistantMessage(ctx, session, run, replyResult)
	if err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{
		Reply:            replyResult.Reply,
		ReasoningContent: replyResult.ReasoningContent,
		Session:          session,
		Run:              run,
		PendingApprovals: approvals,
	}, nil
}

func (r *Runtime) finishPendingApprovalRun(ctx context.Context, session Session, run Run, approvals []Approval) (ChatResponse, error) {
	run.Status = RunStatusPending
	run.ResumeState = "waiting_approval"
	run.Message = "等待用户审批后继续执行。"
	if err := r.store.SaveRun(ctx, run); err != nil {
		return ChatResponse{}, err
	}
	r.audit(ctx, "run.awaiting_approval", run.ID, "Agent run is waiting for approval.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "pendingApprovals": len(approvals),
	})
	reply := "我已经准备好执行需要授权的操作，请先在 ADK 审批队列里确认或拒绝。"
	if _, err := r.store.AddMessage(ctx, session.ID, "assistant", reply, ""); err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{Reply: reply, Session: session, Run: run, PendingApprovals: approvals}, nil
}

func (r *Runtime) prepareChatRequest(ctx context.Context, req ChatRequest) (string, error) {
	if r == nil || r.store == nil {
		return "", fmt.Errorf("adk runtime is unavailable")
	}
	r.ReconcileExpiredRuns(ctx)
	text := strings.TrimSpace(req.Message)
	if text == "" {
		return "", fmt.Errorf("message is required")
	}
	if len([]rune(text)) > MaxMessageLength {
		return "", fmt.Errorf("message exceeds maximum length of %d characters", MaxMessageLength)
	}
	select {
	case r.runSem <- struct{}{}:
		return text, nil
	default:
		return "", fmt.Errorf("maximum concurrent runs (%d) reached, please try again later", MaxConcurrentRuns)
	}
}

func hydrateRunExecutionResult(
	run Run,
	toolContext toolExecutionContext,
	approvals []Approval,
	preToolContent string,
	preToolReasoning string,
) Run {
	run.ToolCalls = toolContext.calls
	run.ToolSummaries = toolContext.summaries
	run.PreToolContent = preToolContent
	run.PreToolReasoning = preToolReasoning
	run.OptimizationTaskID = optimizationTaskID(toolContext.calls)
	run.PendingApprovals = approvals
	if run.Usage != nil {
		run.Usage.ToolCallsTotal = len(toolContext.calls)
	}
	return run
}

func markFailedChatRun(ctx context.Context, run Run, adkErr error) Run {
	run.Status = runStatusForContext(ctx, adkErr)
	run.Message = adkErr.Error()
	run.FailureReason = adkErr.Error()
	run.ErrorCode = runErrorCode(run.Status)
	run.Degraded = true
	completedAt := nowString()
	run.CompletedAt = &completedAt
	if run.Status == RunStatusCancelled {
		run.CancelledAt = &completedAt
	}
	finalizeRunUsage(&run)
	return run
}

func markCompletedChatRun(run Run) Run {
	completedAt := nowString()
	run.Status = RunStatusCompleted
	run.CompletedAt = &completedAt
	run.Message = "completed"
	finalizeRunUsage(&run)
	return run
}

func (r *Runtime) persistRunTerminalState(ctx context.Context, run Run) error {
	if err := r.store.SaveRun(ctx, run); err != nil {
		return err
	}
	r.audit(ctx, runLifecycleAuditKind(run.Status), run.ID, terminalAuditMessage(run.Status), terminalAuditFields(run))
	return nil
}

func (r *Runtime) attachFinalAssistantMessage(
	ctx context.Context,
	session Session,
	run Run,
	replyResult openAIChatResult,
) (Run, error) {
	message, err := r.store.AddMessage(ctx, session.ID, "assistant", replyResult.Reply, replyResult.ReasoningContent)
	if err != nil {
		return run, err
	}
	run.FinalMessageID = message.ID
	if err := r.store.SaveRun(ctx, run); err != nil {
		return run, err
	}
	return run, nil
}

func terminalAuditMessage(status string) string {
	if status == RunStatusCompleted {
		return "Agent run completed."
	}
	return "Agent run finished with a terminal status."
}

func terminalAuditFields(run Run) map[string]any {
	fields := map[string]any{
		"runId":   run.ID,
		"agentId": run.AgentID,
		"status":  run.Status,
	}
	if run.ErrorCode != "" {
		fields["errorCode"] = run.ErrorCode
	}
	if run.FailureReason != "" {
		fields["failureReason"] = run.FailureReason
	}
	return fields
}
