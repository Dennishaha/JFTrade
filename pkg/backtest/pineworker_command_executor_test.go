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

func TestPineWorkerCommandExecutorBusinessBoundaryErrors(t *testing.T) {
	t.Run("unsupported command kind is rejected", func(t *testing.T) {
		commandExecutor := validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{})
		err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{Kind: "unknown"})
		if err == nil || !strings.Contains(err.Error(), "unsupported pine worker command kind") {
			t.Fatalf("Execute(unsupported) error = %v", err)
		}
	})

	t.Run("missing collaborators and symbol fail before order creation", func(t *testing.T) {
		if _, err := (&PineWorkerCommandExecutor{}).SubmitOrderFromCommand(WorkerOrderCommand{}); err == nil || !strings.Contains(err.Error(), "order executor") {
			t.Fatalf("missing order executor error = %v", err)
		}
		if _, err := (&PineWorkerCommandExecutor{OrderExecutor: &fakeWorkerOrderExecutor{}}).SubmitOrderFromCommand(WorkerOrderCommand{}); err == nil || !strings.Contains(err.Error(), "market resolver") {
			t.Fatalf("missing market resolver error = %v", err)
		}
		if _, err := (&PineWorkerCommandExecutor{OrderExecutor: &fakeWorkerOrderExecutor{}, MarketResolver: fakeWorkerMarketResolver{}}).SubmitOrderFromCommand(WorkerOrderCommand{}); err == nil || !strings.Contains(err.Error(), "symbol is required") {
			t.Fatalf("missing symbol error = %v", err)
		}
		commandExecutor := validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{})
		commandExecutor.MarketResolver = fakeWorkerMarketResolver{}
		if _, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{}); err == nil || !strings.Contains(err.Error(), "is not loaded") {
			t.Fatalf("missing market error = %v", err)
		}
	})

	t.Run("side and non-positive sizing are rejected", func(t *testing.T) {
		commandExecutor := validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{})
		if _, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{Kind: "entry", Quantity: 1}); err == nil || !strings.Contains(err.Error(), "side is required") {
			t.Fatalf("missing side error = %v", err)
		}
		if _, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{Kind: "entry", ID: "bad-pct", Side: types.SideTypeBuy, QuantityPct: -1}); err == nil || !strings.Contains(err.Error(), "quantity pct must be positive") {
			t.Fatalf("negative pct error = %v", err)
		}
		commandExecutor.PositionSizer = fixedPineWorkerCommandSizer{quantity: fixedpoint.Zero}
		if _, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{Kind: "entry", ID: "zero-sized", Side: types.SideTypeBuy, QuantityPct: 50}); err == nil || !strings.Contains(err.Error(), "quantity must be positive") {
			t.Fatalf("zero sized error = %v", err)
		}
		commandExecutor.PositionSizer = fixedPineWorkerCommandSizer{err: errors.New("sizer failed")}
		if _, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{Kind: "entry", ID: "sizer-error", Side: types.SideTypeBuy, QuantityPct: 50}); err == nil || !strings.Contains(err.Error(), "sizer failed") {
			t.Fatalf("sizer error = %v", err)
		}
	})
}

func TestPineWorkerCommandExecutorGeneratedOrderIDStopsAndTrackingFallbacks(t *testing.T) {
	orderExecutor := &fakeWorkerOrderExecutor{}
	commandExecutor := validPineWorkerCommandExecutor(orderExecutor)
	commandExecutor.ClientOrderIDPrefix = " run "

	order, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{
		Kind:      "entry",
		Side:      types.SideTypeBuy,
		Quantity:  1.5,
		StopPrice: 95.25,
		BarIndex:  42,
	})
	if err != nil {
		t.Fatalf("SubmitOrderFromCommand() error = %v", err)
	}
	if !strings.HasPrefix(order.ClientOrderID, "run-42-") {
		t.Fatalf("generated ClientOrderID = %q", order.ClientOrderID)
	}
	if order.Type != types.OrderTypeMarket || order.StopPrice.Float64() != 95.25 {
		t.Fatalf("generated market stop order = %#v", order)
	}

	defaultID := (&PineWorkerCommandExecutor{}).clientOrderID(WorkerOrderCommand{BarIndex: 7})
	if !strings.HasPrefix(defaultID, "pine-worker-7-") {
		t.Fatalf("default clientOrderID = %q", defaultID)
	}

	commandExecutor.trackCreatedOrders(WorkerOrderCommand{}, nil)
	commandExecutor.trackCreatedOrders(WorkerOrderCommand{}, types.OrderSlice{{SubmitOrder: types.SubmitOrder{ClientOrderID: " created-id "}}})
	if _, ok := commandExecutor.activeOrders["created-id"]; !ok {
		t.Fatalf("activeOrders missing created-id: %#v", commandExecutor.activeOrders)
	}
	commandExecutor.trackCreatedOrders(WorkerOrderCommand{}, types.OrderSlice{{SubmitOrder: types.SubmitOrder{}}})
	if len(commandExecutor.activeOrders) != 1 {
		t.Fatalf("blank created order should not be tracked: %#v", commandExecutor.activeOrders)
	}
}

func TestPineWorkerCommandExecutorCancelBoundaries(t *testing.T) {
	commandExecutor := validPineWorkerCommandExecutor(&fakeWorkerOrderExecutor{})
	if err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{Kind: "cancel", ID: " "}); err == nil || !strings.Contains(err.Error(), "cancel command id is required") {
		t.Fatalf("blank cancel error = %v", err)
	}
	if err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{Kind: "cancel", ID: "missing"}); err != nil {
		t.Fatalf("cancel missing tracked order error = %v", err)
	}
	if err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{Kind: "cancel_all"}); err != nil {
		t.Fatalf("cancel_all empty error = %v", err)
	}

	cancelErr := errors.New("cancel all failed")
	orderExecutor := &fakeWorkerOrderExecutor{cancelErr: cancelErr}
	commandExecutor = validPineWorkerCommandExecutor(orderExecutor)
	commandExecutor.activeOrders = map[string]types.Order{
		"one": {SubmitOrder: types.SubmitOrder{ClientOrderID: "one"}},
		"two": {SubmitOrder: types.SubmitOrder{ClientOrderID: "two"}},
	}
	if err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{Kind: "cancel_all"}); err == nil || !strings.Contains(err.Error(), "cancel all failed") {
		t.Fatalf("cancel_all error = %v", err)
	}
	if len(commandExecutor.activeOrders) != 2 {
		t.Fatalf("activeOrders should remain after failed cancel_all: %#v", commandExecutor.activeOrders)
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

type fixedPineWorkerCommandSizer struct {
	quantity fixedpoint.Value
	err      error
}

func (sizer fixedPineWorkerCommandSizer) QuantityForCommand(WorkerOrderCommand, types.Market) (fixedpoint.Value, error) {
	if sizer.err != nil {
		return fixedpoint.Zero, sizer.err
	}
	return sizer.quantity, nil
}
