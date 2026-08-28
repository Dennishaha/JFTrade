use std::path::Path;

use jftrade_owner_lock::WriterLeaseError;
use jftrade_store_sqlite::{
    STRATEGY_RUNTIME_TEST_CUTOVER_PROFILE, StrategyRuntimeStoreError,
    StrategyRuntimeTestCutoverStore,
};
use rusqlite::Connection;
use serde_json::json;

const TIMESTAMP_1: &str = "2026-08-22T06:00:00Z";
const TIMESTAMP_2: &str = "2026-08-22T06:01:00Z";

#[test]
fn strategy_runtime_store_rejects_missing_drifted_and_corrupted_go_databases() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let missing_path = directory.path().join("missing-runtime.db");
    assert!(matches!(
        StrategyRuntimeTestCutoverStore::open_existing(
            &missing_path,
            STRATEGY_RUNTIME_TEST_CUTOVER_PROFILE
        ),
        Err(StrategyRuntimeStoreError::NotRegularFile(_))
    ));

    let drifted_path = directory.path().join("drifted-runtime.db");
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
            CREATE TABLE strategy_catalog_operations (
                operation_id TEXT PRIMARY KEY,
                rogue_column TEXT NOT NULL
            );",
        )
        .expect("seed rogue table");
    drop(connection);

    let error = StrategyRuntimeTestCutoverStore::open_existing(
        &drifted_path,
        STRATEGY_RUNTIME_TEST_CUTOVER_PROFILE,
    )
    .expect_err("drifted schema must fail");
    assert!(matches!(error, StrategyRuntimeStoreError::Schema(_)));
}

#[test]
fn strategy_runtime_instance_lifecycle_and_restart_durability() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("strategy.db");
    seed_go_strategy_schema(&path);

    let store = open_store(&path);
    assert_eq!(store.path(), path);

    let conflict = StrategyRuntimeTestCutoverStore::open_existing(
        &path,
        STRATEGY_RUNTIME_TEST_CUTOVER_PROFILE,
    )
    .expect_err("second writer must fail");
    assert!(matches!(
        conflict,
        StrategyRuntimeStoreError::WriterLease(WriterLeaseError::Held { .. })
    ));

    store
        .seed_instance("inst-1", "STOPPED", TIMESTAMP_1)
        .expect("seed stopped instance");

    let initial = store
        .get_instance("inst-1")
        .expect("get instance")
        .expect("must exist");
    assert_eq!(initial.id, "inst-1");
    assert_eq!(initial.status, "STOPPED");
    assert!(!initial.runtime_active);

    let started = store
        .update_status("inst-1", "RUNNING", TIMESTAMP_2)
        .expect("start instance");
    assert_eq!(started.status, "RUNNING");
    assert!(started.runtime_active);

    let updated_binding = store
        .update_binding(
            "inst-1",
            json!({"symbols":["US.AAPL","US.TSLA"]}),
            TIMESTAMP_2,
        )
        .expect("update binding");
    assert_eq!(
        updated_binding.binding["symbols"],
        json!(["US.AAPL", "US.TSLA"])
    );

    let deleted = store
        .delete_instance("inst-1", TIMESTAMP_2)
        .expect("delete instance");
    assert!(deleted.deleted);

    let after_delete = store
        .update_status("inst-1", "RUNNING", TIMESTAMP_2)
        .expect_err("deleted instance cannot update status");
    assert!(matches!(after_delete, StrategyRuntimeStoreError::NotFound));

    drop(store);

    let reopened = open_store(&path);
    let reopened_inst = reopened
        .get_instance("inst-1")
        .expect("get after restart")
        .expect("must exist");
    assert!(reopened_inst.deleted);
}

#[test]
fn strategy_runtime_rejects_corrupt_payload_and_reads_persisted_activity() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("strategy.db");
    seed_go_strategy_schema(&path);
    let store = open_store(&path);
    store
        .seed_instance("inst-activity", "STOPPED", TIMESTAMP_1)
        .expect("seed instance");

    let connection = Connection::open(&path).expect("open activity writer");
    connection
        .execute(
            "INSERT INTO strategy_log_events (instance_id, at_ms, raw, level, source) \
             VALUES ('inst-activity', 1000, 'started', 'info', 'runtime')",
            [],
        )
        .expect("insert log");
    connection
        .execute(
            "INSERT INTO strategy_audit_events (instance_id, kind, detail, at_ms) \
             VALUES ('inst-activity', 'execution', 'submitted', 2000)",
            [],
        )
        .expect("insert audit");
    assert_eq!(
        store.list_log_events("inst-activity").expect("read log")[0].raw,
        "started"
    );
    assert_eq!(
        store
            .list_audit_events("inst-activity")
            .expect("read audit")[0]
            .detail,
        "submitted"
    );

    connection
        .execute(
            "UPDATE strategy_catalog_operations SET payload_json = 'not-json' \
             WHERE operation_id = 'inst-activity'",
            [],
        )
        .expect("corrupt payload");
    assert!(matches!(
        store.list_instances(),
        Err(StrategyRuntimeStoreError::Incompatible(message))
            if message.contains("invalid payload JSON")
    ));
}

fn open_store(path: &Path) -> StrategyRuntimeTestCutoverStore {
    StrategyRuntimeTestCutoverStore::open_existing(path, STRATEGY_RUNTIME_TEST_CUTOVER_PROFILE)
        .expect("open strategy runtime test-cutover store")
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
