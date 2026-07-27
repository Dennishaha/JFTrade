package sqliteconn

import (
	"context"
	"fmt"
)

// Compact serializes checkpoint and VACUUM with the database write queue.
// Store-owned maintenance adapters use this method instead of exposing their
// raw SQLite connection to application assembly.
func (db *DB) Compact(ctx context.Context) error {
	if db == nil {
		return fmt.Errorf("database is unavailable")
	}
	if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `VACUUM`)
	return err
}
