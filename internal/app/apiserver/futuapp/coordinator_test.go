package futuapp

import (
	"context"
	"reflect"
	"testing"
	"time"

	futuintegration "github.com/jftrade/jftrade-main/internal/integration/futu"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestCoordinatorResetPreservesApplicationOrderAndInvalidatesFutu(t *testing.T) {
	registry := broker.NewRegistry()
	registry.Replace(coordinatorTestBroker{})
	events := []string{}
	coordinator := New(Options{
		Registry: registry,
		StopOrderUpdates: func() error {
			events = append(events, "stop-orders")
			return nil
		},
		ResetCollector:  func() { events = append(events, "reset-collector") },
		ResumeCollector: func() { events = append(events, "resume-collector") },
	})

	coordinator.Reset()

	if got, want := events, []string{"stop-orders", "reset-collector", "resume-collector"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reset order = %#v, want %#v", got, want)
	}
	if active := registry.Lookup("futu"); active != nil {
		t.Fatalf("reset retained Futu broker %T", active)
	}
}

func TestCoordinatorDisabledProjectionsAndRetryDiagnostics(t *testing.T) {
	future := time.Now().UTC().Add(time.Minute)
	coordinator := New(Options{
		Settings: coordinatorTestSettings{},
		LiveStreamStats: func() (int, int, bool) {
			return 2, 20, false
		},
		MarketDataState: func() mdsrv.RuntimeState {
			return mdsrv.RuntimeState{QuoteRetryAt: future, QuoteFailures: 3, QuoteLastError: "temporary"}
		},
	})

	runtime := coordinator.BrokerRuntime(t.Context())
	if runtime.Session.Connectivity != "disconnected" || runtime.Session.LiveWebSocketClients.Connected != 2 {
		t.Fatalf("disabled runtime = %#v", runtime)
	}
	health := coordinator.OpenDHealth(t.Context())
	if health["status"] != "offline" {
		t.Fatalf("disabled health = %#v", health)
	}
	marketHealth, err := coordinator.MarketDataHealth(t.Context())
	if err != nil || marketHealth.Connected || marketHealth.LastError == "" {
		t.Fatalf("disabled market-data health = %#v, %v", marketHealth, err)
	}
	diagnostics := health["localSocketDiagnostics"].(map[string]any)
	if diagnostics["liveQuoteBackoffActive"] != true || diagnostics["liveQuoteFailureCount"] != 3 {
		t.Fatalf("retry diagnostics = %#v", diagnostics)
	}
}

func TestMarketDataHealthRequiresHealthyOpenDQuoteSession(t *testing.T) {
	health := marketDataHealthFromProbe(true, futuintegration.Probe{
		Connectivity: "connected",
		Status:       "healthy",
	})
	if health.Connected || health.LastError != "Futu OpenD quote session status is unavailable" {
		t.Fatalf("unknown quote-session market-data health = %#v", health)
	}

	quoteLoggedOut := false
	health = marketDataHealthFromProbe(true, futuintegration.Probe{
		Connectivity:  "connected",
		Status:        "healthy",
		QuoteLoggedIn: &quoteLoggedOut,
	})
	if health.Connected || health.LastError != "Futu OpenD quote session is not logged in" {
		t.Fatalf("logged-out market-data health = %#v", health)
	}

	probeErr := "unsupported OpenD version"
	health = marketDataHealthFromProbe(true, futuintegration.Probe{
		Connectivity: "degraded",
		Status:       "degraded",
		LastError:    &probeErr,
	})
	if health.Connected || health.LastError != probeErr {
		t.Fatalf("degraded market-data health = %#v", health)
	}

	quoteLoggedIn := true
	health = marketDataHealthFromProbe(true, futuintegration.Probe{
		Connectivity:  "connected",
		Status:        "healthy",
		QuoteLoggedIn: &quoteLoggedIn,
	})
	if !health.Connected || health.LastError != "" {
		t.Fatalf("healthy market-data health = %#v", health)
	}
}

func TestCoordinatorOnboardingUsesRuntimeAndAccountReadiness(t *testing.T) {
	coordinator := New(Options{
		Settings: coordinatorTestSettings{
			integration: jfsettings.BrokerIntegration{Enabled: true},
			accounts: []jfsettings.ManagedBrokerAccount{{
				BrokerID: "futu", AccountID: "sim-1", Enabled: true,
			}},
		},
		RuntimeDependencies: func(context.Context) map[string]any {
			return map[string]any{"allRequiredSatisfied": true}
		},
	})

	state := coordinator.OnboardingStateFromSettings(t.Context(), jfsettings.OnboardingSettings{})
	reasons := state["reasons"].([]map[string]any)
	if len(reasons) != 0 || state["shouldShowOobe"] != false {
		t.Fatalf("ready onboarding state = %#v", state)
	}
}

type coordinatorTestSettings struct {
	integration jfsettings.BrokerIntegration
	saved       *jfsettings.BrokerIntegration
	accounts    []jfsettings.ManagedBrokerAccount
	onboarding  jfsettings.OnboardingSettings
}

func (s coordinatorTestSettings) Integration() jfsettings.BrokerIntegration {
	return s.integration
}

func (s coordinatorTestSettings) SavedIntegration() *jfsettings.BrokerIntegration {
	return s.saved
}

func (s coordinatorTestSettings) ManagedAccounts() []jfsettings.ManagedBrokerAccount {
	return s.accounts
}

func (s coordinatorTestSettings) Onboarding() jfsettings.OnboardingSettings {
	return s.onboarding
}

type coordinatorTestBroker struct{}

func (coordinatorTestBroker) ID() string { return "futu" }

func (coordinatorTestBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: "futu"}
}

func (coordinatorTestBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return nil, nil
}

func (coordinatorTestBroker) Trading() broker.TradingService {
	return nil
}

func (coordinatorTestBroker) MarketData() broker.MarketDataReader {
	return nil
}
