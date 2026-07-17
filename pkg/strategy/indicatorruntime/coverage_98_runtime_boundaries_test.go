package indicatorruntime

import (
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestCoverage98RollingStatesRejectIncompleteMarketInputs(t *testing.T) {
	if states := newRollingMovingAverageStates(indicatorRequirements{
		ma: []movingAverageConfig{
			{averageType: "SMA", period: 0},
			{averageType: "SMA", period: 3, timeUnit: "5m"},
		},
	}, 1); states != nil {
		t.Fatalf("invalid/numeric-timeframe MA states = %#v, want nil", states)
	}

	var nilRuntime *indicatorRuntime
	nilRuntime.pushMovingAverageStates(1, 2, 0, 1.5, 10)
	if got := nilRuntime.movingAverageSnapshot(movingAverageConfig{}, nil); got != nil {
		t.Fatalf("nil runtime moving-average snapshot = %#v", got)
	}
	if got := nilRuntime.movingAverageSnapshotForTradingWindow(movingAverageConfig{}, nil); got != nil {
		t.Fatalf("nil runtime trading-window snapshot = %#v", got)
	}

	config := movingAverageConfig{averageType: "SMA", period: 2, source: "close"}
	state := &rollingMovingAverageSnapshotState{kind: "SMA", period: 2}
	runtime := &indicatorRuntime{maStates: map[movingAverageConfig]*rollingMovingAverageSnapshotState{config: state}}
	runtime.pushMovingAverageStates(1, 2, 0, 10, 1)
	if snapshot := state.snapshot(); snapshot != nil {
		t.Fatalf("partial MA snapshot = %#v, want nil", snapshot)
	}
	runtime.pushMovingAverageStates(2, 3, 1, 14, 1)
	if value, ok := state.PreferredScalarValue(); !ok || value != 12 {
		t.Fatalf("rolling SMA value = (%v, %v), want (12, true)", value, ok)
	}
	if current, previous, currentOK, previousOK, handled := state.SeriesField("other"); handled || current != 0 || previous != 0 || currentOK || previousOK {
		t.Fatalf("unexpected MA other-field result = (%v, %v, %v, %v, %v)", current, previous, currentOK, previousOK, handled)
	}
	if value, ok := state.FieldValue("previous"); !ok || value != nil {
		t.Fatalf("rolling SMA previous field = (%#v, %v), want (nil, true)", value, ok)
	}
	if value, ok := state.FieldValue("unknown"); ok || value != nil {
		t.Fatalf("rolling SMA unknown field = (%#v, %v)", value, ok)
	}
	if snapshot := runtime.movingAverageSnapshot(config, newSnapshotSeriesCache()); snapshot == nil {
		t.Fatal("runtime did not expose rolling MA snapshot")
	}

	var nilState *rollingMovingAverageSnapshotState
	nilState.push(1, 1)
	if snapshot := nilState.snapshotValue(); snapshot != nil {
		t.Fatalf("nil MA state snapshot = %#v", snapshot)
	}
	if value, ok := nilState.FieldValue("value"); ok || value != nil {
		t.Fatalf("nil MA field = (%#v, %v)", value, ok)
	}
	if current, previous, currentOK, previousOK := lastTwoSequenceValues(nil); current != 0 || previous != 0 || currentOK || previousOK {
		t.Fatalf("empty MA sequence = (%v, %v, %v, %v)", current, previous, currentOK, previousOK)
	}
}

func TestCoverage98RollingStochasticHandlesFlatAndMalformedSeries(t *testing.T) {
	var nilState *rollingStochState
	nilState.push(3, 1, 2)
	if states := newRollingStochStates(indicatorRequirements{
		stoch: []sourcePeriodConfig{
			{source: "volume", period: 2},
			{source: "unknown", period: 2},
			{source: "close", period: 0},
			{source: "close", period: 2, timeUnit: "day"},
		},
	}); states != nil {
		t.Fatalf("invalid stochastic states = %#v, want nil", states)
	}

	if value, ok := calculateStochAt([]float64{2}, []float64{2}, []float64{2}, 0, 0); ok || value != 0 {
		t.Fatalf("zero-period stoch = (%v, %v)", value, ok)
	}
	if value, ok := calculateStochAt([]float64{2, 2}, []float64{2, 2}, []float64{2}, 2, 1); ok || value != 0 {
		t.Fatalf("mismatched stoch series = (%v, %v)", value, ok)
	}
	if value, ok := calculateStochAt([]float64{2, 2}, []float64{2, 2}, []float64{2, 2}, 2, 1); !ok || value != 50 {
		t.Fatalf("flat stochastic value = (%v, %v), want (50, true)", value, ok)
	}

	config := sourcePeriodConfig{source: "close", period: 2}
	state := &rollingStochState{source: "close", period: 2}
	runtime := &indicatorRuntime{stochStates: map[sourcePeriodConfig]*rollingStochState{config: state}}
	runtime.pushStochStates(1, 4, 2, 3, 10)
	runtime.pushStochStates(2, 6, 1, 5, 10)
	if snapshot := runtime.stochSnapshot(config, newSnapshotSeriesCache()); snapshot == nil {
		t.Fatal("rolling stochastic snapshot is nil after a full window")
	}
	if snapshot := (&indicatorRuntime{}).stochSnapshot(sourcePeriodConfig{source: "close", period: 2, timeUnit: "5m"}, nil); snapshot != nil {
		t.Fatalf("missing fixed-timeframe stochastic snapshot = %#v", snapshot)
	}
}

func TestCoverage98BollingerStateKeepsPartialWindowsOutOfSignals(t *testing.T) {
	var nilState *rollingBollingerState
	nilState.push(1)
	if snapshot := nilState.snapshot(); snapshot != nil {
		t.Fatalf("nil bollinger snapshot = %#v", snapshot)
	}
	if value, ok := nilState.PreferredScalarValue(); ok || value != 0 {
		t.Fatalf("nil bollinger scalar = (%v, %v)", value, ok)
	}
	if value, ok := nilState.FieldValue("middle"); ok || value != nil {
		t.Fatalf("nil bollinger field = (%#v, %v)", value, ok)
	}

	state := &rollingBollingerState{period: 2, multiplier: 2}
	state.push(10)
	if snapshot := state.snapshotValue(); snapshot != nil {
		t.Fatalf("partial bollinger snapshot = %#v", snapshot)
	}
	state.push(14)
	if value, ok := state.PreferredScalarValue(); !ok || value != 12 {
		t.Fatalf("bollinger middle = (%v, %v), want (12, true)", value, ok)
	}
	if value, ok := state.FieldValue("upper"); !ok || value.(float64) <= 12 {
		t.Fatalf("bollinger upper = (%#v, %v)", value, ok)
	}
	if value, ok := state.FieldValue("other"); ok || value != nil {
		t.Fatalf("unknown bollinger field = (%#v, %v)", value, ok)

	}
	runtime := &indicatorRuntime{bollingerStates: map[bollingerConfig]*rollingBollingerState{{period: 2, multiplier: 2}: state}}
	if snapshot := runtime.bollingerSnapshot(bollingerConfig{period: 2, multiplier: 2}); snapshot == nil {
		t.Fatal("runtime did not expose Bollinger state")
	}
}

func TestCoverage98TradingWindowMovingAveragesRejectIncompletePeriods(t *testing.T) {
	values := []float64{10, 12, 14, 16}
	volumes := []float64{2, 1, 3, 2}
	labels := []int64{1, invalidTradingPeriodLabelKey, 2, 3}
	config := movingAverageConfig{averageType: "EMA", period: 2, timeUnit: "day"}

	current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(values, volumes, labels, config)
	if !handled || !currentOK || !previousOK || current <= previous {
		t.Fatalf("EMA trading-window snapshot = (%v, %v, %v, %v, %v)", current, previous, currentOK, previousOK, handled)
	}
	if current, _, currentOK, _, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(values, volumes, labels, movingAverageConfig{averageType: "not-supported", period: 2}); !handled || !currentOK || current <= 0 {
		t.Fatalf("unknown moving average did not use the MA fallback = (%v, %v, %v)", current, currentOK, handled)
	}
	if value, ok, handled := calculateTradingWindowSequenceValueFromKeys(values, labels, "EMA", 2, 0); !handled || ok || value != 0 {
		t.Fatalf("empty upper-bound sequence = (%v, %v, %v)", value, ok, handled)
	}
	if value, ok, handled := calculateTradingWindowSequenceValueFromKeys(values, labels, "unknown", 2, len(values)); handled || ok || value != 0 {
		t.Fatalf("unknown sequence type = (%v, %v, %v)", value, ok, handled)
	}

	summary := tradingWindowSelectionSummary{startKey: 2, startIndex: 0, endIndex: len(values) - 1, count: 3, valid: true}
	if value, ok := calculateEMAFromTradingWindowSelection(values, labels, summary); ok || value != 0 {
		t.Fatalf("incomplete EMA selection = (%v, %v)", value, ok)
	}
	if value, ok := calculateSMMAFromTradingWindowSelection(values, labels, summary); ok || value != 0 {
		t.Fatalf("incomplete SMMA selection = (%v, %v)", value, ok)
	}
	if value, ok, handled := calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(values, nil, nil, movingAverageConfig{averageType: "VWMA", period: 2}, "", len(values), false, nil); !handled || ok || value != 0 {
		t.Fatalf("missing-volume VWMA = (%v, %v, %v)", value, ok, handled)
	}

	endTimes := []time.Time{
		time.Date(2026, time.June, 1, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.June, 2, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.June, 3, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.June, 4, 15, 0, 0, 0, time.UTC),
	}
	if snapshot := buildMovingAverageSnapshotForTradingWindow(values, volumes, endTimes, movingAverageConfig{averageType: "SMA", period: 2, timeUnit: "day"}, "US.AAPL", false, newSnapshotSeriesCache()); snapshot == nil {
		t.Fatal("valid trading-window SMA snapshot is nil")
	}
	if snapshot := buildMovingAverageSnapshotForTradingWindow(values, volumes, endTimes[:1], movingAverageConfig{averageType: "SMA", period: 2, timeUnit: "day"}, "US.AAPL", false, nil); snapshot != nil {
		t.Fatalf("mismatched trading-window snapshot = %#v", snapshot)
	}
}

func TestCoverage98IndicatorRuntimeHelpersPreserveSlicesAndFallbacks(t *testing.T) {
	values := []float64{1, 2, 3}
	if got := trimFloatSeriesInPlace(values, 0); len(got) != len(values) {
		t.Fatalf("unlimited float trim = %#v", got)
	}
	if got := trimFloatSeriesInPlace(values, 2); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("float trim = %#v", got)
	}
	if got := trimInt64SeriesInPlace([]int64{1, 2, 3}, 0); len(got) != 3 {
		t.Fatalf("unlimited int trim = %#v", got)
	}
	if got := trimSessionSeriesInPlace(nil, 2); got != nil {
		t.Fatalf("nil session trim = %#v", got)
	}
	if got := reuseFloat64Slice([]float64{1, 2}, 0); got != nil {
		t.Fatalf("zero-length float reuse = %#v", got)
	}
	if got := reuseInt64Slice([]int64{1, 2}, 0); got != nil {
		t.Fatalf("zero-length int reuse = %#v", got)
	}
	if got := reuseInt64Slice([]int64{1, 2}, 1); len(got) != 1 || got[0] != 1 {
		t.Fatalf("int reuse = %#v", got)
	}

	if session := classifyKLineSession("", types.KLine{}); session != market.SessionUnknown {
		t.Fatalf("missing market session = %v", session)
	}
	if values := snapshotValueToMap(&indicatorScalarSnapshot{current: 3, hasCurrent: true}, [2]string{"value", "previous"}); values != nil {
		t.Fatalf("scalar snapshot map = %#v, want nil because it is not a map", values)
	}
	if value, ok := volumeWeightedMovingAverageFromSelected([]float64{1}, nil, nil); ok || value != 0 {
		t.Fatalf("empty selected VWMA = (%v, %v)", value, ok)
	}
	if value, ok := calculateCenterOfGravity([]float64{1}, 2); ok || value != 0 {
		t.Fatalf("short COG = (%v, %v)", value, ok)
	}
}
