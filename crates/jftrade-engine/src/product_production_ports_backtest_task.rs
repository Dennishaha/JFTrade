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
        let candles = self
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
        if candles.is_empty() {
            return Err(BacktestsWritePortError::Unavailable(
                "backtest K-line data is not ready".to_owned(),
            ));
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
