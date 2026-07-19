package trading

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/besteffort"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

const (
	DefaultOrderUpdateSyncMinInterval = 1500 * time.Millisecond
	DefaultActiveOrdersCacheTTL       = 60 * time.Second
	maxOrderUpdateInvalidations       = 20
)

var ErrOrderUpdateSourceInactive = errors.New("order update source is inactive")

type Account struct {
	ID                 string
	BrokerID           string
	TradingEnvironment string
	MarketAuthorities  []string
}

type OrderQuery struct {
	BrokerID           string
	TradingEnvironment string
	AccountID          string
	Market             string
}

type Order struct {
	BrokerID           string
	AccountID          string
	TradingEnvironment string
	Market             string
	OrderKind          broker.OrderKind
	ProductClass       broker.ProductClass
	QuantityMode       broker.QuantityMode
	BrokerOrderID      string
	BrokerOrderIDEx    *string
	Symbol             string
	SymbolName         *string
	Side               string
	OrderType          string
	Status             string
	Quantity           float64
	Amount             *float64
	Legs               []broker.OrderLegSnapshot
	FilledQuantity     *float64
	Price              *float64
	FilledAveragePrice *float64
	SubmittedAt        string
	UpdatedAt          string
	Remark             *string
	LastError          *string
	TimeInForce        *string
	Currency           *string
}

type Fill struct {
	BrokerID           string
	AccountID          string
	TradingEnvironment string
	Market             string
	BrokerOrderID      string
	BrokerOrderIDEx    *string
	BrokerFillID       string
	BrokerFillIDEx     *string
	Symbol             string
	SymbolName         *string
	Side               string
	FilledQuantity     float64
	FillPrice          *float64
	FilledAt           string
	Status             *string
	Payout             *float64
}

type OrderWriteMetadata struct {
	DiscoveredEventType string
	UpdatedEventType    string
	Source              string
	SourceDetail        string
}

type OrderUpdateHandler interface {
	HandleOrderUpdate(Order)
	HandleFillUpdate(Fill)
}

type OrderUpdateSubscription interface {
	Stop() error
}

type orderUpdateSubscriptionRefresher interface {
	Refresh(context.Context, []Account, []OrderQuery) error
}

type OrderUpdateSource interface {
	DiscoverAccounts(context.Context) ([]Account, error)
	CurrentOrders(context.Context, OrderQuery) ([]Order, error)
	HistoryOrders(context.Context, OrderQuery, time.Time, time.Time) ([]Order, error)
	Subscribe(context.Context, []Account, []OrderQuery, OrderUpdateHandler) (OrderUpdateSubscription, error)
}

type OrderFeeSource interface {
	OrderFees(context.Context, OrderQuery, []string) ([]broker.OrderFeeSnapshot, error)
}

type ExecutionOrderUpdates interface {
	ApplyOrder(context.Context, string, Order, OrderWriteMetadata)
	ApplyFill(context.Context, string, Fill)
}

type ExecutionOrderFeeUpdates interface {
	ApplyFees(context.Context, string, []broker.OrderFeeSnapshot)
}

type OrderUpdatesConfig struct {
	BrokerID        string
	FallbackMarket  string
	HistoryLookback func() int
	SyncMinInterval time.Duration
	CacheTTL        time.Duration
	Now             func() time.Time
}

type orderUpdateSubscriptionState struct {
	SubscriptionKey     string
	BrokerID            string
	TradingEnvironment  *string
	AccountID           *string
	Market              *string
	Status              string
	LastAction          string
	LastActionAt        string
	LastError           *string
	ConsecutiveFailures *int
	RetryDelayMs        *int
	BackoffUntil        *string
}

type orderUpdateInvalidation struct {
	SubscriptionKey    string
	BrokerID           string
	TradingEnvironment *string
	AccountID          *string
	Market             *string
	Kind               string
	Message            *string
	CreatedAt          string
}

type OrderUpdatesWorker struct {
	mu sync.Mutex

	source    OrderUpdateSource
	execution ExecutionOrderUpdates
	config    OrderUpdatesConfig

	subscriptions        map[string]*orderUpdateSubscriptionState
	recentInvalidations  []orderUpdateInvalidation
	lastStoppedAt        *string
	stoppedSubscriptions *int
	lastSyncAt           time.Time
	accountsDiscovered   int
	connectivity         *string
	activeOrdersCache    map[string][]Order
	activeOrdersCachedAt map[string]time.Time
	pushSubscription     OrderUpdateSubscription
	pushSubscriptionKey  string
	subscriptionPending  bool
	subscriptionReady    chan struct{}
	subscriptionEpoch    uint64
}

type orderUpdateSubscribeOp int

const (
	subscribeNoop orderUpdateSubscribeOp = iota
	subscribeFresh
	subscribeRefresh
)

type orderUpdateSubscriptionAttempt struct {
	op           orderUpdateSubscribeOp
	epoch        uint64
	existing     OrderUpdateSubscription
	refresher    orderUpdateSubscriptionRefresher
	subscription OrderUpdateSubscription
	old          OrderUpdateSubscription
	installed    bool
}

func NewOrderUpdatesWorker(source OrderUpdateSource, execution ExecutionOrderUpdates, config OrderUpdatesConfig) *OrderUpdatesWorker {
	config.BrokerID = strings.TrimSpace(config.BrokerID)
	if config.FallbackMarket == "" {
		config.FallbackMarket = "HK"
	}
	if config.SyncMinInterval <= 0 {
		config.SyncMinInterval = DefaultOrderUpdateSyncMinInterval
	}
	if config.CacheTTL <= 0 {
		config.CacheTTL = DefaultActiveOrdersCacheTTL
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.HistoryLookback == nil {
		config.HistoryLookback = func() int { return 3 }
	}
	return &OrderUpdatesWorker{
		source:               source,
		execution:            execution,
		config:               config,
		subscriptions:        make(map[string]*orderUpdateSubscriptionState),
		activeOrdersCache:    make(map[string][]Order),
		activeOrdersCachedAt: make(map[string]time.Time),
	}
}

func (w *OrderUpdatesWorker) Sync(ctx context.Context, force bool, activeOnly bool) {
	if w == nil || w.source == nil || w.execution == nil || !w.shouldSync(force) {
		return
	}

	accounts, err := w.source.DiscoverAccounts(ctx)
	if err != nil {
		if errors.Is(err, ErrOrderUpdateSourceInactive) {
			w.markDiscoveredAccounts(0, "inactive")
			return
		}
		query := OrderQuery{
			BrokerID:           w.config.BrokerID,
			TradingEnvironment: "SIMULATE",
			Market:             strings.ToUpper(strings.TrimSpace(w.config.FallbackMarket)),
		}
		w.markSubscriptions([]OrderQuery{query}, "inactive", "discover-accounts", err)
		return
	}
	if w.config.BrokerID == "" && len(accounts) > 0 {
		w.config.BrokerID = strings.TrimSpace(accounts[0].BrokerID)
	}
	queries := BuildOrderUpdateQueries(accounts, w.config.BrokerID, w.config.FallbackMarket)
	w.markDiscoveredAccounts(len(accounts), "connected")
	if err := w.ensureSubscribed(ctx, accounts, queries); err != nil {
		w.markSubscriptions(queries, "inactive", "bind-push", err)
	}

	for _, query := range queries {
		key := OrderUpdateSubscriptionKey(query)
		if activeOnly && !force {
			if cached, ok := w.cachedActiveOrders(key); ok {
				w.applyOrders(ctx, query.BrokerID, cached, OrderWriteMetadata{
					DiscoveredEventType: "BROKER_CACHE_DISCOVERED",
					UpdatedEventType:    "BROKER_CACHE_UPDATED",
					Source:              "broker",
					SourceDetail:        "broker.cache",
				})
				continue
			}
		}

		orders, err := w.source.CurrentOrders(ctx, query)
		if err != nil {
			w.markSubscriptions([]OrderQuery{query}, "inactive", "sync-orders", err)
			continue
		}
		w.markSubscriptions([]OrderQuery{query}, "active", "sync-orders", nil)
		w.storeActiveOrders(key, orders)
		w.applyOrders(ctx, query.BrokerID, orders, OrderWriteMetadata{
			DiscoveredEventType: "BROKER_SYNC_DISCOVERED",
			UpdatedEventType:    "BROKER_SYNC_UPDATED",
			Source:              "broker",
			SourceDetail:        "broker.current",
		})
		w.syncOrderFees(ctx, query, orders)
		if activeOnly {
			continue
		}

		end := w.now()
		lookbackDays := w.config.HistoryLookback()
		if lookbackDays <= 0 {
			lookbackDays = 3
		}
		history, err := w.source.HistoryOrders(ctx, query, end.Add(-time.Duration(lookbackDays)*24*time.Hour), end)
		if err != nil {
			w.markSubscriptions([]OrderQuery{query}, "inactive", "sync-history-orders", err)
			continue
		}
		w.markSubscriptions([]OrderQuery{query}, "active", "sync-history-orders", nil)
		w.applyOrders(ctx, query.BrokerID, history, OrderWriteMetadata{
			DiscoveredEventType: "BROKER_HISTORY_DISCOVERED",
			UpdatedEventType:    "BROKER_HISTORY_UPDATED",
			Source:              "broker",
			SourceDetail:        "broker.history",
		})
		w.syncOrderFees(ctx, query, history)
	}
}

func (w *OrderUpdatesWorker) SyncExecutionOrderHistory(ctx context.Context, order ExecutionOrder) {
	if w == nil || w.source == nil || w.execution == nil {
		return
	}
	if !executionOrderHasBrokerReference(order) {
		return
	}
	query := OrderQuery{
		BrokerID:           strings.TrimSpace(firstNonEmpty(order.BrokerID, w.config.BrokerID)),
		TradingEnvironment: strings.TrimSpace(order.TradingEnvironment),
		AccountID:          strings.TrimSpace(order.AccountID),
		Market:             strings.ToUpper(strings.TrimSpace(order.Market)),
	}
	if query.BrokerID == "" || query.TradingEnvironment == "" || query.AccountID == "" || query.Market == "" {
		return
	}
	end := w.now()
	lookbackDays := w.config.HistoryLookback()
	if lookbackDays <= 0 {
		lookbackDays = 3
	}
	history, err := w.source.HistoryOrders(ctx, query, end.Add(-time.Duration(lookbackDays)*24*time.Hour), end)
	if err != nil {
		w.markSubscriptions([]OrderQuery{query}, "inactive", "sync-history-orders", err)
		return
	}
	w.markSubscriptions([]OrderQuery{query}, "active", "sync-history-orders", nil)
	w.applyOrders(ctx, query.BrokerID, history, OrderWriteMetadata{
		DiscoveredEventType: "BROKER_HISTORY_DISCOVERED",
		UpdatedEventType:    "BROKER_HISTORY_UPDATED",
		Source:              "broker",
		SourceDetail:        "broker.history",
	})
	w.syncOrderFees(ctx, query, history)
}

func (w *OrderUpdatesWorker) HandleOrderUpdate(order Order) {
	if w == nil || w.execution == nil {
		return
	}
	brokerID := strings.TrimSpace(order.BrokerID)
	if brokerID == "" {
		brokerID = w.config.BrokerID
	}
	query := queryForOrder(brokerID, order.AccountID, order.TradingEnvironment, order.Market)
	key := OrderUpdateSubscriptionKey(query)
	w.markSubscriptions([]OrderQuery{query}, "active", "push-order", nil)
	if IsTerminalOrderStatus(order.Status) {
		w.removeActiveOrder(key, order.BrokerOrderID, order.BrokerOrderIDEx)
	} else {
		w.upsertActiveOrder(key, order)
	}
	w.execution.ApplyOrder(context.Background(), query.BrokerID, cloneOrder(order), OrderWriteMetadata{
		DiscoveredEventType: "BROKER_PUSH_DISCOVERED",
		UpdatedEventType:    "BROKER_PUSH_ORDER",
		Source:              "broker",
		SourceDetail:        "broker.push",
	})
	if IsTerminalOrderStatus(order.Status) {
		w.syncOrderFees(context.Background(), query, []Order{order})
	}
}

func (w *OrderUpdatesWorker) syncOrderFees(ctx context.Context, query OrderQuery, orders []Order) {
	source, ok := w.source.(OrderFeeSource)
	if !ok {
		return
	}
	sink, ok := w.execution.(ExecutionOrderFeeUpdates)
	if !ok {
		return
	}
	orderIDs := make([]string, 0, len(orders))
	seen := make(map[string]struct{}, len(orders))
	for _, order := range orders {
		if !IsTerminalOrderStatus(order.Status) || order.BrokerOrderIDEx == nil {
			continue
		}
		orderID := strings.TrimSpace(*order.BrokerOrderIDEx)
		if orderID == "" {
			continue
		}
		if _, exists := seen[orderID]; exists {
			continue
		}
		seen[orderID] = struct{}{}
		orderIDs = append(orderIDs, orderID)
	}
	if len(orderIDs) == 0 {
		return
	}
	fees, err := source.OrderFees(ctx, query, orderIDs)
	if err != nil {
		w.markSubscriptions([]OrderQuery{query}, "inactive", "sync-order-fees", err)
		return
	}
	sink.ApplyFees(ctx, query.BrokerID, fees)
}

func (w *OrderUpdatesWorker) HandleFillUpdate(fill Fill) {
	if w == nil || w.execution == nil {
		return
	}
	brokerID := strings.TrimSpace(fill.BrokerID)
	if brokerID == "" {
		brokerID = w.config.BrokerID
	}
	query := queryForOrder(brokerID, fill.AccountID, fill.TradingEnvironment, fill.Market)
	w.markSubscriptions([]OrderQuery{query}, "active", "push-fill", nil)
	w.execution.ApplyFill(context.Background(), query.BrokerID, cloneFill(fill))
}

func (w *OrderUpdatesWorker) Stop() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	subscription := w.pushSubscription
	w.pushSubscription = nil
	w.pushSubscriptionKey = ""
	w.subscriptionEpoch++
	if subscription == nil && w.lastStoppedAt != nil {
		w.mu.Unlock()
		return nil
	}
	now := w.now().Format(time.RFC3339Nano)
	w.lastStoppedAt = &now
	w.stoppedSubscriptions = new(len(w.subscriptions))
	w.lastSyncAt = time.Time{}
	w.activeOrdersCache = make(map[string][]Order)
	w.activeOrdersCachedAt = make(map[string]time.Time)
	for _, state := range w.subscriptions {
		state.Status = "inactive"
		state.LastAction = "stopped"
		state.LastActionAt = now
	}
	w.mu.Unlock()
	if subscription != nil {
		return subscription.Stop()
	}
	return nil
}

func (w *OrderUpdatesWorker) ensureSubscribed(ctx context.Context, accounts []Account, queries []OrderQuery) error {
	key := orderUpdatePushSubscriptionKey(accounts, queries)
	attempt, err := w.beginSubscriptionAttempt(ctx, key)
	if err != nil || attempt.op == subscribeNoop {
		return err
	}
	err = w.performSubscriptionAttempt(ctx, &attempt, accounts, queries)
	return w.finishSubscriptionAttempt(attempt, key, queries, err)
}

func (w *OrderUpdatesWorker) beginSubscriptionAttempt(ctx context.Context, key string) (orderUpdateSubscriptionAttempt, error) {
	for {
		attempt, ready, waiting := w.tryBeginSubscriptionAttempt(key)
		if !waiting {
			return attempt, nil
		}
		select {
		case <-ctx.Done():
			return orderUpdateSubscriptionAttempt{}, ctx.Err()
		case <-ready:
		}
	}
}

func (w *OrderUpdatesWorker) tryBeginSubscriptionAttempt(key string) (orderUpdateSubscriptionAttempt, chan struct{}, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	attempt := w.subscriptionAttemptLocked(key)
	if attempt.op == subscribeNoop {
		return attempt, nil, false
	}
	if !w.subscriptionPending {
		w.subscriptionPending = true
		w.subscriptionReady = make(chan struct{})
		attempt.epoch = w.subscriptionEpoch
		return attempt, nil, false
	}
	return orderUpdateSubscriptionAttempt{}, w.subscriptionReady, true
}

func (w *OrderUpdatesWorker) subscriptionAttemptLocked(key string) orderUpdateSubscriptionAttempt {
	attempt := orderUpdateSubscriptionAttempt{existing: w.pushSubscription}
	switch {
	case attempt.existing == nil:
		attempt.op = subscribeFresh
	case w.pushSubscriptionKey == key:
		if refresher, ok := attempt.existing.(orderUpdateSubscriptionRefresher); ok {
			attempt.op = subscribeRefresh
			attempt.refresher = refresher
		} else {
			attempt.op = subscribeNoop
		}
	default:
		if refresher, ok := attempt.existing.(orderUpdateSubscriptionRefresher); ok {
			attempt.op = subscribeRefresh
			attempt.refresher = refresher
		} else {
			attempt.op = subscribeFresh
		}
	}
	return attempt
}

func (w *OrderUpdatesWorker) performSubscriptionAttempt(ctx context.Context, attempt *orderUpdateSubscriptionAttempt, accounts []Account, queries []OrderQuery) error {
	clonedAccounts := cloneAccounts(accounts)
	clonedQueries := append([]OrderQuery(nil), queries...)
	if attempt.op == subscribeRefresh {
		return attempt.refresher.Refresh(ctx, clonedAccounts, clonedQueries)
	}
	subscription, err := w.source.Subscribe(ctx, clonedAccounts, clonedQueries, w)
	attempt.subscription = subscription
	return err
}

func (w *OrderUpdatesWorker) finishSubscriptionAttempt(attempt orderUpdateSubscriptionAttempt, key string, queries []OrderQuery, err error) error {
	w.completeSubscriptionAttempt(&attempt, key, err)
	w.stopSubscriptionSilently(attempt.old)
	w.stopSubscriptionSilently(attempt.subscription)
	if err != nil {
		return err
	}
	if attempt.installed {
		w.markSubscriptions(queries, "active", "subscribe-push", nil)
	}
	return nil
}

func (w *OrderUpdatesWorker) completeSubscriptionAttempt(attempt *orderUpdateSubscriptionAttempt, key string, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err == nil && w.subscriptionEpoch == attempt.epoch {
		switch attempt.op {
		case subscribeRefresh:
			if w.pushSubscription == attempt.existing {
				w.pushSubscriptionKey = key
				attempt.installed = true
			}
		case subscribeFresh:
			attempt.old = w.pushSubscription
			w.pushSubscription = attempt.subscription
			w.pushSubscriptionKey = key
			attempt.subscription = nil
			attempt.installed = true
		}
	}
	w.subscriptionPending = false
	close(w.subscriptionReady)
	w.subscriptionReady = nil
}

func (w *OrderUpdatesWorker) stopSubscriptionSilently(subscription OrderUpdateSubscription) {
	if subscription != nil {
		besteffort.LogError(subscription.Stop())
	}
}
