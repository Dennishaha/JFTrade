package pine

import (
	"testing"
)

var benchmarkCompilationSink Compilation
var benchmarkErrorSink error
var benchmarkLinesSink []parsedLine
var benchmarkASTSink *AST

type pineBenchmarkCase struct {
	name   string
	script string
}

func pineBenchmarkCases() []pineBenchmarkCase {
	return []pineBenchmarkCase{
		{
			name: "minimal",
			script: `//@version=6
strategy("Minimal", overlay=true)
log.info("ready")`,
		},
		{
			name: "indicator_heavy",
			script: `//@version=6
strategy("Indicators", overlay=true)
fast = ta.ema(close, 8)
slow = ta.sma(close, 21)
rsi = ta.rsi(close, 14)
cci = ta.cci(hlc3, 20)
[basis, upper, lower] = ta.bb(close, 20, 2)
if ta.crossover(fast, slow) and rsi < 35 and cci < -100 and close < lower
    strategy.entry("Long", strategy.long, qty_percent=10)`,
		},
		{
			name: "mtf",
			script: `//@version=6
strategy("MTF", overlay=true)
tf = input.timeframe("15", "Signal TF")
mtfClose = request.security(syminfo.tickerid, tf, close)
mtfPrev = request.security(syminfo.tickerid, tf, close[1])
mtfEma = request.security(syminfo.tickerid, "15", ta.ema(hlc3, 3))
if mtfClose > mtfPrev and close > mtfEma
    strategy.entry("Long", strategy.long, qty=1)`,
		},
		{
			name: "native_public_indicators",
			script: `//@version=6
strategy("Native Pine Indicators", overlay=true)
fast = ta.ema(close, 14)
slow = ta.sma(close, 28)
relative = ta.rsi(close, 14)
money = ta.mfi(hlc3, 14)
band = ta.bb(close, 20, 2)
width = ta.bbw(close, 20, 2)
[macdLine, signalLine, histogram] = ta.macd(close, 12, 26, 9)
[diPlus, diMinus, adx] = ta.dmi(14, 14)
[trendValue, trendDirection] = ta.supertrend(3, 10)
[kcBasis, kcUpper, kcLower] = ta.kc(close, 20, 1.5)
dailyFast = request.security(syminfo.tickerid, "D", ta.ema(close, 20))
hourlyBand = request.security(syminfo.tickerid, "60", ta.bb(close, 20, 2))
if ta.crossover(fast, slow) and close < band.upper and close > kcLower and dailyFast > hourlyBand.lower and histogram > 0 and adx > 20 and relative < 65 and money > 40 and width > 0 and trendDirection > 0
    strategy.entry("Long", strategy.long, qty_percent=10)`,
		},
		{
			name: "udf_static_for",
			script: `//@version=6
strategy("UDF", overlay=true)
isBull(src) => src > src[1]
sum = 0
for i = 0 to 20
    sum := sum + nz(close[i], close)
if isBull(close) and sum > 0
    strategy.entry("Long", strategy.long, qty=1)`,
		},
		{
			name: "orders",
			script: `//@version=6
strategy("Orders", overlay=true, pyramiding=2)
if close > open
    strategy.entry("Long", strategy.long, qty_percent=10)
    strategy.order("Net", strategy.long, qty=1)
    strategy.exit("Bracket", "Long", stop=close * 0.98, limit=close * 1.04, qty_percent=50)
else
    strategy.entry("Breakout", strategy.long, stop=high + 1, qty=1)
    strategy.cancel("Breakout")`,
		},
	}
}

func BenchmarkPublicHelperGuard(b *testing.B) {
	cases := []struct {
		name    string
		line    string
		wantErr bool
	}{
		{
			name: "native_pine_entry",
			line: `signal = request.security(syminfo.tickerid, "D", ta.bb(close, 20, 2).upper) + ta.ema(close, 14) + ta.wpr(14)`,
		},
		{
			name:    "disabled_helper",
			line:    `signal = security_source(close, day) + ma(EMA, 14) + bollinger(20, 2).upper`,
			wantErr: true,
		},
		{
			name: "string_literals",
			line: `log.info("ma(EMA, 14) security_source(close, day) bollinger(20, 2)")`,
		},
	}
	for _, benchmarkCase := range cases {
		b.Run(benchmarkCase.name, func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				benchmarkErrorSink = rejectPublicDisabledHelperCalls(3, benchmarkCase.line)
				if benchmarkCase.wantErr && benchmarkErrorSink == nil {
					b.Fatal("expected helper diagnostic")
				}
				if !benchmarkCase.wantErr && benchmarkErrorSink != nil {
					b.Fatal(benchmarkErrorSink)
				}
			}
		})
	}
}

func BenchmarkPineTokenize(b *testing.B) {
	for _, benchmarkCase := range pineBenchmarkCases() {
		b.Run(benchmarkCase.name, func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				benchmarkLinesSink = tokenizeScript(benchmarkCase.script)
			}
		})
	}
}

func BenchmarkPineParseAST(b *testing.B) {
	for _, benchmarkCase := range pineBenchmarkCases() {
		lines := tokenizeScript(benchmarkCase.script)
		b.Run(benchmarkCase.name, func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				benchmarkASTSink, _ = parseAST(lines)
			}
		})
	}
}

func BenchmarkPineLowering(b *testing.B) {
	for _, benchmarkCase := range pineBenchmarkCases() {
		lines := tokenizeScript(benchmarkCase.script)
		ast, diagnostics := parseAST(lines)
		if err := diagnosticError(diagnostics); err != nil {
			b.Fatalf("%s AST: %v", benchmarkCase.name, err)
		}
		b.Run(benchmarkCase.name, func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				compilation, err := compileLoweredAST(benchmarkCase.script, lines, ast)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkCompilationSink = compilation
			}
		})
	}
}
