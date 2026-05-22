package jftradeapi

func buildOpenAPIPaths(genericObject map[string]any) map[string]any {
	return map[string]any{
		"/api/v1/settings/brokers": map[string]any{
			"get": operation(
				"读取 broker 设置",
				"返回当前 broker 集成配置与托管账户列表。",
				[]string{"settings"},
				nil,
				nil,
				map[string]any{"200": jsonResponse("当前 broker 设置", envelopeSchema(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"brokers": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"brokerId":    map[string]any{"type": "string", "example": "futu"},
									"displayName": map[string]any{"type": "string", "example": "Futu OpenD"},
									"integration": schemaRef("BrokerIntegration"),
									"accounts":    map[string]any{"type": "array", "items": schemaRef("ManagedBrokerAccount")},
								},
								"required": []string{"brokerId", "displayName", "integration", "accounts"},
							},
						},
					},
					"required": []string{"brokers"},
				}))},
			),
		},
		"/api/v1/settings/brokers/{brokerId}/integration": map[string]any{
			"put": operation(
				"保存 broker 集成",
				"当前实现主要用于保存 futu 集成配置。",
				[]string{"settings"},
				[]any{pathParameter("brokerId", "Broker 标识", "futu")},
				jsonRequestBody(schemaRef("BrokerIntegrationSaveRequest"), true),
				map[string]any{
					"200": jsonResponse("保存后的集成配置", envelopeSchema(schemaRef("BrokerIntegration"))),
					"400": jsonResponse("请求体错误", envelopeSchema(nil)),
					"500": jsonResponse("保存失败", envelopeSchema(nil)),
				},
			),
		},
		"/api/v1/settings/broker-accounts": map[string]any{
			"post": operation(
				"创建托管账户",
				"创建或覆盖同 scope 的托管账户记录。",
				[]string{"settings"},
				nil,
				jsonRequestBody(schemaRef("ManagedBrokerAccountWriteRequest"), true),
				map[string]any{
					"200": jsonResponse("创建后的托管账户", envelopeSchema(schemaRef("ManagedBrokerAccount"))),
					"400": jsonResponse("请求体错误", envelopeSchema(nil)),
					"500": jsonResponse("保存失败", envelopeSchema(nil)),
				},
			),
		},
		"/api/v1/settings/broker-accounts/{accountRecordId}": map[string]any{
			"put": operation(
				"更新托管账户",
				"按账户记录 ID 更新托管账户。",
				[]string{"settings"},
				[]any{pathParameter("accountRecordId", "托管账户记录 ID", "futu|SIMULATE|123456|HK")},
				jsonRequestBody(schemaRef("ManagedBrokerAccountWriteRequest"), true),
				map[string]any{
					"200": jsonResponse("更新后的托管账户", envelopeSchema(schemaRef("ManagedBrokerAccount"))),
					"400": jsonResponse("请求错误", envelopeSchema(nil)),
					"404": jsonResponse("账户不存在", envelopeSchema(nil)),
					"500": jsonResponse("保存失败", envelopeSchema(nil)),
				},
			),
			"delete": operation(
				"删除托管账户",
				"按账户记录 ID 删除托管账户。",
				[]string{"settings"},
				[]any{pathParameter("accountRecordId", "托管账户记录 ID", "futu|SIMULATE|123456|HK")},
				nil,
				map[string]any{
					"200": jsonResponse("删除结果", envelopeSchema(map[string]any{
						"type": "object",
						"properties": map[string]any{
							"deleted": map[string]any{"type": "boolean", "example": true},
							"id":      map[string]any{"type": "string", "example": "futu|SIMULATE|123456|HK"},
						},
						"required": []string{"deleted", "id"},
					})),
					"400": jsonResponse("请求错误", envelopeSchema(nil)),
					"404": jsonResponse("账户不存在", envelopeSchema(nil)),
				},
			),
		},
		"/api/v1/system/futu-opend": map[string]any{
			"get": operation(
				"OpenD 健康检查",
				"返回 OpenD 连通性、登录状态与程序信息。",
				[]string{"system"},
				nil,
				nil,
				map[string]any{"200": jsonResponse("OpenD 诊断信息", envelopeSchema(genericObject))},
			),
		},
		"/api/v1/system/futu-opend/manual-retry": map[string]any{
			"post": operation(
				"手动重置 OpenD 运行时",
				"触发侧车对 Futu 运行时的重置。",
				[]string{"system"},
				nil,
				nil,
				map[string]any{"200": jsonResponse("重置已接受", envelopeSchema(map[string]any{
					"type":       "object",
					"properties": map[string]any{"accepted": map[string]any{"type": "boolean", "example": true}},
					"required":   []string{"accepted"},
				}))},
			),
		},
		"/api/v1/system/futu-opend/install-guide": map[string]any{
			"get": operation(
				"读取 OpenD 安装指南",
				"返回前端展示的 OpenD 安装指引。",
				[]string{"system"},
				nil,
				nil,
				map[string]any{"200": jsonResponse("安装指南", envelopeSchema(genericObject))},
			),
		},
		"/api/v1/system/status": map[string]any{
			"get": operation(
				"读取系统状态",
				"返回 API、broker 与实时流状态摘要。",
				[]string{"system"},
				nil,
				nil,
				map[string]any{"200": jsonResponse("系统状态", envelopeSchema(genericObject))},
			),
		},
		"/api/v1/market-data/instruments": map[string]any{
			"get": operation(
				"检索行情标的",
				"按关键字查询可用标的。当前实现返回空结果占位。",
				[]string{"market-data"},
				[]any{queryParameter("query", "关键字", false, map[string]any{"type": "string", "example": "00700"})},
				nil,
				map[string]any{"200": jsonResponse("查询结果", envelopeSchema(genericObject))},
			),
		},
		"/api/v1/market-data/subscriptions": map[string]any{
			"get": operation(
				"读取订阅列表",
				"返回当前行情订阅状态。",
				[]string{"market-data"},
				nil,
				nil,
				map[string]any{"200": jsonResponse("订阅列表", envelopeSchema(genericObject))},
			),
			"post": operation(
				"申请行情订阅",
				"按 channel/market/symbol/interval 与 consumerId 注册订阅。",
				[]string{"market-data"},
				nil,
				jsonRequestBody(schemaRef("MarketSubscriptionRequest"), true),
				map[string]any{
					"200": jsonResponse("申请后的订阅列表", envelopeSchema(genericObject)),
					"400": jsonResponse("请求错误", envelopeSchema(nil)),
				},
			),
			"delete": operation(
				"清空行情订阅",
				"按 consumerId 或全量清空当前订阅。",
				[]string{"market-data"},
				[]any{queryParameter("consumerId", "消费者 ID；为空时清空全部", false, map[string]any{"type": "string", "example": "chart-main"})},
				nil,
				map[string]any{"200": jsonResponse("清空后的订阅列表", envelopeSchema(genericObject))},
			),
		},
		"/api/v1/market-data/subscriptions/release": map[string]any{
			"post": operation(
				"释放行情订阅",
				"移除某个 consumerId 对特定订阅的占用。",
				[]string{"market-data"},
				nil,
				jsonRequestBody(schemaRef("MarketSubscriptionRequest"), true),
				map[string]any{
					"200": jsonResponse("释放后的订阅列表", envelopeSchema(genericObject)),
					"400": jsonResponse("请求错误", envelopeSchema(nil)),
				},
			),
		},
		"/api/v1/market-data/subscriptions/heartbeat": map[string]any{
			"post": operation(
				"刷新订阅心跳",
				"按 consumerId 刷新当前订阅的活跃时间。",
				[]string{"market-data"},
				nil,
				jsonRequestBody(schemaRef("MarketSubscriptionHeartbeatRequest"), true),
				map[string]any{
					"200": jsonResponse("刷新后的订阅列表", envelopeSchema(genericObject)),
					"400": jsonResponse("请求错误", envelopeSchema(nil)),
				},
			),
		},
		"/api/v1/market-data/securities/{market}/{symbol}": map[string]any{
			"get": operation(
				"读取证券基础信息与快照扩展",
				"读取证券公共信息与按证券类型拆分的扩展快照字段。",
				[]string{"market-data"},
				[]any{
					pathParameter("market", "市场代码", "HK"),
					pathParameter("symbol", "证券代码", "00700"),
				},
				nil,
				map[string]any{
					"200": jsonResponse("证券详情", envelopeSchema(genericObject)),
					"502": jsonResponse("OpenD 查询失败", envelopeSchema(nil)),
				},
			),
		},
		"/api/v1/market-data/snapshots/{market}/{symbol}": map[string]any{
			"get": operation(
				"读取行情快照",
				"读取最新快照；refresh=true 时会强制直连 OpenD 刷新。",
				[]string{"market-data"},
				[]any{
					pathParameter("market", "市场代码", "HK"),
					pathParameter("symbol", "证券代码", "00700"),
					queryParameter("refresh", "是否绕过缓存强制刷新", false, map[string]any{"type": "boolean", "default": false}),
				},
				nil,
				map[string]any{
					"200": jsonResponse("行情快照", envelopeSchema(genericObject)),
					"502": jsonResponse("OpenD 查询失败", envelopeSchema(nil)),
				},
			),
		},
		"/api/v1/market-data/candles/{market}/{symbol}": map[string]any{
			"get": operation(
				"读取 K 线或逐笔合成",
				"period 支持 tick、1m、5m 等；可用 fromTime/toTime/limit 控制窗口。",
				[]string{"market-data"},
				[]any{
					pathParameter("market", "市场代码", "HK"),
					pathParameter("symbol", "证券代码", "00700"),
					queryParameter("period", "周期，默认 1m", false, map[string]any{"type": "string", "example": "1m"}),
					queryParameter("limit", "返回条数，最大 1000", false, map[string]any{"type": "integer", "default": 200, "minimum": 1, "maximum": 1000}),
					queryParameter("fromTime", "起始时间，RFC3339", false, map[string]any{"type": "string", "format": "date-time"}),
					queryParameter("toTime", "结束时间，RFC3339", false, map[string]any{"type": "string", "format": "date-time"}),
				},
				nil,
				map[string]any{
					"200": jsonResponse("K 线结果", envelopeSchema(genericObject)),
					"502": jsonResponse("OpenD 查询失败", envelopeSchema(nil)),
				},
			),
		},
		"/api/v1/brokers/{brokerId}/runtime": map[string]any{
			"get": operation(
				"读取 broker 运行时",
				"返回 broker 运行态与最近错误信息。",
				[]string{"broker"},
				[]any{pathParameter("brokerId", "Broker 标识", "futu")},
				nil,
				map[string]any{"200": jsonResponse("broker 运行时", envelopeSchema(genericObject))},
			),
		},
		"/api/v1/brokers/{brokerId}/funds": map[string]any{
			"get": operation("读取资金摘要", "返回账户资金摘要。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, nil, map[string]any{"200": jsonResponse("资金摘要", envelopeSchema(genericObject))}),
		},
		"/api/v1/brokers/{brokerId}/positions": map[string]any{
			"get": operation("读取持仓", "返回账户持仓。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, nil, map[string]any{"200": jsonResponse("持仓", envelopeSchema(genericObject))}),
		},
		"/api/v1/brokers/{brokerId}/orders": map[string]any{
			"get": operation("读取订单", "返回订单列表。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, nil, map[string]any{"200": jsonResponse("订单列表", envelopeSchema(genericObject))}),
		},
		"/api/v1/brokers/{brokerId}/cash-flows": map[string]any{
			"get": operation("读取资金流水", "返回现金流列表。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, nil, map[string]any{"200": jsonResponse("资金流水", envelopeSchema(genericObject))}),
		},
		"/api/v1/brokers/{brokerId}/order-fees": map[string]any{
			"get": operation("读取订单费用", "返回订单费用列表。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, nil, map[string]any{"200": jsonResponse("订单费用", envelopeSchema(genericObject))}),
		},
		"/api/v1/plugins": map[string]any{
			"get": operation("读取插件列表", "返回插件安装目标目录与插件列表。", []string{"strategy"}, nil, nil, map[string]any{"200": jsonResponse("插件列表", envelopeSchema(genericObject))}),
		},
		"/api/v1/strategies": map[string]any{
			"get": operation("读取策略列表", "返回当前策略实例列表。", []string{"strategy"}, nil, nil, map[string]any{"200": jsonResponse("策略列表", envelopeSchema(map[string]any{"type": "array", "items": genericObject}))}),
		},
		"/api/v1/strategies/{instanceId}/logs": map[string]any{
			"get": operation(
				"读取策略日志",
				"返回策略实例日志列表。",
				[]string{"strategy"},
				[]any{pathParameter("instanceId", "策略实例 ID", "demo-strategy")},
				nil,
				map[string]any{"200": jsonResponse("策略日志", envelopeSchema(genericObject))},
			),
		},
		"/api/v1/strategies/{instanceId}/audit": map[string]any{
			"get": operation(
				"读取策略审计日志",
				"返回策略实例审计记录。",
				[]string{"strategy"},
				[]any{pathParameter("instanceId", "策略实例 ID", "demo-strategy")},
				nil,
				map[string]any{"200": jsonResponse("策略审计日志", envelopeSchema(genericObject))},
			),
		},
		"/api/v1/ws/live": map[string]any{
			"get": operation(
				"连接实时行情 WebSocket",
				"升级为 WebSocket 后持续返回 heartbeat、实时行情事件与系统通知事件。Swagger UI 仅展示连接说明，不直接建立 WebSocket 交互。",
				[]string{"streaming"},
				nil,
				nil,
				map[string]any{
					"101": map[string]any{"description": "协议已升级为 WebSocket"},
					"503": jsonResponse("连接数已达上限", envelopeSchema(nil)),
				},
			),
		},
		"/api/v1/stream/live": map[string]any{
			"get": streamOperation("连接实时 SSE 流", "返回 text/event-stream 格式的实时事件流。"),
		},
		"/api/v1/streams/console": map[string]any{
			"get": streamOperation("连接控制台 SSE 流", "与 /api/v1/stream/live 指向同一 SSE 处理器。"),
		},
	}
}
