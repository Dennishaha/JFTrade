package pine

import (
	"regexp"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestParseStrategyCallCoversOrderLifecycleBusinessBoundaries(t *testing.T) {
	state := newStrategyCallBoundaryState()

	for _, line := range []string{
		"strategy.risk.allow_entry_in(strategy.direction.long)",
		`strategy.risk.max_drawdown(12.5, strategy.cash, "drawdown")`,
		`strategy.risk.max_intraday_loss(8, strategy.percent_of_equity, alert_message="day loss")`,
		`strategy.risk.max_intraday_filled_orders(5, "fills")`,
		`strategy.risk.max_position_size(2.5)`,
		`strategy.risk.max_cons_loss_days(3, alert_message="loss days")`,
	} {
		stmt, handled, err := state.parseStrategyCall(parsedLine{number: 10, trimmed: line})
		if err != nil || !handled || stmt != nil {
			t.Fatalf("parseStrategyCall(%q) stmt=%#v handled=%v err=%v", line, stmt, handled, err)
		}
	}
	if state.strategyMetadata.AllowedEntryDirection != "long" ||
		state.strategyMetadata.MaxDrawdownType != "cash" ||
		state.strategyMetadata.MaxIntradayLossType != "percent_of_equity" ||
		state.strategyMetadata.MaxIntradayFilledOrders != 5 ||
		state.strategyMetadata.MaxPositionSize != 2.5 ||
		state.strategyMetadata.MaxConsLossDays != 3 {
		t.Fatalf("risk metadata = %#v", state.strategyMetadata)
	}

	entry := parseStrategyBoundaryStatement[*strategyir.OrderStmt](t, state, `strategy.entry("Long", strategy.long, qty_percent=25, limit=close - 1, stop=close + 1, comment="enter", alert_message="entry alert", disable_alert=true, when=close > open)`)
	if entry.ID != "Long" || entry.Action != strategyir.OrderActionBuy || entry.Intent != strategyir.OrderIntentEntry {
		t.Fatalf("entry routing = %#v", entry)
	}
	if entry.QuantityMode != "account_position_percent" || entry.QuantityExpression != "25" || entry.OrderType != "LIMIT" ||
		entry.LimitExpression != "close - 1" || entry.StopExpression != "close + 1" || entry.WhenExpression != "close > open" {
		t.Fatalf("entry order details = %#v", entry)
	}
	if entry.Comment != "enter" || entry.AlertMessage != "entry alert" || !entry.DisableAlert {
		t.Fatalf("entry metadata = %#v", entry)
	}

	order := parseStrategyBoundaryStatement[*strategyir.OrderStmt](t, state, `strategy.order("Rebalance", strategy.short, qty=1000 * strategy.cash, stop=low, when=open > close)`)
	if order.Action != strategyir.OrderActionSell || order.Intent != strategyir.OrderIntentNet ||
		order.QuantityMode != "shares" || order.QuantityExpression != "1000 * strategy.cash" || order.StopExpression != "low" ||
		order.WhenExpression != "open > close" {
		t.Fatalf("net order = %#v", order)
	}

	state.shortEntryIDs["Short"] = true
	closeShort := parseStrategyBoundaryStatement[*strategyir.OrderStmt](t, state, `strategy.close("Short", qty=5, limit=low, stop=high, immediately=true, comment="cover", alert_message="cover alert", disable_alert=true, when=close < open)`)
	if closeShort.Action != strategyir.OrderActionCover || closeShort.Intent != strategyir.OrderIntentClose ||
		closeShort.QuantityMode != "shares" || closeShort.QuantityExpression != "5" || !closeShort.Immediate ||
		closeShort.LimitExpression != "low" || closeShort.StopExpression != "high" || closeShort.WhenExpression != "close < open" {
		t.Fatalf("short close = %#v", closeShort)
	}

	closeAll := parseStrategyBoundaryStatement[*strategyir.OrderStmt](t, state, `strategy.close_all(true, "panic", "flatten alert", true)`)
	if closeAll.Intent != strategyir.OrderIntentFlatten || closeAll.QuantityExpression != "100" ||
		!closeAll.Immediate || closeAll.Comment != "panic" || closeAll.AlertMessage != "flatten alert" || !closeAll.DisableAlert {
		t.Fatalf("close all = %#v", closeAll)
	}

	bracket := parseStrategyBoundaryStatement[*strategyir.ExitStmt](t, state, `strategy.exit("Bracket", "Long", profit=10, loss=5, qty_percent=50, comment_profit="tp", alert_loss="sl", disable_alert=true, when=close > open)`)
	if bracket.ID != "Bracket" || bracket.FromEntry != "Long" || bracket.Direction != "long" ||
		bracket.QuantityMode != "symbol_position_percent" || bracket.QuantityExpression != "50" ||
		bracket.ProfitExpression != "10" || bracket.LossExpression != "5" || bracket.WhenExpression != "close > open" {
		t.Fatalf("bracket exit = %#v", bracket)
	}
	if bracket.CommentProfit != "tp" || bracket.AlertLoss != "sl" || !bracket.DisableAlert {
		t.Fatalf("bracket metadata = %#v", bracket)
	}

	trailing := parseStrategyBoundaryStatement[*strategyir.ExitStmt](t, state, `strategy.exit("Trail", from_entry="Short", trail_points=12, trail_offset=2, alert_trailing="trail", when=low < close)`)
	if trailing.Direction != "short" || trailing.TrailPoints != "12" || trailing.TrailOffset != "2" ||
		trailing.AlertTrailing != "trail" || trailing.WhenExpression != "low < close" {
		t.Fatalf("trailing exit = %#v", trailing)
	}

	cancelAll := parseStrategyBoundaryStatement[*strategyir.CancelStmt](t, state, `strategy.cancel_all()`)
	if !cancelAll.All || cancelAll.ID != "" {
		t.Fatalf("cancel all = %#v", cancelAll)
	}
	cancelOne := parseStrategyBoundaryStatement[*strategyir.CancelStmt](t, state, `strategy.cancel("Long")`)
	if cancelOne.All || cancelOne.ID != "Long" {
		t.Fatalf("cancel one = %#v", cancelOne)
	}
	if stmt, handled, err := state.parseStrategyCall(parsedLine{number: 40, trimmed: `label.new(bar_index, close, "x")`}); err != nil || handled || stmt != nil {
		t.Fatalf("non strategy statement = %#v/%v/%v", stmt, handled, err)
	}
}

func TestParseStrategyCallRejectsUnsupportedOrderBoundaries(t *testing.T) {
	state := newStrategyCallBoundaryState()
	cases := []string{
		`strategy.entry("Long")`,
		`strategy.entry("Long", strategy.long, qty=1, qty_percent=10)`,
		`strategy.order("Net", strategy.long, oca_name="group")`,
		`strategy.close()`,
		`strategy.close_all("now")`,
		`strategy.exit()`,
		`strategy.exit("BothTrail", "Long", trail_points=10, trail_price=close, trail_offset=1)`,
		`strategy.exit("NoOffset", "Long", trail_points=10)`,
		`strategy.exit("Mixed", "Long", trail_points=10, trail_offset=1, stop=low)`,
		`strategy.cancel_all("all")`,
		`strategy.cancel("A", "B")`,
	}
	for _, line := range cases {
		if stmt, handled, err := state.parseStrategyCall(parsedLine{number: 50, trimmed: line}); err == nil || !handled || stmt != nil {
			t.Fatalf("parseStrategyCall(%q) stmt=%#v handled=%v err=%v, want handled error", line, stmt, handled, err)
		}
	}
}

func TestParseStrategyCallRejectsInvalidTradingExpressions(t *testing.T) {
	cases := []string{
		`strategy.risk.allow_entry_in()`,
		`strategy.risk.max_drawdown()`,
		`strategy.risk.max_intraday_loss()`,
		`strategy.risk.max_intraday_filled_orders()`,
		`strategy.risk.max_position_size()`,
		`strategy.risk.max_cons_loss_days()`,
		`strategy.order("Net")`,
		`strategy.entry("Long", strategy.long, qty=close >)`,
		`strategy.entry("Long", strategy.long, limit=close >)`,
		`strategy.entry("Long", strategy.long, stop=close >)`,
		`strategy.entry("Long", strategy.long, when=close >)`,
		`strategy.order("Net", strategy.long, qty=close >)`,
		`strategy.order("Net", strategy.long, limit=close >)`,
		`strategy.order("Net", strategy.long, stop=close >)`,
		`strategy.order("Net", strategy.long, when=close >)`,
		`strategy.close("Long", qty=close >)`,
		`strategy.close("Long", limit=close >)`,
		`strategy.close("Long", stop=close >)`,
		`strategy.close("Long", when=close >)`,
		`strategy.exit("Exit", "Long", qty=close >, stop=low)`,
		`strategy.exit("Exit", "Long", stop=close >)`,
		`strategy.exit("Exit", "Long", limit=close >)`,
		`strategy.exit("Exit", "Long", profit=close >)`,
		`strategy.exit("Exit", "Long", loss=close >)`,
		`strategy.exit("Exit", "Long", stop=low, when=close >)`,
		`strategy.exit("Trail", "Long", trail_points=10, trail_offset=close >)`,
		`strategy.exit("Trail", "Long", trail_points=close >, trail_offset=2)`,
		`strategy.exit("Trail", "Long", trail_price=close >, trail_offset=2)`,
	}
	for _, line := range cases {
		t.Run(line, func(t *testing.T) {
			state := newStrategyCallBoundaryState()
			stmt, handled, err := state.parseStrategyCall(parsedLine{number: 60, trimmed: line})
			if err == nil || !handled || stmt != nil {
				t.Fatalf("parseStrategyCall(%q) stmt=%#v handled=%v err=%v, want handled validation error", line, stmt, handled, err)
			}
		})
	}
}

func parseStrategyBoundaryStatement[T strategyir.Statement](t *testing.T, state *parseState, line string) T {
	t.Helper()
	stmt, handled, err := state.parseStrategyCall(parsedLine{number: 20, trimmed: line})
	if err != nil || !handled {
		t.Fatalf("parseStrategyCall(%q) handled=%v err=%v", line, handled, err)
	}
	typed, ok := stmt.(T)
	if !ok {
		t.Fatalf("parseStrategyCall(%q) = %#v", line, stmt)
	}
	return typed
}

func newStrategyCallBoundaryState() *parseState {
	return &parseState{
		longEntryIDs:         map[string]bool{},
		shortEntryIDs:        map[string]bool{},
		udfs:                 map[string]pineUDF{},
		expressionAliases:    map[string]string{},
		sourceAliases:        map[string]string{},
		valueAliases:         map[string]string{},
		collectionNamespaces: map[string]string{},
		udtTypes:             map[string]strategyir.TypeDefinition{},
		udtMethods:           map[string][]strategyir.MethodDefinition{},
		objectTypes:          map[string]string{},
		objectPersistent:     map[string]bool{},
		loopVariables:        map[string]bool{},
		entryPolicyCache:     map[int]string{},
		regexpCache:          map[string]*regexp.Regexp{},
	}
}
