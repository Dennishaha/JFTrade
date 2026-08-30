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

impl ProductionExecutionPort {
    fn reconcile_pending_orders(&self) {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider != Some(jftrade_settings::MarketDataProvider::Futu)
            || !snapshot.opend_ready
        {
            return;
        }
        let Some(reader) = self.trade_read_port.clone() else {
            return;
        };
        let candidates = match self.store.list_reconciliation_candidates() {
            Ok(candidates) => candidates,
            Err(_) => return,
        };
        for order in candidates {
            let broker_id = order
                .broker_order_id
                .as_deref()
                .and_then(|value| value.trim().parse::<u64>().ok())
                .filter(|value| *value > 0);
            let broker_order_id_ex = order
                .broker_order_id_ex
                .as_deref()
                .map(str::trim)
                .filter(|value| !value.is_empty());
            if broker_id.is_none() && broker_order_id_ex.is_none() {
                if matches!(
                    order.status.to_ascii_uppercase().as_str(),
                    "SUBMITTING" | "SUBMISSION_UNKNOWN"
                ) {
                    let mut unknown = order.clone();
                    let error = failed(
                        502,
                        "EXECUTION_STATE_UNKNOWN",
                        "broker order identity is unavailable for reconciliation",
                    );
                    let now = crate::product::product_production_ports::provider_now_rfc3339();
                    let _ = self.persist_unknown(&mut unknown, &error, "reconcile_unknown", &now);
                }
                continue;
            }
            let Ok(header) = header_from_order(&order) else {
                continue;
            };
            let Ok(expected_revision) = self.store.order_revision(&order.internal_order_id) else {
                continue;
            };
            let snapshots = match reader.read_orders(header.clone(), None, Vec::new(), Some(true)) {
                Ok(value) => value,
                Err(_) => Vec::new(),
            };
            let matches_order = |candidate: &jftrade_integration_futu::TradeOrderSnapshot| {
                broker_id.is_some_and(|id| candidate.order_id == id)
                    || (broker_order_id_ex.is_some_and(|id| id == candidate.order_id_ex.trim()))
            };
            let mut matched = snapshots.into_iter().find(matches_order);
            if matched.is_none() {
                matched = reader
                    .read_history_orders(header, None, Vec::new(), Some(true))
                    .unwrap_or_default()
                    .into_iter()
                    .find(matches_order);
            }
            let Some(matched) = matched else {
                continue;
            };
            let _ = self.apply_broker_snapshot(&order, &matched, expected_revision);
        }
    }

    fn apply_broker_snapshot(
        &self,
        current: &StoredExecutionOrder,
        snapshot: &jftrade_integration_futu::TradeOrderSnapshot,
        expected_revision: u64,
    ) -> Result<(), ExecutionWritePortError> {
        let incoming = canonical_broker_status(order_status_label(snapshot.order_status));
        if incoming == OrderStatus::Unknown {
            return Ok(());
        }
        let stored_current = canonical_stored_status(&current.status);
        let accepted = if current.status.eq_ignore_ascii_case("CANCEL_SUBMITTED") {
            matches!(
                incoming,
                OrderStatus::Filled
                    | OrderStatus::Cancelled
                    | OrderStatus::Rejected
                    | OrderStatus::Expired
            )
        } else {
            reconcile_status(stored_current, incoming).1
        };
        if !accepted {
            return Ok(());
        }
        let mut next = current.clone();
        next.status = storage_status(incoming, current.status.as_str());
        next.raw_broker_status = Some(snapshot.order_status.to_string());
        next.broker_order_id = (snapshot.order_id > 0).then(|| snapshot.order_id.to_string());
        if !snapshot.order_id_ex.trim().is_empty() {
            next.broker_order_id_ex = Some(snapshot.order_id_ex.clone());
        }
        next.filled_quantity = snapshot.fill_qty;
        next.filled_average_price = snapshot.fill_avg_price;
        next.last_error = snapshot.last_err_msg.clone();
        next.last_error_code = None;
        next.last_error_source = snapshot.last_err_msg.as_ref().map(|_| "opend".to_owned());
        if next.status == current.status
            && next.raw_broker_status == current.raw_broker_status
            && next.filled_quantity == current.filled_quantity
            && next.filled_average_price == current.filled_average_price
            && next.last_error == current.last_error
        {
            return Ok(());
        }
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        next.updated_at = now.clone();
        let event_id = format!(
            "{}-reconcile-{}",
            current.internal_order_id,
            self.store.next_sequence("order-event").map_err(store_error)?
        );
        let next_status = next.status.clone();
        let payload_json = json!({
            "brokerOrderId": snapshot.order_id,
            "brokerOrderIdEx": snapshot.order_id_ex,
            "brokerStatus": snapshot.order_status,
            "filledQuantity": snapshot.fill_qty,
            "filledAveragePrice": snapshot.fill_avg_price,
        })
        .to_string();
        let event = StoredExecutionOrderEvent {
            id: &event_id,
            internal_order_id: &current.internal_order_id,
            event_type: "reconciled",
            previous_status: Some(current.status.as_str()),
            next_status: &next_status,
            payload_json: &payload_json,
            created_at: &now,
        };
        self.store
            .transition_order_and_event_fenced(
                next,
                &now,
                &event,
                current.status.as_str(),
                current.updated_at.as_str(),
                Some(expected_revision),
            )
            .map(|_| ())
            .map_err(map_transition_store_error)
    }

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
            .map_or(self.trade_logged_in, |runtime| {
                runtime.snapshot().trade_logged_in
            });
        if trade_logged_in != Some(true) {
            return Err(ExecutionWritePortError::Unavailable(
                "Futu trade account is not logged in".to_owned(),
            ));
        }
        if let Some(runtime) = self.trade_runtime.as_ref() {
            return runtime.writer_snapshot().ok_or_else(|| {
                ExecutionWritePortError::Unavailable(
                    "OpenD trade runtime is unavailable".to_owned(),
                )
            });
        }
        self.trade_write_port.clone().ok_or_else(|| {
            ExecutionWritePortError::Unavailable("OpenD trade runtime is unavailable".to_owned())
        })
    }

    fn place_order(&self, payload: &Value) -> Result<Value, ExecutionWritePortError> {
        let parsed = parse_order(payload).map_err(|message| failed(400, "BAD_REQUEST", message))?;
        let now = crate::product::product_production_ports::provider_now_rfc3339();
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
        let request_hash = preview_request_hash(payload, &parsed, None)?;
        if let Some(client_order_id) = parsed.client_order_id.as_deref()
            && let Some(existing) = self
                .store
                .find_order_by_client_identity(
                    &parsed.broker_id,
                    if parsed.header.trd_env == 1 { "REAL" } else { "SIMULATE" },
                    &parsed.header.acc_id.to_string(),
                    client_order_id,
                )
                .map_err(store_error)?
        {
            return replay_or_conflict(existing, &request_hash);
        }
        // Probe readiness before consuming a preview. A runtime that becomes
        // unavailable after this probe is still fenced as UNKNOWN below.
        let writer = self.writer()?;
        let capability_version = execution_order_previews::jftrade_broker_capability_version();
        let sequence = self
            .store
            .next_sequence("internal-order")
            .map_err(store_error)?;
        let internal_id = format!("rust-order-{sequence}");
        let mut order = new_order(&internal_id, &parsed, &now);
        order.normalized_request = canonical_execution_request(payload, &parsed, None)?;
        let reservation = self
            .store
            .reserve_order_with_preview_checked(
                order.clone(),
                &request_hash,
                &now,
                Some(capability_version.as_str()),
            )
            .map_err(map_reservation_error)?;
        if let ExecutionOrderReservation::Existing(existing) = reservation {
            return replay_or_conflict(existing, &request_hash);
        }
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
        self.persist_external_success(&order, "submitted", &previous_status, &now, &now)?;
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
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        let legs = execution_order_previews::canonical_combo_legs(&parsed);
        let request_hash = preview_request_hash(payload, &parsed.order, Some(legs.clone()))?;
        if let Some(client_order_id) = parsed.order.client_order_id.as_deref()
            && let Some(existing) = self
                .store
                .find_order_by_client_identity(
                    &parsed.order.broker_id,
                    if parsed.order.header.trd_env == 1 {
                        "REAL"
                    } else {
                        "SIMULATE"
                    },
                    &parsed.order.header.acc_id.to_string(),
                    client_order_id,
                )
                .map_err(store_error)?
        {
            return replay_or_conflict(existing, &request_hash);
        }
        // Keep the preview credential untouched when OpenD is already known
        // to be unavailable. A race after this probe is persisted as UNKNOWN.
        let writer = self.writer()?;
        let capability_version = execution_order_previews::jftrade_broker_capability_version();
        let sequence = self
            .store
            .next_sequence("internal-combo-order")
            .map_err(store_error)?;
        let internal_id = format!("rust-combo-{sequence}");
        let mut order = new_order(&internal_id, &parsed.order, &now);
        order.requested_quantity = Some(parsed.combo_quantity());
        order.normalized_request =
            canonical_execution_request(payload, &parsed.order, Some(legs))?;
        let reservation = self
            .store
            .reserve_order_with_preview_checked(
                order.clone(),
                &request_hash,
                &now,
                Some(capability_version.as_str()),
            )
            .map_err(map_reservation_error)?;
        if let ExecutionOrderReservation::Existing(existing) = reservation {
            return replay_or_conflict(existing, &request_hash);
        }
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
        self.persist_external_success(&order, "submitted", &previous_status, &now, &now)?;
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
        let expected_revision = self
            .store
            .order_revision(internal_id)
            .map_err(store_error)?;
        let fence_now = crate::product::product_production_ports::provider_now_rfc3339();
        order.status = "CANCEL_SUBMITTED".to_owned();
        order.updated_at = fence_now.clone();
        self.persist_transition_with_revision(
            &order,
            "cancel_submitted",
            &previous_status,
            &expected_updated_at,
            &fence_now,
            expected_revision,
        )?;
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
            self.persist_unknown(&mut order, &mapped, "cancel_failed", &now)?;
            return Err(mapped);
        }
        // The durable CANCEL_SUBMITTED fence was committed before the
        // external call.  A successful acknowledgement needs no second
        // state write; reconciliation will apply the broker terminal state.
        order_value(&order)
    }

    /// Resolve the public broker cancellation item to the durable local order
    /// identity.  The broker API accepts the numeric `orderId` plus optional
    /// broker/external identifiers; treating those fields as strings (or as a
    /// local id) silently cancels the wrong order or reports a false 400.
    fn resolve_broker_cancel_target(
        &self,
        item: &Value,
    ) -> Result<String, ExecutionWritePortError> {
        let object = item.as_object().ok_or_else(|| {
            failed(
                400,
                "BAD_REQUEST",
                "each order cancellation item must be an object",
            )
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

    fn persist_transition_with_revision(
        &self,
        order: &StoredExecutionOrder,
        event_type: &str,
        expected_status: &str,
        expected_updated_at: &str,
        timestamp: &str,
        expected_revision: u64,
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
            payload_json: &order.normalized_request,
            created_at: timestamp,
        };
        self.store
            .transition_order_and_event_fenced(
                order.clone(),
                timestamp,
                &event,
                expected_status,
                expected_updated_at,
                Some(expected_revision),
            )
            .map(|_| ())
            .map_err(map_transition_store_error)
    }

    /// Persist the local acknowledgement after an external broker command
    /// succeeded.  If the CAS/event transaction fails, the broker side effect
    /// is already irreversible; best-effortly fence the durable order as
    /// `UNKNOWN` so callers do not mistake a stale `SUBMITTING`/
    /// `CANCEL_SUBMITTED` row for a safely retryable command.
    fn persist_external_success(
        &self,
        order: &StoredExecutionOrder,
        event_type: &str,
        previous_status: &str,
        expected_updated_at: &str,
        timestamp: &str,
    ) -> Result<(), ExecutionWritePortError> {
        if let Err(error) = self.persist_transition(
            order,
            event_type,
            Some(previous_status),
            expected_updated_at,
            timestamp,
        ) {
            let mut unknown = order.clone();
            unknown.status = previous_status.to_owned();
            unknown.updated_at = expected_updated_at.to_owned();
            let detail = execution_error_details(&error).0;
            let unknown_error = failed(
                502,
                "EXECUTION_STATE_UNKNOWN",
                format!(
                    "broker accepted {event_type}, but local state could not be persisted: {detail}"
                ),
            );
            let _ = self.persist_unknown(&mut unknown, &unknown_error, "state_unknown", timestamp);
            return Err(unknown_error);
        }
        Ok(())
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

}

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
