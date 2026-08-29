//! Strategy and Research Presets production ports.

use std::sync::Arc;
use std::thread;

use jftrade_research::normalize_definition_v2;
use jftrade_store_sqlite::{
    ResearchPresetMutation, ResearchPresetStore, ResearchPresetStoreError, StrategyDefinitionStore,
    StrategyDefinitionStoreError, StrategyRuntimeStore,
};
use serde_json::{Value, json};
use jftrade_integration_marketdata_helper::{HelperClient, HttpAdapterError};

use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_research_preset_write_port::{
    ResearchPresetWriteMutation, ResearchPresetWritePort, ResearchPresetWritePortError,
};
use crate::product::product_research_screen_write_port::{
    ResearchScreenWritePort, ResearchScreenWritePortError, ResearchScreenWriteQuery,
};
use crate::product::product_strategy_definition_write_port::{
    StrategyDefinitionWriteInput, StrategyDefinitionWriteOperation, StrategyDefinitionWritePort,
    StrategyDefinitionWritePortError,
};
use crate::product::product_strategy_runtime_write_port::{
    StrategyRuntimeWriteInput, StrategyRuntimeWriteOperation, StrategyRuntimeWritePort,
    StrategyRuntimeWritePortError,
};
use crate::product::strategy_pine::{
    StrategyPineAnalyzeInput, StrategyPineAnalyzeSnapshotError, StrategyPineAnalyzeSnapshotPort,
};
use crate::product::{
    ResearchPresetReadSnapshotError, ResearchPresetReadSnapshotPort,
    ResearchReadSnapshotError, ResearchReadSnapshotPort, StrategyDefinitionPreview,
    StrategyDefinitionSnapshotError, StrategyDefinitionSnapshotPort, StrategyReadSnapshotError,
    StrategyReadSnapshotPort, StrategyRuntimeStatusPort, StrategyRuntimeSummary,
};

// ---------------------------------------------------------------------------
// Strategy Definition
// ---------------------------------------------------------------------------

#[derive(Debug)]
pub(crate) struct ProductionStrategyDefinitionPort {
    pub(crate) store: Arc<StrategyDefinitionStore>,
}

impl StrategyDefinitionSnapshotPort for ProductionStrategyDefinitionPort {
    fn list(&self) -> Result<Vec<Value>, StrategyDefinitionSnapshotError> {
        let definitions = self
            .store
            .list_definitions(false)
            .map_err(|e| StrategyDefinitionSnapshotError::Unavailable(e.to_string()))?;
        definitions
            .into_iter()
            .map(|d| {
                serde_json::to_value(&d).map_err(|error| {
                    StrategyDefinitionSnapshotError::Unavailable(error.to_string())
                })
            })
            .collect::<Result<Vec<_>, _>>()
    }

    fn get(
        &self,
        definition_id: &str,
        _preview: &StrategyDefinitionPreview,
    ) -> Result<Option<Value>, StrategyDefinitionSnapshotError> {
        let def = self
            .store
            .get_definition(definition_id, true)
            .map_err(|e| StrategyDefinitionSnapshotError::Unavailable(e.to_string()))?;
        def.map(|d| {
            serde_json::to_value(&d)
                .map_err(|error| StrategyDefinitionSnapshotError::Unavailable(error.to_string()))
        })
        .transpose()
    }

    fn versions(
        &self,
        definition_id: &str,
    ) -> Result<Option<Vec<Value>>, StrategyDefinitionSnapshotError> {
        let versions = self
            .store
            .list_versions(definition_id)
            .map_err(|e| StrategyDefinitionSnapshotError::Unavailable(e.to_string()))?;
        Ok(Some(
            versions
                .into_iter()
                .map(|v| {
                    serde_json::to_value(&v).map_err(|error| {
                        StrategyDefinitionSnapshotError::Unavailable(error.to_string())
                    })
                })
                .collect::<Result<Vec<_>, _>>()?,
        ))
    }

    fn version(
        &self,
        definition_id: &str,
        version: &str,
    ) -> Result<Option<Value>, StrategyDefinitionSnapshotError> {
        let ver = self
            .store
            .get_version(definition_id, version)
            .map_err(|e| StrategyDefinitionSnapshotError::Unavailable(e.to_string()))?;
        ver.map(|v| {
            serde_json::to_value(&v)
                .map_err(|error| StrategyDefinitionSnapshotError::Unavailable(error.to_string()))
        })
        .transpose()
    }
}

impl StrategyDefinitionWritePort for ProductionStrategyDefinitionPort {
    fn mutate(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        let timestamp = time::OffsetDateTime::now_utc()
            .format(&time::format_description::well_known::Rfc3339)
            .unwrap_or_default();

        match input.operation {
            StrategyDefinitionWriteOperation::Create => {
                let Some(Value::Object(def)) = input.definition.as_ref() else {
                    return Err(StrategyDefinitionWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "invalid definition payload".to_owned(),
                    });
                };
                let name = def
                    .get("name")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| StrategyDefinitionWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "strategy name is required".to_owned(),
                    })?;
                let id = def
                    .get("id")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .map(ToOwned::to_owned)
                    .unwrap_or_else(generate_strategy_id);
                let description = def
                    .get("description")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .unwrap_or_default()
                    .to_owned();
                let runtime = def
                    .get("runtime")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .unwrap_or("pine-pinets")
                    .to_owned();
                let source_format = def
                    .get("sourceFormat")
                    .or_else(|| def.get("source_format"))
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .unwrap_or("pine-v6")
                    .to_owned();
                let symbol = def
                    .get("symbol")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .unwrap_or_default()
                    .to_owned();
                let interval = def
                    .get("interval")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .unwrap_or_default()
                    .to_owned();
                let script = def
                    .get("script")
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_owned();
                let visual_model_json = if let Some(vm) = def.get("visualModelJson").and_then(Value::as_str) {
                    vm.to_owned()
                } else if let Some(vm) = def.get("visualModel") {
                    serde_json::to_string(vm).unwrap_or_else(|_| "{}".to_owned())
                } else {
                    "{}".to_owned()
                };
                let version = def
                    .get("version")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .unwrap_or("0.1.0")
                    .to_owned();
                let stored = self
                    .store
                    .save_definition(
                        jftrade_store_sqlite::StoredStrategyDefinition {
                            id,
                            name: name.to_owned(),
                            version,
                            description,
                            runtime,
                            source_format,
                            symbol,
                            interval,
                            script,
                            visual_model_json,
                            created_at: timestamp.clone(),
                            updated_at: timestamp.clone(),
                            deleted_at: None,
                        },
                        &timestamp,
                    )
                    .map_err(map_strategy_store_error)?;
                serde_json::to_value(&stored).map_err(|error| {
                    StrategyDefinitionWritePortError::Failed {
                        status: 500,
                        code: "STRATEGY_DEFINITION_ERROR".to_owned(),
                        message: format!("encode stored strategy definition: {error}"),
                    }
                })
            }
            StrategyDefinitionWriteOperation::Update => {
                let definition_id = input
                    .definition_id
                    .as_deref()
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| StrategyDefinitionWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "invalid definition id".to_owned(),
                    })?;
                let existing = self
                    .store
                    .get_definition(definition_id, false)
                    .map_err(map_strategy_store_error)?
                    .ok_or_else(|| StrategyDefinitionWritePortError::Failed {
                        status: 404,
                        code: "NOT_FOUND".to_owned(),
                        message: "strategy resource not found".to_owned(),
                    })?;
                let Some(Value::Object(def)) = input.definition.as_ref() else {
                    return Err(StrategyDefinitionWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "invalid definition payload".to_owned(),
                    });
                };
                let name = if let Some(raw) = def.get("name") {
                    let trimmed = raw
                        .as_str()
                        .map(str::trim)
                        .filter(|s| !s.is_empty())
                        .ok_or_else(|| StrategyDefinitionWritePortError::Failed {
                            status: 400,
                            code: "BAD_REQUEST".to_owned(),
                            message: "strategy name is required".to_owned(),
                        })?;
                    trimmed.to_owned()
                } else {
                    existing.name.clone()
                };
                let description = def
                    .get("description")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .map(ToOwned::to_owned)
                    .unwrap_or(existing.description);
                let runtime = def
                    .get("runtime")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .map(ToOwned::to_owned)
                    .unwrap_or(existing.runtime);
                let source_format = def
                    .get("sourceFormat")
                    .or_else(|| def.get("source_format"))
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .map(ToOwned::to_owned)
                    .unwrap_or(existing.source_format);
                let symbol = def
                    .get("symbol")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .map(ToOwned::to_owned)
                    .unwrap_or(existing.symbol);
                let interval = def
                    .get("interval")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .map(ToOwned::to_owned)
                    .unwrap_or(existing.interval);
                let script = def
                    .get("script")
                    .and_then(Value::as_str)
                    .map(ToOwned::to_owned)
                    .unwrap_or(existing.script);
                let visual_model_json = if let Some(vm) = def.get("visualModelJson").and_then(Value::as_str) {
                    vm.to_owned()
                } else if let Some(vm) = def.get("visualModel") {
                    serde_json::to_string(vm).unwrap_or_else(|_| existing.visual_model_json.clone())
                } else {
                    existing.visual_model_json
                };
                let stored = self
                    .store
                    .save_definition(
                        jftrade_store_sqlite::StoredStrategyDefinition {
                            id: definition_id.to_owned(),
                            name,
                            version: existing.version,
                            description,
                            runtime,
                            source_format,
                            symbol,
                            interval,
                            script,
                            visual_model_json,
                            created_at: existing.created_at,
                            updated_at: timestamp.clone(),
                            deleted_at: None,
                        },
                        &timestamp,
                    )
                    .map_err(map_strategy_store_error)?;
                serde_json::to_value(&stored).map_err(|error| {
                    StrategyDefinitionWritePortError::Failed {
                        status: 500,
                        code: "STRATEGY_DEFINITION_ERROR".to_owned(),
                        message: format!("encode stored strategy definition: {error}"),
                    }
                })
            }
            StrategyDefinitionWriteOperation::Delete => {
                let definition_id = input
                    .definition_id
                    .as_deref()
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| StrategyDefinitionWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "invalid definition id".to_owned(),
                    })?;
                let deleted = self
                    .store
                    .delete_definition(definition_id, &timestamp)
                    .map_err(map_strategy_store_error)?;
                serde_json::to_value(&deleted).map_err(|error| {
                    StrategyDefinitionWritePortError::Failed {
                        status: 500,
                        code: "STRATEGY_DEFINITION_ERROR".to_owned(),
                        message: format!("encode deleted strategy definition: {error}"),
                    }
                })
            }
            StrategyDefinitionWriteOperation::ApplyLinkedInstances => {
                let definition_id = input
                    .definition_id
                    .as_deref()
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| StrategyDefinitionWritePortError::Failed {
                        status: 404,
                        code: "NOT_FOUND".to_owned(),
                        message: "resource not found".to_owned(),
                    })?;
                if self
                    .store
                    .get_definition(definition_id, false)
                    .map_err(map_strategy_store_error)?
                    .is_none()
                {
                    return Err(StrategyDefinitionWritePortError::Failed {
                        status: 404,
                        code: "NOT_FOUND".to_owned(),
                        message: "resource not found".to_owned(),
                    });
                }
                Err(StrategyDefinitionWritePortError::Unavailable(
                    "linked strategy runtime is not configured".to_owned(),
                ))
            }
            StrategyDefinitionWriteOperation::Instantiate => {
                let definition_id = input
                    .definition_id
                    .as_deref()
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .ok_or_else(|| StrategyDefinitionWritePortError::Failed {
                        status: 404,
                        code: "NOT_FOUND".to_owned(),
                        message: "resource not found".to_owned(),
                    })?;
                let current = self
                    .store
                    .get_definition(definition_id, false)
                    .map_err(map_strategy_store_error)?
                    .ok_or_else(|| StrategyDefinitionWritePortError::Failed {
                        status: 404,
                        code: "NOT_FOUND".to_owned(),
                        message: "resource not found".to_owned(),
                    })?;
                if let Some(message) = input.binding_error.as_deref() {
                    return Err(StrategyDefinitionWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: message.to_owned(),
                    });
                }
                let instance_id = generate_instance_id(definition_id);
                let binding = input.binding.clone().unwrap_or_else(|| json!({}));
                let runtime = StrategyRuntimeStore::from_definition_store(&self.store);
                runtime
                    .seed_instance_with_definition(
                        &instance_id,
                        "STOPPED",
                        binding.clone(),
                        definition_id,
                        &current.name,
                        &current.version,
                        &timestamp,
                    )
                    .map_err(|error| StrategyDefinitionWritePortError::Failed {
                        status: 400,
                        code: "STRATEGY_RUNTIME_ERROR".to_owned(),
                        message: error.to_string(),
                    })?;
                let def_val = serde_json::to_value(&current).unwrap_or_default();
                Ok(json!({
                    "id": instance_id,
                    "definitionId": definition_id,
                    "definitionVersion": current.version,
                    "definition": def_val,
                    "binding": binding,
                    "status": "STOPPED",
                }))
            }
        }
    }
}

fn generate_strategy_id() -> String {
    use std::sync::atomic::{AtomicU64, Ordering};
    static COUNTER: AtomicU64 = AtomicU64::new(1);
    let id = COUNTER.fetch_add(1, Ordering::Relaxed);
    let timestamp = time::OffsetDateTime::now_utc().unix_timestamp_nanos();
    format!("{timestamp:x}_{id}")
}

fn generate_instance_id(definition_id: &str) -> String {
    let timestamp = time::OffsetDateTime::now_utc();
    let format = time::format_description::parse_borrowed::<1>(
        "[year][month][day][hour][minute][second].[subsecond digits:9]",
    )
    .expect("valid strategy instance id format");
    let suffix = timestamp
        .format(&format)
        .unwrap_or_else(|_| generate_strategy_id());
    format!("{}-{}", definition_id.trim(), suffix)
}

fn map_strategy_store_error(error: StrategyDefinitionStoreError) -> StrategyDefinitionWritePortError {
    match error {
        StrategyDefinitionStoreError::NotFound => StrategyDefinitionWritePortError::Failed {
            status: 404,
            code: "NOT_FOUND".to_owned(),
            message: "strategy resource not found".to_owned(),
        },
        StrategyDefinitionStoreError::Conflict => StrategyDefinitionWritePortError::Failed {
            status: 409,
            code: "CONFLICT".to_owned(),
            message: "strategy state conflict".to_owned(),
        },
        StrategyDefinitionStoreError::DeleteGuard(message) => {
            StrategyDefinitionWritePortError::Failed {
                status: 400,
                code: "STRATEGY_INVALID".to_owned(),
                message,
            }
        }
        StrategyDefinitionStoreError::Validation(message) => {
            StrategyDefinitionWritePortError::Failed {
                status: 400,
                code: "STRATEGY_INVALID".to_owned(),
                message,
            }
        }
        other => StrategyDefinitionWritePortError::Failed {
            status: 500,
            code: "STRATEGY_FAILED".to_owned(),
            message: other.to_string(),
        },
    }
}

// ---------------------------------------------------------------------------
// Strategy Runtime
// ---------------------------------------------------------------------------

#[derive(Debug)]
pub(crate) struct ProductionStrategyRuntimePort {
    pub(crate) store: Arc<StrategyRuntimeStore>,
    pub(crate) definitions: Arc<StrategyDefinitionStore>,
}

impl StrategyReadSnapshotPort for ProductionStrategyRuntimePort {
    fn read(&self, path: &str, query: &str) -> Result<Option<Value>, StrategyReadSnapshotError> {
        if path == "/api/v1/strategies" {
                let instances = self
                    .store
                    .list_instances()
                    .map_err(|e| StrategyReadSnapshotError::Unavailable(e.to_string()))?;
                let items = instances
                    .into_iter()
                    .map(|instance| self.runtime_instance_wire(instance))
                    .collect::<Result<Vec<_>, _>>()?;
                return Ok(Some(Value::Array(items)));
        }
        let Some((instance_id, activity)) = strategy_activity_path(path) else {
            return Ok(None);
        };
        if self
            .store
            .get_instance(instance_id)
            .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?
            .is_none()
        {
            return Ok(None);
        }
        let query = parse_activity_query(query, activity)?;
        match activity {
            "logs" => self.logs(instance_id, &query).map(Some),
            "audit" => self.audit(instance_id, &query).map(Some),
            _ => Ok(None),
        }
    }
}

impl ProductionStrategyRuntimePort {
    fn runtime_instance_wire(
        &self,
        instance: jftrade_store_sqlite::StoredRuntimeInstance,
    ) -> Result<Value, StrategyReadSnapshotError> {
        let observation = self
            .store
            .get_observation(&instance.id)
            .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?;
        let mut object = serde_json::Map::new();
        object.insert("id".to_owned(), Value::String(instance.id.clone()));
        object.insert("status".to_owned(), Value::String(instance.status.clone()));
        object.insert("binding".to_owned(), instance.binding.clone());
        object.insert("runtimeRisk".to_owned(), instance.runtime_risk.clone());
        object.insert(
            "definitionRevision".to_owned(),
            Value::from(instance.definition_revision),
        );
        object.insert("runtimeActive".to_owned(), Value::Bool(instance.runtime_active));
        if !instance.plugin_id.is_empty() {
            object.insert("pluginId".to_owned(), Value::String(instance.plugin_id.clone()));
        }
        if let Some(created_at) = instance
            .created_at
            .clone()
            .or_else(|| (!instance.updated_at.is_empty()).then_some(instance.updated_at.clone()))
        {
            object.insert("createdAt".to_owned(), Value::String(created_at));
        }
        if !instance.updated_at.is_empty() {
            object.insert("updatedAt".to_owned(), Value::String(instance.updated_at.clone()));
        }
        // Older catalog rows may not have copied definition metadata into the
        // operation payload. Recover the identity from the persisted binding
        // before consulting the definition store; never invent a definition.
        let definition_id = instance
            .definition_id
            .clone()
            .or_else(|| binding_string_opt(&instance.binding, &["definitionId", "strategyId"]));
        if let Some(definition_id) = definition_id {
            let definition = self
                .definitions
                .get_definition(&definition_id, false)
                .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?;
            let definition_name = instance
                .definition_name
                .clone()
                .or_else(|| binding_string_opt(&instance.binding, &["definitionName", "strategyName"]))
                .unwrap_or_default();
            let definition_version = instance
                .definition_version
                .clone()
                .or_else(|| binding_string_opt(&instance.binding, &["definitionVersion", "version"]))
                .unwrap_or_default();
            object.insert(
                "definition".to_owned(),
                json!({
                    "strategyId": definition_id,
                    "name": definition
                        .as_ref()
                        .map(|item| item.name.clone())
                        .filter(|name| !name.is_empty())
                        .unwrap_or(definition_name),
                    "version": definition
                        .as_ref()
                        .map(|item| item.version.clone())
                        .filter(|version| !version.is_empty())
                        .unwrap_or(definition_version),
                }),
            );
            object.insert(
                "definitionSync".to_owned(),
                json!({
                    "definitionId": definition_id,
                    "appliedVersion": instance.definition_version.clone(),
                    "latestVersion": definition.as_ref().map(|item| item.version.clone()).unwrap_or_default(),
                    "isLatest": definition.as_ref().is_none_or(|item| Some(item.version.clone()) == instance.definition_version),
                    "canApplyLatest": false,
                }),
            );
        }
        let runtime = instance
            .binding
            .get("runtime")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty());
        let source_format = instance
            .binding
            .get("sourceFormat")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty());
        if let Some(runtime) = runtime.as_deref() {
            object.insert("runtime".to_owned(), Value::String(runtime.to_owned()));
        }
        if let Some(source_format) = source_format.as_deref() {
            object.insert("sourceFormat".to_owned(), Value::String(source_format.to_owned()));
        }
        object.insert(
            "startable".to_owned(),
            Value::Bool(runtime.is_some() && source_format.is_some()),
        );
        let mut params = serde_json::Map::new();
        if let Some(definition_id) = instance.definition_id.as_deref() {
            params.insert(
                "definitionId".to_owned(),
                Value::String(definition_id.to_owned()),
            );
        }
        if let Some(runtime) = runtime.as_deref() {
            params.insert("runtime".to_owned(), Value::String(runtime.to_owned()));
        }
        if let Some(source_format) = source_format.as_deref() {
            params.insert("sourceFormat".to_owned(), Value::String(source_format.to_owned()));
        }
        object.insert("params".to_owned(), Value::Object(params));
        let logs = self
            .store
            .list_log_events(&instance.id)
            .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?
            .into_iter()
            .take(20)
            .map(|event| event.raw)
            .collect::<Vec<_>>();
        object.insert("logs".to_owned(), Value::Array(logs.into_iter().map(Value::String).collect()));
        if let Some(observation) = observation {
            object.insert(
                "runtimeObservation".to_owned(),
                json!({
                    "actualStatus": observation.actual_status,
                    "activeSymbols": observation.active_symbols,
                    "lastClosedKlineAt": observation.last_closed_kline_at,
                    "lastSignalAt": observation.last_signal_at,
                    "lastOrderAt": observation.last_order_at,
                    "lastErrorAt": observation.last_error_at,
                    "lastError": observation.last_error,
                    "updatedAt": observation.updated_at,
                }),
            );
        }
        Ok(Value::Object(object))
    }

    fn logs(
        &self,
        instance_id: &str,
        query: &StrategyActivityQuery,
    ) -> Result<Value, StrategyReadSnapshotError> {
        let events = self
            .store
            .list_log_events(instance_id)
            .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?;
        let filtered = events
            .into_iter()
            .filter(|event| query.includes(event.at_ms))
            .filter(|event| query.selector.is_empty() || event.level == query.selector)
            .map(|event| event.raw)
            .collect::<Vec<_>>();
        let (logs, page) = page_values(filtered, query);
        Ok(json!({"instanceId": instance_id, "logs": logs, "page": page}))
    }

    fn audit(
        &self,
        instance_id: &str,
        query: &StrategyActivityQuery,
    ) -> Result<Value, StrategyReadSnapshotError> {
        let events = self
            .store
            .list_audit_events(instance_id)
            .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?;
        let filtered = events
            .into_iter()
            .filter(|event| query.includes(event.at_ms))
            .filter(|event| query.selector.is_empty() || event.kind == query.selector)
            .map(|event| {
                let at = timestamp_from_millis(event.at_ms)?;
                Ok(json!({
                    "instanceId": event.instance_id,
                    "kind": event.kind,
                    "detail": event.detail,
                    "at": at,
                }))
            })
            .collect::<Result<Vec<_>, StrategyReadSnapshotError>>()?;
        let (entries, page) = page_values(filtered, query);
        Ok(json!({"instanceId": instance_id, "entries": entries, "page": page}))
    }
}

#[derive(Debug)]
struct StrategyActivityQuery {
    limit: usize,
    offset: usize,
    selector: String,
    from_ms: Option<i64>,
    to_ms: Option<i64>,
}

impl StrategyActivityQuery {
    fn includes(&self, at_ms: i64) -> bool {
        self.from_ms.is_none_or(|from| at_ms >= from)
            && self.to_ms.is_none_or(|to| at_ms <= to)
    }
}

fn strategy_activity_path(path: &str) -> Option<(&str, &str)> {
    let suffix = path.strip_prefix("/api/v1/strategies/")?;
    let (instance_id, activity) = suffix.split_once('/')?;
    (!instance_id.is_empty() && matches!(activity, "logs" | "audit"))
        .then_some((instance_id, activity))
}

fn parse_activity_query(
    raw_query: &str,
    activity: &str,
) -> Result<StrategyActivityQuery, StrategyReadSnapshotError> {
    let invalid = || {
        StrategyReadSnapshotError::Invalid(format!("invalid {activity} query"))
    };
    let mut query = StrategyActivityQuery {
        limit: 500,
        offset: 0,
        selector: String::new(),
        from_ms: None,
        to_ms: None,
    };
    for pair in raw_query.split('&').filter(|pair| !pair.is_empty()) {
        let (raw_name, raw_value) = pair.split_once('=').unwrap_or((pair, ""));
        let name = decode_query_value(raw_name).map_err(|_| invalid())?;
        let value = decode_query_value(raw_value).map_err(|_| invalid())?;
        match name.as_str() {
            "limit" => {
                let value = value.parse::<i64>().map_err(|_| invalid())?;
                query.limit = value.clamp(1, 5000) as usize;
            }
            "offset" => {
                let value = value.parse::<i64>().map_err(|_| invalid())?;
                query.offset = value.max(0) as usize;
            }
            "level" if activity == "logs" => query.selector = value.trim().to_lowercase(),
            "kind" if activity == "audit" => query.selector = value.trim().to_owned(),
            "fromTime" => query.from_ms = Some(parse_timestamp_millis(&value, invalid())?),
            "toTime" => query.to_ms = Some(parse_timestamp_millis(&value, invalid())?),
            _ => {}
        }
    }
    Ok(query)
}

fn decode_query_value(value: &str) -> Result<String, ()> {
    percent_encoding::percent_decode_str(&value.replace('+', " "))
        .decode_utf8()
        .map(|value| value.into_owned())
        .map_err(|_| ())
}

fn parse_timestamp_millis(
    value: &str,
    invalid: StrategyReadSnapshotError,
) -> Result<i64, StrategyReadSnapshotError> {
    let timestamp = time::OffsetDateTime::parse(
        value,
        &time::format_description::well_known::Rfc3339,
    )
    .map_err(|_| invalid)?;
    i64::try_from(timestamp.unix_timestamp_nanos() / 1_000_000).map_err(|_| {
        StrategyReadSnapshotError::Invalid("strategy activity timestamp is out of range".to_owned())
    })
}

fn timestamp_from_millis(value: i64) -> Result<String, StrategyReadSnapshotError> {
    let timestamp =
        time::OffsetDateTime::from_unix_timestamp_nanos(i128::from(value) * 1_000_000)
        .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))
        ?;
    timestamp
        .format(&time::format_description::well_known::Rfc3339)
        .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))
}

fn page_values<T>(values: Vec<T>, query: &StrategyActivityQuery) -> (Vec<T>, Value) {
    let total = values.len();
    let values = values
        .into_iter()
        .skip(query.offset)
        .take(query.limit)
        .collect::<Vec<_>>();
    let returned = values.len();
    let page = json!({
        "limit": query.limit,
        "offset": query.offset,
        "returned": returned,
        "total": total,
        "hasMore": query.offset.saturating_add(returned) < total,
    });
    (values, page)
}

impl StrategyRuntimeStatusPort for ProductionStrategyRuntimePort {
    fn snapshot(&self) -> StrategyRuntimeSummary {
        let Ok(instances) = self.store.list_instances() else {
            return StrategyRuntimeSummary {
                status: "failed".to_owned(),
                active_strategies: 0,
                supports_backtest_parity: true,
                active_instances: Vec::new(),
            };
        };
        let active_strategies = instances
            .iter()
            .filter(|i| i.runtime_active || i.status.eq_ignore_ascii_case("RUNNING"))
            .count();
        let status = if active_strategies > 0 {
            "active".to_owned()
        } else {
            "idle".to_owned()
        };
        let mut active_instances = Vec::new();
        for instance in instances.into_iter().filter(|instance| {
            instance.runtime_active || instance.status.eq_ignore_ascii_case("RUNNING")
        }) {
            let observation = match self.store.get_observation(&instance.id) {
                Ok(observation) => observation,
                Err(_) => {
                    return StrategyRuntimeSummary {
                        status: "failed".to_owned(),
                        active_strategies: 0,
                        supports_backtest_parity: true,
                        active_instances: Vec::new(),
                    };
                }
            };
            let binding_definition_name = instance
                .definition_name
                .clone()
                .unwrap_or_else(|| binding_string(&instance.binding, &["definitionName", "strategyName"]));
            let binding_symbols = binding_symbols(&instance.binding);
            let actual_status = observation
                .as_ref()
                .map(|item| item.actual_status.trim())
                .filter(|status| !status.is_empty())
                .unwrap_or(instance.status.trim())
                .to_ascii_lowercase();
            active_instances.push(crate::product::StrategyRuntimeActiveInstance {
                instance_id: instance.id,
                definition_name: binding_definition_name,
                actual_status,
                active_symbols: observation
                    .as_ref()
                    .map(|item| item.active_symbols.clone())
                    .or(binding_symbols),
                last_closed_kline_at: observation
                    .as_ref()
                    .and_then(|item| item.last_closed_kline_at.clone()),
                last_signal_at: observation
                    .as_ref()
                    .and_then(|item| item.last_signal_at.clone()),
                last_order_at: observation
                    .as_ref()
                    .and_then(|item| item.last_order_at.clone()),
                last_error_at: observation
                    .as_ref()
                    .and_then(|item| item.last_error_at.clone()),
                last_error: observation
                    .as_ref()
                    .and_then(|item| item.last_error.clone()),
                updated_at: observation
                    .as_ref()
                    .and_then(|item| item.updated_at.clone())
                    .or_else(|| {
                        (!instance.updated_at.is_empty()).then_some(instance.updated_at.clone())
                    }),
            });
        }
        StrategyRuntimeSummary {
            status,
            active_strategies,
            supports_backtest_parity: true,
            active_instances,
        }
    }
}

fn binding_string(binding: &Value, keys: &[&str]) -> String {
    keys.iter()
        .filter_map(|key| binding.get(*key).and_then(Value::as_str))
        .map(str::trim)
        .find(|value| !value.is_empty())
        .unwrap_or_default()
        .to_owned()
}

fn binding_string_opt(binding: &Value, keys: &[&str]) -> Option<String> {
    let value = binding_string(binding, keys);
    (!value.is_empty()).then_some(value)
}

fn binding_symbols(binding: &Value) -> Option<Vec<String>> {
    ["activeSymbols", "symbols"].iter().find_map(|key| {
        let values = binding.get(*key)?.as_array()?;
        Some(
            values
                .iter()
                .filter_map(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(ToOwned::to_owned)
                .collect(),
        )
    })
}

impl StrategyRuntimeWritePort for ProductionStrategyRuntimePort {
    fn mutate(
        &self,
        input: &StrategyRuntimeWriteInput,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        let timestamp = time::OffsetDateTime::now_utc()
            .format(&time::format_description::well_known::Rfc3339)
            .unwrap_or_else(|_| "2026-08-27T00:00:00Z".to_owned());

        let result = match input.operation {
            StrategyRuntimeWriteOperation::Start => {
                self.store.update_status(&input.instance_id, "RUNNING", &timestamp)
            }
            StrategyRuntimeWriteOperation::Stop => {
                self.store.update_status(&input.instance_id, "STOPPED", &timestamp)
            }
            StrategyRuntimeWriteOperation::Pause => {
                self.store.update_status(&input.instance_id, "PAUSED", &timestamp)
            }
            StrategyRuntimeWriteOperation::Delete => {
                let current = self
                    .store
                    .get_instance(&input.instance_id)
                    .map_err(|e| StrategyRuntimeWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: e.to_string(),
                    })?
                    .ok_or_else(|| StrategyRuntimeWritePortError::Failed {
                        status: 404,
                        code: "NOT_FOUND".to_owned(),
                        message: "strategy resource not found".to_owned(),
                    })?;
                if current.runtime_active || current.status == "RUNNING" {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "strategy instance is busy".to_owned(),
                    });
                }
                self.store.delete_instance(&input.instance_id, &timestamp)
            }
            StrategyRuntimeWriteOperation::Update => {
                let binding = input.binding.clone().unwrap_or(Value::Null);
                self.store.update_binding(&input.instance_id, binding, &timestamp)
            }
            StrategyRuntimeWriteOperation::UpdateRuntimeRisk => {
                let risk = input.runtime_risk.clone().unwrap_or(Value::Null);
                self.store.update_risk(&input.instance_id, risk, &timestamp)
            }
            StrategyRuntimeWriteOperation::RefreshDefinition => {
                self.store.refresh_definition(&input.instance_id, &timestamp)
            }
        };

        match result {
            Ok(inst) => Ok(json!({
                "id": inst.id,
                "status": inst.status,
                "binding": inst.binding,
                "runtimeRisk": inst.runtime_risk,
                "definitionRevision": inst.definition_revision,
                "runtimeActive": inst.runtime_active,
                "deleted": inst.deleted,
            })),
            Err(jftrade_store_sqlite::StrategyRuntimeStoreError::NotFound) => {
                Err(StrategyRuntimeWritePortError::Failed {
                    status: 404,
                    code: "NOT_FOUND".to_owned(),
                    message: "strategy instance not found".to_owned(),
                })
            }
            Err(jftrade_store_sqlite::StrategyRuntimeStoreError::Conflict) => {
                Err(StrategyRuntimeWritePortError::Failed {
                    status: 409,
                    code: "CONFLICT".to_owned(),
                    message: "strategy state conflict".to_owned(),
                })
            }
            Err(jftrade_store_sqlite::StrategyRuntimeStoreError::Validation(msg)) => {
                Err(StrategyRuntimeWritePortError::Failed {
                    status: 400,
                    code: "VALIDATION_FAILED".to_owned(),
                    message: msg,
                })
            }
            Err(e) => Err(StrategyRuntimeWritePortError::Failed {
                status: 500,
                code: "STRATEGY_RUNTIME_MUTATION_FAILED".to_owned(),
                message: e.to_string(),
            }),
        }
    }
}

// ---------------------------------------------------------------------------
// Research Presets
// ---------------------------------------------------------------------------

#[derive(Debug)]
pub(crate) struct ProductionResearchPresetPort {
    pub(crate) store: Arc<ResearchPresetStore>,
}

impl ResearchPresetReadSnapshotPort for ProductionResearchPresetPort {
    fn read(&self, path: &str, _query: &str) -> Result<Value, ResearchPresetReadSnapshotError> {
        if path == "/api/v1/research/screens/presets" {
            let presets = self
                .store
                .list()
                .map_err(|e| ResearchPresetReadSnapshotError::Unavailable(e.to_string()))?;
            let items: Vec<Value> = presets
                .into_iter()
                .map(|p| serde_json::to_value(&p).map_err(|error| {
                    ResearchPresetReadSnapshotError::Unavailable(error.to_string())
                }))
                .collect::<Result<Vec<_>, _>>()?;
            return Ok(json!({ "presets": items }));
        }

        if let Some(id) = path.strip_prefix("/api/v1/research/screens/presets/") {
            if id.is_empty() || id.contains('/') {
                return Err(ResearchPresetReadSnapshotError::NotFound);
            }
            let preset = self
                .store
                .get(id)
                .map_err(|e| match e {
                    ResearchPresetStoreError::NotFound => ResearchPresetReadSnapshotError::NotFound,
                    other => ResearchPresetReadSnapshotError::Unavailable(other.to_string()),
                })?;
            return serde_json::to_value(&preset)
                .map_err(|error| ResearchPresetReadSnapshotError::Unavailable(error.to_string()));
        }

        Err(ResearchPresetReadSnapshotError::NotFound)
    }
}

impl ResearchPresetWritePort for ProductionResearchPresetPort {
    fn mutate(
        &self,
        mutation: &ResearchPresetWriteMutation,
    ) -> Result<Value, ResearchPresetWritePortError> {
        let timestamp = time::OffsetDateTime::now_utc()
            .format(&time::format_description::well_known::Rfc3339)
            .unwrap_or_default();

        match mutation {
            ResearchPresetWriteMutation::Create { payload } => {
                let object = payload
                    .as_object()
                    .ok_or_else(|| invalid_preset("name is required"))?;
                let name = normalized_preset_name(object.get("name"))?;
                let definition = normalized_preset_definition(object.get("definition"))?;
                let id = object
                    .get("id")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|s| !s.is_empty())
                    .map(ToOwned::to_owned)
                    .unwrap_or_else(|| format!("preset_{}", generate_strategy_id()));
                let preset = ResearchPresetMutation {
                    preset_id: id,
                    name,
                    query_schema_version: 2,
                    definition,
                    revision: 1,
                };
                let stored = self
                    .store
                    .insert(&preset, &timestamp)
                    .map_err(map_research_preset_store_error)?;
                serde_json::to_value(&stored).map_err(|e| {
                    ResearchPresetWritePortError::Failed(format!("encode stored research preset: {e}"))
                })
            }
            ResearchPresetWriteMutation::Update { preset_id, payload } => {
                if preset_id.trim().is_empty() {
                    return Err(invalid_preset("preset id is required"));
                }
                let object = payload
                    .as_object()
                    .ok_or_else(|| invalid_preset("expectedRevision must be positive"))?;
                let expected_revision = object
                    .get("expectedRevision")
                    .and_then(Value::as_u64)
                    .filter(|r| *r > 0)
                    .ok_or_else(|| invalid_preset("expectedRevision must be positive"))?;
                let has_name = object.get("name").is_some_and(|v| !v.is_null());
                let has_definition = object.get("definition").is_some_and(|v| !v.is_null());
                if !has_name && !has_definition {
                    return Err(invalid_preset("name or definition is required"));
                }
                let current = self.store.get(preset_id).map_err(map_research_preset_store_error)?;
                let name = if has_name {
                    normalized_preset_name(object.get("name"))?
                } else {
                    current.preset.name.clone()
                };
                let definition = if has_definition {
                    normalized_preset_definition(object.get("definition"))?
                } else {
                    current.preset.definition.clone()
                };
                let next_revision = expected_revision.checked_add(1).ok_or_else(|| {
                    invalid_preset("expectedRevision exceeds supported range")
                })?;
                let preset = ResearchPresetMutation {
                    preset_id: current.preset.preset_id,
                    name,
                    query_schema_version: 2,
                    definition,
                    revision: next_revision,
                };
                let stored = self
                    .store
                    .replace_revision(&preset, expected_revision, &timestamp)
                    .map_err(map_research_preset_store_error)?;
                serde_json::to_value(&stored).map_err(|e| {
                    ResearchPresetWritePortError::Failed(format!("encode stored research preset: {e}"))
                })
            }
            ResearchPresetWriteMutation::Delete { preset_id } => {
                if preset_id.trim().is_empty() {
                    return Err(invalid_preset("preset id is required"));
                }
                self.store.delete(preset_id).map_err(map_research_preset_store_error)?;
                Ok(json!({"deleted": true}))
            }
        }
    }
}

fn normalized_preset_name(value: Option<&Value>) -> Result<String, ResearchPresetWritePortError> {
    let name = value
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|name| !name.is_empty())
        .ok_or_else(|| invalid_preset("name is required"))?;
    if name.chars().count() > 80 {
        return Err(invalid_preset("name must not exceed 80 characters"));
    }
    Ok(name.to_owned())
}

fn normalized_preset_definition(value: Option<&Value>) -> Result<Value, ResearchPresetWritePortError> {
    let value = value
        .cloned()
        .ok_or_else(|| invalid_preset("definition is required"))?;
    normalize_definition_v2(value).map_err(|error| invalid_preset(error.to_string()))
}

fn invalid_preset(message: impl Into<String>) -> ResearchPresetWritePortError {
    ResearchPresetWritePortError::Invalid(format!(
        "invalid research screen preset: {}",
        message.into()
    ))
}

fn map_research_preset_store_error(error: ResearchPresetStoreError) -> ResearchPresetWritePortError {
    match error {
        ResearchPresetStoreError::NotFound => {
            ResearchPresetWritePortError::NotFound("research screen preset not found".to_owned())
        }
        ResearchPresetStoreError::Conflict => {
            ResearchPresetWritePortError::Conflict("research screen preset conflict".to_owned())
        }
        ResearchPresetStoreError::Incompatible(message) => invalid_preset(message),
        ResearchPresetStoreError::UnsupportedProfile(_) => {
            ResearchPresetWritePortError::Unavailable
        }
        ResearchPresetStoreError::NotRegularFile(_)
        | ResearchPresetStoreError::EmptyPath
        | ResearchPresetStoreError::WriterLease(_)
        | ResearchPresetStoreError::Open(_)
        | ResearchPresetStoreError::Configure(_)
        | ResearchPresetStoreError::Schema(_)
        | ResearchPresetStoreError::LockUnavailable
        | ResearchPresetStoreError::Query(_) => ResearchPresetWritePortError::Unavailable,
    }
}

// ---------------------------------------------------------------------------
// Research Read & Screen Write
// ---------------------------------------------------------------------------

pub(crate) struct ProductionResearchPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) helper: Option<HelperClient>,
}

impl std::fmt::Debug for ProductionResearchPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionResearchPort")
            .field("helper", &self.helper.is_some())
            .finish()
    }
}

impl ResearchReadSnapshotPort for ProductionResearchPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, ResearchReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        let Some(provider) = snapshot.provider else {
            return Err(ResearchReadSnapshotError::Unavailable(
                "research provider is not configured".to_owned(),
            ));
        };
        if !snapshot.helper_ready {
            return Err(ResearchReadSnapshotError::Unavailable(
                "market-data helper is not ready".to_owned(),
            ));
        }
        let provider = match provider {
            jftrade_settings::MarketDataProvider::Yfinance => "yfinance",
            jftrade_settings::MarketDataProvider::Akshare => "akshare",
            jftrade_settings::MarketDataProvider::Futu => {
                return Err(ResearchReadSnapshotError::Unavailable(
                    "research helper does not support the Futu provider".to_owned(),
                ));
            }
        };
        let (operation, market, symbol, extra_query) = research_helper_request(path, query)?;
        let Some(helper) = self.helper.clone() else {
            return Err(ResearchReadSnapshotError::Unavailable(
                "market-data helper is not configured".to_owned(),
            ));
        };
        let result = thread::spawn(move || {
            let query_refs = extra_query
                .iter()
                .map(|(key, value)| (*key, value.as_str()))
                .collect::<Vec<_>>();
            let runtime = tokio::runtime::Builder::new_current_thread()
                .enable_all()
                .build()
                .map_err(|error| HttpAdapterError::Unavailable(error.to_string()))?;
            runtime.block_on(helper.get_provider_json_with_query::<Value>(
                provider,
                &[operation, &market, &symbol],
                &query_refs,
            ))
        })
        .join()
        .map_err(|_| ResearchReadSnapshotError::Unavailable("research helper task panicked".to_owned()))?;
        let payload = result.map_err(map_research_helper_error)?;
        validate_research_payload(operation, payload)
    }
}

fn research_helper_request(
    path: &str,
    query: &str,
) -> Result<(&'static str, String, String, Vec<(&'static str, String)>), ResearchReadSnapshotError> {
    let (operation, suffix) = if let Some(value) = path.strip_prefix("/api/v1/research/financials/") {
        ("financials", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/research/analyst/") {
        ("analyst", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/research/ownership/") {
        ("ownership", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/research/corporate-actions/") {
        ("corporate-actions", value)
    } else if let Some(value) = path.strip_prefix("/api/v1/research/instruments/") {
        ("profile", value)
    } else {
        return Err(ResearchReadSnapshotError::Unavailable(
            "research operation is not backed by the market-data helper".to_owned(),
        ));
    };
    let mut parts = suffix.splitn(2, '/');
    let first = parts.next().unwrap_or_default().trim();
    let second = parts.next().unwrap_or_default().trim();
    let (market, symbol) = if operation == "profile" {
        first.split_once('.').ok_or_else(|| ResearchReadSnapshotError::Invalid(
            "instrument must use MARKET.SYMBOL form".to_owned(),
        ))?
    } else if !first.is_empty() && !second.is_empty() && !second.contains('/') {
        (first, second)
    } else {
        return Err(ResearchReadSnapshotError::Invalid(
            "research instrument path is invalid".to_owned(),
        ));
    };
    if market.is_empty() || symbol.is_empty() || market.contains('.') {
        return Err(ResearchReadSnapshotError::Invalid(
            "research instrument path is invalid".to_owned(),
        ));
    }
    let mut extra_query = Vec::new();
    for pair in query.split('&').filter(|pair| !pair.is_empty()) {
        let Some((key, value)) = pair.split_once('=') else { continue };
        if operation == "financials" && key == "statement" {
            extra_query.push(("statement", value.to_owned()));
        } else if operation == "corporate-actions" && (key == "from" || key == "to") {
            extra_query.push((if key == "from" { "from" } else { "to" }, value.to_owned()));
        }
    }
    Ok((operation, market.to_owned(), symbol.to_owned(), extra_query))
}

fn validate_research_payload(
    operation: &str,
    payload: Value,
) -> Result<Value, ResearchReadSnapshotError> {
    let Some(object) = payload.as_object() else {
        return Err(ResearchReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: "market-data helper returned a non-object research response".to_owned(),
        });
    };
    let required = match operation {
        "financials" => &["instrumentId", "statement", "fields", "periods"][..],
        "analyst" => &["instrumentId"][..],
        "ownership" => &["instrumentId", "groups"][..],
        "corporate-actions" => &["market", "symbol", "instrumentId", "events", "source"][..],
        "profile" => &["instrumentId"][..],
        _ => &[][..],
    };
    if let Some(missing) = required.iter().find(|key| !object.contains_key(**key)) {
        return Err(ResearchReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message: format!("market-data helper response is missing {missing}"),
        });
    }
    Ok(payload)
}

fn map_research_helper_error(error: HttpAdapterError) -> ResearchReadSnapshotError {
    match error {
        HttpAdapterError::Remote { status, code, message, .. } => ResearchReadSnapshotError::Failed {
            status,
            code: if code.is_empty() { "BAD_GATEWAY".to_owned() } else { code },
            message,
        },
        HttpAdapterError::Timeout => ResearchReadSnapshotError::Failed {
            status: 504,
            code: "GATEWAY_TIMEOUT".to_owned(),
            message: "market-data helper request timed out".to_owned(),
        },
        HttpAdapterError::InvalidResponse(message) => ResearchReadSnapshotError::Failed {
            status: 502,
            code: "BAD_GATEWAY".to_owned(),
            message,
        },
        other => ResearchReadSnapshotError::Unavailable(other.to_string()),
    }
}

#[cfg(test)]
mod research_helper_tests {
    use super::*;
    use std::time::Duration;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::TcpListener;

    fn helper(base_url: String) -> HelperClient {
        HelperClient::new(jftrade_integration_marketdata_helper::HelperClientConfig {
            base_url,
            bearer_token: None,
            request_timeout: Duration::from_secs(1),
            max_attempts: 1,
            retry_delay: Duration::ZERO,
        })
        .expect("helper client")
    }

    #[test]
    fn research_helper_request_rejects_unsupported_or_malformed_paths() {
        assert!(matches!(
            research_helper_request("/api/v1/research/technical-indicators/US.AAPL", ""),
            Err(ResearchReadSnapshotError::Unavailable(_))
        ));
        assert!(matches!(
            research_helper_request("/api/v1/research/financials/US", ""),
            Err(ResearchReadSnapshotError::Invalid(_))
        ));
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn production_research_port_forwards_financials_to_helper() {
        let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
        let address = listener.local_addr().expect("address");
        let server = tokio::spawn(async move {
            let (mut stream, _) = listener.accept().await.expect("accept");
            let mut request = vec![0_u8; 4096];
            let read = stream.read(&mut request).await.expect("read");
            let request = String::from_utf8_lossy(&request[..read]);
            assert!(request.starts_with(
                "GET /providers/yfinance/financials/US/AAPL?statement=balance HTTP/1.1\r\n"
            ));
            let body = r#"{"instrumentId":"US.AAPL","statement":"balance","fields":[],"periods":[]}"#;
            let response = format!(
                "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                body.len(), body
            );
            stream.write_all(response.as_bytes()).await.expect("write");
        });
        let state = Arc::new(ActiveProviderState::new(Some(
            jftrade_settings::MarketDataProvider::Yfinance,
        )));
        state.set_readiness(true, false, false);
        let port = ProductionResearchPort {
            active_provider_state: state,
            helper: Some(helper(format!("http://{address}"))),
        };
        let value = port
            .read(
                "/api/v1/research/financials/US/AAPL",
                "statement=balance",
            )
            .expect("research response");
        assert_eq!(value["statement"], "balance");
        server.await.expect("server");
    }

    #[tokio::test(flavor = "multi_thread", worker_threads = 2)]
    async fn production_research_port_preserves_helper_http_errors() {
        let listener = TcpListener::bind("127.0.0.1:0").await.expect("listen");
        let address = listener.local_addr().expect("address");
        let server = tokio::spawn(async move {
            let (mut stream, _) = listener.accept().await.expect("accept");
            let body = r#"{"error":{"code":"NOT_FOUND","message":"financials not found"}}"#;
            let response = format!(
                "HTTP/1.1 404 Not Found\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                body.len(), body
            );
            stream.write_all(response.as_bytes()).await.expect("write");
            let mut request = [0_u8; 1024];
            let _ = stream.read(&mut request).await;
        });
        let state = Arc::new(ActiveProviderState::new(Some(
            jftrade_settings::MarketDataProvider::Yfinance,
        )));
        state.set_readiness(true, false, false);
        let port = ProductionResearchPort {
            active_provider_state: state,
            helper: Some(helper(format!("http://{address}"))),
        };
        assert!(matches!(
            port.read("/api/v1/research/analyst/US/AAPL", ""),
            Err(ResearchReadSnapshotError::Failed { status: 404, ref code, .. }) if code == "NOT_FOUND"
        ));
        server.await.expect("server");
    }
}

#[derive(Debug)]
pub(crate) struct ProductionResearchScreenPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
}

impl ResearchScreenWritePort for ProductionResearchScreenPort {
    fn query(
        &self,
        _request: &ResearchScreenWriteQuery,
    ) -> Result<Value, ResearchScreenWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || (!snapshot.helper_ready && !snapshot.opend_ready) {
            return Err(ResearchScreenWritePortError::Unavailable);
        }
        Err(ResearchScreenWritePortError::Unavailable)
    }
}

// ---------------------------------------------------------------------------
// Strategy Pine Analyze
// ---------------------------------------------------------------------------

#[derive(Debug)]
pub(crate) struct ProductionStrategyPinePort {
    pub(crate) worker_status: &'static str,
}

impl StrategyPineAnalyzeSnapshotPort for ProductionStrategyPinePort {
    fn analyze(
        &self,
        _input: &StrategyPineAnalyzeInput,
    ) -> Result<Value, StrategyPineAnalyzeSnapshotError> {
        if self.worker_status != "ready" {
            return Err(StrategyPineAnalyzeSnapshotError::Unavailable(
                "pine analyzer is not configured".to_owned(),
            ));
        }
        Err(StrategyPineAnalyzeSnapshotError::Unavailable(
            "pine analyzer is not configured".to_owned(),
        ))
    }
}
