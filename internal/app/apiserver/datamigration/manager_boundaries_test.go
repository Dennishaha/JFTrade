package datamigration

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	"github.com/jmoiron/sqlx"
)

func TestManagerStatusesReflectRuntimeFailuresAndScheduledRebuilds(t *testing.T) {
	manager := newTestManager(t)
	initializeDescriptors(t, manager, nil)

	var nilManager *Manager
	nilManager.SetUnavailable(DatabaseADK, errors.New("ignored"))
	manager.SetUnavailable(DatabaseADK, nil)
	manager.SetUnavailable("unknown", errors.New("ignored"))
	manager.SetUnavailable(DatabaseADK, errors.New("adk startup failed"))
	manager.SetUnavailable(DatabaseStrategy, &sqliteschema.IncompatibleError{
		Component: DatabaseStrategy,
		Path:      manager.descriptorMap()[DatabaseStrategy].Path,
		Reason:    "legacy tables",
	})
	if err := manager.writeMarker(marker{DatabaseIDs: []string{DatabaseADK}}); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	statuses, err := manager.Statuses(t.Context())
	if err != nil {
		t.Fatalf("Statuses() error = %v", err)
	}
	adk := databaseStatusByID(t, statuses, DatabaseADK)
	if adk.Status != "unavailable" || adk.Error != "adk startup failed" || !adk.RebuildScheduled || !adk.RestartRequired {
		t.Fatalf("ADK status = %#v", adk)
	}
	strategy := databaseStatusByID(t, statuses, DatabaseStrategy)
	if strategy.Status != "incompatible" || !strings.Contains(strategy.Error, "legacy tables") || strategy.RebuildScheduled {
		t.Fatalf("strategy status = %#v", strategy)
	}
	backtest := databaseStatusByID(t, statuses, DatabaseBacktest)
	if backtest.Status != "ready" || backtest.ConfirmationText != "REBUILD "+DatabaseBacktest {
		t.Fatalf("backtest status = %#v", backtest)
	}
}

func TestManagerScheduleRebuildValidatesModesAndSelection(t *testing.T) {
	manager := newTestManager(t)
	initializeDescriptors(t, manager, nil)

	tests := []struct {
		name    string
		request RebuildRequest
		want    string
	}{
		{name: "no single id", request: RebuildRequest{Mode: "single", Confirmation: "REBUILD " + DatabaseADK}, want: "exactly one database id is required"},
		{name: "multiple single ids", request: RebuildRequest{Mode: "single", DatabaseIDs: []string{DatabaseADK, DatabaseStrategy}}, want: "exactly one database id is required"},
		{name: "batch confirmation", request: RebuildRequest{Mode: "incompatible", Confirmation: "wrong"}, want: "confirmation text does not match"},
		{name: "batch has no work", request: RebuildRequest{Mode: "incompatible", Confirmation: BatchConfirmationText}, want: "no databases require rebuild"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manager.ScheduleRebuild(t.Context(), tt.request)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("ScheduleRebuild() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestManagerPendingLifecycleHandlesMissingCorruptAndUnknownMarkers(t *testing.T) {
	manager := newTestManager(t)
	if err := manager.ApplyPending(); err != nil {
		t.Fatalf("ApplyPending(no marker) error = %v", err)
	}
	if err := manager.CompletePending(t.Context()); err != nil {
		t.Fatalf("CompletePending(no marker) error = %v", err)
	}

	if err := manager.writeMarker(marker{DatabaseIDs: []string{"unknown"}}); err != nil {
		t.Fatalf("write unknown marker: %v", err)
	}
	if err := manager.ApplyPending(); err == nil || !strings.Contains(err.Error(), "unknown database id") {
		t.Fatalf("ApplyPending(unknown marker) error = %v", err)
	}

	if err := manager.writeMarker(marker{DatabaseIDs: []string{DatabaseADK}}); err != nil {
		t.Fatalf("write pending marker: %v", err)
	}
	if err := manager.CompletePending(t.Context()); err == nil || !strings.Contains(err.Error(), "did not initialize successfully") {
		t.Fatalf("CompletePending(missing database) error = %v", err)
	}

	if err := os.WriteFile(manager.markerPath(), []byte("{"), 0o600); err != nil {
		t.Fatalf("write corrupt marker: %v", err)
	}
	if err := manager.ApplyPending(); err == nil || !strings.Contains(err.Error(), "decode database rebuild marker") {
		t.Fatalf("ApplyPending(corrupt marker) error = %v", err)
	}
	if err := manager.CompletePending(t.Context()); err == nil || !strings.Contains(err.Error(), "decode database rebuild marker") {
		t.Fatalf("CompletePending(corrupt marker) error = %v", err)
	}
}

func TestInspectDatabaseClassifiesFilesystemAndSchemaStates(t *testing.T) {
	root := t.TempDir()
	missing := Descriptor{ID: "missing", Path: filepath.Join(root, "missing.db"), Version: SchemaVersion}
	if status := inspectDatabase(t.Context(), missing); status.Status != "missing" || status.CurrentVersion != nil {
		t.Fatalf("missing status = %#v", status)
	}

	directory := Descriptor{ID: "directory", Path: filepath.Join(root, "directory.db"), Version: SchemaVersion}
	if err := os.Mkdir(directory.Path, 0o755); err != nil {
		t.Fatalf("mkdir database path: %v", err)
	}
	if status := inspectDatabase(t.Context(), directory); status.Status != "unavailable" || status.Error != "database path is not a regular file" {
		t.Fatalf("directory status = %#v", status)
	}

	legacy := Descriptor{ID: "legacy", Path: filepath.Join(root, "legacy.db"), Version: SchemaVersion}
	legacyDB, err := sqlx.Open("sqlite", legacy.Path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := legacyDB.Exec(`CREATE TABLE legacy (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}
	if status := inspectDatabase(t.Context(), legacy); status.Status != "incompatible" || status.Error != "schema metadata is missing or unreadable" {
		t.Fatalf("legacy status = %#v", status)
	}

	versioned := Descriptor{ID: "versioned", Path: filepath.Join(root, "versioned.db"), Version: SchemaVersion}
	initializeDescriptor(t, versioned)
	versionDB, err := sqlx.Open("sqlite", versioned.Path)
	if err != nil {
		t.Fatalf("open versioned database: %v", err)
	}
	if _, err := versionDB.Exec(`UPDATE `+sqliteschema.MetadataTable+` SET version = 2 WHERE component_id = ?`, versioned.ID); err != nil {
		t.Fatalf("update schema version: %v", err)
	}
	if err := versionDB.Close(); err != nil {
		t.Fatalf("close versioned database: %v", err)
	}
	status := inspectDatabase(t.Context(), versioned)
	if status.Status != "incompatible" || status.CurrentVersion == nil || *status.CurrentVersion != 2 || !strings.Contains(status.Error, "does not match required version") {
		t.Fatalf("versioned status = %#v", status)
	}

	initializeDescriptor(t, Descriptor{ID: "ready", Path: filepath.Join(root, "ready.db"), Version: SchemaVersion})
	ready := inspectDatabase(t.Context(), Descriptor{ID: "ready", Path: filepath.Join(root, "ready.db"), Version: SchemaVersion})
	if ready.Status != "ready" || ready.CurrentVersion == nil || *ready.CurrentVersion != SchemaVersion || ready.Error != "" {
		t.Fatalf("ready status = %#v", ready)
	}
}

func TestManagerMarkerPersistenceNormalizesAndSurfacesFilesystemErrors(t *testing.T) {
	manager := newTestManager(t)
	if err := os.WriteFile(manager.markerPath(), []byte(`{"databaseIds":[" strategy ","","adk","strategy"]}`), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	value, err := manager.readMarker()
	if err != nil {
		t.Fatalf("readMarker() error = %v", err)
	}
	if len(value.DatabaseIDs) != 2 || value.DatabaseIDs[0] != "adk" || value.DatabaseIDs[1] != "strategy" {
		t.Fatalf("normalized database IDs = %#v", value.DatabaseIDs)
	}

	if err := os.Remove(manager.markerPath()); err != nil {
		t.Fatalf("remove marker: %v", err)
	}
	if err := os.Mkdir(manager.markerPath(), 0o755); err != nil {
		t.Fatalf("mkdir marker path: %v", err)
	}
	if _, err := manager.readMarker(); err == nil {
		t.Fatal("readMarker(directory) error = nil")
	}

	root := t.TempDir()
	blockedParent := filepath.Join(root, "blocked")
	if err := os.WriteFile(blockedParent, []byte("file"), 0o600); err != nil {
		t.Fatalf("write blocked parent: %v", err)
	}
	blockedManager := NewManager(filepath.Join(blockedParent, "settings.json"), filepath.Join(root, "backtest.db"))
	if err := blockedManager.writeMarker(marker{DatabaseIDs: []string{DatabaseADK}}); err == nil {
		t.Fatal("writeMarker(blocked parent) error = nil")
	}

	tempBlocked := newTestManager(t)
	if err := os.Mkdir(tempBlocked.markerPath()+".tmp", 0o755); err != nil {
		t.Fatalf("mkdir marker temp path: %v", err)
	}
	if err := tempBlocked.writeMarker(marker{DatabaseIDs: []string{DatabaseADK}}); err == nil {
		t.Fatal("writeMarker(blocked temp) error = nil")
	}

	renameBlocked := newTestManager(t)
	if err := os.Mkdir(renameBlocked.markerPath(), 0o755); err != nil {
		t.Fatalf("mkdir marker destination: %v", err)
	}
	if err := renameBlocked.writeMarker(marker{DatabaseIDs: []string{DatabaseADK}}); err == nil {
		t.Fatal("writeMarker(blocked rename) error = nil")
	}
}

func TestManagerPropagatesUnreadableMarkerAndDatabaseStatErrors(t *testing.T) {
	manager := newTestManager(t)
	if err := os.Mkdir(manager.markerPath(), 0o755); err != nil {
		t.Fatalf("mkdir marker path: %v", err)
	}
	if _, err := manager.Statuses(t.Context()); err == nil {
		t.Fatal("Statuses(unreadable marker) error = nil")
	}
	if _, err := manager.ScheduleRebuild(t.Context(), RebuildRequest{
		Mode:         "single",
		DatabaseIDs:  []string{DatabaseADK},
		Confirmation: "REBUILD " + DatabaseADK,
	}); err == nil {
		t.Fatal("ScheduleRebuild(unreadable marker) error = nil")
	}

	root := t.TempDir()
	loopPath := filepath.Join(root, "loop.db")
	if err := os.Symlink(loopPath, loopPath); err != nil {
		t.Fatalf("create symlink loop: %v", err)
	}
	status := inspectDatabase(t.Context(), Descriptor{ID: "loop", Path: loopPath, Version: SchemaVersion})
	if status.Status != "unavailable" || status.Error == "" {
		t.Fatalf("symlink-loop status = %#v", status)
	}
}

func databaseStatusByID(t *testing.T, statuses []DatabaseStatus, id string) DatabaseStatus {
	t.Helper()
	for _, status := range statuses {
		if status.ID == id {
			return status
		}
	}
	t.Fatalf("database status %q not found", id)
	return DatabaseStatus{}
}
