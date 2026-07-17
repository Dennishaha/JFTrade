package datamigration

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
)

func TestMaintenanceExecutionRejectsConcurrentStaleAndPartialCleanup(t *testing.T) {
	manager := newTestManager(t)
	descriptor := manager.descriptorMap()[DatabaseStrategy]
	createDeletedStrategyForMaintenanceTest(t, descriptor.Path)

	newPreview := func(t *testing.T) CleanupPreview {
		t.Helper()
		preview, err := manager.PreviewCleanup(t.Context(), CleanupPreviewRequest{
			Kind: CleanupSoftDeleted, DatabaseID: DatabaseStrategy,
		})
		if err != nil {
			t.Fatalf("PreviewCleanup: %v", err)
		}
		return preview
	}

	preview := newPreview(t)
	lock := manager.maintenance.locks[DatabaseStrategy]
	lock.Lock()
	_, err := manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText})
	lock.Unlock()
	if !errors.Is(err, ErrMaintenanceConflict) {
		t.Fatalf("concurrent cleanup = %v", err)
	}

	manager.SetMaintenanceHooks(MaintenanceHooks{
		Purge: func(context.Context, string, []CleanupCandidate) (int, error) {
			return 0, errors.New("delete transaction failed")
		},
	})
	_, err = manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText})
	if err == nil || err.Error() != "delete transaction failed" {
		t.Fatalf("purge failure = %v", err)
	}

	preview = newPreview(t)
	manager.SetMaintenanceHooks(MaintenanceHooks{
		Purge: func(_ context.Context, _ string, candidates []CleanupCandidate) (int, error) {
			return len(candidates) - 1, nil
		},
	})
	_, err = manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText})
	if !errors.Is(err, ErrPreviewStale) {
		t.Fatalf("partial purge = %v", err)
	}

	preview = newPreview(t)
	manager.SetUnavailable(DatabaseStrategy, errors.New("database reinitializing"))
	_, err = manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText})
	if !errors.Is(err, ErrPreviewStale) {
		t.Fatalf("stale database cleanup = %v", err)
	}
}

func TestMaintenanceCompactAndBackupProtectUnavailableAndBusyDatabases(t *testing.T) {
	manager := newTestManager(t)
	descriptor := manager.descriptorMap()[DatabaseWatchlist]
	initializeDescriptor(t, descriptor)

	lock := manager.maintenance.locks[DatabaseWatchlist]
	lock.Lock()
	_, err := manager.Compact(t.Context(), DatabaseWatchlist, CompactRequest{Confirmation: "COMPACT " + DatabaseWatchlist})
	lock.Unlock()
	if !errors.Is(err, ErrMaintenanceConflict) {
		t.Fatalf("concurrent compact = %v", err)
	}

	manager.SetMaintenanceHooks(MaintenanceHooks{BusyReason: func(string) string { return "backup in progress" }})
	_, err = manager.Compact(t.Context(), DatabaseWatchlist, CompactRequest{Confirmation: "COMPACT " + DatabaseWatchlist})
	if !errors.Is(err, ErrMaintenanceConflict) {
		t.Fatalf("busy compact = %v", err)
	}

	manager.SetMaintenanceHooks(MaintenanceHooks{Compact: func(context.Context, string) error { return errors.New("vacuum failed") }})
	_, err = manager.Compact(t.Context(), DatabaseWatchlist, CompactRequest{Confirmation: "COMPACT " + DatabaseWatchlist})
	if err == nil || err.Error() != "vacuum failed" {
		t.Fatalf("compact failure = %v", err)
	}

	manager.maintenance.backupLock.Lock()
	_, err = manager.Backup(t.Context(), DatabaseWatchlist, BackupConfirmationText(DatabaseWatchlist))
	manager.maintenance.backupLock.Unlock()
	if !errors.Is(err, ErrMaintenanceConflict) {
		t.Fatalf("concurrent backup = %v", err)
	}

	manager.SetUnavailable(DatabaseWatchlist, errors.New("database unavailable"))
	_, err = manager.Backup(t.Context(), DatabaseWatchlist, BackupConfirmationText(DatabaseWatchlist))
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("unavailable backup = %v", err)
	}
}

func TestMaintenanceDatabaseInspectionAndBackupRetentionFailuresStayLocal(t *testing.T) {
	manager := newTestManager(t)
	missingBackups := filepath.Join(t.TempDir(), "missing-backups")
	if err := manager.prepareBackupCapacity(missingBackups, DatabaseWatchlist, 1, 10); err == nil {
		t.Fatal("prepareBackupCapacity accepted a missing directory")
	}
	if err := manager.enforceBackupRetention(missingBackups, filepath.Join(missingBackups, "current.db"), 10); err == nil {
		t.Fatal("enforceBackupRetention accepted a missing directory")
	}
	if _, _, ok := manager.parseManagedBackupFilename(DatabaseWatchlist + "-not-a-time-abcdef12.db"); ok {
		t.Fatal("backup filename with a malformed timestamp was accepted")
	}

	storageDescriptor := Descriptor{ID: "storage-boundary", Path: filepath.Join(t.TempDir(), "storage.db"), Version: SchemaVersion}
	initializeDescriptor(t, storageDescriptor)
	canceledCtx, cancel := context.WithCancel(t.Context())
	cancel()
	if stats := inspectStorage(canceledCtx, DatabaseStatus{Descriptor: storageDescriptor, Status: "ready"}); stats.Error == "" {
		t.Fatalf("canceled storage inspection stats = %+v", stats)
	}

	managerForOverview := newTestManager(t)
	if err := os.Mkdir(managerForOverview.markerPath(), 0o755); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	if _, err := managerForOverview.Overview(t.Context()); err == nil {
		t.Fatal("overview ignored an unreadable rebuild marker")
	}
	if _, err := manager.currentDatabaseStatus(t.Context(), "unknown"); err == nil {
		t.Fatal("currentDatabaseStatus accepted an unknown database")
	}
}

func TestMaintenanceCandidateQueriesFailSafelyWhenSchemaDoesNotMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "maintenance.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if items := queryCleanable(t.Context(), db, CleanupSoftDeleted, "broken", `SELECT nope FROM missing_table`); items != nil {
		t.Fatalf("broken cleanable query = %#v", items)
	}
	if _, err := queryCandidates(t.Context(), db, `SELECT nope FROM missing_table`, "broken"); err == nil {
		t.Fatal("broken candidate query succeeded")
	}

	descriptor := Descriptor{ID: DatabaseStrategy, Path: path, Version: SchemaVersion}
	initializeDescriptor(t, descriptor)
	_, err = cleanupCandidates(t.Context(), descriptor, CleanupPreviewRequest{Kind: CleanupSoftDeleted, DatabaseID: DatabaseStrategy}, time.Now())
	if err == nil {
		t.Fatal("cleanup candidates succeeded without the strategy schema")
	}
}

func TestMaintenancePreviewDefaultsAndConcurrentStateChangesFailClosed(t *testing.T) {
	now := time.Date(2026, time.July, 16, 9, 0, 0, 0, time.UTC)
	historyManager := newTestManager(t)
	historyManager.maintenance.now = func() time.Time { return now }
	createBacktestRunsForMaintenanceTest(t, historyManager.descriptorMap()[DatabaseBacktestRuns].Path, now)
	preview, err := historyManager.PreviewCleanup(t.Context(), CleanupPreviewRequest{
		Kind: CleanupBacktestHistory, DatabaseID: DatabaseBacktestRuns,
	})
	if err != nil || preview.CandidateCount == 0 || preview.ConfirmationText == "" {
		t.Fatalf("default retention preview = %#v err=%v", preview, err)
	}

	missingManager := newTestManager(t)
	if _, err := missingManager.PreviewCleanup(t.Context(), CleanupPreviewRequest{Kind: CleanupSoftDeleted, DatabaseID: DatabaseStrategy}); err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("missing strategy preview = %v", err)
	}
	readyWithoutSchema := newTestManager(t)
	initializeDescriptor(t, readyWithoutSchema.descriptorMap()[DatabaseStrategy])
	if _, err := readyWithoutSchema.PreviewCleanup(t.Context(), CleanupPreviewRequest{Kind: CleanupSoftDeleted, DatabaseID: DatabaseStrategy}); err == nil {
		t.Fatal("preview accepted a ready database without the cleanup table")
	}

	manager := newTestManager(t)
	createDeletedStrategyForMaintenanceTest(t, manager.descriptorMap()[DatabaseStrategy].Path)
	newPreview := func(t *testing.T) CleanupPreview {
		t.Helper()
		value, previewErr := manager.PreviewCleanup(t.Context(), CleanupPreviewRequest{Kind: CleanupSoftDeleted, DatabaseID: DatabaseStrategy})
		if previewErr != nil {
			t.Fatal(previewErr)
		}
		return value
	}
	preview = newPreview(t)
	manager.SetMaintenanceHooks(MaintenanceHooks{BusyReason: func(string) string {
		manager.maintenance.mu.Lock()
		delete(manager.maintenance.previews, preview.PreviewID)
		manager.maintenance.mu.Unlock()
		return ""
	}})
	_, err = manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText})
	if !errors.Is(err, ErrPreviewNotFound) {
		t.Fatalf("withdrawn preview execution = %v", err)
	}

	preview = newPreview(t)
	manager.SetMaintenanceHooks(MaintenanceHooks{BusyReason: func(string) string {
		if writeErr := os.WriteFile(manager.markerPath(), []byte("{"), 0o600); writeErr != nil {
			t.Fatal(writeErr)
		}
		return ""
	}})
	_, err = manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText})
	if err == nil || !strings.Contains(err.Error(), "decode database rebuild marker") {
		t.Fatalf("corrupt marker cleanup execution = %v", err)
	}
}

func TestMaintenanceBackupAndRebuildSurfacePersistentStateErrors(t *testing.T) {
	if err := verifySQLiteBackup(t.Context(), ""); err == nil {
		t.Fatal("empty backup path passed verification")
	}

	manager := newTestManager(t)
	initializeDescriptors(t, manager, nil)
	if err := os.WriteFile(manager.markerPath(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Backup(t.Context(), DatabaseWatchlist, BackupConfirmationText(DatabaseWatchlist)); err == nil || !strings.Contains(err.Error(), "decode database rebuild marker") {
		t.Fatalf("backup with corrupt marker = %v", err)
	}
	if _, err := manager.ScheduleRebuild(t.Context(), RebuildRequest{
		Mode: "single", DatabaseIDs: []string{DatabaseWatchlist}, Confirmation: "REBUILD " + DatabaseWatchlist,
	}); err == nil || !strings.Contains(err.Error(), "decode database rebuild marker") {
		t.Fatalf("rebuild scheduling with corrupt marker = %v", err)
	}
}

func TestBackupRetentionReportsRemovalPermissionFailures(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can remove files from a read-only directory")
	}
	root := t.TempDir()
	backupDir := filepath.Join(root, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))
	for index := range backupRetentionPerDB {
		name := DatabaseWatchlist + "-20260716T090" + string(rune('0'+index)) + "00.000000000Z-abcdef12.db"
		if err := os.WriteFile(filepath.Join(backupDir, name), []byte("snapshot"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(backupDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(backupDir, 0o700) })
	if err := manager.prepareBackupCapacity(backupDir, DatabaseWatchlist, 1, 100); err == nil {
		t.Fatal("prepareBackupCapacity removed a protected backup")
	}
}

func TestMaintenanceStorageAndCandidateBoundaryErrorsAreContained(t *testing.T) {
	emptyStatus := DatabaseStatus{Descriptor: Descriptor{ID: "empty", Path: ""}, Status: "ready"}
	if stats := inspectStorage(t.Context(), emptyStatus); stats.Error == "" {
		t.Fatalf("empty storage inspection = %+v", stats)
	}
	if items := inspectCleanable(t.Context(), emptyStatus); items != nil {
		t.Fatalf("empty cleanable inspection = %#v", items)
	}
	if _, err := cleanupCandidates(t.Context(), emptyStatus.Descriptor, CleanupPreviewRequest{Kind: CleanupSoftDeleted}, time.Now()); err == nil {
		t.Fatal("empty cleanup descriptor succeeded")
	}

	path := filepath.Join(t.TempDir(), "candidates.db")
	descriptor := Descriptor{ID: DatabaseBacktestRuns, Path: path, Version: SchemaVersion}
	initializeDescriptor(t, descriptor)
	if items := inspectCleanable(t.Context(), DatabaseStatus{Descriptor: Descriptor{ID: "other", Path: path}, Status: "ready"}); items != nil {
		t.Fatalf("unsupported cleanable category = %#v", items)
	}
	if _, err := cleanupCandidates(t.Context(), descriptor, CleanupPreviewRequest{Kind: CleanupBacktestHistory, OlderThanDays: 1, KeepLatest: 1}, time.Now()); err == nil {
		t.Fatal("history cleanup candidates succeeded without a backtest_runs table")
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := queryCandidates(t.Context(), db, `SELECT NULL, 1`, "broken-row"); err == nil {
		t.Fatal("candidate row with a NULL ID was accepted")
	}
	if _, err := db.Exec(`CREATE TABLE backtest_runs (id TEXT, status TEXT, request_json TEXT, result_json TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO backtest_runs VALUES (NULL, 'completed', '{}', '{}', '2026-07-16T09:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanupCandidates(t.Context(), descriptor, CleanupPreviewRequest{Kind: CleanupBacktestHistory, OlderThanDays: 1, KeepLatest: 1}, time.Now()); err == nil {
		t.Fatal("history cleanup candidate with a NULL ID was accepted")
	}

	adkDescriptor := Descriptor{ID: DatabaseADK, Path: filepath.Join(t.TempDir(), "adk-candidates.db"), Version: SchemaVersion}
	initializeDescriptor(t, adkDescriptor)
	adkDB, err := sql.Open("sqlite", adkDescriptor.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adkDB.Exec(`CREATE TABLE adk_agents (id TEXT, payload_json TEXT); CREATE TABLE adk_workflows (id TEXT, payload_json TEXT)`); err != nil {
		_ = adkDB.Close()
		t.Fatal(err)
	}
	if err := adkDB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanupCandidates(t.Context(), adkDescriptor, CleanupPreviewRequest{Kind: CleanupSoftDeleted, DatabaseID: DatabaseADK}, time.Now()); err == nil {
		t.Fatal("ADK cleanup candidates ignored a missing trigger table")
	}
}

func TestBackupSnapshotRejectsBlockedDirectoryAndEmptySource(t *testing.T) {
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	blockedManager := NewManager(filepath.Join(blocked, "settings.json"), filepath.Join(root, "backtest.db"))
	if _, err := blockedManager.createBackupSnapshot(t.Context(), Descriptor{ID: DatabaseWatchlist, Path: filepath.Join(root, "source.db")}, "ready", time.Now()); err == nil {
		t.Fatal("backup snapshot created beneath a non-directory settings parent")
	}

	manager := newTestManager(t)
	if _, err := manager.createBackupSnapshot(t.Context(), Descriptor{ID: DatabaseWatchlist, Path: ""}, "ready", time.Now()); err == nil {
		t.Fatal("backup snapshot accepted an empty source path")
	}
}

func TestBackupRetentionEvictsQuotaPressureAcrossDatabaseFiles(t *testing.T) {
	root := t.TempDir()
	backupDir := filepath.Join(root, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))
	writeBackup := func(databaseID, stamp, token string, size int) string {
		t.Helper()
		path := filepath.Join(backupDir, databaseID+"-"+stamp+"-"+token+".db")
		if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	other := writeBackup(DatabaseADK, "20260716T090000.000000000Z", "abcdef12", 8)
	if err := manager.prepareBackupCapacity(backupDir, DatabaseWatchlist, 5, 10); err != nil {
		t.Fatalf("prepare quota eviction: %v", err)
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatalf("quota-pressure backup still exists: %v", err)
	}

	current := writeBackup(DatabaseWatchlist, "20260716T090100.000000000Z", "abcdef13", 4)
	other = writeBackup(DatabaseADK, "20260716T090200.000000000Z", "abcdef14", 8)
	if err := manager.enforceBackupRetention(backupDir, current, 5); err != nil {
		t.Fatalf("retention quota eviction: %v", err)
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatalf("retention quota backup still exists: %v", err)
	}
	if _, _, ok := manager.parseManagedBackupFilename(DatabaseWatchlist + "-20260716T090300.000000000Z-zzzzzzzz.db"); ok {
		t.Fatal("non-hex backup token was accepted")
	}
}

func TestMaintenanceCompactionAndBackupFailClosedWhenPersistentStateBreaks(t *testing.T) {
	compactManager := newTestManager(t)
	initializeDescriptor(t, compactManager.descriptorMap()[DatabaseWatchlist])
	if err := os.WriteFile(compactManager.markerPath(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := compactManager.Compact(t.Context(), DatabaseWatchlist, CompactRequest{Confirmation: "COMPACT " + DatabaseWatchlist}); err == nil || !strings.Contains(err.Error(), "decode database rebuild marker") {
		t.Fatalf("compact with corrupt marker = %v", err)
	}

	backupManager := newTestManager(t)
	for index := range backupManager.descriptors {
		if backupManager.descriptors[index].ID == DatabaseWatchlist {
			backupManager.descriptors[index].Path = ""
		}
	}
	backupManager.SetUnavailable(DatabaseWatchlist, &sqliteschema.IncompatibleError{Component: DatabaseWatchlist, Reason: "legacy schema"})
	if _, err := backupManager.Backup(t.Context(), DatabaseWatchlist, BackupConfirmationText(DatabaseWatchlist)); err == nil {
		t.Fatal("backup of an incompatible database with no source path succeeded")
	}

	missingManager := newTestManager(t)
	if _, err := missingManager.Compact(t.Context(), DatabaseStrategy, CompactRequest{Confirmation: "COMPACT " + DatabaseStrategy}); err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("compact missing database = %v", err)
	}
}

func TestNewPreviewIDSurfacesSecureRandomnessFailure(t *testing.T) {
	previous := readPreviewRandom
	readPreviewRandom = func([]byte) (int, error) { return 0, errors.New("secure random unavailable") }
	t.Cleanup(func() { readPreviewRandom = previous })
	if _, err := newPreviewID(); err == nil || !strings.Contains(err.Error(), "secure random unavailable") {
		t.Fatalf("newPreviewID error = %v", err)
	}
}

func TestMaintenancePreviewRejectsAConfiguredCleanupTargetWithoutDescriptor(t *testing.T) {
	manager := newTestManager(t)
	filtered := make([]Descriptor, 0, len(manager.descriptors)-1)
	for _, descriptor := range manager.descriptors {
		if descriptor.ID != DatabaseStrategy {
			filtered = append(filtered, descriptor)
		}
	}
	manager.descriptors = filtered
	if _, err := manager.PreviewCleanup(t.Context(), CleanupPreviewRequest{Kind: CleanupSoftDeleted, DatabaseID: DatabaseStrategy}); err == nil || !strings.Contains(err.Error(), "unknown database id") {
		t.Fatalf("preview without strategy descriptor = %v", err)
	}
}

func TestMaintenanceCleanupAndCompactionReportActualReclaimedStorage(t *testing.T) {
	manager := newTestManager(t)
	strategy := manager.descriptorMap()[DatabaseStrategy]
	createDeletedStrategyForMaintenanceTest(t, strategy.Path)
	db, err := sql.Open("sqlite", strategy.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO strategy_design_definitions VALUES ('large-deleted', zeroblob(2097152), '{}', '2026-01-01T00:00:00Z')`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	preview, err := manager.PreviewCleanup(t.Context(), CleanupPreviewRequest{Kind: CleanupSoftDeleted, DatabaseID: DatabaseStrategy})
	if err != nil || preview.CandidateCount != 2 {
		t.Fatalf("cleanup preview = %#v err=%v", preview, err)
	}
	manager.SetMaintenanceHooks(MaintenanceHooks{
		Purge: func(ctx context.Context, _ string, candidates []CleanupCandidate) (int, error) {
			writer, openErr := sql.Open("sqlite", strategy.Path)
			if openErr != nil {
				return 0, openErr
			}
			defer func() { _ = writer.Close() }()
			for _, candidate := range candidates {
				if _, deleteErr := writer.ExecContext(ctx, `DELETE FROM strategy_design_definitions WHERE id = ?`, candidate.ID); deleteErr != nil {
					return 0, deleteErr
				}
			}
			return len(candidates), nil
		},
		Compact: func(ctx context.Context, _ string) error {
			writer, openErr := sql.Open("sqlite", strategy.Path)
			if openErr != nil {
				return openErr
			}
			defer func() { _ = writer.Close() }()
			_, compactErr := writer.ExecContext(ctx, `VACUUM`)
			return compactErr
		},
	})
	result, err := manager.ExecuteCleanup(t.Context(), CleanupExecuteRequest{PreviewID: preview.PreviewID, Confirmation: preview.ConfirmationText})
	if err != nil || !result.Compacted || result.ReclaimedBytes <= 0 || result.AfterBytes >= result.BeforeBytes {
		t.Fatalf("cleanup result = %#v err=%v", result, err)
	}

	watchlist := manager.descriptorMap()[DatabaseWatchlist]
	initializeDescriptor(t, watchlist)
	db, err = sql.Open("sqlite", watchlist.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE compact_payload (value BLOB); INSERT INTO compact_payload VALUES (zeroblob(2097152)); DELETE FROM compact_payload`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	manager.SetMaintenanceHooks(MaintenanceHooks{Compact: func(ctx context.Context, _ string) error {
		writer, openErr := sql.Open("sqlite", watchlist.Path)
		if openErr != nil {
			return openErr
		}
		defer func() { _ = writer.Close() }()
		_, compactErr := writer.ExecContext(ctx, `VACUUM`)
		return compactErr
	}})
	compactResult, err := manager.Compact(t.Context(), DatabaseWatchlist, CompactRequest{Confirmation: "COMPACT " + DatabaseWatchlist})
	if err != nil || !compactResult.Compacted || compactResult.ReclaimedBytes <= 0 || compactResult.AfterBytes >= compactResult.BeforeBytes {
		t.Fatalf("compact result = %#v err=%v", compactResult, err)
	}
}
