// Durable execution test-cutover adapter backed by `jftrade-store-sqlite`.
//
// This code is included only by Rust tests. Its SQLite schema connects to
// the real `execution-orders` component with schema validation and single-writer lease.

use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};

use jftrade_store_sqlite::{
    EXECUTION_ORDERS_TEST_CUTOVER_PROFILE, ExecutionOrderTestCutoverStore, StoredExecutionOrder,
    StoredExecutionOrderEvent,
};
use serde_json::{Value, json};
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use super::{
    ExecutionWriteContext, ExecutionWriteInput, ExecutionWriteOperation, ExecutionWritePort,
    ExecutionWritePortError,
};

pub struct ExecutionSqliteTestCutoverPort {
    path: PathBuf,
    store: Arc<ExecutionOrderTestCutoverStore>,
    reject_order_place: Mutex<bool>,
}

impl std::fmt::Debug for ExecutionSqliteTestCutoverPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ExecutionSqliteTestCutoverPort")
            .field("path", &self.path)
            .finish()
    }
}

#[allow(dead_code)]
impl ExecutionSqliteTestCutoverPort {
    pub fn open(path: impl AsRef<Path>) -> Result<Self, String> {
        let path = path.as_ref().to_owned();
        let store = ExecutionOrderTestCutoverStore::open_existing(
            &path,
            EXECUTION_ORDERS_TEST_CUTOVER_PROFILE,
        )
        .map_err(|err| err.to_string())?;
        Ok(Self {
            path,
            store: Arc::new(store),
            reject_order_place: Mutex::new(false),
        })
    }

    pub fn path(&self) -> &Path {
        &self.path
    }

    pub fn store(&self) -> &ExecutionOrderTestCutoverStore {
        &self.store
    }

    pub fn order_count(&self) -> Result<u64, String> {
        self.store.order_count().map_err(|e| e.to_string())
    }

    pub fn order_status(&self, id: &str) -> Result<Option<String>, String> {
        let order = self.store.get_order(id).map_err(|e| e.to_string())?;
        Ok(order.map(|o| o.status))
    }

    pub fn event_count(&self, operation: &str) -> Result<u64, String> {
        self.store.event_count(operation).map_err(|e| e.to_string())
    }

    pub fn reject_order_place_event(&self) -> Result<(), String> {
        let mut reject = self
            .reject_order_place
            .lock()
            .map_err(|_| "poisoned".to_owned())?;
        *reject = true;
        Ok(())
    }

    pub fn clear_rejection(&self) -> Result<(), String> {
        let mut reject = self
            .reject_order_place
            .lock()
            .map_err(|_| "poisoned".to_owned())?;
        *reject = false;
        Ok(())
    }
}

impl ExecutionWritePort for ExecutionSqliteTestCutoverPort {
    fn mutate(&self, input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        reject_non_normal_context(input.context)?;
        let timestamp = now_rfc3339();

        match input.operation {
            ExecutionWriteOperation::OrderPlace => {
                let should_reject = *self
                    .reject_order_place
                    .lock()
                    .map_err(|_| failed(500, "lock poisoned"))?;
                if should_reject {
                    return Err(failed(500, "test-cutover order-place rejection"));
                }
                let seq = self
                    .store
                    .next_sequence("execution_orders")
                    .map_err(|e| failed(500, &e.to_string()))?;
                let id = format!("order-test-{seq}");
                let order = StoredExecutionOrder {
                    internal_order_id: id.clone(),
                    broker_id: input.payload["brokerId"].as_str().unwrap_or("").to_owned(),
                    broker_order_id: Some(id.clone()),
                    broker_order_id_ex: None,
                    source: "execution".to_owned(),
                    source_detail: "".to_owned(),
                    trading_environment: "simulated".to_owned(),
                    account_id: "".to_owned(),
                    market: "".to_owned(),
                    symbol: input.payload["symbol"].as_str().map(ToOwned::to_owned),
                    side: input.payload["side"].as_str().map(ToOwned::to_owned),
                    order_type: input.payload["orderType"].as_str().map(ToOwned::to_owned),
                    status: "submitted".to_owned(),
                    raw_broker_status: None,
                    requested_quantity: input.payload["quantity"].as_f64(),
                    requested_price: input.payload["price"].as_f64(),
                    filled_quantity: Some(0.0),
                    filled_average_price: Some(0.0),
                    remark: None,
                    last_error: None,
                    last_error_code: None,
                    last_error_source: None,
                    submitted_at: Some(timestamp.clone()),
                    updated_at: timestamp.clone(),
                    created_at: timestamp.clone(),
                    order_kind: "single".to_owned(),
                    product_class: "stock".to_owned(),
                    quantity_mode: "units".to_owned(),
                    client_order_id: None,
                    preview_id: None,
                    normalized_request: input.payload.to_string(),
                    requested_amount: None,
                    payout: None,
                    fees: None,
                };
                self.store
                    .save_order(order, &timestamp)
                    .map_err(|e| failed(500, &e.to_string()))?;
                let evt_id = format!("evt_{timestamp}_{seq}");
                let event = StoredExecutionOrderEvent {
                    id: &evt_id,
                    internal_order_id: &id,
                    event_type: "order-place",
                    previous_status: None,
                    next_status: "submitted",
                    payload_json: &input.payload.to_string(),
                    created_at: &timestamp,
                };
                self.store
                    .record_event(&event)
                    .map_err(|e| failed(500, &e.to_string()))?;
                Ok(json!({
                    "internalOrderId": id,
                    "status": "submitted",
                }))
            }
            ExecutionWriteOperation::ComboPlace => {
                let seq = self
                    .store
                    .next_sequence("execution_orders")
                    .map_err(|e| failed(500, &e.to_string()))?;
                let id = format!("combo-test-{seq}");
                let order = StoredExecutionOrder {
                    internal_order_id: id.clone(),
                    broker_id: input.payload["brokerId"].as_str().unwrap_or("").to_owned(),
                    broker_order_id: Some(id.clone()),
                    broker_order_id_ex: None,
                    source: "execution".to_owned(),
                    source_detail: "".to_owned(),
                    trading_environment: "simulated".to_owned(),
                    account_id: "".to_owned(),
                    market: "".to_owned(),
                    symbol: input.payload["symbol"].as_str().map(ToOwned::to_owned),
                    side: input.payload["side"].as_str().map(ToOwned::to_owned),
                    order_type: input.payload["orderType"].as_str().map(ToOwned::to_owned),
                    status: "submitted".to_owned(),
                    raw_broker_status: None,
                    requested_quantity: input.payload["quantity"].as_f64(),
                    requested_price: input.payload["price"].as_f64(),
                    filled_quantity: Some(0.0),
                    filled_average_price: Some(0.0),
                    remark: None,
                    last_error: None,
                    last_error_code: None,
                    last_error_source: None,
                    submitted_at: Some(timestamp.clone()),
                    updated_at: timestamp.clone(),
                    created_at: timestamp.clone(),
                    order_kind: "combo".to_owned(),
                    product_class: "combo".to_owned(),
                    quantity_mode: "units".to_owned(),
                    client_order_id: None,
                    preview_id: None,
                    normalized_request: input.payload.to_string(),
                    requested_amount: None,
                    payout: None,
                    fees: None,
                };
                self.store
                    .save_order(order, &timestamp)
                    .map_err(|e| failed(500, &e.to_string()))?;
                let evt_id = format!("evt_{timestamp}_{seq}");
                let event = StoredExecutionOrderEvent {
                    id: &evt_id,
                    internal_order_id: &id,
                    event_type: "combo-place",
                    previous_status: None,
                    next_status: "submitted",
                    payload_json: &input.payload.to_string(),
                    created_at: &timestamp,
                };
                self.store
                    .record_event(&event)
                    .map_err(|e| failed(500, &e.to_string()))?;
                Ok(json!({
                    "internalOrderId": id,
                    "status": "submitted",
                }))
            }
            ExecutionWriteOperation::OrderCancel | ExecutionWriteOperation::ComboCancel => {
                let id = input
                    .internal_order_id
                    .as_deref()
                    .ok_or_else(|| failed(400, "missing internalOrderId"))?;
                let mut transitioned = false;
                if let Ok(true) = self.store.cancel_order(id, &timestamp) {
                    let evt_id = format!("evt_cancel_{timestamp}_{id}");
                    let event = StoredExecutionOrderEvent {
                        id: &evt_id,
                        internal_order_id: id,
                        event_type: "order-cancel",
                        previous_status: Some("submitted"),
                        next_status: "cancelled",
                        payload_json: "{}",
                        created_at: &timestamp,
                    };
                    let _ = self.store.record_event(&event);
                    transitioned = true;
                }
                Ok(json!({
                    "internalOrderId": id,
                    "transitioned": transitioned,
                }))
            }
            ExecutionWriteOperation::BuyingPower
            | ExecutionWriteOperation::ComboPreview
            | ExecutionWriteOperation::OrderPreview => Ok(json!({
                "durableMutation": false,
            })),
        }
    }
}

fn reject_non_normal_context(context: ExecutionWriteContext) -> Result<(), ExecutionWritePortError> {
    match context {
        ExecutionWriteContext::Normal => Ok(()),
        ExecutionWriteContext::Canceled => Err(failed(499, "request canceled")),
        ExecutionWriteContext::Deadline => Err(failed(504, "request timed out")),
    }
}

fn failed(status: u16, message: &str) -> ExecutionWritePortError {
    ExecutionWritePortError::Failed {
        status,
        code: "EXECUTION_FAILED".to_owned(),
        message: message.to_owned(),
    }
}

fn now_rfc3339() -> String {
    OffsetDateTime::now_utc()
        .format(&Rfc3339)
        .unwrap_or_else(|_| "2026-08-26T00:00:00Z".to_owned())
}
