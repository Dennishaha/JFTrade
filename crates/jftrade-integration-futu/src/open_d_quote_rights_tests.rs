//! Parity coverage for Go `pkg/futu/adapter_capability_runtime_test.go`.
//!
//! The pure state machine (stale notification fencing, notification-wins
//! revision and failure outcomes) is covered in `quote_rights_tests.rs`. This
//! file covers the acquisition owner Go implements in
//! `ensureQuoteRights`/`refreshQuoteRights`: one `GetUserInfo` per connection,
//! retryable failure caching, refresh after reconnect, and never inferring
//! detailed entitlements from the legacy `GetUserInfo` fields.

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::{Arc, Barrier};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use crate::trade_proto::get_user_info::S2c;
use crate::{
    OpenDQuoteRightsOwner, QUOTE_RIGHTS_FAILURE_RETRY_INTERVAL, QuoteRightField,
    QuoteRightSnapshot, QuoteRightState, QuoteRightsError,
};

fn at(seconds: u64) -> SystemTime {
    UNIX_EPOCH + Duration::from_secs(seconds)
}

fn full_rights(value: i32) -> QuoteRightSnapshot {
    QuoteRightSnapshot {
        hk_qot_right: value,
        us_qot_right: value,
        cn_qot_right: value,
        hk_option_qot_right: Some(value),
        has_us_option_qot_right: true,
        us_option_qot_right: Some(value),
        us_index_qot_right: Some(value),
        hk_future_qot_right: Some(value),
        us_future_qot_right: Some(value),
        us_cme_future_qot_right: Some(value),
        us_cbot_future_qot_right: Some(value),
        us_nymex_future_qot_right: Some(value),
        us_comex_future_qot_right: Some(value),
        us_cboe_future_qot_right: Some(value),
        sh_qot_right: Some(value),
        sz_qot_right: Some(value),
        ec_qot_right: Some(value),
    }
}

#[test]
fn entitlement_query_runs_once_per_connection() {
    // Parity: go:pkg/futu/adapter_capability_runtime_test.go:265
    // TestFutuCapabilityLoadsQuoteRightsOncePerConnection. Twenty concurrent
    // capability reads must share one GetUserInfo call for the session.
    let owner = OpenDQuoteRightsOwner::new();
    owner.set_generation(1);
    let calls = Arc::new(AtomicUsize::new(0));
    let barrier = Arc::new(Barrier::new(20));

    std::thread::scope(|scope| {
        for _ in 0..20 {
            let owner = owner.clone();
            let calls = Arc::clone(&calls);
            let barrier = Arc::clone(&barrier);
            scope.spawn(move || {
                barrier.wait();
                let snapshot = owner
                    .ensure(at(10), || {
                        calls.fetch_add(1, Ordering::SeqCst);
                        Ok(full_rights(3))
                    })
                    .expect("entitlement");
                assert_eq!(snapshot.us_qot_right, 3);
            });
        }
    });

    assert_eq!(
        calls.load(Ordering::SeqCst),
        1,
        "GetUserInfo must run once per connection"
    );
}

#[test]
fn entitlement_failure_is_cached_and_refreshed_after_reconnect() {
    // Parity: go:pkg/futu/adapter_capability_runtime_test.go:313
    // TestFutuCapabilityCachesQuoteRightFailuresAndRefreshesAfterReconnect.
    let owner = OpenDQuoteRightsOwner::new();
    owner.set_generation(1);
    let calls = AtomicUsize::new(0);
    for _ in 0..2 {
        let error = owner
            .ensure(at(10), || {
                calls.fetch_add(1, Ordering::SeqCst);
                Err("permission query failed".to_owned())
            })
            .expect_err("failed query");
        assert_eq!(
            error,
            QuoteRightsError::Failed("permission query failed".to_owned())
        );
    }
    assert_eq!(calls.load(Ordering::SeqCst), 1, "failure must be cached");

    // A reconnect bumps the generation; the cached failure must not leak and
    // the new session is queried again.
    owner.set_generation(2);
    let refreshed = owner
        .ensure(at(11), || {
            calls.fetch_add(1, Ordering::SeqCst);
            Ok(full_rights(5))
        })
        .expect("refreshed entitlement");
    assert_eq!(refreshed.us_qot_right, 5);
    assert_eq!(calls.load(Ordering::SeqCst), 2);
    assert_eq!(
        owner.right_value(QuoteRightField::Us),
        Some(5),
        "the refreshed generation owns the entitlement"
    );
}

#[test]
fn stale_snapshot_is_not_reused_for_a_new_generation() {
    // Parity: go:pkg/futu/adapter_capability_runtime_test.go:405
    // TestFutuCapabilityRejectsStaleNotificationsAndFailureWrites
    // (generation half). A snapshot verified for generation 1 must never
    // authorize the replacement session.
    let owner = OpenDQuoteRightsOwner::new();
    owner.set_generation(1);
    owner
        .ensure(at(10), || Ok(full_rights(3)))
        .expect("generation 1 entitlement");
    owner.set_generation(2);
    assert_eq!(
        owner.right_value(QuoteRightField::Us),
        None,
        "a stale generation must stay unverified"
    );

    owner.set_generation(0);
    assert_eq!(
        owner.right_value(QuoteRightField::Us),
        None,
        "a disconnected session must stay unverified"
    );
    assert_eq!(
        owner.ensure(at(12), || Ok(full_rights(3))).unwrap_err(),
        QuoteRightsError::NotConfigured
    );
}

#[test]
fn cached_failure_expires_at_the_retry_interval() {
    // Parity: go:pkg/futu/adapter_capabilities.go `quoteRightsFailureRetryInterval`
    // and `cachedQuoteRightsStatus`.
    let owner = OpenDQuoteRightsOwner::new();
    owner.set_generation(3);
    let calls = AtomicUsize::new(0);
    owner
        .ensure(at(10), || {
            calls.fetch_add(1, Ordering::SeqCst);
            Err("query failed".to_owned())
        })
        .expect_err("first failure");

    let retried = owner.ensure(at(10) + QUOTE_RIGHTS_FAILURE_RETRY_INTERVAL, || {
        calls.fetch_add(1, Ordering::SeqCst);
        Ok(full_rights(3))
    });
    assert!(
        retried.is_ok(),
        "the retry interval must expire the failure"
    );
    assert_eq!(calls.load(Ordering::SeqCst), 2);
}

#[test]
fn user_info_conversion_does_not_infer_detailed_entitlements() {
    // Parity: go:pkg/futu/adapter_capability_runtime_test.go:366
    // TestQuoteRightsFromUserInfoDoesNotInferDetailedRights.
    assert!(
        QuoteRightSnapshot::from_user_info(&S2c::default()).is_none(),
        "empty user info must not produce entitlements"
    );

    let converted = QuoteRightSnapshot::from_user_info(&S2c {
        cn_qot_right: Some(2),
        us_index_qot_right: Some(5),
        has_us_option_qot_right: Some(true),
        ..Default::default()
    })
    .expect("legacy quote rights");
    assert_eq!(converted.cn_qot_right, 2);
    assert_eq!(converted.us_index_qot_right, Some(5));
    assert!(converted.has_us_option_qot_right);
    assert!(
        converted.sh_qot_right.is_none()
            && converted.sz_qot_right.is_none()
            && converted.us_option_qot_right.is_none(),
        "legacy-only fields must not fabricate SH/SZ/US-option entitlements"
    );

    let owner = OpenDQuoteRightsOwner::new();
    owner.set_generation(1);
    owner.store_quote_right(1, converted.clone(), at(1));
    assert_eq!(
        owner.right_value(QuoteRightField::Sh),
        Some(2),
        "SH falls back to the legacy CN field"
    );
    assert_eq!(owner.right_value(QuoteRightField::Sz), Some(2));
    assert_eq!(
        owner.right_value(QuoteRightField::UsIndex),
        Some(5),
        "the legacy US index level is preserved"
    );
    assert_eq!(
        owner.right_value(QuoteRightField::UsOption),
        Some(2),
        "the availability hint does not fabricate a level"
    );
    assert_eq!(
        owner.right_value(QuoteRightField::HkOption),
        Some(0),
        "an unreported option product stays at the unknown zero level"
    );
}

#[test]
fn stale_notifications_and_failure_writes_are_fenced() {
    // Parity: go:pkg/futu/adapter_capability_runtime_test.go:405
    // TestFutuCapabilityRejectsStaleNotificationsAndFailureWrites.
    let owner = OpenDQuoteRightsOwner::new();
    owner.set_generation(2);
    assert!(owner.store_quote_right(2, full_rights(4), at(1)));
    assert!(
        !owner.store_quote_right(1, full_rights(2), at(2)),
        "a stale QotRight push must not replace the current generation"
    );
    assert_eq!(owner.right_value(QuoteRightField::Us), Some(4));

    // A same-generation notification resolves an in-flight query failure, so
    // the capability read reports the notification instead of a query error.
    let resolved = owner
        .ensure(at(3), || Err("stale query error".to_owned()))
        .expect("notification resolves the query failure");
    assert_eq!(resolved.us_qot_right, 4);
    assert!(owner.ensure(at(3), || Ok(full_rights(1))).is_ok());

    // Disconnect fences the cached entitlement without caching a failure for
    // an unrelated generation.
    owner.set_generation(3);
    assert_eq!(owner.right_value(QuoteRightField::Us), None);
    assert_eq!(
        owner.ensure(at(4), || Err("old connection error".to_owned())),
        Err(QuoteRightsError::Failed("old connection error".to_owned()))
    );
}

#[test]
fn notification_keeps_state_fenced_and_revision_observable() {
    // Parity: go:pkg/futu/adapter_capability_runtime_test.go:18
    // TestFutuCapabilityNotificationsAndRuntimeAggregation: a push is cloned
    // into the owner, later mutation of the caller's value cannot change it,
    // and only same-generation pushes move the revision.
    let owner = OpenDQuoteRightsOwner::new();
    owner.set_generation(1);
    let rights = full_rights(2);
    assert!(owner.store_quote_right(1, rights.clone(), at(1)));
    let mut mutated = rights;
    mutated.us_qot_right = 5;
    assert_eq!(mutated.us_qot_right, 5);
    assert_eq!(owner.right_value(QuoteRightField::Us), Some(2));

    owner.store_connect_status(1, false, true, at(1));
    let status = owner
        .connect_status_for_generation(1)
        .expect("connect status");
    assert!(!status.quote_logged_in);
    assert!(status.trade_logged_in);
    assert_eq!(status.generation, 1);

    assert_eq!(owner.right_value(QuoteRightField::Us), Some(2));
    assert!(owner.verified_rights().is_some());
    assert_eq!(
        owner.state().state_for_generation(1, 2),
        QuoteRightState::Available
    );
}
