package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	jfadk "github.com/jftrade/jftrade-main/pkg/adk"
)

func TestBacktestRunMaintenanceKeepsMemoryAndDatabaseInSync(t *testing.T) {
	store, err := newBacktestRunStoreWithDB(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	completed := &backtestRunState{ID: "completed", Status: "completed", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
	running := &backtestRunState{ID: "running", Status: "running", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
	if err := store.add(completed); err != nil {
		t.Fatal(err)
	}
	if err := store.add(running); err != nil {
		t.Fatal(err)
	}

	deleted, err := store.purgeTerminalRuns(t.Context(), []string{completed.ID})
	if err != nil || deleted != 1 {
		t.Fatalf("purge = %d, %v", deleted, err)
	}
	if _, ok := store.get(completed.ID); ok {
		t.Fatal("completed run remains in memory")
	}
	var count int
	if err := store.db.Get(&count, `SELECT COUNT(*) FROM backtest_runs WHERE id = ?`, completed.ID); err != nil || count != 0 {
		t.Fatalf("database count = %d err=%v", count, err)
	}
	if _, err := store.purgeTerminalRuns(t.Context(), []string{running.ID}); !errors.Is(err, datamigration.ErrPreviewStale) {
		t.Fatalf("running purge err = %v", err)
	}
	server := &Server{backtestRuns: store, backtestSyncTasks: newBacktestSyncTaskStore()}
	if reason := server.databaseMaintenanceBusyReason(datamigration.DatabaseBacktestRuns); reason == "" {
		t.Fatal("running backtest did not block maintenance")
	}
}

func TestDataManagementServerCleanupAndCompactionPaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("JFTRADE_BACKTEST_DB", filepath.Join(root, "backtest.db"))
	store, err := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, store)

	if _, err := server.designStore.db.Exec(`INSERT INTO strategy_design_definitions (id, name, version, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, deleted_at) VALUES ('cleanup-strategy', 'Cleanup', '0.1.0', '', 'pinets', 'pine-v6', 'US.AAPL', '1d', '//@version=6', '{}', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	previewValue, err := server.dataManagementSvc.PreviewCleanup(t.Context(), dmsrv.CleanupPreviewRequest{Kind: datamigration.CleanupSoftDeleted, DatabaseID: datamigration.DatabaseStrategy})
	if err != nil {
		t.Fatal(err)
	}
	preview := previewValue.(datamigration.CleanupPreview)
	resultValue, err := server.dataManagementSvc.ExecuteCleanup(t.Context(), dmsrv.CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText})
	if err != nil {
		t.Fatal(err)
	}
	if resultValue.(datamigration.CleanupResult).DeletedCount != 1 {
		t.Fatalf("strategy cleanup = %+v", resultValue)
	}

	agent, err := server.adkRuntime.Store().SaveAgent(t.Context(), jfadk.AgentWriteRequest{ID: "cleanup-agent", Name: "Cleanup Agent", Status: jfadk.AgentStatusEnabled})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.adkRuntime.Store().DeleteAgent(t.Context(), agent.ID); err != nil {
		t.Fatal(err)
	}
	previewValue, err = server.dataManagementSvc.PreviewCleanup(t.Context(), dmsrv.CleanupPreviewRequest{Kind: datamigration.CleanupSoftDeleted, DatabaseID: datamigration.DatabaseADK})
	if err != nil {
		t.Fatal(err)
	}
	preview = previewValue.(datamigration.CleanupPreview)
	if _, err := server.dataManagementSvc.ExecuteCleanup(t.Context(), dmsrv.CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText}); err != nil {
		t.Fatal(err)
	}

	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	for _, id := range []string{"old-a", "old-b"} {
		if err := server.backtestRuns.add(&backtestRunState{ID: id, Status: "completed", CreatedAt: old, UpdatedAt: old}); err != nil {
			t.Fatal(err)
		}
	}
	previewValue, err = server.dataManagementSvc.PreviewCleanup(t.Context(), dmsrv.CleanupPreviewRequest{Kind: datamigration.CleanupBacktestHistory, DatabaseID: datamigration.DatabaseBacktestRuns, OlderThanDays: 1, KeepLatest: 1})
	if err != nil {
		t.Fatal(err)
	}
	preview = previewValue.(datamigration.CleanupPreview)
	if preview.CandidateCount != 1 {
		t.Fatalf("backtest preview = %+v", preview)
	}
	if _, err := server.dataManagementSvc.ExecuteCleanup(t.Context(), dmsrv.CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText}); err != nil {
		t.Fatal(err)
	}

	for _, databaseID := range []string{datamigration.DatabaseBacktest, datamigration.DatabaseExecution, datamigration.DatabaseADKSession} {
		if _, err := server.dataManagementSvc.Compact(t.Context(), databaseID, dmsrv.CompactRequest{Confirmation: "COMPACT " + databaseID}); err != nil {
			t.Fatalf("compact %s: %v", databaseID, err)
		}
	}
}

func TestTranslateDataManagementErrors(t *testing.T) {
	tests := []struct{ input, target error }{
		{nil, nil},
		{datamigration.ErrMaintenanceConflict, dmsrv.ErrDatabaseMaintenanceConflict},
		{datamigration.ErrPreviewNotFound, dmsrv.ErrCleanupPreviewNotFound},
		{datamigration.ErrPreviewStale, dmsrv.ErrCleanupPreviewStale},
		{context.Canceled, context.Canceled},
	}
	for _, test := range tests {
		got := translateDataManagementError(test.input)
		if test.target == nil {
			if got != nil {
				t.Fatalf("translate nil = %v", got)
			}
		} else if !errors.Is(got, test.target) {
			t.Fatalf("translate %v = %v, want %v", test.input, got, test.target)
		}
	}
}
