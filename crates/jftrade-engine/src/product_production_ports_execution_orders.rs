//! Execution-order and broker write production adapters.

use std::collections::BTreeSet;
use std::sync::{Arc, Mutex};

use jftrade_integration_futu::{TradeModifyOrderRequest, TradeUnlockRequest, TradeWritePort};
use jftrade_store_sqlite::{ExecutionOrderStore, StoredExecutionOrder, StoredExecutionOrderEvent};
use serde_json::{Value, json};

use crate::product::product_brokers_write_port::{BrokersWriteInput, BrokersWriteOperation, BrokersWritePort, BrokersWritePortError};
use crate::product::product_execution_write_port::{ExecutionWriteInput, ExecutionWriteOperation, ExecutionWritePort, ExecutionWritePortError};
use super::super::product_production_ports_trade::SharedTradeReadRuntime;
use crate::product::{ActiveProviderState, ExecutionReadSnapshotError, ExecutionReadSnapshotPort};
#[path = "product_production_ports_execution_order_helpers.rs"]
mod execution_order_helpers;
use execution_order_helpers::{CancelInFlightGuard, broker_error, broker_failed, execution_error_details, failed, header_from_order, is_terminal_status, map_trade_error, map_transition_store_error, merge_query, order_value, parse_preview_order, preview_request_hash, store_error, value_identifier};

#[path = "product_production_ports_execution_order_parse.rs"]
mod execution_order_parse;
use execution_order_parse::{
    ParsedOrder, new_order, parse_combo, parse_order, requires_locked_preview,
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
            let query = crate::product::product_query::QueryMap::parse(query).map_err(|_| {
                ExecutionReadSnapshotError::Invalid("invalid execution orders query".to_owned())
            })?;
            let scope_active = query
                .get_first("scope")
                .is_some_and(|scope| scope.trim().eq_ignore_ascii_case("ACTIVE"));
            let broker_id = query.get_first("brokerId").map(str::trim).filter(|v| !v.is_empty());
            let environment = query.get_first("tradingEnvironment").map(str::trim).filter(|v| !v.is_empty());
            let account_id = query.get_first("accountId").map(str::trim).filter(|v| !v.is_empty());
            let market = query.get_first("market").map(str::trim).filter(|v| !v.is_empty());
            let mut orders = self.store.list_orders().map_err(|e| {
                ExecutionReadSnapshotError::Failed { code: "LIST_ORDERS_FAILED".to_owned(), message: e.to_string() }
            })?;
            orders.retain(|order| {
                (!scope_active || !is_terminal_status(&order.status))
                    && broker_id.is_none_or(|value| order.broker_id.eq_ignore_ascii_case(value))
                    && environment.is_none_or(|value| order.trading_environment.eq_ignore_ascii_case(value))
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
                .map_err(|e| ExecutionReadSnapshotError::Failed { code: "GET_ORDER_EVENTS_FAILED".to_owned(), message: e.to_string() })?
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
                .map_err(|e| ExecutionReadSnapshotError::Failed { code: "GET_ORDER_FAILED".to_owned(), message: e.to_string() })?;
            if let Some(o) = order {
                let mut recent_events = self.store.list_order_events(id).map_err(|e| {
                    ExecutionReadSnapshotError::Failed { code: "GET_ORDER_FAILED".to_owned(), message: e.to_string() }
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
                    let id = self.resolve_broker_cancel_target(item).map_err(broker_error)?;
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

impl ProductionExecutionPort {
    fn writer(&self) -> Result<Arc<dyn TradeWritePort>, ExecutionWritePortError> {
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
        let trade_logged_in = self
            .trade_runtime
            .as_ref()
            .map_or(self.trade_logged_in, |runtime| runtime.snapshot().trade_logged_in);
        if trade_logged_in != Some(true) {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu trade account is not logged in".to_owned(),
            ));
        }
        if let Some(runtime) = self.trade_runtime.as_ref() {
            return runtime.writer_snapshot().ok_or_else(|| {
                ExecutionWritePortError::Unavailable("OpenD trade runtime is unavailable".to_owned())
            });
        }
        self.trade_write_port.clone().ok_or_else(|| {
            ExecutionWritePortError::Unavailable("OpenD trade runtime is unavailable".to_owned())
        })
    }

    fn order_preview(&self, payload: &Value) -> Result<Value, ExecutionWritePortError> {
        let object = payload.as_object().ok_or_else(|| {
            failed(400, "BAD_REQUEST", "invalid execution order payload")
        })?;
        let parsed = parse_preview_order(object)?;
        if requires_locked_preview(&parsed) && parsed.client_order_id.is_none() {
            return Err(failed(
                400,
                "BAD_REQUEST",
                "clientOrderId is required for derivative and event-contract previews",
            ));
        }
        // Validate the request before checking external readiness so malformed
        // payloads retain the baseline 400 response even when OpenD is down.
        self.ensure_futu_runtime()?;
        // Input validation remains local so malformed requests preserve the
        // baseline 400 response. A successful preview requires a concrete
        // ProductRule/OpenD adapter, which is not installed in this build.
        let _ = parsed;
        Err(ExecutionWritePortError::Unavailable(
            "Futu product-rule adapter is unavailable".to_owned(),
        ))
    }

    fn consume_preview(
        &self,
        preview_id: &str,
        parsed: &ParsedOrder,
        payload: &Value,
    ) -> Result<(), ExecutionWritePortError> {
        let timestamp = crate::product::product_production_ports::provider_now_rfc3339();
        let request_hash = preview_request_hash(payload, parsed, None)?;
        self.store
            .consume_preview(
                preview_id,
                &parsed.broker_id,
                &parsed.header.acc_id.to_string(),
                &request_hash,
                &timestamp,
            )
            .map_err(|error| failed(400, "PREVIEW_INVALID", error.to_string()))
    }

    fn place_order(&self, payload: &Value) -> Result<Value, ExecutionWritePortError> {
        let parsed = parse_order(payload).map_err(|message| failed(400, "BAD_REQUEST", message))?;
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        if let Some(existing) = self.find_client_order(parsed.client_order_id.as_deref())? {
            return order_value(&existing);
        }
        if requires_locked_preview(&parsed) {
            if parsed.preview_id.is_none() {
                return Err(failed(
                    400,
                    "BAD_REQUEST",
                    "previewId is required for derivative and event-contract orders",
                ));
            }
            if parsed.client_order_id.is_none() {
                return Err(failed(
                    400,
                    "BAD_REQUEST",
                    "clientOrderId is required for idempotent derivative and event-contract submission",
                ));
            }
        } else if parsed.preview_id.is_some() && parsed.client_order_id.is_none() {
            return Err(failed(
                400,
                "BAD_REQUEST",
                "clientOrderId is required when previewId is supplied",
            ));
        }
        if let Some(preview_id) = parsed.preview_id.as_deref() {
            self.consume_preview(preview_id, &parsed, payload)?;
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
        self.persist_transition(
            &order,
            "submitted",
            Some(&previous_status),
            &now,
            &now,
        )?;
        order_value(&order)
    }

    fn place_combo(&self, payload: &Value) -> Result<Value, ExecutionWritePortError> {
        let parsed = parse_combo(payload).map_err(|message| failed(400, "BAD_REQUEST", message))?;
        if parsed.order.preview_id.is_none() {
            return Err(failed(
                400,
                "BAD_REQUEST",
                "previewId is required for combo orders",
            ));
        }
        let writer = self.writer()?;
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        if let Some(preview_id) = parsed.order.preview_id.as_deref() {
            self.consume_preview(preview_id, &parsed.order, payload)?;
        }
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
        self.persist_transition(
            &order,
            "submitted",
            Some(&previous_status),
            &now,
            &now,
        )?;
        order_value(&order)
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
        let _guard = CancelInFlightGuard::acquire(Arc::clone(&self.cancel_inflight), internal_id)?;
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
        if order.status.eq_ignore_ascii_case("CANCEL_SUBMITTED") {
            return Err(failed(
                409,
                "EXECUTION_ORDER_CANCEL_IN_PROGRESS",
                "execution order cancellation is already in progress",
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
        let expected_updated_at = order.updated_at.clone();
        let modify_result = writer.modify_order(TradeModifyOrderRequest {
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
            });
        if let Err(error) = modify_result {
            let mapped = map_trade_error(error);
            let now = crate::product::product_production_ports::provider_now_rfc3339();
            self.persist_failure(
                &mut order,
                &mapped,
                "cancel_failed",
                &previous_status,
                &expected_updated_at,
                &now,
            )?;
            return Err(mapped);
        }
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        order.status = "CANCEL_SUBMITTED".to_owned();
        order.updated_at = now.clone();
        self.persist_transition(
            &order,
            "cancel_submitted",
            Some(&previous_status),
            &expected_updated_at,
            &now,
        )?;
        order_value(&order)
    }

    /// Resolve the public broker cancellation item to the durable local order
    /// identity.  The broker API accepts the numeric `orderId` plus optional
    /// broker/external identifiers; treating those fields as strings (or as a
    /// local id) silently cancels the wrong order or reports a false 400.
    fn resolve_broker_cancel_target(&self, item: &Value) -> Result<String, ExecutionWritePortError> {
        let object = item.as_object().ok_or_else(|| {
            failed(400, "BAD_REQUEST", "each order cancellation item must be an object")
        })?;
        let internal_id = value_identifier(object.get("internalOrderId"));
        let broker_id = value_identifier(object.get("orderId"));
        let broker_order_id = value_identifier(object.get("brokerOrderId"));
        let broker_order_id_ex = value_identifier(object.get("brokerOrderIdEx"));
        if internal_id.is_none()
            && broker_id.is_none()
            && broker_order_id.is_none()
            && broker_order_id_ex.is_none()
        {
            return Err(failed(
                400,
                "BAD_REQUEST",
                "orderId, brokerOrderId, or internalOrderId is required",
            ));
        }
        let symbol = value_identifier(object.get("symbol"));
        let orders = self.store.list_orders().map_err(store_error)?;
        let mut matches = orders
            .iter()
            .filter(|order| {
                internal_id
                    .as_deref()
                    .is_some_and(|id| order.internal_order_id == id)
                    || broker_id
                        .as_deref()
                        .is_some_and(|id| order.broker_order_id.as_deref() == Some(id))
                    || broker_order_id
                        .as_deref()
                        .is_some_and(|id| order.broker_order_id.as_deref() == Some(id))
                    || broker_order_id_ex
                        .as_deref()
                        .is_some_and(|id| order.broker_order_id_ex.as_deref() == Some(id))
            })
            .collect::<Vec<_>>();
        if let Some(symbol) = symbol.as_deref() {
            let narrowed = matches
                .iter()
                .copied()
                .filter(|order| order.symbol.as_deref().is_some_and(|value| value == symbol))
                .collect::<Vec<_>>();
            if !narrowed.is_empty() {
                matches = narrowed;
            }
        }
        match matches.as_slice() {
            [order] => Ok(order.internal_order_id.clone()),
            [] => Err(failed(
                404,
                "EXECUTION_ORDER_NOT_FOUND",
                "execution order not found for supplied broker order identity",
            )),
            _ => Err(failed(
                409,
                "EXECUTION_ORDER_AMBIGUOUS",
                "supplied broker order identity matches multiple execution orders",
            )),
        }
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
        expected_updated_at: &str,
        timestamp: &str,
    ) -> Result<(), ExecutionWritePortError> {
        let expected_status = previous_status.ok_or_else(|| {
            failed(
                500,
                "EXECUTION_STATE_ERROR",
                "state transitions require a previous status",
            )
        })?;
        self.persist_transition_with_payload(
            order,
            event_type,
            expected_status,
            expected_updated_at,
            timestamp,
            &order.normalized_request,
        )
    }

    fn persist_transition_with_payload(
        &self,
        order: &StoredExecutionOrder,
        event_type: &str,
        expected_status: &str,
        expected_updated_at: &str,
        timestamp: &str,
        payload_json: &str,
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
            previous_status: Some(expected_status),
            next_status: &order.status,
            payload_json,
            created_at: timestamp,
        };
        self.store
            .transition_order_and_event(
                order.clone(),
                timestamp,
                &event,
                expected_status,
                expected_updated_at,
            )
            .map(|_| ())
            .map_err(map_transition_store_error)
    }

    fn persist_unknown(
        &self,
        order: &mut StoredExecutionOrder,
        error: &ExecutionWritePortError,
        event_type: &str,
        timestamp: &str,
    ) -> Result<(), ExecutionWritePortError> {
        let previous_status = order.status.clone();
        let expected_updated_at = order.updated_at.clone();
        order.status = "UNKNOWN".to_owned();
        let (message, code) = execution_error_details(error);
        order.last_error = Some(message);
        order.last_error_code = code;
        order.last_error_source = Some("opend".to_owned());
        order.updated_at = timestamp.to_owned();
        self.persist_transition(
            order,
            event_type,
            Some(&previous_status),
            &expected_updated_at,
            timestamp,
        )
    }

    fn persist_failure(
        &self,
        order: &mut StoredExecutionOrder,
        error: &ExecutionWritePortError,
        event_type: &str,
        previous_status: &str,
        expected_updated_at: &str,
        timestamp: &str,
    ) -> Result<(), ExecutionWritePortError> {
        let (message, code) = execution_error_details(error);
        order.last_error = Some(message);
        order.last_error_code = code;
        order.last_error_source = Some("opend".to_owned());
        order.updated_at = timestamp.to_owned();
        let payload_json = json!({
            "error": order.last_error.clone(),
            "code": order.last_error_code.clone(),
            "source": order.last_error_source.clone(),
        })
        .to_string();
        self.persist_transition_with_payload(
            order,
            event_type,
            previous_status,
            expected_updated_at,
            timestamp,
            &payload_json,
        )
    }
}
