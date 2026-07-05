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

func TestSQLiteSessionDirectBoundaryBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("prepare schema rejects unavailable and closed databases", func(t *testing.T) {
		if err := prepareSQLiteSessionSchema(ctx, nil, adksession.InMemoryService(), "missing.db"); err == nil || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("prepareSQLiteSessionSchema(nil) err = %v", err)
		}

		db, err := sqliteconn.Open(filepath.Join(t.TempDir(), "closed.db"))
		if err != nil {
			t.Fatalf("sqliteconn.Open: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := prepareSQLiteSessionSchema(ctx, db, adksession.InMemoryService(), "closed.db"); err == nil {
			t.Fatal("prepareSQLiteSessionSchema accepted a closed database")
		}
	})

	t.Run("metadata validation handles missing rows query errors and version drift", func(t *testing.T) {
		db, err := sqliteconn.Open(filepath.Join(t.TempDir(), "metadata.db"))
		if err != nil {
			t.Fatalf("sqliteconn.Open: %v", err)
		}
		defer func() { jftradeCheckTestError(t, db.Close()) }()

		if current, err := validateSQLiteSessionMetadata(ctx, db, "metadata.db"); err != nil || current {
			t.Fatalf("validateSQLiteSessionMetadata empty = %v/%v, want false/nil", current, err)
		}
		if _, err := db.Exec(`CREATE TABLE ` + sqliteschema.MetadataTable + ` (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)`); err != nil {
			t.Fatalf("create metadata table: %v", err)
		}
		if current, err := validateSQLiteSessionMetadata(ctx, db, "metadata.db"); err != nil || current {
			t.Fatalf("validateSQLiteSessionMetadata missing row = %v/%v, want false/nil", current, err)
		}
		if _, err := db.Exec(`INSERT INTO `+sqliteschema.MetadataTable+` (component_id, version, created_at) VALUES (?, ?, ?)`, sqliteSessionComponent, sqliteSessionSchemaVersion-1, "now"); err != nil {
			t.Fatalf("insert metadata row: %v", err)
		}
		if current, err := validateSQLiteSessionMetadata(ctx, db, "metadata.db"); err == nil || current || !sqliteschema.IsIncompatible(err) {
			t.Fatalf("validateSQLiteSessionMetadata version drift = %v/%v, want incompatible error", current, err)
		}

		closed, err := sqliteconn.Open(filepath.Join(t.TempDir(), "closed-metadata.db"))
		if err != nil {
			t.Fatalf("sqliteconn.Open closed metadata: %v", err)
		}
		if err := closed.Close(); err != nil {
			t.Fatalf("Close closed metadata: %v", err)
		}
		if _, err := validateSQLiteSessionMetadata(ctx, closed, "closed-metadata.db"); err == nil {
			t.Fatal("validateSQLiteSessionMetadata accepted closed database")
		}
	})

	t.Run("ensure metadata reports create and insert failures", func(t *testing.T) {
		closed, err := sqliteconn.Open(filepath.Join(t.TempDir(), "closed-ensure.db"))
		if err != nil {
			t.Fatalf("sqliteconn.Open closed ensure: %v", err)
		}
		if err := closed.Close(); err != nil {
			t.Fatalf("Close closed ensure: %v", err)
		}
		if err := ensureSQLiteSessionMetadata(ctx, closed); err == nil || !strings.Contains(err.Error(), "initialize ADK session schema metadata") {
			t.Fatalf("ensureSQLiteSessionMetadata closed err = %v", err)
		}

		db, err := sqliteconn.Open(filepath.Join(t.TempDir(), "triggered-ensure.db"))
		if err != nil {
			t.Fatalf("sqliteconn.Open trigger ensure: %v", err)
		}
		defer func() { jftradeCheckTestError(t, db.Close()) }()
		if _, err := db.Exec(`CREATE TABLE ` + sqliteschema.MetadataTable + ` (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)`); err != nil {
			t.Fatalf("create metadata table: %v", err)
		}
		if _, err := db.Exec(`CREATE TRIGGER fail_meta_insert BEFORE INSERT ON ` + sqliteschema.MetadataTable + ` BEGIN SELECT RAISE(FAIL, 'meta insert failed'); END;`); err != nil {
			t.Fatalf("create trigger: %v", err)
		}
		if err := ensureSQLiteSessionMetadata(ctx, db); err == nil || !strings.Contains(err.Error(), "record ADK session schema metadata") {
			t.Fatalf("ensureSQLiteSessionMetadata insert err = %v", err)
		}
	})

	t.Run("table validation reports missing tables and columns", func(t *testing.T) {
		db, err := sqliteconn.Open(filepath.Join(t.TempDir(), "validate-tables.db"))
		if err != nil {
			t.Fatalf("sqliteconn.Open: %v", err)
		}
		defer func() { jftradeCheckTestError(t, db.Close()) }()

		if err := validateSQLiteSessionTables(ctx, db, "validate-tables.db"); err == nil || !sqliteschema.IsIncompatible(err) || !strings.Contains(err.Error(), "table is missing") {
			t.Fatalf("validateSQLiteSessionTables missing table err = %v", err)
		}
		for _, statement := range []string{
			`CREATE TABLE sessions (app_name TEXT, user_id TEXT)`,
			`CREATE TABLE events (id TEXT, app_name TEXT, user_id TEXT, session_id TEXT)`,
			`CREATE TABLE app_states (app_name TEXT)`,
			`CREATE TABLE user_states (app_name TEXT, user_id TEXT)`,
		} {
			if _, err := db.Exec(statement); err != nil {
				t.Fatalf("init table %q: %v", statement, err)
			}
		}
		if err := validateSQLiteSessionTables(ctx, db, "validate-tables.db"); err == nil || !sqliteschema.IsIncompatible(err) || !strings.Contains(err.Error(), "sessions.id") {
			t.Fatalf("validateSQLiteSessionTables missing column err = %v", err)
		}
	})

	t.Run("remove and rebuild helpers reject invalid paths", func(t *testing.T) {
		if _, err := rebuildSQLiteSessionService("   "); err == nil || !strings.Contains(err.Error(), "path is empty") {
			t.Fatalf("rebuildSQLiteSessionService(blank) err = %v", err)
		}
		if err := removeSQLiteDatabaseFiles(" "); err == nil || !strings.Contains(err.Error(), "path is empty") {
			t.Fatalf("removeSQLiteDatabaseFiles(blank) err = %v", err)
		}

		root := filepath.Join(t.TempDir(), "blocked-db")
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("MkdirAll blocked-db: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "child"), []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile blocked child: %v", err)
		}
		if err := removeSQLiteDatabaseFiles(root); err == nil || !strings.Contains(err.Error(), "remove ADK session database") {
			t.Fatalf("removeSQLiteDatabaseFiles(non-empty dir) err = %v", err)
		}
	})

	t.Run("table column existence rejects nil and closed databases", func(t *testing.T) {
		if exists, err := sqliteTableColumnExists(ctx, nil, "sessions", "id"); err == nil || exists || !strings.Contains(err.Error(), "unavailable") {
			t.Fatalf("sqliteTableColumnExists(nil) = %v/%v, want unavailable", exists, err)
		}

		db, err := sqliteconn.Open(filepath.Join(t.TempDir(), "column-closed.db"))
		if err != nil {
			t.Fatalf("sqliteconn.Open: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if exists, err := sqliteTableColumnExists(ctx, db, "sessions", "id"); err == nil || exists {
			t.Fatalf("sqliteTableColumnExists(closed) = %v/%v, want error", exists, err)
		}
	})
}
