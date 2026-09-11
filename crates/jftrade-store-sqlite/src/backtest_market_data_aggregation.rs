//! Aggregation helpers for backtest market data.

use std::collections::{BTreeMap, BTreeSet};

use jftrade_kernel::{Decimal, DecimalTradingExt};
use jiff::tz::TimeZone;
use jiff::{ToSpan, civil::Date};
use rusqlite::Connection;

use super::{BacktestMarketDataStoreError, StoredBacktestCandle, read_direct_range, table_exists};

/// Provider-neutral schedule data supplied by the composition root. Keeping
/// this trait in the store crate avoids a dependency on any concrete calendar
/// implementation while allowing production aggregation to honor manual and
/// refreshed calendar overrides.
pub trait CalendarScheduleResolver: Send + Sync {
    fn schedule(&self, market: &str, date: Date) -> Option<CalendarDaySchedule>;
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct CalendarDaySchedule {
    pub status: String,
    pub sessions: Vec<CalendarDaySession>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct CalendarDaySession {
    pub kind: String,
    pub start_minute: i32,
    pub end_minute: i32,
}

pub(crate) fn interval_minutes(interval: &str) -> Option<i64> {
    match interval.trim().to_ascii_lowercase().as_str() {
        "1m" | "1min" => Some(1),
        "3m" | "3min" => Some(3),
        "5m" | "5min" => Some(5),
        "10m" | "10min" => Some(10),
        "15m" | "15min" => Some(15),
        "30m" | "30min" => Some(30),
        "60m" | "60min" | "1h" | "1hour" => Some(60),
        "2h" | "2hour" => Some(120),
        "4h" | "4hour" => Some(240),
        "6h" | "6hour" => Some(360),
        "12h" | "12hour" => Some(720),
        _ => None,
    }
}

pub(crate) fn interval_duration_ms(interval: &str) -> Option<i64> {
    interval_minutes(interval)
        .map(|minutes| minutes.saturating_mul(60_000))
        .or_else(|| period_interval(interval).map(period_nominal_duration_ms))
}

pub(crate) fn is_aggregate_interval(interval: &str) -> bool {
    matches!(interval_minutes(interval), Some(minutes) if minutes > 1)
        || period_interval(interval).is_some()
}

/// Calendar periods cannot be represented by a fixed number of minutes.  The
/// string is kept at this boundary (rather than introducing a public enum) so
/// the existing SQLite/API contract remains unchanged.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum CalendarPeriod {
    Day,
    Week,
    Month,
}

pub(crate) fn period_interval(interval: &str) -> Option<CalendarPeriod> {
    match interval.trim().to_ascii_lowercase().as_str() {
        "1d" | "day" | "daily" | "d" => Some(CalendarPeriod::Day),
        "1w" | "week" | "weekly" | "w" => Some(CalendarPeriod::Week),
        "1mo" | "1mon" | "1month" | "month" | "monthly" | "mo" => Some(CalendarPeriod::Month),
        _ => None,
    }
}

pub(crate) fn period_nominal_duration_ms(period: CalendarPeriod) -> i64 {
    match period {
        CalendarPeriod::Day => 86_400_000,
        CalendarPeriod::Week => 7 * 86_400_000,
        // This duration is only used to make a bounded paging query.  Period
        // labels themselves are always advanced with calendar arithmetic.
        CalendarPeriod::Month => 30 * 86_400_000,
    }
}

/// Base intervals are ordered from the closest practical source to the
/// requested calendar period.  A persisted daily bar is preferred for weekly
/// and monthly synthesis; intraday sources then fall back from 12h to 1m.
pub(crate) fn period_source_intervals(
    period: CalendarPeriod,
    session_scope: &str,
) -> &'static [(&'static str, Option<i64>)] {
    const EXTENDED_INTRADAY: [(&str, Option<i64>); 7] = [
        ("1h", Some(60)),
        ("30m", Some(30)),
        ("15m", Some(15)),
        ("10m", Some(10)),
        ("5m", Some(5)),
        ("3m", Some(3)),
        ("1m", Some(1)),
    ];
    const INTRADAY: [(&str, Option<i64>); 11] = [
        ("12h", Some(720)),
        ("6h", Some(360)),
        ("4h", Some(240)),
        ("2h", Some(120)),
        ("1h", Some(60)),
        ("30m", Some(30)),
        ("15m", Some(15)),
        ("10m", Some(10)),
        ("5m", Some(5)),
        ("3m", Some(3)),
        ("1m", Some(1)),
    ];
    const DAILY_FIRST: [(&str, Option<i64>); 12] = [
        ("1d", None),
        ("12h", Some(720)),
        ("6h", Some(360)),
        ("4h", Some(240)),
        ("2h", Some(120)),
        ("1h", Some(60)),
        ("30m", Some(30)),
        ("15m", Some(15)),
        ("10m", Some(10)),
        ("5m", Some(5)),
        ("3m", Some(3)),
        ("1m", Some(1)),
    ];
    if session_scope.trim().eq_ignore_ascii_case("extended") {
        return &EXTENDED_INTRADAY;
    }
    match period {
        CalendarPeriod::Day => &INTRADAY,
        CalendarPeriod::Week | CalendarPeriod::Month => &DAILY_FIRST,
    }
}

pub(crate) fn aggregation_candidate_intervals(target_interval: &str) -> Vec<(&'static str, i64)> {
    const CANDIDATES: [(&str, i64); 10] = [
        ("6h", 360),
        ("4h", 240),
        ("2h", 120),
        ("1h", 60),
        ("30m", 30),
        ("15m", 15),
        ("10m", 10),
        ("5m", 5),
        ("3m", 3),
        ("1m", 1),
    ];
    let Some(target_min) = interval_minutes(target_interval) else {
        return Vec::new();
    };
    CANDIDATES
        .iter()
        .copied()
        .filter(|&(_, min)| min < target_min && target_min % min == 0)
        .collect()
}

pub(crate) fn normalize_limit(limit: usize) -> usize {
    limit.max(1)
}

pub(crate) fn market_from_symbol(symbol: &str) -> Option<&'static str> {
    let upper = symbol.trim().to_ascii_uppercase();
    if upper.starts_with("US.") {
        Some("US")
    } else if upper.starts_with("HK.") {
        Some("HK")
    } else if upper.starts_with("CN.") || upper.starts_with("SH.") || upper.starts_with("SZ.") {
        Some("CN")
    } else {
        None
    }
}

fn market_timezone(market: &str) -> &'static str {
    match market {
        "US" => "America/New_York",
        "HK" => "Asia/Hong_Kong",
        "CN" => "Asia/Shanghai",
        _ => "UTC",
    }
}

fn is_black_friday(date: Date) -> bool {
    date.month() == 11
        && date.weekday() == jiff::civil::Weekday::Friday
        && (23..=29).contains(&date.day())
}

fn is_christmas_eve_early_close(date: Date) -> bool {
    date.month() == 12
        && date.day() == 24
        && !matches!(
            date.weekday(),
            jiff::civil::Weekday::Saturday | jiff::civil::Weekday::Sunday
        )
}

fn is_independence_day_early_close(date: Date) -> bool {
    date.month() == 7
        && date.day() == 3
        && matches!(
            date.weekday(),
            jiff::civil::Weekday::Monday
                | jiff::civil::Weekday::Tuesday
                | jiff::civil::Weekday::Wednesday
                | jiff::civil::Weekday::Thursday
        )
}

/// Return whether a market-local date is an exchange trading date.  The
/// aggregation layer cannot open a CalendarManager (it owns only a leased
/// SQLite connection), so this deliberately mirrors the built-in calendar's
/// deterministic holiday set.  Manual/source overrides remain an explicit
/// follow-up at the engine boundary; they must not be silently replaced by a
/// weekday heuristic here.
fn market_is_trading_date(
    calendar: Option<&dyn CalendarScheduleResolver>,
    market: &str,
    date: Date,
) -> bool {
    if let Some(schedule) = calendar.and_then(|resolver| resolver.schedule(market, date)) {
        return !schedule.status.eq_ignore_ascii_case("closed") && !schedule.sessions.is_empty();
    }
    if matches!(
        date.weekday(),
        jiff::civil::Weekday::Saturday | jiff::civil::Weekday::Sunday
    ) {
        return false;
    }
    match market {
        "US" => !us_market_holiday(date),
        "CN" => !mainland_market_holiday(date),
        // The built-in HK calendar currently models split sessions but does
        // not ship a static holiday table; keep its weekday behavior rather
        // than inventing closures that could reject valid provider rows.
        "HK" => true,
        _ => false,
    }
}

fn us_market_holiday(date: Date) -> bool {
    let fixed = [(1_i8, 1_i8), (6, 19), (7, 4), (12, 25)];
    if fixed
        .iter()
        .any(|&(month, day)| observed_fixed_date(date.year(), month, day) == Some(date))
    {
        return true;
    }
    nth_weekday_date(date.year(), 1, jiff::civil::Weekday::Monday, 3) == Some(date)
        || nth_weekday_date(date.year(), 2, jiff::civil::Weekday::Monday, 3) == Some(date)
        || last_weekday_date(date.year(), 5, jiff::civil::Weekday::Monday) == Some(date)
        || nth_weekday_date(date.year(), 9, jiff::civil::Weekday::Monday, 1) == Some(date)
        || nth_weekday_date(date.year(), 11, jiff::civil::Weekday::Thursday, 4) == Some(date)
        || good_friday_date(date.year()) == Some(date)
}

fn mainland_market_holiday(date: Date) -> bool {
    matches!(
        (date.year(), date.month(), date.day()),
        (2025, 1, 1)
            | (2026, 1, 1)
            | (2026, 1, 2)
            | (2027, 1, 1)
            | (2025, 1, 28..=31)
            | (2025, 2, 3..=4)
            | (2026, 2, 16..=20)
            | (2026, 2, 23)
            | (2027, 2, 5)
            | (2027, 2, 8..=12)
            | (2025, 4, 4)
            | (2026, 4, 6)
            | (2027, 4, 5)
            | (2025, 5, 1..=2)
            | (2025, 5, 5)
            | (2026, 5, 1)
            | (2026, 5, 4..=5)
            | (2027, 5, 3..=5)
            | (2025, 6, 2)
            | (2026, 6, 19)
            | (2027, 6, 9)
            | (2026, 9, 25)
            | (2027, 9, 15)
            | (2025, 10, 1..=3)
            | (2025, 10, 6..=8)
            | (2026, 10, 1..=2)
            | (2026, 10, 5..=7)
            | (2027, 10, 1)
            | (2027, 10, 4..=7)
    )
}

fn observed_fixed_date(year: i16, month: i8, day: i8) -> Option<Date> {
    let base = Date::new(year, month, day).ok()?;
    let offset = match base.weekday() {
        jiff::civil::Weekday::Saturday => -1,
        jiff::civil::Weekday::Sunday => 1,
        _ => 0,
    };
    base.checked_add(offset.days()).ok()
}

fn nth_weekday_date(year: i16, month: i8, weekday: jiff::civil::Weekday, nth: u8) -> Option<Date> {
    let mut cursor = Date::new(year, month, 1).ok()?;
    let mut count = 0_u8;
    while cursor.month() == month {
        if cursor.weekday() == weekday {
            count = count.saturating_add(1);
            if count == nth {
                return Some(cursor);
            }
        }
        cursor = cursor.tomorrow().ok()?;
    }
    None
}

fn last_weekday_date(year: i16, month: i8, weekday: jiff::civil::Weekday) -> Option<Date> {
    let next_month = if month == 12 {
        Date::new(year.checked_add(1)?, 1, 1).ok()?
    } else {
        Date::new(year, month.saturating_add(1), 1).ok()?
    };
    let mut cursor = next_month.yesterday().ok()?;
    while cursor.month() == month {
        if cursor.weekday() == weekday {
            return Some(cursor);
        }
        cursor = cursor.yesterday().ok()?;
    }
    None
}

fn good_friday_date(year: i16) -> Option<Date> {
    let easter = easter_sunday_date(year)?;
    easter.checked_sub(2.days()).ok()
}

fn easter_sunday_date(year: i16) -> Option<Date> {
    if year <= 0 {
        return None;
    }
    let year = i32::from(year);
    let a = year % 19;
    let b = year / 100;
    let c = year % 100;
    let d = b / 4;
    let e = b % 4;
    let f = (b + 8) / 25;
    let g = (b - f + 1) / 3;
    let h = (19 * a + b - d - g + 15) % 30;
    let i = c / 4;
    let k = c % 4;
    let l = (32 + 2 * e + 2 * i - h - k) % 7;
    let m = (a + 11 * h + 22 * l) / 451;
    let month = (h + l - 7 * m + 114) / 31;
    let day = ((h + l - 7 * m + 114) % 31) + 1;
    Date::new(
        i16::try_from(year).ok()?,
        i8::try_from(month).ok()?,
        i8::try_from(day).ok()?,
    )
    .ok()
}

fn market_session_windows(
    calendar: Option<&dyn CalendarScheduleResolver>,
    market: &str,
    date: Date,
    session_scope: &str,
) -> Vec<(i32, i32)> {
    if let Some(schedule) = calendar.and_then(|resolver| resolver.schedule(market, date)) {
        if schedule.status.eq_ignore_ascii_case("closed") {
            return Vec::new();
        }
        let extended = session_scope.trim().eq_ignore_ascii_case("extended");
        return schedule
            .sessions
            .into_iter()
            .filter(|session| extended || session.kind.eq_ignore_ascii_case("regular"))
            .map(|session| (session.start_minute, session.end_minute))
            .collect();
    }
    if matches!(
        date.weekday(),
        jiff::civil::Weekday::Saturday | jiff::civil::Weekday::Sunday
    ) {
        return Vec::new();
    }
    match market {
        "US" => {
            let early_close = is_black_friday(date)
                || is_christmas_eve_early_close(date)
                || is_independence_day_early_close(date);
            let regular_end = if early_close { 780 } else { 960 };
            let after_end = if early_close { 1080 } else { 1200 };
            match session_scope.trim().to_ascii_lowercase().as_str() {
                "regular" => vec![(570, regular_end)],
                "extended" => vec![
                    (0, 240),
                    (240, 570),
                    (570, regular_end),
                    (regular_end, after_end),
                ],
                _ => vec![(570, regular_end)],
            }
        }
        "HK" => vec![(570, 720), (780, 960)],
        "CN" => vec![(570, 690), (780, 900)],
        _ => Vec::new(),
    }
}

pub(crate) fn resolve_aggregation_buckets(
    symbol: &str,
    session_scope: &str,
    calendar: Option<&dyn CalendarScheduleResolver>,
    target_ms: i64,
    start_time_ms: i64,
    end_time_ms: i64,
) -> Vec<(i64, i64)> {
    if start_time_ms >= end_time_ms || target_ms <= 0 {
        return Vec::new();
    }
    if let Some(market) = market_from_symbol(symbol) {
        let tz_name = market_timezone(market);
        if let Ok(tz) = TimeZone::get(tz_name) {
            let start_ts = jiff::Timestamp::from_millisecond(start_time_ms).ok();
            let end_ts = jiff::Timestamp::from_millisecond(end_time_ms.saturating_sub(1)).ok();
            if let (Some(s_ts), Some(e_ts)) = (start_ts, end_ts) {
                let start_date = s_ts.to_zoned(tz.clone()).date();
                let end_date = e_ts.to_zoned(tz.clone()).date();
                let mut buckets = Vec::new();
                let mut intersects_session = false;
                let mut current_date = start_date;
                while current_date <= end_date {
                    if calendar.is_some()
                        || !matches!(
                            current_date.weekday(),
                            jiff::civil::Weekday::Saturday | jiff::civil::Weekday::Sunday
                        )
                    {
                        for (start_min, end_min) in
                            market_session_windows(calendar, market, current_date, session_scope)
                        {
                            let s_hour = (start_min / 60) as i8;
                            let s_min = (start_min % 60) as i8;
                            let e_hour = (end_min / 60) as i8;
                            let e_min = (end_min % 60) as i8;

                            let s_ms = current_date
                                .at(s_hour, s_min, 0, 0)
                                .in_tz(tz_name)
                                .map(|z| z.timestamp().as_millisecond());
                            let e_ms = current_date
                                .at(e_hour, e_min, 0, 0)
                                .in_tz(tz_name)
                                .map(|z| z.timestamp().as_millisecond());
                            if let (Ok(session_start), Ok(session_end)) = (s_ms, e_ms)
                                && session_end > start_time_ms
                                && session_start < end_time_ms
                            {
                                intersects_session = true;
                                let mut cursor = session_start;
                                while cursor < session_end {
                                    let b_start = cursor;
                                    let b_end = cursor.saturating_add(target_ms).min(session_end);
                                    // Only a real session boundary may shorten a bar.
                                    // A query cutoff does not close the current bucket.
                                    if b_end <= end_time_ms
                                        && b_start >= start_time_ms
                                        && b_start < end_time_ms
                                    {
                                        buckets.push((b_start, b_end));
                                    }
                                    cursor = cursor.saturating_add(target_ms);
                                }
                            }
                        }
                    }
                    let next = match current_date.tomorrow() {
                        Ok(d) => d,
                        Err(_) => break,
                    };
                    current_date = next;
                }
                if intersects_session {
                    return buckets;
                }
            }
        }
    }

    // Generic UTC fallback (e.g. mock timestamps or unknown symbols)
    let first_bucket = floor_div(start_time_ms, target_ms).saturating_mul(target_ms);
    let last_bucket = floor_div(end_time_ms.saturating_sub(1), target_ms).saturating_mul(target_ms);
    if first_bucket > last_bucket {
        return Vec::new();
    }
    let mut buckets = Vec::new();
    let mut bucket = first_bucket;
    while bucket <= last_bucket {
        let b_end = bucket.saturating_add(target_ms);
        buckets.push((bucket, b_end));
        bucket = b_end;
    }
    buckets
}

fn date_start_utc_ms(date: Date) -> Option<i64> {
    date.at(0, 0, 0, 0)
        .in_tz("UTC")
        .ok()
        .map(|zoned| zoned.timestamp().as_millisecond())
}

fn timestamp_date(ms: i64, timezone: &TimeZone) -> Option<Date> {
    jiff::Timestamp::from_millisecond(ms)
        .ok()
        .map(|timestamp| timestamp.to_zoned(timezone.clone()).date())
}

fn utc_timestamp_date(ms: i64) -> Option<Date> {
    jiff::Timestamp::from_millisecond(ms)
        .ok()
        .map(|timestamp| timestamp.to_zoned(TimeZone::UTC).date())
}

fn period_label_date(date: Date, period: CalendarPeriod) -> Option<Date> {
    match period {
        CalendarPeriod::Day => Some(date),
        CalendarPeriod::Week => date
            .checked_sub((i64::from(date.weekday().to_monday_one_offset()) - 1).days())
            .ok(),
        CalendarPeriod::Month => Date::new(date.year(), date.month(), 1).ok(),
    }
}

fn next_period_label(label: Date, period: CalendarPeriod) -> Option<Date> {
    match period {
        CalendarPeriod::Day => label.tomorrow().ok(),
        CalendarPeriod::Week => label.checked_add(7.days()).ok(),
        CalendarPeriod::Month => label.checked_add(1.month()).ok(),
    }
}

fn period_bounds_utc_ms(label: Date, period: CalendarPeriod) -> Option<(i64, i64)> {
    let end_label = next_period_label(label, period)?;
    let start = date_start_utc_ms(label)?;
    let end = date_start_utc_ms(end_label)?.saturating_sub(1);
    Some((start, end))
}

fn session_window_bounds_ms(
    market: &str,
    date: Date,
    start_minute: i32,
    end_minute: i32,
) -> Option<(i64, i64)> {
    let timezone = market_timezone(market);
    let start = date
        .at((start_minute / 60) as i8, (start_minute % 60) as i8, 0, 0)
        .in_tz(timezone)
        .ok()?
        .timestamp()
        .as_millisecond();
    let end = date
        .at((end_minute / 60) as i8, (end_minute % 60) as i8, 0, 0)
        .in_tz(timezone)
        .ok()?
        .timestamp()
        .as_millisecond();
    (end > start).then_some((start, end))
}

fn period_day_segments(
    calendar: Option<&dyn CalendarScheduleResolver>,
    symbol: &str,
    session_scope: &str,
    trading_date: Date,
) -> Vec<(i64, i64)> {
    let Some(market) = market_from_symbol(symbol) else {
        return Vec::new();
    };
    let scope = session_scope.trim().to_ascii_lowercase();
    let mut segments = Vec::new();
    if market == "US" && scope == "extended" {
        // The 20:00-24:00 carry on the preceding local date is labelled as
        // the following trading date.  The current date contributes the
        // 00:00-20:00 windows (with DST/early-close boundaries applied).
        if let Some(previous) = trading_date.checked_sub(1.days()).ok()
            && (previous.weekday() == jiff::civil::Weekday::Sunday
                || !market_session_windows(calendar, market, previous, "regular").is_empty())
            && let Some(bounds) = session_window_bounds_ms(market, previous, 1200, 1440)
        {
            segments.push(bounds);
        }
        for (start, end) in market_session_windows(calendar, market, trading_date, "extended") {
            if let Some(bounds) = session_window_bounds_ms(market, trading_date, start, end) {
                segments.push(bounds);
            }
        }
        return segments;
    }

    for (start, end) in market_session_windows(calendar, market, trading_date, "regular") {
        if let Some(bounds) = session_window_bounds_ms(market, trading_date, start, end) {
            segments.push(bounds);
        }
    }
    segments
}

fn row_trading_date(
    calendar: Option<&dyn CalendarScheduleResolver>,
    symbol: &str,
    session_scope: &str,
    start_time_ms: i64,
) -> Option<Date> {
    let market = market_from_symbol(symbol)?;
    let timezone = TimeZone::get(market_timezone(market)).ok()?;
    let local_date = timestamp_date(start_time_ms, &timezone)?;
    let local = jiff::Timestamp::from_millisecond(start_time_ms)
        .ok()?
        .to_zoned(timezone);
    let minute = i32::from(local.hour()) * 60 + i32::from(local.minute());
    let scope = session_scope.trim().to_ascii_lowercase();
    if market == "US" && scope == "extended" && minute >= 1200 {
        let next = local_date.tomorrow().ok()?;
        if !market_session_windows(calendar, market, next, "regular").is_empty() {
            return Some(next);
        }
        return None;
    }
    let requested_scope = if scope == "extended" {
        "extended"
    } else {
        "regular"
    };
    let in_window = market_session_windows(calendar, market, local_date, requested_scope)
        .into_iter()
        .any(|(start, end)| minute >= start && minute < end);
    in_window.then_some(local_date)
}

fn aggregate_period_group(
    symbol: &str,
    interval: &str,
    label: Date,
    period: CalendarPeriod,
    rows: &[(Date, StoredBacktestCandle)],
) -> Result<StoredBacktestCandle, BacktestMarketDataStoreError> {
    let (start_time, end_time) = period_bounds_utc_ms(label, period)
        .ok_or_else(|| missing_coverage(symbol, interval, 0, 0))?;
    let mut ordered = rows.iter().collect::<Vec<_>>();
    ordered.sort_by_key(|(_, candle)| candle.start_time);
    let first = ordered
        .first()
        .ok_or_else(|| missing_coverage(symbol, interval, start_time, end_time))?;
    let last = ordered
        .last()
        .ok_or_else(|| missing_coverage(symbol, interval, start_time, end_time))?;
    let open = parse_decimal("open", &first.1.open)?;
    let close = parse_decimal("close", &last.1.close)?;
    let mut high = parse_decimal("high", &first.1.high)?;
    let mut low = parse_decimal("low", &first.1.low)?;
    let mut volume = Decimal::ZERO;
    for (_, candle) in ordered {
        let candle_high = parse_decimal("high", &candle.high)?;
        let candle_low = parse_decimal("low", &candle.low)?;
        if candle_high > high {
            high = candle_high;
        }
        if candle_low < low {
            low = candle_low;
        }
        volume = volume
            .checked_add(parse_decimal("volume", &candle.volume)?)
            .ok_or_else(|| {
                BacktestMarketDataStoreError::Validation("volume overflow".to_owned())
            })?;
    }
    Ok(StoredBacktestCandle {
        start_time,
        end_time,
        open: open.to_storage_text(),
        high: high.to_storage_text(),
        low: low.to_storage_text(),
        close: close.to_storage_text(),
        volume: volume.to_storage_text(),
    })
}

fn validate_period_day_coverage(
    calendar: Option<&dyn CalendarScheduleResolver>,
    symbol: &str,
    interval: &str,
    session_scope: &str,
    trading_date: Date,
    source_minutes: i64,
    rows: &[StoredBacktestCandle],
) -> Result<(), BacktestMarketDataStoreError> {
    // Larger source bars (for example, 1h bars spanning a half-hour close)
    // are intentionally accepted as observed provider bars.  Minute/5m
    // sources have deterministic boundaries and can be checked without
    // guessing how a provider split a session tail.
    if source_minutes > 5 {
        return Ok(());
    }
    if market_from_symbol(symbol).is_none() {
        return Err(BacktestMarketDataStoreError::Coverage(format!(
            "missing {interval} coverage for {symbol}"
        )));
    }
    let segments = period_day_segments(calendar, symbol, session_scope, trading_date);
    if segments.is_empty() {
        return Ok(());
    }
    let extended = session_scope.trim().eq_ignore_ascii_case("extended");
    for (segment_start, segment_end) in segments {
        let segment_rows = rows
            .iter()
            .filter(|row| row.start_time >= segment_start && row.start_time < segment_end)
            .collect::<Vec<_>>();
        // Extended-hours requests may legitimately contain only one part of
        // the broad provider window.  Validate continuity wherever data was
        // supplied, while regular requests require every configured session.
        if segment_rows.is_empty() {
            if extended {
                continue;
            }
            return Err(missing_coverage(
                symbol,
                interval,
                segment_start,
                segment_end,
            ));
        }
        let mut by_start = BTreeMap::new();
        for row in segment_rows {
            if by_start.insert(row.start_time, row).is_some() {
                return Err(missing_coverage(
                    symbol,
                    interval,
                    row.start_time,
                    row.end_time,
                ));
            }
        }
        let mut cursor = if extended {
            by_start.keys().next().copied().unwrap_or(segment_start)
        } else {
            segment_start
        };
        while cursor < segment_end {
            let Some(row) = by_start.get(&cursor) else {
                return Err(missing_coverage(symbol, interval, cursor, segment_end));
            };
            let expected_end = cursor
                .saturating_add(source_minutes.saturating_mul(60_000))
                .min(segment_end)
                .saturating_sub(1);
            if row.end_time < expected_end {
                return Err(missing_coverage(symbol, interval, cursor, expected_end));
            }
            cursor = expected_end.saturating_add(1);
        }
    }
    Ok(())
}

fn validate_period_trading_day_coverage(
    calendar: Option<&dyn CalendarScheduleResolver>,
    symbol: &str,
    interval: &str,
    label: Date,
    period: CalendarPeriod,
    rows: &[(Date, StoredBacktestCandle)],
) -> Result<(), BacktestMarketDataStoreError> {
    if !matches!(period, CalendarPeriod::Week | CalendarPeriod::Month) {
        return Ok(());
    }
    let Some(market) = market_from_symbol(symbol) else {
        return Err(missing_coverage(symbol, interval, 0, 0));
    };
    let Some(end_label) = next_period_label(label, period) else {
        return Err(missing_coverage(symbol, interval, 0, 0));
    };
    let present = rows.iter().map(|(date, _)| *date).collect::<BTreeSet<_>>();
    let mut date = label;
    while date < end_label {
        if market_is_trading_date(calendar, market, date) && !present.contains(&date) {
            let (start, end) = period_bounds_utc_ms(label, period).unwrap_or((0, 0));
            return Err(missing_coverage(symbol, interval, start, end));
        }
        date = date
            .tomorrow()
            .map_err(|_| missing_coverage(symbol, interval, 0, 0))?;
    }
    Ok(())
}

#[allow(clippy::too_many_arguments)]
pub(crate) fn aggregate_period_range(
    connection: &Connection,
    base_table: &str,
    source_interval: &str,
    source_minutes: Option<i64>,
    symbol: &str,
    interval: &str,
    session_scope: &str,
    calendar: Option<&dyn CalendarScheduleResolver>,
    start_time_ms: i64,
    end_time_ms: i64,
) -> Result<Vec<StoredBacktestCandle>, BacktestMarketDataStoreError> {
    let Some(period) = period_interval(interval) else {
        return Err(BacktestMarketDataStoreError::Validation(format!(
            "unsupported calendar interval: {interval}"
        )));
    };
    if start_time_ms >= end_time_ms {
        return Ok(Vec::new());
    }
    if !table_exists(connection, base_table)? {
        return Err(missing_coverage(
            symbol,
            interval,
            start_time_ms,
            end_time_ms,
        ));
    }
    let padding = match period {
        CalendarPeriod::Day => 2 * 86_400_000,
        CalendarPeriod::Week => 8 * 86_400_000,
        CalendarPeriod::Month => 32 * 86_400_000,
    };
    let source_start = start_time_ms.saturating_sub(padding);
    let source_end = end_time_ms.saturating_add(padding);
    let source = read_direct_range(connection, base_table, source_start, source_end)?;
    if source.is_empty() {
        return Err(missing_coverage(
            symbol,
            interval,
            start_time_ms,
            end_time_ms,
        ));
    }
    let source_is_daily = source_interval.eq_ignore_ascii_case("1d");
    let mut grouped: BTreeMap<Date, Vec<(Date, StoredBacktestCandle)>> = BTreeMap::new();
    for candle in source {
        let trading_date = if source_is_daily {
            utc_timestamp_date(candle.start_time).filter(|date| {
                let Some(market) = market_from_symbol(symbol) else {
                    return false;
                };
                calendar
                    .and_then(|resolver| resolver.schedule(market, *date))
                    .is_none_or(|schedule| {
                        !schedule.status.eq_ignore_ascii_case("closed")
                            && !schedule.sessions.is_empty()
                    })
            })
        } else {
            row_trading_date(calendar, symbol, session_scope, candle.start_time)
        };
        let Some(trading_date) = trading_date else {
            continue;
        };
        let Some(label) = period_label_date(trading_date, period) else {
            continue;
        };
        grouped
            .entry(label)
            .or_default()
            .push((trading_date, candle));
    }
    if grouped.is_empty() {
        return Err(missing_coverage(
            symbol,
            interval,
            start_time_ms,
            end_time_ms,
        ));
    }

    let mut output = Vec::with_capacity(grouped.len());
    for (label, rows) in grouped {
        let Some((_label_start, label_end)) = period_bounds_utc_ms(label, period) else {
            continue;
        };
        // Never emit a shortened calendar period merely because the caller's
        // query cutoff falls inside it.  This mirrors the Go closed-period
        // behavior and prevents a partial day/week/month from being replayed.
        if label_end < start_time_ms || label_end >= end_time_ms {
            continue;
        }
        if let Some(minutes) = source_minutes {
            let mut by_day: BTreeMap<Date, Vec<StoredBacktestCandle>> = BTreeMap::new();
            for (date, candle) in &rows {
                by_day.entry(*date).or_default().push(candle.clone());
            }
            for (date, day_rows) in by_day {
                validate_period_day_coverage(
                    calendar,
                    symbol,
                    interval,
                    session_scope,
                    date,
                    minutes,
                    &day_rows,
                )?;
            }
        } else if source_is_daily {
            validate_period_trading_day_coverage(calendar, symbol, interval, label, period, &rows)?;
        }
        output.push(aggregate_period_group(
            symbol, interval, label, period, &rows,
        )?);
    }
    Ok(output)
}

#[allow(clippy::too_many_arguments)]
pub(crate) fn aggregate_range(
    connection: &Connection,
    base_table: &str,
    source_minutes: i64,
    symbol: &str,
    interval: &str,
    session_scope: &str,
    calendar: Option<&dyn CalendarScheduleResolver>,
    start_time_ms: i64,
    end_time_ms: i64,
) -> Result<Vec<StoredBacktestCandle>, BacktestMarketDataStoreError> {
    let target_minutes = match interval_minutes(interval) {
        Some(minutes) if minutes > source_minutes && minutes % source_minutes == 0 => minutes,
        _ => {
            return Err(BacktestMarketDataStoreError::Validation(format!(
                "unsupported aggregate interval: {interval}"
            )));
        }
    };
    if !table_exists(connection, base_table)? {
        return Err(missing_coverage(
            symbol,
            interval,
            start_time_ms,
            end_time_ms,
        ));
    }
    let target_ms = target_minutes.saturating_mul(60_000);
    let source_ms = source_minutes.saturating_mul(60_000);

    let buckets = resolve_aggregation_buckets(
        symbol,
        session_scope,
        calendar,
        target_ms,
        start_time_ms,
        end_time_ms,
    );
    if buckets.is_empty() {
        return Ok(Vec::new());
    }

    let source_start = buckets.first().map(|b| b.0).unwrap_or(start_time_ms);
    let source_end = buckets.last().map(|b| b.1).unwrap_or(end_time_ms);
    let source = read_direct_range(connection, base_table, source_start, source_end)?;
    if source.is_empty() {
        return Err(missing_coverage(
            symbol,
            interval,
            start_time_ms,
            end_time_ms,
        ));
    }

    let mut by_bucket: BTreeMap<i64, Vec<StoredBacktestCandle>> = BTreeMap::new();
    for candle in source {
        if let Ok(idx) = buckets.binary_search_by(|&(b_start, b_end)| {
            if candle.start_time < b_start {
                std::cmp::Ordering::Greater
            } else if candle.start_time >= b_end {
                std::cmp::Ordering::Less
            } else {
                std::cmp::Ordering::Equal
            }
        }) {
            by_bucket.entry(buckets[idx].0).or_default().push(candle);
        }
    }

    let mut aggregated = Vec::with_capacity(buckets.len());
    for &(bucket_start, bucket_end) in &buckets {
        let rows = by_bucket
            .get(&bucket_start)
            .ok_or_else(|| missing_coverage(symbol, interval, bucket_start, bucket_end))?;
        aggregated.push(aggregate_bucket(
            symbol,
            interval,
            bucket_start,
            bucket_end,
            source_ms,
            rows,
        )?);
    }
    Ok(aggregated)
}

pub(crate) fn aggregate_bucket(
    symbol: &str,
    interval: &str,
    bucket_start: i64,
    bucket_end: i64,
    source_ms: i64,
    rows: &[StoredBacktestCandle],
) -> Result<StoredBacktestCandle, BacktestMarketDataStoreError> {
    let bucket_ms = bucket_end.saturating_sub(bucket_start);
    let factor = usize::try_from(bucket_ms / source_ms).unwrap_or(0);
    if factor == 0 || rows.len() != factor {
        return Err(missing_coverage(symbol, interval, bucket_start, bucket_end));
    }
    let mut ordered = rows.to_vec();
    ordered.sort_by_key(|candle| candle.start_time);
    let mut values = Vec::with_capacity(factor);
    for (index, candle) in ordered.iter().enumerate() {
        let expected_start = bucket_start.saturating_add((index as i64).saturating_mul(source_ms));
        let expected_end = expected_start.saturating_add(source_ms).saturating_sub(1);
        if candle.start_time != expected_start || candle.end_time != expected_end {
            return Err(missing_coverage(symbol, interval, bucket_start, bucket_end));
        }
        values.push((
            parse_decimal("open", &candle.open)?,
            parse_decimal("high", &candle.high)?,
            parse_decimal("low", &candle.low)?,
            parse_decimal("close", &candle.close)?,
            parse_decimal("volume", &candle.volume)?,
        ));
    }
    let first = values
        .first()
        .ok_or_else(|| missing_coverage(symbol, interval, bucket_start, bucket_end))?;
    let last = values
        .last()
        .ok_or_else(|| missing_coverage(symbol, interval, bucket_start, bucket_end))?;
    let high = values.iter().map(|value| value.1).max().unwrap_or(first.1);
    let low = values.iter().map(|value| value.2).min().unwrap_or(first.2);
    let volume = values.iter().try_fold(Decimal::ZERO, |sum, value| {
        sum.checked_add(value.4)
            .ok_or_else(|| BacktestMarketDataStoreError::Validation("volume overflow".to_owned()))
    })?;
    Ok(StoredBacktestCandle {
        start_time: bucket_start,
        end_time: bucket_end.saturating_sub(1),
        open: first.0.to_storage_text(),
        high: high.to_storage_text(),
        low: low.to_storage_text(),
        close: last.3.to_storage_text(),
        volume: volume.to_storage_text(),
    })
}

fn parse_decimal(name: &str, value: &str) -> Result<Decimal, BacktestMarketDataStoreError> {
    value
        .parse::<Decimal>()
        .map_err(|error| BacktestMarketDataStoreError::Validation(format!("{name}: {error}")))
}

pub(crate) fn missing_coverage(
    symbol: &str,
    interval: &str,
    start_time_ms: i64,
    end_time_ms: i64,
) -> BacktestMarketDataStoreError {
    BacktestMarketDataStoreError::Coverage(format!(
        "missing {interval} coverage for {symbol} [{start_time_ms}, {end_time_ms})"
    ))
}

pub(crate) fn floor_div(value: i64, divisor: i64) -> i64 {
    value.div_euclid(divisor)
}
