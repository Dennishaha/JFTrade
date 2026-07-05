package indicatorruntime

import (
	"math"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestBuildMovingAverageSnapshotSupportsTypedMovingAverages(t *testing.T) {
	values := []float64{10, 12, 11, 13, 15, 14, 16, 18, 17}
	volumes := []float64{100, 140, 90, 160, 200, 150, 180, 220, 170}
	configs := []movingAverageConfig{
		{averageType: "MA", period: 5},
		{averageType: "EMA", period: 5},
		{averageType: "SMA", period: 5},
		{averageType: "SMMA", period: 5},
		{averageType: "LWMA", period: 5},
		{averageType: "TMA", period: 5},
		{averageType: "EXPMA", period: 5},
		{averageType: "HMA", period: 5},
		{averageType: "VWMA", period: 5},
		{averageType: "BOLL", period: 5},
	}

	for _, config := range configs {
		snapshot := buildMovingAverageSnapshot(values, volumes, config, 1)
		if snapshot == nil {
			t.Fatalf("snapshot for %#v is nil", config)
		}
		if _, ok := snapshot["value"]; !ok {
			t.Fatalf("snapshot for %#v missing value", config)
		}
	}

	maValue := readSnapshotNumber(t, buildMovingAverageSnapshot(values, volumes, movingAverageConfig{averageType: "MA", period: 5}, 1), "value")
	smaValue := readSnapshotNumber(t, buildMovingAverageSnapshot(values, volumes, movingAverageConfig{averageType: "SMA", period: 5}, 1), "value")
	bollValue := readSnapshotNumber(t, buildMovingAverageSnapshot(values, volumes, movingAverageConfig{averageType: "BOLL", period: 5}, 1), "value")
	emaValue := readSnapshotNumber(t, buildMovingAverageSnapshot(values, volumes, movingAverageConfig{averageType: "EMA", period: 5}, 1), "value")
	expmaValue := readSnapshotNumber(t, buildMovingAverageSnapshot(values, volumes, movingAverageConfig{averageType: "EXPMA", period: 5}, 1), "value")
	vwmaValue := readSnapshotNumber(t, buildMovingAverageSnapshot(values, volumes, movingAverageConfig{averageType: "VWMA", period: 5}, 1), "value")

	if maValue != smaValue {
		t.Fatalf("MA and SMA should match, got %v vs %v", maValue, smaValue)
	}
	if maValue != bollValue {
		t.Fatalf("MA and BOLL middle should match, got %v vs %v", maValue, bollValue)
	}
	if emaValue != expmaValue {
		t.Fatalf("EMA and EXPMA should match, got %v vs %v", emaValue, expmaValue)
	}
	if vwmaValue == maValue {
		t.Fatalf("VWMA should differ from MA with uneven volumes, both = %v", maValue)
	}
}

func TestAdvancedIndicatorCalculationsUseAuditedVectors(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5}
	if value, ok := calculateLinearRegression(values, 5, 0); !ok || math.Abs(value-5) > 1e-9 {
		t.Fatalf("linreg = %v/%v, want 5/true", value, ok)
	}
	if value, ok := calculateLinearRegression(values, 5, 1); !ok || math.Abs(value-4) > 1e-9 {
		t.Fatalf("linreg offset = %v/%v, want 4/true", value, ok)
	}
	if value, ok := calculatePivot([]float64{1, 3, 5, 4, 2}, 2, 2, true); !ok || value != 5 {
		t.Fatalf("pivot high = %v/%v, want 5/true", value, ok)
	}
	if _, ok := calculatePivot([]float64{1, 3, 5, 5, 2}, 2, 2, true); ok {
		t.Fatal("equal high should not confirm a pivot")
	}
	alma, ok := calculateALMA(values, 5, 0.85, 6)
	if !ok || alma <= 3 || alma >= 5 {
		t.Fatalf("alma = %v/%v, want weighted value in (3,5)", alma, ok)
	}

	mixed := []float64{1, 2, 4, 3, 5, 7}
	if value, ok := calculateCMO(mixed, 5); !ok || math.Abs(value-75) > 1e-9 {
		t.Fatalf("cmo = %v/%v, want 75/true", value, ok)
	}
	if value, ok := calculateBollingerBandWidth(values, 5, 2); !ok || math.Abs(value-(4*math.Sqrt(2)/3)) > 1e-9 {
		t.Fatalf("bbw = %v/%v", value, ok)
	}
	if value, ok := calculateCenterOfGravity(values, 5); !ok || math.Abs(value-(-35.0/15.0)) > 1e-9 {
		t.Fatalf("cog = %v/%v", value, ok)
	}
	times := []time.Time{
		time.Date(2026, 6, 12, 16, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 15, 16, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 16, 16, 0, 0, 0, time.UTC),
	}
	if value, ok := calculateAnchoredVWAP([]float64{10, 20, 30}, []float64{1, 1, 2}, times, "week", "US.AAPL", false); !ok || value != 80.0/3.0 {
		t.Fatalf("anchored weekly vwap = %v/%v", value, ok)
	}
	mixedSessionTimes := []time.Time{
		time.Date(2026, 6, 16, 16, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 20, 16, 0, 0, 0, time.UTC),
	}
	if value, ok := calculateAnchoredVWAP([]float64{10, 20}, []float64{1, 1}, mixedSessionTimes, "week", "US.AAPL", false); !ok || value != 15 {
		t.Fatalf("mixed-session anchored weekly vwap = %v/%v, want 15/true", value, ok)
	}
	if value, ok := calculateTSI([]float64{1, 2, 3, 4, 5, 6}, 2, 3); !ok || math.Abs(value-100) > 1e-9 {
		t.Fatalf("tsi = %v/%v, want 100/true", value, ok)
	}
	if value, ok := calculateCorrelation([]float64{1, 2, 4, 3, 5}, []float64{2, 4, 8, 6, 10}, 5); !ok || math.Abs(value-1) > 1e-9 {
		t.Fatalf("correlation = %v/%v, want 1/true", value, ok)
	}
	if value, ok := calculateMeanDeviation(mixed, 5); !ok || math.Abs(value-1.44) > 1e-9 {
		t.Fatalf("dev = %v/%v, want 1.44/true", value, ok)
	}
	if value, ok := calculateMedian(mixed, 5); !ok || value != 4 {
		t.Fatalf("median = %v/%v, want 4/true", value, ok)
	}
	if value, ok := calculatePercentileLinear(mixed, 5, 50); !ok || value != 4 {
		t.Fatalf("percentile linear = %v/%v, want 4/true", value, ok)
	}
	if value, ok := calculatePercentileNearest(mixed, 5, 80); !ok || value != 5 {
		t.Fatalf("percentile nearest = %v/%v, want 5/true", value, ok)
	}
	if value, ok := calculatePercentRank(mixed, 5); !ok || value != 100 {
		t.Fatalf("percentrank = %v/%v, want 100/true", value, ok)
	}
	if value, ok := calculateSWMA(mixed); !ok || value != 4.5 {
		t.Fatalf("swma = %v/%v, want 4.5/true", value, ok)
	}
}

func TestIndicatorRuntimeSnapshotIncludesV13MigrationIndicators(t *testing.T) {
	runtime := newIndicatorRuntime(`
		function onKLineClosed(ctx) {
			ctx.indicators["cmo:close:5"];
			ctx.indicators["tsi:close:2:3"];
			ctx.indicators["correlation:close:high:5"];
			ctx.indicators["dev:close:5"];
			ctx.indicators["median:close:5"];
			ctx.indicators["percentile_linear_interpolation:close:5:50"];
			ctx.indicators["percentile_nearest_rank:close:5:80"];
			ctx.indicators["percentrank:close:5"];
			ctx.indicators["swma:close"];
		}
	`, types.Interval1m, "US.AAPL")
	if runtime == nil {
		t.Fatal("expected indicator runtime")
	}
	for _, closePrice := range []float64{1, 2, 4, 3, 5, 7} {
		runtime.push(types.KLine{
			High:   fixedpoint.NewFromFloat(closePrice * 2),
			Low:    fixedpoint.NewFromFloat(closePrice - 1),
			Close:  fixedpoint.NewFromFloat(closePrice),
			Volume: fixedpoint.NewFromFloat(1000),
		}, market.SessionRegular)
	}
	snapshot := runtime.snapshot()
	assertScalarSnapshotApprox(t, snapshot, "cmo:close:5", 75)
	assertScalarSnapshotApprox(t, snapshot, "correlation:close:high:5", 1)
	assertScalarSnapshotApprox(t, snapshot, "dev:close:5", 1.44)
	assertScalarSnapshotApprox(t, snapshot, "median:close:5", 4)
	assertScalarSnapshotApprox(t, snapshot, "percentile_linear_interpolation:close:5:50", 4)
	assertScalarSnapshotApprox(t, snapshot, "percentile_nearest_rank:close:5:80", 5)
	assertScalarSnapshotApprox(t, snapshot, "percentrank:close:5", 100)
	assertScalarSnapshotApprox(t, snapshot, "swma:close", 4.5)
	if value, ok := scalarSnapshotValue(snapshot, "tsi:close:2:3"); !ok || value <= 0 || value > 100 {
		t.Fatalf("tsi snapshot = %v/%v, want value in (0, 100]", value, ok)
	}
}

func TestBuildMovingAverageSnapshotSupportsTimeUnits(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13}
	volumes := []float64{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	snapshot := buildMovingAverageSnapshot(values, volumes, movingAverageConfig{averageType: "MA", period: 1, timeUnit: "hour"}, 5)
	if snapshot == nil {
		t.Fatal("expected time-unit MA snapshot")
	}
	if value := readSnapshotNumber(t, snapshot, "value"); value != 7.5 {
		t.Fatalf("value = %v, want 7.5", value)
	}
	if previous := readSnapshotNumber(t, snapshot, "previous"); previous != 6.5 {
		t.Fatalf("previous = %v, want 6.5", previous)
	}
}

func TestBuildMovingAverageSnapshotUsesRegularTradingWindows(t *testing.T) {
	values := []float64{10, 100, 20}
	volumes := []float64{1, 1, 1}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 19, 59, 59, 0, time.UTC),
		time.Date(2026, time.May, 28, 21, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 14, 0, 0, 0, time.UTC),
	}
	snapshot := buildMovingAverageSnapshotForSymbol(values, volumes, endTimes, movingAverageConfig{averageType: "MA", period: 1, timeUnit: "day"}, 1, "US.AAPL", nil)
	if snapshot == nil {
		t.Fatal("expected trading-day MA snapshot")
	}
	if value := readSnapshotNumber(t, snapshot, "value"); value != 20 {
		t.Fatalf("value = %v, want 20", value)
	}
	if previous := readSnapshotNumber(t, snapshot, "previous"); previous != 10 {
		t.Fatalf("previous = %v, want 10", previous)
	}
	if _, ok := snapshot["value"].(float64); !ok {
		t.Fatalf("unexpected snapshot payload: %#v", snapshot)
	}
	if values := buildMovingAverageSnapshotForSymbol([]float64{10, 11, 12}, []float64{1, 1, 1}, []time.Time{
		time.Date(2026, time.May, 29, 0, 30, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 4, 30, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 9, 30, 0, 0, time.UTC),
	}, movingAverageConfig{averageType: "MA", period: 1, timeUnit: "day"}, 60, "HK.00700", nil); values != nil {
		t.Fatalf("expected non-regular HK samples to be ignored, got %#v", values)
	}
}

func TestBuildMovingAverageSnapshotUsesExtendedTradingWindowsWhenEnabled(t *testing.T) {
	values := []float64{1, 2, 3, 4, 10, 20, 30, 40}
	volumes := []float64{1, 1, 1, 1, 1, 1, 1, 1}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 1, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 7, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 13, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 1, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 7, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 13, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
	}
	snapshot := snapshotValueToMap(
		movingAverageSnapshotForSymbol(values, volumes, endTimes, movingAverageConfig{averageType: "MA", period: 1, timeUnit: "day"}, 60, "US.AAPL", true, nil),
		[...]string{"value", "previous"},
	)
	if snapshot == nil {
		t.Fatal("expected extended trading-day MA snapshot")
	}
	if value := readSnapshotNumber(t, snapshot, "value"); value != 25 {
		t.Fatalf("value = %v, want 25", value)
	}
	if previous := readSnapshotNumber(t, snapshot, "previous"); previous != 20 {
		t.Fatalf("previous = %v, want 20", previous)
	}

	regularSnapshot := buildMovingAverageSnapshotForSymbol(values, volumes, endTimes, movingAverageConfig{averageType: "MA", period: 1, timeUnit: "day"}, 60, "US.AAPL", nil)
	if regularSnapshot == nil {
		t.Fatal("expected regular trading-day MA snapshot")
	}
	if value := readSnapshotNumber(t, regularSnapshot, "value"); value != 40 {
		t.Fatalf("regular value = %v, want 40", value)
	}
	if previous := readSnapshotNumber(t, regularSnapshot, "previous"); previous != 4 {
		t.Fatalf("regular previous = %v, want 4", previous)
	}
}

func TestBuildMovingAverageSnapshotHonorsEMAWarmup(t *testing.T) {
	values := []float64{10, 11, 12}
	volumes := []float64{1, 1, 1}
	if snapshot := buildMovingAverageSnapshot(values, volumes, movingAverageConfig{averageType: "EMA", period: 5}, 1); snapshot != nil {
		t.Fatalf("expected nil EMA snapshot before warmup, got %#v", snapshot)
	}

	snapshot := buildMovingAverageSnapshot([]float64{10, 11, 12, 13, 14}, []float64{1, 1, 1, 1, 1}, movingAverageConfig{averageType: "EMA", period: 5}, 1)
	if snapshot == nil {
		t.Fatal("expected EMA snapshot at warmup boundary")
	}
	if snapshot["previous"] != nil {
		t.Fatalf("expected EMA previous to remain nil at warmup boundary, got %#v", snapshot)
	}

	snapshot = buildMovingAverageSnapshot([]float64{10, 11, 12, 13, 14, 15}, []float64{1, 1, 1, 1, 1, 1}, movingAverageConfig{averageType: "EMA", period: 5}, 1)
	if snapshot == nil {
		t.Fatal("expected EMA snapshot after warmup")
	}
	if snapshot["previous"] == nil {
		t.Fatalf("expected EMA previous after warmup, got %#v", snapshot)
	}
}

func TestTradingWindowEMASnapshotFromKeysMatchesMaterializedSelection(t *testing.T) {
	values := []float64{1, 10, 20, 2, 30, 40}
	volumes := []float64{1, 1, 1, 1, 1, 1}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 19, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 19, 0, 0, 0, time.UTC),
	}
	config := movingAverageConfig{averageType: "EMA", period: 1, timeUnit: "day"}
	cache := newSnapshotSeriesCache()
	labelKeys := cache.getTradingPeriodLabels(endTimes, "US.AAPL", config.timeUnit, false)

	current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(values, volumes, labelKeys, config)
	if !handled || !currentOK || !previousOK {
		t.Fatalf("unexpected EMA trading-window snapshot flags: handled=%v currentOK=%v previousOK=%v", handled, currentOK, previousOK)
	}

	expectedCurrentValues, expectedCurrentVolumes := selectTradingWindowSeriesWithCache(values, volumes, endTimes, config.period, config.timeUnit, "US.AAPL", len(values), false, cache)
	expectedCurrent, expectedPrevious, expectedCurrentOK, expectedPreviousOK := calculateMovingAverageSnapshotValuesWithCache(expectedCurrentValues, expectedCurrentVolumes, movingAverageConfig{averageType: "EMA", period: len(expectedCurrentValues)}, cache)
	if !expectedCurrentOK {
		t.Fatal("expected materialized current EMA value")
	}
	if expectedPreviousOK {
		t.Fatalf("materialized current window should not expose previous, got %v", expectedPrevious)
	}

	expectedPreviousValues, expectedPreviousVolumes := selectTradingWindowSeriesWithCache(values, volumes, endTimes, config.period, config.timeUnit, "US.AAPL", len(values)-1, false, cache)
	expectedPreviousCurrent, _, expectedPreviousCurrentOK, _ := calculateMovingAverageSnapshotValuesWithCache(expectedPreviousValues, expectedPreviousVolumes, movingAverageConfig{averageType: "EMA", period: len(expectedPreviousValues)}, cache)
	if !expectedPreviousCurrentOK {
		t.Fatal("expected materialized previous EMA value")
	}

	if math.Abs(current-expectedCurrent) > 1e-9 {
		t.Fatalf("current EMA = %v, want %v", current, expectedCurrent)
	}
	if math.Abs(previous-expectedPreviousCurrent) > 1e-9 {
		t.Fatalf("previous EMA = %v, want %v", previous, expectedPreviousCurrent)
	}
}

func TestTradingWindowEMAValueOnlineWithCacheMatchesMaterializedSelection(t *testing.T) {
	values := []float64{5, 10, 20, 50, 30, 40, 60, 80}
	volumes := []float64{1, 1, 1, 1, 1, 1, 1, 1}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 1, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 7, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 13, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 1, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 7, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 13, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
	}
	config := movingAverageConfig{averageType: "EXPMA", period: 1, timeUnit: "day"}
	cache := newSnapshotSeriesCache()

	actual, actualOK, handled := calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(values, volumes, endTimes, config, "US.AAPL", len(values), true, cache)
	if !handled || !actualOK {
		t.Fatalf("unexpected EMA online flags: handled=%v ok=%v", handled, actualOK)
	}

	selectedValues, selectedVolumes := selectTradingWindowSeriesWithCache(values, volumes, endTimes, config.period, config.timeUnit, "US.AAPL", len(values), true, cache)
	expected, _, expectedOK, _ := calculateMovingAverageSnapshotValuesWithCache(selectedValues, selectedVolumes, movingAverageConfig{averageType: "EXPMA", period: len(selectedValues)}, cache)
	if !expectedOK {
		t.Fatal("expected materialized extended-hours EMA value")
	}
	if math.Abs(actual-expected) > 1e-9 {
		t.Fatalf("online EMA = %v, want %v", actual, expected)
	}
}

func TestTradingWindowSMMASnapshotFromKeysMatchesMaterializedSelection(t *testing.T) {
	values := []float64{10, 20, 30, 40, 50, 60}
	volumes := []float64{1, 1, 1, 1, 1, 1}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 19, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 19, 0, 0, 0, time.UTC),
	}
	config := movingAverageConfig{averageType: "SMMA", period: 1, timeUnit: "day"}
	cache := newSnapshotSeriesCache()
	labelKeys := cache.getTradingPeriodLabels(endTimes, "US.AAPL", config.timeUnit, false)

	current, previous, currentOK, previousOK, handled := calculateTradingWindowMovingAverageSnapshotFromKeys(values, volumes, labelKeys, config)
	if !handled || !currentOK || !previousOK {
		t.Fatalf("unexpected SMMA trading-window snapshot flags: handled=%v currentOK=%v previousOK=%v", handled, currentOK, previousOK)
	}

	expectedCurrentValues, expectedCurrentVolumes := selectTradingWindowSeriesWithCache(values, volumes, endTimes, config.period, config.timeUnit, "US.AAPL", len(values), false, cache)
	expectedCurrent, expectedCurrentOK := calculateMovingAverageCurrentValue(expectedCurrentValues, expectedCurrentVolumes, config)
	if !expectedCurrentOK {
		t.Fatal("expected materialized current SMMA value")
	}
	expectedPreviousValues, expectedPreviousVolumes := selectTradingWindowSeriesWithCache(values, volumes, endTimes, config.period, config.timeUnit, "US.AAPL", len(values)-1, false, cache)
	expectedPrevious, expectedPreviousOK := calculateMovingAverageCurrentValue(expectedPreviousValues, expectedPreviousVolumes, config)
	if !expectedPreviousOK {
		t.Fatal("expected materialized previous SMMA value")
	}

	if math.Abs(current-expectedCurrent) > 1e-9 {
		t.Fatalf("current SMMA = %v, want %v", current, expectedCurrent)
	}
	if math.Abs(previous-expectedPrevious) > 1e-9 {
		t.Fatalf("previous SMMA = %v, want %v", previous, expectedPrevious)
	}
}

func TestTradingWindowTMACurrentValueOnlineWithCacheMatchesMaterializedSelection(t *testing.T) {
	values := []float64{10, 20, 30, 40}
	volumes := []float64{1, 1, 1, 1}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
	}
	config := movingAverageConfig{averageType: "TMA", period: 1, timeUnit: "day"}
	cache := newSnapshotSeriesCache()

	actual, actualOK, handled := calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(values, volumes, endTimes, config, "US.AAPL", len(values), false, cache)
	if !handled {
		t.Fatal("expected TMA trading-window path to be handled")
	}
	selectedValues, selectedVolumes := selectTradingWindowSeriesWithCache(values, volumes, endTimes, config.period, config.timeUnit, "US.AAPL", len(values), false, cache)
	expected, expectedOK := calculateMovingAverageCurrentValue(selectedValues, selectedVolumes, config)
	if actualOK != expectedOK {
		t.Fatalf("TMA ok = %v, want %v", actualOK, expectedOK)
	}
	if actualOK && math.Abs(actual-expected) > 1e-9 {
		t.Fatalf("online TMA = %v, want %v", actual, expected)
	}
}

func TestTradingWindowHMAValueOnlineWithCacheMatchesMaterializedSelection(t *testing.T) {
	values := []float64{10, 16}
	volumes := []float64{1, 1}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
	}
	config := movingAverageConfig{averageType: "HMA", period: 1, timeUnit: "day"}
	cache := newSnapshotSeriesCache()

	actual, actualOK, handled := calculateTradingWindowMovingAverageCurrentValueOnlineWithCache(values, volumes, endTimes, config, "US.AAPL", len(values), false, cache)
	if !handled || !actualOK {
		t.Fatalf("unexpected HMA trading-window flags: handled=%v ok=%v", handled, actualOK)
	}
	selectedValues, selectedVolumes := selectTradingWindowSeriesWithCache(values, volumes, endTimes, config.period, config.timeUnit, "US.AAPL", len(values), false, cache)
	expected, expectedOK := calculateMovingAverageCurrentValue(selectedValues, selectedVolumes, config)
	if !expectedOK {
		t.Fatal("expected materialized HMA value")
	}
	if math.Abs(actual-expected) > 1e-9 {
		t.Fatalf("online HMA = %v, want %v", actual, expected)
	}
}

func TestRollingMovingAverageStateMatchesBatchSnapshots(t *testing.T) {
	state := &rollingMovingAverageSnapshotState{kind: "MA", period: 3}
	vwmaState := &rollingMovingAverageSnapshotState{kind: "VWMA", period: 3}
	values := []float64{10, 12, 14, 16}
	volumes := []float64{1, 2, 3, 4}
	for index, value := range values {
		state.push(value, volumes[index])
		vwmaState.push(value, volumes[index])
	}
	assertSnapshotMapApproxEqual(t, state.snapshot(), buildMovingAverageSnapshot(values, volumes, movingAverageConfig{averageType: "MA", period: 3}, 1))
	assertSnapshotMapApproxEqual(t, vwmaState.snapshot(), buildMovingAverageSnapshot(values, volumes, movingAverageConfig{averageType: "VWMA", period: 3}, 1))
}

func TestCalculateWMASequenceMatchesExpectedWindows(t *testing.T) {
	sequence := calculateWMASequence([]float64{1, 2, 3, 4, 5}, 3)
	assertFloatSliceApproxEqual(t, sequence, []float64{14.0 / 6.0, 20.0 / 6.0, 26.0 / 6.0})
}
