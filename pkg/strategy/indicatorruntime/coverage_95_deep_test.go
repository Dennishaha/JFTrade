package indicatorruntime

import (
	"math"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestCoverage95NilSnapshotCachePreservesSnapshotValues(t *testing.T) {
	var cache *snapshotSeriesCache

	macd := cache.getMACDSnapshotValues(macdConfig{}, 8, 3, 5, 2, true, true).(*indicatorMACDSnapshot)
	if macd.diff != 8 || macd.signal != 3 || macd.histogram != 10 || !macd.hasPrevious || macd.previousHistogram != 6 {
		t.Fatalf("nil-cache MACD snapshot = %#v", macd)
	}
	kdj := cache.getKDJSnapshot(kdjConfig{}, kdjSeries{k: []float64{1, 4}, d: []float64{2, 5}, j: []float64{3, 6}}).(*indicatorKDJSnapshot)
	if kdj.k != 4 || kdj.d != 5 || kdj.j != 6 || !kdj.hasPrevious || kdj.previousK != 1 {
		t.Fatalf("nil-cache KDJ snapshot = %#v", kdj)
	}
	kdj = cache.getKDJSnapshotValues(kdjConfig{}, 7, 8, 9, 1, 2, 3, true, true).(*indicatorKDJSnapshot)
	if kdj.k != 7 || kdj.d != 8 || kdj.j != 9 || !kdj.hasPrevious || kdj.previousJ != 3 {
		t.Fatalf("nil-cache KDJ values snapshot = %#v", kdj)
	}

	if scalar := cache.getScalarSnapshot("value", 12.5, true).(*indicatorScalarSnapshot); scalar.current != 12.5 || !scalar.hasCurrent {
		t.Fatalf("nil-cache scalar snapshot = %#v", scalar)
	}
	if series := cache.getSeriesSnapshot("series", 9, 8, true, true).(*indicatorSeriesSnapshot); series.current != 9 || series.previous != 8 || !series.hasPrevious {
		t.Fatalf("nil-cache series snapshot = %#v", series)
	}
	if ma := cache.getMovingAverageSnapshot(movingAverageConfig{}, 9, 8, true, true).(*indicatorSeriesSnapshot); ma.current != 9 || ma.previous != 8 {
		t.Fatalf("nil-cache MA snapshot = %#v", ma)
	}
	if window := cache.getWindowSnapshot(windowConfig{}, 9, 8, true, true).(*indicatorSeriesSnapshot); window.current != 9 || window.previous != 8 {
		t.Fatalf("nil-cache window snapshot = %#v", window)
	}
	if stopLoss := cache.getStopLossSnapshot(stopLossConfig{}); len(stopLoss) != 0 {
		t.Fatalf("new nil-cache stop-loss snapshot = %#v", stopLoss)
	}

	macdSeries := cache.getMACDSeries([]float64{1, 2, 3, 4}, macdConfig{fastPeriod: 1, slowPeriod: 2, signalPeriod: 1})
	if len(macdSeries.diff) == 0 || len(macdSeries.signal) == 0 {
		t.Fatalf("nil-cache MACD series = %#v", macdSeries)
	}
	kdjSeries := cache.getKDJSeries([]float64{3, 4, 5}, []float64{1, 2, 3}, []float64{2, 3, 4}, kdjConfig{period: 2, m1: 2, m2: 2})
	if len(kdjSeries.k) == 0 || len(kdjSeries.d) == 0 || len(kdjSeries.j) == 0 {
		t.Fatalf("nil-cache KDJ series = %#v", kdjSeries)
	}

	cache.reset()
}

func TestCoverage95SnapshotCacheResetsAndUpdatesCachedFamilies(t *testing.T) {
	cache := newSnapshotSeriesCache()
	values := []float64{2, 4, 6, 8, 10}
	config := macdConfig{fastPeriod: 1, slowPeriod: 2, signalPeriod: 1}
	kdjConfig := kdjConfig{period: 2, m1: 2, m2: 2}

	cache.getEMASequence(values, 2)
	cache.getSMASequence(values, 2)
	cache.getSMMASequence(values, 2)
	cache.getWMASequence(values, 2)
	cache.getTMASequence(values, 2)
	cache.getHMASequence(values, 2)
	cache.getRSISeries(values, 2)
	cache.getMACDSeries(values, config)
	cache.getKDJSeries(values, values, values, kdjConfig)
	cache.getTradingPeriodLabels([]time.Time{time.Date(2026, time.June, 1, 20, 0, 0, 0, time.UTC)}, "US.AAPL", "day", false)
	cache.stopLossWindowStart.valid = true
	cache.stopLossWindowSelect.valid = true
	cache.stopLossWindowExtrema.valid = true
	cache.reset()

	if len(cache.ema) != 0 || len(cache.sma) != 0 || len(cache.smma) != 0 || len(cache.wma) != 0 || len(cache.tma) != 0 || len(cache.hma) != 0 || len(cache.rsi) != 0 || len(cache.macd) != 0 || len(cache.kdj) != 0 || len(cache.tradingPeriodLabels) != 0 {
		t.Fatalf("reset left calculated cache entries: %#v", cache)
	}
	if cache.stopLossWindowStart.valid || cache.stopLossWindowSelect.valid || cache.stopLossWindowExtrema.valid {
		t.Fatalf("reset left stop-loss cache state: %#v", cache)
	}

	series := cache.getSeriesSnapshot("series", 4, 3, true, true).(*indicatorSeriesSnapshot)
	if again := cache.getSeriesSnapshot("series", 5, 0, true, false).(*indicatorSeriesSnapshot); again != series || again.current != 5 || again.hasPrevious {
		t.Fatalf("series cache did not reuse and clear previous value: %#v", again)
	}
	window := cache.getWindowSnapshot(windowConfig{function: "sum", source: "close", period: 2}, 4, 3, true, true).(*indicatorSeriesSnapshot)
	if again := cache.getWindowSnapshot(windowConfig{function: "sum", source: "close", period: 2}, 5, 0, true, false).(*indicatorSeriesSnapshot); again != window || again.hasPrevious {
		t.Fatalf("window cache did not reuse and clear previous value: %#v", again)
	}
	ma := cache.getMovingAverageSnapshot(movingAverageConfig{averageType: "SMA", period: 2}, 4, 3, true, true).(*indicatorSeriesSnapshot)
	if again := cache.getMovingAverageSnapshot(movingAverageConfig{averageType: "SMA", period: 2}, 5, 0, true, false).(*indicatorSeriesSnapshot); again != ma || again.hasPrevious {
		t.Fatalf("MA cache did not reuse and clear previous value: %#v", again)
	}
}

func TestCoverage95SeriesLimitAndIntervalInputVariants(t *testing.T) {
	requirements := indicatorRequirements{
		ma:             []movingAverageConfig{{period: 2, timeUnit: "day"}},
		securitySource: []securitySourceConfig{{lookback: 2, timeUnit: "week"}},
		rsi:            []int{3},
		macd:           []macdConfig{{slowPeriod: 5, signalPeriod: 2}},
		bollinger:      []bollingerConfig{{period: 4}},
		stdev:          []int{5},
		windows:        []windowConfig{{period: 6}},
		cum:            []sourceConfig{{source: "close"}},
		stoch:          []sourcePeriodConfig{{period: 7}},
		kdj:            []kdjConfig{{period: 3, m1: 2, m2: 2}},
		atr:            []int{8},
		cci:            []int{9},
		williamsR:      []int{10},
		sar:            []sarConfig{{}},
		stopLoss:       []stopLossConfig{{timeValue: 2, timeUnit: "day"}},
		rsiDivergence:  []rsiDivergenceConfig{{period: 3, lookback: 4}},
		macdDivergence: []macdDivergenceConfig{{slowPeriod: 5, signalPeriod: 2, lookback: 4}},
		kdjDivergence:  []kdjDivergenceConfig{{period: 3, m1: 2, m2: 2, lookback: 4}},
		advanced: []advancedIndicatorConfig{
			{kind: "anchored_vwap", timeUnit: "month"},
			{kind: "alma", period: 7, left: 2, right: 2, offset: 1, timeUnit: "day"},
		},
	}
	if limit := calculateIndicatorSeriesLimit(requirements, 1); limit <= minimumIndicatorSeriesLimit {
		t.Fatalf("series limit = %d, want > %d for long lookback requirements", limit, minimumIndicatorSeriesLimit)
	}

	tests := []struct {
		interval types.Interval
		want     int
	}{
		{interval: types.Interval(""), want: 1},
		{interval: types.Interval("7min"), want: 7},
		{interval: types.Interval("5m"), want: 5},
		{interval: types.Interval("2h"), want: 120},
		{interval: types.Interval("2d"), want: 2 * tradingSessionMinutesPerDay},
		{interval: types.Interval("2w"), want: 2 * tradingSessionMinutesPerWeek},
		{interval: types.Interval("2mo"), want: 2 * tradingSessionMinutesPerMonth},
		{interval: types.Interval("bad"), want: 1},
		{interval: types.Interval("0m"), want: 1},
	}
	for _, tt := range tests {
		if got := resolveIntervalMinutes(tt.interval); got != tt.want {
			t.Fatalf("resolveIntervalMinutes(%q) = %d, want %d", tt.interval, got, tt.want)
		}
	}

	if labels := buildTradingPeriodLabels([]int64{1}, nil, "US.AAPL", "day", false); labels != nil {
		t.Fatalf("labels for empty timestamps = %#v, want nil", labels)
	}
	labels := buildTradingPeriodLabels(nil, []time.Time{time.Time{}}, "", "day", false)
	if len(labels) != 1 || labels[0] != invalidTradingPeriodLabelKey {
		t.Fatalf("labels for unknown market timestamp = %#v", labels)
	}
	if sequence := fillEMASequence([]float64{1}, nil, 2); sequence != nil {
		t.Fatalf("EMA sequence for empty input = %#v", sequence)
	}
	if sequence := fillEMASequence(nil, []float64{2, 4, 8}, 2); len(sequence) != 3 || sequence[0] != 2 || sequence[2] <= sequence[1] {
		t.Fatalf("EMA sequence = %#v", sequence)
	}
}

func TestCoverage95TradingPeriodAndWindowSelectionBehavior(t *testing.T) {
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.June, 1, 15, 0, 0, 0, time.UTC),
	}
	values := []float64{10, 20, 30}
	volumes := []float64{1, 2, 3}
	labels := buildTradingPeriodLabels(nil, endTimes, "US.AAPL", "day", false)
	if len(labels) != len(endTimes) || labels[0] == invalidTradingPeriodLabelKey {
		t.Fatalf("US day labels = %#v", labels)
	}

	if units := collectTradingPeriodUnits(indicatorRequirements{}, 60, "US.AAPL", false); units != nil {
		t.Fatalf("empty requirements units = %#v, want nil", units)
	}
	units := collectTradingPeriodUnits(indicatorRequirements{
		ma:             []movingAverageConfig{{timeUnit: "day"}, {timeUnit: "week"}},
		securitySource: []securitySourceConfig{{timeUnit: "day"}, {timeUnit: "month"}},
	}, 60, "US.AAPL", false)
	if len(units) != 3 {
		t.Fatalf("trading-period units = %#v, want day/week/month once", units)
	}

	current, previous, currentOK, previousOK := calculateTradingPeriodSourceSnapshotWithLookback(values, values, values, values, volumes, labels, "close", 0)
	if !currentOK || !previousOK || current != 30 || previous != 20 {
		t.Fatalf("trading-period snapshot = (%v, %v, %v, %v)", current, previous, currentOK, previousOK)
	}
	if current, previous, currentOK, previousOK = calculateTradingPeriodSourceSnapshotWithLookback(values, values, values, values, volumes, labels, "close", -1); currentOK || previousOK || current != 0 || previous != 0 {
		t.Fatalf("negative-lookback snapshot = (%v, %v, %v, %v)", current, previous, currentOK, previousOK)
	}
	if start, end := periodBoundsForKey(labels, labels[1], len(labels)); start != 1 || end != 2 {
		t.Fatalf("period bounds = (%d, %d), want (1, 2)", start, end)
	}
	if start, end := periodBoundsForKey(labels, -99, len(labels)); start != 0 || end != 0 {
		t.Fatalf("missing period bounds = (%d, %d), want (0, 0)", start, end)
	}

	if aggregated, ok := aggregateSourceWindow(values, values, values, values, volumes, 2, 2, "close"); ok || aggregated != 0 {
		t.Fatalf("empty aggregate = (%v, %v)", aggregated, ok)
	}
	if aggregated, ok := aggregateSourceWindow(values, values, values, values, volumes, 0, len(values), "not-a-source"); ok || aggregated != 0 {
		t.Fatalf("unknown-source aggregate = (%v, %v)", aggregated, ok)
	}
	series, aggregateVolumes, ok := aggregateFixedBarSeries(values, values, values, values, volumes, 2, "volume")
	if !ok || len(series) != 2 || len(aggregateVolumes) != 2 || series[0] != 3 || series[1] != 3 {
		t.Fatalf("fixed bar volume aggregation = %#v/%#v/%v", series, aggregateVolumes, ok)
	}
	bucketSeries, bucketVolumes, ok := aggregateTimeBucketSeries(values, values, values, values, volumes, endTimes, 24*60, "close")
	if !ok || len(bucketSeries) != 3 || len(bucketVolumes) != 3 {
		t.Fatalf("time-bucket aggregation = %#v/%#v/%v", bucketSeries, bucketVolumes, ok)
	}

	selected := selectTradingWindowIndices(endTimes, 2, "day", "US.AAPL", len(endTimes), false)
	if len(selected) != 2 || selected[0] != 2 || selected[1] != 1 {
		t.Fatalf("window selection = %#v", selected)
	}
	if selected := selectTradingWindowIndices(endTimes, 0, "day", "US.AAPL", len(endTimes), false); selected != nil {
		t.Fatalf("zero-period selection = %#v, want nil", selected)
	}
	cache := newSnapshotSeriesCache()
	selectedCached := selectTradingWindowIndicesWithCache(endTimes, 2, "day", "US.AAPL", len(endTimes), false, cache)
	if len(selectedCached) != len(selected) {
		t.Fatalf("cached selection = %#v, want %#v", selectedCached, selected)
	}
	materialized, selectedVolumes := materializeTradingWindowSeriesFromSelected(values, volumes, selectedCached, cache)
	if len(materialized) != 2 || materialized[0] != 20 || materialized[1] != 30 || len(selectedVolumes) != 2 {
		t.Fatalf("materialized trading window = %#v/%#v", materialized, selectedVolumes)
	}
	if materialized, selectedVolumes := materializeTradingWindowSeriesFromSelected(values, volumes, nil, cache); materialized != nil || selectedVolumes != nil || len(cache.tradingWindowValues) != 0 {
		t.Fatalf("empty materialized window = %#v/%#v cache=%#v", materialized, selectedVolumes, cache.tradingWindowValues)
	}
}

func TestCoverage95TradingWindowSequenceAndAggregatorFailures(t *testing.T) {
	values := []float64{10, 16, 20}
	labels := []int64{1, 2, 3}
	for _, tc := range []struct {
		averageType string
		period      int
	}{
		{averageType: "EMA", period: 2},
		{averageType: "EXPMA", period: 2},
		{averageType: "SMMA", period: 2},
		{averageType: "TMA", period: 1},
		{averageType: "HMA", period: 2},
	} {
		value, ok, handled := calculateTradingWindowSequenceValueFromKeys(values, labels, tc.averageType, tc.period, len(values))
		if !handled || !ok || math.IsNaN(value) {
			t.Fatalf("sequence type %s = (%v, %v, %v)", tc.averageType, value, ok, handled)
		}
	}
	if value, ok, handled := calculateTradingWindowSequenceValueFromKeys(values, labels, "unknown", 2, len(values)); handled || ok || value != 0 {
		t.Fatalf("unknown sequence = (%v, %v, %v)", value, ok, handled)
	}
	if value, ok, handled := calculateTradingWindowSequenceValueFromKeys(values, labels[:2], "EMA", 2, len(values)); !handled || ok || value != 0 {
		t.Fatalf("mismatched sequence = (%v, %v, %v)", value, ok, handled)
	}
	if summary := summarizeTradingWindowSelectionFromKeys([]int64{invalidTradingPeriodLabelKey}, 1, 1); summary.valid {
		t.Fatalf("invalid-only selection summary = %#v", summary)
	}
	if value, ok := calculateEMAFromTradingWindowSelection(values, labels, tradingWindowSelectionSummary{}); ok || value != 0 {
		t.Fatalf("invalid EMA selection = (%v, %v)", value, ok)
	}
	if value, ok := calculateSMMAFromTradingWindowSelection(values, labels, tradingWindowSelectionSummary{}); ok || value != 0 {
		t.Fatalf("invalid SMMA selection = (%v, %v)", value, ok)
	}
	if value, ok := calculateSingleValueFromTradingWindowSelection(values, labels, tradingWindowSelectionSummary{valid: true, count: 2}); ok || value != 0 {
		t.Fatalf("non-single selection = (%v, %v)", value, ok)
	}

	if _, handled := newTradingWindowMovingAverageAggregator(movingAverageConfig{averageType: "EMA"}); handled {
		t.Fatal("EMA unexpectedly uses aggregation path")
	}
	for _, tc := range []struct {
		kind   string
		values []float64
		vols   []float64
		wantOK bool
	}{
		{kind: "SMA", values: []float64{4, 2}, wantOK: true},
		{kind: "LWMA", values: []float64{4, 2}, wantOK: true},
		{kind: "VWMA", values: []float64{4, 2}, vols: []float64{1, 2}, wantOK: true},
		{kind: "VWMA", values: []float64{4}, vols: []float64{0}, wantOK: false},
	} {
		aggregator, handled := newTradingWindowMovingAverageAggregator(movingAverageConfig{averageType: tc.kind})
		if !handled {
			t.Fatalf("%s aggregator not handled", tc.kind)
		}
		for index, value := range tc.values {
			if !aggregator.push(value, tc.vols, index) && tc.wantOK {
				t.Fatalf("%s push %d failed", tc.kind, index)
			}
		}
		if _, ok := aggregator.value(); ok != tc.wantOK {
			t.Fatalf("%s value ok = %v, want %v", tc.kind, ok, tc.wantOK)
		}
	}
	var nilAggregator *tradingWindowMovingAverageAggregator
	if nilAggregator.push(1, nil, 0) {
		t.Fatal("nil aggregator push unexpectedly succeeded")
	}
}

func TestCoverage95AdvancedCalculationsCoverInvalidAndDegenerateData(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6}
	invalidCases := []struct {
		name string
		call func() (float64, bool)
	}{
		{"bbw", func() (float64, bool) { return calculateBollingerBandWidth([]float64{0, 0}, 2, 1) }},
		{"cog", func() (float64, bool) { return calculateCenterOfGravity([]float64{0, 0}, 2) }},
		{"cmo", func() (float64, bool) { return calculateCMO([]float64{1}, 2) }},
		{"tsi", func() (float64, bool) { return calculateTSI([]float64{1}, 1, 1) }},
		{"correlation", func() (float64, bool) { return calculateCorrelation([]float64{1, 1}, []float64{2, 2}, 2) }},
		{"mean deviation", func() (float64, bool) { return calculateMeanDeviation(values, 0) }},
		{"median", func() (float64, bool) { return calculateMedian(values, 0) }},
		{"percentile linear", func() (float64, bool) { return calculatePercentileLinear(values, 2, 101) }},
		{"percentile nearest", func() (float64, bool) { return calculatePercentileNearest(values, 2, -1) }},
		{"percent rank", func() (float64, bool) { return calculatePercentRank(values, 0) }},
		{"linear regression", func() (float64, bool) { return calculateLinearRegression(values, 2, -1) }},
		{"pivot", func() (float64, bool) { return calculatePivot([]float64{1, 3, 3}, 1, 1, true) }},
		{"alma", func() (float64, bool) { return calculateALMA(values, 2, 0.5, 0) }},
	}
	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			if value, ok := tc.call(); ok || value != 0 {
				t.Fatalf("%s = (%v, %v), want (0, false)", tc.name, value, ok)
			}
		})
	}

	validCases := []struct {
		name string
		call func() (float64, bool)
	}{
		{"bbw", func() (float64, bool) { return calculateBollingerBandWidth(values, 3, 2) }},
		{"cog", func() (float64, bool) { return calculateCenterOfGravity(values, 3) }},
		{"cmo", func() (float64, bool) { return calculateCMO(values, 3) }},
		{"tsi", func() (float64, bool) { return calculateTSI(values, 2, 3) }},
		{"correlation", func() (float64, bool) { return calculateCorrelation(values, []float64{2, 4, 6, 8, 10, 12}, 3) }},
		{"mean deviation", func() (float64, bool) { return calculateMeanDeviation(values, 3) }},
		{"median", func() (float64, bool) { return calculateMedian(values, 4) }},
		{"percentile linear", func() (float64, bool) { return calculatePercentileLinear(values, 4, 50) }},
		{"percentile nearest", func() (float64, bool) { return calculatePercentileNearest(values, 4, 50) }},
		{"percent rank", func() (float64, bool) { return calculatePercentRank(values, 3) }},
		{"linear regression", func() (float64, bool) { return calculateLinearRegression(values, 3, 0) }},
		{"pivot", func() (float64, bool) { return calculatePivot([]float64{1, 4, 2}, 1, 1, true) }},
		{"alma", func() (float64, bool) { return calculateALMA(values, 3, 0.5, 6) }},
	}
	for _, tc := range validCases {
		t.Run(tc.name, func(t *testing.T) {
			if value, ok := tc.call(); !ok || math.IsNaN(value) || math.IsInf(value, 0) {
				t.Fatalf("%s = (%v, %v), want finite value", tc.name, value, ok)
			}
		})
	}
	if value, ok := calculatePercentRank([]float64{5}, 1); !ok || value != 0 {
		t.Fatalf("single-value percent rank = (%v, %v)", value, ok)
	}
	if value, ok := calculateCMO([]float64{1, 1, 1}, 2); !ok || value != 0 {
		t.Fatalf("flat CMO = (%v, %v)", value, ok)
	}
	if value, ok := calculateTSI([]float64{1, 1, 1}, 1, 1); !ok || value != 0 {
		t.Fatalf("flat TSI = (%v, %v)", value, ok)
	}
	if value, previous, currentOK, previousOK := calculateOBVSnapshot(nil, nil); currentOK || previousOK || value != 0 || previous != 0 {
		t.Fatalf("empty OBV snapshot = (%v, %v, %v, %v)", value, previous, currentOK, previousOK)
	}
	if value, previous, currentOK, previousOK := calculateOBVSnapshot([]float64{3, 2, 4}, []float64{1, 2, 3}); !currentOK || !previousOK || value != 1 || previous != -2 {
		t.Fatalf("OBV snapshot = (%v, %v, %v, %v)", value, previous, currentOK, previousOK)
	}
	if value, ok := advancedScalarIndicatorValue(advancedIndicatorConfig{kind: "unsupported"}, values); ok || value != 0 {
		t.Fatalf("unsupported scalar advanced value = (%v, %v)", value, ok)
	}
}

func TestCoverage95ParsingRejectsMalformedBusinessKeys(t *testing.T) {
	if unit, ok := parseOptionalAdvancedTimeUnit([]string{"a", "b", "c"}, 1); ok || unit != "" {
		t.Fatalf("extra optional time unit = (%q, %v)", unit, ok)
	}
	if config, ok := parseThreePartMovingAverageConfig([]string{"ma", "ema", "bad"}); ok || config != (movingAverageConfig{}) {
		t.Fatalf("invalid three-part MA = %#v/%v", config, ok)
	}
	if config, ok := parseFivePartMovingAverageConfig([]string{"ma", "ema", "2", "bad", "close"}); ok || config != (movingAverageConfig{}) {
		t.Fatalf("invalid five-part MA time unit = %#v/%v", config, ok)
	}
	if config, ok := parseFivePartMovingAverageConfig([]string{"ma", "ema", "2", "day", "bad"}); ok || config != (movingAverageConfig{}) {
		t.Fatalf("invalid five-part MA source = %#v/%v", config, ok)
	}
	for _, value := range []string{"invalid", "0m", "-1m"} {
		if unit, ok := parseIndicatorTimeUnit(value); ok || unit != "" {
			t.Fatalf("invalid indicator unit %q = (%q, %v)", value, unit, ok)
		}
	}
	for _, value := range []string{"weekish", "5x"} {
		if unit, ok := parseStopLossTimeUnit(value); ok || unit != "" {
			t.Fatalf("invalid stop-loss unit %q = (%q, %v)", value, unit, ok)
		}
	}
	if unit, ok := parseStopLossTimeUnit("\"bars\""); !ok || unit != "" {
		t.Fatalf("quoted bars unit = (%q, %v)", unit, ok)
	}

	strict := newIndicatorRequirementSetBuilder(true)
	invalidKeys := []string{
		"linreg:close:2:-1",
		"pivothigh:close:2:2:invalid",
		"kc:close:2:1:true:invalid",
		"alma:close:2:0.5:1:invalid",
		"stoch:volume:2",
		"divergence:rsi:2:top:bad",
		"divergence:macd:1:2:3:top:2:extra",
		"divergence:kdj:1:2:3:top:2:extra",
	}
	for _, key := range invalidKeys {
		if err := strict.parseKey(key); err == nil {
			t.Fatalf("strict parse of malformed key %q unexpectedly succeeded", key)
		}
	}
	if err := strict.parseKey("divergence:unknown:2:top:1"); err == nil {
		t.Fatal("strict unsupported divergence unexpectedly succeeded")
	}
	permissive := newIndicatorRequirementSetBuilder(false)
	if err := permissive.parseKey("divergence:unknown:2:top:1"); err != nil {
		t.Fatalf("permissive unsupported divergence = %v", err)
	}
}
