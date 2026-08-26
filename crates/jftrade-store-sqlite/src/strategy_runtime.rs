use std::path::{Path, PathBuf};
use std::sync::{Mutex, MutexGuard};
use std::time::Duration;

use jftrade_owner_lock::{OwnerDiagnostic, WriterLease, WriterLeaseError};
use rusqlite::{Connection, OpenFlags, OptionalExtension, TransactionBehavior, params};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use thiserror::Error;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use crate::schema_manifest::{SchemaManifestError, validate_current};

const STRATEGY_COMPONENT: &str = "strategy";
const STRATEGY_SCHEMA_VERSION: i64 = 2;
pub const STRATEGY_RUNTIME_TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct StoredRuntimeInstance {
    pub id: String,
    pub plugin_id: String,
    pub status: String,
    pub binding: Value,
    pub runtime_risk: Value,
    pub definition_revision: i64,
    pub runtime_active: bool,
    pub deleted: bool,
    pub updated_at: String,
}

#[derive(Debug, Error)]
pub enum StrategyRuntimeStoreError {
    #[error("strategy database path is required")]
    EmptyPath,
    #[error("unsupported strategy runtime writer profile: {0}")]
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
    #[error("incompatible strategy database: {0}")]
    Incompatible(String),
}

pub struct StrategyRuntimeTestCutoverStore {
    path: PathBuf,
    connection: Mutex<Connection>,
    _writer_lease: WriterLease,
}

impl std::fmt::Debug for StrategyRuntimeTestCutoverStore {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("StrategyRuntimeTestCutoverStore")
            .field("path", &self.path)
            .finish()
    }
}

impl StrategyRuntimeTestCutoverStore {
    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, StrategyRuntimeStoreError> {
        let path = path.as_ref();
        if path.as_os_str().is_empty() {
            return Err(StrategyRuntimeStoreError::EmptyPath);
        }
        if profile != STRATEGY_RUNTIME_TEST_CUTOVER_PROFILE {
            return Err(StrategyRuntimeStoreError::UnsupportedProfile(
                profile.to_owned(),
            ));
        }
        if !path
            .metadata()
            .map(|metadata| metadata.is_file())
            .unwrap_or(false)
        {
            return Err(StrategyRuntimeStoreError::NotRegularFile(
                path.display().to_string(),
            ));
        }

        let writer_lease = WriterLease::acquire(
            path,
            &OwnerDiagnostic::current("rust", STRATEGY_RUNTIME_TEST_CUTOVER_PROFILE),
        )?;

        let connection = Connection::open_with_flags(
            path,
            OpenFlags::SQLITE_OPEN_READ_WRITE | OpenFlags::SQLITE_OPEN_NO_MUTEX,
        )
        .map_err(StrategyRuntimeStoreError::Open)?;

        connection
            .busy_timeout(Duration::from_secs(10))
            .map_err(StrategyRuntimeStoreError::Configure)?;

        connection
            .execute_batch("PRAGMA foreign_keys = ON;")
            .map_err(StrategyRuntimeStoreError::Configure)?;

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

    fn lock(&self) -> Result<MutexGuard<'_, Connection>, StrategyRuntimeStoreError> {
        self.connection
            .lock()
            .map_err(|_| StrategyRuntimeStoreError::LockUnavailable)
    }

    pub fn seed_instance(
        &self,
        instance_id: &str,
        status: &str,
        timestamp: &str,
    ) -> Result<(), StrategyRuntimeStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let connection = self.lock()?;
        let is_running = status == "RUNNING";
        let payload = json!({
            "binding": {},
            "runtimeRisk": {},
            "definitionRevision": 0,
            "runtimeActive": is_running,
            "deleted": false,
        });

        connection
            .execute(
                "INSERT INTO strategy_catalog_operations (operation_id, plugin_id, status, updated_at, payload_json)
                 VALUES (?1, '', ?2, ?3, ?4)
                 ON CONFLICT(operation_id) DO UPDATE SET
                    status = excluded.status,
                    updated_at = excluded.updated_at,
                    payload_json = excluded.payload_json",
                params![instance_id, status, timestamp, payload.to_string()],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        connection
            .execute(
                "INSERT INTO strategy_runtime_observations (instance_id, actual_status_snapshot, active_symbols_json, updated_at_ms)
                 VALUES (?1, ?2, '[]', 0)
                 ON CONFLICT(instance_id) DO UPDATE SET
                    actual_status_snapshot = excluded.actual_status_snapshot",
                params![instance_id, status],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        Ok(())
    }

    pub fn get_instance(
        &self,
        instance_id: &str,
    ) -> Result<Option<StoredRuntimeInstance>, StrategyRuntimeStoreError> {
        let connection = self.lock()?;
        get_instance_query(&connection, instance_id)
    }

    pub fn update_status(
        &self,
        instance_id: &str,
        new_status: &str,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(StrategyRuntimeStoreError::Query)?;

        let mut instance = get_instance_query(&transaction, instance_id)?
            .ok_or(StrategyRuntimeStoreError::NotFound)?;

        if instance.deleted {
            return Err(StrategyRuntimeStoreError::NotFound);
        }

        instance.status = new_status.to_owned();
        instance.runtime_active = new_status == "RUNNING";
        instance.updated_at = timestamp.to_owned();

        let payload = json!({
            "binding": instance.binding,
            "runtimeRisk": instance.runtime_risk,
            "definitionRevision": instance.definition_revision,
            "runtimeActive": instance.runtime_active,
            "deleted": instance.deleted,
        });

        transaction
            .execute(
                "UPDATE strategy_catalog_operations
                 SET status = ?1, updated_at = ?2, payload_json = ?3
                 WHERE operation_id = ?4",
                params![new_status, timestamp, payload.to_string(), instance_id],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        transaction
            .execute(
                "UPDATE strategy_runtime_observations
                 SET actual_status_snapshot = ?1
                 WHERE instance_id = ?2",
                params![new_status, instance_id],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        let event_kind = match new_status {
            "RUNNING" => "STARTED",
            "STOPPED" => "STOPPED",
            "PAUSED" => "PAUSED",
            _ => "STATUS_CHANGE",
        };
        transaction
            .execute(
                "INSERT INTO strategy_audit_events (instance_id, kind, detail, at_ms)
                 VALUES (?1, ?2, '', 0)",
                params![instance_id, event_kind],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        transaction
            .commit()
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(instance)
    }

    pub fn update_binding(
        &self,
        instance_id: &str,
        binding: Value,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(StrategyRuntimeStoreError::Query)?;

        let mut instance = get_instance_query(&transaction, instance_id)?
            .ok_or(StrategyRuntimeStoreError::NotFound)?;

        if instance.deleted {
            return Err(StrategyRuntimeStoreError::NotFound);
        }

        instance.binding = binding;
        instance.updated_at = timestamp.to_owned();

        let payload = json!({
            "binding": instance.binding,
            "runtimeRisk": instance.runtime_risk,
            "definitionRevision": instance.definition_revision,
            "runtimeActive": instance.runtime_active,
            "deleted": instance.deleted,
        });

        transaction
            .execute(
                "UPDATE strategy_catalog_operations
                 SET updated_at = ?1, payload_json = ?2
                 WHERE operation_id = ?3",
                params![timestamp, payload.to_string(), instance_id],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        transaction
            .commit()
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(instance)
    }

    pub fn update_risk(
        &self,
        instance_id: &str,
        risk: Value,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(StrategyRuntimeStoreError::Query)?;

        let mut instance = get_instance_query(&transaction, instance_id)?
            .ok_or(StrategyRuntimeStoreError::NotFound)?;

        if instance.deleted {
            return Err(StrategyRuntimeStoreError::NotFound);
        }

        instance.runtime_risk = risk;
        instance.updated_at = timestamp.to_owned();

        let payload = json!({
            "binding": instance.binding,
            "runtimeRisk": instance.runtime_risk,
            "definitionRevision": instance.definition_revision,
            "runtimeActive": instance.runtime_active,
            "deleted": instance.deleted,
        });

        transaction
            .execute(
                "UPDATE strategy_catalog_operations
                 SET updated_at = ?1, payload_json = ?2
                 WHERE operation_id = ?3",
                params![timestamp, payload.to_string(), instance_id],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        transaction
            .commit()
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(instance)
    }

    pub fn delete_instance(
        &self,
        instance_id: &str,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(StrategyRuntimeStoreError::Query)?;

        let mut instance = get_instance_query(&transaction, instance_id)?
            .ok_or(StrategyRuntimeStoreError::NotFound)?;

        if instance.deleted {
            return Err(StrategyRuntimeStoreError::NotFound);
        }

        instance.deleted = true;
        instance.runtime_active = false;
        instance.updated_at = timestamp.to_owned();

        let payload = json!({
            "binding": instance.binding,
            "runtimeRisk": instance.runtime_risk,
            "definitionRevision": instance.definition_revision,
            "runtimeActive": false,
            "deleted": true,
        });

        transaction
            .execute(
                "UPDATE strategy_catalog_operations
                 SET status = 'DELETED', updated_at = ?1, payload_json = ?2
                 WHERE operation_id = ?3",
                params![timestamp, payload.to_string(), instance_id],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        transaction
            .execute(
                "DELETE FROM strategy_runtime_observations WHERE instance_id = ?1",
                params![instance_id],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        transaction
            .commit()
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(instance)
    }

    pub fn refresh_definition(
        &self,
        instance_id: &str,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(StrategyRuntimeStoreError::Query)?;

        let mut instance = get_instance_query(&transaction, instance_id)?
            .ok_or(StrategyRuntimeStoreError::NotFound)?;

        if instance.deleted {
            return Err(StrategyRuntimeStoreError::NotFound);
        }

        instance.definition_revision += 1;
        instance.updated_at = timestamp.to_owned();

        let payload = json!({
            "binding": instance.binding,
            "runtimeRisk": instance.runtime_risk,
            "definitionRevision": instance.definition_revision,
            "runtimeActive": instance.runtime_active,
            "deleted": instance.deleted,
        });

        transaction
            .execute(
                "UPDATE strategy_catalog_operations
                 SET updated_at = ?1, payload_json = ?2
                 WHERE operation_id = ?3",
                params![timestamp, payload.to_string(), instance_id],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        transaction
            .commit()
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(instance)
    }
}

fn get_instance_query(
    connection: &Connection,
    instance_id: &str,
) -> Result<Option<StoredRuntimeInstance>, StrategyRuntimeStoreError> {
    let row: Option<(String, String, String, String)> = connection
        .query_row(
            "SELECT operation_id, plugin_id, status, payload_json
             FROM strategy_catalog_operations WHERE operation_id = ?1",
            params![instance_id],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?)),
        )
        .optional()
        .map_err(StrategyRuntimeStoreError::Query)?;

    match row {
        Some((id, plugin_id, status, payload_json)) => {
            let payload: Value = serde_json::from_str(&payload_json).unwrap_or_else(|_| json!({}));
            let binding = payload["binding"].clone();
            let runtime_risk = payload["runtimeRisk"].clone();
            let definition_revision = payload["definitionRevision"].as_i64().unwrap_or(0);
            let runtime_active = payload["runtimeActive"]
                .as_bool()
                .unwrap_or(status == "RUNNING");
            let deleted = payload["deleted"].as_bool().unwrap_or(status == "DELETED");
            Ok(Some(StoredRuntimeInstance {
                id,
                plugin_id,
                status,
                binding,
                runtime_risk,
                definition_revision,
                runtime_active,
                deleted,
                updated_at: "".to_owned(),
            }))
        }
        None => Ok(None),
    }
}

fn validate_rfc3339_timestamp(timestamp: &str) -> Result<(), StrategyRuntimeStoreError> {
    OffsetDateTime::parse(timestamp, &Rfc3339)
        .map(|_| ())
        .map_err(|error| {
            StrategyRuntimeStoreError::Incompatible(format!(
                "invalid RFC3339 timestamp {timestamp:?}: {error}"
            ))
        })
}
