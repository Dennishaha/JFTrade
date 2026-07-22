package indicatorruntime

import (
	"math"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

var benchmarkSnapshotSink map[string]any
var benchmarkMovingAverageSnapshotSink any

func assertSeriesSnapshot(t *testing.T, snapshot map[string]any, key string, current float64, previous float64) {
	t.Helper()
	assertSeriesSnapshotApprox(t, snapshot, key, current, previous)
}

func assertSeriesSnapshotApprox(t *testing.T, snapshot map[string]any, key string, current float64, previous float64) {
	t.Helper()
	reader, ok := snapshot[key].(interface {
		SeriesField(string) (float64, float64, bool, bool, bool)
	})
	if !ok {
		t.Fatalf("snapshot %s type = %T", key, snapshot[key])
	}
	gotCurrent, gotPrevious, currentOK, previousOK, seriesOK := reader.SeriesField("value")
	if !seriesOK || !currentOK || !previousOK {
		t.Fatalf("snapshot %s series = (%v, %v, %v, %v, %v)", key, gotCurrent, gotPrevious, currentOK, previousOK, seriesOK)
	}
	if math.Abs(gotCurrent-current) > 1e-9 || math.Abs(gotPrevious-previous) > 1e-9 {
		t.Fatalf("snapshot %s = (%v, %v), want (%v, %v)", key, gotCurrent, gotPrevious, current, previous)
	}
}

func assertScalarSnapshotApprox(t *testing.T, snapshot map[string]any, key string, expected float64) {
	t.Helper()
	value, ok := scalarSnapshotValue(snapshot, key)
	if !ok {
		t.Fatalf("snapshot %s missing scalar value: %#v", key, snapshot[key])
	}
	if math.Abs(value-expected) > 1e-9 {
		t.Fatalf("snapshot %s = %v, want %v", key, value, expected)
	}
}

func scalarSnapshotValue(snapshot map[string]any, key string) (float64, bool) {
	reader, ok := snapshot[key].(interface {
		ScalarValue() (float64, bool)
	})
	if !ok {
		return 0, false
	}
	return reader.ScalarValue()
}

func readSnapshotNumber(t *testing.T, snapshot map[string]any, key string) float64 {
	t.Helper()
	value, ok := snapshot[key]
	if !ok {
		t.Fatalf("snapshot missing %s: %#v", key, snapshot)
	}
	number, ok := value.(float64)
	if !ok {
		t.Fatalf("snapshot %s type = %T", key, value)
	}
	return number
}

func snapshotToMap(snapshot any, keys []string) map[string]any {
	if snapshot == nil {
		return nil
	}
	if values, ok := snapshot.(map[string]any); ok {
		return values
	}
	reader, ok := snapshot.(interface {
		FieldValue(string) (any, bool)
	})
	if !ok {
		return nil
	}
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		value, ok := reader.FieldValue(key)
		if ok {
			result[key] = value
		}
	}
	return result
}
func readSnapshotBool(t *testing.T, snapshot map[string]any, key string) bool {
	t.Helper()
	value, ok := snapshot[key]
	if !ok {
		t.Fatalf("snapshot missing %s: %#v", key, snapshot)
	}
	flag, ok := value.(bool)
	if !ok {
		t.Fatalf("snapshot %s type = %T", key, value)
	}
	return flag
}

func readSnapshotString(t *testing.T, snapshot map[string]any, key string) string {
	t.Helper()
	value, ok := snapshot[key]
	if !ok {
		t.Fatalf("snapshot missing %s: %#v", key, snapshot)
	}
	text, ok := value.(string)
	if !ok {
		t.Fatalf("snapshot %s type = %T", key, value)
	}
	return text
}

func assertFloatSliceApproxEqual(t *testing.T, actual, expected []float64) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("len(actual) = %d, want %d (%v)", len(actual), len(expected), actual)
	}
	for index := range expected {
		if math.Abs(actual[index]-expected[index]) > 1e-9 {
			t.Fatalf("actual[%d] = %v, want %v", index, actual[index], expected[index])
		}
	}
}

func assertSnapshotNumberApproxEqual(t *testing.T, snapshot map[string]any, key string, expected float64) {
	t.Helper()
	if math.Abs(readSnapshotNumber(t, snapshot, key)-expected) > 1e-9 {
		t.Fatalf("snapshot[%s] = %v, want %v", key, readSnapshotNumber(t, snapshot, key), expected)
	}
}

func assertOptionalNumberApproxEqual(t *testing.T, actual, expected any) {
	t.Helper()
	if actual == nil || expected == nil {
		if actual != expected {
			t.Fatalf("actual = %v, expected = %v", actual, expected)
		}
		return
	}
	actualNumber, ok := actual.(float64)
	if !ok {
		t.Fatalf("actual type = %T", actual)
	}
	expectedNumber, ok := expected.(float64)
	if !ok {
		t.Fatalf("expected type = %T", expected)
	}
	if math.Abs(actualNumber-expectedNumber) > 1e-9 {
		t.Fatalf("actual = %v, expected = %v", actualNumber, expectedNumber)
	}
}

func assertSnapshotMapApproxEqual(t *testing.T, actual, expected map[string]any) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Fatalf("len(actual) = %d, len(expected) = %d", len(actual), len(expected))
	}
	for key, expectedValue := range expected {
		actualValue, ok := actual[key]
		if !ok {
			t.Fatalf("actual missing key %s", key)
		}
		assertOptionalNumberApproxEqual(t, actualValue, expectedValue)
	}
}

func firstOrZero(values []float64, index int) float64 {
	if index < 0 || index >= len(values) {
		return 0
	}
	return values[index]
}

func BenchmarkIndicatorRuntimeSnapshot(b *testing.B) {
	runtime := benchmarkIndicatorRuntime(b)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		benchmarkSnapshotSink = runtime.snapshot()
	}
}

func BenchmarkIndicatorRuntimeProtectSessionSnapshot(b *testing.B) {
	runtime := benchmarkProtectSessionIndicatorRuntime(b)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		benchmarkSnapshotSink = runtime.snapshot()
	}
}

func BenchmarkIndicatorRuntimePushAndSnapshot(b *testing.B) {
	runtime := benchmarkIndicatorRuntime(b)
	baseTime := time.Date(2026, 5, 28, 14, 30, 0, 0, time.UTC)
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		closeValue := 100 + float64(index%37)
		runtime.push(types.KLine{
			Symbol:    "US.AAPL",
			Interval:  types.Interval1m,
			StartTime: types.Time(baseTime.Add(time.Duration(index) * time.Minute)),
			EndTime:   types.Time(baseTime.Add(time.Duration(index+1) * time.Minute)),
			High:      fixedpoint.NewFromFloat(closeValue + 1),
			Low:       fixedpoint.NewFromFloat(closeValue - 1),
			Close:     fixedpoint.NewFromFloat(closeValue),
			Volume:    fixedpoint.NewFromFloat(1000 + float64(index%100)),
		}, market.SessionRegular)
		benchmarkSnapshotSink = runtime.snapshot()
	}
}

func BenchmarkTradingWindowMovingAverageSnapshotFromKeys(b *testing.B) {
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
	cache := newSnapshotSeriesCache()
	labelKeys := cache.getTradingPeriodLabels(endTimes, "US.AAPL", "day", false)
	configs := []movingAverageConfig{
		{averageType: "EMA", period: 1, timeUnit: "day"},
		{averageType: "SMMA", period: 1, timeUnit: "day"},
		{averageType: "TMA", period: 1, timeUnit: "day"},
	}
	for _, config := range configs {
		b.Run(config.averageType, func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				current, previous, currentOK, previousOK, _ := calculateTradingWindowMovingAverageSnapshotFromKeys(values, volumes, labelKeys, config)
				benchmarkMovingAverageSnapshotSink = snapshotValueToMap(
					cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK),
					[...]string{"value", "previous"},
				)
			}
		})
	}

	hmaValues := []float64{10, 16}
	hmaVolumes := []float64{1, 1}
	hmaEndTimes := []time.Time{
		time.Date(2026, time.May, 28, 12, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
	}
	hmaLabelKeys := cache.getTradingPeriodLabels(hmaEndTimes, "US.AAPL", "day", false)
	hmaConfig := movingAverageConfig{averageType: "HMA", period: 1, timeUnit: "day"}
	b.Run("HMA", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			current, previous, currentOK, previousOK, _ := calculateTradingWindowMovingAverageSnapshotFromKeys(hmaValues, hmaVolumes, hmaLabelKeys, hmaConfig)
			benchmarkMovingAverageSnapshotSink = snapshotValueToMap(
				cache.getMovingAverageSnapshot(hmaConfig, current, previous, currentOK, previousOK),
				[...]string{"value", "previous"},
			)
		}
	})
}

func benchmarkIndicatorRuntime(b *testing.B) *indicatorRuntime {
	b.Helper()
	script := `
		function onKLineClosed(ctx) {
			ctx.indicators["ma:20"];
			ctx.indicators["ma:EMA:20"];
			ctx.indicators["ma:VWMA:20"];
			ctx.indicators["rsi:14"];
			ctx.indicators["macd:12:26:9"];
			ctx.indicators["bollinger:20:2"];
			ctx.indicators["kdj:9:3:3"];
			ctx.indicators["atr:14"];
			ctx.indicators["cci:20"];
			ctx.indicators["williamsr:14"];
			ctx.indicators["divergence:rsi:14:top:5"];
			ctx.indicators["divergence:macd:12:26:9:bottom:6"];
			ctx.indicators["divergence:kdj:9:3:3:top:4"];
		}
	`
	runtime := newIndicatorRuntime(script, types.Interval1m, "US.AAPL")
	if runtime == nil {
		b.Fatal("expected benchmark runtime")
	}
	baseTime := time.Date(2026, 5, 28, 9, 30, 0, 0, time.UTC)
	for index := range minimumIndicatorSeriesLimit + 32 {
		closeValue := 100 + float64(index%41)
		runtime.push(types.KLine{
			Symbol:    "US.AAPL",
			Interval:  types.Interval1m,
			StartTime: types.Time(baseTime.Add(time.Duration(index) * time.Minute)),
			EndTime:   types.Time(baseTime.Add(time.Duration(index+1) * time.Minute)),
			High:      fixedpoint.NewFromFloat(closeValue + 1),
			Low:       fixedpoint.NewFromFloat(closeValue - 1),
			Close:     fixedpoint.NewFromFloat(closeValue),
			Volume:    fixedpoint.NewFromFloat(1000 + float64(index%100)),
		}, market.SessionRegular)
	}
	return runtime
}

func benchmarkProtectSessionIndicatorRuntime(b *testing.B) *indicatorRuntime {
	b.Helper()
	program := indicatorTestProgram(
		&strategyir.ProtectStmt{
			Range:                strategyir.SourceRange{StartLine: 1},
			Direction:            "auto",
			Mode:                 "stopLoss",
			TimeValueExpression:  "2",
			TimeUnit:             "hour",
			PercentageExpression: "2",
			WindowPolicy:         "session",
		},
		&strategyir.ProtectStmt{
			Range:                strategyir.SourceRange{StartLine: 2},
			Direction:            "auto",
			Mode:                 "takeProfit",
			TimeValueExpression:  "2",
			TimeUnit:             "hour",
			PercentageExpression: "3",
			WindowPolicy:         "session",
		},
		&strategyir.ProtectStmt{
			Range:                strategyir.SourceRange{StartLine: 3},
			Direction:            "auto",
			Mode:                 "trailingStop",
			TimeValueExpression:  "2",
			TimeUnit:             "hour",
			PercentageExpression: "1.5",
			WindowPolicy:         "session",
		},
	)
	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		b.Fatalf("PlanRequirements() error = %v", err)
	}
	runtime, err := newIndicatorRuntimeFromPlanWithOptions(plan, types.Interval1m, "US.AAPL", RuntimeOptions{IncludeExtendedHours: true})
	if err != nil {
		b.Fatalf("newIndicatorRuntimeFromPlanWithOptions() error = %v", err)
	}
	if runtime == nil {
		b.Fatal("expected protect benchmark runtime")
	}
	baseTime := time.Date(2026, 5, 28, 9, 30, 0, 0, time.UTC)
	for index := range minimumIndicatorSeriesLimit + 128 {
		closeValue := 100 + math.Sin(float64(index)/11.0)*3 + float64(index%17)/10
		runtime.push(types.KLine{
			Symbol:    "US.AAPL",
			Interval:  types.Interval1m,
			StartTime: types.Time(baseTime.Add(time.Duration(index) * time.Minute)),
			EndTime:   types.Time(baseTime.Add(time.Duration(index+1) * time.Minute)),
			High:      fixedpoint.NewFromFloat(closeValue + 1),
			Low:       fixedpoint.NewFromFloat(closeValue - 1),
			Close:     fixedpoint.NewFromFloat(closeValue),
			Volume:    fixedpoint.NewFromFloat(1000 + float64(index%100)),
		}, market.SessionUnknown)
	}
	return runtime
}

func indicatorTestProgram(statements ...strategyir.Statement) *strategyir.Program {
	return &strategyir.Program{
		SourceFormat: strategypine.SourceFormatPineV6,
		Hooks: []strategyir.HookBlock{{
			Kind:       strategyir.HookKLineClose,
			Statements: statements,
		}},
	}
}

func TestVWAPUsesMarketTradingDayAcrossUTCMidnight(t *testing.T) {
	values := []float64{10, 20}
	volumes := []float64{1, 1}
	endTimes := []time.Time{
		time.Date(2026, time.January, 5, 23, 30, 0, 0, time.UTC),
		time.Date(2026, time.January, 6, 0, 30, 0, 0, time.UTC),
	}
	value, ok := calculateSessionVWAP(values, volumes, endTimes, "US.AAPL", true)
	if !ok || value != 15 {
		t.Fatalf("session VWAP = %v/%v, want 15/true", value, ok)
	}
}

func TestVWAPResetsAtExtendedTradingDayBoundary(t *testing.T) {
	values := []float64{10, 20}
	volumes := []float64{1, 1}
	endTimes := []time.Time{
		time.Date(2026, time.January, 6, 0, 30, 0, 0, time.UTC),
		time.Date(2026, time.January, 6, 1, 30, 0, 0, time.UTC),
	}
	value, ok := calculateSessionVWAP(values, volumes, endTimes, "US.AAPL", true)
	if !ok || value != 20 {
		t.Fatalf("session VWAP = %v/%v, want 20/true after trading-day rollover", value, ok)
	}
}

func TestIndicatorRuntimeVWAPStatesUseMarketTradingPeriods(t *testing.T) {
	runtime := newIndicatorRuntimeWithOptions(`
		function onKLineClosed(ctx) {
			ctx.indicators["vwap:close"];
			ctx.indicators["anchored_vwap:day:close"];
		}
	`, types.Interval1m, "US.AAPL", RuntimeOptions{IncludeExtendedHours: true})
	if runtime == nil {
		t.Fatal("expected indicator runtime")
		return
	}
	push := func(at time.Time, closeValue float64) {
		runtime.push(types.KLine{
			Symbol:    "US.AAPL",
			Interval:  types.Interval1m,
			StartTime: types.Time(at.Add(-time.Minute + time.Millisecond)),
			EndTime:   types.Time(at),
			Open:      fixedpoint.NewFromFloat(closeValue),
			High:      fixedpoint.NewFromFloat(closeValue),
			Low:       fixedpoint.NewFromFloat(closeValue),
			Close:     fixedpoint.NewFromFloat(closeValue),
			Volume:    fixedpoint.NewFromFloat(1),
		}, market.SessionUnknown)
	}

	push(time.Date(2026, time.January, 5, 23, 30, 0, 0, time.UTC), 10)
	push(time.Date(2026, time.January, 6, 0, 30, 0, 0, time.UTC), 20)
	snapshot := runtime.snapshot()
	assertScalarSnapshotApprox(t, snapshot, "vwap:close", 15)
	assertScalarSnapshotApprox(t, snapshot, "anchored_vwap:day:close", 15)

	push(time.Date(2026, time.January, 6, 1, 30, 0, 0, time.UTC), 30)
	snapshot = runtime.snapshot()
	assertScalarSnapshotApprox(t, snapshot, "vwap:close", 30)
	assertScalarSnapshotApprox(t, snapshot, "anchored_vwap:day:close", 30)
}

func TestRollingVWAPStateClearsWhenPeriodCannotBeResolved(t *testing.T) {
	state := &rollingVWAPState{}
	state.push("2026-06-20", 10, 2)
	if value, ok := state.value(); !ok || value != 10 {
		t.Fatalf("state value = %v/%v, want 10/true", value, ok)
	}
	state.push("", 20, 1)
	if value, ok := state.value(); ok {
		t.Fatalf("state value after invalid period = %v/%v, want unavailable", value, ok)
	}
}
