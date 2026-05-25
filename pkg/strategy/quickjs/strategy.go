package quickjs

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	bbgo2 "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	"github.com/c9s/bbgo/pkg/types"
	qjs "modernc.org/quickjs"
)

const ID = "quickjs"

func init() {
	bbgo2.RegisterStrategy(ID, &Strategy{})
}

func ValidateScript(script string) error {
	script = strings.TrimSpace(script)
	if script == "" {
		return fmt.Errorf("strategy script is required")
	}
	vm, err := qjs.NewVM()
	if err != nil {
		return fmt.Errorf("create quickjs vm: %w", err)
	}
	defer func() {
		_ = vm.Close()
	}()
	if _, err := vm.Compile(script, qjs.EvalGlobal); err != nil {
		return fmt.Errorf("compile quickjs strategy: %w", err)
	}
	return nil
}

type Strategy struct {
	StrategyID   string         `json:"strategyId"`
	Name         string         `json:"name"`
	Symbol       string         `json:"symbol"`
	Interval     types.Interval `json:"interval"`
	Script       string         `json:"script"`
	DefinitionID string         `json:"definitionId"`
	// WarmupUntil, when set in backtest mode, suppresses placeOrder
	// before this time so that indicators can warm up without
	// executing trades during the warmup period.
	WarmupUntil time.Time `json:"-"`
	// OnError, when set, is called with human-readable error messages
	// from runtime hooks (e.g. onKLineClosed).  This lets the backtest
	// runner collect per-event errors for the frontend.
	OnError func(errMsg string) `json:"-"`
}

func (s *Strategy) ID() string {
	return ID
}

type runtimeBridge struct {
	mu            sync.Mutex
	hookContextMu sync.RWMutex
	vm            *qjs.VM
	indicators    *indicatorRuntime
	strategy      *Strategy
	ctx           context.Context
	session       *bbgo2.ExchangeSession
	executor      bbgo2.OrderExecutor
	orders        map[string]types.Order
	closed        bool
	hookProbes    map[string]bool // cached typeof probe results
	activeHookCtx *HookContext
}

type HookContext struct {
	CurrentKlineTime time.Time
	WarmupUntil      time.Time
}

type hostOrderAck struct {
	Accepted  bool   `json:"accepted"`
	RequestID string `json:"requestId"`
	OrderID   string `json:"orderId,omitempty"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
}

type runtimeRiskSnapshot struct {
	Source             string   `json:"source"`
	AccountAvailable   bool     `json:"accountAvailable"`
	AccountType        string   `json:"accountType"`
	ExecutorAvailable  bool     `json:"executorAvailable"`
	RealTradingEnabled bool     `json:"realTradingEnabled"`
	RiskEnabled        bool     `json:"riskEnabled"`
	KillSwitchActive   bool     `json:"killSwitchActive"`
	BlockedOperations  []string `json:"blockedOperations"`
	AllowsCancel       bool     `json:"allowsCancel"`
	CanTrade           bool     `json:"canTrade"`
	CanDeposit         bool     `json:"canDeposit"`
	CanWithdraw        bool     `json:"canWithdraw"`
}

type positionSnapshot struct {
	Symbol            string  `json:"symbol"`
	Quantity          float64 `json:"quantity"`
	AvailableQuantity float64 `json:"availableQuantity"`
	AverageCost       float64 `json:"averageCost"`
	MarketValue       float64 `json:"marketValue"`
	UnrealizedPnL     float64 `json:"unrealizedPnL"`
	LastPrice         float64 `json:"lastPrice"`
	Direction         string  `json:"direction"`
}

func (s *Strategy) Subscribe(session *bbgo2.ExchangeSession) {
	if strings.TrimSpace(s.Symbol) == "" {
		return
	}
	interval := s.Interval
	if interval == "" {
		interval = types.Interval("1m")
	}
	session.Subscribe(types.KLineChannel, s.Symbol, types.SubscribeOptions{Interval: interval})
}

func (s *Strategy) Run(ctx context.Context, orderExecutor bbgo2.OrderExecutor, session *bbgo2.ExchangeSession) error {
	bridge, err := newRuntimeBridge(ctx, s, orderExecutor, session)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		_ = bridge.close()
	}()

	if err := bridge.invokeHook("onInit", map[string]any{
		"id":           strategyName(s),
		"name":         strategyName(s),
		"definitionId": strings.TrimSpace(s.DefinitionID),
		"symbol":       strings.ToUpper(strings.TrimSpace(s.Symbol)),
		"interval":     string(defaultInterval(s.Interval)),
		"isBacktest":   bbgo2.IsBackTesting,
	}, &HookContext{WarmupUntil: s.WarmupUntil}); err != nil {
		return err
	}

	session.MarketDataStream.OnKLineClosed(func(kline types.KLine) {
		if strings.TrimSpace(s.Symbol) != "" && kline.Symbol != strings.ToUpper(strings.TrimSpace(s.Symbol)) {
			return
		}
		if interval := defaultInterval(s.Interval); interval != "" && kline.Interval != interval {
			return
		}
		bridge.pushIndicators(kline)
		if hookErr := bridge.invokeHook("onKLineClosed", map[string]any{
			"id":           strategyName(s),
			"definitionId": strings.TrimSpace(s.DefinitionID),
			"symbol":       strings.ToUpper(strings.TrimSpace(s.Symbol)),
			"interval":     string(defaultInterval(s.Interval)),
			"kline":        klinePayload(kline),
			"indicators":   bridge.indicatorPayload(),
		}, &HookContext{CurrentKlineTime: kline.EndTime.Time(), WarmupUntil: s.WarmupUntil}); hookErr != nil {
			errMsg := hookErr.Error()
			bbgo2.Notify("quickjs strategy %s onKLineClosed error: %s", strategyName(s), errMsg)
			if s.OnError != nil {
				s.OnError(errMsg)
			}
		}
	})

	return nil
}

func (r *runtimeBridge) pushIndicators(kline types.KLine) {
	if r == nil || r.indicators == nil {
		return
	}
	r.indicators.push(kline)
}

func (r *runtimeBridge) indicatorPayload() map[string]any {
	if r == nil || r.indicators == nil {
		return nil
	}
	return r.indicators.snapshot()
}

func newRuntimeBridge(ctx context.Context, strategy *Strategy, orderExecutor bbgo2.OrderExecutor, session *bbgo2.ExchangeSession) (*runtimeBridge, error) {
	vm, err := qjs.NewVM()
	if err != nil {
		return nil, fmt.Errorf("create quickjs vm: %w", err)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	bridge := &runtimeBridge{
		vm:         vm,
		indicators: newIndicatorRuntime(strategy.Script),
		strategy:   strategy,
		ctx:        ctx,
		session:    session,
		executor:   orderExecutor,
		orders:     make(map[string]types.Order),
		hookProbes: make(map[string]bool),
	}
	if err := bridge.installHostAPI(); err != nil {
		_ = bridge.close()
		return nil, err
	}
	script := strings.TrimSpace(strategy.Script)
	if script == "" {
		script = "function onInit(ctx) { console.log('init strategy', ctx.name); }"
	}
	bytecode, err := vm.Compile(script, qjs.EvalGlobal)
	if err != nil {
		_ = bridge.close()
		return nil, fmt.Errorf("compile quickjs strategy: %w", err)
	}
	if _, err := vm.EvalBytecode(bytecode); err != nil {
		_ = bridge.close()
		return nil, fmt.Errorf("load quickjs strategy: %w", err)
	}
	return bridge, nil
}

func (r *runtimeBridge) installHostAPI() error {
	if err := r.vm.RegisterFunc("__jftradeHostLog", func(message string) {
		bbgo2.Notify("quickjs strategy %s: %s", strategyName(r.strategy), message)
	}, false); err != nil {
		return fmt.Errorf("register quickjs console host: %w", err)
	}
	if err := r.vm.RegisterFunc("__jftradeHostNotify", func(message string) {
		bbgo2.Notify("quickjs strategy %s: %s", strategyName(r.strategy), message)
	}, false); err != nil {
		return fmt.Errorf("register quickjs notify host: %w", err)
	}
	if err := r.vm.RegisterFunc("__jftradeHostPlaceOrderResult", func(request *qjs.Object) (hostOrderAck, any) {
		decodedRequest, decodeErr := decodeHostRequest(request)
		if decodeErr != nil {
			return hostOrderAck{}, decodeErr.Error()
		}
		ack, placeErr := r.placeOrder(decodedRequest)
		if placeErr != nil {
			return hostOrderAck{}, placeErr.Error()
		}
		return ack, nil
	}, false); err != nil {
		return fmt.Errorf("register quickjs placeOrder host: %w", err)
	}
	if err := r.vm.RegisterFunc("__jftradeHostCancelOrderResult", func(orderID string) (int, any) {
		cancelled, cancelErr := r.cancelOrder(orderID)
		if cancelErr != nil {
			return 0, cancelErr.Error()
		}
		if cancelled {
			return 1, nil
		}
		return 0, nil
	}, false); err != nil {
		return fmt.Errorf("register quickjs cancelOrder host: %w", err)
	}
	if err := r.vm.RegisterFunc("__jftradeHostGetPosition", func(symbol string) *positionSnapshot {
		return r.getPosition(symbol)
	}, false); err != nil {
		return fmt.Errorf("register quickjs getPosition host: %w", err)
	}
	if err := r.vm.RegisterFunc("__jftradeHostGetPositions", func() []positionSnapshot {
		return r.getPositions()
	}, false); err != nil {
		return fmt.Errorf("register quickjs getPositions host: %w", err)
	}
	if err := r.vm.RegisterFunc("__jftradeHostGetRiskState", func() runtimeRiskSnapshot {
		return r.getRiskState()
	}, false); err != nil {
		return fmt.Errorf("register quickjs getRiskState host: %w", err)
	}
	if err := r.vm.RegisterFunc("__jftradeHostIsOperationBlocked", func(operation string) int {
		if r.isOperationBlocked(operation) {
			return 1
		}
		return 0
	}, false); err != nil {
		return fmt.Errorf("register quickjs isOperationBlocked host: %w", err)
	}
	if err := r.vm.RegisterFunc("__jftradeHostGetAvailableCash", func() float64 {
		return r.getAvailableCash()
	}, false); err != nil {
		return fmt.Errorf("register quickjs getAvailableCash host: %w", err)
	}
	bootstrap := strings.Join([]string{
		"(() => {",
		"  const formatArg = (value) => {",
		"    if (typeof value === 'string') return value;",
		"    if (value === null) return 'null';",
		"    if (value === undefined) return 'undefined';",
		"    try { return JSON.stringify(value); } catch (_) { return String(value); }",
		"  };",
		"  const joinArgs = (...args) => args.map(formatArg).join(' ');",
		"  const unwrapHostResult = (result) => {",
		"    if (!Array.isArray(result)) return result;",
		"    const [value, err] = result;",
		"    if (err !== null && err !== undefined && err !== '') {",
		"      throw new Error(String(err));",
		"    }",
		"    return value;",
		"  };",
		"  globalThis.console = globalThis.console ?? {};",
		"  globalThis.console.log = (...args) => __jftradeHostLog(joinArgs(...args));",
		"  globalThis.notify = (...args) => __jftradeHostNotify(joinArgs(...args));",
		"  globalThis.placeOrder = (request) => unwrapHostResult(__jftradeHostPlaceOrderResult(request ?? {}));",
		"  globalThis.cancelOrder = (orderId) => Boolean(unwrapHostResult(__jftradeHostCancelOrderResult(String(orderId ?? ''))));",
		"  globalThis.getPosition = (symbol) => __jftradeHostGetPosition(String(symbol ?? ''));",
		"  globalThis.getPositions = () => __jftradeHostGetPositions();",
		"  globalThis.getRiskState = () => __jftradeHostGetRiskState();",
		"  globalThis.isOperationBlocked = (operation) => Boolean(__jftradeHostIsOperationBlocked(String(operation ?? '')));",
		"  globalThis.getAvailableCash = () => __jftradeHostGetAvailableCash();",
		"})();",
	}, "\n")
	if _, err := r.vm.Eval(bootstrap, qjs.EvalGlobal); err != nil {
		return fmt.Errorf("install quickjs host api: %w", err)
	}
	return nil
}

func (r *runtimeBridge) placeOrder(request map[string]any) (hostOrderAck, error) {
	if r.executor == nil {
		return hostOrderAck{}, fmt.Errorf("order executor is not available")
	}
	if r.session == nil {
		return hostOrderAck{}, fmt.Errorf("exchange session is not available")
	}
	if r.isOperationBlocked("PLACE") {
		return hostOrderAck{}, fmt.Errorf("place operation is blocked by current runtime state")
	}

	order, requestID, err := buildSubmitOrderFromHostRequest(request, r.strategy, r.session)
	if err != nil {
		return hostOrderAck{}, err
	}

	createdOrders, err := r.executor.SubmitOrders(r.ctx, order)
	if err != nil {
		return hostOrderAck{}, fmt.Errorf("submit order: %w", err)
	}

	if len(createdOrders) == 0 {
		return hostOrderAck{
			Accepted:  true,
			RequestID: requestID,
			Status:    string(types.OrderStatusNew),
		}, nil
	}

	createdOrder := createdOrders[0]
	r.rememberOrder(createdOrder, requestID)
	return buildOrderAck(createdOrder, requestID), nil
}

func (r *runtimeBridge) cancelOrder(orderID string) (bool, error) {
	if r.executor == nil {
		return false, fmt.Errorf("order executor is not available")
	}
	if r.isOperationBlocked("CANCEL") {
		return false, fmt.Errorf("cancel operation is blocked by current runtime state")
	}

	normalizedID := strings.TrimSpace(orderID)
	if normalizedID == "" {
		return false, fmt.Errorf("orderId is required")
	}

	order, ok := r.lookupOrder(normalizedID)
	if !ok {
		return false, fmt.Errorf("order %s is not known to this strategy runtime", normalizedID)
	}

	if err := r.executor.CancelOrders(r.ctx, order); err != nil {
		return false, fmt.Errorf("cancel order: %w", err)
	}
	return true, nil
}

func (r *runtimeBridge) getPosition(symbol string) *positionSnapshot {
	if r.session == nil {
		return nil
	}

	normalizedSymbol := strings.ToUpper(strings.TrimSpace(symbol))
	if normalizedSymbol == "" {
		normalizedSymbol = strings.ToUpper(strings.TrimSpace(r.strategy.Symbol))
	}
	if normalizedSymbol == "" {
		return nil
	}

	market, ok := r.session.Market(normalizedSymbol)
	if !ok {
		return nil
	}

	lastPrice, _ := r.session.LastPrice(normalizedSymbol)
	return buildPositionSnapshot(normalizedSymbol, market, runtimePositionForSymbol(r.session, normalizedSymbol), lastPrice, runtimeAccount(r.session))
}

func (r *runtimeBridge) getPositions() []positionSnapshot {
	if r.session == nil {
		return nil
	}

	symbolSet := map[string]struct{}{}
	if symbol := strings.ToUpper(strings.TrimSpace(r.strategy.Symbol)); symbol != "" {
		symbolSet[symbol] = struct{}{}
	}
	for symbol := range r.session.Positions() {
		if normalized := strings.ToUpper(strings.TrimSpace(symbol)); normalized != "" {
			symbolSet[normalized] = struct{}{}
		}
	}

	positions := make([]positionSnapshot, 0, len(symbolSet))
	for symbol := range symbolSet {
		market, ok := r.session.Market(symbol)
		if !ok {
			continue
		}
		lastPrice, _ := r.session.LastPrice(symbol)
		snapshot := buildPositionSnapshot(symbol, market, runtimePositionForSymbol(r.session, symbol), lastPrice, runtimeAccount(r.session))
		if snapshot != nil {
			positions = append(positions, *snapshot)
		}
	}
	return positions
}

func (r *runtimeBridge) getRiskState() runtimeRiskSnapshot {
	publicOnly := false
	if r.session != nil {
		publicOnly = r.session.PublicOnly
	}
	return buildRuntimeRiskSnapshot(publicOnly, runtimeAccount(r.session), r.executor != nil)
}

func (r *runtimeBridge) setActiveHookContext(ctx *HookContext) {
	r.hookContextMu.Lock()
	defer r.hookContextMu.Unlock()
	r.activeHookCtx = ctx
}

func (r *runtimeBridge) currentHookContext() *HookContext {
	r.hookContextMu.RLock()
	defer r.hookContextMu.RUnlock()
	if r.activeHookCtx == nil {
		return nil
	}
	ctx := *r.activeHookCtx
	return &ctx
}

// isOperationBlocked checks whether the given operation is blocked.
// In backtest mode, PLACE is also blocked during the warmup period.
func (r *runtimeBridge) isOperationBlocked(operation string) bool {
	snapshot := r.getRiskState()
	normalizedOperation := strings.ToUpper(strings.TrimSpace(operation))
	if normalizedOperation == "" {
		return true
	}

	// During warmup, suppress order placement so indicators can
	// accumulate history before actual trading begins.
	if normalizedOperation == "PLACE" {
		hookContext := r.currentHookContext()
		warmupUntil := time.Time{}
		currentKlineTime := time.Time{}
		if r.strategy != nil {
			warmupUntil = r.strategy.WarmupUntil
		}
		if hookContext != nil {
			currentKlineTime = hookContext.CurrentKlineTime
			if !hookContext.WarmupUntil.IsZero() {
				warmupUntil = hookContext.WarmupUntil
			}
		}
		if !warmupUntil.IsZero() && currentKlineTime.Before(warmupUntil) {
			return true
		}
	}

	for _, blockedOperation := range snapshot.BlockedOperations {
		if blockedOperation == normalizedOperation {
			return true
		}
	}
	if normalizedOperation == "CANCEL" {
		return !snapshot.AllowsCancel
	}
	return false
}

// getAvailableCash returns the available cash for order sizing.
// In backtest mode this reflects the strategy's current portfolio cash;
// in live mode it returns the actual account available funds.
func (r *runtimeBridge) getAvailableCash() float64 {
	account := runtimeAccount(r.session)
	if account == nil {
		return 0
	}

	// Prefer TotalAccountValue (normalized to a single currency).
	if !account.TotalAccountValue.IsZero() {
		return account.TotalAccountValue.Float64()
	}

	// Fallback: sum available balances across all currencies.
	total := fixedpoint.Zero
	for _, bal := range account.Balances() {
		total = total.Add(bal.Available)
	}
	if !total.IsZero() {
		return total.Float64()
	}

	// Last resort: sum net assets.
	for _, bal := range account.Balances() {
		total = total.Add(bal.NetAsset)
	}
	return total.Float64()
}

func (r *runtimeBridge) rememberOrder(order types.Order, requestID string) {
	for _, key := range orderLookupKeys(order, requestID) {
		r.orders[key] = order
	}
}

func (r *runtimeBridge) lookupOrder(orderID string) (types.Order, bool) {
	order, ok := r.orders[strings.TrimSpace(orderID)]
	return order, ok
}

func runtimeAccount(session *bbgo2.ExchangeSession) *types.Account {
	if session == nil {
		return nil
	}
	return session.GetAccount()
}

func runtimePositionForSymbol(session *bbgo2.ExchangeSession, symbol string) *types.Position {
	if session == nil {
		return nil
	}
	positions := session.Positions()
	if positions == nil {
		return nil
	}
	return positions[symbol]
}

func buildSubmitOrderFromHostRequest(request map[string]any, strategy *Strategy, session *bbgo2.ExchangeSession) (types.SubmitOrder, string, error) {
	if session == nil {
		return types.SubmitOrder{}, "", fmt.Errorf("exchange session is not available")
	}

	requestID := strings.TrimSpace(readHostString(request, "clientOrderId"))
	if requestID == "" {
		requestID = fmt.Sprintf("quickjs-%d", time.Now().UnixNano())
	}

	symbol := strings.ToUpper(strings.TrimSpace(readHostString(request, "symbol")))
	if symbol == "" && strategy != nil {
		symbol = strings.ToUpper(strings.TrimSpace(strategy.Symbol))
	}
	if symbol == "" {
		return types.SubmitOrder{}, "", fmt.Errorf("symbol is required")
	}

	market, ok := session.Market(symbol)
	if !ok {
		return types.SubmitOrder{}, "", fmt.Errorf("market %s is not loaded in this session", symbol)
	}

	side, err := parseHostSideType(readHostString(request, "side"))
	if err != nil {
		return types.SubmitOrder{}, "", err
	}

	quantity, err := readHostFixedpoint(request, "quantity")
	if err != nil {
		return types.SubmitOrder{}, "", err
	}
	if quantity.Sign() <= 0 {
		return types.SubmitOrder{}, "", fmt.Errorf("quantity must be positive")
	}

	orderType, err := parseHostOrderType(readHostString(request, "orderType"))
	if err != nil {
		return types.SubmitOrder{}, "", err
	}

	order := types.SubmitOrder{
		ClientOrderID: requestID,
		Symbol:        symbol,
		Side:          side,
		Type:          orderType,
		Quantity:      quantity,
		Market:        market,
		Tag:           strings.TrimSpace(readHostString(request, "note")),
	}

	if stopPrice, stopPriceErr := readOptionalHostFixedpoint(request, "stopPrice"); stopPriceErr != nil {
		return types.SubmitOrder{}, "", stopPriceErr
	} else if stopPrice != nil {
		order.StopPrice = *stopPrice
	}

	if reduceOnly, ok := readHostBool(request, "reduceOnly"); ok {
		order.ReduceOnly = reduceOnly
	}
	if closePosition, ok := readHostBool(request, "closePosition"); ok {
		order.ClosePosition = closePosition
	}

	if orderType != types.OrderTypeMarket {
		price, priceErr := readOptionalHostFixedpoint(request, "limitPrice")
		if priceErr != nil {
			return types.SubmitOrder{}, "", priceErr
		}
		if price == nil || price.Sign() <= 0 {
			return types.SubmitOrder{}, "", fmt.Errorf("limitPrice is required for %s orders", orderType)
		}
		order.Price = *price

		timeInForce, tifErr := parseHostTimeInForce(readHostString(request, "timeInForce"))
		if tifErr != nil {
			return types.SubmitOrder{}, "", tifErr
		}
		order.TimeInForce = timeInForce
	}

	return order, requestID, nil
}

func buildOrderAck(order types.Order, requestID string) hostOrderAck {
	orderID := strings.TrimSpace(order.UUID)
	if orderID == "" && order.OrderID > 0 {
		orderID = fmt.Sprintf("%d", order.OrderID)
	}
	if orderID == "" {
		orderID = requestID
	}

	status := string(order.Status)
	if status == "" {
		status = string(types.OrderStatusNew)
	}

	return hostOrderAck{
		Accepted:  true,
		RequestID: requestID,
		OrderID:   orderID,
		Status:    status,
		Message:   fmt.Sprintf("submitted %s %s %s", order.Symbol, order.Side, order.Quantity.String()),
	}
}

func orderLookupKeys(order types.Order, requestID string) []string {
	keys := []string{}
	appendKey := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range keys {
			if existing == value {
				return
			}
		}
		keys = append(keys, value)
	}

	appendKey(requestID)
	appendKey(order.ClientOrderID)
	appendKey(order.UUID)
	if order.OrderID > 0 {
		appendKey(fmt.Sprintf("%d", order.OrderID))
	}
	return keys
}

func buildRuntimeRiskSnapshot(publicOnly bool, account *types.Account, executorAvailable bool) runtimeRiskSnapshot {
	blockedOperations := make([]string, 0, 3)
	addBlockedOperation := func(operation string) {
		for _, existing := range blockedOperations {
			if existing == operation {
				return
			}
		}
		blockedOperations = append(blockedOperations, operation)
	}

	accountAvailable := account != nil
	canTrade := account != nil && account.CanTrade
	canDeposit := account != nil && account.CanDeposit
	canWithdraw := account != nil && account.CanWithdraw
	accountType := ""
	if account != nil {
		accountType = string(account.AccountType)
	}

	realTradingEnabled := accountAvailable && canTrade && executorAvailable && !publicOnly
	allowsCancel := executorAvailable && !publicOnly

	if publicOnly || !executorAvailable || !accountAvailable || !canTrade {
		addBlockedOperation("PLACE")
		addBlockedOperation("MODIFY")
	}
	if !allowsCancel {
		addBlockedOperation("CANCEL")
	}

	return runtimeRiskSnapshot{
		Source:             "quickjs-runtime",
		AccountAvailable:   accountAvailable,
		AccountType:        accountType,
		ExecutorAvailable:  executorAvailable,
		RealTradingEnabled: realTradingEnabled,
		RiskEnabled:        false,
		KillSwitchActive:   false,
		BlockedOperations:  blockedOperations,
		AllowsCancel:       allowsCancel,
		CanTrade:           canTrade,
		CanDeposit:         canDeposit,
		CanWithdraw:        canWithdraw,
	}
}

func buildPositionSnapshot(symbol string, market types.Market, position *types.Position, lastPrice fixedpoint.Value, account *types.Account) *positionSnapshot {
	baseQuantity := fixedpoint.Zero
	averageCost := fixedpoint.Zero
	if position != nil {
		baseQuantity = position.Base
		averageCost = position.AverageCost
	}

	availableQuantity := fixedpoint.Zero
	if account != nil && market.BaseCurrency != "" {
		if balance, ok := account.Balance(market.BaseCurrency); ok {
			availableQuantity = balance.Available
			if baseQuantity.IsZero() {
				baseQuantity = balance.Total()
			}
		}
	}

	direction := "FLAT"
	if baseQuantity.Sign() > 0 {
		direction = "LONG"
	} else if baseQuantity.Sign() < 0 {
		direction = "SHORT"
	}

	marketPrice := lastPrice
	if marketPrice.IsZero() {
		marketPrice = averageCost
	}

	marketValue := marketPrice.Mul(baseQuantity)
	unrealizedPnL := fixedpoint.Zero
	if !marketPrice.IsZero() && !averageCost.IsZero() {
		unrealizedPnL = marketPrice.Sub(averageCost).Mul(baseQuantity)
	}

	return &positionSnapshot{
		Symbol:            symbol,
		Quantity:          baseQuantity.Float64(),
		AvailableQuantity: availableQuantity.Float64(),
		AverageCost:       averageCost.Float64(),
		MarketValue:       marketValue.Float64(),
		UnrealizedPnL:     unrealizedPnL.Float64(),
		LastPrice:         marketPrice.Float64(),
		Direction:         direction,
	}
}

func (r *runtimeBridge) invokeHook(name string, payload map[string]any, hookCtx *HookContext) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.vm == nil {
		return nil
	}
	// Probe once and cache: evaluating typeof on every kline is wasteful and
	// risks QuickJS internal state issues under high-frequency calls.
	if _, probed := r.hookProbes[name]; !probed {
		hasHook, err := r.vm.Eval(fmt.Sprintf("typeof globalThis[%q] === 'function'", name), qjs.EvalGlobal)
		if err != nil {
			r.hookProbes[name] = false
			return fmt.Errorf("probe %s: %w", name, err)
		}
		available, ok := hasHook.(bool)
		r.hookProbes[name] = ok && available
	}
	if !r.hookProbes[name] {
		return nil
	}
	r.setActiveHookContext(hookCtx)
	defer r.setActiveHookContext(nil)
	if _, err := r.vm.Call(name, payload); err != nil {
		return fmt.Errorf("invoke %s: %w", name, err)
	}
	return nil
}

func (r *runtimeBridge) close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.vm == nil {
		return nil
	}
	r.closed = true
	err := r.vm.Close()
	r.vm = nil
	return err
}

func defaultInterval(interval types.Interval) types.Interval {
	if interval == "" {
		return types.Interval("1m")
	}
	return interval
}

func strategyName(strategy *Strategy) string {
	if strategy == nil {
		return ID
	}
	if name := strings.TrimSpace(strategy.Name); name != "" {
		return name
	}
	if name := strings.TrimSpace(strategy.StrategyID); name != "" {
		return name
	}
	if name := strings.TrimSpace(strategy.DefinitionID); name != "" {
		return name
	}
	return ID
}

func klinePayload(kline types.KLine) map[string]any {
	return map[string]any{
		"symbol":      kline.Symbol,
		"interval":    string(kline.Interval),
		"startTime":   kline.StartTime.Time().Format("2006-01-02T15:04:05.000Z07:00"),
		"endTime":     kline.EndTime.Time().Format("2006-01-02T15:04:05.000Z07:00"),
		"open":        kline.Open.Float64(),
		"high":        kline.High.Float64(),
		"low":         kline.Low.Float64(),
		"close":       kline.Close.Float64(),
		"volume":      kline.Volume.Float64(),
		"quoteVolume": kline.QuoteVolume.Float64(),
		"closed":      kline.Closed,
	}
}
