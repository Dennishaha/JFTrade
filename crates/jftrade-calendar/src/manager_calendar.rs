use jftrade_kernel::WireTimestamp;
use jiff::{Timestamp, civil::Date as CivilDate, tz::TimeZone};
use time::{Date, Duration, Month, OffsetDateTime, Time, UtcOffset, Weekday};

use crate::manager::{normalize_market, supported_market};
use crate::manager_policy::market_day_start;
use crate::{CalendarManagerError, CalendarSnapshot, CalendarSourcePolicy};

pub(crate) fn fetch_window(
    now: OffsetDateTime,
) -> Result<(WireTimestamp, WireTimestamp), CalendarManagerError> {
    fetch_window_with_offset(now, now.offset())
}

/// Build the inclusive two-year fetch range in the market's local timezone.
///
/// The current year is resolved from the instant in the exchange timezone,
/// then each boundary is converted independently so an offset transition
/// between the current instant and either boundary cannot leak into the
/// request range.
pub(crate) fn fetch_window_for_market(
    now: OffsetDateTime,
    market: &str,
) -> Result<(WireTimestamp, WireTimestamp), CalendarManagerError> {
    let Some(timezone) = market_timezone(market) else {
        return fetch_window(now);
    };
    let timestamp = WireTimestamp::from_offset_datetime(now)
        .to_string()
        .parse::<Timestamp>()
        .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
    let zone = TimeZone::get(timezone)
        .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
    let current = timestamp.to_zoned(zone.clone());
    let year = current.date().year();
    let next_year = year.checked_add(1).ok_or_else(|| {
        CalendarManagerError::InvalidSettings("calendar year overflow".to_owned())
    })?;
    let from_date = CivilDate::new(year, 1, 1)
        .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
    let to_date = CivilDate::new(next_year, 12, 31)
        .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
    let from = from_date
        .at(0, 0, 0, 0)
        .to_zoned(zone.clone())
        .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
    let to = to_date
        .at(23, 59, 59, 0)
        .to_zoned(zone)
        .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
    Ok((zoned_to_wire(from)?, zoned_to_wire(to)?))
}

fn fetch_window_with_offset(
    now: OffsetDateTime,
    offset: UtcOffset,
) -> Result<(WireTimestamp, WireTimestamp), CalendarManagerError> {
    let from_date = Date::from_calendar_date(now.year(), Month::January, 1)
        .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
    let to_date = Date::from_calendar_date(now.year() + 1, Month::December, 31)
        .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
    Ok((
        WireTimestamp::from_offset_datetime(
            from_date.with_time(Time::MIDNIGHT).assume_offset(offset),
        ),
        WireTimestamp::from_offset_datetime(
            to_date
                .with_time(Time::from_hms(23, 59, 59).expect("valid end-of-day time"))
                .assume_offset(offset),
        ),
    ))
}

fn zoned_to_wire(zoned: jiff::Zoned) -> Result<WireTimestamp, CalendarManagerError> {
    let instant = OffsetDateTime::from_unix_timestamp_nanos(zoned.timestamp().as_nanosecond())
        .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
    let offset = UtcOffset::from_whole_seconds(zoned.offset().seconds())
        .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
    Ok(WireTimestamp::from_offset_datetime(
        instant.to_offset(offset),
    ))
}

pub(crate) fn market_timezone(market: &str) -> Option<&'static str> {
    match market.trim().to_ascii_uppercase().as_str() {
        "US" => Some("America/New_York"),
        "HK" => Some("Asia/Hong_Kong"),
        "CN" | "SH" | "SZ" => Some("Asia/Shanghai"),
        _ => None,
    }
}

pub(crate) fn validate_snapshot(snapshot: &CalendarSnapshot) -> Result<(), String> {
    if snapshot.source_id.trim().is_empty() || snapshot.market_code.trim().is_empty() {
        return Err("snapshot marketCode and sourceId are required".to_owned());
    }
    let market = normalize_market(&snapshot.market_code);
    if !supported_market(&market) {
        return Err(format!(
            "unsupported snapshot market {:?}",
            snapshot.market_code
        ));
    }
    if is_zero_timestamp(snapshot.from) || is_zero_timestamp(snapshot.to) {
        return Err("missing snapshot range".to_owned());
    }
    if snapshot.to < snapshot.from {
        return Err("snapshot range is invalid".to_owned());
    }
    if snapshot.schedules.is_empty() {
        return Err("no schedules parsed".to_owned());
    }
    for schedule in &snapshot.schedules {
        if is_zero_timestamp(schedule.date) {
            return Err("schedule has empty date".to_owned());
        }
        if !market_matches(&schedule.market_code, &market) {
            return Err("snapshot schedule market does not match snapshot market".to_owned());
        }
        if !schedule_within_market_range(schedule.date, snapshot.from, snapshot.to, &market) {
            return Err("snapshot schedule is outside its market or range".to_owned());
        }
        validate_session_windows(&schedule.sessions)?;
    }
    Ok(())
}

pub(crate) fn snapshot_key(source_id: &str, market: &str, year: i32) -> String {
    format!(
        "{}|{}|{year:04}",
        source_id.trim(),
        normalize_market(market)
    )
}

pub(crate) fn snapshot_identity(snapshot: &CalendarSnapshot) -> String {
    format!(
        "{}|{}|{}|{}",
        snapshot.source_id.trim(),
        normalize_market(&snapshot.market_code),
        snapshot.from,
        snapshot.to
    )
}

pub(crate) fn candidate_markets(market: &str) -> &'static [&'static str] {
    match market {
        "SH" => &["SH", "CN"],
        "SZ" => &["SZ", "CN"],
        "CN" => &["CN", "SH", "SZ"],
        "HK" => &["HK"],
        _ => &["US"],
    }
}

pub(crate) fn market_matches(left: &str, right: &str) -> bool {
    let left = normalize_market(left);
    let right = normalize_market(right);
    left == right
        || (left == "CN" && matches!(right.as_str(), "SH" | "SZ"))
        || (right == "CN" && matches!(left.as_str(), "SH" | "SZ"))
}

pub(crate) fn same_market_day(left: WireTimestamp, right: WireTimestamp, market: &str) -> bool {
    match (
        market_day_start(market, left),
        market_day_start(market, right),
    ) {
        (Ok(left), Ok(right)) => left.into_inner().date() == right.into_inner().date(),
        _ => left.into_inner().date() == right.into_inner().date(),
    }
}

pub(crate) fn snapshot_fresh(
    snapshot: &CalendarSnapshot,
    policy: &CalendarSourcePolicy,
    now: OffsetDateTime,
) -> bool {
    if !is_zero_timestamp(snapshot.valid_until) && snapshot.valid_until.into_inner() < now {
        return false;
    }
    if policy.stale_after_hours > 0
        && !is_zero_timestamp(snapshot.fetched_at)
        && snapshot
            .fetched_at
            .into_inner()
            .checked_add(Duration::hours(i64::from(policy.stale_after_hours)))
            .is_none_or(|expiry| expiry < now)
    {
        return false;
    }
    true
}

fn schedule_within_market_range(
    schedule: WireTimestamp,
    from: WireTimestamp,
    to: WireTimestamp,
    market: &str,
) -> bool {
    let Ok(schedule) = market_local_date(market, schedule) else {
        return false;
    };
    let Ok(from) = market_local_date(market, from) else {
        return false;
    };
    let Ok(to) = market_local_date(market, to) else {
        return false;
    };
    schedule >= from && schedule <= to
}

fn validate_session_windows(sessions: &[crate::CalendarSessionWindow]) -> Result<(), String> {
    let mut ordered = sessions.iter().collect::<Vec<_>>();
    ordered.sort_by_key(|session| (session.start_minute, session.end_minute));
    for (index, session) in ordered.iter().enumerate() {
        if session.start_minute < 0
            || session.end_minute > 24 * 60
            || session.start_minute >= session.end_minute
        {
            return Err("session window must be within one day and open before close".to_owned());
        }
        if index > 0 && ordered[index - 1].end_minute > session.start_minute {
            return Err("session windows must not overlap".to_owned());
        }
    }
    Ok(())
}

fn is_zero_timestamp(value: WireTimestamp) -> bool {
    let value = value.into_inner();
    value.year() == 1
        && value.month() == Month::January
        && value.day() == 1
        && value.time() == Time::MIDNIGHT
        && value.offset() == time::UtcOffset::UTC
}

pub(crate) fn explicit_utc_date_input(value: WireTimestamp) -> bool {
    let value = value.into_inner();
    value.offset() == time::UtcOffset::UTC && value.time() == Time::MIDNIGHT
}

pub(crate) fn is_weekend(weekday: Weekday) -> bool {
    matches!(weekday, Weekday::Saturday | Weekday::Sunday)
}

pub(crate) fn market_local_date(
    market: &str,
    at: WireTimestamp,
) -> Result<Date, CalendarManagerError> {
    Ok(market_day_start(market, at)?.into_inner().date())
}

pub(crate) fn market_local_year(
    market: &str,
    at: WireTimestamp,
) -> Result<i32, CalendarManagerError> {
    Ok(market_local_date(market, at)?.year())
}

pub(crate) fn wire_text(at: OffsetDateTime) -> String {
    WireTimestamp::from_offset_datetime(at).to_string()
}
