package marketdataapp

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/internal/watchlist"
)

func TestApplyProviderSettingsUsesAtomicQuoteProviderSwitch(t *testing.T) {
	runtime, service := newProviderSwitchDataPlane(t)
	store := providerSettingsStoreStub{}
	cache := &atomicQuoteCacheStub{runtime: runtime}

	err := ApplyProviderSettings(
		t.Context(),
		service,
		&store,
		cache,
		jfsettings.MarketDataProviderYFinance,
		false,
	)
	if err != nil {
		t.Fatalf("ApplyProviderSettings: %v", err)
	}
	if cache.changeCalls != 1 || cache.resetCalls != 0 {
		t.Fatalf("quote cache calls = change %d, reset %d", cache.changeCalls, cache.resetCalls)
	}
	if cache.before != ProviderFutu || cache.after != ProviderYFinance {
		t.Fatalf("provider boundary = %q -> %q", cache.before, cache.after)
	}
}

func TestApplyProviderSettingsPreservesAtomicQuoteCacheOnFailure(t *testing.T) {
	runtime, service := newProviderSwitchDataPlane(t)
	switchErr := errors.New("sidecar start failed")
	runtime.sidecar = &sidecarLifecycleStub{ensureErr: switchErr}
	store := providerSettingsStoreStub{}
	cache := &atomicQuoteCacheStub{runtime: runtime}

	err := ApplyProviderSettings(
		t.Context(),
		service,
		&store,
		cache,
		jfsettings.MarketDataProviderYFinance,
		false,
	)
	if !errors.Is(err, switchErr) {
		t.Fatalf("ApplyProviderSettings error = %v", err)
	}
	if cache.changeCalls != 1 || cache.resetCalls != 0 {
		t.Fatalf("quote cache calls = change %d, reset %d", cache.changeCalls, cache.resetCalls)
	}
	if cache.before != ProviderFutu || cache.after != ProviderFutu {
		t.Fatalf("failed provider boundary = %q -> %q", cache.before, cache.after)
	}
}

func TestApplyProviderSettingsKeepsResetterCompatibility(t *testing.T) {
	_, service := newProviderSwitchDataPlane(t)
	store := providerSettingsStoreStub{}
	cache := &resetOnlyQuoteCacheStub{}

	err := ApplyProviderSettings(
		t.Context(),
		service,
		&store,
		cache,
		jfsettings.MarketDataProviderFutu,
		false,
	)
	if err != nil {
		t.Fatalf("ApplyProviderSettings: %v", err)
	}
	if cache.resetCalls != 1 {
		t.Fatalf("quote cache reset calls = %d", cache.resetCalls)
	}

	var unavailableWatchlist *watchlist.Service
	if err := ApplyProviderSettings(
		t.Context(),
		service,
		&store,
		unavailableWatchlist,
		jfsettings.MarketDataProviderFutu,
		false,
	); err != nil {
		t.Fatalf("ApplyProviderSettings with unavailable watchlist: %v", err)
	}
}

func TestApplyProviderSettingsRestoresExistingFutuDemandBeforeReturning(t *testing.T) {
	subscriptions := &physicalSubscriptionStub{}
	runtime, service := newProviderSwitchDataPlaneWithSubscriptions(t, subscriptions)
	store := providerSettingsStoreStub{}
	demand := marketdata.InstrumentRef{
		Channel: "SNAPSHOT",
		Market:  "US",
		Symbol:  "AAPL",
	}
	if _, err := service.AcquireSubscription(t.Context(), "chart", []marketdata.InstrumentRef{demand}); err != nil {
		t.Fatalf("AcquireSubscription: %v", err)
	}
	if len(subscriptions.desired) != 1 {
		t.Fatalf("initial physical subscriptions = %#v", subscriptions.desired)
	}

	if err := ApplyProviderSettings(
		t.Context(),
		service,
		&store,
		nil,
		jfsettings.MarketDataProviderYFinance,
		false,
	); err != nil {
		t.Fatalf("ApplyProviderSettings(yfinance): %v", err)
	}
	if len(subscriptions.desired) != 0 {
		t.Fatalf("Futu subscriptions remained active under yfinance: %#v", subscriptions.desired)
	}

	if err := ApplyProviderSettings(
		t.Context(),
		service,
		&store,
		nil,
		jfsettings.MarketDataProviderFutu,
		false,
	); err != nil {
		t.Fatalf("ApplyProviderSettings(futu): %v", err)
	}
	if runtime.ActiveProviderID() != ProviderFutu || len(subscriptions.desired) != 1 {
		t.Fatalf("synchronous Futu restore = provider %q, desired %#v",
			runtime.ActiveProviderID(), subscriptions.desired)
	}
	snapshot, err := service.GetSubscriptions(t.Context())
	if err != nil {
		t.Fatalf("GetSubscriptions: %v", err)
	}
	entries := snapshot["entries"].([]map[string]any)
	if snapshot["ownActiveCount"] != 1 || len(entries) != 1 || entries[0]["brokerState"] != "active" {
		t.Fatalf("immediate physical subscription state = %#v", snapshot)
	}
}

func TestApplyProviderSettingsRollsBackFailedFutuDemandRestore(t *testing.T) {
	subscriptions := &physicalSubscriptionStub{}
	runtime, service := newProviderSwitchDataPlaneWithSubscriptions(t, subscriptions)
	store := providerSettingsStoreStub{}
	if err := ApplyProviderSettings(
		t.Context(),
		service,
		&store,
		nil,
		jfsettings.MarketDataProviderYFinance,
		false,
	); err != nil {
		t.Fatalf("ApplyProviderSettings(yfinance): %v", err)
	}
	if _, err := service.AcquireSubscription(t.Context(), "chart", []marketdata.InstrumentRef{{
		Channel: "SNAPSHOT",
		Market:  "US",
		Symbol:  "AAPL",
	}}); err != nil {
		t.Fatalf("AcquireSubscription: %v", err)
	}

	restoreErr := errors.New("OpenD rejected subscription")
	subscriptions.reconcileErr = restoreErr
	err := ApplyProviderSettings(
		t.Context(),
		service,
		&store,
		nil,
		jfsettings.MarketDataProviderFutu,
		false,
	)
	if !errors.Is(err, restoreErr) {
		t.Fatalf("failed Futu activation error = %v", err)
	}
	if runtime.ActiveProviderID() != ProviderYFinance {
		t.Fatalf("failed activation exposed provider %q", runtime.ActiveProviderID())
	}
	if len(subscriptions.desired) != 0 || subscriptions.forceCalls != 2 {
		t.Fatalf("failed activation rollback = desired %#v, force calls %d",
			subscriptions.desired, subscriptions.forceCalls)
	}
	sidecar := runtime.sidecar.(*sidecarLifecycleStub)
	if sidecar.ensureCalls != 1 || sidecar.stopCalls != 0 || !sidecar.running {
		t.Fatalf("failed activation sidecar rollback = ensure %d stop %d running %v", sidecar.ensureCalls, sidecar.stopCalls, sidecar.running)
	}
}

func TestNewDataPlaneFallsBackWhenYFinanceHelperIsUnavailable(t *testing.T) {
	// Use the development-only override to exercise the unavailable-helper
	// fallback on both dev and release-assets test builds without starting the
	// frozen helper.
	t.Setenv("JFTRADE_YFINANCE_SIDECAR", filepath.Join(t.TempDir(), "missing-yfinance-sidecar"))
	store := providerSettingsStoreStub{
		active: jfsettings.MarketDataProviderYFinance,
	}
	plane, err := NewDataPlane(RuntimeOptions{
		FutuProvider: &providerStub{id: ProviderFutu},
	}, &store)
	if err != nil {
		t.Fatalf("NewDataPlane: %v", err)
	}
	t.Cleanup(func() {
		if err := plane.Service.Close(); err != nil {
			t.Errorf("service.Close: %v", err)
		}
		if err := plane.Runtime.Close(); err != nil {
			t.Errorf("runtime.Close: %v", err)
		}
	})
	if plane.Runtime.ActiveProviderID() != ProviderFutu ||
		store.active != jfsettings.MarketDataProviderFutu {
		t.Fatalf("invalid startup provider = runtime %q, persisted %q",
			plane.Runtime.ActiveProviderID(), store.active)
	}
}

func newProviderSwitchDataPlane(t *testing.T) (*Runtime, *marketdata.Service) {
	t.Helper()
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider: &providerStub{id: ProviderFutu},
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	installHealthySidecar(runtime, &sidecarLifecycleStub{})
	service := marketdata.NewService(runtime)
	service.SetSubscriptionReconciler(runtime)
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("service.Close: %v", err)
		}
		if err := runtime.Close(); err != nil {
			t.Errorf("runtime.Close: %v", err)
		}
	})
	return runtime, service
}

func newProviderSwitchDataPlaneWithSubscriptions(
	t *testing.T,
	subscriptions marketdata.SubscriptionReconciler,
) (*Runtime, *marketdata.Service) {
	t.Helper()
	runtime, err := NewRuntime(RuntimeOptions{
		FutuProvider:      &providerStub{id: ProviderFutu},
		FutuSubscriptions: subscriptions,
	})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	installHealthySidecar(runtime, &sidecarLifecycleStub{})
	service := marketdata.NewService(runtime)
	service.SetSubscriptionReconciler(runtime)
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("service.Close: %v", err)
		}
		if err := runtime.Close(); err != nil {
			t.Errorf("runtime.Close: %v", err)
		}
	})
	return runtime, service
}

type providerSettingsStoreStub struct {
	active jfsettings.ActiveMarketDataProvider
}

func (s *providerSettingsStoreStub) ActiveMarketDataProvider() jfsettings.ActiveMarketDataProvider {
	return s.active
}

func (s *providerSettingsStoreStub) SaveActiveMarketDataProvider(
	provider jfsettings.ActiveMarketDataProvider,
) error {
	s.active = provider
	return nil
}

type atomicQuoteCacheStub struct {
	runtime     *Runtime
	changeCalls int
	resetCalls  int
	before      string
	after       string
}

func (s *atomicQuoteCacheStub) ChangeQuoteProvider(change func() error) error {
	s.changeCalls++
	s.before = s.runtime.ActiveProviderID()
	err := change()
	s.after = s.runtime.ActiveProviderID()
	return err
}

func (s *atomicQuoteCacheStub) ResetQuoteCache() {
	s.resetCalls++
}

type resetOnlyQuoteCacheStub struct {
	resetCalls int
}

func (s *resetOnlyQuoteCacheStub) ResetQuoteCache() {
	s.resetCalls++
}

type physicalSubscriptionStub struct {
	desired        []marketdata.InstrumentRef
	reconcileErr   error
	reconcileCalls int
	forceCalls     int
}

func (s *physicalSubscriptionStub) ReconcileSubscriptions(
	_ context.Context,
	desired []marketdata.InstrumentRef,
) error {
	s.reconcileCalls++
	s.desired = append([]marketdata.InstrumentRef(nil), desired...)
	return s.reconcileErr
}

func (s *physicalSubscriptionStub) ForceReleaseSubscriptions(context.Context) error {
	s.forceCalls++
	s.desired = nil
	return nil
}

func (s *physicalSubscriptionStub) SubscriptionState() map[string]any {
	entries := make([]map[string]any, 0, len(s.desired))
	for _, ref := range s.desired {
		instrumentID := strings.ToUpper(strings.TrimSpace(ref.Market)) +
			"." + strings.ToUpper(strings.TrimSpace(ref.Symbol))
		entries = append(entries, map[string]any{
			"key":                   "BASIC:" + instrumentID,
			"brokerState":           "active",
			"subscribedAt":          "2026-07-30T00:00:00Z",
			"unsubscribeEligibleAt": "2026-07-30T00:01:00Z",
			"lastError":             nil,
		})
	}
	return map[string]any{
		"desiredCount":        len(s.desired),
		"ownActiveCount":      len(s.desired),
		"pendingReleaseCount": 0,
		"entries":             entries,
	}
}
