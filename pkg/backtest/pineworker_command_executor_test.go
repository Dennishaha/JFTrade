package backtest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

func TestPineWorkerCommandExecutorSubmitsOrders(t *testing.T) {
	orderExecutor := &fakeWorkerOrderExecutor{}
	commandExecutor := validPineWorkerCommandExecutor(orderExecutor)

	err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{
		Kind:       "entry",
		ID:         "long",
		Side:       types.SideTypeBuy,
		OrderType:  types.OrderTypeLimit,
		Quantity:   2,
		LimitPrice: 101.5,
	})
	if err != nil {
		t.Fatalf("Execute error = %v", err)
	}
	if len(orderExecutor.submitted) != 1 {
		t.Fatalf("submitted len = %d, want 1", len(orderExecutor.submitted))
	}
	order := orderExecutor.submitted[0]
	if order.ClientOrderID != "long" || order.Symbol != "US.AAPL" || order.Side != types.SideTypeBuy {
		t.Fatalf("submitted order = %#v", order)
	}
	if order.Type != types.OrderTypeLimit || order.Price.Float64() != 101.5 || order.TimeInForce != types.TimeInForceGTC {
		t.Fatalf("limit order fields = %#v", order)
	}
	if order.Quantity.Float64() != 2 {
		t.Fatalf("quantity = %s, want 2", order.Quantity)
	}
}

func TestPineWorkerCommandExecutorRejectsQuantityPctWithoutSizing(t *testing.T) {
	commandExecutor := validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{})
	_, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{
		Kind:        "exit",
		ID:          "half",
		Side:        types.SideTypeSell,
		OrderType:   types.OrderTypeMarket,
		QuantityPct: 50,
	})
	if err == nil || !strings.Contains(err.Error(), "position sizing") {
		t.Fatalf("error = %v, want position sizing", err)
	}
}

func TestPineWorkerCommandExecutorRejectsMissingQuantity(t *testing.T) {
	commandExecutor := validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{})
	_, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{
		Kind:      "entry",
		ID:        "zero",
		Side:      types.SideTypeBuy,
		OrderType: types.OrderTypeMarket,
	})
	if err == nil || !strings.Contains(err.Error(), "quantity must be positive") {
		t.Fatalf("error = %v, want quantity", err)
	}
}

func TestPineWorkerCommandExecutorSizesEntryQuantityPctFromEquity(t *testing.T) {
	account := types.NewAccount()
	account.UpdateBalances(types.BalanceMap{
		"USD": {Currency: "USD", Available: fixedpoint.NewFromFloat(1000)},
	})
	sizer := newPineWorkerReplaySizer("US.AAPL", "USD", account)
	sizer.onKLineClosed(testPineWorkerShortReplayKLine(time.Now(), 100))

	commandExecutor := validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{})
	commandExecutor.PositionSizer = sizer
	order, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{
		Kind:        "entry",
		ID:          "half-equity",
		Direction:   "long",
		Side:        types.SideTypeBuy,
		OrderType:   types.OrderTypeMarket,
		QuantityPct: 50,
	})
	if err != nil {
		t.Fatalf("SubmitOrderFromCommand error = %v", err)
	}
	if order.Quantity.Float64() != 5 {
		t.Fatalf("Quantity = %s, want 5", order.Quantity)
	}
}

func TestPineWorkerCommandExecutorSizesCloseQuantityPctFromPosition(t *testing.T) {
	account := types.NewAccount()
	sizer := newPineWorkerReplaySizer("US.AAPL", "USD", account)
	sizer.onOrderUpdate(types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol:   "US.AAPL",
			Side:     types.SideTypeBuy,
			Quantity: fixedpoint.NewFromFloat(10),
		},
		Status:           types.OrderStatusFilled,
		ExecutedQuantity: fixedpoint.NewFromFloat(10),
	})

	commandExecutor := validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{})
	commandExecutor.PositionSizer = sizer
	order, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{
		Kind:        "close",
		ID:          "half-position",
		Direction:   "long",
		Side:        types.SideTypeSell,
		OrderType:   types.OrderTypeMarket,
		QuantityPct: 50,
	})
	if err != nil {
		t.Fatalf("SubmitOrderFromCommand error = %v", err)
	}
	if order.Quantity.Float64() != 5 {
		t.Fatalf("Quantity = %s, want 5", order.Quantity)
	}
}

func TestPineWorkerCommandExecutorDefaultsCloseToFullPosition(t *testing.T) {
	account := types.NewAccount()
	sizer := newPineWorkerReplaySizer("US.AAPL", "USD", account)
	sizer.onOrderUpdate(types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol:   "US.AAPL",
			Side:     types.SideTypeBuy,
			Quantity: fixedpoint.NewFromFloat(3),
		},
		Status:           types.OrderStatusFilled,
		ExecutedQuantity: fixedpoint.NewFromFloat(3),
	})

	commandExecutor := validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{})
	commandExecutor.PositionSizer = sizer
	order, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{
		Kind:      "close",
		ID:        "all-position",
		Direction: "long",
		Side:      types.SideTypeSell,
		OrderType: types.OrderTypeMarket,
	})
	if err != nil {
		t.Fatalf("SubmitOrderFromCommand error = %v", err)
	}
	if order.Quantity.Float64() != 3 {
		t.Fatalf("Quantity = %s, want 3", order.Quantity)
	}
}

func TestPineWorkerCommandExecutorTagsShortReplayOrders(t *testing.T) {
	commandExecutor := validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{})
	order, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{
		Kind:      "entry",
		ID:        "short",
		Direction: "short",
		Side:      types.SideTypeSell,
		OrderType: types.OrderTypeMarket,
		Quantity:  2,
	})
	if err != nil {
		t.Fatalf("SubmitOrderFromCommand error = %v", err)
	}
	if order.Tag != pineWorkerShortReplayOrderTag {
		t.Fatalf("Tag = %q, want %q", order.Tag, pineWorkerShortReplayOrderTag)
	}
}

func TestPineWorkerCommandExecutorCancelsTrackedOrders(t *testing.T) {
	orderExecutor := &fakeWorkerOrderExecutor{}
	commandExecutor := validPineWorkerCommandExecutor(orderExecutor)
	err := commandExecutor.ExecuteBarCommands(context.Background(), []WorkerOrderCommand{
		{Kind: "entry", ID: "long", Side: types.SideTypeBuy, OrderType: types.OrderTypeMarket, Quantity: 1},
		{Kind: "cancel", ID: "long"},
	})
	if err != nil {
		t.Fatalf("ExecuteBarCommands error = %v", err)
	}
	if len(orderExecutor.cancelled) != 1 || orderExecutor.cancelled[0].ClientOrderID != "long" {
		t.Fatalf("cancelled = %#v", orderExecutor.cancelled)
	}
}

func TestPineWorkerCommandExecutorCancelAll(t *testing.T) {
	orderExecutor := &fakeWorkerOrderExecutor{}
	commandExecutor := validPineWorkerCommandExecutor(orderExecutor)
	err := commandExecutor.ExecuteBarCommands(context.Background(), []WorkerOrderCommand{
		{Kind: "entry", ID: "long", Side: types.SideTypeBuy, OrderType: types.OrderTypeMarket, Quantity: 1},
		{Kind: "order", ID: "short", Side: types.SideTypeSell, OrderType: types.OrderTypeMarket, Quantity: 1},
		{Kind: "cancel_all"},
	})
	if err != nil {
		t.Fatalf("ExecuteBarCommands error = %v", err)
	}
	if len(orderExecutor.cancelled) != 2 {
		t.Fatalf("cancelled len = %d, want 2", len(orderExecutor.cancelled))
	}
	if len(commandExecutor.activeOrders) != 0 {
		t.Fatalf("activeOrders = %#v, want empty", commandExecutor.activeOrders)
	}
}

func TestPineWorkerCommandExecutorPropagatesExecutorErrors(t *testing.T) {
	submitErr := errors.New("submit failed")
	commandExecutor := validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{submitErr: submitErr})
	err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{
		Kind:      "entry",
		ID:        "long",
		Side:      types.SideTypeBuy,
		OrderType: types.OrderTypeMarket,
		Quantity:  1,
	})
	if err == nil || !strings.Contains(err.Error(), "submit failed") {
		t.Fatalf("submit error = %v", err)
	}

	cancelErr := errors.New("cancel failed")
	orderExecutor := &fakeWorkerOrderExecutor{cancelErr: cancelErr}
	commandExecutor = validPineWorkerCommandExecutor(orderExecutor)
	commandExecutor.activeOrders = map[string]types.Order{"long": {SubmitOrder: types.SubmitOrder{ClientOrderID: "long"}}}
	err = commandExecutor.Execute(context.Background(), WorkerOrderCommand{Kind: "cancel", ID: "long"})
	if err == nil || !strings.Contains(err.Error(), "cancel failed") {
		t.Fatalf("cancel error = %v", err)
	}
}

func validPineWorkerCommandExecutor(orderExecutor *fakeWorkerOrderExecutor) *PineWorkerCommandExecutor {
	return &PineWorkerCommandExecutor{
		Symbol:         "US.AAPL",
		OrderExecutor:  orderExecutor,
		MarketResolver: fakeWorkerMarketResolver{"US.AAPL": testPineWorkerShortReplayMarket()},
	}
}

type fakeWorkerOrderExecutor struct {
	submitted []types.SubmitOrder
	cancelled []types.Order
	submitErr error
	cancelErr error
}

func (executor *fakeWorkerOrderExecutor) SubmitOrders(_ context.Context, orders ...types.SubmitOrder) (types.OrderSlice, error) {
	if executor.submitErr != nil {
		return nil, executor.submitErr
	}
	executor.submitted = append(executor.submitted, orders...)
	created := make(types.OrderSlice, 0, len(orders))
	for _, order := range orders {
		created = append(created, types.Order{
			SubmitOrder:      order,
			Status:           types.OrderStatusNew,
			ExecutedQuantity: fixedpoint.Zero,
		})
	}
	return created, nil
}

func (executor *fakeWorkerOrderExecutor) CancelOrders(_ context.Context, orders ...types.Order) error {
	if executor.cancelErr != nil {
		return executor.cancelErr
	}
	executor.cancelled = append(executor.cancelled, orders...)
	return nil
}

type fakeWorkerMarketResolver map[string]types.Market

func (resolver fakeWorkerMarketResolver) Market(symbol string) (types.Market, bool) {
	market, ok := resolver[symbol]
	return market, ok
}
