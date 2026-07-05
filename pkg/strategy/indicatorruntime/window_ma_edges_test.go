package indicatorruntime

import (
	"testing"
	"time"
)

func TestTradingWindowMovingAverageBuildsOnlineAndFallbackSnapshots(t *testing.T) {
	cache := newSnapshotSeriesCache()
	values := []float64{5, 10, 20, 50, 30, 40, 60, 80}
	volumes := []float64{1, 1, 1, 1, 1, 1, 1, 1}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 1, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 7, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 13, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 1, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 7, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 13, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
	}
	ema, emaOK, handled := calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(
		values, volumes, endTimes,
		movingAverageConfig{averageType: "EMA", period: 1, timeUnit: "day"},
		"US.AAPL", len(values), true, cache,
	)
	if !handled || !emaOK || ema <= 0 {
		t.Fatalf("EMA online current = %v/%v handled=%v", ema, emaOK, handled)
	}

	if snapshot := buildMovingAverageSnapshotForTradingWindow(
		values, volumes, nil,
		movingAverageConfig{averageType: "SMA", period: 2, timeUnit: "day"},
		"US.AAPL", false, cache,
	); snapshot != nil {
		t.Fatalf("missing endTimes snapshot = %#v, want nil", snapshot)
	}

	if value, ok := calculateMovingAverageCurrentValue(nil, nil, movingAverageConfig{averageType: "SMA"}); ok || value != 0 {
		t.Fatalf("empty current MA = %v/%v, want zero false", value, ok)
	}
	if value, ok := calculateTradingWindowMovingAverageCurrentValue(
		values, volumes, endTimes,
		movingAverageConfig{averageType: "SMA", period: 1, timeUnit: "day"},
		"US.AAPL", 0, false, cache,
	); ok || value != 0 {
		t.Fatalf("zero upperBound trading-window MA = %v/%v, want zero false", value, ok)
	}
}

func TestTradingWindowMovingAverageSelectionRejectsInconsistentWindows(t *testing.T) {
	invalidSummary := tradingWindowSelectionSummary{}
	if value, ok := calculateEMAFromTradingWindowSelection([]float64{10}, []int64{1}, invalidSummary); ok || value != 0 {
		t.Fatalf("EMA invalid summary = %v/%v, want zero false", value, ok)
	}
	if value, ok := calculateSMMAFromTradingWindowSelection([]float64{10}, []int64{1}, invalidSummary); ok || value != 0 {
		t.Fatalf("SMMA invalid summary = %v/%v, want zero false", value, ok)
	}
	if value, ok := calculateHMAFromTradingWindowSelection([]float64{10}, []int64{1}, invalidSummary); ok || value != 0 {
		t.Fatalf("HMA invalid summary = %v/%v, want zero false", value, ok)
	}

	mismatched := tradingWindowSelectionSummary{valid: true, count: 2, startKey: 1, startIndex: 0, endIndex: 1}
	if value, ok := calculateEMAFromTradingWindowSelection([]float64{10, 12}, []int64{1}, mismatched); ok || value != 0 {
		t.Fatalf("EMA mismatched labels = %v/%v, want zero false", value, ok)
	}
	if value, ok := calculateSMMAFromTradingWindowSelection([]float64{10, 12}, []int64{1, invalidTradingPeriodLabelKey}, mismatched); ok || value != 0 {
		t.Fatalf("SMMA skipped label = %v/%v, want zero false", value, ok)
	}

	overflow := tradingWindowSelectionSummary{valid: true, count: 2, startKey: 1, startIndex: 0, endIndex: 2}
	if value, ok := calculateHMAFromTradingWindowSelection([]float64{10, 12, 14}, []int64{1, 1, 1}, overflow); ok || value != 0 {
		t.Fatalf("HMA inconsistent count = %v/%v, want zero false", value, ok)
	}
	if value, ok := calculateSingleValueFromTradingWindowSelection([]float64{10}, []int64{invalidTradingPeriodLabelKey}, tradingWindowSelectionSummary{valid: true, count: 1, startKey: 1}); ok || value != 0 {
		t.Fatalf("single value skipped label = %v/%v, want zero false", value, ok)
	}

	if _, ok, handled := calculateTradingWindowSequenceValueFromKeys([]float64{10}, []int64{1}, "UNKNOWN", 1, 1); handled || ok {
		t.Fatalf("unknown trading-window sequence handled=%v ok=%v, want false false", handled, ok)
	}
	var aggregator *tradingWindowMovingAverageAggregator
	if aggregator.push(10, nil, 0) {
		t.Fatal("nil trading-window aggregator accepted a value")
	}
	if value, ok := (tradingWindowMovingAverageAggregator{kind: "unknown", count: 1}).value(); ok || value != 0 {
		t.Fatalf("unknown aggregator value = %v/%v, want zero false", value, ok)
	}
}

func TestTradingWindowMovingAverageSelectionComputesSingleAndSmoothedWindows(t *testing.T) {
	values := []float64{10, 12, 20, 24}
	labelKeys := []int64{1, 1, 2, 2}
	summary := summarizeTradingWindowSelectionFromKeys(labelKeys, 2, len(labelKeys))
	if !summary.valid || summary.count != 4 {
		t.Fatalf("summary = %+v", summary)
	}
	if value, ok := calculateEMAFromTradingWindowSelection(values, labelKeys, summary); !ok || value <= 10 || value >= 24 {
		t.Fatalf("EMA selection = %v/%v", value, ok)
	}
	if value, ok := calculateSMMAFromTradingWindowSelection(values, labelKeys, summary); !ok || value != 16.5 {
		t.Fatalf("SMMA selection = %v/%v, want 16.5/true", value, ok)
	}

	single := summarizeTradingWindowSelectionFromKeys([]int64{1, invalidTradingPeriodLabelKey, 2}, 1, 3)
	if value, ok, handled := calculateTradingWindowSequenceValueFromKeys([]float64{10, 99, 20}, []int64{1, invalidTradingPeriodLabelKey, 2}, "TMA", 1, 3); !handled || !ok || value != 20 {
		t.Fatalf("TMA single-window value = %v/%v handled=%v", value, ok, handled)
	}
	if value, ok := calculateHMAFromTradingWindowSelection([]float64{10, 99, 20}, []int64{1, invalidTradingPeriodLabelKey, 2}, single); !ok || value != 20 {
		t.Fatalf("HMA single-window value = %v/%v", value, ok)
	}

	if value, ok := (tradingWindowMovingAverageAggregator{kind: "vwma", count: 2, weightedSum: 10, volumeSum: 0}).value(); ok || value != 0 {
		t.Fatalf("zero-volume VWMA value = %v/%v, want zero false", value, ok)
	}
}
