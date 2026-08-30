//! Production strategy runtime adapter.

use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_strategy_runtime_write_port::{
    StrategyRuntimeWriteInput, StrategyRuntimeWriteOperation, StrategyRuntimeWritePort,
    StrategyRuntimeWritePortError,
};
use crate::product::{
    StrategyReadSnapshotError, StrategyReadSnapshotPort, StrategyRuntimeStatusPort,
    StrategyRuntimeSummary,
};
use jftrade_integration_pine::{GrpcPineExecutionPort, PineExecutionError};
use jftrade_marketdata::{InstrumentRef, ProviderRouter};
use jftrade_store_sqlite::{StrategyDefinitionStore, StrategyRuntimeStore};
use serde_json::{Value, json};
use std::collections::BTreeMap;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;

#[derive(Debug)]
struct RuntimeTask {
    cancel: Arc<AtomicBool>,
    _join: Option<JoinHandle<()>>,
}

/// Rust-owned lifecycle coordinator for live strategy instances.  The HTTP
/// port remains synchronous, therefore task execution is isolated in a
/// dedicated thread with its own Tokio runtime; cancellation and demand
/// release happen before the persisted state transition.
#[derive(Debug)]
pub(crate) struct StrategyRuntimeManager {
    tasks: Mutex<BTreeMap<String, RuntimeTask>>,
    router: Option<Arc<Mutex<ProviderRouter>>>,
    worker: Option<Arc<GrpcPineExecutionPort>>,
    provider: Arc<ActiveProviderState>,
}

impl StrategyRuntimeManager {
    pub(crate) fn new(
        router: Option<Arc<Mutex<ProviderRouter>>>,
        worker: Option<Arc<GrpcPineExecutionPort>>,
        provider: Arc<ActiveProviderState>,
    ) -> Self {
        Self {
            tasks: Mutex::new(BTreeMap::new()),
            router,
            worker,
            provider,
        }
    }

    fn dependency_error(&self) -> Option<StrategyRuntimeWritePortError> {
        let snapshot = self.provider.snapshot();
        let Some(active) = snapshot.provider else {
            return Some(StrategyRuntimeWritePortError::Unavailable(
                "strategy provider is not configured".to_owned(),
            ));
        };
        let provider_ready = match active {
            jftrade_settings::MarketDataProvider::Futu => {
                snapshot.opend_ready && snapshot.router_ready
            }
            jftrade_settings::MarketDataProvider::Yfinance
            | jftrade_settings::MarketDataProvider::Akshare => snapshot.helper_ready,
        };
        if !provider_ready {
            return Some(StrategyRuntimeWritePortError::Unavailable(
                "strategy market-data provider is unavailable".to_owned(),
            ));
        }
        if self.worker.is_none() {
            return Some(StrategyRuntimeWritePortError::Unavailable(
                "strategy PineTS worker is unavailable".to_owned(),
            ));
        }
        None
    }

    fn cancel(&self, instance_id: &str) {
        let task = self
            .tasks
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .remove(instance_id);
        if let Some(mut task) = task {
            task.cancel.store(true, Ordering::Release);
            // Do not hold the task-map mutex while waiting for the worker;
            // the worker may need to finish a final observation write.
            if let Some(join) = task._join.take() {
                let _ = join.join();
            }
        }
    }

    /// Stop all live strategy tasks before external workers and SQLite leases
    /// are torn down.  Joining here is bounded by the Pine client's request
    /// timeout and prevents a task from retaining a store lease after shutdown.
    pub(crate) fn shutdown(&self) {
        let tasks = std::mem::take(&mut *self.tasks.lock().unwrap_or_else(|e| e.into_inner()));
        let instance_ids = tasks.keys().cloned().collect::<Vec<_>>();
        for task in tasks.values() {
            task.cancel.store(true, Ordering::Release);
        }
        for (_, mut task) in tasks {
            if let Some(join) = task._join.take() {
                let _ = join.join();
            }
        }
        // A shutdown may happen without an explicit Stop mutation.  Release
        // every consumer after its task has joined so the router cannot keep
        // stale strategy demand alive while the provider is being torn down.
        for instance_id in instance_ids {
            self.release_demand(&instance_id);
        }
    }

    fn release_demand(&self, instance_id: &str) {
        if let Some(router) = &self.router {
            let now = now_millis();
            let mut router = router.lock().unwrap_or_else(|e| e.into_inner());
            let _ = router.release_demand_consumer_with_time(instance_id, now);
        }
    }

    fn acquire_demand(
        &self,
        instance_id: &str,
        binding: &Value,
    ) -> Result<(), StrategyRuntimeWritePortError> {
        let Some(router) = &self.router else {
            return Ok(());
        };
        let refs = binding_symbols(binding)
            .unwrap_or_default()
            .into_iter()
            .map(|symbol| {
                let (market, symbol) = match symbol.split_once('.') {
                    Some((market, symbol)) => (market.to_owned(), symbol.to_owned()),
                    None => (
                        binding_string_opt(binding, &["market"]).unwrap_or_else(|| "US".to_owned()),
                        symbol,
                    ),
                };
                InstrumentRef {
                    channel: "KLINE".to_owned(),
                    market,
                    symbol,
                    interval: Some(
                        binding_string_opt(binding, &["interval", "timeframe"])
                            .unwrap_or_else(|| "1m".to_owned()),
                    ),
                }
            })
            .collect::<Vec<_>>();
        if refs.is_empty() {
            return Err(StrategyRuntimeWritePortError::Failed {
                status: 400,
                code: "STRATEGY_SYMBOLS_REQUIRED".to_owned(),
                message: "strategy binding requires at least one symbol".to_owned(),
            });
        }
        let managed =
            self.provider.snapshot().provider == Some(jftrade_settings::MarketDataProvider::Futu);
        router
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .acquire_demand(instance_id, refs, managed, now_millis())
            .map(|_| ())
            .map_err(|error| StrategyRuntimeWritePortError::Unavailable(error.to_string()))
    }

    fn spawn_task(
        &self,
        instance_id: String,
        binding: Value,
        store: Arc<StrategyRuntimeStore>,
    ) -> Result<(), StrategyRuntimeWritePortError> {
        let Some(worker) = self.worker.clone() else {
            return Err(StrategyRuntimeWritePortError::Unavailable(
                "strategy PineTS worker is unavailable".to_owned(),
            ));
        };
        let router = self.router.clone();
        let script = binding_string_opt(&binding, &["script", "source"]).unwrap_or_default();
        let active_symbols = binding_symbols(&binding).unwrap_or_default();
        let cancel = Arc::new(AtomicBool::new(false));
        let cancel_for_thread = Arc::clone(&cancel);
        let id_for_thread = instance_id.clone();
        let join = std::thread::Builder::new()
            .name(format!("strategy-runtime-{instance_id}"))
            .spawn(move || {
                if !script.is_empty() {
                    let result = match tokio::runtime::Runtime::new() {
                        Ok(runtime) => runtime
                            .block_on(worker.analyze_script(
                                &format!("strategy-{id_for_thread}"),
                                &id_for_thread,
                                &script,
                                false,
                            ))
                            .err(),
                        Err(error) => Some(PineExecutionError::Transport(format!(
                            "create strategy runtime executor: {error}"
                        ))),
                    };
                    if cancel_for_thread.load(Ordering::Acquire) {
                        return;
                    }
                    if let Some(error) = result {
                        let message = pine_error_message(error);
                        let _ = store.update_observation(
                            &id_for_thread,
                            "FAILED",
                            &active_symbols,
                            Some(&message),
                            now_millis(),
                        );
                        let _ = store.update_status(&id_for_thread, "FAILED", &now_rfc3339());
                        if let Some(router) = router.as_ref() {
                            let _ = router
                                .lock()
                                .unwrap_or_else(|e| e.into_inner())
                                .release_demand_consumer_with_time(&id_for_thread, now_millis());
                        }
                        return;
                    }
                }
                let _ = store.update_observation(
                    &id_for_thread,
                    "RUNNING",
                    &active_symbols,
                    None,
                    now_millis(),
                );
                while !cancel_for_thread.load(Ordering::Acquire) {
                    std::thread::sleep(std::time::Duration::from_millis(100));
                }
            })
            .map_err(|error| {
                StrategyRuntimeWritePortError::Unavailable(format!(
                    "start strategy runtime task: {error}"
                ))
            })?;
        self.tasks.lock().unwrap_or_else(|e| e.into_inner()).insert(
            instance_id,
            RuntimeTask {
                cancel,
                _join: Some(join),
            },
        );
        Ok(())
    }
}

impl Drop for StrategyRuntimeManager {
    fn drop(&mut self) {
        self.shutdown();
    }
}

fn now_millis() -> i64 {
    time::OffsetDateTime::now_utc().unix_timestamp_nanos() as i64 / 1_000_000
}
fn now_rfc3339() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}
fn pine_error_message(error: PineExecutionError) -> String {
    error.to_string()
}

impl From<jftrade_store_sqlite::StrategyRuntimeStoreError> for StrategyRuntimeWritePortError {
    fn from(error: jftrade_store_sqlite::StrategyRuntimeStoreError) -> Self {
        let message = error.to_string();
        let (status, code) = match error {
            jftrade_store_sqlite::StrategyRuntimeStoreError::NotFound => (404, "NOT_FOUND"),
            jftrade_store_sqlite::StrategyRuntimeStoreError::Conflict => (409, "CONFLICT"),
            jftrade_store_sqlite::StrategyRuntimeStoreError::Validation(_) => {
                (400, "VALIDATION_FAILED")
            }
            _ => (500, "STRATEGY_RUNTIME_MUTATION_FAILED"),
        };
        Self::Failed {
            status,
            code: code.to_owned(),
            message,
        }
    }
}

#[derive(Debug)]
pub(crate) struct ProductionStrategyRuntimePort {
    pub(crate) store: Arc<StrategyRuntimeStore>,
    pub(crate) definitions: Arc<StrategyDefinitionStore>,
    pub(crate) manager: Arc<StrategyRuntimeManager>,
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
    fn effective_binding(
        &self,
        instance: &jftrade_store_sqlite::StoredRuntimeInstance,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        let mut binding = instance.binding.clone();
        let object =
            binding
                .as_object_mut()
                .ok_or_else(|| StrategyRuntimeWritePortError::Failed {
                    status: 400,
                    code: "STRATEGY_BINDING_INVALID".to_owned(),
                    message: "strategy binding must be an object".to_owned(),
                })?;
        if let Some(definition_id) = instance.definition_id.as_deref() {
            if let Some(definition) = self
                .definitions
                .get_definition(definition_id, false)
                .map_err(|error| StrategyRuntimeWritePortError::Failed {
                    status: 500,
                    code: "STRATEGY_DEFINITION_READ_FAILED".to_owned(),
                    message: error.to_string(),
                })?
            {
                if !object.contains_key("script") && !definition.script.trim().is_empty() {
                    object.insert("script".to_owned(), Value::String(definition.script));
                }
                if !object.contains_key("symbol") && !definition.symbol.trim().is_empty() {
                    object.insert("symbols".to_owned(), json!([definition.symbol]));
                }
                if !object.contains_key("interval") && !definition.interval.trim().is_empty() {
                    object.insert("interval".to_owned(), Value::String(definition.interval));
                }
            }
        }
        Ok(binding)
    }

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
        object.insert(
            "runtimeActive".to_owned(),
            Value::Bool(instance.runtime_active),
        );
        if !instance.plugin_id.is_empty() {
            object.insert(
                "pluginId".to_owned(),
                Value::String(instance.plugin_id.clone()),
            );
        }
        if let Some(created_at) = instance
            .created_at
            .clone()
            .or_else(|| (!instance.updated_at.is_empty()).then_some(instance.updated_at.clone()))
        {
            object.insert("createdAt".to_owned(), Value::String(created_at));
        }
        if !instance.updated_at.is_empty() {
            object.insert(
                "updatedAt".to_owned(),
                Value::String(instance.updated_at.clone()),
            );
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
                .or_else(|| {
                    binding_string_opt(&instance.binding, &["definitionName", "strategyName"])
                })
                .unwrap_or_default();
            let definition_version = instance
                .definition_version
                .clone()
                .or_else(|| {
                    binding_string_opt(&instance.binding, &["definitionVersion", "version"])
                })
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
            object.insert(
                "sourceFormat".to_owned(),
                Value::String(source_format.to_owned()),
            );
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
            params.insert(
                "sourceFormat".to_owned(),
                Value::String(source_format.to_owned()),
            );
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
        object.insert(
            "logs".to_owned(),
            Value::Array(logs.into_iter().map(Value::String).collect()),
        );
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
        self.from_ms.is_none_or(|from| at_ms >= from) && self.to_ms.is_none_or(|to| at_ms <= to)
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
    let invalid = || StrategyReadSnapshotError::Invalid(format!("invalid {activity} query"));
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
    let timestamp =
        time::OffsetDateTime::parse(value, &time::format_description::well_known::Rfc3339)
            .map_err(|_| invalid)?;
    i64::try_from(timestamp.unix_timestamp_nanos() / 1_000_000).map_err(|_| {
        StrategyReadSnapshotError::Invalid("strategy activity timestamp is out of range".to_owned())
    })
}

fn timestamp_from_millis(value: i64) -> Result<String, StrategyReadSnapshotError> {
    let timestamp = time::OffsetDateTime::from_unix_timestamp_nanos(i128::from(value) * 1_000_000)
        .map_err(|error| StrategyReadSnapshotError::Unavailable(error.to_string()))?;
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
            let binding_definition_name = instance.definition_name.clone().unwrap_or_else(|| {
                binding_string(&instance.binding, &["definitionName", "strategyName"])
            });
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
    ["activeSymbols", "symbols"]
        .iter()
        .find_map(|key| {
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
        .or_else(|| {
            binding
                .get("symbol")
                .and_then(Value::as_str)
                .map(|symbol| vec![symbol.trim().to_owned()])
                .filter(|symbols| !symbols[0].is_empty())
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

        let current = self
            .store
            .get_instance(&input.instance_id)
            .map_err(|e| StrategyRuntimeWritePortError::Failed {
                status: 500,
                code: "STRATEGY_RUNTIME_READ_FAILED".to_owned(),
                message: e.to_string(),
            })?
            .ok_or_else(|| StrategyRuntimeWritePortError::Failed {
                status: 404,
                code: "NOT_FOUND".to_owned(),
                message: "strategy instance not found".to_owned(),
            })?;
        let result = match input.operation {
            StrategyRuntimeWriteOperation::Start => {
                if current.runtime_active || current.status.eq_ignore_ascii_case("RUNNING") {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 409,
                        code: "CONFLICT".to_owned(),
                        message: "strategy instance is already running".to_owned(),
                    });
                }
                if let Some(error) = self.manager.dependency_error() {
                    return Err(error);
                }
                let runtime_binding = self.effective_binding(&current)?;
                self.manager
                    .acquire_demand(&input.instance_id, &runtime_binding)?;
                match self
                    .store
                    .update_status(&input.instance_id, "RUNNING", &timestamp)
                {
                    Ok(instance) => {
                        if let Err(error) = self.manager.spawn_task(
                            input.instance_id.clone(),
                            runtime_binding,
                            Arc::clone(&self.store),
                        ) {
                            let _ =
                                self.store
                                    .update_status(&input.instance_id, "STOPPED", &timestamp);
                            self.manager.release_demand(&input.instance_id);
                            Err(error)
                        } else {
                            Ok(instance)
                        }
                    }
                    Err(error) => {
                        self.manager.release_demand(&input.instance_id);
                        Err(error.into())
                    }
                }
            }
            StrategyRuntimeWriteOperation::Stop | StrategyRuntimeWriteOperation::Pause => {
                if input.operation == StrategyRuntimeWriteOperation::Pause
                    && !current.status.eq_ignore_ascii_case("RUNNING")
                {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 409,
                        code: "CONFLICT".to_owned(),
                        message: "strategy instance is not running".to_owned(),
                    });
                }
                if input.operation == StrategyRuntimeWriteOperation::Stop
                    && current.status.eq_ignore_ascii_case("STOPPED")
                {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 409,
                        code: "CONFLICT".to_owned(),
                        message: "strategy instance is already stopped".to_owned(),
                    });
                }
                self.manager.cancel(&input.instance_id);
                self.manager.release_demand(&input.instance_id);
                let status = if input.operation == StrategyRuntimeWriteOperation::Pause {
                    "PAUSED"
                } else {
                    "STOPPED"
                };
                self.store
                    .update_status(&input.instance_id, status, &timestamp)
                    .map_err(Into::into)
            }
            StrategyRuntimeWriteOperation::Delete => {
                if current.runtime_active || current.status == "RUNNING" {
                    return Err(StrategyRuntimeWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "strategy instance is busy".to_owned(),
                    });
                }
                self.manager.cancel(&input.instance_id);
                self.manager.release_demand(&input.instance_id);
                self.store
                    .delete_instance(&input.instance_id, &timestamp)
                    .map_err(Into::into)
            }
            StrategyRuntimeWriteOperation::Update => {
                let binding =
                    input
                        .binding
                        .clone()
                        .ok_or_else(|| StrategyRuntimeWritePortError::Failed {
                            status: 400,
                            code: "BAD_REQUEST".to_owned(),
                            message: "strategy binding is required".to_owned(),
                        })?;
                let was_running = current.runtime_active || current.status == "RUNNING";
                if was_running {
                    self.manager.cancel(&input.instance_id);
                    self.manager.release_demand(&input.instance_id);
                }
                let updated =
                    match self
                        .store
                        .update_binding(&input.instance_id, binding.clone(), &timestamp)
                    {
                        Ok(updated) => updated,
                        Err(error) => {
                            if was_running {
                                let _ = self.store.update_status(
                                    &input.instance_id,
                                    "STOPPED",
                                    &timestamp,
                                );
                            }
                            return Err(error.into());
                        }
                    };
                if was_running {
                    if let Some(error) = self.manager.dependency_error() {
                        let _ = self
                            .store
                            .update_status(&input.instance_id, "STOPPED", &timestamp);
                        return Err(error);
                    }
                    if let Err(error) = self.manager.acquire_demand(&input.instance_id, &binding) {
                        let _ = self
                            .store
                            .update_status(&input.instance_id, "STOPPED", &timestamp);
                        return Err(error);
                    }
                    let running =
                        match self
                            .store
                            .update_status(&input.instance_id, "RUNNING", &timestamp)
                        {
                            Ok(running) => running,
                            Err(error) => {
                                self.manager.release_demand(&input.instance_id);
                                return Err(error.into());
                            }
                        };
                    match self.manager.spawn_task(
                        input.instance_id.clone(),
                        binding,
                        Arc::clone(&self.store),
                    ) {
                        Ok(()) => Ok(running),
                        Err(error) => {
                            let _ =
                                self.store
                                    .update_status(&input.instance_id, "STOPPED", &timestamp);
                            self.manager.release_demand(&input.instance_id);
                            Err(error)
                        }
                    }
                } else {
                    Ok(updated)
                }
            }
            StrategyRuntimeWriteOperation::UpdateRuntimeRisk => {
                let risk = input.runtime_risk.clone().ok_or_else(|| {
                    StrategyRuntimeWritePortError::Failed {
                        status: 400,
                        code: "BAD_REQUEST".to_owned(),
                        message: "runtime risk is required".to_owned(),
                    }
                })?;
                self.store
                    .update_risk(&input.instance_id, risk, &timestamp)
                    .map_err(Into::into)
            }
            StrategyRuntimeWriteOperation::RefreshDefinition => {
                let was_running = current.runtime_active || current.status == "RUNNING";
                if was_running {
                    self.manager.cancel(&input.instance_id);
                    self.manager.release_demand(&input.instance_id);
                }
                let refreshed = match self
                    .store
                    .refresh_definition(&input.instance_id, &timestamp)
                {
                    Ok(refreshed) => refreshed,
                    Err(error) => {
                        if was_running {
                            let _ =
                                self.store
                                    .update_status(&input.instance_id, "STOPPED", &timestamp);
                        }
                        return Err(error.into());
                    }
                };
                if was_running {
                    if let Some(error) = self.manager.dependency_error() {
                        let _ = self
                            .store
                            .update_status(&input.instance_id, "STOPPED", &timestamp);
                        return Err(error);
                    }
                    let runtime_binding = match self.effective_binding(&refreshed) {
                        Ok(binding) => binding,
                        Err(error) => {
                            let _ =
                                self.store
                                    .update_status(&input.instance_id, "STOPPED", &timestamp);
                            return Err(error);
                        }
                    };
                    if let Err(error) = self
                        .manager
                        .acquire_demand(&input.instance_id, &runtime_binding)
                    {
                        let _ = self
                            .store
                            .update_status(&input.instance_id, "STOPPED", &timestamp);
                        return Err(error);
                    }
                    let running =
                        match self
                            .store
                            .update_status(&input.instance_id, "RUNNING", &timestamp)
                        {
                            Ok(running) => running,
                            Err(error) => {
                                self.manager.release_demand(&input.instance_id);
                                return Err(error.into());
                            }
                        };
                    match self.manager.spawn_task(
                        input.instance_id.clone(),
                        runtime_binding,
                        Arc::clone(&self.store),
                    ) {
                        Ok(()) => Ok(running),
                        Err(error) => {
                            let _ =
                                self.store
                                    .update_status(&input.instance_id, "STOPPED", &timestamp);
                            self.manager.release_demand(&input.instance_id);
                            Err(error)
                        }
                    }
                } else {
                    Ok(refreshed)
                }
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
            Err(error) => Err(error),
        }
    }
}
