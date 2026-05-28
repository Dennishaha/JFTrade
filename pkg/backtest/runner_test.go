package backtest

import (
	"bytes"
	"context"
	"io"
	"log"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/sirupsen/logrus"
)

var benchmarkBacktestResult *RunResult

const benchmarkBacktestStrategyScript = `strategy DSL Indicator Heavy Benchmark
version 1
symbol US.AAPL
interval 1m

on kline_close:
	let fast = ma(MA, 20)
	let trend = ma(EMA, 55)
	let volumeTrend = ma(VWMA, 20)
	let momentum = rsi(14)
	let flow = macd(12, 26, 9)
	let band = bollinger(20, 2)
	let swing = kdj(9, 3, 3)
	let range = atr(14)
	let channel = cci(20)
	let exhaustion = williams_r(14)
	if cross_over(fast, trend) and momentum > 50 and flow.histogram > 0 and swing.j > 45 and close > band.middle and close > volumeTrend.value and range > 0 and channel > -100 and exhaustion > -85 and not divergence_top(flow, 6):
		buy shares 1
	if cross_under(fast, trend) or (divergence_top(flow, 6) and close < band.middle):
		sell shares 1`

func TestRunExecutesLocalBacktestSmoke(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "backtest.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}

	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	klines := []types.KLine{
		{
			StartTime: types.Time(baseStart),
			EndTime:   types.Time(baseStart.Add(time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100),
			High:      fixedpoint.NewFromFloat(101),
			Low:       fixedpoint.NewFromFloat(99.5),
			Close:     fixedpoint.NewFromFloat(100.5),
			Volume:    fixedpoint.NewFromFloat(1000),
		},
		{
			StartTime: types.Time(baseStart.Add(time.Minute)),
			EndTime:   types.Time(baseStart.Add(2*time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100.5),
			High:      fixedpoint.NewFromFloat(102),
			Low:       fixedpoint.NewFromFloat(100),
			Close:     fixedpoint.NewFromFloat(101.75),
			Volume:    fixedpoint.NewFromFloat(1200),
		},
		{
			StartTime: types.Time(baseStart.Add(2 * time.Minute)),
			EndTime:   types.Time(baseStart.Add(3*time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(101.75),
			High:      fixedpoint.NewFromFloat(103),
			Low:       fixedpoint.NewFromFloat(101.25),
			Close:     fixedpoint.NewFromFloat(102.5),
			Volume:    fixedpoint.NewFromFloat(1500),
		},
	}

	if err := store.InsertKLines(klines, "forward"); err != nil {
		_ = store.Close()
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	result := Run(context.Background(), RunConfig{
		DBPath:       dbPath,
		Symbol:       "US.AAPL",
		Interval:     string(types.Interval1m),
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		StartTime:    klines[1].StartTime.Time(),
		EndTime:      klines[2].EndTime.Time(),
		StrategyScript: `strategy DSL Smoke
version 1
symbol US.AAPL
interval 1m

on init:
  log "dsl smoke init"

on kline_close:
  log "dsl smoke kline"`,
		InitialBalance: 10000,
		WarmupCandles:  1,
	})

	if result == nil {
		t.Fatal("expected run result")
	}
	if result.Error != "" {
		t.Fatalf("Run() error = %s", result.Error)
	}
	if result.QuoteCurrency != "USD" {
		t.Fatalf("QuoteCurrency = %s, want USD", result.QuoteCurrency)
	}
	if result.FinalBalance != 10000 {
		t.Fatalf("FinalBalance = %f, want 10000", result.FinalBalance)
	}
	if result.PnL != 0 {
		t.Fatalf("PnL = %f, want 0", result.PnL)
	}
	if result.TotalTrades != 0 {
		t.Fatalf("TotalTrades = %d, want 0", result.TotalTrades)
	}
	if len(result.RuntimeErrors) != 0 {
		t.Fatalf("RuntimeErrors = %#v, want empty", result.RuntimeErrors)
	}
	if len(result.Candles) == 0 {
		t.Fatal("expected replayed candles")
	}
	if len(result.PnLCurve) != len(result.Candles) {
		t.Fatalf("PnLCurve len = %d, want %d", len(result.PnLCurve), len(result.Candles))
	}
	for _, candle := range result.Candles {
		candleTime, parseErr := time.Parse(time.RFC3339, candle.Time)
		if parseErr != nil {
			t.Fatalf("parse candle time %q: %v", candle.Time, parseErr)
		}
		if candleTime.Before(klines[1].StartTime.Time()) || candleTime.After(klines[2].EndTime.Time()) {
			t.Fatalf("candle time %s outside requested replay window [%s, %s]", candle.Time, klines[1].StartTime.Time().Format(time.RFC3339), klines[2].EndTime.Time().Format(time.RFC3339))
		}
	}
}

func TestRunExecutesDSLBacktestSmoke(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "backtest-dsl.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}

	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	klines := []types.KLine{
		{
			StartTime: types.Time(baseStart),
			EndTime:   types.Time(baseStart.Add(time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100),
			High:      fixedpoint.NewFromFloat(100.5),
			Low:       fixedpoint.NewFromFloat(99.5),
			Close:     fixedpoint.NewFromFloat(100),
			Volume:    fixedpoint.NewFromFloat(1000),
		},
		{
			StartTime: types.Time(baseStart.Add(time.Minute)),
			EndTime:   types.Time(baseStart.Add(2*time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100),
			High:      fixedpoint.NewFromFloat(100.25),
			Low:       fixedpoint.NewFromFloat(98.75),
			Close:     fixedpoint.NewFromFloat(99),
			Volume:    fixedpoint.NewFromFloat(1200),
		},
		{
			StartTime: types.Time(baseStart.Add(2 * time.Minute)),
			EndTime:   types.Time(baseStart.Add(3*time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(99),
			High:      fixedpoint.NewFromFloat(101.5),
			Low:       fixedpoint.NewFromFloat(98.5),
			Close:     fixedpoint.NewFromFloat(101),
			Volume:    fixedpoint.NewFromFloat(1500),
		},
		{
			StartTime: types.Time(baseStart.Add(3 * time.Minute)),
			EndTime:   types.Time(baseStart.Add(4*time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(101),
			High:      fixedpoint.NewFromFloat(102),
			Low:       fixedpoint.NewFromFloat(100.75),
			Close:     fixedpoint.NewFromFloat(101.5),
			Volume:    fixedpoint.NewFromFloat(1300),
		},
	}

	if err := store.InsertKLines(klines, "forward"); err != nil {
		_ = store.Close()
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	result := Run(context.Background(), RunConfig{
		DBPath:       dbPath,
		Symbol:       "US.AAPL",
		Interval:     string(types.Interval1m),
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		StartTime:    klines[1].StartTime.Time(),
		EndTime:      klines[3].EndTime.Time(),
		StrategyScript: `strategy DSL Cross
version 1
symbol US.AAPL
interval 1m

on init:
  log "dsl init"

on kline_close:
  let fast = ma(MA, 1)
  let slow = ma(MA, 2)
  if cross_over(fast, slow):
    buy shares 1`,
		InitialBalance: 10000,
		WarmupCandles:  1,
	})

	if result == nil {
		t.Fatal("expected run result")
	}
	if result.Error != "" {
		t.Fatalf("Run() error = %s", result.Error)
	}
	if result.TotalTrades == 0 {
		t.Fatalf("TotalTrades = %d, want > 0", result.TotalTrades)
	}
	if len(result.OrderBook) == 0 {
		t.Fatal("expected order book entries for DSL backtest")
	}
	if len(result.RuntimeErrors) != 0 {
		t.Fatalf("RuntimeErrors = %#v, want empty", result.RuntimeErrors)
	}
}

func TestRunUsesOneMinuteDataForFiveMinuteBacktest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "backtest-5m.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}

	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	minuteKLines := make([]types.KLine, 0, 15)
	for index := 0; index < 15; index++ {
		startAt := baseStart.Add(time.Duration(index) * time.Minute)
		minuteKLines = append(minuteKLines, types.KLine{
			StartTime: types.Time(startAt),
			EndTime:   types.Time(startAt.Add(time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100 + float64(index)*0.25),
			High:      fixedpoint.NewFromFloat(100.5 + float64(index)*0.25),
			Low:       fixedpoint.NewFromFloat(99.5 + float64(index)*0.25),
			Close:     fixedpoint.NewFromFloat(100.1 + float64(index)*0.25),
			Volume:    fixedpoint.NewFromFloat(float64(1000 + index*10)),
		})
	}
	if err := store.InsertKLines(minuteKLines, "forward"); err != nil {
		_ = store.Close()
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	result := Run(context.Background(), RunConfig{
		DBPath:       dbPath,
		Symbol:       "US.AAPL",
		Interval:     string(types.Interval5m),
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		StartTime:    baseStart.Add(5 * time.Minute),
		EndTime:      baseStart.Add(15*time.Minute - time.Millisecond),
		StrategyScript: `strategy DSL Five Minute
version 1
symbol US.AAPL
interval 5m

on init:
  log "dsl 5m init"

on kline_close:
  log "dsl 5m kline"`,
		InitialBalance: 10000,
		WarmupCandles:  1,
	})

	if result == nil {
		t.Fatal("expected run result")
	}
	if result.Error != "" {
		t.Fatalf("Run() error = %s", result.Error)
	}
	if len(result.Candles) == 0 {
		t.Fatal("expected synthesized 5m candles to be replayed")
	}
}

func TestRunAllowsBoundaryCoveredOneMinuteDataForSyntheticFiveMinuteBacktest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "backtest-5m-missing.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}

	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	minuteKLines := make([]types.KLine, 0, 8)
	for index := 0; index < 8; index++ {
		startAt := baseStart.Add(time.Duration(index) * time.Minute)
		minuteKLines = append(minuteKLines, types.KLine{
			StartTime: types.Time(startAt),
			EndTime:   types.Time(startAt.Add(time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100 + float64(index)*0.25),
			High:      fixedpoint.NewFromFloat(100.5 + float64(index)*0.25),
			Low:       fixedpoint.NewFromFloat(99.5 + float64(index)*0.25),
			Close:     fixedpoint.NewFromFloat(100.1 + float64(index)*0.25),
			Volume:    fixedpoint.NewFromFloat(float64(1000 + index*10)),
		})
	}
	if err := store.InsertKLines(minuteKLines, "forward"); err != nil {
		_ = store.Close()
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	result := Run(context.Background(), RunConfig{
		DBPath:       dbPath,
		Symbol:       "US.AAPL",
		Interval:     string(types.Interval5m),
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		StartTime:    baseStart.Add(5 * time.Minute),
		EndTime:      baseStart.Add(15*time.Minute - time.Millisecond),
		StrategyScript: `strategy DSL Five Minute Missing
version 1
symbol US.AAPL
interval 5m

on init:
  log "dsl 5m init"`,
		InitialBalance: 10000,
		WarmupCandles:  1,
	})

	if result == nil {
		t.Fatal("expected run result")
	}
	if result.Error != "" {
		t.Fatalf("expected simplified boundary coverage to allow run, got %s", result.Error)
	}
}

func TestRunUsesFiveMinuteDataForFifteenMinuteBacktest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "backtest-15m-from-5m.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}

	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	fiveMinuteKLines := make([]types.KLine, 0, 6)
	for index := 0; index < 6; index++ {
		startAt := baseStart.Add(time.Duration(index*5) * time.Minute)
		fiveMinuteKLines = append(fiveMinuteKLines, types.KLine{
			StartTime: types.Time(startAt),
			EndTime:   types.Time(startAt.Add(5*time.Minute - time.Millisecond)),
			Interval:  types.Interval5m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100 + float64(index)*2),
			High:      fixedpoint.NewFromFloat(101 + float64(index)*2),
			Low:       fixedpoint.NewFromFloat(99 + float64(index)*2),
			Close:     fixedpoint.NewFromFloat(100.5 + float64(index)*2),
			Volume:    fixedpoint.NewFromFloat(float64(1000 + index*100)),
		})
	}
	if err := store.InsertKLines(fiveMinuteKLines, "forward"); err != nil {
		_ = store.Close()
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	result := Run(context.Background(), RunConfig{
		DBPath:       dbPath,
		Symbol:       "US.AAPL",
		Interval:     string(types.Interval15m),
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		StartTime:    baseStart.Add(15 * time.Minute),
		EndTime:      baseStart.Add(30*time.Minute - time.Millisecond),
		StrategyScript: `strategy DSL Fifteen Minute
version 1
symbol US.AAPL
interval 15m

on init:
  log "dsl 15m init"

on kline_close:
  log "dsl 15m kline"`,
		InitialBalance: 10000,
		WarmupCandles:  1,
	})

	if result == nil {
		t.Fatal("expected run result")
	}
	if result.Error != "" {
		t.Fatalf("Run() error = %s", result.Error)
	}
	if len(result.RuntimeErrors) != 0 {
		t.Fatalf("RuntimeErrors = %#v, want empty", result.RuntimeErrors)
	}
}

func TestRunLogsDerivedStrategyWarmup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "backtest-auto-warmup.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}

	startAt := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	currentKLine := types.KLine{
		StartTime: types.Time(startAt),
		EndTime:   types.Time(startAt.Add(time.Minute - time.Millisecond)),
		Interval:  types.Interval1m,
		Symbol:    "US.AAPL",
		Open:      fixedpoint.NewFromFloat(100),
		High:      fixedpoint.NewFromFloat(101),
		Low:       fixedpoint.NewFromFloat(99.5),
		Close:     fixedpoint.NewFromFloat(100.5),
		Volume:    fixedpoint.NewFromFloat(1000),
	}

	if err := store.InsertKLines([]types.KLine{currentKLine}, "forward"); err != nil {
		_ = store.Close()
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	var logBuffer bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logBuffer)
	defer log.SetOutput(previousWriter)

	result := Run(context.Background(), RunConfig{
		DBPath:       dbPath,
		Symbol:       "US.AAPL",
		Interval:     string(types.Interval1m),
		SourceFormat: strategydefinition.SourceFormatDSLV1,
		StartTime:    currentKLine.StartTime.Time(),
		EndTime:      currentKLine.EndTime.Time(),
		StrategyScript: `strategy DSL Auto Warmup
version 1
symbol US.AAPL
interval 1m

on kline_close:
  let slow = ma(MA, 20)
  log "auto warmup"`,
		InitialBalance: 10000,
	})

	if result == nil {
		t.Fatal("expected run result")
	}
	if result.Error != "" {
		t.Fatalf("Run() error = %s", result.Error)
	}
	if !strings.Contains(logBuffer.String(), "warmup 20 candles (configured=0 derived=20") {
		t.Fatalf("expected derived warmup log, got %q", logBuffer.String())
	}
}

func BenchmarkRunExecutesIndicatorHeavyDSLBacktest(b *testing.B) {
	b.Setenv("HOME", b.TempDir())
	dbPath, startTime, endTime := seedBenchmarkBacktestStore(b)
	previousWriter := log.Writer()
	previousLogrusWriter := logrus.StandardLogger().Out
	log.SetOutput(io.Discard)
	logrus.SetOutput(io.Discard)
	b.Cleanup(func() {
		log.SetOutput(previousWriter)
		logrus.SetOutput(previousLogrusWriter)
	})
	ctx := context.Background()
	cfg := RunConfig{
		DBPath:         dbPath,
		Symbol:         "US.AAPL",
		Interval:       string(types.Interval1m),
		SourceFormat:   strategydefinition.SourceFormatDSLV1,
		StartTime:      startTime,
		EndTime:        endTime,
		StrategyScript: benchmarkBacktestStrategyScript,
		InitialBalance: 10000,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		benchmarkBacktestResult = Run(ctx, cfg)
		if benchmarkBacktestResult == nil {
			b.Fatal("expected run result")
		}
		if benchmarkBacktestResult.Error != "" {
			b.Fatalf("Run() error = %s", benchmarkBacktestResult.Error)
		}
	}
}

func seedBenchmarkBacktestStore(b *testing.B) (string, time.Time, time.Time) {
	b.Helper()
	dbPath := filepath.Join(b.TempDir(), "benchmark-backtest.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		b.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	klines := buildBenchmarkKLines(baseStart, 2048)
	if err := store.InsertKLines(klines, "forward"); err != nil {
		_ = store.Close()
		b.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		b.Fatalf("store.Close() error = %v", err)
	}
	startIndex := 512
	return dbPath, klines[startIndex].StartTime.Time(), klines[len(klines)-1].EndTime.Time()
}

func buildBenchmarkKLines(baseStart time.Time, count int) []types.KLine {
	klines := make([]types.KLine, 0, count)
	previousClose := 100.0
	for index := 0; index < count; index++ {
		startAt := baseStart.Add(time.Duration(index) * time.Minute)
		cycle := math.Sin(float64(index)/18.0)*4 + math.Cos(float64(index)/7.0)*1.5
		drift := float64(index%97) / 97.0 * 0.4
		closeValue := 100 + cycle + drift
		openValue := previousClose
		highValue := math.Max(openValue, closeValue) + 0.75 + float64(index%5)*0.03
		lowValue := math.Min(openValue, closeValue) - 0.75 - float64(index%7)*0.02
		klines = append(klines, types.KLine{
			StartTime: types.Time(startAt),
			EndTime:   types.Time(startAt.Add(time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(openValue),
			High:      fixedpoint.NewFromFloat(highValue),
			Low:       fixedpoint.NewFromFloat(lowValue),
			Close:     fixedpoint.NewFromFloat(closeValue),
			Volume:    fixedpoint.NewFromFloat(1000 + float64((index*37)%400)),
		})
		previousClose = closeValue
	}
	return klines
}
