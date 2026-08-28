use jftrade_kernel::WireTimestamp;
use jiff::{Timestamp, tz::TimeZone};
use time::{Date, Month, Time};

use crate::{CalendarManager, CalendarManagerError};

impl CalendarManager {
    /// Classifies an instant against the same authoritative schedule used by
    /// calendar status and probe projections. IANA timezone conversion lives
    /// here so market-data adapters do not grow their own DST/calendar rules.
    pub fn classify_session(
        &self,
        market: &str,
        at: WireTimestamp,
    ) -> Result<Option<String>, CalendarManagerError> {
        let market = market.trim().to_ascii_uppercase();
        let timezone = market_timezone(&market)
            .ok_or_else(|| CalendarManagerError::UnsupportedMarket(market.clone()))?;
        let timestamp = at
            .to_string()
            .parse::<Timestamp>()
            .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
        let timezone = TimeZone::get(timezone)
            .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
        let local = timestamp.to_zoned(timezone);
        let date = local.date();
        let local_day = Date::from_calendar_date(
            i32::from(date.year()),
            Month::try_from(
                u8::try_from(date.month())
                    .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?,
            )
            .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?,
            u8::try_from(date.day())
                .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?,
        )
        .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
        let lookup =
            WireTimestamp::from_offset_datetime(local_day.with_time(Time::MIDNIGHT).assume_utc());
        let Some(schedule) = self.schedule(&market, lookup)? else {
            return Ok(None);
        };
        if schedule.status == "closed" || schedule.sessions.is_empty() {
            return Ok(None);
        }
        let minute = i32::from(local.hour()) * 60 + i32::from(local.minute());
        Ok(schedule
            .sessions
            .iter()
            .find(|window| (window.start_minute..window.end_minute).contains(&minute))
            .map(|window| window.kind.clone()))
    }
}

fn market_timezone(market: &str) -> Option<&'static str> {
    match market {
        "US" => Some("America/New_York"),
        "HK" => Some("Asia/Hong_Kong"),
        "CN" | "SH" | "SZ" => Some("Asia/Shanghai"),
        _ => None,
    }
}
