use std::collections::{BTreeMap, BTreeSet};

use serde::{Deserialize, Serialize};

use crate::{InstrumentRef, MarketDataError};

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DemandSnapshot {
    pub consumer_count: usize,
    pub managed_consumer_count: usize,
    pub logical_count: usize,
    pub active: Vec<InstrumentRef>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
struct ConsumerDemand {
    managed: bool,
    heartbeat_ms: i64,
    refs: BTreeSet<InstrumentRef>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct DemandBook {
    consumers: BTreeMap<String, ConsumerDemand>,
}

impl DemandBook {
    pub fn acquire(
        &mut self,
        consumer_id: &str,
        refs: impl IntoIterator<Item = InstrumentRef>,
        managed: bool,
        now_ms: i64,
    ) -> Result<DemandSnapshot, MarketDataError> {
        let consumer_id = consumer_id.trim();
        if consumer_id.is_empty() {
            return Err(MarketDataError::MissingConsumer);
        }
        let mut normalized = BTreeSet::new();
        for reference in refs {
            normalized.insert(reference.normalize()?);
        }
        if normalized.is_empty() {
            return Err(MarketDataError::InvalidSubscription(
                "at least one instrument is required".to_owned(),
            ));
        }
        self.consumers.insert(
            consumer_id.to_owned(),
            ConsumerDemand {
                managed,
                heartbeat_ms: now_ms,
                refs: normalized,
            },
        );
        Ok(self.snapshot())
    }

    pub fn heartbeat(&mut self, consumer_id: &str, now_ms: i64) -> bool {
        match self.consumers.get_mut(consumer_id.trim()) {
            Some(consumer) if !consumer.managed => {
                consumer.heartbeat_ms = now_ms;
                true
            }
            _ => false,
        }
    }

    pub fn release(&mut self, consumer_id: &str) -> bool {
        self.consumers.remove(consumer_id.trim()).is_some()
    }

    pub fn expire(&mut self, now_ms: i64, ttl_ms: i64) -> Vec<String> {
        if ttl_ms < 0 {
            return Vec::new();
        }
        let mut expired = Vec::new();
        self.consumers.retain(|consumer_id, demand| {
            let keep = demand.managed || now_ms.saturating_sub(demand.heartbeat_ms) <= ttl_ms;
            if !keep {
                expired.push(consumer_id.clone());
            }
            keep
        });
        expired
    }

    pub fn has_managed_consumers(&self) -> bool {
        self.consumers.values().any(|consumer| consumer.managed)
    }

    pub fn snapshot(&self) -> DemandSnapshot {
        let active = self
            .consumers
            .values()
            .flat_map(|consumer| consumer.refs.iter().cloned())
            .collect::<BTreeSet<_>>()
            .into_iter()
            .collect::<Vec<_>>();
        DemandSnapshot {
            consumer_count: self.consumers.len(),
            managed_consumer_count: self
                .consumers
                .values()
                .filter(|consumer| consumer.managed)
                .count(),
            logical_count: active.len(),
            active,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn reference(channel: &str, symbol: &str, interval: Option<&str>) -> InstrumentRef {
        InstrumentRef {
            channel: channel.to_owned(),
            market: String::new(),
            symbol: symbol.to_owned(),
            interval: interval.map(str::to_owned),
        }
    }

    #[test]
    fn demand_is_deduplicated_and_managed_leases_do_not_expire() {
        let mut book = DemandBook::default();
        book.acquire("chart", [reference("snapshot", "us.aapl", None)], false, 10)
            .expect("chart demand");
        book.acquire(
            "strategy",
            [reference("KLINE", "US.AAPL", Some("1M"))],
            true,
            10,
        )
        .expect("managed demand");

        assert_eq!(book.expire(31, 20), vec!["chart"]);
        let snapshot = book.snapshot();
        assert_eq!(snapshot.managed_consumer_count, 1);
        assert_eq!(snapshot.active[0].interval.as_deref(), Some("1m"));
    }
}
