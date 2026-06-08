package adk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	adkmodel "google.golang.org/adk/model"
	adkrunner "google.golang.org/adk/runner"
	adksession "google.golang.org/adk/session"
	adktool "google.golang.org/adk/tool"
	skilltoolset "google.golang.org/adk/tool/skilltoolset"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
	"gorm.io/gorm"
)

const googleADKUserID = "jftrade-user"

type googleADKExecution struct {
	mu                sync.Mutex
	runner            *adkrunner.Runner
	sessionService    adksession.Service
	sessionID         string
	appName           string
	agent             Agent
	runID             string
	calls             []ToolCall
	summaries         []string
	reply             strings.Builder
	reasoning         strings.Builder
	preToolContent    strings.Builder
	preToolReasoning  strings.Builder
	bufferedReply     strings.Builder
	bufferedReasoning strings.Builder
	onDelta           func(ChatDelta) error
	sawPartialText    bool
	runBlocking       func(context.Context, *genai.Content) error
}

type googleADKTool struct {
	descriptor      ToolDescriptor
	registered      RegisteredTool
	execution       *googleADKExecution
	requireApproval bool
}

func (r *Runtime) executeGoogleADK(
	ctx context.Context,
	agent Agent,
	session Session,
	runID string,
	text string,
	onDelta func(ChatDelta) error,
) (toolExecutionContext, []Approval, openAIChatResult, string, string, error) {
	execution, err := r.newGoogleADKExecution(ctx, agent, session, runID, onDelta)
	if err != nil {
		return toolExecutionContext{}, nil, openAIChatResult{}, "", "", err
	}
	if err := execution.run(ctx, genai.NewContentFromText(text, genai.RoleUser)); err != nil {
		preToolContent, preToolReasoning := execution.preToolState()
		return execution.toolContext(), nil, execution.result(), preToolContent, preToolReasoning, err
	}
	approvals, err := execution.pendingApprovals(ctx, r.store)
	if err != nil {
		preToolContent, preToolReasoning := execution.preToolState()
		return execution.toolContext(), nil, execution.result(), preToolContent, preToolReasoning, err
	}
	if len(approvals) > 0 {
		r.adkMu.Lock()
		r.adkRuns[runID] = execution
		r.adkMu.Unlock()
	}
	preToolContent, preToolReasoning := execution.preToolState()
	return execution.toolContext(), approvals, execution.result(), preToolContent, preToolReasoning, nil
}

func (r *Runtime) resumeGoogleADK(ctx context.Context, run Run) (Run, *Message, bool, error) {
	r.adkMu.Lock()
	execution := r.adkRuns[run.ID]
	r.adkMu.Unlock()
	if execution == nil {
		var err error
		execution, err = r.rehydrateGoogleADKExecution(ctx, run)
		if err != nil {
			return run, nil, true, err
		}
		if execution == nil {
			return run, nil, false, nil
		}
	}
	parts := approvalResolutionParts(run.PendingApprovals)
	if len(parts) == 0 {
		return run, nil, false, nil
	}
	execution.reply.Reset()
	execution.reasoning.Reset()
	if err := execution.run(ctx, genai.NewContentFromParts(parts, genai.RoleUser)); err != nil {
		if fallbackRun, message, ok, fallbackErr := r.persistResumedRunFallback(ctx, run, execution, err); ok {
			return fallbackRun, message, true, fallbackErr
		}
		return run, nil, true, err
	}
	run, denied := hydrateResumedRun(run, execution)
	result := execution.result()
	if denied {
		result.Reply = approvalResolutionSummary(run, run.PendingApprovals[0], false)
		result.ReasoningContent = ""
	} else if result.Reply == "" {
		result.Reply = approvalResolutionSummary(run, run.PendingApprovals[0], !denied)
	}
	run.CompletedAt = ptrString(nowString())
	message, err := r.persistResumedRunResult(ctx, run, result)
	if err != nil {
		return run, nil, true, err
	}
	r.auditResumedRun(ctx, run)
	r.adkMu.Lock()
	delete(r.adkRuns, run.ID)
	r.adkMu.Unlock()
	return run, message, true, nil
}

func (r *Runtime) persistResumedRunFallback(
	ctx context.Context,
	run Run,
	execution *googleADKExecution,
	cause error,
) (Run, *Message, bool, error) {
	fallbackRun, _ := hydrateResumedRun(run, execution)
	if !resumedApprovalsReachedTerminalTools(fallbackRun) {
		return run, nil, false, nil
	}
	fallbackRun.Degraded = true
	if fallbackRun.Status == RunStatusCompleted {
		fallbackRun.ResumeState = "adk_confirmation_resolved_with_local_reply"
		fallbackRun.Message = "completed with local fallback reply"
	}
	fallbackRun.CompletedAt = ptrString(nowString())
	summary := localReply(fallbackRun.UserMessage, fallbackRun.ToolSummaries, cause)
	if fallbackRun.Status == RunStatusFailed {
		summary = approvalResolutionSummary(fallbackRun, firstResolvedApproval(fallbackRun.PendingApprovals), true)
	}
	message, err := r.persistResumedRunResult(ctx, fallbackRun, openAIChatResult{Reply: summary})
	if err != nil {
		return fallbackRun, nil, true, err
	}
	r.auditResumedRun(ctx, fallbackRun)
	r.adkMu.Lock()
	delete(r.adkRuns, fallbackRun.ID)
	r.adkMu.Unlock()
	return fallbackRun, message, true, nil
}

func resumedApprovalsReachedTerminalTools(run Run) bool {
	for _, approval := range run.PendingApprovals {
		if approval.Status != ApprovalStatusApproved {
			continue
		}
		if !approvalHasTerminalToolCall(approval, run.ToolCalls) {
			return false
		}
	}
	return true
}

func approvalHasTerminalToolCall(approval Approval, calls []ToolCall) bool {
	for _, call := range calls {
		if approval.FunctionCallID != "" && call.IdempotencyKey != approval.FunctionCallID {
			continue
		}
		if approval.FunctionCallID == "" && call.ToolName != approval.ToolName {
			continue
		}
		switch call.Status {
		case "SUCCEEDED", "FAILED", "DENIED":
			return true
		}
	}
	return false
}

func firstResolvedApproval(approvals []Approval) Approval {
	for _, approval := range approvals {
		if approval.Status != ApprovalStatusPending {
			return approval
		}
	}
	return Approval{}
}

func (r *Runtime) rehydrateGoogleADKExecution(ctx context.Context, run Run) (*googleADKExecution, error) {
	agentDefinition, err := r.resolveAgent(ctx, run.AgentID)
	if err != nil {
		return nil, err
	}
	agentDefinition, err = r.prepareAgent(ctx, agentDefinition)
	if err != nil {
		return nil, err
	}
	productSession, ok, err := r.store.Session(ctx, run.SessionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("session not found")
	}
	execution, err := r.newGoogleADKExecution(ctx, agentDefinition, productSession, run.ID, nil)
	if err != nil {
		return nil, err
	}
	execution.calls = append([]ToolCall(nil), run.ToolCalls...)
	execution.summaries = append([]string(nil), run.ToolSummaries...)
	parts := make([]*genai.Part, 0, len(run.PendingApprovals))
	for _, approval := range run.PendingApprovals {
		if approval.ConfirmationCallID == "" || approval.FunctionCallID == "" {
			continue
		}
		original := &genai.FunctionCall{
			ID: approval.FunctionCallID, Name: approval.ToolName, Args: approval.Input,
		}
		parts = append(parts, &genai.Part{FunctionCall: &genai.FunctionCall{
			ID: approval.ConfirmationCallID, Name: toolconfirmation.FunctionCallName,
			Args: map[string]any{
				"originalFunctionCall": original,
				"toolConfirmation": toolconfirmation.ToolConfirmation{
					Hint: "请批准或拒绝 JFTrade 工具调用 " + approval.ToolName,
				},
			},
		}})
	}
	if len(parts) == 0 {
		return nil, nil
	}
	return execution, nil
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
	toolContext := execution.toolContext()
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
	for _, call := range run.ToolCalls {
		if call.Status != "FAILED" {
			continue
		}
		run.Status = RunStatusFailed
		run.ErrorCode = "TOOL_EXECUTION_FAILED"
		run.Message = "confirmed tool execution failed"
		if call.Error != nil {
			run.FailureReason = *call.Error
		}
		break
	}
	return run
}

func (r *Runtime) persistResumedRunResult(ctx context.Context, run Run, result openAIChatResult) (*Message, error) {
	message, err := r.store.AddMessage(ctx, run.SessionID, "assistant", result.Reply, result.ReasoningContent)
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

func ptrString(value string) *string {
	return &value
}

func (r *Runtime) newGoogleADKExecution(
	ctx context.Context,
	definition Agent,
	productSession Session,
	runID string,
	onDelta func(ChatDelta) error,
) (*googleADKExecution, error) {
	var llm adkmodel.LLM = localADKModel{agent: definition, tools: r.tools}
	if strings.TrimSpace(definition.ProviderID) != "" {
		provider, ok, err := r.store.Provider(ctx, definition.ProviderID)
		if err != nil {
			return nil, err
		}
		if !ok || !provider.Enabled {
			return nil, fmt.Errorf("agent provider is unavailable")
		}
		apiKey, _, err := r.store.ProviderAPIKey(provider.ID)
		if err != nil {
			return nil, err
		}
		llm = newOpenAICompatibleADKModel(provider, apiKey, definition.Model)
	}

	execution := &googleADKExecution{
		sessionID: productSession.ID,
		appName:   googleADKAppName(definition.ID),
		agent:     definition,
		runID:     runID,
		calls:     []ToolCall{},
		summaries: []string{},
		onDelta:   onDelta,
	}
	tools, err := r.googleADKTools(execution)
	if err != nil {
		return nil, err
	}
	toolsets, err := r.googleADKToolsets(ctx, definition)
	if err != nil {
		return nil, err
	}
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:                googleADKAgentName(definition.ID),
		Description:         definition.Name,
		InstructionProvider: func(adkagent.ReadonlyContext) (string, error) { return definition.Instruction, nil },
		Model:               llm,
		Tools:               tools,
		Toolsets:            toolsets,
		IncludeContents:     llmagent.IncludeContentsDefault,
	})
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK agent: %w", err)
	}
	service := r.sessionService
	if service == nil {
		service = adksession.InMemoryService()
	}
	if _, err := service.Get(ctx, &adksession.GetRequest{
		AppName: execution.appName, UserID: googleADKUserID, SessionID: productSession.ID,
	}); err != nil {
		lowerErr := strings.ToLower(err.Error())
		if !errors.Is(err, gorm.ErrRecordNotFound) && !strings.Contains(lowerErr, "record not found") && !strings.Contains(lowerErr, "not found") {
			return nil, fmt.Errorf("get GO-ADK session: %w", err)
		}
		if _, createErr := service.Create(ctx, &adksession.CreateRequest{
			AppName: execution.appName, UserID: googleADKUserID, SessionID: productSession.ID,
		}); createErr != nil {
			return nil, fmt.Errorf("create GO-ADK session: %w", createErr)
		}
	}
	adkRunner, err := adkrunner.New(adkrunner.Config{
		AppName: execution.appName, Agent: adkAgent, SessionService: service,
	})
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK runner: %w", err)
	}
	execution.runner = adkRunner
	execution.sessionService = service
	return execution, nil
}

func (r *Runtime) googleADKToolsets(ctx context.Context, definition Agent) ([]adktool.Toolset, error) {
	source, err := r.skills.Source(ctx, definition.Skills)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return nil, nil
	}
	toolset, err := skilltoolset.New(ctx, skilltoolset.Config{Source: source})
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK skill toolset: %w", err)
	}
	return []adktool.Toolset{toolset}, nil
}

func (r *Runtime) googleADKTools(execution *googleADKExecution) ([]adktool.Tool, error) {
	descriptors := ToolDescriptorsForAgent(execution.agent, r.tools)
	result := make([]adktool.Tool, 0, len(descriptors))
	for _, descriptor := range descriptors {
		registered, ok := r.tools.Get(descriptor.Name)
		if !ok {
			continue
		}
		result = append(result, &googleADKTool{
			descriptor: descriptor, registered: registered, execution: execution,
			requireApproval: ToolRequiresApproval(descriptor, execution.agent.PermissionMode),
		})
	}
	return result, nil
}

func (t *googleADKTool) Name() string        { return t.descriptor.Name }
func (t *googleADKTool) Description() string { return t.descriptor.Description }
func (t *googleADKTool) IsLongRunning() bool { return false }

func (t *googleADKTool) Declaration() *genai.FunctionDeclaration {
	schemaRaw := any(t.descriptor.InputSchema)
	if schemaRaw == nil {
		schemaRaw = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	schema, ok := schemaRaw.(map[string]any)
	if !ok {
		raw, _ := json.Marshal(schemaRaw)
		_ = json.Unmarshal(raw, &schema)
	}
	schema = sanitizeSchemaForOpenAI(schema)
	return &genai.FunctionDeclaration{
		Name: t.Name(), Description: t.Description(), ParametersJsonSchema: schema,
	}
}

func (t *googleADKTool) ProcessRequest(_ adktool.Context, req *adkmodel.LLMRequest) error {
	if req.Tools == nil {
		req.Tools = make(map[string]any)
	}
	if _, exists := req.Tools[t.Name()]; exists {
		return fmt.Errorf("duplicate tool: %q", t.Name())
	}
	req.Tools[t.Name()] = t
	if req.Config == nil {
		req.Config = &genai.GenerateContentConfig{}
	}
	var functionTools *genai.Tool
	for _, item := range req.Config.Tools {
		if item != nil && item.FunctionDeclarations != nil {
			functionTools = item
			break
		}
	}
	if functionTools == nil {
		req.Config.Tools = append(req.Config.Tools, &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{t.Declaration()},
		})
	} else {
		functionTools.FunctionDeclarations = append(functionTools.FunctionDeclarations, t.Declaration())
	}
	return nil
}

func (t *googleADKTool) Run(ctx adktool.Context, args any) (map[string]any, error) {
	input, ok := args.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("tool %s received invalid input %T", t.Name(), args)
	}
	// Enforce maximum tool calls per run
	if t.execution.toolCallCount() >= MaxToolCallsPerRun {
		return nil, fmt.Errorf("maximum tool calls per run (%d) exceeded", MaxToolCallsPerRun)
	}
	if !ToolAllowedInMode(t.descriptor, t.execution.agent.PermissionMode) {
		return nil, fmt.Errorf("tool is not allowed in permission mode %s", t.execution.agent.PermissionMode)
	}
	call := t.execution.ensureCall(ctx.FunctionCallID(), t.descriptor, input)
	// Emit a progress delta so the client knows a tool is being executed.
	t.execution.emitToolProgress(call.ID, t.Name())
	if confirmation := ctx.ToolConfirmation(); confirmation != nil {
		if !confirmation.Confirmed {
			err := fmt.Errorf("error tool %q %w", t.Name(), adktool.ErrConfirmationRejected)
			t.execution.finishCall(call.ID, nil, err)
			return nil, err
		}
	} else if t.requireApproval {
		if err := ctx.RequestConfirmation("请批准或拒绝 JFTrade 工具调用 "+t.Name(), nil); err != nil {
			return nil, err
		}
		ctx.Actions().SkipSummarization = true
		return nil, fmt.Errorf("error tool %q %w", t.Name(), adktool.ErrConfirmationRequired)
	}
	output, execErr := executeRegisteredTool(ctx, t.registered, input)
	t.execution.finishCall(call.ID, output, execErr)
	if execErr != nil {
		return nil, execErr
	}
	if mapped, ok := output.(map[string]any); ok {
		return mapped, nil
	}
	return map[string]any{"result": output}, nil
}

func (e *googleADKExecution) run(ctx context.Context, content *genai.Content) error {
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
	maxIterations := MaxToolCallsPerRun * 3 // Allow model calls + tool calls per turn
	iterations := 0
	for event, err := range e.runner.Run(ctx, googleADKUserID, e.sessionID, content, adkagent.RunConfig{
		StreamingMode: adkagent.StreamingModeSSE,
	}) {
		if err != nil {
			return err
		}
		if err := e.consumeEvent(event); err != nil {
			return err
		}
		if countsTowardIterationLimit(event) {
			iterations++
		}
		if iterations >= maxIterations {
			return fmt.Errorf("ADK run exceeded maximum iterations (%d), possible infinite tool call loop", maxIterations)
		}
	}
	return nil
}

func countsTowardIterationLimit(event *adksession.Event) bool {
	if event == nil {
		return false
	}
	// Partial streaming chunks are token-level transport details, not agent
	// reasoning steps. Counting them makes ordinary streamed answers trip the
	// loop guard long before the model has a chance to finish.
	return !event.Partial
}

func (e *googleADKExecution) consumeEvent(event *adksession.Event) error {
	if event == nil || event.Content == nil {
		if event != nil && !event.Partial {
			e.sawPartialText = false
		}
		return nil
	}
	emitText := true
	if event.Partial {
		e.sawPartialText = e.sawPartialText || contentHasText(event.Content)
	} else if e.sawPartialText {
		emitText = false
	}
	for _, part := range event.Content.Parts {
		if part.FunctionCall != nil {
			if part.FunctionCall.Name == toolconfirmation.FunctionCallName {
				continue
			}
			descriptor := ToolDescriptor{Name: part.FunctionCall.Name}
			e.ensureCall(part.FunctionCall.ID, descriptor, part.FunctionCall.Args)
		}
		if part.FunctionResponse != nil {
			e.consumeFunctionResponse(part.FunctionResponse)
		}
		if emitText && part.Text != "" {
			reply, reasoning := splitAssistantContent(part.Text)
			if err := e.appendVisibleText(reply, reasoning); err != nil {
				return err
			}
		}
	}
	if !event.Partial {
		e.sawPartialText = false
	}
	if err := e.flushBufferedTextIfReady(); err != nil {
		return err
	}
	return nil
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
	e.mu.Lock()
	defer e.mu.Unlock()
	for index := range e.calls {
		if e.calls[index].IdempotencyKey == functionCallID {
			if e.calls[index].ToolName == "" {
				e.calls[index].ToolName = descriptor.Name
			}
			if e.calls[index].Permission == "" {
				e.calls[index].Permission = descriptor.Permission
			}
			return &e.calls[index]
		}
	}
	now := nowString()
	call := ToolCall{
		ID: "tool-" + uuid.NewString(), RunID: e.runID, ToolName: descriptor.Name,
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
	e.emitRunSnapshotLocked()
	return &e.calls[len(e.calls)-1]
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
			errText := err.Error()
			call.Status = "FAILED"
			call.Error = &errText
		} else {
			call.Status = "SUCCEEDED"
			call.Output = limitToolOutput(output)
			e.summaries = append(e.summaries, summarizeToolOutput(call.ToolName, output))
		}
		finishToolCall(call)
		changed = true
		break
	}
	e.emitRunSnapshotLocked()
	e.mu.Unlock()
	if changed {
		_ = e.flushBufferedTextIfReady()
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
		if call.Status != "RUNNING" && call.Status != "PENDING" {
			break
		}
		if errorValue, ok := response.Response["error"]; ok {
			errText := fmt.Sprint(errorValue)
			if strings.Contains(errText, adktool.ErrConfirmationRequired.Error()) {
				call.Status = "PENDING_APPROVAL"
				call.RequiresUser = true
				call.UpdatedAt = nowString()
				changed = true
				break
			}
			call.Status = "FAILED"
			call.Error = &errText
			finishToolCall(call)
			changed = true
		} else {
			call.Status = "SUCCEEDED"
			call.Output = limitToolOutput(response.Response)
			e.summaries = append(e.summaries, summarizeToolOutput(call.ToolName, response.Response))
			finishToolCall(call)
			changed = true
		}
		break
	}
	e.emitRunSnapshotLocked()
	e.mu.Unlock()
	if changed {
		_ = e.flushBufferedTextIfReady()
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
			now := nowString()
			approval := Approval{
				ID: "approval-" + uuid.NewString(), RunID: e.runID, AgentID: e.agent.ID,
				ToolName: original.Name, Input: original.Args, Status: ApprovalStatusPending,
				Reason:         "GO-ADK HITL 要求用户审批该工具调用。",
				FunctionCallID: original.ID, ConfirmationCallID: call.ID,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := store.SaveApproval(ctx, approval); err != nil {
				return nil, err
			}
			e.markCallPending(original.ID)
			approvals = append(approvals, approval)
		}
	}
	return approvals, nil
}

func (e *googleADKExecution) hasApprovalForConfirmation(id string) bool {
	return id == ""
}

func (e *googleADKExecution) markCallPending(functionCallID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for index := range e.calls {
		if e.calls[index].IdempotencyKey == functionCallID {
			e.calls[index].Status = "PENDING_APPROVAL"
			e.calls[index].RequiresUser = true
			e.calls[index].UpdatedAt = nowString()
		}
	}
	e.emitRunSnapshotLocked()
}

func (e *googleADKExecution) toolContext() toolExecutionContext {
	e.mu.Lock()
	defer e.mu.Unlock()
	return toolExecutionContext{
		calls: append([]ToolCall(nil), e.calls...), summaries: append([]string(nil), e.summaries...),
	}
}

func (e *googleADKExecution) result() openAIChatResult {
	return openAIChatResult{
		Reply: strings.TrimSpace(e.reply.String()), ReasoningContent: strings.TrimSpace(e.reasoning.String()),
	}
}

func (e *googleADKExecution) preToolState() (string, string) {
	return strings.TrimSpace(e.preToolContent.String()), strings.TrimSpace(e.preToolReasoning.String())
}

func (e *googleADKExecution) toolCallCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.calls)
}

func (e *googleADKExecution) emitToolProgress(callID string, toolName string) {
	if e.onDelta == nil {
		return
	}
	_ = e.onDelta(ChatDelta{ToolProgress: fmt.Sprintf("🔧 执行工具 %s...", toolName)})
}

func (e *googleADKExecution) appendVisibleText(reply string, reasoning string) error {
	if reply == "" && reasoning == "" {
		return nil
	}
	if e.activeToolCallCount() > 0 {
		e.bufferedReply.WriteString(reply)
		e.bufferedReasoning.WriteString(reasoning)
		return nil
	}
	e.reply.WriteString(reply)
	e.reasoning.WriteString(reasoning)
	if e.onDelta != nil {
		if err := e.onDelta(ChatDelta{Reply: reply, ReasoningContent: reasoning}); err != nil {
			return err
		}
	}
	return nil
}

func (e *googleADKExecution) flushBufferedTextIfReady() error {
	if e.activeToolCallCount() > 0 {
		return nil
	}
	reply := strings.TrimSpace(e.bufferedReply.String())
	reasoning := strings.TrimSpace(e.bufferedReasoning.String())
	if reply == "" && reasoning == "" {
		return nil
	}
	e.bufferedReply.Reset()
	e.bufferedReasoning.Reset()
	e.reply.WriteString(reply)
	e.reasoning.WriteString(reasoning)
	if e.onDelta != nil {
		if err := e.onDelta(ChatDelta{Reply: reply, ReasoningContent: reasoning}); err != nil {
			return err
		}
	}
	return nil
}

func (e *googleADKExecution) activeToolCallCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	count := 0
	for _, call := range e.calls {
		switch call.Status {
		case "RUNNING", "PENDING":
			count++
		}
	}
	return count
}

func (e *googleADKExecution) emitRunSnapshotLocked() {
	if e.onDelta == nil {
		return
	}
	run := Run{
		ID:               e.runID,
		SessionID:        e.sessionID,
		AgentID:          e.agent.ID,
		Status:           e.derivedRunStatusLocked(),
		Message:          "",
		ToolCalls:        append([]ToolCall(nil), e.calls...),
		PendingApprovals: []Approval{},
		CreatedAt:        "",
		UpdatedAt:        nowString(),
	}
	_ = e.onDelta(ChatDelta{Run: &run})
}

func (e *googleADKExecution) derivedRunStatusLocked() string {
	if len(e.calls) == 0 {
		return RunStatusRunning
	}
	allCancelled := true
	allCompleted := true
	hasFailed := false
	for _, call := range e.calls {
		switch call.Status {
		case "PENDING_APPROVAL":
			return RunStatusPending
		case "RUNNING", "PENDING":
			return RunStatusRunning
		case "FAILED", "TIMED_OUT", "DENIED":
			hasFailed = true
			allCompleted = false
			allCancelled = false
		case "SUCCEEDED", "COMPLETED":
			allCancelled = false
		case "CANCELLED":
			allCompleted = false
		default:
			allCompleted = false
			allCancelled = false
		}
	}
	if hasFailed {
		return RunStatusFailed
	}
	if allCancelled {
		return RunStatusCancelled
	}
	if allCompleted {
		return RunStatusCompleted
	}
	return RunStatusRunning
}

func googleADKAgentName(id string) string {
	name := strings.ReplaceAll(normalizeID(id), "-", "_")
	if name == "" {
		return "jftrade_agent"
	}
	if name == "user" {
		return "jftrade_user_agent"
	}
	return name
}

func googleADKAppName(id string) string {
	normalized := normalizeID(id)
	if normalized == "" {
		return "jftrade-default"
	}
	return "jftrade-" + normalized
}
