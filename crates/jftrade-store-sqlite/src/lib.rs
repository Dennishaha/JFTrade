#![forbid(unsafe_code)]

//! Strict read-only SQLite inspection for staged migration snapshots.

mod data_management;
mod schema_manifest;

pub use data_management::{ManagedDatabaseCleanupCandidateStore, ManagedDatabaseOverviewStore};

use std::path::Path;
use std::time::Duration;

use jftrade_kernel::Fixed8;
use rusqlite::{Connection, OpenFlags};
use serde::Serialize;
use thiserror::Error;

const BACKTEST_COMPONENT: &str = "backtest";
const BACKTEST_SCHEMA_VERSION: i64 = 3;
const KLINE_PREFIX: &str = "local_klines__";

#[derive(Debug, Error)]
pub enum SnapshotError {
    #[error("SQLite snapshot path is required")]
    EmptyPath,
    #[error("SQLite snapshot is not a regular file: {0}")]
    NotRegularFile(String),
    #[error("open read-only SQLite snapshot: {0}")]
    Open(#[source] rusqlite::Error),
    #[error("configure read-only SQLite snapshot: {0}")]
    Configure(#[source] rusqlite::Error),
    #[error("incompatible backtest SQLite snapshot: {0}")]
    Incompatible(String),
    #[error("inspect backtest SQLite snapshot: {0}")]
    Inspect(#[source] rusqlite::Error),
    #[error("decode fixed8 column {column} value {value:?}: {reason}")]
    Fixed8 {
        column: &'static str,
        value: String,
        reason: String,
    },
}

#[derive(Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BacktestSnapshot {
    pub component_id: String,
    pub version: i64,
    pub pragmas: Pragmas,
    pub tables: Vec<Table>,
    pub klines: Vec<Kline>,
}

#[derive(Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Pragmas {
    pub foreign_keys: i64,
    pub query_only: i64,
    pub busy_timeout: i64,
    pub journal_mode: String,
}

#[derive(Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Table {
    pub name: String,
    pub without_rowid: bool,
    pub columns: Vec<Column>,
}

#[derive(Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Column {
    pub cid: i64,
    pub name: String,
    #[serde(rename = "type")]
    pub type_name: String,
    pub not_null: i64,
    pub primary_key: i64,
    pub hidden: i64,
}

#[derive(Debug, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Kline {
    pub table: String,
    pub end_time: i64,
    pub start_time: i64,
    pub open: String,
    pub high: String,
    pub low: String,
    pub close: String,
    pub volume: String,
}

pub fn inspect_backtest_snapshot(
    path: impl AsRef<Path>,
) -> Result<BacktestSnapshot, SnapshotError> {
    let path = path.as_ref();
    if path.as_os_str().is_empty() {
        return Err(SnapshotError::EmptyPath);
    }
    if !path
        .metadata()
        .map(|metadata| metadata.is_file())
        .unwrap_or(false)
    {
        return Err(SnapshotError::NotRegularFile(path.display().to_string()));
    }
    let connection = Connection::open_with_flags(
        path,
        OpenFlags::SQLITE_OPEN_READ_ONLY | OpenFlags::SQLITE_OPEN_NO_MUTEX,
    )
    .map_err(SnapshotError::Open)?;
    configure_read_only(&connection)?;
    inspect_connection(&connection)
}

fn configure_read_only(connection: &Connection) -> Result<(), SnapshotError> {
    connection
        .execute_batch("PRAGMA query_only = ON; PRAGMA foreign_keys = ON;")
        .map_err(SnapshotError::Configure)?;
    connection
        .busy_timeout(Duration::from_secs(10))
        .map_err(SnapshotError::Configure)
}

fn inspect_connection(connection: &Connection) -> Result<BacktestSnapshot, SnapshotError> {
    let metadata = read_metadata(connection)?;
    let pragmas = read_pragmas(connection)?;
    let tables = read_tables(connection)?;
    validate_tables(&tables)?;
    let klines = read_klines(connection, &tables)?;
    Ok(BacktestSnapshot {
        component_id: metadata.0,
        version: metadata.1,
        pragmas,
        tables,
        klines,
    })
}

fn read_metadata(connection: &Connection) -> Result<(String, i64), SnapshotError> {
    let mut statement = connection
        .prepare("SELECT component_id, version FROM jftrade_schema_meta ORDER BY component_id")
        .map_err(SnapshotError::Inspect)?;
    let rows = statement
        .query_map([], |row| {
            Ok((row.get::<_, String>(0)?, row.get::<_, i64>(1)?))
        })
        .map_err(SnapshotError::Inspect)?
        .collect::<Result<Vec<_>, _>>()
        .map_err(SnapshotError::Inspect)?;
    if rows.as_slice() != [(BACKTEST_COMPONENT.to_owned(), BACKTEST_SCHEMA_VERSION)] {
        return Err(SnapshotError::Incompatible(format!(
            "metadata rows {rows:?}; expected exactly backtest version 3"
        )));
    }
    rows.into_iter()
        .next()
        .ok_or_else(|| SnapshotError::Incompatible("validated metadata row disappeared".to_owned()))
}

fn pragma_i64(connection: &Connection, name: &str) -> Result<i64, SnapshotError> {
    connection
        .query_row(&format!("PRAGMA {name}"), [], |row| row.get(0))
        .map_err(SnapshotError::Inspect)
}

fn read_pragmas(connection: &Connection) -> Result<Pragmas, SnapshotError> {
    let journal_mode = connection
        .query_row("PRAGMA journal_mode", [], |row| row.get(0))
        .map_err(SnapshotError::Inspect)?;
    Ok(Pragmas {
        foreign_keys: pragma_i64(connection, "foreign_keys")?,
        query_only: pragma_i64(connection, "query_only")?,
        busy_timeout: pragma_i64(connection, "busy_timeout")?,
        journal_mode,
    })
}

fn read_tables(connection: &Connection) -> Result<Vec<Table>, SnapshotError> {
    let mut statement = connection
        .prepare(
            "SELECT name, sql FROM sqlite_schema \
             WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name",
        )
        .map_err(SnapshotError::Inspect)?;
    let table_rows = statement
        .query_map([], |row| {
            Ok((row.get::<_, String>(0)?, row.get::<_, String>(1)?))
        })
        .map_err(SnapshotError::Inspect)?
        .collect::<Result<Vec<_>, _>>()
        .map_err(SnapshotError::Inspect)?;
    table_rows
        .into_iter()
        .map(|(name, sql)| {
            if name != "jftrade_schema_meta" && !valid_kline_table_name(&name) {
                return Err(SnapshotError::Incompatible(format!(
                    "unexpected table {name}"
                )));
            }
            let columns = read_columns(connection, &name)?;
            Ok(Table {
                name,
                without_rowid: sql.to_ascii_uppercase().contains("WITHOUT ROWID"),
                columns,
            })
        })
        .collect()
}

fn read_columns(connection: &Connection, table: &str) -> Result<Vec<Column>, SnapshotError> {
    let mut statement = connection
        .prepare(&format!("PRAGMA table_xinfo(\"{table}\")"))
        .map_err(SnapshotError::Inspect)?;
    statement
        .query_map([], |row| {
            Ok(Column {
                cid: row.get(0)?,
                name: row.get(1)?,
                type_name: row.get(2)?,
                not_null: row.get(3)?,
                primary_key: row.get(5)?,
                hidden: row.get(6)?,
            })
        })
        .map_err(SnapshotError::Inspect)?
        .collect::<Result<Vec<_>, _>>()
        .map_err(SnapshotError::Inspect)
}

fn validate_tables(tables: &[Table]) -> Result<(), SnapshotError> {
    let metadata = tables
        .iter()
        .find(|table| table.name == "jftrade_schema_meta")
        .ok_or_else(|| SnapshotError::Incompatible("metadata table is missing".to_owned()))?;
    validate_columns(
        metadata,
        &[
            ("component_id", "TEXT", 0, 1),
            ("version", "INTEGER", 1, 0),
            ("created_at", "TEXT", 1, 0),
        ],
        false,
    )?;
    let kline_tables = tables
        .iter()
        .filter(|table| table.name != "jftrade_schema_meta")
        .collect::<Vec<_>>();
    if kline_tables.is_empty() {
        return Err(SnapshotError::Incompatible(
            "at least one K-line table is required".to_owned(),
        ));
    }
    for table in kline_tables {
        if !valid_kline_table_name(&table.name) {
            return Err(SnapshotError::Incompatible(format!(
                "unexpected table {}",
                table.name
            )));
        }
        validate_columns(
            table,
            &[
                ("end_time", "INTEGER", 1, 1),
                ("start_time", "INTEGER", 1, 0),
                ("open", "TEXT", 1, 0),
                ("high", "TEXT", 1, 0),
                ("low", "TEXT", 1, 0),
                ("close", "TEXT", 1, 0),
                ("volume", "TEXT", 1, 0),
            ],
            true,
        )?;
    }
    Ok(())
}

fn validate_columns(
    table: &Table,
    expected: &[(&str, &str, i64, i64)],
    without_rowid: bool,
) -> Result<(), SnapshotError> {
    let actual = table
        .columns
        .iter()
        .map(|column| {
            (
                column.name.as_str(),
                column.type_name.as_str(),
                column.not_null,
                column.primary_key,
            )
        })
        .collect::<Vec<_>>();
    if actual != expected || table.without_rowid != without_rowid {
        return Err(SnapshotError::Incompatible(format!(
            "table {} does not match the current schema",
            table.name
        )));
    }
    Ok(())
}

fn valid_kline_table_name(name: &str) -> bool {
    let Some(suffix) = name.strip_prefix(KLINE_PREFIX) else {
        return false;
    };
    let parts = suffix.split("__").collect::<Vec<_>>();
    parts.len() == 6
        && parts[..3].iter().all(|part| {
            !part.is_empty()
                && part
                    .bytes()
                    .all(|byte| byte.is_ascii_lowercase() || byte.is_ascii_digit() || byte == b'_')
        })
        && matches!(parts[3], "forward" | "backward" | "none")
        && matches!(parts[4], "r" | "x")
        && parts[5].len() == 8
        && parts[5]
            .bytes()
            .all(|byte| byte.is_ascii_hexdigit() && !byte.is_ascii_uppercase())
}

fn read_klines(connection: &Connection, tables: &[Table]) -> Result<Vec<Kline>, SnapshotError> {
    let mut result = Vec::new();
    for table in tables
        .iter()
        .filter(|table| table.name.starts_with(KLINE_PREFIX))
    {
        let mut statement = connection
            .prepare(&format!(
                "SELECT end_time, start_time, open, high, low, close, volume \
                 FROM \"{}\" ORDER BY end_time ASC",
                table.name
            ))
            .map_err(SnapshotError::Inspect)?;
        let rows = statement
            .query_map([], |row| {
                Ok((
                    row.get::<_, i64>(0)?,
                    row.get::<_, i64>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, String>(3)?,
                    row.get::<_, String>(4)?,
                    row.get::<_, String>(5)?,
                    row.get::<_, String>(6)?,
                ))
            })
            .map_err(SnapshotError::Inspect)?
            .collect::<Result<Vec<_>, _>>()
            .map_err(SnapshotError::Inspect)?;
        for row in rows {
            result.push(Kline {
                table: table.name.clone(),
                end_time: row.0,
                start_time: row.1,
                open: canonical_fixed8("open", row.2)?,
                high: canonical_fixed8("high", row.3)?,
                low: canonical_fixed8("low", row.4)?,
                close: canonical_fixed8("close", row.5)?,
                volume: canonical_fixed8("volume", row.6)?,
            });
        }
    }
    Ok(result)
}

fn canonical_fixed8(column: &'static str, value: String) -> Result<String, SnapshotError> {
    value
        .parse::<Fixed8>()
        .map(|fixed| fixed.storage_text())
        .map_err(|error| SnapshotError::Fixed8 {
            column,
            value,
            reason: error.to_string(),
        })
}
