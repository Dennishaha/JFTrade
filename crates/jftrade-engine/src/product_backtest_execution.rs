//! Rust-owned asynchronous backtest execution boundary.
//!
//! The HTTP adapter deliberately knows nothing about PineTS, strategy
//! definitions, or the matching engine.  Those concerns are supplied through
//! [`BacktestExecutionPort`].  This keeps the default production composition
//! fail-closed while allowing a fixture/worker adapter to exercise the real
//! `jftrade-backtest::run_json` boundary in explicit compositions.

use std::sync::{
    Arc, Mutex,
    atomic::{AtomicU64, Ordering},
};
use std::time::Duration;

use jftrade_store_sqlite::{BacktestRunStore, StoredBacktestCandle, StoredBacktestRun};
use serde_json::{Value, json};
use thiserror::Error;
use tokio::sync::oneshot;
use tokio::task::JoinHandle;

/// Input handed to a strategy/Pine/backtest adapter after request and history
/// validation.  The raw request is retained so adapters can preserve fields
/// that are not part of the Rust domain model yet.
#[derive(Clone, Debug)]
pub struct BacktestExecutionRequest {
    pub run_id: String,
    pub payload: Value,
    pub market_data_provider: String,
    pub candles: Vec<StoredBacktestCandle>,
}

#[derive(Debug, Error, Clone, Eq, PartialEq)]
pub enum BacktestExecutionError {
    #[error("backtest execution is unavailable: {0}")]
    Unavailable(String),
    #[error("invalid backtest execution input: {0}")]
    Invalid(String),
    #[error("backtest execution failed: {0}")]
    Failed(String),
}

/// Narrow adapter contract for strategy/PineTS and the deterministic Rust
/// matcher.  Implementations may perform blocking work; the task registry
/// runs them behind `spawn_blocking` and fences the resulting write with a
/// status CAS.
pub trait BacktestExecutionPort: Send + Sync + std::fmt::Debug {
    fn execute(&self, request: BacktestExecutionRequest) -> Result<Value, BacktestExecutionError>;
}

/// Explicit adapter used by fixtures and local rehearsals.  It invokes the
/// existing `jftrade-backtest::run_json` corpus boundary and returns its
/// decoded JSON output.  A normal StartRequest is not itself a corpus; callers
/// must provide a `corpus` object (or a corpus-shaped payload) produced by the
/// strategy/Pine adapter.
#[derive(Clone, Copy, Debug, Default)]
pub struct RunJsonBacktestExecutionPort;

impl BacktestExecutionPort for RunJsonBacktestExecutionPort {
    fn execute(&self, request: BacktestExecutionRequest) -> Result<Value, BacktestExecutionError> {
        let mut corpus = request
            .payload
            .get("corpus")
            .cloned()
            .unwrap_or_else(|| request.payload.clone());
        // A strategy adapter may provide only corpus metadata while history is
        // resolved by the production market-data store.  Fill an explicitly
        // empty first case from that validated history; never overwrite
        // worker-provided candles.
        if let Some(case) = corpus
            .get_mut("cases")
            .and_then(Value::as_array_mut)
            .and_then(|cases| cases.first_mut())
            && case
                .get("candles")
                .and_then(Value::as_array)
                .is_none_or(Vec::is_empty)
        {
            case["candles"] = Value::Array(request.candles.iter().map(candle_wire).collect());
        }
        let bytes = serde_json::to_vec(&corpus)
            .map_err(|error| BacktestExecutionError::Invalid(error.to_string()))?;
        let output = jftrade_backtest::run_json(&bytes)
            .map_err(|error| BacktestExecutionError::Failed(error.to_string()))?;
        serde_json::from_slice(&output)
            .map_err(|error| BacktestExecutionError::Failed(error.to_string()))
    }
}

fn candle_wire(candle: &StoredBacktestCandle) -> Value {
    let timestamp = |millis: i64| {
        time::OffsetDateTime::from_unix_timestamp_nanos(i128::from(millis) * 1_000_000)
            .ok()
            .and_then(|value| {
                value
                    .format(&time::format_description::well_known::Rfc3339)
                    .ok()
            })
            .unwrap_or_else(|| "1970-01-01T00:00:00Z".to_owned())
    };
    json!({
        "start": timestamp(candle.start_time),
        "end": timestamp(candle.end_time),
        "open": candle.open,
        "high": candle.high,
        "low": candle.low,
        "close": candle.close,
        "volume": candle.volume,
    })
}

/// Lifecycle registry for asynchronous run workers.  It owns every join
/// handle and cancellation sender, and marks a non-terminal record failed if
/// a worker exits unexpectedly.
#[derive(Default)]
pub(crate) struct BacktestExecutionTaskRegistry {
    workers: Mutex<Vec<Worker>>,
}

struct Worker {
    run_id: String,
    store: Arc<BacktestRunStore>,
    handle: JoinHandle<()>,
    cancel: Option<oneshot::Sender<()>>,
}

impl std::fmt::Debug for BacktestExecutionTaskRegistry {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("BacktestExecutionTaskRegistry")
            .field(
                "workers",
                &self
                    .workers
                    .lock()
                    .map(|workers| workers.len())
                    .unwrap_or_default(),
            )
            .finish()
    }
}

impl BacktestExecutionTaskRegistry {
    pub(crate) fn register(
        &self,
        run_id: String,
        store: Arc<BacktestRunStore>,
        handle: JoinHandle<()>,
        cancel: oneshot::Sender<()>,
    ) {
        self.reap_finished();
        self.workers
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .push(Worker {
                run_id,
                store,
                handle,
                cancel: Some(cancel),
            });
    }

    #[allow(dead_code)]
    pub(crate) fn cancel(&self, run_id: &str) -> bool {
        let mut workers = self
            .workers
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        let Some(worker) = workers.iter_mut().find(|worker| worker.run_id == run_id) else {
            return false;
        };
        if let Some(cancel) = worker.cancel.take() {
            let _ = cancel.send(());
            true
        } else {
            false
        }
    }

    pub(crate) fn reap_finished(&self) {
        let mut workers = self
            .workers
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        workers.retain(|worker| {
            if !worker.handle.is_finished() {
                return true;
            }
            let run = worker.store.get_run(&worker.run_id).ok().flatten();
            let terminal = run.as_ref().is_none_or(|run| {
                matches!(run.status.as_str(), "completed" | "failed" | "cancelled")
            });
            if let Some(run) = run.filter(|_| !terminal) {
                let timestamp = now_timestamp();
                let expected = run.status.clone();
                let failed = StoredBacktestRun {
                    status: "failed".to_owned(),
                    result_json: json!({"error":"backtest worker exited unexpectedly"}).to_string(),
                    updated_at: timestamp.clone(),
                    ..run
                };
                let _ = worker.store.update_run_if_status(
                    &worker.run_id,
                    &expected,
                    failed,
                    &timestamp,
                );
            }
            false
        });
    }

    pub(crate) async fn shutdown(&self) {
        let workers = self.take_workers();
        for mut worker in workers {
            if let Some(cancel) = worker.cancel.take() {
                let _ = cancel.send(());
            }
            let _ = worker.handle.await;
        }
    }

    pub(crate) fn terminate(&self) {
        for mut worker in self.take_workers() {
            if let Some(cancel) = worker.cancel.take() {
                let _ = cancel.send(());
            }
            worker.handle.abort();
        }
    }

    fn take_workers(&self) -> Vec<Worker> {
        std::mem::take(
            &mut *self
                .workers
                .lock()
                .unwrap_or_else(|error| error.into_inner()),
        )
    }

    #[cfg(test)]
    pub(crate) fn worker_count(&self) -> usize {
        self.workers
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .len()
    }
}

static RUN_SEQUENCE: AtomicU64 = AtomicU64::new(1);

pub(crate) fn next_run_id() -> String {
    let now = time::OffsetDateTime::now_utc().unix_timestamp_nanos();
    format!("bt-{now}-{}", RUN_SEQUENCE.fetch_add(1, Ordering::Relaxed))
}

pub(crate) fn now_timestamp() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}

pub(crate) const EXECUTION_TIMEOUT: Duration = Duration::from_secs(15 * 60);
