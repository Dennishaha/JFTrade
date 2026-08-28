//! Strategy and Research Presets production ports.

use std::sync::Arc;
use std::time::SystemTime;

use jftrade_store_sqlite::{
    ResearchPresetMutation, ResearchPresetStore, StrategyDefinitionStore, StrategyRuntimeStore,
};
use serde_json::{Value, json};

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
                let def = input.definition.clone().unwrap_or_default();
                let id = def.get("id").and_then(Value::as_str).unwrap_or("strategy-default");
                let name = def.get("name").and_then(Value::as_str).unwrap_or("").to_owned();
                let stored = self.store
                    .save_definition(
                        jftrade_store_sqlite::StoredStrategyDefinition {
                            id: id.to_owned(),
                            name,
                            version: "1.0".to_owned(),
                            description: "".to_owned(),
                            runtime: "pine".to_owned(),
                            source_format: "pine".to_owned(),
                            symbol: "".to_owned(),
                            interval: "".to_owned(),
                            script: "".to_owned(),
                            visual_model_json: "{}".to_owned(),
                            created_at: timestamp.clone(),
                            updated_at: timestamp.clone(),
                            deleted_at: None,
                        },
                        &timestamp,
                    )
                    .map_err(|e| StrategyDefinitionWritePortError::Failed {
                        status: 400,
                        code: "STRATEGY_DEFINITION_ERROR".to_owned(),
                        message: e.to_string(),
                    })?;
                serde_json::to_value(&stored).map_err(|error| {
                    StrategyDefinitionWritePortError::Failed {
                        status: 500,
                        code: "STRATEGY_DEFINITION_ERROR".to_owned(),
                        message: format!("encode stored strategy definition: {error}"),
                    }
                })
            }
            StrategyDefinitionWriteOperation::Update => {
                let def = input.definition.clone().unwrap_or_default();
                let id = input.definition_id.as_deref().unwrap_or("strategy-default");
                let name = def.get("name").and_then(Value::as_str).unwrap_or("").to_owned();
                let stored = self.store
                    .save_definition(
                        jftrade_store_sqlite::StoredStrategyDefinition {
                            id: id.to_owned(),
                            name,
                            version: "1.0".to_owned(),
                            description: "".to_owned(),
                            runtime: "pine".to_owned(),
                            source_format: "pine".to_owned(),
                            symbol: "".to_owned(),
                            interval: "".to_owned(),
                            script: "".to_owned(),
                            visual_model_json: "{}".to_owned(),
                            created_at: timestamp.clone(),
                            updated_at: timestamp.clone(),
                            deleted_at: None,
                        },
                        &timestamp,
                    )
                    .map_err(|e| StrategyDefinitionWritePortError::Failed {
                        status: 400,
                        code: "STRATEGY_DEFINITION_ERROR".to_owned(),
                        message: e.to_string(),
                    })?;
                serde_json::to_value(&stored).map_err(|error| {
                    StrategyDefinitionWritePortError::Failed {
                        status: 500,
                        code: "STRATEGY_DEFINITION_ERROR".to_owned(),
                        message: format!("encode stored strategy definition: {error}"),
                    }
                })
            }
            StrategyDefinitionWriteOperation::Delete => {
                let id = input.definition_id.as_deref().unwrap_or_default();
                self.store
                    .delete_definition(id, &timestamp)
                    .map_err(|e| StrategyDefinitionWritePortError::Failed {
                        status: 400,
                        code: "STRATEGY_DEFINITION_ERROR".to_owned(),
                        message: e.to_string(),
                    })?;
                Ok(json!({"deleted": true}))
            }
            StrategyDefinitionWriteOperation::ApplyLinkedInstances => {
                Err(StrategyDefinitionWritePortError::Unavailable(
                    "linked strategy runtime is not configured".to_owned(),
                ))
            }
            StrategyDefinitionWriteOperation::Instantiate => {
                let id = input.definition_id.as_deref().unwrap_or_default();
                let instance_id = format!("inst_{id}");
                let runtime = StrategyRuntimeStore::from_definition_store(&self.store);
                runtime
                    .seed_instance(&instance_id, "STOPPED", &timestamp)
                    .map_err(|error| StrategyDefinitionWritePortError::Failed {
                        status: 400,
                        code: "STRATEGY_RUNTIME_ERROR".to_owned(),
                        message: error.to_string(),
                    })?;
                Ok(json!({"instanceId": instance_id, "definitionId": id, "status": "STOPPED"}))
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Strategy Runtime
// ---------------------------------------------------------------------------

#[derive(Debug)]
pub(crate) struct ProductionStrategyRuntimePort {
    pub(crate) store: Arc<StrategyRuntimeStore>,
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
                    .map(runtime_instance_wire)
                    .collect::<Vec<_>>();
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

fn runtime_instance_wire(instance: jftrade_store_sqlite::StoredRuntimeInstance) -> Value {
    let mut object = serde_json::Map::new();
    object.insert("id".to_owned(), Value::String(instance.id));
    object.insert("status".to_owned(), Value::String(instance.status));
    object.insert("binding".to_owned(), instance.binding);
    object.insert("runtimeRisk".to_owned(), instance.runtime_risk);
    object.insert(
        "definitionRevision".to_owned(),
        Value::from(instance.definition_revision),
    );
    object.insert(
        "runtimeActive".to_owned(),
        Value::Bool(instance.runtime_active),
    );
    if !instance.plugin_id.is_empty() {
        object.insert("pluginId".to_owned(), Value::String(instance.plugin_id));
    }
    if !instance.updated_at.is_empty() {
        object.insert("updatedAt".to_owned(), Value::String(instance.updated_at));
    }
    Value::Object(object)
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
        StrategyRuntimeSummary {
            status: "running".to_owned(),
            active_strategies: instances.len(),
            supports_backtest_parity: true,
            active_instances: instances
                .into_iter()
                .map(|i| crate::product::StrategyRuntimeActiveInstance {
                    instance_id: i.id,
                    definition_name: "".to_owned(),
                    actual_status: i.status,
                    active_symbols: None,
                    last_closed_kline_at: None,
                    last_signal_at: None,
                    last_order_at: None,
                    last_error_at: None,
                    last_error: None,
                    updated_at: Some(i.updated_at),
                })
                .collect(),
        }
    }
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
            let preset = self
                .store
                .get(id)
                .map_err(|e| ResearchPresetReadSnapshotError::Unavailable(e.to_string()))?;
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
                let name = payload.get("name").and_then(Value::as_str).unwrap_or("Default");
                let id = payload
                    .get("id")
                    .and_then(Value::as_str)
                    .map(|s| s.to_owned())
                    .unwrap_or_else(|| {
                        format!(
                            "preset_{}",
                            SystemTime::now()
                                .duration_since(std::time::UNIX_EPOCH)
                                .map(|d| d.as_millis())
                                .unwrap_or(0)
                        )
                    });
                let definition = if payload.is_object() {
                    payload.clone()
                } else {
                    json!({})
                };
                let m = ResearchPresetMutation {
                    preset_id: id,
                    name: name.to_owned(),
                    query_schema_version: 2,
                    definition,
                    revision: 1,
                };
                let stored = self
                    .store
                    .insert(&m, &timestamp)
                    .map_err(|e| ResearchPresetWritePortError::Invalid(e.to_string()))?;
                serde_json::to_value(&stored).map_err(|error| {
                    ResearchPresetWritePortError::Failed(format!(
                        "encode stored research preset: {error}"
                    ))
                })
            }
            ResearchPresetWriteMutation::Update { preset_id, payload } => {
                let current = self
                    .store
                    .get(preset_id)
                    .map_err(|e| ResearchPresetWritePortError::Invalid(e.to_string()))?;
                let name = payload.get("name").and_then(Value::as_str).unwrap_or(&current.preset.name);
                let definition = if payload.is_object() {
                    payload.clone()
                } else {
                    current.preset.definition.clone()
                };
                let expected_revision = current.preset.revision;
                let m = ResearchPresetMutation {
                    preset_id: preset_id.clone(),
                    name: name.to_owned(),
                    query_schema_version: 2,
                    definition,
                    revision: expected_revision + 1,
                };
                let stored = self
                    .store
                    .replace_revision(&m, expected_revision, &timestamp)
                    .map_err(|e| ResearchPresetWritePortError::Invalid(e.to_string()))?;
                serde_json::to_value(&stored).map_err(|error| {
                    ResearchPresetWritePortError::Failed(format!(
                        "encode stored research preset: {error}"
                    ))
                })
            }
            ResearchPresetWriteMutation::Delete { preset_id } => {
                self.store
                    .delete(preset_id)
                    .map_err(|e| ResearchPresetWritePortError::Invalid(e.to_string()))?;
                Ok(json!({"deleted": true}))
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Research Read & Screen Write
// ---------------------------------------------------------------------------

#[derive(Debug)]
pub(crate) struct ProductionResearchPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
}

impl ResearchReadSnapshotPort for ProductionResearchPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, ResearchReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || (!snapshot.helper_ready && !snapshot.opend_ready) {
            return Err(ResearchReadSnapshotError::Unavailable(
                "research provider is not configured".to_owned(),
            ));
        }
        Err(ResearchReadSnapshotError::Unavailable(
            "research provider is not configured".to_owned(),
        ))
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
