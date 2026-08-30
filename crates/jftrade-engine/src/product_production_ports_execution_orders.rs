//! Execution-order and broker write production adapters.

use std::collections::BTreeSet;
use std::sync::{Arc, Mutex};

use jftrade_integration_futu::{TradeModifyOrderRequest, TradeUnlockRequest, TradeWritePort};
use jftrade_store_sqlite::{
    ExecutionOrderReservation, ExecutionOrderStore, ExecutionOrderStoreError,
    StoredExecutionOrder, StoredExecutionOrderEvent, normalized_request_hash,
};
use jftrade_trading::{
    OrderStatus, canonical_broker_status, canonical_stored_status, reconcile_status,
};
use serde_json::{Value, json};

use super::super::product_production_ports_trade::SharedTradeReadRuntime;
use crate::product::product_brokers_write_port::{
    BrokersWriteInput, BrokersWriteOperation, BrokersWritePort, BrokersWritePortError,
};
use crate::product::product_execution_write_port::{
    ExecutionWriteInput, ExecutionWriteOperation, ExecutionWritePort, ExecutionWritePortError,
};
use crate::product::{ActiveProviderState, ExecutionReadSnapshotError, ExecutionReadSnapshotPort};
#[path = "product_production_ports_execution_order_helpers.rs"]
mod execution_order_helpers;
#[path = "product_production_ports_execution_order_hash.rs"]
mod execution_order_hash;
use execution_order_helpers::{
    CancelInFlightGuard, broker_error, broker_failed, execution_error_details, failed,
    header_from_order, is_terminal_status, map_trade_error, map_transition_store_error,
    merge_query, order_value, store_error, value_identifier,
};
use execution_order_hash::{canonical_execution_request, preview_request_hash};

#[path = "product_production_ports_execution_order_parse.rs"]
mod execution_order_parse;
use execution_order_parse::{
    new_order, parse_combo, parse_order, requires_locked_preview,
};
#[path = "product_production_ports_execution_order_previews.rs"]
mod execution_order_previews;

pub(crate) struct ProductionExecutionPort {
    pub(crate) store: Arc<ExecutionOrderStore>,
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) trade_logged_in: Option<bool>,
    pub(crate) trade_read_port: Option<Arc<dyn jftrade_integration_futu::TradeReadPort>>,
    pub(crate) trade_write_port: Option<Arc<dyn jftrade_integration_futu::TradeWritePort>>,
    pub(crate) trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
    pub(crate) cancel_inflight: Arc<Mutex<BTreeSet<String>>>,
}

impl std::fmt::Debug for ProductionExecutionPort {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ProductionExecutionPort")
            .field("has_trade_read_port", &self.trade_read_port.is_some())
            .field("has_trade_write_port", &self.trade_write_port.is_some())
            .finish_non_exhaustive()
    }
}

impl ExecutionReadSnapshotPort for ProductionExecutionPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, ExecutionReadSnapshotError> {
        if path == "/api/v1/execution/orders" {
            // A read is also the restart recovery rendezvous.  Reconcile
            // only durable candidates and keep the endpoint usable when
            // OpenD is unavailable; no broker result is synthesized.
            self.reconcile_pending_orders();
            let query = crate::product::product_query::QueryMap::parse(query).map_err(|_| {
                ExecutionReadSnapshotError::Invalid("invalid execution orders query".to_owned())
            })?;
            let scope_active = query
                .get_first("scope")
                .is_some_and(|scope| scope.trim().eq_ignore_ascii_case("ACTIVE"));
            let broker_id = query
                .get_first("brokerId")
                .map(str::trim)
                .filter(|v| !v.is_empty());
            let environment = query
                .get_first("tradingEnvironment")
                .map(str::trim)
                .filter(|v| !v.is_empty());
            let account_id = query
                .get_first("accountId")
                .map(str::trim)
                .filter(|v| !v.is_empty());
            let market = query
                .get_first("market")
                .map(str::trim)
                .filter(|v| !v.is_empty());
            let mut orders =
                self.store
                    .list_orders()
                    .map_err(|e| ExecutionReadSnapshotError::Failed {
                        code: "LIST_ORDERS_FAILED".to_owned(),
                        message: e.to_string(),
                    })?;
            orders.retain(|order| {
                (!scope_active || !is_terminal_status(&order.status))
                    && broker_id.is_none_or(|value| order.broker_id.eq_ignore_ascii_case(value))
                    && environment
                        .is_none_or(|value| order.trading_environment.eq_ignore_ascii_case(value))
                    && account_id.is_none_or(|value| order.account_id.trim() == value)
                    && market.is_none_or(|value| order.market.eq_ignore_ascii_case(value))
            });
            orders.sort_by(|left, right| {
                right
                    .updated_at
                    .cmp(&left.updated_at)
                    .then_with(|| right.created_at.cmp(&left.created_at))
                    .then_with(|| right.internal_order_id.cmp(&left.internal_order_id))
            });
            let items: Vec<Value> = orders
                .into_iter()
                .map(|o| {
                    order_value(&o).map_err(|error| match error {
                        ExecutionWritePortError::Failed { code, message, .. } => {
                            ExecutionReadSnapshotError::Failed { code, message }
                        }
                        ExecutionWritePortError::Unavailable(message) => {
                            ExecutionReadSnapshotError::Unavailable(message)
                        }
                    })
                })
                .collect::<Result<_, _>>()?;
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
                .map_err(|e| ExecutionReadSnapshotError::Failed {
                    code: "GET_ORDER_EVENTS_FAILED".to_owned(),
                    message: e.to_string(),
                })?
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
            let order =
                self.store
                    .get_order(id)
                    .map_err(|e| ExecutionReadSnapshotError::Failed {
                        code: "GET_ORDER_FAILED".to_owned(),
                        message: e.to_string(),
                    })?;
            if let Some(o) = order {
                let mut recent_events = self.store.list_order_events(id).map_err(|e| {
                    ExecutionReadSnapshotError::Failed {
                        code: "GET_ORDER_FAILED".to_owned(),
                        message: e.to_string(),
                    }
                })?;
                if recent_events.len() > 10 {
                    recent_events = recent_events.split_off(recent_events.len() - 10);
                }
                let order = order_value(&o).map_err(|error| match error {
                    ExecutionWritePortError::Failed { code, message, .. } => {
                        ExecutionReadSnapshotError::Failed { code, message }
                    }
                    ExecutionWritePortError::Unavailable(message) => {
                        ExecutionReadSnapshotError::Unavailable(message)
                    }
                })?;
                return Ok(json!({
                    "order": order,
                    "recentEvents": recent_events.into_iter().map(|event| json!({
                        "id": event.id,
                        "internalOrderId": event.internal_order_id,
                        "eventType": event.event_type,
                        "previousStatus": event.previous_status,
                        "nextStatus": event.next_status,
                        "payloadJson": event.payload_json,
                        "createdAt": event.created_at,
                    })).collect::<Vec<_>>(),
                    "checkedAt": crate::product::product_production_ports::provider_now_rfc3339(),
                }));
            }
            return Err(ExecutionReadSnapshotError::NotFound);
        }
        Err(ExecutionReadSnapshotError::NotFound)
    }
}

impl ExecutionWritePort for ProductionExecutionPort {
    fn mutate(&self, input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        match input.operation {
            ExecutionWriteOperation::OrderPlace => self.place_order(&input.payload),
            ExecutionWriteOperation::OrderCancel | ExecutionWriteOperation::ComboCancel => {
                let id = input
                    .internal_order_id
                    .as_deref()
                    .ok_or_else(|| failed(400, "BAD_REQUEST", "internalOrderId is required"))?;
                self.cancel_order(id)
            }
            ExecutionWriteOperation::ComboPlace => self.place_combo(&input.payload),
            ExecutionWriteOperation::BuyingPower => self.buying_power_preview(&input.payload),
            ExecutionWriteOperation::OrderPreview => self.order_preview(&input.payload),
            ExecutionWriteOperation::ComboPreview => self.combo_preview(&input.payload),
        }
    }
}

impl BrokersWritePort for ProductionExecutionPort {
    fn mutate(&self, input: &BrokersWriteInput) -> Result<Value, BrokersWritePortError> {
        if !input.query.broker_id.eq_ignore_ascii_case("futu") {
            return Err(BrokersWritePortError::Failed {
                status: 404,
                code: "BROKER_NOT_FOUND".to_owned(),
                message: "requested broker is not active".to_owned(),
            });
        }
        match input.operation {
            BrokersWriteOperation::PlaceOrder => self
                .place_order(&merge_query(&input.payload, &input.query))
                .map_err(broker_error),
            BrokersWriteOperation::CancelOrders => {
                let items = input
                    .payload
                    .get("orders")
                    .and_then(Value::as_array)
                    .ok_or_else(|| broker_failed(400, "BAD_REQUEST", "orders is required"))?;
                let mut cancelled = Vec::with_capacity(items.len());
                for item in items {
                    let id = self
                        .resolve_broker_cancel_target(item)
                        .map_err(broker_error)?;
                    cancelled.push(self.cancel_order(&id).map_err(broker_error)?);
                }
                Ok(json!({"cancelled": cancelled.len(), "orders": cancelled}))
            }
            BrokersWriteOperation::Unlock => {
                let object = input
                    .payload
                    .as_object()
                    .ok_or_else(|| broker_failed(400, "BAD_REQUEST", "invalid unlock payload"))?;
                let writer = self.writer().map_err(broker_error)?;
                writer
                    .unlock_trade(TradeUnlockRequest {
                        unlock: object
                            .get("unlock")
                            .and_then(Value::as_bool)
                            .unwrap_or(true),
                        password_md5: object
                            .get("passwordMd5")
                            .and_then(Value::as_str)
                            .map(str::to_owned),
                        security_firm: object
                            .get("securityFirm")
                            .and_then(Value::as_i64)
                            .map(|value| value as i32),
                    })
                    .map_err(|error| broker_error(map_trade_error(error)))?;
                Ok(json!({"unlocked": true, "brokerId": input.query.broker_id}))
            }
        }
    }
}

include!("product_production_ports_execution_orders_impl.rs");
fn replay_or_conflict(
    existing: StoredExecutionOrder,
    request_hash: &str,
) -> Result<Value, ExecutionWritePortError> {
    let Some(existing_hash) = normalized_request_hash(&existing) else {
        return Err(failed(
            409,
            "EXECUTION_ORDER_IDEMPOTENCY_CONFLICT",
            "clientOrderId is already reserved for a different request",
        ));
    };
    if existing_hash != request_hash {
        return Err(failed(
            409,
            "EXECUTION_ORDER_IDEMPOTENCY_CONFLICT",
            "clientOrderId is already reserved for a different request",
        ));
    }
    order_value(&existing)
}

fn map_reservation_error(error: ExecutionOrderStoreError) -> ExecutionWritePortError {
    match error {
        ExecutionOrderStoreError::Validation(message)
        | ExecutionOrderStoreError::NotFound(message) => {
            failed(400, "PREVIEW_INVALID", message)
        }
        ExecutionOrderStoreError::Conflict(message) => {
            failed(409, "EXECUTION_ORDER_IDEMPOTENCY_CONFLICT", message)
        }
        other => store_error(other),
    }
}

fn order_status_label(value: i32) -> &'static str {
    match value {
        -1 => "UNKNOWN",
        0 => "UNSUBMITTED",
        1 => "WAITING_SUBMIT",
        2 => "SUBMITTING",
        3 => "SUBMITFAILED",
        4 => "TIMEOUT",
        5 => "SUBMITTED",
        10 => "FILLED_PART",
        11 => "FILLED_ALL",
        12 | 13 => "CANCEL_REQUESTED",
        14 | 15 | 23 => "CANCELLED_ALL",
        21 | 22 | 24 => "FAILED",
        _ => "UNKNOWN",
    }
}

fn storage_status(status: OrderStatus, current: &str) -> String {
    match status {
        OrderStatus::Created => "CREATED",
        OrderStatus::Submitting | OrderStatus::SubmissionUnknown => "SUBMITTING",
        OrderStatus::Submitted | OrderStatus::BrokerAccepted => "SUBMITTED",
        OrderStatus::PartiallyFilled => "PARTIALLY_FILLED",
        OrderStatus::Filled => "FILLED",
        OrderStatus::CancelRequested => "CANCEL_SUBMITTED",
        OrderStatus::Cancelled => "CANCELLED",
        OrderStatus::Rejected => "FAILED",
        OrderStatus::Expired => "EXPIRED",
        OrderStatus::Unknown => current,
        OrderStatus::PrecheckRejected => "FAILED",
    }
    .to_owned()
}
