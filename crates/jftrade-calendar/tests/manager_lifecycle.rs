use std::collections::VecDeque;
use std::str::FromStr;
use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
use std::sync::{Arc, Condvar, Mutex};
use std::thread;
use std::time::{Duration as StdDuration, Instant};

use jftrade_calendar::{
    CalendarCancellationToken, CalendarManager, CalendarManagerError, CalendarManagerSettings,
    CalendarManualOverride, CalendarPersistencePort, CalendarRefreshResult,
    CalendarSessionOverride, CalendarSnapshot, CalendarSnapshotLoadResult,
    CalendarSourceDescriptor, CalendarSourceError, CalendarSourcePolicy, CalendarSourcePort,
    CalendarSourceRegistry, ManagerLifecycleState, TradingDaySchedule,
};
use jftrade_kernel::WireTimestamp;
use time::{Duration, OffsetDateTime};

#[derive(Clone)]
struct FixtureSource {
    descriptor: CalendarSourceDescriptor,
    events: Arc<Mutex<Vec<String>>>,
    fetches: Arc<Mutex<VecDeque<Result<CalendarSnapshot, CalendarSourceError>>>>,
    start_error: bool,
    block_until_cancelled: bool,
    fetch_count: Arc<AtomicUsize>,
    fetch_signal: Arc<(Mutex<bool>, Condvar)>,
}

impl FixtureSource {
    fn new(id: &str, events: Arc<Mutex<Vec<String>>>) -> Self {
        Self {
            descriptor: CalendarSourceDescriptor {
                id: id.to_owned(),
                kind: "fixture".to_owned(),
                authority: "tests".to_owned(),
                markets: vec!["US".to_owned()],
            },
            events,
            fetches: Arc::new(Mutex::new(VecDeque::new())),
            start_error: false,
            block_until_cancelled: false,
            fetch_count: Arc::new(AtomicUsize::new(0)),
            fetch_signal: Arc::new((Mutex::new(false), Condvar::new())),
        }
    }

    fn push(&self, result: Result<CalendarSnapshot, CalendarSourceError>) {
        self.fetches
            .lock()
            .expect("fixture fetch queue")
            .push_back(result);
    }

    fn wait_for_fetch(&self) {
        let deadline = Instant::now() + StdDuration::from_secs(3);
        let (lock, signal) = &*self.fetch_signal;
        let mut fetched = lock.lock().expect("fixture fetch signal");
        while !*fetched {
            let remaining = deadline.saturating_duration_since(Instant::now());
            assert!(!remaining.is_zero(), "fixture source was not fetched");
            (fetched, _) = signal
                .wait_timeout(fetched, remaining)
                .expect("wait for fixture fetch");
        }
    }
}

impl CalendarSourcePort for FixtureSource {
    fn descriptor(&self) -> CalendarSourceDescriptor {
        self.descriptor.clone()
    }

    fn start(&self, _cancellation: &CalendarCancellationToken) -> Result<(), CalendarSourceError> {
        self.events
            .lock()
            .expect("fixture events")
            .push(format!("start:{}", self.descriptor.id));
        if self.start_error {
            Err(CalendarSourceError::Failed("startup rejected".to_owned()))
        } else {
            Ok(())
        }
    }

    fn fetch(
        &self,
        _market: &str,
        _from: WireTimestamp,
        _to: WireTimestamp,
        cancellation: &CalendarCancellationToken,
    ) -> Result<CalendarSnapshot, CalendarSourceError> {
        self.fetch_count.fetch_add(1, Ordering::AcqRel);
        let (lock, signal) = &*self.fetch_signal;
        *lock.lock().expect("fixture fetch signal") = true;
        signal.notify_all();
        if self.block_until_cancelled {
            while !cancellation.is_cancelled() {
                thread::sleep(StdDuration::from_millis(5));
            }
            return Err(CalendarSourceError::Cancelled);
        }
        self.fetches
            .lock()
            .expect("fixture fetch queue")
            .pop_front()
            .unwrap_or_else(|| Err(CalendarSourceError::Failed("fixture exhausted".to_owned())))
    }

    fn close(&self) -> Result<(), CalendarSourceError> {
        self.events
            .lock()
            .expect("fixture events")
            .push(format!("close:{}", self.descriptor.id));
        Ok(())
    }
}

struct FixturePersistence {
    loaded: CalendarSnapshotLoadResult,
    fail_save: AtomicBool,
    saved: Mutex<Vec<CalendarSnapshot>>,
}

impl CalendarPersistencePort for FixturePersistence {
    fn load(&self) -> CalendarSnapshotLoadResult {
        self.loaded.clone()
    }

    fn save(&self, snapshot: &CalendarSnapshot) -> Result<(), String> {
        if self.fail_save.load(Ordering::Acquire) {
            return Err("fixture persistence unavailable".to_owned());
        }
        self.saved
            .lock()
            .expect("saved snapshots")
            .push(snapshot.clone());
        Ok(())
    }
}

fn timestamp(value: &str) -> WireTimestamp {
    WireTimestamp::from_str(value).expect("valid fixture timestamp")
}

fn snapshot(source_id: &str, reason: &str) -> CalendarSnapshot {
    CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: source_id.to_owned(),
        from: timestamp("2026-01-01T00:00:00Z"),
        to: timestamp("2027-12-31T23:59:59Z"),
        schedules: vec![TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp("2026-06-19T00:00:00Z"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: reason.to_owned(),
            source_id: source_id.to_owned(),
            observed: false,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-01-02T00:00:00Z"),
        valid_until: timestamp("2027-12-31T23:59:59Z"),
        checksum: reason.to_owned(),
    }
}

fn settings(source_id: &str) -> CalendarManagerSettings {
    CalendarManagerSettings {
        refresh_interval_hours: 24,
        warmup_markets: vec!["US".to_owned()],
        source_policies: vec![CalendarSourcePolicy {
            market: "US".to_owned(),
            preferred_source_ids: vec![source_id.to_owned()],
            enabled_source_ids: vec![source_id.to_owned()],
            fallback_to_builtin: true,
            stale_after_hours: 0,
            ..CalendarSourcePolicy::default()
        }],
        ..CalendarManagerSettings::default()
    }
}

fn manager(
    source: Arc<FixtureSource>,
    persistence: Option<Arc<dyn CalendarPersistencePort>>,
    settings: CalendarManagerSettings,
    now: Arc<Mutex<OffsetDateTime>>,
) -> CalendarManager {
    let mut registry = CalendarSourceRegistry::default();
    registry.register(source).expect("register fixture source");
    CalendarManager::with_clock(
        registry,
        persistence,
        settings,
        Arc::new(move || *now.lock().expect("fixture clock")),
    )
    .expect("create calendar manager")
}

#[test]
fn registry_snapshot_manual_and_builtin_policy_order_is_stable() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("official", events));
    source.push(Ok(snapshot("official", "external")));
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-01T00:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = manager(source, None, settings("official"), now);
    manager.start().expect("start manager");
    assert_eq!(manager.refresh_market("US").expect("refresh").updated, 1);
    let day = timestamp("2026-06-19T00:00:00Z");
    assert_eq!(
        manager
            .schedule("US", day)
            .expect("external schedule")
            .expect("schedule")
            .reason,
        "external"
    );

    let mut manual = settings("official");
    manual.manual_overrides.push(CalendarManualOverride {
        market: "US".to_owned(),
        date: "2026-06-19".to_owned(),
        status: "special".to_owned(),
        sessions: vec![CalendarSessionOverride {
            kind: "regular".to_owned(),
            start_minute: 600,
            end_minute: 660,
        }],
        reason: "manual".to_owned(),
        observed: true,
    });
    manager
        .reload_settings(manual)
        .expect("reload manual policy");
    let schedule = manager
        .schedule("US", day)
        .expect("manual schedule")
        .expect("schedule");
    assert_eq!(schedule.source_id, "manual_override");
    assert_eq!(schedule.reason, "manual");

    let mut builtin = settings("disabled");
    builtin.source_policies[0].enabled_source_ids = vec!["disabled".to_owned()];
    manager
        .reload_settings(builtin)
        .expect("reload builtin policy");
    let schedule = manager
        .schedule("US", timestamp("2026-06-22T00:00:00Z"))
        .expect("builtin schedule")
        .expect("schedule");
    assert_eq!(schedule.source_id, "builtin_rules");
    assert_eq!(schedule.status, "open");
    manager.close().expect("close manager");
}

#[test]
fn persistence_failure_keeps_the_last_valid_memory_snapshot() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("official", events));
    source.push(Ok(snapshot("official", "replacement")));
    let persistence = Arc::new(FixturePersistence {
        loaded: CalendarSnapshotLoadResult {
            snapshots: vec![snapshot("official", "restored")],
            errors: Vec::new(),
        },
        fail_save: AtomicBool::new(true),
        saved: Mutex::new(Vec::new()),
    });
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-01T00:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = manager(source, Some(persistence), settings("official"), now);
    manager.start().expect("start manager");
    let result = manager.refresh_market("US").expect("refresh");
    assert_eq!((result.updated, result.failures), (0, 1));
    let schedule = manager
        .schedule("US", timestamp("2026-06-19T00:00:00Z"))
        .expect("restored schedule")
        .expect("schedule");
    assert_eq!(schedule.reason, "restored");
    manager.close().expect("close manager");
}

#[test]
fn source_failures_back_off_and_recover_after_the_clock_advances() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("official", events));
    source.push(Err(CalendarSourceError::Failed("offline".to_owned())));
    source.push(Ok(snapshot("official", "recovered")));
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-01T00:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = manager(source, None, settings("official"), Arc::clone(&now));
    manager.start().expect("start manager");
    assert_eq!(
        manager
            .refresh_market("US")
            .expect("failed refresh")
            .failures,
        1
    );
    assert_eq!(
        manager
            .refresh_market("US")
            .expect("backoff refresh")
            .skipped_backoff,
        1
    );
    *now.lock().expect("fixture clock") += Duration::hours(2);
    assert_eq!(
        manager
            .refresh_market("US")
            .expect("recovered refresh")
            .updated,
        1
    );
    let status = manager.source_statuses().expect("source status").remove(0);
    assert_eq!(status.health_state, "healthy");
    assert_eq!(status.consecutive_failures, 0);
    assert!(status.next_refresh_at.is_none());
    manager.close().expect("close manager");
}

#[test]
fn startup_failure_closes_previously_started_sources_in_reverse_and_fails_closed() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let first = Arc::new(FixtureSource::new("first", Arc::clone(&events)));
    let mut second = FixtureSource::new("second", Arc::clone(&events));
    second.start_error = true;
    let second = Arc::new(second);
    let third = Arc::new(FixtureSource::new("third", Arc::clone(&events)));
    let mut registry = CalendarSourceRegistry::default();
    registry.register(first).expect("register first");
    registry.register(second).expect("register second");
    registry.register(third).expect("register third");
    let manager = CalendarManager::new(registry, None, CalendarManagerSettings::default())
        .expect("create manager");
    assert!(manager.start().is_err());
    assert_eq!(
        manager.lifecycle_state().expect("lifecycle"),
        ManagerLifecycleState::Closed
    );
    assert_eq!(
        *events.lock().expect("fixture events"),
        ["start:first", "start:second", "close:first"]
    );
    assert!(manager.start().is_err());
}

#[test]
fn settings_reload_starts_auto_refresh_and_close_cancels_it_idempotently() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let mut blocking = FixtureSource::new("official", Arc::clone(&events));
    blocking.block_until_cancelled = true;
    let source = Arc::new(blocking);
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-01T00:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = manager(Arc::clone(&source), None, settings("official"), now);
    manager.start().expect("start manager");
    assert_eq!(source.fetch_count.load(Ordering::Acquire), 0);
    let mut automatic = settings("official");
    automatic.auto_refresh_enabled = true;
    manager
        .reload_settings(automatic)
        .expect("enable auto refresh");
    source.wait_for_fetch();
    manager.close().expect("close manager");
    manager.close().expect("close manager twice");
    assert!(matches!(
        manager.reload_settings(settings("official")),
        Err(CalendarManagerError::Closed)
    ));
    assert_eq!(
        manager.lifecycle_state().expect("lifecycle"),
        ManagerLifecycleState::Closed
    );
    assert_eq!(
        events
            .lock()
            .expect("fixture events")
            .iter()
            .filter(|event| event.as_str() == "close:official")
            .count(),
        1
    );
}

fn _assert_refresh_result_is_send_sync(_: CalendarRefreshResult) {}
