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
	"sort"
	"strconv"
	"strings"
	"time"
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
	for _, requiredMode := range descriptor.RequiresApprovalIn {
		if requiredMode == mode {
			return true
		}
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
	for _, allowed := range descriptor.AllowedModes {
		if allowed == mode {
			return true
		}
	}
	return false
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
	case "workflow.wait":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"seconds":    map[string]any{"type": "number", "minimum": 0, "maximum": 25, "description": "等待秒数，最大 25 秒。"},
				"durationMs": map[string]any{"type": "integer", "minimum": 1, "maximum": 25000, "description": "等待毫秒数，最大 25000。"},
				"reason":     map[string]any{"type": "string", "description": "等待原因，用于工具输出摘要。"},
			},
			"additionalProperties": false,
		}
	case "tasks.create", "tasks.update":
		properties := map[string]any{
			"title":           map[string]any{"type": "string"},
			"description":     map[string]any{"type": "string"},
			"status":          map[string]any{"type": "string", "enum": []string{"TODO", "IN_PROGRESS", "BLOCKED", "DONE", "CANCELLED"}},
			"agentId":         map[string]any{"type": "string"},
			"runId":           map[string]any{"type": "string"},
			"dependsOn":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"order":           map[string]any{"type": "integer", "minimum": 1},
			"modeHint":        map[string]any{"type": "string", "enum": []string{"task", "loop", "chat", ""}},
			"agentRole":       map[string]any{"type": "string"},
			"plannerStepId":   map[string]any{"type": "string"},
			"planSource":      map[string]any{"type": "string", "enum": []string{"planner", "runtime", ""}},
			"workflowMode":    map[string]any{"type": "string", "enum": []string{"task", "loop", "chat", ""}},
			"objective":       map[string]any{"type": "string"},
			"plannerWarnings": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
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
	case "strategy.research_backtest":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script":           map[string]any{"type": "string", "description": "临时 Pine Script v6 策略脚本；不会保存为策略定义。"},
				"market":           map[string]any{"type": "string"},
				"symbol":           map[string]any{"type": "string"},
				"code":             map[string]any{"type": "string"},
				"interval":         map[string]any{"type": "string", "description": "回测原生周期，例如 1m、5m、1d；默认 1m。"},
				"startTime":        map[string]any{"type": "string", "description": "RFC3339 开始时间。"},
				"endTime":          map[string]any{"type": "string", "description": "RFC3339 结束时间。"},
				"initialBalance":   map[string]any{"type": "number", "exclusiveMinimum": 0},
				"rehabType":        map[string]any{"type": "string", "enum": []string{"forward", "backward", "none"}},
				"useExtendedHours": map[string]any{"type": "boolean"},
				"waitForCompletionMs": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"maximum":     25000,
					"description": "可选短等待，最多 25000ms；长轮询请用 workflow.wait 后再查 backtest.result_view。",
				},
				"resultView": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"view":       map[string]any{"type": "string", "enum": []string{"summary", "chart", "orders", "logs", "errors"}},
						"resolution": map[string]any{"type": "string", "description": "chart 视图精度，auto 或 1m/5m/1h/1d 等；不得细于原生周期。"},
						"startTime":  map[string]any{"type": "string"},
						"endTime":    map[string]any{"type": "string"},
						"include":    map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"candles", "trades", "pnlCurve", "drawdownCurve"}}},
						"limit":      map[string]any{"type": "integer", "minimum": 1, "maximum": 2000},
						"cursor":     map[string]any{"type": "string"},
					},
					"additionalProperties": false,
				},
			},
			"required":             []string{"script", "market", "startTime", "endTime"},
			"additionalProperties": false,
		}
	case "backtest.result_view":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"runId":      map[string]any{"type": "string"},
				"view":       map[string]any{"type": "string", "enum": []string{"summary", "chart", "orders", "logs", "errors"}},
				"resolution": map[string]any{"type": "string", "description": "chart 视图精度，auto 或 1m/5m/1h/1d 等；不得细于原生周期。"},
				"startTime":  map[string]any{"type": "string"},
				"endTime":    map[string]any{"type": "string"},
				"include":    map[string]any{"type": "array", "items": map[string]any{"type": "string", "enum": []string{"candles", "trades", "pnlCurve", "drawdownCurve"}}},
				"limit":      map[string]any{"type": "integer", "minimum": 1, "maximum": 2000},
				"cursor":     map[string]any{"type": "string"},
			},
			"required":             []string{"runId"},
			"additionalProperties": false,
		}
	case "backtest.kline_sync_status":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"taskId": map[string]any{"type": "string"},
				"waitForCompletionMs": map[string]any{
					"type": "integer", "minimum": 0, "maximum": 25000,
					"description": "可选短等待，最多 25000ms。",
				},
			},
			"required": []string{"taskId"}, "additionalProperties": false,
		}
	case "strategy.pine_spec":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"section":         map[string]any{"type": "string", "enum": []string{"overview", "syntax", "expressions", "indicators", "orders", "unsupported", "examples"}},
				"includeExamples": map[string]any{"type": "boolean"},
			},
			"additionalProperties": false,
		}
	case "strategy.validate_pine":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script":              map[string]any{"type": "string", "description": "待校验的 Pine Script v6 策略脚本。"},
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
				"script": map[string]any{"type": "string", "description": "Pine Script v6 策略脚本。"},
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
				"script":       map[string]any{"type": "string", "description": "Pine Script v6 策略脚本。"},
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

func defaultToolRiskLevelForTool(name string, permission string) string {
	switch name {
	case "tasks.create", "tasks.update", "tasks.delete", "memory.remember", "memory.forget", "strategy.save_draft":
		return "low"
	}
	return defaultToolRiskLevel(permission)
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
	value := jftradeOptionalTypeAssertion[string](input[key])
	return value
}

func toolIntValue(input map[string]any, key string, defaultValue int) int {
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
	return defaultValue
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
	rawURL := jftradeOptionalTypeAssertion[string](input["url"])
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
	defer func() { jftradeLogError(resp.Body.Close()) }()
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && (!strings.Contains(contentType, "text/") && !strings.Contains(contentType, "json") && !strings.Contains(contentType, "xml") && !strings.Contains(contentType, "rss")) {
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

const maxWorkflowWaitDuration = 25 * time.Second

func workflowWaitTool(ctx context.Context, input map[string]any) (any, error) {
	duration, err := workflowWaitDuration(input)
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(toolStringValue(input, "reason"))
	started := time.Now().UTC()
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}
	completed := time.Now().UTC()
	return map[string]any{
		"waitedMs":    completed.Sub(started).Milliseconds(),
		"startedAt":   started.Format(time.RFC3339Nano),
		"completedAt": completed.Format(time.RFC3339Nano),
		"reason":      reason,
	}, nil
}

func workflowWaitDuration(input map[string]any) (time.Duration, error) {
	durationMs := toolIntValue(input, "durationMs", 0)
	if durationMs <= 0 {
		switch value := input["seconds"].(type) {
		case float64:
			durationMs = int(value * 1000)
		case int:
			durationMs = value * 1000
		case string:
			seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err == nil {
				durationMs = int(seconds * 1000)
			}
		}
	}
	if durationMs <= 0 {
		return 0, fmt.Errorf("seconds or durationMs must be greater than 0")
	}
	duration := time.Duration(durationMs) * time.Millisecond
	if duration > maxWorkflowWaitDuration {
		return 0, fmt.Errorf("workflow.wait duration must be <= %s", maxWorkflowWaitDuration)
	}
	return duration, nil
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
