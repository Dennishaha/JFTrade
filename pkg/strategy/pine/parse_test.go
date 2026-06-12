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

func TestValidateScriptRejectsUnsupportedPineRuntimeFeature(t *testing.T) {
	err := ValidateScript(`//@version=6
strategy("MTF", overlay=true)
x = request.security("NASDAQ:AAPL", "D", close)`)
	if err == nil || !strings.Contains(err.Error(), "request.security") {
		t.Fatalf("ValidateScript() error = %v, want request.security diagnostic", err)
	}
}

func TestCompileSupportsMovingAverageRequestSecuritySubset(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("MTF MA", overlay=true)
fast = request.security(syminfo.tickerid, "D", ta.ema(close, 5))
slow = request.security(syminfo.tickerid, "60", ta.sma(close, 20))`)
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
	if !ok || stmt.Expression != "2.0 * stdev(20)" {
		t.Fatalf("first statement = %#v", compilation.Program.Hooks[0].Statements[0])
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

func TestValidateScriptRejectsQtyPercent(t *testing.T) {
	err := ValidateScript(`//@version=6
strategy("Sizing", overlay=true)
strategy.entry("Long", strategy.long, qty_percent=25)`)
	if err == nil || !strings.Contains(err.Error(), "qty_percent") {
		t.Fatalf("ValidateScript() error = %v, want qty_percent diagnostic", err)
	}
}

func TestCompileIgnoresVisualCallsWithWarning(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Visual", overlay=true)
plot(close)
strategy.entry("Long", strategy.long, qty=(strategy.equity * 25 / 100) / close, limit=101)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compilation.Warnings) != 1 || !strings.Contains(compilation.Warnings[0], "plot") {
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

func TestCompileSupportsStrategyExitSubset(t *testing.T) {
	compilation, err := Compile(`//@version=6
strategy("Exit", overlay=true)
strategy.exit("Long stop", "Long", stop=close * (1 - 2 / 100))
strategy.exit("Short profit", "Short", limit=close * (1 - 3 / 100))
strategy.exit("Long trail", "Long", trail_points=close * 4 / 100, trail_offset=close * 4 / 100)`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(compilation.Program.Hooks[0].Statements) != 3 {
		t.Fatalf("statement count = %d", len(compilation.Program.Hooks[0].Statements))
	}
	stop, ok := compilation.Program.Hooks[0].Statements[0].(*strategyir.ProtectStmt)
	if !ok || stop.Mode != "stopLoss" || stop.Direction != "long" || stop.PercentageExpression != "2" || stop.TimeUnit != "bar" {
		t.Fatalf("stop statement = %#v", compilation.Program.Hooks[0].Statements[0])
	}
	profit, ok := compilation.Program.Hooks[0].Statements[1].(*strategyir.ProtectStmt)
	if !ok || profit.Mode != "takeProfit" || profit.Direction != "short" || profit.PercentageExpression != "3" {
		t.Fatalf("profit statement = %#v", compilation.Program.Hooks[0].Statements[1])
	}
	trailing, ok := compilation.Program.Hooks[0].Statements[2].(*strategyir.ProtectStmt)
	if !ok || trailing.Mode != "trailingStop" || trailing.Direction != "long" || trailing.PercentageExpression != "4" {
		t.Fatalf("trailing statement = %#v", compilation.Program.Hooks[0].Statements[2])
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
	if got := statements[2].(*strategyir.LetStmt).Expression; got != "ifelse(previous(close) == na, 0, nz(previous(close), close))" {
		t.Fatalf("signal expression = %q", got)
	}
	ifStmt := statements[3].(*strategyir.IfStmt)
	if ifStmt.Condition != "close > previous(close)" {
		t.Fatalf("condition = %q", ifStmt.Condition)
	}
}

func TestAnalyzeScriptReturnsStructuredUnsupportedDiagnostics(t *testing.T) {
	analysis := AnalyzeScript(`//@version=6
strategy("Loop", overlay=true)
for i = 0 to 10
    log.info("nope")`, AnalysisOptions{IncludeAST: true})
	if analysis.OK {
		t.Fatal("AnalyzeScript().OK = true, want false")
	}
	if len(analysis.Diagnostics) == 0 || analysis.Diagnostics[0].Code != "PINE_FOR_UNSUPPORTED" {
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
for i = 0 to 10
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
