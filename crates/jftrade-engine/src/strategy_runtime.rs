//! Production strategy runtime adapter.

use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_execution_write_port::ExecutionWritePort;
use crate::product::product_strategy_runtime_write_port::{
    StrategyRuntimeWriteInput, StrategyRuntimeWriteOperation, StrategyRuntimeWritePort,
    StrategyRuntimeWritePortError,
};
use crate::product::{
    MarketDataQuoteReadSnapshotPort, ProductNotificationPort, StrategyReadSnapshotError,
    StrategyReadSnapshotPort, StrategyRuntimeStatusPort, StrategyRuntimeSummary,
};
use jftrade_integration_futu::{TradeFilter, TradeHeader};
use jftrade_integration_pine::{GrpcPineExecutionPort, PineExecutionError, PineRunRequest};
use jftrade_marketdata::{InstrumentRef, ProviderRouter};
use jftrade_store_sqlite::{ExecutionOrderStore, StrategyDefinitionStore, StrategyRuntimeStore};
use serde_json::{Value, json};
use std::collections::{BTreeMap, BTreeSet};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;

pub(crate) const STRATEGY_STOP_TIMEOUT: std::time::Duration = std::time::Duration::from_secs(3);
pub(crate) const STRATEGY_SHUTDOWN_TIMEOUT: std::time::Duration = std::time::Duration::from_secs(5);

#[derive(Debug)]
struct RuntimeTask {
    cancel: Arc<AtomicBool>,
    wake: Arc<tokio::sync::Notify>,
    done_rx: std::sync::mpsc::Receiver<()>,
    thread_handle: Option<JoinHandle<()>>,
}

type RuntimeTaskMap = BTreeMap<String, RuntimeTask>;

#[derive(Debug)]
struct SymbolSessionState {
    revision: u64,
    bar_count: usize,
    submitted_intents: BTreeSet<String>,
}

/// In-memory worker task supervisor for live strategy instances.
///
/// Holds cancellation handles and demand-token tracking. Each task runs on a
/// dedicated thread with its own Tokio runtime; lifecycle mutations publish
/// the persisted CAS transition before applying cancellation side effects.
#[derive(Debug)]
pub(crate) struct StrategyRuntimeManager {
    mutation_lock: Mutex<()>,
    stopping: AtomicBool,
    tasks: Mutex<RuntimeTaskMap>,
    reclaiming_tasks: Mutex<RuntimeTaskMap>,
    router: Option<Arc<Mutex<ProviderRouter>>>,
    worker: Option<Arc<dyn jftrade_integration_pine::PineExecutionPort>>,
    quote: Option<Arc<dyn MarketDataQuoteReadSnapshotPort>>,
    execution: Option<Arc<dyn ExecutionWritePort>>,
    execution_store: Option<Arc<ExecutionOrderStore>>,
    trade_runtime: Option<Arc<crate::product::product_production_ports::SharedTradeReadRuntime>>,
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
            mutation_lock: Mutex::new(()),
            stopping: AtomicBool::new(false),
            tasks: Mutex::new(BTreeMap::new()),
            reclaiming_tasks: Mutex::new(BTreeMap::new()),
            router,
            worker: worker.map(|w| w as Arc<dyn jftrade_integration_pine::PineExecutionPort>),
            quote,
            execution,
            execution_store: None,
            trade_runtime: None,
            provider,
            notification: None,
        }
    }

    pub(crate) fn with_execution_store(
        mut self,
        execution_store: Arc<ExecutionOrderStore>,
    ) -> Self {
        self.execution_store = Some(execution_store);
        self
    }

    pub(crate) fn with_notification(
        mut self,
        notification: Arc<dyn ProductNotificationPort>,
    ) -> Self {
        self.notification = Some(notification);
        self
    }

    pub(crate) fn with_trade_runtime(
        mut self,
        trade_runtime: Option<
            Arc<crate::product::product_production_ports::SharedTradeReadRuntime>,
        >,
    ) -> Self {
        self.trade_runtime = trade_runtime;
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
        let wake = self
            .tasks
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .get(instance_id)
            .map(|task| Arc::clone(&task.wake))
            .or_else(|| {
                self.reclaiming_tasks
                    .lock()
                    .unwrap_or_else(|e| e.into_inner())
                    .get(instance_id)
                    .map(|task| Arc::clone(&task.wake))
            });
        if let Some(wake) = wake {
            wake.notify_waiters();
        }
    }

    fn is_task_alive(&self, instance_id: &str) -> bool {
        self.tasks
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .contains_key(instance_id)
            || self
                .reclaiming_tasks
                .lock()
                .unwrap_or_else(|e| e.into_inner())
                .contains_key(instance_id)
    }

    pub(crate) fn cancel(&self, instance_id: &str) -> bool {
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
            match task.done_rx.recv_timeout(STRATEGY_STOP_TIMEOUT) {
                Ok(()) | Err(std::sync::mpsc::RecvTimeoutError::Disconnected) => {
                    if let Some(join) = task.thread_handle.take() {
                        let _ = join.join();
                    }
                }
                Err(std::sync::mpsc::RecvTimeoutError::Timeout) => {
                    // The call is detached from lifecycle control after the
                    // deadline. Keep a reclaiming owner so later shutdown can
                    // still join it, but let duplicate stops return promptly.
                    tracing::warn!(
                        instance_id = %instance_id,
                        timeout_secs = STRATEGY_STOP_TIMEOUT.as_secs(),
                        "strategy runtime exceeded stop timeout; awaiting reclaim"
                    );
                    self.reclaiming_tasks
                        .lock()
                        .unwrap_or_else(|e| e.into_inner())
                        .insert(instance_id.to_owned(), task);
                    return false;
                }
            }
        }
        let reclaiming_task = self
            .reclaiming_tasks
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .remove(instance_id);
        if let Some(mut task) = reclaiming_task {
            match task.done_rx.try_recv() {
                Ok(()) | Err(std::sync::mpsc::TryRecvError::Disconnected) => {
                    if let Some(join) = task.thread_handle.take() {
                        let _ = join.join();
                    }
                    return true;
                }
                Err(std::sync::mpsc::TryRecvError::Empty) => {
                    self.reclaiming_tasks
                        .lock()
                        .unwrap_or_else(|e| e.into_inner())
                        .insert(instance_id.to_owned(), task);
                    return false;
                }
            }
        }
        true
    }

    /// Stop all live strategy tasks before external workers and SQLite leases
    /// are torn down. Joining here is bounded by a global deadline and
    /// prevents a task from hanging process termination or retaining a store lease.
    pub(crate) fn shutdown(&self) -> bool {
        self.stopping.store(true, Ordering::Release);
        let _mutation = self.mutation_lock.lock().unwrap_or_else(|e| e.into_inner());
        let mut tasks = std::mem::take(&mut *self.tasks.lock().unwrap_or_else(|e| e.into_inner()));
        tasks.extend(std::mem::take(
            &mut *self
                .reclaiming_tasks
                .lock()
                .unwrap_or_else(|e| e.into_inner()),
        ));
        for task in tasks.values() {
            task.cancel.store(true, Ordering::Release);
            task.wake.notify_waiters();
        }
        let deadline = std::time::Instant::now() + STRATEGY_SHUTDOWN_TIMEOUT;
        let mut joined_ids = Vec::new();
        for (id, mut task) in tasks {
            let remaining = deadline.saturating_duration_since(std::time::Instant::now());
            if remaining.is_zero() {
                tracing::warn!(
                    instance_id = %id,
                    "strategy shutdown deadline expired; detaching task"
                );
                joined_ids.push(id);
                continue;
            }
            match task.done_rx.recv_timeout(remaining) {
                Ok(()) | Err(std::sync::mpsc::RecvTimeoutError::Disconnected) => {
                    if let Some(join) = task.thread_handle.take() {
                        let _ = join.join();
                    }
                    joined_ids.push(id);
                }
                Err(std::sync::mpsc::RecvTimeoutError::Timeout) => {
                    tracing::warn!(
                        instance_id = %id,
                        "strategy task exceeded shutdown deadline; retaining task owner"
                    );
                    joined_ids.push(id);
                }
            }
        }
        // A shutdown may happen without an explicit Stop mutation. Release
        // every consumer after joining or detaching its task so the router
        // cannot keep stale strategy demand alive.
        for instance_id in joined_ids {
            self.release_demand(&instance_id);
        }
        let stopped = self
            .tasks
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .is_empty();
        self.stopping.store(false, Ordering::Release);
        stopped
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
        let managed = true;
        router
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .acquire_demand(instance_id, refs, managed, now_millis())
            .map(|_| ())
            .map_err(|error| StrategyRuntimeWritePortError::Unavailable(error.to_string()))
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

/// Rebuild the small amount of live Pine state that must survive an engine
/// restart.  The worker's JavaScript object graph is intentionally not
/// serialized; the durable checkpoint fences closed-candle replays and order
/// intents while a new worker session is warmed up.
type RestoredPineState = (BTreeMap<String, i64>, BTreeMap<String, SymbolSessionState>);

fn restore_pine_runtime_state(
    store: &StrategyRuntimeStore,
    instance_id: &str,
    active_symbols: &[String],
) -> Result<RestoredPineState, String> {
    let mut last_closed = BTreeMap::new();
    let mut sessions = BTreeMap::new();
    let scope = pine_checkpoint_scope(store, instance_id)?;
    let events = store
        .list_audit_events(instance_id)
        .map_err(|error| format!("read Pine recovery checkpoints: {error}"))?;
    let wanted = active_symbols
        .iter()
        .map(String::as_str)
        .collect::<BTreeSet<_>>();
    for event in events {
        if event.kind != "PINE_SESSION_CHECKPOINT" {
            continue;
        }
        let payload = serde_json::from_str::<Value>(&event.detail)
            .map_err(|error| format!("invalid Pine recovery checkpoint: {error}"))?;
        if payload.get("scope").and_then(Value::as_str) != Some(scope.as_str()) {
            continue;
        }
        let symbol = payload
            .get("symbol")
            .and_then(Value::as_str)
            .ok_or_else(|| "Pine recovery checkpoint is missing symbol".to_owned())?;
        if !wanted.contains(symbol) || last_closed.contains_key(symbol) {
            continue;
        }
        let open_time = payload
            .get("lastClosedOpenTime")
            .and_then(Value::as_i64)
            .ok_or_else(|| "Pine recovery checkpoint is missing lastClosedOpenTime".to_owned())?;
        last_closed.insert(symbol.to_owned(), open_time);
        let submitted_intents = payload
            .get("submittedIntentKeys")
            .and_then(Value::as_array)
            .ok_or_else(|| "Pine recovery checkpoint is missing submittedIntentKeys".to_owned())?
            .iter()
            .map(|value| {
                value
                    .as_str()
                    .map(ToOwned::to_owned)
                    .ok_or_else(|| "invalid Pine recovery intent key".to_owned())
            })
            .collect::<Result<BTreeSet<_>, _>>()?;
        sessions.insert(
            symbol.to_owned(),
            SymbolSessionState {
                bar_count: 0,
                revision: payload
                    .get("sessionRevision")
                    .and_then(Value::as_u64)
                    .unwrap_or_default(),
                submitted_intents,
            },
        );
    }
    Ok((last_closed, sessions))
}

fn persist_pine_runtime_checkpoint(
    store: &StrategyRuntimeStore,
    instance_id: &str,
    symbol: &str,
    session_revision: u64,
    last_closed_open_time: i64,
    submitted_intents: &BTreeSet<String>,
) -> Result<(), String> {
    let detail = serde_json::to_string(&json!({
        "scope": pine_checkpoint_scope(store, instance_id)?,
        "symbol": symbol,
        "sessionRevision": session_revision,
        "lastClosedOpenTime": last_closed_open_time,
        "submittedIntentKeys": submitted_intents,
    }))
    .map_err(|error| error.to_string())?;
    store
        .append_audit_event(
            instance_id,
            "PINE_SESSION_CHECKPOINT",
            &detail,
            now_millis(),
        )
        .map_err(|error| error.to_string())
}

fn pine_checkpoint_scope(
    store: &StrategyRuntimeStore,
    instance_id: &str,
) -> Result<String, String> {
    use sha2::{Digest, Sha256};
    let instance = store
        .get_instance(instance_id)
        .map_err(|error| error.to_string())?
        .ok_or_else(|| "Pine checkpoint instance no longer exists".to_owned())?;
    let source =
        json!({"binding": instance.binding, "definitionRevision": instance.definition_revision});
    Ok(Sha256::digest(source.to_string().as_bytes())
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect())
}

#[derive(Default)]
struct StrategyAccountInputs {
    available_cash: Option<f64>,
    current_position: Option<f64>,
    sellable_quantity: Option<f64>,
}

#[cfg(test)]
fn trading_environment_is_real(binding: &Value) -> bool {
    binding_string_opt(binding, &["tradingEnvironment", "environment", "env"])
        .or_else(|| {
            binding.get("brokerAccount").and_then(|account| {
                binding_string_opt(account, &["tradingEnvironment", "environment", "env"])
            })
        })
        .is_some_and(|value| value.eq_ignore_ascii_case("REAL"))
}

fn read_strategy_account_inputs(
    runtime: Option<&crate::product::product_production_ports::SharedTradeReadRuntime>,
    binding: &Value,
    market: &str,
    symbol: &str,
) -> Result<StrategyAccountInputs, String> {
    let runtime = runtime.ok_or_else(|| {
        "trade account runtime is unavailable for REAL strategy execution".to_owned()
    })?;
    let snapshot = runtime.snapshot();
    let client = snapshot
        .client
        .filter(|_| snapshot.trade_logged_in == Some(true))
        .ok_or_else(|| {
            "trade account session is not ready for REAL strategy execution".to_owned()
        })?;
    strategy_account_snapshot(client.as_ref(), binding, market, symbol)
}

fn strategy_account_snapshot(
    client: &dyn jftrade_integration_futu::TradeReadPort,
    binding: &Value,
    market: &str,
    symbol: &str,
) -> Result<StrategyAccountInputs, String> {
    let account_id = strategy_binding_scalar_string(binding, &["accountId", "account"])
        .or_else(|| {
            strategy_nested_binding_scalar_string(
                binding,
                "brokerAccount",
                &["accountId", "account"],
            )
        })
        .ok_or_else(|| "strategy execution accountId is not configured".to_owned())?
        .parse::<u64>()
        .map_err(|_| {
            "strategy execution accountId must be numeric for trade snapshot".to_owned()
        })?;
    let environment =
        strategy_binding_scalar_string(binding, &["tradingEnvironment", "environment", "env"])
            .or_else(|| {
                strategy_nested_binding_scalar_string(
                    binding,
                    "brokerAccount",
                    &["tradingEnvironment", "environment", "env"],
                )
            })
            .unwrap_or_else(|| "REAL".to_owned());
    let trd_env = if environment.eq_ignore_ascii_case("SIMULATE") {
        0
    } else {
        1
    };
    let trd_market = strategy_trade_market_code(market)?;
    let header = TradeHeader {
        trd_env,
        acc_id: account_id,
        trd_market,
        jp_acc_type: None,
    };
    let funds = client
        .read_funds(header.clone(), Some(true), None, None)
        .map_err(|error| format!("read strategy account funds: {error}"))?;
    let available_cash = [
        funds.funds.available_funds,
        funds.funds.net_cash_power,
        Some(funds.funds.power),
        Some(funds.funds.cash),
    ]
    .into_iter()
    .flatten()
    .find(|value| value.is_finite() && *value >= 0.0);
    let positions = client
        .read_positions(
            header,
            Some(TradeFilter {
                code_list: vec![symbol.to_owned()],
                ..TradeFilter::default()
            }),
            None,
            None,
            Some(true),
            None,
            None,
            None,
        )
        .map_err(|error| format!("read strategy account positions: {error}"))?;
    let position = positions
        .into_iter()
        .find(|position| position.code.eq_ignore_ascii_case(symbol));
    let (current_position, sellable_quantity) =
        position.map_or((Some(0.0), Some(0.0)), |position| {
            let signed_qty = if position.position_side == 2 {
                -position.qty.abs()
            } else {
                position.qty.abs()
            };
            (Some(signed_qty), Some(position.can_sell_qty.max(0.0)))
        });
    Ok(StrategyAccountInputs {
        available_cash,
        current_position,
        sellable_quantity,
    })
}

fn strategy_trade_market_code(market: &str) -> Result<i32, String> {
    match market.trim().to_ascii_uppercase().as_str() {
        "HK" => Ok(1),
        "US" => Ok(11),
        "SH" | "CN" => Ok(21),
        "SZ" => Ok(22),
        "SG" => Ok(31),
        "JP" => Ok(41),
        "AU" => Ok(51),
        "MY" => Ok(61),
        "CA" => Ok(71),
        value => Err(format!(
            "unsupported trade market for strategy account snapshot: {value}"
        )),
    }
}

fn strategy_binding_scalar_string(binding: &Value, keys: &[&str]) -> Option<String> {
    keys.iter().find_map(|key| {
        binding.get(*key).and_then(|value| match value {
            Value::String(value) if !value.trim().is_empty() => Some(value.trim().to_owned()),
            Value::Number(value) => Some(value.to_string()),
            _ => None,
        })
    })
}

fn strategy_nested_binding_scalar_string(
    binding: &Value,
    object_key: &str,
    keys: &[&str],
) -> Option<String> {
    binding
        .get(object_key)
        .and_then(Value::as_object)
        .and_then(|object| {
            keys.iter().find_map(|key| {
                object.get(*key).and_then(|value| match value {
                    Value::String(value) if !value.trim().is_empty() => {
                        Some(value.trim().to_owned())
                    }
                    Value::Number(value) => Some(value.to_string()),
                    _ => None,
                })
            })
        })
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
    worker: &dyn jftrade_integration_pine::PineExecutionPort,
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
            let close_result = runtime.block_on(async {
                tokio::time::timeout(std::time::Duration::from_millis(500), worker.run(request))
                    .await
            });
            match close_result {
                Ok(Err(err)) => {
                    let _ = store.append_audit_event(
                        instance_id,
                        "SESSION_CLOSE_FAILED",
                        &format!("close session for {symbol} failed: {err}"),
                        now_millis(),
                    );
                }
                Err(_) => {
                    let _ = store.append_audit_event(
                        instance_id,
                        "SESSION_CLOSE_TIMEOUT",
                        &format!("close session for {symbol} timed out after 500ms"),
                        now_millis(),
                    );
                }
                Ok(Ok(_)) => {}
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

#[path = "strategy_runtime_activity.rs"]
mod strategy_runtime_activity;
#[path = "strategy_runtime_candles.rs"]
mod strategy_runtime_candles;
#[path = "strategy_runtime_execution.rs"]
mod strategy_runtime_execution;
#[path = "strategy_runtime_mutation.rs"]
mod strategy_runtime_mutation;
#[path = "strategy_runtime_port.rs"]
mod strategy_runtime_port;
#[path = "strategy_runtime_session_recovery.rs"]
mod strategy_runtime_session_recovery;
#[path = "strategy_runtime_simulate.rs"]
mod strategy_runtime_simulate;
use strategy_runtime_activity::*;
use strategy_runtime_candles::*;
use strategy_runtime_execution::*;
use strategy_runtime_session_recovery::run_session_request;
use strategy_runtime_simulate::*;

pub(crate) use strategy_runtime_activity::normalize_strategy_binding;
#[allow(unused_imports)]
pub(crate) use strategy_runtime_port::ProductionStrategyRuntimePort;

#[cfg(test)]
#[path = "strategy_runtime_account_tests.rs"]
mod account_tests;
#[cfg(test)]
#[path = "strategy_runtime_owner_tests.rs"]
mod owner_tests;
#[path = "strategy_runtime_task.rs"]
mod strategy_runtime_task;
