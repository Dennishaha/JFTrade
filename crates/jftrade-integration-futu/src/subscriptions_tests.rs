use std::sync::Arc;

use jftrade_marketdata::InstrumentRef;

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
fn subscription_reconciler_sub_unsub_and_reconciliation_cycle_parity() {
    // Parity: go:452dea11:internal/integration/futu/subscription_reconciler_test.go:12 TestSubscriptionReconcilerSubUnsub
    // Parity: go:452dea11:internal/integration/futu/subscription_reconciler_test.go:88 TestSubscriptionReconcilerReconciliation
    let desired = [reference("TICK", None), reference("ORDER_BOOK", None)];
    let plan = desired_subscriptions(&desired);
    assert_eq!(plan.logical_count, 2);
    assert_eq!(plan.physical.len(), 2);

    let mut reconciler = SubscriptionReconciler::new(60_000);
    // Initial reconciliation: produces subscribe actions for all desired physical items
    let actions = reconciler.actions(&desired, 100, 1);
    assert_eq!(actions.len(), 2);
    assert!(
        actions
            .iter()
            .all(|a| matches!(a, super::ReconcileAction::Subscribe { .. }))
    );

    // Record success: subsequent actions at same or later time with same desired are empty
    for action in &actions {
        reconciler.record_success(action, 100, 1);
    }
    assert!(reconciler.actions(&desired, 100, 1).is_empty());
    assert!(reconciler.actions(&desired, 200, 1).is_empty());

    // When desired drops to empty, minimum age (60s) delays unsubscription
    assert!(reconciler.actions(&[], 30_000, 1).is_empty());
    assert!(reconciler.actions(&[], 60_099, 1).is_empty());

    // Once min_age_ms (60_000) expires, unsubscription actions are emitted
    let unsub_actions = reconciler.actions(&[], 60_100, 1);
    assert_eq!(unsub_actions.len(), 2);
    assert!(
        unsub_actions
            .iter()
            .all(|a| matches!(a, super::ReconcileAction::Unsubscribe { .. }))
    );

    for action in &unsub_actions {
        reconciler.record_success(action, 60_100, 1);
    }
    assert!(reconciler.actions(&[], 60_100, 1).is_empty());
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
    assert_eq!(reconciler.record_failure(subscription, 0, 1, None), 5_000);
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
    assert_eq!(reconciler.record_failure(subscription, 0, 1, None), 5_000);
    assert!(
        reconciler
            .active_instruments(SubscriptionKind::Basic, 1)
            .is_empty()
    );
    // While the instrument stays desired the bounded retry window defers a
    // second attempt, matching Go `subscribeDesiredLocked`.
    assert!(reconciler.actions(&desired, 4_999, 1).is_empty());
    assert_eq!(reconciler.actions(&desired, 5_000, 1).len(), 1);
    // Releasing a never-established record drops it immediately; Go has no
    // physical subscription to release and keeps no retry state.
    assert!(reconciler.actions(&[], 5_000, 1).is_empty());
    assert!(
        reconciler
            .physical_snapshot(&desired, 1, None)
            .entries
            .is_empty()
    );

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
        _ => panic!("expected subscribe action"),
    };
    assert_eq!(
        reconciler.record_unsubscribe_failure(subscription, 0, 1, None),
        5_000
    );
    assert!(reconciler.actions(&[], 4_999, 1).is_empty());
    assert!(matches!(
        reconciler.actions(&[], 5_000, 1).as_slice(),
        [ReconcileAction::Unsubscribe { .. }]
    ));
}

#[test]
fn unsubscribe_retry_ladder_escalates_and_reacquire_clears_retry_state() {
    // Parity: go:452dea11:internal/integration/futu/subscription_reconciler_test.go:287 TestSubscriptionReconcilerRetriesFailuresAndCancelsRetryOnReacquire
    let desired = [reference("SNAPSHOT", None)];
    let mut reconciler = SubscriptionReconciler::new(0);
    let subscribe = reconciler.actions(&desired, 0, 1).pop().expect("subscribe");
    reconciler.record_success(&subscribe, 0, 1);

    let mut now_ms = 0;
    // Go asserts the bounded ladder 5s/10s/20s/30s/30s: each failed unsubscribe
    // advances the window by exactly that delay and the next reconcile inside
    // the window is deferred.
    for (attempt, delay) in [5_000, 10_000, 20_000, 30_000, 30_000]
        .into_iter()
        .enumerate()
    {
        let action = reconciler.actions(&[], now_ms, 1);
        let subscription = match action.as_slice() {
            [ReconcileAction::Unsubscribe { subscription }] => subscription.clone(),
            other => panic!("attempt {attempt}: expected one unsubscribe, got {other:?}"),
        };
        assert_eq!(
            reconciler.record_unsubscribe_failure(
                &subscription,
                now_ms,
                1,
                Some("busy".to_owned())
            ),
            delay,
            "attempt {attempt} delay"
        );
        assert!(
            reconciler.actions(&[], now_ms, 1).is_empty(),
            "attempt {attempt} must be deferred inside its window"
        );
        now_ms += delay;
    }
    // Go asserts subscriptionRetryDelay bounds: negative clamps to the first
    // rung and a huge failure count saturates on the last rung.
    assert_eq!(retry_delay_ms(0), 5_000);
    assert_eq!(retry_delay_ms(99), 30_000);

    // Reacquiring while the record is still established cancels the pending
    // unsubscribe retry: Go clears failures/retryAt/lastError without an RPC.
    assert!(reconciler.actions(&desired, now_ms, 1).is_empty());
    let snapshot = reconciler.physical_snapshot(&desired, 1, None);
    assert_eq!(snapshot.entries.len(), 1);
    assert_eq!(snapshot.entries[0].broker_state, "active");
    assert_eq!(snapshot.entries[0].last_error, None);
    assert_eq!(snapshot.fallback_count, 0);
    // Ladder reset: the next unsubscribe failure must restart at 5s.
    let unsubscribe = reconciler.actions(&[], now_ms, 1);
    let subscription = match unsubscribe.as_slice() {
        [ReconcileAction::Unsubscribe { subscription }] => subscription.clone(),
        other => panic!("expected one unsubscribe, got {other:?}"),
    };
    assert_eq!(
        reconciler.record_unsubscribe_failure(&subscription, now_ms, 1, Some("busy".to_owned())),
        5_000
    );
}

#[test]
fn fallback_instruments_are_filtered_from_the_push_stream() {
    // Parity: go:452dea11:internal/integration/futu/marketdata_runtime_test.go:613 TestMarketDataRuntimeFiltersFallbackInstrumentsFromPushStream
    // Parity: go:452dea11:internal/integration/futu/marketdata_runtime_test.go:937 FilterPushInstruments boundaries
    let desired = [
        reference("SNAPSHOT", None),
        InstrumentRef {
            channel: "SNAPSHOT".to_owned(),
            market: "SH".to_owned(),
            symbol: "600519".to_owned(),
            interval: None,
        },
    ];
    let mut reconciler = SubscriptionReconciler::new(0);
    for action in reconciler.actions(&desired, 0, 1) {
        match action {
            ReconcileAction::Subscribe { subscription }
                if subscription.instrument_id == "SH.600519" =>
            {
                reconciler.record_fallback_failure(&subscription, 0, 1, Some("quota".to_owned()));
            }
            other => reconciler.record_success(&other, 0, 1),
        }
    }
    assert!(reconciler.has_fallback_subscriptions());

    // Only the fallback symbol leaves the push stream; the healthy symbol keeps
    // flowing. Inputs are normalized (trim + upper case) exactly like Go.
    let ids = vec!["SH.600519".to_owned(), "US.AAPL".to_owned()];
    assert_eq!(
        reconciler.filter_push_instruments(&ids),
        vec!["US.AAPL".to_owned()]
    );
    let messy = vec![
        " sh.600519 ".to_owned(),
        String::new(),
        "  ".to_owned(),
        " us.aapl ".to_owned(),
    ];
    assert_eq!(
        reconciler.filter_push_instruments(&messy),
        vec!["US.AAPL".to_owned()]
    );
    // Unparseable ids stay on the stream: Go only drops symbols whose BasicQot
    // subscription is currently served by the delayed fallback.
    assert_eq!(
        reconciler.filter_push_instruments(&["invalid".to_owned()]),
        vec!["INVALID".to_owned()]
    );

    // Recovering the fallback subscription re-admits the symbol to pushes.
    for action in reconciler.actions(&desired, FALLBACK_SUBSCRIPTION_RETRY_MS, 1) {
        reconciler.record_success(&action, FALLBACK_SUBSCRIPTION_RETRY_MS, 1);
    }
    assert!(!reconciler.has_fallback_subscriptions());
    assert_eq!(
        reconciler.filter_push_instruments(&ids),
        vec!["SH.600519".to_owned(), "US.AAPL".to_owned()]
    );

    // A reconciler with no fallback record filters nothing: every parseable
    // symbol stays on the stream (only blanks are dropped).
    let empty = SubscriptionReconciler::new(0);
    assert_eq!(
        empty.filter_push_instruments(&messy),
        vec!["SH.600519".to_owned(), "US.AAPL".to_owned()]
    );
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
            .ingest_session_event(&stale, "2026-08-24T00:00:00Z".parse().expect("ts"))
            .expect("stale")
            .is_none()
    );
    assert_eq!(recorder.snapshot().stream_failures, 0);

    let local = OpenDSessionEvent::Closed {
        generation,
        reason: OpenDSessionCloseReason::Local,
    };
    assert!(
        lifecycle
            .ingest_session_event(&local, "2026-08-24T00:00:01Z".parse().expect("ts"))
            .expect("local")
            .is_none()
    );
    assert_eq!(recorder.snapshot().stream_failures, 0);

    let active = OpenDSessionEvent::Closed {
        generation,
        reason: crate::OpenDSessionCloseReason::PeerClosed,
    };
    assert!(
        lifecycle
            .ingest_session_event(&active, "2026-08-24T00:00:01Z".parse().expect("ts"))
            .expect("active")
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
    assert!(lifecycle.poll_started("2026-08-24T00:00:00Z".parse().expect("ts"), generation));
    assert!(lifecycle.stream_connected(generation));
    assert!(lifecycle.quote_failure(
        "2026-08-24T00:00:00Z".parse().expect("ts"),
        "quote timeout",
        generation
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
            None,
        ),
        Some(5_000)
    );

    lifecycle.reconfigure();
    let next = lifecycle.reconcile_demand(&[reference("SNAPSHOT", None)], 1);
    let next_generation = lifecycle.generation();
    assert_ne!(next_generation, generation);
    assert!(!lifecycle.stream_failure(
        "2026-08-24T00:00:00Z".parse().expect("ts"),
        "stale stream",
        generation
    ));
    assert!(!next.is_empty());
    assert!(lifecycle.close());
    assert!(!lifecycle.close());
    assert!(lifecycle.reconcile_demand(&desired, 10).is_empty());
    assert!(recorder.snapshot().closed);
}

#[test]
fn basic_quote_availability_failure_enters_delayed_fallback_and_recovers() {
    // Parity: internal/integration/futu/subscription_reconciler_test.go:346 TestSubscriptionReconcilerUsesDelayedFallbackForBasicQuoteAvailabilityFailures
    let desired = [reference("SNAPSHOT", None)];
    let mut reconciler = SubscriptionReconciler::new(0);
    let subscribe = reconciler.actions(&desired, 0, 1).pop().expect("subscribe");
    let subscription = match &subscribe {
        ReconcileAction::Subscribe { subscription } => subscription.clone(),
        _ => panic!("expected subscribe action"),
    };

    assert_eq!(
        reconciler.record_fallback_failure(&subscription, 0, 1, None),
        15_000
    );
    assert!(reconciler.has_fallback_subscriptions());
    assert!(reconciler.is_fallback_instrument("US.AAPL"));
    assert!(!reconciler.is_fallback_instrument("US.MSFT"));
    let snapshot = reconciler.physical_snapshot(&desired, 1, None);
    assert_eq!(snapshot.fallback_count, 1);
    assert_eq!(snapshot.own_active_count, 0);
    assert_eq!(snapshot.entries.len(), 1);
    assert_eq!(snapshot.entries[0].broker_state, "fallback");

    // Retry is deferred until the fallback window elapses, then succeeds.
    assert!(reconciler.actions(&desired, 14_999, 1).is_empty());
    assert_eq!(reconciler.actions(&desired, 15_000, 1).len(), 1);
    reconciler.record_success(&subscribe, 15_000, 1);
    assert!(!reconciler.has_fallback_subscriptions());
    assert!(!reconciler.is_fallback_instrument("US.AAPL"));
    let snapshot = reconciler.physical_snapshot(&desired, 1, None);
    assert_eq!(snapshot.fallback_count, 0);
    assert_eq!(snapshot.own_active_count, 1);
    assert_eq!(snapshot.entries[0].broker_state, "active");
}

#[test]
fn non_basic_or_non_availability_failures_do_not_enter_fallback() {
    // Parity: internal/integration/futu/subscription_reconciler.go:185 eligibility gate
    let desired = [reference("KLINE", Some("1m"))];
    let mut reconciler = SubscriptionReconciler::new(0);
    let actions = reconciler.actions(&desired, 0, 1);
    let kline = actions
        .iter()
        .find_map(|action| match action {
            ReconcileAction::Subscribe { subscription }
                if subscription.kind == SubscriptionKind::Kline =>
            {
                Some(subscription.clone())
            }
            _ => None,
        })
        .expect("kline subscribe");

    assert_eq!(reconciler.record_failure(&kline, 0, 1, None), 5_000);
    assert!(!reconciler.has_fallback_subscriptions());
    assert!(!reconciler.is_fallback_instrument("US.AAPL"));
    assert_eq!(
        reconciler
            .physical_snapshot(&desired, 1, None)
            .fallback_count,
        0
    );
}

#[test]
fn plain_basic_failure_keeps_retry_semantics_without_fallback_count() {
    // Parity: internal/integration/futu/subscription_reconciler_test.go:287 TestSubscriptionReconcilerRetriesFailuresAndCancelsRetryOnReacquire
    let desired = [reference("SNAPSHOT", None)];
    let mut reconciler = SubscriptionReconciler::new(0);
    let subscribe = reconciler.actions(&desired, 0, 1).pop().expect("subscribe");
    let subscription = match &subscribe {
        ReconcileAction::Subscribe { subscription } => subscription.clone(),
        _ => panic!("expected subscribe action"),
    };

    assert_eq!(
        reconciler.record_failure(&subscription, 0, 1, Some("subscribe denied".to_owned())),
        5_000
    );
    assert_eq!(
        reconciler
            .physical_snapshot(&desired, 1, None)
            .fallback_count,
        0
    );
    assert!(!reconciler.has_fallback_subscriptions());
    assert!(reconciler.actions(&desired, 4_999, 1).is_empty());
    assert_eq!(reconciler.actions(&desired, 5_000, 1).len(), 1);
}

#[test]
fn released_never_established_records_are_dropped_without_fallback_leakage() {
    // Parity: internal/integration/futu/subscription_reconciler_test.go:676 TestSubscriptionReconcilerDropsFailedRecordsReleasedBeforeRetry
    let desired = [reference("SNAPSHOT", None)];
    let mut reconciler = SubscriptionReconciler::new(0);
    let subscribe = reconciler.actions(&desired, 0, 1).pop().expect("subscribe");
    let subscription = match &subscribe {
        ReconcileAction::Subscribe { subscription } => subscription.clone(),
        _ => panic!("expected subscribe action"),
    };
    reconciler.record_failure(&subscription, 0, 1, Some("denied".to_owned()));
    assert_eq!(
        reconciler
            .physical_snapshot(&desired, 1, None)
            .entries
            .len(),
        1
    );

    // Leaving demand before any successful subscribe drops the record entirely.
    assert!(reconciler.actions(&[], 1, 1).is_empty());
    assert!(
        reconciler
            .physical_snapshot(&desired, 1, None)
            .entries
            .is_empty()
    );

    // A fallback record is dropped the same way, and the fallback count follows.
    let mut reconciler = SubscriptionReconciler::new(0);
    let subscribe = reconciler.actions(&desired, 0, 1).pop().expect("subscribe");
    let subscription = match &subscribe {
        ReconcileAction::Subscribe { subscription } => subscription.clone(),
        _ => panic!("expected subscribe action"),
    };
    reconciler.record_fallback_failure(&subscription, 0, 1, Some("quota".to_owned()));
    assert_eq!(
        reconciler
            .physical_snapshot(&desired, 1, None)
            .fallback_count,
        1
    );
    assert!(reconciler.actions(&[], 1, 1).is_empty());
    let snapshot = reconciler.physical_snapshot(&desired, 1, None);
    assert!(snapshot.entries.is_empty());
    assert_eq!(snapshot.fallback_count, 0);
    assert!(!reconciler.has_fallback_subscriptions());
}
