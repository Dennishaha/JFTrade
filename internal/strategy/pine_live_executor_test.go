package strategy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

func TestNewLiveCommandExecutorRetainsInjectedControlPorts(t *testing.T) {
	orders := &fakeWorkerOrderExecutor{}
	markets := fakeWorkerMarketResolver{"US.AAPL": testLiveCommandMarket()}
	sizer := fixedPineWorkerCommandSizer{quantity: fixedpoint.One}
	warnings := &recordingIgnoredOrderWarnings{}
	executor := NewLiveCommandExecutor(LiveCommandExecutorOptions{
		Symbol:                         "US.AAPL",
		OrderExecutor:                  orders,
		MarketResolver:                 markets,
		PositionSizer:                  sizer,
		WarningSink:                    warnings,
		ClientOrderIDPrefix:            "live-instance",
		RejectOrdersWithoutMarketRules: true,
	})
	if executor.Symbol != "US.AAPL" ||
		executor.OrderExecutor != orders ||
		executor.ClientOrderIDPrefix != "live-instance" ||
		!executor.RejectOrdersWithoutMarketRules {
		t.Fatalf("executor options = %#v", executor)
	}
}

func TestLiveCommandExecutorSubmitsOrders(t *testing.T) {
	orderExecutor := &fakeWorkerOrderExecutor{}
	commandExecutor := validLiveCommandExecutor(orderExecutor)

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

func TestLiveCommandExecutorRejectsQuantityPctWithoutSizing(t *testing.T) {
	commandExecutor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{})
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

func TestLiveCommandExecutorRejectsMissingQuantity(t *testing.T) {
	commandExecutor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{})
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

func TestLiveCommandExecutorSizesEntryQuantityPctFromEquity(t *testing.T) {
	account := types.NewAccount()
	account.UpdateBalances(types.BalanceMap{
		"USD": {Currency: "USD", Available: fixedpoint.NewFromFloat(1000)},
	})
	sizer := newLiveCommandTestSizer("US.AAPL", "USD", account)
	sizer.onKLineClosed(liveCommandTestKLine(time.Now(), 100))

	commandExecutor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{})
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

func TestLiveCommandExecutorSizesCloseQuantityPctFromPosition(t *testing.T) {
	account := types.NewAccount()
	sizer := newLiveCommandTestSizer("US.AAPL", "USD", account)
	sizer.onOrderUpdate(types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol:   "US.AAPL",
			Side:     types.SideTypeBuy,
			Quantity: fixedpoint.NewFromFloat(10),
		},
		Status:           types.OrderStatusFilled,
		ExecutedQuantity: fixedpoint.NewFromFloat(10),
	})

	commandExecutor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{})
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

func TestLiveCommandExecutorDefaultsCloseToFullPosition(t *testing.T) {
	account := types.NewAccount()
	sizer := newLiveCommandTestSizer("US.AAPL", "USD", account)
	sizer.onOrderUpdate(types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol:   "US.AAPL",
			Side:     types.SideTypeBuy,
			Quantity: fixedpoint.NewFromFloat(3),
		},
		Status:           types.OrderStatusFilled,
		ExecutedQuantity: fixedpoint.NewFromFloat(3),
	})

	commandExecutor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{})
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

func TestLiveCommandExecutorTagsShortOrders(t *testing.T) {
	commandExecutor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{})
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
	if order.Tag != pineWorkerShortOrderTag {
		t.Fatalf("Tag = %q, want %q", order.Tag, pineWorkerShortOrderTag)
	}
}

func TestLiveCommandExecutorIgnoresCloseWithoutPosition(t *testing.T) {
	account := types.NewAccount()
	sizer := newLiveCommandTestSizer("US.AAPL", "USD", account)
	orderExecutor := &fakeWorkerOrderExecutor{}
	warnings := &recordingIgnoredOrderWarnings{}
	commandExecutor := validLiveCommandExecutor(orderExecutor)
	commandExecutor.PositionSizer = sizer
	commandExecutor.WarningSink = warnings

	err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{
		Kind:      "close",
		ID:        "initial-sell",
		Direction: "long",
		Side:      types.SideTypeSell,
		Quantity:  1,
		BarIndex:  40,
	})
	if err != nil {
		t.Fatalf("Execute close without position error = %v", err)
	}
	if len(orderExecutor.submitted) != 0 {
		t.Fatalf("submitted = %#v, want none", orderExecutor.submitted)
	}
	if warnings.ignored != 1 || len(warnings.messages) != 1 || !strings.Contains(warnings.messages[0], "bar 40: ignored close command") {
		t.Fatalf("warnings = %#v ignored=%d", warnings.messages, warnings.ignored)
	}
}

func TestLiveCommandExecutorIgnoresQuantityBelowMarketStep(t *testing.T) {
	orderExecutor := &fakeWorkerOrderExecutor{}
	warnings := &recordingIgnoredOrderWarnings{}
	commandExecutor := &LiveCommandExecutor{
		Symbol:         "HK.00700",
		OrderExecutor:  orderExecutor,
		MarketResolver: fakeWorkerMarketResolver{"HK.00700": testPineWorkerHKMarket()},
		WarningSink:    warnings,
	}

	err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{
		Kind:      "entry",
		ID:        "fractional-hk",
		Direction: "long",
		Side:      types.SideTypeBuy,
		Quantity:  0.5,
		BarIndex:  152,
	})
	if err != nil {
		t.Fatalf("Execute fractional HK order error = %v", err)
	}
	if len(orderExecutor.submitted) != 0 {
		t.Fatalf("submitted = %#v, want none", orderExecutor.submitted)
	}
	if warnings.ignored != 1 || len(warnings.messages) != 1 || !strings.Contains(warnings.messages[0], "quantity is below the market quantity step") {
		t.Fatalf("warnings = %#v ignored=%d", warnings.messages, warnings.ignored)
	}
}

func TestLiveCommandExecutorIgnoresHKOddLotBelowBoardLot(t *testing.T) {
	orderExecutor := &fakeWorkerOrderExecutor{}
	warnings := &recordingIgnoredOrderWarnings{}
	commandExecutor := &LiveCommandExecutor{
		Symbol:         "HK.00700",
		OrderExecutor:  orderExecutor,
		MarketResolver: fakeWorkerMarketResolver{"HK.00700": testPineWorkerHKMarket()},
		WarningSink:    warnings,
	}

	err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{
		Kind:      "entry",
		ID:        "odd-lot-hk",
		Direction: "long",
		Side:      types.SideTypeBuy,
		Quantity:  50,
		BarIndex:  152,
	})
	if err != nil {
		t.Fatalf("Execute odd-lot HK order error = %v", err)
	}
	if len(orderExecutor.submitted) != 0 {
		t.Fatalf("submitted = %#v, want none", orderExecutor.submitted)
	}
	if warnings.ignored != 1 || len(warnings.messages) != 1 || !strings.Contains(warnings.messages[0], "quantity is below the market quantity step") {
		t.Fatalf("warnings = %#v ignored=%d", warnings.messages, warnings.ignored)
	}
}

func TestLiveCommandExecutorIgnoresOrdersWhenMarketRulesUnavailable(t *testing.T) {
	orderExecutor := &fakeWorkerOrderExecutor{}
	warnings := &recordingIgnoredOrderWarnings{}
	commandExecutor := &LiveCommandExecutor{
		Symbol:                         "HK.00700",
		OrderExecutor:                  orderExecutor,
		MarketResolver:                 fakeWorkerMarketResolver{"HK.00700": testPineWorkerHKInferredMarket()},
		WarningSink:                    warnings,
		RejectOrdersWithoutMarketRules: true,
	}

	err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{
		Kind:      "entry",
		ID:        "unresolved-lot-hk",
		Direction: "long",
		Side:      types.SideTypeBuy,
		Quantity:  1,
		BarIndex:  152,
	})
	if err != nil {
		t.Fatalf("Execute unresolved HK market order error = %v", err)
	}
	if len(orderExecutor.submitted) != 0 {
		t.Fatalf("submitted = %#v, want none", orderExecutor.submitted)
	}
	if warnings.ignored != 1 || len(warnings.messages) != 1 || !strings.Contains(warnings.messages[0], "market quantity rules are unavailable") {
		t.Fatalf("warnings = %#v ignored=%d", warnings.messages, warnings.ignored)
	}
}

func TestLiveCommandExecutorGroupsRepeatedIgnoredOrderWarnings(t *testing.T) {
	orderExecutor := &fakeWorkerOrderExecutor{}
	result := &groupedLiveWarnings{}
	commandExecutor := &LiveCommandExecutor{
		Symbol:                         "HK.00700",
		OrderExecutor:                  orderExecutor,
		MarketResolver:                 fakeWorkerMarketResolver{"HK.00700": testPineWorkerHKInferredMarket()},
		WarningSink:                    result,
		RejectOrdersWithoutMarketRules: true,
	}

	for bar := range 2 {
		err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{
			Kind:      "entry",
			ID:        "unresolved-lot-hk",
			Direction: "long",
			Side:      types.SideTypeBuy,
			Quantity:  1,
			BarIndex:  bar,
		})
		if err != nil {
			t.Fatalf("Execute unresolved HK market order %d error = %v", bar, err)
		}
	}
	if len(orderExecutor.submitted) != 0 {
		t.Fatalf("submitted = %#v, want none", orderExecutor.submitted)
	}
	if result.IgnoredOrders != 2 || result.WarningTotal != 1 || len(result.Warnings) != 1 {
		t.Fatalf("warnings ignored=%d total=%d list=%#v", result.IgnoredOrders, result.WarningTotal, result.Warnings)
	}
	if !strings.Contains(result.Warnings[0], "market quantity rules are unavailable") || !strings.Contains(result.Warnings[0], "occurred 2 times; first occurrence shown") {
		t.Fatalf("grouped warning = %q", result.Warnings[0])
	}
}

func TestLiveCommandExecutorAutoCloseCoversShortPosition(t *testing.T) {
	account := types.NewAccount()
	sizer := newLiveCommandTestSizer("US.AAPL", "USD", account)
	sizer.onOrderUpdate(types.Order{
		SubmitOrder: types.SubmitOrder{
			Symbol:   "US.AAPL",
			Side:     types.SideTypeSell,
			Quantity: fixedpoint.NewFromFloat(3),
		},
		Status:           types.OrderStatusFilled,
		ExecutedQuantity: fixedpoint.NewFromFloat(3),
	})

	orderExecutor := &fakeWorkerOrderExecutor{}
	commandExecutor := validLiveCommandExecutor(orderExecutor)
	commandExecutor.PositionSizer = sizer

	err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{
		Kind:     "close",
		ID:       "auto-close",
		Quantity: 1,
	})
	if err != nil {
		t.Fatalf("Execute auto close error = %v", err)
	}
	if len(orderExecutor.submitted) != 1 {
		t.Fatalf("submitted len = %d, want 1", len(orderExecutor.submitted))
	}
	order := orderExecutor.submitted[0]
	if order.Side != types.SideTypeBuy || order.Tag != pineWorkerShortOrderTag {
		t.Fatalf("auto close order = %#v, want synthetic short cover", order)
	}
}

func TestLiveCommandExecutorHonorsExplicitShortCloseAndOptionalWarnings(t *testing.T) {
	t.Run("explicit short close buys back the open short", func(t *testing.T) {
		account := types.NewAccount()
		sizer := newLiveCommandTestSizer("US.AAPL", "USD", account)
		sizer.onOrderUpdate(types.Order{
			SubmitOrder: types.SubmitOrder{
				Symbol: "US.AAPL", Side: types.SideTypeSell, Quantity: fixedpoint.NewFromFloat(2),
			},
			Status: types.OrderStatusFilled, ExecutedQuantity: fixedpoint.NewFromFloat(2),
		})
		orders := &fakeWorkerOrderExecutor{}
		executor := validLiveCommandExecutor(orders)
		executor.PositionSizer = sizer

		if err := executor.Execute(context.Background(), WorkerOrderCommand{
			Kind: "close", ID: "close-known-short", Direction: "short", Quantity: 1,
		}); err != nil {
			t.Fatalf("Execute explicit short close: %v", err)
		}
		if len(orders.submitted) != 1 || orders.submitted[0].Side != types.SideTypeBuy || orders.submitted[0].Tag != pineWorkerShortOrderTag {
			t.Fatalf("explicit short close order = %#v", orders.submitted)
		}
	})

	t.Run("missing warning collector leaves an ignored close successful", func(t *testing.T) {
		executor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{})
		executor.PositionSizer = newLiveCommandTestSizer("US.AAPL", "USD", types.NewAccount())
		if err := executor.Execute(context.Background(), WorkerOrderCommand{
			Kind: "close", ID: "no-open-position", Direction: "long", Quantity: 1,
		}); err != nil {
			t.Fatalf("ignored close without warning sink: %v", err)
		}
	})

	t.Run("default full-close sizing surfaces an unavailable position", func(t *testing.T) {
		executor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{})
		executor.PositionSizer = newLiveCommandTestSizer("US.AAPL", "USD", types.NewAccount())
		if _, err := executor.SubmitOrderFromCommand(WorkerOrderCommand{
			Kind: "close", ID: "full-close-without-position", Side: types.SideTypeSell,
		}); err == nil || !strings.Contains(err.Error(), "requires an open position") {
			t.Fatalf("full close sizing error = %v", err)
		}
	})
}

func TestLiveCommandExecutorCancelsTrackedOrders(t *testing.T) {
	orderExecutor := &fakeWorkerOrderExecutor{}
	commandExecutor := validLiveCommandExecutor(orderExecutor)
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

func TestLiveCommandExecutorCancelAll(t *testing.T) {
	orderExecutor := &fakeWorkerOrderExecutor{}
	commandExecutor := validLiveCommandExecutor(orderExecutor)
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

func TestLiveCommandExecutorRejectsAtomicBracketBeforeAnySubmission(t *testing.T) {
	orders := &fakeWorkerOrderExecutor{}
	executor := validLiveCommandExecutor(orders)
	err := executor.ExecuteBarCommands(t.Context(), pineWorkerAtomicBracketCommands())
	if err == nil || !strings.Contains(err.Error(), "atomic placement capability") {
		t.Fatalf("ExecuteBarCommands error = %v, want atomic capability rejection", err)
	}
	if len(orders.submitted) != 0 {
		t.Fatalf("ordinary submissions = %#v, want none", orders.submitted)
	}
}

func TestLiveCommandExecutorSubmitsParentOCOBracketAtomically(t *testing.T) {
	orders := &fakeAtomicWorkerOrderExecutor{}
	executor := &LiveCommandExecutor{
		Symbol: "US.AAPL", OrderExecutor: orders,
		MarketResolver: fakeWorkerMarketResolver{"US.AAPL": testLiveCommandMarket()},
	}
	if err := executor.ExecuteBarCommands(t.Context(), pineWorkerAtomicBracketCommands()); err != nil {
		t.Fatalf("ExecuteBarCommands: %v", err)
	}
	if len(orders.atomicGroups) != 1 || orders.atomicGroups[0] != "bracket-1" || len(orders.atomicOrders) != 1 || len(orders.atomicOrders[0]) != 3 {
		t.Fatalf("atomic calls groups=%#v orders=%#v", orders.atomicGroups, orders.atomicOrders)
	}
	if len(orders.submitted) != 0 {
		t.Fatalf("ordinary submissions = %#v, want none", orders.submitted)
	}
	for index, leg := range orders.atomicOrders[0] {
		if index == 0 {
			if leg.ParentID != "" || leg.ReduceOnly {
				t.Fatalf("entry leg = %#v", leg)
			}
			continue
		}
		if leg.ParentID != "long" || leg.OCOGroupID != "protect-oco" || !leg.ReduceOnly || !leg.Order.ReduceOnly || leg.Order.GroupID == 0 {
			t.Fatalf("protective leg %d = %#v", index, leg)
		}
	}
	if err := executor.Execute(t.Context(), WorkerOrderCommand{Kind: "cancel", ID: "protect"}); err != nil {
		t.Fatalf("cancel logical OCO intent: %v", err)
	}
	if len(orders.cancelled) != 2 {
		t.Fatalf("cancelled OCO legs = %#v, want 2", orders.cancelled)
	}
}

func TestLiveCommandExecutorRejectsMalformedAtomicBracket(t *testing.T) {
	orders := &fakeAtomicWorkerOrderExecutor{}
	executor := &LiveCommandExecutor{
		Symbol: "US.AAPL", OrderExecutor: orders,
		MarketResolver: fakeWorkerMarketResolver{"US.AAPL": testLiveCommandMarket()},
	}
	commands := pineWorkerAtomicBracketCommands()
	commands[1].ParentID = "missing-entry"
	if err := executor.ExecuteBarCommands(t.Context(), commands); err == nil || !strings.Contains(err.Error(), "no matching parent entry") {
		t.Fatalf("malformed bracket error = %v", err)
	}
	if len(orders.atomicOrders) != 0 || len(orders.submitted) != 0 {
		t.Fatalf("malformed bracket caused side effects: atomic=%#v submitted=%#v", orders.atomicOrders, orders.submitted)
	}
}

func pineWorkerAtomicBracketCommands() []WorkerOrderCommand {
	return []WorkerOrderCommand{
		{Kind: "entry", ID: "long", IntentID: "long", Side: types.SideTypeBuy, Quantity: 1, AtomicGroupID: "bracket-1"},
		{Kind: "exit", ID: "protect:limit", IntentID: "protect", ParentID: "long", Side: types.SideTypeSell, OrderType: types.OrderTypeLimit, Quantity: 1, LimitPrice: 110, AtomicGroupID: "bracket-1", OCOGroupID: "protect-oco", ReduceOnly: true},
		{Kind: "exit", ID: "protect:stop", IntentID: "protect", ParentID: "long", Side: types.SideTypeSell, OrderType: types.OrderTypeStopMarket, Quantity: 1, StopPrice: 95, AtomicGroupID: "bracket-1", OCOGroupID: "protect-oco", ReduceOnly: true},
	}
}

func TestLiveCommandExecutorPropagatesExecutorErrors(t *testing.T) {
	submitErr := errors.New("submit failed")
	commandExecutor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{submitErr: submitErr})
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
	commandExecutor = validLiveCommandExecutor(orderExecutor)
	commandExecutor.activeOrders = map[string]types.Order{"long": {SubmitOrder: types.SubmitOrder{ClientOrderID: "long"}}}
	err = commandExecutor.Execute(context.Background(), WorkerOrderCommand{Kind: "cancel", ID: "long"})
	if err == nil || !strings.Contains(err.Error(), "cancel failed") {
		t.Fatalf("cancel error = %v", err)
	}
}

func TestLiveCommandExecutorBusinessBoundaryErrors(t *testing.T) {
	t.Run("unsupported command kind is rejected", func(t *testing.T) {
		commandExecutor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{})
		err := commandExecutor.Execute(context.Background(), WorkerOrderCommand{Kind: "unknown"})
		if err == nil || !strings.Contains(err.Error(), "unsupported pine worker command kind") {
			t.Fatalf("Execute(unsupported) error = %v", err)
		}
	})

	t.Run("missing collaborators and symbol fail before order creation", func(t *testing.T) {
		if _, err := (&LiveCommandExecutor{}).SubmitOrderFromCommand(WorkerOrderCommand{}); err == nil || !strings.Contains(err.Error(), "order executor") {
			t.Fatalf("missing order executor error = %v", err)
		}
		if _, err := (&LiveCommandExecutor{OrderExecutor: &fakeWorkerOrderExecutor{}}).SubmitOrderFromCommand(WorkerOrderCommand{}); err == nil || !strings.Contains(err.Error(), "market resolver") {
			t.Fatalf("missing market resolver error = %v", err)
		}
		if _, err := (&LiveCommandExecutor{OrderExecutor: &fakeWorkerOrderExecutor{}, MarketResolver: fakeWorkerMarketResolver{}}).SubmitOrderFromCommand(WorkerOrderCommand{}); err == nil || !strings.Contains(err.Error(), "symbol is required") {
			t.Fatalf("missing symbol error = %v", err)
		}
		commandExecutor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{})
		commandExecutor.MarketResolver = fakeWorkerMarketResolver{}
		if _, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{}); err == nil || !strings.Contains(err.Error(), "is not loaded") {
			t.Fatalf("missing market error = %v", err)
		}
	})

	t.Run("side and non-positive sizing are rejected", func(t *testing.T) {
		commandExecutor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{})
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

func TestLiveCommandExecutorGeneratedOrderIDStopsAndTrackingFallbacks(t *testing.T) {
	orderExecutor := &fakeWorkerOrderExecutor{}
	commandExecutor := validLiveCommandExecutor(orderExecutor)
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
	namedOrder, err := commandExecutor.SubmitOrderFromCommand(WorkerOrderCommand{
		Kind: "entry", ID: "Long", Side: types.SideTypeBuy, Quantity: 1, BarIndex: 43,
	})
	if err != nil {
		t.Fatalf("SubmitOrderFromCommand(named) error = %v", err)
	}
	if namedOrder.ClientOrderID != "run-Long-43" {
		t.Fatalf("named live ClientOrderID = %q", namedOrder.ClientOrderID)
	}
	if order.Type != types.OrderTypeMarket || order.StopPrice.Float64() != 95.25 {
		t.Fatalf("generated market stop order = %#v", order)
	}

	defaultID := (&LiveCommandExecutor{}).clientOrderID(WorkerOrderCommand{BarIndex: 7})
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

func TestLiveCommandExecutorCancelBoundaries(t *testing.T) {
	commandExecutor := validLiveCommandExecutor(&fakeWorkerOrderExecutor{})
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
	commandExecutor = validLiveCommandExecutor(orderExecutor)
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

func validLiveCommandExecutor(orderExecutor *fakeWorkerOrderExecutor) *LiveCommandExecutor {
	return &LiveCommandExecutor{
		Symbol:         "US.AAPL",
		OrderExecutor:  orderExecutor,
		MarketResolver: fakeWorkerMarketResolver{"US.AAPL": testLiveCommandMarket()},
	}
}

type fakeWorkerOrderExecutor struct {
	submitted []types.SubmitOrder
	cancelled []types.Order
	submitErr error
	cancelErr error
}

type fakeAtomicWorkerOrderExecutor struct {
	fakeWorkerOrderExecutor
	atomicGroups []string
	atomicOrders [][]LiveAtomicOrder
}

func (executor *fakeAtomicWorkerOrderExecutor) SubmitAtomicPineOrders(
	_ context.Context,
	groupID string,
	orders ...LiveAtomicOrder,
) (types.OrderSlice, error) {
	if executor.submitErr != nil {
		return nil, executor.submitErr
	}
	executor.atomicGroups = append(executor.atomicGroups, groupID)
	executor.atomicOrders = append(executor.atomicOrders, append([]LiveAtomicOrder(nil), orders...))
	created := make(types.OrderSlice, 0, len(orders))
	for _, order := range orders {
		created = append(created, types.Order{SubmitOrder: order.Order, Status: types.OrderStatusNew})
	}
	return created, nil
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

func testPineWorkerHKMarket() types.Market {
	boardLot := fixedpoint.NewFromFloat(100)
	return types.Market{
		Exchange:        types.ExchangeName("futu"),
		Symbol:          "HK.00700",
		LocalSymbol:     "HK.00700",
		PricePrecision:  3,
		VolumePrecision: 0,
		QuotePrecision:  3,
		BaseCurrency:    "HK.00700",
		QuoteCurrency:   "HKD",
		MinQuantity:     boardLot,
		StepSize:        boardLot,
		TickSize:        fixedpoint.NewFromFloat(0.001),
	}
}

func testPineWorkerHKInferredMarket() types.Market {
	return types.Market{
		Exchange:        types.ExchangeName("futu"),
		Symbol:          "HK.00700",
		LocalSymbol:     "HK.00700",
		PricePrecision:  3,
		VolumePrecision: 0,
		QuotePrecision:  3,
		BaseCurrency:    "HK.00700",
		QuoteCurrency:   "HKD",
		MinQuantity:     fixedpoint.One,
		StepSize:        fixedpoint.One,
		TickSize:        fixedpoint.NewFromFloat(0.001),
	}
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

type recordingIgnoredOrderWarnings struct {
	messages []string
	ignored  int
}

func (warnings *recordingIgnoredOrderWarnings) AddIgnoredOrderWarning(message string) {
	warnings.ignored++
	warnings.messages = append(warnings.messages, message)
}
