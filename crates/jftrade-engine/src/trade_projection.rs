//! JSON projections and enum mappings for Futu trade reads.

use jftrade_integration_futu::{
    TradeFundsSnapshot, TradeMarginRatioSnapshot, TradeMaxTradeQuantitySnapshot,
    TradeOrderFeeSnapshot, TradeSessionError,
};
use serde_json::{Value, json};

use super::{ResolvedTradeRequest, account_identity, checked_at, environment_label_from_code};
use crate::product::{BrokerReadSnapshotError, PortfolioSnapshotError};

/// Projects the discovered accounts exactly like Go's
/// `runtimeAccountsFromProto`: deduplicate on `AccountID|TradingEnvironment`,
/// then sort by environment and account id.
///
/// Go keeps `runtimeAccountsFromProto` as the single owner of this rule, so the
/// runtime projection must not reorder or repeat rows: a duplicate simulate
/// card would otherwise be reported twice with different enum projections.
pub(crate) fn accounts_value(
    accounts: Vec<jftrade_integration_futu::TradeAccountSnapshot>,
) -> Vec<Value> {
    let mut seen = std::collections::HashSet::new();
    let mut projected = accounts
        .into_iter()
        .filter_map(|account| {
            let identity = account_identity(&account).unwrap_or_default();
            let environment = environment_label_from_code(account.trd_env);
            if !seen.insert(format!("{identity}|{environment}")) {
                return None;
            }
            Some((environment.to_owned(), identity, account_value(account)))
        })
        .collect::<Vec<_>>();
    projected.sort_by(|left, right| {
        left.0
            .cmp(&right.0)
            .then_with(|| left.1.cmp(&right.1))
    });
    projected.into_iter().map(|(_, _, value)| value).collect()
}

pub(crate) fn account_value(value: jftrade_integration_futu::TradeAccountSnapshot) -> Value {
    let account_id = account_identity(&value);
    let markets = value
        .trd_market_auth_list
        .iter()
        .filter_map(|market| trade_market_authority(*market))
        .fold(Vec::new(), |mut result, market| {
            if !result.contains(&market) {
                result.push(market);
            }
            result
        });
    json!({
        "tradingEnvironment": environment_label_from_code(value.trd_env),
        "accountId": account_id.unwrap_or_default(),
        "accountType": value.acc_type.map(account_type_label).unwrap_or("UNKNOWN"),
        "accountRole": value.acc_role.and_then(account_role_label),
        "securityFirm": value.security_firm.and_then(security_firm_label),
        "marketAuthorities": markets,
        "simulatedAccountType": value.sim_acc_type.and_then(simulated_account_type_label),
    })
}

pub(super) fn funds_value(request: &ResolvedTradeRequest, value: TradeFundsSnapshot) -> Value {
    let balances = value.funds.cash_info_list.iter().map(|cash| json!({"accountId": request.account_id, "tradingEnvironment": request.environment, "currency": currency_label(cash.currency), "cash": cash.cash, "availableWithdrawalCash": cash.available_balance, "netCashPower": cash.net_cash_power})).collect::<Vec<_>>();
    let assets = value
        .funds
        .market_info_list
        .iter()
        .map(|item| {
            json!({
                "accountId": request.account_id,
                "tradingEnvironment": request.environment,
                "market": market_label_from_code(item.trd_market).unwrap_or(request.market.as_str()),
                "assets": item.assets,
            })
        })
        .collect::<Vec<_>>();
    // Parity: `go:452dea11:pkg/futu/trade_read_proto.go` + `internal/trading/responses.go`.
    // Go's `brokerFundsSnapshotFromProto` maps these protobuf enums through
    // `optionalEnumStringPtr` (enum-name suffix, upper-cased, UNKNOWN -> nil),
    // and the wire contract is `trading.BrokerFundsSummary` in
    // `contracts/openapi/openapi.json`: `power` is named `purchasingPower`,
    // `maxPowerShort` is `shortSellingPower`, and `currency`/`riskStatus`/
    // `dtStatus`/`exposureLevel` are strings, not raw integers. The console
    // (`AccountAssetStrip.vue`, `AccountMoreSection.vue`) reads those names.
    let funds = &value.funds;
    let mut summary = serde_json::Map::with_capacity(37);
    summary.insert("accountId".to_owned(), json!(request.account_id));
    summary.insert(
        "tradingEnvironment".to_owned(),
        json!(request.environment),
    );
    summary.insert("market".to_owned(), json!(request.market));
    summary.insert(
        "currency".to_owned(),
        json!(currency_label(funds.currency)),
    );
    summary.insert("totalAssets".to_owned(), json!(funds.total_assets));
    summary.insert("securitiesAssets".to_owned(), json!(funds.securities_assets));
    summary.insert("fundAssets".to_owned(), json!(funds.fund_assets));
    summary.insert("bondAssets".to_owned(), json!(funds.bond_assets));
    summary.insert("cash".to_owned(), json!(funds.cash));
    summary.insert("marketValue".to_owned(), json!(funds.market_val));
    summary.insert("longMarketValue".to_owned(), json!(funds.long_mv));
    summary.insert("shortMarketValue".to_owned(), json!(funds.short_mv));
    summary.insert("purchasingPower".to_owned(), json!(funds.power));
    summary.insert(
        "shortSellingPower".to_owned(),
        json!(funds.max_power_short),
    );
    summary.insert("netCashPower".to_owned(), json!(funds.net_cash_power));
    summary.insert(
        "availableWithdrawalCash".to_owned(),
        json!(funds.avl_withdrawal_cash),
    );
    summary.insert("maxWithdrawal".to_owned(), json!(funds.max_withdrawal));
    summary.insert("availableFunds".to_owned(), json!(funds.available_funds));
    summary.insert("frozenCash".to_owned(), json!(funds.frozen_cash));
    summary.insert("pendingAsset".to_owned(), json!(funds.pending_asset));
    summary.insert("unrealizedPnl".to_owned(), json!(funds.unrealized_pl));
    summary.insert("realizedPnl".to_owned(), json!(funds.realized_pl));
    summary.insert("initialMargin".to_owned(), json!(funds.initial_margin));
    summary.insert(
        "maintenanceMargin".to_owned(),
        json!(funds.maintenance_margin),
    );
    summary.insert(
        "marginCallMargin".to_owned(),
        json!(funds.margin_call_margin),
    );
    summary.insert(
        "riskStatus".to_owned(),
        json!(risk_status_label(funds.risk_status)),
    );
    summary.insert("debtCash".to_owned(), json!(funds.debt_cash));
    summary.insert("isPdt".to_owned(), json!(funds.is_pdt));
    summary.insert("pdtSeq".to_owned(), json!(funds.pdt_seq));
    summary.insert("beginningDTBP".to_owned(), json!(funds.beginning_dtbp));
    summary.insert("remainingDTBP".to_owned(), json!(funds.remaining_dtbp));
    summary.insert("dtCallAmount".to_owned(), json!(funds.dt_call_amount));
    summary.insert(
        "dtStatus".to_owned(),
        json!(dt_status_label(funds.dt_status)),
    );
    summary.insert(
        "exposureLevel".to_owned(),
        json!(exposure_level_label(funds.exposure_level)),
    );
    summary.insert("exposureLimit".to_owned(), json!(funds.exposure_limit));
    summary.insert("usedLimit".to_owned(), json!(funds.used_limit));
    summary.insert("remainingLimit".to_owned(), json!(funds.remaining_limit));
    json!({
        "checkedAt": checked_at(),
        "connectivity": "connected",
        "currencyBalances": balances,
        // `trading.BrokerFundsResponse` requires `lastError` on every funds
        // response: Go's typed DTO marshals a nil error as JSON null, and the
        // console distinguishes "connected, no error" from a missing key.
        "lastError": Value::Null,
        "marketAssets": assets,
        "summary": summary,
    })
}

/// Parity: `go:452dea11:pkg/futu/trade_read_helpers.go:116`
/// `optionalEnumStringPtr` + `enumName` + `normalizeRuntimeEnum`. The protobuf
/// enum name keeps only the suffix after the first `_` and upper-cases it, so
/// `CltRiskStatus_Level1` becomes `LEVEL1`; `Unknown` (code 0) maps to nil.
pub(super) fn risk_status_label(value: Option<i32>) -> Option<&'static str> {
    match value {
        Some(1) => Some("LEVEL1"),
        Some(2) => Some("LEVEL2"),
        Some(3) => Some("LEVEL3"),
        Some(4) => Some("LEVEL4"),
        Some(5) => Some("LEVEL5"),
        Some(6) => Some("LEVEL6"),
        Some(7) => Some("LEVEL7"),
        Some(8) => Some("LEVEL8"),
        Some(9) => Some("LEVEL9"),
        _ => None,
    }
}

/// Parity: `go:452dea11:pkg/futu/trade_read_proto.go:61` `DTStatus_name`.
pub(super) fn dt_status_label(value: Option<i32>) -> Option<&'static str> {
    match value {
        Some(1) => Some("UNLIMITED"),
        Some(2) => Some("EMCALL"),
        Some(3) => Some("DTCALL"),
        _ => None,
    }
}

/// Parity: `go:452dea11:pkg/futu/trade_read_proto.go:62`
/// `ExposureLevel_name`.
pub(super) fn exposure_level_label(value: Option<i32>) -> Option<&'static str> {
    match value {
        Some(1) => Some("NORMAL"),
        Some(2) => Some("NEARLIMIT"),
        Some(3) => Some("RESTRICTED"),
        Some(4) => Some("SAFE"),
        Some(5) => Some("MODERATE"),
        Some(6) => Some("WARNING"),
        Some(7) => Some("MARGINCALL"),
        _ => None,
    }
}

pub(crate) fn position_value(
    request: &ResolvedTradeRequest,
    value: jftrade_integration_futu::TradePositionSnapshot,
) -> Value {
    // Parity: `go:452dea11:pkg/futu/trade_read_proto.go:146`
    // `brokerPositionSnapshotFromProto` reads each preferred/fallback pair
    // through `preferredFloat64Ptr`, so OpenD's account-level dilution and
    // average-cost fields win over the legacy per-position values:
    // `dilutedCostPrice` > `costPrice`, `unrealizedPL` > `plVal`, and
    // `averagePlRatio` > `plRatio`.  Projecting only the fallback side drops
    // the authoritative security-account cost basis and PnL the console reads.
    let cost_price = value.diluted_cost_price.or(value.cost_price);
    let unrealized_pnl = value.unrealized_pl.or(Some(value.pl_val));
    let pnl_ratio = value.average_pl_ratio.or(value.pl_ratio);
    let market = market_label_from_code(value.trd_market).unwrap_or(request.market.as_str());
    json!({"accountId": request.account_id, "tradingEnvironment": request.environment, "market": market, "symbol": qualify_symbol(market, &value.code), "symbolName": non_empty(&value.name), "quantity": value.qty, "sellableQuantity": value.can_sell_qty, "lastPrice": value.price, "costPrice": cost_price, "averageCostPrice": value.average_cost_price, "marketValue": value.val, "unrealizedPnl": unrealized_pnl, "realizedPnl": value.realized_pl, "pnlRatio": pnl_ratio, "currency": currency_label(value.currency)})
}

pub(crate) fn portfolio_position_value(
    broker_id: &str,
    request: &ResolvedTradeRequest,
    position: jftrade_integration_futu::TradePositionSnapshot,
) -> Value {
    let mut value = position_value(request, position);
    value["brokerId"] = json!(broker_id);
    value["averagePrice"] = if value["averageCostPrice"].is_null() {
        json!(value["costPrice"].as_f64().unwrap_or(0.0))
    } else {
        value["averageCostPrice"].clone()
    };
    let timestamp = checked_at();
    value["createdAt"] = json!(timestamp);
    value["updatedAt"] = value["createdAt"].clone();
    value
}

pub(super) fn order_value(
    request: &ResolvedTradeRequest,
    value: jftrade_integration_futu::TradeOrderSnapshot,
) -> Value {
    json!({"accountId": request.account_id, "brokerOrderId": value.order_id.to_string(), "brokerOrderIdEx": non_empty(&value.order_id_ex), "currency": currency_label(value.currency), "filledAveragePrice": value.fill_avg_price, "filledQuantity": value.fill_qty, "lastError": value.last_err_msg, "market": request.market, "orderType": order_type_label(value.order_type), "price": value.price, "quantity": value.qty, "remark": value.remark, "side": trade_side(value.trd_side), "status": order_status_label(value.order_status), "submittedAt": canonical_time(&value.create_time), "symbol": qualify_symbol(&request.market, &value.code), "symbolName": non_empty(&value.name), "timeInForce": value.time_in_force.map(time_in_force_label), "tradingEnvironment": request.environment, "updatedAt": canonical_time(&value.update_time)})
}

/// Projects broker orders exactly like Go's `brokerOrderSnapshotsFromProto`.
///
/// Go filters on the canonical symbol (`strings.TrimSpace(strings.ToUpper)`)
/// with a case-insensitive compare against each row's `code`, then sorts by
/// descending update time with the broker order id as the tie-breaker. The
/// filter is applied here as well because the provider request is only a hint:
/// rows for another instrument can still come back.
pub(super) fn orders_value(
    request: &ResolvedTradeRequest,
    orders: Vec<jftrade_integration_futu::TradeOrderSnapshot>,
) -> Vec<Value> {
    let canonical_symbol = request
        .order_symbol_filter
        .as_deref()
        .map(|symbol| symbol.trim().to_ascii_uppercase())
        .filter(|symbol| !symbol.is_empty());
    let mut orders = orders
        .into_iter()
        .filter(|order| {
            canonical_symbol
                .as_deref()
                .is_none_or(|symbol| order.code.trim().eq_ignore_ascii_case(symbol))
        })
        .collect::<Vec<_>>();
    orders.sort_by(|left, right| {
        let left_key = order_sort_key(left);
        let right_key = order_sort_key(right);
        right_key
            .cmp(&left_key)
            .then_with(|| right.order_id.cmp(&left.order_id))
    });
    orders
        .into_iter()
        .map(|order| order_value(request, order))
        .collect()
}

/// Go `brokerOrderSortKey`: the update time, falling back to the submit time
/// only when the update time cannot be parsed.
fn order_sort_key(order: &jftrade_integration_futu::TradeOrderSnapshot) -> (i64, String) {
    let updated = parse_broker_order_time(&order.update_time);
    if updated.0 != 0 || !updated.1.is_empty() {
        return updated;
    }
    parse_broker_order_time(&order.create_time)
}

/// Go `parseBrokerOrderTime`: normalize the wire label to a sortable instant.
/// Unparsable labels keep their trimmed text so ordering stays deterministic
/// instead of silently collapsing to the epoch.
fn parse_broker_order_time(value: &str) -> (i64, String) {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        return (0, String::new());
    }
    let canonical =
        crate::product::product_production_ports::product_production_ports_trade::canonical_candle_time(trimmed, "UTC");
    match time::OffsetDateTime::parse(
        &canonical,
        &time::format_description::well_known::Rfc3339,
    ) {
        Ok(parsed) => (parsed.unix_timestamp(), String::new()),
        Err(_) => (0, trimmed.to_owned()),
    }
}

pub(super) fn fill_value(
    request: &ResolvedTradeRequest,
    value: jftrade_integration_futu::TradeFillSnapshot,
) -> Value {
    json!({"accountId": request.account_id, "brokerFillId": value.fill_id.to_string(), "brokerFillIdEx": non_empty(&value.fill_id_ex), "brokerOrderId": value.order_id.map(|v| v.to_string()).unwrap_or_default(), "brokerOrderIdEx": value.order_id_ex, "fillPrice": value.price, "filledAt": canonical_time(&value.create_time), "filledQuantity": value.qty, "market": request.market, "side": trade_side(value.trd_side), "status": value.status.map(fill_status_label), "symbol": qualify_symbol(&request.market, &value.code), "symbolName": non_empty(&value.name), "tradingEnvironment": request.environment})
}

pub(super) fn order_fee_value(
    request: &ResolvedTradeRequest,
    value: TradeOrderFeeSnapshot,
) -> Value {
    let fee_amount = value.fee_amount;
    let fee_items = value
        .fee_items
        .into_iter()
        .map(|item| json!({"title": item.title, "value": item.value}))
        .collect::<Vec<_>>();
    let mut output = json!({
        "accountId": request.account_id,
        "tradingEnvironment": request.environment,
        "market": request.market,
        "brokerOrderIdEx": value.broker_order_id_ex,
        "feeItems": fee_items,
    });
    if let Some(amount) = fee_amount {
        output["feeAmount"] = json!(amount);
    } else {
        output
            .as_object_mut()
            .expect("fee object")
            .remove("feeAmount");
    }
    if output["feeItems"].as_array().is_some_and(Vec::is_empty) {
        output
            .as_object_mut()
            .expect("fee object")
            .remove("feeItems");
    }
    output
}

pub(super) fn margin_ratio_value(
    request: &ResolvedTradeRequest,
    value: TradeMarginRatioSnapshot,
) -> Value {
    let mut output = json!({
        "accountId": request.account_id,
        "tradingEnvironment": request.environment,
        "market": if value.market.is_empty() { request.market.clone() } else { value.market },
        "symbol": value.symbol,
    });
    let object = output.as_object_mut().expect("margin ratio object");
    macro_rules! optional {
        ($name:literal, $value:expr) => {
            if let Some(value) = $value {
                object.insert($name.to_owned(), json!(value));
            }
        };
    }
    optional!("isLongPermit", value.is_long_permit);
    optional!("isShortPermit", value.is_short_permit);
    optional!("shortPoolRemain", value.short_pool_remain);
    optional!("shortFeeRate", value.short_fee_rate);
    optional!("alertLongRatio", value.alert_long_ratio);
    optional!("alertShortRatio", value.alert_short_ratio);
    optional!("initialMarginLongRatio", value.initial_margin_long_ratio);
    optional!("initialMarginShortRatio", value.initial_margin_short_ratio);
    optional!("marginCallLongRatio", value.margin_call_long_ratio);
    optional!("marginCallShortRatio", value.margin_call_short_ratio);
    optional!("maintenanceLongRatio", value.maintenance_long_ratio);
    optional!("maintenanceShortRatio", value.maintenance_short_ratio);
    output
}

pub(super) fn max_trade_quantity_value(
    request: &ResolvedTradeRequest,
    value: TradeMaxTradeQuantitySnapshot,
    symbol_market: &str,
) -> Value {
    let mut output = json!({
        "accountId": request.account_id,
        "tradingEnvironment": request.environment,
        "market": request.market,
        "symbol": qualify_symbol(symbol_market, &value.code),
        "orderType": max_trade_order_type_label(value.order_type),
        "price": value.price,
        "maxCashBuy": value.max_cash_buy,
        "maxPositionSell": value.max_position_sell,
    });
    let object = output.as_object_mut().expect("max quantity object");
    macro_rules! optional {
        ($name:literal, $value:expr) => {
            if let Some(value) = $value {
                object.insert($name.to_owned(), json!(value));
            }
        };
    }
    optional!("maxCashAndMarginBuy", value.max_cash_and_margin_buy);
    optional!("maxSellShort", value.max_sell_short);
    optional!("maxBuyBack", value.max_buy_back);
    optional!("longRequiredIm", value.long_required_im);
    optional!("shortRequiredIm", value.short_required_im);
    if let Some(session) = value.session.and_then(session_label) {
        object.insert("session".to_owned(), json!(session));
    }
    output
}

pub(super) fn cash_flow_value(
    request: &ResolvedTradeRequest,
    value: jftrade_integration_futu::TradeCashFlowSnapshot,
) -> Value {
    json!({
        "accountId": request.account_id,
        "tradingEnvironment": request.environment,
        "market": request.market,
        "cashFlowId": value.cash_flow_id.map(|id| id.to_string()),
        "clearingDate": value.clearing_date,
        "settlementDate": value.settlement_date,
        "currency": currency_label(value.currency),
        "cashFlowType": value.cash_flow_type,
        "cashFlowDirection": value.cash_flow_direction.map(cash_flow_direction_label),
        "cashFlowAmount": value.cash_flow_amount,
        "cashFlowRemark": value.cash_flow_remark,
    })
}

pub(super) fn cash_flow_direction_label(value: i32) -> &'static str {
    match value {
        1 => "IN",
        2 => "OUT",
        _ => "UNKNOWN",
    }
}

pub(super) fn qualify_symbol(market: &str, code: &str) -> String {
    if code.contains('.') || market.is_empty() {
        code.to_owned()
    } else {
        format!("{market}.{code}")
    }
}

pub(super) fn non_empty(value: &str) -> Option<&str> {
    (!value.trim().is_empty()).then_some(value)
}

pub(super) fn currency_label(currency: Option<i32>) -> Option<&'static str> {
    match currency {
        Some(1) => Some("HKD"),
        Some(2) => Some("USD"),
        Some(3) => Some("CNH"),
        Some(4) => Some("JPY"),
        Some(5) => Some("SGD"),
        Some(6) => Some("AUD"),
        Some(7) => Some("CAD"),
        Some(8) => Some("MYR"),
        Some(9) => Some("NZD"),
        _ => None,
    }
}

pub(super) fn market_label_from_code(market: Option<i32>) -> Option<&'static str> {
    match market {
        Some(1 | 4 | 10 | 113) => Some("HK"),
        Some(2 | 11 | 123 | 17) => Some("US"),
        Some(3) => Some("CN"),
        Some(5) => Some("FUTURES"),
        Some(6 | 12 | 124) => Some("SG"),
        Some(7) => Some("CRYPTO"),
        Some(8) => Some("AU"),
        Some(13 | 15 | 126) => Some("JP"),
        Some(111 | 125) => Some("MY"),
        Some(112) => Some("CA"),
        _ => None,
    }
}

pub(crate) fn trade_market_authority(value: i32) -> Option<&'static str> {
    market_label_from_code(Some(value))
}

pub(super) fn trade_side(side: i32) -> &'static str {
    match side {
        1 => "BUY",
        2 => "SELL",
        3 => "SELLSHORT",
        4 => "BUYBACK",
        _ => "UNKNOWN",
    }
}

pub(super) fn account_type_label(value: i32) -> &'static str {
    match value {
        1 => "CASH",
        2 => "MARGIN",
        3 => "TFSA",
        4 => "RRSP",
        5 => "SRRSP",
        6 => "DERIVATIVES",
        _ => "UNKNOWN",
    }
}

pub(super) fn account_role_label(value: i32) -> Option<&'static str> {
    match value {
        1 => Some("NORMAL"),
        2 => Some("MASTER"),
        3 => Some("IPO"),
        _ => None,
    }
}

pub(crate) fn security_firm_label(value: i32) -> Option<&'static str> {
    match value {
        1 => Some("FUTUSECURITIES"),
        2 => Some("FUTUINC"),
        3 => Some("FUTUSG"),
        4 => Some("FUTUAU"),
        5 => Some("FUTUCA"),
        6 => Some("FUTUMY"),
        7 => Some("FUTUJP"),
        _ => None,
    }
}

pub(super) fn simulated_account_type_label(value: i32) -> Option<&'static str> {
    match value {
        1 => Some("STOCK"),
        2 => Some("OPTION"),
        3 => Some("FUTURES"),
        4 => Some("STOCKANDOPTION"),
        5 => Some("COMPETITION"),
        _ => None,
    }
}

pub(super) fn order_type_label(value: i32) -> &'static str {
    match value {
        1 => "NORMAL",
        2 => "MARKET",
        5 => "ABSOLUTELIMIT",
        6 => "AUCTION",
        7 => "AUCTIONLIMIT",
        8 => "SPECIALLIMIT",
        9 => "SPECIALLIMIT_ALL",
        10 => "STOP",
        11 => "STOPLIMIT",
        12 => "MARKETIFTOUCHED",
        13 => "LIMITIFTOUCHED",
        14 => "TRAILINGSTOP",
        15 => "TRAILINGSTOPLIMIT",
        16 => "TWAP_MARKET",
        17 => "TWAP_LIMIT",
        18 => "VWAP_MARKET",
        19 => "VWAP_LIMIT",
        _ => "UNKNOWN",
    }
}

pub(super) fn max_trade_order_type_label(value: i32) -> &'static str {
    match value {
        1 => "LIMIT",
        2 => "MARKET",
        10 => "STOP",
        11 => "STOP_LIMIT",
        12 => "TAKE_PROFIT_MARKET",
        13 => "TAKE_PROFIT",
        _ => "UNKNOWN",
    }
}

pub(super) fn session_label(value: i32) -> Option<&'static str> {
    match value {
        0 => Some("NONE"),
        1 => Some("RTH"),
        2 => Some("ETH"),
        3 => Some("ALL"),
        4 => Some("OVERNIGHT"),
        _ => None,
    }
}

pub(super) fn order_status_label(value: i32) -> &'static str {
    match value {
        -1 => "UNKNOWN",
        0 => "UNSUBMITTED",
        1 => "WAITINGSUBMIT",
        2 => "SUBMITTING",
        3 => "SUBMITFAILED",
        4 => "TIMEOUT",
        5 => "SUBMITTED",
        10 => "FILLED_PART",
        11 => "FILLED_ALL",
        12 => "CANCELLING_PART",
        13 => "CANCELLING_ALL",
        14 => "CANCELLED_PART",
        15 => "CANCELLED_ALL",
        21 => "FAILED",
        22 => "DISABLED",
        23 => "DELETED",
        24 => "FILLCANCELLED",
        _ => "UNKNOWN",
    }
}

/// Returns whether an OpenD order status is still working.
///
/// Parity: `go:452dea11:pkg/futu/trade_read_convert.go:160`
/// `brokerOrderIsWorking`. Filled, cancelled, rejected, disabled and deleted
/// orders are terminal; every other status (including unknown future values)
/// stays visible as working.
pub(super) fn active_order_status(value: i32) -> bool {
    !matches!(value, 3 | 11 | 14 | 15 | 21 | 22 | 23 | 24)
}

pub(super) fn fill_status_label(value: i32) -> &'static str {
    match value {
        0 => "OK",
        1 => "CANCELLED",
        2 => "CHANGED",
        3 => "PAYOUT",
        _ => "UNKNOWN",
    }
}

pub(super) fn time_in_force_label(value: i32) -> &'static str {
    match value {
        0 => "DAY",
        1 => "GTC",
        2 => "IOC",
        3 => "GTD",
        _ => "UNKNOWN",
    }
}

pub(super) fn canonical_time(value: &str) -> &str {
    value.trim()
}

pub(super) fn session_error(error: TradeSessionError) -> BrokerReadSnapshotError {
    unavailable(error.to_string())
}

pub(super) fn unavailable(message: impl Into<String>) -> BrokerReadSnapshotError {
    BrokerReadSnapshotError::Unavailable(message.into())
}

pub(super) fn unavailable_portfolio(message: impl Into<String>) -> PortfolioSnapshotError {
    PortfolioSnapshotError::Unavailable(message.into())
}

pub(super) fn map_broker_header_error(message: String) -> BrokerReadSnapshotError {
    if message == "accountId is required" {
        unavailable(message)
    } else {
        BrokerReadSnapshotError::Invalid(message)
    }
}

pub(super) fn map_portfolio_header_error(message: String) -> PortfolioSnapshotError {
    unavailable_portfolio(message)
}
