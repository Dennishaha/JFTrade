use jftrade_store_sqlite::StoredBacktestSyncTask;
use serde_json::{Value, json};

use crate::product::BacktestSyncReadSnapshotError;

pub(crate) fn sync_task_projection(
    task: &StoredBacktestSyncTask,
) -> Result<Value, BacktestSyncReadSnapshotError> {
    if task.task_id.trim().is_empty() || task.status.trim().is_empty() {
        return Err(BacktestSyncReadSnapshotError::Unavailable(
            "stored sync task has invalid identity".to_owned(),
        ));
    }
    let mut value = json!({
        "completedBatches": task.completed_batches,
        "completedIntervals": task.completed_intervals,
        "currentInterval": task.current_interval,
        "marketDataProvider": task.market_data_provider,
        "retries": task.retries,
        "startedAt": task.started_at,
        "status": task.status,
        "symbol": task.symbol,
        "taskId": task.task_id,
        "totalBatches": task.total_batches,
        "totalIntervals": task.total_intervals,
        "updatedAt": task.updated_at,
    });
    if let Some(error) = &task.error {
        value["error"] = Value::String(error.clone());
    }
    Ok(value)
}
