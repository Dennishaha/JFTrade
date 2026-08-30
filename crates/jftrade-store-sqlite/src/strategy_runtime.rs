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
    pub created_at: Option<String>,
    pub definition_id: Option<String>,
    pub definition_name: Option<String>,
    pub definition_version: Option<String>,
}

/// Persisted runtime observation for a strategy instance.
///
/// The observation table is written by the runtime worker and is deliberately
/// kept separate from the catalog operation payload.  Production status
/// projections must read this record instead of manufacturing empty values.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct StoredRuntimeObservation {
    pub instance_id: String,
    pub actual_status: String,
    pub active_symbols: Vec<String>,
    pub last_closed_kline_at: Option<String>,
    pub last_signal_at: Option<String>,
    pub last_order_at: Option<String>,
    pub last_error_at: Option<String>,
    pub last_error: Option<String>,
    pub updated_at: Option<String>,
}

/// Result of applying one definition version to its linked, stopped runtime
/// instances. The store computes this result inside the same transaction as
/// the catalog updates, so callers never observe a partially applied batch.
#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct LinkedDefinitionApplyResult {
    pub total_linked: usize,
    pub applied: Vec<String>,
    pub already_latest: Vec<String>,
    pub skipped_busy: Vec<String>,
}

type RuntimeObservationRow = (
    String,
    String,
    String,
    Option<i64>,
    Option<i64>,
    Option<i64>,
    Option<i64>,
    Option<String>,
    Option<i64>,
);

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
    StoredStrategyDefinition, StrategyDefinitionStore, StrategyDefinitionStoreError,
    StrategyStoreInner,
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

    pub fn get_observation(
        &self,
        instance_id: &str,
    ) -> Result<Option<StoredRuntimeObservation>, StrategyRuntimeStoreError> {
        let connection = self.lock()?;
        let row: Option<RuntimeObservationRow> = connection
            .query_row(
                "SELECT instance_id, actual_status_snapshot, active_symbols_json,
                        last_closed_kline_at_ms, last_signal_at_ms, last_order_at_ms,
                        last_error_at_ms, last_error, updated_at_ms
                 FROM strategy_runtime_observations WHERE instance_id = ?1",
                params![instance_id],
                |row| {
                    Ok((
                        row.get(0)?,
                        row.get(1)?,
                        row.get(2)?,
                        row.get(3)?,
                        row.get(4)?,
                        row.get(5)?,
                        row.get(6)?,
                        row.get(7)?,
                        row.get(8)?,
                    ))
                },
            )
            .optional()
            .map_err(StrategyRuntimeStoreError::Query)?;

        row.map(|(
            instance_id,
            actual_status,
            active_symbols_json,
            last_closed_kline_at_ms,
            last_signal_at_ms,
            last_order_at_ms,
            last_error_at_ms,
            last_error,
            updated_at_ms,
        )| {
            let active_symbols = serde_json::from_str::<Vec<String>>(&active_symbols_json)
                .map_err(|error| {
                    StrategyRuntimeStoreError::Incompatible(format!(
                        "strategy runtime observation {instance_id:?} contains invalid active symbols JSON: {error}"
                    ))
                })?;
            Ok(StoredRuntimeObservation {
                instance_id,
                actual_status,
                active_symbols,
                last_closed_kline_at: observation_timestamp(last_closed_kline_at_ms)?,
                last_signal_at: observation_timestamp(last_signal_at_ms)?,
                last_order_at: observation_timestamp(last_order_at_ms)?,
                last_error_at: observation_timestamp(last_error_at_ms)?,
                last_error: last_error.and_then(|value| {
                    let value = value.trim().to_owned();
                    (!value.is_empty()).then_some(value)
                }),
                updated_at: observation_timestamp(updated_at_ms)?,
            })
        })
        .transpose()
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

    /// Atomically records the worker's latest observation.  Runtime workers
    /// use this instead of mutating the catalog payload, keeping status and
    /// liveness projections durable across process restarts.
    pub fn update_observation(
        &self,
        instance_id: &str,
        actual_status: &str,
        active_symbols: &[String],
        last_error: Option<&str>,
        updated_at_ms: i64,
    ) -> Result<(), StrategyRuntimeStoreError> {
        self.update_observation_with_events(
            instance_id,
            actual_status,
            active_symbols,
            last_error,
            None,
            None,
            None,
            updated_at_ms,
        )
    }

    /// Persist a worker observation together with the latest market/signal/order
    /// timestamps.  The event timestamps are monotonic projections: a worker
    /// may omit an event on a heartbeat, but it must never erase a timestamp
    /// already recovered from a previous process invocation.
    #[allow(clippy::too_many_arguments)]
    pub fn update_observation_with_events(
        &self,
        instance_id: &str,
        actual_status: &str,
        active_symbols: &[String],
        last_error: Option<&str>,
        last_closed_kline_at_ms: Option<i64>,
        last_signal_at_ms: Option<i64>,
        last_order_at_ms: Option<i64>,
        updated_at_ms: i64,
    ) -> Result<(), StrategyRuntimeStoreError> {
        let connection = self.lock()?;
        let symbols = serde_json::to_string(active_symbols).map_err(|error| {
            StrategyRuntimeStoreError::Incompatible(format!("encode active symbols: {error}"))
        })?;
        connection
            .execute(
                "INSERT INTO strategy_runtime_observations
                    (instance_id, actual_status_snapshot, active_symbols_json,
                     last_closed_kline_at_ms, last_signal_at_ms, last_order_at_ms,
                     last_error, updated_at_ms)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
                 ON CONFLICT(instance_id) DO UPDATE SET
                    actual_status_snapshot = excluded.actual_status_snapshot,
                    active_symbols_json = excluded.active_symbols_json,
                    last_closed_kline_at_ms = COALESCE(excluded.last_closed_kline_at_ms,
                        strategy_runtime_observations.last_closed_kline_at_ms),
                    last_signal_at_ms = COALESCE(excluded.last_signal_at_ms,
                        strategy_runtime_observations.last_signal_at_ms),
                    last_order_at_ms = COALESCE(excluded.last_order_at_ms,
                        strategy_runtime_observations.last_order_at_ms),
                    last_error = excluded.last_error,
                    last_error_at_ms = CASE WHEN excluded.last_error IS NULL OR excluded.last_error = '' THEN strategy_runtime_observations.last_error_at_ms ELSE excluded.updated_at_ms END,
                    updated_at_ms = excluded.updated_at_ms",
                params![
                    instance_id,
                    actual_status,
                    symbols,
                    last_closed_kline_at_ms,
                    last_signal_at_ms,
                    last_order_at_ms,
                    last_error.unwrap_or_default(),
                    updated_at_ms,
                ],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(())
    }

    /// Append a worker diagnostic to the durable strategy activity stream.
    /// Callers are expected to pass a validated instance id; SQLite foreign
    /// key enforcement remains the source of truth for malformed ids.
    pub fn append_log_event(
        &self,
        instance_id: &str,
        raw: &str,
        level: &str,
        at_ms: i64,
    ) -> Result<(), StrategyRuntimeStoreError> {
        if instance_id.trim().is_empty() || raw.trim().is_empty() {
            return Err(StrategyRuntimeStoreError::Validation(
                "strategy log instance and message are required".to_owned(),
            ));
        }
        let connection = self.lock()?;
        connection
            .execute(
                "INSERT INTO strategy_log_events (instance_id, at_ms, raw, level, source)
                 VALUES (?1, ?2, ?3, ?4, 'rust-production-runtime')",
                params![instance_id, at_ms, raw, level],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(())
    }

    /// Append a state transition/audit diagnostic using the same durable
    /// stream read by the strategy activity endpoints.
    pub fn append_audit_event(
        &self,
        instance_id: &str,
        kind: &str,
        detail: &str,
        at_ms: i64,
    ) -> Result<(), StrategyRuntimeStoreError> {
        if instance_id.trim().is_empty() || kind.trim().is_empty() {
            return Err(StrategyRuntimeStoreError::Validation(
                "strategy audit instance and kind are required".to_owned(),
            ));
        }
        let connection = self.lock()?;
        connection
            .execute(
                "INSERT INTO strategy_audit_events (instance_id, kind, detail, at_ms)
                 VALUES (?1, ?2, ?3, ?4)",
                params![instance_id, kind, detail, at_ms],
            )
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(())
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

        let payload = instance_payload(&instance);

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

    /// Apply a definition's current version to every linked stopped instance
    /// in one immediate SQLite transaction. Busy instances are deliberately
    /// skipped (matching the Go lifecycle contract); malformed payloads or a
    /// concurrent CAS miss abort the whole batch and leave prior rows intact.
    pub fn apply_definition_to_linked(
        &self,
        definition: &StoredStrategyDefinition,
        timestamp: &str,
    ) -> Result<LinkedDefinitionApplyResult, StrategyRuntimeStoreError> {
        validate_rfc3339_timestamp(timestamp)?;
        let definition_id = definition.id.trim();
        if definition_id.is_empty() {
            return Err(StrategyRuntimeStoreError::Validation(
                "strategy definition id is required".to_owned(),
            ));
        }
        let timestamp_ms = strategy_timestamp_millis(timestamp)?;
        let mut connection = self.lock()?;
        let transaction = connection
            .transaction_with_behavior(TransactionBehavior::Immediate)
            .map_err(StrategyRuntimeStoreError::Query)?;
        let mut statement = transaction
            .prepare(
                "SELECT operation_id, plugin_id, status, updated_at, payload_json
                 FROM strategy_catalog_operations
                 ORDER BY operation_id ASC",
            )
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
        let mut linked = Vec::new();
        for row in rows {
            let (id, plugin_id, status, updated_at, payload_json) =
                row.map_err(StrategyRuntimeStoreError::Query)?;
            let instance = decode_instance(id, plugin_id, status, updated_at, &payload_json)?;
            let linked_id = instance
                .definition_id
                .as_deref()
                .or_else(|| binding_definition_id(&instance.binding));
            if !instance.deleted && linked_id == Some(definition_id) {
                linked.push(instance);
            }
        }
        drop(statement);

        let mut result = LinkedDefinitionApplyResult {
            total_linked: linked.len(),
            ..LinkedDefinitionApplyResult::default()
        };
        for mut instance in linked {
            if instance.runtime_active || !instance.status.eq_ignore_ascii_case("STOPPED") {
                result.skipped_busy.push(instance.id);
                continue;
            }
            if instance
                .definition_version
                .as_deref()
                .is_some_and(|version| version.trim() == definition.version.trim())
            {
                result.already_latest.push(instance.id);
                continue;
            }

            let expected_updated_at = instance.updated_at.clone();
            instance.definition_revision =
                instance.definition_revision.checked_add(1).ok_or_else(|| {
                    StrategyRuntimeStoreError::Incompatible(format!(
                        "strategy instance {:?} definition revision overflow",
                        instance.id
                    ))
                })?;
            instance.definition_id = Some(definition_id.to_owned());
            instance.definition_name = Some(definition.name.trim().to_owned());
            instance.definition_version = Some(definition.version.trim().to_owned());
            apply_definition_binding(&mut instance.binding, definition)?;
            instance.updated_at = timestamp.to_owned();
            let payload = instance_payload(&instance);
            let changed = transaction
                .execute(
                    "UPDATE strategy_catalog_operations
                     SET updated_at = ?1, payload_json = ?2
                     WHERE operation_id = ?3 AND updated_at = ?4 AND status = ?5",
                    params![
                        timestamp,
                        payload.to_string(),
                        instance.id,
                        expected_updated_at,
                        instance.status,
                    ],
                )
                .map_err(StrategyRuntimeStoreError::Query)?;
            if changed != 1 {
                return Err(StrategyRuntimeStoreError::Conflict);
            }
            transaction
                .execute(
                    "INSERT INTO strategy_audit_events (instance_id, kind, detail, at_ms)
                     VALUES (?1, 'definition.refreshed', ?2, ?3)",
                    params![
                        instance.id,
                        format!(
                            "refreshed strategy definition {} to v{}",
                            definition_id,
                            definition.version.trim()
                        ),
                        timestamp_ms,
                    ],
                )
                .map_err(StrategyRuntimeStoreError::Query)?;
            result.applied.push(instance.id);
        }
        transaction
            .commit()
            .map_err(StrategyRuntimeStoreError::Query)?;
        Ok(result)
    }
}

fn binding_definition_id(binding: &Value) -> Option<&str> {
    binding
        .as_object()
        .and_then(|object| {
            object
                .get("definitionId")
                .or_else(|| object.get("strategyId"))
        })
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
}

fn apply_definition_binding(
    binding: &mut Value,
    definition: &StoredStrategyDefinition,
) -> Result<(), StrategyRuntimeStoreError> {
    let object = binding.as_object_mut().ok_or_else(|| {
        StrategyRuntimeStoreError::Incompatible(
            "linked strategy instance binding must be a JSON object".to_owned(),
        )
    })?;
    object.insert(
        "definitionId".to_owned(),
        Value::String(definition.id.trim().to_owned()),
    );
    object.insert(
        "definitionName".to_owned(),
        Value::String(definition.name.trim().to_owned()),
    );
    object.insert(
        "definitionVersion".to_owned(),
        Value::String(definition.version.trim().to_owned()),
    );
    if !definition.script.trim().is_empty() {
        object.insert(
            "script".to_owned(),
            Value::String(definition.script.clone()),
        );
    }
    if !definition.symbol.trim().is_empty() {
        object.insert(
            "symbol".to_owned(),
            Value::String(definition.symbol.clone()),
        );
        object.insert(
            "symbols".to_owned(),
            Value::Array(vec![Value::String(definition.symbol.clone())]),
        );
    }
    if !definition.interval.trim().is_empty() {
        object.insert(
            "interval".to_owned(),
            Value::String(definition.interval.clone()),
        );
    }
    Ok(())
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
        .unwrap_or_else(|| status.eq_ignore_ascii_case("RUNNING"));
    let deleted = object
        .get("deleted")
        .and_then(Value::as_bool)
        .unwrap_or_else(|| status.eq_ignore_ascii_case("DELETED"));
    let optional_string = |key: &str| {
        object
            .get(key)
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(ToOwned::to_owned)
    };
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
        created_at: optional_string("createdAt"),
        definition_id: optional_string("definitionId"),
        definition_name: optional_string("definitionName"),
        definition_version: optional_string("definitionVersion"),
    })
}

fn instance_payload(instance: &StoredRuntimeInstance) -> Value {
    json!({
        "binding": instance.binding,
        "runtimeRisk": instance.runtime_risk,
        "definitionRevision": instance.definition_revision,
        "runtimeActive": instance.runtime_active,
        "deleted": instance.deleted,
        "createdAt": instance.created_at,
        "definitionId": instance.definition_id,
        "definitionName": instance.definition_name,
        "definitionVersion": instance.definition_version,
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

fn strategy_timestamp_millis(timestamp: &str) -> Result<i64, StrategyRuntimeStoreError> {
    let parsed = OffsetDateTime::parse(timestamp, &Rfc3339).map_err(|error| {
        StrategyRuntimeStoreError::Incompatible(format!(
            "invalid RFC3339 timestamp {timestamp:?}: {error}"
        ))
    })?;
    i64::try_from(parsed.unix_timestamp_nanos() / 1_000_000).map_err(|_| {
        StrategyRuntimeStoreError::Incompatible(
            "strategy definition timestamp is outside SQLite millisecond range".to_owned(),
        )
    })
}

fn observation_timestamp(value: Option<i64>) -> Result<Option<String>, StrategyRuntimeStoreError> {
    let Some(value) = value.filter(|value| *value > 0) else {
        return Ok(None);
    };
    let timestamp = OffsetDateTime::from_unix_timestamp_nanos(i128::from(value) * 1_000_000)
        .map_err(|error| {
            StrategyRuntimeStoreError::Incompatible(format!(
                "strategy runtime observation timestamp is out of range: {error}"
            ))
        })?;
    timestamp.format(&Rfc3339).map(Some).map_err(|error| {
        StrategyRuntimeStoreError::Incompatible(format!(
            "format strategy runtime observation timestamp: {error}"
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
        self.inner.seed_instance_with_definition(
            instance_id,
            status,
            binding,
            definition_id,
            definition_name,
            definition_version,
            timestamp,
        )
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

    pub fn get_observation(
        &self,
        instance_id: &str,
    ) -> Result<Option<StoredRuntimeObservation>, StrategyRuntimeStoreError> {
        self.inner.get_observation(instance_id)
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
