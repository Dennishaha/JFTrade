package indicatorruntime

import (
	"strings"
	"testing"
)

func TestParseIndicatorRequirementKeysStrictRejectsEveryKeyFamilyBoundary(t *testing.T) {
	tests := []struct {
		key       string
		wantError string
	}{
		{key: "cog:spread:10", wantError: "invalid cog key"},
		{key: "bbw:close:20:-2", wantError: "invalid bbw key"},
		{key: "tsi:close:0:25", wantError: "invalid tsi key"},
		{key: "correlation:close:spread:10", wantError: "invalid correlation key"},
		{key: "percentile_linear_interpolation:close:bad:80", wantError: "invalid percentile key"},
		{key: "swma:spread", wantError: "invalid swma key"},
		{key: "linreg:close:20:-1", wantError: "invalid linreg key"},
		{key: "obv:close:quarter", wantError: "invalid obv key"},
		{key: "pivothigh:high:0:3", wantError: "invalid pivot key"},
		{key: "alma:close:9:0.85:0", wantError: "invalid alma key"},
		{key: "ma:20:quarter", wantError: "invalid moving average key"},
		{key: "ma:EMA:20:quarter:close", wantError: "invalid moving average key"},
		{key: "security_source:bar:close", wantError: "invalid security_source key"},
		{key: "rsi:close:9:bar", wantError: "invalid rsi key"},
		{key: "stdev:hlc3:0", wantError: "invalid stdev key"},
		{key: "variance:spread:8", wantError: "invalid variance key"},
		{key: "cum:spread", wantError: "invalid cum key"},
		{key: "macd:12:x:9", wantError: "invalid macd key"},
		{key: "bollinger:20:0", wantError: "invalid bollinger key"},
		{key: "kdj:9:3:x", wantError: "invalid kdj key"},
		{key: "atr:0", wantError: "invalid atr key"},
		{key: "cci:bad:20", wantError: "invalid cci key"},
		{key: "vwap:spread", wantError: "invalid vwap key"},
		{key: "highest:spread:10", wantError: "invalid rolling window key"},
		{key: "mfi:hlc3:0", wantError: "invalid mfi key"},
		{key: "dmi:14:0", wantError: "invalid dmi key"},
		{key: "supertrend:0:10", wantError: "invalid supertrend key"},
		{key: "sar:0.02:0:0.2", wantError: "invalid sar key"},
		{key: "williamsr:0", wantError: "invalid williamsr key"},
		{key: "sl:long:2:quarter:4", wantError: "invalid risk key"},
		{key: "risk:stopLoss:auto:2:quarter:4:session", wantError: "invalid risk key"},
		{key: "divergence:rsi:0:top:5", wantError: "invalid divergence key"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			_, err := parseIndicatorRequirementKeys([]string{tt.key}, true)
			if err == nil {
				t.Fatalf("parseIndicatorRequirementKeys(%q, strict) error = nil", tt.key)
			}
			if !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("parseIndicatorRequirementKeys(%q) error = %v, want detail %q", tt.key, err, tt.wantError)
			}
		})
	}
}

func TestMovingAverageAndRiskKeyParsingBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		parts []string
		want  movingAverageConfig
	}{
		{name: "legacy ma", parts: []string{"ma", "20"}, want: movingAverageConfig{averageType: "MA", period: 20}},
		{name: "legacy ma source", parts: []string{"ma", "20", "volume"}, want: movingAverageConfig{averageType: "MA", period: 20, source: "volume"}},
		{name: "legacy ma time", parts: []string{"ma", "20", "day"}, want: movingAverageConfig{averageType: "MA", period: 20, timeUnit: "day"}},
		{name: "typed ma source", parts: []string{"ma", "EMA", "9", "hlc3"}, want: movingAverageConfig{averageType: "EMA", period: 9, source: "hlc3"}},
		{name: "typed ma time", parts: []string{"ma", "HMA", "9", "60m"}, want: movingAverageConfig{averageType: "HMA", period: 9, timeUnit: "hour"}},
		{name: "typed ma time source", parts: []string{"ma", "VWMA", "9", "15m", "volume"}, want: movingAverageConfig{averageType: "VWMA", period: 9, timeUnit: "15m", source: "volume"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseMovingAverageConfig(tc.parts)
			if !ok || got != tc.want {
				t.Fatalf("parseMovingAverageConfig(%#v) = %#v/%v, want %#v/true", tc.parts, got, ok, tc.want)
			}
		})
	}

	for _, parts := range [][]string{
		{"ma"},
		{"ma", "0"},
		{"ma", "20", "quarter"},
		{"ma", "EMA", "9", "quarter"},
		{"ma", "EMA", "9", "quarter", "close"},
		{"ma", "EMA", "bad", "day"},
	} {
		if got, ok := parseMovingAverageConfig(parts); ok {
			t.Fatalf("parseMovingAverageConfig(%#v) = %#v/true, want invalid", parts, got)
		}
	}

	for _, tc := range []struct {
		name  string
		parts []string
		want  stopLossConfig
	}{
		{name: "sl bar", parts: []string{"sl", "long", "2", "bar", "4"}, want: stopLossConfig{mode: "stopLoss", direction: "long", timeValue: 2, percentage: 4, windowPolicy: "continuous"}},
		{name: "sl minutes", parts: []string{"sl", "short", "3", "15m", "5"}, want: stopLossConfig{mode: "stopLoss", direction: "short", timeValue: 3, timeUnit: "15m", percentage: 5, windowPolicy: "continuous"}},
		{name: "risk session", parts: []string{"risk", "takeProfit", "auto", "4", "week", "6", "session"}, want: stopLossConfig{mode: "takeProfit", direction: "auto", timeValue: 4, timeUnit: "week", percentage: 6, windowPolicy: "session"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseStopLossConfig(tc.parts)
			if !ok || got != tc.want {
				t.Fatalf("parseStopLossConfig(%#v) = %#v/%v, want %#v/true", tc.parts, got, ok, tc.want)
			}
		})
	}

	for _, parts := range [][]string{
		{"sl", "long", "2", "quarter", "4"},
		{"sl", "long", "0", "bar", "4"},
		{"sl", "long", "2", "bar", "-1"},
		{"risk", "bad", "auto", "2", "day", "4", "session"},
		{"risk", "stopLoss", "auto", "2", "quarter", "4", "session"},
		{"risk", "stopLoss", "auto", "2", "day", "0", "session"},
		{"risk", "stopLoss", "auto", "2", "day", "4", "overnight"},
	} {
		if got, ok := parseStopLossConfig(parts); ok {
			t.Fatalf("parseStopLossConfig(%#v) = %#v/true, want invalid", parts, got)
		}
	}
}

func TestIndicatorTimeUnitAndSourceNormalizationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		want    string
		wantOK  bool
		wantMin int
	}{
		{raw: "minute", want: "minute", wantOK: true, wantMin: 1},
		{raw: "60m", want: "hour", wantOK: true, wantMin: 60},
		{raw: "15m", want: "15m", wantOK: true, wantMin: 15},
		{raw: "day", want: "day", wantOK: true},
		{raw: "quarter", wantOK: false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			got, ok := parseIndicatorTimeUnit(tc.raw)
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("parseIndicatorTimeUnit(%q) = %q/%v, want %q/%v", tc.raw, got, ok, tc.want, tc.wantOK)
			}
			if tc.wantMin > 0 {
				minutes, minuteOK := indicatorTimeUnitMinutes(tc.raw)
				if !minuteOK || minutes != tc.wantMin {
					t.Fatalf("indicatorTimeUnitMinutes(%q) = %d/%v, want %d/true", tc.raw, minutes, minuteOK, tc.wantMin)
				}
			}
		})
	}

	if source, ok := parseOHLCVSource("Ohlc4"); !ok || source != "ohlc4" {
		t.Fatalf("parseOHLCVSource = %q/%v, want ohlc4/true", source, ok)
	}
	if source, ok := parseOHLCVSource("spread"); ok || source != "" {
		t.Fatalf("parseOHLCVSource invalid = %q/%v, want miss", source, ok)
	}
	if got := normalizeWindowFunction("MODE"); got != "mode" {
		t.Fatalf("normalizeWindowFunction(MODE) = %q, want mode", got)
	}
	if got := normalizeWindowFunction("median"); got != "" {
		t.Fatalf("normalizeWindowFunction(median) = %q, want empty unsupported window", got)
	}
	if got := normalizeSourceOrClose("spread"); got != "close" {
		t.Fatalf("normalizeSourceOrClose(spread) = %q, want close", got)
	}
	if got := firstNonEmpty(" ", "", "atr:14"); got != "atr:14" {
		t.Fatalf("firstNonEmpty = %q, want atr:14", got)
	}
}
