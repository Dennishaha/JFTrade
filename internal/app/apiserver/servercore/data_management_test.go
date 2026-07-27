package servercore

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/app/apiserver/datamigration"
	appstores "github.com/jftrade/jftrade-main/internal/app/apiserver/stores"
	assistant "github.com/jftrade/jftrade-main/internal/assistant"
	dmsrv "github.com/jftrade/jftrade-main/internal/datamanagement"
	stratsrv "github.com/jftrade/jftrade-main/internal/strategy"
	strategydefinition "github.com/jftrade/jftrade-main/pkg/strategy/definition"
)

func TestBacktestRunMaintenanceKeepsMemoryAndDatabaseInSync(t *testing.T) {
	store, err := newBacktestRunStoreWithDB(filepath.Join(t.TempDir(), "runs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	completed := &backtestRunState{ID: "completed", Status: "completed", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
	running := &backtestRunState{ID: "running", Status: "running", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
	if err := store.Add(completed); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(running); err != nil {
		t.Fatal(err)
	}

	deleted, err := store.PurgeMaintenanceCandidates(
		t.Context(),
		[]dmsrv.CleanupCandidate{{ID: completed.ID}},
	)
	if err != nil || deleted != 1 {
		t.Fatalf("purge = %d, %v", deleted, err)
	}
	if _, ok := store.Get(completed.ID); ok {
		t.Fatal("completed run remains in memory")
	}
	if _, err := store.PurgeMaintenanceCandidates(
		t.Context(),
		[]dmsrv.CleanupCandidate{{ID: running.ID}},
	); !errors.Is(err, dmsrv.ErrCleanupCandidatesChanged) {
		t.Fatalf("running purge err = %v", err)
	}
	server := &Server{serverApplication: serverApplication{
		stores: appstores.Handle{
			BacktestRuns:  store,
			BacktestTasks: newBacktestSyncTaskStore(),
		},
	}}
	if reason := server.newMaintenanceRegistry().BusyReason(t.Context(), datamigration.DatabaseBacktestRuns); reason == "" {
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

	created, err := server.stores.Design.SaveDefinition(stratsrv.Definition{
		ID: "cleanup-strategy", Name: "Cleanup", Runtime: strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Cleanup\")",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.stores.Design.DeleteDefinition(created.ID); err != nil {
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

	adkStore := serverADKTestStore(t, server)
	agent, err := adkStore.SaveAgent(t.Context(), assistant.AgentWriteRequest{ID: "cleanup-agent", Name: "Cleanup Agent", Status: assistant.AgentStatusEnabled})
	if err != nil {
		t.Fatal(err)
	}
	if err := adkStore.DeleteAgent(t.Context(), agent.ID); err != nil {
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
		if err := server.stores.BacktestRuns.Add(&backtestRunState{ID: id, Status: "completed", CreatedAt: old, UpdatedAt: old}); err != nil {
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

	for _, databaseID := range []string{
		datamigration.DatabaseBacktest,
		datamigration.DatabaseBacktestRuns,
		datamigration.DatabaseStrategy,
		datamigration.DatabaseExecution,
		datamigration.DatabaseADK,
		datamigration.DatabaseADKSession,
		datamigration.DatabaseADKArtifact,
		datamigration.DatabaseWatchlist,
		datamigration.DatabaseResearch,
	} {
		if _, err := server.dataManagementSvc.Compact(t.Context(), databaseID, dmsrv.CompactRequest{Confirmation: "COMPACT " + databaseID}); err != nil {
			t.Fatalf("compact %s: %v", databaseID, err)
		}
	}
}

func TestDataManagementAdaptersRejectBusyRuntimeAndMapStalePreview(t *testing.T) {
	root := t.TempDir()
	t.Setenv("JFTRADE_BACKTEST_DB", filepath.Join(root, "backtest.db"))
	settings, err := NewSettingsStore(filepath.Join(root, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := newTestServer(t, settings)

	server.runtimes.SetStrategyRuntime(nil, dmsrv.BusyCheckerFunc(func(context.Context) string {
		return "存在活动策略运行实例"
	}))
	server.configureDataManagement()
	if _, err := server.dataManagementSvc.Compact(
		t.Context(),
		datamigration.DatabaseStrategy,
		dmsrv.CompactRequest{Confirmation: "COMPACT " + datamigration.DatabaseStrategy},
	); !errors.Is(err, dmsrv.ErrDatabaseMaintenanceConflict) {
		t.Fatalf("busy compact error = %v", err)
	}
	server.runtimes.SetStrategyRuntime(
		server.runtimes.StrategyRuntime(),
		server.runtimes.StrategyRuntime(),
	)
	server.configureDataManagement()

	created, err := server.stores.Design.SaveDefinition(stratsrv.Definition{
		ID:           "stale-strategy",
		Name:         "Stale",
		Runtime:      strategyRuntimePinePlan,
		SourceFormat: strategydefinition.SourceFormatPineV6,
		Script:       "//@version=6\nstrategy(\"Stale\")",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.stores.Design.DeleteDefinition(created.ID); err != nil {
		t.Fatal(err)
	}
	previewValue, err := server.dataManagementSvc.PreviewCleanup(
		t.Context(),
		dmsrv.CleanupPreviewRequest{
			Kind:       datamigration.CleanupSoftDeleted,
			DatabaseID: datamigration.DatabaseStrategy,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	preview := previewValue.(datamigration.CleanupPreview)
	if _, err := server.stores.Design.PurgeMaintenanceCandidates(
		t.Context(),
		[]dmsrv.CleanupCandidate{{ID: created.ID}},
	); err != nil {
		t.Fatal(err)
	}
	if _, err := server.dataManagementSvc.ExecuteCleanup(
		t.Context(),
		dmsrv.CleanupExecuteRequest{
			PreviewID:    preview.PreviewID,
			Confirmation: preview.ConfirmationText,
		},
	); !errors.Is(err, dmsrv.ErrCleanupPreviewStale) {
		t.Fatalf("stale cleanup error = %v", err)
	}
}

func TestTranslateDataManagementErrors(t *testing.T) {
	tests := []struct{ input, target error }{
		{nil, nil},
		{datamigration.ErrMaintenanceConflict, dmsrv.ErrDatabaseMaintenanceConflict},
		{datamigration.ErrPreviewNotFound, dmsrv.ErrCleanupPreviewNotFound},
		{datamigration.ErrPreviewStale, dmsrv.ErrCleanupPreviewStale},
		{datamigration.ErrBackupRateLimited, dmsrv.ErrBackupRateLimited},
		{datamigration.ErrBackupQuotaExceeded, dmsrv.ErrBackupQuotaExceeded},
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
