//! Production ownership for the backtest market-data database.
//!
//! The backtest run database and the market-data database are separate
//! resources in the public runtime layout. Keeping the lease and schema
//! validation here prevents a run worker from opening the K-line file outside
//! the Rust ownership boundary.

use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::Duration;

use jftrade_kernel::Fixed8;
use jftrade_owner_lock::{OwnerDiagnostic, WriterLease, WriterLeaseError};
use rusqlite::{Connection, OpenFlags};
use thiserror::Error;

use crate::schema_manifest::{SchemaManifestError, validate_current};

#[path = "backtest_market_data_aggregation.rs"]
mod aggregation;
use aggregation::*;

const BACKTEST_COMPONENT: &str = "backtest";
const BACKTEST_SCHEMA_VERSION: i64 = 3;

pub const BACKTEST_MARKET_DATA_TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";
pub const BACKTEST_MARKET_DATA_PRODUCTION_PROFILE: &str = "production.v1";

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StoredBacktestCandle {
    pub start_time: i64,
    pub end_time: i64,
    pub open: String,
    pub high: String,
    pub low: String,
    pub close: String,
    pub volume: String,
}

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
    #[error("backtest market-data coverage is unavailable: {0}")]
    Coverage(String),
    #[error("invalid backtest market-data candle: {0}")]
    Validation(String),
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

    /// Atomically create the provider/symbol/interval table and upsert a page
    /// of validated candles. Table names follow the Go KLineStore naming
    /// scheme, while all values remain the public Fixed8 text representation.
    pub fn insert_candles(
        &self,
        provider_id: &str,
        symbol: &str,
        interval: &str,
        rehab_type: &str,
        session_scope: &str,
        candles: &[StoredBacktestCandle],
    ) -> Result<usize, BacktestMarketDataStoreError> {
        let table = kline_table_name(provider_id, symbol, interval, rehab_type, session_scope)?;
        let mut connection = self
            .connection
            .lock()
            .map_err(|_| BacktestMarketDataStoreError::LockUnavailable)?;
        let transaction = connection
            .transaction_with_behavior(rusqlite::TransactionBehavior::Immediate)
            .map_err(BacktestMarketDataStoreError::Query)?;
        transaction
            .execute_batch(&format!(
                "CREATE TABLE IF NOT EXISTS \"{table}\" (
                    end_time INTEGER NOT NULL,
                    start_time INTEGER NOT NULL,
                    open TEXT NOT NULL,
                    high TEXT NOT NULL,
                    low TEXT NOT NULL,
                    close TEXT NOT NULL,
                    volume TEXT NOT NULL,
                    PRIMARY KEY (end_time)
                ) WITHOUT ROWID"
            ))
            .map_err(BacktestMarketDataStoreError::Query)?;
        validate_kline_table_schema(&transaction, &table)?;
        for candle in candles {
            let candle = canonical_candle(candle)?;
            transaction
                .execute(
                    &format!(
                        "INSERT INTO \"{table}\" (end_time, start_time, open, high, low, close, volume)
                         VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)
                         ON CONFLICT(end_time) DO UPDATE SET
                           start_time = excluded.start_time,
                           open = excluded.open,
                           high = excluded.high,
                           low = excluded.low,
                           close = excluded.close,
                           volume = excluded.volume"
                    ),
                    rusqlite::params![
                        candle.end_time,
                        candle.start_time,
                        candle.open,
                        candle.high,
                        candle.low,
                        candle.close,
                        candle.volume,
                    ],
                )
                .map_err(BacktestMarketDataStoreError::Query)?;
        }
        transaction
            .commit()
            .map_err(BacktestMarketDataStoreError::Query)?;
        Ok(candles.len())
    }

    /// Read candles for a single instrument/interval in ascending start-time
    /// order.  The dynamic table name is derived by the same canonicalizer as
    /// `insert_candles`, preventing a caller from injecting SQL identifiers.
    #[allow(clippy::too_many_arguments)]
    pub fn read_candles(
        &self,
        provider_id: &str,
        symbol: &str,
        interval: &str,
        rehab_type: &str,
        session_scope: &str,
        start_time_ms: i64,
        end_time_ms: i64,
    ) -> Result<Vec<StoredBacktestCandle>, BacktestMarketDataStoreError> {
        if end_time_ms <= start_time_ms {
            return Err(BacktestMarketDataStoreError::Validation(
                "candle query end_time must be after start_time".to_owned(),
            ));
        }
        let table = kline_table_name(provider_id, symbol, interval, rehab_type, session_scope)?;
        let connection = self
            .connection
            .lock()
            .map_err(|_| BacktestMarketDataStoreError::LockUnavailable)?;
        if table_exists(&connection, &table)? {
            let candles = read_direct_range(&connection, &table, start_time_ms, end_time_ms)?;
            if !candles.is_empty() || !is_aggregate_interval(interval) {
                return Ok(candles);
            }
        } else if !is_aggregate_interval(interval) {
            // Preserve the existing direct-read error for a missing table.
            return read_direct_range(&connection, &table, start_time_ms, end_time_ms);
        }
        let candidates = aggregation_candidate_intervals(interval);
        let mut last_coverage_error = None;
        for (cand_interval, cand_min) in &candidates {
            let cand_table = kline_table_name(
                provider_id,
                symbol,
                cand_interval,
                rehab_type,
                session_scope,
            )?;
            if table_exists(&connection, &cand_table)? {
                match aggregate_range(
                    &connection,
                    &cand_table,
                    *cand_min,
                    symbol,
                    interval,
                    start_time_ms,
                    end_time_ms,
                ) {
                    Ok(candles) if !candles.is_empty() => return Ok(candles),
                    Ok(_) => {}
                    Err(BacktestMarketDataStoreError::Coverage(err)) => {
                        last_coverage_error = Some(BacktestMarketDataStoreError::Coverage(err));
                    }
                    Err(other_err) => {
                        return Err(other_err);
                    }
                }
            }
        }

        if let Some(err) = last_coverage_error {
            return Err(err);
        }

        let fallback_table =
            kline_table_name(provider_id, symbol, "1m", rehab_type, session_scope)?;
        aggregate_range(
            &connection,
            &fallback_table,
            1,
            symbol,
            interval,
            start_time_ms,
            end_time_ms,
        )
    }

    /// Read a bounded range with deterministic ordering and an optional limit.
    /// Direct target-interval rows win; an empty/missing 5m or 15m target is
    /// synthesized from complete 1m or 5m coverage.
    #[allow(clippy::too_many_arguments)]
    pub fn query_candles(
        &self,
        provider_id: &str,
        symbol: &str,
        interval: &str,
        rehab_type: &str,
        session_scope: &str,
        start_time_ms: i64,
        end_time_ms: i64,
        order: &str,
        limit: usize,
    ) -> Result<Vec<StoredBacktestCandle>, BacktestMarketDataStoreError> {
        let is_desc = match order.trim().to_ascii_uppercase().as_str() {
            "ASC" => false,
            "DESC" => true,
            _ => {
                return Err(BacktestMarketDataStoreError::Validation(
                    "candle query order must be ASC or DESC".to_owned(),
                ));
            }
        };

        let table = kline_table_name(provider_id, symbol, interval, rehab_type, session_scope)?;
        let connection = self
            .connection
            .lock()
            .map_err(|_| BacktestMarketDataStoreError::LockUnavailable)?;
        if table_exists(&connection, &table)? {
            if is_desc {
                let candles = read_direct_desc_limit(
                    &connection,
                    &table,
                    start_time_ms,
                    end_time_ms,
                    normalize_limit(limit),
                )?;
                if !candles.is_empty() || !is_aggregate_interval(interval) {
                    return Ok(candles);
                }
            } else {
                let candles = read_direct_range(&connection, &table, start_time_ms, end_time_ms)?;
                if !candles.is_empty() || !is_aggregate_interval(interval) {
                    let mut res = candles;
                    res.truncate(normalize_limit(limit));
                    return Ok(res);
                }
            }
        }
        drop(connection);

        let mut candles = self.read_candles(
            provider_id,
            symbol,
            interval,
            rehab_type,
            session_scope,
            start_time_ms,
            end_time_ms,
        )?;
        if is_desc {
            candles.reverse();
        }
        candles.truncate(normalize_limit(limit));
        Ok(candles)
    }

    #[allow(clippy::too_many_arguments)]
    pub fn query_candles_forward(
        &self,
        provider_id: &str,
        symbol: &str,
        interval: &str,
        rehab_type: &str,
        session_scope: &str,
        start_time_ms: i64,
        limit: usize,
    ) -> Result<Vec<StoredBacktestCandle>, BacktestMarketDataStoreError> {
        let duration = interval_duration_ms(interval).unwrap_or(1);
        let first = floor_div(start_time_ms, duration).saturating_mul(duration);
        let end = first.saturating_add(duration.saturating_mul(normalize_limit(limit) as i64));
        self.query_candles(
            provider_id,
            symbol,
            interval,
            rehab_type,
            session_scope,
            start_time_ms,
            end,
            "ASC",
            limit,
        )
    }

    #[allow(clippy::too_many_arguments)]
    pub fn query_candles_backward(
        &self,
        provider_id: &str,
        symbol: &str,
        interval: &str,
        rehab_type: &str,
        session_scope: &str,
        end_time_ms: i64,
        limit: usize,
    ) -> Result<Vec<StoredBacktestCandle>, BacktestMarketDataStoreError> {
        let count = normalize_limit(limit) as i64;
        let duration = interval_duration_ms(interval).unwrap_or(1);
        let start = end_time_ms.saturating_sub(duration.saturating_mul(count));
        self.query_candles(
            provider_id,
            symbol,
            interval,
            rehab_type,
            session_scope,
            start,
            end_time_ms,
            "ASC",
            limit,
        )
    }
}

fn table_exists(
    connection: &Connection,
    table: &str,
) -> Result<bool, BacktestMarketDataStoreError> {
    connection
        .query_row(
            "SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?1)",
            [table],
            |row| row.get::<_, i64>(0),
        )
        .map(|value| value != 0)
        .map_err(BacktestMarketDataStoreError::Query)
}

fn read_direct_range(
    connection: &Connection,
    table: &str,
    start_time_ms: i64,
    end_time_ms: i64,
) -> Result<Vec<StoredBacktestCandle>, BacktestMarketDataStoreError> {
    let mut statement = connection
        .prepare(&format!(
            "SELECT start_time, end_time, open, high, low, close, volume
             FROM \"{table}\" WHERE start_time >= ?1 AND start_time < ?2
             ORDER BY start_time ASC, end_time ASC"
        ))
        .map_err(BacktestMarketDataStoreError::Query)?;
    let rows = statement
        .query_map(rusqlite::params![start_time_ms, end_time_ms], |row| {
            Ok(StoredBacktestCandle {
                start_time: row.get(0)?,
                end_time: row.get(1)?,
                open: row.get(2)?,
                high: row.get(3)?,
                low: row.get(4)?,
                close: row.get(5)?,
                volume: row.get(6)?,
            })
        })
        .map_err(BacktestMarketDataStoreError::Query)?;
    let candles = rows
        .collect::<Result<Vec<_>, _>>()
        .map_err(BacktestMarketDataStoreError::Query)?;
    for candle in &candles {
        validate_candle(candle)?;
    }
    Ok(candles)
}

fn read_direct_desc_limit(
    connection: &Connection,
    table: &str,
    start_time_ms: i64,
    end_time_ms: i64,
    limit: usize,
) -> Result<Vec<StoredBacktestCandle>, BacktestMarketDataStoreError> {
    let mut statement = connection
        .prepare(&format!(
            "SELECT start_time, end_time, open, high, low, close, volume
             FROM \"{table}\" WHERE start_time >= ?1 AND start_time < ?2
             ORDER BY start_time DESC, end_time DESC LIMIT ?3"
        ))
        .map_err(BacktestMarketDataStoreError::Query)?;
    let rows = statement
        .query_map(
            rusqlite::params![start_time_ms, end_time_ms, limit as i64],
            |row| {
                Ok(StoredBacktestCandle {
                    start_time: row.get(0)?,
                    end_time: row.get(1)?,
                    open: row.get(2)?,
                    high: row.get(3)?,
                    low: row.get(4)?,
                    close: row.get(5)?,
                    volume: row.get(6)?,
                })
            },
        )
        .map_err(BacktestMarketDataStoreError::Query)?;
    let candles = rows
        .collect::<Result<Vec<_>, _>>()
        .map_err(BacktestMarketDataStoreError::Query)?;
    for candle in &candles {
        validate_candle(candle)?;
    }
    Ok(candles)
}

fn validate_kline_table_schema(
    connection: &rusqlite::Transaction<'_>,
    table: &str,
) -> Result<(), BacktestMarketDataStoreError> {
    let mut statement = connection
        .prepare(&format!("PRAGMA table_info(\"{table}\")"))
        .map_err(BacktestMarketDataStoreError::Query)?;
    let columns = statement
        .query_map([], |row| {
            Ok((
                row.get::<_, String>(1)?,
                row.get::<_, String>(2)?,
                row.get::<_, i64>(3)?,
                row.get::<_, i64>(5)?,
            ))
        })
        .map_err(BacktestMarketDataStoreError::Query)?
        .collect::<Result<Vec<_>, _>>()
        .map_err(BacktestMarketDataStoreError::Query)?;
    let expected = vec![
        ("end_time".to_owned(), "INTEGER".to_owned(), 1, 1),
        ("start_time".to_owned(), "INTEGER".to_owned(), 1, 0),
        ("open".to_owned(), "TEXT".to_owned(), 1, 0),
        ("high".to_owned(), "TEXT".to_owned(), 1, 0),
        ("low".to_owned(), "TEXT".to_owned(), 1, 0),
        ("close".to_owned(), "TEXT".to_owned(), 1, 0),
        ("volume".to_owned(), "TEXT".to_owned(), 1, 0),
    ];
    if columns != expected {
        return Err(BacktestMarketDataStoreError::Validation(
            "dynamic K-line table structure does not match production schema".to_owned(),
        ));
    }
    let sql: String = connection
        .query_row(
            "SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?1",
            [table],
            |row| row.get(0),
        )
        .map_err(BacktestMarketDataStoreError::Query)?;
    let normalized = sql
        .split_whitespace()
        .collect::<Vec<_>>()
        .join(" ")
        .to_ascii_lowercase();
    if !normalized.ends_with("without rowid") || !normalized.contains("primary key (end_time)") {
        return Err(BacktestMarketDataStoreError::Validation(
            "dynamic K-line table must use WITHOUT ROWID and end_time primary key".to_owned(),
        ));
    }
    Ok(())
}

fn validate_candle(candle: &StoredBacktestCandle) -> Result<(), BacktestMarketDataStoreError> {
    if candle.end_time <= candle.start_time {
        return Err(BacktestMarketDataStoreError::Validation(
            "candle end_time must be after start_time".to_owned(),
        ));
    }
    let parse = |name: &str, value: &str| {
        value
            .parse::<Fixed8>()
            .map_err(|error| BacktestMarketDataStoreError::Validation(format!("{name}: {error}")))
    };
    let open = parse("open", &candle.open)?;
    let high = parse("high", &candle.high)?;
    let low = parse("low", &candle.low)?;
    let close = parse("close", &candle.close)?;
    let volume = parse("volume", &candle.volume)?;
    if open <= Fixed8::ZERO || high <= Fixed8::ZERO || low <= Fixed8::ZERO || close <= Fixed8::ZERO
    {
        return Err(BacktestMarketDataStoreError::Validation(
            "OHLC values must be positive".to_owned(),
        ));
    }
    if high < low || open < low || open > high || close < low || close > high {
        return Err(BacktestMarketDataStoreError::Validation(
            "OHLC values are inconsistent".to_owned(),
        ));
    }
    if volume < Fixed8::ZERO {
        return Err(BacktestMarketDataStoreError::Validation(
            "volume cannot be negative".to_owned(),
        ));
    }
    Ok(())
}

fn canonical_candle(
    candle: &StoredBacktestCandle,
) -> Result<StoredBacktestCandle, BacktestMarketDataStoreError> {
    validate_candle(candle)?;
    let canonical = |value: &str| {
        value
            .parse::<Fixed8>()
            .map(|parsed| parsed.storage_text())
            .map_err(|error| BacktestMarketDataStoreError::Validation(error.to_string()))
    };
    Ok(StoredBacktestCandle {
        start_time: candle.start_time,
        end_time: candle.end_time,
        open: canonical(&candle.open)?,
        high: canonical(&candle.high)?,
        low: canonical(&candle.low)?,
        close: canonical(&candle.close)?,
        volume: canonical(&candle.volume)?,
    })
}

fn kline_table_name(
    provider_id: &str,
    symbol: &str,
    interval: &str,
    rehab_type: &str,
    session_scope: &str,
) -> Result<String, BacktestMarketDataStoreError> {
    let raw_provider = provider_id.trim().to_ascii_lowercase();
    if !matches!(raw_provider.as_str(), "futu" | "yfinance" | "akshare") {
        return Err(BacktestMarketDataStoreError::Validation(format!(
            "unsupported market-data provider: {provider_id}"
        )));
    }
    let raw_symbol = symbol.trim().to_ascii_lowercase();
    if raw_symbol.is_empty() {
        return Err(BacktestMarketDataStoreError::Validation(
            "symbol is required".to_owned(),
        ));
    }
    let raw_interval = canonical_interval(interval);
    if raw_interval.is_empty() {
        return Err(BacktestMarketDataStoreError::Validation(
            "interval is required".to_owned(),
        ));
    }
    let provider = normalize_component(&raw_provider, "");
    let symbol = normalize_component(&raw_symbol, "");
    let interval = normalize_component(&raw_interval, "");
    if symbol.is_empty() {
        return Err(BacktestMarketDataStoreError::Validation(
            "symbol is required".to_owned(),
        ));
    }
    if interval.is_empty() {
        return Err(BacktestMarketDataStoreError::Validation(
            "interval is required".to_owned(),
        ));
    }
    let rehab = match rehab_type.trim().to_ascii_lowercase().as_str() {
        "forward" | "backward" | "none" => rehab_type.trim().to_ascii_lowercase(),
        _ => {
            return Err(BacktestMarketDataStoreError::Validation(
                "invalid rehab type".to_owned(),
            ));
        }
    };
    let scope_tag = match session_scope.trim().to_ascii_lowercase().as_str() {
        "regular" => "r",
        "extended" => "x",
        _ => {
            return Err(BacktestMarketDataStoreError::Validation(
                "invalid session scope".to_owned(),
            ));
        }
    };
    let hash = fnv1a(format!("{raw_provider}|{raw_symbol}").as_bytes());
    Ok(format!(
        "local_klines__{provider}__{symbol}__{interval}__{rehab}__{scope_tag}__{hash:08x}"
    ))
}

fn canonical_interval(interval: &str) -> String {
    match interval.trim().to_ascii_lowercase().as_str() {
        "1min" => "1m",
        "5min" => "5m",
        "15min" => "15m",
        normalized => normalized,
    }
    .to_owned()
}

fn normalize_component(value: &str, default: &str) -> String {
    let mut out = String::new();
    let mut underscore = false;
    for ch in value.trim().to_ascii_lowercase().chars() {
        if ch.is_ascii_alphanumeric() {
            out.push(ch);
            underscore = false;
        } else if !underscore {
            out.push('_');
            underscore = true;
        }
    }
    let trimmed = out.trim_matches('_');
    if trimmed.is_empty() {
        default.to_owned()
    } else {
        trimmed.to_owned()
    }
}

fn fnv1a(bytes: &[u8]) -> u32 {
    let mut hash = 0x811c9dc5u32;
    for byte in bytes {
        hash ^= u32::from(*byte);
        hash = hash.wrapping_mul(0x01000193);
    }
    hash
}
