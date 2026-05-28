package backtest

import "testing"

func TestRunResultSnapshotReturnsIndependentCopy(t *testing.T) {
	original := &RunResult{
		Symbol:          "HK.00700",
		Interval:        "1m",
		FinalBalance:    123456,
		MaxDrawdown:     0.12,
		CurrentDrawdown: 0.03,
		Trades:          []TradeEvent{{Time: "2026-01-02T00:00:00Z", Side: "BUY", Price: 100, Qty: 1}},
		OrderBook:       []OrderBookEntry{{OrderID: "1", Side: "BUY", Quantity: 1, Status: "FILLED", FilledPrice: 100}},
		PnLCurve:        []PnLPoint{{Time: "2026-01-02T00:00:00Z", Equity: 100000}},
		DrawdownCurve:   []DrawdownPoint{{Time: "2026-01-02T00:00:00Z", Drawdown: 0.12}},
		Candles:         []Candle{{Time: "2026-01-02T00:00:00Z", Open: 100, High: 101, Low: 99, Close: 100.5, Volume: 10}},
		Logs:            []string{"warmup complete"},
		RuntimeErrors:   []string{"risk warning"},
	}

	snapshot := original.Snapshot()
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}

	snapshot.FinalBalance = 42
	snapshot.MaxDrawdown = 0.5
	snapshot.CurrentDrawdown = 0.4
	snapshot.Trades[0].Price = 999
	snapshot.OrderBook[0].FilledPrice = 88
	snapshot.PnLCurve[0].Equity = 12
	snapshot.DrawdownCurve[0].Drawdown = 0.8
	snapshot.Candles[0].Close = 1
	snapshot.Logs[0] = "changed"
	snapshot.RuntimeErrors[0] = "changed"

	if original.FinalBalance != 123456 {
		t.Fatalf("original final balance mutated: %f", original.FinalBalance)
	}
	if original.MaxDrawdown != 0.12 {
		t.Fatalf("original max drawdown mutated: %f", original.MaxDrawdown)
	}
	if original.CurrentDrawdown != 0.03 {
		t.Fatalf("original current drawdown mutated: %f", original.CurrentDrawdown)
	}
	if original.Trades[0].Price != 100 {
		t.Fatalf("original trade mutated: %f", original.Trades[0].Price)
	}
	if original.OrderBook[0].FilledPrice != 100 {
		t.Fatalf("original order book mutated: %f", original.OrderBook[0].FilledPrice)
	}
	if original.PnLCurve[0].Equity != 100000 {
		t.Fatalf("original pnl point mutated: %f", original.PnLCurve[0].Equity)
	}
	if original.DrawdownCurve[0].Drawdown != 0.12 {
		t.Fatalf("original drawdown point mutated: %f", original.DrawdownCurve[0].Drawdown)
	}
	if original.Candles[0].Close != 100.5 {
		t.Fatalf("original candle mutated: %f", original.Candles[0].Close)
	}
	if original.Logs[0] != "warmup complete" {
		t.Fatalf("original logs mutated: %s", original.Logs[0])
	}
	if original.RuntimeErrors[0] != "risk warning" {
		t.Fatalf("original runtime errors mutated: %s", original.RuntimeErrors[0])
	}
}
