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

/// Delayed snapshot retry window used when a BasicQot subscription cannot be
/// established because of entitlement, unsupported symbol or quota pressure.
/// Matches Go `fallbackSubscriptionRetry`.
pub const FALLBACK_SUBSCRIPTION_RETRY_MS: i64 = 15_000;

impl ReconcileAction {
    pub fn key(&self) -> &str {
        match self {
            ReconcileAction::Subscribe { subscription }
            | ReconcileAction::Unsubscribe { subscription } => &subscription.key,
        }
    }
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
    /// `None` until OpenD acknowledges the subscribe, matching Go's zero
    /// `time.Time` sentinel for records that were never established.
    subscribed_at_ms: Option<i64>,
    generation: u64,
    active: bool,
    failures: usize,
    last_error: Option<String>,
    retry_at_ms: i64,
    fallback: bool,
}

#[derive(Clone, Debug)]
pub struct SubscriptionReconciler {
    records: BTreeMap<String, ActiveRecord>,
    minimum_age_ms: i64,
    total_used_quota: Option<u64>,
    remain_quota: Option<u64>,
    own_used_quota: Option<u64>,
    quota_checked_at_ms: Option<i64>,
    quota_last_error: Option<String>,
    last_reconciled_at_ms: Option<i64>,
}

impl SubscriptionReconciler {
    pub fn new(minimum_age_ms: i64) -> Self {
        Self {
            records: BTreeMap::new(),
            minimum_age_ms: minimum_age_ms.max(0),
            total_used_quota: None,
            remain_quota: None,
            own_used_quota: None,
            quota_checked_at_ms: None,
            quota_last_error: None,
            last_reconciled_at_ms: None,
        }
    }

    pub fn set_quota(
        &mut self,
        total_used: Option<u64>,
        remain: Option<u64>,
        own_used: Option<u64>,
        checked_at_ms: i64,
        error: Option<String>,
    ) {
        if total_used.is_some() {
            self.total_used_quota = total_used;
        }
        if remain.is_some() {
            self.remain_quota = remain;
        }
        if own_used.is_some() {
            self.own_used_quota = own_used;
        }
        self.quota_checked_at_ms = if checked_at_ms > 0 {
            Some(checked_at_ms)
        } else {
            None
        };
        self.quota_last_error = error;
    }

    pub fn set_last_reconciled_at_ms(&mut self, ms: i64) {
        self.last_reconciled_at_ms = if ms > 0 { Some(ms) } else { None };
    }

    pub fn actions(
        &mut self,
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
                Some(record) if record.generation == generation && now_ms < record.retry_at_ms => {}
                _ => actions.push(ReconcileAction::Subscribe {
                    subscription: subscription.clone(),
                }),
            }
        }
        // Go `subscribeDesiredLocked` cancels a pending unsubscribe retry as soon
        // as the record is wanted again: an already established subscription has
        // nothing to re-subscribe, so the bounded retry ladder, the deferred
        // deadline and the last error are cleared without an OpenD round trip.
        for (key, record) in &mut self.records {
            if record.generation != generation
                || !record.active
                || !desired_by_key.contains_key(key)
            {
                continue;
            }
            record.retry_at_ms = 0;
            record.failures = 0;
            record.last_error = None;
            record.fallback = false;
        }
        // Go drops records that were never established (subscribe rejected or
        // served by the delayed fallback) as soon as they leave demand: there is
        // no physical subscription to release and no state to retain.
        let never_established = self
            .records
            .iter()
            .filter(|(key, record)| {
                record.subscribed_at_ms.is_none() && !desired_by_key.contains_key(*key)
            })
            .map(|(key, _)| key.clone())
            .collect::<Vec<_>>();
        for key in never_established {
            self.records.remove(&key);
        }
        for (key, record) in &self.records {
            if record.active
                && !desired_by_key.contains_key(key)
                && now_ms.saturating_sub(record.subscribed_at_ms.unwrap_or_default())
                    >= self.minimum_age_ms
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
                        subscribed_at_ms: Some(now_ms),
                        generation,
                        active: true,
                        failures: 0,
                        last_error: None,
                        retry_at_ms: 0,
                        fallback: false,
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
        error: Option<String>,
    ) -> i64 {
        let record = self
            .records
            .entry(subscription.key.clone())
            .or_insert_with(|| ActiveRecord {
                subscription: subscription.clone(),
                subscribed_at_ms: None,
                generation,
                active: false,
                failures: 0,
                last_error: None,
                retry_at_ms: 0,
                fallback: false,
            });
        record.generation = generation;
        if record.fallback {
            record.fallback = false;
        }
        record.active = false;
        record.last_error = error;
        let delay = retry_delay_ms(record.failures);
        record.failures = record.failures.saturating_add(1);
        record.retry_at_ms = now_ms.saturating_add(delay);
        delay
    }

    /// Records a BasicQot subscribe failure that is served by the delayed
    /// fallback source. Matches Go `recordSubscriptionFallback`: the record is
    /// neither active nor counted toward the retry ladder, is retried after
    /// [`FALLBACK_SUBSCRIPTION_RETRY_MS`], and is billed to the fallback count.
    pub fn record_fallback_failure(
        &mut self,
        subscription: &PhysicalSubscription,
        now_ms: i64,
        generation: u64,
        error: Option<String>,
    ) -> i64 {
        let record = self
            .records
            .entry(subscription.key.clone())
            .or_insert_with(|| ActiveRecord {
                subscription: subscription.clone(),
                subscribed_at_ms: None,
                generation,
                active: false,
                failures: 0,
                last_error: None,
                retry_at_ms: 0,
                fallback: false,
            });
        record.generation = generation;
        record.subscribed_at_ms = None;
        record.active = false;
        record.failures = 0;
        record.last_error = error;
        record.retry_at_ms = now_ms.saturating_add(FALLBACK_SUBSCRIPTION_RETRY_MS);
        record.fallback = true;
        FALLBACK_SUBSCRIPTION_RETRY_MS
    }

    /// Reports whether the active connection serves this instrument from the
    /// delayed snapshot fallback instead of an OpenD BasicQot subscription.
    pub fn is_fallback_instrument(&self, instrument_id: &str) -> bool {
        let Ok(reference) = (InstrumentRef {
            channel: "SNAPSHOT".to_owned(),
            market: String::new(),
            symbol: instrument_id.to_owned(),
            interval: None,
        })
        .normalize() else {
            return false;
        };
        self.records
            .get(&format!("BASIC:{}", reference.instrument_id()))
            .is_some_and(|record| record.fallback)
    }

    /// Reports whether any desired BasicQot subscription is currently served by
    /// the delayed snapshot fallback.
    pub fn has_fallback_subscriptions(&self) -> bool {
        self.records
            .values()
            .any(|record| record.subscription.kind == SubscriptionKind::Basic && record.fallback)
    }

    /// Removes only the symbols whose BasicQot subscription is currently served
    /// by the delayed snapshot fallback. Every other symbol keeps sharing the
    /// same OpenD push stream.
    ///
    /// Matches Go `MarketDataRuntime.FilterPushInstruments`: inputs are trimmed
    /// and upper-cased, blank entries are dropped, and unparseable symbols stay
    /// on the stream because they cannot be served by the fallback either.
    pub fn filter_push_instruments(&self, instrument_ids: &[String]) -> Vec<String> {
        instrument_ids
            .iter()
            .filter_map(|raw| {
                let instrument_id = raw.trim().to_ascii_uppercase();
                if instrument_id.is_empty() || self.is_fallback_instrument(&instrument_id) {
                    return None;
                }
                Some(instrument_id)
            })
            .collect()
    }

    pub fn record_unsubscribe_failure(
        &mut self,
        subscription: &PhysicalSubscription,
        now_ms: i64,
        generation: u64,
        error: Option<String>,
    ) -> i64 {
        let Some(record) = self.records.get_mut(&subscription.key) else {
            return retry_delay_ms(0);
        };
        if record.generation != generation {
            return retry_delay_ms(0);
        }
        record.last_error = error;
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
            record.fallback = false;
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

    pub fn physical_snapshot(
        &self,
        desired: &[InstrumentRef],
        generation: u64,
        observed_generation: Option<u64>,
    ) -> jftrade_marketdata::PhysicalSubscriptionSnapshot {
        let desired_by_key = desired_subscriptions(desired)
            .physical
            .into_iter()
            .map(|sub| (sub.key.clone(), sub))
            .collect::<BTreeMap<_, _>>();
        let mut entries = Vec::with_capacity(self.records.len());
        let mut active_count = 0;
        let mut pending_release_count = 0;
        // Go derives `fallbackCount` from the records it can still see, so a
        // stale connection (or one whose records were dropped) reports zero
        // instead of a counter that outlived its subscription.
        let mut fallback_count = 0;
        // Go `connectionChangedLocked`: when OpenD reports a different
        // connection generation than the one this reconciler established, none
        // of the recorded subscriptions are live any more. Reporting them as
        // `pending_reconnect` keeps a stale generation from being mistaken for
        // current ownership before the replay pass runs.
        let connection_changed = observed_generation.is_some_and(|observed| observed != generation);
        for (key, record) in &self.records {
            let desired = desired_by_key.contains_key(key);
            let state = if connection_changed {
                "pending_reconnect"
            } else if record.fallback {
                fallback_count += 1;
                "fallback"
            } else if record.active && record.generation == generation {
                // Go counts every established subscription in `ownActiveCount`,
                // including the ones whose demand already went away: the
                // physical subscription is still held (and still billed to the
                // OpenD quota) until the minimum age elapses, which is exactly
                // what `pendingReleaseCount` separately reports.
                active_count += 1;
                if desired {
                    "active"
                } else {
                    pending_release_count += 1;
                    // Go/`marketDataContract.ts` name this state
                    // `pending_unsubscribe`; the web contract drops any other
                    // spelling, so the entry would otherwise render as unknown.
                    "pending_unsubscribe"
                }
            } else if record.failures > 0 {
                "retrying"
            } else {
                "pending_subscribe"
            };
            let subscribed_at = record.subscribed_at_ms.and_then(format_millis_rfc3339);
            let unsubscribe_eligible_at = record
                .subscribed_at_ms
                .and_then(|subscribed_at_ms| subscribed_at_ms.checked_add(self.minimum_age_ms))
                .and_then(format_millis_rfc3339);
            let kind_str = match record.subscription.kind {
                SubscriptionKind::Basic => "BASIC",
                SubscriptionKind::Kline => "KLINE",
                SubscriptionKind::OrderBook => "ORDER_BOOK",
            };
            entries.push(jftrade_marketdata::PhysicalSubscriptionEntry {
                key: key.clone(),
                kind: kind_str.to_owned(),
                instrument_id: record.subscription.instrument_id.clone(),
                interval: record.subscription.interval.clone(),
                broker_state: state.to_owned(),
                subscribed_at,
                unsubscribe_eligible_at,
                last_error: record.last_error.clone(),
            });
        }
        jftrade_marketdata::PhysicalSubscriptionSnapshot {
            desired_count: desired_by_key.len(),
            own_active_count: active_count,
            pending_release_count,
            fallback_count,
            connection_generation: Some(generation),
            observed_connection_generation: observed_generation.or(Some(generation)),
            total_used_quota: self.total_used_quota,
            remain_quota: self.remain_quota,
            own_used_quota: self.own_used_quota,
            checked_at: self.quota_checked_at_ms.and_then(format_millis_rfc3339),
            last_error: self.quota_last_error.clone(),
            reconciled_at: self.last_reconciled_at_ms.and_then(format_millis_rfc3339),
            entries,
        }
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
    reconnect_generation: u64,
    closed: bool,
}

impl OpenDSubscriptionLifecycle {
    pub fn new(recorder: Arc<MarketDataRuntimeRecorder>, minimum_subscription_age_ms: i64) -> Self {
        Self {
            reconciler: SubscriptionReconciler::new(minimum_subscription_age_ms),
            recorder,
            desired: Vec::new(),
            generation: 0,
            reconnect_generation: 0,
            closed: false,
        }
    }

    pub fn set_quota(
        &mut self,
        total_used: Option<u64>,
        remain: Option<u64>,
        own_used: Option<u64>,
        checked_at_ms: i64,
        error: Option<String>,
    ) {
        self.reconciler
            .set_quota(total_used, remain, own_used, checked_at_ms, error);
    }

    pub fn set_last_reconciled_at_ms(&mut self, ms: i64) {
        self.reconciler.set_last_reconciled_at_ms(ms);
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

    pub fn physical_snapshot(&self) -> jftrade_marketdata::PhysicalSubscriptionSnapshot {
        self.physical_snapshot_with_observed(None)
    }

    pub fn physical_snapshot_with_observed(
        &self,
        observed_generation: Option<u64>,
    ) -> jftrade_marketdata::PhysicalSubscriptionSnapshot {
        if self.closed {
            return jftrade_marketdata::PhysicalSubscriptionSnapshot::default();
        }
        self.reconciler
            .physical_snapshot(&self.desired, self.generation, observed_generation)
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
        self.reconnect_generation = self.generation;
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
        error: Option<String>,
    ) -> Option<i64> {
        if self.closed || generation != self.generation {
            return None;
        }
        Some(
            self.reconciler
                .record_failure(subscription, now_ms, generation, error),
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
            || executor.session().managed_session().generation() < self.reconnect_generation
            || executor.session().managed_session().generation() > generation
        {
            return Ok(false);
        }
        match executor.execute(action) {
            Ok(()) => {
                self.reconciler.record_success(action, now_ms, generation);
                Ok(true)
            }
            Err(error) => {
                let err_str = error.to_string();
                match action {
                    ReconcileAction::Subscribe { subscription }
                        if subscription.kind == SubscriptionKind::Basic
                            && error.snapshot_availability_kind().is_some() =>
                    {
                        // Go records the quote-access failure as a delayed
                        // fallback instead of surfacing a reconcile error, so a
                        // single unavailable BasicQot does not fail the whole
                        // reconcile pass.
                        self.reconciler.record_fallback_failure(
                            subscription,
                            now_ms,
                            generation,
                            Some(err_str),
                        );
                        return Ok(true);
                    }
                    ReconcileAction::Subscribe { subscription } => {
                        self.reconciler.record_failure(
                            subscription,
                            now_ms,
                            generation,
                            Some(err_str),
                        );
                    }
                    ReconcileAction::Unsubscribe { subscription } => {
                        self.reconciler.record_unsubscribe_failure(
                            subscription,
                            now_ms,
                            generation,
                            Some(err_str),
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

fn format_millis_rfc3339(ms: i64) -> Option<String> {
    if ms <= 0 {
        return None;
    }
    time::OffsetDateTime::from_unix_timestamp_nanos(i128::from(ms) * 1_000_000)
        .ok()
        .and_then(|dt| {
            dt.format(&time::format_description::well_known::Rfc3339)
                .ok()
        })
}

#[cfg(test)]
#[path = "subscriptions_tests.rs"]
mod tests;
