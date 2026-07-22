package indicatorruntime

import (
	"math"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestCoverage98TradingWindowMathPreservesFailureSemantics(t *testing.T) {
	values := []float64{10, 12, 15}
	volumes := []float64{2, 3, 4}
	labels := []int64{1, invalidTradingPeriodLabelKey, 2}

	if current, ok := calculateTradingWindowMovingAverageCurrentValue(values, volumes, nil, movingAverageConfig{
		averageType: "SMA", period: 2, timeUnit: "day",
	}, "US.AAPL", len(values), false, newSnapshotSeriesCache()); ok || current != 0 {
		t.Fatalf("missing trading-window timestamps = (%v, %v)", current, ok)
	}

	if current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(
		values,
		volumes,
		labels,
		movingAverageConfig{averageType: "VWMA", period: 2},
	); !handled || !currentOK || !previousOK || current <= previous || previous <= 0 {
		t.Fatalf("VWMA snapshot across completed trading windows = (%v, %v, %v, %v, %v)", current, previous, currentOK, previousOK, handled)
	}

	if current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(
		values,
		volumes,
		labels,
		movingAverageConfig{averageType: "EMA", period: 2},
	); !handled || !currentOK || !previousOK || current <= previous || previous <= 0 {
		t.Fatalf("EMA snapshot across completed trading windows = (%v, %v, %v, %v, %v)", current, previous, currentOK, previousOK, handled)
	}

	invalidSummary := tradingWindowSelectionSummary{
		startKey: 1, startIndex: 0, endIndex: 1, count: 3, valid: true,
	}
	if value, ok := calculateHMAFromTradingWindowSelection(values, labels, invalidSummary); ok || value != 0 {
		t.Fatalf("HMA with an invalid selected count = (%v, %v)", value, ok)
	}

	var nilAggregator *tradingWindowMovingAverageAggregator
	if nilAggregator.push(10, nil, 0) {
		t.Fatal("nil trading-window aggregator accepted a value")
	}
	unknownAggregator := tradingWindowMovingAverageAggregator{kind: "unknown"}
	if unknownAggregator.push(10, nil, 0) {
		t.Fatal("unknown trading-window aggregator accepted a value")
	}
	if value, ok := unknownAggregator.value(); ok || value != 0 {
		t.Fatalf("unknown trading-window aggregator value = (%v, %v)", value, ok)
	}
	missingVolume := tradingWindowMovingAverageAggregator{kind: "vwma"}
	if missingVolume.push(10, nil, 0) {
		t.Fatal("VWMA aggregator accepted missing volume")
	}
	if value, ok := missingVolume.value(); ok || value != 0 {
		t.Fatalf("missing-volume VWMA value = (%v, %v)", value, ok)
	}

	state := &tradingWindowMovingAverageState{aggregator: tradingWindowMovingAverageAggregator{kind: "sma"}}
	state.push(0, 1, 10, nil, 0)
	if !state.done {
		t.Fatalf("zero-period trading-window state should stop: %#v", state)
	}
}

func TestCoverage98IndicatorConfigValidationKeepsInvalidBusinessInputOut(t *testing.T) {
	if _, ok := parseMovingAverageConfig([]string{"ma"}); ok {
		t.Fatal("one-part moving average config was accepted")
	}
	if _, ok := parseMovingAverageConfig([]string{"ma", "ema", "0", "day"}); ok {
		t.Fatal("zero-period moving average config was accepted")
	}
	if _, ok := parseMovingAverageConfig([]string{"ma", "ema", "5", "day", "unknown"}); ok {
		t.Fatal("unknown moving-average source was accepted")
	}
	if _, ok := parseDefaultMovingAverageTail("EMA", 5, "not-a-timeframe"); ok {
		t.Fatal("invalid moving-average tail was accepted")
	}
	if source := normalizeSourceOrClose("not-a-source"); source != "close" {
		t.Fatalf("invalid source normalized to %q, want close", source)
	}
	if source, ok := parseOHLCVSource(" OHLC4 "); !ok || source != "ohlc4" {
		t.Fatalf("OHLC4 source parse = (%q, %v)", source, ok)
	}

	for _, parts := range [][]string{
		{"sl", "long", "0", "day", "3"},
		{"sl", "long", "2", "bad", "3"},
		{"risk", "not-a-mode", "long", "2", "day", "3", "continuous"},
		{"risk", "stopLoss", "short", "2", "day", "0", "continuous"},
		{"risk", "stopLoss", "short", "2", "day", "3", "bad"},
	} {
		if _, ok := parseStopLossConfig(parts); ok {
			t.Fatalf("invalid stop-loss config was accepted: %#v", parts)
		}
	}
	if unit, ok := parseStopLossTimeUnit(`"bars"`); !ok || unit != "" {
		t.Fatalf("quoted bars stop-loss unit = (%q, %v)", unit, ok)
	}
	if _, ok := parseStopLossTimeUnit("years"); ok {
		t.Fatal("unsupported stop-loss time unit was accepted")
	}

	if got := resolveBarCount(2, "day", 0); got != 2*tradingSessionMinutesPerDay {
		t.Fatalf("day bar count with zero interval = %d", got)
	}
	if got := resolveBarCount(2, "7m", 5); got != 3 {
		t.Fatalf("seven-minute bar count at five-minute interval = %d", got)
	}
	if minutes, ok := indicatorTimeUnitMinutes("invalid"); ok || minutes != 0 {
		t.Fatalf("invalid indicator unit = (%d, %v)", minutes, ok)
	}
	if unit, ok := parseIndicatorTimeUnit("invalid"); ok || unit != "" {
		t.Fatalf("invalid indicator timeframe = (%q, %v)", unit, ok)
	}
	if normalized := normalizeIndicatorTimeUnit(`"15m"`); normalized != "15m" {
		t.Fatalf("quoted 15m normalization = %q", normalized)
	}
}

func TestCoverage98AlgorithmBoundariesRemainSafeForRiskAndAdvancedIndicators(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   float64
	}{
		{source: "open", want: 10},
		{source: "high", want: 14},
		{source: "low", want: 8},
		{source: "close", want: 12},
		{source: "volume", want: 100},
		{source: "hl2", want: 11},
		{source: "hlc3", want: (14 + 8 + 12) / 3.0},
		{source: "ohlc4", want: 11},
	} {
		got, ok := ohlcvSourceValue(tc.source, 10, 14, 8, 12, 100)
		if !ok || math.Abs(got-tc.want) > 1e-9 {
			t.Fatalf("source %s = (%v, %v), want (%v, true)", tc.source, got, ok, tc.want)
		}
	}
	if value, ok := ohlcvSourceValue("unknown", 1, 2, 0, 1, 10); ok || value != 0 {
		t.Fatalf("unknown OHLCV source = (%v, %v)", value, ok)
	}

	if invalidStopLossPrice(10) || !invalidStopLossPrice(0) || !invalidStopLossPrice(math.NaN()) || !invalidStopLossPrice(math.Inf(1)) {
		t.Fatal("stop-loss price validity did not reject non-tradable values")
	}
	if got := stopLossTriggerPercent("long", true, false, 4, 9); got != 4 {
		t.Fatalf("long trigger percent = %v", got)
	}
	if got := stopLossTriggerPercent("short", false, true, 4, 9); got != 9 {
		t.Fatalf("short trigger percent = %v", got)
	}
	if got := stopLossTriggerPercent("auto", true, true, 4, 9); got != 9 {
		t.Fatalf("auto trigger percent = %v", got)
	}
	if _, _, ok := stopLossWindowStart([]float64{10}, nil, nil, stopLossConfig{timeValue: 1}, 1, nil); ok {
		t.Fatal("stop-loss window accepted insufficient bars")
	}

	if value, ok := calculateBollingerBandWidth([]float64{0, 0}, 2, 2); ok || value != 0 {
		t.Fatalf("zero-basis Bollinger bandwidth = (%v, %v)", value, ok)
	}
	if value, ok := calculateCenterOfGravity([]float64{0, 0}, 2); ok || value != 0 {
		t.Fatalf("zero-denominator center of gravity = (%v, %v)", value, ok)
	}
	if value, ok := calculateCorrelation([]float64{1, 1}, []float64{2, 2}, 2); ok || value != 0 {
		t.Fatalf("constant correlation = (%v, %v)", value, ok)
	}
	if value, ok := calculatePivot([]float64{1, 2, 1}, 0, 1, true); ok || value != 0 {
		t.Fatalf("zero-left pivot = (%v, %v)", value, ok)
	}
	if value, ok := calculatePivot([]float64{1, 2, 1}, 1, 1, true); !ok || value != 2 {
		t.Fatalf("strict pivot high = (%v, %v)", value, ok)
	}
	if value, ok := calculatePivot([]float64{1, 2, 1}, 1, 1, false); ok || value != 0 {
		t.Fatalf("non-strict pivot low = (%v, %v)", value, ok)
	}
	if value, ok := calculateALMA([]float64{1, 2}, 2, 0.5, 0); ok || value != 0 {
		t.Fatalf("zero-sigma ALMA = (%v, %v)", value, ok)
	}
	if value, ok := calculateSWMA([]float64{1, 2, 3}); ok || value != 0 {
		t.Fatalf("short SWMA = (%v, %v)", value, ok)
	}

	if state := newRollingKDJState(kdjConfig{period: 0, m1: 2, m2: 2}, 5, nil); state != nil {
		t.Fatalf("invalid KDJ state = %#v", state)
	}
	state := newRollingKDJState(kdjConfig{period: 2, m1: 2, m2: 2}, 0, nil)
	if state == nil || state.limit != minimumIndicatorSeriesLimit {
		t.Fatalf("default KDJ state = %#v", state)
		return
	}
	state.trimState(nil, nil, nil)
	if len(state.kTail) != 0 || len(state.dTail) != 0 || len(state.jTail) != 0 {
		t.Fatalf("empty KDJ trim did not reset tails: %#v", state)
	}
	if got := state.boundaryKAt(-1); got != 0 {
		t.Fatalf("negative KDJ boundary K = %v", got)
	}
	if got := state.boundaryDByKAt(-1); got != 0 {
		t.Fatalf("negative KDJ boundary D/K = %v", got)
	}
	if got := state.boundaryDByDAt(-1); got != 0 {
		t.Fatalf("negative KDJ boundary D/D = %v", got)
	}

	endTime := time.Date(2026, time.July, 15, 15, 0, 0, 0, time.UTC)
	if got := estimateTradingPeriodBars(2, "day", 5, "US.AAPL", false); got <= 0 {
		t.Fatalf("US daily warmup estimate = %d", got)
	}
	if got := estimateTradingPeriodBars(2, "day", 5, "UNKNOWN", false); got != 2*tradingSessionMinutesPerDay/5 {
		t.Fatalf("unknown-market daily warmup estimate = %d", got)
	}
	if labels := buildTradingPeriodLabels(nil, []time.Time{endTime}, "UNKNOWN", "day", false); len(labels) != 1 || labels[0] != invalidTradingPeriodLabelKey {
		t.Fatalf("unknown-market period label = %#v", labels)
	}
	if got := resolveIntervalMinutes(types.Interval("3x")); got != 1 {
		t.Fatalf("unsupported interval = %d", got)
	}
}
