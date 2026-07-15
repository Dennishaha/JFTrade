package marketdata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestCacheRemainingLifecycleBoundaries(t *testing.T) {
	now := time.Date(2026, time.July, 15, 8, 0, 0, 0, time.UTC)
	cache := NewCache()
	cache.now = func() time.Time { return now }
	if cache.Store(Tick{}) != nil {
		t.Fatal("invalid tick was stored")
	}
	if cache.Latest("US.AAPL", 0) != nil {
		t.Fatal("Latest with zero max age returned a tick")
	}
	if cache.LatestMany(nil, time.Second) != nil {
		t.Fatal("LatestMany with no instruments returned ticks")
	}

	cache.Seed(Tick{InstrumentID: "US.STALE", Price: decimal.NewFromInt(1), ObservedAt: now.Add(-time.Hour).Format(time.RFC3339Nano)})
	cache.Seed(Tick{InstrumentID: "US.INVALID", Price: decimal.NewFromInt(1), ObservedAt: "invalid"})
	if got := cache.LatestMany([]string{"US.MISSING", "US.STALE", "US.INVALID"}, time.Minute); len(got) != 0 {
		t.Fatalf("stale LatestMany = %#v", got)
	}

	inheritTickContext(nil, nil)
	if cloneTick(nil) != nil {
		t.Fatal("cloneTick(nil) != nil")
	}

	weekday := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	weekend := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	if sharesTradingDay("US.AAPL", time.Time{}, weekday) {
		t.Fatal("zero timestamp shares a trading day")
	}
	if sharesTradingDay("US.AAPL", weekday, weekend) {
		t.Fatal("US weekday and weekend share a trading day")
	}
	if sharesTradingDay("XX.UNKNOWN", weekend, weekend) {
		t.Fatal("unknown market resolved a trading day")
	}
	if !sharesTradingDay("HK.00700", weekend, weekend) {
		t.Fatal("HK same local weekend date fallback did not match")
	}
}

func TestSubscriptionRegistryRemainingLifecycleBoundaries(t *testing.T) {
	now := time.Date(2026, time.July, 15, 8, 0, 0, 0, time.UTC)
	registry := newSubscriptionRegistry()
	registry.now = func() time.Time { return now }
	registry.release("missing", InstrumentRef{Market: "US", Symbol: "AAPL"})

	registry.acquireManaged("strategy", []InstrumentRef{{Market: "US", Symbol: "AAPL"}})
	registry.heartbeat("missing")
	registry.heartbeat("strategy")
	registry.externalTTL = 0
	if got := registry.snapshot()["totalActiveSubscriptions"]; got != 1 {
		t.Fatalf("disabled expiry snapshot = %#v", got)
	}

	registry.externalTTL = time.Minute
	registry.acquire("", []InstrumentRef{{Market: "US", Symbol: "MSFT"}})
	registry.subscriptions["BROKEN"] = &subscription{
		key: "BROKEN", consumers: map[string]subscriptionConsumer{"web": {seenAt: now}},
	}
	registry.acquire("other", []InstrumentRef{{Channel: "TICK", Market: "US", Symbol: "AAPL"}})
	ids := registry.activeInstruments()
	if len(ids) != 2 {
		t.Fatalf("deduplicated active instruments = %#v", ids)
	}

	now = now.Add(2 * time.Minute)
	registry.snapshot()
	if entry := registry.subscriptions["SNAPSHOT:US:MSFT"]; entry != nil {
		t.Fatalf("expired web entry remained: %#v", entry)
	}
	if normalizeConsumerID(" ") != "web" {
		t.Fatal("blank consumer did not normalize to web")
	}
}

func TestNormalizeInstrumentIDRejectsIncompleteValues(t *testing.T) {
	if _, _, _, ok := NormalizeInstrumentID("US."); ok {
		t.Fatal("incomplete instrument ID was accepted")
	}
}

func TestServiceRemainingLifecycleBoundaries(t *testing.T) {
	ctx := context.Background()
	wantErr := errors.New("expected lifecycle error")

	var nilService *Service
	nilService.StartCollector(nil, nil, nil)
	nilService.SetSubscriptionReconciler(nil)
	if _, err := nilService.ResolveInstrument(ctx, "US", "AAPL", 1); err == nil {
		t.Fatal("nil ResolveInstrument error = nil")
	}
	if err := nilService.reconcileDesired(ctx, nil); err != nil {
		t.Fatalf("nil reconcileDesired = %v", err)
	}

	if _, err := NewService(&dataProviderStub{healthErr: wantErr}).ProviderStatus(ctx); !errors.Is(err, wantErr) {
		t.Fatalf("ProviderStatus health error = %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := NewService(&dataProviderStub{}).ProviderStatus(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("ProviderStatus canceled subscriptions = %v", err)
	}

	service := NewService(&dataProviderStub{candles: CandlesResponse{"ok": true}})
	_, _ = service.ResolveInstrument(ctx, "US", "AAPL", 1)
	if response, err := service.GetCandles(ctx, "US", "AAPL", "", 1, "", ""); err != nil || response["ok"] != true {
		t.Fatalf("default candle period = %#v, %v", response, err)
	}

	refs := []InstrumentRef{{Market: "US", Symbol: "AAPL"}}
	if _, err := service.AcquireSubscription(canceled, "chart", refs); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled AcquireSubscription = %v", err)
	}
	if _, err := service.AcquireSubscription(ctx, "chart", nil); err == nil {
		t.Fatal("invalid AcquireSubscription error = nil")
	}
	if _, err := service.AcquireManagedSubscription(canceled, "strategy", refs); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled AcquireManagedSubscription = %v", err)
	}
	if _, err := service.AcquireManagedSubscription(ctx, "strategy", nil); err == nil {
		t.Fatal("invalid AcquireManagedSubscription error = nil")
	}
	if err := service.ReleaseSubscription(canceled, "chart"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ReleaseSubscription = %v", err)
	}
	if err := service.ClearSubscriptions(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ClearSubscriptions = %v", err)
	}
	if _, err := service.GetSubscriptions(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled GetSubscriptions = %v", err)
	}
	if err := service.ReleaseSubscription(ctx, "chart"); err != nil {
		t.Fatalf("consumer ReleaseSubscription = %v", err)
	}

	releaseReconciler := &fakeSubscriptionReconciler{errors: []error{nil, wantErr}}
	service.SetSubscriptionReconciler(releaseReconciler)
	lease, err := service.AcquireManagedSubscription(ctx, "strategy", refs)
	if err != nil {
		t.Fatalf("AcquireManagedSubscription = %v", err)
	}
	lease.Release()

	collectorService := NewService(&dataProviderStub{health: HealthStatus{Connected: true}})
	collectorService.StartCollector(nil, nil, nil)
	collectorService.SetSubscriptionReconciler(&fakeSubscriptionReconciler{})
	if _, err := collectorService.Health(ctx); err != nil {
		t.Fatalf("collector Health = %v", err)
	}
	if err := collectorService.Close(); err != nil {
		t.Fatalf("collector Close = %v", err)
	}

	closeService := NewService(&dataProviderStub{})
	closeService.SetSubscriptionReconciler(&fakeSubscriptionReconciler{errors: []error{wantErr}})
	if err := closeService.Close(); err != nil {
		t.Fatalf("Close without collector = %v", err)
	}
	jftradeLogError(nil, wantErr)
}

type generationChangingPushSource struct {
	collector *Collector
	stream    PushStream
}

type exactOnlyInstrumentProvider struct{}

func (exactOnlyInstrumentProvider) LookupInstrument(context.Context, string, string) ([]InstrumentCandidate, error) {
	return nil, nil
}

type errOnlyContext struct {
	context.Context
	err error
}

func (c errOnlyContext) Done() <-chan struct{} { return nil }
func (c errOnlyContext) Err() error            { return c.err }

func (s *generationChangingPushSource) NewStream([]string, PushTickHandler) (PushStream, error) {
	s.collector.mu.Lock()
	s.collector.state.Generation++
	s.collector.mu.Unlock()
	return s.stream, nil
}

func TestCollectorRemainingLifecycleBoundaries(t *testing.T) {
	if DemandSourceFunc(nil).ActiveInstruments() != nil {
		t.Fatal("nil demand source returned instruments")
	}
	if SubscriptionDemandSourceFunc(nil).ActiveSubscriptions() != nil {
		t.Fatal("nil subscription demand source returned subscriptions")
	}
	var nilCollector *Collector
	nilCollector.SetDemandSources(nil)
	nilCollector.SetSubscriptionReconciler(nil, nil)
	nilCollector.Wake()
	if state := nilCollector.State(); !state.Closed {
		t.Fatalf("nil collector state = %#v", state)
	}
	nilCollector.Reset()
	nilCollector.Resume()
	if err := nilCollector.Close(); err != nil {
		t.Fatalf("nil collector Close = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	now := time.Date(2026, time.July, 15, 9, 0, 0, 0, time.UTC)
	collector := &Collector{
		cache: NewCache(), ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1),
		now: func() time.Time { return now }, pollInterval: time.Second,
		queryTimeout: time.Second, connectTimeout: time.Second,
	}
	collector.Wake()
	collector.Wake()
	collector.state.Closed = true
	collector.SetDemandSources(DemandSourceFunc(func() []string { return []string{"US.AAPL"} }))
	collector.SetSubscriptionReconciler(nil, nil)
	collector.Reset()
	collector.Resume()
	collector.reconcile()

	collector.state.Closed = false
	collector.subscriptionDemand = SubscriptionDemandSourceFunc(func() []InstrumentRef {
		return []InstrumentRef{{Market: "US", Symbol: "AAPL"}}
	})
	collector.subscriptionReconciler = &fakeSubscriptionReconciler{errors: []error{errors.New("expected reconciliation error")}}
	collector.reconcileSubscriptions()

	collector.push = &collectorPushSource{}
	collector.demandSources = []DemandSource{DemandSourceFunc(func() []string { return []string{"US.AAPL"} })}
	collector.key = "US.AAPL"
	collector.state.Generation = 2
	collector.reconcile()
	collector.key = "US.AAPL"
	collector.state.StreamRetryAt = now.Add(time.Minute)
	collector.startStream(collector.state.Generation, []string{"US.AAPL"})
	collector.state.StreamRetryAt = time.Time{}
	collector.startStream(collector.state.Generation+1, []string{"US.AAPL"})

	stream := &collectorStream{connectStarted: make(chan struct{}), connectDone: make(chan struct{})}
	changing := &generationChangingPushSource{collector: collector, stream: stream}
	collector.push = changing
	collector.key = "US.AAPL"
	collector.startStream(collector.state.Generation, []string{"US.AAPL"})

	collector.pushHandler = func(Tick) {}
	collector.commitPush(collector.state.Generation, Tick{})
	collector.commitPush(collector.state.Generation, testTick("US.AAPL", "190", TickKindTrade))
	collector.commitQuoteFailure(collector.state.Generation, nil)
	collector.commitQuoteFailure(collector.state.Generation+1, errors.New("stale quote error"))
	collector.commitStreamFailure(collector.state.Generation, nil)
	collector.commitStreamFailure(collector.state.Generation+1, errors.New("stale stream error"))
	_, activeStreamCancel := context.WithCancel(context.Background())
	collector.streamCancel = activeStreamCancel
	collector.stream = stream
	collector.commitStreamFailure(collector.state.Generation, errors.New("active stream error"))

	streamCtx, streamCancel := context.WithCancel(context.Background())
	collector.streamCancel = streamCancel
	collector.stream = stream
	if detached := collector.detachStreamLocked(); detached != stream {
		t.Fatalf("detached stream = %#v", detached)
	}
	if streamCtx.Err() == nil {
		t.Fatal("detaching stream did not cancel its context")
	}

	collector.state.Closed = false
	collector.paused = false
	collector.demandSources = []DemandSource{
		nil,
		DemandSourceFunc(func() []string { return []string{" us.aapl ", "", "US.AAPL"} }),
	}
	if got := collector.activeInstruments(); len(got) != 1 || got[0] != "US.AAPL" {
		t.Fatalf("activeInstruments = %#v", got)
	}
	if retryDelay(-1) != 5*time.Second {
		t.Fatal("negative retry delay did not clamp")
	}

	collector.quotes = &collectorQuoteSource{}
	collector.demandSources = []DemandSource{DemandSourceFunc(func() []string {
		collector.mu.Lock()
		collector.state.Closed = true
		collector.mu.Unlock()
		return []string{"US.MSFT"}
	})}
	collector.poll()
	cancel()

	timed := NewCollector(NewCache(), nil, nil, nil, CollectorOptions{
		PollInterval: time.Millisecond, DemandInterval: time.Millisecond,
	})
	time.Sleep(10 * time.Millisecond)
	if err := timed.Close(); err != nil {
		t.Fatalf("timed collector Close = %v", err)
	}
}

func TestInstrumentResolverRemainingLifecycleBoundaries(t *testing.T) {
	provider := &resolverProviderStub{search: func(context.Context, string, int) ([]InstrumentCandidate, error) {
		return []InstrumentCandidate{resolverCandidate("US", "AAPL", "Apple")}, nil
	}}
	resolver := NewMarketSubsetInstrumentResolver(provider)
	//nolint:staticcheck // Exercise the resolver's explicit nil-context fallback.
	if result, err := resolver.Resolve(nil, "", "Apple", 0); err != nil || result.TotalReturned != 1 {
		t.Fatalf("nil-context default-limit Resolve = %+v, %v", result, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.Resolve(canceled, "", "Apple", 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("initially canceled Resolve = %v", err)
	}
	if _, err := resolver.resolveQualified(canceled, InstrumentResolution{}, "US.AAPL"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled qualified Resolve = %v", err)
	}
	var nilResolver *MarketSubsetInstrumentResolver
	if _, err := nilResolver.Resolve(context.Background(), "", "Apple", 1); err == nil {
		t.Fatal("nil resolver error = nil")
	}
	if _, err := resolver.Resolve(context.Background(), "", "US.", 1); !IsInstrumentSearchInputError(err) {
		t.Fatalf("invalid qualified Resolve = %v", err)
	}
	noSearch := NewMarketSubsetInstrumentResolver(exactOnlyInstrumentProvider{})
	if _, err := noSearch.Resolve(context.Background(), "", "Apple", 1); err == nil {
		t.Fatal("missing search provider error = nil")
	}

	now := time.Date(2026, time.July, 15, 10, 0, 0, 0, time.UTC)
	resolver.now = func() time.Time { return now }
	resolver.searchCache["STALE"] = cachedInstrumentSearch{expiresAt: now.Add(-time.Second)}
	if _, err := resolver.Resolve(context.Background(), "", "Microsoft", 1); err != nil {
		t.Fatalf("Resolve with stale peer cache = %v", err)
	}
	if _, exists := resolver.searchCache["STALE"]; exists {
		t.Fatal("stale peer cache was not removed")
	}

	blocked := make(chan struct{})
	lookupStarted := make(chan struct{})
	lookupResolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{lookup: func(context.Context, string, string) ([]InstrumentCandidate, error) {
		close(lookupStarted)
		<-blocked
		return nil, nil
	}})
	lookupCtx, lookupCancel := context.WithCancel(context.Background())
	lookupDone := make(chan error, 1)
	go func() {
		_, _, err := lookupResolver.lookupLeaves(lookupCtx, []string{"US"}, "AAPL")
		lookupDone <- err
	}()
	<-lookupStarted
	lookupCancel()
	if err := <-lookupDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("lookup cancellation = %v", err)
	}
	close(blocked)

	errContext := errOnlyContext{Context: context.Background(), err: context.Canceled}
	if _, _, err := resolver.lookupLeaves(errContext, []string{"US"}, "AAPL"); !errors.Is(err, context.Canceled) {
		t.Fatalf("post-response lookup cancellation = %v", err)
	}
	failureResolver := NewMarketSubsetInstrumentResolver(&resolverProviderStub{lookup: func(context.Context, string, string) ([]InstrumentCandidate, error) {
		return nil, errors.New("leaf unavailable")
	}})
	entries, failures, err := failureResolver.lookupLeaves(context.Background(), []string{"US"}, "AAPL")
	if err != nil || len(entries) != 0 || len(failures) != 1 {
		t.Fatalf("failed leaf lookup = %#v %#v %v", entries, failures, err)
	}

	for _, test := range []struct {
		candidate InstrumentCandidate
		want      string
	}{
		{candidate: InstrumentCandidate{Symbol: "US:AAPL"}, want: "US.AAPL"},
		{candidate: InstrumentCandidate{Symbol: "AAPL"}, want: "US.AAPL"},
		{candidate: InstrumentCandidate{}, want: "US.AAPL"},
	} {
		candidate, ok := normalizeExactInstrumentCandidate(test.candidate, "US", "AAPL")
		if !ok || candidate.InstrumentID != test.want {
			t.Fatalf("normalizeExactInstrumentCandidate(%#v) = %#v, %t", test.candidate, candidate, ok)
		}
	}

	searchCandidate, ok := normalizeSearchInstrumentCandidate(InstrumentCandidate{
		Market: "UNKNOWN", Symbol: "US.AAPL",
	})
	if !ok || searchCandidate.InstrumentID != "US.AAPL" {
		t.Fatalf("prefixed unknown search candidate = %#v, %t", searchCandidate, ok)
	}
	searchCandidate, ok = normalizeSearchInstrumentCandidate(InstrumentCandidate{
		Market: "US", InstrumentID: "US.AAPL",
	})
	if !ok || searchCandidate.Code != "AAPL" {
		t.Fatalf("code fallback search candidate = %#v, %t", searchCandidate, ok)
	}
	if _, ok := normalizeSearchInstrumentCandidate(InstrumentCandidate{}); ok {
		t.Fatal("empty search candidate was accepted")
	}
	if _, _, ok := splitStableSearchInstrument("US. "); ok {
		t.Fatal("blank stable code was accepted")
	}

	marketAliases := map[string]string{
		"CNSH": "SH", "CNSZ": "SZ", "HKFUTURE": "HK_FUTURE",
		"CC": "CRYPTO", "": "UNKNOWN",
	}
	for input, want := range marketAliases {
		if got := stableSearchMarketCode(input); got != want {
			t.Fatalf("stableSearchMarketCode(%q) = %q", input, got)
		}
	}
	if _, err := normalizeInstrumentSearchMarket("not-a-market"); err == nil {
		t.Fatal("invalid market filter error = nil")
	}
	if got, err := normalizeInstrumentSearchMarket("SH"); err != nil || got != "SH" {
		t.Fatalf("SH market filter = %q, %v", got, err)
	}
}
