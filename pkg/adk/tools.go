package adk

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	adksession "google.golang.org/adk/v2/session"
)

type ToolFunc func(context.Context, map[string]any) (any, error)

type toolContextKey string

const toolContextAgentKey toolContextKey = "adkToolAgent"

const toolContextSessionIDKey toolContextKey = "adkToolSessionID"

const toolContextSkillActivationKey toolContextKey = "adkToolSkillActivation"

type toolInvocationSkillActivation struct {
	agentName string
	state     adksession.ReadonlyState
}

func contextWithToolAgent(ctx context.Context, agent Agent) context.Context {
	return context.WithValue(ctx, toolContextAgentKey, agent)
}

func toolAgentFromContext(ctx context.Context) (Agent, bool) {
	agent, ok := ctx.Value(toolContextAgentKey).(Agent)
	return agent, ok
}

// ToolInvocationSessionID returns the product session associated with an ADK
// tool call. The value is copied out of the GO-ADK context before the generic
// tool timeout wrapper hides its extended context methods.
func ToolInvocationSessionID(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	sessionID, ok := ctx.Value(toolContextSessionIDKey).(string)
	sessionID = strings.TrimSpace(sessionID)
	if ok && sessionID != "" {
		return sessionID, true
	}
	if source, sourceOK := ctx.(interface{ SessionID() string }); sourceOK {
		sessionID = strings.TrimSpace(source.SessionID())
		return sessionID, sessionID != ""
	}
	return "", false
}

func contextWithToolInvocationMetadata(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	out := ctx
	if sessionID, ok := ctx.Value(toolContextSessionIDKey).(string); !ok || strings.TrimSpace(sessionID) == "" {
		if source, sourceOK := ctx.(interface{ SessionID() string }); sourceOK {
			if sessionID = strings.TrimSpace(source.SessionID()); sessionID != "" {
				out = context.WithValue(out, toolContextSessionIDKey, sessionID)
			}
		}
	}
	if _, ok := ctx.Value(toolContextSkillActivationKey).(toolInvocationSkillActivation); !ok {
		if source, sourceOK := ctx.(interface {
			AgentName() string
			ReadonlyState() adksession.ReadonlyState
		}); sourceOK && source.ReadonlyState() != nil {
			activation := toolInvocationSkillActivation{
				agentName: strings.TrimSpace(source.AgentName()),
				state:     source.ReadonlyState(),
			}
			out = context.WithValue(out, toolContextSkillActivationKey, activation)
		}
	}
	return out
}

// ToolInvocationSkillActive reports whether a skill was loaded for the
// current agent in this invocation. The state projection survives the generic
// tool timeout wrapper, which otherwise hides GO-ADK's extended context API.
func ToolInvocationSkillActive(ctx context.Context, skillName string) bool {
	if ctx == nil {
		return false
	}
	if activation, ok := ctx.Value(toolContextSkillActivationKey).(toolInvocationSkillActivation); ok {
		return skillActiveInState(activation.state, activation.agentName, skillName)
	}
	source, ok := ctx.(interface {
		AgentName() string
		ReadonlyState() adksession.ReadonlyState
	})
	return ok && skillActiveInState(source.ReadonlyState(), source.AgentName(), skillName)
}

// ToolInvocationAnySkillActive reports whether any required skill was loaded
// for the current agent in this invocation.
func ToolInvocationAnySkillActive(ctx context.Context, skillNames []string) bool {
	for _, skillName := range normalizeStringSlice(skillNames) {
		if ToolInvocationSkillActive(ctx, skillName) {
			return true
		}
	}
	return false
}

// ToolRequiredSkillNames returns the normalized set of skills that can unlock
// a tool. RequiredSkill remains the backwards-compatible single-skill field;
// RequiredSkills is used when any one of multiple skills can unlock a tool.
func ToolRequiredSkillNames(descriptor ToolDescriptor) []string {
	return normalizeStringSlice(append([]string{descriptor.RequiredSkill}, descriptor.RequiredSkills...))
}

type RegisteredTool struct {
	Descriptor ToolDescriptor
	Handler    ToolFunc
}

type ToolRegistry struct {
	tools map[string]RegisteredTool
}

func NewToolRegistry() *ToolRegistry {
	registry := &ToolRegistry{tools: map[string]RegisteredTool{}}
	registerInputRequestTool(registry)
	registry.Register(ToolDescriptor{
		Name:               "workflow.wait",
		DisplayName:        "等待",
		Description:        "短暂等待指定时间后返回，用于轮询异步任务进度；最大等待 25 秒，不创建持久化调度任务。",
		Category:           "workflow",
		Permission:         "read_internal",
		AllowedModes:       []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll},
		RequiresApprovalIn: nil,
		OutputSummary:      "实际等待时长、开始和完成时间。",
		RiskLevel:          "low",
	}, workflowWaitTool)
	registry.Register(ToolDescriptor{
		Name:               "http.fetch",
		DisplayName:        "读取外部 HTTP",
		Description:        "读取公网 HTTP/HTTPS 文本或 JSON 资源，默认阻止本机、私网和 metadata 地址。",
		Category:           "external",
		Permission:         "read_external",
		AllowedModes:       []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll},
		RequiresApprovalIn: nil,
	}, httpFetchTool)
	registry.Register(ToolDescriptor{
		Name:               "tools.search",
		DisplayName:        "搜索 ADK 工具",
		Description:        "按名称、分类、权限、风险等级或描述搜索当前已注册的 JFTrade ADK 工具。",
		Category:           "system",
		Permission:         "read_internal",
		AllowedModes:       []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll},
		RequiresApprovalIn: nil,
		OutputSummary:      "匹配到的工具 descriptor、风险等级与输入 schema。",
		RiskLevel:          "low",
	}, func(ctx context.Context, input map[string]any) (any, error) {
		query := strings.ToLower(strings.TrimSpace(toolStringValue(input, "query")))
		category := strings.ToLower(strings.TrimSpace(toolStringValue(input, "category")))
		limit := min(max(toolIntValue(input, "limit", 12), 1), 50)
		descriptors := registry.List()
		if agent, ok := toolAgentFromContext(ctx); ok {
			descriptors = ToolDescriptorsForAgent(agent, registry)
		}
		matches := make([]map[string]any, 0)
		for _, descriptor := range descriptors {
			if descriptor.Name == "tools.search" {
				continue
			}
			requiredSkills := ToolRequiredSkillNames(descriptor)
			if len(requiredSkills) > 0 && !ToolInvocationAnySkillActive(ctx, requiredSkills) {
				continue
			}
			if category != "" && strings.ToLower(descriptor.Category) != category {
				continue
			}
			haystack := strings.ToLower(strings.Join([]string{
				descriptor.Name, descriptor.DisplayName, descriptor.Description, descriptor.Category,
				descriptor.Permission, descriptor.OutputSummary, descriptor.RiskLevel, strings.Join(requiredSkills, " "),
			}, " "))
			if query != "" && !strings.Contains(haystack, query) {
				continue
			}
			item := map[string]any{
				"name": descriptor.Name, "displayName": descriptor.DisplayName, "category": descriptor.Category,
				"permission": descriptor.Permission, "riskLevel": descriptor.RiskLevel, "description": descriptor.Description,
				"inputSchema": descriptor.InputSchema, "outputSummary": descriptor.OutputSummary,
				"requiresApprovalIn": descriptor.RequiresApprovalIn,
			}
			if descriptor.RequiredSkill != "" {
				item["requiredSkill"] = descriptor.RequiredSkill
			}
			if len(requiredSkills) > 0 {
				item["requiredSkills"] = requiredSkills
			}
			matches = append(matches, item)
			if len(matches) >= limit {
				break
			}
		}
		return map[string]any{"query": query, "category": category, "tools": matches, "totalReturned": len(matches)}, nil
	})
	return registry
}

func registerInputRequestTool(registry *ToolRegistry) {
	registry.Register(inputRequestToolDescriptor(), func(context.Context, map[string]any) (any, error) {
		return nil, fmt.Errorf("%s is only available from an ADK agent run", interactionRequestUserTool)
	})
}

func (r *ToolRegistry) Register(descriptor ToolDescriptor, handler ToolFunc) {
	if r == nil || strings.TrimSpace(descriptor.Name) == "" || handler == nil {
		return
	}
	descriptor.Name = strings.TrimSpace(descriptor.Name)
	descriptor.Permission = strings.TrimSpace(descriptor.Permission)
	descriptor.RequiredSkill = strings.TrimSpace(descriptor.RequiredSkill)
	descriptor.RequiredSkills = normalizeStringSlice(descriptor.RequiredSkills)
	if len(descriptor.AllowedModes) == 0 {
		if descriptor.Permission == "live_trading" {
			descriptor.AllowedModes = []string{PermissionModeAll}
		} else {
			descriptor.AllowedModes = []string{PermissionModeApproval, PermissionModeLessApproval, PermissionModeAll}
		}
	}
	if descriptor.InputSchema == nil {
		descriptor.InputSchema = defaultToolInputSchema(descriptor.Name)
	}
	if descriptor.RiskLevel == "" {
		descriptor.RiskLevel = defaultToolRiskLevelForTool(descriptor.Name, descriptor.Permission)
	}
	r.tools[descriptor.Name] = RegisteredTool{Descriptor: descriptor, Handler: handler}
}

func (r *ToolRegistry) List() []ToolDescriptor {
	if r == nil {
		return []ToolDescriptor{}
	}
	items := make([]ToolDescriptor, 0, len(r.tools))
	for _, tool := range r.tools {
		items = append(items, tool.Descriptor)
	}
	sort.Slice(items, func(i int, j int) bool { return items[i].Name < items[j].Name })
	return items
}

func (r *ToolRegistry) Get(name string) (RegisteredTool, bool) {
	if r == nil {
		return RegisteredTool{}, false
	}
	tool, ok := r.tools[strings.TrimSpace(name)]
	return tool, ok
}

func (r *ToolRegistry) AvailableNames() []string {
	descriptors := r.List()
	names := make([]string, 0, len(descriptors))
	for _, descriptor := range descriptors {
		names = append(names, descriptor.Name)
	}
	return names
}

type ToolInvocation struct {
	Name  string
	Input map[string]any
}

func ToolDescriptorsForAgent(agent Agent, registry *ToolRegistry) []ToolDescriptor {
	if registry == nil {
		return nil
	}
	allowed := map[string]struct{}{}
	if len(agent.Tools) == 0 {
		for _, name := range registry.AvailableNames() {
			allowed[name] = struct{}{}
		}
	} else {
		for _, name := range agent.Tools {
			if canonical, ok := registry.CanonicalName(name); ok {
				allowed[canonical] = struct{}{}
			}
		}
	}
	if _, researchEnabled := allowed["strategy.research_backtest"]; researchEnabled {
		allowed["backtest.kline_sync_status"] = struct{}{}
	}
	if _, optimizeEnabled := allowed["strategy.optimize"]; optimizeEnabled {
		allowed["backtest.kline_sync_status"] = struct{}{}
	}
	descriptors := registry.List()
	out := make([]ToolDescriptor, 0, len(descriptors))
	for _, descriptor := range descriptors {
		if _, ok := allowed[descriptor.Name]; ok {
			out = append(out, descriptor)
		}
	}
	return out
}

func (r *ToolRegistry) CanonicalName(name string) (string, bool) {
	if r == nil {
		return "", false
	}
	raw := strings.TrimSpace(name)
	if _, ok := r.tools[raw]; ok {
		return raw, true
	}
	normalized := normalizeToolAlias(raw)
	if normalized == "" {
		return "", false
	}
	if _, ok := r.tools[normalized]; ok {
		return normalized, true
	}
	for _, tool := range r.tools {
		if normalizeToolAlias(tool.Descriptor.DisplayName) == normalized {
			return tool.Descriptor.Name, true
		}
	}
	return "", false
}

func ToolRequiresApproval(descriptor ToolDescriptor, mode string) bool {
	mode = normalizePermissionMode(mode)
	if toolExplicitlySkipsApproval(descriptor.Name) {
		return false
	}
	if slices.Contains(descriptor.RequiresApprovalIn, mode) {
		return true
	}
	if mode == PermissionModeApproval && mediumOrHigherRisk(descriptor.RiskLevel) {
		return true
	}
	switch descriptor.Permission {
	case "install_skill", "write_strategy", "optimize_strategy", "write_task", "write_memory":
		return mode == PermissionModeApproval
	case "create_strategy_instance":
		return mode != PermissionModeAll
	case "live_trading":
		return false
	default:
		return false
	}
}

func mediumOrHigherRisk(risk string) bool {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "medium", "high", "critical":
		return true
	default:
		return false
	}
}

func toolExplicitlySkipsApproval(name string) bool {
	switch strings.TrimSpace(name) {
	case "tasks.create", "tasks.update", "tasks.delete", "memory.remember", "memory.forget", "strategy.save_draft", "strategy.research_backtest":
		return true
	default:
		return false
	}
}

func ToolAllowedInMode(descriptor ToolDescriptor, mode string) bool {
	mode = normalizePermissionMode(mode)
	if descriptor.Permission == "live_trading" {
		return mode == PermissionModeAll
	}
	return slices.Contains(descriptor.AllowedModes, mode)
}

func finishToolCall(call *ToolCall) {
	if call == nil {
		return
	}
	completedAt := nowString()
	call.CompletedAt = &completedAt
	call.UpdatedAt = completedAt
	startedAt, startErr := time.Parse(time.RFC3339Nano, call.StartedAt)
	completed, completedErr := time.Parse(time.RFC3339Nano, completedAt)
	if startErr == nil && completedErr == nil {
		call.DurationMs = completed.Sub(startedAt).Milliseconds()
	}
}

func executeRegisteredTool(ctx context.Context, registered RegisteredTool, input map[string]any) (output any, err error) {
	ctx = contextWithToolInvocationMetadata(ctx)
	toolCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	type result struct {
		output any
		err    error
	}
	done := make(chan result, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				select {
				case done <- result{err: fmt.Errorf("tool panic: %v", recovered)}:
				default:
				}
			}
		}()
		value, callErr := registered.Handler(toolCtx, input)
		select {
		case done <- result{output: value, err: callErr}:
		default:
			// Context already timed out; discard result to avoid goroutine leak.
		}
	}()
	select {
	case <-toolCtx.Done():
		return nil, toolCtx.Err()
	case result := <-done:
		return result.output, result.err
	}
}

func limitToolOutputWithMetadata(output any) (any, bool) {
	raw, err := json.Marshal(output)
	if err != nil || len(raw) <= MaxToolOutputBytes {
		return output, false
	}
	return map[string]any{
		"truncated": true,
		"preview":   string(raw[:MaxToolOutputBytes]),
	}, true
}

func limitToolOutput(output any) any {
	limited, _ := limitToolOutputWithMetadata(output)
	return limited
}

func summarizeToolOutput(toolName string, output any) string {
	raw, err := json.Marshal(output)
	if err != nil {
		return fmt.Sprintf("%s: %v", toolName, output)
	}
	text := string(raw)
	if len(text) > 1800 {
		text = text[:1800] + "...(truncated)"
	}
	return fmt.Sprintf("%s => %s", toolName, text)
}

// sanitizeToolNameForOpenAI replaces characters that OpenAI-compatible providers
// reject in function names. The API requires names matching ^[a-zA-Z0-9_-]+$.
func sanitizeToolNameForOpenAI(name string) string {
	return strings.ReplaceAll(name, ".", "-")
}

// restoreToolNameFromOpenAI reverses the sanitization applied by sanitizeToolNameForOpenAI.
func restoreToolNameFromOpenAI(name string) string {
	return strings.ReplaceAll(name, "-", ".")
}

func openAIToolsFromDescriptors(descriptors []ToolDescriptor) []openAITool {
	tools := make([]openAITool, 0, len(descriptors))
	for _, descriptor := range descriptors {
		if strings.TrimSpace(descriptor.Name) == "" {
			continue
		}
		schema := descriptor.InputSchema
		if schema == nil {
			schema = defaultToolInputSchema(descriptor.Name)
		}
		schema = sanitizeSchemaForOpenAI(schema)
		description := strings.TrimSpace(descriptor.Description)
		if descriptor.OutputSummary != "" {
			description += "\nOutput: " + descriptor.OutputSummary
		}
		if descriptor.RiskLevel != "" {
			description += "\nRisk: " + descriptor.RiskLevel
		}
		tools = append(tools, openAITool{
			Type: "function",
			Function: openAIToolFunction{
				Name:        sanitizeToolNameForOpenAI(descriptor.Name),
				Description: strings.TrimSpace(description),
				Parameters:  schema,
			},
		})
	}
	return tools
}

func toolInvocationsFromOpenAI(calls []openAIToolCall) []ToolInvocation {
	invocations := make([]ToolInvocation, 0, len(calls))
	for _, call := range calls {
		name := restoreToolNameFromOpenAI(strings.TrimSpace(call.Function.Name))
		if name == "" {
			continue
		}
		input := map[string]any{}
		if strings.TrimSpace(call.Function.Arguments) != "" {
			if err := json.Unmarshal([]byte(call.Function.Arguments), &input); err != nil {
				input = map[string]any{"rawParameters": call.Function.Arguments, "parseError": err.Error()}
			}
		}
		invocations = append(invocations, ToolInvocation{Name: name, Input: input})
		if len(invocations) >= 5 {
			break
		}
	}
	return invocations
}
