package adk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	adkskill "google.golang.org/adk/tool/skilltoolset/skill"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"
	"gorm.io/gorm"
)

const googleADKUserID = "jftrade-user"

type googleADKExecution struct {
	mu                 sync.Mutex
	runner             *adkrunner.Runner
	sessionService     adksession.Service
	sessionID          string
	appName            string
	agent              Agent
	runID              string
	descriptors        map[string]ToolDescriptor
	calls              []ToolCall
	summaries          []string
	reply              strings.Builder
	reasoning          strings.Builder
	preToolContent     strings.Builder
	preToolReasoning   strings.Builder
	bufferedReply      strings.Builder
	bufferedReasoning  strings.Builder
	onDelta            func(ChatDelta) error
	sawPartialText     bool
	runBlocking        func(context.Context, *genai.Content) error
	persistRunSnapshot func(Run) error
}

type googleADKTool struct {
	descriptor ToolDescriptor
	registered RegisteredTool
	execution  *googleADKExecution
}

type googleADKProductToolset struct {
	name  string
	tools []adktool.Tool
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
		execution.detachDeltaSink()
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
	execution.detachDeltaSink()
	parts := approvalResolutionParts(run.PendingApprovals)
	if len(parts) == 0 {
		return run, nil, false, nil
	}
	execution.reply.Reset()
	execution.reasoning.Reset()
	if err := execution.run(ctx, genai.NewContentFromParts(parts, genai.RoleUser)); err != nil {
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
	if !applyRunFailureFromToolCalls(&run) {
		return run
	}
	return run
}

func classifyToolExecutionError(err error) (string, string) {
	if err == nil {
		return "SUCCEEDED", ""
	}
	return classifyToolErrorText(err.Error())
}

func classifyToolErrorText(text string) (string, string) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "context deadline exceeded"):
		return "TIMED_OUT", prefixedToolError(trimmed, "tool execution timed out")
	case strings.Contains(lower, "context canceled"):
		return "CANCELLED", prefixedToolError(trimmed, "tool execution cancelled")
	default:
		return "FAILED", trimmed
	}
}

func prefixedToolError(text string, prefix string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return prefix
	}
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, prefix) {
		return trimmed
	}
	return prefix + ": " + trimmed
}

func firstToolCallByStatus(calls []ToolCall, statuses ...string) *ToolCall {
	if len(calls) == 0 || len(statuses) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		allowed[strings.ToUpper(strings.TrimSpace(status))] = struct{}{}
	}
	for index := range calls {
		if _, ok := allowed[strings.ToUpper(strings.TrimSpace(calls[index].Status))]; ok {
			return &calls[index]
		}
	}
	return nil
}

func toolCallFailureMessage(call *ToolCall) string {
	if call == nil {
		return ""
	}
	if call.Error != nil && strings.TrimSpace(*call.Error) != "" {
		return strings.TrimSpace(*call.Error)
	}
	switch strings.ToUpper(strings.TrimSpace(call.Status)) {
	case "TIMED_OUT":
		return "tool execution timed out"
	case "CANCELLED":
		return "tool execution cancelled"
	default:
		return "tool execution failed"
	}
}

func applyRunFailureFromToolCalls(run *Run) bool {
	if run == nil {
		return false
	}
	call := firstToolCallByStatus(run.ToolCalls, "TIMED_OUT")
	errorCode := "TOOL_EXECUTION_TIMED_OUT"
	status := RunStatusFailed
	if call == nil {
		call = firstToolCallByStatus(run.ToolCalls, "FAILED")
		errorCode = "TOOL_EXECUTION_FAILED"
	}
	if call == nil {
		allCancelled := len(run.ToolCalls) > 0
		for _, toolCall := range run.ToolCalls {
			if strings.ToUpper(strings.TrimSpace(toolCall.Status)) != "CANCELLED" {
				allCancelled = false
				break
			}
		}
		if allCancelled {
			call = &ToolCall{Status: "CANCELLED"}
			errorCode = runErrorCode(RunStatusCancelled)
			status = RunStatusCancelled
		}
	}
	if call == nil {
		return false
	}
	message := toolCallFailureMessage(call)
	run.Status = status
	run.ErrorCode = errorCode
	run.Message = message
	run.FailureReason = message
	run.Degraded = true
	completedAt := nowString()
	if run.CompletedAt == nil {
		run.CompletedAt = &completedAt
	}
	if run.Status == RunStatusCancelled && run.CancelledAt == nil {
		run.CancelledAt = &completedAt
	}
	finalizeRunUsage(run)
	return true
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
		sessionID:   productSession.ID,
		appName:     googleADKAppName(definition.ID),
		agent:       definition,
		runID:       runID,
		descriptors: toolDescriptorIndex(ToolDescriptorsForAgent(definition, r.tools)),
		calls:       []ToolCall{},
		summaries:   []string{},
		onDelta:     onDelta,
		persistRunSnapshot: func(snapshot Run) error {
			return r.persistRunActivitySnapshot(context.Background(), snapshot)
		},
	}
	toolsets, err := r.googleADKToolsets(ctx, definition)
	if err != nil {
		return nil, err
	}
	adkAgent, err := llmagent.New(llmagent.Config{
		Name:        googleADKAgentName(definition.ID),
		Description: definition.Name,
		InstructionProvider: func(ctx adkagent.ReadonlyContext) (string, error) {
			instruction := strings.TrimSpace(definition.Instruction)
			if r.contextManager == nil || ctx == nil {
				return instruction, nil
			}
			suffix, err := r.contextManager.InstructionSuffix(ctx, ctx.SessionID())
			if err != nil || strings.TrimSpace(suffix) == "" {
				return instruction, nil
			}
			if instruction == "" {
				return suffix, nil
			}
			return instruction + "\n\n" + suffix, nil
		},
		Model: llm,
		BeforeToolCallbacks: []llmagent.BeforeToolCallback{
			execution.beforeToolCallback,
		},
		AfterToolCallbacks: []llmagent.AfterToolCallback{
			execution.afterToolCallback,
		},
		Toolsets:        toolsets,
		IncludeContents: llmagent.IncludeContentsDefault,
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
	if r.rawSessionService != nil {
		execution.sessionService = r.rawSessionService
	}
	return execution, nil
}

func (r *Runtime) googleADKToolsets(ctx context.Context, definition Agent) ([]adktool.Toolset, error) {
	baseToolset := r.googleADKProductToolset(definition)
	toolsets := make([]adktool.Toolset, 0, 2)
	if baseToolset != nil {
		filtered := adktool.FilterToolset(baseToolset, func(_ adkagent.ReadonlyContext, tool adktool.Tool) bool {
			if descriptor, ok := descriptorFromADKTool(tool); ok {
				return ToolAllowedInMode(descriptor, definition.PermissionMode)
			}
			return false
		})
		toolsets = append(toolsets, adktool.WithConfirmation(filtered, false, func(toolName string, _ any) bool {
			registered, ok := r.tools.Get(toolName)
			if !ok {
				return false
			}
			return ToolRequiresApproval(registered.Descriptor, definition.PermissionMode)
		}))
	}
	source, err := r.skills.Source(ctx, definition.Skills)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return toolsets, nil
	}
	source, err = r.filteredSkillSourceForAgent(ctx, source, definition)
	if err != nil {
		return nil, err
	}
	toolset, err := skilltoolset.New(ctx, skilltoolset.Config{Source: source})
	if err != nil {
		return nil, fmt.Errorf("create GO-ADK skill toolset: %w", err)
	}
	toolsets = append(toolsets, toolset)
	return toolsets, nil
}

func (r *Runtime) filteredSkillSourceForAgent(ctx context.Context, source adkskill.Source, definition Agent) (adkskill.Source, error) {
	if source == nil {
		return nil, nil
	}
	frontmatters, err := source.ListFrontmatters(ctx)
	if err != nil {
		return nil, err
	}
	allowedTools := r.allowedToolNamesForAgent(definition)
	allowedSkills := make(map[string]struct{}, len(frontmatters))
	for _, frontmatter := range frontmatters {
		if skillAllowedForAgent(frontmatter, allowedTools, r.tools, definition.PermissionMode) {
			allowedSkills[frontmatter.Name] = struct{}{}
		}
	}
	return &agentFilteredSkillSource{base: source, allowed: allowedSkills}, nil
}

func (r *Runtime) allowedToolNamesForAgent(definition Agent) map[string]struct{} {
	descriptors := ToolDescriptorsForAgent(definition, r.tools)
	allowed := make(map[string]struct{}, len(descriptors))
	for _, descriptor := range descriptors {
		if ToolAllowedInMode(descriptor, definition.PermissionMode) {
			allowed[descriptor.Name] = struct{}{}
		}
	}
	return allowed
}

func skillAllowedForAgent(
	frontmatter *adkskill.Frontmatter,
	allowedTools map[string]struct{},
	registry *ToolRegistry,
	mode string,
) bool {
	if frontmatter == nil {
		return false
	}
	for _, toolName := range frontmatter.AllowedTools {
		if registry == nil {
			return false
		}
		canonical, ok := registry.CanonicalName(toolName)
		if !ok {
			return false
		}
		registered, ok := registry.Get(canonical)
		if !ok {
			return false
		}
		if !ToolAllowedInMode(registered.Descriptor, mode) {
			return false
		}
		if _, ok := allowedTools[canonical]; !ok {
			return false
		}
	}
	return true
}

type agentFilteredSkillSource struct {
	base    adkskill.Source
	allowed map[string]struct{}
}

func (s *agentFilteredSkillSource) ListFrontmatters(ctx context.Context) ([]*adkskill.Frontmatter, error) {
	items, err := s.base.ListFrontmatters(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]*adkskill.Frontmatter, 0, len(items))
	for _, item := range items {
		if s.isAllowed(item.Name) {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *agentFilteredSkillSource) ListResources(ctx context.Context, name, subpath string) ([]string, error) {
	if !s.isAllowed(name) {
		return nil, adkskill.ErrSkillNotFound
	}
	return s.base.ListResources(ctx, name, subpath)
}

func (s *agentFilteredSkillSource) LoadFrontmatter(ctx context.Context, name string) (*adkskill.Frontmatter, error) {
	if !s.isAllowed(name) {
		return nil, adkskill.ErrSkillNotFound
	}
	return s.base.LoadFrontmatter(ctx, name)
}

func (s *agentFilteredSkillSource) LoadInstructions(ctx context.Context, name string) (string, error) {
	if !s.isAllowed(name) {
		return "", adkskill.ErrSkillNotFound
	}
	return s.base.LoadInstructions(ctx, name)
}

func (s *agentFilteredSkillSource) LoadResource(ctx context.Context, name, resourcePath string) (io.ReadCloser, error) {
	if !s.isAllowed(name) {
		return nil, adkskill.ErrSkillNotFound
	}
	return s.base.LoadResource(ctx, name, resourcePath)
}

func (s *agentFilteredSkillSource) isAllowed(name string) bool {
	if s == nil {
		return false
	}
	_, ok := s.allowed[strings.TrimSpace(name)]
	return ok
}

func (r *Runtime) googleADKProductToolset(definition Agent) adktool.Toolset {
	descriptors := ToolDescriptorsForAgent(definition, r.tools)
	if len(descriptors) == 0 {
		return nil
	}
	tools := make([]adktool.Tool, 0, len(descriptors))
	for _, descriptor := range descriptors {
		registered, ok := r.tools.Get(descriptor.Name)
		if !ok {
			continue
		}
		tools = append(tools, &googleADKTool{
			descriptor: descriptor,
			registered: registered,
		})
	}
	if len(tools) == 0 {
		return nil
	}
	return &googleADKProductToolset{name: "jftrade-tools", tools: tools}
}

func (t *googleADKTool) Name() string        { return t.descriptor.Name }
func (t *googleADKTool) Description() string { return t.descriptor.Description }
func (t *googleADKTool) IsLongRunning() bool { return false }

func (t *googleADKProductToolset) Name() string { return t.name }

func (t *googleADKProductToolset) Tools(_ adkagent.ReadonlyContext) ([]adktool.Tool, error) {
	if t == nil {
		return nil, nil
	}
	tools := make([]adktool.Tool, 0, len(t.tools))
	tools = append(tools, t.tools...)
	return tools, nil
}

func descriptorFromADKTool(tool adktool.Tool) (ToolDescriptor, bool) {
	typed, ok := tool.(*googleADKTool)
	if !ok || typed == nil {
		return ToolDescriptor{}, false
	}
	return typed.descriptor, true
}

func toolDescriptorIndex(descriptors []ToolDescriptor) map[string]ToolDescriptor {
	if len(descriptors) == 0 {
		return nil
	}
	index := make(map[string]ToolDescriptor, len(descriptors))
	for _, descriptor := range descriptors {
		index[descriptor.Name] = descriptor
	}
	return index
}

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
	output, execErr := executeRegisteredTool(ctx, t.registered, input)
	if execErr != nil {
		return nil, execErr
	}
	if mapped, ok := output.(map[string]any); ok {
		return mapped, nil
	}
	return map[string]any{"result": output}, nil
}

func (e *googleADKExecution) beforeToolCallback(ctx adktool.Context, tool adktool.Tool, args map[string]any) (map[string]any, error) {
	descriptor, ok := e.descriptorForTool(tool)
	if !ok {
		return nil, nil
	}
	if e.toolCallCount() >= MaxToolCallsPerRun {
		return nil, fmt.Errorf("maximum tool calls per run (%d) exceeded", MaxToolCallsPerRun)
	}
	call := e.ensureCall(ctx.FunctionCallID(), descriptor, args)
	e.emitToolProgress(call.ID, tool.Name())
	if !ToolAllowedInMode(descriptor, e.agent.PermissionMode) {
		return nil, fmt.Errorf("tool is not allowed in permission mode %s", e.agent.PermissionMode)
	}
	return nil, nil
}

func (e *googleADKExecution) afterToolCallback(
	ctx adktool.Context,
	tool adktool.Tool,
	args map[string]any,
	result map[string]any,
	err error,
) (map[string]any, error) {
	descriptor, ok := e.descriptorForTool(tool)
	if !ok {
		return nil, nil
	}
	call := e.ensureCall(ctx.FunctionCallID(), descriptor, args)
	switch {
	case err == nil:
		e.finishCall(call.ID, result, nil)
	case errors.Is(err, adktool.ErrConfirmationRequired):
		// ADK will emit a tool confirmation function response that transitions the
		// tracked call into PENDING_APPROVAL; keep the call open until then.
	case errors.Is(err, adktool.ErrConfirmationRejected):
		e.finishCall(call.ID, nil, err)
	default:
		e.finishCall(call.ID, result, err)
	}
	return nil, nil
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
			reply, reasoning := visibleTextFromParts([]*genai.Part{part})
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
			var errText string
			call.Status, errText = classifyToolExecutionError(err)
			call.Error = &errText
			call.RequiresUser = false
		} else {
			call.Status = "SUCCEEDED"
			call.Output = limitToolOutput(output)
			call.Error = nil
			call.RequiresUser = false
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
			errText := strings.TrimSpace(fmt.Sprint(errorValue))
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
			call.Output = limitToolOutput(response.Response)
			call.Error = nil
			call.RequiresUser = false
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

func (e *googleADKExecution) detachDeltaSink() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.onDelta = nil
}

func (e *googleADKExecution) emitToolProgress(callID string, toolName string) {
	if e.onDelta == nil {
		return
	}
	_ = e.onDelta(ChatDelta{ToolProgress: projectedToolProgress(toolName)})
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
	run := Run{
		ID:               e.runID,
		SessionID:        e.sessionID,
		AgentID:          e.agent.ID,
		Status:           e.derivedRunStatusLocked(),
		Message:          "",
		ToolCalls:        append([]ToolCall(nil), e.calls...),
		ToolSummaries:    append([]string(nil), e.summaries...),
		PendingApprovals: []Approval{},
		CreatedAt:        "",
		UpdatedAt:        nowString(),
	}
	applyRunFailureFromToolCalls(&run)
	if e.persistRunSnapshot != nil {
		persisted := run
		persisted.Status = e.persistedRunStatusLocked()
		if persisted.Status == RunStatusRunning || persisted.Status == RunStatusPending {
			persisted.CompletedAt = nil
			persisted.CancelledAt = nil
			persisted.Degraded = false
			if persisted.Status != RunStatusFailed {
				persisted.Message = ""
				persisted.FailureReason = ""
				persisted.ErrorCode = ""
			}
		}
		_ = e.persistRunSnapshot(persisted)
	}
	if e.onDelta != nil {
		_ = e.onDelta(ChatDelta{Run: &run})
	}
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

func (e *googleADKExecution) persistedRunStatusLocked() string {
	if len(e.calls) == 0 {
		return RunStatusRunning
	}
	allCancelled := true
	hasFailure := false
	for _, call := range e.calls {
		switch call.Status {
		case "PENDING_APPROVAL":
			return RunStatusPending
		case "RUNNING", "PENDING":
			return RunStatusRunning
		case "FAILED", "TIMED_OUT", "DENIED":
			hasFailure = true
			allCancelled = false
		case "SUCCEEDED", "COMPLETED":
			allCancelled = false
		case "CANCELLED":
		default:
			allCancelled = false
		}
	}
	if hasFailure {
		return RunStatusFailed
	}
	if allCancelled {
		return RunStatusCancelled
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
