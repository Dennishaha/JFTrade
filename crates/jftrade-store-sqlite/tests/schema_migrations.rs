use std::fs;

use jftrade_store_sqlite::{
    BacktestMarketDataStore, StoredBacktestCandle, current_version, initialize_current,
    migrate_legacy_schema, validate_current,
};
use rusqlite::{Connection, OpenFlags};
use tempfile::tempdir;

fn migrate(path: &std::path::Path, component: &str, from: i64, to: i64) {
    let connection = Connection::open_with_flags(
        path,
        OpenFlags::SQLITE_OPEN_READ_WRITE | OpenFlags::SQLITE_OPEN_CREATE,
    )
    .expect("open database");
    let transaction = connection
        .unchecked_transaction()
        .expect("begin transaction");
    migrate_legacy_schema(
        &transaction,
        &path.display().to_string(),
        component,
        from,
        to,
    )
    .expect("apply legacy migration");
    validate_current(&transaction, &path.display().to_string(), component, to)
        .expect("validate migrated schema");
    transaction.commit().expect("commit migration");
}

#[test]
fn strategy_v1_migration_creates_definition_history_objects() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("strategy.db");
    let connection = Connection::open(&path).expect("create database");
    initialize_current(&connection, "strategy").expect("current schema");
    connection
        .execute_batch(
            "INSERT INTO strategy_design_definitions
                (id, name, version, description, runtime, source_format, symbol,
                 interval, script, visual_model_json, created_at, updated_at)
             VALUES
                ('definition-1', 'Momentum', 'v1', 'description', 'pine', 'pine',
                 'HK.00700', '1m', 'close > open', '{\"nodes\":[]}', 't0', 't1');
             DROP TRIGGER trg_strategy_definition_versions_immutable;
             DROP INDEX idx_strategy_definition_versions_saved_at;
             DROP TABLE strategy_definition_versions;
             UPDATE jftrade_schema_meta SET version = 1 WHERE component_id = 'strategy';",
        )
        .expect("shape legacy strategy schema");
    drop(connection);

    migrate(&path, "strategy", 1, 2);
    let migrated = Connection::open(&path).expect("reopen strategy database");
    assert_eq!(current_version(&migrated, "strategy"), Some(2));
    assert!(
        migrated
            .query_row(
                "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'strategy_definition_versions'",
                [],
                |row| row.get::<_, i64>(0),
            )
            .expect("history table")
            == 1
    );
    let history = migrated
        .query_row(
            "SELECT definition_id, version, name, description, runtime,
                    source_format, symbol, interval, script, visual_model_json,
                    created_at, updated_at, saved_at
             FROM strategy_definition_versions WHERE definition_id = 'definition-1'",
            [],
            |row| {
                Ok(vec![
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, String>(3)?,
                    row.get::<_, String>(4)?,
                    row.get::<_, String>(5)?,
                    row.get::<_, String>(6)?,
                    row.get::<_, String>(7)?,
                    row.get::<_, String>(8)?,
                    row.get::<_, String>(9)?,
                    row.get::<_, String>(10)?,
                    row.get::<_, String>(11)?,
                    row.get::<_, String>(12)?,
                ])
            },
        )
        .expect("backfilled definition history");
    assert_eq!(
        history,
        vec![
            "definition-1".to_owned(),
            "v1".to_owned(),
            "Momentum".to_owned(),
            "description".to_owned(),
            "pine".to_owned(),
            "pine".to_owned(),
            "HK.00700".to_owned(),
            "1m".to_owned(),
            "close > open".to_owned(),
            r#"{"nodes":[]}"#.to_owned(),
            "t0".to_owned(),
            "t1".to_owned(),
            "t1".to_owned(),
        ]
    );
}

#[test]
fn adk_v2_migration_rebuilds_runs_without_losing_payload() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("adk.db");
    let connection = Connection::open(&path).expect("create database");
    initialize_current(&connection, "adk").expect("current schema");
    connection
        .execute_batch(
            "DROP INDEX idx_adk_runs_client_request;
             DROP INDEX idx_adk_runs_session;
             DROP TABLE adk_runs;
             CREATE TABLE adk_runs (
                 id TEXT PRIMARY KEY,
                 session_id TEXT NOT NULL,
                 agent_id TEXT NOT NULL,
                 status TEXT NOT NULL,
                 payload_json TEXT NOT NULL,
                 created_at TEXT NOT NULL,
                 updated_at TEXT NOT NULL
             );
             INSERT INTO adk_runs VALUES ('run-1', 'session-1', 'agent-1', 'queued', '{}', 't0', 't1');
             UPDATE jftrade_schema_meta SET version = 2 WHERE component_id = 'adk';",
        )
        .expect("shape legacy ADK schema");
    drop(connection);

    migrate(&path, "adk", 2, 4);
    let migrated = Connection::open(&path).expect("reopen ADK database");
    assert_eq!(current_version(&migrated, "adk"), Some(4));
    let row = migrated
        .query_row(
            "SELECT payload_json, client_request_id, request_fingerprint FROM adk_runs WHERE id = 'run-1'",
            [],
            |row| Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?, row.get::<_, String>(2)?)),
        )
        .expect("migrated run");
    assert_eq!(row, ("{}".to_owned(), String::new(), String::new()));
}

#[test]
fn backtest_v2_migration_renames_legacy_kline_tables_in_place() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("backtest.db");
    let connection = Connection::open(&path).expect("create database");
    initialize_current(&connection, "backtest").expect("current schema");
    connection
        .execute_batch(
            "ALTER TABLE local_klines__manifest__symbol__1m__forward__r__00000000
                 RENAME TO local_klines__manifest__1m__forward__r__00000000;
             UPDATE jftrade_schema_meta SET version = 2 WHERE component_id = 'backtest';",
        )
        .expect("shape legacy backtest schema");
    drop(connection);

    migrate(&path, "backtest", 2, 3);
    let migrated = Connection::open(&path).expect("reopen backtest database");
    assert_eq!(current_version(&migrated, "backtest"), Some(3));
    let exists = migrated
        .query_row(
            "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'local_klines__manifest__symbol__1m__forward__r__00000000'",
            [],
            |row| row.get::<_, i64>(0),
        )
        .expect("renamed kline table");
    assert_eq!(exists, 1);
}

#[test]
fn backtest_v2_migration_preserves_legacy_symbol_data_under_go_table_name() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("backtest.db");
    let connection = Connection::open(&path).expect("create database");
    initialize_current(&connection, "backtest").expect("current schema");
    connection
        .execute_batch(
            "ALTER TABLE local_klines__manifest__symbol__1m__forward__r__00000000
                 RENAME TO local_klines__manifest__1m__forward__r__00000000;
             CREATE TABLE local_klines__hk_00700__1m__forward__r__0c59bfa3 (
                 end_time INTEGER NOT NULL,
                 start_time INTEGER NOT NULL,
                 open TEXT NOT NULL,
                 high TEXT NOT NULL,
                 low TEXT NOT NULL,
                 close TEXT NOT NULL,
                 volume TEXT NOT NULL,
                 PRIMARY KEY (end_time)
             ) WITHOUT ROWID;
             INSERT INTO local_klines__hk_00700__1m__forward__r__0c59bfa3
                 (end_time, start_time, open, high, low, close, volume)
             VALUES (2000, 1000, '100000000', '120000000', '90000000', '110000000', '42');
             UPDATE jftrade_schema_meta SET version = 2 WHERE component_id = 'backtest';",
        )
        .expect("shape legacy backtest schema");
    drop(connection);

    migrate(&path, "backtest", 2, 3);
    let migrated = Connection::open(&path).expect("reopen backtest database");
    let target_exists = migrated
        .query_row(
            "SELECT EXISTS(
                SELECT 1 FROM sqlite_master
                WHERE type = 'table'
                  AND name = 'local_klines__futu__hk_00700__1m__forward__r__53d46117'
             )",
            [],
            |row| row.get::<_, i64>(0),
        )
        .expect("target table");
    assert_eq!(target_exists, 1);
    drop(migrated);

    let store = BacktestMarketDataStore::open(&path).expect("open migrated store");
    let candles = store
        .read_candles("futu", "HK.00700", "1m", "forward", "regular", 0, 3000)
        .expect("read migrated candles");
    assert_eq!(
        candles,
        vec![StoredBacktestCandle {
            start_time: 1000,
            end_time: 2000,
            open: "100000000".to_owned(),
            high: "120000000".to_owned(),
            low: "90000000".to_owned(),
            close: "110000000".to_owned(),
            volume: "42".to_owned(),
        }]
    );
}

#[test]
fn backtest_v2_migration_rejects_unrecoverable_legacy_symbol_hash() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("backtest.db");
    let connection = Connection::open(&path).expect("create database");
    initialize_current(&connection, "backtest").expect("current schema");
    connection
        .execute_batch(
            "ALTER TABLE local_klines__manifest__symbol__1m__forward__r__00000000
                 RENAME TO local_klines__manifest__1m__forward__r__00000000;
             CREATE TABLE local_klines__hk_00700__1m__forward__r__deadbeef (
                 end_time INTEGER NOT NULL,
                 start_time INTEGER NOT NULL,
                 open TEXT NOT NULL,
                 high TEXT NOT NULL,
                 low TEXT NOT NULL,
                 close TEXT NOT NULL,
                 volume TEXT NOT NULL,
                 PRIMARY KEY (end_time)
             ) WITHOUT ROWID;
             UPDATE jftrade_schema_meta SET version = 2 WHERE component_id = 'backtest';",
        )
        .expect("shape invalid legacy schema");

    let transaction = connection.unchecked_transaction().expect("begin migration");
    let result = migrate_legacy_schema(&transaction, &path.display().to_string(), "backtest", 2, 3);
    assert!(
        result.is_err(),
        "unrecoverable symbol hash must fail closed"
    );
    drop(transaction);
    assert_eq!(current_version(&connection, "backtest"), Some(2));
}

#[test]
fn failed_migration_rolls_back_metadata_and_preserves_original_bytes() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("strategy.db");
    let connection = Connection::open(&path).expect("create database");
    initialize_current(&connection, "strategy").expect("current schema");
    connection
        .execute_batch(
            "DROP TRIGGER trg_strategy_definition_versions_immutable;
             DROP INDEX idx_strategy_definition_versions_saved_at;
             DROP TABLE strategy_definition_versions;
             CREATE TABLE strategy_definition_versions (broken TEXT);
             UPDATE jftrade_schema_meta SET version = 1 WHERE component_id = 'strategy';",
        )
        .expect("shape incompatible strategy schema");
    drop(connection);
    let before = fs::read(&path).expect("read original database");

    let connection = Connection::open(&path).expect("reopen database");
    let transaction = connection
        .unchecked_transaction()
        .expect("begin transaction");
    assert!(
        migrate_legacy_schema(&transaction, &path.display().to_string(), "strategy", 1, 2).is_err()
    );
    drop(transaction);

    let after = fs::read(&path).expect("read rolled back database");
    assert_eq!(before, after);
    let reopened = Connection::open(&path).expect("open rolled back schema");
    assert_eq!(current_version(&reopened, "strategy"), Some(1));
}
