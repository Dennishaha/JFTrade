package indicatorruntime

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/pkg/market"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

func TestParseIndicatorRequirements(t *testing.T) {
	requirements := parseIndicatorRequirements(`
		function onKLineClosed(ctx) {
			const fastAverage = ctx.indicators["ma:5"];
			const slowAverage = ctx.indicators['ma:EMA:20'];
			const dayAverage = ctx.indicators["ma:EMA:20:day"];
			const dailyHlc3 = ctx.indicators["security_source:day:hlc3"];
			const highAverage = ctx.indicators["ma:EMA:5:high"];
			const volumeAverage = ctx.indicators["ma:SMA:20:volume"];
			const volumeWeightedAverage = ctx.indicators["ma:VWMA:10"];
			const latestRsi = ctx.indicators["rsi:14"];
			const latestMacd = ctx.indicators["macd:12:26:9"];
			const latestBollinger = ctx.indicators["bollinger:20:2"];
			const latestKdj = ctx.indicators["kdj:9:3:3"];
			const latestAtr = ctx.indicators["atr:14"];
			const latestStdDev = ctx.indicators["stdev:20"];
			const highestHigh = ctx.indicators["highest:high:20"];
			const lowestLow = ctx.indicators["lowest:low:10"];
			const closeChange = ctx.indicators["change:close:1"];
			const closeMomentum = ctx.indicators["mom:close:5"];
			const closeRoc = ctx.indicators["roc:close:12"];
			const risingClose = ctx.indicators["rising:close:3"];
			const fallingClose = ctx.indicators["falling:close:3"];
			const volumeSum = ctx.indicators["sum:volume:20"];
			const latestCci = ctx.indicators["cci:20"];
			const latestWilliamsR = ctx.indicators["williamsr:14"];
			const sessionStopLoss = ctx.indicators["sl:auto:1:day:10"];
			const sessionTakeProfit = ctx.indicators["risk:takeProfit:auto:2:hour:4:session"];
			const topRsiDivergence = ctx.indicators["divergence:rsi:14:top:5"];
			const bottomMacdDivergence = ctx.indicators["divergence:macd:12:26:9:bottom:6"];
			const topKdjDivergence = ctx.indicators["divergence:kdj:9:3:3:top:4"];
		}
	`)

	if len(requirements.ma) != 6 {
		t.Fatalf("ma requirements = %#v", requirements.ma)
	}
	expectedMAs := map[movingAverageConfig]bool{
		{averageType: "MA", period: 5}:                     true,
		{averageType: "VWMA", period: 10}:                  true,
		{averageType: "EMA", period: 20}:                   true,
		{averageType: "EMA", period: 20, timeUnit: "day"}:  true,
		{averageType: "EMA", period: 5, source: "high"}:    true,
		{averageType: "SMA", period: 20, source: "volume"}: true,
	}
	for _, config := range requirements.ma {
		if !expectedMAs[config] {
			t.Fatalf("unexpected ma config %#v", config)
		}
		delete(expectedMAs, config)
	}
	for config := range expectedMAs {
		t.Fatalf("missing ma config %#v", config)
	}
	if len(requirements.securitySource) != 1 || requirements.securitySource[0] != (securitySourceConfig{source: "hlc3", timeUnit: "day"}) {
		t.Fatalf("security source requirements = %#v", requirements.securitySource)
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
	expectedWindows := map[windowConfig]bool{
		{function: "highest", source: "high", period: 20}: true,
		{function: "lowest", source: "low", period: 10}:   true,
		{function: "change", source: "close", period: 1}:  true,
		{function: "mom", source: "close", period: 5}:     true,
		{function: "roc", source: "close", period: 12}:    true,
		{function: "rising", source: "close", period: 3}:  true,
		{function: "falling", source: "close", period: 3}: true,
		{function: "sum", source: "volume", period: 20}:   true,
	}
	if len(requirements.windows) != len(expectedWindows) {
		t.Fatalf("window requirements = %#v", requirements.windows)
	}
	for _, config := range requirements.windows {
		if !expectedWindows[config] {
			t.Fatalf("unexpected window config %#v", config)
		}
		delete(expectedWindows, config)
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
		return
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
