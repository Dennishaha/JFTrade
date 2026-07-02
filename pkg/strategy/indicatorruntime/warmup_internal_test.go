package indicatorruntime

import (
	"strings"
	"testing"
)

func TestCalculateIndicatorWarmupBarsCoversSourceAwareWindowsAndDivergences(t *testing.T) {
	requirements := indicatorRequirements{
		rsiSource:      []sourcePeriodConfig{{source: "hlc3", period: 14}},
		stdevSource:    []sourcePeriodConfig{{source: "hlc3", period: 11}},
		variance:       []sourcePeriodConfig{{source: "close", period: 9}},
		windows:        []windowConfig{{function: "change", source: "close", period: 4}, {function: "range", source: "high", period: 7}},
		cum:            []sourceConfig{{source: "volume"}},
		stoch:          []sourcePeriodConfig{{source: "hlc3", period: 12, timeUnit: "day"}},
		kdj:            []kdjConfig{{period: 9, m1: 3, m2: 3}},
		cciSource:      []sourcePeriodConfig{{source: "close", period: 20}},
		williamsR:      []int{14},
		mfi:            []sourcePeriodConfig{{source: "hlc3", period: 15}},
		dmi:            []dmiConfig{{diLength: 14, adxSmoothing: 14}},
		supertrend:     []supertrendConfig{{factor: 3, atrPeriod: 10}},
		sar:            []sarConfig{{start: 0.02, increment: 0.02, maximum: 0.2}},
		rsiDivergence:  []rsiDivergenceConfig{{period: 14, direction: "top", lookback: 5}},
		macdDivergence: []macdDivergenceConfig{{fastPeriod: 12, slowPeriod: 26, signalPeriod: 9, direction: "bottom", lookback: 6}},
		kdjDivergence:  []kdjDivergenceConfig{{period: 9, m1: 3, m2: 3, direction: "top", lookback: 4}},
		stopLoss:       []stopLossConfig{{mode: "stopLoss", direction: "auto", timeValue: 3, timeUnit: "bar", percentage: 5, windowPolicy: "continuous"}},
		securitySource: []securitySourceConfig{{source: "close", timeUnit: "week", lookback: 1}},
	}

	warmupBars := calculateIndicatorWarmupBars(requirements, 5, "US.AAPL", false)
	if warmupBars != 1170 {
		t.Fatalf("calculateIndicatorWarmupBars() = %d, want 1170", warmupBars)
	}
}

func TestEstimateTradingPeriodBarsHandlesFallbackAndInvalidInputs(t *testing.T) {
	if got := estimateTradingPeriodBars(0, "day", 5, "US.AAPL", false); got != 0 {
		t.Fatalf("estimateTradingPeriodBars(period=0) = %d, want 0", got)
	}
	if got := estimateTradingPeriodBars(3, "hour", 0, "", false); got != 180 {
		t.Fatalf("estimateTradingPeriodBars(hour, interval=0) = %d, want 180", got)
	}
	if got := estimateTradingPeriodBars(2, "week", 5, "CRYPTO.BTC", false); got != 780 {
		t.Fatalf("estimateTradingPeriodBars(unknown symbol week) = %d, want 780", got)
	}
}

func TestAdvancedIndicatorLookbackReflectsWarmupSemantics(t *testing.T) {
	if got := advancedIndicatorLookback(advancedIndicatorConfig{kind: "anchored_vwap", period: 200}); got != 1 {
		t.Fatalf("anchored_vwap lookback = %d, want 1 because it resets by anchor period", got)
	}
	if got := advancedIndicatorLookback(advancedIndicatorConfig{kind: "pivothigh", left: 4, right: 3}); got != 9 {
		t.Fatalf("pivot lookback = %d, want left + right + confirmation bars", got)
	}
	if got := advancedIndicatorLookback(advancedIndicatorConfig{kind: "linreg", period: 20, offset: 2}); got != 24 {
		t.Fatalf("period/offset lookback = %d, want period + offset + confirmation bars", got)
	}
}

func TestValidateFixedTimeframeRequirementsCoversAllConfigFamilies(t *testing.T) {
	valid := indicatorRequirements{
		ma:             []movingAverageConfig{{period: 5, timeUnit: "15m"}},
		securitySource: []securitySourceConfig{{source: "close", timeUnit: "hour"}},
		rsiSource:      []sourcePeriodConfig{{source: "close", period: 14, timeUnit: "day"}},
		stdevSource:    []sourcePeriodConfig{{source: "close", period: 14, timeUnit: "week"}},
		variance:       []sourcePeriodConfig{{source: "close", period: 14, timeUnit: "month"}},
		stoch:          []sourcePeriodConfig{{source: "close", period: 14, timeUnit: "15m"}},
		cciSource:      []sourcePeriodConfig{{source: "hlc3", period: 20, timeUnit: "hour"}},
		mfi:            []sourcePeriodConfig{{source: "hlc3", period: 14, timeUnit: "day"}},
		advanced:       []advancedIndicatorConfig{{kind: "linreg", period: 20, timeUnit: "week"}},
	}
	if err := validateFixedTimeframeRequirements(valid, 5); err != nil {
		t.Fatalf("validateFixedTimeframeRequirements(valid) error = %v", err)
	}

	tests := []struct {
		name        string
		requirement indicatorRequirements
		wantDetail  string
	}{
		{
			name:        "lower than strategy interval",
			requirement: indicatorRequirements{ma: []movingAverageConfig{{period: 5, timeUnit: "minute"}}},
			wantDetail:  "fixed timeframe 1m is lower than strategy interval 5m",
		},
		{
			name:        "not aligned with strategy interval",
			requirement: indicatorRequirements{advanced: []advancedIndicatorConfig{{kind: "linreg", timeUnit: "7m"}}},
			wantDetail:  "fixed timeframe 7m is not aligned with strategy interval 5m",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFixedTimeframeRequirements(tt.requirement, 5)
			if err == nil {
				t.Fatal("validateFixedTimeframeRequirements() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tt.wantDetail) {
				t.Fatalf("validation error = %v, want detail %q", err, tt.wantDetail)
			}
		})
	}
	if minutes, ok := comparableTimeUnitMinutes("quarter"); ok || minutes != 0 {
		t.Fatalf("comparableTimeUnitMinutes(quarter) = %d/%v, want 0/false", minutes, ok)
	}
}

func TestFormatFixedTimeframeLabels(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"minute", "1m"},
		{"hour", "60m"},
		{"day", "D"},
		{"week", "W"},
		{"month", "M"},
		{"7m", "7m"},
	} {
		if got := formatIndicatorTimeUnit(tc.input); got != tc.want {
			t.Fatalf("formatIndicatorTimeUnit(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
	for _, tc := range []struct {
		input int
		want  string
	}{
		{0, "1m"},
		{5, "5m"},
		{tradingSessionMinutesPerDay, "D"},
		{tradingSessionMinutesPerWeek, "W"},
		{tradingSessionMinutesPerMonth, "M"},
	} {
		if got := formatIntervalMinutes(tc.input); got != tc.want {
			t.Fatalf("formatIntervalMinutes(%d) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
