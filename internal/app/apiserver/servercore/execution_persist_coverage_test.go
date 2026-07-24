package servercore

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
)

type executionMigrationTxStub struct {
	failAt      int
	execCalls   int
	commitErr   error
	rollbackErr error
	rollbacks   int
}

func (s *executionMigrationTxStub) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	s.execCalls++
	if s.execCalls == s.failAt {
		return nil, errors.New("forced migration exec failure")
	}
	return nil, nil
}

func (s *executionMigrationTxStub) Commit() error { return s.commitErr }

func (s *executionMigrationTxStub) Rollback() error {
	s.rollbacks++
	return s.rollbackErr
}

func TestExecutionPersistenceConstructorDependencyFailures(t *testing.T) {
	statErr := errors.New("forced stat failure")
	if _, err := newExecutionOrderSQLiteStoreWithDeps("execution.db", func(string) (os.FileInfo, error) {
		return nil, statErr
	}, sqliteconn.OpenX); !errors.Is(err, statErr) || !strings.Contains(err.Error(), "inspect") {
		t.Fatalf("stat error = %v", err)
	}

	probe := filepath.Join(t.TempDir(), "existing.db")
	if err := os.WriteFile(probe, []byte("not-empty"), 0o600); err != nil {
		t.Fatalf("write probe: %v", err)
	}
	openErr := errors.New("forced open failure")
	if _, err := newExecutionOrderSQLiteStoreWithDeps(probe, os.Stat, func(string, ...sqliteconn.Option) (*sqliteconn.DB, error) {
		return nil, openErr
	}); !errors.Is(err, openErr) || !strings.Contains(err.Error(), "open") {
		t.Fatalf("open error = %v", err)
	}

	migrationPath := filepath.Join(t.TempDir(), "migration-error.db")
	migrationDB, err := sqliteconn.OpenX(migrationPath)
	if err != nil {
		t.Fatalf("open migration seed: %v", err)
	}
	if _, err := migrationDB.Exec(`CREATE TABLE ` + sqliteschema.MetadataTable + ` (component_id TEXT PRIMARY KEY); INSERT INTO ` + sqliteschema.MetadataTable + ` VALUES ('execution-orders')`); err != nil {
		t.Fatalf("seed malformed migration metadata: %v", err)
	}
	jftradeCheckTestError(t, migrationDB.Close())
	if store, err := newExecutionOrderSQLiteStore(migrationPath); err == nil || store != nil || !sqliteschema.IsIncompatible(err) {
		t.Fatalf("malformed metadata constructor = %#v, %v", store, err)
	}

	persistence, err := newExecutionOrderSQLiteStore(filepath.Join(t.TempDir(), "load.db"))
	if err != nil {
		t.Fatalf("new persistence: %v", err)
	}
	if _, err := persistence.db.Exec(`DROP TABLE ` + executionOrderTable); err != nil {
		t.Fatalf("drop orders: %v", err)
	}
	if store, err := newExecutionOrderStoreWithPersistence(persistence); err == nil || store != nil {
		t.Fatalf("load constructor = %#v, %v", store, err)
	}
}

func TestExecutionPersistenceLoadsStoredSequenceHighWaterMarks(t *testing.T) {
	persistence, err := newExecutionOrderSQLiteStore(filepath.Join(t.TempDir(), "sequences.db"))
	if err != nil {
		t.Fatalf("new persistence: %v", err)
	}
	defer func() { jftradeCheckTestError(t, persistence.Close()) }()
	if err := persistence.persistSequence("orders", 41); err != nil {
		t.Fatalf("persist orders sequence: %v", err)
	}
	if err := persistence.persistSequence("events", 42); err != nil {
		t.Fatalf("persist events sequence: %v", err)
	}
	store := newExecutionOrderStore()
	store.persistence = persistence
	if err := store.loadFromDB(); err != nil {
		t.Fatalf("loadFromDB: %v", err)
	}
	if store.nextOrderSeq != 41 || store.nextEventSeq != 42 {
		t.Fatalf("sequence high water = %d/%d", store.nextOrderSeq, store.nextEventSeq)
	}
}
