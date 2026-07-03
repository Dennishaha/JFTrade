package backtest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type PineWorkerOrderExecutor interface {
	SubmitOrders(context.Context, ...types.SubmitOrder) (types.OrderSlice, error)
	CancelOrders(context.Context, ...types.Order) error
}

type PineWorkerMarketResolver interface {
	Market(symbol string) (types.Market, bool)
}

type PineWorkerCommandExecutor struct {
	Symbol                         string
	OrderExecutor                  PineWorkerOrderExecutor
	MarketResolver                 PineWorkerMarketResolver
	PositionSizer                  pineWorkerCommandPositionSizer
	WarningSink                    pineWorkerIgnoredOrderWarner
	ClientOrderIDPrefix            string
	RejectOrdersWithoutMarketRules bool

	activeOrders map[string]types.Order
}

type pineWorkerCommandPositionSizer interface {
	QuantityForCommand(command WorkerOrderCommand, market types.Market) (fixedpoint.Value, error)
}

type pineWorkerCommandPositionReader interface {
	NetPosition() fixedpoint.Value
}

type pineWorkerIgnoredOrderWarner interface {
	AddIgnoredOrderWarning(string)
}

type pineWorkerIgnoredOrderGroupWarner interface {
	AddIgnoredOrderWarningGroup(string, string)
}

type pineWorkerIgnoredOrderError struct {
	reason string
}

func (err pineWorkerIgnoredOrderError) Error() string {
	return err.reason
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
	resolved, skip, err := executor.resolvePositionCloseCommand(command)
	if err != nil {
		return err
	}
	if skip {
		return nil
	}
	command = resolved
	order, err := executor.SubmitOrderFromCommand(command)
	if err != nil {
		if ignored, ok := err.(pineWorkerIgnoredOrderError); ok {
			executor.warnIgnoredOrder(command, ignored.reason)
			return nil
		}
		return err
	}
	createdOrders, err := executor.OrderExecutor.SubmitOrders(ctx, order)
	if err != nil {
		return fmt.Errorf("submit pine worker command %s: %w", command.ID, err)
	}
	executor.trackCreatedOrders(command, createdOrders)
	return nil
}

func (executor *PineWorkerCommandExecutor) resolvePositionCloseCommand(command WorkerOrderCommand) (WorkerOrderCommand, bool, error) {
	if !isPineWorkerPositionCloseCommand(command) {
		return command, false, nil
	}
	reader, ok := executor.PositionSizer.(pineWorkerCommandPositionReader)
	if !ok {
		return command, false, nil
	}
	netPosition := reader.NetPosition()
	direction := strings.TrimSpace(strings.ToLower(command.Direction))
	switch direction {
	case "", "flat", "auto":
		switch netPosition.Sign() {
		case 1:
			command.Direction = "long"
			command.Side = types.SideTypeSell
			return command, false, nil
		case -1:
			command.Direction = "short"
			command.Side = types.SideTypeBuy
			return command, false, nil
		default:
			executor.warnIgnoredOrder(command, "no open position is available")
			return command, true, nil
		}
	case "long", "sell":
		if netPosition.Sign() <= 0 {
			executor.warnIgnoredOrder(command, "no long position is open")
			return command, true, nil
		}
		command.Direction = "long"
		command.Side = types.SideTypeSell
		return command, false, nil
	case "short", "buy", "cover":
		if netPosition.Sign() >= 0 {
			executor.warnIgnoredOrder(command, "no short position is open")
			return command, true, nil
		}
		command.Direction = "short"
		command.Side = types.SideTypeBuy
		return command, false, nil
	default:
		return command, false, nil
	}
}

func (executor *PineWorkerCommandExecutor) warnIgnoredOrder(command WorkerOrderCommand, reason string) {
	if executor.WarningSink == nil {
		return
	}
	id := strings.TrimSpace(command.ID)
	if id == "" {
		id = strings.TrimSpace(command.FromEntry)
	}
	if id == "" {
		id = "<anonymous>"
	}
	symbol := strings.TrimSpace(executor.Symbol)
	if symbol == "" {
		symbol = "<unknown>"
	}
	kind := normalizeWorkerIntentKind(command.Kind)
	message := fmt.Sprintf(
		"bar %d: ignored %s command %q for %s because %s",
		command.BarIndex,
		kind,
		id,
		symbol,
		reason,
	)
	if grouped, ok := executor.WarningSink.(pineWorkerIgnoredOrderGroupWarner); ok {
		grouped.AddIgnoredOrderWarningGroup(strings.Join([]string{symbol, kind, id, reason}, "|"), message)
		return
	}
	executor.WarningSink.AddIgnoredOrderWarning(message)
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
	if executor.RejectOrdersWithoutMarketRules {
		return types.SubmitOrder{}, pineWorkerIgnoredOrderError{reason: "market quantity rules are unavailable"}
	}
	if command.Side == "" {
		return types.SubmitOrder{}, fmt.Errorf("pine worker command %s side is required", command.Kind)
	}
	quantity, err := executor.orderQuantity(command, market)
	if err != nil {
		return types.SubmitOrder{}, err
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
		Quantity:      quantity,
		Market:        market,
	}
	if isPineWorkerShortReplayCommand(command) {
		order.Tag = pineWorkerShortReplayOrderTag
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

func (executor *PineWorkerCommandExecutor) orderQuantity(command WorkerOrderCommand, market types.Market) (fixedpoint.Value, error) {
	if command.QuantityPct > 0 {
		if executor.PositionSizer == nil {
			return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct requires position sizing", command.ID)
		}
		quantity, err := executor.PositionSizer.QuantityForCommand(command, market)
		if err != nil {
			return fixedpoint.Zero, err
		}
		return requireTradablePineWorkerCommandQuantity(command, market, quantity)
	}
	if command.QuantityPct < 0 {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity pct must be positive", command.ID)
	}
	if command.Quantity > 0 {
		return requireTradablePineWorkerCommandQuantity(command, market, fixedpoint.NewFromFloat(command.Quantity))
	}
	if isPineWorkerPositionCloseCommand(command) && executor.PositionSizer != nil {
		command.QuantityPct = 100
		quantity, err := executor.PositionSizer.QuantityForCommand(command, market)
		if err != nil {
			return fixedpoint.Zero, err
		}
		return requireTradablePineWorkerCommandQuantity(command, market, quantity)
	}
	return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity must be positive", command.ID)
}

func requireTradablePineWorkerCommandQuantity(command WorkerOrderCommand, market types.Market, quantity fixedpoint.Value) (fixedpoint.Value, error) {
	if quantity.Sign() <= 0 {
		return fixedpoint.Zero, fmt.Errorf("pine worker command %s quantity must be positive", command.ID)
	}
	normalized := normalizePineWorkerOrderQuantity(market, quantity)
	if normalized.Sign() <= 0 {
		return fixedpoint.Zero, pineWorkerIgnoredOrderError{reason: "quantity is below the market quantity step"}
	}
	if market.MinQuantity.Sign() > 0 && normalized.Compare(market.MinQuantity) < 0 {
		return fixedpoint.Zero, pineWorkerIgnoredOrderError{
			reason: fmt.Sprintf("quantity %s is less than market min quantity %s", normalized.String(), market.MinQuantity.String()),
		}
	}
	return normalized, nil
}

func normalizePineWorkerOrderQuantity(market types.Market, quantity fixedpoint.Value) fixedpoint.Value {
	if quantity.Sign() <= 0 {
		return fixedpoint.Zero
	}
	if !market.StepSize.IsZero() {
		return market.TruncateQuantity(quantity)
	}
	if market.VolumePrecision > 0 {
		return market.RoundDownQuantityByPrecision(quantity)
	}
	return quantity
}

const pineWorkerShortReplayOrderTag = "pine-worker-short-replay"

func isPineWorkerShortReplayCommand(command WorkerOrderCommand) bool {
	direction := strings.TrimSpace(strings.ToLower(command.Direction))
	if direction != "short" {
		return false
	}
	switch normalizeWorkerIntentKind(command.Kind) {
	case "entry", "order", "exit", "close", "close_all":
		return true
	default:
		return false
	}
}

func isPineWorkerPositionCloseCommand(command WorkerOrderCommand) bool {
	switch normalizeWorkerIntentKind(command.Kind) {
	case "exit", "close", "close_all":
		return true
	default:
		return false
	}
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
