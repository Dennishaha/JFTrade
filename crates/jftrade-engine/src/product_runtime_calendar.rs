//! Bridge from the production calendar owner to the Futu quote adapter.

use std::sync::{Arc, RwLock};

use jftrade_calendar::CalendarManager;
use jftrade_integration_futu::{QuoteSessionContext, QuoteSessionResolver, QuoteSessionWindow};
use jftrade_kernel::WireTimestamp;
use jftrade_store_sqlite::{CalendarDaySchedule, CalendarDaySession, CalendarScheduleResolver};
use jiff::civil::Date;
use time::OffsetDateTime;

#[derive(Default)]
pub(crate) struct RuntimeCalendarSessionResolver {
    manager: RwLock<Option<Arc<CalendarManager>>>,
}

impl std::fmt::Debug for RuntimeCalendarSessionResolver {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("RuntimeCalendarSessionResolver")
            .field(
                "bound",
                &self
                    .manager
                    .read()
                    .ok()
                    .is_some_and(|manager| manager.is_some()),
            )
            .finish()
    }
}

impl RuntimeCalendarSessionResolver {
    pub(crate) fn new() -> Arc<Self> {
        Arc::new(Self::default())
    }

    pub(crate) fn bind(&self, manager: Arc<CalendarManager>) {
        if let Ok(mut slot) = self.manager.write() {
            *slot = Some(manager);
        }
    }
}

impl QuoteSessionResolver for RuntimeCalendarSessionResolver {
    fn resolve_quote_session(
        &self,
        instrument_id: &str,
        observed_at_ms: i64,
    ) -> Option<QuoteSessionContext> {
        let market = instrument_id.split_once('.')?.0;
        let manager = self.manager.read().ok()?.as_ref().cloned()?;
        let nanos = i128::from(observed_at_ms).checked_mul(1_000_000)?;
        let at = OffsetDateTime::from_unix_timestamp_nanos(nanos).ok()?;
        let context = manager
            .session_context(market, WireTimestamp::from_offset_datetime(at))
            .ok()??;
        Some(QuoteSessionContext {
            session: context.session,
            trading_date: context.trading_date,
            timezone: context.timezone,
            sessions: context
                .sessions
                .into_iter()
                .map(|window| QuoteSessionWindow {
                    kind: window.kind,
                    start_minute: window.start_minute,
                    end_minute: window.end_minute,
                })
                .collect(),
        })
    }
}

/// Adapter used by backtest aggregation so persisted bars and live quote
/// projection consult the same CalendarManager (including manual overrides).
pub(crate) struct RuntimeBacktestCalendarResolver {
    manager: Arc<CalendarManager>,
}

impl RuntimeBacktestCalendarResolver {
    pub(crate) fn new(manager: Arc<CalendarManager>) -> Arc<Self> {
        Arc::new(Self { manager })
    }
}

impl CalendarScheduleResolver for RuntimeBacktestCalendarResolver {
    fn schedule(&self, market: &str, date: Date) -> Option<CalendarDaySchedule> {
        // Noon UTC stays on the requested local date for every supported
        // exchange, unlike UTC midnight which is the previous US date.
        let at = format!("{date}T12:00:00Z").parse::<WireTimestamp>().ok()?;
        let schedule = self.manager.schedule(market, at).ok()??;
        Some(CalendarDaySchedule {
            status: schedule.status,
            sessions: schedule
                .sessions
                .into_iter()
                .map(|session| CalendarDaySession {
                    kind: session.kind,
                    start_minute: session.start_minute,
                    end_minute: session.end_minute,
                })
                .collect(),
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_calendar::{CalendarManagerSettings, CalendarSourceRegistry};

    #[test]
    fn backtest_resolver_projects_calendar_manager_holiday_schedule() {
        let manager = Arc::new(
            CalendarManager::new(
                CalendarSourceRegistry::default(),
                None,
                CalendarManagerSettings::default(),
            )
            .expect("calendar manager"),
        );
        let resolver = RuntimeBacktestCalendarResolver::new(manager);
        let schedule = resolver
            .schedule("US", "2026-06-19".parse().expect("calendar date"))
            .expect("US holiday schedule");
        assert_eq!(schedule.status, "closed");
        assert!(schedule.sessions.is_empty());
    }
}
