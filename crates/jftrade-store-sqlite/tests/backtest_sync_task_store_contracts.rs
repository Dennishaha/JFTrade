use std::path::Path;
use std::sync::Arc;

use jftrade_owner_lock::WriterLeaseError;
use jftrade_store_sqlite::{
    BACKTEST_RUNS_PRODUCTION_PROFILE, BacktestRunStoreError, BacktestSyncTaskStore,
    CancelBacktestSyncResult, StoredBacktestSyncTask,
};
use rusqlite::Connection;

const TIMESTAMP_1: &str = "2026-08-29T00:00:00Z";
const TIMESTAMP_2: &str = "2026-08-29T00:01:00Z";

#[test]
fn sync_task_persists_across_reopen_and_uses_run_store_lease() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("backtest-runs.db");
    seed_schema(&path);
    let store = open_store(&path);
    let created = task("sync-1", "queued", TIMESTAMP_1);
    store.create(created.clone()).expect("create task");
    assert_eq!(store.get("sync-1").expect("get task"), Some(created));
    assert!(matches!(
        BacktestSyncTaskStore::open(&path, BACKTEST_RUNS_PRODUCTION_PROFILE),
        Err(BacktestRunStoreError::WriterLease(
            WriterLeaseError::Held { .. }
        ))
    ));
    drop(store);
    let reopened = open_store(&path);
    assert_eq!(
        reopened.get("sync-1").expect("reopen get").unwrap().status,
        "queued"
    );
}

#[test]
fn sync_task_cancel_distinguishes_missing_active_and_terminal() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("backtest-runs.db");
    seed_schema(&path);
    let store = open_store(&path);
    store
        .create(task("active", "running", TIMESTAMP_1))
        .expect("create active");
    store
        .create(task("done", "completed", TIMESTAMP_1))
        .expect("create terminal");
    assert_eq!(
        store.cancel("active", TIMESTAMP_2).expect("cancel active"),
        CancelBacktestSyncResult::Cancelled
    );
    assert_eq!(
        store.get("active").expect("read cancelled").unwrap().status,
        "cancelled"
    );
    assert_eq!(
        store.cancel("done", TIMESTAMP_2).expect("terminal cancel"),
        CancelBacktestSyncResult::AlreadyTerminal
    );
    assert_eq!(
        store
            .cancel("missing", TIMESTAMP_2)
            .expect("missing cancel"),
        CancelBacktestSyncResult::Missing
    );
}

#[test]
fn sync_task_update_enforces_revision_compare_and_swap() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("backtest-runs.db");
    seed_schema(&path);
    let store = open_store(&path);
    let mut value = task("sync-cas", "running", TIMESTAMP_1);
    store.create(value.clone()).expect("create task");
    value.status = "completed".to_owned();
    value.updated_at = TIMESTAMP_2.to_owned();
    assert!(store.update(value.clone(), 0).expect("update task"));
    assert!(!store.update(value, 0).expect("stale update"));
    assert_eq!(
        store.get("sync-cas").expect("read task").unwrap().revision,
        1
    );
}

#[test]
fn malformed_internal_sync_table_fails_closed_without_replacing_database() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("backtest-runs.db");
    seed_schema(&path);
    let connection = Connection::open(&path).expect("open database");
    connection
        .execute_batch(
            "CREATE TABLE jftrade_internal__backtest_sync_tasks (task_id TEXT PRIMARY KEY)",
        )
        .expect("create malformed internal table");
    drop(connection);
    let before = std::fs::read(&path).expect("read database before open");
    let result = BacktestSyncTaskStore::open(&path, BACKTEST_RUNS_PRODUCTION_PROFILE);
    assert!(matches!(
        result,
        Err(BacktestRunStoreError::Incompatible(_))
    ));
    assert_eq!(
        std::fs::read(&path).expect("read database after open"),
        before
    );
}

fn task(id: &str, status: &str, timestamp: &str) -> StoredBacktestSyncTask {
    StoredBacktestSyncTask {
        task_id: id.to_owned(),
        status: status.to_owned(),
        symbol: "US.AAPL".to_owned(),
        market_data_provider: "yfinance".to_owned(),
        total_intervals: 2,
        completed_intervals: 0,
        total_batches: 2,
        completed_batches: 0,
        current_interval: String::new(),
        retries: 0,
        error: None,
        started_at: timestamp.to_owned(),
        updated_at: timestamp.to_owned(),
        revision: 0,
    }
}

fn open_store(path: &Path) -> Arc<BacktestSyncTaskStore> {
    Arc::new(
        BacktestSyncTaskStore::open(path, BACKTEST_RUNS_PRODUCTION_PROFILE)
            .expect("open sync task store"),
    )
}

fn seed_schema(path: &Path) {
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
                VALUES ('backtest-runs', 1, '2026-08-29T00:00:00Z');",
        )
        .expect("seed schema");
    drop(connection);
}
