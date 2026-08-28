#![forbid(unsafe_code)]

mod manager;
mod manager_policy;
mod manager_probe;
mod manager_projection;
mod manager_registry;
mod manager_session;
mod manager_types;
mod snapshot;
mod sources;
mod status;

pub use manager::{CalendarManager, CalendarManagerError};
pub use manager_registry::CalendarSourceRegistry;
pub use manager_types::{
    CalendarCancellationToken, CalendarManagerSettings, CalendarManualOverride,
    CalendarPersistencePort, CalendarProbeItem, CalendarProbeResult, CalendarRefreshResult,
    CalendarSessionOverride, CalendarSourceError, CalendarSourcePolicy, CalendarSourcePort,
    CalendarSourceRuntimeStatus, ManagerLifecycleState,
};

pub use snapshot::{
    CalendarSessionWindow, CalendarSnapshot, CalendarSnapshotLoadError,
    CalendarSnapshotLoadErrorKind, CalendarSnapshotLoadResult, CalendarSnapshotStore,
    CalendarSnapshotStoreError, TradingDaySchedule,
};
pub use sources::{
    BUILTIN_SOURCE_ID, CalendarSourceDescriptor, CalendarSourceProjection, CalendarSourceStatus,
    CalendarSourcesSnapshot, MANUAL_OVERRIDE_SOURCE_ID, default_source_descriptors,
    normalize_source_ids, project_default_sources, project_sources, source_availability_note,
    source_enabled,
};
pub use status::{
    CalendarMarketStatus, CalendarSampleSchedule, CalendarSampleSession, CalendarSnapshotSummary,
    CalendarStatusSnapshot,
};

use serde::{Deserialize, Serialize};
use thiserror::Error;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SessionWindow {
    pub open_minute: u16,
    pub close_minute: u16,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CalendarPolicy {
    pub market: String,
    pub sources: Vec<String>,
    pub sessions: Vec<SessionWindow>,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum CalendarError {
    #[error("market is required")]
    MissingMarket,
    #[error("at least one source is required")]
    MissingSource,
    #[error("session window must be within one day and open before close")]
    InvalidSession,
    #[error("session windows must not overlap")]
    OverlappingSessions,
}

pub fn normalize_policy(
    market: &str,
    sources: impl IntoIterator<Item = String>,
    mut sessions: Vec<SessionWindow>,
) -> Result<CalendarPolicy, CalendarError> {
    let market = market.trim().to_uppercase();
    if market.is_empty() {
        return Err(CalendarError::MissingMarket);
    }
    let mut normalized_sources = Vec::new();
    for source in sources {
        let source = source.trim().to_lowercase();
        if !source.is_empty() && !normalized_sources.contains(&source) {
            normalized_sources.push(source);
        }
    }
    if normalized_sources.is_empty() {
        return Err(CalendarError::MissingSource);
    }
    sessions.sort_by_key(|window| window.open_minute);
    for (index, window) in sessions.iter().enumerate() {
        if window.open_minute >= window.close_minute || window.close_minute > 24 * 60 {
            return Err(CalendarError::InvalidSession);
        }
        if index > 0 && sessions[index - 1].close_minute > window.open_minute {
            return Err(CalendarError::OverlappingSessions);
        }
    }
    Ok(CalendarPolicy {
        market,
        sources: normalized_sources,
        sessions,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn policy_preserves_source_priority_and_orders_sessions() {
        let policy = normalize_policy(
            " hk ",
            [" Manual ".into(), "futu".into(), "manual".into()],
            vec![
                SessionWindow {
                    open_minute: 780,
                    close_minute: 960,
                },
                SessionWindow {
                    open_minute: 570,
                    close_minute: 720,
                },
            ],
        )
        .expect("valid policy");
        assert_eq!(policy.market, "HK");
        assert_eq!(policy.sources, ["manual", "futu"]);
        assert_eq!(policy.sessions[0].open_minute, 570);
    }

    #[test]
    fn overlapping_or_cross_day_sessions_are_rejected() {
        assert_eq!(
            normalize_policy(
                "US",
                ["manual".into()],
                vec![
                    SessionWindow {
                        open_minute: 500,
                        close_minute: 700,
                    },
                    SessionWindow {
                        open_minute: 650,
                        close_minute: 800,
                    },
                ],
            ),
            Err(CalendarError::OverlappingSessions)
        );
    }
}
