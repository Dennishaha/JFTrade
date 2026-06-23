package indicatorruntime

import (
	"math"
	"testing"
	"time"
)

func TestIndicatorRuntimeMovingAverageSnapshotForTradingWindowUsesCachedLabels(t *testing.T) {
	values := []float64{10, 20, 30, 40}
	volumes := []float64{1, 1, 1, 1}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 19, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 19, 0, 0, 0, time.UTC),
	}
	config := movingAverageConfig{averageType: "SMA", period: 1, timeUnit: "day", source: "close"}
	cache := newSnapshotSeriesCache()
	labelKeys := cache.getTradingPeriodLabels(endTimes, "US.AAPL", config.timeUnit, false)
	runtime := &indicatorRuntime{
		symbol:              "US.AAPL",
		volumes:             volumes,
		closes:              values,
		endTimes:            endTimes,
		tradingPeriodLabels: map[string][]int64{"day": labelKeys},
	}

	actual := snapshotValueToMap(runtime.movingAverageSnapshotForTradingWindow(config, cache), [...]string{"value", "previous"})
	current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(values, volumes, labelKeys, config)
	if !handled || !currentOK || !previousOK {
		t.Fatalf("snapshot from keys flags = handled:%v currentOK:%v previousOK:%v", handled, currentOK, previousOK)
	}
	expected := snapshotValueToMap(cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK), [...]string{"value", "previous"})
	assertSnapshotMapApproxEqual(t, actual, expected)
}

func TestIndicatorRuntimeMovingAverageSnapshotForTradingWindowFallsBackWhenLabelsMismatch(t *testing.T) {
	values := []float64{10, 20, 30, 40}
	volumes := []float64{1, 2, 3, 4}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 19, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 19, 0, 0, 0, time.UTC),
	}
	config := movingAverageConfig{averageType: "VWMA", period: 1, timeUnit: "day", source: "close"}
	cache := newSnapshotSeriesCache()
	runtime := &indicatorRuntime{
		symbol:              "US.AAPL",
		volumes:             volumes,
		closes:              values,
		endTimes:            endTimes,
		tradingPeriodLabels: map[string][]int64{"day": {1}},
	}

	actual := snapshotValueToMap(runtime.movingAverageSnapshotForTradingWindow(config, cache), [...]string{"value", "previous"})
	expected := snapshotValueToMap(buildMovingAverageSnapshotForTradingWindow(values, volumes, endTimes, config, "US.AAPL", false, cache), [...]string{"value", "previous"})
	assertSnapshotMapApproxEqual(t, actual, expected)
}

func TestCalculateTradingWindowMovingAverageCurrentValueMatchesMaterializedWindow(t *testing.T) {
	values := []float64{10, 20, 30, 40}
	volumes := []float64{1, 2, 3, 4}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 19, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 19, 0, 0, 0, time.UTC),
	}
	config := movingAverageConfig{averageType: "VWMA", period: 1, timeUnit: "day"}
	cache := newSnapshotSeriesCache()

	actual, actualOK := calculateTradingWindowMovingAverageCurrentValue(values, volumes, endTimes, config, "US.AAPL", len(values), false, cache)
	selectedValues, selectedVolumes := selectTradingWindowSeriesWithCache(values, volumes, endTimes, config.period, config.timeUnit, "US.AAPL", len(values), false, cache)
	expected, expectedOK := calculateMovingAverageCurrentValue(selectedValues, selectedVolumes, config)
	if actualOK != expectedOK {
		t.Fatalf("current value ok = %v, want %v", actualOK, expectedOK)
	}
	if !actualOK {
		t.Fatal("expected current value to be available")
	}
	if math.Abs(actual-expected) > 1e-9 {
		t.Fatalf("current value = %v, want %v", actual, expected)
	}
}

func TestCalculateMovingAverageCurrentValueFromSelectedCoversWeightedAndMaterializedBranches(t *testing.T) {
	values := []float64{10, 20, 30, 40, 50}
	volumes := []float64{1, 2, 3, 4, 5}
	selected := []int{1, 3, 4}
	cache := newSnapshotSeriesCache()

	tests := []struct {
		name   string
		config movingAverageConfig
		want   func() (float64, bool)
	}{
		{
			name:   "ma",
			config: movingAverageConfig{averageType: "MA", period: len(selected)},
			want: func() (float64, bool) {
				return simpleMovingAverageFromSelected(values, selected)
			},
		},
		{
			name:   "lwma",
			config: movingAverageConfig{averageType: "LWMA", period: len(selected)},
			want: func() (float64, bool) {
				return linearWeightedMovingAverageFromSelected(values, selected, len(selected))
			},
		},
		{
			name:   "vwma",
			config: movingAverageConfig{averageType: "VWMA", period: len(selected)},
			want: func() (float64, bool) {
				return volumeWeightedMovingAverageFromSelected(values, volumes, selected)
			},
		},
		{
			name:   "ema-materialized",
			config: movingAverageConfig{averageType: "EMA", period: len(selected)},
			want: func() (float64, bool) {
				selectedValues, selectedVolumes := materializeTradingWindowSeriesFromSelected(values, volumes, selected, cache)
				return calculateMovingAverageCurrentValue(selectedValues, selectedVolumes, movingAverageConfig{averageType: "EMA", period: len(selected)})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, actualOK := calculateMovingAverageCurrentValueFromSelected(values, volumes, selected, tt.config, cache)
			expected, expectedOK := tt.want()
			if actualOK != expectedOK {
				t.Fatalf("ok = %v, want %v", actualOK, expectedOK)
			}
			if actualOK && math.Abs(actual-expected) > 1e-9 {
				t.Fatalf("value = %v, want %v", actual, expected)
			}
		})
	}

	if value, ok := calculateMovingAverageCurrentValueFromSelected(values, volumes, nil, movingAverageConfig{averageType: "MA", period: 1}, cache); ok || value != 0 {
		t.Fatalf("empty selection = (%v, %v), want (0, false)", value, ok)
	}
}

func TestTradingWindowMovingAverageAggregatorAndStateHandleBusinessCases(t *testing.T) {
	if _, handled := newTradingWindowMovingAverageAggregator(movingAverageConfig{averageType: "EMA"}); handled {
		t.Fatal("expected EMA to bypass aggregator path")
	}

	sma, handled := newTradingWindowMovingAverageAggregator(movingAverageConfig{averageType: "SMA"})
	if !handled {
		t.Fatal("expected SMA aggregator")
	}
	for _, value := range []float64{40, 30, 20} {
		if !sma.push(value, nil, 0) {
			t.Fatal("expected SMA push to succeed")
		}
	}
	if value, ok := sma.value(); !ok || math.Abs(value-30) > 1e-9 {
		t.Fatalf("SMA aggregator value = (%v, %v), want (30, true)", value, ok)
	}

	lwma, handled := newTradingWindowMovingAverageAggregator(movingAverageConfig{averageType: "LWMA"})
	if !handled {
		t.Fatal("expected LWMA aggregator")
	}
	for _, value := range []float64{30, 20, 10} {
		if !lwma.push(value, nil, 0) {
			t.Fatal("expected LWMA push to succeed")
		}
	}
	if value, ok := lwma.value(); !ok || math.Abs(value-(140.0/6.0)) > 1e-9 {
		t.Fatalf("LWMA aggregator value = (%v, %v)", value, ok)
	}

	vwma, handled := newTradingWindowMovingAverageAggregator(movingAverageConfig{averageType: "VWMA"})
	if !handled {
		t.Fatal("expected VWMA aggregator")
	}
	if vwma.push(10, []float64{1}, 4) {
		t.Fatal("expected VWMA push with missing volume to fail")
	}
	if value, ok := vwma.value(); ok || value != 0 {
		t.Fatalf("VWMA missing-volume value = (%v, %v), want (0, false)", value, ok)
	}

	state := tradingWindowMovingAverageState{aggregator: tradingWindowMovingAverageAggregator{kind: "sma"}}
	state.push(2, 2, 40, nil, 0)
	state.push(2, 2, 30, nil, 1)
	state.push(2, 1, 20, nil, 2)
	state.push(2, 0, 10, nil, 3)
	if !state.done {
		t.Fatal("expected state to stop once enough distinct trading windows were collected")
	}
	if value, ok := state.value(); !ok || math.Abs(value-30) > 1e-9 {
		t.Fatalf("state value = (%v, %v), want (30, true)", value, ok)
	}
}

func TestTradingWindowHMAAndSnapshotHelpersCoverEdgeSemantics(t *testing.T) {
	values := []float64{10, 16}
	labelKeys := []int64{9, 9}
	summary := summarizeTradingWindowSelectionFromKeys(labelKeys, 1, len(labelKeys))
	if !summary.valid || summary.count != 2 {
		t.Fatalf("summary = %#v, want valid two-point window", summary)
	}

	hma, ok := calculateHMAFromTradingWindowSelection(values, labelKeys, summary)
	if !ok || math.Abs(hma-18) > 1e-9 {
		t.Fatalf("HMA selection = (%v, %v), want (18, true)", hma, ok)
	}

	current, currentOK, handled := calculateTradingWindowSequenceValueFromKeys(values, labelKeys, "HMA", 1, len(values))
	if !handled || !currentOK || math.Abs(current-18) > 1e-9 {
		t.Fatalf("HMA sequence = (%v, %v, %v), want (18, true, true)", current, currentOK, handled)
	}

	invalidSummary := tradingWindowSelectionSummary{startKey: 9, startIndex: 0, endIndex: 2, count: 3, valid: true}
	if value, ok := calculateHMAFromTradingWindowSelection([]float64{10, 16, 20}, []int64{9, 9, 9}, invalidSummary); ok || value != 0 {
		t.Fatalf("invalid HMA summary = (%v, %v), want (0, false)", value, ok)
	}

	current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(
		[]float64{10},
		[]float64{1},
		nil,
		movingAverageConfig{averageType: "SMA", period: 1, timeUnit: "day"},
	)
	if !handled || currentOK || previousOK || current != 0 || previous != 0 {
		t.Fatalf("length-mismatch snapshot = current:%v previous:%v currentOK:%v previousOK:%v handled:%v", current, previous, currentOK, previousOK, handled)
	}
}

func TestTradingWindowCurrentValueOnlineWithCacheHandlesShortCircuitInputs(t *testing.T) {
	values := []float64{10, 20}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
	}
	cache := newSnapshotSeriesCache()
	config := movingAverageConfig{averageType: "VWMA", period: 1, timeUnit: "day"}

	if value, ok, handled := calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(values, []float64{1, 2}, endTimes, config, "US.AAPL", 0, false, cache); !handled || ok || value != 0 {
		t.Fatalf("upperBound=0 result = (%v, %v, %v), want (0, false, true)", value, ok, handled)
	}

	if value, ok, handled := calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(values, []float64{1}, endTimes, config, "US.AAPL", len(values), false, cache); !handled || ok || value != 0 {
		t.Fatalf("missing-volume result = (%v, %v, %v), want (0, false, true)", value, ok, handled)
	}
}
