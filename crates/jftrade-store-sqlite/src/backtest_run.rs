use std::path::{Path, PathBuf};
use std::sync::{Mutex, MutexGuard};
use std::time::Duration;

use jftrade_owner_lock::{OwnerDiagnostic, WriterLease, WriterLeaseError};
use rusqlite::{Connection, OpenFlags, OptionalExtension, TransactionBehavior, params};
use serde::{Deserialize, Serialize};
use thiserror::Error;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use crate::schema_manifest::{SchemaManifestError, validate_current};

const BACKTEST_RUNS_COMPONENT: &str = "backtest-runs";
const BACKTEST_RUNS_SCHEMA_VERSION: i64 = 1;
pub const BACKTEST_RUNS_TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredBacktestRun {
    pub id: String,
    pub status: String,
    pub request_json: String,
    pub result_json: String,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Debug, Error)]
pub enum BacktestRunStoreError {
    #[error("backtest runs database path is required")]
    EmptyPath,
    #[error("unsupported backtest runs writer profile: {0}")]
    UnsupportedProfile(String),
    #[error("backtest runs database is not an existing regular file: {0}")]
    NotRegularFile(String),
    #[error(transparent)]
    WriterLease(#[from] WriterLeaseError),
    #[error("open backtest runs database: {0}")]
    Open(#[source] rusqlite::Error),
    #[error("configure backtest runs database: {0}")]
    Configure(#[source] rusqlite::Error),
    #[error(transparent)]
    Schema(#[from] SchemaManifestError),
    #[error("backtest runs database lock is unavailable")]
    LockUnavailable,
    #[error("query backtest runs database: {0}")]
    Query(#[source] rusqlite::Error),
    #[error("backtest run not found: {0}")]
    NotFound(String),
    #[error("backtest run is not terminal: {0}")]
    NotTerminal(String),
    #[error("invalid backtest runs request: {0}")]
    Validation(String),
    #[error("incompatible backtest runs database: {0}")]
    Incompatible(String),
}

pub struct BacktestRunTestCutoverStore {
    path: PathBuf,
    connection: Mutex<Connection>,
    _writer_lease: WriterLease,
}

impl std::fmt::Debug for BacktestRunTestCutoverStore {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("BacktestRunTestCutoverStore")
            .field("path", &self.path)
            .finish()
    }
}

impl BacktestRunTestCutoverStore {
    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, BacktestRunStoreError> {
        let path = path.as_ref();
        if path.as_os_str().is_empty() {
            return Err(BacktestRunStoreError::EmptyPath);
        }
        if profile != BACKTEST_RUNS_TEST_CUTOVER_PROFILE {
            return Err(BacktestRunStoreError::UnsupportedProfile(
                profile.to_owned(),
            ));
        }
        if !path
            .metadata()
            .map(|metadata| metadata.is_file())
            .unwrap_or(false)
        {
            return Err(BacktestRunStoreError::NotRegularFile(
                path.display().to_string(),
            ));
        }

        let writer_lease = WriterLease::acquire(
            path,
            &OwnerDiagnostic::current("rust", BACKTEST_RUNS_TEST_CUTOVER_PROFILE),
        )?;

        let connection = Connection::open_with_flags(
            path,
            OpenFlags::SQLITE_OPEN_READ_WRITE | OpenFlags::SQLITE_OPEN_NO_MUTEX,
        )
        .map_err(BacktestRunStoreError::Open)?;

        connection
            .busy_timeout(Duration::from_secs(10))
            .map_err(BacktestRunStoreError::Configure)?;

        connection
            .execute_batch("PRAGMA foreign_keys = ON;")
            .map_err(BacktestRunStoreError::Configure)?;

        validate_current(
            &connection,
            &path.display().to_string(),
            BACKTEST_RUNS_COMPONENT,
            BACKTEST_RUNS_SCHEMA_VERSION,
        )?;

        Ok(Self {
            path: path.to_path_buf(),
            connection: Mutex::new(connection),
            _writer_lease: writer_lease,
        })
    }

    pub fn path(&self) -> &Path {
        &self.path
    }

    fn lock(&self) -> Result<MutexGuard<'_, Connection>, BacktestRunStoreError> {
        self.connection
            .lock()
            .map_err(|_| BacktestRunStoreError::LockUnavailable)
    }

    pub fn save_run(
        &self,
        run: StoredBacktestRun,
        timestamp: &str,
    ) -> Result<StoredBacktestRun, BacktestRunStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(BacktestRunStoreError::Query)?;

        let created_at = if run.created_at.is_empty() {
            timestamp.to_owned()
        } else {
            run.created_at.clone()
        };

        transaction
            .execute(
                "INSERT INTO backtest_runs (id, status, request_json, result_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6)
                 ON CONFLICT(id) DO UPDATE SET
                    status = excluded.status,
                    request_json = excluded.request_json,
                    result_json = excluded.result_json,
                    updated_at = excluded.updated_at",
                params![run.id, run.status, run.request_json, run.result_json, created_at, timestamp],
            )
            .map_err(BacktestRunStoreError::Query)?;

        transaction.commit().map_err(BacktestRunStoreError::Query)?;
        Ok(StoredBacktestRun {
            id: run.id,
            status: run.status,
            request_json: run.request_json,
            result_json: run.result_json,
            created_at,
            updated_at: timestamp.to_owned(),
        })
    }

    pub fn get_run(&self, id: &str) -> Result<Option<StoredBacktestRun>, BacktestRunStoreError> {
        let connection = self.lock()?;
        let row = connection
            .query_row(
                "SELECT id, status, request_json, result_json, created_at, updated_at
                 FROM backtest_runs WHERE id = ?1",
                params![id],
                |row| {
                    Ok(StoredBacktestRun {
                        id: row.get(0)?,
                        status: row.get(1)?,
                        request_json: row.get(2)?,
                        result_json: row.get(3)?,
                        created_at: row.get(4)?,
                        updated_at: row.get(5)?,
                    })
                },
            )
            .optional()
            .map_err(BacktestRunStoreError::Query)?;
        Ok(row)
    }

    pub fn run_count(&self) -> Result<u64, BacktestRunStoreError> {
        let connection = self.lock()?;
        let count: i64 = connection
            .query_row("SELECT COUNT(*) FROM backtest_runs", [], |row| row.get(0))
            .map_err(BacktestRunStoreError::Query)?;
        Ok(count as u64)
    }

    pub fn delete_run(&self, id: &str) -> Result<bool, BacktestRunStoreError> {
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(BacktestRunStoreError::Query)?;

        let run: Option<(String,)> = transaction
            .query_row(
                "SELECT status FROM backtest_runs WHERE id = ?1",
                params![id],
                |row| Ok((row.get(0)?,)),
            )
            .optional()
            .map_err(BacktestRunStoreError::Query)?;

        let Some((status,)) = run else {
            return Ok(false);
        };

        if status == "running" || status == "queued" {
            return Err(BacktestRunStoreError::NotTerminal(id.to_owned()));
        }

        transaction
            .execute("DELETE FROM backtest_runs WHERE id = ?1", params![id])
            .map_err(BacktestRunStoreError::Query)?;

        transaction.commit().map_err(BacktestRunStoreError::Query)?;
        Ok(true)
    }
}

fn validate_rfc3339_timestamp(timestamp: &str) -> Result<(), BacktestRunStoreError> {
    OffsetDateTime::parse(timestamp, &Rfc3339)
        .map(|_| ())
        .map_err(|error| {
            BacktestRunStoreError::Incompatible(format!(
                "invalid RFC3339 timestamp {timestamp:?}: {error}"
            ))
        })
}
