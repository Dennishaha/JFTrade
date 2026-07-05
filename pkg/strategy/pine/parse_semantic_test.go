package pine

import (
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

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

func TestAnalyzeScriptReportsSupportedTASemanticSignatures(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("TA Semantic", overlay=true)
hh = ta.highest(high, 20)
lb = ta.lowestbars(low, 5)
delta = ta.change(close)
rank = ta.percentrank(close, 14)
width = ta.kcw(close, 20, 1.5)
almaValue = ta.alma(close, 9, 0.85, 6)
cciValue = ta.cci(hlc3, 20)
if ta.crossover(close, open) and ta.barssince(close > open) > 2
    strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript() diagnostics = %#v", analysis.Diagnostics)
	}
	if analysis.Semantic == nil {
		t.Fatal("Semantic is nil")
	}
	calls := map[string]SemanticFunctionCall{}
	for _, call := range analysis.Semantic.FunctionCalls {
		calls[call.Name] = call
	}
	for _, name := range []string{
		"ta.highest",
		"ta.lowestbars",
		"ta.change",
		"ta.percentrank",
		"ta.kcw",
		"ta.alma",
		"ta.cci",
		"ta.crossover",
		"ta.barssince",
	} {
		call, ok := calls[name]
		if !ok {
			t.Fatalf("semantic call %s missing from %#v", name, analysis.Semantic.FunctionCalls)
		}
		if !call.Supported || call.Signature == "" {
			t.Fatalf("semantic call %s = %#v, want supported signature", name, call)
		}
	}
}

func TestAnalyzeScriptReportsSupportedUtilitySemanticSignatures(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Utility Semantic", overlay=true)
plain = input(5, "Plain")
lengthInput = input.int(14, "Length")
factor = input.float(2.0, "Factor")
enabled = input.bool(true, "Enabled")
label = input.string("alpha", "Label")
src = input.source(close, "Source")
startTime = input.time(timestamp(2026, 1, 1), "Start")
tf = input.timeframe("15", "TF")
lineColor = input.color(color.green, "Line")
average = math.avg(close, open)
rounded = math.round_to_mintick(average)
wide = math.max(close, open)
narrow = math.min(close, open)
root = math.sqrt(4)
distance = math.abs(wide - narrow)
roundedDistance = math.round(distance)
ceiling = math.ceil(root)
score = math.sign(wide - narrow) + math.floor(root) + ceiling + math.pow(2, 3) + math.log(10)
upper = str.upper(label)
lower = str.lower(label)
length = str.length(label)
asText = str.tostring(length)
position = str.pos(upper, "A")
part = str.substring(upper, 0, 2)
clean = str.replace(part, "A", lower)
text = str.format("{0}:{1}", clean, length)
if enabled and str.contains(text, "a") and rounded > 0 and roundedDistance >= 0 and position >= 0 and score > 0
    strategy.entry("Long", strategy.long, qty=1)`, AnalysisOptions{IncludeAST: true, IncludeSemantic: true})
	if !analysis.OK {
		t.Fatalf("AnalyzeScript() diagnostics = %#v", analysis.Diagnostics)
	}
	if analysis.Semantic == nil {
		t.Fatal("Semantic is nil")
	}
	calls := map[string]SemanticFunctionCall{}
	for _, call := range analysis.Semantic.FunctionCalls {
		calls[call.Name] = call
	}
	for _, name := range []string{
		"input.int",
		"input.float",
		"input.bool",
		"input.string",
		"input.source",
		"input.time",
		"input.timeframe",
		"input.color",
		"math.round_to_mintick",
		"math.avg",
		"math.sign",
		"math.max",
		"math.min",
		"math.floor",
		"math.ceil",
		"math.abs",
		"math.round",
		"math.sqrt",
		"math.pow",
		"math.log",
		"str.format",
		"str.upper",
		"str.lower",
		"str.length",
		"str.tostring",
		"str.pos",
		"str.substring",
		"str.replace",
		"str.contains",
	} {
		call, ok := calls[name]
		if !ok {
			t.Fatalf("semantic call %s missing from %#v", name, analysis.Semantic.FunctionCalls)
		}
		if !call.Supported || call.Signature == "" {
			t.Fatalf("semantic call %s = %#v, want supported signature", name, call)
		}
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
