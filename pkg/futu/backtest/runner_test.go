package backtest

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

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
