// Package tradingapp owns application wiring for trading background workers.
// Trading rules and order state machines remain in internal/trading.
package tradingapp

import (
	"context"
	"errors"
	"strings"
	"time"

	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

// OrderUpdateSourceOptions supplies the runtime-owned broker surfaces needed
// by the trading order synchronization worker.
type OrderUpdateSourceOptions struct {
	Brokers         func() []broker.Broker
	ActivateBroker  func()
	ResolveBroker   func(string) broker.Broker
	SubscribeOrders func(context.Context, []trdsrv.Account, trdsrv.OrderUpdateHandler) (trdsrv.OrderUpdateSubscription, error)
}

// OrderUpdateSource adapts runtime broker reads and pushes to internal/trading.
type OrderUpdateSource struct {
	options OrderUpdateSourceOptions
}

var _ trdsrv.OrderUpdateSource = (*OrderUpdateSource)(nil)
var _ trdsrv.OrderFeeSource = (*OrderUpdateSource)(nil)

func NewOrderUpdateSource(options OrderUpdateSourceOptions) *OrderUpdateSource {
	return &OrderUpdateSource{options: options}
}

func (s *OrderUpdateSource) DiscoverAccounts(ctx context.Context) ([]trdsrv.Account, error) {
	brokers := s.availableBrokers()
	if len(brokers) == 0 && s != nil && s.options.ActivateBroker != nil {
		s.options.ActivateBroker()
		brokers = s.availableBrokers()
	}
	if len(brokers) == 0 {
		return nil, trdsrv.ErrOrderUpdateSourceInactive
	}
	var result []trdsrv.Account
	var discoveryErrors []error
	excludedOrderAccounts := 0
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
			marketAuthorities := account.MarketAuthorities
			if account.OrderMarketAuthorities != nil {
				if len(account.OrderMarketAuthorities) == 0 {
					excludedOrderAccounts++
					continue
				}
				marketAuthorities = account.OrderMarketAuthorities
			}
			result = append(result, trdsrv.Account{
				ID: account.ID, BrokerID: brokerID, TradingEnvironment: account.TradingEnvironment,
				MarketAuthorities: append([]string(nil), marketAuthorities...),
			})
		}
	}
	if len(result) == 0 && len(discoveryErrors) > 0 {
		return nil, errors.Join(discoveryErrors...)
	}
	if len(result) == 0 && excludedOrderAccounts > 0 {
		return nil, trdsrv.ErrOrderUpdateSourceInactive
	}
	return result, nil
}

func (s *OrderUpdateSource) CurrentOrders(ctx context.Context, query trdsrv.OrderQuery) ([]trdsrv.Order, error) {
	selected := s.resolveBroker(query.BrokerID)
	if selected == nil || selected.MarketData() == nil {
		return nil, nil
	}
	orders, err := selected.MarketData().QueryOrders(ctx, brokerOrderQuery(query), "")
	if err != nil {
		return nil, err
	}
	return tradingOrdersFromBroker(query.BrokerID, orders), nil
}

func (s *OrderUpdateSource) HistoryOrders(
	ctx context.Context,
	query trdsrv.OrderQuery,
	start time.Time,
	end time.Time,
) ([]trdsrv.Order, error) {
	selected := s.resolveBroker(query.BrokerID)
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

func (s *OrderUpdateSource) OrderFees(
	ctx context.Context,
	query trdsrv.OrderQuery,
	orderIDs []string,
) ([]broker.OrderFeeSnapshot, error) {
	selected := s.resolveBroker(query.BrokerID)
	if selected == nil || selected.MarketData() == nil {
		return nil, nil
	}
	return selected.MarketData().QueryOrderFees(ctx, broker.OrderFeeQuery{
		ReadQuery:     brokerOrderQuery(query),
		OrderIDExList: append([]string(nil), orderIDs...),
	})
}

func (s *OrderUpdateSource) Subscribe(
	ctx context.Context,
	accounts []trdsrv.Account,
	_ []trdsrv.OrderQuery,
	handler trdsrv.OrderUpdateHandler,
) (trdsrv.OrderUpdateSubscription, error) {
	futuAccounts := make([]trdsrv.Account, 0, len(accounts))
	for _, account := range accounts {
		if strings.EqualFold(account.BrokerID, "futu") {
			futuAccounts = append(futuAccounts, account)
		}
	}
	if len(futuAccounts) == 0 || s == nil || s.options.SubscribeOrders == nil {
		return noOpOrderUpdateSubscription{}, nil
	}
	subscription, err := s.options.SubscribeOrders(ctx, futuAccounts, handler)
	if err != nil {
		return nil, err
	}
	if subscription == nil {
		return noOpOrderUpdateSubscription{}, nil
	}
	return subscription, nil
}

func (s *OrderUpdateSource) availableBrokers() []broker.Broker {
	if s == nil || s.options.Brokers == nil {
		return nil
	}
	return s.options.Brokers()
}

func (s *OrderUpdateSource) resolveBroker(id string) broker.Broker {
	if s == nil || s.options.ResolveBroker == nil {
		return nil
	}
	return s.options.ResolveBroker(id)
}

type noOpOrderUpdateSubscription struct{}

func (noOpOrderUpdateSubscription) Stop() error { return nil }

// ExecutionOrderUpdates applies broker updates to the durable trading ledger.
type ExecutionOrderUpdates struct {
	store  trdsrv.ExecutionReconciliationStore
	notify func(trdsrv.ExecutionOrder, *trdsrv.ExecutionOrderEvent)
}

var _ trdsrv.ExecutionOrderUpdates = (*ExecutionOrderUpdates)(nil)
var _ trdsrv.ExecutionOrderFeeUpdates = (*ExecutionOrderUpdates)(nil)

func NewExecutionOrderUpdates(
	store trdsrv.ExecutionReconciliationStore,
	notify func(trdsrv.ExecutionOrder, *trdsrv.ExecutionOrderEvent),
) *ExecutionOrderUpdates {
	return &ExecutionOrderUpdates{store: store, notify: notify}
}

func (u *ExecutionOrderUpdates) ApplyOrder(
	_ context.Context,
	brokerID string,
	order trdsrv.Order,
	metadata trdsrv.OrderWriteMetadata,
) {
	if u == nil || u.store == nil {
		return
	}
	updated, event, changed := u.store.ApplyBrokerOrder(
		brokerID, brokerOrderFromTrading(order), metadata.DiscoveredEventType, metadata.UpdatedEventType,
		metadata.Source, metadata.SourceDetail,
	)
	if changed && u.notify != nil {
		u.notify(updated, event)
	}
}

func (u *ExecutionOrderUpdates) ApplyFill(_ context.Context, brokerID string, fill trdsrv.Fill) {
	if u == nil || u.store == nil {
		return
	}
	updated, event, changed := u.store.ApplyBrokerFill(brokerID, brokerFillFromTrading(fill))
	if changed && u.notify != nil {
		u.notify(updated, event)
	}
}

func (u *ExecutionOrderUpdates) ApplyFees(
	_ context.Context,
	brokerID string,
	fees []broker.OrderFeeSnapshot,
) {
	if u == nil || u.store == nil {
		return
	}
	for _, fee := range fees {
		u.store.ApplyBrokerFee(brokerID, fee)
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
			BrokerID: brokerID, AccountID: order.AccountID, TradingEnvironment: order.TradingEnvironment,
			Market: order.Market, OrderKind: order.OrderKind, ProductClass: order.ProductClass,
			QuantityMode: order.QuantityMode, BrokerOrderID: order.BrokerOrderID, BrokerOrderIDEx: order.BrokerOrderIDEx,
			Symbol: order.Symbol, SymbolName: order.SymbolName, Side: order.Side, OrderType: order.OrderType,
			Status: order.Status, Quantity: order.Quantity, Amount: order.Amount,
			Legs: append([]broker.OrderLegSnapshot(nil), order.Legs...), FilledQuantity: order.FilledQuantity,
			Price: order.Price, FilledAveragePrice: order.FilledAveragePrice, SubmittedAt: order.SubmittedAt,
			UpdatedAt: order.UpdatedAt, Remark: order.Remark, LastError: order.LastError,
			TimeInForce: order.TimeInForce, Currency: order.Currency,
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
		Legs: append([]broker.OrderLegSnapshot(nil), order.Legs...), FilledQuantity: order.FilledQuantity,
		Price: order.Price, FilledAveragePrice: order.FilledAveragePrice, SubmittedAt: order.SubmittedAt,
		UpdatedAt: order.UpdatedAt, Remark: order.Remark, LastError: order.LastError,
		TimeInForce: order.TimeInForce, Currency: order.Currency,
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

func NewOrderUpdatesWorker(
	source trdsrv.OrderUpdateSource,
	execution trdsrv.ExecutionOrderUpdates,
	config trdsrv.OrderUpdatesConfig,
) *trdsrv.OrderUpdatesWorker {
	return trdsrv.NewOrderUpdatesWorker(source, execution, config)
}
