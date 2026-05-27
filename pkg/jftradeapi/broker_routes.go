package jftradeapi

import (
	"context"
	"net/http"
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
		s.writeOK(w, s.brokerOrdersResponse(r.Context(), query))
	case "cash-flows":
		s.writeOK(w, s.emptyConnectivityList("cashFlows", []any{}))
	case "order-fees":
		s.writeOK(w, s.emptyConnectivityList("fees", []any{}))
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

func (s *Server) brokerOrdersResponse(ctx context.Context, query futu.BrokerReadQuery) map[string]any {
	orders, err := s.futuExchange().QueryBrokerOrders(ctx, query, "")
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
