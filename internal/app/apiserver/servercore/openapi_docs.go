//nolint:unused // These annotation-only stubs are consumed by swag during go generate.
package servercore

import "github.com/jftrade/jftrade-main/internal/datamanagement"

type dataCleanupPreviewRequest = datamanagement.CleanupPreviewRequest
type dataCleanupExecuteRequest = datamanagement.CleanupExecuteRequest
type databaseCompactRequest = datamanagement.CompactRequest
type databaseRebuildRequest = datamanagement.RebuildRequest

// documentDataMigrationRoutes godoc
// @Summary Database compatibility status and rebuild scheduling
// @Tags settings
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/settings/data-migration/databases [get]
// @Router /api/v1/settings/data-migration/databases/rebuild [post]
func documentDataMigrationRoutes() {}

// documentDataManagementOverview godoc
// @Summary Database storage usage and cleanup opportunities
// @Tags settings
// @Produce json
// @Param summaryOnly query bool false "Return only database status without SQLite storage or cleanup statistics"
// @Param databaseId query string false "Return one database overview for incremental loading"
// @Success 200 {object} envelope
// @Router /api/v1/settings/data-management/databases [get]
func documentDataManagementOverview() {}

// documentDataCleanupPreview godoc
// @Summary Preview an exact database cleanup candidate set
// @Tags settings
// @Accept json
// @Produce json
// @Param request body dataCleanupPreviewRequest true "Cleanup preview request"
// @Success 200 {object} envelope
// @Router /api/v1/settings/data-management/cleanup/preview [post]
func documentDataCleanupPreview() {}

// documentDataCleanupExecute godoc
// @Summary Execute a previously previewed database cleanup
// @Tags settings
// @Accept json
// @Produce json
// @Param request body dataCleanupExecuteRequest true "Cleanup execution request"
// @Success 200 {object} envelope
// @Router /api/v1/settings/data-management/cleanup/execute [post]
func documentDataCleanupExecute() {}

// documentDatabaseCompact godoc
// @Summary Checkpoint and compact one database
// @Tags settings
// @Accept json
// @Produce json
// @Param databaseId path string true "Database ID"
// @Param request body databaseCompactRequest true "Compaction confirmation"
// @Success 200 {object} envelope
// @Router /api/v1/settings/data-management/databases/{databaseId}/compact [post]
func documentDatabaseCompact() {}

// documentDatabaseRebuild godoc
// @Summary Schedule a database rebuild on next startup
// @Tags settings
// @Accept json
// @Produce json
// @Param request body databaseRebuildRequest true "Database rebuild request"
// @Success 200 {object} envelope
// @Router /api/v1/settings/data-management/databases/rebuild [post]
func documentDatabaseRebuild() {}

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
func documentAssistantCatalogRoutes() {}

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
func documentAssistantTaskMemoryRoutes() {}

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
func documentAssistantSessionRunRoutes() {}

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
// @Router /api/v1/adk/skills [get]
// @Router /api/v1/adk/skills [post]
// @Router /api/v1/adk/skills/{skillId} [put]
// @Router /api/v1/adk/skills/{skillId} [delete]
func documentAssistantChatApprovalSkillRoutes() {}

// documentAssistantOptimizationRoutes godoc
// @Summary ADK optimization task routes
// @Tags adk
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/adk/optimization-tasks [get]
// @Router /api/v1/adk/optimization-tasks/{taskId} [get]
// @Router /api/v1/adk/optimization-tasks/{taskId}/cancel [post]
func documentAssistantOptimizationRoutes() {}

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
func documentAssistantWorkflowRoutes() {}

// documentBacktestSyncTaskRoutes godoc
// @Summary Backtest historical data sync task routes
// @Tags backtest
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/backtests/sync/{taskId} [get]
// @Router /api/v1/backtests/sync/{taskId} [delete]
func documentBacktestSyncTaskRoutes() {}

// documentMarketUtilityRoutes godoc
// @Summary Market data utility routes
// @Tags market-data
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/market-data/markets [get]
// @Router /api/v1/market-data/subscriptions [get]
// @Router /api/v1/market-data/instruments/normalize [post]
func documentMarketUtilityRoutes() {}

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
func documentPluginRoutes() {}

// documentPortfolioRoutes godoc
// @Summary Portfolio routes
// @Tags portfolio
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/portfolio/{brokerId}/cash-balances [get]
// @Router /api/v1/portfolio/{brokerId}/positions [get]
// @Router /api/v1/portfolio/{brokerId}/cash-reconciliation [get]
// @Router /api/v1/portfolio/{brokerId}/reconciliation [get]
func documentPortfolioRoutes() {}

// documentBrokerFundsRoute godoc
// @Summary 读取券商资金
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/funds [get]
func documentBrokerFundsRoute() {}

// documentBrokerPositionsRoute godoc
// @Summary 读取券商持仓
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/positions [get]
func documentBrokerPositionsRoute() {}

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
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/orders [get]
func documentBrokerOrdersRoute() {}

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
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/fills [get]
func documentBrokerFillsRoute() {}

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
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/cash-flows [get]
func documentBrokerCashFlowsRoute() {}

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
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/order-fees [get]
func documentBrokerOrderFeesRoute() {}

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
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/margin-ratios [get]
func documentBrokerMarginRatiosRoute() {}

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
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/max-trade-qtys [get]
func documentBrokerMaxTradeQuantityRoute() {}

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
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/quote [get]
func documentBrokerQuoteRoute() {}

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
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/klines [get]
func documentBrokerKLinesRoute() {}

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
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/securities [get]
func documentBrokerSecuritiesRoute() {}

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
func documentExecutionOrdersRoute() {}

// documentExecutionOrderDetailsRoute godoc
// @Summary 读取单笔执行订单及最近事件
// @Tags execution
// @Produce json
// @Param internalOrderId path string true "内部订单 ID"
// @Success 200 {object} envelope{data=executionOrderDetailsResponse}
// @Failure 404 {object} envelope
// @Router /api/v1/execution/orders/{internalOrderId} [get]
func documentExecutionOrderDetailsRoute() {}

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
func documentExecutionPlaceRoute() {}

// documentExecutionCancelRoute godoc
// @Summary 取消执行订单
// @Tags execution
// @Produce json
// @Param internalOrderId path string true "内部订单 ID"
// @Success 200 {object} envelope{data=brokerOrderCommandResponse}
// @Failure 400 {object} envelope
// @Failure 404 {object} envelope
// @Router /api/v1/execution/orders/{internalOrderId}/cancel [post]
func documentExecutionCancelRoute() {}

// documentExecutionEventsRoute godoc
// @Summary 读取执行订单事件
// @Tags execution
// @Produce json
// @Param internalOrderId path string true "内部订单 ID"
// @Success 200 {object} envelope{data=executionOrderEventsResponse}
// @Failure 400 {object} envelope
// @Router /api/v1/execution/orders/{internalOrderId}/events [get]
func documentExecutionEventsRoute() {}

// documentSystemOperationalRoutes godoc
// @Summary System operational routes
// @Tags system
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/system/storage/overview [get]
// @Router /api/v1/system/real-trade-approvals [get]
// @Router /api/v1/system/real-trade-hard-stops [get]
// @Router /api/v1/system/real-trade-hard-stops [post]
// @Router /api/v1/system/real-trade-hard-stops/{hardStopId}/release [post]
// @Router /api/v1/system/real-trade-hard-stop-events [get]
// @Router /api/v1/system/real-trade-kill-switch [get]
// @Router /api/v1/system/real-trade-kill-switch/activate [post]
// @Router /api/v1/system/real-trade-kill-switch/release [post]
// @Router /api/v1/system/real-trade-kill-switch-events [get]
// @Router /api/v1/system/real-trade-risk-limits [get]
// @Router /api/v1/system/real-trade-risk-limits [put]
// @Router /api/v1/system/real-trade-risk-limits [delete]
// @Router /api/v1/system/real-trade-risk-events [get]
// @Router /api/v1/system/worker/broker-order-updates [get]
func documentSystemOperationalRoutes() {}

// documentExecutionPreviewRoute godoc
// @Summary 预览执行订单但不提交
// @Description 规范化并校验订单请求，返回预览结果，不会向券商提交订单。
// @Tags execution
// @Produce json
// @Success 200 {object} envelope
// @Router /api/v1/execution/orders/preview [post]
func documentExecutionPreviewRoute() {}
