package status

import (
	"sort"
	"testing"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
)

func TestLiveStatsSortsActiveInstruments(t *testing.T) {
	stats := LiveStats(3, 20, true, []string{"HK.00700", "US.AAPL", "HK.00700"})
	if stats.Connected != 3 || stats.Limit != 20 || !stats.AtLimit {
		t.Fatalf("live stats = %#v", stats)
	}
	if instruments := stats.ActiveInstruments; !sort.StringsAreSorted(instruments) || len(instruments) != 3 {
		t.Fatalf("activeInstruments = %#v", instruments)
	}
	stats = LiveStats(0, 0, false, nil)
	if stats.ActiveInstruments == nil {
		t.Fatal("activeInstruments must remain a string slice")
	}
}

func TestMarketDataRuntimeSummaryStates(t *testing.T) {
	if got := MarketDataRuntimeSummary(nil); got.Status != "unavailable" {
		t.Fatalf("nil service summary = %#v", got)
	}

	if got := marketDataRuntimeSummary(mdsrv.RuntimeState{}); got.Status != "idle" {
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
	if got.Status != "closed" || got.Connected || got.Generation != uint64(7) || got.ActiveCount != 2 {
		t.Fatalf("closed summary = %#v", got)
	}
	if got.QuoteLastError == nil || *got.QuoteLastError != "quote down" ||
		got.StreamLastError == nil || *got.StreamLastError != "stream down" {
		t.Fatalf("trimmed error summaries = %#v", got)
	}
	if got.LastRefreshAt == nil || got.StreamRetryAt == nil {
		t.Fatalf("time pointers missing = %#v", got)
	}

	if got := marketDataRuntimeSummary(mdsrv.RuntimeState{Connected: true, ActiveCount: 4}); got.Status != "connected" {
		t.Fatalf("connected summary = %#v", got)
	}
	if got := marketDataRuntimeSummary(mdsrv.RuntimeState{QuoteLastError: "boom"}); got.Status != "degraded" {
		t.Fatalf("degraded summary = %#v", got)
	}
	if got := marketDataRuntimeSummary(mdsrv.RuntimeState{ActiveCount: 1}); got.Status != "connecting" {
		t.Fatalf("connecting summary = %#v", got)
	}
}

type fakeStrategyRuntimeSummary struct {
	summary stratsrv.RuntimeSummary
}

func (f fakeStrategyRuntimeSummary) RuntimeSummary() stratsrv.RuntimeSummary {
	return f.summary
}

func TestStrategyRuntimeSummaryDelegatesAndDefaults(t *testing.T) {
	got := StrategyRuntimeSummary(nil)
	if got.Status != "idle" || got.ActiveStrategies != 0 ||
		!got.SupportsBacktestParity || len(got.ActiveInstances) != 0 {
		t.Fatalf("nil runtime summary = %#v", got)
	}
	source := fakeStrategyRuntimeSummary{summary: stratsrv.RuntimeSummary{Status: "running", ActiveStrategies: 2}}
	if got := StrategyRuntimeSummary(source); got.ActiveStrategies != 2 {
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
