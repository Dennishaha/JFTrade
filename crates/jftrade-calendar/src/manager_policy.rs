use jftrade_kernel::WireTimestamp;
use jiff::{Timestamp, civil::Date as CivilDate, tz::TimeZone};
use time::{Date, Duration, Month, OffsetDateTime, Time, UtcOffset, Weekday};

use crate::{
    BUILTIN_SOURCE_ID, CalendarManagerSettings, CalendarManualOverride, CalendarSessionWindow,
    TradingDaySchedule,
};

pub(crate) fn manual_schedule(
    settings: &CalendarManagerSettings,
    market: &str,
    at: WireTimestamp,
) -> Option<TradingDaySchedule> {
    manual_schedule_for_date(settings, market, at.into_inner().date().to_string(), at)
}

/// Resolve a manual override using the market-local date first.  A number of
/// existing callers pass a UTC midnight as a date-only value, however; for a
/// west-of-UTC market that instant belongs to the previous local date.  Keep
/// the local-time semantics authoritative while accepting that legacy shape as
/// a compatibility fallback.
pub(crate) fn manual_schedule_with_raw_fallback(
    settings: &CalendarManagerSettings,
    market: &str,
    normalized_at: WireTimestamp,
    raw_at: WireTimestamp,
) -> Option<TradingDaySchedule> {
    let normalized_date = normalized_at.into_inner().date();
    if let Some(schedule) = manual_schedule(settings, market, normalized_at) {
        return Some(schedule);
    }
    let raw = raw_at.into_inner();
    if raw.offset() != UtcOffset::UTC || raw.time() != Time::MIDNIGHT {
        return None;
    }
    let raw_date = raw.date();
    (raw_date != normalized_date)
        .then(|| manual_schedule_for_date(settings, market, raw_date.to_string(), normalized_at))?
}

fn manual_schedule_for_date(
    settings: &CalendarManagerSettings,
    market: &str,
    date: String,
    at: WireTimestamp,
) -> Option<TradingDaySchedule> {
    settings
        .manual_overrides
        .iter()
        .find(|manual| {
            let manual_market = normalize_market(&manual.market);
            (manual_market == market || (manual_market == "CN" && matches!(market, "SH" | "SZ")))
                && manual.date.trim() == date
        })
        .map(|manual| schedule_from_manual(manual, market, at))
}

fn schedule_from_manual(
    manual: &CalendarManualOverride,
    market: &str,
    at: WireTimestamp,
) -> TradingDaySchedule {
    let mut sessions = manual
        .sessions
        .iter()
        .filter(|session| session.end_minute > session.start_minute)
        .map(|session| CalendarSessionWindow {
            kind: normalize_session_kind(&session.kind),
            start_minute: session.start_minute,
            end_minute: session.end_minute,
        })
        .collect::<Vec<_>>();
    sessions.sort_by_key(|session| (session.start_minute, session.end_minute));
    TradingDaySchedule {
        market_code: market.to_owned(),
        date: at,
        status: normalize_status(&manual.status),
        sessions,
        reason: manual.reason.trim().to_owned(),
        source_id: crate::MANUAL_OVERRIDE_SOURCE_ID.to_owned(),
        observed: manual.observed,
        updated_at: None,
    }
}

pub(crate) fn builtin_schedule(market: &str, at: WireTimestamp) -> TradingDaySchedule {
    let local_date = at.into_inner().date();
    let closed = matches!(local_date.weekday(), Weekday::Saturday | Weekday::Sunday);
    let sessions = if closed {
        Vec::new()
    } else if market == "US" {
        us_schedule_sessions(local_date)
    } else {
        builtin_sessions(market)
    };
    let (status, reason, observed, sessions) = if closed {
        ("closed", "weekend", false, Vec::new())
    } else if market == "US" {
        us_holiday_or_early_close(local_date).map_or_else(
            || ("open", "", false, sessions.clone()),
            |(status, reason, observed, holiday_sessions)| {
                (
                    status,
                    reason,
                    observed,
                    holiday_sessions.unwrap_or_else(|| sessions.clone()),
                )
            },
        )
    } else if matches!(market, "CN" | "SH" | "SZ") {
        mainland_holiday_reason(local_date).map_or_else(
            || ("open", "", false, sessions),
            |reason| ("closed", reason, false, Vec::new()),
        )
    } else {
        ("open", "", false, sessions)
    };
    TradingDaySchedule {
        market_code: market.to_owned(),
        date: at,
        status: status.to_owned(),
        sessions,
        reason: reason.to_owned(),
        source_id: BUILTIN_SOURCE_ID.to_owned(),
        observed,
        updated_at: None,
    }
}

fn us_schedule_sessions(day: Date) -> Vec<CalendarSessionWindow> {
    let regular_end = if us_early_close_reason(day).is_some() {
        780
    } else {
        960
    };
    let after_end = if regular_end == 780 { 1_080 } else { 1_200 };
    vec![
        session("overnight", 0, 240),
        session("pre", 240, 570),
        session("regular", 570, regular_end),
        session("after", regular_end, after_end),
    ]
}

fn session(kind: &str, start_minute: i32, end_minute: i32) -> CalendarSessionWindow {
    CalendarSessionWindow {
        kind: kind.to_owned(),
        start_minute,
        end_minute,
    }
}

fn us_holiday_or_early_close(
    day: Date,
) -> Option<(
    &'static str,
    &'static str,
    bool,
    Option<Vec<CalendarSessionWindow>>,
)> {
    if observed_fixed_holiday(day.year() + 1, Month::January, 1) == Some(day) {
        return Some(("closed", "new_years_day_observed", true, None));
    }
    if let Some((reason, observed)) = us_holiday_reason(day) {
        return Some(("closed", reason, observed, None));
    }
    us_early_close_reason(day).map(|reason| {
        (
            "early_close",
            reason,
            false,
            Some(us_schedule_sessions(day)),
        )
    })
}

fn us_holiday_reason(day: Date) -> Option<(&'static str, bool)> {
    let candidates = [
        (
            "new_years_day",
            observed_fixed_holiday(day.year(), Month::January, 1),
            true,
        ),
        (
            "martin_luther_king_jr_day",
            nth_weekday_of_month(day.year(), Month::January, Weekday::Monday, 3),
            false,
        ),
        (
            "presidents_day",
            nth_weekday_of_month(day.year(), Month::February, Weekday::Monday, 3),
            false,
        ),
        ("good_friday", good_friday(day.year()), false),
        (
            "memorial_day",
            last_weekday_of_month(day.year(), Month::May, Weekday::Monday),
            false,
        ),
        (
            "juneteenth",
            observed_fixed_holiday(day.year(), Month::June, 19),
            true,
        ),
        (
            "independence_day",
            observed_fixed_holiday(day.year(), Month::July, 4),
            true,
        ),
        (
            "labor_day",
            nth_weekday_of_month(day.year(), Month::September, Weekday::Monday, 1),
            false,
        ),
        (
            "thanksgiving",
            nth_weekday_of_month(day.year(), Month::November, Weekday::Thursday, 4),
            false,
        ),
        (
            "christmas_day",
            observed_fixed_holiday(day.year(), Month::December, 25),
            true,
        ),
    ];
    candidates
        .into_iter()
        .find_map(|(reason, date, observed_if_shifted)| {
            (date == Some(day)).then_some((
                reason,
                date != date_for(day.year(), reason) && observed_if_shifted,
            ))
        })
}

fn us_early_close_reason(day: Date) -> Option<&'static str> {
    if independence_day_early_close(day) {
        Some("independence_day_early_close")
    } else if is_black_friday(day) {
        Some("black_friday_early_close")
    } else if is_christmas_eve_early_close(day) {
        Some("christmas_eve_early_close")
    } else {
        None
    }
}

fn date_for(year: i32, reason: &str) -> Option<Date> {
    match reason {
        "new_years_day" => date(year, Month::January, 1),
        "juneteenth" => date(year, Month::June, 19),
        "independence_day" => date(year, Month::July, 4),
        "christmas_day" => date(year, Month::December, 25),
        _ => None,
    }
}

fn observed_fixed_holiday(year: i32, month: Month, day: u8) -> Option<Date> {
    let base = date(year, month, day)?;
    let delta = match base.weekday() {
        Weekday::Saturday => -1,
        Weekday::Sunday => 1,
        _ => 0,
    };
    base.checked_add(Duration::days(delta))
}

fn nth_weekday_of_month(year: i32, month: Month, weekday: Weekday, nth: u8) -> Option<Date> {
    let first = date(year, month, 1)?;
    let delta = (i16::from(weekday.number_from_monday())
        - i16::from(first.weekday().number_from_monday())
        + 7)
        % 7;
    let days = i64::from(delta) + 7 * i64::from(nth.saturating_sub(1));
    first.checked_add(Duration::days(days))
}

fn last_weekday_of_month(year: i32, month: Month, weekday: Weekday) -> Option<Date> {
    let next_month = if month == Month::December {
        date(year + 1, Month::January, 1)?
    } else {
        date(year, Month::try_from(u8::from(month) + 1).ok()?, 1)?
    };
    let last = next_month.checked_add(Duration::days(-1))?;
    let delta = (i16::from(last.weekday().number_from_monday())
        - i16::from(weekday.number_from_monday())
        + 7)
        % 7;
    last.checked_add(Duration::days(-i64::from(delta)))
}

fn good_friday(year: i32) -> Option<Date> {
    let easter = easter_sunday(year)?;
    easter.checked_add(Duration::days(-2))
}

fn easter_sunday(year: i32) -> Option<Date> {
    if year <= 0 {
        return None;
    }
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
    date(
        year,
        Month::try_from(u8::try_from(month).ok()?).ok()?,
        u8::try_from(day).ok()?,
    )
}

fn independence_day_early_close(day: Date) -> bool {
    let Some(july_fourth) = date(day.year(), Month::July, 4) else {
        return false;
    };
    let candidate = match july_fourth.weekday() {
        Weekday::Saturday | Weekday::Sunday => july_fourth.checked_add(Duration::days(-2)),
        _ => july_fourth.checked_add(Duration::days(-1)),
    };
    candidate == Some(day) && !matches!(day.weekday(), Weekday::Saturday | Weekday::Sunday)
}

fn is_black_friday(day: Date) -> bool {
    nth_weekday_of_month(day.year(), Month::November, Weekday::Thursday, 4)
        .and_then(|date| date.checked_add(Duration::days(1)))
        == Some(day)
}

fn is_christmas_eve_early_close(day: Date) -> bool {
    day.month() == Month::December
        && day.day() == 24
        && !matches!(day.weekday(), Weekday::Saturday | Weekday::Sunday)
        && observed_fixed_holiday(day.year(), Month::December, 25) != Some(day)
}

fn mainland_holiday_reason(day: Date) -> Option<&'static str> {
    let key = (day.year(), day.month() as u8, day.day());
    match key {
        (2025, 1, 1) | (2026, 1, 1) | (2026, 1, 2) | (2027, 1, 1) => Some("new_year_holiday"),
        (2025, 1, 28..=31)
        | (2025, 2, 3..=4)
        | (2026, 2, 16..=20)
        | (2026, 2, 23)
        | (2027, 2, 5)
        | (2027, 2, 8..=12) => Some("spring_festival_holiday"),
        (2025, 4, 4) | (2026, 4, 6) | (2027, 4, 5) => Some("qingming_festival_holiday"),
        (2025, 5, 1..=2) | (2025, 5, 5) | (2026, 5, 1) | (2026, 5, 4..=5) | (2027, 5, 3..=5) => {
            Some("labour_day_holiday")
        }
        (2025, 6, 2) | (2026, 6, 19) | (2027, 6, 9) => Some("dragon_boat_festival_holiday"),
        (2026, 9, 25) | (2027, 9, 15) => Some("mid_autumn_festival_holiday"),
        (2025, 10, 1..=3)
        | (2025, 10, 6..=8)
        | (2026, 10, 1..=2)
        | (2026, 10, 5..=7)
        | (2027, 10, 1)
        | (2027, 10, 4..=7) => Some("national_day_holiday"),
        _ => None,
    }
}

fn date(year: i32, month: Month, day: u8) -> Option<Date> {
    Date::from_calendar_date(year, month, day).ok()
}

/// Resolve an instant to the market-local midnight used by calendar dates.
/// This keeps UTC callers from accidentally looking up the previous/next
/// exchange day around a timezone boundary.
pub(crate) fn market_day_start(
    market: &str,
    at: WireTimestamp,
) -> Result<WireTimestamp, crate::CalendarManagerError> {
    let timezone = match market {
        "US" => "America/New_York",
        "HK" => "Asia/Hong_Kong",
        "CN" | "SH" | "SZ" => "Asia/Shanghai",
        _ => return Ok(at),
    };
    let timestamp = at
        .to_string()
        .parse::<Timestamp>()
        .map_err(|error| crate::CalendarManagerError::InvalidSettings(error.to_string()))?;
    let zone = TimeZone::get(timezone)
        .map_err(|error| crate::CalendarManagerError::InvalidSettings(error.to_string()))?;
    let local = timestamp.to_zoned(zone);
    let date = local.date();
    let date = CivilDate::new(date.year(), date.month(), date.day())
        .map_err(|error| crate::CalendarManagerError::InvalidSettings(error.to_string()))?;
    let midnight = date
        .in_tz(timezone)
        .map_err(|error| crate::CalendarManagerError::InvalidSettings(error.to_string()))?;
    let instant =
        OffsetDateTime::from_unix_timestamp_nanos(midnight.timestamp().as_nanosecond())
            .map_err(|error| crate::CalendarManagerError::InvalidSettings(error.to_string()))?;
    let offset = UtcOffset::from_whole_seconds(midnight.offset().seconds())
        .map_err(|error| crate::CalendarManagerError::InvalidSettings(error.to_string()))?;
    Ok(WireTimestamp::from_offset_datetime(
        instant.to_offset(offset),
    ))
}

fn builtin_sessions(market: &str) -> Vec<CalendarSessionWindow> {
    let session = |kind: &str, start_minute, end_minute| CalendarSessionWindow {
        kind: kind.to_owned(),
        start_minute,
        end_minute,
    };
    match market {
        "US" => vec![
            session("overnight", 0, 240),
            session("pre", 240, 570),
            session("regular", 570, 960),
            session("after", 960, 1200),
        ],
        "HK" => vec![session("regular", 570, 720), session("regular", 780, 960)],
        "CN" | "SH" | "SZ" => {
            vec![session("regular", 570, 690), session("regular", 780, 900)]
        }
        _ => vec![session("regular", 570, 960)],
    }
}

fn normalize_market(market: &str) -> String {
    market.trim().to_uppercase()
}

fn normalize_status(status: &str) -> String {
    match status.trim().to_lowercase().as_str() {
        "open" | "closed" | "early_close" | "special" => status.trim().to_lowercase(),
        _ => "unknown".to_owned(),
    }
}

fn normalize_session_kind(kind: &str) -> String {
    match kind.trim().to_lowercase().as_str() {
        "closed" | "pre" | "regular" | "after" | "overnight" => kind.trim().to_lowercase(),
        _ => "unknown".to_owned(),
    }
}
