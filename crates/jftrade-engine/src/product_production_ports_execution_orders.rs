//! Execution-order and broker write production adapters.

use std::sync::Arc;

use jftrade_store_sqlite::ExecutionOrderStore;
use serde_json::{json, Value};

use crate::product::product_brokers_write_port::{BrokersWriteInput, BrokersWritePort, BrokersWritePortError};
use crate::product::product_execution_write_port::{ExecutionWriteInput, ExecutionWritePort, ExecutionWritePortError};
use crate::product::{ExecutionReadSnapshotError, ExecutionReadSnapshotPort};

#[derive(Debug)]
pub(crate) struct ProductionExecutionPort {
    pub(crate) store: Arc<ExecutionOrderStore>,
}

impl ExecutionReadSnapshotPort for ProductionExecutionPort {
    fn read(&self, path: &str, _query: &str) -> Result<Value, ExecutionReadSnapshotError> {
        if path == "/api/v1/execution/orders" {
            let orders = self.store.list_orders().map_err(|e| ExecutionReadSnapshotError::Unavailable(e.to_string()))?;
            let items: Vec<Value> = orders.into_iter().map(|o| json!({
                "internalOrderId": o.internal_order_id, "brokerId": o.broker_id,
                "brokerOrderId": o.broker_order_id, "status": o.status, "symbol": o.symbol,
                "side": o.side, "orderType": o.order_type, "requestedQuantity": o.requested_quantity,
                "requestedPrice": o.requested_price, "filledQuantity": o.filled_quantity,
                "filledAveragePrice": o.filled_average_price, "createdAt": o.created_at,
                "updatedAt": o.updated_at,
            })).collect();
            return Ok(json!({ "orders": items }));
        }
        if let Some(id) = path.strip_prefix("/api/v1/execution/orders/").and_then(|suffix| suffix.strip_suffix("/events")) {
            if id.is_empty() || id.contains('/') { return Err(ExecutionReadSnapshotError::NotFound); }
            let events = self.store.list_order_events(id).map_err(|e| ExecutionReadSnapshotError::Unavailable(e.to_string()))?.into_iter().map(|event| json!({
                "id": event.id, "internalOrderId": event.internal_order_id, "eventType": event.event_type,
                "previousStatus": event.previous_status, "nextStatus": event.next_status,
                "payloadJson": event.payload_json, "createdAt": event.created_at,
            })).collect::<Vec<_>>();
            return Ok(json!({"internalOrderId": id, "events": events}));
        }
        if let Some(id) = path.strip_prefix("/api/v1/execution/orders/") {
            if id.is_empty() || id.contains('/') { return Err(ExecutionReadSnapshotError::NotFound); }
            let order = self.store.get_order(id).map_err(|e| ExecutionReadSnapshotError::Unavailable(e.to_string()))?;
            if let Some(o) = order {
                return Ok(json!({
                    "internalOrderId": o.internal_order_id, "brokerId": o.broker_id,
                    "brokerOrderId": o.broker_order_id, "status": o.status, "symbol": o.symbol,
                    "side": o.side, "orderType": o.order_type, "requestedQuantity": o.requested_quantity,
                    "requestedPrice": o.requested_price, "filledQuantity": o.filled_quantity,
                    "filledAveragePrice": o.filled_average_price, "createdAt": o.created_at,
                    "updatedAt": o.updated_at,
                }));
            }
            return Err(ExecutionReadSnapshotError::NotFound);
        }
        Err(ExecutionReadSnapshotError::NotFound)
    }
}

impl ExecutionWritePort for ProductionExecutionPort {
    fn mutate(&self, _input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        Err(ExecutionWritePortError::Unavailable("execution broker/OpenD runtime is not configured".to_owned()))
    }
}

impl BrokersWritePort for ProductionExecutionPort {
    fn mutate(&self, _input: &BrokersWriteInput) -> Result<Value, BrokersWritePortError> {
        Err(BrokersWritePortError::Unavailable("broker/OpenD runtime is not configured".to_owned()))
    }
}
