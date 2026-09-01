use crate::{
    CalendarManager, CalendarManagerError, manager_calendar::market_timezone,
    manager_policy::market_day_start,
};
use jftrade_kernel::WireTimestamp;
use jiff::{Timestamp, tz::TimeZone};

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
        let lookup = market_day_start(&market, at)?;
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
