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

func TestCalculateRSISeriesMatchesExpectedValues(t *testing.T) {
	series := calculateRSISeries([]float64{10, 13, 12, 14, 15}, 3)
	assertFloatSliceApproxEqual(t, series, []float64{83.33333333333333, 75})
	if value := calculateRSI([]float64{10, 13, 12, 14, 15}, 3); jftradeCheckedTypeAssertion[float64](value) != series[len(series)-1] {
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
	if state == nil {
		t.Fatal("expected rolling RSI state")
	}
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
	if state == nil {
		t.Fatal("expected rolling KDJ state")
	}
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

func TestIndicatorRuntimeSnapshotIncludesSAR(t *testing.T) {
	runtime := newIndicatorRuntime(`
		function onKLineClosed(ctx) {
			ctx.indicators["sar:0.02:0.02:0.2"];
		}
	`, types.Interval1m, "US.AAPL")
	if runtime == nil {
		t.Fatal("expected indicator runtime")
	}
	for _, bar := range []struct {
		high  float64
		low   float64
		close float64
	}{
		{high: 10, low: 9, close: 9.5},
		{high: 11, low: 10, close: 10.5},
		{high: 12, low: 11, close: 11.5},
		{high: 13, low: 12, close: 12.5},
		{high: 14, low: 13, close: 13.5},
	} {
		runtime.push(types.KLine{
			High:   fixedpoint.NewFromFloat(bar.high),
			Low:    fixedpoint.NewFromFloat(bar.low),
			Close:  fixedpoint.NewFromFloat(bar.close),
			Volume: fixedpoint.NewFromFloat(1000),
		}, market.SessionRegular)
	}
	snapshot := runtime.snapshot()
	sar, ok := snapshot["sar:0.02:0.02:0.2"].(*indicatorSeriesSnapshot)
	if !ok || sar == nil {
		t.Fatalf("sar snapshot = %#v", snapshot["sar:0.02:0.02:0.2"])
	}
	if !sar.hasCurrent || math.Abs(sar.current-9.3528) > 0.0000001 {
		t.Fatalf("sar.current = %v, want 9.3528", sar.current)
	}
	if !sar.hasPrevious || math.Abs(sar.previous-9.12) > 0.0000001 {
		t.Fatalf("sar.previous = %v, want 9.12", sar.previous)
	}
}

func TestIndicatorRuntimeSnapshotIncludesSecuritySource(t *testing.T) {
	runtime := newIndicatorRuntime(`
		function onKLineClosed(ctx) {
			ctx.indicators["security_source:day:close"];
			ctx.indicators["security_source:day:hlc3"];
		}
	`, types.Interval1m, "US.AAPL")
	if runtime == nil {
		t.Fatal("expected indicator runtime")
	}
	bars := []struct {
		at     time.Time
		open   float64
		high   float64
		low    float64
		close  float64
		volume float64
	}{
		{at: time.Date(2026, time.June, 11, 14, 30, 0, 0, time.UTC), open: 10, high: 12, low: 8, close: 11, volume: 100},
		{at: time.Date(2026, time.June, 11, 14, 31, 0, 0, time.UTC), open: 11, high: 16, low: 9, close: 14, volume: 200},
		{at: time.Date(2026, time.June, 12, 14, 30, 0, 0, time.UTC), open: 20, high: 22, low: 18, close: 21, volume: 300},
		{at: time.Date(2026, time.June, 12, 14, 31, 0, 0, time.UTC), open: 21, high: 25, low: 19, close: 24, volume: 400},
	}
	for _, bar := range bars {
		runtime.push(types.KLine{
			Symbol:    "US.AAPL",
			Interval:  types.Interval1m,
			StartTime: types.Time(bar.at),
			EndTime:   types.Time(bar.at.Add(time.Minute - time.Millisecond)),
			Open:      fixedpoint.NewFromFloat(bar.open),
			High:      fixedpoint.NewFromFloat(bar.high),
			Low:       fixedpoint.NewFromFloat(bar.low),
			Close:     fixedpoint.NewFromFloat(bar.close),
			Volume:    fixedpoint.NewFromFloat(bar.volume),
		}, market.SessionRegular)
	}

	snapshot := runtime.snapshot()
	assertSeriesSnapshot(t, snapshot, "security_source:day:close", 24, 14)
	assertSeriesSnapshotApprox(t, snapshot, "security_source:day:hlc3", (25+18+24)/3.0, (16+8+14)/3.0)
}

func TestIndicatorRuntimeSnapshotIncludesIntradaySecurityTimeframes(t *testing.T) {
	runtime := newIndicatorRuntime(`
		function onKLineClosed(ctx) {
			ctx.indicators["security_source:15m:close"];
			ctx.indicators["security_source:15m:close:1"];
			ctx.indicators["security_source:30m:hlc3"];
			ctx.indicators["ma:EMA:3:15m:hlc3"];
			ctx.indicators["linreg:close:3:0:15m"];
			ctx.indicators["obv:close:15m"];
			ctx.indicators["alma:close:3:0.85:6:15m"];
			ctx.indicators["cmo:close:3:15m"];
			ctx.indicators["correlation:close:high:3:15m"];
			ctx.indicators["percentile_nearest_rank:close:3:80:15m"];
			ctx.indicators["swma:close:15m"];
			ctx.indicators["stoch:close:3:15m"];
		}
	`, types.Interval1m, "US.AAPL")
	if runtime == nil {
		t.Fatal("expected indicator runtime")
	}
	base := time.Date(2026, time.June, 12, 14, 30, 0, 0, time.UTC)
	for index := range 60 {
		closePrice := float64(index + 1)
		start := base.Add(time.Duration(index) * time.Minute)
		runtime.push(types.KLine{
			Symbol:    "US.AAPL",
			Interval:  types.Interval1m,
			StartTime: types.Time(start),
			EndTime:   types.Time(start.Add(time.Minute - time.Millisecond)),
			Open:      fixedpoint.NewFromFloat(closePrice),
			High:      fixedpoint.NewFromFloat(closePrice + 1),
			Low:       fixedpoint.NewFromFloat(closePrice - 1),
			Close:     fixedpoint.NewFromFloat(closePrice),
			Volume:    fixedpoint.NewFromFloat(1000),
		}, market.SessionRegular)
	}

	snapshot := runtime.snapshot()
	assertSeriesSnapshot(t, snapshot, "security_source:15m:close", 60, 45)
	assertSeriesSnapshot(t, snapshot, "security_source:15m:close:1", 45, 30)
	assertSeriesSnapshotApprox(t, snapshot, "security_source:30m:hlc3", (61+30+60)/3.0, (31+0+30)/3.0)
	assertSeriesSnapshotApprox(t, snapshot, "ma:EMA:3:15m:hlc3", 42.208333333333336, 29.083333333333336)
	linregSnapshot, ok := snapshot["linreg:close:3:0:15m"].(interface {
		ScalarValue() (float64, bool)
	})
	if !ok {
		t.Fatalf("MTF linreg snapshot type = %T", snapshot["linreg:close:3:0:15m"])
	}
	if value, valueOK := linregSnapshot.ScalarValue(); !valueOK || math.Abs(value-60) > 1e-9 {
		t.Fatalf("MTF linreg value = (%v, %v), want (60, true)", value, valueOK)
	}
	assertSeriesSnapshot(t, snapshot, "obv:close:15m", 45000, 30000)
	if _, ok := snapshot["alma:close:3:0.85:6:15m"].(interface {
		ScalarValue() (float64, bool)
	}); !ok {
		t.Fatalf("MTF ALMA snapshot type = %T", snapshot["alma:close:3:0.85:6:15m"])
	}
	assertScalarSnapshotApprox(t, snapshot, "cmo:close:3:15m", 100)
	assertScalarSnapshotApprox(t, snapshot, "correlation:close:high:3:15m", 1)
	assertScalarSnapshotApprox(t, snapshot, "percentile_nearest_rank:close:3:80:15m", 60)
	assertScalarSnapshotApprox(t, snapshot, "swma:close:15m", 37.5)
	assertSeriesSnapshotApprox(t, snapshot, "stoch:close:3:15m", 100.0*(60-15)/(61-15), 100.0*(45-0)/(46-0))
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

func TestIndicatorEngineComputesRollingWindowFunctions(t *testing.T) {
	program, err := strategypine.ParseScript(`//@version=6
strategy("Windows", overlay=true)
hh = ta.highest(high, 3)
ll = ta.lowest(low, 3)
delta = ta.change(close)
momentum = ta.mom(close, 2)
rate = ta.roc(close, 2)
up = ta.rising(close, 2)
down = ta.falling(low, 2)
avgVol = ta.sma(volume, 2)
emaHigh = ta.ema(high, 2)
volSum = ta.sum(volume, 2)`)
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
	baseTime := time.Date(2026, time.June, 13, 9, 30, 0, 0, time.UTC)
	bars := []struct {
		high   float64
		low    float64
		close  float64
		volume float64
	}{
		{high: 10, low: 7, close: 100, volume: 100},
		{high: 12, low: 6, close: 105, volume: 200},
		{high: 11, low: 5, close: 102, volume: 300},
		{high: 13, low: 4, close: 110, volume: 400},
	}
	for index, bar := range bars {
		start := baseTime.Add(time.Duration(index) * time.Minute)
		engine.Push(types.KLine{
			Symbol:    "US.AAPL",
			Interval:  types.Interval1m,
			StartTime: types.Time(start),
			EndTime:   types.Time(start.Add(time.Minute - time.Millisecond)),
			Open:      fixedpoint.NewFromFloat(bar.close),
			High:      fixedpoint.NewFromFloat(bar.high),
			Low:       fixedpoint.NewFromFloat(bar.low),
			Close:     fixedpoint.NewFromFloat(bar.close),
			Volume:    fixedpoint.NewFromFloat(bar.volume),
		}, market.SessionRegular)
	}
	snapshot := engine.SnapshotBorrowed()
	assertSeriesSnapshot(t, snapshot, "highest:high:3", 13, 12)
	assertSeriesSnapshot(t, snapshot, "lowest:low:3", 4, 5)
	assertSeriesSnapshot(t, snapshot, "change:close:1", 8, -3)
	assertSeriesSnapshot(t, snapshot, "mom:close:2", 5, 2)
	assertSeriesSnapshotApprox(t, snapshot, "roc:close:2", (110-105)/105.0*100, (102-100)/100.0*100)
	assertSeriesSnapshot(t, snapshot, "ma:SMA:2:volume", 350, 250)
	assertSeriesSnapshotApprox(t, snapshot, "ma:EMA:2:high", 12.37037037037037, 11.11111111111111)
	assertSeriesSnapshot(t, snapshot, "sum:volume:2", 700, 500)
	if value, ok := snapshot["rising:close:2"].(bool); !ok || !value {
		t.Fatalf("rising snapshot = %#v, want true", snapshot["rising:close:2"])
	}
	if value, ok := snapshot["falling:low:2"].(bool); !ok || !value {
		t.Fatalf("falling snapshot = %#v, want true", snapshot["falling:low:2"])
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
