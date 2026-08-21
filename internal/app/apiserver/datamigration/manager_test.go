package datamigration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jftrade/jftrade-main/internal/store/ownerlock"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	"github.com/jmoiron/sqlx"
)

func TestManagerSchedulesSingleAndBatchRebuilds(t *testing.T) {
	manager := newTestManager(t)
	initializeDescriptors(t, manager, map[string]bool{DatabaseADK: true, DatabaseStrategy: true})

	if _, err := manager.ScheduleRebuild(t.Context(), RebuildRequest{
		Mode: "single", DatabaseIDs: []string{DatabaseBacktest}, Confirmation: "wrong",
	}); err == nil {
		t.Fatal("expected confirmation error")
	}
	if _, err := manager.ScheduleRebuild(t.Context(), RebuildRequest{
		Mode: "single", DatabaseIDs: []string{"unknown"}, Confirmation: "REBUILD unknown",
	}); err == nil {
		t.Fatal("expected database id whitelist error")
	}
	result, err := manager.ScheduleRebuild(t.Context(), RebuildRequest{
		Mode: "single", DatabaseIDs: []string{DatabaseBacktest}, Confirmation: "REBUILD " + DatabaseBacktest,
	})
	if err != nil || !result.RestartRequired {
		t.Fatalf("schedule healthy database: result=%+v err=%v", result, err)
	}
	result, err = manager.ScheduleRebuild(t.Context(), RebuildRequest{
		Mode: "incompatible", Confirmation: BatchConfirmationText,
	})
	if err != nil {
		t.Fatalf("schedule incompatible databases: %v", err)
	}
	want := map[string]bool{DatabaseBacktest: true, DatabaseADK: true, DatabaseStrategy: true}
	for _, id := range result.DatabaseIDs {
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("scheduled ids = %v, missing %v", result.DatabaseIDs, want)
	}
	if err := manager.ApplyPending(); err != nil {
		t.Fatalf("apply batch rebuild: %v", err)
	}
	for _, id := range result.DatabaseIDs {
		if _, err := os.Stat(manager.descriptorMap()[id].Path); !os.IsNotExist(err) {
			t.Fatalf("batch database %s still exists: %v", id, err)
		}
	}
}

func TestManagerScheduleRebuildSharesMaintenanceLocks(t *testing.T) {
	manager := newTestManager(t)
	descriptor := manager.descriptorMap()[DatabaseWatchlist]
	initializeDescriptor(t, descriptor)
	request := RebuildRequest{
		Mode: "single", DatabaseIDs: []string{DatabaseWatchlist}, Confirmation: "REBUILD " + DatabaseWatchlist,
	}

	manager.maintenance.backupLock.Lock()
	_, err := manager.ScheduleRebuild(t.Context(), request)
	manager.maintenance.backupLock.Unlock()
	if !errors.Is(err, ErrMaintenanceConflict) {
		t.Fatalf("ScheduleRebuild during backup error = %v", err)
	}

	manager.maintenance.locks[DatabaseWatchlist].Lock()
	_, err = manager.ScheduleRebuild(t.Context(), request)
	manager.maintenance.locks[DatabaseWatchlist].Unlock()
	if !errors.Is(err, ErrMaintenanceConflict) {
		t.Fatalf("ScheduleRebuild during database maintenance error = %v", err)
	}
	if _, err := os.Stat(manager.markerPath()); !os.IsNotExist(err) {
		t.Fatalf("conflicting schedule wrote rebuild marker: %v", err)
	}
}

func TestManagerDescriptorsMatchSchemaCatalog(t *testing.T) {
	manager := newTestManager(t)
	definitions := sqliteschema.Definitions()
	if len(manager.descriptors) != len(definitions) || len(manager.descriptors) != 9 {
		t.Fatalf("managed descriptors = %d, catalog definitions = %d", len(manager.descriptors), len(definitions))
	}
	byID := manager.descriptorMap()
	for _, definition := range definitions {
		descriptor, ok := byID[definition.ID]
		if !ok {
			t.Fatalf("schema catalog database %q is not managed", definition.ID)
		}
		if descriptor.Version != definition.Version || descriptor.Path == "" {
			t.Fatalf("descriptor %q = %#v, catalog version = %d", definition.ID, descriptor, definition.Version)
		}
		if manager.maintenance.locks[definition.ID] == nil {
			t.Fatalf("database %q has no maintenance lock", definition.ID)
		}
	}
	artifact := byID[DatabaseADKArtifact]
	if artifact.Path != filepath.Join(filepath.Dir(manager.settingsPath), "adk-artifact.db") {
		t.Fatalf("artifact path = %q", artifact.Path)
	}
}

func TestManagerApplyPendingDeletesOnlySelectedDatabaseFiles(t *testing.T) {
	manager := newTestManager(t)
	initializeDescriptors(t, manager, nil)
	selected := manager.descriptorMap()[DatabaseADK]
	other := manager.descriptorMap()[DatabaseStrategy]
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.WriteFile(selected.Path+suffix, []byte("sidecar"), 0o600); err != nil {
			t.Fatalf("write selected sidecar: %v", err)
		}
	}
	nonDatabase := filepath.Join(filepath.Dir(manager.settingsPath), "settings.json")
	if err := os.WriteFile(nonDatabase, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}
	if _, err := manager.ScheduleRebuild(t.Context(), RebuildRequest{
		Mode: "single", DatabaseIDs: []string{DatabaseADK}, Confirmation: "REBUILD " + DatabaseADK,
	}); err != nil {
		t.Fatalf("schedule rebuild: %v", err)
	}
	if err := manager.ApplyPending(); err != nil {
		t.Fatalf("apply rebuild: %v", err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if _, err := os.Stat(selected.Path + suffix); !os.IsNotExist(err) {
			t.Fatalf("selected file %s still exists: %v", selected.Path+suffix, err)
		}
	}
	if _, err := os.Stat(other.Path); err != nil {
		t.Fatalf("unselected database was removed: %v", err)
	}
	if _, err := os.Stat(nonDatabase); err != nil {
		t.Fatalf("non-database file was removed: %v", err)
	}
	if _, err := os.Stat(manager.markerPath()); err != nil {
		t.Fatalf("marker should remain until successful initialization: %v", err)
	}
	initializeDescriptor(t, selected)
	if err := manager.CompletePending(context.Background()); err != nil {
		t.Fatalf("complete rebuild: %v", err)
	}
	if _, err := os.Stat(manager.markerPath()); !os.IsNotExist(err) {
		t.Fatalf("marker still exists after completion: %v", err)
	}
}

func TestManagerApplyPendingFailsClosedWhenDatabaseWriterLeaseIsHeld(t *testing.T) {
	manager := newTestManager(t)
	initializeDescriptors(t, manager, nil)
	selected := manager.descriptorMap()[DatabaseADK]
	if _, err := manager.ScheduleRebuild(t.Context(), RebuildRequest{
		Mode: "single", DatabaseIDs: []string{DatabaseADK}, Confirmation: "REBUILD " + DatabaseADK,
	}); err != nil {
		t.Fatalf("schedule rebuild: %v", err)
	}
	lease, err := ownerlock.Acquire(selected.Path, ownerlock.CurrentDiagnostic("rust-test", "conflict"))
	if err != nil {
		t.Fatalf("hold external writer lease: %v", err)
	}
	defer func() { _ = lease.Close() }()
	if err := manager.ApplyPending(); !errors.Is(err, ownerlock.ErrHeld) {
		t.Fatalf("ApplyPending conflict error = %v", err)
	}
	if _, err := os.Stat(selected.Path); err != nil {
		t.Fatalf("conflicting rebuild removed source database: %v", err)
	}
}

func TestManagerApplyPendingRejectsTamperedBackupBeforeDeletingAnySource(t *testing.T) {
	manager := newTestManager(t)
	initializeDescriptors(t, manager, nil)
	for _, id := range []string{DatabaseADK, DatabaseStrategy} {
		if _, err := manager.ScheduleRebuild(t.Context(), RebuildRequest{
			Mode: "single", DatabaseIDs: []string{id}, Confirmation: "REBUILD " + id,
		}); err != nil {
			t.Fatalf("schedule %s rebuild: %v", id, err)
		}
	}
	pending, err := manager.readMarker()
	if err != nil {
		t.Fatalf("read rebuild marker: %v", err)
	}
	if len(pending.Backups) != 2 {
		t.Fatalf("marker backups = %#v", pending.Backups)
	}
	sourceDigests := make(map[string]string, len(pending.DatabaseIDs))
	for _, id := range pending.DatabaseIDs {
		digest, err := fileSHA256(manager.descriptorMap()[id].Path)
		if err != nil {
			t.Fatalf("digest %s source: %v", id, err)
		}
		sourceDigests[id] = digest
	}
	if err := os.WriteFile(pending.Backups[len(pending.Backups)-1].Path, []byte("tampered"), 0o600); err != nil {
		t.Fatalf("tamper rebuild backup: %v", err)
	}
	if err := manager.ApplyPending(); err == nil {
		t.Fatal("ApplyPending(tampered backup) error = nil")
	}
	for id, before := range sourceDigests {
		after, err := fileSHA256(manager.descriptorMap()[id].Path)
		if err != nil {
			t.Fatalf("source %s was removed after backup verification failed: %v", id, err)
		}
		if after != before {
			t.Fatalf("source %s changed after backup verification failed", id)
		}
	}
	if _, err := os.Stat(manager.markerPath()); err != nil {
		t.Fatalf("marker was removed after backup verification failed: %v", err)
	}
}

func TestManagerKeepsMarkerWhenDeleteFails(t *testing.T) {
	manager := newTestManager(t)
	descriptor := manager.descriptorMap()[DatabaseADK]
	if err := os.MkdirAll(descriptor.Path, 0o755); err != nil {
		t.Fatalf("create blocking directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(descriptor.Path, "keep"), []byte("x"), 0o600); err != nil {
		t.Fatalf("make blocking directory non-empty: %v", err)
	}
	if err := manager.writeMarker(marker{DatabaseIDs: []string{DatabaseADK}}); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if err := manager.ApplyPending(); err == nil {
		t.Fatal("expected database deletion failure")
	}
	if _, err := os.Stat(manager.markerPath()); err != nil {
		t.Fatalf("marker was removed after failure: %v", err)
	}
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	root := t.TempDir()
	return NewManager(filepath.Join(root, "settings.json"), filepath.Join(root, "backtest.db"))
}

func initializeDescriptors(t *testing.T, manager *Manager, incompatible map[string]bool) {
	t.Helper()
	for _, descriptor := range manager.descriptors {
		if incompatible[descriptor.ID] {
			db, err := sqlx.Open("sqlite", descriptor.Path)
			if err != nil {
				t.Fatalf("open incompatible %s: %v", descriptor.ID, err)
			}
			if _, err := db.Exec(`CREATE TABLE legacy (id TEXT PRIMARY KEY)`); err != nil {
				t.Fatalf("create incompatible %s: %v", descriptor.ID, err)
			}
			_ = db.Close()
			continue
		}
		initializeDescriptor(t, descriptor)
	}
}

func initializeDescriptor(t *testing.T, descriptor Descriptor) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(descriptor.Path), 0o755); err != nil {
		t.Fatalf("create descriptor directory: %v", err)
	}
	db, err := sqlx.Open("sqlite", descriptor.Path)
	if err != nil {
		t.Fatalf("open %s: %v", descriptor.ID, err)
	}
	var initializeErr error
	if _, managed := sqliteschema.DefinitionFor(descriptor.ID); managed {
		initializeErr = sqliteschema.InitializeCurrent(t.Context(), db, descriptor.Path, descriptor.ID)
	} else {
		initializeErr = sqliteschema.InitializeOrValidate(t.Context(), db, descriptor.Path, descriptor.ID, descriptor.Version, nil, nil)
	}
	if initializeErr != nil {
		t.Fatalf("initialize %s: %v", descriptor.ID, initializeErr)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close %s: %v", descriptor.ID, err)
	}
}
