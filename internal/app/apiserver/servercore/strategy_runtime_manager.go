package servercore

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	"github.com/jftrade/jftrade-main/pkg/bbgo/fixedpoint"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"

	"github.com/jftrade/jftrade-main/internal/store/settingsfile"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	"github.com/jftrade/jftrade-main/internal/strategy/runtimecontrol"
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
		recordNotification: func(note strategyRuntimeNotification) {
			server.recordLiveNotification(liveNotification{
				At:       note.At,
				Level:    note.Level,
				Title:    note.Title,
				Message:  note.Message,
				Source:   note.Source,
				BrokerID: note.BrokerID,
				Category: note.Category,
			})
		},
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
	interval := bbgotypes.Interval(strings.TrimSpace(instance.Binding.Interval))
	if duration, ok := strategyRuntimeIntervalDuration(interval); !ok || duration <= 0 {
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

	releaseStartReservation, err := m.reserveRuntimeStart(instance.ID)
	if err != nil {
		return err
	}
	defer releaseStartReservation()

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
		m.wakeMarketDataCollector()
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

func (r *strategySymbolRuntime) syncClosedKLinesLoop() {
	if r == nil || strategyRuntimeClosedKLineSyncInterval <= 0 {
		return
	}
	ticker := time.NewTicker(strategyRuntimeClosedKLineSyncInterval)
	defer ticker.Stop()
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.syncClosedKLines()
		}
	}
}

func (r *strategySymbolRuntime) syncClosedKLines() {
	if r == nil || r.runtimeExchange == nil {
		return
	}
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	limit := strategyRuntimeClosedKLineSyncLimit
	if limit <= 0 {
		limit = 8
	}
	klines, err := r.runtimeExchange.QueryKLines(ctx, r.symbol, r.interval, bbgotypes.KLineQueryOptions{Limit: limit})
	if err != nil {
		r.handleRuntimeError(fmt.Errorf("refresh strategy klines for %s: %w", r.symbol, err))
		return
	}
	sort.SliceStable(klines, func(left int, right int) bool {
		return klines[left].StartTime.Time().Before(klines[right].StartTime.Time())
	})
	for index := range klines {
		kline := klines[index]
		if !kline.Closed {
			if index == len(klines)-1 {
				r.setCurrentBucket(new(kline))
			}
			continue
		}
		kline.Closed = true
		r.emitClosedKLine(kline)
	}
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
		r.emitClosedKLine(*closed)
	}
}

func (r *strategySymbolRuntime) recordClosedKLineState(closed bbgotypes.KLine) bool {
	closedAt := closed.EndTime.Time().UTC()
	r.mu.Lock()
	defer r.mu.Unlock()
	if !closedAt.After(r.lastClosedKLine) {
		if closed.Close.Sign() > 0 {
			r.lastClosedPrice = closed.Close.Float64()
		}
		return false
	}
	r.lastClosedKLine = closedAt
	if closed.Close.Sign() > 0 {
		r.lastClosedPrice = closed.Close.Float64()
	}
	return true
}

func (r *strategySymbolRuntime) emitClosedKLine(closed bbgotypes.KLine) {
	if !r.recordClosedKLineState(closed) {
		return
	}
	if err := r.refreshBrokerAccount(); err != nil {
		r.handleRuntimeError(err)
	}
	if r.onClosedKLine != nil {
		r.onClosedKLine(closed.EndTime.Time().UTC())
	}
	if r.pineWorkerLive != nil {
		if err := r.pineWorkerLive.onClosedKLine(r.context(), closed); err != nil {
			r.handleRuntimeError(err)
		}
	}
	r.emitter.EmitKLineClosed(closed)
}

func (r *strategySymbolRuntime) context() context.Context {
	if r != nil && r.ctx != nil {
		return r.ctx
	}
	return context.Background()
}

func (r *strategySymbolRuntime) handleRuntimeError(err error) {
	if err == nil {
		return
	}
	if r.onError != nil {
		r.onError(err.Error())
		return
	}
	log.Printf("JFTrade strategy runtime degraded: %v", err)
}

func (r *strategySymbolRuntime) refreshBrokerAccount() error {
	if r == nil || r.runtimeExchange == nil || r.session == nil {
		return nil
	}
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	funds := r.cachedFunds
	positions := r.cachedPositions
	freshFunds, err := r.runtimeExchange.QueryBrokerFunds(ctx, r.brokerQuery)
	if err != nil {
		if connectivityFromBrokerReadError(err) != "disconnected" {
			return fmt.Errorf("refresh strategy broker funds for %s: %w", r.symbol, err)
		}
		log.Printf("JFTrade strategy runtime broker funds refresh disconnected for %s: %v", r.symbol, err)
	} else {
		r.cachedFunds = cloneStrategyRuntimeFundsSnapshot(freshFunds)
		funds = r.cachedFunds
	}
	freshPositions, err := r.runtimeExchange.QueryBrokerPositions(ctx, r.brokerQuery)
	if err != nil {
		if connectivityFromBrokerReadError(err) != "disconnected" {
			return fmt.Errorf("refresh strategy broker positions for %s: %w", r.symbol, err)
		}
		log.Printf("JFTrade strategy runtime broker positions refresh disconnected for %s: %v", r.symbol, err)
	} else {
		r.cachedPositions = cloneStrategyRuntimePositions(freshPositions)
		positions = r.cachedPositions
	}
	r.session.Account = buildStrategyRuntimeAccount(funds, positions, r.market, r.symbol)
	return nil
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
	createdOrders := make(bbgotypes.OrderSlice, 0, len(orders))
	for _, order := range orders {
		e.manager.recordSignal(e.instance.ID, time.Now().UTC())
		message := e.describeOrderSignal(order)
		e.manager.recordNotification(strategyRuntimeNotification{
			At:       time.Now().UTC().Format(time.RFC3339Nano),
			Level:    "info",
			Title:    "策略下单信号",
			Message:  message,
			Source:   "strategy.runtime",
			BrokerID: strategyRuntimeBrokerID(e.instance.Binding),
			Category: "strategy.order.signal",
		})
		jftradeErr4 := e.manager.appendRuntimeEvent(
			e.instance.ID,
			fmt.Sprintf("notify-only signal %s %s %s", order.Symbol, strings.ToUpper(string(order.Side)), strategyRuntimeFormatNumber(order.Quantity.Float64())),
			"signal_notified",
			message,
		)
		jftradeLogError(jftradeErr4)
		createdOrders = append(createdOrders, bbgotypes.Order{SubmitOrder: order})
	}
	return createdOrders, nil
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

		placeQuery := strategyRuntimeBrokerPlaceOrderQuery(e.instance.Binding, order.Symbol)
		placeQuery.Side = strings.ToUpper(string(order.Side))
		placeQuery.OrderType = strings.ToUpper(string(order.Type))
		placeQuery.Quantity = order.Quantity.Float64()
		if order.Price.Sign() > 0 {
			placeQuery.Price = new(order.Price.Float64())
		}
		timeInForce := strings.ToUpper(string(order.TimeInForce))
		if timeInForce == "" {
			timeInForce = "DAY"
		}
		placeQuery.TimeInForce = &timeInForce
		remark := fmt.Sprintf("strategy runtime %s", e.instance.ID)
		placeQuery.Remark = &remark
		if order.ClientOrderID != "" {
			placeQuery.ClientOrderID = order.ClientOrderID
		}

		command := trdsrv.ExecutionOrderCommand{
			BrokerID:  strategyRuntimeBrokerID(e.instance.Binding),
			Query:     placeQuery,
			Symbol:    order.Symbol,
			Side:      strings.ToUpper(string(order.Side)),
			OrderType: strings.ToUpper(string(order.Type)),
			Remark:    remark,
		}
		decision := e.evaluateRuntimeRisk(command)
		e.recordRuntimeRiskDecision(decision, command)
		if decision.Rejected {
			return placedOrders, fmt.Errorf("runtime risk rejected order: %s", decision.Reason)
		}

		placed, err := e.manager.placeExecutionOrder(ctx, command)
		if err != nil {
			e.manager.recordError(e.instance.ID, err.Error(), time.Now().UTC())
			jftradeErr5 := e.manager.appendRuntimeEvent(
				e.instance.ID,
				fmt.Sprintf("live order failed %s %s %s", order.Symbol, strings.ToUpper(string(order.Side)), strategyRuntimeFormatNumber(order.Quantity.Float64())),
				"order_submit_failed",
				err.Error(),
			)
			jftradeLogError(jftradeErr5)
			return placedOrders, err
		}
		e.manager.recordOrder(e.instance.ID, time.Now().UTC())
		jftradeErr6 := e.manager.appendRuntimeEvent(
			e.instance.ID,
			fmt.Sprintf("live order submitted %s %s %s", order.Symbol, strings.ToUpper(string(order.Side)), strategyRuntimeFormatNumber(order.Quantity.Float64())),
			"order_submitted",
			fmt.Sprintf("internalOrderId=%s", placed.InternalOrderID),
		)
		jftradeLogError(jftradeErr6)
		e.trackOrder(order.ClientOrderID, placed.InternalOrderID)
		placedOrders = append(placedOrders, bbgotypes.Order{SubmitOrder: order})
	}
	return placedOrders, nil
}

func (e *strategyLiveOrderExecutor) CancelOrders(ctx context.Context, orders ...bbgotypes.Order) error {
	for _, order := range orders {
		clientOrderID := strings.TrimSpace(order.ClientOrderID)
		if clientOrderID == "" {
			continue
		}
		internalOrderID, ok := e.trackedInternalOrderID(clientOrderID)
		if !ok {
			continue
		}
		cancelled, err := e.manager.cancelExecutionOrder(ctx, internalOrderID)
		if err != nil {
			e.manager.recordError(e.instance.ID, err.Error(), time.Now().UTC())
			jftradeErr := e.manager.appendRuntimeEvent(
				e.instance.ID,
				fmt.Sprintf("live order cancel failed %s", clientOrderID),
				"order_cancel_failed",
				err.Error(),
			)
			jftradeLogError(jftradeErr)
			return err
		}
		e.untrackOrder(clientOrderID)
		jftradeErr := e.manager.appendRuntimeEvent(
			e.instance.ID,
			fmt.Sprintf("live order cancel requested %s", clientOrderID),
			"order_cancel_requested",
			fmt.Sprintf("internalOrderId=%s", cancelled.InternalOrderID),
		)
		jftradeLogError(jftradeErr)
	}
	return nil
}

func (e *strategyLiveOrderExecutor) trackOrder(clientOrderID string, internalOrderID string) {
	clientOrderID = strings.TrimSpace(clientOrderID)
	internalOrderID = strings.TrimSpace(internalOrderID)
	if clientOrderID == "" || internalOrderID == "" {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.trackedInternalOrderIDs == nil {
		e.trackedInternalOrderIDs = map[string]string{}
	}
	e.trackedInternalOrderIDs[clientOrderID] = internalOrderID
}

func (e *strategyLiveOrderExecutor) trackedInternalOrderID(clientOrderID string) (string, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	internalOrderID, ok := e.trackedInternalOrderIDs[strings.TrimSpace(clientOrderID)]
	return internalOrderID, ok
}

func (e *strategyLiveOrderExecutor) untrackOrder(clientOrderID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.trackedInternalOrderIDs, strings.TrimSpace(clientOrderID))
}

func (m *strategyRuntimeManager) runtimeObservation(instanceID string) (strategyRuntimeObservation, bool) {
	runtime := m.runtime(instanceID)
	if runtime == nil {
		return strategyRuntimeObservation{}, false
	}
	return runtime.observation(), true
}

func (m *strategyRuntimeManager) runtimeSummary() map[string]any {
	summary := m.typedRuntimeSummary()
	return map[string]any{
		"status":                 summary.Status,
		"activeStrategies":       summary.ActiveStrategies,
		"supportsBacktestParity": summary.SupportsBacktestParity,
		"activeInstances":        summary.ActiveInstances,
	}
}

func (m *strategyRuntimeManager) typedRuntimeSummary() stratsrv.RuntimeSummary {
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
	return stratsrv.RuntimeSummary{
		Status:                 status,
		ActiveStrategies:       len(activeInstances),
		SupportsBacktestParity: true,
		ActiveInstances:        activeInstances,
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
		m.persistObservationSnapshot(runtime.snapshot(strategyStatusRunning))
	}
}

func (m *strategyRuntimeManager) recordSignal(instanceID string, at time.Time) {
	if runtime := m.runtime(instanceID); runtime != nil {
		runtime.recordSignal(at)
		m.persistObservationSnapshot(runtime.snapshot(strategyStatusRunning))
	}
}

func (m *strategyRuntimeManager) recordOrder(instanceID string, at time.Time) {
	if runtime := m.runtime(instanceID); runtime != nil {
		runtime.recordOrder(at)
		m.persistObservationSnapshot(runtime.snapshot(strategyStatusRunning))
	}
}

func (m *strategyRuntimeManager) recordError(instanceID string, message string, at time.Time) {
	if runtime := m.runtime(instanceID); runtime != nil {
		runtime.recordError(message, at)
		m.persistObservationSnapshot(runtime.snapshot(strategyStatusRunning))
	}
}

func (m *strategyRuntimeManager) handleRuntimePanic(instanceID string, symbol string, recovered any) {
	detail := fmt.Sprintf("strategy runtime panic on %s: %v", symbol, recovered)
	m.recordError(instanceID, detail, time.Now().UTC())
	m.stopStrategy(instanceID)
	jftradeErr1 := m.reconcileRuntimeFailure(instanceID, detail)
	jftradeLogError(jftradeErr1)
	m.recordNotification(strategyRuntimeNotification{
		At:       time.Now().UTC().Format(time.RFC3339Nano),
		Level:    "error",
		Title:    "策略运行异常退出",
		Message:  detail,
		Source:   "strategy.runtime",
		Category: "strategy.runtime.exit",
	})
	m.wakeMarketDataCollector()
}

func (runtime *managedStrategyRuntime) observation() strategyRuntimeObservation {
	snapshot := runtime.snapshot(strategyStatusRunning)
	return strategyRuntimeObservationFromSnapshot(snapshot, strategyStatusRunning)
}

func (runtime *managedStrategyRuntime) snapshot(actualStatus string) runtimeactivity.ObservationSnapshot {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	return runtimeactivity.ObservationSnapshot{
		InstanceID:        runtime.instanceID,
		ActualStatus:      strings.TrimSpace(actualStatus),
		ActiveSymbols:     strategyRuntimeSortedSymbols(runtime.symbols),
		LastClosedKLineAt: strategyRuntimeOptionalTime(runtime.lastClosedKLineAt),
		LastSignalAt:      strategyRuntimeOptionalTime(runtime.lastSignalAt),
		LastOrderAt:       strategyRuntimeOptionalTime(runtime.lastOrderAt),
		LastErrorAt:       strategyRuntimeOptionalTime(runtime.lastErrorAt),
		LastError:         strings.TrimSpace(runtime.lastError),
		UpdatedAt:         strategyRuntimeOptionalTime(runtime.updatedAt),
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

func (m *strategyRuntimeManager) persistObservationSnapshot(snapshot runtimeactivity.ObservationSnapshot) {
	if m == nil || m.deps.upsertObservation == nil {
		return
	}
	if err := m.deps.upsertObservation(context.Background(), snapshot); err != nil {
		log.Printf("JFTrade persist strategy runtime observation degraded: %v", err)
	}
}

func strategyRuntimeObservationFromSnapshot(snapshot runtimeactivity.ObservationSnapshot, actualStatus string) strategyRuntimeObservation {
	observation := runtimecontrol.ObservationFromSnapshot(snapshot, actualStatus, strategyStatusStopped)
	return strategyRuntimeObservation{
		ActualStatus:      observation.ActualStatus,
		ActiveSymbols:     observation.ActiveSymbols,
		LastClosedKLineAt: observation.LastClosedKLineAt,
		LastSignalAt:      observation.LastSignalAt,
		LastOrderAt:       observation.LastOrderAt,
		LastErrorAt:       observation.LastErrorAt,
		LastError:         observation.LastError,
		UpdatedAt:         observation.UpdatedAt,
	}
}

func strategyRuntimeOptionalTime(value time.Time) *time.Time {
	return runtimecontrol.OptionalTime(value)
}

func strategyRuntimeBrokerReadQuery(binding strategyInstanceBinding) broker.ReadQuery {
	query := broker.ReadQuery{}
	if binding.BrokerAccount == nil {
		return query
	}
	query.AccountID = strings.TrimSpace(binding.BrokerAccount.AccountID)
	query.TradingEnvironment = strings.TrimSpace(binding.BrokerAccount.TradingEnvironment)
	query.Market = strings.TrimSpace(binding.BrokerAccount.Market)
	return query
}

func strategyRuntimeBrokerPlaceOrderQuery(binding strategyInstanceBinding, symbol string) broker.PlaceOrderQuery {
	readQuery := strategyRuntimeBrokerReadQuery(binding)
	if strings.TrimSpace(readQuery.Market) == "" {
		readQuery.Market = strategyRuntimeMarketFromSymbol(symbol, "")
	}
	return broker.PlaceOrderQuery{
		ReadQuery: readQuery,
		Symbol:    symbol,
	}
}

func strategyRuntimeBrokerID(binding strategyInstanceBinding) string {
	if binding.BrokerAccount == nil || strings.TrimSpace(binding.BrokerAccount.BrokerID) == "" {
		return "futu"
	}
	return strings.ToLower(strings.TrimSpace(binding.BrokerAccount.BrokerID))
}

func strategyRuntimeDefinitionID(instance managedStrategyInstance) string {
	definitionID := jftradeOptionalTypeAssertion[string](instance.Params["definitionId"])
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
	return runtimecontrol.FormatNumber(value)
}

func strategyRuntimeIntervalDuration(interval bbgotypes.Interval) (duration time.Duration, ok bool) {
	defer func() {
		if recover() != nil {
			duration = 0
			ok = false
		}
	}()
	duration = interval.Duration()
	return duration, duration > 0
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

func cloneStrategyRuntimeFundsSnapshot(snapshot *broker.FundsSnapshot) *broker.FundsSnapshot {
	if snapshot == nil {
		return nil
	}
	copyValue := *snapshot
	copyValue.CurrencyBalances = append([]broker.CurrencyBalanceSnapshot(nil), snapshot.CurrencyBalances...)
	return &copyValue
}

func cloneStrategyRuntimePositions(positions []broker.PositionSnapshot) []broker.PositionSnapshot {
	if len(positions) == 0 {
		return nil
	}
	return append([]broker.PositionSnapshot(nil), positions...)
}

func strategyRuntimePositionToControl(position broker.PositionSnapshot) runtimecontrol.Position {
	return runtimecontrol.Position{
		Market:           position.Market,
		Symbol:           position.Symbol,
		Quantity:         position.Quantity,
		SellableQuantity: position.SellableQuantity,
	}
}

func strategyRuntimePositionsToControl(positions []broker.PositionSnapshot) []runtimecontrol.Position {
	if len(positions) == 0 {
		return nil
	}
	result := make([]runtimecontrol.Position, 0, len(positions))
	for _, position := range positions {
		result = append(result, strategyRuntimePositionToControl(position))
	}
	return result
}

func strategyRuntimePositionMatchesSymbol(position broker.PositionSnapshot, symbol string) bool {
	return runtimecontrol.PositionMatchesSymbol(strategyRuntimePositionToControl(position), symbol)
}

func buildStrategyRuntimeAccount(funds *broker.FundsSnapshot, positions []broker.PositionSnapshot, market bbgotypes.Market, symbol string) *bbgotypes.Account {
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
			if !strategyRuntimePositionMatchesSymbol(position, symbol) {
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

func strategyRuntimeMarketFromSymbol(symbol string, defaultMarket string) string {
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
	return strings.ToUpper(strings.TrimSpace(defaultMarket))
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

func strategyRuntimeSortedSymbols(symbols map[string]*strategySymbolRuntime) []string {
	result := make([]string, 0, len(symbols))
	for symbol := range symbols {
		result = append(result, symbol)
	}
	sort.Strings(result)
	return result
}

func strategyRuntimeMaxTime(left time.Time, right time.Time) time.Time {
	return runtimecontrol.MaxTime(left, right)
}

func jftradeLogError(values ...any) {
	for _, value := range values {
		if err, ok := value.(error); ok && err != nil {
			log.Printf("best-effort operation failed: %v", err)
		}
	}
}
