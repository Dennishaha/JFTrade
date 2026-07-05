package pine

import (
	"strconv"
	"strings"
	"testing"
)

func TestRequestSecurityLoweringHelperBusinessBoundaries(t *testing.T) {
	original := `x = request.security(syminfo.tickerid, "D", close`
	if got := replaceSupportedRequestSecurity(original); got != original {
		t.Fatalf("malformed request.security replacement = %q, want original", got)
	}
	unsupported := `x = request.security("NASDAQ:AAPL", "D", close)`
	if got := replaceSupportedRequestSecurity(unsupported); got != unsupported {
		t.Fatalf("unsupported symbol replacement = %q, want original", got)
	}
	if got := replaceSupportedRequestSecurity(`x = request.security(syminfo.tickerid, "D", close) + request.security(syminfo.tickerid, "D", open)`); !strings.Contains(got, "security_source(close, day)") || !strings.Contains(got, "security_source(open, day)") {
		t.Fatalf("supported replacement = %q", got)
	}

	lowered, ok := lowerSupportedRequestSecurity([]string{"syminfo.tickerid", `"15"`, "ta.ema(hlc3, 20)", "barmerge.gaps_off", "barmerge.lookahead_off"})
	if !ok || lowered != `ma(EMA, 20, "15m", hlc3)` {
		t.Fatalf("lowerSupportedRequestSecurity EMA = %q/%v", lowered, ok)
	}
	for _, args := range [][]string{
		{"syminfo.tickerid", `"15"`},
		{`"NASDAQ:AAPL"`, `"15"`, "close"},
		{"syminfo.tickerid", "tf + \"\"", "close"},
		{"syminfo.tickerid", `"15"`, "close", "barmerge.gaps_on"},
	} {
		if lowered, ok := lowerSupportedRequestSecurity(args); ok || lowered != "" {
			t.Fatalf("lowerSupportedRequestSecurity(%#v) = %q/%v, want empty false", args, lowered, ok)
		}
	}

	generalTuple, ok := lowerSupportedRequestSecurityTupleGeneral([]string{"syminfo.tickerid", `"15"`, "[open, high, low, close]"})
	if !ok || len(generalTuple) != 4 || generalTuple[0] != `security_source(open, "15m")` || generalTuple[3] != `security_source(close, "15m")` {
		t.Fatalf("lowerSupportedRequestSecurityTupleGeneral = %#v/%v", generalTuple, ok)
	}
	if tuple, ok := lowerSupportedRequestSecurityTuple([]string{"syminfo.tickerid", `"15"`, "[open, high, low, close]"}); ok || tuple != nil {
		t.Fatalf("lowerSupportedRequestSecurityTuple width four = %#v/%v, want nil false", tuple, ok)
	}
	for _, args := range [][]string{
		{`"NASDAQ:AAPL"`, `"15"`, "[open, close]"},
		{"syminfo.tickerid", "tf", "[open, close]"},
		{"syminfo.tickerid", `"15"`, "close"},
		{"syminfo.tickerid", `"15"`, "[close]"},
		{"syminfo.tickerid", `"15"`, "[close, request.security(syminfo.tickerid, \"D\", open)]"},
		{"syminfo.tickerid", `"15"`, "[close, ta.not_supported(close)]"},
	} {
		if tuple, ok := lowerSupportedRequestSecurityTupleGeneral(args); ok || tuple != nil {
			t.Fatalf("lowerSupportedRequestSecurityTupleGeneral(%#v) = %#v/%v, want nil false", args, tuple, ok)
		}
	}

	innerCases := []struct {
		name string
		expr string
		want string
	}{
		{name: "source history", expr: "close[2]", want: `security_source(close, "15m", 2)`},
		{name: "ta obv property", expr: "ta.obv", want: `obv(close, "15m")`},
		{name: "atr default", expr: "ta.atr()", want: `(atr(14, "15m"))`},
		{name: "stoch", expr: "ta.stoch(close, high, low, 14)", want: `(stoch(close, high, low, 14, "15m"))`},
		{name: "pure expression", expr: "nz(close[1], open) > hl2", want: `(nz(security_source(close, "15m", 1), security_source(open, "15m")) > security_source(hl2, "15m"))`},
	}
	for _, tc := range innerCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := lowerSupportedRequestSecurityInner(tc.expr, strconv.Quote("15m"))
			if !ok || got != tc.want {
				t.Fatalf("lowerSupportedRequestSecurityInner(%q) = %q/%v, want %q/true", tc.expr, got, ok, tc.want)
			}
		})
	}
	for _, expr := range []string{
		"request.security(syminfo.tickerid, \"D\", close)",
		"ta.ema(strategy.position_size, 10)",
		"ta.macd(close, 12, 26)",
		"ta.stoch(volume, high, low, 14)",
		"ta.not_supported(close)",
		"[close, open]",
		"close => open",
		"close[" + strconv.Itoa(maxHistoryLookback+1) + "]",
	} {
		if got, ok := lowerSupportedRequestSecurityInner(expr, strconv.Quote("15m")); ok || got != "" {
			t.Fatalf("lowerSupportedRequestSecurityInner(%q) = %q/%v, want empty false", expr, got, ok)
		}
	}
}

func TestRequestSecurityPurityAndMergeArgumentBoundaries(t *testing.T) {
	if got, ok := lowerPureRequestSecurityExpression(`str.upper("ok") == "OK" and timeframe.in_seconds("15") > 0 and math.sqrt(close) > 1`, strconv.Quote("15m")); !ok || !strings.Contains(got, "str_upper") || !strings.Contains(got, "timeframe_in_seconds") || !strings.Contains(got, "sqrt") {
		t.Fatalf("lowerPureRequestSecurityExpression helpers = %q/%v", got, ok)
	}
	if got, ok := lowerPureRequestSecurityExpression(`ta.obv + close`, strconv.Quote("15m")); !ok || !strings.Contains(got, `obv(close, "15m")`) || !strings.Contains(got, `security_source(close, "15m")`) {
		t.Fatalf("lowerPureRequestSecurityExpression ta.obv property = %q/%v", got, ok)
	}
	for _, expr := range []string{
		"alert(\"no\")",
		"array.get(values, 0)",
		"ta.bad(close)",
		"ta.ema(close, 14",
		"close[" + strconv.Itoa(maxHistoryLookback+1) + "]",
	} {
		if got, ok := lowerPureRequestSecurityExpression(expr, strconv.Quote("15m")); ok || got != "" {
			t.Fatalf("lowerPureRequestSecurityExpression(%q) = %q/%v, want empty false", expr, got, ok)
		}
	}

	if !requestSecurityLoweredASTIsPure(`close > open ? security_source(close, "15m") : -security_source(open, "15m")`) {
		t.Fatal("conditional lowered request.security expression should be pure")
	}
	for _, expr := range []string{"unknown(close)", "security_source(close, 15m", "foo.bar()"} {
		if requestSecurityLoweredASTIsPure(expr) {
			t.Fatalf("requestSecurityLoweredASTIsPure(%q) = true, want false", expr)
		}
	}
	if !pureRequestSecurityRuntimeCall("collection_array_get") || pureRequestSecurityRuntimeCall("strategy_entry") {
		t.Fatal("pureRequestSecurityRuntimeCall did not preserve collection whitelist and strategy rejection")
	}

	mergeCases := []struct {
		args []string
		ok   bool
	}{
		{args: nil, ok: true},
		{args: []string{"barmerge.gaps_off", "barmerge.lookahead_off"}, ok: true},
		{args: []string{"gaps=barmerge.gaps_off", "lookahead=barmerge.lookahead_off"}, ok: true},
		{args: []string{"barmerge.gaps_off", "barmerge.lookahead_off", "calc_bars_count=100"}, ok: false},
		{args: []string{"gaps=barmerge.gaps_on"}, ok: false},
		{args: []string{"lookahead=barmerge.lookahead_on"}, ok: false},
		{args: []string{"calc_bars_count=100"}, ok: false},
	}
	for _, tc := range mergeCases {
		if got := supportedRequestSecurityMergeArgs(tc.args); got != tc.ok {
			t.Fatalf("supportedRequestSecurityMergeArgs(%#v) = %v, want %v", tc.args, got, tc.ok)
		}
	}

	source, lookback, ok := supportedRequestSecuritySourceHistory("ohlc4[3]")
	if !ok || source != "ohlc4" || lookback != 3 {
		t.Fatalf("supportedRequestSecuritySourceHistory = %q/%d/%v", source, lookback, ok)
	}
	for _, expr := range []string{"badsource[1]", "close[-1]", "close[" + strconv.Itoa(maxHistoryLookback+1) + "]", "close[1] + open"} {
		if source, lookback, ok := supportedRequestSecuritySourceHistory(expr); ok || source != "" || lookback != 0 {
			t.Fatalf("supportedRequestSecuritySourceHistory(%q) = %q/%d/%v, want empty false", expr, source, lookback, ok)
		}
	}
	for _, source := range []string{"open", "high", "low", "close", "volume", "hl2", "hlc3", "ohlc4"} {
		if got, ok := supportedRequestSecuritySource(source); !ok || got != source {
			t.Fatalf("supportedRequestSecuritySource(%q) = %q/%v", source, got, ok)
		}
	}
	if got, ok := supportedRequestSecuritySource("vwap"); ok || got != "" {
		t.Fatalf("unsupported source = %q/%v", got, ok)
	}
	if !requestSecurityUsesTimeframeAlias(`x = request.security(syminfo.tickerid, tf, close)`) {
		t.Fatal("requestSecurityUsesTimeframeAlias(tf) = false, want true")
	}
	for _, expr := range []string{`close > open`, `x = request.security(syminfo.tickerid, "15", close`, `x = request.security(syminfo.tickerid)`} {
		if requestSecurityUsesTimeframeAlias(expr) {
			t.Fatalf("requestSecurityUsesTimeframeAlias(%q) = true, want false", expr)
		}
	}
}

func TestRequestSecurityAdvancedTALoweringBoundaries(t *testing.T) {
	successCases := []struct {
		name string
		call string
		args []string
		want string
	}{
		{name: "pivot high default source", call: "pivothigh", args: []string{"2", "3"}, want: `pivothigh(high, 2, 3, "15m")`},
		{name: "pivot low default source", call: "pivotlow", args: []string{"4", "5"}, want: `pivotlow(low, 4, 5, "15m")`},
		{name: "keltner channel default use true range", call: "kc", args: []string{"hlc3", "20", "1.5"}, want: `kc(hlc3, 20, 1.5, true, "15m")`},
		{name: "tsi", call: "tsi", args: []string{"close", "13", "25"}, want: `tsi(close, 13, 25, "15m")`},
		{name: "correlation with supported second source", call: "correlation", args: []string{"close", "volume", "20"}, want: `correlation(close, volume, 20, "15m")`},
		{name: "percentile nearest rank", call: "percentile_nearest_rank", args: []string{"close", "20", "80"}, want: `percentile_nearest_rank(close, 20, 80, "15m")`},
		{name: "swma", call: "swma", args: []string{"close"}, want: `swma(close, "15m")`},
	}
	for _, tc := range successCases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := lowerRequestSecurityTACall(tc.call, tc.args, strconv.Quote("15m"))
			if !ok || got != tc.want {
				t.Fatalf("lowerRequestSecurityTACall(%s, %#v) = %q/%v, want %q/true", tc.call, tc.args, got, ok, tc.want)
			}
		})
	}

	rejectCases := []struct {
		name     string
		call     string
		args     []string
		timeUnit string
	}{
		{name: "advanced daily timeframe rejected", call: "tsi", args: []string{"close", "13", "25"}, timeUnit: "day"},
		{name: "linreg wrong arity", call: "linreg", args: []string{"close", "20"}, timeUnit: strconv.Quote("15m")},
		{name: "correlation unsupported second source", call: "correlation", args: []string{"close", "strategy.position_size", "20"}, timeUnit: strconv.Quote("15m")},
		{name: "swma wrong arity", call: "swma", args: []string{"close", "extra"}, timeUnit: strconv.Quote("15m")},
		{name: "unsupported first source", call: "tsi", args: []string{"strategy.position_size", "13", "25"}, timeUnit: strconv.Quote("15m")},
		{name: "unknown advanced call", call: "not_supported", args: []string{"close"}, timeUnit: strconv.Quote("15m")},
	}
	for _, tc := range rejectCases {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := lowerAdvancedRequestSecurity(tc.call, tc.args, tc.timeUnit); ok || got != "" {
				t.Fatalf("lowerAdvancedRequestSecurity(%s, %#v, %q) = %q/%v, want empty false", tc.call, tc.args, tc.timeUnit, got, ok)
			}
		})
	}

	if !supportedAdvancedRequestSecurityTimeUnit("minute") || !supportedAdvancedRequestSecurityTimeUnit("hour") || !supportedAdvancedRequestSecurityTimeUnit(strconv.Quote("15m")) || supportedAdvancedRequestSecurityTimeUnit("day") {
		t.Fatal("supportedAdvancedRequestSecurityTimeUnit should allow minute/hour intraday units only")
	}
}
