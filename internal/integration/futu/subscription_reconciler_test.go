package futu

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	pkgfutu "github.com/jftrade/jftrade-main/pkg/futu"
)

type fakePhysicalSubscriptionExchange struct {
	calls       []string
	failures    map[string][]error
	quota       pkgfutu.SubscriptionQuota
	quotaCalls  int
	quotaErrors []error
}

func (f *fakePhysicalSubscriptionExchange) call(name string) error {
	f.calls = append(f.calls, name)
	queued := f.failures[name]
	if len(queued) == 0 {
		return nil
	}
	err := queued[0]
	f.failures[name] = queued[1:]
	return err
}

func (f *fakePhysicalSubscriptionExchange) SubscribeBasicQuote(_ context.Context, symbol string, push bool) error {
	return f.call("subscribe-basic:" + symbol + ":" + map[bool]string{true: "push", false: "no-push"}[push])
}

func (f *fakePhysicalSubscriptionExchange) UnsubscribeBasicQuote(_ context.Context, symbol string) error {
	return f.call("unsubscribe-basic:" + symbol)
}

func (f *fakePhysicalSubscriptionExchange) SubscribeKLine(_ context.Context, symbol string, interval bbgotypes.Interval) error {
	return f.call("subscribe-kline:" + symbol + ":" + string(interval))
}

func (f *fakePhysicalSubscriptionExchange) UnsubscribeKLine(_ context.Context, symbol string, interval bbgotypes.Interval) error {
	return f.call("unsubscribe-kline:" + symbol + ":" + string(interval))
}

func (f *fakePhysicalSubscriptionExchange) QuerySubscriptionQuota(context.Context) (pkgfutu.SubscriptionQuota, error) {
	f.quotaCalls++
	if len(f.quotaErrors) > 0 {
		err := f.quotaErrors[0]
		f.quotaErrors = f.quotaErrors[1:]
		return pkgfutu.SubscriptionQuota{}, err
	}
	return f.quota, nil
}

func TestSubscriptionReconcilerSharesExactPhysicalSubscriptionsAndDefersFinalRelease(t *testing.T) {
	now := time.Date(2026, time.July, 15, 1, 2, 3, 0, time.UTC)
	exchange := &fakePhysicalSubscriptionExchange{quota: pkgfutu.SubscriptionQuota{TotalUsed: 7, Remaining: 93, OwnUsed: 3}}
	reconciler := newMarketDataSubscriptionReconciler(func() physicalSubscriptionExchange { return exchange }, func() time.Time { return now })
	desired := []marketdata.InstrumentRef{
		{Channel: "KLINE", Market: "hk", Symbol: "00700", Interval: "1m"},
		{Channel: "kline", Market: "HK", Symbol: "HK.00700", Interval: "1M"},
		{Channel: "KLINE", Market: "HK", Symbol: "00700", Interval: "5m"},
		{Channel: "TICK", Market: "HK", Symbol: "00700"},
	}
	if err := reconciler.ReconcileSubscriptions(context.Background(), desired); err != nil {
		t.Fatalf("initial reconcile: %v", err)
	}
	wantCalls := []string{
		"subscribe-basic:HK.00700:push",
		"subscribe-kline:HK.00700:1m",
		"subscribe-kline:HK.00700:5m",
	}
	if !reflect.DeepEqual(exchange.calls, wantCalls) {
		t.Fatalf("physical calls = %#v, want %#v", exchange.calls, wantCalls)
	}
	state := reconciler.SubscriptionState()
	if state["desiredCount"] != 3 || state["ownActiveCount"] != 3 || state["pendingReleaseCount"] != 0 {
		t.Fatalf("state counts = %#v", state)
	}
	if state["totalUsedQuota"] != 7 || state["remainQuota"] != 93 || state["ownUsedQuota"] != 3 {
		t.Fatalf("quota state = %#v", state)
	}

	exchange.calls = nil
	if err := reconciler.ReconcileSubscriptions(nil, nil); err != nil {
		t.Fatalf("early release reconcile: %v", err)
	}
	if len(exchange.calls) != 0 {
		t.Fatalf("released before minimum age: %#v", exchange.calls)
	}
	state = reconciler.SubscriptionState()
	if state["pendingReleaseCount"] != 3 {
		t.Fatalf("pending release state = %#v", state)
	}

	if err := reconciler.ReconcileSubscriptions(context.Background(), desired[:1]); err != nil {
		t.Fatalf("reacquire while pending: %v", err)
	}
	if len(exchange.calls) != 0 {
		t.Fatalf("pending subscription should be reused: %#v", exchange.calls)
	}
	state = reconciler.SubscriptionState()
	if state["pendingReleaseCount"] != 1 {
		t.Fatalf("only 5m should remain pending: %#v", state)
	}

	now = now.Add(time.Minute)
	if err := reconciler.ReconcileSubscriptions(context.Background(), desired[:1]); err != nil {
		t.Fatalf("eligible partial release: %v", err)
	}
	if !reflect.DeepEqual(exchange.calls, []string{"unsubscribe-kline:HK.00700:5m"}) {
		t.Fatalf("partial unsubscribe calls = %#v", exchange.calls)
	}
	exchange.calls = nil
	if err := reconciler.ReconcileSubscriptions(context.Background(), nil); err != nil {
		t.Fatalf("eligible final release: %v", err)
	}
	if !reflect.DeepEqual(exchange.calls, []string{"unsubscribe-basic:HK.00700", "unsubscribe-kline:HK.00700:1m"}) {
		t.Fatalf("final unsubscribe calls = %#v", exchange.calls)
	}
	if state := reconciler.SubscriptionState(); state["ownActiveCount"] != 0 || state["pendingReleaseCount"] != 0 {
		t.Fatalf("final state = %#v", state)
	}
}

func TestSubscriptionReconcilerConcurrentReconcileIsIdempotent(t *testing.T) {
	now := time.Date(2026, time.July, 15, 5, 0, 0, 0, time.UTC)
	exchange := &fakePhysicalSubscriptionExchange{}
	reconciler := newMarketDataSubscriptionReconciler(
		func() physicalSubscriptionExchange { return exchange },
		func() time.Time { return now },
	)
	desired := []marketdata.InstrumentRef{{Channel: "KLINE", Market: "US", Symbol: "AAPL", Interval: "1m"}}

	runConcurrentReconciles(t, reconciler, desired, 64)
	if want := []string{"subscribe-basic:US.AAPL:push", "subscribe-kline:US.AAPL:1m"}; !reflect.DeepEqual(exchange.calls, want) {
		t.Fatalf("concurrent subscribe calls = %#v, want %#v", exchange.calls, want)
	}

	now = now.Add(minimumFutuSubscriptionAge)
	runConcurrentReconciles(t, reconciler, nil, 64)
	want := []string{
		"subscribe-basic:US.AAPL:push",
		"subscribe-kline:US.AAPL:1m",
		"unsubscribe-basic:US.AAPL",
		"unsubscribe-kline:US.AAPL:1m",
	}
	if !reflect.DeepEqual(exchange.calls, want) {
		t.Fatalf("concurrent final release calls = %#v, want %#v", exchange.calls, want)
	}
	state := reconciler.SubscriptionState()
	if state["desiredCount"] != 0 || state["ownActiveCount"] != 0 || state["pendingReleaseCount"] != 0 {
		t.Fatalf("concurrent final state = %#v", state)
	}
}

func runConcurrentReconciles(t *testing.T, reconciler *marketDataSubscriptionReconciler, desired []marketdata.InstrumentRef, workers int) {
	t.Helper()
	start := make(chan struct{})
	errors := make(chan error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			<-start
			errors <- reconciler.ReconcileSubscriptions(context.Background(), desired)
		}()
	}
	close(start)
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent reconcile: %v", err)
		}
	}
}

func TestSubscriptionReconcilerRetriesFailuresAndCancelsRetryOnReacquire(t *testing.T) {
	now := time.Date(2026, time.July, 15, 2, 0, 0, 0, time.UTC)
	subscribeErr := errors.New("subscribe denied")
	unsubscribeErr := errors.New("unsubscribe busy")
	exchange := &fakePhysicalSubscriptionExchange{failures: map[string][]error{
		"subscribe-basic:US.AAPL:push": {subscribeErr},
	}}
	reconciler := newMarketDataSubscriptionReconciler(func() physicalSubscriptionExchange { return exchange }, func() time.Time { return now })
	desired := []marketdata.InstrumentRef{{Channel: "KLINE", Market: "US", Symbol: "AAPL", Interval: "1m"}}

	err := reconciler.ReconcileSubscriptions(context.Background(), desired)
	if !errors.Is(err, subscribeErr) || !strings.Contains(err.Error(), "subscribe BASIC:US.AAPL") {
		t.Fatalf("subscribe error = %v", err)
	}
	if state := reconciler.SubscriptionState(); state["ownActiveCount"] != 1 || state["lastError"] != nil {
		t.Fatalf("subscribe retry state = %#v", state)
	}
	if err := reconciler.ReconcileSubscriptions(context.Background(), desired); err != nil {
		t.Fatalf("retry before deadline should be deferred: %v", err)
	}
	if len(exchange.calls) != 2 {
		t.Fatalf("unexpected early retry calls = %#v", exchange.calls)
	}
	now = now.Add(5 * time.Second)
	if err := reconciler.ReconcileSubscriptions(context.Background(), desired); err != nil {
		t.Fatalf("subscribe retry: %v", err)
	}
	if len(exchange.calls) != 3 {
		t.Fatalf("basic and kline should subscribe after retry: %#v", exchange.calls)
	}

	now = now.Add(time.Minute)
	exchange.failures["unsubscribe-basic:US.AAPL"] = []error{unsubscribeErr, unsubscribeErr, unsubscribeErr, unsubscribeErr, unsubscribeErr}
	for attempt, delay := range []time.Duration{5 * time.Second, 10 * time.Second, 20 * time.Second, 30 * time.Second, 30 * time.Second} {
		err = reconciler.ReconcileSubscriptions(context.Background(), nil)
		if !errors.Is(err, unsubscribeErr) {
			t.Fatalf("unsubscribe attempt %d error = %v", attempt, err)
		}
		if got := reconciler.records["BASIC:US.AAPL"].retryAt; !got.Equal(now.Add(delay)) {
			t.Fatalf("retry %d at %s, want %s", attempt, got, now.Add(delay))
		}
		if err := reconciler.ReconcileSubscriptions(context.Background(), nil); err != nil {
			t.Fatalf("deferred attempt %d: %v", attempt, err)
		}
		now = now.Add(delay)
	}

	if err := reconciler.ReconcileSubscriptions(context.Background(), desired); err != nil {
		t.Fatalf("reacquire cancels unsubscribe retry: %v", err)
	}
	record := reconciler.records["BASIC:US.AAPL"]
	if record.failures != 0 || !record.retryAt.IsZero() || record.lastError != "" {
		t.Fatalf("reacquired record retained retry state: %#v", record)
	}
	if subscriptionRetryDelay(-1) != 5*time.Second || subscriptionRetryDelay(99) != 30*time.Second {
		t.Fatal("retry delay bounds are incorrect")
	}
}

func TestSubscriptionReconcilerHandlesQuotaExchangeReplacementResetAndNilBoundaries(t *testing.T) {
	now := time.Date(2026, time.July, 15, 3, 0, 0, 0, time.UTC)
	quotaErr := errors.New("quota unavailable")
	first := &fakePhysicalSubscriptionExchange{quotaErrors: []error{quotaErr}, quota: pkgfutu.SubscriptionQuota{TotalUsed: 1}}
	second := &fakePhysicalSubscriptionExchange{quota: pkgfutu.SubscriptionQuota{TotalUsed: 2, Remaining: 98, OwnUsed: 1}}
	current := physicalSubscriptionExchange(first)
	reconciler := newMarketDataSubscriptionReconciler(func() physicalSubscriptionExchange { return current }, func() time.Time { return now })
	desired := []marketdata.InstrumentRef{{Market: "US", Symbol: "MSFT"}}
	if err := reconciler.ReconcileSubscriptions(context.Background(), desired); err != nil {
		t.Fatalf("quota failure must be diagnostic only: %v", err)
	}
	if state := reconciler.SubscriptionState(); state["lastError"] != quotaErr.Error() || state["checkedAt"] == nil {
		t.Fatalf("quota failure state = %#v", state)
	}
	if err := reconciler.ReconcileSubscriptions(context.Background(), desired); err != nil || first.quotaCalls != 1 {
		t.Fatalf("quota refresh throttle = err %v calls %d", err, first.quotaCalls)
	}

	current = second
	if err := reconciler.ReconcileSubscriptions(context.Background(), desired); err != nil {
		t.Fatalf("replacement reconcile: %v", err)
	}
	if !reflect.DeepEqual(second.calls, []string{"subscribe-basic:US.MSFT:push"}) || second.quotaCalls != 1 {
		t.Fatalf("replacement state = calls %#v quota %d", second.calls, second.quotaCalls)
	}
	current = nil
	if err := reconciler.ReconcileSubscriptions(context.Background(), nil); err != nil {
		t.Fatalf("nil replacement without desired subscriptions: %v", err)
	}
	current = second

	reconciler.ResetPhysicalSubscriptions()
	if state := reconciler.SubscriptionState(); state["ownActiveCount"] != 0 || state["checkedAt"] != nil || state["lastError"] != nil {
		t.Fatalf("reset state = %#v", state)
	}
	current = nil
	if err := reconciler.ReconcileSubscriptions(context.Background(), desired); err == nil {
		t.Fatal("nil exchange with desired subscriptions should fail")
	}
	if err := reconciler.ReconcileSubscriptions(context.Background(), nil); err != nil {
		t.Fatalf("nil exchange without physical work: %v", err)
	}

	var nilReconciler *marketDataSubscriptionReconciler
	if err := nilReconciler.ReconcileSubscriptions(nil, desired); err != nil || nilReconciler.SubscriptionState() != nil {
		t.Fatalf("nil reconciler boundary = state %#v err %v", nilReconciler.SubscriptionState(), err)
	}
	nilReconciler.ResetPhysicalSubscriptions()
	if nullableString("") != nil || nullableString("x") != "x" || nullableTime(time.Time{}) != nil || nullableTime(now) == nil {
		t.Fatal("nullable diagnostics helpers returned unexpected values")
	}
}

func TestSubscriptionReconcilerDropsFailedRecordsReleasedBeforeRetry(t *testing.T) {
	now := time.Date(2026, time.July, 15, 4, 0, 0, 0, time.UTC)
	exchange := &fakePhysicalSubscriptionExchange{failures: map[string][]error{
		"subscribe-basic:US.AAPL:push": {errors.New("denied")},
	}}
	reconciler := newMarketDataSubscriptionReconciler(
		func() physicalSubscriptionExchange { return exchange },
		nil,
	)
	reconciler.now = func() time.Time { return now }
	if err := reconciler.ReconcileSubscriptions(context.Background(), []marketdata.InstrumentRef{{Market: "US", Symbol: "AAPL"}}); err == nil {
		t.Fatal("initial subscribe error = nil")
	}
	if err := reconciler.ReconcileSubscriptions(context.Background(), nil); err != nil {
		t.Fatalf("release failed record: %v", err)
	}
	if len(reconciler.records) != 0 {
		t.Fatalf("failed records after release = %#v", reconciler.records)
	}
}

func TestDesiredPhysicalSubscriptionsRejectsIncompleteRefsAndNormalizesSymbols(t *testing.T) {
	physical, logical := desiredPhysicalSubscriptions([]marketdata.InstrumentRef{
		{},
		{Channel: "ORDER_BOOK", Symbol: "us.nvda"},
		{Channel: "KLINE", Market: "US", Symbol: "NVDA"},
	})
	if logical != 2 || len(physical) != 1 || physical["BASIC:US.NVDA"].instrument != "US.NVDA" {
		t.Fatalf("desired physical = %#v logical=%d", physical, logical)
	}
	if market, symbol := normalizedInstrument("", "bad"); market != "" || symbol != "BAD" {
		t.Fatalf("unqualified normalization = %q/%q", market, symbol)
	}
}
