package adk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	adksession "google.golang.org/adk/v2/session"
)

func TestSQLiteSessionServiceRejectsUnavailableAndRebuildsIncompatibleDatabases(t *testing.T) {
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
	service, err := NewSQLiteSessionService(legacyPath)
	if err != nil {
		t.Fatalf("NewSQLiteSessionService did not rebuild incompatible schema: %v", err)
	}
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close rebuilt service: %v", err)
		}
	}()
	if err := ValidateSQLiteSessionService(service); err != nil {
		t.Fatalf("validate rebuilt service: %v", err)
	}
	if exists, err := sqliteTableColumnExists(context.Background(), service.db, "sessions", "payload"); err != nil || exists {
		t.Fatalf("legacy sessions.payload exists = %v/%v, want false/nil", exists, err)
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

func TestSQLiteSessionServiceRebuildsV1SchemaWithoutPreservingEvents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "adk-session-v1.db")
	db, err := sqliteconn.Open(path)
	if err != nil {
		t.Fatalf("open v1 database: %v", err)
	}
	for _, statement := range []string{
		"CREATE TABLE sessions (app_name TEXT, user_id TEXT, id TEXT, state TEXT, create_time TIMESTAMP, update_time TIMESTAMP, PRIMARY KEY (app_name,user_id,id))",
		"CREATE TABLE events (id TEXT, app_name TEXT, user_id TEXT, session_id TEXT, invocation_id TEXT, author TEXT, actions BLOB, long_running_tool_ids_json TEXT, branch TEXT, timestamp TIMESTAMP, content TEXT, grounding_metadata TEXT, custom_metadata TEXT, usage_metadata TEXT, citation_metadata TEXT, partial NUMERIC, turn_complete NUMERIC, error_code TEXT, error_message TEXT, interrupted NUMERIC, PRIMARY KEY (id,app_name,user_id,session_id), FOREIGN KEY (app_name,user_id,session_id) REFERENCES sessions(app_name,user_id,id) ON DELETE CASCADE)",
		"CREATE TABLE app_states (app_name TEXT PRIMARY KEY, state TEXT, update_time TIMESTAMP)",
		"CREATE TABLE user_states (app_name TEXT, user_id TEXT, state TEXT, update_time TIMESTAMP, PRIMARY KEY (app_name,user_id))",
		"CREATE TABLE " + sqliteschema.MetadataTable + " (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)",
		"INSERT INTO " + sqliteschema.MetadataTable + " (component_id, version, created_at) VALUES ('adk-session', 1, 'now')",
		"INSERT INTO sessions (app_name, user_id, id, state) VALUES ('app', 'user', 'session', '{}')",
		"INSERT INTO events (id, app_name, user_id, session_id, invocation_id, author, branch, content) VALUES ('event', 'app', 'user', 'session', 'invocation', 'agent', 'branch', '{}')",
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("initialize v1 database with %q: %v", statement, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v1 database: %v", err)
	}

	service, err := NewSQLiteSessionService(path)
	if err != nil {
		t.Fatalf("rebuild v1 database: %v", err)
	}
	defer func() {
		if err := service.Close(); err != nil {
			t.Fatalf("close rebuilt service: %v", err)
		}
	}()
	if err := ValidateSQLiteSessionService(service); err != nil {
		t.Fatalf("validate rebuilt service: %v", err)
	}
	for _, column := range []string{"routes_json", "output_json", "node_info_json", "requested_input_json", "isolation_scope"} {
		exists, err := sqliteTableColumnExists(context.Background(), service.db, "events", column)
		if err != nil || !exists {
			t.Fatalf("rebuilt events.%s exists = %v/%v", column, exists, err)
		}
	}
	var version int
	if err := service.db.QueryRowxContext(context.Background(),
		`SELECT version FROM `+sqliteschema.MetadataTable+` WHERE component_id = ?`,
		"adk-session",
	).Scan(&version); err != nil || version != sqliteSessionSchemaVersion {
		t.Fatalf("rebuilt schema version = %d/%v, want %d", version, err, sqliteSessionSchemaVersion)
	}
	var eventCount int
	if err := service.db.QueryRowxContext(context.Background(), `SELECT COUNT(*) FROM events`).Scan(&eventCount); err != nil || eventCount != 0 {
		t.Fatalf("rebuilt event count = %d/%v, want 0", eventCount, err)
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
