package marketdata

import (
	"context"
	"errors"
	"reflect"
	"strconv"
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
	for index := 0; index < consumers; index++ {
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
	for index := 0; index < consumers; index++ {
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
		{Channel: "ORDER_BOOK", Market: "US", Symbol: "MSFT"},
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
