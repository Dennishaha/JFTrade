package servercore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	settingssvc "github.com/jftrade/jftrade-main/internal/settings"
	"github.com/jftrade/jftrade-main/internal/system"
)

func TestCoverage98ServerBootstrapPersistsUnavailableDatabaseReasons(t *testing.T) {
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
	bootstrap.probeADKDatabase()
	bootstrap.probeADKSessionDatabase()
	for _, databaseID := range []string{datamigration.DatabaseADK, datamigration.DatabaseADKSession} {
		if bootstrap.unavailableDatabases[databaseID] == nil {
			t.Fatalf("%s probe failure was not persisted", databaseID)
		}
	}

	failedInspection := &Server{
		dataMigration:        bootstrap.dataMigration,
		unavailableDatabases: map[string]error{},
	}
	failedInspection.refreshUnavailableDatabaseStatuses()
	if len(failedInspection.unavailableDatabases) != 0 {
		t.Fatalf("failed status inspection must not manufacture database states: %#v", failedInspection.unavailableDatabases)
	}

	missingData := &Server{
		dataMigration:        datamigration.NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db")),
		unavailableDatabases: map[string]error{},
	}
	missingData.refreshUnavailableDatabaseStatuses()
	if reason := missingData.unavailableDatabases[datamigration.DatabaseBacktest]; reason == nil || reason.Error() != "database was not initialized" {
		t.Fatalf("missing backtest reason = %v", reason)
	}
}

func TestCoverage98ServerOptionCallbacksExposeNilRuntimeStatesSafely(t *testing.T) {
	server := &Server{}
	riskService := system.NewService(server.systemRuntimeOptions()...)
	if limits := riskService.RealTradeRiskLimits(); limits["riskEnabled"] != false || limits["entry"] != nil {
		t.Fatalf("nil real-trade gateway limits = %#v", limits)
	}

	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	settingsService := settingssvc.NewService(settings, server.settingsServiceOptions()...)
	if snapshot := settingsService.GetMCPServerSettingsSnapshot(); snapshot.Status.Running || snapshot.Status.Endpoint != "" {
		t.Fatalf("nil MCP manager status = %#v", snapshot.Status)
	}
}

func TestCoverage98ServerBuildsBrokerBridgeForEnabledIntegration(t *testing.T) {
	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.SaveIntegration(BrokerIntegration{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)

	bridge, ok := server.brokerExecutionExchange().(*strategyRuntimeBrokerBridge)
	if !ok || bridge == nil || bridge.Exchange == nil || bridge.broker == nil {
		t.Fatalf("enabled broker execution bridge = %#v", bridge)
	}
	if _, err := server.futuExchangeOrError(); err != nil {
		t.Fatalf("enabled Futu exchange: %v", err)
	}
	if _, err := server.futuBrokerOrError(); err != nil {
		t.Fatalf("enabled Futu broker: %v", err)
	}
	server.settingsSideEffects().OnExchangeCalendarsChanged(ExchangeCalendarSettings{})
}
