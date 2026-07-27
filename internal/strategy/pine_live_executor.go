package strategy

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

// LiveOrderExecutor is the broker-neutral placement surface used by live
// strategy execution.
type LiveOrderExecutor interface {
	SubmitOrders(context.Context, ...types.SubmitOrder) (types.OrderSlice, error)
	CancelOrders(context.Context, ...types.Order) error
}

// LiveAtomicOrder preserves the Pine parent/OCO semantics alongside the
// broker-neutral order. Implementations must accept every leg or no leg, keep
// children inactive until their parent fills, cancel OCO siblings after the
// first fill, and enforce ReduceOnly at match time.
type LiveAtomicOrder struct {
	CommandID  string
	IntentID   string
	ParentID   string
	OCOGroupID string
	ReduceOnly bool
	Order      types.SubmitOrder
}

// LiveAtomicOrderExecutor preserves parent and OCO placement atomically.
type LiveAtomicOrderExecutor interface {
	SubmitAtomicPineOrders(context.Context, string, ...LiveAtomicOrder) (types.OrderSlice, error)
}

// LiveMarketResolver provides the active market rules for order normalization.
type LiveMarketResolver interface {
	Market(symbol string) (types.Market, bool)
}

// LiveCommandExecutor applies one closed bar of Pine commands to the live order
// surface while retaining cancellation tracking and atomic-group semantics.
type LiveCommandExecutor struct {
	Symbol                         string
	OrderExecutor                  LiveOrderExecutor
	MarketResolver                 LiveMarketResolver
	PositionSizer                  LiveCommandPositionSizer
	WarningSink                    LiveIgnoredOrderWarningSink
	ClientOrderIDPrefix            string
	RejectOrdersWithoutMarketRules bool

	activeOrders       map[string]types.Order
	activeOrderAliases map[string][]string
}

// LiveCommandExecutorOptions contains the live strategy dependencies needed to
// turn broker-neutral commands into normalized orders.
type LiveCommandExecutorOptions struct {
	Symbol                         string
	OrderExecutor                  LiveOrderExecutor
	MarketResolver                 LiveMarketResolver
	PositionSizer                  LiveCommandPositionSizer
	WarningSink                    LiveIgnoredOrderWarningSink
	ClientOrderIDPrefix            string
	RejectOrdersWithoutMarketRules bool
}

// NewLiveCommandExecutor builds a live strategy command executor.
func NewLiveCommandExecutor(options LiveCommandExecutorOptions) *LiveCommandExecutor {
	return &LiveCommandExecutor{
		Symbol:                         options.Symbol,
		OrderExecutor:                  options.OrderExecutor,
		MarketResolver:                 options.MarketResolver,
		PositionSizer:                  options.PositionSizer,
		WarningSink:                    options.WarningSink,
		ClientOrderIDPrefix:            options.ClientOrderIDPrefix,
		RejectOrdersWithoutMarketRules: options.RejectOrdersWithoutMarketRules,
	}
}

// LiveCommandPositionSizer resolves percentage quantities for live commands.
type LiveCommandPositionSizer interface {
	QuantityForCommand(command WorkerOrderCommand, market types.Market) (fixedpoint.Value, error)
}

type liveCommandPositionReader interface {
	NetPosition() fixedpoint.Value
}

// LiveIgnoredOrderWarningSink receives commands ignored by market rules.
type LiveIgnoredOrderWarningSink interface {
	AddIgnoredOrderWarning(string)
}

type liveIgnoredOrderGroupWarningSink interface {
	AddIgnoredOrderWarningGroup(string, string)
}

type liveIgnoredOrderError struct {
	reason string
}

func (err liveIgnoredOrderError) Error() string {
	return err.reason
}

// ExecuteBarCommands preflights every command before applying a closed bar, so
// malformed atomic groups cannot partially reach the broker.
func (executor *LiveCommandExecutor) ExecuteBarCommands(ctx context.Context, commands []WorkerOrderCommand) error {
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

type liveCommandPlan struct {
	command WorkerOrderCommand
	order   types.SubmitOrder
	skip    bool
}

func (executor *LiveCommandExecutor) preflightBarCommands(commands []WorkerOrderCommand) ([]liveCommandPlan, error) {
	plans := make([]liveCommandPlan, len(commands))
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
					if ignored, ok := orderErr.(liveIgnoredOrderError); ok {
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
		if _, ok := executor.OrderExecutor.(LiveAtomicOrderExecutor); !ok {
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

func validatePineWorkerAtomicGroup(groupID string, group []WorkerOrderCommand, plans []liveCommandPlan) error {
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

func (executor *LiveCommandExecutor) executePlanned(ctx context.Context, plan liveCommandPlan) error {
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

func (executor *LiveCommandExecutor) executeAtomicGroup(ctx context.Context, groupID string, plans []liveCommandPlan) error {
	atomicExecutor, ok := executor.OrderExecutor.(LiveAtomicOrderExecutor)
	if !ok {
		return fmt.Errorf("pine worker atomic order groups require an executor with parent/OCO atomic placement capability")
	}
	commands := make([]WorkerOrderCommand, 0)
	orders := make([]LiveAtomicOrder, 0)
	for _, plan := range plans {
		if strings.TrimSpace(plan.command.AtomicGroupID) != groupID {
			continue
		}
		commands = append(commands, plan.command)
		orders = append(orders, LiveAtomicOrder{
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

// Execute applies one command that is not part of an atomic group.
func (executor *LiveCommandExecutor) Execute(ctx context.Context, command WorkerOrderCommand) error {
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

func (executor *LiveCommandExecutor) submit(ctx context.Context, command WorkerOrderCommand) error {
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
		if ignored, ok := err.(liveIgnoredOrderError); ok {
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

func (executor *LiveCommandExecutor) resolvePositionCloseCommand(command WorkerOrderCommand) (WorkerOrderCommand, bool, error) {
	if !isPineWorkerPositionCloseCommand(command) {
		return command, false, nil
	}
	if strings.TrimSpace(command.ParentID) != "" && strings.TrimSpace(command.AtomicGroupID) != "" {
		return command, false, nil
	}
	reader, ok := executor.PositionSizer.(liveCommandPositionReader)
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

func (executor *LiveCommandExecutor) warnIgnoredOrder(command WorkerOrderCommand, reason string) {
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
	if grouped, ok := executor.WarningSink.(liveIgnoredOrderGroupWarningSink); ok {
		grouped.AddIgnoredOrderWarningGroup(strings.Join([]string{symbol, kind, id, reason}, "|"), message)
		return
	}
	executor.WarningSink.AddIgnoredOrderWarning(message)
}

// SubmitOrderFromCommand resolves quantity and market rules into one
// broker-neutral order without submitting it.
func (executor *LiveCommandExecutor) SubmitOrderFromCommand(command WorkerOrderCommand) (types.SubmitOrder, error) {
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
		return types.SubmitOrder{}, liveIgnoredOrderError{reason: "market quantity rules are unavailable"}
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
	if isPineWorkerShortCommand(command) {
		order.Tag = pineWorkerShortOrderTag
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

func (executor *LiveCommandExecutor) orderQuantity(command WorkerOrderCommand, market types.Market) (fixedpoint.Value, error) {
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
		return fixedpoint.Zero, liveIgnoredOrderError{reason: "quantity is below the market quantity step"}
	}
	if market.MinQuantity.Sign() > 0 && normalized.Compare(market.MinQuantity) < 0 {
		return fixedpoint.Zero, liveIgnoredOrderError{
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

// Keep the historical tag value stable because downstream order and result
// handling may already persist or inspect it.
const pineWorkerShortOrderTag = "pine-worker-short-replay"

func isPineWorkerShortCommand(command WorkerOrderCommand) bool {
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

func (executor *LiveCommandExecutor) cancel(ctx context.Context, id string) error {
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

func (executor *LiveCommandExecutor) cancelAll(ctx context.Context) error {
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

func (executor *LiveCommandExecutor) trackCreatedOrders(command WorkerOrderCommand, createdOrders types.OrderSlice) {
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

func (executor *LiveCommandExecutor) clientOrderID(command WorkerOrderCommand) string {
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
