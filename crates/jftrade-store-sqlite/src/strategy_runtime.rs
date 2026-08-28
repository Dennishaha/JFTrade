use std::path::Path;
use std::sync::MutexGuard;

use jftrade_owner_lock::WriterLeaseError;
use rusqlite::{Connection, OptionalExtension, TransactionBehavior, params};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use thiserror::Error;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use crate::schema_manifest::SchemaManifestError;

pub const STRATEGY_RUNTIME_TEST_CUTOVER_PROFILE: &str = "cutover-test-only.v1";
pub const STRATEGY_RUNTIME_PRODUCTION_PROFILE: &str = "production.v1";

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

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StoredStrategyLogEvent {
    pub raw: String,
    pub level: String,
    pub at_ms: i64,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StoredStrategyAuditEvent {
    pub instance_id: String,
    pub kind: String,
    pub detail: String,
    pub at_ms: i64,
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

use crate::strategy_definition::{
    StrategyDefinitionStore, StrategyDefinitionStoreError, StrategyStoreInner,
};
use std::sync::Arc;

impl From<StrategyDefinitionStoreError> for StrategyRuntimeStoreError {
    fn from(err: StrategyDefinitionStoreError) -> Self {
        match err {
            StrategyDefinitionStoreError::EmptyPath => Self::EmptyPath,
            StrategyDefinitionStoreError::UnsupportedProfile(p) => Self::UnsupportedProfile(p),
            StrategyDefinitionStoreError::NotRegularFile(p) => Self::NotRegularFile(p),
            StrategyDefinitionStoreError::WriterLease(e) => Self::WriterLease(e),
            StrategyDefinitionStoreError::Open(e) => Self::Open(e),
            StrategyDefinitionStoreError::Configure(e) => Self::Configure(e),
            StrategyDefinitionStoreError::Schema(e) => Self::Schema(e),
            StrategyDefinitionStoreError::LockUnavailable => Self::LockUnavailable,
            StrategyDefinitionStoreError::Query(e) => Self::Query(e),
            StrategyDefinitionStoreError::NotFound => Self::NotFound,
            StrategyDefinitionStoreError::Conflict => Self::Conflict,
            StrategyDefinitionStoreError::Validation(m) => Self::Validation(m),
            StrategyDefinitionStoreError::DeleteGuard(m)
            | StrategyDefinitionStoreError::Incompatible(m) => Self::Incompatible(m),
        }
    }
}

#[derive(Clone, Debug)]
pub struct StrategyRuntimeStore {
    pub(crate) inner: Arc<StrategyStoreInner>,
}

impl StrategyRuntimeStore {
    pub fn open(path: impl AsRef<Path>) -> Result<Self, StrategyRuntimeStoreError> {
        Self::open_existing(path, STRATEGY_RUNTIME_PRODUCTION_PROFILE)
    }

    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, StrategyRuntimeStoreError> {
        let inner = StrategyStoreInner::open_existing(path, profile)?;
        Ok(Self {
            inner: Arc::new(inner),
        })
    }

    pub fn from_definition_store(definition_store: &StrategyDefinitionStore) -> Self {
        Self {
            inner: Arc::clone(definition_store.inner()),
        }
    }

    pub fn path(&self) -> &Path {
        &self.inner.path
    }

    fn lock(&self) -> Result<MutexGuard<'_, Connection>, StrategyRuntimeStoreError> {
        self.inner
            .connection
            .lock()
            .map_err(|_| StrategyRuntimeStoreError::LockUnavailable)
    }

    pub fn seed_instance(
        &self,
        instance_id: &str,
        status: &str,
        timestamp: &str,
    ) -> Result<(), StrategyRuntimeStoreError> {
        self.seed_instance_with_binding(instance_id, status, json!({}), timestamp)
    }

    pub fn seed_instance_with_binding(
        &self,
        instance_id: &str,
        status: &str,
        binding: Value,
        timestamp: &str,
    ) -> Result<(), StrategyRuntimeStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(StrategyRuntimeStoreError::Query)?;
        let is_running = status == "RUNNING";
        let runtime_risk = binding
            .get("runtimeRisk")
            .cloned()
            .unwrap_or_else(|| json!({}));
        let payload = json!({
            "binding": binding,
            "runtimeRisk": runtime_risk,
            "definitionRevision": 0,
            "runtimeActive": is_running,
            "deleted": false,
        });

        transaction
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

        transaction
            .execute(
                "INSERT INTO strategy_runtime_observations (instance_id, actual_status_snapshot, active_symbols_json, updated_at_ms)
                 VALUES (?1, ?2, '[]', 0)
                 ON CONFLICT(instance_id) DO UPDATE SET
                    actual_status_snapshot = excluded.actual_status_snapshot",
                params![instance_id, status],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        transaction
            .commit()
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(())
    }

    pub fn list_instances(&self) -> Result<Vec<StoredRuntimeInstance>, StrategyRuntimeStoreError> {
        let connection = self.lock()?;
        let mut statement = connection
            .prepare("SELECT operation_id, plugin_id, status, updated_at, payload_json FROM strategy_catalog_operations ORDER BY updated_at ASC, operation_id ASC")
            .map_err(StrategyRuntimeStoreError::Query)?;
        let rows = statement
            .query_map([], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, String>(3)?,
                    row.get::<_, String>(4)?,
                ))
            })
            .map_err(StrategyRuntimeStoreError::Query)?;
        let mut instances = Vec::new();
        for row in rows {
            let (id, plugin_id, status, updated_at, payload_json) =
                row.map_err(StrategyRuntimeStoreError::Query)?;
            let decoded = decode_instance(id, plugin_id, status, updated_at, &payload_json)?;
            let deleted = decoded.deleted;
            if !deleted {
                instances.push(decoded);
            }
        }
        Ok(instances)
    }

    pub fn get_instance(
        &self,
        instance_id: &str,
    ) -> Result<Option<StoredRuntimeInstance>, StrategyRuntimeStoreError> {
        let connection = self.lock()?;
        get_instance_query(&connection, instance_id)
    }

    pub fn list_log_events(
        &self,
        instance_id: &str,
    ) -> Result<Vec<StoredStrategyLogEvent>, StrategyRuntimeStoreError> {
        let connection = self.lock()?;
        let mut statement = connection
            .prepare(
                "SELECT raw, level, at_ms FROM strategy_log_events \
                 WHERE instance_id = ?1 ORDER BY at_ms DESC, id DESC",
            )
            .map_err(StrategyRuntimeStoreError::Query)?;
        let rows = statement
            .query_map([instance_id], |row| {
                Ok(StoredStrategyLogEvent {
                    raw: row.get(0)?,
                    level: row.get(1)?,
                    at_ms: row.get(2)?,
                })
            })
            .map_err(StrategyRuntimeStoreError::Query)?;
        rows.collect::<Result<Vec<_>, _>>()
            .map_err(StrategyRuntimeStoreError::Query)
    }

    pub fn list_audit_events(
        &self,
        instance_id: &str,
    ) -> Result<Vec<StoredStrategyAuditEvent>, StrategyRuntimeStoreError> {
        let connection = self.lock()?;
        let mut statement = connection
            .prepare(
                "SELECT instance_id, kind, detail, at_ms FROM strategy_audit_events \
                 WHERE instance_id = ?1 ORDER BY at_ms DESC, id DESC",
            )
            .map_err(StrategyRuntimeStoreError::Query)?;
        let rows = statement
            .query_map([instance_id], |row| {
                Ok(StoredStrategyAuditEvent {
                    instance_id: row.get(0)?,
                    kind: row.get(1)?,
                    detail: row.get(2)?,
                    at_ms: row.get(3)?,
                })
            })
            .map_err(StrategyRuntimeStoreError::Query)?;
        rows.collect::<Result<Vec<_>, _>>()
            .map_err(StrategyRuntimeStoreError::Query)
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
    let row: Option<(String, String, String, String, String)> = connection
        .query_row(
            "SELECT operation_id, plugin_id, status, updated_at, payload_json
             FROM strategy_catalog_operations WHERE operation_id = ?1",
            params![instance_id],
            |row| {
                Ok((
                    row.get(0)?,
                    row.get(1)?,
                    row.get(2)?,
                    row.get(3)?,
                    row.get(4)?,
                ))
            },
        )
        .optional()
        .map_err(StrategyRuntimeStoreError::Query)?;

    match row {
        Some((id, plugin_id, status, updated_at, payload_json)) => {
            decode_instance(id, plugin_id, status, updated_at, &payload_json).map(Some)
        }
        None => Ok(None),
    }
}

fn decode_instance(
    id: String,
    plugin_id: String,
    status: String,
    updated_at: String,
    payload_json: &str,
) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
    let payload: Value = serde_json::from_str(payload_json).map_err(|error| {
        StrategyRuntimeStoreError::Incompatible(format!(
            "strategy instance {id:?} contains invalid payload JSON: {error}"
        ))
    })?;
    let object = payload.as_object().ok_or_else(|| {
        StrategyRuntimeStoreError::Incompatible(format!(
            "strategy instance {id:?} payload must be a JSON object"
        ))
    })?;
    let binding = object.get("binding").cloned().unwrap_or_else(|| json!({}));
    let runtime_risk = object
        .get("runtimeRisk")
        .cloned()
        .unwrap_or_else(|| json!({}));
    let definition_revision = object
        .get("definitionRevision")
        .and_then(Value::as_i64)
        .unwrap_or(0);
    let runtime_active = object
        .get("runtimeActive")
        .and_then(Value::as_bool)
        .unwrap_or(status == "RUNNING");
    let deleted = object
        .get("deleted")
        .and_then(Value::as_bool)
        .unwrap_or(status == "DELETED");
    Ok(StoredRuntimeInstance {
        id,
        plugin_id,
        status,
        binding,
        runtime_risk,
        definition_revision,
        runtime_active,
        deleted,
        updated_at,
    })
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

#[derive(Debug)]
pub struct StrategyRuntimeTestCutoverStore {
    inner: StrategyRuntimeStore,
}

impl StrategyRuntimeTestCutoverStore {
    pub fn open_existing(
        path: impl AsRef<Path>,
        profile: &str,
    ) -> Result<Self, StrategyRuntimeStoreError> {
        let inner = StrategyRuntimeStore::open_existing(path, profile)?;
        Ok(Self { inner })
    }

    pub fn path(&self) -> &Path {
        self.inner.path()
    }

    pub fn seed_instance(
        &self,
        instance_id: &str,
        status: &str,
        timestamp: &str,
    ) -> Result<(), StrategyRuntimeStoreError> {
        self.inner.seed_instance(instance_id, status, timestamp)
    }

    pub fn seed_instance_with_binding(
        &self,
        instance_id: &str,
        status: &str,
        binding: Value,
        timestamp: &str,
    ) -> Result<(), StrategyRuntimeStoreError> {
        self.inner
            .seed_instance_with_binding(instance_id, status, binding, timestamp)
    }

    pub fn list_instances(&self) -> Result<Vec<StoredRuntimeInstance>, StrategyRuntimeStoreError> {
        self.inner.list_instances()
    }

    pub fn get_instance(
        &self,
        instance_id: &str,
    ) -> Result<Option<StoredRuntimeInstance>, StrategyRuntimeStoreError> {
        self.inner.get_instance(instance_id)
    }

    pub fn list_log_events(
        &self,
        instance_id: &str,
    ) -> Result<Vec<StoredStrategyLogEvent>, StrategyRuntimeStoreError> {
        self.inner.list_log_events(instance_id)
    }

    pub fn list_audit_events(
        &self,
        instance_id: &str,
    ) -> Result<Vec<StoredStrategyAuditEvent>, StrategyRuntimeStoreError> {
        self.inner.list_audit_events(instance_id)
    }

    pub fn update_status(
        &self,
        instance_id: &str,
        new_status: &str,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        self.inner.update_status(instance_id, new_status, timestamp)
    }

    pub fn update_binding(
        &self,
        instance_id: &str,
        binding: Value,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        self.inner.update_binding(instance_id, binding, timestamp)
    }

    pub fn update_risk(
        &self,
        instance_id: &str,
        risk: Value,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        self.inner.update_risk(instance_id, risk, timestamp)
    }

    pub fn delete_instance(
        &self,
        instance_id: &str,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        self.inner.delete_instance(instance_id, timestamp)
    }

    pub fn refresh_definition(
        &self,
        instance_id: &str,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        self.inner.refresh_definition(instance_id, timestamp)
    }
}
