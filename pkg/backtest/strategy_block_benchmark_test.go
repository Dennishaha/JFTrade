package backtest

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

var benchmarkStrategyBlockResult *RunResult

type strategyBlockBenchmarkCase struct {
	name   string
	script string
}

// Keep this matrix aligned with the supported visual blocks in
// apps/web/src/features/strategyVisualBuilderCatalog.ts and the built-in
// templates in apps/web/src/features/strategyVisualBuilderTemplates.ts.
// Deprecated codeBlock is intentionally omitted because the current Pine
// generator lowers it to a plain log statement and it is no longer a primary
// authoring path.
func strategyBlockBenchmarkCases() []strategyBlockBenchmarkCase {
	return []strategyBlockBenchmarkCase{
		{
			name: "moving_average_windowed",
			script: `//@version=6
strategy("Pine Block Matrix Moving Average", overlay=true)
log.info("ma matrix")
fast = ta.sma(close, 2)
slow = ta.ema(close, 4)
if ta.crossover(fast, slow)
    strategy.entry("Long", strategy.long, qty=1)
if ta.crossunder(fast, slow)
    strategy.close("Long")`,
		},
		{
			name: "rsi_reversion",
			script: `//@version=6
strategy("Pine Block Matrix RSI", overlay=true)
log.info("rsi matrix")
momentum = ta.rsi(close, 14)
if close < 99
    strategy.entry("Long", strategy.long, qty=1)
if close > 101
    strategy.close("Long")`,
		},
		{
			name: "macd_momentum",
			script: `//@version=6
strategy("Pine Block Matrix MACD", overlay=true)
log.info("macd matrix")
[macdLine, signalLine, histLine] = ta.macd(close, 12, 26, 9)
if close > 101
    strategy.entry("Long", strategy.long, qty=1)
if close < 99
    strategy.close("Long")`,
		},
		{
			name: "kdj_reversion",
			script: `//@version=6
strategy("Pine Block Matrix KDJ", overlay=true)
log.info("kdj matrix")
momentum = ta.rsi(close, 9)
if close < 99
    strategy.entry("Long", strategy.long, qty=1)
if close > 101
    strategy.close("Long")`,
		},
		{
			name: "bollinger_reversion",
			script: `//@version=6
strategy("Pine Block Matrix Bollinger", overlay=true)
log.info("bollinger matrix")
middle = ta.sma(close, 20)
if close < 99
    strategy.entry("Long", strategy.long, qty=1)
if close > 101
    strategy.close("Long")`,
		},
		{
			name: "atr_volatility",
			script: `//@version=6
strategy("Pine Block Matrix ATR", overlay=true)
log.info("atr matrix")
range = ta.atr(14)
if close > 101
    strategy.entry("Long", strategy.long, qty=1)
if close < 99
    strategy.close("Long")`,
		},
		{
			name: "cci_reversion",
			script: `//@version=6
strategy("Pine Block Matrix CCI", overlay=true)
log.info("cci matrix")
channel = ta.cci(close, 20)
if close < 99
    strategy.entry("Long", strategy.long, qty=1)
if close > 101
    strategy.close("Long")`,
		},
		{
			name: "williamsr_reversion",
			script: `//@version=6
strategy("Pine Block Matrix WilliamsR", overlay=true)
log.info("williamsr matrix")
momentum = ta.rsi(close, 14)
if close < 99
    strategy.entry("Long", strategy.long, qty=1)
if close > 101
    strategy.close("Long")`,
		},
		{
			name: "breakout_notify",
			script: `//@version=6
strategy("Pine Block Matrix Breakout", overlay=true)
alert("breakout matrix init")
if close > 101
    alert("breakout hit")
    strategy.entry("Long", strategy.long, qty=(strategy.equity * 2 / 100) / close)
else
    log.info("breakout idle")`,
		},
		{
			name: "mean_reversion_price",
			script: `//@version=6
strategy("Pine Block Matrix Mean Reversion", overlay=true)
log.info("mean reversion matrix")
if close < 99
    log.info("dip detected")
    strategy.entry("Long", strategy.long, qty=1)
else
    alert("mean reversion idle")`,
		},
		{
			name: "protect_session_risk",
			script: `//@version=6
strategy("Pine Block Matrix Protect", overlay=true)
log.info("protect matrix")
if close > 100.5
    strategy.entry("Long", strategy.long, qty=1)
alert("protect evaluated")`,
		},
	}
}

func TestStrategyBlockBenchmarkCasesSmoke(t *testing.T) {
	isolateBacktestHome(t)
	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()

	ctx := context.Background()
	for _, benchmarkCase := range strategyBlockBenchmarkCases() {
		t.Run(benchmarkCase.name, func(t *testing.T) {
			result := Run(ctx, strategyBlockBenchmarkRunConfig(dbPath, startTime, endTime, benchmarkCase.script))
			if result == nil {
				t.Fatal("expected run result")
			}
			if result.Error != "" {
				t.Fatalf("Run() error = %s", result.Error)
			}
			if len(result.RuntimeErrors) != 0 {
				t.Fatalf("RuntimeErrors = %#v, want empty", result.RuntimeErrors)
			}
			if len(result.Candles) == 0 {
				t.Fatal("expected replayed candles")
			}
		})
	}
}

func BenchmarkRunExecutesStrategyBlockMatrix(b *testing.B) {
	isolateBacktestHome(b)
	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(b)
	restoreLogs := suppressBacktestRunLogs(b)
	defer restoreLogs()

	ctx := context.Background()
	for _, benchmarkCase := range strategyBlockBenchmarkCases() {
		b.Run(benchmarkCase.name, func(b *testing.B) {
			cfg := strategyBlockBenchmarkRunConfig(dbPath, startTime, endTime, benchmarkCase.script)
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				benchmarkStrategyBlockResult = Run(ctx, cfg)
				if benchmarkStrategyBlockResult == nil {
					b.Fatal("expected run result")
				}
				if benchmarkStrategyBlockResult.Error != "" {
					b.Fatalf("Run() error = %s", benchmarkStrategyBlockResult.Error)
				}
			}
		})
	}
}

func strategyBlockBenchmarkRunConfig(dbPath string, startTime, endTime time.Time, script string) RunConfig {
	return RunConfig{
		DBPath:           dbPath,
		Symbol:           "US.AAPL",
		Interval:         "1m",
		SourceFormat:     strategydefinition.SourceFormatPineV6,
		StartTime:        startTime,
		EndTime:          endTime,
		StrategyScript:   script,
		InitialBalance:   10000,
		WarmupCandles:    256,
		UseExtendedHours: new(true),
	}
}

func seedStrategyBlockBenchmarkStore(tb testing.TB) (string, time.Time, time.Time) {
	tb.Helper()
	dbPath := filepath.Join(tb.TempDir(), "strategy-block-benchmark.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		tb.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	klines := buildBenchmarkKLines(baseStart, 2048)
	if err := store.InsertKLines(klines, "forward"); err != nil {
		jftradeErr1 := store.Close()
		jftradeCheckTestError(tb, jftradeErr1)
		tb.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		tb.Fatalf("store.Close() error = %v", err)
	}
	startIndex := 512
	return dbPath, klines[startIndex].StartTime.Time(), klines[len(klines)-1].EndTime.Time()
}

func suppressBacktestRunLogs(tb testing.TB) func() {
	tb.Helper()
	previousWriter := log.Writer()
	previousLogrusWriter := logrus.StandardLogger().Out
	log.SetOutput(io.Discard)
	logrus.SetOutput(io.Discard)
	return func() {
		log.SetOutput(previousWriter)
		logrus.SetOutput(previousLogrusWriter)
	}
}
