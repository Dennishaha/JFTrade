//nolint:unused // These annotation-only stubs are consumed by swag during go generate.
package servercore

import "github.com/jftrade/jftrade-main/internal/datamanagement"

type dataCleanupPreviewRequest = datamanagement.CleanupPreviewRequest
type dataCleanupExecuteRequest = datamanagement.CleanupExecuteRequest
type databaseCompactRequest = datamanagement.CompactRequest
type databaseRebuildRequest = datamanagement.RebuildRequest

// documentDataMigrationRoutes godoc
// @Summary Database compatibility status and rebuild scheduling
// @Description 已废弃的旧名别名，请改用 /api/v1/settings/data-management 下的对应端点。
// @Tags settings
// @Produce json
// @Success 200 {object} envelope
// @Deprecated
// @Router /api/v1/settings/data-migration/databases [get]
// @Router /api/v1/settings/data-migration/databases/rebuild [post]
func documentDataMigrationRoutes() string { return "data-migration" }

// documentDataManagementOverview godoc
// @Summary Database storage usage and cleanup opportunities
// @Tags settings
// @Produce json
// @Param summaryOnly query bool false "Return only database status without SQLite storage or cleanup statistics"
// @Param databaseId query string false "Return one database overview for incremental loading"
// @Success 200 {object} envelope
// @Router /api/v1/settings/data-management/databases [get]
func documentDataManagementOverview() string { return "data-management-overview" }

// documentDataCleanupPreview godoc
// @Summary Preview an exact database cleanup candidate set
// @Tags settings
// @Accept json
// @Produce json
// @Param request body dataCleanupPreviewRequest true "Cleanup preview request"
// @Success 200 {object} envelope
// @Router /api/v1/settings/data-management/cleanup/preview [post]
func documentDataCleanupPreview() string { return "data-cleanup-preview" }

// documentDataCleanupExecute godoc
// @Summary Execute a previously previewed database cleanup
// @Tags settings
// @Accept json
// @Produce json
// @Param request body dataCleanupExecuteRequest true "Cleanup execution request"
// @Success 200 {object} envelope
// @Router /api/v1/settings/data-management/cleanup/execute [post]
func documentDataCleanupExecute() string { return "data-cleanup-execute" }

// documentDatabaseCompact godoc
// @Summary Checkpoint and compact one database
// @Tags settings
// @Accept json
// @Produce json
// @Param databaseId path string true "Database ID"
// @Param request body databaseCompactRequest true "Compaction confirmation"
// @Success 200 {object} envelope
// @Router /api/v1/settings/data-management/databases/{databaseId}/compact [post]
func documentDatabaseCompact() string { return "database-compact" }

// documentDatabaseRebuild godoc
// @Summary Schedule a database rebuild on next startup
// @Tags settings
// @Accept json
// @Produce json
// @Param request body databaseRebuildRequest true "Database rebuild request"
// @Success 200 {object} envelope
// @Router /api/v1/settings/data-management/databases/rebuild [post]
func documentDatabaseRebuild() string { return "database-rebuild" }

// documentAssistantCatalogRoutes godoc
// @Summary ADK catalog and provider management routes
// @Tags adk
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/adk/tools [get]
// @Router /api/v1/adk/agent-templates [get]
// @Router /api/v1/adk/audit [get]
// @Router /api/v1/adk/metrics [get]
// @Router /api/v1/adk/providers [post]
// @Router /api/v1/adk/providers/{providerId} [put]
// @Router /api/v1/adk/providers/{providerId} [delete]
// @Router /api/v1/adk/providers/{providerId}/default [post]
// @Router /api/v1/adk/providers/{providerId}/test [post]
// @Router /api/v1/adk/agents [post]
// @Router /api/v1/adk/agents/{agentId} [put]
// @Router /api/v1/adk/agents/{agentId} [delete]
func documentAssistantCatalogRoutes() string { return "assistant-catalog" }

// documentAssistantTaskMemoryRoutes godoc
// @Summary ADK task and memory routes
// @Description ADK tasks include lightweight agent tasks and goal workflow step projections.
// @Tags adk
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/adk/tasks [get]
// @Router /api/v1/adk/tasks [post]
// @Router /api/v1/adk/tasks/{taskId} [get]
// @Router /api/v1/adk/tasks/{taskId} [put]
// @Router /api/v1/adk/tasks/{taskId} [delete]
// @Router /api/v1/adk/memory [get]
// @Router /api/v1/adk/memory [post]
// @Router /api/v1/adk/memory/{memoryId} [delete]
func documentAssistantTaskMemoryRoutes() string { return "assistant-task-memory" }

// documentAssistantSessionRunRoutes godoc
// @Summary ADK session and run routes
// @Tags adk
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/adk/sessions [post]
// @Router /api/v1/adk/sessions/{sessionId} [get]
// @Router /api/v1/adk/sessions/{sessionId} [put]
// @Router /api/v1/adk/sessions/{sessionId} [delete]
// @Router /api/v1/adk/sessions/{sessionId}/composer-state [patch]
// @Router /api/v1/adk/sessions/{sessionId}/context [get]
// @Router /api/v1/adk/sessions/{sessionId}/context/compact [post]
// @Router /api/v1/adk/runs/{runId} [get]
// @Router /api/v1/adk/runs/{runId}/stream [get]
// @Router /api/v1/adk/runs/{runId}/cancel [post]
// @Router /api/v1/adk/runs/{runId}/pause [post]
// @Router /api/v1/adk/runs/{runId}/resume [post]
// @Router /api/v1/adk/runs/{runId}/objective [patch]
// @Router /api/v1/adk/streams/{streamId} [get]
func documentAssistantSessionRunRoutes() string { return "assistant-session-run" }

// documentAssistantChatApprovalSkillRoutes godoc
// @Summary ADK chat, approval, and skill routes
// @Tags adk
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/adk/chat [post]
// @Router /api/v1/adk/chat/stream [post]
// @Router /api/v1/adk/approvals [get]
// @Router /api/v1/adk/approvals/{approvalId}/approve [post]
// @Router /api/v1/adk/approvals/{approvalId}/deny [post]
// @Router /api/v1/adk/runs/{runId}/input-response [post]
// @Router /api/v1/adk/skills [get]
// @Router /api/v1/adk/skills [post]
// @Router /api/v1/adk/skills/{skillId} [delete]
func documentAssistantChatApprovalSkillRoutes() string { return "assistant-chat-approval-skill" }

// documentAssistantSkillUpdateRemovedRoute godoc
// @Summary 更新 ADK 技能（已废弃）
// @Description 该端点是兼容 tombstone，恒返 410 Gone；请改为在 agent 上直接绑定技能。
// @Tags adk
// @Produce json
// @Failure 410 {object} envelope
// @Deprecated
// @Router /api/v1/adk/skills/{skillId} [put]
func documentAssistantSkillUpdateRemovedRoute() string { return "assistant-skill-update-removed" }

// documentAssistantOptimizationRoutes godoc
// @Summary ADK optimization task routes
// @Tags adk
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/adk/optimization-tasks [get]
// @Router /api/v1/adk/optimization-tasks/{taskId} [get]
// @Router /api/v1/adk/optimization-tasks/{taskId}/cancel [post]
func documentAssistantOptimizationRoutes() string { return "assistant-optimization" }

// documentAssistantWorkflowRoutes godoc
// @Summary ADK workflow definition, trigger, and trigger log routes
// @Tags adk
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/adk/workflows [get]
// @Router /api/v1/adk/workflows [post]
// @Router /api/v1/adk/workflows/{workflowId} [get]
// @Router /api/v1/adk/workflows/{workflowId} [put]
// @Router /api/v1/adk/workflows/{workflowId} [delete]
// @Router /api/v1/adk/workflows/{workflowId}/run [post]
// @Router /api/v1/adk/workflows/{workflowId}/triggers [get]
// @Router /api/v1/adk/workflows/{workflowId}/triggers [post]
// @Router /api/v1/adk/workflows/{workflowId}/triggers/{triggerId} [put]
// @Router /api/v1/adk/workflows/{workflowId}/triggers/{triggerId} [delete]
// @Router /api/v1/adk/workflow-triggers/{triggerId}/run [post]
// @Router /api/v1/adk/workflow-trigger-logs [get]
// @Router /api/v1/adk/workflow-webhooks/{triggerId} [post]
func documentAssistantWorkflowRoutes() string { return "assistant-workflow" }

// documentBacktestSyncTaskRoutes godoc
// @Summary Backtest historical data sync task routes
// @Tags backtest
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/backtests/sync/{taskId} [get]
// @Router /api/v1/backtests/sync/{taskId} [delete]
func documentBacktestSyncTaskRoutes() string { return "backtest-sync-task" }

// documentMarketUtilityRoutes godoc
// @Summary Market data utility routes
// @Tags market-data
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/market-data/markets [get]
// @Router /api/v1/market-data/subscriptions [get]
// @Router /api/v1/market-data/instruments/normalize [post]
func documentMarketUtilityRoutes() string { return "market-utility" }

// documentPluginRoutes godoc
// @Summary Plugin catalog and operation routes
// @Tags plugins
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/plugins [get]
// @Router /api/v1/plugins/operations/{operationId} [get]
// @Router /api/v1/plugins/{pluginId}/install [post]
// @Router /api/v1/plugins/{pluginId}/uninstall [post]
// @Router /api/v1/plugins/{pluginId}/uninstall-guidance [get]
func documentPluginRoutes() string { return "plugin" }

// documentPortfolioCashBalancesRoute godoc
// @Summary 读取 portfolio 现金余额
// @Tags portfolio
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Success 200 {object} envelope{data=trading.PortfolioCashBalancesResponse}
// @Router /api/v1/portfolio/{brokerId}/cash-balances [get]
func documentPortfolioCashBalancesRoute() string { return "portfolio-cash-balances" }

// documentPortfolioPositionsRoute godoc
// @Summary 读取 portfolio 持仓
// @Tags portfolio
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Success 200 {object} envelope{data=trading.PortfolioPositionsResponse}
// @Router /api/v1/portfolio/{brokerId}/positions [get]
func documentPortfolioPositionsRoute() string { return "portfolio-positions" }

// documentPortfolioCashReconciliationRoute godoc
// @Summary 读取 portfolio 现金对账
// @Tags portfolio
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Success 200 {object} envelope{data=trading.PortfolioCashReconciliationResponse}
// @Router /api/v1/portfolio/{brokerId}/cash-reconciliation [get]
func documentPortfolioCashReconciliationRoute() string { return "portfolio-cash-reconciliation" }

// documentPortfolioReconciliationRoute godoc
// @Summary 读取 portfolio 持仓对账
// @Tags portfolio
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Success 200 {object} envelope{data=trading.PortfolioReconciliationResponse}
// @Router /api/v1/portfolio/{brokerId}/reconciliation [get]
func documentPortfolioReconciliationRoute() string { return "portfolio-reconciliation" }

// documentBrokerFundsRoute godoc
// @Summary 读取券商资金
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Success 200 {object} envelope{data=trading.BrokerFundsResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/funds [get]
func documentBrokerFundsRoute() string { return "broker-funds" }

// documentBrokerPositionsRoute godoc
// @Summary 读取券商持仓
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Success 200 {object} envelope{data=trading.BrokerPositionsResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/positions [get]
func documentBrokerPositionsRoute() string { return "broker-positions" }

// documentBrokerOrdersRoute godoc
// @Summary 读取券商订单
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param scope query string false "CURRENT 或 HISTORY"
// @Param symbol query string false "证券代码"
// @Param startTime query string false "历史查询起始时间"
// @Param endTime query string false "历史查询结束时间"
// @Param status query []string false "订单状态"
// @Param statuses query []string false "订单状态，逗号分隔或重复参数"
// @Success 200 {object} envelope{data=trading.BrokerOrdersResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/orders [get]
func documentBrokerOrdersRoute() string { return "broker-orders" }

// documentBrokerFillsRoute godoc
// @Summary 读取券商成交
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param scope query string false "CURRENT 或 HISTORY"
// @Param symbol query string false "证券代码"
// @Param startTime query string false "历史查询起始时间"
// @Param endTime query string false "历史查询结束时间"
// @Success 200 {object} envelope{data=trading.BrokerFillsResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/fills [get]
func documentBrokerFillsRoute() string { return "broker-fills" }

// documentBrokerCashFlowsRoute godoc
// @Summary 读取券商资金流水
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param clearingDate query string true "清算日期"
// @Param direction query string false "方向"
// @Success 200 {object} envelope{data=trading.BrokerCashFlowsResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/cash-flows [get]
func documentBrokerCashFlowsRoute() string { return "broker-cash-flows" }

// documentBrokerOrderFeesRoute godoc
// @Summary 读取券商订单费用
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param orderIdEx query []string true "外部订单号"
// @Param orderIdExList query []string false "外部订单号列表"
// @Success 200 {object} envelope{data=trading.BrokerOrderFeesResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/order-fees [get]
func documentBrokerOrderFeesRoute() string { return "broker-order-fees" }

// documentBrokerMarginRatiosRoute godoc
// @Summary 读取券商融资融券比例
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param symbol query []string true "证券代码"
// @Param symbols query []string false "证券代码列表"
// @Success 200 {object} envelope{data=trading.BrokerMarginRatiosResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/margin-ratios [get]
func documentBrokerMarginRatiosRoute() string { return "broker-margin-ratios" }

// documentBrokerMaxTradeQuantityRoute godoc
// @Summary 读取券商最大可交易数量
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param symbol query string true "证券代码"
// @Param orderType query string true "订单类型"
// @Param price query number true "价格"
// @Param orderIdEx query string false "订单扩展 ID"
// @Param adjustSideAndLimit query number false "调整系数"
// @Param session query string false "交易时段"
// @Param positionId query int false "持仓 ID"
// @Success 200 {object} envelope{data=trading.BrokerMaxTradeQuantityResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/max-trade-qtys [get]
func documentBrokerMaxTradeQuantityRoute() string { return "broker-max-trade-quantity" }

// documentBrokerQuoteRoute godoc
// @Summary 读取券商行情
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param symbol query []string true "证券代码"
// @Param symbols query []string false "证券代码列表"
// @Success 200 {object} envelope{data=trading.BrokerQuoteResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/quote [get]
func documentBrokerQuoteRoute() string { return "broker-quote" }

// documentBrokerKLinesRoute godoc
// @Summary 读取券商 K 线
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param symbol query string true "证券代码"
// @Param period query string false "K 线周期，默认 1d"
// @Param fromTime query string false "起始时间"
// @Param toTime query string false "结束时间"
// @Param limit query int false "返回条数"
// @Success 200 {object} envelope{data=trading.BrokerKLinesResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/klines [get]
func documentBrokerKLinesRoute() string { return "broker-klines" }

// documentBrokerSecuritiesRoute godoc
// @Summary 读取券商证券快照
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param symbol query []string true "证券代码"
// @Param symbols query []string false "证券代码列表"
// @Success 200 {object} envelope{data=trading.BrokerSecuritiesResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/securities [get]
func documentBrokerSecuritiesRoute() string { return "broker-securities" }

// documentBrokerRuntimeRoute godoc
// @Summary 读取券商运行时状态
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Success 200 {object} envelope{data=trading.BrokerRuntimeResponse}
// @Failure 404 {object} envelope
// @Router /api/v1/brokers/{brokerId}/runtime [get]
func documentBrokerRuntimeRoute() string { return "broker-runtime" }

// documentBrokerPlaceOrderRoute godoc
// @Summary 券商下单
// @Tags broker
// @Accept json
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param request body trading.PlaceOrderRequest true "下单请求"
// @Success 200 {object} envelope{data=trading.BrokerPlaceOrderResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/orders [post]
func documentBrokerPlaceOrderRoute() string { return "broker-place-order" }

// documentBrokerCancelOrdersRoute godoc
// @Summary 券商批量撤单
// @Tags broker
// @Accept json
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param request body trading.CancelOrdersRequest true "撤单请求"
// @Success 200 {object} envelope{data=trading.BrokerCancelOrdersResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/orders [delete]
func documentBrokerCancelOrdersRoute() string { return "broker-cancel-orders" }

// documentBrokerUnlockTradeRoute godoc
// @Summary 券商交易解锁
// @Tags broker
// @Accept json
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Param request body trading.UnlockTradeRequest true "解锁请求"
// @Success 200 {object} envelope{data=trading.BrokerUnlockTradeResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/unlock [post]
func documentBrokerUnlockTradeRoute() string { return "broker-unlock-trade" }

// documentExecutionOrdersRoute godoc
// @Summary 读取执行订单
// @Tags execution
// @Produce json
// @Param scope query string false "ACTIVE 表示仅活动订单"
// @Param brokerId query string false "Broker 标识"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场"
// @Success 200 {object} envelope{data=executionOrdersResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/execution/orders [get]
func documentExecutionOrdersRoute() string { return "execution-orders" }

// documentExecutionOrderDetailsRoute godoc
// @Summary 读取单笔执行订单及最近事件
// @Tags execution
// @Produce json
// @Param internalOrderId path string true "内部订单 ID"
// @Success 200 {object} envelope{data=executionOrderDetailsResponse}
// @Failure 404 {object} envelope
// @Router /api/v1/execution/orders/{internalOrderId} [get]
func documentExecutionOrderDetailsRoute() string { return "execution-order-details" }

// documentExecutionPlaceRoute godoc
// @Summary 提交执行订单
// @Tags execution
// @Accept json
// @Produce json
// @Param request body executionPlaceOrderRequest true "执行订单"
// @Success 200 {object} envelope{data=brokerOrderCommandResponse}
// @Failure 400 {object} envelope
// @Failure 409 {object} envelope
// @Failure 500 {object} envelope
// @Router /api/v1/execution/orders [post]
func documentExecutionPlaceRoute() string { return "execution-place" }

// documentExecutionCancelRoute godoc
// @Summary 取消执行订单
// @Tags execution
// @Produce json
// @Param internalOrderId path string true "内部订单 ID"
// @Success 200 {object} envelope{data=brokerOrderCommandResponse}
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/execution/orders/{internalOrderId}/cancel [post]
func documentExecutionCancelRoute() string { return "execution-cancel" }

// documentExecutionEventsRoute godoc
// @Summary 读取执行订单事件
// @Tags execution
// @Produce json
// @Param internalOrderId path string true "内部订单 ID"
// @Success 200 {object} envelope{data=executionOrderEventsResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/execution/orders/{internalOrderId}/events [get]
func documentExecutionEventsRoute() string { return "execution-events" }

// documentSystemOperationalRoutes godoc
// @Summary System operational routes
// @Tags system
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/system/storage/overview [get]
// @Router /api/v1/system/worker/broker-order-updates [get]
func documentSystemOperationalRoutes() string { return "system-operational" }

// documentRealTradeApprovalsRoute godoc
// @Summary 读取实盘审批状态
// @Tags system
// @Produce json
// @Success 200 {object} envelope{data=system.RealTradeApprovalsResponse}
// @Router /api/v1/system/real-trade-approvals [get]
func documentRealTradeApprovalsRoute() string { return "real-trade-approvals" }

// documentRealTradeHardStopsRoute godoc
// @Summary 读取实盘硬停止列表
// @Tags system
// @Produce json
// @Success 200 {object} envelope{data=system.RealTradeHardStopsResponse}
// @Router /api/v1/system/real-trade-hard-stops [get]
func documentRealTradeHardStopsRoute() string { return "real-trade-hard-stops" }

// documentRealTradeHardStopActivateRoute godoc
// @Summary 创建实盘硬停止
// @Tags system
// @Accept json
// @Produce json
// @Param request body system.RealTradeHardStopCommand true "硬停止创建请求"
// @Success 200 {object} envelope{data=trading.RealTradeRiskSnapshot}
// @Failure 400 {object} envelope
// @Failure 409 {object} envelope
// @Router /api/v1/system/real-trade-hard-stops [post]
func documentRealTradeHardStopActivateRoute() string { return "real-trade-hard-stop-activate" }

// documentRealTradeHardStopReleaseRoute godoc
// @Summary 解除实盘硬停止
// @Tags system
// @Accept json
// @Produce json
// @Param hardStopId path string true "硬停止 ID"
// @Param request body system.RealTradeHardStopCommand false "硬停止解除请求"
// @Success 200 {object} envelope{data=trading.RealTradeRiskSnapshot}
// @Failure 400 {object} envelope
// @Failure 409 {object} envelope
// @Router /api/v1/system/real-trade-hard-stops/{hardStopId}/release [post]
func documentRealTradeHardStopReleaseRoute() string { return "real-trade-hard-stop-release" }

// documentRealTradeHardStopEventsRoute godoc
// @Summary 读取实盘硬停止事件
// @Tags system
// @Produce json
// @Success 200 {object} envelope{data=system.RealTradeHardStopEventsResponse}
// @Router /api/v1/system/real-trade-hard-stop-events [get]
func documentRealTradeHardStopEventsRoute() string { return "real-trade-hard-stop-events" }

// documentRealTradeKillSwitchRoute godoc
// @Summary 读取实盘熔断状态
// @Tags system
// @Produce json
// @Success 200 {object} envelope{data=system.RealTradeKillSwitchStateResponse}
// @Router /api/v1/system/real-trade-kill-switch [get]
func documentRealTradeKillSwitchRoute() string { return "real-trade-kill-switch" }

// documentRealTradeKillSwitchActivateRoute godoc
// @Summary 激活实盘熔断
// @Tags system
// @Accept json
// @Produce json
// @Param request body system.RealTradeKillSwitchCommand true "熔断激活请求"
// @Success 200 {object} envelope{data=trading.RealTradeRiskSnapshot}
// @Failure 400 {object} envelope
// @Failure 409 {object} envelope
// @Router /api/v1/system/real-trade-kill-switch/activate [post]
func documentRealTradeKillSwitchActivateRoute() string { return "real-trade-kill-switch-activate" }

// documentRealTradeKillSwitchReleaseRoute godoc
// @Summary 解除实盘熔断
// @Tags system
// @Accept json
// @Produce json
// @Param request body system.RealTradeKillSwitchCommand false "熔断解除请求"
// @Success 200 {object} envelope{data=trading.RealTradeRiskSnapshot}
// @Failure 400 {object} envelope
// @Failure 409 {object} envelope
// @Router /api/v1/system/real-trade-kill-switch/release [post]
func documentRealTradeKillSwitchReleaseRoute() string { return "real-trade-kill-switch-release" }

// documentRealTradeKillSwitchEventsRoute godoc
// @Summary 读取实盘熔断事件
// @Tags system
// @Produce json
// @Success 200 {object} envelope{data=system.RealTradeKillSwitchEventsResponse}
// @Router /api/v1/system/real-trade-kill-switch-events [get]
func documentRealTradeKillSwitchEventsRoute() string { return "real-trade-kill-switch-events" }

// documentRealTradeRiskLimitsRoute godoc
// @Summary 读取实盘运行时风控限额
// @Tags system
// @Produce json
// @Success 200 {object} envelope{data=system.RealTradeRiskLimitsResponse}
// @Router /api/v1/system/real-trade-risk-limits [get]
func documentRealTradeRiskLimitsRoute() string { return "real-trade-risk-limits" }

// documentRealTradeRiskLimitsUpdateRoute godoc
// @Summary 更新实盘运行时风控限额
// @Tags system
// @Accept json
// @Produce json
// @Param request body system.RealTradeRuntimeRiskCommand true "运行时风控配置"
// @Success 200 {object} envelope{data=trading.RealTradeRiskSnapshot}
// @Failure 400 {object} envelope
// @Failure 409 {object} envelope
// @Router /api/v1/system/real-trade-risk-limits [put]
func documentRealTradeRiskLimitsUpdateRoute() string { return "real-trade-risk-limits-update" }

// documentRealTradeRiskLimitsDisableRoute godoc
// @Summary 禁用实盘运行时风控限额
// @Tags system
// @Accept json
// @Produce json
// @Param request body system.RealTradeRuntimeRiskCommand false "禁用请求"
// @Success 200 {object} envelope{data=trading.RealTradeRiskSnapshot}
// @Failure 409 {object} envelope
// @Router /api/v1/system/real-trade-risk-limits [delete]
func documentRealTradeRiskLimitsDisableRoute() string { return "real-trade-risk-limits-disable" }

// documentRealTradeRiskEventsRoute godoc
// @Summary 读取实盘运行时风控事件
// @Tags system
// @Produce json
// @Success 200 {object} envelope{data=system.RealTradeRiskEventsResponse}
// @Router /api/v1/system/real-trade-risk-events [get]
func documentRealTradeRiskEventsRoute() string { return "real-trade-risk-events" }

// documentExecutionPreviewRoute godoc
// @Summary 预览执行订单但不提交（已废弃别名）
// @Description 规范化并校验订单请求，返回预览结果，不会向券商提交订单。已废弃，请改用 POST /api/v1/execution/previews。
// @Tags execution
// @Produce json
// @Success 200 {object} envelope
// @Deprecated
// @Router /api/v1/execution/orders/preview [post]
func documentExecutionPreviewRoute() string { return "execution-preview" }
