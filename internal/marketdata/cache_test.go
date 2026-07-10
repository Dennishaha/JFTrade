package marketdata

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestCacheDeduplicatesPromotesAndInherits(t *testing.T) {
	now := time.Date(2026, time.June, 15, 14, 0, 0, 0, time.UTC)
	cache := NewCache()
	cache.now = func() time.Time { return now }

	first := tickAt("US.AAPL", "100", 1000, now.Add(-time.Second))
	first.PreviousClosePrice = new(decimal.RequireFromString("99"))
	first.PreMarket = &ExtendedQuote{Price: new(decimal.RequireFromString("100.5"))}
	first.Turnover = decimal.RequireFromString("12345")
	stored := cache.Store(first)
	if stored == nil {
		t.Fatal("expected first sample")
	}

	duplicate := first
	duplicate.ObservedAt = now.Format(time.RFC3339Nano)
	stored = cache.Store(duplicate)
	if stored == nil {
		t.Fatal("expected duplicate sample")
	}
	if stored.ObservedAt != first.ObservedAt {
		t.Fatalf("dedupe changed observedAt: %s", stored.ObservedAt)
	}

	promoted := first
	promoted.Source = "bbgo:futu:stream"
	promoted.ObservedAt = now.Format(time.RFC3339Nano)
	stored = cache.Store(promoted)
	if stored == nil {
		t.Fatal("expected promoted sample")
	}
	if cache.Count(first.InstrumentID) != 1 {
		t.Fatalf("dedupe count = %d", cache.Count(first.InstrumentID))
	}
	if stored.Source != promoted.Source || stored.ObservedAt != promoted.ObservedAt {
		t.Fatalf("promotion = %#v", stored)
	}

	trade := tickAt("US.AAPL", "101", 0, now.Add(time.Second))
	trade.Kind = TickKindTrade
	trade.Source = "bbgo:futu:stream"
	trade.Bid = trade.Price
	trade.Ask = trade.Price
	trade.Session = "unknown"
	stored = cache.Store(trade)
	if stored == nil {
		t.Fatal("expected trade sample")
	}
	if stored.Bid.String() != "100" || stored.Ask.String() != "100" || stored.Volume != 1000 {
		t.Fatalf("trade book/volume inheritance = %#v", stored)
	}
	if stored.PreviousClosePrice == nil || stored.PreviousClosePrice.String() != "99" || stored.PreMarket == nil {
		t.Fatalf("context inheritance = %#v", stored)
	}
	if stored.Turnover.String() != "12345" || stored.Session != "regular" {
		t.Fatalf("turnover/session inheritance = %#v", stored)
	}
}

func TestCacheFreshnessRetentionAndMaximum(t *testing.T) {
	now := time.Date(2026, time.June, 14, 10, 0, 0, 0, time.UTC)
	cache := NewCache()
	cache.now = func() time.Time { return now }
	cache.retention = 30 * time.Minute
	cache.max = 3

	cache.Seed(tickAt("HK.00700", "99", 1, now.Add(-31*time.Minute)))
	for index, price := range []string{"100", "101", "102", "103"} {
		cache.Store(tickAt("HK.00700", price, float64(index+2), now.Add(time.Duration(index)*time.Second)))
	}
	samples := cache.Snapshot("HK.00700")
	if len(samples) != 3 || samples[0].Price.String() != "101" || samples[2].Price.String() != "103" {
		t.Fatalf("retained samples = %#v", samples)
	}
	if cache.Latest("HK.00700", TickFreshness) == nil {
		t.Fatal("expected fresh latest sample")
	}
	now = now.Add(5 * time.Second)
	if cache.Latest("HK.00700", TickFreshness) != nil {
		t.Fatal("expected sample to become stale")
	}
	if cache.AllFresh([]string{"HK.00700"}, TickFreshness) {
		t.Fatal("AllFresh accepted a stale sample")
	}
}

func TestCacheDoesNotInheritExtendedSessionsAcrossTradingDays(t *testing.T) {
	holidayAt := time.Date(2026, time.June, 19, 16, 0, 0, 0, time.UTC)
	cache := NewCache()
	cache.now = func() time.Time { return holidayAt }

	first := tickAt("US.AAPL", "100", 1000, time.Date(2026, time.June, 18, 17, 0, 0, 0, time.UTC))
	first.Session = "pre"
	first.ExtendedHours = true
	first.PreMarket = &ExtendedQuote{
		Price:     new(decimal.RequireFromString("100.5")),
		QuoteTime: first.ObservedAt,
	}
	cache.Store(first)

	incoming := tickAt("US.AAPL", "101", 0, holidayAt)
	incoming.Kind = TickKindTrade
	incoming.Session = "unknown"
	stored := cache.Store(incoming)
	if stored == nil {
		t.Fatal("expected stored sample")
	}
	if stored.Session != "unknown" || stored.ExtendedHours {
		t.Fatalf("session inheritance should stop at holiday boundary: %#v", stored)
	}
	if stored.PreMarket != nil || stored.AfterMarket != nil || stored.Overnight != nil {
		t.Fatalf("extended quote inheritance should stop at holiday boundary: %#v", stored)
	}
}

func TestCachePromotesUSRegularCloseWhenAfterHoursTradeArrives(t *testing.T) {
	afterHoursAt := time.Date(2026, time.July, 9, 16, 0, 1, 0, time.FixedZone("EDT", -4*60*60))
	cache := NewCache()
	cache.now = func() time.Time { return afterHoursAt }

	regularClose := tickAt("US.BABA", "111.14", 14106666, afterHoursAt.Add(-2*time.Second))
	regularClose.PreviousClosePrice = new(decimal.RequireFromString("108.98"))
	regularClose.LastClosePrice = new(decimal.RequireFromString("108.98"))
	cache.Store(regularClose)

	afterHoursTrade := tickAt("US.BABA", "111.81", 100, afterHoursAt)
	afterHoursTrade.Kind = TickKindTrade
	afterHoursTrade.Source = "bbgo:futu:stream"
	afterHoursTrade.Session = "after"
	afterHoursTrade.ExtendedHours = true
	stored := cache.Store(afterHoursTrade)
	if stored == nil {
		t.Fatal("expected after-hours trade to be stored")
	}
	if stored.PreviousClosePrice == nil || stored.PreviousClosePrice.String() != "111.14" {
		t.Fatalf("previous close = %v, want Thursday regular close 111.14", stored.PreviousClosePrice)
	}
	if stored.LastClosePrice == nil || stored.LastClosePrice.String() != "108.98" {
		t.Fatalf("last close = %v, want prior trading-day close 108.98", stored.LastClosePrice)
	}
	if stored.Price.String() != "111.81" {
		t.Fatalf("after-hours price = %s, want 111.81", stored.Price)
	}
	serialized := SnapshotJSON(stored)
	if serialized["previousClosePrice"] != "111.14" || serialized["lastClosePrice"] != "108.98" {
		t.Fatalf("serialized close prices = %#v", serialized)
	}
	if serialized["price"] != "111.81" || serialized["session"] != "after" {
		t.Fatalf("serialized after-hours snapshot = %#v", serialized)
	}

	nextAfterHoursTrade := afterHoursTrade
	nextAfterHoursTrade.Price = decimal.RequireFromString("111.82")
	nextAfterHoursTrade.QuoteAt = afterHoursAt.Add(time.Second).Format(time.RFC3339Nano)
	nextAfterHoursTrade.ObservedAt = nextAfterHoursTrade.QuoteAt
	stored = cache.Store(nextAfterHoursTrade)
	if stored == nil || stored.PreviousClosePrice == nil || stored.PreviousClosePrice.String() != "111.14" {
		t.Fatalf("subsequent after-hours trade lost regular close context: %#v", stored)
	}
}

func TestTickCandlesVolumeWindowAndLimit(t *testing.T) {
	now := time.Date(2026, time.June, 14, 10, 0, 0, 0, time.UTC)
	samples := []Tick{
		tickAt("HK.00700", "100.10", 100, now.Add(-16*time.Minute)),
		tickAt("HK.00700", "101.20", 150, now.Add(-2*time.Minute)),
		tickAt("HK.00700", "102.30", 120, now.Add(-time.Minute)),
		tickAt("HK.00700", "103.40", 170, now),
	}
	unlimited := TickCandles(samples, time.Time{}, now, 0)
	if len(unlimited) != 3 {
		t.Fatalf("default 15 minute window returned %d candles", len(unlimited))
	}
	candles := TickCandles(samples, time.Time{}, now, 2)
	if len(candles) != 2 {
		t.Fatalf("len(candles) = %d", len(candles))
	}
	if candles[0]["open"] != "102.3" || candles[0]["volume"] != float64(0) {
		t.Fatalf("negative delta candle = %#v", candles[0])
	}
	if candles[1]["open"] != "103.4" || candles[1]["volume"] != float64(50) {
		t.Fatalf("latest candle = %#v", candles[1])
	}
	if candles[0]["session"] != "regular" {
		t.Fatalf("session = %#v", candles[0]["session"])
	}
}

func TestSerializationPreservesNullExtendedAndStringPrices(t *testing.T) {
	now := time.Date(2026, time.June, 14, 10, 0, 0, 0, time.UTC)
	sample := tickAt("US.AAPL", "100.25", 12, now)
	sample.AfterMarket = &ExtendedQuote{
		Price:     new(decimal.RequireFromString("101.75")),
		QuoteTime: now.Format(time.RFC3339Nano),
	}

	snapshot := SnapshotJSON(&sample)
	if snapshot["price"] != "100.25" || snapshot["openPrice"] != nil {
		t.Fatalf("snapshot prices = %#v", snapshot)
	}
	extended := jftradeCheckedTypeAssertion[map[string]any](snapshot["extended"])
	if extended["preMarket"] != nil || extended["overnight"] != nil {
		t.Fatalf("extended null behavior = %#v", extended)
	}
	if jftradeCheckedTypeAssertion[map[string]any](extended["afterMarket"])["price"] != "101.75" {
		t.Fatalf("after market = %#v", extended["afterMarket"])
	}
	if jftradeCheckedTypeAssertion[map[string]any](extended["afterMarket"])["quoteTime"] != now.Format(time.RFC3339Nano) {
		t.Fatalf("after market quoteTime = %#v", extended["afterMarket"])
	}

	event := LiveTickJSON(&sample, now.Add(time.Second).Format(time.RFC3339Nano))
	if event["at"] != jftradeCheckedTypeAssertion[map[string]any](event["snapshot"])["observedAt"] {
		t.Fatalf("observedAt override = %#v", event)
	}
}

func TestServiceUsesSingleCacheForSnapshotCandlesAndLatest(t *testing.T) {
	now := time.Now().UTC()
	provider := &dataProviderStub{
		snapshot: new(tickAt("HK.00700", "321.4", 100, now)),
		ticker:   new(tickAt("HK.00700", "322.5", 150, now)),
	}
	service := NewService(provider)

	snapshot, err := service.GetSnapshot(context.Background(), "hk", "00700", false)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if jftradeCheckedTypeAssertion[map[string]any](snapshot["meta"])["fromCache"] != false {
		t.Fatalf("snapshot meta = %#v", snapshot["meta"])
	}
	if provider.snapshotCalls != 1 {
		t.Fatalf("snapshot calls = %d", provider.snapshotCalls)
	}

	snapshot, err = service.GetSnapshot(context.Background(), "HK", "00700", false)
	if err != nil || jftradeCheckedTypeAssertion[map[string]any](snapshot["meta"])["fromCache"] != true {
		t.Fatalf("cached snapshot = %#v, err=%v", snapshot, err)
	}
	candles, err := service.GetCandles(context.Background(), "HK", "00700", "tick", 10, "", "")
	if err != nil {
		t.Fatalf("GetCandles: %v", err)
	}
	if provider.tickerCalls != 0 || candles["totalReturned"] != 1 {
		t.Fatalf("tick candles = %#v, ticker calls=%d", candles, provider.tickerCalls)
	}
	latest, err := service.GetLatestTicks(context.Background(), []string{"HK.00700"})
	if err != nil || latest["totalReturned"] != 1 {
		t.Fatalf("latest ticks = %#v, err=%v", latest, err)
	}
}

func TestServiceTickCandleFallsBackToRetainedCache(t *testing.T) {
	now := time.Now().UTC()
	provider := &dataProviderStub{tickerErr: errors.New("ticker unavailable")}
	service := NewService(provider)
	service.Seed(tickAt("HK.00700", "321.4", 100, now.Add(-time.Minute)))

	response, err := service.GetCandles(context.Background(), "HK", "00700", "tick", 2, "", "")
	if err != nil {
		t.Fatalf("GetCandles fallback: %v", err)
	}
	if jftradeCheckedTypeAssertion[map[string]any](response["meta"])["fromCache"] != true || response["totalReturned"] != 1 {
		t.Fatalf("fallback response = %#v", response)
	}
}

func tickAt(instrumentID, price string, volume float64, observedAt time.Time) Tick {
	normalized, market, symbol, _ := NormalizeInstrumentID(instrumentID)
	value := decimal.RequireFromString(price)
	return Tick{
		InstrumentID: normalized,
		Market:       market,
		Symbol:       symbol,
		Price:        value,
		Bid:          value,
		Ask:          value,
		Volume:       volume,
		QuoteAt:      observedAt.Format(time.RFC3339Nano),
		ObservedAt:   observedAt.Format(time.RFC3339Nano),
		Source:       "bbgo:futu",
		Session:      "regular",
		Kind:         TickKindQuote,
	}
}

type dataProviderStub struct {
	descriptor    ProviderDescriptor
	descriptorErr error
	markets       []MarketProfile
	marketsErr    error
	details       SecurityDetails
	detailsErr    error
	snapshot      *Tick
	snapshotErr   error
	ticker        *Tick
	tickerErr     error
	candles       CandlesResponse
	candlesErr    error
	depth         DepthResponse
	depthErr      error
	normalized    map[string]any
	normalizedErr error
	health        HealthStatus
	healthErr     error

	detailsMarket string
	detailsSymbol string
	snapshotID    string
	tickerID      string
	candlesMarket string
	candlesSymbol string
	candlesPeriod string
	candlesLimit  int
	candlesFrom   string
	candlesTo     string
	depthMarket   string
	depthSymbol   string
	depthNum      int

	snapshotCalls int
	tickerCalls   int
}

func (p *dataProviderStub) Descriptor(context.Context) (ProviderDescriptor, error) {
	if p.descriptor.ProviderID == "" {
		p.descriptor.ProviderID = "stub-provider"
		p.descriptor.DisplayName = "Stub Provider"
		p.descriptor.Source = "stub"
	}
	return p.descriptor, p.descriptorErr
}

func (p *dataProviderStub) GetMarkets(context.Context) ([]MarketProfile, error) {
	return p.markets, p.marketsErr
}

func (p *dataProviderStub) GetSecurityDetails(_ context.Context, market, symbol string) (SecurityDetails, error) {
	p.detailsMarket = market
	p.detailsSymbol = symbol
	return p.details, p.detailsErr
}

func (p *dataProviderStub) QuerySnapshot(_ context.Context, instrumentID string) (*Tick, error) {
	p.snapshotCalls++
	p.snapshotID = instrumentID
	return cloneTick(p.snapshot), p.snapshotErr
}

func (p *dataProviderStub) QueryTicker(_ context.Context, instrumentID string) (*Tick, error) {
	p.tickerCalls++
	p.tickerID = instrumentID
	return cloneTick(p.ticker), p.tickerErr
}

func (p *dataProviderStub) GetHistoricalCandles(_ context.Context, market, symbol, period string, limit int, fromTime, toTime string) (CandlesResponse, error) {
	p.candlesMarket = market
	p.candlesSymbol = symbol
	p.candlesPeriod = period
	p.candlesLimit = limit
	p.candlesFrom = fromTime
	p.candlesTo = toTime
	return p.candles, p.candlesErr
}

func (p *dataProviderStub) GetDepth(_ context.Context, market, symbol string, num int) (DepthResponse, error) {
	p.depthMarket = market
	p.depthSymbol = symbol
	p.depthNum = num
	return p.depth, p.depthErr
}

func (p *dataProviderStub) NormalizeInstrument(_ context.Context, input map[string]any) (map[string]any, error) {
	return p.normalized, p.normalizedErr
}

func (p *dataProviderStub) Health(context.Context) (HealthStatus, error) {
	return p.health, p.healthErr
}
