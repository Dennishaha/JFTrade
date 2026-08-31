//! Durable task-state helpers shared by the candle sync worker.

use jftrade_store_sqlite::{BacktestSyncTaskStore, StoredBacktestSyncTask};

use super::super::product_backtest_sync_request::format_timestamp;

pub(super) fn is_cancelled(
    tasks: &BacktestSyncTaskStore,
    task_id: &str,
) -> Result<bool, String> {
    tasks
        .get(task_id)
        .map(|task| task.is_some_and(|task| task.status == "cancelled"))
        .map_err(|error| error.to_string())
}

pub(super) fn mark_task_cancelled(tasks: &BacktestSyncTaskStore, task_id: &str) {
    let timestamp = format_timestamp(time::OffsetDateTime::now_utc());
    if let Err(error) = tasks.cancel(task_id, &timestamp) {
        eprintln!("backtest sync {task_id} cancellation persistence failed: {error}");
    }
}

pub(super) fn persist_task(
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
