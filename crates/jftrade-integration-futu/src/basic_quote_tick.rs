use std::collections::BTreeMap;
use std::str::FromStr;

use jftrade_kernel::{DecimalText, Fixed8};
use jftrade_marketdata::Tick;
use thiserror::Error;

use crate::{BasicQuote, Security};

#[derive(Clone, Debug, Error, Eq, PartialEq)]
pub enum BasicQuoteTickError {
    #[error("OpenD BasicQot price is not finite for {instrument_id}")]
    NonFinitePrice { instrument_id: String },
    #[error("OpenD BasicQot price is outside Fixed8 range for {instrument_id}: {price}")]
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
        let price = fixed8_from_price(&instrument_id, price)?;
        let volume = collector_volume(&instrument_id, &quote)?;
        ticks.insert(
            instrument_id.clone(),
            Tick {
                instrument_id,
                price,
                volume,
                observed_at_ms,
                provider_generation,
            },
        );
    }
    Ok(ticks.into_values().collect())
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

fn fixed8_from_price(instrument_id: &str, price: f64) -> Result<Fixed8, BasicQuoteTickError> {
    if !price.is_finite() {
        return Err(BasicQuoteTickError::NonFinitePrice {
            instrument_id: instrument_id.to_owned(),
        });
    }
    let price = price.to_string();
    Fixed8::from_str(&price).map_err(|_| BasicQuoteTickError::PriceOutOfRange {
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
    use serde::Deserialize;

    use super::*;

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
        assert_eq!(ticks[1].price.to_string(), "189.12345678");
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
            "../../../tests/fixtures/rust-migration/stage9/basic-quote-nonfinite.json"
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

    #[test]
    fn preserves_fractional_high_precision_volume() {
        let mut fractional_volume = quote(11, "AAPL", 1.0, 1);
        fractional_volume.hp_volume = Some(1.5);
        let ticks = basic_quote_ticks(vec![fractional_volume], 0, 1).expect("fractional volume");
        assert_eq!(ticks[0].volume.to_string(), "1.5");
    }

    #[test]
    fn unusable_high_precision_volume_falls_back_to_the_required_int64_field() {
        let mut quote = quote(11, "AAPL", 1.0, 9);
        quote.hp_volume = Some(f64::INFINITY);
        let ticks = basic_quote_ticks(vec![quote], 0, 1).expect("fallback volume");
        assert_eq!(ticks[0].volume.to_string(), "9");
    }
}
