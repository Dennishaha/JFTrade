package datamigration

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVerifyMarkerBackupRejectsEveryUntrustedMarkerField(t *testing.T) {
	manager := newTestManager(t)
	descriptor := manager.descriptorMap()[DatabaseWatchlist]
	initializeDescriptor(t, descriptor)
	result, err := manager.createBackupSnapshot(t.Context(), descriptor, "ready", time.Now().UTC())
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}
	valid := markerBackup{DatabaseID: descriptor.ID, Path: result.BackupPath, SizeBytes: result.SizeBytes, SHA256: result.SHA256}

	tests := []struct {
		name   string
		backup markerBackup
		want   string
	}{
		{name: "database id", backup: markerBackup{DatabaseID: DatabaseADK}, want: "does not match"},
		{name: "managed root itself", backup: markerBackup{DatabaseID: descriptor.ID, Path: filepath.Dir(result.BackupPath)}, want: "outside the managed backup directory"},
		{name: "outside directory", backup: markerBackup{DatabaseID: descriptor.ID, Path: descriptor.Path}, want: "outside the managed backup directory"},
		{name: "unmanaged filename", backup: markerBackup{DatabaseID: descriptor.ID, Path: filepath.Join(filepath.Dir(result.BackupPath), "not-managed.db")}, want: "filename is not managed"},
		{name: "missing file", backup: markerBackup{DatabaseID: descriptor.ID, Path: filepath.Join(filepath.Dir(result.BackupPath), descriptor.ID+"-20260724T010203.000000000Z-abcdef12.db")}, want: "no such file"},
		{name: "wrong size", backup: markerBackup{DatabaseID: descriptor.ID, Path: valid.Path, SizeBytes: valid.SizeBytes + 1, SHA256: valid.SHA256}, want: "size or file type"},
		{name: "wrong digest", backup: markerBackup{DatabaseID: descriptor.ID, Path: valid.Path, SizeBytes: valid.SizeBytes, SHA256: strings.Repeat("0", 64)}, want: "SHA-256"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := manager.verifyMarkerBackup(t.Context(), descriptor, test.backup)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("verifyMarkerBackup() error = %v", err)
			}
		})
	}

	invalidPath := filepath.Join(filepath.Dir(result.BackupPath), descriptor.ID+"-20260724T010204.000000000Z-abcdef13.db")
	if err := os.WriteFile(invalidPath, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("write invalid backup: %v", err)
	}
	digest, err := fileSHA256(invalidPath)
	if err != nil {
		t.Fatalf("digest invalid backup: %v", err)
	}
	err = manager.verifyMarkerBackup(t.Context(), descriptor, markerBackup{
		DatabaseID: descriptor.ID,
		Path:       invalidPath,
		SizeBytes:  int64(len("not sqlite")),
		SHA256:     digest,
	})
	if err == nil {
		t.Fatal("verifyMarkerBackup(non-SQLite) error = nil")
	}

	symlinkPath := filepath.Join(filepath.Dir(result.BackupPath), descriptor.ID+"-20260724T010205.000000000Z-abcdef14.db")
	if err := os.Symlink(result.BackupPath, symlinkPath); err != nil {
		t.Fatalf("create backup symlink: %v", err)
	}
	err = manager.verifyMarkerBackup(t.Context(), descriptor, markerBackup{
		DatabaseID: descriptor.ID,
		Path:       symlinkPath,
		SizeBytes:  valid.SizeBytes,
		SHA256:     valid.SHA256,
	})
	if err == nil || !strings.Contains(err.Error(), "file type") {
		t.Fatalf("verifyMarkerBackup(symlink) error = %v", err)
	}
}

func TestApplyPendingRejectsDuplicateAndMissingBackups(t *testing.T) {
	manager := newTestManager(t)
	duplicate := markerBackup{DatabaseID: DatabaseADK, Path: "unused"}
	if err := manager.writeMarker(marker{
		DatabaseIDs: []string{DatabaseADK},
		Backups:     []markerBackup{duplicate, duplicate},
	}); err != nil {
		t.Fatalf("write duplicate marker: %v", err)
	}
	if err := manager.ApplyPending(); err == nil || !strings.Contains(err.Error(), "duplicate backup") {
		t.Fatalf("ApplyPending(duplicate) error = %v", err)
	}
	if err := manager.writeMarker(marker{DatabaseIDs: []string{DatabaseADK}}); err != nil {
		t.Fatalf("write missing backup marker: %v", err)
	}
	if err := manager.ApplyPending(); err == nil || !strings.Contains(err.Error(), "has no verified backup") {
		t.Fatalf("ApplyPending(missing backup) error = %v", err)
	}
	if err := manager.writeMarker(marker{
		DatabaseIDs: []string{DatabaseADK},
		Backups:     []markerBackup{{DatabaseID: DatabaseWatchlist}},
	}); err != nil {
		t.Fatalf("write marker with extra backup: %v", err)
	}
	if err := manager.ApplyPending(); err == nil || !strings.Contains(err.Error(), "unscheduled database") {
		t.Fatalf("ApplyPending(extra backup) error = %v", err)
	}
}

func TestScheduleRebuildLockedRemovesSnapshotsAfterBatchFailure(t *testing.T) {
	manager := newTestManager(t)
	valid := manager.descriptorMap()[DatabaseWatchlist]
	initializeDescriptor(t, valid)
	missing := manager.descriptorMap()[DatabaseResearch]
	statuses := map[string]DatabaseStatus{
		valid.ID:   {Descriptor: valid, Status: "ready"},
		missing.ID: {Descriptor: missing, Status: "ready"},
	}
	_, err := manager.scheduleRebuildLocked(t.Context(), []string{valid.ID, missing.ID}, statuses)
	if err == nil || !strings.Contains(err.Error(), "create verified rebuild backup") {
		t.Fatalf("scheduleRebuildLocked() error = %v", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(filepath.Dir(manager.settingsPath), "backups"))
	if readErr != nil {
		t.Fatalf("read backup directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed batch retained backups: %v", entries)
	}
}

func TestScheduleRebuildLockedRequiresBackupForExistingMarkerIDs(t *testing.T) {
	manager := newTestManager(t)
	if err := manager.writeMarker(marker{DatabaseIDs: []string{DatabaseADK}}); err != nil {
		t.Fatalf("write incomplete marker: %v", err)
	}
	descriptor := manager.descriptorMap()[DatabaseWatchlist]
	initializeDescriptor(t, descriptor)
	_, err := manager.scheduleRebuildLocked(t.Context(), []string{descriptor.ID}, map[string]DatabaseStatus{
		descriptor.ID: {Descriptor: descriptor, Status: "ready"},
	})
	if err == nil || !strings.Contains(err.Error(), "has no verified backup") {
		t.Fatalf("scheduleRebuildLocked(incomplete marker) error = %v", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(filepath.Dir(manager.settingsPath), "backups"))
	if readErr != nil {
		t.Fatalf("read backup directory: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("incomplete marker retained new backup: %v", entries)
	}
}

func TestScheduleRebuildIsIdempotentForAnExistingVerifiedBackup(t *testing.T) {
	manager := newTestManager(t)
	descriptor := manager.descriptorMap()[DatabaseWatchlist]
	initializeDescriptor(t, descriptor)
	request := RebuildRequest{
		Mode: "single", DatabaseIDs: []string{descriptor.ID}, Confirmation: "REBUILD " + descriptor.ID,
	}
	first, err := manager.ScheduleRebuild(t.Context(), request)
	if err != nil {
		t.Fatalf("first ScheduleRebuild() error = %v", err)
	}
	second, err := manager.ScheduleRebuild(t.Context(), request)
	if err != nil {
		t.Fatalf("second ScheduleRebuild() error = %v", err)
	}
	if len(first.DatabaseIDs) != 1 || len(second.DatabaseIDs) != 1 || second.DatabaseIDs[0] != descriptor.ID {
		t.Fatalf("idempotent rebuild results = (%+v, %+v)", first, second)
	}
	pending, err := manager.readMarker()
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if len(pending.Backups) != 1 {
		t.Fatalf("idempotent schedule created %d backups", len(pending.Backups))
	}
}

func TestRebuildSelectionAndLockRollbackBoundaries(t *testing.T) {
	statuses := []DatabaseStatus{{Descriptor: Descriptor{ID: DatabaseADK}, Status: "unavailable"}}
	statusByID := map[string]DatabaseStatus{DatabaseADK: statuses[0]}
	if _, err := selectRebuildIDs(statuses, statusByID, RebuildRequest{
		Mode: "single", DatabaseIDs: []string{DatabaseADK}, Confirmation: "",
	}); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("selectRebuildIDs(unavailable) error = %v", err)
	}
	if _, err := selectRebuildIDs(nil, map[string]DatabaseStatus{}, RebuildRequest{
		Mode: "incompatible", Confirmation: BatchConfirmationText, DatabaseIDs: []string{"unknown"},
	}); err == nil || !strings.Contains(err.Error(), "no databases require rebuild") {
		t.Fatalf("selectRebuildIDs(empty incompatible) error = %v", err)
	}
	if _, err := selectRebuildIDs(
		[]DatabaseStatus{{Descriptor: Descriptor{ID: DatabaseADK}, Status: "incompatible"}},
		map[string]DatabaseStatus{},
		RebuildRequest{Mode: "incompatible", Confirmation: BatchConfirmationText},
	); err == nil || !strings.Contains(err.Error(), "unknown database id") {
		t.Fatalf("selectRebuildIDs(missing status map entry) error = %v", err)
	}

	manager := newTestManager(t)
	manager.maintenance.locks[DatabaseStrategy].Lock()
	_, err := manager.tryLockDatabases([]string{DatabaseADK, DatabaseStrategy})
	manager.maintenance.locks[DatabaseStrategy].Unlock()
	if !errors.Is(err, ErrMaintenanceConflict) {
		t.Fatalf("tryLockDatabases() error = %v", err)
	}
	if !manager.maintenance.locks[DatabaseADK].TryLock() {
		t.Fatal("tryLockDatabases did not roll back its earlier lock")
	}
	manager.maintenance.locks[DatabaseADK].Unlock()
}

func TestScheduleRebuildLockedPropagatesUnreadableMarker(t *testing.T) {
	manager := newTestManager(t)
	if err := os.WriteFile(manager.markerPath(), []byte("{"), 0o600); err != nil {
		t.Fatalf("write corrupt marker: %v", err)
	}
	if _, err := manager.scheduleRebuildLocked(t.Context(), []string{DatabaseADK}, nil); err == nil || !strings.Contains(err.Error(), "decode database rebuild marker") {
		t.Fatalf("scheduleRebuildLocked() error = %v", err)
	}
}

func TestProtectedBackupAndDigestFailureBoundaries(t *testing.T) {
	manager := newTestManager(t)
	if err := os.WriteFile(manager.markerPath(), []byte("{"), 0o600); err != nil {
		t.Fatalf("write corrupt marker: %v", err)
	}
	if _, err := manager.protectedBackupPaths("extra"); err == nil || !strings.Contains(err.Error(), "decode database rebuild marker") {
		t.Fatalf("protectedBackupPaths() error = %v", err)
	}
	if _, err := fileSHA256(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fileSHA256(missing) error = %v", err)
	}
	if _, err := fileSHA256(t.TempDir()); err == nil {
		t.Fatal("fileSHA256(directory) error = nil")
	}
}
