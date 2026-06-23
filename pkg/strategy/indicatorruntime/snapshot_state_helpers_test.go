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
