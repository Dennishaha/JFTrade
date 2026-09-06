//! Research backtest data readiness check and synchronization orchestration.
//!
//! Provides deterministic coverage checking directly on the SQLite market-data
//! store, eliminating start-probe failures, and manages sync task lifecycles with
//! canonical sync keys and terminal state tracking.

use std::collections::HashMap;
use std::sync::Mutex;
use std::time::Instant;

use serde_json::{Value, json};

use crate::product::BacktestDataCoverageRequest;
use crate::product::product_backtests_write_port::{BacktestsWriteInput, BacktestsWritePortResult};
use crate::product::product_production_ports::ProductionPortBundle;
use crate::product::product_research_backtest_execution::{
    extract_run_metadata, research_script_hash,
};

#[derive(Clone, Debug)]
pub(crate) enum SyncTaskState {
    Syncing {
        task_id: String,
        started_at: Instant,
    },
    Terminal {
        task_id: String,
        status: String,
        error: String,
        recorded_at: Instant,
    },
}

#[derive(Default)]
pub(crate) struct SyncStateTracker {
    states: Mutex<HashMap<String, SyncTaskState>>,
}

impl SyncStateTracker {
    pub(crate) fn global() -> &'static SyncStateTracker {
        static INSTANCE: std::sync::OnceLock<SyncStateTracker> = std::sync::OnceLock::new();
        INSTANCE.get_or_init(SyncStateTracker::default)
    }

    pub(crate) fn get(&self, sync_key: &str) -> Option<SyncTaskState> {
        let states = self.states.lock().unwrap_or_else(|e| e.into_inner());
        states.get(sync_key).cloned()
    }

    pub(crate) fn set_syncing(&self, sync_key: String, task_id: String) {
        let mut states = self.states.lock().unwrap_or_else(|e| e.into_inner());
        states.insert(
            sync_key,
            SyncTaskState::Syncing {
                task_id,
                started_at: Instant::now(),
            },
        );
    }

    pub(crate) fn set_terminal(
        &self,
        sync_key: String,
        task_id: String,
        status: String,
        error: String,
    ) {
        let mut states = self.states.lock().unwrap_or_else(|e| e.into_inner());
        states.insert(
            sync_key,
            SyncTaskState::Terminal {
                task_id,
                status,
                error,
                recorded_at: Instant::now(),
            },
        );
    }

    #[allow(dead_code)]
    pub(crate) fn clear(&self) {
        let mut states = self.states.lock().unwrap_or_else(|e| e.into_inner());
        states.clear();
    }
}

pub(crate) enum EnsureDataOutcome {
    Ready,
    Syncing(Value),
}

pub(crate) fn build_sync_key(
    provider: &str,
    symbol: &str,
    interval: &str,
    since: &str,
    until: &str,
    rehab: &str,
    session: &str,
) -> String {
    format!("{provider}|{symbol}|{interval}|{since}|{until}|{rehab}|{session}")
}

pub(crate) fn derive_effective_since_time(
    start_time_str: &str,
    interval_str: &str,
    warmup_bars: usize,
) -> String {
    let interval_ms: i64 = match interval_str.trim().to_ascii_lowercase().as_str() {
        "1m" | "1min" => 60_000,
        "5m" | "5min" => 300_000,
        "15m" | "15min" => 900_000,
        "30m" | "30min" => 1_800_000,
        "60m" | "60min" | "1h" => 3_600_000,
        "1d" | "d" => 86_400_000,
        "1w" | "w" => 7 * 86_400_000,
        _ => 60_000,
    };

    let start_ms = if let Ok(dt) = time::OffsetDateTime::parse(
        start_time_str,
        &time::format_description::well_known::Rfc3339,
    ) {
        (dt.unix_timestamp_nanos() / 1_000_000) as i64
    } else {
        start_time_str.parse::<i64>().unwrap_or(1_704_067_200_000)
    };

    if warmup_bars == 0 {
        return start_time_str.to_owned();
    }

    let buffer_ms = (warmup_bars as i64)
        .saturating_mul(interval_ms)
        .saturating_mul(7);
    let since_ms = start_ms.saturating_sub(buffer_ms);

    time::OffsetDateTime::from_unix_timestamp_nanos((since_ms as i128) * 1_000_000)
        .ok()
        .and_then(|dt| {
            dt.format(&time::format_description::well_known::Rfc3339)
                .ok()
        })
        .unwrap_or_else(|| start_time_str.to_owned())
}

#[allow(clippy::too_many_arguments)]
pub(crate) fn format_syncing_response(
    sync_data: &Value,
    task_id: &str,
    status: &str,
    progress: f64,
    symbol: &str,
    interval: &str,
    provider: &str,
    since: &str,
    until: &str,
    session_scope: &str,
    rehab_type: &str,
    script: &str,
    validation: &jftrade_strategy::pinespec::ValidationPayload,
    arguments: &Value,
) -> Value {
    let (_, chart_type, inst_type, extended, exec_model, fees) =
        extract_run_metadata(None, arguments);

    json!({
        "ok": true,
        "status": "syncing_data",
        "dataSync": {
            "taskId": task_id,
            "status": status,
            "progress": progress,
            "symbol": symbol,
            "intervals": sync_data.get("intervals").cloned().unwrap_or_else(|| json!([interval])),
            "marketDataProvider": provider,
            "since": since,
            "until": until,
            "sessionScope": session_scope,
            "rehabType": rehab_type,
        },
        "nextTool": {
            "name": "backtest.kline_sync_status",
            "input": {
                "taskId": task_id,
                "waitForCompletionMs": 25000,
            }
        },
        "nextAction": "wait_kline_sync",
        "suggestedArguments": {
            "taskId": task_id,
        },
        "message": format!(
            "K-line data is being synchronized (task {task_id}). Please check status via backtest.kline_sync_status before executing research backtest."
        ),
        "chartType": chart_type,
        "instrumentType": inst_type,
        "useExtendedHours": extended,
        "executionModel": exec_model,
        "tradingCosts": fees,
        "scriptHash": research_script_hash(script),
        "validation": {
            "metadata": validation.metadata,
            "hooks": validation.hooks,
            "warnings": validation.warnings,
        },
        "saveRecommendation": "仅当用户明确要求保存/发布/更新策略定义时，再调用 strategy.save_definition。",
    })
}

fn parse_timestamp_ms_helper(s: &str, is_end: bool) -> i64 {
    if let Ok(dt) = time::OffsetDateTime::parse(s, &time::format_description::well_known::Rfc3339) {
        (dt.unix_timestamp_nanos() / 1_000_000) as i64
    } else if let Ok(val) = s.parse::<i64>() {
        val
    } else if is_end {
        1_704_153_600_000
    } else {
        1_704_067_200_000
    }
}

struct ReadinessContext<'a> {
    symbol: &'a str,
    interval: &'a str,
    market: &'a str,
    session_scope: &'a str,
    rehab_type: &'a str,
    provider: String,
    start_time_str: &'a str,
    end_time_str: &'a str,
    start_time_ms: i64,
    end_time_ms: i64,
    warmup_bars: usize,
    script: &'a str,
}

impl<'a> ReadinessContext<'a> {
    fn from_payload(
        start_payload: &'a Value,
        arguments: &'a Value,
        validation: &jftrade_strategy::pinespec::ValidationPayload,
        script: &'a str,
    ) -> Result<Self, String> {
        let symbol = start_payload
            .get("symbol")
            .and_then(Value::as_str)
            .unwrap_or_default();
        let interval = start_payload
            .get("interval")
            .or_else(|| start_payload.get("period"))
            .and_then(Value::as_str)
            .unwrap_or("1m");
        let market = start_payload
            .get("market")
            .and_then(Value::as_str)
            .unwrap_or("HK");
        let session_scope = match start_payload
            .get("sessionScope")
            .and_then(Value::as_str)
            .unwrap_or("regular")
        {
            "extended" => "extended",
            _ => "regular",
        };
        let rehab_type = match start_payload
            .get("rehabType")
            .and_then(Value::as_str)
            .unwrap_or("forward")
        {
            "backward" => "backward",
            "none" => "none",
            _ => "forward",
        };
        let provider = extract_run_metadata(None, arguments).0;
        let warmup_bars = resolve_warmup_bars(
            arguments,
            validation,
            symbol,
            interval,
            session_scope == "extended",
        )?;

        let start_time_str = start_payload
            .get("startTime")
            .or_else(|| start_payload.get("startDate"))
            .and_then(Value::as_str)
            .unwrap_or("2026-01-01T00:00:00Z");
        let end_time_str = start_payload
            .get("endTime")
            .or_else(|| start_payload.get("endDate"))
            .and_then(Value::as_str)
            .unwrap_or("2026-01-02T00:00:00Z");

        let start_time_ms = parse_timestamp_ms_helper(start_time_str, false);
        let end_time_ms = parse_timestamp_ms_helper(end_time_str, true);

        Ok(Self {
            symbol,
            interval,
            market,
            session_scope,
            rehab_type,
            provider,
            start_time_str,
            end_time_str,
            start_time_ms,
            end_time_ms,
            warmup_bars,
            script,
        })
    }
}

fn resolve_warmup_bars(
    arguments: &Value,
    validation: &jftrade_strategy::pinespec::ValidationPayload,
    symbol: &str,
    interval: &str,
    use_extended_hours: bool,
) -> Result<usize, String> {
    let derived_warmup = match validation.requirements.as_ref() {
        Some(r) => r
            .try_derived_warmup_bars_with_session(symbol, interval, use_extended_hours)
            .map_err(|e| format!("invalid timeframe alignment: {e}"))?,
        None => 0,
    };
    let explicit_warmup = arguments
        .get("warmupBars")
        .or_else(|| arguments.get("warmup_bars"))
        .and_then(Value::as_u64)
        .map(|v| v as usize)
        .unwrap_or(0);
    Ok(explicit_warmup.max(derived_warmup))
}

#[allow(clippy::too_many_arguments)]
fn poll_active_sync_task(
    ports: &ProductionPortBundle,
    tracker: &SyncStateTracker,
    sync_key: &str,
    task_id: &str,
    coverage_req: &BacktestDataCoverageRequest,
    ctx: &ReadinessContext<'_>,
    since_str: &str,
    validation: &jftrade_strategy::pinespec::ValidationPayload,
    arguments: &Value,
) -> Result<Option<EnsureDataOutcome>, String> {
    let Some(task) = ports.backtest_sync.progress(task_id).ok().flatten() else {
        return Ok(None);
    };
    let st = task.get("status").and_then(Value::as_str).unwrap_or("");
    if st == "completed" {
        if let Ok(true) = ports.backtest_sync.check_coverage(coverage_req) {
            return Ok(Some(EnsureDataOutcome::Ready));
        }
        tracker.set_terminal(
            sync_key.to_owned(),
            task_id.to_owned(),
            "failed".to_owned(),
            "insufficient candles after sync completed".to_owned(),
        );
        return Err(format!(
            "K-line data sync completed but coverage is still insufficient for {}. Cannot proceed.",
            ctx.symbol
        ));
    }
    if st == "failed" || st == "cancelled" {
        let err_msg = task
            .get("error")
            .and_then(Value::as_str)
            .unwrap_or("sync task terminated")
            .to_owned();
        tracker.set_terminal(
            sync_key.to_owned(),
            task_id.to_owned(),
            st.to_owned(),
            err_msg.clone(),
        );
        return Err(format!(
            "K-line data sync for {} terminated with {st}: {err_msg}",
            ctx.symbol
        ));
    }
    let total_intervals = task
        .get("totalIntervals")
        .and_then(Value::as_i64)
        .unwrap_or(1);
    let completed_intervals = task
        .get("completedIntervals")
        .and_then(Value::as_i64)
        .unwrap_or(0);
    let progress = if total_intervals > 0 {
        (completed_intervals as f64 / total_intervals as f64 * 100.0).round()
    } else {
        0.0
    };
    Ok(Some(EnsureDataOutcome::Syncing(format_syncing_response(
        &task,
        task_id,
        st,
        progress,
        ctx.symbol,
        ctx.interval,
        &ctx.provider,
        since_str,
        ctx.end_time_str,
        ctx.session_scope,
        ctx.rehab_type,
        ctx.script,
        validation,
        arguments,
    ))))
}

fn find_reusable_active_sync_task(
    ports: &ProductionPortBundle,
    tracker: &SyncStateTracker,
    sync_key: &str,
    ctx: &ReadinessContext<'_>,
    since_str: &str,
    validation: &jftrade_strategy::pinespec::ValidationPayload,
    arguments: &Value,
) -> Option<EnsureDataOutcome> {
    let active_tasks = ports.backtest_sync.active_tasks().ok()?;
    for task in active_tasks {
        let st = task.get("status").and_then(Value::as_str).unwrap_or("");
        let sym = task.get("symbol").and_then(Value::as_str).unwrap_or("");
        let prov = task
            .get("marketDataProvider")
            .and_then(Value::as_str)
            .unwrap_or("");
        if (st == "queued" || st == "running")
            && (sym == ctx.symbol || sym.ends_with(ctx.symbol) || ctx.symbol.ends_with(sym))
            && (prov.is_empty() || prov == ctx.provider)
        {
            let task_id = task
                .get("taskId")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .to_owned();
            tracker.set_syncing(sync_key.to_owned(), task_id.clone());
            return Some(EnsureDataOutcome::Syncing(format_syncing_response(
                &task,
                &task_id,
                st,
                0.0,
                ctx.symbol,
                ctx.interval,
                &ctx.provider,
                since_str,
                ctx.end_time_str,
                ctx.session_scope,
                ctx.rehab_type,
                ctx.script,
                validation,
                arguments,
            )));
        }
    }
    None
}

fn trigger_new_kline_sync(
    ports: &ProductionPortBundle,
    tracker: &SyncStateTracker,
    sync_key: &str,
    ctx: &ReadinessContext<'_>,
    since_str: &str,
    validation: &jftrade_strategy::pinespec::ValidationPayload,
    arguments: &Value,
) -> Result<EnsureDataOutcome, String> {
    let sync_payload = json!({
        "market": ctx.market,
        "symbol": ctx.symbol,
        "intervals": [ctx.interval],
        "since": since_str,
        "until": ctx.end_time_str,
        "sessionScope": ctx.session_scope,
        "rehabType": ctx.rehab_type,
        "marketDataProvider": ctx.provider,
    });
    let sync_result = ports
        .backtests_write
        .mutate(&BacktestsWriteInput::Sync {
            payload: sync_payload,
        })
        .map_err(|e| format!("failed to start K-line data sync: {e:?}"))?;

    match sync_result {
        BacktestsWritePortResult::Data(sync_data) => {
            let task_id = sync_data
                .get("taskId")
                .and_then(Value::as_str)
                .unwrap_or("unknown")
                .to_owned();
            tracker.set_syncing(sync_key.to_owned(), task_id.clone());
            Ok(EnsureDataOutcome::Syncing(format_syncing_response(
                &sync_data,
                &task_id,
                "queued",
                0.0,
                ctx.symbol,
                ctx.interval,
                &ctx.provider,
                since_str,
                ctx.end_time_str,
                ctx.session_scope,
                ctx.rehab_type,
                ctx.script,
                validation,
                arguments,
            )))
        }
        other => Err(format!("unexpected sync task start result: {other:?}")),
    }
}

pub(crate) fn ensure_research_data_readiness(
    ports: &ProductionPortBundle,
    arguments: &Value,
    validation: &jftrade_strategy::pinespec::ValidationPayload,
    start_payload: &Value,
    script: &str,
) -> Result<EnsureDataOutcome, String> {
    let ctx = ReadinessContext::from_payload(start_payload, arguments, validation, script)?;

    let coverage_req = BacktestDataCoverageRequest {
        provider: ctx.provider.clone(),
        symbol: ctx.symbol.to_owned(),
        interval: ctx.interval.to_owned(),
        rehab_type: ctx.rehab_type.to_owned(),
        session_scope: ctx.session_scope.to_owned(),
        start_time_ms: ctx.start_time_ms,
        end_time_ms: ctx.end_time_ms,
        warmup_bars: ctx.warmup_bars,
    };
    if let Ok(true) = ports.backtest_sync.check_coverage(&coverage_req) {
        return Ok(EnsureDataOutcome::Ready);
    }

    let since_str = derive_effective_since_time(ctx.start_time_str, ctx.interval, ctx.warmup_bars);
    let sync_key = build_sync_key(
        &ctx.provider,
        ctx.symbol,
        ctx.interval,
        &since_str,
        ctx.end_time_str,
        ctx.rehab_type,
        ctx.session_scope,
    );

    let tracker = SyncStateTracker::global();
    if let Some(state) = tracker.get(&sync_key) {
        match state {
            SyncTaskState::Terminal {
                status,
                error,
                task_id,
                recorded_at,
            } => {
                if recorded_at.elapsed() < std::time::Duration::from_secs(60) {
                    return Err(format!(
                        "K-line data sync for {} (task {task_id}) terminated with {status}: {error}. Cannot execute backtest without required market data.",
                        ctx.symbol
                    ));
                }
            }
            SyncTaskState::Syncing {
                task_id,
                started_at,
            } => {
                let outcome = if started_at.elapsed() < std::time::Duration::from_secs(300) {
                    poll_active_sync_task(
                        ports,
                        tracker,
                        &sync_key,
                        &task_id,
                        &coverage_req,
                        &ctx,
                        &since_str,
                        validation,
                        arguments,
                    )?
                } else {
                    None
                };
                if let Some(outcome) = outcome {
                    return Ok(outcome);
                }
            }
        }
    }

    if let Some(outcome) = find_reusable_active_sync_task(
        ports, tracker, &sync_key, &ctx, &since_str, validation, arguments,
    ) {
        return Ok(outcome);
    }

    trigger_new_kline_sync(
        ports, tracker, &sync_key, &ctx, &since_str, validation, arguments,
    )
}
