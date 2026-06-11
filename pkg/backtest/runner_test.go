package backtest

import (
	"bytes"
	"context"
	"fmt"
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

const benchmarkBacktestStrategyScript = `//@version=6
strategy("Pine Indicator Heavy Benchmark", overlay=true)

fast = ta.sma(close, 20)
trend = ta.ema(close, 55)
momentum = ta.rsi(close, 14)
[macdLine, signalLine, histLine] = ta.macd(close, 12, 26, 9)
range = ta.atr(14)
channel = ta.cci(close, 20)
if ta.crossover(fast, trend) and close > fast
    strategy.entry("Long", strategy.long, qty=1)
if ta.crossunder(fast, trend) or close < trend
    strategy.close("Long")`

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
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StartTime:    klines[1].StartTime.Time(),
		EndTime:      klines[2].EndTime.Time(),
		StrategyScript: `//@version=6
strategy("Pine Smoke", overlay=true)
log.info("pine smoke kline")`,
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

func TestSessionFilteredBacktestStoreFiltersUSExtendedHours(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "backtest-rth.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}

	premarketStart := time.Date(2026, time.May, 26, 13, 0, 0, 0, time.UTC)
	regularStart := time.Date(2026, time.May, 26, 13, 30, 0, 0, time.UTC)
	klines := []types.KLine{
		{
			StartTime: types.Time(premarketStart),
			EndTime:   types.Time(premarketStart.Add(time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100),
			High:      fixedpoint.NewFromFloat(100.5),
			Low:       fixedpoint.NewFromFloat(99.8),
			Close:     fixedpoint.NewFromFloat(100.2),
			Volume:    fixedpoint.NewFromFloat(500),
		},
		{
			StartTime: types.Time(regularStart),
			EndTime:   types.Time(regularStart.Add(time.Minute - time.Millisecond)),
			Interval:  types.Interval1m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(101),
			High:      fixedpoint.NewFromFloat(101.5),
			Low:       fixedpoint.NewFromFloat(100.9),
			Close:     fixedpoint.NewFromFloat(101.2),
			Volume:    fixedpoint.NewFromFloat(700),
		},
	}

	if err := store.InsertKLines(klines, "forward"); err != nil {
		_ = store.Close()
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	reopenedStore, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore(reopen) error = %v", err)
	}
	defer reopenedStore.Close()
	reopenedStore.SetRehabType("forward")

	filteredStore := newBacktestReplayStore(reopenedStore, boolPtr(false))
	backwardRows, err := filteredStore.QueryKLinesBackward(nil, "US.AAPL", types.Interval1m, regularStart.Add(time.Minute-time.Millisecond), 2)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(filtered) error = %v", err)
	}
	if len(backwardRows) != 1 {
		t.Fatalf("QueryKLinesBackward(filtered) len = %d, want 1", len(backwardRows))
	}
	if got := backwardRows[0].StartTime.Time(); !got.Equal(regularStart) {
		t.Fatalf("QueryKLinesBackward(filtered) start = %s, want %s", got, regularStart)
	}

	filteredCh, filteredErrCh := filteredStore.QueryKLinesCh(
		premarketStart,
		regularStart.Add(time.Minute-time.Millisecond),
		nil,
		[]string{"US.AAPL"},
		[]types.Interval{types.Interval1m},
	)
	filteredRows, err := collectKLinesFromChannels(filteredCh, filteredErrCh)
	if err != nil {
		t.Fatalf("QueryKLinesCh(filtered) error = %v", err)
	}
	if len(filteredRows) != 1 {
		t.Fatalf("QueryKLinesCh(filtered) len = %d, want 1", len(filteredRows))
	}
	if got := filteredRows[0].StartTime.Time(); !got.Equal(regularStart) {
		t.Fatalf("QueryKLinesCh(filtered) start = %s, want %s", got, regularStart)
	}

	unfilteredStore := newBacktestReplayStore(reopenedStore, boolPtr(true))
	unfilteredCh, unfilteredErrCh := unfilteredStore.QueryKLinesCh(
		premarketStart,
		regularStart.Add(time.Minute-time.Millisecond),
		nil,
		[]string{"US.AAPL"},
		[]types.Interval{types.Interval1m},
	)
	unfilteredRows, err := collectKLinesFromChannels(unfilteredCh, unfilteredErrCh)
	if err != nil {
		t.Fatalf("QueryKLinesCh(unfiltered) error = %v", err)
	}
	if len(unfilteredRows) != 2 {
		t.Fatalf("QueryKLinesCh(unfiltered) len = %d, want 2", len(unfilteredRows))
	}

	filteredStreamRows, err := collectKLinesFromStreamer(filteredStore, premarketStart, regularStart.Add(time.Minute-time.Millisecond), []string{"US.AAPL"}, []types.Interval{types.Interval1m})
	if err != nil {
		t.Fatalf("StreamKLines(filtered) error = %v", err)
	}
	if len(filteredStreamRows) != 1 {
		t.Fatalf("StreamKLines(filtered) len = %d, want 1", len(filteredStreamRows))
	}
	if got := filteredStreamRows[0].StartTime.Time(); !got.Equal(regularStart) {
		t.Fatalf("StreamKLines(filtered) start = %s, want %s", got, regularStart)
	}

	unfilteredStreamRows, err := collectKLinesFromStreamer(unfilteredStore, premarketStart, regularStart.Add(time.Minute-time.Millisecond), []string{"US.AAPL"}, []types.Interval{types.Interval1m})
	if err != nil {
		t.Fatalf("StreamKLines(unfiltered) error = %v", err)
	}
	if len(unfilteredStreamRows) != 2 {
		t.Fatalf("StreamKLines(unfiltered) len = %d, want 2", len(unfilteredStreamRows))
	}
}

func TestSessionFilteredBacktestStoreSynthesizesUSDailyWithOvernightWhenExtendedEnabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "backtest-extended-daily.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}

	baseRows := []types.KLine{
		{
			StartTime: types.Time(time.Date(2026, time.January, 8, 1, 0, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.January, 8, 1, 59, 59, 999000000, time.UTC)),
			Interval:  types.Interval1h,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(90),
			High:      fixedpoint.NewFromFloat(92),
			Low:       fixedpoint.NewFromFloat(89),
			Close:     fixedpoint.NewFromFloat(91),
			Volume:    fixedpoint.NewFromFloat(400),
		},
		{
			StartTime: types.Time(time.Date(2026, time.January, 8, 14, 30, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.January, 8, 15, 29, 59, 999000000, time.UTC)),
			Interval:  types.Interval1h,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100),
			High:      fixedpoint.NewFromFloat(106),
			Low:       fixedpoint.NewFromFloat(99),
			Close:     fixedpoint.NewFromFloat(105),
			Volume:    fixedpoint.NewFromFloat(1000),
		},
		{
			StartTime: types.Time(time.Date(2026, time.January, 9, 0, 0, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.January, 9, 0, 59, 59, 999000000, time.UTC)),
			Interval:  types.Interval1h,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(105),
			High:      fixedpoint.NewFromFloat(108),
			Low:       fixedpoint.NewFromFloat(104),
			Close:     fixedpoint.NewFromFloat(107),
			Volume:    fixedpoint.NewFromFloat(700),
		},
	}
	if err := store.InsertKLines(baseRows, "forward"); err != nil {
		_ = store.Close()
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	reopenedStore, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore(reopen) error = %v", err)
	}
	defer reopenedStore.Close()
	reopenedStore.SetRehabType("forward")

	regularDailyStore := newBacktestReplayStore(reopenedStore, boolPtr(false))
	regularRows, err := regularDailyStore.QueryKLinesBackward(nil, "US.AAPL", types.Interval1d, time.Date(2026, time.January, 9, 0, 0, 0, 0, time.UTC), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(regular 1d) error = %v", err)
	}
	if len(regularRows) != 1 {
		t.Fatalf("regular daily rows len = %d, want 1", len(regularRows))
	}
	if regularRows[0].Volume.Compare(fixedpoint.NewFromFloat(1000)) != 0 {
		t.Fatalf("regular daily volume = %s, want 1000", regularRows[0].Volume.String())
	}

	extendedDailyStore := newBacktestReplayStore(reopenedStore, boolPtr(true))
	extendedRows, err := extendedDailyStore.QueryKLinesBackward(nil, "US.AAPL", types.Interval1d, time.Date(2026, time.January, 9, 0, 0, 0, 0, time.UTC), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(extended 1d) error = %v", err)
	}
	if len(extendedRows) != 1 {
		t.Fatalf("extended daily rows len = %d, want 1", len(extendedRows))
	}
	if extendedRows[0].Open.Compare(fixedpoint.NewFromFloat(90)) != 0 {
		t.Fatalf("extended daily open = %s, want 90", extendedRows[0].Open.String())
	}
	if extendedRows[0].High.Compare(fixedpoint.NewFromFloat(108)) != 0 {
		t.Fatalf("extended daily high = %s, want 108", extendedRows[0].High.String())
	}
	if extendedRows[0].Low.Compare(fixedpoint.NewFromFloat(89)) != 0 {
		t.Fatalf("extended daily low = %s, want 89", extendedRows[0].Low.String())
	}
	if extendedRows[0].Close.Compare(fixedpoint.NewFromFloat(107)) != 0 {
		t.Fatalf("extended daily close = %s, want 107", extendedRows[0].Close.String())
	}
	if extendedRows[0].Volume.Compare(fixedpoint.NewFromFloat(2100)) != 0 {
		t.Fatalf("extended daily volume = %s, want 2100", extendedRows[0].Volume.String())
	}

	extendedCh, extendedErrCh := extendedDailyStore.QueryKLinesCh(
		time.Date(2026, time.January, 8, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 8, 23, 59, 59, 999000000, time.UTC),
		nil,
		[]string{"US.AAPL"},
		[]types.Interval{types.Interval1d},
	)
	extendedChannelRows, err := collectKLinesFromChannels(extendedCh, extendedErrCh)
	if err != nil {
		t.Fatalf("QueryKLinesCh(extended 1d) error = %v", err)
	}
	if len(extendedChannelRows) != 1 {
		t.Fatalf("extended daily channel rows len = %d, want 1", len(extendedChannelRows))
	}
	if extendedChannelRows[0].Volume.Compare(fixedpoint.NewFromFloat(2100)) != 0 {
		t.Fatalf("extended daily channel volume = %s, want 2100", extendedChannelRows[0].Volume.String())
	}

	extendedStreamRows, err := collectKLinesFromStreamer(extendedDailyStore, time.Date(2026, time.January, 8, 0, 0, 0, 0, time.UTC), time.Date(2026, time.January, 8, 23, 59, 59, 999000000, time.UTC), []string{"US.AAPL"}, []types.Interval{types.Interval1d})
	if err != nil {
		t.Fatalf("StreamKLines(extended 1d) error = %v", err)
	}
	if len(extendedStreamRows) != 1 {
		t.Fatalf("extended daily stream rows len = %d, want 1", len(extendedStreamRows))
	}
	if extendedStreamRows[0].Volume.Compare(fixedpoint.NewFromFloat(2100)) != 0 {
		t.Fatalf("extended daily stream volume = %s, want 2100", extendedStreamRows[0].Volume.String())
	}
}

func TestSessionFilteredBacktestStoreSynthesizesUSWeeklyWithOvernightWhenExtendedEnabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "backtest-extended-weekly.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}

	baseRows := []types.KLine{
		{
			StartTime: types.Time(time.Date(2026, time.January, 5, 1, 0, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.January, 5, 1, 59, 59, 999000000, time.UTC)),
			Interval:  types.Interval1h,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(90),
			High:      fixedpoint.NewFromFloat(92),
			Low:       fixedpoint.NewFromFloat(89),
			Close:     fixedpoint.NewFromFloat(91),
			Volume:    fixedpoint.NewFromFloat(400),
		},
		{
			StartTime: types.Time(time.Date(2026, time.January, 5, 14, 30, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.January, 5, 15, 29, 59, 999000000, time.UTC)),
			Interval:  types.Interval1h,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(100),
			High:      fixedpoint.NewFromFloat(103),
			Low:       fixedpoint.NewFromFloat(99),
			Close:     fixedpoint.NewFromFloat(102),
			Volume:    fixedpoint.NewFromFloat(1000),
		},
		{
			StartTime: types.Time(time.Date(2026, time.January, 9, 14, 30, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.January, 9, 15, 29, 59, 999000000, time.UTC)),
			Interval:  types.Interval1h,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(104),
			High:      fixedpoint.NewFromFloat(110),
			Low:       fixedpoint.NewFromFloat(103),
			Close:     fixedpoint.NewFromFloat(109),
			Volume:    fixedpoint.NewFromFloat(1200),
		},
		{
			StartTime: types.Time(time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC)),
			EndTime:   types.Time(time.Date(2026, time.January, 10, 0, 59, 59, 999000000, time.UTC)),
			Interval:  types.Interval1h,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(109),
			High:      fixedpoint.NewFromFloat(112),
			Low:       fixedpoint.NewFromFloat(108),
			Close:     fixedpoint.NewFromFloat(111),
			Volume:    fixedpoint.NewFromFloat(700),
		},
	}
	if err := store.InsertKLines(baseRows, "forward"); err != nil {
		_ = store.Close()
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	reopenedStore, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore(reopen) error = %v", err)
	}
	defer reopenedStore.Close()
	reopenedStore.SetRehabType("forward")

	regularWeeklyStore := newBacktestReplayStore(reopenedStore, boolPtr(false))
	regularRows, err := regularWeeklyStore.QueryKLinesBackward(nil, "US.AAPL", types.Interval1w, time.Date(2026, time.January, 12, 0, 0, 0, 0, time.UTC), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(regular 1w) error = %v", err)
	}
	if len(regularRows) != 1 {
		t.Fatalf("regular weekly rows len = %d, want 1", len(regularRows))
	}
	if regularRows[0].Open.Compare(fixedpoint.NewFromFloat(100)) != 0 {
		t.Fatalf("regular weekly open = %s, want 100", regularRows[0].Open.String())
	}
	if regularRows[0].Close.Compare(fixedpoint.NewFromFloat(109)) != 0 {
		t.Fatalf("regular weekly close = %s, want 109", regularRows[0].Close.String())
	}
	if regularRows[0].Volume.Compare(fixedpoint.NewFromFloat(2200)) != 0 {
		t.Fatalf("regular weekly volume = %s, want 2200", regularRows[0].Volume.String())
	}

	extendedWeeklyStore := newBacktestReplayStore(reopenedStore, boolPtr(true))
	extendedRows, err := extendedWeeklyStore.QueryKLinesBackward(nil, "US.AAPL", types.Interval1w, time.Date(2026, time.January, 12, 0, 0, 0, 0, time.UTC), 1)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(extended 1w) error = %v", err)
	}
	if len(extendedRows) != 1 {
		t.Fatalf("extended weekly rows len = %d, want 1", len(extendedRows))
	}
	if extendedRows[0].Open.Compare(fixedpoint.NewFromFloat(90)) != 0 {
		t.Fatalf("extended weekly open = %s, want 90", extendedRows[0].Open.String())
	}
	if extendedRows[0].High.Compare(fixedpoint.NewFromFloat(112)) != 0 {
		t.Fatalf("extended weekly high = %s, want 112", extendedRows[0].High.String())
	}
	if extendedRows[0].Low.Compare(fixedpoint.NewFromFloat(89)) != 0 {
		t.Fatalf("extended weekly low = %s, want 89", extendedRows[0].Low.String())
	}
	if extendedRows[0].Close.Compare(fixedpoint.NewFromFloat(111)) != 0 {
		t.Fatalf("extended weekly close = %s, want 111", extendedRows[0].Close.String())
	}
	if extendedRows[0].Volume.Compare(fixedpoint.NewFromFloat(3300)) != 0 {
		t.Fatalf("extended weekly volume = %s, want 3300", extendedRows[0].Volume.String())
	}
}

func TestSessionFilteredBacktestStoreSynthesizesUSTwoHourWithPreMarketWhenExtendedEnabled(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dbPath := filepath.Join(t.TempDir(), "backtest-extended-2h.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}

	starts := []time.Time{
		time.Date(2026, time.January, 8, 13, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 8, 13, 30, 0, 0, time.UTC),
		time.Date(2026, time.January, 8, 14, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 8, 14, 30, 0, 0, time.UTC),
		time.Date(2026, time.January, 8, 15, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 8, 15, 30, 0, 0, time.UTC),
		time.Date(2026, time.January, 8, 16, 0, 0, 0, time.UTC),
	}
	baseRows := make([]types.KLine, 0, len(starts))
	for index, startAt := range starts {
		openValue := 90 + float64(index)
		baseRows = append(baseRows, types.KLine{
			StartTime: types.Time(startAt),
			EndTime:   types.Time(startAt.Add(30*time.Minute - time.Millisecond)),
			Interval:  types.Interval30m,
			Symbol:    "US.AAPL",
			Open:      fixedpoint.NewFromFloat(openValue),
			High:      fixedpoint.NewFromFloat(openValue + 1),
			Low:       fixedpoint.NewFromFloat(openValue - 1),
			Close:     fixedpoint.NewFromFloat(openValue + 0.5),
			Volume:    fixedpoint.NewFromFloat(100 + float64(index)*10),
		})
	}
	if err := store.InsertKLines(baseRows, "forward"); err != nil {
		_ = store.Close()
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	reopenedStore, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore(reopen) error = %v", err)
	}
	defer reopenedStore.Close()
	reopenedStore.SetRehabType("forward")

	regularStore := newBacktestReplayStore(reopenedStore, boolPtr(false))
	regularRows, err := regularStore.QueryKLinesBackward(nil, "US.AAPL", types.Interval2h, time.Date(2026, time.January, 8, 16, 30, 0, 0, time.UTC), 2)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(regular 2h) error = %v", err)
	}
	if len(regularRows) != 1 {
		t.Fatalf("regular 2h rows len = %d, want 1", len(regularRows))
	}
	if !regularRows[0].StartTime.Time().Equal(time.Date(2026, time.January, 8, 14, 30, 0, 0, time.UTC)) {
		t.Fatalf("regular 2h start = %s, want 2026-01-08T14:30:00Z", regularRows[0].StartTime.Time())
	}
	if regularRows[0].Volume.Compare(fixedpoint.NewFromFloat(580)) != 0 {
		t.Fatalf("regular 2h volume = %s, want 580", regularRows[0].Volume.String())
	}

	extendedStore := newBacktestReplayStore(reopenedStore, boolPtr(true))
	extendedRows, err := extendedStore.QueryKLinesBackward(nil, "US.AAPL", types.Interval2h, time.Date(2026, time.January, 8, 16, 30, 0, 0, time.UTC), 2)
	if err != nil {
		t.Fatalf("QueryKLinesBackward(extended 2h) error = %v", err)
	}
	if len(extendedRows) != 2 {
		t.Fatalf("extended 2h rows len = %d, want 2", len(extendedRows))
	}
	if !extendedRows[0].StartTime.Time().Equal(time.Date(2026, time.January, 8, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("extended pre-market 2h start = %s, want 2026-01-08T13:00:00Z", extendedRows[0].StartTime.Time())
	}
	if !extendedRows[1].StartTime.Time().Equal(time.Date(2026, time.January, 8, 14, 30, 0, 0, time.UTC)) {
		t.Fatalf("extended regular 2h start = %s, want 2026-01-08T14:30:00Z", extendedRows[1].StartTime.Time())
	}
	if extendedRows[0].Volume.Compare(fixedpoint.NewFromFloat(330)) != 0 {
		t.Fatalf("extended pre-market 2h volume = %s, want 330", extendedRows[0].Volume.String())
	}
	if extendedRows[1].Volume.Compare(fixedpoint.NewFromFloat(580)) != 0 {
		t.Fatalf("extended regular 2h volume = %s, want 580", extendedRows[1].Volume.String())
	}

	extendedCh, extendedErrCh := extendedStore.QueryKLinesCh(
		time.Date(2026, time.January, 8, 13, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 8, 16, 29, 59, 999000000, time.UTC),
		nil,
		[]string{"US.AAPL"},
		[]types.Interval{types.Interval2h},
	)
	extendedChannelRows, err := collectKLinesFromChannels(extendedCh, extendedErrCh)
	if err != nil {
		t.Fatalf("QueryKLinesCh(extended 2h) error = %v", err)
	}
	if len(extendedChannelRows) != 2 {
		t.Fatalf("extended 2h channel rows len = %d, want 2", len(extendedChannelRows))
	}
	if !extendedChannelRows[0].StartTime.Time().Equal(time.Date(2026, time.January, 8, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("extended channel pre-market 2h start = %s, want 2026-01-08T13:00:00Z", extendedChannelRows[0].StartTime.Time())
	}
	if !extendedChannelRows[1].StartTime.Time().Equal(time.Date(2026, time.January, 8, 14, 30, 0, 0, time.UTC)) {
		t.Fatalf("extended channel regular 2h start = %s, want 2026-01-08T14:30:00Z", extendedChannelRows[1].StartTime.Time())
	}
}

func collectKLinesFromChannels(ch chan types.KLine, errCh chan error) ([]types.KLine, error) {
	rows := make([]types.KLine, 0)
	for ch != nil || errCh != nil {
		select {
		case kline, ok := <-ch:
			if !ok {
				ch = nil
				continue
			}
			rows = append(rows, kline)
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if err != nil {
				return nil, err
			}
		}
	}
	return rows, nil
}

func collectKLinesFromStreamer(store any, since, until time.Time, symbols []string, intervals []types.Interval) ([]types.KLine, error) {
	streamer, ok := store.(klineRangeStreamer)
	if !ok {
		return nil, fmt.Errorf("store does not implement klineRangeStreamer")
	}
	rows := make([]types.KLine, 0)
	err := streamer.StreamKLines(since, until, nil, symbols, intervals, func(kline types.KLine) {
		rows = append(rows, kline)
	})
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func boolPtr(value bool) *bool {
	return &value
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
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StartTime:    klines[1].StartTime.Time(),
		EndTime:      klines[3].EndTime.Time(),
		StrategyScript: `//@version=6
strategy("Pine Cross", overlay=true)
fast = ta.sma(close, 1)
slow = ta.sma(close, 2)
strategy.entry("Long", strategy.long, qty=1)`,
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
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StartTime:    baseStart.Add(5 * time.Minute),
		EndTime:      baseStart.Add(15*time.Minute - time.Millisecond),
		StrategyScript: `//@version=6
strategy("Pine Five Minute", overlay=true)
log.info("pine 5m kline")`,
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
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StartTime:    baseStart.Add(5 * time.Minute),
		EndTime:      baseStart.Add(15*time.Minute - time.Millisecond),
		StrategyScript: `//@version=6
strategy("Pine Five Minute Missing", overlay=true)
log.info("pine 5m init")`,
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
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StartTime:    baseStart.Add(15 * time.Minute),
		EndTime:      baseStart.Add(30*time.Minute - time.Millisecond),
		StrategyScript: `//@version=6
strategy("Pine Fifteen Minute", overlay=true)
log.info("pine 15m kline")`,
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
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StartTime:    currentKLine.StartTime.Time(),
		EndTime:      currentKLine.EndTime.Time(),
		StrategyScript: `//@version=6
strategy("Pine Auto Warmup", overlay=true)
slow = ta.sma(close, 20)
log.info("auto warmup")`,
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
		SourceFormat:   strategydefinition.SourceFormatPineV6,
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
