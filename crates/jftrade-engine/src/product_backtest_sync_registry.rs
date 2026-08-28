use std::sync::Mutex;

use tokio::sync::oneshot;
use tokio::task::JoinHandle;
use jftrade_store_sqlite::BacktestSyncTaskStore;

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
        self.workers
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .push(Worker { task_id, tasks, handle, cancel: Some(cancel) });
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
            let _ = worker.tasks.cancel(&worker.task_id, &timestamp);
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
            let _ = worker.tasks.cancel(&worker.task_id, &timestamp);
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

}
