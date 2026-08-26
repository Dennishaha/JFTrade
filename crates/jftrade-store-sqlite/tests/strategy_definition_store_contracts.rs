use std::path::Path;

use jftrade_owner_lock::WriterLeaseError;
use jftrade_store_sqlite::{
    STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE, StoredStrategyDefinition,
    StrategyDefinitionStoreError, StrategyDefinitionTestCutoverStore,
};
use rusqlite::Connection;

const TIMESTAMP_1: &str = "2026-08-22T06:00:00Z";
const TIMESTAMP_2: &str = "2026-08-22T06:01:00Z";

#[test]
fn strategy_definition_store_rejects_missing_drifted_and_corrupted_go_databases() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let missing_path = directory.path().join("missing-strategy.db");
    assert!(matches!(
        StrategyDefinitionTestCutoverStore::open_existing(
            &missing_path,
            STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE
        ),
        Err(StrategyDefinitionStoreError::NotRegularFile(_))
    ));

    let drifted_path = directory.path().join("drifted-strategy.db");
    let connection = Connection::open(&drifted_path).expect("create drifted db");
    connection
        .execute_batch(
            "CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                VALUES ('strategy', 2, '2026-08-22T06:00:00Z');
            CREATE TABLE strategy_design_definitions (
                id TEXT PRIMARY KEY,
                rogue_column TEXT NOT NULL
            );",
        )
        .expect("seed rogue table");
    drop(connection);

    let error = StrategyDefinitionTestCutoverStore::open_existing(
        &drifted_path,
        STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE,
    )
    .expect_err("drifted schema must fail");
    assert!(matches!(error, StrategyDefinitionStoreError::Schema(_)));
}

#[test]
fn strategy_definition_lifecycle_versioning_and_restart_durability() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("strategy.db");
    seed_go_strategy_schema(&path);

    let store = open_store(&path);
    assert_eq!(store.path(), path);

    let conflict = StrategyDefinitionTestCutoverStore::open_existing(
        &path,
        STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE,
    )
    .expect_err("second writer must fail");
    assert!(matches!(
        conflict,
        StrategyDefinitionStoreError::WriterLease(WriterLeaseError::Held { .. })
    ));

    let def1 = StoredStrategyDefinition {
        id: "strat-1".to_owned(),
        name: "Momentum Alpha".to_owned(),
        version: "".to_owned(),
        description: "Initial momentum strategy".to_owned(),
        runtime: "pine".to_owned(),
        source_format: "pine".to_owned(),
        symbol: "US.AAPL".to_owned(),
        interval: "1m".to_owned(),
        script: "//@version=5\nstrategy('Momentum')".to_owned(),
        visual_model_json: "{}".to_owned(),
        created_at: "".to_owned(),
        updated_at: "".to_owned(),
        deleted_at: None,
    };

    let created = store
        .save_definition(def1.clone(), TIMESTAMP_1)
        .expect("save initial definition");
    assert_eq!(created.id, "strat-1");
    assert_eq!(created.version, "0.1.0");
    assert_eq!(created.created_at, TIMESTAMP_1);

    let versions = store
        .list_versions("strat-1")
        .expect("list versions after creation");
    assert_eq!(versions.len(), 1);
    assert_eq!(versions[0].version, "0.1.0");

    let mut updated_input = created.clone();
    updated_input.name = "Momentum Alpha V2".to_owned();
    let updated = store
        .save_definition(updated_input, TIMESTAMP_2)
        .expect("update definition");
    assert_eq!(updated.version, "0.1.1");
    assert_eq!(updated.created_at, TIMESTAMP_1);
    assert_eq!(updated.updated_at, TIMESTAMP_2);

    let versions_after_update = store
        .list_versions("strat-1")
        .expect("list versions after update");
    assert_eq!(versions_after_update.len(), 2);
    assert_eq!(versions_after_update[0].version, "0.1.1");
    assert_eq!(versions_after_update[1].version, "0.1.0");

    let deleted = store
        .delete_definition("strat-1", TIMESTAMP_2)
        .expect("delete definition");
    assert!(deleted.deleted_at.is_some());

    let get_non_deleted = store
        .get_definition("strat-1", false)
        .expect("get non-deleted");
    assert!(get_non_deleted.is_none());

    let get_with_deleted = store
        .get_definition("strat-1", true)
        .expect("get with deleted")
        .expect("must find deleted row");
    assert_eq!(get_with_deleted.id, "strat-1");
    assert!(get_with_deleted.deleted_at.is_some());

    drop(store);

    let reopened = open_store(&path);
    let reopened_deleted = reopened
        .get_definition("strat-1", true)
        .expect("get after restart")
        .expect("must exist");
    assert_eq!(reopened_deleted.version, "0.1.1");
    assert!(reopened_deleted.deleted_at.is_some());
}

fn open_store(path: &Path) -> StrategyDefinitionTestCutoverStore {
    StrategyDefinitionTestCutoverStore::open_existing(
        path,
        STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE,
    )
    .expect("open strategy definition test-cutover store")
}

fn seed_go_strategy_schema(path: &Path) {
    let connection = Connection::open(path).expect("create strategy fixture");
    connection
        .execute_batch(
            "CREATE TABLE strategy_log_events (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                instance_id TEXT NOT NULL,
                at_ms INTEGER NOT NULL,
                raw TEXT NOT NULL,
                level TEXT NOT NULL DEFAULT '',
                source TEXT NOT NULL DEFAULT ''
            );
            CREATE TABLE strategy_audit_events (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                instance_id TEXT NOT NULL,
                kind TEXT NOT NULL,
                detail TEXT NOT NULL DEFAULT '',
                at_ms INTEGER NOT NULL
            );
            CREATE TABLE strategy_runtime_observations (
                instance_id TEXT PRIMARY KEY,
                actual_status_snapshot TEXT NOT NULL DEFAULT '',
                active_symbols_json TEXT NOT NULL DEFAULT '[]',
                last_closed_kline_at_ms INTEGER,
                last_signal_at_ms INTEGER,
                last_order_at_ms INTEGER,
                last_error_at_ms INTEGER,
                last_error TEXT NOT NULL DEFAULT '',
                updated_at_ms INTEGER
            );
            CREATE TABLE strategy_catalog_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '');
            CREATE TABLE strategy_catalog_plugins (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '');
            CREATE TABLE strategy_catalog_strategies (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '');
            CREATE TABLE strategy_catalog_operations (operation_id TEXT PRIMARY KEY, plugin_id TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT '', updated_at TEXT NOT NULL DEFAULT '', payload_json TEXT NOT NULL DEFAULT '');
            CREATE TABLE strategy_design_definitions (
                id TEXT PRIMARY KEY,
                name TEXT NOT NULL DEFAULT '',
                version TEXT NOT NULL DEFAULT '',
                description TEXT NOT NULL DEFAULT '',
                runtime TEXT NOT NULL DEFAULT '',
                source_format TEXT NOT NULL DEFAULT '',
                symbol TEXT NOT NULL DEFAULT '',
                interval TEXT NOT NULL DEFAULT '',
                script TEXT NOT NULL DEFAULT '',
                visual_model_json TEXT NOT NULL DEFAULT '',
                created_at TEXT NOT NULL DEFAULT '',
                updated_at TEXT NOT NULL DEFAULT '',
                deleted_at TEXT
            );
            CREATE TABLE strategy_definition_versions (
                definition_id TEXT NOT NULL,
                version TEXT NOT NULL,
                name TEXT NOT NULL DEFAULT '',
                description TEXT NOT NULL DEFAULT '',
                runtime TEXT NOT NULL DEFAULT '',
                source_format TEXT NOT NULL DEFAULT '',
                symbol TEXT NOT NULL DEFAULT '',
                interval TEXT NOT NULL DEFAULT '',
                script TEXT NOT NULL DEFAULT '',
                visual_model_json TEXT NOT NULL DEFAULT '',
                created_at TEXT NOT NULL DEFAULT '',
                updated_at TEXT NOT NULL DEFAULT '',
                saved_at TEXT NOT NULL DEFAULT '',
                PRIMARY KEY (definition_id, version),
                FOREIGN KEY (definition_id) REFERENCES strategy_design_definitions(id) ON DELETE CASCADE
            );
            CREATE INDEX idx_strategy_log_events_instance_at ON strategy_log_events (instance_id, at_ms DESC, id DESC);
            CREATE INDEX idx_strategy_log_events_level ON strategy_log_events (level);
            CREATE INDEX idx_strategy_audit_events_instance_at ON strategy_audit_events (instance_id, at_ms DESC, id DESC);
            CREATE INDEX idx_strategy_audit_events_kind ON strategy_audit_events (kind);
            CREATE INDEX idx_strategy_catalog_strategies_created_at ON strategy_catalog_strategies (created_at ASC, id ASC);
            CREATE INDEX idx_strategy_catalog_operations_updated_at ON strategy_catalog_operations (updated_at DESC, operation_id ASC);
            CREATE INDEX idx_strategy_design_definitions_updated_at ON strategy_design_definitions (updated_at DESC, id ASC);
            CREATE INDEX idx_strategy_design_definitions_deleted_at ON strategy_design_definitions (deleted_at);
            CREATE INDEX idx_strategy_definition_versions_saved_at ON strategy_definition_versions (definition_id, saved_at DESC, version DESC);
            CREATE TRIGGER trg_strategy_definition_versions_immutable
                BEFORE UPDATE ON strategy_definition_versions
                BEGIN
                    SELECT RAISE(ABORT, 'strategy definition versions are immutable');
                END;
            CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                VALUES ('strategy', 2, '2026-08-22T06:00:00Z');",
        )
        .expect("seed Go-compatible strategy schema");
}
