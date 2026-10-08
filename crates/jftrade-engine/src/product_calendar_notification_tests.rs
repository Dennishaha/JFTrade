use super::*;
use jftrade_calendar::CalendarSourceAlert;
use jftrade_settings::ExchangeCalendarSettingsStorePort;
use jftrade_store_settings_file::SettingsFileStore;
use std::sync::Arc;

fn alert() -> CalendarSourceAlert {
    CalendarSourceAlert {
        source_id: "nyse_official".to_owned(),
        market: "US".to_owned(),
        level: "warn".to_owned(),
        kind: "fetch_failed".to_owned(),
        title: "交易所日历源抓取失败".to_owned(),
        message: "US 市场日历源 nyse_official 抓取失败。".to_owned(),
        ..CalendarSourceAlert::default()
    }
}

// Parity: go:452dea11:internal/app/apiserver/servercore/notification_sources_test.go:13 TestExchangeCalendarAlertRecordingHonorsNotificationSetting
#[tokio::test]
async fn calendar_alert_delivery_reads_the_current_persisted_notification_setting() {
    let directory = tempfile::tempdir().expect("directory");
    let store = SettingsFileStore::open(directory.path().join("settings.json")).expect("store");
    let hub = Arc::new(jftrade_api::LiveHub::new(8));
    record_calendar_alert(&store, Some(&hub), alert());
    store
        .save_exchange_calendars(&ExchangeCalendarSettings {
            auto_refresh_enabled: true,
            error_notifications_enabled: false,
            refresh_interval_hours: 24,
            warmup_markets: vec!["US".to_owned()],
            ..ExchangeCalendarSettings::default()
        })
        .expect("disable notifications");
    record_calendar_alert(&store, Some(&hub), alert());
    hub.mark_serving();
    let mut connection = hub.connect();
    let first = tokio::time::timeout(std::time::Duration::from_secs(1), connection.recv())
        .await
        .expect("retained alert")
        .expect("notification");
    assert_eq!(first["payload"]["title"], "交易所日历源抓取失败");
    assert_eq!(first["type"], "system.notification");
    assert_eq!(first["source"], "notification");
    assert_eq!(first["entityId"], first["payload"]["id"]);
    assert!(!first["eventId"].as_str().expect("event id").is_empty());
    assert_eq!(first["serverTime"], first["payload"]["at"]);
    assert!(
        tokio::time::timeout(std::time::Duration::from_millis(25), connection.recv())
            .await
            .is_err()
    );
}

// Parity: go:452dea11:internal/app/apiserver/servercore/notification_sources_test.go:46 TestLiveNotificationFromExchangeCalendarAlertMapsSourceAndCategory
#[test]
fn calendar_structure_alert_maps_its_notification_source_category_and_timestamp() {
    let mut input = alert();
    input.level = "error".to_owned();
    input.kind = "structure_changed".to_owned();
    input.title = "交易所日历源解析异常".to_owned();
    input.message = "US 市场日历源 nyse_official 抓取成功但未解析到有效交易日。".to_owned();
    let note = calendar_notification(input).expect("notification");
    assert_eq!(note["level"], "error");
    assert_eq!(note["source"], "exchange-calendars");
    assert_eq!(note["category"], "market.calendar.source");
    assert!(!note["at"].as_str().expect("at").trim().is_empty());
    assert!(
        note["at"]
            .as_str()
            .expect("at")
            .parse::<jftrade_kernel::WireTimestamp>()
            .is_ok()
    );
}

#[test]
// Parity: go:452dea11:internal/app/apiserver/servercore/notification_market_workflow_contracts_test.go:18 TestBusinessNotificationsAndOptionalSecurityFieldsPreserveWireSemantics
fn calendar_notification_normalizes_levels_titles_and_empty_messages() {
    assert!(calendar_notification(CalendarSourceAlert::default()).is_none());
    for (level, expected) in [
        ("success", "success"),
        ("error", "error"),
        ("WARN", "warn"),
        ("unknown", "info"),
    ] {
        let note = calendar_notification(CalendarSourceAlert {
            level: level.to_owned(),
            title: " NYSE source stale ".to_owned(),
            market: " US ".to_owned(),
            source_id: " nyse ".to_owned(),
            ..CalendarSourceAlert::default()
        })
        .expect("note");
        assert_eq!(note["level"], expected);
        assert_eq!(note["title"], "NYSE source stale");
        assert_eq!(note["message"], "US 市场日历源 nyse 状态发生变化。");
    }
}

struct UnreadableCalendarSettings;

impl ExchangeCalendarSettingsStorePort for UnreadableCalendarSettings {
    fn load_exchange_calendars(
        &self,
    ) -> Result<Option<ExchangeCalendarSettings>, jftrade_settings::SettingsStoreError> {
        Err(jftrade_settings::SettingsStoreError::new(
            "settings unavailable",
        ))
    }

    fn save_exchange_calendars(
        &self,
        _: &ExchangeCalendarSettings,
    ) -> Result<ExchangeCalendarSettings, jftrade_settings::SettingsStoreError> {
        panic!("notification delivery must never write settings")
    }
}

#[tokio::test]
async fn calendar_notification_delivery_fails_closed_when_settings_are_unreadable() {
    let hub = Arc::new(jftrade_api::LiveHub::new(8));
    record_calendar_alert(&UnreadableCalendarSettings, Some(&hub), alert());
    record_calendar_alert(&UnreadableCalendarSettings, None, alert());
    hub.mark_serving();
    let mut connection = hub.connect();
    assert!(
        tokio::time::timeout(std::time::Duration::from_millis(25), connection.recv())
            .await
            .is_err()
    );
}

type CalendarResponse =
    Result<jftrade_calendar::CalendarSnapshot, jftrade_calendar::CalendarSourceError>;
struct SequenceSource(std::sync::Mutex<std::collections::VecDeque<CalendarResponse>>);

impl jftrade_calendar::CalendarSourcePort for SequenceSource {
    fn descriptor(&self) -> jftrade_calendar::CalendarSourceDescriptor {
        jftrade_calendar::CalendarSourceDescriptor {
            id: "nyse_official".to_owned(),
            kind: "fixture".to_owned(),
            authority: "fixture".to_owned(),
            markets: vec!["US".to_owned()],
        }
    }

    fn fetch(
        &self,
        _: &str,
        _: jftrade_kernel::WireTimestamp,
        _: jftrade_kernel::WireTimestamp,
        _: &jftrade_calendar::CalendarCancellationToken,
    ) -> Result<jftrade_calendar::CalendarSnapshot, jftrade_calendar::CalendarSourceError> {
        self.0
            .lock()
            .expect("source queue")
            .pop_front()
            .expect("queued response")
    }
}

#[tokio::test]
async fn production_calendar_source_alerts_reach_livehub_after_deduplication_and_settings_filtering()
 {
    let directory = tempfile::tempdir().expect("directory");
    let store =
        Arc::new(SettingsFileStore::open(directory.path().join("settings.json")).expect("store"));
    let settings = ExchangeCalendarSettings {
        auto_refresh_enabled: false,
        warmup_markets: vec!["US".to_owned()],
        ..ExchangeCalendarSettings::default()
    };
    store.save_exchange_calendars(&settings).expect("settings");
    let failure = || {
        Err(jftrade_calendar::CalendarSourceError::Failed(
            "temporary fetch failure".to_owned(),
        ))
    };
    let timestamp = |value: &str| value.parse().expect("timestamp");
    let mut snapshot = jftrade_calendar::CalendarSnapshot {
        source_id: "nyse_official".to_owned(),
        market_code: "US".to_owned(),
        from: timestamp("2026-01-01T00:00:00Z"),
        to: timestamp("2027-12-31T23:59:59Z"),
        fetched_at: timestamp("2026-06-01T00:00:00Z"),
        valid_until: timestamp("2027-12-31T23:59:59Z"),
        checksum: "fixture".to_owned(),
        schedules: vec![],
    };
    let empty = snapshot.clone();
    snapshot
        .schedules
        .push(jftrade_calendar::TradingDaySchedule {
            market_code: "US".to_owned(),
            source_id: "nyse_official".to_owned(),
            date: timestamp("2026-06-19T00:00:00Z"),
            status: "closed".to_owned(),
            sessions: vec![],
            reason: String::new(),
            observed: false,
            updated_at: None,
        });
    let source = Arc::new(SequenceSource(std::sync::Mutex::new(
        [failure(), failure(), Ok(empty), Ok(snapshot), failure()].into(),
    )));
    let mut registry = jftrade_calendar::CalendarSourceRegistry::default();
    registry.register(source).expect("registry");
    let hub = Arc::new(jftrade_api::LiveHub::new(8));
    let manager = production_calendar_manager(
        registry,
        None,
        settings.clone(),
        store.clone(),
        Some(hub.clone()),
    )
    .expect("manager");
    manager.start().expect("start");
    assert_eq!(manager.refresh_all().expect("failed refresh").failures, 1);
    assert_eq!(manager.probe_all().expect("repeated failure").failures, 1);
    assert_eq!(manager.probe_all().expect("structure failure").failures, 1);
    let disabled = ExchangeCalendarSettings {
        error_notifications_enabled: false,
        ..settings.clone()
    };
    store.save_exchange_calendars(&disabled).expect("disable");
    assert_eq!(manager.probe_all().expect("recovery").healthy, 1);
    assert_eq!(
        manager.source_statuses().expect("statuses")[0].last_alert_status,
        "recovered"
    );
    store.save_exchange_calendars(&settings).expect("enable");
    assert_eq!(manager.probe_all().expect("new failure").failures, 1);
    manager.close().expect("close");
    hub.mark_serving();
    let mut connection = hub.connect();
    let mut events = Vec::new();
    for _ in 0..3 {
        events.push(
            tokio::time::timeout(std::time::Duration::from_secs(1), connection.recv())
                .await
                .expect("alert delivered")
                .expect("event"),
        );
    }
    assert_eq!(
        events
            .iter()
            .map(|event| event["payload"]["level"].as_str().expect("level"))
            .collect::<Vec<_>>(),
        ["warn", "error", "warn"]
    );
    assert_ne!(events[0]["eventId"], events[2]["eventId"]);
    for event in events {
        assert_eq!(event["payload"]["source"], "exchange-calendars");
        assert_eq!(event["payload"]["category"], "market.calendar.source");
    }
    assert!(
        tokio::time::timeout(std::time::Duration::from_millis(25), connection.recv())
            .await
            .is_err()
    );
}
