//! Production strategy runtime adapter.

use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_execution_write_port::{
    ExecutionWriteInput, ExecutionWriteOperation, ExecutionWritePort, ExecutionWritePortError,
};
use crate::product::product_strategy_runtime_write_port::{
    StrategyRuntimeWriteInput, StrategyRuntimeWriteOperation, StrategyRuntimeWritePort,
    StrategyRuntimeWritePortError,
};
use crate::product::{
    MarketDataQuoteReadSnapshotError, MarketDataQuoteReadSnapshotPort, StrategyReadSnapshotError,
    StrategyReadSnapshotPort, StrategyRuntimeStatusPort, StrategyRuntimeSummary,
};
use jftrade_integration_pine::{
    GrpcPineExecutionPort, PineCandle, PineExecutionError, PineOrderIntent, PineRunRequest,
};
use jftrade_marketdata::{InstrumentRef, ProviderRouter};
use jftrade_store_sqlite::{StrategyDefinitionStore, StrategyRuntimeStore};
use serde_json::{Value, json};
use std::collections::BTreeMap;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::thread::JoinHandle;
use std::time::Duration;

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
    quote: Option<Arc<dyn MarketDataQuoteReadSnapshotPort>>,
    execution: Option<Arc<dyn ExecutionWritePort>>,
    provider: Arc<ActiveProviderState>,
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
        if self.quote.is_none() {
            return Some(StrategyRuntimeWritePortError::Unavailable(
                "strategy market-data quote port is unavailable".to_owned(),
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
        let Some(quote) = self.quote.clone() else {
            return Err(StrategyRuntimeWritePortError::Unavailable(
                "strategy market-data quote port is unavailable".to_owned(),
            ));
        };
        let execution = self.execution.clone();
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
        let execute_orders = binding
            .get("executeOrders")
            .and_then(Value::as_bool)
            .unwrap_or(true);
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
                let mut first_cycle = true;
                while !cancel_for_thread.load(Ordering::Acquire) {
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
                            candles,
                            params: binding_params(&binding),
                            session_id: String::new(),
                            session_operation: String::new(),
                            expected_revision: 0,
                        };
                        let response = match runtime.block_on(worker.run_script(request)) {
                            Ok(response) => response,
                            Err(error) => {
                                cycle_error = Some(pine_error_message(error));
                                break;
                            }
                        };
                        let current_intents = current_bar_intents(
                            &response.order_intents,
                            latest_bar_index,
                            latest_open_time,
                        );
                        if !current_intents.is_empty() {
                            last_signal = Some(latest_open_time);
                            if execute_orders {
                                match execute_strategy_intents(
                                    execution.as_deref(),
                                    &provider,
                                    &id_for_thread,
                                    &market,
                                    &symbol,
                                    &binding,
                                    &current_intents,
                                ) {
                                    Ok(true) => last_order = Some(latest_open_time),
                                    Ok(false) => {}
                                    Err(error) => {
                                        cycle_error = Some(error);
                                        break;
                                    }
                                }
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
fn now_rfc3339() -> Result<String, String> {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .map_err(|error| format!("format strategy runtime timestamp: {error}"))
}
fn pine_error_message(error: PineExecutionError) -> String {
    error.to_string()
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

async fn read_strategy_candles(
    quote: &dyn MarketDataQuoteReadSnapshotPort,
    market: &str,
    symbol: &str,
    timeframe: &str,
    limit: usize,
    sessions: &[String],
) -> Result<Vec<PineCandle>, String> {
    let path = format!("/api/v1/market-data/candles/{market}/{symbol}");
    let query = format!(
        "period={timeframe}&limit={limit}&sessions={}",
        sessions.join(",")
    );
    let value = quote.read(&path, &query).await.map_err(quote_error_message)?;
    parse_strategy_candles(&value)
}

fn quote_error_message(error: MarketDataQuoteReadSnapshotError) -> String {
    match error {
        MarketDataQuoteReadSnapshotError::Unavailable(message) => {
            format!("market-data unavailable: {message}")
        }
        MarketDataQuoteReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => match retry_after_seconds {
            Some(retry) => format!("market-data failed ({status} {code}): {message}; retry after {retry}s"),
            None => format!("market-data failed ({status} {code}): {message}"),
        },
    }
}

fn parse_strategy_candles(value: &Value) -> Result<Vec<PineCandle>, String> {
    let entries = value
        .get("candles")
        .and_then(Value::as_array)
        .ok_or_else(|| "market-data candle response is missing candles".to_owned())?;
    let mut previous = None;
    let mut candles = Vec::with_capacity(entries.len());
    for (index, entry) in entries.iter().enumerate() {
        let at = entry
            .get("at")
            .and_then(Value::as_str)
            .ok_or_else(|| format!("candle[{index}] is missing at"))?;
        let timestamp = time::OffsetDateTime::parse(
            at,
            &time::format_description::well_known::Rfc3339,
        )
        .map_err(|error| format!("candle[{index}] has invalid at: {error}"))?;
        let open_time = timestamp.unix_timestamp_nanos() / 1_000_000;
        let open_time = i64::try_from(open_time)
            .map_err(|_| format!("candle[{index}] timestamp is out of range"))?;
        if previous.is_some_and(|previous| open_time <= previous) {
            return Err("market-data candles are not strictly chronological".to_owned());
        }
        previous = Some(open_time);
        let open = candle_number(entry, "open", index)?;
        let high = candle_number(entry, "high", index)?;
        let low = candle_number(entry, "low", index)?;
        let close = candle_number(entry, "close", index)?;
        if high < low || high < open || high < close || low > open || low > close {
            return Err(format!("candle[{index}] has invalid OHLC bounds"));
        }
        let volume = entry
            .get("volume")
            .filter(|value| !value.is_null())
            .map(|value| candle_number_value(value, "volume", index))
            .transpose()?
            .unwrap_or(0.0);
        if volume < 0.0 {
            return Err(format!("candle[{index}] has negative volume"));
        }
        candles.push(PineCandle {
            open_time,
            close_time: open_time,
            open,
            high,
            low,
            close,
            volume,
        });
    }
    Ok(candles)
}

fn candle_number(entry: &Value, field: &str, index: usize) -> Result<f64, String> {
    let value = entry
        .get(field)
        .ok_or_else(|| format!("candle[{index}] is missing {field}"))?;
    candle_number_value(value, field, index)
}

fn candle_number_value(value: &Value, field: &str, index: usize) -> Result<f64, String> {
    let parsed = match value {
        Value::Number(number) => number
            .as_f64()
            .ok_or_else(|| format!("candle[{index}] {field} is not finite"))?,
        Value::String(text) => text
            .trim()
            .parse::<f64>()
            .map_err(|_| format!("candle[{index}] {field} is not numeric"))?,
        _ => return Err(format!("candle[{index}] {field} is not numeric")),
    };
    if !parsed.is_finite() {
        return Err(format!("candle[{index}] {field} is not finite"));
    }
    Ok(parsed)
}

fn current_bar_intents(
    intents: &[PineOrderIntent],
    bar_index: i32,
    open_time: i64,
) -> Vec<PineOrderIntent> {
    intents
        .iter()
        .filter(|intent| {
            intent.bar_index == bar_index || (intent.time > 0 && intent.time == open_time)
        })
        .cloned()
        .collect()
}

fn execute_strategy_intents(
    execution: Option<&dyn ExecutionWritePort>,
    provider: &ActiveProviderState,
    instance_id: &str,
    market: &str,
    symbol: &str,
    binding: &Value,
    intents: &[PineOrderIntent],
) -> Result<bool, String> {
    let Some(execution) = execution else {
        return Err("strategy execution order port is unavailable".to_owned());
    };
    let (broker_id, account_id, trading_environment) =
        strategy_execution_binding(binding, provider)?;
    let mut placed = false;
    for (index, intent) in intents.iter().enumerate() {
        if intent.has_quantity_pct && !intent.has_quantity {
            return Err(format!(
                "strategy order intent {index} uses quantity percent without account sizing"
            ));
        }
        if !intent.has_quantity || !intent.quantity.is_finite() || intent.quantity <= 0.0 {
            return Err(format!(
                "strategy order intent {index} requires a positive finite quantity"
            ));
        }
        let side = match intent.direction.trim().to_ascii_lowercase().as_str() {
            "buy" | "long" | "bull" | "bullish" => "BUY",
            "sell" | "short" | "bear" | "bearish" => "SELL",
            _ => return Err(format!("strategy order intent {index} has invalid direction")),
        };
        let order_type = if intent.has_limit_price {
            "LIMIT"
        } else if intent.has_stop_price {
            "STOP"
        } else {
            "MARKET"
        };
        let mut payload = json!({
            "brokerId": broker_id,
            "accountId": account_id,
            "tradingEnvironment": trading_environment,
            "market": market,
            "symbol": symbol,
            "code": symbol,
            "side": side,
            "orderType": order_type,
            "quantity": intent.quantity,
            "orderKind": intent.kind,
            "remark": format!("strategy runtime {instance_id}"),
            "clientOrderId": strategy_client_order_id(instance_id, intent, index),
        });
        if intent.has_limit_price {
            payload["price"] = json!(intent.limit_price);
        }
        if intent.has_stop_price {
            payload["stopPrice"] = json!(intent.stop_price);
        }
        let input = ExecutionWriteInput {
            operation: ExecutionWriteOperation::OrderPlace,
            internal_order_id: None,
            payload,
            context: crate::product::product_execution_write_port::ExecutionWriteContext::Normal,
        };
        execution
            .mutate(&input)
            .map_err(execution_error_message)?;
        placed = true;
    }
    Ok(placed)
}

fn validate_strategy_execution_binding(
    binding: &Value,
    provider: &ActiveProviderState,
) -> Result<(), String> {
    strategy_execution_binding(binding, provider).map(|_| ())
}

/// Resolve the account context required by the execution parser. Strategy
/// bindings historically stored this under `brokerAccount`, while a few
/// persisted rows use flat keys; accept both forms but never invent an
/// account or trading environment. The execution adapter requires a numeric
/// account id for Futu, so validating that shape here prevents a running task
/// from failing only after Pine emits its first order intent.
fn strategy_execution_binding(
    binding: &Value,
    provider: &ActiveProviderState,
) -> Result<(String, String, String), String> {
    let broker_id = binding_scalar_string(binding, &["brokerId", "broker"])
        .or_else(|| {
            nested_binding_scalar_string(binding, "brokerAccount", &["brokerId", "broker"])
        })
        .or_else(|| {
            (provider.snapshot().provider == Some(jftrade_settings::MarketDataProvider::Futu))
                .then_some("futu".to_owned())
        })
        .ok_or_else(|| "strategy execution broker is not configured".to_owned())?;
    let account_id = binding_scalar_string(binding, &["accountId", "account"])
        .or_else(|| {
            nested_binding_scalar_string(binding, "brokerAccount", &["accountId", "account"])
        })
        .ok_or_else(|| "strategy execution accountId is not configured".to_owned())?;
    if account_id.parse::<u64>().is_err() {
        return Err("strategy execution accountId must be numeric for Futu".to_owned());
    }
    let trading_environment = binding_scalar_string(
        binding,
        &["tradingEnvironment", "environment", "env"],
    )
    .or_else(|| {
        nested_binding_scalar_string(
            binding,
            "brokerAccount",
            &["tradingEnvironment", "environment", "env"],
        )
    })
    .ok_or_else(|| "strategy execution tradingEnvironment is not configured".to_owned())?
    .to_ascii_uppercase();
    if !matches!(trading_environment.as_str(), "REAL" | "SIMULATE") {
        return Err("strategy execution tradingEnvironment must be REAL or SIMULATE".to_owned());
    }
    Ok((broker_id, account_id, trading_environment))
}

fn binding_scalar_string(binding: &Value, keys: &[&str]) -> Option<String> {
    keys.iter()
        .find_map(|key| binding.get(*key).and_then(value_scalar_string))
}

fn nested_binding_scalar_string(
    binding: &Value,
    object_key: &str,
    keys: &[&str],
) -> Option<String> {
    binding
        .get(object_key)
        .and_then(Value::as_object)
        .and_then(|object| {
            keys.iter()
                .find_map(|key| object.get(*key).and_then(value_scalar_string))
        })
}

fn value_scalar_string(value: &Value) -> Option<String> {
    match value {
        Value::String(value) => {
            let value = value.trim();
            (!value.is_empty()).then_some(value.to_owned())
        }
        Value::Number(value) => Some(value.to_string()),
        _ => None,
    }
}

fn strategy_client_order_id(instance_id: &str, intent: &PineOrderIntent, index: usize) -> String {
    let identity = if intent.id.trim().is_empty() {
        format!("bar-{}-{index}", intent.bar_index)
    } else {
        intent.id.trim().to_owned()
    };
    format!("strategy-{instance_id}-{identity}")
}

fn execution_error_message(error: ExecutionWritePortError) -> String {
    match error {
        ExecutionWritePortError::Unavailable(message) => {
            format!("strategy execution unavailable: {message}")
        }
        ExecutionWritePortError::Failed {
            status,
            code,
            message,
        } => format!("strategy execution failed ({status} {code}): {message}"),
    }
}

fn record_worker_output(
    store: &StrategyRuntimeStore,
    instance_id: &str,
    response: &jftrade_integration_pine::PineRunResult,
    at_ms: i64,
) -> Result<(), String> {
    for message in response.logs.iter().chain(response.warnings.iter()) {
        store
            .append_log_event(instance_id, message, "info", at_ms)
            .map_err(|error| error.to_string())?;
    }
    for diagnostic in &response.diagnostics {
        let detail = if diagnostic.code.trim().is_empty() {
            diagnostic.message.clone()
        } else {
            format!("{}: {}", diagnostic.code, diagnostic.message)
        };
        store
            .append_log_event(instance_id, &detail, &diagnostic.severity, at_ms)
            .map_err(|error| error.to_string())?;
    }
    if !response.order_intents.is_empty() {
        store
            .append_audit_event(
                instance_id,
                "SIGNAL",
                &format!("{} order intent(s)", response.order_intents.len()),
                at_ms,
            )
            .map_err(|error| error.to_string())?;
    }
    Ok(())
}

fn sleep_until_next_strategy_poll(cancel: &AtomicBool) {
    const POLL_INTERVAL: Duration = Duration::from_secs(1);
    let deadline = std::time::Instant::now() + POLL_INTERVAL;
    while !cancel.load(Ordering::Acquire) {
        let remaining = deadline.saturating_duration_since(std::time::Instant::now());
        if remaining.is_zero() {
            break;
        }
        std::thread::sleep(remaining.min(Duration::from_millis(100)));
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
use strategy_runtime_activity::*;

pub(crate) use strategy_runtime_port::ProductionStrategyRuntimePort;
