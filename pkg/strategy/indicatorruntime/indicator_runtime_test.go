package indicatorruntime

import (
	"math"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

var benchmarkSnapshotSink map[string]any
var benchmarkMovingAverageSnapshotSink any

func TestParseIndicatorRequirements(t *testing.T) {
	requirements := parseIndicatorRequirements(`
		function onKLineClosed(ctx) {
			const fastAverage = ctx.indicators["ma:5"];
			const slowAverage = ctx.indicators['ma:EMA:20'];
			const dayAverage = ctx.indicators["ma:EMA:20:day"];
			const volumeWeightedAverage = ctx.indicators["ma:VWMA:10"];
			const latestRsi = ctx.indicators["rsi:14"];
			const latestMacd = ctx.indicators["macd:12:26:9"];
			const latestBollinger = ctx.indicators["bollinger:20:2"];
			const latestKdj = ctx.indicators["kdj:9:3:3"];
			const latestAtr = ctx.indicators["atr:14"];
			const latestStdDev = ctx.indicators["stdev:20"];
			const latestCci = ctx.indicators["cci:20"];
			const latestWilliamsR = ctx.indicators["williamsr:14"];
			const sessionStopLoss = ctx.indicators["sl:auto:1:day:10"];
			const sessionTakeProfit = ctx.indicators["risk:takeProfit:auto:2:hour:4:session"];
			const topRsiDivergence = ctx.indicators["divergence:rsi:14:top:5"];
			const bottomMacdDivergence = ctx.indicators["divergence:macd:12:26:9:bottom:6"];
			const topKdjDivergence = ctx.indicators["divergence:kdj:9:3:3:top:4"];
		}
	`)

	if len(requirements.ma) != 4 {
		t.Fatalf("ma requirements = %#v", requirements.ma)
	}
	if requirements.ma[0] != (movingAverageConfig{averageType: "MA", period: 5}) {
		t.Fatalf("ma[0] = %#v", requirements.ma[0])
	}
	if requirements.ma[1] != (movingAverageConfig{averageType: "VWMA", period: 10}) {
		t.Fatalf("ma[1] = %#v", requirements.ma[1])
	}
	if requirements.ma[2] != (movingAverageConfig{averageType: "EMA", period: 20}) {
		t.Fatalf("ma[2] = %#v", requirements.ma[2])
	}
	if requirements.ma[3] != (movingAverageConfig{averageType: "EMA", period: 20, timeUnit: "day"}) {
		t.Fatalf("ma[3] = %#v", requirements.ma[3])
	}
	if len(requirements.rsi) != 1 || requirements.rsi[0] != 14 {
		t.Fatalf("rsi requirements = %#v", requirements.rsi)
	}
	if len(requirements.macd) != 1 || requirements.macd[0] != (macdConfig{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9}) {
		t.Fatalf("macd requirements = %#v", requirements.macd)
	}
	if len(requirements.bollinger) != 1 || requirements.bollinger[0] != (bollingerConfig{period: 20, multiplier: 2}) {
		t.Fatalf("bollinger requirements = %#v", requirements.bollinger)
	}
	if len(requirements.kdj) != 1 || requirements.kdj[0] != (kdjConfig{period: 9, m1: 3, m2: 3}) {
		t.Fatalf("kdj requirements = %#v", requirements.kdj)
	}
	if len(requirements.atr) != 1 || requirements.atr[0] != 14 {
		t.Fatalf("atr requirements = %#v", requirements.atr)
	}
	if len(requirements.stdev) != 1 || requirements.stdev[0] != 20 {
		t.Fatalf("stdev requirements = %#v", requirements.stdev)
	}
	if len(requirements.cci) != 1 || requirements.cci[0] != 20 {
		t.Fatalf("cci requirements = %#v", requirements.cci)
	}
	if len(requirements.williamsR) != 1 || requirements.williamsR[0] != 14 {
		t.Fatalf("williamsR requirements = %#v", requirements.williamsR)
	}
	if len(requirements.stopLoss) != 2 {
		t.Fatalf("stopLoss requirements = %#v", requirements.stopLoss)
	}
	if requirements.stopLoss[0] != (stopLossConfig{mode: "stopLoss", direction: "auto", timeValue: 1, timeUnit: "day", percentage: 10, windowPolicy: "continuous"}) {
		t.Fatalf("stopLoss[0] = %#v", requirements.stopLoss[0])
	}
	if requirements.stopLoss[1] != (stopLossConfig{mode: "takeProfit", direction: "auto", timeValue: 2, timeUnit: "hour", percentage: 4, windowPolicy: "session"}) {
		t.Fatalf("stopLoss[1] = %#v", requirements.stopLoss[1])
	}
	if len(requirements.rsiDivergence) != 1 || requirements.rsiDivergence[0] != (rsiDivergenceConfig{period: 14, direction: "top", lookback: 5}) {
		t.Fatalf("rsi divergence requirements = %#v", requirements.rsiDivergence)
	}
	if len(requirements.macdDivergence) != 1 || requirements.macdDivergence[0] != (macdDivergenceConfig{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9, direction: "bottom", lookback: 6}) {
		t.Fatalf("macd divergence requirements = %#v", requirements.macdDivergence)
	}
	if len(requirements.kdjDivergence) != 1 || requirements.kdjDivergence[0] != (kdjDivergenceConfig{period: 9, m1: 3, m2: 3, direction: "top", lookback: 4}) {
		t.Fatalf("kdj divergence requirements = %#v", requirements.kdjDivergence)
	}
}

func TestNewIndicatorRuntimeFromPlan(t *testing.T) {
	program := indicatorTestProgram(
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 1}, Name: "fast", Expression: "ma(EMA, 3)"},
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 2}, Name: "signal", Expression: "macd(3, 5, 2)"},
		&strategyir.IfStmt{
			Range:     strategyir.SourceRange{StartLine: 3},
			Condition: "divergence_top(signal, 3)",
			Then:      []strategyir.Statement{&strategyir.NotifyStmt{Range: strategyir.SourceRange{StartLine: 4}, Message: "top"}},
			Else: []strategyir.Statement{&strategyir.ProtectStmt{
				Range:                strategyir.SourceRange{StartLine: 5},
				Direction:            "auto",
				Mode:                 "stop_loss",
				TimeValueExpression:  "2",
				TimeUnit:             "minute",
				PercentageExpression: "1%",
				WindowPolicy:         "continuous",
			}},
		},
	)

	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	runtime, err := newIndicatorRuntimeFromPlan(plan, types.Interval1m, "BTCUSDT")
	if err != nil {
		t.Fatalf("newIndicatorRuntimeFromPlan() error = %v", err)
	}
	if runtime == nil {
		t.Fatal("newIndicatorRuntimeFromPlan() = nil, want runtime")
	}

	for _, closePrice := range []float64{100, 101, 102, 103, 104, 105, 103, 99} {
		runtime.push(types.KLine{
			High:   fixedpoint.NewFromFloat(closePrice + 1),
			Low:    fixedpoint.NewFromFloat(closePrice - 1),
			Close:  fixedpoint.NewFromFloat(closePrice),
			Volume: fixedpoint.NewFromFloat(1000),
		}, market.SessionRegular)
	}

	snapshot := runtime.snapshot()
	if snapshot == nil {
		t.Fatal("snapshot() = nil, want indicator payload")
	}
	if snapshot["ma:EMA:3"] == nil {
		t.Fatalf("snapshot missing ma:EMA:3: %#v", snapshot)
	}
	if snapshot["macd:3:5:2"] == nil {
		t.Fatalf("snapshot missing macd:3:5:2: %#v", snapshot)
	}
	if snapshot["divergence:macd:3:5:2:top:3"] == nil {
		t.Fatalf("snapshot missing divergence:macd:3:5:2:top:3: %#v", snapshot)
	}
	stopLoss, ok := snapshot["sl:auto:2:minute:1"].(map[string]any)
	if !ok {
		t.Fatalf("snapshot stop-loss type = %T", snapshot["sl:auto:2:minute:1"])
	}
	if !readSnapshotBool(t, stopLoss, "triggered") {
		t.Fatal("expected planned stop-loss snapshot to trigger")
	}
}

func TestWarmupBarsFromPlanUsesLargestIndicatorRequirement(t *testing.T) {
	program, err := strategypine.ParseScript(`//@version=6
strategy("Warmup Max", overlay=true)
fast = ta.sma(close, 5)
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 20))
signal = ta.macd(close, 12, 26, 9)
if ta.crossover(fast, signal)
    alert("go")`)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	warmupBars, err := WarmupBarsFromPlanForSymbol(plan, types.Interval1m, "US.AAPL")
	if err != nil {
		t.Fatalf("WarmupBarsFromPlanForSymbol() error = %v", err)
	}

	const want = 20 * tradingSessionMinutesPerDay
	if warmupBars != want {
		t.Fatalf("WarmupBarsFromPlanForSymbol() = %d, want %d", warmupBars, want)
	}
}

func TestWarmupBarsFromPlanForSymbolUsesMarketTradingProfiles(t *testing.T) {
	program, err := strategypine.ParseScript(`//@version=6
strategy("Warmup Markets", overlay=true)
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 20))`)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	testCases := []struct {
		symbol string
		want   int
	}{
		{symbol: "US.AAPL", want: 20 * 390},
		{symbol: "HK.00700", want: 20 * 330},
		{symbol: "SH.600519", want: 20 * 240},
		{symbol: "SZ.000001", want: 20 * 240},
	}

	for _, tt := range testCases {
		warmupBars, warmupErr := WarmupBarsFromPlanForSymbol(plan, types.Interval1m, tt.symbol)
		if warmupErr != nil {
			t.Fatalf("WarmupBarsFromPlanForSymbol(%s) error = %v", tt.symbol, warmupErr)
		}
		if warmupBars != tt.want {
			t.Fatalf("WarmupBarsFromPlanForSymbol(%s) = %d, want %d", tt.symbol, warmupBars, tt.want)
		}
	}
}

func TestWarmupBarsFromPlanForSymbolUsesExtendedTradingDayWhenEnabled(t *testing.T) {
	program, err := strategypine.ParseScript(`//@version=6
strategy("Warmup Extended", overlay=true)
slow = request.security(syminfo.tickerid, "D", ta.sma(close, 5))`)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	warmupBars, err := WarmupBarsFromPlanForSymbolWithOptions(plan, types.Interval1m, "US.AAPL", RuntimeOptions{IncludeExtendedHours: true})
	if err != nil {
		t.Fatalf("WarmupBarsFromPlanForSymbolWithOptions() error = %v", err)
	}

	const want = 5 * 24 * 60
	if warmupBars != want {
		t.Fatalf("WarmupBarsFromPlanForSymbolWithOptions() = %d, want %d", warmupBars, want)
	}
}

func TestWarmupBarsFromPlanDoesNotApplyRuntimeSeriesFloor(t *testing.T) {
	program, err := strategypine.ParseScript(`//@version=6
strategy("Warmup Small", overlay=true)
fast = ta.sma(close, 5)`)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	warmupBars, err := WarmupBarsFromPlan(plan, types.Interval1m)
	if err != nil {
		t.Fatalf("WarmupBarsFromPlan() error = %v", err)
	}

	if warmupBars != 5 {
		t.Fatalf("WarmupBarsFromPlan() = %d, want 5", warmupBars)
	}
	if warmupBars >= minimumIndicatorSeriesLimit {
		t.Fatalf("WarmupBarsFromPlan() = %d, expected no %d-bar runtime floor", warmupBars, minimumIndicatorSeriesLimit)
	}
}

func TestWarmupBarsFromPlanHandlesDivergenceAndProtectLookback(t *testing.T) {
	program := indicatorTestProgram(
		&strategyir.LetStmt{Range: strategyir.SourceRange{StartLine: 1}, Name: "signal", Expression: "rsi(14)"},
		&strategyir.IfStmt{
			Range:     strategyir.SourceRange{StartLine: 2},
			Condition: "divergence_top(signal, 8)",
			Then: []strategyir.Statement{&strategyir.ProtectStmt{
				Range:                strategyir.SourceRange{StartLine: 3},
				Direction:            "auto",
				Mode:                 "stop_loss",
				TimeValueExpression:  "2",
				TimeUnit:             "hour",
				PercentageExpression: "1%",
				WindowPolicy:         "continuous",
			}},
		},
	)

	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}

	warmupBars, err := WarmupBarsFromPlan(plan, types.Interval5m)
	if err != nil {
		t.Fatalf("WarmupBarsFromPlan() error = %v", err)
	}

	const want = 24
	if warmupBars != want {
		t.Fatalf("WarmupBarsFromPlan() = %d, want %d", warmupBars, want)
	}
}

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

func TestCalculateRSISeriesMatchesExpectedValues(t *testing.T) {
	series := calculateRSISeries([]float64{10, 13, 12, 14, 15}, 3)
	assertFloatSliceApproxEqual(t, series, []float64{83.33333333333333, 75})
	if value := calculateRSI([]float64{10, 13, 12, 14, 15}, 3); value.(float64) != series[len(series)-1] {
		t.Fatalf("calculateRSI() = %v, want %v", value, series[len(series)-1])
	}
}

func TestRollingRSIStateMatchesBatchSeriesWithTrim(t *testing.T) {
	state := &rollingRSIState{period: 3, maxLength: 2}
	closes := []float64{10, 13, 12, 14, 15, 14, 16}
	for index, closeValue := range closes {
		if index == 0 {
			state.push(closeValue, 0, false)
			continue
		}
		state.push(closeValue, closes[index-1], true)
	}
	expectedCloses := closes[len(closes)-5:]
	assertFloatSliceApproxEqual(t, state.seriesValues(), calculateRSISeries(expectedCloses, 3))
}

func TestRollingRSIStateMatchesBatchDivergenceWithTrim(t *testing.T) {
	lookback := 3
	state := newRollingRSIState(3, 4, []int{lookback})
	window := make([]float64, 0, 7)
	for _, closeValue := range []float64{10, 13, 12, 14, 15, 14, 16, 15, 17, 18, 16, 19} {
		hasPrevious := len(window) > 0
		previousClose := 0.0
		if hasPrevious {
			previousClose = window[len(window)-1]
		}
		state.push(closeValue, previousClose, hasPrevious)
		window = append(window, closeValue)
		if len(window) > 7 {
			window = window[len(window)-7:]
		}
		expectedSeries := calculateRSISeries(window, 3)
		expectedTail := expectedSeries
		if len(expectedTail) > lookback+1 {
			expectedTail = expectedTail[len(expectedTail)-(lookback+1):]
		}
		assertFloatSliceApproxEqual(t, state.valueTail, expectedTail)
		if actual := state.detectDivergence(window, "top", lookback); actual != detectRSIDivergence(window, expectedSeries, "top", lookback) {
			t.Fatalf("top divergence mismatch after close %v: actual=%v expected=%v", closeValue, actual, detectRSIDivergence(window, expectedSeries, "top", lookback))
		}
		if actual := state.detectDivergence(window, "bottom", lookback); actual != detectRSIDivergence(window, expectedSeries, "bottom", lookback) {
			t.Fatalf("bottom divergence mismatch after close %v: actual=%v expected=%v", closeValue, actual, detectRSIDivergence(window, expectedSeries, "bottom", lookback))
		}
	}
}

func TestCalculateMACDSnapshotMatchesExpectedValues(t *testing.T) {
	snapshot := calculateMACDSnapshot([]float64{1, 2, 3, 4, 5}, macdConfig{fastPeriod: 2, slowPeriod: 3, signalPeriod: 2})
	if snapshot == nil {
		t.Fatal("expected MACD snapshot")
	}
	assertSnapshotNumberApproxEqual(t, snapshot, "diff", 0.4436728395061724)
	assertSnapshotNumberApproxEqual(t, snapshot, "signal", 0.4099794238683127)
	assertSnapshotNumberApproxEqual(t, snapshot, "histogram", 0.0673868312757194)
	assertSnapshotNumberApproxEqual(t, snapshot, "previousDiff", 0.3935185185185186)
	assertSnapshotNumberApproxEqual(t, snapshot, "previousSignal", 0.34259259259259267)
	assertSnapshotNumberApproxEqual(t, snapshot, "previousHistogram", 0.10185185185185186)
}

func TestRollingEMATailStateMatchesBatchSnapshotWithTrim(t *testing.T) {
	config := movingAverageConfig{averageType: "EMA", period: 5}
	state := newRollingEMATailState(config.period, 6, 2)
	cache := newSnapshotSeriesCache()
	window := make([]float64, 0, 6)
	volumes := make([]float64, 0, 6)
	for _, closeValue := range []float64{10, 11, 12, 13, 14, 15, 16, 17, 18, 19} {
		oldFirst := 0.0
		oldSecond := 0.0
		hasOldFirst := len(window) > 0
		hasOldSecond := len(window) > 1
		if hasOldFirst {
			oldFirst = window[0]
		}
		if hasOldSecond {
			oldSecond = window[1]
		}
		trimmed := len(window)+1 > 6
		state.push(closeValue, trimmed, oldFirst, oldSecond, hasOldFirst, hasOldSecond)
		window = append(window, closeValue)
		volumes = append(volumes, 1)
		if len(window) > 6 {
			window = window[len(window)-6:]
			volumes = volumes[len(volumes)-6:]
		}
		current, previous, currentOK, previousOK := state.snapshotValues()
		actual := snapshotToMap(cache.getMovingAverageSnapshot(config, current, previous, currentOK, previousOK), []string{"value", "previous"})
		expected := buildMovingAverageSnapshot(window, volumes, config, 1)
		assertSnapshotMapApproxEqual(t, actual, expected)
	}
}

func TestRollingMACDStateMatchesBatchSnapshotAndDivergenceWithTrim(t *testing.T) {
	config := macdConfig{fastPeriod: 3, slowPeriod: 5, signalPeriod: 2}
	lookback := 3
	state := newRollingMACDState(config, 7, []int{lookback})
	cache := newSnapshotSeriesCache()
	window := make([]float64, 0, 7)
	for _, closeValue := range []float64{10, 11, 12, 13, 12, 14, 16, 15, 17, 19, 18, 20} {
		oldFirst := 0.0
		oldSecond := 0.0
		hasOldFirst := len(window) > 0
		hasOldSecond := len(window) > 1
		if hasOldFirst {
			oldFirst = window[0]
		}
		if hasOldSecond {
			oldSecond = window[1]
		}
		trimmed := len(window)+1 > 7
		state.push(closeValue, trimmed, oldFirst, oldSecond, hasOldFirst, hasOldSecond)
		window = append(window, closeValue)
		if len(window) > 7 {
			window = window[len(window)-7:]
		}
		currentDiff, currentSignal, previousDiff, previousSignal, currentOK, previousOK := state.snapshotValues()
		actualSnapshot := snapshotToMap(cache.getMACDSnapshotValues(config, currentDiff, currentSignal, previousDiff, previousSignal, currentOK, previousOK), []string{"diff", "signal", "histogram", "previousDiff", "previousSignal", "previousHistogram"})
		expectedSnapshot := calculateMACDSnapshot(window, config)
		assertSnapshotMapApproxEqual(t, actualSnapshot, expectedSnapshot)

		expectedSeries := calculateMACDSeries(window, config)
		if actual := state.detectDivergence(window, "top", lookback); actual != detectMACDDivergence(window, expectedSeries.diff, "top", lookback) {
			t.Fatalf("top divergence mismatch after close %v: actual=%v expected=%v", closeValue, actual, detectMACDDivergence(window, expectedSeries.diff, "top", lookback))
		}
		if actual := state.detectDivergence(window, "bottom", lookback); actual != detectMACDDivergence(window, expectedSeries.diff, "bottom", lookback) {
			t.Fatalf("bottom divergence mismatch after close %v: actual=%v expected=%v", closeValue, actual, detectMACDDivergence(window, expectedSeries.diff, "bottom", lookback))
		}
	}
}

func TestRollingKDJStateMatchesBatchSnapshotAndDivergenceWithTrim(t *testing.T) {
	config := kdjConfig{period: 3, m1: 3, m2: 3}
	lookback := 3
	state := newRollingKDJState(config, 7, []int{lookback})
	cache := newSnapshotSeriesCache()
	highWindow := make([]float64, 0, 7)
	lowWindow := make([]float64, 0, 7)
	closeWindow := make([]float64, 0, 7)
	highs := []float64{11, 13, 12, 14, 15, 16, 15, 17, 16, 18, 17, 19}
	lows := []float64{9, 10, 10, 11, 12, 13, 12, 14, 13, 15, 14, 16}
	closes := []float64{10, 12, 11, 13, 14, 15, 13, 16, 14, 17, 15, 18}
	for index := range closes {
		trimmed := len(closeWindow)+1 > 7
		state.push(highWindow, lowWindow, closeWindow, highs[index], lows[index], closes[index], trimmed)
		highWindow = append(highWindow, highs[index])
		lowWindow = append(lowWindow, lows[index])
		closeWindow = append(closeWindow, closes[index])
		if len(closeWindow) > 7 {
			highWindow = highWindow[len(highWindow)-7:]
			lowWindow = lowWindow[len(lowWindow)-7:]
			closeWindow = closeWindow[len(closeWindow)-7:]
		}
		currentK, currentD, currentJ, previousK, previousD, previousJ, currentOK, previousOK := state.snapshotValues()
		actualSnapshot := snapshotToMap(cache.getKDJSnapshotValues(config, currentK, currentD, currentJ, previousK, previousD, previousJ, currentOK, previousOK), []string{"k", "d", "j", "previousK", "previousD", "previousJ"})
		expectedSnapshot := calculateKDJSnapshot(highWindow, lowWindow, closeWindow, config)
		if _, ok := expectedSnapshot["previousK"]; !ok {
			expectedSnapshot["previousK"] = nil
			expectedSnapshot["previousD"] = nil
			expectedSnapshot["previousJ"] = nil
		}
		assertSnapshotMapApproxEqual(t, actualSnapshot, expectedSnapshot)

		_, _, expectedJ := calculateKDJSeries(highWindow, lowWindow, closeWindow, config)
		expectedTail := expectedJ
		if len(expectedTail) > lookback+1 {
			expectedTail = expectedTail[len(expectedTail)-(lookback+1):]
		}
		assertFloatSliceApproxEqual(t, state.jTail, expectedTail)

		if actual := state.detectDivergence(closeWindow, "top", lookback); actual != detectKDJDivergence(closeWindow, expectedJ, "top", lookback) {
			t.Fatalf("top divergence mismatch after close %v: actual=%v expected=%v", closes[index], actual, detectKDJDivergence(closeWindow, expectedJ, "top", lookback))
		}
		if actual := state.detectDivergence(closeWindow, "bottom", lookback); actual != detectKDJDivergence(closeWindow, expectedJ, "bottom", lookback) {
			t.Fatalf("bottom divergence mismatch after close %v: actual=%v expected=%v", closes[index], actual, detectKDJDivergence(closeWindow, expectedJ, "bottom", lookback))
		}
	}
}

func TestCalculateATRSeriesMatchesRollingAverage(t *testing.T) {
	highs := []float64{10, 13, 15, 14}
	lows := []float64{8, 10, 11, 12}
	closes := []float64{9, 12, 13, 13}
	series := calculateATRSeries(highs, lows, closes, 2)
	assertFloatSliceApproxEqual(t, series, []float64{3, 4, 3})
}

func TestRollingATRStateMatchesBatchCurrentValue(t *testing.T) {
	state := &rollingATRState{period: 2}
	highs := []float64{10, 13, 15, 14}
	lows := []float64{8, 10, 11, 12}
	closes := []float64{9, 12, 13, 13}
	for index := range closes {
		state.push(highs[index], lows[index], closes[index], firstOrZero(closes, index-1), index > 0)
	}
	assertOptionalNumberApproxEqual(t, state.value(), calculateATR(highs, lows, closes, 2))
}

func TestCalculateKDJSeriesMatchesExpectedValues(t *testing.T) {
	config := kdjConfig{period: 3, m1: 3, m2: 3}
	highs := []float64{11, 13, 12, 14}
	lows := []float64{9, 10, 10, 11}
	closes := []float64{10, 12, 11, 13}
	kValues, dValues, jValues := calculateKDJSeries(highs, lows, closes, config)
	assertFloatSliceApproxEqual(t, kValues, []float64{50, 58.333333333333336, 55.555555555555564, 62.037037037037045})
	assertFloatSliceApproxEqual(t, dValues, []float64{50, 52.77777777777778, 53.70370370370371, 56.48148148148149})
	assertFloatSliceApproxEqual(t, jValues, []float64{50, 69.44444444444446, 59.25925925925927, 73.14814814814817})
}

func TestCalculateWilliamsRSeriesMatchesExpectedValues(t *testing.T) {
	highs := []float64{11, 12, 13, 14}
	lows := []float64{9, 10, 11, 12}
	closes := []float64{10, 11, 12, 13}
	series := calculateWilliamsRSeries(highs, lows, closes, 3)
	assertFloatSliceApproxEqual(t, series, []float64{-25, -25})
}

func TestRollingBollingerStateMatchesBatchSnapshot(t *testing.T) {
	state := &rollingBollingerState{period: 3, multiplier: 2}
	values := []float64{10, 12, 14, 16}
	for _, value := range values {
		state.push(value)
	}
	assertSnapshotMapApproxEqual(t, state.snapshot(), calculateBollingerSnapshot(values, bollingerConfig{period: 3, multiplier: 2}))
}

func TestRollingStdDevStateMatchesBatchValue(t *testing.T) {
	state := &rollingStdDevState{period: 3}
	values := []float64{10, 12, 14, 16}
	for _, value := range values {
		state.push(value)
	}
	actual, actualOK := state.currentValue()
	expected, expectedOK := calculateStdDev(values, 3)
	if !actualOK || !expectedOK {
		t.Fatalf("stddev ok = (%v, %v), want true", actualOK, expectedOK)
	}
	assertOptionalNumberApproxEqual(t, actual, expected)
}

func TestRollingWilliamsRStateMatchesBatchCurrentValue(t *testing.T) {
	state := &rollingWilliamsRState{period: 3}
	highs := []float64{11, 12, 13, 14}
	lows := []float64{9, 10, 11, 12}
	closes := []float64{10, 11, 12, 13}
	for index := range closes {
		state.push(highs[index], lows[index], closes[index])
	}
	assertOptionalNumberApproxEqual(t, state.value(), calculateWilliamsR(highs, lows, closes, 3))
}

func TestCalculateCCISeriesMatchesExpectedValues(t *testing.T) {
	highs := []float64{105, 108, 112}
	lows := []float64{99, 102, 106}
	closes := []float64{104, 107, 111}
	series := calculateCCISeries(highs, lows, closes, 3)
	assertFloatSliceApproxEqual(t, series, []float64{100})
}

func TestRollingCCIStateMatchesBatchCurrentValue(t *testing.T) {
	state := &rollingCCIState{period: 3}
	highs := []float64{105, 108, 112}
	lows := []float64{99, 102, 106}
	closes := []float64{104, 107, 111}
	for index := range closes {
		state.push((highs[index] + lows[index] + closes[index]) / 3)
	}
	assertOptionalNumberApproxEqual(t, state.value(), calculateCCI(highs, lows, closes, 3))
}

func TestBuildStopLossSnapshot(t *testing.T) {
	snapshot := buildStopLossSnapshot([]float64{100, 99, 98}, nil, nil, stopLossConfig{mode: "stopLoss", direction: "auto", timeValue: 2, timeUnit: "minute", percentage: 1.5, windowPolicy: "continuous"}, 1)
	if snapshot == nil {
		t.Fatal("expected stop-loss snapshot")
	}
	if !readSnapshotBool(t, snapshot, "triggered") {
		t.Fatal("expected stop-loss trigger")
	}
	if !readSnapshotBool(t, snapshot, "longTriggered") {
		t.Fatal("expected long stop-loss trigger")
	}
	if readSnapshotBool(t, snapshot, "shortTriggered") {
		t.Fatal("did not expect short stop-loss trigger")
	}
	if changePercent := readSnapshotNumber(t, snapshot, "changePercent"); changePercent != -2 {
		t.Fatalf("changePercent = %v, want -2", changePercent)
	}
	if triggerPercent := readSnapshotNumber(t, snapshot, "triggerPercent"); triggerPercent != 2 {
		t.Fatalf("triggerPercent = %v, want 2", triggerPercent)
	}
}

func TestBuildStopLossSnapshotSupportsTakeProfitAndTrailingStop(t *testing.T) {
	takeProfit := buildStopLossSnapshot([]float64{100, 101, 103}, nil, nil, stopLossConfig{mode: "takeProfit", direction: "auto", timeValue: 2, timeUnit: "minute", percentage: 2, windowPolicy: "continuous"}, 1)
	if takeProfit == nil {
		t.Fatal("expected take-profit snapshot")
	}
	if !readSnapshotBool(t, takeProfit, "longTriggered") {
		t.Fatal("expected long take-profit trigger")
	}
	if readSnapshotBool(t, takeProfit, "shortTriggered") {
		t.Fatal("did not expect short take-profit trigger")
	}
	if mode := readSnapshotString(t, takeProfit, "mode"); mode != "takeProfit" {
		t.Fatalf("mode = %q, want takeProfit", mode)
	}

	trailing := buildStopLossSnapshot([]float64{100, 110, 107}, nil, nil, stopLossConfig{mode: "trailingStop", direction: "auto", timeValue: 2, timeUnit: "minute", percentage: 2, windowPolicy: "continuous"}, 1)
	if trailing == nil {
		t.Fatal("expected trailing-stop snapshot")
	}
	if !readSnapshotBool(t, trailing, "longTriggered") {
		t.Fatal("expected long trailing-stop trigger")
	}
	if drawdown := readSnapshotNumber(t, trailing, "longDrawdownPercent"); drawdown <= 2 {
		t.Fatalf("longDrawdownPercent = %v, want > 2", drawdown)
	}
}

func TestBuildStopLossSnapshotSupportsSessionAwareWindow(t *testing.T) {
	endTimes := []time.Time{
		time.Date(2026, 5, 27, 13, 29, 59, 0, time.UTC),
		time.Date(2026, 5, 27, 13, 34, 59, 0, time.UTC),
		time.Date(2026, 5, 27, 13, 39, 59, 0, time.UTC),
	}
	sessions := []market.Session{
		market.SessionPre,
		market.SessionRegular,
		market.SessionRegular,
	}
	snapshot := buildStopLossSnapshot([]float64{100, 99, 98}, endTimes, sessions, stopLossConfig{mode: "stopLoss", direction: "auto", timeValue: 10, timeUnit: "minute", percentage: 1, windowPolicy: "session"}, 5)
	if snapshot != nil {
		t.Fatalf("expected session-aware window to reject pre-regular boundary, got %#v", snapshot)
	}
}

func TestBuildStopLossSnapshotUsesRegularTradingWindows(t *testing.T) {
	closes := []float64{100, 80, 90, 85}
	endTimes := []time.Time{
		time.Date(2026, time.May, 28, 19, 59, 59, 0, time.UTC),
		time.Date(2026, time.May, 28, 21, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 14, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 19, 30, 0, 0, time.UTC),
	}
	snapshot := buildStopLossSnapshotForSymbol(closes, endTimes, nil, stopLossConfig{mode: "stopLoss", direction: "auto", timeValue: 2, timeUnit: "day", percentage: 5, windowPolicy: "continuous"}, 1, "US.AAPL")
	if snapshot == nil {
		t.Fatal("expected trading-day stop-loss snapshot")
	}
	if changePercent := readSnapshotNumber(t, snapshot, "changePercent"); changePercent != -15 {
		t.Fatalf("changePercent = %v, want -15", changePercent)
	}
	if !readSnapshotBool(t, snapshot, "longTriggered") {
		t.Fatal("expected stop-loss to ignore extended-hours close and trigger on regular-session window")
	}
	if windowBars := readSnapshotNumber(t, snapshot, "windowBars"); windowBars != 2 {
		t.Fatalf("windowBars = %v, want 2", windowBars)
	}
}

func TestBuildStopLossSnapshotUsesExtendedTradingWindowsWhenEnabled(t *testing.T) {
	closes := []float64{1, 2, 3, 4, 10, 20, 30, 40}
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
	snapshot := buildStopLossSnapshotForSymbolWithOptions(closes, endTimes, nil, stopLossConfig{mode: "stopLoss", direction: "auto", timeValue: 2, timeUnit: "day", percentage: 5, windowPolicy: "continuous"}, 1, "US.AAPL", true)
	if snapshot == nil {
		t.Fatal("expected extended trading-day stop-loss snapshot")
	}
	if changePercent := readSnapshotNumber(t, snapshot, "changePercent"); changePercent != 3900 {
		t.Fatalf("changePercent = %v, want 3900", changePercent)
	}
	if readSnapshotBool(t, snapshot, "longTriggered") {
		t.Fatal("did not expect long stop-loss trigger for extended trading-day window")
	}
	if !readSnapshotBool(t, snapshot, "shortTriggered") {
		t.Fatal("expected short stop-loss trigger for extended trading-day window")
	}
	if windowBars := readSnapshotNumber(t, snapshot, "windowBars"); windowBars != 7 {
		t.Fatalf("windowBars = %v, want 7", windowBars)
	}

	regularSnapshot := buildStopLossSnapshotForSymbol(closes, endTimes, nil, stopLossConfig{mode: "stopLoss", direction: "auto", timeValue: 2, timeUnit: "day", percentage: 5, windowPolicy: "continuous"}, 1, "US.AAPL")
	if regularSnapshot == nil {
		t.Fatal("expected regular trading-day stop-loss snapshot")
	}
	if changePercent := readSnapshotNumber(t, regularSnapshot, "changePercent"); changePercent != 900 {
		t.Fatalf("regular changePercent = %v, want 900", changePercent)
	}
}

func TestIndicatorRuntimeSnapshotIncludesTimeBoundIndicators(t *testing.T) {
	runtime := newIndicatorRuntime(`
		function onKLineClosed(ctx) {
			ctx.indicators["ma:EMA:1:hour"];
			ctx.indicators["sl:auto:1:hour:2"];
			ctx.indicators["risk:takeProfit:auto:1:hour:2:continuous"];
			ctx.indicators["divergence:rsi:3:top:3"];
		}
	`, types.Interval5m, "BTCUSDT")
	if runtime == nil {
		t.Fatal("expected indicator runtime")
	}

	for _, closePrice := range []float64{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 97} {
		runtime.push(types.KLine{
			High:   fixedpoint.NewFromFloat(closePrice + 1),
			Low:    fixedpoint.NewFromFloat(closePrice - 1),
			Close:  fixedpoint.NewFromFloat(closePrice),
			Volume: fixedpoint.NewFromFloat(1000),
		}, market.SessionRegular)
	}

	snapshot := runtime.snapshot()
	if snapshot == nil {
		t.Fatal("expected runtime snapshot")
	}
	if snapshot["ma:EMA:1:hour"] == nil {
		t.Fatalf("expected time-bound MA snapshot, got %#v", snapshot)
	}
	stopLoss, ok := snapshot["sl:auto:1:hour:2"].(map[string]any)
	if !ok {
		t.Fatalf("stop loss snapshot type = %T", snapshot["sl:auto:1:hour:2"])
	}
	if !readSnapshotBool(t, stopLoss, "longTriggered") {
		t.Fatalf("expected long stop loss trigger, got %#v", stopLoss)
	}
	takeProfit, ok := snapshot["risk:takeProfit:auto:1:hour:2:continuous"].(map[string]any)
	if !ok {
		t.Fatalf("take profit snapshot type = %T", snapshot["risk:takeProfit:auto:1:hour:2:continuous"])
	}
	if mode := readSnapshotString(t, takeProfit, "mode"); mode != "takeProfit" {
		t.Fatalf("take profit mode = %q, want takeProfit", mode)
	}
	if _, ok := snapshot["divergence:rsi:3:top:3"].(bool); !ok {
		t.Fatalf("expected divergence snapshot bool, got %T", snapshot["divergence:rsi:3:top:3"])
	}
}

func TestIndicatorEngineSnapshotReturnsIndependentMap(t *testing.T) {
	program, err := strategypine.ParseScript(`//@version=6
strategy("Snapshot", overlay=true)
momentum = ta.rsi(close, 2)`)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}
	plan, err := strategyir.PlanRequirements(program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}
	engine, err := NewIndicatorEngineForPlan(plan, types.Interval1m, "US.AAPL")
	if err != nil {
		t.Fatalf("NewIndicatorEngineForPlan() error = %v", err)
	}
	if engine == nil {
		t.Fatal("expected indicator engine")
	}

	pushClose := func(closeValue float64) {
		engine.Push(types.KLine{
			Symbol:   "US.AAPL",
			Interval: types.Interval1m,
			High:     fixedpoint.NewFromFloat(closeValue + 1),
			Low:      fixedpoint.NewFromFloat(closeValue - 1),
			Close:    fixedpoint.NewFromFloat(closeValue),
			Volume:   fixedpoint.NewFromFloat(1000),
		}, market.SessionRegular)
	}

	pushClose(100)
	pushClose(101)
	pushClose(103)
	firstSnapshot := engine.Snapshot()
	firstRSI, ok := firstSnapshot["rsi:2"].(float64)
	if !ok {
		t.Fatalf("first snapshot rsi type = %T", firstSnapshot["rsi:2"])
	}

	pushClose(99)
	secondSnapshot := engine.Snapshot()
	secondRSI, ok := secondSnapshot["rsi:2"].(float64)
	if !ok {
		t.Fatalf("second snapshot rsi type = %T", secondSnapshot["rsi:2"])
	}
	if firstRSI == secondRSI {
		t.Fatalf("expected independent snapshots with different RSI values, both = %v", firstRSI)
	}
	if current, ok := firstSnapshot["rsi:2"].(float64); !ok || current != firstRSI {
		t.Fatalf("first snapshot mutated after second snapshot: %#v", firstSnapshot)
	}
	secondSnapshot["manual"] = true
	if _, ok := firstSnapshot["manual"]; ok {
		t.Fatalf("first snapshot unexpectedly shared outer map with second snapshot: %#v", firstSnapshot)
	}
	borrowedSnapshot := engine.SnapshotBorrowed()
	if borrowedSnapshot == nil {
		t.Fatal("expected borrowed snapshot")
	}
	borrowedRSI, ok := borrowedSnapshot["rsi:2"].(interface{ ScalarValue() (float64, bool) })
	if !ok {
		t.Fatalf("borrowed snapshot rsi type = %T", borrowedSnapshot["rsi:2"])
	}
	borrowedValue, borrowedValueOK := borrowedRSI.ScalarValue()
	if !borrowedValueOK || borrowedValue != secondRSI {
		t.Fatalf("borrowed snapshot rsi = (%v, %v), want (%v, true)", borrowedValue, borrowedValueOK, secondRSI)
	}
}

func TestDetectDivergence(t *testing.T) {
	if !detectDivergence([]float64{10, 11, 12, 13}, []float64{60, 65, 63, 61}, "top", 3) {
		t.Fatal("expected top divergence to be detected")
	}
	if !detectDivergence([]float64{10, 9, 8, 7}, []float64{40, 35, 37, 39}, "bottom", 3) {
		t.Fatal("expected bottom divergence to be detected")
	}
	if detectDivergence([]float64{10, 11, 12, 13}, []float64{60, 62, 64, 66}, "top", 3) {
		t.Fatal("did not expect divergence when indicator confirms price")
	}
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
		config := config
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
	for index := 0; index < minimumIndicatorSeriesLimit+32; index++ {
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
	for index := 0; index < minimumIndicatorSeriesLimit+128; index++ {
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
