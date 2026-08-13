// Package storage contains the provider-qualified SQLite backtest store and
// compact local_klines schema helpers.
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

// KLineStore implements service.BackTestable for provider-qualified data stored in
// SQLite. It uses a compact backtest-only local_klines table keyed by
// provider+symbol+interval+adjustment+session (table-level) and end_time
// (row-level).
type KLineStore struct {
	db                *sqliteconn.DB
	dbPath            string
	rehabType         atomic.Value // string: "forward" | "backward" | "none"
	readSessionScope  atomic.Value // string
	writeSessionScope atomic.Value // string
	providerID        atomic.Value // string
	tableExistsCache  sync.Map
}

// FutuKLineStore preserves the legacy internal name while all implementation
// and physical keys remain provider-neutral.
type FutuKLineStore = KLineStore

// NewKLineStore opens or creates a SQLite database at the given path and
// lazily creates per-series tables as data is inserted.
func NewKLineStore(dbPath, providerID string) (*KLineStore, error) {
	if err := sqliteschema.ValidateCurrentFile(context.Background(), dbPath, sqliteschema.DatabaseBacktest); err != nil {
		return nil, fmt.Errorf("validate sqlite backtest store: %w", err)
	}
	db, err := sqliteconn.OpenX(dbPath, sqliteconn.WithMaxOpenConns(8))
	if err != nil {
		return nil, fmt.Errorf("open sqlite backtest store: %w", err)
	}
	store := &KLineStore{
		db:     db,
		dbPath: dbPath,
	}
	store.rehabType.Store(normalizeRehabTypeName("forward"))
	store.providerID.Store(normalizeProviderID(providerID))
	store.readSessionScope.Store(klineSessionScopeRegular)
	store.writeSessionScope.Store(klineSessionScopeRegular)
	if err := sqliteschema.InitializeCurrent(context.Background(), db, dbPath, sqliteschema.DatabaseBacktest); err != nil {
		jftradeErr1 := db.Close()
		besteffort.LogError(jftradeErr1)
		return nil, fmt.Errorf("validate sqlite backtest store: %w", err)
	}
	return store, nil
}

// NewFutuKLineStore is the compatibility constructor for published callers.
func NewFutuKLineStore(dbPath string) (*KLineStore, error) {
	return NewKLineStore(dbPath, "futu")
}

// Close shuts down the database connection.
func (s *KLineStore) Close() error {
	return s.db.Close()
}

// SetRehabType configures the price-adjustment mode used for all subsequent
// queries.  Must be called before a backtest run.  Valid values:
// "forward" (前复权), "backward" (后复权), "none" (不复权).
func (s *KLineStore) SetRehabType(rehabType string) {
	s.rehabType.Store(normalizeRehabTypeName(rehabType))
}

func (s *KLineStore) SetReadSessionScope(scope string) {
	s.readSessionScope.Store(normalizeReadSessionScopeName(scope))
}

func (s *KLineStore) SetWriteSessionScope(scope string) {
	s.writeSessionScope.Store(normalizeKLineSessionScopeName(scope))
}

func (s *KLineStore) SetProviderID(providerID string) {
	s.providerID.Store(normalizeProviderID(providerID))
}

// DB returns the managed SQLite database for advanced queries.
func (s *KLineStore) DB() *sqliteconn.DB {
	return s.db
}

func (s *KLineStore) CompactDatabase(ctx context.Context) error {
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

func (s *KLineStore) rehabTypeName() string {
	return s.rehabType.Load().(string)
}

func (s *KLineStore) readSessionScopeName() string {
	return s.readSessionScope.Load().(string)
}

func (s *KLineStore) writeSessionScopeName() string {
	return s.writeSessionScope.Load().(string)
}

func (s *KLineStore) providerIDName() string {
	return s.providerID.Load().(string)
}
