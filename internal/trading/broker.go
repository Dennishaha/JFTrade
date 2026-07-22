package trading

import (
	"context"
	"errors"
	"fmt"
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

// Runtime 返回券商运行时状态。
func (s *Service) Runtime(ctx context.Context, brokerID string) (*BrokerRuntimeResponse, error) {
	if _, err := s.resolveBroker(brokerID, false); err != nil {
		return nil, err
	}
	if s.brokerRuntime == nil {
		return emptyBrokerRuntimeResponse(), nil
	}
	return s.brokerRuntime.Runtime(ctx), nil
}

func (s *Service) Funds(ctx context.Context, query broker.ReadQuery) (*BrokerFundsResponse, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return fundsReadError(errors.New("broker market data not available")), nil
	}
	snapshot, readErr := reader.QueryFunds(ctx, query)
	if readErr != nil {
		return fundsReadError(readErr), nil
	}

	currencyBalances := make([]BrokerCurrencyBalance, 0, len(snapshot.CurrencyBalances))
	for _, balance := range snapshot.CurrencyBalances {
		currencyBalances = append(currencyBalances, BrokerCurrencyBalance{
			AccountID: balance.AccountID, TradingEnvironment: balance.TradingEnvironment,
			Currency: balance.Currency, Cash: balance.Cash,
			AvailableWithdrawalCash: balance.AvailableWithdrawalCash, NetCashPower: balance.NetCashPower,
		})
	}
	marketAssets := make([]BrokerMarketAsset, 0, len(snapshot.MarketAssets))
	for _, asset := range snapshot.MarketAssets {
		marketAssets = append(marketAssets, BrokerMarketAsset{
			AccountID: asset.AccountID, TradingEnvironment: asset.TradingEnvironment,
			Market: asset.Market, Assets: asset.Assets,
		})
	}
	return &BrokerFundsResponse{
		BrokerReadStatus: connectedReadStatus(),
		Summary: &BrokerFundsSummary{
			AccountID: snapshot.AccountID, TradingEnvironment: snapshot.TradingEnvironment,
			Market: snapshot.Market, Currency: snapshot.Currency, TotalAssets: snapshot.TotalAssets,
			SecuritiesAssets: snapshot.SecuritiesAssets, FundAssets: snapshot.FundAssets,
			BondAssets: snapshot.BondAssets, Cash: snapshot.Cash, MarketValue: snapshot.MarketValue,
			LongMarketValue: snapshot.LongMarketValue, ShortMarketValue: snapshot.ShortMarketValue,
			PurchasingPower: snapshot.PurchasingPower, ShortSellingPower: snapshot.ShortSellingPower,
			NetCashPower: snapshot.NetCashPower, AvailableWithdrawalCash: snapshot.AvailableWithdrawalCash,
			MaxWithdrawal: snapshot.MaxWithdrawal, AvailableFunds: snapshot.AvailableFunds,
			FrozenCash: snapshot.FrozenCash, PendingAsset: snapshot.PendingAsset,
			UnrealizedPnl: snapshot.UnrealizedPnl, RealizedPnl: snapshot.RealizedPnl,
			InitialMargin: snapshot.InitialMargin, MaintenanceMargin: snapshot.MaintenanceMargin,
			MarginCallMargin: snapshot.MarginCallMargin, RiskStatus: snapshot.RiskStatus,
			DebtCash: snapshot.DebtCash, IsPDT: snapshot.IsPDT, PDTSeq: snapshot.PDTSeq,
			BeginningDTBP: snapshot.BeginningDTBP, RemainingDTBP: snapshot.RemainingDTBP,
			DTCallAmount: snapshot.DTCallAmount, DTStatus: snapshot.DTStatus,
			ExposureLevel: snapshot.ExposureLevel, ExposureLimit: snapshot.ExposureLimit,
			UsedLimit: snapshot.UsedLimit, RemainingLimit: snapshot.RemainingLimit,
		},
		CurrencyBalances: currencyBalances,
		MarketAssets:     marketAssets,
	}, nil
}

func (s *Service) Positions(ctx context.Context, query broker.ReadQuery) (*BrokerPositionsResponse, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return positionsReadError(errors.New("broker market data not available")), nil
	}
	positions, readErr := reader.QueryPositions(ctx, query)
	if readErr != nil {
		return positionsReadError(readErr), nil
	}
	entries := make([]BrokerPosition, 0, len(positions))
	for _, position := range positions {
		entries = append(entries, BrokerPosition{
			AccountID: position.AccountID, TradingEnvironment: position.TradingEnvironment,
			Market: position.Market, Symbol: position.Symbol, SymbolName: position.SymbolName,
			Quantity: position.Quantity, SellableQuantity: position.SellableQuantity,
			LastPrice: position.LastPrice, CostPrice: position.CostPrice,
			AverageCostPrice: position.AverageCostPrice, MarketValue: position.MarketValue,
			UnrealizedPnl: position.UnrealizedPnl, RealizedPnl: position.RealizedPnl,
			PnlRatio: position.PnlRatio, Currency: position.Currency,
		})
	}
	return &BrokerPositionsResponse{BrokerReadStatus: connectedReadStatus(), Positions: entries}, nil
}

func (s *Service) Orders(ctx context.Context, query OrdersQuery) (*BrokerOrdersResponse, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return ordersReadError(errors.New("broker market data not available")), nil
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
		return ordersReadError(err), nil
	}
	entries := make([]BrokerOrder, 0, len(orders))
	for _, order := range orders {
		entries = append(entries, BrokerOrder{
			AccountID:          firstNonEmpty(order.AccountID, query.AccountID),
			TradingEnvironment: firstNonEmpty(order.TradingEnvironment, query.TradingEnvironment),
			Market:             firstNonEmpty(order.Market, query.Market), BrokerOrderID: order.BrokerOrderID,
			BrokerOrderIDEx: order.BrokerOrderIDEx, Symbol: order.Symbol, SymbolName: order.SymbolName,
			Side: order.Side, OrderType: order.OrderType, Status: order.Status,
			Quantity: order.Quantity, FilledQuantity: order.FilledQuantity, Price: order.Price,
			FilledAveragePrice: order.FilledAveragePrice, SubmittedAt: order.SubmittedAt,
			UpdatedAt: order.UpdatedAt, Remark: order.Remark, LastError: order.LastError,
			TimeInForce: order.TimeInForce, Currency: order.Currency,
		})
	}
	return &BrokerOrdersResponse{BrokerReadStatus: connectedReadStatus(), Orders: entries}, nil
}

func (s *Service) Fills(ctx context.Context, query FillsQuery) (*BrokerFillsResponse, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return fillsReadError(errors.New("broker market data not available")), nil
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
		return fillsReadError(err), nil
	}
	entries := make([]BrokerFill, 0, len(fills))
	for _, fill := range fills {
		entries = append(entries, BrokerFill{
			AccountID: fill.AccountID, TradingEnvironment: fill.TradingEnvironment,
			Market: fill.Market, BrokerOrderID: fill.BrokerOrderID,
			BrokerOrderIDEx: fill.BrokerOrderIDEx, BrokerFillID: fill.BrokerFillID,
			BrokerFillIDEx: fill.BrokerFillIDEx, Symbol: fill.Symbol, SymbolName: fill.SymbolName,
			Side: fill.Side, FilledQuantity: fill.FilledQuantity, FillPrice: fill.FillPrice,
			FilledAt: fill.FilledAt, Status: fill.Status,
		})
	}
	return &BrokerFillsResponse{BrokerReadStatus: connectedReadStatus(), Fills: entries}, nil
}

func (s *Service) CashFlows(ctx context.Context, query broker.CashFlowQuery) (*BrokerCashFlowsResponse, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return cashFlowsReadError(errors.New("broker market data not available")), nil
	}
	flows, readErr := reader.QueryCashFlows(ctx, query)
	if readErr != nil {
		return cashFlowsReadError(readErr), nil
	}
	entries := make([]broker.CashFlowSnapshot, len(flows))
	copy(entries, flows)
	return &BrokerCashFlowsResponse{BrokerReadStatus: connectedReadStatus(), CashFlows: entries}, nil
}

func (s *Service) OrderFees(ctx context.Context, query broker.OrderFeeQuery) (*BrokerOrderFeesResponse, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return orderFeesReadError(errors.New("broker market data not available")), nil
	}
	fees, readErr := reader.QueryOrderFees(ctx, query)
	if readErr != nil {
		return orderFeesReadError(readErr), nil
	}
	entries := make([]broker.OrderFeeSnapshot, len(fees))
	copy(entries, fees)
	return &BrokerOrderFeesResponse{BrokerReadStatus: connectedReadStatus(), Fees: entries}, nil
}

func (s *Service) MarginRatios(ctx context.Context, query broker.MarginRatioQuery) (*BrokerMarginRatiosResponse, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return marginRatiosReadError(errors.New("broker market data not available")), nil
	}
	ratios, readErr := reader.QueryMarginRatios(ctx, query)
	if readErr != nil {
		return marginRatiosReadError(readErr), nil
	}
	entries := make([]broker.MarginRatioSnapshot, len(ratios))
	copy(entries, ratios)
	return &BrokerMarginRatiosResponse{BrokerReadStatus: connectedReadStatus(), MarginRatios: entries}, nil
}

func (s *Service) MaxTradeQuantity(ctx context.Context, query broker.MaxTradeQuantityQuery) (*BrokerMaxTradeQuantityResponse, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return maxTradeQuantityReadError(errors.New("broker market data not available")), nil
	}
	snapshot, readErr := reader.QueryMaxTradeQuantity(ctx, query)
	if readErr != nil {
		return maxTradeQuantityReadError(readErr), nil
	}
	return &BrokerMaxTradeQuantityResponse{BrokerReadStatus: connectedReadStatus(), MaxTradeQuantity: snapshot}, nil
}

func (s *Service) Quote(ctx context.Context, query broker.QuoteQuery) (*BrokerQuoteResponse, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return quoteReadError(errors.New("broker market data not available")), nil
	}
	quote, readErr := reader.QueryQuote(ctx, query)
	if readErr != nil {
		return quoteReadError(readErr), nil
	}
	return &BrokerQuoteResponse{BrokerReadStatus: connectedReadStatus(), Quote: quote}, nil
}

func (s *Service) KLines(ctx context.Context, query broker.KLineQuery) (*BrokerKLinesResponse, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return klinesReadError(errors.New("broker market data not available")), nil
	}
	snapshot, readErr := reader.QueryKLines(ctx, query)
	if readErr != nil {
		return klinesReadError(readErr), nil
	}
	return &BrokerKLinesResponse{BrokerReadStatus: connectedReadStatus(), KLines: snapshot}, nil
}

func (s *Service) Securities(ctx context.Context, query broker.SecuritySnapshotQuery) (*BrokerSecuritiesResponse, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return securitiesReadError(errors.New("broker market data not available")), nil
	}
	result, readErr := reader.QuerySecuritySnapshot(ctx, query)
	if readErr != nil {
		return securitiesReadError(readErr), nil
	}
	return &BrokerSecuritiesResponse{BrokerReadStatus: connectedReadStatus(), Securities: result}, nil
}

func (s *Service) PortfolioCashBalances(ctx context.Context, query broker.ReadQuery) (*PortfolioCashBalancesResponse, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return &PortfolioCashBalancesResponse{Balances: []PortfolioCashBalance{}}, nil
	}
	snapshot, err := reader.QueryFunds(ctx, query)
	if err != nil {
		return &PortfolioCashBalancesResponse{Balances: []PortfolioCashBalance{}}, nil
	}
	timestamp := now()
	balances := make([]PortfolioCashBalance, 0, len(snapshot.CurrencyBalances))
	for _, balance := range snapshot.CurrencyBalances {
		balances = append(balances, PortfolioCashBalance{
			BrokerID: query.BrokerID, TradingEnvironment: balance.TradingEnvironment,
			AccountID: balance.AccountID, Currency: balance.Currency,
			CashBalance: floatValue(balance.Cash), UpdatedAt: timestamp, CreatedAt: timestamp,
		})
	}
	if len(balances) == 0 && snapshot.Currency != nil {
		balances = append(balances, PortfolioCashBalance{
			BrokerID: query.BrokerID, TradingEnvironment: snapshot.TradingEnvironment,
			AccountID: snapshot.AccountID, Currency: *snapshot.Currency,
			CashBalance: floatValue(snapshot.Cash), UpdatedAt: timestamp, CreatedAt: timestamp,
		})
	}
	return &PortfolioCashBalancesResponse{Balances: balances}, nil
}

func (s *Service) PortfolioPositions(ctx context.Context, query broker.ReadQuery) (*PortfolioPositionsResponse, error) {
	reader, err := s.marketDataReader(query.BrokerID)
	if err != nil {
		return nil, err
	}
	if reader == nil {
		return &PortfolioPositionsResponse{Positions: []PortfolioPosition{}}, nil
	}
	snapshots, err := reader.QueryPositions(ctx, query)
	if err != nil {
		return &PortfolioPositionsResponse{Positions: []PortfolioPosition{}}, nil
	}
	timestamp := now()
	positions := make([]PortfolioPosition, 0, len(snapshots))
	for _, position := range snapshots {
		positions = append(positions, PortfolioPosition{
			BrokerID: query.BrokerID, TradingEnvironment: position.TradingEnvironment,
			AccountID: position.AccountID, Market: position.Market, Symbol: position.Symbol,
			Quantity: position.Quantity, AveragePrice: firstFloat(position.AverageCostPrice, position.CostPrice),
			MarketValue: position.MarketValue, UpdatedAt: timestamp, CreatedAt: timestamp,
		})
	}
	return &PortfolioPositionsResponse{Positions: positions}, nil
}

func (s *Service) PlaceBrokerOrder(ctx context.Context, query broker.PlaceOrderQuery) (*BrokerPlaceOrderResponse, error) {
	// Resolve the environment before risk evaluation. Leaving it empty lets a
	// broker choose its only available account, which may be a REAL account.
	query.TradingEnvironment = s.executionEnvironment(query.TradingEnvironment)
	active, err := s.resolveBroker(query.BrokerID, true)
	if err != nil {
		return nil, err
	}
	trading := active.Trading()
	if trading == nil {
		return nil, ErrTradingUnsupported
	}
	command := ExecutionOrderCommand{
		BrokerID:     firstNonEmpty(active.ID(), strings.TrimSpace(query.BrokerID)),
		Query:        query,
		Symbol:       query.Symbol,
		Side:         query.Side,
		OrderType:    query.OrderType,
		ProductClass: query.ProductClass,
		QuantityMode: query.QuantityMode,
	}
	if query.Remark != nil {
		command.Remark = *query.Remark
	}
	if query.Session != nil {
		command.Session = *query.Session
	}
	var result *broker.PlaceOrderResult
	err = s.executePlaceOrderWithRisk(ctx, command, func() error {
		var submitErr error
		result, submitErr = trading.PlaceOrder(ctx, query)
		return submitErr
	})
	if err != nil {
		return nil, err
	}
	return &BrokerPlaceOrderResponse{PlacedAt: now(), Order: result}, nil
}

func (s *Service) CancelBrokerOrders(ctx context.Context, query broker.ReadQuery, orders []broker.CancelOrder) (*BrokerCancelOrdersResponse, error) {
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
	return &BrokerCancelOrdersResponse{CancelledAt: now(), Cancelled: len(orders)}, nil
}

func (s *Service) UnlockTrade(ctx context.Context, req broker.UnlockTradeRequest) (*BrokerUnlockTradeResponse, error) {
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
	return &BrokerUnlockTradeResponse{UnlockedAt: now(), Unlocked: true}, nil
}

func (s *Service) FundsWithTimeout(ctx context.Context, query broker.ReadQuery, timeout time.Duration) *BrokerFundsResponse {
	return withTimeout(ctx, timeout, "summary", func(queryCtx context.Context) (*BrokerFundsResponse, error) {
		return s.Funds(queryCtx, query)
	}, fundsReadError)
}

func (s *Service) PositionsWithTimeout(ctx context.Context, query broker.ReadQuery, timeout time.Duration) *BrokerPositionsResponse {
	return withTimeout(ctx, timeout, "positions", func(queryCtx context.Context) (*BrokerPositionsResponse, error) {
		return s.Positions(queryCtx, query)
	}, positionsReadError)
}

func withTimeout[T any](ctx context.Context, timeout time.Duration, key string, fn func(context.Context) (T, error), onError func(error) T) T {
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	value, err := fn(queryCtx)
	if queryCtx.Err() != nil {
		return onError(fmt.Errorf("%s query timed out after %s", key, timeout))
	}
	if err != nil {
		return onError(err)
	}
	return value
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
