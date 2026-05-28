package indicatorruntime

import (
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	"github.com/jftrade/jftrade-main/pkg/futu"
	strategydsl "github.com/jftrade/jftrade-main/pkg/strategy/dsl"
	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

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
	script := `on kline_close:
  let fast = ma(EMA, 3)
  let signal = macd(3, 5, 2)
  if divergence_top(signal, 3):
    notify "top"
  else:
    protect auto stop_loss 2 minute 1%`

	program, err := strategydsl.ParseScript(script)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}

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

	for _, closePrice := range []float64{100, 101, 103, 99} {
		runtime.push(types.KLine{
			High:   fixedpoint.NewFromFloat(closePrice + 1),
			Low:    fixedpoint.NewFromFloat(closePrice - 1),
			Close:  fixedpoint.NewFromFloat(closePrice),
			Volume: fixedpoint.NewFromFloat(1000),
		}, futu.MarketSessionRegular)
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
	sessions := []futu.MarketSession{
		futu.MarketSessionPre,
		futu.MarketSessionRegular,
		futu.MarketSessionRegular,
	}
	snapshot := buildStopLossSnapshot([]float64{100, 99, 98}, endTimes, sessions, stopLossConfig{mode: "stopLoss", direction: "auto", timeValue: 10, timeUnit: "minute", percentage: 1, windowPolicy: "session"}, 5)
	if snapshot != nil {
		t.Fatalf("expected session-aware window to reject pre-regular boundary, got %#v", snapshot)
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
		}, futu.MarketSessionRegular)
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
