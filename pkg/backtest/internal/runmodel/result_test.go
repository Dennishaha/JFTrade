package runmodel

import (
	"fmt"
	"testing"
)

func TestRunResultSnapshotHandlesNilAndReturnsIndependentCopy(t *testing.T) {
	var nilResult *RunResult
	if snapshot := nilResult.Snapshot(); snapshot != nil {
		t.Fatalf("nil snapshot = %#v, want nil", snapshot)
	}

	original := &RunResult{
		Symbol:                 "US.AAPL",
		Interval:               "1m",
		StartTime:              "2026-06-23T13:30:00Z",
		EndTime:                "2026-06-23T20:00:00Z",
		QuoteCurrency:          "USD",
		FinalBalance:           125000,
		PnL:                    5000,
		MaxDrawdown:            0.08,
		CurrentDrawdown:        0.01,
		TotalTrades:            12,
		WinRate:                0.58,
		Trades:                 []TradeEvent{{Time: "2026-06-23T13:31:00Z", Side: "BUY", Price: "100", Qty: "10"}},
		OrderBook:              []OrderBookEntry{{OrderID: "1", Status: "FILLED", FilledPrice: "100"}},
		PnLCurve:               []PnLPoint{{Time: "2026-06-23T13:31:00Z", Equity: 101000}},
		DrawdownCurve:          []DrawdownPoint{{Time: "2026-06-23T13:32:00Z", Drawdown: 0.02}},
		Candles:                []Candle{{Time: "2026-06-23T13:31:00Z", Close: "100.5"}},
		Logs:                   []string{"warmup complete"},
		Error:                  "transient warning",
		RuntimeErrors:          []string{"partial fill warning"},
		RuntimeErrorCounts:     map[string]int{"partial fill warning": 2},
		RuntimeErrorTotal:      2,
		RuntimeErrorsTruncated: true,
	}

	snapshot := original.Snapshot()
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}

	snapshot.Trades[0].Price = "999"
	snapshot.OrderBook[0].FilledPrice = "77"
	snapshot.PnLCurve[0].Equity = 5
	snapshot.DrawdownCurve[0].Drawdown = 0.9
	snapshot.Candles[0].Close = "1"
	snapshot.Logs[0] = "changed"
	snapshot.RuntimeErrors[0] = "changed"
	snapshot.RuntimeErrorCounts["partial fill warning"] = 9

	if original.Trades[0].Price != "100" {
		t.Fatalf("original trade mutated: %#v", original.Trades)
	}
	if original.OrderBook[0].FilledPrice != "100" {
		t.Fatalf("original order book mutated: %#v", original.OrderBook)
	}
	if original.PnLCurve[0].Equity != 101000 {
		t.Fatalf("original pnl curve mutated: %#v", original.PnLCurve)
	}
	if original.DrawdownCurve[0].Drawdown != 0.02 {
		t.Fatalf("original drawdown curve mutated: %#v", original.DrawdownCurve)
	}
	if original.Candles[0].Close != "100.5" {
		t.Fatalf("original candles mutated: %#v", original.Candles)
	}
	if original.Logs[0] != "warmup complete" {
		t.Fatalf("original logs mutated: %#v", original.Logs)
	}
	if original.RuntimeErrors[0] != "partial fill warning" {
		t.Fatalf("original runtime errors mutated: %#v", original.RuntimeErrors)
	}
	if original.RuntimeErrorCounts["partial fill warning"] != 2 {
		t.Fatalf("original runtime error counts mutated: %#v", original.RuntimeErrorCounts)
	}
}

func TestRunResultSnapshotOmitsEmptyRuntimeErrorCounts(t *testing.T) {
	snapshot := (&RunResult{Symbol: "US.AAPL"}).Snapshot()
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}
	if snapshot.RuntimeErrorCounts != nil {
		t.Fatalf("RuntimeErrorCounts = %#v, want nil for empty map", snapshot.RuntimeErrorCounts)
	}
}

func TestAddRuntimeErrorReusesExistingSamplesAndCapsUniqueList(t *testing.T) {
	result := &RunResult{
		RuntimeErrors:      []string{"preloaded warning"},
		RuntimeErrorCounts: map[string]int{"preloaded warning": 1},
	}

	result.AddRuntimeError("preloaded warning")
	result.AddRuntimeError("new warning")
	for index := range 101 {
		result.AddRuntimeError(fmt.Sprintf("unique-%03d", index))
	}

	if result.RuntimeErrorTotal != 103 {
		t.Fatalf("RuntimeErrorTotal = %d, want 103", result.RuntimeErrorTotal)
	}
	if result.RuntimeErrorCounts["preloaded warning"] != 2 {
		t.Fatalf("preloaded warning count = %d, want 2", result.RuntimeErrorCounts["preloaded warning"])
	}
	if result.RuntimeErrorCounts["new warning"] != 1 {
		t.Fatalf("new warning count = %d, want 1", result.RuntimeErrorCounts["new warning"])
	}
	if len(result.RuntimeErrors) != 100 {
		t.Fatalf("RuntimeErrors len = %d, want 100", len(result.RuntimeErrors))
	}
	if result.RuntimeErrors[0] != "preloaded warning" || result.RuntimeErrors[1] != "new warning" {
		t.Fatalf("RuntimeErrors prefix = %#v", result.RuntimeErrors[:2])
	}
	if result.RuntimeErrors[len(result.RuntimeErrors)-1] != "unique-097" {
		t.Fatalf("last sampled runtime error = %q, want unique-097", result.RuntimeErrors[len(result.RuntimeErrors)-1])
	}
	if !result.RuntimeErrorsTruncated {
		t.Fatal("expected RuntimeErrorsTruncated after unique sample cap")
	}
}
