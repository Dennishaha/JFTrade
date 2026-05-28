package jftradeapi

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	bbgo "github.com/c9s/bbgo/pkg/bbgo"
	"github.com/c9s/bbgo/pkg/fixedpoint"
	bbgotypes "github.com/c9s/bbgo/pkg/types"

	"github.com/jftrade/jftrade-main/pkg/futu"
	"github.com/jftrade/jftrade-main/pkg/strategy/dslruntime"
	strategyindicatorruntime "github.com/jftrade/jftrade-main/pkg/strategy/indicatorruntime"
)

type strategyRuntimeExchange interface {
	bbgotypes.Exchange
	QueryBrokerFunds(ctx context.Context, query futu.BrokerReadQuery) (*futu.BrokerFundsSnapshot, error)
	QueryBrokerPositions(ctx context.Context, query futu.BrokerReadQuery) ([]futu.BrokerPositionSnapshot, error)
	PlaceBrokerOrder(ctx context.Context, query futu.BrokerPlaceOrderQuery, submitOrder bbgotypes.SubmitOrder) (*futu.BrokerPlaceOrderResult, error)
}

type strategyRuntimeMarketEnsurer interface {
	EnsureMarket(symbol string)
}

type strategyRuntimeManager struct {
	server           *Server
	exchangeProvider func() strategyRuntimeExchange

	mu       sync.RWMutex
	runtimes map[string]*managedStrategyRuntime
}

type managedStrategyRuntime struct {
	instanceID        string
	definition        strategyDefinitionSummary
	cancel            context.CancelFunc
	symbols           map[string]*strategySymbolRuntime
	mu                sync.RWMutex
	lastClosedKLineAt time.Time
	lastSignalAt      time.Time
	lastOrderAt       time.Time
	lastErrorAt       time.Time
	lastError         string
	updatedAt         time.Time
}

type strategySymbolRuntime struct {
	instanceID    string
	name          string
	symbol        string
	interval      bbgotypes.Interval
	exchange      bbgotypes.ExchangeName
	emitter       bbgotypes.StandardStreamEmitter
	onClosedKLine func(time.Time)

	mu              sync.RWMutex
	currentBucket   *bbgotypes.KLine
	lastClosedPrice float64
}

type strategyNotifyOnlyOrderExecutor struct {
	manager  *strategyRuntimeManager
	server   *Server
	instance managedStrategyInstance
	runner   *strategySymbolRuntime
}

type strategyLiveOrderExecutor struct {
	manager  *strategyRuntimeManager
	server   *Server
	instance managedStrategyInstance
	runner   *strategySymbolRuntime
}

func newStrategyRuntimeManager(server *Server) *strategyRuntimeManager {
	manager := &strategyRuntimeManager{
		server:   server,
		runtimes: map[string]*managedStrategyRuntime{},
	}
	manager.exchangeProvider = func() strategyRuntimeExchange {
		return server.futuExchange()
	}
	return manager
}

func (m *strategyRuntimeManager) activeInstrumentIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	seen := map[string]struct{}{}
	result := make([]string, 0)
	for _, runtime := range m.runtimes {
		for symbol := range runtime.symbols {
			if _, exists := seen[symbol]; exists {
				continue
			}
			seen[symbol] = struct{}{}
			result = append(result, symbol)
		}
	}
	sort.Strings(result)
	return result
}

func (m *strategyRuntimeManager) startStrategy(ctx context.Context, instance managedStrategyInstance) error {
	interval := bbgotypes.Interval(strings.TrimSpace(instance.Binding.Interval))
	if interval.Duration() <= 0 {
		return fmt.Errorf("strategy interval %q is invalid", instance.Binding.Interval)
	}
	if len(instance.Binding.Symbols) == 0 {
		return fmt.Errorf("strategy instance requires at least one symbol binding")
	}
	script, ok := instance.Params["script"].(string)
	if !ok || strings.TrimSpace(script) == "" {
		return fmt.Errorf("strategy instance is missing script")
	}

	m.mu.Lock()
	if _, exists := m.runtimes[instance.ID]; exists {
		m.mu.Unlock()
		return fmt.Errorf("strategy instance is already running")
	}
	m.mu.Unlock()

	exchange := m.exchangeProvider()
	if exchange == nil {
		return fmt.Errorf("strategy runtime exchange is unavailable")
	}
	if marketEnsurer, ok := exchange.(strategyRuntimeMarketEnsurer); ok {
		for _, symbol := range instance.Binding.Symbols {
			marketEnsurer.EnsureMarket(symbol)
		}
	}

	markets, err := exchange.QueryMarkets(ctx)
	if err != nil {
		return fmt.Errorf("load strategy markets: %w", err)
	}
	brokerQuery := strategyRuntimeBrokerReadQuery(instance.Binding)
	funds, err := exchange.QueryBrokerFunds(ctx, brokerQuery)
	if err != nil {
		return fmt.Errorf("load strategy broker funds: %w", err)
	}
	positions, err := exchange.QueryBrokerPositions(ctx, brokerQuery)
	if err != nil {
		return fmt.Errorf("load strategy broker positions: %w", err)
	}

	runtimeCtx, cancel := context.WithCancel(context.Background())
	managed := &managedStrategyRuntime{
		instanceID: instance.ID,
		definition: instance.Definition,
		cancel:     cancel,
		symbols:    make(map[string]*strategySymbolRuntime, len(instance.Binding.Symbols)),
		updatedAt:  time.Now().UTC(),
	}

	for _, symbol := range instance.Binding.Symbols {
		runner, runnerErr := m.buildSymbolRuntime(ctx, runtimeCtx, exchange, markets, funds, positions, instance, script, symbol, interval)
		if runnerErr != nil {
			cancel()
			return runnerErr
		}
		managed.symbols[symbol] = runner
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.runtimes[instance.ID]; exists {
		cancel()
		return fmt.Errorf("strategy instance is already running")
	}
	m.runtimes[instance.ID] = managed
	return nil
}

func (m *strategyRuntimeManager) stopStrategy(instanceID string) {
	m.mu.Lock()
	runtime, exists := m.runtimes[instanceID]
	if exists {
		delete(m.runtimes, instanceID)
	}
	m.mu.Unlock()
	if exists && runtime.cancel != nil {
		runtime.cancel()
	}
}

func (m *strategyRuntimeManager) handleMarketTrade(trade bbgotypes.Trade) {
	symbol := strings.ToUpper(strings.TrimSpace(trade.Symbol))
	if symbol == "" {
		return
	}

	m.mu.RLock()
	runners := make([]*strategySymbolRuntime, 0, len(m.runtimes))
	for _, runtime := range m.runtimes {
		runner, exists := runtime.symbols[symbol]
		if !exists {
			continue
		}
		runners = append(runners, runner)
	}
	m.mu.RUnlock()

	for _, runner := range runners {
		func(runner *strategySymbolRuntime) {
			defer func() {
				if recovered := recover(); recovered != nil {
					m.handleRuntimePanic(runner.instanceID, runner.symbol, recovered)
				}
			}()
			runner.handleTrade(trade)
		}(runner)
	}
}

func (m *strategyRuntimeManager) buildSymbolRuntime(
	ctx context.Context,
	runtimeCtx context.Context,
	exchange strategyRuntimeExchange,
	markets bbgotypes.MarketMap,
	funds *futu.BrokerFundsSnapshot,
	positions []futu.BrokerPositionSnapshot,
	instance managedStrategyInstance,
	script string,
	symbol string,
	interval bbgotypes.Interval,
) (*strategySymbolRuntime, error) {
	market, ok := markets[symbol]
	if !ok {
		return nil, fmt.Errorf("market metadata for %s is unavailable", symbol)
	}

	session := bbgo.NewExchangeSession("strategy-runtime", exchange)
	session.SetMarkets(markets)
	session.Account = buildStrategyRuntimeAccount(funds, positions, market, symbol)

	emitter, ok := session.MarketDataStream.(bbgotypes.StandardStreamEmitter)
	if !ok {
		return nil, fmt.Errorf("strategy market stream does not support kline emission")
	}

	runner := &strategySymbolRuntime{
		instanceID: instance.ID,
		name:       strings.TrimSpace(instance.Definition.Name),
		symbol:     symbol,
		interval:   interval,
		exchange:   exchange.Name(),
		emitter:    emitter,
		onClosedKLine: func(at time.Time) {
			m.recordClosedKLine(instance.ID, at)
		},
	}

	strategy := &dslruntime.Strategy{
		StrategyID:   strings.TrimSpace(instance.Definition.StrategyID),
		Name:         strings.TrimSpace(instance.Definition.Name),
		Symbol:       symbol,
		Interval:     interval,
		Script:       script,
		DefinitionID: strategyRuntimeDefinitionID(instance),
		OnError: func(message string) {
			_ = m.server.strategyStore.appendStrategyRuntimeEvent(
				instance.ID,
				fmt.Sprintf("runtime error %s: %s", symbol, strings.TrimSpace(message)),
				"runtime_error",
				fmt.Sprintf("%s: %s", symbol, strings.TrimSpace(message)),
			)
		},
	}
	strategy.Subscribe(session)
	if err := strategy.Run(runtimeCtx, m.newOrderExecutor(instance, runner), session); err != nil {
		return nil, fmt.Errorf("start strategy runtime for %s: %w", symbol, err)
	}

	if err := m.seedSymbolRuntime(ctx, exchange, strategy, runner); err != nil {
		return nil, err
	}
	return runner, nil
}

func (m *strategyRuntimeManager) seedSymbolRuntime(ctx context.Context, exchange strategyRuntimeExchange, strategy *dslruntime.Strategy, runner *strategySymbolRuntime) error {
	warmupBars, err := strategyindicatorruntime.WarmupBarsFromScript(strategy.Script, strategy.Interval)
	if err != nil {
		return fmt.Errorf("analyze strategy warmup for %s: %w", runner.symbol, err)
	}
	queryLimit := strategyRuntimeMaxInt(warmupBars+2, 2)
	klines, err := exchange.QueryKLines(ctx, runner.symbol, runner.interval, bbgotypes.KLineQueryOptions{Limit: queryLimit})
	if err != nil {
		return fmt.Errorf("load warmup klines for %s: %w", runner.symbol, err)
	}
	strategy.WarmupUntil = strategyRuntimeWarmupUntil(klines, runner.interval)
	for index := range klines {
		kline := klines[index]
		if !kline.Closed && index == len(klines)-1 {
			current := kline
			runner.setCurrentBucket(&current)
			continue
		}
		closed := kline
		closed.Closed = true
		runner.setLastClosedPrice(closed.Close.Float64())
		runner.emitter.EmitKLineClosed(closed)
	}
	return nil
}

func (m *strategyRuntimeManager) newOrderExecutor(instance managedStrategyInstance, runner *strategySymbolRuntime) bbgo.OrderExecutor {
	if instance.Binding.ExecutionMode == strategyExecutionModeNotifyOnly {
		return &strategyNotifyOnlyOrderExecutor{manager: m, server: m.server, instance: instance, runner: runner}
	}
	return &strategyLiveOrderExecutor{manager: m, server: m.server, instance: instance, runner: runner}
}

func (r *strategySymbolRuntime) handleTrade(trade bbgotypes.Trade) {
	tradeTime := trade.Time.Time().UTC()
	if tradeTime.IsZero() {
		tradeTime = time.Now().UTC()
	}
	windowStart, windowEnd := strategyRuntimeBucketWindow(tradeTime, r.interval)
	if trade.Price.Sign() <= 0 {
		return
	}

	tradeKLine := strategyRuntimeTradeKLine(r.exchange, r.symbol, r.interval, trade, windowStart, windowEnd)
	var closed *bbgotypes.KLine

	r.mu.Lock()
	current := r.currentBucket
	switch {
	case current == nil:
		r.currentBucket = &tradeKLine
	case current.StartTime.Time().Equal(windowStart):
		current.Merge(&tradeKLine)
	case windowStart.After(current.StartTime.Time()):
		closedCopy := *current
		closedCopy.Closed = true
		closed = &closedCopy
		r.lastClosedPrice = closedCopy.Close.Float64()
		r.currentBucket = &tradeKLine
	default:
		current.Merge(&tradeKLine)
	}
	r.mu.Unlock()

	if closed != nil {
		if r.onClosedKLine != nil {
			r.onClosedKLine(closed.EndTime.Time().UTC())
		}
		r.emitter.EmitKLineClosed(*closed)
	}
}

func (r *strategySymbolRuntime) currentPrice() float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.currentBucket != nil {
		return r.currentBucket.Close.Float64()
	}
	return r.lastClosedPrice
}

func (r *strategySymbolRuntime) setCurrentBucket(bucket *bbgotypes.KLine) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.currentBucket = bucket
	if bucket != nil {
		r.lastClosedPrice = bucket.Close.Float64()
	}
}

func (r *strategySymbolRuntime) setLastClosedPrice(price float64) {
	r.mu.Lock()
	r.lastClosedPrice = price
	r.mu.Unlock()
}

func (e *strategyNotifyOnlyOrderExecutor) SubmitOrders(_ context.Context, orders ...bbgotypes.SubmitOrder) (bbgotypes.OrderSlice, error) {
	for _, order := range orders {
		e.manager.recordSignal(e.instance.ID, time.Now().UTC())
		message := e.describeOrderSignal(order)
		e.server.recordLiveNotification(liveNotification{
			At:       time.Now().UTC().Format(time.RFC3339Nano),
			Level:    "info",
			Title:    "策略下单信号",
			Message:  message,
			Source:   "strategy.runtime",
			BrokerID: strategyRuntimeBrokerID(e.instance.Binding),
			Category: "strategy.order.signal",
		})
		_ = e.server.strategyStore.appendStrategyRuntimeEvent(
			e.instance.ID,
			fmt.Sprintf("notify-only signal %s %s %s", order.Symbol, strings.ToUpper(string(order.Side)), strategyRuntimeFormatNumber(order.Quantity.Float64())),
			"signal_notified",
			message,
		)
	}
	return nil, nil
}

func (e *strategyNotifyOnlyOrderExecutor) CancelOrders(context.Context, ...bbgotypes.Order) error {
	return nil
}

func (e *strategyNotifyOnlyOrderExecutor) describeOrderSignal(order bbgotypes.SubmitOrder) string {
	marketPrice := e.runner.currentPrice()
	preparedPrice := marketPrice
	if order.Price.Sign() > 0 {
		preparedPrice = order.Price.Float64()
	}
	return fmt.Sprintf(
		"%s / %s: %s %s 股，预备下单价格 %s，当时市价 %s，仅通知模式",
		strategyRuntimeDisplayName(e.instance, e.runner),
		order.Symbol,
		strategyRuntimeSideLabel(order.Side),
		strategyRuntimeFormatNumber(order.Quantity.Float64()),
		strategyRuntimeFormatPrice(preparedPrice),
		strategyRuntimeFormatPrice(marketPrice),
	)
}

func (e *strategyLiveOrderExecutor) SubmitOrders(ctx context.Context, orders ...bbgotypes.SubmitOrder) (bbgotypes.OrderSlice, error) {
	placedOrders := make(bbgotypes.OrderSlice, 0, len(orders))
	for _, order := range orders {
		e.manager.recordSignal(e.instance.ID, time.Now().UTC())
		placed, err := e.server.placeExecutionOrder(ctx, normalizedExecutionPlaceOrder{
			brokerID:    strategyRuntimeBrokerID(e.instance.Binding),
			query:       strategyRuntimeBrokerPlaceOrderQuery(e.instance.Binding, order.Symbol),
			submitOrder: order,
			symbol:      order.Symbol,
			side:        strings.ToUpper(string(order.Side)),
			orderType:   strings.ToUpper(string(order.Type)),
			remark:      fmt.Sprintf("strategy runtime %s", e.instance.ID),
			session:     "",
		})
		if err != nil {
			e.manager.recordError(e.instance.ID, err.Error(), time.Now().UTC())
			_ = e.server.strategyStore.appendStrategyRuntimeEvent(
				e.instance.ID,
				fmt.Sprintf("live order failed %s %s %s", order.Symbol, strings.ToUpper(string(order.Side)), strategyRuntimeFormatNumber(order.Quantity.Float64())),
				"order_submit_failed",
				err.Error(),
			)
			return placedOrders, err
		}
		e.manager.recordOrder(e.instance.ID, time.Now().UTC())
		_ = e.server.strategyStore.appendStrategyRuntimeEvent(
			e.instance.ID,
			fmt.Sprintf("live order submitted %s %s %s", order.Symbol, strings.ToUpper(string(order.Side)), strategyRuntimeFormatNumber(order.Quantity.Float64())),
			"order_submitted",
			fmt.Sprintf("internalOrderId=%s", placed.InternalOrderID),
		)
		placedOrders = append(placedOrders, bbgotypes.Order{SubmitOrder: order})
	}
	return placedOrders, nil
}

func (e *strategyLiveOrderExecutor) CancelOrders(context.Context, ...bbgotypes.Order) error {
	return nil
}

func (m *strategyRuntimeManager) runtimeObservation(instanceID string) (strategyRuntimeObservation, bool) {
	runtime := m.runtime(instanceID)
	if runtime == nil {
		return strategyRuntimeObservation{}, false
	}
	return runtime.observation(), true
}

func (m *strategyRuntimeManager) runtimeSummary() map[string]any {
	m.mu.RLock()
	runtimes := make([]*managedStrategyRuntime, 0, len(m.runtimes))
	for _, runtime := range m.runtimes {
		runtimes = append(runtimes, runtime)
	}
	m.mu.RUnlock()

	activeInstances := make([]strategyRuntimeActiveInstanceSummary, 0, len(runtimes))
	for _, runtime := range runtimes {
		observation := runtime.observation()
		activeInstances = append(activeInstances, strategyRuntimeActiveInstanceSummary{
			InstanceID:        runtime.instanceID,
			DefinitionName:    strings.TrimSpace(runtime.definition.Name),
			ActualStatus:      observation.ActualStatus,
			ActiveSymbols:     observation.ActiveSymbols,
			LastClosedKLineAt: observation.LastClosedKLineAt,
			LastSignalAt:      observation.LastSignalAt,
			LastOrderAt:       observation.LastOrderAt,
			LastErrorAt:       observation.LastErrorAt,
			LastError:         observation.LastError,
			UpdatedAt:         observation.UpdatedAt,
		})
	}
	sort.Slice(activeInstances, func(i int, j int) bool {
		return activeInstances[i].InstanceID < activeInstances[j].InstanceID
	})

	status := "idle"
	if len(activeInstances) > 0 {
		status = "active"
	}
	return map[string]any{
		"status":                 status,
		"activeStrategies":       len(activeInstances),
		"supportsBacktestParity": true,
		"activeInstances":        activeInstances,
	}
}

func (m *strategyRuntimeManager) runtime(instanceID string) *managedStrategyRuntime {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.runtimes[instanceID]
}

func (m *strategyRuntimeManager) recordClosedKLine(instanceID string, at time.Time) {
	if runtime := m.runtime(instanceID); runtime != nil {
		runtime.recordClosedKLine(at)
	}
}

func (m *strategyRuntimeManager) recordSignal(instanceID string, at time.Time) {
	if runtime := m.runtime(instanceID); runtime != nil {
		runtime.recordSignal(at)
	}
}

func (m *strategyRuntimeManager) recordOrder(instanceID string, at time.Time) {
	if runtime := m.runtime(instanceID); runtime != nil {
		runtime.recordOrder(at)
	}
}

func (m *strategyRuntimeManager) recordError(instanceID string, message string, at time.Time) {
	if runtime := m.runtime(instanceID); runtime != nil {
		runtime.recordError(message, at)
	}
}

func (m *strategyRuntimeManager) handleRuntimePanic(instanceID string, symbol string, recovered any) {
	detail := fmt.Sprintf("strategy runtime panic on %s: %v", symbol, recovered)
	m.recordError(instanceID, detail, time.Now().UTC())
	m.stopStrategy(instanceID)
	_ = m.server.strategyStore.reconcileStrategyRuntimeFailure(instanceID, detail)
	m.server.recordLiveNotification(liveNotification{
		At:       time.Now().UTC().Format(time.RFC3339Nano),
		Level:    "error",
		Title:    "策略运行异常退出",
		Message:  detail,
		Source:   "strategy.runtime",
		Category: "strategy.runtime.exit",
	})
	if activeInstrumentIDs := m.server.activeLiveStreamInstrumentIDs(); len(activeInstrumentIDs) > 0 {
		m.server.ensureLiveMarketStream(context.Background(), activeInstrumentIDs)
	}
}

func (runtime *managedStrategyRuntime) observation() strategyRuntimeObservation {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	return strategyRuntimeObservation{
		ActualStatus:      strategyStatusRunning,
		ActiveSymbols:     strategyRuntimeSortedSymbols(runtime.symbols),
		LastClosedKLineAt: strategyRuntimeOptionalTimestamp(runtime.lastClosedKLineAt),
		LastSignalAt:      strategyRuntimeOptionalTimestamp(runtime.lastSignalAt),
		LastOrderAt:       strategyRuntimeOptionalTimestamp(runtime.lastOrderAt),
		LastErrorAt:       strategyRuntimeOptionalTimestamp(runtime.lastErrorAt),
		LastError:         strategyRuntimeOptionalString(runtime.lastError),
		UpdatedAt:         strategyRuntimeOptionalTimestamp(runtime.updatedAt),
	}
}

func (runtime *managedStrategyRuntime) recordClosedKLine(at time.Time) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if at.After(runtime.lastClosedKLineAt) {
		runtime.lastClosedKLineAt = at
	}
	runtime.updatedAt = strategyRuntimeMaxTime(runtime.updatedAt, at)
}

func (runtime *managedStrategyRuntime) recordSignal(at time.Time) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if at.After(runtime.lastSignalAt) {
		runtime.lastSignalAt = at
	}
	runtime.updatedAt = strategyRuntimeMaxTime(runtime.updatedAt, at)
}

func (runtime *managedStrategyRuntime) recordOrder(at time.Time) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if at.After(runtime.lastOrderAt) {
		runtime.lastOrderAt = at
	}
	runtime.updatedAt = strategyRuntimeMaxTime(runtime.updatedAt, at)
}

func (runtime *managedStrategyRuntime) recordError(message string, at time.Time) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if at.After(runtime.lastErrorAt) {
		runtime.lastErrorAt = at
	}
	runtime.lastError = strings.TrimSpace(message)
	runtime.updatedAt = strategyRuntimeMaxTime(runtime.updatedAt, at)
}

func strategyRuntimeBrokerReadQuery(binding strategyInstanceBinding) futu.BrokerReadQuery {
	query := futu.BrokerReadQuery{}
	if binding.BrokerAccount == nil {
		return query
	}
	query.AccountID = strings.TrimSpace(binding.BrokerAccount.AccountID)
	query.TradingEnvironment = strings.TrimSpace(binding.BrokerAccount.TradingEnvironment)
	query.Market = strings.TrimSpace(binding.BrokerAccount.Market)
	return query
}

func strategyRuntimeBrokerPlaceOrderQuery(binding strategyInstanceBinding, symbol string) futu.BrokerPlaceOrderQuery {
	query := futu.BrokerPlaceOrderQuery{BrokerReadQuery: strategyRuntimeBrokerReadQuery(binding)}
	if strings.TrimSpace(query.Market) == "" {
		query.Market = strategyRuntimeMarketFromSymbol(symbol, "")
	}
	return query
}

func strategyRuntimeBrokerID(binding strategyInstanceBinding) string {
	if binding.BrokerAccount == nil || strings.TrimSpace(binding.BrokerAccount.BrokerID) == "" {
		return "futu"
	}
	return strings.ToLower(strings.TrimSpace(binding.BrokerAccount.BrokerID))
}

func strategyRuntimeDefinitionID(instance managedStrategyInstance) string {
	definitionID, _ := instance.Params["definitionId"].(string)
	return strings.TrimSpace(definitionID)
}

func strategyRuntimeDisplayName(instance managedStrategyInstance, runner *strategySymbolRuntime) string {
	name := strings.TrimSpace(instance.Definition.Name)
	if name == "" && runner != nil {
		name = strings.TrimSpace(runner.name)
	}
	if name == "" {
		name = strings.TrimSpace(instance.Definition.StrategyID)
	}
	if name == "" {
		name = strings.TrimSpace(instance.ID)
	}
	return name
}

func strategyRuntimeSideLabel(side bbgotypes.SideType) string {
	if strings.EqualFold(string(side), string(bbgotypes.SideTypeSell)) {
		return "卖出"
	}
	return "买入"
}

func strategyRuntimeFormatPrice(value float64) string {
	if value <= 0 {
		return "-"
	}
	return strategyRuntimeFormatNumber(value)
}

func strategyRuntimeFormatNumber(value float64) string {
	text := strconv.FormatFloat(value, 'f', 4, 64)
	text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

func strategyRuntimeBucketWindow(tradeTime time.Time, interval bbgotypes.Interval) (time.Time, time.Time) {
	duration := interval.Duration()
	start := tradeTime.UTC().Truncate(duration)
	end := start.Add(duration).Add(-time.Millisecond)
	return start, end
}

func strategyRuntimeTradeKLine(exchange bbgotypes.ExchangeName, symbol string, interval bbgotypes.Interval, trade bbgotypes.Trade, start time.Time, end time.Time) bbgotypes.KLine {
	quoteVolume := trade.QuoteQuantity
	if quoteVolume.Sign() <= 0 {
		quoteVolume = trade.Quantity.Mul(trade.Price)
	}
	kline := bbgotypes.KLine{
		Exchange:    exchange,
		Symbol:      symbol,
		StartTime:   bbgotypes.Time(start),
		EndTime:     bbgotypes.Time(end),
		Interval:    interval,
		Open:        trade.Price,
		Close:       trade.Price,
		High:        trade.Price,
		Low:         trade.Price,
		Volume:      trade.Quantity,
		QuoteVolume: quoteVolume,
		Closed:      false,
	}
	if strings.EqualFold(string(trade.Side), string(bbgotypes.SideTypeBuy)) {
		kline.TakerBuyBaseAssetVolume = trade.Quantity
		kline.TakerBuyQuoteAssetVolume = quoteVolume
	}
	if trade.ID > 0 {
		kline.LastTradeID = trade.ID
	}
	kline.NumberOfTrades = 1
	return kline
}

func buildStrategyRuntimeAccount(funds *futu.BrokerFundsSnapshot, positions []futu.BrokerPositionSnapshot, market bbgotypes.Market, symbol string) *bbgotypes.Account {
	account := bbgotypes.NewAccount()
	account.CanDeposit = true
	account.CanTrade = true
	account.CanWithdraw = true
	if funds != nil {
		account.RawAccount = funds
		if funds.TotalAssets != nil {
			account.TotalAccountValue = fixedpoint.NewFromFloat(*funds.TotalAssets)
		}
		for _, balance := range funds.CurrencyBalances {
			currency := strings.ToUpper(strings.TrimSpace(balance.Currency))
			if currency == "" {
				continue
			}
			entry := bbgotypes.NewZeroBalance(currency)
			if balance.NetCashPower != nil {
				entry.Available = fixedpoint.NewFromFloat(*balance.NetCashPower)
				entry.NetAsset = fixedpoint.NewFromFloat(*balance.NetCashPower)
			}
			if balance.Cash != nil && balance.NetCashPower == nil {
				entry.Available = fixedpoint.NewFromFloat(*balance.Cash)
				entry.NetAsset = fixedpoint.NewFromFloat(*balance.Cash)
			}
			if balance.AvailableWithdrawalCash != nil {
				entry.MaxWithdrawAmount = fixedpoint.NewFromFloat(*balance.AvailableWithdrawalCash)
			}
			account.SetBalance(currency, entry)
		}
		if len(funds.CurrencyBalances) == 0 {
			currency := strings.ToUpper(strings.TrimSpace(market.QuoteCurrency))
			if currency == "" && funds.Currency != nil {
				currency = strings.ToUpper(strings.TrimSpace(*funds.Currency))
			}
			if currency != "" {
				entry := bbgotypes.NewZeroBalance(currency)
				if funds.AvailableFunds != nil {
					entry.Available = fixedpoint.NewFromFloat(*funds.AvailableFunds)
					entry.NetAsset = fixedpoint.NewFromFloat(*funds.AvailableFunds)
				}
				if funds.MaxWithdrawal != nil {
					entry.MaxWithdrawAmount = fixedpoint.NewFromFloat(*funds.MaxWithdrawal)
				}
				account.SetBalance(currency, entry)
			}
		}
	}

	baseCurrency := strings.ToUpper(strings.TrimSpace(market.BaseCurrency))
	if baseCurrency != "" {
		baseEntry := bbgotypes.NewZeroBalance(baseCurrency)
		for _, position := range positions {
			if !strings.EqualFold(strings.TrimSpace(position.Symbol), symbol) {
				continue
			}
			baseEntry.Available = baseEntry.Available.Add(fixedpoint.NewFromFloat(position.SellableQuantity))
			baseEntry.NetAsset = baseEntry.NetAsset.Add(fixedpoint.NewFromFloat(position.Quantity))
			lockedQuantity := position.Quantity - position.SellableQuantity
			if lockedQuantity > 0 {
				baseEntry.Locked = baseEntry.Locked.Add(fixedpoint.NewFromFloat(lockedQuantity))
			}
		}
		if baseEntry.Available.Sign() > 0 || baseEntry.Locked.Sign() > 0 || baseEntry.NetAsset.Sign() > 0 {
			account.SetBalance(baseCurrency, baseEntry)
		}
	}
	return account
}

func strategyRuntimeMarketFromSymbol(symbol string, fallback string) string {
	normalized := strings.ToUpper(strings.TrimSpace(symbol))
	if strings.Contains(normalized, ".") {
		parts := strings.SplitN(normalized, ".", 2)
		if strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}
	if strings.Contains(normalized, ":") {
		parts := strings.SplitN(normalized, ":", 2)
		if strings.TrimSpace(parts[0]) != "" {
			return strings.TrimSpace(parts[0])
		}
	}
	return strings.ToUpper(strings.TrimSpace(fallback))
}

func strategyRuntimeStartError(err error) (int, string) {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "required"),
		strings.Contains(message, "missing"),
		strings.Contains(message, "invalid"),
		strings.Contains(message, "unsupported"),
		strings.Contains(message, "already running"):
		return 400, "BAD_REQUEST"
	default:
		return 502, "STRATEGY_RUNTIME_START_FAILED"
	}
}

func strategyRuntimeMaxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func strategyRuntimeWarmupUntil(klines []bbgotypes.KLine, interval bbgotypes.Interval) time.Time {
	for index := len(klines) - 1; index >= 0; index-- {
		kline := klines[index]
		if !kline.Closed {
			currentStart := kline.StartTime.Time().UTC()
			if !currentStart.IsZero() {
				return currentStart
			}
			continue
		}
		closedStart := kline.StartTime.Time().UTC()
		if !closedStart.IsZero() {
			return closedStart.Add(interval.Duration())
		}
	}
	return time.Time{}
}

func strategyRuntimeOptionalTimestamp(value time.Time) *string {
	if value.IsZero() {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func strategyRuntimeOptionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func strategyRuntimeSortedSymbols(symbols map[string]*strategySymbolRuntime) []string {
	result := make([]string, 0, len(symbols))
	for symbol := range symbols {
		result = append(result, symbol)
	}
	sort.Strings(result)
	return result
}

func strategyRuntimeMaxTime(left time.Time, right time.Time) time.Time {
	if right.After(left) {
		return right
	}
	return left
}
