//! Production strategy runtime adapter.

use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_execution_write_port::ExecutionWritePort;
use crate::product::product_strategy_runtime_write_port::{
    StrategyRuntimeWriteInput, StrategyRuntimeWriteOperation, StrategyRuntimeWritePort,
    StrategyRuntimeWritePortError,
};
use crate::product::{
    MarketDataQuoteReadSnapshotPort, ProductNotificationPort,
    StrategyReadSnapshotError, StrategyReadSnapshotPort, StrategyRuntimeStatusPort,
    StrategyRuntimeSummary,
};
use jftrade_integration_pine::{
    GrpcPineExecutionPort, PineExecutionError, PineRunRequest,
};
use jftrade_marketdata::{InstrumentRef, ProviderRouter};
use jftrade_store_sqlite::{StrategyDefinitionStore, StrategyRuntimeStore};
use serde_json::{Value, json};
use std::collections::{BTreeMap, BTreeSet};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;

#[derive(Debug)]
struct RuntimeTask {
    cancel: Arc<AtomicBool>,
    wake: Arc<tokio::sync::Notify>,
    _join: Option<JoinHandle<()>>,
}

#[derive(Debug)]
struct SymbolSessionState {
    revision: u64,
    submitted_intents: BTreeSet<String>,
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
    quote: Option<Arc<dyn MarketDataQuoteReadSnapshotPort>>,
    execution: Option<Arc<dyn ExecutionWritePort>>,
    provider: Arc<ActiveProviderState>,
    notification: Option<Arc<dyn ProductNotificationPort>>,
}

impl StrategyRuntimeManager {
    pub(crate) fn new(
        router: Option<Arc<Mutex<ProviderRouter>>>,
        worker: Option<Arc<GrpcPineExecutionPort>>,
        quote: Option<Arc<dyn MarketDataQuoteReadSnapshotPort>>,
        execution: Option<Arc<dyn ExecutionWritePort>>,
        provider: Arc<ActiveProviderState>,
    ) -> Self {
        Self {
            tasks: Mutex::new(BTreeMap::new()),
            router,
            worker,
            quote,
            execution,
            provider,
            notification: None,
        }
    }

    pub(crate) fn with_notification(
        mut self,
        notification: Arc<dyn ProductNotificationPort>,
    ) -> Self {
        self.notification = Some(notification);
        self
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
        if self.quote.is_none() {
            return Some(StrategyRuntimeWritePortError::Unavailable(
                "strategy market-data quote port is unavailable".to_owned(),
            ));
        }
        None
    }

    fn wake(&self, instance_id: &str) {
        if let Some(task) = self
            .tasks
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .get(instance_id)
        {
            task.wake.notify_waiters();
        }
    }

    fn is_task_alive(&self, instance_id: &str) -> bool {
        self.tasks
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .contains_key(instance_id)
    }

    fn cancel(&self, instance_id: &str) {
        let task = self
            .tasks
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .remove(instance_id);
        if let Some(mut task) = task {
            task.cancel.store(true, Ordering::Release);
            task.wake.notify_waiters();
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
            task.wake.notify_waiters();
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
        let Some(quote) = self.quote.clone() else {
            return Err(StrategyRuntimeWritePortError::Unavailable(
                "strategy market-data quote port is unavailable".to_owned(),
            ));
        };
        let execution = self.execution.clone();
        let notification = self.notification.clone();
        let provider = Arc::clone(&self.provider);
        let router = self.router.clone();
        let script = binding_string_opt(&binding, &["script", "source"]);
        let script = script.ok_or_else(|| StrategyRuntimeWritePortError::Failed {
            status: 400,
            code: "STRATEGY_SCRIPT_REQUIRED".to_owned(),
            message: "strategy runtime requires a non-empty Pine script".to_owned(),
        })?;
        let active_symbols = binding_symbols(&binding).unwrap_or_default();
        if active_symbols.is_empty() {
            return Err(StrategyRuntimeWritePortError::Failed {
                status: 400,
                code: "STRATEGY_SYMBOLS_REQUIRED".to_owned(),
                message: "strategy runtime requires at least one symbol".to_owned(),
            });
        }
        let timeframe = binding_string_opt(&binding, &["interval", "timeframe"])
            .unwrap_or_else(|| "1m".to_owned());
        let script_id = binding_string_opt(&binding, &["scriptId", "definitionId", "strategyId"])
            .unwrap_or_else(|| instance_id.clone());
        let default_market = binding_string_opt(&binding, &["market"])
            .unwrap_or_else(|| "US".to_owned());
        let candle_limit = binding
            .get("candleLimit")
            .or_else(|| binding.get("limit"))
            .and_then(Value::as_u64)
            .map(|value| value.clamp(1, 1_000) as usize)
            .unwrap_or(200);
        let sessions = binding_sessions(&binding);
        let execution_mode = binding_string_opt(&binding, &["executionMode"]);
        let execute_orders = match execution_mode.as_deref() {
            Some("notify_only") => false,
            Some("live") => true,
            _ => binding
                .get("executeOrders")
                .and_then(Value::as_bool)
                .unwrap_or(true),
        };
        if execute_orders {
            if execution.is_none() {
                return Err(StrategyRuntimeWritePortError::Unavailable(
                    "strategy execution order port is unavailable".to_owned(),
                ));
            }
            validate_strategy_execution_binding(&binding, &provider).map_err(|message| {
                StrategyRuntimeWritePortError::Failed {
                    status: 400,
                    code: "STRATEGY_EXECUTION_BINDING_INVALID".to_owned(),
                    message,
                }
            })?;
        }
        let cancel = Arc::new(AtomicBool::new(false));
        let cancel_for_thread = Arc::clone(&cancel);
        let wake = Arc::new(tokio::sync::Notify::new());
        let wake_for_thread = Arc::clone(&wake);
        let id_for_thread = instance_id.clone();
        let join = std::thread::Builder::new()
            .name(format!("strategy-runtime-{instance_id}"))
            .spawn(move || {
                let runtime = match tokio::runtime::Runtime::new() {
                    Ok(runtime) => runtime,
                    Err(error) => {
                        fail_strategy_task(
                            &store,
                            &router,
                            &id_for_thread,
                            &active_symbols,
                            format!("create strategy runtime executor: {error}"),
                        );
                        return;
                    }
                };
                let mut last_closed_by_symbol = BTreeMap::<String, i64>::new();
                let mut session_state_by_symbol = BTreeMap::<String, SymbolSessionState>::new();
                let mut first_cycle = true;
                while !cancel_for_thread.load(Ordering::Acquire) {
                    let current_instance = match store.get_instance(&id_for_thread) {
                        Ok(Some(inst)) => inst,
                        Ok(None) => break,
                        Err(_) => {
                            sleep_until_next_strategy_poll(&cancel_for_thread);
                            continue;
                        }
                    };
                    if current_instance.status.eq_ignore_ascii_case("PAUSED") {
                        runtime.block_on(async {
                            tokio::select! {
                                _ = wake_for_thread.notified() => {}
                                _ = tokio::time::sleep(std::time::Duration::from_millis(500)) => {}
                            }
                        });
                        continue;
                    }
                    if !current_instance.status.eq_ignore_ascii_case("RUNNING")
                        && !current_instance.status.eq_ignore_ascii_case("STARTING")
                    {
                        break;
                    }
                    let current_risk_revision = Some(current_instance.runtime_risk_revision);
                    let mut last_closed = None;
                    let mut last_signal = None;
                    let mut last_order = None;
                    let mut cycle_error = None;
                    for requested_symbol in &active_symbols {
                        if cancel_for_thread.load(Ordering::Acquire) {
                            break;
                        }
                        let (market, symbol) =
                            split_strategy_symbol(requested_symbol, &default_market);
                        let candles = match runtime.block_on(read_strategy_candles(
                            quote.as_ref(),
                            &market,
                            &symbol,
                            &timeframe,
                            candle_limit,
                            &sessions,
                        )) {
                            Ok(candles) => candles,
                            Err(error) => {
                                cycle_error = Some(error);
                                break;
                            }
                        };
                        let Some(latest) = candles.last() else {
                            continue;
                        };
                        let latest_open_time = latest.open_time;
                        let latest_close = latest.close;
                        last_closed = Some(last_closed.map_or(latest_open_time, |value: i64| {
                            value.max(latest_open_time)
                        }));
                        if !first_cycle
                            && last_closed_by_symbol
                                .get(requested_symbol)
                                .is_some_and(|previous| *previous >= latest_open_time)
                        {
                            continue;
                        }
                        let latest_bar_index = i32::try_from(candles.len().saturating_sub(1))
                            .unwrap_or(i32::MAX);
                        let session = session_state_by_symbol
                            .entry(requested_symbol.clone())
                            .or_insert_with(|| SymbolSessionState {
                                revision: 0,
                                submitted_intents: BTreeSet::new(),
                            });
                        let (session_operation, expected_revision, candles_to_send) =
                            if session.revision == 0 {
                                ("open".to_owned(), 0, candles)
                            } else {
                                ("append".to_owned(), session.revision, vec![latest.clone()])
                            };
                        let request = PineRunRequest {
                            job_id: format!(
                                "live:{id_for_thread}:{symbol}:{}",
                                latest_open_time
                            ),
                            script_id: script_id.clone(),
                            source: script.clone(),
                            symbol: format!("{market}.{symbol}"),
                            timeframe: timeframe.clone(),
                            chart_type: binding_string_opt(&binding, &["chartType"])
                                .unwrap_or_else(|| "standard".to_owned()),
                            mode: "live".to_owned(),
                            candles: candles_to_send,
                            params: binding_params(&binding),
                            session_id: format!("strategy:{id_for_thread}:{symbol}"),
                            session_operation,
                            expected_revision,
                        };
                        let response = match runtime.block_on(worker.run_script(request)) {
                            Ok(response) => {
                                session.revision =
                                    response.session_revision.max(session.revision + 1);
                                response
                            }
                            Err(error) => {
                                let was_append = session.revision > 0;
                                session.revision = 0;
                                if was_append {
                                    let err_msg = pine_error_message(error);
                                    let _ = store.append_audit_event(
                                        &id_for_thread,
                                        "SESSION_APPEND_RETRY",
                                        &format!("Pine session append failed, resetting to open: {err_msg}"),
                                        now_millis(),
                                    );
                                    continue;
                                }
                                cycle_error = Some(pine_error_message(error));
                                break;
                            }
                        };
                        let raw_intents = current_bar_intents(
                            &response.order_intents,
                            latest_bar_index,
                            latest_open_time,
                        );
                        let mut current_intents = Vec::new();
                        for intent in raw_intents {
                            let key = format!("{}:{}:{}", latest_open_time, intent.id, intent.bar_index);
                            if !session.submitted_intents.contains(&key) {
                                session.submitted_intents.insert(key);
                                current_intents.push(intent);
                            }
                        }
                        if !current_intents.is_empty() {
                            last_signal = Some(latest_open_time);
                            if execute_orders {
                                match execute_strategy_intents(
                                    StrategyExecutionContext {
                                        execution: execution.as_deref(),
                                        provider: &provider,
                                        store: &store,
                                        instance_id: &id_for_thread,
                                        market: &market,
                                        symbol: &symbol,
                                        binding: &binding,
                                        expected_risk_revision: current_risk_revision,
                                        fallback_price: Some(latest_close),
                                        sellable_quantity: None,
                                    },
                                    &current_intents,
                                ) {
                                    Ok(true) => last_order = Some(latest_open_time),
                                    Ok(false) => {}
                                    Err(error) => {
                                        cycle_error = Some(error);
                                        break;
                                    }
                                }
                            } else if let Err(error) = notify_strategy_intents(
                                notification.as_deref(),
                                &store,
                                &id_for_thread,
                                &format!("{market}.{symbol}"),
                                &current_intents,
                            ) {
                                cycle_error = Some(error);
                                break;
                            }
                        }
                        if let Err(error) =
                            record_worker_output(&store, &id_for_thread, &response, latest_open_time)
                        {
                            cycle_error = Some(format!("persist Pine worker output: {error}"));
                            break;
                        }
                        last_closed_by_symbol.insert(requested_symbol.clone(), latest_open_time);
                    }
                    first_cycle = false;
                    if let Some(error) = cycle_error {
                        let is_paused = store
                            .get_instance(&id_for_thread)
                            .ok()
                            .flatten()
                            .is_some_and(|inst| inst.status.eq_ignore_ascii_case("PAUSED"));
                        if is_paused {
                            continue;
                        }
                        close_strategy_pine_sessions(
                            &runtime,
                            worker.as_ref(),
                            &store,
                            &id_for_thread,
                            &script_id,
                            &script,
                            &default_market,
                            &timeframe,
                            &binding,
                            &session_state_by_symbol,
                        );
                        fail_strategy_task(
                            &store,
                            &router,
                            &id_for_thread,
                            &active_symbols,
                            error,
                        );
                        return;
                    }
                    let _ = store.update_observation_with_events(
                        &id_for_thread,
                        "RUNNING",
                        &active_symbols,
                        None,
                        last_closed,
                        last_signal,
                        last_order,
                        now_millis(),
                    );
                    sleep_until_next_strategy_poll(&cancel_for_thread);
                }
                close_strategy_pine_sessions(
                    &runtime,
                    worker.as_ref(),
                    &store,
                    &id_for_thread,
                    &script_id,
                    &script,
                    &default_market,
                    &timeframe,
                    &binding,
                    &session_state_by_symbol,
                );
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
                wake,
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
fn now_rfc3339() -> Result<String, String> {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .map_err(|error| format!("format strategy runtime timestamp: {error}"))
}
fn pine_error_message(error: PineExecutionError) -> String {
    error.to_string()
}

#[allow(clippy::too_many_arguments)]
fn close_strategy_pine_sessions(
    runtime: &tokio::runtime::Runtime,
    worker: &GrpcPineExecutionPort,
    store: &StrategyRuntimeStore,
    instance_id: &str,
    script_id: &str,
    script: &str,
    default_market: &str,
    timeframe: &str,
    binding: &Value,
    sessions: &BTreeMap<String, SymbolSessionState>,
) {
    for (requested_symbol, session) in sessions {
        if session.revision > 0 {
            let (market, symbol) = split_strategy_symbol(requested_symbol, default_market);
            let request = PineRunRequest {
                job_id: format!("close:{instance_id}:{symbol}:{}", now_millis()),
                script_id: script_id.to_owned(),
                source: script.to_owned(),
                symbol: format!("{market}.{symbol}"),
                timeframe: timeframe.to_owned(),
                chart_type: "standard".to_owned(),
                mode: "live".to_owned(),
                candles: Vec::new(),
                params: binding_params(binding),
                session_id: format!("strategy:{instance_id}:{symbol}"),
                session_operation: "close".to_owned(),
                expected_revision: session.revision,
            };
            if let Err(err) = runtime.block_on(worker.run_script(request)) {
                let _ = store.append_audit_event(
                    instance_id,
                    "SESSION_CLOSE_FAILED",
                    &format!("close session for {symbol} failed: {err}"),
                    now_millis(),
                );
            }
        }
    }
}

fn strategy_write_error_message(error: StrategyRuntimeWritePortError) -> String {
    match error {
        StrategyRuntimeWritePortError::Unavailable(message) => message,
        StrategyRuntimeWritePortError::Failed {
            status,
            code,
            message,
        } => format!("strategy runtime failed ({status} {code}): {message}"),
    }
}

fn fail_strategy_task(
    store: &StrategyRuntimeStore,
    router: &Option<Arc<Mutex<ProviderRouter>>>,
    instance_id: &str,
    active_symbols: &[String],
    message: String,
) {
    let now = now_millis();
    let _ = store.update_observation_with_events(
        instance_id,
        "FAILED",
        active_symbols,
        Some(&message),
        None,
        None,
        None,
        now,
    );
    let _ = store.append_log_event(instance_id, &message, "error", now);
    match now_rfc3339() {
        Ok(timestamp) => {
            let _ = store.update_status(instance_id, "FAILED", &timestamp);
        }
        Err(error) => {
            let _ = store.append_log_event(instance_id, &error, "error", now);
        }
    }
    if let Some(router) = router.as_ref() {
        let _ = router
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .release_demand_consumer_with_time(instance_id, now);
    }
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

#[path = "strategy_runtime_port.rs"]
mod strategy_runtime_port;
#[path = "strategy_runtime_activity.rs"]
mod strategy_runtime_activity;
#[path = "strategy_runtime_mutation.rs"]
mod strategy_runtime_mutation;
#[path = "strategy_runtime_execution.rs"]
mod strategy_runtime_execution;
#[path = "strategy_runtime_candles.rs"]
mod strategy_runtime_candles;
use strategy_runtime_activity::*;
use strategy_runtime_candles::*;
use strategy_runtime_execution::*;

pub(crate) use strategy_runtime_port::ProductionStrategyRuntimePort;
