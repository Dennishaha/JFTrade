package backtest

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestResultCollectorClosedTradeStatsUseWeightedPositionCost(t *testing.T) {
	result, collector := newTradeStatsCollector()
	at := time.Date(2026, time.July, 21, 13, 30, 0, 0, time.UTC)

	collector.onOrderUpdate(tradeStatsOrder(1, types.SideTypeBuy, 2, 2, 100, types.OrderStatusFilled, at))
	collector.onOrderUpdate(tradeStatsOrder(2, types.SideTypeBuy, 3, 3, 110, types.OrderStatusFilled, at.Add(time.Minute)))
	collector.onOrderUpdate(tradeStatsOrder(3, types.SideTypeSell, 5, 5, 105, types.OrderStatusFilled, at.Add(2*time.Minute)))
	finalizeTradeStatsCollector(t, collector)

	assertTradeStats(t, result, 3, 1, 0)
	if got := result.Trades[2].PnL; math.Abs(got-(-5)) > 1e-9 {
		t.Fatalf("closing PnL = %f, want -5 from weighted average cost 106", got)
	}
}

func TestResultCollectorClosedTradeStatsHandleLongShortAndReversal(t *testing.T) {
	t.Run("short covers", func(t *testing.T) {
		result, collector := newTradeStatsCollector()
		at := time.Date(2026, time.July, 21, 13, 30, 0, 0, time.UTC)

		collector.onOrderUpdate(tradeStatsOrder(1, types.SideTypeSell, 2, 2, 100, types.OrderStatusFilled, at))
		collector.onOrderUpdate(tradeStatsOrder(2, types.SideTypeBuy, 1, 1, 90, types.OrderStatusFilled, at.Add(time.Minute)))
		collector.onOrderUpdate(tradeStatsOrder(3, types.SideTypeBuy, 1, 1, 110, types.OrderStatusFilled, at.Add(2*time.Minute)))
		finalizeTradeStatsCollector(t, collector)

		assertTradeStats(t, result, 3, 2, 0.5)
		if result.Trades[1].PnL != 10 || result.Trades[2].PnL != -10 {
			t.Fatalf("short-cover PnL = %f/%f, want 10/-10", result.Trades[1].PnL, result.Trades[2].PnL)
		}
	})

	t.Run("sell reversal", func(t *testing.T) {
		result, collector := newTradeStatsCollector()
		at := time.Date(2026, time.July, 21, 13, 30, 0, 0, time.UTC)

		collector.onOrderUpdate(tradeStatsOrder(1, types.SideTypeBuy, 3, 3, 100, types.OrderStatusFilled, at))
		collector.onOrderUpdate(tradeStatsOrder(2, types.SideTypeSell, 5, 5, 90, types.OrderStatusFilled, at.Add(time.Minute)))
		if collector.netPosition.String() != "-2" || collector.averageEntryPrice.String() != "90" {
			t.Fatalf("reversed position = %s @ %s, want -2 @ 90", collector.netPosition, collector.averageEntryPrice)
		}
		collector.onOrderUpdate(tradeStatsOrder(3, types.SideTypeBuy, 2, 2, 80, types.OrderStatusFilled, at.Add(2*time.Minute)))
		finalizeTradeStatsCollector(t, collector)

		assertTradeStats(t, result, 3, 2, 0.5)
		if result.Trades[1].PnL != -30 || result.Trades[2].PnL != 20 {
			t.Fatalf("reversal PnL = %f/%f, want -30/20", result.Trades[1].PnL, result.Trades[2].PnL)
		}
	})
}

func TestResultCollectorClosedTradeStatsAggregatePartialClosingOrder(t *testing.T) {
	result, collector := newTradeStatsCollector()
	at := time.Date(2026, time.July, 21, 13, 30, 0, 0, time.UTC)

	collector.onOrderUpdate(tradeStatsOrder(1, types.SideTypeBuy, 10, 10, 100, types.OrderStatusFilled, at))
	collector.onOrderUpdate(tradeStatsOrder(2, types.SideTypeSell, 10, 4, 110, types.OrderStatusPartiallyFilled, at.Add(time.Minute)))
	if collector.closedTrades != 0 {
		t.Fatalf("closedTrades after partial update = %d, want pending order aggregation", collector.closedTrades)
	}
	collector.onOrderUpdate(tradeStatsOrder(2, types.SideTypeSell, 10, 10, 104, types.OrderStatusFilled, at.Add(2*time.Minute)))
	finalizeTradeStatsCollector(t, collector)

	assertTradeStats(t, result, 3, 1, 1)
	if result.Trades[1].PnL != 40 || math.Abs(result.Trades[2].PnL) > 1e-9 {
		t.Fatalf("partial close PnL events = %f/%f, want 40/0", result.Trades[1].PnL, result.Trades[2].PnL)
	}
}

func TestResultCollectorClosedTradeStatsFinalizePartiallyFilledCancellation(t *testing.T) {
	result, collector := newTradeStatsCollector()
	at := time.Date(2026, time.July, 21, 13, 30, 0, 0, time.UTC)

	collector.onOrderUpdate(tradeStatsOrder(1, types.SideTypeBuy, 10, 10, 100, types.OrderStatusFilled, at))
	partialClose := tradeStatsOrder(2, types.SideTypeSell, 10, 4, 90, types.OrderStatusPartiallyFilled, at.Add(time.Minute))
	collector.onOrderUpdate(partialClose)
	partialClose.Status = types.OrderStatusCanceled
	partialClose.UpdateTime = types.Time(at.Add(2 * time.Minute))
	collector.onOrderUpdate(partialClose)
	finalizeTradeStatsCollector(t, collector)

	assertTradeStats(t, result, 2, 1, 0)
}

func TestResultCollectorClosedTradeStatsIncludeWarmupClosuresThatAffectEquity(t *testing.T) {
	warmupUntil := time.Date(2026, time.July, 21, 13, 30, 0, 0, time.UTC)
	result := &RunResult{}
	collector := newResultCollector("US.AAPL", types.Interval1m, "USD", warmupUntil, result)

	collector.onOrderUpdate(tradeStatsOrder(1, types.SideTypeBuy, 1, 1, 100, types.OrderStatusFilled, warmupUntil.Add(-3*time.Minute)))
	collector.onOrderUpdate(tradeStatsOrder(2, types.SideTypeSell, 1, 1, 120, types.OrderStatusFilled, warmupUntil.Add(-2*time.Minute)))
	collector.onOrderUpdate(tradeStatsOrder(3, types.SideTypeSell, 2, 2, 110, types.OrderStatusFilled, warmupUntil.Add(-time.Minute)))
	collector.onOrderUpdate(tradeStatsOrder(4, types.SideTypeBuy, 2, 2, 100, types.OrderStatusFilled, warmupUntil.Add(time.Minute)))
	finalizeTradeStatsCollector(t, collector)

	assertTradeStats(t, result, 4, 2, 1)
	if !result.Trades[0].Warmup || !result.Trades[1].Warmup || !result.Trades[2].Warmup || result.Trades[3].Warmup {
		t.Fatalf("warmup flags = %#v, want first three warmup fills marked", result.Trades)
	}
	if result.Trades[1].PnL != 20 || result.Trades[3].PnL != 20 {
		t.Fatalf("closing PnL = %f/%f, want warmup and formal PnL included", result.Trades[1].PnL, result.Trades[3].PnL)
	}
}

func TestResultCollectorTradeStatsDefensiveBoundaries(t *testing.T) {
	result, collector := newTradeStatsCollector()
	at := time.Date(2026, time.July, 21, 13, 30, 0, 0, time.UTC)

	collector.orderExecuted = nil
	collector.orderNotional = nil
	filled := tradeStatsOrder(10, types.SideTypeBuy, 1, 1, 100, types.OrderStatusFilled, at)
	if fill := collector.incrementalFill(filled); fill.quantity.String() != "1" {
		t.Fatalf("incremental fill = %+v, want quantity 1", fill)
	}
	if fill := collector.incrementalFill(filled); fill.quantity.Sign() != 0 {
		t.Fatalf("duplicate incremental fill = %+v, want zero", fill)
	}
	collector.onOrderUpdate(filled)

	if quantity, pnl, known := collector.applyPositionFill(types.SideTypeBuy, fixedpoint.Zero, fixedpoint.NewFromInt(1)); quantity.Sign() != 0 || pnl.Sign() != 0 || known {
		t.Fatalf("zero fill result = %s/%s/%v", quantity, pnl, known)
	}
	if quantity, pnl, known := collector.applyPositionFill(types.SideType("UNKNOWN"), fixedpoint.NewFromInt(1), fixedpoint.NewFromInt(1)); quantity.Sign() != 0 || pnl.Sign() != 0 || known {
		t.Fatalf("unknown-side result = %s/%s/%v", quantity, pnl, known)
	}
	collector.netPosition = fixedpoint.NewFromInt(1)
	collector.averageEntryPrice = fixedpoint.Zero
	collector.positionCostKnown = false
	collector.applyPositionFill(types.SideTypeBuy, fixedpoint.NewFromInt(1), fixedpoint.NewFromInt(100))
	if collector.positionCostKnown || !collector.averageEntryPrice.IsZero() {
		t.Fatalf("unknown position cost was fabricated: known=%v average=%s", collector.positionCostKnown, collector.averageEntryPrice)
	}

	collector.recordClosingFill("ignored", fixedpoint.Zero, fixedpoint.Zero, false)
	collector.closingOrders = nil
	collector.recordClosingFill("pending", fixedpoint.NewFromInt(1), fixedpoint.NewFromInt(5), true)
	collector.finalizeClosingOrder("missing")
	collector.closingOrders["empty"] = &closingOrderStats{}
	collector.finalizeClosingOrder("empty")
	collector.finalizePendingClosingOrders()
	if collector.closedTrades != 1 || collector.winningTrades != 1 {
		t.Fatalf("finalized closing stats = %d/%d, want 1/1", collector.closedTrades, collector.winningTrades)
	}

	collector.onKLineClosed(context.Background(), stubAccountQuerier{}, types.KLine{Symbol: "US.MSFT"})
	collector.netPosition = fixedpoint.NewFromInt(1)
	collector.candles = []Candle{{Close: "not-a-price"}}
	account := types.NewAccount()
	account.SetBalance("USD", types.Balance{Currency: "USD", Available: fixedpoint.NewFromInt(1000)})
	collector.finalize(context.Background(), stubAccountQuerier{account: account}, 1000)
	if len(result.RuntimeErrors) == 0 {
		t.Fatal("invalid final close did not produce a runtime warning")
	}
}

func newTradeStatsCollector() (*RunResult, *resultCollector) {
	result := &RunResult{}
	return result, newResultCollector("US.AAPL", types.Interval1m, "USD", time.Time{}, result)
}

func tradeStatsOrder(
	orderID uint64,
	side types.SideType,
	quantity float64,
	executed float64,
	averagePrice float64,
	status types.OrderStatus,
	at time.Time,
) types.Order {
	return types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol:       "US.AAPL",
			Side:         side,
			Quantity:     fixedpoint.NewFromFloat(quantity),
			AveragePrice: fixedpoint.NewFromFloat(averagePrice),
		},
		OrderID:          orderID,
		Status:           status,
		ExecutedQuantity: fixedpoint.NewFromFloat(executed),
		UpdateTime:       types.Time(at),
	}
}

func finalizeTradeStatsCollector(t *testing.T, collector *resultCollector) {
	t.Helper()
	account := types.NewAccount()
	account.SetBalance("USD", types.Balance{Currency: "USD", Available: fixedpoint.NewFromInt(1000)})
	collector.finalize(context.Background(), stubAccountQuerier{account: account}, 1000)
}

func assertTradeStats(t *testing.T, result *RunResult, totalFills int, totalTrades int, winRate float64) {
	t.Helper()
	if result.TradeStatsVersion != closedTradeStatsVersion {
		t.Fatalf("TradeStatsVersion = %d, want %d", result.TradeStatsVersion, closedTradeStatsVersion)
	}
	if result.TotalFills != totalFills {
		t.Fatalf("TotalFills = %d, want %d", result.TotalFills, totalFills)
	}
	if result.TotalTrades != totalTrades {
		t.Fatalf("TotalTrades = %d, want %d", result.TotalTrades, totalTrades)
	}
	if math.Abs(result.WinRate-winRate) > 1e-9 {
		t.Fatalf("WinRate = %f, want %f", result.WinRate, winRate)
	}
}
