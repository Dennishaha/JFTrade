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

// --- Trade write-side handlers ---

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

// --- Read-side handlers ---

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
