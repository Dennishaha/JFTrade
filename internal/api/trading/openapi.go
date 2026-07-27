//nolint:unused // These annotation-only stubs are consumed by swag during go generate.
package trading

import srv "github.com/jftrade/jftrade-main/internal/trading"

var _ srv.ExecutionOrders

// documentPortfolioCashBalancesRoute godoc
// @Summary 读取 portfolio 现金余额
// @Tags portfolio
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Success 200 {object} httpserver.Envelope{data=srv.PortfolioCashBalancesResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/portfolio/{brokerId}/cash-balances [get]
func documentPortfolioCashBalancesRoute() string { return "portfolio-cash-balances" }

// documentPortfolioPositionsRoute godoc
// @Summary 读取 portfolio 持仓
// @Tags portfolio
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Success 200 {object} httpserver.Envelope{data=srv.PortfolioPositionsResponse}
// @Failure 401 {object} httpserver.ErrorEnvelope
// @Router /api/v1/portfolio/{brokerId}/positions [get]
func documentPortfolioPositionsRoute() string { return "portfolio-positions" }

// documentBrokerFundsRoute godoc
// @Summary 读取券商资金
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Param tradingEnvironment query string false "交易环境"
// @Param accountId query string false "账户 ID"
// @Param market query string false "市场代码"
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerFundsResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerPositionsResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerOrdersResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerFillsResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerCashFlowsResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerOrderFeesResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerMarginRatiosResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerMaxTradeQuantityResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerQuoteResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerKLinesResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerSecuritiesResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Router /api/v1/brokers/{brokerId}/securities [get]
func documentBrokerSecuritiesRoute() string { return "broker-securities" }

// documentBrokerRuntimeRoute godoc
// @Summary 读取券商运行时状态
// @Tags broker
// @Produce json
// @Param brokerId path string true "券商 ID"
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerRuntimeResponse}
// @Failure 404 {object} httpserver.ErrorEnvelope
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
// @Param request body PlaceOrderRequest true "下单请求"
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerPlaceOrderResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
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
// @Param request body CancelOrdersRequest true "撤单请求"
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerCancelOrdersResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
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
// @Param request body UnlockTradeRequest true "解锁请求"
// @Success 200 {object} httpserver.Envelope{data=srv.BrokerUnlockTradeResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
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
// @Success 200 {object} httpserver.Envelope{data=srv.ExecutionOrders}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Router /api/v1/execution/orders [get]
func documentExecutionOrdersRoute() string { return "execution-orders" }

// documentExecutionOrderDetailsRoute godoc
// @Summary 读取单笔执行订单及最近事件
// @Tags execution
// @Produce json
// @Param internalOrderId path string true "内部订单 ID"
// @Success 200 {object} httpserver.Envelope{data=srv.ExecutionOrderDetails}
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Router /api/v1/execution/orders/{internalOrderId} [get]
func documentExecutionOrderDetailsRoute() string { return "execution-order-details" }

// documentExecutionPlaceRoute godoc
// @Summary 提交执行订单
// @Tags execution
// @Accept json
// @Produce json
// @Param request body srv.ExecutionPlaceRequest true "执行订单"
// @Success 200 {object} httpserver.Envelope{data=srv.ExecutionCommandResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 409 {object} httpserver.ErrorEnvelope
// @Failure 500 {object} httpserver.ErrorEnvelope
// @Router /api/v1/execution/orders [post]
func documentExecutionPlaceRoute() string { return "execution-place" }

// documentExecutionCancelRoute godoc
// @Summary 取消执行订单
// @Tags execution
// @Produce json
// @Param internalOrderId path string true "内部订单 ID"
// @Success 200 {object} httpserver.Envelope{data=srv.ExecutionCommandResponse}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Failure 404 {object} httpserver.ErrorEnvelope
// @Router /api/v1/execution/orders/{internalOrderId}/cancel [post]
func documentExecutionCancelRoute() string { return "execution-cancel" }

// documentExecutionEventsRoute godoc
// @Summary 读取执行订单事件
// @Tags execution
// @Produce json
// @Param internalOrderId path string true "内部订单 ID"
// @Success 200 {object} httpserver.Envelope{data=srv.ExecutionOrderEvents}
// @Failure 400 {object} httpserver.ErrorEnvelope
// @Router /api/v1/execution/orders/{internalOrderId}/events [get]
func documentExecutionEventsRoute() string { return "execution-events" }
