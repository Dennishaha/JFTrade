package marketdataapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/internal/marketdata"
	settingssrv "github.com/jftrade/jftrade-main/internal/settings"
)

type assistantProviderSettingsStore struct {
	settingssrv.Store
	active      jfsettings.ActiveMarketDataProvider
	backtest    jfsettings.ActiveMarketDataProvider
	activeErr   error
	backtestErr error
}

func (s *assistantProviderSettingsStore) ActiveMarketDataProvider() jfsettings.ActiveMarketDataProvider {
	return s.active
}

func (s *assistantProviderSettingsStore) SaveActiveMarketDataProvider(
	provider jfsettings.ActiveMarketDataProvider,
) error {
	if s.activeErr != nil {
		return s.activeErr
	}
	s.active = provider
	return nil
}

func (s *assistantProviderSettingsStore) BacktestMarketDataProvider() jfsettings.ActiveMarketDataProvider {
	return s.backtest
}

func (s *assistantProviderSettingsStore) SaveBacktestMarketDataProvider(
	provider jfsettings.ActiveMarketDataProvider,
) error {
	if s.backtestErr != nil {
		return s.backtestErr
	}
	s.backtest = provider
	return nil
}

type assistantStatusProvider struct {
	marketdata.Provider
	descriptorErr error
	healthErr     error
}

func (p *assistantStatusProvider) Descriptor(context.Context) (marketdata.ProviderDescriptor, error) {
	if p.descriptorErr != nil {
		return marketdata.ProviderDescriptor{}, p.descriptorErr
	}
	return marketdata.ProviderDescriptor{ProviderID: "futu-opend"}, nil
}

func (p *assistantStatusProvider) Health(context.Context) (marketdata.HealthStatus, error) {
	if p.healthErr != nil {
		return marketdata.HealthStatus{}, p.healthErr
	}
	return marketdata.HealthStatus{Connected: true, Readiness: marketdata.ProviderReadinessReady}, nil
}

func newAssistantProviderServices(t *testing.T) (*marketdata.Service, *settingssrv.Service, *assistantProviderSettingsStore) {
	t.Helper()
	provider := &assistantStatusProvider{}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: provider})
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	service := marketdata.NewService(runtime)
	store := &assistantProviderSettingsStore{
		active: jfsettings.MarketDataProviderFutu, backtest: jfsettings.MarketDataProviderYFinance,
	}
	settingsService := settingssrv.NewService(store, settingssrv.WithMarketDataProviderCatalog(
		ProviderCatalog(service),
	))
	t.Cleanup(func() {
		if err := service.Close(); err != nil {
			t.Errorf("marketdata service Close: %v", err)
		}
		if err := runtime.Close(); err != nil {
			t.Errorf("marketdata runtime Close: %v", err)
		}
	})
	return service, settingsService, store
}

func TestAssistantMarketProvidersReportsSelectionAndActiveHealth(t *testing.T) {
	service, settingsService, _ := newAssistantProviderServices(t)
	result, err := AssistantMarketProviders(t.Context(), service, settingsService)
	if err != nil {
		t.Fatalf("AssistantMarketProviders: %v", err)
	}
	payload := result.(map[string]any)
	if payload["liveProvider"] != ProviderFutu || payload["backtestProvider"] != string(jfsettings.MarketDataProviderYFinance) {
		t.Fatalf("provider selection = %#v", payload)
	}
	if len(payload["providers"].([]any)) != 3 {
		t.Fatalf("provider descriptors = %#v", payload["providers"])
	}
	health := payload["liveHealth"].(marketdata.HealthStatus)
	if !health.Connected || health.Readiness != marketdata.ProviderReadinessReady {
		t.Fatalf("live health = %#v", health)
	}
}

func TestSelectAssistantMarketProviderPersistsScopeAndReturnsBeforeAfter(t *testing.T) {
	service, settingsService, store := newAssistantProviderServices(t)
	for _, test := range []struct {
		scope string
		want  jfsettings.ActiveMarketDataProvider
	}{
		{scope: "live", want: jfsettings.MarketDataProviderYFinance},
		{scope: "backtest", want: jfsettings.MarketDataProviderAKShare},
	} {
		result, err := SelectAssistantMarketProvider(t.Context(), service, settingsService, test.scope, string(test.want))
		if err != nil {
			t.Fatalf("SelectAssistantMarketProvider(%s): %v", test.scope, err)
		}
		payload := result.(map[string]any)
		if payload["scope"] != test.scope || payload["providerId"] != string(test.want) || payload["before"] == nil || payload["after"] == nil {
			t.Fatalf("selection payload = %#v", payload)
		}
	}
	if store.active != jfsettings.MarketDataProviderYFinance || store.backtest != jfsettings.MarketDataProviderAKShare {
		t.Fatalf("persisted providers = %q/%q", store.active, store.backtest)
	}
	if _, err := SelectAssistantMarketProvider(t.Context(), service, settingsService, "live", " "); err == nil {
		t.Fatal("blank providerId error = nil")
	}
	if _, err := SelectAssistantMarketProvider(t.Context(), service, settingsService, "other", "futu"); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("invalid scope error = %v", err)
	}
}

func TestAssistantProviderPortsAndUnavailableServices(t *testing.T) {
	ports := NewAssistantProviderPorts(nil, nil)
	if _, err := ports.MarketProviders(context.Background()); err == nil {
		t.Fatal("nil MarketProviders error = nil")
	}
	if _, err := ports.SelectMarketProvider(context.Background(), "live", "futu"); err == nil {
		t.Fatal("nil SelectMarketProvider error = nil")
	}
	if _, err := AssistantMarketProviders(context.Background(), nil, nil); err == nil {
		t.Fatal("nil AssistantMarketProviders error = nil")
	}
	if _, err := SelectAssistantMarketProvider(context.Background(), nil, nil, "live", "futu"); err == nil {
		t.Fatal("nil SelectAssistantMarketProvider error = nil")
	}
}

func TestAssistantMarketProviderReportsAndPropagatesFailureBoundaries(t *testing.T) {
	service, _, _ := newAssistantProviderServices(t)
	getCalls := 0
	store := &assistantProviderSettingsStore{
		active:   jfsettings.MarketDataProviderFutu,
		backtest: jfsettings.MarketDataProviderYFinance,
	}
	settingsService := settingssrv.NewService(store, settingssrv.WithMarketDataProviderCatalog(func(context.Context) ([]marketdata.ProviderDescriptor, error) {
		getCalls++
		if getCalls > 1 {
			return nil, context.DeadlineExceeded
		}
		return nil, nil
	}))
	if _, err := AssistantMarketProviders(t.Context(), service, settingsService); err != nil {
		t.Fatalf("initial provider status: %v", err)
	}
	if _, err := SelectAssistantMarketProvider(t.Context(), service, settingsService, "live", "yfinance"); err == nil {
		t.Fatal("provider status failure after selection = nil")
	}

	healthProvider := &assistantStatusProvider{healthErr: context.Canceled}
	runtime, err := NewRuntime(RuntimeOptions{FutuProvider: healthProvider})
	if err != nil {
		t.Fatalf("NewRuntime(health error): %v", err)
	}
	healthService := marketdata.NewService(runtime)
	t.Cleanup(func() { _ = healthService.Close(); _ = runtime.Close() })
	healthSettings := settingssrv.NewService(&assistantProviderSettingsStore{
		active: jfsettings.MarketDataProviderFutu, backtest: jfsettings.MarketDataProviderYFinance,
	}, settingssrv.WithMarketDataProviderCatalog(ProviderCatalog(healthService)))
	result, err := AssistantMarketProviders(t.Context(), healthService, healthSettings)
	if err != nil {
		t.Fatalf("health error provider status: %v", err)
	}
	health, ok := result.(map[string]any)["liveHealth"].(marketdata.HealthStatus)
	if !ok || health.LastError == "" || health.Connected {
		t.Fatalf("live health status = %#v", result.(map[string]any)["liveHealth"])
	}

	activeErr := errors.New("active provider save failed")
	backtestErr := errors.New("backtest provider save failed")
	for _, test := range []struct {
		name  string
		scope string
		err   error
	}{
		{name: "live", scope: "live", err: activeErr},
		{name: "backtest", scope: "backtest", err: backtestErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &assistantProviderSettingsStore{
				active: jfsettings.MarketDataProviderFutu, backtest: jfsettings.MarketDataProviderYFinance,
				activeErr: activeErr, backtestErr: backtestErr,
			}
			settingsService := settingssrv.NewService(store, settingssrv.WithMarketDataProviderCatalog(func(context.Context) ([]marketdata.ProviderDescriptor, error) {
				return nil, nil
			}))
			_, err := SelectAssistantMarketProvider(t.Context(), service, settingsService, test.scope, "akshare")
			if !errors.Is(err, test.err) {
				t.Fatalf("selection error = %v, want %v", err, test.err)
			}
		})
	}
}
