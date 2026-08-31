//! Historical candle sync request and task lifecycle.

use std::sync::{
    Arc,
    atomic::{AtomicU64, Ordering},
};

use jftrade_integration_futu::{
    HistoricalKline, HistoricalKlineError, HistoricalKlineQuery, HistoricalKlineResult,
};
use jftrade_integration_marketdata_helper::{HelperCandlesResponse, HelperClient};
use jftrade_settings::MarketDataProvider;
use jftrade_store_sqlite::{
    BacktestMarketDataStore, BacktestSyncTaskStore, CancelBacktestSyncResult, StoredBacktestCandle,
    StoredBacktestSyncTask,
};
use serde_json::{Value, json};
use tokio::sync::oneshot;

use super::ProductionBacktestPort;
use crate::product::product_production_ports::SharedTradeReadRuntime;
use super::product_backtest_sync_request::{
    SyncRequest, format_timestamp, parse_sync_request, parse_timestamp,
};
use super::requested_provider;
use crate::product::product_backtests_write_port::{
    BacktestsWritePortError, BacktestsWritePortResult,
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

#[allow(clippy::too_many_arguments)]
async fn fetch_futu_page_with_retry(
    tasks: &Arc<BacktestSyncTaskStore>,
    runtime: &SharedTradeReadRuntime,
    market: i32,
    code: &str,
    interval: &str,
    request: &SyncRequest,
    cursor: &[u8],
    task_id: &str,
    task: &mut StoredBacktestSyncTask,
) -> Result<HistoricalKlineResult, String> {
    let begin_time = opend_wall_clock(&request.since, &request.market)?;
    let end_time = opend_wall_clock(&request.until, &request.market)?;
    let query = HistoricalKlineQuery {
        market,
        symbol: code.to_owned(),
        period: interval.to_owned(),
        adjustment: match request.rehab_type.as_str() {
            "none" => 0,
            "backward" => 2,
            _ => 1,
        },
        begin_time,
        end_time,
        max_ack_kl_num: Some(1000),
        next_req_key: cursor.to_vec(),
        extended_time: (request.session_scope == "extended").then_some(true),
        session: (request.session_scope == "extended").then_some(3),
    };
    let mut last_error = None;
    for attempt in 0..4 {
        if is_cancelled(tasks, task_id)? {
            return Err("sync cancelled".to_owned());
        }
        let Some(reader) = runtime.historical_klines_reader() else {
            return Err("Futu historical candle sync is unavailable".to_owned());
        };
        let call_query = query.clone();
        let joined = tokio::task::spawn_blocking(move || reader.query(&call_query));
        let outcome = tokio::time::timeout(std::time::Duration::from_secs(30), joined).await;
        match outcome {
            Ok(Ok(Ok(page))) => return Ok(page),
            Ok(Ok(Err(error))) => {
                let retryable = futu_error_retryable(&error);
                last_error = Some(error.to_string());
                if !retryable || attempt == 3 {
                    break;
                }
            }
            Ok(Err(error)) => {
                last_error = Some(format!("OpenD historical worker failed: {error}"));
                if attempt == 3 {
                    break;
                }
            }
            Err(_) => {
                last_error = Some("OpenD historical request timed out".to_owned());
                if attempt == 3 {
                    break;
                }
            }
        }
        task.retries += 1;
        persist_task(tasks, task, "running", None)?;
        tokio::time::sleep(std::time::Duration::from_millis(250 * (attempt as u64 + 1))).await;
    }
    Err(last_error.unwrap_or_else(|| "OpenD historical request failed".to_owned()))
}

fn futu_error_retryable(error: &HistoricalKlineError) -> bool {
    match error {
        HistoricalKlineError::Session(_) => true,
        HistoricalKlineError::Rejected { err_code, .. } => {
            *err_code == 408 || *err_code == 425 || *err_code == 429 || *err_code >= 500
        }
        HistoricalKlineError::Decode(_) | HistoricalKlineError::MissingS2c => false,
    }
}

fn validate_futu_page(
    page: &HistoricalKlineResult,
    market: i32,
    code: &str,
    interval: &str,
) -> Result<(), String> {
    if page.security.market != market || !page.security.code.eq_ignore_ascii_case(code) {
        return Err("OpenD historical response identity is invalid".to_owned());
    }
    if page
        .klines
        .iter()
        .any(|candle| candle.time.trim().is_empty())
    {
        return Err("OpenD historical response contains an empty candle timestamp".to_owned());
    }
    if !matches!(
        interval,
        "1m" | "5m" | "15m" | "30m" | "1h" | "1d" | "1w" | "1mo"
    ) {
        return Err(format!("OpenD does not support interval {interval}"));
    }
    Ok(())
}

fn futu_rows(
    candles: &[HistoricalKline],
    _symbol: &str,
    interval: &str,
    since: time::OffsetDateTime,
    until: time::OffsetDateTime,
    market: i32,
) -> Result<Vec<StoredBacktestCandle>, String> {
    let market_label = futu_market_label(market);
    let mut rows = Vec::with_capacity(candles.len());
    for candle in candles {
        if candle.is_blank {
            continue;
        }
        let at = parse_futu_candle_time(&candle.time, market_label)?;
        if at < since || at >= until {
            continue;
        }
        let (Some(open), Some(high), Some(low), Some(close)) = (
            candle.open_price,
            candle.high_price,
            candle.low_price,
            candle.close_price,
        ) else {
            return Err("OpenD historical candle is missing OHLC values".to_owned());
        };
        rows.push(StoredBacktestCandle {
            start_time: at.unix_timestamp_nanos() as i64 / 1_000_000,
            end_time: (at + interval_duration(interval) - time::Duration::milliseconds(1))
                .unix_timestamp_nanos() as i64
                / 1_000_000,
            open: futu_decimal(open, "open")?,
            high: futu_decimal(high, "high")?,
            low: futu_decimal(low, "low")?,
            close: futu_decimal(close, "close")?,
            volume: candle.volume.unwrap_or_default().to_string(),
        });
    }
    rows.sort_by_key(|row| row.start_time);
    Ok(rows)
}

fn futu_decimal(value: f64, field: &str) -> Result<String, String> {
    if !value.is_finite() {
        return Err(format!("OpenD historical {field} is not finite"));
    }
    Ok(format!("{value:.8}"))
}

fn futu_market_code(symbol: &str) -> Result<i32, String> {
    match symbol.split_once('.').map(|(market, _)| market) {
        Some("HK") => Ok(1),
        Some("US") => Ok(11),
        Some("SH") => Ok(21),
        Some("SZ") => Ok(22),
        _ => Err("Futu historical sync requires a supported exchange-qualified symbol".to_owned()),
    }
}

fn futu_market_label(market: i32) -> &'static str {
    match market {
        11 => "US",
        21 => "SH",
        22 => "SZ",
        _ => "HK",
    }
}

fn futu_timezone(market: &str) -> &'static str {
    match market {
        "US" => "America/New_York",
        "HK" => "Asia/Hong_Kong",
        "SH" | "SZ" | "CN" => "Asia/Shanghai",
        _ => "UTC",
    }
}

fn opend_wall_clock(value: &str, market: &str) -> Result<String, String> {
    let timestamp: jiff::Timestamp = value
        .parse()
        .map_err(|error| format!("invalid timestamp for OpenD: {error}"))?;
    let local = timestamp
        .to_zoned(jiff::tz::TimeZone::get(futu_timezone(market)).map_err(|e| e.to_string())?);
    Ok(local.strftime("%Y-%m-%d %H:%M:%S").to_string())
}

fn parse_futu_candle_time(value: &str, market: &str) -> Result<time::OffsetDateTime, String> {
    if value.contains('T') || value.ends_with('Z') || value.contains('+') {
        return parse_timestamp(value);
    }
    let local = jiff::civil::DateTime::strptime("%Y-%m-%d %H:%M:%S", value.trim())
        .map_err(|error| format!("invalid OpenD candle timestamp: {error}"))?;
    let zoned = local
        .in_tz(futu_timezone(market))
        .map_err(|error| format!("invalid OpenD candle timestamp: {error}"))?;
    parse_timestamp(&zoned.timestamp().to_string())
}

async fn fetch_helper_page_with_retry(
    tasks: &Arc<BacktestSyncTaskStore>,
    helper: &HelperClient,
    provider: &str,
    segments: &[&str],
    query: &[(&str, &str)],
    task_id: &str,
    task: &mut StoredBacktestSyncTask,
) -> Result<HelperCandlesResponse, String> {
    let mut last_error = None;
    for attempt in 0..4 {
        if is_cancelled(tasks, task_id)? {
            return Err("sync cancelled".to_owned());
        }
        match helper
            .get_provider_json_with_query(provider, segments, query)
            .await
        {
            Ok(response) => return Ok(response),
            Err(error) => {
                let retryable = is_retryable_helper_error(&error);
                last_error = Some(error.to_string());
                if !retryable || attempt == 3 {
                    break;
                }
                task.retries += 1;
                persist_task(tasks, task, "running", None)?;
                let delay = std::time::Duration::from_millis(250 * (attempt as u64 + 1));
                tokio::time::sleep(delay).await;
            }
        }
    }
    Err(last_error.unwrap_or_else(|| "helper request failed".to_owned()))
}

fn is_retryable_helper_error(
    error: &jftrade_integration_marketdata_helper::HttpAdapterError,
) -> bool {
    use jftrade_integration_marketdata_helper::HttpAdapterError;
    match error {
        HttpAdapterError::Timeout | HttpAdapterError::Unavailable(_) => true,
        HttpAdapterError::Remote { status, .. } => {
            *status == 408 || *status == 425 || *status == 429 || *status >= 500
        }
        HttpAdapterError::InvalidUrl(_)
        | HttpAdapterError::WeakToken
        | HttpAdapterError::InvalidResponse(_) => false,
    }
}

fn symbol_code(symbol: &str) -> &str {
    symbol.split_once('.').map_or(symbol, |(_, code)| code)
}

fn symbol_market(symbol: &str) -> &str {
    symbol.split_once('.').map_or("", |(market, _)| market)
}

fn interval_duration(interval: &str) -> time::Duration {
    match interval {
        "1m" => time::Duration::minutes(1),
        "5m" => time::Duration::minutes(5),
        "15m" => time::Duration::minutes(15),
        "30m" => time::Duration::minutes(30),
        "1h" => time::Duration::hours(1),
        "1w" => time::Duration::days(7),
        "1mo" => time::Duration::days(30),
        _ => time::Duration::days(1),
    }
}

fn validate_helper_page(
    response: &HelperCandlesResponse,
    market: &str,
    symbol: &str,
    interval: &str,
) -> Result<(), String> {
    let expected_instrument = format!("{market}.{}", symbol_code(symbol));
    if !response.market.eq_ignore_ascii_case(market)
        || !response.symbol.eq_ignore_ascii_case(symbol_code(symbol))
        || !response
            .instrument_id
            .eq_ignore_ascii_case(&expected_instrument)
        || response.period != interval
        || response.total_returned != response.candles.len()
    {
        return Err("helper candle response identity is invalid".to_owned());
    }
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
    if let Err(error) = tasks.cancel(task_id, &timestamp) {
        eprintln!("backtest sync {task_id} cancellation persistence failed: {error}");
    }
}

fn persist_task(
    tasks: &BacktestSyncTaskStore,
    task: &mut StoredBacktestSyncTask,
    status: &str,
    error: Option<String>,
) -> Result<(), String> {
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
