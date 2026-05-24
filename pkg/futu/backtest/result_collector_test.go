package backtest

import (
	"context"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

type stubAccountQuerier struct {
	account *types.Account
	err     error
}

func (s stubAccountQuerier) QueryAccount(context.Context) (*types.Account, error) {
	return s.account, s.err
}

func TestResultCollectorBuildsTradesAndFinalStats(t *testing.T) {
	warmupUntil := time.Date(2026, time.May, 25, 9, 0, 0, 0, time.UTC)
	result := &RunResult{}
	collector := newResultCollector("BTCUSDT", types.Interval("1m"), "USDT", warmupUntil, result)

	collector.onOrderUpdate(types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol:   "BTCUSDT",
			Side:     types.SideTypeBuy,
			Quantity: fixedpoint.NewFromFloat(0.5),
		},
		OrderID: 1,
		Status:  types.OrderStatusNew,
	})
	collector.onOrderUpdate(types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol:       "BTCUSDT",
			Side:         types.SideTypeBuy,
			Quantity:     fixedpoint.NewFromFloat(1),
			AveragePrice: fixedpoint.NewFromFloat(100),
		},
		OrderID:    2,
		Status:     types.OrderStatusFilled,
		UpdateTime: types.Time(warmupUntil.Add(time.Minute)),
	})
	collector.onOrderUpdate(types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol:       "BTCUSDT",
			Side:         types.SideTypeSell,
			Quantity:     fixedpoint.NewFromFloat(1),
			AveragePrice: fixedpoint.NewFromFloat(110),
		},
		OrderID:    3,
		Status:     types.OrderStatusFilled,
		UpdateTime: types.Time(warmupUntil.Add(2 * time.Minute)),
	})

	account := types.NewAccount()
	account.SetBalance("USDT", types.Balance{Currency: "USDT", Available: fixedpoint.NewFromFloat(1000)})
	querier := stubAccountQuerier{account: account}

	collector.onKLineClosed(context.Background(), querier, types.KLine{
		Symbol:   "BTCUSDT",
		Interval: types.Interval("1m"),
		EndTime:  types.Time(warmupUntil.Add(-time.Minute)),
		Open:     fixedpoint.NewFromFloat(90),
		High:     fixedpoint.NewFromFloat(95),
		Low:      fixedpoint.NewFromFloat(85),
		Close:    fixedpoint.NewFromFloat(92),
		Volume:   fixedpoint.NewFromFloat(10),
	})
	collector.onKLineClosed(context.Background(), querier, types.KLine{
		Symbol:   "BTCUSDT",
		Interval: types.Interval("1m"),
		EndTime:  types.Time(warmupUntil.Add(3 * time.Minute)),
		Open:     fixedpoint.NewFromFloat(110),
		High:     fixedpoint.NewFromFloat(120),
		Low:      fixedpoint.NewFromFloat(108),
		Close:    fixedpoint.NewFromFloat(118),
		Volume:   fixedpoint.NewFromFloat(12),
	})

	totalOrders, filledOrders := collector.finalize(context.Background(), querier, 1000)
	if totalOrders != 3 {
		t.Fatalf("totalOrders = %d", totalOrders)
	}
	if filledOrders != 2 {
		t.Fatalf("filledOrders = %d", filledOrders)
	}
	if result.FinalBalance != 1000 {
		t.Fatalf("finalBalance = %f", result.FinalBalance)
	}
	if result.PnL != 0 {
		t.Fatalf("pnl = %f", result.PnL)
	}
	if result.TotalTrades != 2 {
		t.Fatalf("totalTrades = %d", result.TotalTrades)
	}
	if result.WinRate != 0.5 {
		t.Fatalf("winRate = %f", result.WinRate)
	}
	if len(result.Trades) != 2 {
		t.Fatalf("trades len = %d", len(result.Trades))
	}
	if len(result.PnLCurve) != 1 {
		t.Fatalf("pnl curve len = %d", len(result.PnLCurve))
	}
	if len(result.Candles) != 1 {
		t.Fatalf("candles len = %d", len(result.Candles))
	}
}

func TestResultCollectorWarnsOnNonPositiveCloseOnce(t *testing.T) {
	warmupUntil := time.Date(2026, time.May, 25, 9, 0, 0, 0, time.UTC)
	result := &RunResult{}
	collector := newResultCollector("BTCUSDT", types.Interval("1m"), "USDT", warmupUntil, result)
	collector.onOrderUpdate(types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol:       "BTCUSDT",
			Side:         types.SideTypeBuy,
			Quantity:     fixedpoint.NewFromFloat(1),
			AveragePrice: fixedpoint.NewFromFloat(100),
		},
		OrderID:    9,
		Status:     types.OrderStatusFilled,
		UpdateTime: types.Time(warmupUntil.Add(time.Minute)),
	})

	account := types.NewAccount()
	account.SetBalance("USDT", types.Balance{Currency: "USDT", Available: fixedpoint.NewFromFloat(1000)})
	querier := stubAccountQuerier{account: account}

	kline := types.KLine{
		Symbol:   "BTCUSDT",
		Interval: types.Interval("1m"),
		EndTime:  types.Time(warmupUntil.Add(2 * time.Minute)),
		Open:     fixedpoint.NewFromFloat(100),
		High:     fixedpoint.NewFromFloat(101),
		Low:      fixedpoint.NewFromFloat(99),
		Close:    fixedpoint.Zero,
		Volume:   fixedpoint.NewFromFloat(10),
	}
	collector.onKLineClosed(context.Background(), querier, kline)
	collector.onKLineClosed(context.Background(), querier, kline)

	if len(result.RuntimeErrors) != 1 {
		t.Fatalf("runtimeErrors len = %d", len(result.RuntimeErrors))
	}
	if len(result.PnLCurve) != 0 {
		t.Fatalf("run result pnl curve should not be populated before finalize, got %d", len(result.PnLCurve))
	}
}
