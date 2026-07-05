package pine

import (
	"strings"
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
)

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
