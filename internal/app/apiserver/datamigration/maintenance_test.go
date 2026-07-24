package datamigration

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestOverviewCountsMainWALSHMAndKeepsDatabaseErrorsLocal(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))
	descriptor := manager.descriptorMap()[DatabaseBacktest]
	if err := os.WriteFile(descriptor.Path, make([]byte, 100), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(descriptor.Path+"-wal", make([]byte, 40), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(descriptor.Path+"-shm", make([]byte, 20), 0o600); err != nil {
		t.Fatal(err)
	}

	overview, err := manager.Overview(t.Context())
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}
	var found DatabaseOverview
	for _, database := range overview.Databases {
		if database.ID == DatabaseBacktest {
			found = database
		}
	}
	if found.Storage.MainBytes != 100 || found.Storage.WALBytes != 40 || found.Storage.SHMBytes < 20 || found.Storage.TotalBytes != found.Storage.MainBytes+found.Storage.WALBytes+found.Storage.SHMBytes {
		t.Fatalf("storage = %+v", found.Storage)
	}
	if found.Status != "incompatible" || overview.Totals.TotalBytes != found.Storage.TotalBytes {
		t.Fatalf("overview = %+v", overview)
	}
}

func TestOverviewSupportsSummaryOnlyAndSingleDatabase(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))
	createDeletedStrategyForMaintenanceTest(t, manager.descriptorMap()[DatabaseStrategy].Path)

	summary, err := manager.Overview(t.Context(), OverviewRequest{SummaryOnly: true})
	if err != nil {
		t.Fatalf("summary overview: %v", err)
	}
	if len(summary.Databases) != 8 || summary.Totals.TotalBytes != 0 {
		t.Fatalf("summary overview = %+v", summary)
	}
	for _, database := range summary.Databases {
		if database.Storage.TotalBytes != 0 || len(database.Cleanable) != 0 {
			t.Fatalf("summary database should not include heavy stats: %+v", database)
		}
	}

	single, err := manager.Overview(t.Context(), OverviewRequest{DatabaseID: DatabaseStrategy})
	if err != nil {
		t.Fatalf("single overview: %v", err)
	}
	if len(single.Databases) != 1 || single.Databases[0].ID != DatabaseStrategy {
		t.Fatalf("single overview databases = %+v", single.Databases)
	}
	if single.Totals.TotalBytes != single.Databases[0].Storage.TotalBytes || len(single.Databases[0].Cleanable) != 1 {
		t.Fatalf("single overview = %+v", single)
	}
	if _, err := manager.Overview(t.Context(), OverviewRequest{DatabaseID: "missing"}); err == nil {
		t.Fatal("unknown database id succeeded")
	}
}

func TestDatabaseBackupCreatesVerifiedPrivateSnapshot(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))
	now := time.Date(2026, time.July, 11, 15, 0, 0, 0, time.UTC)
	manager.maintenance.now = func() time.Time { return now }
	descriptor := manager.descriptorMap()[DatabaseWatchlist]
	initializeDescriptor(t, descriptor)
	db, err := sql.Open("sqlite", descriptor.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE backup_payload (value TEXT); INSERT INTO backup_payload(value) VALUES ('watchlist')`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Backup(t.Context(), DatabaseWatchlist, ""); err == nil {
		t.Fatal("backup without confirmation succeeded")
	}
	confirmation := BackupConfirmationText(DatabaseWatchlist)
	result, err := manager.Backup(t.Context(), DatabaseWatchlist, confirmation)
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if result.DatabaseID != DatabaseWatchlist || result.SizeBytes <= 0 || result.CreatedAt != now.Format(time.RFC3339Nano) {
		t.Fatalf("backup result = %+v", result)
	}
	if filepath.Dir(result.BackupPath) != filepath.Join(root, "backups") {
		t.Fatalf("backup path = %q", result.BackupPath)
	}
	info, err := os.Stat(result.BackupPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("backup permissions = %v", info.Mode().Perm())
	}
	backupDB, err := sql.Open("sqlite", result.BackupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = backupDB.Close() }()
	var value string
	if err := backupDB.QueryRow(`SELECT value FROM backup_payload`).Scan(&value); err != nil || value != "watchlist" {
		t.Fatalf("backup payload = %q err=%v", value, err)
	}

	lock := manager.maintenance.locks[DatabaseWatchlist]
	lock.Lock()
	_, conflictErr := manager.Backup(t.Context(), DatabaseWatchlist, confirmation)
	lock.Unlock()
	if !errors.Is(conflictErr, ErrMaintenanceConflict) {
		t.Fatalf("concurrent backup = %v", conflictErr)
	}
	if _, err := manager.Backup(t.Context(), DatabaseWatchlist, confirmation); !errors.Is(err, ErrBackupRateLimited) {
		t.Fatalf("repeated backup = %v, want rate limit", err)
	}
	if _, err := manager.Backup(t.Context(), "unknown", BackupConfirmationText("unknown")); err == nil {
		t.Fatal("unknown database backup succeeded")
	}
}

func TestBackupCapacityPrunesManagedFilesOnlyAndEnforcesQuota(t *testing.T) {
	root := t.TempDir()
	backupDir := filepath.Join(root, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))
	for index, name := range []string{
		"watchlist-20260711T140000.000000000Z-00000001.db",
		"watchlist-20260711T140100.000000000Z-00000002.db",
		"watchlist-20260711T140200.000000000Z-00000003.db",
	} {
		if err := os.WriteFile(filepath.Join(backupDir, name), make([]byte, index+3), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	unmanagedPath := filepath.Join(backupDir, "watchlist-not-managed.db")
	if err := os.WriteFile(unmanagedPath, make([]byte, 20), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.prepareBackupCapacity(backupDir, DatabaseWatchlist, 4, 10); err != nil {
		t.Fatalf("prepareBackupCapacity: %v", err)
	}
	files, err := manager.listManagedBackupFiles(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("managed backups after pruning = %d, want 1", len(files))
	}
	if _, err := os.Stat(unmanagedPath); err != nil {
		t.Fatalf("unmanaged file was removed: %v", err)
	}
	if err := manager.prepareBackupCapacity(backupDir, DatabaseWatchlist, 11, 10); !errors.Is(err, ErrBackupQuotaExceeded) {
		t.Fatalf("quota error = %v", err)
	}
}

func TestBacktestCleanupPreviewUsesAgeAndLatestProtection(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))
	now := time.Date(2026, time.July, 3, 12, 0, 0, 0, time.UTC)
	manager.maintenance.now = func() time.Time { return now }
	descriptor := manager.descriptorMap()[DatabaseBacktestRuns]
	createBacktestRunsForMaintenanceTest(t, descriptor.Path, now)

	preview, err := manager.PreviewCleanup(t.Context(), CleanupPreviewRequest{
		Kind: CleanupBacktestHistory, DatabaseID: DatabaseBacktestRuns,
		OlderThanDays: 30, KeepLatest: 20,
	})
	if err != nil {
		t.Fatalf("PreviewCleanup: %v", err)
	}
	if preview.CandidateCount != 5 || preview.ConfirmationText != "CLEANUP backtest-runs 5" {
		t.Fatalf("preview = %+v", preview)
	}

	var purged []CleanupCandidate
	manager.SetMaintenanceHooks(MaintenanceHooks{
		Purge: func(_ context.Context, id string, candidates []CleanupCandidate) (int, error) {
			if id != DatabaseBacktestRuns {
				t.Fatalf("database id = %q", id)
			}
			purged = append([]CleanupCandidate{}, candidates...)
			return len(candidates), nil
		},
		Compact: func(context.Context, string) error { return nil },
	})
	if _, err := manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: "wrong"}); err == nil {
		t.Fatal("wrong confirmation succeeded")
	}
	result, err := manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText})
	if err != nil {
		t.Fatalf("ExecuteCleanup: %v", err)
	}
	if result.DeletedCount != 5 || !result.Compacted || len(purged) != 5 {
		t.Fatalf("result = %+v purged=%d", result, len(purged))
	}
}

func TestCleanupPreviewExpiresAndMaintenanceConflictIsTyped(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))
	now := time.Date(2026, time.July, 3, 12, 0, 0, 0, time.UTC)
	manager.maintenance.now = func() time.Time { return now }
	descriptor := manager.descriptorMap()[DatabaseStrategy]
	createDeletedStrategyForMaintenanceTest(t, descriptor.Path)
	preview, err := manager.PreviewCleanup(t.Context(), CleanupPreviewRequest{Kind: CleanupSoftDeleted, DatabaseID: DatabaseStrategy})
	if err != nil {
		t.Fatal(err)
	}
	manager.SetMaintenanceHooks(MaintenanceHooks{BusyReason: func(string) string { return "active strategy" }})
	_, err = manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText})
	if !errors.Is(err, ErrMaintenanceConflict) {
		t.Fatalf("conflict err = %v", err)
	}

	preview, err = manager.PreviewCleanup(t.Context(), CleanupPreviewRequest{Kind: CleanupSoftDeleted, DatabaseID: DatabaseStrategy})
	if err != nil {
		t.Fatal(err)
	}
	manager.maintenance.now = func() time.Time { return now.Add(11 * time.Minute) }
	_, err = manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText})
	if !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("expiry err = %v", err)
	}
}

func TestOverviewCleanableCategoriesADKPreviewAndCompact(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))
	now := time.Date(2026, time.July, 3, 12, 0, 0, 0, time.UTC)
	manager.maintenance.now = func() time.Time { return now }
	createDeletedStrategyForMaintenanceTest(t, manager.descriptorMap()[DatabaseStrategy].Path)
	createBacktestRunsForMaintenanceTest(t, manager.descriptorMap()[DatabaseBacktestRuns].Path, now)
	createDeletedADKForMaintenanceTest(t, manager.descriptorMap()[DatabaseADK].Path)

	overview, err := manager.Overview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cleanable := map[string][]CleanableItem{}
	for _, database := range overview.Databases {
		cleanable[database.ID] = database.Cleanable
	}
	if len(cleanable[DatabaseStrategy]) != 1 || cleanable[DatabaseStrategy][0].Count != 1 {
		t.Fatalf("strategy cleanable = %+v", cleanable[DatabaseStrategy])
	}
	if len(cleanable[DatabaseADK]) != 3 {
		t.Fatalf("adk cleanable = %+v", cleanable[DatabaseADK])
	}
	if len(cleanable[DatabaseBacktestRuns]) != 1 || cleanable[DatabaseBacktestRuns][0].Count != 25 {
		t.Fatalf("backtest cleanable = %+v", cleanable[DatabaseBacktestRuns])
	}

	preview, err := manager.PreviewCleanup(t.Context(), CleanupPreviewRequest{Kind: CleanupSoftDeleted, DatabaseID: DatabaseADK})
	if err != nil {
		t.Fatal(err)
	}
	if preview.CandidateCount != 3 {
		t.Fatalf("ADK preview = %+v", preview)
	}
	manager.SetMaintenanceHooks(MaintenanceHooks{
		Purge:   func(context.Context, string, []CleanupCandidate) (int, error) { return 3, nil },
		Compact: func(context.Context, string) error { return errors.New("disk full") },
	})
	result, err := manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText})
	if err != nil {
		t.Fatal(err)
	}
	if result.Warning == "" || result.Compacted {
		t.Fatalf("cleanup result = %+v", result)
	}

	compacted := false
	manager.SetMaintenanceHooks(MaintenanceHooks{Compact: func(context.Context, string) error { compacted = true; return nil }})
	compactResult, err := manager.Compact(t.Context(), DatabaseStrategy, CompactRequest{Confirmation: "COMPACT strategy"})
	if err != nil || !compacted || !compactResult.Compacted {
		t.Fatalf("compact = %+v err=%v", compactResult, err)
	}
	if _, err := manager.Compact(t.Context(), "unknown", CompactRequest{}); err == nil {
		t.Fatal("unknown compact succeeded")
	}
	if _, err := manager.Compact(t.Context(), DatabaseStrategy, CompactRequest{Confirmation: "wrong"}); err == nil {
		t.Fatal("wrong compact confirmation succeeded")
	}
}

func TestCleanupAndCompactRejectInvalidOrUnavailableOperations(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))

	previewCases := []CleanupPreviewRequest{
		{Kind: CleanupBacktestHistory, DatabaseID: DatabaseStrategy},
		{Kind: CleanupBacktestHistory, DatabaseID: DatabaseBacktestRuns, OlderThanDays: 3651, KeepLatest: 1},
		{Kind: CleanupBacktestHistory, DatabaseID: DatabaseBacktestRuns, OlderThanDays: 1, KeepLatest: 10001},
		{Kind: CleanupSoftDeleted, DatabaseID: DatabaseWatchlist},
		{Kind: "unsupported", DatabaseID: DatabaseStrategy},
	}
	for _, request := range previewCases {
		if _, err := manager.PreviewCleanup(t.Context(), request); err == nil {
			t.Errorf("PreviewCleanup(%+v) succeeded", request)
		}
	}

	if _, err := manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: "missing"}); !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("missing preview error = %v", err)
	}

	createDeletedStrategyForMaintenanceTest(t, manager.descriptorMap()[DatabaseStrategy].Path)
	preview, err := manager.PreviewCleanup(t.Context(), CleanupPreviewRequest{Kind: CleanupSoftDeleted, DatabaseID: DatabaseStrategy})
	if err != nil {
		t.Fatalf("PreviewCleanup: %v", err)
	}
	if _, err := manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("cleanup without purge hook error = %v", err)
	}

	if _, err := manager.Compact(t.Context(), DatabaseStrategy, CompactRequest{Confirmation: "COMPACT strategy"}); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("compact without hook error = %v", err)
	}
}

func createBacktestRunsForMaintenanceTest(t *testing.T, path string, now time.Time) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE backtest_runs (id TEXT PRIMARY KEY, status TEXT, request_json TEXT, result_json TEXT, created_at TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	createMaintenanceMetadata(t, db, DatabaseBacktestRuns)
	for index := range 25 {
		updated := now.Add(-time.Duration(40+index) * 24 * time.Hour).Format(time.RFC3339Nano)
		if _, err := db.Exec(`INSERT INTO backtest_runs VALUES (?, 'completed', '{}', ?, ?, ?)`, index, string(make([]byte, index+1)), updated, updated); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO backtest_runs VALUES ('running', 'running', '{}', '{}', ?, ?)`, now.Add(-100*24*time.Hour).Format(time.RFC3339Nano), now.Add(-100*24*time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
}

func createDeletedStrategyForMaintenanceTest(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE strategy_design_definitions (id TEXT PRIMARY KEY, script TEXT, visual_model_json TEXT, deleted_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	createMaintenanceMetadata(t, db, DatabaseStrategy)
	if _, err := db.Exec(`INSERT INTO strategy_design_definitions VALUES ('deleted', 'script', '{}', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
}

func createDeletedADKForMaintenanceTest(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, statement := range []string{
		`CREATE TABLE adk_agents (id TEXT PRIMARY KEY, payload_json TEXT)`,
		`CREATE TABLE adk_workflows (id TEXT PRIMARY KEY, payload_json TEXT)`,
		`CREATE TABLE adk_workflow_triggers (id TEXT PRIMARY KEY, workflow_id TEXT, payload_json TEXT)`,
		`INSERT INTO adk_agents VALUES ('agent-deleted', '{"deletedAt":"2026-01-01T00:00:00Z"}')`,
		`INSERT INTO adk_workflows VALUES ('workflow-deleted', '{"deletedAt":"2026-01-01T00:00:00Z"}')`,
		`INSERT INTO adk_workflow_triggers VALUES ('trigger-child', 'workflow-deleted', '{}')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	createMaintenanceMetadata(t, db, DatabaseADK)
}

func createMaintenanceMetadata(t *testing.T, db *sql.DB, component string) {
	t.Helper()
	if _, err := db.Exec(`CREATE TABLE jftrade_schema_meta (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO jftrade_schema_meta VALUES (?, 1, '2026-01-01T00:00:00Z')`, component); err != nil {
		t.Fatal(err)
	}
}
