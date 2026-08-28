//! JSON projections and enum mappings for Futu trade reads.

use jftrade_integration_futu::{
    TradeFundsSnapshot, TradeMarginRatioSnapshot, TradeMaxTradeQuantitySnapshot,
    TradeOrderFeeSnapshot, TradeSessionError,
};
use serde_json::{Value, json};

use super::{ResolvedTradeRequest, account_identity, checked_at, environment_label_from_code};
use crate::product::{BrokerReadSnapshotError, PortfolioSnapshotError};

pub(super) fn account_value(value: jftrade_integration_futu::TradeAccountSnapshot) -> Value {
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
    json!({"checkedAt": checked_at(), "connectivity": "connected", "currencyBalances": balances, "marketAssets": assets, "summary": {"accountId": request.account_id, "tradingEnvironment": request.environment, "market": request.market, "power": value.funds.power, "totalAssets": value.funds.total_assets, "cash": value.funds.cash, "marketValue": value.funds.market_val, "frozenCash": value.funds.frozen_cash, "debtCash": value.funds.debt_cash, "availableWithdrawalCash": value.funds.avl_withdrawal_cash, "currency": value.funds.currency, "availableFunds": value.funds.available_funds, "unrealizedPnl": value.funds.unrealized_pl, "realizedPnl": value.funds.realized_pl, "securitiesAssets": value.funds.securities_assets, "fundAssets": value.funds.fund_assets, "bondAssets": value.funds.bond_assets, "longMarketValue": value.funds.long_mv, "shortMarketValue": value.funds.short_mv, "netCashPower": value.funds.net_cash_power, "maxWithdrawal": value.funds.max_withdrawal, "pendingAsset": value.funds.pending_asset, "initialMargin": value.funds.initial_margin, "maintenanceMargin": value.funds.maintenance_margin, "marginCallMargin": value.funds.margin_call_margin, "isPdt": value.funds.is_pdt, "pdtSeq": value.funds.pdt_seq, "beginningDTBP": value.funds.beginning_dtbp, "remainingDTBP": value.funds.remaining_dtbp, "dtCallAmount": value.funds.dt_call_amount, "exposureLevel": value.funds.exposure_level, "exposureLimit": value.funds.exposure_limit, "usedLimit": value.funds.used_limit, "remainingLimit": value.funds.remaining_limit}})
}

pub(super) fn position_value(request: &ResolvedTradeRequest, value: jftrade_integration_futu::TradePositionSnapshot) -> Value {
    json!({"accountId": request.account_id, "tradingEnvironment": request.environment, "market": request.market, "symbol": qualify_symbol(&request.market, &value.code), "symbolName": non_empty(&value.name), "quantity": value.qty, "sellableQuantity": value.can_sell_qty, "lastPrice": value.price, "costPrice": value.cost_price, "averageCostPrice": value.average_cost_price, "marketValue": value.val, "unrealizedPnl": value.unrealized_pl, "realizedPnl": value.realized_pl, "pnlRatio": value.pl_ratio, "currency": currency_label(value.currency)})
}

pub(super) fn order_value(request: &ResolvedTradeRequest, value: jftrade_integration_futu::TradeOrderSnapshot) -> Value {
    json!({"accountId": request.account_id, "brokerOrderId": value.order_id.to_string(), "brokerOrderIdEx": non_empty(&value.order_id_ex), "currency": currency_label(value.currency), "filledAveragePrice": value.fill_avg_price, "filledQuantity": value.fill_qty, "lastError": value.last_err_msg, "market": request.market, "orderType": order_type_label(value.order_type), "price": value.price, "quantity": value.qty, "remark": value.remark, "side": trade_side(value.trd_side), "status": order_status_label(value.order_status), "submittedAt": canonical_time(&value.create_time), "symbol": qualify_symbol(&request.market, &value.code), "symbolName": non_empty(&value.name), "timeInForce": value.time_in_force.map(time_in_force_label), "tradingEnvironment": request.environment, "updatedAt": canonical_time(&value.update_time)})
}

pub(super) fn fill_value(request: &ResolvedTradeRequest, value: jftrade_integration_futu::TradeFillSnapshot) -> Value {
    json!({"accountId": request.account_id, "brokerFillId": value.fill_id.to_string(), "brokerFillIdEx": non_empty(&value.fill_id_ex), "brokerOrderId": value.order_id.map(|v| v.to_string()).unwrap_or_default(), "brokerOrderIdEx": value.order_id_ex, "fillPrice": value.price, "filledAt": canonical_time(&value.create_time), "filledQuantity": value.qty, "market": request.market, "side": trade_side(value.trd_side), "status": value.status.map(fill_status_label), "symbol": qualify_symbol(&request.market, &value.code), "symbolName": non_empty(&value.name), "tradingEnvironment": request.environment})
}

pub(super) fn order_fee_value(request: &ResolvedTradeRequest, value: TradeOrderFeeSnapshot) -> Value {
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
    })
    ;
    if let Some(amount) = fee_amount {
        output["feeAmount"] = json!(amount);
    } else {
        output.as_object_mut().expect("fee object").remove("feeAmount");
    }
    if output["feeItems"].as_array().is_some_and(Vec::is_empty) {
        output.as_object_mut().expect("fee object").remove("feeItems");
    }
    output
}

pub(super) fn margin_ratio_value(request: &ResolvedTradeRequest, value: TradeMarginRatioSnapshot) -> Value {
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
    if code.contains('.') || market.is_empty() { code.to_owned() } else { format!("{market}.{code}") }
}

pub(super) fn non_empty(value: &str) -> Option<&str> {
    (!value.trim().is_empty()).then_some(value)
}

pub(super) fn currency_label(currency: Option<i32>) -> Option<&'static str> {
    match currency {
        Some(1) => Some("HKD"), Some(2) => Some("USD"), Some(3) => Some("CNH"),
        Some(4) => Some("JPY"), Some(5) => Some("SGD"), Some(6) => Some("AUD"),
        Some(7) => Some("CAD"), Some(8) => Some("MYR"), Some(9) => Some("NZD"),
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

pub(super) fn trade_market_authority(value: i32) -> Option<&'static str> {
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
    match value { 1 => "CASH", 2 => "MARGIN", 3 => "TFSA", 4 => "RRSP", 5 => "SRRSP", 6 => "DERIVATIVES", _ => "UNKNOWN" }
}

pub(super) fn account_role_label(value: i32) -> Option<&'static str> {
    match value { 1 => Some("NORMAL"), 2 => Some("MASTER"), 3 => Some("IPO"), _ => None }
}

pub(super) fn security_firm_label(value: i32) -> Option<&'static str> {
    match value { 1 => Some("FUTUSECURITIES"), 2 => Some("FUTUINC"), 3 => Some("FUTUSG"), 4 => Some("FUTUAU"), 5 => Some("FUTUCA"), 6 => Some("FUTUMY"), 7 => Some("FUTUJP"), _ => None }
}

pub(super) fn simulated_account_type_label(value: i32) -> Option<&'static str> {
    match value { 1 => Some("STOCK"), 2 => Some("OPTION"), 3 => Some("FUTURES"), 4 => Some("STOCKANDOPTION"), 5 => Some("COMPETITION"), _ => None }
}

pub(super) fn order_type_label(value: i32) -> &'static str {
    match value {
        1 => "NORMAL", 2 => "MARKET", 5 => "ABSOLUTELIMIT", 6 => "AUCTION",
        7 => "AUCTIONLIMIT", 8 => "SPECIALLIMIT", 9 => "SPECIALLIMIT_ALL",
        10 => "STOP", 11 => "STOPLIMIT", 12 => "MARKETIFTOUCHED",
        13 => "LIMITIFTOUCHED", 14 => "TRAILINGSTOP", 15 => "TRAILINGSTOPLIMIT",
        16 => "TWAP_MARKET", 17 => "TWAP_LIMIT", 18 => "VWAP_MARKET", 19 => "VWAP_LIMIT",
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
        -1 => "UNKNOWN", 0 => "UNSUBMITTED", 1 => "WAITINGSUBMIT", 2 => "SUBMITTING",
        3 => "SUBMITFAILED", 4 => "TIMEOUT", 5 => "SUBMITTED", 10 => "FILLED_PART",
        11 => "FILLED_ALL", 12 => "CANCELLING_PART", 13 => "CANCELLING_ALL",
        14 => "CANCELLED_PART", 15 => "CANCELLED_ALL", 21 => "FAILED", 22 => "DISABLED",
        23 => "DELETED", 24 => "FILLCANCELLED", _ => "UNKNOWN",
    }
}

pub(super) fn fill_status_label(value: i32) -> &'static str {
    match value { 0 => "OK", 1 => "CANCELLED", 2 => "CHANGED", 3 => "PAYOUT", _ => "UNKNOWN" }
}

pub(super) fn time_in_force_label(value: i32) -> &'static str {
    match value { 0 => "DAY", 1 => "GTC", 2 => "IOC", 3 => "GTD", _ => "UNKNOWN" }
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
