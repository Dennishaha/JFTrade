//! Provider-neutral projection of retained tick samples onto the
//! `period=tick` candle contract.
//!
//! Parity: `internal/marketdata/candles.go` (`TickCandles`) and the tick
//! branch of `internal/marketdata/service.go` (`GetCandles`).
//!
//! Go serves a `period=tick` request from the same tick cache the snapshot and
//! live paths use. It never asks the provider for historical K-lines: when the
//! cache holds no fresh sample it queries the provider ticker once and ingests
//! the result, and when that query fails it still answers from whatever the
//! cache retained. Keeping the projection here means the candle reader, the
//! live tick reader and the snapshot reader cannot drift apart.

use jftrade_kernel::{Decimal, DecimalText};
use serde_json::{Value, json};

use crate::Tick;

/// One projected tick candle.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct TickCandle {
    pub observed_at_ms: i64,
    pub price: Decimal,
    pub volume: DecimalText,
    pub session: Option<String>,
}

impl TickCandle {
    /// Renders the Go `TickCandles` map shape. A tick candle repeats the same
    /// price for open/high/low/close because one tick cannot span a range.
    ///
    /// Prices go through `DecimalText` so the wire value matches Go's
    /// `decimal.String()`: `"102.30"` is published as `"102.3"` rather than
    /// echoing the provider's trailing zero.
    pub fn to_value(&self, observed_at: &str) -> Value {
        let price = canonical_price(&self.price);
        json!({
            "at": observed_at,
            "close": price,
            "high": price,
            "low": price,
            "open": price,
            "period": "tick",
            "session": self.session,
            "volume": self.volume.as_str(),
        })
    }
}

fn canonical_price(price: &Decimal) -> String {
    price
        .to_string()
        .parse::<DecimalText>()
        .map(|value| value.as_str().to_owned())
        .unwrap_or_else(|_| price.to_string())
}

/// Default lookback Go applies when a tick request omits `fromTime`.
const DEFAULT_TICK_WINDOW_MS: i64 = 15 * 60 * 1000;

/// Projects retained ticks onto tick candles.
///
/// `from_ms`/`to_ms` are the inclusive bounds the caller derived from
/// `fromTime`/`toTime`. An omitted `to` defaults to `now_ms`; an omitted `from`
/// defaults to 15 minutes before `to`. `limit == 0` keeps every sample in the
/// window, and a non-zero limit keeps the newest candles, matching Go's
/// `limitCandleMaps`.
pub fn tick_candles(
    samples: &[Tick],
    from_ms: Option<i64>,
    to_ms: Option<i64>,
    now_ms: i64,
    limit: usize,
) -> Vec<TickCandle> {
    let to = to_ms.unwrap_or(now_ms);
    let from = from_ms.unwrap_or_else(|| to.saturating_sub(DEFAULT_TICK_WINDOW_MS));

    let mut candles = Vec::with_capacity(samples.len());
    for sample in samples {
        if sample.observed_at_ms < from || sample.observed_at_ms > to {
            continue;
        }
        candles.push(TickCandle {
            observed_at_ms: sample.observed_at_ms,
            price: sample.price,
            // Go publishes the tick's `VolumeDelta`, not its cumulative
            // counter, and clamps a negative delta to zero so a provider
            // correction can never emit a negative-volume candle. A quote
            // snapshot without an explicit delta reports zero, matching Go's
            // zero-value `VolumeDelta`.
            volume: sample
                .volume_delta
                .as_ref()
                .map(non_negative_volume)
                .unwrap_or_else(zero_volume),
            session: sample
                .snapshot
                .as_ref()
                .and_then(|snapshot| snapshot.session.clone()),
        });
    }
    if limit > 0 && candles.len() > limit {
        candles = candles.split_off(candles.len() - limit);
    }
    candles
}

fn non_negative_volume(volume: &DecimalText) -> DecimalText {
    if volume
        .as_str()
        .trim()
        .parse::<Decimal>()
        .is_ok_and(|parsed| parsed.is_sign_negative())
    {
        zero_volume()
    } else {
        volume.clone()
    }
}

fn zero_volume() -> DecimalText {
    "0".parse::<DecimalText>()
        .expect("zero is valid decimal text")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::TradeQuoteSnapshot;

    fn tick(
        price: &str,
        volume_delta: Option<&str>,
        observed_at_ms: i64,
        session: Option<&str>,
    ) -> Tick {
        Tick {
            instrument_id: "HK.00700".to_owned(),
            price: price.parse().expect("price"),
            volume: "100".parse().expect("cumulative volume"),
            volume_delta: volume_delta.map(|value| value.parse().expect("delta")),
            snapshot: session.map(|session| TradeQuoteSnapshot {
                session: Some(session.to_owned()),
                ..Default::default()
            }),
            observed_at_ms,
            provider_generation: 1,
        }
    }

    #[test]
    fn tick_candles_default_to_a_fifteen_minute_window_and_clamp_negative_volume() {
        // Parity: internal/marketdata/cache_test.go:207 TestTickCandlesVolumeWindowAndLimit
        let now = 1_760_000_000_000_i64;
        let samples = vec![
            tick("100.10", Some("100"), now - 16 * 60_000, Some("regular")),
            tick("101.20", Some("50"), now - 2 * 60_000, Some("regular")),
            tick("102.30", Some("0"), now - 60_000, Some("regular")),
            tick("103.40", Some("-1"), now, Some("regular")),
        ];

        let unlimited = tick_candles(&samples, None, Some(now), now, 0);
        assert_eq!(
            unlimited.len(),
            3,
            "the default 15 minute window drops the oldest sample"
        );
        assert_eq!(
            unlimited[0].to_value("2026-06-14T10:00:00Z")["open"],
            "101.2",
            "trailing zeros are trimmed like Go's decimal.String()"
        );

        let limited = tick_candles(&samples, None, Some(now), now, 2);
        assert_eq!(limited.len(), 2, "limit keeps the newest candles");
        assert_eq!(limited[0].to_value("2026-06-14T10:00:00Z")["open"], "102.3");
        assert_eq!(limited[0].volume.as_str(), "0", "a zero delta stays zero");
        assert_eq!(limited[1].to_value("2026-06-14T10:00:00Z")["open"], "103.4");
        assert_eq!(
            limited[1].volume.as_str(),
            "0",
            "a negative delta is clamped to zero"
        );
        assert_eq!(limited[0].session.as_deref(), Some("regular"));
    }

    #[test]
    fn tick_candles_without_an_explicit_delta_report_zero_volume() {
        // A quote snapshot carries a cumulative counter but no per-event
        // quantity, so Go's zero-value `VolumeDelta` reports `0`.
        let now = 1_760_000_000_000_i64;
        let samples = vec![tick("100", None, now, Some("regular"))];
        let candles = tick_candles(&samples, None, Some(now), now, 0);
        assert_eq!(candles.len(), 1);
        assert_eq!(candles[0].volume.as_str(), "0");
    }

    #[test]
    fn tick_candles_use_explicit_volume_delta_across_trading_days() {
        // Parity: internal/marketdata/cache_test.go:241
        // TestTickCandlesUsesExplicitVolumeDeltaAcrossTradingDays
        //
        // The candle volume is the provider's explicit per-event delta, never
        // the difference between two cumulative counters, so a session reset
        // (10_000 yesterday, 250 today) still reports 12 then 7.
        let previous_day = 1_753_012_800_000_i64;
        let current_day = previous_day + 18 * 3_600_000;
        let samples = vec![
            tick("100", Some("12"), previous_day, Some("regular")),
            tick("101", Some("7"), current_day, Some("regular")),
        ];
        let candles = tick_candles(
            &samples,
            Some(previous_day - 1_000),
            Some(current_day + 1_000),
            current_day,
            0,
        );
        assert_eq!(candles.len(), 2);
        assert_eq!(candles[0].volume.as_str(), "12");
        assert_eq!(candles[1].volume.as_str(), "7");
    }

    #[test]
    fn tick_candles_respect_explicit_bounds() {
        let now = 1_760_000_000_000_i64;
        let samples = vec![
            tick("100", Some("1"), now - 5_000, Some("regular")),
            tick("101", Some("1"), now - 3_000, Some("regular")),
            tick("102", Some("1"), now - 1_000, Some("regular")),
        ];
        let candles = tick_candles(&samples, Some(now - 4_000), Some(now - 2_000), now, 0);
        assert_eq!(candles.len(), 1, "only the in-window sample survives");
        assert_eq!(candles[0].to_value("2026-06-14T10:00:00Z")["open"], "101");
    }
}
