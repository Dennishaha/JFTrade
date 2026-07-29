package application

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	btsrv "github.com/jftrade/jftrade-main/internal/backtest"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	mdsrv "github.com/jftrade/jftrade-main/internal/marketdata"
	productsrv "github.com/jftrade/jftrade-main/internal/productfeatures"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/system"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	"github.com/jftrade/jftrade-main/internal/watchlist"
)

type assistantSettingsStub struct {
	runtime     jfsettings.ADKRuntimeSettings
	accounts    []jfsettings.ManagedBrokerAccount
	integration jfsettings.BrokerIntegration
}

func (s assistantSettingsStub) ADKSettings() jfsettings.ADKRuntimeSettings {
	return s.runtime
}

func (s assistantSettingsStub) ManagedAccounts() []jfsettings.ManagedBrokerAccount {
	return s.accounts
}

func (s assistantSettingsStub) Integration() jfsettings.BrokerIntegration {
	return s.integration
}

type assistantHealthStub struct {
	status map[string]any
}

func (s assistantHealthStub) OpenDHealth(context.Context) map[string]any {
	return s.status
}

func TestAssistantPortsProjectSettingsAndHealth(t *testing.T) {
	settings := assistantSettingsStub{
		runtime:  jfsettings.ADKRuntimeSettings{RunTimeoutMs: 1250},
		accounts: []jfsettings.ManagedBrokerAccount{{ID: "paper-account"}},
		integration: jfsettings.BrokerIntegration{
			Enabled: true,
			Config:  jfsettings.FutuIntegrationConfig{TradeMarket: "US"},
		},
	}
	health := map[string]any{"status": "connected"}
	ports := AssistantPorts(AssistantOptions{
		Settings: settings,
		Health:   assistantHealthStub{status: health},
	})

	if got := ports.RuntimeSettings(); !reflect.DeepEqual(got, settings.runtime) {
		t.Fatalf("runtime settings = %#v, want %#v", got, settings.runtime)
	}
	if got := ports.ManagedAccounts(); !reflect.DeepEqual(got, settings.accounts) {
		t.Fatalf("managed accounts = %#v, want %#v", got, settings.accounts)
	}
	if got := ports.BrokerIntegration(); !reflect.DeepEqual(got, settings.integration) {
		t.Fatalf("broker integration = %#v, want %#v", got, settings.integration)
	}
	gotHealth, err := ports.FutuOpenDHealth(t.Context())
	if err != nil || !reflect.DeepEqual(gotHealth, health) {
		t.Fatalf("Futu health = %#v, %v, want %#v", gotHealth, err, health)
	}
}

func TestAssistantPortsAndPathsAreNilSafe(t *testing.T) {
	ports := AssistantPorts(AssistantOptions{})
	if got := ports.RuntimeSettings(); !reflect.DeepEqual(got, jfsettings.ADKRuntimeSettings{}) {
		t.Fatalf("nil runtime settings = %#v", got)
	}
	if got := ports.ManagedAccounts(); got != nil {
		t.Fatalf("nil managed accounts = %#v", got)
	}
	if got := ports.BrokerIntegration(); !reflect.DeepEqual(got, jfsettings.BrokerIntegration{}) {
		t.Fatalf("nil broker integration = %#v", got)
	}
	health, err := ports.FutuOpenDHealth(t.Context())
	if err != nil || !reflect.DeepEqual(health, map[string]any{"status": "unavailable"}) {
		t.Fatalf("nil Futu health = %#v, %v", health, err)
	}

	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	paths := AssistantPaths(settingsPath)
	if paths.Database == "" || paths.Session == "" || paths.Secrets == "" || paths.Skills == "" {
		t.Fatalf("derived Assistant paths are incomplete: %#v", paths)
	}
}

func TestAssistantCompositionOpensRuntimeAndProjectsServices(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	runtime, err := OpenAssistant(AssistantOptions{SettingsPath: settingsPath})
	if err != nil {
		t.Fatalf("OpenAssistant: %v", err)
	}
	if !runtime.Available() || runtime.Service() == nil {
		t.Fatal("opened Assistant runtime is unavailable")
	}

	systemService := &system.Service{}
	marketDataService := &mdsrv.Service{}
	strategyService := &stratsrv.Service{}
	tradingService := &trdsrv.Service{}
	backtestService := &btsrv.Service{}
	productFeaturesService := &productsrv.Service{}
	watchlistService := &watchlist.Service{}
	ports := AssistantPorts(AssistantOptions{
		Runtime:         runtime,
		System:          systemService,
		MarketData:      marketDataService,
		Strategy:        strategyService,
		Trading:         tradingService,
		Backtest:        backtestService,
		ProductFeatures: productFeaturesService,
		Watchlist:       watchlistService,
	})
	if ports.Runtime() != runtime || ports.System() != systemService || ports.MarketData() != marketDataService ||
		ports.Strategy() != strategyService || ports.Trading() != tradingService || ports.Backtest() != backtestService ||
		ports.ProductFeatures() != productFeaturesService || ports.Watchlist() != watchlistService {
		t.Fatal("Assistant service providers did not preserve application dependencies")
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("close Assistant runtime: %v", err)
	}

	if probe := InspectAssistantRuntimeDatabase(settingsPath); probe.OpenError != nil || probe.CloseError != nil {
		t.Fatalf("runtime database probe = %#v", probe)
	}
	if probe := InspectAssistantSessionDatabase(settingsPath); probe.OpenError != nil || probe.CloseError != nil {
		t.Fatalf("session database probe = %#v", probe)
	}
}
