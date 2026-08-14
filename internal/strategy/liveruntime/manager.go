package liveruntime

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/instancebinding"
	runtimeactivity "github.com/jftrade/jftrade-main/internal/strategy/runtimeactivity"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/bbgo/bbgo"
	bbgotypes "github.com/jftrade/jftrade-main/pkg/bbgo/types"
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

// MarketDataSource is the realtime market-data port consumed by strategy
// runtimes. It deliberately excludes account and order operations.
type MarketDataSource interface {
	bbgotypes.ExchangeMinimal
	bbgotypes.ExchangeMarketDataService
}

// AccountSource is the broker-account read port used only by live execution.
type AccountSource interface {
	QueryBrokerFunds(ctx context.Context, query broker.ReadQuery) (*broker.FundsSnapshot, error)
	QueryBrokerPositions(ctx context.Context, query broker.ReadQuery) ([]broker.PositionSnapshot, error)
}

// TradeCommandPort routes live strategy commands through the application
// trading boundary instead of exposing broker order methods to the runtime.
type TradeCommandPort interface {
	PlaceExecutionOrder(context.Context, trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error)
	CancelExecutionOrder(context.Context, string) (trdsrv.ExecutionOrder, error)
}

// TradeCommandFuncs adapts function-based composition and tests to the command
// port without adding broker operations to the market-data source.
type TradeCommandFuncs struct {
	Place  func(context.Context, trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error)
	Cancel func(context.Context, string) (trdsrv.ExecutionOrder, error)
}

func (commands TradeCommandFuncs) PlaceExecutionOrder(ctx context.Context, command trdsrv.ExecutionOrderCommand) (trdsrv.ExecutionOrder, error) {
	if commands.Place == nil {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("strategy runtime order placement is unavailable")
	}
	return commands.Place(ctx, command)
}

func (commands TradeCommandFuncs) CancelExecutionOrder(ctx context.Context, internalOrderID string) (trdsrv.ExecutionOrder, error) {
	if commands.Cancel == nil {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("strategy runtime order cancellation is unavailable")
	}
	return commands.Cancel(ctx, internalOrderID)
}

// Exchange retains the previous runtime override name for compatibility.
type Exchange = MarketDataSource

type marketEnsurer interface {
	EnsureMarket(symbol string)
}

type Manager struct {
	marketDataProvider func() MarketDataSource
	accountResolver    func(stratsrv.InstanceBinding) AccountSource
	pineWorkerRunner   PineWorker
	deps               Dependencies

	exchangeMu   sync.RWMutex
	pineWorkerMu sync.RWMutex
	mu           sync.RWMutex
	runtimes     map[string]*managedRuntime
	starting     map[string]struct{}
	closed       bool
	// marketDataHealthOverridden is set when an explicit exchange override is
	// installed. Test/embedded callers use that bridge to supply a complete
	// market-data fixture whose health is owned by the override rather than the
	// process-level provider status callback.
	marketDataHealthOverridden bool
	startWG                    sync.WaitGroup

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
	MarketDataProvider func() MarketDataSource
	AccountResolver    func(stratsrv.InstanceBinding) AccountSource
	TradeCommands      TradeCommandPort
	// ExchangeProvider and ExchangeResolver are compatibility inputs for
	// tests and embedders that have not yet split market data from accounts.
	ExchangeProvider        func() Exchange
	ExchangeResolver        func(stratsrv.InstanceBinding) Exchange
	MarketDataCapabilities  func(context.Context) (mdsrv.ProviderCapabilities, error)
	MarketDataHealth        func(context.Context) (mdsrv.HealthStatus, error)
	PineWorker              PineWorker
	PineWorkerLimit         func() int
	WakeMarketDataCollector func()
	CurrentInstance         func(instanceID string) (stratsrv.ManagedInstance, bool)
	AppendRuntimeEvent      func(instanceID string, logMessage string, kind string, detail string) error
	TransitionInstance      func(instanceID string, nextStatus string, kind string, detail string) error
	ReconcileRuntimeFailure func(instanceID string, detail string) error
	RecordNotification      func(Notification)
	CountRuntimeAudit       func(context.Context, runtimeactivity.AuditQuery) (int, error)
	UpsertObservation       func(context.Context, runtimeactivity.ObservationSnapshot) error
	AcquireMarketDataLease  func(context.Context, string, []mdsrv.InstrumentRef) (SubscriptionLease, error)
}

// NewManager creates the process-owned live strategy runtime manager.
func NewManager(deps Dependencies) *Manager {
	marketDataProvider := deps.MarketDataProvider
	if marketDataProvider == nil {
		marketDataProvider = deps.ExchangeProvider
	}
	accountResolver := deps.AccountResolver
	if accountResolver == nil && deps.ExchangeResolver != nil {
		accountResolver = func(binding stratsrv.InstanceBinding) AccountSource {
			resolved := deps.ExchangeResolver(binding)
			account, _ := resolved.(AccountSource)
			return account
		}
	}
	if accountResolver == nil && deps.ExchangeProvider != nil {
		accountResolver = func(stratsrv.InstanceBinding) AccountSource {
			account, _ := deps.ExchangeProvider().(AccountSource)
			return account
		}
	}
	return &Manager{
		marketDataProvider:      marketDataProvider,
		accountResolver:         accountResolver,
		pineWorkerRunner:        deps.PineWorker,
		deps:                    deps,
		runtimes:                map[string]*managedRuntime{},
		starting:                map[string]struct{}{},
		closedKLineSyncInterval: defaultClosedKLineSyncInterval,
	}
}

// SetExchangeProvider atomically installs an explicit runtime bridge override
// used by future starts. Application composition uses ExchangeResolver so a
// strategy binding is still resolved by its exact broker ID.
func (m *Manager) SetExchangeProvider(provider func() Exchange) {
	if m == nil {
		return
	}
	m.exchangeMu.Lock()
	m.marketDataProvider = provider
	m.marketDataHealthOverridden = true
	m.accountResolver = func(stratsrv.InstanceBinding) AccountSource {
		if provider == nil {
			return nil
		}
		account, _ := provider().(AccountSource)
		return account
	}
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
	backgroundWG      sync.WaitGroup
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
	marketDataSource        MarketDataSource
	accountSource           AccountSource
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
	if m == nil {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("strategy runtime order placement is unavailable")
	}
	if m.deps.TradeCommands == nil {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("strategy runtime order placement is unavailable")
	}
	return m.deps.TradeCommands.PlaceExecutionOrder(ctx, command)
}

func (m *Manager) cancelExecutionOrder(ctx context.Context, internalOrderID string) (trdsrv.ExecutionOrder, error) {
	if m == nil {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("strategy runtime order cancellation is unavailable")
	}
	if m.deps.TradeCommands == nil {
		return trdsrv.ExecutionOrder{}, fmt.Errorf("strategy runtime order cancellation is unavailable")
	}
	return m.deps.TradeCommands.CancelExecutionOrder(ctx, internalOrderID)
}

// CurrentExchange returns the realtime market-data source assembled by the
// application. It is retained as a compatibility name for existing callers.
func (m *Manager) CurrentExchange() Exchange {
	if m == nil {
		return nil
	}
	m.exchangeMu.RLock()
	provider := m.marketDataProvider
	m.exchangeMu.RUnlock()
	if provider == nil {
		return nil
	}
	return provider()
}

func (m *Manager) resolveAccount(binding stratsrv.InstanceBinding) AccountSource {
	if m == nil {
		return nil
	}
	m.exchangeMu.RLock()
	resolver := m.accountResolver
	m.exchangeMu.RUnlock()
	if resolver == nil {
		return nil
	}
	return resolver(binding)
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
	if err := m.validateMarketDataCapabilities(ctx); err != nil {
		return err
	}
	releaseStartReservation, err := m.reserveRuntimeStart(instance.ID)
	if err != nil {
		return err
	}
	defer releaseStartReservation()
	marketData, account, markets, funds, positions, err := m.loadStrategyRuntimeInputs(ctx, instance)
	if err != nil {
		return err
	}
	managed, err := m.buildManagedStrategyRuntime(ctx, marketData, account, markets, funds, positions, instance, script, interval)
	if err != nil {
		return err
	}
	return m.activateStrategyRuntime(instance.ID, managed)
}

func (m *Manager) validateMarketDataCapabilities(ctx context.Context) error {
	if m == nil || m.deps.MarketDataCapabilities == nil {
		return nil
	}
	capabilities, err := m.deps.MarketDataCapabilities(ctx)
	if err != nil {
		return fmt.Errorf("load market-data capabilities: %w", err)
	}
	if !capabilities.StreamingCandles {
		return fmt.Errorf("active market-data provider does not support streaming candles")
	}
	m.exchangeMu.RLock()
	healthOverridden := m.marketDataHealthOverridden
	m.exchangeMu.RUnlock()
	if m.deps.MarketDataHealth != nil && !healthOverridden {
		health, err := m.deps.MarketDataHealth(ctx)
		if err != nil {
			return fmt.Errorf("load market-data health: %w", err)
		}
		if !health.Connected || health.Readiness == mdsrv.ProviderReadinessFailed {
			if health.LastError != "" {
				return fmt.Errorf("active market-data provider is unhealthy: %s", health.LastError)
			}
			return fmt.Errorf("active market-data provider is unhealthy")
		}
	}
	return nil
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

func (m *Manager) loadStrategyRuntimeInputs(ctx context.Context, instance stratsrv.ManagedInstance) (MarketDataSource, AccountSource, map[string]bbgotypes.Market, *broker.FundsSnapshot, []broker.PositionSnapshot, error) {
	marketData := m.CurrentExchange()
	if marketData == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("strategy realtime market-data source is unavailable")
	}
	if marketEnsurer, ok := marketData.(marketEnsurer); ok {
		for _, symbol := range instance.Binding.Symbols {
			marketEnsurer.EnsureMarket(symbol)
		}
	}
	markets, err := marketData.QueryMarkets(ctx)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("load strategy markets: %w", err)
	}
	if instancebinding.NormalizeExecutionMode(instance.Binding.ExecutionMode) == instancebinding.ExecutionModeNotifyOnly {
		return marketData, nil, markets, nil, nil, nil
	}
	if err := validateLiveBrokerBinding(instance.Binding); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	brokerID := strategyRuntimeBrokerID(instance.Binding)
	account := m.resolveAccount(instance.Binding)
	if account == nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("live strategy broker %q is unavailable or not tradable", brokerID)
	}
	brokerQuery := strategyRuntimeBrokerReadQuery(instance.Binding)
	funds, err := account.QueryBrokerFunds(ctx, brokerQuery)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("load strategy broker funds: %w", err)
	}
	positions, err := account.QueryBrokerPositions(ctx, brokerQuery)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("load strategy broker positions: %w", err)
	}
	return marketData, account, markets, funds, positions, nil
}

func (m *Manager) buildManagedStrategyRuntime(ctx context.Context, marketData MarketDataSource, account AccountSource, markets map[string]bbgotypes.Market, funds *broker.FundsSnapshot, positions []broker.PositionSnapshot, instance stratsrv.ManagedInstance, script string, interval bbgotypes.Interval) (*managedRuntime, error) {
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
		runner, err := m.buildSymbolRuntime(ctx, runtimeCtx, marketData, account, markets, funds, positions, instance, script, symbol, interval)
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
	managed.startBackgroundLoops()
	m.runtimes[instanceID] = managed
	m.mu.Unlock()
	m.persistObservationSnapshot(managed.snapshot(strategyStatusRunning))
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
	marketData MarketDataSource,
	account AccountSource,
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
	session, emitter, err := newStrategyRuntimeSession(marketData, markets, funds, positions, market, symbol)
	if err != nil {
		return nil, err
	}
	runner := m.newSymbolRuntime(runtimeCtx, marketData, account, instance, symbol, interval, market, session, emitter, funds, positions)
	live, err := newStrategyRuntimePineWorkerLive(
		m.currentPineWorkerRunner(), instance, symbol, interval, script,
		m.newOrderExecutor(instance, runner), runner,
		func(message string) { m.recordIgnoredOrder(instance.ID, symbol, message) },
	)
	if err != nil {
		return nil, fmt.Errorf("start strategy runtime for %s: %w", symbol, err)
	}
	runner.pineWorkerLive = live
	if err := m.seedSymbolRuntime(ctx, marketData, live, runner); err != nil {
		return nil, err
	}
	if err := live.openSession(runtimeCtx); err != nil {
		return nil, err
	}
	return runner, nil
}

func (m *Manager) seedSymbolRuntime(ctx context.Context, marketData MarketDataSource, live *pineWorkerLive, runner *symbolRuntime) error {
	klines, err := live.loadWarmup(ctx, marketData)
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
