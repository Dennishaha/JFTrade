package indicatorruntime

import (
	"math"
	"reflect"
	"testing"
)

func TestRollingWindowRangeAndModeTrackCompletedTradingWindows(t *testing.T) {
	rangeState := &rollingWindowState{config: windowConfig{function: "range", period: 3}}
	for _, value := range []float64{10, 12, 8} {
		rangeState.push(value)
	}
	if !rangeState.hasCurrent || rangeState.current != 4 {
		t.Fatalf("range current = (%v, %v), want (4, true)", rangeState.current, rangeState.hasCurrent)
	}
	rangeState.push(9)
	if !rangeState.hasPrevious || rangeState.previous != 4 || rangeState.current != 4 {
		t.Fatalf("range rolling state = current:%v previous:%v hasPrevious:%v", rangeState.current, rangeState.previous, rangeState.hasPrevious)
	}

	modeState := &rollingWindowState{config: windowConfig{function: "mode", period: 3}}
	for _, value := range []float64{2, 3, 2} {
		modeState.push(value)
	}
	if !modeState.hasCurrent || modeState.current != 2 {
		t.Fatalf("mode current = (%v, %v), want (2, true)", modeState.current, modeState.hasCurrent)
	}
	modeState.push(3)
	if !modeState.hasPrevious || modeState.previous != 2 || modeState.current != 3 {
		t.Fatalf("mode rolling state = current:%v previous:%v hasPrevious:%v", modeState.current, modeState.previous, modeState.hasPrevious)
	}
}

func TestStreamingSourceStatesUseOHLCVBusinessSources(t *testing.T) {
	requirements := indicatorRequirements{
		cum: []sourceConfig{{source: "volume"}, {source: "does_not_exist"}},
	}
	cumStates := newRollingCumStates(requirements)
	if len(cumStates) != 1 {
		t.Fatalf("cum states len = %d, want only valid volume source", len(cumStates))
	}
	runtime := &indicatorRuntime{cumStates: cumStates}
	runtime.pushCumStates(10, 12, 9, 11, 100)
	runtime.pushCumStates(11, 13, 10, 12, 125)
	cum := runtime.cumStates[sourceConfig{source: "volume"}]
	if cum == nil || !cum.hasCurrent || !cum.hasPrevious || cum.current != 225 || cum.previous != 100 {
		t.Fatalf("volume cum state = %+v, want current 225 previous 100", cum)
	}

	stdDevStates := newRollingStdDevStates(indicatorRequirements{stdev: []int{3, 0}})
	stdRuntime := &indicatorRuntime{stdevStates: stdDevStates, closes: []float64{10, 12, 14}}
	for _, closeValue := range stdRuntime.closes {
		stdRuntime.pushStdDevStates(closeValue)
	}
	gotStdDev, ok := stdRuntime.stdDevSnapshotValue(3)
	wantStdDev, wantOK := calculateStdDev(stdRuntime.closes, 3)
	if !ok || !wantOK || math.Abs(gotStdDev-wantStdDev) > 1e-9 {
		t.Fatalf("stddev snapshot = (%v, %v), want (%v, %v)", gotStdDev, ok, wantStdDev, wantOK)
	}

	williamsStates := newRollingWilliamsRStates(indicatorRequirements{williamsR: []int{3, -1}})
	williamsRuntime := &indicatorRuntime{williamsRStates: williamsStates}
	bars := []struct {
		high  float64
		low   float64
		close float64
	}{
		{10, 8, 9},
		{12, 9, 11},
		{13, 10, 12},
	}
	for _, bar := range bars {
		williamsRuntime.pushWilliamsRStates(bar.high, bar.low, bar.close)
	}
	gotWilliams, ok := williamsRuntime.williamsRSnapshotValue(3)
	wantWilliams := -100.0 * (13 - 12) / (13 - 8)
	if !ok || math.Abs(gotWilliams-wantWilliams) > 1e-9 {
		t.Fatalf("Williams %%R snapshot = (%v, %v), want %v", gotWilliams, ok, wantWilliams)
	}
	if flat := (&rollingWilliamsRState{period: 1}); true {
		flat.push(5, 5, 5)
		if value, ok := flat.currentValue(); !ok || value != -50 {
			t.Fatalf("flat Williams %%R = (%v, %v), want (-50, true)", value, ok)
		}
	}
}

func TestStreamingStochStateAndFixedBarAggregationReflectMarketStructure(t *testing.T) {
	requirements := indicatorRequirements{
		stoch: []sourcePeriodConfig{
			{source: "close", period: 3},
			{source: "volume", period: 3},
			{source: "close", period: 3, timeUnit: "day"},
		},
	}
	stochStates := newRollingStochStates(requirements)
	if len(stochStates) != 1 {
		t.Fatalf("stoch states len = %d, want only intraday close source", len(stochStates))
	}
	runtime := &indicatorRuntime{stochStates: stochStates}
	for _, bar := range []struct {
		high   float64
		low    float64
		close  float64
		volume float64
	}{
		{10, 8, 9, 100},
		{12, 9, 11, 110},
		{13, 10, 12, 120},
		{13, 10, 11, 130},
	} {
		runtime.pushStochStates(0, bar.high, bar.low, bar.close, bar.volume)
	}
	config := sourcePeriodConfig{source: "close", period: 3}
	state := runtime.stochStates[config]
	if state == nil || !state.hasCurrent || !state.hasPrevious || math.Abs(state.previous-80) > 1e-9 || math.Abs(state.current-50) > 1e-9 {
		t.Fatalf("stoch state = %+v, want previous 80 current 50", state)
	}
	snapshot := snapshotValueToMap(runtime.stochSnapshot(config, newSnapshotSeriesCache()), [...]string{"value", "previous"})
	if snapshot["value"] != state.current || snapshot["previous"] != state.previous {
		t.Fatalf("stoch snapshot = %#v, want current/previous from state %+v", snapshot, state)
	}
	flat := &rollingStochState{period: 1}
	flat.push(5, 5, 5)
	if !flat.hasCurrent || flat.current != 50 {
		t.Fatalf("flat stoch state = %+v, want neutral 50", flat)
	}

	opens := []float64{10, 12, 14, 16, 18}
	highs := []float64{11, 15, 16, 18, 20}
	lows := []float64{9, 11, 13, 15, 17}
	closes := []float64{10.5, 14, 15, 17, 19}
	volumes := []float64{100, 200, 300, 400, 500}
	values, aggregatedVolumes, ok := aggregateFixedBarSeries(opens, highs, lows, closes, volumes, 2, "hlc3")
	wantValues := []float64{
		(15 + 9 + 14) / 3.0,
		(18 + 13 + 17) / 3.0,
		(20 + 17 + 19) / 3.0,
	}
	wantVolumes := []float64{300, 700, 500}
	if !ok || !reflect.DeepEqual(values, wantValues) || !reflect.DeepEqual(aggregatedVolumes, wantVolumes) {
		t.Fatalf("fixed-bar aggregation = values:%#v volumes:%#v ok:%v", values, aggregatedVolumes, ok)
	}
	if values, volumes, ok := aggregateFixedBarSeries(opens, highs, lows, closes, volumes, 0, "close"); ok || values != nil || volumes != nil {
		t.Fatalf("invalid fixed-bar aggregation = values:%#v volumes:%#v ok:%v", values, volumes, ok)
	}
}
