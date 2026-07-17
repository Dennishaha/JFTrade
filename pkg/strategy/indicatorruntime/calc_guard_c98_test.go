package indicatorruntime

import (
	"math"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestCoverage98CalculatorGuardsRejectUnwarmedAndMalformedSeries(t *testing.T) {
	if _, ok := simpleMovingAverage([]float64{1}, 2); ok {
		t.Fatal("SMA accepted fewer bars than its period")
	}
	if sequence := calculateTMASequence([]float64{1, 2}, 2); sequence != nil {
		t.Fatalf("TMA from a single intermediate SMA point = %#v", sequence)
	}
	if sequence := calculateHMASequence([]float64{1}, 2); sequence != nil {
		t.Fatalf("HMA accepted fewer bars than its period = %#v", sequence)
	}
	if _, ok := volumeWeightedMovingAverage([]float64{10, 20}, []float64{1}, 2); ok {
		t.Fatal("VWMA accepted a truncated volume series")
	}
	if _, ok := volumeWeightedMovingAverage([]float64{10, 20}, []float64{0, 0}, 2); ok {
		t.Fatal("VWMA reported a price without traded volume")
	}

	if snapshot := calculateBollingerSnapshot([]float64{10}, bollingerConfig{period: 2, multiplier: 2}); snapshot != nil {
		t.Fatalf("Bollinger snapshot accepted incomplete series = %#v", snapshot)
	}
	if _, ok := calculateCCIFromValues([]float64{10}, 2); ok {
		t.Fatal("CCI accepted fewer values than its period")
	}
	if value, ok := calculateCCIFromValues([]float64{10, 10}, 2); !ok || value != 0 {
		t.Fatalf("flat CCI = %v/%v, want 0/true", value, ok)
	}

	if snapshot := calculateKDJSnapshotFromSeries(kdjSeries{}); snapshot != nil {
		t.Fatalf("KDJ snapshot from an empty series = %#v", snapshot)
	}
	if series := calculateKDJSeriesWithBuffer(nil, []float64{1}, []float64{1, 2}, []float64{1}, kdjConfig{period: 1, m1: 1, m2: 1}); len(series.k) != 0 || len(series.d) != 0 || len(series.j) != 0 {
		t.Fatalf("KDJ accepted mismatched OHLC data = %#v", series)
	}
	if value := calculateKDJRSV(5, 5, 4); value != 50 {
		t.Fatalf("flat KDJ range RSV = %v, want 50", value)
	}

	if series := calculateMACDSeriesWithCache([]float64{1, 2, 3, 4}, macdConfig{fastPeriod: 0, slowPeriod: 2, signalPeriod: 2}, nil); len(series.diff) != 0 || len(series.signal) != 0 {
		t.Fatalf("MACD accepted a zero fast period = %#v", series)
	}
	if snapshot := calculateMACDSnapshotFromSeries(macdSeries{}); snapshot != nil {
		t.Fatalf("MACD snapshot from empty sequences = %#v", snapshot)
	}
	if value := calculateRSIFromSeries(nil); value != nil {
		t.Fatalf("RSI from an empty series = %#v", value)
	}
	if series := calculateRSISeries([]float64{1, 2}, 2); series != nil {
		t.Fatalf("RSI accepted no change window = %#v", series)
	}

	if _, _, currentOK, previousOK := calculateSARSnapshotValues(nil, nil, nil, sarConfig{start: 0.02, increment: 0.02, maximum: 0.2}); currentOK || previousOK {
		t.Fatal("SAR reported values for an empty OHLC series")
	}
	current, previous, currentOK, previousOK := calculateSARSnapshotValues(
		[]float64{11, 12}, []float64{9, 10}, []float64{10, 11}, sarConfig{start: 0.02, increment: 0.02, maximum: 0.2},
	)
	if !currentOK || previousOK || current == 0 || previous != 0 {
		t.Fatalf("one-step SAR snapshot = %v/%v/%v/%v", current, previous, currentOK, previousOK)
	}
	if snapshot := calculateSupertrendSnapshot([]float64{1, 2}, []float64{0, 1}, []float64{1, 2}, supertrendConfig{factor: 0, atrPeriod: 1}); snapshot != nil {
		t.Fatalf("supertrend accepted zero multiplier = %#v", snapshot)
	}

	if _, ok := calculateTSI([]float64{1}, 2, 2); ok {
		t.Fatal("TSI accepted a series without a price change")
	}
	if value, ok := calculateTSI([]float64{1, 2}, 2, 2); !ok || value != 100 {
		t.Fatalf("TSI must keep its EMA-seeded first change = %v/%v", value, ok)
	}
	if _, _, currentOK, previousOK := calculateOBVSnapshot(nil, nil); currentOK || previousOK {
		t.Fatal("OBV reported values for an empty price and volume series")
	}
	if value, ok := calculatePercentileLinear([]float64{7}, 1, 50); !ok || value != 7 {
		t.Fatalf("single-point linear percentile = %v/%v", value, ok)
	}
	if value, ok := calculatePercentRank([]float64{7}, 1); !ok || value != 0 {
		t.Fatalf("single-point percent rank = %v/%v", value, ok)
	}
}

func TestCoverage98VWAPAndRollingStateGuardContracts(t *testing.T) {
	at := time.Date(2026, time.January, 5, 15, 0, 0, 0, time.UTC)
	if _, ok := calculateSessionVWAP([]float64{10}, nil, []time.Time{at}, "US.AAPL", false); ok {
		t.Fatal("session VWAP accepted mismatched inputs")
	}
	if _, ok := calculateSessionVWAP([]float64{10}, []float64{0}, []time.Time{at}, "US.AAPL", false); ok {
		t.Fatal("session VWAP reported a value with no volume")
	}
	if _, ok := calculateAnchoredVWAP([]float64{10}, []float64{0}, []time.Time{at}, "week", "US.AAPL", false); ok {
		t.Fatal("anchored VWAP reported a value with no volume")
	}

	var nilWindow *rollingFloatWindow
	if _, ok := nilWindow.last(); ok {
		t.Fatal("nil rolling window reported a last value")
	}
	var window rollingFloatWindow
	if _, evicted := window.push(1, 0); evicted {
		t.Fatal("zero-capacity rolling window evicted a value")
	}
	window.ensureCapacity(0)
	if _, ok := window.at(-1); ok {
		t.Fatal("rolling window accepted a negative offset")
	}

	state := &rollingVWAPState{}
	state.push("", 10, 1)
	if _, ok := state.value(); ok {
		t.Fatal("cleared VWAP state reported a value")
	}
	state.push("2026-01-05", 10, 0)
	if _, ok := state.value(); ok {
		t.Fatal("zero-volume rolling VWAP reported a value")
	}
	state.push("2026-01-05", 20, 2)
	if value, ok := state.value(); !ok || value != 20 {
		t.Fatalf("rolling VWAP = %v/%v, want 20/true", value, ok)
	}

	windowState := &rollingWindowState{config: windowConfig{function: "roc", period: 1}}
	windowState.push(0)
	windowState.push(1)
	if _, ok := windowState.calculateRateOfChange(); ok {
		t.Fatal("ROC accepted a zero comparison base")
	}
	if _, ok := (&rollingWindowState{config: windowConfig{period: 2}}).calculateRange(); ok {
		t.Fatal("range accepted an unwarmed window")
	}
}

func TestCoverage98RollingIndicatorStateFailuresStayUnavailable(t *testing.T) {
	if state := newRollingEMATailState(0, 10, 2); state != nil {
		t.Fatalf("zero-period EMA tail state = %#v", state)
	}
	ema := newRollingEMATailState(2, 1, 1)
	if ema == nil || ema.powerAt(-1) != 0 {
		t.Fatalf("EMA tail state / negative power lookup = %#v", ema)
	}
	if value := (*rollingCCIState)(nil).value(); value != nil {
		t.Fatalf("nil CCI state value = %#v", value)
	}
	if value := (*rollingWilliamsRState)(nil).value(); value != nil {
		t.Fatalf("nil Williams %%R state value = %#v", value)
	}
	if snapshot := (*rollingBollingerState)(nil).snapshot(); snapshot != nil {
		t.Fatalf("nil Bollinger snapshot = %#v", snapshot)
	}
	if _, ok := (*rollingBollingerState)(nil).PreferredScalarValue(); ok {
		t.Fatal("nil Bollinger state reported a scalar value")
	}

	if states := newRollingDivergenceWindowStates([]int{0, -1}); states != nil {
		t.Fatalf("non-positive divergence lookbacks allocated states = %#v", states)
	}
	divergence := &rollingDivergenceWindowState{ready: true}
	if divergence.detect("unknown") {
		t.Fatal("unknown divergence direction was accepted")
	}
	if states := newRollingKDJStates(indicatorRequirements{kdj: []kdjConfig{{period: 0, m1: 1, m2: 1}}}, 10); states != nil {
		t.Fatalf("invalid KDJ config allocated states = %#v", states)
	}

	runtime := &indicatorRuntime{intervalMinutes: 1, closes: []float64{10}}
	if highs, lows, ok := runtime.fixedTimeframeHighLow("invalid"); ok || highs != nil || lows != nil {
		t.Fatalf("invalid higher timeframe high/low = %#v/%#v/%v", highs, lows, ok)
	}
	if snapshot := runtime.stochSnapshot(sourcePeriodConfig{source: "close", period: 2, timeUnit: "invalid"}, newSnapshotSeriesCache()); snapshot != nil {
		t.Fatalf("invalid higher timeframe stoch snapshot = %#v", snapshot)
	}
}

func TestCoverage98VWAPAndEngineWarmupKeepUnavailableValuesExplicit(t *testing.T) {
	at := time.Date(2026, time.January, 5, 15, 0, 0, 0, time.UTC)
	if _, ok := calculateSessionVWAP([]float64{10}, []float64{1}, []time.Time{at}, "UNKNOWN.SYMBOL", false); ok {
		t.Fatal("session VWAP accepted a symbol without a market calendar")
	}
	if _, ok := calculateAnchoredVWAP(nil, nil, nil, "day", "US.AAPL", false); ok {
		t.Fatal("anchored VWAP accepted an uninitialized feed")
	}
	if _, ok := calculateAnchoredVWAP([]float64{10}, []float64{1}, []time.Time{at}, "unsupported", "US.AAPL", false); ok {
		t.Fatal("anchored VWAP accepted an unsupported period")
	}

	requirements := indicatorRequirements{rsi: []int{2}}
	runtime := newIndicatorRuntimeWithRequirements(requirements, types.Interval1m, "US.AAPL", RuntimeOptions{})
	engine := &IndicatorEngine{runtime: runtime}
	snapshot := engine.Snapshot()
	if value, exists := snapshot["rsi:2"]; !exists || value != nil {
		t.Fatalf("unwarmed engine snapshot = %#v, want an explicit nil RSI value", snapshot)
	}
	borrowed := engine.SnapshotBorrowed()
	if value, exists := borrowed["rsi:2"]; !exists || value != nil {
		t.Fatalf("borrowed unwarmed RSI = %#v, want an explicit nil value", borrowed)
	}

	passthrough := map[string]any{"value": 10.0}
	if got := snapshotValueToMap(passthrough, [2]string{"value", "previous"}); got["value"] != 10.0 {
		t.Fatalf("map snapshot passthrough = %#v", got)
	}
}

func TestCoverage98RequirementAccountingAndIndicatorKeysStayDeterministic(t *testing.T) {
	requirements := indicatorRequirements{
		ma:             []movingAverageConfig{{averageType: "SMA", period: 2, timeUnit: "day"}},
		securitySource: []securitySourceConfig{{source: "close", timeUnit: "week", lookback: 1}},
		rsi:            []int{2},
		rsiSource:      []sourcePeriodConfig{{source: "close", period: 2, timeUnit: "day"}},
		macd:           []macdConfig{{fastPeriod: 1, slowPeriod: 2, signalPeriod: 1}},
		bollinger:      []bollingerConfig{{period: 2, multiplier: 2}},
		kdj:            []kdjConfig{{period: 2, m1: 1, m2: 1}},
		atr:            []int{2},
		stdev:          []int{2},
		stdevSource:    []sourcePeriodConfig{{source: "close", period: 2, timeUnit: "day"}},
		variance:       []sourcePeriodConfig{{source: "close", period: 2, timeUnit: "day"}},
		windows:        []windowConfig{{function: "roc", source: "close", period: 2}},
		cum:            []sourceConfig{{source: "close"}},
		stoch:          []sourcePeriodConfig{{source: "close", period: 2, timeUnit: "day"}},
		cci:            []int{2},
		cciSource:      []sourcePeriodConfig{{source: "close", period: 2, timeUnit: "day"}},
		williamsR:      []int{2},
		mfi:            []sourcePeriodConfig{{source: "close", period: 2, timeUnit: "day"}},
		dmi:            []dmiConfig{{diLength: 1, adxSmoothing: 1}},
		supertrend:     []supertrendConfig{{factor: 2, atrPeriod: 2}},
		sar:            []sarConfig{{start: 0.02, increment: 0.02, maximum: 0.2}},
		stopLoss:       []stopLossConfig{{mode: "stopLoss", timeValue: 1, timeUnit: "month", percentage: 1}},
		rsiDivergence:  []rsiDivergenceConfig{{period: 2, lookback: 2}},
		macdDivergence: []macdDivergenceConfig{{fastPeriod: 1, slowPeriod: 2, signalPeriod: 1, lookback: 2}},
		kdjDivergence:  []kdjDivergenceConfig{{period: 2, m1: 1, m2: 1, lookback: 2}},
		advanced:       []advancedIndicatorConfig{{kind: "linreg", period: 1, left: 1, right: 1, timeUnit: "month"}},
	}
	if got, want := calculateIndicatorWarmupBars(requirements, 1, "", false), 4*tradingSessionMinutesPerMonth; got != want {
		t.Fatalf("combined requirement warmup = %d, want %d", got, want)
	}
	if got := estimateTradingPeriodBars(0, "day", 1, "", false); got != 0 {
		t.Fatalf("zero-period warmup estimate = %d", got)
	}

	if got := sourceIndicatorKey("cum", sourceConfig{source: " Custom_Source "}); got != "cum:custom_source" {
		t.Fatalf("custom source key = %q", got)
	}
	if got := windowIndicatorKey(windowConfig{function: "sum", source: "Custom_Source", period: 2}); got != "sum:custom_source:2" {
		t.Fatalf("custom window key = %q", got)
	}
	if got := stochIndicatorKey(sourcePeriodConfig{source: "", period: 2, timeUnit: "day"}); got != "stoch:close:2:day" {
		t.Fatalf("default stoch key = %q", got)
	}

	windowConfigs := sortedWindowConfigs(map[windowConfig]struct{}{
		{function: "sum", source: "high", period: 2}:      {},
		{function: "highest", source: "close", period: 3}: {},
		{function: "sum", source: "close", period: 1}:     {},
	})
	if len(windowConfigs) != 3 || windowConfigs[0].function != "highest" || windowConfigs[1].source != "close" || windowConfigs[2].source != "high" {
		t.Fatalf("sorted window configs = %#v", windowConfigs)
	}
	sourcePeriodConfigs := sortedSourcePeriodConfigs(map[sourcePeriodConfig]struct{}{
		{source: "high", period: 2, timeUnit: "day"}:   {},
		{source: "close", period: 1, timeUnit: "week"}: {},
		{source: "close", period: 2, timeUnit: "day"}:  {},
	})
	if len(sourcePeriodConfigs) != 3 || sourcePeriodConfigs[0].period != 1 || sourcePeriodConfigs[1].source != "close" || sourcePeriodConfigs[2].source != "high" {
		t.Fatalf("sorted source-period configs = %#v", sourcePeriodConfigs)
	}
	movingConfigs := sortedMovingAverageConfigs(map[movingAverageConfig]struct{}{
		{averageType: "SMA", period: 2, timeUnit: "day", source: "high"}:   {},
		{averageType: "EMA", period: 2, timeUnit: "day", source: "close"}:  {},
		{averageType: "EMA", period: 1, timeUnit: "week", source: "close"}: {},
	})
	if len(movingConfigs) != 3 || movingConfigs[0].period != 1 || movingConfigs[1].averageType != "EMA" || movingConfigs[2].averageType != "SMA" {
		t.Fatalf("sorted moving-average configs = %#v", movingConfigs)
	}
}

func TestCoverage98RiskAndSnapshotHelpersRejectCorruptedData(t *testing.T) {
	trailing := stopLossConfig{mode: "trailingStop", direction: "long", timeValue: 2, percentage: 1}
	if snapshot := buildStopLossSnapshotForSymbolWithOptionsAndCache([]float64{10, math.Inf(1), 11}, nil, nil, trailing, 1, "", false, nil); snapshot != nil {
		t.Fatalf("trailing stop accepted an infinite intrawindow price = %#v", snapshot)
	}
	day := stopLossConfig{mode: "stopLoss", timeValue: 1, timeUnit: "day", percentage: 1}
	if snapshot := buildStopLossSnapshotForTradingWindowWithCache([]float64{10}, []time.Time{time.Date(2026, time.January, 5, 15, 0, 0, 0, time.UTC)}, nil, day, 1, "US.AAPL", false, nil); snapshot != nil {
		t.Fatalf("trading-window stop loss accepted a single selected bar = %#v", snapshot)
	}

	runtime := &indicatorRuntime{intervalMinutes: 1, closes: []float64{10}}
	if _, _, currentOK, previousOK := runtime.securitySourceSnapshotValues(securitySourceConfig{source: "close", timeUnit: "5m", lookback: 1}, nil); currentOK || previousOK {
		t.Fatal("higher-timeframe security source reported an unavailable lookback")
	}
	if _, ok := volumeWeightedMovingAverageFromSelected([]float64{10, 20}, []float64{1}, []int{1}); ok {
		t.Fatal("selected VWMA accepted an index outside the volume series")
	}
	if result := snapshotValueToMap(struct{}{}, [2]string{"value", "previous"}); result != nil {
		t.Fatalf("opaque snapshot converted to a map = %#v", result)
	}
}

func TestCoverage98RuntimeStateFallbacksDoNotInventValues(t *testing.T) {
	var nilRuntime *indicatorRuntime
	if _, ok := nilRuntime.atrSnapshotValue(2); ok {
		t.Fatal("nil ATR runtime reported a value")
	}
	if _, ok := nilRuntime.stdDevSnapshotValue(2); ok {
		t.Fatal("nil standard-deviation runtime reported a value")
	}
	if snapshot := nilRuntime.bollingerSnapshot(bollingerConfig{period: 2}); snapshot != nil {
		t.Fatalf("nil Bollinger runtime snapshot = %#v", snapshot)
	}
	var atr rollingATRState
	atr.push(10, 9, 9, 0, false)
	if _, ok := atr.currentValue(); ok {
		t.Fatal("zero-period ATR state reported a value")
	}
	var stdev rollingStdDevState
	stdev.push(10)
	if _, ok := stdev.currentValue(); ok {
		t.Fatal("zero-period standard-deviation state reported a value")
	}

	if states := newRollingCumStates(indicatorRequirements{cum: []sourceConfig{{source: "invalid"}}}); states != nil {
		t.Fatalf("invalid cumulative source allocated states = %#v", states)
	}
	var nilCum *rollingCumState
	nilCum.push(10)
	invalidState := &rollingCumState{}
	runtime := &indicatorRuntime{cumStates: map[sourceConfig]*rollingCumState{{source: "invalid"}: invalidState}}
	runtime.pushCumStates(1, 2, 0, 1, 10)
	if invalidState.hasCurrent {
		t.Fatal("unknown cumulative source mutated runtime state")
	}

	if states := newRollingMACDStates(indicatorRequirements{macd: []macdConfig{{fastPeriod: 0, slowPeriod: 2, signalPeriod: 1}}}, 5); states != nil {
		t.Fatalf("invalid MACD config allocated states = %#v", states)
	}
	if states := newRollingRSIStates(indicatorRequirements{}, 5); states != nil {
		t.Fatalf("empty RSI requirements allocated states = %#v", states)
	}
	if series := nilRuntime.rsiSeries(2); series != nil {
		t.Fatalf("nil RSI runtime series = %#v", series)
	}
}
