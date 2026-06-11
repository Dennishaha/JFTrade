package indicatorbinding

import (
	"reflect"
	"testing"
)

// --- ParseFunctionCall ---

func TestParseFunctionCall(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantFn  string
		wantArg []string
		wantOk  bool
	}{
		{
			name:    "simple ma",
			input:   "ma(EMA,14,m)",
			wantFn:  "ma",
			wantArg: []string{"EMA", "14", "m"},
			wantOk:  true,
		},
		{
			name:    "no arguments",
			input:   "log()",
			wantFn:  "log",
			wantArg: nil,
			wantOk:  true,
		},
		{
			name:    "nested parentheses",
			input:   "protect(auto, trailing_stop, 2, day, 4%)",
			wantFn:  "protect",
			wantArg: []string{"auto", "trailing_stop", "2", "day", "4%"},
			wantOk:  true,
		},
		{
			name:    "leading trailing whitespace",
			input:   "  ma ( EMA , 14 , m )  ",
			wantFn:  "ma",
			wantArg: []string{"EMA", "14", "m"},
			wantOk:  true,
		},
		{
			name:   "no open paren",
			input:  "ma",
			wantOk: false,
		},
		{
			name:   "open paren at position 0",
			input:  "(EMA,14)",
			wantOk: false,
		},
		{
			name:   "no close paren",
			input:  "ma(EMA,14",
			wantOk: false,
		},
		{
			name:   "close paren before open",
			input:  ")ma(",
			wantOk: false,
		},
		{
			name:   "empty string",
			input:  "",
			wantOk: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFn, gotArg, gotOk := ParseFunctionCall(tt.input)
			if gotOk != tt.wantOk {
				t.Fatalf("ok = %v, want %v", gotOk, tt.wantOk)
			}
			if !tt.wantOk {
				return
			}
			if gotFn != tt.wantFn {
				t.Fatalf("fn = %q, want %q", gotFn, tt.wantFn)
			}
			if !reflect.DeepEqual(gotArg, tt.wantArg) {
				t.Fatalf("args = %v, want %v", gotArg, tt.wantArg)
			}
		})
	}
}

// --- SplitArguments ---

func TestSplitArguments(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "simple comma",
			input: "EMA,14,m",
			want:  []string{"EMA", "14", "m"},
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "single value",
			input: "14",
			want:  []string{"14"},
		},
		{
			name:  "nested parens preserve commas",
			input: "auto, trailing_stop, 2, day, 4%",
			want:  []string{"auto", "trailing_stop", "2", "day", "4%"},
		},
		{
			name:  "double-quoted comma",
			input: `EMA, "hello, world", 14`,
			want:  []string{"EMA", `"hello, world"`, "14"},
		},
		{
			name:  "single-quoted comma",
			input: `EMA, 'hello, world', 14`,
			want:  []string{"EMA", "'hello, world'", "14"},
		},
		{
			name:  "backtick-quoted comma",
			input: "EMA, `hello, world`, 14",
			want:  []string{"EMA", "`hello, world`", "14"},
		},
		{
			name:  "whitespace trimming",
			input: " EMA ,  14 , m ",
			want:  []string{"EMA", "14", "m"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SplitArguments(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("SplitArguments(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// --- NormalizeFunctionName ---

func TestNormalizeFunctionName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ma", "ma"},
		{"MA", "ma"},
		{"  Ma  ", "ma"},
		{"MACD", "macd"},
		{"  ", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeFunctionName(tt.input)
			if got != tt.want {
				t.Fatalf("NormalizeFunctionName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- ParseMovingAverageType ---

func TestParseMovingAverageType(t *testing.T) {
	validTypes := []string{"MA", "EMA", "SMA", "SMMA", "LWMA", "TMA", "EXPMA", "HMA", "VWMA", "BOLL"}
	for _, vt := range validTypes {
		t.Run("valid "+vt, func(t *testing.T) {
			got, ok := ParseMovingAverageType(vt)
			if !ok {
				t.Fatalf("ParseMovingAverageType(%q) ok = false", vt)
			}
			if got != vt {
				t.Fatalf("ParseMovingAverageType(%q) = %q, want %q", vt, got, vt)
			}
		})
		// lower-case variant
		lower := stringsToLowerASCII(vt)
		t.Run("valid lower "+lower, func(t *testing.T) {
			got, ok := ParseMovingAverageType(lower)
			if !ok {
				t.Fatalf("ParseMovingAverageType(%q) ok = false", lower)
			}
			if got != vt {
				t.Fatalf("ParseMovingAverageType(%q) = %q, want %q", lower, got, vt)
			}
		})
	}

	invalidTypes := []string{"WILD", "avg", "ema2", "", "  "}
	for _, it := range invalidTypes {
		t.Run("invalid "+it, func(t *testing.T) {
			got, ok := ParseMovingAverageType(it)
			if ok {
				t.Fatalf("ParseMovingAverageType(%q) ok = true, want false (got=%q)", it, got)
			}
		})
	}
}

func TestNormalizeMovingAverageType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"EMA", "EMA"},
		{"ema", "EMA"},
		{"sma", "SMA"},
		{"WILD", "MA"}, // unknown falls back to MA
		{"", "MA"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeMovingAverageType(tt.input)
			if got != tt.want {
				t.Fatalf("NormalizeMovingAverageType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- ParseIndicatorTimeUnitValue ---

func TestParseIndicatorTimeUnitValue(t *testing.T) {
	tests := []struct {
		input    string
		wantUnit string
		wantOk   bool
	}{
		{"", "", true},
		{"bar", "", true},
		{"bars", "", true},
		{"m", "minute", true},
		{"min", "minute", true},
		{"mins", "minute", true},
		{"minute", "minute", true},
		{"minutes", "minute", true},
		{"h", "hour", true},
		{"hr", "hour", true},
		{"hrs", "hour", true},
		{"hour", "hour", true},
		{"hours", "hour", true},
		{"d", "day", true},
		{"day", "day", true},
		{"days", "day", true},
		{"w", "week", true},
		{"week", "week", true},
		{"weeks", "week", true},
		{"mo", "month", true},
		{"mon", "month", true},
		{"month", "month", true},
		{"months", "month", true},
		{"Minute", "minute", true}, // case insensitive
		{"HOUR", "hour", true},
		{"year", "", false},
		{"nanosecond", "", false},
		{"  ", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotUnit, gotOk := ParseIndicatorTimeUnitValue(tt.input)
			if gotOk != tt.wantOk {
				t.Fatalf("ParseIndicatorTimeUnitValue(%q) ok = %v, want %v", tt.input, gotOk, tt.wantOk)
			}
			if gotUnit != tt.wantUnit {
				t.Fatalf("ParseIndicatorTimeUnitValue(%q) unit = %q, want %q", tt.input, gotUnit, tt.wantUnit)
			}
		})
	}
}

func TestNormalizeIndicatorTimeUnit(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"m", "minute"},
		{"year", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := NormalizeIndicatorTimeUnit(tt.input)
			if got != tt.want {
				t.Fatalf("NormalizeIndicatorTimeUnit(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- BuildMovingAverageKey ---

func TestBuildMovingAverageKey(t *testing.T) {
	tests := []struct {
		name     string
		avgType  string
		period   int
		timeUnit string
		want     string
	}{
		{
			name:     "with time unit",
			avgType:  "EMA",
			period:   14,
			timeUnit: "minute",
			want:     "ma:EMA:14:minute",
		},
		{
			name:     "empty time unit",
			avgType:  "MA",
			period:   20,
			timeUnit: "",
			want:     "ma:MA:20",
		},
		{
			name:     "day time unit",
			avgType:  "SMA",
			period:   5,
			timeUnit: "day",
			want:     "ma:SMA:5:day",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildMovingAverageKey(tt.avgType, tt.period, tt.timeUnit)
			if got != tt.want {
				t.Fatalf("BuildMovingAverageKey(%q, %d, %q) = %q, want %q",
					tt.avgType, tt.period, tt.timeUnit, got, tt.want)
			}
		})
	}
}

// --- ParseQuantityMode ---

func TestParseQuantityMode(t *testing.T) {
	valid := []struct {
		input string
		want  string
	}{
		{"account_position_percent", "account_position_percent"},
		{"accountPositionPercent", "account_position_percent"},
		{"symbol_position_percent", "symbol_position_percent"},
		{"position_percent", "symbol_position_percent"},
		{"positionPercent", "symbol_position_percent"},
		{"amount", "amount"},
		{"shares", "shares"},
		{"share", "shares"},
	}
	for _, tt := range valid {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseQuantityMode(tt.input)
			if !ok {
				t.Fatalf("ParseQuantityMode(%q) ok = false", tt.input)
			}
			if got != tt.want {
				t.Fatalf("ParseQuantityMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}

	invalid := []string{"bananas", "percent", "", "  ", "cash_percent", "cashPercent", "margin_buying_power_percent", "short_selling_power_percent"}
	for _, it := range invalid {
		t.Run("invalid "+it, func(t *testing.T) {
			_, ok := ParseQuantityMode(it)
			if ok {
				t.Fatalf("ParseQuantityMode(%q) ok = true, want false", it)
			}
		})
	}
}

func TestNormalizeQuantityMode(t *testing.T) {
	if got := NormalizeQuantityMode("bananas"); got != "shares" {
		t.Fatalf("NormalizeQuantityMode(bananas) = %q, want shares", got)
	}
	if got := NormalizeQuantityMode("cashPercent"); got != "shares" {
		t.Fatalf("NormalizeQuantityMode(cashPercent) = %q, want shares", got)
	}
}

// --- ParseProtectMode ---

func TestParseProtectMode(t *testing.T) {
	valid := []struct {
		input string
		want  string
	}{
		{"stopLoss", "stopLoss"},
		{"stop_loss", "stopLoss"},
		{"takeProfit", "takeProfit"},
		{"take_profit", "takeProfit"},
		{"trailingStop", "trailingStop"},
		{"trailing_stop", "trailingStop"},
	}
	for _, tt := range valid {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseProtectMode(tt.input)
			if !ok {
				t.Fatalf("ParseProtectMode(%q) ok = false", tt.input)
			}
			if got != tt.want {
				t.Fatalf("ParseProtectMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}

	_, ok := ParseProtectMode("unknown")
	if ok {
		t.Fatal("ParseProtectMode(unknown) ok = true, want false")
	}
}

func TestNormalizeProtectMode(t *testing.T) {
	if got := NormalizeProtectMode("unknown"); got != "stopLoss" {
		t.Fatalf("NormalizeProtectMode(unknown) = %q, want stopLoss", got)
	}
}

// --- ParseProtectDirection ---

func TestParseProtectDirection(t *testing.T) {
	valid := []struct {
		input string
		want  string
	}{
		{"long", "long"},
		{"Long", "long"},
		{"short", "short"},
		{"Short", "short"},
		{"auto", "auto"},
		{"Auto", "auto"},
		// both maps to auto: the key semantic lock.
		{"both", "auto"},
		{"Both", "auto"},
		{"BOTH", "auto"},
	}
	for _, tt := range valid {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := ParseProtectDirection(tt.input)
			if !ok {
				t.Fatalf("ParseProtectDirection(%q) ok = false", tt.input)
			}
			if got != tt.want {
				t.Fatalf("ParseProtectDirection(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}

	_, ok := ParseProtectDirection("up")
	if ok {
		t.Fatal("ParseProtectDirection(up) ok = true, want false")
	}
}

func TestNormalizeProtectDirection(t *testing.T) {
	if got := NormalizeProtectDirection("up"); got != "auto" {
		t.Fatalf("NormalizeProtectDirection(up) = %q, want auto", got)
	}
}

// --- ParseProtectWindowPolicy ---

func TestParseProtectWindowPolicy(t *testing.T) {
	tests := []struct {
		input  string
		want   string
		wantOk bool
	}{
		{"", "continuous", true},
		{"continuous", "continuous", true},
		{"Continuous", "continuous", true},
		{"session", "session", true},
		{"Session", "session", true},
		{"unknown", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, gotOk := ParseProtectWindowPolicy(tt.input)
			if gotOk != tt.wantOk {
				t.Fatalf("ParseProtectWindowPolicy(%q) ok = %v, want %v", tt.input, gotOk, tt.wantOk)
			}
			if got != tt.want {
				t.Fatalf("ParseProtectWindowPolicy(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeProtectWindowPolicy(t *testing.T) {
	if got := NormalizeProtectWindowPolicy("unknown"); got != "continuous" {
		t.Fatalf("NormalizeProtectWindowPolicy(unknown) = %q, want continuous", got)
	}
}

// --- numeric parsers ---

func TestParsePositiveInt(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"14", 14, false},
		{"1", 1, false},
		{"  42  ", 42, false},
		{"0", 0, true},
		{"-1", 0, true},
		{"abc", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParsePositiveInt(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePositiveInt(%q) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePositiveInt(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParsePositiveInt(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParsePositiveFloat(t *testing.T) {
	tests := []struct {
		input   string
		want    float64
		wantErr bool
	}{
		{"3.14", 3.14, false},
		{"1", 1.0, false},
		{"  0.5  ", 0.5, false},
		{"0", 0, true},
		{"-1.0", 0, true},
		{"abc", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParsePositiveFloat(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePositiveFloat(%q) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePositiveFloat(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParsePositiveFloat(%q) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

func TestParsePercentage(t *testing.T) {
	tests := []struct {
		input   string
		want    float64
		wantErr bool
	}{
		{"4%", 4.0, false},
		{"4", 4.0, false},
		{" 0.5%", 0.5, false},
		{"  0.5%  ", 0, true},
		{"0%", 0, true},
		{"-1%", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParsePercentage(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePercentage(%q) error = nil, want error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePercentage(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParsePercentage(%q) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

// --- arg-count helpers ---

func TestExpectOnePositiveIntArg(t *testing.T) {
	tests := []struct {
		name     string
		line     int
		funcName string
		args     []string
		want     int
		wantErr  bool
	}{
		{
			name:     "valid single arg",
			line:     10,
			funcName: "period",
			args:     []string{"14"},
			want:     14,
		},
		{
			name:     "too many args",
			line:     10,
			funcName: "period",
			args:     []string{"14", "20"},
			wantErr:  true,
		},
		{
			name:     "zero args",
			line:     10,
			funcName: "period",
			args:     nil,
			wantErr:  true,
		},
		{
			name:     "non-integer arg",
			line:     10,
			funcName: "period",
			args:     []string{"abc"},
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpectOnePositiveIntArg(tt.line, tt.funcName, tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ExpectOnePositiveIntArg error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ExpectOnePositiveIntArg error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ExpectOnePositiveIntArg = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestExpectPositiveIntArgs(t *testing.T) {
	tests := []struct {
		name     string
		line     int
		funcName string
		args     []string
		count    int
		want     []int
		wantErr  bool
	}{
		{
			name:     "valid three args",
			line:     12,
			funcName: "macd",
			args:     []string{"12", "26", "9"},
			count:    3,
			want:     []int{12, 26, 9},
		},
		{
			name:     "wrong count",
			line:     12,
			funcName: "macd",
			args:     []string{"12", "26"},
			count:    3,
			wantErr:  true,
		},
		{
			name:     "non-integer in list",
			line:     12,
			funcName: "macd",
			args:     []string{"12", "abc", "9"},
			count:    3,
			wantErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpectPositiveIntArgs(tt.line, tt.funcName, tt.args, tt.count)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ExpectPositiveIntArgs error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ExpectPositiveIntArgs error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ExpectPositiveIntArgs = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIntsToStrings(t *testing.T) {
	got := IntsToStrings([]int{12, 26, 9})
	want := []string{"12", "26", "9"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("IntsToStrings = %v, want %v", got, want)
	}

	gotNil := IntsToStrings(nil)
	if gotNil == nil || len(gotNil) != 0 {
		t.Fatalf("IntsToStrings(nil) = %v, want empty slice", gotNil)
	}
}

// stringsToLowerASCII is a test helper to avoid importing strings twice.
func stringsToLowerASCII(s string) string {
	b := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
