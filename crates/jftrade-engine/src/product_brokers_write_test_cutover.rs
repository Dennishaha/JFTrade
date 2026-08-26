// Durable broker mutation test-cutover adapter backed by `jftrade-store-sqlite`.
//
// This code is included only by Rust tests. Its SQLite schema connects to
// the real `execution-orders` component with schema validation and single-writer lease.

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};

use jftrade_store_sqlite::{
    EXECUTION_ORDERS_TEST_CUTOVER_PROFILE, ExecutionOrderTestCutoverStore, StoredExecutionOrder,
    StoredExecutionOrderEvent,
};
use serde::{Deserialize, Serialize};
use serde_json::{Value, json};
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use super::{
    BrokersWriteContext, BrokersWriteInput, BrokersWriteOperation, BrokersWritePort,
    BrokersWritePortError,
};

#[derive(Default, Serialize, Deserialize)]
struct BrokerCompanionState {
    sessions: BTreeMap<String, bool>,
}

pub struct BrokersWriteSqliteTestCutoverPort {
    path: PathBuf,
    store: Arc<ExecutionOrderTestCutoverStore>,
    companion_path: PathBuf,
    state: Mutex<BrokerCompanionState>,
    reject_next_event: Mutex<bool>,
}

impl std::fmt::Debug for BrokersWriteSqliteTestCutoverPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("BrokersWriteSqliteTestCutoverPort")
            .field("path", &self.path)
            .finish()
    }
}

#[allow(dead_code)]
impl BrokersWriteSqliteTestCutoverPort {
    pub fn open(path: impl AsRef<Path>) -> Result<Self, String> {
        let path = path.as_ref().to_owned();
        let store = ExecutionOrderTestCutoverStore::open_existing(
            &path,
            EXECUTION_ORDERS_TEST_CUTOVER_PROFILE,
        )
        .map_err(|err| err.to_string())?;
        let companion_path = path.with_extension("broker_sessions.json");
        let state = if companion_path.exists() {
            let bytes = std::fs::read(&companion_path).map_err(|e| e.to_string())?;
            serde_json::from_slice(&bytes).unwrap_or_default()
        } else {
            BrokerCompanionState::default()
        };
        Ok(Self {
            path,
            store: Arc::new(store),
            companion_path,
            state: Mutex::new(state),
            reject_next_event: Mutex::new(false),
        })
    }

    pub fn path(&self) -> &Path {
        &self.path
    }

    pub fn store(&self) -> &ExecutionOrderTestCutoverStore {
        &self.store
    }

    fn persist_companion(&self, state: &BrokerCompanionState) -> Result<(), String> {
        let bytes = serde_json::to_vec_pretty(state).map_err(|e| e.to_string())?;
        std::fs::write(&self.companion_path, bytes).map_err(|e| e.to_string())?;
        Ok(())
    }

    pub fn order_count(&self) -> Result<u64, String> {
        self.store.order_count().map_err(|e| e.to_string())
    }

    pub fn order_status(&self, order_id: i64) -> Result<Option<String>, String> {
        let order = self
            .store
            .get_order(&order_id.to_string())
            .map_err(|e| e.to_string())?;
        Ok(order.map(|o| o.status))
    }

    pub fn event_count(&self, operation: &str) -> Result<u64, String> {
        self.store.event_count(operation).map_err(|e| e.to_string())
    }

    pub fn session_unlocked(&self, broker_id: &str) -> Result<Option<bool>, String> {
        let state = self.state.lock().map_err(|_| "poisoned".to_owned())?;
        Ok(state.sessions.get(broker_id).copied())
    }

    pub fn reject_next_event(&self) -> Result<(), String> {
        let mut reject = self
            .reject_next_event
            .lock()
            .map_err(|_| "poisoned".to_owned())?;
        *reject = true;
        Ok(())
    }

    fn take_event_rejection(&self) -> Result<bool, BrokersWritePortError> {
        let mut reject = self
            .reject_next_event
            .lock()
            .map_err(|_| failed(500, "poisoned"))?;
        let value = *reject;
        *reject = false;
        Ok(value)
    }
}

impl BrokersWritePort for BrokersWriteSqliteTestCutoverPort {
    fn mutate(&self, input: &BrokersWriteInput) -> Result<Value, BrokersWritePortError> {
        reject_non_normal_context(input.context)?;
        let reject_event = self.take_event_rejection()?;
        if reject_event {
            return Err(failed(500, "test-cutover broker event rejected"));
        }
        let timestamp = now_rfc3339();

        match input.operation {
            BrokersWriteOperation::PlaceOrder => {
                let seq = self
                    .store
                    .next_sequence("broker_orders")
                    .map_err(|e| failed(500, &e.to_string()))?;
                let id = seq.to_string();
                let order = StoredExecutionOrder {
                    internal_order_id: id.clone(),
                    broker_id: input.query.broker_id.clone(),
                    broker_order_id: Some(id.clone()),
                    broker_order_id_ex: None,
                    source: "broker".to_owned(),
                    source_detail: "".to_owned(),
                    trading_environment: input.query.trading_environment.clone(),
                    account_id: input.query.account_id.clone(),
                    market: input.query.market.clone(),
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
                    event_type: "place-order",
                    previous_status: None,
                    next_status: "submitted",
                    payload_json: &input.payload.to_string(),
                    created_at: &timestamp,
                };
                self.store
                    .record_event(&event)
                    .map_err(|e| failed(500, &e.to_string()))?;
                Ok(json!({
                    "orderId": seq,
                    "status": "submitted",
                }))
            }
            BrokersWriteOperation::CancelOrders => {
                let orders = input.payload["orders"].as_array().cloned().unwrap_or_default();
                let mut cancelled = 0;
                for item in orders {
                    let order_id = item["orderId"].as_i64().unwrap_or(0);
                    let id = order_id.to_string();
                    if let Ok(true) = self.store.cancel_order(&id, &timestamp) {
                        let evt_id = format!("evt_cancel_{timestamp}_{order_id}");
                        let event = StoredExecutionOrderEvent {
                            id: &evt_id,
                            internal_order_id: &id,
                            event_type: "cancel-orders",
                            previous_status: Some("submitted"),
                            next_status: "cancelled",
                            payload_json: "{}",
                            created_at: &timestamp,
                        };
                        let _ = self.store.record_event(&event);
                        cancelled += 1;
                    }
                }
                Ok(json!({
                    "cancelled": cancelled,
                }))
            }
            BrokersWriteOperation::Unlock => {
                let mut state = self.state.lock().map_err(|_| failed(500, "poisoned"))?;
                state.sessions.insert(input.query.broker_id.clone(), true);
                self.persist_companion(&state).map_err(|e| failed(500, &e.to_string()))?;
                let evt_id = format!("evt_unlock_{timestamp}");
                let event = StoredExecutionOrderEvent {
                    id: &evt_id,
                    internal_order_id: &input.query.broker_id,
                    event_type: "unlock",
                    previous_status: None,
                    next_status: "unlocked",
                    payload_json: "{}",
                    created_at: &timestamp,
                };
                let _ = self.store.record_event(&event);
                Ok(json!({
                    "unlocked": true,
                }))
            }
        }
    }
}

fn reject_non_normal_context(context: BrokersWriteContext) -> Result<(), BrokersWritePortError> {
    match context {
        BrokersWriteContext::Normal => Ok(()),
        BrokersWriteContext::Canceled => Err(failed(499, "request canceled")),
        BrokersWriteContext::Deadline => Err(failed(504, "request timed out")),
    }
}

fn failed(status: u16, message: &str) -> BrokersWritePortError {
    BrokersWritePortError::Failed {
        status,
        code: "BROKER_FAILED".to_owned(),
        message: message.to_owned(),
    }
}

fn now_rfc3339() -> String {
    OffsetDateTime::now_utc()
        .format(&Rfc3339)
        .unwrap_or_else(|_| "2026-08-26T00:00:00Z".to_owned())
}
