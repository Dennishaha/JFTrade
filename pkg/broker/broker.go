// Package broker defines the unified broker abstraction layer for JFTrade.
//
// All broker adapters (Futu, future brokers) implement these interfaces so that
// the API sidecar and other consumers remain decoupled from any specific broker.
//
// Design principles:
//   - Interfaces are small and composable (Go interface segregation).
//   - Types use pointer fields for optional values, matching the existing Futu convention.
//   - Each broker adapter maps its internal types to these unified types via an Adapter.
package broker

import "context"

// Broker is the top-level interface that every broker adapter must implement.
// Use Register to add brokers at startup; the API sidecar discovers them by ID.
type Broker interface {
	// ID returns the unique broker identifier (e.g. "futu", "interactive-brokers").
	ID() string

	// Descriptor returns the static capability descriptor for this broker.
	Descriptor() Descriptor

	// DiscoverAccounts returns the trading accounts currently available.
	DiscoverAccounts(ctx context.Context) ([]Account, error)

	// Trading returns the trading service, or nil if the broker does not support trading.
	Trading() TradingService

	// MarketData returns the market-data reader, or nil if the broker does not support quotes.
	MarketData() MarketDataReader
}

// MarketDataReader provides read-side broker operations: funds, positions, orders, fills, etc.
type MarketDataReader interface {
	QueryFunds(ctx context.Context, query ReadQuery) (*FundsSnapshot, error)
	QueryPositions(ctx context.Context, query ReadQuery) ([]PositionSnapshot, error)
	QueryOrders(ctx context.Context, query ReadQuery, symbol string) ([]OrderSnapshot, error)
	QueryHistoryOrders(ctx context.Context, query OrderHistoryQuery) ([]OrderSnapshot, error)
	QueryOrderFills(ctx context.Context, query OrderFillQuery) ([]OrderFillSnapshot, error)
	QueryHistoryOrderFills(ctx context.Context, query OrderFillHistoryQuery) ([]OrderFillSnapshot, error)
	QueryOrderFees(ctx context.Context, query OrderFeeQuery) ([]OrderFeeSnapshot, error)
	QueryMarginRatios(ctx context.Context, query MarginRatioQuery) ([]MarginRatioSnapshot, error)
	QueryCashFlows(ctx context.Context, query CashFlowQuery) ([]CashFlowSnapshot, error)
	QueryMaxTradeQuantity(ctx context.Context, query MaxTradeQuantityQuery) (*MaxTradeQuantitySnapshot, error)

	// QueryQuote fetches basic quote snapshots for the given securities.
	QueryQuote(ctx context.Context, query QuoteQuery) (*QuoteSnapshot, error)

	// QueryKLines fetches historical K-lines for the given query window.
	QueryKLines(ctx context.Context, query KLineQuery) (*KLineSnapshot, error)

	// QuerySecurityInfo retrieves static info for the given securities.
	QuerySecurityInfo(ctx context.Context, query SecurityInfoQuery) (*SecurityInfoSnapshot, error)

	// QuerySecuritySnapshot retrieves full snapshots (basic + extended data).
	QuerySecuritySnapshot(ctx context.Context, query SecuritySnapshotQuery) (*SecuritySnapshotResult, error)

	// QueryOrderBook retrieves the order book depth (bid/ask) for a single security.
	QueryOrderBook(ctx context.Context, query OrderBookQuery) (*OrderBookSnapshot, error)
}

// MarketRuleProvider is an optional capability for brokers that can return
// symbol-level trading rules such as board lot size.
type MarketRuleProvider interface {
	QueryMarketRules(ctx context.Context, query MarketRuleQuery) (*MarketRuleSnapshot, error)
}

// TradingService provides write-side broker operations: place and cancel orders.
type TradingService interface {
	PlaceOrder(ctx context.Context, query PlaceOrderQuery) (*PlaceOrderResult, error)
	CancelOrders(ctx context.Context, query ReadQuery, orders ...CancelOrder) error
}

// OrderUpdateHandler is called by OrderPushSubscriber when an order or fill update arrives.
type OrderUpdateHandler func(update OrderUpdateEvent)

// OrderPushSubscriber is an optional interface for brokers that support push-based
// order and fill updates (e.g. via WebSocket or TCP notification streams).
type OrderPushSubscriber interface {
	SubscribeOrderUpdates(ctx context.Context, queries []ReadQuery, handler OrderUpdateHandler) error
	UnsubscribeOrderUpdates(queries []ReadQuery)
}

// BrokerConnector is an optional interface for brokers that require an explicit
// connect/disconnect lifecycle (e.g. WebSocket-based brokers).
type BrokerConnector interface {
	Connect(ctx context.Context) error
	Close() error
}

// QuoteSubscriber is an optional interface for brokers that support real-time
// quote push subscriptions.
type QuoteSubscriber interface {
	// SubscribeQuotes subscribes to real-time quote pushes for the given securities.
	// The reader is called with each push; it MUST return quickly.
	SubscribeQuotes(ctx context.Context, req QuoteSubscribeRequest) error
}

// OrderBookSubscriber is an optional interface for brokers that support real-time
// order book depth push subscriptions (e.g. Qot_UpdateOrderBook, 3013).
type OrderBookSubscriber interface {
	// SubscribeOrderBook subscribes to real-time order book push for the given securities.
	SubscribeOrderBook(ctx context.Context, req OrderBookSubscribeRequest) error
}

// UnlockTrader is an optional interface for brokers that require explicit
// trading session unlock before placing orders.
type UnlockTrader interface {
	// UnlockTrade unlocks the trading session with the given password (MD5 hex).
	UnlockTrade(ctx context.Context, req UnlockTradeRequest) error
}
