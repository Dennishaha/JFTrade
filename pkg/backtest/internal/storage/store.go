// Package storage contains the SQLite-backed Futu backtest store
// implementation and compact local_klines schema helpers.
package storage

import (
	"context"
	"fmt"
	"sync"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	"github.com/jmoiron/sqlx"
)

// FutuKLineStore implements service.BackTestable for Futu data stored in
// SQLite. It uses a compact backtest-only local_klines table keyed by
// symbol+interval+rehab_type (table-level) and end_time (row-level).
type FutuKLineStore struct {
	mu                sync.RWMutex
	db                *sqlx.DB
	dbPath            string
	rehabType         string // "forward" | "backward" | "none" — filters all queries
	readSessionScope  string
	writeSessionScope string
	tableExistsCache  sync.Map
	accessQueue       *klineAccessQueue
}

// NewFutuKLineStore opens or creates a SQLite database at the given path and
// lazily creates per-series tables as data is inserted.
func NewFutuKLineStore(dbPath string) (*FutuKLineStore, error) {
	db, err := sqliteconn.OpenX(dbPath, sqliteconn.WithMaxOpenConns(8))
	if err != nil {
		return nil, fmt.Errorf("open sqlite backtest store: %w", err)
	}
	queue := klineAccessQueueForPath(dbPath)
	store := &FutuKLineStore{
		db:                db,
		dbPath:            dbPath,
		rehabType:         normalizeRehabTypeName("forward"),
		readSessionScope:  klineReadSessionScopeAuto,
		writeSessionScope: klineSessionScopeLegacy,
		accessQueue:       queue,
	}
	if err := queue.enqueueWrite(func() error {
		return sqliteschema.InitializeOrValidate(context.Background(), db, dbPath, "backtest", 1, nil, nil)
	}); err != nil {
		jftradeErr1 := db.Close()
		jftradeLogError(jftradeErr1)
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
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rehabType = normalizeRehabTypeName(rehabType)
}

func (s *FutuKLineStore) SetReadSessionScope(scope string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readSessionScope = normalizeReadSessionScopeName(scope)
}

func (s *FutuKLineStore) SetWriteSessionScope(scope string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeSessionScope = normalizeKLineSessionScopeName(scope)
}

// DB returns the underlying sqlx.DB for advanced queries.
func (s *FutuKLineStore) DB() *sqlx.DB {
	return s.db
}

func (s *FutuKLineStore) CompactDatabase(ctx context.Context) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("backtest database is unavailable")
	}
	if err := s.accessQueue.enqueueWrite(func() error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if _, err := s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
			return err
		}
		_, err := s.db.ExecContext(ctx, `VACUUM`)
		return err
	}); err != nil {
		return fmt.Errorf("compact backtest database: %w", err)
	}
	return nil
}
