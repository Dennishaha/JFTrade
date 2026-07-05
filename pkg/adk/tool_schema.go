package adk

import (
	"fmt"
	"strings"
)

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
	case "models.list":
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":        map[string]any{"type": "string", "description": "Filter by provider name, provider id, model, base URL or capability."},
				"providerId":   map[string]any{"type": "string", "description": "Optional ADK provider id to inspect."},
				"callableOnly": map[string]any{"type": "boolean", "description": "When true, only providers that are enabled and have an API key are returned. Defaults to true."},
				"limit":        map[string]any{"type": "integer", "minimum": 1, "maximum": 100},
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
			"childProviderId": map[string]any{"type": "string"},
			"childModel":      map[string]any{"type": "string"},
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
