package indicatorruntime

import (
	"testing"
	"time"
)

func TestCoverage95AdvancedParserRejectsEveryMalformedShape(t *testing.T) {
	builder := newIndicatorRequirementSetBuilder(true)
	invalidParsers := []struct {
		name  string
		parse func() error
	}{
		{"advanced default", func() error { return builder.parseAdvancedKey("unknown", []string{"unknown"}) }},
		{"anchored length", func() error {
			return builder.parseAnchoredVWAPKey("anchored_vwap:day", []string{"anchored_vwap", "day"})
		}},
		{"advanced period length", func() error {
			return builder.parseAdvancedSourcePeriodKey("cog:close", []string{"cog", "close"}, "cog", "invalid cog key: %s")
		}},
		{"bbw length", func() error { return builder.parseBBWKey("bbw:close:2", []string{"bbw", "close", "2"}) }},
		{"tsi length", func() error { return builder.parseTSIKey("tsi:close:2", []string{"tsi", "close", "2"}) }},
		{"correlation length", func() error {
			return builder.parseCorrelationKey("correlation:close:high", []string{"correlation", "close", "high"})
		}},
		{"percentile length", func() error {
			return builder.parsePercentileKey("percentile_nearest_rank:close:2", []string{"percentile_nearest_rank", "close", "2"})
		}},
		{"advanced source length", func() error {
			return builder.parseAdvancedSourceKey("obv", []string{"obv"}, "obv", "invalid obv key: %s")
		}},
		{"linreg length", func() error { return builder.parseLinregKey("linreg:close:2", []string{"linreg", "close", "2"}) }},
		{"pivot length", func() error { return builder.parsePivotKey("pivothigh:high:2", []string{"pivothigh", "high", "2"}) }},
		{"keltner length", func() error { return builder.parseKeltnerKey("kc:close:2:1", []string{"kc", "close", "2", "1"}) }},
		{"alma length", func() error { return builder.parseALMAKey("alma:close:2:0.5", []string{"alma", "close", "2", "0.5"}) }},
		{"stoch invalid unit", func() error {
			return builder.parseStochKey("stoch:close:2:bad", []string{"stoch", "close", "2", "bad"})
		}},
		{"rsi divergence length", func() error {
			return builder.parseRSIDivergenceKey("divergence:rsi", []string{"divergence", "rsi"}, "top", 2)
		}},
		{"macd divergence length", func() error {
			return builder.parseMACDDivergenceKey("divergence:macd", []string{"divergence", "macd"}, "top", 2)
		}},
		{"kdj divergence length", func() error {
			return builder.parseKDJDivergenceKey("divergence:kdj", []string{"divergence", "kdj"}, "top", 2)
		}},
	}
	for _, tc := range invalidParsers {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.parse(); err == nil {
				t.Fatalf("%s malformed shape unexpectedly parsed", tc.name)
			}
		})
	}
}

func TestCoverage95FixedTimeframeValidationReportsEveryIndicatorFamily(t *testing.T) {
	invalidRequirements := []struct {
		name         string
		requirements indicatorRequirements
	}{
		{"moving average", indicatorRequirements{ma: []movingAverageConfig{{timeUnit: "5m"}}}},
		{"security source", indicatorRequirements{securitySource: []securitySourceConfig{{timeUnit: "5m"}}}},
		{"rsi", indicatorRequirements{rsiSource: []sourcePeriodConfig{{timeUnit: "5m"}}}},
		{"stdev", indicatorRequirements{stdevSource: []sourcePeriodConfig{{timeUnit: "5m"}}}},
		{"variance", indicatorRequirements{variance: []sourcePeriodConfig{{timeUnit: "5m"}}}},
		{"stoch", indicatorRequirements{stoch: []sourcePeriodConfig{{timeUnit: "5m"}}}},
		{"cci", indicatorRequirements{cciSource: []sourcePeriodConfig{{timeUnit: "5m"}}}},
		{"mfi", indicatorRequirements{mfi: []sourcePeriodConfig{{timeUnit: "5m"}}}},
		{"advanced", indicatorRequirements{advanced: []advancedIndicatorConfig{{kind: "cmo", timeUnit: "5m"}}}},
	}
	for _, tc := range invalidRequirements {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateFixedTimeframeRequirements(tc.requirements, 2); err == nil {
				t.Fatalf("%s invalid fixed timeframe unexpectedly accepted", tc.name)
			}
		})
	}
	if err := validateFixedTimeframe("moving average", "5m", 2); err == nil {
		t.Fatal("unaligned five-minute timeframe was accepted for two-minute bars")
	}
	if err := validateFixedTimeframe("moving average", "1m", 5); err == nil {
		t.Fatal("lower fixed timeframe was accepted")
	}
	if err := validateFixedTimeframe("moving average", "day", 0); err != nil {
		t.Fatalf("daily timeframe at default one-minute interval = %v", err)
	}
	if got := formatIndicatorTimeUnit("month"); got != "M" {
		t.Fatalf("month display = %q", got)
	}
	if got := formatIntervalMinutes(0); got != "1m" {
		t.Fatalf("zero interval display = %q", got)
	}
}

func TestCoverage95MovingAverageHelpersRejectInsufficientInput(t *testing.T) {
	if value, ok := simpleMovingAverage([]float64{1}, 2); ok || value != 0 {
		t.Fatalf("short SMA = (%v, %v)", value, ok)
	}
	if values := calculateTMASequenceWithCache([]float64{1, 2}, 3, nil); values != nil {
		t.Fatalf("short TMA sequence = %#v", values)
	}
	if values := calculateHMASequenceWithCache([]float64{1}, 2, nil); values != nil {
		t.Fatalf("short HMA sequence = %#v", values)
	}
	if value, ok := volumeWeightedMovingAverage([]float64{1, 2}, []float64{0, 0}, 2); ok || value != 0 {
		t.Fatalf("zero-volume VWMA = (%v, %v)", value, ok)
	}
	if value, ok := volumeWeightedMovingAverage([]float64{1}, []float64{1}, 2); ok || value != 0 {
		t.Fatalf("short VWMA = (%v, %v)", value, ok)
	}
	if values := calculateSMASequence([]float64{1}, 2); values != nil {
		t.Fatalf("short SMA sequence = %#v", values)
	}
	if values := calculateSMMASequence([]float64{1}, 2); values != nil {
		t.Fatalf("short SMMA sequence = %#v", values)
	}
	if values := calculateRMASequence([]float64{1}, 2); values != nil {
		t.Fatalf("short RMA sequence = %#v", values)
	}
	if values := calculateWMASequence([]float64{1}, 2); values != nil {
		t.Fatalf("short WMA sequence = %#v", values)
	}
}

func TestCoverage95TradingPeriodHelpersRejectInvalidSelections(t *testing.T) {
	if current, previous, currentOK, previousOK := calculateTradingPeriodSourceSnapshotWithLookback(nil, nil, nil, nil, nil, nil, "close", 0); currentOK || previousOK || current != 0 || previous != 0 {
		t.Fatalf("empty trading-period snapshot = (%v, %v, %v, %v)", current, previous, currentOK, previousOK)
	}
	if value, ok := aggregateSourceWindow(nil, nil, nil, nil, nil, 0, 1, "close"); ok || value != 0 {
		t.Fatalf("empty source window = (%v, %v)", value, ok)
	}
	if values, volumes, ok := aggregateTimeBucketSeries([]float64{1}, []float64{1}, []float64{1}, []float64{1}, []float64{1}, []time.Time{time.Now()}, 0, "close"); ok || values != nil || volumes != nil {
		t.Fatalf("invalid time bucket series = %#v/%#v/%v", values, volumes, ok)
	}
	if key := timeframeBucketKey(time.Unix(123, 0), 0); key != 123 {
		t.Fatalf("zero-minute bucket key = %d", key)
	}
	if values, volumes := selectTradingWindowSeriesWithCache(nil, nil, nil, 1, "day", "US.AAPL", 1, false, nil); values != nil || volumes != nil {
		t.Fatalf("empty trading-window series = %#v/%#v", values, volumes)
	}
	if selected := selectTradingWindowIndicesWithCache(nil, 1, "day", "US.AAPL", 1, false, nil); selected != nil {
		t.Fatalf("nil-cache empty selection = %#v", selected)
	}
	if selected := selectTradingWindowIndicesInto(nil, nil, 0, "day", "US.AAPL", 0, false); selected != nil {
		t.Fatalf("invalid in-place selection = %#v", selected)
	}
	if usesTradingPeriodWindow("day", 1, "", []time.Time{time.Now()}, false) {
		t.Fatal("trading window accepted empty symbol")
	}
	if usesTradingPeriodWindow("day", 1, "UNKNOWN", []time.Time{time.Now()}, false) {
		t.Fatal("trading window accepted unknown market")
	}
	if usesFixedIntradayTimeframe("5m", 0) {
		t.Fatal("fixed intraday timeframe accepted non-positive interval")
	}
	if usesFixedIntradayTimeframe("day", 1) || isNumericMinuteTimeUnit("day") || isNumericMinuteTimeUnit("xm") {
		t.Fatal("minute timeframe classification accepted non-minute unit")
	}
}

func TestCoverage95SnapshotReadersAndRollingBuffersHandleEmptyState(t *testing.T) {
	var macd *indicatorMACDSnapshot
	if value, ok := macd.PreferredScalarValue(); ok || value != 0 {
		t.Fatalf("nil MACD preferred value = (%v, %v)", value, ok)
	}
	if _, _, _, _, ok := macd.SeriesField("diff"); ok {
		t.Fatal("nil MACD exposed a series field")
	}
	macd = &indicatorMACDSnapshot{diff: 3, signal: 2, histogram: 2}
	for _, key := range []string{"previousDiff", "previousSignal", "previousHistogram"} {
		if value, ok := macd.FieldValue(key); !ok || value != nil {
			t.Fatalf("MACD %s = %#v/%v", key, value, ok)
		}
	}
	if _, ok := macd.FieldValue("unknown"); ok {
		t.Fatal("MACD accepted unknown field")
	}

	var kdj *indicatorKDJSnapshot
	if value, ok := kdj.PreferredScalarValue(); ok || value != 0 {
		t.Fatalf("nil KDJ preferred value = (%v, %v)", value, ok)
	}
	if _, _, _, _, ok := kdj.SeriesField("k"); ok {
		t.Fatal("nil KDJ exposed a series field")
	}
	kdj = &indicatorKDJSnapshot{k: 3, d: 2, j: 5}
	for _, key := range []string{"previousK", "previousD", "previousJ"} {
		if value, ok := kdj.FieldValue(key); !ok || value != nil {
			t.Fatalf("KDJ %s = %#v/%v", key, value, ok)
		}
	}
	if _, _, _, _, ok := kdj.SeriesField("unknown"); ok {
		t.Fatal("KDJ accepted unknown series field")
	}

	var deque *monotonicWindowValueDeque
	deque.compact()
	deque.popExpired(1)
	deque.pushMax(0, 1)
	deque.pushMin(0, 1)
	if value, ok := deque.frontValue(); ok || value != 0 {
		t.Fatalf("nil monotonic deque front = (%v, %v)", value, ok)
	}
	deque = &monotonicWindowValueDeque{values: []windowValue{{index: 0, value: 1}, {index: 1, value: 2}}, start: 2}
	deque.compact()
	if len(deque.values) != 0 || deque.start != 0 {
		t.Fatalf("exhausted monotonic deque compact = %#v", deque)
	}
	deque.pushMax(1, 2)
	deque.pushMax(2, 3)
	deque.popExpired(3)
	if _, ok := deque.frontValue(); ok {
		t.Fatal("expired max deque still has a front value")
	}

	var window *rollingFloatWindow
	if value, ok := window.push(1, 1); ok || value != 0 || window.len() != 0 {
		t.Fatalf("nil rolling window push = (%v, %v), len=%d", value, ok, window.len())
	}
	window = &rollingFloatWindow{}
	window.ensureCapacity(2)
	window.push(1, 2)
	window.push(2, 2)
	if evicted, ok := window.push(3, 2); !ok || evicted != 1 {
		t.Fatalf("rolling window eviction = (%v, %v)", evicted, ok)
	}
	if value, ok := window.last(); !ok || value != 3 {
		t.Fatalf("rolling window last = (%v, %v)", value, ok)
	}
	if _, ok := window.at(-1); ok {
		t.Fatal("rolling window accepted negative offset")
	}
}

func TestCoverage95OscillatorAndVWAPHelpersCoverFlatAndInvalidSeries(t *testing.T) {
	if value := calculateCCI([]float64{1}, []float64{1}, []float64{1}, 2); value != nil {
		t.Fatalf("short CCI = %#v", value)
	}
	if values := calculateCCISeries([]float64{1, 1}, []float64{1, 1}, []float64{1, 1}, 2); len(values) != 1 || values[0] != 0 {
		t.Fatalf("flat CCI series = %#v", values)
	}
	if value, ok := calculateCCIFromValues([]float64{1}, 2); ok || value != 0 {
		t.Fatalf("short source CCI = (%v, %v)", value, ok)
	}
	if value := calculateWilliamsR([]float64{1}, []float64{1}, []float64{1}, 2); value != nil {
		t.Fatalf("short Williams R = %#v", value)
	}
	if values := calculateWilliamsRSeries([]float64{1, 1}, []float64{1, 1}, []float64{1, 1}, 2); len(values) != 1 || values[0] != -50 {
		t.Fatalf("flat Williams R series = %#v", values)
	}
	if value, ok := calculateSessionVWAP([]float64{1}, nil, []time.Time{time.Now()}, "US.AAPL", false); ok || value != 0 {
		t.Fatalf("mismatched session VWAP = (%v, %v)", value, ok)
	}
	if value, ok := calculateAnchoredVWAP([]float64{1}, []float64{0}, []time.Time{time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC)}, "day", "US.AAPL", false); ok || value != 0 {
		t.Fatalf("zero-volume anchored VWAP = (%v, %v)", value, ok)
	}
}
