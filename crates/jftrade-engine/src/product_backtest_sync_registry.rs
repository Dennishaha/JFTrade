use std::sync::Mutex;

use jftrade_store_sqlite::BacktestSyncTaskStore;
use tokio::sync::oneshot;
use tokio::task::JoinHandle;

/// Rust-owned lifecycle for asynchronous historical sync workers.
#[derive(Default)]
pub(crate) struct BacktestSyncWorkerRegistry {
    workers: Mutex<Vec<Worker>>,
}

struct Worker {
    task_id: String,
    tasks: std::sync::Arc<BacktestSyncTaskStore>,
    handle: JoinHandle<()>,
    cancel: Option<oneshot::Sender<()>>,
}

impl BacktestSyncWorkerRegistry {
    pub(crate) fn register(
        &self,
        task_id: String,
        tasks: std::sync::Arc<BacktestSyncTaskStore>,
        handle: JoinHandle<()>,
        cancel: oneshot::Sender<()>,
    ) {
        self.reap_finished();
        self.workers
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .push(Worker {
                task_id,
                tasks,
                handle,
                cancel: Some(cancel),
            });
    }

    /// Remove completed worker handles. If a worker exited without reaching a
    /// terminal task state (for example after an unexpected panic), mark the
    /// durable task cancelled before dropping its handle so no running/queued
    /// record is left behind.
    pub(crate) fn reap_finished(&self) {
        let mut workers = self
            .workers
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        workers.retain(|worker| {
            if !worker.handle.is_finished() {
                return true;
            }
            let terminal = match worker.tasks.get(&worker.task_id) {
                Ok(Some(task)) => {
                    matches!(task.status.as_str(), "completed" | "failed" | "cancelled")
                }
                Ok(None) => true,
                Err(error) => {
                    eprintln!(
                        "backtest sync {} failed to inspect finished worker: {error}",
                        worker.task_id
                    );
                    return true;
                }
            };
            if !terminal {
                let timestamp = time::OffsetDateTime::now_utc()
                    .format(&time::format_description::well_known::Rfc3339)
                    .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned());
                if let Err(error) = worker.tasks.cancel(&worker.task_id, &timestamp) {
                    eprintln!(
                        "backtest sync {} failed to persist crash cancellation: {error}",
                        worker.task_id
                    );
                    return true;
                }
            }
            false
        });
    }

    pub(crate) async fn shutdown(&self) {
        let workers = self.take_workers();
        let mut workers = workers;
        let timestamp = time::OffsetDateTime::now_utc()
            .format(&time::format_description::well_known::Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned());
        for worker in &mut workers {
            // Persist cancellation before signalling the task. This closes
            // the race where HTTP shutdown tears down the Tokio runtime and
            // the worker can no longer run its cancellation branch.
            if let Err(error) = worker.tasks.cancel(&worker.task_id, &timestamp) {
                eprintln!(
                    "backtest sync {} shutdown cancellation persistence failed: {error}",
                    worker.task_id
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
            if let Some(cancel) = worker.cancel.take() {
                let _ = cancel.send(());
            }
            let timestamp = time::OffsetDateTime::now_utc()
                .format(&time::format_description::well_known::Rfc3339)
                .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned());
            if let Err(error) = worker.tasks.cancel(&worker.task_id, &timestamp) {
                eprintln!(
                    "backtest sync {} termination cancellation persistence failed: {error}",
                    worker.task_id
                );
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
    fn worker_count(&self) -> usize {
        self.workers
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .len()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_store_sqlite::{
        BACKTEST_RUNS_PRODUCTION_PROFILE, BacktestRunStore, BacktestSyncTaskStore,
        initialize_current,
    };
    use rusqlite::Connection;
    use tempfile::tempdir;

    fn task_store() -> (std::sync::Arc<BacktestSyncTaskStore>, tempfile::TempDir) {
        let directory = tempdir().expect("tempdir");
        let path = directory.path().join("backtest-runs.db");
        let connection = Connection::open(&path).expect("database");
        initialize_current(&connection, "backtest-runs").expect("schema");
        drop(connection);
        let runs = std::sync::Arc::new(
            BacktestRunStore::open_existing(&path, BACKTEST_RUNS_PRODUCTION_PROFILE)
                .expect("store"),
        );
        (
            std::sync::Arc::new(BacktestSyncTaskStore::new(runs)),
            directory,
        )
    }

    #[tokio::test]
    async fn reap_finished_removes_completed_join_handles() {
        let registry = BacktestSyncWorkerRegistry::default();
        let (tasks, _directory) = task_store();
        let (cancel, _receiver) = oneshot::channel();
        let handle = tokio::spawn(async {});
        registry.register("missing".to_owned(), tasks, handle, cancel);
        tokio::task::yield_now().await;
        registry.reap_finished();
        assert_eq!(registry.worker_count(), 0);
    }
}
