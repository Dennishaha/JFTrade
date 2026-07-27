package servercore

import (
	"context"
	"path/filepath"
	"testing"

	fututestkit "github.com/jftrade/jftrade-main/internal/integration/futu/testkit"
	jfsettings "github.com/jftrade/jftrade-main/pkg/jftsettings"
)

func TestFutuRuntimeReturnsLiveDiscoveredAccounts(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	opendServer.setAccounts([]fututestkit.Account{{
		Environment: "SIMULATE", ID: 1001, Markets: []string{"HK"}, Type: "CASH",
	}, {
		Environment: "REAL", ID: 2001, Markets: []string{"HK"}, Type: "MARGIN",
	}})
	defer opendServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, opendServer.addr)
	runtime := server.futuCoordinator().BrokerRuntime(t.Context())
	if got := runtime.Session.Connectivity; got != "connected" {
		t.Fatalf("broker runtime connectivity = %v, want connected", got)
	}
	if got := runtime.Session.AccountsDiscovered; got != 2 {
		t.Fatalf("accountsDiscovered = %v, want 2", got)
	}
	if len(runtime.Accounts) != 2 {
		t.Fatalf("discovered accounts = %#v", runtime.Accounts)
	}
	first := runtime.Accounts[0]
	if first.AccountID != "2001" || first.TradingEnvironment != "REAL" {
		t.Fatalf("first normalized discovered account = %#v", first)
	}
}

func TestFutuRuntimeFallsBackWhenConnectedProbeHasNoExchange(t *testing.T) {
	opendServer := startBrokerRouteOpenDServer(t)
	defer opendServer.stop()

	server := newMarketDataTestServerWithQuoteRuntime(t, opendServer.addr)
	if probe := server.futuCoordinator().Probe(t.Context()); probe.Connectivity != "connected" {
		t.Fatalf("precondition healthy probe = %#v", probe)
	}
	marketdataRuntime := server.runtimes.MarketData()
	server.runtimes.SetMarketData(nil)
	t.Cleanup(func() { _ = marketdataRuntime.Close() })

	runtime := server.futuCoordinator().BrokerRuntime(t.Context())
	if runtime.Session.Connectivity != "disconnected" || runtime.Session.AccountsDiscovered != 0 {
		t.Fatalf("runtime without exchange must stay safely empty: %#v", runtime)
	}
	if len(runtime.Accounts) != 0 {
		t.Fatalf("runtime without exchange accounts = %#v", runtime.Accounts)
	}
}

func TestFutuOnboardingDoesNotReportMissingEnabledManagedAccount(t *testing.T) {
	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.CreateManagedAccount(jfsettings.ManagedBrokerAccount{
		BrokerID: "futu", AccountID: "sim-1001", TradingEnvironment: "SIMULATE", Market: "HK", Enabled: true,
	}); err != nil {
		t.Fatalf("CreateManagedAccount: %v", err)
	}
	server := newTestServer(t, settings)
	stubHealthyNodeRuntimeForFutuProbe(t)

	state := server.futuCoordinator().OnboardingStateFromSettings(t.Context(), jfsettings.OnboardingSettings{})
	reasons, ok := state["reasons"].([]map[string]any)
	if !ok {
		t.Fatalf("onboarding reasons = %#v", state["reasons"])
	}
	for _, reason := range reasons {
		if reason["code"] == "NO_MANAGED_ACCOUNTS" {
			t.Fatalf("enabled account was treated as missing: %#v", reasons)
		}
	}
}

func stubHealthyNodeRuntimeForFutuProbe(t *testing.T) {
	t.Helper()
	previousLookPath := runtimeDependencyLookPath
	previousOutput := runtimeDependencyOutput
	runtimeDependencyLookPath = func(string) (string, error) { return "/test/node", nil }
	runtimeDependencyOutput = func(context.Context, string, ...string) ([]byte, error) { return []byte("v22.0.0"), nil }
	t.Cleanup(func() {
		runtimeDependencyLookPath = previousLookPath
		runtimeDependencyOutput = previousOutput
	})
}
