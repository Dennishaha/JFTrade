package pineruntime

import (
	"reflect"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestParseIndicatorBindingAdvancedFamilies(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		wantKind string
		wantKey  string
		wantArgs []string
	}{
		{
			name:     "cog with timeframe",
			expr:     "cog(hlc3,10,day)",
			wantKind: "cog",
			wantKey:  "cog:hlc3:10:day",
			wantArgs: []string{"hlc3", "10"},
		},
		{
			name:     "tsi with timeframe",
			expr:     "tsi(hlc3,5,20,week)",
			wantKind: "tsi",
			wantKey:  "tsi:hlc3:5:20:week",
			wantArgs: []string{"hlc3", "5", "20"},
		},
		{
			name:     "correlation with second source",
			expr:     "correlation(close,hl2,30,hour)",
			wantKind: "correlation",
			wantKey:  "correlation:close:hl2:30:hour",
			wantArgs: []string{"close", "hl2", "30"},
		},
		{
			name:     "percentile interpolation",
			expr:     "percentile_linear_interpolation(hlc3,20,25,month)",
			wantKind: "percentile_linear_interpolation",
			wantKey:  "percentile_linear_interpolation:hlc3:20:25:month",
			wantArgs: []string{"hlc3", "20", "25"},
		},
		{
			name:     "swma with timeframe",
			expr:     "swma(ohlc4,day)",
			wantKind: "swma",
			wantKey:  "swma:ohlc4:day",
			wantArgs: []string{"ohlc4"},
		},
		{
			name:     "pivot high default source",
			expr:     "pivothigh(2,3)",
			wantKind: "pivothigh",
			wantKey:  "pivothigh:high:2:3",
			wantArgs: []string{"2", "3"},
		},
		{
			name:     "pivot low custom source with timeframe",
			expr:     "pivotlow(hl2,2,3,week)",
			wantKind: "pivotlow",
			wantKey:  "pivotlow:hl2:2:3:week",
			wantArgs: []string{"hl2", "2", "3"},
		},
		{
			name:     "kcw with explicit use true range",
			expr:     "kcw(close,20,1.5,false,day)",
			wantKind: "kcw",
			wantKey:  "kcw:close:20:1.5:false:day",
			wantArgs: []string{"close", "20", "1.5", "false"},
		},
		{
			name:     "alma with timeframe",
			expr:     "alma(hlc3,9,0.85,6,month)",
			wantKind: "alma",
			wantKey:  "alma:hlc3:9:0.85:6:month",
			wantArgs: []string{"hlc3", "9", "0.85", "6"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stmt := &strategyir.LetStmt{
				Range:      strategyir.SourceRange{StartLine: 21},
				Name:       "advanced",
				Expression: tc.expr,
			}
			binding, recognized, err := parseIndicatorBinding(stmt)
			if err != nil {
				t.Fatalf("parseIndicatorBinding(%q) error = %v", tc.expr, err)
			}
			if !recognized {
				t.Fatalf("parseIndicatorBinding(%q) recognized = false", tc.expr)
			}
			if binding.Kind != tc.wantKind || binding.Key != tc.wantKey {
				t.Fatalf("binding = %#v, want kind=%q key=%q", binding, tc.wantKind, tc.wantKey)
			}
			if len(binding.Args) != len(tc.wantArgs) {
				t.Fatalf("binding.Args = %v, want %v", binding.Args, tc.wantArgs)
			}
			for index, want := range tc.wantArgs {
				if binding.Args[index] != want {
					t.Fatalf("binding.Args[%d] = %q, want %q", index, binding.Args[index], want)
				}
			}
		})
	}
}

func TestParseIndicatorBindingRuntimeIndicatorFamilies(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		wantKind string
		wantKey  string
		wantArgs []string
	}{
		{
			name:     "stdev source aware",
			expr:     "stdev(hlc3,20)",
			wantKind: "stdev",
			wantKey:  "stdev:hlc3:20",
			wantArgs: []string{"hlc3", "20"},
		},
		{
			name:     "variance close source",
			expr:     "variance(close,20)",
			wantKind: "variance",
			wantKey:  "variance:close:20",
			wantArgs: []string{"close", "20"},
		},
		{
			name:     "cum volume",
			expr:     "cum(volume)",
			wantKind: "cum",
			wantKey:  "cum:volume",
			wantArgs: []string{"volume"},
		},
		{
			name:     "highest with explicit source",
			expr:     "highest(high,3)",
			wantKind: "highest",
			wantKey:  "highest:high:3",
			wantArgs: []string{"high", "3"},
		},
		{
			name:     "stoch with timeframe",
			expr:     "stoch(hlc3,high,low,14,day)",
			wantKind: "stoch",
			wantKey:  "stoch:hlc3:14:day",
			wantArgs: []string{"hlc3", "14", "day"},
		},
		{
			name:     "vwap derived source",
			expr:     "vwap(hlc3)",
			wantKind: "vwap",
			wantKey:  "vwap:hlc3",
			wantArgs: []string{"hlc3"},
		},
		{
			name:     "anchored vwap",
			expr:     "anchored_vwap(ohlc4,week)",
			wantKind: "anchored_vwap",
			wantKey:  "anchored_vwap:week:ohlc4",
			wantArgs: []string{"ohlc4", "week"},
		},
		{
			name:     "mfi explicit source",
			expr:     "mfi(hlc3,14)",
			wantKind: "mfi",
			wantKey:  "mfi:hlc3:14",
			wantArgs: []string{"hlc3", "14"},
		},
		{
			name:     "dmi",
			expr:     "dmi(14,14)",
			wantKind: "dmi",
			wantKey:  "dmi:14:14",
			wantArgs: []string{"14", "14"},
		},
		{
			name:     "supertrend timeframe",
			expr:     "supertrend(3,10,day)",
			wantKind: "supertrend",
			wantKey:  "supertrend:3:10:day",
			wantArgs: []string{"3", "10", "day"},
		},
		{
			name:     "security source with lookback",
			expr:     "security_source(hlc3,day,2)",
			wantKind: "security_source",
			wantKey:  "security_source:day:hlc3:2",
			wantArgs: []string{"hlc3", "day", "2"},
		},
		{
			name:     "bollinger source and timeframe aware",
			expr:     "bollinger(20,2,day,hlc3)",
			wantKind: "bollinger",
			wantKey:  "bollinger:hlc3:20:2:day",
			wantArgs: []string{"20", "2", "day", "hlc3"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stmt := &strategyir.LetStmt{
				Range:      strategyir.SourceRange{StartLine: 28},
				Name:       "runtime",
				Expression: tc.expr,
			}
			binding, recognized, err := parseIndicatorBinding(stmt)
			if err != nil {
				t.Fatalf("parseIndicatorBinding(%q) error = %v", tc.expr, err)
			}
			if !recognized {
				t.Fatalf("parseIndicatorBinding(%q) recognized = false", tc.expr)
			}
			if binding.Kind != tc.wantKind || binding.Key != tc.wantKey {
				t.Fatalf("binding = %#v, want kind=%q key=%q", binding, tc.wantKind, tc.wantKey)
			}
			if !reflect.DeepEqual(binding.Args, tc.wantArgs) {
				t.Fatalf("binding.Args = %v, want %v", binding.Args, tc.wantArgs)
			}
		})
	}
}

func TestStrategyRuntimeParseIndicatorBindingDelegatesAndCaches(t *testing.T) {
	stmt := &strategyir.LetStmt{
		Range:      strategyir.SourceRange{StartLine: 34},
		Name:       "rank",
		Expression: "percentrank(hlc3,14,hour)",
	}

	var nilRuntime *strategyRuntime
	binding, recognized, err := nilRuntime.parseIndicatorBinding(stmt)
	if err != nil || !recognized || binding.Key != "percentrank:hlc3:14:hour" {
		t.Fatalf("nil runtime parseIndicatorBinding() = %#v, %v, %v", binding, recognized, err)
	}

	runtime := &strategyRuntime{bindingCache: map[*strategyir.LetStmt]cachedIndicatorBinding{}}
	first, firstRecognized, firstErr := runtime.parseIndicatorBinding(stmt)
	if firstErr != nil || !firstRecognized {
		t.Fatalf("first parseIndicatorBinding() = %#v, %v, %v", first, firstRecognized, firstErr)
	}

	stmt.Expression = "ma(UNKNOWN,14)"
	cached, cachedRecognized, cachedErr := runtime.parseIndicatorBinding(stmt)
	if cachedErr != nil || !cachedRecognized {
		t.Fatalf("cached parseIndicatorBinding() = %#v, %v, %v", cached, cachedRecognized, cachedErr)
	}
	if !reflect.DeepEqual(cached, first) {
		t.Fatalf("cached binding = %#v, want %#v", cached, first)
	}
}

func TestBuildDivergenceRequirementKeySupportsAllRuntimeIndicators(t *testing.T) {
	tests := []struct {
		name    string
		binding indicatorBinding
		wantKey string
		wantOK  bool
	}{
		{
			name:    "rsi",
			binding: indicatorBinding{Kind: "rsi", Key: "rsi:14"},
			wantKey: "divergence:rsi:14:bottom:8",
			wantOK:  true,
		},
		{
			name:    "kdj",
			binding: indicatorBinding{Kind: "kdj", Key: "kdj:9:3:3"},
			wantKey: "divergence:kdj:9:3:3:bottom:8",
			wantOK:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key, ok := buildDivergenceRequirementKey(tc.binding, "bottom", 8)
			if ok != tc.wantOK || key != tc.wantKey {
				t.Fatalf("buildDivergenceRequirementKey(%s) = %q, %v", tc.binding.Kind, key, ok)
			}
		})
	}
}
