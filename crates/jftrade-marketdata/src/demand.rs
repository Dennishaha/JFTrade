use std::collections::{BTreeMap, BTreeSet};

use serde::{Deserialize, Serialize};

use crate::{InstrumentRef, MarketDataError};

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SubscriptionEntrySnapshot {
    pub key: String,
    pub channel: String,
    pub market: String,
    pub symbol: String,
    pub instrument_id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub interval: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub depth_level: Option<i32>,
    pub consumers: Vec<String>,
    pub ref_count: usize,
    pub created_at_ms: i64,
    pub updated_at_ms: i64,
}

#[derive(Clone, Debug, Default, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct DemandSnapshot {
    pub consumer_count: usize,
    pub managed_consumer_count: usize,
    pub logical_count: usize,
    pub active: Vec<InstrumentRef>,
    #[serde(default, skip_serializing)]
    pub entries: Vec<SubscriptionEntrySnapshot>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
struct EntryRecord {
    reference: InstrumentRef,
    created_at_ms: i64,
    updated_at_ms: i64,
    depth_level: Option<i32>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
struct ConsumerDemand {
    managed: bool,
    heartbeat_ms: i64,
    created_at_ms: i64,
    updated_at_ms: i64,
    refs: BTreeSet<InstrumentRef>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct DemandBook {
    consumers: BTreeMap<String, ConsumerDemand>,
    entries: BTreeMap<InstrumentRef, EntryRecord>,
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
            let norm = reference.normalize()?;
            if let Some(record) = self.entries.get_mut(&norm) {
                record.updated_at_ms = now_ms;
            } else {
                self.entries.insert(
                    norm.clone(),
                    EntryRecord {
                        reference: norm.clone(),
                        created_at_ms: now_ms,
                        updated_at_ms: now_ms,
                        depth_level: None,
                    },
                );
            }
            normalized.insert(norm);
        }
        if normalized.is_empty() {
            return Err(MarketDataError::InvalidSubscription(
                "at least one instrument is required".to_owned(),
            ));
        }

        if let Some(consumer) = self.consumers.get_mut(consumer_id) {
            consumer.managed = managed;
            consumer.heartbeat_ms = now_ms;
            consumer.updated_at_ms = now_ms;
            consumer.refs.extend(normalized);
        } else {
            self.consumers.insert(
                consumer_id.to_owned(),
                ConsumerDemand {
                    managed,
                    heartbeat_ms: now_ms,
                    created_at_ms: now_ms,
                    updated_at_ms: now_ms,
                    refs: normalized,
                },
            );
        }
        self.cleanup_orphan_entries();
        Ok(self.snapshot())
    }

    pub fn replace(
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
            let norm = reference.normalize()?;
            normalized.insert(norm);
        }
        if normalized.is_empty() {
            let (_, snapshot) = self.release_consumer(consumer_id);
            return Ok(snapshot);
        }

        for norm in &normalized {
            if let Some(record) = self.entries.get_mut(norm) {
                record.updated_at_ms = now_ms;
            } else {
                self.entries.insert(
                    norm.clone(),
                    EntryRecord {
                        reference: norm.clone(),
                        created_at_ms: now_ms,
                        updated_at_ms: now_ms,
                        depth_level: None,
                    },
                );
            }
        }

        if let Some(consumer) = self.consumers.get_mut(consumer_id) {
            consumer.managed = managed;
            consumer.heartbeat_ms = now_ms;
            consumer.updated_at_ms = now_ms;
            consumer.refs = normalized;
        } else {
            self.consumers.insert(
                consumer_id.to_owned(),
                ConsumerDemand {
                    managed,
                    heartbeat_ms: now_ms,
                    created_at_ms: now_ms,
                    updated_at_ms: now_ms,
                    refs: normalized,
                },
            );
        }
        self.cleanup_orphan_entries();
        Ok(self.snapshot())
    }

    pub fn heartbeat(&mut self, consumer_id: &str, now_ms: i64) -> (bool, DemandSnapshot) {
        let updated = match self.consumers.get_mut(consumer_id.trim()) {
            Some(consumer) => {
                consumer.heartbeat_ms = now_ms;
                consumer.updated_at_ms = now_ms;
                for r in &consumer.refs {
                    if let Some(entry) = self.entries.get_mut(r) {
                        entry.updated_at_ms = now_ms;
                    }
                }
                true
            }
            None => false,
        };
        (updated, self.snapshot())
    }

    pub fn release_instrument(
        &mut self,
        consumer_id: &str,
        target: &InstrumentRef,
        now_ms: i64,
    ) -> (bool, DemandSnapshot) {
        let consumer_id = consumer_id.trim();
        let target_norm = match target.clone().normalize() {
            Ok(n) => n,
            Err(_) => return (false, self.snapshot()),
        };

        let mut removed = false;
        if let Some(consumer) = self.consumers.get_mut(consumer_id) {
            removed = consumer.refs.remove(&target_norm);
            if removed {
                consumer.updated_at_ms = now_ms;
            }
            if consumer.refs.is_empty() {
                self.consumers.remove(consumer_id);
            }
        }
        if removed {
            if let Some(entry) = self.entries.get_mut(&target_norm) {
                entry.updated_at_ms = now_ms;
            }
            self.cleanup_orphan_entries();
        }
        (removed, self.snapshot())
    }

    pub fn release_consumer_with_time(
        &mut self,
        consumer_id: &str,
        now_ms: i64,
    ) -> (bool, DemandSnapshot) {
        let removed_consumer = self.consumers.remove(consumer_id.trim());
        let removed = if let Some(consumer) = removed_consumer {
            for r in &consumer.refs {
                if let Some(entry) = self.entries.get_mut(r) {
                    entry.updated_at_ms = now_ms;
                }
            }
            self.cleanup_orphan_entries();
            true
        } else {
            false
        };
        (removed, self.snapshot())
    }

    pub fn release_consumer(&mut self, consumer_id: &str) -> (bool, DemandSnapshot) {
        let now_ms = self
            .consumers
            .get(consumer_id.trim())
            .map(|c| c.updated_at_ms)
            .unwrap_or(0);
        self.release_consumer_with_time(consumer_id, now_ms)
    }

    pub fn release(&mut self, consumer_id: &str) -> bool {
        self.release_consumer(consumer_id).0
    }

    pub fn clear(&mut self, consumer_id: Option<&str>, now_ms: i64) -> DemandSnapshot {
        if let Some(consumer_id) = consumer_id.map(str::trim).filter(|s| !s.is_empty()) {
            if let Some(consumer) = self.consumers.remove(consumer_id) {
                for r in &consumer.refs {
                    if let Some(entry) = self.entries.get_mut(r) {
                        entry.updated_at_ms = now_ms;
                    }
                }
            }
        } else {
            let mut cleared_refs = Vec::new();
            self.consumers.retain(|_, consumer| {
                if !consumer.managed {
                    cleared_refs.extend(consumer.refs.iter().cloned());
                    false
                } else {
                    true
                }
            });
            for r in &cleared_refs {
                if let Some(entry) = self.entries.get_mut(r) {
                    entry.updated_at_ms = now_ms;
                }
            }
        }
        self.cleanup_orphan_entries();
        self.snapshot()
    }

    pub fn expire(&mut self, now_ms: i64, ttl_ms: i64) -> Vec<String> {
        if ttl_ms < 0 {
            return Vec::new();
        }
        let mut expired = Vec::new();
        let mut expired_refs = Vec::new();
        self.consumers.retain(|consumer_id, demand| {
            let keep = demand.managed || now_ms.saturating_sub(demand.heartbeat_ms) <= ttl_ms;
            if !keep {
                expired.push(consumer_id.clone());
                expired_refs.extend(demand.refs.iter().cloned());
            }
            keep
        });
        for r in &expired_refs {
            if let Some(entry) = self.entries.get_mut(r) {
                entry.updated_at_ms = now_ms;
            }
        }
        if !expired.is_empty() {
            self.cleanup_orphan_entries();
        }
        expired
    }

    pub fn has_managed_consumers(&self) -> bool {
        self.consumers.values().any(|consumer| consumer.managed)
    }

    fn cleanup_orphan_entries(&mut self) {
        let mut active_refs = BTreeSet::new();
        for consumer in self.consumers.values() {
            active_refs.extend(consumer.refs.iter().cloned());
        }
        self.entries.retain(|r, _| active_refs.contains(r));
    }

    pub fn snapshot(&self) -> DemandSnapshot {
        let mut entries = Vec::new();
        for (reference, record) in &self.entries {
            let mut consumers = Vec::new();
            for (consumer_id, consumer) in &self.consumers {
                if consumer.refs.contains(reference) {
                    consumers.push(consumer_id.clone());
                }
            }
            consumers.sort();
            let ref_count = consumers.len();
            entries.push(SubscriptionEntrySnapshot {
                key: reference.key(),
                channel: reference.channel.clone(),
                market: reference.market.clone(),
                symbol: reference.symbol.clone(),
                instrument_id: reference.instrument_id(),
                interval: reference.interval.clone(),
                depth_level: record.depth_level,
                consumers,
                ref_count,
                created_at_ms: record.created_at_ms,
                updated_at_ms: record.updated_at_ms,
            });
        }
        entries.sort_by(|a, b| a.key.cmp(&b.key));

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
            logical_count: entries.len(),
            active,
            entries,
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
        assert_eq!(snapshot.entries.len(), 1);
        assert_eq!(snapshot.entries[0].key, "KLINE:US:AAPL:1m");
        assert_eq!(snapshot.entries[0].consumers, vec!["strategy"]);
    }

    #[test]
    fn partial_release_and_clear_operations() {
        let mut book = DemandBook::default();
        book.acquire(
            "chart",
            [
                reference("SNAPSHOT", "US.AAPL", None),
                reference("SNAPSHOT", "HK.00700", None),
            ],
            false,
            10,
        )
        .expect("acquire two");
        book.acquire(
            "strategy",
            [reference("SNAPSHOT", "US.AAPL", None)],
            true,
            15,
        )
        .expect("acquire managed");
        assert_eq!(book.snapshot().logical_count, 2);

        // Partial release: chart releases US.AAPL, but strategy still holds it.
        // entry updated_at_ms should be updated to 20!
        let (released, snap) =
            book.release_instrument("chart", &reference("SNAPSHOT", "US.AAPL", None), 20);
        assert!(released);
        assert_eq!(snap.logical_count, 2);
        let aapl_entry = snap
            .entries
            .iter()
            .find(|e| e.key == "SNAPSHOT:US:AAPL")
            .unwrap();
        assert_eq!(aapl_entry.updated_at_ms, 20);
        assert_eq!(aapl_entry.consumers, vec!["strategy"]);

        // clear(None) should only clear unmanaged consumers (chart), keeping managed (strategy)
        let snap_clear = book.clear(None, 30);
        assert_eq!(snap_clear.logical_count, 1);
        assert_eq!(snap_clear.consumer_count, 1);
        assert_eq!(snap_clear.managed_consumer_count, 1);
        assert_eq!(snap_clear.entries[0].key, "SNAPSHOT:US:AAPL");

        // explicit clear of managed consumer
        let snap_clear_managed = book.clear(Some("strategy"), 40);
        assert_eq!(snap_clear_managed.logical_count, 0);
        assert_eq!(snap_clear_managed.consumer_count, 0);
    }

    #[test]
    fn heartbeat_updates_consumer_and_entry_timestamps() {
        let mut book = DemandBook::default();
        book.acquire("chart", [reference("SNAPSHOT", "US.AAPL", None)], false, 10)
            .expect("acquire");
        assert_eq!(book.snapshot().entries[0].updated_at_ms, 10);

        let (updated, snap) = book.heartbeat("chart", 50);
        assert!(updated);
        assert_eq!(snap.entries[0].updated_at_ms, 50);
    }

    #[test]
    fn channel_and_interval_validation_rules() {
        // Supported 3m interval for KLINE
        let kline_3m = reference("KLINE", "US.AAPL", Some("3m"))
            .normalize()
            .unwrap();
        assert_eq!(kline_3m.interval.as_deref(), Some("3m"));

        // Reject KLINE tick, 60m, 1y (Go canonical allows 1h, 1d, 1w, 1mo)
        assert!(
            reference("KLINE", "US.AAPL", Some("tick"))
                .normalize()
                .is_err()
        );
        assert!(
            reference("KLINE", "US.AAPL", Some("60m"))
                .normalize()
                .is_err()
        );
        assert!(
            reference("KLINE", "US.AAPL", Some("1y"))
                .normalize()
                .is_err()
        );
        assert!(
            reference("KLINE", "US.AAPL", Some("1h"))
                .normalize()
                .is_ok()
        );

        // Reject interval on non-KLINE
        let snapshot_with_interval = reference("SNAPSHOT", "US.AAPL", Some("1m")).normalize();
        assert!(snapshot_with_interval.is_err());

        let tick_with_interval = reference("TICK", "US.AAPL", Some("1m")).normalize();
        assert!(tick_with_interval.is_err());

        let order_book_with_interval = reference("ORDER_BOOK", "HK.00700", Some("1m")).normalize();
        assert!(order_book_with_interval.is_err());
    }
}
