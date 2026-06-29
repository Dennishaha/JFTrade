package backtest

import (
	"context"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

func TestPineWorkerShortReplayExecutorFillsShortEntryAndCover(t *testing.T) {
	account := types.NewAccount()
	account.UpdateBalances(types.BalanceMap{
		"USD": {Currency: "USD", Available: fixedpoint.NewFromFloat(1000)},
	})
	stream := types.NewStandardStream()
	var orders []types.Order
	var trades []types.Trade
	stream.OnOrderUpdate(func(order types.Order) { orders = append(orders, order) })
	stream.OnTradeUpdate(func(trade types.Trade) { trades = append(trades, trade) })

	executor := newPineWorkerShortReplayExecutor(&fakeWorkerOrderExecutor{}, account, &stream)
	executor.onKLineClosed(testPineWorkerShortReplayKLine(time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC), 100))

	created, err := executor.SubmitOrders(context.Background(), types.SubmitOrder{
		ClientOrderID: "ES",
		Symbol:        "US.AAPL",
		Side:          types.SideTypeSell,
		Type:          types.OrderTypeMarket,
		Quantity:      fixedpoint.NewFromFloat(2),
		Market:        testPineWorkerShortReplayMarket(),
		Tag:           pineWorkerShortReplayOrderTag,
	})
	if err != nil {
		t.Fatalf("short entry SubmitOrders error = %v", err)
	}
	if len(created) != 1 || created[0].Status != types.OrderStatusFilled || !created[0].IsMargin {
		t.Fatalf("created short entry = %#v", created)
	}
	balance, _ := account.Balance("USD")
	if balance.Available.Float64() != 1200 {
		t.Fatalf("USD after short entry = %s, want 1200", balance.Available)
	}

	executor.onKLineClosed(testPineWorkerShortReplayKLine(time.Date(2026, time.June, 29, 9, 31, 0, 0, time.UTC), 90))
	created, err = executor.SubmitOrders(context.Background(), types.SubmitOrder{
		ClientOrderID: "XS",
		Symbol:        "US.AAPL",
		Side:          types.SideTypeBuy,
		Type:          types.OrderTypeMarket,
		Quantity:      fixedpoint.NewFromFloat(2),
		Market:        testPineWorkerShortReplayMarket(),
		Tag:           pineWorkerShortReplayOrderTag,
	})
	if err != nil {
		t.Fatalf("short cover SubmitOrders error = %v", err)
	}
	if len(created) != 1 || created[0].Status != types.OrderStatusFilled || created[0].Side != types.SideTypeBuy {
		t.Fatalf("created cover = %#v", created)
	}
	balance, _ = account.Balance("USD")
	if balance.Available.Float64() != 1020 {
		t.Fatalf("USD after cover = %s, want 1020", balance.Available)
	}
	if len(orders) != 4 {
		t.Fatalf("order updates len = %d, want 4", len(orders))
	}
	if len(trades) != 2 || trades[0].Side != types.SideTypeSell || trades[1].Side != types.SideTypeBuy {
		t.Fatalf("trades = %#v", trades)
	}
}

func testPineWorkerShortReplayMarket() types.Market {
	return types.Market{
		Exchange:      types.ExchangeBacktest,
		Symbol:        "US.AAPL",
		BaseCurrency:  "AAPL",
		QuoteCurrency: "USD",
		MinQuantity:   fixedpoint.NewFromFloat(0.0001),
		MinNotional:   fixedpoint.NewFromFloat(1),
	}
}

func testPineWorkerShortReplayKLine(start time.Time, closePrice float64) types.KLine {
	price := fixedpoint.NewFromFloat(closePrice)
	return types.KLine{
		Symbol:    "US.AAPL",
		Interval:  types.Interval1m,
		StartTime: types.Time(start),
		EndTime:   types.Time(start.Add(time.Minute - time.Millisecond)),
		Open:      price,
		High:      price,
		Low:       price,
		Close:     price,
		Volume:    fixedpoint.NewFromFloat(1000),
	}
}
