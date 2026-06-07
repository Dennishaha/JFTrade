package jftradeapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (s *Server) handleBrokerRead(c *gin.Context) {
	brokerID, resource, ok := s.brokerRouteParams(c)
	if !ok {
		s.notFound(c)
		return
	}
	// Look up the broker by ID from the registry.
	activeBroker := s.activeBroker()
	if activeBroker == nil || activeBroker.ID() != brokerID {
		// For backward compatibility, only "futu" is supported currently.
		if brokerID != "futu" {
			s.notFound(c)
			return
		}
	}

	switch resource {
	case "runtime":
		s.writeOK(c, s.brokerRuntime(c.Request.Context()))
	case "funds":
		s.handleBrokerFundsRead(c)
	case "positions":
		s.handleBrokerPositionsRead(c)
	case "orders":
		s.handleBrokerOrdersRead(c)
	case "fills":
		s.handleBrokerFillsRead(c)
	case "cash-flows":
		s.handleBrokerCashFlowsRead(c)
	case "order-fees":
		s.handleBrokerOrderFeesRead(c)
	case "margin-ratios":
		s.handleBrokerMarginRatiosRead(c)
	case "max-trade-qtys":
		s.handleBrokerMaxTradeQuantityRead(c)
	case "quote":
		s.handleBrokerQuoteRead(c)
	case "klines":
		s.handleBrokerKLinesRead(c)
	case "securities":
		s.handleBrokerSecuritiesRead(c)
	default:
		s.notFound(c)
	}
}

func (s *Server) handleBrokerWrite(c *gin.Context) {
	brokerID, resource, ok := s.brokerRouteParams(c)
	if !ok {
		s.notFound(c)
		return
	}
	activeBroker := s.activeBroker()
	if activeBroker == nil || activeBroker.ID() != brokerID {
		if brokerID != "futu" {
			s.notFound(c)
			return
		}
	}
	var query brokerBaseReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker write query")
		return
	}
	readQuery := s.brokerReadQuery(query)
	switch {
	case resource == "orders" && c.Request.Method == http.MethodPost:
		s.handlePlaceOrder(c, readQuery)
	case resource == "orders" && c.Request.Method == http.MethodDelete:
		s.handleCancelOrders(c, readQuery)
	case resource == "unlock" && c.Request.Method == http.MethodPost:
		s.handleUnlockTrade(c, readQuery)
	default:
		s.notFound(c)
	}
}

func (s *Server) brokerRouteParams(c *gin.Context) (brokerID string, resource string, ok bool) {
	var uri brokerResourceURI
	if err := bindURI(c, &uri); err != nil || strings.TrimSpace(uri.BrokerID) == "" || strings.TrimSpace(uri.Resource) == "" {
		return "", "", false
	}
	return uri.BrokerID, uri.Resource, true
}

type brokerOrdersReadRequest struct {
	ReadQuery broker.ReadQuery
	Scope     string
	Symbol    string
	StartTime string
	EndTime   string
	Statuses  []string
}

type brokerFillsReadRequest struct {
	ReadQuery broker.ReadQuery
	Scope     string
	Symbol    string
	StartTime string
	EndTime   string
}

func (s *Server) brokerReadQuery(query brokerBaseReadQuery) broker.ReadQuery {
	market := strings.TrimSpace(query.Market)
	if market == "" {
		market = strings.TrimSpace(s.store.integration().Config.TradeMarket)
	}
	if market == "" {
		market = "HK"
	}
	return broker.ReadQuery{
		BrokerID:           "futu",
		TradingEnvironment: strings.TrimSpace(query.TradingEnvironment),
		AccountID:          strings.TrimSpace(query.AccountID),
		Market:             market,
	}
}

// handleBrokerFundsRead godoc
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
func (s *Server) handleBrokerFundsRead(c *gin.Context) {
	var query brokerBaseReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker funds query")
		return
	}
	s.writeOK(c, s.brokerFundsResponse(c.Request.Context(), s.brokerReadQuery(query)))
}

// handleBrokerPositionsRead godoc
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
func (s *Server) handleBrokerPositionsRead(c *gin.Context) {
	var query brokerBaseReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker positions query")
		return
	}
	s.writeOK(c, s.brokerPositionsResponse(c.Request.Context(), s.brokerReadQuery(query)))
}

// handleBrokerOrdersRead godoc
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
func (s *Server) handleBrokerOrdersRead(c *gin.Context) {
	var query brokerOrdersReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker orders query")
		return
	}
	request, err := s.brokerOrdersRequest(query)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	s.writeOK(c, s.brokerOrdersResponse(c.Request.Context(), request))
}

// handleBrokerFillsRead godoc
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
func (s *Server) handleBrokerFillsRead(c *gin.Context) {
	var query brokerFillsReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker fills query")
		return
	}
	request, err := s.brokerFillsRequest(query)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	s.writeOK(c, s.brokerFillsResponse(c.Request.Context(), request))
}

// handleBrokerCashFlowsRead godoc
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
func (s *Server) handleBrokerCashFlowsRead(c *gin.Context) {
	var query brokerCashFlowsReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker cash flows query")
		return
	}
	request, err := s.brokerCashFlowsRequest(query)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	s.writeOK(c, s.brokerCashFlowsResponse(c.Request.Context(), request))
}

// handleBrokerOrderFeesRead godoc
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
func (s *Server) handleBrokerOrderFeesRead(c *gin.Context) {
	var query brokerOrderFeesReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker order fees query")
		return
	}
	request, err := s.brokerOrderFeesRequest(query)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	s.writeOK(c, s.brokerOrderFeesResponse(c.Request.Context(), request))
}

// handleBrokerMarginRatiosRead godoc
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
func (s *Server) handleBrokerMarginRatiosRead(c *gin.Context) {
	var query brokerMarginRatiosReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker margin ratios query")
		return
	}
	request, err := s.brokerMarginRatiosRequest(query)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	s.writeOK(c, s.brokerMarginRatiosResponse(c.Request.Context(), request))
}

// handleBrokerMaxTradeQuantityRead godoc
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
// @Param positionId query integer false "持仓 ID"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/max-trade-qtys [get]
func (s *Server) handleBrokerMaxTradeQuantityRead(c *gin.Context) {
	var query brokerMaxTradeQuantityReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker max trade quantity query")
		return
	}
	request, err := s.brokerMaxTradeQuantityRequest(query)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	s.writeOK(c, s.brokerMaxTradeQuantityResponse(c.Request.Context(), request))
}

// handleBrokerQuoteRead godoc
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
func (s *Server) handleBrokerQuoteRead(c *gin.Context) {
	var query brokerQuoteReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker quote query")
		return
	}
	request, err := s.brokerQuoteRequest(query)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	s.writeOK(c, s.brokerQuoteResponse(c.Request.Context(), request))
}

// handleBrokerKLinesRead godoc
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
// @Param limit query integer false "返回条数"
// @Success 200 {object} envelope
// @Failure 400 {object} envelope
// @Router /api/v1/brokers/{brokerId}/klines [get]
func (s *Server) handleBrokerKLinesRead(c *gin.Context) {
	var query brokerKLinesReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker klines query")
		return
	}
	request, err := s.brokerKLinesRequest(query)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	s.writeOK(c, s.brokerKLinesResponse(c.Request.Context(), request))
}

// handleBrokerSecuritiesRead godoc
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
func (s *Server) handleBrokerSecuritiesRead(c *gin.Context) {
	var query brokerSecuritiesReadQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid broker securities query")
		return
	}
	request, err := s.brokerSecuritiesRequest(query)
	if err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	s.writeOK(c, s.brokerSecuritiesSnapshotResponse(c.Request.Context(), request))
}

// brokerMarketDataReader returns the MarketDataReader for the active broker,
// or falls back to the legacy futuExchange() path if the broker does not support market data.
func (s *Server) brokerMarketDataReader() broker.MarketDataReader {
	b := s.activeBroker()
	if b == nil {
		return nil
	}
	return b.MarketData()
}

func (s *Server) brokerFundsResponse(ctx context.Context, query broker.ReadQuery) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("summary", nil, fmt.Errorf("broker market data not available"), "currencyBalances", "marketAssets")
	}
	snapshot, err := reader.QueryFunds(ctx, query)
	if err != nil {
		return brokerReadErrorResponse("summary", nil, err, "currencyBalances", "marketAssets")
	}

	currencyBalances := make([]any, 0, len(snapshot.CurrencyBalances))
	for _, balance := range snapshot.CurrencyBalances {
		currencyBalances = append(currencyBalances, map[string]any{
			"accountId":               balance.AccountID,
			"tradingEnvironment":      balance.TradingEnvironment,
			"currency":                balance.Currency,
			"cash":                    balance.Cash,
			"availableWithdrawalCash": balance.AvailableWithdrawalCash,
			"netCashPower":            balance.NetCashPower,
		})
	}

	marketAssets := make([]any, 0, len(snapshot.MarketAssets))
	for _, asset := range snapshot.MarketAssets {
		marketAssets = append(marketAssets, map[string]any{
			"accountId":          asset.AccountID,
			"tradingEnvironment": asset.TradingEnvironment,
			"market":             asset.Market,
			"assets":             asset.Assets,
		})
	}

	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"summary": map[string]any{
			"accountId":               snapshot.AccountID,
			"tradingEnvironment":      snapshot.TradingEnvironment,
			"market":                  snapshot.Market,
			"currency":                snapshot.Currency,
			"totalAssets":             snapshot.TotalAssets,
			"securitiesAssets":        snapshot.SecuritiesAssets,
			"fundAssets":              snapshot.FundAssets,
			"bondAssets":              snapshot.BondAssets,
			"cash":                    snapshot.Cash,
			"marketValue":             snapshot.MarketValue,
			"longMarketValue":         snapshot.LongMarketValue,
			"shortMarketValue":        snapshot.ShortMarketValue,
			"purchasingPower":         snapshot.PurchasingPower,
			"shortSellingPower":       snapshot.ShortSellingPower,
			"netCashPower":            snapshot.NetCashPower,
			"availableWithdrawalCash": snapshot.AvailableWithdrawalCash,
			"maxWithdrawal":           snapshot.MaxWithdrawal,
			"availableFunds":          snapshot.AvailableFunds,
			"frozenCash":              snapshot.FrozenCash,
			"pendingAsset":            snapshot.PendingAsset,
			"unrealizedPnl":           snapshot.UnrealizedPnl,
			"realizedPnl":             snapshot.RealizedPnl,
			"initialMargin":           snapshot.InitialMargin,
			"maintenanceMargin":       snapshot.MaintenanceMargin,
			"marginCallMargin":        snapshot.MarginCallMargin,
			"riskStatus":              snapshot.RiskStatus,
			// Margin & Financing 融资融券
			"debtCash":       snapshot.DebtCash,
			"isPdt":          snapshot.IsPDT,
			"pdtSeq":         snapshot.PDTSeq,
			"beginningDTBP":  snapshot.BeginningDTBP,
			"remainingDTBP":  snapshot.RemainingDTBP,
			"dtCallAmount":   snapshot.DTCallAmount,
			"dtStatus":       snapshot.DTStatus,
			"exposureLevel":  snapshot.ExposureLevel,
			"exposureLimit":  snapshot.ExposureLimit,
			"usedLimit":      snapshot.UsedLimit,
			"remainingLimit": snapshot.RemainingLimit,
		},
		"currencyBalances": currencyBalances,
		"marketAssets":     marketAssets,
	}
}

func (s *Server) brokerPositionsResponse(ctx context.Context, query broker.ReadQuery) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("positions", []any{}, fmt.Errorf("broker market data not available"))
	}
	positions, err := reader.QueryPositions(ctx, query)
	if err != nil {
		return brokerReadErrorResponse("positions", []any{}, err)
	}

	entries := make([]any, 0, len(positions))
	for _, position := range positions {
		entries = append(entries, map[string]any{
			"accountId":          position.AccountID,
			"tradingEnvironment": position.TradingEnvironment,
			"market":             position.Market,
			"symbol":             position.Symbol,
			"symbolName":         position.SymbolName,
			"quantity":           position.Quantity,
			"sellableQuantity":   position.SellableQuantity,
			"lastPrice":          position.LastPrice,
			"costPrice":          position.CostPrice,
			"averageCostPrice":   position.AverageCostPrice,
			"marketValue":        position.MarketValue,
			"unrealizedPnl":      position.UnrealizedPnl,
			"realizedPnl":        position.RealizedPnl,
			"pnlRatio":           position.PnlRatio,
			"currency":           position.Currency,
		})
	}

	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"positions":    entries,
	}
}

// brokerFundsResponseWithTimeout wraps brokerFundsResponse with a dedicated timeout.
// On timeout it returns a degraded result instead of blocking indefinitely.
func (s *Server) brokerFundsResponseWithTimeout(ctx context.Context, query broker.ReadQuery, timeout time.Duration) map[string]any {
	fundsCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type result struct {
		value map[string]any
	}
	done := make(chan result, 1)
	go func() {
		done <- result{value: s.brokerFundsResponse(fundsCtx, query)}
	}()
	select {
	case <-fundsCtx.Done():
		return brokerReadErrorResponse("summary", nil, fmt.Errorf("funds query timed out after %s", timeout), "currencyBalances", "marketAssets")
	case r := <-done:
		return r.value
	}
}

// brokerPositionsResponseWithTimeout wraps brokerPositionsResponse with a dedicated timeout.
// On timeout it returns a degraded result instead of blocking indefinitely.
func (s *Server) brokerPositionsResponseWithTimeout(ctx context.Context, query broker.ReadQuery, timeout time.Duration) map[string]any {
	positionsCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type result struct {
		value map[string]any
	}
	done := make(chan result, 1)
	go func() {
		done <- result{value: s.brokerPositionsResponse(positionsCtx, query)}
	}()
	select {
	case <-positionsCtx.Done():
		return brokerReadErrorResponse("positions", []any{}, fmt.Errorf("positions query timed out after %s", timeout))
	case r := <-done:
		return r.value
	}
}

func (s *Server) brokerOrdersRequest(query brokerOrdersReadQuery) (brokerOrdersReadRequest, error) {
	scope, err := normalizeBrokerReadScope(query.Scope)
	if err != nil {
		return brokerOrdersReadRequest{}, err
	}
	return brokerOrdersReadRequest{
		ReadQuery: s.brokerReadQuery(query.brokerBaseReadQuery),
		Scope:     scope,
		Symbol:    strings.TrimSpace(query.Symbol),
		StartTime: strings.TrimSpace(query.StartTime),
		EndTime:   strings.TrimSpace(query.EndTime),
		Statuses:  mergeBrokerQueryValues(query.Status, query.Statuses),
	}, nil
}

func (s *Server) brokerOrdersResponse(ctx context.Context, request brokerOrdersReadRequest) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("orders", []any{}, fmt.Errorf("broker market data not available"))
	}

	var (
		orders []broker.OrderSnapshot
		err    error
	)
	if request.Scope == "HISTORY" {
		orders, err = reader.QueryHistoryOrders(ctx, broker.OrderHistoryQuery{
			ReadQuery: request.ReadQuery,
			Symbol:    request.Symbol,
			StartTime: request.StartTime,
			EndTime:   request.EndTime,
			Statuses:  request.Statuses,
		})
	} else {
		orders, err = reader.QueryOrders(ctx, request.ReadQuery, request.Symbol)
	}
	if err != nil {
		return brokerReadErrorResponse("orders", []any{}, err)
	}

	entries := make([]any, 0, len(orders))
	for _, order := range orders {
		accountID := firstNonEmptyString(order.AccountID, request.ReadQuery.AccountID)
		tradingEnvironment := firstNonEmptyString(order.TradingEnvironment, request.ReadQuery.TradingEnvironment)
		market := firstNonEmptyString(order.Market, request.ReadQuery.Market)
		entries = append(entries, map[string]any{
			"accountId":          accountID,
			"tradingEnvironment": tradingEnvironment,
			"market":             market,
			"brokerOrderId":      order.BrokerOrderID,
			"brokerOrderIdEx":    order.BrokerOrderIDEx,
			"symbol":             order.Symbol,
			"symbolName":         order.SymbolName,
			"side":               order.Side,
			"orderType":          order.OrderType,
			"status":             order.Status,
			"quantity":           order.Quantity,
			"filledQuantity":     order.FilledQuantity,
			"price":              order.Price,
			"filledAveragePrice": order.FilledAveragePrice,
			"submittedAt":        order.SubmittedAt,
			"updatedAt":          order.UpdatedAt,
			"remark":             order.Remark,
			"lastError":          order.LastError,
			"timeInForce":        order.TimeInForce,
			"currency":           order.Currency,
		})
	}

	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"orders":       entries,
	}
}

func (s *Server) brokerFillsRequest(query brokerFillsReadQuery) (brokerFillsReadRequest, error) {
	scope, err := normalizeBrokerReadScope(query.Scope)
	if err != nil {
		return brokerFillsReadRequest{}, err
	}
	return brokerFillsReadRequest{
		ReadQuery: s.brokerReadQuery(query.brokerBaseReadQuery),
		Scope:     scope,
		Symbol:    strings.TrimSpace(query.Symbol),
		StartTime: strings.TrimSpace(query.StartTime),
		EndTime:   strings.TrimSpace(query.EndTime),
	}, nil
}

func (s *Server) brokerFillsResponse(ctx context.Context, request brokerFillsReadRequest) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("fills", []any{}, fmt.Errorf("broker market data not available"))
	}

	fillQuery := broker.OrderFillQuery{
		ReadQuery: request.ReadQuery,
		Symbol:    request.Symbol,
		StartTime: request.StartTime,
		EndTime:   request.EndTime,
	}
	var (
		fills []broker.OrderFillSnapshot
		err   error
	)
	if request.Scope == "HISTORY" {
		fills, err = reader.QueryHistoryOrderFills(ctx, broker.OrderFillHistoryQuery(fillQuery))
	} else {
		fills, err = reader.QueryOrderFills(ctx, fillQuery)
	}
	if err != nil {
		return brokerReadErrorResponse("fills", []any{}, err)
	}

	entries := make([]any, 0, len(fills))
	for _, fill := range fills {
		entries = append(entries, map[string]any{
			"accountId":          fill.AccountID,
			"tradingEnvironment": fill.TradingEnvironment,
			"market":             fill.Market,
			"brokerOrderId":      fill.BrokerOrderID,
			"brokerOrderIdEx":    fill.BrokerOrderIDEx,
			"brokerFillId":       fill.BrokerFillID,
			"brokerFillIdEx":     fill.BrokerFillIDEx,
			"symbol":             fill.Symbol,
			"symbolName":         fill.SymbolName,
			"side":               fill.Side,
			"filledQuantity":     fill.FilledQuantity,
			"fillPrice":          fill.FillPrice,
			"filledAt":           fill.FilledAt,
			"status":             fill.Status,
		})
	}

	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"fills":        entries,
	}
}

func (s *Server) brokerCashFlowsRequest(query brokerCashFlowsReadQuery) (broker.CashFlowQuery, error) {
	clearingDate := strings.TrimSpace(query.ClearingDate)
	if clearingDate == "" {
		return broker.CashFlowQuery{}, fmt.Errorf("query parameter clearingDate is required")
	}
	return broker.CashFlowQuery{
		ReadQuery:    s.brokerReadQuery(query.brokerBaseReadQuery),
		ClearingDate: clearingDate,
		Direction:    strings.TrimSpace(query.Direction),
	}, nil
}

func (s *Server) brokerCashFlowsResponse(ctx context.Context, query broker.CashFlowQuery) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("cashFlows", []any{}, fmt.Errorf("broker market data not available"))
	}
	flows, err := reader.QueryCashFlows(ctx, query)
	if err != nil {
		return brokerReadErrorResponse("cashFlows", []any{}, err)
	}

	entries := make([]any, 0, len(flows))
	for _, flow := range flows {
		entries = append(entries, map[string]any{
			"accountId":          flow.AccountID,
			"tradingEnvironment": flow.TradingEnvironment,
			"market":             flow.Market,
			"cashFlowId":         flow.CashFlowID,
			"clearingDate":       flow.ClearingDate,
			"settlementDate":     flow.SettlementDate,
			"currency":           flow.Currency,
			"cashFlowType":       flow.CashFlowType,
			"cashFlowDirection":  flow.CashFlowDirection,
			"cashFlowAmount":     flow.CashFlowAmount,
			"cashFlowRemark":     flow.CashFlowRemark,
		})
	}

	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"cashFlows":    entries,
	}
}

func (s *Server) brokerOrderFeesRequest(query brokerOrderFeesReadQuery) (broker.OrderFeeQuery, error) {
	orderIDExList := mergeBrokerQueryValues(query.OrderIDEx, query.OrderIDExList)
	if len(orderIDExList) == 0 {
		return broker.OrderFeeQuery{}, fmt.Errorf("query parameter orderIdEx is required")
	}
	return broker.OrderFeeQuery{
		ReadQuery:     s.brokerReadQuery(query.brokerBaseReadQuery),
		OrderIDExList: orderIDExList,
	}, nil
}

func (s *Server) brokerOrderFeesResponse(ctx context.Context, query broker.OrderFeeQuery) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("fees", []any{}, fmt.Errorf("broker market data not available"))
	}
	fees, err := reader.QueryOrderFees(ctx, query)
	if err != nil {
		return brokerReadErrorResponse("fees", []any{}, err)
	}

	entries := make([]any, 0, len(fees))
	for _, fee := range fees {
		feeItems := make([]any, 0, len(fee.FeeItems))
		for _, item := range fee.FeeItems {
			feeItems = append(feeItems, map[string]any{"title": item.Title, "value": item.Value})
		}
		entries = append(entries, map[string]any{
			"accountId":          fee.AccountID,
			"tradingEnvironment": fee.TradingEnvironment,
			"market":             fee.Market,
			"brokerOrderIdEx":    fee.BrokerOrderIDEx,
			"feeAmount":          fee.FeeAmount,
			"feeItems":           feeItems,
		})
	}

	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"fees":         entries,
	}
}

func (s *Server) brokerMarginRatiosRequest(query brokerMarginRatiosReadQuery) (broker.MarginRatioQuery, error) {
	readQuery := s.brokerReadQuery(query.brokerBaseReadQuery)
	rawSymbols := mergeBrokerQueryValues(query.Symbol, query.Symbols)
	symbols := make([]string, 0, len(rawSymbols))
	for _, symbol := range rawSymbols {
		instrument, err := normalizeInstrumentInput(readQuery.Market, symbol, "")
		if err != nil {
			return broker.MarginRatioQuery{}, fmt.Errorf("query parameter symbol %q is invalid: %w", symbol, err)
		}
		symbols = append(symbols, instrument.Symbol)
	}
	if len(symbols) == 0 {
		return broker.MarginRatioQuery{}, fmt.Errorf("query parameter symbol is required")
	}
	return broker.MarginRatioQuery{
		ReadQuery: readQuery,
		Symbols:   symbols,
	}, nil
}

func (s *Server) brokerMarginRatiosResponse(ctx context.Context, query broker.MarginRatioQuery) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("marginRatios", []any{}, fmt.Errorf("broker market data not available"))
	}
	ratios, err := reader.QueryMarginRatios(ctx, query)
	if err != nil {
		return brokerReadErrorResponse("marginRatios", []any{}, err)
	}

	entries := make([]any, 0, len(ratios))
	for _, ratio := range ratios {
		entries = append(entries, map[string]any{
			"accountId":               ratio.AccountID,
			"tradingEnvironment":      ratio.TradingEnvironment,
			"market":                  ratio.Market,
			"symbol":                  ratio.Symbol,
			"isLongPermit":            ratio.IsLongPermit,
			"isShortPermit":           ratio.IsShortPermit,
			"shortPoolRemain":         ratio.ShortPoolRemain,
			"shortFeeRate":            ratio.ShortFeeRate,
			"alertLongRatio":          ratio.AlertLongRatio,
			"alertShortRatio":         ratio.AlertShortRatio,
			"initialMarginLongRatio":  ratio.InitialMarginLongRatio,
			"initialMarginShortRatio": ratio.InitialMarginShortRatio,
			"marginCallLongRatio":     ratio.MarginCallLongRatio,
			"marginCallShortRatio":    ratio.MarginCallShortRatio,
			"maintenanceLongRatio":    ratio.MaintenanceLongRatio,
			"maintenanceShortRatio":   ratio.MaintenanceShortRatio,
		})
	}

	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"marginRatios": entries,
	}
}

func (s *Server) brokerMaxTradeQuantityRequest(query brokerMaxTradeQuantityReadQuery) (broker.MaxTradeQuantityQuery, error) {
	symbol := strings.TrimSpace(query.Symbol)
	orderType := strings.TrimSpace(query.OrderType)
	priceValue := strings.TrimSpace(query.Price)
	if symbol == "" || orderType == "" || priceValue == "" {
		return broker.MaxTradeQuantityQuery{}, fmt.Errorf("query parameters symbol, orderType, and price are required")
	}
	price, err := strconv.ParseFloat(priceValue, 64)
	if err != nil {
		return broker.MaxTradeQuantityQuery{}, fmt.Errorf("query parameter price is invalid: %w", err)
	}

	var adjustSideAndLimit *float64
	if raw := strings.TrimSpace(query.AdjustSideAndLimit); raw != "" {
		parsed, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil {
			return broker.MaxTradeQuantityQuery{}, fmt.Errorf("query parameter adjustSideAndLimit is invalid: %w", parseErr)
		}
		adjustSideAndLimit = &parsed
	}

	var session *string
	if raw := strings.TrimSpace(query.Session); raw != "" {
		session = &raw
	}

	var positionID *uint64
	if raw := strings.TrimSpace(query.PositionID); raw != "" {
		parsed, parseErr := strconv.ParseUint(raw, 10, 64)
		if parseErr != nil {
			return broker.MaxTradeQuantityQuery{}, fmt.Errorf("query parameter positionId is invalid: %w", parseErr)
		}
		positionID = &parsed
	}

	return broker.MaxTradeQuantityQuery{
		ReadQuery:          s.brokerReadQuery(query.brokerBaseReadQuery),
		Symbol:             symbol,
		OrderType:          orderType,
		Price:              price,
		OrderIDEx:          strings.TrimSpace(query.OrderIDEx),
		AdjustSideAndLimit: adjustSideAndLimit,
		Session:            session,
		PositionID:         positionID,
	}, nil
}

func (s *Server) brokerMaxTradeQuantityResponse(ctx context.Context, query broker.MaxTradeQuantityQuery) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("maxTradeQuantity", nil, fmt.Errorf("broker market data not available"))
	}
	snapshot, err := reader.QueryMaxTradeQuantity(ctx, query)
	if err != nil {
		return brokerReadErrorResponse("maxTradeQuantity", nil, err)
	}

	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"maxTradeQuantity": map[string]any{
			"accountId":           snapshot.AccountID,
			"tradingEnvironment":  snapshot.TradingEnvironment,
			"market":              snapshot.Market,
			"symbol":              snapshot.Symbol,
			"orderType":           snapshot.OrderType,
			"price":               snapshot.Price,
			"maxCashBuy":          snapshot.MaxCashBuy,
			"maxCashAndMarginBuy": snapshot.MaxCashAndMarginBuy,
			"maxPositionSell":     snapshot.MaxPositionSell,
			"maxSellShort":        snapshot.MaxSellShort,
			"maxBuyBack":          snapshot.MaxBuyBack,
			"longRequiredIM":      snapshot.LongRequiredIM,
			"shortRequiredIM":     snapshot.ShortRequiredIM,
			"session":             snapshot.Session,
		},
	}
}

func normalizeBrokerReadScope(value string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "", "CURRENT":
		return "CURRENT", nil
	case "HISTORY":
		return "HISTORY", nil
	default:
		return "", fmt.Errorf("query parameter scope is invalid")
	}
}

func mergeBrokerQueryValues(groups ...[]string) []string {
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, group := range groups {
		for _, raw := range group {
			for _, part := range strings.Split(raw, ",") {
				trimmed := strings.TrimSpace(part)
				if trimmed == "" {
					continue
				}
				normalized := strings.ToUpper(trimmed)
				if _, ok := seen[normalized]; ok {
					continue
				}
				seen[normalized] = struct{}{}
				values = append(values, trimmed)
			}
		}
	}
	return values
}

func brokerReadErrorResponse(key string, value any, err error, extraKeys ...string) map[string]any {
	result := map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": connectivityFromBrokerReadError(err),
		"lastError":    err.Error(),
		key:            value,
	}
	for _, extraKey := range extraKeys {
		result[extraKey] = []any{}
	}
	return result
}

func connectivityFromBrokerReadError(err error) string {
	if err == nil {
		return "connected"
	}
	lower := strings.ToLower(err.Error())
	for _, marker := range []string{"connection refused", "dial ", "i/o timeout", "timeout", "client closed", "broken pipe", "connection reset", "eof", "unavailable"} {
		if strings.Contains(lower, marker) {
			return "disconnected"
		}
	}
	return "degraded"
}

// --- New write-side handlers ---

func (s *Server) brokerTradingService() (broker.TradingService, int, string, string) {
	activeBroker := s.activeBroker()
	if activeBroker == nil {
		return nil, http.StatusServiceUnavailable, "NO_BROKER", "no active broker"
	}
	trading := activeBroker.Trading()
	if trading == nil {
		return nil, http.StatusServiceUnavailable, "NO_TRADING", "broker does not support trading"
	}
	return trading, 0, "", ""
}

func (s *Server) brokerUnlockTrader() (broker.UnlockTrader, int, string, string) {
	activeBroker := s.activeBroker()
	if activeBroker == nil {
		return nil, http.StatusServiceUnavailable, "NO_BROKER", "no active broker"
	}
	unlocker, ok := activeBroker.(broker.UnlockTrader)
	if !ok {
		return nil, http.StatusServiceUnavailable, "NOT_SUPPORTED", "broker does not support trade unlock"
	}
	return unlocker, 0, "", ""
}

func (s *Server) handlePlaceOrder(c *gin.Context, query broker.ReadQuery) {
	var body brokerPlaceOrderRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}
	req := broker.PlaceOrderQuery{
		ReadQuery:      query,
		Symbol:         body.Symbol,
		Side:           body.Side,
		OrderType:      body.OrderType,
		Price:          body.Price,
		Quantity:       body.Quantity,
		TimeInForce:    body.TimeInForce,
		ClientOrderID:  body.ClientOrderID,
		Remark:         body.Remark,
		Session:        body.Session,
		FillOutsideRTH: body.FillOutsideRTH,
	}

	trading, status, code, message := s.brokerTradingService()
	if trading == nil {
		s.writeError(c, status, code, message)
		return
	}
	result, err := trading.PlaceOrder(c.Request.Context(), req)
	if err != nil {
		s.writeError(c, http.StatusBadGateway, "PLACE_ORDER_FAILED", err.Error())
		return
	}
	s.writeOK(c, map[string]any{
		"placedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"order":    result,
	})
}

func (s *Server) handleCancelOrders(c *gin.Context, query broker.ReadQuery) {
	var req brokerCancelOrdersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}

	trading, status, code, message := s.brokerTradingService()
	if trading == nil {
		s.writeError(c, status, code, message)
		return
	}
	orders := make([]broker.CancelOrder, len(req.Orders))
	for i, o := range req.Orders {
		orders[i] = broker.CancelOrder{
			OrderID:       o.OrderID,
			BrokerOrderID: o.BrokerOrderID,
			Symbol:        o.Symbol,
		}
	}
	if err := trading.CancelOrders(c.Request.Context(), query, orders...); err != nil {
		s.writeError(c, http.StatusBadGateway, "CANCEL_FAILED", err.Error())
		return
	}
	s.writeOK(c, map[string]any{
		"cancelledAt": time.Now().UTC().Format(time.RFC3339Nano),
		"cancelled":   len(orders),
	})
}

func (s *Server) handleUnlockTrade(c *gin.Context, query broker.ReadQuery) {
	var body brokerUnlockTradeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		s.writeError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}
	req := broker.UnlockTradeRequest{
		ReadQuery:   query,
		Unlock:      body.Unlock,
		PasswordMD5: body.PasswordMD5,
	}

	unlocker, status, code, message := s.brokerUnlockTrader()
	if unlocker == nil {
		s.writeError(c, status, code, message)
		return
	}
	if err := unlocker.UnlockTrade(c.Request.Context(), req); err != nil {
		s.writeError(c, http.StatusBadGateway, "UNLOCK_FAILED", err.Error())
		return
	}
	s.writeOK(c, map[string]any{
		"unlockedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"unlocked":   true,
	})
}

// --- New read-side handlers ---

func (s *Server) brokerQuoteRequest(query brokerQuoteReadQuery) (broker.QuoteQuery, error) {
	symbols := mergeBrokerQueryValues(query.Symbol, query.Symbols)
	if len(symbols) == 0 {
		return broker.QuoteQuery{}, fmt.Errorf("query parameter symbol is required")
	}
	return broker.QuoteQuery{
		ReadQuery: s.brokerReadQuery(query.brokerBaseReadQuery),
		Symbols:   symbols,
	}, nil
}

func (s *Server) brokerQuoteResponse(ctx context.Context, query broker.QuoteQuery) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("quotes", []any{}, fmt.Errorf("broker market data not available"))
	}
	quote, err := reader.QueryQuote(ctx, query)
	if err != nil {
		return brokerReadErrorResponse("quotes", []any{}, err)
	}
	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"quote":        quote,
	}
}

func (s *Server) brokerKLinesRequest(query brokerKLinesReadQuery) (broker.KLineQuery, error) {
	symbol := strings.TrimSpace(query.Symbol)
	if symbol == "" {
		return broker.KLineQuery{}, fmt.Errorf("query parameter symbol is required")
	}
	period := strings.TrimSpace(query.Period)
	if period == "" {
		period = "1d"
	}

	var limit int32
	if raw := strings.TrimSpace(query.Limit); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 32)
		if err != nil {
			return broker.KLineQuery{}, fmt.Errorf("query parameter limit is invalid: %w", err)
		}
		limit = int32(parsed)
	}

	return broker.KLineQuery{
		ReadQuery: s.brokerReadQuery(query.brokerBaseReadQuery),
		Symbol:    symbol,
		Period:    period,
		FromTime:  strings.TrimSpace(query.FromTime),
		ToTime:    strings.TrimSpace(query.ToTime),
		Limit:     limit,
	}, nil
}

func (s *Server) brokerKLinesResponse(ctx context.Context, query broker.KLineQuery) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("klines", []any{}, fmt.Errorf("broker market data not available"))
	}
	snapshot, err := reader.QueryKLines(ctx, query)
	if err != nil {
		return brokerReadErrorResponse("klines", []any{}, err)
	}
	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"klines":       snapshot,
	}
}

func (s *Server) brokerSecuritiesRequest(query brokerSecuritiesReadQuery) (broker.SecuritySnapshotQuery, error) {
	symbols := mergeBrokerQueryValues(query.Symbol, query.Symbols)
	if len(symbols) == 0 {
		return broker.SecuritySnapshotQuery{}, fmt.Errorf("query parameter symbol is required")
	}
	return broker.SecuritySnapshotQuery{
		ReadQuery: s.brokerReadQuery(query.brokerBaseReadQuery),
		Symbols:   symbols,
	}, nil
}

func (s *Server) brokerSecuritiesSnapshotResponse(ctx context.Context, query broker.SecuritySnapshotQuery) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("securities", []any{}, fmt.Errorf("broker market data not available"))
	}
	result, err := reader.QuerySecuritySnapshot(ctx, query)
	if err != nil {
		return brokerReadErrorResponse("securities", []any{}, err)
	}
	return map[string]any{
		"checkedAt":    time.Now().UTC().Format(time.RFC3339Nano),
		"connectivity": "connected",
		"lastError":    nil,
		"securities":   result,
	}
}
