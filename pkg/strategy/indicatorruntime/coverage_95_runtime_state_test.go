package indicatorruntime

import (
	"math"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/market"
)

func TestCoverage95StateFactoriesRejectInvalidConfigurations(t *testing.T) {
	if states := newRollingKDJStates(indicatorRequirements{}, 2); states != nil {
		t.Fatalf("empty KDJ state set = %#v", states)
	}
	if states := newRollingKDJStates(indicatorRequirements{kdj: []kdjConfig{{period: 0, m1: 2, m2: 2}}}, 2); states != nil {
		t.Fatalf("invalid KDJ state set = %#v", states)
	}
	kdjStates := newRollingKDJStates(indicatorRequirements{kdjDivergence: []kdjDivergenceConfig{{period: 2, m1: 2, m2: 2, lookback: 3}}}, 2)
	if len(kdjStates) != 1 {
		t.Fatalf("KDJ divergence states = %#v", kdjStates)
	}

	if states := newRollingMACDStates(indicatorRequirements{}, 2); states != nil {
		t.Fatalf("empty MACD state set = %#v", states)
	}
	if states := newRollingMACDStates(indicatorRequirements{macd: []macdConfig{{fastPeriod: 0, slowPeriod: 2, signalPeriod: 1}}}, 2); states != nil {
		t.Fatalf("invalid MACD state set = %#v", states)
	}
	macdStates := newRollingMACDStates(indicatorRequirements{macdDivergence: []macdDivergenceConfig{{fastPeriod: 1, slowPeriod: 2, signalPeriod: 2, lookback: 3}}}, 2)
	if len(macdStates) != 1 {
		t.Fatalf("MACD divergence states = %#v", macdStates)
	}

	if states := newRollingEMAStates(indicatorRequirements{}, 1, 2); states != nil {
		t.Fatalf("empty EMA state set = %#v", states)
	}
	if states := newRollingEMAStates(indicatorRequirements{ma: []movingAverageConfig{{averageType: "SMA", period: 2}, {averageType: "EMA", period: 2, timeUnit: "5m"}}}, 1, 2); states != nil {
		t.Fatalf("unsupported EMA state set = %#v", states)
	}
	emaStates := newRollingEMAStates(indicatorRequirements{ma: []movingAverageConfig{{averageType: "EMA", period: 2}}}, 1, 2)
	if len(emaStates) != 1 {
		t.Fatalf("EMA states = %#v", emaStates)
	}

	if states := newRollingMovingAverageStates(indicatorRequirements{}, 1); states != nil {
		t.Fatalf("empty moving-average state set = %#v", states)
	}
	if states := newRollingMovingAverageStates(indicatorRequirements{ma: []movingAverageConfig{{averageType: "EMA", period: 2}, {averageType: "SMA", period: 2, timeUnit: "5m"}}}, 1); states != nil {
		t.Fatalf("unsupported moving-average state set = %#v", states)
	}
	maStates := newRollingMovingAverageStates(indicatorRequirements{ma: []movingAverageConfig{{averageType: "VWMA", period: 2, source: "close"}}}, 1)
	if len(maStates) != 1 {
		t.Fatalf("moving-average states = %#v", maStates)
	}

	if states := newRollingStochStates(indicatorRequirements{}); states != nil {
		t.Fatalf("empty stoch state set = %#v", states)
	}
	if states := newRollingStochStates(indicatorRequirements{stoch: []sourcePeriodConfig{{source: "volume", period: 2}}}); states != nil {
		t.Fatalf("invalid stoch state set = %#v", states)
	}
}

func TestCoverage95RollingKDJStateRemainsCoherentAfterSeriesTrim(t *testing.T) {
	if state := newRollingKDJState(kdjConfig{period: 0, m1: 2, m2: 2}, 2, nil); state != nil {
		t.Fatalf("invalid KDJ state = %#v", state)
	}
	state := newRollingKDJState(kdjConfig{period: 2, m1: 2, m2: 2}, 2, []int{3})
	if state == nil || state.limit != 2 || state.tailLen < 4 {
		t.Fatalf("KDJ state = %#v", state)
		return
	}
	state.push(nil, nil, nil, 4, 1, 3, false)
	state.push([]float64{4}, []float64{1}, []float64{3}, 5, 2, 4, false)
	state.push([]float64{4, 5}, []float64{1, 2}, []float64{3, 4}, 6, 3, 5, true)
	if len(state.kTail) == 0 || state.windowLen != state.limit {
		t.Fatalf("trimmed KDJ state = %#v", state)
	}
	if _, _, _, _, _, _, currentOK, _ := state.snapshotValues(); !currentOK {
		t.Fatalf("trimmed KDJ snapshot unavailable: %#v", state)
	}
	if state.boundaryKAt(-1) != 0 || state.boundaryDByKAt(-1) != 0 || state.boundaryDByDAt(-1) != 0 {
		t.Fatal("negative KDJ boundary lookup did not return zero")
	}
	if state.boundaryKAt(20) == 0 || state.boundaryDByKAt(20) == 0 || state.boundaryDByDAt(20) == 0 {
		t.Fatal("extended KDJ boundary lookup did not calculate a positive decay")
	}

	short := newRollingKDJState(kdjConfig{period: 2, m1: 2, m2: 2}, 2, nil)
	short.windowLen = 1
	short.kTail = append(short.kTail, 50)
	short.dTail = append(short.dTail, 50)
	short.jTail = append(short.jTail, 50)
	short.prefixK = append(short.prefixK, 50)
	short.prefixD = append(short.prefixD, 50)
	short.prefixJ = append(short.prefixJ, 50)
	short.trimState([]float64{3}, []float64{1}, []float64{2})
	if len(short.kTail) != 0 || len(short.prefixK) != 0 {
		t.Fatalf("short KDJ trim retained stale series: %#v", short)
	}
	var nilState *rollingKDJState
	nilState.push(nil, nil, nil, 0, 0, 0, false)
	if _, _, _, _, _, _, currentOK, previousOK := nilState.snapshotValues(); currentOK || previousOK {
		t.Fatal("nil KDJ state exposed snapshot values")
	}
}

func TestCoverage95RollingEmaMacdAndMovingAverageBoundaryValues(t *testing.T) {
	if state := newRollingEMATailState(0, 2, 2); state != nil {
		t.Fatalf("invalid EMA tail state = %#v", state)
	}
	ema := newRollingEMATailState(2, 2, 2)
	ema.push(1, false, 0, 0, false, false)
	ema.push(3, false, 0, 0, false, false)
	ema.push(5, true, 1, 3, true, true)
	if current, _, currentOK, _ := ema.snapshotValues(); !currentOK || current <= 0 {
		t.Fatalf("trimmed EMA current value is unavailable: %#v", ema)
	}
	ema.push(7, true, 0, 0, false, false)
	if len(ema.tail) != 1 || ema.windowLen != 1 {
		t.Fatalf("EMA reset after insufficient trim context = %#v", ema)
	}
	if ema.powerAt(-1) != 0 || ema.powerAt(30) <= 0 {
		t.Fatalf("EMA power lookup invalid: negative=%v extended=%v", ema.powerAt(-1), ema.powerAt(30))
	}

	if state := newRollingMACDState(macdConfig{}, 2, nil); state != nil {
		t.Fatalf("invalid MACD state = %#v", state)
	}
	macd := newRollingMACDState(macdConfig{fastPeriod: 1, slowPeriod: 2, signalPeriod: 1}, 3, []int{2})
	for _, value := range []float64{1, 2, 4} {
		macd.push(value, false, 0, 0, false, false)
	}
	if _, _, _, _, currentOK, _ := macd.snapshotValues(); !currentOK {
		t.Fatalf("MACD snapshot unavailable after warmup: %#v", macd)
	}
	macd.push(5, true, 1, 2, true, true)
	if _, _, _, _, currentOK, _ := macd.snapshotValues(); !currentOK {
		t.Fatalf("MACD snapshot unavailable after a shifted three-bar window: %#v", macd)
	}
	if signal := macd.currentSignal(); math.IsNaN(signal) || math.IsInf(signal, 0) {
		t.Fatalf("MACD signal = %v", signal)
	}
	if _, ok := macd.previousSignal(); !ok {
		t.Fatalf("MACD previous signal unavailable: %#v", macd)
	}
	var nilMACD *rollingMACDState
	nilMACD.push(1, false, 0, 0, false, false)
	if value, ok := nilMACD.currentDiff(); ok || value != 0 {
		t.Fatalf("nil MACD diff = (%v, %v)", value, ok)
	}
	if value, ok := nilMACD.previousDiff(); ok || value != 0 {
		t.Fatalf("nil MACD previous diff = (%v, %v)", value, ok)
	}

	ma := &rollingMovingAverageSnapshotState{kind: "VWMA", period: 2}
	ma.push(10, 0)
	ma.push(20, 0)
	if ma.hasCurrent {
		t.Fatalf("zero-volume VWMA became available: %#v", ma)
	}
	ma.push(30, 3)
	if !ma.hasCurrent || ma.current != 30 {
		t.Fatalf("VWMA did not recover with positive volume: %#v", ma)
	}
	if snapshot := ma.snapshot(); snapshot == nil || snapshot["value"] != 30.0 {
		t.Fatalf("moving-average map snapshot = %#v", snapshot)
	}
	if value, ok := ma.FieldValue("previous"); !ok || value != nil {
		t.Fatalf("moving-average previous field = %#v/%v", value, ok)
	}
	if _, ok := ma.FieldValue("unsupported"); ok {
		t.Fatal("moving-average accepted unsupported field")
	}
}

func TestCoverage95RuntimeHelpersCoverTimeframesAndSecurityValues(t *testing.T) {
	var nilRuntime *indicatorRuntime
	if values := nilRuntime.oldSourceValuesAt(0); len(values) != 0 {
		t.Fatalf("nil old source values = %#v", values)
	}
	runtime := &indicatorRuntime{
		symbol:          "US.AAPL",
		intervalMinutes: 1,
		opens:           []float64{10, 20, 30},
		highs:           []float64{12, 22, 32},
		lows:            []float64{8, 18, 28},
		closes:          []float64{11, 21, 31},
		volumes:         []float64{1, 2, 3},
	}
	if values := runtime.oldSourceValuesAt(-1); len(values) != 0 {
		t.Fatalf("negative old source values = %#v", values)
	}
	values := runtime.oldSourceValuesAt(1)
	if values["hl2"] != 20 || values["hlc3"] != (22+18+21)/3.0 || values["ohlc4"] != (20+22+18+21)/4.0 {
		t.Fatalf("derived old source values = %#v", values)
	}
	if got := appendTailValue([]float64{1, 2}, 3, 0); len(got) != 0 {
		t.Fatalf("zero-limit tail = %#v", got)
	}
	if got := appendTailValue([]float64{1, 2}, 3, 2); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("full tail append = %#v", got)
	}
	if got := sortedLookbacks(map[int]struct{}{3: {}, -1: {}, 1: {}}); len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("sorted lookbacks = %#v", got)
	}
	if got := runtime.seriesForSource("ohlc4"); len(got) != 3 || got[0] != 10.25 {
		t.Fatalf("OHLC4 source series = %#v", got)
	}

	current, previous, currentOK, previousOK := runtime.securitySourceSnapshotValues(securitySourceConfig{source: "close"}, nil)
	if !currentOK || !previousOK || current != 31 || previous != 21 {
		t.Fatalf("bar security snapshot = (%v, %v, %v, %v)", current, previous, currentOK, previousOK)
	}
	current, previous, currentOK, previousOK = runtime.securitySourceSnapshotValues(securitySourceConfig{source: "close", timeUnit: "2m"}, nil)
	if !currentOK || !previousOK || current != 31 || previous != 21 {
		t.Fatalf("fixed-timeframe security snapshot = (%v, %v, %v, %v)", current, previous, currentOK, previousOK)
	}
	if fixed, volumes, ok := runtime.fixedTimeframeSeries("2m", "volume"); !ok || len(fixed) != 2 || fixed[0] != 3 || fixed[1] != 3 || len(volumes) != 2 {
		t.Fatalf("fixed-timeframe series = %#v/%#v/%v", fixed, volumes, ok)
	}
	if fixed, _, ok := runtime.fixedTimeframeSeries("bad", "close"); ok || fixed != nil {
		t.Fatalf("invalid fixed-timeframe series = %#v/%v", fixed, ok)
	}

	runtime.endTimes = []time.Time{
		time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.May, 29, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.June, 1, 15, 0, 0, 0, time.UTC),
	}
	runtime.tradingPeriodLabels = map[string][]int64{"day": buildTradingPeriodLabels(nil, runtime.endTimes, runtime.symbol, "day", false)}
	current, previous, currentOK, previousOK = runtime.securitySourceSnapshotValues(securitySourceConfig{source: "close", timeUnit: "day"}, newSnapshotSeriesCache())
	if !currentOK || !previousOK || current != 31 || previous != 21 {
		t.Fatalf("trading-period security snapshot = (%v, %v, %v, %v)", current, previous, currentOK, previousOK)
	}
	if !hasUsableEndTimes(runtime.endTimes) || hasUsableEndTimes([]time.Time{}) {
		t.Fatal("usable end-time detection failed")
	}
	if got := derivedSeriesHL2([]float64{4, 6}, []float64{2}); len(got) != 1 || got[0] != 3 {
		t.Fatalf("derived HL2 = %#v", got)
	}
	if got := derivedSeriesHLC3([]float64{4}, []float64{2}, []float64{3}); len(got) != 1 || got[0] != 3 {
		t.Fatalf("derived HLC3 = %#v", got)
	}
	if got := derivedSeriesOHLC4([]float64{2}, []float64{4}, []float64{0}, []float64{2}); len(got) != 1 || got[0] != 2 {
		t.Fatalf("derived OHLC4 = %#v", got)
	}
}

func TestCoverage95RuntimePushTrimsAlignedSeriesAndStopLossModes(t *testing.T) {
	runtime := &indicatorRuntime{
		symbol:              "US.AAPL",
		intervalMinutes:     1,
		seriesLimit:         2,
		tradingPeriodUnits:  []string{"day"},
		tradingPeriodLabels: map[string][]int64{"day": nil},
		snapshotCache:       newSnapshotSeriesCache(),
	}
	base := time.Date(2026, time.May, 28, 15, 0, 0, 0, time.UTC)
	for index, closeValue := range []float64{10, 11, 12} {
		at := base.Add(time.Duration(index) * time.Minute)
		runtime.push(types.KLine{
			Symbol:    "US.AAPL",
			Interval:  types.Interval1m,
			StartTime: types.Time(at),
			EndTime:   types.Time(at.Add(time.Minute)),
			Open:      fixedpoint.NewFromFloat(closeValue - 0.5),
			High:      fixedpoint.NewFromFloat(closeValue + 1),
			Low:       fixedpoint.NewFromFloat(closeValue - 1),
			Close:     fixedpoint.NewFromFloat(closeValue),
			Volume:    fixedpoint.NewFromFloat(100),
		}, market.SessionRegular)
	}
	if len(runtime.closes) != 2 || len(runtime.opens) != 2 || len(runtime.highs) != 2 || len(runtime.lows) != 2 || len(runtime.volumes) != 2 || len(runtime.endTimes) != 2 || len(runtime.sessions) != 2 || len(runtime.tradingPeriodLabels["day"]) != 2 {
		t.Fatalf("trimmed runtime series are misaligned: %#v", runtime)
	}
	if snapshot := runtime.snapshot(); snapshot != nil {
		t.Fatalf("runtime without requested indicator snapshots = %#v", snapshot)
	}
	var nilRuntime *indicatorRuntime
	if snapshot := nilRuntime.snapshot(); snapshot != nil {
		t.Fatalf("nil runtime snapshot = %#v", snapshot)
	}

	closes := []float64{10, 12, 11}
	config := stopLossConfig{mode: "takeProfit", direction: "long", timeValue: 2, percentage: 5, windowPolicy: "continuous"}
	snapshot := buildStopLossSnapshotForSymbolWithOptionsAndCache(closes, nil, nil, config, 1, "", false, newSnapshotSeriesCache())
	if snapshot == nil || snapshot["triggered"] != true || snapshot["mode"] != "takeProfit" {
		t.Fatalf("take-profit snapshot = %#v", snapshot)
	}
	trailing := buildStopLossSnapshotForSymbolWithOptionsAndCache([]float64{10, 14, 11}, nil, nil, stopLossConfig{mode: "trailingStop", direction: "long", timeValue: 2, percentage: 10}, 1, "", false, newSnapshotSeriesCache())
	if trailing == nil || trailing["triggered"] != true || trailing["peakClose"] != 14.0 {
		t.Fatalf("trailing-stop snapshot = %#v", trailing)
	}
	if snapshot := buildStopLossSnapshotForSymbolWithOptionsAndCache([]float64{0, 1, 2}, nil, nil, config, 1, "", false, nil); snapshot != nil {
		t.Fatalf("invalid-price stop-loss snapshot = %#v", snapshot)
	}
	if start, _, ok := stopLossWindowStart([]float64{10, 11}, nil, nil, config, 1, nil); ok || start != 0 {
		t.Fatalf("too-short stop-loss window = (%d, %v)", start, ok)
	}
	if invalidStopLossPrice(1) || !invalidStopLossPrice(0) || !invalidStopLossPrice(math.Inf(1)) {
		t.Fatal("stop-loss price validation failed")
	}
	if got := stopLossTriggerPercent("long", true, false, 3, 4); got != 3 {
		t.Fatalf("long trigger percent = %v", got)
	}
	if got := stopLossTriggerPercent("short", false, true, 3, 4); got != 4 {
		t.Fatalf("short trigger percent = %v", got)
	}
	if got := stopLossTriggerPercent("both", false, false, 3, 4); got != 4 {
		t.Fatalf("combined trigger percent = %v", got)
	}
}
