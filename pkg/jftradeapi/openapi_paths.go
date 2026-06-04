package jftradeapi

func buildOpenAPIPaths(genericObject map[string]any) map[string]any {
	return map[string]any{
		"/api/v1/settings/ui": map[string]any{
			"get": operation(
				"读取 UI 颜色配置",
				"返回市场涨跌色配置，保存在 settings.json 中。",
				[]string{"settings"},
				nil,
				nil,
				map[string]any{"200": jsonResponse("当前 UI 配置", envelopeSchema(schemaRef("UIAppearanceSettingsResponse")))},
			),
			"put": operation(
				"保存 UI 颜色配置",
				"更新市场涨跌色并写入 settings.json。",
				[]string{"settings"},
				nil,
				jsonRequestBody(schemaRef("UIAppearanceSettingsWriteRequest"), true),
				map[string]any{
					"200": jsonResponse("保存后的 UI 配置", envelopeSchema(schemaRef("UIAppearanceSettingsResponse"))),
					"400": jsonResponse("请求体错误", envelopeSchema(nil)),
					"500": jsonResponse("保存失败", envelopeSchema(nil)),
				},
			),
		},
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
		"/api/v1/market-data/depth/{market}/{symbol}": map[string]any{
			"get": operation(
				"读取盘口深度",
				"返回买卖盘口深度数据（bid/ask），num 控制档数（1-50，默认 10）。",
				[]string{"market-data"},
				[]any{
					pathParameter("market", "市场代码", "HK"),
					pathParameter("symbol", "证券代码", "00700"),
					queryParameter("num", "请求档数，默认 10，最大 50", false, map[string]any{"type": "integer", "default": 10, "minimum": 1, "maximum": 50}),
				},
				nil,
				map[string]any{
					"200": jsonResponse("盘口深度数据", envelopeSchema(genericObject)),
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
		"/api/v1/brokers/{brokerId}/fills": map[string]any{
			"get": operation("读取成交", "返回成交列表。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, nil, map[string]any{"200": jsonResponse("成交列表", envelopeSchema(genericObject))}),
		},
		"/api/v1/brokers/{brokerId}/cash-flows": map[string]any{
			"get": operation("读取资金流水", "返回现金流列表。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, nil, map[string]any{"200": jsonResponse("资金流水", envelopeSchema(genericObject))}),
		},
		"/api/v1/brokers/{brokerId}/order-fees": map[string]any{
			"get": operation("读取订单费用", "返回订单费用列表。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, nil, map[string]any{"200": jsonResponse("订单费用", envelopeSchema(genericObject))}),
		},
		"/api/v1/brokers/{brokerId}/margin-ratios": map[string]any{
			"get": operation("读取融资融券数据", "返回标的融资融券参数。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, nil, map[string]any{"200": jsonResponse("融资融券数据", envelopeSchema(genericObject))}),
		},
		"/api/v1/brokers/{brokerId}/max-trade-qtys": map[string]any{
			"get": operation("读取最大可交易数量", "返回给定下单参数下的最大可交易数量。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, nil, map[string]any{"200": jsonResponse("最大可交易数量", envelopeSchema(genericObject))}),
		},
		"/api/v1/brokers/{brokerId}/quote": map[string]any{
			"get": operation("读取实时行情", "返回证券基本报价。需要 query 参数 symbol。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, nil, map[string]any{"200": jsonResponse("行情数据", envelopeSchema(genericObject))}),
		},
		"/api/v1/brokers/{brokerId}/klines": map[string]any{
			"get": operation("读取K线", "返回历史K线数据。需要 query 参数 symbol、period。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, nil, map[string]any{"200": jsonResponse("K线数据", envelopeSchema(genericObject))}),
		},
		"/api/v1/brokers/{brokerId}/securities": map[string]any{
			"get": operation("读取证券快照", "返回证券快照数据（基础行情+扩展信息）。需要 query 参数 symbol。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, nil, map[string]any{"200": jsonResponse("证券快照", envelopeSchema(genericObject))}),
		},
		"/api/v1/brokers/{brokerId}/unlock": map[string]any{
			"post": operation("解锁/锁定交易", "解锁或锁定交易会话。请求体包含 unlock 布尔和 passwordMd5 字段。", []string{"broker"}, []any{pathParameter("brokerId", "Broker 标识", "futu")}, jsonRequestBody(genericObject, true), map[string]any{"200": jsonResponse("解锁结果", envelopeSchema(genericObject))}),
		},
		"/api/v1/plugins": map[string]any{
			"get": operation("读取插件列表", "返回插件安装目标目录与插件列表。", []string{"strategy"}, nil, nil, map[string]any{"200": jsonResponse("插件列表", envelopeSchema(genericObject))}),
		},
		"/api/v1/plugins/operations/{operationId}": map[string]any{
			"get": operation("读取插件操作状态", "返回插件安装或卸载操作的最新状态。", []string{"strategy"}, []any{pathParameter("operationId", "插件操作 ID", "demo-plugin-20260522")}, nil, map[string]any{"200": jsonResponse("插件操作状态", envelopeSchema(genericObject)), "404": jsonResponse("操作不存在", envelopeSchema(nil))}),
		},
		"/api/v1/plugins/{pluginId}/install": map[string]any{
			"post": operation("安装插件", "将插件元数据标记为已安装。当前实现先落控制面状态。", []string{"strategy"}, []any{pathParameter("pluginId", "插件 ID", "demo-plugin")}, nil, map[string]any{"200": jsonResponse("安装结果", envelopeSchema(genericObject)), "404": jsonResponse("插件不存在", envelopeSchema(nil))}),
		},
		"/api/v1/plugins/{pluginId}/uninstall": map[string]any{
			"post": operation("卸载插件", "将插件元数据标记为未安装。当前实现先落控制面状态。", []string{"strategy"}, []any{pathParameter("pluginId", "插件 ID", "demo-plugin")}, nil, map[string]any{"200": jsonResponse("卸载结果", envelopeSchema(genericObject)), "404": jsonResponse("插件不存在", envelopeSchema(nil))}),
		},
		"/api/v1/plugins/{pluginId}/uninstall-guidance": map[string]any{
			"get": operation("读取插件卸载指引", "返回当前插件的本地卸载路径与命令。", []string{"strategy"}, []any{pathParameter("pluginId", "插件 ID", "demo-plugin")}, nil, map[string]any{"200": jsonResponse("卸载指引", envelopeSchema(genericObject)), "404": jsonResponse("插件不存在", envelopeSchema(nil))}),
		},
		"/api/v1/strategy-definitions": map[string]any{
			"get":  operation("读取策略定义列表", "返回当前 DSL 策略定义列表。", []string{"strategy"}, nil, nil, map[string]any{"200": jsonResponse("策略定义列表", envelopeSchema(map[string]any{"type": "array", "items": schemaRef("StrategyDefinition")}))}),
			"post": operation("创建策略定义", "创建一个新的 DSL 策略定义；definitionId 由服务端生成 GUID，若请求体携带 id 会被忽略。", []string{"strategy"}, nil, jsonRequestBody(schemaRef("StrategyDefinitionWriteRequest"), true), map[string]any{"200": jsonResponse("创建后的策略定义", envelopeSchema(schemaRef("StrategyDefinition"))), "400": jsonResponse("请求错误", envelopeSchema(nil))}),
		},
		"/api/v1/strategy-definitions/{definitionId}": map[string]any{
			"get":    operation("读取策略定义", "按 definitionId 返回 DSL 策略定义。", []string{"strategy"}, []any{pathParameter("definitionId", "策略定义 ID", "dsl-mean-revert")}, nil, map[string]any{"200": jsonResponse("策略定义", envelopeSchema(schemaRef("StrategyDefinition"))), "404": jsonResponse("策略定义不存在", envelopeSchema(nil))}),
			"put":    operation("更新策略定义", "按 definitionId 更新 DSL 策略定义。", []string{"strategy"}, []any{pathParameter("definitionId", "策略定义 ID", "dsl-mean-revert")}, jsonRequestBody(schemaRef("StrategyDefinitionWriteRequest"), true), map[string]any{"200": jsonResponse("更新后的策略定义", envelopeSchema(schemaRef("StrategyDefinition"))), "400": jsonResponse("请求错误", envelopeSchema(nil)), "404": jsonResponse("策略定义不存在", envelopeSchema(nil))}),
			"delete": operation("删除策略定义", "软删除策略定义；若仍有实例关联该定义，则必须先删除相关实例。", []string{"strategy"}, []any{pathParameter("definitionId", "策略定义 ID", "dsl-mean-revert")}, nil, map[string]any{"200": jsonResponse("已删除的策略定义", envelopeSchema(schemaRef("StrategyDefinition"))), "400": jsonResponse("仍有关联实例，无法删除", envelopeSchema(nil)), "404": jsonResponse("策略定义不存在", envelopeSchema(nil))}),
		},
		"/api/v1/strategy-definitions/{definitionId}/instantiate": map[string]any{
			"post": operation("从定义创建策略实例", "基于保存的 DSL 策略定义创建一个运行时实例，并携带实例级标的、周期、账号与执行模式绑定。", []string{"strategy"}, []any{pathParameter("definitionId", "策略定义 ID", "dsl-mean-revert")}, jsonRequestBody(schemaRef("StrategyInstanceBinding"), false), map[string]any{"200": jsonResponse("策略实例", envelopeSchema(schemaRef("StrategyInstance"))), "400": jsonResponse("脚本校验失败", envelopeSchema(nil)), "404": jsonResponse("策略定义不存在", envelopeSchema(nil))}),
		},
		"/api/v1/strategy-definitions/{definitionId}/apply-linked-instances": map[string]any{
			"post": operation("将最新定义应用到关联实例", "把最新保存的策略定义批量刷新到所有关联的 STOPPED 实例；运行中或暂停中的实例会被跳过。", []string{"strategy"}, []any{pathParameter("definitionId", "策略定义 ID", "dsl-mean-revert")}, nil, map[string]any{"200": jsonResponse("批量应用结果", envelopeSchema(schemaRef("StrategyApplyLinkedInstancesResponse"))), "404": jsonResponse("策略定义不存在", envelopeSchema(nil)), "500": jsonResponse("批量应用失败", envelopeSchema(nil))}),
		},
		"/api/v1/strategies": map[string]any{
			"get": operation("读取策略列表", "返回当前策略实例列表。", []string{"strategy"}, nil, nil, map[string]any{"200": jsonResponse("策略列表", envelopeSchema(map[string]any{"type": "array", "items": schemaRef("StrategyInstance")}))}),
		},
		"/api/v1/strategies/{instanceId}": map[string]any{
			"put":    operation("更新策略实例绑定", "更新策略实例的标的、周期、账号与执行模式绑定。实例需处于 STOPPED。", []string{"strategy"}, []any{pathParameter("instanceId", "策略实例 ID", "dsl-mean-revert-20260523")}, jsonRequestBody(schemaRef("StrategyInstanceBinding"), true), map[string]any{"200": jsonResponse("策略实例", envelopeSchema(schemaRef("StrategyInstance"))), "400": jsonResponse("实例当前不可修改", envelopeSchema(nil)), "404": jsonResponse("策略实例不存在", envelopeSchema(nil))}),
			"delete": operation("删除策略实例", "删除已停止的策略实例。", []string{"strategy"}, []any{pathParameter("instanceId", "策略实例 ID", "dsl-mean-revert-20260523")}, nil, map[string]any{"200": jsonResponse("已删除的策略实例", envelopeSchema(schemaRef("StrategyInstance"))), "400": jsonResponse("实例当前不可删除", envelopeSchema(nil)), "404": jsonResponse("策略实例不存在", envelopeSchema(nil))}),
		},
		"/api/v1/strategies/{instanceId}/start": map[string]any{
			"post": operation("启动策略实例", "启动绑定标的的真实 DSL 运行时，将实例切换到 RUNNING，并按执行模式发通知或提交券商订单。", []string{"strategy"}, []any{pathParameter("instanceId", "策略实例 ID", "dsl-mean-revert-20260523")}, nil, map[string]any{"200": jsonResponse("策略实例", envelopeSchema(schemaRef("StrategyInstance"))), "400": jsonResponse("实例当前不可启动", envelopeSchema(nil)), "404": jsonResponse("策略实例不存在", envelopeSchema(nil))}),
		},
		"/api/v1/strategies/{instanceId}/pause": map[string]any{
			"post": operation("暂停策略实例", "停止内存中的策略运行时，并将实例状态切换到 PAUSED。", []string{"strategy"}, []any{pathParameter("instanceId", "策略实例 ID", "dsl-mean-revert-20260523")}, nil, map[string]any{"200": jsonResponse("策略实例", envelopeSchema(schemaRef("StrategyInstance"))), "404": jsonResponse("策略实例不存在", envelopeSchema(nil))}),
		},
		"/api/v1/strategies/{instanceId}/stop": map[string]any{
			"post": operation("停止策略实例", "停止内存中的策略运行时，并将实例状态切换到 STOPPED。", []string{"strategy"}, []any{pathParameter("instanceId", "策略实例 ID", "dsl-mean-revert-20260523")}, nil, map[string]any{"200": jsonResponse("策略实例", envelopeSchema(schemaRef("StrategyInstance"))), "404": jsonResponse("策略实例不存在", envelopeSchema(nil))}),
		},
		"/api/v1/strategies/{instanceId}/refresh-definition": map[string]any{
			"post": operation("刷新实例到最新策略定义", "把单个 STOPPED 策略实例的脚本快照刷新到当前定义的最新版本。", []string{"strategy"}, []any{pathParameter("instanceId", "策略实例 ID", "dsl-mean-revert-20260523")}, nil, map[string]any{"200": jsonResponse("刷新后的策略实例", envelopeSchema(schemaRef("StrategyInstance"))), "400": jsonResponse("实例当前不可刷新", envelopeSchema(nil)), "404": jsonResponse("策略实例或定义不存在", envelopeSchema(nil))}),
		},
		"/api/v1/strategies/{instanceId}/logs": map[string]any{
			"get": operation(
				"读取策略日志",
				"返回策略实例日志列表，支持 limit、offset、level、fromTime、toTime 过滤。",
				[]string{"strategy"},
				[]any{
					pathParameter("instanceId", "策略实例 ID", "demo-strategy"),
					queryParameter("limit", "分页大小，默认 500，最大 5000", false, map[string]any{"type": "integer", "default": 500, "minimum": 1, "maximum": 5000}),
					queryParameter("offset", "分页偏移，默认 0", false, map[string]any{"type": "integer", "default": 0, "minimum": 0}),
					queryParameter("level", "日志级别过滤，支持 info、warning、error", false, map[string]any{"type": "string", "example": "error"}),
					queryParameter("fromTime", "起始时间，RFC3339", false, map[string]any{"type": "string", "format": "date-time"}),
					queryParameter("toTime", "结束时间，RFC3339", false, map[string]any{"type": "string", "format": "date-time"}),
				},
				nil,
				map[string]any{"200": jsonResponse("策略日志", envelopeSchema(schemaRef("StrategyLogsResponse")))},
			),
		},
		"/api/v1/strategies/{instanceId}/audit": map[string]any{
			"get": operation(
				"读取策略审计日志",
				"返回策略实例审计记录，支持 limit、offset、kind、fromTime、toTime 过滤。",
				[]string{"strategy"},
				[]any{
					pathParameter("instanceId", "策略实例 ID", "demo-strategy"),
					queryParameter("limit", "分页大小，默认 500，最大 5000", false, map[string]any{"type": "integer", "default": 500, "minimum": 1, "maximum": 5000}),
					queryParameter("offset", "分页偏移，默认 0", false, map[string]any{"type": "integer", "default": 0, "minimum": 0}),
					queryParameter("kind", "审计类型过滤，例如 started、runtime_exited", false, map[string]any{"type": "string", "example": "runtime_exited"}),
					queryParameter("fromTime", "起始时间，RFC3339", false, map[string]any{"type": "string", "format": "date-time"}),
					queryParameter("toTime", "结束时间，RFC3339", false, map[string]any{"type": "string", "format": "date-time"}),
				},
				nil,
				map[string]any{"200": jsonResponse("策略审计日志", envelopeSchema(schemaRef("StrategyAuditResponse")))},
			),
		},
		"/api/v1/ws/live": map[string]any{
			"get": streamOperation("连接实时 WebSocket", "升级到唯一实时 WebSocket 连接，返回 heartbeat、实时行情、系统通知、console refresh、security details 与 depth 事件。"),
		},
	}
}
