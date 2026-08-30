//! Execution-order and broker write production adapters.

use std::sync::Arc;

use jftrade_integration_futu::{
    TradeModifyOrderRequest, TradeSessionError, TradeUnlockRequest, TradeWritePort,
};
use jftrade_store_sqlite::{ExecutionOrderStore, StoredExecutionOrder, StoredExecutionOrderEvent};
use serde_json::{Value, json};

use crate::product::product_brokers_write_port::{
    BrokersWriteInput, BrokersWriteOperation, BrokersWritePort, BrokersWritePortError,
};
use crate::product::product_execution_write_port::{
    ExecutionWriteInput, ExecutionWriteOperation, ExecutionWritePort, ExecutionWritePortError,
};
use crate::product::{ActiveProviderState, ExecutionReadSnapshotError, ExecutionReadSnapshotPort};

#[path = "product_production_ports_execution_order_parse.rs"]
mod execution_order_parse;
use execution_order_parse::{new_order, parse_combo, parse_order, trade_market};

pub(crate) struct ProductionExecutionPort {
    pub(crate) store: Arc<ExecutionOrderStore>,
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) trade_logged_in: Option<bool>,
    pub(crate) trade_read_port: Option<Arc<dyn jftrade_integration_futu::TradeReadPort>>,
    pub(crate) trade_write_port: Option<Arc<dyn jftrade_integration_futu::TradeWritePort>>,
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
    fn read(&self, path: &str, _query: &str) -> Result<Value, ExecutionReadSnapshotError> {
        if path == "/api/v1/execution/orders" {
            let orders = self
                .store
                .list_orders()
                .map_err(|e| ExecutionReadSnapshotError::Unavailable(e.to_string()))?;
            let items: Vec<Value> = orders
                .into_iter()
                .map(|o| {
                    json!({
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
                    })
                })
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
            ExecutionWriteOperation::BuyingPower
            | ExecutionWriteOperation::ComboPreview
            | ExecutionWriteOperation::OrderPreview => Err(ExecutionWritePortError::Unavailable(
                "execution preview requires broker product-rule adapter".to_owned(),
            )),
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
                    let id = item
                        .get("internalOrderId")
                        .or_else(|| item.get("orderId"))
                        .and_then(Value::as_str)
                        .ok_or_else(|| {
                            broker_failed(400, "BAD_REQUEST", "internalOrderId is required")
                        })?;
                    cancelled.push(self.cancel_order(id).map_err(broker_error)?);
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

impl ProductionExecutionPort {
    fn writer(&self) -> Result<&dyn TradeWritePort, ExecutionWritePortError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider != Some(jftrade_settings::MarketDataProvider::Futu) {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu is not the active market-data provider".to_owned(),
            ));
        }
        if !snapshot.opend_ready {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu OpenD runtime is not ready".to_owned(),
            ));
        }
        if self.trade_logged_in != Some(true) {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu trade account is not logged in".to_owned(),
            ));
        }
        self.trade_write_port.as_deref().ok_or_else(|| {
            ExecutionWritePortError::Unavailable("OpenD trade runtime is unavailable".to_owned())
        })
    }

    fn place_order(&self, payload: &Value) -> Result<Value, ExecutionWritePortError> {
        let parsed = parse_order(payload).map_err(|message| failed(400, "BAD_REQUEST", message))?;
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        if let Some(existing) = self.find_client_order(parsed.client_order_id.as_deref())? {
            return Ok(order_value(&existing));
        }
        let writer = self.writer()?;
        let sequence = self
            .store
            .next_sequence("internal-order")
            .map_err(store_error)?;
        let internal_id = format!("rust-order-{sequence}");
        let mut order = new_order(&internal_id, &parsed, &now);
        order.normalized_request = payload.to_string();
        self.store
            .save_order(order.clone(), &now)
            .map_err(store_error)?;
        let result = match writer.place_order(parsed.to_trade_request()) {
            Ok(result) => result,
            Err(error) => {
                let mapped = map_trade_error(error);
                self.persist_unknown(&mut order, &mapped, "submission_failed", &now)?;
                return Err(mapped);
            }
        };
        if result.order_id.is_none() && result.order_id_ex.as_deref().is_none_or(str::is_empty) {
            let error = failed(
                502,
                "BROKER_INVALID_RESPONSE",
                "OpenD response did not include an order id",
            );
            self.persist_unknown(&mut order, &error, "submission_failed", &now)?;
            return Err(error);
        }
        let previous_status = order.status.clone();
        order.status = "SUBMITTED".to_owned();
        order.broker_order_id = result.order_id.map(|id| id.to_string());
        order.broker_order_id_ex = result.order_id_ex;
        order.submitted_at = Some(now.clone());
        order.updated_at = now.clone();
        self.persist_transition(&order, "submitted", Some(&previous_status), &now)?;
        Ok(order_value(&order))
    }

    fn place_combo(&self, payload: &Value) -> Result<Value, ExecutionWritePortError> {
        let parsed = parse_combo(payload).map_err(|message| failed(400, "BAD_REQUEST", message))?;
        let writer = self.writer()?;
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        let sequence = self
            .store
            .next_sequence("internal-combo-order")
            .map_err(store_error)?;
        let internal_id = format!("rust-combo-{sequence}");
        let mut order = new_order(&internal_id, &parsed.order, &now);
        order.order_kind = "combo".to_owned();
        order.normalized_request = payload.to_string();
        self.store
            .save_order(order.clone(), &now)
            .map_err(store_error)?;
        let result = match writer.place_combo_order(parsed.to_trade_request()) {
            Ok(result) => result,
            Err(error) => {
                let mapped = map_trade_error(error);
                self.persist_unknown(&mut order, &mapped, "submission_failed", &now)?;
                return Err(mapped);
            }
        };
        let Some(order_id_ex) = result.order_id_ex.filter(|value| !value.trim().is_empty()) else {
            let error = failed(
                502,
                "BROKER_INVALID_RESPONSE",
                "OpenD combo response did not include orderIDEx",
            );
            self.persist_unknown(&mut order, &error, "submission_failed", &now)?;
            return Err(error);
        };
        let previous_status = order.status.clone();
        order.status = "SUBMITTED".to_owned();
        order.broker_order_id_ex = Some(order_id_ex);
        order.submitted_at = Some(now.clone());
        order.updated_at = now.clone();
        self.persist_transition(&order, "submitted", Some(&previous_status), &now)?;
        Ok(order_value(&order))
    }

    fn cancel_order(&self, internal_id: &str) -> Result<Value, ExecutionWritePortError> {
        let mut order = self
            .store
            .get_order(internal_id)
            .map_err(store_error)?
            .ok_or_else(|| {
                failed(
                    404,
                    "EXECUTION_ORDER_NOT_FOUND",
                    "execution order not found",
                )
            })?;
        if matches!(
            order.status.to_ascii_uppercase().as_str(),
            "FILLED" | "CANCELLED" | "FAILED" | "UNKNOWN"
        ) {
            return Err(failed(
                400,
                "EXECUTION_ORDER_TERMINAL",
                "execution order is already terminal",
            ));
        }
        let broker_order_id = order
            .broker_order_id
            .as_deref()
            .and_then(|value| value.parse::<u64>().ok())
            .unwrap_or(0);
        if broker_order_id == 0
            && order
                .broker_order_id_ex
                .as_deref()
                .is_none_or(|value| value.trim().is_empty())
        {
            return Err(failed(
                400,
                "BROKER_ORDER_ID_MISSING",
                "execution order has no broker order id or orderIDEx",
            ));
        }
        let writer = self.writer()?;
        let previous_status = order.status.clone();
        writer
            .modify_order(TradeModifyOrderRequest {
                header: header_from_order(&order)?,
                order_id: broker_order_id,
                operation: 2,
                for_all: None,
                trd_market: None,
                quantity: None,
                price: None,
                adjust_price: None,
                adjust_side_and_limit: None,
                aux_price: None,
                trail_type: None,
                trail_value: None,
                trail_spread: None,
                order_id_ex: order.broker_order_id_ex.clone(),
            })
            .map_err(map_trade_error)?;
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        order.status = "CANCEL_SUBMITTED".to_owned();
        order.updated_at = now.clone();
        self.persist_transition(&order, "cancel_submitted", Some(&previous_status), &now)?;
        Ok(order_value(&order))
    }

    fn find_client_order(
        &self,
        id: Option<&str>,
    ) -> Result<Option<StoredExecutionOrder>, ExecutionWritePortError> {
        let Some(id) = id.filter(|value| !value.trim().is_empty()) else {
            return Ok(None);
        };
        self.store.list_orders().map_err(store_error).map(|orders| {
            orders
                .into_iter()
                .find(|order| order.client_order_id.as_deref() == Some(id))
        })
    }

    fn persist_transition(
        &self,
        order: &StoredExecutionOrder,
        event_type: &str,
        previous_status: Option<&str>,
        timestamp: &str,
    ) -> Result<(), ExecutionWritePortError> {
        let event_id = format!(
            "{}-{event_type}-{}",
            order.internal_order_id,
            self.store
                .next_sequence("order-event")
                .map_err(store_error)?
        );
        let event = StoredExecutionOrderEvent {
            id: &event_id,
            internal_order_id: &order.internal_order_id,
            event_type,
            previous_status,
            next_status: &order.status,
            payload_json: &order.normalized_request,
            created_at: timestamp,
        };
        self.store
            .save_order_and_event(order.clone(), timestamp, &event)
            .map(|_| ())
            .map_err(store_error)
    }

    fn persist_unknown(
        &self,
        order: &mut StoredExecutionOrder,
        error: &ExecutionWritePortError,
        event_type: &str,
        timestamp: &str,
    ) -> Result<(), ExecutionWritePortError> {
        let previous_status = order.status.clone();
        order.status = "UNKNOWN".to_owned();
        let (message, code) = execution_error_details(error);
        order.last_error = Some(message);
        order.last_error_code = code;
        order.last_error_source = Some("opend".to_owned());
        order.updated_at = timestamp.to_owned();
        self.persist_transition(order, event_type, Some(&previous_status), timestamp)
    }
}

fn header_from_order(
    order: &StoredExecutionOrder,
) -> Result<jftrade_integration_futu::TradeHeader, ExecutionWritePortError> {
    let account_id = order
        .account_id
        .parse::<u64>()
        .map_err(|_| failed(400, "BAD_REQUEST", "stored accountId is not numeric"))?;
    Ok(jftrade_integration_futu::TradeHeader {
        trd_env: i32::from(order.trading_environment.eq_ignore_ascii_case("REAL")),
        acc_id: account_id,
        trd_market: trade_market(&order.market),
        jp_acc_type: None,
    })
}

fn merge_query(
    payload: &Value,
    query: &crate::product::product_brokers_write_port::BrokersWriteQuery,
) -> Value {
    let mut object = payload.as_object().cloned().unwrap_or_default();
    object
        .entry("brokerId".to_owned())
        .or_insert_with(|| Value::String(query.broker_id.clone()));
    object
        .entry("accountId".to_owned())
        .or_insert_with(|| Value::String(query.account_id.clone()));
    object
        .entry("tradingEnvironment".to_owned())
        .or_insert_with(|| Value::String(query.trading_environment.clone()));
    object
        .entry("market".to_owned())
        .or_insert_with(|| Value::String(query.market.clone()));
    Value::Object(object)
}

fn order_value(order: &StoredExecutionOrder) -> Value {
    json!({
        "internalOrderId": order.internal_order_id,
        "brokerId": order.broker_id,
        "brokerOrderId": order.broker_order_id,
        "brokerOrderIdEx": order.broker_order_id_ex,
        "status": order.status,
        "symbol": order.symbol,
        "side": order.side,
        "orderType": order.order_type,
        "requestedQuantity": order.requested_quantity,
        "requestedPrice": order.requested_price,
        "filledQuantity": order.filled_quantity,
        "filledAveragePrice": order.filled_average_price,
        "createdAt": order.created_at,
        "updatedAt": order.updated_at,
    })
}
fn store_error(error: impl std::fmt::Display) -> ExecutionWritePortError {
    failed(500, "EXECUTION_STORE_ERROR", error.to_string())
}

fn failed(
    status: u16,
    code: impl Into<String>,
    message: impl Into<String>,
) -> ExecutionWritePortError {
    ExecutionWritePortError::Failed {
        status,
        code: code.into(),
        message: message.into(),
    }
}

fn broker_failed(
    status: u16,
    code: impl Into<String>,
    message: impl Into<String>,
) -> BrokersWritePortError {
    BrokersWritePortError::Failed {
        status,
        code: code.into(),
        message: message.into(),
    }
}

fn broker_error(error: ExecutionWritePortError) -> BrokersWritePortError {
    match error {
        ExecutionWritePortError::Unavailable(message) => {
            BrokersWritePortError::Unavailable(message)
        }
        ExecutionWritePortError::Failed {
            status,
            code,
            message,
        } => BrokersWritePortError::Failed {
            status,
            code,
            message,
        },
    }
}

fn execution_error_details(error: &ExecutionWritePortError) -> (String, Option<String>) {
    match error {
        ExecutionWritePortError::Unavailable(message) => (message.clone(), None),
        ExecutionWritePortError::Failed {
            code, message, ..
        } => (message.clone(), Some(code.clone())),
    }
}

fn map_trade_error(error: TradeSessionError) -> ExecutionWritePortError {
    let message = error.to_string();
    let lower = message.to_ascii_lowercase();
    if lower.contains("timeout") || lower.contains("timed out") {
        failed(504, "BROKER_TIMEOUT", message)
    } else if lower.contains("rate") || lower.contains("quota") {
        failed(429, "BROKER_RATE_LIMITED", message)
    } else {
        failed(502, "BROKER_UNAVAILABLE", message)
    }
}
