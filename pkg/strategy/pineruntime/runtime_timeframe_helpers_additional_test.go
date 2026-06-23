package pineruntime

import (
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestPineBarTimeAndCloseTimeCoverFallbackBranches(t *testing.T) {
	if got := pineBarTime(nil); !got.IsZero() {
		t.Fatalf("pineBarTime(nil) = %v, want zero", got)
	}

	start := time.Date(2026, time.June, 23, 14, 30, 0, 0, time.UTC)
	end := start.Add(time.Minute)
	kline := &types.KLine{
		Symbol:    "US.AAPL",
		Interval:  types.Interval1m,
		StartTime: types.Time(start),
		EndTime:   types.Time(end),
		Close:     fixedpoint.NewFromFloat(101),
	}
	if got := pineBarTime(kline); !got.Equal(start) {
		t.Fatalf("pineBarTime() = %v, want %v", got, start)
	}
	if got, ok := pineBarCloseTime(kline, types.Interval1m); !ok || !got.Equal(end) {
		t.Fatalf("pineBarCloseTime(with explicit end) = %v %v, want %v true", got, ok, end)
	}

	derived := &types.KLine{
		Symbol:    "US.AAPL",
		Interval:  types.Interval5m,
		StartTime: types.Time(start),
		Close:     fixedpoint.NewFromFloat(101),
	}
	if got, ok := pineBarCloseTime(derived, types.Interval5m); !ok || !got.Equal(start.Add(5*time.Minute)) {
		t.Fatalf("pineBarCloseTime(derived) = %v %v, want %v true", got, ok, start.Add(5*time.Minute))
	}

	if got, ok := pineBarCloseTime(&types.KLine{}, types.Interval("bad")); ok || !got.IsZero() {
		t.Fatalf("pineBarCloseTime(invalid) = %v %v, want zero false", got, ok)
	}
}

func TestPineRuntimeTimeframeHelpersCoverCoreSemantics(t *testing.T) {
	scope := &evaluationScope{
		runtime: &strategyRuntime{interval: types.Interval15m, symbol: "US.AAPL"},
		currentKline: &types.KLine{
			Symbol:   "HK.00700",
			Interval: types.Interval1m,
		},
	}
	if got := scope.runtimeInterval(); got != types.Interval15m {
		t.Fatalf("runtimeInterval(runtime) = %q, want 15m", got)
	}
	scope.runtime.interval = ""
	if got := scope.runtimeInterval(); got != types.Interval1m {
		t.Fatalf("runtimeInterval(kline) = %q, want 1m", got)
	}
	if got := (*evaluationScope)(nil).runtimeInterval(); got != "" {
		t.Fatalf("runtimeInterval(nil) = %q, want empty", got)
	}

	if got := pineSymbolPrefix("NASDAQ:AAPL"); got != "NASDAQ" {
		t.Fatalf("pineSymbolPrefix(colon) = %q, want NASDAQ", got)
	}
	if got := pineSymbolPrefix("HK.00700"); got != "HK" {
		t.Fatalf("pineSymbolPrefix(dot) = %q, want HK", got)
	}
	if got := pineSymbolPrefix("AAPL"); got != "" {
		t.Fatalf("pineSymbolPrefix(no prefix) = %q, want empty", got)
	}

	unitCases := map[types.Interval]string{
		types.Interval("15s"):     "second",
		types.Interval("3month"):  "month",
		types.Interval("2w"):      "week",
		types.Interval("1d"):      "day",
		types.Interval("4h"):      "hour",
		types.Interval("30m"):     "minute",
		types.Interval("unknown"): "",
	}
	for interval, want := range unitCases {
		if got := pineTimeframeUnit(interval); got != want {
			t.Fatalf("pineTimeframeUnit(%q) = %q, want %q", interval, got, want)
		}
	}

	if !pineTimeframeIsIntraday(types.Interval("15s")) || !pineTimeframeIsIntraday(types.Interval("4h")) {
		t.Fatal("pineTimeframeIsIntraday should accept second/minute/hour intervals")
	}
	if pineTimeframeIsIntraday(types.Interval("1d")) {
		t.Fatal("pineTimeframeIsIntraday(day) = true, want false")
	}

	if duration, ok := pineIntervalDuration(types.Interval("15m")); !ok || duration != 15*time.Minute {
		t.Fatalf("pineIntervalDuration(15m) = %v %v, want 15m true", duration, ok)
	}
	if duration, ok := pineIntervalDuration(types.Interval("2w")); !ok || duration != 14*24*time.Hour {
		t.Fatalf("pineIntervalDuration(2w) = %v %v, want 14d true", duration, ok)
	}
	if duration, ok := pineIntervalDuration(types.Interval("1month")); ok || duration != 0 {
		t.Fatalf("pineIntervalDuration(month) = %v %v, want 0 false", duration, ok)
	}
	if duration, ok := pineIntervalDuration(types.Interval("bad")); ok || duration != 0 {
		t.Fatalf("pineIntervalDuration(bad) = %v %v, want 0 false", duration, ok)
	}
}

func TestPendingOrderTriggerHelpersCoverStopLimitAndDirectionBranches(t *testing.T) {
	if pendingOrderTriggered(pendingOrder{
		action:    strategyir.OrderActionBuy,
		hasStop:   true,
		stopPrice: 101,
		hasLimit:  true,
		limitPrice: 100,
	}, 105, 99) {
		t.Fatal("inactive stop-limit order should not trigger before activation")
	}

	if !pendingOrderTriggered(pendingOrder{
		action:     strategyir.OrderActionBuy,
		hasStop:    true,
		stopPrice:  101,
		hasLimit:   true,
		limitPrice: 100,
		activated:  true,
	}, 105, 99) {
		t.Fatal("activated buy stop-limit should trigger when low reaches limit")
	}

	if !pendingOrderTriggered(pendingOrder{
		action:     strategyir.OrderActionShort,
		hasStop:    true,
		stopPrice:  98,
		hasLimit:   true,
		limitPrice: 99,
		activated:  true,
	}, 100, 95) {
		t.Fatal("activated short stop-limit should trigger when high reaches limit")
	}

	if !pendingStopTriggered(pendingOrder{action: strategyir.OrderActionCover, stopPrice: 102}, 103, 100) {
		t.Fatal("pendingStopTriggered(cover) = false, want true")
	}
	if !pendingLimitTriggered(pendingOrder{action: strategyir.OrderActionShort, limitPrice: 99}, 100, 95) {
		t.Fatal("pendingLimitTriggered(short) = false, want true")
	}
	if pendingLimitTriggered(pendingOrder{action: strategyir.OrderAction("bad"), limitPrice: 99}, 100, 95) {
		t.Fatal("pendingLimitTriggered(invalid action) = true, want false")
	}
}
