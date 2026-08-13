package marketdataapp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
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
	runtime.healthCheck = func(context.Context, marketdata.Provider, bool) error { return nil }

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

func TestRuntimeRejectsInvalidActivationAndDefersFutuCleanupFailure(t *testing.T) {
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
	if err := runtime.Activate(t.Context(), enabled); err != nil {
		t.Fatalf("Futu cleanup failure blocked provider switch: %v", err)
	}
	if runtime.ActiveProviderID() != ProviderYFinance {
		t.Fatalf("provider after deferred cleanup = %q", runtime.ActiveProviderID())
	}
	if sidecar.ensureCalls != 1 || sidecar.stopCalls != 0 || !sidecar.running {
		t.Fatalf("committed sidecar = ensure %d stop %d running %v", sidecar.ensureCalls, sidecar.stopCalls, sidecar.running)
	}
}

func TestRuntimeRollsBackYFinanceWhenNonFutuProviderRetirementFails(t *testing.T) {
	releaseErr := errors.New("custom provider retirement failed")
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: "futu-opend"}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	runtime.active = runtimeState{
		providerID:    "custom-provider",
		provider:      &providerStub{id: "custom-provider"},
		subscriptions: &subscriptionReconcilerStub{err: releaseErr},
	}
	sidecar := &sidecarLifecycleStub{}
	installHealthySidecar(runtime, sidecar)

	err = runtime.Activate(t.Context(), Activation{ProviderID: ProviderYFinance})
	if !errors.Is(err, releaseErr) || runtime.ActiveProviderID() != "custom-provider" {
		t.Fatalf("custom provider rollback = provider %q, err %v",
			runtime.ActiveProviderID(), err)
	}
	if sidecar.ensureCalls != 1 || sidecar.stopCalls != 1 || sidecar.running {
		t.Fatalf("custom provider sidecar rollback = ensure %d stop %d running %v",
			sidecar.ensureCalls, sidecar.stopCalls, sidecar.running)
	}
}

func TestRuntimeRetiresPreviousSubscriptionsThroughBrokerReconciliation(t *testing.T) {
	subscriptions := &subscriptionReconcilerStub{
		desired: []marketdata.InstrumentRef{{Market: "US", Symbol: "AAPL"}},
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
	if subscriptions.reconcileCalls != 1 || len(subscriptions.desired) != 0 {
		t.Fatalf("subscription retirement = calls %d desired %#v",
			subscriptions.reconcileCalls, subscriptions.desired)
	}
}

func TestRuntimeReportsFutuActivationAndRollbackReleaseFailures(t *testing.T) {
	subscriptions := &subscriptionReconcilerStub{}
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
	subscriptions.reconcileErrors = []error{activationErr, rollbackErr}
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

func TestRuntimeContinuesInactiveFutuCleanupWhileYFinanceIsActive(t *testing.T) {
	subscriptions := &subscriptionReconcilerStub{}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      &providerStub{id: "futu-opend"},
		FutuSubscriptions: subscriptions,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	installHealthySidecar(runtime, &sidecarLifecycleStub{})
	if err := runtime.Activate(t.Context(), Activation{ProviderID: ProviderYFinance}); err != nil {
		t.Fatalf("Activate(yfinance): %v", err)
	}

	cleanupErr := errors.New("OpenD deferred unsubscribe failed")
	subscriptions.err = cleanupErr
	if err := runtime.ReconcileSubscriptions(t.Context(), []marketdata.InstrumentRef{{
		Market: "US", Symbol: "AAPL",
	}}); err != nil {
		t.Fatalf("foreground Yahoo reconciliation depended on Futu cleanup: %v", err)
	}
	if subscriptions.reconcileCalls != 1 {
		t.Fatalf("foreground Yahoo reconciliation reached Futu %d times", subscriptions.reconcileCalls)
	}
	err = runtime.ReconcileInactiveSubscriptions(t.Context())
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("inactive Futu cleanup error = %v", err)
	}
	if runtime.ActiveProviderID() != ProviderYFinance {
		t.Fatalf("cleanup failure changed provider to %q", runtime.ActiveProviderID())
	}
	if subscriptions.reconcileCalls != 2 || len(subscriptions.desired) != 0 {
		t.Fatalf("inactive Futu cleanup = calls %d desired %#v",
			subscriptions.reconcileCalls, subscriptions.desired)
	}

	subscriptions.err = nil
	if err := runtime.ReconcileInactiveSubscriptions(t.Context()); err != nil {
		t.Fatalf("retry inactive Futu cleanup: %v", err)
	}
	if subscriptions.reconcileCalls != 3 || len(subscriptions.desired) != 0 {
		t.Fatalf("inactive Futu retry = calls %d desired %#v",
			subscriptions.reconcileCalls, subscriptions.desired)
	}
}

func TestRuntimeSerializesInactiveCleanupWithFutuReactivation(t *testing.T) {
	subscriptions := &gatedSubscriptionReconciler{}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      &providerStub{id: "futu-opend"},
		FutuSubscriptions: subscriptions,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	installHealthySidecar(runtime, &sidecarLifecycleStub{})
	if err := runtime.Activate(t.Context(), Activation{ProviderID: ProviderYFinance}); err != nil {
		t.Fatalf("Activate(yfinance): %v", err)
	}

	cleanupStarted, cleanupRelease := subscriptions.blockNextReconcile()
	cleanupReleased := false
	defer func() {
		if !cleanupReleased {
			close(cleanupRelease)
		}
	}()
	cleanupErrors := make(chan error, 1)
	go func() {
		cleanupErrors <- runtime.ReconcileInactiveSubscriptions(context.Background())
	}()
	select {
	case <-cleanupStarted:
	case <-time.After(time.Second):
		t.Fatal("inactive cleanup did not start")
	}

	desired := []marketdata.InstrumentRef{{Channel: "KLINE", Market: "US", Symbol: "AAPL", Interval: "1m"}}
	activationStarted := make(chan struct{})
	activationErrors := make(chan error, 1)
	go func() {
		close(activationStarted)
		activationErrors <- runtime.Activate(context.Background(), Activation{
			ProviderID: ProviderFutu, DesiredSubscriptions: desired,
		})
	}()
	<-activationStarted
	select {
	case err := <-activationErrors:
		t.Fatalf("Futu reactivation overtook inactive cleanup: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(cleanupRelease)
	cleanupReleased = true
	if err := <-cleanupErrors; err != nil {
		t.Fatalf("inactive cleanup: %v", err)
	}
	if err := <-activationErrors; err != nil {
		t.Fatalf("Activate(futu): %v", err)
	}
	got := subscriptions.desiredSnapshot()
	if runtime.ActiveProviderID() != ProviderFutu || len(got) != 1 || got[0] != desired[0] {
		t.Fatalf("serialized reactivation = provider %q desired %#v", runtime.ActiveProviderID(), got)
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

func TestRuntimeCommitsSwitchWhenFutuRetirementContextExpires(t *testing.T) {
	subscriptions := &blockingSubscriptionReconciler{}
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
	if err != nil {
		t.Fatalf("expired Futu retirement blocked provider switch: %v", err)
	}
	if runtime.ActiveProviderID() != ProviderYFinance || subscriptions.reconcileCalls != 1 {
		t.Fatalf("timed-out activation = provider %q, reconcile calls %d",
			runtime.ActiveProviderID(), subscriptions.reconcileCalls)
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

func TestProviderLeasesKeepSharedPythonSidecarUntilLastRelease(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: ProviderFutu}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	installHealthySidecar(runtime, sidecar)

	yfinanceLease, err := runtime.AcquireProvider(t.Context(), ProviderYFinance, false)
	if err != nil {
		t.Fatalf("AcquireProvider(yfinance): %v", err)
	}
	secondYFinanceLease, err := runtime.AcquireProvider(t.Context(), ProviderYFinance, false)
	if err != nil {
		t.Fatalf("AcquireProvider(yfinance second): %v", err)
	}
	if yfinanceLease.Provider() != secondYFinanceLease.Provider() {
		t.Fatal("concurrent yfinance leases did not share one provider instance")
	}
	akshareLease, err := runtime.AcquireProvider(t.Context(), ProviderAKShare, false)
	if err != nil {
		t.Fatalf("AcquireProvider(akshare): %v", err)
	}
	yfinanceLease.Release()
	if sidecar.stopCalls != 0 || !sidecar.running {
		t.Fatalf("sidecar after first release = running %t, stops %d", sidecar.running, sidecar.stopCalls)
	}
	secondYFinanceLease.Release()
	if sidecar.stopCalls != 0 || !sidecar.running {
		t.Fatalf("sidecar with AKShare lease = running %t, stops %d", sidecar.running, sidecar.stopCalls)
	}
	akshareLease.Release()
	if sidecar.stopCalls != 1 || sidecar.running {
		t.Fatalf("sidecar after final release = running %t, stops %d", sidecar.running, sidecar.stopCalls)
	}
	// Release is idempotent and must not alter refcounts.
	akshareLease.Release()
	if sidecar.stopCalls != 1 {
		t.Fatalf("sidecar stops after duplicate release = %d", sidecar.stopCalls)
	}
}

func TestProviderSwitchKeepsAcceptedLeaseOnOldInstance(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: ProviderFutu}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	installHealthySidecar(runtime, sidecar)
	if err := runtime.Activate(t.Context(), Activation{ProviderID: ProviderYFinance}); err != nil {
		t.Fatalf("Activate(yfinance): %v", err)
	}
	lease, err := runtime.AcquireProvider(t.Context(), ProviderYFinance, false)
	if err != nil {
		t.Fatalf("AcquireProvider(yfinance): %v", err)
	}
	acceptedProvider := lease.Provider()
	if acceptedProvider != runtime.snapshot().provider {
		t.Fatal("accepted lease did not pin the active provider instance")
	}
	if err := runtime.Activate(t.Context(), Activation{ProviderID: ProviderFutu}); err != nil {
		t.Fatalf("Activate(futu): %v", err)
	}
	if runtime.ActiveProviderID() != ProviderFutu || lease.Provider() != acceptedProvider {
		t.Fatalf("switch changed accepted lease: active=%q lease=%p want=%p",
			runtime.ActiveProviderID(), lease.Provider(), acceptedProvider)
	}
	if !sidecar.running || sidecar.stopCalls != 0 {
		t.Fatalf("old lease lost sidecar: running=%t stops=%d", sidecar.running, sidecar.stopCalls)
	}
	lease.Release()
	if sidecar.running || sidecar.stopCalls != 1 {
		t.Fatalf("released old lease retained sidecar: running=%t stops=%d", sidecar.running, sidecar.stopCalls)
	}
}

func TestBacktestProviderHelpersListAndPrepareProviders(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: ProviderFutu}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &sidecarLifecycleStub{}
	installHealthySidecar(runtime, sidecar)
	service := marketdata.NewService(runtime)

	descriptors, err := ProviderCatalog(service)(t.Context())
	if err != nil {
		t.Fatalf("ProviderCatalog: %v", err)
	}
	wantIDs := []string{ProviderFutu, ProviderYFinance, ProviderAKShare}
	if len(descriptors) != len(wantIDs) {
		t.Fatalf("provider descriptor count = %d, want %d", len(descriptors), len(wantIDs))
	}
	for index, wantID := range wantIDs {
		if descriptors[index].SelectionID != wantID {
			t.Fatalf("descriptor[%d].SelectionID = %q, want %q",
				index, descriptors[index].SelectionID, wantID)
		}
	}

	prepare := BacktestProviderPreparer(service)
	if err := prepare(jfsettings.MarketDataProviderYFinance); err != nil {
		t.Fatalf("prepare yfinance: %v", err)
	}
	if sidecar.ensureCalls != 1 || sidecar.stopCalls != 1 || sidecar.running {
		t.Fatalf("prepared lease lifecycle = ensure %d stop %d running %t",
			sidecar.ensureCalls, sidecar.stopCalls, sidecar.running)
	}
	if len(runtime.providerLeases) != 0 {
		t.Fatalf("released provider leases = %#v", runtime.providerLeases)
	}
}

func TestBacktestProviderPreparerReturnsPreparationFailure(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: ProviderFutu}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	healthErr := errors.New("provider is unavailable")
	runtime.healthCheck = func(context.Context, marketdata.Provider, bool) error {
		return healthErr
	}
	service := marketdata.NewService(runtime)

	err = BacktestProviderPreparer(service)(jfsettings.MarketDataProviderFutu)
	if !errors.Is(err, healthErr) {
		t.Fatalf("prepare error = %v, want %v", err, healthErr)
	}
	if len(runtime.providerLeases) != 0 {
		t.Fatalf("failed preparation retained leases = %#v", runtime.providerLeases)
	}
}

func TestProviderLeaseNilBoundariesAndRuntimeInitialization(t *testing.T) {
	var missingRuntime *Runtime
	var missingContext context.Context
	if _, err := missingRuntime.AcquireProvider(missingContext, ProviderFutu, false); err == nil {
		t.Fatal("nil runtime acquired a provider")
	}
	var missingLease *ProviderLease
	if missingLease.ProviderID() != "" || missingLease.Provider() != nil {
		t.Fatal("nil lease exposed provider state")
	}
	if _, err := missingLease.Descriptor(t.Context()); err == nil {
		t.Fatal("nil lease returned a descriptor")
	}
	missingLease.Release()

	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &providerStub{id: ProviderFutu}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	runtime.providerPool = nil
	runtime.providerLeases = nil
	lease, err := runtime.AcquireProvider(missingContext, ProviderFutu, false)
	if err != nil {
		t.Fatalf("AcquireProvider: %v", err)
	}
	if lease.ProviderID() != ProviderFutu || lease.Provider() == nil {
		t.Fatalf("initialized lease = id %q provider %v", lease.ProviderID(), lease.Provider())
	}
	descriptor, err := lease.Descriptor(t.Context())
	if err != nil || descriptor.ProviderID != ProviderFutu {
		t.Fatalf("lease descriptor = %#v, %v", descriptor, err)
	}
	lease.Release()
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
	desired         []marketdata.InstrumentRef
	state           map[string]any
	err             error
	reconcileErrors []error
	reconcileCalls  int
}

func (s *subscriptionReconcilerStub) ReconcileSubscriptions(_ context.Context, desired []marketdata.InstrumentRef) error {
	s.reconcileCalls++
	s.desired = append([]marketdata.InstrumentRef(nil), desired...)
	if len(s.reconcileErrors) > 0 {
		err := s.reconcileErrors[0]
		s.reconcileErrors = s.reconcileErrors[1:]
		return err
	}
	return s.err
}

func (s *subscriptionReconcilerStub) SubscriptionState() map[string]any {
	return s.state
}

type blockingSubscriptionReconciler struct {
	reconcileCalls int
}

func (s *blockingSubscriptionReconciler) ReconcileSubscriptions(
	ctx context.Context,
	_ []marketdata.InstrumentRef,
) error {
	s.reconcileCalls++
	<-ctx.Done()
	return ctx.Err()
}

func (s *blockingSubscriptionReconciler) SubscriptionState() map[string]any {
	return nil
}

type gatedSubscriptionReconciler struct {
	mu           sync.Mutex
	desired      []marketdata.InstrumentRef
	blockNext    bool
	blockStarted chan struct{}
	blockRelease chan struct{}
}

func (s *gatedSubscriptionReconciler) blockNextReconcile() (<-chan struct{}, chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blockNext = true
	s.blockStarted = make(chan struct{})
	s.blockRelease = make(chan struct{})
	return s.blockStarted, s.blockRelease
}

func (s *gatedSubscriptionReconciler) ReconcileSubscriptions(
	ctx context.Context,
	desired []marketdata.InstrumentRef,
) error {
	s.mu.Lock()
	s.desired = append([]marketdata.InstrumentRef(nil), desired...)
	block := s.blockNext
	started, release := s.blockStarted, s.blockRelease
	s.blockNext = false
	s.mu.Unlock()
	if !block {
		return nil
	}
	close(started)
	select {
	case <-release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *gatedSubscriptionReconciler) SubscriptionState() map[string]any {
	return nil
}

func (s *gatedSubscriptionReconciler) desiredSnapshot() []marketdata.InstrumentRef {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]marketdata.InstrumentRef(nil), s.desired...)
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
	runtime.healthCheck = func(context.Context, marketdata.Provider, bool) error { return nil }
}
