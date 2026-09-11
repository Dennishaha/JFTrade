//! Broker order/fill discovery and reconciliation projections.

use super::*;

/// A broker query scope is derived from the authenticated account list rather
/// than from local ledger rows.  This is important after a process restart (or
/// when another client placed an order): an empty local ledger must still scan
/// every account/environment/market OpenD owns.
#[derive(Clone, Debug, Eq, Hash, PartialEq)]
pub(super) struct ReconciliationScopeKey {
    account_id: String,
    environment: String,
    market: String,
    trd_market: i32,
}

#[derive(Clone, Debug)]
struct ReconciliationScope {
    key: ReconciliationScopeKey,
    orders: Vec<TradeOrderSnapshot>,
    fills: Vec<TradeFillSnapshot>,
}

impl ProductionExecutionPort {
    /// Read every account/market scope and fold broker snapshots into the
    /// durable execution ledger.  This pass is deliberately independent from
    /// `reconciliation_candidates`: a user may have placed an order outside
    /// this process, leaving no local placeholder to drive reconciliation.
    pub(super) fn discover_external_ledger(
        &self,
        reader: &std::sync::Arc<dyn TradeReadPort>,
        accounts: &[jftrade_integration_futu::TradeAccountSnapshot],
    ) -> Result<usize, String> {
        let scopes = reconciliation_scopes(accounts);
        let mut discovered = 0;
        for scope in scopes {
            let scope = self.read_scope(reader, scope)?;
            discovered += self.persist_scope_orders(&scope)?;
            discovered += self.persist_scope_fills(&scope)?;
        }
        Ok(discovered)
    }

    fn read_scope(
        &self,
        reader: &std::sync::Arc<dyn TradeReadPort>,
        scope: ReconciliationScopeKey,
    ) -> Result<ReconciliationScope, String> {
        let acc_id = scope
            .account_id
            .parse::<u64>()
            .ok()
            .filter(|value| *value > 0)
            .ok_or_else(|| {
                format!(
                    "broker order discovery refused invalid account identity {:?}",
                    scope.account_id
                )
            })?;
        let header = jftrade_integration_futu::TradeHeader {
            trd_env: if scope.environment == "REAL" { 1 } else { 0 },
            acc_id,
            trd_market: scope.trd_market,
            jp_acc_type: None,
        };
        let orders = read_order_scope(reader, &header, &scope)?;
        let fills = read_fill_scope(reader, &header, &scope)?;
        Ok(ReconciliationScope {
            key: scope,
            orders,
            fills,
        })
    }

    fn persist_scope_orders(&self, scope: &ReconciliationScope) -> Result<usize, String> {
        let existing = self
            .store
            .list_orders()
            .map_err(|error| format!("list orders for broker discovery: {error}"))?;
        let mut changed = 0;
        for snapshot in &scope.orders {
            if order_identity(snapshot).is_none() {
                continue;
            }
            if let Some(current) = find_scoped_order(&existing, &scope.key, snapshot) {
                let revision = self
                    .store
                    .order_revision(&current.internal_order_id)
                    .map_err(|error| format!("read discovered order revision: {error}"))?;
                match self.apply_broker_snapshot(&current, snapshot, revision) {
                    Ok(value) => changed += usize::from(value),
                    Err(error) => {
                        if is_unknown_status_error(&error) {
                            self.persist_unknown_if_needed(
                                &current,
                                &error,
                                "reconcile_status_unknown",
                            )?;
                        } else {
                            return Err(format_error(&error));
                        }
                    }
                }
                continue;
            }
            if self.persist_discovered_order(&scope.key, snapshot)? {
                changed += 1;
            }
        }
        Ok(changed)
    }

    fn persist_scope_fills(&self, scope: &ReconciliationScope) -> Result<usize, String> {
        let mut changed = 0;
        for fill in &scope.fills {
            if !fill_matches_scope(fill, &scope.key) {
                continue;
            }
            validate_fill_snapshot(fill).map_err(|error| format_error(&error))?;
            let existing = self
                .store
                .list_orders()
                .map_err(|error| format!("list orders for broker fill discovery: {error}"))?;
            let current = find_scoped_fill_order(&existing, &scope.key, fill);
            let current = match current {
                Some(current) => current,
                None => {
                    self.persist_discovered_fill(&scope.key, fill)?;
                    changed += 1;
                    continue;
                }
            };
            let revision = self
                .store
                .order_revision(&current.internal_order_id)
                .map_err(|error| format!("read discovered fill revision: {error}"))?;
            changed += usize::from(
                self.apply_fill_snapshot(&current, fill, revision)
                    .map_err(|error| format_error(&error))?,
            );
        }
        Ok(changed)
    }

    fn persist_discovered_order(
        &self,
        scope: &ReconciliationScopeKey,
        snapshot: &TradeOrderSnapshot,
    ) -> Result<bool, String> {
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        let internal_id = format!(
            "rust-broker-order-{}",
            self.store
                .next_sequence("internal-order")
                .map_err(|error| format!("allocate discovered order identity: {error}"))?
        );
        let order = discovered_order(&internal_id, scope, snapshot, &now);
        let status = order.status.clone();
        let event_id = format!(
            "{}-discovered-{}",
            internal_id,
            self.store
                .next_sequence("order-event")
                .map_err(|error| format!("allocate discovered order event: {error}"))?
        );
        let payload_json = order_snapshot_payload(snapshot);
        let event = StoredExecutionOrderEvent {
            id: &event_id,
            internal_order_id: &internal_id,
            event_type: "BROKER_SYNC_DISCOVERED",
            previous_status: None,
            next_status: &status,
            payload_json: &payload_json,
            created_at: &now,
        };
        self.store
            .save_order_and_event(order, &now, &event)
            .map_err(|error| format!("persist discovered broker order: {error}"))?;
        Ok(true)
    }

    fn persist_discovered_fill(
        &self,
        scope: &ReconciliationScopeKey,
        fill: &TradeFillSnapshot,
    ) -> Result<(), String> {
        let now = crate::product::product_production_ports::provider_now_rfc3339();
        let internal_id = format!(
            "rust-broker-fill-{}",
            self.store
                .next_sequence("internal-order")
                .map_err(|error| format!("allocate discovered fill identity: {error}"))?
        );
        let order = discovered_fill(&internal_id, scope, fill, &now);
        let status = order.status.clone();
        let event_id = format!(
            "{}-fill-{}",
            internal_id,
            self.store
                .next_sequence("order-event")
                .map_err(|error| format!("allocate discovered fill event: {error}"))?
        );
        let payload_json = fill_snapshot_payload(fill);
        let event = StoredExecutionOrderEvent {
            id: &event_id,
            internal_order_id: &internal_id,
            event_type: "BROKER_FILL_RECEIVED",
            previous_status: None,
            next_status: &status,
            payload_json: &payload_json,
            created_at: &now,
        };
        self.store
            .save_order_and_event(order, &now, &event)
            .map_err(|error| format!("persist discovered broker fill: {error}"))?;
        Ok(())
    }

    pub(super) fn reconciliation_candidates(&self) -> Result<Vec<StoredExecutionOrder>, String> {
        let mut candidates = self
            .store
            .list_reconciliation_candidates()
            .map_err(|error| format!("list execution reconciliation candidates: {error}"))?;
        let known = candidates
            .iter()
            .map(|order| order.internal_order_id.clone())
            .collect::<HashSet<_>>();
        let terminal_fee_candidates = self
            .store
            .list_orders()
            .map_err(|error| format!("list terminal fee reconciliation candidates: {error}"))?
            .into_iter()
            .filter(|order| {
                !known.contains(&order.internal_order_id)
                    && is_terminal(&order.status)
                    && order.fees.is_none()
                    && order
                        .broker_order_id_ex
                        .as_deref()
                        .is_some_and(|value| !value.trim().is_empty())
            });
        candidates.extend(terminal_fee_candidates);
        candidates.sort_by(|left, right| {
            left.updated_at
                .cmp(&right.updated_at)
                .then_with(|| left.created_at.cmp(&right.created_at))
                .then_with(|| left.internal_order_id.cmp(&right.internal_order_id))
        });
        Ok(candidates)
    }
}
pub(super) fn reconciliation_scopes(
    accounts: &[jftrade_integration_futu::TradeAccountSnapshot],
) -> Vec<ReconciliationScopeKey> {
    let mut scopes = Vec::new();
    let mut seen = HashSet::new();
    for account in accounts {
        let Some(account_id) = account_identity(account) else {
            continue;
        };
        let Some(environment) = environment_label(account.trd_env) else {
            continue;
        };
        for trd_market in &account.trd_market_auth_list {
            let Some(market) = market_label(*trd_market) else {
                continue;
            };
            let key = ReconciliationScopeKey {
                account_id: account_id.clone(),
                environment: environment.to_owned(),
                market: market.to_owned(),
                trd_market: *trd_market,
            };
            if seen.insert(key.clone()) {
                scopes.push(key);
            }
        }
    }
    scopes
}

pub(super) fn read_order_scope(
    reader: &std::sync::Arc<dyn TradeReadPort>,
    header: &jftrade_integration_futu::TradeHeader,
    scope: &ReconciliationScopeKey,
) -> Result<Vec<TradeOrderSnapshot>, String> {
    let active = reader.read_orders(header.clone(), None, Vec::new(), Some(true));
    let history = reader.read_history_orders(header.clone(), None, Vec::new(), Some(true));
    let mut snapshots = match (active, history) {
        (Ok(mut active), Ok(history)) => {
            active.extend(history);
            active
        }
        (Ok(_), Err(history)) => {
            return Err(format!(
                "broker order discovery failed for {}/{} {}: history read failed: {history}",
                scope.account_id, scope.environment, scope.market
            ));
        }
        (Err(active), Ok(_)) => {
            return Err(format!(
                "broker order discovery failed for {}/{} {}: active read failed: {active}",
                scope.account_id, scope.environment, scope.market
            ));
        }
        (Err(active), Err(history)) => {
            return Err(format!(
                "broker order discovery failed for {}/{} {}: {active}; {history}",
                scope.account_id, scope.environment, scope.market
            ));
        }
    };
    snapshots.sort_by(|left, right| {
        order_identity(left)
            .cmp(&order_identity(right))
            .then_with(|| left.update_time.cmp(&right.update_time))
    });
    let mut deduplicated = Vec::with_capacity(snapshots.len());
    for snapshot in snapshots {
        if order_identity(&snapshot).is_none() || !order_matches_scope(&snapshot, scope) {
            continue;
        }
        if let Some(previous) = deduplicated
            .iter_mut()
            .find(|previous| order_identity(previous) == order_identity(&snapshot))
        {
            if time_after(&snapshot.update_time, &previous.update_time) {
                *previous = snapshot;
            }
        } else {
            deduplicated.push(snapshot);
        }
    }
    Ok(deduplicated)
}

pub(super) fn read_fill_scope(
    reader: &std::sync::Arc<dyn TradeReadPort>,
    header: &jftrade_integration_futu::TradeHeader,
    scope: &ReconciliationScopeKey,
) -> Result<Vec<TradeFillSnapshot>, String> {
    let active = reader.read_fills(header.clone(), None, Some(true));
    let history = reader.read_history_fills(header.clone(), None, Some(true));
    let fills = match (active, history) {
        (Ok(mut active), Ok(history)) => {
            active.extend(history);
            active
        }
        (Ok(_), Err(history)) => {
            return Err(format!(
                "broker fill discovery failed for {}/{} {}: history read failed: {history}",
                scope.account_id, scope.environment, scope.market
            ));
        }
        (Err(active), Ok(_)) => {
            return Err(format!(
                "broker fill discovery failed for {}/{} {}: active read failed: {active}",
                scope.account_id, scope.environment, scope.market
            ));
        }
        (Err(active), Err(history)) => {
            return Err(format!(
                "broker fill discovery failed for {}/{} {}: {active}; {history}",
                scope.account_id, scope.environment, scope.market
            ));
        }
    };
    deduplicate_fills(
        fills
            .into_iter()
            .filter(|fill| fill_matches_scope(fill, scope))
            .collect(),
    )
}

pub(super) fn deduplicate_fills(
    mut fills: Vec<TradeFillSnapshot>,
) -> Result<Vec<TradeFillSnapshot>, String> {
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

pub(super) fn account_identity(
    account: &jftrade_integration_futu::TradeAccountSnapshot,
) -> Option<String> {
    if account.acc_id > 0 {
        return Some(account.acc_id.to_string());
    }
    account
        .card_num
        .as_deref()
        .into_iter()
        .chain(account.uni_card_num.as_deref())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .filter(|value| value.parse::<u64>().ok().is_some_and(|id| id > 0))
        .map(ToOwned::to_owned)
        .next()
}

pub(super) fn environment_label(value: i32) -> Option<&'static str> {
    match value {
        0 => Some("SIMULATE"),
        1 => Some("REAL"),
        _ => None,
    }
}

pub(super) fn market_label(value: i32) -> Option<&'static str> {
    match value {
        // Reconciliation currently owns the stock order/fill contract only.
        // Do not collapse futures, funds, prediction, quote-market or other
        // enum values into a stock scope: doing so can query the wrong account
        // ledger and persist unrelated orders under HK/US/CN.
        1 => Some("HK"),
        2 => Some("US"),
        3 => Some("CN"),
        _ => None,
    }
}

pub(super) fn order_identity(snapshot: &TradeOrderSnapshot) -> Option<String> {
    if snapshot.order_id > 0 {
        Some(format!("id:{}", snapshot.order_id))
    } else if !snapshot.order_id_ex.trim().is_empty() {
        Some(format!("ex:{}", snapshot.order_id_ex.trim()))
    } else {
        None
    }
}

pub(super) fn fill_matches_scope(fill: &TradeFillSnapshot, scope: &ReconciliationScopeKey) -> bool {
    fill.trd_market
        .is_none_or(|market| market == scope.trd_market)
        && fill
            .sec_market
            .is_none_or(|market| security_market_matches(scope.trd_market, market))
}

pub(super) fn order_matches_scope(
    snapshot: &TradeOrderSnapshot,
    scope: &ReconciliationScopeKey,
) -> bool {
    snapshot
        .trd_market
        .is_none_or(|market| market == scope.trd_market)
        && snapshot
            .sec_market
            .is_none_or(|market| security_market_matches(scope.trd_market, market))
}

pub(super) fn security_market_matches(trd_market: i32, security_market: i32) -> bool {
    match trd_market {
        1 => security_market == 1,
        2 => security_market == 11,
        3 => matches!(security_market, 21 | 22),
        6 => security_market == 31,
        7 => security_market == 101,
        _ => true,
    }
}

pub(super) fn find_scoped_order(
    orders: &[StoredExecutionOrder],
    scope: &ReconciliationScopeKey,
    snapshot: &TradeOrderSnapshot,
) -> Option<StoredExecutionOrder> {
    orders.iter().find_map(|order| {
        if !same_scope(order, scope) {
            return None;
        }
        let numeric = snapshot.order_id > 0
            && order
                .broker_order_id
                .as_deref()
                .and_then(|value| value.trim().parse::<u64>().ok())
                == Some(snapshot.order_id);
        let extended = !snapshot.order_id_ex.trim().is_empty()
            && order.broker_order_id_ex.as_deref().is_some_and(|value| {
                value
                    .trim()
                    .eq_ignore_ascii_case(snapshot.order_id_ex.trim())
            });
        (numeric || extended).then(|| order.clone())
    })
}

pub(super) fn find_scoped_fill_order(
    orders: &[StoredExecutionOrder],
    scope: &ReconciliationScopeKey,
    fill: &TradeFillSnapshot,
) -> Option<StoredExecutionOrder> {
    orders.iter().find_map(|order| {
        if !same_scope(order, scope) {
            return None;
        }
        let numeric = fill.order_id.is_some_and(|id| id > 0)
            && order
                .broker_order_id
                .as_deref()
                .and_then(|value| value.trim().parse::<u64>().ok())
                == fill.order_id;
        let extended = fill.order_id_ex.as_deref().is_some_and(|id| {
            !id.trim().is_empty()
                && order
                    .broker_order_id_ex
                    .as_deref()
                    .is_some_and(|value| value.trim().eq_ignore_ascii_case(id.trim()))
        });
        (numeric || extended).then(|| order.clone())
    })
}

pub(super) fn same_scope(order: &StoredExecutionOrder, scope: &ReconciliationScopeKey) -> bool {
    order.broker_id.trim().eq_ignore_ascii_case("futu")
        && order.account_id.trim() == scope.account_id
        && order
            .trading_environment
            .trim()
            .eq_ignore_ascii_case(&scope.environment)
        && order.market.trim().eq_ignore_ascii_case(&scope.market)
}

pub(super) fn is_unknown_status_error(error: &ExecutionWritePortError) -> bool {
    matches!(
        error,
        ExecutionWritePortError::Failed { code, .. } if code == "BROKER_STATUS_UNKNOWN"
    )
}

pub(super) fn discovered_order(
    internal_id: &str,
    scope: &ReconciliationScopeKey,
    snapshot: &TradeOrderSnapshot,
    now: &str,
) -> StoredExecutionOrder {
    let broker_status =
        canonical_broker_status(super::super::order_status_label(snapshot.order_status));
    let status = if broker_status == OrderStatus::Unknown {
        "UNKNOWN".to_owned()
    } else {
        super::super::storage_status(broker_status, "UNKNOWN")
    };
    let unknown = broker_status == OrderStatus::Unknown;
    let create_time = valid_timestamp_or(snapshot.create_time.as_str(), now);
    let symbol = normalized_broker_symbol(&scope.market, &snapshot.code);
    let requested_quantity = finite_non_negative(snapshot.qty);
    let requested_price = snapshot
        .price
        .filter(|value| value.is_finite() && *value >= 0.0);
    let filled_quantity = snapshot
        .fill_qty
        .filter(|value| value.is_finite() && *value >= 0.0);
    let filled_average_price = snapshot
        .fill_avg_price
        .filter(|value| value.is_finite() && *value >= 0.0);
    let status_message = format!(
        "OpenD returned unknown order status {} for broker order {}",
        snapshot.order_status, snapshot.order_id
    );
    StoredExecutionOrder {
        internal_order_id: internal_id.to_owned(),
        broker_id: "futu".to_owned(),
        broker_order_id: (snapshot.order_id > 0).then(|| snapshot.order_id.to_string()),
        broker_order_id_ex: non_blank(snapshot.order_id_ex.as_str()),
        source: "broker-sync".to_owned(),
        source_detail: "futu external order discovery".to_owned(),
        trading_environment: scope.environment.clone(),
        account_id: scope.account_id.clone(),
        market: scope.market.clone(),
        symbol,
        side: Some(super::super::execution_order_parse::side_label(snapshot.trd_side).to_owned()),
        order_type: Some(
            super::super::execution_order_parse::order_type_label(snapshot.order_type).to_owned(),
        ),
        status,
        raw_broker_status: Some(snapshot.order_status.to_string()),
        requested_quantity,
        requested_price,
        filled_quantity,
        filled_average_price,
        remark: snapshot.remark.clone(),
        last_error: unknown.then_some(status_message),
        last_error_code: unknown.then_some("BROKER_STATUS_UNKNOWN".to_owned()),
        last_error_source: unknown.then_some("opend".to_owned()),
        submitted_at: Some(create_time.clone()),
        updated_at: now.to_owned(),
        created_at: create_time,
        order_kind: "single".to_owned(),
        product_class: "equity".to_owned(),
        quantity_mode: "quantity".to_owned(),
        client_order_id: None,
        preview_id: None,
        normalized_request: order_snapshot_payload(snapshot),
        requested_amount: snapshot
            .order_amount
            .filter(|value| value.is_finite() && *value >= 0.0),
        payout: None,
        fees: None,
    }
}

pub(super) fn discovered_fill(
    internal_id: &str,
    scope: &ReconciliationScopeKey,
    fill: &TradeFillSnapshot,
    now: &str,
) -> StoredExecutionOrder {
    let quantity = fill.qty.max(0.0);
    let symbol = normalized_broker_symbol(&scope.market, &fill.code);
    let create_time = valid_timestamp_or(fill.create_time.as_str(), now);
    StoredExecutionOrder {
        internal_order_id: internal_id.to_owned(),
        broker_id: "futu".to_owned(),
        broker_order_id: fill
            .order_id
            .filter(|value| *value > 0)
            .map(|value| value.to_string()),
        broker_order_id_ex: fill.order_id_ex.as_deref().and_then(non_blank),
        source: "broker-sync".to_owned(),
        source_detail: "futu external fill discovery".to_owned(),
        trading_environment: scope.environment.clone(),
        account_id: scope.account_id.clone(),
        market: scope.market.clone(),
        symbol,
        side: Some(super::super::execution_order_parse::side_label(fill.trd_side).to_owned()),
        order_type: None,
        status: "FILLED".to_owned(),
        raw_broker_status: Some("FILLED_ALL".to_owned()),
        requested_quantity: Some(quantity),
        requested_price: Some(fill.price),
        filled_quantity: Some(quantity),
        filled_average_price: Some(fill.price),
        remark: None,
        last_error: None,
        last_error_code: None,
        last_error_source: None,
        submitted_at: Some(create_time.clone()),
        updated_at: now.to_owned(),
        created_at: create_time,
        order_kind: "single".to_owned(),
        product_class: "equity".to_owned(),
        quantity_mode: "quantity".to_owned(),
        client_order_id: None,
        preview_id: None,
        normalized_request: fill_snapshot_payload(fill),
        requested_amount: None,
        payout: None,
        fees: None,
    }
}

pub(super) fn normalized_broker_symbol(market: &str, code: &str) -> Option<String> {
    let code = code.trim();
    if code.is_empty() {
        return None;
    }
    if code.contains('.') {
        Some(code.to_ascii_uppercase())
    } else {
        Some(format!(
            "{}.{}",
            market.trim().to_ascii_uppercase(),
            code.to_ascii_uppercase()
        ))
    }
}

pub(super) fn finite_non_negative(value: f64) -> Option<f64> {
    (value.is_finite() && value >= 0.0).then_some(value)
}

pub(super) fn non_blank(value: &str) -> Option<String> {
    let value = value.trim();
    (!value.is_empty()).then(|| value.to_owned())
}

pub(super) fn valid_timestamp_or(value: &str, fallback: &str) -> String {
    let value = value.trim();
    if time::OffsetDateTime::parse(value, &time::format_description::well_known::Rfc3339).is_ok() {
        value.to_owned()
    } else {
        fallback.to_owned()
    }
}

pub(super) fn order_snapshot_payload(snapshot: &TradeOrderSnapshot) -> String {
    json!({
        "kind": "external_order",
        "brokerOrderId": (snapshot.order_id > 0).then_some(snapshot.order_id),
        "brokerOrderIdEx": non_blank(&snapshot.order_id_ex),
        "brokerStatus": snapshot.order_status,
        "code": snapshot.code,
        "name": snapshot.name,
        "side": super::super::execution_order_parse::side_label(snapshot.trd_side),
        "orderType": super::super::execution_order_parse::order_type_label(snapshot.order_type),
        "requestedQuantity": finite_non_negative(snapshot.qty),
        "requestedPrice": snapshot.price.filter(|value| value.is_finite()),
        "filledQuantity": snapshot.fill_qty.filter(|value| value.is_finite()),
        "filledAveragePrice": snapshot.fill_avg_price.filter(|value| value.is_finite()),
        "createdAt": snapshot.create_time,
        "updatedAt": snapshot.update_time,
        "remark": snapshot.remark,
    })
    .to_string()
}

pub(super) fn fill_snapshot_payload(fill: &TradeFillSnapshot) -> String {
    json!({
        "kind": "external_fill",
        "fillIdentity": fill_identity(fill),
        "brokerFillId": (fill.fill_id > 0).then_some(fill.fill_id),
        "brokerFillIdEx": non_blank(&fill.fill_id_ex),
        "brokerOrderId": fill.order_id,
        "brokerOrderIdEx": fill.order_id_ex,
        "filledQuantity": fill.qty,
        "fillPrice": fill.price,
        "filledAt": fill.create_time,
        "code": fill.code,
    })
    .to_string()
}

pub(super) fn matches_order(
    snapshot: &TradeOrderSnapshot,
    id: Option<u64>,
    id_ex: Option<&str>,
) -> bool {
    let id_matches = id.is_some_and(|value| snapshot.order_id == value);
    let ex_matches = id_ex.is_some_and(|value| snapshot.order_id_ex.trim() == value);
    let id_consistent = id.is_none_or(|value| snapshot.order_id == value);
    let ex_consistent = id_ex.is_none_or(|value| {
        let actual = snapshot.order_id_ex.trim();
        actual.is_empty() || actual == value
    });
    (id_matches || ex_matches) && id_consistent && ex_consistent
}

pub(super) fn matches_fill(
    snapshot: &TradeFillSnapshot,
    id: Option<u64>,
    id_ex: Option<&str>,
) -> bool {
    let id_matches = id.is_some_and(|value| snapshot.order_id == Some(value));
    let ex_matches = id_ex.is_some_and(|value| {
        snapshot
            .order_id_ex
            .as_deref()
            .is_some_and(|actual| actual.trim() == value)
    });
    let id_consistent =
        id.is_none_or(|value| snapshot.order_id.is_none_or(|actual| actual == value));
    let ex_consistent = id_ex.is_none_or(|value| {
        snapshot
            .order_id_ex
            .as_deref()
            .is_none_or(|actual| actual.trim().is_empty() || actual.trim() == value)
    });
    (id_matches || ex_matches) && id_consistent && ex_consistent
}

pub(super) fn matches_fee(snapshot: &TradeOrderFeeSnapshot, order_id_ex: &str) -> bool {
    let expected = order_id_ex.trim();
    !expected.is_empty() && snapshot.broker_order_id_ex.trim() == expected
}

pub(super) fn fill_identity(fill: &TradeFillSnapshot) -> String {
    if !fill.fill_id_ex.trim().is_empty() {
        fill.fill_id_ex.trim().to_owned()
    } else {
        fill.fill_id.to_string()
    }
}

pub(super) fn equivalent_fill_observation(
    left: &TradeFillSnapshot,
    right: &TradeFillSnapshot,
) -> bool {
    left.order_id == right.order_id
        && left.order_id_ex.as_deref().map(str::trim) == right.order_id_ex.as_deref().map(str::trim)
        && left.code.trim().eq_ignore_ascii_case(right.code.trim())
        && left.qty.to_bits() == right.qty.to_bits()
        && left.price.to_bits() == right.price.to_bits()
        && left.create_time.trim() == right.create_time.trim()
}

pub(super) fn validate_fill_snapshot(
    fill: &TradeFillSnapshot,
) -> Result<(), ExecutionWritePortError> {
    if fill.fill_id == 0 && fill.fill_id_ex.trim().is_empty() {
        return Err(invalid_fill(
            "OpenD returned a fill without a broker fill identity",
        ));
    }
    if fill.order_id.is_none()
        && fill
            .order_id_ex
            .as_deref()
            .is_none_or(|value| value.trim().is_empty())
    {
        return Err(invalid_fill(
            "OpenD returned a fill without a broker order identity",
        ));
    }
    if !fill.qty.is_finite() || fill.qty <= 0.0 {
        return Err(invalid_fill("OpenD returned a non-positive fill quantity"));
    }
    if !fill.price.is_finite() || fill.price <= 0.0 {
        return Err(invalid_fill("OpenD returned a non-positive fill price"));
    }
    if fill.code.trim().is_empty() {
        return Err(invalid_fill(
            "OpenD returned a fill without a security code",
        ));
    }
    time::OffsetDateTime::parse(
        fill.create_time.trim(),
        &time::format_description::well_known::Rfc3339,
    )
    .map(|_| ())
    .map_err(|error| invalid_fill(format!("OpenD returned an invalid fill timestamp: {error}")))
}

pub(super) fn invalid_fill(message: impl Into<String>) -> ExecutionWritePortError {
    failed(502, "BROKER_INVALID_RESPONSE", message)
}

pub(super) fn fee_amount(fee: &TradeOrderFeeSnapshot) -> Option<f64> {
    fee.fee_amount.or_else(|| {
        (!fee.fee_items.is_empty()).then(|| fee.fee_items.iter().map(|item| item.value).sum())
    })
}

pub(super) fn finite_positive(value: f64) -> Option<f64> {
    (value.is_finite() && value > 0.0).then_some(value)
}

pub(super) fn covered_by_snapshot(
    events: &[jftrade_store_sqlite::StoredExecutionOrderEventRecord],
    fill: &TradeFillSnapshot,
) -> Result<f64, ExecutionWritePortError> {
    let fill_at = fill.create_time.trim();
    let mut best = 0.0;
    let mut best_at = String::new();
    let mut known = 0.0;
    for event in events {
        let payload = serde_json::from_str::<Value>(&event.payload_json).map_err(|error| {
            invalid_stored(format!(
                "stored order event {} has invalid JSON payload: {error}",
                event.id
            ))
        })?;
        if event.event_type == "BROKER_FILL_RECEIVED" {
            if let Some(at) = payload.get("filledAt").filter(|value| !value.is_null()) {
                let at = at.as_str().ok_or_else(|| {
                    invalid_stored(format!(
                        "stored fill event {} has invalid filledAt",
                        event.id
                    ))
                })?;
                if !at.is_empty() && !time_after(at, fill_at) {
                    let quantity = payload
                        .get("filledQuantity")
                        .and_then(Value::as_f64)
                        .ok_or_else(|| {
                            invalid_stored(format!(
                                "stored fill event {} has invalid filledQuantity",
                                event.id
                            ))
                        })?;
                    if !quantity.is_finite() || quantity < 0.0 {
                        return Err(invalid_stored(format!(
                            "stored fill event {} has an invalid filledQuantity",
                            event.id
                        )));
                    }
                    known += quantity;
                }
            }
            continue;
        }
        let Some(quantity_value) = payload
            .get("filledQuantity")
            .filter(|value| !value.is_null())
        else {
            continue;
        };
        let quantity = quantity_value.as_f64().ok_or_else(|| {
            invalid_stored(format!(
                "stored order event {} has invalid filledQuantity",
                event.id
            ))
        })?;
        if !quantity.is_finite() || quantity < 0.0 {
            return Err(invalid_stored(format!(
                "stored order event {} has an invalid filledQuantity",
                event.id
            )));
        }
        let at = payload
            .get("updatedAt")
            .filter(|value| !value.is_null())
            .map_or(Ok(event.created_at.as_str()), |value| {
                value.as_str().ok_or_else(|| {
                    invalid_stored(format!(
                        "stored order event {} has invalid updatedAt",
                        event.id
                    ))
                })
            })?;
        if quantity > best && !time_after(fill_at, at) {
            best = quantity;
            best_at = at.to_owned();
        }
    }
    if best <= 0.0 || best_at.is_empty() {
        return Ok(0.0);
    }
    Ok((best - known).max(0.0).min(fill.qty))
}

pub(super) fn invalid_stored(message: impl Into<String>) -> ExecutionWritePortError {
    failed(500, "EXECUTION_ORDER_DATA_INVALID", message)
}
