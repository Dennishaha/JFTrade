package futu

import (
	"fmt"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestKLineSessionRegistryHandlesInvalidStaleAndBoundedRecords(t *testing.T) {
	start := time.Date(2026, time.June, 22, 13, 30, 0, 0, time.UTC)
	kline := coverageMarginKLine("US.AAPL", start, 5*time.Minute)

	var nilExchange *Exchange
	nilExchange.RegisterKLineSession(kline, market.SessionRegular)
	nilExchange.RecordMarketSessionSample(kline.Symbol, market.SessionRegular, start)
	if got, ok := nilExchange.ResolveKLineSession(kline); ok || got != market.SessionUnknown {
		t.Fatalf("nil ResolveKLineSession = %s/%v, want unknown/false", got, ok)
	}

	exchange := NewExchange("")
	exchange.RegisterKLineSession(kline, market.SessionUnknown)
	exchange.RegisterKLineSession(types.KLine{}, market.SessionRegular)
	exchange.RecordMarketSessionSample("", market.SessionRegular, start)
	exchange.RecordMarketSessionSample(kline.Symbol, market.SessionUnknown, start)

	key := klineSessionCacheKey(kline)
	exchange.klineSessions = map[string]klineSessionRecord{
		key: {session: market.SessionPre, recordedAt: time.Now().UTC().Add(-trackedKLineSessionTTL - time.Minute)},
	}
	if got, ok := exchange.ResolveKLineSession(kline); ok || got != market.SessionUnknown {
		t.Fatalf("stale ResolveKLineSession = %s/%v, want unknown/false", got, ok)
	}

	exchange.RecordMarketSessionSample(" us.aapl ", market.SessionRegular, time.Time{})
	if got, ok := exchange.ResolveKLineSession(coverageMarginKLine("US.AAPL", time.Now().UTC(), time.Minute)); !ok || got != market.SessionRegular {
		t.Fatalf("sampled ResolveKLineSession = %s/%v, want regular/true", got, ok)
	}

	for _, invalid := range []types.KLine{
		{},
		{Symbol: "US.AAPL", StartTime: types.Time(start)},
		{Symbol: "US.AAPL", EndTime: types.Time(start)},
	} {
		if got := klineSessionCacheKey(invalid); got != "" {
			t.Fatalf("klineSessionCacheKey(%#v) = %q, want empty", invalid, got)
		}
	}
	if got := klineSessionCacheKey(coverageMarginKLine(" us.aapl ", start, time.Minute)); got == "" {
		t.Fatal("valid kline session cache key is empty")
	}
}

func TestKLineSessionRegistryPruningAndSampleResolutionBoundaries(t *testing.T) {
	now := time.Date(2026, time.June, 22, 14, 0, 0, 0, time.UTC)
	pruneKLineSessionCacheLocked(map[string]klineSessionRecord{}, now)

	cache := map[string]klineSessionRecord{
		"expired": {session: market.SessionPre, recordedAt: now.Add(-trackedKLineSessionTTL - time.Second)},
		"fresh":   {session: market.SessionRegular, recordedAt: now},
	}
	pruneKLineSessionCacheLocked(cache, now)
	if _, exists := cache["expired"]; exists || len(cache) != 1 {
		t.Fatalf("expired records were not pruned: %#v", cache)
	}

	for index := 0; index <= maxTrackedKLineSessions; index++ {
		cache[fmt.Sprintf("key-%d", index)] = klineSessionRecord{session: market.SessionRegular, recordedAt: now}
	}
	pruneKLineSessionCacheLocked(cache, now)
	if len(cache) != maxTrackedKLineSessions {
		t.Fatalf("bounded record count = %d, want %d", len(cache), maxTrackedKLineSessions)
	}

	if got := pruneMarketSessionSamples(nil, now); got != nil {
		t.Fatalf("empty samples = %#v, want nil", got)
	}
	samples := make([]marketSessionSample, 0, maxMarketSessionSamplesEach+3)
	samples = append(samples,
		marketSessionSample{at: now.Add(-marketSessionSampleTTL - time.Minute), session: market.SessionPre},
		marketSessionSample{at: now, session: market.SessionUnknown},
	)
	for index := 0; index < maxMarketSessionSamplesEach+1; index++ {
		samples = append(samples, marketSessionSample{at: now.Add(time.Duration(index) * time.Second), session: market.SessionRegular})
	}
	pruned := pruneMarketSessionSamples(samples, now)
	if len(pruned) != maxMarketSessionSamplesEach {
		t.Fatalf("pruned market samples = %d, want %d", len(pruned), maxMarketSessionSamplesEach)
	}

	if got, ok := resolveSessionFromSamples(nil, now, now, time.Minute); ok || got != market.SessionUnknown {
		t.Fatalf("empty session samples = %s/%v, want unknown/false", got, ok)
	}
	if got, ok := resolveSessionFromSamples(
		[]marketSessionSample{{at: time.Time{}, session: market.SessionRegular}},
		time.Time{},
		time.Time{},
		0,
	); !ok || got != market.SessionRegular {
		t.Fatalf("zero-window session samples = %s/%v, want regular/true", got, ok)
	}
	if got, ok := resolveSessionFromSamples(
		[]marketSessionSample{{at: now.Add(-time.Hour), session: market.SessionPre}},
		now,
		now,
		30*time.Second,
	); ok || got != market.SessionUnknown {
		t.Fatalf("out-of-window session samples = %s/%v, want unknown/false", got, ok)
	}
}

func TestResolveKLineSessionByClockUsesProvidedSymbolAndKLineFallback(t *testing.T) {
	preMarketUTC := time.Date(2026, time.June, 22, 12, 0, 0, 0, time.UTC)
	kline := coverageMarginKLine("US.AAPL", preMarketUTC, 5*time.Minute)
	if got := resolveKLineSessionByClock(" us.aapl ", kline); got != market.SessionPre {
		t.Fatalf("provided-symbol session = %s, want pre", got)
	}
	if got := resolveKLineSessionByClock("", kline); got != market.SessionPre {
		t.Fatalf("kline-symbol session = %s, want pre", got)
	}
	if got := resolveKLineSessionByClock("", types.KLine{}); got != market.SessionUnknown {
		t.Fatalf("empty session = %s, want unknown", got)
	}
}

func coverageMarginKLine(symbol string, start time.Time, interval time.Duration) types.KLine {
	return types.KLine{
		Symbol:    symbol,
		Interval:  types.Interval5m,
		StartTime: types.Time(start),
		EndTime:   types.Time(start.Add(interval)),
	}
}
