//! Provider-neutral conversion of cumulative quote volume into per-event
//! trade quantity.
//!
//! Parity: `pkg/futu/stream.go` (`Stream.nextTradeQuantity`) and
//! `Stream.emitBasicQotSnapshot`.
//!
//! OpenD's BasicQot push carries a cumulative session volume, not the size of
//! the trade that produced it. Go derives the per-event quantity by comparing
//! the sample with the previous one for the same symbol, trading day and
//! session, and publishes `0` whenever there is no usable baseline:
//!
//! * the first sample for a symbol,
//! * the first sample of a new trading day or session,
//! * a sample whose cumulative counter moved backwards,
//! * a negative cumulative counter.
//!
//! The subtraction runs on arbitrary-precision decimals so a counter beyond
//! IEEE-754 integer range (Go's `9_007_199_254_740_995` fixture) keeps an exact
//! delta instead of losing the low bits.

use std::collections::BTreeMap;
use std::str::FromStr;

use jftrade_kernel::DecimalText;

use crate::cache::trading_day_key;

#[derive(Clone, Debug, Eq, PartialEq)]
struct TradeVolumeSample {
    trading_day: String,
    session: String,
    cumulative: DecimalText,
}

/// Tracks the last cumulative volume per instrument so each push can publish
/// the exact per-event quantity.
///
/// The tracker owns no clock and no I/O: the caller supplies the observation
/// time and session label it already derived from the provider calendar, so
/// polling and streaming share one definition of "same trading day".
#[derive(Debug, Default)]
pub struct TradeVolumeTracker {
    samples: BTreeMap<String, TradeVolumeSample>,
}

impl TradeVolumeTracker {
    pub fn new() -> Self {
        Self::default()
    }

    /// Returns the per-event quantity for one cumulative volume sample.
    ///
    /// `session` is the already-resolved broker-neutral session label
    /// (`regular`, `pre`, `after`, `overnight`, `closed`). Observations whose
    /// session changed reset the baseline, matching Go's key on
    /// `(tradingDay, session)`.
    pub fn next_quantity(
        &mut self,
        instrument_id: &str,
        observed_at_ms: i64,
        session: &str,
        cumulative: &DecimalText,
    ) -> DecimalText {
        if parsed(cumulative).is_sign_negative() {
            return zero();
        }
        let trading_day = trading_day_key(instrument_id, observed_at_ms)
            .unwrap_or_else(|| observed_at_ms.to_string());
        let key = instrument_id.trim().to_ascii_uppercase();
        let previous = self.samples.insert(
            key,
            TradeVolumeSample {
                trading_day: trading_day.clone(),
                session: session.to_owned(),
                cumulative: cumulative.clone(),
            },
        );
        let Some(previous) = previous else {
            return zero();
        };
        if previous.trading_day != trading_day || previous.session != session {
            return zero();
        }
        subtract(cumulative, &previous.cumulative)
    }
}

fn subtract(current: &DecimalText, previous: &DecimalText) -> DecimalText {
    let (Some(current), Some(previous)) = (decimal(current), decimal(previous)) else {
        return zero();
    };
    subtract_decimals(current, previous)
}

fn subtract_decimals(
    current: rust_decimal::Decimal,
    previous: rust_decimal::Decimal,
) -> DecimalText {
    if current <= previous {
        return zero();
    }
    DecimalText::from_str(&(current - previous).to_string()).unwrap_or_else(|_| zero())
}

fn parsed(value: &DecimalText) -> rust_decimal::Decimal {
    decimal(value).unwrap_or(rust_decimal::Decimal::ZERO)
}

fn decimal(value: &DecimalText) -> Option<rust_decimal::Decimal> {
    rust_decimal::Decimal::from_str(value.as_str()).ok()
}

fn zero() -> DecimalText {
    "0".parse::<DecimalText>()
        .expect("zero is valid decimal text")
}

#[cfg(test)]
mod tests {
    use super::*;

    fn volume(value: &str) -> DecimalText {
        value.parse().expect("volume")
    }

    // Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:143
    // TestStreamConvertsCumulativeQuoteVolumeToIncrementalTradeQuantity
    #[test]
    fn first_sample_is_a_baseline_and_decreases_or_negatives_report_zero() {
        let mut tracker = TradeVolumeTracker::new();
        // 2026-07-20 10:00 Asia/Hong_Kong.
        let first = 1_784_557_200_000_i64;
        assert_eq!(
            tracker.next_quantity("HK.00700", first, "regular", &volume("1000")),
            volume("0"),
            "the first cumulative sample is only a baseline"
        );
        assert_eq!(
            tracker.next_quantity("HK.00700", first + 1_000, "regular", &volume("1015")),
            volume("15")
        );
        assert_eq!(
            tracker.next_quantity("HK.00700", first + 2_000, "regular", &volume("1010")),
            volume("0"),
            "a decreasing counter reports zero instead of a negative quantity"
        );
        assert_eq!(
            tracker.next_quantity("HK.00700", first + 3_000, "regular", &volume("-1")),
            volume("0"),
            "a negative counter reports zero"
        );
        // The rejected sample must not disturb the retained baseline.
        assert_eq!(
            tracker.next_quantity("HK.00700", first + 4_000, "regular", &volume("1020")),
            volume("10")
        );
    }

    #[test]
    fn a_new_trading_day_or_session_resets_the_cumulative_baseline() {
        let mut tracker = TradeVolumeTracker::new();
        let first = 1_784_557_200_000_i64;
        assert_eq!(
            tracker.next_quantity("HK.00700", first, "regular", &volume("1000")),
            volume("0")
        );
        assert_eq!(
            tracker.next_quantity("HK.00700", first + 1_000, "regular", &volume("1015")),
            volume("15")
        );
        // Same instant, different session: the counter restarts at zero for the
        // new session, so the first sample is a baseline again.
        assert_eq!(
            tracker.next_quantity("HK.00700", first + 2_000, "after", &volume("5")),
            volume("0"),
            "a session change resets the baseline"
        );
        let next_day = first + 86_400_000;
        assert_eq!(
            tracker.next_quantity("HK.00700", next_day, "regular", &volume("25")),
            volume("0"),
            "a new trading day resets the baseline"
        );
    }

    #[test]
    fn fractional_and_out_of_fixedpoint_counters_keep_their_exact_delta() {
        // Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:174
        // TestStreamPreservesFractionalCumulativeVolumeDelta
        let mut tracker = TradeVolumeTracker::new();
        let at = 1_784_557_200_000_i64;
        assert_eq!(
            tracker.next_quantity("US.AAPL", at, "regular", &volume("1000.5")),
            volume("0")
        );
        assert_eq!(
            tracker.next_quantity("US.AAPL", at + 1_000, "regular", &volume("1000.75")),
            volume("0.25")
        );

        // Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:215
        // TestStreamMarketTradePreservesVolumeBeyondLegacyFixedpointRange
        let mut tracker = TradeVolumeTracker::new();
        assert_eq!(
            tracker.next_quantity("HK.00700", at, "regular", &volume("9007199254740993")),
            volume("0")
        );
        assert_eq!(
            tracker.next_quantity(
                "HK.00700",
                at + 1_000,
                "regular",
                &volume("9007199254740995")
            ),
            volume("2"),
            "the delta stays exact beyond 2^53"
        );
    }

    #[test]
    fn instruments_are_tracked_independently() {
        let mut tracker = TradeVolumeTracker::new();
        let at = 1_784_557_200_000_i64;
        assert_eq!(
            tracker.next_quantity("HK.00700", at, "regular", &volume("1000")),
            volume("0")
        );
        assert_eq!(
            tracker.next_quantity("HK.09988", at, "regular", &volume("500")),
            volume("0"),
            "a second instrument gets its own baseline"
        );
        assert_eq!(
            tracker.next_quantity("HK.00700", at + 1_000, "regular", &volume("1012")),
            volume("12")
        );
        assert_eq!(
            tracker.next_quantity("HK.09988", at + 1_000, "regular", &volume("509")),
            volume("9")
        );
    }
}
