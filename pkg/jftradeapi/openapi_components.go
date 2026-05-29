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
					"script":       map[string]any{"type": "string", "example": "strategy DSL Mean Revert\non kline_close:\n  log \"kline closed\""},
					"visualModel":  schemaRef("StrategyVisualModel"),
					"createdAt":    map[string]any{"type": "string", "format": "date-time"},
					"updatedAt":    map[string]any{"type": "string", "format": "date-time"},
				},
				"required": []string{"id", "name", "version", "runtime", "sourceFormat", "script", "createdAt", "updatedAt"},
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
					"script":       map[string]any{"type": "string", "example": "strategy DSL Mean Revert\non kline_close:\n  log \"kline closed\""},
					"visualModel":  schemaRef("StrategyVisualModel"),
				},
				"required": []string{"name", "script"},
			},
			"StrategyBrokerAccountBinding": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"brokerId":           map[string]any{"type": "string", "example": "futu"},
					"accountId":          map[string]any{"type": "string", "example": "123456"},
					"tradingEnvironment": map[string]any{"type": "string", "example": "SIMULATE"},
					"market":             map[string]any{"type": "string", "example": "US"},
				},
				"required": []string{"brokerId", "accountId", "tradingEnvironment", "market"},
			},
			"StrategyBindingInstrument": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"market": map[string]any{"type": "string", "example": "US"},
					"code":   map[string]any{"type": "string", "example": "AAPL"},
				},
				"required": []string{"market", "code"},
			},
			"StrategyInstanceBinding": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instruments":   map[string]any{"type": "array", "items": schemaRef("StrategyBindingInstrument")},
					"symbols":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"interval":      map[string]any{"type": "string", "example": "5m"},
					"executionMode": map[string]any{"type": "string", "example": "notify_only"},
					"brokerAccount": schemaRef("StrategyBrokerAccountBinding"),
				},
				"required": []string{"symbols", "interval", "executionMode"},
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
			"StrategyDefinitionSyncStatus": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"definitionId":   map[string]any{"type": "string", "example": "dsl-mean-revert"},
					"appliedVersion": map[string]any{"type": "string", "example": "0.1.0"},
					"latestVersion":  map[string]any{"type": "string", "example": "0.1.1"},
					"isLatest":       map[string]any{"type": "boolean", "example": false},
					"canApplyLatest": map[string]any{"type": "boolean", "example": true},
					"blockedReason":  map[string]any{"type": "string", "nullable": true, "example": "当前实例不是 STOPPED，先停止后才能刷新到最新策略。"},
				},
				"required": []string{"definitionId", "appliedVersion", "latestVersion", "isLatest", "canApplyLatest"},
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
					"binding":      schemaRef("StrategyInstanceBinding"),
					"params":       map[string]any{"type": "object", "additionalProperties": true},
					"status":       map[string]any{"type": "string", "example": "STOPPED"},
					"createdAt":    map[string]any{"type": "string", "format": "date-time"},
					"logs": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
					"definitionSync":     schemaRef("StrategyDefinitionSyncStatus"),
					"runtimeObservation": schemaRef("StrategyRuntimeObservation"),
				},
				"required": []string{"id", "definition", "runtime", "sourceFormat", "startable", "binding", "params", "status", "createdAt", "logs"},
			},
			"StrategyApplyLinkedInstancesResponse": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"definitionId":  map[string]any{"type": "string", "example": "dsl-mean-revert"},
					"latestVersion": map[string]any{"type": "string", "example": "0.1.1"},
					"totalLinked":   map[string]any{"type": "integer", "example": 3},
					"applied":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"alreadyLatest": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"skippedBusy":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"definitionId", "latestVersion", "totalLinked", "applied", "alreadyLatest", "skippedBusy"},
			},
			"StrategyRuntimeObservation": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"actualStatus":      map[string]any{"type": "string", "example": "RUNNING"},
					"activeSymbols":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"lastClosedKlineAt": map[string]any{"type": "string", "format": "date-time", "nullable": true},
					"lastSignalAt":      map[string]any{"type": "string", "format": "date-time", "nullable": true},
					"lastOrderAt":       map[string]any{"type": "string", "format": "date-time", "nullable": true},
					"lastErrorAt":       map[string]any{"type": "string", "format": "date-time", "nullable": true},
					"lastError":         map[string]any{"type": "string", "nullable": true},
					"updatedAt":         map[string]any{"type": "string", "format": "date-time", "nullable": true},
				},
				"required": []string{"actualStatus", "activeSymbols"},
			},
			"StrategyActivityPage": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit":    map[string]any{"type": "integer", "example": 500},
					"offset":   map[string]any{"type": "integer", "example": 0},
					"total":    map[string]any{"type": "integer", "example": 12},
					"returned": map[string]any{"type": "integer", "example": 12},
					"hasMore":  map[string]any{"type": "boolean", "example": false},
				},
				"required": []string{"limit", "offset", "total", "returned", "hasMore"},
			},
			"StrategyLogsResponse": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"instanceId": map[string]any{"type": "string", "example": "js-mean-revert-20260523000000"},
					"logs": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
					"page": schemaRef("StrategyActivityPage"),
				},
				"required": []string{"instanceId", "logs", "page"},
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
					"page": schemaRef("StrategyActivityPage"),
				},
				"required": []string{"instanceId", "entries", "page"},
			},
		},
	}
}
