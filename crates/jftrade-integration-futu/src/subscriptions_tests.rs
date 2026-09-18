use std::sync::Arc;

use jftrade_marketdata::InstrumentRef;

use super::*;
use crate::{PROTO_UPDATE_BASIC_QOT, decode_frame, encode_frame};

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
fn quote_push_ingestion_drops_malformed_and_stale_frames_without_registering_handlers() {
    // Parity: go:452dea11:pkg/futu/opend/client_transport_boundaries_test.go:202 TestSubscribeNotifySkipsNilAndMalformedPush
    //
    // Go's SubscribeNotify ignores a nil handler and swallows a malformed
    // notify payload. Rust has no callback registry; the equivalent boundary is
    // that a malformed unsolicited frame is dropped by the lifecycle without
    // producing a push or a stream failure, and a stale generation is rejected
    // before decode.
    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let mut lifecycle = OpenDSubscriptionLifecycle::new(Arc::clone(&recorder), 60_000);
    let generation = lifecycle.generation();
    lifecycle.reconcile_demand(
        &[InstrumentRef {
            channel: "SNAPSHOT".to_owned(),
            market: "US".to_owned(),
            symbol: "AAPL".to_owned(),
            interval: None,
        }],
        0,
    );

    let malformed =
        decode_frame(&encode_frame(PROTO_UPDATE_BASIC_QOT, 0, &[0xff]).expect("malformed frame"))
            .expect("decode frame");
    let now: jftrade_kernel::WireTimestamp = "2026-08-24T00:00:00Z".parse().expect("timestamp");
    assert_eq!(
        lifecycle
            .ingest_quote_push(&malformed, now, generation)
            .expect("malformed push is dropped, not an error"),
        None
    );
    assert_eq!(
        lifecycle
            .ingest_quote_push(&malformed, now, generation + 1)
            .expect("stale generation is rejected before decode"),
        None
    );
    assert_eq!(recorder.snapshot().stream_failures, 0);
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
fn exact_physical_subscriptions_are_shared_and_final_release_is_deferred() {
    // Parity: go:452dea11:internal/integration/futu/subscription_reconciler_test.go:88 TestSubscriptionReconcilerSharesExactPhysicalSubscriptionsAndDefersFinalRelease
    let desired = [
        InstrumentRef {
            channel: "KLINE".to_owned(),
            market: "hk".to_owned(),
            symbol: "00700".to_owned(),
            interval: Some("1m".to_owned()),
        },
        InstrumentRef {
            channel: "kline".to_owned(),
            market: "HK".to_owned(),
            symbol: "HK.00700".to_owned(),
            interval: Some("1M".to_owned()),
        },
        InstrumentRef {
            channel: "KLINE".to_owned(),
            market: "HK".to_owned(),
            symbol: "00700".to_owned(),
            interval: Some("5m".to_owned()),
        },
        InstrumentRef {
            channel: "TICK".to_owned(),
            market: "HK".to_owned(),
            symbol: "00700".to_owned(),
            interval: None,
        },
    ];
    let plan = desired_subscriptions(&desired);
    // Case/prefix variants collapse into one logical KLINE:1m entry, so three
    // logical capabilities map onto three physical subscriptions.
    assert_eq!(plan.logical_count, 3);
    let keys: Vec<_> = plan.physical.iter().map(|s| s.key.as_str()).collect();
    assert_eq!(
        keys,
        vec!["BASIC:HK.00700", "KLINE:HK.00700:1m", "KLINE:HK.00700:5m"]
    );

    let mut reconciler = SubscriptionReconciler::new(60_000);
    let actions = reconciler.actions(&desired, 0, 1);
    assert_eq!(actions.len(), 3);
    for action in &actions {
        reconciler.record_success(action, 0, 1);
    }
    let snapshot = reconciler.physical_snapshot(&desired, 1, None);
    assert_eq!(snapshot.desired_count, 3);
    assert_eq!(snapshot.own_active_count, 3);
    assert_eq!(snapshot.pending_release_count, 0);

    // Releasing everything before the minimum age keeps the physical
    // subscriptions alive and only marks them pending.
    assert!(reconciler.actions(&[], 30_000, 1).is_empty());
    let snapshot = reconciler.physical_snapshot(&[], 1, None);
    assert_eq!(snapshot.pending_release_count, 3);

    // Re-acquiring one capability while pending reuses every physical
    // subscription without a new RPC.
    assert!(reconciler.actions(&desired[..1], 30_000, 1).is_empty());
    let snapshot = reconciler.physical_snapshot(&desired[..1], 1, None);
    assert_eq!(snapshot.pending_release_count, 1);

    // Past the minimum age only the no-longer-desired 5m K-line is released.
    let release = reconciler.actions(&desired[..1], 61_000, 1);
    let released: Vec<_> = release
        .iter()
        .map(|action| match action {
            ReconcileAction::Unsubscribe { subscription } => subscription.key.as_str(),
            other => panic!("expected unsubscribe, got {other:?}"),
        })
        .collect();
    assert_eq!(released, vec!["KLINE:HK.00700:5m"]);
    // Go performs the physical unsubscribe inside the reconcile pass; here the
    // plan is only committed once the executor reports success.
    for action in &release {
        reconciler.record_success(action, 61_000, 1);
    }

    // Final release drops the remaining two physical subscriptions.
    let final_release = reconciler.actions(&[], 61_000, 1);
    assert_eq!(final_release.len(), 2);
    for action in &final_release {
        reconciler.record_success(action, 61_000, 1);
    }
    let snapshot = reconciler.physical_snapshot(&[], 1, None);
    assert_eq!(snapshot.own_active_count, 0);
    assert_eq!(snapshot.pending_release_count, 0);
    assert!(snapshot.entries.is_empty());
}

#[test]
fn viewers_share_capabilities_and_only_the_stale_order_book_is_released() {
    // Parity: go:452dea11:internal/integration/futu/subscription_reconciler_test.go:713 TestSubscriptionReconcilerKeepsThreeViewerCapabilitiesAndReleasesOnlyOldOrderBook
    let aapl = |channel: &str, interval: Option<&str>| InstrumentRef {
        channel: channel.to_owned(),
        market: "US".to_owned(),
        symbol: "AAPL".to_owned(),
        interval: interval.map(str::to_owned),
    };
    let msft = |channel: &str, interval: Option<&str>| InstrumentRef {
        channel: channel.to_owned(),
        market: "US".to_owned(),
        symbol: "MSFT".to_owned(),
        interval: interval.map(str::to_owned),
    };
    let viewer_a = [aapl("KLINE", Some("1m")), aapl("ORDER_BOOK", None)];
    // Strategy keeps watching AAPL 1m while viewer B replaces the A view.
    let strategy_a_viewer_b = [
        aapl("KLINE", Some("1m")),
        msft("KLINE", Some("1m")),
        msft("ORDER_BOOK", None),
    ];

    let mut reconciler = SubscriptionReconciler::new(60_000);
    let plan = desired_subscriptions(&viewer_a);
    let keys: Vec<_> = plan.physical.iter().map(|s| s.key.as_str()).collect();
    assert_eq!(
        keys,
        vec!["BASIC:US.AAPL", "KLINE:US.AAPL:1m", "ORDER_BOOK:US.AAPL"]
    );
    for action in reconciler.actions(&viewer_a, 0, 1) {
        reconciler.record_success(&action, 0, 1);
    }

    // Switching viewers subscribes exactly the new B capabilities and leaves
    // only the old AAPL order book pending, because the strategy capability is
    // still wanted.
    let switch = reconciler.actions(&strategy_a_viewer_b, 0, 1);
    let new_keys: Vec<_> = switch
        .iter()
        .map(|action| match action {
            ReconcileAction::Subscribe { subscription } => subscription.key.as_str(),
            other => panic!("expected subscribe, got {other:?}"),
        })
        .collect();
    assert_eq!(
        new_keys,
        vec!["BASIC:US.MSFT", "KLINE:US.MSFT:1m", "ORDER_BOOK:US.MSFT"]
    );
    for action in &switch {
        reconciler.record_success(action, 0, 1);
    }
    assert_eq!(
        reconciler
            .physical_snapshot(&strategy_a_viewer_b, 1, None)
            .pending_release_count,
        1
    );

    // Switching back before eligibility reuses the pending AAPL order book.
    assert!(reconciler.actions(&viewer_a, 0, 1).is_empty());
    assert_eq!(
        reconciler
            .physical_snapshot(&viewer_a, 1, None)
            .pending_release_count,
        3
    );
    // ...and switching forward again also reuses every pending subscription.
    assert!(reconciler.actions(&strategy_a_viewer_b, 0, 1).is_empty());

    // Past the minimum age the stale AAPL order book is released and the
    // strategy's AAPL 1m K-line survives.
    let release = reconciler.actions(&strategy_a_viewer_b, 60_000, 1);
    let released: Vec<_> = release
        .iter()
        .map(|action| match action {
            ReconcileAction::Unsubscribe { subscription } => subscription.key.as_str(),
            other => panic!("expected unsubscribe, got {other:?}"),
        })
        .collect();
    assert_eq!(released, vec!["ORDER_BOOK:US.AAPL"]);
}

#[test]
fn stale_observed_generation_reports_pending_reconnect_instead_of_active() {
    // Parity: go:452dea11:internal/integration/futu/subscription_reconciler_test.go:191 TestSubscriptionReconcilerReplaysAllDesiredSubscriptionsWhenConnectionGenerationChanges
    let desired = [reference("KLINE", Some("1m"))];
    let mut reconciler = SubscriptionReconciler::new(0);
    for action in reconciler.actions(&desired, 0, 2) {
        reconciler.record_success(&action, 0, 2);
    }
    let current = reconciler.physical_snapshot(&desired, 2, Some(2));
    assert_eq!(current.own_active_count, 2);
    assert_eq!(current.connection_generation, Some(2));
    assert_eq!(current.observed_connection_generation, Some(2));
    assert!(current.entries.iter().all(|e| e.broker_state == "active"));

    // Out-of-band connection change: OpenD reports generation 3 while this
    // reconciler still believes it owns generation 2. Go reports the entries as
    // `pending_reconnect` and counts no active subscription, so a stale
    // generation can never be mistaken for live ownership.
    let stale = reconciler.physical_snapshot(&desired, 2, Some(3));
    assert_eq!(stale.connection_generation, Some(2));
    assert_eq!(stale.observed_connection_generation, Some(3));
    assert_eq!(stale.own_active_count, 0);
    assert_eq!(stale.pending_release_count, 0);
    assert!(
        stale
            .entries
            .iter()
            .all(|entry| entry.broker_state == "pending_reconnect"),
        "stale entries = {:?}",
        stale.entries
    );

    // Go derives `fallbackCount` from the records it can still see, so a stale
    // connection reports zero fallbacks instead of the stale counter. A
    // non-zero count here would keep the delayed-snapshot path advertised for a
    // connection that no longer owns any subscription.
    assert_eq!(stale.fallback_count, 0);
    let fallback_desired = [reference("SNAPSHOT", None)];
    let mut fallback_reconciler = SubscriptionReconciler::new(0);
    let subscribe = fallback_reconciler
        .actions(&fallback_desired, 0, 2)
        .pop()
        .expect("subscribe");
    let subscription = match &subscribe {
        ReconcileAction::Subscribe { subscription } => subscription.clone(),
        _ => panic!("expected subscribe"),
    };
    fallback_reconciler.record_fallback_failure(&subscription, 0, 2, Some("quota".to_owned()));
    assert_eq!(
        fallback_reconciler
            .physical_snapshot(&fallback_desired, 2, Some(2))
            .fallback_count,
        1
    );
    assert_eq!(
        fallback_reconciler
            .physical_snapshot(&fallback_desired, 2, Some(3))
            .fallback_count,
        0,
        "a stale connection must not advertise the fallback path"
    );
}

#[test]
fn retention_is_measured_from_the_opend_acknowledgement() {
    // Parity: go:452dea11:internal/integration/futu/subscription_reconciler_test.go:459 TestSubscriptionReconcilerMeasuresRetentionFromOpenDAcknowledgement
    let desired = [reference("SNAPSHOT", None)];
    let mut reconciler = SubscriptionReconciler::new(60_000);
    let subscribe = reconciler.actions(&desired, 0, 1).pop().expect("subscribe");

    // The subscribe RPC started at t=0 and was acknowledged at t=10s. Go stamps
    // `subscribedAt` with the acknowledgement so the retention window is
    // measured from OpenD's view, not from the beginning of a slow pass.
    let acknowledged_at_ms = 10_000;
    reconciler.record_success(&subscribe, acknowledged_at_ms, 1);
    let snapshot = reconciler.physical_snapshot(&desired, 1, None);
    assert_eq!(
        snapshot.entries[0].subscribed_at.as_deref(),
        Some("1970-01-01T00:00:10Z")
    );
    assert_eq!(
        snapshot.entries[0].unsubscribe_eligible_at.as_deref(),
        Some("1970-01-01T00:01:10Z")
    );

    // Releasing at start+minimum_age (60s) must NOT be eligible, because only
    // 50s have elapsed since the acknowledgement.
    assert!(reconciler.actions(&[], 60_000, 1).is_empty());
    // At acknowledgement+minimum_age (70s) the record is released.
    assert_eq!(reconciler.actions(&[], 70_000, 1).len(), 1);
}

#[test]
fn retry_is_measured_from_the_opend_failure_acknowledgement() {
    // Parity: go:452dea11:internal/integration/futu/subscription_reconciler_test.go:499 TestSubscriptionReconcilerMeasuresRetryFromOpenDFailureAcknowledgement
    let desired = [reference("SNAPSHOT", None)];
    let mut reconciler = SubscriptionReconciler::new(60_000);
    let subscribe = reconciler.actions(&desired, 0, 1).pop().expect("subscribe");
    reconciler.record_success(&subscribe, 0, 1);
    let unsubscribe = reconciler
        .actions(&[], 60_000, 1)
        .pop()
        .expect("unsubscribe");
    let subscription = match &unsubscribe {
        ReconcileAction::Unsubscribe { subscription } => subscription.clone(),
        _ => panic!("expected unsubscribe"),
    };

    // The failed unsubscribe started at 60s and was acknowledged at 70s, so the
    // 5s retry ladder is anchored at 70s rather than the pass start.
    let failure_acknowledged_at_ms = 70_000;
    assert_eq!(
        reconciler.record_unsubscribe_failure(
            &subscription,
            failure_acknowledged_at_ms,
            1,
            Some("busy".to_owned())
        ),
        5_000
    );
    assert!(reconciler.actions(&[], 74_999, 1).is_empty());
    assert!(matches!(
        reconciler.actions(&[], 75_000, 1).as_slice(),
        [ReconcileAction::Unsubscribe { .. }]
    ));
}

#[test]
fn concurrent_reconcile_passes_are_idempotent_for_subscribe_and_release() {
    // Parity: go:452dea11:internal/integration/futu/subscription_reconciler_test.go:160 TestSubscriptionReconcilerConcurrentReconcileIsIdempotent
    // Go runs 64 concurrent ReconcileSubscriptions calls and asserts exactly two
    // subscribe calls, then exactly two unsubscribe calls. Rust enforces the
    // same single-writer rule two ways: `SubscriptionReconciler` only exposes
    // `&mut self`, and shared owners wrap it in `Arc<Mutex<_>>`. Exercise the
    // shared-owner form so real thread interleaving is covered.
    let desired = [reference("KLINE", Some("1m"))];
    let shared = Arc::new(std::sync::Mutex::new(SubscriptionReconciler::new(0)));
    let desired_arc = Arc::new(desired.to_vec());

    let subscribe_actions = run_shared_reconciles(&shared, &desired_arc, 64, 0);
    assert_eq!(
        subscribe_actions, 2,
        "concurrent reconcile must plan the two physical subscriptions once"
    );
    {
        let reconciler = shared.lock().expect("lock");
        let snapshot = reconciler.physical_snapshot(&desired, 1, None);
        assert_eq!(snapshot.desired_count, 2);
        assert_eq!(snapshot.own_active_count, 2);
        assert_eq!(snapshot.pending_release_count, 0);
    }

    // Releasing concurrently after the minimum age must plan each physical
    // unsubscribe exactly once even though every pass sees the same records.
    let release_actions = run_shared_reconciles(&shared, &Arc::new(Vec::new()), 64, 0);
    assert_eq!(
        release_actions, 2,
        "concurrent release must not double-unsubscribe"
    );
    let reconciler = shared.lock().expect("lock");
    let snapshot = reconciler.physical_snapshot(&[], 1, None);
    assert_eq!(snapshot.desired_count, 0);
    assert_eq!(snapshot.own_active_count, 0);
    assert_eq!(snapshot.pending_release_count, 0);
    assert!(snapshot.entries.is_empty());
}

/// Runs `workers` shared-owner reconcile passes and returns how many subscribe
/// plans the first pass produced. The first pass commits its plan (as the
/// executor does after a successful OpenD round trip); every later pass must
/// then observe the committed state and plan nothing.
fn run_shared_reconciles(
    shared: &Arc<std::sync::Mutex<SubscriptionReconciler>>,
    desired: &Arc<Vec<InstrumentRef>>,
    workers: usize,
    now_ms: i64,
) -> usize {
    let first_pass = {
        let mut reconciler = shared.lock().expect("lock");
        let actions = reconciler.actions(desired, now_ms, 1);
        for action in &actions {
            reconciler.record_success(action, now_ms, 1);
        }
        actions.len()
    };
    let mut handles = Vec::with_capacity(workers);
    for _ in 0..workers {
        let shared = Arc::clone(shared);
        let desired = Arc::clone(desired);
        handles.push(std::thread::spawn(move || {
            // `actions` takes `&mut self`; the mutex guarantees one writer.
            let mut reconciler = shared.lock().expect("lock");
            reconciler.actions(&desired, now_ms, 1).len()
        }));
    }
    for handle in handles {
        assert_eq!(
            handle.join().expect("worker"),
            0,
            "a later pass must not re-plan committed actions"
        );
    }
    first_pass
}

#[test]
fn desired_physical_subscriptions_reject_incomplete_refs_and_normalize_symbols() {
    // Parity: go:452dea11:internal/integration/futu/subscription_reconciler_test.go:697 TestDesiredPhysicalSubscriptionsRejectsIncompleteRefsAndNormalizesSymbols
    let desired = [
        // Empty reference is dropped entirely.
        InstrumentRef {
            channel: String::new(),
            market: String::new(),
            symbol: String::new(),
            interval: None,
        },
        InstrumentRef {
            channel: "ORDER_BOOK".to_owned(),
            market: "US".to_owned(),
            symbol: "NVDA".to_owned(),
            interval: None,
        },
        // KLINE without an interval is dropped (Go `continue`).
        InstrumentRef {
            channel: "KLINE".to_owned(),
            market: "US".to_owned(),
            symbol: "NVDA".to_owned(),
            interval: None,
        },
        // Market is inferred from the qualified symbol.
        InstrumentRef {
            channel: "SNAPSHOT".to_owned(),
            market: String::new(),
            symbol: "us.nvda".to_owned(),
            interval: None,
        },
        // Unsupported channel is dropped.
        InstrumentRef {
            channel: "NEWS".to_owned(),
            market: "US".to_owned(),
            symbol: "TSLA".to_owned(),
            interval: None,
        },
    ];
    let plan = desired_subscriptions(&desired);
    assert_eq!(plan.logical_count, 2);
    let keys: Vec<_> = plan.physical.iter().map(|s| s.key.as_str()).collect();
    assert_eq!(keys, vec!["BASIC:US.NVDA", "ORDER_BOOK:US.NVDA"]);
    let basic = plan
        .physical
        .iter()
        .find(|s| s.key == "BASIC:US.NVDA")
        .expect("basic");
    assert_eq!(basic.instrument_id, "US.NVDA");
    let order_book = plan
        .physical
        .iter()
        .find(|s| s.key == "ORDER_BOOK:US.NVDA")
        .expect("order book");
    assert_eq!(order_book.kind, SubscriptionKind::OrderBook);
    assert_eq!(order_book.interval, None);

    // An unqualified symbol cannot be normalized and is rejected rather than
    // silently subscribed with an empty market.
    let unqualified = InstrumentRef {
        channel: "SNAPSHOT".to_owned(),
        market: String::new(),
        symbol: "bad".to_owned(),
        interval: None,
    };
    assert!(unqualified.clone().normalize().is_err());
    assert!(desired_subscriptions(&[unqualified]).physical.is_empty());
}

#[test]
fn basic_quote_requests_extract_only_the_basic_channel_like_go() {
    // Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:78
    // TestStreamConnectionAndSubscriptionBoundaries.
    //
    // Go calls `basicQotRequestsFromSubscriptions` over three subscriptions:
    // Fixture entries: {KLine, "BAD"}, {MarketTrade, "HK.00700"},
    // {BookTicker, "hk.00700"}.
    // It expects exactly **one** request (the MarketTrade one) with no error:
    // the KLine entry carries an unparsable symbol and the BookTicker entry
    // belongs to the separate order-book family, so neither contributes a
    // BasicQot request.
    let desired = [
        // Go's fixture carries no market for this entry, so the symbol has no
        // resolvable `MARKET.CODE` pair and the reference cannot normalize.
        InstrumentRef {
            channel: "KLINE".to_owned(),
            market: String::new(),
            symbol: "BAD".to_owned(),
            interval: Some("1m".to_owned()),
        },
        InstrumentRef {
            channel: "TICK".to_owned(),
            market: "HK".to_owned(),
            symbol: "HK.00700".to_owned(),
            interval: None,
        },
        InstrumentRef {
            channel: "ORDER_BOOK".to_owned(),
            market: "HK".to_owned(),
            symbol: "hk.00700".to_owned(),
            interval: None,
        },
    ];
    let plan = desired_subscriptions(&desired);
    let basic = plan
        .physical
        .iter()
        .filter(|subscription| subscription.kind == SubscriptionKind::Basic)
        .collect::<Vec<_>>();
    assert_eq!(
        basic.len(),
        1,
        "only the MarketTrade channel contributes a BasicQot request: {plan:?}"
    );
    assert_eq!(basic[0].instrument_id, "HK.00700");

    // The unparsable KLine entry is dropped rather than encoded with an empty
    // market, and the order-book entry stays in its own family, so the two
    // HK.00700 spellings produce exactly one BasicQot request plus one
    // order-book subscription.
    assert_eq!(plan.physical.len(), 2, "{plan:?}");
    let orders = plan
        .physical
        .iter()
        .filter(|subscription| subscription.kind == SubscriptionKind::OrderBook)
        .count();
    assert_eq!(orders, 1);
    assert!(
        !plan
            .physical
            .iter()
            .any(|subscription| subscription.instrument_id.contains("BAD")),
        "the unparsable symbol must not reach any request: {plan:?}"
    );

    // A list whose only entry is unparsable produces no BasicQot request at
    // all, which is the state Go's `connectOpenDBasicQot` rejects.
    let invalid_only = [InstrumentRef {
        channel: "TICK".to_owned(),
        market: String::new(),
        symbol: "BAD".to_owned(),
        interval: None,
    }];
    let plan = desired_subscriptions(&invalid_only);
    assert_eq!(plan.logical_count, 0);
    assert!(
        plan.physical
            .iter()
            .all(|subscription| subscription.kind != SubscriptionKind::Basic),
        "an unparsable symbol must never reach a BasicQot request"
    );

    // The unparsable symbol is rejected at normalization time, so the failure
    // is observable instead of silently subscribing an empty market.
    assert!(invalid_only[0].clone().normalize().is_err());
}

#[test]
fn provider_switch_defers_physical_release_until_opend_eligible() {
    // Parity: go:452dea11:internal/integration/futu/subscription_reconciler_test.go:394 TestSubscriptionReconcilerProviderSwitchDefersPhysicalReleaseUntilOpenDEligible
    let desired = [
        reference("KLINE", Some("1m")),
        InstrumentRef {
            channel: "ORDER_BOOK".to_owned(),
            market: "US".to_owned(),
            symbol: "MSFT".to_owned(),
            interval: None,
        },
    ];
    let mut reconciler = SubscriptionReconciler::new(60_000);
    let subscribe = reconciler.actions(&desired, 0, 1);
    let keys: Vec<_> = subscribe
        .iter()
        .map(|action| match action {
            ReconcileAction::Subscribe { subscription } => subscription.key.as_str(),
            other => panic!("expected subscribe, got {other:?}"),
        })
        .collect();
    assert_eq!(
        keys,
        vec!["BASIC:US.AAPL", "KLINE:US.AAPL:1m", "ORDER_BOOK:US.MSFT"]
    );
    for action in &subscribe {
        reconciler.record_success(action, 0, 1);
    }

    // Deactivating Futu (desired -> empty) before the retention window must not
    // release the physical subscriptions: a provider switch is reversible until
    // OpenD's minimum subscription age elapses.
    assert!(reconciler.actions(&[], 30_000, 1).is_empty());
    let snapshot = reconciler.physical_snapshot(&[], 1, None);
    assert_eq!(snapshot.desired_count, 0);
    assert_eq!(snapshot.own_active_count, 3);
    assert_eq!(snapshot.pending_release_count, 3);
    // The web contract (`marketDataContract.ts`) only accepts
    // `pending_unsubscribe`; any other spelling is dropped by the UI mapper.
    assert!(
        snapshot
            .entries
            .iter()
            .all(|entry| entry.broker_state == "pending_unsubscribe"),
        "entries = {:?}",
        snapshot.entries
    );

    // Reactivating Futu inside the window reuses every pending subscription and
    // produces no duplicate subscribe.
    assert!(reconciler.actions(&desired, 30_000, 1).is_empty());
    assert_eq!(
        reconciler
            .physical_snapshot(&desired, 1, None)
            .pending_release_count,
        0
    );

    // Deactivating again and waiting past the minimum age releases all three.
    assert!(reconciler.actions(&[], 30_000, 1).is_empty());
    let release = reconciler.actions(&[], 90_000, 1);
    let released: Vec<_> = release
        .iter()
        .map(|action| match action {
            ReconcileAction::Unsubscribe { subscription } => subscription.key.as_str(),
            other => panic!("expected unsubscribe, got {other:?}"),
        })
        .collect();
    assert_eq!(
        released,
        vec!["BASIC:US.AAPL", "KLINE:US.AAPL:1m", "ORDER_BOOK:US.MSFT"]
    );
    for action in &release {
        reconciler.record_success(action, 90_000, 1);
    }
    let snapshot = reconciler.physical_snapshot(&[], 1, None);
    assert_eq!(snapshot.own_active_count, 0);
    assert_eq!(snapshot.pending_release_count, 0);
    assert!(snapshot.entries.is_empty());
}

#[test]
fn pending_provider_cleanup_drops_closed_connection_ownership() {
    // Parity: go:452dea11:internal/integration/futu/subscription_reconciler_test.go:587 TestSubscriptionReconcilerPendingProviderCleanupDropsClosedConnectionOwnership
    let desired = [
        reference("KLINE", Some("1m")),
        InstrumentRef {
            channel: "ORDER_BOOK".to_owned(),
            market: "US".to_owned(),
            symbol: "MSFT".to_owned(),
            interval: None,
        },
    ];
    let mut reconciler = SubscriptionReconciler::new(60_000);
    for action in reconciler.actions(&desired, 0, 1) {
        reconciler.record_success(&action, 0, 1);
    }

    // Provider cleanup is pending: demand is gone but the physical
    // subscriptions are still inside the retention window.
    assert!(reconciler.actions(&[], 0, 1).is_empty());
    assert_eq!(
        reconciler
            .physical_snapshot(&[], 1, None)
            .pending_release_count,
        3
    );

    // The OpenD connection is replaced (generation 2) while cleanup is still
    // pending. Ownership of the old subscriptions belonged to generation 1, so
    // releasing them through the replacement connection would unsubscribe a
    // subscription this connection never established. Go drops the records
    // instead; the snapshot must report a clean slate for the new generation.
    let replayed = reconciler.replay_actions(&[], 2);
    assert!(
        replayed.is_empty(),
        "no demand remains, so the replacement connection replays nothing"
    );
    let snapshot = reconciler.physical_snapshot(&[], 2, Some(2));
    assert_eq!(snapshot.connection_generation, Some(2));
    assert_eq!(snapshot.own_active_count, 0);
    assert_eq!(snapshot.pending_release_count, 0);
    assert!(snapshot.entries.is_empty());
    assert!(reconciler.actions(&[], 61_000, 2).is_empty());
}

#[test]
fn connection_replacement_clears_quota_ownership_and_reset_is_idempotent() {
    // Parity: go:452dea11:internal/integration/futu/subscription_reconciler_test.go:623 TestSubscriptionReconcilerHandlesQuotaExchangeReplacementResetAndNilBoundaries
    let desired = [reference("SNAPSHOT", None)];
    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let mut lifecycle = OpenDSubscriptionLifecycle::new(Arc::clone(&recorder), 60_000);
    for action in lifecycle.reconcile_demand(&desired, 0) {
        assert!(lifecycle.record_subscription_success(&action, 0, lifecycle.generation()));
    }
    lifecycle.set_quota(Some(1), Some(99), Some(1), 5_000, None);
    let snapshot = lifecycle.physical_snapshot();
    assert_eq!(snapshot.total_used_quota, Some(1));
    assert_eq!(snapshot.checked_at.as_deref(), Some("1970-01-01T00:00:05Z"));
    assert_eq!(snapshot.last_error, None);

    // Go `resetConnectionStateLocked`: quota diagnostics belong to the
    // connection that answered Qot_GetSubInfo, so replacing the connection
    // drops the cached totals and the recorded check time instead of
    // advertising a dead connection's quota to the UI.
    let replayed = lifecycle.reconfigure_for_reconnect(&desired);
    assert!(
        !replayed.is_empty(),
        "the replacement connection must replay demand"
    );
    let snapshot = lifecycle.physical_snapshot();
    assert_eq!(snapshot.total_used_quota, None);
    assert_eq!(snapshot.remain_quota, None);
    assert_eq!(snapshot.own_used_quota, None);
    assert_eq!(snapshot.checked_at, None);
    assert_eq!(snapshot.last_error, None);
    assert_eq!(snapshot.own_active_count, 0);

    // A quota failure is diagnostic only: it is recorded for the UI and never
    // turns into a reconcile error.
    lifecycle.set_quota(
        None,
        None,
        None,
        6_000,
        Some("quota unavailable".to_owned()),
    );
    let snapshot = lifecycle.physical_snapshot();
    assert_eq!(snapshot.last_error.as_deref(), Some("quota unavailable"));
    assert_eq!(snapshot.checked_at.as_deref(), Some("1970-01-01T00:00:06Z"));

    // Closing is idempotent and a closed lifecycle ignores further work.
    assert!(lifecycle.close());
    assert!(!lifecycle.close());
    assert!(lifecycle.reconcile_demand(&desired, 7_000).is_empty());
    assert_eq!(lifecycle.physical_snapshot().own_active_count, 0);
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

#[test]
fn order_book_connect_boundaries_reject_empty_invalid_and_unavailable_demand() {
    // Parity: go:452dea11:pkg/futu/stream_connection_quote_boundaries_test.go:292
    // TestOrderBookStreamConnectionBoundaries.
    //
    // Go's `connectOpenDOrderBook` walks four branches: an empty subscription
    // list fails with "no order book subscriptions", an unparsable symbol fails
    // while extracting requests, a valid HK order book subscribes with the SF
    // detail flag and accepts a push, and an unreachable OpenD fails the
    // session. Rust spreads those owners over the subscription plan, the
    // lifecycle and the coordinator, so this test pins the decision points
    // together; the HK detail flag and the push envelope keep their own
    // function-level evidence.
    let hk_lease = InstrumentRef {
        channel: "ORDER_BOOK".to_owned(),
        market: "HK".to_owned(),
        symbol: "00700".to_owned(),
        interval: None,
    };

    // Empty demand: no order-book request exists at all, which is the state
    // Go rejects with "no order book subscriptions".
    let recorder = Arc::new(jftrade_marketdata::MarketDataRuntimeRecorder::default());
    let mut lifecycle = OpenDSubscriptionLifecycle::new(recorder, 0);
    assert!(
        lifecycle.reconcile_demand(&[], 0).is_empty(),
        "empty demand must not produce an order-book connect"
    );
    let empty = desired_subscriptions(&[]);
    assert!(empty.physical.is_empty(), "{empty:?}");

    // Unparsable symbol: dropped instead of being encoded with an empty market.
    let invalid = desired_subscriptions(&[InstrumentRef {
        channel: "ORDER_BOOK".to_owned(),
        market: String::new(),
        symbol: "BAD".to_owned(),
        interval: None,
    }]);
    assert!(
        invalid.physical.is_empty(),
        "an unparsable order-book symbol must be dropped: {invalid:?}"
    );

    // Valid HK order book: the plan yields exactly the order-book family for
    // that instrument, and the lifecycle only asks OpenD after demand exists.
    let valid = desired_subscriptions(std::slice::from_ref(&hk_lease));
    assert_eq!(valid.physical.len(), 1, "{valid:?}");
    assert_eq!(valid.physical[0].kind, SubscriptionKind::OrderBook);
    assert_eq!(valid.physical[0].instrument_id, "HK.00700");
    let actions = lifecycle.reconcile_demand(std::slice::from_ref(&hk_lease), 0);
    assert_eq!(actions.len(), 1, "{actions:?}");
    assert_eq!(actions[0].key(), "ORDER_BOOK:HK.00700");

    // Unreachable OpenD: the connect must fail closed instead of handing out a
    // half-open order-book session.
    let unavailable = crate::OpenDSessionCoordinator::connect(
        crate::OpenDTcpProbeConfig::new(
            "127.0.0.1:1".parse().expect("addr"),
            std::time::Duration::from_millis(30),
        ),
        Arc::new(jftrade_marketdata::MarketDataRuntimeRecorder::default()),
        vec![hk_lease],
        0,
    );
    assert!(
        unavailable.is_err(),
        "an unavailable OpenD endpoint must not yield an order-book session"
    );
}

#[test]
fn order_book_registry_marks_stay_independent_and_reset_clears_them() {
    // Parity: go:452dea11:pkg/futu/exchange_orderbook_test.go:99
    // TestSubscriptionRegistryOrderBook and :129 TestSubscriptionRegistryOrderBookReset.
    //
    // Rust has no mark table; the reconciler record is the equivalent owner and
    // must keep ORDER_BOOK independent from BASIC for the same instrument, then
    // drop both when the connection is replaced.
    let mut reconciler = SubscriptionReconciler::new(0);
    let order_book = [InstrumentRef {
        channel: "ORDER_BOOK".to_owned(),
        market: "HK".to_owned(),
        symbol: "00700".to_owned(),
        interval: None,
    }];
    let order_book_and_quote = [
        order_book[0].clone(),
        InstrumentRef {
            channel: "SNAPSHOT".to_owned(),
            market: "HK".to_owned(),
            symbol: "00700".to_owned(),
            interval: None,
        },
    ];
    // Recording only the order book leaves the quote family still unestablished.
    let order_book_actions = reconciler.actions(&order_book, 0, 1);
    assert_eq!(order_book_actions.len(), 1);
    reconciler.record_success(&order_book_actions[0], 0, 1);
    assert!(reconciler.actions(&order_book, 0, 1).is_empty());
    // Adding the quote family keeps the order book desired, so the only new
    // action is the BASIC subscribe.
    let basic_actions = reconciler
        .actions(&order_book_and_quote, 0, 1)
        .into_iter()
        .filter(|action| {
            matches!(
                action,
                ReconcileAction::Subscribe { subscription }
                    if subscription.kind == SubscriptionKind::Basic
            )
        })
        .collect::<Vec<_>>();
    assert_eq!(
        basic_actions.len(),
        1,
        "an ORDER_BOOK record must not satisfy the quote family for the same instrument"
    );
    assert!(matches!(
        &basic_actions[0],
        ReconcileAction::Subscribe { subscription } if subscription.kind == SubscriptionKind::Basic
    ));

    // A reset (fresh reconciler / new connection generation) clears the marks:
    // the generic family comparison no longer sees the old order book.
    let mut reset = SubscriptionReconciler::new(0);
    let actions = reset.actions(&order_book, 0, 1);
    assert_eq!(actions.len(), 1);
    assert!(matches!(
        &actions[0],
        ReconcileAction::Subscribe { subscription } if subscription.kind == SubscriptionKind::OrderBook
    ));
}

#[test]
fn order_book_registry_ensure_lazily_yields_an_order_book_plan_entry() {
    // Parity: go:452dea11:pkg/futu/exchange_orderbook_test.go:144
    // TestSubscriptionRegistryOrderBookEnsure. Go's ensure() lazily creates the
    // orderBook/orderBookPush maps so a fresh registry can be marked. Rust has
    // no mark map: a fresh reconciler must derive the ORDER_BOOK plan entry from
    // demand on first use, before any record exists.
    let mut reconciler = SubscriptionReconciler::new(0);
    let desired = [
        InstrumentRef {
            channel: "ORDER_BOOK".to_owned(),
            market: "US".to_owned(),
            symbol: "NVDA".to_owned(),
            interval: None,
        },
        // A KLINE reference without an interval is dropped and must not create
        // an order-book entry.
        InstrumentRef {
            channel: "KLINE".to_owned(),
            market: "US".to_owned(),
            symbol: "NVDA".to_owned(),
            interval: None,
        },
    ];
    let actions = reconciler.actions(&desired, 0, 1);
    assert_eq!(actions.len(), 1);
    let subscription = match &actions[0] {
        ReconcileAction::Subscribe { subscription } => subscription,
        other => panic!("expected subscribe action, got {other:?}"),
    };
    assert_eq!(subscription.key, "ORDER_BOOK:US.NVDA");
    assert_eq!(subscription.kind, SubscriptionKind::OrderBook);
    assert_eq!(subscription.interval, None);
    assert_eq!(subscription.instrument_id, "US.NVDA");
}

#[test]
fn order_book_registry_reset_clears_marks_for_a_replacement_connection() {
    // Parity: go:452dea11:pkg/futu/exchange_orderbook_test.go:129
    // TestSubscriptionRegistryOrderBookReset. Go's reset() drops both the
    // orderBook and orderBookPush marks for every instrument. Rust ties marks to
    // a connection generation: records established under generation 1 must not
    // satisfy generation 2, which forces a fresh subscribe.
    let desired = [InstrumentRef {
        channel: "ORDER_BOOK".to_owned(),
        market: "HK".to_owned(),
        symbol: "00700".to_owned(),
        interval: None,
    }];
    let mut reconciler = SubscriptionReconciler::new(0);
    let first = reconciler.actions(&desired, 0, 1);
    assert_eq!(first.len(), 1);
    reconciler.record_success(&first[0], 0, 1);
    assert!(reconciler.actions(&desired, 0, 1).is_empty());

    // Generation 2 is the replacement connection: the old mark is not reused.
    let replay = reconciler.actions(&desired, 0, 2);
    assert_eq!(
        replay.len(),
        1,
        "a reset connection must re-subscribe the order book"
    );
    let subscription = match &replay[0] {
        ReconcileAction::Subscribe { subscription } => subscription,
        other => panic!("expected subscribe action, got {other:?}"),
    };
    assert_eq!(subscription.key, "ORDER_BOOK:HK.00700");
    assert_eq!(subscription.kind, SubscriptionKind::OrderBook);
}

#[test]
fn closed_session_generation_invalidates_its_subscriptions_and_requires_replay() {
    // Parity: go:8a78fc78:pkg/futu/subscription_lifecycle_test.go:162
    // TestConnectionGenerationInvalidatesClosedSessionAndItsSubscriptions.
    //
    // Go closes the active client, observes a newer ConnectionGeneration and
    // then requires the stale lease to be re-established. Rust ties every
    // record to a generation: replaying after a reconnect makes the old
    // generation inactive until OpenD acknowledges the replacement session.
    let recorder = Arc::new(jftrade_marketdata::MarketDataRuntimeRecorder::default());
    let mut lifecycle = OpenDSubscriptionLifecycle::new(Arc::clone(&recorder), 0);
    let desired = [reference("SNAPSHOT", None)];
    let first = lifecycle.reconcile_demand(&desired, 0);
    assert_eq!(first.len(), 1);
    let initial_generation = lifecycle.generation();
    assert!(lifecycle.record_subscription_success(&first[0], 0, initial_generation));
    assert_eq!(
        lifecycle.active_basic_instruments(),
        vec!["US.AAPL".to_owned()]
    );

    // The peer closes the session: replay actions are fenced to a strictly
    // newer generation and the old subscription stops being active.
    let replay = lifecycle.reconfigure_for_reconnect(&desired);
    let invalidated_generation = lifecycle.generation();
    assert!(
        invalidated_generation > initial_generation,
        "generation did not advance after session close: \
         {initial_generation} -> {invalidated_generation}"
    );
    assert_eq!(replay.len(), 1);
    assert!(
        lifecycle.active_basic_instruments().is_empty(),
        "a closed session must not retain a stale lease"
    );
    // Stale acknowledgements from the old generation are rejected outright.
    assert!(!lifecycle.record_subscription_success(&first[0], 1, initial_generation));

    // Re-subscribing on the replacement session makes it active again.
    assert!(lifecycle.record_subscription_success(&replay[0], 1, invalidated_generation));
    assert_eq!(
        lifecycle.active_basic_instruments(),
        vec!["US.AAPL".to_owned()]
    );
}

#[test]
fn failed_or_replayed_subscriptions_are_not_active_until_opend_confirms() {
    // Parity: go:8a78fc78:pkg/futu/subscription_lifecycle_test.go:80
    // TestExchangeSubscriptionCacheUpdatesOnlyAfterOpenDConfirmation.
    //
    // Go returns the OpenD rejection, keeps the subscription out of the cache
    // and lets the caller retry. Rust records the failure, keeps the record
    // inactive, and only flips it active once OpenD acknowledges the retry.
    let desired = [reference("KLINE", Some("5m"))];
    let mut reconciler = SubscriptionReconciler::new(0);
    let actions = reconciler.actions(&desired, 0, 1);
    assert_eq!(actions.len(), 2, "KLINE also owns its BASIC subscription");

    // The BASIC companion subscription succeeds on the first attempt so the
    // retry assertions below isolate the rejected K-line record.
    let basic = actions
        .iter()
        .find(|action| {
            matches!(
                action,
                ReconcileAction::Subscribe { subscription }
                    if subscription.kind == SubscriptionKind::Basic
            )
        })
        .expect("basic subscription");
    reconciler.record_success(basic, 0, 1);

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
        .expect("kline subscription");

    // First attempt fails with a quota rejection: nothing becomes active and
    // the bounded retry window is armed.
    assert_eq!(
        reconciler.record_failure(&kline, 0, 1, Some("quota exceeded".to_owned())),
        5_000
    );
    assert!(
        reconciler
            .active_instruments(SubscriptionKind::Kline, 1)
            .is_empty()
    );
    let retry = reconciler.actions(&desired, 5_000, 1);
    assert_eq!(retry.len(), 1, "retry after the failure window");

    // Only the OpenD-confirmed retry marks the record active.
    reconciler.record_success(&retry[0], 5_000, 1);
    assert_eq!(
        reconciler.active_instruments(SubscriptionKind::Kline, 1),
        vec!["US.AAPL".to_owned()]
    );

    // Releasing demand produces the exact K-line unsubscribe plus the BASIC
    // companion release; a rejected K-line release keeps its record and
    // retries.
    let release = reconciler.actions(&[], 6_000, 1);
    assert_eq!(release.len(), 2, "K-line and its BASIC companion");
    // The BASIC companion releases cleanly; only the rejected K-line
    // unsubscribe stays behind on its bounded retry window.
    for action in &release {
        if matches!(
            action,
            ReconcileAction::Unsubscribe { subscription }
                if subscription.kind == SubscriptionKind::Basic
        ) {
            reconciler.record_success(action, 6_000, 1);
        }
    }
    let subscription = release
        .iter()
        .find_map(|action| match action {
            ReconcileAction::Unsubscribe { subscription }
                if subscription.kind == SubscriptionKind::Kline =>
            {
                Some(subscription.clone())
            }
            _ => None,
        })
        .expect("kline unsubscribe");
    assert_eq!(
        reconciler.record_unsubscribe_failure(&subscription, 6_000, 1, Some("busy".to_owned())),
        5_000
    );
    assert_eq!(
        reconciler.active_instruments(SubscriptionKind::Kline, 1),
        vec!["US.AAPL".to_owned()],
        "a rejected release must not drop the active record"
    );
    assert!(reconciler.actions(&[], 10_999, 1).is_empty());
    assert!(matches!(
        reconciler
            .actions(&[], 11_000, 1)
            .iter()
            .filter(|action| matches!(
                action,
                ReconcileAction::Unsubscribe { subscription }
                    if subscription.kind == SubscriptionKind::Kline
            ))
            .count(),
        1
    ));
}

#[test]
fn exchange_invalidate_client_clears_ready_state_and_every_subscription_kind() {
    // Parity: go:452dea11:pkg/futu/exchange_business_boundary_test.go:326
    // TestExchangeInvalidateClientClearsReadyStateAndSubscriptions. Go drops
    // `ready` and every subscription set (BasicQot, BasicQotPush, KLine,
    // OrderBook, OrderBookPush) together, so no surface can keep rendering a
    // quote that the invalidated client no longer owns.
    //
    // Rust has no `invalidateClient`; the equivalent owner is
    // `OpenDSubscriptionLifecycle::close`, which is what the coordinator calls
    // when the physical client is invalidated (peer close or explicit close).
    let recorder = Arc::new(MarketDataRuntimeRecorder::default());
    let mut lifecycle = OpenDSubscriptionLifecycle::new(Arc::clone(&recorder), 60_000);
    let desired = [
        reference("SNAPSHOT", None),
        reference("KLINE", Some("1m")),
        reference("ORDER_BOOK", None),
    ];
    for action in lifecycle.reconcile_demand(&desired, 0) {
        assert!(lifecycle.record_subscription_success(&action, 0, lifecycle.generation()));
    }
    // The invalidated client still owns live subscriptions, in every kind.
    let snapshot = lifecycle.physical_snapshot();
    assert!(
        !snapshot.entries.is_empty(),
        "precondition: subscriptions exist"
    );
    assert!(snapshot.own_active_count > 0);
    assert!(
        !lifecycle.active_basic_instruments().is_empty(),
        "precondition: a Basic subscription is active"
    );
    assert!(
        snapshot
            .entries
            .iter()
            .any(|entry| entry.kind.to_lowercase().contains("kline")),
        "precondition: a KLine subscription is active"
    );
    assert!(
        snapshot
            .entries
            .iter()
            .any(|entry| entry.kind.to_lowercase().contains("order")),
        "precondition: an order-book subscription is active"
    );

    assert!(lifecycle.close(), "first invalidate reports a change");

    // After invalidation nothing is ready and no kind survives.
    let snapshot = lifecycle.physical_snapshot();
    assert!(snapshot.entries.is_empty(), "entries = {snapshot:?}");
    assert_eq!(snapshot.own_active_count, 0);
    assert_eq!(snapshot.fallback_count, 0);
    assert_eq!(snapshot.pending_release_count, 0);
    assert!(lifecycle.active_basic_instruments().is_empty());
    // A closed lifecycle also refuses new work and stays idempotent, matching
    // Go's repeated invalidateClient() being a no-op.
    assert!(!lifecycle.close());
    assert!(lifecycle.reconcile_demand(&desired, 1_000).is_empty());
    assert!(lifecycle.physical_snapshot().entries.is_empty());
}
