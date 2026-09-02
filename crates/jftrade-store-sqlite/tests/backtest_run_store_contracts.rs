use std::path::Path;

use jftrade_owner_lock::WriterLeaseError;
use jftrade_store_sqlite::{
    BACKTEST_RUNS_TEST_CUTOVER_PROFILE, BacktestRunStoreError, BacktestRunTestCutoverStore,
    StoredBacktestRun,
};
use rusqlite::Connection;

const TIMESTAMP_1: &str = "2026-08-22T06:00:00Z";
const TIMESTAMP_2: &str = "2026-08-22T06:01:00Z";

#[test]
fn backtest_runs_store_rejects_missing_drifted_and_corrupted_go_databases() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let missing_path = directory.path().join("missing-runs.db");
    assert!(matches!(
        BacktestRunTestCutoverStore::open_existing(
            &missing_path,
            BACKTEST_RUNS_TEST_CUTOVER_PROFILE
        ),
        Err(BacktestRunStoreError::NotRegularFile(_))
    ));

    let drifted_path = directory.path().join("drifted-runs.db");
    let connection = Connection::open(&drifted_path).expect("create drifted db");
    connection
        .execute_batch(
            "CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                VALUES ('backtest-runs', 1, '2026-08-22T06:00:00Z');
            CREATE TABLE backtest_runs (
                id TEXT PRIMARY KEY,
                rogue_column TEXT NOT NULL
            );",
        )
        .expect("seed rogue table");
    drop(connection);

    let error = BacktestRunTestCutoverStore::open_existing(
        &drifted_path,
        BACKTEST_RUNS_TEST_CUTOVER_PROFILE,
    )
    .expect_err("drifted schema must fail");
    assert!(matches!(error, BacktestRunStoreError::Schema(_)));
}

#[test]
fn backtest_runs_lifecycle_and_restart_durability() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("runs.db");
    seed_go_backtest_runs_schema(&path);

    let store = open_store(&path);
    assert_eq!(store.path(), path);

    let conflict =
        BacktestRunTestCutoverStore::open_existing(&path, BACKTEST_RUNS_TEST_CUTOVER_PROFILE)
            .expect_err("second writer must fail");
    assert!(matches!(
        conflict,
        BacktestRunStoreError::WriterLease(WriterLeaseError::Held { .. })
    ));

    let run1 = StoredBacktestRun {
        id: "run-1".to_owned(),
        status: "queued".to_owned(),
        request_json: r#"{"symbol":"US.AAPL"}"#.to_owned(),
        result_json: "".to_owned(),
        created_at: TIMESTAMP_1.to_owned(),
        updated_at: TIMESTAMP_1.to_owned(),
    };

    store.save_run(run1, TIMESTAMP_1).expect("save run 1");
    assert_eq!(store.run_count().expect("run count"), 1);

    let loaded = store
        .get_run("run-1")
        .expect("get run")
        .expect("must exist");
    assert_eq!(loaded.id, "run-1");
    assert_eq!(loaded.status, "queued");

    let non_terminal_delete = store
        .delete_run("run-1")
        .expect_err("queued run cannot be deleted");
    assert!(matches!(
        non_terminal_delete,
        BacktestRunStoreError::NotTerminal(_)
    ));

    let mut updated = loaded.clone();
    updated.status = "completed".to_owned();
    updated.result_json = r#"{"pnl":100.0}"#.to_owned();
    store
        .save_run(updated, TIMESTAMP_2)
        .expect("update run status to completed");

    let deleted = store.delete_run("run-1").expect("delete completed run");
    assert!(deleted);
    assert_eq!(store.run_count().expect("run count after delete"), 0);

    let missing_delete = store.delete_run("missing-run").expect("delete missing");
    assert!(!missing_delete);

    drop(store);

    let reopened = open_store(&path);
    assert_eq!(reopened.run_count().expect("reopened run count"), 0);
}

fn open_store(path: &Path) -> BacktestRunTestCutoverStore {
    BacktestRunTestCutoverStore::open_existing(path, BACKTEST_RUNS_TEST_CUTOVER_PROFILE)
        .expect("open backtest runs test-cutover store")
}

fn seed_go_backtest_runs_schema(path: &Path) {
    let connection = Connection::open(path).expect("create backtest runs fixture");
    connection
        .execute_batch(
            "CREATE TABLE backtest_runs (
                id TEXT PRIMARY KEY,
                status TEXT NOT NULL DEFAULT '',
                request_json TEXT NOT NULL DEFAULT '',
                result_json TEXT NOT NULL DEFAULT '',
                created_at TEXT NOT NULL DEFAULT '',
                updated_at TEXT NOT NULL DEFAULT ''
            );
            CREATE INDEX idx_backtest_runs_updated_at ON backtest_runs (updated_at DESC, id ASC);
            CREATE INDEX idx_backtest_runs_status ON backtest_runs (status, updated_at DESC);
            CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                VALUES ('backtest-runs', 1, '2026-08-22T06:00:00Z');",
        )
        .expect("seed Go-compatible backtest-runs schema");
}
