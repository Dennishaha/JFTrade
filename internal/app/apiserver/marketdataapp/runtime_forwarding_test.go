package marketdataapp

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestRuntimeForwardsEveryDataPlaneOperationWithoutRewritingProviderResults(t *testing.T) {
	provider := &forwardingProviderStub{}
	quotes := &forwardingQuoteSourceStub{}
	push := &forwardingPushSourceStub{stream: &forwardingPushStreamStub{}}
	subscriptions := &subscriptionReconcilerStub{
		state: map[string]any{"provider": "futu", "activeCount": 1},
	}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      provider,
		FutuQuotes:        quotes,
		FutuPush:          push,
		FutuSubscriptions: subscriptions,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := t.Context()

	descriptor, err := runtime.Descriptor(ctx)
	if err != nil || descriptor.ProviderID != "forwarding-provider" {
		t.Fatalf("Descriptor = %#v, err=%v", descriptor, err)
	}
	markets, err := runtime.GetMarkets(ctx)
	if err != nil || len(markets) != 1 || markets[0]["code"] != "US" {
		t.Fatalf("GetMarkets = %#v, err=%v", markets, err)
	}
	details, err := runtime.GetSecurityDetails(ctx, "US", "AAPL")
	if err != nil || details["instrumentId"] != "US.AAPL" {
		t.Fatalf("GetSecurityDetails = %#v, err=%v", details, err)
	}
	lookup, err := runtime.LookupInstrument(ctx, "US", "AAPL")
	if err != nil || len(lookup) != 1 || lookup[0].InstrumentID != "US.AAPL" {
		t.Fatalf("LookupInstrument = %#v, err=%v", lookup, err)
	}
	search, err := runtime.SearchInstruments(ctx, "apple", 7)
	if err != nil || len(search) != 1 || search[0].Name != "Apple" {
		t.Fatalf("SearchInstruments = %#v, err=%v", search, err)
	}
	snapshot, err := runtime.QuerySnapshot(ctx, "US.AAPL")
	if err != nil || snapshot == nil || snapshot.Source != "snapshot" {
		t.Fatalf("QuerySnapshot = %#v, err=%v", snapshot, err)
	}
	ticker, err := runtime.QueryTicker(ctx, "US.AAPL")
	if err != nil || ticker == nil || ticker.Source != "ticker" {
		t.Fatalf("QueryTicker = %#v, err=%v", ticker, err)
	}
	candles, err := runtime.GetHistoricalCandles(
		ctx, marketdata.HistoricalCandlesQuery{Market: "US", Symbol: "AAPL", Period: "1d", Limit: 20, FromTime: "2026-07-01T00:00:00Z", ToTime: "2026-07-29T00:00:00Z"},
	)
	if err != nil || candles["period"] != "1d" {
		t.Fatalf("GetHistoricalCandles = %#v, err=%v", candles, err)
	}
	depth, err := runtime.GetDepth(ctx, "US", "AAPL", 10)
	if err != nil || depth["levels"] != 10 {
		t.Fatalf("GetDepth = %#v, err=%v", depth, err)
	}
	normalized, err := runtime.NormalizeInstrument(ctx, map[string]any{"instrumentId": "us.aapl"})
	if err != nil || normalized["instrumentId"] != "US.AAPL" {
		t.Fatalf("NormalizeInstrument = %#v, err=%v", normalized, err)
	}
	health, err := runtime.Health(ctx)
	if err != nil || !health.Connected || health.StreamMode != "forwarded" {
		t.Fatalf("Health = %#v, err=%v", health, err)
	}

	ticks, err := runtime.QueryTickers(ctx, []string{"US.AAPL", "US.MSFT"})
	if err != nil || len(ticks) != 2 || !slices.Equal(quotes.instrumentIDs, []string{"US.AAPL", "US.MSFT"}) {
		t.Fatalf("QueryTickers = %#v, ids=%#v, err=%v", ticks, quotes.instrumentIDs, err)
	}
	handler := func(marketdata.Tick) {}
	stream, err := runtime.NewStream([]string{"US.AAPL"}, handler)
	if err != nil || stream != push.stream || !slices.Equal(push.instrumentIDs, []string{"US.AAPL"}) ||
		push.handler == nil {
		t.Fatalf("NewStream = %#v, push=%#v, err=%v", stream, push, err)
	}
	desired := []marketdata.InstrumentRef{{Channel: "BASIC", Market: "US", Symbol: "AAPL"}}
	if err := runtime.ReconcileSubscriptions(ctx, desired); err != nil {
		t.Fatalf("ReconcileSubscriptions: %v", err)
	}
	if !slices.Equal(subscriptions.desired, desired) || runtime.SubscriptionState()["provider"] != "futu" {
		t.Fatalf("subscription forwarding = %#v/%#v", subscriptions.desired, runtime.SubscriptionState())
	}

	provider.assertCalls(t)
}

func TestRuntimePreservesErrorsFromEveryActiveCapability(t *testing.T) {
	providerErr := errors.New("provider failed")
	quoteErr := errors.New("quote polling failed")
	pushErr := errors.New("stream failed")
	subscriptionErr := errors.New("subscription failed")
	provider := &forwardingProviderStub{err: providerErr}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider: provider,
		FutuQuotes:   &forwardingQuoteSourceStub{err: quoteErr},
		FutuPush:     &forwardingPushSourceStub{err: pushErr},
		FutuSubscriptions: &subscriptionReconcilerStub{
			err: subscriptionErr,
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	ctx := t.Context()

	providerCalls := []struct {
		name string
		call func() error
	}{
		{"Descriptor", func() error { _, callErr := runtime.Descriptor(ctx); return callErr }},
		{"GetMarkets", func() error { _, callErr := runtime.GetMarkets(ctx); return callErr }},
		{"GetSecurityDetails", func() error {
			_, callErr := runtime.GetSecurityDetails(ctx, "US", "AAPL")
			return callErr
		}},
		{"LookupInstrument", func() error {
			_, callErr := runtime.LookupInstrument(ctx, "US", "AAPL")
			return callErr
		}},
		{"SearchInstruments", func() error {
			_, callErr := runtime.SearchInstruments(ctx, "apple", 5)
			return callErr
		}},
		{"QuerySnapshot", func() error { _, callErr := runtime.QuerySnapshot(ctx, "US.AAPL"); return callErr }},
		{"QueryTicker", func() error { _, callErr := runtime.QueryTicker(ctx, "US.AAPL"); return callErr }},
		{"GetHistoricalCandles", func() error {
			_, callErr := runtime.GetHistoricalCandles(ctx, marketdata.HistoricalCandlesQuery{Market: "US", Symbol: "AAPL", Period: "1d", Limit: 5})
			return callErr
		}},
		{"GetDepth", func() error { _, callErr := runtime.GetDepth(ctx, "US", "AAPL", 5); return callErr }},
		{"NormalizeInstrument", func() error {
			_, callErr := runtime.NormalizeInstrument(ctx, map[string]any{"instrumentId": "US.AAPL"})
			return callErr
		}},
		{"Health", func() error { _, callErr := runtime.Health(ctx); return callErr }},
	}
	for _, test := range providerCalls {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, providerErr) {
				t.Fatalf("%s error = %v", test.name, err)
			}
		})
	}
	if _, err := runtime.QueryTickers(ctx, []string{"US.AAPL"}); !errors.Is(err, quoteErr) {
		t.Fatalf("QueryTickers error = %v", err)
	}
	if _, err := runtime.NewStream([]string{"US.AAPL"}, nil); !errors.Is(err, pushErr) {
		t.Fatalf("NewStream error = %v", err)
	}
	if err := runtime.ReconcileSubscriptions(ctx, nil); !errors.Is(err, subscriptionErr) {
		t.Fatalf("ReconcileSubscriptions error = %v", err)
	}
}

func TestRuntimeSameProviderActivationDoesNotReleasePhysicalSubscriptions(t *testing.T) {
	subscriptions := &subscriptionReconcilerStub{
		desired: []marketdata.InstrumentRef{{Channel: "BASIC", Market: "US", Symbol: "AAPL"}},
	}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      &forwardingProviderStub{},
		FutuSubscriptions: subscriptions,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	installHealthySidecar(runtime, sidecar)

	if err := runtime.Activate(t.Context(), Activation{ProviderID: " FUTU "}); err != nil {
		t.Fatalf("Activate(futu): %v", err)
	}
	if len(subscriptions.desired) != 1 || subscriptions.desired[0].Symbol != "AAPL" {
		t.Fatalf("same-provider activation released subscriptions: %#v", subscriptions.desired)
	}
	if sidecar.ensureCalls != 0 || sidecar.stopCalls != 0 {
		t.Fatalf("same-provider sidecar calls = ensure %d stop %d", sidecar.ensureCalls, sidecar.stopCalls)
	}
}

func TestRuntimeSidecarFailureAndCloseKeepSelectionStable(t *testing.T) {
	sidecarErr := errors.New("sidecar failed")
	closeErr := errors.New("close failed")
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &forwardingProviderStub{}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &failingSidecarLifecycleStub{ensureErr: sidecarErr, closeErr: closeErr}
	runtime.sidecar = sidecar

	err = runtime.Activate(t.Context(), Activation{
		ProviderID: ProviderYFinance,
	})
	if !errors.Is(err, sidecarErr) || runtime.ActiveProviderID() != ProviderFutu {
		t.Fatalf("failed activation = provider %q, err=%v", runtime.ActiveProviderID(), err)
	}
	if err := runtime.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close error = %v", err)
	}
	runtime.sidecar = nil
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close(nil sidecar): %v", err)
	}
}

type forwardingProviderStub struct {
	err   error
	calls map[string]int
}

func (p *forwardingProviderStub) record(name string) {
	if p.calls == nil {
		p.calls = make(map[string]int)
	}
	p.calls[name]++
}

func (p *forwardingProviderStub) Descriptor(context.Context) (marketdata.ProviderDescriptor, error) {
	p.record("Descriptor")
	return marketdata.ProviderDescriptor{ProviderID: "forwarding-provider"}, p.err
}

func (p *forwardingProviderStub) GetMarkets(context.Context) ([]marketdata.MarketProfile, error) {
	p.record("GetMarkets")
	return []marketdata.MarketProfile{{"code": "US"}}, p.err
}

func (p *forwardingProviderStub) GetSecurityDetails(
	context.Context,
	string,
	string,
) (marketdata.SecurityDetails, error) {
	p.record("GetSecurityDetails")
	return marketdata.SecurityDetails{"instrumentId": "US.AAPL"}, p.err
}

func (p *forwardingProviderStub) LookupInstrument(
	context.Context,
	string,
	string,
) ([]marketdata.InstrumentCandidate, error) {
	p.record("LookupInstrument")
	return []marketdata.InstrumentCandidate{{InstrumentID: "US.AAPL"}}, p.err
}

func (p *forwardingProviderStub) SearchInstruments(
	context.Context,
	string,
	int,
) ([]marketdata.InstrumentCandidate, error) {
	p.record("SearchInstruments")
	return []marketdata.InstrumentCandidate{{InstrumentID: "US.AAPL", Name: "Apple"}}, p.err
}

func (p *forwardingProviderStub) QuerySnapshot(context.Context, string) (*marketdata.Tick, error) {
	p.record("QuerySnapshot")
	return &marketdata.Tick{InstrumentID: "US.AAPL", Source: "snapshot"}, p.err
}

func (p *forwardingProviderStub) QueryTicker(context.Context, string) (*marketdata.Tick, error) {
	p.record("QueryTicker")
	return &marketdata.Tick{InstrumentID: "US.AAPL", Source: "ticker"}, p.err
}

func (p *forwardingProviderStub) GetHistoricalCandles(
	context.Context,
	marketdata.HistoricalCandlesQuery,
) (marketdata.CandlesResponse, error) {
	p.record("GetHistoricalCandles")
	return marketdata.CandlesResponse{"period": "1d"}, p.err
}

func (p *forwardingProviderStub) GetDepth(
	context.Context,
	string,
	string,
	int,
) (marketdata.DepthResponse, error) {
	p.record("GetDepth")
	return marketdata.DepthResponse{"levels": 10}, p.err
}

func (p *forwardingProviderStub) NormalizeInstrument(
	context.Context,
	map[string]any,
) (map[string]any, error) {
	p.record("NormalizeInstrument")
	return map[string]any{"instrumentId": "US.AAPL"}, p.err
}

func (p *forwardingProviderStub) Health(context.Context) (marketdata.HealthStatus, error) {
	p.record("Health")
	return marketdata.HealthStatus{Connected: true, StreamMode: "forwarded"}, p.err
}

func (p *forwardingProviderStub) assertCalls(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"Descriptor", "GetMarkets", "GetSecurityDetails", "LookupInstrument", "SearchInstruments",
		"QuerySnapshot", "QueryTicker", "GetHistoricalCandles", "GetDepth", "NormalizeInstrument", "Health",
	} {
		if p.calls[name] != 1 {
			t.Fatalf("%s call count = %d", name, p.calls[name])
		}
	}
}

type forwardingQuoteSourceStub struct {
	instrumentIDs []string
	err           error
}

func (s *forwardingQuoteSourceStub) QueryTickers(
	_ context.Context,
	instrumentIDs []string,
) (map[string]marketdata.Tick, error) {
	s.instrumentIDs = append([]string(nil), instrumentIDs...)
	return map[string]marketdata.Tick{
		"US.AAPL": {InstrumentID: "US.AAPL"},
		"US.MSFT": {InstrumentID: "US.MSFT"},
	}, s.err
}

type forwardingPushSourceStub struct {
	instrumentIDs []string
	handler       marketdata.PushTickHandler
	stream        marketdata.PushStream
	err           error
}

func (s *forwardingPushSourceStub) NewStream(
	instrumentIDs []string,
	handler marketdata.PushTickHandler,
) (marketdata.PushStream, error) {
	s.instrumentIDs = append([]string(nil), instrumentIDs...)
	s.handler = handler
	return s.stream, s.err
}

type forwardingPushStreamStub struct{}

func (*forwardingPushStreamStub) Connect(context.Context) error { return nil }
func (*forwardingPushStreamStub) Close() error                  { return nil }

type failingSidecarLifecycleStub struct {
	ensureErr error
	closeErr  error
}

func (s *failingSidecarLifecycleStub) EnsureStarted() (string, error) {
	return "", s.ensureErr
}

func (*failingSidecarLifecycleStub) Stop() error {
	return nil
}

func (s *failingSidecarLifecycleStub) Close() error {
	return s.closeErr
}
