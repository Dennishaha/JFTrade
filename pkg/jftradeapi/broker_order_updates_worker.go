package jftradeapi

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/futu/opend"
	trdcommonpb "github.com/jftrade/jftrade-main/pkg/futu/pb/trdcommon"
)

const brokerOrderUpdateSyncMinInterval = 1500 * time.Millisecond

type brokerOrderUpdateSubscription struct {
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

type brokerOrderUpdateInvalidation struct {
	SubscriptionKey    string
	BrokerID           string
	TradingEnvironment *string
	AccountID          *string
	Market             *string
	Kind               string
	Message            *string
	CreatedAt          string
}

type brokerOrderUpdateWorker struct {
	mu                   sync.Mutex
	subscriptions        map[string]*brokerOrderUpdateSubscription
	recentInvalidations  []brokerOrderUpdateInvalidation
	lastStoppedAt        *string
	stoppedSubscriptions *int
	lastSyncAt           time.Time
	accountsDiscovered   int
	connectivity         *string
	boundClient          *opend.Client
}

func newBrokerOrderUpdateWorker() *brokerOrderUpdateWorker {
	return &brokerOrderUpdateWorker{subscriptions: make(map[string]*brokerOrderUpdateSubscription)}
}

func buildBrokerOrderUpdateQueries(accounts []broker.Account, fallbackMarket string) []broker.ReadQuery {
	queries := make([]broker.ReadQuery, 0, len(accounts))
	seen := make(map[string]struct{})
	for _, account := range accounts {
		markets := append([]string(nil), account.MarketAuthorities...)
		if len(markets) == 0 {
			markets = []string{fallbackMarket}
		}
		for _, market := range markets {
			query := broker.ReadQuery{
				BrokerID:           account.BrokerID,
				TradingEnvironment: strings.TrimSpace(account.TradingEnvironment),
				AccountID:          strings.TrimSpace(account.ID),
				Market:             strings.ToUpper(strings.TrimSpace(market)),
			}
			key := brokerOrderUpdateSubscriptionKey(query)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			queries = append(queries, query)
		}
	}
	if len(queries) == 0 && strings.TrimSpace(fallbackMarket) != "" {
		queries = append(queries, broker.ReadQuery{BrokerID: "futu", TradingEnvironment: "SIMULATE", Market: fallbackMarket})
	}
	sort.Slice(queries, func(i, j int) bool {
		left := brokerOrderUpdateSubscriptionKey(queries[i])
		right := brokerOrderUpdateSubscriptionKey(queries[j])
		return left < right
	})
	return queries
}

func brokerOrderUpdateSubscriptionKey(query broker.ReadQuery) string {
	return strings.Join([]string{
		query.BrokerID,
		strings.ToUpper(strings.TrimSpace(query.TradingEnvironment)),
		strings.TrimSpace(query.AccountID),
		strings.ToUpper(strings.TrimSpace(query.Market)),
	}, "|")
}

func (w *brokerOrderUpdateWorker) shouldSync(force bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if force {
		w.lastSyncAt = time.Now().UTC()
		return true
	}
	if !w.lastSyncAt.IsZero() && time.Since(w.lastSyncAt) < brokerOrderUpdateSyncMinInterval {
		return false
	}
	w.lastSyncAt = time.Now().UTC()
	return true
}

func (w *brokerOrderUpdateWorker) markSubscriptions(queries []broker.ReadQuery, status string, action string, err error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, query := range queries {
		key := brokerOrderUpdateSubscriptionKey(query)
		subscription := w.subscriptions[key]
		if subscription == nil {
			subscription = &brokerOrderUpdateSubscription{
				SubscriptionKey:    key,
				BrokerID:           query.BrokerID,
				TradingEnvironment: stringPointerOrNil(query.TradingEnvironment),
				AccountID:          stringPointerOrNil(query.AccountID),
				Market:             stringPointerOrNil(query.Market),
			}
			w.subscriptions[key] = subscription
		}
		subscription.Status = status
		subscription.LastAction = action
		subscription.LastActionAt = now
		if err != nil {
			message := strings.TrimSpace(err.Error())
			subscription.LastError = stringPointerOrNil(message)
			failures := 1
			if subscription.ConsecutiveFailures != nil {
				failures = *subscription.ConsecutiveFailures + 1
			}
			subscription.ConsecutiveFailures = &failures
			kind := "ERROR"
			if strings.Contains(strings.ToLower(message), "dial") || strings.Contains(strings.ToLower(message), "closed") || strings.Contains(strings.ToLower(message), "timeout") {
				kind = "DISCONNECTED"
			}
			w.recentInvalidations = append(w.recentInvalidations, brokerOrderUpdateInvalidation{
				SubscriptionKey:    key,
				BrokerID:           query.BrokerID,
				TradingEnvironment: stringPointerOrNil(query.TradingEnvironment),
				AccountID:          stringPointerOrNil(query.AccountID),
				Market:             stringPointerOrNil(query.Market),
				Kind:               kind,
				Message:            stringPointerOrNil(message),
				CreatedAt:          now,
			})
			if len(w.recentInvalidations) > 20 {
				w.recentInvalidations = append([]brokerOrderUpdateInvalidation(nil), w.recentInvalidations[len(w.recentInvalidations)-20:]...)
			}
		} else {
			subscription.LastError = nil
			subscription.ConsecutiveFailures = nil
			subscription.RetryDelayMs = nil
			subscription.BackoffUntil = nil
		}
	}
}

func (w *brokerOrderUpdateWorker) markDiscoveredAccounts(count int, connectivity string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.accountsDiscovered = count
	w.connectivity = stringPointerOrNil(connectivity)
}

func (w *brokerOrderUpdateWorker) shouldBindClient(client *opend.Client) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.boundClient == client {
		return false
	}
	w.boundClient = client
	return true
}

func (w *brokerOrderUpdateWorker) markStopped() {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastStoppedAt = &now
	stopped := len(w.subscriptions)
	w.stoppedSubscriptions = &stopped
	w.boundClient = nil
	w.lastSyncAt = time.Time{}
	for _, subscription := range w.subscriptions {
		subscription.Status = "inactive"
		subscription.LastAction = "stopped"
		subscription.LastActionAt = now
	}
}

func (w *brokerOrderUpdateWorker) markPush(query broker.ReadQuery, action string) {
	w.markSubscriptions([]broker.ReadQuery{query}, "active", action, nil)
}

func (w *brokerOrderUpdateWorker) snapshotResponse() map[string]any {
	w.mu.Lock()
	defer w.mu.Unlock()

	subscriptionKeys := make([]string, 0, len(w.subscriptions))
	for key := range w.subscriptions {
		subscriptionKeys = append(subscriptionKeys, key)
	}
	sort.Strings(subscriptionKeys)
	subscriptions := make([]any, 0, len(subscriptionKeys))
	latestAction := "idle"
	latestActionAt := time.Now().UTC().Format(time.RFC3339Nano)
	activeSubscriptions := 0
	inactiveSubscriptions := 0
	for _, key := range subscriptionKeys {
		subscription := w.subscriptions[key]
		if subscription == nil {
			continue
		}
		if subscription.Status == "active" {
			activeSubscriptions++
		} else {
			inactiveSubscriptions++
		}
		if subscription.LastActionAt > latestActionAt || latestAction == "idle" {
			latestAction = subscription.LastAction
			latestActionAt = subscription.LastActionAt
		}
		subscriptions = append(subscriptions, map[string]any{
			"subscriptionKey":     subscription.SubscriptionKey,
			"brokerId":            subscription.BrokerID,
			"tradingEnvironment":  subscription.TradingEnvironment,
			"accountId":           subscription.AccountID,
			"market":              subscription.Market,
			"status":              subscription.Status,
			"lastAction":          subscription.LastAction,
			"lastActionAt":        subscription.LastActionAt,
			"lastError":           subscription.LastError,
			"lastErrorContext":    nil,
			"consecutiveFailures": subscription.ConsecutiveFailures,
			"retryDelayMs":        subscription.RetryDelayMs,
			"backoffUntil":        subscription.BackoffUntil,
		})
	}

	recentInvalidations := make([]any, 0, len(w.recentInvalidations))
	for _, invalidation := range w.recentInvalidations {
		recentInvalidations = append(recentInvalidations, map[string]any{
			"subscriptionKey":     invalidation.SubscriptionKey,
			"brokerId":            invalidation.BrokerID,
			"tradingEnvironment":  invalidation.TradingEnvironment,
			"accountId":           invalidation.AccountID,
			"market":              invalidation.Market,
			"kind":                invalidation.Kind,
			"message":             invalidation.Message,
			"errorContext":        nil,
			"consecutiveFailures": nil,
			"retryDelayMs":        nil,
			"backoffUntil":        nil,
			"createdAt":           invalidation.CreatedAt,
		})
	}

	brokers := []any{}
	if len(subscriptionKeys) > 0 || w.accountsDiscovered > 0 || w.connectivity != nil {
		brokers = append(brokers, map[string]any{
			"brokerId":                            "futu",
			"lastAction":                          latestAction,
			"lastActionAt":                        latestActionAt,
			"connectivity":                        w.connectivity,
			"lastError":                           latestSubscriptionError(w.subscriptions),
			"accountsDiscovered":                  nullableInt(w.accountsDiscovered),
			"activeSubscriptions":                 activeSubscriptions,
			"retryingSubscriptions":               0,
			"inactiveSubscriptions":               inactiveSubscriptions,
			"backoffSubscriptions":                0,
			"disconnectedBackoffSubscriptions":    0,
			"subscribeFailedBackoffSubscriptions": 0,
			"errorBackoffSubscriptions":           0,
			"dominantBackoffSource":               nil,
			"dominantBackoffCount":                0,
			"longestBackoffSource":                nil,
			"longestBackoffRemainingMs":           nil,
			"longestBackoffSubscriptionKey":       nil,
			"longestBackoffMarket":                nil,
			"longestBackoffTradingEnvironment":    nil,
			"longestBackoffAccountId":             nil,
			"topBackoffHotspots":                  []any{},
			"layeredBackoffSummaries":             []any{},
			"recentInvalidationCount":             len(w.recentInvalidations),
			"lastInvalidationKind":                latestInvalidationKind(w.recentInvalidations),
			"lastInvalidationAt":                  latestInvalidationAt(w.recentInvalidations),
			"backoffActive":                       false,
			"backoffSource":                       nil,
			"backoffUntil":                        nil,
			"backoffRemainingMs":                  nil,
		})
	}

	return map[string]any{
		"subscriptions":       subscriptions,
		"recentInvalidations": recentInvalidations,
		"brokers":             brokers,
		"runtime": map[string]any{
			"lastStoppedAt":        w.lastStoppedAt,
			"stoppedSubscriptions": w.stoppedSubscriptions,
		},
	}
}

func nullableInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

func latestSubscriptionError(subscriptions map[string]*brokerOrderUpdateSubscription) *string {
	var latest *string
	latestAt := ""
	for _, subscription := range subscriptions {
		if subscription == nil || subscription.LastError == nil {
			continue
		}
		if subscription.LastActionAt >= latestAt {
			latestAt = subscription.LastActionAt
			latest = subscription.LastError
		}
	}
	return latest
}

func latestInvalidationKind(invalidations []brokerOrderUpdateInvalidation) any {
	if len(invalidations) == 0 {
		return nil
	}
	return invalidations[len(invalidations)-1].Kind
}

func latestInvalidationAt(invalidations []brokerOrderUpdateInvalidation) any {
	if len(invalidations) == 0 {
		return nil
	}
	return invalidations[len(invalidations)-1].CreatedAt
}

func (s *Server) syncBrokerOrderUpdates(ctx context.Context, force bool) {
	if s.brokerOrderUpdates == nil || !s.brokerOrderUpdates.shouldSync(force) {
		return
	}
	fallbackMarket := strings.ToUpper(strings.TrimSpace(s.store.integration().Config.TradeMarket))
	if fallbackMarket == "" {
		fallbackMarket = "HK"
	}

	// Use the broker interface for account discovery.
	activeBroker := s.activeBroker()
	if activeBroker == nil {
		s.brokerOrderUpdates.markDiscoveredAccounts(0, "inactive")
		return
	}
	accounts, err := activeBroker.DiscoverAccounts(ctx)
	if err != nil {
		query := broker.ReadQuery{BrokerID: "futu", TradingEnvironment: "SIMULATE", Market: fallbackMarket}
		s.brokerOrderUpdates.markSubscriptions([]broker.ReadQuery{query}, "inactive", "discover-accounts", err)
		return
	}
	queries := buildBrokerOrderUpdateQueries(accounts, fallbackMarket)
	s.brokerOrderUpdates.markDiscoveredAccounts(len(accounts), "connected")

	// Bind push notifications — this still requires the Futu-specific client for now.
	futuAccounts := convertBrokerAccountsToFutu(accounts)
	if err := s.bindBrokerOrderUpdatePush(ctx, futuAccounts, convertBrokerReadQueriesToFutu(queries)); err != nil {
		s.brokerOrderUpdates.markSubscriptions(queries, "inactive", "bind-push", err)
	}

	// Sync orders via broker interface.
	reader := activeBroker.MarketData()
	if reader == nil {
		return
	}
	for _, query := range queries {
		orders, err := reader.QueryOrders(ctx, query, "")
		if err != nil {
			s.brokerOrderUpdates.markSubscriptions([]broker.ReadQuery{query}, "inactive", "sync-orders", err)
			continue
		}
		s.brokerOrderUpdates.markSubscriptions([]broker.ReadQuery{query}, "active", "sync-orders", nil)
		for _, order := range orders {
			updated, event, changed := s.executionOrders.upsertBrokerOrder("futu", order, "BROKER_SYNC_DISCOVERED", "BROKER_SYNC_UPDATED")
			if changed {
				s.notifyExecutionOrderLifecycle(updated, event)
			}
		}
	}
}

func (s *Server) bindBrokerOrderUpdatePush(ctx context.Context, accounts []futu.RuntimeAccount, queries []futu.BrokerReadQuery) error {
	exchange := s.futuExchange()
	if exchange == nil {
		return nil
	}
	if err := exchange.Connect(ctx); err != nil {
		return err
	}
	client := exchange.Client()
	if client == nil {
		return context.Canceled
	}
	if s.brokerOrderUpdates.shouldBindClient(client) {
		client.SubscribeOrderUpdate(s.handleFutuBrokerOrderPush)
		client.SubscribeOrderFillUpdate(s.handleFutuBrokerOrderFillPush)
	}
	accountIDs := make([]uint64, 0, len(accounts))
	seen := make(map[uint64]struct{})
	for _, account := range accounts {
		parsed, err := strconv.ParseUint(strings.TrimSpace(account.AccountID), 10, 64)
		if err != nil {
			continue
		}
		if _, exists := seen[parsed]; exists {
			continue
		}
		seen[parsed] = struct{}{}
		accountIDs = append(accountIDs, parsed)
	}
	if len(accountIDs) > 0 {
		if err := client.SubscribeAccountPush(ctx, accountIDs); err != nil {
			return err
		}
	}
	// Mark subscriptions using broker.ReadQuery types.
	brokerQueries := convertFutuReadQueriesToBroker(queries)
	s.brokerOrderUpdates.markSubscriptions(brokerQueries, "active", "subscribe-push", nil)
	return nil
}

func (s *Server) handleFutuBrokerOrderPush(header *trdcommonpb.TrdHeader, order *trdcommonpb.Order) {
	futuSnapshot := futu.BrokerOrderSnapshotFromPush(header, order)
	snapshot := convertFutuOrderSnapshotToBroker(futuSnapshot)
	query := broker.ReadQuery{
		BrokerID:           "futu",
		TradingEnvironment: snapshot.TradingEnvironment,
		AccountID:          snapshot.AccountID,
		Market:             snapshot.Market,
	}
	s.brokerOrderUpdates.markPush(query, "push-order")
	updated, event, changed := s.executionOrders.upsertBrokerOrder("futu", snapshot, "BROKER_PUSH_DISCOVERED", "BROKER_PUSH_ORDER")
	if changed {
		s.notifyExecutionOrderLifecycle(updated, event)
	}
}

func (s *Server) handleFutuBrokerOrderFillPush(header *trdcommonpb.TrdHeader, fill *trdcommonpb.OrderFill) {
	futuSnapshot := futu.BrokerOrderFillSnapshotFromPush(header, fill)
	snapshot := convertFutuOrderFillSnapshotToBroker(futuSnapshot)
	query := broker.ReadQuery{
		BrokerID:           "futu",
		TradingEnvironment: snapshot.TradingEnvironment,
		AccountID:          snapshot.AccountID,
		Market:             snapshot.Market,
	}
	s.brokerOrderUpdates.markPush(query, "push-fill")
	updated, event, changed := s.executionOrders.recordBrokerOrderFill("futu", snapshot)
	if changed {
		s.notifyExecutionOrderLifecycle(updated, event)
	}
}

// --- Conversion helpers for the transition period ---

func convertBrokerAccountsToFutu(accounts []broker.Account) []futu.RuntimeAccount {
	result := make([]futu.RuntimeAccount, len(accounts))
	for i, a := range accounts {
		result[i] = futu.RuntimeAccount{
			AccountID:            a.ID,
			TradingEnvironment:   a.TradingEnvironment,
			AccountType:          a.AccountType,
			AccountRole:          a.AccountRole,
			SecurityFirm:         a.SecurityFirm,
			MarketAuthorities:    a.MarketAuthorities,
			SimulatedAccountType: a.SimulatedAccountType,
		}
	}
	return result
}

func convertBrokerReadQueriesToFutu(queries []broker.ReadQuery) []futu.BrokerReadQuery {
	result := make([]futu.BrokerReadQuery, len(queries))
	for i, q := range queries {
		result[i] = futu.BrokerReadQuery{
			AccountID:          q.AccountID,
			TradingEnvironment: q.TradingEnvironment,
			Market:             q.Market,
		}
	}
	return result
}

func convertFutuReadQueriesToBroker(queries []futu.BrokerReadQuery) []broker.ReadQuery {
	result := make([]broker.ReadQuery, len(queries))
	for i, q := range queries {
		result[i] = broker.ReadQuery{
			BrokerID:           "futu",
			AccountID:          q.AccountID,
			TradingEnvironment: q.TradingEnvironment,
			Market:             q.Market,
		}
	}
	return result
}

func convertFutuOrderSnapshotToBroker(s futu.BrokerOrderSnapshot) broker.OrderSnapshot {
	return broker.OrderSnapshot{
		AccountID:          s.AccountID,
		TradingEnvironment: s.TradingEnvironment,
		Market:             s.Market,
		BrokerOrderID:      s.BrokerOrderID,
		BrokerOrderIDEx:    s.BrokerOrderIDEx,
		Symbol:             s.Symbol,
		SymbolName:         s.SymbolName,
		Side:               s.Side,
		OrderType:          s.OrderType,
		Status:             s.Status,
		Quantity:           s.Quantity,
		FilledQuantity:     s.FilledQuantity,
		Price:              s.Price,
		FilledAveragePrice: s.FilledAveragePrice,
		SubmittedAt:        s.SubmittedAt,
		UpdatedAt:          s.UpdatedAt,
		Remark:             s.Remark,
		LastError:          s.LastError,
		TimeInForce:        s.TimeInForce,
		Currency:           s.Currency,
	}
}

func convertFutuOrderFillSnapshotToBroker(s futu.BrokerOrderFillSnapshot) broker.OrderFillSnapshot {
	return broker.OrderFillSnapshot{
		AccountID:          s.AccountID,
		TradingEnvironment: s.TradingEnvironment,
		Market:             s.Market,
		BrokerOrderID:      s.BrokerOrderID,
		BrokerOrderIDEx:    s.BrokerOrderIDEx,
		BrokerFillID:       s.BrokerFillID,
		BrokerFillIDEx:     s.BrokerFillIDEx,
		Symbol:             s.Symbol,
		SymbolName:         s.SymbolName,
		Side:               s.Side,
		FilledQuantity:     s.FilledQuantity,
		FillPrice:          s.FillPrice,
		FilledAt:           s.FilledAt,
		Status:             s.Status,
	}
}
