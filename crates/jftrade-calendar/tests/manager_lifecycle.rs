use std::collections::VecDeque;
use std::str::FromStr;
use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};
use std::sync::{Arc, Condvar, Mutex};
use std::thread;
use std::time::{Duration as StdDuration, Instant};

use jftrade_calendar::{
    BUILTIN_SOURCE_ID, CalendarCancellationToken, CalendarManager, CalendarManagerError,
    CalendarManagerSettings, CalendarManualOverride, CalendarPersistencePort,
    CalendarRefreshResult, CalendarSessionOverride, CalendarSnapshot, CalendarSnapshotLoadResult,
    CalendarSourceDescriptor, CalendarSourceError, CalendarSourcePolicy, CalendarSourcePort,
    CalendarSourceRegistry, MANUAL_OVERRIDE_SOURCE_ID, ManagerLifecycleState, TradingDaySchedule,
};
use jftrade_kernel::WireTimestamp;
use time::{Duration, OffsetDateTime};

#[derive(Clone)]
struct FixtureSource {
    descriptor: CalendarSourceDescriptor,
    /// Overrides `descriptor.markets` so a fixture can declare mainland codes.
    descriptor_markets: Vec<String>,
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
                authority: "fixture".to_owned(),
                markets: vec!["US".to_owned()],
            },
            descriptor_markets: vec!["US".to_owned()],
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
        let mut descriptor = self.descriptor.clone();
        descriptor.markets = self.descriptor_markets.clone();
        descriptor
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
    snapshot_on(source_id, reason, "2026-06-19T00:00:00Z")
}

fn snapshot_on(source_id: &str, reason: &str, date: &str) -> CalendarSnapshot {
    CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: source_id.to_owned(),
        from: timestamp("2026-01-01T00:00:00Z"),
        to: timestamp("2027-12-31T23:59:59Z"),
        schedules: vec![TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp(date),
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

fn build_manager(
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

/// A source that answers every declared market successfully while recording the
/// markets it was asked for, so a test can assert which markets a probe or a
/// warmup refresh actually touched.
struct MarketRecordingSource {
    id: String,
    markets: Vec<String>,
    log: Arc<Mutex<Vec<String>>>,
}

impl MarketRecordingSource {
    fn new(id: &str, markets: Vec<&str>, log: Arc<Mutex<Vec<String>>>) -> Self {
        Self {
            id: id.to_owned(),
            markets: markets.into_iter().map(str::to_owned).collect(),
            log,
        }
    }
}

impl CalendarSourcePort for MarketRecordingSource {
    fn descriptor(&self) -> CalendarSourceDescriptor {
        CalendarSourceDescriptor {
            id: self.id.clone(),
            kind: "recording".to_owned(),
            authority: "fixture".to_owned(),
            markets: self.markets.clone(),
        }
    }

    fn fetch(
        &self,
        market: &str,
        from: WireTimestamp,
        _to: WireTimestamp,
        _cancellation: &CalendarCancellationToken,
    ) -> Result<CalendarSnapshot, CalendarSourceError> {
        self.log
            .lock()
            .expect("market fetch log")
            .push(market.to_owned());
        Ok(CalendarSnapshot {
            market_code: market.to_owned(),
            source_id: self.id.clone(),
            from,
            to: timestamp("2027-12-31T23:59:59Z"),
            schedules: vec![TradingDaySchedule {
                market_code: market.to_owned(),
                date: timestamp("2026-06-19T00:00:00Z"),
                status: "closed".to_owned(),
                sessions: Vec::new(),
                reason: "juneteenth".to_owned(),
                source_id: self.id.clone(),
                observed: false,
                updated_at: None,
            }],
            fetched_at: timestamp("2026-06-19T12:00:00Z"),
            valid_until: timestamp("2026-06-26T12:00:00Z"),
            checksum: String::new(),
        })
    }
}

/// A source that records the order in which it is asked to fetch, so a test can
/// assert the registry's resolve order through the real refresh path.
struct RecordingOrderSource {
    id: String,
    markets: Vec<String>,
    order: Arc<Mutex<Vec<String>>>,
}

impl RecordingOrderSource {
    fn new(id: &str, market: &str, order: Arc<Mutex<Vec<String>>>) -> Self {
        Self {
            id: id.to_owned(),
            markets: vec![market.to_owned()],
            order,
        }
    }
}

impl CalendarSourcePort for RecordingOrderSource {
    fn descriptor(&self) -> CalendarSourceDescriptor {
        CalendarSourceDescriptor {
            id: self.id.clone(),
            kind: "recording".to_owned(),
            authority: "fixture".to_owned(),
            markets: self.markets.clone(),
        }
    }

    fn fetch(
        &self,
        _market: &str,
        _from: WireTimestamp,
        _to: WireTimestamp,
        _cancellation: &CalendarCancellationToken,
    ) -> Result<CalendarSnapshot, CalendarSourceError> {
        self.order
            .lock()
            .expect("fetch order")
            .push(self.id.clone());
        Err(CalendarSourceError::Failed("recording fixture".to_owned()))
    }
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
    let manager = build_manager(source, None, settings("official"), now);
    manager.start().expect("start manager");
    assert_eq!(manager.refresh_market("US").expect("refresh").updated, 1);
    let sources = manager.sources_snapshot().expect("source projection");
    let official = sources
        .sources
        .iter()
        .find(|source| source.id == "official")
        .expect("registered source projection");
    assert_eq!(official.health_state, "healthy");
    let status = manager.status_snapshot().expect("status projection");
    assert_eq!(status.snapshots[0].checksum, "external");
    assert_eq!(status.sources.len(), 3);
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
fn restored_snapshot_is_served_while_source_adapter_is_temporarily_absent() {
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-01T00:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let persistence = Arc::new(FixturePersistence {
        loaded: CalendarSnapshotLoadResult {
            snapshots: vec![snapshot("official", "restored")],
            errors: Vec::new(),
        },
        fail_save: AtomicBool::new(false),
        saved: Mutex::new(Vec::new()),
    });
    let manager = CalendarManager::with_clock(
        CalendarSourceRegistry::default(),
        Some(persistence),
        settings("official"),
        Arc::new(move || *now.lock().expect("fixture clock")),
    )
    .expect("create manager");
    manager.start().expect("start manager");

    let schedule = manager
        .schedule("US", timestamp("2026-06-19T00:00:00Z"))
        .expect("schedule")
        .expect("restored schedule");
    assert_eq!(schedule.source_id, "official");
    assert_eq!(schedule.reason, "restored");
    manager.close().expect("close manager");
}

#[test]
fn disabled_preferred_source_snapshot_is_not_served() {
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-01T00:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let persistence = Arc::new(FixturePersistence {
        loaded: CalendarSnapshotLoadResult {
            snapshots: vec![snapshot("official", "restored")],
            errors: Vec::new(),
        },
        fail_save: AtomicBool::new(false),
        saved: Mutex::new(Vec::new()),
    });
    let mut policy = settings("official");
    policy.source_policies[0].enabled_source_ids = vec![BUILTIN_SOURCE_ID.to_owned()];
    let manager = CalendarManager::with_clock(
        CalendarSourceRegistry::default(),
        Some(persistence),
        policy,
        Arc::new(move || *now.lock().expect("fixture clock")),
    )
    .expect("create manager");
    manager.start().expect("start manager");

    let schedule = manager
        .schedule("US", timestamp("2026-06-19T00:00:00Z"))
        .expect("schedule")
        .expect("builtin schedule");
    assert_eq!(schedule.source_id, BUILTIN_SOURCE_ID);
    assert_eq!(schedule.status, "open");
    let status = manager.status_snapshot().expect("status");
    assert!(
        !status.markets[0]
            .fallback_chain
            .contains(&"official".to_owned())
    );
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_runtime_test.go:86
/// TestManagerRefreshKeepsValidSnapshotWhenPersistenceFails.
///
/// Go caches a freshly fetched snapshot in memory *before* persisting it, so a
/// store that cannot write still leaves the operator with a usable calendar:
/// the refresh counts a failure and records the store error on the source, but
/// the fetched remote snapshot keeps serving the day instead of falling back to
/// builtin rules. The Rust owner used to persist first and skip the cache on
/// error, which silently discarded a valid remote calendar during a durable
/// store outage; this test pins the Go ordering.
#[test]
fn persistence_failure_keeps_the_fetched_snapshot_served_from_memory() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("official", events));
    source.push(Ok(snapshot("official", "remote emergency closure")));
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
    let manager = build_manager(
        source,
        Some(Arc::clone(&persistence) as Arc<dyn CalendarPersistencePort>),
        settings("official"),
        now,
    );
    manager.start().expect("start manager");

    let result = manager.refresh_market("US").expect("refresh");
    assert_eq!(
        (result.updated, result.failures),
        (0, 1),
        "a rejected write is a failure, not an update"
    );
    assert!(
        persistence
            .saved
            .lock()
            .expect("saved snapshots")
            .is_empty(),
        "the fixture store must reject every write"
    );

    // The freshly fetched snapshot replaced the restored one in memory.
    let schedule = manager
        .schedule("US", timestamp("2026-06-19T00:00:00Z"))
        .expect("in-memory schedule")
        .expect("schedule");
    assert_eq!(schedule.source_id, "official");
    assert_eq!(schedule.reason, "remote emergency closure");

    // The store error stays visible on the source that failed to persist.
    let status = manager
        .source_statuses()
        .expect("source statuses")
        .into_iter()
        .find(|status| status.source_id == "official")
        .expect("source status");
    assert_eq!(status.last_error, "fixture persistence unavailable");
    assert_eq!(status.consecutive_failures, 1);
    assert_eq!(
        status.health_state, "healthy",
        "the provider itself succeeded; only the store write failed, so Go's \
         recordOperationFailure must leave the health state alone"
    );
    assert_eq!(
        status.last_alert_status, "",
        "a durable-store failure is not a provider alert"
    );
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
    let manager = build_manager(source, None, settings("official"), Arc::clone(&now));
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
    let manager = build_manager(Arc::clone(&source), None, settings("official"), now);
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

#[test]
fn probe_reports_partial_success_then_all_source_failure_without_caching() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let first = Arc::new(FixtureSource::new("first", Arc::clone(&events)));
    first.push(Ok(snapshot("first", "healthy")));
    first.push(Err(CalendarSourceError::Failed("first offline".to_owned())));
    let second = Arc::new(FixtureSource::new("second", events));
    second.push(Err(CalendarSourceError::Failed(
        "second offline".to_owned(),
    )));
    second.push(Err(CalendarSourceError::Failed(
        "second still offline".to_owned(),
    )));
    let mut registry = CalendarSourceRegistry::default();
    registry.register(first).expect("register first source");
    registry.register(second).expect("register second source");
    let mut probe_settings = settings("first");
    probe_settings.source_policies[0]
        .enabled_source_ids
        .push("second".to_owned());
    let manager = CalendarManager::new(registry, None, probe_settings).expect("create manager");
    manager.start().expect("start manager");

    let partial = manager.probe_market("US").expect("partial probe");
    assert_eq!((partial.healthy, partial.failures), (1, 1));
    assert!(manager.snapshots().expect("probe cache").is_empty());
    let failed = manager.probe_market("US").expect("failed probe");
    assert_eq!((failed.healthy, failed.failures), (0, 2));
    assert!(
        failed
            .results
            .iter()
            .all(|result| result.status == "unhealthy")
    );
    manager.close().expect("close manager");
}

#[test]
fn probe_timeout_is_reported_and_close_cancels_an_inflight_probe() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let mut blocking = FixtureSource::new("official", events);
    blocking.block_until_cancelled = true;
    let source = Arc::new(blocking);
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-01T00:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = Arc::new(build_manager(
        Arc::clone(&source),
        None,
        settings("official"),
        now,
    ));
    manager.start().expect("start manager");
    let timed_out = manager
        .probe_market_with_timeout("US", StdDuration::from_millis(20))
        .expect("timed out probe result");
    assert_eq!(timed_out.failures, 1);
    assert!(timed_out.results[0].error.contains("timed out"));

    *source
        .fetch_signal
        .0
        .lock()
        .expect("reset fixture fetch signal") = false;
    let manager_for_probe = Arc::clone(&manager);
    let probe = thread::spawn(move || manager_for_probe.probe_market("US"));
    source.wait_for_fetch();
    manager.close().expect("close manager during probe");
    let cancelled = probe.join().expect("join probe").expect("cancelled result");
    assert_eq!(cancelled.failures, 1);
    assert!(cancelled.results[0].error.contains("cancelled"));
}

#[test]
fn unknown_market_probe_and_refresh_are_accepted_noops() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("official", events));
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-01T00:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, settings("official"), now);
    manager.start().expect("start manager");
    let refresh = manager.refresh_market("MARS").expect("unknown refresh");
    let probe = manager.probe_market("MARS").expect("unknown probe");
    assert!(refresh.accepted && probe.accepted);
    assert_eq!(refresh.market, "MARS");
    assert_eq!(probe.market, "MARS");
    assert_eq!((refresh.updated, refresh.failures), (0, 0));
    assert_eq!((probe.healthy, probe.failures), (0, 0));
    manager.close().expect("close manager");
}

#[test]
fn calendar_control_wire_matches_the_current_go_owner_fixture() {
    let fixture: serde_json::Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/calendar-control.json"
    ))
    .expect("calendar control fixture");
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("fixture_source", events));
    let mut control_snapshot = snapshot("fixture_source", "unused");
    control_snapshot.fetched_at = timestamp("2026-01-02T03:00:00Z");
    control_snapshot.valid_until = timestamp("2026-01-09T03:00:00Z");
    control_snapshot.checksum = "fixture-checksum-1".to_owned();
    control_snapshot.schedules = vec![
        TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp("2026-01-02T00:00:00Z"),
            status: "open".to_owned(),
            sessions: Vec::new(),
            reason: String::new(),
            source_id: "fixture_source".to_owned(),
            observed: false,
            updated_at: None,
        },
        TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp("2026-01-19T00:00:00Z"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: "fixture_holiday".to_owned(),
            source_id: "fixture_source".to_owned(),
            observed: true,
            updated_at: None,
        },
        TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp("2026-11-27T00:00:00Z"),
            status: "early_close".to_owned(),
            sessions: vec![jftrade_calendar::CalendarSessionWindow {
                kind: "regular".to_owned(),
                start_minute: 570,
                end_minute: 780,
            }],
            reason: "fixture_early_close".to_owned(),
            source_id: "fixture_source".to_owned(),
            observed: false,
            updated_at: None,
        },
    ];
    source.push(Ok(control_snapshot.clone()));
    source.push(Ok(control_snapshot));
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-19T12:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, settings("fixture_source"), now);
    manager.start().expect("start manager");
    assert_eq!(
        serde_json::to_value(manager.refresh_all().expect("refresh all")).expect("refresh wire"),
        fixture["refreshAll"]
    );
    let status_fixture: serde_json::Value = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/calendar-status.json"
    ))
    .expect("calendar status fixture");
    assert_eq!(
        serde_json::to_value(manager.status_snapshot().expect("status snapshot"))
            .expect("status wire"),
        status_fixture["status"]
    );
    assert_eq!(
        serde_json::to_value(manager.refresh_market("MARS").expect("unknown refresh"))
            .expect("unknown refresh wire"),
        fixture["refreshUnknown"]
    );
    assert_eq!(
        serde_json::to_value(manager.probe_market("US").expect("probe US")).expect("probe wire"),
        fixture["probeUS"]
    );
    assert_eq!(
        serde_json::to_value(manager.probe_market("MARS").expect("unknown probe"))
            .expect("unknown probe wire"),
        fixture["probeUnknown"]
    );
    manager.close().expect("close manager");
}

fn _assert_refresh_result_is_send_sync(_: CalendarRefreshResult) {}

/// Parity: go:452dea11:internal/exchangecalendar/manager_runtime_test.go:125
/// TestManagerRestoreReportsMalformedCachedSnapshot.
///
/// Go restores cached snapshots at construction, and a file whose JSON cannot
/// be decoded is reported on the builtin source with the decode error *and* the
/// offending path, so an operator can find the corrupt cache. This pins the
/// Rust equivalent: the store reports a `Decode` load error carrying the path,
/// the manager records it against `builtin_rules`, and no schedule is served
/// from the malformed file.
#[test]
fn restore_reports_malformed_cached_snapshot_with_its_path() {
    let directory = std::env::temp_dir().join(format!(
        "jftrade-calendar-restore-{}-{}",
        std::process::id(),
        OffsetDateTime::now_utc().unix_timestamp_nanos()
    ));
    let path = directory.join("US").join("2026").join("broken.json");
    std::fs::create_dir_all(path.parent().expect("snapshot parent")).expect("prepare snapshot dir");
    std::fs::write(&path, "{\"sourceId\":").expect("write malformed snapshot");

    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-01T00:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = CalendarManager::with_clock(
        CalendarSourceRegistry::default(),
        Some(Arc::new(jftrade_calendar::CalendarSnapshotStore::new(
            directory.clone(),
        )) as Arc<dyn CalendarPersistencePort>),
        settings("official"),
        Arc::new(move || *now.lock().expect("fixture clock")),
    )
    .expect("create manager");
    manager.start().expect("start manager");

    let status = manager
        .status_snapshot()
        .expect("status snapshot")
        .sources
        .into_iter()
        .find(|status| status.id == BUILTIN_SOURCE_ID)
        .expect("builtin source status");
    assert!(
        status.last_error.contains("broken.json"),
        "restore error must name the offending file: {:?}",
        status.last_error
    );
    assert!(
        status.last_error.to_lowercase().contains("decode")
            || status.last_error.to_lowercase().contains("expected")
            || status.last_error.to_lowercase().contains("eof"),
        "restore error must describe the decode failure: {:?}",
        status.last_error
    );
    // A malformed cache file never becomes a served schedule.
    let schedule = manager
        .schedule("US", timestamp("2026-06-19T00:00:00Z"))
        .expect("builtin schedule")
        .expect("schedule");
    assert_eq!(
        schedule.source_id, BUILTIN_SOURCE_ID,
        "no snapshot may be served from the malformed cache"
    );
    manager.close().expect("close manager");
    std::fs::remove_dir_all(&directory).expect("clean snapshot root");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_runtime_test.go:142
/// TestManagerStatusReportsManualAndRemoteOverrideModes.
///
/// Go's `Status()` reports `manual_override` when an operator override owns the
/// checked day, and `remote_override` when a fresh remote snapshot supplies the
/// special schedule for it. The two modes must never collapse into one another,
/// because the console uses them to tell an operator decision apart from a
/// provider's.
#[test]
fn status_reports_manual_and_remote_override_modes_distinctly() {
    // `Status()` describes the current day, so the clock and the override both
    // sit on the same 2026-07-02 trading day as the Go fixture.
    let day = timestamp("2026-07-02T00:00:00Z");
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-07-02T16:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));

    // Manual override: the operator's decision is authoritative for the day.
    let mut manual_settings = settings("official");
    manual_settings
        .manual_overrides
        .push(CalendarManualOverride {
            market: "US".to_owned(),
            date: "2026-07-02".to_owned(),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: "operator closure".to_owned(),
            observed: true,
        });
    let manual_manager = CalendarManager::with_clock(
        CalendarSourceRegistry::default(),
        None,
        manual_settings,
        Arc::new({
            let now = Arc::clone(&now);
            move || *now.lock().expect("fixture clock")
        }),
    )
    .expect("create manual manager");
    manual_manager.start().expect("start manual manager");
    let manual = manual_manager.status_snapshot().expect("manual status");
    assert_eq!(manual.markets[0].effective_source, "manual_override");
    assert_eq!(manual.markets[0].effective_mode, "manual_override");
    assert_eq!(
        manual.markets[0].effective_reason,
        "manual override is active for the checked trading day"
    );
    let schedule = manual_manager
        .schedule("US", day)
        .expect("manual schedule")
        .expect("schedule");
    assert_eq!(schedule.source_id, "manual_override");
    assert_eq!(schedule.status, "closed");
    manual_manager.close().expect("close manual manager");

    // Remote override: a fresh snapshot carries a special schedule for the day.
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("official-source", events));
    // The provider supplies a special schedule for the checked day itself.
    // The schedule instant resolves to the checked US trading day, exactly
    // like Go's fixture that stamps the schedule with `now`.
    source.push(Ok(snapshot_on(
        "official-source",
        "remote holiday",
        "2026-07-02T16:00:00Z",
    )));
    let remote_manager = build_manager(source, None, settings("official-source"), now);
    remote_manager.start().expect("start remote manager");
    assert_eq!(
        remote_manager
            .refresh_all()
            .expect("remote refresh")
            .updated,
        1
    );
    let remote = remote_manager.status_snapshot().expect("remote status");
    assert_eq!(remote.markets[0].effective_source, "official-source");
    assert_eq!(
        remote.markets[0].effective_mode, "remote_override",
        "a fresh snapshot that owns the checked day is a remote override"
    );
    assert_eq!(
        remote.markets[0].effective_reason,
        "a fresh source snapshot covers the checked trading day"
    );
    remote_manager.close().expect("close remote manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_runtime_test.go:17
/// TestManagerBackgroundRefreshFollowsSettingsReload.
///
/// Go's background loop does no work while `AutoRefreshEnabled` is false, and a
/// settings reload immediately runs a warmup refresh once it is turned on. The
/// Rust owner keeps the same contract: `reload_settings` triggers the refresh
/// loop, the fetched snapshot becomes the served schedule, and `close`
/// cancels the loop exactly once.
#[test]
fn background_refresh_follows_an_auto_refresh_settings_reload() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("reload-source", Arc::clone(&events)));
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-07-02T16:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let mut policy = settings("reload-source");
    policy.auto_refresh_enabled = false;
    let manager = build_manager(Arc::clone(&source), None, policy, now);

    // A started manager with auto refresh disabled never touches the source.
    manager.start().expect("start manager");
    thread::sleep(StdDuration::from_millis(40));
    assert_eq!(
        source.fetch_count.load(Ordering::Acquire),
        0,
        "auto refresh must stay idle while disabled"
    );

    // Every reload observation must be satisfiable: queue one snapshot per
    // potential background pass, all carrying the same current-day schedule.
    for _ in 0..4 {
        source.push(Ok(snapshot_on(
            "reload-source",
            "reload test",
            "2026-07-02T16:00:00Z",
        )));
    }
    let mut automatic = settings("reload-source");
    automatic.auto_refresh_enabled = true;
    manager
        .reload_settings(automatic)
        .expect("enable auto refresh");
    source.wait_for_fetch();

    let deadline = Instant::now() + StdDuration::from_secs(3);
    loop {
        // Go looks the schedule up with the same `now` the refresh used.
        let schedule = manager
            .schedule("US", timestamp("2026-07-02T16:00:00Z"))
            .expect("schedule after background refresh");
        if let Some(ref schedule) = schedule
            && schedule.source_id == "reload-source"
            && schedule.reason == "reload test"
        {
            break;
        }
        assert!(
            Instant::now() < deadline,
            "background refresh never served the reloaded snapshot: {schedule:?}"
        );
        thread::sleep(StdDuration::from_millis(5));
    }
    manager.close().expect("close manager");
    assert_eq!(
        events
            .lock()
            .expect("fixture events")
            .iter()
            .filter(|event| event.as_str() == "close:reload-source")
            .count(),
        1,
        "close must cancel the refresh loop exactly once"
    );
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:50
/// TestDefaultWarmupRefreshTimeoutCoversSequentialRemoteSources.
///
/// Go bounds one background warmup refresh with a 60s deadline sized to cover
/// three sequential 15s HTTP providers. Rust has no HTTP calendar provider at
/// all (the `CalendarSourcePort` is implemented only by embedders and tests),
/// so there is no warmup deadline constant to compare against; the property
/// that survives is that a single provider call is bounded by the probe budget
/// instead of being able to hang a warmup.
///
/// Deliberate difference: Go's `defaultWarmupRefreshTimeout >= 3 *
/// defaultHTTPTimeout` guard exists because a warmup may fan out across several
/// remote sources. With no remote source in the Rust composition there is
/// nothing to size that budget for, so the equivalent guarantee is asserted at
/// the call level: `probe_market_with_timeout` returns a bounded failure
/// instead of hanging.
#[test]
fn probe_budget_bounds_one_provider_call_without_hanging_the_manager() {
    let source = Arc::new(FixtureSource::new(
        "budget",
        Arc::new(Mutex::new(Vec::new())),
    ));
    let manager = CalendarManager::new(
        {
            let mut registry = CalendarSourceRegistry::default();
            registry.register(source).expect("register source");
            registry
        },
        None,
        settings("budget"),
    )
    .expect("create manager");
    manager.start().expect("start manager");

    let started = Instant::now();
    let result = manager
        .probe_market_with_timeout("US", StdDuration::from_millis(25))
        .expect("bounded probe");
    assert_eq!(result.failures, 1, "an exhausted budget is a failure");
    assert!(
        started.elapsed() < StdDuration::from_secs(3),
        "the provider budget must bound one call, not hang the manager"
    );
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:64
/// TestManagerFallsBackToBuiltinWhenOfficialRefreshFails.
///
/// Go refreshes through a failing provider, counts the failure, and then serves
/// the builtin rules for the day while the provider's `lastError` stays visible
/// in the sources projection. The same shape is asserted here against the Rust
/// manager's refresh result, schedule resolution and source status.
#[test]
fn failing_provider_falls_back_to_builtin_and_keeps_the_error_visible() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("nyse_official", events));
    source.push(Err(CalendarSourceError::Failed("boom".to_owned())));
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-19T12:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let mut policy = settings("nyse_official");
    policy.source_policies[0].stale_after_hours = 24;
    let manager = build_manager(source, None, policy, now);
    manager.start().expect("start manager");

    let result = manager.refresh_all().expect("refresh all");
    assert_eq!((result.updated, result.failures), (0, 1));

    // 2026-06-19 is Juneteenth: the builtin template closes US equities.
    let schedule = manager
        .schedule("US", timestamp("2026-06-19T12:00:00Z"))
        .expect("builtin schedule")
        .expect("schedule");
    assert_eq!(schedule.source_id, BUILTIN_SOURCE_ID);
    assert_eq!(schedule.status, "closed");

    let status = manager
        .status_snapshot()
        .expect("status snapshot")
        .sources
        .into_iter()
        .find(|source| source.id == "nyse_official")
        .expect("provider projection");
    assert_eq!(status.last_error, "boom");
    assert_eq!(status.consecutive_failures, 1);
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:119
/// TestManagerStatusIncludesSnapshotSummariesAndSampleSchedules.
///
/// Go's status projection lists one summary per cached snapshot with its
/// identity, `schedulesParsed` count and checksum, and only the non-open days
/// become `sampleSchedules` (capped at eight). This pins the same projection
/// for a provider that returns one open day, one closed holiday and one
/// early-close day with a shortened regular session.
#[test]
fn status_summaries_expose_snapshot_metadata_and_non_open_samples() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("nyse_official", events));
    // One open day plus ten non-open days, so the eight-sample cap is real.
    let mut schedules = vec![TradingDaySchedule {
        market_code: "US".to_owned(),
        date: timestamp("2026-01-02T00:00:00Z"),
        status: "open".to_owned(),
        sessions: Vec::new(),
        reason: String::new(),
        source_id: "nyse_official".to_owned(),
        observed: false,
        updated_at: None,
    }];
    for day in 1..=10 {
        schedules.push(TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp(&format!("2026-08-{day:02}T00:00:00Z")),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: format!("closure-{day}"),
            source_id: "nyse_official".to_owned(),
            observed: false,
            updated_at: None,
        });
    }
    // The first two non-open days carry the shapes the console renders
    // specially: an observed holiday and an early close with a shortened
    // regular session. Both sit inside the eight-sample window.
    schedules[1].date = timestamp("2026-01-19T00:00:00Z");
    schedules[1].reason = "holiday".to_owned();
    schedules[1].observed = true;
    schedules[2].date = timestamp("2026-02-20T00:00:00Z");
    schedules[2].status = "early_close".to_owned();
    schedules[2].reason = "early close".to_owned();
    schedules[2].sessions = vec![jftrade_calendar::CalendarSessionWindow {
        kind: "regular".to_owned(),
        start_minute: 570,
        end_minute: 780,
    }];
    schedules.sort_by_key(|schedule| schedule.date);
    source.push(Ok(CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: "nyse_official".to_owned(),
        from: timestamp("2026-01-01T00:00:00Z"),
        to: timestamp("2027-12-31T23:59:59Z"),
        schedules,
        fetched_at: timestamp("2026-01-02T03:00:00Z"),
        valid_until: timestamp("2026-01-09T03:00:00Z"),
        checksum: "checksum-1".to_owned(),
    }));
    let mut policy = settings("nyse_official");
    policy.source_policies[0].stale_after_hours = 72;
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-19T12:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, policy, now);
    manager.start().expect("start manager");
    let refreshed = manager.refresh_all().expect("refresh all");
    assert_eq!((refreshed.updated, refreshed.failures), (1, 0));

    let status = manager.status_snapshot().expect("status snapshot");
    assert_eq!(status.snapshots.len(), 1, "one cached snapshot");
    let summary = &status.snapshots[0];
    assert_eq!(summary.market, "US");
    assert_eq!(summary.source_id, "nyse_official");
    assert_eq!(
        summary.schedules_parsed, 11,
        "one open day plus ten closures"
    );
    assert_eq!(summary.checksum, "checksum-1");
    assert_eq!(
        summary.sample_schedules.len(),
        8,
        "only non-open days become samples and the list is capped at eight: {:?}",
        summary.sample_schedules
    );
    let holiday = summary
        .sample_schedules
        .iter()
        .find(|sample| sample.date == "2026-01-19")
        .expect("observed holiday sample");
    assert_eq!(holiday.status, "closed");
    assert_eq!(holiday.reason, "holiday");
    assert!(holiday.observed);
    let early_close = summary
        .sample_schedules
        .iter()
        .find(|sample| sample.date == "2026-02-20")
        .expect("early-close sample");
    assert_eq!(early_close.status, "early_close");
    assert_eq!(
        early_close.sessions.as_ref().expect("early-close sessions"),
        &[jftrade_calendar::CalendarSampleSession {
            kind: "regular".to_owned(),
            start_minute: 570,
            end_minute: 780,
        }]
    );
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:207
/// TestManagerManualOverridesBeatRemoteAndBuiltin.
///
/// An operator override reopens a day the builtin template closes, and it wins
/// over both the remote snapshot and the builtin rules because the schedule is
/// resolved from the manual entry first.
#[test]
fn manual_override_reopens_a_day_instead_of_the_builtin_closure() {
    let mut policy = settings("official");
    policy.manual_overrides.push(CalendarManualOverride {
        market: "US".to_owned(),
        date: "2026-06-19".to_owned(),
        status: "open".to_owned(),
        sessions: vec![CalendarSessionOverride {
            kind: "regular".to_owned(),
            start_minute: 570,
            end_minute: 960,
        }],
        reason: "manual_reopen".to_owned(),
        observed: true,
    });
    let manager = CalendarManager::new(CalendarSourceRegistry::default(), None, policy)
        .expect("create manager");
    manager.start().expect("start manager");

    let schedule = manager
        .schedule("US", timestamp("2026-06-19T12:00:00Z"))
        .expect("manual schedule")
        .expect("schedule");
    assert_eq!(schedule.source_id, "manual_override");
    assert_eq!(schedule.status, "open");
    assert_eq!(schedule.reason, "manual_reopen");
    assert_eq!(schedule.sessions.len(), 1);
    assert_eq!(schedule.sessions[0].start_minute, 570);
    assert_eq!(schedule.sessions[0].end_minute, 960);
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:235
/// TestManagerSharedMainlandSourceAppliesToSHAndSZ.
///
/// The mainland notice provider declares `CN` and a refresh for `CN` must
/// answer schedules for `SH` and `SZ` from that one shared snapshot, because
/// the three codes describe the same mainland session.
#[test]
fn shared_mainland_snapshot_applies_to_shanghai_and_shenzhen() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let mut mainland = FixtureSource::new("mainland_official_notice", events);
    mainland.descriptor_markets = vec!["CN".to_owned(), "SH".to_owned(), "SZ".to_owned()];
    let source = Arc::new(mainland);
    source.push(Ok(CalendarSnapshot {
        market_code: "CN".to_owned(),
        source_id: "mainland_official_notice".to_owned(),
        from: timestamp("2026-01-01T00:00:00+08:00"),
        to: timestamp("2026-12-31T00:00:00+08:00"),
        schedules: vec![TradingDaySchedule {
            market_code: "CN".to_owned(),
            date: timestamp("2026-10-01T00:00:00+08:00"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: "national_day".to_owned(),
            source_id: "mainland_official_notice".to_owned(),
            observed: false,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-09-15T00:00:00Z"),
        valid_until: timestamp("2027-01-31T00:00:00Z"),
        checksum: String::new(),
    }));
    let mut policy = settings("mainland_official_notice");
    policy.warmup_markets = vec!["CN".to_owned()];
    policy.source_policies[0].market = "CN".to_owned();
    policy.source_policies[0].stale_after_hours = 24 * 30;
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-10-01T08:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, policy, now);
    manager.start().expect("start manager");
    assert_eq!(
        manager
            .refresh_market("CN")
            .expect("mainland refresh")
            .updated,
        1
    );

    for market in ["SH", "SZ"] {
        let schedule = manager
            .schedule(market, timestamp("2026-10-01T10:00:00+08:00"))
            .expect("shared mainland schedule")
            .expect("schedule");
        assert_eq!(schedule.source_id, "mainland_official_notice");
        assert_eq!(schedule.status, "closed");
        assert_eq!(schedule.reason, "national_day");
        assert_eq!(
            schedule.market_code, market,
            "the shared snapshot is re-labelled for the requested mainland market"
        );
    }
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:291
/// TestManagerIgnoresStaleRemoteSnapshots.
///
/// A snapshot whose `ValidUntil` has already passed must not answer the day
/// even though it covers it: Go falls back to builtin rules, and the builtin
/// template still knows the real session for that date.
#[test]
fn stale_remote_snapshot_is_ignored_in_favour_of_builtin_rules() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("nyse_official", events));
    source.push(Ok(CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: "nyse_official".to_owned(),
        from: timestamp("2026-01-01T00:00:00-05:00"),
        to: timestamp("2026-12-31T00:00:00-05:00"),
        schedules: vec![TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp("2026-06-22T00:00:00-05:00"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: "stale_remote".to_owned(),
            source_id: "nyse_official".to_owned(),
            observed: false,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-06-01T00:00:00Z"),
        // Already expired relative to the injected clock.
        valid_until: timestamp("2026-06-02T00:00:00Z"),
        checksum: String::new(),
    }));
    let mut policy = settings("nyse_official");
    policy.source_policies[0].stale_after_hours = 24;
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-22T14:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, policy, now);
    manager.start().expect("start manager");
    manager.refresh_all().expect("refresh all");

    let schedule = manager
        .schedule("US", timestamp("2026-06-22T14:00:00Z"))
        .expect("builtin schedule")
        .expect("schedule");
    assert_eq!(
        schedule.source_id, BUILTIN_SOURCE_ID,
        "an expired snapshot must not answer the day"
    );
    assert_eq!(schedule.status, "open");
    assert_ne!(schedule.reason, "stale_remote");
    manager.close().expect("close manager");

    // Isolate the absolute expiry from the age rule: with `staleAfterHours = 0`
    // the `fetchedAt` age no longer matters, so only `validUntil` can disqualify
    // the snapshot. Go checks that field first in `snapshotFresh`.
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("nyse_official", events));
    source.push(Ok(CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: "nyse_official".to_owned(),
        from: timestamp("2026-01-01T00:00:00-05:00"),
        to: timestamp("2026-12-31T00:00:00-05:00"),
        schedules: vec![TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp("2026-06-22T00:00:00-05:00"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: "expired_remote".to_owned(),
            source_id: "nyse_official".to_owned(),
            observed: false,
            updated_at: None,
        }],
        // Fresh by age, but past its validity window.
        fetched_at: timestamp("2026-06-22T13:00:00Z"),
        valid_until: timestamp("2026-06-22T13:30:00Z"),
        checksum: String::new(),
    }));
    let mut expiry_policy = settings("nyse_official");
    expiry_policy.source_policies[0].stale_after_hours = 0;
    let expiry_now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-22T14:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let expiry_manager = build_manager(source, None, expiry_policy, Arc::clone(&expiry_now));
    expiry_manager.start().expect("start expiry manager");
    expiry_manager.refresh_all().expect("expiry refresh");
    let schedule = expiry_manager
        .schedule("US", timestamp("2026-06-22T14:00:00Z"))
        .expect("builtin schedule")
        .expect("schedule");
    assert_eq!(
        schedule.source_id, BUILTIN_SOURCE_ID,
        "an expired validUntil must disqualify an otherwise fresh snapshot"
    );
    assert_ne!(schedule.reason, "expired_remote");
    expiry_manager.close().expect("close expiry manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:346
/// TestManagerDiscardInvalidCachedSnapshotOnRestore.
///
/// Go restores cached snapshots at construction and discards one whose schedule
/// falls outside the snapshot's own range: the manager records the reason on
/// the owning source, deletes the file from the store, and serves builtin rules
/// for the day instead. This pins all three effects, including the on-disk
/// deletion that keeps the bad value from being reconsidered next startup.
#[test]
fn invalid_cached_snapshot_is_discarded_deleted_and_replaced_by_builtin_rules() {
    let directory = std::env::temp_dir().join(format!(
        "jftrade-calendar-discard-{}-{}",
        std::process::id(),
        OffsetDateTime::now_utc().unix_timestamp_nanos()
    ));
    let store = jftrade_calendar::CalendarSnapshotStore::new(directory.clone());
    // 2028 schedule inside a 2026-2027 range: outside the snapshot's own window.
    let invalid = CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: "nyse_official".to_owned(),
        from: timestamp("2026-01-01T00:00:00-05:00"),
        to: timestamp("2027-12-31T23:59:59-05:00"),
        schedules: vec![TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp("2028-07-03T00:00:00-05:00"),
            status: "early_close".to_owned(),
            sessions: Vec::new(),
            reason: String::new(),
            source_id: "nyse_official".to_owned(),
            observed: false,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-06-19T15:29:25Z"),
        valid_until: timestamp("2026-07-03T15:29:25Z"),
        checksum: String::new(),
    };
    let saved_path = store.save(&invalid).expect("seed invalid cached snapshot");
    assert!(saved_path.exists(), "the invalid snapshot starts on disk");

    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-19T12:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    // The source is registered so its status row stays visible in the
    // projection. Auto refresh is off, so it is never fetched here.
    let mut registry = CalendarSourceRegistry::default();
    registry
        .register(Arc::new(FixtureSource::new(
            "nyse_official",
            Arc::new(Mutex::new(Vec::new())),
        )))
        .expect("register provider");
    let manager = CalendarManager::with_clock(
        registry,
        Some(Arc::new(jftrade_calendar::CalendarSnapshotStore::new(
            directory.clone(),
        )) as Arc<dyn CalendarPersistencePort>),
        settings("nyse_official"),
        Arc::new(move || *now.lock().expect("fixture clock")),
    )
    .expect("create manager");
    manager.start().expect("start manager");

    // Juneteenth is a builtin US closure, and the bad cache is not served.
    let schedule = manager
        .schedule("US", timestamp("2026-06-19T12:00:00Z"))
        .expect("builtin schedule")
        .expect("schedule");
    assert_eq!(schedule.source_id, BUILTIN_SOURCE_ID);
    assert_eq!(schedule.status, "closed");

    // The corrupt cache is removed so it cannot be reconsidered next startup.
    assert!(
        !saved_path.exists(),
        "an invalid cached snapshot must be deleted: {saved_path:?}"
    );
    // The reason is recorded against the snapshot's own source, which is how
    // Go's `recordOperationFailure(snapshot.SourceID, ...)` attributes it.
    let discarded = manager
        .status_snapshot()
        .expect("status snapshot")
        .sources
        .into_iter()
        .find(|source| source.id == "nyse_official")
        .expect("provider projection");
    assert!(
        discarded
            .last_error
            .contains("discard invalid cached snapshot"),
        "discard reason missing: {:?}",
        discarded.last_error
    );
    assert_eq!(discarded.consecutive_failures, 1);
    manager.close().expect("close manager");
    std::fs::remove_dir_all(&directory).expect("clean snapshot root");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:393
/// TestManagerProbeMarksHealthySources.
///
/// A probe that parses at least one schedule marks the provider healthy and
/// records the probe instant, market and parsed count on its status row.
#[test]
fn probe_marks_a_productive_provider_healthy_with_its_market_and_count() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("nyse_official", events));
    source.push(Ok(CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: "nyse_official".to_owned(),
        from: timestamp("2026-01-01T00:00:00Z"),
        to: timestamp("2027-12-31T23:59:59Z"),
        schedules: vec![TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp("2026-06-19T00:00:00Z"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: String::new(),
            source_id: "nyse_official".to_owned(),
            observed: false,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-06-01T00:00:00Z"),
        valid_until: timestamp("2026-07-01T00:00:00Z"),
        checksum: String::new(),
    }));
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-02T12:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, settings("nyse_official"), now);
    manager.start().expect("start manager");

    let result = manager.probe_all().expect("probe all");
    assert_eq!((result.healthy, result.failures), (1, 0));
    assert_eq!(result.results.len(), 1);
    assert_eq!(result.results[0].status, "healthy");
    assert_eq!(result.results[0].schedules_parsed, 1);

    let status = manager
        .source_statuses()
        .expect("source statuses")
        .into_iter()
        .find(|status| status.source_id == "nyse_official")
        .expect("provider status");
    assert_eq!(status.last_probe_status, "healthy");
    assert_eq!(status.last_probe_schedules, 1);
    assert_eq!(status.last_probe_market, "US");
    assert!(status.last_probe_at.is_some());
    assert!(status.last_probe_success_at.is_some());
    assert_eq!(status.health_state, "healthy");
    // A successful probe does not fabricate a snapshot.
    assert!(manager.snapshots().expect("snapshot cache").is_empty());
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:444
/// TestManagerProbeMarksEmptyParsesUnhealthy.
///
/// A provider that answers with a well-formed but empty snapshot is a
/// `structure_changed` failure: the probe reports it unhealthy with the literal
/// `no schedules parsed` error instead of treating the empty result as success.
#[test]
fn probe_treats_an_empty_parse_as_structure_changed_unhealthy() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let mut hk = FixtureSource::new("hk_gov_1823_ical", events);
    hk.descriptor_markets = vec!["HK".to_owned()];
    let source = Arc::new(hk);
    source.push(Ok(CalendarSnapshot {
        market_code: "HK".to_owned(),
        source_id: "hk_gov_1823_ical".to_owned(),
        from: timestamp("2026-01-01T00:00:00+08:00"),
        to: timestamp("2026-12-31T23:59:59+08:00"),
        schedules: Vec::new(),
        fetched_at: timestamp("2026-06-01T00:00:00Z"),
        valid_until: timestamp("2026-07-01T00:00:00Z"),
        checksum: String::new(),
    }));
    let mut policy = settings("hk_gov_1823_ical");
    policy.warmup_markets = vec!["HK".to_owned()];
    policy.source_policies[0].market = "HK".to_owned();
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-02T12:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, policy, now);
    manager.start().expect("start manager");

    let result = manager.probe_all().expect("probe all");
    assert_eq!((result.healthy, result.failures), (0, 1));
    assert_eq!(result.results[0].status, "unhealthy");
    assert_eq!(result.results[0].error, "no schedules parsed");

    let status = manager
        .source_statuses()
        .expect("source statuses")
        .into_iter()
        .find(|status| status.source_id == "hk_gov_1823_ical")
        .expect("provider status");
    assert_eq!(status.last_probe_status, "unhealthy");
    assert_eq!(status.last_probe_error, "no schedules parsed");
    assert_eq!(
        status.health_state, "unhealthy",
        "an empty parse is a health failure"
    );
    assert_eq!(
        status.health_fingerprint,
        "hk_gov_1823_ical|HK|structure_changed|structure_changed"
    );
    assert_eq!(status.last_alert_status, "triggered");
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:488
/// TestManagerRefreshTreatsEmptyParsesAsFailureAndAlerts.
///
/// The refresh path must classify an empty parse the same way the probe does:
/// one failure, the provider marked unhealthy, and a `structure_changed` alert
/// recorded. Go raises this through `WithAlertSink`; Rust records the alert on
/// the source status (`lastAlertStatus`/`lastAlertFingerprint`) because there is
/// no alert-sink port in the Rust composition.
#[test]
fn refresh_treats_an_empty_parse_as_structure_changed_failure() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("nyse_official", events));
    source.push(Ok(CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: "nyse_official".to_owned(),
        from: timestamp("2026-01-01T00:00:00Z"),
        to: timestamp("2027-12-31T23:59:59Z"),
        schedules: Vec::new(),
        fetched_at: timestamp("2026-06-01T00:00:00Z"),
        valid_until: timestamp("2026-07-01T00:00:00Z"),
        checksum: String::new(),
    }));
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-02T12:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, settings("nyse_official"), now);
    manager.start().expect("start manager");

    let result = manager.refresh_all().expect("refresh all");
    assert_eq!((result.updated, result.failures), (0, 1));

    let status = manager
        .source_statuses()
        .expect("source statuses")
        .into_iter()
        .find(|status| status.source_id == "nyse_official")
        .expect("provider status");
    assert_eq!(status.last_error, "no schedules parsed");
    assert_eq!(
        status.health_state, "unhealthy",
        "an empty parse is a health failure"
    );
    assert_eq!(
        status.health_fingerprint, "nyse_official|US|structure_changed|structure_changed",
        "the refresh path must fingerprint the structure change like Go's recordSourceFailure"
    );
    assert_eq!(status.last_alert_status, "triggered");
    assert_eq!(
        status.last_alert_fingerprint, status.health_fingerprint,
        "the triggered alert carries the same fingerprint"
    );
    assert!(status.consecutive_failures >= 1);
    // The empty snapshot never becomes a served calendar.
    assert!(manager.snapshots().expect("snapshot cache").is_empty());
    let schedule = manager
        .schedule("US", timestamp("2026-06-19T00:00:00Z"))
        .expect("builtin schedule")
        .expect("schedule");
    assert_eq!(schedule.source_id, BUILTIN_SOURCE_ID);
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:543
/// TestManagerSourceAlertsDeduplicateAndRecover.
///
/// Three probes hit the same fetch failure twice and then succeed: the source
/// must go unhealthy, the repeated identical fault must not raise a second
/// alert, and the recovery must clear the failure state and record a
/// `recovered` alert carrying the previous fingerprint.
#[test]
fn source_alerts_deduplicate_repeats_and_record_recovery() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let mut hk = FixtureSource::new("hk_gov_1823_ical", events);
    hk.descriptor_markets = vec!["HK".to_owned()];
    let source = Arc::new(hk);
    source.push(Err(CalendarSourceError::Failed(
        "temporary fetch failure".to_owned(),
    )));
    source.push(Err(CalendarSourceError::Failed(
        "temporary fetch failure".to_owned(),
    )));
    source.push(Ok(CalendarSnapshot {
        market_code: "HK".to_owned(),
        source_id: "hk_gov_1823_ical".to_owned(),
        from: timestamp("2026-01-01T00:00:00+08:00"),
        to: timestamp("2026-12-31T23:59:59+08:00"),
        schedules: vec![TradingDaySchedule {
            market_code: "HK".to_owned(),
            date: timestamp("2026-06-19T00:00:00+08:00"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: String::new(),
            source_id: "hk_gov_1823_ical".to_owned(),
            observed: false,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-06-01T00:00:00Z"),
        valid_until: timestamp("2026-07-01T00:00:00Z"),
        checksum: String::new(),
    }));
    let mut policy = settings("hk_gov_1823_ical");
    policy.warmup_markets = vec!["HK".to_owned()];
    policy.source_policies[0].market = "HK".to_owned();
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-02T12:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, policy, now);
    manager.start().expect("start manager");

    let status = |manager: &CalendarManager| {
        manager
            .source_statuses()
            .expect("source statuses")
            .into_iter()
            .find(|status| status.source_id == "hk_gov_1823_ical")
            .expect("provider status")
    };
    let expected = "hk_gov_1823_ical|HK|fetch_failed|temporary fetch failure";

    // First failure triggers the alert.
    assert_eq!(manager.probe_all().expect("first probe").failures, 1);
    let first = status(&manager);
    assert_eq!(first.health_state, "unhealthy");
    assert_eq!(first.health_fingerprint, expected);
    assert_eq!(first.last_alert_status, "triggered");
    assert_eq!(first.last_alert_fingerprint, expected);
    assert!(first.last_alert_at.is_some());

    // The identical repeat keeps the state but must not re-trigger.
    assert_eq!(manager.probe_all().expect("second probe").failures, 1);
    let repeated = status(&manager);
    assert_eq!(repeated.health_fingerprint, expected, "same fault identity");
    assert_eq!(
        repeated.last_alert_status, "triggered",
        "the deduplicated repeat does not invent a new alert state"
    );
    assert_eq!(repeated.last_alert_at, first.last_alert_at);

    // Recovery clears the failure state and records the previous fingerprint.
    assert_eq!(manager.probe_all().expect("recovering probe").healthy, 1);
    let recovered = status(&manager);
    assert_eq!(recovered.health_state, "healthy");
    assert_eq!(recovered.health_fingerprint, "");
    assert_eq!(recovered.last_error, "");
    assert_eq!(recovered.consecutive_failures, 0);
    assert_eq!(recovered.next_refresh_at, None);
    assert_eq!(recovered.last_alert_status, "recovered");
    assert_eq!(
        recovered.last_alert_fingerprint, expected,
        "the recovery alert carries the fingerprint it recovered from"
    );
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:613
/// TestManagerSourceAlertsDeduplicateNetworkTimeoutVariants.
///
/// Three different network faults — a cancelled context, a deadline and an HTTP
/// client timeout message — must collapse into the single
/// `network_timeout_or_cancelled` fingerprint, so an operator gets one alert for
/// one outage instead of three.
#[test]
fn network_timeout_variants_share_one_alert_fingerprint() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("nyse_official", events));
    for message in [
        "calendar source operation was cancelled",
        "context deadline exceeded",
        "Get \"https://www.nyse.com/trade/hours-calendars\": context deadline exceeded (Client.Timeout exceeded while awaiting headers)",
    ] {
        source.push(Err(CalendarSourceError::Failed(message.to_owned())));
    }
    let clock_now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-02T12:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(
        source,
        None,
        settings("nyse_official"),
        Arc::clone(&clock_now),
    );
    manager.start().expect("start manager");

    let expected = "nyse_official|US|fetch_failed|network_timeout_or_cancelled";
    let mut fingerprints = Vec::new();
    for round in 0..3 {
        // Rust additionally throttles inside the retry window (Go only writes
        // `NextRefreshAt` for display), so step past it to reach the provider.
        if round > 0 {
            let mut clock = clock_now.lock().expect("fixture clock");
            *clock += Duration::hours(25);
        }
        let result = manager.refresh_all().expect("failing refresh");
        assert_eq!(
            (result.failures, result.skipped_backoff),
            (1, 0),
            "round {round} must reach the provider"
        );
        let status = manager
            .source_statuses()
            .expect("source statuses")
            .into_iter()
            .find(|status| status.source_id == "nyse_official")
            .expect("provider status");
        assert_eq!(
            status.health_fingerprint, expected,
            "every timeout variant shares one fingerprint"
        );
        assert_eq!(status.health_state, "unhealthy");
        assert_eq!(status.last_alert_status, "triggered");
        assert_eq!(status.last_alert_fingerprint, expected);
        fingerprints.push(status.last_alert_at.clone());
    }
    assert_eq!(
        fingerprints.iter().filter(|at| at.is_some()).count(),
        3,
        "the alert instant is recorded on the first trigger and retained"
    );
    assert_eq!(
        fingerprints.windows(2).all(|pair| pair[0] == pair[1]),
        true,
        "same normalized timeout fingerprint must not refresh the alert instant"
    );
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:670
/// TestManagerProbeRecoveryClearsCurrentFetchError.
///
/// After a fetch failure leaves the source unhealthy with a retry scheduled and
/// a recorded error, a successful probe must clear all of it: the health state
/// becomes healthy, the fetch error and probe error are emptied, the failure
/// counter resets and the retry instant is dropped. A stale failure must never
/// survive a successful verification.
#[test]
fn successful_probe_recovery_clears_the_recorded_fetch_failure() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("nyse_official", events));
    source.push(Err(CalendarSourceError::Failed(
        "context deadline exceeded".to_owned(),
    )));
    source.push(Ok(CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: "nyse_official".to_owned(),
        from: timestamp("2026-01-01T00:00:00Z"),
        to: timestamp("2027-12-31T23:59:59Z"),
        schedules: vec![TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp("2026-06-19T00:00:00Z"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: String::new(),
            source_id: "nyse_official".to_owned(),
            observed: false,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-06-01T00:00:00Z"),
        valid_until: timestamp("2026-07-01T00:00:00Z"),
        checksum: String::new(),
    }));
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-02T12:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, settings("nyse_official"), now);
    manager.start().expect("start manager");

    let status = |manager: &CalendarManager| {
        manager
            .source_statuses()
            .expect("source statuses")
            .into_iter()
            .find(|status| status.source_id == "nyse_official")
            .expect("provider status")
    };

    // The fetch failure leaves a retry scheduled and the error recorded.
    assert_eq!(manager.refresh_all().expect("failing refresh").failures, 1);
    let failed = status(&manager);
    assert_eq!(failed.health_state, "unhealthy");
    assert!(
        !failed.last_error.is_empty(),
        "a fetch failure records its error"
    );
    assert!(
        failed.next_refresh_at.is_some(),
        "a fetch failure schedules a retry"
    );
    assert!(failed.consecutive_failures >= 1);

    // A successful probe clears the whole failure picture.
    assert_eq!(manager.probe_all().expect("recovering probe").healthy, 1);
    let recovered = status(&manager);
    assert_eq!(recovered.health_state, "healthy");
    assert_eq!(recovered.last_error, "");
    assert_eq!(recovered.last_probe_error, "");
    assert_eq!(recovered.consecutive_failures, 0);
    assert_eq!(recovered.next_refresh_at, None);
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:736
/// TestSourceRegistryHonorsPreferredSourceOrder.
///
/// Registration order is `b` then `a`, but a policy that prefers `a` must
/// resolve `a` first while still keeping the other enabled source reachable.
/// The observable effect is the refresh fan-out order, so this drives real
/// refreshes and reads the order back from a recording fixture.
#[test]
fn source_registry_honours_preferred_order_over_registration_order() {
    let fetch_order = Arc::new(Mutex::new(Vec::new()));
    let mut registry = CalendarSourceRegistry::default();
    registry
        .register(Arc::new(RecordingOrderSource::new(
            "b",
            "US",
            Arc::clone(&fetch_order),
        )))
        .expect("register b");
    registry
        .register(Arc::new(RecordingOrderSource::new(
            "a",
            "US",
            Arc::clone(&fetch_order),
        )))
        .expect("register a");
    let policy = CalendarManagerSettings {
        refresh_interval_hours: 24,
        warmup_markets: vec!["US".to_owned()],
        source_policies: vec![CalendarSourcePolicy {
            market: "US".to_owned(),
            preferred_source_ids: vec!["a".to_owned()],
            enabled_source_ids: vec!["a".to_owned(), "b".to_owned()],
            fallback_to_builtin: false,
            ..CalendarSourcePolicy::default()
        }],
        ..CalendarManagerSettings::default()
    };
    let manager = CalendarManager::new(registry, None, policy).expect("create manager");
    manager.start().expect("start manager");
    manager.refresh_market("US").expect("refresh US");

    assert_eq!(
        fetch_order.lock().expect("fetch order").clone(),
        vec!["a".to_owned(), "b".to_owned()],
        "the preferred source is fetched first and the remaining enabled source follows"
    );
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:751
/// TestManagerSourcesExposeAvailabilityNotes.
///
/// Every source row an operator can see carries an availability note, and the
/// curated notes for the four official providers stay available even though this
/// process wires no HTTP adapter for them.
///
/// Boundary: Go's `NewManager` seeds `DefaultRegistry(nil)`, so its `Sources()`
/// lists the four official HTTP providers next to builtin/manual. Rust ships no
/// calendar HTTP adapter, production registers an empty registry, and an
/// injected registry deliberately replaces the curated set (that is what the
/// frozen `calendar-status.json` fixture pins). The equivalent guarantee is
/// therefore asserted in two parts: the rows the manager actually lists always
/// carry their note, and the curated descriptor set with its notes remains
/// available through `default_source_descriptors`/`source_availability_note`.
#[test]
fn every_source_row_exposes_an_availability_note() {
    let mut policy = settings("official");
    policy.warmup_markets = vec!["HK".to_owned(), "CN".to_owned()];
    let manager = CalendarManager::new(CalendarSourceRegistry::default(), None, policy)
        .expect("create manager");
    manager.start().expect("start manager");

    let sources = manager.sources_snapshot().expect("sources").sources;
    let notes = sources
        .iter()
        .map(|source| (source.id.clone(), source.availability_note.clone()))
        .collect::<std::collections::BTreeMap<_, _>>();
    // The manager always lists these two, with curated notes.
    for id in [BUILTIN_SOURCE_ID, "manual_override"] {
        assert!(
            notes.get(id).is_some_and(|note| !note.is_empty()),
            "missing availability note for {id}: {notes:?}"
        );
    }

    // The curated official-provider descriptors and notes survive even with no
    // adapter wired, which is what Go's DefaultRegistry-seeded manager shows.
    let curated = jftrade_calendar::default_source_descriptors();
    for id in [
        "hk_gov_1823_ical",
        "mainland_official_notice",
        "nyse_official",
        "nasdaq_verifier",
    ] {
        assert!(
            curated.iter().any(|descriptor| descriptor.id == id),
            "missing curated descriptor for {id}"
        );
        assert!(
            !jftrade_calendar::source_availability_note(id).is_empty(),
            "missing curated availability note for {id}"
        );
    }
    assert!(
        jftrade_calendar::source_availability_note("hk_gov_1823_ical").contains("iCal"),
        "the HK note describes the provider"
    );
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:779
/// TestManagerStatusExplainsBuiltinEffectiveReason.
///
/// When a market enables only builtin rules, the status must say exactly why
/// builtin is serving it: no external source is enabled for this market. That
/// wording is what separates a configured offline policy from an outage.
#[test]
fn status_explains_why_builtin_rules_serve_a_market() {
    let policy = CalendarManagerSettings {
        auto_refresh_enabled: false,
        refresh_interval_hours: 24,
        warmup_markets: vec!["CN".to_owned()],
        source_policies: vec![CalendarSourcePolicy {
            market: "CN".to_owned(),
            enabled_source_ids: vec![BUILTIN_SOURCE_ID.to_owned()],
            fallback_to_builtin: true,
            ..CalendarSourcePolicy::default()
        }],
        ..CalendarManagerSettings::default()
    };
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-20T12:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = CalendarManager::with_clock(
        CalendarSourceRegistry::default(),
        None,
        policy,
        Arc::new(move || *now.lock().expect("fixture clock")),
    )
    .expect("create manager");
    manager.start().expect("start manager");

    let status = manager.status_snapshot().expect("status");
    assert_eq!(status.markets.len(), 1);
    let market = &status.markets[0];
    assert_eq!(market.effective_source, BUILTIN_SOURCE_ID);
    assert_eq!(market.effective_mode, "builtin_fallback");
    assert_eq!(
        market.effective_reason,
        "current policy uses builtin_rules because no external source is enabled for this market"
    );
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:811
/// TestManagerStatusUsesRemoteCoverageSourceForRegularDay.
///
/// A fresh remote snapshot that covers the checked day but carries no special
/// schedule for it makes the provider the *coverage* source: the mode is
/// `remote_covered_day` and the reason spells out that the builtin template is
/// still supplying the standard session result.
#[test]
fn status_credits_the_covering_provider_on_a_regular_day() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let mut hk = FixtureSource::new("hk_gov_1823_ical", events);
    hk.descriptor_markets = vec!["HK".to_owned()];
    let source = Arc::new(hk);
    source.push(Ok(CalendarSnapshot {
        market_code: "HK".to_owned(),
        source_id: "hk_gov_1823_ical".to_owned(),
        from: timestamp("2026-01-01T00:00:00+08:00"),
        to: timestamp("2027-12-31T23:59:59+08:00"),
        schedules: vec![TradingDaySchedule {
            market_code: "HK".to_owned(),
            date: timestamp("2026-06-19T00:00:00+08:00"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: "tuen_ng_festival".to_owned(),
            source_id: "hk_gov_1823_ical".to_owned(),
            observed: false,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-06-19T17:10:34Z"),
        valid_until: timestamp("2026-07-19T17:10:34Z"),
        checksum: String::new(),
    }));
    let mut policy = settings("hk_gov_1823_ical");
    policy.warmup_markets = vec!["HK".to_owned()];
    policy.source_policies[0].market = "HK".to_owned();
    policy.source_policies[0].stale_after_hours = 168;
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-19T17:19:42Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, policy, now);
    manager.start().expect("start manager");
    manager.refresh_all().expect("refresh all");

    let status = manager.status_snapshot().expect("status");
    assert_eq!(status.markets.len(), 1);
    let market = &status.markets[0];
    assert_eq!(market.effective_source, "hk_gov_1823_ical");
    assert_eq!(market.effective_mode, "remote_covered_day");
    assert_eq!(
        market.effective_reason,
        "a fresh source snapshot covers the checked trading day; builtin template supplies the standard session result because that date has no special override"
    );
    // The provider's own holiday is still the answer for that date.
    let schedule = manager
        .schedule("HK", timestamp("2026-06-19T12:00:00+08:00"))
        .expect("remote schedule")
        .expect("schedule");
    assert_eq!(schedule.source_id, "hk_gov_1823_ical");
    assert_eq!(schedule.status, "closed");
    assert_eq!(schedule.reason, "tuen_ng_festival");
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:917
/// TestManagerCurrentTimeNormalizesInjectedClockToUTC.
///
/// The injected clock is always read through the manager's UTC-normalizing
/// accessor: an in-process clock in UTC+8 at 09:30 must be reported as 01:30Z,
/// so persisted and projected timestamps do not depend on the host locale.
#[test]
fn injected_clock_is_normalized_to_utc_for_projected_timestamps() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("official", events));
    // 09:30 in a UTC+8 clock is 01:30Z on the same date.
    let local = OffsetDateTime::parse(
        "2026-06-20T09:30:00+08:00",
        &time::format_description::well_known::Rfc3339,
    )
    .expect("clock");
    let manager = build_manager(
        source,
        None,
        settings("official"),
        Arc::new(Mutex::new(local)),
    );
    manager.start().expect("start manager");
    manager.refresh_market("US").expect("refresh");

    let status = manager.status_snapshot().expect("status");
    assert_eq!(
        status.markets[0].checked_at, "2026-06-20T01:30:00Z",
        "the projected instant is UTC-normalized"
    );
    let source_row = manager
        .source_statuses()
        .expect("source statuses")
        .into_iter()
        .find(|status| status.source_id == "official")
        .expect("provider status");
    assert_eq!(
        source_row.last_failure_at.as_deref(),
        Some("2026-06-20T01:30:00Z"),
        "recorded failure instants are UTC-normalized too"
    );
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_test.go:890
/// TestSnapshotCacheIndexesEveryCoveredMarketYear.
///
/// A snapshot spanning 2026-01-01..2027-12-31 must answer a day in *either*
/// year, because Go indexes one cache entry per covered market-local year, and
/// the status projection must still report it as one logical snapshot rather
/// than one row per year.
#[test]
fn cross_year_snapshot_is_cached_for_every_covered_year_and_summarised_once() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("cross-year", events));
    source.push(Ok(CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: "cross-year".to_owned(),
        from: timestamp("2026-01-01T00:00:00-05:00"),
        to: timestamp("2027-12-31T23:59:59-05:00"),
        schedules: vec![
            TradingDaySchedule {
                market_code: "US".to_owned(),
                date: timestamp("2026-06-19T00:00:00-04:00"),
                status: "closed".to_owned(),
                sessions: Vec::new(),
                reason: "first-year".to_owned(),
                source_id: "cross-year".to_owned(),
                observed: false,
                updated_at: None,
            },
            TradingDaySchedule {
                market_code: "US".to_owned(),
                date: timestamp("2027-01-02T00:00:00-05:00"),
                status: "closed".to_owned(),
                sessions: Vec::new(),
                reason: "second-year".to_owned(),
                source_id: "cross-year".to_owned(),
                observed: false,
                updated_at: None,
            },
        ],
        fetched_at: timestamp("2026-06-19T12:00:00Z"),
        valid_until: timestamp("2027-12-31T23:59:59Z"),
        checksum: "cross-year".to_owned(),
    }));
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-06-19T12:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, settings("cross-year"), now);
    manager.start().expect("start manager");
    manager.refresh_all().expect("refresh all");

    for (query, reason) in [
        ("2026-06-19T14:00:00Z", "first-year"),
        ("2027-01-02T14:00:00Z", "second-year"),
    ] {
        let schedule = manager
            .schedule("US", timestamp(query))
            .expect("cross-year schedule")
            .expect("schedule");
        assert_eq!(
            schedule.source_id, "cross-year",
            "the snapshot must answer {query}"
        );
        assert_eq!(schedule.reason, reason);
    }

    // One logical snapshot, not one row per indexed year.
    let status = manager.status_snapshot().expect("status");
    assert_eq!(
        status.snapshots.len(),
        1,
        "snapshot summaries = {:?}",
        status.snapshots
    );
    assert_eq!(status.snapshots[0].checksum, "cross-year");
    assert_eq!(status.snapshots[0].schedules_parsed, 2);
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_probe_test.go:14
/// TestManagerProbeMarketWarmupAndSnapshotOrdering.
///
/// Three properties in one Go case: `ProbeMarket("US")` touches only the
/// requested market, the warmup refresh follows the configured warmup markets,
/// and the status projection orders its snapshot rows by market-local sort key
/// (HK before US) rather than by insertion order.
#[test]
fn probe_targets_one_market_while_warmup_follows_settings() {
    let fetch_log = Arc::new(Mutex::new(Vec::new()));
    let mut registry = CalendarSourceRegistry::default();
    registry
        .register(Arc::new(MarketRecordingSource::new(
            "nyse_official",
            vec!["US", "HK"],
            Arc::clone(&fetch_log),
        )))
        .expect("register source");
    let policy = CalendarManagerSettings {
        // Auto refresh stays off so only the explicit calls below fetch; the
        // background loop would otherwise add its own warmup pass.
        auto_refresh_enabled: false,
        refresh_interval_hours: 24,
        // Warmup only covers HK, so a targeted US probe must not warm HK up.
        warmup_markets: vec!["HK".to_owned()],
        source_policies: vec![
            CalendarSourcePolicy {
                market: "US".to_owned(),
                preferred_source_ids: vec!["nyse_official".to_owned()],
                enabled_source_ids: vec!["nyse_official".to_owned()],
                fallback_to_builtin: true,
                stale_after_hours: 24,
                ..CalendarSourcePolicy::default()
            },
            CalendarSourcePolicy {
                market: "HK".to_owned(),
                preferred_source_ids: vec!["nyse_official".to_owned()],
                enabled_source_ids: vec!["nyse_official".to_owned()],
                fallback_to_builtin: true,
                stale_after_hours: 24,
                ..CalendarSourcePolicy::default()
            },
        ],
        ..CalendarManagerSettings::default()
    };
    let manager = CalendarManager::new(registry, None, policy).expect("create manager");
    manager.start().expect("start manager");

    let probe = manager.probe_market("US").expect("probe US");
    assert_eq!(probe.market, "US");
    assert_eq!((probe.healthy, probe.failures), (1, 0));
    assert_eq!(
        fetch_log.lock().expect("fetch log").clone(),
        vec!["US".to_owned()],
        "a targeted probe must not touch other markets"
    );

    // Warmup follows settings, so HK is fetched there and the US row comes only
    // from an explicit targeted refresh (a probe never caches a snapshot).
    manager.refresh_all().expect("warmup refresh");
    manager.refresh_market("US").expect("refresh US");
    assert_eq!(
        fetch_log.lock().expect("fetch log").clone(),
        vec!["US".to_owned(), "HK".to_owned(), "US".to_owned()],
        "the warmup refresh follows the configured warmup markets"
    );

    // Snapshot rows are ordered by the market-local sort key and the builtin
    // source is filtered out, so the HK row sorts before the US row regardless
    // of the fetch order. Both markets still have to appear.
    let summaries = manager.status_snapshot().expect("status").snapshots;
    let markets = summaries
        .iter()
        .map(|summary| summary.market.clone())
        .collect::<Vec<_>>();
    assert_eq!(
        markets,
        vec!["HK".to_owned(), "US".to_owned()],
        "snapshot summaries sort HK before US: {summaries:?}"
    );
    assert!(
        summaries
            .iter()
            .all(|summary| summary.source_id != BUILTIN_SOURCE_ID),
        "builtin snapshots are never summarised: {summaries:?}"
    );
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/source_health_status_test.go:17
/// TestRefreshAndProbeKeepPerSourceHealthTruthful.
///
/// With three providers — one network failure, one structure failure and one
/// healthy — a refresh must keep the usable source while making both failures
/// visible, and a probe must report the same split. An unsupported operator
/// target is a no-op for both entry points rather than a success for an
/// unrelated market.
#[test]
fn refresh_and_probe_keep_per_source_health_truthful() {
    let mut failing = FixtureSource::new(
        "coverage98-network-failure",
        Arc::new(Mutex::new(Vec::new())),
    );
    failing.descriptor_markets = vec!["US".to_owned()];
    failing.push(Err(CalendarSourceError::Failed(
        "official endpoint unavailable".to_owned(),
    )));
    failing.push(Err(CalendarSourceError::Failed(
        "official endpoint unavailable".to_owned(),
    )));
    let network = Arc::new(failing);

    // A well-formed answer with zero schedules is a structure failure.
    let mut structural = FixtureSource::new(
        "coverage98-structure-failure",
        Arc::new(Mutex::new(Vec::new())),
    );
    structural.descriptor_markets = vec!["US".to_owned()];
    structural.push(Ok(CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: "coverage98-structure-failure".to_owned(),
        from: timestamp("2026-01-01T00:00:00Z"),
        to: timestamp("2027-12-31T23:59:59Z"),
        schedules: Vec::new(),
        fetched_at: timestamp("2026-07-02T16:00:00Z"),
        valid_until: timestamp("2026-07-03T16:00:00Z"),
        checksum: String::new(),
    }));
    structural.push(Ok(CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: "coverage98-structure-failure".to_owned(),
        from: timestamp("2026-01-01T00:00:00Z"),
        to: timestamp("2027-12-31T23:59:59Z"),
        schedules: Vec::new(),
        fetched_at: timestamp("2026-07-02T16:00:00Z"),
        valid_until: timestamp("2026-07-03T16:00:00Z"),
        checksum: String::new(),
    }));
    let structure = Arc::new(structural);

    let mut healthy = FixtureSource::new("coverage98-official", Arc::new(Mutex::new(Vec::new())));
    healthy.descriptor_markets = vec!["US".to_owned()];
    for _ in 0..2 {
        healthy.push(Ok(CalendarSnapshot {
            market_code: "US".to_owned(),
            source_id: "coverage98-official".to_owned(),
            from: timestamp("2026-01-01T00:00:00Z"),
            to: timestamp("2027-12-31T23:59:59Z"),
            schedules: vec![TradingDaySchedule {
                market_code: "US".to_owned(),
                // 16:00Z is the US local trading day 2026-07-02; UTC midnight
                // would land on the previous local date.
                date: timestamp("2026-07-02T16:00:00Z"),
                status: "closed".to_owned(),
                sessions: Vec::new(),
                reason: "official test closure".to_owned(),
                source_id: "coverage98-official".to_owned(),
                observed: false,
                updated_at: None,
            }],
            fetched_at: timestamp("2026-07-02T16:00:00Z"),
            valid_until: timestamp("2026-07-03T16:00:00Z"),
            checksum: "coverage98-checksum".to_owned(),
        }));
    }
    let official = Arc::new(healthy);

    let mut registry = CalendarSourceRegistry::default();
    for source in [
        Arc::clone(&network) as Arc<dyn CalendarSourcePort>,
        Arc::clone(&structure) as Arc<dyn CalendarSourcePort>,
        Arc::clone(&official) as Arc<dyn CalendarSourcePort>,
    ] {
        registry.register(source).expect("register provider");
    }
    let policy = CalendarManagerSettings {
        refresh_interval_hours: 24,
        warmup_markets: vec!["US".to_owned()],
        source_policies: vec![CalendarSourcePolicy {
            market: "US".to_owned(),
            preferred_source_ids: vec![
                "coverage98-network-failure".to_owned(),
                "coverage98-structure-failure".to_owned(),
                "coverage98-official".to_owned(),
            ],
            enabled_source_ids: vec![
                "coverage98-network-failure".to_owned(),
                "coverage98-structure-failure".to_owned(),
                "coverage98-official".to_owned(),
            ],
            fallback_to_builtin: true,
            ..CalendarSourcePolicy::default()
        }],
        ..CalendarManagerSettings::default()
    };
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-07-02T16:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = CalendarManager::with_clock(
        registry,
        None,
        policy,
        Arc::new(move || *now.lock().expect("fixture clock")),
    )
    .expect("create manager");
    manager.start().expect("start manager");

    // The usable source survives while both failures stay visible.
    let refresh = manager.refresh_market("US").expect("refresh");
    assert_eq!(
        (refresh.updated, refresh.failures),
        (1, 2),
        "one healthy provider and two failures"
    );
    let status = |manager: &CalendarManager, id: &str| {
        manager
            .source_statuses()
            .expect("source statuses")
            .into_iter()
            .find(|status| status.source_id == id)
            .expect("provider status")
    };
    for id in ["coverage98-network-failure", "coverage98-structure-failure"] {
        let failed = status(&manager, id);
        assert!(
            !failed.last_error.is_empty(),
            "{id} must record its failure: {failed:?}"
        );
        assert_eq!(failed.consecutive_failures, 1, "{id}");
        assert_eq!(failed.health_state, "unhealthy", "{id}");
    }
    let good = status(&manager, "coverage98-official");
    assert!(good.last_success_at.is_some(), "healthy provider: {good:?}");
    assert!(good.last_snapshot_fetched_at.is_some());
    assert_eq!(good.health_state, "healthy");
    // The healthy provider is the one actually serving the day.
    let schedule = manager
        .schedule("US", timestamp("2026-07-02T16:00:00Z"))
        .expect("remote schedule")
        .expect("schedule");
    assert_eq!(schedule.source_id, "coverage98-official");
    assert_eq!(schedule.status, "closed");

    // The probe reports the same provider-level split.
    let probe = manager.probe_market("US").expect("probe US");
    assert_eq!((probe.healthy, probe.failures), (1, 2));
    assert_eq!(probe.results.len(), 3);
    for result in &probe.results {
        let expected = if result.source_id == "coverage98-official" {
            "healthy"
        } else {
            "unhealthy"
        };
        assert_eq!(result.status, expected, "probe row {result:?}");
    }

    // An unsupported target is a no-op, not a success for another market.
    let unknown_refresh = manager.refresh_market("MARS").expect("unknown refresh");
    assert!(unknown_refresh.accepted);
    assert_eq!(unknown_refresh.market, "MARS");
    assert_eq!((unknown_refresh.updated, unknown_refresh.failures), (0, 0));
    let unknown_probe = manager.probe_market("MARS").expect("unknown probe");
    assert!(unknown_probe.accepted);
    assert_eq!(unknown_probe.market, "MARS");
    assert_eq!((unknown_probe.healthy, unknown_probe.failures), (0, 0));
    assert!(unknown_probe.results.is_empty());
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/source_health_status_test.go:98
/// TestStatusDistinguishesRemoteCoverageFromRemoteOverride.
///
/// A fresh snapshot that covers the checked day but carries its special schedule
/// on a *different* covered day must report `remote_covered_day`, not
/// `remote_override`: the provider is credited with coverage while the builtin
/// template keeps answering the ordinary day. Go deliberately places the early
/// close one day after `now` to pin exactly that distinction.
#[test]
fn fresh_snapshot_without_a_special_day_is_coverage_not_override() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("coverage98-covered", events));
    source.push(Ok(CalendarSnapshot {
        market_code: "US".to_owned(),
        source_id: "coverage98-covered".to_owned(),
        from: timestamp("2026-06-06T00:00:00Z"),
        to: timestamp("2026-08-06T00:00:00Z"),
        // The only special schedule sits on 2026-07-07, one day after the
        // checked day, so it must not override 2026-07-06.
        schedules: vec![TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp("2026-07-07T16:00:00Z"),
            status: "early_close".to_owned(),
            sessions: Vec::new(),
            reason: String::new(),
            source_id: "coverage98-covered".to_owned(),
            observed: false,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-07-06T13:00:00Z"),
        valid_until: timestamp("2026-07-06T15:00:00Z"),
        checksum: String::new(),
    }));
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-07-06T14:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, settings("coverage98-covered"), now);
    manager.start().expect("start manager");
    manager.refresh_all().expect("refresh all");

    let status = manager.status_snapshot().expect("status");
    assert_eq!(status.markets.len(), 1);
    let market = &status.markets[0];
    assert_eq!(market.effective_source, "coverage98-covered");
    assert_eq!(
        market.effective_mode, "remote_covered_day",
        "a fresh snapshot without a special day only supplies coverage"
    );
    assert_eq!(
        market.effective_reason,
        "a fresh source snapshot covers the checked trading day; builtin template supplies the standard session result because that date has no special override"
    );
    // The checked day itself still resolves to the builtin session.
    let schedule = manager
        .schedule("US", timestamp("2026-07-06T14:00:00Z"))
        .expect("schedule")
        .expect("builtin session");
    assert_eq!(
        schedule.source_id, BUILTIN_SOURCE_ID,
        "the off-day early close must not leak onto the checked day"
    );
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:229
/// TestCalendarSourceAlertLifecycleRecordsFailuresDeduplicatesAndRecovers.
///
/// Go drives the recording helpers directly (`recordOperationFailure`,
/// `recordSourceFailure`, `recordProbeFailure`, `recordProbeSuccess`,
/// `recordSuccess`) and asserts three things at once: an operation failure only
/// grows the retry ladder, a repeated provider fault is deduplicated against a
/// single alert, and a later success records a `recovered` alert carrying the
/// previous fingerprint. The recording helpers are private in Rust, so the same
/// lifecycle is driven through the public refresh/probe surface and read back
/// from the source status, which is the projection Go's alert sink feeds.
#[test]
fn source_alert_lifecycle_records_failures_deduplicates_and_recovers() {
    let events = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(FixtureSource::new("remote-source", Arc::clone(&events)));
    for _ in 0..2 {
        source.push(Err(CalendarSourceError::Failed(
            "disk unavailable".to_owned(),
        )));
    }
    source.push(Err(CalendarSourceError::Failed(
        "disk unavailable".to_owned(),
    )));
    let now = Arc::new(Mutex::new(
        OffsetDateTime::parse(
            "2026-07-02T08:00:00Z",
            &time::format_description::well_known::Rfc3339,
        )
        .expect("clock"),
    ));
    let manager = build_manager(source, None, settings("remote-source"), Arc::clone(&now));
    manager.start().expect("start manager");

    let status = |manager: &CalendarManager| {
        manager
            .source_statuses()
            .expect("source statuses")
            .into_iter()
            .find(|status| status.source_id == "remote-source")
            .expect("provider status")
    };

    // The first provider fault opens the retry ladder at one hour and raises one
    // alert.
    assert_eq!(
        manager
            .refresh_market("US")
            .expect("first refresh")
            .failures,
        1
    );
    let first = status(&manager);
    assert_eq!(first.consecutive_failures, 1);
    assert_eq!(first.last_error, "disk unavailable");
    assert_eq!(
        first.next_refresh_at.as_deref(),
        Some("2026-07-02T09:00:00Z"),
        "the retry ladder starts one hour after the manager clock"
    );
    assert_eq!(first.health_state, "unhealthy");
    assert_eq!(first.last_alert_status, "triggered");
    let first_alert_at = first.last_alert_at.clone();
    let fingerprint = first.health_fingerprint.clone();

    // An identical repeat is deduplicated: the state stays, the alert instant
    // does not move.
    *now.lock().expect("fixture clock") += Duration::hours(2);
    assert_eq!(
        manager
            .refresh_market("US")
            .expect("repeat refresh")
            .failures,
        1
    );
    let repeated = status(&manager);
    assert_eq!(repeated.consecutive_failures, 2);
    assert_eq!(
        repeated.health_fingerprint, fingerprint,
        "same fault identity"
    );
    assert_eq!(
        repeated.last_alert_status, "triggered",
        "a repeated identical fault does not invent a new alert state"
    );
    assert_eq!(repeated.last_alert_at, first_alert_at);

    // A probe that recovers the provider clears the failure state and records
    // the previous fingerprint on the recovery alert.
    let recovering = Arc::new(FixtureSource::new("probe-source", Arc::clone(&events)));
    recovering.push(Err(CalendarSourceError::Failed(
        "context deadline exceeded".to_owned(),
    )));
    recovering.push(Ok(snapshot("probe-source", "recovered")));
    let mut registry = CalendarSourceRegistry::default();
    registry
        .register(recovering)
        .expect("register probe source");
    registry
        .register(Arc::new(FixtureSource::new(
            "remote-source",
            Arc::clone(&events),
        )))
        .expect("register refresh source");
    let mut probe_settings = settings("remote-source");
    probe_settings.source_policies[0]
        .enabled_source_ids
        .push("probe-source".to_owned());
    let probe_manager = CalendarManager::with_clock(
        registry,
        None,
        probe_settings,
        Arc::new({
            let now = Arc::clone(&now);
            move || *now.lock().expect("fixture clock")
        }),
    )
    .expect("create probe manager");
    probe_manager.start().expect("start probe manager");

    let failed = probe_manager.probe_market("US").expect("failing probe");
    assert_eq!(failed.failures, 2, "both providers fail this round");
    let probe_status = probe_manager
        .source_statuses()
        .expect("probe statuses")
        .into_iter()
        .find(|status| status.source_id == "probe-source")
        .expect("probe status");
    assert_eq!(probe_status.last_probe_status, "unhealthy");
    assert_eq!(probe_status.health_state, "unhealthy");
    assert_eq!(
        probe_status.health_fingerprint,
        "probe-source|US|fetch_failed|network_timeout_or_cancelled",
        "a probe timeout shares the refresh fingerprint vocabulary"
    );

    let success = probe_manager.probe_market("US").expect("recovering probe");
    assert_eq!((success.healthy, success.failures), (1, 1));
    let recovered = probe_manager
        .source_statuses()
        .expect("probe statuses")
        .into_iter()
        .find(|status| status.source_id == "probe-source")
        .expect("probe status");
    assert_eq!(recovered.last_probe_status, "healthy");
    assert_eq!(recovered.health_state, "healthy");
    assert_eq!(recovered.last_probe_schedules, 1);
    assert_eq!(
        recovered.last_alert_status, "recovered",
        "a recovered provider records the recovery alert"
    );
    assert_eq!(
        recovered.last_alert_fingerprint,
        "probe-source|US|fetch_failed|network_timeout_or_cancelled",
        "the recovery alert carries the fingerprint it recovered from"
    );
    probe_manager.close().expect("close probe manager");
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/http_source_boundaries_test.go:298
/// TestNilCalendarManagerOperationsRemainSafeDuringStartupAndShutdown.
///
/// Go's nil-receiver guards make `recordSuccess`/`record*Failure`/`settings`
/// safe on a `(*Manager)(nil)` and fall back to `time.Now().UTC()` when no clock
/// is installed. Rust has no nil receiver: `CalendarManager::new` always builds
/// a clock, and a manager with an empty registry and no clock injection must
/// still answer settings, time and lifecycle questions and stay safe to start,
/// probe and close.
#[test]
fn manager_without_configured_sources_stays_safe_across_its_lifecycle() {
    let manager = CalendarManager::new(
        CalendarSourceRegistry::default(),
        None,
        CalendarManagerSettings::default(),
    )
    .expect("create manager without sources or persistence");

    // Equivalent of `settings()` on a nil manager: default policy, no panic.
    // The projection always publishes the two local owners so an operator can
    // see that builtin/manual coverage exists even with no remote provider
    // configured; nothing is enabled and no health state was invented.
    let snapshot = manager.status_snapshot().expect("status snapshot");
    assert_eq!(snapshot.refresh_interval_hours, 0);
    assert!(!snapshot.auto_refresh_enabled);
    assert_eq!(
        snapshot
            .sources
            .iter()
            .filter(|source| source.enabled)
            .map(|source| source.id.as_str())
            .collect::<Vec<_>>(),
        [BUILTIN_SOURCE_ID, MANUAL_OVERRIDE_SOURCE_ID],
        "only the local owners are enabled without policy"
    );
    assert!(
        snapshot
            .sources
            .iter()
            .all(|source| source.health_state.is_empty()),
        "recording helpers must not invent health state: {:?}",
        snapshot.sources
    );

    // Equivalent of `currentTime()` with no clock: the projection stamps a real
    // instant, since `CalendarManager::new` always installs a UTC clock.
    assert!(
        snapshot
            .markets
            .iter()
            .all(|market| market.checked_at.starts_with("20")),
        "an unconfigured manager still reports a usable clock: {:?}",
        snapshot.markets
    );

    // Equivalent of the nil-safe recording helpers: every entry point is a
    // no-op rather than a panic, and the lifecycle stays consistent.
    manager.start().expect("start without sources");
    assert_eq!(
        manager.lifecycle_state().expect("lifecycle"),
        ManagerLifecycleState::Running
    );
    let refresh = manager.refresh_all().expect("refresh without sources");
    let probe = manager.probe_all().expect("probe without sources");
    assert_eq!((refresh.updated, refresh.failures), (0, 0));
    assert_eq!((probe.healthy, probe.failures), (0, 0));
    assert!(manager.source_statuses().expect("statuses").is_empty());
    manager.close().expect("close");
    assert_eq!(
        manager.lifecycle_state().expect("lifecycle"),
        ManagerLifecycleState::Closed
    );
    manager.close().expect("repeat close stays a no-op");
}
