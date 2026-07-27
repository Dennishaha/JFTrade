// Package runtimes owns application runtime references and their shutdown
// order. Runtime state is grouped by lifecycle rather than flattened into the
// API server composition root.
package runtimes

import (
	"errors"
	"fmt"
	"reflect"
	"sync"

	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	appcomposition "github.com/jftrade/jftrade-main/internal/app/apiserver/application"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/futuapp"
	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	"github.com/jftrade/jftrade-main/internal/live"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
	"github.com/jftrade/jftrade-main/internal/strategy/pineruntime"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
)

type startupGroup struct {
	liveNotifications    *live.ReplayPublisher
	liveNotificationSink func(live.Event) live.NotificationDelivery
	brokers              *broker.Registry
	exchangeCalendars    *exchangecalendar.Manager
	previousResolver     marketcalendar.Resolver
	realTradeControl     *trdsrv.RealTradeControlPlane
	preTradeRisk         trdsrv.PreTradeRiskGateway
}

type lazyGroup struct {
	liveWebSocket              *apilive.Handler
	strategyRuntime            *liveruntime.Manager
	strategyRuntimeMaintenance dmsrv.BusyChecker
	strategyPineRunnerSink     pineRunnerSink
	assistant                  assistantassembly.Runtime
}

type resettableGroup struct {
	marketData      *futuintegration.MarketDataRuntime
	futuCoordinator *futuapp.Coordinator
	pineWorker      *pineruntime.Manager
	backtestRunner  pineruntime.Runner
	instanceRunner  pineruntime.Runner
	pineClosed      bool
}

type pineRunnerSink interface {
	SetPineWorkerRunner(liveruntime.PineWorker)
}

// Handle is the application-owned runtime aggregate. Setters register
// successful runtime construction in startup order; Close releases them in
// reverse order and is safe to call repeatedly.
type Handle struct {
	mu         sync.RWMutex
	startup    startupGroup
	lazy       lazyGroup
	resettable resettableGroup

	providers     appcomposition.Resources
	consumers     appcomposition.Resources
	setupErrMu    sync.Mutex
	setupErr      error
	pineCloseOnce sync.Once
	closed        bool
}

func (h *Handle) SetLiveNotifications(
	publisher *live.ReplayPublisher,
	sink func(live.Event) live.NotificationDelivery,
) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		if publisher != nil {
			h.rejectClosed("live notification publisher", publisher.Close)
		}
		return
	}
	if publisher != nil && !h.registerLocked(&h.providers, "live notification publisher", publisher.Close) {
		h.mu.Unlock()
		return
	}
	h.startup.liveNotifications = publisher
	h.startup.liveNotificationSink = sink
	h.mu.Unlock()
}

func (h *Handle) LiveNotifications() *live.ReplayPublisher {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.startup.liveNotifications
}

func (h *Handle) LiveNotificationSink() func(live.Event) live.NotificationDelivery {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.startup.liveNotificationSink
}

func (h *Handle) SetLiveNotificationSink(
	sink func(live.Event) live.NotificationDelivery,
) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.startup.liveNotificationSink = sink
	h.mu.Unlock()
}

func (h *Handle) SetBrokerRegistry(registry *broker.Registry) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.startup.brokers = registry
	h.mu.Unlock()
}

func (h *Handle) Brokers() *broker.Registry {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.startup.brokers
}

func (h *Handle) SetExchangeCalendars(
	manager *exchangecalendar.Manager,
	previous marketcalendar.Resolver,
) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		h.rejectClosed("exchange calendars", func() error {
			if previous != nil {
				marketpkg.SetCalendarResolver(previous)
			} else {
				marketpkg.ResetCalendarResolver()
			}
			if manager != nil {
				return manager.Close()
			}
			return nil
		})
		return
	}
	if manager != nil && !h.registerLocked(&h.providers, "exchange calendar manager", manager.Close) {
		h.mu.Unlock()
		return
	}
	if !h.registerLocked(&h.providers, "exchange calendar resolver", func() error {
		if previous != nil {
			marketpkg.SetCalendarResolver(previous)
		} else {
			marketpkg.ResetCalendarResolver()
		}
		return nil
	}) {
		h.mu.Unlock()
		return
	}
	h.startup.exchangeCalendars = manager
	h.startup.previousResolver = previous
	h.mu.Unlock()
}

func (h *Handle) ExchangeCalendars() *exchangecalendar.Manager {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.startup.exchangeCalendars
}

func (h *Handle) SetRealTradeControl(
	control *trdsrv.RealTradeControlPlane,
	risk trdsrv.PreTradeRiskGateway,
) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.startup.realTradeControl = control
	h.startup.preTradeRisk = risk
	h.mu.Unlock()
}

func (h *Handle) RealTradeControl() *trdsrv.RealTradeControlPlane {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.startup.realTradeControl
}

func (h *Handle) PreTradeRisk() trdsrv.PreTradeRiskGateway {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.startup.preTradeRisk
}

func (h *Handle) SetLiveWebSocket(handler *apilive.Handler) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		if handler != nil {
			h.rejectClosed("live WebSocket", handler.Close)
		}
		return
	}
	if handler != nil && !h.registerLocked(&h.consumers, "live WebSocket", handler.Close) {
		h.mu.Unlock()
		return
	}
	h.lazy.liveWebSocket = handler
	h.mu.Unlock()
}

func (h *Handle) LiveWebSocket() *apilive.Handler {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lazy.liveWebSocket
}

func (h *Handle) SetStrategyRuntime(
	manager *liveruntime.Manager,
	maintenance dmsrv.BusyChecker,
) {
	h.setStrategyRuntime(manager, maintenance, manager)
}

func (h *Handle) setStrategyRuntime(
	manager *liveruntime.Manager,
	maintenance dmsrv.BusyChecker,
	pineSink pineRunnerSink,
) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		if manager != nil {
			h.rejectClosed("strategy runtime manager", manager.Close)
		}
		return
	}
	if manager != nil && !h.registerLocked(&h.consumers, "strategy runtime manager", manager.Close) {
		h.mu.Unlock()
		return
	}
	h.lazy.strategyRuntime = manager
	h.lazy.strategyRuntimeMaintenance = maintenance
	h.lazy.strategyPineRunnerSink = pineSink
	if pineSink != nil {
		pineSink.SetPineWorkerRunner(h.resettable.instanceRunner)
	}
	h.mu.Unlock()
}

func (h *Handle) StrategyRuntime() *liveruntime.Manager {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lazy.strategyRuntime
}

func (h *Handle) StrategyRuntimeMaintenance() dmsrv.BusyChecker {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lazy.strategyRuntimeMaintenance
}

func (h *Handle) SetAssistant(runtime assistantassembly.Runtime) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		if runtime != nil {
			h.rejectClosed("assistant runtime", runtime.Close)
		}
		return
	}
	if runtime != nil && !h.registerLocked(&h.consumers, "assistant runtime", runtime.Close) {
		h.mu.Unlock()
		return
	}
	h.lazy.assistant = runtime
	h.mu.Unlock()
}

func (h *Handle) Assistant() assistantassembly.Runtime {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lazy.assistant
}

func (h *Handle) SetMarketData(runtime *futuintegration.MarketDataRuntime) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		if runtime != nil {
			h.rejectClosed("Futu market data runtime", runtime.Close)
		}
		return
	}
	if runtime != nil && !h.registerLocked(&h.providers, "Futu market data runtime", runtime.Close) {
		h.mu.Unlock()
		return
	}
	h.resettable.marketData = runtime
	h.mu.Unlock()
}

func (h *Handle) MarketData() *futuintegration.MarketDataRuntime {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.resettable.marketData
}

func (h *Handle) SetFutuCoordinator(coordinator *futuapp.Coordinator) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.resettable.futuCoordinator = coordinator
	h.mu.Unlock()
}

func (h *Handle) FutuCoordinator() *futuapp.Coordinator {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.resettable.futuCoordinator
}

// EnsurePineWorker returns the managed Pine runtime, creating and registering
// it atomically. When adopting a manager after unmanaged runners were injected,
// those runners are returned to the caller for retirement after reconfigure.
func (h *Handle) EnsurePineWorker(
	create func() *pineruntime.Manager,
) (*pineruntime.Manager, pineruntime.Runner, pineruntime.Runner) {
	if h == nil {
		return nil, nil, nil
	}
	h.mu.Lock()
	if h.closed || h.resettable.pineClosed {
		h.mu.Unlock()
		return nil, nil, nil
	}
	manager := h.resettable.pineWorker
	var unmanagedBacktest pineruntime.Runner
	var unmanagedInstance pineruntime.Runner
	if manager == nil && create != nil {
		manager = create()
		if manager != nil {
			if !h.registerPineCloseLocked() {
				h.mu.Unlock()
				return nil, nil, nil
			}
			h.resettable.pineWorker = manager
			unmanagedBacktest = h.resettable.backtestRunner
			unmanagedInstance = h.resettable.instanceRunner
		}
	}
	h.mu.Unlock()
	return manager, unmanagedBacktest, unmanagedInstance
}

func (h *Handle) PineWorker() *pineruntime.Manager {
	if h == nil {
		return nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.resettable.pineWorker
}

func (h *Handle) SetPineWorkerRunners(
	backtest pineruntime.Runner,
	instance pineruntime.Runner,
) {
	if h == nil {
		return
	}
	h.mu.Lock()
	if h.closed || h.resettable.pineClosed {
		h.mu.Unlock()
		h.addSetupError(errors.Join(
			appcomposition.ErrResourcesClosed,
			pineruntime.CloseRunners(backtest, instance),
		))
		return
	}
	if !h.registerPineCloseLocked() {
		h.mu.Unlock()
		return
	}
	previousBacktest := h.resettable.backtestRunner
	previousInstance := h.resettable.instanceRunner
	manager := h.resettable.pineWorker
	h.resettable.backtestRunner = backtest
	h.resettable.instanceRunner = instance
	if h.lazy.strategyPineRunnerSink != nil {
		h.lazy.strategyPineRunnerSink.SetPineWorkerRunner(instance)
	}
	h.mu.Unlock()
	if manager == nil {
		h.addSetupError(pineruntime.CloseRunners(
			replacedRunner(previousBacktest, backtest),
			replacedRunner(previousInstance, instance),
		))
	}
}

func (h *Handle) PineWorkerRunners() (pineruntime.Runner, pineruntime.Runner) {
	if h == nil {
		return nil, nil
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.resettable.backtestRunner, h.resettable.instanceRunner
}

func replacedRunner(previous pineruntime.Runner, current pineruntime.Runner) pineruntime.Runner {
	if sameRunner(previous, current) {
		return nil
	}
	return previous
}

func sameRunner(left pineruntime.Runner, right pineruntime.Runner) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftType := reflect.TypeOf(left)
	if leftType != reflect.TypeOf(right) || !leftType.Comparable() {
		return false
	}
	return reflect.ValueOf(left).Interface() == reflect.ValueOf(right).Interface()
}

func (h *Handle) registerPineCloseLocked() bool {
	registered := true
	h.pineCloseOnce.Do(func() {
		registered = h.registerLocked(&h.providers, "Pine worker runtime", h.closePineWorker)
	})
	return registered
}

func (h *Handle) closePineWorker() error {
	h.mu.Lock()
	if h.resettable.pineClosed {
		h.mu.Unlock()
		return nil
	}
	h.resettable.pineClosed = true
	manager := h.resettable.pineWorker
	backtest := h.resettable.backtestRunner
	instance := h.resettable.instanceRunner
	h.resettable.pineWorker = nil
	h.resettable.backtestRunner = nil
	h.resettable.instanceRunner = nil
	if h.lazy.strategyPineRunnerSink != nil {
		h.lazy.strategyPineRunnerSink.SetPineWorkerRunner(nil)
	}
	h.mu.Unlock()
	if manager != nil {
		return manager.Close()
	}
	return pineruntime.CloseRunners(backtest, instance)
}

func (h *Handle) register(name string, closeFn func() error) {
	if closeFn == nil {
		return
	}
	if err := h.providers.Register(name, closeFn); err != nil {
		h.addSetupError(err)
	}
}

func (h *Handle) registerConsumer(name string, closeFn func() error) {
	if closeFn == nil {
		return
	}
	if err := h.consumers.Register(name, closeFn); err != nil {
		h.addSetupError(err)
	}
}

func (h *Handle) registerLocked(resources *appcomposition.Resources, name string, closeFn func() error) bool {
	if closeFn == nil {
		return true
	}
	if err := resources.Register(name, closeFn); err != nil {
		h.addSetupError(err)
		return false
	}
	return true
}

func (h *Handle) rejectClosed(name string, closeFn func() error) {
	var closeErr error
	if closeFn != nil {
		if err := closeFn(); err != nil {
			closeErr = fmt.Errorf("close %s: %w", name, err)
		}
	}
	h.addSetupError(errors.Join(appcomposition.ErrResourcesClosed, closeErr))
}

func (h *Handle) addSetupError(err error) {
	if h == nil || err == nil {
		return
	}
	h.setupErrMu.Lock()
	h.setupErr = errors.Join(h.setupErr, err)
	h.setupErrMu.Unlock()
}

// SetupError returns runtime assembly and late-registration failures.
func (h *Handle) SetupError() error {
	if h == nil {
		return nil
	}
	h.setupErrMu.Lock()
	defer h.setupErrMu.Unlock()
	return h.setupErr
}

// CloseConsumers stops runtime consumers before the business services they use.
func (h *Handle) CloseConsumers() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
	return h.consumers.Close()
}

// CloseProviders stops runtime providers after dependent business services.
func (h *Handle) CloseProviders() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	h.closed = true
	h.mu.Unlock()
	return errors.Join(h.SetupError(), h.providers.Close())
}

// Close releases consumers before providers and is safe to call repeatedly.
func (h *Handle) Close() error {
	if h == nil {
		return nil
	}
	return errors.Join(h.CloseConsumers(), h.CloseProviders())
}
