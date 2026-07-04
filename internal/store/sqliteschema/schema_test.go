package sqliteschema

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func TestInitializeOrValidateStrictSchemaForAllDatabases(t *testing.T) {
	components := []string{"backtest", "backtest-runs", "strategy", "execution-orders", "adk", "adk-session"}
	for _, component := range components {
		t.Run(component, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), component+".db")
			db := openTestDB(t, path)
			statements := []string{`CREATE TABLE records (id TEXT PRIMARY KEY, value TEXT NOT NULL)`}
			validate := func(ctx context.Context, db Database) error {
				return ValidateTable(ctx, db, "records", []string{"id:TEXT:1", "value:TEXT:0"})
			}
			if err := InitializeOrValidate(ctx, db, path, component, 1, statements, validate); err != nil {
				t.Fatalf("create current database: %v", err)
			}
			if err := InitializeOrValidate(ctx, db, path, component, 1, statements, validate); err != nil {
				t.Fatalf("open current database: %v", err)
			}
			closeTestDB(t, db)

			t.Run("missing metadata is not repaired", func(t *testing.T) {
				legacyPath := filepath.Join(t.TempDir(), "legacy.db")
				legacy := openTestDB(t, legacyPath)
				if _, err := legacy.Exec(`CREATE TABLE records (id TEXT PRIMARY KEY, value TEXT NOT NULL);
					INSERT INTO records (id, value) VALUES ('keep', 'original')`); err != nil {
					t.Fatalf("create legacy schema: %v", err)
				}
				err := InitializeOrValidate(ctx, legacy, legacyPath, component, 1, statements, validate)
				if !IsIncompatible(err) {
					t.Fatalf("missing metadata error = %v", err)
				}
				var value string
				if err := legacy.Get(&value, `SELECT value FROM records WHERE id='keep'`); err != nil || value != "original" {
					t.Fatalf("legacy data changed: value=%q err=%v", value, err)
				}
				closeTestDB(t, legacy)
			})

			t.Run("wrong version", func(t *testing.T) {
				versionPath := filepath.Join(t.TempDir(), "version.db")
				versionDB := openTestDB(t, versionPath)
				if err := InitializeOrValidate(ctx, versionDB, versionPath, component, 1, statements, validate); err != nil {
					t.Fatalf("initialize version db: %v", err)
				}
				if _, err := versionDB.Exec(`UPDATE `+MetadataTable+` SET version=99 WHERE component_id=?`, component); err != nil {
					t.Fatalf("change version: %v", err)
				}
				if err := InitializeOrValidate(ctx, versionDB, versionPath, component, 1, statements, validate); !IsIncompatible(err) {
					t.Fatalf("version error = %v", err)
				}
				closeTestDB(t, versionDB)
			})

			t.Run("wrong structure", func(t *testing.T) {
				structurePath := filepath.Join(t.TempDir(), "structure.db")
				structureDB := openTestDB(t, structurePath)
				if _, err := structureDB.Exec(`CREATE TABLE records (id TEXT PRIMARY KEY);
					CREATE TABLE `+MetadataTable+` (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL);
					INSERT INTO `+MetadataTable+` VALUES (?, 1, 'now')`, component); err != nil {
					t.Fatalf("create wrong structure: %v", err)
				}
				if err := InitializeOrValidate(ctx, structureDB, structurePath, component, 1, statements, validate); !IsIncompatible(err) {
					t.Fatalf("structure error = %v", err)
				}
				closeTestDB(t, structureDB)
			})
		})
	}
}

func openTestDB(t *testing.T, path string) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func closeTestDB(t *testing.T, db *sqlx.DB) {
	t.Helper()
	if err := db.Close(); err != nil && err != sql.ErrConnDone {
		t.Fatalf("close sqlite: %v", err)
	}
}

func TestNewDatabaseFailureDoesNotLeaveSchemaMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failed.db")
	db := openTestDB(t, path)
	err := InitializeOrValidate(context.Background(), db, path, "adk", 1, []string{
		`CREATE TABLE records (id TEXT PRIMARY KEY)`,
		`THIS IS NOT SQL`,
	}, nil)
	if err == nil {
		t.Fatal("expected initialization failure")
	}
	closeTestDB(t, db)
	info, statErr := os.Stat(path)
	if statErr == nil && info.Size() > 0 {
		raw, openErr := sql.Open("sqlite", path)
		if openErr != nil {
			t.Fatalf("inspect failed database: %v", openErr)
		}
		defer func() {
			if err := raw.Close(); err != nil && err != sql.ErrConnDone {
				t.Fatalf("close sqlite inspector: %v", err)
			}
		}()
		var count int
		if err := raw.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name=?`, MetadataTable).Scan(&count); err != nil {
			t.Fatalf("inspect metadata: %v", err)
		}
		if count != 0 {
			t.Fatal("failed transaction left schema metadata")
		}
	}
}
