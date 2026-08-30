use std::path::Path;
use std::sync::Arc;

use rusqlite::{Connection, OptionalExtension, TransactionBehavior, params};
use serde::{Deserialize, Serialize};

use crate::backtest_run::{BacktestRunStore, BacktestRunStoreError, validate_rfc3339_timestamp};

const SYNC_TASKS_TABLE: &str = "jftrade_internal__backtest_sync_tasks";
const SYNC_TASKS_INDEX: &str = "idx_backtest_sync_tasks_updated_at";
const SYNC_TASKS_DDL: &str = "CREATE TABLE jftrade_internal__backtest_sync_tasks (
    task_id TEXT PRIMARY KEY,
    status TEXT NOT NULL,
    symbol TEXT NOT NULL,
    market_data_provider TEXT NOT NULL,
    total_intervals INTEGER NOT NULL,
    completed_intervals INTEGER NOT NULL,
    total_batches INTEGER NOT NULL,
    completed_batches INTEGER NOT NULL,
    current_interval TEXT NOT NULL,
    retries INTEGER NOT NULL,
    error TEXT,
    started_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    revision INTEGER NOT NULL
);
CREATE INDEX idx_backtest_sync_tasks_updated_at
    ON jftrade_internal__backtest_sync_tasks (updated_at DESC, task_id ASC);";

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredBacktestSyncTask {
    pub task_id: String,
    pub status: String,
    pub symbol: String,
    pub market_data_provider: String,
    pub total_intervals: i64,
    pub completed_intervals: i64,
    pub total_batches: i64,
    pub completed_batches: i64,
    pub current_interval: String,
    pub retries: i64,
    pub error: Option<String>,
    pub started_at: String,
    pub updated_at: String,
    pub revision: i64,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum CancelBacktestSyncResult {
    Cancelled,
    Missing,
    AlreadyTerminal,
}

impl BacktestRunStore {
    pub(crate) fn create_sync_task(
        &self,
        task: &StoredBacktestSyncTask,
    ) -> Result<(), BacktestRunStoreError> {
        validate_sync_task(task)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(BacktestRunStoreError::Query)?;
        transaction
            .execute(
                &format!(
                    "INSERT INTO {SYNC_TASKS_TABLE}
                    (task_id, status, symbol, market_data_provider, total_intervals,
                     completed_intervals, total_batches, completed_batches,
                     current_interval, retries, error, started_at, updated_at, revision)
                    VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14)"
                ),
                params![
                    task.task_id,
                    task.status,
                    task.symbol,
                    task.market_data_provider,
                    task.total_intervals,
                    task.completed_intervals,
                    task.total_batches,
                    task.completed_batches,
                    task.current_interval,
                    task.retries,
                    task.error,
                    task.started_at,
                    task.updated_at,
                    task.revision,
                ],
            )
            .map_err(|error| {
                if matches!(error, rusqlite::Error::SqliteFailure(ref e, _)
                    if e.extended_code == rusqlite::ffi::SQLITE_CONSTRAINT_PRIMARYKEY
                        || e.extended_code == rusqlite::ffi::SQLITE_CONSTRAINT)
                {
                    BacktestRunStoreError::Conflict(format!(
                        "sync task already exists: {}",
                        task.task_id
                    ))
                } else {
                    BacktestRunStoreError::Query(error)
                }
            })?;
        transaction.commit().map_err(BacktestRunStoreError::Query)
    }

    pub(crate) fn get_sync_task(
        &self,
        task_id: &str,
    ) -> Result<Option<StoredBacktestSyncTask>, BacktestRunStoreError> {
        let connection = self.lock()?;
        let task = connection
            .query_row(
                &format!(
                    "SELECT task_id, status, symbol, market_data_provider,
                    total_intervals, completed_intervals, total_batches,
                    completed_batches, current_interval, retries, error,
                    started_at, updated_at, revision
                    FROM {SYNC_TASKS_TABLE} WHERE task_id = ?1"
                ),
                params![task_id],
                |row| {
                    Ok(StoredBacktestSyncTask {
                        task_id: row.get(0)?,
                        status: row.get(1)?,
                        symbol: row.get(2)?,
                        market_data_provider: row.get(3)?,
                        total_intervals: row.get(4)?,
                        completed_intervals: row.get(5)?,
                        total_batches: row.get(6)?,
                        completed_batches: row.get(7)?,
                        current_interval: row.get(8)?,
                        retries: row.get(9)?,
                        error: row.get(10)?,
                        started_at: row.get(11)?,
                        updated_at: row.get(12)?,
                        revision: row.get(13)?,
                    })
                },
            )
            .optional()
            .map_err(BacktestRunStoreError::Query)?;
        if let Some(task) = &task {
            validate_sync_task(task)?;
        }
        Ok(task)
    }

    pub(crate) fn cancel_sync_task(
        &self,
        task_id: &str,
        timestamp: &str,
    ) -> Result<CancelBacktestSyncResult, BacktestRunStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(BacktestRunStoreError::Query)?;
        let task: Option<StoredBacktestSyncTask> = transaction
            .query_row(
                &format!(
                    "SELECT task_id, status, symbol, market_data_provider,
                    total_intervals, completed_intervals, total_batches,
                    completed_batches, current_interval, retries, error,
                    started_at, updated_at, revision
                    FROM {SYNC_TASKS_TABLE} WHERE task_id = ?1"
                ),
                params![task_id],
                |row| {
                    Ok(StoredBacktestSyncTask {
                        task_id: row.get(0)?,
                        status: row.get(1)?,
                        symbol: row.get(2)?,
                        market_data_provider: row.get(3)?,
                        total_intervals: row.get(4)?,
                        completed_intervals: row.get(5)?,
                        total_batches: row.get(6)?,
                        completed_batches: row.get(7)?,
                        current_interval: row.get(8)?,
                        retries: row.get(9)?,
                        error: row.get(10)?,
                        started_at: row.get(11)?,
                        updated_at: row.get(12)?,
                        revision: row.get(13)?,
                    })
                },
            )
            .optional()
            .map_err(BacktestRunStoreError::Query)?;
        let Some(task) = task else {
            transaction.commit().map_err(BacktestRunStoreError::Query)?;
            return Ok(CancelBacktestSyncResult::Missing);
        };
        validate_sync_task(&task)?;
        if matches!(task.status.as_str(), "completed" | "failed" | "cancelled") {
            transaction.commit().map_err(BacktestRunStoreError::Query)?;
            return Ok(CancelBacktestSyncResult::AlreadyTerminal);
        }
        let changed = transaction
            .execute(
                &format!(
                    "UPDATE {SYNC_TASKS_TABLE}
                    SET status = 'cancelled', updated_at = ?2, revision = revision + 1
                    WHERE task_id = ?1 AND revision = ?3"
                ),
                params![task_id, timestamp, task.revision],
            )
            .map_err(BacktestRunStoreError::Query)?;
        transaction.commit().map_err(BacktestRunStoreError::Query)?;
        if changed == 1 {
            Ok(CancelBacktestSyncResult::Cancelled)
        } else {
            Ok(CancelBacktestSyncResult::AlreadyTerminal)
        }
    }

    pub(crate) fn update_sync_task(
        &self,
        task: &StoredBacktestSyncTask,
        expected_revision: i64,
    ) -> Result<bool, BacktestRunStoreError> {
        validate_sync_task(task)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(BacktestRunStoreError::Query)?;
        let changed = transaction
            .execute(
                &format!(
                    "UPDATE {SYNC_TASKS_TABLE} SET status = ?2, symbol = ?3,
                    market_data_provider = ?4, total_intervals = ?5,
                    completed_intervals = ?6, total_batches = ?7,
                    completed_batches = ?8, current_interval = ?9, retries = ?10,
                    error = ?11, started_at = ?12, updated_at = ?13,
                    revision = revision + 1 WHERE task_id = ?1 AND revision = ?14"
                ),
                params![
                    task.task_id,
                    task.status,
                    task.symbol,
                    task.market_data_provider,
                    task.total_intervals,
                    task.completed_intervals,
                    task.total_batches,
                    task.completed_batches,
                    task.current_interval,
                    task.retries,
                    task.error,
                    task.started_at,
                    task.updated_at,
                    expected_revision,
                ],
            )
            .map_err(BacktestRunStoreError::Query)?;
        transaction.commit().map_err(BacktestRunStoreError::Query)?;
        Ok(changed == 1)
    }
}

#[derive(Clone, Debug)]
pub struct BacktestSyncTaskStore {
    run_store: Arc<BacktestRunStore>,
}

impl BacktestSyncTaskStore {
    pub fn open(path: impl AsRef<Path>, profile: &str) -> Result<Self, BacktestRunStoreError> {
        Ok(Self::new(Arc::new(BacktestRunStore::open_existing(
            path, profile,
        )?)))
    }

    pub fn new(run_store: Arc<BacktestRunStore>) -> Self {
        Self { run_store }
    }

    pub fn create(&self, task: StoredBacktestSyncTask) -> Result<(), BacktestRunStoreError> {
        self.run_store.create_sync_task(&task)
    }

    pub fn get(
        &self,
        task_id: &str,
    ) -> Result<Option<StoredBacktestSyncTask>, BacktestRunStoreError> {
        self.run_store.get_sync_task(task_id)
    }

    /// Load tasks that were active when the previous process exited.  The
    /// production engine uses this at startup to make interrupted work
    /// terminal before exposing progress reads; no stale queued/running row
    /// is allowed to survive a restart as if a worker still owned it.
    pub fn list_active(&self) -> Result<Vec<StoredBacktestSyncTask>, BacktestRunStoreError> {
        self.run_store.list_active_sync_tasks()
    }

    /// List the durable sync-task history in deterministic newest-first order.
    /// The production storage overview uses this projection instead of an
    /// in-memory task registry, so completed and recovered jobs survive a
    /// process restart.
    pub fn list_all(&self) -> Result<Vec<StoredBacktestSyncTask>, BacktestRunStoreError> {
        self.run_store.list_sync_tasks()
    }

    pub fn cancel(
        &self,
        task_id: &str,
        timestamp: &str,
    ) -> Result<CancelBacktestSyncResult, BacktestRunStoreError> {
        self.run_store.cancel_sync_task(task_id, timestamp)
    }

    pub fn update(
        &self,
        task: StoredBacktestSyncTask,
        expected_revision: i64,
    ) -> Result<bool, BacktestRunStoreError> {
        self.run_store.update_sync_task(&task, expected_revision)
    }
}

impl BacktestRunStore {
    fn list_sync_tasks(&self) -> Result<Vec<StoredBacktestSyncTask>, BacktestRunStoreError> {
        let connection = self.lock()?;
        let mut statement = connection
            .prepare(&format!(
                "SELECT task_id, status, symbol, market_data_provider,
                total_intervals, completed_intervals, total_batches,
                completed_batches, current_interval, retries, error,
                started_at, updated_at, revision
                FROM {SYNC_TASKS_TABLE} ORDER BY updated_at DESC, task_id ASC"
            ))
            .map_err(BacktestRunStoreError::Query)?;
        let rows = statement
            .query_map([], |row| {
                Ok(StoredBacktestSyncTask {
                    task_id: row.get(0)?,
                    status: row.get(1)?,
                    symbol: row.get(2)?,
                    market_data_provider: row.get(3)?,
                    total_intervals: row.get(4)?,
                    completed_intervals: row.get(5)?,
                    total_batches: row.get(6)?,
                    completed_batches: row.get(7)?,
                    current_interval: row.get(8)?,
                    retries: row.get(9)?,
                    error: row.get(10)?,
                    started_at: row.get(11)?,
                    updated_at: row.get(12)?,
                    revision: row.get(13)?,
                })
            })
            .map_err(BacktestRunStoreError::Query)?;
        let tasks = rows
            .collect::<Result<Vec<_>, _>>()
            .map_err(BacktestRunStoreError::Query)?;
        for task in &tasks {
            validate_sync_task(task)?;
        }
        Ok(tasks)
    }

    fn list_active_sync_tasks(&self) -> Result<Vec<StoredBacktestSyncTask>, BacktestRunStoreError> {
        let connection = self.lock()?;
        let mut statement = connection
            .prepare(&format!(
                "SELECT task_id, status, symbol, market_data_provider,
                total_intervals, completed_intervals, total_batches,
                completed_batches, current_interval, retries, error,
                started_at, updated_at, revision
                FROM {SYNC_TASKS_TABLE} WHERE status IN ('queued', 'running')
                ORDER BY updated_at ASC, task_id ASC"
            ))
            .map_err(BacktestRunStoreError::Query)?;
        let rows = statement
            .query_map([], |row| {
                Ok(StoredBacktestSyncTask {
                    task_id: row.get(0)?,
                    status: row.get(1)?,
                    symbol: row.get(2)?,
                    market_data_provider: row.get(3)?,
                    total_intervals: row.get(4)?,
                    completed_intervals: row.get(5)?,
                    total_batches: row.get(6)?,
                    completed_batches: row.get(7)?,
                    current_interval: row.get(8)?,
                    retries: row.get(9)?,
                    error: row.get(10)?,
                    started_at: row.get(11)?,
                    updated_at: row.get(12)?,
                    revision: row.get(13)?,
                })
            })
            .map_err(BacktestRunStoreError::Query)?;
        let tasks = rows
            .collect::<Result<Vec<_>, _>>()
            .map_err(BacktestRunStoreError::Query)?;
        for task in &tasks {
            validate_sync_task(task)?;
        }
        Ok(tasks)
    }
}

pub(crate) fn ensure_sync_task_schema(
    connection: &mut Connection,
    path: &str,
) -> Result<(), BacktestRunStoreError> {
    let transaction = connection
        .transaction_with_behavior(TransactionBehavior::Immediate)
        .map_err(BacktestRunStoreError::Configure)?;
    let table_exists: bool = transaction
        .query_row(
            "SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?1)",
            [SYNC_TASKS_TABLE],
            |row| row.get(0),
        )
        .map_err(BacktestRunStoreError::Configure)?;
    if !table_exists {
        transaction
            .execute_batch(SYNC_TASKS_DDL)
            .map_err(BacktestRunStoreError::Configure)?;
        transaction
            .commit()
            .map_err(BacktestRunStoreError::Configure)?;
        return Ok(());
    }

    let mut statement = transaction
        .prepare(&format!("PRAGMA table_info(\"{SYNC_TASKS_TABLE}\")"))
        .map_err(BacktestRunStoreError::Configure)?;
    let columns = statement
        .query_map([], |row| {
            Ok((
                row.get::<_, String>(1)?,
                row.get::<_, String>(2)?,
                row.get::<_, i64>(3)?,
                row.get::<_, i64>(5)?,
            ))
        })
        .map_err(BacktestRunStoreError::Configure)?
        .collect::<Result<Vec<_>, _>>()
        .map_err(BacktestRunStoreError::Configure)?;
    drop(statement);
    let expected = vec![
        ("task_id".to_owned(), "TEXT".to_owned(), 0, 1),
        ("status".to_owned(), "TEXT".to_owned(), 1, 0),
        ("symbol".to_owned(), "TEXT".to_owned(), 1, 0),
        ("market_data_provider".to_owned(), "TEXT".to_owned(), 1, 0),
        ("total_intervals".to_owned(), "INTEGER".to_owned(), 1, 0),
        ("completed_intervals".to_owned(), "INTEGER".to_owned(), 1, 0),
        ("total_batches".to_owned(), "INTEGER".to_owned(), 1, 0),
        ("completed_batches".to_owned(), "INTEGER".to_owned(), 1, 0),
        ("current_interval".to_owned(), "TEXT".to_owned(), 1, 0),
        ("retries".to_owned(), "INTEGER".to_owned(), 1, 0),
        ("error".to_owned(), "TEXT".to_owned(), 0, 0),
        ("started_at".to_owned(), "TEXT".to_owned(), 1, 0),
        ("updated_at".to_owned(), "TEXT".to_owned(), 1, 0),
        ("revision".to_owned(), "INTEGER".to_owned(), 1, 0),
    ];
    if columns != expected {
        return Err(BacktestRunStoreError::Incompatible(format!(
            "internal sync-task table structure is incompatible at {path}"
        )));
    }
    let index_sql: Option<String> = transaction
        .query_row(
            "SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?1",
            [SYNC_TASKS_INDEX],
            |row| row.get(0),
        )
        .optional()
        .map_err(BacktestRunStoreError::Configure)?;
    let normalized_index = index_sql.as_deref().map(normalize_sql).unwrap_or_default();
    if normalized_index.is_empty() {
        transaction
            .execute_batch(
                "CREATE INDEX idx_backtest_sync_tasks_updated_at
                 ON jftrade_internal__backtest_sync_tasks (updated_at DESC, task_id ASC);",
            )
            .map_err(BacktestRunStoreError::Configure)?;
    } else if normalized_index
        != "create index idx_backtest_sync_tasks_updated_at on jftrade_internal__backtest_sync_tasks (updated_at desc, task_id asc)"
    {
        return Err(BacktestRunStoreError::Incompatible(format!(
            "internal sync-task index structure is incompatible at {path}"
        )));
    }
    transaction
        .commit()
        .map_err(BacktestRunStoreError::Configure)?;
    Ok(())
}

fn normalize_sql(sql: &str) -> String {
    sql.split_whitespace()
        .collect::<Vec<_>>()
        .join(" ")
        .to_ascii_lowercase()
}

fn validate_sync_task(task: &StoredBacktestSyncTask) -> Result<(), BacktestRunStoreError> {
    if task.task_id.trim().is_empty() || task.status.trim().is_empty() {
        return Err(BacktestRunStoreError::Validation(
            "sync task id and status are required".to_owned(),
        ));
    }
    if !matches!(
        task.status.as_str(),
        "queued" | "running" | "completed" | "failed" | "cancelled"
    ) {
        return Err(BacktestRunStoreError::Validation(format!(
            "invalid sync task status: {}",
            task.status
        )));
    }
    for (name, value) in [
        ("totalIntervals", task.total_intervals),
        ("completedIntervals", task.completed_intervals),
        ("totalBatches", task.total_batches),
        ("completedBatches", task.completed_batches),
        ("retries", task.retries),
        ("revision", task.revision),
    ] {
        if value < 0 {
            return Err(BacktestRunStoreError::Validation(format!(
                "sync task {name} cannot be negative"
            )));
        }
    }
    validate_rfc3339_timestamp(&task.started_at)?;
    validate_rfc3339_timestamp(&task.updated_at)
}
