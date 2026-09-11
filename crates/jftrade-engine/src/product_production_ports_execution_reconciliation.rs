//! Durable production reconciliation for orders, fills and fees.

use std::collections::HashSet;

use jftrade_integration_futu::{
    TradeFillSnapshot, TradeFilter, TradeOrderFeeSnapshot, TradeOrderSnapshot, TradeReadPort,
};
use jftrade_store_sqlite::{StoredExecutionOrder, StoredExecutionOrderEvent};
use jftrade_trading::{OrderStatus, canonical_broker_status, canonical_stored_status};
use serde_json::{Value, json};

use super::ProductionExecutionPort;
use super::execution_order_helpers::{
    execution_error_details, failed, header_from_order, store_error,
};
use crate::product::product_execution_write_port::ExecutionWritePortError;

#[path = "product_production_ports_execution_reconciliation_recovery.rs"]
mod recovery;
use recovery::*;

#[path = "execution_reconciliation_discovery.rs"]
mod discovery;
use discovery::*;

impl ProductionExecutionPort {
    #[allow(dead_code)]
    pub(super) fn reconcile_pending_orders(&self) -> Result<usize, String> {
        // A timed-out `spawn_blocking` scan cannot be force-cancelled.  Keep
        // the store-owned process-local fence for the whole read/project
        // sequence so a replacement worker cannot race the late owner.
        let _scan_guard = self
            .store
            .lock_reconciliation_scan()
            .map_err(|error| format!("acquire reconciliation scan fence: {error}"))?;
        self.reconcile_pending_orders_inner()
    }

    /// Run a worker-owned scan while retaining the same store fence through
    /// notification projection.  `accept_results` is checked only after the
    /// blocking provider call returns, so a shutdown that races that call
    /// suppresses all late user-visible side effects while preserving durable
    /// order/fill reconciliation.
    pub(super) fn reconcile_pending_orders_with_projection(
        &self,
        accept_results: &std::sync::atomic::AtomicBool,
    ) -> Result<usize, String> {
        let _scan_guard = self
            .store
            .lock_reconciliation_scan()
            .map_err(|error| format!("acquire reconciliation scan fence: {error}"))?;
        let result = self.reconcile_pending_orders_inner();
        if accept_results.load(std::sync::atomic::Ordering::Acquire) {
            self.project_notifications();
        }
        result
    }

    fn reconcile_pending_orders_inner(&self) -> Result<usize, String> {
        let reader = self.reconciliation_reader()?;
        let accounts = reader
            .read_accounts(0, None, None)
            .map_err(|error| format!("broker account discovery failed: {error}"))?;
        if accounts.is_empty() {
            return Err("broker account discovery returned no accounts".to_owned());
        }
        let mut account_keys = HashSet::with_capacity(accounts.len() * 2);
        for account in &accounts {
            let environment = match account.trd_env {
                0 => "SIMULATE",
                1 => "REAL",
                value => {
                    return Err(format!(
                        "broker account discovery returned unknown trading environment {value}"
                    ));
                }
            };
            if account.acc_id > 0 {
                account_keys.insert((account.acc_id.to_string(), environment.to_owned()));
            }
            if let Some(identity) = account_identity(account) {
                account_keys.insert((identity, environment.to_owned()));
            }
        }
        let candidates = self.reconciliation_candidates()?;
        let mut failures = Vec::new();
        let mut reconciled = 0;
        for order in candidates {
            match self.reconcile_order(&reader, &account_keys, &order) {
                Ok(changed) => reconciled += usize::from(changed),
                Err(error) => failures.push(format!("{}: {error}", order.internal_order_id)),
            }
        }
        // Existing local submissions are reconciled first so an unidentified
        // submission can claim its unique broker identity before the same
        // snapshot is considered an external order.  The discovery pass then
        // persists every remaining active/history order and fill, including
        // the empty-ledger and fill-before-order restart cases.
        match self.discover_external_ledger(&reader, &accounts) {
            Ok(count) => reconciled += count,
            Err(error) => failures.push(error),
        }
        if failures.is_empty() {
            Ok(reconciled)
        } else {
            Err(failures.join("; "))
        }
    }

    fn reconciliation_reader(&self) -> Result<std::sync::Arc<dyn TradeReadPort>, String> {
        // A client handle plus a positive login bit is not sufficient proof
        // that the physical OpenD session is currently usable: the handle can
        // survive a disconnect/reconnect cycle.  The composition root
        // publishes the probe/recorder result through the shared provider
        // state, independently of which market-data provider owns catalog
        // reads.  Keep helper-backed yfinance/AKShare operation decoupled by
        // checking only the OpenD readiness bit, never the active provider.
        if !self.active_provider_state.snapshot().opend_ready {
            return Err(
                "Futu execution reconciliation is unavailable: OpenD runtime is not ready"
                    .to_owned(),
            );
        }
        if let Some(runtime) = self.trade_runtime.as_ref() {
            let snapshot = runtime.snapshot();
            if snapshot.trade_logged_in != Some(true) {
                return Err(
                    "Futu execution reconciliation is unavailable: trade account is not logged in"
                        .to_owned(),
                );
            }
            return snapshot.client.ok_or_else(|| {
                "Futu execution reconciliation is unavailable: trade reader is not ready".to_owned()
            });
        }
        if self.trade_logged_in != Some(true) {
            return Err(
                "Futu execution reconciliation is unavailable: trade account is not logged in"
                    .to_owned(),
            );
        }
        self.trade_read_port.clone().ok_or_else(|| {
            "Futu execution reconciliation is unavailable: trade reader is unavailable".to_owned()
        })
    }

    fn reconcile_order(
        &self,
        reader: &std::sync::Arc<dyn TradeReadPort>,
        account_keys: &HashSet<(String, String)>,
        order: &StoredExecutionOrder,
    ) -> Result<bool, String> {
        let account_key = (
            order.account_id.trim().to_owned(),
            order.trading_environment.trim().to_ascii_uppercase(),
        );
        if !account_keys.contains(&account_key) {
            let error = failed(
                502,
                "BROKER_ACCOUNT_NOT_DISCOVERED",
                "broker account was not returned by account discovery",
            );
            self.persist_unknown_if_needed(order, &error, "reconcile_account_unknown")?;
            return Err(format!(
                "broker account not discovered for {}",
                order.account_id
            ));
        }
        let mut broker_id = order
            .broker_order_id
            .as_deref()
            .and_then(|value| value.trim().parse::<u64>().ok())
            .filter(|value| *value > 0);
        let mut broker_order_id_ex = order
            .broker_order_id_ex
            .as_deref()
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(str::to_owned);
        let header = header_from_order(order).map_err(|error| format_error(&error))?;
        let matched = if broker_id.is_none() && broker_order_id_ex.is_none() {
            let snapshot = self.resolve_unidentified_submission(reader, order, &header)?;
            broker_id = (snapshot.order_id > 0).then_some(snapshot.order_id);
            broker_order_id_ex = (!snapshot.order_id_ex.trim().is_empty())
                .then(|| snapshot.order_id_ex.trim().to_owned());
            Some(snapshot)
        } else {
            let filter = TradeFilter {
                id_list: broker_id.into_iter().collect(),
                order_id_ex_list: broker_order_id_ex.clone().into_iter().collect(),
                ..TradeFilter::default()
            };
            let active =
                reader.read_orders(header.clone(), Some(filter.clone()), Vec::new(), Some(true));
            let history = reader.read_history_orders(
                header.clone(),
                Some(filter.clone()),
                Vec::new(),
                Some(true),
            );
            let matched = match (active, history) {
                (Ok(active), Ok(history)) => active.into_iter().chain(history).find(|candidate| {
                    matches_order(candidate, broker_id, broker_order_id_ex.as_deref())
                }),
                (Err(active), Ok(_)) => {
                    return Err(format!("broker order read failed: {active}"));
                }
                (Ok(_), Err(history)) => {
                    return Err(format!("broker order history read failed: {history}"));
                }
                (Err(active), Err(history)) => {
                    return Err(format!(
                        "broker order reads failed: active={active}; history={history}"
                    ));
                }
            };
            if matched.is_none() {
                if is_terminal(&order.status) {
                    return self.reconcile_terminal_fees_only(reader, header, order);
                }
                let error = failed(
                    502,
                    "BROKER_ORDER_NOT_FOUND",
                    "broker order identity was not found in active or history snapshots",
                );
                self.persist_unknown_if_needed(order, &error, "reconcile_order_missing")?;
                return Err(
                    "broker order identity was not found in active or history snapshots".to_owned(),
                );
            }
            matched
        };
        let mut changed = false;
        if let Some(snapshot) = matched {
            match self.apply_broker_snapshot_with_recovery(order, &snapshot) {
                Ok(value) => changed |= value,
                Err(error) => {
                    if matches!(
                        &error,
                        ExecutionWritePortError::Failed { code, .. }
                            if code == "BROKER_STATUS_UNKNOWN"
                    ) {
                        self.persist_unknown_if_needed(order, &error, "reconcile_status_unknown")?;
                    }
                    return Err(format_error(&error));
                }
            }
        }

        let filter = TradeFilter {
            id_list: broker_id.into_iter().collect(),
            order_id_ex_list: broker_order_id_ex.clone().into_iter().collect(),
            ..TradeFilter::default()
        };
        let fills = self.read_fills(reader, &header, &filter)?;
        for fill in fills {
            if !matches_fill(&fill, broker_id, broker_order_id_ex.as_deref()) {
                continue;
            }
            let current = self
                .store
                .get_order(&order.internal_order_id)
                .map_err(|error| format!("reload order for fill: {error}"))?
                .ok_or_else(|| {
                    "execution order disappeared during fill reconciliation".to_owned()
                })?;
            let revision = self
                .store
                .order_revision(&current.internal_order_id)
                .map_err(|error| format!("read fill revision: {error}"))?;
            let fill_changed = self
                .apply_fill_snapshot(&current, &fill, revision)
                .map_err(|error| format_error(&error))?;
            changed |= fill_changed;
        }

        let current = self
            .store
            .get_order(&order.internal_order_id)
            .map_err(|error| format!("reload order for fees: {error}"))?
            .ok_or_else(|| "execution order disappeared during fee reconciliation".to_owned())?;
        if current
            .broker_order_id_ex
            .as_deref()
            .is_some_and(|value| !value.trim().is_empty())
            && (is_terminal(&current.status) || current.fees.is_none())
        {
            let fee_ids = vec![current.broker_order_id_ex.clone().unwrap_or_default()];
            let fees = reader
                .read_order_fees(header, fee_ids)
                .map_err(|error| format!("broker order fee read failed: {error}"))?;
            for fee in fees {
                if !matches_fee(
                    &fee,
                    current.broker_order_id_ex.as_deref().unwrap_or_default(),
                ) {
                    continue;
                }
                let fee_current = self
                    .store
                    .get_order(&current.internal_order_id)
                    .map_err(|error| format!("reload order for fee: {error}"))?
                    .ok_or_else(|| {
                        "execution order disappeared during fee reconciliation".to_owned()
                    })?;
                let revision = self
                    .store
                    .order_revision(&fee_current.internal_order_id)
                    .map_err(|error| format!("read fee revision: {error}"))?;
                let fee_changed = self
                    .apply_fee_snapshot(&fee_current, &fee, revision)
                    .map_err(|error| format_error(&error))?;
                changed |= fee_changed;
            }
        }
        Ok(changed)
    }

    fn reconcile_terminal_fees_only(
        &self,
        reader: &std::sync::Arc<dyn TradeReadPort>,
        header: jftrade_integration_futu::TradeHeader,
        order: &StoredExecutionOrder,
    ) -> Result<bool, String> {
        let Some(order_id_ex) = order
            .broker_order_id_ex
            .as_deref()
            .map(str::trim)
            .filter(|value| !value.is_empty())
        else {
            return Ok(false);
        };
        let fees = reader
            .read_order_fees(header, vec![order_id_ex.to_owned()])
            .map_err(|error| format!("broker order fee read failed: {error}"))?;
        let mut changed = false;
        for fee in fees {
            if !matches_fee(&fee, order_id_ex) {
                continue;
            }
            let current = self
                .store
                .get_order(&order.internal_order_id)
                .map_err(|error| format!("reload terminal order for fee: {error}"))?
                .ok_or_else(|| {
                    "execution order disappeared during fee reconciliation".to_owned()
                })?;
            let revision = self
                .store
                .order_revision(&current.internal_order_id)
                .map_err(|error| format!("read terminal fee revision: {error}"))?;
            changed |= self
                .apply_fee_snapshot(&current, &fee, revision)
                .map_err(|error| format_error(&error))?;
        }
        Ok(changed)
    }

    fn read_fills(
        &self,
        reader: &std::sync::Arc<dyn TradeReadPort>,
        header: &jftrade_integration_futu::TradeHeader,
        filter: &TradeFilter,
    ) -> Result<Vec<TradeFillSnapshot>, String> {
        let active = reader.read_fills(header.clone(), Some(filter.clone()), Some(true));
        let history = reader.read_history_fills(header.clone(), Some(filter.clone()), Some(true));
        let mut fills = match (active, history) {
            (Ok(mut active), Ok(history)) => {
                active.extend(history);
                active
            }
            (Err(active), Ok(_)) => {
                return Err(format!("broker fill read failed: {active}"));
            }
            (Ok(_), Err(history)) => {
                return Err(format!("broker fill history read failed: {history}"));
            }
            (Err(active), Err(history)) => {
                return Err(format!(
                    "broker fill and history reads failed: {active}; {history}"
                ));
            }
        };
        fills.sort_by(|left, right| {
            fill_identity(left)
                .cmp(&fill_identity(right))
                .then_with(|| left.create_time.cmp(&right.create_time))
        });
        let mut deduplicated = Vec::with_capacity(fills.len());
        for fill in fills {
            if let Some(previous) = deduplicated
                .iter()
                .find(|previous| fill_identity(previous) == fill_identity(&fill))
            {
                if !equivalent_fill_observation(previous, &fill) {
                    return Err(format!(
                        "broker returned conflicting snapshots for fill {}",
                        fill_identity(&fill)
                    ));
                }
                continue;
            }
            deduplicated.push(fill);
        }
        deduplicated.sort_by(|left, right| {
            left.create_time
                .cmp(&right.create_time)
                .then_with(|| fill_identity(left).cmp(&fill_identity(right)))
        });
        Ok(deduplicated)
    }

    fn apply_broker_snapshot_with_recovery(
        &self,
        order: &StoredExecutionOrder,
        snapshot: &TradeOrderSnapshot,
    ) -> Result<bool, ExecutionWritePortError> {
        let revision = self
            .store
            .order_revision(&order.internal_order_id)
            .map_err(store_error)?;
        self.apply_broker_snapshot(order, snapshot, revision)
    }

    fn persist_unknown_if_needed(
        &self,
        order: &StoredExecutionOrder,
        error: &ExecutionWritePortError,
        event_type: &str,
    ) -> Result<(), String> {
        if order.status.eq_ignore_ascii_case("UNKNOWN")
            && order.last_error_code.as_deref() == execution_error_details(error).1.as_deref()
        {
            return Ok(());
        }
        let mut unknown = order.clone();
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        self.persist_unknown(&mut unknown, error, event_type, &now)
            .map_err(|error| format_error(&error))
    }

    fn apply_fill_snapshot(
        &self,
        current: &StoredExecutionOrder,
        fill: &TradeFillSnapshot,
        expected_revision: u64,
    ) -> Result<bool, ExecutionWritePortError> {
        validate_fill_snapshot(fill)?;
        let events = self
            .store
            .list_order_events(&current.internal_order_id)
            .map_err(store_error)?;
        let identity = fill_identity(fill);
        for event in &events {
            let payload = serde_json::from_str::<Value>(&event.payload_json).map_err(|error| {
                invalid_stored(format!(
                    "stored order event {} has invalid JSON payload: {error}",
                    event.id
                ))
            })?;
            if event.event_type == "BROKER_FILL_RECEIVED"
                && payload.get("fillIdentity").and_then(Value::as_str) == Some(identity.as_str())
            {
                return Ok(false);
            }
        }
        let covered = covered_by_snapshot(&events, fill)?;
        let applied_quantity = (fill.qty - covered).max(0.0);
        let mut next = current.clone();
        if applied_quantity > 0.0 {
            let previous_quantity = current.filled_quantity.unwrap_or(0.0).max(0.0);
            let next_quantity = previous_quantity + applied_quantity;
            if let Some(price) = finite_positive(fill.price) {
                let previous_value = current
                    .filled_average_price
                    .filter(|value| value.is_finite() && *value >= 0.0)
                    .unwrap_or(price)
                    * previous_quantity;
                next.filled_average_price = Some(
                    (previous_value + price * applied_quantity) / next_quantity.max(f64::EPSILON),
                );
            }
            next.filled_quantity = Some(next_quantity);
            if current
                .requested_quantity
                .is_some_and(|qty| next_quantity >= qty)
            {
                next.status = "FILLED".to_owned();
            } else if matches!(
                canonical_stored_status(&current.status),
                OrderStatus::Submitting | OrderStatus::Submitted | OrderStatus::BrokerAccepted
            ) {
                next.status = "PARTIALLY_FILLED".to_owned();
            }
        }
        if fill.order_id.is_some_and(|value| value > 0) && next.broker_order_id.is_none() {
            next.broker_order_id = fill.order_id.map(|value| value.to_string());
        }
        if next.broker_order_id_ex.is_none()
            && fill
                .order_id_ex
                .as_deref()
                .is_some_and(|value| !value.trim().is_empty())
        {
            next.broker_order_id_ex = fill.order_id_ex.clone();
        }
        if !fill.code.trim().is_empty() {
            next.symbol = Some(if fill.code.contains('.') {
                fill.code.trim().to_ascii_uppercase()
            } else {
                format!("{}.{}", next.market, fill.code.trim().to_ascii_uppercase())
            });
        }
        next.side = Some(super::execution_order_parse::side_label(fill.trd_side).to_owned());
        next.last_error = None;
        next.last_error_code = None;
        next.last_error_source = None;
        next.updated_at = crate::product::product_production_ports::provider_now_rfc3339();
        let timestamp = next.updated_at.clone();
        let next_status = next.status.clone();
        let payload_json = json!({
            "fillIdentity": identity,
            "brokerFillId": fill.fill_id,
            "brokerFillIdEx": fill.fill_id_ex,
            "brokerOrderId": fill.order_id,
            "brokerOrderIdEx": fill.order_id_ex,
            "filledQuantity": fill.qty,
            "fillPrice": fill.price,
            "filledAt": fill.create_time,
        })
        .to_string();
        let event_id = format!(
            "{}-fill-{}",
            current.internal_order_id,
            self.store
                .next_sequence("order-event")
                .map_err(store_error)?
        );
        let event = StoredExecutionOrderEvent {
            id: &event_id,
            internal_order_id: &current.internal_order_id,
            event_type: "BROKER_FILL_RECEIVED",
            previous_status: Some(current.status.as_str()),
            next_status: &next_status,
            payload_json: &payload_json,
            created_at: &timestamp,
        };
        self.store
            .transition_order_and_event_fenced(
                next,
                &timestamp,
                &event,
                current.status.as_str(),
                current.updated_at.as_str(),
                Some(expected_revision),
            )
            .map_err(super::execution_order_helpers::map_transition_store_error)?;
        Ok(applied_quantity > 0.0)
    }

    fn apply_fee_snapshot(
        &self,
        current: &StoredExecutionOrder,
        fee: &TradeOrderFeeSnapshot,
        expected_revision: u64,
    ) -> Result<bool, ExecutionWritePortError> {
        let Some(amount) = fee_amount(fee) else {
            return Ok(false);
        };
        if !amount.is_finite() || amount < 0.0 {
            return Err(failed(
                502,
                "BROKER_INVALID_RESPONSE",
                "OpenD returned an invalid order fee",
            ));
        }
        if current
            .fees
            .is_some_and(|value| (value - amount).abs() <= f64::EPSILON)
        {
            return Ok(false);
        }
        let mut next = current.clone();
        next.fees = Some(amount);
        next.updated_at = crate::product::product_production_ports::provider_now_rfc3339();
        let timestamp = next.updated_at.clone();
        let next_status = next.status.clone();
        let payload_json = json!({
            "brokerOrderIdEx": fee.broker_order_id_ex,
            "feeAmount": amount,
            "feeItems": fee.fee_items,
        })
        .to_string();
        let event_id = format!(
            "{}-fee-{}",
            current.internal_order_id,
            self.store
                .next_sequence("order-event")
                .map_err(store_error)?
        );
        let event = StoredExecutionOrderEvent {
            id: &event_id,
            internal_order_id: &current.internal_order_id,
            event_type: "BROKER_ORDER_FEES_UPDATED",
            previous_status: Some(current.status.as_str()),
            next_status: &next_status,
            payload_json: &payload_json,
            created_at: &timestamp,
        };
        self.store
            .transition_order_and_event_fenced(
                next,
                &timestamp,
                &event,
                current.status.as_str(),
                current.updated_at.as_str(),
                Some(expected_revision),
            )
            .map_err(super::execution_order_helpers::map_transition_store_error)?;
        Ok(true)
    }
}

#[cfg(test)]
#[path = "product_production_ports_execution_reconciliation_tests.rs"]
mod tests;
