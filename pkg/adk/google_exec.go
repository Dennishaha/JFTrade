package adk

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
	adkagent "google.golang.org/adk/v2/agent"
	adksession "google.golang.org/adk/v2/session"
	adktool "google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/toolconfirmation"
	adkworkflow "google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

func (e *googleADKExecution) shouldInterruptForUserGoalPause(runID string) bool {
	runID = strings.TrimSpace(runID)
	if runID == "" || runID != e.runID || e.loadRun == nil {
		return false
	}
	run, ok, err := e.loadRun(context.Background(), runID)
	if err != nil || !ok {
		return false
	}
	return userPauseRequestedGoalParent(run) || userPausedGoalParent(run)
}

func (e *googleADKExecution) descriptorForTool(tool adktool.Tool) (ToolDescriptor, bool) {
	if descriptor, ok := descriptorFromADKTool(tool); ok {
		return descriptor, true
	}
	if tool == nil || len(e.descriptors) == 0 {
		return ToolDescriptor{}, false
	}
	descriptor, ok := e.descriptors[tool.Name()]
	return descriptor, ok
}

func (e *googleADKExecution) run(ctx context.Context, content *genai.Content) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	runBlocking := e.runBlocking
	if runBlocking == nil {
		runBlocking = e.runBlockingWithRunner
	}
	done := make(chan error, 1)
	go func() {
		done <- runBlocking(ctx, content)
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *googleADKExecution) runBlockingWithRunner(ctx context.Context, content *genai.Content) error {
	for event, err := range e.runner.Run(ctx, googleADKUserID, e.sessionID, content, adkagent.RunConfig{
		StreamingMode: adkagent.StreamingModeSSE,
	}) {
		if err != nil {
			return err
		}
		if err := e.consumeEvent(event); err != nil {
			return err
		}
	}
	return nil
}

func (e *googleADKExecution) consumeEvent(event *adksession.Event) error {
	if requestedInputUnsupported(event) {
		return errADKInputUnsupported
	}
	if event == nil || event.Content == nil {
		if event != nil && !event.Partial {
			e.mu.Lock()
			e.sawPartialText = false
			e.mu.Unlock()
		}
		return nil
	}
	emitText := true
	e.mu.Lock()
	if event.Partial {
		e.sawPartialText = e.sawPartialText || contentHasText(event.Content)
	} else if e.sawPartialText {
		emitText = false
	}
	e.mu.Unlock()
	for _, part := range event.Content.Parts {
		if part.FunctionCall != nil {
			if part.FunctionCall.Name == adkworkflow.WorkflowInputFunctionCallName {
				continue
			}
			if part.FunctionCall.Name == toolconfirmation.FunctionCallName {
				continue
			}
			descriptor := ToolDescriptor{Name: part.FunctionCall.Name}
			e.ensureCallForAgent(part.FunctionCall.ID, descriptor, part.FunctionCall.Args, event.Author)
		}
		if part.FunctionResponse != nil {
			e.consumeFunctionResponse(part.FunctionResponse)
		}
		if emitText && part.Text != "" {
			reply, reasoning := visibleTextFromParts([]*genai.Part{part})
			if err := e.appendVisibleTextForRun(e.runIDForAgentName(event.Author), reply, reasoning); err != nil {
				return err
			}
		}
	}
	if !event.Partial {
		e.mu.Lock()
		e.sawPartialText = false
		e.mu.Unlock()
	}
	if err := e.flushBufferedTextIfReady(); err != nil {
		return err
	}
	return nil
}

func requestedInputUnsupported(event *adksession.Event) bool {
	if event == nil {
		return false
	}
	if event.RequestedInput != nil {
		return true
	}
	if event.Content == nil {
		return false
	}
	for _, part := range event.Content.Parts {
		if part != nil && part.FunctionCall != nil && part.FunctionCall.Name == adkworkflow.WorkflowInputFunctionCallName {
			return true
		}
	}
	return false
}

func contentHasText(content *genai.Content) bool {
	if content == nil {
		return false
	}
	for _, part := range content.Parts {
		if part != nil && part.Text != "" {
			return true
		}
	}
	return false
}

func (e *googleADKExecution) ensureCall(functionCallID string, descriptor ToolDescriptor, input map[string]any) *ToolCall {
	return e.ensureCallForRun(functionCallID, descriptor, input, e.runID)
}

func (e *googleADKExecution) ensureCallForAgent(functionCallID string, descriptor ToolDescriptor, input map[string]any, agentName string) *ToolCall {
	return e.ensureCallForRun(functionCallID, descriptor, input, e.runIDForAgentName(agentName))
}

func (e *googleADKExecution) runIDForAgentName(agentName string) string {
	normalized := strings.TrimSpace(agentName)
	if normalized != "" && e.runIDByAgentName != nil {
		if runID := strings.TrimSpace(e.runIDByAgentName[normalized]); runID != "" {
			return runID
		}
	}
	return e.runID
}

func (e *googleADKExecution) agentNameForRunID(runID string) string {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ""
	}
	for agentName, mappedRunID := range e.runIDByAgentName {
		if strings.TrimSpace(mappedRunID) == runID {
			return agentName
		}
	}
	return ""
}

func (e *googleADKExecution) ensureCallForRun(functionCallID string, descriptor ToolDescriptor, input map[string]any, runID string) *ToolCall {
	e.mu.Lock()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	for index := range e.calls {
		if e.calls[index].IdempotencyKey == functionCallID {
			if e.calls[index].ToolName == "" {
				e.calls[index].ToolName = descriptor.Name
			}
			if e.calls[index].Permission == "" {
				e.calls[index].Permission = descriptor.Permission
			}
			call := &e.calls[index]
			e.mu.Unlock()
			return call
		}
	}
	now := nowString()
	call := ToolCall{
		ID: "tool-" + uuid.NewString(), RunID: runID, ToolName: descriptor.Name,
		Permission: descriptor.Permission, Status: "RUNNING", Input: input,
		IdempotencyKey: functionCallID, CreatedAt: now, StartedAt: now, UpdatedAt: now,
	}
	if len(e.calls) == 0 {
		e.preToolContent.Reset()
		e.preToolReasoning.Reset()
		e.preToolContent.WriteString(strings.TrimSpace(e.reply.String()))
		e.preToolReasoning.WriteString(strings.TrimSpace(e.reasoning.String()))
	}
	e.calls = append(e.calls, call)
	deltas := e.collectRunSnapshotDeltasLocked()
	created := &e.calls[len(e.calls)-1]
	e.mu.Unlock()
	e.emitRunSnapshotDeltas(deltas)
	return created
}

func (e *googleADKExecution) finishCall(callID string, output any, err error) {
	e.mu.Lock()
	changed := false
	for index := range e.calls {
		call := &e.calls[index]
		if call.ID != callID {
			continue
		}
		if err != nil {
			var errText string
			call.Status, errText = classifyToolExecutionError(err)
			call.Error = &errText
			call.RequiresUser = false
		} else {
			call.Status = "SUCCEEDED"
			call.Output = e.materializeToolOutput(call.ToolName, call.ID, output)
			call.Error = nil
			call.RequiresUser = false
			e.summaries = append(e.summaries, summarizeToolOutput(call.ToolName, output))
		}
		finishToolCall(call)
		changed = true
		break
	}
	deltas := e.collectRunSnapshotDeltasLocked()
	e.mu.Unlock()
	e.emitRunSnapshotDeltas(deltas)
	if changed {
		jftradeErr1 := e.flushBufferedTextIfReady()
		besteffort.LogError(jftradeErr1)
	}
}

func (e *googleADKExecution) consumeFunctionResponse(response *genai.FunctionResponse) {
	if response == nil {
		return
	}
	e.mu.Lock()
	changed := false
	for index := range e.calls {
		call := &e.calls[index]
		if call.IdempotencyKey != response.ID {
			continue
		}
		e.markToolResponseSeenLocked(call.RunID)
		if call.Status != "RUNNING" && call.Status != "PENDING" {
			break
		}
		if isToolResponseError(response.Response) {
			errText := toolResponseErrorMessage(response.Response)
			if strings.Contains(errText, adktool.ErrConfirmationRequired.Error()) {
				call.Status = "PENDING_APPROVAL"
				call.RequiresUser = true
				call.UpdatedAt = nowString()
				changed = true
				break
			}
			call.Status, errText = classifyToolErrorText(errText)
			call.Error = &errText
			call.RequiresUser = false
			finishToolCall(call)
			changed = true
		} else {
			call.Status = "SUCCEEDED"
			call.Output = e.materializeToolOutput(call.ToolName, call.ID, response.Response)
			call.Error = nil
			call.RequiresUser = false
			e.summaries = append(e.summaries, summarizeToolOutput(call.ToolName, response.Response))
			finishToolCall(call)
			changed = true
		}
		break
	}
	deltas := e.collectRunSnapshotDeltasLocked()
	e.mu.Unlock()
	e.emitRunSnapshotDeltas(deltas)
	if changed {
		jftradeErr2 := e.flushBufferedTextIfReady()
		besteffort.LogError(jftradeErr2)
	}
}

func (e *googleADKExecution) pendingApprovals(ctx context.Context, store *Store) ([]Approval, error) {
	response, err := e.sessionService.Get(ctx, &adksession.GetRequest{
		AppName: e.appName, UserID: googleADKUserID, SessionID: e.sessionID,
	})
	if err != nil {
		return nil, err
	}
	var approvals []Approval
	for event := range response.Session.Events().All() {
		if event.Content == nil {
			continue
		}
		for _, part := range event.Content.Parts {
			call := part.FunctionCall
			if call == nil || call.Name != toolconfirmation.FunctionCallName {
				continue
			}
			original, err := toolconfirmation.OriginalCallFrom(call)
			if err != nil {
				return nil, err
			}
			if e.hasApprovalForConfirmation(call.ID) {
				continue
			}
			runID, tracked := e.trackedRunIDForFunctionCall(original.ID)
			if !tracked {
				continue
			}
			now := nowString()
			approval := Approval{
				ID: "approval-" + uuid.NewString(), RunID: runID, AgentID: e.agent.ID,
				ToolName: original.Name, Input: original.Args, Status: ApprovalStatusPending,
				Reason:         "GO-ADK HITL 要求用户审批该工具调用。",
				FunctionCallID: original.ID, ConfirmationCallID: call.ID,
				CreatedAt: now, UpdatedAt: now,
			}
			saved, created, err := store.SaveApprovalIfConfirmationAbsent(ctx, approval)
			if err != nil {
				return nil, err
			}
			e.markConfirmationProcessed(call.ID)
			if !created {
				_ = saved
				continue
			}
			e.markCallPending(original.ID)
			approvals = append(approvals, saved)
		}
	}
	return approvals, nil
}

func (e *googleADKExecution) hasApprovalForConfirmation(id string) bool {
	if id == "" {
		return true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.processedConfirmationIDs != nil {
		_, ok := e.processedConfirmationIDs[id]
		return ok
	}
	return false
}

func (e *googleADKExecution) markConfirmationProcessed(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.processedConfirmationIDs == nil {
		e.processedConfirmationIDs = make(map[string]struct{})
	}
	e.processedConfirmationIDs[id] = struct{}{}
}

func (e *googleADKExecution) markCallPending(functionCallID string) {
	e.mu.Lock()
	for index := range e.calls {
		if e.calls[index].IdempotencyKey == functionCallID {
			e.calls[index].Status = "PENDING_APPROVAL"
			e.calls[index].RequiresUser = true
			e.calls[index].UpdatedAt = nowString()
		}
	}
	deltas := e.collectRunSnapshotDeltasLocked()
	e.mu.Unlock()
	e.emitRunSnapshotDeltas(deltas)
}

func (e *googleADKExecution) markCallWaitingForInput(functionCallID string) {
	e.mu.Lock()
	for index := range e.calls {
		if e.calls[index].IdempotencyKey == functionCallID {
			e.calls[index].Status = RunStatusPendingInput
			e.calls[index].RequiresUser = true
			e.calls[index].UpdatedAt = nowString()
		}
	}
	deltas := e.collectRunSnapshotDeltasLocked()
	e.mu.Unlock()
	e.emitRunSnapshotDeltas(deltas)
}

func (e *googleADKExecution) toolContext() toolExecutionContext {
	return e.toolContextForRun("")
}

func (e *googleADKExecution) toolContextForRun(runID string) toolExecutionContext {
	e.mu.Lock()
	defer e.mu.Unlock()
	runID = strings.TrimSpace(runID)
	calls := make([]ToolCall, 0, len(e.calls))
	summaries := make([]string, 0, len(e.summaries))
	for _, call := range e.calls {
		if runID != "" && call.RunID != runID {
			continue
		}
		calls = append(calls, call)
		if summary := summarizeToolCall(call); summary != "" {
			summaries = append(summaries, summary)
		}
	}
	if runID == "" {
		summaries = append([]string(nil), e.summaries...)
	}
	var inputRequest *InputRequest
	requestRunID := runID
	if requestRunID == "" {
		requestRunID = e.runID
	}
	if base, ok := e.runSnapshotBaseByID[requestRunID]; ok {
		inputRequest = normalizeInputRequest(base.InputRequest)
	}
	return toolExecutionContext{calls: calls, summaries: summaries, inputRequest: inputRequest}
}

func (e *googleADKExecution) setInputRequests(requests map[string]*InputRequest) {
	if e == nil || len(requests) == 0 {
		return
	}
	e.mu.Lock()
	for runID, request := range requests {
		base := e.runBaseLocked(runID)
		base.InputRequest = normalizeInputRequest(request)
		base.InputRequests = appendInputRequestIfMissing(base.InputRequests, *request)
		e.runSnapshotBaseByID[runID] = base
	}
	deltas := e.collectRunSnapshotDeltasLocked()
	e.mu.Unlock()
	e.emitRunSnapshotDeltas(deltas)
}

func (e *googleADKExecution) result() openAIChatResult {
	return e.resultForRun(e.runID)
}

func (e *googleADKExecution) resultForRun(runID string) openAIChatResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ensureTextMapsLocked()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	if runID == e.runID {
		return openAIChatResult{
			Reply: strings.TrimSpace(e.reply.String()), ReasoningContent: strings.TrimSpace(e.reasoning.String()),
		}
	}
	reply := e.replyByRunID[runID]
	reasoning := e.reasoningByRunID[runID]
	var replyText, reasoningText string
	if reply != nil {
		replyText = reply.String()
	}
	if reasoning != nil {
		reasoningText = reasoning.String()
	}
	return openAIChatResult{
		Reply: strings.TrimSpace(replyText), ReasoningContent: strings.TrimSpace(reasoningText),
	}
}

func (e *googleADKExecution) trackedRunIDForFunctionCall(functionCallID string) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, call := range e.calls {
		if call.IdempotencyKey == functionCallID && strings.TrimSpace(call.RunID) != "" {
			return call.RunID, true
		}
	}
	return "", false
}

func (e *googleADKExecution) preToolState() (string, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return strings.TrimSpace(e.preToolContent.String()), strings.TrimSpace(e.preToolReasoning.String())
}

func (e *googleADKExecution) detachDeltaSink() {
	e.deltaMu.Lock()
	defer e.deltaMu.Unlock()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onDelta = nil
}

func (e *googleADKExecution) emitToolProgress(callID string, toolName string) {
	jftradeErr4 := e.emitDelta(ChatDelta{ToolProgress: projectedToolProgress(toolName)})
	besteffort.LogError(jftradeErr4)
}

func (e *googleADKExecution) appendVisibleTextForRun(runID string, reply string, reasoning string) error {
	if reply == "" && reasoning == "" {
		return nil
	}
	e.mu.Lock()
	delta, emit := e.appendVisibleTextForRunLocked(runID, reply, reasoning)
	e.mu.Unlock()
	if !emit {
		return nil
	}
	return e.emitDelta(delta)
}

func (e *googleADKExecution) appendVisibleTextForRunLocked(runID string, reply string, reasoning string) (ChatDelta, bool) {
	e.ensureTextMapsLocked()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	if e.activeToolCallCountForRunLocked(runID) > 0 {
		e.builderForRun(e.bufferedReplyByRunID, runID).WriteString(reply)
		e.builderForRun(e.bufferedReasoningByRunID, runID).WriteString(reasoning)
		if runID == e.runID {
			e.bufferedReply.WriteString(reply)
			e.bufferedReasoning.WriteString(reasoning)
		}
		return ChatDelta{}, false
	}
	e.builderForRun(e.replyByRunID, runID).WriteString(reply)
	e.builderForRun(e.reasoningByRunID, runID).WriteString(reasoning)
	if runID == e.runID {
		e.reply.WriteString(reply)
		e.reasoning.WriteString(reasoning)
	}
	if e.toolResponseSeenForRunLocked(runID) {
		e.markPostToolTextForRunLocked(runID)
	}
	if runID == e.runID {
		return ChatDelta{Reply: reply, ReasoningContent: reasoning}, true
	}
	return ChatDelta{}, false
}

func (e *googleADKExecution) flushBufferedTextIfReady() error {
	e.mu.Lock()
	e.ensureTextMapsLocked()
	runIDs := make([]string, 0, len(e.bufferedReplyByRunID)+1)
	seen := make(map[string]struct{}, len(e.bufferedReplyByRunID)+1)
	for runID := range e.bufferedReplyByRunID {
		if _, ok := seen[runID]; !ok {
			seen[runID] = struct{}{}
			runIDs = append(runIDs, runID)
		}
	}
	if _, ok := seen[e.runID]; !ok {
		runIDs = append(runIDs, e.runID)
	}
	deltas := make([]ChatDelta, 0, len(runIDs))
	for _, runID := range runIDs {
		if delta, emit := e.flushBufferedTextForRunIfReadyLocked(runID); emit {
			deltas = append(deltas, delta)
		}
	}
	e.mu.Unlock()
	for _, delta := range deltas {
		if err := e.emitDelta(delta); err != nil {
			return err
		}
	}
	return nil
}

func (e *googleADKExecution) flushBufferedTextForRunIfReady(runID string) error {
	e.mu.Lock()
	delta, emit := e.flushBufferedTextForRunIfReadyLocked(runID)
	e.mu.Unlock()
	if !emit {
		return nil
	}
	return e.emitDelta(delta)
}

func (e *googleADKExecution) flushBufferedTextForRunIfReadyLocked(runID string) (ChatDelta, bool) {
	e.ensureTextMapsLocked()
	runID = strings.TrimSpace(runID)
	if runID == "" {
		runID = e.runID
	}
	if e.activeToolCallCountForRunLocked(runID) > 0 {
		return ChatDelta{}, false
	}
	replyBuffer := e.builderForRun(e.bufferedReplyByRunID, runID)
	reasoningBuffer := e.builderForRun(e.bufferedReasoningByRunID, runID)
	reply := strings.TrimSpace(replyBuffer.String())
	reasoning := strings.TrimSpace(reasoningBuffer.String())
	if reply == "" && reasoning == "" {
		return ChatDelta{}, false
	}
	replyBuffer.Reset()
	reasoningBuffer.Reset()
	e.builderForRun(e.replyByRunID, runID).WriteString(reply)
	e.builderForRun(e.reasoningByRunID, runID).WriteString(reasoning)
	if runID == e.runID {
		e.bufferedReply.Reset()
		e.bufferedReasoning.Reset()
		e.reply.WriteString(reply)
		e.reasoning.WriteString(reasoning)
	}
	if e.toolResponseSeenForRunLocked(runID) {
		e.markPostToolTextForRunLocked(runID)
	}
	if runID == e.runID {
		return ChatDelta{Reply: reply, ReasoningContent: reasoning}, true
	}
	return ChatDelta{}, false
}

func (e *googleADKExecution) ensureTextMaps() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ensureTextMapsLocked()
}

func (e *googleADKExecution) ensureTextMapsLocked() {
	if e.replyByRunID == nil {
		e.replyByRunID = map[string]*strings.Builder{}
	}
	if e.reasoningByRunID == nil {
		e.reasoningByRunID = map[string]*strings.Builder{}
	}
	if e.bufferedReplyByRunID == nil {
		e.bufferedReplyByRunID = map[string]*strings.Builder{}
	}
	if e.bufferedReasoningByRunID == nil {
		e.bufferedReasoningByRunID = map[string]*strings.Builder{}
	}
	if e.toolResponseSeenByRunID == nil {
		e.toolResponseSeenByRunID = map[string]bool{}
	}
	if e.postToolTextByRunID == nil {
		e.postToolTextByRunID = map[string]bool{}
	}
	if e.toolResponseSeqByRunID == nil {
		e.toolResponseSeqByRunID = map[string]int{}
	}
	if e.postToolTextSeqByRunID == nil {
		e.postToolTextSeqByRunID = map[string]int{}
	}
}

func (e *googleADKExecution) builderForRun(store map[string]*strings.Builder, runID string) *strings.Builder {
	if store == nil {
		return &strings.Builder{}
	}
	builder := store[runID]
	if builder == nil {
		builder = &strings.Builder{}
		store[runID] = builder
	}
	return builder
}

func (e *googleADKExecution) activeToolCallCountForRun(runID string) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.activeToolCallCountForRunLocked(runID)
}

func (e *googleADKExecution) activeToolCallCountForRunLocked(runID string) int {
	runID = strings.TrimSpace(runID)
	count := 0
	for _, call := range e.calls {
		if runID != "" && call.RunID != runID {
			continue
		}
		switch call.Status {
		case "RUNNING", "PENDING":
			count++
		}
	}
	return count
}

func (e *googleADKExecution) emitDelta(delta ChatDelta) error {
	e.deltaMu.Lock()
	defer e.deltaMu.Unlock()
	e.mu.Lock()
	onDelta := e.onDelta
	e.mu.Unlock()
	if onDelta == nil {
		return nil
	}
	return onDelta(delta)
}

func (e *googleADKExecution) emitRunSnapshotDeltas(deltas []ChatDelta) {
	for _, delta := range deltas {
		besteffort.LogError(e.emitDelta(delta))
	}
}
