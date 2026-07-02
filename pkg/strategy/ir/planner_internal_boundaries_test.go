package ir

import (
	"slices"
	"strings"
	"testing"
)

func TestParseIndicatorBindingBusinessKeys(t *testing.T) {
	tests := []struct {
		name string
		expr string
		kind string
		key  string
		args []string
	}{
		{name: "moving average with timeframe and source", expr: `ma(HMA, 16, "60m", hlc3)`, kind: "ma", key: "ma:HMA:16:hour:hlc3", args: []string{"HMA", "16", "hour", "hlc3"}},
		{name: "rsi source aware", expr: "rsi(open, 9)", kind: "rsi", key: "rsi:open:9", args: []string{"open", "9"}},
		{name: "macd timeframe source", expr: "macd(12, 26, 9, day, hlc3)", kind: "macd", key: "macd:hlc3:12:26:9:day", args: []string{"12", "26", "9", "day", "hlc3"}},
		{name: "stoch timeframe source", expr: "stoch(hlc3, high, low, 12, day)", kind: "stoch", key: "stoch:hlc3:12:day", args: []string{"hlc3", "12", "day"}},
		{name: "anchored vwap", expr: "anchored_vwap(close, week)", kind: "anchored_vwap", key: "anchored_vwap:week:close", args: []string{"close", "week"}},
		{name: "security source lookback", expr: "security_source(volume, day, 3)", kind: "security_source", key: "security_source:day:volume:3", args: []string{"volume", "day", "3"}},
		{name: "bollinger source timeframe", expr: "bollinger(20, 2, day, hlc3)", kind: "bollinger", key: "bollinger:hlc3:20:2:day", args: []string{"20", "2", "day", "hlc3"}},
		{name: "williams alias", expr: "williamsr(14)", kind: "williamsr", key: "williamsr:14", args: []string{"14"}},
		{name: "supertrend timeframe", expr: "supertrend(3.5, 10, week)", kind: "supertrend", key: "supertrend:3.5:10:week", args: []string{"3.5", "10", "week"}},
		{name: "sar floats", expr: "sar(0.02, 0.02, 0.2)", kind: "sar", key: "sar:0.02:0.02:0.2", args: []string{"0.02", "0.02", "0.2"}},
		{name: "advanced cmo", expr: "cmo(close, 14, day)", kind: "cmo", key: "cmo:close:14:day", args: []string{"close", "14"}},
		{name: "advanced kcw", expr: "kcw(close, 20, 1.5, false, day)", kind: "kcw", key: "kcw:close:20:1.5:false:day", args: []string{"close", "20", "1.5", "false"}},
		{name: "advanced obv default source", expr: "obv()", kind: "obv", key: "obv:close", args: []string{"close"}},
		{name: "advanced pivot low default source", expr: "pivotlow(2, 3)", kind: "pivotlow", key: "pivotlow:low:2:3", args: []string{"low", "2", "3"}},
		{name: "advanced alma", expr: "alma(close, 9, 0.85, 6, day)", kind: "alma", key: "alma:close:9:0.85:6:day", args: []string{"close", "9", "0.85", "6"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binding, recognized, err := parseIndicatorBinding(&LetStmt{
				Range:      SourceRange{StartLine: 12},
				Name:       "signal",
				Expression: tt.expr,
			})
			if err != nil {
				t.Fatalf("parseIndicatorBinding(%q) error = %v", tt.expr, err)
			}
			if !recognized {
				t.Fatalf("parseIndicatorBinding(%q) recognized = false", tt.expr)
			}
			if binding.Alias != "signal" || binding.Kind != tt.kind || binding.Key != tt.key {
				t.Fatalf("binding = %+v, want alias signal kind %s key %s", binding, tt.kind, tt.key)
			}
			if !slices.Equal(binding.Args, tt.args) {
				t.Fatalf("binding args = %#v, want %#v", binding.Args, tt.args)
			}
		})
	}
}

func TestParseIndicatorBindingRejectsInvalidBusinessParameters(t *testing.T) {
	tests := []struct {
		name       string
		expr       string
		wantDetail string
	}{
		{name: "ma missing period", expr: "ma(EMA)", wantDetail: "ma() requires type"},
		{name: "ma bad optional source", expr: "ma(EMA, 9, day, spread)", wantDetail: "moving-average source"},
		{name: "cum bad source", expr: "cum(spread)", wantDetail: "cum() source"},
		{name: "window bad source", expr: "highest(close - open, 20)", wantDetail: "highest() source"},
		{name: "stoch bad high low", expr: "stoch(close, open, low, 14)", wantDetail: "literal high and low"},
		{name: "anchored vwap bad anchor", expr: "anchored_vwap(close, quarter)", wantDetail: "day/week/month"},
		{name: "mfi bad period", expr: "mfi(hlc3, 0)", wantDetail: "length must be a positive integer"},
		{name: "dmi missing smoothing", expr: "dmi(14)", wantDetail: "requires 2 positive integer"},
		{name: "supertrend bad timeframe", expr: "supertrend(3, 10, quarter)", wantDetail: "timeframe"},
		{name: "security source bad lookback", expr: "security_source(close, day, -1)", wantDetail: "lookback"},
		{name: "bollinger bad source", expr: "bollinger(20, 2, day, spread)", wantDetail: "supports OHLCV"},
		{name: "advanced linreg bad offset", expr: "linreg(close, 20, -1)", wantDetail: "offset"},
		{name: "advanced kcw bad boolean", expr: "kcw(close, 20, 1.5, maybe)", wantDetail: "useTrueRange"},
		{name: "advanced swma bad timeframe", expr: "swma(close, quarter)", wantDetail: "timeframe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, recognized, err := parseIndicatorBinding(&LetStmt{
				Range:      SourceRange{StartLine: 21},
				Name:       "bad",
				Expression: tt.expr,
			})
			if err == nil {
				t.Fatalf("parseIndicatorBinding(%q) error = nil, recognized=%v", tt.expr, recognized)
			}
			if !strings.Contains(err.Error(), tt.wantDetail) {
				t.Fatalf("parseIndicatorBinding(%q) error = %v, want detail %q", tt.expr, err, tt.wantDetail)
			}
		})
	}

	_, recognized, err := parseIndicatorBinding(&LetStmt{Range: SourceRange{StartLine: 31}, Name: "plain", Expression: "close + open"})
	if err != nil || recognized {
		t.Fatalf("plain expression recognized=%v err=%v, want ignored expression", recognized, err)
	}
}

func TestCollectExpressionRequirementsCoversCompoundIndicatorExpressions(t *testing.T) {
	requirements, err := collectExpressionRequirements(40, strings.Join([]string{
		"rsi(close, 14, day)",
		"rsi(close, 14, day)",
		"stdev(hlc3, 11)",
		"variance(volume, 8)",
		"cum(volume)",
		"highestbars(high, 10)",
		"lowestbars(low, 10)",
		"range(close, 5)",
		"mode(volume, 5)",
		"stoch(hlc3, high, low, 12, week)",
		"vwap(hlc3)",
		"anchored_vwap(close, month)",
		"mfi(hlc3, 15)",
		"dmi(14, 14)",
		"supertrend(3, 10, day)",
		"sar(0.02, 0.02, 0.2)",
		"ma(EMA, 9, day, volume)",
		"security_source(open, week, 2)",
	}, " + "))
	if err != nil {
		t.Fatalf("collectExpressionRequirements error = %v", err)
	}

	keys := map[string]bool{}
	for _, requirement := range requirements {
		keys[requirement.Key] = true
	}
	for _, key := range []string{
		"rsi:close:14:day",
		"stdev:hlc3:11",
		"variance:volume:8",
		"cum:volume",
		"highestbars:high:10",
		"lowestbars:low:10",
		"range:close:5",
		"mode:volume:5",
		"stoch:hlc3:12:week",
		"vwap:hlc3",
		"anchored_vwap:month:close",
		"mfi:hlc3:15",
		"dmi:14:14",
		"supertrend:3:10:day",
		"sar:0.02:0.02:0.2",
		"ma:EMA:9:day:volume",
		"security_source:week:open:2",
	} {
		if !keys[key] {
			t.Fatalf("missing expression requirement key %q in %#v", key, requirements)
		}
	}
	if got := len(keys); got != len(requirements) {
		t.Fatalf("requirements contain duplicate keys: %#v", requirements)
	}
}

func TestCollectExpressionRequirementsRejectsInvalidCompoundIndicatorCalls(t *testing.T) {
	tests := []struct {
		name       string
		expr       string
		wantDetail string
	}{
		{name: "rsi bad source", expr: "rsi(spread, 14)", wantDetail: "rsi() source"},
		{name: "rsi bad timeframe", expr: "rsi(close, 14, quarter)", wantDetail: "timeframe"},
		{name: "macd bad timeframe", expr: "macd(12, 26, 9, quarter, close)", wantDetail: "macd() supports"},
		{name: "atr bad timeframe", expr: "atr(14, quarter)", wantDetail: "timeframe"},
		{name: "bollinger bad source", expr: "bollinger(20, 2, day, spread)", wantDetail: "bollinger() supports"},
		{name: "stoch bad high low", expr: "stoch(close, open, low, 14)", wantDetail: "literal high and low"},
		{name: "mfi bad source", expr: "mfi(spread, 14)", wantDetail: "mfi() source"},
		{name: "security source bad source", expr: "security_source(spread, day)", wantDetail: "security_source() supports"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := collectExpressionRequirements(50, tt.expr)
			if err == nil {
				t.Fatalf("collectExpressionRequirements(%q) error = nil", tt.expr)
			}
			if !strings.Contains(err.Error(), tt.wantDetail) {
				t.Fatalf("collectExpressionRequirements(%q) error = %v, want detail %q", tt.expr, err, tt.wantDetail)
			}
		})
	}
}

func TestProtectAndDivergenceKeyBusinessBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		stmt *ProtectStmt
		want string
	}{
		{
			name: "legacy continuous stop loss",
			stmt: &ProtectStmt{Range: SourceRange{StartLine: 61}, Mode: "stop_loss", Direction: "long", TimeValueExpression: "2", TimeUnit: "bar", PercentageExpression: "4%", WindowPolicy: "continuous"},
			want: "sl:long:2:bar:4",
		},
		{
			name: "session trailing stop",
			stmt: &ProtectStmt{Range: SourceRange{StartLine: 62}, Mode: "trailing_stop", Direction: "both", TimeValueExpression: "3", TimeUnit: "day", PercentageExpression: "2.5", WindowPolicy: "session"},
			want: "risk:trailingStop:auto:3:day:2.5:session",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildProtectRequirementKey(tc.stmt)
			if err != nil {
				t.Fatalf("buildProtectRequirementKey error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("protect key = %q, want %q", got, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name       string
		stmt       *ProtectStmt
		wantDetail string
	}{
		{name: "bad mode", stmt: &ProtectStmt{Range: SourceRange{StartLine: 63}, Mode: "hedge", Direction: "long", TimeValueExpression: "2", TimeUnit: "bar", PercentageExpression: "4", WindowPolicy: "continuous"}, wantDetail: "protect mode"},
		{name: "bad direction", stmt: &ProtectStmt{Range: SourceRange{StartLine: 64}, Mode: "stop_loss", Direction: "sideways", TimeValueExpression: "2", TimeUnit: "bar", PercentageExpression: "4", WindowPolicy: "continuous"}, wantDetail: "protect direction"},
		{name: "bad time", stmt: &ProtectStmt{Range: SourceRange{StartLine: 65}, Mode: "stop_loss", Direction: "long", TimeValueExpression: "0", TimeUnit: "bar", PercentageExpression: "4", WindowPolicy: "continuous"}, wantDetail: "time value"},
		{name: "bad percent", stmt: &ProtectStmt{Range: SourceRange{StartLine: 66}, Mode: "stop_loss", Direction: "long", TimeValueExpression: "2", TimeUnit: "bar", PercentageExpression: "-1", WindowPolicy: "continuous"}, wantDetail: "percentage"},
		{name: "bad window", stmt: &ProtectStmt{Range: SourceRange{StartLine: 67}, Mode: "stop_loss", Direction: "long", TimeValueExpression: "2", TimeUnit: "bar", PercentageExpression: "4", WindowPolicy: "overnight"}, wantDetail: "window policy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildProtectRequirementKey(tc.stmt)
			if err == nil {
				t.Fatalf("buildProtectRequirementKey(%+v) error = nil", tc.stmt)
			}
			if !strings.Contains(err.Error(), tc.wantDetail) {
				t.Fatalf("protect error = %v, want detail %q", err, tc.wantDetail)
			}
		})
	}

	for _, tc := range []struct {
		name      string
		binding   plannedBinding
		direction string
		lookback  int
		want      string
		wantOK    bool
	}{
		{name: "rsi", binding: plannedBinding{Kind: "rsi", Args: []string{"14"}}, direction: "top", lookback: 5, want: "divergence:rsi:14:top:5", wantOK: true},
		{name: "macd", binding: plannedBinding{Kind: "macd", Args: []string{"12", "26", "9"}}, direction: "bottom", lookback: 6, want: "divergence:macd:12:26:9:bottom:6", wantOK: true},
		{name: "kdj", binding: plannedBinding{Kind: "kdj", Args: []string{"9", "3", "3"}}, direction: "top", lookback: 4, want: "divergence:kdj:9:3:3:top:4", wantOK: true},
		{name: "unsupported", binding: plannedBinding{Kind: "ma", Args: []string{"EMA", "9"}}, direction: "top", lookback: 4, wantOK: false},
	} {
		t.Run("divergence_"+tc.name, func(t *testing.T) {
			got, ok := buildDivergenceRequirementKey(tc.binding, tc.direction, tc.lookback)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("buildDivergenceRequirementKey = %q/%v, want %q/%v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}
