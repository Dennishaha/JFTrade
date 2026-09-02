//! Calendar settings projection used by the production composition root.

use std::path::{Path, PathBuf};

use jftrade_calendar::{
    CalendarManagerSettings, CalendarManualOverride, CalendarSessionOverride, CalendarSourcePolicy,
};
use jftrade_settings::ExchangeCalendarSettings;

const EXCHANGE_CALENDAR_DIR_ENV: &str = "JFTRADE_EXCHANGE_CALENDAR_DIR";

pub(crate) fn exchange_calendar_snapshot_root(settings_path: &Path) -> PathBuf {
    std::env::var_os(EXCHANGE_CALENDAR_DIR_ENV)
        .filter(|value| !value.is_empty())
        .map(PathBuf::from)
        .unwrap_or_else(|| {
            settings_path
                .parent()
                .filter(|parent| !parent.as_os_str().is_empty())
                .map_or_else(
                    || PathBuf::from("exchange-calendars"),
                    |parent| parent.join("exchange-calendars"),
                )
        })
}

pub(crate) fn calendar_manager_settings(
    input: ExchangeCalendarSettings,
) -> CalendarManagerSettings {
    CalendarManagerSettings {
        auto_refresh_enabled: input.auto_refresh_enabled,
        error_notifications_enabled: input.error_notifications_enabled,
        refresh_interval_hours: input.refresh_interval_hours,
        warmup_markets: input.warmup_markets,
        source_policies: input
            .source_policies
            .into_iter()
            .map(|policy| CalendarSourcePolicy {
                market: policy.market,
                preferred_source_ids: policy.preferred_source_ids,
                enabled_source_ids: policy.enabled_source_ids,
                fallback_to_builtin: policy.fallback_to_builtin,
                require_official: policy.require_official,
                stale_after_hours: policy.stale_after_hours,
            })
            .collect(),
        manual_overrides: input
            .manual_overrides
            .into_iter()
            .map(|override_| CalendarManualOverride {
                market: override_.market,
                date: override_.date,
                status: override_.status,
                sessions: override_
                    .sessions
                    .into_iter()
                    .map(|session| CalendarSessionOverride {
                        kind: session.kind,
                        start_minute: session.start_minute,
                        end_minute: session.end_minute,
                    })
                    .collect(),
                reason: override_.reason,
                observed: override_.observed,
            })
            .collect(),
    }
}
