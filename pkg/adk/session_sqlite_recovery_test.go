package adk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	adksession "google.golang.org/adk/session"
)

func TestSQLiteSessionServiceRejectsUnavailableAndIncompatibleDatabases(t *testing.T) {
	root := t.TempDir()
	blockingFile := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blockingFile, []byte("blocked"), 0o600); err != nil {
		t.Fatalf("write blocking file: %v", err)
	}
	if _, err := NewSQLiteSessionService(filepath.Join(blockingFile, "adk-session.db")); err == nil {
		t.Fatal("NewSQLiteSessionService accepted path below regular file")
	}

	legacyPath := filepath.Join(root, "legacy-session.db")
	legacyDB, err := sqliteconn.Open(legacyPath)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := legacyDB.Exec(`CREATE TABLE sessions (id TEXT PRIMARY KEY, payload TEXT)`); err != nil {
		t.Fatalf("create incompatible sessions table: %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}
	if _, err := NewSQLiteSessionService(legacyPath); err == nil {
		t.Fatal("NewSQLiteSessionService accepted incompatible sessions schema")
	}
}

func TestSQLiteSessionSchemaHealthReportsMissingAndClosedConnections(t *testing.T) {
	if ready, err := sqliteSessionSchemaReady(nil); err == nil || ready || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil schema readiness = %v/%v", ready, err)
	}
	if err := ValidateSQLiteSessionService(adksession.InMemoryService()); err == nil || !strings.Contains(err.Error(), "schema is unavailable") {
		t.Fatalf("in-memory service validation error = %v", err)
	}
	if err := ValidateSQLiteSessionService(nil); err == nil || !strings.Contains(err.Error(), "schema is unavailable") {
		t.Fatalf("nil service validation error = %v", err)
	}

	db, err := sqliteconn.Open(filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatalf("open empty database: %v", err)
	}
	ready, err := sqliteSessionSchemaReady(db)
	if err != nil || ready {
		t.Fatalf("empty schema readiness = %v/%v, want false/nil", ready, err)
	}
	if exists, err := sqliteTableExists(db, "missing"); err != nil || exists {
		t.Fatalf("missing table existence = %v/%v", exists, err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close empty database: %v", err)
	}
	if _, err := sqliteTableExists(db, "sessions"); err == nil {
		t.Fatal("sqliteTableExists accepted closed database")
	}
	if _, err := sqliteSessionSchemaReady(db); err == nil {
		t.Fatal("sqliteSessionSchemaReady accepted closed database")
	}
}

func TestSQLiteSessionServiceCloseNilBoundaries(t *testing.T) {
	var nilService *SQLiteSessionService
	if err := nilService.Close(); err != nil {
		t.Fatalf("nil service Close: %v", err)
	}
	if err := (&SQLiteSessionService{}).Close(); err != nil {
		t.Fatalf("service without database Close: %v", err)
	}
	if err := CloseSessionService(adksession.InMemoryService()); err != nil {
		t.Fatalf("CloseSessionService non-closer: %v", err)
	}
}
