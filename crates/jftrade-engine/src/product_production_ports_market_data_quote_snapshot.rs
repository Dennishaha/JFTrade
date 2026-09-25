//! Projection of provider-neutral Futu quote snapshots onto the workspace
//! snapshot contract.
//!
//! OpenD exposes the regular quote and the pre/after/overnight blocks in one
//! response.  The workspace contract, however, presents the currently active
//! session as the primary quote while retaining every block under `extended`.
//! Keeping this conversion here means cache reads and the delayed security
//! snapshot fallback cannot drift apart.

use std::str::FromStr;

use jftrade_kernel::{Decimal, DecimalText};
use jftrade_marketdata::{ExtendedQuoteSnapshot, Tick, TradeQuoteSnapshot};
use serde_json::{Map, Value, json};

/// Projects one cached broker-neutral tick onto the public workspace snapshot.
pub(super) fn project_cached_snapshot(tick: &Tick, market: &str, observed_at: &str) -> Value {
    let snapshot = tick.snapshot.as_ref();
    let authoritative = snapshot.is_some_and(|value| value.authoritative);
    let session = normalized_session(snapshot.and_then(|value| value.session.as_deref()));
    let active = snapshot.and_then(|value| active_extended(value, &session));
    let active_price = active.and_then(|value| value.price);
    let use_active = active_price.is_some_and(is_positive);
    let last_price = snapshot
        .and_then(|value| value.last_price)
        .unwrap_or(tick.price);
    let previous_close = snapshot.and_then(|value| value.previous_close);
    let last_close = snapshot
        .and_then(|value| value.last_close)
        .or(previous_close);

    let price = if use_active {
        decimal_value(active_price)
    } else {
        decimal_value(Some(last_price))
    };
    let high_price = active
        .filter(|_| use_active)
        .and_then(|value| value.high_price)
        .filter(|value| is_positive(*value))
        .map(|value| decimal_value(Some(value)))
        .unwrap_or_else(|| decimal_value(snapshot.and_then(|value| value.high_price)));
    let low_price = active
        .filter(|_| use_active)
        .and_then(|value| value.low_price)
        .filter(|value| is_positive(*value))
        .map(|value| decimal_value(Some(value)))
        .unwrap_or_else(|| decimal_value(snapshot.and_then(|value| value.low_price)));
    let volume = active
        .filter(|_| use_active)
        .and_then(|value| value.volume.as_ref())
        .filter(|value| is_non_negative(value))
        .map(|value| decimal_text_value(Some(value)))
        .unwrap_or_else(|| {
            snapshot
                .and_then(|value| value.volume.as_ref())
                .map(|value| decimal_text_value(Some(value)))
                .or_else(|| (!authoritative).then(|| decimal_text_value(Some(&tick.volume))))
                .unwrap_or(Value::Null)
        });
    let turnover = active
        .filter(|_| use_active)
        .and_then(|value| value.turnover.as_ref())
        .filter(|value| is_non_negative(value))
        .map(|value| decimal_text_value(Some(value)))
        .unwrap_or_else(|| {
            snapshot
                .and_then(|value| value.turnover.as_ref())
                .map(|value| decimal_text_value(Some(value)))
                .or_else(|| (!authoritative).then(|| decimal_text_value(Some(&zero_decimal_text()))))
                .unwrap_or(Value::Null)
        });
    let previous_close_price = if uses_regular_close_as_previous_close(market, &session, last_price)
    {
        decimal_value(Some(last_price))
    } else {
        decimal_value(previous_close)
    };

    json!({
        "ask": quote_field_value(snapshot.and_then(|value| value.ask_price), snapshot.is_none()),
        "at": observed_at,
        "bid": quote_field_value(snapshot.and_then(|value| value.bid_price), snapshot.is_none()),
        "extended": {
            "afterMarket": extended_value(snapshot.and_then(|value| value.after_market.as_ref())),
            "overnight": extended_value(snapshot.and_then(|value| value.overnight.as_ref())),
            "preMarket": extended_value(snapshot.and_then(|value| value.pre_market.as_ref())),
        },
        "extendedHours": is_extended_session(&session),
        "highPrice": high_price,
        "lastClosePrice": decimal_value(last_close),
        "lowPrice": low_price,
        "observedAt": observed_at,
        "openPrice": decimal_value(snapshot.and_then(|value| value.open_price)),
        "previousClosePrice": previous_close_price,
        "price": price,
        "session": session,
        "turnover": turnover,
        "volume": volume,
    })
}

fn quote_field_value(value: Option<Decimal>, legacy_zero: bool) -> Value {
    value
        .map(|value| decimal_value(Some(value)))
        .unwrap_or_else(|| {
            if legacy_zero {
                decimal_value(Some(Decimal::ZERO))
            } else {
                Value::Null
            }
        })
}

fn zero_decimal_text() -> DecimalText {
    DecimalText::from_str("0").expect("zero is valid decimal text")
}

/// Projects a security-snapshot value returned by the trade runtime.  The
/// value can originate from the typed OpenD reader or from a cache fallback,
/// so fields may be JSON numbers or strings.  Public quote fields are always
/// rendered as strings, matching the OpenAPI snapshot contract.
pub(super) fn project_fallback_snapshot(
    snapshot: &Value,
    market: &str,
    fallback_observed_at: &str,
) -> Option<Value> {
    let raw_price = first_value(snapshot, &["lastPrice", "currentPrice", "price"])?;
    if !positive_value(raw_price) {
        return None;
    }
    let raw_price = string_value(raw_price)?;
    let session = normalized_session(snapshot.get("session").and_then(Value::as_str));
    let active = snapshot
        .get(active_block_key(&session))
        .filter(|value| value.is_object());
    let use_active = active
        .and_then(|value| value.get("price"))
        .is_some_and(positive_value);
    let active_or = |key: &str| {
        active
            .filter(|_| use_active)
            .and_then(|value| value.get(key))
    };
    let high = active_or("highPrice")
        .filter(|value| positive_value(value))
        .or_else(|| snapshot.get("highPrice"));
    let low = active_or("lowPrice")
        .filter(|value| positive_value(value))
        .or_else(|| snapshot.get("lowPrice"));
    let volume = active_or("volume")
        .filter(|value| non_negative_value(value))
        .or_else(|| snapshot.get("volume"));
    let turnover = active_or("turnover")
        .filter(|value| non_negative_value(value))
        .or_else(|| snapshot.get("turnover"));
    let last_price = first_value(snapshot, &["lastPrice", "currentPrice", "price"])
        .and_then(string_value)
        .unwrap_or_else(|| raw_price.clone());
    let previous_close = snapshot
        .get("previousClose")
        .or_else(|| snapshot.get("previousClosePrice"));
    let last_close = snapshot
        .get("lastClosePrice")
        .or_else(|| snapshot.get("lastClose"))
        .or(previous_close);
    let previous_close_price = if uses_regular_close_as_previous_close(
        market,
        &session,
        Decimal::from_str(&last_price).unwrap_or(Decimal::ZERO),
    ) {
        Value::String(last_price)
    } else {
        quote_value(previous_close)
    };
    let observed_at = first_string(snapshot, &["observedAt", "quoteAt", "updateTime"])
        .unwrap_or_else(|| fallback_observed_at.to_owned());

    Some(json!({
        "ask": quote_value(snapshot.get("askPrice").or_else(|| snapshot.get("ask"))),
        "at": observed_at,
        "bid": quote_value(snapshot.get("bidPrice").or_else(|| snapshot.get("bid"))),
        "extended": {
            "afterMarket": fallback_extended_value(snapshot.get("afterMarket")),
            "overnight": fallback_extended_value(snapshot.get("overnight")),
            "preMarket": fallback_extended_value(snapshot.get("preMarket")),
        },
        "extendedHours": is_extended_session(&session),
        "highPrice": quote_value(high),
        "lastClosePrice": quote_value(last_close),
        "lowPrice": quote_value(low),
        "observedAt": observed_at,
        "openPrice": quote_value(snapshot.get("openPrice")),
        "previousClosePrice": previous_close_price,
        "price": if use_active {
            quote_value(active.and_then(|value| value.get("price")))
        } else {
            Value::String(raw_price)
        },
        "session": session,
        "turnover": quote_value(turnover),
        "volume": quote_value(volume),
    }))
}

fn active_extended<'a>(
    snapshot: &'a TradeQuoteSnapshot,
    session: &str,
) -> Option<&'a ExtendedQuoteSnapshot> {
    match session {
        "pre" => snapshot.pre_market.as_ref(),
        "after" => snapshot.after_market.as_ref(),
        "overnight" => snapshot.overnight.as_ref(),
        _ => None,
    }
}

fn extended_value(value: Option<&ExtendedQuoteSnapshot>) -> Value {
    let Some(value) = value else {
        return Value::Null;
    };
    let mut result = Map::from_iter([
        ("price".to_owned(), decimal_value(value.price)),
        ("highPrice".to_owned(), decimal_value(value.high_price)),
        ("lowPrice".to_owned(), decimal_value(value.low_price)),
        (
            "volume".to_owned(),
            decimal_text_value(value.volume.as_ref()),
        ),
        (
            "turnover".to_owned(),
            decimal_text_value(value.turnover.as_ref()),
        ),
        (
            "changeVal".to_owned(),
            decimal_text_value(value.change.as_ref()),
        ),
        (
            "changeRate".to_owned(),
            decimal_text_value(value.change_rate.as_ref()),
        ),
        (
            "amplitude".to_owned(),
            decimal_text_value(value.amplitude.as_ref()),
        ),
        (
            "quoteTime".to_owned(),
            Value::String(value.quote_time.clone().unwrap_or_default()),
        ),
    ]);
    for (key, value) in [
        ("tradingDate", value.trading_date.as_ref()),
        ("exchangeTimezone", value.exchange_timezone.as_ref()),
        ("sessionStartAt", value.session_start_at.as_ref()),
        ("sessionEndAt", value.session_end_at.as_ref()),
    ] {
        if let Some(value) = value.filter(|value| !value.trim().is_empty()) {
            result.insert(key.to_owned(), Value::String(value.clone()));
        }
    }
    Value::Object(result)
}

fn fallback_extended_value(value: Option<&Value>) -> Value {
    let Some(Value::Object(object)) = value else {
        return Value::Null;
    };
    let mut result = Map::from_iter([
        ("price".to_owned(), quote_value(object.get("price"))),
        ("highPrice".to_owned(), quote_value(object.get("highPrice"))),
        ("lowPrice".to_owned(), quote_value(object.get("lowPrice"))),
        ("volume".to_owned(), quote_value(object.get("volume"))),
        ("turnover".to_owned(), quote_value(object.get("turnover"))),
        (
            "changeVal".to_owned(),
            quote_value(object.get("changeVal").or_else(|| object.get("change"))),
        ),
        (
            "changeRate".to_owned(),
            quote_value(object.get("changeRate")),
        ),
        ("amplitude".to_owned(), quote_value(object.get("amplitude"))),
        (
            "quoteTime".to_owned(),
            object
                .get("quoteTime")
                .and_then(Value::as_str)
                .map(|value| Value::String(value.to_owned()))
                .unwrap_or_else(|| Value::String(String::new())),
        ),
    ]);
    for key in [
        "tradingDate",
        "exchangeTimezone",
        "sessionStartAt",
        "sessionEndAt",
    ] {
        if let Some(value) = object.get(key).and_then(Value::as_str)
            && !value.trim().is_empty()
        {
            result.insert(key.to_owned(), Value::String(value.to_owned()));
        }
    }
    Value::Object(result)
}

fn decimal_value(value: Option<Decimal>) -> Value {
    value
        .map(|value| Value::String(value.to_string()))
        .unwrap_or(Value::Null)
}

fn decimal_text_value(value: Option<&DecimalText>) -> Value {
    value
        .map(|value| Value::String(value.as_str().to_owned()))
        .unwrap_or(Value::Null)
}

fn quote_value(value: Option<&Value>) -> Value {
    value
        .and_then(string_value)
        .map(Value::String)
        .unwrap_or(Value::Null)
}

fn string_value(value: &Value) -> Option<String> {
    match value {
        Value::String(value) => Some(value.trim().to_owned()),
        Value::Number(value) => Some(value.to_string()),
        _ => None,
    }
}

fn first_value<'a>(value: &'a Value, keys: &[&str]) -> Option<&'a Value> {
    keys.iter().find_map(|key| value.get(*key))
}

fn first_string(value: &Value, keys: &[&str]) -> Option<String> {
    first_value(value, keys).and_then(string_value)
}

fn positive_value(value: &Value) -> bool {
    string_value(value)
        .and_then(|value| Decimal::from_str(&value).ok())
        .is_some_and(is_positive)
}

fn non_negative_value(value: &Value) -> bool {
    string_value(value)
        .and_then(|value| Decimal::from_str(&value).ok())
        .is_some_and(|value| value >= Decimal::ZERO)
}

fn is_positive(value: Decimal) -> bool {
    value > Decimal::ZERO
}

fn is_non_negative(value: &DecimalText) -> bool {
    Decimal::from_str(value.as_str()).is_ok_and(|value| value >= Decimal::ZERO)
}

fn normalized_session(value: Option<&str>) -> String {
    let value = value.unwrap_or("regular").trim().to_ascii_lowercase();
    if value.is_empty() {
        "regular".to_owned()
    } else {
        value
    }
}

fn active_block_key(session: &str) -> &str {
    match session {
        "pre" => "preMarket",
        "after" => "afterMarket",
        "overnight" => "overnight",
        _ => "",
    }
}

fn is_us_market(market: &str) -> bool {
    market.trim().eq_ignore_ascii_case("US")
}

fn is_extended_session(session: &str) -> bool {
    matches!(session, "pre" | "after" | "overnight")
}

/// Go's `market.ShouldUseRegularCloseAsPreviousClose`: every US session other
/// than `regular` (including `unknown`) reports the latest regular-session
/// close as `previousClosePrice`, and only when that close is positive.
///
/// The `unknown` arm is intentional rather than an allow-list of known
/// extended sessions: an unresolved calendar must still surface the most
/// recent regular close instead of the previous trading day's close.
fn uses_regular_close_as_previous_close(
    market: &str,
    session: &str,
    regular_close: Decimal,
) -> bool {
    is_us_market(market) && !session.eq_ignore_ascii_case("regular") && regular_close > Decimal::ZERO
}

#[cfg(test)]
#[path = "product_production_ports_market_data_quote_snapshot_tests.rs"]
mod tests;
