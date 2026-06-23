package backtest

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

var benchmarkDrawdownMax float64
var benchmarkDrawdownCurrent float64
var benchmarkDrawdownCurve []DrawdownPoint
var benchmarkDrawdownResult *RunResult

func BenchmarkBuildDrawdownMetrics(b *testing.B) {
	pnlCurve := buildBenchmarkPnLCurve(16384)
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		benchmarkDrawdownMax, benchmarkDrawdownCurrent, benchmarkDrawdownCurve = buildDrawdownMetrics(pnlCurve)
		if len(benchmarkDrawdownCurve) != len(pnlCurve) {
			b.Fatalf("drawdown curve len = %d, want %d", len(benchmarkDrawdownCurve), len(pnlCurve))
		}
	}
}

func BenchmarkResultCollectorFinalizeWithDrawdown(b *testing.B) {
	pnlCurve := buildBenchmarkPnLCurve(16384)
	candles := buildBenchmarkDrawdownCandles(pnlCurve)
	lastEquity := pnlCurve[len(pnlCurve)-1].Equity
	account := types.NewAccount()
	account.SetBalance("USD", types.Balance{Currency: "USD", Available: fixedpoint.NewFromFloat(lastEquity)})
	querier := stubAccountQuerier{account: account}
	ctx := context.Background()
	initialBalance := pnlCurve[0].Equity

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		result := &RunResult{}
		collector := &resultCollector{
			quoteCurrency: "USD",
			result:        result,
			pnlCurve:      pnlCurve,
			candles:       candles,
		}
		totalOrders, filledOrders := collector.finalize(ctx, querier, initialBalance)
		if totalOrders != 0 || filledOrders != 0 {
			b.Fatalf("finalize orders = %d/%d, want 0/0", totalOrders, filledOrders)
		}
		if len(result.DrawdownCurve) != len(pnlCurve) {
			b.Fatalf("drawdown curve len = %d, want %d", len(result.DrawdownCurve), len(pnlCurve))
		}
		benchmarkDrawdownResult = result
	}
}

func buildBenchmarkPnLCurve(count int) []PnLPoint {
	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	pnlCurve := make([]PnLPoint, count)
	equity := 100000.0
	for index := range count {
		if index%37 == 0 {
			equity += 95
		} else if index%13 == 0 {
			equity -= 140
		} else {
			equity += 12 - float64(index%5)
		}
		if equity < 1000 {
			equity = 1000
		}
		pnlCurve[index] = PnLPoint{
			Time:   baseStart.Add(time.Duration(index) * time.Minute).Format(time.RFC3339),
			Equity: equity,
		}
	}
	return pnlCurve
}

func buildBenchmarkDrawdownCandles(pnlCurve []PnLPoint) []Candle {
	candles := make([]Candle, len(pnlCurve))
	for index := range pnlCurve {
		equity := strconv.FormatFloat(pnlCurve[index].Equity, 'f', -1, 64)
		candles[index] = Candle{
			Time:   pnlCurve[index].Time,
			Open:   equity,
			High:   equity,
			Low:    equity,
			Close:  equity,
			Volume: "0",
		}
	}
	return candles
}
