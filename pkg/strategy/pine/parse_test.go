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

func TestCompileSupportsV14WindowMomentumAndStatefulIndicators(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v1.4 window state", overlay=true)
dev = ta.stdev(close, 5)
variance = ta.variance(close, 5)
hb = ta.highestbars(high, 5)
lb = ta.lowestbars(low, 5)
delta = ta.change(close)
momentum = ta.mom(close, 3)
rate = ta.roc(close, 3)
up = ta.rising(close, 3)
down = ta.falling(close, 3)
bars = ta.barssince(close > open)
value = ta.valuewhen(close > open, close, 0)
trTrue = ta.tr(true)
trFalse = ta.tr(false)
if up and not down and nz(bars, 999) < 5 and nz(value, close) > 0 and trTrue >= trFalse
    strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript() diagnostics = %#v", analysis.Diagnostics)
	}
	keys := map[string]bool{}
	for _, requirement := range analysis.Requirements.Indicators {
		keys[requirement.Key] = true
	}
	for _, key := range []string{
		"stdev:5",
		"variance:close:5",
		"highestbars:high:5",
		"lowestbars:low:5",
		"change:close:1",
		"mom:close:3",
		"roc:close:3",
		"rising:close:3",
		"falling:close:3",
	} {
		if !keys[key] {
			t.Fatalf("requirements missing %q: %#v", key, analysis.Requirements.Indicators)
		}
	}
}

func TestCompileSupportsV14RequestSecurityPureExpression(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v1.4 MTF pure", overlay=true)
signal = request.security(syminfo.tickerid, "15", close > ta.sma(close, 3) and nz(close[1], close) > open)
if signal
    strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript() diagnostics = %#v", analysis.Diagnostics)
	}
	statements := analysis.Program.Hooks[0].Statements
	first, ok := statements[0].(*strategyir.LetStmt)
	if !ok {
		t.Fatalf("first statement = %T", statements[0])
	}
	wantExpression := `security_source(close, "15m") > ma(SMA, 3, "15m") && nz(security_source(close, "15m", 1), security_source(close, "15m")) > security_source(open, "15m")`
	if first.Expression != wantExpression {
		t.Fatalf("expression = %q, want %q", first.Expression, wantExpression)
	}
	keys := map[string]bool{}
	for _, requirement := range analysis.Requirements.Indicators {
		keys[requirement.Key] = true
	}
	for _, key := range []string{
		"security_source:15m:close",
		"security_source:15m:close:1",
		"security_source:15m:open",
		"ma:SMA:3:15m",
	} {
		if !keys[key] {
			t.Fatalf("requirements missing %q: %#v", key, analysis.Requirements.Indicators)
		}
	}
}

func TestCompileSupportsV15RequestSecurityCommonTAExpression(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v1.5 MTF common TA", overlay=true)
signal = request.security(syminfo.tickerid, "15", nz(ta.rsi(close, 14), 50) > 50 and nz(ta.macd(close, 12, 26, 9).diff, 0) > 0 and nz(ta.atr(14), 0) > 0 and nz(ta.bb(close, 20, 2).upper, close) > close and nz(ta.supertrend(3, 10).direction, 0) > 0)
spread = ta.range(close, 5)
modeValue = ta.mode(close, 5)
if signal
    strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript() diagnostics = %#v", analysis.Diagnostics)
	}
	first, ok := analysis.Program.Hooks[0].Statements[0].(*strategyir.LetStmt)
	if !ok {
		t.Fatalf("first statement = %T", analysis.Program.Hooks[0].Statements[0])
	}
	for _, fragment := range []string{
		`rsi(close, 14, "15m")`,
		`macd(12, 26, 9, "15m", close).diff`,
		`atr(14, "15m")`,
		`bollinger(20, 2, "15m", close).upper`,
		`supertrend(3, 10, "15m").direction`,
	} {
		if !strings.Contains(first.Expression, fragment) {
			t.Fatalf("expression = %q, missing %q", first.Expression, fragment)
		}
	}
	keys := map[string]bool{}
	for _, requirement := range analysis.Requirements.Indicators {
		keys[requirement.Key] = true
	}
	for _, key := range []string{
		"security_source:15m:close",
		"rsi:close:14:15m",
		"macd:close:12:26:9:15m",
		"atr:14:15m",
		"bollinger:close:20:2:15m",
		"supertrend:3:10:15m",
		"range:close:5",
		"mode:close:5",
	} {
		if !keys[key] {
			t.Fatalf("requirements missing %q: %#v", key, analysis.Requirements.Indicators)
		}
	}
}

func TestCompileSupportsV16RequestSecurityTupleWhitelist(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v1.6 MTF tuple", overlay=true)
[mtfClose, mtfFast, mtfUp] = request.security(syminfo.tickerid, "15", [close, ta.ema(hlc3, 5), close > ta.sma(close, 3)])
[macdLine, signalLine, histLine] = request.security(syminfo.tickerid, "15", ta.macd(close, 12, 26, 9))
[basis, upper, lower] = request.security(syminfo.tickerid, "15", ta.bb(close, 20, 2))
if mtfClose > mtfFast and mtfUp and histLine > signalLine and close < lower
    strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript() diagnostics = %#v", analysis.Diagnostics)
	}
	statements := analysis.Program.Hooks[0].Statements
	first, ok := statements[0].(*strategyir.LetStmt)
	if !ok {
		t.Fatalf("first statement = %T", statements[0])
	}
	if first.Expression != `security_source(close, "15m")` {
		t.Fatalf("first expression = %q", first.Expression)
	}
	second, ok := statements[1].(*strategyir.LetStmt)
	if !ok {
		t.Fatalf("second statement = %T", statements[1])
	}
	if second.Expression != `macd(12, 26, 9, "15m", close)` {
		t.Fatalf("second expression = %q", second.Expression)
	}
	third, ok := statements[2].(*strategyir.LetStmt)
	if !ok {
		t.Fatalf("third statement = %T", statements[2])
	}
	if third.Expression != `bollinger(20, 2, "15m", close)` {
		t.Fatalf("third expression = %q", third.Expression)
	}
	ifStmt, ok := statements[3].(*strategyir.IfStmt)
	if !ok {
		t.Fatalf("fourth statement = %T", statements[3])
	}
	for _, fragment := range []string{
		`ma(EMA, 5, "15m", hlc3)`,
		`security_source(close, "15m") > ma(SMA, 3, "15m")`,
		`macdLine.histogram > macdLine.signal`,
		`close < basis.lower`,
	} {
		if !strings.Contains(ifStmt.Condition, fragment) {
			t.Fatalf("condition = %q, missing %q", ifStmt.Condition, fragment)
		}
	}
	keys := map[string]bool{}
	for _, requirement := range analysis.Requirements.Indicators {
		keys[requirement.Key] = true
	}
	for _, key := range []string{
		"security_source:15m:close",
		"ma:EMA:5:15m:hlc3",
		"ma:SMA:3:15m",
		"macd:close:12:26:9:15m",
		"bollinger:close:20:2:15m",
	} {
		if !keys[key] {
			t.Fatalf("requirements missing %q: %#v", key, analysis.Requirements.Indicators)
		}
	}
}

func TestAnalyzeScriptIncludesV17SemanticSummary(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v1.7 Semantic", overlay=true)
len = input.int(8, "Length")
fast = ta.ema(close, len)
[mtfClose, mtfFast] = request.security(syminfo.tickerid, "15", [close, ta.ema(close, 5)])
if mtfClose > mtfFast and fast > fast[1]
    strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript() diagnostics = %#v", analysis.Diagnostics)
	}
	if analysis.Semantic == nil {
		t.Fatal("Semantic is nil")
	}
	symbolKinds := map[string]SemanticValueKind{}
	for _, symbol := range analysis.Semantic.Symbols {
		symbolKinds[symbol.Name] = symbol.ValueKind
	}
	if symbolKinds["len"] != SemanticValueSimple && symbolKinds["len"] != SemanticValueConst {
		t.Fatalf("len semantic kind = %q", symbolKinds["len"])
	}
	if symbolKinds["fast"] != SemanticValueSeries {
		t.Fatalf("fast semantic kind = %q", symbolKinds["fast"])
	}
	if symbolKinds["mtfClose"] != SemanticValueSeries || symbolKinds["mtfFast"] != SemanticValueSeries {
		t.Fatalf("mtf semantic kinds = %#v", symbolKinds)
	}
	if len(analysis.Semantic.TupleBindings) == 0 || analysis.Semantic.TupleBindings[0].ReturnCount != 2 || !analysis.Semantic.TupleBindings[0].Supported {
		t.Fatalf("tuple bindings = %#v", analysis.Semantic.TupleBindings)
	}
	foundEMA := false
	foundSecurity := false
	for _, call := range analysis.Semantic.FunctionCalls {
		if call.Name == "ta.ema" && call.Signature == "ta.ema(source, length)" {
			foundEMA = true
		}
		if call.Name == "request.security" && call.Supported {
			foundSecurity = true
		}
	}
	if !foundEMA || !foundSecurity {
		t.Fatalf("function calls = %#v", analysis.Semantic.FunctionCalls)
	}
}

func TestAnalyzeScriptReportsSemanticSignatureDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Bad Signature", overlay=true)
fast = ta.ema(close)`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want false")
	}
	if analysis.Semantic == nil {
		t.Fatal("Semantic is nil")
	}
	if len(analysis.Diagnostics) == 0 || analysis.Diagnostics[0].Code != "PINE_SEMANTIC_SIGNATURE" || analysis.Diagnostics[0].Line != 3 {
		t.Fatalf("diagnostics = %#v", analysis.Diagnostics)
	}
}

func TestCompileSupportsV15StaticForLoopControl(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Static For Control", overlay=true)
score = 0
for i = 1 to 4
    score := score + i
    continue
    score := score + 100
for j = 1 to 4
    score := score + j
    break
    score := score + 100
if score > 0
    strategy.entry("Long", strategy.long, qty=1)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	statements := compilation.Program.Hooks[0].Statements
	lets := make([]string, 0)
	for _, statement := range statements {
		if let, ok := statement.(*strategyir.LetStmt); ok {
			lets = append(lets, let.Expression)
		}
	}
	want := []string{"0", "score + 1", "score + 2", "score + 3", "score + 4", "score + 1"}
	if len(lets) != len(want) {
		t.Fatalf("let expressions = %#v, want %#v", lets, want)
	}
	for index := range want {
		if lets[index] != want[index] {
			t.Fatalf("let expression %d = %q, want %q (all=%#v)", index, lets[index], want[index], lets)
		}
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
	entry := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](statements[0])
	if entry.Intent != strategyir.OrderIntentEntry || entry.QuantityMode != "account_position_percent" || entry.QuantityExpression != "25" {
		t.Fatalf("entry = %#v", entry)
	}
	closeStmt := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](statements[1])
	if closeStmt.Intent != strategyir.OrderIntentClose || closeStmt.QuantityMode != "symbol_position_percent" || closeStmt.QuantityExpression != "50" {
		t.Fatalf("close = %#v", closeStmt)
	}
	netShort := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](statements[2])
	if netShort.Intent != strategyir.OrderIntentNet || netShort.Action != strategyir.OrderActionSell || netShort.QuantityMode != "shares" || netShort.QuantityExpression != "5" {
		t.Fatalf("net short = %#v", netShort)
	}
	netDefault := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](statements[3])
	if netDefault.Intent != strategyir.OrderIntentNet || netDefault.Action != strategyir.OrderActionBuy || netDefault.QuantityMode != "account_position_percent" || netDefault.QuantityExpression != "10" {
		t.Fatalf("net default = %#v", netDefault)
	}
	flatten := jftradeCheckedTypeAssertion[*strategyir.OrderStmt](statements[4])
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

func TestAnalyzeScriptReturnsVisualMetadata(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Visual Metadata", overlay=true)
plot(close, title="Close")
alertcondition(close > open, "Up")
if close > open
    plotshape(true)
label.new(bar_index, close, "Entry")
lbl = label.new(bar_index, close, "Assigned")
tbl = table.new(position.top_right, 1, 1)
table.cell(tbl, 0, 0, "Value")
strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript() diagnostics = %#v", analysis.Diagnostics)
	}
	if len(analysis.Visuals) != 7 {
		t.Fatalf("visuals = %#v, want seven", analysis.Visuals)
	}
	if analysis.Visuals[0].Kind != "plot" || analysis.Visuals[0].Call != "plot" || analysis.Visuals[0].Target != "close" || analysis.Visuals[0].Title != "Close" || analysis.Visuals[0].NamedArgs["title"] != `"Close"` {
		t.Fatalf("first visual = %#v", analysis.Visuals[0])
	}
	if analysis.Visuals[1].Kind != "alert" || analysis.Visuals[1].Title != "Up" || analysis.Visuals[1].Target != "close > open" {
		t.Fatalf("alert visual = %#v", analysis.Visuals[1])
	}
	if analysis.Visuals[3].Kind != "drawing" || analysis.Visuals[3].Title != "Entry" || analysis.Visuals[3].Target != `"Entry"` {
		t.Fatalf("drawing visual = %#v", analysis.Visuals[3])
	}
	if analysis.Visuals[4].Kind != "drawing" || analysis.Visuals[4].Call != "label.new" || analysis.Visuals[4].Variable != "lbl" || analysis.Visuals[4].Title != "Assigned" {
		t.Fatalf("assigned drawing visual = %#v", analysis.Visuals[4])
	}
	if analysis.Visuals[5].Kind != "table" || analysis.Visuals[5].Call != "table.new" || analysis.Visuals[5].Variable != "tbl" || analysis.Visuals[5].Target != "position.top_right" {
		t.Fatalf("assigned table visual = %#v", analysis.Visuals[5])
	}
	if analysis.Visuals[6].Kind != "table" || analysis.Visuals[6].Target != "tbl" {
		t.Fatalf("table visual = %#v", analysis.Visuals[6])
	}
	if analysis.Semantic == nil || len(analysis.Semantic.Visuals) != 7 {
		t.Fatalf("semantic visuals = %#v", analysis.Semantic)
	}
}

func TestAnalyzeScriptIncludesV20CollectionAndDeclarationSemantics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v2 Foundation", overlay=true)
arr = array.new_float(0)
prices = map.new<string, float>()
grid = matrix.new<float>(1, 1)
array.push(arr, close)
latest = array.get(arr, 0)
map.put(prices, "last", latest)
matrix.set(grid, 0, 0, latest)
type TradeBox
    float price = close
    int bars = 0
method reset(TradeBox box, float limit = 0) =>
    box
box = TradeBox.new(close, 0)
resetBox = box.reset(10)
import TradingView/ta/7 as tav7
export helper(float src, int length = 1) => src
library("JFTradeFoundation")`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want unsupported diagnostics for v2 parse-only surfaces")
	}
	if analysis.Semantic == nil {
		t.Fatal("Semantic is nil")
	}
	if len(analysis.Declarations) == 0 || len(analysis.Declarations) != len(analysis.Semantic.Declarations) {
		t.Fatalf("analysis declarations = %#v, semantic declarations = %#v", analysis.Declarations, analysis.Semantic.Declarations)
	}
	codes := map[string]bool{}
	for _, diagnostic := range analysis.Diagnostics {
		codes[diagnostic.Code] = true
	}
	if codes["PINE_COLLECTION_UNSUPPORTED"] || !codes["PINE_DECLARATION_UNSUPPORTED"] {
		t.Fatalf("diagnostic codes = %#v", codes)
	}
	namespaces := map[string]SemanticDeclaration{}
	declarations := map[string]SemanticDeclaration{}
	for _, declaration := range analysis.Semantic.Declarations {
		if declaration.Kind == "collection" {
			namespaces[declaration.Namespace] = declaration
			continue
		}
		declarations[declaration.Kind] = declaration
	}
	if namespaces["array"].Call != "array.new_float" || namespaces["array"].Name != "arr" {
		t.Fatalf("array declaration = %#v", namespaces["array"])
	}
	if namespaces["map"].Call != "map.new" || namespaces["map"].TypeArgs != "string, float" || namespaces["map"].Name != "prices" {
		t.Fatalf("map declaration = %#v", namespaces["map"])
	}
	if namespaces["matrix"].Call != "matrix.new" || namespaces["matrix"].TypeArgs != "float" || namespaces["matrix"].Name != "grid" {
		t.Fatalf("matrix declaration = %#v", namespaces["matrix"])
	}
	if declarations["type"].Name != "TradeBox" || declarations["method"].Name != "reset" || declarations["import"].Name != "TradingView/ta/7" || declarations["import"].Alias != "tav7" || declarations["export"].Name != "helper" || declarations["library"].Name != "JFTradeFoundation" {
		t.Fatalf("declarations = %#v", declarations)
	}
	if len(declarations["type"].Fields) != 2 || declarations["type"].Fields[0].Type != "float" || declarations["type"].Fields[0].Name != "price" || declarations["type"].Fields[0].Default != "close" || declarations["type"].Fields[1].Type != "int" || declarations["type"].Fields[1].Name != "bars" {
		t.Fatalf("type fields = %#v", declarations["type"].Fields)
	}
	if declarations["method"].Receiver == nil || declarations["method"].Receiver.Type != "TradeBox" || declarations["method"].Receiver.Name != "box" {
		t.Fatalf("method receiver = %#v", declarations["method"].Receiver)
	}
	if len(declarations["method"].Parameters) != 2 || declarations["method"].Parameters[1].Type != "float" || declarations["method"].Parameters[1].Name != "limit" || declarations["method"].Parameters[1].Default != "0" {
		t.Fatalf("method parameters = %#v", declarations["method"].Parameters)
	}
	if declarations["import"].ImportPath != "TradingView/ta/7" || declarations["import"].Version != "7" || declarations["import"].Alias != "tav7" {
		t.Fatalf("import declaration = %#v", declarations["import"])
	}
	if len(declarations["export"].Parameters) != 2 || declarations["export"].Parameters[0].Type != "float" || declarations["export"].Parameters[0].Name != "src" || declarations["export"].Parameters[1].Default != "1" {
		t.Fatalf("export declaration = %#v", declarations["export"])
	}
	if len(analysis.CollectionOperations) != 7 || len(analysis.CollectionOperations) != len(analysis.Semantic.CollectionOperations) {
		t.Fatalf("collection operations = %#v, semantic = %#v", analysis.CollectionOperations, analysis.Semantic.CollectionOperations)
	}
	operations := map[string]SemanticCollectionOperation{}
	for _, operation := range analysis.CollectionOperations {
		operations[operation.Call] = operation
	}
	if operations["array.new_float"].Target != "arr" || !operations["array.new_float"].Mutates {
		t.Fatalf("array.new_float operation = %#v", operations["array.new_float"])
	}
	if operations["array.push"].Target != "arr" || !operations["array.push"].Mutates || !operations["array.push"].Supported || operations["array.push"].Signature != "array.push(id, value)" || len(operations["array.push"].Arguments) != 2 {
		t.Fatalf("array.push operation = %#v", operations["array.push"])
	}
	if operations["array.get"].Target != "arr" || operations["array.get"].Mutates {
		t.Fatalf("array.get operation = %#v", operations["array.get"])
	}
	if operations["map.put"].Target != "prices" || !operations["map.put"].Mutates {
		t.Fatalf("map.put operation = %#v", operations["map.put"])
	}
	if operations["matrix.set"].Target != "grid" || !operations["matrix.set"].Mutates {
		t.Fatalf("matrix.set operation = %#v", operations["matrix.set"])
	}
	for _, operation := range analysis.CollectionOperations {
		if !operation.Executable {
			t.Fatalf("collection operation = %#v, want executable", operation)
		}
	}
	if len(analysis.ObjectOperations) != 2 || len(analysis.ObjectOperations) != len(analysis.Semantic.ObjectOperations) {
		t.Fatalf("object operations = %#v, semantic = %#v", analysis.ObjectOperations, analysis.Semantic.ObjectOperations)
	}
	objectOperations := map[string]SemanticObjectOperation{}
	for _, operation := range analysis.ObjectOperations {
		objectOperations[operation.Kind] = operation
	}
	if objectOperations["constructor"].Type != "TradeBox" || objectOperations["constructor"].Call != "TradeBox.new" || objectOperations["constructor"].Target != "box" || objectOperations["constructor"].Signature != "TradeBox.new(float price = close, int bars = 0)" || len(objectOperations["constructor"].Arguments) != 2 || objectOperations["constructor"].Executable {
		t.Fatalf("constructor object operation = %#v", objectOperations["constructor"])
	}
	if objectOperations["method"].Type != "TradeBox" || objectOperations["method"].Method != "reset" || objectOperations["method"].Call != "box.reset" || objectOperations["method"].Target != "box" || objectOperations["method"].Signature != "reset(TradeBox box, float limit = 0)" || len(objectOperations["method"].Arguments) != 1 || !objectOperations["method"].Supported || objectOperations["method"].Executable {
		t.Fatalf("method object operation = %#v", objectOperations["method"])
	}
	symbolKinds := map[string]SemanticValueKind{}
	for _, symbol := range analysis.Semantic.Symbols {
		symbolKinds[symbol.Name] = symbol.ValueKind
	}
	if symbolKinds["arr"] != SemanticValueObject || symbolKinds["prices"] != SemanticValueObject || symbolKinds["grid"] != SemanticValueObject || symbolKinds["box"] != SemanticValueObject {
		t.Fatalf("symbol kinds = %#v", symbolKinds)
	}
	if symbolKinds["latest"] != SemanticValueUnknown {
		t.Fatalf("latest semantic kind = %#v", symbolKinds["latest"])
	}
	for _, line := range analysis.AST.Lines {
		if strings.HasPrefix(line.Text, "arr = ") && line.Kind != NodeKindCollection {
			t.Fatalf("array AST kind = %q, want collection", line.Kind)
		}
	}
}

func TestPineV20LanguageFoundationGate(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v2 Foundation Gate", overlay=true)
var array<float> arr = array.new_float(0)
map<string, float> prices = map.new<string, float>()
matrix<float> grid = matrix.new<float>(1, 1)
arr.push(close)
prices.put("last", close)
grid.set(0, 0, close)
type TradeBox
    float price = close
method reset(TradeBox box, float limit = 0) =>
    box
box = TradeBox.new(close)
updated = box.reset(10)
import TradingView/ta/7 as tav7
library("JFTradeFoundation")
lbl = label.new(bar_index, close, "Entry")
tbl = table.new(position.top_right, 1, 1)
table.cell(tbl, 0, 0, "Ready")
plot(close, title="Close")`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want parse-only diagnostics for v2 foundation surfaces")
	}
	if analysis.AST == nil || analysis.Semantic == nil {
		t.Fatalf("analysis missing AST/semantic payload: %#v", analysis)
	}
	if len(analysis.Declarations) != 7 {
		t.Fatalf("declarations = %#v, want three collections plus type/method/import/library", analysis.Declarations)
	}
	if len(analysis.CollectionOperations) != 6 {
		t.Fatalf("collection operations = %#v", analysis.CollectionOperations)
	}
	if len(analysis.ObjectOperations) != 2 {
		t.Fatalf("object operations = %#v", analysis.ObjectOperations)
	}
	if len(analysis.Visuals) != 4 {
		t.Fatalf("visuals = %#v", analysis.Visuals)
	}
	codes := map[string]bool{}
	for _, diagnostic := range analysis.Diagnostics {
		codes[diagnostic.Code] = true
		if strings.HasPrefix(diagnostic.Code, "PINE_SEMANTIC_") {
			t.Fatalf("valid v2 foundation script returned semantic diagnostic: %#v", diagnostic)
		}
	}
	if codes["PINE_COLLECTION_UNSUPPORTED"] || !codes["PINE_DECLARATION_UNSUPPORTED"] {
		t.Fatalf("diagnostic codes = %#v, want explicit parse-only execution boundaries", codes)
	}
	capabilities := map[string]Capability{}
	for _, capability := range CapabilityRegistry() {
		capabilities[capability.ID] = capability
	}
	if capability := capabilities["syntax.arrays_maps_matrices"]; capability.Status != CapabilityPartial || !capability.Layers.Parser || !capability.Layers.Runtime || !capability.Layers.Planner {
		t.Fatalf("collection capability = %#v, want v2.1 partial runtime surface", capability)
	}
	if capability := capabilities["syntax.methods_types_libraries"]; capability.Status != CapabilityPartial || !capability.Layers.Parser || !capability.Layers.Runtime || !capability.Layers.Planner {
		t.Fatalf("declaration capability = %#v, want v2.2 partial runtime surface", capability)
	}
	if capability := capabilities["visual.noop_calls"]; capability.Status != CapabilityWarning || !capability.Layers.Parser || !capability.Layers.Frontend {
		t.Fatalf("visual capability = %#v", capability)
	}
	if capability := capabilities["tooling.visual_metadata_output"]; capability.Status != CapabilitySupported || !capability.Layers.Frontend {
		t.Fatalf("visual metadata capability = %#v", capability)
	}
	if capability := capabilities["order.full_tv_broker_emulator"]; capability.Status != CapabilityUnsupported {
		t.Fatalf("broker emulator capability = %#v, want explicitly separate unsupported roadmap", capability)
	}
}

func TestAnalyzeScriptReportsCollectionOperationSignatureDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Bad Collection", overlay=true)
arr = array.new_float(0)
array.push(arr)
matrix.set(grid, 0, 0)`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want collection diagnostics")
	}
	if len(analysis.CollectionOperations) != 3 {
		t.Fatalf("collection operations = %#v", analysis.CollectionOperations)
	}
	signatureDiagnostics := 0
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_COLLECTION_SIGNATURE" {
			signatureDiagnostics++
		}
	}
	if signatureDiagnostics != 2 {
		t.Fatalf("diagnostics = %#v, want two collection signature diagnostics", analysis.Diagnostics)
	}
}

func TestAnalyzeScriptIncludesCollectionMethodStyleOperations(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Collection Methods", overlay=true)
arr = array.new_float(0)
prices = map.new<string, float>()
grid = matrix.new<float>(1, 1)
arr.push(close)
latest = arr.get(0)
prices.put("last", latest)
grid.set(0, 0, latest)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	if len(analysis.CollectionOperations) != 7 {
		t.Fatalf("collection operations = %#v", analysis.CollectionOperations)
	}
	methodOperations := map[string]SemanticCollectionOperation{}
	for _, operation := range analysis.CollectionOperations {
		if !strings.HasPrefix(operation.Operation, "new") {
			methodOperations[operation.Target+"."+operation.Operation] = operation
		}
	}
	if methodOperations["arr.push"].Call != "array.push" || methodOperations["arr.push"].Signature != "array.push(id, value)" || len(methodOperations["arr.push"].Arguments) != 2 || methodOperations["arr.push"].Arguments[0] != "arr" || !methodOperations["arr.push"].Mutates {
		t.Fatalf("arr.push operation = %#v", methodOperations["arr.push"])
	}
	if methodOperations["arr.get"].Call != "array.get" || methodOperations["arr.get"].Target != "arr" || len(methodOperations["arr.get"].Arguments) != 2 || methodOperations["arr.get"].Mutates {
		t.Fatalf("arr.get operation = %#v", methodOperations["arr.get"])
	}
	if methodOperations["prices.put"].Call != "map.put" || methodOperations["prices.put"].Target != "prices" || len(methodOperations["prices.put"].Arguments) != 3 || !methodOperations["prices.put"].Mutates {
		t.Fatalf("prices.put operation = %#v", methodOperations["prices.put"])
	}
	if methodOperations["grid.set"].Call != "matrix.set" || methodOperations["grid.set"].Target != "grid" || len(methodOperations["grid.set"].Arguments) != 4 || !methodOperations["grid.set"].Mutates {
		t.Fatalf("grid.set operation = %#v", methodOperations["grid.set"])
	}
	for _, operation := range analysis.CollectionOperations {
		if !operation.Executable {
			t.Fatalf("operation = %#v, want executable", operation)
		}
	}
}

func TestAnalyzeScriptIncludesTypedCollectionDeclarations(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Typed Collections", overlay=true)
var array<float> arr = array.new_float(0)
map<string, float> prices = na
matrix<float> grid = matrix.new<float>(1, 1)
arr.push(close)
prices.put("last", close)
grid.set(0, 0, close)`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want parse-only collection diagnostics")
	}
	declarations := map[string]SemanticDeclaration{}
	for _, declaration := range analysis.Declarations {
		if declaration.Kind == "collection" {
			declarations[declaration.Name] = declaration
		}
	}
	if declarations["arr"].Namespace != "array" || declarations["arr"].TypeArgs != "float" || declarations["arr"].Call != "array.new_float" {
		t.Fatalf("array declaration = %#v", declarations["arr"])
	}
	if declarations["prices"].Namespace != "map" || declarations["prices"].TypeArgs != "string, float" || declarations["prices"].Call != "" {
		t.Fatalf("map declaration = %#v", declarations["prices"])
	}
	if declarations["grid"].Namespace != "matrix" || declarations["grid"].TypeArgs != "float" || declarations["grid"].Call != "matrix.new" {
		t.Fatalf("matrix declaration = %#v", declarations["grid"])
	}
	if len(analysis.CollectionOperations) != 5 {
		t.Fatalf("collection operations = %#v", analysis.CollectionOperations)
	}
	methodOperations := map[string]SemanticCollectionOperation{}
	for _, operation := range analysis.CollectionOperations {
		if !strings.HasPrefix(operation.Operation, "new") {
			methodOperations[operation.Target+"."+operation.Operation] = operation
		}
	}
	if methodOperations["arr.push"].Call != "array.push" || len(methodOperations["arr.push"].Arguments) != 2 {
		t.Fatalf("arr.push operation = %#v", methodOperations["arr.push"])
	}
	if methodOperations["prices.put"].Call != "map.put" || len(methodOperations["prices.put"].Arguments) != 3 {
		t.Fatalf("prices.put operation = %#v", methodOperations["prices.put"])
	}
	if methodOperations["grid.set"].Call != "matrix.set" || len(methodOperations["grid.set"].Arguments) != 4 {
		t.Fatalf("grid.set operation = %#v", methodOperations["grid.set"])
	}
	foundTypedAST := false
	for _, line := range analysis.AST.Lines {
		if line.Name == "arr" && line.Type == "array<float>" && line.Kind == NodeKindCollection {
			foundTypedAST = true
		}
	}
	if !foundTypedAST {
		t.Fatalf("typed AST lines = %#v", analysis.AST.Lines)
	}
}

func TestCompileSupportsV21ExecutableCollectionCore(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Executable Collections", overlay=true)
var array<float> values = array.new_float(0)
var map<string, float> prices = map.new<string, float>()
var matrix<float> grid = matrix.new<float>(1, 1, 0)
values.push(close)
prices.put("last", close)
grid.set(0, 0, close)
latest = values.last()
known = prices.contains("last")
cell = grid.get(0, 0)
if values.size() > 0 and known and cell == latest
    strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	if analysis.Program == nil || len(analysis.Program.Hooks) != 1 {
		t.Fatalf("program = %#v", analysis.Program)
	}
	collectionStatements := 0
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if _, ok := statement.(*strategyir.CollectionStmt); ok {
			collectionStatements++
		}
	}
	if collectionStatements != 9 {
		t.Fatalf("collection statements = %d, program = %#v", collectionStatements, analysis.Program.Hooks[0].Statements)
	}
	for _, operation := range analysis.CollectionOperations {
		if !operation.Executable {
			t.Fatalf("operation = %#v, want executable", operation)
		}
	}
	declarations := map[string]SemanticDeclaration{}
	for _, declaration := range analysis.Declarations {
		declarations[declaration.Name] = declaration
	}
	for _, name := range []string{"values", "prices", "grid"} {
		if !declarations[name].Executable {
			t.Fatalf("declaration %s = %#v, want executable", name, declarations[name])
		}
	}
}

func TestCompileSupportsV21CollectionAliases(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("collection aliases")
var values = array.new_float()
alias = values
alias.push(close)
latest = alias.last()
if latest > 0
    strategy.entry("Long", strategy.long)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	var push *strategyir.CollectionStmt
	for _, statement := range compilation.Program.Hooks[0].Statements {
		collection, ok := statement.(*strategyir.CollectionStmt)
		if ok && collection.Operation == "push" {
			push = collection
			break
		}
	}
	if push == nil || push.Namespace != "array" || push.Target != "alias" {
		t.Fatalf("alias push = %#v", push)
	}
}

func TestCompileSupportsV21BBWAndCOG(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("v21 ta")
width = ta.bbw(close, 5, 2)
gravity = ta.cog(hlc3, 5)
weeklyVWAP = ta.vwap(hlc3, timeframe.change("W"))
mtfWidth = request.security(syminfo.tickerid, "15", ta.bbw(close, 5, 2))
mtfGravity = request.security(syminfo.tickerid, "15", ta.cog(hlc3, 5))
if width >= 0 and gravity <= 0 and weeklyVWAP > 0 and mtfWidth >= 0 and mtfGravity <= 0
    strategy.entry("Long", strategy.long)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	keys := map[string]bool{}
	for _, requirement := range compilation.Requirements.Indicators {
		keys[requirement.Key] = true
	}
	for _, key := range []string{
		"bbw:close:5:2",
		"cog:hlc3:5",
		"anchored_vwap:week:hlc3",
		"bbw:close:5:2:15m",
		"cog:hlc3:5:15m",
	} {
		if !keys[key] {
			t.Fatalf("requirements = %#v, missing %s", compilation.Requirements.Indicators, key)
		}
	}
}

func TestCompileSupportsV22StructuredASTGeneralTupleAndDynamicLoops(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v22 structured")
[a, b, _, d] = [open, high, low, close]
[mtfOpen, mtfHigh, mtfLow, mtfClose] = request.security(syminfo.tickerid, "15", [open, high, low, close])
limit = bar_index % 3
total = 0
for i = 0 to limit
    total := total + i
count = 0
while count < 3
    count := count + 1
    if count == 2
        continue
    if count >= 3
        break
if d >= a and mtfClose >= mtfOpen and total >= 0 and count == 3
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	if analysis.AST == nil || len(analysis.AST.Nodes) == 0 {
		t.Fatalf("structured AST = %#v", analysis.AST)
	}
	foundLoopChildren := false
	for _, node := range analysis.AST.Nodes {
		if (node.Line.Kind == NodeKindFor || node.Line.Kind == NodeKindWhile) && len(node.Children) > 0 {
			foundLoopChildren = true
		}
	}
	if !foundLoopChildren {
		t.Fatalf("AST nodes = %#v, want loop children", analysis.AST.Nodes)
	}
	tuples, loops := 0, 0
	for _, statement := range analysis.Program.Hooks[0].Statements {
		switch statement.(type) {
		case *strategyir.TupleStmt:
			tuples++
		case *strategyir.LoopStmt:
			loops++
		}
	}
	if tuples != 2 || loops != 2 {
		t.Fatalf("tuples=%d loops=%d statements=%#v", tuples, loops, analysis.Program.Hooks[0].Statements)
	}
}

func TestCompileSupportsV22PureUDTAndMethodSubset(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v22 objects")
type PriceBox
    float price = close
    int bars = 1
method score(PriceBox self, float factor = 1) => self.price * factor + self.bars
box = PriceBox.new(close, 2)
value = box.score(2)
if value > close
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	if len(analysis.Program.Types) != 1 || len(analysis.Program.Methods) != 1 {
		t.Fatalf("types=%#v methods=%#v", analysis.Program.Types, analysis.Program.Methods)
	}
	objects := 0
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if _, ok := statement.(*strategyir.ObjectStmt); ok {
			objects++
		}
	}
	if objects != 2 {
		t.Fatalf("object statements = %d, statements = %#v", objects, analysis.Program.Hooks[0].Statements)
	}
	for _, declaration := range analysis.Declarations {
		if (declaration.Kind == "type" || declaration.Kind == "method") && !declaration.Executable {
			t.Fatalf("declaration = %#v, want executable", declaration)
		}
	}
	for _, operation := range analysis.ObjectOperations {
		if !operation.Executable {
			t.Fatalf("object operation = %#v, want executable", operation)
		}
	}
}

func TestCompileSupportsV23NamedObjectArgsAndPureMethodBody(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v23 objects")
type PriceBox
    float price = close
    int bars = 1
method score(PriceBox self, float factor = 1, float offset = 0) =>
    base = self.price * factor
    base + self.bars + offset
box = PriceBox.new(bars=3, price=close)
value = box.score(offset=2, factor=2)
if value > close
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	if len(analysis.Program.Methods) != 1 {
		t.Fatalf("methods = %#v", analysis.Program.Methods)
	}
	if got, want := analysis.Program.Methods[0].Body, "(self.price * factor) + self.bars + offset"; got != want {
		t.Fatalf("method body = %q, want %q", got, want)
	}
	objectStatements := make([]*strategyir.ObjectStmt, 0)
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if object, ok := statement.(*strategyir.ObjectStmt); ok {
			objectStatements = append(objectStatements, object)
		}
	}
	if len(objectStatements) != 2 {
		t.Fatalf("object statements = %#v", objectStatements)
	}
	if got, want := strings.Join(objectStatements[0].Arguments, ","), "close,3"; got != want {
		t.Fatalf("constructor args = %q, want %q", got, want)
	}
	if got, want := strings.Join(objectStatements[1].Arguments, ","), "2,2"; got != want {
		t.Fatalf("method args = %q, want %q", got, want)
	}
}

func TestCompileSupportsV23LocalObjectFieldReassignment(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v23 object fields")
type PriceBox
    float price = close
box = PriceBox.new()
box.price := close + 1
if box.price > close
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	fieldSets := 0
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if object, ok := statement.(*strategyir.ObjectStmt); ok && object.Operation == "field_set" {
			fieldSets++
		}
	}
	if fieldSets != 1 {
		t.Fatalf("field sets = %d, statements = %#v", fieldSets, analysis.Program.Hooks[0].Statements)
	}

	persistent := AnalyzeScript(`//@version=6
strategy("v24 persistent object fields")
type PriceBox
    float price = close
var box = PriceBox.new()
box.price := close + 1`, AnalysisOptions{IncludeAST: true})
	if !persistent.OK {
		t.Fatalf("persistent object field reassignment diagnostics = %#v, want OK", persistent.Diagnostics)
	}
}

func TestCompileSupportsV23RequestSecurityPureObjectAndCollectionExpressions(t *testing.T) {
	state := &parseState{
		collectionNamespaces: map[string]string{"values": "array"},
		objectTypes:          map[string]string{},
		udtMethods:           map[string][]strategyir.MethodDefinition{},
	}
	normalized := state.normalizeExpression(`request.security(syminfo.tickerid, "15", values.last())`)
	if normalized != "collection_array_last(values)" {
		t.Fatalf("normalized collection MTF expression = %q", normalized)
	}

	analysis := AnalyzeScript(`//@version=6
strategy("v23 mtf object collection")
values = array.new_float(0)
values.push(close)
type PriceBox
    float price = close
method score(PriceBox self, float factor = 1) => self.price * factor
box = PriceBox.new()
mtfLast = request.security(syminfo.tickerid, "15", values.last())
mtfField = request.security(syminfo.tickerid, "15", box.price)
mtfScore = request.security(syminfo.tickerid, "15", box.score(2))
if mtfLast > 0 and mtfField > 0 and mtfScore > 0
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	got := make([]string, 0)
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if let, ok := statement.(*strategyir.LetStmt); ok && strings.HasPrefix(let.Name, "mtf") {
			got = append(got, let.Expression)
		}
	}
	joined := strings.Join(got, "\n")
	for _, fragment := range []string{"collection_array_last(values)", "box.price", "object_method"} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("mtf expressions = %q, missing %q", joined, fragment)
		}
	}
}

func TestCompileSupportsV24CollectionExpansionAndMTFStoch(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v24 collections and stoch")
values = array.from(close, open, high)
values.sort(order.descending)
indices = values.sort_indices(order.ascending)
joined = indices.join(",")
lookup = values.binary_search(close)
middle = values.median()
spread = values.range()
prices = map.new<string, float>()
prices.put("b", close)
prices.put("a", open)
keys = prices.keys()
vals = prices.values()
mtfStoch = request.security(syminfo.tickerid, "15", ta.stoch(close, high, low, 14))
if mtfStoch >= 0 and middle >= 0 and spread >= 0 and lookup >= -1 and vals.size() >= 0
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	var joined strings.Builder
	var collectionOps strings.Builder
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if let, ok := statement.(*strategyir.LetStmt); ok {
			joined.WriteString(let.Expression + "\n")
		}
		if collection, ok := statement.(*strategyir.CollectionStmt); ok {
			collectionOps.WriteString(collection.Namespace + "." + collection.Operation + "\n")
		}
	}
	for _, fragment := range []string{`stoch(close, high, low, 14, "15m")`} {
		if !strings.Contains(joined.String(), fragment) {
			t.Fatalf("compiled expressions = %q, missing %q", joined.String(), fragment)
		}
	}
	for _, fragment := range []string{"array.from", "array.median", "map.values"} {
		if !strings.Contains(collectionOps.String(), fragment) {
			t.Fatalf("collection ops = %q, missing %q", collectionOps.String(), fragment)
		}
	}
	keys := map[string]bool{}
	for _, requirement := range analysis.Requirements.Indicators {
		keys[requirement.Key] = true
	}
	if !keys["stoch:close:14:15m"] {
		t.Fatalf("requirements = %#v, missing stoch:close:14:15m", analysis.Requirements.Indicators)
	}
}

func TestCompileSupportsV24NamedObjectMethodExpressionAndRuntimeLoopFallback(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v24 object method and loop")
type PriceBox
    float price = close
    int bars = 1
method score(PriceBox self, float factor = 1, float offset = 0) =>
    base = self.price * factor
    base + offset + self.bars
box = PriceBox.new(price=close, bars=2)
value = box.score(offset=3, factor=2)
mtfValue = request.security(syminfo.tickerid, "15", box.score(offset=1, factor=2))
total = 0
for i = 0 to 5
    if i == 3
        break
    total := total + i
if value > 0 and mtfValue > 0 and total >= 0
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	foundLoop := false
	foundNamedMethod := false
	for _, statement := range analysis.Program.Hooks[0].Statements {
		switch typed := statement.(type) {
		case *strategyir.LoopStmt:
			foundLoop = true
		case *strategyir.ObjectStmt:
			if typed.Operation == "method" && typed.Method == "score" && strings.Join(typed.Arguments, ",") == "2,3" {
				foundNamedMethod = true
			}
		case *strategyir.LetStmt:
			if strings.Contains(typed.Expression, "object_method") && strings.Contains(typed.Expression, "2, 1") {
				foundNamedMethod = true
			}
		}
	}
	if !foundLoop {
		t.Fatalf("statements = %#v, want runtime loop fallback", analysis.Program.Hooks[0].Statements)
	}
	if !foundNamedMethod {
		t.Fatalf("statements = %#v, want named method expression lowering", analysis.Program.Hooks[0].Statements)
	}
}

func TestCompileSupportsV25ArrayStringAndTimeframeHelpers(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v25 helpers")
values = array.from(-2, 1, 2, 2, 5)
absValues = values.abs()
left = values.binary_search_leftmost(2)
right = values.binary_search_rightmost(2)
rank = values.percentrank(3)
p50 = values.percentile_nearest_rank(50)
p50lin = values.percentile_linear_interpolation(50)
dev = values.stdev()
variance = values.variance()
other = array.from(2, 4, 6, 8, 10)
cov = values.covariance(other)
labelText = str.format("{0}:{1}", str.upper("alpha"), str.length("beta"))
changed = timeframe.change("15")
tc = time_close
if absValues.size() == 5 and left >= 0 and right >= left and rank >= 0 and p50 >= 0 and p50lin >= 0 and dev >= 0 and variance >= 0 and cov >= 0 and str.contains(labelText, "ALPHA") and tc > time and changed
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	var collectionOps strings.Builder
	var expressions strings.Builder
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if collection, ok := statement.(*strategyir.CollectionStmt); ok {
			collectionOps.WriteString(collection.Namespace + "." + collection.Operation + "\n")
		}
		if let, ok := statement.(*strategyir.LetStmt); ok {
			expressions.WriteString(let.Expression + "\n")
		}
	}
	for _, fragment := range []string{"array.abs", "array.binary_search_leftmost", "array.percentile_linear_interpolation", "array.covariance"} {
		if !strings.Contains(collectionOps.String(), fragment) {
			t.Fatalf("collection ops = %q, missing %q", collectionOps.String(), fragment)
		}
	}
	for _, fragment := range []string{"str_format", "str_upper", "str_length", "timeframe_change", "time_close"} {
		if !strings.Contains(expressions.String(), fragment) {
			t.Fatalf("expressions = %q, missing %q", expressions.String(), fragment)
		}
	}
}

func TestCompileSupportsV26CollectionIterationHistoryAndObjectCollectionFields(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v26 collection foundation")
type Box
    array<float> values
values = array.from(1, 2, 3)
total = 0
for [i, value] in values
    if i == 2
        break
    total := total + value
previousFirst = values[1].get(0)
box = Box.new(array.new_float())
box.values.push(close)
fieldSize = box.values.size()
if total >= 3 and nz(previousFirst, 0) >= 0 and fieldSize > 0
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	foundCollectionLoop := false
	var expressions strings.Builder
	var collectionOps strings.Builder
	for _, statement := range analysis.Program.Hooks[0].Statements {
		switch typed := statement.(type) {
		case *strategyir.LoopStmt:
			if typed.Collection == "values" && typed.IndexVariable == "i" && typed.Variable == "value" {
				foundCollectionLoop = true
			}
		case *strategyir.LetStmt:
			expressions.WriteString(typed.Expression + "\n")
		case *strategyir.CollectionStmt:
			collectionOps.WriteString(typed.Target + "." + typed.Operation + "\n")
		case *strategyir.ObjectStmt:
			expressions.WriteString(strings.Join(typed.Arguments, "\n") + "\n")
		}
	}
	if !foundCollectionLoop {
		t.Fatalf("statements = %#v, want collection for loop", analysis.Program.Hooks[0].Statements)
	}
	for _, fragment := range []string{"collection_array_get(history(values, 1), 0)", "collection_array_new_float()"} {
		if !strings.Contains(expressions.String(), fragment) {
			t.Fatalf("expressions = %q, missing %q", expressions.String(), fragment)
		}
	}
	if !strings.Contains(collectionOps.String(), "box.values.push") {
		t.Fatalf("collection ops = %q, missing box.values.push", collectionOps.String())
	}
	if !strings.Contains(collectionOps.String(), "box.values.size") {
		t.Fatalf("collection ops = %q, missing box.values.size", collectionOps.String())
	}
}

func TestCompileSupportsV27CollectionTimeframeAndMTFHelpers(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v27 helpers")
values = array.from(1, 2, 4, 8)
prevRange = values[1].range()
prevDev = values[1].stdev()
labels = map.new<string, float>()
labels.put("b", 2)
labels.put("a", 1)
total = 0
for key in labels.keys()
    total := total + labels.get(key)
grid = matrix.new<float>(2, 2, 0)
grid.set(1, 1, close)
cell = grid.get(1, 1)
rows = grid.rows()
cols = grid.columns()
seconds = timeframe.in_seconds("15")
mult = timeframe.multiplier
mtf = request.security(syminfo.tickerid, "15", str.length(str.format("{0}", close)) + timeframe.in_seconds("15"))
if nz(prevRange, 0) >= 0 and nz(prevDev, 0) >= 0 and total == 3 and rows == 2 and cols == 2 and cell > 0 and seconds == 900 and mult >= 1 and mtf > 0
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	foundCollectionLoop := false
	var expressions strings.Builder
	var collectionOps strings.Builder
	for _, statement := range analysis.Program.Hooks[0].Statements {
		switch typed := statement.(type) {
		case *strategyir.LoopStmt:
			if typed.Collection == "collection_map_keys(labels)" && typed.Variable == "key" {
				foundCollectionLoop = true
			}
		case *strategyir.LetStmt:
			expressions.WriteString(typed.Expression + "\n")
		case *strategyir.CollectionStmt:
			collectionOps.WriteString(typed.Namespace + "." + typed.Operation + ":" + typed.Target + "\n")
		}
	}
	if !foundCollectionLoop {
		t.Fatalf("statements = %#v, want map.keys collection loop", analysis.Program.Hooks[0].Statements)
	}
	for _, fragment := range []string{"collection_array_range(history(values, 1))", "collection_array_stdev(history(values, 1))", "timeframe_in_seconds", "timeframe_multiplier", "str_length", "str_format"} {
		if !strings.Contains(expressions.String(), fragment) {
			t.Fatalf("expressions = %q, missing %q", expressions.String(), fragment)
		}
	}
	for _, fragment := range []string{"matrix.set:grid", "matrix.get:grid", "matrix.rows:grid", "matrix.columns:grid"} {
		if !strings.Contains(collectionOps.String(), fragment) {
			t.Fatalf("collection ops = %q, missing %q", collectionOps.String(), fragment)
		}
	}
}

func TestCompileSupportsV28ObjectHistoryMethodChainAndExportMetadata(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v28 object semantic")
type PriceBox
    float price = close
method identity(PriceBox self) => self
method score(PriceBox self, float factor = 1) => self.price * factor
box = PriceBox.new(close)
prevPrice = box[1].price
chained = box.identity().score(2)
export helper(float src) => src
export type ExportedBox
export method exportedScore(PriceBox self) => self.price
if nz(prevPrice, 0) >= 0 and chained > 0
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	var expressions strings.Builder
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if let, ok := statement.(*strategyir.LetStmt); ok {
			expressions.WriteString(let.Expression + "\n")
		}
	}
	if !strings.Contains(expressions.String(), "history(box, 1).price") || !strings.Contains(expressions.String(), `object_method("PriceBox", "score", object_method("PriceBox", "identity", box), 2)`) {
		t.Fatalf("expressions = %q, want object history and method chain lowering", expressions.String())
	}
	exports := map[string]string{}
	for _, declaration := range analysis.Semantic.Declarations {
		if declaration.Kind == "export" {
			exports[declaration.Name] = declaration.ExportedKind
		}
	}
	if exports["helper"] != "function" || exports["ExportedBox"] != "type" || exports["exportedScore"] != "method" {
		t.Fatalf("exports = %#v", exports)
	}
}

func TestCompileSupportsV29ObjectHistoryMethodReceiverAndMTFHistoryExpression(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v29 object history receiver")
type PriceBox
    float price = close
method identity(PriceBox self) => self
method score(PriceBox self, float factor = 1, float offset = 0) => self.price * factor + offset
box = PriceBox.new(close)
prevScore = box[1].score(factor=2, offset=1)
chained = box.identity().score(offset=1, factor=2)
mtfPrev = request.security(syminfo.tickerid, "15", box[1].price + box[1].score(offset=1, factor=2))
if nz(prevScore, 0) >= 0 and chained > 0 and mtfPrev >= 0
    strategy.entry("Long", strategy.long)`, AnalysisOptions{IncludeAST: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	var expressions strings.Builder
	for _, statement := range analysis.Program.Hooks[0].Statements {
		if let, ok := statement.(*strategyir.LetStmt); ok {
			expressions.WriteString(let.Expression + "\n")
		}
	}
	for _, fragment := range []string{
		`object_method("PriceBox", "score", history(box, 1), 2, 1)`,
		`object_method("PriceBox", "score", object_method("PriceBox", "identity", box), 2, 1)`,
		`history(box, 1).price`,
	} {
		if !strings.Contains(expressions.String(), fragment) {
			t.Fatalf("expressions = %q, missing %q", expressions.String(), fragment)
		}
	}
}

func TestAnalyzeScriptReportsV29RequestSecurityDiagnostics(t *testing.T) {
	cases := []struct {
		name string
		body string
		code string
	}{
		{name: "dynamic symbol", body: `x = request.security("NASDAQ:AAPL", "D", close)`, code: "PINE_REQUEST_SECURITY_DYNAMIC_SYMBOL"},
		{name: "dynamic timeframe", body: `tf = input.timeframe("15", "TF")
x = request.security(syminfo.tickerid, tf + "", close)`, code: "PINE_REQUEST_SECURITY_DYNAMIC_TIMEFRAME"},
		{name: "nested", body: `x = request.security(syminfo.tickerid, "D", request.security(syminfo.tickerid, "15", close))`, code: "PINE_REQUEST_SECURITY_NESTED"},
		{name: "side effect", body: `x = request.security(syminfo.tickerid, "D", alert("no side effects"))`, code: "PINE_REQUEST_SECURITY_SIDE_EFFECT"},
		{name: "lookahead", body: `x = request.security(syminfo.tickerid, "D", close, lookahead=barmerge.lookahead_on)`, code: "PINE_REQUEST_SECURITY_LOOKAHEAD"},
		{name: "gaps", body: `x = request.security(syminfo.tickerid, "D", close, gaps=barmerge.gaps_on)`, code: "PINE_REQUEST_SECURITY_GAPS"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			analysis := AnalyzeScript(`//@version=6
strategy("request diagnostics")
`+item.body, AnalysisOptions{IncludeAST: true})
			if analysis.OK {
				t.Fatalf("AnalyzeScript().OK = true, want false")
			}
			found := false
			for _, diagnostic := range analysis.Diagnostics {
				if diagnostic.Code == item.code {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("diagnostics = %#v, missing %s", analysis.Diagnostics, item.code)
			}
		})
	}
}

func TestCompileSupportsV30SemanticDeclarationModelAndVaripPolicy(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("v30 semantic")
type PriceBox
    float price = close
method score(PriceBox self, float factor = 1) => self.price * factor
varip count = 0
box = PriceBox.new(close)
score = box.score(2)
count := count + 1
export helper(float src) => src`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript().OK = false, diagnostics = %#v", analysis.Diagnostics)
	}
	if len(analysis.Warnings) == 0 || !strings.Contains(strings.Join(analysis.Warnings, "\n"), "varip uses closed-bar var semantics") {
		t.Fatalf("warnings = %#v, want varip policy warning", analysis.Warnings)
	}
	declarations := map[string]SemanticDeclaration{}
	for _, declaration := range analysis.Semantic.Declarations {
		declarations[declaration.Kind+":"+declaration.Name] = declaration
	}
	if declarations["type:PriceBox"].Signature != "PriceBox.new(float price = close)" || !declarations["type:PriceBox"].Executable || declarations["type:PriceBox"].UnsupportedReason != "" {
		t.Fatalf("type declaration = %#v", declarations["type:PriceBox"])
	}
	if declarations["method:score"].Signature != "score(PriceBox self, float factor = 1)" || !declarations["method:score"].Executable || declarations["method:score"].UnsupportedReason != "" {
		t.Fatalf("method declaration = %#v", declarations["method:score"])
	}
	if declarations["export:helper"].Signature != "export helper(float src)" || declarations["export:helper"].UnsupportedReason == "" {
		t.Fatalf("export declaration = %#v", declarations["export:helper"])
	}

	importAnalysis := AnalyzeScript(`//@version=6
strategy("v30 import")
import TradingView/ta/7 as tools`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if importAnalysis.Semantic == nil || len(importAnalysis.Semantic.Declarations) == 0 {
		t.Fatalf("import semantic = %#v", importAnalysis.Semantic)
	}
	importDeclaration := importAnalysis.Semantic.Declarations[0]
	if importDeclaration.Signature != "import TradingView/ta/7 as tools" || importDeclaration.Version != "7" || importDeclaration.UnsupportedReason == "" {
		t.Fatalf("import declaration = %#v", importDeclaration)
	}
}

func TestAnalyzeScriptReportsCollectionTypeDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Bad Collection Types", overlay=true)
array<float, int> tooMany = na
map<string> missingValue = na
matrix<float, int> badMatrix = matrix.new<float, int>(1, 1)
array<float> wrongNamespace = map.new<string, float>()
array<int> wrongElement = array.new_float(0)
map<string, float> wrongMap = map.new<string, int>()`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want collection type diagnostics")
	}
	typeDiagnostics := make([]Diagnostic, 0)
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_COLLECTION_TYPE" {
			typeDiagnostics = append(typeDiagnostics, diagnostic)
		}
	}
	if len(typeDiagnostics) != 7 {
		t.Fatalf("diagnostics = %#v, want seven collection type diagnostics", analysis.Diagnostics)
	}
	messageParts := make([]string, 0, len(typeDiagnostics))
	for _, diagnostic := range typeDiagnostics {
		messageParts = append(messageParts, diagnostic.Message)
	}
	messages := strings.Join(messageParts, "\n")
	for _, fragment := range []string{
		"type annotation requires 1 type argument(s), got 2",
		"type annotation requires 2 type argument(s), got 1",
		"matrix.new requires 1 type argument(s), got 2",
		"array declaration cannot be initialized with map.new",
		"wrongElement type arguments <int> do not match array.new_float element types <float>",
		"wrongMap type arguments <string, float> do not match map.new element types <string, int>",
	} {
		if !strings.Contains(messages, fragment) {
			t.Fatalf("diagnostic messages = %q, missing %q", messages, fragment)
		}
	}
}

func TestAnalyzeScriptReportsCollectionMethodStyleSignatureDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Bad Collection Methods", overlay=true)
arr = array.new_float(0)
grid = matrix.new<float>(1, 1)
arr.push()
grid.set(0, 0)`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want collection method diagnostics")
	}
	if len(analysis.CollectionOperations) != 4 {
		t.Fatalf("collection operations = %#v", analysis.CollectionOperations)
	}
	signatureDiagnostics := 0
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_COLLECTION_SIGNATURE" {
			signatureDiagnostics++
		}
	}
	if signatureDiagnostics != 2 {
		t.Fatalf("diagnostics = %#v, want two collection method signature diagnostics", analysis.Diagnostics)
	}
}

func TestAnalyzeScriptReportsDeclarationSemanticDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Bad Declaration", overlay=true)
type TradeBox
    float price = close
    int price = 0
method reset(TradeBox box, float limit, int limit) =>
    box`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want declaration diagnostics")
	}
	declarationDiagnostics := 0
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_DECLARATION" {
			declarationDiagnostics++
		}
	}
	if declarationDiagnostics != 2 {
		t.Fatalf("diagnostics = %#v, want two declaration diagnostics", analysis.Diagnostics)
	}
	if analysis.Semantic == nil || len(analysis.Semantic.Declarations) != 2 {
		t.Fatalf("semantic declarations = %#v", analysis.Semantic)
	}
	if len(analysis.Semantic.Declarations[0].Fields) != 2 {
		t.Fatalf("type fields = %#v", analysis.Semantic.Declarations[0].Fields)
	}
}

func TestAnalyzeScriptReportsTypeAndMethodRegistryDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Declaration Registry", overlay=true)
type TradeBox
    float price
type TradeBox
    int bars
method missing() =>
    na
method untyped(box) =>
    box
method haunt(Ghost ghost) =>
    ghost
method reset(TradeBox box, float limit) =>
    box
method reset(TradeBox target, float threshold = 0) =>
    target
method reset(TradeBox box, float limit, int bars = 0) =>
    box
method put(map<string, float> values, string key, float value = close) =>
    values
box = TradeBox.new(close)
updated = box.reset(10, 1)`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want declaration registry diagnostics")
	}
	declarationDiagnostics := make([]Diagnostic, 0)
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_DECLARATION" {
			declarationDiagnostics = append(declarationDiagnostics, diagnostic)
		}
	}
	if len(declarationDiagnostics) != 5 {
		t.Fatalf("diagnostics = %#v, want five declaration registry diagnostics", analysis.Diagnostics)
	}
	messages := make([]string, 0, len(declarationDiagnostics))
	for _, diagnostic := range declarationDiagnostics {
		messages = append(messages, diagnostic.Message)
	}
	joined := strings.Join(messages, "\n")
	for _, fragment := range []string{
		"type TradeBox is already declared",
		"method missing requires a receiver parameter",
		"method untyped requires a typed receiver",
		"method haunt receiver type Ghost is not declared",
		"method reset is already declared for receiver TradeBox with this signature",
	} {
		if !strings.Contains(joined, fragment) {
			t.Fatalf("diagnostic messages = %q, missing %q", joined, fragment)
		}
	}
	var collectionMethod SemanticDeclaration
	for _, declaration := range analysis.Declarations {
		if declaration.Kind == "method" && declaration.Name == "put" {
			collectionMethod = declaration
		}
	}
	if collectionMethod.Receiver == nil || collectionMethod.Receiver.Type != "map<string, float>" || collectionMethod.Receiver.Name != "values" || len(collectionMethod.Parameters) != 3 {
		t.Fatalf("collection method declaration = %#v", collectionMethod)
	}
	if len(analysis.ObjectOperations) != 2 {
		t.Fatalf("object operations = %#v", analysis.ObjectOperations)
	}
	methodOperation := analysis.ObjectOperations[1]
	if methodOperation.Kind != "method" || methodOperation.Signature != "reset(TradeBox box, float limit, int bars = 0)" || len(methodOperation.Arguments) != 2 {
		t.Fatalf("overloaded method operation = %#v", methodOperation)
	}
}

func TestAnalyzeScriptReportsImportAliasDeclarationDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Bad Import Alias", overlay=true)
import TradingView/ta/7 as tools
import TradingView/math/1 as tools`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want declaration diagnostics")
	}
	if analysis.Semantic == nil || len(analysis.Semantic.Declarations) != 2 {
		t.Fatalf("semantic declarations = %#v", analysis.Semantic)
	}
	if analysis.Semantic.Declarations[0].ImportPath != "TradingView/ta/7" || analysis.Semantic.Declarations[0].Alias != "tools" || analysis.Semantic.Declarations[0].Version != "7" {
		t.Fatalf("first import declaration = %#v", analysis.Semantic.Declarations[0])
	}
	if analysis.Semantic.Declarations[1].ImportPath != "TradingView/math/1" || analysis.Semantic.Declarations[1].Alias != "tools" || analysis.Semantic.Declarations[1].Version != "1" {
		t.Fatalf("second import declaration = %#v", analysis.Semantic.Declarations[1])
	}
	aliasDiagnostics := 0
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_DECLARATION" {
			aliasDiagnostics++
		}
	}
	if aliasDiagnostics != 1 {
		t.Fatalf("diagnostics = %#v, want one import alias diagnostic", analysis.Diagnostics)
	}
}

func TestAnalyzeScriptReportsObjectOperationSignatureDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Bad Object", overlay=true)
type TradeBox
    float price
    int bars = 0
method reset(TradeBox box, float limit, int bars = 0) =>
    box
box = TradeBox.new()
tooWide = TradeBox.new(close, 0, 1)
resetTooFew = box.reset()
resetTooWide = box.reset(10, 1, 2)`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want object diagnostics")
	}
	if len(analysis.ObjectOperations) != 4 {
		t.Fatalf("object operations = %#v", analysis.ObjectOperations)
	}
	signatureDiagnostics := 0
	for _, diagnostic := range analysis.Diagnostics {
		if diagnostic.Code == "PINE_SEMANTIC_OBJECT_SIGNATURE" {
			signatureDiagnostics++
		}
	}
	if signatureDiagnostics != 4 {
		t.Fatalf("diagnostics = %#v, want four object signature diagnostics", analysis.Diagnostics)
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

func TestCompileSupportsPendingStopAndCancelOrders(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Pending", overlay=true)
strategy.entry("Breakout", strategy.long, stop=ta.highest(high, 20), qty=1)
strategy.order("Net short", strategy.short, stop=low - 1, qty=5)
strategy.entry("StopLimit", strategy.long, stop=101, limit=99, qty=2)
strategy.cancel("Breakout")
strategy.cancel_all()`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	statements := compilation.Program.Hooks[0].Statements
	if len(statements) != 5 {
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
	stopLimit, ok := statements[2].(*strategyir.OrderStmt)
	if !ok || stopLimit.StopExpression != "101" || stopLimit.LimitExpression != "99" || stopLimit.OrderType != "LIMIT" {
		t.Fatalf("stop-limit statement = %#v", statements[2])
	}
	cancel, ok := statements[3].(*strategyir.CancelStmt)
	if !ok || cancel.ID != "Breakout" || cancel.All {
		t.Fatalf("cancel statement = %#v", statements[3])
	}
	cancelAll, ok := statements[4].(*strategyir.CancelStmt)
	if !ok || !cancelAll.All {
		t.Fatalf("cancel_all statement = %#v", statements[4])
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

func TestCompatibilityScoreAndSupportedFeatureIDsAreRegistryDriven(t *testing.T) {
	assessment := CompatibilityScore()
	if assessment.ScoreModelVersion != "closed-bar-strategy-v3.0" {
		t.Fatalf("ScoreModelVersion = %q", assessment.ScoreModelVersion)
	}
	if assessment.Score < 98 || assessment.Score > 100 {
		t.Fatalf("Score = %v, want v3.0 closed-bar score", assessment.Score)
	}
	if len(assessment.Dimensions) != 5 {
		t.Fatalf("Dimensions = %#v, want 5 dimensions", assessment.Dimensions)
	}
	features := map[string]bool{}
	for _, id := range SupportedFeatureIDs() {
		features[id] = true
	}
	for _, id := range []string{"indicator.v13_migration_set", "indicator.v14_window_momentum_set", "indicator.v15_common_ta_set", "indicator.v16_mtf_tuple_bindings", "indicator.v17_source_aware_semantic_requirements", "indicator.v21_bbw_cog_anchored_vwap", "indicator.v24_mtf_stoch", "request.security.pure_expression", "request.security.v15_common_ta_expression", "request.security.v16_tuple_whitelist", "request.security.v17_semantic_tuple_corpus", "request.security.v21_ast_pure_expression", "request.security.v22_general_tuple", "request.security.v23_pure_collection_object_expression", "request.security.v24_mtf_stoch", "request.security.v27_pure_helper_expression", "request.security.v28_object_method_expression", "request.security.v29_object_history_expression", "syntax.v15_loop_control_subset", "syntax.v16_security_tuple_destructure", "syntax.v17_ast_semantic_transition", "syntax.v21_collection_runtime_core", "syntax.v22_structured_loop_runtime", "syntax.v22_pure_udt_method_runtime", "syntax.v23_collection_api_expansion", "syntax.v23_pure_method_body_named_args", "syntax.v24_collection_api_expansion", "syntax.v24_runtime_loop_fallback", "syntax.v24_persistent_object_field_set", "syntax.v25_array_stat_api", "syntax.v26_collection_iteration", "syntax.v26_collection_history_snapshot", "syntax.v26_object_collection_fields", "syntax.v26_library_export_metadata", "syntax.v27_collection_history_aggregates", "syntax.v27_map_matrix_iteration", "syntax.v28_object_history_read", "syntax.v28_method_chain", "syntax.v28_export_metadata", "syntax.v29_object_history_method_receiver", "syntax.v29_method_chain_named_defaults", "syntax.v29_request_security_diagnostics", "syntax.v30_stable_semantic_declarations", "syntax.v30_varip_closed_bar_policy", "syntax.v30_parser_whitespace_comments", "syntax.arrays_maps_matrices", "syntax.methods_types_libraries", "syntax.dynamic_loops_while", "expression.v22_general_tuple", "expression.v23_object_field_set", "expression.v25_string_helpers", "expression.v25_timeframe_change", "expression.v27_timeframe_helpers", "tooling.visual_metadata_output", "tooling.v20_language_foundation", "tooling.migration_corpus_v21", "tooling.migration_corpus_v22", "tooling.migration_corpus_v23", "tooling.migration_corpus_v24", "tooling.migration_corpus_v25", "tooling.migration_corpus_v26", "tooling.migration_corpus_v27", "tooling.migration_corpus_v28", "tooling.migration_corpus_v29", "tooling.migration_corpus_v30", "order.entry_reversal", "order.allow_entry_in"} {
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
