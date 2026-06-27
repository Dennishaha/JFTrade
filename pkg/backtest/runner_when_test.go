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

func TestRunHonorsPineWhenConditions(t *testing.T) {
	isolateBacktestHome(t)

	dbPath := filepath.Join(t.TempDir(), "backtest-pine-when.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	klines := []types.KLine{
		{
			Open:   fixedpoint.NewFromFloat(101),
			High:   fixedpoint.NewFromFloat(102),
			Low:    fixedpoint.NewFromFloat(98),
			Close:  fixedpoint.NewFromFloat(99),
			Volume: fixedpoint.NewFromFloat(1000),
		},
		{
			Open:   fixedpoint.NewFromFloat(99),
			High:   fixedpoint.NewFromFloat(103),
			Low:    fixedpoint.NewFromFloat(98),
			Close:  fixedpoint.NewFromFloat(102),
			Volume: fixedpoint.NewFromFloat(1000),
		},
		{
			Open:   fixedpoint.NewFromFloat(103),
			High:   fixedpoint.NewFromFloat(104),
			Low:    fixedpoint.NewFromFloat(100),
			Close:  fixedpoint.NewFromFloat(101),
			Volume: fixedpoint.NewFromFloat(1000),
		},
		{
			Open:   fixedpoint.NewFromFloat(100),
			High:   fixedpoint.NewFromFloat(101),
			Low:    fixedpoint.NewFromFloat(99),
			Close:  fixedpoint.NewFromFloat(100),
			Volume: fixedpoint.NewFromFloat(1000),
		},
	}
	for index := range klines {
		start := baseStart.Add(time.Duration(index) * time.Minute)
		klines[index].StartTime = types.Time(start)
		klines[index].EndTime = types.Time(start.Add(time.Minute - time.Millisecond))
		klines[index].Interval = types.Interval1m
		klines[index].Symbol = "US.AAPL"
	}
	if err := store.InsertKLines(klines, "forward"); err != nil {
		closeErr := store.Close()
		jftradeCheckTestError(t, closeErr)
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	result := Run(context.Background(), RunConfig{
		DBPath:       dbPath,
		Symbol:       "US.AAPL",
		Interval:     string(types.Interval1m),
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StartTime:    klines[0].StartTime.Time(),
		EndTime:      klines[len(klines)-1].EndTime.Time(),
		StrategyScript: `//@version=6
strategy("When gates", overlay=true)
if bar_index == 0
    strategy.entry("Long", strategy.long, qty=1, when=close > open)
if bar_index == 1
    strategy.entry("Long", strategy.long, qty=1, when=close > open)
if bar_index == 2
    strategy.close("Long", when=close < open)`,
		InitialBalance: 100000,
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
	if len(result.Trades) != 2 {
		t.Fatalf("trades = %#v, want gated BUY then gated SELL", result.Trades)
	}
	if result.Trades[0].Side != "BUY" || result.Trades[0].Qty != "1" {
		t.Fatalf("first trade = %#v, want BUY 1", result.Trades[0])
	}
	if result.Trades[1].Side != "SELL" || result.Trades[1].Qty != "1" {
		t.Fatalf("second trade = %#v, want SELL 1", result.Trades[1])
	}
}

func TestRunSupportsPineExitProfitLossTicks(t *testing.T) {
	isolateBacktestHome(t)

	dbPath := filepath.Join(t.TempDir(), "backtest-pine-exit-profit-loss.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	baseStart := time.Date(2026, time.May, 26, 9, 30, 0, 0, time.UTC)
	klines := []types.KLine{
		{
			Open:   fixedpoint.NewFromFloat(100),
			High:   fixedpoint.NewFromFloat(100),
			Low:    fixedpoint.NewFromFloat(100),
			Close:  fixedpoint.NewFromFloat(100),
			Volume: fixedpoint.NewFromFloat(1000),
		},
		{
			Open:   fixedpoint.NewFromFloat(100),
			High:   fixedpoint.NewFromFloat(101),
			Low:    fixedpoint.NewFromFloat(99.8),
			Close:  fixedpoint.NewFromFloat(100.6),
			Volume: fixedpoint.NewFromFloat(1000),
		},
		{
			Open:   fixedpoint.NewFromFloat(100.6),
			High:   fixedpoint.NewFromFloat(100.7),
			Low:    fixedpoint.NewFromFloat(100.4),
			Close:  fixedpoint.NewFromFloat(100.5),
			Volume: fixedpoint.NewFromFloat(1000),
		},
	}
	for index := range klines {
		start := baseStart.Add(time.Duration(index) * time.Minute)
		klines[index].StartTime = types.Time(start)
		klines[index].EndTime = types.Time(start.Add(time.Minute - time.Millisecond))
		klines[index].Interval = types.Interval1m
		klines[index].Symbol = "US.AAPL"
	}
	if err := store.InsertKLines(klines, "forward"); err != nil {
		closeErr := store.Close()
		jftradeCheckTestError(t, closeErr)
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	result := Run(context.Background(), RunConfig{
		DBPath:       dbPath,
		Symbol:       "US.AAPL",
		Interval:     string(types.Interval1m),
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StartTime:    klines[0].StartTime.Time(),
		EndTime:      klines[len(klines)-1].EndTime.Time(),
		StrategyScript: `//@version=6
strategy("Exit points", overlay=true)
if bar_index == 0
    strategy.entry("Long", strategy.long, qty=1)
if bar_index == 1
    strategy.exit("TakeProfit", "Long", profit=50)`,
		InitialBalance: 100000,
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
	if len(result.Trades) != 2 {
		t.Fatalf("trades = %#v, want BUY then SELL from profit ticks", result.Trades)
	}
	if result.Trades[0].Side != "BUY" || result.Trades[1].Side != "SELL" {
		t.Fatalf("trades = %#v, want BUY then SELL", result.Trades)
	}
}
