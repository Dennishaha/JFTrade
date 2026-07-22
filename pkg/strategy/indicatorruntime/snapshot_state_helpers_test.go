package indicatorruntime

import (
	"math"
	"testing"
)

func TestSnapshotSeriesCacheCachesSeriesAndReusableSnapshots(t *testing.T) {
	cache := newSnapshotSeriesCache()
	values := []float64{1, 2, 3, 2, 4, 5, 4, 6}
	highs := []float64{2, 3, 4, 3, 5, 6, 5, 7}
	lows := []float64{0, 1, 2, 1, 3, 4, 3, 5}

	ema := cache.getEMASequence(values, 3)
	if len(ema) != len(values) {
		t.Fatalf("getEMASequence() len = %d, want %d", len(ema), len(values))
	}
	if got := cache.getEMASequence(values, 3); &got[0] != &ema[0] {
		t.Fatal("getEMASequence() did not reuse cached slice")
	}
	sma := cache.getSMASequence(values, 3)
	if len(sma) == 0 {
		t.Fatal("getSMASequence() returned empty result")
	}

	rsi := cache.getRSISeries(values, 2)
	if len(rsi) == 0 {
		t.Fatal("getRSISeries() returned empty result")
	}
	if got := cache.getRSISeries(values, 2); &got[0] != &rsi[0] {
		t.Fatal("getRSISeries() did not reuse cached slice")
	}

	macdConfig := macdConfig{fastPeriod: 2, slowPeriod: 3, signalPeriod: 2}
	macdSeries := cache.getMACDSeries(values, macdConfig)
	if len(macdSeries.diff) == 0 || len(macdSeries.signal) == 0 {
		t.Fatalf("getMACDSeries() = %#v, want populated series", macdSeries)
	}
	if got := cache.getMACDSeries(values, macdConfig); &got.diff[0] != &macdSeries.diff[0] {
		t.Fatal("getMACDSeries() did not reuse cached series")
	}
	macdSnapshot := cache.getMACDSnapshot(macdConfig, macdSeries).(*indicatorMACDSnapshot)
	if !macdSnapshot.hasPrevious {
		t.Fatal("getMACDSnapshot() hasPrevious = false, want true")
	}
	if macdSnapshot.histogram != (macdSnapshot.diff-macdSnapshot.signal)*2 {
		t.Fatalf("getMACDSnapshot() histogram = %v", macdSnapshot.histogram)
	}

	kdjConfig := kdjConfig{period: 3, m1: 3, m2: 3}
	kdjSeries := cache.getKDJSeries(highs, lows, values, kdjConfig)
	if len(kdjSeries.k) == 0 || len(kdjSeries.d) == 0 || len(kdjSeries.j) == 0 {
		t.Fatalf("getKDJSeries() = %#v, want populated series", kdjSeries)
	}
	if got := cache.getKDJSeries(highs, lows, values, kdjConfig); &got.k[0] != &kdjSeries.k[0] {
		t.Fatal("getKDJSeries() did not reuse cached series")
	}
	kdjSnapshot := cache.getKDJSnapshot(kdjConfig, kdjSeries).(*indicatorKDJSnapshot)
	if !kdjSnapshot.hasPrevious {
		t.Fatal("getKDJSnapshot() hasPrevious = false, want true")
	}

	scalar1 := cache.getScalarSnapshot("scalar", 1.5, true).(*indicatorScalarSnapshot)
	scalar2 := cache.getScalarSnapshot("scalar", 2.5, true).(*indicatorScalarSnapshot)
	if scalar1 != scalar2 || scalar2.current != 2.5 || !scalar2.hasCurrent {
		t.Fatalf("getScalarSnapshot() reuse = %#v %#v", scalar1, scalar2)
	}

	series1 := cache.getSeriesSnapshot("series", 2, 1, true, true).(*indicatorSeriesSnapshot)
	series2 := cache.getSeriesSnapshot("series", 3, 2, true, true).(*indicatorSeriesSnapshot)
	if series1 != series2 || series2.current != 3 || series2.previous != 2 {
		t.Fatalf("getSeriesSnapshot() reuse = %#v %#v", series1, series2)
	}

	slConfig := stopLossConfig{mode: "stopLoss", direction: "long", timeValue: 3, percentage: 2}
	stopLoss1 := cache.getStopLossSnapshot(slConfig)
	stopLoss1["mode"] = "x"
	stopLoss2 := cache.getStopLossSnapshot(slConfig)
	if stopLoss2["mode"] != "x" || len(stopLoss2) != 1 {
		t.Fatalf("getStopLossSnapshot() reuse = %#v %#v", stopLoss1, stopLoss2)
	}

	cache.reset()
	if len(cache.ema) != 0 || len(cache.sma) != 0 || len(cache.rsi) != 0 || len(cache.macd) != 0 || len(cache.kdj) != 0 {
		t.Fatalf("reset() did not clear cached series: ema=%d sma=%d rsi=%d macd=%d kdj=%d", len(cache.ema), len(cache.sma), len(cache.rsi), len(cache.macd), len(cache.kdj))
	}
}

func TestRollingATRBollingerCCIAndRSIHelpersFollowBusinessFallbacks(t *testing.T) {
	requirements := indicatorRequirements{
		atr:           []int{3, 0},
		bollinger:     []bollingerConfig{{period: 3, multiplier: 2}, {period: 0, multiplier: 1}},
		cci:           []int{3, 0},
		rsi:           []int{3},
		rsiDivergence: []rsiDivergenceConfig{{period: 3, lookback: 2}},
	}

	atrStates := newRollingATRStates(requirements)
	if len(atrStates) != 1 || atrStates[3] == nil {
		t.Fatalf("newRollingATRStates() = %#v", atrStates)
	}
	atrState := atrStates[3]
	atrState.push(10, 8, 9, 0, false)
	atrState.push(11, 9, 10, 9, true)
	atrState.push(13, 10, 12, 10, true)
	atrCurrent, ok := atrState.currentValue()
	if !ok || math.Abs(atrCurrent-7.0/3.0) > 1e-9 {
		t.Fatalf("rollingATRState.currentValue() = (%v, %v)", atrCurrent, ok)
	}
	if atrState.value() == nil {
		t.Fatal("rollingATRState.value() = nil, want scalar")
	}
	runtimeWithATR := &indicatorRuntime{atrStates: atrStates}
	if got, ok := runtimeWithATR.atrSnapshotValue(3); !ok || math.Abs(got-atrCurrent) > 1e-9 {
		t.Fatalf("atrSnapshotValue(state) = (%v, %v), want %v", got, ok, atrCurrent)
	}
	runtimeFallbackATR := &indicatorRuntime{
		highs:  []float64{10, 11, 13},
		lows:   []float64{8, 9, 10},
		closes: []float64{9, 10, 12},
	}
	if got, ok := runtimeFallbackATR.atrSnapshotValue(2); !ok || math.Abs(got-2.5) > 1e-9 {
		t.Fatalf("atrSnapshotValue(fallback) = (%v, %v), want 2.5", got, ok)
	}

	bollingerStates := newRollingBollingerStates(requirements)
	config := bollingerConfig{period: 3, multiplier: 2}
	if len(bollingerStates) != 1 || bollingerStates[config] == nil {
		t.Fatalf("newRollingBollingerStates() = %#v", bollingerStates)
	}
	bollinger := bollingerStates[config]
	bollinger.push(1)
	bollinger.push(2)
	bollinger.push(3)
	snapshot := bollinger.snapshot()
	if snapshot == nil || math.Abs(snapshot["middle"].(float64)-2) > 1e-9 {
		t.Fatalf("rollingBollingerState.snapshot() = %#v", snapshot)
	}
	if got := bollinger.snapshotValue(); got != bollinger {
		t.Fatalf("snapshotValue() = %#v, want state pointer", got)
	}
	if middle, ok := bollinger.PreferredScalarValue(); !ok || math.Abs(middle-2) > 1e-9 {
		t.Fatalf("PreferredScalarValue() = (%v, %v)", middle, ok)
	}
	if value, ok := bollinger.FieldValue("middle"); !ok || math.Abs(value.(float64)-2) > 1e-9 {
		t.Fatalf("FieldValue(middle) = (%#v, %v)", value, ok)
	}
	if value, ok := bollinger.FieldValue("upper"); !ok || value.(float64) <= 2 {
		t.Fatalf("FieldValue(upper) = (%#v, %v)", value, ok)
	}
	if value, ok := bollinger.FieldValue("lower"); !ok || value.(float64) >= 2 {
		t.Fatalf("FieldValue(lower) = (%#v, %v)", value, ok)
	}
	if _, ok := bollinger.FieldValue("missing"); ok {
		t.Fatal("FieldValue(missing) ok = true, want false")
	}
	runtimeWithBollinger := &indicatorRuntime{bollingerStates: bollingerStates}
	if got := runtimeWithBollinger.bollingerSnapshot(config); got != bollinger {
		t.Fatalf("bollingerSnapshot(state) = %#v, want state pointer", got)
	}
	runtimeFallbackBollinger := &indicatorRuntime{closes: []float64{1, 2, 3}, bollingerStates: nil}
	if got := runtimeFallbackBollinger.bollingerSnapshot(config); got == nil {
		t.Fatal("bollingerSnapshot(fallback) = nil, want snapshot")
	}

	cciStates := newRollingCCIStates(requirements)
	if len(cciStates) != 1 || cciStates[3] == nil {
		t.Fatalf("newRollingCCIStates() = %#v", cciStates)
	}
	cciState := cciStates[3]
	cciState.push(10)
	cciState.push(12)
	cciState.push(14)
	cciCurrent, ok := cciState.currentValue()
	if !ok || math.Abs(cciCurrent-100) > 1e-9 {
		t.Fatalf("rollingCCIState.currentValue() = (%v, %v), want 100", cciCurrent, ok)
	}
	runtimeWithCCI := &indicatorRuntime{cciStates: cciStates}
	if got, ok := runtimeWithCCI.cciSnapshotValue(3); !ok || math.Abs(got-100) > 1e-9 {
		t.Fatalf("cciSnapshotValue(state) = (%v, %v), want 100", got, ok)
	}
	runtimeFallbackCCI := &indicatorRuntime{
		highs:  []float64{11, 13, 15},
		lows:   []float64{9, 11, 13},
		closes: []float64{10, 12, 14},
	}
	if got, ok := runtimeFallbackCCI.cciSnapshotValue(3); !ok || math.Abs(got-100) > 1e-9 {
		t.Fatalf("cciSnapshotValue(fallback) = (%v, %v), want 100", got, ok)
	}

	rsiStates := newRollingRSIStates(requirements, 10)
	if len(rsiStates) != 1 || rsiStates[3] == nil || rsiStates[3].tailLen != 3 {
		t.Fatalf("newRollingRSIStates() = %#v", rsiStates)
	}
	rsiState := rsiStates[3]
	closes := []float64{10, 11, 13, 12, 14}
	for index := 1; index < len(closes); index++ {
		rsiState.push(closes[index], closes[index-1], true)
	}
	runtimeWithRSI := &indicatorRuntime{rsiStates: rsiStates}
	if series := runtimeWithRSI.rsiSeries(3); len(series) == 0 {
		t.Fatal("rsiSeries(state) returned empty series")
	}
	runtimeFallbackRSI := &indicatorRuntime{closes: closes}
	rsiCache := newSnapshotSeriesCache()
	if got, ok := runtimeFallbackRSI.rsiSnapshotValue(3, rsiCache); !ok || got <= 0 {
		t.Fatalf("rsiSnapshotValue(fallback) = (%v, %v)", got, ok)
	}
	if len(rsiCache.rsi[3]) == 0 {
		t.Fatal("rsiSnapshotValue(fallback) did not populate cache")
	}
}

func TestRollingStateConstructorsSkipInvalidRequirementEntries(t *testing.T) {
	requirements := indicatorRequirements{
		atr:       []int{0, 3},
		stdev:     []int{0, 4},
		cci:       []int{0, 5},
		williamsR: []int{0, 6},
		bollinger: []bollingerConfig{
			{period: 0, multiplier: 2},
			{period: 7, multiplier: 2},
		},
		kdj: []kdjConfig{
			{period: 0, m1: 3, m2: 3},
			{period: 8, m1: 3, m2: 3},
		},
		kdjDivergence: []kdjDivergenceConfig{
			{period: 9, m1: 3, m2: 3, direction: "top", lookback: 2},
			{period: 10, m1: 0, m2: 3, direction: "bottom", lookback: 2},
			{period: 11, m1: 3, m2: 0, direction: "bottom", lookback: 2},
		},
	}

	if states := newRollingATRStates(requirements); len(states) != 1 || states[3] == nil {
		t.Fatalf("newRollingATRStates() = %#v, want only period 3", states)
	}
	if states := newRollingStdDevStates(requirements); len(states) != 1 || states[4] == nil {
		t.Fatalf("newRollingStdDevStates() = %#v, want only period 4", states)
	}
	if states := newRollingCCIStates(requirements); len(states) != 1 || states[5] == nil {
		t.Fatalf("newRollingCCIStates() = %#v, want only period 5", states)
	}
	if states := newRollingWilliamsRStates(requirements); len(states) != 1 || states[6] == nil {
		t.Fatalf("newRollingWilliamsRStates() = %#v, want only period 6", states)
	}
	bollingerStates := newRollingBollingerStates(requirements)
	if len(bollingerStates) != 1 || bollingerStates[bollingerConfig{period: 7, multiplier: 2}] == nil {
		t.Fatalf("newRollingBollingerStates() = %#v, want only valid period 7", bollingerStates)
	}
	kdjStates := newRollingKDJStates(requirements, 16)
	if len(kdjStates) != 2 || kdjStates[kdjConfig{period: 8, m1: 3, m2: 3}] == nil || kdjStates[kdjConfig{period: 9, m1: 3, m2: 3}] == nil {
		t.Fatalf("newRollingKDJStates() = %#v, want valid direct and divergence configs", kdjStates)
	}
	if states := newRollingKDJStates(indicatorRequirements{kdj: []kdjConfig{{period: 0, m1: 0, m2: 0}}}, 16); states != nil {
		t.Fatalf("newRollingKDJStates(invalid only) = %#v, want nil", states)
	}
}

func TestMovingAverageAndSnapshotAccessorsExposeExpectedFields(t *testing.T) {
	requirements := indicatorRequirements{
		ma: []movingAverageConfig{
			{averageType: "SMA", period: 3, source: "close"},
			{averageType: "VWMA", period: 3, source: "close"},
			{averageType: "EMA", period: 3, source: "close"},
		},
	}

	maStates := newRollingMovingAverageStates(requirements, 1)
	smaConfig := movingAverageConfig{averageType: "SMA", period: 3, source: "close"}
	vwmaConfig := movingAverageConfig{averageType: "VWMA", period: 3, source: "close"}
	smaState := maStates[smaConfig]
	vwmaState := maStates[vwmaConfig]
	if smaState == nil || vwmaState == nil {
		t.Fatalf("newRollingMovingAverageStates() = %#v", maStates)
		return
	}

	smaState.push(1, 1)
	smaState.push(2, 1)
	smaState.push(3, 1)
	smaState.push(4, 1)
	if snapshot := smaState.snapshot(); snapshot == nil || snapshot["value"].(float64) != 3 || snapshot["previous"].(float64) != 2 {
		t.Fatalf("rollingMovingAverageSnapshotState.snapshot() = %#v", snapshot)
	}
	if got := smaState.snapshotValue(); got != smaState {
		t.Fatalf("snapshotValue() = %#v, want state pointer", got)
	}
	if value, ok := smaState.PreferredScalarValue(); !ok || value != 3 {
		t.Fatalf("PreferredScalarValue() = (%v, %v)", value, ok)
	}
	if current, previous, currentOK, previousOK, ok := smaState.SeriesField("value"); !ok || !currentOK || !previousOK || current != 3 || previous != 2 {
		t.Fatalf("SeriesField(value) = (%v, %v, %v, %v, %v)", current, previous, currentOK, previousOK, ok)
	}
	if value, ok := smaState.FieldValue("value"); !ok || value.(float64) != 3 {
		t.Fatalf("FieldValue(value) = (%#v, %v)", value, ok)
	}
	if value, ok := smaState.FieldValue("previous"); !ok || value.(float64) != 2 {
		t.Fatalf("FieldValue(previous) = (%#v, %v)", value, ok)
	}
	if _, ok := smaState.FieldValue("missing"); ok {
		t.Fatal("FieldValue(missing) ok = true, want false")
	}

	vwmaState.push(1, 1)
	vwmaState.push(2, 2)
	vwmaState.push(4, 3)
	if value, ok := vwmaState.PreferredScalarValue(); !ok || math.Abs(value-(17.0/6.0)) > 1e-9 {
		t.Fatalf("VWMA PreferredScalarValue() = (%v, %v)", value, ok)
	}

	cache := newSnapshotSeriesCache()
	runtimeWithState := &indicatorRuntime{
		maStates:        maStates,
		emaStates:       map[movingAverageConfig]*rollingEMATailState{},
		intervalMinutes: 1,
	}
	if got := runtimeWithState.movingAverageSnapshot(smaConfig, cache); got != smaState {
		t.Fatalf("movingAverageSnapshot(state) = %#v, want state pointer", got)
	}
	runtimeFallback := &indicatorRuntime{
		closes:          []float64{1, 2, 3, 4},
		volumes:         []float64{1, 2, 3, 4},
		intervalMinutes: 1,
	}
	if got := runtimeFallback.movingAverageSnapshot(movingAverageConfig{averageType: "EMA", period: 3, source: "close"}, cache); got == nil {
		t.Fatal("movingAverageSnapshot(fallback) = nil, want snapshot")
	}

	seriesSnapshot := &indicatorSeriesSnapshot{current: 3, previous: 2, hasCurrent: true, hasPrevious: true}
	if value, ok := seriesSnapshot.PreferredScalarValue(); !ok || value != 3 {
		t.Fatalf("indicatorSeriesSnapshot.PreferredScalarValue() = (%v, %v)", value, ok)
	}
	if value, ok := seriesSnapshot.FieldValue("value"); !ok || value.(float64) != 3 {
		t.Fatalf("indicatorSeriesSnapshot.FieldValue(value) = (%#v, %v)", value, ok)
	}
	if value, ok := seriesSnapshot.FieldValue("previous"); !ok || value.(float64) != 2 {
		t.Fatalf("indicatorSeriesSnapshot.FieldValue(previous) = (%#v, %v)", value, ok)
	}

	macdSnapshot := &indicatorMACDSnapshot{diff: 3, signal: 2, histogram: 2, previousDiff: 2, previousSignal: 1.5, previousHistogram: 1, hasPrevious: true}
	if value, ok := macdSnapshot.PreferredScalarValue(); !ok || value != 3 {
		t.Fatalf("indicatorMACDSnapshot.PreferredScalarValue() = (%v, %v)", value, ok)
	}
	if current, previous, currentOK, previousOK, ok := macdSnapshot.SeriesField("histogram"); !ok || !currentOK || !previousOK || current != 2 || previous != 1 {
		t.Fatalf("indicatorMACDSnapshot.SeriesField(histogram) = (%v, %v, %v, %v, %v)", current, previous, currentOK, previousOK, ok)
	}
	if value, ok := macdSnapshot.FieldValue("previousSignal"); !ok || value.(float64) != 1.5 {
		t.Fatalf("indicatorMACDSnapshot.FieldValue(previousSignal) = (%#v, %v)", value, ok)
	}

	kdjSnapshot := &indicatorKDJSnapshot{k: 70, d: 60, j: 90, previousK: 65, previousD: 55, previousJ: 85, hasPrevious: true}
	if value, ok := kdjSnapshot.PreferredScalarValue(); !ok || value != 70 {
		t.Fatalf("indicatorKDJSnapshot.PreferredScalarValue() = (%v, %v)", value, ok)
	}
	if current, previous, currentOK, previousOK, ok := kdjSnapshot.SeriesField("k"); !ok || !currentOK || !previousOK || current != 70 || previous != 65 {
		t.Fatalf("indicatorKDJSnapshot.SeriesField(k) = (%v, %v, %v, %v, %v)", current, previous, currentOK, previousOK, ok)
	}
	if value, ok := kdjSnapshot.FieldValue("previousJ"); !ok || value.(float64) != 85 {
		t.Fatalf("indicatorKDJSnapshot.FieldValue(previousJ) = (%#v, %v)", value, ok)
	}
}

func TestTradingWindowMovingAverageSelectionBusinessBoundaries(t *testing.T) {
	values := []float64{10, 12, 14, 20, 22}
	volumes := []float64{100, 120, 140, 200, 220}
	labelKeys := []int64{20260612, 20260612, 20260615, 20260615, 20260616}

	current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(
		values, volumes, labelKeys, movingAverageConfig{averageType: "SMA", period: 2},
	)
	if !handled || !currentOK || !previousOK || math.Abs(current-18.6666666667) > 1e-9 || math.Abs(previous-14) > 1e-9 {
		t.Fatalf("SMA trading-window snapshot = current %v/%v previous %v/%v handled=%v", current, currentOK, previous, previousOK, handled)
	}

	current, previous, currentOK, previousOK, handled = calculateTradingWindowMovingAverageSnapshotFromKeys(
		values, volumes, labelKeys, movingAverageConfig{averageType: "VWMA", period: 2},
	)
	if !handled || !currentOK || !previousOK || current <= previous {
		t.Fatalf("VWMA trading-window snapshot = current %v/%v previous %v/%v handled=%v", current, currentOK, previous, previousOK, handled)
	}

	current, previous, currentOK, previousOK, handled = calculateTradingWindowMovingAverageSnapshotFromKeys(
		values, volumes, labelKeys, movingAverageConfig{averageType: "EMA", period: 2},
	)
	if !handled || !currentOK || !previousOK || current <= previous {
		t.Fatalf("EMA trading-window snapshot = current %v/%v previous %v/%v handled=%v", current, currentOK, previous, previousOK, handled)
	}

	for _, averageType := range []string{"SMMA", "HMA"} {
		t.Run(averageType, func(t *testing.T) {
			current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(
				[]float64{30, 40}, []float64{1, 1}, []int64{20260615, 20260616}, movingAverageConfig{averageType: averageType, period: 2},
			)
			if !handled || !currentOK || !previousOK || current == 0 || previous == 0 {
				t.Fatalf("%s trading-window snapshot = current %v/%v previous %v/%v handled=%v", averageType, current, currentOK, previous, previousOK, handled)
			}
		})
	}
	t.Run("TMA", func(t *testing.T) {
		current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(
			[]float64{30, 40}, []float64{1, 1}, []int64{20260615, 20260616}, movingAverageConfig{averageType: "TMA", period: 1},
		)
		if !handled || !currentOK || !previousOK || current != 40 || previous != 30 {
			t.Fatalf("TMA trading-window snapshot = current %v/%v previous %v/%v handled=%v", current, currentOK, previous, previousOK, handled)
		}
	})
	if _, _, _, _, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(values, volumes, labelKeys, movingAverageConfig{averageType: "UNKNOWN", period: 2}); !handled {
		t.Fatal("unknown average type should fall back to SMA-compatible trading-window aggregation")
	}
	if current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(values[:2], volumes[:2], labelKeys, movingAverageConfig{averageType: "SMA", period: 2}); !handled || currentOK || previousOK || current != 0 || previous != 0 {
		t.Fatalf("mismatched label keys snapshot = current %v/%v previous %v/%v handled=%v", current, currentOK, previous, previousOK, handled)
	}
	if current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys([]float64{10, 12}, []float64{100}, []int64{invalidTradingPeriodLabelKey, invalidTradingPeriodLabelKey}, movingAverageConfig{averageType: "VWMA", period: 2}); !handled || currentOK || previousOK || current != 0 || previous != 0 {
		t.Fatalf("invalid labels snapshot = current %v/%v previous %v/%v handled=%v", current, currentOK, previous, previousOK, handled)
	}

	summary := summarizeTradingWindowSelectionFromKeys(labelKeys, 2, len(labelKeys))
	if !summary.valid || summary.startKey != 20260615 || summary.startIndex != 2 || summary.endIndex != 4 || summary.count != 3 {
		t.Fatalf("selection summary = %+v", summary)
	}
	if summary := summarizeTradingWindowSelectionFromKeys(labelKeys, 0, len(labelKeys)); summary.valid {
		t.Fatalf("zero-period selection summary = %+v", summary)
	}
	if value, ok := calculateSingleValueFromTradingWindowSelection(values, labelKeys, tradingWindowSelectionSummary{valid: true, count: 2}); ok || value != 0 {
		t.Fatalf("single-value selection with two labels = %v/%v, want zero false", value, ok)
	}
}

func TestAdvancedIndicatorFormulaAndDivergenceBoundaries(t *testing.T) {
	values := []float64{10, 12, 14, 16, 18}
	if value, ok := calculateBollingerBandWidth(values, 5, 2); !ok || math.Abs(value-(4*math.Sqrt(8)/14)) > 1e-9 {
		t.Fatalf("calculateBollingerBandWidth() = %v/%v", value, ok)
	}
	if value, ok := calculateBollingerBandWidth([]float64{-1, 0, 1}, 3, 2); ok || value != 0 {
		t.Fatalf("zero-basis Bollinger width = %v/%v, want zero false", value, ok)
	}
	if value, ok := calculateBollingerBandWidth(values, 0, 2); ok || value != 0 {
		t.Fatalf("invalid Bollinger period = %v/%v, want zero false", value, ok)
	}

	if value, ok := calculateCenterOfGravity([]float64{1, 2, 3, 4}, 3); !ok || math.Abs(value+(16.0/9.0)) > 1e-9 {
		t.Fatalf("calculateCenterOfGravity() = %v/%v", value, ok)
	}
	if value, ok := calculateCenterOfGravity([]float64{-1, 0, 1}, 3); ok || value != 0 {
		t.Fatalf("zero-denominator COG = %v/%v, want zero false", value, ok)
	}

	if value, ok := calculateCMO([]float64{10, 12, 11, 15}, 3); !ok || math.Abs(value-(500.0/7.0)) > 1e-9 {
		t.Fatalf("calculateCMO() = %v/%v", value, ok)
	}
	if value, ok := calculateCMO([]float64{10, 10, 10, 10}, 3); !ok || value != 0 {
		t.Fatalf("flat CMO = %v/%v, want zero true", value, ok)
	}
	if value, ok := calculateCMO([]float64{10, 12}, 3); ok || value != 0 {
		t.Fatalf("insufficient CMO = %v/%v, want zero false", value, ok)
	}

	if value, ok := calculateTSI([]float64{10, 11, 12, 13}, 1, 1); !ok || value != 100 {
		t.Fatalf("rising TSI = %v/%v, want 100 true", value, ok)
	}
	if value, ok := calculateTSI([]float64{10, 10, 10}, 1, 1); !ok || value != 0 {
		t.Fatalf("flat TSI = %v/%v, want zero true", value, ok)
	}
	if value, ok := calculateTSI([]float64{10}, 1, 1); ok || value != 0 {
		t.Fatalf("insufficient TSI = %v/%v, want zero false", value, ok)
	}

	if value, ok := calculateCorrelation([]float64{1, 2, 3}, []float64{2, 4, 6}, 3); !ok || math.Abs(value-1) > 1e-9 {
		t.Fatalf("positive correlation = %v/%v", value, ok)
	}
	if value, ok := calculateCorrelation([]float64{1, 2, 3}, []float64{6, 4, 2}, 3); !ok || math.Abs(value+1) > 1e-9 {
		t.Fatalf("negative correlation = %v/%v", value, ok)
	}
	if value, ok := calculateCorrelation([]float64{1, 1, 1}, []float64{2, 3, 4}, 3); ok || value != 0 {
		t.Fatalf("constant correlation = %v/%v, want zero false", value, ok)
	}

	if value, ok := calculateMeanDeviation([]float64{1, 2, 4}, 3); !ok || math.Abs(value-(10.0/9.0)) > 1e-9 {
		t.Fatalf("calculateMeanDeviation() = %v/%v", value, ok)
	}
	if value, ok := calculateMedian([]float64{5, 1, 3}, 3); !ok || value != 3 {
		t.Fatalf("odd median = %v/%v", value, ok)
	}
	if value, ok := calculateMedian([]float64{5, 1, 3, 7}, 4); !ok || value != 4 {
		t.Fatalf("even median = %v/%v", value, ok)
	}
	if value, ok := calculatePercentileLinear([]float64{1, 3, 5, 7}, 4, 25); !ok || math.Abs(value-2.5) > 1e-9 {
		t.Fatalf("linear percentile = %v/%v", value, ok)
	}
	if value, ok := calculatePercentileLinear([]float64{7}, 1, 75); !ok || value != 7 {
		t.Fatalf("single-value linear percentile = %v/%v", value, ok)
	}
	if value, ok := calculatePercentileLinear([]float64{1, 2, 3}, 3, 101); ok || value != 0 {
		t.Fatalf("invalid linear percentile = %v/%v, want zero false", value, ok)
	}
	if value, ok := calculatePercentileNearest([]float64{1, 3, 5, 7}, 4, 75); !ok || value != 5 {
		t.Fatalf("nearest percentile = %v/%v", value, ok)
	}
	if value, ok := calculatePercentileNearest([]float64{1, 2, 3}, 3, -1); ok || value != 0 {
		t.Fatalf("invalid nearest percentile = %v/%v, want zero false", value, ok)
	}
	if value, ok := calculatePercentRank([]float64{1, 2, 4, 3}, 4); !ok || math.Abs(value-(200.0/3.0)) > 1e-9 {
		t.Fatalf("percent rank = %v/%v", value, ok)
	}
	if value, ok := calculatePercentRank([]float64{9}, 1); !ok || value != 0 {
		t.Fatalf("single-value percent rank = %v/%v", value, ok)
	}
	if value, ok := calculateSWMA([]float64{1, 2, 4, 8}); !ok || math.Abs(value-(21.0/6.0)) > 1e-9 {
		t.Fatalf("SWMA = %v/%v", value, ok)
	}
	if value, ok := calculateSWMA([]float64{1, 2, 3}); ok || value != 0 {
		t.Fatalf("insufficient SWMA = %v/%v, want zero false", value, ok)
	}

	current, previous, currentOK, previousOK := calculateOBVSnapshot([]float64{10, 12, 11, 11, 13}, []float64{100, 20, 5, 99, 7})
	if !currentOK || !previousOK || current != 22 || previous != 15 {
		t.Fatalf("OBV snapshot = current %v/%v previous %v/%v", current, currentOK, previous, previousOK)
	}
	if current, previous, currentOK, previousOK := calculateOBVSnapshot(nil, nil); currentOK || previousOK || current != 0 || previous != 0 {
		t.Fatalf("empty OBV snapshot = current %v/%v previous %v/%v", current, currentOK, previous, previousOK)
	}

	topState := &rollingMACDState{
		fast:              &rollingEMATailState{tail: []float64{1, 4, 3}},
		slow:              &rollingEMATailState{tail: []float64{0, 0, 0}},
		divergenceWindows: map[int]*rollingDivergenceWindowState{},
	}
	if !topState.detectDivergence([]float64{10, 11, 13}, "top", 2) {
		t.Fatal("MACD top divergence fallback = false, want true")
	}
	bottomState := &rollingMACDState{
		fast:              &rollingEMATailState{tail: []float64{1, 0, 2}},
		slow:              &rollingEMATailState{tail: []float64{0, 0, 0}},
		divergenceWindows: map[int]*rollingDivergenceWindowState{},
	}
	if !bottomState.detectDivergence([]float64{10, 9, 8}, "bottom", 2) {
		t.Fatal("MACD bottom divergence fallback = false, want true")
	}
	if topState.detectDivergence([]float64{10, 11, 13}, "sideways", 2) {
		t.Fatal("MACD divergence with unknown direction = true, want false")
	}
	if (&rollingMACDState{}).detectDivergence([]float64{10, 11, 13}, "top", 2) {
		t.Fatal("MACD divergence without EMA tails = true, want false")
	}
}
