package sqliteschema

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestIncompatibleErrorIncludesRecoveryContext(t *testing.T) {
	err := (&IncompatibleError{Component: "strategy", Path: "/data/strategy.db", Reason: "version drift"}).Error()
	if err != "strategy database schema is incompatible: version drift; rebuild database /data/strategy.db" {
		t.Fatalf("Error() = %q", err)
	}
}

func TestInitializeOrValidateRejectsUnavailableAndInvalidDatabasePaths(t *testing.T) {
	if err := InitializeOrValidate(t.Context(), nil, "/tmp/missing.db", "test", 1, nil, nil); err == nil || err.Error() != "test database is unavailable" {
		t.Fatalf("InitializeOrValidate(nil db) error = %v", err)
	}

	root := t.TempDir()
	invalidPath := filepath.Join(root, "invalid\x00.db")
	db := openTestDB(t, filepath.Join(root, "unused.db"))
	defer closeTestDB(t, db)
	if err := InitializeOrValidate(t.Context(), db, invalidPath, "test", 1, nil, nil); err == nil {
		t.Fatal("InitializeOrValidate(invalid path) error = nil")
	}

	closedPath := filepath.Join(root, "closed.db")
	closed := openTestDB(t, closedPath)
	closeTestDB(t, closed)
	if err := InitializeOrValidate(t.Context(), closed, closedPath, "test", 1, nil, nil); err == nil || !strings.Contains(err.Error(), "database is closed") {
		t.Fatalf("InitializeOrValidate(closed db) error = %v", err)
	}
}

func TestInitializeOrValidateHandlesBlankStatementsAndValidationErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.db")
	db := openTestDB(t, path)
	defer closeTestDB(t, db)

	incompatible := &IncompatibleError{Component: "test", Path: path, Reason: "records table drifted"}
	err := InitializeOrValidate(t.Context(), db, path, "test", 1, []string{
		" \t\n ",
		`CREATE TABLE records (id TEXT PRIMARY KEY)`,
	}, func(_ context.Context, _ *sqlx.DB) error {
		return incompatible
	})
	if !errors.Is(err, incompatible) {
		t.Fatalf("InitializeOrValidate(existing incompatible error) = %v", err)
	}

	conflictPath := filepath.Join(t.TempDir(), "metadata-conflict.db")
	conflictDB := openTestDB(t, conflictPath)
	defer closeTestDB(t, conflictDB)
	err = InitializeOrValidate(t.Context(), conflictDB, conflictPath, "test", 1, []string{
		`CREATE TABLE ` + MetadataTable + ` (component_id TEXT PRIMARY KEY)`,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "initialize test schema metadata") {
		t.Fatalf("InitializeOrValidate(metadata conflict) error = %v", err)
	}
}

func TestValidateMetadataClassifiesMissingAndUnreadableComponentRows(t *testing.T) {
	missingPath := filepath.Join(t.TempDir(), "missing-component.db")
	missingDB := openTestDB(t, missingPath)
	defer closeTestDB(t, missingDB)
	if _, err := missingDB.Exec(`CREATE TABLE ` + MetadataTable + ` (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL);
		INSERT INTO ` + MetadataTable + ` VALUES ('other', 1, 'now')`); err != nil {
		t.Fatalf("create missing-component metadata: %v", err)
	}
	err := ValidateMetadata(t.Context(), missingDB, missingPath, "test", 1)
	if !IsIncompatible(err) || !strings.Contains(err.Error(), "component metadata is missing") {
		t.Fatalf("ValidateMetadata(missing component) error = %v", err)
	}

	unreadablePath := filepath.Join(t.TempDir(), "unreadable-version.db")
	unreadableDB := openTestDB(t, unreadablePath)
	defer closeTestDB(t, unreadableDB)
	if _, err := unreadableDB.Exec(`CREATE TABLE ` + MetadataTable + ` (component_id TEXT PRIMARY KEY, version TEXT NOT NULL, created_at TEXT NOT NULL);
		INSERT INTO ` + MetadataTable + ` VALUES ('test', 'not-an-integer', 'now')`); err != nil {
		t.Fatalf("create unreadable metadata: %v", err)
	}
	if err := ValidateMetadata(t.Context(), unreadableDB, unreadablePath, "test", 1); err == nil || IsIncompatible(err) {
		t.Fatalf("ValidateMetadata(unreadable version) error = %v", err)
	}

	closedPath := filepath.Join(t.TempDir(), "closed.db")
	closedDB := openTestDB(t, closedPath)
	closeTestDB(t, closedDB)
	if err := ValidateMetadata(t.Context(), closedDB, closedPath, "test", 1); err == nil || !strings.Contains(err.Error(), "database is closed") {
		t.Fatalf("ValidateMetadata(closed db) error = %v", err)
	}
}

func TestDatabaseAndTableValidationFilesystemBoundaries(t *testing.T) {
	directoryPath := filepath.Join(t.TempDir(), "directory.db")
	if err := os.Mkdir(directoryPath, 0o755); err != nil {
		t.Fatalf("mkdir database path: %v", err)
	}
	if isNew, err := databaseIsNew(directoryPath); err == nil || isNew || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("databaseIsNew(directory) = (%t, %v)", isNew, err)
	}

	path := filepath.Join(t.TempDir(), "table.db")
	db := openTestDB(t, path)
	if _, err := db.Exec(`CREATE TABLE records (value TEXT NOT NULL, id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create reordered table: %v", err)
	}
	if err := ValidateTable(t.Context(), db, "records", []string{"id:TEXT:1", "value:TEXT:0"}); err == nil || !strings.Contains(err.Error(), "columns do not match") {
		t.Fatalf("ValidateTable(reordered columns) error = %v", err)
	}
	closeTestDB(t, db)
	if err := ValidateTable(t.Context(), db, "records", nil); err == nil || !strings.Contains(err.Error(), "database is closed") {
		t.Fatalf("ValidateTable(closed db) error = %v", err)
	}
}

func TestInitializeOrValidateRollsBackWhenDeferredConstraintFailsAtCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deferred-constraint.db")
	db := openTestDB(t, path)
	defer closeTestDB(t, db)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}
	err := InitializeOrValidate(t.Context(), db, path, "test", 1, []string{
		`CREATE TABLE parent (id INTEGER PRIMARY KEY)`,
		`CREATE TABLE child (parent_id INTEGER, FOREIGN KEY(parent_id) REFERENCES parent(id) DEFERRABLE INITIALLY DEFERRED)`,
		`INSERT INTO child (parent_id) VALUES (99)`,
	}, nil)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "foreign key") {
		t.Fatalf("InitializeOrValidate(deferred constraint) error = %v", err)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM sqlite_master WHERE name IN (?, ?, ?)`, "parent", "child", MetadataTable); err != nil {
		t.Fatalf("inspect rolled-back schema: %v", err)
	}
	if count != 0 {
		t.Fatalf("rolled-back schema left %d tables", count)
	}
}
