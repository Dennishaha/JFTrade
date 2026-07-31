package marketdataapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestRuntimeSwitchesStableDataPlaneBetweenFutuAndYFinance(t *testing.T) {
	futuProvider := &providerStub{id: "futu-opend"}
	futuQuotes := &quoteSourceStub{}
	futuPush := &pushSourceStub{}
	futuSubscriptions := &subscriptionReconcilerStub{
		state: map[string]any{"ownActiveCount": 1},
	}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      futuProvider,
		FutuQuotes:        futuQuotes,
		FutuPush:          futuPush,
		FutuSubscriptions: futuSubscriptions,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	installHealthySidecar(runtime, sidecar)

	if runtime.ActiveProviderID() != ProviderFutu || !runtime.PushAvailable() {
		t.Fatalf("initial runtime state = %q, push=%v", runtime.ActiveProviderID(), runtime.PushAvailable())
	}
	descriptor, err := runtime.Descriptor(t.Context())
	if err != nil || descriptor.ProviderID != "futu-opend" {
		t.Fatalf("initial descriptor = %#v, %v", descriptor, err)
	}

	activation := Activation{ProviderID: ProviderYFinance}
	if err := runtime.Activate(t.Context(), activation); err != nil {
		t.Fatalf("Activate(yfinance): %v", err)
	}
	if runtime.ActiveProviderID() != ProviderYFinance || runtime.PushAvailable() {
		t.Fatalf("yfinance runtime state = %q, push=%v", runtime.ActiveProviderID(), runtime.PushAvailable())
	}
	descriptor, err = runtime.Descriptor(t.Context())
	if err != nil || descriptor.ProviderID != "yahoo-finance" {
		t.Fatalf("yfinance descriptor = %#v, %v", descriptor, err)
	}
	if len(futuSubscriptions.desired) != 0 {
		t.Fatalf("released subscriptions = %#v", futuSubscriptions.desired)
	}
	if runtime.SubscriptionState() != nil {
		t.Fatalf("yfinance subscription state = %#v", runtime.SubscriptionState())
	}
	if _, err := runtime.NewStream(nil, nil); !errors.Is(err, ErrStreamingUnavailable) {
		t.Fatalf("NewStream(yfinance) error = %v", err)
	}
	if sidecar.ensureCalls != 1 || !sidecar.running {
		t.Fatalf("sidecar start state = ensure %d, running %v", sidecar.ensureCalls, sidecar.running)
	}

	if err := runtime.Activate(t.Context(), Activation{ProviderID: ProviderFutu}); err != nil {
		t.Fatalf("Activate(futu): %v", err)
	}
	if runtime.ActiveProviderID() != ProviderFutu || !runtime.PushAvailable() {
		t.Fatalf("restored runtime state = %q, push=%v", runtime.ActiveProviderID(), runtime.PushAvailable())
	}
	if sidecar.stopCalls != 1 || sidecar.running {
		t.Fatalf("sidecar stop state = calls %d, running %v", sidecar.stopCalls, sidecar.running)
	}
	if _, err := runtime.QueryTickers(t.Context(), []string{"US.AAPL"}); err != nil {
		t.Fatalf("QueryTickers(futu): %v", err)
	}
	if futuQuotes.calls != 1 {
		t.Fatalf("Futu quote calls = %d", futuQuotes.calls)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestRuntimeCommitsFutuWhenSidecarCleanupNeedsRetry(t *testing.T) {
	stopErr := errors.New("sidecar cleanup pending")
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: "futu-opend"}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	installHealthySidecar(runtime, sidecar)
	runtime.healthCheck = func(context.Context, marketdata.Provider) error { return nil }

	if err := runtime.Activate(t.Context(), Activation{ProviderID: ProviderYFinance}); err != nil {
		t.Fatalf("Activate(yfinance): %v", err)
	}
	sidecar.stopErr = stopErr
	if err := runtime.Activate(t.Context(), Activation{ProviderID: ProviderFutu}); err != nil {
		t.Fatalf("Activate(futu) should tolerate deferred cleanup: %v", err)
	}
	if runtime.ActiveProviderID() != ProviderFutu {
		t.Fatalf("provider after deferred cleanup = %q, want futu", runtime.ActiveProviderID())
	}

	// A later close can retry the retained sidecar cleanup state.
	sidecar.stopErr = nil
	if err := runtime.Close(); err != nil {
		t.Fatalf("Runtime.Close retry: %v", err)
	}
}

func TestRuntimeRejectsInvalidActivationAndRollsBackSidecar(t *testing.T) {
	releaseErr := errors.New("release failed")
	reconciler := &subscriptionReconcilerStub{err: releaseErr}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      &providerStub{id: "futu-opend"},
		FutuSubscriptions: reconciler,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	runtime.sidecar = sidecar

	if err := runtime.Activate(t.Context(), Activation{ProviderID: "unknown"}); err == nil {
		t.Fatal("unknown provider was accepted")
	}

	installHealthySidecar(runtime, sidecar)
	enabled := Activation{ProviderID: ProviderYFinance}
	if err := runtime.Activate(t.Context(), enabled); !errors.Is(err, releaseErr) {
		t.Fatalf("release failure = %v", err)
	}
	if runtime.ActiveProviderID() != ProviderFutu {
		t.Fatalf("provider changed after failed release: %q", runtime.ActiveProviderID())
	}
	if sidecar.ensureCalls != 1 || sidecar.stopCalls != 1 || sidecar.running {
		t.Fatalf("sidecar rollback = ensure %d stop %d running %v", sidecar.ensureCalls, sidecar.stopCalls, sidecar.running)
	}
}

func TestRuntimeUsesForcedPhysicalReleaseWhenProviderSupportsIt(t *testing.T) {
	subscriptions := &forcedSubscriptionReconcilerStub{
		subscriptionReconcilerStub: &subscriptionReconcilerStub{},
	}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      &providerStub{id: "futu-opend"},
		FutuSubscriptions: subscriptions,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	installHealthySidecar(runtime, &sidecarLifecycleStub{})
	if err := runtime.Activate(t.Context(), Activation{
		ProviderID: ProviderYFinance,
	}); err != nil {
		t.Fatalf("Activate(yfinance): %v", err)
	}
	if subscriptions.forceCalls != 1 || subscriptions.reconcileCalls != 0 {
		t.Fatalf("release calls = force %d, reconcile %d",
			subscriptions.forceCalls, subscriptions.reconcileCalls)
	}
}

func TestRuntimeReportsFutuActivationAndRollbackReleaseFailures(t *testing.T) {
	subscriptions := &forcedSubscriptionReconcilerStub{
		subscriptionReconcilerStub: &subscriptionReconcilerStub{},
	}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      &providerStub{id: "futu-opend"},
		FutuSubscriptions: subscriptions,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	installHealthySidecar(runtime, sidecar)
	if err := runtime.Activate(t.Context(), Activation{
		ProviderID: ProviderYFinance,
	}); err != nil {
		t.Fatalf("Activate(yfinance): %v", err)
	}

	activationErr := errors.New("OpenD rejected subscription")
	rollbackErr := errors.New("OpenD rejected rollback release")
	subscriptions.err = activationErr
	subscriptions.forceErr = rollbackErr
	err = runtime.Activate(t.Context(), Activation{
		ProviderID: ProviderFutu,
		DesiredSubscriptions: []marketdata.InstrumentRef{{
			Channel: "SNAPSHOT",
			Market:  "US",
			Symbol:  "AAPL",
		}},
	})
	if !errors.Is(err, activationErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("joined activation/rollback error = %v", err)
	}
	if runtime.ActiveProviderID() != ProviderYFinance {
		t.Fatalf("failed Futu activation exposed provider %q", runtime.ActiveProviderID())
	}
	if sidecar.ensureCalls != 1 || sidecar.stopCalls != 0 || !sidecar.running {
		t.Fatalf("failed Futu activation sidecar state = ensure %d stop %d running %v", sidecar.ensureCalls, sidecar.stopCalls, sidecar.running)
	}
}

func TestRuntimeRestoresPreviousSubscriptionsAfterActivationFailure(t *testing.T) {
	previousSubscriptions := &subscriptionReconcilerStub{}
	activationErr := errors.New("new provider rejected subscriptions")
	nextSubscriptions := &subscriptionReconcilerStub{err: activationErr}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: "futu"}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	runtime.active = runtimeState{
		providerID:    "old-provider",
		provider:      &providerStub{id: "old-provider"},
		subscriptions: previousSubscriptions,
	}
	runtime.futu = runtimeState{
		providerID:    ProviderFutu,
		provider:      &providerStub{id: "futu"},
		subscriptions: nextSubscriptions,
	}
	desired := []marketdata.InstrumentRef{{Channel: "SNAPSHOT", Market: "US", Symbol: "AAPL"}}

	err = runtime.Activate(t.Context(), Activation{ProviderID: ProviderFutu, DesiredSubscriptions: desired})
	if !errors.Is(err, activationErr) {
		t.Fatalf("activation error = %v, want %v", err, activationErr)
	}
	if runtime.ActiveProviderID() != "old-provider" {
		t.Fatalf("failed activation exposed provider %q", runtime.ActiveProviderID())
	}
	if previousSubscriptions.reconcileCalls != 2 || len(previousSubscriptions.desired) != 1 ||
		previousSubscriptions.desired[0] != desired[0] {
		t.Fatalf("previous subscriptions were not restored: calls=%d desired=%#v", previousSubscriptions.reconcileCalls, previousSubscriptions.desired)
	}
}

func TestRuntimeBoundsForcedReleaseWithActivationContext(t *testing.T) {
	subscriptions := &blockingForcedSubscriptionReconciler{
		subscriptionReconcilerStub: &subscriptionReconcilerStub{},
	}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      &providerStub{id: "futu-opend"},
		FutuSubscriptions: subscriptions,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	installHealthySidecar(runtime, &sidecarLifecycleStub{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err = runtime.Activate(ctx, Activation{
		ProviderID: ProviderYFinance,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("bounded forced release error = %v", err)
	}
	if runtime.ActiveProviderID() != ProviderFutu || subscriptions.forceCalls != 1 {
		t.Fatalf("timed-out activation = provider %q, force calls %d",
			runtime.ActiveProviderID(), subscriptions.forceCalls)
	}
}

func TestRuntimeConstructorAndUnavailablePollingBoundaries(t *testing.T) {
	if _, err := NewRuntime(RuntimeOptions{}); err == nil {
		t.Fatal("NewRuntime accepted nil Futu provider")
	}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: "futu"}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	if _, err := runtime.QueryTickers(t.Context(), nil); err == nil {
		t.Fatal("QueryTickers accepted unavailable source")
	}
	var nilRuntime *Runtime
	if err := nilRuntime.Activate(t.Context(), Activation{}); err == nil {
		t.Fatal("nil runtime activation succeeded")
	}
	if err := nilRuntime.Close(); err != nil {
		t.Fatalf("nil runtime Close: %v", err)
	}
}

func TestRuntimeCannotRestartManagedSidecarAfterClose(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider: &providerStub{id: "futu"},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	runtime.sidecar = sidecar
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	err = runtime.Activate(t.Context(), Activation{ProviderID: ProviderYFinance})
	if !errors.Is(err, ErrRuntimeClosed) {
		t.Fatalf("Activate after Close error = %v", err)
	}
	if sidecar.ensureCalls != 0 || runtime.ActiveProviderID() != ProviderFutu {
		t.Fatalf("closed runtime changed provider: ensure=%d provider=%q",
			sidecar.ensureCalls, runtime.ActiveProviderID())
	}
}

func TestRuntimeCloseWinsConcurrentActivationWithoutRestartingSidecar(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider: &providerStub{id: "futu"},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{
		closeStarted: make(chan struct{}),
		closeRelease: make(chan struct{}),
	}
	installHealthySidecar(runtime, sidecar)
	closeErrors := make(chan error, 1)
	go func() {
		closeErrors <- runtime.Close()
	}()
	<-sidecar.closeStarted

	activationErrors := make(chan error, 1)
	go func() {
		activationErrors <- runtime.Activate(context.Background(), Activation{ProviderID: ProviderYFinance})
	}()
	select {
	case err := <-activationErrors:
		t.Fatalf("activation escaped shutdown lock: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(sidecar.closeRelease)
	if err := <-closeErrors; err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := <-activationErrors; !errors.Is(err, ErrRuntimeClosed) {
		t.Fatalf("concurrent Activate error = %v", err)
	}
	if sidecar.ensureCalls != 0 {
		t.Fatalf("sidecar restarted after Close: %d", sidecar.ensureCalls)
	}
}

func TestRuntimeDoesNotCommitActivationCanceledWhileQueued(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider: &providerStub{id: "futu"},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	installHealthySidecar(runtime, sidecar)
	runtime.switchMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	activationErrors := make(chan error, 1)
	go func() {
		activationErrors <- runtime.Activate(ctx, Activation{ProviderID: ProviderYFinance})
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	runtime.switchMu.Unlock()

	if err := <-activationErrors; !errors.Is(err, context.Canceled) {
		t.Fatalf("queued Activate error = %v", err)
	}
	if sidecar.ensureCalls != 0 || runtime.ActiveProviderID() != ProviderFutu {
		t.Fatalf("canceled activation changed runtime: ensure=%d provider=%q",
			sidecar.ensureCalls, runtime.ActiveProviderID())
	}
}

func TestProviderChangeCannotRestoreSubscriptionsAfterRuntimeClose(t *testing.T) {
	subscriptions := &subscriptionReconcilerStub{}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      &providerStub{id: "futu"},
		FutuSubscriptions: subscriptions,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{
		closeStarted: make(chan struct{}),
		closeRelease: make(chan struct{}),
	}
	installHealthySidecar(runtime, sidecar)
	service := marketdata.NewService(runtime)
	service.SetSubscriptionReconciler(runtime)

	closeErrors := make(chan error, 1)
	go func() {
		closeErrors <- runtime.Close()
	}()
	<-sidecar.closeStarted
	changeErrors := make(chan error, 1)
	go func() {
		changeErrors <- service.ChangeProvider(func() error {
			return runtime.Activate(context.Background(), Activation{ProviderID: ProviderYFinance})
		})
	}()
	time.Sleep(10 * time.Millisecond)
	close(sidecar.closeRelease)

	if err := <-closeErrors; err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := <-changeErrors; !errors.Is(err, ErrRuntimeClosed) {
		t.Fatalf("provider change error = %v", err)
	}
	if subscriptions.reconcileCalls != 0 {
		t.Fatalf("closed runtime restored %d physical subscription sets",
			subscriptions.reconcileCalls)
	}
}

func TestRuntimeRetriesSidecarCleanupWithinClose(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider: &providerStub{id: "futu"},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	closeErr := errors.New("temporary close failure")
	sidecar := &retryingSidecarLifecycleStub{errors: []error{closeErr, nil}}
	runtime.sidecar = sidecar
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close after successful cleanup retry: %v", err)
	}
	if sidecar.closeCalls != 2 {
		t.Fatalf("sidecar close calls = %d", sidecar.closeCalls)
	}
	if err := runtime.Activate(t.Context(), Activation{
		ProviderID: ProviderFutu,
	}); !errors.Is(err, ErrRuntimeClosed) {
		t.Fatalf("Activate after cleanup retry error = %v", err)
	}
}

func TestRuntimeReportsBothBoundedSidecarCleanupFailures(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider: &providerStub{id: "futu"},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	firstErr := errors.New("first close timeout")
	retryErr := errors.New("retry close timeout")
	sidecar := &retryingSidecarLifecycleStub{errors: []error{firstErr, retryErr}}
	runtime.sidecar = sidecar

	err = runtime.Close()
	if !errors.Is(err, firstErr) || !errors.Is(err, retryErr) {
		t.Fatalf("Close errors = %v", err)
	}
	if sidecar.closeCalls != 2 {
		t.Fatalf("sidecar close calls = %d", sidecar.closeCalls)
	}
}

type providerStub struct {
	marketdata.Provider
	id string
}

func (p *providerStub) Descriptor(context.Context) (marketdata.ProviderDescriptor, error) {
	return marketdata.ProviderDescriptor{ProviderID: p.id}, nil
}

type quoteSourceStub struct {
	calls int
}

func (s *quoteSourceStub) QueryTickers(context.Context, []string) (map[string]marketdata.Tick, error) {
	s.calls++
	return map[string]marketdata.Tick{}, nil
}

type pushSourceStub struct {
	marketdata.PushSource
}

type subscriptionReconcilerStub struct {
	desired        []marketdata.InstrumentRef
	state          map[string]any
	err            error
	reconcileCalls int
}

func (s *subscriptionReconcilerStub) ReconcileSubscriptions(_ context.Context, desired []marketdata.InstrumentRef) error {
	s.reconcileCalls++
	s.desired = append([]marketdata.InstrumentRef(nil), desired...)
	return s.err
}

func (s *subscriptionReconcilerStub) SubscriptionState() map[string]any {
	return s.state
}

type forcedSubscriptionReconcilerStub struct {
	*subscriptionReconcilerStub
	forceCalls int
	forceErr   error
}

func (s *forcedSubscriptionReconcilerStub) ForceReleaseSubscriptions(context.Context) error {
	s.forceCalls++
	return s.forceErr
}

type blockingForcedSubscriptionReconciler struct {
	*subscriptionReconcilerStub
	forceCalls int
}

func (s *blockingForcedSubscriptionReconciler) ForceReleaseSubscriptions(ctx context.Context) error {
	s.forceCalls++
	<-ctx.Done()
	return ctx.Err()
}

type sidecarLifecycleStub struct {
	ensureCalls  int
	stopCalls    int
	running      bool
	ensureErr    error
	stopErr      error
	closeStarted chan struct{}
	closeRelease chan struct{}
}

type retryingSidecarLifecycleStub struct {
	errors     []error
	closeCalls int
}

func (s *retryingSidecarLifecycleStub) EnsureStarted() (string, error) {
	return "http://127.0.0.1:43123", nil
}

func (*retryingSidecarLifecycleStub) Stop() error {
	return nil
}

func (s *retryingSidecarLifecycleStub) Close() error {
	s.closeCalls++
	if len(s.errors) == 0 {
		return nil
	}
	err := s.errors[0]
	s.errors = s.errors[1:]
	return err
}

func (s *sidecarLifecycleStub) EnsureStarted() (string, error) {
	s.ensureCalls++
	if s.ensureErr != nil {
		return "", s.ensureErr
	}
	s.running = true
	return "http://127.0.0.1:43123", nil
}

func (s *sidecarLifecycleStub) Stop() error {
	if !s.running {
		return nil
	}
	s.stopCalls++
	if s.stopErr != nil {
		return s.stopErr
	}
	s.running = false
	return nil
}

func (s *sidecarLifecycleStub) Close() error {
	if s.closeStarted != nil {
		close(s.closeStarted)
	}
	if s.closeRelease != nil {
		<-s.closeRelease
	}
	return nil
}

func installHealthySidecar(runtime *Runtime, sidecar *sidecarLifecycleStub) {
	runtime.sidecar = sidecar
	runtime.healthCheck = func(context.Context, marketdata.Provider) error { return nil }
}
