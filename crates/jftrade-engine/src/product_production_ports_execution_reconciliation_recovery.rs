use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use jftrade_integration_futu::{TradeFilter, TradeOrderSnapshot, TradeReadPort};
use jftrade_store_sqlite::StoredExecutionOrder;

use super::*;

pub(super) enum RecoveryResolution {
    Recovered(Box<TradeOrderSnapshot>),
    TerminalFailed,
}

impl ProductionExecutionPort {
    pub(super) fn resolve_unidentified_submission(
        &self,
        reader: &Arc<dyn TradeReadPort>,
        order: &StoredExecutionOrder,
        header: &jftrade_integration_futu::TradeHeader,
    ) -> Result<RecoveryResolution, String> {
        let active_orders = reader
            .read_orders(header.clone(), Some(TradeFilter::default()), Vec::new(), Some(true))
            .map_err(|error| format!("broker order read failed: {error}"))?;
        let history_orders = reader
            .read_history_orders(header.clone(), Some(TradeFilter::default()), Vec::new(), Some(true))
            .map_err(|error| format!("broker order history read failed: {error}"))?;

        let mut broker_order_map: HashMap<(u64, String), TradeOrderSnapshot> = HashMap::new();
        for snapshot in active_orders.into_iter().chain(history_orders) {
            let key = (snapshot.order_id, snapshot.order_id_ex.trim().to_owned());
            if key.0 == 0 && key.1.is_empty() {
                continue;
            }
            match broker_order_map.entry(key) {
                std::collections::hash_map::Entry::Vacant(entry) => {
                    entry.insert(snapshot);
                }
                std::collections::hash_map::Entry::Occupied(mut entry) => {
                    if time_after(&snapshot.update_time, &entry.get().update_time) {
                        entry.insert(snapshot);
                    }
                }
            }
        }
        let broker_orders: Vec<TradeOrderSnapshot> = broker_order_map.into_values().collect();

        let local_orders = self
            .store
            .list_orders()
            .map_err(|error| format!("list execution orders for reconciliation: {error}"))?;
        let mut claimed_numeric_ids = HashSet::new();
        let mut claimed_ex_ids = HashSet::new();
        for other in &local_orders {
            if other.internal_order_id == order.internal_order_id {
                continue;
            }
            if let Some(id) = other
                .broker_order_id
                .as_deref()
                .and_then(|v| v.trim().parse::<u64>().ok())
                .filter(|v| *v > 0)
            {
                claimed_numeric_ids.insert(id);
            }
            if let Some(ex) = other
                .broker_order_id_ex
                .as_deref()
                .map(str::trim)
                .filter(|v| !v.is_empty())
            {
                claimed_ex_ids.insert(ex.to_owned());
            }
        }

        let unclaimed: Vec<TradeOrderSnapshot> = broker_orders
            .into_iter()
            .filter(|candidate| {
                if candidate.order_id > 0 && claimed_numeric_ids.contains(&candidate.order_id) {
                    return false;
                }
                let ex = candidate.order_id_ex.trim();
                if !ex.is_empty() && claimed_ex_ids.contains(ex) {
                    return false;
                }
                true
            })
            .collect();

        let candidates = find_recovery_candidates(&unclaimed, order);

        match candidates.len() {
            1 => {
                let snapshot = candidates.into_iter().next().unwrap();
                Ok(RecoveryResolution::Recovered(Box::new(snapshot)))
            }
            count if count > 1 => {
                let error = failed(
                    502,
                    "EXECUTION_STATE_AMBIGUOUS",
                    "multiple broker orders match pending submission without unique identity",
                );
                self.persist_unknown_if_needed(order, &error, "reconcile_identity_ambiguous")?;
                Err(
                    "multiple broker orders match pending submission without unique identity"
                        .to_owned(),
                )
            }
            _ => {
                let error = failed(
                    502,
                    "BROKER_ORDER_NOT_FOUND",
                    "broker order was not found in active or history snapshots; order was never placed/accepted by broker",
                );
                self.persist_failed_if_needed(order, &error, "reconcile_order_never_accepted")?;
                Ok(RecoveryResolution::TerminalFailed)
            }
        }
    }

    pub(super) fn persist_failed_if_needed(
        &self,
        order: &StoredExecutionOrder,
        error: &ExecutionWritePortError,
        event_type: &str,
    ) -> Result<(), String> {
        if order.status.eq_ignore_ascii_case("FAILED")
            && order.last_error_code.as_deref() == execution_error_details(error).1.as_deref()
        {
            return Ok(());
        }
        let mut failed_order = order.clone();
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        let previous_status = failed_order.status.clone();
        let expected_updated_at = failed_order.updated_at.clone();
        failed_order.status = "FAILED".to_owned();
        let (message, code) = execution_error_details(error);
        failed_order.last_error = Some(message);
        failed_order.last_error_code = code;
        failed_order.last_error_source = Some("reconciliation".to_owned());
        failed_order.updated_at = now.clone();
        self.persist_transition(
            &failed_order,
            event_type,
            Some(&previous_status),
            &expected_updated_at,
            &now,
        )
        .map_err(|error| format_error(&error))
    }
}

fn symbols_match(local_symbol: Option<&str>, local_market: &str, broker_code: &str) -> bool {
    let Some(local_symbol) = local_symbol.map(str::trim).filter(|s| !s.is_empty()) else {
        return false;
    };
    let broker_code = broker_code.trim();
    if broker_code.is_empty() {
        return false;
    }
    if local_symbol.eq_ignore_ascii_case(broker_code) {
        return true;
    }
    let broker_normalized = if broker_code.contains('.') {
        broker_code.to_ascii_uppercase()
    } else {
        format!(
            "{}.{}",
            local_market.trim().to_ascii_uppercase(),
            broker_code.to_ascii_uppercase()
        )
    };
    let local_normalized = if local_symbol.contains('.') {
        local_symbol.to_ascii_uppercase()
    } else {
        format!(
            "{}.{}",
            local_market.trim().to_ascii_uppercase(),
            local_symbol.to_ascii_uppercase()
        )
    };
    broker_normalized == local_normalized
}

fn parse_timestamp_to_seconds(s: &str) -> Option<i64> {
    let s = s.trim();
    if let Ok(dt) = time::OffsetDateTime::parse(s, &time::format_description::well_known::Rfc3339) {
        return Some(dt.unix_timestamp());
    }
    let format = time::format_description::parse_borrowed::<1>(
        "[year]-[month]-[day] [hour]:[minute]:[second]",
    )
    .ok()?;
    let pdt = time::PrimitiveDateTime::parse(s, &format).ok()?;
    Some(pdt.assume_utc().unix_timestamp())
}

fn within_submission_window(
    cand_time_str: &str,
    cand_ts: Option<f64>,
    order_created: &str,
    order_submitted: Option<&str>,
) -> bool {
    let order_time_str = order_submitted.unwrap_or(order_created);
    let order_epoch = parse_timestamp_to_seconds(order_time_str);
    let cand_epoch = cand_ts
        .map(|ts| ts as i64)
        .or_else(|| parse_timestamp_to_seconds(cand_time_str));
    match (order_epoch, cand_epoch) {
        (Some(o), Some(c)) => (o - 60..=o + 300).contains(&c),
        _ => cand_time_str.trim() == order_time_str.trim(),
    }
}

fn matches_priority_1(candidate: &TradeOrderSnapshot, order: &StoredExecutionOrder) -> bool {
    let Some(cand_remark) = candidate.remark.as_deref().map(str::trim).filter(|v| !v.is_empty()) else {
        return false;
    };
    if order
        .symbol
        .as_deref()
        .is_some_and(|symbol| !symbols_match(Some(symbol), &order.market, &candidate.code))
    {
        return false;
    }
    let matches_client_id = order
        .client_order_id
        .as_deref()
        .map(str::trim)
        .is_some_and(|client_id| !client_id.is_empty() && cand_remark == client_id);
    let matches_remark = order
        .remark
        .as_deref()
        .map(str::trim)
        .is_some_and(|remark| !remark.is_empty() && cand_remark == remark);
    matches_client_id || matches_remark
}

fn has_conflicting_remark(candidate: &TradeOrderSnapshot, order: &StoredExecutionOrder) -> bool {
    let Some(cand_remark) = candidate.remark.as_deref().map(str::trim).filter(|v| !v.is_empty()) else {
        return false;
    };
    let order_client_id = order.client_order_id.as_deref().map(str::trim).filter(|v| !v.is_empty());
    let order_remark = order.remark.as_deref().map(str::trim).filter(|v| !v.is_empty());

    let matches_client_id = order_client_id.is_some_and(|id| cand_remark == id);
    let matches_remark = order_remark.is_some_and(|rem| cand_remark == rem);

    !(matches_client_id || matches_remark)
}

fn matches_safe_attributes(candidate: &TradeOrderSnapshot, order: &StoredExecutionOrder) -> bool {
    if !symbols_match(order.symbol.as_deref(), &order.market, &candidate.code) {
        return false;
    }
    if order.side.as_deref().is_some_and(|expected_side| {
        !super::super::execution_order_parse::side_label(candidate.trd_side)
            .eq_ignore_ascii_case(expected_side)
    }) {
        return false;
    }
    let Some(expected_qty) = order.requested_quantity else {
        return false;
    };
    if !candidate.qty.is_finite() || (candidate.qty - expected_qty).abs() > 1e-6 {
        return false;
    }
    if order.order_type.as_deref().is_some_and(|expected_type| {
        !super::super::execution_order_parse::order_type_label(candidate.order_type)
            .eq_ignore_ascii_case(expected_type)
    }) {
        return false;
    }
    let is_limit = order.order_type.as_deref().is_none_or(|t| t.eq_ignore_ascii_case("LIMIT"));
    if is_limit {
        match (order.requested_price, candidate.price) {
            (Some(expected), Some(cand)) if cand.is_finite() && (cand - expected).abs() <= 1e-6 => {}
            (Some(_), _) => return false,
            _ => {}
        }
    }
    if !within_submission_window(
        &candidate.create_time,
        candidate.create_timestamp,
        &order.created_at,
        order.submitted_at.as_deref(),
    ) {
        return false;
    }
    true
}

fn find_recovery_candidates(
    unclaimed: &[TradeOrderSnapshot],
    order: &StoredExecutionOrder,
) -> Vec<TradeOrderSnapshot> {
    let priority_1_matches: Vec<TradeOrderSnapshot> = unclaimed
        .iter()
        .filter(|candidate| matches_priority_1(candidate, order))
        .cloned()
        .collect();

    if !priority_1_matches.is_empty() {
        return priority_1_matches;
    }

    let priority_2_matches: Vec<TradeOrderSnapshot> = unclaimed
        .iter()
        .filter(|candidate| {
            if has_conflicting_remark(candidate, order) {
                return false;
            }
            matches_safe_attributes(candidate, order)
        })
        .cloned()
        .collect();

    priority_2_matches
}

pub(super) fn time_after(left: &str, right: &str) -> bool {
    let left = time::OffsetDateTime::parse(left, &time::format_description::well_known::Rfc3339);
    let right = time::OffsetDateTime::parse(right, &time::format_description::well_known::Rfc3339);
    match (left, right) {
        (Ok(left), Ok(right)) => left > right,
        _ => false,
    }
}

pub(super) fn is_terminal(status: &str) -> bool {
    canonical_stored_status(status).is_terminal()
}

pub(super) fn format_error(error: &ExecutionWritePortError) -> String {
    let (message, code) = execution_error_details(error);
    match code {
        Some(code) => format!("{code}: {message}"),
        None => message,
    }
}

