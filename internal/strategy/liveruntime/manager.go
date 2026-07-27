package liveruntime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
	"github.com/jftrade/jftrade-main/pkg/besteffort"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/instancebinding"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

var (
	strategyRuntimeClosedKLineSyncLimit = 8

	// ErrClosed is returned when a start races with, or follows, manager close.
	ErrClosed = errors.New("strategy live runtime manager is closed")
)

const defaultClosedKLineSyncInterval = 5 * time.Second

const (
	strategyStatusRunning = "RUNNING"
	strategyStatusPaused  = "PAUSED"
	strategyStatusStopped = "STOPPED"
)

type Exchange interface {
	bbgotypes.Exchange
	QueryBrokerFunds(ctx context.Context, query broker.ReadQuery) (*broker.FundsSnapshot, error)
	QueryBrokerPositions(ctx context.Context, query broker.ReadQuery) ([]broker.PositionSnapshot, error)
	PlaceBrokerOrder(ctx context.Context, query broker.PlaceOrderQuery) (*broker.PlaceOrderResult, error)
	CancelBrokerOrder(ctx context.Context, query broker.ReadQuery, order broker.CancelOrder) error
}

type marketEnsurer interface {
	EnsureMarket(symbol string)
}

type Manager struct {
	exchangeProvider func() Exchange
	pineWorkerRunner PineWorker
	deps             Dependencies

	exchangeMu   sync.RWMutex
	pineWorkerMu sync.RWMutex
	mu           sync.RWMutex
	runtimes     map[string]*managedRuntime
	starting     map[string]struct{}
	closed       bool
	startWG      sync.WaitGroup

	closeOnce sync.Once
	closeErr  error

	closeErrorsMu sync.Mutex
	closeErrors   []error

	closedKLineSyncInterval time.Duration
}

var _ stratsrv.RuntimeManager = (*Manager)(nil)

// SubscriptionLease is the consumer-owned lifetime handle returned by the
// market-data application adapter.
type SubscriptionLease interface {
	Release()
}

// Dependencies contains the narrow application ports used by the live
// strategy runtime. It intentionally exposes no server, store, or broker
// implementation.
type Dependencies struct {
	ExchangeProvider        func() Exchange
	PineWorker              PineWorker
	PineWorkerLimit         func() int
	WakeMarketDataCollector func()
	CurrentInstance         func(instanceID string) (stratsrv.ManagedInstance, bool)
	AppendRuntimeEvent      func(instanceID string, logMessage string, kind string, detail string) error
	TransitionInstance      func(instanceID string, nextStatus string, kind string, detail string) error
	ReconcileRuntimeFailure func(instanceID string, detail string) error
	RecordNotification      func(Notification)
	PlaceExecutionOrder     func(context.Context, trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error)
	CancelExecutionOrder    func(context.Context, string) (trdsrv.ExecutionOrder, error)
	CountRuntimeAudit       func(context.Context, runtimeactivity.AuditQuery) (int, error)
	UpsertObservation       func(context.Context, runtimeactivity.ObservationSnapshot) error
	AcquireMarketDataLease  func(context.Context, string, []mdsrv.InstrumentRef) (SubscriptionLease, error)
}

// NewManager creates the process-owned live strategy runtime manager.
func NewManager(deps Dependencies) *Manager {
	return &Manager{
		exchangeProvider:        deps.ExchangeProvider,
		pineWorkerRunner:        deps.PineWorker,
		deps:                    deps,
		runtimes:                map[string]*managedRuntime{},
		starting:                map[string]struct{}{},
		closedKLineSyncInterval: defaultClosedKLineSyncInterval,
	}
}

// SetExchangeProvider atomically replaces the application-owned integration
// bridge used by future starts.
func (m *Manager) SetExchangeProvider(provider func() Exchange) {
	if m == nil {
		return
	}
	m.exchangeMu.Lock()
	m.exchangeProvider = provider
	m.exchangeMu.Unlock()
}

// SetPineWorkerRunner atomically replaces the runner used by future starts.
func (m *Manager) SetPineWorkerRunner(runner PineWorker) {
	if m == nil {
		return
	}
	m.pineWorkerMu.Lock()
	m.pineWorkerRunner = runner
	m.pineWorkerMu.Unlock()
}

// SetClosedKLineSyncInterval configures the fallback polling cadence used by
// symbol runtimes started after this call. Non-positive values disable polling.
func (m *Manager) SetClosedKLineSyncInterval(interval time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.closedKLineSyncInterval = interval
	m.mu.Unlock()
}

func (m *Manager) currentClosedKLineSyncInterval() time.Duration {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.closedKLineSyncInterval
}

func (m *Manager) currentPineWorkerRunner() PineWorker {
	if m == nil {
		return nil
	}
	m.pineWorkerMu.RLock()
	defer m.pineWorkerMu.RUnlock()
	return m.pineWorkerRunner
}

type Notification struct {
	At       string
	Level    string
	Title    string
	Message  string
	Source   string
	BrokerID string
	Category string
}

type managedRuntime struct {
	instanceID        string
	definition        stratsrv.DefinitionSummary
	cancel            context.CancelFunc
	symbols           map[string]*symbolRuntime
	mu                sync.RWMutex
	lastClosedKLineAt time.Time
	lastSignalAt      time.Time
	lastOrderAt       time.Time
	lastErrorAt       time.Time
	lastError         string
	updatedAt         time.Time
	subscriptionLease SubscriptionLease

	closeOnce sync.Once
	closeErr  error
}

type symbolRuntime struct {
	instanceID              string
	name                    string
	symbol                  string
	interval                bbgotypes.Interval
	exchange                bbgotypes.ExchangeName
	ctx                     context.Context
	runtimeExchange         Exchange
	brokerQuery             broker.ReadQuery
	market                  bbgotypes.Market
	accountRefreshMu        sync.Mutex
	accountMu               sync.RWMutex
	cachedFunds             *broker.FundsSnapshot
	cachedPositions         []broker.PositionSnapshot
	session                 *bbgo.ExchangeSession
	emitter                 bbgotypes.StandardStreamEmitter
	pineWorkerLive          *pineWorkerLive
	onClosedKLine           func(time.Time)
	onError                 func(string)
	closedKLineSyncInterval time.Duration

	mu              sync.RWMutex
	currentBucket   *bbgotypes.KLine
	lastClosedPrice float64
	lastClosedKLine time.Time
}

type strategyNotifyOnlyOrderExecutor struct {
	manager  *Manager
	instance stratsrv.ManagedInstance
	runner   *symbolRuntime
}

type strategyLiveOrderExecutor struct {
	manager  *Manager
	instance stratsrv.ManagedInstance
	runner   *symbolRuntime

	mu                      sync.Mutex
	trackedInternalOrderIDs map[string]string
}

func (m *Manager) pineWorkerLimit() int {
	const defaultInstanceWorkerLimit = 10
	limit := defaultInstanceWorkerLimit
	if m != nil && m.deps.PineWorkerLimit != nil {
		if configured := m.deps.PineWorkerLimit(); configured > 0 {
			limit = configured
		}
	}
	return limit
}

func (m *Manager) wakeMarketDataCollector() {
	if m != nil && m.deps.WakeMarketDataCollector != nil {
		m.deps.WakeMarketDataCollector()
	}
}

func (m *Manager) currentInstance(instanceID string) (stratsrv.ManagedInstance, bool) {
	if m == nil || m.deps.CurrentInstance == nil {
		return stratsrv.ManagedInstance{}, false
	}
	return m.deps.CurrentInstance(instanceID)
}

func (m *Manager) appendRuntimeEvent(instanceID string, logMessage string, kind string, detail string) error {
	if m == nil || m.deps.AppendRuntimeEvent == nil {
		return nil
	}
	return m.deps.AppendRuntimeEvent(instanceID, logMessage, kind, detail)
}

func (m *Manager) transitionInstance(instanceID string, nextStatus string, kind string, detail string) error {
	if m == nil || m.deps.TransitionInstance == nil {
		return nil
	}
	return m.deps.TransitionInstance(instanceID, nextStatus, kind, detail)
}

func (m *Manager) reconcileRuntimeFailure(instanceID string, detail string) error {
	if m == nil || m.deps.ReconcileRuntimeFailure == nil {
		return nil
	}
	return m.deps.ReconcileRuntimeFailure(instanceID, detail)
}

func (m *Manager) recordNotification(note Notification) {
	if m != nil && m.deps.RecordNotification != nil {
		m.deps.RecordNotification(note)
	}
}

func (m *Manager) placeExecutionOrder(ctx context.Context, command trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error) {
	if m == nil || m.deps.PlaceExecutionOrder == nil {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("strategy runtime order placement is unavailable")
	}
	return m.deps.PlaceExecutionOrder(ctx, command)
}

func (m *Manager) cancelExecutionOrder(ctx context.Context, internalOrderID string) (trdsrv.ExecutionOrder, error) {
	if m == nil || m.deps.CancelExecutionOrder == nil {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("strategy runtime order cancellation is unavailable")
	}
	return m.deps.CancelExecutionOrder(ctx, internalOrderID)
}

// CurrentExchange returns the broker-neutral runtime exchange assembled by the
// application. Callers must treat nil as an unavailable broker runtime.
func (m *Manager) CurrentExchange() Exchange {
	if m == nil {
		return nil
	}
	m.exchangeMu.RLock()
	provider := m.exchangeProvider
	m.exchangeMu.RUnlock()
	if provider == nil {
		return nil
	}
	return provider()
}

// ActiveInstrumentIDs implements strategy.RuntimeManager.
func (m *Manager) ActiveInstrumentIDs() []string {
	return m.activeInstrumentIDs()
}

func (m *Manager) activeInstrumentIDs() []string {
	if m == nil {
		return nil
	}
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

// MaintenanceBusyReason keeps maintenance inspection behind the runtime owner.
func (m *Manager) MaintenanceBusyReason(context.Context) string {
	if m == nil {
		return ""
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.runtimes) > 0 || len(m.starting) > 0 {
		return "存在活动策略实例"
	}
	return ""
}

// Start implements strategy.RuntimeManager and keeps protocol-facing error
// classification at the strategy domain boundary.
func (m *Manager) Start(ctx context.Context, instance stratsrv.ManagedInstance) error {
	err := m.startStrategy(ctx, instance)
	if err == nil || errors.Is(err, pineworker.ErrCapacityExceeded) {
		return err
	}
	if errors.Is(err, ErrClosed) {
		return stratsrv.BusyError(err.Error())
	}
	status, _ := strategyRuntimeStartError(err)
	if status == 400 {
		return stratsrv.BadRequestError(err.Error())
	}
	return stratsrv.UpstreamError(err.Error())
}

func (m *Manager) startStrategy(ctx context.Context, instance stratsrv.ManagedInstance) error {
	if m == nil {
		return fmt.Errorf("strategy runtime manager is unavailable")
	}
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

func validateStrategyRuntimeInstance(instance stratsrv.ManagedInstance) (bbgotypes.Interval, string, error) {
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

func (m *Manager) ensureStrategyStopped(instanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	if _, exists := m.runtimes[instanceID]; exists {
		return fmt.Errorf("strategy instance is already running")
	}
	return nil
}

func (m *Manager) loadStrategyRuntimeInputs(ctx context.Context, instance stratsrv.ManagedInstance) (Exchange, map[string]bbgotypes.Market, *broker.FundsSnapshot, []broker.PositionSnapshot, error) {
	exchange := m.CurrentExchange()
	if exchange == nil {
		return nil, nil, nil, nil, fmt.Errorf("strategy runtime exchange is unavailable")
	}
	if marketEnsurer, ok := exchange.(marketEnsurer); ok {
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

func (m *Manager) buildManagedStrategyRuntime(ctx context.Context, exchange Exchange, markets map[string]bbgotypes.Market, funds *broker.FundsSnapshot, positions []broker.PositionSnapshot, instance stratsrv.ManagedInstance, script string, interval bbgotypes.Interval) (*managedRuntime, error) {
	runtimeCtx, cancel := context.WithCancel(context.Background())
	managed := &managedRuntime{
		instanceID: instance.ID,
		definition: instance.Definition,
		cancel:     cancel,
		symbols:    make(map[string]*symbolRuntime, len(instance.Binding.Symbols)),
		updatedAt:  time.Now().UTC(),
	}
	if m.deps.AcquireMarketDataLease != nil {
		lease, err := m.deps.AcquireMarketDataLease(ctx, "strategy-runtime:"+instance.ID, strategyKLineSubscriptionRefs(instance.Binding.Symbols, interval))
		if err != nil {
			cancel()
			return nil, fmt.Errorf("acquire strategy market-data subscriptions: %w", err)
		}
		managed.subscriptionLease = lease
	}
	for _, symbol := range instance.Binding.Symbols {
		runner, err := m.buildSymbolRuntime(ctx, runtimeCtx, exchange, markets, funds, positions, instance, script, symbol, interval)
		if err != nil {
			closeErr := managed.close(context.Background())
			return nil, errors.Join(err, closeErr)
		}
		managed.symbols[symbol] = runner
	}
	return managed, nil
}

func (m *Manager) activateStrategyRuntime(instanceID string, managed *managedRuntime) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		closeErr := managed.close(context.Background())
		m.recordCloseError(closeErr)
		return errors.Join(ErrClosed, closeErr)
	}
	if _, exists := m.runtimes[instanceID]; exists {
		m.mu.Unlock()
		closeErr := managed.close(context.Background())
		return errors.Join(fmt.Errorf("strategy instance is already running"), closeErr)
	}
	m.runtimes[instanceID] = managed
	m.mu.Unlock()
	m.persistObservationSnapshot(managed.snapshot(strategyStatusRunning))
	for _, runner := range managed.symbols {
		go runner.syncClosedKLinesLoop()
	}
	m.wakeMarketDataCollector()
	return nil
}

func (m *Manager) reserveRuntimeStart(instanceID string) (func(), error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrClosed
	}
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
	m.startWG.Add(1)
	var releaseOnce sync.Once
	return func() {
		releaseOnce.Do(func() {
			m.mu.Lock()
			delete(m.starting, instanceID)
			m.mu.Unlock()
			m.startWG.Done()
		})
	}, nil
}

// Stop implements strategy.RuntimeManager.
func (m *Manager) Stop(instanceID string) {
	m.stopStrategy(instanceID)
}

func (m *Manager) stopStrategy(instanceID string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	runtime, exists := m.runtimes[instanceID]
	if exists {
		delete(m.runtimes, instanceID)
	}
	m.mu.Unlock()
	if exists {
		m.persistObservationSnapshot(runtime.snapshot(strategyStatusStopped))
	}
	if exists {
		if err := runtime.close(context.Background()); err != nil {
			besteffort.LogError(err)
		}
	}
	if exists {
		m.wakeMarketDataCollector()
	}
}

// Close stops all active and in-flight starts exactly once. Every Pine session
// close failure is returned with its instance and symbol name.
func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		runtimes := make([]*managedRuntime, 0, len(m.runtimes))
		for instanceID, runtime := range m.runtimes {
			delete(m.runtimes, instanceID)
			runtimes = append(runtimes, runtime)
		}
		m.mu.Unlock()

		closeErrors := make([]error, 0)
		for _, runtime := range runtimes {
			m.persistObservationSnapshot(runtime.snapshot(strategyStatusStopped))
			closeErrors = append(closeErrors, runtime.close(context.Background()))
		}
		m.startWG.Wait()
		closeErrors = append(closeErrors, m.takeCloseErrors()...)
		m.closeErr = errors.Join(closeErrors...)
		if len(runtimes) > 0 {
			m.wakeMarketDataCollector()
		}
	})
	return m.closeErr
}

func (m *Manager) recordCloseError(err error) {
	if err == nil {
		return
	}
	m.closeErrorsMu.Lock()
	m.closeErrors = append(m.closeErrors, err)
	m.closeErrorsMu.Unlock()
}

func (m *Manager) takeCloseErrors() []error {
	m.closeErrorsMu.Lock()
	defer m.closeErrorsMu.Unlock()
	result := append([]error(nil), m.closeErrors...)
	m.closeErrors = nil
	return result
}

func (runtime *managedRuntime) close(ctx context.Context) error {
	if runtime == nil {
		return nil
	}
	runtime.closeOnce.Do(func() {
		if runtime.cancel != nil {
			runtime.cancel()
		}
		symbols := strategyRuntimeSortedSymbols(runtime.symbols)
		closeErrors := make([]error, 0, len(symbols))
		for _, symbol := range symbols {
			runner := runtime.symbols[symbol]
			if runner == nil || runner.pineWorkerLive == nil {
				continue
			}
			if err := runner.pineWorkerLive.closeSession(ctx); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf(
					"strategy runtime %s symbol %s pine session close: %w",
					runtime.instanceID,
					symbol,
					err,
				))
			}
		}
		if runtime.subscriptionLease != nil {
			runtime.subscriptionLease.Release()
		}
		runtime.closeErr = errors.Join(closeErrors...)
	})
	return runtime.closeErr
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

// HandleMarketTrade routes a broker-neutral trade to matching live symbols.
func (m *Manager) HandleMarketTrade(trade bbgotypes.Trade) {
	symbol := strings.ToUpper(strings.TrimSpace(trade.Symbol))
	if symbol == "" {
		return
	}

	m.mu.RLock()
	runners := make([]*symbolRuntime, 0, len(m.runtimes))
	for _, runtime := range m.runtimes {
		runner, exists := runtime.symbols[symbol]
		if !exists {
			continue
		}
		runners = append(runners, runner)
	}
	m.mu.RUnlock()

	for _, runner := range runners {
		func(runner *symbolRuntime) {
			defer func() {
				if recovered := recover(); recovered != nil {
					m.handleRuntimePanic(runner.instanceID, runner.symbol, recovered)
				}
			}()
			runner.handleTrade(trade)
		}(runner)
	}
}

func (m *Manager) buildSymbolRuntime(
	ctx context.Context,
	runtimeCtx context.Context,
	exchange Exchange,
	markets bbgotypes.MarketMap,
	funds *broker.FundsSnapshot,
	positions []broker.PositionSnapshot,
	instance stratsrv.ManagedInstance,
	script string,
	symbol string,
	interval bbgotypes.Interval,
) (*symbolRuntime, error) {
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

	runner := &symbolRuntime{
		instanceID:              instance.ID,
		name:                    strings.TrimSpace(instance.Definition.Name),
		symbol:                  symbol,
		interval:                interval,
		exchange:                exchange.Name(),
		ctx:                     runtimeCtx,
		runtimeExchange:         exchange,
		brokerQuery:             strategyRuntimeBrokerReadQuery(instance.Binding),
		market:                  market,
		cachedFunds:             cloneStrategyRuntimeFundsSnapshot(funds),
		cachedPositions:         cloneStrategyRuntimePositions(positions),
		session:                 session,
		emitter:                 emitter,
		closedKLineSyncInterval: m.currentClosedKLineSyncInterval(),
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
			besteffort.LogError(jftradeErr2)
		},
	}

	recordIgnoredOrder := func(message string) {
		jftradeErr := m.appendRuntimeEvent(
			instance.ID,
			fmt.Sprintf("live order ignored %s", symbol),
			"order_ignored",
			message,
		)
		besteffort.LogError(jftradeErr)
	}
	live, err := newStrategyRuntimePineWorkerLive(m.currentPineWorkerRunner(), instance, symbol, interval, script, m.newOrderExecutor(instance, runner), runner, recordIgnoredOrder)
	if err != nil {
		return nil, fmt.Errorf("start strategy runtime for %s: %w", symbol, err)
	}
	runner.pineWorkerLive = live
	if err := m.seedSymbolRuntime(ctx, exchange, live, runner); err != nil {
		return nil, err
	}
	if err := live.openSession(runtimeCtx); err != nil {
		return nil, err
	}
	return runner, nil
}

func (m *Manager) seedSymbolRuntime(ctx context.Context, exchange Exchange, live *pineWorkerLive, runner *symbolRuntime) error {
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

func (m *Manager) newOrderExecutor(instance stratsrv.ManagedInstance, runner *symbolRuntime) bbgo.OrderExecutor {
	if instance.Binding.ExecutionMode == instancebinding.ExecutionModeNotifyOnly {
		return &strategyNotifyOnlyOrderExecutor{manager: m, instance: instance, runner: runner}
	}
	return &strategyLiveOrderExecutor{manager: m, instance: instance, runner: runner}
}
