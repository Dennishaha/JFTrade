package backtest

import (
	"context"
	"io"
	"log"
	"path/filepath"
	"testing"
	"time"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/sirupsen/logrus"
)

var benchmarkStrategyBlockResult *RunResult

type strategyBlockBenchmarkCase struct {
	name   string
	script string
}

// Keep this matrix aligned with the supported visual blocks in
// apps/web/src/features/strategyVisualBuilderCatalog.ts and the built-in
// templates in apps/web/src/features/strategyVisualBuilderTemplates.ts.
// Deprecated codeBlock is intentionally omitted because the current DSL
// generator lowers it to a plain log statement and it is no longer a primary
// authoring path.
func strategyBlockBenchmarkCases() []strategyBlockBenchmarkCase {
	return []strategyBlockBenchmarkCase{
		{
			name: "moving_average_windowed",
			script: `strategy DSL Block Matrix Moving Average
version 1
symbol US.AAPL
interval 1m

on init:
  log "ma matrix"

on kline_close:
  let fast = ma(MA, 2, hour)
  let slow = ma(EMA, 4, hour)
  if cross_over(fast, slow):
    buy shares 1 policy same_direction type MARKET
  if cross_under(fast, slow):
    sell shares 1 policy allow type MARKET`,
		},
		{
			name: "rsi_reversion",
			script: `strategy DSL Block Matrix RSI
version 1
symbol US.AAPL
interval 1m

on init:
  log "rsi matrix"

on kline_close:
  let momentum = rsi(14)
  if momentum and momentum < 45:
    buy shares 1 policy same_direction type MARKET
  if momentum and momentum > 55:
    sell shares 1 policy allow type MARKET`,
		},
		{
			name: "macd_momentum",
			script: `strategy DSL Block Matrix MACD
version 1
symbol US.AAPL
interval 1m

on init:
  log "macd matrix"

on kline_close:
  let flow = macd(12, 26, 9)
  if flow and cross_over(flow.diff, flow.signal) and not divergence_top(flow, 6):
    buy shares 1 policy same_direction type MARKET
  if flow and (cross_under(flow.diff, flow.signal) or divergence_top(flow, 6)):
    sell shares 1 policy allow type MARKET`,
		},
		{
			name: "kdj_reversion",
			script: `strategy DSL Block Matrix KDJ
version 1
symbol US.AAPL
interval 1m

on init:
  log "kdj matrix"

on kline_close:
  let swing = kdj(9, 3, 3)
  if swing and (cross_over(swing.k, swing.d) or swing.j < 35):
    buy shares 1 policy same_direction type MARKET
  if swing and (cross_under(swing.k, swing.d) or swing.j > 65):
    sell shares 1 policy allow type MARKET`,
		},
		{
			name: "bollinger_reversion",
			script: `strategy DSL Block Matrix Bollinger
version 1
symbol US.AAPL
interval 1m

on init:
  log "bollinger matrix"

on kline_close:
  let band = bollinger(20, 1.5)
  if band and close < band.lower:
    buy shares 1 policy same_direction type MARKET
  if band and close > band.upper:
    sell shares 1 policy allow type MARKET`,
		},
		{
			name: "atr_volatility",
			script: `strategy DSL Block Matrix ATR
version 1
symbol US.AAPL
interval 1m

on init:
  log "atr matrix"

on kline_close:
  let range = atr(14)
  if range and range > 1.1:
    buy shares 1 policy same_direction type MARKET
  if range and range < 0.95:
    sell shares 1 policy allow type MARKET`,
		},
		{
			name: "cci_reversion",
			script: `strategy DSL Block Matrix CCI
version 1
symbol US.AAPL
interval 1m

on init:
  log "cci matrix"

on kline_close:
  let channel = cci(20)
  if channel and channel < -50:
    buy shares 1 policy same_direction type MARKET
  if channel and channel > 50:
    sell shares 1 policy allow type MARKET`,
		},
		{
			name: "williamsr_reversion",
			script: `strategy DSL Block Matrix WilliamsR
version 1
symbol US.AAPL
interval 1m

on init:
  log "williamsr matrix"

on kline_close:
  let exhaustion = williams_r(14)
  if exhaustion and exhaustion < -60:
    buy shares 1 policy same_direction type MARKET
  if exhaustion and exhaustion > -40:
    sell shares 1 policy allow type MARKET`,
		},
		{
			name: "breakout_notify",
			script: `strategy DSL Block Matrix Breakout
version 1
symbol US.AAPL
interval 1m

on init:
  notify "breakout matrix init"

on kline_close:
  if close > 101:
    notify "breakout hit"
    buy cash_percent 2 policy same_direction type MARKET
  else:
    log "breakout idle"`,
		},
		{
			name: "mean_reversion_price",
			script: `strategy DSL Block Matrix Mean Reversion
version 1
symbol US.AAPL
interval 1m

on init:
  log "mean reversion matrix"

on kline_close:
  if close < 99:
    log "dip detected"
    buy shares 1 policy flat_only type MARKET
  else:
    notify "mean reversion idle"`,
		},
		{
			name: "protect_session_risk",
			script: `strategy DSL Block Matrix Protect
version 1
symbol US.AAPL
interval 1m

on init:
  log "protect matrix"

on kline_close:
  if close > 100.5:
    buy shares 1 policy same_direction type MARKET
  protect auto stopLoss 2 hour 2 window session
  protect auto takeProfit 2 hour 3 window session
  protect auto trailingStop 2 hour 1.5 window session
  notify "protect evaluated"`,
		},
	}
}

func TestStrategyBlockBenchmarkCasesSmoke(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(t)
	restoreLogs := suppressBacktestRunLogs(t)
	defer restoreLogs()

	ctx := context.Background()
	for _, benchmarkCase := range strategyBlockBenchmarkCases() {
		benchmarkCase := benchmarkCase
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
	b.Setenv("HOME", b.TempDir())
	dbPath, startTime, endTime := seedStrategyBlockBenchmarkStore(b)
	restoreLogs := suppressBacktestRunLogs(b)
	defer restoreLogs()

	ctx := context.Background()
	for _, benchmarkCase := range strategyBlockBenchmarkCases() {
		benchmarkCase := benchmarkCase
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
		SourceFormat:     strategydefinition.SourceFormatDSLV1,
		StartTime:        startTime,
		EndTime:          endTime,
		StrategyScript:   script,
		InitialBalance:   10000,
		WarmupCandles:    256,
		UseExtendedHours: boolPtr(true),
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
		_ = store.Close()
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
