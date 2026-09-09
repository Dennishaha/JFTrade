use jftrade_calendar::{
    CalendarManager, CalendarManagerSettings, CalendarManualOverride, CalendarSessionOverride,
    CalendarSourceRegistry, candle_is_closed,
};
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

fn at(text: &str) -> OffsetDateTime {
    OffsetDateTime::parse(text, &Rfc3339).unwrap()
}

#[test]
fn hong_kong_daily_bar_remains_open_across_utc_midnight() {
    let open = at("2026-09-07T16:00:00Z");
    assert!(
        !candle_is_closed(
            None,
            "HK",
            "1d",
            open,
            at("2026-09-08T01:00:00Z"),
            &["regular"]
        )
        .unwrap()
    );
    assert!(
        candle_is_closed(
            None,
            "HK",
            "1d",
            open,
            at("2026-09-08T08:00:00Z"),
            &["regular"]
        )
        .unwrap()
    );
}

#[test]
fn weekly_and_monthly_bars_wait_for_the_last_trading_session() {
    assert!(
        !candle_is_closed(
            None,
            "US",
            "1w",
            at("2026-09-07T04:00:00Z"),
            at("2026-09-08T15:00:00Z"),
            &["regular"]
        )
        .unwrap()
    );
    assert!(
        candle_is_closed(
            None,
            "US",
            "1w",
            at("2026-09-07T04:00:00Z"),
            at("2026-09-11T20:00:00Z"),
            &["regular"]
        )
        .unwrap()
    );
    assert!(
        !candle_is_closed(
            None,
            "US",
            "1mo",
            at("2026-09-01T04:00:00Z"),
            at("2026-09-08T15:00:00Z"),
            &["regular"]
        )
        .unwrap()
    );
    assert!(
        candle_is_closed(
            None,
            "US",
            "1mo",
            at("2026-09-01T04:00:00Z"),
            at("2026-09-30T20:00:00Z"),
            &["regular"]
        )
        .unwrap()
    );
}

#[test]
fn authoritative_half_day_closes_daily_and_partial_intraday_bars() {
    let calendar = CalendarManager::new(
        CalendarSourceRegistry::default(),
        None,
        CalendarManagerSettings {
            manual_overrides: vec![CalendarManualOverride {
                market: "HK".into(),
                date: "2026-09-08".into(),
                status: "open".into(),
                sessions: vec![CalendarSessionOverride {
                    kind: "regular".into(),
                    start_minute: 570,
                    end_minute: 720,
                }],
                ..Default::default()
            }],
            ..Default::default()
        },
    )
    .unwrap();
    assert!(
        !candle_is_closed(
            Some(&calendar),
            "HK",
            "1d",
            at("2026-09-07T16:00:00Z"),
            at("2026-09-08T03:59:00Z"),
            &["regular"]
        )
        .unwrap()
    );
    assert!(
        candle_is_closed(
            Some(&calendar),
            "HK",
            "1d",
            at("2026-09-07T16:00:00Z"),
            at("2026-09-08T04:00:00Z"),
            &["regular"]
        )
        .unwrap()
    );
    assert!(
        candle_is_closed(
            Some(&calendar),
            "HK",
            "1h",
            at("2026-09-08T03:30:00Z"),
            at("2026-09-08T04:00:00Z"),
            &["regular"]
        )
        .unwrap()
    );
}
