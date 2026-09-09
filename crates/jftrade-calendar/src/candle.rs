//! Candle completion uses exchange-local dates and the authoritative trading schedule.
use jftrade_kernel::WireTimestamp;
use jiff::{Timestamp, ToSpan, civil::Date, tz::TimeZone};
use time::OffsetDateTime;

use crate::{CalendarManager, CalendarManagerError, TradingDaySchedule};

pub fn candle_is_closed(
    calendar: Option<&CalendarManager>,
    market: &str,
    period: &str,
    open: OffsetDateTime,
    now: OffsetDateTime,
    sessions: &[&str],
) -> Result<bool, CalendarManagerError> {
    let timezone = crate::manager_calendar::market_timezone(market)
        .or(match market {
            "JP" => Some("Asia/Tokyo"),
            "SG" => Some("Asia/Singapore"),
            "MY" => Some("Asia/Kuala_Lumpur"),
            "AU" => Some("Australia/Sydney"),
            "CA" => Some("America/Toronto"),
            _ => None,
        })
        .ok_or_else(|| CalendarManagerError::UnsupportedMarket(market.to_owned()))?;
    let zone = TimeZone::get(timezone).map_err(invalid)?;
    let open_ts = Timestamp::from_nanosecond(open.unix_timestamp_nanos()).map_err(invalid)?;
    let now_ts = Timestamp::from_nanosecond(now.unix_timestamp_nanos()).map_err(invalid)?;
    if now_ts < open_ts {
        return Ok(false);
    }
    let local = open_ts.to_zoned(zone.clone());
    let period = period.trim().to_ascii_lowercase();
    if period == "tick" {
        return Ok(true);
    }
    let end_day = match period.as_str() {
        "1d" | "day" => Some(local.date()),
        "1w" | "week" => Some(
            local
                .date()
                .checked_add((7 - local.weekday().to_monday_one_offset()).days())
                .map_err(invalid)?,
        ),
        "1mo" | "month" => Some(local.date().last_of_month()),
        _ => None,
    };
    let close = if let Some(end_day) = end_day {
        if crate::manager::supported_market(market) {
            last_session_close(calendar, market, local.date(), end_day, &zone, sessions)?
        } else {
            // No authoritative session calendar for this market: never guess
            // a 16:00 close. Wait for the period boundary (or a subsequent bar).
            Some(at_minute(
                end_day.checked_add(1.days()).map_err(invalid)?,
                0,
                &zone,
            )?)
        }
    } else {
        let minutes = match period.as_str() {
            "1m" => 1,
            "3m" => 3,
            "5m" => 5,
            "10m" => 10,
            "15m" => 15,
            "30m" => 30,
            "60m" | "1h" => 60,
            "120m" | "2h" => 120,
            "180m" | "3h" => 180,
            "240m" | "4h" => 240,
            _ => return Err(invalid(format!("unsupported candle period {period}"))),
        };
        let nominal = open_ts
            .checked_add(jiff::SignedDuration::from_secs(minutes * 60))
            .map_err(invalid)?;
        let schedule = day_schedule(calendar, market, local.date(), &zone)?;
        let minute = i32::from(local.hour()) * 60 + i32::from(local.minute());
        let end = schedule.and_then(|schedule| {
            schedule.sessions.into_iter().find(|s| {
                s.start_minute <= minute && minute < s.end_minute && requested(&s.kind, sessions)
            })
        });
        Some(match end {
            Some(end) => nominal.min(at_minute(local.date(), end.end_minute, &zone)?),
            None => nominal,
        })
    };
    Ok(close.is_some_and(|close| now_ts >= close))
}

fn last_session_close(
    calendar: Option<&CalendarManager>,
    market: &str,
    start: Date,
    mut date: Date,
    zone: &TimeZone,
    sessions: &[&str],
) -> Result<Option<Timestamp>, CalendarManagerError> {
    while date >= start {
        if let Some(schedule) = day_schedule(calendar, market, date, zone)?
            && schedule.status != "closed"
            && let Some(end) = schedule
                .sessions
                .iter()
                .filter(|s| requested(&s.kind, sessions))
                .map(|s| s.end_minute)
                .max()
        {
            return at_minute(date, end, zone).map(Some);
        }
        date = date.checked_sub(1.days()).map_err(invalid)?;
    }
    Ok(None)
}

fn day_schedule(
    calendar: Option<&CalendarManager>,
    market: &str,
    date: Date,
    zone: &TimeZone,
) -> Result<Option<TradingDaySchedule>, CalendarManagerError> {
    if !crate::manager::supported_market(market) {
        return Ok(None);
    }
    let noon = date
        .at(12, 0, 0, 0)
        .to_zoned(zone.clone())
        .map_err(invalid)?;
    let at = OffsetDateTime::from_unix_timestamp_nanos(noon.timestamp().as_nanosecond())
        .map_err(invalid)?;
    let wire = WireTimestamp::from_offset_datetime(at);
    match calendar {
        Some(calendar) => calendar.schedule(market, wire),
        None => {
            let start = crate::manager_policy::market_day_start(market, wire)?;
            Ok(Some(crate::manager_policy::builtin_schedule(market, start)))
        }
    }
}

fn at_minute(date: Date, minute: i32, zone: &TimeZone) -> Result<Timestamp, CalendarManagerError> {
    date.at(0, 0, 0, 0)
        .checked_add(i64::from(minute).minutes())
        .map_err(invalid)?
        .to_zoned(zone.clone())
        .map(|z| z.timestamp())
        .map_err(invalid)
}

fn requested(kind: &str, sessions: &[&str]) -> bool {
    sessions.is_empty()
        || sessions.contains(&"all")
        || sessions.contains(&kind)
        || (sessions.contains(&"extended") && matches!(kind, "pre" | "after"))
}
fn invalid(error: impl std::fmt::Display) -> CalendarManagerError {
    CalendarManagerError::InvalidSettings(error.to_string())
}
