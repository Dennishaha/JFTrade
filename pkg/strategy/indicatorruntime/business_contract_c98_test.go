package indicatorruntime

import (
	"math"
	"testing"
	"time"
)

func TestCoverage98TradingWindowAlgorithmsKeepSessionBoundariesAndFailureSignals(t *testing.T) {
	labels := []int64{10, 10, invalidTradingPeriodLabelKey, 11, 11}
	values := []float64{10, 12, 99, 20, 22}
	volumes := []float64{1, 1, 1, 2, 2}

	summary := summarizeTradingWindowSelectionFromKeys(labels, 2, len(labels))
	if !summary.valid || summary.count != 4 || summary.startIndex != 0 || summary.endIndex != 4 {
		t.Fatalf("session summary = %#v", summary)
	}
	if _, ok := calculateEMAFromTradingWindowSelection(values, labels, summary); !ok {
		t.Fatal("EMA must use every selected valid bar")
	}
	if current, ok := calculateSMMAFromTradingWindowSelection(values, labels, summary); !ok || current != 16 {
		t.Fatalf("SMMA = %v/%v, want 16/true", current, ok)
	}
	if _, ok := calculateSingleValueFromTradingWindowSelection(values, labels, summary); ok {
		t.Fatal("single-value selection must reject multi-bar trading windows")
	}
	if _, ok := calculateHMAFromTradingWindowSelection(values, labels, summary); ok {
		t.Fatal("HMA must reject a trading-window selection other than one or two bars")
	}

	oneLabel := []int64{20}
	if current, ok := calculateSingleValueFromTradingWindowSelection([]float64{7}, oneLabel, summarizeTradingWindowSelectionFromKeys(oneLabel, 1, 1)); !ok || current != 7 {
		t.Fatalf("single trading bar = %v/%v", current, ok)
	}
	twoLabels := []int64{21, 22}
	if current, ok := calculateHMAFromTradingWindowSelection([]float64{10, 20}, twoLabels, summarizeTradingWindowSelectionFromKeys(twoLabels, 2, 2)); !ok || current != 20+(20.0-10)/3 {
		t.Fatalf("two-bar HMA = %v/%v", current, ok)
	}

	for _, averageType := range []string{"EMA", "EXPMA", "SMMA"} {
		current, ok, handled := calculateTradingWindowSequenceValueFromKeys(values, labels, averageType, 2, len(values))
		if !handled || !ok || !isFiniteBusinessNumber(current) {
			t.Fatalf("%s sequence = %v/%v/%v", averageType, current, ok, handled)
		}
	}
	if current, ok, handled := calculateTradingWindowSequenceValueFromKeys([]float64{7}, []int64{30}, "TMA", 1, 1); !handled || !ok || current != 7 {
		t.Fatalf("TMA sequence = %v/%v/%v", current, ok, handled)
	}
	if current, ok, handled := calculateTradingWindowSequenceValueFromKeys([]float64{10, 20}, []int64{31, 32}, "HMA", 2, 2); !handled || !ok || current != 20+(20.0-10)/3 {
		t.Fatalf("HMA sequence = %v/%v/%v", current, ok, handled)
	}
	if _, ok, handled := calculateTradingWindowSequenceValueFromKeys(values, labels, "UNSUPPORTED", 2, len(values)); handled || ok {
		t.Fatal("unsupported sequence kind must request the materialized fallback")
	}
	if _, ok, handled := calculateTradingWindowSequenceValueFromKeys(values, labels, "EMA", 0, len(values)); !handled || ok {
		t.Fatal("zero trading-window period must remain handled but unavailable")
	}

	config := movingAverageConfig{averageType: "SMA", period: 2}
	current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(values, volumes, labels, config)
	if !handled || !currentOK || !previousOK || current != 16 || previous != 14 {
		t.Fatalf("SMA trading snapshot = %v/%v current=%v previous=%v", handled, currentOK, current, previous)
	}
	if _, _, currentOK, _, handled := calculateTradingWindowMovingAverageSnapshotFromKeys([]float64{1}, nil, []int64{1, 2}, config); !handled || currentOK {
		t.Fatal("misaligned trading-window input must be safely unavailable")
	}
}

func TestCoverage98TradingWindowOnlineAggregatorsRejectMissingVolumeWithoutChangingWindowSemantics(t *testing.T) {
	config := movingAverageConfig{averageType: "VWMA", period: 2}
	values := []float64{10, 20, 30}

	aggregator, handled := newTradingWindowMovingAverageAggregator(config)
	if !handled || !aggregator.push(10, []float64{2}, 0) || aggregator.push(20, []float64{2}, 1) {
		t.Fatalf("VWMA aggregator handling = %#v/%v", aggregator, handled)
	}
	if _, ok := aggregator.value(); ok {
		t.Fatal("VWMA must not report a value after an omitted volume")
	}
	if (*tradingWindowMovingAverageAggregator)(nil).push(1, nil, 0) {
		t.Fatal("nil aggregator accepted input")
	}

	for _, tc := range []struct {
		name   string
		config movingAverageConfig
		want   float64
	}{
		{name: "simple", config: movingAverageConfig{averageType: "SMA", period: 2, timeUnit: "day"}, want: 20},
		{name: "linear", config: movingAverageConfig{averageType: "LWMA", period: 2, timeUnit: "day"}, want: 70.0 / 3},
		{name: "volume", config: movingAverageConfig{averageType: "VWMA", period: 2, timeUnit: "day"}, want: 25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current, ok, handled := calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(
				values,
				[]float64{1, 1, 3},
				[]time.Time{
					time.Date(2026, time.January, 5, 15, 0, 0, 0, time.UTC),
					time.Date(2026, time.January, 5, 15, 1, 0, 0, time.UTC),
					time.Date(2026, time.January, 6, 15, 0, 0, 0, time.UTC),
				},
				tc.config,
				"US.AAPL",
				len(values),
				false,
				newSnapshotSeriesCache(),
			)
			if !handled || !ok || !isFiniteBusinessNumber(current) {
				t.Fatalf("online %s = %v/%v/%v", tc.name, current, ok, handled)
			}
		})
	}

	if _, ok, handled := calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(nil, nil, nil, config, "US.AAPL", 0, false, newSnapshotSeriesCache()); !handled || ok {
		t.Fatal("empty online window must be handled as unavailable")
	}
	if _, ok := calculateMovingAverageCurrentValueFromSelected(values, nil, nil, config, nil); ok {
		t.Fatal("an empty selection cannot produce a current value")
	}
	if current, ok := calculateMovingAverageCurrentValueFromSelected(values, nil, []int{0, 2}, movingAverageConfig{averageType: "unexpected"}, nil); !ok || current != 20 {
		t.Fatalf("fallback simple selected average = %v/%v", current, ok)
	}

	state := tradingWindowMovingAverageState{aggregator: tradingWindowMovingAverageAggregator{kind: "sma"}}
	state.push(2, 1, 10, nil, 0)
	state.push(2, 1, 20, nil, 1)
	state.push(2, 2, 30, nil, 2)
	state.push(2, 3, 40, nil, 3)
	if value, ok := state.value(); !ok || value != 20 || !state.done {
		t.Fatalf("window state = %#v value=%v/%v", state, value, ok)
	}
}

func TestCoverage98AdvancedIndicatorAndFormulaFailurePathsStayExplicit(t *testing.T) {
	if _, ok := calculateALMA([]float64{1, 2, 3}, 3, math.Inf(1), 6); ok {
		t.Fatal("an infinite ALMA offset must not manufacture a value")
	}
	if _, ok := calculateCorrelation([]float64{2, 2, 2}, []float64{3, 3, 3}, 3); ok {
		t.Fatal("constant series correlation is undefined")
	}
	if value, ok := calculateCMO([]float64{4, 4, 4}, 2); !ok || value != 0 {
		t.Fatalf("flat CMO = %v/%v", value, ok)
	}
	if value, ok := calculateTSI([]float64{4, 4, 4}, 1, 1); !ok || value != 0 {
		t.Fatalf("flat TSI = %v/%v", value, ok)
	}
	if _, ok := calculateCenterOfGravity([]float64{0, 0}, 2); ok {
		t.Fatal("zero center-of-gravity denominator must be unavailable")
	}
	if _, ok := calculateStdDevValue([]float64{1}, 2); ok {
		t.Fatal("insufficient standard-deviation data must be unavailable")
	}

	cache := newSnapshotSeriesCache()
	runtime := &indicatorRuntime{
		highs:  []float64{10, 11, 12},
		lows:   []float64{8, 9, 10},
		closes: []float64{9, 10, 11},
		endTimes: []time.Time{
			time.Date(2026, time.January, 5, 15, 0, 0, 0, time.UTC),
			time.Date(2026, time.January, 5, 15, 1, 0, 0, time.UTC),
			time.Date(2026, time.January, 5, 15, 2, 0, 0, time.UTC),
		},
	}
	if snapshot := runtime.advancedATRSnapshot(advancedIndicatorConfig{kind: "atr", key: "missing", timeUnit: "day", period: 2}, cache); snapshot != nil {
		t.Fatalf("incomplete higher-timeframe ATR snapshot = %#v", snapshot)
	}
	if snapshot := runtime.calculateKeltnerSnapshot(advancedIndicatorConfig{kind: "kc", source: "close", timeUnit: "bad", period: 2}); snapshot != nil {
		t.Fatalf("invalid higher-timeframe Keltner snapshot = %#v", snapshot)
	}
	if snapshot := calculateKDJSnapshot([]float64{1}, []float64{1, 2}, []float64{1}, kdjConfig{period: 1, m1: 1, m2: 1}); snapshot != nil {
		t.Fatalf("misaligned KDJ snapshot = %#v", snapshot)
	}
	if snapshot := calculateSupertrendSnapshot([]float64{1}, []float64{1}, []float64{1}, supertrendConfig{factor: 2, atrPeriod: 1}); snapshot != nil {
		t.Fatalf("insufficient supertrend snapshot = %#v", snapshot)
	}
	if _, _, _, ok := calculateDMIValues([]float64{1, 1, 1, 1}, []float64{1, 1, 1, 1}, []float64{1, 1, 1, 1}, dmiConfig{diLength: 1, adxSmoothing: 1}); !ok {
		t.Fatal("flat but complete DMI inputs must remain a valid zero-strength result")
	}
}

func isFiniteBusinessNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
