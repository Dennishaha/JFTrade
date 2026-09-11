//! JSON projections for quote and security values owned by the trade runtime.

use serde_json::{Map, Value, json};

pub(super) fn insert_rich_quote_fields(
    item: &mut Map<String, Value>,
    rich: &jftrade_marketdata::TradeQuoteSnapshot,
) -> Result<(), String> {
    if let Some(name) = rich.name.as_ref() {
        item.insert("symbolName".to_owned(), Value::String(name.clone()));
    }
    for (key, value) in [
        ("openPrice", rich.open_price),
        ("highPrice", rich.high_price),
        ("lowPrice", rich.low_price),
        ("lastClose", rich.previous_close),
    ] {
        if let Some(value) = value {
            item.insert(key.to_owned(), json!(value));
        }
    }
    if let Some(value) = rich.last_close {
        item.insert("lastClosePrice".to_owned(), json!(value));
    }
    if let Some(value) = rich.trading_date.as_ref() {
        item.insert("tradingDate".to_owned(), Value::String(value.clone()));
    }
    if let Some(value) = rich.session.as_ref() {
        item.insert("session".to_owned(), Value::String(value.clone()));
    }
    if let Some(turnover) = rich.turnover.as_ref() {
        item.insert("turnover".to_owned(), decimal_number(turnover)?);
    }
    if let Some(update_time) = rich.update_time.as_ref() {
        item.insert("marketTime".to_owned(), Value::String(update_time.clone()));
    }
    for (key, value) in [
        ("preMarket", rich.pre_market.as_ref()),
        ("afterMarket", rich.after_market.as_ref()),
        ("overnight", rich.overnight.as_ref()),
    ] {
        if let Some(value) = value {
            let mut projected = extended_value(value)?;
            if let Value::Object(object) = &mut projected {
                insert_extended_metadata(object, value);
            }
            item.insert(key.to_owned(), projected);
        }
    }
    Ok(())
}

fn extended_value(value: &jftrade_marketdata::ExtendedQuoteSnapshot) -> Result<Value, String> {
    let mut result = Map::new();
    for (key, number) in [
        ("price", value.price),
        ("highPrice", value.high_price),
        ("lowPrice", value.low_price),
    ] {
        if let Some(number) = number {
            result.insert(key.to_owned(), json!(number));
        }
    }
    for (key, number) in [
        ("volume", value.volume.as_ref()),
        ("turnover", value.turnover.as_ref()),
        ("change", value.change.as_ref()),
        ("changeRate", value.change_rate.as_ref()),
        ("amplitude", value.amplitude.as_ref()),
    ] {
        if let Some(number) = number {
            result.insert(key.to_owned(), decimal_number(number)?);
        }
    }
    Ok(Value::Object(result))
}

fn insert_extended_metadata(
    object: &mut Map<String, Value>,
    value: &jftrade_marketdata::ExtendedQuoteSnapshot,
) {
    for (key, value) in [
        ("quoteTime", value.quote_time.as_ref()),
        ("tradingDate", value.trading_date.as_ref()),
        ("exchangeTimezone", value.exchange_timezone.as_ref()),
        ("sessionStartAt", value.session_start_at.as_ref()),
        ("sessionEndAt", value.session_end_at.as_ref()),
    ] {
        if let Some(value) = value.filter(|value| !value.trim().is_empty()) {
            object.insert(key.to_owned(), Value::String(value.clone()));
        }
    }
}

pub(super) fn insert_rich_security_fields(
    item: &mut Map<String, Value>,
    rich: &jftrade_marketdata::TradeQuoteSnapshot,
) -> Result<(), String> {
    insert_rich_quote_fields(item, rich)?;
    if let Some(name) = rich.name.as_ref() {
        item.remove("symbolName");
        item.insert("name".to_owned(), Value::String(name.clone()));
    }
    if let Some(previous_close) = item.remove("lastClose") {
        item.insert("previousClose".to_owned(), previous_close);
    }
    if let Some(value) = rich.is_suspended {
        item.insert("isSuspended".to_owned(), Value::Bool(value));
        item.insert("isSuspend".to_owned(), Value::Bool(value));
    }
    if let Some(status) = rich.status {
        item.insert("status".to_owned(), json!(status));
    }
    if let Some(update_time) = rich.update_time.as_ref() {
        item.insert("updateTime".to_owned(), Value::String(update_time.clone()));
    }
    if let Some(value) = rich.lot_size {
        item.insert("lotSize".to_owned(), json!(value));
    }
    if let Some(value) = rich.security_type.as_ref() {
        item.insert("securityType".to_owned(), Value::String(value.clone()));
    }
    if let Some(value) = rich.pe_rate.as_ref() {
        item.insert("peRate".to_owned(), decimal_number(value)?);
    }
    if let Some(value) = rich.pb_rate.as_ref() {
        item.insert("pbRate".to_owned(), decimal_number(value)?);
    }
    if let Some(value) = rich.bid_price {
        item.insert("bidPrice".to_owned(), json!(value));
    }
    if let Some(value) = rich.ask_price {
        item.insert("askPrice".to_owned(), json!(value));
    }
    item.remove("marketTime");
    Ok(())
}

pub(super) fn security_snapshot_value(
    snapshot: jftrade_marketdata::BrokerSecuritySnapshot,
) -> Result<Value, String> {
    let mut item = Map::new();
    if let Some(symbol) = snapshot.symbol {
        item.insert("symbol".to_owned(), Value::String(symbol));
    }
    if let Some(market) = snapshot.market {
        item.insert("market".to_owned(), Value::String(market));
    }
    if let Some(name) = snapshot.name {
        item.insert("name".to_owned(), Value::String(name));
    }
    for (key, value) in [
        ("lastPrice", snapshot.last_price),
        ("bidPrice", snapshot.bid_price),
        ("askPrice", snapshot.ask_price),
        ("openPrice", snapshot.open_price),
        ("highPrice", snapshot.high_price),
        ("lowPrice", snapshot.low_price),
        ("previousClose", snapshot.previous_close),
        ("lastClosePrice", snapshot.last_close),
    ] {
        if let Some(value) = value {
            item.insert(key.to_owned(), json!(value));
        }
    }
    if let Some(turnover) = snapshot.turnover.as_ref() {
        item.insert("turnover".to_owned(), decimal_number(turnover)?);
    }
    if let Some(volume) = snapshot.volume.as_ref() {
        item.insert("volume".to_owned(), decimal_number(volume)?);
    }
    if let Some(status) = snapshot.status {
        item.insert("status".to_owned(), json!(status));
    }
    if let Some(value) = snapshot.is_suspended {
        item.insert("isSuspended".to_owned(), Value::Bool(value));
        item.insert("isSuspend".to_owned(), Value::Bool(value));
    }
    if let Some(value) = snapshot.lot_size {
        item.insert("lotSize".to_owned(), json!(value));
    }
    if let Some(value) = snapshot.security_type {
        item.insert("securityType".to_owned(), Value::String(value));
    }
    if let Some(value) = snapshot.update_time {
        item.insert("updateTime".to_owned(), Value::String(value));
    }
    if let Some(value) = snapshot.pe_rate.as_ref() {
        item.insert("peRate".to_owned(), decimal_number(value)?);
    }
    if let Some(value) = snapshot.pb_rate.as_ref() {
        item.insert("pbRate".to_owned(), decimal_number(value)?);
    }
    if let Some(value) = snapshot.trading_date.as_ref() {
        item.insert("tradingDate".to_owned(), Value::String(value.clone()));
    }
    if let Some(value) = snapshot.session.as_ref() {
        item.insert("session".to_owned(), Value::String(value.clone()));
    }
    for (key, value) in [
        ("preMarket", snapshot.pre_market.as_ref()),
        ("afterMarket", snapshot.after_market.as_ref()),
        ("overnight", snapshot.overnight.as_ref()),
    ] {
        if let Some(value) = value {
            let mut projected = extended_value(value)?;
            if let Value::Object(object) = &mut projected {
                insert_extended_metadata(object, value);
            }
            item.insert(key.to_owned(), projected);
        }
    }
    Ok(Value::Object(item))
}

fn decimal_number(value: &jftrade_kernel::DecimalText) -> Result<Value, String> {
    value
        .as_str()
        .parse::<serde_json::Number>()
        .map(Value::Number)
        .map_err(|error| format!("invalid cached decimal {}: {error}", value.as_str()))
}
