package servercore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	appstores "github.com/jftrade/jftrade-main/internal/app/apiserver/stores"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	backteststore "github.com/jftrade/jftrade-main/internal/store/backtest"
	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	strategystore "github.com/jftrade/jftrade-main/internal/store/strategy"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
)

func TestDataManagementBackendRemainingOperations(t *testing.T) {
	root := t.TempDir()
	t.Setenv("JFTRADE_BACKTEST_DB", filepath.Join(root, "backtest.db"))
	settings, err := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	backend := dataManagementBackend{manager: server.dataMigration}

	if overview, err := backend.Overview(t.Context(), dmsrv.OverviewRequest{SummaryOnly: true}); err != nil || overview == nil {
		t.Fatalf("Overview() = %#v, %v", overview, err)
	}
	if _, err := (dataManagementBackend{}).Backup(t.Context(), dmsrv.BackupRequest{}); err == nil {
		t.Fatal("nil-manager Backup error = nil")
	}
	if _, err := backend.Backup(t.Context(), dmsrv.BackupRequest{DatabaseID: datamigration.DatabaseWatchlist, Confirmation: "wrong"}); err == nil {
		t.Fatal("Backup accepted an invalid confirmation")
	}
	backup, err := backend.Backup(t.Context(), dmsrv.BackupRequest{
		DatabaseID:   datamigration.DatabaseWatchlist,
		Confirmation: datamigration.BackupConfirmationText(datamigration.DatabaseWatchlist),
	})
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if result, ok := backup.(dmsrv.BackupResult); !ok || result.DatabaseID != datamigration.DatabaseWatchlist || result.BackupPath == "" || result.SizeBytes <= 0 || result.CreatedAt == "" {
		t.Fatalf("Backup result = %#v", backup)
	}

	if _, err := backend.Rebuild(t.Context(), dmsrv.RebuildRequest{
		DatabaseID:   datamigration.DatabaseStrategy,
		Mode:         "single",
		Confirmation: "REBUILD " + datamigration.DatabaseStrategy,
	}); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}

	var nilServer *Server
	nilServer.configureDataManagement()
}

func TestDatabaseMaintenanceRemainingBusyReasons(t *testing.T) {
	syncTasks := newBacktestSyncTaskStore()
	syncTasks.Add("sync", nil, func() {})
	syncServer := &Server{serverApplication: serverApplication{
		stores: appstores.Handle{BacktestTasks: syncTasks},
	}}
	if reason := syncServer.newMaintenanceRegistry().BusyReason(t.Context(), datamigration.DatabaseBacktest); !strings.Contains(reason, "行情同步") {
		t.Fatalf("sync busy reason = %q", reason)
	}

	strategyServer := &Server{}
	strategyServer.runtimes.SetStrategyRuntime(nil, dmsrv.BusyCheckerFunc(func(context.Context) string {
		return "存在活动策略运行实例"
	}))
	if reason := strategyServer.newMaintenanceRegistry().BusyReason(t.Context(), datamigration.DatabaseStrategy); !strings.Contains(reason, "活动策略") {
		t.Fatalf("strategy busy reason = %q", reason)
	}

	orders := newExecutionOrderStore()
	orders.RecordPlacedOrder(trdsrv.ExecutionPlacedOrderRecord{
		InternalOrderID: "order",
		Status:          trdsrv.OrderStatusSubmitted,
	})
	executionServer := &Server{serverApplication: serverApplication{
		stores: appstores.Handle{ExecutionOrders: orders},
	}}
	if reason := executionServer.newMaintenanceRegistry().BusyReason(t.Context(), datamigration.DatabaseExecution); !strings.Contains(reason, "非终态") {
		t.Fatalf("execution busy reason = %q", reason)
	}

}

func TestDataManagementRemainingPurgeAndCompactBoundaries(t *testing.T) {
	ctx := t.Context()
	bare := &Server{}
	maintenance := bare.newMaintenanceRegistry()
	if _, err := maintenance.Purge(ctx, datamigration.DatabaseStrategy, []dmsrv.CleanupCandidate{{ID: "missing"}}); err == nil {
		t.Fatal("nil strategy store purge error = nil")
	}
	if _, err := maintenance.Purge(ctx, datamigration.DatabaseBacktestRuns, []dmsrv.CleanupCandidate{{ID: "missing"}}); err == nil {
		t.Fatal("nil backtest run store purge error = nil")
	}
	if _, err := maintenance.Purge(ctx, "unknown", nil); err == nil {
		t.Fatal("unknown database purge error = nil")
	}

	for _, databaseID := range []string{
		datamigration.DatabaseBacktestRuns,
		datamigration.DatabaseStrategy,
		datamigration.DatabaseExecution,
		datamigration.DatabaseWatchlist,
		datamigration.DatabaseResearch,
		"unknown",
	} {
		if err := maintenance.Compact(ctx, databaseID); err == nil {
			t.Fatalf("bare compact %q error = nil", databaseID)
		}
	}
	if bare.dataMigrationPath("missing") != "" {
		t.Fatal("missing data migration path was non-empty")
	}
	if statuses := mustDatabaseStatuses(nil); statuses != nil {
		t.Fatalf("nil manager statuses = %#v", statuses)
	}
	if err := (*sqliteconn.DB)(nil).Compact(ctx); err == nil {
		t.Fatal("nil compact database error = nil")
	}

	db, err := sqliteconn.Open(filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Compact(ctx); err == nil {
		t.Fatal("closed compact database error = nil")
	}
}

func TestDataManagementRemainingStorePurgeFailures(t *testing.T) {
	if _, err := (*strategystore.Store)(nil).PurgeMaintenanceCandidates(t.Context(), nil); err == nil {
		t.Fatal("nil strategy purge error = nil")
	}
	if _, err := (*backteststore.Store)(nil).PurgeMaintenanceCandidates(t.Context(), nil); err == nil {
		t.Fatal("nil backtest purge error = nil")
	}

	strategy, err := strategystore.New(filepath.Join(t.TempDir(), "strategy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := strategy.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := strategy.PurgeMaintenanceCandidates(
		t.Context(),
		[]dmsrv.CleanupCandidate{{ID: "id"}},
	); err == nil {
		t.Fatal("closed strategy purge error = nil")
	}

	runs, err := newBacktestRunStoreWithDB(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := runs.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.PurgeMaintenanceCandidates(
		t.Context(),
		[]dmsrv.CleanupCandidate{{ID: "id"}},
	); err == nil {
		t.Fatal("closed backtest database purge error = nil")
	}
}

func TestCompactBacktestRejectsInvalidDatabasePath(t *testing.T) {
	root := t.TempDir()
	directoryPath := filepath.Join(root, "database-directory")
	if err := os.Mkdir(directoryPath, 0o755); err != nil {
		t.Fatal(err)
	}
	manager := datamigration.NewManager(filepath.Join(root, "settings.json"), directoryPath)
	server := &Server{serverApplication: serverApplication{dataMigration: manager}}
	if err := server.newMaintenanceRegistry().Compact(t.Context(), datamigration.DatabaseBacktest); err == nil {
		t.Fatal("directory backtest path compact error = nil")
	}
}

func TestDataManagementStatusErrorIsIgnoredByPathLookup(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, "missing", "settings.json")
	manager := datamigration.NewManager(settingsPath, filepath.Join(root, "backtest.db"))
	markerPath := filepath.Join(filepath.Dir(settingsPath), datamigration.RebuildMarkerFilename)
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if statuses := mustDatabaseStatuses(manager); statuses != nil {
		t.Fatalf("errored statuses = %#v, want nil", statuses)
	}
	if got := (&Server{serverApplication: serverApplication{
		dataMigration: manager,
	}}).dataMigrationPath(datamigration.DatabaseStrategy); got != "" {
		t.Fatalf("path with errored statuses = %q", got)
	}
}
