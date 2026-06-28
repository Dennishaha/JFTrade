package backtest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
)

type PineWorkerOrderExecutor interface {
	SubmitOrders(context.Context, ...types.SubmitOrder) (types.OrderSlice, error)
	CancelOrders(context.Context, ...types.Order) error
}

type PineWorkerMarketResolver interface {
	Market(symbol string) (types.Market, bool)
}

type PineWorkerCommandExecutor struct {
	Symbol              string
	OrderExecutor       PineWorkerOrderExecutor
	MarketResolver      PineWorkerMarketResolver
	ClientOrderIDPrefix string

	activeOrders map[string]types.Order
}

func (executor *PineWorkerCommandExecutor) ExecuteBarCommands(ctx context.Context, commands []WorkerOrderCommand) error {
	for _, command := range commands {
		if err := executor.Execute(ctx, command); err != nil {
			return err
		}
	}
	return nil
}

func (executor *PineWorkerCommandExecutor) Execute(ctx context.Context, command WorkerOrderCommand) error {
	switch normalizeWorkerIntentKind(command.Kind) {
	case "entry", "order", "exit", "close", "close_all":
		return executor.submit(ctx, command)
	case "cancel":
		return executor.cancel(ctx, command.ID)
	case "cancel_all":
		return executor.cancelAll(ctx)
	default:
		return fmt.Errorf("unsupported pine worker command kind: %s", command.Kind)
	}
}

func (executor *PineWorkerCommandExecutor) submit(ctx context.Context, command WorkerOrderCommand) error {
	order, err := executor.SubmitOrderFromCommand(command)
	if err != nil {
		return err
	}
	createdOrders, err := executor.OrderExecutor.SubmitOrders(ctx, order)
	if err != nil {
		return fmt.Errorf("submit pine worker command %s: %w", command.ID, err)
	}
	executor.trackCreatedOrders(command, createdOrders)
	return nil
}

func (executor *PineWorkerCommandExecutor) SubmitOrderFromCommand(command WorkerOrderCommand) (types.SubmitOrder, error) {
	if executor.OrderExecutor == nil {
		return types.SubmitOrder{}, fmt.Errorf("pine worker order executor is required")
	}
	if executor.MarketResolver == nil {
		return types.SubmitOrder{}, fmt.Errorf("pine worker market resolver is required")
	}
	symbol := strings.TrimSpace(executor.Symbol)
	if symbol == "" {
		return types.SubmitOrder{}, fmt.Errorf("pine worker command symbol is required")
	}
	market, ok := executor.MarketResolver.Market(symbol)
	if !ok {
		return types.SubmitOrder{}, fmt.Errorf("market %s is not loaded in this session", symbol)
	}
	if command.Side == "" {
		return types.SubmitOrder{}, fmt.Errorf("pine worker command %s side is required", command.Kind)
	}
	if command.QuantityPct > 0 {
		return types.SubmitOrder{}, fmt.Errorf("pine worker command %s quantity pct requires position sizing", command.ID)
	}
	if command.Quantity <= 0 {
		return types.SubmitOrder{}, fmt.Errorf("pine worker command %s quantity must be positive", command.ID)
	}
	orderType := command.OrderType
	if orderType == "" {
		orderType = types.OrderTypeMarket
	}
	order := types.SubmitOrder{
		ClientOrderID: executor.clientOrderID(command),
		Symbol:        symbol,
		Side:          command.Side,
		Type:          orderType,
		Quantity:      fixedpoint.NewFromFloat(command.Quantity),
		Market:        market,
	}
	if command.LimitPrice > 0 {
		order.Price = fixedpoint.NewFromFloat(command.LimitPrice)
		order.TimeInForce = types.TimeInForceGTC
	}
	if command.StopPrice > 0 {
		order.StopPrice = fixedpoint.NewFromFloat(command.StopPrice)
	}
	return order, nil
}

func (executor *PineWorkerCommandExecutor) cancel(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("pine worker cancel command id is required")
	}
	order, ok := executor.activeOrders[strings.TrimSpace(id)]
	if !ok {
		return nil
	}
	if err := executor.OrderExecutor.CancelOrders(ctx, order); err != nil {
		return fmt.Errorf("cancel pine worker command %s: %w", id, err)
	}
	delete(executor.activeOrders, strings.TrimSpace(id))
	return nil
}

func (executor *PineWorkerCommandExecutor) cancelAll(ctx context.Context) error {
	if len(executor.activeOrders) == 0 {
		return nil
	}
	orders := make([]types.Order, 0, len(executor.activeOrders))
	for _, order := range executor.activeOrders {
		orders = append(orders, order)
	}
	if err := executor.OrderExecutor.CancelOrders(ctx, orders...); err != nil {
		return fmt.Errorf("cancel all pine worker commands: %w", err)
	}
	clear(executor.activeOrders)
	return nil
}

func (executor *PineWorkerCommandExecutor) trackCreatedOrders(command WorkerOrderCommand, createdOrders types.OrderSlice) {
	if len(createdOrders) == 0 {
		return
	}
	if executor.activeOrders == nil {
		executor.activeOrders = make(map[string]types.Order)
	}
	key := strings.TrimSpace(command.ID)
	if key == "" {
		key = strings.TrimSpace(createdOrders[0].ClientOrderID)
	}
	if key == "" {
		return
	}
	executor.activeOrders[key] = createdOrders[0]
}

func (executor *PineWorkerCommandExecutor) clientOrderID(command WorkerOrderCommand) string {
	if trimmed := strings.TrimSpace(command.ID); trimmed != "" {
		return trimmed
	}
	prefix := strings.TrimSpace(executor.ClientOrderIDPrefix)
	if prefix == "" {
		prefix = "pine-worker"
	}
	return fmt.Sprintf("%s-%d-%d", prefix, command.BarIndex, time.Now().UnixNano())
}
