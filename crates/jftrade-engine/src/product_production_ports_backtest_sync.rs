//! Historical candle sync request and task lifecycle.

use std::sync::{Arc, atomic::{AtomicU64, Ordering}};

use jftrade_integration_marketdata_helper::{HelperCandlesResponse, HelperClient};
use jftrade_settings::MarketDataProvider;
use jftrade_store_sqlite::{
    BacktestMarketDataStore, BacktestSyncTaskStore, CancelBacktestSyncResult,
    StoredBacktestCandle, StoredBacktestSyncTask,
};
use serde_json::{json, Value};
use tokio::sync::oneshot;

use crate::product::product_backtests_write_port::{
    BacktestsWritePortError, BacktestsWritePortResult,
};
use super::product_backtest_sync_request::{
    format_timestamp, parse_sync_request, parse_timestamp, SyncRequest,
};
use super::ProductionBacktestPort;

static SYNC_TASK_SEQUENCE: AtomicU64 = AtomicU64::new(1);

impl ProductionBacktestPort {
    pub(super) fn start_sync_task(
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

    pub(super) fn cancel_sync_task(
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
