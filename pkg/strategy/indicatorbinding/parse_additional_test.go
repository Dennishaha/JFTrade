package indicatorbinding

import (
	"strings"
	"testing"
)

func TestMatchingFunctionCallParenHandlesQuotesAndEscapes(t *testing.T) {
	input := "fn(\"a\\\"b\", nested('x,y'), tail)"
	open := 2
	if got := matchingFunctionCallParen(input, open); got != len(input)-1 {
		t.Fatalf("matchingFunctionCallParen() = %d, want %d", got, len(input)-1)
	}

	if got := matchingFunctionCallParen(`fn("unterminated, tail)`, open); got != -1 {
		t.Fatalf("matchingFunctionCallParen(unterminated quote) = %d, want -1", got)
	}
}

func TestParseIndicatorTimeUnitValueSupportsQuotedAndMinuteCountInputs(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{`"minute"`, "minute", true},
		{`"60m"`, "hour", true},
		{`"15m"`, "15m", true},
		{`"001m"`, "minute", true},
		{`"15"`, "", false},
		{`"1D"`, "", false},
		{`"0m"`, "", false},
		{`"badm"`, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseIndicatorTimeUnitValue(tt.input)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("ParseIndicatorTimeUnitValue(%q) = %q %v, want %q %v", tt.input, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestParseMovingAverageOptionalArgsCoversSourceAndTimeUnitSemantics(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantUnit  string
		wantSrc   string
		wantError string
	}{
		{
			name:     "no optional args",
			args:     nil,
			wantSrc:  "close",
			wantUnit: "",
		},
		{
			name:     "single source arg",
			args:     []string{"hlc3"},
			wantSrc:  "hlc3",
			wantUnit: "",
		},
		{
			name:     "single time unit arg",
			args:     []string{"15m"},
			wantSrc:  "close",
			wantUnit: "15m",
		},
		{
			name:     "time unit plus source",
			args:     []string{"hour", "ohlc4"},
			wantSrc:  "ohlc4",
			wantUnit: "hour",
		},
		{
			name:      "too many optional args",
			args:      []string{"hour", "close", "extra"},
			wantError: "too many moving-average optional arguments",
		},
		{
			name:      "unsupported first arg",
			args:      []string{"quarter"},
			wantError: `moving-average time unit or source "quarter" is not supported`,
		},
		{
			name:      "unsupported source arg",
			args:      []string{"day", "typical"},
			wantError: `moving-average source "typical" is not supported`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUnit, gotSrc, err := ParseMovingAverageOptionalArgs(tt.args)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("ParseMovingAverageOptionalArgs(%v) err = %v, want %q", tt.args, err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMovingAverageOptionalArgs(%v) error = %v", tt.args, err)
			}
			if gotUnit != tt.wantUnit || gotSrc != tt.wantSrc {
				t.Fatalf("ParseMovingAverageOptionalArgs(%v) = %q %q, want %q %q", tt.args, gotUnit, gotSrc, tt.wantUnit, tt.wantSrc)
			}
		})
	}
}

func TestParsePriceSourceAndBuildMovingAverageKeyWithSource(t *testing.T) {
	validSources := map[string]string{
		"OPEN":   "open",
		"High":   "high",
		" low ":  "low",
		"close":  "close",
		"hl2":    "hl2",
		"hlc3":   "hlc3",
		"ohlc4":  "ohlc4",
		"volume": "volume",
	}
	for input, want := range validSources {
		got, ok := ParsePriceSource(input)
		if !ok || got != want {
			t.Fatalf("ParsePriceSource(%q) = %q %v, want %q true", input, got, ok, want)
		}
	}
	if got, ok := ParsePriceSource("typical"); ok || got != "" {
		t.Fatalf("ParsePriceSource(typical) = %q %v, want empty false", got, ok)
	}

	keyCases := []struct {
		name string
		src  string
		unit string
		want string
	}{
		{name: "default close omits source", src: "close", unit: "", want: "ma:EMA:14"},
		{name: "non-close source only", src: "hl2", unit: "", want: "ma:EMA:14:hl2"},
		{name: "time unit only", src: "bad", unit: "day", want: "ma:EMA:14:day"},
		{name: "time unit and source", src: "hlc3", unit: "hour", want: "ma:EMA:14:hour:hlc3"},
	}
	for _, tt := range keyCases {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildMovingAverageKeyWithSource("EMA", 14, tt.unit, tt.src); got != tt.want {
				t.Fatalf("BuildMovingAverageKeyWithSource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeFallbackHelpersCoverDefaultBranches(t *testing.T) {
	if got := NormalizeQuantityMode(" POSITIONPERCENT "); got != "symbol_position_percent" {
		t.Fatalf("NormalizeQuantityMode(positionpercent) = %q, want symbol_position_percent", got)
	}
	if got := NormalizeProtectMode(" trailing_stop "); got != "trailingStop" {
		t.Fatalf("NormalizeProtectMode(trailing_stop) = %q, want trailingStop", got)
	}
	if got := NormalizeProtectDirection(" both "); got != "auto" {
		t.Fatalf("NormalizeProtectDirection(both) = %q, want auto", got)
	}
	if got := NormalizeProtectWindowPolicy(" session "); got != "session" {
		t.Fatalf("NormalizeProtectWindowPolicy(session) = %q, want session", got)
	}
}
