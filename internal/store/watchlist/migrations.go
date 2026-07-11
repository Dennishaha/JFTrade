package watchlist

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	domain "github.com/jftrade/jftrade-main/internal/watchlist"
)

const (
	ComponentID   = "watchlist"
	SchemaVersion = 1
)

type migration struct {
	version    int
	statements []string
}

var migrations = []migration{{
	version: 1,
	statements: []string{
		`CREATE TABLE watchlist_groups (
			group_id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			name_key TEXT NOT NULL UNIQUE,
			is_default INTEGER NOT NULL DEFAULT 0,
			protected INTEGER NOT NULL DEFAULT 0,
			revision INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX watchlist_groups_one_default ON watchlist_groups(is_default) WHERE is_default = 1`,
		`CREATE TABLE watchlist_instruments (
			instrument_id TEXT PRIMARY KEY,
			market TEXT NOT NULL,
			symbol TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			instrument_type TEXT NOT NULL DEFAULT '',
			membership_revision INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE watchlist_memberships (
			group_id TEXT NOT NULL,
			instrument_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY (group_id, instrument_id)
		)`,
		`CREATE INDEX watchlist_memberships_instrument ON watchlist_memberships(instrument_id, group_id)`,
		`CREATE TABLE watchlist_sources (
			source_id TEXT PRIMARY KEY,
			broker TEXT NOT NULL,
			display_name TEXT NOT NULL,
			status TEXT NOT NULL,
			last_error TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE watchlist_remote_groups (
			source_id TEXT NOT NULL,
			remote_group_id TEXT NOT NULL,
			name TEXT NOT NULL,
			group_type TEXT NOT NULL,
			ambiguous INTEGER NOT NULL DEFAULT 0,
			member_count INTEGER NOT NULL DEFAULT 0,
			remote_hash TEXT NOT NULL DEFAULT '',
			observed_at TEXT NOT NULL,
			PRIMARY KEY (source_id, remote_group_id)
		)`,
		`CREATE TABLE watchlist_bindings (
			binding_id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			remote_group_id TEXT NOT NULL,
			remote_name TEXT NOT NULL,
			local_group_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE (source_id, remote_group_id)
		)`,
		`CREATE INDEX watchlist_bindings_local_group ON watchlist_bindings(local_group_id)`,
		`CREATE TABLE watchlist_remote_memberships (
			source_id TEXT NOT NULL,
			remote_group_id TEXT NOT NULL,
			instrument_id TEXT NOT NULL,
			remote_hash TEXT NOT NULL,
			observed_at TEXT NOT NULL,
			PRIMARY KEY (source_id, remote_group_id, instrument_id)
		)`,
		`CREATE TABLE watchlist_membership_origins (
			group_id TEXT NOT NULL,
			instrument_id TEXT NOT NULL,
			source_id TEXT NOT NULL,
			remote_group_id TEXT NOT NULL,
			last_imported_at TEXT NOT NULL,
			PRIMARY KEY (group_id, instrument_id, source_id, remote_group_id)
		)`,
		`CREATE INDEX watchlist_membership_origins_instrument ON watchlist_membership_origins(instrument_id, group_id)`,
		`CREATE TABLE watchlist_instrument_aliases (
			source_id TEXT NOT NULL,
			alias_kind TEXT NOT NULL,
			alias_value TEXT NOT NULL,
			instrument_id TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (source_id, alias_kind, alias_value)
		)`,
		`CREATE TABLE watchlist_import_previews (
			preview_id TEXT PRIMARY KEY,
			source_id TEXT NOT NULL,
			remote_group_id TEXT NOT NULL,
			remote_group_name TEXT NOT NULL,
			local_group_id TEXT NOT NULL DEFAULT '',
			new_group_name TEXT NOT NULL DEFAULT '',
			remote_hash TEXT NOT NULL,
			local_group_revision INTEGER NOT NULL,
			added_json TEXT NOT NULL,
			unchanged_json TEXT NOT NULL,
			local_only_json TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		)`,
		`CREATE INDEX watchlist_import_previews_expiry ON watchlist_import_previews(status, expires_at)`,
		`CREATE TABLE watchlist_import_runs (
			run_id TEXT PRIMARY KEY,
			preview_id TEXT NOT NULL,
			source_id TEXT NOT NULL,
			remote_group_id TEXT NOT NULL,
			remote_group_name TEXT NOT NULL,
			local_group_id TEXT NOT NULL,
			status TEXT NOT NULL,
			added_count INTEGER NOT NULL,
			removed_count INTEGER NOT NULL,
			unchanged_count INTEGER NOT NULL,
			remote_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			completed_at TEXT NOT NULL
		)`,
		`CREATE INDEX watchlist_import_runs_source ON watchlist_import_runs(source_id, run_id DESC)`,
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
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS watchlist_schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create watchlist migration ledger: %w", err)
	}
	var current int
	if err := tx.GetContext(ctx, &current, `SELECT COALESCE(MAX(version), 0) FROM watchlist_schema_migrations`); err != nil {
		return fmt.Errorf("read watchlist schema version: %w", err)
	}
	if current > SchemaVersion {
		return fmt.Errorf("watchlist database schema version %d is newer than supported version %d", current, SchemaVersion)
	}
	for _, candidate := range migrations {
		if candidate.version <= current {
			continue
		}
		for _, statement := range candidate.statements {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("apply watchlist migration %d: %w", candidate.version, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO watchlist_schema_migrations (version, applied_at) VALUES (?, ?)`, candidate.version, nowText()); err != nil {
			return fmt.Errorf("record watchlist migration %d: %w", candidate.version, err)
		}
		current = candidate.version
	}
	if current != SchemaVersion {
		return fmt.Errorf("watchlist database schema version %d does not match required version %d", current, SchemaVersion)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+sqliteschema.MetadataTable+` (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)`); err != nil {
		return fmt.Errorf("create watchlist schema metadata: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO `+sqliteschema.MetadataTable+` (component_id, version, created_at) VALUES (?, ?, ?)
		ON CONFLICT(component_id) DO UPDATE SET version = excluded.version`, ComponentID, SchemaVersion, nowText()); err != nil {
		return fmt.Errorf("record watchlist schema metadata: %w", err)
	}
	if err := ensureDefaultGroup(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rollback = false
	return nil
}

func ensureDefaultGroup(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}) error {
	now := nowText()
	if _, err := tx.ExecContext(ctx, `INSERT INTO watchlist_groups
		(group_id, name, name_key, is_default, protected, revision, created_at, updated_at)
		SELECT 'default', ?, ?, 1, 1, 1, ?, ?
		WHERE NOT EXISTS (SELECT 1 FROM watchlist_groups WHERE is_default = 1)`,
		domain.DefaultGroupName, domain.GroupNameKey(domain.DefaultGroupName), now, now); err != nil {
		return fmt.Errorf("ensure default watchlist group: %w", err)
	}
	return nil
}

func nowText() string { return time.Now().UTC().Format(time.RFC3339Nano) }
