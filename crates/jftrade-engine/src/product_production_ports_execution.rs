//! Backtests, Execution Orders, Brokers, and ADK production ports.

use std::sync::Arc;
use jftrade_integration_marketdata_helper::HelperClient;
use jftrade_store_sqlite::{
    BacktestMarketDataStore, BacktestRunStore, BacktestSyncTaskStore, StrategyDefinitionStore,
};
use serde_json::{Value, json};
use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::product_backtest_execution::{
    BacktestExecutionPort, BacktestExecutionTaskRegistry,
};
use super::BacktestSyncWorkerRegistry;
use crate::product::product_backtests_write_port::{
    BacktestsWriteDeleteResult, BacktestsWriteInput, BacktestsWritePort, BacktestsWritePortError,
    BacktestsWritePortResult,
};
use crate::product::{
    BacktestReadSnapshotError,
    BacktestReadSnapshotPort, BacktestSyncReadSnapshotError, BacktestSyncReadSnapshotPort,
};

#[path = "product_backtest_sync_request.rs"]
mod product_backtest_sync_request;
#[path = "product_production_ports_backtest_parse.rs"]
mod product_production_ports_backtest_parse;
#[path = "product_production_ports_backtest_sync_projection.rs"]
mod product_production_ports_backtest_sync_projection;
use product_production_ports_backtest_sync_projection::sync_task_projection;
#[path = "product_production_ports_execution_orders.rs"]
mod product_production_ports_execution_orders;
pub(crate) use product_production_ports_execution_orders::ProductionExecutionPort;

#[path = "product_production_ports_backtest_task.rs"]
mod product_production_ports_backtest_task;
#[path = "product_production_ports_backtest_sync.rs"]
mod product_production_ports_backtest_sync;
#[path = "product_production_ports_backtest_strategy.rs"]
mod product_production_ports_backtest_strategy;

pub(crate) struct ProductionBacktestPort {
    pub(crate) store: Arc<BacktestRunStore>,
    pub(crate) sync_tasks: Arc<BacktestSyncTaskStore>,
    pub(crate) _market_data_store: Arc<BacktestMarketDataStore>,
    pub(crate) helper: Option<HelperClient>,
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) sync_workers: Arc<BacktestSyncWorkerRegistry>,
    pub(crate) execution: Option<Arc<dyn BacktestExecutionPort>>,
    pub(crate) execution_workers: Arc<BacktestExecutionTaskRegistry>,
    pub(crate) strategy_definitions: Arc<StrategyDefinitionStore>,
}

impl std::fmt::Debug for ProductionBacktestPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionBacktestPort")
            .field("has_helper", &self.helper.is_some())
            .field("has_execution", &self.execution.is_some())
            .finish_non_exhaustive()
    }
}

impl BacktestReadSnapshotPort for ProductionBacktestPort {
    fn list(&self) -> Result<Value, BacktestReadSnapshotError> {
        self.execution_workers.reap_finished();
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
        self.execution_workers.reap_finished();
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
        self.execution_workers.reap_finished();
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
        self.sync_workers.reap_finished();
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
            BacktestsWriteInput::Start { payload } => self.start_backtest(payload),
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

#[cfg(test)]
#[path = "product_backtest_sync_start_tests.rs"]
mod sync_start_tests;
