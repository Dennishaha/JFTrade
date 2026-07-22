package backtest

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type PineWorkerOrderExecutor interface {
	SubmitOrders(context.Context, ...types.SubmitOrder) (types.OrderSlice, error)
	CancelOrders(context.Context, ...types.Order) error
}

// PineWorkerAtomicOrder preserves the Pine parent/OCO semantics alongside the
// broker-neutral order. Implementations must accept every leg or no leg, keep
// children inactive until their parent fills, cancel OCO siblings after the
// first fill, and enforce ReduceOnly at match time.
type PineWorkerAtomicOrder struct {
	CommandID  string
	IntentID   string
	ParentID   string
	OCOGroupID string
	ReduceOnly bool
	Order      types.SubmitOrder
}

type PineWorkerAtomicOrderExecutor interface {
	SubmitAtomicPineOrders(context.Context, string, ...PineWorkerAtomicOrder) (types.OrderSlice, error)
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

	activeOrders       map[string]types.Order
	activeOrderAliases map[string][]string
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
	plans, err := executor.preflightBarCommands(commands)
	if err != nil {
		return err
	}
	executedGroups := make(map[string]struct{})
	for index, command := range commands {
		groupID := strings.TrimSpace(command.AtomicGroupID)
		if groupID == "" {
			if err := executor.executePlanned(ctx, plans[index]); err != nil {
				return err
			}
			continue
		}
		if _, done := executedGroups[groupID]; done {
			continue
		}
		executedGroups[groupID] = struct{}{}
		if err := executor.executeAtomicGroup(ctx, groupID, plans); err != nil {
			return err
		}
	}
	return nil
}

type pineWorkerCommandPlan struct {
	command WorkerOrderCommand
	order   types.SubmitOrder
	skip    bool
}

func (executor *PineWorkerCommandExecutor) preflightBarCommands(commands []WorkerOrderCommand) ([]pineWorkerCommandPlan, error) {
	plans := make([]pineWorkerCommandPlan, len(commands))
	atomicGroups := make(map[string][]WorkerOrderCommand)
	for index, command := range commands {
		plans[index].command = command
		switch normalizeWorkerIntentKind(command.Kind) {
		case "entry", "order", "exit", "close", "close_all":
			resolved, skip, err := executor.resolvePositionCloseCommand(command)
			if err != nil {
				return nil, err
			}
			plans[index].command = resolved
			plans[index].skip = skip
			if !skip {
				order, orderErr := executor.SubmitOrderFromCommand(resolved)
				if orderErr != nil {
					if ignored, ok := orderErr.(pineWorkerIgnoredOrderError); ok {
						executor.warnIgnoredOrder(resolved, ignored.reason)
						plans[index].skip = true
					} else {
						return nil, orderErr
					}
				} else {
					plans[index].order = order
				}
			}
		case "cancel":
			if strings.TrimSpace(command.ID) == "" {
				return nil, fmt.Errorf("pine worker cancel command id is required")
			}
		case "cancel_all":
		default:
			return nil, fmt.Errorf("unsupported pine worker command kind: %s", command.Kind)
		}
		if groupID := strings.TrimSpace(command.AtomicGroupID); groupID != "" {
			atomicGroups[groupID] = append(atomicGroups[groupID], plans[index].command)
		}
	}
	if len(atomicGroups) > 0 {
		if _, ok := executor.OrderExecutor.(PineWorkerAtomicOrderExecutor); !ok {
			return nil, fmt.Errorf("pine worker atomic order groups require an executor with parent/OCO atomic placement capability")
		}
	}
	for groupID, group := range atomicGroups {
		if err := validatePineWorkerAtomicGroup(groupID, group, plans); err != nil {
			return nil, err
		}
	}
	return plans, nil
}

func validatePineWorkerAtomicGroup(groupID string, group []WorkerOrderCommand, plans []pineWorkerCommandPlan) error {
	if len(group) < 2 {
		return fmt.Errorf("pine worker atomic group %q requires at least two commands", groupID)
	}
	entries := make(map[string]struct{})
	ocoGroups := make(map[string][]WorkerOrderCommand)
	for _, command := range group {
		kind := normalizeWorkerIntentKind(command.Kind)
		switch kind {
		case "entry", "order":
			if command.ReduceOnly {
				return fmt.Errorf("pine worker atomic group %q contains a reduce-only entry", groupID)
			}
			entryID := strings.TrimSpace(command.ID)
			if entryID == "" {
				return fmt.Errorf("pine worker atomic group %q contains an entry without an id", groupID)
			}
			entries[entryID] = struct{}{}
		case "exit", "close", "close_all":
		case "cancel", "cancel_all":
			return fmt.Errorf("pine worker atomic group %q cannot contain cancellation commands", groupID)
		default:
			return fmt.Errorf("pine worker atomic group %q contains unsupported command kind %q", groupID, command.Kind)
		}
		if ocoGroupID := strings.TrimSpace(command.OCOGroupID); ocoGroupID != "" {
			if kind == "entry" || kind == "order" {
				return fmt.Errorf("pine worker atomic group %q entry %q cannot be an OCO child", groupID, command.ID)
			}
			ocoGroups[ocoGroupID] = append(ocoGroups[ocoGroupID], command)
		}
	}
	for _, plan := range plans {
		command := plan.command
		if strings.TrimSpace(command.AtomicGroupID) != groupID {
			continue
		}
		if plan.skip {
			return fmt.Errorf("pine worker atomic group %q contains an order that cannot be placed", groupID)
		}
		kind := normalizeWorkerIntentKind(command.Kind)
		if kind == "exit" || kind == "close" || kind == "close_all" {
			if !command.ReduceOnly {
				return fmt.Errorf("pine worker atomic group %q contains a non-reduce-only protective exit", groupID)
			}
			if len(entries) > 0 {
				if _, ok := entries[strings.TrimSpace(command.ParentID)]; !ok {
					return fmt.Errorf("pine worker atomic group %q exit %q has no matching parent entry", groupID, command.ID)
				}
			}
		}
	}
	for ocoGroupID, commands := range ocoGroups {
		if len(commands) != 2 {
			return fmt.Errorf("pine worker OCO group %q requires exactly two protective legs", ocoGroupID)
		}
		orderTypes := make(map[types.OrderType]struct{}, len(commands))
		for _, plan := range plans {
			if strings.TrimSpace(plan.command.AtomicGroupID) == groupID && strings.TrimSpace(plan.command.OCOGroupID) == ocoGroupID {
				orderTypes[plan.order.Type] = struct{}{}
			}
		}
		if _, ok := orderTypes[types.OrderTypeLimit]; !ok {
			return fmt.Errorf("pine worker OCO group %q requires one limit leg", ocoGroupID)
		}
		if _, ok := orderTypes[types.OrderTypeStopMarket]; !ok {
			return fmt.Errorf("pine worker OCO group %q requires one stop leg", ocoGroupID)
		}
	}
	return nil
}

func (executor *PineWorkerCommandExecutor) executePlanned(ctx context.Context, plan pineWorkerCommandPlan) error {
	if plan.skip {
		return nil
	}
	switch normalizeWorkerIntentKind(plan.command.Kind) {
	case "entry", "order", "exit", "close", "close_all":
		created, err := executor.OrderExecutor.SubmitOrders(ctx, plan.order)
		if err != nil {
			return fmt.Errorf("submit pine worker command %s: %w", plan.command.ID, err)
		}
		executor.trackCreatedOrders(plan.command, created)
		return nil
	case "cancel":
		return executor.cancel(ctx, plan.command.ID)
	case "cancel_all":
		return executor.cancelAll(ctx)
	default:
		return fmt.Errorf("unsupported pine worker command kind: %s", plan.command.Kind)
	}
}

func (executor *PineWorkerCommandExecutor) executeAtomicGroup(ctx context.Context, groupID string, plans []pineWorkerCommandPlan) error {
	atomicExecutor, ok := executor.OrderExecutor.(PineWorkerAtomicOrderExecutor)
	if !ok {
		return fmt.Errorf("pine worker atomic order groups require an executor with parent/OCO atomic placement capability")
	}
	commands := make([]WorkerOrderCommand, 0)
	orders := make([]PineWorkerAtomicOrder, 0)
	for _, plan := range plans {
		if strings.TrimSpace(plan.command.AtomicGroupID) != groupID {
			continue
		}
		commands = append(commands, plan.command)
		orders = append(orders, PineWorkerAtomicOrder{
			CommandID: plan.command.ID, IntentID: plan.command.IntentID,
			ParentID: plan.command.ParentID, OCOGroupID: plan.command.OCOGroupID,
			ReduceOnly: plan.command.ReduceOnly, Order: plan.order,
		})
	}
	created, err := atomicExecutor.SubmitAtomicPineOrders(ctx, groupID, orders...)
	if err != nil {
		return fmt.Errorf("submit pine worker atomic group %s: %w", groupID, err)
	}
	if len(created) != len(commands) {
		return fmt.Errorf("pine worker atomic group %q returned %d orders for %d commands", groupID, len(created), len(commands))
	}
	for index := range commands {
		executor.trackCreatedOrders(commands[index], types.OrderSlice{created[index]})
	}
	return nil
}

func (executor *PineWorkerCommandExecutor) Execute(ctx context.Context, command WorkerOrderCommand) error {
	if strings.TrimSpace(command.AtomicGroupID) != "" {
		return fmt.Errorf("pine worker atomic command %q must be executed with its complete bar command group", command.ID)
	}
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
	if strings.TrimSpace(command.ParentID) != "" && strings.TrimSpace(command.AtomicGroupID) != "" {
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
		ReduceOnly:    command.ReduceOnly,
	}
	if groupID := strings.TrimSpace(command.OCOGroupID); groupID != "" {
		order.GroupID = pineWorkerOrderGroupID(groupID)
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

func pineWorkerOrderGroupID(value string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(value))
	groupID := hash.Sum32()
	if groupID == 0 {
		return 1
	}
	return groupID
}

func (executor *PineWorkerCommandExecutor) orderQuantity(command WorkerOrderCommand, market types.Market) (fixedpoint.Value, error) {
	if command.Quantity > 0 {
		return requireTradablePineWorkerCommandQuantity(command, market, fixedpoint.NewFromFloat(command.Quantity))
	}
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
	key := strings.TrimSpace(id)
	keys := append([]string(nil), executor.activeOrderAliases[key]...)
	if _, ok := executor.activeOrders[key]; ok {
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}
	orders := make(types.OrderSlice, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, orderKey := range keys {
		if _, ok := seen[orderKey]; ok {
			continue
		}
		seen[orderKey] = struct{}{}
		if order, ok := executor.activeOrders[orderKey]; ok {
			orders = append(orders, order)
		}
	}
	if len(orders) == 0 {
		return nil
	}
	if err := executor.OrderExecutor.CancelOrders(ctx, orders...); err != nil {
		return fmt.Errorf("cancel pine worker command %s: %w", id, err)
	}
	for orderKey := range seen {
		delete(executor.activeOrders, orderKey)
	}
	delete(executor.activeOrderAliases, key)
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
	clear(executor.activeOrderAliases)
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
	intentID := strings.TrimSpace(command.IntentID)
	if intentID != "" && intentID != key {
		if executor.activeOrderAliases == nil {
			executor.activeOrderAliases = make(map[string][]string)
		}
		executor.activeOrderAliases[intentID] = append(executor.activeOrderAliases[intentID], key)
	}
}

func (executor *PineWorkerCommandExecutor) clientOrderID(command WorkerOrderCommand) string {
	prefix := strings.TrimSpace(executor.ClientOrderIDPrefix)
	commandID := strings.TrimSpace(command.ID)
	if commandID != "" {
		if prefix == "" {
			return commandID
		}
		// A Pine order ID identifies the logical order across the strategy
		// lifetime, not one broker submission. Include the closed bar when a
		// live-runtime prefix is configured so retries of the same bar remain
		// idempotent without collapsing later bars into the first order.
		return fmt.Sprintf("%s-%s-%d", prefix, commandID, command.BarIndex)
	}
	if prefix == "" {
		prefix = "pine-worker"
	}
	return fmt.Sprintf("%s-%d-%d", prefix, command.BarIndex, time.Now().UnixNano())
}
