package pine_test

import (
	"testing"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

var benchmarkPublicAnalysisSink strategypine.AnalysisResult
var benchmarkPublicRequirementsSink strategyir.Requirements

func publicPineBenchmarkCases() map[string]string {
	return map[string]string{
		"minimal": `//@version=6
strategy("Minimal", overlay=true)
log.info("ready")`,
		"golden": `//@version=6
strategy("Golden", overlay=true, pyramiding=2)
len = input.int(3, "Length")
isBull(src) => src > src[1]
fast = ta.ema(close, len)
avgVol = ta.sma(volume, 2)
sum = 0
for i = 0 to 2
    sum := sum + nz(close[i], close)
if isBull(close) and close > fast and volume > avgVol and sum > 0
    strategy.entry("Long", strategy.long, qty_percent=10)
    strategy.exit("Bracket", "Long", stop=close * 0.98, limit=close * 1.04, qty_percent=50)`,
		"native_indicators": `//@version=6
strategy("Native Indicators", overlay=true)
fast = ta.ema(close, 14)
slow = ta.sma(close, 28)
relative = ta.rsi(close, 14)
band = ta.bb(close, 20, 2)
[macdLine, signalLine, histogram] = ta.macd(close, 12, 26, 9)
[diPlus, diMinus, adx] = ta.dmi(14, 14)
[trendValue, trendDirection] = ta.supertrend(3, 10)
dailyFast = request.security(syminfo.tickerid, "D", ta.ema(close, 20))
hourlyBand = request.security(syminfo.tickerid, "60", ta.bb(close, 20, 2))
if ta.crossover(fast, slow) and close < band.upper and dailyFast > hourlyBand.lower and histogram > 0 and adx > 20 and relative < 65 and trendDirection > 0
    strategy.entry("Long", strategy.long, qty_percent=10)`,
	}
}

func BenchmarkPineAnalyzeScript(b *testing.B) {
	for name, script := range publicPineBenchmarkCases() {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				benchmarkPublicAnalysisSink = strategypine.AnalyzeScript(script, strategypine.AnalysisOptions{})
				if !benchmarkPublicAnalysisSink.OK {
					b.Fatalf("diagnostics: %#v", benchmarkPublicAnalysisSink.Diagnostics)
				}
			}
		})
	}
}

func BenchmarkPinePlanRequirements(b *testing.B) {
	for name, script := range publicPineBenchmarkCases() {
		compilation, err := strategypine.Compile(script)
		if err != nil {
			b.Fatalf("%s compile: %v", name, err)
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				requirements, err := strategyir.PlanRequirements(compilation.Program)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkPublicRequirementsSink = requirements
			}
		})
	}
}
