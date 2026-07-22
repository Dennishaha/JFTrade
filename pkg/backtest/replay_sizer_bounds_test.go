package backtest

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestPineWorkerReplaySizerRejectsInvalidQuantityPctInputs(t *testing.T) {
	var nilSizer *pineWorkerReplaySizer
	nilSizer.onKLineClosed(testPineWorkerShortReplayKLine(time.Now(), 100))
	nilSizer.onOrderUpdate(types.Order{})

	if _, err := nilSizer.QuantityForCommand(WorkerOrderCommand{ID: "nil", QuantityPct: 50}, testPineWorkerShortReplayMarket()); err == nil || !strings.Contains(err.Error(), "position sizing") {
		t.Fatalf("nil sizer error = %v", err)
	}

	sizer := newPineWorkerReplaySizer("US.AAPL", "USD", types.NewAccount())
	market := testPineWorkerShortReplayMarket()
	for _, pct := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		_, err := sizer.QuantityForCommand(WorkerOrderCommand{ID: "bad-pct", Kind: "entry", QuantityPct: pct}, market)
		if err == nil || !strings.Contains(err.Error(), "quantity pct must be positive") {
			t.Fatalf("QuantityForCommand pct %v error = %v", pct, err)
		}
	}
	if _, err := sizer.QuantityForCommand(WorkerOrderCommand{ID: "unsupported", Kind: "alert", QuantityPct: 50}, market); err == nil || !strings.Contains(err.Error(), "does not support quantity pct") {
		t.Fatalf("unsupported kind error = %v", err)
	}
}

func TestPineWorkerReplaySizerMaintainsPositionFromIncrementalFills(t *testing.T) {
	sizer := newPineWorkerReplaySizer("US.AAPL", "USD", types.NewAccount())
	market := testPineWorkerShortReplayMarket()

	sizer.onKLineClosed(testPineWorkerShortReplayKLine(time.Now(), 100))
	sizer.onKLineClosed(types.KLine{Symbol: "US.MSFT", Close: fixedpoint.NewFromFloat(250)})
	if got := sizer.priceForCommand(WorkerOrderCommand{}, market); got.Float64() != 100 {
		t.Fatalf("last price after unrelated kline = %s, want 100", got)
	}

	sizer.onOrderUpdate(types.Order{SubmitOrder: types.SubmitOrder{Symbol: "US.AAPL", Side: types.SideTypeBuy, Quantity: fixedpoint.NewFromFloat(5)}, Status: types.OrderStatusNew})
	sizer.onOrderUpdate(types.Order{SubmitOrder: types.SubmitOrder{Symbol: "US.MSFT", Side: types.SideTypeBuy, Quantity: fixedpoint.NewFromFloat(5)}, Status: types.OrderStatusFilled})
	sizer.onOrderUpdate(types.Order{SubmitOrder: types.SubmitOrder{Symbol: "US.AAPL", Side: types.SideTypeBuy, Quantity: fixedpoint.Zero}, Status: types.OrderStatusFilled})
	sizer.onOrderUpdate(types.Order{SubmitOrder: types.SubmitOrder{Symbol: "US.AAPL", Side: types.SideTypeBuy, Quantity: fixedpoint.NewFromFloat(-1)}, Status: types.OrderStatusFilled})

	if _, err := sizer.QuantityForCommand(WorkerOrderCommand{ID: "none", Kind: "close", QuantityPct: 50}, market); err == nil || !strings.Contains(err.Error(), "open position") {
		t.Fatalf("close without filled position error = %v", err)
	}

	sizer.onOrderUpdate(types.Order{
		SubmitOrder: types.SubmitOrder{Symbol: "US.AAPL", Side: types.SideTypeBuy, Quantity: fixedpoint.NewFromFloat(3)},
		Status:      types.OrderStatusFilled,
	})
	sizer.onOrderUpdate(types.Order{
		SubmitOrder:      types.SubmitOrder{Symbol: "US.AAPL", Side: types.SideTypeSell, Quantity: fixedpoint.NewFromFloat(1)},
		Status:           types.OrderStatusFilled,
		ExecutedQuantity: fixedpoint.NewFromFloat(1),
	})
	qty, err := sizer.QuantityForCommand(WorkerOrderCommand{ID: "close-more-than-open", Kind: "close", QuantityPct: 150}, market)
	if err != nil {
		t.Fatalf("QuantityForCommand close error = %v", err)
	}
	if qty.Float64() != 2 {
		t.Fatalf("close quantity = %s, want clamped open position 2", qty)
	}

	partial := types.Order{
		SubmitOrder:      types.SubmitOrder{Symbol: "US.AAPL", Side: types.SideTypeBuy, Quantity: fixedpoint.NewFromFloat(5)},
		OrderID:          42,
		Status:           types.OrderStatusPartiallyFilled,
		ExecutedQuantity: fixedpoint.NewFromFloat(2),
	}
	sizer.onOrderUpdate(partial)
	partial.ExecutedQuantity = fixedpoint.NewFromFloat(4)
	sizer.onOrderUpdate(partial)
	partial.Status = types.OrderStatusFilled
	partial.ExecutedQuantity = fixedpoint.NewFromFloat(5)
	sizer.onOrderUpdate(partial)
	if got := sizer.NetPosition().Float64(); got != 7 {
		t.Fatalf("net position after cumulative partial fills = %v, want 7", got)
	}
}

func TestPineWorkerReplaySizerEntryEquityBoundaries(t *testing.T) {
	market := testPineWorkerShortReplayMarket()

	noPrice := newPineWorkerReplaySizer("US.AAPL", "USD", types.NewAccount())
	if _, err := noPrice.QuantityForCommand(WorkerOrderCommand{ID: "no-price", Kind: "entry", QuantityPct: 50}, market); err == nil || !strings.Contains(err.Error(), "positive price") {
		t.Fatalf("missing price error = %v", err)
	}

	noAccount := newPineWorkerReplaySizer("US.AAPL", "USD", nil)
	if _, err := noAccount.QuantityForCommand(WorkerOrderCommand{ID: "no-account", Kind: "entry", LimitPrice: 100, QuantityPct: 50}, market); err == nil || !strings.Contains(err.Error(), "account is required") {
		t.Fatalf("missing account error = %v", err)
	}

	noQuote := newPineWorkerReplaySizer("US.AAPL", "", types.NewAccount())
	quoteLessMarket := market
	quoteLessMarket.QuoteCurrency = ""
	if _, err := noQuote.QuantityForCommand(WorkerOrderCommand{ID: "no-quote", Kind: "entry", LimitPrice: 100, QuantityPct: 50}, quoteLessMarket); err == nil || !strings.Contains(err.Error(), "quote currency") {
		t.Fatalf("missing quote error = %v", err)
	}

	zeroEquity := newPineWorkerReplaySizer("US.AAPL", "USD", types.NewAccount())
	if _, err := zeroEquity.QuantityForCommand(WorkerOrderCommand{ID: "zero-equity", Kind: "entry", LimitPrice: 100, QuantityPct: 50}, quoteLessMarket); err == nil || !strings.Contains(err.Error(), "positive equity") {
		t.Fatalf("zero equity error = %v", err)
	}

	account := types.NewAccount()
	account.UpdateBalances(types.BalanceMap{"USD": {Currency: "USD", Available: fixedpoint.NewFromFloat(1000)}})
	sizer := newPineWorkerReplaySizer("US.AAPL", "USD", account)
	sizer.onOrderUpdate(types.Order{
		SubmitOrder:      types.SubmitOrder{Symbol: "US.AAPL", Side: types.SideTypeBuy},
		Status:           types.OrderStatusFilled,
		ExecutedQuantity: fixedpoint.NewFromFloat(2),
	})
	if _, err := sizer.QuantityForCommand(WorkerOrderCommand{ID: "needs-mark", Kind: "entry", LimitPrice: 100, QuantityPct: 50}, market); err == nil || !strings.Contains(err.Error(), "mark price") {
		t.Fatalf("missing mark price error = %v", err)
	}

	sizer.onKLineClosed(testPineWorkerShortReplayKLine(time.Now(), 120))
	qty, err := sizer.QuantityForCommand(WorkerOrderCommand{ID: "with-mtm", Kind: "entry", LimitPrice: 100, QuantityPct: 50}, market)
	if err != nil {
		t.Fatalf("QuantityForCommand with mark-to-market error = %v", err)
	}
	if qty.Float64() != 6.2 {
		t.Fatalf("entry quantity = %s, want equity including mark-to-market 6.2", qty)
	}
}

func TestPineWorkerReplaySizerPriceAndQuantityPrecision(t *testing.T) {
	sizer := newPineWorkerReplaySizer("US.AAPL", "USD", types.NewAccount())
	market := testPineWorkerShortReplayMarket()
	market.TickSize = fixedpoint.NewFromFloat(0.05)
	market.StepSize = fixedpoint.NewFromFloat(0.1)

	sizer.onKLineClosed(testPineWorkerShortReplayKLine(time.Now(), 100.17))
	if got := sizer.priceForCommand(WorkerOrderCommand{LimitPrice: 101.23, StopPrice: 99.87}, market); got.Float64() != 101.23 {
		t.Fatalf("limit price priority = %s, want 101.23", got)
	}
	if got := sizer.priceForCommand(WorkerOrderCommand{StopPrice: 99.87}, market); got.Float64() != 99.87 {
		t.Fatalf("stop price fallback = %s, want 99.87", got)
	}
	if got := sizer.priceForCommand(WorkerOrderCommand{}, market); got.Float64() != 100.1 {
		t.Fatalf("last price truncation = %s, want 100.1", got)
	}

	if got := truncatePineWorkerSizedQuantity(market, fixedpoint.NewFromFloat(1.29)); got.Float64() != 1.2 {
		t.Fatalf("step quantity truncation = %s, want 1.2", got)
	}
	precisionMarket := testPineWorkerShortReplayMarket()
	precisionMarket.VolumePrecision = 2
	if got := truncatePineWorkerSizedQuantity(precisionMarket, fixedpoint.NewFromFloat(1.239)); got.Float64() != 1.23 {
		t.Fatalf("precision quantity truncation = %s, want 1.23", got)
	}
	if got := truncatePineWorkerSizedQuantity(precisionMarket, fixedpoint.NewFromFloat(-1)); !got.IsZero() {
		t.Fatalf("negative quantity truncation = %s, want zero", got)
	}
}
