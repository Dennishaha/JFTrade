//! Execution-order and broker write production adapters.

use std::collections::BTreeSet;
use std::sync::{Arc, Mutex};

use jftrade_integration_futu::{TradeModifyOrderRequest, TradeUnlockRequest, TradeWritePort};
use jftrade_store_sqlite::{
    ExecutionOrderReservation, ExecutionOrderStore, ExecutionOrderStoreError, StoredExecutionOrder,
    StoredExecutionOrderEvent, normalized_request_hash,
};
use jftrade_trading::{
    OrderStatus, PreTradeRiskOrder, TradingEnvironment, canonical_broker_status,
    canonical_stored_status, reconcile_status,
};
use serde_json::{Value, json};
use tokio::sync::{Notify, oneshot};
use tokio::task::JoinHandle;

use super::super::product_production_ports_trade::SharedTradeReadRuntime;
use crate::product::product_brokers_write_port::{
    BrokersWriteInput, BrokersWriteOperation, BrokersWritePort, BrokersWritePortError,
};
use crate::product::product_execution_write_port::{
    ExecutionWriteInput, ExecutionWriteOperation, ExecutionWritePort, ExecutionWritePortError,
};
use crate::product::{ActiveProviderState, ExecutionReadSnapshotError, ExecutionReadSnapshotPort};
#[path = "product_production_ports_execution_order_hash.rs"]
mod execution_order_hash;
#[path = "product_production_ports_execution_order_helpers.rs"]
mod execution_order_helpers;
use execution_order_hash::{canonical_execution_request, preview_request_hash};
use execution_order_helpers::{
    CancelInFlightGuard, broker_error, broker_failed, build_pre_trade_risk_combo_order,
    build_pre_trade_risk_order, execution_error_details, failed, header_from_order,
    is_terminal_status, map_trade_error, map_transition_store_error, merge_query, order_value,
    prefetch_combo_leg_quotes, store_error, value_identifier,
};

#[path = "product_production_ports_execution_order_parse.rs"]
mod execution_order_parse;
use execution_order_parse::{
    new_order, parse_combo_with_defaults, parse_order_with_defaults, requires_locked_preview,
};
#[path = "product_production_ports_execution_order_previews.rs"]
mod execution_order_previews;
#[path = "product_production_ports_execution_reconciliation.rs"]
mod execution_reconciliation;

pub(crate) struct ProductionExecutionPort {
    pub(crate) store: Arc<ExecutionOrderStore>,
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
    pub(crate) trade_logged_in: Option<bool>,
    pub(crate) trade_read_port: Option<Arc<dyn jftrade_integration_futu::TradeReadPort>>,
    pub(crate) trade_write_port: Option<Arc<dyn jftrade_integration_futu::TradeWritePort>>,
    pub(crate) trade_runtime: Option<Arc<SharedTradeReadRuntime>>,
    pub(crate) cancel_inflight: Arc<Mutex<BTreeSet<String>>>,
    pub(crate) risk_coordinator: Option<Arc<crate::product::ExecutionRiskCoordinator>>,
    pub(crate) default_trading_environment: Option<Arc<dyn Fn() -> String + Send + Sync>>,
    pub(crate) notification_projector:
        Option<Arc<crate::product::ExecutionNotificationProjector>>,
}

impl ProductionExecutionPort {
    pub(crate) fn project_notifications(&self) {
        if let Some(ref projector) = self.notification_projector {
            let _ = projector.project_pending();
        }
    }
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

/// Runtime-owned execution reconciliation status.  This is intentionally
/// separate from the HTTP read port: GET orders remains a pure durable read,
/// while broker scans report their health through the runtime worker.
#[derive(Clone, Debug, Eq, PartialEq, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct ExecutionReconciliationWorkerStatus {
    pub state: String,
    pub scans: u64,
    pub reconciled: u64,
    pub failures: u64,
    pub last_scan_at: Option<String>,
    pub last_error: Option<String>,
    pub next_retry_at: Option<String>,
}

impl Default for ExecutionReconciliationWorkerStatus {
    fn default() -> Self {
        Self {
            state: "starting".to_owned(),
            scans: 0,
            reconciled: 0,
            failures: 0,
            last_scan_at: None,
            last_error: None,
            next_retry_at: None,
        }
    }
}

/// Owns the asynchronous broker reconciliation cadence and shutdown handle.
/// It uses the shared trade runtime at scan time so dynamic Futu activation
/// does not couple execution to the active market-data router.
pub(crate) struct ExecutionReconciliationWorker {
    stop_tx: Mutex<Option<oneshot::Sender<()>>>,
    #[allow(dead_code)]
    wake: Arc<Notify>,
    handle: Mutex<Option<JoinHandle<()>>>,
    status: Arc<Mutex<ExecutionReconciliationWorkerStatus>>,
}

impl std::fmt::Debug for ExecutionReconciliationWorker {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("ExecutionReconciliationWorker")
            .field("status", &self.status())
            .finish_non_exhaustive()
    }
}

impl ExecutionReconciliationWorker {
    pub(crate) fn start(
        port: Arc<ProductionExecutionPort>,
        wake: Option<Arc<Notify>>,
    ) -> Arc<Self> {
        // Keep the worker from extending the WriterLease lifetime after a
        // synchronous `ProductRuntimeHandle` drop.  Each scan upgrades this
        // weak reference only for the blocking reconciliation call and drops
        // the port before waiting for the next wake/timer event.
        let port = Arc::downgrade(&port);
        let wake = wake.unwrap_or_else(|| Arc::new(Notify::new()));
        let status = Arc::new(Mutex::new(ExecutionReconciliationWorkerStatus::default()));
        let (stop_tx, mut stop_rx) = oneshot::channel();
        let task_wake = Arc::clone(&wake);
        let task_status = Arc::clone(&status);
        let handle = tokio::spawn(async move {
            let mut retry_delay = std::time::Duration::from_secs(1);
            let max_retry_delay = std::time::Duration::from_secs(60);
            loop {
                let scan = {
                    let Some(port) = port.upgrade() else {
                        break;
                    };
                    let result = port.reconcile_pending_orders();
                    port.project_notifications();
                    result
                };
                let now = crate::product::product_production_ports::provider_now_rfc3339();
                let mut next_delay = std::time::Duration::from_secs(15);
                {
                    let mut state = task_status
                        .lock()
                        .unwrap_or_else(|error| error.into_inner());
                    state.scans = state.scans.saturating_add(1);
                    state.last_scan_at = Some(now.clone());
                    match scan {
                        Ok(reconciled) => {
                            state.state = "ready".to_owned();
                            state.reconciled = state.reconciled.saturating_add(reconciled as u64);
                            state.last_error = None;
                            state.next_retry_at = None;
                            retry_delay = std::time::Duration::from_secs(1);
                        }
                        Err(error) => {
                            state.state = "degraded".to_owned();
                            state.failures = state.failures.saturating_add(1);
                            state.last_error = Some(error);
                            next_delay = retry_delay;
                            retry_delay = retry_delay.saturating_mul(2).min(max_retry_delay);
                            state.next_retry_at = time::OffsetDateTime::now_utc()
                                .checked_add(time::Duration::seconds(next_delay.as_secs() as i64))
                                .and_then(|value| {
                                    value
                                        .format(&time::format_description::well_known::Rfc3339)
                                        .ok()
                                });
                        }
                    }
                }
                tokio::select! {
                    _ = &mut stop_rx => break,
                    _ = task_wake.notified() => {},
                    _ = tokio::time::sleep(next_delay) => {},
                }
            }
            let mut state = task_status
                .lock()
                .unwrap_or_else(|error| error.into_inner());
            state.state = "stopped".to_owned();
            state.next_retry_at = None;
        });
        Arc::new(Self {
            stop_tx: Mutex::new(Some(stop_tx)),
            wake,
            handle: Mutex::new(Some(handle)),
            status,
        })
    }

    #[allow(dead_code)]
    pub(crate) fn wake(&self) {
        self.wake.notify_one();
    }

    pub(crate) fn status(&self) -> ExecutionReconciliationWorkerStatus {
        self.status
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .clone()
    }

    pub(crate) async fn shutdown(&self) {
        if let Some(sender) = self
            .stop_tx
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .take()
        {
            let _ = sender.send(());
        }
        let handle = self
            .handle
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .take();
        if let Some(handle) = handle
            && let Err(error) = handle.await
        {
            let mut state = self
                .status
                .lock()
                .unwrap_or_else(|poisoned| poisoned.into_inner());
            state.state = "failed".to_owned();
            state.last_error = Some(format!(
                "execution reconciliation worker join failed: {error}"
            ));
        }
    }

    pub(crate) fn terminate(&self) {
        if let Some(sender) = self
            .stop_tx
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .take()
        {
            let _ = sender.send(());
        }
        if let Some(handle) = self
            .handle
            .lock()
            .unwrap_or_else(|error| error.into_inner())
            .take()
        {
            handle.abort();
        }
        let mut state = self
            .status
            .lock()
            .unwrap_or_else(|error| error.into_inner());
        state.state = "stopped".to_owned();
        state.next_retry_at = None;
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
        let result = match input.operation {
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
        };
        self.project_notifications();
        result
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

#[cfg(test)]
#[path = "product_production_ports_execution_preview_tests.rs"]
mod execution_preview_tests;
#[cfg(test)]
#[path = "product_production_ports_execution_order_validation_tests.rs"]
mod execution_order_validation_tests;

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
        | ExecutionOrderStoreError::NotFound(message) => failed(400, "PREVIEW_INVALID", message),
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
