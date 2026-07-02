package ir

import (
	"strings"
	"testing"
)

func TestParseIndicatorBindingSupportedMatrix(t *testing.T) {
	tests := []struct {
		expr string
		kind string
		key  string
	}{
		{"variance(close, 20)", "variance", "variance:close:20"},
		{"vwap(hlc3)", "vwap", "vwap:hlc3"},
		{"mfi(hlc3, 14)", "mfi", "mfi:hlc3:14"},
		{"dmi(14, 14)", "dmi", "dmi:14:14"},
		{"bollinger(20, 2)", "bollinger", "bollinger:20:2"},
		{"williams_r(14)", "williamsr", "williamsr:14"},
		{"kdj(9, 3, 3)", "kdj", "kdj:9:3:3"},
		{"atr(14)", "atr", "atr:14"},
		{"stdev(close, 20)", "stdev", "stdev:20"},
		{"cci(hlc3, 20)", "cci", "cci:20"},
		{"rising(close, 3)", "rising", "rising:close:3"},
		{"security_source(close, day)", "security_source", "security_source:day:close"},
		{"cog(close, 10)", "cog", "cog:close:10"},
		{"dev(close, 10)", "dev", "dev:close:10"},
		{"median(close, 10)", "median", "median:close:10"},
		{"percentrank(close, 10)", "percentrank", "percentrank:close:10"},
		{"bbw(close, 20, 2)", "bbw", "bbw:close:20:2"},
		{"tsi(close, 13, 25)", "tsi", "tsi:close:13:25"},
		{"correlation(close, open, 20)", "correlation", "correlation:close:open:20"},
		{"percentile_linear_interpolation(close, 20, 90)", "percentile_linear_interpolation", "percentile_linear_interpolation:close:20:90"},
		{"percentile_nearest_rank(close, 20, 90)", "percentile_nearest_rank", "percentile_nearest_rank:close:20:90"},
		{"swma(close)", "swma", "swma:close"},
		{"linreg(close, 20, 1)", "linreg", "linreg:close:20:1"},
		{"obv(volume, day)", "obv", "obv:volume:day"},
		{"pivothigh(close, 2, 3)", "pivothigh", "pivothigh:close:2:3"},
		{"kc(close, 20, 1.5)", "kc", "kc:close:20:1.5:true"},
		{"alma(close, 9, 0.85, 6)", "alma", "alma:close:9:0.85:6"},
	}
	for _, tt := range tests {
		t.Run(tt.kind+"_"+tt.key, func(t *testing.T) {
			binding, recognized, err := parseIndicatorBinding(&LetStmt{Range: SourceRange{StartLine: 7}, Name: "value", Expression: tt.expr})
			if err != nil || !recognized {
				t.Fatalf("parseIndicatorBinding(%q) = (%#v, %t, %v)", tt.expr, binding, recognized, err)
			}
			if binding.Kind != tt.kind || binding.Key != tt.key || binding.Alias != "value" {
				t.Fatalf("binding = %#v, want kind=%q key=%q", binding, tt.kind, tt.key)
			}
		})
	}
}

func TestParseIndicatorBindingValidationMatrix(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"rsi source", "rsi(spread, 14)", "source"},
		{"rsi period", "rsi(close, 0)", "positive integer"},
		{"kdj period", "kdj(9, 0, 3)", "positive integer"},
		{"atr period", "atr(0)", "positive integer"},
		{"stdev source", "stdev(spread, 20)", "source"},
		{"variance source", "variance(spread, 20)", "source"},
		{"variance period", "variance(close, 0)", "positive integer"},
		{"cum count", "cum()", "one source"},
		{"window count", "highest(close)", "source and length"},
		{"window period", "highest(close, 0)", "positive integer"},
		{"stoch count", "stoch(close, high, low)", "requires source"},
		{"stoch source", "stoch(volume, high, low, 14)", "source"},
		{"stoch period", "stoch(close, high, low, 0)", "positive integer"},
		{"stoch timeframe", "stoch(close, high, low, 14, quarter)", "time unit"},
		{"cci source", "cci(spread, 20)", "source"},
		{"vwap count", "vwap()", "one source"},
		{"vwap source", "vwap(spread)", "source"},
		{"anchored vwap count", "anchored_vwap(close)", "requires source"},
		{"mfi source", "mfi(spread, 14)", "source"},
		{"supertrend count", "supertrend(3)", "requires factor"},
		{"supertrend factor", "supertrend(0, 10)", "factor"},
		{"supertrend period", "supertrend(3, 0)", "atrPeriod"},
		{"sar count", "sar(0.02, 0.02)", "requires start"},
		{"sar start", "sar(0, 0.02, 0.2)", "start"},
		{"sar increment", "sar(0.02, 0, 0.2)", "increment"},
		{"sar max", "sar(0.02, 0.02, 0)", "max"},
		{"security count", "security_source(close)", "requires source"},
		{"security source", "security_source(spread, day)", "source"},
		{"security timeframe", "security_source(close, quarter)", "time unit"},
		{"williams period", "williams_r(0)", "positive integer"},
		{"bollinger count", "bollinger(20)", "requires period"},
		{"bollinger period", "bollinger(0, 2)", "period"},
		{"bollinger multiplier", "bollinger(20, 0)", "multiplier"},
		{"macd count", "macd(12, 26)", "requires fast"},
		{"macd period", "macd(12, 0, 9)", "positive integer"},
		{"cog extra args", "cog(close, 14, day, extra)", "invalid argument count"},
		{"cog count", "cog(close)", "requires source and length"},
		{"cog source", "cog(spread, 14)", "source"},
		{"cog period", "cog(close, 0)", "length"},
		{"bbw count", "bbw(close, 20)", "requires source"},
		{"bbw source", "bbw(spread, 20, 2)", "source"},
		{"bbw period", "bbw(close, 0, 2)", "length"},
		{"bbw multiplier", "bbw(close, 20, 0)", "multiplier"},
		{"tsi count", "tsi(close, 13)", "requires source"},
		{"tsi source", "tsi(spread, 13, 25)", "source"},
		{"tsi short", "tsi(close, 0, 25)", "short length"},
		{"tsi long", "tsi(close, 13, 0)", "long length"},
		{"correlation count", "correlation(close, 20)", "requires source"},
		{"correlation source", "correlation(spread, open, 20)", "source"},
		{"correlation second source", "correlation(close, spread, 20)", "source"},
		{"correlation period", "correlation(close, open, 0)", "length"},
		{"percentile count", "percentile_linear_interpolation(close, 20)", "requires source"},
		{"percentile source", "percentile_linear_interpolation(spread, 20, 90)", "source"},
		{"percentile period", "percentile_linear_interpolation(close, 0, 90)", "length"},
		{"percentile range", "percentile_linear_interpolation(close, 20, 101)", "between 0 and 100"},
		{"swma count", "swma()", "one source"},
		{"swma source", "swma(spread)", "source"},
		{"linreg count", "linreg(close, 20)", "requires source"},
		{"linreg source", "linreg(spread, 20, 1)", "source"},
		{"linreg period", "linreg(close, 0, 1)", "length"},
		{"obv extra args", "obv(close, day, extra)", "invalid argument count"},
		{"obv source", "obv(spread)", "source"},
		{"pivot count", "pivothigh(2)", "requires left and right"},
		{"pivot source", "pivothigh(spread, 2, 3)", "source"},
		{"pivot left", "pivothigh(0, 3)", "left bars"},
		{"pivot right", "pivothigh(2, 0)", "right bars"},
		{"kc count", "kc(close, 20)", "requires source"},
		{"kc source", "kc(spread, 20, 1.5)", "source"},
		{"kc period", "kc(close, 0, 1.5)", "length"},
		{"kc multiplier", "kc(close, 20, 0)", "multiplier"},
		{"alma count", "alma(close, 9, 0.85)", "requires source"},
		{"alma source", "alma(spread, 9, 0.85, 6)", "source"},
		{"alma period", "alma(close, 0, 0.85, 6)", "length"},
		{"alma offset", "alma(close, 9, bad, 6)", "offset"},
		{"alma sigma", "alma(close, 9, 0.85, 0)", "sigma"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, recognized, err := parseIndicatorBinding(&LetStmt{Range: SourceRange{StartLine: 19}, Name: "bad", Expression: tt.expr})
			if err == nil || recognized || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parseIndicatorBinding(%q) = recognized:%t error:%v, want %q", tt.expr, recognized, err, tt.want)
			}
		})
	}
}

func TestCollectExpressionRequirementsHandlesInvalidAndBaseIndicatorForms(t *testing.T) {
	ignored := []string{
		"stdev(close, 0)",
		"rsi(close, 0)",
		"macd(0, 26, 9)",
		"atr(0)",
		"bollinger(0, 2)",
		"bollinger(20, 0)",
		"cci(close, 0)",
		"variance(close, 0)",
		"supertrend(0, 10)",
		"supertrend(3, 0)",
		"sar(0, 0.02, 0.2)",
	}
	for _, expression := range ignored {
		t.Run("ignored_"+expression, func(t *testing.T) {
			requirements, err := collectExpressionRequirements(30, expression)
			if err != nil || len(requirements) != 0 {
				t.Fatalf("collectExpressionRequirements(%q) = (%#v, %v), want ignored", expression, requirements, err)
			}
		})
	}

	base := []struct {
		expr string
		key  string
	}{
		{"rsi(close, 14)", "rsi:14"},
		{"macd(12, 26, 9)", "macd:12:26:9"},
		{"atr(14)", "atr:14"},
		{"bollinger(20, 2)", "bollinger:20:2"},
		{"supertrend(3, 10)", "supertrend:3:10"},
	}
	for _, tt := range base {
		t.Run("base_"+tt.key, func(t *testing.T) {
			requirements, err := collectExpressionRequirements(31, tt.expr)
			if err != nil || len(requirements) != 1 || requirements[0].Key != tt.key {
				t.Fatalf("collectExpressionRequirements(%q) = (%#v, %v), want %q", tt.expr, requirements, err, tt.key)
			}
		})
	}

	invalid := []struct {
		expr string
		want string
	}{
		{"stdev(spread, 20)", "stdev() source"},
		{"cci(spread, 20)", "cci() source"},
		{"variance(spread, 20)", "variance() source"},
		{"cum(spread)", "cum() source"},
		{"highest(spread, 20)", "highest() source"},
		{"highest(close, 0)", "length must be"},
		{"stoch(volume, high, low, 14)", "stoch() source"},
		{"stoch(close, high, low, 0)", "length must be"},
		{"stoch(close, high, low, 14, quarter)", "time unit"},
		{"vwap(spread)", "vwap() source"},
		{"anchored_vwap(close, quarter)", "day/week/month"},
		{"mfi(close, 0)", "length must be"},
		{"supertrend(3, 10, quarter)", "timeframe"},
		{"ma(BAD, 20)", "type"},
		{"ma(EMA, 0)", "period"},
		{"ma(EMA, 20, day, spread)", "source"},
		{"security_source(close, day, -1)", "lookback"},
	}
	for _, tt := range invalid {
		t.Run("invalid_"+tt.expr, func(t *testing.T) {
			_, err := collectExpressionRequirements(32, tt.expr)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("collectExpressionRequirements(%q) error = %v, want %q", tt.expr, err, tt.want)
			}
		})
	}
}
