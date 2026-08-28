//! Backtests, Execution Orders, Brokers, and ADK production ports.

use std::sync::Arc;
use jftrade_store_sqlite::{
    BacktestMarketDataStore, BacktestRunStore, BacktestSyncTaskStore,
    CancelBacktestSyncResult, ExecutionOrderStore, StoredBacktestSyncTask,
};
use serde_json::{Value, json};
use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_backtests_write_port::{
    BacktestsWriteDeleteResult, BacktestsWriteInput, BacktestsWritePort, BacktestsWritePortError,
    BacktestsWritePortResult,
};
use crate::product::product_brokers_write_port::{
    BrokersWriteInput, BrokersWritePort, BrokersWritePortError,
};
use crate::product::product_execution_write_port::{
    ExecutionWriteInput, ExecutionWritePort, ExecutionWritePortError,
};
use crate::product::{
    BacktestReadSnapshotError,
    BacktestReadSnapshotPort, BacktestSyncReadSnapshotError, BacktestSyncReadSnapshotPort,
    BrokerReadSnapshotError, BrokerReadSnapshotPort, ExecutionReadSnapshotError,
    ExecutionReadSnapshotPort, PortfolioSnapshotError, PortfolioSnapshotPort,
};

// ---------------------------------------------------------------------------
// Backtest
// ---------------------------------------------------------------------------

#[derive(Debug)]
pub(crate) struct ProductionBacktestPort {
    pub(crate) store: Arc<BacktestRunStore>,
    pub(crate) sync_tasks: Arc<BacktestSyncTaskStore>,
    pub(crate) _market_data_store: Arc<BacktestMarketDataStore>,
}

impl BacktestReadSnapshotPort for ProductionBacktestPort {
    fn list(&self) -> Result<Value, BacktestReadSnapshotError> {
        let runs = self
            .store
            .list_runs()
            .map_err(|e| BacktestReadSnapshotError::Unavailable(e.to_string()))?;
        let items = runs
            .into_iter()
            .map(|r| {
                let request = decode_json_field(&r.request_json, "backtest request")?;
                // Go's ListLightweight validates the persisted result while
                // omitting the potentially large result payload from the
                // response.  Empty result_json is the normal representation
                // for queued/running runs and must not turn the whole list
                // into a store failure.
                let result = decode_optional_json_field(&r.result_json, "backtest result")?;
                let market_data_provider = result
                    .as_ref()
                    .and_then(|value| value.get("marketDataProvider"))
                    .and_then(Value::as_str)
                    .unwrap_or_default();
                Ok(json!({
                    "id": r.id,
                    "status": r.status,
                    "request": request,
                    "createdAt": r.created_at,
                    "updatedAt": r.updated_at,
                    "marketDataProvider": market_data_provider,
                }))
            })
            .collect::<Result<Vec<_>, BacktestReadSnapshotError>>()?;
        Ok(json!({ "runs": items }))
    }

    fn status(&self, run_id: &str) -> Result<Option<Value>, BacktestReadSnapshotError> {
        let run = self
            .store
            .get_run(run_id)
            .map_err(|e| BacktestReadSnapshotError::Unavailable(e.to_string()))?;
        Ok(run.map(|r| json!({
            "id": r.id,
            "status": r.status,
            "createdAt": r.created_at,
            "updatedAt": r.updated_at,
        })))
    }

    fn result(&self, run_id: &str) -> Result<Option<Value>, BacktestReadSnapshotError> {
        let run = self
            .store
            .get_run(run_id)
            .map_err(|e| BacktestReadSnapshotError::Unavailable(e.to_string()))?;
        run.map(|r| {
            let request = decode_json_field(&r.request_json, "backtest request")?;
            let result = decode_optional_json_field(&r.result_json, "backtest result")?;
            let market_data_provider = result
                .as_ref()
                .and_then(|value| value.get("marketDataProvider"))
                .and_then(Value::as_str)
                .unwrap_or_default();
            let mut response = json!({
                "id": r.id,
                "status": r.status,
                "request": request,
                "createdAt": r.created_at,
                "updatedAt": r.updated_at,
                "marketDataProvider": market_data_provider,
            });
            if let Some(result) = result {
                response["result"] = result;
            }
            Ok(response)
        })
        .transpose()
    }
}

fn decode_json_field(raw: &str, field: &str) -> Result<Value, BacktestReadSnapshotError> {
    serde_json::from_str(raw).map_err(|error| {
        BacktestReadSnapshotError::Unavailable(format!("stored {field} is invalid JSON: {error}"))
    })
}

fn decode_optional_json_field(
    raw: &str,
    field: &str,
) -> Result<Option<Value>, BacktestReadSnapshotError> {
    let trimmed = raw.trim();
    if trimmed.is_empty() || trimmed == "null" {
        return Ok(None);
    }
    decode_json_field(trimmed, field).map(Some)
}

impl BacktestSyncReadSnapshotPort for ProductionBacktestPort {
    fn progress(&self, task_id: &str) -> Result<Option<Value>, BacktestSyncReadSnapshotError> {
        self.sync_tasks
            .get(task_id)
            .map_err(|error| BacktestSyncReadSnapshotError::Unavailable(error.to_string()))?
            .map(|task| sync_task_projection(&task))
            .transpose()
    }
}

impl BacktestsWritePort for ProductionBacktestPort {
    fn mutate(&self, input: &BacktestsWriteInput) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        match input {
            BacktestsWriteInput::Start { .. } => {
                Err(BacktestsWritePortError::Unavailable(
                    "backtest worker runtime is not configured".to_owned(),
                ))
            }
            BacktestsWriteInput::Sync { payload } => {
                self.start_sync_task(payload)
            }
            BacktestsWriteInput::CancelSync { task_id } => {
                self.cancel_sync_task(task_id)
            }
            BacktestsWriteInput::Delete { run_id } => {
                match self.store.delete_run(run_id) {
                    Ok(true) => {
                        Ok(BacktestsWritePortResult::RunDeleted(BacktestsWriteDeleteResult::Deleted))
                    }
                    Ok(false) => {
                        Ok(BacktestsWritePortResult::RunDeleted(BacktestsWriteDeleteResult::Missing))
                    }
                    Err(jftrade_store_sqlite::BacktestRunStoreError::NotTerminal(_)) => {
                        Ok(BacktestsWritePortResult::RunDeleted(BacktestsWriteDeleteResult::NotTerminal))
                    }
                    Err(e) => Err(BacktestsWritePortError::Failed(e.to_string())),
                }
            }
        }
    }
}

impl ProductionBacktestPort {
    fn start_sync_task(
        &self,
        _payload: &Value,
    ) -> Result<BacktestsWritePortResult, BacktestsWritePortError> {
        // Persisting a queued task without an actual provider/sync executor
        // would be a false success. The production composition currently has
        // no Rust K-line sync runner, so keep the route reachable and report a
        // truthful baseline 503 until one is injected.
        Err(BacktestsWritePortError::Unavailable(
            "backtest market-data sync runtime is not configured".to_owned(),
        ))
    }

    fn cancel_sync_task(
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
            })?
        {
            CancelBacktestSyncResult::Cancelled => {
                Ok(BacktestsWritePortResult::SyncCancelled(true))
            }
            CancelBacktestSyncResult::Missing => {
                Ok(BacktestsWritePortResult::SyncCancelled(false))
            }
            // Go's CancelSync intentionally collapses a terminal task and an
            // unknown task into the same 404 response.
            CancelBacktestSyncResult::AlreadyTerminal => {
                Ok(BacktestsWritePortResult::SyncCancelled(false))
            }
        }
    }

}

#[cfg(test)]
mod sync_tests {
    use super::*;
    use jftrade_store_sqlite::{
        BACKTEST_RUNS_PRODUCTION_PROFILE, BacktestRunStore, initialize_current,
    };
    use rusqlite::Connection;
    use tempfile::tempdir;

    fn production_port() -> (ProductionBacktestPort, tempfile::TempDir) {
        let directory = tempdir().expect("temporary directory");
        let runs_path = directory.path().join("backtest-runs.db");
        let connection = Connection::open(&runs_path).expect("create runs database");
        initialize_current(&connection, "backtest-runs").expect("initialize runs database");
        drop(connection);
        let runs = Arc::new(
            BacktestRunStore::open_existing(&runs_path, BACKTEST_RUNS_PRODUCTION_PROFILE)
                .expect("open runs store"),
        );
        let sync_tasks = Arc::new(BacktestSyncTaskStore::new(Arc::clone(&runs)));
        let market_data_path = directory.path().join("backtest.db");
        let connection = Connection::open(&market_data_path).expect("create market database");
        initialize_current(&connection, "backtest").expect("initialize market database");
        drop(connection);
        let market_data = Arc::new(
            BacktestMarketDataStore::open_existing(
                &market_data_path,
                jftrade_store_sqlite::BACKTEST_MARKET_DATA_PRODUCTION_PROFILE,
            )
            .expect("open market store"),
        );
        (
            ProductionBacktestPort {
                store: runs,
                sync_tasks,
                _market_data_store: market_data,
            },
            directory,
        )
    }

    #[test]
    fn production_sync_read_projects_persisted_task() {
        let (port, _directory) = production_port();
        port.sync_tasks
            .create(StoredBacktestSyncTask {
                task_id: "sync-production".to_owned(),
                status: "running".to_owned(),
                symbol: "US.AAPL".to_owned(),
                market_data_provider: "yfinance".to_owned(),
                total_intervals: 2,
                completed_intervals: 1,
                total_batches: 2,
                completed_batches: 1,
                current_interval: "1d".to_owned(),
                retries: 0,
                error: None,
                started_at: "2026-08-29T00:00:00Z".to_owned(),
                updated_at: "2026-08-29T00:01:00Z".to_owned(),
                revision: 0,
            })
            .expect("persist task");
        let projected = port.progress("sync-production").expect("project task").unwrap();
        assert_eq!(projected["status"], "running");
        assert_eq!(projected["completedIntervals"], 1);
        assert!(port.progress("missing").expect("missing task").is_none());
    }

    #[test]
    fn production_sync_cancel_matches_not_found_for_terminal_task() {
        let (port, _directory) = production_port();
        port.sync_tasks
            .create(StoredBacktestSyncTask {
                task_id: "sync-terminal".to_owned(),
                status: "completed".to_owned(),
                symbol: "US.AAPL".to_owned(),
                market_data_provider: "yfinance".to_owned(),
                total_intervals: 0,
                completed_intervals: 0,
                total_batches: 0,
                completed_batches: 0,
                current_interval: String::new(),
                retries: 0,
                error: None,
                started_at: "2026-08-29T00:00:00Z".to_owned(),
                updated_at: "2026-08-29T00:00:00Z".to_owned(),
                revision: 0,
            })
            .expect("persist terminal task");
        let result = port.mutate(&BacktestsWriteInput::CancelSync {
            task_id: "sync-terminal".to_owned(),
        });
        assert_eq!(
            result,
            Ok(BacktestsWritePortResult::SyncCancelled(false))
        );
    }
}

fn sync_task_projection(task: &StoredBacktestSyncTask) -> Result<Value, BacktestSyncReadSnapshotError> {
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

// ---------------------------------------------------------------------------
// Execution Orders & Brokers
// ---------------------------------------------------------------------------

#[derive(Debug)]
pub(crate) struct ProductionExecutionPort {
    pub(crate) store: Arc<ExecutionOrderStore>,
}

impl ExecutionReadSnapshotPort for ProductionExecutionPort {
    fn read(&self, path: &str, _query: &str) -> Result<Value, ExecutionReadSnapshotError> {
        if path == "/api/v1/execution/orders" {
            let orders = self
                .store
                .list_orders()
                .map_err(|e| ExecutionReadSnapshotError::Unavailable(e.to_string()))?;
            let items: Vec<Value> = orders
                .into_iter()
                .map(|o| json!({
                    "internalOrderId": o.internal_order_id,
                    "brokerId": o.broker_id,
                    "brokerOrderId": o.broker_order_id,
                    "status": o.status,
                    "symbol": o.symbol,
                    "side": o.side,
                    "orderType": o.order_type,
                    "requestedQuantity": o.requested_quantity,
                    "requestedPrice": o.requested_price,
                    "filledQuantity": o.filled_quantity,
                    "filledAveragePrice": o.filled_average_price,
                    "createdAt": o.created_at,
                    "updatedAt": o.updated_at,
                }))
                .collect();
            return Ok(json!({ "orders": items }));
        }

        if let Some(id) = path
            .strip_prefix("/api/v1/execution/orders/")
            .and_then(|suffix| suffix.strip_suffix("/events"))
        {
            if id.is_empty() || id.contains('/') {
                return Err(ExecutionReadSnapshotError::NotFound);
            }
            let events = self
                .store
                .list_order_events(id)
                .map_err(|e| ExecutionReadSnapshotError::Unavailable(e.to_string()))?
                .into_iter()
                .map(|event| {
                    json!({
                        "id": event.id,
                        "internalOrderId": event.internal_order_id,
                        "eventType": event.event_type,
                        "previousStatus": event.previous_status,
                        "nextStatus": event.next_status,
                        "payloadJson": event.payload_json,
                        "createdAt": event.created_at,
                    })
                })
                .collect::<Vec<_>>();
            return Ok(json!({"internalOrderId": id, "events": events}));
        }

        if let Some(id) = path.strip_prefix("/api/v1/execution/orders/") {
            if id.is_empty() || id.contains('/') {
                return Err(ExecutionReadSnapshotError::NotFound);
            }
            let order = self
                .store
                .get_order(id)
                .map_err(|e| ExecutionReadSnapshotError::Unavailable(e.to_string()))?;
            if let Some(o) = order {
                return Ok(json!({
                    "internalOrderId": o.internal_order_id,
                    "brokerId": o.broker_id,
                    "brokerOrderId": o.broker_order_id,
                    "status": o.status,
                    "symbol": o.symbol,
                    "side": o.side,
                    "orderType": o.order_type,
                    "requestedQuantity": o.requested_quantity,
                    "requestedPrice": o.requested_price,
                    "filledQuantity": o.filled_quantity,
                    "filledAveragePrice": o.filled_average_price,
                    "createdAt": o.created_at,
                    "updatedAt": o.updated_at,
                }));
            }
            return Err(ExecutionReadSnapshotError::NotFound);
        }

        Err(ExecutionReadSnapshotError::NotFound)
    }
}

impl ExecutionWritePort for ProductionExecutionPort {
    fn mutate(&self, input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        let _ = input;
        Err(ExecutionWritePortError::Unavailable(
            "execution broker/OpenD runtime is not configured".to_owned(),
        ))
    }
}

impl BrokersWritePort for ProductionExecutionPort {
    fn mutate(&self, input: &BrokersWriteInput) -> Result<Value, BrokersWritePortError> {
        let _ = input;
        Err(BrokersWritePortError::Unavailable(
            "broker/OpenD runtime is not configured".to_owned(),
        ))
    }
}

// ---------------------------------------------------------------------------
// Broker & Portfolio
// ---------------------------------------------------------------------------

#[derive(Debug)]
pub(crate) struct ProductionBrokerPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
}

impl BrokerReadSnapshotPort for ProductionBrokerPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, BrokerReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(BrokerReadSnapshotError::Unavailable(
                "broker integration is not enabled".to_owned(),
            ));
        }
        Err(BrokerReadSnapshotError::Unavailable(
            "broker integration is not enabled".to_owned(),
        ))
    }
}

#[derive(Debug)]
pub(crate) struct ProductionPortfolioPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) _execution_store: Arc<ExecutionOrderStore>,
}

impl PortfolioSnapshotPort for ProductionPortfolioPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, PortfolioSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(PortfolioSnapshotError::Unavailable(
                "portfolio provider is not configured".to_owned(),
            ));
        }
        Err(PortfolioSnapshotError::Unavailable(
            "portfolio provider is not configured".to_owned(),
        ))
    }
}
