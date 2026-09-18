//! Boundary coverage for `internal/exchangecalendar/manager_boundaries_test.go`.
//!
//! The Go file pins narrow decode/validation edges rather than lifecycle
//! flows: nil-receiver safety, manual-override normalization, cached-snapshot
//! validation, registry normalization and request-target aliasing. Each test
//! names the Go case it answers.

use std::collections::BTreeMap;
use std::str::FromStr;
use std::sync::{Arc, Mutex};

use jftrade_calendar::{
    BUILTIN_SOURCE_ID, CalendarCancellationToken, CalendarManager, CalendarManagerSettings,
    CalendarManualOverride, CalendarSessionOverride, CalendarSnapshot, CalendarSourceDescriptor,
    CalendarSourceError, CalendarSourcePolicy, CalendarSourcePort, CalendarSourceRegistry,
    TradingDaySchedule, default_source_descriptors, normalize_source_ids, project_default_sources,
    source_availability_note,
};
use jftrade_kernel::WireTimestamp;

fn timestamp(value: &str) -> WireTimestamp {
    WireTimestamp::from_str(value).expect("valid fixture timestamp")
}

fn manual_override(market: &str, date: &str, status: &str) -> CalendarManualOverride {
    CalendarManualOverride {
        market: market.to_owned(),
        date: date.to_owned(),
        status: status.to_owned(),
        sessions: Vec::new(),
        reason: "fixture override".to_owned(),
        observed: false,
    }
}

fn schedule_for(settings: CalendarManagerSettings, market: &str, at: &str) -> TradingDaySchedule {
    let manager = CalendarManager::new(CalendarSourceRegistry::default(), None, settings)
        .expect("create calendar manager");
    manager.start().expect("start manager");
    let schedule = manager
        .schedule(market, timestamp(at))
        .expect("resolve schedule")
        .expect("a supported market always resolves a schedule");
    manager.close().expect("close manager");
    schedule
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:50
/// TestManualOverrideStatusAndSessionBoundaries.
///
/// The manual status decoder is case-insensitive and maps unknown text to
/// `unknown`; zero-width session windows are dropped while surviving windows
/// keep their declared kinds; and a `CN` override applies to both `SH` and `SZ`
/// with `observed` preserved.
#[test]
fn manual_override_status_and_session_window_boundaries_match_go() {
    for (raw, expected) in [
        (" open ", "open"),
        ("CLOSED", "closed"),
        ("early_close", "early_close"),
        ("special", "special"),
        ("not-a-status", "unknown"),
    ] {
        let settings = CalendarManagerSettings {
            manual_overrides: vec![manual_override("US", "2026-07-02", raw)],
            ..CalendarManagerSettings::default()
        };
        let schedule = schedule_for(settings, "US", "2026-07-02T10:00:00-04:00");
        assert_eq!(schedule.status, expected, "manual status {raw:?}");
        assert_eq!(schedule.source_id, "manual_override");
    }

    // Go runs the windows through `NormalizeSessions`, which drops invalid
    // zero-width windows and keeps the declared kinds.
    let mut settings = CalendarManagerSettings {
        manual_overrides: vec![manual_override("US", "2026-07-02", "open")],
        ..CalendarManagerSettings::default()
    };
    settings.manual_overrides[0].sessions = vec![
        CalendarSessionOverride {
            kind: "regular".to_owned(),
            start_minute: 570,
            end_minute: 960,
        },
        CalendarSessionOverride {
            kind: "pre".to_owned(),
            start_minute: 240,
            end_minute: 570,
        },
        // Zero width: must not survive normalization.
        CalendarSessionOverride {
            kind: "regular".to_owned(),
            start_minute: 700,
            end_minute: 700,
        },
        CalendarSessionOverride {
            kind: "mystery".to_owned(),
            start_minute: 960,
            end_minute: 1200,
        },
    ];
    let schedule = schedule_for(settings, "US", "2026-07-02T10:00:00-04:00");
    let windows = schedule
        .sessions
        .iter()
        .map(|session| {
            (
                session.kind.as_str(),
                session.start_minute,
                session.end_minute,
            )
        })
        .collect::<Vec<_>>();
    assert_eq!(
        windows,
        vec![
            ("pre", 240, 570),
            ("regular", 570, 960),
            ("unknown", 960, 1200)
        ],
        "zero-width windows are dropped and windows stay ordered"
    );

    for market in ["SH", "SZ"] {
        let mut settings = CalendarManagerSettings {
            manual_overrides: vec![manual_override("CN", "2026-10-01", "special")],
            ..CalendarManagerSettings::default()
        };
        settings.manual_overrides[0].observed = true;
        settings.manual_overrides[0].reason = "national_day_override".to_owned();
        settings.manual_overrides[0].sessions = vec![CalendarSessionOverride {
            kind: "regular".to_owned(),
            start_minute: 570,
            end_minute: 690,
        }];
        let schedule = schedule_for(settings, market, "2026-10-01T12:00:00+08:00");
        assert_eq!(schedule.market_code, market);
        assert_eq!(schedule.status, "special");
        assert_eq!(schedule.reason, "national_day_override");
        assert_eq!(schedule.source_id, "manual_override");
        assert!(schedule.observed, "{market} override keeps observed=true");
        assert_eq!(schedule.sessions.len(), 1);
        assert_eq!(schedule.sessions[0].kind, "regular");
    }
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:146
/// TestCalendarSourceAvailabilityNotesAndRefreshTargets.
///
/// Every curated provider keeps a non-empty operator note and an unknown
/// provider keeps none; a refresh target folds onto its owner market, so blank,
/// CN, SH and SZ all reach the CN provider while HK does not.
#[test]
fn source_availability_notes_and_refresh_target_folding_match_go() {
    for source_id in [
        "nyse_official",
        "nasdaq_verifier",
        "hk_gov_1823_ical",
        "mainland_official_notice",
        BUILTIN_SOURCE_ID,
        "manual_override",
    ] {
        assert!(
            !source_availability_note(source_id).trim().is_empty(),
            "availability note for {source_id:?} is empty"
        );
    }
    assert_eq!(source_availability_note("custom_source"), "");

    let curated = default_source_descriptors();
    assert!(
        curated
            .iter()
            .all(|descriptor| !source_availability_note(&descriptor.id).is_empty()),
        "every curated descriptor carries a note"
    );

    // Rust's owner for `refreshMarketsForTarget` is the target folding plus the
    // policy lookup: a refresh for `CN`, `SH` or `SZ` must reach the provider
    // that declares `CN`, and the same provider must not answer HK.
    let seen = Arc::new(Mutex::new(Vec::new()));
    let source = Arc::new(StubSource::new("mainland", &["CN"]));
    source.observe(Arc::clone(&seen));
    for (target, expected) in [("CN", "CN"), ("SH", "CN"), ("SZ", "CN"), ("HK", "HK")] {
        source.push(Ok(snapshot_for(expected)));
        let mut registry = CalendarSourceRegistry::default();
        registry
            .register(Arc::clone(&source) as Arc<dyn CalendarSourcePort>)
            .expect("register provider");
        let settings = CalendarManagerSettings {
            warmup_markets: vec![target.to_owned()],
            source_policies: vec![CalendarSourcePolicy {
                market: "CN".to_owned(),
                preferred_source_ids: vec!["mainland".to_owned()],
                enabled_source_ids: vec!["mainland".to_owned()],
                fallback_to_builtin: true,
                ..CalendarSourcePolicy::default()
            }],
            ..CalendarManagerSettings::default()
        };
        let manager = CalendarManager::new(registry, None, settings).expect("create manager");
        manager.start().expect("start manager");
        let result = manager.refresh_market(target).expect("refresh target");
        assert_eq!(
            result.market, target,
            "the reported market stays the requested target"
        );
        manager.close().expect("close manager");
    }
    let fetched = seen.lock().expect("fetch log").clone();
    assert_eq!(
        fetched,
        vec!["CN".to_owned(), "CN".to_owned(), "CN".to_owned()],
        "CN/SH/SZ fold onto the CN owner while HK never reaches the mainland provider"
    );
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:172
/// TestManagerValidateCachedSnapshotRejectsCorruptSnapshots.
///
/// Go's table rejects seven corrupt cached snapshots and accepts the valid one.
/// Restore is the only path that reaches the validator from the public surface,
/// so this test pins the observable effects (nothing cached, builtin fallback,
/// the valid value served) while the condition-by-condition reasons live in the
/// owner's unit test in `manager_calendar.rs`.
#[test]
fn corrupt_cached_snapshots_are_rejected_with_the_go_conditions() {
    // US snapshots carry exchange-local offsets, so both range bounds and the
    // schedule date land on the intended New York trading day.
    let from = timestamp("2026-01-01T00:00:00-05:00");
    let to = timestamp("2026-12-31T23:59:59-05:00");
    let valid = CalendarSnapshot {
        source_id: "cache_probe".to_owned(),
        market_code: "US".to_owned(),
        from,
        to,
        schedules: vec![TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp("2026-07-03T00:00:00-04:00"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: "independence_day".to_owned(),
            source_id: "cache_probe".to_owned(),
            observed: true,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-01-02T00:00:00Z"),
        valid_until: timestamp("2026-12-31T23:59:59Z"),
        checksum: "cache_probe".to_owned(),
    };

    let cases: Vec<(&str, CalendarSnapshot)> = vec![
        ("missing source", {
            let mut snapshot = valid.clone();
            snapshot.source_id = " ".to_owned();
            snapshot
        }),
        ("missing market", {
            let mut snapshot = valid.clone();
            snapshot.market_code = " ".to_owned();
            snapshot
        }),
        ("unsupported market", {
            let mut snapshot = valid.clone();
            snapshot.market_code = "MARS".to_owned();
            snapshot
        }),
        ("missing range", {
            let mut snapshot = valid.clone();
            snapshot.from = timestamp("0001-01-01T00:00:00Z");
            snapshot
        }),
        ("backward range", {
            let mut snapshot = valid.clone();
            snapshot.from = to;
            snapshot.to = from;
            snapshot
        }),
        ("empty schedule date", {
            let mut snapshot = valid.clone();
            snapshot.schedules = vec![TradingDaySchedule {
                market_code: "US".to_owned(),
                date: timestamp("0001-01-01T00:00:00Z"),
                status: "closed".to_owned(),
                sessions: Vec::new(),
                reason: String::new(),
                source_id: "cache_probe".to_owned(),
                observed: false,
                updated_at: None,
            }];
            snapshot
        }),
        ("schedule outside snapshot range", {
            let mut snapshot = valid.clone();
            snapshot.schedules = vec![TradingDaySchedule {
                market_code: "US".to_owned(),
                date: timestamp("2029-01-01T00:00:00Z"),
                status: "closed".to_owned(),
                sessions: Vec::new(),
                reason: String::new(),
                source_id: "cache_probe".to_owned(),
                observed: false,
                updated_at: None,
            }];
            snapshot
        }),
    ];

    for (label, snapshot) in cases {
        let directory = fixture_directory(label);
        // The store refuses to *write* an invalid identity, so the corrupt
        // fixture is placed on disk the way a crashed/older build left it.
        write_raw_snapshot(&directory, &snapshot);
        let manager = manager_with_store(&directory);
        manager.start().expect("start manager");
        assert!(
            manager.snapshots().expect("snapshots").is_empty(),
            "{label} must not be cached"
        );
        // The rejected value never becomes a served schedule either.
        let fallback = manager
            .schedule("US", timestamp("2026-07-03T14:00:00Z"))
            .expect("schedule")
            .expect("schedule");
        assert_eq!(
            fallback.source_id, BUILTIN_SOURCE_ID,
            "{label} must fall back to builtin rules"
        );
        // The exact rejection reason is pinned by the domain owner's unit test
        // (`manager_calendar::tests::validate_snapshot_rejects_the_go_boundary_table`),
        // which is where Go's direct `validateCachedSnapshot` call maps.
        manager.close().expect("close manager");
        let _ = std::fs::remove_dir_all(&directory);
    }

    let directory = fixture_directory("valid");
    persist(&directory, &valid);
    // A cached snapshot is only served for a policy that enables its source.
    let serving_settings = CalendarManagerSettings {
        warmup_markets: vec!["US".to_owned()],
        source_policies: vec![CalendarSourcePolicy {
            market: "US".to_owned(),
            preferred_source_ids: vec!["cache_probe".to_owned()],
            enabled_source_ids: vec!["cache_probe".to_owned()],
            fallback_to_builtin: true,
            ..CalendarSourcePolicy::default()
        }],
        ..CalendarManagerSettings::default()
    };
    let manager = manager_with_store_and_settings(
        &directory,
        serving_settings,
        timestamp("2026-06-01T00:00:00Z"),
    );
    manager.start().expect("start manager");
    let served = manager
        .schedule("US", timestamp("2026-07-03T14:00:00Z"))
        .expect("schedule")
        .expect("schedule");
    assert_eq!(
        served.source_id, "cache_probe",
        "a valid cached snapshot must be served"
    );
    assert_eq!(served.status, "closed");
    assert_eq!(manager.snapshots().expect("snapshots").len(), 1);
    manager.close().expect("close manager");
    let _ = std::fs::remove_dir_all(&directory);
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:248
/// TestManagerCachedSnapshotMainlandFallbackAndFreshnessBoundaries.
///
/// A cached CN snapshot answers SH/SZ after a restart even while the provider
/// adapter is absent, an expired snapshot must never stay an effective coverage
/// source, and an unknown market has no remote coverage.
#[test]
fn cached_mainland_snapshot_fallback_and_freshness_boundaries_match_go() {
    let now = timestamp("2026-10-01T08:00:00Z");
    // The provider declares CN, so a cached snapshot carries that market code;
    // both mainland templates resolve it through the candidate-market list.
    let shared = CalendarSnapshot {
        source_id: "mainland-cache".to_owned(),
        market_code: "CN".to_owned(),
        from: timestamp("2026-09-01T00:00:00+08:00"),
        to: timestamp("2026-11-01T00:00:00+08:00"),
        schedules: vec![TradingDaySchedule {
            market_code: "CN".to_owned(),
            date: timestamp("2026-10-01T00:00:00+08:00"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: "national_day".to_owned(),
            source_id: "mainland-cache".to_owned(),
            observed: false,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-09-30T09:00:00Z"),
        valid_until: timestamp("2026-10-02T08:00:00Z"),
        checksum: "mainland-cache".to_owned(),
    };
    let settings = CalendarManagerSettings {
        warmup_markets: vec!["CN".to_owned()],
        source_policies: vec![CalendarSourcePolicy {
            market: "CN".to_owned(),
            preferred_source_ids: vec!["mainland-cache".to_owned()],
            enabled_source_ids: vec!["mainland-cache".to_owned()],
            fallback_to_builtin: true,
            stale_after_hours: 24,
            ..CalendarSourcePolicy::default()
        }],
        ..CalendarManagerSettings::default()
    };

    let directory = fixture_directory("mainland-fallback");
    persist(&directory, &shared);
    let manager = manager_with_store_and_settings(&directory, settings.clone(), now);
    manager.start().expect("start manager");
    for market in ["SH", "SZ"] {
        let schedule = manager
            .schedule(market, timestamp("2026-10-01T10:00:00+08:00"))
            .expect("schedule")
            .expect("schedule");
        assert_eq!(schedule.source_id, "mainland-cache", "{market}");
        assert_eq!(schedule.status, "closed", "{market}");
        assert_eq!(schedule.reason, "national_day", "{market}");
        assert_eq!(schedule.market_code, market);
    }
    manager.close().expect("close manager");
    let _ = std::fs::remove_dir_all(&directory);

    let mut expired = shared.clone();
    expired.fetched_at = timestamp("2026-09-01T07:00:00Z");
    expired.valid_until = timestamp("2026-09-02T08:00:00Z");
    expired.schedules[0].reason = "expired_remote".to_owned();
    let directory = fixture_directory("mainland-expired");
    persist(&directory, &expired);
    let manager = manager_with_store_and_settings(&directory, settings, now);
    manager.start().expect("start manager");
    let schedule = manager
        .schedule("SH", timestamp("2026-10-01T10:00:00+08:00"))
        .expect("schedule")
        .expect("schedule");
    assert_eq!(
        schedule.source_id, BUILTIN_SOURCE_ID,
        "an expired snapshot must not stay an effective coverage source"
    );
    assert_ne!(schedule.reason, "expired_remote");
    // A market with no coverage keeps builtin rules rather than borrowing the
    // mainland snapshot.
    let hk = manager
        .schedule("HK", timestamp("2026-10-01T10:00:00+08:00"))
        .expect("HK schedule")
        .expect("schedule");
    assert_eq!(
        hk.source_id, BUILTIN_SOURCE_ID,
        "no remote HK coverage exists"
    );
    manager.close().expect("close manager");
    let _ = std::fs::remove_dir_all(&directory);
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:287
/// TestSourceRegistryNilDuplicateAndMarketNormalizationBoundaries.
///
/// An empty registry answers nothing, a blank identifier is rejected, a repeat
/// registration replaces the adapter while keeping a single descriptor row,
/// markets normalize/deduplicate/sort, and lookup trims the identifier.
#[test]
fn source_registry_normalization_and_duplicate_boundaries_match_go() {
    let empty = CalendarSourceRegistry::default();
    assert!(empty.source("anything").is_none());
    assert!(empty.descriptors().is_empty());

    let mut registry = CalendarSourceRegistry::default();
    registry
        .register(Arc::new(StubSource::new(" ", &["US"])))
        .expect_err("a blank identifier is rejected");
    registry
        .register(Arc::new(StubSource::new("secondary", &["HK"])))
        .expect("register secondary");
    registry
        .register(Arc::new(StubSource::new(
            "primary",
            &[" us ", "US", "", "cn"],
        )))
        .expect("register primary");
    registry
        .register(Arc::new(StubSource::with_authority(
            "primary",
            " Replacement ",
            &["CN"],
        )))
        .expect("replace primary");

    let replaced = registry.source(" primary ").expect("trimmed lookup");
    assert_eq!(
        replaced.descriptor().authority.trim(),
        "Replacement",
        "the later registration replaces the adapter"
    );

    let descriptors = registry.descriptors();
    let primary = descriptors
        .iter()
        .find(|descriptor| descriptor.id == "primary")
        .expect("primary descriptor");
    assert_eq!(
        primary.markets,
        vec!["CN"],
        "markets normalize, deduplicate and sort; the blank entry disappears"
    );
    assert_eq!(
        descriptors
            .iter()
            .map(|descriptor| descriptor.id.as_str())
            .collect::<Vec<_>>(),
        vec!["primary", "secondary"],
        "descriptor rows are unique and sorted by id"
    );

    // The CN policy reaches the `primary` provider for an SZ target, and the
    // blank-id registration above contributed no row.
    let seen = Arc::new(Mutex::new(Vec::new()));
    let primary = Arc::new(StubSource::new("primary", &["CN"]));
    primary.observe(Arc::clone(&seen));
    primary.push(Ok(snapshot_for("CN")));
    let mut refresh_registry = CalendarSourceRegistry::default();
    refresh_registry
        .register(Arc::clone(&primary) as Arc<dyn CalendarSourcePort>)
        .expect("register primary");
    let settings = CalendarManagerSettings {
        warmup_markets: vec!["SZ".to_owned()],
        source_policies: vec![CalendarSourcePolicy {
            market: "CN".to_owned(),
            preferred_source_ids: vec!["secondary".to_owned(), "primary".to_owned()],
            enabled_source_ids: vec!["primary".to_owned()],
            fallback_to_builtin: true,
            ..CalendarSourcePolicy::default()
        }],
        ..CalendarManagerSettings::default()
    };
    let manager = CalendarManager::new(refresh_registry, None, settings).expect("create manager");
    manager.start().expect("start manager");
    assert_eq!(manager.refresh_market("SZ").expect("refresh SZ").updated, 1);
    manager.close().expect("close manager");
    assert_eq!(
        seen.lock().expect("fetch log").clone(),
        vec!["CN".to_owned()],
        "only the enabled CN provider is contacted for an SZ target"
    );
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:14
/// TestManagerLifecycleAndTemplateBoundaries.
///
/// Start/close/settings-reload are idempotent; a market code is trimmed and
/// upper-cased before lookup; an unknown market is rejected rather than
/// silently defaulted.
#[test]
fn manager_lifecycle_and_market_lookup_boundaries_match_go() {
    let settings = CalendarManagerSettings {
        auto_refresh_enabled: false,
        refresh_interval_hours: 0,
        ..CalendarManagerSettings::default()
    };
    let manager = CalendarManager::new(CalendarSourceRegistry::default(), None, settings)
        .expect("create manager");

    let schedule = manager
        .schedule(" us ", timestamp("2026-06-19T14:00:00Z"))
        .expect("normalized schedule")
        .expect("schedule");
    assert_eq!(
        schedule.market_code, "US",
        "a market code is normalized before the template lookup"
    );
    assert!(matches!(
        manager.schedule("MARS", timestamp("2026-06-19T14:00:00Z")),
        Err(jftrade_calendar::CalendarManagerError::UnsupportedMarket(_))
    ));

    manager.start().expect("start");
    manager.start().expect("repeat start is idempotent");
    manager
        .reload_settings(CalendarManagerSettings::default())
        .expect("reload");
    manager
        .reload_settings(CalendarManagerSettings::default())
        .expect("repeat reload");
    manager.close().expect("close");
    manager.close().expect("repeat close is idempotent");
    assert_eq!(
        manager.lifecycle_state().expect("lifecycle"),
        jftrade_calendar::ManagerLifecycleState::Closed
    );
    assert!(matches!(
        manager.refresh_market("US"),
        Err(jftrade_calendar::CalendarManagerError::Closed)
    ));
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:119
/// TestHTTPCalendarSourceValidateSnapshotBoundary.
///
/// Go puts `ValidateSnapshot` on the transport adapter and returns nil for a
/// nil adapter or one without a validator. Rust has no adapter-level method:
/// validation lives in the domain owner and is reached through the manager.
/// This pins the replacement behaviour — a structurally empty provider answer
/// is rejected, attributed to that provider, and never cached.
#[test]
fn provider_snapshot_validation_has_a_single_domain_owner() {
    let source = Arc::new(StubSource::new("nyse_official", &["US"]));
    source.push(Ok(CalendarSnapshot {
        source_id: "nyse_official".to_owned(),
        market_code: "US".to_owned(),
        from: timestamp("2026-01-01T00:00:00Z"),
        to: timestamp("2026-12-31T23:59:59Z"),
        schedules: Vec::new(),
        fetched_at: timestamp("2026-06-19T12:00:00Z"),
        valid_until: timestamp("2026-12-31T23:59:59Z"),
        checksum: String::new(),
    }));
    let settings = CalendarManagerSettings {
        warmup_markets: vec!["US".to_owned()],
        source_policies: vec![CalendarSourcePolicy {
            market: "US".to_owned(),
            preferred_source_ids: vec!["nyse_official".to_owned()],
            enabled_source_ids: vec!["nyse_official".to_owned()],
            fallback_to_builtin: true,
            ..CalendarSourcePolicy::default()
        }],
        ..CalendarManagerSettings::default()
    };
    let mut registry = CalendarSourceRegistry::default();
    registry
        .register(Arc::clone(&source) as Arc<dyn CalendarSourcePort>)
        .expect("register source");
    let manager = CalendarManager::new(registry, None, settings).expect("create manager");
    manager.start().expect("start manager");

    let refresh = manager.refresh_market("US").expect("refresh");
    assert_eq!((refresh.updated, refresh.failures), (0, 1));
    assert!(manager.snapshots().expect("snapshots").is_empty());
    let status = manager
        .status_snapshot()
        .expect("status")
        .sources
        .into_iter()
        .find(|row| row.id == "nyse_official")
        .expect("provider row");
    assert_eq!(status.health_state, "unhealthy");
    assert!(
        status.last_error.contains("no schedules parsed"),
        "the structure failure keeps its reason: {:?}",
        status.last_error
    );
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:216
/// TestManagerValidateCachedSnapshotUsesRegisteredSourceValidator.
///
/// Go lets a registered provider inject an extra validator during cache
/// validation. Rust keeps one validation owner, so the equivalent guarantee is
/// that a provider-supplied schedule set is validated by the domain before it
/// is cached or served — asserted through the observable rejection above and
/// the accepted snapshot here.
#[test]
fn registered_provider_snapshots_pass_domain_validation_before_caching() {
    let source = Arc::new(StubSource::new("nyse_official", &["US"]));
    source.push(Ok(CalendarSnapshot {
        source_id: "nyse_official".to_owned(),
        market_code: "US".to_owned(),
        from: timestamp("2026-01-01T00:00:00-05:00"),
        to: timestamp("2026-12-31T23:59:59-05:00"),
        schedules: vec![TradingDaySchedule {
            market_code: "US".to_owned(),
            date: timestamp("2026-06-19T00:00:00-04:00"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: "juneteenth".to_owned(),
            source_id: "nyse_official".to_owned(),
            observed: false,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-06-19T12:00:00Z"),
        valid_until: timestamp("2026-12-31T23:59:59Z"),
        checksum: "nyse-fixture".to_owned(),
    }));
    let settings = CalendarManagerSettings {
        warmup_markets: vec!["US".to_owned()],
        source_policies: vec![CalendarSourcePolicy {
            market: "US".to_owned(),
            preferred_source_ids: vec!["nyse_official".to_owned()],
            enabled_source_ids: vec!["nyse_official".to_owned()],
            fallback_to_builtin: true,
            ..CalendarSourcePolicy::default()
        }],
        ..CalendarManagerSettings::default()
    };
    let mut registry = CalendarSourceRegistry::default();
    registry
        .register(Arc::clone(&source) as Arc<dyn CalendarSourcePort>)
        .expect("register source");
    let manager = CalendarManager::new(registry, None, settings).expect("create manager");
    manager.start().expect("start manager");

    assert_eq!(manager.refresh_market("US").expect("refresh").updated, 1);
    let served = manager
        .schedule("US", timestamp("2026-06-19T14:00:00Z"))
        .expect("schedule")
        .expect("schedule");
    assert_eq!(served.source_id, "nyse_official");
    assert_eq!(served.reason, "juneteenth");
    manager.close().expect("close manager");
}

/// Parity: go:452dea11:internal/exchangecalendar/manager_boundaries_test.go:333
/// TestExtractNYSEHeaderYearsSkipsMalformedRowsBeforeValidHeader.
///
/// `extractNYSEHeaderYears` walks the NYSE HTML holiday table. Rust has no HTML
/// calendar parser and no transport-side scraper, so there is no equivalent
/// owner to test; the curated descriptor/note projection is what survives and
/// is asserted here so the retired parser is anchored to its replacement.
#[test]
fn nyse_header_year_extraction_is_a_retired_go_parser_boundary() {
    let nyse = default_source_descriptors()
        .into_iter()
        .find(|descriptor| descriptor.id == "nyse_official")
        .expect("curated NYSE descriptor");
    assert_eq!(nyse.markets, vec!["US"]);
    assert!(
        source_availability_note("nyse_official").contains("multi-year holiday table"),
        "the curated note records what the retired parser used to read"
    );
    let projected = project_default_sources(
        normalize_source_ids(["nyse_official".to_owned()]),
        &BTreeMap::new(),
    );
    let row = projected
        .sources
        .into_iter()
        .find(|source| source.id == "nyse_official")
        .expect("projected NYSE row");
    assert!(row.enabled);
    assert!(!row.availability_note.is_empty());
}

/// Shared log of the markets a stub provider was asked to fetch.
type FetchLog = Arc<Mutex<Vec<String>>>;

#[derive(Clone)]
struct StubSource {
    id: String,
    authority: String,
    markets: Vec<String>,
    queued: Arc<Mutex<Vec<Result<CalendarSnapshot, CalendarSourceError>>>>,
    observed: Arc<Mutex<Option<FetchLog>>>,
}

impl StubSource {
    fn new(id: &str, markets: &[&str]) -> Self {
        Self::with_authority(id, "fixture", markets)
    }

    fn with_authority(id: &str, authority: &str, markets: &[&str]) -> Self {
        Self {
            id: id.to_owned(),
            authority: authority.to_owned(),
            markets: markets.iter().map(|market| (*market).to_owned()).collect(),
            queued: Arc::new(Mutex::new(Vec::new())),
            observed: Arc::new(Mutex::new(None)),
        }
    }

    fn push(&self, result: Result<CalendarSnapshot, CalendarSourceError>) {
        self.queued.lock().expect("stub queue").push(result);
    }

    fn observe(&self, log: FetchLog) {
        self.observed
            .lock()
            .expect("stub observer slot")
            .replace(log);
    }
}

impl CalendarSourcePort for StubSource {
    fn descriptor(&self) -> CalendarSourceDescriptor {
        CalendarSourceDescriptor {
            id: self.id.clone(),
            kind: "remote".to_owned(),
            authority: self.authority.clone(),
            markets: self.markets.clone(),
        }
    }

    fn fetch(
        &self,
        market: &str,
        _from: WireTimestamp,
        _to: WireTimestamp,
        _cancellation: &CalendarCancellationToken,
    ) -> Result<CalendarSnapshot, CalendarSourceError> {
        if let Some(log) = self.observed.lock().expect("stub observer slot").as_ref() {
            log.lock().expect("stub fetch log").push(market.to_owned());
        }
        let mut queued = self.queued.lock().expect("stub queue");
        if queued.is_empty() {
            return Err(CalendarSourceError::Failed("stub exhausted".to_owned()));
        }
        queued.remove(0)
    }
}

/// A minimal valid CN snapshot, used to drive the alias/target assertions.
fn snapshot_for(market: &str) -> CalendarSnapshot {
    CalendarSnapshot {
        source_id: "mainland".to_owned(),
        market_code: market.to_owned(),
        from: timestamp("2026-01-01T00:00:00+08:00"),
        to: timestamp("2026-12-31T23:59:59+08:00"),
        schedules: vec![TradingDaySchedule {
            market_code: market.to_owned(),
            date: timestamp("2026-10-01T00:00:00+08:00"),
            status: "closed".to_owned(),
            sessions: Vec::new(),
            reason: "national_day".to_owned(),
            source_id: "mainland".to_owned(),
            observed: false,
            updated_at: None,
        }],
        fetched_at: timestamp("2026-09-30T00:00:00Z"),
        valid_until: timestamp("2027-01-31T00:00:00Z"),
        checksum: "mainland".to_owned(),
    }
}

fn fixture_directory(label: &str) -> std::path::PathBuf {
    std::env::temp_dir().join(format!(
        "jftrade-calendar-boundaries-{}-{}-{}",
        label.replace(' ', "-"),
        std::process::id(),
        time::OffsetDateTime::now_utc().unix_timestamp_nanos()
    ))
}

/// Write a snapshot JSON body without the store's own identity validation, so
/// a corrupt cache can be placed where the loader will find it.
fn write_raw_snapshot(directory: &std::path::Path, snapshot: &CalendarSnapshot) {
    let path = directory.join("raw").join("2026").join("fixture.json");
    std::fs::create_dir_all(path.parent().expect("snapshot parent")).expect("prepare snapshot dir");
    std::fs::write(
        &path,
        serde_json::to_vec_pretty(snapshot).expect("encode fixture snapshot"),
    )
    .expect("write fixture snapshot");
}

fn persist(directory: &std::path::Path, snapshot: &CalendarSnapshot) {
    let store = jftrade_calendar::CalendarSnapshotStore::new(directory.to_path_buf());
    store.save(snapshot).expect("persist fixture snapshot");
}

fn manager_with_store(directory: &std::path::Path) -> CalendarManager {
    manager_with_store_and_settings(
        directory,
        CalendarManagerSettings::default(),
        timestamp("2026-06-01T00:00:00Z"),
    )
}

fn manager_with_store_and_settings(
    directory: &std::path::Path,
    settings: CalendarManagerSettings,
    now: WireTimestamp,
) -> CalendarManager {
    let fixed = now.into_inner();
    let store = jftrade_calendar::CalendarSnapshotStore::new(directory.to_path_buf());
    CalendarManager::with_clock(
        CalendarSourceRegistry::default(),
        Some(Arc::new(store) as Arc<dyn jftrade_calendar::CalendarPersistencePort>),
        settings,
        Arc::new(move || fixed),
    )
    .expect("create manager")
}
