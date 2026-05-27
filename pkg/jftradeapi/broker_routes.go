package jftradeapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/futu"
)

func (s *Server) serveBrokerRoutes(w http.ResponseWriter, r *http.Request) bool {
	brokerID, resource, ok := parseBrokerRoute(r.URL.Path)
	if !ok || r.Method != http.MethodGet {
		return false
	}
	if brokerID != "futu" {
		return false
	}

	query := s.brokerReadQueryFromRequest(r)
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
	default:
		return false
	}
	return true
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

func (s *Server) brokerReadQueryFromRequest(r *http.Request) futu.BrokerReadQuery {
	query := r.URL.Query()
	fallbackMarket := strings.TrimSpace(s.store.integration().Config.TradeMarket)
	if fallbackMarket == "" {
		fallbackMarket = "HK"
	}
	market := strings.TrimSpace(query.Get("market"))
	if market == "" {
		market = fallbackMarket
	}
	return futu.BrokerReadQuery{
		TradingEnvironment: strings.TrimSpace(query.Get("tradingEnvironment")),
		AccountID:          strings.TrimSpace(query.Get("accountId")),
		Market:             market,
	}
}

func (s *Server) brokerFundsResponse(ctx context.Context, query futu.BrokerReadQuery) map[string]any {
	snapshot, err := s.futuExchange().QueryBrokerFunds(ctx, query)
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
		},
		"currencyBalances": currencyBalances,
		"marketAssets":     marketAssets,
	}
}

func (s *Server) brokerPositionsResponse(ctx context.Context, query futu.BrokerReadQuery) map[string]any {
	positions, err := s.futuExchange().QueryBrokerPositions(ctx, query)
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

func (s *Server) brokerOrdersResponse(ctx context.Context, query futu.BrokerReadQuery, params url.Values) map[string]any {
	scope := strings.ToUpper(strings.TrimSpace(params.Get("scope")))
	symbol := strings.TrimSpace(params.Get("symbol"))
	var (
		orders []futu.BrokerOrderSnapshot
		err    error
	)
	if scope == "HISTORY" {
		orders, err = s.futuExchange().QueryBrokerHistoryOrders(ctx, futu.BrokerOrderHistoryQuery{
			BrokerReadQuery: query,
			Symbol:          symbol,
			StartTime:       strings.TrimSpace(params.Get("startTime")),
			EndTime:         strings.TrimSpace(params.Get("endTime")),
			Statuses:        queryListValues(params, "status", "statuses"),
		})
	} else {
		orders, err = s.futuExchange().QueryBrokerOrders(ctx, query, symbol)
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

func (s *Server) brokerFillsResponse(ctx context.Context, query futu.BrokerReadQuery, params url.Values) map[string]any {
	scope := strings.ToUpper(strings.TrimSpace(params.Get("scope")))
	fillQuery := futu.BrokerOrderFillQuery{
		BrokerReadQuery: query,
		Symbol:          strings.TrimSpace(params.Get("symbol")),
		StartTime:       strings.TrimSpace(params.Get("startTime")),
		EndTime:         strings.TrimSpace(params.Get("endTime")),
	}
	var (
		fills []futu.BrokerOrderFillSnapshot
		err   error
	)
	if scope == "HISTORY" {
		fills, err = s.futuExchange().QueryBrokerHistoryOrderFills(ctx, futu.BrokerOrderFillHistoryQuery(fillQuery))
	} else {
		fills, err = s.futuExchange().QueryBrokerOrderFills(ctx, fillQuery)
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

func (s *Server) brokerCashFlowsResponse(ctx context.Context, query futu.BrokerReadQuery, params url.Values) map[string]any {
	clearingDate := strings.TrimSpace(params.Get("clearingDate"))
	if clearingDate == "" {
		return brokerReadErrorResponse("cashFlows", []any{}, fmt.Errorf("query parameter clearingDate is required"))
	}
	flows, err := s.futuExchange().QueryBrokerCashFlows(ctx, futu.BrokerCashFlowQuery{
		BrokerReadQuery: query,
		ClearingDate:    clearingDate,
		Direction:       strings.TrimSpace(params.Get("direction")),
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

func (s *Server) brokerOrderFeesResponse(ctx context.Context, query futu.BrokerReadQuery, params url.Values) map[string]any {
	orderIDExList := queryListValues(params, "orderIdEx", "orderIdExList")
	if len(orderIDExList) == 0 {
		return brokerReadErrorResponse("fees", []any{}, fmt.Errorf("query parameter orderIdEx is required"))
	}
	fees, err := s.futuExchange().QueryBrokerOrderFees(ctx, futu.BrokerOrderFeeQuery{BrokerReadQuery: query, OrderIDExList: orderIDExList})
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

func (s *Server) brokerMarginRatiosResponse(ctx context.Context, query futu.BrokerReadQuery, params url.Values) map[string]any {
	symbols := queryListValues(params, "symbol", "symbols")
	if len(symbols) == 0 {
		return brokerReadErrorResponse("marginRatios", []any{}, fmt.Errorf("query parameter symbol is required"))
	}
	ratios, err := s.futuExchange().QueryBrokerMarginRatios(ctx, futu.BrokerMarginRatioQuery{BrokerReadQuery: query, Symbols: symbols})
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

func (s *Server) brokerMaxTradeQuantityResponse(ctx context.Context, query futu.BrokerReadQuery, params url.Values) map[string]any {
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

	snapshot, err := s.futuExchange().QueryBrokerMaxTradeQuantity(ctx, futu.BrokerMaxTradeQuantityQuery{
		BrokerReadQuery:    query,
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
