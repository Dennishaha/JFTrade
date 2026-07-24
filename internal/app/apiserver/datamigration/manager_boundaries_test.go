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
	missing := Descriptor{ID: "missing", Path: filepath.Join(root, "missing.db"), Version: 1}
	if status := inspectDatabase(t.Context(), missing); status.Status != "missing" || status.CurrentVersion != nil {
		t.Fatalf("missing status = %#v", status)
	}

	directory := Descriptor{ID: "directory", Path: filepath.Join(root, "directory.db"), Version: 1}
	if err := os.Mkdir(directory.Path, 0o755); err != nil {
		t.Fatalf("mkdir database path: %v", err)
	}
	if status := inspectDatabase(t.Context(), directory); status.Status != "unavailable" || status.Error != "database path is not a regular file" {
		t.Fatalf("directory status = %#v", status)
	}

	legacy := Descriptor{ID: DatabaseStrategy, Path: filepath.Join(root, "legacy.db"), Version: sqliteschema.StrategyVersion}
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
	if status := inspectDatabase(t.Context(), legacy); status.Status != "incompatible" || !strings.Contains(status.Error, "schema metadata is missing") {
		t.Fatalf("legacy status = %#v", status)
	}

	versioned := Descriptor{ID: DatabaseStrategy, Path: filepath.Join(root, "versioned.db"), Version: sqliteschema.StrategyVersion}
	initializeDescriptor(t, versioned)
	versionDB, err := sqlx.Open("sqlite", versioned.Path)
	if err != nil {
		t.Fatalf("open versioned database: %v", err)
	}
	if _, err := versionDB.Exec(`UPDATE `+sqliteschema.MetadataTable+` SET version = ? WHERE component_id = ?`, sqliteschema.StrategyVersion+1, versioned.ID); err != nil {
		t.Fatalf("update schema version: %v", err)
	}
	if err := versionDB.Close(); err != nil {
		t.Fatalf("close versioned database: %v", err)
	}
	status := inspectDatabase(t.Context(), versioned)
	if status.Status != "incompatible" || status.CurrentVersion == nil || *status.CurrentVersion != sqliteschema.StrategyVersion+1 || !strings.Contains(status.Error, "does not match required version") {
		t.Fatalf("versioned status = %#v", status)
	}

	readyDescriptor := Descriptor{ID: DatabaseStrategy, Path: filepath.Join(root, "ready.db"), Version: sqliteschema.StrategyVersion}
	initializeDescriptor(t, readyDescriptor)
	ready := inspectDatabase(t.Context(), readyDescriptor)
	if ready.Status != "ready" || ready.CurrentVersion == nil || *ready.CurrentVersion != sqliteschema.StrategyVersion || ready.Error != "" {
		t.Fatalf("ready status = %#v", ready)
	}
}

func TestInspectDatabaseRejectsManifestDrift(t *testing.T) {
	descriptor := Descriptor{
		ID:      DatabaseStrategy,
		Path:    filepath.Join(t.TempDir(), "strategy.db"),
		Version: sqliteschema.StrategyVersion,
	}
	initializeDescriptor(t, descriptor)
	db, err := sqlx.Open("sqlite", descriptor.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE unknown_application_table (id TEXT PRIMARY KEY)`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	status := inspectDatabase(t.Context(), descriptor)
	if status.Status != "incompatible" || status.CurrentVersion == nil || !strings.Contains(status.Error, "unknown application table") {
		t.Fatalf("manifest drift status = %#v", status)
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
	tempTarget := filepath.Join(t.TempDir(), "must-not-change")
	if err := os.WriteFile(tempTarget, []byte("unchanged"), 0o600); err != nil {
		t.Fatalf("write marker symlink target: %v", err)
	}
	if err := os.Symlink(tempTarget, tempBlocked.markerPath()+".tmp"); err != nil {
		t.Fatalf("create marker temp symlink: %v", err)
	}
	if err := tempBlocked.writeMarker(marker{DatabaseIDs: []string{DatabaseADK}}); err == nil {
		t.Fatal("writeMarker(blocked temp) error = nil")
	}
	if raw, err := os.ReadFile(tempTarget); err != nil || string(raw) != "unchanged" {
		t.Fatalf("marker temp symlink target changed: %q, %v", raw, err)
	}

	renameBlocked := newTestManager(t)
	if err := os.Mkdir(renameBlocked.markerPath(), 0o755); err != nil {
		t.Fatalf("mkdir marker destination: %v", err)
	}
	if err := renameBlocked.writeMarker(marker{DatabaseIDs: []string{DatabaseADK}}); err == nil {
		t.Fatal("writeMarker(blocked rename) error = nil")
	}

	for _, test := range []struct {
		name     string
		writeErr error
		closeErr error
	}{
		{name: "write", writeErr: errors.New("marker write failed")},
		{name: "close", closeErr: errors.New("marker close failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			faultManager := newTestManager(t)
			faultManager.openMarkerTemp = func(string) (markerTemporaryFile, error) {
				return &markerFaultFile{writeErr: test.writeErr, closeErr: test.closeErr}, nil
			}
			err := faultManager.writeMarker(marker{DatabaseIDs: []string{DatabaseADK}})
			if err == nil || !strings.Contains(err.Error(), "marker "+test.name+" failed") {
				t.Fatalf("writeMarker(%s failure) error = %v", test.name, err)
			}
		})
	}
}

type markerFaultFile struct {
	writeErr error
	closeErr error
}

func (f *markerFaultFile) Write(data []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(data), nil
}

func (f *markerFaultFile) Close() error {
	return f.closeErr
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

	// A NUL byte is rejected by os.Stat on every supported platform. Using an
	// invalid path keeps this boundary deterministic without requiring the
	// symbolic-link privilege that is disabled on a default Windows install.
	invalidPath := filepath.Join(t.TempDir(), "invalid\x00.db")
	status := inspectDatabase(t.Context(), Descriptor{ID: "invalid", Path: invalidPath, Version: 1})
	if status.Status != "unavailable" || status.Error == "" {
		t.Fatalf("invalid-path status = %#v", status)
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
