//! Rust-owned asynchronous backtest execution boundary.
//!
//! The HTTP adapter deliberately knows nothing about PineTS, strategy
//! definitions, or the matching engine. Those concerns are supplied through
//! the worker-owned [`BacktestExecutionPort`] adapter. This keeps the default
//! production composition fail-closed while allowing fixture/worker
//! compositions to opt into deterministic execution.

use std::sync::{
    Arc, Mutex,
    atomic::{AtomicU64, Ordering},
};
use std::time::Duration;

use jftrade_store_sqlite::{BacktestRunStore, StoredBacktestRun};
use serde_json::json;
use tokio::sync::oneshot;
use tokio::task::JoinHandle;

pub use jftrade_integration_pine::{
    BacktestExecutionCandle, BacktestExecutionError, BacktestExecutionPort,
    BacktestExecutionRequest, RunJsonBacktestExecutionPort,
};

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
        let (store, cancel) = {
            let mut workers = self
                .workers
                .lock()
                .unwrap_or_else(|error| error.into_inner());
            let Some(worker) = workers.iter_mut().find(|worker| worker.run_id == run_id) else {
                return false;
            };
            let Some(cancel) = worker.cancel.take() else {
                return false;
            };
            (Arc::clone(&worker.store), cancel)
        };
        if let Err(error) = mark_run_cancelled(&store, run_id) {
            eprintln!("backtest {run_id} cancellation persistence failed: {error}");
        }
        let _ = cancel.send(());
        true
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
            let run = match worker.store.get_run(&worker.run_id) {
                Ok(run) => run,
                Err(error) => {
                    eprintln!(
                        "backtest {} failed to inspect finished worker: {error}",
                        worker.run_id
                    );
                    return true;
                }
            };
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
                match worker.store.update_run_if_status(
                    &worker.run_id,
                    &expected,
                    failed,
                    &timestamp,
                ) {
                    Ok(true) => {}
                    Ok(false) => eprintln!(
                        "backtest {} was changed before crash recovery",
                        worker.run_id
                    ),
                    Err(error) => {
                        eprintln!(
                            "backtest {} failed to persist crash recovery: {error}",
                            worker.run_id
                        );
                        return true;
                    }
                }
            }
            false
        });
    }

    pub(crate) async fn shutdown(&self) {
        let mut workers = self.take_workers();
        for worker in &mut workers {
            if let Err(error) = mark_run_cancelled(&worker.store, &worker.run_id) {
                eprintln!(
                    "backtest {} shutdown cancellation persistence failed: {error}",
                    worker.run_id
                );
            }
            if let Some(cancel) = worker.cancel.take() {
                let _ = cancel.send(());
            }
        }
        for worker in workers {
            let _ = worker.handle.await;
        }
    }

    pub(crate) fn terminate(&self) {
        for mut worker in self.take_workers() {
            if let Err(error) = mark_run_cancelled(&worker.store, &worker.run_id) {
                eprintln!(
                    "backtest {} termination cancellation persistence failed: {error}",
                    worker.run_id
                );
            }
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

fn mark_run_cancelled(store: &BacktestRunStore, run_id: &str) -> Result<(), String> {
    let run = store
        .get_run(run_id)
        .map_err(|error| error.to_string())?
        .ok_or_else(|| format!("backtest run {run_id} not found"))?;
    if !matches!(run.status.as_str(), "queued" | "running") {
        return Ok(());
    }
    let timestamp = now_timestamp();
    let expected = run.status.clone();
    let cancelled = StoredBacktestRun {
        status: "cancelled".to_owned(),
        result_json: json!({"error": "backtest cancelled"}).to_string(),
        updated_at: timestamp.clone(),
        ..run
    };
    match store.update_run_if_status(run_id, &expected, cancelled, &timestamp) {
        Ok(true) => Ok(()),
        Ok(false) => Err(format!(
            "backtest run {run_id} changed before cancellation was persisted"
        )),
        Err(error) => Err(error.to_string()),
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

#[cfg(test)]
mod tests {
    use super::*;
    use rusqlite::Connection;
    use tempfile::TempDir;

    fn run_store() -> (Arc<BacktestRunStore>, TempDir) {
        let directory = tempfile::tempdir().expect("temporary directory");
        let path = directory.path().join("backtest-runs.db");
        let connection = Connection::open(&path).expect("create runs database");
        jftrade_store_sqlite::initialize_current(&connection, "backtest-runs")
            .expect("initialize runs database");
        drop(connection);
        let store = BacktestRunStore::open_existing(
            &path,
            jftrade_store_sqlite::BACKTEST_RUNS_PRODUCTION_PROFILE,
        )
        .expect("open runs store");
        (Arc::new(store), directory)
    }

    fn seed_run(store: &BacktestRunStore, run_id: &str, status: &str) {
        let timestamp = "2026-08-29T00:00:00Z";
        store
            .save_run(
                StoredBacktestRun {
                    id: run_id.to_owned(),
                    status: status.to_owned(),
                    request_json: "{}".to_owned(),
                    result_json: String::new(),
                    created_at: timestamp.to_owned(),
                    updated_at: timestamp.to_owned(),
                },
                timestamp,
            )
            .expect("seed run");
    }

    #[tokio::test]
    async fn reap_finished_marks_nonterminal_worker_failed() {
        let (store, _directory) = run_store();
        seed_run(&store, "run-crash", "running");
        let registry = BacktestExecutionTaskRegistry::default();
        let (cancel, _receiver) = oneshot::channel();
        let handle = tokio::spawn(async {});
        registry.register("run-crash".to_owned(), Arc::clone(&store), handle, cancel);
        tokio::task::yield_now().await;
        registry.reap_finished();

        let run = store.get_run("run-crash").expect("load run").expect("run");
        assert_eq!(run.status, "failed");
        assert!(run.result_json.contains("unexpectedly"));
        assert_eq!(registry.worker_count(), 0);
    }

    #[tokio::test]
    async fn cancel_persists_cancelled_state_before_signalling_worker() {
        let (store, _directory) = run_store();
        seed_run(&store, "run-cancel", "queued");
        let registry = BacktestExecutionTaskRegistry::default();
        let (cancel, receiver) = oneshot::channel();
        let handle = tokio::spawn(async move {
            let _ = receiver.await;
        });
        registry.register("run-cancel".to_owned(), Arc::clone(&store), handle, cancel);

        assert!(registry.cancel("run-cancel"));
        let run = store.get_run("run-cancel").expect("load run").expect("run");
        assert_eq!(run.status, "cancelled");
        registry.shutdown().await;
        assert_eq!(registry.worker_count(), 0);
    }

    #[tokio::test]
    async fn shutdown_persists_cancelled_state_before_joining_workers() {
        let (store, _directory) = run_store();
        seed_run(&store, "run-shutdown", "running");
        let registry = BacktestExecutionTaskRegistry::default();
        let (cancel, receiver) = oneshot::channel();
        let handle = tokio::spawn(async move {
            let _ = receiver.await;
        });
        registry.register(
            "run-shutdown".to_owned(),
            Arc::clone(&store),
            handle,
            cancel,
        );

        registry.shutdown().await;
        let run = store
            .get_run("run-shutdown")
            .expect("load run")
            .expect("run");
        assert_eq!(run.status, "cancelled");
        assert_eq!(registry.worker_count(), 0);
    }
}
