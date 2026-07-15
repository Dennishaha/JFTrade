package servercore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

var (
	strategyRuntimeClosedKLineSyncInterval = 5 * time.Second
	strategyRuntimeClosedKLineSyncLimit    = 8
)

type strategyRuntimeExchange interface {
	bbgotypes.Exchange
	QueryBrokerFunds(ctx context.Context, query broker.ReadQuery) (*broker.FundsSnapshot, error)
	QueryBrokerPositions(ctx context.Context, query broker.ReadQuery) ([]broker.PositionSnapshot, error)
	PlaceBrokerOrder(ctx context.Context, query broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error)
	CancelBrokerOrder(ctx context.Context, query broker.ReadQuery, order broker.CancelOrder) error
}

type strategyRuntimeMarketEnsurer interface {
	EnsureMarket(symbol string)
}

type strategyRuntimeManager struct {
	exchangeProvider func() strategyRuntimeExchange
	pineWorkerRunner strategyRuntimePineWorker
	deps             strategyRuntimeManagerDeps

	mu       sync.RWMutex
	runtimes map[string]*managedStrategyRuntime
	starting map[string]struct{}
}

type strategyRuntimeManagerDeps struct {
	pineWorkerLimit         func() int
	wakeMarketDataCollector func()
	currentInstance         func(instanceID string) (managedStrategyInstance, bool)
	appendRuntimeEvent      func(instanceID string, logMessage string, kind string, detail string) error
	transitionInstance      func(instanceID string, nextStatus string, kind string, detail string) error
	reconcileRuntimeFailure func(instanceID string, detail string) error
	recordNotification      func(strategyRuntimeNotification)
	placeExecutionOrder     func(context.Context, trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error)
	cancelExecutionOrder    func(context.Context, string) (trdsrv.ExecutionOrder, error)
	countRuntimeAudit       func(context.Context, runtimeactivity.AuditQuery) (int, error)
	upsertObservation       func(context.Context, runtimeactivity.ObservationSnapshot) error
	acquireMarketDataLease  func(context.Context, string, []mdsrv.InstrumentRef) (*mdsrv.ManagedSubscription, error)
}

type strategyRuntimeNotification struct {
	At       string
	Level    string
	Title    string
	Message  string
	Source   string
	BrokerID string
	Category string
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
	subscriptionLease *mdsrv.ManagedSubscription
}

type strategySymbolRuntime struct {
	instanceID      string
	name            string
	symbol          string
	interval        bbgotypes.Interval
	exchange        bbgotypes.ExchangeName
	ctx             context.Context
	runtimeExchange strategyRuntimeExchange
	brokerQuery     broker.ReadQuery
	market          bbgotypes.Market
	cachedFunds     *broker.FundsSnapshot
	cachedPositions []broker.PositionSnapshot
	session         *bbgo.ExchangeSession
	emitter         bbgotypes.StandardStreamEmitter
	pineWorkerLive  *strategyRuntimePineWorkerLive
	onClosedKLine   func(time.Time)
	onError         func(string)

	mu              sync.RWMutex
	currentBucket   *bbgotypes.KLine
	lastClosedPrice float64
	lastClosedKLine time.Time
}

type strategyNotifyOnlyOrderExecutor struct {
	manager  *strategyRuntimeManager
	instance managedStrategyInstance
	runner   *strategySymbolRuntime
}

type strategyLiveOrderExecutor struct {
	manager  *strategyRuntimeManager
	instance managedStrategyInstance
	runner   *strategySymbolRuntime

	mu                      sync.Mutex
	trackedInternalOrderIDs map[string]string
}

func newStrategyRuntimeManager(server *Server) *strategyRuntimeManager {
	manager := &strategyRuntimeManager{
		runtimes: map[string]*managedStrategyRuntime{},
		starting: map[string]struct{}{},
		deps:     newStrategyRuntimeManagerDeps(server),
	}
	manager.exchangeProvider = func() strategyRuntimeExchange {
		exchange := server.futuExchange()
		activeBroker := server.activeBroker()
		if exchange == nil || activeBroker == nil {
			return nil
		}
		return &strategyRuntimeBrokerBridge{
			Exchange: exchange,
			broker:   activeBroker,
		}
	}
	manager.pineWorkerRunner = server.instancePineWorkerRunner
	return manager
}

func (s *Server) recordStrategyRuntimeNotification(note strategyRuntimeNotification) {
	s.recordLiveNotification(liveNotification{
		At:       note.At,
		Level:    note.Level,
		Title:    note.Title,
		Message:  note.Message,
		Source:   note.Source,
		BrokerID: note.BrokerID,
		Category: note.Category,
	})
}

func newStrategyRuntimeManagerDeps(server *Server) strategyRuntimeManagerDeps {
	return strategyRuntimeManagerDeps{
		pineWorkerLimit: func() int {
			return settingsfile.NormalizePineWorkerSettings(server.pineWorkerSettings()).InstanceWorkerLimit
		},
		wakeMarketDataCollector: func() {
			if server.marketdataSvc != nil {
				server.marketdataSvc.WakeCollector()
			}
		},
		currentInstance: func(instanceID string) (managedStrategyInstance, bool) {
			if server.strategyStore == nil {
				return managedStrategyInstance{}, false
			}
			return server.strategyStore.strategy(instanceID)
		},
		appendRuntimeEvent: func(instanceID string, logMessage string, kind string, detail string) error {
			if server.strategyStore == nil {
				return nil
			}
			return server.strategyStore.appendStrategyRuntimeEvent(instanceID, logMessage, kind, detail)
		},
		transitionInstance: func(instanceID string, nextStatus string, kind string, detail string) error {
			if server.strategyStore == nil {
				return nil
			}
			_, err := server.strategyStore.transitionStrategy(instanceID, nextStatus, kind, detail)
			return err
		},
		reconcileRuntimeFailure: func(instanceID string, detail string) error {
			if server.strategyStore == nil {
				return nil
			}
			return server.strategyStore.reconcileStrategyRuntimeFailure(instanceID, detail)
		},
		recordNotification: server.recordStrategyRuntimeNotification,
		placeExecutionOrder: func(ctx context.Context, command trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error) {
			if server.tradingSvc == nil {
				return trdsrv.ExecutionOrder{}, fmt.Errorf("trading service is unavailable")
			}
			return server.tradingSvc.PlaceExecutionOrder(ctx, command)
		},
		cancelExecutionOrder: func(ctx context.Context, internalOrderID string) (trdsrv.ExecutionOrder, error) {
			if server.tradingSvc == nil {
				return trdsrv.ExecutionOrder{}, fmt.Errorf("trading service is unavailable")
			}
			response, err := server.tradingSvc.CancelExecutionOrder(ctx, internalOrderID)
			if err != nil {
				return trdsrv.ExecutionOrder{}, err
			}
			if response.InternalOrderID == nil {
				return trdsrv.ExecutionOrder{}, fmt.Errorf("cancel execution order response missing internal order id")
			}
			return trdsrv.ExecutionOrder{InternalOrderID: *response.InternalOrderID}, nil
		},
		countRuntimeAudit: func(ctx context.Context, query runtimeactivity.AuditQuery) (int, error) {
			if server.strategyRuntimeStore == nil {
				return 0, nil
			}
			return server.strategyRuntimeStore.CountAudit(ctx, query)
		},
		upsertObservation: func(ctx context.Context, snapshot runtimeactivity.ObservationSnapshot) error {
			if server.strategyRuntimeStore == nil {
				return nil
			}
			return server.strategyRuntimeStore.UpsertObservation(ctx, snapshot)
		},
		acquireMarketDataLease: func(ctx context.Context, consumerID string, refs []mdsrv.InstrumentRef) (*mdsrv.ManagedSubscription, error) {
			if server.marketdataSvc == nil {
				return nil, fmt.Errorf("market-data service is unavailable")
			}
			return server.marketdataSvc.AcquireManagedSubscription(ctx, consumerID, refs)
		},
	}
}

func (m *strategyRuntimeManager) pineWorkerLimit() int {
	limit := settingsfile.DefaultPineWorkerSettings().InstanceWorkerLimit
	if m != nil && m.deps.pineWorkerLimit != nil {
		if configured := m.deps.pineWorkerLimit(); configured > 0 {
			limit = configured
		}
	}
	return limit
}

func (m *strategyRuntimeManager) wakeMarketDataCollector() {
	if m != nil && m.deps.wakeMarketDataCollector != nil {
		m.deps.wakeMarketDataCollector()
	}
}

func (m *strategyRuntimeManager) currentInstance(instanceID string) (managedStrategyInstance, bool) {
	if m == nil || m.deps.currentInstance == nil {
		return managedStrategyInstance{}, false
	}
	return m.deps.currentInstance(instanceID)
}

func (m *strategyRuntimeManager) appendRuntimeEvent(instanceID string, logMessage string, kind string, detail string) error {
	if m == nil || m.deps.appendRuntimeEvent == nil {
		return nil
	}
	return m.deps.appendRuntimeEvent(instanceID, logMessage, kind, detail)
}

func (m *strategyRuntimeManager) transitionInstance(instanceID string, nextStatus string, kind string, detail string) error {
	if m == nil || m.deps.transitionInstance == nil {
		return nil
	}
	return m.deps.transitionInstance(instanceID, nextStatus, kind, detail)
}

func (m *strategyRuntimeManager) reconcileRuntimeFailure(instanceID string, detail string) error {
	if m == nil || m.deps.reconcileRuntimeFailure == nil {
		return nil
	}
	return m.deps.reconcileRuntimeFailure(instanceID, detail)
}

func (m *strategyRuntimeManager) recordNotification(note strategyRuntimeNotification) {
	if m != nil && m.deps.recordNotification != nil {
		m.deps.recordNotification(note)
	}
}

func (m *strategyRuntimeManager) placeExecutionOrder(ctx context.Context, command trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error) {
	if m == nil || m.deps.placeExecutionOrder == nil {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("strategy runtime order placement is unavailable")
	}
	return m.deps.placeExecutionOrder(ctx, command)
}

func (m *strategyRuntimeManager) cancelExecutionOrder(ctx context.Context, internalOrderID string) (trdsrv.ExecutionOrder, error) {
	if m == nil || m.deps.cancelExecutionOrder == nil {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("strategy runtime order cancellation is unavailable")
	}
	return m.deps.cancelExecutionOrder(ctx, internalOrderID)
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
	interval, script, err := validateStrategyRuntimeInstance(instance)
	if err != nil {
		return err
	}
	if err := m.ensureStrategyStopped(instance.ID); err != nil {
		return err
	}
	releaseStartReservation, err := m.reserveRuntimeStart(instance.ID)
	if err != nil {
		return err
	}
	defer releaseStartReservation()
	exchange, markets, funds, positions, err := m.loadStrategyRuntimeInputs(ctx, instance)
	if err != nil {
		return err
	}
	managed, err := m.buildManagedStrategyRuntime(ctx, exchange, markets, funds, positions, instance, script, interval)
	if err != nil {
		return err
	}
	return m.activateStrategyRuntime(instance.ID, managed)
}

func validateStrategyRuntimeInstance(instance managedStrategyInstance) (bbgotypes.Interval, string, error) {
	interval := bbgotypes.Interval(strings.TrimSpace(instance.Binding.Interval))
	if duration, ok := strategyRuntimeIntervalDuration(interval); !ok || duration <= 0 {
		return "", "", fmt.Errorf("strategy interval %q is invalid", instance.Binding.Interval)
	}
	if len(instance.Binding.Symbols) == 0 {
		return "", "", fmt.Errorf("strategy instance requires at least one symbol binding")
	}
	script, ok := instance.Params["script"].(string)
	if !ok || strings.TrimSpace(script) == "" {
		return "", "", fmt.Errorf("strategy instance is missing script")
	}
	return interval, script, nil
}

func (m *strategyRuntimeManager) ensureStrategyStopped(instanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.runtimes[instanceID]; exists {
		return fmt.Errorf("strategy instance is already running")
	}
	return nil
}

func (m *strategyRuntimeManager) loadStrategyRuntimeInputs(ctx context.Context, instance managedStrategyInstance) (strategyRuntimeExchange, map[string]bbgotypes.Market, *broker.FundsSnapshot, []broker.PositionSnapshot, error) {
	exchange := m.exchangeProvider()
	if exchange == nil {
		return nil, nil, nil, nil, fmt.Errorf("strategy runtime exchange is unavailable")
	}
	if marketEnsurer, ok := exchange.(strategyRuntimeMarketEnsurer); ok {
		for _, symbol := range instance.Binding.Symbols {
			marketEnsurer.EnsureMarket(symbol)
		}
	}
	markets, err := exchange.QueryMarkets(ctx)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("load strategy markets: %w", err)
	}
	brokerQuery := strategyRuntimeBrokerReadQuery(instance.Binding)
	funds, err := exchange.QueryBrokerFunds(ctx, brokerQuery)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("load strategy broker funds: %w", err)
	}
	positions, err := exchange.QueryBrokerPositions(ctx, brokerQuery)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("load strategy broker positions: %w", err)
	}
	return exchange, markets, funds, positions, nil
}

func (m *strategyRuntimeManager) buildManagedStrategyRuntime(ctx context.Context, exchange strategyRuntimeExchange, markets map[string]bbgotypes.Market, funds *broker.FundsSnapshot, positions []broker.PositionSnapshot, instance managedStrategyInstance, script string, interval bbgotypes.Interval) (*managedStrategyRuntime, error) {
	runtimeCtx, cancel := context.WithCancel(context.Background())
	managed := &managedStrategyRuntime{
		instanceID: instance.ID,
		definition: instance.Definition,
		cancel:     cancel,
		symbols:    make(map[string]*strategySymbolRuntime, len(instance.Binding.Symbols)),
		updatedAt:  time.Now().UTC(),
	}
	if m.deps.acquireMarketDataLease != nil {
		lease, err := m.deps.acquireMarketDataLease(ctx, "strategy-runtime:"+instance.ID, strategyKLineSubscriptionRefs(instance.Binding.Symbols, interval))
		if err != nil {
			cancel()
			return nil, fmt.Errorf("acquire strategy market-data subscriptions: %w", err)
		}
		managed.subscriptionLease = lease
	}
	for _, symbol := range instance.Binding.Symbols {
		runner, err := m.buildSymbolRuntime(ctx, runtimeCtx, exchange, markets, funds, positions, instance, script, symbol, interval)
		if err != nil {
			cancel()
			managed.subscriptionLease.Release()
			return nil, err
		}
		managed.symbols[symbol] = runner
	}
	return managed, nil
}

func (m *strategyRuntimeManager) activateStrategyRuntime(instanceID string, managed *managedStrategyRuntime) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.runtimes[instanceID]; exists {
		managed.cancel()
		managed.subscriptionLease.Release()
		return fmt.Errorf("strategy instance is already running")
	}
	m.runtimes[instanceID] = managed
	m.persistObservationSnapshot(managed.snapshot(strategyStatusRunning))
	for _, runner := range managed.symbols {
		go runner.syncClosedKLinesLoop()
	}
	m.wakeMarketDataCollector()
	return nil
}

func (m *strategyRuntimeManager) reserveRuntimeStart(instanceID string) (func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.runtimes[instanceID]; exists {
		return nil, fmt.Errorf("strategy instance is already running")
	}
	if _, exists := m.starting[instanceID]; exists {
		return nil, fmt.Errorf("strategy instance is already starting")
	}
	if m.starting == nil {
		m.starting = map[string]struct{}{}
	}
	limit := m.pineWorkerLimit()
	if len(m.runtimes)+len(m.starting) >= limit {
		return nil, pineworker.CapacityExceededError{Workers: limit}
	}
	m.starting[instanceID] = struct{}{}
	return func() {
		m.mu.Lock()
		delete(m.starting, instanceID)
		m.mu.Unlock()
	}, nil
}

func (m *strategyRuntimeManager) stopStrategy(instanceID string) {
	m.mu.Lock()
	runtime, exists := m.runtimes[instanceID]
	if exists {
		delete(m.runtimes, instanceID)
	}
	m.mu.Unlock()
	if exists {
		m.persistObservationSnapshot(runtime.snapshot(strategyStatusStopped))
	}
	if exists && runtime.cancel != nil {
		runtime.cancel()
	}
	if exists {
		runtime.subscriptionLease.Release()
	}
	if exists {
		m.wakeMarketDataCollector()
	}
}

func (m *strategyRuntimeManager) close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	runtimes := make([]*managedStrategyRuntime, 0, len(m.runtimes))
	for instanceID, runtime := range m.runtimes {
		delete(m.runtimes, instanceID)
		runtimes = append(runtimes, runtime)
	}
	m.starting = map[string]struct{}{}
	m.mu.Unlock()
	for _, runtime := range runtimes {
		m.persistObservationSnapshot(runtime.snapshot(strategyStatusStopped))
		if runtime.cancel != nil {
			runtime.cancel()
		}
		runtime.subscriptionLease.Release()
	}
	if len(runtimes) > 0 {
		m.wakeMarketDataCollector()
	}
}

func strategyKLineSubscriptionRefs(symbols []string, interval bbgotypes.Interval) []mdsrv.InstrumentRef {
	refs := make([]mdsrv.InstrumentRef, 0, len(symbols))
	for _, raw := range symbols {
		parts := strings.SplitN(strings.ToUpper(strings.TrimSpace(raw)), ".", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		refs = append(refs, mdsrv.InstrumentRef{
			Channel: "KLINE", Market: parts[0], Symbol: parts[1], Interval: string(interval),
		})
	}
	return refs
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
	funds *broker.FundsSnapshot,
	positions []broker.PositionSnapshot,
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
		instanceID:      instance.ID,
		name:            strings.TrimSpace(instance.Definition.Name),
		symbol:          symbol,
		interval:        interval,
		exchange:        exchange.Name(),
		ctx:             runtimeCtx,
		runtimeExchange: exchange,
		brokerQuery:     strategyRuntimeBrokerReadQuery(instance.Binding),
		market:          market,
		cachedFunds:     cloneStrategyRuntimeFundsSnapshot(funds),
		cachedPositions: cloneStrategyRuntimePositions(positions),
		session:         session,
		emitter:         emitter,
		onClosedKLine: func(at time.Time) {
			m.recordClosedKLine(instance.ID, at)
		},
		onError: func(message string) {
			message = strings.TrimSpace(message)
			if message == "" {
				return
			}
			m.recordError(instance.ID, message, time.Now().UTC())
			jftradeErr2 := m.appendRuntimeEvent(
				instance.ID,
				fmt.Sprintf("runtime error %s: %s", symbol, message),
				"runtime_error",
				fmt.Sprintf("%s: %s", symbol, message),
			)
			jftradeLogError(jftradeErr2)
		},
	}

	recordIgnoredOrder := func(message string) {
		jftradeErr := m.appendRuntimeEvent(
			instance.ID,
			fmt.Sprintf("live order ignored %s", symbol),
			"order_ignored",
			message,
		)
		jftradeLogError(jftradeErr)
	}
	live, err := newStrategyRuntimePineWorkerLive(m.pineWorkerRunner, instance, symbol, interval, script, m.newOrderExecutor(instance, runner), runner, recordIgnoredOrder)
	if err != nil {
		return nil, fmt.Errorf("start strategy runtime for %s: %w", symbol, err)
	}
	runner.pineWorkerLive = live
	if err := m.seedSymbolRuntime(ctx, exchange, live, runner); err != nil {
		return nil, err
	}
	return runner, nil
}

func (m *strategyRuntimeManager) seedSymbolRuntime(ctx context.Context, exchange strategyRuntimeExchange, live *strategyRuntimePineWorkerLive, runner *strategySymbolRuntime) error {
	klines, err := live.loadWarmup(ctx, exchange)
	if err != nil {
		return err
	}
	for index := range klines {
		kline := klines[index]
		if !kline.Closed && index == len(klines)-1 {
			runner.setCurrentBucket(new(kline))
			continue
		}
		closed := kline
		closed.Closed = true
		runner.setLastClosedPrice(closed.Close.Float64())
		runner.recordClosedKLineState(closed)
		live.recordWarmupClosed(closed)
		runner.emitter.EmitKLineClosed(closed)
	}
	return nil
}

func (m *strategyRuntimeManager) newOrderExecutor(instance managedStrategyInstance, runner *strategySymbolRuntime) bbgo.OrderExecutor {
	if instance.Binding.ExecutionMode == strategyExecutionModeNotifyOnly {
		return &strategyNotifyOnlyOrderExecutor{manager: m, instance: instance, runner: runner}
	}
	return &strategyLiveOrderExecutor{manager: m, instance: instance, runner: runner}
}
