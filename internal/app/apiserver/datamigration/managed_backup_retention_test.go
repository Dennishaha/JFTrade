package datamigration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestManagedBackupRetentionKeepsCurrentSnapshotAndPrunesOldFiles(t *testing.T) {
	root := t.TempDir()
	backupDir := filepath.Join(root, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	manager := NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))
	writeBackup := func(databaseID string, stamp string, token string, size int) string {
		t.Helper()
		path := filepath.Join(backupDir, databaseID+"-"+stamp+"-"+token+".db")
		if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", path, err)
		}
		return path
	}
	watchlist := []string{
		writeBackup(DatabaseWatchlist, "20260711T140000.000000000Z", "00000001", 4),
		writeBackup(DatabaseWatchlist, "20260711T140100.000000000Z", "00000002", 4),
		writeBackup(DatabaseWatchlist, "20260711T140200.000000000Z", "00000003", 4),
		writeBackup(DatabaseWatchlist, "20260711T140300.000000000Z", "00000004", 4),
	}
	other := writeBackup(DatabaseADK, "20260711T140000.000000000Z", "00000005", 4)
	current := watchlist[len(watchlist)-1]

	if err := manager.enforceBackupRetention(backupDir, current, 8); err != nil {
		t.Fatalf("enforceBackupRetention: %v", err)
	}
	if _, err := os.Stat(current); err != nil {
		t.Fatalf("current backup was removed: %v", err)
	}
	files, err := manager.listManagedBackupFiles(backupDir)
	if err != nil {
		t.Fatalf("listManagedBackupFiles: %v", err)
	}
	var total int64
	for _, file := range files {
		total += file.size
	}
	if total > 8 || len(files) > 2 {
		t.Fatalf("retained backups total/files = %d/%d, want within quota", total, len(files))
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatalf("old non-current backup should be pruned under quota pressure: %v", err)
	}
	if err := manager.enforceBackupRetention(backupDir, current, 1); !errors.Is(err, ErrBackupQuotaExceeded) {
		t.Fatalf("current-over-quota error = %v", err)
	}
}

func TestManagedBackupFileDiscoveryAndFilenameBoundaries(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))
	if _, err := manager.listManagedBackupFiles(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing backup directory should fail discovery")
	}
	backupDir := filepath.Join(root, "backups")
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		t.Fatal(err)
	}
	validName := DatabaseWatchlist + "-20260711T140000.000000000Z-abcdef12.db"
	if err := os.WriteFile(filepath.Join(backupDir, validName), []byte("snapshot"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "watchlist-invalid-token.db"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(backupDir, DatabaseWatchlist+"-20260711T140100.000000000Z-12345678.db"), 0o700); err != nil {
		t.Fatal(err)
	}
	files, err := manager.listManagedBackupFiles(backupDir)
	if err != nil || len(files) != 1 || filepath.Base(files[0].path) != validName {
		t.Fatalf("managed files = %#v err=%v", files, err)
	}
	if id, createdAt, ok := manager.parseManagedBackupFilename(validName); !ok || id != DatabaseWatchlist || createdAt.IsZero() {
		t.Fatalf("valid managed filename = %q %v %v", id, createdAt, ok)
	}
	for _, name := range []string{"", "watchlist-20260711T140000.000000000Z-deadbeef.txt", "unknown-20260711T140000.000000000Z-deadbeef.db", "watchlist-not-a-time-deadbeef.db"} {
		if _, _, ok := manager.parseManagedBackupFilename(name); ok {
			t.Fatalf("invalid managed backup name was accepted: %q", name)
		}
	}
	nonEmpty := filepath.Join(root, "nonempty")
	if err := os.Mkdir(nonEmpty, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nonEmpty, "child"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeManagedBackup(managedBackupFile{path: nonEmpty}); err == nil {
		t.Fatal("removing a non-empty directory should report a backup removal error")
	}
}

func TestBackupSnapshotFailureCleansUpPartialFilesAndVerificationRejectsInvalidSQLite(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))
	descriptor := Descriptor{ID: DatabaseWatchlist, Path: root}
	if _, err := manager.createBackupSnapshot(context.Background(), descriptor, "ready", time.Now().UTC()); err == nil {
		t.Fatal("backup snapshot of a missing source database succeeded")
	}
	backupDir := filepath.Join(root, "backups")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("ReadDir backup failures: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("failed backup left partial snapshots: %#v", entries)
	}
	badBackup := filepath.Join(root, "not-sqlite.db")
	if err := os.WriteFile(badBackup, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifySQLiteBackup(context.Background(), badBackup); err == nil {
		t.Fatal("invalid SQLite backup passed quick-check verification")
	}
}
