package adk

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	adkmodel "google.golang.org/adk/v2/model"
	adksession "google.golang.org/adk/v2/session"
	"google.golang.org/genai"

	"github.com/jftrade/jftrade-main/pkg/observability"
)

func (r *Runtime) runChat(ctx context.Context, req ChatRequest, onDelta func(ChatDelta) error, emitRun bool) (ChatResponse, error) {
	var requestFingerprint string
	var err error
	req, requestFingerprint, err = ensureChatRequestIdentity(req)
	if err != nil {
		return ChatResponse{}, err
	}
	response, reused, err := r.reusedChatResponse(ctx, req, requestFingerprint)
	if err != nil {
		return ChatResponse{}, err
	}
	if reused {
		return response, nil
	}
	text, err := r.prepareChatRequest(ctx, req)
	if err != nil {
		return ChatResponse{}, err
	}
	defer func() { <-r.runSem }()
	permissionModeOverride, err := validateChatOverrides(req)
	if err != nil {
		return ChatResponse{}, err
	}
	agent, err := r.resolveAgentDefinition(ctx, req.AgentID)
	if err != nil {
		return ChatResponse{}, err
	}
	agent = applyChatModelOverride(agent, req)
	agent, err = r.resolveAgentProvider(ctx, agent)
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
			ClientRequestID: req.ClientRequestID, RequestFingerprint: requestFingerprint,
		})
	}
	run, runCtx, finishRun, err := r.startRunWithOptions(ctx, session.ID, agent, text, runStartOptions{
		WorkMode: agent.WorkMode, ClientRequestID: req.ClientRequestID, RequestFingerprint: requestFingerprint,
	})
	if err != nil {
		var reused *reusedChatRequestError
		if errors.As(err, &reused) {
			return r.chatResponseForExistingRun(ctx, reused.Run)
		}
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
	return r.completeChatRun(runCtx, session, run, text, toolContext, approvals, replyResult, adkErr)
}

func (r *Runtime) reusedChatResponse(ctx context.Context, req ChatRequest, requestFingerprint string) (ChatResponse, bool, error) {
	if r == nil || r.store == nil {
		return ChatResponse{}, false, nil
	}
	existing, existingFingerprint, ok, err := r.store.ChatRunByClientRequestID(ctx, req.ClientRequestID)
	if err != nil {
		return ChatResponse{}, false, err
	}
	if !ok {
		return ChatResponse{}, false, nil
	}
	if existingFingerprint != requestFingerprint {
		observability.InfoWithImportance(ctx, observability.ImportanceNormal, "adk chat request conflict",
			"request_state", "conflict", "client_request_id", req.ClientRequestID, "run_id", existing.ID)
		return ChatResponse{}, false, &ChatRequestConflictError{ClientRequestID: req.ClientRequestID}
	}
	observability.InfoWithImportance(ctx, observability.ImportanceNormal, "adk chat request reused",
		"request_state", "reused", "client_request_id", req.ClientRequestID, "run_id", existing.ID)
	response, err := r.chatResponseForExistingRun(ctx, existing)
	return response, true, err
}

func (r *Runtime) chatResponseForExistingRun(ctx context.Context, run Run) (ChatResponse, error) {
	session, ok, err := r.store.Session(ctx, run.SessionID)
	if err != nil {
		return ChatResponse{}, err
	}
	if !ok {
		return ChatResponse{}, fmt.Errorf("session not found: %s", run.SessionID)
	}
	return r.projectedChatResponse(ctx, session, run, assistantExecutionResult{}), nil
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

func validateChatOverrides(req ChatRequest) (string, error) {
	if !validWorkMode(req.WorkModeOverride) {
		return "", fmt.Errorf("invalid work mode %q", req.WorkModeOverride)
	}
	permissionMode := strings.TrimSpace(req.PermissionModeOverride)
	if permissionMode != "" && !validPermissionMode(permissionMode) {
		return "", fmt.Errorf("invalid permission mode %q", permissionMode)
	}
	return permissionMode, nil
}

func (r *Runtime) completeChatRun(
	ctx context.Context,
	session Session,
	run Run,
	text string,
	toolContext toolExecutionContext,
	approvals []Approval,
	replyResult assistantExecutionResult,
	adkErr error,
) (ChatResponse, error) {
	ctx = adkRunObservabilityContext(ctx, run)
	if toolContext.inputRequest != nil {
		return r.finishPendingInputRun(ctx, session, run, toolContext.inputRequest)
	}
	if len(approvals) > 0 {
		return r.finishPendingApprovalRun(ctx, session, run, approvals)
	}
	if adkErr != nil {
		observability.ErrorWithImportance(ctx, observability.ImportanceHigh, "adk run failed", adkErr, "status", RunStatusFailed)
		run = markFailedChatRun(ctx, run, adkErr)
		if err := r.persistRunTerminalState(ctx, run); err != nil {
			return ChatResponse{}, err
		}
		replyResult = assistantExecutionResult{Reply: userFacingADKError(adkErr), SyntheticKind: "provider_error"}
	} else {
		var toolFailure string
		run, toolFailure = markCompletedChatRun(run)
		if toolFailure != "" && strings.TrimSpace(replyResult.Reply) == "" {
			replyResult = assistantExecutionResult{Reply: toolFailure, SyntheticKind: "tool_failure"}
		}
		if err := r.persistRunTerminalState(ctx, run); err != nil {
			return ChatResponse{}, err
		}
		observability.InfoWithImportance(ctx, observability.ImportanceNormal, "adk run finished", "status", run.Status, "tool_calls", len(run.ToolCalls))
	}
	var err error
	run, err = r.attachFinalAssistantMessage(ctx, session, run, replyResult)
	if err != nil {
		return ChatResponse{}, err
	}
	return r.projectedChatResponse(ctx, session, run, replyResult), nil
}

func (r *Runtime) finishPendingApprovalRun(ctx context.Context, session Session, run Run, approvals []Approval) (ChatResponse, error) {
	ctx = adkRunObservabilityContext(ctx, run)
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
	observability.InfoWithImportance(ctx, observability.ImportanceNormal, "adk run awaiting approval", "status", run.Status, "pending_approvals", len(run.PendingApprovals))
	reply := "我已经准备好执行需要授权的操作，请先在 ADK 审批队列里确认或拒绝。"
	return r.projectedChatResponse(ctx, session, run, assistantExecutionResult{Reply: reply}), nil
}

func applyChatModelOverride(agent Agent, req ChatRequest) Agent {
	if providerID := strings.TrimSpace(req.ProviderID); providerID != "" {
		agent.ProviderID = providerID
	}
	if model := strings.TrimSpace(req.Model); model != "" {
		agent.Model = model
	}
	return agent
}

func (r *Runtime) prepareChatRequest(ctx context.Context, req ChatRequest) (string, error) {
	if r == nil || r.store == nil {
		return "", fmt.Errorf("adk runtime is unavailable")
	}
	if err := r.ReconcileExpiredRuns(ctx); err != nil {
		return "", err
	}
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
	run.InputRequest = normalizeInputRequest(toolContext.inputRequest)
	if run.Usage != nil {
		run.Usage.ToolCallsTotal = len(toolContext.calls)
	}
	return run
}

func (r *Runtime) finishPendingInputRun(ctx context.Context, session Session, run Run, request *InputRequest) (ChatResponse, error) {
	run.InputRequest = normalizeInputRequest(request)
	run.InputRequests = appendInputRequestIfMissing(run.InputRequests, *request)
	run.Status = RunStatusPendingInput
	run.ResumeState = "waiting_input"
	run.Message = "等待用户回答后继续执行。"
	if err := r.store.SaveRun(ctx, run); err != nil {
		return ChatResponse{}, err
	}
	r.audit(ctx, "run.awaiting_input", run.ID, "Agent run is waiting for user input.", map[string]any{
		"runId": run.ID, "agentId": run.AgentID, "status": run.Status, "requestId": request.ID,
	})
	return r.projectedChatResponse(ctx, session, run, assistantExecutionResult{Reply: "我需要你确认几个选择，回答后会继续执行。"}), nil
}

func markFailedChatRun(ctx context.Context, run Run, adkErr error) Run {
	run.Status = runStatusForContext(ctx, adkErr)
	run.Message = adkErr.Error()
	run.FailureReason = adkErr.Error()
	run.ErrorCode = runErrorCode(run.Status, adkErr)
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
	replyResult assistantExecutionResult,
) (Run, error) {
	if strings.TrimSpace(replyResult.SourceEventID) == "" && strings.TrimSpace(replyResult.SyntheticKind) == "" {
		replyResult.SyntheticKind = "local_fallback"
	}
	message, err := r.ensureAssistantMessage(ctx, session, run, replyResult)
	if err != nil {
		return run, err
	}
	run.FinalMessageID = message.ID
	if err := r.store.SaveRun(ctx, run); err != nil {
		return run, err
	}
	finalSource := "synthetic"
	if strings.TrimSpace(replyResult.SourceEventID) != "" {
		finalSource = "native"
	}
	observability.InfoWithImportance(ctx, observability.ImportanceNormal, "adk final message attached",
		"final_event_source", finalSource, "final_message_id", message.ID, "synthetic_kind", replyResult.SyntheticKind)
	return run, nil
}

func (r *Runtime) ensureAssistantMessage(
	ctx context.Context,
	session Session,
	run Run,
	replyResult assistantExecutionResult,
) (TranscriptEntry, error) {
	if sourceID := strings.TrimSpace(replyResult.SourceEventID); sourceID != "" {
		message, ok, err := r.assistantMessageByID(ctx, session, sourceID)
		if err != nil {
			return TranscriptEntry{}, err
		}
		if !ok {
			return TranscriptEntry{}, fmt.Errorf("persisted ADK assistant event %s is missing from session %s", sourceID, session.ID)
		}
		return message, nil
	}
	return r.appendAssistantMessageEvent(ctx, session, run, replyResult)
}

func (r *Runtime) assistantMessageByID(ctx context.Context, session Session, messageID string) (TranscriptEntry, bool, error) {
	if r == nil || r.store == nil {
		return TranscriptEntry{}, false, fmt.Errorf("adk store is unavailable")
	}
	if r.rawSessionService != nil {
		response, err := r.rawSessionService.Get(ctx, &adksession.GetRequest{
			AppName: googleADKAppName(session.AgentID), UserID: googleADKUserID, SessionID: session.ID,
		})
		if err != nil && !isADKSessionNotFound(err) {
			return TranscriptEntry{}, false, err
		}
		if err == nil && response != nil && response.Session != nil {
			for _, event := range eventSlice(response.Session.Events()) {
				if event == nil || event.Partial || event.ID != messageID || isUserEvent(event) {
					continue
				}
				message, visible := transcriptEntryFromADKEvent(event)
				if visible {
					message.SessionID = session.ID
					return message, true, nil
				}
			}
		}
	}
	projection, ok, err := r.store.SessionProjection(ctx, session.ID)
	if err != nil || !ok {
		return TranscriptEntry{}, false, err
	}
	message, ok := projection.MessagesByEventID[messageID]
	if ok && strings.EqualFold(message.Role, "assistant") {
		return message, true, nil
	}
	for _, projected := range projection.Messages {
		if projected.ID == messageID && strings.EqualFold(projected.Role, "assistant") {
			return projected, true, nil
		}
	}
	return TranscriptEntry{}, false, nil
}

func (r *Runtime) appendAssistantMessageEvent(
	ctx context.Context,
	session Session,
	run Run,
	replyResult assistantExecutionResult,
) (TranscriptEntry, error) {
	if r == nil || r.rawSessionService == nil {
		return TranscriptEntry{}, fmt.Errorf("adk session service is unavailable")
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
			return TranscriptEntry{}, createErr
		}
		response = &adksession.GetResponse{Session: created.Session}
	}
	eventID := syntheticAssistantMessageID(run.ID, replyResult)
	if existing, ok, lookupErr := r.assistantMessageByID(ctx, session, eventID); lookupErr != nil {
		return TranscriptEntry{}, lookupErr
	} else if ok {
		return existing, nil
	}
	event := adksession.NewEvent(ctx, run.ID)
	event.ID = eventID
	event.Author = googleADKAgentName(defaultString(run.AgentID, session.AgentID))
	event.LLMResponse = adkmodel.LLMResponse{
		Content:      genai.NewContentFromParts(partsFromReplyAndReasoning(replyResult.Reply, replyResult.ReasoningContent), genai.RoleModel),
		TurnComplete: true,
	}
	if err := appendADKEventWithStaleRetry(ctx, runtimeAppendLocks(r), r.rawSessionService, response.Session, event); err != nil {
		if existing, ok, lookupErr := r.assistantMessageByID(ctx, session, eventID); lookupErr == nil && ok {
			return existing, nil
		}
		return TranscriptEntry{}, err
	}
	message, _ := transcriptEntryFromADKEvent(event)
	message.SessionID = session.ID
	message.RunID = run.ID
	return message, nil
}

func syntheticAssistantMessageID(runID string, result assistantExecutionResult) string {
	kind := strings.TrimSpace(result.SyntheticKind)
	if kind == "" {
		kind = "local"
	}
	digest := sha256.Sum256([]byte(kind + "\x00" + strings.TrimSpace(result.ReasoningContent) + "\x00" + strings.TrimSpace(result.Reply)))
	return fmt.Sprintf("jftrade-%s-%s-%x", strings.TrimSpace(runID), kind, digest[:8])
}

func (r *Runtime) projectedChatResponse(
	ctx context.Context,
	session Session,
	run Run,
	replyResult assistantExecutionResult,
) ChatResponse {
	run = r.authoritativeRunSnapshot(ctx, run)
	response := ChatResponse{
		Reply:            replyResult.Reply,
		ReasoningContent: replyResult.ReasoningContent,
		Session:          session,
		Run:              run,
		PendingApprovals: pendingApprovalsOnly(run.PendingApprovals),
		InputRequest:     normalizeInputRequest(run.InputRequest),
		Timeline:         []TimelineEntry{},
		Context:          r.contextSnapshotForRunOrNil(ctx, session, run),
	}
	if r == nil || r.store == nil {
		return response
	}
	projection, ok, err := r.store.SessionProjection(ctx, session.ID)
	if err != nil || !ok {
		return response
	}
	if message := projectedAssistantMessageForRun(projection, response.Run); message != nil {
		response.Reply = message.Content
		response.ReasoningContent = message.ReasoningContent
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

func projectedAssistantMessageForRun(projection SessionProjection, run Run) *TranscriptEntry {
	finalMessageID := strings.TrimSpace(run.FinalMessageID)
	if finalMessageID != "" {
		if message, ok := projection.MessagesByEventID[finalMessageID]; ok {
			return new(message)
		}
	}
	for index := range projection.Messages {
		message := &projection.Messages[index]
		if finalMessageID != "" && message.ID == finalMessageID {
			return message
		}
		if finalMessageID == "" && strings.TrimSpace(message.RunID) == strings.TrimSpace(run.ID) {
			return message
		}
	}
	return nil
}

func normalizedTimelineEntries(entries []TimelineEntry) []TimelineEntry {
	return normalizeTimelineEntries(entries)
}

func applySessionProjectionToRun(run Run, projection SessionProjection) Run {
	run.PendingApprovals = pendingApprovalsOnly(run.PendingApprovals)
	if strings.TrimSpace(run.FinalMessageID) == "" && strings.TrimSpace(projection.FinalMessageID) != "" {
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
	if shouldPreferProjectedToolCalls(run, projection.ToolCalls) {
		run.ToolCalls = append([]ToolCall(nil), projection.ToolCalls...)
	}
	if len(run.ToolCalls) > 0 {
		run.ToolSummaries = toolSummariesForRun(run)
		run.OptimizationTaskID = optimizationTaskID(run.ToolCalls)
		if run.Usage != nil {
			run.Usage.ToolCallsTotal = len(run.ToolCalls)
		}
	}
	if run.Status == RunStatusPaused && run.PausedReason == "user" {
		run, _ = pruneInterruptedGoalWorkflowToolCalls(run)
	}
	return NormalizeRun(run)
}

func shouldPreferProjectedToolCalls(run Run, projected []ToolCall) bool {
	current := run.ToolCalls
	if len(projected) == 0 {
		return false
	}
	if len(current) == 0 {
		if strings.TrimSpace(run.ParentRunID) == "" && normalizeWorkMode(run.WorkMode) != WorkModeChat {
			return false
		}
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

func (r *Runtime) contextSnapshotForRunOrNil(ctx context.Context, session Session, run Run) *SessionContextSnapshot {
	if r == nil || r.contextManager == nil || strings.TrimSpace(session.ID) == "" {
		return nil
	}
	agent, err := r.resolveSessionContextAgent(ctx, session)
	if err != nil {
		return r.contextSnapshotOrNil(ctx, session.ID)
	}
	if providerID := strings.TrimSpace(run.ProviderID); providerID != "" {
		agent.ProviderID = providerID
	}
	if model := strings.TrimSpace(run.Model); model != "" {
		agent.Model = model
	}
	agent, err = r.prepareAgent(ctx, agent)
	if err != nil {
		return r.contextSnapshotOrNil(ctx, session.ID)
	}
	snapshot, err := r.contextManager.Snapshot(ctx, session, agent)
	if err != nil {
		return r.contextSnapshotOrNil(ctx, session.ID)
	}
	return &snapshot
}
