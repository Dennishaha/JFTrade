package marketdata

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeSubscriptionReconciler struct {
	mu        sync.Mutex
	calls     [][]InstrumentRef
	errors    []error
	state     map[string]any
	stateHits int
}

func (f *fakeSubscriptionReconciler) ReconcileSubscriptions(_ context.Context, refs []InstrumentRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]InstrumentRef(nil), refs...))
	if len(f.errors) == 0 {
		return nil
	}
	err := f.errors[0]
	f.errors = f.errors[1:]
	return err
}

func (f *fakeSubscriptionReconciler) SubscriptionState() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stateHits++
	return f.state
}

func (f *fakeSubscriptionReconciler) snapshots() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	lengths := make([]int, 0, len(f.calls))
	for _, call := range f.calls {
		lengths = append(lengths, len(call))
	}
	return lengths
}

func TestSubscriptionRegistryExpiresOnlyWebConsumersAndPreservesManagedLeases(t *testing.T) {
	now := time.Date(2026, time.July, 15, 4, 0, 0, 0, time.UTC)
	registry := newSubscriptionRegistry()
	registry.now = func() time.Time { return now }
	refs := []InstrumentRef{{Channel: "KLINE", Market: "HK", Symbol: "00700", Interval: "1m"}}
	registry.acquire("web-chart", refs)
	registry.acquireManaged("strategy-runtime:one", refs)

	now = now.Add(WebSubscriptionTTL - time.Second)
	if got := registry.snapshot()["totalActiveSubscriptions"]; got != 1 {
		t.Fatalf("subscription expired early: %#v", got)
	}
	registry.heartbeat("web-chart")
	now = now.Add(WebSubscriptionTTL)
	entry := singleSubscriptionEntry(t, registry.snapshot())
	if entry["refCount"] != 1 || !reflect.DeepEqual(entry["consumers"], []string{"strategy-runtime:one"}) {
		t.Fatalf("TTL cleanup = %#v", entry)
	}

	registry.acquire("web-chart", refs)
	registry.clear("")
	entry = singleSubscriptionEntry(t, registry.snapshot())
	if entry["refCount"] != 1 || !reflect.DeepEqual(entry["consumers"], []string{"strategy-runtime:one"}) {
		t.Fatalf("clear all should preserve managed consumers: %#v", entry)
	}
	registry.release("missing", refs[0])
	registry.release("strategy-runtime:one", refs[0])
	if len(registry.activeSubscriptions()) != 0 || len(registry.activeInstruments()) != 0 {
		t.Fatal("final managed release should remove the subscription")
	}
}

func TestSubscriptionRegistryConcurrentAcquireHeartbeatAndRelease(t *testing.T) {
	registry := newSubscriptionRegistry()
	ref := InstrumentRef{Channel: "KLINE", Market: "US", Symbol: "AAPL", Interval: "1m"}
	const consumers = 64

	start := make(chan struct{})
	var acquired sync.WaitGroup
	acquired.Add(consumers)
	for index := range consumers {
		consumerID := "chart-" + strconv.Itoa(index)
		go func() {
			defer acquired.Done()
			<-start
			registry.acquire(consumerID, []InstrumentRef{ref})
			registry.heartbeat(consumerID)
		}()
	}
	close(start)
	acquired.Wait()

	entry := singleSubscriptionEntry(t, registry.snapshot())
	if entry["refCount"] != consumers || len(entry["consumers"].([]string)) != consumers {
		t.Fatalf("concurrent acquire snapshot = %#v", entry)
	}

	start = make(chan struct{})
	var released sync.WaitGroup
	released.Add(consumers)
	for index := range consumers {
		consumerID := "chart-" + strconv.Itoa(index)
		go func() {
			defer released.Done()
			<-start
			registry.heartbeat(consumerID)
			registry.release(consumerID, ref)
		}()
	}
	close(start)
	released.Wait()

	if snapshot := registry.snapshot(); snapshot["totalActiveSubscriptions"] != 0 {
		t.Fatalf("concurrent final release snapshot = %#v", snapshot)
	}
}

func TestManagedSubscriptionConcurrentReleaseIsIdempotent(t *testing.T) {
	releases := make(chan struct{}, 1)
	lease := newManagedSubscription(func() { releases <- struct{}{} })
	const workers = 64
	start := make(chan struct{})
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			<-start
			lease.Release()
		}()
	}
	close(start)
	group.Wait()
	if len(releases) != 1 {
		t.Fatalf("concurrent release count = %d, want 1", len(releases))
	}
}

func TestServiceReconcilesWebAndManagedSubscriptionLifecycles(t *testing.T) {
	ctx := context.Background()
	physicalEntries := []map[string]any{{
		"key": "KLINE:HK.00700:1m", "brokerState": "active", "subscribedAt": "2026-07-15T04:00:00Z",
		"unsubscribeEligibleAt": "2026-07-15T04:01:00Z", "lastError": nil,
	}}
	reconciler := &fakeSubscriptionReconciler{state: map[string]any{
		"desiredCount": 1, "ownActiveCount": 2, "pendingReleaseCount": 0,
		"totalUsedQuota": 8, "remainQuota": 92, "entries": physicalEntries,
	}}
	service := NewService(stubProvider{})
	service.SetSubscriptionReconciler(reconciler)
	refs := []InstrumentRef{{Channel: "KLINE", Market: "hk", Symbol: "00700", Interval: "1M"}}

	result, err := service.AcquireSubscription(ctx, "chart", refs)
	if err != nil {
		t.Fatalf("AcquireSubscription: %v", err)
	}
	if result["desiredCount"] != 1 || result["ownActiveCount"] != 2 || result["totalUsedQuota"] != 8 {
		t.Fatalf("decorated acquire result = %#v", result)
	}
	entry := singleSubscriptionEntry(t, SubscriptionsSnapshot(result))
	if entry["brokerState"] != "active" || entry["subscribedAt"] == nil {
		t.Fatalf("logical broker fields = %#v", entry)
	}

	lease, err := service.AcquireManagedSubscription(ctx, "strategy-runtime:one", refs)
	if err != nil || lease == nil {
		t.Fatalf("AcquireManagedSubscription = %#v, %v", lease, err)
	}
	if err := service.ClearSubscriptions(ctx); err != nil {
		t.Fatalf("ClearSubscriptions(web only): %v", err)
	}
	snapshot, _ := service.GetSubscriptions(ctx)
	entry = singleSubscriptionEntry(t, snapshot)
	if !reflect.DeepEqual(entry["consumers"], []string{"strategy-runtime:one"}) {
		t.Fatalf("managed consumer was cleared by web cleanup: %#v", entry)
	}
	lease.Release()
	lease.Release()
	snapshot, _ = service.GetSubscriptions(ctx)
	if snapshot["totalActiveSubscriptions"] != 0 {
		t.Fatalf("idempotent lease release = %#v", snapshot)
	}
	if got := reconciler.snapshots(); !reflect.DeepEqual(got, []int{1, 1, 1, 0}) {
		t.Fatalf("reconcile desired lengths = %#v", got)
	}
}

func TestServiceRollsBackFailedAcquireAndHandlesDeferredReleaseFailures(t *testing.T) {
	physicalErr := errors.New("physical subscribe failed")
	reconciler := &fakeSubscriptionReconciler{errors: []error{physicalErr, nil}}
	service := NewService(stubProvider{})
	service.SetSubscriptionReconciler(reconciler)
	refs := []InstrumentRef{{Market: "US", Symbol: "AAPL"}}
	if _, err := service.AcquireSubscription(context.Background(), "chart", refs); !errors.Is(err, physicalErr) {
		t.Fatalf("AcquireSubscription error = %v", err)
	}
	snapshot, _ := service.GetSubscriptions(context.Background())
	if snapshot["totalActiveSubscriptions"] != 0 {
		t.Fatalf("failed acquire was not rolled back: %#v", snapshot)
	}

	reconciler.errors = []error{nil, errors.New("release retry later"), errors.New("heartbeat retry later"), errors.New("clear retry later")}
	if _, err := service.AcquireSubscription(context.Background(), "chart", refs); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if err := service.ReleaseSubscription(context.Background(), "chart", refs[0]); err != nil {
		t.Fatalf("logical release should be successful: %v", err)
	}
	if _, err := service.Heartbeat(context.Background(), "chart"); err != nil {
		t.Fatalf("heartbeat diagnostics: %v", err)
	}
	if err := service.ClearSubscriptions(context.Background(), "chart"); err != nil {
		t.Fatalf("clear diagnostics: %v", err)
	}

	reconciler.errors = []error{physicalErr, nil}
	if lease, err := service.AcquireManagedSubscription(context.Background(), "strategy", refs); lease != nil || !errors.Is(err, physicalErr) {
		t.Fatalf("failed managed acquire = %#v, %v", lease, err)
	}
	var nilService *Service
	if lease, err := nilService.AcquireManagedSubscription(context.Background(), "strategy", refs); lease != nil || err == nil {
		t.Fatalf("nil service managed acquire = %#v, %v", lease, err)
	}
}

func TestSubscriptionValidationAndSnapshotDecorationBoundaries(t *testing.T) {
	valid := []InstrumentRef{
		{Market: "HK", Symbol: "00700"},
		{Channel: "TICK", Market: "US", Symbol: "AAPL"},
		{Channel: "ORDER_BOOK", Market: "US", Symbol: "AAPL"},
	}
	for _, interval := range []string{"1m", "3m", "5m", "15m", "30m", "1h", "1d", "1w", "1mo"} {
		valid = append(valid, InstrumentRef{Channel: "KLINE", Market: "US", Symbol: "NVDA", Interval: interval})
	}
	if err := ValidateSubscriptionRefs(valid); err != nil {
		t.Fatalf("valid refs: %v", err)
	}
	invalid := [][]InstrumentRef{
		nil,
		{{Market: "", Symbol: "AAPL"}},
		{{Market: "US", Symbol: "AAPL", Channel: "SNAPSHOT", Interval: "1m"}},
		{{Market: "US", Symbol: "AAPL", Channel: "KLINE", Interval: "2m"}},
		{{Market: "US", Symbol: "AAPL", Channel: "ORDER_BOOK", Interval: "1m"}},
		{{Market: "US", Symbol: "AAPL", Channel: "NEWS"}},
	}
	for index, refs := range invalid {
		if err := ValidateSubscriptionRefs(refs); err == nil {
			t.Fatalf("invalid refs %d accepted: %#v", index, refs)
		}
	}

	snapshot := SubscriptionsSnapshot{
		"totalActiveSubscriptions": 1,
		"entries":                  []map[string]any{{"channel": "SNAPSHOT", "instrumentId": "US.AAPL", "interval": nil}},
	}
	decorated := decorateSubscriptionSnapshot(snapshot, nil)
	entry := decorated["entries"].([]map[string]any)[0]
	if decorated["desiredCount"] != 1 || entry["brokerState"] != "unmanaged" || decorated["brokerState"] == nil {
		t.Fatalf("default decoration = %#v", decorated)
	}
	orderBookSnapshot := SubscriptionsSnapshot{
		"totalActiveSubscriptions": 1,
		"entries": []map[string]any{{
			"channel": "ORDER_BOOK", "instrumentId": "US.AAPL", "interval": nil,
		}},
	}
	orderBookBrokerEntry := map[string]any{
		"key": "ORDER_BOOK:US.AAPL", "brokerState": "active", "subscribedAt": "now",
		"unsubscribeEligibleAt": "later", "lastError": nil,
	}
	decorated = decorateSubscriptionSnapshot(orderBookSnapshot, map[string]any{
		"desiredCount": 1, "ownActiveCount": 1, "pendingReleaseCount": 0,
		"totalUsedQuota": 3, "remainQuota": 297, "entries": []map[string]any{orderBookBrokerEntry},
	})
	entry = decorated["entries"].([]map[string]any)[0]
	if entry["brokerState"] != "active" || entry["unsubscribeEligibleAt"] != "later" {
		t.Fatalf("order-book decoration = %#v", decorated)
	}

	var nilLease *ManagedSubscription
	nilLease.Release()
	released := 0
	lease := newManagedSubscription(func() { released++ })
	lease.Release()
	lease.Release()
	newManagedSubscription(nil).Release()
	if released != 1 {
		t.Fatalf("managed release count = %d", released)
	}
}

func TestFailedAcquireRestoresOnlyTheAttemptedConsumerState(t *testing.T) {
	physicalErr := errors.New("physical subscribe failed")
	reconciler := &fakeSubscriptionReconciler{}
	service := NewService(stubProvider{})
	service.SetSubscriptionReconciler(reconciler)
	retained := InstrumentRef{Channel: "SNAPSHOT", Market: "US", Symbol: "AAPL"}
	newRef := InstrumentRef{Channel: "KLINE", Market: "US", Symbol: "MSFT", Interval: "1m"}

	if _, err := service.AcquireSubscription(context.Background(), "chart", []InstrumentRef{retained}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	reconciler.errors = []error{physicalErr, nil}
	if _, err := service.AcquireSubscription(context.Background(), "chart", []InstrumentRef{retained, newRef}); !errors.Is(err, physicalErr) {
		t.Fatalf("failed extension error = %v", err)
	}
	snapshot, err := service.GetSubscriptions(context.Background())
	if err != nil {
		t.Fatalf("GetSubscriptions: %v", err)
	}
	entry := singleSubscriptionEntry(t, snapshot)
	if entry["key"] != "SNAPSHOT:US:AAPL" || !reflect.DeepEqual(entry["consumers"], []string{"chart"}) {
		t.Fatalf("failed extension damaged prior ownership: %#v", snapshot)
	}

	reconciler.errors = []error{physicalErr, nil}
	if lease, err := service.AcquireManagedSubscription(context.Background(), "chart", []InstrumentRef{retained, newRef}); lease != nil || !errors.Is(err, physicalErr) {
		t.Fatalf("failed managed extension = %#v, %v", lease, err)
	}
	snapshot, _ = service.GetSubscriptions(context.Background())
	entry = singleSubscriptionEntry(t, snapshot)
	if entry["key"] != "SNAPSHOT:US:AAPL" || !reflect.DeepEqual(entry["consumers"], []string{"chart"}) {
		t.Fatalf("failed managed extension damaged prior ownership: %#v", snapshot)
	}
}

func TestManagedLeaseReleaseRestoresPreexistingConsumerOwnership(t *testing.T) {
	service := NewService(stubProvider{})
	retained := InstrumentRef{Channel: "SNAPSHOT", Market: "US", Symbol: "AAPL"}
	managedOnly := InstrumentRef{Channel: "KLINE", Market: "US", Symbol: "MSFT", Interval: "1m"}
	if _, err := service.AcquireSubscription(context.Background(), "shared", []InstrumentRef{retained}); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	lease, err := service.AcquireManagedSubscription(context.Background(), "shared", []InstrumentRef{retained, managedOnly})
	if err != nil {
		t.Fatalf("managed acquire: %v", err)
	}
	lease.Release()
	snapshot, _ := service.GetSubscriptions(context.Background())
	entry := singleSubscriptionEntry(t, snapshot)
	if entry["key"] != "SNAPSHOT:US:AAPL" || !reflect.DeepEqual(entry["consumers"], []string{"shared"}) {
		t.Fatalf("managed release did not restore prior ownership: %#v", snapshot)
	}
}

func TestSubscriptionRequiredErrorsAndManagedReadDemandBoundaries(t *testing.T) {
	var nilService *Service
	if err := nilService.requireBasicSubscriptionDemand("US", "AAPL", "TICK"); !errors.Is(err, ErrSubscriptionRequired) {
		t.Fatalf("nil service demand error = %v", err)
	}
	if err := nilService.requireOrderBookSubscriptionDemand("US", "AAPL"); !errors.Is(err, ErrSubscriptionRequired) {
		t.Fatalf("nil service order-book demand error = %v", err)
	}

	service := NewService(stubProvider{})
	service.SetSubscriptionReconciler(&fakeSubscriptionReconciler{})
	if _, err := service.GetSnapshot(context.Background(), "US", "AAPL", false); !errors.Is(err, ErrSubscriptionRequired) {
		t.Fatalf("snapshot without demand error = %v", err)
	}
	if _, err := service.GetCandles(context.Background(), "US", "AAPL", "tick", 1, "", ""); !errors.Is(err, ErrSubscriptionRequired) {
		t.Fatalf("tick candles without demand error = %v", err)
	}
	if _, err := service.GetDepth(context.Background(), "US", "AAPL", 10); !errors.Is(err, ErrSubscriptionRequired) {
		t.Fatalf("depth without order-book demand error = %v", err)
	}

	if _, err := service.AcquireSubscription(context.Background(), "other", []InstrumentRef{{Channel: "SNAPSHOT", Market: "HK", Symbol: "00700"}}); err != nil {
		t.Fatalf("seed mismatched demand: %v", err)
	}
	if err := service.requireBasicSubscriptionDemand("US", "AAPL", "SNAPSHOT"); !errors.Is(err, ErrSubscriptionRequired) {
		t.Fatalf("mismatched demand error = %v", err)
	}
	if _, err := service.AcquireSubscription(context.Background(), "chart", []InstrumentRef{{Channel: "KLINE", Market: "US", Symbol: "AAPL", Interval: "1m"}}); err != nil {
		t.Fatalf("seed matching demand: %v", err)
	}
	if err := service.requireBasicSubscriptionDemand(" us ", "us.aapl", "TICK"); err != nil {
		t.Fatalf("matching K-line demand should authorize Basic read: %v", err)
	}
	if _, err := service.GetDepth(context.Background(), "US", "AAPL", 10); !errors.Is(err, ErrSubscriptionRequired) {
		t.Fatalf("K-line demand must not authorize order-book reads: %v", err)
	}
	if _, err := service.AcquireSubscription(context.Background(), "depth", []InstrumentRef{{Channel: "ORDER_BOOK", Market: "US", Symbol: "AAPL"}}); err != nil {
		t.Fatalf("seed order-book demand: %v", err)
	}
	if _, err := service.GetDepth(context.Background(), " us ", "us.aapl", 10); err != nil {
		t.Fatalf("matching order-book demand should authorize depth read: %v", err)
	}

	required := NewSubscriptionRequiredError(" kline ", " hk ", " 00700 ", " 1M ")
	if required.Channel != "KLINE" || required.Market != "HK" || required.Symbol != "00700" || required.Interval != "1m" {
		t.Fatalf("normalized subscription error = %#v", required)
	}
	if !errors.Is(required, ErrSubscriptionRequired) || !strings.Contains(required.Error(), "HK.00700:1m") {
		t.Fatalf("typed subscription error = %v", required)
	}
	if got := (&SubscriptionRequiredError{}).Error(); !strings.Contains(got, "acquire a SNAPSHOT lease") {
		t.Fatalf("default subscription error = %q", got)
	}
	if got := (&SubscriptionRequiredError{Channel: "TICK"}).Error(); !strings.Contains(got, "before reading live data") {
		t.Fatalf("instrument-free subscription error = %q", got)
	}
	if got := (&SubscriptionRequiredError{Channel: "TICK", Market: "US", Symbol: "AAPL"}).Error(); !strings.Contains(got, "US.AAPL before") {
		t.Fatalf("interval-free subscription error = %q", got)
	}
	var nilRequired *SubscriptionRequiredError
	if got := nilRequired.Error(); got != ErrSubscriptionRequired.Error() {
		t.Fatalf("nil subscription error = %q", got)
	}

	registry := newSubscriptionRegistry()
	registry.restore(subscriptionRollback{})
	registry.restore(subscriptionRollback{
		consumerID: "missing",
		entries:    []subscriptionRollbackEntry{{key: "SNAPSHOT:US:MISSING"}},
	})
}

func TestServiceMergesAdditionalDemandIntoExactReconciliation(t *testing.T) {
	reconciler := &fakeSubscriptionReconciler{}
	service := NewService(stubProvider{})
	service.SetSubscriptionReconciler(reconciler)
	service.StartCollector(nil, nil, nil,
		DemandSourceFunc(func() []string { return []string{"us.aapl", "bad", ""} }),
		nil,
	)
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()
	if _, err := service.AcquireSubscription(context.Background(), "chart", []InstrumentRef{{Channel: "KLINE", Market: "HK", Symbol: "00700", Interval: "5m"}}); err != nil {
		t.Fatalf("AcquireSubscription: %v", err)
	}
	refs := service.activeSubscriptionDemand()
	if len(refs) != 2 {
		t.Fatalf("merged exact demand = %#v", refs)
	}
	if refs[0].Channel != "KLINE" || refs[1].Channel != "SNAPSHOT" || refs[1].Market != "US" || refs[1].Symbol != "AAPL" {
		t.Fatalf("merged exact refs = %#v", refs)
	}
}
