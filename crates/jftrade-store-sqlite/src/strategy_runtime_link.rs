use rusqlite::{TransactionBehavior, params};
use serde_json::Value;

use crate::strategy_definition::StoredStrategyDefinition;

use crate::strategy_runtime::{StrategyRuntimeStore, StrategyRuntimeStoreError};
use crate::strategy_runtime_records::{
    decode_instance, instance_payload, strategy_timestamp_millis, validate_rfc3339_timestamp,
};

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

impl StrategyRuntimeStore {
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

pub(super) fn binding_definition_id(binding: &Value) -> Option<&str> {
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

pub(super) fn apply_definition_binding(
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
