package backtest

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"

	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

func TestRunWithPineWorkerExecutesReplayThroughGoMatching(t *testing.T) {
	isolateBacktestHome(t)

	dbPath := filepath.Join(t.TempDir(), "pinets-worker-backtest.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	baseStart := time.Date(2026, time.June, 29, 13, 30, 0, 0, time.UTC)
	klines := []types.KLine{
		testPineWorkerRunnerKLine(baseStart, 100),
		testPineWorkerRunnerKLine(baseStart.Add(time.Minute), 101),
		testPineWorkerRunnerKLine(baseStart.Add(2*time.Minute), 110),
		testPineWorkerRunnerKLine(baseStart.Add(3*time.Minute), 111),
	}
	if err := store.InsertKLines(klines, "forward"); err != nil {
		jftradeCheckTestError(t, store.Close())
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	runner := &fakePineWorkerBacktestRunner{
		response: pineworker.RunScriptResponse{
			OrderIntents: []pineworker.OrderIntent{
				{Kind: "entry", ID: "long", Direction: "long", Quantity: 1, HasQuantity: true, LimitPrice: 101, HasLimitPrice: true, BarIndex: 0},
				{Kind: "close", ID: "close-long", Direction: "long", Quantity: 1, HasQuantity: true, BarIndex: 2},
			},
			Metadata: pineworker.WorkerMetadata{WorkerID: "worker-1"},
		},
	}
	result := RunWithPineWorker(context.Background(), RunConfig{
		DBPath:       dbPath,
		Symbol:       "US.AAPL",
		Interval:     string(types.Interval1m),
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StartTime:    klines[0].StartTime.Time(),
		EndTime:      klines[len(klines)-1].EndTime.Time(),
		StrategyScript: `//@version=6
strategy("worker smoke")`,
		InitialBalance: 10000,
	}, runner)

	if result == nil {
		t.Fatal("RunWithPineWorker returned nil")
	}
	if result.Error != "" {
		t.Fatalf("RunWithPineWorker error = %s", result.Error)
	}
	if runner.request.Mode != pineworker.ModeBacktest || len(runner.request.Candles) != len(klines) {
		t.Fatalf("worker request = %#v", runner.request)
	}
	if result.QuoteCurrency != "USD" {
		t.Fatalf("QuoteCurrency = %s, want USD", result.QuoteCurrency)
	}
	if result.TotalTrades == 0 {
		t.Fatalf("TotalTrades = %d, want fills", result.TotalTrades)
	}
	if len(result.OrderBook) == 0 {
		t.Fatal("OrderBook is empty, want submitted worker orders")
	}
	if len(result.Candles) != len(klines)-1 {
		t.Fatalf("Candles len = %d, want %d", len(result.Candles), len(klines)-1)
	}
}

func TestRunWithPineWorkerExecutesQuantityPctReplay(t *testing.T) {
	isolateBacktestHome(t)

	dbPath := filepath.Join(t.TempDir(), "pinets-worker-quantity-pct.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	baseStart := time.Date(2026, time.June, 29, 13, 30, 0, 0, time.UTC)
	klines := []types.KLine{
		testPineWorkerRunnerKLine(baseStart, 100),
		testPineWorkerRunnerKLine(baseStart.Add(time.Minute), 100),
		testPineWorkerRunnerKLine(baseStart.Add(2*time.Minute), 100),
		testPineWorkerRunnerKLine(baseStart.Add(3*time.Minute), 100),
	}
	if err := store.InsertKLines(klines, "forward"); err != nil {
		jftradeCheckTestError(t, store.Close())
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	runner := &fakePineWorkerBacktestRunner{
		response: pineworker.RunScriptResponse{
			OrderIntents: []pineworker.OrderIntent{
				{Kind: "entry", ID: "half-equity", Direction: "long", QuantityPct: 50, HasQuantityPct: true, BarIndex: 0},
				{Kind: "close", ID: "half-position", Direction: "long", QuantityPct: 50, HasQuantityPct: true, BarIndex: 2},
			},
			Metadata: pineworker.WorkerMetadata{WorkerID: "worker-1"},
		},
	}
	result := RunWithPineWorker(context.Background(), RunConfig{
		DBPath:       dbPath,
		Symbol:       "US.AAPL",
		Interval:     string(types.Interval1m),
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StartTime:    klines[0].StartTime.Time(),
		EndTime:      klines[len(klines)-1].EndTime.Time(),
		StrategyScript: `//@version=6
strategy("worker qty pct")`,
		InitialBalance: 10000,
	}, runner)

	if result == nil {
		t.Fatal("RunWithPineWorker returned nil")
	}
	if result.Error != "" {
		t.Fatalf("RunWithPineWorker error = %s", result.Error)
	}
	if result.TotalTrades != 2 {
		t.Fatalf("TotalTrades = %d, want 2", result.TotalTrades)
	}
	entry, ok := findOrderBookEntry(result.OrderBook, "half-equity")
	if !ok {
		t.Fatalf("entry order not found in %#v", result.OrderBook)
	}
	if entry.Quantity != "50" || entry.FilledQuantity != "50" {
		t.Fatalf("entry quantities = %#v, want 50", entry)
	}
	closeOrder, ok := findOrderBookEntry(result.OrderBook, "half-position")
	if !ok {
		t.Fatalf("close order not found in %#v", result.OrderBook)
	}
	if closeOrder.Quantity != "25" || closeOrder.FilledQuantity != "25" {
		t.Fatalf("close quantities = %#v, want 25", closeOrder)
	}
}

func TestRunWithPineWorkerWarnsAndIgnoresInitialCloseSignal(t *testing.T) {
	isolateBacktestHome(t)

	dbPath := filepath.Join(t.TempDir(), "pinets-worker-initial-close.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	baseStart := time.Date(2026, time.June, 29, 13, 30, 0, 0, time.UTC)
	klines := []types.KLine{
		testPineWorkerRunnerKLine(baseStart, 100),
		testPineWorkerRunnerKLine(baseStart.Add(time.Minute), 101),
	}
	if err := store.InsertKLines(klines, "forward"); err != nil {
		jftradeCheckTestError(t, store.Close())
		t.Fatalf("InsertKLines() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	runner := &fakePineWorkerBacktestRunner{
		response: pineworker.RunScriptResponse{
			OrderIntents: []pineworker.OrderIntent{
				{Kind: "close", ID: "initial-sell", Direction: "long", Quantity: 1, HasQuantity: true, BarIndex: 0},
			},
			Metadata: pineworker.WorkerMetadata{WorkerID: "worker-1"},
		},
	}
	result := RunWithPineWorker(context.Background(), RunConfig{
		DBPath:       dbPath,
		Symbol:       "US.AAPL",
		Interval:     string(types.Interval1m),
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StartTime:    klines[0].StartTime.Time(),
		EndTime:      klines[len(klines)-1].EndTime.Time(),
		StrategyScript: `//@version=6
strategy("worker initial close")`,
		InitialBalance: 10000,
	}, runner)

	if result == nil {
		t.Fatal("RunWithPineWorker returned nil")
	}
	if result.Error != "" {
		t.Fatalf("RunWithPineWorker error = %s", result.Error)
	}
	if result.IgnoredOrders != 1 || result.WarningTotal != 1 || len(result.Warnings) != 1 {
		t.Fatalf("warnings ignored=%d total=%d list=%#v", result.IgnoredOrders, result.WarningTotal, result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "ignored close command") {
		t.Fatalf("warning = %q", result.Warnings[0])
	}
	if len(result.OrderBook) != 0 {
		t.Fatalf("OrderBook = %#v, want no submitted orders", result.OrderBook)
	}
}

func TestRunWithPineWorkerMapsWorkerErrors(t *testing.T) {
	isolateBacktestHome(t)

	dbPath := filepath.Join(t.TempDir(), "pinets-worker-error.db")
	store, err := NewFutuKLineStore(dbPath)
	if err != nil {
		t.Fatalf("NewFutuKLineStore() error = %v", err)
	}
	baseStart := time.Date(2026, time.June, 29, 13, 30, 0, 0, time.UTC)
	if err := store.InsertKLine(testPineWorkerRunnerKLine(baseStart, 100), "forward"); err != nil {
		jftradeCheckTestError(t, store.Close())
		t.Fatalf("InsertKLine() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("store.Close() error = %v", err)
	}

	result := RunWithPineWorker(context.Background(), RunConfig{
		DBPath:       dbPath,
		Symbol:       "US.AAPL",
		Interval:     string(types.Interval1m),
		SourceFormat: strategydefinition.SourceFormatPineV6,
		StartTime:    baseStart,
		EndTime:      baseStart.Add(time.Minute - time.Millisecond),
		StrategyScript: `//@version=6
strategy("worker error")`,
		InitialBalance: 10000,
	}, &fakePineWorkerBacktestRunner{response: pineworker.RunScriptResponse{Error: "compile failed"}})

	if result == nil || !strings.Contains(result.Error, "compile failed") {
		t.Fatalf("result error = %#v", result)
	}
}

func findOrderBookEntry(entries []OrderBookEntry, clientOrderID string) (OrderBookEntry, bool) {
	for _, entry := range entries {
		if entry.ClientOrderID == clientOrderID {
			return entry, true
		}
	}
	return OrderBookEntry{}, false
}

func testPineWorkerRunnerKLine(start time.Time, closePrice float64) types.KLine {
	price := fixedpoint.NewFromFloat(closePrice)
	return types.KLine{
		StartTime: types.Time(start),
		EndTime:   types.Time(start.Add(time.Minute - time.Millisecond)),
		Interval:  types.Interval1m,
		Symbol:    "US.AAPL",
		Open:      price,
		High:      price.Add(fixedpoint.NewFromFloat(1)),
		Low:       price.Sub(fixedpoint.NewFromFloat(1)),
		Close:     price,
		Volume:    fixedpoint.NewFromFloat(1000),
	}
}
