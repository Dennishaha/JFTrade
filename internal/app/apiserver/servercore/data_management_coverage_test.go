package servercore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	trdsrv "github.com/jftrade/jftrade-main/internal/trading"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
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
	syncTasks.cancels["sync"] = func() {}
	if reason := (&Server{backtestSyncTasks: syncTasks}).databaseMaintenanceBusyReason(datamigration.DatabaseBacktest); !strings.Contains(reason, "行情同步") {
		t.Fatalf("sync busy reason = %q", reason)
	}

	runtimes := &strategyRuntimeManager{runtimes: map[string]*managedStrategyRuntime{"runtime": {}}, starting: map[string]struct{}{}}
	if reason := (&Server{strategyRuntimeManager: runtimes}).databaseMaintenanceBusyReason(datamigration.DatabaseStrategy); !strings.Contains(reason, "活动策略") {
		t.Fatalf("strategy busy reason = %q", reason)
	}

	orders := newExecutionOrderStore()
	orders.orders["order"] = trdsrv.ExecutionOrder{InternalOrderID: "order", Status: "submitted"}
	if reason := (&Server{executionOrders: orders}).databaseMaintenanceBusyReason(datamigration.DatabaseExecution); !strings.Contains(reason, "非终态") {
		t.Fatalf("execution busy reason = %q", reason)
	}

	root := t.TempDir()
	settings, err := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	if err := server.adkRuntime.Store().SaveRun(t.Context(), jfadk.Run{ID: "active-run", Status: jfadk.RunStatusRunning}); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}
	if reason := server.databaseMaintenanceBusyReason(datamigration.DatabaseADK); !strings.Contains(reason, "ADK") {
		t.Fatalf("ADK busy reason = %q", reason)
	}
}

func TestDataManagementRemainingPurgeAndCompactBoundaries(t *testing.T) {
	ctx := t.Context()
	bare := &Server{}
	if _, err := bare.purgeDatabaseCandidates(ctx, datamigration.DatabaseStrategy, []datamigration.CleanupCandidate{{ID: "missing"}}); err == nil {
		t.Fatal("nil strategy store purge error = nil")
	}
	if _, err := bare.purgeDatabaseCandidates(ctx, datamigration.DatabaseADK, nil); err == nil {
		t.Fatal("nil ADK store purge error = nil")
	}
	if _, err := bare.purgeDatabaseCandidates(ctx, datamigration.DatabaseBacktestRuns, []datamigration.CleanupCandidate{{ID: "missing"}}); err == nil {
		t.Fatal("nil backtest run store purge error = nil")
	}
	if _, err := bare.purgeDatabaseCandidates(ctx, "unknown", nil); err == nil {
		t.Fatal("unknown database purge error = nil")
	}

	for _, databaseID := range []string{
		datamigration.DatabaseBacktestRuns,
		datamigration.DatabaseStrategy,
		datamigration.DatabaseExecution,
		datamigration.DatabaseADK,
		datamigration.DatabaseADKSession,
		"unknown",
	} {
		if err := bare.compactDatabase(ctx, databaseID); err == nil {
			t.Fatalf("bare compact %q error = nil", databaseID)
		}
	}
	if bare.dataMigrationPath("missing") != "" {
		t.Fatal("missing data migration path was non-empty")
	}
	if statuses := mustDatabaseStatuses(nil); statuses != nil {
		t.Fatalf("nil manager statuses = %#v", statuses)
	}
	if err := compactSQLX(ctx, nil); err == nil {
		t.Fatal("nil compact database error = nil")
	}

	db, err := sqliteconn.Open(filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compactSQLX(ctx, db); err == nil {
		t.Fatal("closed compact database error = nil")
	}
}

func TestDataManagementRemainingADKPurgeMappingsAndErrors(t *testing.T) {
	root := t.TempDir()
	settings, err := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)
	candidates := []datamigration.CleanupCandidate{
		{ID: "missing-agent", Category: "智能体"},
		{ID: "missing-workflow", Category: "工作流"},
		{ID: "missing-trigger", Category: "触发器"},
	}
	if _, err := server.purgeDatabaseCandidates(t.Context(), datamigration.DatabaseADK, candidates); !errors.Is(err, datamigration.ErrPreviewStale) {
		t.Fatalf("changed ADK candidates error = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := server.purgeDatabaseCandidates(canceled, datamigration.DatabaseADK, []datamigration.CleanupCandidate{{ID: "missing-agent", Category: "智能体"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled ADK purge error = %v", err)
	}
}

func TestDataManagementRemainingStorePurgeFailures(t *testing.T) {
	if _, err := (*strategyDesignStore)(nil).purgeDeletedDefinitions(t.Context(), nil); err == nil {
		t.Fatal("nil strategy purge error = nil")
	}
	if _, err := (*backtestRunStore)(nil).purgeTerminalRuns(t.Context(), nil); err == nil {
		t.Fatal("nil backtest purge error = nil")
	}

	strategy, err := NewStrategyDesignStore(filepath.Join(t.TempDir(), "strategy.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := strategy.db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := strategy.purgeDeletedDefinitions(t.Context(), []string{"id"}); err == nil {
		t.Fatal("closed strategy purge error = nil")
	}

	runs, err := newBacktestRunStoreWithDB(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runs.db.Exec(`DROP TABLE ` + backtestRunTable); err != nil {
		t.Fatal(err)
	}
	if _, err := runs.purgeTerminalRuns(t.Context(), []string{"id"}); err == nil {
		t.Fatal("missing backtest table purge error = nil")
	}
	_ = runs.Close()
}

func TestCompactBacktestRejectsInvalidDatabasePath(t *testing.T) {
	root := t.TempDir()
	directoryPath := filepath.Join(root, "database-directory")
	if err := os.Mkdir(directoryPath, 0o755); err != nil {
		t.Fatal(err)
	}
	manager := datamigration.NewManager(filepath.Join(root, "settings.json"), directoryPath)
	server := &Server{dataMigration: manager}
	if err := server.compactDatabase(t.Context(), datamigration.DatabaseBacktest); err == nil {
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
	if got := (&Server{dataMigration: manager}).dataMigrationPath(datamigration.DatabaseStrategy); got != "" {
		t.Fatalf("path with errored statuses = %q", got)
	}
}
