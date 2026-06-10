package adk

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ToolFunc func(context.Context, map[string]any) (any, error)

type toolContextKey string

const toolContextAgentKey toolContextKey = "adkToolAgent"

func contextWithToolAgent(ctx context.Context, agent Agent) context.Context {
	return context.WithValue(ctx, toolContextAgentKey, agent)
}

func toolAgentFromContext(ctx context.Context) (Agent, bool) {
	agent, ok := ctx.Value(toolContextAgentKey).(Agent)
	return agent, ok
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
	registry.Register(ToolDescriptor{
		Name:               "http.fetch",
		DisplayName:        "读取外部 HTTP",
		Description:        "读取公网 HTTP/HTTPS 文本或 JSON 资源，默认阻止本机、私网和 metadata 地址。",
		Category:           "external",
		Permission:         "read_external",
		AllowedModes:       []string{PermissionModeApproval, PermissionModeSandboxAuto, PermissionModeHighAuto},
		RequiresApprovalIn: nil,
	}, httpFetchTool)
	registry.Register(ToolDescriptor{
		Name:               "tools.search",
		DisplayName:        "搜索 ADK 工具",
		Description:        "按名称、分类、权限、风险等级或描述搜索当前已注册的 JFTrade ADK 工具。",
		Category:           "system",
		Permission:         "read_internal",
		AllowedModes:       []string{PermissionModeApproval, PermissionModeSandboxAuto, PermissionModeHighAuto},
		RequiresApprovalIn: nil,
		OutputSummary:      "匹配到的工具 descriptor、风险等级与输入 schema。",
		RiskLevel:          "low",
	}, func(ctx context.Context, input map[string]any) (any, error) {
		query := strings.ToLower(strings.TrimSpace(toolStringValue(input, "query")))
		category := strings.ToLower(strings.TrimSpace(toolStringValue(input, "category")))
		limit := toolIntValue(input, "limit", 12)
		if limit < 1 {
			limit = 1
		}
		if limit > 50 {
			limit = 50
		}
		descriptors := registry.List()
		if agent, ok := toolAgentFromContext(ctx); ok {
			descriptors = ToolDescriptorsForAgent(agent, registry)
		}
		matches := make([]map[string]any, 0)
		for _, descriptor := range descriptors {
			if descriptor.Name == "tools.search" {
				continue
			}
			if category != "" && strings.ToLower(descriptor.Category) != category {
				continue
			}
			haystack := strings.ToLower(strings.Join([]string{
				descriptor.Name, descriptor.DisplayName, descriptor.Description, descriptor.Category,
				descriptor.Permission, descriptor.OutputSummary, descriptor.RiskLevel,
			}, " "))
			if query != "" && !strings.Contains(haystack, query) {
				continue
			}
			matches = append(matches, map[string]any{
				"name": descriptor.Name, "displayName": descriptor.DisplayName, "category": descriptor.Category,
				"permission": descriptor.Permission, "riskLevel": descriptor.RiskLevel, "description": descriptor.Description,
				"inputSchema": descriptor.InputSchema, "outputSummary": descriptor.OutputSummary,
				"requiresApprovalIn": descriptor.RequiresApprovalIn,
			})
			if len(matches) >= limit {
				break
			}
		}
		return map[string]any{"query": query, "category": category, "tools": matches, "totalReturned": len(matches)}, nil
	})
	return registry
}

func (r *ToolRegistry) Register(descriptor ToolDescriptor, handler ToolFunc) {
	if r == nil || strings.TrimSpace(descriptor.Name) == "" || handler == nil {
		return
	}
	descriptor.Name = strings.TrimSpace(descriptor.Name)
	descriptor.Permission = strings.TrimSpace(descriptor.Permission)
	if len(descriptor.AllowedModes) == 0 {
		descriptor.AllowedModes = []string{PermissionModeApproval, PermissionModeSandboxAuto, PermissionModeHighAuto}
	}
	if descriptor.InputSchema == nil {
		descriptor.InputSchema = defaultToolInputSchema(descriptor.Name)
	}
	if descriptor.RiskLevel == "" {
		descriptor.RiskLevel = defaultToolRiskLevel(descriptor.Permission)
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

func SelectTools(question string, agent Agent, registry *ToolRegistry) []string {
	invocations := SelectToolInvocations(question, agent, registry)
	names := make([]string, 0, len(invocations))
	for _, invocation := range invocations {
		names = append(names, invocation.Name)
	}
	return names
}

type ToolInvocation struct {
	Name  string
	Input map[string]any
}

func SelectToolInvocations(question string, agent Agent, registry *ToolRegistry) []ToolInvocation {
	if registry == nil {
		return []ToolInvocation{}
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
	lower := strings.ToLower(question)
	candidates := []ToolInvocation{}
	add := func(name string, input map[string]any) {
		canonical, ok := registry.CanonicalName(name)
		if !ok {
			return
		}
		name = canonical
		if _, ok := allowed[name]; !ok {
			return
		}
		if _, ok := registry.Get(name); !ok {
			return
		}
		for _, existing := range candidates {
			if existing.Name == name {
				return
			}
		}
		if input == nil {
			input = inferToolInput(name, question)
		}
		candidates = append(candidates, ToolInvocation{Name: name, Input: input})
	}
	for _, invocation := range parseExecuteToolInvocations(question, registry) {
		add(invocation.Name, invocation.Input)
	}
	if strings.Contains(lower, "@") {
		for name := range allowed {
			if strings.Contains(lower, "@"+strings.ToLower(name)) {
				add(name, nil)
			}
		}
	}
	if strings.Contains(lower, "行情") || strings.Contains(lower, "market") || strings.Contains(lower, "quote") || strings.Contains(lower, "订阅") {
		add("market.subscriptions", nil)
		add("market.snapshot", nil)
	}
	if strings.Contains(lower, "持仓") || strings.Contains(lower, "账户") || strings.Contains(lower, "portfolio") || strings.Contains(lower, "position") || strings.Contains(lower, "订单") {
		add("portfolio.summary", nil)
		add("account.orders", nil)
	}
	if strings.Contains(lower, "策略") || strings.Contains(lower, "strategy") || strings.Contains(lower, "定义") {
		add("strategy.definitions", nil)
	}
	if strings.Contains(lower, "dsl") || strings.Contains(lower, "语法") || strings.Contains(lower, "spec") {
		add("strategy.dsl_spec", nil)
	}
	if (strings.Contains(lower, "dsl") || strings.Contains(lower, "语法") || strings.Contains(lower, "脚本") || strings.Contains(lower, "script")) &&
		(strings.Contains(lower, "校验") || strings.Contains(lower, "验证") || strings.Contains(lower, "检查") || strings.Contains(lower, "validate")) {
		add("strategy.validate_dsl", nil)
	}
	if strings.Contains(lower, "修改策略定义") || strings.Contains(lower, "save definition") || strings.Contains(lower, "update definition") ||
		((strings.Contains(lower, "保存") || strings.Contains(lower, "save") || strings.Contains(lower, "更新") || strings.Contains(lower, "update")) &&
			(strings.Contains(lower, "策略") || strings.Contains(lower, "strategy") || strings.Contains(lower, "定义") || strings.Contains(lower, "definition"))) {
		add("strategy.save_definition", nil)
	}
	if strings.Contains(lower, "notify_only") || strings.Contains(lower, "executionmode") || strings.Contains(lower, "执行模式") ||
		strings.Contains(lower, "切换模式") || strings.Contains(lower, "修改模式") ||
		((strings.Contains(lower, "live") || strings.Contains(lower, "mode")) && strings.Contains(lower, "instance")) {
		add("strategy.update_instance_mode", nil)
	}
	if strings.Contains(lower, "回测") || strings.Contains(lower, "backtest") || strings.Contains(lower, "优化") || strings.Contains(lower, "optimize") {
		add("backtest.runs", nil)
		if strings.Contains(lower, "优化") || strings.Contains(lower, "optimize") {
			add("strategy.optimize", nil)
		}
	}
	if strings.Contains(lower, "系统") || strings.Contains(lower, "状态") || strings.Contains(lower, "system") || strings.Contains(lower, "opend") {
		add("system.status", nil)
		add("system.futu_opend", nil)
	}
	if strings.Contains(lower, "http") || strings.Contains(lower, "https://") || strings.Contains(lower, "http://") || strings.Contains(lower, "外部") || strings.Contains(lower, "网页") {
		add("http.fetch", nil)
	}
	if strings.Contains(lower, "depth") || strings.Contains(lower, "order book") || strings.Contains(lower, "盘口") {
		add("market.depth", nil)
	}
	if strings.Contains(lower, "broker") || strings.Contains(lower, "order") || strings.Contains(lower, "订单") {
		add("broker.orders", nil)
	}
	if strings.Contains(lower, "fill") || strings.Contains(lower, "成交") {
		add("broker.fills", nil)
	}
	if strings.Contains(lower, "fee") || strings.Contains(lower, "费用") {
		add("broker.fees", nil)
	}
	if strings.Contains(lower, "margin") || strings.Contains(lower, "保证金") || strings.Contains(lower, "融资") {
		add("broker.margin_ratios", nil)
	}
	if strings.Contains(lower, "risk") || strings.Contains(lower, "风控") || strings.Contains(lower, "kill switch") {
		add("risk.state", nil)
		add("risk.events", nil)
	}
	if strings.Contains(lower, "task") || strings.Contains(lower, "任务") {
		add("tasks.list", nil)
	}
	if strings.Contains(lower, "memory") || strings.Contains(lower, "记忆") || strings.Contains(lower, "偏好") {
		add("memory.list", nil)
	}
	if strings.Contains(lower, "tool") || strings.Contains(lower, "工具") || strings.Contains(lower, "search") {
		add("tools.search", nil)
	}
	if len(candidates) == 0 {
		add("system.status", nil)
	}
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}
	return candidates
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
	for _, requiredMode := range descriptor.RequiresApprovalIn {
		if requiredMode == mode {
			return true
		}
	}
	switch descriptor.Permission {
	case "install_skill", "write_strategy", "optimize_strategy", "write_task", "write_memory":
		return mode == PermissionModeApproval
	case "create_strategy_instance":
		return mode != PermissionModeHighAuto
	case "live_trading":
		return true
	default:
		return false
	}
}

func ToolAllowedInMode(descriptor ToolDescriptor, mode string) bool {
	mode = normalizePermissionMode(mode)
	for _, allowed := range descriptor.AllowedModes {
		if allowed == mode {
			return true
		}
	}
	return false
}

func buildToolCall(runID string, descriptor ToolDescriptor, input map[string]any, status string) ToolCall {
	now := nowString()
	id := "tool-" + uuid.NewString()
	return ToolCall{
		ID:             id,
		RunID:          runID,
		ToolName:       descriptor.Name,
		Permission:     descriptor.Permission,
		Status:         status,
		Input:          input,
		RequiresUser:   status == "PENDING_APPROVAL",
		IdempotencyKey: runID + ":" + id,
		CreatedAt:      now,
		StartedAt:      now,
		UpdatedAt:      now,
	}
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

func defaultToolInputSchema(name string) map[string]any {
	switch name {
	case "http.fetch":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{"type": "string", "description": "Public http or https URL to fetch."},
			},
			"required":             []string{"url"},
			"additionalProperties": false,
		}
	case "tools.search":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":    map[string]any{"type": "string"},
				"category": map[string]any{"type": "string"},
				"limit":    map[string]any{"type": "integer", "minimum": 1, "maximum": 50},
			},
			"additionalProperties": false,
		}
	case "tasks.create", "tasks.update":
		properties := map[string]any{
			"title":       map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
			"status":      map[string]any{"type": "string", "enum": []string{"TODO", "IN_PROGRESS", "BLOCKED", "DONE", "CANCELLED"}},
			"agentId":     map[string]any{"type": "string"},
			"runId":       map[string]any{"type": "string"},
			"dependsOn":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		}
		required := []string{"title"}
		if name == "tasks.update" {
			properties["id"] = map[string]any{"type": "string"}
			required = []string{"id"}
		}
		return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
	case "tasks.delete":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string"},
			},
			"required":             []string{"id"},
			"additionalProperties": false,
		}
	case "tasks.list":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":  map[string]any{"type": "string"},
				"agentId": map[string]any{"type": "string"},
				"runId":   map[string]any{"type": "string"},
				"limit":   map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
				"offset":  map[string]any{"type": "integer", "minimum": 0},
			},
			"additionalProperties": false,
		}
	case "memory.remember":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"key":     map[string]any{"type": "string"},
				"value":   map[string]any{"type": "string"},
				"scope":   map[string]any{"type": "string", "enum": []string{"workspace", "agent"}},
				"agentId": map[string]any{"type": "string"},
			},
			"required":             []string{"key", "value"},
			"additionalProperties": false,
		}
	case "memory.list":
		return map[string]any{"type": "object", "properties": map[string]any{"scope": map[string]any{"type": "string", "enum": []string{"workspace", "agent"}}, "agentId": map[string]any{"type": "string"}, "key": map[string]any{"type": "string"}}, "additionalProperties": false}
	case "memory.forget":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "string"},
			},
			"required":             []string{"id"},
			"additionalProperties": false,
		}
	case "broker.orders", "broker.fills":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tradingEnvironment": map[string]any{"type": "string", "enum": []string{"SIMULATE", "REAL"}},
				"accountId":          map[string]any{"type": "string"},
				"market":             map[string]any{"type": "string"},
				"scope":              map[string]any{"type": "string", "enum": []string{"CURRENT", "HISTORY"}},
				"symbol":             map[string]any{"type": "string"},
				"startTime":          map[string]any{"type": "string"},
				"endTime":            map[string]any{"type": "string"},
			},
			"additionalProperties": false,
		}
	case "broker.cash_flows":
		return map[string]any{"type": "object", "properties": map[string]any{"clearingDate": map[string]any{"type": "string"}, "direction": map[string]any{"type": "string"}, "tradingEnvironment": map[string]any{"type": "string"}, "accountId": map[string]any{"type": "string"}, "market": map[string]any{"type": "string"}}, "required": []string{"clearingDate"}, "additionalProperties": false}
	case "broker.fees":
		return map[string]any{"type": "object", "properties": map[string]any{"orderIdEx": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "orderIdExList": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "tradingEnvironment": map[string]any{"type": "string"}, "accountId": map[string]any{"type": "string"}, "market": map[string]any{"type": "string"}}, "additionalProperties": false}
	case "broker.margin_ratios":
		return map[string]any{"type": "object", "properties": map[string]any{"symbol": map[string]any{"type": "string"}, "symbols": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "tradingEnvironment": map[string]any{"type": "string"}, "accountId": map[string]any{"type": "string"}, "market": map[string]any{"type": "string"}}, "additionalProperties": false}
	case "market.depth":
		return map[string]any{"type": "object", "properties": map[string]any{"market": map[string]any{"type": "string"}, "symbol": map[string]any{"type": "string"}, "query": map[string]any{"type": "string"}, "num": map[string]any{"type": "integer", "minimum": 1, "maximum": 50}}, "required": []string{"market", "symbol"}, "additionalProperties": false}
	case "execution.order_events":
		return map[string]any{"type": "object", "properties": map[string]any{"internalOrderId": map[string]any{"type": "string"}}, "additionalProperties": false}
	case "market.snapshot", "market.candles":
		properties := map[string]any{
			"query":  map[string]any{"type": "string", "description": "原始用户请求，可包含类似 HK.00700 或 US.AAPL 的标的。"},
			"market": map[string]any{"type": "string", "enum": []string{"HK", "US", "SH", "SZ", "CN", "JP", "SG"}},
			"symbol": map[string]any{"type": "string"},
		}
		if name == "market.candles" {
			properties["period"] = map[string]any{"type": "string", "description": "K 线周期，例如 1m、5m、1d。"}
			properties["limit"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 500}
		}
		return map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             []string{"market", "symbol"},
			"additionalProperties": false,
		}
	case "portfolio.summary":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"accountId":          map[string]any{"type": "string"},
				"tradingEnvironment": map[string]any{"type": "string", "enum": []string{"SIMULATE", "REAL"}},
				"market":             map[string]any{"type": "string"},
			},
			"additionalProperties": false,
		}
	case "strategy.optimize":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"definitionIds":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1, "maxItems": 12},
				"market":         map[string]any{"type": "string"},
				"symbol":         map[string]any{"type": "string"},
				"interval":       map[string]any{"type": "string"},
				"startTime":      map[string]any{"type": "string"},
				"endTime":        map[string]any{"type": "string"},
				"initialBalance": map[string]any{"type": "number", "exclusiveMinimum": 0},
				"objective":      map[string]any{"type": "string", "enum": []string{"return", "sharpe", "drawdown"}},
			},
			"required":             []string{"definitionIds", "market", "symbol", "startTime", "endTime"},
			"additionalProperties": false,
		}
	case "strategy.dsl_spec":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"section":         map[string]any{"type": "string", "enum": []string{"overview", "syntax", "expressions", "indicators", "orders", "protect", "examples"}},
				"includeExamples": map[string]any{"type": "boolean"},
			},
			"additionalProperties": false,
		}
	case "strategy.validate_dsl":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script":              map[string]any{"type": "string", "description": "待校验的 JFTrade DSL v1 策略脚本。"},
				"includeRequirements": map[string]any{"type": "boolean"},
			},
			"required":             []string{"script"},
			"additionalProperties": false,
		}
	case "strategy.save_draft":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string"},
				"script": map[string]any{"type": "string", "description": "JFTrade DSL v1 策略脚本，不要传入 TradingView Pine Script。"},
			},
			"additionalProperties": false,
		}
	case "strategy.save_definition":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"definitionId": map[string]any{"type": "string"},
				"name":         map[string]any{"type": "string"},
				"description":  map[string]any{"type": "string"},
				"script":       map[string]any{"type": "string", "description": "JFTrade DSL v1 策略脚本，不要传入 TradingView Pine Script。"},
				"symbol":       map[string]any{"type": "string"},
				"interval":     map[string]any{"type": "string"},
				"visualModel":  map[string]any{"type": "object"},
			},
			"required":             []string{"name", "script"},
			"additionalProperties": false,
		}
	case "strategy.update_instance_mode":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"instanceId":    map[string]any{"type": "string"},
				"executionMode": map[string]any{"type": "string", "enum": []string{"live", "notify_only"}},
			},
			"required":             []string{"instanceId", "executionMode"},
			"additionalProperties": false,
		}
	default:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "原始用户请求或提取后的查询内容。"},
			},
			"additionalProperties": false,
		}
	}
}

func defaultToolRiskLevel(permission string) string {
	switch permission {
	case "read_internal":
		return "low"
	case "read_external":
		return "medium"
	case "write_strategy", "optimize_strategy", "install_skill":
		return "high"
	case "live_trading":
		return "critical"
	default:
		return "medium"
	}
}

func toolStringValue(input map[string]any, key string) string {
	value, _ := input[key].(string)
	return value
}

func toolIntValue(input map[string]any, key string, fallback int) int {
	switch value := input[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}

var (
	executeToolTagPattern      = regexp.MustCompile(`(?is)<\s*execute-tool\b([^>]*)/?>`)
	executeToolNamePattern     = regexp.MustCompile(`(?is)\bname\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	executeToolParamsPattern   = regexp.MustCompile(`(?is)\bparameters\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	executeToolParamKeyPattern = regexp.MustCompile(`(?is)\b(?:params|input|arguments)\s*=\s*(?:"([^"]*)"|'([^']*)')`)
)

func parseExecuteToolInvocations(text string, registry *ToolRegistry) []ToolInvocation {
	matches := executeToolTagPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return []ToolInvocation{}
	}
	invocations := make([]ToolInvocation, 0, len(matches))
	for _, match := range matches {
		attrs := ""
		if len(match) > 1 {
			attrs = match[1]
		}
		name := firstAttrValue(executeToolNamePattern, attrs)
		if name == "" {
			continue
		}
		canonical, ok := registry.CanonicalName(name)
		if !ok {
			continue
		}
		input := map[string]any{}
		rawParams := firstAttrValue(executeToolParamsPattern, attrs)
		if rawParams == "" {
			rawParams = firstAttrValue(executeToolParamKeyPattern, attrs)
		}
		if strings.TrimSpace(rawParams) != "" {
			if err := json.Unmarshal([]byte(rawParams), &input); err != nil {
				input = map[string]any{"rawParameters": rawParams, "parseError": err.Error()}
			}
		}
		invocations = append(invocations, ToolInvocation{Name: canonical, Input: input})
	}
	return invocations
}

func firstAttrValue(pattern *regexp.Regexp, text string) string {
	match := pattern.FindStringSubmatch(text)
	if len(match) == 0 {
		return ""
	}
	for _, value := range match[1:] {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizeToolAlias(name string) string {
	value := strings.ToLower(strings.TrimSpace(name))
	value = strings.TrimPrefix(value, "@")
	value = strings.TrimPrefix(value, "jftrade.")
	value = strings.TrimPrefix(value, "jftrade:")
	value = strings.TrimPrefix(value, "jftrade ")
	value = strings.Join(strings.Fields(value), ".")
	value = strings.NewReplacer("-", ".", "/", ".", ":", ".").Replace(value)
	for strings.Contains(value, "..") {
		value = strings.ReplaceAll(value, "..", ".")
	}
	return strings.Trim(value, ".")
}

func httpFetchTool(ctx context.Context, input map[string]any) (any, error) {
	rawURL, _ := input["url"].(string)
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("only http and https are supported")
	}
	if err := rejectUnsafeHost(ctx, parsed.Hostname()); err != nil {
		return nil, err
	}
	timeout := 12 * time.Second
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if err := rejectUnsafeHost(req.Context(), req.URL.Hostname()); err != nil {
				return fmt.Errorf("redirect to unsafe host %q blocked: %w", req.URL.Hostname(), err)
			}
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects (max 5)")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "JFTrade-ADK/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && !(strings.Contains(contentType, "text/") || strings.Contains(contentType, "json") || strings.Contains(contentType, "xml") || strings.Contains(contentType, "rss")) {
		return nil, fmt.Errorf("unsupported content type %q", contentType)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"url":         parsed.String(),
		"status":      resp.StatusCode,
		"contentType": resp.Header.Get("Content-Type"),
		"body":        string(body),
		"truncated":   len(body) >= 1<<20,
		"fetchedAt":   nowString(),
	}, nil
}

func rejectUnsafeHost(ctx context.Context, host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("host is required")
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return fmt.Errorf("localhost targets are blocked")
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if unsafeAddr(addr) {
			return fmt.Errorf("private, loopback, link-local, multicast and metadata addresses are blocked")
		}
		return nil
	}
	resolver := net.DefaultResolver
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("resolve host: %w", err)
	}
	for _, addr := range addrs {
		if unsafeAddr(addr) {
			return fmt.Errorf("private, loopback, link-local, multicast and metadata addresses are blocked")
		}
	}
	return nil
}

func unsafeAddr(addr netip.Addr) bool {
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return true
	}
	if addr.String() == "169.254.169.254" {
		return true
	}
	return false
}
