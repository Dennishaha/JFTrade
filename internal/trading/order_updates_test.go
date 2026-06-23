package trading

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeOrderUpdateSource struct {
	mu             sync.Mutex
	accounts       []Account
	current        []Order
	history        []Order
	discoverErr    error
	currentCalls   int
	historyCalls   int
	subscribeCalls int
	handler        OrderUpdateHandler
	subscription   *fakeOrderUpdateSubscription
	subscriptions  []*fakeOrderUpdateSubscription
}

func (f *fakeOrderUpdateSource) DiscoverAccounts(context.Context) ([]Account, error) {
	return cloneAccounts(f.accounts), f.discoverErr
}

func (f *fakeOrderUpdateSource) CurrentOrders(context.Context, OrderQuery) ([]Order, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.currentCalls++
	return cloneOrders(f.current), nil
}

func (f *fakeOrderUpdateSource) HistoryOrders(context.Context, OrderQuery, time.Time, time.Time) ([]Order, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.historyCalls++
	return cloneOrders(f.history), nil
}

func (f *fakeOrderUpdateSource) Subscribe(_ context.Context, _ []Account, _ []OrderQuery, handler OrderUpdateHandler) (OrderUpdateSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subscribeCalls++
	f.handler = handler
	f.subscription = &fakeOrderUpdateSubscription{}
	f.subscriptions = append(f.subscriptions, f.subscription)
	return f.subscription, nil
}

type fakeOrderUpdateSubscription struct {
	mu    sync.Mutex
	stops int
}

func (s *fakeOrderUpdateSubscription) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stops++
	return nil
}

type fakeRefreshOrderUpdateSubscription struct {
	fakeOrderUpdateSubscription
	mu           sync.Mutex
	refreshCalls int
	accountIDs   []string
}

func (s *fakeRefreshOrderUpdateSubscription) Refresh(_ context.Context, accounts []Account, _ []OrderQuery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshCalls++
	s.accountIDs = s.accountIDs[:0]
	for _, account := range accounts {
		s.accountIDs = append(s.accountIDs, account.ID)
	}
	return nil
}

type fakeRefreshOrderUpdateSource struct {
	fakeOrderUpdateSource
	refreshSubscription *fakeRefreshOrderUpdateSubscription
}

func (f *fakeRefreshOrderUpdateSource) Subscribe(_ context.Context, _ []Account, _ []OrderQuery, handler OrderUpdateHandler) (OrderUpdateSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subscribeCalls++
	f.handler = handler
	if f.refreshSubscription == nil {
		f.refreshSubscription = &fakeRefreshOrderUpdateSubscription{}
	}
	return f.refreshSubscription, nil
}

type appliedOrder struct {
	order    Order
	metadata OrderWriteMetadata
}

type fakeExecutionOrderUpdates struct {
	mu     sync.Mutex
	orders []appliedOrder
	fills  []Fill
}

func (f *fakeExecutionOrderUpdates) ApplyOrder(_ context.Context, _ string, order Order, metadata OrderWriteMetadata) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.orders = append(f.orders, appliedOrder{order: cloneOrder(order), metadata: metadata})
}

func (f *fakeExecutionOrderUpdates) ApplyFill(_ context.Context, _ string, fill Fill) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fills = append(f.fills, cloneFill(fill))
}

func TestOrderUpdatesWorkerThrottleForceAndSubscribeOnce(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	source := &fakeOrderUpdateSource{accounts: []Account{{
		ID: "1001", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"HK"},
	}}}
	execution := &fakeExecutionOrderUpdates{}
	worker := NewOrderUpdatesWorker(source, execution, OrderUpdatesConfig{Now: func() time.Time { return now }})

	worker.Sync(context.Background(), false, false)
	worker.Sync(context.Background(), false, false)
	if source.currentCalls != 1 || source.historyCalls != 1 {
		t.Fatalf("throttled calls current=%d history=%d", source.currentCalls, source.historyCalls)
	}
	worker.Sync(context.Background(), true, false)
	if source.currentCalls != 2 || source.historyCalls != 2 {
		t.Fatalf("forced calls current=%d history=%d", source.currentCalls, source.historyCalls)
	}
	if source.subscribeCalls != 1 {
		t.Fatalf("subscribe calls = %d, want 1", source.subscribeCalls)
	}
}

func TestOrderUpdatesWorkerCacheTTLTerminalRemovalAndDefensiveCopy(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	source := &fakeOrderUpdateSource{
		accounts: []Account{{ID: "1001", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"HK"}}},
		current:  []Order{{AccountID: "1001", TradingEnvironment: "SIMULATE", Market: "HK", BrokerOrderID: "1", Status: "SUBMITTED", Price: new(100.0)}},
	}
	execution := &fakeExecutionOrderUpdates{}
	worker := NewOrderUpdatesWorker(source, execution, OrderUpdatesConfig{
		Now: func() time.Time { return now }, CacheTTL: time.Minute,
	})

	worker.Sync(context.Background(), true, true)
	*source.current[0].Price = 999
	execution.orders = nil
	now = now.Add(30 * time.Second)
	worker.Sync(context.Background(), true, true)
	if source.currentCalls != 1 {
		t.Fatalf("current calls with live cache = %d, want 1", source.currentCalls)
	}
	if got := *execution.orders[0].order.Price; got != 100 {
		t.Fatalf("cached price = %v, want defensive copy 100", got)
	}
	*execution.orders[0].order.Price = 777
	execution.orders = nil
	worker.Sync(context.Background(), true, true)
	if got := *execution.orders[0].order.Price; got != 100 {
		t.Fatalf("cached price after consumer mutation = %v, want 100", got)
	}

	worker.HandleOrderUpdate(Order{
		AccountID: "1001", TradingEnvironment: "SIMULATE", Market: "HK",
		BrokerOrderID: "1", Status: "CANCELLED_ALL",
	})
	execution.orders = nil
	worker.Sync(context.Background(), true, true)
	if len(execution.orders) != 0 {
		t.Fatalf("terminal order remained in cache: %#v", execution.orders)
	}

	now = now.Add(61 * time.Second)
	worker.Sync(context.Background(), true, true)
	if source.currentCalls != 2 {
		t.Fatalf("current calls after TTL = %d, want 2", source.currentCalls)
	}
}

func TestOrderUpdatesWorkerCurrentHistoryCacheAndPushMetadata(t *testing.T) {
	now := time.Date(2026, 6, 14, 10, 0, 0, 0, time.UTC)
	source := &fakeOrderUpdateSource{
		accounts: []Account{{ID: "1001", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"HK"}}},
		current:  []Order{{BrokerOrderID: "current"}},
		history:  []Order{{BrokerOrderID: "history"}},
	}
	execution := &fakeExecutionOrderUpdates{}
	worker := NewOrderUpdatesWorker(source, execution, OrderUpdatesConfig{Now: func() time.Time { return now }})

	worker.Sync(context.Background(), true, false)
	worker.Sync(context.Background(), true, true)
	worker.HandleOrderUpdate(Order{BrokerOrderID: "push", AccountID: "1001", TradingEnvironment: "SIMULATE", Market: "HK"})
	worker.HandleFillUpdate(Fill{BrokerFillID: "fill", AccountID: "1001", TradingEnvironment: "SIMULATE", Market: "HK"})

	want := []OrderWriteMetadata{
		{DiscoveredEventType: "BROKER_SYNC_DISCOVERED", UpdatedEventType: "BROKER_SYNC_UPDATED", Source: "broker", SourceDetail: "broker.current"},
		{DiscoveredEventType: "BROKER_HISTORY_DISCOVERED", UpdatedEventType: "BROKER_HISTORY_UPDATED", Source: "broker", SourceDetail: "broker.history"},
		{DiscoveredEventType: "BROKER_CACHE_DISCOVERED", UpdatedEventType: "BROKER_CACHE_UPDATED", Source: "broker", SourceDetail: "broker.cache"},
		{DiscoveredEventType: "BROKER_PUSH_DISCOVERED", UpdatedEventType: "BROKER_PUSH_ORDER", Source: "broker", SourceDetail: "broker.push"},
	}
	if len(execution.orders) != len(want) {
		t.Fatalf("applied orders = %#v", execution.orders)
	}
	for i := range want {
		if execution.orders[i].metadata != want[i] {
			t.Fatalf("metadata[%d] = %#v, want %#v", i, execution.orders[i].metadata, want[i])
		}
	}
	if len(execution.fills) != 1 || execution.fills[0].BrokerFillID != "fill" {
		t.Fatalf("fills = %#v", execution.fills)
	}
}

func TestOrderUpdatesWorkerStopIsIdempotentAndCanResubscribe(t *testing.T) {
	source := &fakeOrderUpdateSource{accounts: []Account{{
		ID: "1001", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"HK"},
	}}}
	worker := NewOrderUpdatesWorker(source, &fakeExecutionOrderUpdates{}, OrderUpdatesConfig{})
	worker.Sync(context.Background(), true, true)
	if err := worker.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := worker.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	if source.subscription.stops != 1 {
		t.Fatalf("subscription stops = %d, want 1", source.subscription.stops)
	}
	worker.Sync(context.Background(), true, true)
	if source.subscribeCalls != 2 {
		t.Fatalf("subscribe calls after restart = %d, want 2", source.subscribeCalls)
	}
}

func TestOrderUpdatesWorkerResubscribesWhenAccountSetChanges(t *testing.T) {
	source := &fakeOrderUpdateSource{accounts: []Account{{
		ID: "1001", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"HK"},
	}}}
	worker := NewOrderUpdatesWorker(source, &fakeExecutionOrderUpdates{}, OrderUpdatesConfig{})

	worker.Sync(context.Background(), true, true)
	source.accounts = append(source.accounts, Account{
		ID: "1002", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"HK"},
	})
	worker.Sync(context.Background(), true, true)

	if source.subscribeCalls != 2 {
		t.Fatalf("subscribe calls after account change = %d, want 2", source.subscribeCalls)
	}
	if len(source.subscriptions) != 2 {
		t.Fatalf("subscriptions = %d, want 2", len(source.subscriptions))
	}
	if source.subscriptions[0].stops != 1 {
		t.Fatalf("old subscription stops = %d, want 1", source.subscriptions[0].stops)
	}
	if source.subscriptions[1].stops != 0 {
		t.Fatalf("new subscription stops = %d, want 0", source.subscriptions[1].stops)
	}
}

func TestOrderUpdatesWorkerRefreshesExistingSubscriptionOnSync(t *testing.T) {
	source := &fakeRefreshOrderUpdateSource{fakeOrderUpdateSource: fakeOrderUpdateSource{accounts: []Account{{
		ID: "1001", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"HK"},
	}}}}
	worker := NewOrderUpdatesWorker(source, &fakeExecutionOrderUpdates{}, OrderUpdatesConfig{})

	worker.Sync(context.Background(), true, true)
	worker.Sync(context.Background(), true, true)
	if source.refreshSubscription.refreshCalls != 1 {
		t.Fatalf("refresh calls for unchanged account set = %d, want 1", source.refreshSubscription.refreshCalls)
	}

	source.accounts = append(source.accounts, Account{
		ID: "1002", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"HK"},
	})
	worker.Sync(context.Background(), true, true)

	if source.subscribeCalls != 1 {
		t.Fatalf("subscribe calls = %d, want 1", source.subscribeCalls)
	}
	if source.refreshSubscription.refreshCalls != 2 {
		t.Fatalf("refresh calls = %d, want 2", source.refreshSubscription.refreshCalls)
	}
	if got := source.refreshSubscription.accountIDs; len(got) != 2 || got[0] != "1001" || got[1] != "1002" {
		t.Fatalf("refresh account ids = %#v, want [1001 1002]", got)
	}
}

func TestOrderUpdatesWorkerConcurrentSyncSubscribesOnce(t *testing.T) {
	source := &fakeOrderUpdateSource{accounts: []Account{{
		ID: "1001", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"HK"},
	}}}
	worker := NewOrderUpdatesWorker(source, &fakeExecutionOrderUpdates{}, OrderUpdatesConfig{})
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			worker.Sync(context.Background(), true, true)
		})
	}
	wg.Wait()
	if source.subscribeCalls != 1 {
		t.Fatalf("concurrent subscribe calls = %d, want 1", source.subscribeCalls)
	}
}

func TestOrderUpdatesWorkerSnapshotCapsInvalidations(t *testing.T) {
	source := &fakeOrderUpdateSource{discoverErr: errors.New("dial timeout")}
	worker := NewOrderUpdatesWorker(source, &fakeExecutionOrderUpdates{}, OrderUpdatesConfig{})
	for range 25 {
		worker.Sync(context.Background(), true, false)
	}
	snapshot := worker.SnapshotResponse()
	invalidations := jftradeCheckedTypeAssertion[[]any](snapshot["recentInvalidations"])
	if len(invalidations) != 20 {
		t.Fatalf("invalidations = %d, want 20", len(invalidations))
	}
}

func TestOrderUpdatesWorkerInactiveSourcePreservesDiagnosticState(t *testing.T) {
	source := &fakeOrderUpdateSource{discoverErr: ErrOrderUpdateSourceInactive}
	worker := NewOrderUpdatesWorker(source, &fakeExecutionOrderUpdates{}, OrderUpdatesConfig{})
	worker.Sync(context.Background(), true, false)
	snapshot := worker.SnapshotResponse()
	if got := len(jftradeCheckedTypeAssertion[[]any](snapshot["subscriptions"])); got != 0 {
		t.Fatalf("subscriptions = %d, want 0", got)
	}
	brokers := jftradeCheckedTypeAssertion[[]any](snapshot["brokers"])
	if len(brokers) != 1 || jftradeCheckedTypeAssertion[map[string]any](brokers[0])["connectivity"] == nil ||
		*jftradeCheckedTypeAssertion[*string](jftradeCheckedTypeAssertion[map[string]any](brokers[0])["connectivity"]) != "inactive" {
		t.Fatalf("brokers = %#v", brokers)
	}
}

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}
