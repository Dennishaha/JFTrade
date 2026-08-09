package status

import (
	"sort"
	"testing"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestLiveStatsSortsActiveInstruments(t *testing.T) {
	stats := LiveStats(3, 20, true, []string{"HK.00700", "US.AAPL", "HK.00700"})
	if stats["connected"] != 3 || stats["limit"] != 20 || stats["atLimit"] != true {
		t.Fatalf("live stats = %#v", stats)
	}
	instruments, ok := stats["activeInstruments"].([]string)
	if !ok || !sort.StringsAreSorted(instruments) || len(instruments) != 3 {
		t.Fatalf("activeInstruments = %#v", stats["activeInstruments"])
	}
	stats = LiveStats(0, 0, false, nil)
	if _, ok := stats["activeInstruments"].([]string); !ok {
		t.Fatal("activeInstruments must remain a string slice")
	}
}

func TestMarketDataRuntimeSummaryStates(t *testing.T) {
	if got := MarketDataRuntimeSummary(nil); got["status"] != "unavailable" {
		t.Fatalf("nil service summary = %#v", got)
	}

	if got := marketDataRuntimeSummary(mdsrv.RuntimeState{}); got["status"] != "idle" {
		t.Fatalf("idle summary = %#v", got)
	}

	got := marketDataRuntimeSummary(mdsrv.RuntimeState{
		Closed:          true,
		Generation:      7,
		ActiveCount:     2,
		LastRefreshAt:   time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		QuoteFailures:   1,
		QuoteLastError:  " quote down ",
		StreamRetryAt:   time.Date(2026, 6, 1, 12, 1, 0, 0, time.UTC),
		StreamFailures:  3,
		StreamLastError: " stream down ",
	})
	if got["status"] != "closed" || got["connected"] != false || got["generation"] != uint64(7) || got["activeCount"] != 2 {
		t.Fatalf("closed summary = %#v", got)
	}
	if got["quoteLastError"].(*string) == nil || *got["quoteLastError"].(*string) != "quote down" ||
		got["streamLastError"].(*string) == nil || *got["streamLastError"].(*string) != "stream down" {
		t.Fatalf("trimmed error summaries = %#v", got)
	}
	if got["lastRefreshAt"] == nil || got["streamRetryAt"] == nil {
		t.Fatalf("time pointers missing = %#v", got)
	}

	if got := marketDataRuntimeSummary(mdsrv.RuntimeState{Connected: true, ActiveCount: 4}); got["status"] != "connected" {
		t.Fatalf("connected summary = %#v", got)
	}
	if got := marketDataRuntimeSummary(mdsrv.RuntimeState{QuoteLastError: "boom"}); got["status"] != "degraded" {
		t.Fatalf("degraded summary = %#v", got)
	}
	if got := marketDataRuntimeSummary(mdsrv.RuntimeState{ActiveCount: 1}); got["status"] != "connecting" {
		t.Fatalf("connecting summary = %#v", got)
	}
}

type fakeStrategyRuntimeSummary struct {
	summary map[string]any
}

func (f fakeStrategyRuntimeSummary) SummaryMap() map[string]any {
	return f.summary
}

func TestStrategyRuntimeSummaryDelegatesAndDefaults(t *testing.T) {
	got := StrategyRuntimeSummary(nil)
	if got["status"] != "idle" || got["activeStrategies"] != 0 ||
		got["supportsBacktestParity"] != true ||
		len(got["activeInstances"].([]map[string]any)) != 0 {
		t.Fatalf("nil runtime summary = %#v", got)
	}
	source := fakeStrategyRuntimeSummary{summary: map[string]any{"status": "running", "activeStrategies": 2}}
	if got := StrategyRuntimeSummary(source); got["activeStrategies"] != 2 {
		t.Fatalf("delegated summary = %#v", got)
	}
}

func TestTimeAndStringPointers(t *testing.T) {
	if got := OptionalTimeString(time.Time{}); got != nil {
		t.Fatalf("zero time = %v", got)
	}
	got := OptionalTimeString(time.Date(2026, 6, 1, 12, 0, 0, 123000000, time.FixedZone("CST", 8*3600)))
	if got == nil || *got != "2026-06-01T04:00:00.123Z" {
		t.Fatalf("formatted time = %v", got)
	}
	if got := StringPointerOrNil("  "); got != nil {
		t.Fatalf("blank string = %v", got)
	}
	if got := StringPointerOrNil("  futu  "); got == nil || *got != "futu" {
		t.Fatalf("trimmed string = %v", got)
	}
}
