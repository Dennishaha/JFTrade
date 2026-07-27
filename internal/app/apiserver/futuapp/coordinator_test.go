package futuapp

import (
	"context"
	"reflect"
	"testing"
	"time"

	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	"github.com/jftrade/jftrade-main/pkg/broker"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
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
	diagnostics := health["localSocketDiagnostics"].(map[string]any)
	if diagnostics["liveQuoteBackoffActive"] != true || diagnostics["liveQuoteFailureCount"] != 3 {
		t.Fatalf("retry diagnostics = %#v", diagnostics)
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
