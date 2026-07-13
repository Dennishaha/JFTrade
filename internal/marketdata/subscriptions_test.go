package marketdata

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestSubscriptionRegistryContract(t *testing.T) {
	now := time.Date(2026, time.June, 14, 8, 9, 10, 123456789, time.FixedZone("CST", 8*60*60))
	registry := newSubscriptionRegistry()
	registry.now = func() time.Time { return now }

	result := registry.acquire(" chart-main ", []InstrumentRef{
		{Market: " hk ", Symbol: "00700"},
		{Market: "HK", Symbol: "hk.00700"},
	})
	entry := singleSubscriptionEntry(t, SubscriptionsSnapshot(result))
	assertSubscriptionEntry(t, entry, "SNAPSHOT:HK:00700", "SNAPSHOT", "HK", "00700", nil, "chart-main", now.UTC())
	assertSubscriptionQuota(t, SubscriptionsSnapshot(result), 1, "HK", 1)

	now = now.Add(15 * time.Second)
	heartbeat := registry.heartbeat("chart-main")
	entry = singleSubscriptionEntry(t, SubscriptionsSnapshot(heartbeat))
	if got := entry["createdAt"]; got != "2026-06-14T00:09:10.123456789Z" {
		t.Fatalf("createdAt = %#v", got)
	}
	if got := entry["updatedAt"]; got != "2026-06-14T00:09:25.123456789Z" {
		t.Fatalf("updatedAt after heartbeat = %#v", got)
	}

	registry.acquire("secondary", []InstrumentRef{{Market: "HK", Symbol: "00700"}})
	entry = singleSubscriptionEntry(t, registry.snapshot())
	if got := entry["refCount"]; got != 2 {
		t.Fatalf("refCount = %#v", got)
	}

	registry.clear("chart-main")
	entry = singleSubscriptionEntry(t, registry.snapshot())
	if got := entry["refCount"]; got != 1 {
		t.Fatalf("refCount after consumer clear = %#v", got)
	}

	registry.clear("secondary")
	if got := registry.snapshot()["totalActiveSubscriptions"]; got != 0 {
		t.Fatalf("totalActiveSubscriptions after final release = %#v", got)
	}
}

func TestSubscriptionRegistrySeparatesChannelAndInterval(t *testing.T) {
	registry := newSubscriptionRegistry()
	registry.acquire("chart-main", []InstrumentRef{
		{Channel: " kline ", Market: " hk ", Symbol: "00700", Interval: " 1M "},
		{Channel: "tick", Market: "HK", Symbol: "hk.00700"},
	})

	snapshot := registry.snapshot()
	if got := snapshot["totalActiveSubscriptions"]; got != 2 {
		t.Fatalf("totalActiveSubscriptions = %#v", got)
	}
	entries := subscriptionEntriesByKey(t, snapshot)
	assertSubscriptionEntry(t, entries["KLINE:HK:00700:1m"], "KLINE:HK:00700:1m", "KLINE", "HK", "00700", "1m", "chart-main", time.Time{})
	assertSubscriptionEntry(t, entries["TICK:HK:00700"], "TICK:HK:00700", "TICK", "HK", "00700", nil, "chart-main", time.Time{})

	active := registry.activeInstruments()
	if len(active) != 1 || active[0] != "HK.00700" {
		t.Fatalf("active instruments = %#v", active)
	}

	registry.release("chart-main", InstrumentRef{Channel: "KLINE", Market: "HK", Symbol: "00700", Interval: "1m"})
	snapshot = registry.snapshot()
	if got := snapshot["totalActiveSubscriptions"]; got != 1 {
		t.Fatalf("totalActiveSubscriptions after single release = %#v", got)
	}
	entry := singleSubscriptionEntry(t, snapshot)
	if entry["key"] != "TICK:HK:00700" {
		t.Fatalf("remaining entry = %#v", entry)
	}
}

func TestSubscriptionRegistryClearAllAndActiveInstruments(t *testing.T) {
	registry := newSubscriptionRegistry()
	registry.acquire("consumer", []InstrumentRef{
		{Market: "hk", Symbol: "00700"},
		{Market: "us", Symbol: "AAPL"},
	})

	active := registry.activeInstruments()
	if len(active) != 2 || !containsString(active, "HK.00700") || !containsString(active, "US.AAPL") {
		t.Fatalf("active instruments = %#v", active)
	}

	registry.clear("")
	if got := registry.snapshot()["totalActiveSubscriptions"]; got != 0 {
		t.Fatalf("totalActiveSubscriptions after clear all = %#v", got)
	}
}

func TestServiceOwnsSubscriptionsAndHealthMode(t *testing.T) {
	ctx := context.Background()
	provider := stubProvider{health: HealthStatus{Connected: false, StreamMode: "provider-mode", ActiveCount: 99}}
	service := NewService(provider)

	health, err := service.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if health.StreamMode != "idle" || health.ActiveCount != 0 {
		t.Fatalf("idle health = %#v", health)
	}

	if _, err := service.AcquireSubscription(ctx, "consumer", []InstrumentRef{{Market: "HK", Symbol: "00700"}}); err != nil {
		t.Fatalf("AcquireSubscription: %v", err)
	}
	health, err = service.Health(ctx)
	if err != nil {
		t.Fatalf("Health after acquire: %v", err)
	}
	if health.StreamMode != "snapshot-poll-fallback" || health.ActiveCount != 1 {
		t.Fatalf("fallback health = %#v", health)
	}

	service = NewService(stubProvider{health: HealthStatus{Connected: true}})
	health, err = service.Health(ctx)
	if err != nil {
		t.Fatalf("connected Health: %v", err)
	}
	if health.StreamMode != "push-stream" || health.ActiveCount != 0 {
		t.Fatalf("push health = %#v", health)
	}
}

func singleSubscriptionEntry(t *testing.T, snapshot SubscriptionsSnapshot) map[string]any {
	t.Helper()
	entries, ok := snapshot["entries"].([]map[string]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("entries = %#v", snapshot["entries"])
	}
	return entries[0]
}

func subscriptionEntriesByKey(t *testing.T, snapshot SubscriptionsSnapshot) map[string]map[string]any {
	t.Helper()
	entries, ok := snapshot["entries"].([]map[string]any)
	if !ok {
		t.Fatalf("entries = %#v", snapshot["entries"])
	}
	byKey := make(map[string]map[string]any, len(entries))
	for _, entry := range entries {
		key := jftradeCheckedTypeAssertion[string](entry["key"])
		byKey[key] = entry
	}
	return byKey
}

func assertSubscriptionEntry(t *testing.T, entry map[string]any, key, channel, market, symbol string, interval any, consumer string, at time.Time) {
	t.Helper()
	if entry == nil {
		t.Fatalf("entry %s not found", key)
	}
	if entry["key"] != key || entry["channel"] != channel || entry["market"] != market || entry["symbol"] != symbol {
		t.Fatalf("entry identity = %#v", entry)
	}
	if entry["instrumentId"] != market+"."+symbol || entry["interval"] != interval || entry["depthLevel"] != nil {
		t.Fatalf("entry transport fields = %#v", entry)
	}
	consumers, ok := entry["consumers"].([]string)
	if !ok || len(consumers) != 1 || consumers[0] != consumer || entry["refCount"] != 1 {
		t.Fatalf("entry consumers = %#v", entry)
	}
	if at.IsZero() {
		return
	}
	wantTime := at.Format(time.RFC3339Nano)
	if entry["createdAt"] != wantTime || entry["updatedAt"] != wantTime {
		t.Fatalf("entry timestamps = %#v", entry)
	}
}

func assertSubscriptionQuota(t *testing.T, snapshot SubscriptionsSnapshot, total int, market string, used int) {
	t.Helper()
	if snapshot["totalActiveSubscriptions"] != total {
		t.Fatalf("totalActiveSubscriptions = %#v", snapshot["totalActiveSubscriptions"])
	}
	quota := jftradeCheckedTypeAssertion[map[string]any](snapshot["quota"])
	if quota["totalUsed"] != total || quota["totalLimit"] != nil || quota["totalRemaining"] != nil {
		t.Fatalf("quota = %#v", quota)
	}
	buckets := jftradeCheckedTypeAssertion[[]map[string]any](quota["byMarket"])
	if len(buckets) != 1 || buckets[0]["market"] != market || buckets[0]["used"] != used ||
		buckets[0]["limit"] != nil || buckets[0]["remaining"] != nil {
		t.Fatalf("quota buckets = %#v", buckets)
	}
}

func containsString(values []string, want string) bool {
	return slices.Contains(values, want)
}

type stubProvider struct {
	health HealthStatus
}

func (p stubProvider) Descriptor(context.Context) (ProviderDescriptor, error) {
	return ProviderDescriptor{ProviderID: "stub-provider", DisplayName: "Stub Provider", Source: "stub"}, nil
}

func (p stubProvider) GetMarkets(context.Context) ([]MarketProfile, error) {
	return nil, nil
}

func (p stubProvider) GetSecurityDetails(context.Context, string, string) (SecurityDetails, error) {
	return nil, nil
}

func (p stubProvider) LookupInstrument(context.Context, string, string) ([]InstrumentCandidate, error) {
	return nil, nil
}

func (p stubProvider) QuerySnapshot(context.Context, string) (*Tick, error) {
	return nil, nil
}

func (p stubProvider) QueryTicker(context.Context, string) (*Tick, error) {
	return nil, nil
}

func (p stubProvider) GetHistoricalCandles(context.Context, string, string, string, int, string, string) (CandlesResponse, error) {
	return nil, nil
}

func (p stubProvider) GetDepth(context.Context, string, string, int) (DepthResponse, error) {
	return nil, nil
}

func (p stubProvider) NormalizeInstrument(context.Context, map[string]any) (map[string]any, error) {
	return nil, nil
}

func (p stubProvider) Health(context.Context) (HealthStatus, error) {
	return p.health, nil
}
