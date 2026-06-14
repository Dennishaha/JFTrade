package trading

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
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
	AccountID          string
	TradingEnvironment string
	Market             string
	BrokerOrderID      string
	BrokerOrderIDEx    *string
	Symbol             string
	SymbolName         *string
	Side               string
	OrderType          string
	Status             string
	Quantity           float64
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

type ExecutionOrderUpdates interface {
	ApplyOrder(context.Context, string, Order, OrderWriteMetadata)
	ApplyFill(context.Context, string, Fill)
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

func NewOrderUpdatesWorker(source OrderUpdateSource, execution ExecutionOrderUpdates, config OrderUpdatesConfig) *OrderUpdatesWorker {
	if config.BrokerID == "" {
		config.BrokerID = "futu"
	}
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
	queries := BuildOrderUpdateQueries(accounts, w.config.FallbackMarket)
	w.markDiscoveredAccounts(len(accounts), "connected")
	if err := w.ensureSubscribed(ctx, accounts, queries); err != nil {
		w.markSubscriptions(queries, "inactive", "bind-push", err)
	}

	for _, query := range queries {
		key := OrderUpdateSubscriptionKey(query)
		if activeOnly {
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
	}
}

func (w *OrderUpdatesWorker) HandleOrderUpdate(order Order) {
	if w == nil || w.execution == nil {
		return
	}
	query := queryForOrder(w.config.BrokerID, order.AccountID, order.TradingEnvironment, order.Market)
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
}

func (w *OrderUpdatesWorker) HandleFillUpdate(fill Fill) {
	if w == nil || w.execution == nil {
		return
	}
	query := queryForOrder(w.config.BrokerID, fill.AccountID, fill.TradingEnvironment, fill.Market)
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
	stopped := len(w.subscriptions)
	w.stoppedSubscriptions = &stopped
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

func (w *OrderUpdatesWorker) SnapshotResponse() map[string]any {
	if w == nil {
		return map[string]any{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	keys := make([]string, 0, len(w.subscriptions))
	for key := range w.subscriptions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	subscriptions := make([]any, 0, len(keys))
	latestAction := "idle"
	latestActionAt := w.now().Format(time.RFC3339Nano)
	active, inactive := 0, 0
	for _, key := range keys {
		state := w.subscriptions[key]
		if state == nil {
			continue
		}
		if state.Status == "active" {
			active++
		} else {
			inactive++
		}
		if state.LastActionAt > latestActionAt || latestAction == "idle" {
			latestAction = state.LastAction
			latestActionAt = state.LastActionAt
		}
		subscriptions = append(subscriptions, map[string]any{
			"subscriptionKey": state.SubscriptionKey, "brokerId": state.BrokerID,
			"tradingEnvironment": state.TradingEnvironment, "accountId": state.AccountID, "market": state.Market,
			"status": state.Status, "lastAction": state.LastAction, "lastActionAt": state.LastActionAt,
			"lastError": state.LastError, "lastErrorContext": nil, "consecutiveFailures": state.ConsecutiveFailures,
			"retryDelayMs": state.RetryDelayMs, "backoffUntil": state.BackoffUntil,
		})
	}
	invalidations := make([]any, 0, len(w.recentInvalidations))
	for _, invalidation := range w.recentInvalidations {
		invalidations = append(invalidations, map[string]any{
			"subscriptionKey": invalidation.SubscriptionKey, "brokerId": invalidation.BrokerID,
			"tradingEnvironment": invalidation.TradingEnvironment, "accountId": invalidation.AccountID,
			"market": invalidation.Market, "kind": invalidation.Kind, "message": invalidation.Message,
			"errorContext": nil, "consecutiveFailures": nil, "retryDelayMs": nil, "backoffUntil": nil,
			"createdAt": invalidation.CreatedAt,
		})
	}
	brokers := []any{}
	if len(keys) > 0 || w.accountsDiscovered > 0 || w.connectivity != nil {
		brokers = append(brokers, map[string]any{
			"brokerId": w.config.BrokerID, "lastAction": latestAction, "lastActionAt": latestActionAt,
			"connectivity": w.connectivity, "lastError": latestSubscriptionError(w.subscriptions),
			"accountsDiscovered": nullablePositiveInt(w.accountsDiscovered), "activeSubscriptions": active,
			"retryingSubscriptions": 0, "inactiveSubscriptions": inactive, "backoffSubscriptions": 0,
			"disconnectedBackoffSubscriptions": 0, "subscribeFailedBackoffSubscriptions": 0,
			"errorBackoffSubscriptions": 0, "dominantBackoffSource": nil, "dominantBackoffCount": 0,
			"longestBackoffSource": nil, "longestBackoffRemainingMs": nil,
			"longestBackoffSubscriptionKey": nil, "longestBackoffMarket": nil,
			"longestBackoffTradingEnvironment": nil, "longestBackoffAccountId": nil,
			"topBackoffHotspots": []any{}, "layeredBackoffSummaries": []any{},
			"recentInvalidationCount": len(w.recentInvalidations),
			"lastInvalidationKind":    latestInvalidationKind(w.recentInvalidations),
			"lastInvalidationAt":      latestInvalidationAt(w.recentInvalidations),
			"backoffActive":           false, "backoffSource": nil, "backoffUntil": nil, "backoffRemainingMs": nil,
		})
	}
	return map[string]any{
		"subscriptions": subscriptions, "recentInvalidations": invalidations, "brokers": brokers,
		"runtime": map[string]any{"lastStoppedAt": w.lastStoppedAt, "stoppedSubscriptions": w.stoppedSubscriptions},
	}
}

func BuildOrderUpdateQueries(accounts []Account, fallbackMarket string) []OrderQuery {
	queries := make([]OrderQuery, 0, len(accounts))
	seen := make(map[string]struct{})
	for _, account := range accounts {
		markets := append([]string(nil), account.MarketAuthorities...)
		if len(markets) == 0 {
			markets = []string{fallbackMarket}
		}
		for _, market := range markets {
			query := OrderQuery{
				BrokerID: account.BrokerID, TradingEnvironment: strings.TrimSpace(account.TradingEnvironment),
				AccountID: strings.TrimSpace(account.ID), Market: strings.ToUpper(strings.TrimSpace(market)),
			}
			key := OrderUpdateSubscriptionKey(query)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			queries = append(queries, query)
		}
	}
	if len(queries) == 0 && strings.TrimSpace(fallbackMarket) != "" {
		queries = append(queries, OrderQuery{BrokerID: "futu", TradingEnvironment: "SIMULATE", Market: strings.ToUpper(strings.TrimSpace(fallbackMarket))})
	}
	sort.Slice(queries, func(i, j int) bool {
		return OrderUpdateSubscriptionKey(queries[i]) < OrderUpdateSubscriptionKey(queries[j])
	})
	return queries
}

func OrderUpdateSubscriptionKey(query OrderQuery) string {
	return strings.Join([]string{
		strings.TrimSpace(query.BrokerID), strings.ToUpper(strings.TrimSpace(query.TradingEnvironment)),
		strings.TrimSpace(query.AccountID), strings.ToUpper(strings.TrimSpace(query.Market)),
	}, "|")
}

func IsTerminalOrderStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FILLED_ALL", "CANCELLED_ALL", "CANCELLED_PART", "FAILED", "DELETED", "EXPIRED":
		return true
	default:
		return false
	}
}

func (w *OrderUpdatesWorker) shouldSync(force bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	now := w.now()
	if force || w.lastSyncAt.IsZero() || now.Sub(w.lastSyncAt) >= w.config.SyncMinInterval {
		w.lastSyncAt = now
		return true
	}
	return false
}

func (w *OrderUpdatesWorker) ensureSubscribed(ctx context.Context, accounts []Account, queries []OrderQuery) error {
	key := orderUpdatePushSubscriptionKey(accounts, queries)
	type subscribeOp int
	const (
		subscribeFresh subscribeOp = iota
		subscribeRefresh
	)

	var (
		op           subscribeOp
		epoch        uint64
		existing     OrderUpdateSubscription
		refresher    orderUpdateSubscriptionRefresher
		subscription OrderUpdateSubscription
		old          OrderUpdateSubscription
		installed    bool
	)

	for {
		w.mu.Lock()
		existing = w.pushSubscription
		if existing != nil {
			if r, ok := existing.(orderUpdateSubscriptionRefresher); ok {
				refresher = r
				op = subscribeRefresh
			} else if w.pushSubscriptionKey == key {
				w.mu.Unlock()
				return nil
			} else {
				op = subscribeFresh
			}
		} else {
			op = subscribeFresh
		}
		if !w.subscriptionPending {
			w.subscriptionPending = true
			w.subscriptionReady = make(chan struct{})
			epoch = w.subscriptionEpoch
			w.mu.Unlock()
			break
		}
		ready := w.subscriptionReady
		w.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ready:
		}
	}

	clonedAccounts := cloneAccounts(accounts)
	clonedQueries := append([]OrderQuery(nil), queries...)
	var err error
	if op == subscribeRefresh {
		err = refresher.Refresh(ctx, clonedAccounts, clonedQueries)
	} else {
		subscription, err = w.source.Subscribe(ctx, clonedAccounts, clonedQueries, w)
	}

	w.mu.Lock()
	if err == nil && w.subscriptionEpoch == epoch {
		switch op {
		case subscribeRefresh:
			if w.pushSubscription == existing {
				w.pushSubscriptionKey = key
				installed = true
			}
		case subscribeFresh:
			old = w.pushSubscription
			w.pushSubscription = subscription
			w.pushSubscriptionKey = key
			subscription = nil
			installed = true
		}
	}
	w.subscriptionPending = false
	close(w.subscriptionReady)
	w.subscriptionReady = nil
	w.mu.Unlock()
	if old != nil {
		_ = old.Stop()
	}
	if subscription != nil {
		_ = subscription.Stop()
	}
	if err != nil {
		return err
	}
	if installed {
		w.markSubscriptions(queries, "active", "subscribe-push", nil)
	}
	return nil
}

func orderUpdatePushSubscriptionKey(accounts []Account, queries []OrderQuery) string {
	accountKeys := make([]string, 0, len(accounts))
	for _, account := range accounts {
		markets := make([]string, 0, len(account.MarketAuthorities))
		for _, market := range account.MarketAuthorities {
			if trimmed := strings.ToUpper(strings.TrimSpace(market)); trimmed != "" {
				markets = append(markets, trimmed)
			}
		}
		sort.Strings(markets)
		accountKeys = append(accountKeys, strings.Join([]string{
			strings.TrimSpace(account.BrokerID),
			strings.ToUpper(strings.TrimSpace(account.TradingEnvironment)),
			strings.TrimSpace(account.ID),
			strings.Join(markets, ","),
		}, "|"))
	}
	sort.Strings(accountKeys)

	queryKeys := make([]string, 0, len(queries))
	for _, query := range queries {
		queryKeys = append(queryKeys, OrderUpdateSubscriptionKey(query))
	}
	sort.Strings(queryKeys)
	return strings.Join([]string{
		strings.Join(accountKeys, ";"),
		strings.Join(queryKeys, ";"),
	}, "\n")
}

func (w *OrderUpdatesWorker) applyOrders(ctx context.Context, brokerID string, orders []Order, metadata OrderWriteMetadata) {
	for _, order := range orders {
		w.execution.ApplyOrder(ctx, brokerID, cloneOrder(order), metadata)
	}
}

func (w *OrderUpdatesWorker) markSubscriptions(queries []OrderQuery, status, action string, err error) {
	now := w.now().Format(time.RFC3339Nano)
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, query := range queries {
		key := OrderUpdateSubscriptionKey(query)
		state := w.subscriptions[key]
		if state == nil {
			state = &orderUpdateSubscriptionState{
				SubscriptionKey: key, BrokerID: query.BrokerID,
				TradingEnvironment: stringPointer(query.TradingEnvironment), AccountID: stringPointer(query.AccountID),
				Market: stringPointer(query.Market),
			}
			w.subscriptions[key] = state
		}
		state.Status, state.LastAction, state.LastActionAt = status, action, now
		if err == nil {
			state.LastError, state.ConsecutiveFailures, state.RetryDelayMs, state.BackoffUntil = nil, nil, nil, nil
			continue
		}
		message := strings.TrimSpace(err.Error())
		state.LastError = stringPointer(message)
		failures := 1
		if state.ConsecutiveFailures != nil {
			failures = *state.ConsecutiveFailures + 1
		}
		state.ConsecutiveFailures = &failures
		kind := "ERROR"
		lower := strings.ToLower(message)
		if strings.Contains(lower, "dial") || strings.Contains(lower, "closed") || strings.Contains(lower, "timeout") {
			kind = "DISCONNECTED"
		}
		w.recentInvalidations = append(w.recentInvalidations, orderUpdateInvalidation{
			SubscriptionKey: key, BrokerID: query.BrokerID,
			TradingEnvironment: stringPointer(query.TradingEnvironment), AccountID: stringPointer(query.AccountID),
			Market: stringPointer(query.Market), Kind: kind, Message: stringPointer(message), CreatedAt: now,
		})
		if len(w.recentInvalidations) > maxOrderUpdateInvalidations {
			w.recentInvalidations = append([]orderUpdateInvalidation(nil), w.recentInvalidations[len(w.recentInvalidations)-maxOrderUpdateInvalidations:]...)
		}
	}
}

func (w *OrderUpdatesWorker) markDiscoveredAccounts(count int, connectivity string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.accountsDiscovered = count
	w.connectivity = stringPointer(connectivity)
}

func (w *OrderUpdatesWorker) storeActiveOrders(key string, orders []Order) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.activeOrdersCache[key] = cloneOrders(orders)
	w.activeOrdersCachedAt[key] = w.now()
}

func (w *OrderUpdatesWorker) cachedActiveOrders(key string) ([]Order, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cachedAt, ok := w.activeOrdersCachedAt[key]
	if !ok || w.now().Sub(cachedAt) > w.config.CacheTTL {
		return nil, false
	}
	orders, ok := w.activeOrdersCache[key]
	if !ok {
		return nil, false
	}
	return cloneOrders(orders), true
}

func (w *OrderUpdatesWorker) upsertActiveOrder(key string, order Order) {
	w.mu.Lock()
	defer w.mu.Unlock()
	orders := w.activeOrdersCache[key]
	for i, existing := range orders {
		if sameOrder(existing, order.BrokerOrderID, order.BrokerOrderIDEx) {
			orders[i] = cloneOrder(order)
			w.activeOrdersCache[key] = orders
			return
		}
	}
	w.activeOrdersCache[key] = append(orders, cloneOrder(order))
	w.activeOrdersCachedAt[key] = w.now()
}

func (w *OrderUpdatesWorker) removeActiveOrder(key, orderID string, orderIDEx *string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	orders, ok := w.activeOrdersCache[key]
	if !ok {
		return
	}
	filtered := make([]Order, 0, len(orders))
	for _, order := range orders {
		if !sameOrder(order, orderID, orderIDEx) {
			filtered = append(filtered, cloneOrder(order))
		}
	}
	w.activeOrdersCache[key] = filtered
}

func (w *OrderUpdatesWorker) now() time.Time {
	return w.config.Now().UTC()
}

func queryForOrder(brokerID, accountID, environment, market string) OrderQuery {
	return OrderQuery{BrokerID: brokerID, AccountID: accountID, TradingEnvironment: environment, Market: market}
}

func sameOrder(order Order, orderID string, orderIDEx *string) bool {
	if orderIDEx != nil && order.BrokerOrderIDEx != nil && strings.TrimSpace(*orderIDEx) == strings.TrimSpace(*order.BrokerOrderIDEx) {
		return true
	}
	return strings.TrimSpace(orderID) != "" && strings.TrimSpace(orderID) == strings.TrimSpace(order.BrokerOrderID)
}

func cloneAccounts(accounts []Account) []Account {
	out := make([]Account, len(accounts))
	for i, account := range accounts {
		out[i] = account
		out[i].MarketAuthorities = append([]string(nil), account.MarketAuthorities...)
	}
	return out
}

func cloneOrders(orders []Order) []Order {
	out := make([]Order, len(orders))
	for i, order := range orders {
		out[i] = cloneOrder(order)
	}
	return out
}

func cloneOrder(order Order) Order {
	order.BrokerOrderIDEx = cloneString(order.BrokerOrderIDEx)
	order.SymbolName = cloneString(order.SymbolName)
	order.FilledQuantity = cloneFloat(order.FilledQuantity)
	order.Price = cloneFloat(order.Price)
	order.FilledAveragePrice = cloneFloat(order.FilledAveragePrice)
	order.Remark = cloneString(order.Remark)
	order.LastError = cloneString(order.LastError)
	order.TimeInForce = cloneString(order.TimeInForce)
	order.Currency = cloneString(order.Currency)
	return order
}

func cloneFill(fill Fill) Fill {
	fill.BrokerOrderIDEx = cloneString(fill.BrokerOrderIDEx)
	fill.BrokerFillIDEx = cloneString(fill.BrokerFillIDEx)
	fill.SymbolName = cloneString(fill.SymbolName)
	fill.FillPrice = cloneFloat(fill.FillPrice)
	fill.Status = cloneString(fill.Status)
	return fill
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func stringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func nullablePositiveInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func latestSubscriptionError(states map[string]*orderUpdateSubscriptionState) *string {
	var latest *string
	latestAt := ""
	for _, state := range states {
		if state != nil && state.LastError != nil && state.LastActionAt >= latestAt {
			latestAt, latest = state.LastActionAt, state.LastError
		}
	}
	return latest
}

func latestInvalidationKind(invalidations []orderUpdateInvalidation) any {
	if len(invalidations) == 0 {
		return nil
	}
	return invalidations[len(invalidations)-1].Kind
}

func latestInvalidationAt(invalidations []orderUpdateInvalidation) any {
	if len(invalidations) == 0 {
		return nil
	}
	return invalidations[len(invalidations)-1].CreatedAt
}
