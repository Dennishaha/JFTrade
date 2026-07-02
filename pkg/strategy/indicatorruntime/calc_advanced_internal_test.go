package indicatorruntime

import (
	"math"
	"reflect"
	"testing"
)

func TestPushOBVStatesHandlesUpDownAndFlatSources(t *testing.T) {
	closeConfig := advancedIndicatorConfig{kind: "obv", source: "close"}
	lowConfig := advancedIndicatorConfig{kind: "obv", source: "low"}
	hlc3Config := advancedIndicatorConfig{kind: "obv", source: "hlc3"}

	runtime := &indicatorRuntime{
		obvStates: map[advancedIndicatorConfig]*rollingCumState{
			closeConfig: {current: 100, hasCurrent: true},
			lowConfig:   {current: 50, hasCurrent: true},
			hlc3Config:  {current: 25, hasCurrent: true},
		},
	}

	runtime.pushOBVStates(
		10, 13, 7, 12, 1000,
		map[string]float64{"close": 11, "low": 8, "hlc3": 32.0 / 3.0},
		true,
	)

	if state := runtime.obvStates[closeConfig]; state.current != 1100 || state.previous != 100 || !state.hasPrevious || !state.hasCurrent {
		t.Fatalf("close OBV state = %+v", *state)
	}
	if state := runtime.obvStates[lowConfig]; state.current != -950 || state.previous != 50 || !state.hasPrevious || !state.hasCurrent {
		t.Fatalf("low OBV state = %+v", *state)
	}
	if state := runtime.obvStates[hlc3Config]; state.current != 25 || state.previous != 25 || !state.hasPrevious || !state.hasCurrent {
		t.Fatalf("hlc3 OBV state = %+v", *state)
	}

	fresh := &indicatorRuntime{
		obvStates: map[advancedIndicatorConfig]*rollingCumState{
			closeConfig: {},
		},
	}
	fresh.pushOBVStates(10, 11, 9, 10, 500, nil, false)
	if state := fresh.obvStates[closeConfig]; state.current != 0 || state.previous != 0 || state.hasPrevious || !state.hasCurrent {
		t.Fatalf("fresh OBV state = %+v", *state)
	}
}

func TestFixedTimeframeOHLCAndKeltnerSnapshot(t *testing.T) {
	runtime := &indicatorRuntime{
		intervalMinutes: 15,
		opens:           []float64{10, 11, 12, 13},
		highs:           []float64{11, 13, 14, 16},
		lows:            []float64{9, 10, 11, 12},
		closes:          []float64{10, 12, 13, 15},
		volumes:         []float64{100, 100, 100, 100},
	}

	if highs, lows, closes, ok := ((*indicatorRuntime)(nil)).fixedTimeframeOHLC("15m"); ok || highs != nil || lows != nil || closes != nil {
		t.Fatalf("nil fixedTimeframeOHLC() = %v %v %v %v", highs, lows, closes, ok)
	}
	if highs, lows, closes, ok := runtime.fixedTimeframeOHLC("15m"); !ok {
		t.Fatal("fixedTimeframeOHLC(15m) ok = false")
	} else {
		if len(highs) != 4 || highs[3] != 16 || len(lows) != 4 || lows[0] != 9 || len(closes) != 4 || closes[2] != 13 {
			t.Fatalf("fixedTimeframeOHLC(15m) = %v %v %v", highs, lows, closes)
		}
	}

	config := advancedIndicatorConfig{
		kind:       "kc",
		source:     "close",
		timeUnit:   "15m",
		period:     3,
		multiplier: 1.5,
		useTR:      true,
	}
	snapshot := runtime.calculateKeltnerSnapshot(config)
	if snapshot == nil {
		t.Fatal("calculateKeltnerSnapshot() = nil")
	}

	values := runtime.closes
	basisSeries := calculateEMASequence(values, config.period)
	ranges := []float64{
		2,
		3,
		math.Max(14-11, math.Max(math.Abs(14-12), math.Abs(11-12))),
		math.Max(16-12, math.Max(math.Abs(16-13), math.Abs(12-13))),
	}
	rangeSeries := calculateEMASequence(ranges, config.period)
	wantBasis := basisSeries[len(basisSeries)-1]
	wantBand := rangeSeries[len(rangeSeries)-1] * config.multiplier
	wantUpper := wantBasis + wantBand
	wantLower := wantBasis - wantBand
	wantWidth := (wantBand * 2) / wantBasis

	for key, want := range map[string]float64{
		"value": wantBasis,
		"basis": wantBasis,
		"upper": wantUpper,
		"lower": wantLower,
		"width": wantWidth,
	} {
		got := snapshot[key].(float64)
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("snapshot[%q] = %v, want %v", key, got, want)
		}
	}
}

func TestSeriesForSourceSupportsDerivedAndDefaultSources(t *testing.T) {
	var nilRuntime *indicatorRuntime
	if values := nilRuntime.seriesForSource("close"); values != nil {
		t.Fatalf("nil seriesForSource(close) = %v, want nil", values)
	}

	runtime := &indicatorRuntime{
		opens:   []float64{10, 20},
		highs:   []float64{14, 24},
		lows:    []float64{8, 18},
		closes:  []float64{12, 22},
		volumes: []float64{100, 200},
	}

	tests := []struct {
		source string
		want   []float64
	}{
		{source: "open", want: []float64{10, 20}},
		{source: "high", want: []float64{14, 24}},
		{source: "low", want: []float64{8, 18}},
		{source: "volume", want: []float64{100, 200}},
		{source: "hl2", want: []float64{11, 21}},
		{source: "hlc3", want: []float64{34.0 / 3.0, 64.0 / 3.0}},
		{source: "ohlc4", want: []float64{11, 21}},
		{source: "unknown", want: []float64{12, 22}},
	}

	for _, tc := range tests {
		if got := runtime.seriesForSource(tc.source); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("seriesForSource(%q) = %v, want %v", tc.source, got, tc.want)
		}
	}
}

func TestAdvancedIndicatorSnapshotDispatchesAdvancedBranches(t *testing.T) {
	cache := newSnapshotSeriesCache()
	runtime := &indicatorRuntime{
		intervalMinutes: 15,
		opens:           []float64{10, 11, 12, 13},
		highs:           []float64{11, 13, 14, 16},
		lows:            []float64{9, 10, 11, 12},
		closes:          []float64{10, 12, 13, 15},
		volumes:         []float64{100, 200, 150, 300},
		anchoredVWAPStates: map[advancedIndicatorConfig]*rollingVWAPState{
			{key: "anchored_vwap:week:close", kind: "anchored_vwap", source: "close", timeUnit: "week"}: {
				periodKey:   "week:2026-06-23",
				totalPV:     4500,
				totalVolume: 150,
				hasCurrent:  true,
			},
		},
	}

	t.Run("anchored vwap returns scalar snapshot", func(t *testing.T) {
		config := advancedIndicatorConfig{key: "anchored_vwap:week:close", kind: "anchored_vwap", source: "close", timeUnit: "week"}
		snapshot, ok := runtime.advancedIndicatorSnapshot(config, cache).(interface {
			ScalarValue() (float64, bool)
		})
		if !ok {
			t.Fatalf("anchored_vwap snapshot type = %T", runtime.advancedIndicatorSnapshot(config, cache))
		}
		if value, valueOK := snapshot.ScalarValue(); !valueOK || value != 30 {
			t.Fatalf("anchored_vwap scalar = %v, %v", value, valueOK)
		}
	})

	t.Run("anchored vwap missing state returns nil", func(t *testing.T) {
		config := advancedIndicatorConfig{key: "anchored_vwap:day:close", kind: "anchored_vwap", source: "close", timeUnit: "day"}
		if snapshot := runtime.advancedIndicatorSnapshot(config, cache); snapshot != nil {
			t.Fatalf("missing anchored_vwap snapshot = %T, want nil", snapshot)
		}
	})

	t.Run("kcw returns width scalar", func(t *testing.T) {
		config := advancedIndicatorConfig{
			key:        "kcw:close:3:1.5:true",
			kind:       "kcw",
			source:     "close",
			period:     3,
			multiplier: 1.5,
			useTR:      true,
		}
		expected := runtime.calculateKeltnerSnapshot(advancedIndicatorConfig{
			kind:       "kc",
			source:     "close",
			period:     3,
			multiplier: 1.5,
			useTR:      true,
		})["width"].(float64)
		snapshot, ok := runtime.advancedIndicatorSnapshot(config, cache).(interface {
			ScalarValue() (float64, bool)
		})
		if !ok {
			t.Fatalf("kcw snapshot type = %T", runtime.advancedIndicatorSnapshot(config, cache))
		}
		if value, valueOK := snapshot.ScalarValue(); !valueOK || math.Abs(value-expected) > 1e-9 {
			t.Fatalf("kcw scalar = %v, %v, want %v", value, valueOK, expected)
		}
	})

	t.Run("obv timeframe snapshot returns current and previous", func(t *testing.T) {
		config := advancedIndicatorConfig{key: "obv:close:15m", kind: "obv", source: "close", timeUnit: "15m"}
		snapshot, ok := runtime.advancedIndicatorSnapshot(config, cache).(interface {
			SeriesField(string) (float64, float64, bool, bool, bool)
		})
		if !ok {
			t.Fatalf("obv snapshot type = %T", runtime.advancedIndicatorSnapshot(config, cache))
		}
		current, previous, currentOK, previousOK, fieldOK := snapshot.SeriesField("value")
		if !fieldOK || !currentOK || !previousOK || current != 650 || previous != 350 {
			t.Fatalf("obv series = %v %v %v %v %v", current, previous, currentOK, previousOK, fieldOK)
		}
	})

	t.Run("scalar indicators return consumable strategy values", func(t *testing.T) {
		tests := []advancedIndicatorConfig{
			{key: "rsi:close:2", kind: "rsi", source: "close", period: 2},
			{key: "atr:15m:3", kind: "atr", source: "close", timeUnit: "15m", period: 3},
			{key: "bbw:close:3", kind: "bbw", source: "close", period: 3, multiplier: 2},
			{key: "cog:close:3", kind: "cog", source: "close", period: 3},
			{key: "linreg:close:3", kind: "linreg", source: "close", period: 3, offset: 1},
			{key: "alma:close:3", kind: "alma", source: "close", period: 3, multiplier: 0.85, parameter: 6},
			{key: "cmo:close:3", kind: "cmo", source: "close", period: 3},
			{key: "tsi:close:1:1", kind: "tsi", source: "close", period: 1, right: 1},
			{key: "correlation:close:high:3", kind: "correlation", source: "close", source2: "high", period: 3},
			{key: "dev:close:3", kind: "dev", source: "close", period: 3},
			{key: "median:close:3", kind: "median", source: "close", period: 3},
			{key: "percentile_linear:close:3:50", kind: "percentile_linear_interpolation", source: "close", period: 3, multiplier: 50},
			{key: "percentile_nearest:close:3:50", kind: "percentile_nearest_rank", source: "close", period: 3, multiplier: 50},
			{key: "percentrank:close:3", kind: "percentrank", source: "close", period: 3},
			{key: "swma:close", kind: "swma", source: "close"},
		}
		for _, config := range tests {
			t.Run(config.kind, func(t *testing.T) {
				snapshot, ok := runtime.advancedIndicatorSnapshot(config, cache).(interface {
					ScalarValue() (float64, bool)
				})
				if !ok {
					t.Fatalf("%s snapshot type = %T", config.kind, runtime.advancedIndicatorSnapshot(config, cache))
				}
				if value, valueOK := snapshot.ScalarValue(); !valueOK || math.IsNaN(value) {
					t.Fatalf("%s scalar = %v, %v", config.kind, value, valueOK)
				}
			})
		}
	})

	t.Run("structured indicators return full snapshots", func(t *testing.T) {
		tests := []advancedIndicatorConfig{
			{key: "macd:close:2:3:1", kind: "macd", source: "close", period: 2, right: 3, offset: 1},
			{key: "bollinger:close:3:2", kind: "bollinger", source: "close", period: 3, multiplier: 2},
			{key: "supertrend:15m:2:2", kind: "supertrend", source: "close", timeUnit: "15m", period: 2, multiplier: 2},
			{key: "kc:close:3:1.5", kind: "kc", source: "close", period: 3, multiplier: 1.5, useTR: false},
		}
		for _, config := range tests {
			t.Run(config.kind, func(t *testing.T) {
				snapshot, ok := runtime.advancedIndicatorSnapshot(config, cache).(map[string]any)
				if !ok || len(snapshot) == 0 {
					t.Fatalf("%s snapshot = %#v", config.kind, runtime.advancedIndicatorSnapshot(config, cache))
				}
			})
		}
	})

	t.Run("pivot snapshots only surface confirmed swing points", func(t *testing.T) {
		pivotRuntime := &indicatorRuntime{
			intervalMinutes: 15,
			closes:          []float64{10, 15, 11},
			volumes:         []float64{100, 100, 100},
		}
		highConfig := advancedIndicatorConfig{key: "pivothigh:close:1:1", kind: "pivothigh", source: "close", left: 1, right: 1}
		highSnapshot, ok := pivotRuntime.advancedIndicatorSnapshot(highConfig, cache).(interface {
			ScalarValue() (float64, bool)
		})
		if !ok {
			t.Fatalf("pivothigh snapshot type = %T", pivotRuntime.advancedIndicatorSnapshot(highConfig, cache))
		}
		if value, valueOK := highSnapshot.ScalarValue(); !valueOK || value != 15 {
			t.Fatalf("pivothigh scalar = %v, %v, want 15 true", value, valueOK)
		}

		lowConfig := advancedIndicatorConfig{key: "pivotlow:close:1:1", kind: "pivotlow", source: "close", left: 1, right: 1}
		if snapshot := pivotRuntime.advancedIndicatorSnapshot(lowConfig, cache); snapshot != nil {
			t.Fatalf("unconfirmed pivotlow snapshot = %T, want nil", snapshot)
		}
	})

	t.Run("invalid timeframe returns nil", func(t *testing.T) {
		config := advancedIndicatorConfig{key: "obv:close:5m", kind: "obv", source: "close", timeUnit: "5m"}
		if snapshot := runtime.advancedIndicatorSnapshot(config, cache); snapshot != nil {
			t.Fatalf("invalid timeframe snapshot = %T, want nil", snapshot)
		}
	})
}
