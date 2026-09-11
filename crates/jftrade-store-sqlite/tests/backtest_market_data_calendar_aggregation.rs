use std::path::Path;
use std::sync::Arc;

use jftrade_store_sqlite::{
    BACKTEST_MARKET_DATA_PRODUCTION_PROFILE, BacktestMarketDataStore, BacktestMarketDataStoreError,
    CalendarDaySchedule, CalendarDaySession, CalendarScheduleResolver, StoredBacktestCandle,
    initialize_current,
};
use jiff::civil::Date;
use rusqlite::Connection;

const MINUTE_MS: i64 = 60_000;
const DAY_MS: i64 = 86_400_000;
const HOUR_MS: i64 = 60 * MINUTE_MS;

struct OverrideCalendar;

impl CalendarScheduleResolver for OverrideCalendar {
    fn schedule(&self, market: &str, date: Date) -> Option<CalendarDaySchedule> {
        if market != "US" {
            return None;
        }
        let closed = date.to_string() == "2026-05-20"
            || matches!(
                date.weekday(),
                jiff::civil::Weekday::Saturday | jiff::civil::Weekday::Sunday
            );
        Some(CalendarDaySchedule {
            status: if closed { "closed" } else { "open" }.to_owned(),
            sessions: if closed {
                Vec::new()
            } else {
                vec![CalendarDaySession {
                    kind: "regular".to_owned(),
                    start_minute: 570,
                    end_minute: 960,
                }]
            },
        })
    }
}

fn store(path: &Path) -> BacktestMarketDataStore {
    let connection = Connection::open(path).expect("create database");
    initialize_current(&connection, "backtest").expect("initialize schema");
    drop(connection);
    BacktestMarketDataStore::open_existing(path, BACKTEST_MARKET_DATA_PRODUCTION_PROFILE)
        .expect("open store")
}

fn candle(start_time: i64, duration_ms: i64, price: i64, volume: i64) -> StoredBacktestCandle {
    StoredBacktestCandle {
        start_time,
        end_time: start_time + duration_ms - 1,
        open: price.to_string(),
        high: (price + 2).to_string(),
        low: (price - 1).to_string(),
        close: (price + 1).to_string(),
        volume: volume.to_string(),
    }
}

fn utc_midnight(year: i16, month: i8, day: i8) -> i64 {
    Date::new(year, month, day)
        .expect("valid fixture date")
        .at(0, 0, 0, 0)
        .in_tz("UTC")
        .expect("UTC fixture timestamp")
        .timestamp()
        .as_millisecond()
}

fn market_local_time(year: i16, month: i8, day: i8, hour: i8) -> i64 {
    Date::new(year, month, day)
        .expect("valid fixture date")
        .at(hour, 0, 0, 0)
        .in_tz("America/New_York")
        .expect("US fixture timestamp")
        .timestamp()
        .as_millisecond()
}

#[test]
fn missing_daily_target_synthesizes_us_regular_day_from_minutes() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let store = store(&directory.path().join("daily-from-minutes.db"));

    // 2024-03-08 09:30 EST -> 14:30 UTC, before the DST switch.
    let session_start = 1_709_908_200_000i64;
    let day_start = 1_709_856_000_000i64; // 2024-03-08T00:00:00Z
    let day_end = day_start + DAY_MS;
    let rows = (0..390)
        .map(|index| candle(session_start + index * MINUTE_MS, 60_000, 100 + index, 1))
        .collect::<Vec<_>>();
    store
        .insert_candles("futu", "US.AAPL", "1m", "forward", "regular", &rows)
        .expect("insert minute rows");

    let daily = store
        .read_candles(
            "futu", "US.AAPL", "1d", "forward", "regular", day_start, day_end,
        )
        .expect("synthesize daily candle");
    assert_eq!(daily.len(), 1);
    assert_eq!(daily[0].start_time, day_start);
    assert_eq!(daily[0].end_time, day_end - 1);
    assert_eq!(daily[0].open, "100");
    assert_eq!(daily[0].high, "491");
    assert_eq!(daily[0].low, "99");
    assert_eq!(daily[0].close, "490");
    assert_eq!(daily[0].volume, "390");
}

#[test]
fn missing_daily_target_uses_ten_minute_source_before_failing() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let store = store(&directory.path().join("daily-from-ten-minutes.db"));

    // 2024-03-08 EST regular session: 14:30~21:00 UTC (390 minutes).
    let session_start = 1_709_908_200_000i64;
    let day_start = 1_709_856_000_000i64;
    let rows = (0..39)
        .map(|index| {
            candle(
                session_start + index * 10 * MINUTE_MS,
                10 * MINUTE_MS,
                200 + index,
                2,
            )
        })
        .collect::<Vec<_>>();
    store
        .insert_candles("futu", "US.AAPL", "10m", "forward", "regular", &rows)
        .expect("insert ten-minute rows");

    let daily = store
        .read_candles(
            "futu",
            "US.AAPL",
            "1d",
            "forward",
            "regular",
            day_start,
            day_start + DAY_MS,
        )
        .expect("synthesize daily candle from ten-minute rows");
    assert_eq!(daily.len(), 1);
    assert_eq!(daily[0].open, "200");
    assert_eq!(daily[0].high, "240");
    assert_eq!(daily[0].low, "199");
    assert_eq!(daily[0].close, "239");
    assert_eq!(daily[0].volume, "78");
}

#[test]
fn weekly_and_monthly_targets_synthesize_from_daily_rows_with_calendar_boundaries() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let weekly_store = store(&directory.path().join("periods-from-daily.db"));

    // 2026-05-18 through 2026-05-22 are all US trading weekdays.  Persisting 1d rows
    // makes the 1w/1mo fallback exercise calendar labels rather than fixed
    // 7*24h/30*24h durations.
    let may_25 = 1_779_062_400_000i64; // 2026-05-18T00:00:00Z
    let daily_rows = (0..5)
        .map(|index| candle(may_25 + index * DAY_MS, DAY_MS, 100 + index, 10 + index))
        .collect::<Vec<_>>();
    weekly_store
        .insert_candles("futu", "US.AAPL", "1d", "forward", "regular", &daily_rows)
        .expect("insert daily rows");

    let week = weekly_store
        .read_candles(
            "futu",
            "US.AAPL",
            "1w",
            "forward",
            "regular",
            may_25,
            may_25 + 7 * DAY_MS,
        )
        .expect("synthesize weekly candle");
    assert_eq!(week.len(), 1);
    assert_eq!(week[0].start_time, may_25);
    assert_eq!(week[0].end_time, may_25 + 7 * DAY_MS - 1);
    assert_eq!(week[0].open, "100");
    assert_eq!(week[0].high, "106");
    assert_eq!(week[0].low, "99");
    assert_eq!(week[0].close, "105");
    assert_eq!(week[0].volume, "60");

    // A monthly bucket requires every open trading date.  Use a separate
    // fixture containing all May 2026 US sessions (Memorial Day is closed);
    // the end must be June 1, not a fixed 30-day approximation.
    let monthly_store = store(&directory.path().join("monthly-period.db"));
    let may_open_offsets = [
        0, 3, 4, 5, 6, 7, 10, 11, 12, 13, 14, 17, 18, 19, 20, 21, 25, 26, 27, 28,
    ];
    let monthly_rows = may_open_offsets
        .into_iter()
        .enumerate()
        .map(|(index, offset)| {
            candle(
                may_25 - 17 * DAY_MS + offset * DAY_MS,
                DAY_MS,
                100 + index as i64,
                1,
            )
        })
        .collect::<Vec<_>>();
    monthly_store
        .insert_candles("futu", "US.AAPL", "1d", "forward", "regular", &monthly_rows)
        .expect("insert complete monthly rows");
    let may_2026 = 1_777_593_600_000i64; // 2026-05-01T00:00:00Z
    let june_2026 = 1_780_272_000_000i64; // 2026-06-01T00:00:00Z
    let monthly_2026 = monthly_store
        .read_candles(
            "futu", "US.AAPL", "1mo", "forward", "regular", may_2026, june_2026,
        )
        .expect("synthesize 2026 monthly candle");
    assert_eq!(monthly_2026.len(), 1);
    assert_eq!(monthly_2026[0].start_time, may_2026);
    assert_eq!(monthly_2026[0].end_time, june_2026 - 1);

    let backward = monthly_store
        .query_candles_backward("futu", "US.AAPL", "1mo", "forward", "regular", june_2026, 1)
        .expect("query monthly candle backward");
    assert_eq!(backward.len(), 1);
    assert_eq!(backward[0].start_time, may_2026);
}

#[test]
fn weekly_daily_source_requires_every_open_trading_date() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let store = store(&directory.path().join("daily-week-gap.db"));
    let monday = 1_779_062_400_000i64; // 2026-05-18T00:00:00Z
    let friday = monday + 4 * DAY_MS;
    store
        .insert_candles(
            "futu",
            "US.AAPL",
            "1d",
            "forward",
            "regular",
            &[
                candle(monday, DAY_MS, 100, 1),
                candle(friday, DAY_MS, 104, 1),
            ],
        )
        .expect("insert incomplete daily rows");

    let result = store.read_candles(
        "futu",
        "US.AAPL",
        "1w",
        "forward",
        "regular",
        monday,
        monday + 7 * DAY_MS,
    );
    assert!(matches!(
        result,
        Err(BacktestMarketDataStoreError::Coverage(_))
    ));
}

#[test]
fn weekly_calendar_aggregation_crosses_year_boundary_and_omits_cutoff_partial_bucket() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let store = store(&directory.path().join("cross-year-week.db"));
    let week_start = utc_midnight(2025, 12, 29);
    let week_end = utc_midnight(2026, 1, 5);

    // New Year's Day is closed, but the other four weekdays in this ISO week
    // are open.  Keeping the rows in one UTC table exercises the resolver's
    // year transition instead of relying on a fixed seven-day row count.
    let daily_rows = [0_i64, 1, 2, 4]
        .into_iter()
        .enumerate()
        .map(|(index, offset)| {
            candle(
                week_start + offset * DAY_MS,
                DAY_MS,
                100 + index as i64,
                10 + index as i64,
            )
        })
        .collect::<Vec<_>>();
    store
        .insert_candles("futu", "US.AAPL", "1d", "forward", "regular", &daily_rows)
        .expect("insert cross-year daily rows");

    // A cutoff before Monday's next-period boundary must not expose a partial
    // weekly bucket, even though the padded source range can see all rows.
    let partial = store
        .read_candles(
            "futu",
            "US.AAPL",
            "1w",
            "forward",
            "regular",
            week_start,
            utc_midnight(2026, 1, 3),
        )
        .expect("partial cutoff is a valid empty result");
    assert!(partial.is_empty(), "cutoff must not emit an open week");

    let first = store
        .read_candles(
            "futu", "US.AAPL", "1w", "forward", "regular", week_start, week_end,
        )
        .expect("aggregate complete cross-year week");
    let second = store
        .read_candles(
            "futu", "US.AAPL", "1w", "forward", "regular", week_start, week_end,
        )
        .expect("repeat cross-year aggregation");
    assert_eq!(first, second, "repeated reads must be stable");
    assert_eq!(first.len(), 1);
    assert_eq!(first[0].start_time, week_start);
    assert_eq!(first[0].end_time, week_end - 1);
    assert_eq!(first[0].open, "100");
    assert_eq!(first[0].high, "105");
    assert_eq!(first[0].low, "99");
    assert_eq!(first[0].close, "104");
    assert_eq!(first[0].volume, "46");
}

#[test]
fn extended_daily_aggregation_assigns_overnight_rows_to_next_trading_date() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let store = store(&directory.path().join("extended-overnight.db"));
    let monday = utc_midnight(2026, 5, 25);
    let overnight_start = market_local_time(2026, 5, 24, 20);
    let rows = (0..24)
        .map(|index| candle(overnight_start + index * HOUR_MS, HOUR_MS, 100 + index, 1))
        .collect::<Vec<_>>();
    store
        .insert_candles("futu", "US.AAPL", "1h", "forward", "extended", &rows)
        .expect("insert extended overnight rows");

    let daily = store
        .read_candles(
            "futu",
            "US.AAPL",
            "1d",
            "forward",
            "extended",
            monday,
            monday + DAY_MS,
        )
        .expect("aggregate overnight rows into Monday trading date");
    assert_eq!(daily.len(), 1);
    assert_eq!(daily[0].start_time, monday);
    assert_eq!(daily[0].end_time, monday + DAY_MS - 1);
    assert_eq!(daily[0].open, "100");
    assert_eq!(daily[0].high, "125");
    assert_eq!(daily[0].low, "99");
    assert_eq!(daily[0].close, "124");
    assert_eq!(daily[0].volume, "24");
}

#[test]
fn weekly_daily_aggregation_honors_bound_calendar_override() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let store = store(&directory.path().join("calendar-override.db"));
    store.set_calendar_resolver(Arc::new(OverrideCalendar));
    let monday = 1_779_062_400_000i64; // 2026-05-18T00:00:00Z
    let rows = (0..5)
        .map(|index| candle(monday + index * DAY_MS, DAY_MS, 100 + index, 1))
        .collect::<Vec<_>>();
    store
        .insert_candles("futu", "US.AAPL", "1d", "forward", "regular", &rows)
        .expect("insert daily rows");

    let weekly = store
        .read_candles(
            "futu",
            "US.AAPL",
            "1w",
            "forward",
            "regular",
            monday,
            monday + 7 * DAY_MS,
        )
        .expect("aggregate with manual closure");
    assert_eq!(weekly.len(), 1);
    assert_eq!(weekly[0].volume, "4");
    assert_eq!(weekly[0].high, "106");
    assert_eq!(weekly[0].close, "105");
}

#[test]
fn extended_weekly_aggregation_prefers_intraday_source_over_daily_rows() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let store = store(&directory.path().join("extended-weekly-source.db"));

    // The regular daily table is intentionally populated with a different
    // price. Go's extended-hours resolver restricts weekly/monthly synthesis
    // to <=1h sources so that pre/post-market observations are not discarded.
    let may_25 = 1_779_667_200_000i64; // 2026-05-25T00:00:00Z
    let daily_rows = (0..5)
        .map(|index| candle(may_25 + index * DAY_MS, DAY_MS, 100 + index, 1))
        .collect::<Vec<_>>();
    store
        .insert_candles("futu", "US.AAPL", "1d", "forward", "extended", &daily_rows)
        .expect("insert daily rows");

    // 2026-05-25 09:30 EDT = 13:30 UTC. A 1h row is enough to identify the
    // selected extended source; larger source intervals must not win first.
    let intraday = candle(1_779_715_800_000, 60 * MINUTE_MS, 200, 2);
    store
        .insert_candles(
            "futu",
            "US.AAPL",
            "1h",
            "forward",
            "extended",
            std::slice::from_ref(&intraday),
        )
        .expect("insert extended intraday row");

    let weekly = store
        .read_candles(
            "futu",
            "US.AAPL",
            "1w",
            "forward",
            "extended",
            may_25,
            may_25 + 7 * DAY_MS,
        )
        .expect("synthesize extended weekly candle");
    assert_eq!(weekly.len(), 1);
    assert_eq!(weekly[0].open, "200");
    assert_eq!(weekly[0].volume, "2");
}

#[test]
fn extended_daily_aggregation_uses_intraday_source_before_stored_daily_fallback() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let store = store(&directory.path().join("extended-daily-source.db"));
    let day_start = 1_779_667_200_000i64; // 2026-05-25T00:00:00Z

    let stored_daily = candle(day_start, DAY_MS, 100, 1);
    store
        .insert_candles(
            "futu",
            "US.AAPL",
            "1d",
            "forward",
            "extended",
            std::slice::from_ref(&stored_daily),
        )
        .expect("insert stored daily fallback");
    let intraday = candle(1_779_715_800_000, 60 * MINUTE_MS, 200, 2);
    store
        .insert_candles(
            "futu",
            "US.AAPL",
            "1h",
            "forward",
            "extended",
            std::slice::from_ref(&intraday),
        )
        .expect("insert extended intraday row");

    let daily = store
        .read_candles(
            "futu",
            "US.AAPL",
            "1d",
            "forward",
            "extended",
            day_start,
            day_start + DAY_MS,
        )
        .expect("synthesize extended daily candle");
    assert_eq!(daily.len(), 1);
    assert_eq!(daily[0].open, "200");
    assert_eq!(daily[0].volume, "2");

    let queried = store
        .query_candles(
            "futu",
            "US.AAPL",
            "1d",
            "forward",
            "extended",
            day_start,
            day_start + DAY_MS,
            "ASC",
            1,
        )
        .expect("query synthesized extended daily candle");
    assert_eq!(queried.len(), 1);
    assert_eq!(queried[0].open, "200");
}

#[test]
fn direct_calendar_target_rows_win_over_lower_period_synthesis() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let store = store(&directory.path().join("period-priority.db"));
    let day_start = 1_779_667_200_000i64;
    let direct = candle(day_start, 7 * DAY_MS, 900, 1);
    store
        .insert_candles(
            "futu",
            "US.AAPL",
            "1w",
            "forward",
            "regular",
            std::slice::from_ref(&direct),
        )
        .expect("insert direct weekly row");
    store
        .insert_candles("futu", "US.AAPL", "1d", "forward", "regular", &[])
        .expect("create empty daily fallback table");

    let got = store
        .read_candles(
            "futu",
            "US.AAPL",
            "1w",
            "forward",
            "regular",
            day_start,
            day_start + 7 * DAY_MS,
        )
        .expect("read direct weekly row");
    assert_eq!(got, vec![direct]);
}

#[test]
fn missing_regular_minute_fails_closed_for_synthesized_daily_bar() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let store = store(&directory.path().join("daily-gap.db"));
    let session_start = 1_709_908_200_000i64;
    let day_start = 1_709_856_000_000i64;
    let rows = (0..390)
        .filter(|index| *index != 120)
        .map(|index| candle(session_start + index * MINUTE_MS, 60_000, 100 + index, 1))
        .collect::<Vec<_>>();
    store
        .insert_candles("futu", "US.AAPL", "1m", "forward", "regular", &rows)
        .expect("insert minute rows with gap");

    let result = store.read_candles(
        "futu",
        "US.AAPL",
        "1d",
        "forward",
        "regular",
        day_start,
        day_start + DAY_MS,
    );
    assert!(matches!(
        result,
        Err(BacktestMarketDataStoreError::Coverage(_))
    ));
}
