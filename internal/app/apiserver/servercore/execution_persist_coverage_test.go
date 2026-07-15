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

type executionSchemaRowsStub struct {
	next       bool
	scanErr    error
	iterateErr error
	closeErr   error
}

func (s *executionSchemaRowsStub) Next() bool {
	if !s.next {
		return false
	}
	s.next = false
	return true
}

func (s *executionSchemaRowsStub) Scan(dest ...any) error {
	if s.scanErr != nil {
		return s.scanErr
	}
	*(dest[0].(*int)) = 0
	*(dest[1].(*string)) = "actual"
	*(dest[2].(*string)) = "text"
	*(dest[3].(*int)) = 0
	*(dest[4].(*sql.NullString)) = sql.NullString{}
	*(dest[5].(*int)) = 0
	return nil
}

func (s *executionSchemaRowsStub) Err() error   { return s.iterateErr }
func (s *executionSchemaRowsStub) Close() error { return s.closeErr }

func seedExecutionMigrationMetadata(t *testing.T, version int) *executionOrderSQLiteStore {
	t.Helper()
	db, err := sqliteconn.OpenX(filepath.Join(t.TempDir(), "migration.db"))
	if err != nil {
		t.Fatalf("OpenX: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE ` + sqliteschema.MetadataTable + ` (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("create metadata: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO `+sqliteschema.MetadataTable+` VALUES ('execution-orders', ?, 'now')`, version); err != nil {
		t.Fatalf("insert metadata: %v", err)
	}
	return &executionOrderSQLiteStore{db: db}
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
	if store, err := newExecutionOrderSQLiteStore(migrationPath); err == nil || store != nil || !strings.Contains(err.Error(), "migrate execution order") {
		t.Fatalf("migration constructor = %#v, %v", store, err)
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

func TestExecutionPersistenceMigrationBoundaries(t *testing.T) {
	t.Run("metadata query", func(t *testing.T) {
		store := seedExecutionMigrationMetadata(t, 2)
		jftradeCheckTestError(t, store.db.Close())
		if err := store.migrateSchemaV1ToV2(); err == nil {
			t.Fatal("expected closed metadata query error")
		}
	})

	t.Run("metadata absent", func(t *testing.T) {
		db, err := sqliteconn.OpenX(filepath.Join(t.TempDir(), "absent.db"))
		if err != nil {
			t.Fatalf("OpenX: %v", err)
		}
		defer func() { jftradeCheckTestError(t, db.Close()) }()
		if err := (&executionOrderSQLiteStore{db: db}).migrateSchemaV1ToV2(); err != nil {
			t.Fatalf("absent metadata: %v", err)
		}
	})

	t.Run("version query", func(t *testing.T) {
		db, err := sqliteconn.OpenX(filepath.Join(t.TempDir(), "bad-version.db"))
		if err != nil {
			t.Fatalf("OpenX: %v", err)
		}
		defer func() { jftradeCheckTestError(t, db.Close()) }()
		if _, err := db.Exec(`CREATE TABLE ` + sqliteschema.MetadataTable + ` (component_id TEXT PRIMARY KEY)`); err != nil {
			t.Fatalf("create malformed metadata: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO ` + sqliteschema.MetadataTable + ` VALUES ('execution-orders')`); err != nil {
			t.Fatalf("insert malformed metadata: %v", err)
		}
		if err := (&executionOrderSQLiteStore{db: db}).migrateSchemaV1ToV2(); err == nil {
			t.Fatal("expected version query error")
		}
	})

	t.Run("version absent", func(t *testing.T) {
		store := seedExecutionMigrationMetadata(t, 2)
		defer func() { jftradeCheckTestError(t, store.Close()) }()
		if _, err := store.db.Exec(`DELETE FROM ` + sqliteschema.MetadataTable); err != nil {
			t.Fatalf("delete metadata row: %v", err)
		}
		if err := store.migrateSchemaV1ToV2(); err != nil {
			t.Fatalf("missing version row: %v", err)
		}
	})

	t.Run("different version", func(t *testing.T) {
		store := seedExecutionMigrationMetadata(t, 2)
		defer func() { jftradeCheckTestError(t, store.Close()) }()
		if err := store.migrateSchemaV1ToV2(); err != nil {
			t.Fatalf("different version: %v", err)
		}
	})

	beginErr := errors.New("forced begin failure")
	store := seedExecutionMigrationMetadata(t, 1)
	store.beginMigration = func(context.Context, *sql.TxOptions) (executionMigrationTx, error) {
		return nil, beginErr
	}
	if err := store.migrateSchemaV1ToV2(); !errors.Is(err, beginErr) {
		t.Fatalf("begin error = %v", err)
	}
	jftradeCheckTestError(t, store.Close())

	for _, tc := range []struct {
		name      string
		failAt    int
		commitErr error
		wantErr   bool
		rollback  int
	}{
		{name: "alter", failAt: 1, wantErr: true, rollback: 1},
		{name: "order update", failAt: 2, wantErr: true, rollback: 1},
		{name: "metadata update", failAt: 3, wantErr: true, rollback: 1},
		{name: "commit", commitErr: errors.New("forced commit failure"), wantErr: true, rollback: 1},
		{name: "success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := seedExecutionMigrationMetadata(t, 1)
			defer func() { jftradeCheckTestError(t, store.Close()) }()
			tx := &executionMigrationTxStub{failAt: tc.failAt, commitErr: tc.commitErr, rollbackErr: errors.New("ignored rollback failure")}
			store.beginMigration = func(context.Context, *sql.TxOptions) (executionMigrationTx, error) { return tx, nil }
			err := store.migrateSchemaV1ToV2()
			if (err != nil) != tc.wantErr || tx.rollbacks != tc.rollback {
				t.Fatalf("migration err=%v rollbacks=%d", err, tx.rollbacks)
			}
		})
	}
}

func TestExecutionPersistenceSchemaInspectionBoundaries(t *testing.T) {
	db, err := sqliteconn.OpenX(filepath.Join(t.TempDir(), "empty.db"))
	if err != nil {
		t.Fatalf("OpenX: %v", err)
	}
	store := &executionOrderSQLiteStore{db: db}
	if err := store.ensureExistingSchemaCanBeOpened(); err != nil {
		t.Fatalf("empty schema: %v", err)
	}
	jftradeCheckTestError(t, db.Close())
	if err := store.ensureExistingSchemaCanBeOpened(); err == nil {
		t.Fatal("expected closed schema select error")
	}
	if err := store.ensureSchema("anything", nil); err == nil || !strings.Contains(err.Error(), "inspect") {
		t.Fatalf("closed schema query error = %v", err)
	}
	if err := store.validateExecutionSchemas(context.Background()); err == nil {
		t.Fatal("expected validation open error")
	}

	wrong, err := sqliteconn.OpenX(filepath.Join(t.TempDir(), "wrong.db"))
	if err != nil {
		t.Fatalf("OpenX wrong: %v", err)
	}
	defer func() { jftradeCheckTestError(t, wrong.Close()) }()
	for _, statement := range []string{
		`CREATE TABLE execution_orders (internal_order_id TEXT PRIMARY KEY)`,
		`CREATE TABLE execution_order_events (id TEXT PRIMARY KEY)`,
		`CREATE TABLE execution_seen_fills (fill_key TEXT PRIMARY KEY)`,
		`CREATE TABLE execution_sequences (name TEXT PRIMARY KEY)`,
	} {
		if _, err := wrong.Exec(statement); err != nil {
			t.Fatalf("create wrong schema: %v", err)
		}
	}
	if err := (&executionOrderSQLiteStore{db: wrong}).validateExecutionSchemas(context.Background()); err == nil {
		t.Fatal("expected column validation error")
	}

	if err := inspectExecutionSchemaRows("scan", []string{"actual:TEXT:0"}, &executionSchemaRowsStub{next: true, scanErr: errors.New("scan")}); err == nil || !strings.Contains(err.Error(), "scan") {
		t.Fatalf("scan error = %v", err)
	}
	if err := inspectExecutionSchemaRows("iterate", nil, &executionSchemaRowsStub{iterateErr: errors.New("iterate")}); err == nil || !strings.Contains(err.Error(), "iterate") {
		t.Fatalf("iterate error = %v", err)
	}
	if err := inspectExecutionSchemaRows("mismatch", []string{"wanted:TEXT:0"}, &executionSchemaRowsStub{next: true, closeErr: errors.New("ignored close")}); err == nil || !strings.Contains(err.Error(), "obsolete") {
		t.Fatalf("same-length mismatch = %v", err)
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
