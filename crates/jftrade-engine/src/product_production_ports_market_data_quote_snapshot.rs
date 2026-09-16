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
                .unwrap_or_else(|| decimal_text_value(Some(&tick.volume)))
        });
    let turnover = active
        .filter(|_| use_active)
        .and_then(|value| value.turnover.as_ref())
        .filter(|value| is_non_negative(value))
        .map(|value| decimal_text_value(Some(value)))
        .unwrap_or_else(|| decimal_text_value(snapshot.and_then(|value| value.turnover.as_ref())));
    let previous_close_price = if uses_regular_close_as_previous_close(market, &session, last_price)
    {
        decimal_value(Some(last_price))
    } else {
        decimal_value(previous_close)
    };

    json!({
        "ask": decimal_value(snapshot.and_then(|value| value.ask_price)),
        "at": observed_at,
        "bid": decimal_value(snapshot.and_then(|value| value.bid_price)),
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
mod tests {
    use super::*;

    fn decimal(value: &str) -> Decimal {
        value.parse().expect("decimal fixture")
    }

    fn decimal_text(value: &str) -> DecimalText {
        value.parse().expect("decimal text fixture")
    }

    fn tick(snapshot: TradeQuoteSnapshot) -> Tick {
        Tick {
            instrument_id: "US.AAPL".to_owned(),
            price: decimal("114.97"),
            volume: decimal_text("1179135"),
            volume_delta: None,
            snapshot: Some(snapshot),
            observed_at_ms: 1_750_000_000_000,
            provider_generation: 1,
        }
    }

    #[test]
    fn cached_projection_uses_active_after_quote_and_separates_closes() {
        let snapshot = TradeQuoteSnapshot {
            symbol: Some("US.AAPL".to_owned()),
            last_price: Some(decimal("114.97")),
            previous_close: Some(decimal("114.97")),
            last_close: Some(decimal("112.50")),
            open_price: Some(decimal("113.00")),
            high_price: Some(decimal("115.70")),
            low_price: Some(decimal("114.13")),
            volume: Some(decimal_text("1179135")),
            turnover: Some(decimal_text("135465382.564")),
            session: Some("after".to_owned()),
            after_market: Some(ExtendedQuoteSnapshot {
                price: Some(decimal("118.40")),
                high_price: Some(decimal("121.20")),
                low_price: Some(decimal("115.50")),
                volume: Some(decimal_text("0")),
                turnover: Some(decimal_text("67722995.69")),
                quote_time: Some("2026-07-18T20:15:00Z".to_owned()),
                trading_date: Some("2026-07-18".to_owned()),
                exchange_timezone: Some("America/New_York".to_owned()),
                session_start_at: Some("2026-07-18T16:00:00Z".to_owned()),
                session_end_at: Some("2026-07-19T00:00:00Z".to_owned()),
                ..Default::default()
            }),
            ..Default::default()
        };
        let value = project_cached_snapshot(
            &tick(snapshot),
            "US",
            "2026-07-18T20:15:00Z",
        );

        assert_eq!(value["price"], "118.40");
        assert_eq!(value["highPrice"], "121.20");
        assert_eq!(value["lowPrice"], "115.50");
        assert_eq!(value["volume"], "0");
        assert_eq!(value["turnover"], "67722995.69");
        assert_eq!(value["previousClosePrice"], "114.97");
        assert_eq!(value["lastClosePrice"], "112.50");
        assert_eq!(value["session"], "after");
        assert_eq!(value["extendedHours"], true);
        assert_eq!(value["extended"]["afterMarket"]["quoteTime"], "2026-07-18T20:15:00Z");
        assert_eq!(
            value["extended"]["afterMarket"]["sessionStartAt"],
            "2026-07-18T16:00:00Z"
        );
    }

    /// Parity: go:452dea11:pkg/futu/quote_snapshot_test.go:217
    /// TestPreviousClosePriceConditionBySessionType
    ///
    /// Go's `ShouldUseRegularCloseAsPreviousClose` is `IsUSSymbol(symbol) &&
    /// session != SessionRegular && regularClose > 0`. Every non-regular US
    /// session (pre/after/overnight/closed/**unknown**) reports the latest
    /// regular-session close; a non-positive close never rewrites the value.
    #[test]
    fn previous_close_condition_switches_on_session_type() {
        let regular_close = decimal("9.22");
        for session in ["pre", "after", "overnight", "closed", "unknown", "UNKNOWN"] {
            assert!(
                uses_regular_close_as_previous_close("US", session, regular_close),
                "US {session} must prefer the latest regular-session close"
            );
        }
        assert!(
            !uses_regular_close_as_previous_close("US", "regular", regular_close),
            "the US regular session must keep the provider LastClosePrice"
        );
        assert!(
            !uses_regular_close_as_previous_close("US", "closed", Decimal::ZERO),
            "a zero regular close must never replace the provider LastClosePrice"
        );
        assert_ne!(
            decimal("9.22"),
            decimal("9.09"),
            "test data sanity"
        );
    }

    /// Parity: go:452dea11:pkg/futu/quote_snapshot_test.go:258
    /// TestPreviousClosePriceConditionDoesNotRewriteNonUSUnknownSession
    #[test]
    fn previous_close_condition_does_not_rewrite_non_us_unknown_sessions() {
        let regular_close = decimal("321.40");
        for instrument_market in ["HK", "SH", "SZ", "CN"] {
            assert!(
                !uses_regular_close_as_previous_close(
                    instrument_market,
                    "unknown",
                    regular_close
                ),
                "{instrument_market} unknown session must keep LastClosePrice"
            );
            assert!(
                !uses_regular_close_as_previous_close(
                    instrument_market,
                    "closed",
                    regular_close
                ),
                "{instrument_market} closed session must keep LastClosePrice"
            );
        }
    }

    /// Parity: go:452dea11:pkg/futu/quote_snapshot_test.go:49
    /// TestQuoteSnapshotPreviousClosePriceInClosedSession and
    /// go:452dea11:pkg/futu/quote_snapshot_test.go:164
    /// TestQuoteSnapshotPreviousClosePriceZeroCurPrice
    ///
    /// Closed US session: `previousClosePrice` is the latest regular-session
    /// close (Friday's CurPrice) while `lastClosePrice` stays the raw provider
    /// value. With a zero current price the condition fails and the provider
    /// LastClosePrice is preserved instead of emitting an empty close.
    #[test]
    fn closed_us_session_reports_regular_close_as_previous_close() {
        let closed = TradeQuoteSnapshot {
            last_price: Some(decimal("9.22")),
            previous_close: Some(decimal("9.09")),
            last_close: Some(decimal("9.09")),
            session: Some("closed".to_owned()),
            ..Default::default()
        };
        let value = project_cached_snapshot(&tick(closed), "US", "2026-05-31T16:00:00Z");
        assert_eq!(
            value["previousClosePrice"], "9.22",
            "a closed US session must report the latest regular-session close"
        );
        assert_eq!(value["lastClosePrice"], "9.09");
        assert_eq!(value["session"], "closed");
    }

    #[test]
    fn zero_current_price_falls_back_to_provider_last_close() {
        let zero_price = TradeQuoteSnapshot {
            last_price: Some(Decimal::ZERO),
            previous_close: Some(decimal("9.09")),
            last_close: Some(decimal("9.09")),
            session: Some("closed".to_owned()),
            ..Default::default()
        };
        let mut zero_tick = tick(zero_price);
        zero_tick.price = Decimal::ZERO;
        let value = project_cached_snapshot(&zero_tick, "US", "2026-05-31T16:00:00Z");
        assert_eq!(
            value["previousClosePrice"], "9.09",
            "a zero regular close must fall back to the provider LastClosePrice"
        );
    }

    /// Parity: go:452dea11:pkg/futu/quote_snapshot_test.go:93
    /// TestQuoteSnapshotHolidayRemainsClosedWithStaleExtendedBlocks
    ///
    /// On a market holiday OpenD keeps returning the previous session's
    /// pre/after/overnight blocks. Go keeps `session=closed`, reports the
    /// regular close as both `price` and `previousClosePrice`, and still
    /// exposes the three blocks (without independent quote timestamps) so the
    /// UI can decide whether to display them.
    #[test]
    fn holiday_snapshot_stays_closed_with_stale_extended_blocks() {
        let extended = |price: &str| ExtendedQuoteSnapshot {
            price: Some(decimal(price)),
            quote_time: None,
            ..Default::default()
        };
        let snapshot = TradeQuoteSnapshot {
            last_price: Some(decimal("195.50")),
            previous_close: Some(decimal("193.20")),
            last_close: Some(decimal("193.20")),
            session: Some("closed".to_owned()),
            pre_market: Some(extended("196.10")),
            after_market: Some(extended("195.30")),
            overnight: Some(extended("194.90")),
            ..Default::default()
        };
        let mut holiday_tick = tick(snapshot);
        holiday_tick.price = decimal("195.50");
        let value = project_cached_snapshot(&holiday_tick, "US", "2026-06-19T16:00:00Z");
        assert_eq!(value["session"], "closed");
        assert_eq!(value["extendedHours"], false);
        assert_eq!(value["price"], "195.50");
        assert_eq!(value["previousClosePrice"], "195.50");
        for block in ["preMarket", "afterMarket", "overnight"] {
            assert!(
                !value["extended"][block].is_null(),
                "stale {block} block must stay available for display decisions"
            );
            assert_eq!(
                value["extended"][block]["quoteTime"], "",
                "{block} must not carry an independent OpenD quote timestamp"
            );
        }
    }

    /// Parity: go:452dea11:pkg/futu/quote_snapshot_test.go:133
    /// TestQuoteSnapshotPreviousClosePriceInAfterHours
    ///
    /// After hours the primary price is the after-market price, but
    /// `previousClosePrice` must be today's regular-session close while
    /// `lastClosePrice` keeps the raw provider value. The after-market block
    /// must not reuse the regular quote timestamp.
    #[test]
    fn after_hours_projection_uses_todays_regular_close_and_keeps_block_time_empty() {
        let snapshot = TradeQuoteSnapshot {
            last_price: Some(decimal("195.50")),
            previous_close: Some(decimal("193.20")),
            last_close: Some(decimal("193.20")),
            session: Some("after".to_owned()),
            after_market: Some(ExtendedQuoteSnapshot {
                price: Some(decimal("195.30")),
                change_rate: Some(decimal_text("-0.10")),
                quote_time: None,
                ..Default::default()
            }),
            ..Default::default()
        };
        let mut after_tick = tick(snapshot);
        after_tick.price = decimal("195.50");
        let value = project_cached_snapshot(&after_tick, "US", "2026-06-02T21:00:00Z");
        assert_eq!(value["price"], "195.30");
        assert_eq!(value["previousClosePrice"], "195.50");
        assert_eq!(value["lastClosePrice"], "193.20");
        assert_eq!(value["session"], "after");
        assert_eq!(value["extendedHours"], true);
        assert_eq!(value["extended"]["afterMarket"]["price"], "195.30");
        assert_eq!(
            value["extended"]["afterMarket"]["quoteTime"], "",
            "the after block must not reuse the BasicQot regular quote time"
        );
    }

    #[test]
    fn cached_projection_does_not_promote_stale_overnight_during_regular_hours() {
        let snapshot = TradeQuoteSnapshot {
            last_price: Some(decimal("114.97")),
            previous_close: Some(decimal("112.50")),
            session: Some("regular".to_owned()),
            overnight: Some(ExtendedQuoteSnapshot {
                price: Some(decimal("119.10")),
                ..Default::default()
            }),
            ..Default::default()
        };
        let value = project_cached_snapshot(
            &tick(snapshot),
            "US",
            "2026-07-18T14:00:00Z",
        );

        assert_eq!(value["price"], "114.97");
        assert_eq!(value["extendedHours"], false);
        assert_eq!(value["extended"]["overnight"]["price"], "119.10");
    }

    #[test]
    fn fallback_projection_normalizes_numbers_and_projects_active_extended_fields() {
        let value = project_fallback_snapshot(
            &json!({
                "lastPrice": 114.97,
                "previousClose": 114.97,
                "lastClosePrice": 112.5,
                "session": "after",
                "highPrice": 115.7,
                "lowPrice": 114.13,
                "volume": 1179135,
                "afterMarket": {
                    "price": 118.4,
                    "highPrice": 121.2,
                    "lowPrice": 115.5,
                    "volume": 0,
                    "turnover": 67722995.69,
                    "quoteTime": "2026-07-18T20:15:00Z",
                    "tradingDate": "2026-07-18",
                    "exchangeTimezone": "America/New_York"
                }
            }),
            "US",
            "2026-07-18T20:15:00Z",
        )
        .expect("fallback projection");

        assert_eq!(value["price"], "118.4");
        assert_eq!(value["previousClosePrice"], "114.97");
        assert_eq!(value["lastClosePrice"], "112.5");
        assert_eq!(value["volume"], "0");
        assert_eq!(value["extended"]["afterMarket"]["volume"], "0");
        assert_eq!(value["extended"]["afterMarket"]["tradingDate"], "2026-07-18");
    }
}
