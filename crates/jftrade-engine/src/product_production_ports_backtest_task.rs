//! Production backtest execution request and task lifecycle.

use std::sync::Arc;
use std::time::Duration;

use jftrade_store_sqlite::BacktestRunStore;
use serde_json::{Value, json};
use tokio::sync::oneshot;

use super::ProductionBacktestPort;
use super::{persist_request_with_provider, requested_provider};
use super::product_production_ports_backtest_parse::{provider_id, with_execution_model};
use super::product_production_ports_backtest_strategy::{
    parse_start_request, resolve_strategy_payload,
};
use crate::product::product_backtest_execution::{
    BacktestExecutionCandle, BacktestExecutionPort, BacktestExecutionRequest, EXECUTION_TIMEOUT,
    next_run_id, now_timestamp,
};
use crate::product::product_backtests_write_port::{
    BacktestsWritePortError, BacktestsWritePortResult,
};

impl ProductionBacktestPort {
    #[allow(dead_code)]
    pub(crate) fn cancel_backtest(&self, run_id: &str) -> bool {
        let run = match self.store.get_run(run_id) {
            Ok(Some(run)) => run,
            Ok(None) => return false,
            Err(error) => {
                eprintln!("backtest {run_id} failed to load before cancellation: {error}");
                return false;
            }
        };
        if matches!(run.status.as_str(), "completed" | "failed" | "cancelled") {
            return false;
        }
        let timestamp = now_timestamp();
        let cancelled = jftrade_store_sqlite::StoredBacktestRun {
            status: "cancelled".to_owned(),
            result_json: json!({"error": "backtest cancelled"}).to_string(),
            updated_at: timestamp.clone(),
            ..run
        };
        let queued =
            match self
                .store
                .update_run_if_status(run_id, "queued", cancelled.clone(), &timestamp)
            {
                Ok(changed) => changed,
                Err(error) => {
                    eprintln!("backtest {run_id} queued cancellation failed: {error}");
                    false
                }
            };
        let running = if queued {
            false
        } else {
            match self
                .store
                .update_run_if_status(run_id, "running", cancelled, &timestamp)
            {
                Ok(changed) => changed,
                Err(error) => {
                    eprintln!("backtest {run_id} running cancellation failed: {error}");
                    false
                }
            }
        };
        if !queued && !running {
            return false;
        }
        if !self.execution_workers.cancel(run_id) {
            eprintln!("backtest {run_id} has no live execution worker to cancel");
        }
        true
    }

    pub(super) fn start_backtest(
        &self,
        payload: &Value,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        let execution_payload = resolve_strategy_payload(payload, &self.strategy_definitions)?;
        let provider_id = if let Some(provider_id) = requested_provider(payload)? {
            provider_id
        } else {
            provider_id(self.backtest_market_data_provider_state.get())
        };
        let request = parse_start_request(&execution_payload)?;
        let execution_payload = with_execution_model(&execution_payload, &request.execution_model)?;
        if let Some(readiness) = self.pine_readiness.as_ref()
            && !readiness.is_ready()
        {
            return Err(BacktestsWritePortError::Unavailable(
                readiness.unavailable_message(),
            ));
        }
        let execution = self.execution.clone().ok_or_else(|| {
            BacktestsWritePortError::Unavailable(
                "backtest worker runtime is not configured".to_owned(),
            )
        })?;
        let persisted_payload = with_execution_model(payload, &request.execution_model)?;

        let explicit_warmup = payload
            .get("warmupBars")
            .or_else(|| payload.get("warmup_bars"))
            .and_then(Value::as_u64)
            .map(|v| v as usize);
        let script = execution_payload
            .get("strategyScript")
            .or_else(|| execution_payload.get("script"))
            .or_else(|| execution_payload.get("strategySource"))
            .or_else(|| execution_payload.get("source"))
            .and_then(Value::as_str)
            .unwrap_or_default();
        let use_extended_hours = request.session_scope == "extended";
        let validation = jftrade_strategy::pinespec::validate_script(script, true, false);
        let derived_warmup = validation
            .requirements
            .as_ref()
            .map(|r| {
                r.derived_warmup_bars_with_session(
                    &request.symbol,
                    &request.interval,
                    use_extended_hours,
                )
            })
            .unwrap_or(0);
        let warmup_bars = match explicit_warmup {
            Some(explicit) => explicit.max(derived_warmup),
            None => derived_warmup,
        };

        let mut actual_warmup_count = 0;
        let mut candles = Vec::new();
        if warmup_bars > 0 {
            let interval_ms: i64 = match request.interval.trim().to_ascii_lowercase().as_str() {
                "1m" | "1min" => 60_000,
                "5m" | "5min" => 300_000,
                "15m" | "15min" => 900_000,
                "30m" | "30min" => 1_800_000,
                "60m" | "60min" | "1h" => 3_600_000,
                "1d" | "d" => 86_400_000,
                "1w" | "w" | "week" => 7 * 86_400_000,
                "1mo" | "1mon" | "1month" | "mo" | "month" => 30 * 86_400_000,
                _ => 60_000,
            };

            let multipliers = [3, 7, 14, 30, 60];
            let mut resolved_warmup_candles = None;

            for &multiplier in &multipliers {
                let bounded_start = request.start_time_ms.saturating_sub(
                    interval_ms.saturating_mul((warmup_bars as i64).saturating_mul(multiplier)),
                );
                let query_result = self._market_data_store.query_candles(
                    provider_id,
                    &request.symbol,
                    &request.interval,
                    &request.rehab_type,
                    &request.session_scope,
                    bounded_start,
                    request.start_time_ms,
                    "DESC",
                    warmup_bars,
                );

                match query_result {
                    Ok(mut warmup_candles) => {
                        if warmup_candles.len() >= warmup_bars {
                            warmup_candles.reverse();
                            resolved_warmup_candles = Some(warmup_candles);
                            break;
                        }
                        if resolved_warmup_candles
                            .as_ref()
                            .map(|c: &Vec<_>| c.len())
                            .unwrap_or(0)
                            < warmup_candles.len()
                        {
                            warmup_candles.reverse();
                            resolved_warmup_candles = Some(warmup_candles);
                        }
                    }
                    Err(err) => {
                        return Err(BacktestsWritePortError::Unavailable(format!(
                            "warmup candle query failed: {err}"
                        )));
                    }
                }
            }

            let warmup_candles = resolved_warmup_candles.unwrap_or_default();
            if warmup_candles.len() < warmup_bars {
                return Err(BacktestsWritePortError::Unavailable(format!(
                    "insufficient warmup candles: required {warmup_bars}, found {}",
                    warmup_candles.len()
                )));
            }

            for window in warmup_candles.windows(2) {
                if window[0].start_time >= window[1].start_time {
                    return Err(BacktestsWritePortError::Unavailable(
                        "warmup candles are not strictly ascending".to_owned(),
                    ));
                }
            }
            if let Some(last) = warmup_candles.last()
                && last.start_time >= request.start_time_ms
            {
                return Err(BacktestsWritePortError::Unavailable(
                    "warmup candle overlaps formal start time".to_owned(),
                ));
            }

            actual_warmup_count = warmup_candles.len();
            candles = warmup_candles;
        }

        let formal_candles = self
            ._market_data_store
            .read_candles(
                provider_id,
                &request.symbol,
                &request.interval,
                &request.rehab_type,
                &request.session_scope,
                request.start_time_ms,
                request.end_time_ms,
            )
            .map_err(|error| {
                BacktestsWritePortError::Unavailable(format!(
                    "backtest K-line data is not ready: {error}"
                ))
            })?;
        if formal_candles.is_empty() {
            return Err(BacktestsWritePortError::Unavailable(
                "backtest K-line data is not ready".to_owned(),
            ));
        }
        candles.extend(formal_candles);

        let mut execution_payload = execution_payload;
        if actual_warmup_count > 0
            && let Some(obj) = execution_payload.as_object_mut()
        {
            obj.insert("warmupBars".to_owned(), json!(actual_warmup_count));
        }
        let runtime = tokio::runtime::Handle::try_current().map_err(|_| {
            BacktestsWritePortError::Unavailable(
                "backtest execution runtime is not available".to_owned(),
            )
        })?;
        let run_id = next_run_id();
        let timestamp = now_timestamp();
        let run = jftrade_store_sqlite::StoredBacktestRun {
            id: run_id.clone(),
            status: "queued".to_owned(),
            request_json: persist_request_with_provider(&persisted_payload, provider_id),
            result_json: String::new(),
            created_at: timestamp.clone(),
            updated_at: timestamp.clone(),
        };
        self.store.save_run(run, &timestamp).map_err(|error| {
            BacktestsWritePortError::Failed(format!("persist backtest run: {error}"))
        })?;

        let worker_request = BacktestExecutionRequest {
            run_id: run_id.clone(),
            payload: execution_payload,
            market_data_provider: provider_id.to_owned(),
            candles: candles
                .into_iter()
                .map(|candle| BacktestExecutionCandle {
                    start_time: candle.start_time,
                    end_time: candle.end_time,
                    open: candle.open,
                    high: candle.high,
                    low: candle.low,
                    close: candle.close,
                    volume: candle.volume,
                })
                .collect(),
        };
        let store = Arc::clone(&self.store);
        let registry = Arc::clone(&self.execution_workers);
        let (cancel_tx, cancel_rx) = oneshot::channel();
        let handle = runtime.spawn(async move {
            let _ = execute_backtest_task(
                store,
                execution,
                worker_request,
                cancel_rx,
                EXECUTION_TIMEOUT,
            )
            .await;
        });
        registry.register(run_id.clone(), Arc::clone(&self.store), handle, cancel_tx);
        Ok(BacktestsWritePortResult::Data(json!({
            "id": run_id,
            "status": "queued",
            "request": persisted_payload,
            "createdAt": timestamp,
            "updatedAt": timestamp,
            "marketDataProvider": provider_id,
        })))
    }
}
async fn execute_backtest_task(
    store: Arc<BacktestRunStore>,
    execution: Arc<dyn BacktestExecutionPort>,
    request: BacktestExecutionRequest,
    mut cancel_rx: oneshot::Receiver<()>,
    timeout: Duration,
) {
    let timestamp = now_timestamp();
    let run = match store.get_run(&request.run_id) {
        Ok(Some(run)) => run,
        Ok(None) => {
            eprintln!("backtest {} disappeared before execution", request.run_id);
            return;
        }
        Err(error) => {
            eprintln!(
                "backtest {} failed to load before execution: {error}",
                request.run_id
            );
            return;
        }
    };
    let running = jftrade_store_sqlite::StoredBacktestRun {
        status: "running".to_owned(),
        updated_at: timestamp.clone(),
        ..run.clone()
    };
    match store.update_run_if_status(&request.run_id, "queued", running, &timestamp) {
        Ok(true) => {}
        Ok(false) => {
            eprintln!(
                "backtest {} was changed before execution started",
                request.run_id
            );
            return;
        }
        Err(error) => {
            eprintln!(
                "backtest {} failed to persist running state: {error}",
                request.run_id
            );
            return;
        }
    }
    let execution_request = request.clone();
    let join = tokio::task::spawn_blocking(move || execution.execute(execution_request));
    let outcome = tokio::select! {
        _ = &mut cancel_rx => TaskOutcome::Cancelled,
        _ = tokio::time::sleep(timeout) => TaskOutcome::TimedOut,
        result = join => match result {
            Ok(Ok(value)) => TaskOutcome::Completed(value),
            Ok(Err(error)) => TaskOutcome::Failed(error.to_string()),
            Err(error) if error.is_panic() => TaskOutcome::Failed("backtest worker panicked".to_owned()),
            Err(error) => TaskOutcome::Failed(error.to_string()),
        },
    };
    let (status, result) = match outcome {
        TaskOutcome::Completed(mut value) => {
            if value.is_object() && value.get("marketDataProvider").is_none() {
                value["marketDataProvider"] = Value::String(request.market_data_provider.clone());
            }
            ("completed", value)
        }
        TaskOutcome::Cancelled => ("cancelled", json!({"error": "backtest cancelled"})),
        TaskOutcome::TimedOut => ("failed", json!({"error": "backtest execution timed out"})),
        TaskOutcome::Failed(message) => ("failed", json!({"error": message})),
    };
    let timestamp = now_timestamp();
    let run = match store.get_run(&request.run_id) {
        Ok(Some(run)) => run,
        Ok(None) => {
            eprintln!("backtest {} disappeared before completion", request.run_id);
            return;
        }
        Err(error) => {
            eprintln!(
                "backtest {} failed to load before completion: {error}",
                request.run_id
            );
            return;
        }
    };
    let terminal = jftrade_store_sqlite::StoredBacktestRun {
        status: status.to_owned(),
        result_json: result.to_string(),
        updated_at: timestamp.clone(),
        ..run
    };
    match store.update_run_if_status(&request.run_id, "running", terminal, &timestamp) {
        Ok(true) => {}
        Ok(false) => eprintln!(
            "backtest {} completion was superseded by another state transition",
            request.run_id
        ),
        Err(error) => eprintln!(
            "backtest {} failed to persist terminal state: {error}",
            request.run_id
        ),
    }
}

enum TaskOutcome {
    Completed(Value),
    Cancelled,
    TimedOut,
    Failed(String),
}
