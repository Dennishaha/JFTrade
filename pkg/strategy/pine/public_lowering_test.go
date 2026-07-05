package pine

import (
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestParseScriptPublicEntryReturnsProgramAndPropagatesErrors(t *testing.T) {
	program, err := ParseScript(`//@version=6
strategy("Public Parse", overlay=true)
if close > open
    strategy.entry("Long", strategy.long, qty=1)`)
	if err != nil {
		t.Fatalf("ParseScript() error = %v", err)
	}
	if program == nil || program.Metadata.Name != "Public Parse" || len(program.Hooks) != 1 {
		t.Fatalf("program = %#v", program)
	}
	ifStmt, ok := program.Hooks[0].Statements[0].(*strategyir.IfStmt)
	if !ok {
		t.Fatalf("first statement = %T", program.Hooks[0].Statements[0])
	}
	order, ok := ifStmt.Then[0].(*strategyir.OrderStmt)
	if !ok || order.Action != strategyir.OrderActionBuy || order.QuantityExpression != "1" {
		t.Fatalf("then order = %#v/%v", ifStmt.Then[0], ok)
	}

	if _, err := ParseScript(`//@version=6
strategy("Bad Public Parse")
value = ma(EMA, 20)`); err == nil || !strings.Contains(err.Error(), "internal JFTrade helper") {
		t.Fatalf("ParseScript invalid helper error = %v", err)
	}
}

func TestTALoweringBoundariesPreserveInvalidNativeCalls(t *testing.T) {
	if got := replaceTAMacd("fast, slow, hist = ta.macd(close, 5, 13, 4)"); got != "fast, slow, hist = macd(5, 13, 4)" {
		t.Fatalf("replaceTAMacd explicit = %q", got)
	}
	if got := replaceTAMacd("fast, slow, hist = ta.macd(close)"); got != "fast, slow, hist = macd(12, 26, 9)" {
		t.Fatalf("replaceTAMacd defaults = %q", got)
	}
	if raw := "fast = ta.macd(close"; replaceTAMacd(raw) != raw {
		t.Fatalf("malformed macd call should be preserved")
	}

	if got := replaceTASourceRequiredFunction("moment = ta.change(close)", "change", "change"); got != "moment = change(close)" {
		t.Fatalf("replaceTASourceRequiredFunction = %q", got)
	}
	if raw := "moment = ta.change(close, open)"; replaceTASourceRequiredFunction(raw, "change", "change") != raw {
		t.Fatalf("multi-arg source-required call should be preserved")
	}

	if got := replaceTAStoch("k = ta.stoch(high, low, close, 14)"); got != "k = stoch(high, low, close, 14)" {
		t.Fatalf("replaceTAStoch = %q", got)
	}
	if raw := "k = ta.stoch(high, low, close)"; replaceTAStoch(raw) != raw {
		t.Fatalf("short stoch call should be preserved")
	}
}

func TestPineMovingAverageTypeCoversNativeAliases(t *testing.T) {
	cases := map[string]string{
		"ema":  "EMA",
		"SMA":  "SMA",
		" rma": "SMMA",
		"wma ": "LWMA",
		"HMA":  "HMA",
		"vwma": "VWMA",
	}
	for raw, want := range cases {
		got, ok := pineMovingAverageType(raw)
		if !ok || got != want {
			t.Fatalf("pineMovingAverageType(%q) = %q/%v, want %q/true", raw, got, ok, want)
		}
	}
	if got, ok := pineMovingAverageType("kama"); ok || got != "" {
		t.Fatalf("unknown MA = %q/%v, want empty/false", got, ok)
	}
}

func TestRequestSecurityArgsFromLineCoversAssignmentForms(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []string
	}{
		{
			name: "plain assignment",
			line: `daily = request.security(syminfo.tickerid, "D", close)`,
			want: []string{"syminfo.tickerid", `"D"`, "close"},
		},
		{
			name: "tuple assignment",
			line: `[dailyOpen, dailyClose] = request.security(syminfo.tickerid, "D", [open, close])`,
			want: []string{"syminfo.tickerid", `"D"`, "[open, close]"},
		},
		{
			name: "standalone call",
			line: `request.security(syminfo.tickerid, "60", ta.ema(close, 20))`,
			want: []string{"syminfo.tickerid", `"60"`, "ta.ema(close, 20)"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := requestSecurityArgsFromLine(tc.line)
			if !ok {
				t.Fatalf("requestSecurityArgsFromLine(%q) did not match", tc.line)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("args = %#v, want %#v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("args[%d] = %q, want %q in %#v", i, got[i], tc.want[i], got)
				}
			}
		})
	}

	for _, line := range []string{"close > open", "daily = request.security(syminfo.tickerid, \"D\", close"} {
		if got, ok := requestSecurityArgsFromLine(line); ok || got != nil {
			t.Fatalf("requestSecurityArgsFromLine(%q) = %#v/%v, want nil/false", line, got, ok)
		}
	}
}

func TestStrategyQuantityAndMetadataBoundaries(t *testing.T) {
	modeCases := map[string]struct {
		want string
		ok   bool
	}{
		"":                           {want: "fixed", ok: true},
		"strategy.fixed":             {want: "fixed", ok: true},
		" strategy.cash ":            {want: "cash", ok: true},
		"strategy.percent_of_equity": {want: "percent_of_equity", ok: true},
		"strategy.contracts":         {ok: false},
	}
	for raw, want := range modeCases {
		got, ok := normalizeStrategyDefaultQtyMode(raw)
		if got != want.want || ok != want.ok {
			t.Fatalf("normalizeStrategyDefaultQtyMode(%q) = %q/%v, want %q/%v", raw, got, ok, want.want, want.ok)
		}
	}

	pyramidingCases := map[string]struct {
		want int
		ok   bool
	}{
		"0":   {want: 1, ok: true},
		"3":   {want: 3, ok: true},
		"(2)": {want: 2, ok: true},
		"-1":  {want: 1, ok: false},
		"foo": {want: 1, ok: false},
	}
	for raw, want := range pyramidingCases {
		got, ok := parseStrategyPyramiding(raw)
		if got != want.want || ok != want.ok {
			t.Fatalf("parseStrategyPyramiding(%q) = %d/%v, want %d/%v", raw, got, ok, want.want, want.ok)
		}
	}

	quantityCases := []struct {
		args     []string
		wantMode string
		wantExpr string
		wantOK   bool
	}{
		{args: []string{"qty_percent=25"}, wantMode: "account_position_percent", wantExpr: "25", wantOK: true},
		{args: []string{"qty=strategy.equity * 15 / 100 / close"}, wantMode: "account_position_percent", wantExpr: "15", wantOK: true},
		{args: []string{"qty=5000 / close"}, wantMode: "amount", wantExpr: "5000", wantOK: true},
		{args: []string{"qty=shares"}, wantMode: "shares", wantExpr: "shares", wantOK: true},
		{args: []string{"2500 / close"}, wantMode: "amount", wantExpr: "2500", wantOK: true},
		{args: []string{"risk=1"}, wantOK: false},
	}
	for _, tc := range quantityCases {
		mode, expression, ok := pineExplicitQuantity(tc.args)
		if mode != tc.wantMode || expression != tc.wantExpr || ok != tc.wantOK {
			t.Fatalf("pineExplicitQuantity(%#v) = %q/%q/%v, want %q/%q/%v", tc.args, mode, expression, ok, tc.wantMode, tc.wantExpr, tc.wantOK)
		}
	}

	state := parseState{strategyMetadata: strategyir.StrategyMetadata{DefaultQtyMode: "cash", DefaultQtyValue: "7500"}}
	if mode, expr := state.pineEntryQuantity(nil); mode != "amount" || expr != "7500" {
		t.Fatalf("cash default quantity = %q/%q", mode, expr)
	}
	state.strategyMetadata = strategyir.StrategyMetadata{DefaultQtyMode: "percent_of_equity", DefaultQtyValue: "12.5"}
	if mode, expr := state.pineEntryQuantity(nil); mode != "account_position_percent" || expr != "12.5" {
		t.Fatalf("percent default quantity = %q/%q", mode, expr)
	}
	state.strategyMetadata = strategyir.StrategyMetadata{}
	if mode, expr := state.pineEntryQuantity(nil); mode != "shares" || expr != "1" {
		t.Fatalf("empty default quantity = %q/%q", mode, expr)
	}
}
