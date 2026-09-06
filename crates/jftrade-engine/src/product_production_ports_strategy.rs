//! Strategy and Research Presets production ports.

use crate::product::product_strategy_definition_write_port::{
    StrategyDefinitionWriteInput, StrategyDefinitionWriteOperation, StrategyDefinitionWritePort,
    StrategyDefinitionWritePortError,
};
use crate::product::product_strategy_runtime_write_port::StrategyRuntimeWritePortError;
use crate::product::strategy_pine::{
    StrategyPineAnalyzeInput, StrategyPineAnalyzeSnapshotError, StrategyPineAnalyzeSnapshotPort,
};
use crate::product::{
    StrategyDefinitionPreview, StrategyDefinitionSnapshotError, StrategyDefinitionSnapshotPort,
};
use jftrade_store_sqlite::{
    StrategyDefinitionStore, StrategyDefinitionStoreError, StrategyRuntimeStore,
};
use serde_json::{Value, json};
use std::sync::Arc;
use self::product_production_ports_strategy_runtime::normalize_strategy_binding;

#[path = "product_production_ports_research.rs"]
mod product_production_ports_research;
#[path = "product_production_ports_research_calendar.rs"]
mod product_production_ports_research_calendar;
#[path = "product_production_ports_research_market.rs"]
mod product_production_ports_research_market;
pub(crate) use product_production_ports_research::{
    ProductionResearchPort, ProductionResearchPresetPort, ProductionResearchScreenHelperPort,
};
pub(crate) use product_production_ports_research_calendar::read_market_calendar;
pub(crate) use product_production_ports_research_market::read_market_research;

#[path = "strategy_runtime.rs"]
mod product_production_ports_strategy_runtime;
pub(crate) use product_production_ports_strategy_runtime::{
    ProductionStrategyRuntimePort, StrategyRuntimeManager,
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
        preview: &StrategyDefinitionPreview,
    ) -> Result<Option<Value>, StrategyDefinitionSnapshotError> {
        let def = self
            .store
            .get_definition(definition_id, true)
            .map_err(|e| StrategyDefinitionSnapshotError::Unavailable(e.to_string()))?;
        let Some(mut value) = def
            .map(|d| {
                serde_json::to_value(&d)
                    .map_err(|error| StrategyDefinitionSnapshotError::Unavailable(error.to_string()))
            })
            .transpose()?
        else {
            return Ok(None);
        };

        if let Some(obj) = value.as_object_mut() {
            let script = obj.get("script").and_then(Value::as_str).unwrap_or_default();
            let validation = jftrade_strategy::pinespec::validate_script(script, true, false);
            let symbol = preview
                .symbol
                .as_deref()
                .or_else(|| obj.get("symbol").and_then(Value::as_str))
                .unwrap_or_default();
            let interval = preview
                .interval
                .clone()
                .or_else(|| obj.get("interval").and_then(Value::as_str).map(str::to_owned))
                .unwrap_or_else(|| "5m".to_owned());
            let warmup_bars = validation
                .requirements
                .as_ref()
                .map(|r| {
                    r.derived_warmup_bars_with_session(symbol, &interval, preview.use_extended_hours)
                })
                .unwrap_or(0);
            obj.insert("derivedWarmupBars".to_owned(), json!(warmup_bars));
            obj.insert("derivedWarmupInterval".to_owned(), json!(interval));
        }
        Ok(Some(value))
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
                let visual_model_json =
                    if let Some(vm) = def.get("visualModelJson").and_then(Value::as_str) {
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
                let visual_model_json = if let Some(vm) =
                    def.get("visualModelJson").and_then(Value::as_str)
                {
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
                let definition = self
                    .store
                    .get_definition(definition_id, false)
                    .map_err(map_strategy_store_error)?
                    .ok_or_else(|| StrategyDefinitionWritePortError::Failed {
                        status: 404,
                        code: "NOT_FOUND".to_owned(),
                        message: "resource not found".to_owned(),
                    })?;
                let runtime = StrategyRuntimeStore::from_definition_store(&self.store);
                let applied = runtime
                    .apply_definition_to_linked(
                        &definition,
                        &timestamp,
                    )
                    .map_err(|error| StrategyDefinitionWritePortError::Failed {
                        status: match &error {
                            jftrade_store_sqlite::StrategyRuntimeStoreError::Conflict => 409,
                            jftrade_store_sqlite::StrategyRuntimeStoreError::Validation(_) => 400,
                            _ => 500,
                        },
                        code: "STRATEGY_LINKED_APPLY_FAILED".to_owned(),
                        message: error.to_string(),
                    })?;
                Ok(json!({
                    "definitionId": definition.id,
                    "latestVersion": definition.version,
                    "totalLinked": applied.total_linked,
                    "applied": applied.applied,
                    "alreadyLatest": applied.already_latest,
                    "skippedBusy": applied.skipped_busy,
                }))
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
                let mut binding = input.binding.clone().unwrap_or_else(|| json!({}));
                normalize_strategy_binding(&mut binding).map_err(|error| {
                    let message = match error {
                        StrategyRuntimeWritePortError::Unavailable(message) => message,
                        StrategyRuntimeWritePortError::Failed { message, .. } => message,
                    };
                    StrategyDefinitionWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message,
                    }
                })?;
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

fn map_strategy_store_error(
    error: StrategyDefinitionStoreError,
) -> StrategyDefinitionWritePortError {
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
    pub(crate) worker: Option<Arc<jftrade_integration_pine::GrpcPineExecutionPort>>,
}

impl StrategyPineAnalyzeSnapshotPort for ProductionStrategyPinePort {
    fn analyze(
        &self,
        input: &StrategyPineAnalyzeInput,
    ) -> Result<Value, StrategyPineAnalyzeSnapshotError> {
        let Some(worker) = self.worker.as_ref() else {
            return Err(StrategyPineAnalyzeSnapshotError::Unavailable(
                "pine analyzer is not configured".to_owned(),
            ));
        };
        let worker = Arc::clone(worker);
        let script = input.script.clone();
        let include_ast = input.include_ast;
        let job_id = next_pine_job_id("strategy-pine");
        let result = std::thread::Builder::new()
            .name("jftrade-pine-analyze".to_owned())
            .spawn(move || {
                let runtime = tokio::runtime::Runtime::new().map_err(|error| {
                    jftrade_integration_pine::PineExecutionError::Transport(error.to_string())
                })?;
                runtime.block_on(worker.analyze_script(
                    &job_id,
                    "strategy-pine",
                    &script,
                    include_ast,
                ))
            })
            .map_err(|error| StrategyPineAnalyzeSnapshotError::Unavailable(error.to_string()))?;
        let result = result
            .join()
            .map_err(|_| StrategyPineAnalyzeSnapshotError::Failed {
                status: 502,
                code: "STRATEGY_PINE_ANALYZE_FAILED".to_owned(),
                message: "pine analyzer worker thread panicked".to_owned(),
                retry_after_seconds: None,
            })?;
        result.map_err(map_pine_analysis_error)
    }

    fn evaluate_shadow(
        &self,
        input: &StrategyPineAnalyzeInput,
    ) -> Result<Value, StrategyPineAnalyzeSnapshotError> {
        let Some(worker) = self.worker.as_ref() else {
            return Err(StrategyPineAnalyzeSnapshotError::Unavailable(
                "pine analyzer is not configured".to_owned(),
            ));
        };
        let worker = Arc::clone(worker);
        let request = pine_shadow_request(next_pine_job_id("strategy-pine-shadow"), &input.script);
        let result = std::thread::Builder::new()
            .name("jftrade-pine-shadow".to_owned())
            .spawn(move || {
                let runtime = tokio::runtime::Runtime::new().map_err(|error| {
                    jftrade_integration_pine::PineExecutionError::Transport(error.to_string())
                })?;
                runtime.block_on(worker.run_script(request))
            })
            .map_err(|error| StrategyPineAnalyzeSnapshotError::Unavailable(error.to_string()))?;
        let result = result
            .join()
            .map_err(|_| StrategyPineAnalyzeSnapshotError::Failed {
                status: 502,
                code: "STRATEGY_PINE_ANALYZE_FAILED".to_owned(),
                message: "pine shadow worker thread panicked".to_owned(),
                retry_after_seconds: None,
            })?;
        result
            .map(pine_shadow_result)
            .map_err(map_pine_analysis_error)
    }
}

fn next_pine_job_id(prefix: &str) -> String {
    use std::sync::atomic::{AtomicU64, Ordering};
    static NEXT_PINE_JOB_ID: AtomicU64 = AtomicU64::new(1);
    format!(
        "{prefix}-{}-{}",
        time::OffsetDateTime::now_utc().unix_timestamp_nanos(),
        NEXT_PINE_JOB_ID.fetch_add(1, Ordering::Relaxed)
    )
}

fn pine_shadow_request(job_id: String, script: &str) -> jftrade_integration_pine::PineRunRequest {
    const START_MILLIS: i64 = 1_704_067_200_000;
    const STEP_MILLIS: i64 = 60_000;
    let candles = (0..80)
        .map(|index| {
            let close = 100.0 + f64::from(index) + (f64::from(index) / 3.0).sin();
            jftrade_integration_pine::PineCandle {
                open_time: START_MILLIS + i64::from(index) * STEP_MILLIS,
                close_time: START_MILLIS + i64::from(index + 1) * STEP_MILLIS,
                open: close - 0.4,
                high: close + 1.0,
                low: close - 1.0,
                close,
                volume: 1_000.0 + f64::from(index),
            }
        })
        .collect();
    jftrade_integration_pine::PineRunRequest {
        job_id,
        script_id: "strategy-pine".to_owned(),
        source: script.to_owned(),
        symbol: "JFTRADE.SAMPLE".to_owned(),
        timeframe: "1m".to_owned(),
        chart_type: "standard".to_owned(),
        // Analyze mode on RunScript preserves plot output while executing the
        // same 80 deterministic candles as the Go shadow worker.
        mode: "analyze".to_owned(),
        candles,
        ..Default::default()
    }
}

fn pine_shadow_result(result: jftrade_integration_pine::PineRunResult) -> Value {
    let mut plots = serde_json::Map::new();
    let mut signals = serde_json::Map::new();
    for plot in result.plots {
        let name = plot.name;
        signals.insert(
            name.clone(),
            plot.values.last().copied().map_or(Value::Null, Value::from),
        );
        plots.insert(name.clone(), json!({"title": name, "data": plot.values}));
    }
    let diagnostics = result
        .diagnostics
        .into_iter()
        .map(|item| {
            json!({
                "severity": item.severity,
                "code": item.code,
                "message": item.message,
                "line": item.line,
                "column": item.column,
                "endLine": item.line,
                "endColumn": item.column,
            })
        })
        .collect::<Vec<_>>();
    json!({
        "ok": true,
        "engineVersion": result.metadata.pine_ts_version,
        "license": "AGPL-3.0-only",
        "diagnostics": diagnostics,
        "plots": plots,
        "signals": signals,
    })
}

fn map_pine_analysis_error(
    error: jftrade_integration_pine::PineExecutionError,
) -> StrategyPineAnalyzeSnapshotError {
    use jftrade_integration_pine::PineExecutionError as PineError;
    match error {
        PineError::Unavailable(message) => StrategyPineAnalyzeSnapshotError::Unavailable(message),
        PineError::InvalidRequest(message) => StrategyPineAnalyzeSnapshotError::Failed {
            status: 400,
            code: "BAD_REQUEST".to_owned(),
            message,
            retry_after_seconds: None,
        },
        PineError::Timeout => StrategyPineAnalyzeSnapshotError::Failed {
            status: 503,
            code: "STRATEGY_PINE_ANALYZE_TIMEOUT".to_owned(),
            message: "pine analyzer request timed out".to_owned(),
            retry_after_seconds: Some(1),
        },
        PineError::Cancelled => StrategyPineAnalyzeSnapshotError::Failed {
            status: 503,
            code: "STRATEGY_PINE_ANALYZE_CANCELLED".to_owned(),
            message: "pine analyzer request cancelled".to_owned(),
            retry_after_seconds: None,
        },
        PineError::Remote(message)
        | PineError::Transport(message)
        | PineError::InvalidResponse(message) => StrategyPineAnalyzeSnapshotError::Failed {
            status: 502,
            code: "STRATEGY_PINE_ANALYZE_FAILED".to_owned(),
            message,
            retry_after_seconds: None,
        },
        PineError::InvalidEndpoint(message) => {
            StrategyPineAnalyzeSnapshotError::Unavailable(message)
        }
        PineError::WeakToken => {
            StrategyPineAnalyzeSnapshotError::Unavailable("pine worker token is invalid".to_owned())
        }
    }
}

#[path = "product_production_ports_strategy_tests.rs"]
#[cfg(test)]
mod tests;
