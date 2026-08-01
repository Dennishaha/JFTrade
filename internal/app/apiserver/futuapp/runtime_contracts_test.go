package futuapp

import (
	"context"
	"net"
	"strconv"
	"testing"

	fututestkit "github.com/jftrade/jftrade-main/internal/integration/futu/testkit"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	"github.com/jftrade/jftrade-main/pkg/broker"
)

func TestCoordinatorProjectsConnectedRuntimeAndDiscoveredAccounts(t *testing.T) {
	opend := fututestkit.StartBrokerServer(t)
	defer opend.Close()
	opend.SetServerVersion(1009, 6908)
	coordinator, registry := coordinatorForOpenD(t, opend.Addr())
	accounts := []broker.Account{
		{ID: "2001", BrokerID: "futu", TradingEnvironment: "REAL", Market: "HK", AccountType: "MARGIN"},
		{ID: "1001", BrokerID: "futu", TradingEnvironment: "SIMULATE", Market: "HK", AccountType: "CASH"},
	}
	registry.Replace(coordinatorRuntimeBroker{accounts: accounts})

	runtime := coordinator.BrokerRuntime(t.Context())
	if runtime.Session.Connectivity != "connected" {
		t.Fatalf("connectivity = %q, want connected", runtime.Session.Connectivity)
	}
	if runtime.Session.AccountsDiscovered != len(accounts) || len(runtime.Accounts) != len(accounts) {
		t.Fatalf("runtime accounts = %#v, want %d", runtime.Accounts, len(accounts))
	}
	if runtime.Accounts[0].AccountID != "2001" || runtime.Accounts[0].TradingEnvironment != "REAL" {
		t.Fatalf("first account = %#v, want real account first", runtime.Accounts[0])
	}
	if runtime.Session.GlobalState == nil || runtime.Session.GlobalState.ServerVersion == nil {
		t.Fatalf("global state = %#v, want OpenD version projection", runtime.Session.GlobalState)
	}
	health, err := coordinator.MarketDataHealth(t.Context())
	if err != nil || !health.Connected || health.LastError != "" {
		t.Fatalf("market-data health = %#v, %v", health, err)
	}
}

func TestCoordinatorConnectedProbeWithoutBrokerFailsClosed(t *testing.T) {
	opend := fututestkit.StartBrokerServer(t)
	defer opend.Close()
	coordinator, _ := coordinatorForOpenD(t, opend.Addr())

	if probe := coordinator.Probe(t.Context()); probe.Connectivity != "connected" {
		t.Fatalf("probe = %#v, want connected fixture", probe)
	}
	runtime := coordinator.BrokerRuntime(t.Context())
	if runtime.Session.Connectivity != "disconnected" || runtime.Session.AccountsDiscovered != 0 || len(runtime.Accounts) != 0 {
		t.Fatalf("runtime without broker = %#v, want empty disconnected projection", runtime)
	}
}

func TestCoordinatorEnabledClosedPortReportsManualRetryDiagnosis(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	coordinator, _ := coordinatorForOpenD(t, address)

	health := coordinator.OpenDHealth(t.Context())
	diagnosis, ok := health["diagnosis"].(map[string]any)
	if !ok || diagnosis["code"] != "OPEND_API_CONNECTIVITY" || diagnosis["manualRetryRequired"] != true {
		t.Fatalf("health diagnosis = %#v, want connectivity/manual retry", health)
	}
	if runtime := coordinator.BrokerRuntime(t.Context()); runtime.Session.Connectivity != "disconnected" {
		t.Fatalf("runtime connectivity = %q, want disconnected", runtime.Session.Connectivity)
	}
	marketHealth, err := coordinator.MarketDataHealth(t.Context())
	if err != nil || marketHealth.Connected || marketHealth.LastError == "" {
		t.Fatalf("disconnected market-data health = %#v, %v", marketHealth, err)
	}
}

func coordinatorForOpenD(t *testing.T, address string) (*Coordinator, *broker.Registry) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split OpenD address %q: %v", address, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("parse OpenD port %q: %v", portText, err)
	}
	registry := broker.NewRegistry()
	return New(Options{
		Settings: coordinatorTestSettings{
			integration: jfsettings.BrokerIntegration{
				Enabled: true,
				Config:  jfsettings.FutuIntegrationConfig{Type: "futu", Host: host, APIPort: port},
			},
		},
		Registry: registry,
	}), registry
}

type coordinatorRuntimeBroker struct {
	accounts []broker.Account
}

func (b coordinatorRuntimeBroker) ID() string { return "futu" }

func (b coordinatorRuntimeBroker) Descriptor() broker.Descriptor {
	return broker.Descriptor{ID: "futu"}
}

func (b coordinatorRuntimeBroker) DiscoverAccounts(context.Context) ([]broker.Account, error) {
	return append([]broker.Account(nil), b.accounts...), nil
}

func (b coordinatorRuntimeBroker) Trading() broker.TradingService { return nil }

func (b coordinatorRuntimeBroker) MarketData() broker.MarketDataReader { return nil }
