use std::path::Path;
use std::sync::{Arc, MutexGuard};

use jftrade_owner_lock::WriterLeaseError;
use rusqlite::{Connection, TransactionBehavior, params};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use thiserror::Error;

use crate::schema_manifest::SchemaManifestError;
use crate::strategy_definition::{
    StrategyDefinitionStore, StrategyDefinitionStoreError, StrategyStoreInner,
};

pub use crate::strategy_runtime_link::LinkedDefinitionApplyResult;
pub use crate::strategy_runtime_observation::{
    StoredRuntimeObservation, StoredStrategyAuditEvent, StoredStrategyLogEvent,
};
pub use crate::strategy_runtime_test_cutover::StrategyRuntimeTestCutoverStore;

use crate::strategy_runtime_records::{
    decode_instance, get_instance_query, instance_payload, strategy_timestamp_millis,
    validate_rfc3339_timestamp,
};

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
    #[serde(default)]
    pub runtime_risk_revision: i64,
    pub definition_revision: i64,
    pub runtime_active: bool,
    pub deleted: bool,
    pub updated_at: String,
    pub created_at: Option<String>,
    pub definition_id: Option<String>,
    pub definition_name: Option<String>,
    pub definition_version: Option<String>,
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

    pub(crate) fn lock(&self) -> Result<MutexGuard<'_, Connection>, StrategyRuntimeStoreError> {
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
        self.seed_instance_with_metadata(instance_id, status, binding, None, None, None, timestamp)
    }

    #[allow(clippy::too_many_arguments)]
    pub fn seed_instance_with_definition(
        &self,
        instance_id: &str,
        status: &str,
        binding: Value,
        definition_id: &str,
        definition_name: &str,
        definition_version: &str,
        timestamp: &str,
    ) -> Result<(), StrategyRuntimeStoreError> {
        self.seed_instance_with_metadata(
            instance_id,
            status,
            binding,
            Some(definition_id),
            Some(definition_name),
            Some(definition_version),
            timestamp,
        )
    }

    #[allow(clippy::too_many_arguments)]
    fn seed_instance_with_metadata(
        &self,
        instance_id: &str,
        status: &str,
        binding: Value,
        definition_id: Option<&str>,
        definition_name: Option<&str>,
        definition_version: Option<&str>,
        timestamp: &str,
    ) -> Result<(), StrategyRuntimeStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(StrategyRuntimeStoreError::Query)?;
        let is_running = status.eq_ignore_ascii_case("RUNNING");
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
            "createdAt": timestamp,
            "definitionId": definition_id,
            "definitionName": definition_name,
            "definitionVersion": definition_version,
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

    pub fn update_status(
        &self,
        instance_id: &str,
        new_status: &str,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let timestamp_ms = strategy_timestamp_millis(timestamp)?;
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
        instance.runtime_active = new_status.eq_ignore_ascii_case("RUNNING");
        instance.updated_at = timestamp.to_owned();

        let payload = instance_payload(&instance);

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

        let event_kind = if new_status.eq_ignore_ascii_case("RUNNING") {
            "STARTED"
        } else if new_status.eq_ignore_ascii_case("STOPPED") {
            "STOPPED"
        } else if new_status.eq_ignore_ascii_case("PAUSED") {
            "PAUSED"
        } else {
            "STATUS_CHANGE"
        };
        transaction
            .execute(
                "INSERT INTO strategy_audit_events (instance_id, kind, detail, at_ms)
                 VALUES (?1, ?2, '', ?3)",
                params![instance_id, event_kind, timestamp_ms],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        transaction
            .commit()
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(instance)
    }

    pub fn update_status_cas(
        &self,
        instance_id: &str,
        expected_statuses: &[&str],
        new_status: &str,
        timestamp: &str,
    ) -> Result<StoredRuntimeInstance, StrategyRuntimeStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let timestamp_ms = strategy_timestamp_millis(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(StrategyRuntimeStoreError::Query)?;

        let mut instance = get_instance_query(&transaction, instance_id)?
            .ok_or(StrategyRuntimeStoreError::NotFound)?;

        if instance.deleted {
            return Err(StrategyRuntimeStoreError::NotFound);
        }

        let current_upper = instance.status.to_ascii_uppercase();
        let matches_expected = expected_statuses
            .iter()
            .any(|s| s.eq_ignore_ascii_case(&current_upper));
        if !matches_expected {
            return Err(StrategyRuntimeStoreError::Conflict);
        }

        let expected_updated_at = instance.updated_at.clone();
        instance.status = new_status.to_owned();
        instance.runtime_active = new_status.eq_ignore_ascii_case("RUNNING");
        instance.updated_at = timestamp.to_owned();

        let payload = instance_payload(&instance);

        let in_clause = expected_statuses
            .iter()
            .map(|s| format!("'{}'", s.to_ascii_uppercase()))
            .collect::<Vec<_>>()
            .join(", ");
        let query = format!(
            "UPDATE strategy_catalog_operations
             SET status = ?1, updated_at = ?2, payload_json = ?3
             WHERE operation_id = ?4 AND updated_at = ?5 AND status IN ({in_clause})"
        );

        let rows = transaction
            .execute(
                &query,
                params![
                    new_status,
                    timestamp,
                    payload.to_string(),
                    instance_id,
                    expected_updated_at
                ],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        if rows == 0 {
            return Err(StrategyRuntimeStoreError::Conflict);
        }

        transaction
            .execute(
                "UPDATE strategy_runtime_observations
                 SET actual_status_snapshot = ?1
                 WHERE instance_id = ?2",
                params![new_status, instance_id],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        let event_kind = if new_status.eq_ignore_ascii_case("RUNNING") {
            "STARTED"
        } else if new_status.eq_ignore_ascii_case("STOPPED") {
            "STOPPED"
        } else if new_status.eq_ignore_ascii_case("PAUSED") {
            "PAUSED"
        } else {
            "STATUS_CHANGE"
        };
        transaction
            .execute(
                "INSERT INTO strategy_audit_events (instance_id, kind, detail, at_ms)
                 VALUES (?1, ?2, '', ?3)",
                params![instance_id, event_kind, timestamp_ms],
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

        let expected_updated_at = instance.updated_at.clone();
        instance.binding = binding;
        instance.updated_at = timestamp.to_owned();

        let payload = instance_payload(&instance);

        let rows = transaction
            .execute(
                "UPDATE strategy_catalog_operations
                 SET updated_at = ?1, payload_json = ?2
                 WHERE operation_id = ?3 AND updated_at = ?4 AND status = 'STOPPED'",
                params![
                    timestamp,
                    payload.to_string(),
                    instance_id,
                    expected_updated_at
                ],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        if rows == 0 {
            return Err(StrategyRuntimeStoreError::Conflict);
        }

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

        let status_upper = instance.status.to_ascii_uppercase();
        if !matches!(
            status_upper.as_str(),
            "RUNNING" | "STOPPED" | "PAUSED" | "FAILED"
        ) {
            return Err(StrategyRuntimeStoreError::Conflict);
        }

        let expected_revision = risk
            .get("expectedRevision")
            .or_else(|| risk.get("expected_revision"))
            .and_then(Value::as_i64);
        if expected_revision.is_some_and(|rev| rev != instance.runtime_risk_revision) {
            return Err(StrategyRuntimeStoreError::Conflict);
        }

        let expected_updated_at = instance.updated_at.clone();
        instance.runtime_risk = risk;
        instance.runtime_risk_revision += 1;
        instance.updated_at = timestamp.to_owned();

        let payload = instance_payload(&instance);

        let rows = transaction
            .execute(
                "UPDATE strategy_catalog_operations
                 SET updated_at = ?1, payload_json = ?2
                 WHERE operation_id = ?3 AND updated_at = ?4",
                params![
                    timestamp,
                    payload.to_string(),
                    instance_id,
                    expected_updated_at
                ],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        if rows == 0 {
            return Err(StrategyRuntimeStoreError::Conflict);
        }

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

        let status_upper = instance.status.to_ascii_uppercase();
        if !matches!(status_upper.as_str(), "STOPPED" | "FAILED") {
            return Err(StrategyRuntimeStoreError::Conflict);
        }

        let expected_updated_at = instance.updated_at.clone();
        instance.deleted = true;
        instance.runtime_active = false;
        instance.updated_at = timestamp.to_owned();

        let payload = instance_payload(&instance);

        let rows = transaction
            .execute(
                "UPDATE strategy_catalog_operations
                 SET status = 'DELETED', updated_at = ?1, payload_json = ?2
                 WHERE operation_id = ?3 AND updated_at = ?4 AND status IN ('STOPPED', 'FAILED')",
                params![
                    timestamp,
                    payload.to_string(),
                    instance_id,
                    expected_updated_at
                ],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;

        if rows == 0 {
            return Err(StrategyRuntimeStoreError::Conflict);
        }

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

        let payload = instance_payload(&instance);

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
