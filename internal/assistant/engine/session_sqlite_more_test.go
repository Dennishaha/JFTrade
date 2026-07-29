package adk

import (
	"context"
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

	t.Run("metadata validation rejects missing rows query errors and version drift", func(t *testing.T) {
		db, err := sqliteconn.Open(filepath.Join(t.TempDir(), "metadata.db"))
		if err != nil {
			t.Fatalf("sqliteconn.Open: %v", err)
		}
		defer func() { jftradeCheckTestError(t, db.Close()) }()

		if err := sqliteschema.ValidateMetadata(ctx, db, "metadata.db", sqliteSessionComponent, sqliteSessionSchemaVersion); err == nil || !sqliteschema.IsIncompatible(err) {
			t.Fatalf("ValidateMetadata empty = %v, want incompatible", err)
		}
		if _, err := db.Exec(`CREATE TABLE ` + sqliteschema.MetadataTable + ` (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)`); err != nil {
			t.Fatalf("create metadata table: %v", err)
		}
		if err := sqliteschema.ValidateMetadata(ctx, db, "metadata.db", sqliteSessionComponent, sqliteSessionSchemaVersion); err == nil || !sqliteschema.IsIncompatible(err) {
			t.Fatalf("ValidateMetadata missing row = %v, want incompatible", err)
		}
		if _, err := db.Exec(`INSERT INTO `+sqliteschema.MetadataTable+` (component_id, version, created_at) VALUES (?, ?, ?)`, sqliteSessionComponent, sqliteSessionSchemaVersion-1, "now"); err != nil {
			t.Fatalf("insert metadata row: %v", err)
		}
		if err := sqliteschema.ValidateMetadata(ctx, db, "metadata.db", sqliteSessionComponent, sqliteSessionSchemaVersion); err == nil || !sqliteschema.IsIncompatible(err) {
			t.Fatalf("ValidateMetadata version drift = %v, want incompatible error", err)
		}

		closed, err := sqliteconn.Open(filepath.Join(t.TempDir(), "closed-metadata.db"))
		if err != nil {
			t.Fatalf("sqliteconn.Open closed metadata: %v", err)
		}
		if err := closed.Close(); err != nil {
			t.Fatalf("Close closed metadata: %v", err)
		}
		if err := sqliteschema.ValidateMetadata(ctx, closed, "closed-metadata.db", sqliteSessionComponent, sqliteSessionSchemaVersion); err == nil {
			t.Fatal("ValidateMetadata accepted closed database")
		}
	})

	t.Run("table validation reports missing tables and columns", func(t *testing.T) {
		db, err := sqliteconn.Open(filepath.Join(t.TempDir(), "validate-tables.db"))
		if err != nil {
			t.Fatalf("sqliteconn.Open: %v", err)
		}
		defer func() { jftradeCheckTestError(t, db.Close()) }()

		if err := validateSQLiteSessionTables(ctx, db, "validate-tables.db"); err == nil || !sqliteschema.IsIncompatible(err) || !strings.Contains(err.Error(), "columns do not match") {
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
		if err := validateSQLiteSessionTables(ctx, db, "validate-tables.db"); err == nil || !sqliteschema.IsIncompatible(err) || !strings.Contains(err.Error(), "columns do not match") {
			t.Fatalf("validateSQLiteSessionTables missing column err = %v", err)
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
