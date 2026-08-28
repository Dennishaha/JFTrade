//! Backtests, Execution Orders, Brokers, and ADK production ports.

use std::sync::{Arc, atomic::{AtomicU64, Ordering}};
use jftrade_integration_marketdata_helper::{HelperCandlesResponse, HelperClient};
use jftrade_settings::MarketDataProvider;
use jftrade_store_sqlite::{
    BacktestMarketDataStore, BacktestRunStore, BacktestSyncTaskStore, StoredBacktestCandle,
    CancelBacktestSyncResult, ExecutionOrderStore, StoredBacktestSyncTask,
};
use serde_json::{Value, json};
use tokio::sync::oneshot;
use crate::product::product_active_provider_state::ActiveProviderState;
use super::BacktestSyncWorkerRegistry;
use crate::product::product_backtests_write_port::{
    BacktestsWriteDeleteResult, BacktestsWriteInput, BacktestsWritePort, BacktestsWritePortError,
    BacktestsWritePortResult,
};
use crate::product::product_brokers_write_port::{
    BrokersWriteInput, BrokersWritePort, BrokersWritePortError,
};
use crate::product::product_execution_write_port::{
    ExecutionWriteInput, ExecutionWritePort, ExecutionWritePortError,
};
use crate::product::{
    BacktestReadSnapshotError,
    BacktestReadSnapshotPort, BacktestSyncReadSnapshotError, BacktestSyncReadSnapshotPort,
    ExecutionReadSnapshotError, ExecutionReadSnapshotPort,
};

#[path = "product_backtest_sync_request.rs"]
mod product_backtest_sync_request;
use product_backtest_sync_request::{SyncRequest, format_timestamp, parse_sync_request, parse_timestamp};

pub(crate) struct ProductionBacktestPort {
    pub(crate) store: Arc<BacktestRunStore>,
    pub(crate) sync_tasks: Arc<BacktestSyncTaskStore>,
    pub(crate) _market_data_store: Arc<BacktestMarketDataStore>,
    pub(crate) helper: Option<HelperClient>,
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) sync_workers: Arc<BacktestSyncWorkerRegistry>,
}

impl std::fmt::Debug for ProductionBacktestPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionBacktestPort")
            .field("has_helper", &self.helper.is_some())
            .finish_non_exhaustive()
    }
}

static SYNC_TASK_SEQUENCE: AtomicU64 = AtomicU64::new(1);

impl BacktestReadSnapshotPort for ProductionBacktestPort {
    fn list(&self) -> Result<Value, BacktestReadSnapshotError> {
        let runs = self
            .store
            .list_runs()
            .map_err(|e| BacktestReadSnapshotError::Unavailable(e.to_string()))?;
        let items = runs
            .into_iter()
            .map(|r| {
                let request = decode_json_field(&r.request_json, "backtest request")?;
                // Go's ListLightweight validates the persisted result while
                // omitting the potentially large result payload from the
                // response.  Empty result_json is the normal representation
                // for queued/running runs and must not turn the whole list
                // into a store failure.
                let result = decode_optional_json_field(&r.result_json, "backtest result")?;
                let market_data_provider = result
                    .as_ref()
                    .and_then(|value| value.get("marketDataProvider"))
                    .and_then(Value::as_str)
                    .unwrap_or_default();
                Ok(json!({
                    "id": r.id,
                    "status": r.status,
                    "request": request,
                    "createdAt": r.created_at,
                    "updatedAt": r.updated_at,
                    "marketDataProvider": market_data_provider,
                }))
            })
            .collect::<Result<Vec<_>, BacktestReadSnapshotError>>()?;
        Ok(json!({ "runs": items }))
    }

    fn status(&self, run_id: &str) -> Result<Option<Value>, BacktestReadSnapshotError> {
        let run = self
            .store
            .get_run(run_id)
            .map_err(|e| BacktestReadSnapshotError::Unavailable(e.to_string()))?;
        Ok(run.map(|r| json!({
            "id": r.id,
            "status": r.status,
            "createdAt": r.created_at,
            "updatedAt": r.updated_at,
        })))
    }

    fn result(&self, run_id: &str) -> Result<Option<Value>, BacktestReadSnapshotError> {
        let run = self
            .store
            .get_run(run_id)
            .map_err(|e| BacktestReadSnapshotError::Unavailable(e.to_string()))?;
        run.map(|r| {
            let request = decode_json_field(&r.request_json, "backtest request")?;
            let result = decode_optional_json_field(&r.result_json, "backtest result")?;
            let market_data_provider = result
                .as_ref()
                .and_then(|value| value.get("marketDataProvider"))
                .and_then(Value::as_str)
                .unwrap_or_default();
            let mut response = json!({
                "id": r.id,
                "status": r.status,
                "request": request,
                "createdAt": r.created_at,
                "updatedAt": r.updated_at,
                "marketDataProvider": market_data_provider,
            });
            if let Some(result) = result {
                response["result"] = result;
            }
            Ok(response)
        })
        .transpose()
    }
}

fn decode_json_field(raw: &str, field: &str) -> Result<Value, BacktestReadSnapshotError> {
    serde_json::from_str(raw).map_err(|error| {
        BacktestReadSnapshotError::Unavailable(format!("stored {field} is invalid JSON: {error}"))
    })
}

fn decode_optional_json_field(
    raw: &str,
    field: &str,
) -> Result<Option<Value>, BacktestReadSnapshotError> {
    let trimmed = raw.trim();
    if trimmed.is_empty() || trimmed == "null" {
        return Ok(None);
    }
    decode_json_field(trimmed, field).map(Some)
}

impl BacktestSyncReadSnapshotPort for ProductionBacktestPort {
    fn progress(&self, task_id: &str) -> Result<Option<Value>, BacktestSyncReadSnapshotError> {
        self.sync_workers.reap_finished();
        self.sync_tasks
            .get(task_id)
            .map_err(|error| BacktestSyncReadSnapshotError::Unavailable(error.to_string()))?
            .map(|task| sync_task_projection(&task))
            .transpose()
    }
}

impl BacktestsWritePort for ProductionBacktestPort {
    fn mutate(&self, input: &BacktestsWriteInput) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        match input {
            BacktestsWriteInput::Start { .. } => {
                Err(BacktestsWritePortError::Unavailable(
                    "backtest worker runtime is not configured".to_owned(),
                ))
            }
            BacktestsWriteInput::Sync { payload } => {
                self.start_sync_task(payload)
            }
            BacktestsWriteInput::CancelSync { task_id } => {
                self.cancel_sync_task(task_id)
            }
            BacktestsWriteInput::Delete { run_id } => {
                match self.store.delete_run(run_id) {
                    Ok(true) => {
                        Ok(BacktestsWritePortResult::RunDeleted(BacktestsWriteDeleteResult::Deleted))
                    }
                    Ok(false) => {
                        Ok(BacktestsWritePortResult::RunDeleted(BacktestsWriteDeleteResult::Missing))
                    }
                    Err(jftrade_store_sqlite::BacktestRunStoreError::NotTerminal(_)) => {
                        Ok(BacktestsWritePortResult::RunDeleted(BacktestsWriteDeleteResult::NotTerminal))
                    }
                    Err(e) => Err(BacktestsWritePortError::Failed(e.to_string())),
                }
            }
        }
    }
}

impl ProductionBacktestPort {
    fn start_sync_task(
        &self,
        payload: &Value,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        let provider = self.active_provider_state.get().ok_or_else(|| {
            BacktestsWritePortError::Unavailable("active market-data provider is not configured".to_owned())
        })?;
        let provider_id = match provider {
            MarketDataProvider::Yfinance => "yfinance",
            MarketDataProvider::Akshare => "akshare",
            MarketDataProvider::Futu => {
                return Err(BacktestsWritePortError::Unavailable(
                    "Futu historical candle sync is not configured".to_owned(),
                ));
            }
        };
        let helper = self.helper.clone().ok_or_else(|| {
            BacktestsWritePortError::Unavailable("market-data helper is not configured".to_owned())
        })?;
        let request = parse_sync_request(payload)?;
        let now = time::OffsetDateTime::now_utc();
        let timestamp = now
            .format(&time::format_description::well_known::Rfc3339)
            .map_err(|error| BacktestsWritePortError::Failed(error.to_string()))?;
        let task_id = format!(
            "sync-{}-{}",
            now.unix_timestamp_nanos(),
            SYNC_TASK_SEQUENCE.fetch_add(1, Ordering::Relaxed)
        );
        let task = StoredBacktestSyncTask {
            task_id: task_id.clone(),
            status: "queued".to_owned(),
            symbol: request.symbol.clone(),
            market_data_provider: provider_id.to_owned(),
            total_intervals: request.intervals.len() as i64,
            completed_intervals: 0,
            // The helper pagination depth is not knowable before the first
            // response. Go leaves this field at zero and only increments the
            // completed count for each fetched page; keeping the same
            // semantics avoids ever reporting completed > total.
            total_batches: 0,
            completed_batches: 0,
            current_interval: String::new(),
            retries: 0,
            error: None,
            started_at: timestamp.clone(),
            updated_at: timestamp.clone(),
            revision: 0,
        };
        // Resolve the runtime before creating any durable task. An API call
        // made outside Tokio cannot ever service the helper worker, so it
        // must fail without leaving an orphaned queued record.
        let runtime = tokio::runtime::Handle::try_current().map_err(|_| {
            BacktestsWritePortError::Unavailable(
                "backtest sync runtime is not available".to_owned(),
            )
        })?;
        self.sync_tasks
            .create(task.clone())
            .map_err(|error| BacktestsWritePortError::Failed(error.to_string()))?;
        let response_intervals = request.intervals.clone();
        let response_since = request.since.clone();
        let response_until = request.until.clone();
        let response_session_scope = request.session_scope.clone();
        let response_task_id = task.task_id.clone();
        let tasks = Arc::clone(&self.sync_tasks);
        let market_store = Arc::clone(&self._market_data_store);
        let registry = Arc::clone(&self.sync_workers);
        let worker_task_id = task_id.clone();
        let registry_task_id = worker_task_id.clone();
        let registry_tasks = Arc::clone(&tasks);
        let (cancel_tx, cancel_rx) = oneshot::channel();
        let handle = runtime.spawn(async move {
            tokio::select! {
                _ = run_sync_task(Arc::clone(&tasks), market_store, helper, provider_id, request, task_id) => {}
                _ = cancel_rx => mark_task_cancelled(&tasks, &worker_task_id),
            }
        });
        registry.register(registry_task_id, registry_tasks, handle, cancel_tx);
        Ok(BacktestsWritePortResult::Data(json!({
            "taskId": response_task_id,
            "symbol": task.symbol,
            "intervals": response_intervals,
            "since": response_since,
            "until": response_until,
            "sessionScope": response_session_scope,
            "message": "sync started",
            "marketDataProvider": provider_id,
        })))
    }

    fn cancel_sync_task(
        &self,
        task_id: &str,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        let timestamp = time::OffsetDateTime::now_utc()
            .format(&time::format_description::well_known::Rfc3339)
            .map_err(|error| BacktestsWritePortError::Failed(error.to_string()))?;
        match self
            .sync_tasks
            .cancel(task_id, &timestamp)
            .map_err(|error| match error {
                jftrade_store_sqlite::BacktestRunStoreError::Conflict(message) => {
                    BacktestsWritePortError::Conflict(message)
                }
                other => BacktestsWritePortError::Failed(other.to_string()),
            })?
        {
            CancelBacktestSyncResult::Cancelled => {
                Ok(BacktestsWritePortResult::SyncCancelled(true))
            }
            CancelBacktestSyncResult::Missing => {
                Ok(BacktestsWritePortResult::SyncCancelled(false))
            }
            // Go's CancelSync intentionally collapses a terminal task and an
            // unknown task into the same 404 response.
            CancelBacktestSyncResult::AlreadyTerminal => {
                Ok(BacktestsWritePortResult::SyncCancelled(false))
            }
        }
    }

}


async fn run_sync_task(
    tasks: Arc<BacktestSyncTaskStore>,
    market_store: Arc<BacktestMarketDataStore>,
    helper: HelperClient,
    provider: &str,
    request: SyncRequest,
    task_id: String,
) {
    let task_snapshot = match tasks.get(&task_id) {
        Ok(task) => task,
        Err(error) => {
            eprintln!("backtest sync {task_id} failed to load task: {error}");
            return;
        }
    };
    let Some(mut task) = task_snapshot else { return };
    if matches!(task.status.as_str(), "cancelled" | "completed" | "failed") {
        return;
    }
    if let Err(error) = persist_task(&tasks, &mut task, "running", None) {
        eprintln!("backtest sync {task_id} failed to mark running: {error}");
        return;
    }
    let result = sync_request_pages(&tasks, &market_store, &helper, provider, &request, &task_id, &mut task).await;
    let cancelled = match is_cancelled(&tasks, &task_id) {
        Ok(cancelled) => cancelled,
        Err(error) => {
            eprintln!("backtest sync {task_id} failed to read cancellation state: {error}");
            return;
        }
    };
    match (result, cancelled) {
        (Ok(()), true) => {}
        (Ok(()), false) => {
            if let Err(error) = persist_task(&tasks, &mut task, "completed", None) {
                eprintln!("backtest sync {task_id} failed to mark completed: {error}");
            }
        }
        (Err(_error), true) => {}
        (Err(error), false) => {
            if let Err(persist_error) = persist_task(&tasks, &mut task, "failed", Some(error)) {
                eprintln!("backtest sync {task_id} failed to persist failure: {persist_error}");
            }
        }
    }
}

async fn sync_request_pages(
    tasks: &Arc<BacktestSyncTaskStore>,
    market_store: &Arc<BacktestMarketDataStore>,
    helper: &HelperClient,
    provider: &str,
    request: &SyncRequest,
    task_id: &str,
    task: &mut StoredBacktestSyncTask,
) -> Result<(), String> {
    let since = parse_timestamp(&request.since)?;
    let until = parse_timestamp(&request.until)?;
    for (index, interval) in request.intervals.iter().enumerate() {
        if is_cancelled(tasks, task_id)? { return Ok(()); }
        task.current_interval = interval.clone();
        persist_task(tasks, task, "running", None)?;
        // Go's historical source asks for `until + 1ns` so a candle exactly
        // on the upper boundary is not lost by an exclusive helper query.
        let mut before = until + time::Duration::nanoseconds(1);
        let mut seen = std::collections::BTreeSet::new();
        let mut interval_inserted = false;
        loop {
            if is_cancelled(tasks, task_id)? { return Ok(()); }
            let before_text = format_timestamp(before);
            let sessions = if request.session_scope == "extended" { "regular,extended" } else { "regular" };
            let query = [("period", interval.as_str()), ("limit", "1000"), ("before", before_text.as_str()), ("sessions", sessions)];
            let helper_market = if request.market == "CN" {
                symbol_market(&request.symbol)
            } else {
                request.market.as_str()
            };
            let response: HelperCandlesResponse = helper
                .get_provider_json_with_query(provider, &["candles", helper_market, symbol_code(&request.symbol)], &query)
                .await
                .map_err(|error| error.to_string())?;
            validate_helper_page(&response, helper_market, &request.symbol, interval)?;
            let mut rows = Vec::with_capacity(response.candles.len());
            for candle in response.candles {
                let at = parse_timestamp(&candle.at)?;
                if at < since || at >= until { continue; }
                let end = at + interval_duration(interval) - time::Duration::milliseconds(1);
                rows.push(StoredBacktestCandle { start_time: at.unix_timestamp_nanos() as i64 / 1_000_000, end_time: end.unix_timestamp_nanos() as i64 / 1_000_000, open: candle.open.0, high: candle.high.0, low: candle.low.0, close: candle.close.0, volume: candle.volume.map_or_else(|| "0".to_owned(), |value| value.0) });
            }
            interval_inserted |= !rows.is_empty();
            if !rows.is_empty() {
                market_store.insert_candles(provider, &request.symbol, interval, &request.rehab_type, &request.session_scope, &rows).map_err(|error| error.to_string())?;
            }
            task.completed_batches += 1;
            persist_task(tasks, task, "running", None)?;
            if !response.has_more { break; }
            let next = response.next_before.as_deref().ok_or_else(|| "helper returned hasMore without nextBefore".to_owned()).and_then(parse_timestamp)?;
            if next >= before || !seen.insert(next.unix_timestamp_nanos()) { return Err("helper pagination cursor did not move backward".to_owned()); }
            if next <= since {
                if !interval_inserted {
                    return Err("helper returned no candles in the requested range".to_owned());
                }
                break;
            }
            before = next;
        }
        if !interval_inserted {
            return Err("helper returned no candles in the requested range".to_owned());
        }
        task.completed_intervals = (index + 1) as i64;
        persist_task(tasks, task, "running", None)?;
    }
    Ok(())
}

fn symbol_code(symbol: &str) -> &str { symbol.split_once('.').map_or(symbol, |(_, code)| code) }

fn symbol_market(symbol: &str) -> &str { symbol.split_once('.').map_or("", |(market, _)| market) }

fn interval_duration(interval: &str) -> time::Duration {
    match interval { "1m" => time::Duration::minutes(1), "5m" => time::Duration::minutes(5), "15m" => time::Duration::minutes(15), "30m" => time::Duration::minutes(30), "1h" => time::Duration::hours(1), "1w" => time::Duration::days(7), "1mo" => time::Duration::days(30), _ => time::Duration::days(1) }
}

fn validate_helper_page(response: &HelperCandlesResponse, market: &str, symbol: &str, interval: &str) -> Result<(), String> {
    let expected_instrument = format!("{market}.{}", symbol_code(symbol));
    if !response.market.eq_ignore_ascii_case(market) || !response.symbol.eq_ignore_ascii_case(symbol_code(symbol)) || !response.instrument_id.eq_ignore_ascii_case(&expected_instrument) || response.period != interval || response.total_returned != response.candles.len() { return Err("helper candle response identity is invalid".to_owned()); }
    if response.has_more && response.candles.is_empty() {
        return Err("helper returned hasMore with an empty candle page".to_owned());
    }
    let now = time::OffsetDateTime::now_utc();
    let mut previous = None;
    for candle in &response.candles {
        let at = parse_timestamp(&candle.at)?;
        if at.unix_timestamp() < 0 || at > now + time::Duration::days(1) {
            return Err("helper candle timestamp is outside the supported range".to_owned());
        }
        if previous.is_some_and(|previous| at <= previous) {
            return Err("helper candle timestamps are not strictly increasing".to_owned());
        }
        previous = Some(at);
    }
    Ok(())
}

fn is_cancelled(tasks: &BacktestSyncTaskStore, task_id: &str) -> Result<bool, String> {
    tasks
        .get(task_id)
        .map(|task| task.is_some_and(|task| task.status == "cancelled"))
        .map_err(|error| error.to_string())
}

fn mark_task_cancelled(tasks: &BacktestSyncTaskStore, task_id: &str) {
    let timestamp = format_timestamp(time::OffsetDateTime::now_utc());
    let _ = tasks.cancel(task_id, &timestamp);
}

fn persist_task(tasks: &BacktestSyncTaskStore, task: &mut StoredBacktestSyncTask, status: &str, error: Option<String>) -> Result<(), String> {
    task.status = status.to_owned();
    task.error = error;
    task.updated_at = format_timestamp(time::OffsetDateTime::now_utc());
    let expected = task.revision;
    match tasks.update(task.clone(), expected) {
        Ok(true) => {
            task.revision += 1;
            Ok(())
        }
        Ok(false) => Err("sync task revision conflict".to_owned()),
        Err(error) => Err(error.to_string()),
    }
}

fn sync_task_projection(task: &StoredBacktestSyncTask) -> Result<Value, BacktestSyncReadSnapshotError> {
    if task.task_id.trim().is_empty() || task.status.trim().is_empty() {
        return Err(BacktestSyncReadSnapshotError::Unavailable(
            "stored sync task has invalid identity".to_owned(),
        ));
    }
    let mut value = json!({
        "completedBatches": task.completed_batches,
        "completedIntervals": task.completed_intervals,
        "currentInterval": task.current_interval,
        "marketDataProvider": task.market_data_provider,
        "retries": task.retries,
        "startedAt": task.started_at,
        "status": task.status,
        "symbol": task.symbol,
        "taskId": task.task_id,
        "totalBatches": task.total_batches,
        "totalIntervals": task.total_intervals,
        "updatedAt": task.updated_at,
    });
    if let Some(error) = &task.error {
        value["error"] = Value::String(error.clone());
    }
    Ok(value)
}

#[cfg(test)]
#[path = "product_backtest_sync_start_tests.rs"]
mod sync_start_tests;

#[derive(Debug)]
pub(crate) struct ProductionExecutionPort {
    pub(crate) store: Arc<ExecutionOrderStore>,
}

impl ExecutionReadSnapshotPort for ProductionExecutionPort {
    fn read(&self, path: &str, _query: &str) -> Result<Value, ExecutionReadSnapshotError> {
        if path == "/api/v1/execution/orders" {
            let orders = self
                .store
                .list_orders()
                .map_err(|e| ExecutionReadSnapshotError::Unavailable(e.to_string()))?;
            let items: Vec<Value> = orders
                .into_iter()
                .map(|o| json!({
                    "internalOrderId": o.internal_order_id,
                    "brokerId": o.broker_id,
                    "brokerOrderId": o.broker_order_id,
                    "status": o.status,
                    "symbol": o.symbol,
                    "side": o.side,
                    "orderType": o.order_type,
                    "requestedQuantity": o.requested_quantity,
                    "requestedPrice": o.requested_price,
                    "filledQuantity": o.filled_quantity,
                    "filledAveragePrice": o.filled_average_price,
                    "createdAt": o.created_at,
                    "updatedAt": o.updated_at,
                }))
                .collect();
            return Ok(json!({ "orders": items }));
        }

        if let Some(id) = path
            .strip_prefix("/api/v1/execution/orders/")
            .and_then(|suffix| suffix.strip_suffix("/events"))
        {
            if id.is_empty() || id.contains('/') {
                return Err(ExecutionReadSnapshotError::NotFound);
            }
            let events = self
                .store
                .list_order_events(id)
                .map_err(|e| ExecutionReadSnapshotError::Unavailable(e.to_string()))?
                .into_iter()
                .map(|event| {
                    json!({
                        "id": event.id,
                        "internalOrderId": event.internal_order_id,
                        "eventType": event.event_type,
                        "previousStatus": event.previous_status,
                        "nextStatus": event.next_status,
                        "payloadJson": event.payload_json,
                        "createdAt": event.created_at,
                    })
                })
                .collect::<Vec<_>>();
            return Ok(json!({"internalOrderId": id, "events": events}));
        }

        if let Some(id) = path.strip_prefix("/api/v1/execution/orders/") {
            if id.is_empty() || id.contains('/') {
                return Err(ExecutionReadSnapshotError::NotFound);
            }
            let order = self
                .store
                .get_order(id)
                .map_err(|e| ExecutionReadSnapshotError::Unavailable(e.to_string()))?;
            if let Some(o) = order {
                return Ok(json!({
                    "internalOrderId": o.internal_order_id,
                    "brokerId": o.broker_id,
                    "brokerOrderId": o.broker_order_id,
                    "status": o.status,
                    "symbol": o.symbol,
                    "side": o.side,
                    "orderType": o.order_type,
                    "requestedQuantity": o.requested_quantity,
                    "requestedPrice": o.requested_price,
                    "filledQuantity": o.filled_quantity,
                    "filledAveragePrice": o.filled_average_price,
                    "createdAt": o.created_at,
                    "updatedAt": o.updated_at,
                }));
            }
            return Err(ExecutionReadSnapshotError::NotFound);
        }

        Err(ExecutionReadSnapshotError::NotFound)
    }
}

impl ExecutionWritePort for ProductionExecutionPort {
    fn mutate(&self, input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        let _ = input;
        Err(ExecutionWritePortError::Unavailable(
            "execution broker/OpenD runtime is not configured".to_owned(),
        ))
    }
}

impl BrokersWritePort for ProductionExecutionPort {
    fn mutate(&self, input: &BrokersWriteInput) -> Result<Value, BrokersWritePortError> {
        let _ = input;
        Err(BrokersWritePortError::Unavailable(
            "broker/OpenD runtime is not configured".to_owned(),
        ))
    }
}
