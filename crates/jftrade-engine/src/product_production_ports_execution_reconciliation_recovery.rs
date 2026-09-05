use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use jftrade_integration_futu::{TradeFilter, TradeOrderSnapshot, TradeReadPort};
use jftrade_store_sqlite::StoredExecutionOrder;

use super::*;

impl ProductionExecutionPort {
    pub(super) fn resolve_unidentified_submission(
        &self,
        reader: &Arc<dyn TradeReadPort>,
        order: &StoredExecutionOrder,
        header: &jftrade_integration_futu::TradeHeader,
    ) -> Result<TradeOrderSnapshot, String> {
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
        let unclaimed = unclaimed_orders(broker_orders, &local_orders, order);

        let candidates = find_recovery_candidates(&unclaimed, order);

        match candidates.len() {
            1 => {
                let snapshot = candidates.into_iter().next().unwrap();
                Ok(snapshot)
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
                    "broker snapshots do not establish whether the submission was accepted",
                );
                self.persist_unknown_if_needed(order, &error, "reconcile_order_missing")?;
                Err("broker snapshots do not establish submission outcome".to_owned())
            }
        }
    }

}

fn same_order_scope(left: &StoredExecutionOrder, right: &StoredExecutionOrder) -> bool {
    left.broker_id.trim().eq_ignore_ascii_case(right.broker_id.trim())
        && left.account_id.trim() == right.account_id.trim()
        && left.trading_environment.trim().eq_ignore_ascii_case(right.trading_environment.trim())
        && left.market.trim().eq_ignore_ascii_case(right.market.trim())
}

fn unclaimed_orders(
    broker_orders: Vec<TradeOrderSnapshot>,
    local_orders: &[StoredExecutionOrder],
    order: &StoredExecutionOrder,
) -> Vec<TradeOrderSnapshot> {
    let mut claimed_numeric_ids = HashSet::new();
    let mut claimed_ex_ids = HashSet::new();
    for other in local_orders {
        if other.internal_order_id == order.internal_order_id || !same_order_scope(other, order) {
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

    broker_orders
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
        .collect()
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

fn matches_submission_identity(candidate: &TradeOrderSnapshot, order: &StoredExecutionOrder) -> bool {
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
    // Only the client id actually sent on the wire is correlation evidence.
    // User remarks and matching market attributes are not unique identities.
    let Some(client_id) = order.client_order_id.as_deref().map(str::trim).filter(|id| !id.is_empty())
    else {
        return false;
    };
    let sent_remark = order.remark.as_deref().map(str::trim).filter(|value| !value.is_empty())
        .unwrap_or(client_id);
    sent_remark == client_id && cand_remark == client_id && matches_safe_attributes(candidate, order)
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
    true
}

fn find_recovery_candidates(
    unclaimed: &[TradeOrderSnapshot],
    order: &StoredExecutionOrder,
) -> Vec<TradeOrderSnapshot> {
    unclaimed
        .iter()
        .filter(|candidate| matches_submission_identity(candidate, order))
        .cloned()
        .collect()
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
