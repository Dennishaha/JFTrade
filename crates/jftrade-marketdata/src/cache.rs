use std::collections::{BTreeMap, VecDeque};

use jiff::Timestamp;

use crate::{MarketDataError, Tick};

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum CacheLookup {
    Fresh(Tick),
    Stale(Tick),
    Missing,
}

#[derive(Clone, Debug)]
pub struct TickCache {
    capacity_per_instrument: usize,
    ticks: BTreeMap<String, VecDeque<Tick>>,
}

impl TickCache {
    pub fn new(capacity_per_instrument: usize) -> Self {
        Self {
            capacity_per_instrument: capacity_per_instrument.max(1),
            ticks: BTreeMap::new(),
        }
    }

    pub fn insert(&mut self, tick: Tick, active_generation: u64) -> Result<(), MarketDataError> {
        if tick.provider_generation != active_generation {
            return Err(MarketDataError::ProviderChanged);
        }
        let instrument_id = tick.instrument_id.trim().to_ascii_uppercase();
        if instrument_id.is_empty() {
            return Err(MarketDataError::InvalidSubscription(
                "tick instrumentId is required".to_owned(),
            ));
        }
        let entries = self.ticks.entry(instrument_id).or_default();
        if let Some(last) = entries.back()
            && tick.observed_at_ms < last.observed_at_ms
        {
            return Err(MarketDataError::InvalidSubscription(
                "tick timestamp moved backwards".to_owned(),
            ));
        }
        let mut tick = tick;
        if let Some(last) = entries.back().cloned() {
            inherit_snapshot_context(&mut tick, &last);
            if is_duplicate_quote(&tick, &last) {
                return Ok(());
            }
        }
        entries.push_back(tick);
        while entries.len() > self.capacity_per_instrument {
            entries.pop_front();
        }
        Ok(())
    }

    pub fn lookup(&self, instrument_id: &str, now_ms: i64, max_age_ms: i64) -> CacheLookup {
        let Some(tick) = self
            .ticks
            .get(&instrument_id.trim().to_ascii_uppercase())
            .and_then(|entries| entries.back())
            .cloned()
        else {
            return CacheLookup::Missing;
        };
        // A wall-clock rollback must never make a future observation appear
        // fresh. `saturating_sub` intentionally protects the age arithmetic
        // from overflow, but by itself would turn a negative age into zero.
        if max_age_ms >= 0
            && now_ms >= tick.observed_at_ms
            && now_ms.saturating_sub(tick.observed_at_ms) <= max_age_ms
        {
            CacheLookup::Fresh(tick)
        } else {
            CacheLookup::Stale(tick)
        }
    }

    /// Looks up a sample only when it belongs to the active provider
    /// generation. A sample from another generation is treated as missing so
    /// callers fail closed instead of displaying or reusing a fresh value
    /// from a previous provider/session.
    pub fn lookup_for_generation(
        &self,
        instrument_id: &str,
        now_ms: i64,
        max_age_ms: i64,
        active_generation: u64,
    ) -> CacheLookup {
        match self.lookup(instrument_id, now_ms, max_age_ms) {
            CacheLookup::Fresh(tick) if tick.provider_generation == active_generation => {
                CacheLookup::Fresh(tick)
            }
            CacheLookup::Stale(tick) if tick.provider_generation == active_generation => {
                CacheLookup::Stale(tick)
            }
            CacheLookup::Fresh(_) | CacheLookup::Stale(_) => CacheLookup::Missing,
            CacheLookup::Missing => CacheLookup::Missing,
        }
    }

    pub fn require_fresh(
        &self,
        instrument_id: &str,
        now_ms: i64,
        max_age_ms: i64,
    ) -> Result<Tick, MarketDataError> {
        match self.lookup(instrument_id, now_ms, max_age_ms) {
            CacheLookup::Fresh(tick) => Ok(tick),
            CacheLookup::Stale(_) => Err(MarketDataError::CacheStale(instrument_id.to_owned())),
            CacheLookup::Missing => Err(MarketDataError::CacheMiss(instrument_id.to_owned())),
        }
    }

    /// Requires a fresh sample from the active provider generation.
    pub fn require_fresh_for_generation(
        &self,
        instrument_id: &str,
        now_ms: i64,
        max_age_ms: i64,
        active_generation: u64,
    ) -> Result<Tick, MarketDataError> {
        match self.lookup_for_generation(instrument_id, now_ms, max_age_ms, active_generation) {
            CacheLookup::Fresh(tick) => Ok(tick),
            CacheLookup::Stale(_) => Err(MarketDataError::CacheStale(instrument_id.to_owned())),
            CacheLookup::Missing => Err(MarketDataError::CacheMiss(instrument_id.to_owned())),
        }
    }

    pub fn clear(&mut self) {
        self.ticks.clear();
    }

    /// Returns every retained sample for one instrument in observation order.
    ///
    /// Go's cache exposes `Snapshot(instrumentID)` to the tick-candle reader so
    /// a `period=tick` request projects the retained window instead of only the
    /// freshest sample. Samples are cloned so the caller never holds the cache
    /// lock while a response is assembled.
    pub fn history(&self, instrument_id: &str) -> Vec<Tick> {
        self.ticks
            .get(&instrument_id.trim().to_ascii_uppercase())
            .map(|entries| entries.iter().cloned().collect())
            .unwrap_or_default()
    }

    pub fn instrument_count(&self) -> usize {
        self.ticks.len()
    }
}

// Parity: go:452dea11:internal/marketdata/cache_test.go:12 TestCacheDeduplicatesPromotesAndInherits
fn is_duplicate_quote(incoming: &Tick, latest: &Tick) -> bool {
    // Only snapshots with a provider quote time identify the same observation.
    // Trade deltas are separate events even at the same price and quantity.
    incoming.volume_delta.is_none()
        && latest.volume_delta.is_none()
        && incoming.snapshot.as_ref().is_some_and(|snapshot| {
            snapshot
                .update_time
                .as_deref()
                .is_some_and(|time| !time.is_empty())
        })
        && incoming.provider_generation == latest.provider_generation
        && incoming.price == latest.price
        && incoming.volume == latest.volume
        && incoming.snapshot == latest.snapshot
}

/// Preserve the context that BasicQot deliberately omits from trade pushes.
/// The Go cache performs this merge before deciding whether an observation is
/// equivalent; keeping it in the provider-neutral cache means snapshots from
/// polling and streaming follow the same close/session rules.
fn inherit_snapshot_context(incoming: &mut Tick, latest: &Tick) {
    let same_trading_day = shares_trading_day(incoming, latest);
    let (Some(incoming_snapshot), Some(latest_snapshot)) =
        (incoming.snapshot.as_mut(), latest.snapshot.as_ref())
    else {
        return;
    };

    if incoming_snapshot.open_price.is_none() {
        incoming_snapshot.open_price = latest_snapshot.open_price;
    }
    if incoming_snapshot.high_price.is_none() {
        incoming_snapshot.high_price = latest_snapshot.high_price;
    }
    if incoming_snapshot.low_price.is_none() {
        incoming_snapshot.low_price = latest_snapshot.low_price;
    }
    if incoming_snapshot.previous_close.is_none() {
        if should_promote_regular_close(
            &incoming.instrument_id,
            incoming_snapshot,
            latest_snapshot,
            same_trading_day,
        ) {
            incoming_snapshot.previous_close = latest_snapshot.last_price;
        } else {
            incoming_snapshot.previous_close = latest_snapshot.previous_close;
        }
    }
    if incoming_snapshot.last_close.is_none() {
        incoming_snapshot.last_close = latest_snapshot.last_close;
    }
    if same_trading_day {
        if incoming_snapshot.pre_market.is_none() {
            incoming_snapshot.pre_market = latest_snapshot.pre_market.clone();
        }
        if incoming_snapshot.after_market.is_none() {
            incoming_snapshot.after_market = latest_snapshot.after_market.clone();
        }
        if incoming_snapshot.overnight.is_none() {
            incoming_snapshot.overnight = latest_snapshot.overnight.clone();
        }
        if incoming_snapshot
            .session
            .as_deref()
            .is_none_or(|value| value.trim().is_empty() || value.eq_ignore_ascii_case("unknown"))
        {
            incoming_snapshot.session = latest_snapshot.session.clone();
        }
    }
    if incoming_snapshot.trading_date.is_none() {
        incoming_snapshot.trading_date = latest_snapshot.trading_date.clone();
    }
    if incoming_snapshot.symbol.is_none() {
        incoming_snapshot.symbol = latest_snapshot.symbol.clone();
    }
    if incoming_snapshot.market.is_none() {
        incoming_snapshot.market = latest_snapshot.market.clone();
    }
}

fn should_promote_regular_close(
    instrument_id: &str,
    incoming: &crate::TradeQuoteSnapshot,
    latest: &crate::TradeQuoteSnapshot,
    same_trading_day: bool,
) -> bool {
    same_trading_day
        && is_us_symbol(instrument_id)
        && latest
            .session
            .as_deref()
            .is_some_and(|value| value.eq_ignore_ascii_case("regular"))
        && incoming
            .session
            .as_deref()
            .is_some_and(|value| value.eq_ignore_ascii_case("after"))
        && latest
            .last_price
            .is_some_and(|value| value > rust_decimal::Decimal::ZERO)
}

fn is_us_symbol(value: &str) -> bool {
    value
        .trim()
        .to_ascii_uppercase()
        .split_once('.')
        .is_some_and(|(market, _)| market == "US")
        || value.trim().eq_ignore_ascii_case("US")
}

fn shares_trading_day(incoming: &Tick, latest: &Tick) -> bool {
    let incoming_date = incoming
        .snapshot
        .as_ref()
        .and_then(|snapshot| snapshot.trading_date.as_deref())
        .map(str::to_owned);
    let latest_date = latest
        .snapshot
        .as_ref()
        .and_then(|snapshot| snapshot.trading_date.as_deref())
        .map(str::to_owned);
    if let (Some(left), Some(right)) = (incoming_date, latest_date) {
        return left == right;
    }
    trading_day_key(&incoming.instrument_id, incoming.observed_at_ms)
        == trading_day_key(&latest.instrument_id, latest.observed_at_ms)
}

/// Provider-neutral trading-day key for one instrument and instant.
///
/// Mirrors Go `market.TradingDayKey(symbol, at, true)`: the key is the
/// exchange-local calendar day, with US extended-hours observations after
/// 20:00 local time rolling into the next trading day.
pub fn trading_day_key(instrument_id: &str, observed_at_ms: i64) -> Option<String> {
    let timestamp = Timestamp::from_millisecond(observed_at_ms).ok()?;
    let market = instrument_id
        .split_once('.')
        .map(|(market, _)| market.to_ascii_uppercase())?;
    let timezone = match market.as_str() {
        "US" => "America/New_York",
        "HK" => "Asia/Hong_Kong",
        "CN" | "SH" | "SZ" => "Asia/Shanghai",
        _ => "UTC",
    };
    let zone = jiff::tz::TimeZone::get(timezone).ok()?;
    let local = timestamp.to_zoned(zone);
    let mut date = local.date();
    if market == "US" && local.hour() >= 20 {
        date = date.tomorrow().ok()?;
    }
    Some(date.to_string())
}

#[cfg(test)]
mod tests {
    use rust_decimal::Decimal;

    use super::*;

    #[test]
    // Parity: go:452dea11:internal/app/apiserver/marketdataapp/data_plane_switch_test.go:40 TestApplyProviderSettingsPreservesAtomicQuoteCacheOnFailure
    fn cache_rejects_stale_generation_and_classifies_freshness() {
        let mut cache = TickCache::new(2);
        let tick = Tick {
            instrument_id: "US.AAPL".to_owned(),
            price: Decimal::new(1885, 1),
            volume: "10".parse().expect("volume"),
            volume_delta: None,
            snapshot: None,
            observed_at_ms: 100,
            provider_generation: 2,
        };
        assert_eq!(
            cache.insert(tick.clone(), 1),
            Err(MarketDataError::ProviderChanged)
        );
        cache.insert(tick.clone(), 2).expect("current generation");
        assert!(matches!(
            cache.lookup("us.aapl", 110, 10),
            CacheLookup::Fresh(_)
        ));
        assert!(matches!(
            cache.lookup("US.AAPL", 111, 10),
            CacheLookup::Stale(_)
        ));
        assert!(matches!(
            cache.lookup_for_generation("US.AAPL", 110, 10, 2),
            CacheLookup::Fresh(_)
        ));
        assert_eq!(
            cache.lookup_for_generation("US.AAPL", 110, 10, 1),
            CacheLookup::Missing
        );
        assert_eq!(
            cache.require_fresh_for_generation("US.AAPL", 110, 10, 1),
            Err(MarketDataError::CacheMiss("US.AAPL".to_owned()))
        );
    }
}
