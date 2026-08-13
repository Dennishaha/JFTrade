package servercore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/futuapp"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
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
		unavailableDatabases: dmsrv.NewAvailabilitySnapshot(),
	}
	bootstrap.recordUnavailable(dmsrv.DatabaseID("ignored"), nil)
	bootstrap.probeBacktestDatabase()
	if bootstrap.unavailableDatabases.Unavailable(dmsrv.DatabaseBacktest) == nil {
		t.Fatal("backtest probe failure was not recorded")
	}

	fallbackCatalog := bootstrap.loadStrategyCatalog()
	if fallbackCatalog == nil || fallbackCatalog.Available() {
		t.Fatalf("fallback strategy catalog = %#v", fallbackCatalog)
	}
	if got := bootstrap.loadDesignStore(); got == nil || got.Available() {
		t.Fatalf("invalid design fallback = %#v", got)
	}
	if got := bootstrap.loadBacktestRunStore(); got == nil || got.Available() {
		t.Fatalf("invalid backtest run fallback = %#v", got)
	}
	if got := bootstrap.loadExecutionOrderStore(jfsettings.ExecutionSettings{SeenFillRetentionDays: 7}); got == nil || got.Available() || got.SeenFillRetentionDays() != 7 {
		t.Fatalf("invalid execution fallback = %#v", got)
	}

	validSettings, err := NewSettingsStore(filepath.Join(root, "valid-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	state := bootstrap.loadPersistentState(validSettings)
	if state.stores.StrategyCatalog == nil || state.stores.StrategyCatalog.Available() {
		t.Fatalf("persistent fallback state = %#v", state)
	}
	if state.auth == nil {
		t.Fatal("persistent fallback auth = nil")
	}
	if err := state.resources.Close(); err != nil {
		t.Fatalf("close persistent state: %v", err)
	}
}

func TestServerRemainingPublicSettersAndRuntimeBoundaries(t *testing.T) {
	var nilServer *Server
	nilServer.SetWebAccessReconfigure(nil)
	nilServer.SetAPIPort(1)
	nilServer.ConfigureAuthOrigins("http://example.test")
	nilServer.SetFrontendFS(nil, "")
	nilServer.ApplySecuritySettings(jfsettings.SecuritySettings{})

	server := &Server{}
	called := false
	server.SetWebAccessReconfigure(func(jfsettings.SecuritySettings) error {
		called = true
		return errors.New("reconfigure failed")
	})
	if server.webAccessReconfigure == nil {
		t.Fatal("web access reconfigure callback was not installed")
	}
	if err := settingsSideEffects(server).OnSecurityChanged(jfsettings.SecuritySettings{}); err == nil || !called {
		t.Fatalf("security side effect = %v, called=%v", err, called)
	}

	if got := liveWebSocketDemand(server); got != nil {
		t.Fatalf("nil live websocket demand = %#v", got)
	}
	if got := strategyRuntimeDemand(server); got != nil {
		t.Fatalf("nil strategy runtime demand = %#v", got)
	}
	startAssistantWorkflowScheduler(server)

	options := settingsServiceOptions(server)
	if len(options) == 0 {
		t.Fatal("settings service options are empty")
	}
	if err := settingsSideEffects(server).OnMCPServerChanged(jfsettings.MCPServerSettings{}); err == nil {
		t.Fatal("nil MCP manager change error = nil")
	}
	settingsSideEffects(server).OnExchangeCalendarsChanged(jfsettings.ExchangeCalendarSettings{})

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
	if _, err := futuapp.ExchangeOrError(server.futuCoordinator()); !errors.Is(err, futuapp.ErrFutuIntegrationNotEnabled) {
		t.Fatalf("disabled Futu exchange error = %v", err)
	}
	if brokerExecutionExchangeFor(&server.serverApplication) != nil {
		t.Fatal("disabled broker execution exchange was non-nil")
	}

	bare := &Server{}
	core := systemCoreOptions(bare, settings.Path(), filepath.Join(root, "backtest.db"))
	runtime := systemRuntimeOptions(bare)
	if len(core) == 0 || len(runtime) == 0 {
		t.Fatalf("system options core/runtime = %d/%d", len(core), len(runtime))
	}
	bare.runtimes.SetRealTradeControl(nil, nil)
	_ = systemRuntimeOptions(bare)
}
