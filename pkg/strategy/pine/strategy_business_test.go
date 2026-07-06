package pine

import (
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

//nolint:funlen
func TestStrategyRiskArgumentParsersCoverBusinessBoundaries(t *testing.T) {
	directionCases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "", want: "all", ok: true},
		{raw: "strategy.all", want: "all", ok: true},
		{raw: "strategy.direction.long", want: "long", ok: true},
		{raw: "strategy.short", want: "short", ok: true},
		{raw: "strategy.direction.both", ok: false},
	}
	for _, tc := range directionCases {
		got, ok := normalizeStrategyAllowedEntryDirection(tc.raw)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("normalizeStrategyAllowedEntryDirection(%q) = %q/%v, want %q/%v", tc.raw, got, ok, tc.want, tc.ok)
		}
	}

	amountTypeCases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "strategy.percent_of_equity", want: "percent_of_equity", ok: true},
		{raw: " cash ", want: "cash", ok: true},
		{raw: "strategy.fixed", ok: false},
	}
	for _, tc := range amountTypeCases {
		got, ok := normalizeStrategyRiskAmountType(tc.raw)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("normalizeStrategyRiskAmountType(%q) = %q/%v, want %q/%v", tc.raw, got, ok, tc.want, tc.ok)
		}
	}

	value, amountType, alert, err := parseStrategyRiskAmountArgs(7, "strategy.risk.max_drawdown", []string{"12.5", "strategy.cash", `"cash drawdown"`})
	if err != nil || value != 12.5 || amountType != "cash" || alert != "cash drawdown" {
		t.Fatalf("parseStrategyRiskAmountArgs positional = %v/%q/%q/%v", value, amountType, alert, err)
	}
	_, _, alert, err = parseStrategyRiskAmountArgs(8, "strategy.risk.max_intraday_loss", []string{"8", "strategy.percent_of_equity", `alert_message="day loss"`})
	if err != nil || alert != "day loss" {
		t.Fatalf("parseStrategyRiskAmountArgs named alert = %q/%v", alert, err)
	}
	for _, args := range [][]string{
		{"strategy.cash"},
		{"0", "strategy.cash"},
		{"10", "strategy.fixed"},
		{"10", "strategy.cash", "alert", "unexpected=1"},
	} {
		if _, _, _, err := parseStrategyRiskAmountArgs(9, "strategy.risk.max_drawdown", args); err == nil {
			t.Fatalf("parseStrategyRiskAmountArgs(%#v) error = nil, want validation error", args)
		}
	}

	count, alert, err := parseStrategyRiskCountArgs(10, "strategy.risk.max_cons_loss_days", []string{"3", `"loss days"`})
	if err != nil || count != 3 || alert != "loss days" {
		t.Fatalf("parseStrategyRiskCountArgs positional = %d/%q/%v", count, alert, err)
	}
	count, alert, err = parseStrategyRiskCountArgs(11, "strategy.risk.max_intraday_filled_orders", []string{"5", `alert_message="fills"`})
	if err != nil || count != 5 || alert != "fills" {
		t.Fatalf("parseStrategyRiskCountArgs named = %d/%q/%v", count, alert, err)
	}
	for _, args := range [][]string{
		nil,
		{"0"},
		{"abc"},
		{"2", "alert", "foo=bar"},
	} {
		if _, _, err := parseStrategyRiskCountArgs(12, "strategy.risk.max_cons_loss_days", args); err == nil {
			t.Fatalf("parseStrategyRiskCountArgs(%#v) error = nil, want validation error", args)
		}
	}

	positionSize, err := parseStrategyRiskPositionSize(13, []string{"2.5"})
	if err != nil || positionSize != 2.5 {
		t.Fatalf("parseStrategyRiskPositionSize valid = %v/%v", positionSize, err)
	}
	for _, args := range [][]string{{}, {"1", "2"}, {"-1"}} {
		if _, err := parseStrategyRiskPositionSize(14, args); err == nil {
			t.Fatalf("parseStrategyRiskPositionSize(%#v) error = nil, want validation error", args)
		}
	}
}

func TestCompileCoversTrailPriceExitAndShortCloseBusinessSemantics(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Trail price and short cover", overlay=true)
strategy.entry("Short", strategy.short, qty=1)
strategy.close("Short", qty_percent=25)
strategy.exit("Auto trail", trail_price=high + 1, trail_offset=2, qty=1, when=close > open, comment_trailing="trail comment", alert_trailing="trail alert", disable_alert=true)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	statements := compilation.Program.Hooks[0].Statements
	if len(statements) != 3 {
		t.Fatalf("statement count = %d, want 3", len(statements))
	}

	entry := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](statements[0])
	if entry.Action != strategyir.OrderActionShort || entry.Intent != strategyir.OrderIntentEntry {
		t.Fatalf("short entry = %#v", entry)
	}
	closeShort := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](statements[1])
	if closeShort.Action != strategyir.OrderActionCover || closeShort.Intent != strategyir.OrderIntentClose || closeShort.QuantityMode != "symbol_position_percent" || closeShort.QuantityExpression != "25" {
		t.Fatalf("short close should cover existing short entry: %#v", closeShort)
	}

	exit := jftradeCheckedTypeAssertion[*strategyir.ExitStmt](statements[2])
	if exit.Direction != "auto" || exit.FromEntry != "" || exit.TrailPrice != "high + 1" || exit.TrailOffset != "2" {
		t.Fatalf("trail price exit routing = %#v", exit)
	}
	if exit.TrailPoints != "" || exit.QuantityMode != "shares" || exit.QuantityExpression != "1" || exit.WhenExpression != "close > open" {
		t.Fatalf("trail price exit quantity/condition = %#v", exit)
	}
	if exit.CommentTrailing != "trail comment" || exit.AlertTrailing != "trail alert" || !exit.DisableAlert {
		t.Fatalf("trail price exit metadata = %#v", exit)
	}
}

func TestValidateScriptReportsRiskDeclarationBoundaryErrors(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		message string
	}{
		{name: "allow entry invalid direction", line: `strategy.risk.allow_entry_in(strategy.direction.both)`, message: "direction"},
		{name: "drawdown bad amount type", line: `strategy.risk.max_drawdown(10, strategy.fixed)`, message: "not supported"},
		{name: "filled order count must be positive", line: `strategy.risk.max_intraday_filled_orders(0)`, message: "positive constant integer"},
		{name: "position size single argument", line: `strategy.risk.max_position_size(1, 2)`, message: "requires one argument"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateScript("//@version=6\nstrategy(\"Risk boundary\", overlay=true)\n" + tc.line)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("ValidateScript() error = %v, want message containing %q", err, tc.message)
			}
		})
	}
}
