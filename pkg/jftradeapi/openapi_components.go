package jftradeapi

func buildOpenAPIComponents() map[string]any {
	return map[string]any{
		"schemas": map[string]any{
			"ApiError": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"code":    map[string]any{"type": "string", "example": "BAD_REQUEST"},
					"message": map[string]any{"type": "string", "example": "accountId is required"},
				},
				"required": []string{"code", "message"},
			},
			"FutuIntegrationConfig": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"type":                    map[string]any{"type": "string", "example": "futu"},
					"host":                    map[string]any{"type": "string", "example": "127.0.0.1"},
					"apiPort":                 map[string]any{"type": "integer", "example": 11110},
					"websocketPort":           map[string]any{"type": "integer", "example": 11111},
					"maxWebSocketConnections": map[string]any{"type": "integer", "example": 20},
					"useEncryption":           map[string]any{"type": "boolean", "example": false},
					"websocketKey":            map[string]any{"type": "string", "example": "123456"},
					"tradeMarket":             map[string]any{"type": "string", "example": "HK"},
					"securityFirm":            map[string]any{"type": "string", "example": "FUTUSECURITIES"},
				},
				"required": []string{"type", "host", "apiPort", "websocketPort", "maxWebSocketConnections", "useEncryption", "tradeMarket", "securityFirm"},
			},
			"BrokerIntegration": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"brokerId":  map[string]any{"type": "string", "example": "futu"},
					"enabled":   map[string]any{"type": "boolean", "example": true},
					"config":    schemaRef("FutuIntegrationConfig"),
					"updatedAt": map[string]any{"type": "string", "format": "date-time"},
					"createdAt": map[string]any{"type": "string", "format": "date-time"},
				},
				"required": []string{"brokerId", "enabled", "config", "updatedAt", "createdAt"},
			},
			"BrokerIntegrationSaveRequest": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"enabled": map[string]any{"type": "boolean", "example": true},
					"config":  schemaRef("FutuIntegrationConfig"),
				},
				"required": []string{"enabled", "config"},
			},
			"ManagedBrokerAccount": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":                 map[string]any{"type": "string", "example": "futu|SIMULATE|123456|HK"},
					"brokerId":           map[string]any{"type": "string", "example": "futu"},
					"accountId":          map[string]any{"type": "string", "example": "123456"},
					"displayName":        map[string]any{"type": "string", "example": "模拟账户"},
					"tradingEnvironment": map[string]any{"type": "string", "example": "SIMULATE"},
					"market":             map[string]any{"type": "string", "example": "HK"},
					"securityFirm":       map[string]any{"type": "string", "nullable": true, "example": "FUTUSECURITIES"},
					"enabled":            map[string]any{"type": "boolean", "example": true},
					"updatedAt":          map[string]any{"type": "string", "format": "date-time"},
					"createdAt":          map[string]any{"type": "string", "format": "date-time"},
				},
				"required": []string{"id", "brokerId", "accountId", "displayName", "tradingEnvironment", "market", "enabled", "updatedAt", "createdAt"},
			},
			"ManagedBrokerAccountWriteRequest": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"brokerId":           map[string]any{"type": "string", "example": "futu"},
					"accountId":          map[string]any{"type": "string", "example": "123456"},
					"displayName":        map[string]any{"type": "string", "example": "模拟账户"},
					"tradingEnvironment": map[string]any{"type": "string", "example": "SIMULATE"},
					"market":             map[string]any{"type": "string", "example": "HK"},
					"securityFirm":       map[string]any{"type": "string", "example": "FUTUSECURITIES"},
					"enabled":            map[string]any{"type": "boolean", "example": true},
				},
				"required": []string{"accountId"},
			},
			"MarketSubscriptionRequest": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"channel":    map[string]any{"type": "string", "example": "kline"},
					"market":     map[string]any{"type": "string", "example": "HK"},
					"symbol":     map[string]any{"type": "string", "example": "00700"},
					"interval":   map[string]any{"type": "string", "example": "1m"},
					"consumerId": map[string]any{"type": "string", "example": "chart-main"},
				},
				"required": []string{"channel", "market", "symbol"},
			},
			"MarketSubscriptionHeartbeatRequest": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"consumerId": map[string]any{"type": "string", "example": "chart-main"},
				},
				"required": []string{"consumerId"},
			},
			"StrategyVisualNode": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":         map[string]any{"type": "string", "example": "on-kline-root"},
					"type":       map[string]any{"type": "string", "example": "circle"},
					"x":          map[string]any{"type": "number", "example": 180},
					"y":          map[string]any{"type": "number", "example": 300},
					"text":       map[string]any{"type": "string", "example": "K 线收盘"},
					"properties": map[string]any{"type": "object", "additionalProperties": true},
				},
				"required": []string{"id", "type", "x", "y", "text", "properties"},
			},
			"StrategyVisualEdge": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":           map[string]any{"type": "string", "example": "edge-kline-log"},
					"type":         map[string]any{"type": "string", "example": "polyline"},
					"sourceNodeId": map[string]any{"type": "string", "example": "on-kline-root"},
					"targetNodeId": map[string]any{"type": "string", "example": "kline-log"},
					"text":         map[string]any{"type": "string", "example": ""},
					"properties":   map[string]any{"type": "object", "additionalProperties": true},
				},
				"required": []string{"type", "sourceNodeId", "targetNodeId"},
			},
			"StrategyVisualModel": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"engine":  map[string]any{"type": "string", "example": "logic-flow"},
					"version": map[string]any{"type": "integer", "example": 1},
					"nodes":   map[string]any{"type": "array", "items": schemaRef("StrategyVisualNode")},
					"edges":   map[string]any{"type": "array", "items": schemaRef("StrategyVisualEdge")},
				},
				"required": []string{"engine", "version", "nodes", "edges"},
			},
			"StrategyDefinition": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":           map[string]any{"type": "string", "example": "dsl-mean-revert"},
					"name":         map[string]any{"type": "string", "example": "DSL Mean Revert"},
					"version":      map[string]any{"type": "string", "example": "0.1.0"},
					"description":  map[string]any{"type": "string", "example": "使用 DSL v1 的最小均值回归策略"},
					"runtime":      map[string]any{"type": "string", "example": "dsl-go-plan"},
					"sourceFormat": map[string]any{"type": "string", "example": "dsl-v1"},
					"symbol":       map[string]any{"type": "string", "example": "00700"},
					"interval":     map[string]any{"type": "string", "example": "1m"},
					"script":       map[string]any{"type": "string", "example": "strategy DSL Mean Revert\non kline_close:\n  log \"kline closed\""},
					"visualModel":  schemaRef("StrategyVisualModel"),
					"createdAt":    map[string]any{"type": "string", "format": "date-time"},
					"updatedAt":    map[string]any{"type": "string", "format": "date-time"},
				},
				"required": []string{"id", "name", "version", "runtime", "sourceFormat", "symbol", "interval", "script", "createdAt", "updatedAt"},
			},
			"StrategyDefinitionWriteRequest": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":           map[string]any{"type": "string", "example": "dsl-mean-revert"},
					"name":         map[string]any{"type": "string", "example": "DSL Mean Revert"},
					"version":      map[string]any{"type": "string", "example": "0.1.0"},
					"description":  map[string]any{"type": "string", "example": "使用 DSL v1 的最小均值回归策略"},
					"runtime":      map[string]any{"type": "string", "example": "dsl-go-plan"},
					"sourceFormat": map[string]any{"type": "string", "example": "dsl-v1"},
					"symbol":       map[string]any{"type": "string", "example": "00700"},
					"interval":     map[string]any{"type": "string", "example": "1m"},
					"script":       map[string]any{"type": "string", "example": "strategy DSL Mean Revert\non kline_close:\n  log \"kline closed\""},
					"visualModel":  schemaRef("StrategyVisualModel"),
				},
				"required": []string{"name", "symbol", "interval", "script"},
			},
			"StrategyDefinitionSummary": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"strategyId": map[string]any{"type": "string", "example": "dsl-mean-revert"},
					"name":       map[string]any{"type": "string", "example": "DSL Mean Revert"},
					"version":    map[string]any{"type": "string", "example": "0.1.0"},
				},
				"required": []string{"strategyId", "name", "version"},
			},
			"StrategyInstance": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":           map[string]any{"type": "string", "example": "dsl-mean-revert-20260523000000"},
					"pluginId":     map[string]any{"type": "string", "example": "dsl-go-plan"},
					"definition":   schemaRef("StrategyDefinitionSummary"),
					"runtime":      map[string]any{"type": "string", "example": "dsl-go-plan"},
					"sourceFormat": map[string]any{"type": "string", "example": "dsl-v1"},
					"startable":    map[string]any{"type": "boolean", "example": true},
					"params":       map[string]any{"type": "object", "additionalProperties": true},
					"status":       map[string]any{"type": "string", "example": "STOPPED"},
					"createdAt":    map[string]any{"type": "string", "format": "date-time"},
					"logs": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
				"required": []string{"id", "definition", "runtime", "sourceFormat", "startable", "params", "status", "createdAt", "logs"},
			},
			"StrategyLogsResponse": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instanceId": map[string]any{"type": "string", "example": "js-mean-revert-20260523000000"},
					"logs": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
				},
				"required": []string{"instanceId", "logs"},
			},
			"StrategyAuditEntry": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instanceId": map[string]any{"type": "string", "example": "js-mean-revert-20260523000000"},
					"kind":       map[string]any{"type": "string", "example": "started"},
					"detail":     map[string]any{"type": "string", "example": "strategy runtime requested start"},
					"at":         map[string]any{"type": "string", "format": "date-time"},
				},
				"required": []string{"instanceId", "kind", "at"},
			},
			"StrategyAuditResponse": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instanceId": map[string]any{"type": "string", "example": "js-mean-revert-20260523000000"},
					"entries": map[string]any{
						"type":  "array",
						"items": schemaRef("StrategyAuditEntry"),
					},
				},
				"required": []string{"instanceId", "entries"},
			},
		},
	}
}
