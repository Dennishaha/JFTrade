package servercore

import (
	"context"
	"strings"

	"github.com/jftrade/jftrade-main/internal/store/sqliteschema"
	"github.com/jmoiron/sqlx"
)

func strategyDatabaseStatements() []string {
	return []string{
		strings.Join([]string{`CREATE TABLE ` + strategyRuntimeLogTable + ` (`,
			`id INTEGER PRIMARY KEY AUTOINCREMENT, instance_id TEXT NOT NULL, at_ms INTEGER NOT NULL, raw TEXT NOT NULL,`,
			`level TEXT NOT NULL DEFAULT '', source TEXT NOT NULL DEFAULT '')`}, " "),
		strings.Join([]string{`CREATE TABLE ` + strategyRuntimeAuditTable + ` (`,
			`id INTEGER PRIMARY KEY AUTOINCREMENT, instance_id TEXT NOT NULL, kind TEXT NOT NULL,`,
			`detail TEXT NOT NULL DEFAULT '', at_ms INTEGER NOT NULL)`}, " "),
		strings.Join([]string{`CREATE TABLE ` + strategyRuntimeObservationTable + ` (`,
			`instance_id TEXT PRIMARY KEY, actual_status_snapshot TEXT NOT NULL DEFAULT '',`,
			`active_symbols_json TEXT NOT NULL DEFAULT '[]', last_closed_kline_at_ms INTEGER,`,
			`last_signal_at_ms INTEGER, last_order_at_ms INTEGER, last_error_at_ms INTEGER,`,
			`last_error TEXT NOT NULL DEFAULT '', updated_at_ms INTEGER)`}, " "),
		`CREATE TABLE ` + strategyCatalogMetaTable + ` (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE ` + strategyCatalogPluginTable + ` (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE ` + strategyCatalogStrategyTable + ` (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE ` + strategyCatalogOperationTable + ` (operation_id TEXT PRIMARY KEY, plugin_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '', payload_json TEXT NOT NULL DEFAULT '')`,
		strings.Join([]string{`CREATE TABLE ` + strategyDesignDefinitionTable + ` (`,
			`id TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', version TEXT NOT NULL DEFAULT '',`,
			`description TEXT NOT NULL DEFAULT '', runtime TEXT NOT NULL DEFAULT '', source_format TEXT NOT NULL DEFAULT '',`,
			`symbol TEXT NOT NULL DEFAULT '', interval TEXT NOT NULL DEFAULT '', script TEXT NOT NULL DEFAULT '',`,
			`visual_model_json TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '',`,
			`updated_at TEXT NOT NULL DEFAULT '', deleted_at TEXT)`}, " "),
		`CREATE INDEX idx_strategy_log_events_instance_at ON ` + strategyRuntimeLogTable + ` (instance_id, at_ms DESC, id DESC)`,
		`CREATE INDEX idx_strategy_log_events_level ON ` + strategyRuntimeLogTable + ` (level)`,
		`CREATE INDEX idx_strategy_audit_events_instance_at ON ` + strategyRuntimeAuditTable + ` (instance_id, at_ms DESC, id DESC)`,
		`CREATE INDEX idx_strategy_audit_events_kind ON ` + strategyRuntimeAuditTable + ` (kind)`,
		`CREATE INDEX idx_strategy_catalog_strategies_created_at ON ` + strategyCatalogStrategyTable + ` (created_at ASC, id ASC)`,
		`CREATE INDEX idx_strategy_catalog_operations_updated_at ON ` + strategyCatalogOperationTable + ` (updated_at DESC, operation_id ASC)`,
		`CREATE INDEX idx_strategy_design_definitions_updated_at ON ` + strategyDesignDefinitionTable + ` (updated_at DESC, id ASC)`,
		`CREATE INDEX idx_strategy_design_definitions_deleted_at ON ` + strategyDesignDefinitionTable + ` (deleted_at)`,
	}
}

func validateStrategyDatabase(ctx context.Context, db *sqlx.DB) error {
	for _, schema := range []struct {
		table   string
		columns []string
	}{
		{strategyRuntimeLogTable, expectedStrategyRuntimeLogSchemaColumns()},
		{strategyRuntimeAuditTable, expectedStrategyRuntimeAuditSchemaColumns()},
		{strategyRuntimeObservationTable, expectedStrategyRuntimeObservationSchemaColumns()},
		{strategyCatalogMetaTable, []string{"key:TEXT:1", "value:TEXT:0"}},
		{strategyCatalogPluginTable, []string{"id:TEXT:1", "payload_json:TEXT:0", "updated_at:TEXT:0"}},
		{strategyCatalogStrategyTable, []string{"id:TEXT:1", "payload_json:TEXT:0", "created_at:TEXT:0", "updated_at:TEXT:0"}},
		{strategyCatalogOperationTable, []string{"operation_id:TEXT:1", "plugin_id:TEXT:0", "status:TEXT:0", "updated_at:TEXT:0", "payload_json:TEXT:0"}},
		{strategyDesignDefinitionTable, []string{
			"id:TEXT:1", "name:TEXT:0", "version:TEXT:0", "description:TEXT:0", "runtime:TEXT:0",
			"source_format:TEXT:0", "symbol:TEXT:0", "interval:TEXT:0", "script:TEXT:0",
			"visual_model_json:TEXT:0", "created_at:TEXT:0", "updated_at:TEXT:0", "deleted_at:TEXT:0",
		}},
	} {
		if err := sqliteschema.ValidateTable(ctx, db, schema.table, schema.columns); err != nil {
			return err
		}
	}
	return nil
}

func initializeStrategyDatabase(db *sqlx.DB, path string) error {
	return sqliteschema.InitializeOrValidate(
		context.Background(),
		db,
		path,
		"strategy",
		1,
		strategyDatabaseStatements(),
		validateStrategyDatabase,
	)
}
