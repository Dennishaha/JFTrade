package indicatorruntime

import "testing"

func TestCoverage98SnapshotAssemblyOmitsIncompleteDataAndPreservesLegacyMAKey(t *testing.T) {
	incomplete := movingAverageConfig{averageType: "SMA", period: 2, source: "close"}
	incompleteRuntime := &indicatorRuntime{
		requirements: indicatorRequirements{ma: []movingAverageConfig{incomplete}},
		maStates:     map[movingAverageConfig]*rollingMovingAverageSnapshotState{incomplete: {kind: "SMA", period: 2}},
		snapshotKeys: buildSnapshotKeyCache(indicatorRequirements{ma: []movingAverageConfig{incomplete}}),
	}
	incompleteCache, incompleteResult := incompleteRuntime.prepareSnapshotState()
	incompleteRuntime.appendMovingAverageSnapshots(incompleteResult, incompleteCache)
	if len(incompleteResult) != 0 {
		t.Fatalf("incomplete moving average became a strategy value: %#v", incompleteResult)
	}

	legacy := movingAverageConfig{averageType: "MA", period: 2, source: "close"}
	legacyRequirements := indicatorRequirements{ma: []movingAverageConfig{legacy}}
	legacyState := &rollingMovingAverageSnapshotState{kind: "MA", period: 2}
	legacyState.push(10, 1)
	legacyState.push(14, 1)
	legacyRuntime := &indicatorRuntime{
		requirements: legacyRequirements,
		maStates:     map[movingAverageConfig]*rollingMovingAverageSnapshotState{legacy: legacyState},
		snapshotKeys: buildSnapshotKeyCache(legacyRequirements),
	}
	cache, result := legacyRuntime.prepareSnapshotState()
	legacyRuntime.appendMovingAverageSnapshots(result, cache)
	modernKey := legacyRuntime.snapshotKeys.ma[legacy]
	legacyKey := legacyRuntime.snapshotKeys.maLegacy[legacy]
	if result[modernKey] == nil || result[legacyKey] == nil {
		t.Fatalf("legacy MA compatibility snapshot = %#v", result)
	}
	if result[modernKey] != result[legacyKey] {
		t.Fatalf("modern and legacy MA keys must expose the same snapshot: %#v", result)
	}
}

func TestCoverage98HistoricalDivergenceFallbackMatchesStoredSeries(t *testing.T) {
	rsiDivergence := rsiDivergenceConfig{period: 2, direction: "top", lookback: 2}
	macdDivergence := macdDivergenceConfig{fastPeriod: 1, slowPeriod: 2, signalPeriod: 1, direction: "bottom", lookback: 2}
	kdjDivergence := kdjDivergenceConfig{period: 2, m1: 2, m2: 2, direction: "top", lookback: 2}
	requirements := indicatorRequirements{
		rsiDivergence:  []rsiDivergenceConfig{rsiDivergence},
		macdDivergence: []macdDivergenceConfig{macdDivergence},
		kdjDivergence:  []kdjDivergenceConfig{kdjDivergence},
	}
	runtime := &indicatorRuntime{
		requirements: requirements,
		snapshotKeys: buildSnapshotKeyCache(requirements),
		highs:        []float64{12, 15, 14, 16, 13},
		lows:         []float64{8, 10, 9, 11, 7},
		closes:       []float64{10, 14, 11, 15, 9},
	}
	result := map[string]any{}
	cache := newSnapshotSeriesCache()
	runtime.appendDivergenceSnapshots(result, cache)

	wantRSI := detectRSIDivergence(runtime.closes, runtime.rsiSeries(rsiDivergence.period), rsiDivergence.direction, rsiDivergence.lookback)
	if got, ok := result[runtime.snapshotKeys.rsiDivergence[rsiDivergence]].(bool); !ok || got != wantRSI {
		t.Fatalf("rehydrated RSI divergence = %#v, want %v", result, wantRSI)
	}
	baseMACD := macdConfig{fastPeriod: macdDivergence.fastPeriod, slowPeriod: macdDivergence.slowPeriod, signalPeriod: macdDivergence.signalPeriod}
	wantMACD := detectMACDDivergence(runtime.closes, cache.getMACDSeries(runtime.closes, baseMACD).diff, macdDivergence.direction, macdDivergence.lookback)
	if got, ok := result[runtime.snapshotKeys.macdDivergence[macdDivergence]].(bool); !ok || got != wantMACD {
		t.Fatalf("rehydrated MACD divergence = %#v, want %v", result, wantMACD)
	}
	baseKDJ := kdjConfig{period: kdjDivergence.period, m1: kdjDivergence.m1, m2: kdjDivergence.m2}
	wantKDJ := detectKDJDivergence(runtime.closes, cache.getKDJSeries(runtime.highs, runtime.lows, runtime.closes, baseKDJ).j, kdjDivergence.direction, kdjDivergence.lookback)
	if got, ok := result[runtime.snapshotKeys.kdjDivergence[kdjDivergence]].(bool); !ok || got != wantKDJ {
		t.Fatalf("rehydrated KDJ divergence = %#v, want %v", result, wantKDJ)
	}
}

func TestCoverage98RollingStatesRecoverWithoutProducingInventedSignals(t *testing.T) {
	if state := newRollingEMATailState(2, 0, 2); state == nil || state.limit != minimumIndicatorSeriesLimit {
		t.Fatalf("EMA default retention = %#v", state)
	}

	recoveredEMA := newRollingEMATailState(2, 4, 2)
	recoveredEMA.windowLen = 2
	recoveredEMA.tail = recoveredEMA.tail[:0]
	recoveredEMA.push(20, true, 10, 12, true, true)
	if recoveredEMA.windowLen != 2 || len(recoveredEMA.tail) != 1 || recoveredEMA.tail[0] != 20 {
		t.Fatalf("EMA recovery from a lost retained tail = %#v", recoveredEMA)
	}
	var nilEMA *rollingEMATailState
	nilEMA.appendValue(1)

	var nilRuntime *indicatorRuntime
	if snapshot := nilRuntime.stochSnapshot(sourcePeriodConfig{source: "close", period: 2}, newSnapshotSeriesCache()); snapshot != nil {
		t.Fatalf("nil runtime stochastic snapshot = %#v", snapshot)
	}

	var nilKDJ *rollingKDJState
	nilKDJ.trimState(nil, nil, nil)
	nilKDJ.appendValues(1, 2, 3)
	nilKDJ.pushDivergenceSample(10)
	state := newRollingKDJState(kdjConfig{period: 2, m1: 2, m2: 2}, 4, nil)
	state.pushDivergenceSample(10)
	if len(state.closeTail) != 0 {
		t.Fatalf("KDJ without requested divergence lookbacks retained a phantom price: %#v", state.closeTail)
	}
}

func TestCoverage98AdvancedSnapshotsRejectUnavailableOrInsufficientSeries(t *testing.T) {
	if value, ok := calculateCorrelation([]float64{1}, []float64{2}, 2); ok || value != 0 {
		t.Fatalf("short correlation = (%v, %v)", value, ok)
	}

	// A partially restored fixed-timeframe runtime can have a close series before
	// its high/low arrays. ATR must remain absent rather than assert a number.
	runtime := &indicatorRuntime{
		intervalMinutes: 1,
		closes:          []float64{10},
	}
	if snapshot := runtime.advancedATRSnapshot(advancedIndicatorConfig{kind: "atr", key: "atr", timeUnit: "1m", period: 2}, newSnapshotSeriesCache()); snapshot != nil {
		t.Fatalf("incomplete fixed-timeframe ATR snapshot = %#v", snapshot)
	}
}
