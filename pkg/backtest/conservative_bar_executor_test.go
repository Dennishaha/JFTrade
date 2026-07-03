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
	if err := nilExecutor.CancelOrders(context.Background(), types.Order{}); err == nil || !strings.Contains(err.Error(), "conservative bar executor is required") {
		t.Fatalf("nil CancelOrders error = %v", err)
	}

	stream := types.NewStandardStream()
	if _, err := newConservativeBarExecutor(nil, &stream, conservativeBarExecutorOptions{}).SubmitOrders(context.Background(), types.SubmitOrder{}); err == nil || !strings.Contains(err.Error(), "account is required") {
		t.Fatalf("missing account error = %v", err)
	}
	if _, err := newConservativeBarExecutor(types.NewAccount(), nil, conservativeBarExecutorOptions{}).SubmitOrders(context.Background(), types.SubmitOrder{}); err == nil || !strings.Contains(err.Error(), "stream is required") {
		t.Fatalf("missing stream submit error = %v", err)
	}
	if err := newConservativeBarExecutor(types.NewAccount(), nil, conservativeBarExecutorOptions{}).CancelOrders(context.Background(), types.Order{}); err == nil || !strings.Contains(err.Error(), "stream is required") {
		t.Fatalf("missing stream cancel error = %v", err)
	}
}

func TestConservativeBarExecutorFillsMarketOrderOnNextOpenWithLiquidityCap(t *testing.T) {
	account := types.NewAccount()
	account.UpdateBalances(types.BalanceMap{
		"USD": {Currency: "USD", Available: fixedpoint.NewFromFloat(10000)},
	})
	stream := types.NewStandardStream()
	var orders []types.Order
	var trades []types.Trade
	stream.OnOrderUpdate(func(order types.Order) { orders = append(orders, order) })
	stream.OnTradeUpdate(func(trade types.Trade) { trades = append(trades, trade) })

	executor := newConservativeBarExecutor(account, &stream, conservativeBarExecutorOptions{})
	base := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	executor.onKLineClosed(testConservativeBarKLine(base, 100, 101, 99, 100, 1000))
	created, err := executor.SubmitOrders(context.Background(), types.SubmitOrder{
		ClientOrderID: "next-open",
		Symbol:        "US.AAPL",
		Side:          types.SideTypeBuy,
		Type:          types.OrderTypeMarket,
		Quantity:      fixedpoint.NewFromFloat(50),
		Market:        testPineWorkerShortReplayMarket(),
	})
	if err != nil {
		t.Fatalf("SubmitOrders error = %v", err)
	}
	if len(created) != 1 || created[0].Status != types.OrderStatusNew {
		t.Fatalf("created = %#v, want one NEW order", created)
	}

	executor.onKLineClosed(testConservativeBarKLine(base.Add(time.Minute), 101, 102, 100, 101, 100))
	executor.onKLineClosed(testConservativeBarKLine(base.Add(2*time.Minute), 102, 103, 101, 102, 1000))

	if len(trades) != 2 {
		t.Fatalf("trades len = %d, want 2 partial fills", len(trades))
	}
	if trades[0].Quantity.String() != "10" || trades[0].Price.String() != "101" {
		t.Fatalf("first fill = %#v, want 10 @ 101", trades[0])
	}
	if trades[1].Quantity.String() != "40" || trades[1].Price.String() != "102" {
		t.Fatalf("second fill = %#v, want 40 @ 102", trades[1])
	}
	if len(orders) != 3 ||
		orders[1].Status != types.OrderStatusPartiallyFilled ||
		orders[1].ExecutedQuantity.String() != "10" ||
		orders[2].Status != types.OrderStatusFilled ||
		orders[2].ExecutedQuantity.String() != "50" {
		t.Fatalf("order updates = %#v", orders)
	}
	balance, _ := account.Balance("USD")
	if balance.Available.String() != "4910" {
		t.Fatalf("USD balance = %s, want 4910", balance.Available)
	}
	baseBalance, _ := account.Balance("AAPL")
	if baseBalance.Available.String() != "50" {
		t.Fatalf("AAPL balance = %s, want 50", baseBalance.Available)
	}
}

func TestConservativeBarExecutorCancelOrders(t *testing.T) {
	account := types.NewAccount()
	stream := types.NewStandardStream()
	var orders []types.Order
	stream.OnOrderUpdate(func(order types.Order) { orders = append(orders, order) })
	executor := newConservativeBarExecutor(account, &stream, conservativeBarExecutorOptions{})

	created, err := executor.SubmitOrders(
		context.Background(),
		types.SubmitOrder{
			ClientOrderID: "cancel-by-id",
			Symbol:        "US.AAPL",
			Side:          types.SideTypeBuy,
			Quantity:      fixedpoint.NewFromFloat(1),
			Market:        testPineWorkerShortReplayMarket(),
		},
		types.SubmitOrder{
			ClientOrderID: "keep",
			Symbol:        "US.AAPL",
			Side:          types.SideTypeBuy,
			Quantity:      fixedpoint.NewFromFloat(1),
			Market:        testPineWorkerShortReplayMarket(),
		},
	)
	if err != nil {
		t.Fatalf("SubmitOrders error = %v", err)
	}
	if err := executor.CancelOrders(context.Background(), types.Order{OrderID: created[0].OrderID}); err != nil {
		t.Fatalf("CancelOrders by order id error = %v", err)
	}
	if err := executor.CancelOrders(context.Background(), types.Order{SubmitOrder: types.SubmitOrder{ClientOrderID: "keep"}}); err != nil {
		t.Fatalf("CancelOrders by client id error = %v", err)
	}
	if err := executor.CancelOrders(context.Background(), types.Order{SubmitOrder: types.SubmitOrder{ClientOrderID: "missing"}}); err != nil {
		t.Fatalf("CancelOrders missing order error = %v", err)
	}
	if len(orders) != 4 {
		t.Fatalf("order update len = %d, want 4", len(orders))
	}
	if orders[2].Status != types.OrderStatusCanceled || orders[3].Status != types.OrderStatusCanceled {
		t.Fatalf("cancel updates = %#v", orders[2:])
	}
	if len(executor.pending) != 0 {
		t.Fatalf("pending len = %d, want 0", len(executor.pending))
	}
}

func TestConservativeBarExecutorProcessOrdersOnCloseUsesSignalClose(t *testing.T) {
	account := types.NewAccount()
	account.UpdateBalances(types.BalanceMap{
		"USD": {Currency: "USD", Available: fixedpoint.NewFromFloat(1000)},
	})
	stream := types.NewStandardStream()
	var trades []types.Trade
	stream.OnTradeUpdate(func(trade types.Trade) { trades = append(trades, trade) })

	executor := newConservativeBarExecutor(account, &stream, conservativeBarExecutorOptions{ProcessOrdersOnClose: true})
	base := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	bar := testConservativeBarKLine(base, 100, 106, 99, 105, 1000)
	executor.onKLineClosed(bar)
	if _, err := executor.SubmitOrders(context.Background(), types.SubmitOrder{
		ClientOrderID: "same-close",
		Symbol:        "US.AAPL",
		Side:          types.SideTypeBuy,
		Type:          types.OrderTypeMarket,
		Quantity:      fixedpoint.NewFromFloat(2),
		Market:        testPineWorkerShortReplayMarket(),
	}); err != nil {
		t.Fatalf("SubmitOrders error = %v", err)
	}

	if len(trades) != 1 || trades[0].Price.String() != "105" || !trades[0].Time.Time().Equal(bar.EndTime.Time()) {
		t.Fatalf("trades = %#v, want same-bar close fill", trades)
	}
}

func TestConservativeBarExecutorSellMarketAndSlippage(t *testing.T) {
	account := types.NewAccount()
	account.UpdateBalances(types.BalanceMap{
		"USD": {Currency: "USD", Available: fixedpoint.NewFromFloat(1000)},
	})
	stream := types.NewStandardStream()
	var trades []types.Trade
	stream.OnTradeUpdate(func(trade types.Trade) { trades = append(trades, trade) })

	executor := newConservativeBarExecutor(account, &stream, conservativeBarExecutorOptions{SlippageTicks: 2})
	base := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	executor.onKLineClosed(testConservativeBarKLine(base, 100, 101, 99, 100, 1000))
	market := testPineWorkerShortReplayMarket()
	market.TickSize = fixedpoint.NewFromFloat(0.05)
	if _, err := executor.SubmitOrders(context.Background(), types.SubmitOrder{
		ClientOrderID: "sell-with-slippage",
		Symbol:        "US.AAPL",
		Side:          types.SideTypeSell,
		Type:          types.OrderTypeMarket,
		Quantity:      fixedpoint.NewFromFloat(2),
		Market:        market,
	}); err != nil {
		t.Fatalf("SubmitOrders error = %v", err)
	}
	executor.onKLineClosed(testConservativeBarKLine(base.Add(time.Minute), 101, 102, 100, 101, 1000))

	if len(trades) != 1 || trades[0].Price.String() != "100.9" {
		t.Fatalf("trades = %#v, want sell at open minus two ticks", trades)
	}
	balance, _ := account.Balance("USD")
	if balance.Available.String() != "1201.8" {
		t.Fatalf("USD balance = %s, want 1201.8", balance.Available)
	}
}

func TestConservativeBarExecutorLimitOrderGetsGapImprovement(t *testing.T) {
	account := types.NewAccount()
	account.UpdateBalances(types.BalanceMap{
		"USD": {Currency: "USD", Available: fixedpoint.NewFromFloat(1000)},
	})
	stream := types.NewStandardStream()
	var trades []types.Trade
	stream.OnTradeUpdate(func(trade types.Trade) { trades = append(trades, trade) })

	executor := newConservativeBarExecutor(account, &stream, conservativeBarExecutorOptions{})
	base := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	executor.onKLineClosed(testConservativeBarKLine(base, 101, 102, 100, 101, 1000))
	if _, err := executor.SubmitOrders(context.Background(), types.SubmitOrder{
		ClientOrderID: "buy-limit",
		Symbol:        "US.AAPL",
		Side:          types.SideTypeBuy,
		Type:          types.OrderTypeLimit,
		Price:         fixedpoint.NewFromFloat(100),
		Quantity:      fixedpoint.NewFromFloat(1),
		Market:        testPineWorkerShortReplayMarket(),
	}); err != nil {
		t.Fatalf("SubmitOrders error = %v", err)
	}
	executor.onKLineClosed(testConservativeBarKLine(base.Add(time.Minute), 99, 101, 98, 100, 1000))

	if len(trades) != 1 || trades[0].Price.String() != "99" {
		t.Fatalf("trades = %#v, want buy limit gap improvement at 99", trades)
	}
}

func TestConservativeBarExecutorLimitSellAndClosePointBranches(t *testing.T) {
	account := types.NewAccount()
	stream := types.NewStandardStream()
	var trades []types.Trade
	stream.OnTradeUpdate(func(trade types.Trade) { trades = append(trades, trade) })

	executor := newConservativeBarExecutor(account, &stream, conservativeBarExecutorOptions{ProcessOrdersOnClose: true})
	base := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	bar := testConservativeBarKLine(base, 100, 106, 99, 105, 1000)
	executor.onKLineClosed(bar)
	if _, err := executor.SubmitOrders(context.Background(), types.SubmitOrder{
		ClientOrderID: "sell-limit-close",
		Symbol:        "US.AAPL",
		Side:          types.SideTypeSell,
		Type:          types.OrderTypeLimit,
		Price:         fixedpoint.NewFromFloat(104),
		Quantity:      fixedpoint.NewFromFloat(1),
		Market:        testPineWorkerShortReplayMarket(),
	}); err != nil {
		t.Fatalf("SubmitOrders error = %v", err)
	}

	if len(trades) != 1 || trades[0].Price.String() != "105" {
		t.Fatalf("trades = %#v, want sell limit close fill at 105", trades)
	}

	if price, ok := conservativeBarLimitPrice(types.Order{
		SubmitOrder: types.SubmitOrder{Side: types.SideTypeSell, Price: fixedpoint.NewFromFloat(101)},
	}, testConservativeBarKLine(base.Add(time.Minute), 102, 103, 101, 102, 1000), conservativeBarFullBar); !ok || price.String() != "102" {
		t.Fatalf("sell limit open improvement price=%s ok=%v, want 102 true", price, ok)
	}
	if price, ok := conservativeBarLimitPrice(types.Order{
		SubmitOrder: types.SubmitOrder{Side: types.SideTypeSell, Price: fixedpoint.NewFromFloat(103)},
	}, testConservativeBarKLine(base.Add(2*time.Minute), 100, 104, 99, 101, 1000), conservativeBarFullBar); !ok || price.String() != "103" {
		t.Fatalf("sell limit intrabar price=%s ok=%v, want 103 true", price, ok)
	}
}

func TestConservativeBarExecutorStopOrders(t *testing.T) {
	account := types.NewAccount()
	account.UpdateBalances(types.BalanceMap{"USD": {Currency: "USD", Available: fixedpoint.NewFromFloat(1000)}})
	stream := types.NewStandardStream()
	var trades []types.Trade
	stream.OnTradeUpdate(func(trade types.Trade) { trades = append(trades, trade) })

	executor := newConservativeBarExecutor(account, &stream, conservativeBarExecutorOptions{})
	base := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)
	executor.onKLineClosed(testConservativeBarKLine(base, 100, 101, 99, 100, 1000))
	if _, err := executor.SubmitOrders(
		context.Background(),
		types.SubmitOrder{
			ClientOrderID: "buy-stop-market",
			Symbol:        "US.AAPL",
			Side:          types.SideTypeBuy,
			Type:          types.OrderTypeStopMarket,
			StopPrice:     fixedpoint.NewFromFloat(105),
			Quantity:      fixedpoint.NewFromFloat(1),
			Market:        testPineWorkerShortReplayMarket(),
		},
		types.SubmitOrder{
			ClientOrderID: "sell-stop-market",
			Symbol:        "US.AAPL",
			Side:          types.SideTypeSell,
			Type:          types.OrderTypeStopMarket,
			StopPrice:     fixedpoint.NewFromFloat(95),
			Quantity:      fixedpoint.NewFromFloat(1),
			Market:        testPineWorkerShortReplayMarket(),
		},
		types.SubmitOrder{
			ClientOrderID: "stop-limit",
			Symbol:        "US.AAPL",
			Side:          types.SideTypeBuy,
			Type:          types.OrderTypeStopLimit,
			StopPrice:     fixedpoint.NewFromFloat(104),
			Price:         fixedpoint.NewFromFloat(103),
			Quantity:      fixedpoint.NewFromFloat(1),
			Market:        testPineWorkerShortReplayMarket(),
		},
	); err != nil {
		t.Fatalf("SubmitOrders error = %v", err)
	}
	executor.onKLineClosed(testConservativeBarKLine(base.Add(time.Minute), 106, 107, 94, 100, 1000))
	executor.onKLineClosed(testConservativeBarKLine(base.Add(2*time.Minute), 102, 103, 101, 102, 1000))

	if len(trades) != 3 {
		t.Fatalf("trades len = %d, want 3: %#v", len(trades), trades)
	}
	if trades[0].Side != types.SideTypeBuy || trades[0].Price.String() != "106" {
		t.Fatalf("buy stop market trade = %#v, want BUY @ 106", trades[0])
	}
	if trades[1].Side != types.SideTypeSell || trades[1].Price.String() != "95" {
		t.Fatalf("sell stop market trade = %#v, want SELL @ 95", trades[1])
	}
	if trades[2].Price.String() != "102" {
		t.Fatalf("stop-limit trade = %#v, want open improvement @ 102", trades[2])
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

func TestConservativeBarExecutorLiquidityWarnings(t *testing.T) {
	account := types.NewAccount()
	stream := types.NewStandardStream()
	sink := &conservativeBarTestWarningSink{}
	base := time.Date(2026, time.June, 29, 9, 30, 0, 0, time.UTC)

	t.Run("below step", func(t *testing.T) {
		executor := newConservativeBarExecutor(account, &stream, conservativeBarExecutorOptions{WarningSink: sink})
		executor.onKLineClosed(testConservativeBarKLine(base, 100, 101, 99, 100, 1000))
		market := testPineWorkerShortReplayMarket()
		market.StepSize = fixedpoint.NewFromFloat(1)
		if _, err := executor.SubmitOrders(context.Background(), types.SubmitOrder{
			ClientOrderID: "below-step",
			Symbol:        "US.AAPL",
			Side:          types.SideTypeBuy,
			Type:          types.OrderTypeMarket,
			Quantity:      fixedpoint.NewFromFloat(1),
			Market:        market,
		}); err != nil {
			t.Fatalf("SubmitOrders error = %v", err)
		}
		executor.onKLineClosed(testConservativeBarKLine(base.Add(time.Minute), 100, 101, 99, 100, 0.5))
		if len(sink.warnings) != 1 || !strings.Contains(sink.warnings[0], "below tradable quantity step") {
			t.Fatalf("warnings = %#v", sink.warnings)
		}
	})

	t.Run("below min", func(t *testing.T) {
		localSink := &conservativeBarTestWarningSink{}
		executor := newConservativeBarExecutor(account, &stream, conservativeBarExecutorOptions{WarningSink: localSink})
		executor.onKLineClosed(testConservativeBarKLine(base, 100, 101, 99, 100, 1000))
		market := testPineWorkerShortReplayMarket()
		market.MinQuantity = fixedpoint.NewFromFloat(2)
		if _, err := executor.SubmitOrders(context.Background(), types.SubmitOrder{
			ClientOrderID: "below-min",
			Symbol:        "US.AAPL",
			Side:          types.SideTypeBuy,
			Type:          types.OrderTypeMarket,
			Quantity:      fixedpoint.NewFromFloat(1),
			Market:        market,
		}); err != nil {
			t.Fatalf("SubmitOrders error = %v", err)
		}
		executor.onKLineClosed(testConservativeBarKLine(base.Add(time.Minute), 100, 101, 99, 100, 10))
		if len(localSink.warnings) != 1 || !strings.Contains(localSink.warnings[0], "below min quantity 2") {
			t.Fatalf("warnings = %#v", localSink.warnings)
		}
	})
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
