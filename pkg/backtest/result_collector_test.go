package backtest

import (
	"context"
	"math"
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
			Quantity: fixedpoint.NewFromFloat(1),
		},
		OrderID:    2,
		Status:     types.OrderStatusNew,
		UpdateTime: types.Time(warmupUntil.Add(30 * time.Second)),
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
	if len(result.OrderBook) != 2 {
		t.Fatalf("orderBook len = %d", len(result.OrderBook))
	}
	if result.OrderBook[0].OrderID != "2" {
		t.Fatalf("first order id = %s", result.OrderBook[0].OrderID)
	}
	if result.OrderBook[0].Status != string(types.OrderStatusFilled) {
		t.Fatalf("first order status = %s", result.OrderBook[0].Status)
	}
	if result.OrderBook[0].FilledPrice != 100 {
		t.Fatalf("first order filled price = %f", result.OrderBook[0].FilledPrice)
	}
	if result.OrderBook[0].FilledQuantity != 1 {
		t.Fatalf("first order filled quantity = %f", result.OrderBook[0].FilledQuantity)
	}
	if result.OrderBook[0].SubmittedAt != warmupUntil.Add(30*time.Second).Format(time.RFC3339) {
		t.Fatalf("first order submittedAt = %s", result.OrderBook[0].SubmittedAt)
	}
	if result.OrderBook[0].FilledAt != warmupUntil.Add(time.Minute).Format(time.RFC3339) {
		t.Fatalf("first order filledAt = %s", result.OrderBook[0].FilledAt)
	}
	if len(result.PnLCurve) != 1 {
		t.Fatalf("pnl curve len = %d", len(result.PnLCurve))
	}
	if result.MaxDrawdown != 0 {
		t.Fatalf("maxDrawdown = %f", result.MaxDrawdown)
	}
	if result.CurrentDrawdown != 0 {
		t.Fatalf("currentDrawdown = %f", result.CurrentDrawdown)
	}
	if len(result.DrawdownCurve) != 1 {
		t.Fatalf("drawdown curve len = %d", len(result.DrawdownCurve))
	}
	if result.DrawdownCurve[0].Drawdown != 0 {
		t.Fatalf("drawdown = %f", result.DrawdownCurve[0].Drawdown)
	}
	if len(result.Candles) != 1 {
		t.Fatalf("candles len = %d", len(result.Candles))
	}
}

func TestResultCollectorTracksDrawdownMetrics(t *testing.T) {
	result := &RunResult{}
	collector := newResultCollector("BTCUSDT", types.Interval("1m"), "USDT", time.Time{}, result)
	collector.pnlCurve = []PnLPoint{
		{Time: "2026-05-25T09:00:00Z", Equity: 100},
		{Time: "2026-05-25T09:01:00Z", Equity: 120},
		{Time: "2026-05-25T09:02:00Z", Equity: 90},
		{Time: "2026-05-25T09:03:00Z", Equity: 110},
	}

	account := types.NewAccount()
	account.SetBalance("USDT", types.Balance{Currency: "USDT", Available: fixedpoint.NewFromFloat(110)})
	querier := stubAccountQuerier{account: account}

	collector.finalize(context.Background(), querier, 100)

	if math.Abs(result.MaxDrawdown-0.25) > 1e-9 {
		t.Fatalf("maxDrawdown = %f, want 0.25", result.MaxDrawdown)
	}
	if math.Abs(result.CurrentDrawdown-((120.0-110.0)/120.0)) > 1e-9 {
		t.Fatalf("currentDrawdown = %f", result.CurrentDrawdown)
	}
	if len(result.DrawdownCurve) != 4 {
		t.Fatalf("drawdown curve len = %d", len(result.DrawdownCurve))
	}
	if math.Abs(result.DrawdownCurve[2].Drawdown-0.25) > 1e-9 {
		t.Fatalf("drawdown curve[2] = %f, want 0.25", result.DrawdownCurve[2].Drawdown)
	}
	if math.Abs(result.DrawdownCurve[3].Drawdown-((120.0-110.0)/120.0)) > 1e-9 {
		t.Fatalf("drawdown curve[3] = %f", result.DrawdownCurve[3].Drawdown)
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
