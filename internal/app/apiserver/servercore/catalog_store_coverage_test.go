package servercore

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newCatalogCoverageStore(t *testing.T) *strategyCatalogStore {
	t.Helper()
	store, err := NewStrategyCatalogStore(filepath.Join(t.TempDir(), "catalog.json"), "plugins")
	if err != nil {
		t.Fatalf("NewStrategyCatalogStore: %v", err)
	}
	t.Cleanup(func() { jftradeCheckTestError(t, store.Close()) })
	return store
}

func TestStrategyCatalogLoadDatabaseFailures(t *testing.T) {
	for _, tableName := range []string{
		strategyCatalogMetaTable,
		strategyCatalogPluginTable,
		strategyCatalogStrategyTable,
		strategyCatalogOperationTable,
	} {
		t.Run(tableName, func(t *testing.T) {
			store := newCatalogCoverageStore(t)
			if _, err := store.db.Exec(`DROP TABLE ` + tableName); err != nil {
				t.Fatalf("drop %s: %v", tableName, err)
			}
			if err := store.load(); err == nil {
				t.Fatalf("expected load failure after dropping %s", tableName)
			}
		})
	}

	store := newCatalogCoverageStore(t)
	if got, err := store.loadCatalogMetaLocked(" \t "); err != nil || got != "" {
		t.Fatalf("blank metadata key = %q, %v", got, err)
	}
	if _, err := store.db.Exec(`DELETE FROM ` + strategyCatalogMetaTable); err != nil {
		t.Fatalf("delete target metadata: %v", err)
	}
	store.targetDir = ""
	if err := store.load(); err != nil || store.data.TargetDir != "" {
		t.Fatalf("empty target migration: %#v, %v", store.data, err)
	}
}

func TestStrategyCatalogOrderingAndNonMatchingUpdate(t *testing.T) {
	store := newCatalogCoverageStore(t)
	for _, id := range []string{"z-plugin", "a-plugin"} {
		if err := store.savePlugin(managedStrategyPlugin{Descriptor: strategyPluginDescriptor{ID: id}}); err != nil {
			t.Fatalf("save plugin %s: %v", id, err)
		}
	}
	if got := store.pluginCatalog().Plugins; len(got) != 2 || got[0].Descriptor.ID != "a-plugin" {
		t.Fatalf("sorted plugins = %#v", got)
	}
	updated := managedStrategyPlugin{Descriptor: strategyPluginDescriptor{ID: "a-plugin", DisplayName: "updated"}}
	if err := store.savePlugin(updated); err != nil {
		t.Fatalf("update later plugin: %v", err)
	}
}

func TestStrategyCatalogPersistFailureBoundaries(t *testing.T) {
	beginErr := errors.New("forced catalog begin failure")
	store := newCatalogCoverageStore(t)
	store.beginPersist = func(context.Context, *sql.TxOptions) (executionMigrationTx, error) { return nil, beginErr }
	store.data.TargetDir = ""
	if err := store.persistLocked(); !errors.Is(err, beginErr) || store.data.TargetDir != store.targetDir {
		t.Fatalf("begin failure = %v target=%q", err, store.data.TargetDir)
	}

	for _, tc := range []struct {
		name      string
		failAt    int
		commitErr error
		data      strategyCatalogFile
	}{
		{name: "delete metadata", failAt: 1},
		{name: "delete plugins", failAt: 2},
		{name: "delete strategies", failAt: 3},
		{name: "delete operations", failAt: 4},
		{name: "insert metadata", failAt: 5},
		{name: "insert plugin", failAt: 6, data: strategyCatalogFile{Plugins: []managedStrategyPlugin{{Descriptor: strategyPluginDescriptor{ID: "plugin"}}}}},
		{name: "insert strategy", failAt: 6, data: strategyCatalogFile{Strategies: []managedStrategyInstance{{ID: "strategy"}}}},
		{name: "insert operation", failAt: 6, data: strategyCatalogFile{Operations: []strategyPluginOperation{{OperationID: "operation"}}}},
		{name: "commit", commitErr: errors.New("forced commit failure")},
		{name: "success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newCatalogCoverageStore(t)
			store.data = tc.data
			store.data.TargetDir = "plugins"
			tx := &executionMigrationTxStub{failAt: tc.failAt, commitErr: tc.commitErr, rollbackErr: errors.New("ignored rollback failure")}
			store.beginPersist = func(context.Context, *sql.TxOptions) (executionMigrationTx, error) { return tx, nil }
			err := store.persistLocked()
			wantErr := tc.failAt > 0 || tc.commitErr != nil
			if (err != nil) != wantErr {
				t.Fatalf("persist error = %v, wantErr=%v", err, wantErr)
			}
			wantRollback := 0
			if wantErr {
				wantRollback = 1
			}
			if tx.rollbacks != wantRollback {
				t.Fatalf("rollbacks = %d, want %d", tx.rollbacks, wantRollback)
			}
		})
	}
}

func TestStrategyCatalogPersistMarshalFailuresRollback(t *testing.T) {
	for _, tc := range []struct {
		name string
		data strategyCatalogFile
	}{
		{name: "plugin", data: strategyCatalogFile{Plugins: []managedStrategyPlugin{{Descriptor: strategyPluginDescriptor{ID: "plugin"}}}}},
		{name: "strategy", data: strategyCatalogFile{Strategies: []managedStrategyInstance{{ID: "strategy"}}}},
		{name: "operation", data: strategyCatalogFile{Operations: []strategyPluginOperation{{OperationID: "operation"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newCatalogCoverageStore(t)
			store.data = tc.data
			store.data.TargetDir = "plugins"
			tx := &executionMigrationTxStub{}
			store.beginPersist = func(context.Context, *sql.TxOptions) (executionMigrationTx, error) { return tx, nil }
			marshalErr := errors.New("forced " + tc.name + " marshal failure")
			store.marshalJSON = func(any) ([]byte, error) { return nil, marshalErr }
			if err := store.persistLocked(); !errors.Is(err, marshalErr) || tx.rollbacks != 1 {
				t.Fatalf("marshal failure = %v, rollbacks=%d", err, tx.rollbacks)
			}
		})
	}
}

func TestStrategyCatalogConstructorReportsRuntimePathFailure(t *testing.T) {
	occupied := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(occupied, []byte("file"), 0o600); err != nil {
		t.Fatalf("write occupied path: %v", err)
	}
	if store, err := NewStrategyCatalogStore(filepath.Join(occupied, "catalog.json"), "plugins"); err == nil || store != nil || !strings.Contains(strings.ToLower(err.Error()), "directory") {
		t.Fatalf("runtime path failure = %#v, %v", store, err)
	}
}
