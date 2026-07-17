package backtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestCoverage98FeeSchedulePreservesExplicitPresetIntentAndSafeEmptyFallback(t *testing.T) {
	preset := FeeSchedule{Mode: tradingCostModeMarketPreset, Rules: []FeeRule{{ID: "preset-rule"}}}
	resolved := resolveFeeSchedule(feeGroupBroker, FeeSchedule{PresetID: "selected-by-user"}, preset, FeeSchedule{})
	if resolved.Mode != tradingCostModeMarketPreset || len(resolved.Rules) != 1 || resolved.Rules[0].ID != "preset-rule" {
		t.Fatalf("explicit preset was not applied: %#v", resolved)
	}

	emptyPreset := resolveFeeSchedule(feeGroupBroker, FeeSchedule{Mode: tradingCostModeMarketPreset}, FeeSchedule{}, FeeSchedule{})
	if emptyPreset.Mode != tradingCostModeNone || len(emptyPreset.Rules) != 0 {
		t.Fatalf("empty preset must fail closed to no fees: %#v", emptyPreset)
	}
}

func TestCoverage98FeeEngineRejectsNonBillableTradesAndDoesNotDoubleChargeOrderFee(t *testing.T) {
	result := &RunResult{}
	engine := newBacktestFeeEngine(nil, "USD", "stock", TradingCosts{BrokerFees: FeeSchedule{
		Mode: tradingCostModeCustom,
		Rules: []FeeRule{{
			ID: "per-order", Category: feeCategoryBroker, Side: feeSideBoth, Basis: feeBasisOrder, FixedAmount: 2, Currency: "USD",
		}},
	}}, result, nil)

	engine.onTradeUpdate(types.Trade{
		ID: 1, OrderID: 9, Symbol: "US.AAPL", Side: types.SideTypeBuy,
		Price: fixedpoint.Zero, Quantity: fixedpoint.NewFromFloat(1),
	})
	if result.TotalFees != 0 {
		t.Fatalf("zero-notional trade was charged: %#v", result)
	}

	for tradeID := uint64(2); tradeID <= 3; tradeID++ {
		engine.onTradeUpdate(types.Trade{
			ID: tradeID, OrderID: 9, Symbol: "US.AAPL", Side: types.SideTypeBuy,
			Price: fixedpoint.NewFromFloat(100), Quantity: fixedpoint.NewFromFloat(1),
		})
	}
	if result.TotalBrokerFees != 2 || result.TotalFees != 2 {
		t.Fatalf("per-order fee was not charged exactly once: %#v", result)
	}
	engine.finalize()
	assertBreakdownAmount(t, result.FeeBreakdown, feeGroupBroker, "per-order", 2)
}

func TestCoverage98AccountReadFailureDoesNotCreateSpeculativeEquityPoint(t *testing.T) {
	result := &RunResult{}
	collector := newResultCollector("US.AAPL", types.Interval("1m"), "USD", time.Time{}, result)
	collector.onKLineClosed(context.Background(), stubAccountQuerier{err: errors.New("account unavailable")}, types.KLine{
		Symbol: "US.AAPL", Interval: types.Interval("1m"), EndTime: types.Time(time.Date(2026, time.July, 1, 10, 0, 0, 0, time.UTC)),
		Open: fixedpoint.NewFromFloat(100), High: fixedpoint.NewFromFloat(101), Low: fixedpoint.NewFromFloat(99), Close: fixedpoint.NewFromFloat(100), Volume: fixedpoint.NewFromFloat(10),
	})
	if len(collector.candles) != 1 || len(collector.pnlCurve) != 0 {
		t.Fatalf("account failure should retain market data but not fabricate equity: candles=%d pnl=%d", len(collector.candles), len(collector.pnlCurve))
	}
}

func TestCoverage98PrepareKLineErrorClassifierRequiresBothStableFragments(t *testing.T) {
	if isMissingPrepareKLineError(nil) {
		t.Fatal("nil error classified as missing K-line data")
	}
	if isMissingPrepareKLineError(errors.New("no kline data found for symbol")) {
		t.Fatal("partial error message classified as missing K-line data")
	}
	if !isMissingPrepareKLineError(errors.New("no kline data found for symbol US.AAPL 1m before start time")) {
		t.Fatal("complete missing K-line error was not classified")
	}
}
