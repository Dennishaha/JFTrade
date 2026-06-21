package datamigration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
	if err := sqliteschema.InitializeOrValidate(t.Context(), db, descriptor.Path, descriptor.ID, SchemaVersion, nil, nil); err != nil {
		t.Fatalf("initialize %s: %v", descriptor.ID, err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close %s: %v", descriptor.ID, err)
	}
}
