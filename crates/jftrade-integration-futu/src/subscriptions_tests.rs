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
