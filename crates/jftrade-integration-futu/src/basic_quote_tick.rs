use std::collections::BTreeMap;
use std::str::FromStr;

use jftrade_kernel::{Decimal, DecimalText};
use jftrade_marketdata::{ExtendedQuoteSnapshot, Tick, TradeQuoteSnapshot};
use jiff::{Timestamp, ToSpan, civil::Date, tz::TimeZone};
use thiserror::Error;

use crate::{BasicQuote, QuoteSessionResolver, Security};

#[derive(Clone, Debug, Error, Eq, PartialEq)]
pub enum BasicQuoteTickError {
    #[error("OpenD BasicQot price is not finite for {instrument_id}")]
    NonFinitePrice { instrument_id: String },
    #[error("OpenD BasicQot price is outside Decimal range for {instrument_id}: {price}")]
    PriceOutOfRange {
        instrument_id: String,
        price: String,
    },
    #[error("OpenD BasicQot volume is invalid for {instrument_id}: {volume}")]
    InvalidVolume {
        instrument_id: String,
        volume: String,
    },
}

/// Maps OpenD BasicQot rows into the current broker-neutral collector model.
///
/// Invalid securities and zero-price rows are dropped like the Go BasicQot map
/// and `tickFromSnapshot` adapter. Duplicate rows keep the last value. The
/// caller owns the observation clock and provider generation; this mapper does
/// not mutate cache, recorder, subscriptions or provider lifecycle.
pub fn basic_quote_ticks(
    quotes: Vec<BasicQuote>,
    observed_at_ms: i64,
    provider_generation: u64,
) -> Result<Vec<Tick>, BasicQuoteTickError> {
    basic_quote_ticks_with_resolver(quotes, observed_at_ms, provider_generation, None)
}

/// Maps BasicQot rows with a composition-owned exchange calendar resolver.
/// The no-resolver wrapper above intentionally keeps the legacy fallback for
/// explicit embedders; production OpenD always supplies the shared manager.
pub fn basic_quote_ticks_with_resolver(
    quotes: Vec<BasicQuote>,
    observed_at_ms: i64,
    provider_generation: u64,
    resolver: Option<&dyn QuoteSessionResolver>,
) -> Result<Vec<Tick>, BasicQuoteTickError> {
    let mut ticks = BTreeMap::new();
    for quote in quotes {
        let Some(instrument_id) = quote
            .security
            .as_ref()
            .and_then(instrument_id_from_security)
        else {
            continue;
        };
        let Some(price) = quote.cur_price else {
            continue;
        };
        if price == 0.0 {
            continue;
        }
        let price = decimal_from_price(&instrument_id, price)?;
        let volume = collector_volume(&instrument_id, &quote)?;
        let session = session_context_with_resolver(&instrument_id, observed_at_ms, resolver);
        ticks.insert(
            instrument_id.clone(),
            Tick {
                instrument_id: instrument_id.clone(),
                price,
                volume: volume.clone(),
                // BasicQot is a quote snapshot, not a trade print, so it
                // carries no per-event delta; Go's `tickFromSnapshot` leaves
                // `VolumeDelta` at its zero value for the same reason.
                volume_delta: None,
                snapshot: Some(TradeQuoteSnapshot {
                    authoritative: true,
                    symbol: Some(instrument_id.clone()),
                    name: quote.name,
                    is_suspended: quote.is_suspended,
                    last_price: Some(price),
                    volume: quote.volume.map(|_| volume.clone()),
                    open_price: optional_price(quote.open_price),
                    high_price: optional_price(quote.high_price),
                    low_price: optional_price(quote.low_price),
                    previous_close: optional_price(quote.last_close_price),
                    last_close: optional_price(quote.last_close_price),
                    turnover: optional_decimal(quote.turnover),
                    update_time: quote.update_time,
                    status: quote.sec_status,
                    trading_date: session.as_ref().map(|value| value.trading_date.clone()),
                    session: session.as_ref().map(|value| value.session.clone()),
                    pre_market: quote
                        .pre_market
                        .map(|value| extended_snapshot(value, session.as_ref(), "pre")),
                    after_market: quote
                        .after_market
                        .map(|value| extended_snapshot(value, session.as_ref(), "after")),
                    overnight: quote
                        .overnight
                        .map(|value| extended_snapshot(value, session.as_ref(), "overnight")),
                    ..Default::default()
                }),
                observed_at_ms,
                provider_generation,
            },
        );
    }
    Ok(ticks.into_values().collect())
}

fn optional_price(value: Option<f64>) -> Option<Decimal> {
    value
        .filter(|value| value.is_finite())
        .and_then(|value| Decimal::from_str(&value.to_string()).ok())
}

fn optional_decimal(value: Option<f64>) -> Option<DecimalText> {
    value
        .filter(|value| value.is_finite())
        .and_then(|value| DecimalText::from_str(&value.to_string()).ok())
}

fn extended_snapshot(
    value: crate::PreAfterMarketData,
    context: Option<&SessionContext>,
    session: &str,
) -> ExtendedQuoteSnapshot {
    let (trading_date, exchange_timezone, session_start_at, session_end_at) =
        extended_session_metadata(context, session);
    ExtendedQuoteSnapshot {
        price: optional_price(value.price),
        high_price: optional_price(value.high_price),
        low_price: optional_price(value.low_price),
        volume: value
            .volume
            .and_then(|v| DecimalText::from_str(&v.to_string()).ok()),
        turnover: optional_decimal(value.turnover),
        change: optional_decimal(value.change_value),
        change_rate: optional_decimal(value.change_rate),
        amplitude: optional_decimal(value.amplitude),
        quote_time: None,
        trading_date,
        exchange_timezone,
        session_start_at,
        session_end_at,
    }
}

#[derive(Clone, Debug)]
struct SessionContext {
    session: String,
    trading_date: String,
    timezone: String,
    sessions: Vec<SessionWindow>,
}

#[derive(Clone, Debug)]
struct SessionWindow {
    kind: String,
    start_minute: i32,
    end_minute: i32,
}

fn session_context_with_resolver(
    instrument_id: &str,
    observed_at_ms: i64,
    resolver: Option<&dyn QuoteSessionResolver>,
) -> Option<SessionContext> {
    session_context_with_resolver_and_timestamp(instrument_id, observed_at_ms, resolver)
}

/// Resolves only the broker-neutral session label for one observation.
///
/// The streaming trade path needs the same session definition as the snapshot
/// projection so a session change resets the cumulative-volume baseline; this
/// wrapper keeps that decision in one place instead of re-deriving it in the
/// composition root.
pub fn quote_session_label(
    instrument_id: &str,
    observed_at_ms: i64,
    resolver: Option<&dyn QuoteSessionResolver>,
) -> String {
    session_context_with_resolver_and_timestamp(instrument_id, observed_at_ms, resolver)
        .map(|context| context.session)
        .unwrap_or_else(|| "regular".to_owned())
}

fn session_context_with_resolver_and_timestamp(
    instrument_id: &str,
    observed_at_ms: i64,
    resolver: Option<&dyn QuoteSessionResolver>,
) -> Option<SessionContext> {
    let market = instrument_id.split_once('.')?.0.to_ascii_uppercase();
    let timezone_name = match market.as_str() {
        "US" => "America/New_York",
        "HK" => "Asia/Hong_Kong",
        "CN" | "SH" | "SZ" => "Asia/Shanghai",
        _ => return None,
    };
    let timestamp = Timestamp::from_millisecond(observed_at_ms).ok()?;
    let zone = TimeZone::get(timezone_name).ok()?;
    if let Some(resolver) = resolver
        && let Some(context) = resolver.resolve_quote_session(instrument_id, observed_at_ms)
    {
        return Some(SessionContext {
            session: context.session,
            trading_date: context.trading_date,
            timezone: context.timezone,
            sessions: context
                .sessions
                .into_iter()
                .map(|window| SessionWindow {
                    kind: window.kind,
                    start_minute: window.start_minute,
                    end_minute: window.end_minute,
                })
                .collect(),
        });
    }
    let local = timestamp.to_zoned(zone);
    let minute = i32::from(local.hour()) * 60 + i32::from(local.minute());
    let mut trading_date = local.date();
    let session = if market == "US" {
        match minute {
            0..=239 => "overnight".to_owned(),
            240..=569 => "pre".to_owned(),
            570..=959 => "regular".to_owned(),
            960..=1199 => "after".to_owned(),
            1200..=1439 => {
                trading_date = trading_date.tomorrow().ok()?;
                "overnight".to_owned()
            }
            _ => "closed".to_owned(),
        }
    } else {
        "regular".to_owned()
    };
    Some(SessionContext {
        session,
        trading_date: trading_date.to_string(),
        timezone: timezone_name.to_owned(),
        sessions: fallback_sessions(&market),
    })
}

fn fallback_sessions(market: &str) -> Vec<SessionWindow> {
    let windows: &[(&str, i32, i32)] = match market {
        "US" => &[
            ("overnight", 0, 240),
            ("pre", 240, 570),
            ("regular", 570, 960),
            ("after", 960, 1200),
        ],
        "HK" => &[("regular", 570, 720), ("regular", 780, 960)],
        "CN" | "SH" | "SZ" => &[("regular", 570, 690), ("regular", 780, 900)],
        _ => &[],
    };
    windows
        .iter()
        .map(|(kind, start_minute, end_minute)| SessionWindow {
            kind: (*kind).to_owned(),
            start_minute: *start_minute,
            end_minute: *end_minute,
        })
        .collect()
}

fn extended_session_metadata(
    context: Option<&SessionContext>,
    session: &str,
) -> (
    Option<String>,
    Option<String>,
    Option<String>,
    Option<String>,
) {
    let Some(context) = context else {
        return (None, None, None, None);
    };
    let Some(date) = Date::strptime("%Y-%m-%d", &context.trading_date).ok() else {
        return (None, None, None, None);
    };
    let Ok(zone) = TimeZone::get(&context.timezone) else {
        return (None, None, None, None);
    };
    let Some(window) = context
        .sessions
        .iter()
        .find(|window| window.kind == session)
    else {
        // Go's ResolveTradingDaySessionWindow fails when the exchange calendar
        // has no window of this kind (for example HK has no pre-market), and
        // attachFutuSessionWindow then leaves the whole block unannotated.  A
        // partial annotation would advertise a trading date/timezone for a
        // session the exchange never opens, so drop all four fields together.
        return (None, None, None, None);
    };
    let (start_date, start_minute, end_date, end_minute) = if session == "overnight" {
        (
            date.checked_sub(1.days()).ok().unwrap_or(date),
            1200,
            date,
            window.end_minute,
        )
    } else {
        (date, window.start_minute, date, window.end_minute)
    };
    let start = start_date
        .at((start_minute / 60) as i8, (start_minute % 60) as i8, 0, 0)
        .to_zoned(zone.clone())
        .ok()
        .map(|value| value.timestamp().to_string());
    let end = end_date
        .at((end_minute / 60) as i8, (end_minute % 60) as i8, 0, 0)
        .to_zoned(zone)
        .ok()
        .map(|value| value.timestamp().to_string());
    (
        Some(context.trading_date.clone()),
        Some(context.timezone.clone()),
        start,
        end,
    )
}

fn instrument_id_from_security(security: &Security) -> Option<String> {
    let market = match security.market? {
        1 => "HK",
        11 => "US",
        21 => "SH",
        22 => "SZ",
        31 => "SG",
        41 => "JP",
        51 => "AU",
        61 => "MY",
        71 => "CA",
        _ => return None,
    };
    let code = security.code.as_deref()?.trim().to_ascii_uppercase();
    (!code.is_empty()).then(|| format!("{market}.{code}"))
}

fn decimal_from_price(instrument_id: &str, price: f64) -> Result<Decimal, BasicQuoteTickError> {
    if !price.is_finite() {
        return Err(BasicQuoteTickError::NonFinitePrice {
            instrument_id: instrument_id.to_owned(),
        });
    }
    let price = price.to_string();
    Decimal::from_str(&price).map_err(|_| BasicQuoteTickError::PriceOutOfRange {
        instrument_id: instrument_id.to_owned(),
        price,
    })
}

fn collector_volume(
    instrument_id: &str,
    quote: &BasicQuote,
) -> Result<DecimalText, BasicQuoteTickError> {
    let volume = quote.volume.unwrap_or_default();
    let text = match quote.hp_volume {
        Some(high_precision)
            if high_precision.is_finite()
                && high_precision >= 0.0
                && (high_precision > 0.0 || volume == 0) =>
        {
            high_precision.to_string()
        }
        _ => volume.to_string(),
    };
    DecimalText::from_str(&text).map_err(|_| BasicQuoteTickError::InvalidVolume {
        instrument_id: instrument_id.to_owned(),
        volume: text,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn security_projection_builds_market_qualified_symbol() {
        // Parity: go:452dea11:pkg/futu/adapter_new_methods_test.go:175
        // TestSecuritySymbol.
        assert_eq!(
            instrument_id_from_security(&Security {
                market: Some(1),
                code: Some("00700".to_owned()),
            })
            .as_deref(),
            Some("HK.00700")
        );
        assert_eq!(
            instrument_id_from_security(&Security {
                market: Some(11),
                code: Some("aapl".to_owned()),
            })
            .as_deref(),
            Some("US.AAPL")
        );
    }

    #[test]
    fn security_projection_rejects_nil_or_unknown_market() {
        // Parity: go:452dea11:pkg/futu/adapter_new_methods_test.go:182
        // TestSecuritySymbolNil and :156 TestSecuritiesFromSymbolsInvalid.
        assert_eq!(
            instrument_id_from_security(&Security {
                market: None,
                code: Some("00700".to_owned()),
            }),
            None
        );
        assert_eq!(
            instrument_id_from_security(&Security {
                market: Some(1),
                code: Some("  ".to_owned()),
            }),
            None
        );
        assert_eq!(
            instrument_id_from_security(&Security {
                market: Some(9_999),
                code: Some("00700".to_owned()),
            }),
            None
        );
    }
    use serde::Deserialize;

    use crate::{
        PreAfterMarketData, QuoteSessionContext, QuoteSessionResolver, QuoteSessionWindow,
    };

    #[derive(Clone, Debug)]
    struct StaticSessionResolver(QuoteSessionContext);

    impl QuoteSessionResolver for StaticSessionResolver {
        fn resolve_quote_session(
            &self,
            _instrument_id: &str,
            _observed_at_ms: i64,
        ) -> Option<QuoteSessionContext> {
            Some(self.0.clone())
        }
    }

    #[derive(Debug, Deserialize)]
    struct NonFinitePriceCorpus {
        version: String,
        cases: Vec<NonFinitePriceCase>,
    }

    #[derive(Debug, Deserialize)]
    #[serde(rename_all = "camelCase")]
    struct NonFinitePriceCase {
        name: String,
        price: String,
        instrument: String,
        go_behavior: String,
        rust_behavior: String,
    }

    fn quote(market: i32, code: &str, price: f64, volume: i64) -> BasicQuote {
        BasicQuote {
            security: Some(Security {
                market: Some(market),
                code: Some(code.to_owned()),
            }),
            cur_price: Some(price),
            volume: Some(volume),
            ..empty_quote()
        }
    }

    fn empty_quote() -> BasicQuote {
        BasicQuote {
            security: None,
            name: None,
            is_suspended: None,
            list_time: None,
            price_spread: None,
            update_time: None,
            high_price: None,
            open_price: None,
            low_price: None,
            cur_price: None,
            last_close_price: None,
            volume: None,
            turnover: None,
            turnover_rate: None,
            amplitude: None,
            dark_status: None,
            list_timestamp: None,
            update_timestamp: None,
            pre_market: None,
            after_market: None,
            sec_status: None,
            overnight: None,
            hp_volume: None,
        }
    }

    #[test]
    fn maps_normalized_requested_rows_and_keeps_the_last_duplicate() {
        let mut high_precision = quote(11, " aapl ", 189.123_456_789, 10);
        high_precision.hp_volume = Some(12.0);
        let ticks = basic_quote_ticks(
            vec![
                quote(1, "00700", 300.0, 20),
                quote(11, "AAPL", 188.0, 9),
                high_precision,
                quote(999, "UNKNOWN", 1.0, 1),
                quote(11, "ZERO", 0.0, 1),
            ],
            1_724_464_001_250,
            7,
        )
        .expect("mapped ticks");

        assert_eq!(ticks.len(), 2);
        assert_eq!(ticks[0].instrument_id, "HK.00700");
        assert_eq!(ticks[0].price.to_string(), "300");
        assert_eq!(ticks[1].instrument_id, "US.AAPL");
        assert_eq!(ticks[1].price.to_string(), "189.123456789");
        assert_eq!(ticks[1].volume.to_string(), "12");
        assert_eq!(ticks[1].observed_at_ms, 1_724_464_001_250);
        assert_eq!(ticks[1].provider_generation, 7);
    }

    #[test]
    fn rejects_non_finite_price_without_panicking() {
        assert!(matches!(
            basic_quote_ticks(vec![quote(11, "AAPL", f64::NAN, 1)], 0, 1),
            Err(BasicQuoteTickError::NonFinitePrice { instrument_id })
                if instrument_id == "US.AAPL"
        ));
    }

    #[test]
    fn non_finite_price_corpus_matches_go_failure_boundary_and_rust_rejection() {
        let corpus: NonFinitePriceCorpus = serde_json::from_str(include_str!(
            "../../../tests/fixtures/compatibility/api-transport/basic-quote-nonfinite.json"
        ))
        .expect("non-finite price corpus");
        assert_eq!(corpus.version, "stage9.basic-quote-nonfinite.v1");
        assert!(!corpus.cases.is_empty());

        for case in corpus.cases {
            assert_eq!(case.go_behavior, "panic", "case={}", case.name);
            assert_eq!(case.rust_behavior, "reject", "case={}", case.name);
            let price = match case.price.as_str() {
                "NaN" => f64::NAN,
                "+Inf" => f64::INFINITY,
                "-Inf" => f64::NEG_INFINITY,
                other => other.parse().expect("finite corpus price"),
            };
            let instrument = case.instrument.strip_prefix("US.").expect("US instrument");
            let result = basic_quote_ticks(vec![quote(11, instrument, price, 1)], 0, 1);
            assert!(matches!(
                result,
                Err(BasicQuoteTickError::NonFinitePrice { instrument_id })
                    if instrument_id == case.instrument
            ));
        }
    }

    /// Parity: go:452dea11:pkg/futu/quote_snapshot_test.go:14
    /// TestQuoteSnapshotResolvesHighPrecisionVolumeWithoutLosingInt64Precision
    ///
    /// `ordinaryVolume` (9_007_199_254_740_993) is not representable as f64, so
    /// the required int64 field must survive untouched unless `hpVolume` is a
    /// usable positive value; a zero or non-finite `hpVolume` never erases it.
    #[test]
    fn high_precision_volume_never_loses_required_int64_precision() {
        const ORDINARY_VOLUME: i64 = 9_007_199_254_740_993;
        let cases: [(&str, Option<f64>, &str); 4] = [
            ("fractional high precision value", Some(1000.5), "1000.5"),
            (
                "zero high precision value keeps positive ordinary volume",
                Some(0.0),
                "9007199254740993",
            ),
            (
                "non-finite high precision value falls back",
                Some(f64::NAN),
                "9007199254740993",
            ),
            (
                "missing high precision value falls back",
                None,
                "9007199254740993",
            ),
        ];
        for (name, hp_volume, expected) in cases {
            let mut basic = quote(11, "AAPL", 200.0, ORDINARY_VOLUME);
            basic.hp_volume = hp_volume;
            let ticks = basic_quote_ticks(vec![basic], 1_752_595_200_000, 1)
                .unwrap_or_else(|error| panic!("{name}: {error}"));
            assert_eq!(ticks[0].volume.to_string(), expected, "case: {name}");
        }
    }

    #[test]
    fn preserves_fractional_high_precision_volume() {
        let mut fractional_volume = quote(11, "AAPL", 1.0, 1);
        fractional_volume.hp_volume = Some(1.5);
        let ticks = basic_quote_ticks(vec![fractional_volume], 0, 1).expect("fractional volume");
        assert_eq!(ticks[0].volume.to_string(), "1.5");
    }

    #[test]
    fn preserves_basic_quote_display_and_market_fields_in_neutral_snapshot() {
        let mut value = quote(11, "AAPL", 189.25, 1_000);
        value.name = Some("Apple Inc.".to_owned());
        value.is_suspended = Some(false);
        value.open_price = Some(188.5);
        value.high_price = Some(190.0);
        value.low_price = Some(187.75);
        value.last_close_price = Some(187.0);
        value.turnover = Some(123_456.5);
        value.update_time = Some("15:59:59".to_owned());
        value.sec_status = Some(3);

        let ticks = basic_quote_ticks(vec![value], 42, 1).expect("rich tick");
        let snapshot = ticks[0].snapshot.as_ref().expect("rich snapshot");
        assert_eq!(snapshot.name.as_deref(), Some("Apple Inc."));
        assert_eq!(snapshot.is_suspended, Some(false));
        assert_eq!(snapshot.open_price.expect("open").to_string(), "188.5");
        assert_eq!(snapshot.high_price.expect("high").to_string(), "190");
        assert_eq!(snapshot.low_price.expect("low").to_string(), "187.75");
        assert_eq!(
            snapshot.previous_close.expect("previous close").to_string(),
            "187"
        );
        assert_eq!(
            snapshot.turnover.as_ref().expect("turnover").to_string(),
            "123456.5"
        );
        assert_eq!(snapshot.update_time.as_deref(), Some("15:59:59"));
        assert_eq!(snapshot.status, Some(3));
    }

    #[test]
    fn test_tick_conversion_rejects_unusable_prices_and_uses_quote_fallbacks() {
        // Parity: internal/integration/futu/marketdata_runtime_test.go:267 TestTickConversionRejectsUnusablePricesAndUsesQuoteFallbacks
        // 1. Invalid security dropped
        let mut bad_sec = quote(11, "AAPL", 100.0, 10);
        bad_sec.security = None;
        let ticks = basic_quote_ticks(vec![bad_sec], 100, 1).expect("mapped");
        assert!(ticks.is_empty());

        // 2. Zero price dropped
        let zero_price = quote(11, "AAPL", 0.0, 10);
        let ticks = basic_quote_ticks(vec![zero_price], 100, 1).expect("mapped");
        assert!(ticks.is_empty());

        // 3. Valid price preserves snapshot range fields
        let mut fallback_quote = quote(11, "MSFT", 401.10, 900);
        fallback_quote.open_price = Some(399.00);
        fallback_quote.high_price = Some(402.00);
        fallback_quote.low_price = Some(398.50);
        let ticks = basic_quote_ticks(vec![fallback_quote], 100, 1).expect("mapped");
        assert_eq!(ticks.len(), 1);
        assert_eq!(ticks[0].price.to_string(), "401.1");
        let snapshot = ticks[0].snapshot.as_ref().expect("snapshot");
        assert_eq!(snapshot.open_price.expect("open").to_string(), "399");
        assert_eq!(snapshot.high_price.expect("high").to_string(), "402");
        assert_eq!(snapshot.low_price.expect("low").to_string(), "398.5");
    }

    #[test]
    fn unusable_high_precision_volume_falls_back_to_the_required_int64_field() {
        let mut quote = quote(11, "AAPL", 1.0, 9);
        quote.hp_volume = Some(f64::INFINITY);
        let ticks = basic_quote_ticks(vec![quote], 0, 1).expect("fallback volume");
        assert_eq!(ticks[0].volume.to_string(), "9");
    }

    #[test]
    fn snapshot_projection_reclassifies_us_regular_boundary_and_clears_extended_hours() {
        // Parity: internal/integration/futu/marketdata_runtime_test.go:432 TestTickFromTickerReclassifiesUSRegularBoundary
        // 2026-01-07T16:00:00Z is 11:00 America/New_York, inside the US regular window.
        let ticks = basic_quote_ticks(vec![quote(11, "AAPL", 189.5, 0)], 1_767_801_600_000, 1)
            .expect("regular boundary tick");
        let snapshot = ticks[0].snapshot.as_ref().expect("snapshot");
        assert_eq!(snapshot.session.as_deref(), Some("regular"));
        assert_eq!(snapshot.trading_date.as_deref(), Some("2026-01-07"));
        assert!(snapshot.pre_market.is_none());
        assert!(snapshot.after_market.is_none());
        assert!(snapshot.overnight.is_none());
    }

    #[test]
    fn hk_lunch_snapshot_keeps_previous_close_and_does_not_mark_extended_hours() {
        // Parity: internal/integration/futu/marketdata_runtime_test.go:318 TestTickFromTickerPreservesHKPreviousCloseDuringLunchBreak
        let resolver = StaticSessionResolver(QuoteSessionContext {
            session: "closed".to_owned(),
            trading_date: "2026-06-12".to_owned(),
            timezone: "Asia/Hong_Kong".to_owned(),
            sessions: vec![
                QuoteSessionWindow {
                    kind: "regular".to_owned(),
                    start_minute: 570,
                    end_minute: 720,
                },
                QuoteSessionWindow {
                    kind: "regular".to_owned(),
                    start_minute: 780,
                    end_minute: 960,
                },
            ],
        });
        let mut lunch = quote(1, "00700", 701.1, 22222);
        lunch.last_close_price = Some(698.9);
        lunch.open_price = Some(700.1);
        lunch.high_price = Some(702.0);
        lunch.low_price = Some(699.0);
        let ticks =
            basic_quote_ticks_with_resolver(vec![lunch], 1_781_301_000_000, 1, Some(&resolver))
                .expect("lunch tick");
        let tick = &ticks[0];
        let snapshot = tick.snapshot.as_ref().expect("snapshot");
        assert_eq!(snapshot.session.as_deref(), Some("closed"));
        assert_eq!(
            snapshot.previous_close.expect("previous close").to_string(),
            "698.9"
        );
        assert!(
            snapshot.previous_close.expect("previous close") != tick.price,
            "previous close must not be overwritten by the current lunch price"
        );
        assert!(snapshot.pre_market.is_none() && snapshot.after_market.is_none());
    }

    /// Parity: go:452dea11:pkg/futu/quote_snapshot_test.go:183
    /// TestQuoteSnapshotPreviousClosePriceForHKLunchBreak
    ///
    /// HK has no US extended-hours model: during the lunch break the calendar
    /// resolves `closed`, `price` stays the latest traded price, and
    /// `previousClose` must remain the provider LastClosePrice so the change
    /// percentage does not collapse to zero.
    #[test]
    fn hk_lunch_break_keeps_provider_last_close_as_previous_close() {
        let resolver = StaticSessionResolver(QuoteSessionContext {
            session: "closed".to_owned(),
            trading_date: "2026-06-12".to_owned(),
            timezone: "Asia/Hong_Kong".to_owned(),
            sessions: vec![
                QuoteSessionWindow {
                    kind: "regular".to_owned(),
                    start_minute: 570,
                    end_minute: 720,
                },
                QuoteSessionWindow {
                    kind: "regular".to_owned(),
                    start_minute: 780,
                    end_minute: 960,
                },
            ],
        });
        let mut lunch = quote(1, "00700", 321.40, 12_345_678);
        lunch.last_close_price = Some(318.90);
        lunch.open_price = Some(320.00);
        lunch.high_price = Some(322.20);
        lunch.low_price = Some(319.80);
        lunch.turnover = Some(3_955_555_555.0);
        let ticks =
            basic_quote_ticks_with_resolver(vec![lunch], 1_781_238_600_000, 1, Some(&resolver))
                .expect("HK lunch tick");
        let tick = &ticks[0];
        let snapshot = tick.snapshot.as_ref().expect("snapshot");
        assert_eq!(snapshot.session.as_deref(), Some("closed"));
        assert_eq!(tick.price.to_string(), "321.4");
        assert_eq!(
            snapshot.previous_close.expect("previous close").to_string(),
            "318.9"
        );
        assert!(
            snapshot.previous_close.expect("previous close") != tick.price,
            "Price and PreviousClosePrice must not collapse to the same value"
        );
    }

    #[test]
    fn resolver_keeps_hk_lunch_closed_in_snapshot_projection() {
        let resolver = StaticSessionResolver(QuoteSessionContext {
            session: "closed".to_owned(),
            trading_date: "2026-06-22".to_owned(),
            timezone: "Asia/Hong_Kong".to_owned(),
            sessions: vec![
                QuoteSessionWindow {
                    kind: "regular".to_owned(),
                    start_minute: 570,
                    end_minute: 720,
                },
                QuoteSessionWindow {
                    kind: "regular".to_owned(),
                    start_minute: 780,
                    end_minute: 960,
                },
            ],
        });
        let ticks = basic_quote_ticks_with_resolver(
            vec![quote(1, "00700", 300.0, 1)],
            1_766_000_000_000,
            3,
            Some(&resolver),
        )
        .expect("mapped lunch tick");
        let snapshot = ticks[0].snapshot.as_ref().expect("snapshot");
        assert_eq!(snapshot.session.as_deref(), Some("closed"));
        assert_eq!(snapshot.trading_date.as_deref(), Some("2026-06-22"));
    }

    #[test]
    fn resolver_projects_us_early_close_extended_window_with_timezone() {
        let resolver = StaticSessionResolver(QuoteSessionContext {
            session: "regular".to_owned(),
            trading_date: "2026-07-02".to_owned(),
            timezone: "America/New_York".to_owned(),
            sessions: vec![
                QuoteSessionWindow {
                    kind: "overnight".to_owned(),
                    start_minute: 0,
                    end_minute: 240,
                },
                QuoteSessionWindow {
                    kind: "pre".to_owned(),
                    start_minute: 240,
                    end_minute: 570,
                },
                QuoteSessionWindow {
                    kind: "regular".to_owned(),
                    start_minute: 570,
                    end_minute: 780,
                },
                QuoteSessionWindow {
                    kind: "after".to_owned(),
                    start_minute: 780,
                    end_minute: 1080,
                },
            ],
        });
        let mut value = quote(11, "AAPL", 189.25, 1);
        let extended = || PreAfterMarketData {
            price: Some(188.0),
            high_price: None,
            low_price: None,
            volume: Some(1),
            turnover: None,
            change_value: None,
            change_rate: None,
            amplitude: None,
        };
        value.pre_market = Some(extended());
        value.after_market = Some(extended());
        value.overnight = Some(extended());
        let ticks =
            basic_quote_ticks_with_resolver(vec![value], 1_767_000_000_000, 4, Some(&resolver))
                .expect("mapped early-close tick");
        let snapshot = ticks[0].snapshot.as_ref().expect("snapshot");
        assert_eq!(snapshot.session.as_deref(), Some("regular"));
        assert_eq!(snapshot.last_close, None);
        let after = snapshot.after_market.as_ref().expect("after block");
        assert_eq!(after.trading_date.as_deref(), Some("2026-07-02"));
        assert_eq!(after.exchange_timezone.as_deref(), Some("America/New_York"));
        assert_eq!(
            after.session_start_at.as_deref(),
            Some("2026-07-02T17:00:00Z")
        );
        assert_eq!(
            after.session_end_at.as_deref(),
            Some("2026-07-02T22:00:00Z")
        );
    }

    #[test]
    fn extended_quote_blocks_project_go_field_and_window_contract() {
        // Parity: internal/integration/futu/marketdata_runtime_test.go:712 TestTickFromSnapshotMapsExtendedQuoteFields
        let resolver = StaticSessionResolver(QuoteSessionContext {
            session: "after".to_owned(),
            trading_date: "2026-06-23".to_owned(),
            timezone: "America/New_York".to_owned(),
            sessions: vec![
                QuoteSessionWindow {
                    kind: "overnight".to_owned(),
                    start_minute: 0,
                    end_minute: 240,
                },
                QuoteSessionWindow {
                    kind: "pre".to_owned(),
                    start_minute: 240,
                    end_minute: 570,
                },
                QuoteSessionWindow {
                    kind: "regular".to_owned(),
                    start_minute: 570,
                    end_minute: 960,
                },
                QuoteSessionWindow {
                    kind: "after".to_owned(),
                    start_minute: 960,
                    end_minute: 1200,
                },
            ],
        });
        let mut value = quote(11, "AAPL", 321.2, 9_007_199_254_740_993);
        value.last_close_price = Some(316.2);
        value.open_price = Some(318.0);
        value.high_price = Some(322.0);
        value.low_price = Some(317.8);
        value.turnover = Some(999_999.9);
        value.pre_market = Some(PreAfterMarketData {
            price: Some(320.1),
            high_price: Some(321.8),
            low_price: Some(319.5),
            volume: Some(4567),
            turnover: Some(12_345.6),
            change_value: Some(2.3),
            change_rate: Some(0.72),
            amplitude: Some(1.1),
        });
        value.after_market = Some(PreAfterMarketData {
            price: Some(322.4),
            high_price: None,
            low_price: None,
            volume: None,
            turnover: None,
            change_value: None,
            change_rate: None,
            amplitude: None,
        });
        value.overnight = Some(PreAfterMarketData {
            price: Some(323.7),
            high_price: None,
            low_price: None,
            volume: None,
            turnover: None,
            change_value: None,
            change_rate: None,
            amplitude: None,
        });
        let ticks =
            basic_quote_ticks_with_resolver(vec![value], 1_782_615_301_000, 5, Some(&resolver))
                .expect("mapped extended tick");
        let snapshot = ticks[0].snapshot.as_ref().expect("snapshot");
        assert_eq!(snapshot.session.as_deref(), Some("after"));
        let pre = snapshot.pre_market.as_ref().expect("pre block");
        assert_eq!(pre.price.expect("pre price").to_string(), "320.1");
        assert_eq!(pre.high_price.expect("pre high").to_string(), "321.8");
        assert_eq!(pre.low_price.expect("pre low").to_string(), "319.5");
        assert_eq!(pre.volume.as_ref().expect("pre volume").to_string(), "4567");
        assert_eq!(
            pre.turnover.as_ref().expect("pre turnover").to_string(),
            "12345.6"
        );
        assert_eq!(pre.change.as_ref().expect("pre change").to_string(), "2.3");
        assert_eq!(
            pre.change_rate
                .as_ref()
                .expect("pre change rate")
                .to_string(),
            "0.72"
        );
        assert_eq!(
            pre.amplitude.as_ref().expect("pre amplitude").to_string(),
            "1.1"
        );
        assert_eq!(pre.trading_date.as_deref(), Some("2026-06-23"));
        assert_eq!(pre.exchange_timezone.as_deref(), Some("America/New_York"));
        assert_eq!(
            pre.session_start_at.as_deref(),
            Some("2026-06-23T08:00:00Z")
        );
        assert_eq!(pre.session_end_at.as_deref(), Some("2026-06-23T13:30:00Z"));
        let after = snapshot.after_market.as_ref().expect("after block");
        assert_eq!(after.price.expect("after price").to_string(), "322.4");
        assert_eq!(
            after.session_end_at.as_deref(),
            Some("2026-06-24T00:00:00Z")
        );
        let overnight = snapshot.overnight.as_ref().expect("overnight block");
        assert_eq!(
            overnight.price.expect("overnight price").to_string(),
            "323.7"
        );
        assert_eq!(
            overnight.session_start_at.as_deref(),
            Some("2026-06-23T00:00:00Z")
        );
        assert_eq!(
            overnight.session_end_at.as_deref(),
            Some("2026-06-23T08:00:00Z")
        );
    }

    #[test]
    fn extended_block_without_a_calendar_window_stays_unannotated() {
        // Parity: internal/integration/futu/marketdata_runtime_test.go:712 TestTickFromSnapshotMapsExtendedQuoteFields
        // The HK template has no pre-market window, so Go's
        // ResolveTradingDaySessionWindow fails and attachFutuSessionWindow
        // leaves the block completely unannotated.  Publishing a trading date
        // for a session the exchange never opens would be a false contract.
        let resolver = StaticSessionResolver(QuoteSessionContext {
            session: "regular".to_owned(),
            trading_date: "2026-06-23".to_owned(),
            timezone: "Asia/Hong_Kong".to_owned(),
            sessions: vec![
                QuoteSessionWindow {
                    kind: "regular".to_owned(),
                    start_minute: 570,
                    end_minute: 720,
                },
                QuoteSessionWindow {
                    kind: "regular".to_owned(),
                    start_minute: 780,
                    end_minute: 960,
                },
            ],
        });
        let mut value = quote(1, "00700", 300.0, 1);
        value.pre_market = Some(PreAfterMarketData {
            price: Some(299.0),
            high_price: None,
            low_price: None,
            volume: Some(10),
            turnover: None,
            change_value: None,
            change_rate: None,
            amplitude: None,
        });
        let ticks =
            basic_quote_ticks_with_resolver(vec![value], 1_782_600_000_000, 6, Some(&resolver))
                .expect("mapped hk tick");
        let snapshot = ticks[0].snapshot.as_ref().expect("snapshot");
        let pre = snapshot.pre_market.as_ref().expect("pre block");
        assert_eq!(
            pre.trading_date, None,
            "missing window must stay unannotated"
        );
        assert_eq!(pre.exchange_timezone, None);
        assert_eq!(pre.session_start_at, None);
        assert_eq!(pre.session_end_at, None);
    }
}
