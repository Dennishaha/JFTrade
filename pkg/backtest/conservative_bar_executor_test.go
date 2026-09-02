package backtest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type conservativeBarTestWarningSink struct {
	warnings []string
}

func (sink *conservativeBarTestWarningSink) AddWarning(message string) {
	sink.warnings = append(sink.warnings, message)
}

func TestNormalizeExecutionModelName(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr string
	}{
		{name: "empty default", want: ExecutionModelConservativeBarV1},
		{name: "trim lower case", value: "  conservative-bar-v1  ", want: ExecutionModelConservativeBarV1},
		{name: "upper case", value: "CONSERVATIVE-BAR-V1", want: ExecutionModelConservativeBarV1},
		{name: "unsupported", value: "optimistic", wantErr: "unsupported backtest executionModel: optimistic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeExecutionModelName(tt.value)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("NormalizeExecutionModelName(%q) error = %v, want %q", tt.value, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeExecutionModelName(%q) error = %v", tt.value, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeExecutionModelName(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestConservativeBarExecutorValidationErrors(t *testing.T) {
	var nilExecutor *conservativeBarExecutor
	if _, err := nilExecutor.SubmitOrders(context.Background(), types.SubmitOrder{}); err == nil || !strings.Contains(err.Error(), "conservative bar executor is required") {
		t.Fatalf("nil SubmitOrders error = %v", err)
	}
	if _, err := nilExecutor.SubmitAtomicPineOrders(context.Background(), "group", PineWorkerAtomicOrder{}, PineWorkerAtomicOrder{}); err == nil || !strings.Contains(err.Error(), "conservative bar executor is required") {
		t.Fatalf("nil SubmitAtomicPineOrders error = %v", err)
	}
	if err := nilExecutor.CancelOrders(context.Background(), types.Order{}); err == nil || !strings.Contains(err.Error(), "conservative bar executor is required") {
		t.Fatalf("nil CancelOrders error = %v", err)
	}

	stream := types.NewStandardStream()
	missingAccount := newConservativeBarExecutor(nil, &stream, conservativeBarExecutorOptions{})
	if _, err := missingAccount.SubmitOrders(context.Background(), types.SubmitOrder{}); err == nil || !strings.Contains(err.Error(), "account is required") {
		t.Fatalf("missing account error = %v", err)
	}
	if _, err := missingAccount.SubmitAtomicPineOrders(context.Background(), "group", PineWorkerAtomicOrder{}, PineWorkerAtomicOrder{}); err == nil || !strings.Contains(err.Error(), "account is required") {
		t.Fatalf("missing atomic account error = %v", err)
	}
	missingStream := newConservativeBarExecutor(types.NewAccount(), nil, conservativeBarExecutorOptions{})
	if _, err := missingStream.SubmitOrders(context.Background(), types.SubmitOrder{}); err == nil || !strings.Contains(err.Error(), "stream is required") {
		t.Fatalf("missing stream submit error = %v", err)
	}
	if _, err := missingStream.SubmitAtomicPineOrders(context.Background(), "group", PineWorkerAtomicOrder{}, PineWorkerAtomicOrder{}); err == nil || !strings.Contains(err.Error(), "stream is required") {
		t.Fatalf("missing atomic stream error = %v", err)
	}
	if err := missingStream.CancelOrders(context.Background(), types.Order{}); err == nil || !strings.Contains(err.Error(), "stream is required") {
		t.Fatalf("missing stream cancel error = %v", err)
	}
	valid := newConservativeBarExecutor(types.NewAccount(), &stream, conservativeBarExecutorOptions{})
	if _, err := valid.SubmitAtomicPineOrders(context.Background(), "", PineWorkerAtomicOrder{}); err == nil || !strings.Contains(err.Error(), "requires an id and at least two orders") {
		t.Fatalf("invalid atomic group error = %v", err)
	}
}

func TestConservativeBarExecutorWarningsAndUnmatchedOrders(t *testing.T) {
	account := types.NewAccount()
	stream := types.NewStandardStream()
	sink := &conservativeBarTestWarningSink{}
	executor := newConservativeBarExecutor(account, &stream, conservativeBarExecutorOptions{WarningSink: sink})
	base := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)

	market := testPineWorkerShortReplayMarket()
	if _, err := executor.SubmitOrders(
		context.Background(),
		types.SubmitOrder{ClientOrderID: "unsupported", Symbol: "US.AAPL", Side: types.SideTypeBuy, Type: types.OrderType("TRAILING_STOP"), Quantity: fixedpoint.NewFromFloat(1), Market: market},
		types.SubmitOrder{ClientOrderID: "wrong-symbol", Symbol: "US.MSFT", Side: types.SideTypeBuy, Type: types.OrderTypeMarket, Quantity: fixedpoint.NewFromFloat(1), Market: market},
	); err != nil {
		t.Fatalf("SubmitOrders error = %v", err)
	}
	executor.onKLineClosed(testConservativeBarKLine(base, 100, 101, 99, 100, 0))
	executor.onKLineClosed(testConservativeBarKLine(base.Add(time.Minute), 100, 101, 99, 100, 0))
	executor.onKLineClosed(testConservativeBarKLine(base.Add(2*time.Minute), 100, 101, 99, 100, 1000))

	if len(sink.warnings) != 2 {
		t.Fatalf("warnings = %#v, want zero-volume and unsupported", sink.warnings)
	}
	if !strings.Contains(sink.warnings[0], "has no positive volume") || !strings.Contains(sink.warnings[1], "unsupported order type") {
		t.Fatalf("warnings = %#v", sink.warnings)
	}
}

func TestConservativeBarExecutorHelperBranches(t *testing.T) {
	base := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	bar := testConservativeBarKLine(base, 100, 105, 95, 101, 100)
	var nilExecutor *conservativeBarExecutor
	nilExecutor.onKLineClosed(bar)

	if budget := conservativeBarLiquidityBudget(testConservativeBarKLine(base, 100, 101, 99, 100, 0)); !budget.IsZero() {
		t.Fatalf("zero-volume liquidity budget = %s, want 0", budget)
	}
	if price, ok := conservativeBarLimitPrice(types.Order{
		SubmitOrder: types.SubmitOrder{Side: types.SideTypeBuy, Price: fixedpoint.NewFromFloat(102)},
	}, testConservativeBarKLine(base, 100, 101, 99, 101, 100), conservativeBarClosePoint); !ok || price.String() != "101" {
		t.Fatalf("buy limit close price=%s ok=%v, want 101 true", price, ok)
	}
	if price, ok := conservativeBarLimitPrice(types.Order{
		SubmitOrder: types.SubmitOrder{Side: types.SideTypeBuy, Price: fixedpoint.NewFromFloat(98)},
	}, testConservativeBarKLine(base, 100, 101, 97, 99, 100), conservativeBarFullBar); !ok || price.String() != "98" {
		t.Fatalf("buy limit intrabar price=%s ok=%v, want 98 true", price, ok)
	}
	if price, ok := conservativeBarLimitPrice(types.Order{SubmitOrder: types.SubmitOrder{Side: types.SideTypeBuy}}, bar, conservativeBarFullBar); ok || !price.IsZero() {
		t.Fatalf("limit without price price=%s ok=%v, want zero false", price, ok)
	}
	if price, ok := conservativeBarLimitPrice(types.Order{SubmitOrder: types.SubmitOrder{Side: "BAD", Price: fixedpoint.NewFromFloat(100)}}, bar, conservativeBarClosePoint); ok || !price.IsZero() {
		t.Fatalf("limit bad side price=%s ok=%v, want zero false", price, ok)
	}
	if price, ok := conservativeBarLimitPrice(types.Order{
		SubmitOrder: types.SubmitOrder{Side: types.SideTypeBuy, Price: fixedpoint.NewFromFloat(90)},
	}, bar, conservativeBarFullBar); ok || !price.IsZero() {
		t.Fatalf("unfilled limit price=%s ok=%v, want zero false", price, ok)
	}
	if price, ok := conservativeBarStopMarketPrice(types.Order{SubmitOrder: types.SubmitOrder{Side: types.SideTypeBuy}}, bar, conservativeBarFullBar); ok || !price.IsZero() {
		t.Fatalf("stop without price price=%s ok=%v, want zero false", price, ok)
	}
	if price, ok := conservativeBarStopMarketPrice(types.Order{
		SubmitOrder: types.SubmitOrder{Side: types.SideTypeBuy, StopPrice: fixedpoint.NewFromFloat(100)},
	}, testConservativeBarKLine(base, 99, 101, 98, 100, 100), conservativeBarClosePoint); !ok || price.String() != "100" {
		t.Fatalf("buy stop close price=%s ok=%v, want 100 true", price, ok)
	}
	if price, ok := conservativeBarStopMarketPrice(types.Order{
		SubmitOrder: types.SubmitOrder{Side: types.SideTypeSell, StopPrice: fixedpoint.NewFromFloat(100)},
	}, testConservativeBarKLine(base, 101, 102, 99, 100, 100), conservativeBarClosePoint); !ok || price.String() != "100" {
		t.Fatalf("sell stop close price=%s ok=%v, want 100 true", price, ok)
	}
	if price, ok := conservativeBarStopMarketPrice(types.Order{
		SubmitOrder: types.SubmitOrder{Side: types.SideTypeBuy, StopPrice: fixedpoint.NewFromFloat(104)},
	}, testConservativeBarKLine(base, 100, 105, 99, 101, 100), conservativeBarFullBar); !ok || price.String() != "104" {
		t.Fatalf("buy stop intrabar price=%s ok=%v, want 104 true", price, ok)
	}
	if price, ok := conservativeBarStopMarketPrice(types.Order{
		SubmitOrder: types.SubmitOrder{Side: types.SideTypeSell, StopPrice: fixedpoint.NewFromFloat(100)},
	}, testConservativeBarKLine(base, 99, 101, 98, 100, 100), conservativeBarFullBar); !ok || price.String() != "99" {
		t.Fatalf("sell stop open price=%s ok=%v, want 99 true", price, ok)
	}
	if price, ok := conservativeBarStopMarketPrice(types.Order{SubmitOrder: types.SubmitOrder{Side: "BAD", StopPrice: fixedpoint.NewFromFloat(100)}}, bar, conservativeBarClosePoint); ok || !price.IsZero() {
		t.Fatalf("stop bad side price=%s ok=%v, want zero false", price, ok)
	}
	if conservativeBarStopTriggered(types.Order{SubmitOrder: types.SubmitOrder{Side: types.SideTypeBuy, StopPrice: fixedpoint.NewFromFloat(200)}}, bar, conservativeBarFullBar) {
		t.Fatal("stop triggered for unreachable price")
	}

	stream := types.NewStandardStream()
	executor := newConservativeBarExecutor(types.NewAccount(), &stream, conservativeBarExecutorOptions{SlippageTicks: 1})
	executor.fillPendingOrderLocked(nil, bar, conservativeBarFullBar)
	executor.fillPendingOrderLocked(&conservativeBarPendingOrder{}, bar, conservativeBarFullBar)
	executor.currentBarBudgetSymbol = "US.MSFT"
	executor.currentBarBudget = fixedpoint.Zero
	executor.fillPendingOrderLocked(&conservativeBarPendingOrder{
		order:     types.Order{SubmitOrder: types.SubmitOrder{Symbol: "US.AAPL", Side: types.SideTypeBuy, Type: types.OrderTypeMarket, Quantity: fixedpoint.NewFromFloat(1), Market: testPineWorkerShortReplayMarket()}},
		remaining: fixedpoint.NewFromFloat(1),
	}, bar, conservativeBarFullBar)
	executor.currentBarBudgetSymbol = bar.Symbol
	executor.currentBarBudget = fixedpoint.Zero
	executor.fillPendingOrderLocked(&conservativeBarPendingOrder{
		order:     types.Order{SubmitOrder: types.SubmitOrder{Symbol: "US.AAPL", Side: types.SideTypeBuy, Type: types.OrderTypeMarket, Quantity: fixedpoint.NewFromFloat(1), Market: testPineWorkerShortReplayMarket()}},
		remaining: fixedpoint.NewFromFloat(1),
	}, bar, conservativeBarFullBar)
	beforeTradeID := executor.nextTradeID
	executor.applyFillLocked(&conservativeBarPendingOrder{
		order:     types.Order{SubmitOrder: types.SubmitOrder{Symbol: "US.AAPL", Side: "BAD", Quantity: fixedpoint.NewFromFloat(1), Market: testPineWorkerShortReplayMarket()}},
		remaining: fixedpoint.NewFromFloat(1),
	}, fixedpoint.NewFromFloat(1), fixedpoint.NewFromFloat(100), base)
	if executor.nextTradeID != beforeTradeID {
		t.Fatalf("bad-side applyFill advanced trade id from %d to %d", beforeTradeID, executor.nextTradeID)
	}
	if price, ok := executor.matchPriceLocked(&conservativeBarPendingOrder{
		order: types.Order{SubmitOrder: types.SubmitOrder{Side: types.SideTypeBuy, Type: types.OrderTypeStopMarket, StopPrice: fixedpoint.NewFromFloat(200)}},
	}, bar, conservativeBarFullBar); ok || !price.IsZero() {
		t.Fatalf("untriggered stop-market price=%s ok=%v, want zero false", price, ok)
	}
	if price, ok := executor.matchPriceLocked(&conservativeBarPendingOrder{
		order: types.Order{SubmitOrder: types.SubmitOrder{Side: types.SideTypeBuy, Type: types.OrderTypeStopLimit, StopPrice: fixedpoint.NewFromFloat(200), Price: fixedpoint.NewFromFloat(100)}},
	}, bar, conservativeBarFullBar); ok || !price.IsZero() {
		t.Fatalf("untriggered stop-limit price=%s ok=%v, want zero false", price, ok)
	}
	if got := executor.applyMarketSlippage(types.Order{SubmitOrder: types.SubmitOrder{Side: types.SideTypeSell, Market: types.Market{TickSize: fixedpoint.NewFromFloat(1)}}}, fixedpoint.NewFromFloat(0.5)); !got.IsZero() {
		t.Fatalf("negative slippage price = %s, want zero", got)
	}
	if got := executor.applyMarketSlippage(types.Order{SubmitOrder: types.SubmitOrder{Side: types.SideTypeBuy, Market: types.Market{TickSize: fixedpoint.NewFromFloat(1)}}}, fixedpoint.NewFromFloat(1)); got.String() != "2" {
		t.Fatalf("buy slippage price = %s, want 2", got)
	}
	if !executor.eventTimeLocked().After(time.Time{}) {
		t.Fatal("eventTimeLocked before current bar should return current time")
	}
	if executor.hasPendingSymbolLocked("US.AAPL") {
		t.Fatal("hasPendingSymbolLocked = true, want false")
	}
	executor.warnOnceLocked("no-sink", "ignored")
	if len(executor.warned) != 0 {
		t.Fatalf("warned with nil sink = %#v, want empty", executor.warned)
	}
	sink := &conservativeBarTestWarningSink{}
	executor.options.WarningSink = sink
	executor.warnOnceLocked("once", "first")
	executor.warnOnceLocked("once", "second")
	if len(sink.warnings) != 1 || sink.warnings[0] != "first" {
		t.Fatalf("warnings = %#v, want one first warning", sink.warnings)
	}
}

func TestConservativeBarExecutorCancelSkipsUnmatchedPendingOrders(t *testing.T) {
	stream := types.NewStandardStream()
	executor := newConservativeBarExecutor(types.NewAccount(), &stream, conservativeBarExecutorOptions{})
	executor.pending = []*conservativeBarPendingOrder{
		nil,
		{order: types.Order{OrderID: 1}, remaining: fixedpoint.Zero},
		{order: types.Order{OrderID: 2, SubmitOrder: types.SubmitOrder{ClientOrderID: "other"}}, remaining: fixedpoint.NewFromFloat(1)},
	}
	if err := executor.CancelOrders(context.Background(), types.Order{OrderID: 3}); err != nil {
		t.Fatalf("CancelOrders error = %v", err)
	}
	if len(executor.pending) != 1 || executor.pending[0].order.OrderID != 2 {
		t.Fatalf("pending after unmatched cancel = %#v", executor.pending)
	}
}

func testConservativeBarKLine(start time.Time, open, high, low, close, volume float64) types.KLine {
	return types.KLine{
		Symbol:    "US.AAPL",
		Interval:  types.Interval1m,
		StartTime: types.Time(start),
		EndTime:   types.Time(start.Add(time.Minute - time.Millisecond)),
		Open:      fixedpoint.NewFromFloat(open),
		High:      fixedpoint.NewFromFloat(high),
		Low:       fixedpoint.NewFromFloat(low),
		Close:     fixedpoint.NewFromFloat(close),
		Volume:    fixedpoint.NewFromFloat(volume),
	}
}
