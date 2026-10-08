//! Calendar settings projection used by the production composition root.

use std::path::{Path, PathBuf};

use jftrade_calendar::{
    CalendarManagerSettings, CalendarManualOverride, CalendarSessionOverride, CalendarSourcePolicy,
};
use jftrade_settings::ExchangeCalendarSettings;

use crate::product::ProductError;

pub(crate) fn configured_calendar_manager(
    config: &crate::product::ProductConfig,
    settings: ExchangeCalendarSettings,
    store: std::sync::Arc<dyn jftrade_settings::ExchangeCalendarSettingsStorePort>,
) -> Result<std::sync::Arc<jftrade_calendar::CalendarManager>, ProductError> {
    // Register the official adapters used by the enabled source policies.
    let manager = production_calendar_manager(
        calendar_source_registry()?,
        Some(std::sync::Arc::new(
            jftrade_calendar::CalendarSnapshotStore::new(exchange_calendar_snapshot_root(
                config.settings_path(),
            )),
        )),
        settings,
        store,
        config.live_hub.clone(),
    )
    .map_err(ProductError::Calendar)?;
    Ok(std::sync::Arc::new(manager))
}

pub(crate) fn production_calendar_manager(
    registry: jftrade_calendar::CalendarSourceRegistry,
    persistence: Option<std::sync::Arc<dyn jftrade_calendar::CalendarPersistencePort>>,
    settings: ExchangeCalendarSettings,
    store: std::sync::Arc<dyn jftrade_settings::ExchangeCalendarSettingsStorePort>,
    hub: Option<std::sync::Arc<jftrade_api::LiveHub>>,
) -> Result<jftrade_calendar::CalendarManager, jftrade_calendar::CalendarManagerError> {
    let sink = std::sync::Arc::new(move |alert| {
        record_calendar_alert(store.as_ref(), hub.as_deref(), alert);
    });
    jftrade_calendar::CalendarManager::with_clock_and_alert_sink(
        registry,
        persistence,
        calendar_manager_settings(settings),
        std::sync::Arc::new(time::OffsetDateTime::now_utc),
        Some(sink),
    )
}

fn record_calendar_alert(
    store: &dyn jftrade_settings::ExchangeCalendarSettingsStorePort,
    hub: Option<&jftrade_api::LiveHub>,
    alert: jftrade_calendar::CalendarSourceAlert,
) {
    let Ok(settings) = store.load_exchange_calendars() else {
        return;
    };
    if !settings.unwrap_or_default().error_notifications_enabled {
        return;
    }
    if let Some(hub) = hub
        && let Some(note) = calendar_notification(alert)
    {
        hub.publish(serde_json::json!({
            "eventId": format!("calendar-alert|{}", note["id"].as_str().unwrap_or_default()),
            "type": "system.notification",
            "source": "notification",
            "entityId": note["id"],
            "serverTime": note["at"],
            "payload": note,
        }));
    }
}

fn calendar_notification(
    alert: jftrade_calendar::CalendarSourceAlert,
) -> Option<serde_json::Value> {
    let title = alert.title.trim();
    if title.is_empty() {
        return None;
    }
    let level = match alert.level.trim().to_lowercase().as_str() {
        "success" => "success",
        "error" => "error",
        "warn" => "warn",
        _ => "info",
    };
    let message = if alert.message.trim().is_empty() {
        format!(
            "{} 市场日历源 {} 状态发生变化。",
            alert.market.trim(),
            alert.source_id.trim()
        )
    } else {
        alert.message.trim().to_owned()
    };
    let now = time::OffsetDateTime::now_utc();
    let sequence =
        CALENDAR_NOTIFICATION_SEQUENCE.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
    Some(serde_json::json!({
        "id": format!("calendar-{}-{sequence}", now.unix_timestamp_nanos()),
        "brokerId": "",
        "source": "exchange-calendars",
        "category": "market.calendar.source",
        "level": level,
        "title": title,
        "message": message,
        "at": jftrade_kernel::WireTimestamp::from_offset_datetime(now).to_string(),
    }))
}

#[cfg(test)]
#[path = "product_calendar_notification_tests.rs"]
mod notification_tests;

const EXCHANGE_CALENDAR_DIR_ENV: &str = "JFTRADE_EXCHANGE_CALENDAR_DIR";
static CALENDAR_NOTIFICATION_SEQUENCE: std::sync::atomic::AtomicU64 =
    std::sync::atomic::AtomicU64::new(0);

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

/// Build the production calendar source registry.
///
/// Mirrors Go's `exchangecalendar.DefaultRegistry(nil)`: the four official
/// providers share one HTTP client with the default request timeout. If the
/// client cannot be built the composition root fails closed rather than
/// starting with an empty registry that would report sources as enabled that
/// can never be fetched.
pub(crate) fn calendar_source_registry()
-> Result<jftrade_calendar::CalendarSourceRegistry, ProductError> {
    let calendar_error = |error: jftrade_calendar::CalendarSourceError| {
        ProductError::Calendar(jftrade_calendar::CalendarManagerError::InvalidSettings(
            error.to_string(),
        ))
    };
    let client = std::sync::Arc::new(
        jftrade_integration_calendar::ReqwestCalendarClient::with_default_timeout()
            .map_err(calendar_error)?,
    );
    jftrade_integration_calendar::default_registry(client).map_err(calendar_error)
}
