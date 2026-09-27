//! Historical candle sync request and task lifecycle.

use std::sync::{
    Arc,
    atomic::{AtomicU64, Ordering},
};

use jftrade_integration_marketdata_helper::{HelperCandlesResponse, HelperClient};
use jftrade_settings::MarketDataProvider;
use jftrade_store_sqlite::{
    BacktestMarketDataStore, BacktestSyncTaskStore, CancelBacktestSyncResult,
    StoredBacktestCandle, StoredBacktestSyncTask,
};
use serde_json::{Value, json};
use tokio::sync::oneshot;

use super::ProductionBacktestPort;
use crate::product::product_production_ports::SharedTradeReadRuntime;
use super::product_backtest_sync_request::{
    SyncRequest, format_timestamp, parse_sync_request, parse_timestamp,
    validate_sync_provider_capabilities,
};
use super::requested_provider;
use crate::product::product_backtests_write_port::{
    BacktestsWritePortError, BacktestsWritePortResult,
};

#[path = "product_production_ports_backtest_sync_helpers.rs"]
mod sync_helpers;
#[path = "product_production_ports_backtest_sync_source_helpers.rs"]
mod source_helpers;

use sync_helpers::{is_cancelled, mark_task_cancelled, persist_task};
use source_helpers::{
    fetch_futu_page_with_retry, fetch_helper_page_with_retry, futu_market_code, futu_rows,
    interval_duration, symbol_code, symbol_market, validate_futu_page, validate_helper_page,
};

static SYNC_TASK_SEQUENCE: AtomicU64 = AtomicU64::new(1);

impl ProductionBacktestPort {
    /// Mark durable tasks left by a crashed process as terminal.  The worker
    /// registry is process-local, therefore a queued/running row discovered at
    /// composition time cannot still have an owner and must not be reported as
    /// live work after restart.
    pub(crate) fn recover_orphaned_sync_tasks(&self) -> Result<(), String> {
        let tasks = self
            .sync_tasks
            .list_active()
            .map_err(|error| format!("failed to scan backtest sync tasks: {error}"))?;
        for task in tasks {
            let timestamp = format_timestamp(time::OffsetDateTime::now_utc());
            let recovered = StoredBacktestSyncTask {
                status: "failed".to_owned(),
                error: Some("sync task interrupted by process restart".to_owned()),
                updated_at: timestamp,
                ..task.clone()
            };
            let changed = self
                .sync_tasks
                .update(recovered, task.revision)
                .map_err(|error| {
                    format!(
                        "backtest sync {} restart recovery failed: {error}",
                        task.task_id
                    )
                })?;
            if !changed {
                return Err(format!(
                    "backtest sync {} restart recovery conflicted",
                    task.task_id
                ));
            }
        }
        Ok(())
    }

    pub(super) fn start_sync_task(
        &self,
        payload: &Value,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        let provider_id = if let Some(provider_id) = requested_provider(payload)? {
            provider_id
        } else {
            match self.backtest_market_data_provider_state.get() {
                MarketDataProvider::Yfinance => "yfinance",
                MarketDataProvider::Akshare => "akshare",
                MarketDataProvider::Futu => "futu",
            }
        };
        let request = parse_sync_request(payload)?;
        let helper = self.helper.clone();
        let historical_ready = self
            .trade_runtime
            .as_ref()
            .is_some_and(|runtime| runtime.historical_klines_available());
        match provider_id {
            "futu" if !historical_ready => {
                return Err(BacktestsWritePortError::Unavailable(
                    "Futu historical candle sync is unavailable".to_owned(),
                ));
            }
            "yfinance" | "akshare" if helper.is_none() => {
                return Err(BacktestsWritePortError::Unavailable(
                    "market-data helper is not configured".to_owned(),
                ));
            }
            _ => {}
        }
        // Capability checks follow runtime availability so unavailable providers keep 503.
        validate_sync_provider_capabilities(provider_id, &request)?;
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
        let trade_runtime = self.trade_runtime.clone();
        let registry = Arc::clone(&self.sync_workers);
        let worker_task_id = task_id.clone();
        let registry_task_id = worker_task_id.clone();
        let registry_tasks = Arc::clone(&tasks);
        let (cancel_tx, cancel_rx) = oneshot::channel();
        let handle = runtime.spawn(async move {
            tokio::select! {
                _ = run_sync_task(Arc::clone(&tasks), market_store, helper, trade_runtime, provider_id, request, task_id) => {}
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
            })? {
            CancelBacktestSyncResult::Cancelled => {
                self.sync_workers.request_cancel(task_id);
                Ok(BacktestsWritePortResult::SyncCancelled(true))
            }
            CancelBacktestSyncResult::Missing => Ok(BacktestsWritePortResult::SyncCancelled(false)),
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
    helper: Option<HelperClient>,
    trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
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
    let Some(mut task) = task_snapshot else {
        return;
    };
    if matches!(task.status.as_str(), "cancelled" | "completed" | "failed") {
        return;
    }
    if let Err(error) = persist_task(&tasks, &mut task, "running", None) {
        eprintln!("backtest sync {task_id} failed to mark running: {error}");
        return;
    }
    let result = if provider == "futu" {
        sync_futu_request_pages(
            &tasks,
            &market_store,
            trade_runtime.as_ref(),
            &request,
            &task_id,
            &mut task,
        )
        .await
    } else {
        match helper.as_ref() {
            Some(helper) => {
                sync_request_pages(
                    &tasks,
                    &market_store,
                    helper,
                    provider,
                    &request,
                    &task_id,
                    &mut task,
                )
                .await
            }
            None => Err("market-data helper is not configured".to_owned()),
        }
    };
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
        if is_cancelled(tasks, task_id)? {
            return Ok(());
        }
        task.current_interval = interval.clone();
        persist_task(tasks, task, "running", None)?;
        // Go's historical source asks for `until + 1ns` so a candle exactly
        // on the upper boundary is not lost by an exclusive helper query.
        let mut before = until + time::Duration::nanoseconds(1);
        let mut seen = std::collections::BTreeSet::new();
        let mut interval_inserted = false;
        loop {
            if is_cancelled(tasks, task_id)? {
                return Ok(());
            }
            let before_text = format_timestamp(before);
            let sessions = if request.session_scope == "extended" {
                "regular,extended"
            } else {
                "regular"
            };
            let query = [
                ("period", interval.as_str()),
                ("adjustment", request.rehab_type.as_str()),
                ("limit", "1000"),
                ("before", before_text.as_str()),
                ("sessions", sessions),
            ];
            let helper_market = if request.market == "CN" {
                symbol_market(&request.symbol)
            } else {
                request.market.as_str()
            };
            let response: HelperCandlesResponse = fetch_helper_page_with_retry(
                tasks,
                helper,
                provider,
                &["candles", helper_market, symbol_code(&request.symbol)],
                &query,
                task_id,
                task,
            )
            .await?;
            validate_helper_page(&response, helper_market, &request.symbol, interval)?;
            // Cancellation is persisted independently of the worker task.
            // Re-check immediately before writing a page so a response that
            // raced with CancelSync cannot insert candles after cancellation.
            if is_cancelled(tasks, task_id)? {
                return Ok(());
            }
            let mut rows = Vec::with_capacity(response.candles.len());
            for candle in response.candles {
                let at = parse_timestamp(&candle.at)?;
                if at < since || at >= until {
                    continue;
                }
                let end = at + interval_duration(interval) - time::Duration::milliseconds(1);
                rows.push(StoredBacktestCandle {
                    start_time: at.unix_timestamp_nanos() as i64 / 1_000_000,
                    end_time: end.unix_timestamp_nanos() as i64 / 1_000_000,
                    open: candle.open.0,
                    high: candle.high.0,
                    low: candle.low.0,
                    close: candle.close.0,
                    volume: candle
                        .volume
                        .map_or_else(|| "0".to_owned(), |value| value.0),
                });
            }
            interval_inserted |= !rows.is_empty();
            if !rows.is_empty() {
                if is_cancelled(tasks, task_id)? {
                    return Ok(());
                }
                market_store
                    .insert_candles(
                        provider,
                        &request.symbol,
                        interval,
                        &request.rehab_type,
                        &request.session_scope,
                        &rows,
                    )
                    .map_err(|error| error.to_string())?;
            }
            task.completed_batches += 1;
            persist_task(tasks, task, "running", None)?;
            if !response.has_more {
                break;
            }
            let next = response
                .next_before
                .as_deref()
                .ok_or_else(|| "helper returned hasMore without nextBefore".to_owned())
                .and_then(parse_timestamp)?;
            if next >= before || !seen.insert(next.unix_timestamp_nanos()) {
                return Err("helper pagination cursor did not move backward".to_owned());
            }
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

/// Sync OpenD pages using its opaque `nextReqKey` cursor.  The helper-backed
/// source uses a timestamp cursor, while Qot_RequestHistoryKL requires the
/// binary cursor to be passed back verbatim on each page.
async fn sync_futu_request_pages(
    tasks: &Arc<BacktestSyncTaskStore>,
    market_store: &Arc<BacktestMarketDataStore>,
    runtime: Option<&Arc<SharedTradeReadRuntime>>,
    request: &SyncRequest,
    task_id: &str,
    task: &mut StoredBacktestSyncTask,
) -> Result<(), String> {
    let runtime = runtime.ok_or_else(|| "Futu historical candle sync is unavailable".to_owned())?;
    let since = parse_timestamp(&request.since)?;
    let until = parse_timestamp(&request.until)?;
    let market = futu_market_code(&request.symbol)?;
    let code = symbol_code(&request.symbol).to_ascii_uppercase();
    for (index, interval) in request.intervals.iter().enumerate() {
        if is_cancelled(tasks, task_id)? {
            return Ok(());
        }
        task.current_interval = interval.clone();
        persist_task(tasks, task, "running", None)?;
        let mut cursor = Vec::new();
        let mut seen_cursors = std::collections::BTreeSet::new();
        let mut inserted = false;
        let mut exhausted = false;
        for _ in 0..32 {
            if is_cancelled(tasks, task_id)? {
                return Ok(());
            }
            let page = fetch_futu_page_with_retry(
                tasks, runtime, market, &code, interval, request, &cursor, task_id, task,
            )
            .await?;
            validate_futu_page(&page, market, &code, interval)?;
            if is_cancelled(tasks, task_id)? {
                return Ok(());
            }
            let rows = futu_rows(
                &page.klines,
                &request.symbol,
                interval,
                since,
                until,
                market,
            )?;
            inserted |= !rows.is_empty();
            if !rows.is_empty() {
                market_store
                    .insert_candles(
                        "futu",
                        &request.symbol,
                        interval,
                        &request.rehab_type,
                        &request.session_scope,
                        &rows,
                    )
                    .map_err(|error| error.to_string())?;
            }
            task.completed_batches += 1;
            task.total_batches = task.total_batches.max(task.completed_batches);
            persist_task(tasks, task, "running", None)?;
            if page.next_req_key.is_empty() {
                exhausted = true;
                break;
            }
            if !seen_cursors.insert(page.next_req_key.clone()) {
                return Err("OpenD historical pagination repeated nextReqKey".to_owned());
            }
            cursor = page.next_req_key;
        }
        if !exhausted {
            return Err("OpenD historical pagination exceeded 32 pages".to_owned());
        }
        if !inserted {
            return Err(
                "OpenD historical provider returned no candles in the requested range".to_owned(),
            );
        }
        task.completed_intervals = (index + 1) as i64;
        persist_task(tasks, task, "running", None)?;
    }
    Ok(())
}
