//! Production strategy runtime adapter.

use std::sync::Arc;
use jftrade_store_sqlite::{StrategyDefinitionStore, StrategyRuntimeStore};
use serde_json::{Value, json};
use crate::product::product_strategy_runtime_write_port::{
    StrategyRuntimeWriteInput, StrategyRuntimeWriteOperation, StrategyRuntimeWritePort,
    StrategyRuntimeWritePortError,
};
use crate::product::{StrategyReadSnapshotError, StrategyReadSnapshotPort, StrategyRuntimeStatusPort, StrategyRuntimeSummary};

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
        if let Some(runtime) = runtime {
            object.insert("runtime".to_owned(), Value::String(runtime.to_owned()));
        }
        if let Some(source_format) = source_format {
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
        if let Some(runtime) = runtime {
            params.insert("runtime".to_owned(), Value::String(runtime.to_owned()));
        }
        if let Some(source_format) = source_format {
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
