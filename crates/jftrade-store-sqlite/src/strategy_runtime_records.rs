use rusqlite::{Connection, OptionalExtension, params};
use serde_json::{Value, json};
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use crate::strategy_runtime::{StoredRuntimeInstance, StrategyRuntimeStoreError};

pub(crate) type RuntimeObservationRow = (
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

pub(crate) fn get_instance_query(
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

pub(crate) fn decode_instance(
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
    let runtime_risk_revision = object
        .get("runtimeRiskRevision")
        .and_then(Value::as_i64)
        .unwrap_or(0);
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
        runtime_risk_revision,
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

pub(crate) fn instance_payload(instance: &StoredRuntimeInstance) -> Value {
    json!({
        "binding": instance.binding,
        "runtimeRisk": instance.runtime_risk,
        "runtimeRiskRevision": instance.runtime_risk_revision,
        "definitionRevision": instance.definition_revision,
        "runtimeActive": instance.runtime_active,
        "deleted": instance.deleted,
        "createdAt": instance.created_at,
        "definitionId": instance.definition_id,
        "definitionName": instance.definition_name,
        "definitionVersion": instance.definition_version,
    })
}

pub(crate) fn validate_rfc3339_timestamp(timestamp: &str) -> Result<(), StrategyRuntimeStoreError> {
    OffsetDateTime::parse(timestamp, &Rfc3339)
        .map(|_| ())
        .map_err(|error| {
            StrategyRuntimeStoreError::Incompatible(format!(
                "invalid RFC3339 timestamp {timestamp:?}: {error}"
            ))
        })
}

pub(crate) fn strategy_timestamp_millis(timestamp: &str) -> Result<i64, StrategyRuntimeStoreError> {
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

pub(crate) fn observation_timestamp(
    value: Option<i64>,
) -> Result<Option<String>, StrategyRuntimeStoreError> {
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
