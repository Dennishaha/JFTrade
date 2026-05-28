package jftradeapi

import (
	"testing"

	"github.com/jftrade/jftrade-main/pkg/backtest"
)

func TestBacktestRunStoreGetReturnsDeepCopy(t *testing.T) {
	store := newBacktestRunStore()
	original := &backtestRunState{
		ID:     "bt-1",
		Status: "completed",
		Request: backtestStartRequest{
			DefinitionID: "def-1",
			Symbol:       "HK.00700",
		},
		Result: &backtest.RunResult{
			Symbol:          "HK.00700",
			Interval:        "1m",
			FinalBalance:    123456,
			MaxDrawdown:     0.12,
			CurrentDrawdown: 0.03,
			Trades:          []backtest.TradeEvent{{Time: "2026-01-02T00:00:00Z", Side: "BUY", Price: 100, Qty: 1}},
			OrderBook:       []backtest.OrderBookEntry{{OrderID: "1", Side: "BUY", Quantity: 1, Status: "FILLED", FilledPrice: 100}},
			PnLCurve:        []backtest.PnLPoint{{Time: "2026-01-02T00:00:00Z", Equity: 100000}},
			DrawdownCurve:   []backtest.DrawdownPoint{{Time: "2026-01-02T00:00:00Z", Drawdown: 0.12}},
			Candles:         []backtest.Candle{{Time: "2026-01-02T00:00:00Z", Open: 100, High: 101, Low: 99, Close: 100.5, Volume: 10}},
			Logs:            []string{"warmup complete"},
			RuntimeErrors:   []string{"risk warning"},
		},
	}
	store.add(original)

	snapshot, ok := store.get(original.ID)
	if !ok {
		t.Fatal("expected run snapshot")
	}

	snapshot.Status = "failed"
	snapshot.Request.Symbol = "US.TSLA"
	snapshot.Result.FinalBalance = 42
	snapshot.Result.MaxDrawdown = 0.5
	snapshot.Result.CurrentDrawdown = 0.4
	snapshot.Result.Trades[0].Price = 999
	snapshot.Result.OrderBook[0].FilledPrice = 77
	snapshot.Result.PnLCurve[0].Equity = 12
	snapshot.Result.DrawdownCurve[0].Drawdown = 0.8
	snapshot.Result.Candles[0].Close = 1
	snapshot.Result.Logs[0] = "changed"
	snapshot.Result.RuntimeErrors[0] = "changed"

	if original.Status != "completed" {
		t.Fatalf("original status mutated: %s", original.Status)
	}
	if original.Request.Symbol != "HK.00700" {
		t.Fatalf("original request symbol mutated: %s", original.Request.Symbol)
	}
	if original.Result.FinalBalance != 123456 {
		t.Fatalf("original final balance mutated: %f", original.Result.FinalBalance)
	}
	if original.Result.MaxDrawdown != 0.12 {
		t.Fatalf("original max drawdown mutated: %f", original.Result.MaxDrawdown)
	}
	if original.Result.CurrentDrawdown != 0.03 {
		t.Fatalf("original current drawdown mutated: %f", original.Result.CurrentDrawdown)
	}
	if original.Result.Trades[0].Price != 100 {
		t.Fatalf("original trade mutated: %f", original.Result.Trades[0].Price)
	}
	if original.Result.OrderBook[0].FilledPrice != 100 {
		t.Fatalf("original order book mutated: %f", original.Result.OrderBook[0].FilledPrice)
	}
	if original.Result.PnLCurve[0].Equity != 100000 {
		t.Fatalf("original pnl point mutated: %f", original.Result.PnLCurve[0].Equity)
	}
	if original.Result.DrawdownCurve[0].Drawdown != 0.12 {
		t.Fatalf("original drawdown point mutated: %f", original.Result.DrawdownCurve[0].Drawdown)
	}
	if original.Result.Candles[0].Close != 100.5 {
		t.Fatalf("original candle mutated: %f", original.Result.Candles[0].Close)
	}
	if original.Result.Logs[0] != "warmup complete" {
		t.Fatalf("original logs mutated: %s", original.Result.Logs[0])
	}
	if original.Result.RuntimeErrors[0] != "risk warning" {
		t.Fatalf("original runtime errors mutated: %s", original.Result.RuntimeErrors[0])
	}
}

func TestBacktestRunStoreListReturnsIndependentSnapshots(t *testing.T) {
	store := newBacktestRunStore()
	original := &backtestRunState{
		ID:     "bt-2",
		Status: "queued",
		Result: &backtest.RunResult{
			Logs: []string{"queued"},
		},
	}
	store.add(original)

	runs := store.list()
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}

	runs[0].Status = "running"
	runs[0].Result.Logs[0] = "mutated"

	fresh, ok := store.get(original.ID)
	if !ok {
		t.Fatal("expected run snapshot")
	}
	if fresh.Status != "queued" {
		t.Fatalf("store status mutated through list snapshot: %s", fresh.Status)
	}
	if fresh.Result.Logs[0] != "queued" {
		t.Fatalf("store logs mutated through list snapshot: %s", fresh.Result.Logs[0])
	}
}
