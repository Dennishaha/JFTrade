use crate::{
    CalendarManager, CalendarManagerError, CalendarSessionWindow,
    manager_calendar::market_timezone, manager_policy::market_day_start,
};
use jftrade_kernel::WireTimestamp;
use jiff::{Timestamp, tz::TimeZone};

/// Calendar-owned context required by quote projection.  The full schedule is
/// retained alongside the selected session so extended/split windows and
/// exchange-specific exceptions (DST, early closes, lunch breaks) are not
/// reimplemented by provider adapters.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct CalendarSessionContext {
    pub session: String,
    pub trading_date: String,
    pub timezone: String,
    pub sessions: Vec<CalendarSessionWindow>,
}

impl CalendarManager {
    /// Classifies an instant against the same authoritative schedule used by
    /// calendar status and probe projections. IANA timezone conversion lives
    /// here so market-data adapters do not grow their own DST/calendar rules.
    pub fn classify_session(
        &self,
        market: &str,
        at: WireTimestamp,
    ) -> Result<Option<String>, CalendarManagerError> {
        Ok(self
            .session_context(market, at)?
            .and_then(|context| (context.session != "closed").then_some(context.session)))
    }

    /// Resolve an instant against the manager's authoritative schedule and
    /// retain the selected trading date/windows for downstream projections.
    pub fn session_context(
        &self,
        market: &str,
        at: WireTimestamp,
    ) -> Result<Option<CalendarSessionContext>, CalendarManagerError> {
        let market = market.trim().to_ascii_uppercase();
        let timezone_name = market_timezone(&market)
            .ok_or_else(|| CalendarManagerError::UnsupportedMarket(market.clone()))?;
        let timestamp = at
            .to_string()
            .parse::<Timestamp>()
            .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
        let timezone = TimeZone::get(timezone_name)
            .map_err(|error| CalendarManagerError::InvalidSettings(error.to_string()))?;
        let local = timestamp.to_zoned(timezone);
        let minute = i32::from(local.hour()) * 60 + i32::from(local.minute());
        let lookup = market_day_start(&market, at)?;
        let schedule = self.schedule(&market, lookup)?;

        if let Some(schedule) = schedule.as_ref()
            && schedule.status != "closed"
            && !schedule.sessions.is_empty()
            && let Some(window) = schedule
                .sessions
                .iter()
                .find(|window| (window.start_minute..window.end_minute).contains(&minute))
        {
            return Ok(Some(context_from_schedule(
                schedule,
                window.kind.clone(),
                timezone_name,
            )));
        }

        // US's 20:00-04:00 carry belongs to the following trading date.  A
        // Sunday evening is valid when Monday is open; a Friday/holiday eve
        // must not fabricate an overnight session when the next date is
        // closed. For ordinary weekday carry, require today's schedule to be
        // open as well so a holiday evening remains closed.
        let current_schedule_open = schedule
            .as_ref()
            .is_some_and(|value| value.status != "closed" && !value.sessions.is_empty());
        if market == "US"
            && minute >= 20 * 60
            && (local.weekday() == jiff::civil::Weekday::Sunday || current_schedule_open)
        {
            let next_at = at
                .into_inner()
                .checked_add(time::Duration::days(1))
                .map(WireTimestamp::from_offset_datetime)
                .ok_or_else(|| {
                    CalendarManagerError::InvalidSettings(
                        "calendar next-day timestamp overflow".to_owned(),
                    )
                })?;
            if let Some(next_schedule) = self.schedule(&market, next_at)?
                && next_schedule.status != "closed"
                && !next_schedule.sessions.is_empty()
            {
                let overnight = next_schedule
                    .sessions
                    .iter()
                    .find(|window| window.kind == "overnight")
                    .map(|window| window.kind.clone())
                    .unwrap_or_else(|| "overnight".to_owned());
                return Ok(Some(context_from_schedule(
                    &next_schedule,
                    overnight,
                    timezone_name,
                )));
            }
        }

        let (trading_date, sessions) = schedule
            .as_ref()
            .map(|schedule| {
                (
                    schedule.date.into_inner().date().to_string(),
                    schedule.sessions.clone(),
                )
            })
            .unwrap_or_else(|| (local.date().to_string(), Vec::new()));
        Ok(Some(CalendarSessionContext {
            session: "closed".to_owned(),
            trading_date,
            timezone: timezone_name.to_owned(),
            sessions,
        }))
    }
}

fn context_from_schedule(
    schedule: &crate::TradingDaySchedule,
    session: String,
    timezone: &'static str,
) -> CalendarSessionContext {
    CalendarSessionContext {
        session,
        trading_date: schedule.date.into_inner().date().to_string(),
        timezone: timezone.to_owned(),
        sessions: schedule.sessions.clone(),
    }
}

#[cfg(test)]
mod tests {
    use std::str::FromStr;

    use super::*;
    use crate::{
        CalendarManagerSettings, CalendarManualOverride, CalendarSessionOverride,
        CalendarSourceRegistry,
    };

    fn at(value: &str) -> WireTimestamp {
        WireTimestamp::from_str(value).expect("valid timestamp fixture")
    }

    fn manager() -> CalendarManager {
        CalendarManager::new(
            CalendarSourceRegistry::default(),
            None,
            CalendarManagerSettings::default(),
        )
        .expect("calendar manager")
    }

    #[test]
    fn session_context_distinguishes_hk_lunch_and_weekend() {
        let manager = manager();
        let regular = manager
            .session_context("HK", at("2026-06-22T10:00:00+08:00"))
            .expect("regular context")
            .expect("supported market");
        assert_eq!(regular.session, "regular");
        assert_eq!(regular.trading_date, "2026-06-22");

        let lunch = manager
            .session_context("HK", at("2026-06-22T12:30:00+08:00"))
            .expect("lunch context")
            .expect("supported market");
        assert_eq!(lunch.session, "closed");
        assert_eq!(lunch.trading_date, "2026-06-22");

        let weekend = manager
            .session_context("HK", at("2026-06-20T10:00:00+08:00"))
            .expect("weekend context")
            .expect("supported market");
        assert_eq!(weekend.session, "closed");
    }

    #[test]
    fn session_context_handles_us_holidays_early_close_and_sunday_overnight() {
        let manager = manager();
        for timestamp in [
            "2026-06-19T10:00:00-04:00", // Juneteenth
            "2026-04-03T10:00:00-04:00", // Good Friday
            "2026-07-03T10:00:00-04:00", // observed Independence Day
        ] {
            let context = manager
                .session_context("US", at(timestamp))
                .expect("holiday context")
                .expect("supported market");
            assert_eq!(context.session, "closed", "timestamp={timestamp}");
        }

        let early = manager
            .session_context("US", at("2026-07-02T13:30:00-04:00"))
            .expect("early-close context")
            .expect("supported market");
        assert_eq!(early.session, "after");
        assert_eq!(early.trading_date, "2026-07-02");
        assert_eq!(
            early
                .sessions
                .iter()
                .find(|window| window.kind == "regular")
                .map(|window| (window.start_minute, window.end_minute)),
            Some((570, 780))
        );
        assert_eq!(
            early
                .sessions
                .iter()
                .find(|window| window.kind == "after")
                .map(|window| (window.start_minute, window.end_minute)),
            Some((780, 1080))
        );

        let sunday = manager
            .session_context("US", at("2026-06-21T20:00:00-04:00"))
            .expect("sunday context")
            .expect("supported market");
        assert_eq!(sunday.session, "overnight");
        assert_eq!(sunday.trading_date, "2026-06-22");

        // Parity: pkg/market/market_test.go:11 TestClassifySessionForUS
        // Friday after 20:00 is closed, not overnight carry
        let friday_closed = manager
            .session_context("US", at("2026-06-12T20:00:00-04:00"))
            .expect("friday context")
            .expect("supported market");
        assert_eq!(friday_closed.session, "closed");

        // Pre-market starts exactly at 04:00
        let pre_market = manager
            .session_context("US", at("2026-06-12T04:00:00-04:00"))
            .expect("premarket context")
            .expect("supported market");
        assert_eq!(pre_market.session, "pre");

        // Regular trading starts at 09:30
        let regular = manager
            .session_context("US", at("2026-06-12T09:30:00-04:00"))
            .expect("regular context")
            .expect("supported market");
        assert_eq!(regular.session, "regular");

        // After-hours starts at 16:00
        let after = manager
            .session_context("US", at("2026-06-12T16:00:00-04:00"))
            .expect("after context")
            .expect("supported market");
        assert_eq!(after.session, "after");
    }

    #[test]
    fn manual_override_is_authoritative_for_closed_and_custom_windows() {
        let mut settings = CalendarManagerSettings::default();
        settings.manual_overrides.push(CalendarManualOverride {
            market: "US".to_owned(),
            date: "2026-07-02".to_owned(),
            status: "closed".to_owned(),
            reason: "manual maintenance".to_owned(),
            sessions: Vec::new(),
            observed: false,
        });
        let manager = CalendarManager::new(CalendarSourceRegistry::default(), None, settings)
            .expect("calendar manager");
        let closed = manager
            .session_context("US", at("2026-07-02T10:00:00-04:00"))
            .expect("manual context")
            .expect("supported market");
        assert_eq!(closed.session, "closed");

        let mut settings = CalendarManagerSettings::default();
        settings.manual_overrides.push(CalendarManualOverride {
            market: "HK".to_owned(),
            date: "2026-06-22".to_owned(),
            status: "early_close".to_owned(),
            reason: "manual event".to_owned(),
            sessions: vec![CalendarSessionOverride {
                kind: "regular".to_owned(),
                start_minute: 570,
                end_minute: 720,
            }],
            observed: false,
        });
        let manager = CalendarManager::new(CalendarSourceRegistry::default(), None, settings)
            .expect("calendar manager");
        let custom = manager
            .session_context("HK", at("2026-06-22T13:00:00+08:00"))
            .expect("manual context")
            .expect("supported market");
        assert_eq!(custom.session, "closed");
        assert_eq!(custom.sessions.len(), 1);
    }
}
