use std::collections::{BTreeMap, BTreeSet};
use std::sync::Arc;

use jftrade_kernel::WireTimestamp;
use jftrade_marketdata::{InstrumentRef, MarketDataRuntimeRecorder};
use serde::{Deserialize, Serialize};

use crate::subscription_executor::{OpenDSubscriptionExecutor, SubscriptionExecutorError};
use crate::{
    Frame, OpenDSessionCloseReason, OpenDSessionEvent, QuotePush, QuotePushDecodeError,
    decode_quote_push,
};

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
    active: bool,
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
                Some(record) if record.generation == generation && record.active => {}
                Some(record)
                    if record.generation == generation
                        && record.failures > 0
                        && now_ms < record.retry_at_ms => {}
                _ => actions.push(ReconcileAction::Subscribe {
                    subscription: subscription.clone(),
                }),
            }
        }
        for (key, record) in &self.records {
            if record.active
                && !desired_by_key.contains_key(key)
                && now_ms.saturating_sub(record.subscribed_at_ms) >= self.minimum_age_ms
                && (record.failures == 0 || now_ms >= record.retry_at_ms)
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
                        active: true,
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

    pub fn record_failure(
        &mut self,
        subscription: &PhysicalSubscription,
        now_ms: i64,
        generation: u64,
    ) -> i64 {
        let record = self
            .records
            .entry(subscription.key.clone())
            .or_insert_with(|| ActiveRecord {
                subscription: subscription.clone(),
                subscribed_at_ms: 0,
                generation,
                active: false,
                failures: 0,
                retry_at_ms: 0,
            });
        record.generation = generation;
        record.active = false;
        let delay = retry_delay_ms(record.failures);
        record.failures = record.failures.saturating_add(1);
        record.retry_at_ms = now_ms.saturating_add(delay);
        delay
    }

    fn record_unsubscribe_failure(
        &mut self,
        subscription: &PhysicalSubscription,
        now_ms: i64,
        generation: u64,
    ) -> i64 {
        let Some(record) = self.records.get_mut(&subscription.key) else {
            return retry_delay_ms(0);
        };
        if record.generation != generation {
            return retry_delay_ms(0);
        }
        let delay = retry_delay_ms(record.failures);
        record.failures = record.failures.saturating_add(1);
        record.retry_at_ms = now_ms.saturating_add(delay);
        delay
    }

    pub fn replay_actions(
        &mut self,
        desired: &[InstrumentRef],
        generation: u64,
    ) -> Vec<ReconcileAction> {
        let plan = desired_subscriptions(desired);
        let desired_by_key = plan
            .physical
            .into_iter()
            .map(|subscription| (subscription.key.clone(), subscription))
            .collect::<BTreeMap<_, _>>();
        self.records
            .retain(|key, _| desired_by_key.contains_key(key));
        for record in self.records.values_mut() {
            record.generation = generation;
            record.active = false;
            record.failures = 0;
            record.retry_at_ms = 0;
        }
        desired_by_key
            .values()
            .cloned()
            .map(|subscription| ReconcileAction::Subscribe { subscription })
            .collect()
    }

    pub fn active_instruments(&self, kind: SubscriptionKind, generation: u64) -> Vec<String> {
        self.records
            .values()
            .filter(|record| {
                record.generation == generation && record.active && record.subscription.kind == kind
            })
            .map(|record| record.subscription.instrument_id.clone())
            .collect()
    }
}

/// Explicit OpenD subscription lifecycle seam.
///
/// This coordinator owns no socket and performs no external I/O. The future
/// product composition supplies the physical subscribe/unsubscribe executor;
/// this type keeps demand, generation fencing, retry timing and recorder
/// state in one owner so stale callbacks cannot mutate a newer connection.
#[derive(Debug)]
pub struct OpenDSubscriptionLifecycle {
    reconciler: SubscriptionReconciler,
    recorder: Arc<MarketDataRuntimeRecorder>,
    desired: Vec<InstrumentRef>,
    generation: u64,
    closed: bool,
}

impl OpenDSubscriptionLifecycle {
    pub fn new(recorder: Arc<MarketDataRuntimeRecorder>, minimum_subscription_age_ms: i64) -> Self {
        Self {
            reconciler: SubscriptionReconciler::new(minimum_subscription_age_ms),
            recorder,
            desired: Vec::new(),
            generation: 0,
            closed: false,
        }
    }

    pub fn reconcile_demand(
        &mut self,
        desired: &[InstrumentRef],
        now_ms: i64,
    ) -> Vec<ReconcileAction> {
        if self.closed {
            return Vec::new();
        }
        self.desired = desired.to_vec();
        self.generation = self.recorder.reconcile(runtime_instruments(&self.desired));
        self.reconciler
            .actions(&self.desired, now_ms, self.generation)
    }

    pub fn generation(&self) -> u64 {
        self.generation
    }

    pub fn recorder(&self) -> Arc<MarketDataRuntimeRecorder> {
        Arc::clone(&self.recorder)
    }

    pub fn active_basic_instruments(&self) -> Vec<String> {
        if self.closed {
            return Vec::new();
        }
        self.reconciler
            .active_instruments(SubscriptionKind::Basic, self.generation)
    }

    pub fn reconfigure(&mut self) -> u64 {
        if self.closed {
            return self.generation;
        }
        self.generation = self.recorder.reconfigure();
        self.generation
    }

    pub fn reconfigure_for_reconnect(&mut self, desired: &[InstrumentRef]) -> Vec<ReconcileAction> {
        if self.closed {
            return Vec::new();
        }
        self.desired = desired.to_vec();
        let previous = self.generation;
        let reconciled = self.recorder.reconcile(runtime_instruments(&self.desired));
        self.generation = if reconciled == previous {
            self.recorder.reconfigure()
        } else {
            reconciled
        };
        self.reconciler
            .replay_actions(&self.desired, self.generation)
    }

    pub fn record_subscription_success(
        &mut self,
        action: &ReconcileAction,
        now_ms: i64,
        generation: u64,
    ) -> bool {
        if self.closed || generation != self.generation {
            return false;
        }
        self.reconciler.record_success(action, now_ms, generation);
        true
    }

    pub fn record_subscription_failure(
        &mut self,
        subscription: &PhysicalSubscription,
        now_ms: i64,
        generation: u64,
    ) -> Option<i64> {
        if self.closed || generation != self.generation {
            return None;
        }
        Some(
            self.reconciler
                .record_failure(subscription, now_ms, generation),
        )
    }

    /// Applies one planned action through the protocol executor and commits
    /// the result only for the active generation. A failed subscribe remains
    /// fenced by the reconciler's bounded retry window; stale or closed work
    /// is ignored without touching OpenD.
    pub fn execute_action(
        &mut self,
        action: &ReconcileAction,
        now_ms: i64,
        generation: u64,
        executor: &mut OpenDSubscriptionExecutor,
    ) -> Result<bool, SubscriptionExecutorError> {
        if self.closed
            || generation != self.generation
            || executor.session().managed_session().generation() != generation
        {
            return Ok(false);
        }
        match executor.execute(action) {
            Ok(()) => {
                self.reconciler.record_success(action, now_ms, generation);
                Ok(true)
            }
            Err(error) => {
                match action {
                    ReconcileAction::Subscribe { subscription } => {
                        self.reconciler
                            .record_failure(subscription, now_ms, generation);
                    }
                    ReconcileAction::Unsubscribe { subscription } => {
                        self.reconciler.record_unsubscribe_failure(
                            subscription,
                            now_ms,
                            generation,
                        );
                    }
                }
                Err(error)
            }
        }
    }

    /// Consume one generation-tagged event from the managed session's single
    /// reader. Active pushes reuse the existing decoder; an active non-local
    /// close marks the stream failed so reconnect orchestration can advance
    /// generations. Local and stale closes have no retry side effect.
    pub fn ingest_session_event(
        &self,
        event: &OpenDSessionEvent,
        now: WireTimestamp,
    ) -> Result<Option<QuotePush>, QuotePushDecodeError> {
        match event {
            OpenDSessionEvent::UnsolicitedFrame { generation, frame } => {
                self.ingest_quote_push(frame, now, *generation)
            }
            OpenDSessionEvent::Closed { generation, reason } => {
                if *reason != OpenDSessionCloseReason::Local && self.accepts_generation(*generation)
                {
                    let _ =
                        self.recorder
                            .record_stream_failure(*generation, now, reason.to_string());
                }
                Ok(None)
            }
        }
    }

    /// Accept one unsolicited Qot_Update frame for the active connection.
    ///
    /// The Go stream handler drops unknown, rejected, empty, and malformed
    /// pushes. Keep the same fail-closed behavior here: malformed external
    /// data is not allowed to poison the session lifecycle or trigger a
    /// reconnect. A stale or closed generation is rejected before decode so
    /// old callbacks cannot mutate runtime state.
    pub fn ingest_quote_push(
        &self,
        frame: &Frame,
        _now: WireTimestamp,
        generation: u64,
    ) -> Result<Option<QuotePush>, QuotePushDecodeError> {
        if !self.accepts_generation(generation) {
            return Ok(None);
        }
        match decode_quote_push(frame) {
            Ok(Some(push)) => Ok(Some(push)),
            Ok(None) => Ok(None),
            Err(_error) => Ok(None),
        }
    }

    pub fn poll_started(&self, now: WireTimestamp, generation: u64) -> bool {
        self.accepts_generation(generation) && self.recorder.record_poll_started(generation, now)
    }

    pub fn quote_success(&self, generation: u64) -> bool {
        self.accepts_generation(generation) && self.recorder.record_quote_success(generation)
    }

    pub fn quote_failure(
        &self,
        now: WireTimestamp,
        error: impl Into<String>,
        generation: u64,
    ) -> bool {
        self.accepts_generation(generation)
            && self.recorder.record_quote_failure(generation, now, error)
    }

    pub fn stream_connected(&self, generation: u64) -> bool {
        self.accepts_generation(generation) && self.recorder.record_stream_connected(generation)
    }

    pub fn stream_failure(
        &self,
        now: WireTimestamp,
        error: impl Into<String>,
        generation: u64,
    ) -> bool {
        self.accepts_generation(generation)
            && self.recorder.record_stream_failure(generation, now, error)
    }

    pub fn close(&mut self) -> bool {
        if self.closed {
            return false;
        }
        self.closed = true;
        self.recorder.close();
        true
    }

    pub(crate) fn accepts_session_generation(&self, generation: u64) -> bool {
        self.accepts_generation(generation)
    }

    fn accepts_generation(&self, generation: u64) -> bool {
        !self.closed && generation == self.generation
    }
}

fn runtime_instruments(desired: &[InstrumentRef]) -> Vec<String> {
    desired
        .iter()
        .filter_map(|reference| reference.clone().normalize().ok())
        .map(|reference| reference.instrument_id())
        .collect()
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
        assert!(reconciler.actions(&desired, 1, 1).is_empty());
        assert!(reconciler.actions(&[], 59_999, 1).is_empty());
        assert_eq!(reconciler.actions(&[], 60_000, 1).len(), 2);
        assert_eq!(reconciler.actions(&desired, 60_000, 2).len(), 2);
    }

    #[test]
    fn subscription_failure_retry_is_fenced_to_its_generation() {
        let desired = [reference("SNAPSHOT", None)];
        let mut reconciler = SubscriptionReconciler::new(0);
        let actions = reconciler.actions(&desired, 0, 1);
        let subscription = match &actions[0] {
            ReconcileAction::Subscribe { subscription } => subscription,
            _ => panic!("expected subscribe action"),
        };
        assert_eq!(reconciler.record_failure(subscription, 0, 1), 5_000);
        assert!(reconciler.actions(&desired, 4_999, 1).is_empty());
        assert_eq!(reconciler.actions(&desired, 5_000, 1).len(), 1);
        assert_eq!(reconciler.actions(&desired, 0, 2).len(), 1);
    }

    #[test]
    fn failed_or_replayed_subscriptions_are_not_active_until_success() {
        let desired = [reference("SNAPSHOT", None)];
        let mut reconciler = SubscriptionReconciler::new(0);
        let actions = reconciler.actions(&desired, 0, 1);
        let subscription = match &actions[0] {
            ReconcileAction::Subscribe { subscription } => subscription,
            _ => panic!("expected subscribe action"),
        };

        assert!(
            reconciler
                .active_instruments(SubscriptionKind::Basic, 1)
                .is_empty()
        );
        assert_eq!(reconciler.record_failure(subscription, 0, 1), 5_000);
        assert!(
            reconciler
                .active_instruments(SubscriptionKind::Basic, 1)
                .is_empty()
        );
        assert!(reconciler.actions(&[], 60_000, 1).is_empty());
        assert!(reconciler.actions(&desired, 4_999, 1).is_empty());
        assert_eq!(reconciler.actions(&desired, 5_000, 1).len(), 1);

        reconciler.record_success(&actions[0], 5_000, 1);
        assert_eq!(
            reconciler.active_instruments(SubscriptionKind::Basic, 1),
            vec!["US.AAPL".to_owned()]
        );

        let replay = reconciler.replay_actions(&desired, 2);
        assert_eq!(replay.len(), 1);
        assert!(
            reconciler
                .active_instruments(SubscriptionKind::Basic, 2)
                .is_empty()
        );
        reconciler.record_success(&replay[0], 6_000, 2);
        assert_eq!(
            reconciler.active_instruments(SubscriptionKind::Basic, 2),
            vec!["US.AAPL".to_owned()]
        );
    }

    #[test]
    fn failed_unsubscribe_is_deferred_until_its_retry_window() {
        let desired = [reference("SNAPSHOT", None)];
        let mut reconciler = SubscriptionReconciler::new(0);
        let subscribe = reconciler.actions(&desired, 0, 1).pop().expect("subscribe");
        reconciler.record_success(&subscribe, 0, 1);
        let unsubscribe = reconciler.actions(&[], 0, 1).pop().expect("unsubscribe");
        let subscription = match &unsubscribe {
            ReconcileAction::Unsubscribe { subscription } => subscription,
            _ => panic!("expected unsubscribe action"),
        };
        assert_eq!(
            reconciler.record_unsubscribe_failure(subscription, 0, 1),
            5_000
        );
        assert!(reconciler.actions(&[], 4_999, 1).is_empty());
        assert!(matches!(
            reconciler.actions(&[], 5_000, 1).as_slice(),
            [ReconcileAction::Unsubscribe { .. }]
        ));
    }

    #[test]
    fn managed_session_close_updates_only_the_active_generation() {
        let recorder = Arc::new(MarketDataRuntimeRecorder::default());
        let mut lifecycle = OpenDSubscriptionLifecycle::new(Arc::clone(&recorder), 60_000);
        lifecycle.reconcile_demand(&[reference("SNAPSHOT", None)], 0);
        let generation = lifecycle.generation();
        let stale = OpenDSessionEvent::Closed {
            generation: generation + 1,
            reason: crate::OpenDSessionCloseReason::PeerClosed,
        };
        assert!(
            lifecycle
                .ingest_session_event(&stale, "2026-08-24T00:00:00Z".parse().expect("timestamp"),)
                .expect("stale close")
                .is_none()
        );
        assert_eq!(recorder.snapshot().stream_failures, 0);

        let local = OpenDSessionEvent::Closed {
            generation,
            reason: OpenDSessionCloseReason::Local,
        };
        assert!(
            lifecycle
                .ingest_session_event(&local, "2026-08-24T00:00:01Z".parse().expect("timestamp"),)
                .expect("local close")
                .is_none()
        );
        assert_eq!(recorder.snapshot().stream_failures, 0);

        let active = OpenDSessionEvent::Closed {
            generation,
            reason: crate::OpenDSessionCloseReason::PeerClosed,
        };
        assert!(
            lifecycle
                .ingest_session_event(&active, "2026-08-24T00:00:01Z".parse().expect("timestamp"),)
                .expect("active close")
                .is_none()
        );
        let snapshot = recorder.snapshot();
        assert_eq!(snapshot.stream_failures, 1);
        assert_eq!(
            snapshot.stream_last_error.as_deref(),
            Some("OpenD peer closed the TCP session")
        );
    }

    #[test]
    fn lifecycle_rejects_stale_callbacks_and_closes_recorder_once() {
        let recorder = Arc::new(MarketDataRuntimeRecorder::default());
        let mut lifecycle = OpenDSubscriptionLifecycle::new(Arc::clone(&recorder), 60_000);
        let desired = [reference("KLINE", Some("1m"))];
        let actions = lifecycle.reconcile_demand(&desired, 0);
        let generation = lifecycle.generation();
        assert_eq!(actions.len(), 2);
        assert!(lifecycle.poll_started(
            "2026-08-24T00:00:00Z".parse().expect("timestamp"),
            generation
        ));
        assert!(lifecycle.stream_connected(generation));
        assert!(lifecycle.quote_failure(
            "2026-08-24T00:00:00Z".parse().expect("timestamp"),
            "quote timeout",
            generation,
        ));
        assert!(lifecycle.quote_success(generation));
        assert!(lifecycle.record_subscription_success(&actions[0], 0, generation));
        assert_eq!(
            lifecycle.record_subscription_failure(
                match &actions[1] {
                    ReconcileAction::Subscribe { subscription } => subscription,
                    _ => panic!("expected subscribe action"),
                },
                0,
                generation,
            ),
            Some(5_000)
        );

        lifecycle.reconfigure();
        let next = lifecycle.reconcile_demand(&[reference("SNAPSHOT", None)], 1);
        let next_generation = lifecycle.generation();
        assert_ne!(next_generation, generation);
        assert!(!lifecycle.stream_failure(
            "2026-08-24T00:00:00Z".parse().expect("timestamp"),
            "stale stream",
            generation,
        ));
        assert!(!next.is_empty());
        assert!(lifecycle.close());
        assert!(!lifecycle.close());
        assert!(lifecycle.reconcile_demand(&desired, 10).is_empty());
        assert!(recorder.snapshot().closed);
    }
}
