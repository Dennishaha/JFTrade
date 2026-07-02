package indicatorruntime

import (
	"math"
	"testing"
	"time"
)

func TestKDJBoundaryAndDivergenceFallbackEdges(t *testing.T) {
	if state := newRollingKDJState(kdjConfig{period: 0, m1: 3, m2: 3}, 8, []int{2}); state != nil {
		t.Fatalf("newRollingKDJState(invalid period) = %#v, want nil", state)
	}

	state := newRollingKDJState(kdjConfig{period: 3, m1: 3, m2: 3}, 2, nil)
	if state == nil {
		t.Fatal("newRollingKDJState() = nil")
	}
	if state.boundaryKAt(-1) != 0 || (*rollingKDJState)(nil).boundaryDByKAt(1) != 0 || (*rollingKDJState)(nil).boundaryDByDAt(1) != 0 {
		t.Fatal("nil/negative KDJ boundary lookups should return 0")
	}
	if got := state.boundaryKAt(5); math.Abs(got-math.Pow(state.kBeta, 5)) > 1e-12 {
		t.Fatalf("boundaryKAt(5) = %v", got)
	}
	if got := state.boundaryDByDAt(5); math.Abs(got-math.Pow(state.dBeta, 5)) > 1e-12 {
		t.Fatalf("boundaryDByDAt(5) = %v", got)
	}
	if got := state.boundaryDByKAt(5); got <= 0 {
		t.Fatalf("boundaryDByKAt(5) = %v, want positive recursive shift", got)
	}
	if state.detectDivergence([]float64{10, 11, 12}, "top", 2) {
		t.Fatal("empty KDJ tail should not report divergence")
	}

	state.jTail = []float64{60, 65, 63, 61}
	if !state.detectDivergence([]float64{10, 11, 12, 13}, "top", 3) {
		t.Fatal("KDJ fallback top divergence not detected")
	}
	state.jTail = []float64{40, 35, 37, 39}
	if !state.detectDivergence([]float64{10, 9, 8, 7}, "bottom", 3) {
		t.Fatal("KDJ fallback bottom divergence not detected")
	}
	if state.detectDivergence([]float64{10, 9, 8, 7}, "sideways", 3) {
		t.Fatal("unknown KDJ divergence direction should be false")
	}
}

func TestRSIStateBoundaryAndFallbackEdges(t *testing.T) {
	if state := newRollingRSIState(0, 5, []int{2}); state != nil {
		t.Fatalf("newRollingRSIState(0) = %#v, want nil", state)
	}
	if (&indicatorRuntime{}).rsiSeries(3) != nil {
		t.Fatal("runtime without closes/states should return nil RSI series")
	}
	if got, ok := (&indicatorRuntime{}).rsiSnapshotValue(3, nil); ok || got != 0 {
		t.Fatalf("empty RSI snapshot = (%v, %v), want zero/false", got, ok)
	}
	if (&rollingRSIState{}).detectDivergence([]float64{1, 2}, "top", 0) {
		t.Fatal("RSI divergence with non-positive lookback should be false")
	}
	nilValue, nilOK := (*rollingRSIState)(nil).currentValue()
	if (*rollingRSIState)(nil).seriesValues() != nil || nilOK || nilValue != 0 {
		t.Fatalf("nil RSI state should expose empty series and no current value, got (%v, %v)", nilValue, nilOK)
	}

	state := newRollingRSIState(2, 0, nil)
	state.push(10, 9, false)
	if len(state.seriesValues()) != 0 {
		t.Fatal("RSI push without previous close should not append")
	}
	state.push(11, 10, true)
	state.push(10, 11, true)
	if len(state.seriesValues()) != 0 {
		t.Fatal("RSI maxLength <= 0 should discard calculated series after update")
	}

	fallback := &rollingRSIState{series: []float64{60, 65, 63, 61}}
	if !fallback.detectDivergence([]float64{10, 11, 12, 13}, "top", 3) {
		t.Fatal("RSI fallback top divergence not detected")
	}
	fallback.series = []float64{40, 35, 37, 39}
	if !fallback.detectDivergence([]float64{10, 9, 8, 7}, "bottom", 3) {
		t.Fatal("RSI fallback bottom divergence not detected")
	}
}

func TestTradingWindowMovingAverageCurrentValueFallbackEdges(t *testing.T) {
	cache := newSnapshotSeriesCache()
	values := []float64{10, 12, 14}
	volumes := []float64{100, 110, 120}
	endTimes := []time.Time{
		time.Date(2026, time.June, 12, 13, 30, 0, 0, time.UTC),
		time.Date(2026, time.June, 12, 14, 30, 0, 0, time.UTC),
		time.Date(2026, time.June, 15, 13, 30, 0, 0, time.UTC),
	}

	if value, ok := calculateTradingWindowMovingAverageCurrentValue(values, volumes, endTimes, movingAverageConfig{averageType: "SMA", period: 2, timeUnit: "day"}, "US.AAPL", len(values), false, cache); !ok || value <= 0 {
		t.Fatalf("trading-window SMA current = (%v, %v), want positive", value, ok)
	}
	if value, ok := calculateTradingWindowMovingAverageCurrentValue(values, volumes, endTimes, movingAverageConfig{averageType: "UNKNOWN", period: 2, timeUnit: "day"}, "US.AAPL", len(values), false, cache); !ok || value != 12 {
		t.Fatalf("unknown trading-window MA current = (%v, %v), want SMA fallback 12/true", value, ok)
	}
	if value, ok := calculateTradingWindowMovingAverageCurrentValue(values, volumes, nil, movingAverageConfig{averageType: "SMA", period: 2, timeUnit: "day"}, "US.AAPL", len(values), false, cache); ok || value != 0 {
		t.Fatalf("missing endTimes trading-window MA current = (%v, %v), want zero/false", value, ok)
	}
	if snapshot := buildMovingAverageSnapshotForTradingWindow(nil, nil, nil, movingAverageConfig{averageType: "SMA", period: 2, timeUnit: "day"}, "US.AAPL", false, cache); snapshot != nil {
		t.Fatalf("empty trading-window snapshot = %#v, want nil", snapshot)
	}
}
