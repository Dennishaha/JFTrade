package runtimes

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apilive "github.com/jftrade/jftrade-main/internal/api/live"
	appcomposition "github.com/jftrade/jftrade-main/internal/app/apiserver/application"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/futuapp"
	assistantassembly "github.com/jftrade/jftrade-main/internal/assistant/assembly"
	"github.com/jftrade/jftrade-main/internal/exchangecalendar"
	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	"github.com/jftrade/jftrade-main/internal/live"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/strategy/liveruntime"
	"github.com/jftrade/jftrade-main/internal/strategy/pineruntime"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/pkg/broker"
	marketpkg "github.com/jftrade/jftrade-main/pkg/market"
	marketcalendar "github.com/jftrade/jftrade-main/pkg/market/calendar"
	"github.com/jftrade/jftrade-main/pkg/strategy/pineworker"
)

type recordingRunner struct {
	name   string
	closed *[]string
	calls  atomic.Int32
}

type recordingAssistantRuntime struct {
	assistantassembly.Runtime
	closeCalls atomic.Int32
}

type fixedCalendarResolver struct {
	market string
}

func (r *fixedCalendarResolver) Template(market string) (marketcalendar.MarketTemplate, bool) {
	if market != r.market {
		return marketcalendar.MarketTemplate{}, false
	}
	return marketcalendar.MarketTemplate{MarketCode: market, Timezone: "UTC"}, true
}

func (r *fixedCalendarResolver) Schedule(
	market string,
	day time.Time,
) (marketcalendar.TradingDaySchedule, bool) {
	if market != r.market {
		return marketcalendar.TradingDaySchedule{}, false
	}
	return marketcalendar.TradingDaySchedule{MarketCode: market, Date: day}, true
}

func (r *recordingAssistantRuntime) Close() error {
	r.closeCalls.Add(1)
	return nil
}

func (r *recordingRunner) RunScript(
	context.Context,
	pineworker.RunScriptRequest,
) (pineworker.RunScriptResponse, error) {
	return pineworker.RunScriptResponse{}, nil
}

func (r *recordingRunner) Close(context.Context) error {
	r.calls.Add(1)
	if r.closed != nil {
		*r.closed = append(*r.closed, r.name)
	}
	return nil
}

type blockingPineSink struct {
	started chan struct{}
	release chan struct{}

	mu      sync.Mutex
	current liveruntime.PineWorker
	calls   int
}

func (s *blockingPineSink) SetPineWorkerRunner(runner liveruntime.PineWorker) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if call == 1 {
		close(s.started)
		<-s.release
	}
	s.mu.Lock()
	s.current = runner
	s.mu.Unlock()
}

func (s *blockingPineSink) Current() liveruntime.PineWorker {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func TestNilHandleSupportsOptionalRuntimeAssembly(t *testing.T) {
	var handle *Handle

	handle.SetLiveNotifications(nil, nil)
	handle.SetLiveNotificationSink(nil)
	handle.SetBrokerRegistry(nil)
	handle.SetExchangeCalendars(nil, nil)
	handle.SetRealTradeControl(nil, nil)
	handle.SetLiveWebSocket(nil)
	handle.SetStrategyRuntime(nil, nil)
	handle.SetAssistant(nil)
	handle.SetMarketData(nil)
	handle.SetFutuCoordinator(nil)
	handle.SetPineWorkerRunners(nil, nil)

	if handle.LiveNotifications() != nil ||
		handle.LiveNotificationSink() != nil ||
		handle.Brokers() != nil ||
		handle.ExchangeCalendars() != nil ||
		handle.RealTradeControl() != nil ||
		handle.PreTradeRisk() != nil ||
		handle.LiveWebSocket() != nil ||
		handle.StrategyRuntime() != nil ||
		handle.StrategyRuntimeMaintenance() != nil ||
		handle.Assistant() != nil ||
		handle.MarketData() != nil ||
		handle.FutuCoordinator() != nil ||
		handle.PineWorker() != nil {
		t.Fatal("nil handle exposed a runtime")
	}
	backtest, instance := handle.PineWorkerRunners()
	if backtest != nil || instance != nil {
		t.Fatalf("nil handle runners = (%v, %v)", backtest, instance)
	}
	manager, unmanagedBacktest, unmanagedInstance := handle.EnsurePineWorker(func() *pineruntime.Manager {
		t.Fatal("nil handle invoked Pine manager factory")
		return nil
	})
	if manager != nil || unmanagedBacktest != nil || unmanagedInstance != nil {
		t.Fatalf(
			"nil handle Pine manager = (%v, %v, %v)",
			manager,
			unmanagedBacktest,
			unmanagedInstance,
		)
	}
	if handle.SetupError() != nil || handle.Close() != nil {
		t.Fatal("nil handle lifecycle returned an error")
	}
}

func TestHandlePublishesRuntimeGroupsBeforeShutdown(t *testing.T) {
	var handle Handle
	originalResolver := marketpkg.CurrentCalendarResolver()
	t.Cleanup(func() { marketpkg.SetCalendarResolver(originalResolver) })

	publisher := live.NewReplayPublisher()
	var sinkCalls atomic.Int32
	sink := func(live.Event) live.NotificationDelivery {
		sinkCalls.Add(1)
		return live.NotificationDelivered("published")
	}
	registry := broker.NewRegistry()
	calendars := exchangecalendar.NewManager(nil, nil)
	previousResolver := marketpkg.SwapCalendarResolver(calendars)
	control, err := trdsrv.NewRealTradeControlPlane("")
	if err != nil {
		t.Fatalf("create real-trade control: %v", err)
	}
	risk := trdsrv.NewStaticPreTradeRiskGateway(func() trdsrv.PreTradeRiskConfig {
		return trdsrv.PreTradeRiskConfig{}
	})
	webSocket := apilive.NewHandler(nil, apilive.Options{})
	strategyRuntime := liveruntime.NewManager(liveruntime.Dependencies{})
	assistantRuntime := &recordingAssistantRuntime{}
	marketData := futuintegration.NewMarketDataRuntime(futuintegration.MarketDataRuntimeOptions{})
	coordinator := futuapp.New(futuapp.Options{})

	handle.SetLiveNotifications(publisher, nil)
	handle.SetLiveNotificationSink(sink)
	handle.SetBrokerRegistry(registry)
	handle.SetExchangeCalendars(calendars, previousResolver)
	handle.SetRealTradeControl(control, risk)
	handle.SetLiveWebSocket(webSocket)
	handle.SetStrategyRuntime(strategyRuntime, strategyRuntime)
	handle.SetAssistant(assistantRuntime)
	handle.SetMarketData(marketData)
	handle.SetFutuCoordinator(coordinator)
	pineManager, unmanagedBacktest, unmanagedInstance := handle.EnsurePineWorker(
		func() *pineruntime.Manager { return pineruntime.NewManager() },
	)
	backtestRunner, instanceRunner := handle.PineWorkerRunners()

	if handle.LiveNotifications() != publisher ||
		handle.Brokers() != registry ||
		handle.ExchangeCalendars() != calendars ||
		handle.RealTradeControl() != control ||
		handle.PreTradeRisk() != risk ||
		handle.LiveWebSocket() != webSocket ||
		handle.StrategyRuntime() != strategyRuntime ||
		handle.StrategyRuntimeMaintenance() != strategyRuntime ||
		handle.Assistant() != assistantRuntime ||
		handle.MarketData() != marketData ||
		handle.FutuCoordinator() != coordinator ||
		handle.PineWorker() != pineManager {
		t.Fatal("runtime aggregate did not preserve an injected runtime identity")
	}
	if unmanagedBacktest != nil || unmanagedInstance != nil {
		t.Fatalf("new managed Pine runtime adopted runners (%v, %v)", unmanagedBacktest, unmanagedInstance)
	}
	if backtestRunner != nil || instanceRunner != nil {
		t.Fatalf("new managed Pine runtime exposed runners (%v, %v)", backtestRunner, instanceRunner)
	}
	if delivery := handle.LiveNotificationSink()(live.Event{}); !delivery.Delivered {
		t.Fatalf("notification sink delivery = %+v, want delivered", delivery)
	}
	if sinkCalls.Load() != 1 {
		t.Fatalf("notification sink calls = %d, want 1", sinkCalls.Load())
	}
	if got := strategyRuntime.MaintenanceBusyReason(t.Context()); got != "" {
		t.Fatalf("idle strategy maintenance reason = %q, want empty", got)
	}
	if handle.SetupError() != nil {
		t.Fatalf("runtime setup error: %v", handle.SetupError())
	}

	if err := handle.Close(); err != nil {
		t.Fatalf("close runtime aggregate: %v", err)
	}
	if publisher.Publish(live.Notification{}) != nil {
		t.Fatal("live publisher accepted a notification after aggregate shutdown")
	}
	if assistantRuntime.closeCalls.Load() != 1 {
		t.Fatalf("Assistant close calls = %d, want 1", assistantRuntime.closeCalls.Load())
	}
	if got := marketpkg.CurrentCalendarResolver(); got != previousResolver {
		t.Fatalf("calendar resolver after shutdown = %T, want previous resolver %T", got, previousResolver)
	}
}

func TestHandleRejectsConcreteResourcesAfterShutdown(t *testing.T) {
	assertClosedError := func(t *testing.T, handle *Handle) {
		t.Helper()
		if !errors.Is(handle.SetupError(), appcomposition.ErrResourcesClosed) {
			t.Fatalf("setup error = %v, want resources closed", handle.SetupError())
		}
	}

	t.Run("live notifications", func(t *testing.T) {
		var handle Handle
		if err := handle.Close(); err != nil {
			t.Fatalf("initial close: %v", err)
		}
		publisher := live.NewReplayPublisher()
		handle.SetLiveNotifications(publisher, nil)
		if handle.LiveNotifications() != nil {
			t.Fatal("late live publisher was published")
		}
		if publisher.Publish(live.Notification{}) != nil {
			t.Fatal("late live publisher was not closed")
		}
		assertClosedError(t, &handle)
	})

	t.Run("WebSocket", func(t *testing.T) {
		var handle Handle
		if err := handle.Close(); err != nil {
			t.Fatalf("initial close: %v", err)
		}
		handle.SetLiveWebSocket(apilive.NewHandler(nil, apilive.Options{}))
		if handle.LiveWebSocket() != nil {
			t.Fatal("late WebSocket handler was published")
		}
		assertClosedError(t, &handle)
	})

	t.Run("strategy runtime", func(t *testing.T) {
		var handle Handle
		if err := handle.Close(); err != nil {
			t.Fatalf("initial close: %v", err)
		}
		manager := liveruntime.NewManager(liveruntime.Dependencies{})
		handle.SetStrategyRuntime(manager, manager)
		if handle.StrategyRuntime() != nil || handle.StrategyRuntimeMaintenance() != nil {
			t.Fatal("late strategy runtime was published")
		}
		startErr := manager.Start(t.Context(), stratsrv.ManagedInstance{
			ID: "late-runtime",
			Binding: stratsrv.InstanceBinding{
				Symbols:  []string{"AAPL"},
				Interval: "1m",
			},
			Params: map[string]any{"script": "close > open"},
		})
		if !errors.Is(startErr, stratsrv.ErrBusy) {
			t.Fatalf("late strategy runtime start error = %v, want busy after close", startErr)
		}
		assertClosedError(t, &handle)
	})

	t.Run("market data", func(t *testing.T) {
		var handle Handle
		if err := handle.Close(); err != nil {
			t.Fatalf("initial close: %v", err)
		}
		runtime := futuintegration.NewMarketDataRuntime(futuintegration.MarketDataRuntimeOptions{
			ConfigSource: func() futuintegration.MarketDataConfig {
				return futuintegration.MarketDataConfig{Enabled: true, Host: "127.0.0.1", APIPort: 11110}
			},
		})
		handle.SetMarketData(runtime)
		if handle.MarketData() != nil {
			t.Fatal("late market-data runtime was published")
		}
		if runtime.Ensure() != nil {
			t.Fatal("late market-data runtime was not closed before first exchange creation")
		}
		assertClosedError(t, &handle)
	})
}

func TestHandleRestoresCalendarResolverOnShutdown(t *testing.T) {
	originalResolver := marketpkg.CurrentCalendarResolver()
	t.Cleanup(func() { marketpkg.SetCalendarResolver(originalResolver) })
	previousResolver := &fixedCalendarResolver{market: "TEST"}
	marketpkg.SetCalendarResolver(previousResolver)
	manager := exchangecalendar.NewManager(nil, nil)
	if swapped := marketpkg.SwapCalendarResolver(manager); swapped != previousResolver {
		t.Fatalf("swapped resolver = %T, want previous resolver", swapped)
	}

	var handle Handle
	handle.SetExchangeCalendars(manager, previousResolver)
	if handle.ExchangeCalendars() != manager {
		t.Fatal("exchange calendar manager was not published")
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("close runtimes: %v", err)
	}
	if got := marketpkg.CurrentCalendarResolver(); got != previousResolver {
		t.Fatalf("resolver after shutdown = %T, want previous resolver", got)
	}
	if template, ok := marketpkg.CurrentCalendarResolver().Template("TEST"); !ok || template.MarketCode != "TEST" {
		t.Fatalf("restored resolver template = (%+v, %v), want TEST", template, ok)
	}
	if err := manager.Close(); err != nil {
		t.Fatalf("calendar manager repeated close: %v", err)
	}

	lateManager := exchangecalendar.NewManager(nil, nil)
	marketpkg.SetCalendarResolver(lateManager)
	handle.SetExchangeCalendars(lateManager, previousResolver)
	if got := marketpkg.CurrentCalendarResolver(); got != previousResolver {
		t.Fatalf("resolver after late calendar registration = %T, want previous resolver", got)
	}
	if !errors.Is(handle.SetupError(), appcomposition.ErrResourcesClosed) {
		t.Fatalf("late calendar setup error = %v, want resources closed", handle.SetupError())
	}
}

func TestHandleRefusesPublicationWhenResourceGroupAlreadyClosed(t *testing.T) {
	var handle Handle
	if err := handle.providers.Close(); err != nil {
		t.Fatalf("close provider group: %v", err)
	}
	if err := handle.consumers.Close(); err != nil {
		t.Fatalf("close consumer group: %v", err)
	}

	publisher := live.NewReplayPublisher()
	handle.SetLiveNotifications(publisher, nil)
	assistantRuntime := &recordingAssistantRuntime{}
	handle.SetAssistant(assistantRuntime)
	if handle.LiveNotifications() != nil || handle.Assistant() != nil {
		t.Fatal("resource was published after its lifecycle group closed")
	}
	if publisher.Publish(live.Notification{}) != nil {
		t.Fatal("provider group did not immediately close the rejected publisher")
	}
	if assistantRuntime.closeCalls.Load() != 1 {
		t.Fatalf("rejected Assistant close calls = %d, want 1", assistantRuntime.closeCalls.Load())
	}

	var providerCalls atomic.Int32
	var consumerCalls atomic.Int32
	handle.register("late provider", func() error {
		providerCalls.Add(1)
		return nil
	})
	handle.registerConsumer("late consumer", func() error {
		consumerCalls.Add(1)
		return nil
	})
	handle.register("ignored provider", nil)
	handle.registerConsumer("ignored consumer", nil)
	if providerCalls.Load() != 1 || consumerCalls.Load() != 1 {
		t.Fatalf(
			"late direct registration close calls = (%d provider, %d consumer), want one each",
			providerCalls.Load(),
			consumerCalls.Load(),
		)
	}
	if !errors.Is(handle.SetupError(), appcomposition.ErrResourcesClosed) {
		t.Fatalf("setup error = %v, want resources closed", handle.SetupError())
	}
}

func TestHandleConcurrentCloseIsIdempotentAndAggregatesErrors(t *testing.T) {
	consumerErr := errors.New("consumer close failed")
	providerErr := errors.New("provider close failed")
	var handle Handle
	var sequenceMu sync.Mutex
	var sequence []string
	var consumerCalls atomic.Int32
	var providerCalls atomic.Int32
	handle.register("provider", func() error {
		providerCalls.Add(1)
		sequenceMu.Lock()
		sequence = append(sequence, "provider")
		sequenceMu.Unlock()
		return providerErr
	})
	handle.registerConsumer("consumer", func() error {
		consumerCalls.Add(1)
		sequenceMu.Lock()
		sequence = append(sequence, "consumer")
		sequenceMu.Unlock()
		return consumerErr
	})

	const callers = 16
	start := make(chan struct{})
	errs := make(chan error, callers)
	var calls sync.WaitGroup
	calls.Add(callers)
	for range callers {
		go func() {
			defer calls.Done()
			<-start
			errs <- handle.Close()
		}()
	}
	close(start)
	calls.Wait()
	close(errs)

	for err := range errs {
		if !errors.Is(err, consumerErr) || !errors.Is(err, providerErr) {
			t.Fatalf("concurrent close error = %v, want both close failures", err)
		}
	}
	if consumerCalls.Load() != 1 || providerCalls.Load() != 1 {
		t.Fatalf(
			"close calls = (%d consumer, %d provider), want one each",
			consumerCalls.Load(),
			providerCalls.Load(),
		)
	}
	sequenceMu.Lock()
	defer sequenceMu.Unlock()
	if want := []string{"consumer", "provider"}; !reflect.DeepEqual(sequence, want) {
		t.Fatalf("close sequence = %v, want %v", sequence, want)
	}
}

func TestHandleClosesConsumersBeforeProvidersAndProvidersInReverseOrder(t *testing.T) {
	var handle Handle
	var closed []string
	handle.register("startup runtime", func() error {
		closed = append(closed, "startup runtime")
		return nil
	})
	backtest := &recordingRunner{name: "backtest runner", closed: &closed}
	instance := &recordingRunner{name: "instance runner", closed: &closed}
	handle.SetPineWorkerRunners(backtest, instance)
	handle.registerConsumer("lazy runtime", func() error {
		closed = append(closed, "lazy runtime")
		return nil
	})

	for range 2 {
		if err := handle.Close(); err != nil {
			t.Fatalf("close runtimes: %v", err)
		}
	}
	want := []string{
		"lazy runtime",
		"backtest runner",
		"instance runner",
		"startup runtime",
	}
	if !reflect.DeepEqual(closed, want) {
		t.Fatalf("close order = %v, want %v", closed, want)
	}
	if backtest.calls.Load() != 1 || instance.calls.Load() != 1 {
		t.Fatalf(
			"runner close calls = (%d, %d), want (1, 1)",
			backtest.calls.Load(),
			instance.calls.Load(),
		)
	}
}

func TestHandleSerializesStrategyRuntimeAndPineRunnerUpdates(t *testing.T) {
	var handle Handle
	oldRunner := &recordingRunner{name: "old"}
	newRunner := &recordingRunner{name: "new"}
	handle.SetPineWorkerRunners(nil, oldRunner)
	sink := &blockingPineSink{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}

	strategySet := make(chan struct{})
	go func() {
		handle.setStrategyRuntime(nil, nil, sink)
		close(strategySet)
	}()
	<-sink.started

	runnersSet := make(chan struct{})
	go func() {
		handle.SetPineWorkerRunners(nil, newRunner)
		close(runnersSet)
	}()
	select {
	case <-runnersSet:
		t.Fatal("runner update crossed an in-flight strategy runtime update")
	default:
	}
	close(sink.release)
	<-strategySet
	<-runnersSet

	if got := sink.Current(); got != newRunner {
		t.Fatalf("strategy Pine runner = %T %p, want newest runner %p", got, got, newRunner)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("close runtimes: %v", err)
	}
}

func TestHandleClosesPineRunnersInjectedAfterShutdown(t *testing.T) {
	var handle Handle
	initial := &recordingRunner{name: "initial"}
	handle.SetPineWorkerRunners(initial, nil)
	if err := handle.Close(); err != nil {
		t.Fatalf("initial close: %v", err)
	}

	late := &recordingRunner{name: "late"}
	handle.SetPineWorkerRunners(late, nil)
	if got := late.calls.Load(); got != 1 {
		t.Fatalf("late runner close calls = %d, want 1", got)
	}
	if !errors.Is(handle.SetupError(), appcomposition.ErrResourcesClosed) {
		t.Fatalf("setup error = %v, want resources closed", handle.SetupError())
	}
	if !errors.Is(handle.Close(), appcomposition.ErrResourcesClosed) {
		t.Fatalf("repeated close error = %v, want resources closed", handle.Close())
	}
}

func TestHandleRejectsAssistantRuntimeInjectedAfterShutdown(t *testing.T) {
	var handle Handle
	if err := handle.Close(); err != nil {
		t.Fatalf("initial close: %v", err)
	}

	late := &recordingAssistantRuntime{}
	handle.SetAssistant(late)
	if got := late.closeCalls.Load(); got != 1 {
		t.Fatalf("late Assistant close calls = %d, want 1", got)
	}
	if got := handle.Assistant(); got != nil {
		t.Fatalf("late Assistant was published after shutdown: %T", got)
	}
	if !errors.Is(handle.SetupError(), appcomposition.ErrResourcesClosed) {
		t.Fatalf("setup error = %v, want resources closed", handle.SetupError())
	}
}

func TestHandleCloseAndAssistantPublicationAreAtomic(t *testing.T) {
	const iterations = 100
	for range iterations {
		var handle Handle
		runtime := &recordingAssistantRuntime{}
		start := make(chan struct{})
		var calls sync.WaitGroup
		calls.Add(2)
		go func() {
			defer calls.Done()
			<-start
			handle.SetAssistant(runtime)
		}()
		go func() {
			defer calls.Done()
			<-start
			_ = handle.Close()
		}()
		close(start)
		calls.Wait()

		if got := runtime.closeCalls.Load(); got != 1 {
			t.Fatalf("Assistant close calls = %d, want 1", got)
		}
		if got := handle.Assistant(); got != nil && got != runtime {
			t.Fatalf("Assistant publication changed identity during shutdown: %T", got)
		}
	}
}

func TestHandleCreatesAndRegistersOnePineManagerConcurrently(t *testing.T) {
	const callers = 16
	var handle Handle
	var createCalls atomic.Int32
	managers := make(chan *pineruntime.Manager, callers)
	var callersDone sync.WaitGroup
	callersDone.Add(callers)
	for range callers {
		go func() {
			defer callersDone.Done()
			manager, _, _ := handle.EnsurePineWorker(func() *pineruntime.Manager {
				createCalls.Add(1)
				return pineruntime.NewManager()
			})
			managers <- manager
		}()
	}
	callersDone.Wait()
	close(managers)

	var first *pineruntime.Manager
	for manager := range managers {
		if manager == nil {
			t.Fatal("Pine manager is nil")
		}
		if first == nil {
			first = manager
			continue
		}
		if manager != first {
			t.Fatal("concurrent callers received different Pine managers")
		}
	}
	if got := createCalls.Load(); got != 1 {
		t.Fatalf("manager create calls = %d, want 1", got)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("close runtimes: %v", err)
	}
	if manager, _, _ := handle.EnsurePineWorker(func() *pineruntime.Manager {
		return pineruntime.NewManager()
	}); manager != nil {
		t.Fatal("Pine manager was created after runtime shutdown")
	}
}
