package research

import (
	"context"
	"fmt"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
)

const (
	ComponentID   = "research"
	SchemaVersion = 1
)

type migration struct {
	version    int
	statements []string
}

var migrations = []migration{{
	version: 1,
	statements: []string{
		`CREATE TABLE research_screen_presets (
			preset_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			name_key TEXT NOT NULL UNIQUE,
			query_schema_version INTEGER NOT NULL,
			query_json TEXT NOT NULL,
			revision INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX research_screen_presets_updated_at ON research_screen_presets(updated_at DESC, preset_id)`,
	},
}}

func migrate(ctx context.Context, db *sqliteconn.DB) error {
	tx, err := db.BeginWrite(ctx, nil)
	if err != nil {
		return err
	}
	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS research_schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create research migration ledger: %w", err)
	}
	var current int
	if err := tx.GetContext(ctx, &current, `SELECT COALESCE(MAX(version), 0) FROM research_schema_migrations`); err != nil {
		return fmt.Errorf("read research schema version: %w", err)
	}
	if current > SchemaVersion {
		return fmt.Errorf("research database schema version %d is newer than supported version %d", current, SchemaVersion)
	}
	for _, candidate := range migrations {
		if candidate.version <= current {
			continue
		}
		for _, statement := range candidate.statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply research migration %d: %w", candidate.version, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO research_schema_migrations (version, applied_at) VALUES (?, ?)`, candidate.version, nowText()); err != nil {
			return fmt.Errorf("record research migration %d: %w", candidate.version, err)
		}
		current = candidate.version
	}
	if current != SchemaVersion {
		return fmt.Errorf("research database schema version %d does not match required version %d", current, SchemaVersion)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+sqliteschema.MetadataTable+` (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create research schema metadata: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO `+sqliteschema.MetadataTable+` (component_id, version, created_at) VALUES (?, ?, ?)
		ON CONFLICT(component_id) DO UPDATE SET version = excluded.version`, ComponentID, SchemaVersion, nowText()); err != nil {
		return fmt.Errorf("record research schema metadata: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

func nowText() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
