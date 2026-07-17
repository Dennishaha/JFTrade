package indicatorruntime

import "testing"

func TestCoverage98EMATailStateHandlesRetentionAndTrimRecovery(t *testing.T) {
	if state := newRollingEMATailState(0, 2, 2); state != nil {
		t.Fatalf("zero-period EMA state = %#v", state)
	}
	if states := newRollingEMAStates(indicatorRequirements{
		ma: []movingAverageConfig{
			{averageType: "SMA", period: 2},
			{averageType: "EMA", period: 0},
			{averageType: "EMA", period: 2, timeUnit: "5m"},
		},
	}, 1, 4); states != nil {
		t.Fatalf("non-rolling EMA states = %#v", states)
	}

	state := newRollingEMATailState(2, 2, 2)
	state.push(10, false, 0, 0, false, false)
	if _, _, currentOK, previousOK := state.snapshotValues(); currentOK || previousOK {
		t.Fatalf("single EMA input was reported as complete: %#v", state)
	}
	state.push(14, false, 0, 0, false, false)
	state.push(18, false, 0, 0, false, false)
	if current, _, currentOK, _ := state.snapshotValues(); !currentOK || current <= 0 {
		t.Fatalf("rolling EMA snapshot = (%v, %v)", current, currentOK)
	}
	state.push(20, true, 0, 0, false, false)
	if len(state.tail) != 1 || state.tail[0] != 20 || state.windowLen != 1 {
		t.Fatalf("EMA trim recovery did not reset a partial window: %#v", state)
	}
	state.push(24, true, 10, 14, true, true)
	if got := state.powerAt(10); got <= 0 || got >= 1 {
		t.Fatalf("EMA extrapolated beta power = %v", got)
	}

	var nilState *rollingEMATailState
	nilState.push(1, false, 0, 0, false, false)
	if got := nilState.powerAt(-1); got != 0 {
		t.Fatalf("nil EMA power = %v", got)
	}
	if _, _, currentOK, previousOK := nilState.snapshotValues(); currentOK || previousOK {
		t.Fatal("nil EMA state produced values")
	}
}

func TestCoverage98RollingWindowStatesModelIndicatorAndBooleanSignals(t *testing.T) {
	if states := newRollingWindowStates(indicatorRequirements{
		windows: []windowConfig{
			{function: "sum", source: "close", period: 0},
			{function: "unknown", source: "close", period: 2},
			{function: "sum", source: "unknown", period: 2},
		},
	}); states != nil {
		t.Fatalf("invalid window states = %#v", states)
	}

	cases := []struct {
		name     string
		function string
		values   []float64
		want     float64
		wantBool *bool
	}{
		{name: "highest", function: "highest", values: []float64{2, 5}, want: 5},
		{name: "lowest", function: "lowest", values: []float64{2, 5}, want: 2},
		{name: "sum", function: "sum", values: []float64{2, 5}, want: 7},
		{name: "range", function: "range", values: []float64{2, 5}, want: 3},
		{name: "mode", function: "mode", values: []float64{4, 4}, want: 4},
		{name: "momentum", function: "mom", values: []float64{2, 4, 7}, want: 5},
		{name: "roc", function: "roc", values: []float64{2, 4, 7}, want: 250},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := &rollingWindowState{config: windowConfig{function: tc.function, source: "close", period: 2}}
			for _, value := range tc.values {
				state.push(value)
			}
			if !state.hasCurrent || state.current != tc.want {
				t.Fatalf("%s state = %#v, want current %v", tc.function, state, tc.want)
			}
		})
	}

	rising := &rollingWindowState{config: windowConfig{function: "rising", source: "close", period: 2}}
	for _, value := range []float64{1, 2, 3} {
		rising.push(value)
	}
	if !rising.hasCurrent || !rising.boolCurrent {
		t.Fatalf("rising state = %#v", rising)
	}
	falling := &rollingWindowState{config: windowConfig{function: "falling", source: "close", period: 2}}
	for _, value := range []float64{3, 2, 1} {
		falling.push(value)
	}
	if !falling.hasCurrent || !falling.boolCurrent {
		t.Fatalf("falling state = %#v", falling)
	}
	if value, ok := (&rollingWindowState{config: windowConfig{function: "roc", period: 2}}).calculateRateOfChange(); ok || value != 0 {
		t.Fatalf("incomplete ROC = (%v, %v)", value, ok)
	}

	var nilState *rollingWindowState
	nilState.push(1)
	if value, ok := nilState.calculateSum(); ok || value != 0 {
		t.Fatalf("nil window sum = (%v, %v)", value, ok)
	}
}

func TestCoverage98AdvancedSnapshotsKeepUnsupportedAndUnavailableDataDistinct(t *testing.T) {
	runtime := &indicatorRuntime{
		opens:   []float64{1, 2, 3, 4, 5},
		highs:   []float64{2, 3, 4, 5, 6},
		lows:    []float64{0, 1, 2, 3, 4},
		closes:  []float64{1, 2, 3, 4, 5},
		volumes: []float64{10, 11, 12, 13, 14},
	}
	cache := newSnapshotSeriesCache()
	for _, config := range []advancedIndicatorConfig{
		{key: "rsi", kind: "rsi", source: "close", period: 2},
		{key: "macd", kind: "macd", source: "close", period: 1, right: 2, offset: 1},
		{key: "boll", kind: "bollinger", source: "close", period: 2, multiplier: 2},
		{key: "cmo", kind: "cmo", source: "close", period: 2},
		{key: "correlation", kind: "correlation", source: "close", source2: "high", period: 2},
	} {
		if snapshot := runtime.advancedIndicatorSnapshot(config, cache); snapshot == nil {
			t.Fatalf("advanced %s snapshot is nil", config.kind)
		}
	}
	if snapshot := runtime.advancedIndicatorSnapshot(advancedIndicatorConfig{kind: "not-supported", source: "close"}, cache); snapshot != nil {
		t.Fatalf("unsupported advanced snapshot = %#v", snapshot)
	}
	if snapshot := runtime.advancedSupertrendSnapshot(advancedIndicatorConfig{kind: "supertrend", timeUnit: "bad"}); snapshot != nil {
		t.Fatalf("unavailable fixed-timeframe supertrend snapshot = %#v", snapshot)
	}
	if snapshot := runtime.advancedATRSnapshot(advancedIndicatorConfig{kind: "atr", timeUnit: "bad"}, cache); snapshot != nil {
		t.Fatalf("unavailable fixed-timeframe ATR snapshot = %#v", snapshot)
	}
	if snapshot := runtime.advancedOBVSnapshot(advancedIndicatorConfig{kind: "obv", timeUnit: "bad"}, runtime.closes, cache); snapshot != nil {
		t.Fatalf("unavailable fixed-timeframe OBV snapshot = %#v", snapshot)
	}
	if snapshot, handled := runtime.anchoredVWAPSnapshot(advancedIndicatorConfig{kind: "anchored_vwap", key: "anchored"}, cache); !handled || snapshot != nil {
		t.Fatalf("missing anchored VWAP = (%#v, %v)", snapshot, handled)
	}
}
