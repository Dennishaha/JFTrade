package trading

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/market"
)

var (
	ErrBrokerNotFound     = errors.New("broker not found")
	ErrNoBroker           = errors.New("no active broker")
	ErrTradingUnsupported = errors.New("broker does not support trading")
	ErrUnlockUnsupported  = errors.New("broker does not support trade unlock")
)

type OrdersQuery struct {
	broker.ReadQuery
	Scope     string
	Symbol    string
	StartTime string
	EndTime   string
	Statuses  []string
}

type FillsQuery struct {
	broker.ReadQuery
	Scope     string
	Symbol    string
	StartTime string
	EndTime   string
}

func (s *Service) ReadQuery(brokerID, tradingEnvironment, accountID, marketCode string) broker.ReadQuery {
	marketCode = strings.TrimSpace(marketCode)
	if marketCode == "" && s.defaultMarket != nil {
		marketCode = strings.TrimSpace(s.defaultMarket())
	}
	if marketCode == "" {
		marketCode = "HK"
	}
	return broker.ReadQuery{
		BrokerID:           strings.TrimSpace(brokerID),
		TradingEnvironment: strings.TrimSpace(tradingEnvironment),
		AccountID:          strings.TrimSpace(accountID),
		Market:             marketCode,
	}
}

func NormalizeSymbols(marketCode string, symbols []string) ([]string, error) {
	normalized := make([]string, 0, len(symbols))
	for _, symbol := range symbols {
		instrument, err := market.ParseInstrument(market.InstrumentInput{
			Market: marketCode,
			Symbol: symbol,
		})
		if err != nil {
			return nil, fmt.Errorf("query parameter symbol %q is invalid: %w", symbol, err)
		}
		normalized = append(normalized, instrument.Symbol)
	}
	return normalized, nil
}

func (s *Service) Runtime(ctx context.Context, brokerID string) (map[string]any, error) {
	if _, err := s.resolveBroker(brokerID, false); err != nil {
		return nil, err
	}
	if s.brokerRuntime == nil {
		return map[string]any{}, nil
	}
	return s.brokerRuntime.Runtime(ctx), nil
}

func (s *Service) Funds(ctx context.Context, query broker.ReadQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return readErrorResponse("summary", nil, errors.New("broker market data not available"), "currencyBalances", "marketAssets"), nil
	}
	snapshot, readErr := reader.QueryFunds(ctx, query)
	if readErr != nil {
		return readErrorResponse("summary", nil, readErr, "currencyBalances", "marketAssets"), nil
	}

	currencyBalances := make([]any, 0, len(snapshot.CurrencyBalances))
	for _, balance := range snapshot.CurrencyBalances {
		currencyBalances = append(currencyBalances, map[string]any{
			"accountId": balance.AccountID, "tradingEnvironment": balance.TradingEnvironment,
			"currency": balance.Currency, "cash": balance.Cash,
			"availableWithdrawalCash": balance.AvailableWithdrawalCash, "netCashPower": balance.NetCashPower,
		})
	}
	marketAssets := make([]any, 0, len(snapshot.MarketAssets))
	for _, asset := range snapshot.MarketAssets {
		marketAssets = append(marketAssets, map[string]any{
			"accountId": asset.AccountID, "tradingEnvironment": asset.TradingEnvironment,
			"market": asset.Market, "assets": asset.Assets,
		})
	}
	return connectedResponse(map[string]any{
		"summary": map[string]any{
			"accountId": snapshot.AccountID, "tradingEnvironment": snapshot.TradingEnvironment,
			"market": snapshot.Market, "currency": snapshot.Currency, "totalAssets": snapshot.TotalAssets,
			"securitiesAssets": snapshot.SecuritiesAssets, "fundAssets": snapshot.FundAssets,
			"bondAssets": snapshot.BondAssets, "cash": snapshot.Cash, "marketValue": snapshot.MarketValue,
			"longMarketValue": snapshot.LongMarketValue, "shortMarketValue": snapshot.ShortMarketValue,
			"purchasingPower": snapshot.PurchasingPower, "shortSellingPower": snapshot.ShortSellingPower,
			"netCashPower": snapshot.NetCashPower, "availableWithdrawalCash": snapshot.AvailableWithdrawalCash,
			"maxWithdrawal": snapshot.MaxWithdrawal, "availableFunds": snapshot.AvailableFunds,
			"frozenCash": snapshot.FrozenCash, "pendingAsset": snapshot.PendingAsset,
			"unrealizedPnl": snapshot.UnrealizedPnl, "realizedPnl": snapshot.RealizedPnl,
			"initialMargin": snapshot.InitialMargin, "maintenanceMargin": snapshot.MaintenanceMargin,
			"marginCallMargin": snapshot.MarginCallMargin, "riskStatus": snapshot.RiskStatus,
			"debtCash": snapshot.DebtCash, "isPdt": snapshot.IsPDT, "pdtSeq": snapshot.PDTSeq,
			"beginningDTBP": snapshot.BeginningDTBP, "remainingDTBP": snapshot.RemainingDTBP,
			"dtCallAmount": snapshot.DTCallAmount, "dtStatus": snapshot.DTStatus,
			"exposureLevel": snapshot.ExposureLevel, "exposureLimit": snapshot.ExposureLimit,
			"usedLimit": snapshot.UsedLimit, "remainingLimit": snapshot.RemainingLimit,
		},
		"currencyBalances": currencyBalances,
		"marketAssets":     marketAssets,
	}), nil
}

func (s *Service) Positions(ctx context.Context, query broker.ReadQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return readErrorResponse("positions", []any{}, errors.New("broker market data not available")), nil
	}
	positions, readErr := reader.QueryPositions(ctx, query)
	if readErr != nil {
		return readErrorResponse("positions", []any{}, readErr), nil
	}
	entries := make([]any, 0, len(positions))
	for _, position := range positions {
		entries = append(entries, map[string]any{
			"accountId": position.AccountID, "tradingEnvironment": position.TradingEnvironment,
			"market": position.Market, "symbol": position.Symbol, "symbolName": position.SymbolName,
			"quantity": position.Quantity, "sellableQuantity": position.SellableQuantity,
			"lastPrice": position.LastPrice, "costPrice": position.CostPrice,
			"averageCostPrice": position.AverageCostPrice, "marketValue": position.MarketValue,
			"unrealizedPnl": position.UnrealizedPnl, "realizedPnl": position.RealizedPnl,
			"pnlRatio": position.PnlRatio, "currency": position.Currency,
		})
	}
	return connectedResponse(map[string]any{"positions": entries}), nil
}

func (s *Service) Orders(ctx context.Context, query OrdersQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return readErrorResponse("orders", []any{}, errors.New("broker market data not available")), nil
	}
	var orders []broker.OrderSnapshot
	if query.Scope == "HISTORY" {
		orders, err = reader.QueryHistoryOrders(ctx, broker.OrderHistoryQuery{
			ReadQuery: query.ReadQuery, Symbol: query.Symbol, StartTime: query.StartTime,
			EndTime: query.EndTime, Statuses: query.Statuses,
		})
	} else {
		orders, err = reader.QueryOrders(ctx, query.ReadQuery, query.Symbol)
	}
	if err != nil {
		return readErrorResponse("orders", []any{}, err), nil
	}
	entries := make([]any, 0, len(orders))
	for _, order := range orders {
		entries = append(entries, map[string]any{
			"accountId":          firstNonEmpty(order.AccountID, query.AccountID),
			"tradingEnvironment": firstNonEmpty(order.TradingEnvironment, query.TradingEnvironment),
			"market":             firstNonEmpty(order.Market, query.Market), "brokerOrderId": order.BrokerOrderID,
			"brokerOrderIdEx": order.BrokerOrderIDEx, "symbol": order.Symbol, "symbolName": order.SymbolName,
			"side": order.Side, "orderType": order.OrderType, "status": order.Status,
			"quantity": order.Quantity, "filledQuantity": order.FilledQuantity, "price": order.Price,
			"filledAveragePrice": order.FilledAveragePrice, "submittedAt": order.SubmittedAt,
			"updatedAt": order.UpdatedAt, "remark": order.Remark, "lastError": order.LastError,
			"timeInForce": order.TimeInForce, "currency": order.Currency,
		})
	}
	return connectedResponse(map[string]any{"orders": entries}), nil
}

func (s *Service) Fills(ctx context.Context, query FillsQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return readErrorResponse("fills", []any{}, errors.New("broker market data not available")), nil
	}
	fillQuery := broker.OrderFillQuery{
		ReadQuery: query.ReadQuery, Symbol: query.Symbol, StartTime: query.StartTime, EndTime: query.EndTime,
	}
	var fills []broker.OrderFillSnapshot
	if query.Scope == "HISTORY" {
		fills, err = reader.QueryHistoryOrderFills(ctx, broker.OrderFillHistoryQuery(fillQuery))
	} else {
		fills, err = reader.QueryOrderFills(ctx, fillQuery)
	}
	if err != nil {
		return readErrorResponse("fills", []any{}, err), nil
	}
	entries := make([]any, 0, len(fills))
	for _, fill := range fills {
		entries = append(entries, map[string]any{
			"accountId": fill.AccountID, "tradingEnvironment": fill.TradingEnvironment,
			"market": fill.Market, "brokerOrderId": fill.BrokerOrderID,
			"brokerOrderIdEx": fill.BrokerOrderIDEx, "brokerFillId": fill.BrokerFillID,
			"brokerFillIdEx": fill.BrokerFillIDEx, "symbol": fill.Symbol, "symbolName": fill.SymbolName,
			"side": fill.Side, "filledQuantity": fill.FilledQuantity, "fillPrice": fill.FillPrice,
			"filledAt": fill.FilledAt, "status": fill.Status,
		})
	}
	return connectedResponse(map[string]any{"fills": entries}), nil
}

func (s *Service) CashFlows(ctx context.Context, query broker.CashFlowQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return readErrorResponse("cashFlows", []any{}, errors.New("broker market data not available")), nil
	}
	flows, readErr := reader.QueryCashFlows(ctx, query)
	if readErr != nil {
		return readErrorResponse("cashFlows", []any{}, readErr), nil
	}
	entries := make([]any, 0, len(flows))
	for _, flow := range flows {
		entries = append(entries, flow)
	}
	return connectedResponse(map[string]any{"cashFlows": entries}), nil
}

func (s *Service) OrderFees(ctx context.Context, query broker.OrderFeeQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return readErrorResponse("fees", []any{}, errors.New("broker market data not available")), nil
	}
	fees, readErr := reader.QueryOrderFees(ctx, query)
	if readErr != nil {
		return readErrorResponse("fees", []any{}, readErr), nil
	}
	entries := make([]any, 0, len(fees))
	for _, fee := range fees {
		entries = append(entries, fee)
	}
	return connectedResponse(map[string]any{"fees": entries}), nil
}

func (s *Service) MarginRatios(ctx context.Context, query broker.MarginRatioQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return readErrorResponse("marginRatios", []any{}, errors.New("broker market data not available")), nil
	}
	ratios, readErr := reader.QueryMarginRatios(ctx, query)
	if readErr != nil {
		return readErrorResponse("marginRatios", []any{}, readErr), nil
	}
	entries := make([]any, 0, len(ratios))
	for _, ratio := range ratios {
		entries = append(entries, ratio)
	}
	return connectedResponse(map[string]any{"marginRatios": entries}), nil
}

func (s *Service) MaxTradeQuantity(ctx context.Context, query broker.MaxTradeQuantityQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return readErrorResponse("maxTradeQuantity", nil, errors.New("broker market data not available")), nil
	}
	snapshot, readErr := reader.QueryMaxTradeQuantity(ctx, query)
	if readErr != nil {
		return readErrorResponse("maxTradeQuantity", nil, readErr), nil
	}
	return connectedResponse(map[string]any{"maxTradeQuantity": snapshot}), nil
}

func (s *Service) Quote(ctx context.Context, query broker.QuoteQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return readErrorResponse("quotes", []any{}, errors.New("broker market data not available")), nil
	}
	quote, readErr := reader.QueryQuote(ctx, query)
	if readErr != nil {
		return readErrorResponse("quotes", []any{}, readErr), nil
	}
	return connectedResponse(map[string]any{"quote": quote}), nil
}

func (s *Service) KLines(ctx context.Context, query broker.KLineQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return readErrorResponse("klines", []any{}, errors.New("broker market data not available")), nil
	}
	snapshot, readErr := reader.QueryKLines(ctx, query)
	if readErr != nil {
		return readErrorResponse("klines", []any{}, readErr), nil
	}
	return connectedResponse(map[string]any{"klines": snapshot}), nil
}

func (s *Service) Securities(ctx context.Context, query broker.SecuritySnapshotQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return readErrorResponse("securities", []any{}, errors.New("broker market data not available")), nil
	}
	result, readErr := reader.QuerySecuritySnapshot(ctx, query)
	if readErr != nil {
		return readErrorResponse("securities", []any{}, readErr), nil
	}
	return connectedResponse(map[string]any{"securities": result}), nil
}

func (s *Service) PortfolioCashBalances(ctx context.Context, query broker.ReadQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return map[string]any{"balances": []any{}}, nil
	}
	snapshot, err := reader.QueryFunds(ctx, query)
	if err != nil {
		return map[string]any{"balances": []any{}}, nil
	}
	timestamp := now()
	balances := make([]any, 0, len(snapshot.CurrencyBalances))
	for _, balance := range snapshot.CurrencyBalances {
		balances = append(balances, map[string]any{
			"brokerId": query.BrokerID, "tradingEnvironment": balance.TradingEnvironment,
			"accountId": balance.AccountID, "currency": balance.Currency,
			"cashBalance": floatValue(balance.Cash), "updatedAt": timestamp, "createdAt": timestamp,
		})
	}
	if len(balances) == 0 && snapshot.Currency != nil {
		balances = append(balances, map[string]any{
			"brokerId": query.BrokerID, "tradingEnvironment": snapshot.TradingEnvironment,
			"accountId": snapshot.AccountID, "currency": *snapshot.Currency,
			"cashBalance": floatValue(snapshot.Cash), "updatedAt": timestamp, "createdAt": timestamp,
		})
	}
	return map[string]any{"balances": balances}, nil
}

func (s *Service) PortfolioPositions(ctx context.Context, query broker.ReadQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return map[string]any{"positions": []any{}}, nil
	}
	snapshots, err := reader.QueryPositions(ctx, query)
	if err != nil {
		return map[string]any{"positions": []any{}}, nil
	}
	timestamp := now()
	positions := make([]any, 0, len(snapshots))
	for _, position := range snapshots {
		positions = append(positions, map[string]any{
			"brokerId": query.BrokerID, "tradingEnvironment": position.TradingEnvironment,
			"accountId": position.AccountID, "market": position.Market, "symbol": position.Symbol,
			"quantity": position.Quantity, "averagePrice": firstFloat(position.AverageCostPrice, position.CostPrice),
			"marketValue": position.MarketValue, "updatedAt": timestamp, "createdAt": timestamp,
		})
	}
	return map[string]any{"positions": positions}, nil
}

func (s *Service) PortfolioReconciliation(ctx context.Context, query broker.ReadQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return readErrorResponse("positions", []any{}, errors.New("broker market data unavailable")), nil
	}
	snapshots, readErr := reader.QueryPositions(ctx, query)
	if readErr != nil {
		return readErrorResponse("positions", []any{}, readErr), nil
	}
	positions := make([]any, 0, len(snapshots))
	for _, position := range snapshots {
		positions = append(positions, map[string]any{
			"brokerId": query.BrokerID, "tradingEnvironment": position.TradingEnvironment,
			"accountId": position.AccountID, "market": position.Market, "symbol": position.Symbol,
			"symbolName": position.SymbolName, "status": "missing-in-projection",
			"projectedQuantity": nil, "brokerQuantity": position.Quantity, "quantityDelta": position.Quantity,
			"projectedAveragePrice": nil, "brokerAverageCostPrice": firstFloatPointer(position.AverageCostPrice, position.CostPrice),
			"averagePriceDelta": nil, "projectedRealizedPnl": nil, "brokerRealizedPnl": position.RealizedPnl,
			"realizedPnlDelta": nil, "projectedUpdatedAt": nil,
		})
	}
	return connectedResponse(map[string]any{"positions": positions}), nil
}

func (s *Service) PortfolioCashReconciliation(ctx context.Context, query broker.ReadQuery) (map[string]any, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return readErrorResponse("balances", []any{}, errors.New("broker market data unavailable")), nil
	}
	snapshot, readErr := reader.QueryFunds(ctx, query)
	if readErr != nil {
		return readErrorResponse("balances", []any{}, readErr), nil
	}
	balances := make([]any, 0, len(snapshot.CurrencyBalances))
	for _, balance := range snapshot.CurrencyBalances {
		balances = append(balances, map[string]any{
			"brokerId": query.BrokerID, "tradingEnvironment": balance.TradingEnvironment,
			"accountId": balance.AccountID, "currency": balance.Currency, "status": "missing-in-projection",
			"projectedCashBalance": nil, "brokerCash": balance.Cash, "cashDelta": floatValue(balance.Cash),
			"brokerAvailableWithdrawalCash": balance.AvailableWithdrawalCash,
			"brokerNetCashPower":            balance.NetCashPower, "projectedUpdatedAt": nil,
		})
	}
	return connectedResponse(map[string]any{"balances": balances}), nil
}

func (s *Service) PlaceBrokerOrder(ctx context.Context, query broker.PlaceOrderQuery) (map[string]any, error) {
	active, err := s.resolveBroker(query.BrokerID, true)
	if err != nil {
		return nil, err
	}
	trading := active.Trading()
	if trading == nil {
		return nil, ErrTradingUnsupported
	}
	result, err := trading.PlaceOrder(ctx, query)
	if err != nil {
		return nil, err
	}
	return map[string]any{"placedAt": now(), "order": result}, nil
}

func (s *Service) CancelBrokerOrders(ctx context.Context, query broker.ReadQuery, orders []broker.CancelOrder) (map[string]any, error) {
	active, err := s.resolveBroker(query.BrokerID, true)
	if err != nil {
		return nil, err
	}
	trading := active.Trading()
	if trading == nil {
		return nil, ErrTradingUnsupported
	}
	if err := trading.CancelOrders(ctx, query, orders...); err != nil {
		return nil, err
	}
	return map[string]any{"cancelledAt": now(), "cancelled": len(orders)}, nil
}

func (s *Service) UnlockTrade(ctx context.Context, req broker.UnlockTradeRequest) (map[string]any, error) {
	active, err := s.resolveBroker(req.BrokerID, true)
	if err != nil {
		return nil, err
	}
	unlocker, ok := active.(broker.UnlockTrader)
	if !ok {
		return nil, ErrUnlockUnsupported
	}
	if err := unlocker.UnlockTrade(ctx, req); err != nil {
		return nil, err
	}
	return map[string]any{"unlockedAt": now(), "unlocked": true}, nil
}

func (s *Service) FundsWithTimeout(ctx context.Context, query broker.ReadQuery, timeout time.Duration) map[string]any {
	return s.withTimeout(ctx, timeout, "summary", nil, []string{"currencyBalances", "marketAssets"}, func(queryCtx context.Context) (map[string]any, error) {
		return s.Funds(queryCtx, query)
	})
}

func (s *Service) PositionsWithTimeout(ctx context.Context, query broker.ReadQuery, timeout time.Duration) map[string]any {
	return s.withTimeout(ctx, timeout, "positions", []any{}, nil, func(queryCtx context.Context) (map[string]any, error) {
		return s.Positions(queryCtx, query)
	})
}

func (s *Service) withTimeout(ctx context.Context, timeout time.Duration, key string, empty any, extra []string, fn func(context.Context) (map[string]any, error)) map[string]any {
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	done := make(chan map[string]any, 1)
	go func() {
		value, err := fn(queryCtx)
		if err != nil {
			value = readErrorResponse(key, empty, err, extra...)
		}
		done <- value
	}()
	select {
	case <-queryCtx.Done():
		return readErrorResponse(key, empty, fmt.Errorf("%s query timed out after %s", key, timeout), extra...)
	case value := <-done:
		return value
	}
}

func (s *Service) marketDataReader(brokerID string) (broker.MarketDataReader, error) {
	active, err := s.resolveBroker(brokerID, false)
	if err != nil || active == nil {
		return nil, err
	}
	return active.MarketData(), nil
}

func (s *Service) resolveBroker(brokerID string, required bool) (broker.Broker, error) {
	brokerID = strings.TrimSpace(brokerID)
	if s.brokerRuntime == nil {
		if required {
			return nil, ErrNoBroker
		}
		return nil, nil
	}

	if brokerID != "" {
		if resolver, ok := s.brokerRuntime.(interface {
			ResolveBroker(string) broker.Broker
		}); ok {
			if selected := resolver.ResolveBroker(brokerID); selected != nil {
				return selected, nil
			}
			if s.brokerRuntime.ActiveBroker() == nil && !required {
				return nil, nil
			}
			return nil, ErrBrokerNotFound
		}
		active := s.brokerRuntime.ActiveBroker()
		if active == nil {
			if !required {
				return nil, nil
			}
			return nil, ErrNoBroker
		}
		if !strings.EqualFold(active.ID(), brokerID) {
			return nil, ErrBrokerNotFound
		}
		return active, nil
	}

	active := s.brokerRuntime.ActiveBroker()
	if active != nil {
		return active, nil
	}
	if required {
		return nil, ErrNoBroker
	}
	return nil, nil
}

func connectedResponse(values map[string]any) map[string]any {
	result := map[string]any{"checkedAt": now(), "connectivity": "connected", "lastError": nil}
	maps.Copy(result, values)
	return result
}

func readErrorResponse(key string, value any, err error, extraKeys ...string) map[string]any {
	result := map[string]any{
		"checkedAt": now(), "connectivity": connectivity(err), "lastError": err.Error(), key: value,
	}
	for _, extraKey := range extraKeys {
		result[extraKey] = []any{}
	}
	return result
}

func connectivity(err error) string {
	lower := strings.ToLower(err.Error())
	for _, marker := range []string{"connection refused", "dial ", "i/o timeout", "timeout", "client closed", "broken pipe", "connection reset", "eof", "unavailable"} {
		if strings.Contains(lower, marker) {
			return "disconnected"
		}
	}
	return "degraded"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func floatValue(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func firstFloat(values ...*float64) float64 {
	return floatValue(firstFloatPointer(values...))
}

func firstFloatPointer(values ...*float64) *float64 {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
