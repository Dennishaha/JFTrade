//! Short-lived margin-ratio cache shared by the production trade runtime.
//!
//! OpenD applies a request rate limit to margin-ratio lookups.  The cache is
//! deliberately process-local and bounded by time: it is only a resilience
//! layer for a recent successful response, never a source of synthetic data.

use std::collections::HashMap;
use std::sync::{Arc, RwLock};
use std::time::{Duration, Instant};

use jftrade_integration_futu::TradeMarginRatioSnapshot;

pub(crate) const MARGIN_RATIO_CACHE_TTL: Duration = Duration::from_secs(30);
pub(crate) const MARGIN_RATIO_CACHE_FALLBACK_TTL: Duration = Duration::from_secs(2 * 60);

#[derive(Clone, Default)]
pub(crate) struct MarginRatioCache {
    entries: Arc<RwLock<HashMap<String, MarginRatioCacheEntry>>>,
}

#[derive(Clone)]
struct MarginRatioCacheEntry {
    snapshots: Vec<TradeMarginRatioSnapshot>,
    updated_at: Instant,
}

impl MarginRatioCache {
    pub(crate) fn get(
        &self,
        key: &str,
        max_age: Duration,
    ) -> Option<Vec<TradeMarginRatioSnapshot>> {
        if key.is_empty() {
            return None;
        }
        let entry = self
            .entries
            .read()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .get(key)
            .cloned()?;
        if Instant::now().saturating_duration_since(entry.updated_at) > max_age {
            return None;
        }
        Some(entry.snapshots)
    }

    pub(crate) fn put(&self, key: String, snapshots: Vec<TradeMarginRatioSnapshot>) {
        self.put_at(key, snapshots, Instant::now());
    }

    pub(crate) fn put_at(
        &self,
        key: String,
        snapshots: Vec<TradeMarginRatioSnapshot>,
        updated_at: Instant,
    ) {
        if key.is_empty() {
            return;
        }
        self.entries
            .write()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .insert(
                key,
                MarginRatioCacheEntry {
                    snapshots,
                    updated_at,
                },
            );
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn cache_returns_cloned_snapshots_only_within_requested_age() {
        let cache = MarginRatioCache::default();
        let mut snapshots = Vec::new();
        cache.put("fresh".to_owned(), snapshots.clone());
        assert_eq!(cache.get("fresh", MARGIN_RATIO_CACHE_TTL), Some(Vec::new()));

        cache.put_at(
            "stale".to_owned(),
            Vec::new(),
            Instant::now() - MARGIN_RATIO_CACHE_FALLBACK_TTL - Duration::from_secs(1),
        );
        assert!(
            cache
                .get("stale", MARGIN_RATIO_CACHE_FALLBACK_TTL)
                .is_none()
        );

        snapshots.push(TradeMarginRatioSnapshot {
            header: jftrade_integration_futu::TradeHeader {
                trd_env: 1,
                acc_id: 42,
                trd_market: 1,
                jp_acc_type: None,
            },
            market: "HK".to_owned(),
            symbol: "HK.00700".to_owned(),
            is_long_permit: None,
            is_short_permit: None,
            short_pool_remain: None,
            short_fee_rate: None,
            alert_long_ratio: None,
            alert_short_ratio: None,
            initial_margin_long_ratio: None,
            initial_margin_short_ratio: None,
            margin_call_long_ratio: None,
            margin_call_short_ratio: None,
            maintenance_long_ratio: None,
            maintenance_short_ratio: None,
        });
        cache.put("clone".to_owned(), snapshots.clone());
        let mut first = cache.get("clone", MARGIN_RATIO_CACHE_TTL).expect("cache");
        first[0].symbol = "MUTATED".to_owned();
        assert_eq!(
            cache.get("clone", MARGIN_RATIO_CACHE_TTL).unwrap()[0].symbol,
            "HK.00700"
        );
    }
}
