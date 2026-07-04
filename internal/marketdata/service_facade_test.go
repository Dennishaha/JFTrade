package marketdata

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestServiceDelegatesProviderFacadeAndRefreshBoundaries(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	sample := tickAt("US.AAPL", "188.5", 10, now)
	provider := &dataProviderStub{
		markets:    []MarketProfile{{"market": "US", "timezone": "America/New_York"}},
		details:    SecurityDetails{"name": "Apple Inc.", "lotSize": 1},
		candles:    CandlesResponse{"provider": "historical"},
		depth:      DepthResponse{"levels": 5},
		normalized: map[string]any{"market": "US", "symbol": "AAPL", "instrumentId": "US.AAPL"},
		snapshot:   &sample,
	}
	service := NewService(provider)

	markets, err := service.GetMarkets(ctx)
	if err != nil || len(markets) != 1 || markets[0]["market"] != "US" {
		t.Fatalf("GetMarkets = %#v, err=%v", markets, err)
	}
	details, err := service.GetSecurityDetails(ctx, "us", "aapl")
	if err != nil || details["name"] != "Apple Inc." || provider.detailsMarket != "us" || provider.detailsSymbol != "aapl" {
		t.Fatalf("GetSecurityDetails = %#v, provider=%s/%s, err=%v", details, provider.detailsMarket, provider.detailsSymbol, err)
	}
	candles, err := service.GetCandles(ctx, "us", "aapl", " 1D ", 20, "2026-06-01T00:00:00Z", "2026-06-02T00:00:00Z")
	if err != nil || candles["provider"] != "historical" {
		t.Fatalf("GetCandles historical = %#v, err=%v", candles, err)
	}
	if provider.candlesMarket != "us" || provider.candlesSymbol != "aapl" || provider.candlesPeriod != "1d" ||
		provider.candlesLimit != 20 || provider.candlesFrom == "" || provider.candlesTo == "" {
		t.Fatalf("historical candle request = %#v", provider)
	}
	depth, err := service.GetDepth(ctx, "us", "aapl", 5)
	if err != nil || depth["levels"] != 5 || provider.depthMarket != "us" || provider.depthSymbol != "aapl" || provider.depthNum != 5 {
		t.Fatalf("GetDepth = %#v, provider=%s/%s/%d, err=%v", depth, provider.depthMarket, provider.depthSymbol, provider.depthNum, err)
	}
	normalized, err := service.NormalizeInstrument(ctx, map[string]any{"symbol": "aapl"})
	if err != nil || normalized["instrumentId"] != "US.AAPL" {
		t.Fatalf("NormalizeInstrument = %#v, err=%v", normalized, err)
	}

	snapshot, err := service.GetSnapshot(ctx, " us ", " aapl ", true)
	if err != nil {
		t.Fatalf("GetSnapshot refresh: %v", err)
	}
	request := jftradeCheckedTypeAssertion[map[string]any](snapshot["request"])
	meta := jftradeCheckedTypeAssertion[map[string]any](snapshot["meta"])
	if request["instrumentId"] != "US.AAPL" || meta["fromCache"] != false || provider.snapshotID != "US.AAPL" {
		t.Fatalf("refresh snapshot = %#v, provider id=%s", snapshot, provider.snapshotID)
	}

	cached, err := service.GetSnapshot(ctx, "US", "AAPL", false)
	if err != nil {
		t.Fatalf("GetSnapshot cached: %v", err)
	}
	if jftradeCheckedTypeAssertion[map[string]any](cached["meta"])["fromCache"] != true || provider.snapshotCalls != 1 {
		t.Fatalf("cached snapshot = %#v, snapshot calls=%d", cached, provider.snapshotCalls)
	}
}

func TestServiceSnapshotErrorsAreBusinessVisible(t *testing.T) {
	ctx := context.Background()
	snapshotErr := errors.New("provider denied snapshot")
	_, err := NewService(&dataProviderStub{snapshotErr: snapshotErr}).GetSnapshot(ctx, "HK", "00700", true)
	if !errors.Is(err, snapshotErr) {
		t.Fatalf("snapshot provider err = %v", err)
	}

	_, err = NewService(&dataProviderStub{}).GetSnapshot(ctx, "HK", "00700", true)
	if err == nil || !strings.Contains(err.Error(), "no snapshot available for HK.00700") {
		t.Fatalf("missing snapshot err = %v", err)
	}
}

func TestServiceTickCandlesProviderAndFallbackBoundaries(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	sample := tickAt("US.AAPL", "188.5", 500, now)
	provider := &dataProviderStub{ticker: &sample}
	service := NewService(provider)

	response, err := service.GetCandles(ctx, " us ", " aapl ", " tick ", 1501, "", "")
	if err != nil {
		t.Fatalf("GetCandles provider tick: %v", err)
	}
	request := jftradeCheckedTypeAssertion[map[string]any](response["request"])
	instrument := jftradeCheckedTypeAssertion[map[string]any](request["instrument"])
	meta := jftradeCheckedTypeAssertion[map[string]any](response["meta"])
	if provider.tickerID != "US.AAPL" || request["limit"] != 1000 || instrument["market"] != "US" ||
		response["totalReturned"] != 1 || meta["fromCache"] != false || meta["session"] != "all" {
		t.Fatalf("provider tick response = %#v, ticker id=%s", response, provider.tickerID)
	}

	cached, err := service.GetCandles(ctx, "US", "AAPL", "tick", 0, "", "")
	if err != nil {
		t.Fatalf("GetCandles cached tick: %v", err)
	}
	cachedRequest := jftradeCheckedTypeAssertion[map[string]any](cached["request"])
	if provider.tickerCalls != 1 || cachedRequest["limit"] != 200 ||
		jftradeCheckedTypeAssertion[map[string]any](cached["meta"])["fromCache"] != true {
		t.Fatalf("cached tick response = %#v, ticker calls=%d", cached, provider.tickerCalls)
	}

	tickerErr := errors.New("ticker unavailable")
	_, err = NewService(&dataProviderStub{tickerErr: tickerErr}).GetCandles(ctx, "HK", "00700", "tick", 0, "", "")
	if !errors.Is(err, tickerErr) {
		t.Fatalf("ticker error without retained cache = %v", err)
	}
}

func TestServiceSubscriptionFacadeCacheHelpersAndLifecycle(t *testing.T) {
	ctx := context.Background()
	service := NewService(&dataProviderStub{})
	now := time.Now().UTC()
	sample := tickAt("US.AAPL", "189.1", 50, now)

	service.Ingest(sample)
	if service.CachedCount("US.AAPL") != 1 {
		t.Fatalf("CachedCount = %d", service.CachedCount("US.AAPL"))
	}
	latest := service.Latest("US.AAPL", TickFreshness)
	if latest == nil || latest.Price.String() != "189.1" {
		t.Fatalf("Latest = %#v", latest)
	}
	if got := service.LatestMany([]string{"US.AAPL", "HK.00700"}, TickFreshness); len(got) != 1 || got[0].InstrumentID != "US.AAPL" {
		t.Fatalf("LatestMany = %#v", got)
	}
	if !service.AllFresh([]string{"US.AAPL"}, TickFreshness) || service.AllFresh([]string{"US.AAPL", "HK.00700"}, TickFreshness) {
		t.Fatalf("AllFresh returned inconsistent cache freshness")
	}
	event := service.LiveTick(latest, now.Add(time.Second).Format(time.RFC3339Nano))
	if event["type"] != "market-data.tick" || event["at"] != jftradeCheckedTypeAssertion[map[string]any](event["snapshot"])["observedAt"] {
		t.Fatalf("LiveTick = %#v", event)
	}

	result, err := service.AcquireSubscription(ctx, " trader ", []InstrumentRef{
		{Channel: "snapshot", Market: "us", Symbol: "aapl"},
		{Channel: "kline", Market: "hk", Symbol: "00700", Interval: "5m"},
	})
	if err != nil || result["totalActiveSubscriptions"] != 2 {
		t.Fatalf("AcquireSubscription = %#v, err=%v", result, err)
	}
	if heartbeat, err := service.Heartbeat(ctx, "trader"); err != nil || heartbeat["totalActiveSubscriptions"] != 2 {
		t.Fatalf("Heartbeat = %#v, err=%v", heartbeat, err)
	}
	active, err := service.GetActiveInstruments(ctx)
	if err != nil || !containsString(active, "US.AAPL") || !containsString(active, "HK.00700") {
		t.Fatalf("GetActiveInstruments = %#v, err=%v", active, err)
	}
	snapshot, err := service.GetSubscriptions(ctx)
	if err != nil || snapshot["totalActiveSubscriptions"] != 2 {
		t.Fatalf("GetSubscriptions = %#v, err=%v", snapshot, err)
	}
	if err := service.ReleaseSubscription(ctx, "trader", InstrumentRef{Channel: "snapshot", Market: "US", Symbol: "AAPL"}); err != nil {
		t.Fatalf("ReleaseSubscription target: %v", err)
	}
	if snapshot, _ := service.GetSubscriptions(ctx); snapshot["totalActiveSubscriptions"] != 1 {
		t.Fatalf("subscriptions after targeted release = %#v", snapshot)
	}
	if err := service.ClearSubscriptions(ctx, "trader"); err != nil {
		t.Fatalf("ClearSubscriptions consumer: %v", err)
	}
	if snapshot, _ := service.GetSubscriptions(ctx); snapshot["totalActiveSubscriptions"] != 0 {
		t.Fatalf("subscriptions after clear = %#v", snapshot)
	}

	var nilService *Service
	nilService.WakeCollector()
	nilService.ResetCollector()
	nilService.ResumeCollector()
	if nilService.RuntimeState() != (RuntimeState{}) {
		t.Fatalf("nil RuntimeState should be zero")
	}
	if err := nilService.Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}

	service.StartCollector(nil, nil, nil)
	service.WakeCollector()
	service.ResetCollector()
	if service.RuntimeState().Generation == 0 {
		t.Fatalf("collector reset should advance generation")
	}
	service.ResumeCollector()
	service.StartCollector(nil, nil, nil)
	if err := service.Close(); err != nil {
		t.Fatalf("collector Close: %v", err)
	}
	if !service.RuntimeState().Closed {
		t.Fatalf("collector should report closed state")
	}
}

func TestServiceHealthAndSerializationNilBoundaries(t *testing.T) {
	ctx := context.Background()
	healthErr := errors.New("health probe failed")
	if _, err := NewService(&dataProviderStub{healthErr: healthErr}).Health(ctx); !errors.Is(err, healthErr) {
		t.Fatalf("Health err = %v", err)
	}
	if SnapshotJSON(nil) != nil || LiveTickJSON(nil, "") != nil {
		t.Fatalf("nil serialization should return nil")
	}
	latest := LatestTicksJSON([]*Tick{nil})
	if latest["totalReturned"] != 0 {
		t.Fatalf("LatestTicksJSON nil sample = %#v", latest)
	}
}

func TestServiceProviderStatusCombinesDescriptorHealthAndDemand(t *testing.T) {
	ctx := context.Background()
	service := NewService(&dataProviderStub{
		descriptor: ProviderDescriptor{
			ProviderID:    "futu-opend",
			DisplayName:   "Futu OpenD",
			Source:        "bbgo:futu",
			DefaultMarket: "HK",
			Capabilities: ProviderCapabilities{
				Snapshots:         true,
				HistoricalCandles: true,
				OrderBookDepth:    true,
			},
			Constraints: ProviderConstraints{
				RequiresOpenD:           true,
				RequiresMarketDataRight: true,
				UsesSubscriptionQuota:   true,
			},
		},
		health: HealthStatus{Connected: false},
	})

	_, err := service.AcquireSubscription(ctx, "chart", []InstrumentRef{{Market: "US", Symbol: "AAPL"}})
	if err != nil {
		t.Fatalf("AcquireSubscription: %v", err)
	}
	status, err := service.ProviderStatus(ctx)
	if err != nil {
		t.Fatalf("ProviderStatus: %v", err)
	}
	if status.Descriptor.ProviderID != "futu-opend" || !status.Descriptor.Capabilities.OrderBookDepth {
		t.Fatalf("descriptor = %+v", status.Descriptor)
	}
	if status.Health.ActiveCount != 1 || status.Health.StreamMode != "snapshot-poll-fallback" {
		t.Fatalf("health = %+v", status.Health)
	}
	if status.Subscriptions["totalActiveSubscriptions"] != 1 {
		t.Fatalf("subscriptions = %#v", status.Subscriptions)
	}
	if status.CheckedAt == "" {
		t.Fatalf("CheckedAt should be populated")
	}

	expectedErr := errors.New("descriptor unavailable")
	_, err = NewService(&dataProviderStub{descriptorErr: expectedErr}).ProviderStatus(ctx)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("ProviderStatus descriptor err = %v", err)
	}
}
