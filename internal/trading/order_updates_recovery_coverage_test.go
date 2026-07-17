package trading

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCoverage98ExecutionOrderHistoryFailsClosedForInvalidOrUnavailableBackfill(t *testing.T) {
	var nilWorker *OrderUpdatesWorker
	nilWorker.SyncExecutionOrderHistory(t.Context(), ExecutionOrder{})
	if err := nilWorker.Stop(); err != nil {
		t.Fatalf("nil Stop: %v", err)
	}

	now := time.Date(2026, time.July, 17, 12, 0, 0, 0, time.UTC)
	source := &fakeOrderUpdateSource{historyErr: errors.New("history endpoint unavailable")}
	worker := NewOrderUpdatesWorker(source, &fakeExecutionOrderUpdates{}, OrderUpdatesConfig{
		Now:             func() time.Time { return now },
		HistoryLookback: func() int { return 0 },
	})

	// A locally-created order has no broker reference and must not trigger a
	// history request. A malformed broker-backed order is equally ignored.
	worker.SyncExecutionOrderHistory(t.Context(), ExecutionOrder{BrokerID: "futu"})
	brokerOrderID := "broker-1"
	worker.SyncExecutionOrderHistory(t.Context(), ExecutionOrder{BrokerOrderID: &brokerOrderID, BrokerID: "futu"})
	if source.historyCalls != 0 {
		t.Fatalf("invalid history backfills = %d", source.historyCalls)
	}

	worker.SyncExecutionOrderHistory(t.Context(), ExecutionOrder{
		BrokerOrderID: &brokerOrderID,
		BrokerID:      "futu", TradingEnvironment: "SIMULATE", AccountID: "ACC-1", Market: "US",
	})
	if source.historyCalls != 1 {
		t.Fatalf("history backfills = %d, want 1", source.historyCalls)
	}
	subscriptions := jftradeCheckedTypeAssertion[[]any](worker.SnapshotResponse()["subscriptions"])
	if len(subscriptions) != 1 {
		t.Fatalf("history failure subscriptions = %#v", subscriptions)
	}
	state := jftradeCheckedTypeAssertion[map[string]any](subscriptions[0])
	if state["status"] != "inactive" || state["lastAction"] != "sync-history-orders" {
		t.Fatalf("history failure state = %#v", state)
	}
}

type blockingSubscriptionSource struct {
	fakeOrderUpdateSource
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingSubscriptionSource) Subscribe(_ context.Context, _ []Account, _ []OrderQuery, handler OrderUpdateHandler) (OrderUpdateSubscription, error) {
	s.mu.Lock()
	s.subscribeCalls++
	s.handler = handler
	s.mu.Unlock()
	s.once.Do(func() { close(s.started) })
	<-s.release
	return &fakeOrderUpdateSubscription{}, nil
}

func TestCoverage98ConcurrentSubscriptionWaitHonorsCallerCancellation(t *testing.T) {
	source := &blockingSubscriptionSource{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	worker := NewOrderUpdatesWorker(source, &fakeExecutionOrderUpdates{}, OrderUpdatesConfig{})
	accounts := []Account{{ID: "ACC-1", BrokerID: "futu", TradingEnvironment: "SIMULATE", MarketAuthorities: []string{"US"}}}
	queries := BuildOrderUpdateQueries(accounts, "US")

	firstDone := make(chan error, 1)
	go func() { firstDone <- worker.ensureSubscribed(context.Background(), accounts, queries) }()
	<-source.started

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := worker.ensureSubscribed(cancelled, accounts, queries); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting subscription error = %v, want context canceled", err)
	}

	close(source.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first subscription: %v", err)
	}
}
