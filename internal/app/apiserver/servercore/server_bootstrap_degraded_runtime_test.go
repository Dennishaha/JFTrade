package servercore

import (
	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	"github.com/jftrade/jftrade-main/internal/app/apiserver/futuapp"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	jfsettings "github.com/jftrade/jftrade-main/internal/jftsettings"
	settingssvc "github.com/jftrade/jftrade-main/internal/settings"
	"github.com/jftrade/jftrade-main/internal/system"
	"os"
	"path/filepath"
	"testing"
)

func TestServerBootstrapPersistsUnavailableDatabaseReasons(t *testing.T) {
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
	bootstrap.probeADKDatabase()
	bootstrap.probeADKSessionDatabase()
	for _, databaseID := range []dmsrv.DatabaseID{dmsrv.DatabaseADK, dmsrv.DatabaseADKSession} {
		if bootstrap.unavailableDatabases.Unavailable(databaseID) == nil {
			t.Fatalf("%s probe failure was not persisted", databaseID)
		}
	}

	failedInspection := &Server{
		serverApplication: serverApplication{
			RouteDependencies: RouteDependencies{
				dataMigration:        datamigration.NewManager(filepath.Join(root, "failed-settings.json"), filepath.Join(root, "failed-backtest.db")),
				unavailableDatabases: dmsrv.NewAvailabilitySnapshot(),
			},
		},
	}
	if err := os.WriteFile(filepath.Join(root, datamigration.RebuildMarkerFilename), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	refreshUnavailableDatabaseStatuses(failedInspection)
	if len(failedInspection.unavailableDatabases) != 0 {
		t.Fatalf("failed status inspection must not manufacture database states: %#v", failedInspection.unavailableDatabases)
	}

	missingRoot := t.TempDir()
	missingData := &Server{
		serverApplication: serverApplication{
			RouteDependencies: RouteDependencies{
				dataMigration:        datamigration.NewManager(filepath.Join(missingRoot, "settings.json"), filepath.Join(missingRoot, "backtest.db")),
				unavailableDatabases: dmsrv.NewAvailabilitySnapshot(),
			},
		},
	}
	refreshUnavailableDatabaseStatuses(missingData)
	if reason := missingData.unavailableDatabases.Unavailable(dmsrv.DatabaseBacktest); reason == nil || reason.Error() != "database was not initialized" {
		t.Fatalf("missing backtest reason = %v", reason)
	}
}

func TestServerOptionCallbacksExposeNilRuntimeStatesSafely(t *testing.T) {
	server := &Server{}
	riskService := system.NewService(systemRuntimeOptions(server)...)
	if limits := riskService.RealTradeRiskLimits(); limits.RiskEnabled || limits.Entry != nil {
		t.Fatalf("nil real-trade gateway limits = %#v", limits)
	}

	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	settingsService := settingssvc.NewService(settings, settingsServiceOptions(server)...)
	if snapshot := settingsService.GetMCPServerSettingsSnapshot(); snapshot.Status.Running || snapshot.Status.Endpoint != "" {
		t.Fatalf("nil MCP manager status = %#v", snapshot.Status)
	}
}

func TestServerBootstrapBuildsBrokerBridgeForEnabledIntegration(t *testing.T) {
	settings, err := NewSettingsStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.SaveIntegration(jfsettings.BrokerIntegration{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)

	bridge, ok := brokerExecutionExchangeFor(&server.serverApplication).(*strategyRuntimeBrokerBridge)
	if !ok || bridge == nil || bridge.RuntimeExchange == nil || bridge.broker == nil {
		t.Fatalf("enabled broker execution bridge = %#v", bridge)
	}
	if _, err := futuapp.ExchangeOrError(server.futuCoordinator()); err != nil {
		t.Fatalf("enabled Futu exchange: %v", err)
	}
	if _, err := futuapp.BrokerOrError(server.futuCoordinator()); err != nil {
		t.Fatalf("enabled Futu broker: %v", err)
	}
	settingsSideEffects(server).OnExchangeCalendarsChanged(jfsettings.ExchangeCalendarSettings{})
}
