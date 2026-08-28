//! Backtests, Execution Orders, Brokers, and ADK production ports.

use std::collections::BTreeMap;
use std::sync::{Arc, Mutex};
use jftrade_store_sqlite::{BacktestMarketDataStore, BacktestRunStore, ExecutionOrderStore};
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
    pub(crate) _market_data_store: Arc<BacktestMarketDataStore>,
    pub(crate) sync_tasks: Mutex<BTreeMap<String, Value>>,
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
                let result = decode_json_field(&r.result_json, "backtest result")?;
                Ok(json!({
                    "id": r.id,
                    "status": r.status,
                    "request": request,
                    "result": result,
                    "createdAt": r.created_at,
                    "updatedAt": r.updated_at,
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
        run.map(|r| decode_json_field(&r.result_json, "backtest result"))
            .transpose()
    }
}

fn decode_json_field(raw: &str, field: &str) -> Result<Value, BacktestReadSnapshotError> {
    serde_json::from_str(raw).map_err(|error| {
        BacktestReadSnapshotError::Unavailable(format!("stored {field} is invalid JSON: {error}"))
    })
}

impl BacktestSyncReadSnapshotPort for ProductionBacktestPort {
    fn progress(&self, task_id: &str) -> Result<Option<Value>, BacktestSyncReadSnapshotError> {
        Ok(self
            .sync_tasks
            .lock()
            .map_err(|_| {
                BacktestSyncReadSnapshotError::Unavailable(
                    "backtest sync task registry lock is poisoned".to_owned(),
                )
            })?
            .get(task_id)
            .cloned())
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
            BacktestsWriteInput::Sync { .. } => {
                Err(BacktestsWritePortError::Unavailable(
                    "backtest market-data sync runtime is not configured".to_owned(),
                ))
            }
            BacktestsWriteInput::CancelSync { task_id } => {
                let mut tasks = self.sync_tasks.lock().map_err(|_| {
                    BacktestsWritePortError::Failed(
                        "backtest sync task registry lock is poisoned".to_owned(),
                    )
                })?;
                if tasks.is_empty() {
                    return Err(BacktestsWritePortError::Unavailable(
                        "backtest market-data sync runtime is not configured".to_owned(),
                    ));
                }
                Ok(BacktestsWritePortResult::SyncCancelled(
                    tasks.remove(task_id).is_some(),
                ))
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
