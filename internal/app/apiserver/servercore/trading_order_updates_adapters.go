package servercore

import (
	"context"
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

func (s *tradingOrderUpdateSource) DiscoverAccounts(ctx context.Context) ([]trdsrv.Account, error) {
	active := s.server.activeBroker()
	if active == nil {
		return nil, trdsrv.ErrOrderUpdateSourceInactive
	}
	accounts, err := active.DiscoverAccounts(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]trdsrv.Account, len(accounts))
	for i, account := range accounts {
		result[i] = trdsrv.Account{
			ID: account.ID, BrokerID: account.BrokerID, TradingEnvironment: account.TradingEnvironment,
			MarketAuthorities: append([]string(nil), account.MarketAuthorities...),
		}
	}
	return result, nil
}

func (s *tradingOrderUpdateSource) CurrentOrders(ctx context.Context, query trdsrv.OrderQuery) ([]trdsrv.Order, error) {
	active := s.server.activeBroker()
	if active == nil || active.MarketData() == nil {
		return nil, nil
	}
	orders, err := active.MarketData().QueryOrders(ctx, brokerOrderQuery(query), "")
	if err != nil {
		return nil, err
	}
	return tradingOrdersFromBroker(orders), nil
}

func (s *tradingOrderUpdateSource) HistoryOrders(ctx context.Context, query trdsrv.OrderQuery, start, end time.Time) ([]trdsrv.Order, error) {
	active := s.server.activeBroker()
	if active == nil || active.MarketData() == nil {
		return nil, nil
	}
	orders, err := active.MarketData().QueryHistoryOrders(ctx, broker.OrderHistoryQuery{
		ReadQuery: brokerOrderQuery(query),
		StartTime: start.UTC().Format(time.RFC3339Nano),
		EndTime:   end.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, err
	}
	return tradingOrdersFromBroker(orders), nil
}

func (s *tradingOrderUpdateSource) Subscribe(ctx context.Context, accounts []trdsrv.Account, _ []trdsrv.OrderQuery, handler trdsrv.OrderUpdateHandler) (trdsrv.OrderUpdateSubscription, error) {
	exchange := s.server.futuExchange()
	if exchange == nil {
		return noOpTradingOrderUpdateSubscription{}, nil
	}
	return futuintegration.NewOrderUpdatesAdapter(exchange).Subscribe(ctx, accounts, handler)
}

type noOpTradingOrderUpdateSubscription struct{}

func (noOpTradingOrderUpdateSubscription) Stop() error { return nil }

type tradingExecutionOrderUpdates struct {
	server *Server
}

var _ trdsrv.ExecutionOrderUpdates = (*tradingExecutionOrderUpdates)(nil)

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

func brokerOrderQuery(query trdsrv.OrderQuery) broker.ReadQuery {
	return broker.ReadQuery{
		BrokerID: strings.TrimSpace(query.BrokerID), AccountID: strings.TrimSpace(query.AccountID),
		TradingEnvironment: strings.TrimSpace(query.TradingEnvironment), Market: strings.TrimSpace(query.Market),
	}
}

func tradingOrdersFromBroker(orders []broker.OrderSnapshot) []trdsrv.Order {
	result := make([]trdsrv.Order, len(orders))
	for i, order := range orders {
		result[i] = trdsrv.Order{
			AccountID: order.AccountID, TradingEnvironment: order.TradingEnvironment, Market: order.Market,
			BrokerOrderID: order.BrokerOrderID, BrokerOrderIDEx: order.BrokerOrderIDEx,
			Symbol: order.Symbol, SymbolName: order.SymbolName, Side: order.Side, OrderType: order.OrderType,
			Status: order.Status, Quantity: order.Quantity, FilledQuantity: order.FilledQuantity, Price: order.Price,
			FilledAveragePrice: order.FilledAveragePrice, SubmittedAt: order.SubmittedAt, UpdatedAt: order.UpdatedAt,
			Remark: order.Remark, LastError: order.LastError, TimeInForce: order.TimeInForce, Currency: order.Currency,
		}
	}
	return result
}

func brokerOrderFromTrading(order trdsrv.Order) broker.OrderSnapshot {
	return broker.OrderSnapshot{
		AccountID: order.AccountID, TradingEnvironment: order.TradingEnvironment, Market: order.Market,
		BrokerOrderID: order.BrokerOrderID, BrokerOrderIDEx: order.BrokerOrderIDEx,
		Symbol: order.Symbol, SymbolName: order.SymbolName, Side: order.Side, OrderType: order.OrderType,
		Status: order.Status, Quantity: order.Quantity, FilledQuantity: order.FilledQuantity, Price: order.Price,
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
		FilledQuantity: fill.FilledQuantity, FillPrice: fill.FillPrice, FilledAt: fill.FilledAt, Status: fill.Status,
	}
}
