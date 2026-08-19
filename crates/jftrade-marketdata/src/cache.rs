use std::collections::{BTreeMap, VecDeque};

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
        if max_age_ms >= 0 && now_ms.saturating_sub(tick.observed_at_ms) <= max_age_ms {
            CacheLookup::Fresh(tick)
        } else {
            CacheLookup::Stale(tick)
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

    pub fn clear(&mut self) {
        self.ticks.clear();
    }

    pub fn instrument_count(&self) -> usize {
        self.ticks.len()
    }
}

#[cfg(test)]
mod tests {
    use jftrade_kernel::Fixed8;

    use super::*;

    #[test]
    fn cache_rejects_stale_generation_and_classifies_freshness() {
        let mut cache = TickCache::new(2);
        let tick = Tick {
            instrument_id: "US.AAPL".to_owned(),
            price: Fixed8::from_scaled(18_850_000_000),
            volume: 10,
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
    }
}
