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
	ComponentID   = sqliteschema.DatabaseWatchlist
	SchemaVersion = sqliteschema.WatchlistVersion
)

func initializeSchema(ctx context.Context, db *sqliteconn.DB, path string) error {
	if err := sqliteschema.InitializeCurrent(ctx, db, path, ComponentID); err != nil {
		return err
	}
	return db.WriteTx(ctx, nil, func(tx *sqliteconn.Tx) error {
		return ensureDefaultGroup(ctx, tx)
	})
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
