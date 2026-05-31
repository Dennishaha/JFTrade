package jftradeapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
)

func (s *Server) serveBrokerRoutes(w http.ResponseWriter, r *http.Request) bool {
	brokerID, resource, ok := parseBrokerRoute(r.URL.Path)
	if !ok {
		return false
	}

	// Look up the broker by ID from the registry.
	activeBroker := s.activeBroker()
	if activeBroker == nil || activeBroker.ID() != brokerID {
		// For backward compatibility, only "futu" is supported currently.
		if brokerID != "futu" {
			return false
		}
	}

	query := s.brokerReadQueryFromRequest(r)

	// Handle write-side methods
	if r.Method == http.MethodPost || r.Method == http.MethodDelete {
		return s.serveBrokerWriteRoutes(w, r, resource, query)
	}

	if r.Method != http.MethodGet {
		return false
	}

	switch resource {
	case "runtime":
		s.writeOK(w, s.brokerRuntime(r.Context()))
	case "funds":
		s.writeOK(w, s.brokerFundsResponse(r.Context(), query))
	case "positions":
		s.writeOK(w, s.brokerPositionsResponse(r.Context(), query))
	case "orders":
		s.writeOK(w, s.brokerOrdersResponse(r.Context(), query, r.URL.Query()))
	case "fills":
		s.writeOK(w, s.brokerFillsResponse(r.Context(), query, r.URL.Query()))
	case "cash-flows":
		s.writeOK(w, s.brokerCashFlowsResponse(r.Context(), query, r.URL.Query()))
	case "order-fees":
		s.writeOK(w, s.brokerOrderFeesResponse(r.Context(), query, r.URL.Query()))
	case "margin-ratios":
		s.writeOK(w, s.brokerMarginRatiosResponse(r.Context(), query, r.URL.Query()))
	case "max-trade-qtys":
		s.writeOK(w, s.brokerMaxTradeQuantityResponse(r.Context(), query, r.URL.Query()))
	case "quote":
		s.writeOK(w, s.brokerQuoteResponse(r.Context(), query, r.URL.Query()))
	case "klines":
		s.writeOK(w, s.brokerKLinesResponse(r.Context(), query, r.URL.Query()))
	case "securities":
		s.writeOK(w, s.brokerSecuritiesSnapshotResponse(r.Context(), query, r.URL.Query()))
	case "unlock":
		return false // unlock is POST-only
	default:
		return false
	}
	return true
}

func (s *Server) serveBrokerWriteRoutes(w http.ResponseWriter, r *http.Request, resource string, query broker.ReadQuery) bool {
	switch {
	case resource == "orders" && r.Method == http.MethodPost:
		s.handlePlaceOrder(w, r, query)
		return true
	case resource == "orders" && r.Method == http.MethodDelete:
		s.handleCancelOrders(w, r, query)
		return true
	case resource == "unlock" && r.Method == http.MethodPost:
		s.handleUnlockTrade(w, r, query)
		return true
	default:
		return false
	}
}

func parseBrokerRoute(path string) (brokerID string, resource string, ok bool) {
	if !strings.HasPrefix(path, "/api/v1/brokers/") {
		return "", "", false
	}
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/v1/brokers/"), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	if strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func (s *Server) brokerReadQueryFromRequest(r *http.Request) broker.ReadQuery {
	query := r.URL.Query()
	fallbackMarket := strings.TrimSpace(s.store.integration().Config.TradeMarket)
	if fallbackMarket == "" {
		fallbackMarket = "HK"
	}
	market := strings.TrimSpace(query.Get("market"))
	if market == "" {
		market = fallbackMarket
	}
	return broker.ReadQuery{
		BrokerID:           "futu",
		TradingEnvironment: strings.TrimSpace(query.Get("tradingEnvironment")),
		AccountID:          strings.TrimSpace(query.Get("accountId")),
		Market:             market,
	}
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

func (s *Server) brokerOrdersResponse(ctx context.Context, query broker.ReadQuery, params url.Values) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("orders", []any{}, fmt.Errorf("broker market data not available"))
	}

	scope := strings.ToUpper(strings.TrimSpace(params.Get("scope")))
	symbol := strings.TrimSpace(params.Get("symbol"))
	var (
		orders []broker.OrderSnapshot
		err    error
	)
	if scope == "HISTORY" {
		orders, err = reader.QueryHistoryOrders(ctx, broker.OrderHistoryQuery{
			ReadQuery: query,
			Symbol:    symbol,
			StartTime: strings.TrimSpace(params.Get("startTime")),
			EndTime:   strings.TrimSpace(params.Get("endTime")),
			Statuses:  queryListValues(params, "status", "statuses"),
		})
	} else {
		orders, err = reader.QueryOrders(ctx, query, symbol)
	}
	if err != nil {
		return brokerReadErrorResponse("orders", []any{}, err)
	}

	entries := make([]any, 0, len(orders))
	for _, order := range orders {
		entries = append(entries, map[string]any{
			"accountId":          order.AccountID,
			"tradingEnvironment": order.TradingEnvironment,
			"market":             order.Market,
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

func (s *Server) brokerFillsResponse(ctx context.Context, query broker.ReadQuery, params url.Values) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("fills", []any{}, fmt.Errorf("broker market data not available"))
	}

	scope := strings.ToUpper(strings.TrimSpace(params.Get("scope")))
	fillQuery := broker.OrderFillQuery{
		ReadQuery: query,
		Symbol:    strings.TrimSpace(params.Get("symbol")),
		StartTime: strings.TrimSpace(params.Get("startTime")),
		EndTime:   strings.TrimSpace(params.Get("endTime")),
	}
	var (
		fills []broker.OrderFillSnapshot
		err   error
	)
	if scope == "HISTORY" {
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

func (s *Server) brokerCashFlowsResponse(ctx context.Context, query broker.ReadQuery, params url.Values) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("cashFlows", []any{}, fmt.Errorf("broker market data not available"))
	}

	clearingDate := strings.TrimSpace(params.Get("clearingDate"))
	if clearingDate == "" {
		return brokerReadErrorResponse("cashFlows", []any{}, fmt.Errorf("query parameter clearingDate is required"))
	}
	flows, err := reader.QueryCashFlows(ctx, broker.CashFlowQuery{
		ReadQuery:    query,
		ClearingDate: clearingDate,
		Direction:    strings.TrimSpace(params.Get("direction")),
	})
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

func (s *Server) brokerOrderFeesResponse(ctx context.Context, query broker.ReadQuery, params url.Values) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("fees", []any{}, fmt.Errorf("broker market data not available"))
	}

	orderIDExList := queryListValues(params, "orderIdEx", "orderIdExList")
	if len(orderIDExList) == 0 {
		return brokerReadErrorResponse("fees", []any{}, fmt.Errorf("query parameter orderIdEx is required"))
	}
	fees, err := reader.QueryOrderFees(ctx, broker.OrderFeeQuery{ReadQuery: query, OrderIDExList: orderIDExList})
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

func (s *Server) brokerMarginRatiosResponse(ctx context.Context, query broker.ReadQuery, params url.Values) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("marginRatios", []any{}, fmt.Errorf("broker market data not available"))
	}

	rawSymbols := queryListValues(params, "symbol", "symbols")
	symbols := make([]string, 0, len(rawSymbols))
	for _, symbol := range rawSymbols {
		instrument, err := normalizeInstrumentInput(query.Market, symbol, "")
		if err != nil {
			return brokerReadErrorResponse("marginRatios", []any{}, fmt.Errorf("query parameter symbol %q is invalid: %w", symbol, err))
		}
		symbols = append(symbols, instrument.Symbol)
	}
	if len(symbols) == 0 {
		return brokerReadErrorResponse("marginRatios", []any{}, fmt.Errorf("query parameter symbol is required"))
	}
	ratios, err := reader.QueryMarginRatios(ctx, broker.MarginRatioQuery{ReadQuery: query, Symbols: symbols})
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

func (s *Server) brokerMaxTradeQuantityResponse(ctx context.Context, query broker.ReadQuery, params url.Values) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("maxTradeQuantity", nil, fmt.Errorf("broker market data not available"))
	}

	symbol := strings.TrimSpace(params.Get("symbol"))
	orderType := strings.TrimSpace(params.Get("orderType"))
	priceValue := strings.TrimSpace(params.Get("price"))
	if symbol == "" || orderType == "" || priceValue == "" {
		return brokerReadErrorResponse("maxTradeQuantity", nil, fmt.Errorf("query parameters symbol, orderType, and price are required"))
	}
	price, err := strconv.ParseFloat(priceValue, 64)
	if err != nil {
		return brokerReadErrorResponse("maxTradeQuantity", nil, fmt.Errorf("query parameter price is invalid: %w", err))
	}

	var adjustSideAndLimit *float64
	if raw := strings.TrimSpace(params.Get("adjustSideAndLimit")); raw != "" {
		parsed, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil {
			return brokerReadErrorResponse("maxTradeQuantity", nil, fmt.Errorf("query parameter adjustSideAndLimit is invalid: %w", parseErr))
		}
		adjustSideAndLimit = &parsed
	}

	var session *string
	if raw := strings.TrimSpace(params.Get("session")); raw != "" {
		session = &raw
	}

	var positionID *uint64
	if raw := strings.TrimSpace(params.Get("positionId")); raw != "" {
		parsed, parseErr := strconv.ParseUint(raw, 10, 64)
		if parseErr != nil {
			return brokerReadErrorResponse("maxTradeQuantity", nil, fmt.Errorf("query parameter positionId is invalid: %w", parseErr))
		}
		positionID = &parsed
	}

	snapshot, err := reader.QueryMaxTradeQuantity(ctx, broker.MaxTradeQuantityQuery{
		ReadQuery:          query,
		Symbol:             symbol,
		OrderType:          orderType,
		Price:              price,
		OrderIDEx:          strings.TrimSpace(params.Get("orderIdEx")),
		AdjustSideAndLimit: adjustSideAndLimit,
		Session:            session,
		PositionID:         positionID,
	})
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

func queryListValues(params url.Values, keys ...string) []string {
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, key := range keys {
		for _, raw := range params[key] {
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

func (s *Server) handlePlaceOrder(w http.ResponseWriter, r *http.Request, query broker.ReadQuery) {
	var req broker.PlaceOrderQuery
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}
	req.ReadQuery = query

	activeBroker := s.activeBroker()
	if activeBroker == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NO_BROKER", "no active broker")
		return
	}
	trading := activeBroker.Trading()
	if trading == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NO_TRADING", "broker does not support trading")
		return
	}
	result, err := trading.PlaceOrder(r.Context(), req)
	if err != nil {
		s.writeError(w, http.StatusBadGateway, "PLACE_ORDER_FAILED", err.Error())
		return
	}
	s.writeOK(w, map[string]any{
		"placedAt": time.Now().UTC().Format(time.RFC3339Nano),
		"order":    result,
	})
}

func (s *Server) handleCancelOrders(w http.ResponseWriter, r *http.Request, query broker.ReadQuery) {
	var req struct {
		Orders []struct {
			OrderID       uint64 `json:"orderId"`
			BrokerOrderID string `json:"brokerOrderId"`
			Symbol        string `json:"symbol"`
		} `json:"orders"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}

	activeBroker := s.activeBroker()
	if activeBroker == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NO_BROKER", "no active broker")
		return
	}
	trading := activeBroker.Trading()
	if trading == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NO_TRADING", "broker does not support trading")
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
	if err := trading.CancelOrders(r.Context(), query, orders...); err != nil {
		s.writeError(w, http.StatusBadGateway, "CANCEL_FAILED", err.Error())
		return
	}
	s.writeOK(w, map[string]any{
		"cancelledAt": time.Now().UTC().Format(time.RFC3339Nano),
		"cancelled":   len(orders),
	})
}

func (s *Server) handleUnlockTrade(w http.ResponseWriter, r *http.Request, query broker.ReadQuery) {
	var req broker.UnlockTradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "BAD_REQUEST", "invalid request body: "+err.Error())
		return
	}
	req.ReadQuery = query

	activeBroker := s.activeBroker()
	if activeBroker == nil {
		s.writeError(w, http.StatusServiceUnavailable, "NO_BROKER", "no active broker")
		return
	}
	if unlocker, ok := activeBroker.(broker.UnlockTrader); ok {
		if err := unlocker.UnlockTrade(r.Context(), req); err != nil {
			s.writeError(w, http.StatusBadGateway, "UNLOCK_FAILED", err.Error())
			return
		}
		s.writeOK(w, map[string]any{
			"unlockedAt": time.Now().UTC().Format(time.RFC3339Nano),
			"unlocked":   true,
		})
		return
	}
	s.writeError(w, http.StatusServiceUnavailable, "NOT_SUPPORTED", "broker does not support trade unlock")
}

// --- New read-side handlers ---

func (s *Server) brokerQuoteResponse(ctx context.Context, query broker.ReadQuery, params url.Values) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("quotes", []any{}, fmt.Errorf("broker market data not available"))
	}
	symbols := queryListValues(params, "symbol", "symbols")
	if len(symbols) == 0 {
		return brokerReadErrorResponse("quotes", []any{}, fmt.Errorf("query parameter symbol is required"))
	}
	quote, err := reader.QueryQuote(ctx, broker.QuoteQuery{
		ReadQuery: query,
		Symbols:   symbols,
	})
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

func (s *Server) brokerKLinesResponse(ctx context.Context, query broker.ReadQuery, params url.Values) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("klines", []any{}, fmt.Errorf("broker market data not available"))
	}
	symbol := strings.TrimSpace(params.Get("symbol"))
	if symbol == "" {
		return brokerReadErrorResponse("klines", []any{}, fmt.Errorf("query parameter symbol is required"))
	}
	period := strings.TrimSpace(params.Get("period"))
	if period == "" {
		period = "1d"
	}
	fromTime := strings.TrimSpace(params.Get("fromTime"))
	toTime := strings.TrimSpace(params.Get("toTime"))
	limitStr := strings.TrimSpace(params.Get("limit"))
	var limit int32
	if limitStr != "" {
		if parsed, err := strconv.ParseInt(limitStr, 10, 32); err == nil {
			limit = int32(parsed)
		}
	}
	snapshot, err := reader.QueryKLines(ctx, broker.KLineQuery{
		ReadQuery: query,
		Symbol:    symbol,
		Period:    period,
		FromTime:  fromTime,
		ToTime:    toTime,
		Limit:     limit,
	})
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

func (s *Server) brokerSecuritiesSnapshotResponse(ctx context.Context, query broker.ReadQuery, params url.Values) map[string]any {
	reader := s.brokerMarketDataReader()
	if reader == nil {
		return brokerReadErrorResponse("securities", []any{}, fmt.Errorf("broker market data not available"))
	}
	symbols := queryListValues(params, "symbol", "symbols")
	if len(symbols) == 0 {
		return brokerReadErrorResponse("securities", []any{}, fmt.Errorf("query parameter symbol is required"))
	}
	result, err := reader.QuerySecuritySnapshot(ctx, broker.SecuritySnapshotQuery{
		ReadQuery: query,
		Symbols:   symbols,
	})
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
