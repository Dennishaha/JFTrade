package pine

import (
	"strings"
	"testing"
)

func TestCoverage98RequestSecurityDiagnosticsRejectUnsafeOrAmbiguousInputs(t *testing.T) {
	cases := []struct {
		name string
		line string
		code string
	}{
		{name: "unclosed call", line: `value = request.security(`, code: "PINE_REQUEST_SECURITY_UNSUPPORTED"},
		{name: "missing expression", line: `value = request.security(syminfo.tickerid, "60")`, code: "PINE_REQUEST_SECURITY_UNSUPPORTED"},
		{name: "lookahead", line: `value = request.security(syminfo.tickerid, "60", close, barmerge.gaps_off, barmerge.lookahead_on)`, code: "PINE_REQUEST_SECURITY_LOOKAHEAD"},
		{name: "gaps", line: `value = request.security(syminfo.tickerid, "60", close, barmerge.gaps_on)`, code: "PINE_REQUEST_SECURITY_GAPS"},
		{name: "calc bars count", line: `value = request.security(syminfo.tickerid, "60", close, calc_bars_count=100)`, code: "PINE_REQUEST_SECURITY_CALC_BARS_COUNT"},
		{name: "external symbol", line: `value = request.security("NASDAQ:AAPL", "60", close)`, code: "PINE_REQUEST_SECURITY_DYNAMIC_SYMBOL"},
		{name: "dynamic timeframe", line: `value = request.security(syminfo.tickerid, timeframe.input("60"), close)`, code: "PINE_REQUEST_SECURITY_DYNAMIC_TIMEFRAME"},
		{name: "nested call", line: `value = request.security(syminfo.tickerid, "60", request.security(syminfo.tickerid, "5", close))`, code: "PINE_REQUEST_SECURITY_NESTED"},
		{name: "strategy side effect", line: `value = request.security(syminfo.tickerid, "60", strategy.position_size)`, code: "PINE_REQUEST_SECURITY_SIDE_EFFECT"},
		{name: "tuple without aliases", line: `value = request.security(syminfo.tickerid, "60", [close, open])`, code: "PINE_REQUEST_SECURITY_TUPLE_ASSIGNMENT"},
		{name: "tuple alias mismatch", line: `[only] = request.security(syminfo.tickerid, "60", [close, open])`, code: "PINE_REQUEST_SECURITY_TUPLE_MISMATCH"},
		{name: "unsupported ta", line: `value = request.security(syminfo.tickerid, "60", ta.sum(close, 5))`, code: "PINE_REQUEST_SECURITY_EXPRESSION_UNSUPPORTED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			diagnostic, ok := requestSecurityUnsupportedDiagnostic(parsedLine{number: 17, trimmed: tc.line})
			if !ok || diagnostic.Code != tc.code || diagnostic.Line != 17 {
				t.Fatalf("request.security diagnostic = %+v, ok=%v; want %q", diagnostic, ok, tc.code)
			}
		})
	}

	for _, tc := range []struct {
		expression string
		want       bool
	}{
		{expression: `close := open`, want: true},
		{expression: `array.push(values, close)`, want: true},
		{expression: `map.put(cache, "x", close)`, want: true},
		{expression: `line.new(bar_index, close, bar_index, open)`, want: true},
		{expression: `close + math.max(open, high)`, want: false},
	} {
		if got := requestSecurityExpressionHasSideEffect(tc.expression); got != tc.want {
			t.Fatalf("side-effect classification for %q = %v, want %v", tc.expression, got, tc.want)
		}
	}
}

func TestCoverage98RequestSecurityLoweringRetainsOnlyPureStaticExpressions(t *testing.T) {
	if lowered, ok := lowerSupportedRequestSecurity([]string{"syminfo.tickerid", `"60"`, "close[3]"}); !ok || lowered != "security_source(close, hour, 3)" {
		t.Fatalf("source-history lowering = %q, %v", lowered, ok)
	}
	if lowered, ok := lowerSupportedRequestSecurity([]string{"syminfo.tickerid", `"60"`, "ta.sma(hl2, 7)"}); !ok || lowered != "ma(SMA, 7, hour, hl2)" {
		t.Fatalf("source-aware MA lowering = %q, %v", lowered, ok)
	}
	if lowered, ok := lowerSupportedRequestSecurity([]string{"syminfo.tickerid", `"60"`, "ta.obv"}); !ok || lowered == "" {
		t.Fatalf("OBV lowering = %q, %v", lowered, ok)
	}
	if lowered, ok := lowerSupportedRequestSecurity([]string{"other", `"60"`, "close"}); ok || lowered != "" {
		t.Fatalf("dynamic-symbol lowering = %q, %v", lowered, ok)
	}
	if lowered, ok := lowerSupportedRequestSecurity([]string{"syminfo.tickerid", `"D"`, "ta.obv"}); ok || lowered != "" {
		t.Fatalf("unsupported daily advanced lowering = %q, %v", lowered, ok)
	}

	if tuple, ok := lowerSupportedRequestSecurityTupleGeneral([]string{"syminfo.tickerid", `"60"`, "[close, ta.rsi(close, 14), high[1]]"}); !ok || len(tuple) != 3 || !strings.Contains(tuple[1], "rsi(") || tuple[2] != "security_source(high, hour, 1)" {
		t.Fatalf("general tuple lowering = %#v, %v", tuple, ok)
	}
	if tuple, ok := lowerSupportedRequestSecurityTupleGeneral([]string{"syminfo.tickerid", `"60"`, "[close]"}); ok || tuple != nil {
		t.Fatalf("single-value tuple must remain unsupported: %#v, %v", tuple, ok)
	}

	for _, tc := range []struct {
		expression string
		wantOK     bool
	}{
		{expression: `close + high[2] + ta.obv`, wantOK: true},
		{expression: `ta.sma(close, 20`, wantOK: false},
		{expression: `ta.stoch(volume, high, low, 14)`, wantOK: false},
		{expression: `close[501]`, wantOK: false},
		{expression: `strategy.position_size`, wantOK: false},
	} {
		lowered, ok := lowerPureRequestSecurityExpression(tc.expression, "minute")
		if ok != tc.wantOK {
			t.Fatalf("pure lowering for %q = %q, %v; want ok=%v", tc.expression, lowered, ok, tc.wantOK)
		}
	}
}
