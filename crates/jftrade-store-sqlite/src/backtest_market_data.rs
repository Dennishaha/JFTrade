//! Production ownership for the backtest market-data database.
//!
//! The backtest run database and the market-data database are separate
//! resources in the public runtime layout. Keeping the lease and schema
//! validation here prevents a run worker from opening the K-line file outside
//! the Rust ownership boundary.

use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::Duration;

use jftrade_owner_lock::{OwnerDiagnostic, WriterLease, WriterLeaseError};
use rusqlite::{Connection, OpenFlags};
use thiserror::Error;

use crate::schema_manifest::{SchemaManifestError, validate_current};

const BACKTEST_COMPONENT: &str = "backtest";
const BACKTEST_SCHEMA_VERSION: i64 = 3;

pub const BACKTEST_MARKET_DATA_TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";
pub const BACKTEST_MARKET_DATA_PRODUCTION_PROFILE: &str = "production.v1";

#[derive(Debug, Error)]
pub enum BacktestMarketDataStoreError {
    #[error("backtest market-data database path is required")]
    EmptyPath,
    #[error("unsupported backtest market-data writer profile: {0}")]
    UnsupportedProfile(String),
    #[error("backtest market-data database is not an existing regular file: {0}")]
    NotRegularFile(String),
    #[error(transparent)]
    WriterLease(#[from] WriterLeaseError),
    #[error("open backtest market-data database: {0}")]
    Open(#[source] rusqlite::Error),
    #[error("configure backtest market-data database: {0}")]
    Configure(#[source] rusqlite::Error),
    #[error(transparent)]
    Schema(#[from] SchemaManifestError),
    #[error("backtest market-data database lock is unavailable")]
    LockUnavailable,
    #[error("query backtest market-data database: {0}")]
    Query(#[source] rusqlite::Error),
}

/// A single leased connection for the dynamic K-line tables.
pub struct BacktestMarketDataStore {
    path: PathBuf,
    connection: Mutex<Connection>,
    _writer_lease: WriterLease,
}

impl std::fmt::Debug for BacktestMarketDataStore {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("BacktestMarketDataStore")
            .field("path", &self.path)
            .finish()
    }
}

impl BacktestMarketDataStore {
    pub fn open(path: impl AsRef<Path>) -> Result<Self, BacktestMarketDataStoreError> {
        Self::open_existing(path, BACKTEST_MARKET_DATA_PRODUCTION_PROFILE)
    }

    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, BacktestMarketDataStoreError> {
        let path = path.as_ref();
        if path.as_os_str().is_empty() {
            return Err(BacktestMarketDataStoreError::EmptyPath);
        }
        if profile != BACKTEST_MARKET_DATA_TEST_CUTOVER_PROFILE
            && profile != BACKTEST_MARKET_DATA_PRODUCTION_PROFILE
        {
            return Err(BacktestMarketDataStoreError::UnsupportedProfile(
                profile.to_owned(),
            ));
        }
        if !path
            .metadata()
            .map(|metadata| metadata.is_file())
            .unwrap_or(false)
        {
            return Err(BacktestMarketDataStoreError::NotRegularFile(
                path.display().to_string(),
            ));
        }

        let writer_lease = WriterLease::acquire(path, &OwnerDiagnostic::current("rust", profile))?;
        let connection = Connection::open_with_flags(
            path,
            OpenFlags::SQLITE_OPEN_READ_WRITE | OpenFlags::SQLITE_OPEN_NO_MUTEX,
        )
        .map_err(BacktestMarketDataStoreError::Open)?;
        connection
            .busy_timeout(Duration::from_secs(10))
            .map_err(BacktestMarketDataStoreError::Configure)?;
        connection
            .execute_batch("PRAGMA foreign_keys = ON;")
            .map_err(BacktestMarketDataStoreError::Configure)?;
        validate_current(
            &connection,
            &path.display().to_string(),
            BACKTEST_COMPONENT,
            BACKTEST_SCHEMA_VERSION,
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

    /// Return the dynamic K-line tables currently present in the database.
    /// Names are read from SQLite, so a fresh database is represented as an
    /// empty catalog once the manifest prototype is filtered out by callers.
    pub fn kline_tables(&self) -> Result<Vec<String>, BacktestMarketDataStoreError> {
        let connection = self
            .connection
            .lock()
            .map_err(|_| BacktestMarketDataStoreError::LockUnavailable)?;
        let mut statement = connection
            .prepare(
                "SELECT name FROM sqlite_master
                 WHERE type = 'table' AND name LIKE 'local_klines__%'
                 ORDER BY name ASC",
            )
            .map_err(BacktestMarketDataStoreError::Query)?;
        let rows = statement
            .query_map([], |row| row.get::<_, String>(0))
            .map_err(BacktestMarketDataStoreError::Query)?;
        rows.collect::<Result<Vec<_>, _>>()
            .map_err(BacktestMarketDataStoreError::Query)
    }

    pub fn kline_table_count(&self) -> Result<usize, BacktestMarketDataStoreError> {
        Ok(self.kline_tables()?.len())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::initialize_current;
    use tempfile::tempdir;

    #[test]
    fn production_store_validates_dynamic_schema_and_reports_manifest_table() {
        let directory = tempdir().expect("temporary directory");
        let path = directory.path().join("backtest.db");
        let connection = Connection::open(&path).expect("create database");
        initialize_current(&connection, BACKTEST_COMPONENT).expect("initialize schema");
        drop(connection);

        let store = BacktestMarketDataStore::open(&path).expect("open production store");
        assert_eq!(
            store.kline_tables().expect("read tables"),
            vec!["local_klines__manifest__symbol__1m__forward__r__00000000"]
        );
        assert_eq!(store.kline_table_count().expect("count tables"), 1);
    }

    #[test]
    fn production_store_rejects_a_second_writer_lease() {
        let directory = tempdir().expect("temporary directory");
        let path = directory.path().join("backtest.db");
        let connection = Connection::open(&path).expect("create database");
        initialize_current(&connection, BACKTEST_COMPONENT).expect("initialize schema");
        drop(connection);

        let first = BacktestMarketDataStore::open(&path).expect("first store");
        let second = BacktestMarketDataStore::open(&path);
        assert!(matches!(
            second,
            Err(BacktestMarketDataStoreError::WriterLease(
                WriterLeaseError::Held { .. }
            ))
        ));
        drop(first);
    }
}
