//! Durable backtests test-cutover adapter.
//!
//! This module is compiled only for Rust tests. Its SQLite schema is isolated
//! from the Go production run/task stores and it never starts PineTS, a
//! market-data worker, Provider/OpenD, notifications, or user-visible tasks.

use rusqlite::OptionalExtension;
use serde_json::{Value, json};

use super::product_backtests_write_port::{
    BacktestsWriteDeleteResult, BacktestsWriteInput, BacktestsWritePort, BacktestsWritePortError,
    BacktestsWritePortResult,
};

pub struct BacktestsSqliteTestCutoverPort {
    path: std::path::PathBuf,
    connection: std::sync::Mutex<rusqlite::Connection>,
}

impl std::fmt::Debug for BacktestsSqliteTestCutoverPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("BacktestsSqliteTestCutoverPort")
            .field("path", &self.path)
            .finish()
    }
}

#[allow(dead_code)]
impl BacktestsSqliteTestCutoverPort {
    pub fn open(path: impl AsRef<std::path::Path>) -> Result<Self, String> {
        let path = path.as_ref().to_owned();
        let connection = rusqlite::Connection::open(&path).map_err(|error| error.to_string())?;
        connection
            .execute_batch(
                "PRAGMA foreign_keys = ON;
                 CREATE TABLE IF NOT EXISTS backtests_test_ids (
                    singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
                    next_value INTEGER NOT NULL
                 );
                 INSERT OR IGNORE INTO backtests_test_ids (singleton, next_value) VALUES (1, 1);
                 CREATE TABLE IF NOT EXISTS backtests_test_runs (
                    id TEXT PRIMARY KEY,
                    status TEXT NOT NULL,
                    payload TEXT NOT NULL
                 );
                 CREATE TABLE IF NOT EXISTS backtests_test_tasks (
                    id TEXT PRIMARY KEY,
                    status TEXT NOT NULL,
                    payload TEXT NOT NULL
                 );
                 CREATE TABLE IF NOT EXISTS backtests_test_events (
                    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
                    operation TEXT NOT NULL,
                    resource_id TEXT NOT NULL,
                    payload TEXT NOT NULL
                 );",
            )
            .map_err(|error| error.to_string())?;
        Ok(Self {
            path,
            connection: std::sync::Mutex::new(connection),
        })
    }

    pub fn seed_run(&self, id: &str, status: &str) -> Result<(), String> {
        let connection = self.lock()?;
        connection
            .execute(
                "INSERT OR REPLACE INTO backtests_test_runs (id, status, payload)
                 VALUES (?1, ?2, '{}')",
                rusqlite::params![id, status],
            )
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    pub fn seed_task(&self, id: &str, status: &str) -> Result<(), String> {
        let connection = self.lock()?;
        connection
            .execute(
                "INSERT OR REPLACE INTO backtests_test_tasks (id, status, payload)
                 VALUES (?1, ?2, '{}')",
                rusqlite::params![id, status],
            )
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    pub fn run_count(&self) -> Result<u64, String> {
        self.count_rows("backtests_test_runs")
    }

    pub fn run_exists(&self, id: &str) -> Result<bool, String> {
        let connection = self.lock()?;
        connection
            .query_row(
                "SELECT 1 FROM backtests_test_runs WHERE id = ?1",
                rusqlite::params![id],
                |_| Ok(true),
            )
            .optional()
            .map(|value| value.unwrap_or(false))
            .map_err(|error| error.to_string())
    }

    pub fn task_status(&self, id: &str) -> Result<Option<String>, String> {
        let connection = self.lock()?;
        connection
            .query_row(
                "SELECT status FROM backtests_test_tasks WHERE id = ?1",
                rusqlite::params![id],
                |row| row.get(0),
            )
            .optional()
            .map_err(|error| error.to_string())
    }

    pub fn event_count(&self, operation: &str) -> Result<u64, String> {
        let connection = self.lock()?;
        let count = connection
            .query_row(
                "SELECT COUNT(*) FROM backtests_test_events WHERE operation = ?1",
                rusqlite::params![operation],
                |row| row.get::<_, i64>(0),
            )
            .map_err(|error| error.to_string())?;
        u64::try_from(count).map_err(|_| "negative backtests event count".to_owned())
    }

    pub fn reject_start_event(&self) -> Result<(), String> {
        let connection = self.lock()?;
        connection
            .execute_batch(
                "DROP TRIGGER IF EXISTS backtests_test_reject_start;
                 CREATE TRIGGER backtests_test_reject_start
                 BEFORE INSERT ON backtests_test_events
                 WHEN NEW.operation = 'start' BEGIN
                    SELECT RAISE(ABORT, 'test-cutover start rejection');
                 END;",
            )
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    pub fn clear_rejection(&self) -> Result<(), String> {
        let connection = self.lock()?;
        connection
            .execute_batch("DROP TRIGGER IF EXISTS backtests_test_reject_start")
            .map_err(|error| error.to_string())?;
        Ok(())
    }

    fn count_rows(&self, table: &str) -> Result<u64, String> {
        let connection = self.lock()?;
        let statement = format!("SELECT COUNT(*) FROM {table}");
        let count = connection
            .query_row(&statement, [], |row| row.get::<_, i64>(0))
            .map_err(|error| error.to_string())?;
        u64::try_from(count).map_err(|_| "negative backtests row count".to_owned())
    }

    fn lock(&self) -> Result<std::sync::MutexGuard<'_, rusqlite::Connection>, String> {
        self.connection
            .lock()
            .map_err(|_| "backtests fixture lock poisoned".to_owned())
    }

    fn mutate_transaction(
        &self,
        input: &BacktestsWriteInput,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| failed("backtests fixture lock poisoned"))?;
        let transaction = connection
            .unchecked_transaction()
            .map_err(|_| failed("backtests transaction failed"))?;
        let (result, event) = match input {
            BacktestsWriteInput::Start { payload } => {
                let (result, id) = start_run(&transaction, payload)?;
                (result, Some(("start", id, payload.clone())))
            }
            BacktestsWriteInput::Sync { payload } => {
                let (result, id) = start_sync(&transaction, payload)?;
                (result, Some(("sync", id, payload.clone())))
            }
            BacktestsWriteInput::CancelSync { task_id } => {
                let cancelled = cancel_sync(&transaction, task_id)?;
                let event = cancelled.then_some(("cancel-sync", task_id.clone(), Value::Null));
                (BacktestsWritePortResult::SyncCancelled(cancelled), event)
            }
            BacktestsWriteInput::Delete { run_id } => {
                let deleted = delete_run(&transaction, run_id)?;
                let event = (deleted == BacktestsWriteDeleteResult::Deleted).then_some((
                    "delete",
                    run_id.clone(),
                    Value::Null,
                ));
                (BacktestsWritePortResult::RunDeleted(deleted), event)
            }
        };
        if let Some((operation, resource_id, payload)) = event {
            insert_event(&transaction, operation, &resource_id, &payload)?;
        }
        transaction
            .commit()
            .map_err(|_| failed("backtests commit failed"))?;
        Ok(result)
    }
}

impl BacktestsWritePort for BacktestsSqliteTestCutoverPort {
    fn mutate(
        &self,
        input: &BacktestsWriteInput,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        self.mutate_transaction(input)
    }
}

fn start_run(
    transaction: &rusqlite::Transaction<'_>,
    payload: &Value,
) -> Result<(BacktestsWritePortResult, String), BacktestsWritePortError> {
    let id = next_id(transaction, "run")?;
    insert_resource(transaction, "backtests_test_runs", &id, "queued", payload)?;
    let result = BacktestsWritePortResult::Data(json!({
        "id": id.clone(),
        "status": "queued",
        "message": "backtest queued",
    }));
    Ok((result, id))
}

fn start_sync(
    transaction: &rusqlite::Transaction<'_>,
    payload: &Value,
) -> Result<(BacktestsWritePortResult, String), BacktestsWritePortError> {
    let id = next_id(transaction, "task")?;
    insert_resource(transaction, "backtests_test_tasks", &id, "running", payload)?;
    let result = BacktestsWritePortResult::Data(json!({"taskId": id.clone(), "status": "running"}));
    Ok((result, id))
}

fn next_id(
    transaction: &rusqlite::Transaction<'_>,
    prefix: &str,
) -> Result<String, BacktestsWritePortError> {
    let value = transaction
        .query_row(
            "SELECT next_value FROM backtests_test_ids WHERE singleton = 1",
            [],
            |row| row.get::<_, i64>(0),
        )
        .map_err(|_| failed("backtests id allocation failed"))?;
    transaction
        .execute(
            "UPDATE backtests_test_ids SET next_value = next_value + 1 WHERE singleton = 1",
            [],
        )
        .map_err(|_| failed("backtests id allocation failed"))?;
    Ok(format!("{prefix}-test-{value}"))
}

fn insert_resource(
    transaction: &rusqlite::Transaction<'_>,
    table: &str,
    id: &str,
    status: &str,
    payload: &Value,
) -> Result<(), BacktestsWritePortError> {
    let statement = format!("INSERT INTO {table} (id, status, payload) VALUES (?1, ?2, ?3)");
    let payload = serde_json::to_string(payload).map_err(|_| failed("payload encode failed"))?;
    transaction
        .execute(&statement, rusqlite::params![id, status, payload])
        .map_err(|_| failed("backtests resource write failed"))?;
    Ok(())
}

fn cancel_sync(
    transaction: &rusqlite::Transaction<'_>,
    task_id: &str,
) -> Result<bool, BacktestsWritePortError> {
    transaction
        .execute(
            "UPDATE backtests_test_tasks SET status = 'cancelled'
             WHERE id = ?1 AND status = 'running'",
            rusqlite::params![task_id],
        )
        .map(|changed| changed == 1)
        .map_err(|_| failed("backtests sync cancellation failed"))
}

fn delete_run(
    transaction: &rusqlite::Transaction<'_>,
    run_id: &str,
) -> Result<BacktestsWriteDeleteResult, BacktestsWritePortError> {
    let status = transaction
        .query_row(
            "SELECT status FROM backtests_test_runs WHERE id = ?1",
            rusqlite::params![run_id],
            |row| row.get::<_, String>(0),
        )
        .optional()
        .map_err(|_| failed("backtests run load failed"))?;
    let Some(status) = status else {
        return Ok(BacktestsWriteDeleteResult::Missing);
    };
    if !matches!(status.as_str(), "completed" | "failed" | "cancelled") {
        return Ok(BacktestsWriteDeleteResult::NotTerminal);
    }
    transaction
        .execute(
            "DELETE FROM backtests_test_runs WHERE id = ?1",
            rusqlite::params![run_id],
        )
        .map_err(|_| failed("backtests run delete failed"))?;
    Ok(BacktestsWriteDeleteResult::Deleted)
}

fn insert_event(
    transaction: &rusqlite::Transaction<'_>,
    operation: &str,
    resource_id: &str,
    payload: &Value,
) -> Result<(), BacktestsWritePortError> {
    let payload = serde_json::to_string(payload).map_err(|_| failed("payload encode failed"))?;
    transaction
        .execute(
            "INSERT INTO backtests_test_events (operation, resource_id, payload)
             VALUES (?1, ?2, ?3)",
            rusqlite::params![operation, resource_id, payload],
        )
        .map_err(|_| failed("backtests event write failed"))?;
    Ok(())
}

fn failed(message: &str) -> BacktestsWritePortError {
    BacktestsWritePortError::Failed(message.to_owned())
}
