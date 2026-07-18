package servercore

import (
	"context"
	"errors"
	"strings"
	"time"

	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

type tradingOrderUpdateSource struct {
	server *Server
}

var _ trdsrv.OrderUpdateSource = (*tradingOrderUpdateSource)(nil)
var _ trdsrv.OrderFeeSource = (*tradingOrderUpdateSource)(nil)

func (s *tradingOrderUpdateSource) DiscoverAccounts(ctx context.Context) ([]trdsrv.Account, error) {
	brokers := s.server.brokers.All()
	if len(brokers) == 0 {
		_ = s.server.activeBroker()
		brokers = s.server.brokers.All()
	}
	if len(brokers) == 0 {
		return nil, trdsrv.ErrOrderUpdateSourceInactive
	}
	var result []trdsrv.Account
	var discoveryErrors []error
	for _, selected := range brokers {
		accounts, err := selected.DiscoverAccounts(ctx)
		if err != nil {
			discoveryErrors = append(discoveryErrors, err)
			continue
		}
		for _, account := range accounts {
			brokerID := strings.TrimSpace(account.BrokerID)
			if brokerID == "" {
				brokerID = selected.ID()
			}
			result = append(result, trdsrv.Account{
				ID: account.ID, BrokerID: brokerID, TradingEnvironment: account.TradingEnvironment,
				MarketAuthorities: append([]string(nil), account.MarketAuthorities...),
			})
		}
	}
	if len(result) == 0 && len(discoveryErrors) > 0 {
		return nil, errors.Join(discoveryErrors...)
	}
	return result, nil
}

func (s *tradingOrderUpdateSource) CurrentOrders(ctx context.Context, query trdsrv.OrderQuery) ([]trdsrv.Order, error) {
	selected := s.server.resolveBroker(query.BrokerID)
	if selected == nil || selected.MarketData() == nil {
		return nil, nil
	}
	orders, err := selected.MarketData().QueryOrders(ctx, brokerOrderQuery(query), "")
	if err != nil {
		return nil, err
	}
	return tradingOrdersFromBroker(query.BrokerID, orders), nil
}

func (s *tradingOrderUpdateSource) HistoryOrders(ctx context.Context, query trdsrv.OrderQuery, start, end time.Time) ([]trdsrv.Order, error) {
	selected := s.server.resolveBroker(query.BrokerID)
	if selected == nil || selected.MarketData() == nil {
		return nil, nil
	}
	orders, err := selected.MarketData().QueryHistoryOrders(ctx, broker.OrderHistoryQuery{
		ReadQuery: brokerOrderQuery(query),
		StartTime: start.UTC().Format(time.RFC3339Nano),
		EndTime:   end.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, err
	}
	return tradingOrdersFromBroker(query.BrokerID, orders), nil
}

func (s *tradingOrderUpdateSource) OrderFees(
	ctx context.Context,
	query trdsrv.OrderQuery,
	orderIDs []string,
) ([]broker.OrderFeeSnapshot, error) {
	selected := s.server.resolveBroker(query.BrokerID)
	if selected == nil || selected.MarketData() == nil {
		return nil, nil
	}
	return selected.MarketData().QueryOrderFees(ctx, broker.OrderFeeQuery{
		ReadQuery:     brokerOrderQuery(query),
		OrderIDExList: append([]string(nil), orderIDs...),
	})
}

func (s *tradingOrderUpdateSource) Subscribe(ctx context.Context, accounts []trdsrv.Account, _ []trdsrv.OrderQuery, handler trdsrv.OrderUpdateHandler) (trdsrv.OrderUpdateSubscription, error) {
	futuAccounts := make([]trdsrv.Account, 0, len(accounts))
	for _, account := range accounts {
		if strings.EqualFold(account.BrokerID, "futu") {
			futuAccounts = append(futuAccounts, account)
		}
	}
	if len(futuAccounts) == 0 {
		return noOpTradingOrderUpdateSubscription{}, nil
	}
	exchange := s.server.futuExchange()
	if exchange == nil {
		return noOpTradingOrderUpdateSubscription{}, nil
	}
	return futuintegration.NewOrderUpdatesAdapter(exchange).Subscribe(ctx, futuAccounts, handler)
}

type noOpTradingOrderUpdateSubscription struct{}

func (noOpTradingOrderUpdateSubscription) Stop() error { return nil }

type tradingExecutionOrderUpdates struct {
	server *Server
}

var _ trdsrv.ExecutionOrderUpdates = (*tradingExecutionOrderUpdates)(nil)
var _ trdsrv.ExecutionOrderFeeUpdates = (*tradingExecutionOrderUpdates)(nil)

func (a *tradingExecutionOrderUpdates) ApplyOrder(ctx context.Context, brokerID string, order trdsrv.Order, metadata trdsrv.OrderWriteMetadata) {
	if a == nil || a.server == nil || a.server.executionOrders == nil {
		return
	}
	updated, event, changed := a.server.executionOrders.upsertBrokerOrderWithSource(
		brokerID, brokerOrderFromTrading(order), metadata.DiscoveredEventType, metadata.UpdatedEventType,
		metadata.Source, metadata.SourceDetail,
	)
	if changed {
		a.server.notifyExecutionOrderLifecycle(updated, event)
	}
}

func (a *tradingExecutionOrderUpdates) ApplyFill(ctx context.Context, brokerID string, fill trdsrv.Fill) {
	if a == nil || a.server == nil || a.server.executionOrders == nil {
		return
	}
	updated, event, changed := a.server.executionOrders.recordBrokerOrderFill(brokerID, brokerFillFromTrading(fill))
	if changed {
		a.server.notifyExecutionOrderLifecycle(updated, event)
	}
}

func (a *tradingExecutionOrderUpdates) ApplyFees(
	_ context.Context,
	brokerID string,
	fees []broker.OrderFeeSnapshot,
) {
	if a == nil || a.server == nil || a.server.executionOrders == nil {
		return
	}
	for _, fee := range fees {
		a.server.executionOrders.recordBrokerOrderFee(brokerID, fee)
	}
}

func brokerOrderQuery(query trdsrv.OrderQuery) broker.ReadQuery {
	return broker.ReadQuery{
		BrokerID: strings.TrimSpace(query.BrokerID), AccountID: strings.TrimSpace(query.AccountID),
		TradingEnvironment: strings.TrimSpace(query.TradingEnvironment), Market: strings.TrimSpace(query.Market),
	}
}

func tradingOrdersFromBroker(brokerID string, orders []broker.OrderSnapshot) []trdsrv.Order {
	result := make([]trdsrv.Order, len(orders))
	for i, order := range orders {
		result[i] = trdsrv.Order{
			BrokerID:  brokerID,
			AccountID: order.AccountID, TradingEnvironment: order.TradingEnvironment, Market: order.Market,
			OrderKind: order.OrderKind, ProductClass: order.ProductClass, QuantityMode: order.QuantityMode,
			BrokerOrderID: order.BrokerOrderID, BrokerOrderIDEx: order.BrokerOrderIDEx,
			Symbol: order.Symbol, SymbolName: order.SymbolName, Side: order.Side, OrderType: order.OrderType,
			Status: order.Status, Quantity: order.Quantity, Amount: order.Amount,
			Legs:           append([]broker.OrderLegSnapshot(nil), order.Legs...),
			FilledQuantity: order.FilledQuantity, Price: order.Price,
			FilledAveragePrice: order.FilledAveragePrice, SubmittedAt: order.SubmittedAt, UpdatedAt: order.UpdatedAt,
			Remark: order.Remark, LastError: order.LastError, TimeInForce: order.TimeInForce, Currency: order.Currency,
		}
	}
	return result
}

func brokerOrderFromTrading(order trdsrv.Order) broker.OrderSnapshot {
	return broker.OrderSnapshot{
		AccountID: order.AccountID, TradingEnvironment: order.TradingEnvironment, Market: order.Market,
		OrderKind: order.OrderKind, ProductClass: order.ProductClass, QuantityMode: order.QuantityMode,
		BrokerOrderID: order.BrokerOrderID, BrokerOrderIDEx: order.BrokerOrderIDEx,
		Symbol: order.Symbol, SymbolName: order.SymbolName, Side: order.Side, OrderType: order.OrderType,
		Status: order.Status, Quantity: order.Quantity, Amount: order.Amount,
		Legs:           append([]broker.OrderLegSnapshot(nil), order.Legs...),
		FilledQuantity: order.FilledQuantity, Price: order.Price,
		FilledAveragePrice: order.FilledAveragePrice, SubmittedAt: order.SubmittedAt, UpdatedAt: order.UpdatedAt,
		Remark: order.Remark, LastError: order.LastError, TimeInForce: order.TimeInForce, Currency: order.Currency,
	}
}

func brokerFillFromTrading(fill trdsrv.Fill) broker.OrderFillSnapshot {
	return broker.OrderFillSnapshot{
		AccountID: fill.AccountID, TradingEnvironment: fill.TradingEnvironment, Market: fill.Market,
		BrokerOrderID: fill.BrokerOrderID, BrokerOrderIDEx: fill.BrokerOrderIDEx,
		BrokerFillID: fill.BrokerFillID, BrokerFillIDEx: fill.BrokerFillIDEx,
		Symbol: fill.Symbol, SymbolName: fill.SymbolName, Side: fill.Side,
		FilledQuantity: fill.FilledQuantity, FillPrice: fill.FillPrice, FilledAt: fill.FilledAt,
		Status: fill.Status, Payout: fill.Payout,
	}
}
