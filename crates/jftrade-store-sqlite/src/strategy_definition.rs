use std::path::{Path, PathBuf};
use std::sync::{Mutex, MutexGuard};
use std::time::Duration;

use jftrade_owner_lock::{OwnerDiagnostic, WriterLease, WriterLeaseError};
use rusqlite::{Connection, OpenFlags, OptionalExtension, TransactionBehavior, params};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use thiserror::Error;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use crate::schema_manifest::{SchemaManifestError, validate_current};

const STRATEGY_COMPONENT: &str = "strategy";
const STRATEGY_SCHEMA_VERSION: i64 = 2;
pub const STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredStrategyDefinition {
    pub id: String,
    pub name: String,
    pub version: String,
    pub description: String,
    pub runtime: String,
    pub source_format: String,
    pub symbol: String,
    pub interval: String,
    pub script: String,
    pub visual_model_json: String,
    pub created_at: String,
    pub updated_at: String,
    pub deleted_at: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredStrategyVersion {
    pub definition_id: String,
    pub version: String,
    pub name: String,
    pub description: String,
    pub runtime: String,
    pub source_format: String,
    pub symbol: String,
    pub interval: String,
    pub script: String,
    pub visual_model_json: String,
    pub created_at: String,
    pub updated_at: String,
    pub saved_at: String,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredStrategyInstance {
    pub id: String,
    pub definition_id: String,
    pub definition_version: String,
    pub payload: Value,
    pub binding: Value,
    pub status: String,
}

#[derive(Debug, Error)]
pub enum StrategyDefinitionStoreError {
    #[error("strategy database path is required")]
    EmptyPath,
    #[error("unsupported strategy writer profile: {0}")]
    UnsupportedProfile(String),
    #[error("strategy database is not an existing regular file: {0}")]
    NotRegularFile(String),
    #[error(transparent)]
    WriterLease(#[from] WriterLeaseError),
    #[error("open strategy database: {0}")]
    Open(#[source] rusqlite::Error),
    #[error("configure strategy database: {0}")]
    Configure(#[source] rusqlite::Error),
    #[error(transparent)]
    Schema(#[from] SchemaManifestError),
    #[error("strategy database lock is unavailable")]
    LockUnavailable,
    #[error("query strategy database: {0}")]
    Query(#[source] rusqlite::Error),
    #[error("strategy resource not found")]
    NotFound,
    #[error("strategy state conflict")]
    Conflict,
    #[error("invalid strategy request: {0}")]
    Validation(String),
    #[error("strategy deletion guard: {0}")]
    DeleteGuard(String),
    #[error("incompatible strategy database: {0}")]
    Incompatible(String),
}

pub struct StrategyDefinitionTestCutoverStore {
    path: PathBuf,
    connection: Mutex<Connection>,
    _writer_lease: WriterLease,
}

impl std::fmt::Debug for StrategyDefinitionTestCutoverStore {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("StrategyDefinitionTestCutoverStore")
            .field("path", &self.path)
            .finish()
    }
}

impl StrategyDefinitionTestCutoverStore {
    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, StrategyDefinitionStoreError> {
        let path = path.as_ref();
        if path.as_os_str().is_empty() {
            return Err(StrategyDefinitionStoreError::EmptyPath);
        }
        if profile != STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE {
            return Err(StrategyDefinitionStoreError::UnsupportedProfile(
                profile.to_owned(),
            ));
        }
        if !path
            .metadata()
            .map(|metadata| metadata.is_file())
            .unwrap_or(false)
        {
            return Err(StrategyDefinitionStoreError::NotRegularFile(
                path.display().to_string(),
            ));
        }

        let writer_lease = WriterLease::acquire(
            path,
            &OwnerDiagnostic::current("rust", STRATEGY_DEFINITION_TEST_CUTOVER_PROFILE),
        )?;

        let connection = Connection::open_with_flags(
            path,
            OpenFlags::SQLITE_OPEN_READ_WRITE | OpenFlags::SQLITE_OPEN_NO_MUTEX,
        )
        .map_err(StrategyDefinitionStoreError::Open)?;

        connection
            .busy_timeout(Duration::from_secs(10))
            .map_err(StrategyDefinitionStoreError::Configure)?;

        connection
            .execute_batch("PRAGMA foreign_keys = ON;")
            .map_err(StrategyDefinitionStoreError::Configure)?;

        validate_current(
            &connection,
            &path.display().to_string(),
            STRATEGY_COMPONENT,
            STRATEGY_SCHEMA_VERSION,
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

    fn lock(&self) -> Result<MutexGuard<'_, Connection>, StrategyDefinitionStoreError> {
        self.connection
            .lock()
            .map_err(|_| StrategyDefinitionStoreError::LockUnavailable)
    }

    pub fn get_definition(
        &self,
        id: &str,
        include_deleted: bool,
    ) -> Result<Option<StoredStrategyDefinition>, StrategyDefinitionStoreError> {
        let connection = self.lock()?;
        get_definition_query(&connection, id, include_deleted)
    }

    pub fn save_definition(
        &self,
        mut definition: StoredStrategyDefinition,
        timestamp: &str,
    ) -> Result<StoredStrategyDefinition, StrategyDefinitionStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(StrategyDefinitionStoreError::Query)?;

        let existing = get_definition_query(&transaction, &definition.id, true)?;

        let changed = match &existing {
            Some(curr) => {
                definition.created_at = curr.created_at.clone();
                let is_changed = curr.name != definition.name
                    || curr.description != definition.description
                    || curr.runtime != definition.runtime
                    || curr.source_format != definition.source_format
                    || curr.symbol != definition.symbol
                    || curr.interval != definition.interval
                    || curr.script != definition.script
                    || curr.visual_model_json != definition.visual_model_json;
                if is_changed {
                    definition.version = increment_patch_version(&curr.version);
                } else {
                    definition.version = curr.version.clone();
                }
                is_changed
            }
            None => {
                definition.created_at = timestamp.to_owned();
                definition.version = "0.1.0".to_owned();
                true
            }
        };

        definition.updated_at = timestamp.to_owned();
        definition.deleted_at = None;

        transaction
            .execute(
                "INSERT INTO strategy_design_definitions
                    (id, name, version, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, deleted_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, NULL)
                 ON CONFLICT(id) DO UPDATE SET
                    name = excluded.name,
                    version = excluded.version,
                    description = excluded.description,
                    runtime = excluded.runtime,
                    source_format = excluded.source_format,
                    symbol = excluded.symbol,
                    interval = excluded.interval,
                    script = excluded.script,
                    visual_model_json = excluded.visual_model_json,
                    updated_at = excluded.updated_at,
                    deleted_at = NULL",
                params![
                    definition.id,
                    definition.name,
                    definition.version,
                    definition.description,
                    definition.runtime,
                    definition.source_format,
                    definition.symbol,
                    definition.interval,
                    definition.script,
                    definition.visual_model_json,
                    definition.created_at,
                    definition.updated_at,
                ],
            )
            .map_err(StrategyDefinitionStoreError::Query)?;

        if changed {
            transaction
                .execute(
                    "INSERT INTO strategy_definition_versions
                        (definition_id, version, name, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, saved_at)
                     VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13)",
                    params![
                        definition.id,
                        definition.version,
                        definition.name,
                        definition.description,
                        definition.runtime,
                        definition.source_format,
                        definition.symbol,
                        definition.interval,
                        definition.script,
                        definition.visual_model_json,
                        definition.created_at,
                        definition.updated_at,
                        timestamp,
                    ],
                )
                .map_err(StrategyDefinitionStoreError::Query)?;
        }

        transaction
            .commit()
            .map_err(StrategyDefinitionStoreError::Query)?;
        Ok(definition)
    }

    pub fn delete_definition(
        &self,
        id: &str,
        timestamp: &str,
    ) -> Result<StoredStrategyDefinition, StrategyDefinitionStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(StrategyDefinitionStoreError::Query)?;

        let existing = get_definition_query(&transaction, id, false)?
            .ok_or(StrategyDefinitionStoreError::NotFound)?;

        let active_count: i64 = transaction
            .query_row(
                "SELECT COUNT(*) FROM strategy_catalog_operations WHERE plugin_id = ?1",
                params![id],
                |row| row.get(0),
            )
            .optional()
            .map_err(StrategyDefinitionStoreError::Query)?
            .unwrap_or(0);

        if active_count > 0 {
            return Err(StrategyDefinitionStoreError::DeleteGuard(format!(
                "cannot delete strategy definition with {active_count} active instances"
            )));
        }

        transaction
            .execute(
                "UPDATE strategy_design_definitions SET updated_at = ?1, deleted_at = ?1 WHERE id = ?2",
                params![timestamp, id],
            )
            .map_err(StrategyDefinitionStoreError::Query)?;

        let mut deleted = existing;
        deleted.updated_at = timestamp.to_owned();
        deleted.deleted_at = Some(timestamp.to_owned());

        transaction
            .commit()
            .map_err(StrategyDefinitionStoreError::Query)?;
        Ok(deleted)
    }

    pub fn list_versions(
        &self,
        definition_id: &str,
    ) -> Result<Vec<StoredStrategyVersion>, StrategyDefinitionStoreError> {
        let connection = self.lock()?;
        let mut statement = connection
            .prepare(
                "SELECT definition_id, version, name, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, saved_at
                 FROM strategy_definition_versions
                 WHERE definition_id = ?1
                 ORDER BY saved_at DESC, version DESC",
            )
            .map_err(StrategyDefinitionStoreError::Query)?;

        let rows = statement
            .query_map(params![definition_id], |row| {
                Ok(StoredStrategyVersion {
                    definition_id: row.get(0)?,
                    version: row.get(1)?,
                    name: row.get(2)?,
                    description: row.get(3)?,
                    runtime: row.get(4)?,
                    source_format: row.get(5)?,
                    symbol: row.get(6)?,
                    interval: row.get(7)?,
                    script: row.get(8)?,
                    visual_model_json: row.get(9)?,
                    created_at: row.get(10)?,
                    updated_at: row.get(11)?,
                    saved_at: row.get(12)?,
                })
            })
            .map_err(StrategyDefinitionStoreError::Query)?;

        let mut versions = Vec::new();
        for row in rows {
            versions.push(row.map_err(StrategyDefinitionStoreError::Query)?);
        }
        Ok(versions)
    }
}

fn get_definition_query(
    connection: &Connection,
    id: &str,
    include_deleted: bool,
) -> Result<Option<StoredStrategyDefinition>, StrategyDefinitionStoreError> {
    let sql = if include_deleted {
        "SELECT id, name, version, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, deleted_at
         FROM strategy_design_definitions WHERE id = ?1"
    } else {
        "SELECT id, name, version, description, runtime, source_format, symbol, interval, script, visual_model_json, created_at, updated_at, deleted_at
         FROM strategy_design_definitions WHERE id = ?1 AND (deleted_at IS NULL OR TRIM(deleted_at) = '')"
    };

    connection
        .query_row(sql, params![id], |row| {
            Ok(StoredStrategyDefinition {
                id: row.get(0)?,
                name: row.get(1)?,
                version: row.get(2)?,
                description: row.get(3)?,
                runtime: row.get(4)?,
                source_format: row.get(5)?,
                symbol: row.get(6)?,
                interval: row.get(7)?,
                script: row.get(8)?,
                visual_model_json: row.get(9)?,
                created_at: row.get(10)?,
                updated_at: row.get(11)?,
                deleted_at: row.get(12)?,
            })
        })
        .optional()
        .map_err(StrategyDefinitionStoreError::Query)
}

fn increment_patch_version(version: &str) -> String {
    let mut parts = version
        .split('.')
        .map(|part| part.parse::<u64>().unwrap_or(0));
    let major = parts.next().unwrap_or(0);
    let minor = parts.next().unwrap_or(1);
    let patch = parts.next().unwrap_or(0).saturating_add(1);
    format!("{major}.{minor}.{patch}")
}

fn validate_rfc3339_timestamp(timestamp: &str) -> Result<(), StrategyDefinitionStoreError> {
    OffsetDateTime::parse(timestamp, &Rfc3339)
        .map(|_| ())
        .map_err(|error| {
            StrategyDefinitionStoreError::Incompatible(format!(
                "invalid RFC3339 timestamp {timestamp:?}: {error}"
            ))
        })
}
