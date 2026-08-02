package marketdataapp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/marketdata"
)

func TestRuntimeExplicitYFinanceActivationRequiresHealthBeforePublishing(t *testing.T) {
	subscriptions := &subscriptionReconcilerStub{
		desired: []marketdata.InstrumentRef{{Channel: "BASIC", Market: "US", Symbol: "AAPL"}},
	}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      &forwardingProviderStub{},
		FutuSubscriptions: subscriptions,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &healthSidecarLifecycleStub{}
	runtime.sidecar = sidecar
	healthCalls := 0
	runtime.healthCheck = func(_ context.Context, provider marketdata.Provider, requireReady bool) error {
		healthCalls++
		if !requireReady {
			t.Fatal("explicit activation did not require a ready provider")
		}
		descriptor, descriptorErr := provider.Descriptor(t.Context())
		if descriptorErr != nil || descriptor.ProviderID != "yahoo-finance" {
			t.Fatalf("health-check provider = %#v, err=%v", descriptor, descriptorErr)
		}
		return nil
	}

	err = runtime.Activate(t.Context(), Activation{
		ProviderID: ProviderYFinance, RequireHealthy: true,
	})
	if err != nil {
		t.Fatalf("Activate(yfinance): %v", err)
	}
	if healthCalls != 1 || runtime.ActiveProviderID() != ProviderYFinance {
		t.Fatalf("healthy activation = calls %d, provider %q", healthCalls, runtime.ActiveProviderID())
	}
	if len(subscriptions.desired) != 0 {
		t.Fatalf("healthy activation did not release Futu subscriptions: %#v", subscriptions.desired)
	}
	if err := runtime.ReconcileSubscriptions(t.Context(), []marketdata.InstrumentRef{{
		Channel: "BASIC", Market: "US", Symbol: "MSFT",
	}}); err != nil {
		t.Fatalf("yfinance no-op subscription reconciliation: %v", err)
	}
	if len(subscriptions.desired) != 0 {
		t.Fatalf("yfinance reconciliation leaked into Futu: %#v", subscriptions.desired)
	}
	if sidecar.ensureCalls != 1 || sidecar.stopCalls != 0 {
		t.Fatalf("healthy activation sidecar calls = ensure %d stop %d", sidecar.ensureCalls, sidecar.stopCalls)
	}
}

func TestRuntimeFailedHealthCheckRestoresSidecarWithoutChangingProvider(t *testing.T) {
	healthErr := errors.New("sidecar health unavailable")
	subscriptions := &subscriptionReconcilerStub{
		desired: []marketdata.InstrumentRef{{Channel: "BASIC", Market: "US", Symbol: "AAPL"}},
	}
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      &forwardingProviderStub{},
		FutuSubscriptions: subscriptions,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &healthSidecarLifecycleStub{}
	runtime.sidecar = sidecar
	runtime.healthCheck = func(context.Context, marketdata.Provider, bool) error { return healthErr }

	err = runtime.Activate(t.Context(), Activation{
		ProviderID: ProviderYFinance, RequireHealthy: true,
	})
	if !errors.Is(err, healthErr) || runtime.ActiveProviderID() != ProviderFutu {
		t.Fatalf("failed health activation = provider %q, err=%v", runtime.ActiveProviderID(), err)
	}
	if sidecar.ensureCalls != 1 || sidecar.stopCalls != 1 {
		t.Fatalf("health rollback sidecar calls = ensure %d stop %d", sidecar.ensureCalls, sidecar.stopCalls)
	}
	if len(subscriptions.desired) != 1 {
		t.Fatalf("subscriptions released before health passed: %#v", subscriptions.desired)
	}
}

func TestRuntimeReportsBothHealthAndSidecarRestoreFailures(t *testing.T) {
	healthErr := errors.New("health failed")
	restoreErr := errors.New("sidecar restore failed")
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &forwardingProviderStub{}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &healthSidecarLifecycleStub{errors: []error{nil, restoreErr}}
	runtime.sidecar = sidecar
	runtime.healthCheck = func(context.Context, marketdata.Provider, bool) error { return healthErr }

	err = runtime.Activate(t.Context(), Activation{
		ProviderID: ProviderYFinance, RequireHealthy: true,
	})
	if !errors.Is(err, healthErr) || !errors.Is(err, restoreErr) ||
		runtime.ActiveProviderID() != ProviderFutu {
		t.Fatalf("joined health/restore failure = provider %q, err=%v", runtime.ActiveProviderID(), err)
	}
}

func TestRuntimeDefersSubscriptionReleaseFailureAfterHealthyActivation(t *testing.T) {
	releaseErr := errors.New("subscription release failed")
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      &forwardingProviderStub{},
		FutuSubscriptions: &subscriptionReconcilerStub{err: releaseErr},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	sidecar := &healthSidecarLifecycleStub{}
	runtime.sidecar = sidecar
	runtime.healthCheck = func(context.Context, marketdata.Provider, bool) error { return nil }

	err = runtime.Activate(t.Context(), Activation{
		ProviderID: ProviderYFinance,
	})
	if err != nil || runtime.ActiveProviderID() != ProviderYFinance {
		t.Fatalf("deferred release activation = provider %q, err=%v",
			runtime.ActiveProviderID(), err)
	}
	if cleanupErr := runtime.ReconcileInactiveSubscriptions(t.Context()); !errors.Is(cleanupErr, releaseErr) {
		t.Fatalf("background cleanup error = %v", cleanupErr)
	}
	if sidecar.ensureCalls != 1 || sidecar.stopCalls != 0 {
		t.Fatalf("committed sidecar calls = ensure %d stop %d", sidecar.ensureCalls, sidecar.stopCalls)
	}
}

func TestRuntimeChecksEmbeddedYFinanceOnStartupButNotFutu(t *testing.T) {
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: &forwardingProviderStub{}})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	runtime.sidecar = &healthSidecarLifecycleStub{}
	healthCalls := 0
	runtime.healthCheck = func(context.Context, marketdata.Provider, bool) error {
		healthCalls++
		return nil
	}

	if err := runtime.Activate(t.Context(), Activation{ProviderID: ProviderYFinance}); err != nil {
		t.Fatalf("startup restore yfinance activation: %v", err)
	}
	if err := runtime.Activate(t.Context(), Activation{
		ProviderID: ProviderFutu, RequireHealthy: true,
	}); err != nil {
		t.Fatalf("Futu activation: %v", err)
	}
	if healthCalls != 1 {
		t.Fatalf("health gate calls = %d, want one embedded yfinance startup check", healthCalls)
	}
}

func TestWaitForProviderHealthRetriesUntilConnected(t *testing.T) {
	provider := &healthSequenceProviderStub{
		statuses: []marketdata.HealthStatus{
			{Connected: false},
			{Connected: true, StreamMode: "snapshot-poll-delayed"},
		},
	}
	startedAt := time.Now()
	if err := waitForProviderHealth(t.Context(), provider, false); err != nil {
		t.Fatalf("waitForProviderHealth: %v", err)
	}
	if provider.calls != 2 || time.Since(startedAt) < providerHealthRetryDelay {
		t.Fatalf("health retry = calls %d, elapsed %s", provider.calls, time.Since(startedAt))
	}
}

func TestWaitForProviderHealthAllowsWarmingOnlyDuringStartupRestore(t *testing.T) {
	t.Parallel()
	startupProvider := &healthSequenceProviderStub{statuses: []marketdata.HealthStatus{{
		Connected: true,
		Readiness: marketdata.ProviderReadinessWarming,
	}}}
	if err := waitForProviderHealth(t.Context(), startupProvider, false); err != nil {
		t.Fatalf("startup warming health: %v", err)
	}

	explicitProvider := &healthSequenceProviderStub{statuses: []marketdata.HealthStatus{
		{Connected: true, Readiness: marketdata.ProviderReadinessWarming},
		{Connected: true, Readiness: marketdata.ProviderReadinessReady},
	}}
	if err := waitForProviderHealth(t.Context(), explicitProvider, true); err != nil {
		t.Fatalf("explicit ready health: %v", err)
	}
	if explicitProvider.calls != 2 {
		t.Fatalf("explicit health calls = %d, want 2", explicitProvider.calls)
	}
}

func TestWaitForProviderHealthStopsOnFailedWarmup(t *testing.T) {
	t.Parallel()
	provider := &healthSequenceProviderStub{statuses: []marketdata.HealthStatus{{
		Connected: true,
		Readiness: marketdata.ProviderReadinessFailed,
		LastError: "missing runtime asset",
	}}}

	err := waitForProviderHealth(t.Context(), provider, true)
	if err == nil || !strings.Contains(err.Error(), "missing runtime asset") {
		t.Fatalf("failed warmup error = %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("failed warmup calls = %d, want 1", provider.calls)
	}
}

func TestProviderHealthRetryDelayBacksOffAndCaps(t *testing.T) {
	delay := time.Duration(0)
	want := []time.Duration{
		providerHealthRetryDelay,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		providerHealthMaxRetryDelay,
		providerHealthMaxRetryDelay,
	}
	for index, expected := range want {
		delay = nextProviderHealthRetryDelay(delay)
		if delay != expected {
			t.Fatalf("retry delay %d = %s, want %s", index, delay, expected)
		}
	}
}

func TestWaitForProviderHealthPreservesLastFailureOnCancellation(t *testing.T) {
	probeErr := errors.New("connection refused")
	provider := &healthSequenceProviderStub{fallbackErr: probeErr}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := waitForProviderHealth(ctx, provider, false); !errors.Is(err, probeErr) {
		t.Fatalf("health probe failure = %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("health probe calls = %d", provider.calls)
	}

	disconnected := &healthSequenceProviderStub{}
	ctx, cancel = context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	err := waitForProviderHealth(ctx, disconnected, false)
	if err == nil || !strings.Contains(err.Error(), "reported disconnected") {
		t.Fatalf("disconnected health error = %v", err)
	}
}

func TestWaitForProviderHealthAndRuntimeDefaultCheckerBoundaries(t *testing.T) {
	if err := waitForProviderHealth(t.Context(), nil, false); err == nil ||
		!strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil provider health error = %v", err)
	}
	healthy := &healthSequenceProviderStub{
		statuses: []marketdata.HealthStatus{{Connected: true}},
	}
	runtime := &Runtime{}
	if err := runtime.checkHealth(t.Context(), healthy, false); err != nil {
		t.Fatalf("default checkHealth: %v", err)
	}
	if healthy.calls != 1 {
		t.Fatalf("default checkHealth calls = %d", healthy.calls)
	}
}

type healthSidecarLifecycleStub struct {
	ensureCalls int
	stopCalls   int
	errors      []error
}

func (s *healthSidecarLifecycleStub) EnsureStarted() (string, error) {
	s.ensureCalls++
	if len(s.errors) == 0 {
		return "http://127.0.0.1:43123", nil
	}
	err := s.errors[0]
	s.errors = s.errors[1:]
	if err == nil {
		return "http://127.0.0.1:43123", nil
	}
	return "", err
}

func (s *healthSidecarLifecycleStub) Stop() error {
	s.stopCalls++
	if len(s.errors) == 0 {
		return nil
	}
	err := s.errors[0]
	s.errors = s.errors[1:]
	if err == nil {
		return nil
	}
	return err
}

func (*healthSidecarLifecycleStub) Close() error { return nil }

type healthSequenceProviderStub struct {
	marketdata.Provider
	statuses    []marketdata.HealthStatus
	errors      []error
	fallbackErr error
	calls       int
}

func (p *healthSequenceProviderStub) Health(context.Context) (marketdata.HealthStatus, error) {
	index := p.calls
	p.calls++
	if index < len(p.statuses) {
		var err error
		if index < len(p.errors) {
			err = p.errors[index]
		}
		return p.statuses[index], err
	}
	return marketdata.HealthStatus{}, p.fallbackErr
}
