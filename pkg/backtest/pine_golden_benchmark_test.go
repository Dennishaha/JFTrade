package backtest

import (
	"context"
	"testing"

	"github.com/jftrade/jftrade-main/pkg/strategy/pinespec"
)

var benchmarkPineGoldenResult *RunResult

func TestPineGoldenBenchmarkCasesSmoke(t *testing.T) {
	isolateBacktestHome(t)
	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()

	for _, example := range pinespec.GoldenExamples() {
		t.Run(example.ID, func(t *testing.T) {
			cfg := strategyBlockBenchmarkRunConfig(dbPath, startTime, endTime, example.Script)
			cfg.InitialBalance = 1_000_000_000
			result := Run(context.Background(), cfg)
			if result == nil {
				t.Fatal("expected run result")
			}
			if result.Error != "" {
				t.Fatalf("Run() error = %s", result.Error)
			}
		})
	}
}

func BenchmarkRunExecutesPineGoldenMatrix(b *testing.B) {
	isolateBacktestHome(b)
	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(b)
	restoreLogs := suppressBacktestRunLogs(b)
	defer restoreLogs()

	for _, example := range pinespec.GoldenExamples() {
		b.Run(example.ID, func(b *testing.B) {
			cfg := strategyBlockBenchmarkRunConfig(dbPath, startTime, endTime, example.Script)
			cfg.InitialBalance = 1_000_000_000
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				benchmarkPineGoldenResult = Run(context.Background(), cfg)
				if benchmarkPineGoldenResult == nil {
					b.Fatal("expected run result")
				}
				if benchmarkPineGoldenResult.Error != "" {
					b.Fatalf("Run() error = %s", benchmarkPineGoldenResult.Error)
				}
			}
		})
	}
}
