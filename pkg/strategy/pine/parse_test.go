package pine

import (
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

func TestParseScriptLowersPineStrategyToIR(t *testing.T) {
	script := `//@version=6
strategy("EMA Crossover", overlay=true)

fast = ta.ema(close, 8)
slow = ta.sma(close, 21)
if ta.crossover(fast, slow)
    strategy.entry("Long", strategy.long, qty=1)
else
    alert("waiting")`

	compilation, err := Compile(script)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	program := compilation.Program
	if program.SourceFormat != SourceFormatPineV6 {
		t.Fatalf("SourceFormat = %q", program.SourceFormat)
	}
	if program.Metadata.Name != "EMA Crossover" {
		t.Fatalf("Metadata.Name = %q", program.Metadata.Name)
	}
	if len(program.Hooks) != 1 || program.Hooks[0].Kind != strategyir.HookKLineClose {
		t.Fatalf("Hooks = %#v", program.Hooks)
	}
	if len(program.Hooks[0].Statements) != 3 {
		t.Fatalf("statement count = %d", len(program.Hooks[0].Statements))
	}
	ifStmt, ok := program.Hooks[0].Statements[2].(*strategyir.IfStmt)
	if !ok {
		t.Fatalf("statement 2 = %T", program.Hooks[0].Statements[2])
	}
	if ifStmt.Condition != "cross_over(fast, slow)" {
		t.Fatalf("condition = %q", ifStmt.Condition)
	}
	order, ok := ifStmt.Then[0].(*strategyir.OrderStmt)
	if !ok {
		t.Fatalf("then statement = %T", ifStmt.Then[0])
	}
	if order.Action != strategyir.OrderActionBuy || order.QuantityMode != "shares" || order.QuantityExpression != "1" {
		t.Fatalf("order = %#v", order)
	}
}

func TestCompileRejectsPublicInternalHelperCalls(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "ma", line: "fast = ma(EMA, 14)", want: "ma() is an internal JFTrade helper"},
		{name: "security source", line: "daily = security_source(close, day)", want: "security_source() is an internal JFTrade helper"},
		{name: "bollinger", line: "band = bollinger(20, 2)", want: "bollinger() is an internal JFTrade helper"},
		{name: "barssince", line: "bars = barssince(close > open)", want: "barssince() is an internal JFTrade helper"},
		{name: "valuewhen", line: "last = valuewhen(close > open, close, 0)", want: "valuewhen() is an internal JFTrade helper"},
		{name: "history", line: "prev = history(close, 1)", want: "history() is an internal JFTrade helper"},
		{name: "ifelse", line: "picked = ifelse(close > open, close, open)", want: "ifelse() is an internal JFTrade helper"},
		{name: "cross over", line: "x = cross_over(fast, slow)", want: "cross_over() is an internal JFTrade helper"},
		{name: "cross under", line: "x = cross_under(fast, slow)", want: "cross_under() is an internal JFTrade helper"},
		{name: "notify", line: "notify(\"hello\")", want: "notify() is an internal JFTrade helper"},
		{name: "ta adx shortcut", line: "adx = ta.adx(14)", want: "ta.adx() is a JFTrade-only shortcut"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compile(`//@version=6
strategy("reject helpers", overlay=true)
` + tt.line)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Compile() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestAnalyzeScriptReportsPublicInternalHelperDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		line string
		code string
		want string
	}{
		{name: "internal helper", line: "fast = ma(EMA, 14)", code: "PINE_INTERNAL_HELPER_PUBLIC", want: "use Pine v6 ta.sma/ta.ema"},
		{name: "ta shortcut", line: "adx = ta.adx(14)", code: "PINE_PUBLIC_TA_SHORTCUT", want: "use Pine v6 ta.dmi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analysis := AnalyzeScript(`//@version=6
strategy("helper diagnostics", overlay=true)
`+tt.line, AnalysisOptions{IncludeAST: true})
			if analysis.OK {
				t.Fatal("AnalyzeScript().OK = true, want false")
			}
			if len(analysis.Diagnostics) == 0 {
				t.Fatal("AnalyzeScript() diagnostics empty")
			}
			diagnostic := analysis.Diagnostics[0]
			if diagnostic.Code != tt.code || diagnostic.Line != 3 || !strings.Contains(diagnostic.Message, tt.want) {
				t.Fatalf("diagnostic = %#v, want code %s and message containing %q", diagnostic, tt.code, tt.want)
			}
		})
	}
}

func TestCompileAcceptsNativePineIndicatorPublicEntry(t *testing.T) {
	_, err := Compile(`//@version=6
strategy("native indicators", overlay=true)
fast = ta.ema(close, 14)
band = ta.bb(close, 20, 2)
daily = request.security(syminfo.tickerid, "D", ta.sma(close, 20))
if close > fast and close < band.upper and daily > 0
    strategy.entry("Long", strategy.long, qty=1)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
}

func TestCompileUsesStrategyDefaultQuantityForEntryWithoutQty(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Default Qty", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10, pyramiding=2)
strategy.entry("Long", strategy.long)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if compilation.Program.Metadata.DefaultQtyMode != "percent_of_equity" || compilation.Program.Metadata.DefaultQtyValue != "10" || compilation.Program.Metadata.Pyramiding != 2 {
		t.Fatalf("metadata = %#v", compilation.Program.Metadata)
	}
	order, ok := compilation.Program.Hooks[0].Statements[0].(*strategyir.OrderStmt)
	if !ok {
		t.Fatalf("statement = %T", compilation.Program.Hooks[0].Statements[0])
	}
	if order.QuantityMode != "account_position_percent" || order.QuantityExpression != "10" {
		t.Fatalf("order = %#v", order)
	}
}

func TestCompileParsesBacktestStrategyMetadata(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Costs", initial_capital=250000, commission_type=strategy.commission.percent, commission_value=0.15, slippage=3, process_orders_on_close=true)
strategy.entry("Long", strategy.long, qty=1)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	metadata := compilation.Program.Metadata
	if metadata.InitialCapital != 250000 {
		t.Fatalf("InitialCapital = %v, want 250000", metadata.InitialCapital)
	}
	if metadata.CommissionType != "percent" || metadata.CommissionValue != 0.15 {
		t.Fatalf("commission metadata = %q/%v", metadata.CommissionType, metadata.CommissionValue)
	}
	if metadata.Slippage != 3 {
		t.Fatalf("Slippage = %d, want 3", metadata.Slippage)
	}
	if !metadata.ProcessOnClose {
		t.Fatal("ProcessOnClose = false, want true")
	}
}

func TestCompilePreservesOrderNotificationMetadataAndImmediateClose(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Order Metadata")
strategy.entry("Long", strategy.long, qty=1, comment="entry", alert_message="opened", disable_alert=false)
strategy.close("Long", immediately=true, comment="close", alert_message="closed", disable_alert=true)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	statements := compilation.Program.Hooks[0].Statements
	entry := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](statements[0])
	if entry.Comment != "entry" || entry.AlertMessage != "opened" || entry.DisableAlert {
		t.Fatalf("entry metadata = %#v", entry)
	}
	closeOrder := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](statements[1])
	if !closeOrder.Immediate || closeOrder.Comment != "close" || closeOrder.AlertMessage != "closed" || !closeOrder.DisableAlert {
		t.Fatalf("close metadata = %#v", closeOrder)
	}
}

func TestCompileExplicitEntryQtyOverridesStrategyDefaultQuantity(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Explicit Qty", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)
strategy.entry("Long", strategy.long, qty=5)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	order, ok := compilation.Program.Hooks[0].Statements[0].(*strategyir.OrderStmt)
	if !ok {
		t.Fatalf("statement = %T", compilation.Program.Hooks[0].Statements[0])
	}
	if order.QuantityMode != "shares" || order.QuantityExpression != "5" {
		t.Fatalf("order = %#v", order)
	}
}

func TestValidateScriptRejectsUnsupportedPineRuntimeFeature(t *testing.T) {
	err := ValidateScript(`//@version=6
strategy("MTF", overlay=true)
x = request.security("NASDAQ:AAPL", "D", close)`)
	if err == nil || !strings.Contains(err.Error(), "request.security") {
		t.Fatalf("ValidateScript() error = %v, want request.security diagnostic", err)
	}

	err = ValidateScript(`//@version=6
strategy("MTF", overlay=true)
x = request.security(syminfo.tickerid, "D", alert("no side effects"))`)
	if err == nil || !strings.Contains(err.Error(), "request.security") {
		t.Fatalf("ValidateScript() side-effect expression error = %v, want request.security diagnostic", err)
	}

	err = ValidateScript(`//@version=6
strategy("MTF", overlay=true)
x = request.security(syminfo.tickerid, "D", close, lookahead=barmerge.lookahead_on)`)
	if err == nil || !strings.Contains(err.Error(), "lookahead_on") {
		t.Fatalf("ValidateScript() lookahead error = %v, want lookahead diagnostic", err)
	}

	err = ValidateScript(`//@version=6
strategy("MTF", overlay=true)
x = request.security(syminfo.tickerid, "D", close, gaps=barmerge.gaps_on)`)
	if err == nil || !strings.Contains(err.Error(), "gaps_on") {
		t.Fatalf("ValidateScript() gaps error = %v, want gaps diagnostic", err)
	}
}

func TestCompileSupportsMultiBarHistoryReferences(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("History", overlay=true)
[basis, upper, lower] = ta.bb(close, 20, 2)
emaFast = ta.ema(close, 3)
if close > close[2] and hlc3 > hlc3[3] and emaFast > emaFast[5] and close > upper[2]
    strategy.entry("Long", strategy.long, qty=1)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	ifStmt, ok := compilation.Program.Hooks[0].Statements[2].(*strategyir.IfStmt)
	if !ok {
		t.Fatalf("statement = %T", compilation.Program.Hooks[0].Statements[2])
	}
	for _, expected := range []string{"history(close, 2)", "history(hlc3, 3)", "history(emaFast, 5)", "history(basis.upper, 2)"} {
		if !strings.Contains(ifStmt.Condition, expected) {
			t.Fatalf("condition = %q, want %q", ifStmt.Condition, expected)
		}
	}
}

func TestValidateScriptReportsUnsupportedHistoryReferences(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		message string
	}{
		{
			name: "function result history",
			script: `//@version=6
strategy("History", overlay=true)
if ta.sma(close, 20)[2] > close
    strategy.entry("Long", strategy.long, qty=1)`,
			message: "assign the function result first",
		},
		{
			name: "lookback limit",
			script: `//@version=6
strategy("History", overlay=true)
if close[501] > close
    strategy.entry("Long", strategy.long, qty=1)`,
			message: "exceeds JFTrade maximum 500",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateScript(tc.script)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("ValidateScript() error = %v, want %q", err, tc.message)
			}
		})
	}
}

func TestCompileSupportsStrategyExitSubset(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Exit", overlay=true)
strategy.exit("Long stop", "Long", stop=close * (1 - 2 / 100), qty_percent=50)
strategy.exit("Short profit", "Short", limit=close * (1 - 3 / 100), qty=5)
strategy.exit("Bracket", from_entry="Long", stop=close - 2, limit=close + 3)
strategy.exit("Long trail", "Long", trail_points=close * 4 / 100, trail_offset=close * 4 / 100)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compilation.Program.Hooks[0].Statements) != 4 {
		t.Fatalf("statement count = %d", len(compilation.Program.Hooks[0].Statements))
	}
	stop, ok := compilation.Program.Hooks[0].Statements[0].(*strategyir.ExitStmt)
	if !ok || stop.Direction != "long" || stop.StopExpression != "close * (1 - 2 / 100)" || stop.LimitExpression != "" || stop.QuantityMode != "symbol_position_percent" || stop.QuantityExpression != "50" {
		t.Fatalf("stop statement = %#v", compilation.Program.Hooks[0].Statements[0])
	}
	profit, ok := compilation.Program.Hooks[0].Statements[1].(*strategyir.ExitStmt)
	if !ok || profit.Direction != "short" || profit.StopExpression != "" || profit.LimitExpression != "close * (1 - 3 / 100)" || profit.QuantityMode != "shares" || profit.QuantityExpression != "5" {
		t.Fatalf("profit statement = %#v", compilation.Program.Hooks[0].Statements[1])
	}
	bracket, ok := compilation.Program.Hooks[0].Statements[2].(*strategyir.ExitStmt)
	if !ok || bracket.FromEntry != "Long" || bracket.StopExpression != "close - 2" || bracket.LimitExpression != "close + 3" {
		t.Fatalf("bracket statement = %#v", compilation.Program.Hooks[0].Statements[2])
	}
	trailing, ok := compilation.Program.Hooks[0].Statements[3].(*strategyir.ExitStmt)
	if !ok || trailing.ID != "Long trail" || trailing.Direction != "long" || trailing.TrailPoints != "close * 4 / 100" || trailing.TrailOffset != "close * 4 / 100" || trailing.QuantityMode != "symbol_position_percent" || trailing.QuantityExpression != "100" {
		t.Fatalf("trailing statement = %#v", compilation.Program.Hooks[0].Statements[3])
	}
}

func TestCompileCapturesWhenExpressionsForWorkflowOrders(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("When", overlay=true)
strategy.entry("Long", strategy.long, qty=1, when=close > open)
strategy.order("Net short", strategy.short, qty=2, when=close < open)
strategy.close("Long", when=ta.crossunder(close, open))
strategy.exit("Exit", "Long", stop=close - 2, when=high > low)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compilation.Program.Hooks[0].Statements) != 4 {
		t.Fatalf("statement count = %d", len(compilation.Program.Hooks[0].Statements))
	}
	entry := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](compilation.Program.Hooks[0].Statements[0])
	if entry.WhenExpression != "close > open" {
		t.Fatalf("entry when = %q, want close > open", entry.WhenExpression)
	}
	order := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](compilation.Program.Hooks[0].Statements[1])
	if order.WhenExpression != "close < open" {
		t.Fatalf("order when = %q, want close < open", order.WhenExpression)
	}
	closeStmt := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](compilation.Program.Hooks[0].Statements[2])
	if closeStmt.WhenExpression != "cross_under(close, open)" {
		t.Fatalf("close when = %q, want cross_under(close, open)", closeStmt.WhenExpression)
	}
	exit := jftradeCheckedTypeAssertion[*strategyir.ExitStmt](compilation.Program.Hooks[0].Statements[3])
	if exit.WhenExpression != "high > low" {
		t.Fatalf("exit when = %q, want high > low", exit.WhenExpression)
	}
}

func TestCompileSupportsStrategyExitProfitLossTicks(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Exit points", overlay=true)
strategy.exit("Points", "Long", profit=50, loss=25, qty_percent=50)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compilation.Program.Hooks[0].Statements) != 1 {
		t.Fatalf("statement count = %d", len(compilation.Program.Hooks[0].Statements))
	}
	exit := jftradeCheckedTypeAssertion[*strategyir.ExitStmt](compilation.Program.Hooks[0].Statements[0])
	if exit.ProfitExpression != "50" || exit.LossExpression != "25" {
		t.Fatalf("exit profit/loss = %#v", exit)
	}
	if exit.QuantityMode != "symbol_position_percent" || exit.QuantityExpression != "50" {
		t.Fatalf("exit quantity = %#v", exit)
	}
}

func TestCompileCapturesStrategyExitSpecificMetadata(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Exit metadata", overlay=true)
strategy.exit("Bracket", "Long", stop=98, limit=105, comment="generic", comment_profit="tp", comment_loss="sl", alert_message="base", alert_profit="ap", alert_loss="al")
strategy.exit("Trail", "Long", trail_points=10, trail_offset=5, comment="generic trail", comment_trailing="trail comment", alert_message="trail base", alert_trailing="trail alert")`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compilation.Program.Hooks[0].Statements) != 2 {
		t.Fatalf("statement count = %d", len(compilation.Program.Hooks[0].Statements))
	}
	bracket := jftradeCheckedTypeAssertion[*strategyir.ExitStmt](compilation.Program.Hooks[0].Statements[0])
	if bracket.Comment != "generic" || bracket.CommentProfit != "tp" || bracket.CommentLoss != "sl" ||
		bracket.AlertMessage != "base" || bracket.AlertProfit != "ap" || bracket.AlertLoss != "al" {
		t.Fatalf("bracket metadata = %#v", bracket)
	}
	trailing := jftradeCheckedTypeAssertion[*strategyir.ExitStmt](compilation.Program.Hooks[0].Statements[1])
	if trailing.Comment != "generic trail" || trailing.CommentTrailing != "trail comment" ||
		trailing.AlertMessage != "trail base" || trailing.AlertTrailing != "trail alert" {
		t.Fatalf("trailing metadata = %#v", trailing)
	}
}

func TestCompileSupportsPendingStopAndCancelOrders(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Pending", overlay=true)
strategy.entry("Breakout", strategy.long, stop=ta.highest(high, 20), qty=1)
strategy.order("Net short", strategy.short, stop=low - 1, qty=5)
strategy.close("Long", stop=99, limit=101, qty_percent=50)
strategy.entry("StopLimit", strategy.long, stop=101, limit=99, qty=2)
strategy.cancel("Breakout")
strategy.cancel_all()`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	statements := compilation.Program.Hooks[0].Statements
	if len(statements) != 6 {
		t.Fatalf("statement count = %d", len(statements))
	}
	entry, ok := statements[0].(*strategyir.OrderStmt)
	if !ok || entry.Intent != strategyir.OrderIntentEntry || entry.ID != "Breakout" || entry.StopExpression != "highest(high, 20)" || entry.LimitExpression != "" {
		t.Fatalf("entry statement = %#v", statements[0])
	}
	net, ok := statements[1].(*strategyir.OrderStmt)
	if !ok || net.Intent != strategyir.OrderIntentNet || net.ID != "Net short" || net.Action != strategyir.OrderActionSell || net.StopExpression != "low - 1" {
		t.Fatalf("net statement = %#v", statements[1])
	}
	closeStopLimit, ok := statements[2].(*strategyir.OrderStmt)
	if !ok || closeStopLimit.Intent != strategyir.OrderIntentClose || closeStopLimit.ID != "Long" || closeStopLimit.StopExpression != "99" || closeStopLimit.LimitExpression != "101" || closeStopLimit.QuantityMode != "symbol_position_percent" || closeStopLimit.QuantityExpression != "50" {
		t.Fatalf("close stop-limit statement = %#v", statements[2])
	}
	stopLimit, ok := statements[3].(*strategyir.OrderStmt)
	if !ok || stopLimit.StopExpression != "101" || stopLimit.LimitExpression != "99" || stopLimit.OrderType != "LIMIT" {
		t.Fatalf("stop-limit statement = %#v", statements[3])
	}
	cancel, ok := statements[4].(*strategyir.CancelStmt)
	if !ok || cancel.ID != "Breakout" || cancel.All {
		t.Fatalf("cancel statement = %#v", statements[4])
	}
	cancelAll, ok := statements[5].(*strategyir.CancelStmt)
	if !ok || !cancelAll.All {
		t.Fatalf("cancel_all statement = %#v", statements[5])
	}
}

func TestCompileSupportsCloseAllPositionalMetadata(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Close all positional", overlay=true)
strategy.close_all(true, "flat", "done", false)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compilation.Program.Hooks[0].Statements) != 1 {
		t.Fatalf("statement count = %d", len(compilation.Program.Hooks[0].Statements))
	}
	order := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](compilation.Program.Hooks[0].Statements[0])
	if order.Intent != strategyir.OrderIntentFlatten || !order.Immediate || order.Comment != "flat" || order.AlertMessage != "done" || order.DisableAlert {
		t.Fatalf("close_all positional metadata = %#v", order)
	}
}

func TestCompileSupportsClosePositionalQty(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Close positional", overlay=true)
strategy.close("Long", 2)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compilation.Program.Hooks[0].Statements) != 1 {
		t.Fatalf("statement count = %d", len(compilation.Program.Hooks[0].Statements))
	}
	order := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](compilation.Program.Hooks[0].Statements[0])
	if order.Intent != strategyir.OrderIntentClose || order.QuantityMode != "shares" || order.QuantityExpression != "2" {
		t.Fatalf("close positional qty = %#v", order)
	}
}

func TestValidateScriptReportsUnsupportedAdvancedOrders(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		message string
	}{
		{
			name: "trail with stop",
			script: `//@version=6
strategy("Trail Stop", overlay=true)
strategy.exit("Exit", "Long", stop=close - 2, trail_points=close * 4 / 100, trail_offset=close * 4 / 100)`,
			message: "trail with stop/limit",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateScript(tc.script)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("ValidateScript() error = %v, want %q", err, tc.message)
			}
		})
	}
}

func TestAnalyzeScriptReportsV40BrokerBoundaryDiagnostics(t *testing.T) {
	cases := []struct {
		name   string
		script string
		code   string
		line   int
	}{
		{
			name: "oca argument",
			script: `//@version=6
strategy("OCA", overlay=true)
strategy.entry("Long", strategy.long, qty=1, oca_name="group")`,
			code: "PINE_ORDER_OCA_UNSUPPORTED",
			line: 3,
		},
		{
			name: "exit oca argument",
			script: `//@version=6
strategy("Exit OCA", overlay=true)
strategy.exit("Exit", "Long", stop=98, oca_name="group")`,
			code: "PINE_ORDER_OCA_UNSUPPORTED",
			line: 3,
		},
		{
			name: "conflicting qty",
			script: `//@version=6
strategy("Qty conflict", overlay=true)
strategy.close("Long", qty=1, qty_percent=50)`,
			code: "PINE_ORDER_QTY_CONFLICT",
			line: 3,
		},
		{
			name: "unsupported close all arg",
			script: `//@version=6
strategy("Close all unsupported", overlay=true)
strategy.close_all(foo=1)`,
			code: "PINE_COMPILE_ERROR",
			line: 3,
		},
		{
			name: "trail bracket mix",
			script: `//@version=6
strategy("Trail Stop", overlay=true)
strategy.exit("Exit", "Long", stop=close - 2, trail_points=close * 4 / 100, trail_offset=close * 4 / 100)`,
			code: "PINE_ORDER_EXIT_TRAIL_BRACKET_UNSUPPORTED",
			line: 3,
		},
		{
			name: "advanced exit",
			script: `//@version=6
strategy("Advanced Exit", overlay=true)
strategy.exit("Exit", "Long")`,
			code: "PINE_ORDER_EXIT_ADVANCED_UNSUPPORTED",
			line: 3,
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			analysis := AnalyzeScript(item.script, AnalysisOptions{IncludeAST: true})
			if analysis.OK {
				t.Fatalf("AnalyzeScript().OK = true, want false")
			}
			for _, diagnostic := range analysis.Diagnostics {
				if diagnostic.Code == item.code && diagnostic.Line == item.line {
					return
				}
			}
			t.Fatalf("diagnostics = %#v, missing %s on line %d", analysis.Diagnostics, item.code, item.line)
		})
	}
}

func TestCompileSupportsFrameworkLanguageFeatures(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Framework", overlay=true)
var count = 0
count := count + 1
signal = close[1] == na ? 0 : nz(close[1], close)
if close > close[1]
    log.info("up")`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	statements := compilation.Program.Hooks[0].Statements
	if got := jftradeCheckedTypeAssertion[*strategyir.LetStmt](statements[0]).Mode; got != strategyir.AssignmentModeVar {
		t.Fatalf("first assignment mode = %q", got)
	}
	if got := jftradeCheckedTypeAssertion[*strategyir.LetStmt](statements[1]).Mode; got != strategyir.AssignmentModeReassign {
		t.Fatalf("second assignment mode = %q", got)
	}
	if got := jftradeCheckedTypeAssertion[*strategyir.LetStmt](statements[2]).Expression; got != "ifelse(history(close, 1) == na, 0, nz(history(close, 1), close))" {
		t.Fatalf("signal expression = %q", got)
	}
	ifStmt := jftradeCheckedTypeAssertion[*strategyir.IfStmt](statements[3])
	if ifStmt.Condition != "close > history(close, 1)" {
		t.Fatalf("condition = %q", ifStmt.Condition)
	}
}

func TestCompileSupportsV12AdvancedIndicators(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Advanced indicators", overlay=true)
lr = ta.linreg(close, 5, 0)
obvValue = ta.obv
pivotHigh = ta.pivothigh(high, 2, 2)
pivotLow = ta.pivotlow(low, 2, 2)
[basis, upper, lower] = ta.kc(close, 5, 1.5)
width = ta.kcw(close, 5, 1.5)
almaValue = ta.alma(close, 5, 0.85, 6)
if close > lr and obvValue > 0 and upper > lower and width > 0 and almaValue > 0
    strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript() diagnostics = %#v", analysis.Diagnostics)
	}
	keys := map[string]bool{}
	for _, requirement := range analysis.Requirements.Indicators {
		keys[requirement.Key] = true
	}
	for _, key := range []string{
		"linreg:close:5:0",
		"obv:close",
		"pivothigh:high:2:2",
		"pivotlow:low:2:2",
		"kc:close:5:1.5:true",
		"kcw:close:5:1.5:true",
		"alma:close:5:0.85:6",
	} {
		if !keys[key] {
			t.Fatalf("requirements missing %q: %#v", key, analysis.Requirements.Indicators)
		}
	}
}

func TestCompileSupportsV12AdvancedIndicatorsInStaticIntradaySecurity(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Advanced MTF", overlay=true)
lr = request.security(syminfo.tickerid, "15", ta.linreg(close, 5, 0))
obvValue = request.security(syminfo.tickerid, "15", ta.obv)
pivotHigh = request.security(syminfo.tickerid, "15", ta.pivothigh(2, 2))
[basis, upper, lower] = request.security(syminfo.tickerid, "15", ta.kc(close, 5, 1.5))
almaValue = request.security(syminfo.tickerid, "15", ta.alma(close, 5, 0.85, 6))
if close > lr and obvValue > 0 and pivotHigh > 0 and upper > lower and almaValue > 0
    strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript() diagnostics = %#v", analysis.Diagnostics)
	}
	keys := map[string]bool{}
	for _, requirement := range analysis.Requirements.Indicators {
		keys[requirement.Key] = true
	}
	for _, key := range []string{
		"linreg:close:5:0:15m",
		"obv:close:15m",
		"pivothigh:high:2:2:15m",
		"kc:close:5:1.5:true:15m",
		"alma:close:5:0.85:6:15m",
	} {
		if !keys[key] {
			t.Fatalf("requirements missing %q: %#v", key, analysis.Requirements.Indicators)
		}
	}
}

func TestCompileSupportsV13MigrationIndicators(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v1.3 indicators", overlay=true)
cmoValue = ta.cmo(close, 5)
tsiValue = ta.tsi(close, 2, 3)
corrValue = ta.correlation(close, high, 5)
devValue = ta.dev(close, 5)
medianValue = ta.median(close, 5)
pLinear = ta.percentile_linear_interpolation(close, 5, 50)
pNearest = ta.percentile_nearest_rank(close, 5, 80)
rankValue = ta.percentrank(close, 5)
swmaValue = ta.swma(close)
rounded = math.round_to_mintick(math.avg(close, open))
if cmoValue > 0 and tsiValue > 0 and corrValue > 0 and devValue > 0 and medianValue > 0 and pLinear > 0 and pNearest > 0 and rankValue > 0 and swmaValue > 0 and rounded > 0
    strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript() diagnostics = %#v", analysis.Diagnostics)
	}
	keys := map[string]bool{}
	for _, requirement := range analysis.Requirements.Indicators {
		keys[requirement.Key] = true
	}
	for _, key := range []string{
		"cmo:close:5",
		"tsi:close:2:3",
		"correlation:close:high:5",
		"dev:close:5",
		"median:close:5",
		"percentile_linear_interpolation:close:5:50",
		"percentile_nearest_rank:close:5:80",
		"percentrank:close:5",
		"swma:close",
	} {
		if !keys[key] {
			t.Fatalf("requirements missing %q: %#v", key, analysis.Requirements.Indicators)
		}
	}
}

func TestCompileSupportsV13IndicatorsInStaticIntradaySecurity(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v1.3 MTF indicators", overlay=true)
cmoValue = request.security(syminfo.tickerid, "15", ta.cmo(close, 5))
corrValue = request.security(syminfo.tickerid, "15", ta.correlation(close, high, 5))
pctValue = request.security(syminfo.tickerid, "15", ta.percentile_nearest_rank(close, 5, 80))
swmaValue = request.security(syminfo.tickerid, "15", ta.swma(close))
if cmoValue > 0 and corrValue > 0 and pctValue > 0 and swmaValue > 0
    strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript() diagnostics = %#v", analysis.Diagnostics)
	}
	keys := map[string]bool{}
	for _, requirement := range analysis.Requirements.Indicators {
		keys[requirement.Key] = true
	}
	for _, key := range []string{
		"cmo:close:5:15m",
		"correlation:close:high:5:15m",
		"percentile_nearest_rank:close:5:80:15m",
		"swma:close:15m",
	} {
		if !keys[key] {
			t.Fatalf("requirements missing %q: %#v", key, analysis.Requirements.Indicators)
		}
	}
}

func TestCompileSupportsAllowEntryInRiskDeclaration(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Allow entry", overlay=true)
strategy.risk.allow_entry_in(strategy.direction.long)
if close > open
    strategy.entry("Long", strategy.long, qty=1)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if got := compilation.Program.Metadata.AllowedEntryDirection; got != "long" {
		t.Fatalf("AllowedEntryDirection = %q, want long", got)
	}
}

func TestCompileSupportsRuntimeRiskDeclarations(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Risk declarations", overlay=true)
strategy.risk.max_drawdown(10, strategy.percent_of_equity, alert_message="dd")
strategy.risk.max_intraday_loss(5, strategy.cash, "day")
strategy.risk.max_intraday_filled_orders(3, alert_message="fills")
strategy.risk.max_position_size(12)
strategy.risk.max_cons_loss_days(2, "days")
if close > open
    strategy.entry("Long", strategy.long, qty=1)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	metadata := compilation.Program.Metadata
	if metadata.MaxDrawdownValue != 10 || metadata.MaxDrawdownType != "percent_of_equity" || metadata.MaxDrawdownAlert != "dd" {
		t.Fatalf("max_drawdown metadata = %#v", metadata)
	}
	if metadata.MaxIntradayLossValue != 5 || metadata.MaxIntradayLossType != "cash" || metadata.MaxIntradayLossAlert != "day" {
		t.Fatalf("max_intraday_loss metadata = %#v", metadata)
	}
	if metadata.MaxIntradayFilledOrders != 3 || metadata.MaxIntradayFilledOrdersAlert != "fills" {
		t.Fatalf("max_intraday_filled_orders metadata = %#v", metadata)
	}
	if metadata.MaxPositionSize != 12 {
		t.Fatalf("MaxPositionSize = %v, want 12", metadata.MaxPositionSize)
	}
	if metadata.MaxConsLossDays != 2 || metadata.MaxConsLossDaysAlert != "days" {
		t.Fatalf("max_cons_loss_days metadata = %#v", metadata)
	}
}

func TestCompatibilityScoreAndSupportedFeatureIDsAreRegistryDriven(t *testing.T) {
	assessment := CompatibilityScore()
	if assessment.ScoreModelVersion != "closed-bar-strategy-v4.0" {
		t.Fatalf("ScoreModelVersion = %q", assessment.ScoreModelVersion)
	}
	if assessment.Score < 98 || assessment.Score > 100 {
		t.Fatalf("Score = %v, want v4.0 closed-bar score", assessment.Score)
	}
	if len(assessment.Dimensions) != 5 {
		t.Fatalf("Dimensions = %#v, want 5 dimensions", assessment.Dimensions)
	}
	features := map[string]bool{}
	for _, id := range SupportedFeatureIDs() {
		features[id] = true
	}
	for _, id := range []string{"indicator.v13_migration_set", "indicator.v14_window_momentum_set", "indicator.v15_common_ta_set", "indicator.v16_mtf_tuple_bindings", "indicator.v17_source_aware_semantic_requirements", "indicator.v21_bbw_cog_anchored_vwap", "indicator.v24_mtf_stoch", "request.security.pure_expression", "request.security.v15_common_ta_expression", "request.security.v16_tuple_whitelist", "request.security.v17_semantic_tuple_corpus", "request.security.v21_ast_pure_expression", "request.security.v22_general_tuple", "request.security.v23_pure_collection_object_expression", "request.security.v24_mtf_stoch", "request.security.v27_pure_helper_expression", "request.security.v28_object_method_expression", "request.security.v29_object_history_expression", "request.security.v32_diagnostic_matrix", "request.security.v32_lower_timeframe_preflight", "syntax.v15_loop_control_subset", "syntax.v16_security_tuple_destructure", "syntax.v17_ast_semantic_transition", "syntax.v21_collection_runtime_core", "syntax.v22_structured_loop_runtime", "syntax.v22_pure_udt_method_runtime", "syntax.v23_collection_api_expansion", "syntax.v23_pure_method_body_named_args", "syntax.v24_collection_api_expansion", "syntax.v24_runtime_loop_fallback", "syntax.v24_persistent_object_field_set", "syntax.v25_array_stat_api", "syntax.v26_collection_iteration", "syntax.v26_collection_history_snapshot", "syntax.v26_object_collection_fields", "syntax.v26_library_export_metadata", "syntax.v27_collection_history_aggregates", "syntax.v27_map_matrix_iteration", "syntax.v28_object_history_read", "syntax.v28_method_chain", "syntax.v28_export_metadata", "syntax.v29_object_history_method_receiver", "syntax.v29_method_chain_named_defaults", "syntax.v29_request_security_diagnostics", "syntax.v30_stable_semantic_declarations", "syntax.v30_varip_closed_bar_policy", "syntax.v30_parser_whitespace_comments", "syntax.v31_public_surface_lock", "syntax.v33_advanced_language_boundary", "syntax.arrays_maps_matrices", "syntax.methods_types_libraries", "syntax.dynamic_loops_while", "expression.v22_general_tuple", "expression.v23_object_field_set", "expression.v25_string_helpers", "expression.v25_timeframe_change", "expression.v27_timeframe_helpers", "tooling.visual_metadata_output", "tooling.v20_language_foundation", "tooling.v31_structured_helper_diagnostics", "tooling.v33_structured_language_diagnostics", "tooling.v34_generated_support_snapshot", "tooling.v40_broker_boundary_snapshot", "tooling.migration_corpus_v21", "tooling.migration_corpus_v22", "tooling.migration_corpus_v23", "tooling.migration_corpus_v24", "tooling.migration_corpus_v25", "tooling.migration_corpus_v26", "tooling.migration_corpus_v27", "tooling.migration_corpus_v28", "tooling.migration_corpus_v29", "tooling.migration_corpus_v30", "order.entry_reversal", "order.allow_entry_in", "strategy.v40_broker_boundary_decision"} {
		if !features[id] {
			t.Fatalf("SupportedFeatureIDs missing %q", id)
		}
	}
	for _, id := range []string{"order.oca_partial_fill", "request.security.dynamic_symbol_timeframe"} {
		if features[id] {
			t.Fatalf("SupportedFeatureIDs includes unsupported feature %q", id)
		}
	}
}

func TestCompileSupportsExpressionUDFAndStaticForUnroll(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("UDF For", overlay=true)
isBull(src) => src > src[1]
smooth(src, len) => ta.ema(src, len)
len = input.int(3, "Length")
fast = smooth(close, len)
sum = 0
for i = 0 to 3
    sum := sum + close[i]
if isBull(close) and fast > fast[1] and sum > 0
    strategy.entry("Long", strategy.long, qty=1)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	statements := compilation.Program.Hooks[0].Statements
	expected := []string{
		"3",
		"ma(EMA, 3)",
		"0",
		"sum + history(close, 0)",
		"sum + history(close, 1)",
		"sum + history(close, 2)",
		"sum + history(close, 3)",
	}
	for index, want := range expected {
		stmt, ok := statements[index].(*strategyir.LetStmt)
		if !ok || stmt.Expression != want {
			t.Fatalf("statement %d = %#v, want expression %q", index, statements[index], want)
		}
	}
	ifStmt, ok := statements[7].(*strategyir.IfStmt)
	if !ok {
		t.Fatalf("statement 7 = %T", statements[7])
	}
	if want := "(close > history(close, 1)) && fast > history(fast, 1) && sum > 0"; ifStmt.Condition != want {
		t.Fatalf("condition = %q, want %q", ifStmt.Condition, want)
	}

	requirements, err := strategyir.PlanRequirements(compilation.Program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}
	if len(requirements.Indicators) != 1 || requirements.Indicators[0].Key != "ma:EMA:3" {
		t.Fatalf("requirements = %#v", requirements.Indicators)
	}
}

func TestValidateScriptReportsUnsupportedUDFAndStaticForCases(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		message string
	}{
		{
			name: "argument mismatch",
			script: `//@version=6
strategy("UDF", overlay=true)
f(x) => x
y = f(close, open)`,
			message: `expects 1 arguments, got 2`,
		},
		{
			name: "recursive udf",
			script: `//@version=6
strategy("UDF", overlay=true)
f(x) => f(x)
y = f(close)`,
			message: `recursive user-defined function`,
		},
		{
			name: "zero step",
			script: `//@version=6
strategy("Loop", overlay=true)
for i = 0 to 3 by 0
    log.info("nope")`,
			message: `for loop step cannot be 0`,
		},
		{
			name: "too many iterations",
			script: `//@version=6
strategy("Loop", overlay=true)
for i = 0 to 100
    log.info("nope")`,
			message: `for loop expands to more than 100 iterations`,
		},
		{
			name: "loop var readonly",
			script: `//@version=6
strategy("Loop", overlay=true)
for i = 0 to 3
    i := 1`,
			message: `loop variable "i" is read-only`,
		},
		{
			name: "call history in unrolled loop",
			script: `//@version=6
strategy("Loop", overlay=true)
for i = 0 to 3
    x = ta.sma(close, 20)[i]`,
			message: `assign the function result first`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateScript(tc.script)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("ValidateScript() error = %v, want %q", err, tc.message)
			}
		})
	}
}

func TestAnalyzeScriptReportsV33AdvancedLanguageBoundaryDiagnostics(t *testing.T) {
	cases := []struct {
		name   string
		script string
		code   string
		line   int
	}{
		{
			name: "recursive udf",
			script: `//@version=6
strategy("UDF", overlay=true)
f(x) => f(x)
y = f(close)`,
			code: "PINE_UDF_RECURSIVE_UNSUPPORTED",
			line: 4,
		},
		{
			name: "nested udf",
			script: `//@version=6
strategy("UDF", overlay=true)
outer(x) =>
    inner(y) => y
y = outer(close)`,
			code: "PINE_UDF_NESTED_UNSUPPORTED",
			line: 3,
		},
		{
			name: "udf signature",
			script: `//@version=6
strategy("UDF", overlay=true)
f(x) => x
y = f(close, open)`,
			code: "PINE_UDF_SIGNATURE_UNSUPPORTED",
			line: 4,
		},
		{
			name: "loop limit",
			script: `//@version=6
strategy("Loop", overlay=true)
for i = 0 to 100
    log.info("nope")`,
			code: "PINE_LOOP_LIMIT_UNSUPPORTED",
			line: 3,
		},
		{
			name: "loop readonly",
			script: `//@version=6
strategy("Loop", overlay=true)
for i = 0 to 3
    i := 1`,
			code: "PINE_LOOP_VARIABLE_READONLY",
			line: 4,
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			analysis := AnalyzeScript(item.script, AnalysisOptions{IncludeAST: true})
			if analysis.OK {
				t.Fatalf("AnalyzeScript().OK = true, want false")
			}
			found := false
			for _, diagnostic := range analysis.Diagnostics {
				if diagnostic.Code == item.code && diagnostic.Line == item.line {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("diagnostics = %#v, missing %s on line %d", analysis.Diagnostics, item.code, item.line)
			}
		})
	}
}

func TestCompileSupportsSwitchAndMultiStatementUDF(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Switch UDF", overlay=true)
classify(x) =>
    doubled = x * 2
    if doubled > 0
        doubled
    else
        -doubled
score = classify(close)
signal = switch
    close > open => 1
    => 0
switch signal
    1 => strategy.entry("Long", strategy.long, qty=1)
    => log.info("flat")`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	statements := compilation.Program.Hooks[0].Statements
	if len(statements) != 3 {
		t.Fatalf("statements = %#v, want score, signal, switch", statements)
	}
	score, ok := statements[0].(*strategyir.LetStmt)
	if !ok || !strings.Contains(score.Expression, "ifelse") || !strings.Contains(score.Expression, "close * 2") {
		t.Fatalf("score statement = %#v", statements[0])
	}
	signal, ok := statements[1].(*strategyir.LetStmt)
	if !ok || !strings.Contains(signal.Expression, "ifelse") {
		t.Fatalf("signal statement = %#v", statements[1])
	}
	switchStmt, ok := statements[2].(*strategyir.IfStmt)
	if !ok || len(switchStmt.Then) != 1 || len(switchStmt.Else) != 1 {
		t.Fatalf("switch statement = %#v", statements[2])
	}
}

func TestAnalyzeScriptReturnsStructuredUnsupportedDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Unsupported", overlay=true)
x = request.security("NASDAQ:AAPL", "D", close)`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want false")
	}
	if len(analysis.Diagnostics) == 0 || !strings.Contains(analysis.Diagnostics[0].Message, "request.security") {
		t.Fatalf("diagnostics = %#v", analysis.Diagnostics)
	}
	if analysis.AST == nil || len(analysis.AST.Lines) == 0 {
		t.Fatalf("AST = %#v", analysis.AST)
	}
}

func TestAnalyzeScriptPreservesOriginalLineNumbers(t *testing.T) {
	analysis := AnalyzeScript(`

//@version=6
strategy("Loop", overlay=true)
for i = 0 to 3 by 0
    log.info("nope")`, AnalysisOptions{})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want false")
	}
	if len(analysis.Diagnostics) == 0 || analysis.Diagnostics[0].Line != 5 {
		t.Fatalf("diagnostics = %#v, want first diagnostic on line 5", analysis.Diagnostics)
	}
}

func TestHistoryReferencesIgnoreStringLiterals(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Strings", overlay=true)
label = "close[1]"
deeper = "close[2]"`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	statements := compilation.Program.Hooks[0].Statements
	if got := jftradeCheckedTypeAssertion[*strategyir.LetStmt](statements[0]).Expression; got != `"close[1]"` {
		t.Fatalf("label expression = %q", got)
	}
	if got := jftradeCheckedTypeAssertion[*strategyir.LetStmt](statements[1]).Expression; got != `"close[2]"` {
		t.Fatalf("deeper expression = %q", got)
	}
}

func jftradeCheckedTypeAssertion[T any](value any) T {
	typed, ok := value.(T)
	if !ok {
		panic("unexpected dynamic type")
	}
	return typed
}
