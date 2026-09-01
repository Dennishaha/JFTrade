use std::str::FromStr;
use std::sync::{Arc, Mutex};

use jftrade_calendar::{
    CalendarCancellationToken, CalendarManager, CalendarManagerSettings, CalendarSessionWindow,
    CalendarSnapshot, CalendarSourceDescriptor, CalendarSourceError, CalendarSourcePolicy,
    CalendarSourcePort, CalendarSourceRegistry, TradingDaySchedule,
};
use jftrade_kernel::WireTimestamp;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

#[derive(Clone, Debug, Eq, PartialEq)]
struct FetchCall {
    market: String,
    from: WireTimestamp,
    to: WireTimestamp,
}

#[derive(Clone)]
struct RecordingSource {
    calls: Arc<Mutex<Vec<FetchCall>>>,
}

impl RecordingSource {
    fn new() -> (Self, Arc<Mutex<Vec<FetchCall>>>) {
        let calls = Arc::new(Mutex::new(Vec::new()));
        (
            Self {
                calls: Arc::clone(&calls),
            },
            calls,
        )
    }
}

impl CalendarSourcePort for RecordingSource {
    fn descriptor(&self) -> CalendarSourceDescriptor {
        CalendarSourceDescriptor {
            id: "recording".to_owned(),
            kind: "fixture".to_owned(),
            authority: "fixture".to_owned(),
            markets: vec!["US".to_owned(), "HK".to_owned(), "CN".to_owned()],
        }
    }

    fn fetch(
        &self,
        market: &str,
        from: WireTimestamp,
        to: WireTimestamp,
        _cancellation: &CalendarCancellationToken,
    ) -> Result<CalendarSnapshot, CalendarSourceError> {
        self.calls.lock().expect("recording calls").push(FetchCall {
            market: market.to_owned(),
            from,
            to,
        });
        Ok(CalendarSnapshot {
            market_code: market.to_owned(),
            source_id: "recording".to_owned(),
            from,
            to,
            schedules: vec![TradingDaySchedule {
                market_code: market.to_owned(),
                date: from,
                status: "open".to_owned(),
                sessions: vec![CalendarSessionWindow {
                    kind: "regular".to_owned(),
                    start_minute: 0,
                    end_minute: 1,
                }],
                reason: String::new(),
                source_id: "recording".to_owned(),
                observed: false,
                updated_at: None,
            }],
            fetched_at: from,
            valid_until: to,
            checksum: market.to_owned(),
        })
    }
}

fn timestamp(value: &str) -> OffsetDateTime {
    OffsetDateTime::parse(value, &Rfc3339).expect("valid timestamp")
}

fn wire(value: &str) -> WireTimestamp {
    WireTimestamp::from_str(value).expect("valid wire timestamp")
}

fn settings() -> CalendarManagerSettings {
    CalendarManagerSettings {
        refresh_interval_hours: 24,
        source_policies: ["US", "HK", "CN"]
            .into_iter()
            .map(|market| CalendarSourcePolicy {
                market: market.to_owned(),
                preferred_source_ids: vec!["recording".to_owned()],
                enabled_source_ids: vec!["recording".to_owned()],
                fallback_to_builtin: false,
                ..CalendarSourcePolicy::default()
            })
            .collect(),
        ..CalendarManagerSettings::default()
    }
}

fn manager(source: RecordingSource, now: Arc<Mutex<OffsetDateTime>>) -> CalendarManager {
    let mut registry = CalendarSourceRegistry::default();
    registry
        .register(Arc::new(source))
        .expect("register source");
    CalendarManager::with_clock(
        registry,
        None,
        settings(),
        Arc::new(move || *now.lock().expect("clock")),
    )
    .expect("create manager")
}

fn assert_call(call: &FetchCall, market: &str, from: &str, to: &str) {
    assert_eq!(
        call,
        &FetchCall {
            market: market.to_owned(),
            from: wire(from),
            to: wire(to),
        }
    );
}

#[test]
fn refresh_uses_market_local_boundaries_and_not_current_dst_offset() {
    let (source, calls) = RecordingSource::new();
    let now = Arc::new(Mutex::new(timestamp("2026-07-01T12:00:00-04:00")));
    let manager = manager(source, now);
    manager.start().expect("start manager");

    assert_eq!(manager.refresh_market("US").expect("refresh US").updated, 1);
    assert_eq!(manager.refresh_market("HK").expect("refresh HK").updated, 1);
    assert_eq!(manager.refresh_market("CN").expect("refresh CN").updated, 1);

    let calls = calls.lock().expect("recording calls");
    assert_eq!(calls.len(), 3);
    assert_call(
        &calls[0],
        "US",
        "2026-01-01T00:00:00-05:00",
        "2027-12-31T23:59:59-05:00",
    );
    assert_call(
        &calls[1],
        "HK",
        "2026-01-01T00:00:00+08:00",
        "2027-12-31T23:59:59+08:00",
    );
    assert_call(
        &calls[2],
        "CN",
        "2026-01-01T00:00:00+08:00",
        "2027-12-31T23:59:59+08:00",
    );
    drop(calls);
    manager.close().expect("close manager");
}

#[test]
fn probe_uses_market_local_year_when_us_crosses_utc_new_year() {
    let (source, calls) = RecordingSource::new();
    let now = Arc::new(Mutex::new(timestamp("2026-01-01T04:30:00Z")));
    let manager = manager(source, Arc::clone(&now));
    manager.start().expect("start manager");

    let result = manager.probe_market("US").expect("probe US");
    assert_eq!((result.healthy, result.failures), (1, 0));
    assert_call(
        &calls.lock().expect("recording calls")[0],
        "US",
        "2025-01-01T00:00:00-05:00",
        "2026-12-31T23:59:59-05:00",
    );

    calls.lock().expect("recording calls").clear();
    *now.lock().expect("clock") = timestamp("2026-01-01T05:30:00Z");
    let result = manager.probe_market("US").expect("probe US after midnight");
    assert_eq!((result.healthy, result.failures), (1, 0));
    assert_call(
        &calls.lock().expect("recording calls")[0],
        "US",
        "2026-01-01T00:00:00-05:00",
        "2027-12-31T23:59:59-05:00",
    );
    manager.close().expect("close manager");
}
