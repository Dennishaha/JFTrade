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
		},
	}
}
