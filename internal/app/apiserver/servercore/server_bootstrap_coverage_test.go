package servercore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
)

func TestServerBootstrapRemainingFailureAndFallbackPaths(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0o600); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(blocker, "settings.json")
	bootstrap := serverBootstrap{
		settingsPath:         settingsPath,
		backtestDBPath:       blocker,
		dataMigration:        datamigration.NewManager(settingsPath, blocker),
		unavailableDatabases: map[string]error{},
	}
	bootstrap.recordUnavailable("ignored", nil)
	bootstrap.probeBacktestDatabase()
	if bootstrap.unavailableDatabases[datamigration.DatabaseBacktest] == nil {
		t.Fatal("backtest probe failure was not recorded")
	}

	fallback := bootstrap.newFallbackStrategyStore()
	if fallback == nil || fallback.data.TargetDir == "" {
		t.Fatalf("fallback strategy store = %#v", fallback)
	}
	if got := bootstrap.loadStrategyStore(); got != nil {
		t.Fatalf("invalid strategy store = %#v, want nil", got)
	}
	if got := bootstrap.loadDesignStore(); got == nil || got.db != nil {
		t.Fatalf("invalid design fallback = %#v", got)
	}
	if got := bootstrap.loadBacktestRunStore(); got == nil || got.db != nil {
		t.Fatalf("invalid backtest run fallback = %#v", got)
	}
	if got := bootstrap.loadExecutionOrderStore(ExecutionSettings{SeenFillRetentionDays: 7}); got == nil || got.persistence != nil || got.seenFillRetentionDays != 7 {
		t.Fatalf("invalid execution fallback = %#v", got)
	}

	validSettings, err := NewSettingsStore(filepath.Join(root, "valid-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	state := bootstrap.loadPersistentState(validSettings)
	if state.strategyStore == nil || state.strategyStore.runtimeStore != nil || state.runtimeStore != nil {
		t.Fatalf("persistent fallback state = %#v", state)
	}
	if state.auth == nil {
		t.Fatal("persistent fallback auth = nil")
	}
	_ = state.strategyStore.Close()
	_ = state.designStore.Close()
	_ = state.backtestRunStore.Close()
	_ = state.executionOrderStore.Close()
}

func TestServerRemainingPublicSettersAndRuntimeBoundaries(t *testing.T) {
	var nilServer *Server
	nilServer.SetWebAccessReconfigure(nil)
	nilServer.SetAPIPort(1)
	nilServer.ConfigureAuthOrigins("http://example.test")
	nilServer.SetFrontendFS(nil, "")
	nilServer.ApplySecuritySettings(SecuritySettings{})

	server := &Server{}
	called := false
	server.SetWebAccessReconfigure(func(SecuritySettings) error {
		called = true
		return errors.New("reconfigure failed")
	})
	if server.webAccessReconfigure == nil {
		t.Fatal("web access reconfigure callback was not installed")
	}
	if err := server.settingsSideEffects().OnSecurityChanged(SecuritySettings{}); err == nil || !called {
		t.Fatalf("security side effect = %v, called=%v", err, called)
	}

	if got := server.liveWebSocketDemand(); got != nil {
		t.Fatalf("nil live websocket demand = %#v", got)
	}
	if got := server.strategyRuntimeDemand(); got != nil {
		t.Fatalf("nil strategy runtime demand = %#v", got)
	}
	server.startAssistantWorkflowScheduler()

	options := server.settingsServiceOptions()
	if len(options) == 0 {
		t.Fatal("settings service options are empty")
	}
	if err := server.settingsSideEffects().OnMCPServerChanged(MCPServerSettings{}); err == nil {
		t.Fatal("nil MCP manager change error = nil")
	}
	server.settingsSideEffects().OnExchangeCalendarsChanged(ExchangeCalendarSettings{})

	if persistenceOnlySettingsStore(nil) != nil {
		t.Fatal("nil persistence settings store became non-nil")
	}
}

func TestServerRemainingBrokerAndSystemOptionBoundaries(t *testing.T) {
	root := t.TempDir()
	settings, err := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	if _, err := server.futuExchangeOrError(); !errors.Is(err, errFutuIntegrationNotEnabled) {
		t.Fatalf("disabled Futu exchange error = %v", err)
	}
	if server.brokerExecutionExchange() != nil {
		t.Fatal("disabled broker execution exchange was non-nil")
	}

	bare := &Server{}
	core := bare.systemCoreOptions(settings.Path(), filepath.Join(root, "backtest.db"))
	runtime := bare.systemRuntimeOptions()
	if len(core) == 0 || len(runtime) == 0 {
		t.Fatalf("system options core/runtime = %d/%d", len(core), len(runtime))
	}
	bare.preTradeRiskGateway = nil
	_ = bare.systemRuntimeOptions()
}
