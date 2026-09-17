use super::*;
use crate::trade_proto::notify::QotRight;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

fn at(seconds: u64) -> SystemTime {
    UNIX_EPOCH + Duration::from_secs(seconds)
}

fn rights() -> QuoteRightSnapshot {
    QuoteRightSnapshot::from_notification(&QotRight {
        hk_qot_right: 3,
        us_qot_right: 3,
        cn_qot_right: 3,
        ..Default::default()
    })
}

#[test]
fn stale_connect_and_quote_right_notifications_cannot_replace_newer_generation() {
    // Parity: go:pkg/futu/adapter_research_normalization_boundaries_test.go:306
    // (TestQuoteRightsRefreshStateEdges, stale generation fencing half).
    let mut state = QuoteRightsState::new();
    state.store_connect_status(3, true, true, at(1));
    state.store_connect_status(2, false, false, at(2));
    assert_eq!(
        state
            .connect_status_for_generation(3)
            .expect("generation 3")
            .generation,
        3
    );
    state.store_quote_right(3, rights(), at(3));
    assert!(!state.store_quote_right(2, rights(), at(4)));
    assert_eq!(state.state_for_generation(3, 3), QuoteRightState::Available);
    assert_eq!(
        state.state_for_generation(2, 3),
        QuoteRightState::Unverified
    );
}

#[test]
fn notification_resolution_wins_over_inflight_query_result() {
    // Parity: go:pkg/futu/adapter_research_normalization_boundaries_test.go:306
    // (notification-won store and stale store error).
    let mut state = QuoteRightsState::new();
    state.remember_failure(3, "query failed", at(10));
    let revision = state.revision();
    state.store_quote_right(3, rights(), at(11));
    assert!(state.cached_failure_due(3, at(11)).is_none());

    let accepted = state.store_query_result(3, revision, rights(), at(12));
    assert!(
        !accepted,
        "query result must not overwrite the notification"
    );
    assert_eq!(state.revision(), revision + 1);

    // An older generation result after a newer snapshot must be rejected.
    state.store_quote_right(4, rights(), at(13));
    assert!(!state.store_query_result(3, revision, rights(), at(14)));
    assert_eq!(state.state_for_generation(4, 3), QuoteRightState::Available);
}

#[test]
fn fresh_generation_query_result_is_accepted() {
    // Parity: go:pkg/futu/adapter_research_normalization_boundaries_test.go:306
    // (fresh quote-right store).
    let mut state = QuoteRightsState::new();
    state.remember_failure(1, "cached", at(1));
    let revision = state.revision();
    assert!(state.store_query_result(1, revision, rights(), at(2)));
    assert!(state.cached_failure_due(1, at(2)).is_none());
    assert_eq!(state.state_for_generation(1, 3), QuoteRightState::Available);
}

#[test]
fn fetch_failure_outcomes_follow_generation_and_notification_state() {
    // Parity: go:pkg/futu/adapter_research_normalization_boundaries_test.go:368
    // (TestQuoteRightsFailureAndExchangeGenerationEdges failure half).
    let mut state = QuoteRightsState::new();
    state.store_quote_right(7, rights(), at(1));
    assert_eq!(
        state.handle_fetch_failure(7, 7, "late query failure", at(2)),
        QuoteRightsFetchOutcome::ResolvedByNotification
    );
    assert_eq!(
        state.handle_fetch_failure(8, 7, "stale query failure", at(2)),
        QuoteRightsFetchOutcome::RefreshRequired
    );

    let mut fresh = QuoteRightsState::new();
    assert_eq!(
        fresh.handle_fetch_failure(7, 7, "query failed", at(2)),
        QuoteRightsFetchOutcome::Failed("query failed".to_owned())
    );
    assert_eq!(
        fresh.cached_failure_due(7, at(2)),
        Some("query failed"),
        "failure cache must be retryable for the same generation"
    );
    assert!(
        fresh
            .cached_failure_due(7, at(2) + QUOTE_RIGHTS_FAILURE_RETRY_INTERVAL)
            .is_none()
    );

    fresh.store_quote_right(7, rights(), at(3));
    assert!(fresh.cached_failure_due(7, at(3)).is_none());
}

#[test]
fn zero_generation_is_never_verified() {
    // Parity: go:pkg/futu/adapter_research_normalization_boundaries_test.go:368
    // (unconnected query generation=0).
    let mut state = QuoteRightsState::new();
    state.store_quote_right(0, rights(), at(1));
    assert_eq!(
        state.state_for_generation(0, 3),
        QuoteRightState::Unverified
    );
}

#[test]
fn product_states_match_go_quote_right_table() {
    // Parity: go:pkg/futu/adapter_capabilities.go evaluateQuoteCapability.
    let mut state = QuoteRightsState::new();
    state.store_quote_right(5, rights(), at(1));
    for right in [2, 3, 4, 6] {
        assert_eq!(
            state.state_for_generation(5, right),
            QuoteRightState::Available
        );
    }
    assert_eq!(
        state.state_for_generation(5, 1),
        QuoteRightState::PollingOnly
    );
    assert_eq!(state.state_for_generation(5, 5), QuoteRightState::Denied);
    assert_eq!(state.state_for_generation(5, 0), QuoteRightState::Unknown);
    assert_eq!(state.state_for_generation(5, 99), QuoteRightState::Unknown);
}
