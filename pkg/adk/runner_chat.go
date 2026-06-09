package adk

import (
	"context"
	"fmt"
	"strings"

	adkmodel "google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"
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
	r.maybeAutoCompactSession(ctx, session, agent, text)
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
	return r.projectedChatResponse(ctx, session, run, approvals, replyResult), nil
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
	return r.projectedChatResponse(ctx, session, run, approvals, openAIChatResult{Reply: reply}), nil
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
	message, err := r.ensureAssistantMessage(ctx, session, run, replyResult)
	if err != nil {
		return run, err
	}
	run.FinalMessageID = message.ID
	if err := r.store.SaveRun(ctx, run); err != nil {
		return run, err
	}
	return run, nil
}

func (r *Runtime) ensureAssistantMessage(
	ctx context.Context,
	session Session,
	run Run,
	replyResult openAIChatResult,
) (Message, error) {
	if r != nil && r.store != nil {
		if projection, ok, err := r.store.SessionProjection(ctx, session.ID); err != nil {
			return Message{}, err
		} else if ok && projection.LatestAssistant != nil && projectedMessageMatchesReply(*projection.LatestAssistant, replyResult) {
			return *projection.LatestAssistant, nil
		}
	}
	return r.appendAssistantMessageEvent(ctx, session, run, replyResult)
}

func (r *Runtime) appendAssistantMessageEvent(
	ctx context.Context,
	session Session,
	run Run,
	replyResult openAIChatResult,
) (Message, error) {
	if r == nil || r.rawSessionService == nil {
		return Message{}, fmt.Errorf("adk session service is unavailable")
	}
	response, err := r.rawSessionService.Get(ctx, &adksession.GetRequest{
		AppName:   googleADKAppName(defaultString(session.AgentID, run.AgentID)),
		UserID:    googleADKUserID,
		SessionID: session.ID,
	})
	if err != nil {
		created, createErr := r.rawSessionService.Create(ctx, &adksession.CreateRequest{
			AppName:   googleADKAppName(defaultString(session.AgentID, run.AgentID)),
			UserID:    googleADKUserID,
			SessionID: session.ID,
		})
		if createErr != nil {
			return Message{}, createErr
		}
		response = &adksession.GetResponse{Session: created.Session}
	}
	event := adksession.NewEvent(run.ID)
	event.Author = googleADKAgentName(defaultString(run.AgentID, session.AgentID))
	event.LLMResponse = adkmodel.LLMResponse{
		Content:      genai.NewContentFromParts(partsFromReplyAndReasoning(replyResult.Reply, replyResult.ReasoningContent), genai.RoleModel),
		TurnComplete: true,
	}
	if err := r.rawSessionService.AppendEvent(ctx, response.Session, event); err != nil {
		return Message{}, err
	}
	message, _ := transcriptEntryFromADKEvent(event)
	message.SessionID = session.ID
	message.RunID = run.ID
	return message, nil
}

func projectedMessageMatchesReply(message Message, replyResult openAIChatResult) bool {
	return strings.TrimSpace(message.Content) == strings.TrimSpace(replyResult.Reply) &&
		strings.TrimSpace(message.ReasoningContent) == strings.TrimSpace(replyResult.ReasoningContent)
}

func (r *Runtime) projectedChatResponse(
	ctx context.Context,
	session Session,
	run Run,
	approvals []Approval,
	fallback openAIChatResult,
) ChatResponse {
	response := ChatResponse{
		Reply:            fallback.Reply,
		ReasoningContent: fallback.ReasoningContent,
		Session:          session,
		Run:              run,
		PendingApprovals: append([]Approval(nil), approvals...),
		Context:          r.contextSnapshotOrNil(ctx, session.ID),
	}
	if r == nil || r.store == nil {
		return response
	}
	projection, ok, err := r.store.SessionProjection(ctx, session.ID)
	if err != nil || !ok {
		return response
	}
	if projection.LatestAssistant != nil {
		response.Reply = projection.LatestAssistant.Content
		response.ReasoningContent = projection.LatestAssistant.ReasoningContent
	}
	if len(response.PendingApprovals) == 0 && len(projection.PendingApprovals) > 0 {
		response.PendingApprovals = append([]Approval(nil), projection.PendingApprovals...)
	}
	if len(response.PendingApprovals) > 0 {
		response.Run.PendingApprovals = append([]Approval(nil), response.PendingApprovals...)
	}
	response.Run = applySessionProjectionToRun(response.Run, projection)
	if timeline, ok, timelineErr := r.store.SessionTimeline(ctx, session.ID); timelineErr == nil && ok {
		response.Timeline = timeline
	}
	return response
}

func applySessionProjectionToRun(run Run, projection SessionProjection) Run {
	if strings.TrimSpace(projection.FinalMessageID) != "" {
		run.FinalMessageID = projection.FinalMessageID
	}
	if strings.TrimSpace(projection.PreToolContent) != "" {
		run.PreToolContent = projection.PreToolContent
	}
	if strings.TrimSpace(projection.PreToolReasoning) != "" {
		run.PreToolReasoning = projection.PreToolReasoning
	}
	if len(projection.PendingApprovals) > 0 {
		run.PendingApprovals = append([]Approval(nil), projection.PendingApprovals...)
	}
	if shouldPreferProjectedToolCalls(run.ToolCalls, projection.ToolCalls) {
		run.ToolCalls = append([]ToolCall(nil), projection.ToolCalls...)
	}
	if len(run.ToolCalls) > 0 {
		run.ToolSummaries = toolSummariesForRun(run)
		run.OptimizationTaskID = optimizationTaskID(run.ToolCalls)
		if run.Usage != nil {
			run.Usage.ToolCallsTotal = len(run.ToolCalls)
		}
	}
	return run
}

func shouldPreferProjectedToolCalls(current []ToolCall, projected []ToolCall) bool {
	if len(projected) == 0 {
		return false
	}
	if len(current) == 0 {
		return true
	}
	projectedTerminal := terminalToolCallCount(projected)
	currentTerminal := terminalToolCallCount(current)
	if projectedTerminal != currentTerminal {
		return projectedTerminal > currentTerminal
	}
	projectedPending := pendingApprovalToolCallCount(projected)
	currentPending := pendingApprovalToolCallCount(current)
	if projectedPending != currentPending {
		return projectedPending > currentPending
	}
	return len(projected) > len(current)
}

func terminalToolCallCount(calls []ToolCall) int {
	count := 0
	for _, call := range calls {
		switch call.Status {
		case "SUCCEEDED", "FAILED", "DENIED", "COMPLETED", "CANCELLED", "TIMED_OUT":
			count++
		}
	}
	return count
}

func pendingApprovalToolCallCount(calls []ToolCall) int {
	count := 0
	for _, call := range calls {
		if call.Status == "PENDING_APPROVAL" {
			count++
		}
	}
	return count
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

func (r *Runtime) maybeAutoCompactSession(ctx context.Context, session Session, agent Agent, pendingUserText string) {
	if r == nil || r.contextManager == nil || strings.TrimSpace(session.ID) == "" {
		return
	}
	snapshot, err := r.contextManager.ProjectedSnapshot(ctx, session, agent, pendingUserText)
	if err != nil {
		return
	}
	mode, shouldCompact := r.contextManager.ShouldAutoCompact(snapshot)
	if !shouldCompact {
		return
	}
	active, err := r.contextManager.HasActiveRun(ctx, session.ID)
	if err != nil || active {
		return
	}
	reason := "context usage exceeded automatic compaction threshold"
	if mode == "aggressive" {
		reason = "context usage exceeded aggressive failsafe threshold"
	}
	_, _ = r.contextManager.Compact(ctx, session, agent, SessionCompactRequest{
		Mode:    mode,
		Trigger: "auto",
		Reason:  reason,
	})
}

func (r *Runtime) contextSnapshotOrNil(ctx context.Context, sessionID string) *SessionContextSnapshot {
	if r == nil || r.contextManager == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	snapshot, err := r.SessionContext(ctx, sessionID)
	if err != nil {
		return nil
	}
	return &snapshot
}
