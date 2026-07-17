package indicatorruntime

import (
	"testing"
	"time"
)

type coverage95SnapshotReader struct {
	values map[string]any
}

func (r coverage95SnapshotReader) FieldValue(key string) (any, bool) {
	value, ok := r.values[key]
	return value, ok
}

func TestCoverage95SnapshotPreparationAndFallbackFamilies(t *testing.T) {
	runtime := &indicatorRuntime{}
	cache, result := runtime.prepareSnapshotState()
	if cache == nil || result == nil || runtime.snapshotCache != cache || runtime.snapshotResult == nil {
		t.Fatalf("prepared snapshot state = cache:%#v result:%#v", cache, result)
	}
	result["stale"] = true
	cache, result = runtime.prepareSnapshotState()
	if len(result) != 0 || cache != runtime.snapshotCache {
		t.Fatalf("prepared snapshot state did not reset reusable values: cache:%#v result:%#v", cache, result)
	}

	requirements := indicatorRequirements{
		ma:             []movingAverageConfig{{averageType: "SMA", period: 2, source: "close"}},
		securitySource: []securitySourceConfig{{source: "close"}},
		rsi:            []int{2},
		rsiSource:      []sourcePeriodConfig{{source: "high", period: 2}},
		macd:           []macdConfig{{fastPeriod: 1, slowPeriod: 2, signalPeriod: 1}},
		bollinger:      []bollingerConfig{{period: 2, multiplier: 2}},
		kdj:            []kdjConfig{{period: 2, m1: 2, m2: 2}},
		atr:            []int{2},
		stdev:          []int{2},
		stdevSource:    []sourcePeriodConfig{{source: "high", period: 2}},
		variance:       []sourcePeriodConfig{{source: "low", period: 2}},
		windows: []windowConfig{
			{function: "sum", source: "close", period: 2},
			{function: "rising", source: "close", period: 2},
		},
		cum:        []sourceConfig{{source: "volume"}},
		stoch:      []sourcePeriodConfig{{source: "close", period: 2}},
		cci:        []int{2},
		cciSource:  []sourcePeriodConfig{{source: "close", period: 2}},
		williamsR:  []int{2},
		mfi:        []sourcePeriodConfig{{source: "hlc3", period: 2}},
		dmi:        []dmiConfig{{diLength: 2, adxSmoothing: 2}},
		supertrend: []supertrendConfig{{factor: 2, atrPeriod: 2}},
		sar:        []sarConfig{{start: 0.02, increment: 0.02, maximum: 0.2}},
	}
	runtime.requirements = requirements
	runtime.snapshotKeys = buildSnapshotKeyCache(requirements)
	runtime.opens = []float64{10, 11, 12, 13}
	runtime.highs = []float64{12, 13, 14, 15}
	runtime.lows = []float64{8, 9, 10, 11}
	runtime.closes = []float64{10, 12, 11, 13}
	runtime.volumes = []float64{100, 120, 110, 130}
	runtime.windowStates = map[windowConfig]*rollingWindowState{
		{function: "sum", source: "close", period: 2}:    {current: 24, previous: 23, hasCurrent: true, hasPrevious: true},
		{function: "rising", source: "close", period: 2}: {boolCurrent: true, hasCurrent: true},
	}
	runtime.cumStates = map[sourceConfig]*rollingCumState{
		{source: "volume"}: {current: 460, previous: 330, hasCurrent: true, hasPrevious: true},
	}
	cache, result = runtime.prepareSnapshotState()
	runtime.appendMovingAverageSnapshots(result, cache)
	runtime.appendSecuritySourceSnapshots(result, cache)
	runtime.appendRSISnapshots(result, cache)
	runtime.appendMACDSnapshots(result, cache)
	runtime.appendBollingerSnapshots(result)
	runtime.appendKDJSnapshots(result, cache)
	runtime.appendATRSnapshots(result, cache)
	runtime.appendStdDevSnapshots(result, cache)
	runtime.appendVarianceSnapshots(result, cache)
	runtime.appendWindowSnapshots(result, cache)
	runtime.appendCumSnapshots(result, cache)
	runtime.appendStochSnapshots(result, cache)
	runtime.appendCCISnapshots(result, cache)
	runtime.appendWilliamsRSnapshots(result, cache)
	runtime.appendMFISnapshots(result, cache)
	runtime.appendDMISnapshots(result)
	runtime.appendSupertrendSnapshots(result)
	runtime.appendSARSnapshots(result, cache)
	if len(result) < 10 {
		t.Fatalf("fallback snapshot families omitted expected values: %#v", result)
	}
	if result[runtime.snapshotKeys.windows[windowConfig{function: "rising", source: "close", period: 2}]] != true {
		t.Fatalf("rising window snapshot = %#v", result)
	}
	if snapshotValueToMap(coverage95SnapshotReader{values: map[string]any{"value": 3.0}}, [...]string{"value", "previous"})["value"] != 3.0 {
		t.Fatal("field reader snapshot was not materialized")
	}
	if snapshotValueToMap(12, [...]string{"value", "previous"}) != nil {
		t.Fatal("non-reader snapshot was materialized")
	}
}

func TestCoverage95TradingWindowAndStopLossFallbackPaths(t *testing.T) {
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.June, 1, 15, 0, 0, 0, time.UTC),
	}
	values := []float64{10, 12, 11}
	volumes := []float64{1, 2, 3}
	cache := newSnapshotSeriesCache()
	config := movingAverageConfig{averageType: "VWMA", period: 2, timeUnit: "day"}

	if snapshot := buildMovingAverageSnapshotForTradingWindow(nil, nil, endTimes, config, "US.AAPL", false, cache); snapshot != nil {
		t.Fatalf("empty online trading-window snapshot = %#v", snapshot)
	}
	if current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotOnlineWithCache(values, volumes[:1], endTimes, config, "US.AAPL", false, cache); !handled || currentOK || previousOK || current != 0 || previous != 0 {
		t.Fatalf("mismatched online snapshot = (%v, %v, %v, %v, %v)", current, previous, currentOK, previousOK, handled)
	}
	if current, ok := calculateMovingAverageCurrentValue(nil, nil, config); ok || current != 0 {
		t.Fatalf("empty moving-average current value = (%v, %v)", current, ok)
	}
	if current, ok, handled := calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(values, volumes[:1], endTimes, config, "US.AAPL", len(values), false, cache); !handled || ok || current != 0 {
		t.Fatalf("mismatched online current value = (%v, %v, %v)", current, ok, handled)
	}

	sequenceConfig := movingAverageConfig{averageType: "EMA", period: 2, timeUnit: "day"}
	labels := cache.getTradingPeriodLabels(endTimes, "US.AAPL", "day", false)
	current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(values, volumes, labels, sequenceConfig)
	if !handled || !currentOK || !previousOK || current == 0 || previous == 0 {
		t.Fatalf("sequence snapshot from labels = (%v, %v, %v, %v, %v)", current, previous, currentOK, previousOK, handled)
	}
	if current, ok := calculateTradingWindowMovingAverageCurrentValue(values, volumes, nil, config, "US.AAPL", len(values), false, cache); ok || current != 0 {
		t.Fatalf("current value without timestamps = (%v, %v)", current, ok)
	}

	state := &tradingWindowMovingAverageState{aggregator: tradingWindowMovingAverageAggregator{kind: "vwma"}}
	state.push(1, 1, 10, nil, 0)
	if !state.done {
		t.Fatalf("missing-volume window state did not stop: %#v", state)
	}
	state.push(1, 0, 20, []float64{1}, 0)
	if _, ok := state.value(); ok {
		t.Fatalf("missing-volume state unexpectedly has a value: %#v", state)
	}
	var nilState *tradingWindowMovingAverageState
	nilState.push(1, 1, 10, nil, 0)

	stopLoss := stopLossConfig{mode: "takeProfit", direction: "short", timeValue: 2, timeUnit: "day", percentage: 5, windowPolicy: "session"}
	snapshot := buildStopLossSnapshotForTradingWindowWithCache([]float64{12, 10, 8}, endTimes, nil, stopLoss, 1, "US.AAPL", false, cache)
	if snapshot == nil || snapshot["direction"] != "short" || snapshot["triggered"] != true || snapshot["windowPolicy"] != "session" {
		t.Fatalf("trading-window short take-profit snapshot = %#v", snapshot)
	}
	first := selectStopLossTradingWindowIndicesWithCache(endTimes, 2, "day", "US.AAPL", len(endTimes), false, cache)
	second := selectStopLossTradingWindowIndicesWithCache(endTimes, 2, "day", "US.AAPL", len(endTimes), false, cache)
	if len(first) < 2 || len(second) != len(first) || &first[0] != &second[0] {
		t.Fatalf("stop-loss cached selection was not reused: %#v/%#v", first, second)
	}
	if snapshot := buildStopLossSnapshotForTradingWindowWithCache([]float64{10, 0, 8}, endTimes, nil, stopLoss, 1, "US.AAPL", false, cache); snapshot != nil {
		t.Fatalf("invalid trading-window stop-loss snapshot = %#v", snapshot)
	}
}

func TestCoverage95AdvancedRuntimeFailureAndKeltnerCases(t *testing.T) {
	runtime := &indicatorRuntime{
		intervalMinutes: 1,
		opens:           []float64{10, 11, 12, 13},
		highs:           []float64{12, 14, 13, 15},
		lows:            []float64{8, 9, 10, 11},
		closes:          []float64{10, 12, 11, 13},
		volumes:         []float64{100, 110, 120, 130},
	}
	cache := newSnapshotSeriesCache()
	if snapshot := runtime.advancedIndicatorSnapshot(advancedIndicatorConfig{kind: "unsupported", source: "close"}, cache); snapshot != nil {
		t.Fatalf("unsupported advanced snapshot = %#v", snapshot)
	}
	if snapshot := runtime.advancedSupertrendSnapshot(advancedIndicatorConfig{kind: "supertrend", timeUnit: "day", period: 2, multiplier: 2}); snapshot != nil {
		t.Fatalf("missing fixed-timeframe supertrend = %#v", snapshot)
	}
	if snapshot := runtime.advancedATRSnapshot(advancedIndicatorConfig{kind: "atr", timeUnit: "day", period: 2}, cache); snapshot != nil {
		t.Fatalf("missing fixed-timeframe ATR = %#v", snapshot)
	}
	if snapshot := runtime.advancedCorrelationSnapshot(advancedIndicatorConfig{kind: "correlation", source: "close", source2: "high", timeUnit: "bad", period: 2}, runtime.closes, cache); snapshot != nil {
		t.Fatalf("invalid-timeframe correlation = %#v", snapshot)
	}
	if snapshot := runtime.advancedOBVSnapshot(advancedIndicatorConfig{kind: "obv", source: "close", timeUnit: "bad"}, runtime.closes, cache); snapshot != nil {
		t.Fatalf("invalid-timeframe OBV = %#v", snapshot)
	}
	if highs, lows, closes, ok := runtime.fixedTimeframeOHLC("bad"); ok || highs != nil || lows != nil || closes != nil {
		t.Fatalf("invalid fixed-timeframe OHLC = %#v/%#v/%#v/%v", highs, lows, closes, ok)
	}
	if snapshot := runtime.calculateKeltnerSnapshot(advancedIndicatorConfig{kind: "kc", source: "close", period: 0}); snapshot != nil {
		t.Fatalf("invalid-period Keltner snapshot = %#v", snapshot)
	}
	for _, useTR := range []bool{false, true} {
		snapshot := runtime.calculateKeltnerSnapshot(advancedIndicatorConfig{kind: "kc", source: "close", period: 2, multiplier: 1.5, useTR: useTR})
		if snapshot == nil || snapshot["upper"] == nil || snapshot["lower"] == nil || snapshot["width"] == nil {
			t.Fatalf("Keltner snapshot useTR=%v = %#v", useTR, snapshot)
		}
	}
	obvConfig := advancedIndicatorConfig{kind: "obv", source: "close"}
	states := newOBVStates(indicatorRequirements{advanced: []advancedIndicatorConfig{obvConfig, {kind: "obv", source: "close", timeUnit: "day"}}})
	if len(states) != 1 {
		t.Fatalf("OBV states = %#v", states)
	}
	badConfig := advancedIndicatorConfig{kind: "obv", source: "unsupported"}
	runtime.obvStates = map[advancedIndicatorConfig]*rollingCumState{badConfig: {}}
	runtime.pushOBVStates(1, 2, 0, 1, 10, map[string]float64{"close": 1}, true)
	if runtime.obvStates[badConfig].hasCurrent {
		t.Fatalf("invalid OBV source unexpectedly changed state: %#v", runtime.obvStates[badConfig])
	}
}
