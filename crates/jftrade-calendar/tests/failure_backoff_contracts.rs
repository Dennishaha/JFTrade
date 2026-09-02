use std::sync::{Arc, Mutex};

use jftrade_calendar::{
    CalendarCancellationToken, CalendarManager, CalendarManagerSettings, CalendarSourceDescriptor,
    CalendarSourceError, CalendarSourcePolicy, CalendarSourcePort, CalendarSourceRegistry,
    ManagerLifecycleState,
};
use jftrade_kernel::WireTimestamp;
use time::{Duration, OffsetDateTime, format_description::well_known::Rfc3339};

struct FailingSource;

impl CalendarSourcePort for FailingSource {
    fn descriptor(&self) -> CalendarSourceDescriptor {
        CalendarSourceDescriptor {
            id: "official".to_owned(),
            kind: "fixture".to_owned(),
            authority: "fixture".to_owned(),
            markets: vec!["US".to_owned()],
        }
    }

    fn fetch(
        &self,
        _market: &str,
        _from: WireTimestamp,
        _to: WireTimestamp,
        _cancellation: &CalendarCancellationToken,
    ) -> Result<jftrade_calendar::CalendarSnapshot, CalendarSourceError> {
        Err(CalendarSourceError::Failed("offline".to_owned()))
    }
}

fn timestamp(value: &str) -> OffsetDateTime {
    OffsetDateTime::parse(value, &Rfc3339).expect("valid fixture timestamp")
}

fn wire(value: OffsetDateTime) -> String {
    WireTimestamp::from_offset_datetime(value).to_string()
}

fn settings() -> CalendarManagerSettings {
    CalendarManagerSettings {
        refresh_interval_hours: 24,
        warmup_markets: vec!["US".to_owned()],
        source_policies: vec![CalendarSourcePolicy {
            market: "US".to_owned(),
            preferred_source_ids: vec!["official".to_owned()],
            enabled_source_ids: vec!["official".to_owned()],
            fallback_to_builtin: true,
            ..CalendarSourcePolicy::default()
        }],
        ..CalendarManagerSettings::default()
    }
}

#[test]
fn source_failure_retry_delay_starts_at_one_hour_and_caps_at_one_day() {
    let now = Arc::new(Mutex::new(timestamp("2026-06-20T08:00:00Z")));
    let mut registry = CalendarSourceRegistry::default();
    registry
        .register(Arc::new(FailingSource))
        .expect("register failing source");
    let manager = CalendarManager::with_clock(
        registry,
        None,
        settings(),
        Arc::new({
            let now = Arc::clone(&now);
            move || *now.lock().expect("fixture clock")
        }),
    )
    .expect("create calendar manager");

    manager.start().expect("start manager");
    assert_eq!(
        manager.lifecycle_state().expect("lifecycle"),
        ManagerLifecycleState::Running
    );

    let first = manager.refresh_market("US").expect("first refresh");
    assert_eq!(
        (first.updated, first.failures, first.skipped_backoff),
        (0, 1, 0)
    );
    let first_retry = manager
        .source_statuses()
        .expect("source statuses")
        .into_iter()
        .find(|status| status.source_id == "official")
        .expect("official status")
        .next_refresh_at
        .expect("first retry");
    assert_eq!(first_retry, wire(timestamp("2026-06-20T09:00:00Z")));

    for _ in 1..30 {
        *now.lock().expect("fixture clock") += Duration::hours(25);
        let result = manager.refresh_market("US").expect("retry refresh");
        assert_eq!(
            (result.updated, result.failures, result.skipped_backoff),
            (0, 1, 0)
        );
    }

    let current = *now.lock().expect("fixture clock");
    let status = manager
        .source_statuses()
        .expect("source statuses")
        .into_iter()
        .find(|status| status.source_id == "official")
        .expect("official status");
    assert_eq!(status.consecutive_failures, 30);
    assert_eq!(status.health_state, "unhealthy");
    assert_eq!(status.last_error, "offline");
    assert_eq!(
        status.next_refresh_at,
        Some(wire(current + Duration::hours(24)))
    );
    manager.close().expect("close manager");
}
