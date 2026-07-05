package servercore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jftrade/jftrade-main/internal/store/sqliteconn"
	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
)

func TestExecutionOrderDatabasePathResolution(t *testing.T) {
	configured := filepath.Join(t.TempDir(), "configured-execution.db")
	t.Setenv("JFTRADE_EXECUTION_ORDER_DB", "  "+configured+"  ")
	if got := deriveExecutionOrderDBPath("ignored-settings.json"); got != configured {
		t.Fatalf("configured execution db path = %q, want %q", got, configured)
	}

	t.Setenv("JFTRADE_EXECUTION_ORDER_DB", "")
	if got := deriveExecutionOrderDBPath("settings.json"); got != defaultExecutionOrderDBFilename {
		t.Fatalf("bare settings execution db path = %q", got)
	}
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	if got := deriveExecutionOrderDBPath(settingsPath); got != filepath.Join(filepath.Dir(settingsPath), defaultExecutionOrderDBFilename) {
		t.Fatalf("derived execution db path = %q", got)
	}
}

func TestExecutionOrderPersistenceMigratesV1StatusWithoutDataLoss(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "execution-v1.db")
	persistence, err := newExecutionOrderSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("initialize v2 store: %v", err)
	}
	if err := persistence.Close(); err != nil {
		t.Fatalf("close v2 seed: %v", err)
	}

	db, err := sqliteconn.OpenX(dbPath)
	if err != nil {
		t.Fatalf("open v2 seed: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE execution_orders DROP COLUMN raw_broker_status`); err != nil {
		t.Fatalf("downgrade execution schema: %v", err)
	}
	if _, err := db.Exec(`UPDATE ` + sqliteschema.MetadataTable + ` SET version = 1 WHERE component_id = 'execution-orders'`); err != nil {
		t.Fatalf("downgrade schema metadata: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO execution_orders (internal_order_id, status, updated_at, created_at) VALUES ('exec-v1', 'FILLED_PART', '2026-07-03T00:00:00Z', '2026-07-03T00:00:00Z')`); err != nil {
		t.Fatalf("insert v1 order: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close v1 seed: %v", err)
	}

	store, err := newExecutionOrderStoreWithDB(dbPath)
	if err != nil {
		t.Fatalf("migrate v1 store: %v", err)
	}
	defer func() { jftradeCheckTestError(t, store.Close()) }()
	order, ok := store.order("exec-v1")
	if !ok || order.Status != "PARTIALLY_FILLED" {
		t.Fatalf("migrated order = %#v", order)
	}
	if order.RawBrokerStatus == nil || *order.RawBrokerStatus != "FILLED_PART" {
		t.Fatalf("migrated raw broker status = %#v", order.RawBrokerStatus)
	}
}

func TestExecutionOrderPersistenceRejectsInvalidPaths(t *testing.T) {
	if _, err := newExecutionOrderStoreWithDB(" \t "); err == nil || !strings.Contains(err.Error(), "db path is required") {
		t.Fatalf("empty execution db path err = %v", err)
	}

	occupied := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(occupied, []byte("file"), 0o600); err != nil {
		t.Fatalf("prepare occupied path: %v", err)
	}
	if _, err := newExecutionOrderSQLiteStore(filepath.Join(occupied, "execution.db")); err == nil || !strings.Contains(err.Error(), "create execution order db directory") {
		t.Fatalf("occupied parent path err = %v", err)
	}
}

func TestExecutionOrderPersistenceRejectsPartialLegacySchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "partial-legacy.db")
	db, err := sqliteconn.OpenX(dbPath)
	if err != nil {
		t.Fatalf("OpenX: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE execution_orders (internal_order_id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("create partial legacy table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	if _, err := newExecutionOrderSQLiteStore(dbPath); err == nil || !strings.Contains(err.Error(), "schema is incompatible") || !strings.Contains(err.Error(), "rebuild") {
		t.Fatalf("partial legacy schema err = %v", err)
	}
	db, err = sqliteconn.OpenX(dbPath)
	if err != nil {
		t.Fatalf("reopen partial schema: %v", err)
	}
	persistence := &executionOrderSQLiteStore{db: db, path: dbPath}
	if err := persistence.ensureExistingSchemaCanBeOpened(); err == nil || !strings.Contains(err.Error(), "schema is obsolete") {
		t.Fatalf("partial table validation err = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close partial schema validation db: %v", err)
	}
}

func TestExecutionOrderPersistenceRejectsWrongColumnLayout(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "wrong-columns.db")
	db, err := sqliteconn.OpenX(dbPath)
	if err != nil {
		t.Fatalf("OpenX: %v", err)
	}
	statements := []string{
		`CREATE TABLE execution_orders (internal_order_id TEXT PRIMARY KEY)`,
		`CREATE TABLE execution_order_events (id TEXT PRIMARY KEY)`,
		`CREATE TABLE execution_seen_fills (fill_key TEXT PRIMARY KEY)`,
		`CREATE TABLE execution_sequences (name TEXT PRIMARY KEY)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed wrong schema %q: %v", statement, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	if _, err := newExecutionOrderSQLiteStore(dbPath); err == nil || !strings.Contains(err.Error(), "schema is incompatible") {
		t.Fatalf("wrong column schema err = %v", err)
	}
	db, err = sqliteconn.OpenX(dbPath)
	if err != nil {
		t.Fatalf("reopen wrong column schema: %v", err)
	}
	persistence := &executionOrderSQLiteStore{db: db, path: dbPath}
	if err := persistence.ensureExistingSchemaCanBeOpened(); err != nil {
		t.Fatalf("all expected table names should be present: %v", err)
	}
	if err := persistence.ensureSchema(executionOrderTable, expectedExecutionOrderColumns()); err == nil || !strings.Contains(err.Error(), "schema is obsolete") {
		t.Fatalf("wrong column validation err = %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close wrong column validation db: %v", err)
	}
}

func TestExecutionPersistenceNilLifecycleAndSequenceSuffix(t *testing.T) {
	var persistence *executionOrderSQLiteStore
	if err := persistence.Close(); err != nil {
		t.Fatalf("nil persistence close: %v", err)
	}
	var store *executionOrderStore
	if err := store.loadFromDB(); err != nil {
		t.Fatalf("nil store load: %v", err)
	}
	for _, tc := range []struct {
		value  string
		prefix string
		want   uint64
	}{
		{value: "exec-000042", prefix: "exec-", want: 42},
		{value: "evt-7", prefix: "evt-", want: 7},
		{value: "missing-suffix", prefix: "exec-", want: 0},
		{value: "exec-not-a-number", prefix: "exec-", want: 0},
	} {
		if got := executionSequenceSuffix(tc.value, tc.prefix); got != tc.want {
			t.Fatalf("executionSequenceSuffix(%q) = %d, want %d", tc.value, got, tc.want)
		}
	}
}

func TestExecutionOrderPersistenceLoadRejectsMissingRuntimeTables(t *testing.T) {
	for _, tableName := range []string{
		executionOrderTable,
		executionOrderEventTable,
		executionSeenFillTable,
		executionSequenceTable,
	} {
		t.Run(tableName, func(t *testing.T) {
			persistence, err := newExecutionOrderSQLiteStore(filepath.Join(t.TempDir(), "execution.db"))
			if err != nil {
				t.Fatalf("newExecutionOrderSQLiteStore: %v", err)
			}
			defer func() { _ = persistence.Close() }()
			if _, err := persistence.db.Exec(`DROP TABLE ` + tableName); err != nil {
				t.Fatalf("drop %s: %v", tableName, err)
			}
			store := newExecutionOrderStore()
			store.persistence = persistence
			if err := store.loadFromDB(); err == nil || !strings.Contains(strings.ToLower(err.Error()), "no such table") {
				t.Fatalf("loadFromDB after dropping %s err = %v", tableName, err)
			}
		})
	}
}
