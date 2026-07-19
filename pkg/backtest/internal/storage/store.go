// Package storage contains the SQLite-backed Futu backtest store
// implementation and compact local_klines schema helpers.
package storage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	"github.com/jftrade/jftrade-main/pkg/besteffort"
)

// FutuKLineStore implements service.BackTestable for Futu data stored in
// SQLite. It uses a compact backtest-only local_klines table keyed by
// symbol+interval+rehab_type (table-level) and end_time (row-level).
type FutuKLineStore struct {
	db                *sqliteconn.DB
	dbPath            string
	rehabType         atomic.Value // string: "forward" | "backward" | "none"
	readSessionScope  atomic.Value // string
	writeSessionScope atomic.Value // string
	tableExistsCache  sync.Map
}

// NewFutuKLineStore opens or creates a SQLite database at the given path and
// lazily creates per-series tables as data is inserted.
func NewFutuKLineStore(dbPath string) (*FutuKLineStore, error) {
	db, err := sqliteconn.OpenX(dbPath, sqliteconn.WithMaxOpenConns(8))
	if err != nil {
		return nil, fmt.Errorf("open sqlite backtest store: %w", err)
	}
	store := &FutuKLineStore{
		db:     db,
		dbPath: dbPath,
	}
	store.rehabType.Store(normalizeRehabTypeName("forward"))
	store.readSessionScope.Store(klineReadSessionScopeAuto)
	store.writeSessionScope.Store(klineSessionScopeLegacy)
	if err := sqliteschema.InitializeOrValidate(context.Background(), db, dbPath, "backtest", 1, nil, nil); err != nil {
		jftradeErr1 := db.Close()
		besteffort.LogError(jftradeErr1)
		return nil, fmt.Errorf("validate sqlite backtest store: %w", err)
	}
	return store, nil
}

// Close shuts down the database connection.
func (s *FutuKLineStore) Close() error {
	return s.db.Close()
}

// SetRehabType configures the price-adjustment mode used for all subsequent
// queries.  Must be called before a backtest run.  Valid values:
// "forward" (前复权), "backward" (后复权), "none" (不复权).
func (s *FutuKLineStore) SetRehabType(rehabType string) {
	s.rehabType.Store(normalizeRehabTypeName(rehabType))
}

func (s *FutuKLineStore) SetReadSessionScope(scope string) {
	s.readSessionScope.Store(normalizeReadSessionScopeName(scope))
}

func (s *FutuKLineStore) SetWriteSessionScope(scope string) {
	s.writeSessionScope.Store(normalizeKLineSessionScopeName(scope))
}

// DB returns the managed SQLite database for advanced queries.
func (s *FutuKLineStore) DB() *sqliteconn.DB {
	return s.db
}

func (s *FutuKLineStore) CompactDatabase(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("backtest database is unavailable")
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("compact backtest database: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("compact backtest database: %w", err)
	}
	return nil
}

func (s *FutuKLineStore) rehabTypeName() string {
	return s.rehabType.Load().(string)
}

func (s *FutuKLineStore) readSessionScopeName() string {
	return s.readSessionScope.Load().(string)
}

func (s *FutuKLineStore) writeSessionScopeName() string {
	return s.writeSessionScope.Load().(string)
}
