use std::collections::{BTreeMap, BTreeSet};

use jftrade_marketdata::InstrumentRef;
use serde::{Deserialize, Serialize};

#[derive(Clone, Debug, Deserialize, Eq, Ord, PartialEq, PartialOrd, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum SubscriptionKind {
    Basic,
    Kline,
    OrderBook,
}

#[derive(Clone, Debug, Deserialize, Eq, Ord, PartialEq, PartialOrd, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PhysicalSubscription {
    pub key: String,
    pub kind: SubscriptionKind,
    pub instrument_id: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub interval: Option<String>,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SubscriptionPlan {
    pub logical_count: usize,
    pub physical: Vec<PhysicalSubscription>,
}

pub fn desired_subscriptions(desired: &[InstrumentRef]) -> SubscriptionPlan {
    let mut logical = BTreeSet::new();
    let mut physical = BTreeSet::new();
    for raw in desired {
        let Ok(reference) = raw.clone().normalize() else {
            continue;
        };
        let instrument_id = reference.instrument_id();
        let logical_key = match &reference.interval {
            Some(interval) => format!("{}:{instrument_id}:{interval}", reference.channel),
            None => format!("{}:{instrument_id}", reference.channel),
        };
        logical.insert(logical_key);
        if reference.channel == "ORDER_BOOK" {
            physical.insert(PhysicalSubscription {
                key: format!("ORDER_BOOK:{instrument_id}"),
                kind: SubscriptionKind::OrderBook,
                instrument_id,
                interval: None,
            });
            continue;
        }
        physical.insert(PhysicalSubscription {
            key: format!("BASIC:{instrument_id}"),
            kind: SubscriptionKind::Basic,
            instrument_id: instrument_id.clone(),
            interval: None,
        });
        if reference.channel == "KLINE"
            && let Some(interval) = reference.interval
        {
            physical.insert(PhysicalSubscription {
                key: format!("KLINE:{instrument_id}:{interval}"),
                kind: SubscriptionKind::Kline,
                instrument_id,
                interval: Some(interval),
            });
        }
    }
    SubscriptionPlan {
        logical_count: logical.len(),
        physical: physical.into_iter().collect(),
    }
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", tag = "action")]
pub enum ReconcileAction {
    Subscribe { subscription: PhysicalSubscription },
    Unsubscribe { subscription: PhysicalSubscription },
}

#[derive(Clone, Debug)]
struct ActiveRecord {
    subscription: PhysicalSubscription,
    subscribed_at_ms: i64,
    generation: u64,
    failures: usize,
    retry_at_ms: i64,
}

#[derive(Clone, Debug)]
pub struct SubscriptionReconciler {
    records: BTreeMap<String, ActiveRecord>,
    minimum_age_ms: i64,
}

impl SubscriptionReconciler {
    pub fn new(minimum_age_ms: i64) -> Self {
        Self {
            records: BTreeMap::new(),
            minimum_age_ms: minimum_age_ms.max(0),
        }
    }

    pub fn actions(
        &self,
        desired: &[InstrumentRef],
        now_ms: i64,
        generation: u64,
    ) -> Vec<ReconcileAction> {
        let plan = desired_subscriptions(desired);
        let desired_by_key = plan
            .physical
            .into_iter()
            .map(|subscription| (subscription.key.clone(), subscription))
            .collect::<BTreeMap<_, _>>();
        let mut actions = Vec::new();
        for (key, subscription) in &desired_by_key {
            match self.records.get(key) {
                Some(record) if record.generation == generation && now_ms >= record.retry_at_ms => {
                }
                _ => actions.push(ReconcileAction::Subscribe {
                    subscription: subscription.clone(),
                }),
            }
        }
        for (key, record) in &self.records {
            if !desired_by_key.contains_key(key)
                && now_ms.saturating_sub(record.subscribed_at_ms) >= self.minimum_age_ms
            {
                actions.push(ReconcileAction::Unsubscribe {
                    subscription: record.subscription.clone(),
                });
            }
        }
        actions
    }

    pub fn record_success(&mut self, action: &ReconcileAction, now_ms: i64, generation: u64) {
        match action {
            ReconcileAction::Subscribe { subscription } => {
                self.records.insert(
                    subscription.key.clone(),
                    ActiveRecord {
                        subscription: subscription.clone(),
                        subscribed_at_ms: now_ms,
                        generation,
                        failures: 0,
                        retry_at_ms: 0,
                    },
                );
            }
            ReconcileAction::Unsubscribe { subscription } => {
                self.records.remove(&subscription.key);
            }
        }
    }

    pub fn record_failure(&mut self, subscription: &PhysicalSubscription, now_ms: i64) -> i64 {
        let record = self
            .records
            .entry(subscription.key.clone())
            .or_insert_with(|| ActiveRecord {
                subscription: subscription.clone(),
                subscribed_at_ms: 0,
                generation: 0,
                failures: 0,
                retry_at_ms: 0,
            });
        let delay = retry_delay_ms(record.failures);
        record.failures = record.failures.saturating_add(1);
        record.retry_at_ms = now_ms.saturating_add(delay);
        delay
    }
}

pub fn retry_delay_ms(failures: usize) -> i64 {
    const DELAYS: [i64; 4] = [5_000, 10_000, 20_000, 30_000];
    DELAYS[failures.min(DELAYS.len() - 1)]
}

#[cfg(test)]
mod tests {
    use super::*;

    fn reference(channel: &str, interval: Option<&str>) -> InstrumentRef {
        InstrumentRef {
            channel: channel.to_owned(),
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            interval: interval.map(str::to_owned),
        }
    }

    #[test]
    fn kline_adds_basic_and_minimum_age_delays_unsubscribe() {
        let desired = [reference("KLINE", Some("1m"))];
        let plan = desired_subscriptions(&desired);
        assert_eq!(plan.logical_count, 1);
        assert_eq!(plan.physical.len(), 2);

        let mut reconciler = SubscriptionReconciler::new(60_000);
        let actions = reconciler.actions(&desired, 0, 1);
        for action in &actions {
            reconciler.record_success(action, 0, 1);
        }
        assert!(reconciler.actions(&[], 59_999, 1).is_empty());
        assert_eq!(reconciler.actions(&[], 60_000, 1).len(), 2);
        assert_eq!(reconciler.actions(&desired, 60_000, 2).len(), 2);
    }
}
