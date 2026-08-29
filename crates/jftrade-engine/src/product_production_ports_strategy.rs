//! Strategy and Research Presets production ports.

use std::sync::Arc;
use jftrade_store_sqlite::{
    StrategyDefinitionStore, StrategyDefinitionStoreError, StrategyRuntimeStore,
};
use serde_json::{Value, json};
use crate::product::product_strategy_definition_write_port::{
    StrategyDefinitionWriteInput, StrategyDefinitionWriteOperation, StrategyDefinitionWritePort,
    StrategyDefinitionWritePortError,
};
use crate::product::strategy_pine::{
    StrategyPineAnalyzeInput, StrategyPineAnalyzeSnapshotError, StrategyPineAnalyzeSnapshotPort,
};
use crate::product::{
    StrategyDefinitionPreview, StrategyDefinitionSnapshotError, StrategyDefinitionSnapshotPort,
};

#[path = "product_production_ports_research.rs"]
mod product_production_ports_research;
pub(crate) use product_production_ports_research::{ProductionResearchPort, ProductionResearchPresetPort, ProductionResearchScreenPort};

#[path = "product_production_ports_strategy_runtime.rs"]
mod product_production_ports_strategy_runtime;
pub(crate) use product_production_ports_strategy_runtime::ProductionStrategyRuntimePort;

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

pub(crate) fn generate_strategy_id() -> String {
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
