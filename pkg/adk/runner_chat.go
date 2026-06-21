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
	if !validWorkMode(req.WorkModeOverride) {
		return ChatResponse{}, fmt.Errorf("invalid work mode %q", req.WorkModeOverride)
	}
	permissionModeOverride := strings.TrimSpace(req.PermissionModeOverride)
	if permissionModeOverride != "" && !validPermissionMode(permissionModeOverride) {
		return ChatResponse{}, fmt.Errorf("invalid permission mode %q", permissionModeOverride)
	}
	agent, err := r.resolveAgent(ctx, req.AgentID)
	if err != nil {
		return ChatResponse{}, err
	}
	agent, err = r.prepareAgent(ctx, agent)
	if err != nil {
		return ChatResponse{}, err
	}
	if permissionModeOverride != "" {
		agent.PermissionMode = normalizePermissionMode(permissionModeOverride)
	}
	workMode, runOptions, objective, err := resolveChatWorkflowOptions(req, agent)
	if err != nil {
		return ChatResponse{}, err
	}
	agent.WorkMode = workMode
	session, err := r.resolveSession(ctx, req.SessionID, agent, text)
	if err != nil {
		return ChatResponse{}, err
	}
	if err := r.maybeAutoCompactSession(ctx, session, agent, text, onDelta); err != nil {
		return ChatResponse{}, err
	}
	if workMode != WorkModeChat {
		return r.workflowExecutor().Run(ctx, workflowRequest{
			Agent: agent, Session: session, Message: text, Mode: workMode, Objective: objective,
			RunOptions: runOptions, OnDelta: onDelta, EmitRun: emitRun,
		})
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
	run = hydrateRunExecutionResult(run, toolContext, approvals, preToolContent, preToolReasoning)
	return r.completeChatRun(ctx, session, run, text, toolContext, approvals, replyResult, adkErr)
}

func resolveChatWorkflowOptions(req ChatRequest, agent Agent) (string, RunOptions, string, error) {
	if !validWorkMode(req.WorkModeOverride) {
		return "", RunOptions{}, "", fmt.Errorf("invalid work mode %q", req.WorkModeOverride)
	}
	mode := normalizeWorkMode(agent.WorkMode)
	if strings.TrimSpace(req.WorkModeOverride) != "" {
		mode = normalizeWorkMode(req.WorkModeOverride)
	}
	options := RunOptions{
		LoopMaxIterations: normalizeLoopMaxIterations(agent.LoopMaxIterations),
	}
	if req.RunOptions != nil {
		if req.RunOptions.LoopMaxIterations > 0 {
			options.LoopMaxIterations = normalizeLoopMaxIterations(req.RunOptions.LoopMaxIterations)
		}
	}
	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		objective = strings.TrimSpace(req.Message)
	}
	return mode, options, objective, nil
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
		replyResult = openAIChatResult{Reply: userFacingADKError(adkErr)}
	} else {
		var toolFailure string
		run, toolFailure = markCompletedChatRun(run)
		if toolFailure != "" && strings.TrimSpace(replyResult.Reply) == "" {
			replyResult = openAIChatResult{Reply: toolFailure}
		}
		if err := r.persistRunTerminalState(ctx, run); err != nil {
			return ChatResponse{}, err
		}
	}
	var err error
	run, err = r.attachFinalAssistantMessage(ctx, session, run, replyResult)
	if err != nil {
		return ChatResponse{}, err
	}
	return r.projectedChatResponse(ctx, session, run, replyResult), nil
}

func (r *Runtime) finishPendingApprovalRun(ctx context.Context, session Session, run Run, approvals []Approval) (ChatResponse, error) {
	run.PendingApprovals = pendingApprovalsOnly(approvals)
	run.Status = RunStatusPending
	run.ResumeState = "waiting_approval"
	run.Message = "等待用户审批后继续执行。"
	if err := r.store.SaveRun(ctx, run); err != nil {
		return ChatResponse{}, err
	}
	r.audit(ctx, "run.awaiting_approval", run.ID, "Agent run is waiting for approval.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "pendingApprovals": len(run.PendingApprovals),
	})
	reply := "我已经准备好执行需要授权的操作，请先在 ADK 审批队列里确认或拒绝。"
	return r.projectedChatResponse(ctx, session, run, openAIChatResult{Reply: reply}), nil
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

func markCompletedChatRun(run Run) (Run, string) {
	run.Status = RunStatusCompleted
	run.CompletedAt = new(nowString())
	run.Message = "completed"
	run.FailureReason = ""
	run.ErrorCode = ""
	toolFailure := firstToolCallFailure(&run)
	run.Degraded = toolFailure != ""
	finalizeRunUsage(&run)
	return run, toolFailure
}

func (r *Runtime) persistRunActivitySnapshot(ctx context.Context, snapshot Run) (Run, error) {
	if r == nil || r.store == nil || strings.TrimSpace(snapshot.ID) == "" {
		return NormalizeRun(snapshot), nil
	}
	run, ok, err := r.store.Run(ctx, snapshot.ID)
	if err != nil {
		return Run{}, err
	}
	if ok {
		mergeRunActivitySnapshot(&run, snapshot)
		return r.saveRunPreservingUserGoalPause(ctx, run)
	}
	return r.saveRunPreservingUserGoalPause(ctx, snapshot)
}

func (r *Runtime) authoritativeRunSnapshot(ctx context.Context, run Run) Run {
	run = NormalizeRun(run)
	if r == nil || r.store == nil || strings.TrimSpace(run.ID) == "" {
		return run
	}
	stored, ok, err := r.store.Run(ctx, run.ID)
	if err != nil || !ok {
		return run
	}
	return NormalizeRun(stored)
}

func mergeRunActivitySnapshot(run *Run, snapshot Run) {
	if run == nil {
		return
	}
	if strings.TrimSpace(snapshot.SessionID) != "" {
		run.SessionID = snapshot.SessionID
	}
	if strings.TrimSpace(snapshot.AgentID) != "" {
		run.AgentID = snapshot.AgentID
	}
	if strings.TrimSpace(snapshot.ProviderID) != "" {
		run.ProviderID = snapshot.ProviderID
	}
	if strings.TrimSpace(snapshot.Status) != "" {
		run.Status = snapshot.Status
	}
	if strings.TrimSpace(snapshot.Message) != "" {
		run.Message = snapshot.Message
	}
	if strings.TrimSpace(snapshot.FailureReason) != "" {
		run.FailureReason = snapshot.FailureReason
	}
	if strings.TrimSpace(snapshot.ErrorCode) != "" {
		run.ErrorCode = snapshot.ErrorCode
	}
	if snapshot.Degraded {
		run.Degraded = true
	}
	if strings.TrimSpace(snapshot.PreToolContent) != "" {
		run.PreToolContent = snapshot.PreToolContent
	}
	if strings.TrimSpace(snapshot.PreToolReasoning) != "" {
		run.PreToolReasoning = snapshot.PreToolReasoning
	}
	if len(snapshot.ToolSummaries) > 0 {
		run.ToolSummaries = append([]string(nil), snapshot.ToolSummaries...)
	}
	if len(snapshot.ToolCalls) > 0 {
		run.ToolCalls = append([]ToolCall(nil), snapshot.ToolCalls...)
	}
	if len(snapshot.PendingApprovals) > 0 {
		run.PendingApprovals = append([]Approval(nil), snapshot.PendingApprovals...)
	}
	if strings.TrimSpace(snapshot.ResumeState) != "" {
		run.ResumeState = snapshot.ResumeState
	}
	if strings.TrimSpace(snapshot.FinalMessageID) != "" {
		run.FinalMessageID = snapshot.FinalMessageID
	}
	if snapshot.Usage != nil {
		run.Usage = new(*snapshot.Usage)
	}
	if strings.TrimSpace(snapshot.StartedAt) != "" {
		run.StartedAt = snapshot.StartedAt
	}
	if snapshot.CompletedAt != nil {
		run.CompletedAt = new(*snapshot.CompletedAt)
	}
	if snapshot.CancelledAt != nil {
		run.CancelledAt = new(*snapshot.CancelledAt)
	}
	if strings.TrimSpace(snapshot.OptimizationTaskID) != "" {
		run.OptimizationTaskID = snapshot.OptimizationTaskID
	}
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
	if err := appendADKEventWithStaleRetry(ctx, runtimeAppendLocks(r), r.rawSessionService, response.Session, event); err != nil {
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
	replyResult openAIChatResult,
) ChatResponse {
	run = r.authoritativeRunSnapshot(ctx, run)
	response := ChatResponse{
		Reply:            replyResult.Reply,
		ReasoningContent: replyResult.ReasoningContent,
		Session:          session,
		Run:              run,
		PendingApprovals: pendingApprovalsOnly(run.PendingApprovals),
		Timeline:         []TimelineEntry{},
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
		response.PendingApprovals = pendingApprovalsOnly(projection.PendingApprovals)
	}
	response.Run = applySessionProjectionToRun(response.Run, projection)
	response.Run.PendingApprovals = append([]Approval(nil), response.PendingApprovals...)
	if timeline, ok, timelineErr := r.store.SessionTimeline(ctx, session.ID); timelineErr == nil && ok {
		response.Timeline = normalizedTimelineEntries(timeline)
	}
	return NormalizeChatResponse(response)
}

func normalizedTimelineEntries(entries []TimelineEntry) []TimelineEntry {
	return normalizeTimelineEntries(entries)
}

func applySessionProjectionToRun(run Run, projection SessionProjection) Run {
	run.PendingApprovals = pendingApprovalsOnly(run.PendingApprovals)
	if strings.TrimSpace(projection.FinalMessageID) != "" {
		run.FinalMessageID = projection.FinalMessageID
	}
	if strings.TrimSpace(projection.PreToolContent) != "" {
		run.PreToolContent = projection.PreToolContent
	}
	if strings.TrimSpace(projection.PreToolReasoning) != "" {
		run.PreToolReasoning = projection.PreToolReasoning
	}
	projectedPendingApprovals := pendingApprovalsOnly(projection.PendingApprovals)
	if len(projectedPendingApprovals) > 0 {
		run.PendingApprovals = projectedPendingApprovals
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
	return NormalizeRun(run)
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

func (r *Runtime) maybeAutoCompactSession(ctx context.Context, session Session, agent Agent, pendingUserText string, onDelta func(ChatDelta) error) error {
	return r.maybeAutoCompactSessionWithOptions(ctx, session, agent, pendingUserText, onDelta, false)
}

func (r *Runtime) maybeAutoCompactSessionDuringWorkflow(ctx context.Context, session Session, agent Agent, pendingUserText string, onDelta func(ChatDelta) error) error {
	return r.maybeAutoCompactSessionWithOptions(ctx, session, agent, pendingUserText, onDelta, true)
}

func (r *Runtime) maybeAutoCompactSessionWithOptions(ctx context.Context, session Session, agent Agent, pendingUserText string, onDelta func(ChatDelta) error, allowActiveRun bool) error {
	if r == nil || r.contextManager == nil || strings.TrimSpace(session.ID) == "" {
		return nil
	}
	snapshot, err := r.contextManager.ProjectedSnapshot(ctx, session, agent, pendingUserText)
	if err != nil {
		return nil
	}
	mode, shouldCompact := r.contextManager.ShouldAutoCompact(snapshot)
	if !shouldCompact {
		return nil
	}
	if !allowActiveRun {
		active, err := r.contextManager.HasActiveRun(ctx, session.ID)
		if err != nil || active {
			return nil
		}
	}
	release, acquired := r.beginSessionCompaction(session.ID)
	if !acquired {
		return nil
	}
	defer release()
	reason := "context usage exceeded automatic compaction threshold"
	if mode == "aggressive" {
		reason = "context usage exceeded aggressive failsafe threshold"
	}
	notice := r.createContextCompactionNotice(ctx, session.ID)
	if err := emitContextCompactionNotice(onDelta, notice); err != nil {
		return err
	}
	compacted, err := r.contextManager.Compact(ctx, session, agent, SessionCompactRequest{
		Mode:    mode,
		Trigger: "auto",
		Reason:  reason,
	})
	if err != nil {
		notice = r.updateContextCompactionNotice(ctx, notice, TimelineStatusError, contextCompactionFailedText)
		return emitContextCompactionNotice(onDelta, notice)
	}
	notice = r.updateContextCompactionNotice(ctx, notice, TimelineStatusFinal, contextCompactionDoneText)
	if err := emitContextCompactionNotice(onDelta, notice); err != nil {
		return err
	}
	if onDelta != nil {
		if err := onDelta(ChatDelta{Context: &compacted}); err != nil {
			return err
		}
	}
	return nil
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
