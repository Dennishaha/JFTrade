package servercore

import (
	"context"
	"path/filepath"
	"testing"

	settingssvc "github.com/jftrade/jftrade-main/internal/settings"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	"github.com/jftrade/jftrade-main/internal/system"
)

func TestServerSystemAndSettingsOptionCallbacks(t *testing.T) {
	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, settings)
	server.tradingSvc = nil

	options := append(systemCoreOptions(server, settings.Path(), deriveBacktestDBPath()), systemRuntimeOptions(server)...)
	service := system.NewService(options...)
	if got := service.Status(); got.Name != "JFTrade" {
		t.Fatalf("system status = %#v", got)
	}
	if got := service.BrokerOrderUpdatesSnapshot(); len(got) != 0 {
		t.Fatalf("nil trading order snapshot = %#v", got)
	}
	if got := service.FutuOpenDHealth(context.Background()); got["status"] == nil {
		t.Fatalf("Futu health = %#v", got)
	}
	if got := service.FutuOpenDInstallGuide(); got["brokerId"] != "futu" {
		t.Fatalf("Futu install guide = %#v", got)
	}
	service.ResetFutuRuntime()
	if got := service.RuntimeDependencies(context.Background()); got["allRequiredSatisfied"] == nil {
		t.Fatalf("runtime dependencies = %#v", got)
	}
	if got := service.RealTradeApprovals(); got.RealTradingEnabled {
		t.Fatalf("nil risk gateway state = %#v", got)
	}

	settingsService := settingssvc.NewService(settings, settingsServiceOptions(server)...)
	if got := settingsService.BrokerSettings(); got == nil {
		t.Fatal("broker settings callback returned nil")
	}
	if got := settingsService.OnboardingState(context.Background()); got["recommendedBrokerId"] != "futu" {
		t.Fatalf("onboarding callback = %#v", got)
	}
	_ = settingsService.GetMCPServerSettingsSnapshot()
}

func TestServerStrategyDemandWithRuntimeManager(t *testing.T) {
	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	server := newTestServer(t, settings)
	stub := newStrategyRuntimeStubExchange()
	installStrategyRuntimeTestExchange(server, stub)
	instanceID := instantiateStrategyRuntimeTestInstance(t, server, stratsrv.InstanceBinding{
		Symbols:       []string{"US.AAPL"},
		Interval:      "1m",
		ExecutionMode: strategyExecutionModeNotifyOnly,
	})
	instance, ok := server.stores.StrategyCatalog.GetInstance(instanceID)
	if !ok {
		t.Fatalf("strategy instance %q not found", instanceID)
	}
	if err := server.runtimes.StrategyRuntime().Start(t.Context(), instance); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer server.runtimes.StrategyRuntime().Stop(instanceID)
	if got := strategyRuntimeDemand(server); len(got) != 1 || got[0] != "US.AAPL" {
		t.Fatalf("strategy runtime demand = %#v", got)
	}
}
