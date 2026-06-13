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
x = request.security(syminfo.tickerid, "D", close > ta.sma(close, 20))`)
	if err == nil || !strings.Contains(err.Error(), "request.security") {
		t.Fatalf("ValidateScript() complex expression error = %v, want request.security diagnostic", err)
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

func TestCompileSupportsMovingAverageRequestSecuritySubset(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("MTF MA", overlay=true)
fast = request.security(syminfo.tickerid, "D", ta.ema(close, 5))
slow = request.security(syminfo.tickerid, "60", ta.sma(close, 20))
dailyClose = request.security(syminfo.tickerid, "D", close)
dailyHlc3 = request.security(syminfo.tickerid, "D", hlc3)
dailyHlc3Ema = request.security(syminfo.tickerid, "D", ta.ema(hlc3, 20))
tf = input.timeframe("15", "MTF")
fifteenClose = request.security(syminfo.tickerid, tf, close)
fourHourHlc3 = request.security(syminfo.tickerid, "240", hlc3)
dailyPreviousClose = request.security(syminfo.tickerid, "D", close[1])
fifteenHlc3Ema = request.security(syminfo.tickerid, "15", ta.ema(hlc3, 20), gaps=barmerge.gaps_off, lookahead=barmerge.lookahead_off)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	first, ok := compilation.Program.Hooks[0].Statements[0].(*strategyir.LetStmt)
	if !ok || first.Expression != "ma(EMA, 5, day)" {
		t.Fatalf("first statement = %#v", compilation.Program.Hooks[0].Statements[0])
	}
	second, ok := compilation.Program.Hooks[0].Statements[1].(*strategyir.LetStmt)
	if !ok || second.Expression != "ma(SMA, 20, hour)" {
		t.Fatalf("second statement = %#v", compilation.Program.Hooks[0].Statements[1])
	}
	third, ok := compilation.Program.Hooks[0].Statements[2].(*strategyir.LetStmt)
	if !ok || third.Expression != "security_source(close, day)" {
		t.Fatalf("third statement = %#v", compilation.Program.Hooks[0].Statements[2])
	}
	fourth, ok := compilation.Program.Hooks[0].Statements[3].(*strategyir.LetStmt)
	if !ok || fourth.Expression != "security_source(hlc3, day)" {
		t.Fatalf("fourth statement = %#v", compilation.Program.Hooks[0].Statements[3])
	}
	fifth, ok := compilation.Program.Hooks[0].Statements[4].(*strategyir.LetStmt)
	if !ok || fifth.Expression != "ma(EMA, 20, day, hlc3)" {
		t.Fatalf("fifth statement = %#v", compilation.Program.Hooks[0].Statements[4])
	}
	sixth, ok := compilation.Program.Hooks[0].Statements[5].(*strategyir.LetStmt)
	if !ok || sixth.Expression != `"15"` {
		t.Fatalf("sixth statement = %#v", compilation.Program.Hooks[0].Statements[5])
	}
	seventh, ok := compilation.Program.Hooks[0].Statements[6].(*strategyir.LetStmt)
	if !ok || seventh.Expression != `security_source(close, "15m")` {
		t.Fatalf("seventh statement = %#v", compilation.Program.Hooks[0].Statements[6])
	}
	eighth, ok := compilation.Program.Hooks[0].Statements[7].(*strategyir.LetStmt)
	if !ok || eighth.Expression != `security_source(hlc3, "240m")` {
		t.Fatalf("eighth statement = %#v", compilation.Program.Hooks[0].Statements[7])
	}
	ninth, ok := compilation.Program.Hooks[0].Statements[8].(*strategyir.LetStmt)
	if !ok || ninth.Expression != "security_source(close, day, 1)" {
		t.Fatalf("ninth statement = %#v", compilation.Program.Hooks[0].Statements[8])
	}
	tenth, ok := compilation.Program.Hooks[0].Statements[9].(*strategyir.LetStmt)
	if !ok || tenth.Expression != `ma(EMA, 20, "15m", hlc3)` {
		t.Fatalf("tenth statement = %#v", compilation.Program.Hooks[0].Statements[9])
	}

	requirements, err := strategyir.PlanRequirements(compilation.Program)
	if err != nil {
		t.Fatalf("PlanRequirements() error = %v", err)
	}
	keys := map[string]bool{}
	for _, requirement := range requirements.Indicators {
		keys[requirement.Key] = true
	}
	for _, want := range []string{"ma:EMA:5:day", "ma:SMA:20:hour", "security_source:day:close", "security_source:day:hlc3", "ma:EMA:20:day:hlc3", "security_source:15m:close", "security_source:240m:hlc3", "security_source:day:close:1", "ma:EMA:20:15m:hlc3"} {
		if !keys[want] {
			t.Fatalf("requirements missing %q: %#v", want, requirements.Indicators)
		}
	}
}

func TestCompileSupportsPineStdev(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Stdev", overlay=true)
dev = 2.0 * ta.stdev(close, 20)
if close > dev
    strategy.entry("Long", strategy.long, qty=1)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	stmt, ok := compilation.Program.Hooks[0].Statements[0].(*strategyir.LetStmt)
	if !ok || stmt.Expression != "2.0 * stdev(close, 20)" {
		t.Fatalf("first statement = %#v", compilation.Program.Hooks[0].Statements[0])
	}
}

func TestCompileSupportsCommonTradingViewTAFunctions(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Common TA", overlay=true)
hh = ta.highest(high, 20)
hhDefault = ta.highest(20)
ll = ta.lowest(low, 10)
delta = ta.change(close)
momentum = ta.mom(close, 5)
rate = ta.roc(close, 12)
trendUp = ta.rising(close, 3)
wr = ta.wpr(14)
[basis, upper, lower] = ta.bb(close, 20, 2)
if trendUp and close > hh and close < upper and wr < -20
    strategy.entry("Long", strategy.long, qty=1)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	statements := compilation.Program.Hooks[0].Statements
	expected := []string{
		"highest(high, 20)",
		"highest(high, 20)",
		"lowest(low, 10)",
		"change(close, 1)",
		"mom(close, 5)",
		"roc(close, 12)",
		"rising(close, 3)",
		"williams_r(14)",
		"bollinger(20, 2)",
	}
	for index, want := range expected {
		stmt, ok := statements[index].(*strategyir.LetStmt)
		if !ok || stmt.Expression != want {
			t.Fatalf("statement %d = %#v, want expression %q", index, statements[index], want)
		}
	}
	ifStmt, ok := statements[9].(*strategyir.IfStmt)
	if !ok {
		t.Fatalf("statement 9 = %T", statements[9])
	}
	if want := "trendUp && close > hh && close < basis.upper && wr < -20"; ifStmt.Condition != want {
		t.Fatalf("condition = %q, want %q", ifStmt.Condition, want)
	}
}

func TestCompileSupportsInputMathCrossAndSourceAwareMovingAverages(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Input Math", overlay=true)
len = input.int(20, "Length")
src = input.source(close)
mult = input.float(defval=2.0, title="Mult")
avgVol = ta.sma(volume, len)
avgSrc = ta.ema(src, len)
wide = math.max(close, open) + math.floor(mult)
if ta.cross(avgSrc, avgVol) or bar_index > 20
    strategy.entry("Long", strategy.long, qty=1)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	statements := compilation.Program.Hooks[0].Statements
	expected := []string{
		"20",
		"close",
		"2.0",
		"ma(SMA, 20, volume)",
		"ma(EMA, 20)",
		"max(close, open) + floor(2.0)",
	}
	for index, want := range expected {
		stmt, ok := statements[index].(*strategyir.LetStmt)
		if !ok || stmt.Expression != want {
			t.Fatalf("statement %d = %#v, want expression %q", index, statements[index], want)
		}
	}
	ifStmt, ok := statements[6].(*strategyir.IfStmt)
	if !ok {
		t.Fatalf("statement 6 = %T", statements[6])
	}
	if want := "(cross_over(avgSrc, avgVol) || cross_under(avgSrc, avgVol)) || bar_index > 20"; ifStmt.Condition != want {
		t.Fatalf("condition = %q, want %q", ifStmt.Condition, want)
	}
}

func TestCompileSupportsPineStrategyPositionVariables(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Position vars", overlay=true)
stopPrice = strategy.position_avg_price * 0.95
if strategy.position_size > 0 and close < stopPrice
    strategy.close("Long")`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	stmt, ok := compilation.Program.Hooks[0].Statements[0].(*strategyir.LetStmt)
	if !ok || stmt.Expression != "position_avg_price * 0.95" {
		t.Fatalf("first statement = %#v", compilation.Program.Hooks[0].Statements[0])
	}
	ifStmt, ok := compilation.Program.Hooks[0].Statements[1].(*strategyir.IfStmt)
	if !ok || ifStmt.Condition != "position_size > 0 && close < stopPrice" {
		t.Fatalf("if statement = %#v", compilation.Program.Hooks[0].Statements[1])
	}
}

func TestAnalyzeScriptSupportsTrendAndStatefulTAFunctions(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Supertrend", overlay=true)
[line, direction] = ta.supertrend(3, 10)
[plusDI, minusDI, adx] = ta.dmi(14, 14)
v = ta.vwap(hlc3)
m = ta.mfi(hlc3, 14)
if ta.barssince(close > open) > 2 and ta.valuewhen(ta.cross(close, open), close, 0) > v and adx > 20
    strategy.entry("Long", strategy.long)`, AnalysisOptions{})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
}

func TestAnalyzeScriptSupportsSarBarstateSessionAndPineConstants(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("SAR", overlay=true)
startTime = input.time(timestamp(2026, 1, 1), "Start")
signalColor = input.color(color.green, "Signal")
transparent = color.new(color.red, 80)
custom = color.rgb(12, 34, 56)
sar = ta.sar(0.02, 0.02, 0.2)
if barstate.isconfirmed and session.ismarket and dayofweek == dayofweek.monday and month == month.january and time >= startTime and close > sar
    strategy.entry("Long", strategy.long)`, AnalysisOptions{})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	if len(analysis.Requirements.Indicators) != 1 || analysis.Requirements.Indicators[0].Key != "sar:0.02:0.02:0.2" {
		t.Fatalf("requirements = %#v", analysis.Requirements.Indicators)
	}
	statements := analysis.Program.Hooks[0].Statements
	expected := []string{
		"timestamp(2026, 1, 1)",
		"\"#4caf50\"",
		"\"#ff5252\"",
		"\"#0c2238\"",
		"sar(0.02, 0.02, 0.2)",
	}
	for index, want := range expected {
		stmt, ok := statements[index].(*strategyir.LetStmt)
		if !ok || stmt.Expression != want {
			t.Fatalf("statement %d = %#v, want %q", index, statements[index], want)
		}
	}
	ifStmt, ok := statements[5].(*strategyir.IfStmt)
	if !ok {
		t.Fatalf("statement 5 = %T", statements[5])
	}
	if want := "barstate_isconfirmed && session_ismarket && dayofweek == 2 && month == 1 && time >= startTime && close > sar"; ifStmt.Condition != want {
		t.Fatalf("condition = %q, want %q", ifStmt.Condition, want)
	}
}

func TestCompileSupportsOrderQtyPercentStrategyOrderAndCloseAll(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Orders", overlay=true, default_qty_type=strategy.percent_of_equity, default_qty_value=10)
strategy.entry("Long", strategy.long, qty_percent=25)
strategy.close("Long", qty_percent=50)
strategy.order("Net short", strategy.short, qty=5)
strategy.order("Net default", strategy.long)
strategy.close_all()`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	statements := compilation.Program.Hooks[0].Statements
	if len(statements) != 5 {
		t.Fatalf("statement count = %d", len(statements))
	}
	entry := statements[0].(*strategyir.OrderStmt)
	if entry.Intent != strategyir.OrderIntentEntry || entry.QuantityMode != "account_position_percent" || entry.QuantityExpression != "25" {
		t.Fatalf("entry = %#v", entry)
	}
	closeStmt := statements[1].(*strategyir.OrderStmt)
	if closeStmt.Intent != strategyir.OrderIntentClose || closeStmt.QuantityMode != "symbol_position_percent" || closeStmt.QuantityExpression != "50" {
		t.Fatalf("close = %#v", closeStmt)
	}
	netShort := statements[2].(*strategyir.OrderStmt)
	if netShort.Intent != strategyir.OrderIntentNet || netShort.Action != strategyir.OrderActionSell || netShort.QuantityMode != "shares" || netShort.QuantityExpression != "5" {
		t.Fatalf("net short = %#v", netShort)
	}
	netDefault := statements[3].(*strategyir.OrderStmt)
	if netDefault.Intent != strategyir.OrderIntentNet || netDefault.Action != strategyir.OrderActionBuy || netDefault.QuantityMode != "account_position_percent" || netDefault.QuantityExpression != "10" {
		t.Fatalf("net default = %#v", netDefault)
	}
	flatten := statements[4].(*strategyir.OrderStmt)
	if flatten.Intent != strategyir.OrderIntentFlatten || flatten.QuantityMode != "symbol_position_percent" || flatten.QuantityExpression != "100" {
		t.Fatalf("flatten = %#v", flatten)
	}
}

func TestCompileIgnoresVisualCallsWithWarning(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Visual", overlay=true)
plot(close)
alertcondition(close > open, "Up")
label.new(bar_index, close, "x")
if close > open
    plotshape(true)
strategy.entry("Long", strategy.long, qty=(strategy.equity * 25 / 100) / close, limit=101)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compilation.Warnings) != 4 || !strings.Contains(strings.Join(compilation.Warnings, "\n"), "alertcondition") || !strings.Contains(strings.Join(compilation.Warnings, "\n"), "label.new") {
		t.Fatalf("warnings = %#v", compilation.Warnings)
	}
	order, ok := compilation.Program.Hooks[0].Statements[0].(*strategyir.OrderStmt)
	if !ok {
		t.Fatalf("statement = %T", compilation.Program.Hooks[0].Statements[0])
	}
	if order.QuantityMode != "account_position_percent" || order.QuantityExpression != "25" || order.OrderType != "LIMIT" || order.LimitExpression != "101" {
		t.Fatalf("order = %#v", order)
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
	trailing, ok := compilation.Program.Hooks[0].Statements[3].(*strategyir.ProtectStmt)
	if !ok || trailing.Mode != "trailingStop" || trailing.Direction != "long" || trailing.PercentageExpression != "4" || trailing.QuantityMode != "symbol_position_percent" || trailing.QuantityExpression != "100" {
		t.Fatalf("trailing statement = %#v", compilation.Program.Hooks[0].Statements[3])
	}
}

func TestCompileSupportsPendingStopAndCancelOrders(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Pending", overlay=true)
strategy.entry("Breakout", strategy.long, stop=ta.highest(high, 20), qty=1)
strategy.order("Net short", strategy.short, stop=low - 1, qty=5)
strategy.cancel("Breakout")
strategy.cancel_all()`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	statements := compilation.Program.Hooks[0].Statements
	if len(statements) != 4 {
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
	cancel, ok := statements[2].(*strategyir.CancelStmt)
	if !ok || cancel.ID != "Breakout" || cancel.All {
		t.Fatalf("cancel statement = %#v", statements[2])
	}
	cancelAll, ok := statements[3].(*strategyir.CancelStmt)
	if !ok || !cancelAll.All {
		t.Fatalf("cancel_all statement = %#v", statements[3])
	}
}

func TestValidateScriptReportsUnsupportedAdvancedOrders(t *testing.T) {
	cases := []struct {
		name    string
		script  string
		message string
	}{
		{
			name: "strategy entry stop limit",
			script: `//@version=6
strategy("Entry stop limit", overlay=true)
strategy.entry("StopLimit", strategy.long, qty=1, stop=101, limit=99)`,
			message: "stop-limit",
		},
		{
			name: "strategy order stop limit",
			script: `//@version=6
strategy("Order stop limit", overlay=true)
strategy.order("StopLimit", strategy.long, qty=1, stop=101, limit=99)`,
			message: "stop-limit",
		},
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
	if got := statements[0].(*strategyir.LetStmt).Mode; got != strategyir.AssignmentModeVar {
		t.Fatalf("first assignment mode = %q", got)
	}
	if got := statements[1].(*strategyir.LetStmt).Mode; got != strategyir.AssignmentModeReassign {
		t.Fatalf("second assignment mode = %q", got)
	}
	if got := statements[2].(*strategyir.LetStmt).Expression; got != "ifelse(history(close, 1) == na, 0, nz(history(close, 1), close))" {
		t.Fatalf("signal expression = %q", got)
	}
	ifStmt := statements[3].(*strategyir.IfStmt)
	if ifStmt.Condition != "close > history(close, 1)" {
		t.Fatalf("condition = %q", ifStmt.Condition)
	}
}

func TestCompileSupportsExpressionUDFAndStaticForUnroll(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("UDF For", overlay=true)
isBull(src) => src > src[1]
ma(src, len) => ta.ema(src, len)
len = input.int(3, "Length")
fast = ma(close, len)
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
		"0 + history(close, 0)",
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
			name: "multi statement udf",
			script: `//@version=6
strategy("UDF", overlay=true)
f(x) =>
    y = x
    y
z = f(close)`,
			message: `multi-statement user-defined functions`,
		},
		{
			name: "dynamic for bound",
			script: `//@version=6
strategy("Loop", overlay=true)
limit = close
for i = 0 to limit
    log.info("nope")`,
			message: `for end must be a static integer constant or input.int default`,
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
			name: "break",
			script: `//@version=6
strategy("Loop", overlay=true)
for i = 0 to 3
    break`,
			message: `break is not supported`,
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

func TestAnalyzeScriptReturnsStructuredUnsupportedDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Loop", overlay=true)
limit = close
for i = 0 to limit
    log.info("nope")`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want false")
	}
	if len(analysis.Diagnostics) == 0 || !strings.Contains(analysis.Diagnostics[0].Message, "for end must be a static integer") {
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
	if got := statements[0].(*strategyir.LetStmt).Expression; got != `"close[1]"` {
		t.Fatalf("label expression = %q", got)
	}
	if got := statements[1].(*strategyir.LetStmt).Expression; got != `"close[2]"` {
		t.Fatalf("deeper expression = %q", got)
	}
}
