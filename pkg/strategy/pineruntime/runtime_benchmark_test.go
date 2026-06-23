package pineruntime

import (
	"context"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	strategyir "github.com/jftrade/jftrade-main/pkg/strategy/ir"
	strategypine "github.com/jftrade/jftrade-main/pkg/strategy/pine"
)

var benchmarkRuntimeBarIndexSink int

type runtimeBenchmarkCase struct {
	name     string
	script   string
	barCount int
}

func pineRuntimeBenchmarkCases() []runtimeBenchmarkCase {
	return []runtimeBenchmarkCase{
		{
			name: "price_expression_2000",
			script: `//@version=6
strategy("Price Runtime", overlay=true)
spread = high - low
body = close - open
signal = spread > 0 and body != 0`,
			barCount: 2000,
		},
		{
			name: "indicator_history_2000",
			script: `//@version=6
strategy("Indicator Runtime", overlay=true)
fast = ta.ema(close, 8)
slow = ta.sma(volume, 21)
signal = fast > fast[1] and volume > slow and close > close[20]`,
			barCount: 2000,
		},
		{
			name: "stateful_10000",
			script: `//@version=6
strategy("Stateful Runtime", overlay=true)
up = close > close[1]
bars = ta.barssince(up)
last = ta.valuewhen(up, close, 0)
signal = bars >= 0 and close >= nz(last, close)`,
			barCount: 10000,
		},
	}
}

func BenchmarkPineRuntimePushKLines(b *testing.B) {
	for _, benchmarkCase := range pineRuntimeBenchmarkCases() {
		compilation, err := strategypine.Compile(benchmarkCase.script)
		if err != nil {
			b.Fatalf("%s compile: %v", benchmarkCase.name, err)
		}
		requirements, err := strategyir.PlanRequirements(compilation.Program)
		if err != nil {
			b.Fatalf("%s plan: %v", benchmarkCase.name, err)
		}
		b.Run(benchmarkCase.name, func(b *testing.B) {
			b.ReportAllocs()
			for iteration := 0; iteration < b.N; iteration++ {
				runtime, err := newStrategyRuntime(context.Background(), &Strategy{
					Name:     benchmarkCase.name,
					Symbol:   "US.AAPL",
					Interval: types.Interval1m,
					Script:   benchmarkCase.script,
				}, compilation.Program, requirements, nil, nil)
				if err != nil {
					b.Fatal(err)
				}
				pushBenchmarkKLines(runtime, benchmarkCase.barCount)
				benchmarkRuntimeBarIndexSink = runtime.barIndex
			}
		})
	}
}

func pushBenchmarkKLines(runtime *strategyRuntime, count int) {
	start := time.Date(2026, time.January, 5, 14, 30, 0, 0, time.UTC)
	for index := range count {
		price := 100 + float64(index%37)*0.05
		barStart := start.Add(time.Duration(index) * time.Minute)
		runtime.handleKLineClosed(types.KLine{
			Symbol:    "US.AAPL",
			Interval:  types.Interval1m,
			StartTime: types.Time(barStart),
			EndTime:   types.Time(barStart.Add(time.Minute - time.Millisecond)),
			Open:      fixedpoint.NewFromFloat(price - 0.1),
			High:      fixedpoint.NewFromFloat(price + 0.4),
			Low:       fixedpoint.NewFromFloat(price - 0.5),
			Close:     fixedpoint.NewFromFloat(price),
			Volume:    fixedpoint.NewFromFloat(1000 + float64(index%19)*25),
		})
	}
}
