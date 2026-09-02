use std::sync::Arc;

use serde::{Deserialize, Serialize};

use crate::SettingsStoreError;

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct ExchangeCalendarSessionWindow {
    pub kind: String,
    pub start_minute: i32,
    pub end_minute: i32,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct ExchangeCalendarManualOverride {
    pub market: String,
    pub date: String,
    pub status: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub sessions: Vec<ExchangeCalendarSessionWindow>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub reason: String,
    #[serde(skip_serializing_if = "is_false")]
    pub observed: bool,
}

#[derive(Clone, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct ExchangeCalendarSourcePolicy {
    pub market: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub preferred_source_ids: Vec<String>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub enabled_source_ids: Vec<String>,
    pub fallback_to_builtin: bool,
    #[serde(skip_serializing_if = "is_false")]
    pub require_official: bool,
    #[serde(skip_serializing_if = "is_zero")]
    pub stale_after_hours: i32,
}

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct ExchangeCalendarSettings {
    pub auto_refresh_enabled: bool,
    pub error_notifications_enabled: bool,
    pub refresh_interval_hours: i32,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub warmup_markets: Vec<String>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub source_policies: Vec<ExchangeCalendarSourcePolicy>,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub manual_overrides: Vec<ExchangeCalendarManualOverride>,
}

impl Default for ExchangeCalendarSettings {
    fn default() -> Self {
        Self {
            auto_refresh_enabled: true,
            error_notifications_enabled: true,
            refresh_interval_hours: 24,
            warmup_markets: vec!["US".into(), "HK".into(), "CN".into()],
            source_policies: vec![
                ExchangeCalendarSourcePolicy {
                    market: "US".into(),
                    preferred_source_ids: vec!["nyse_official".into()],
                    enabled_source_ids: vec!["nyse_official".into(), "builtin_rules".into()],
                    fallback_to_builtin: true,
                    require_official: false,
                    stale_after_hours: 72,
                },
                ExchangeCalendarSourcePolicy {
                    market: "HK".into(),
                    preferred_source_ids: vec!["hk_gov_1823_ical".into()],
                    enabled_source_ids: vec!["hk_gov_1823_ical".into(), "builtin_rules".into()],
                    fallback_to_builtin: true,
                    require_official: false,
                    stale_after_hours: 168,
                },
                ExchangeCalendarSourcePolicy {
                    market: "CN".into(),
                    preferred_source_ids: Vec::new(),
                    enabled_source_ids: vec!["builtin_rules".into()],
                    fallback_to_builtin: true,
                    require_official: false,
                    stale_after_hours: 168,
                },
            ],
            manual_overrides: Vec::new(),
        }
    }
}

pub trait ExchangeCalendarSettingsStorePort: Send + Sync {
    fn load_exchange_calendars(
        &self,
    ) -> Result<Option<ExchangeCalendarSettings>, SettingsStoreError>;

    fn save_exchange_calendars(
        &self,
        settings: &ExchangeCalendarSettings,
    ) -> Result<ExchangeCalendarSettings, SettingsStoreError>;
}

#[derive(Clone)]
pub struct ExchangeCalendarSettingsService {
    store: Arc<dyn ExchangeCalendarSettingsStorePort>,
}

impl ExchangeCalendarSettingsService {
    pub fn new(store: Arc<dyn ExchangeCalendarSettingsStorePort>) -> Self {
        Self { store }
    }

    pub fn settings(&self) -> Result<ExchangeCalendarSettings, SettingsStoreError> {
        Ok(self
            .store
            .load_exchange_calendars()?
            .map(normalize_exchange_calendar_settings)
            .unwrap_or_default())
    }

    pub fn save(
        &self,
        settings: ExchangeCalendarSettings,
    ) -> Result<ExchangeCalendarSettings, SettingsStoreError> {
        let normalized = normalize_exchange_calendar_settings(settings);
        self.store.save_exchange_calendars(&normalized)
    }
}

pub fn normalize_exchange_calendar_settings(
    mut settings: ExchangeCalendarSettings,
) -> ExchangeCalendarSettings {
    let defaults = ExchangeCalendarSettings::default();
    if settings.refresh_interval_hours <= 0 {
        settings.refresh_interval_hours = defaults.refresh_interval_hours;
    }
    settings.refresh_interval_hours = settings.refresh_interval_hours.clamp(1, 24 * 30);
    settings.warmup_markets = if settings.warmup_markets.is_empty() {
        defaults.warmup_markets
    } else {
        normalize_values(settings.warmup_markets, |value| {
            value.trim().to_ascii_uppercase()
        })
    };
    settings.source_policies = if settings.source_policies.is_empty() {
        defaults.source_policies
    } else {
        settings
            .source_policies
            .into_iter()
            .filter_map(|mut policy| {
                policy.market = policy.market.trim().to_ascii_uppercase();
                if policy.market.is_empty() {
                    return None;
                }
                policy.preferred_source_ids =
                    normalize_values(policy.preferred_source_ids, normalize_source_id);
                policy.enabled_source_ids =
                    normalize_values(policy.enabled_source_ids, normalize_source_id);
                policy.stale_after_hours = policy.stale_after_hours.max(0);
                Some(policy)
            })
            .collect()
    };
    settings.manual_overrides = settings
        .manual_overrides
        .into_iter()
        .filter_map(|mut calendar_override| {
            calendar_override.market = calendar_override.market.trim().to_ascii_uppercase();
            calendar_override.date = calendar_override.date.trim().to_owned();
            calendar_override.status = calendar_override.status.trim().to_ascii_lowercase();
            calendar_override.reason = calendar_override.reason.trim().to_owned();
            calendar_override.sessions = calendar_override
                .sessions
                .into_iter()
                .filter_map(|mut session| {
                    session.kind = session.kind.trim().to_ascii_lowercase();
                    (!session.kind.is_empty() && session.end_minute > session.start_minute)
                        .then_some(session)
                })
                .collect();
            (!calendar_override.market.is_empty()
                && !calendar_override.date.is_empty()
                && !calendar_override.status.is_empty())
            .then_some(calendar_override)
        })
        .collect();
    settings
}

fn normalize_source_id(value: &str) -> String {
    match value.trim() {
        "hkex_official" => "hk_gov_1823_ical".to_owned(),
        value => value.to_owned(),
    }
}

fn normalize_values(values: Vec<String>, normalize: impl Fn(&str) -> String) -> Vec<String> {
    let mut normalized = Vec::new();
    for value in values {
        let value = normalize(&value);
        if !value.is_empty() && !normalized.contains(&value) {
            normalized.push(value);
        }
    }
    normalized
}

const fn is_zero(value: &i32) -> bool {
    *value == 0
}

const fn is_false(value: &bool) -> bool {
    !*value
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn legacy_sources_and_invalid_overrides_match_go_normalization() {
        let settings = normalize_exchange_calendar_settings(ExchangeCalendarSettings {
            auto_refresh_enabled: false,
            error_notifications_enabled: false,
            refresh_interval_hours: 1000,
            warmup_markets: vec![" us ".into(), "US".into(), " hk ".into()],
            source_policies: vec![ExchangeCalendarSourcePolicy {
                market: " hk ".into(),
                preferred_source_ids: vec![" hkex_official ".into()],
                enabled_source_ids: vec!["builtin_rules".into(), "builtin_rules".into()],
                fallback_to_builtin: true,
                require_official: false,
                stale_after_hours: -1,
            }],
            manual_overrides: vec![ExchangeCalendarManualOverride {
                market: " us ".into(),
                date: " 2026-01-02 ".into(),
                status: " CLOSED ".into(),
                sessions: vec![ExchangeCalendarSessionWindow {
                    kind: " regular ".into(),
                    start_minute: 100,
                    end_minute: 100,
                }],
                reason: " holiday ".into(),
                observed: true,
            }],
        });
        assert_eq!(settings.refresh_interval_hours, 720);
        assert_eq!(settings.warmup_markets, ["US", "HK"]);
        assert_eq!(
            settings.source_policies[0].preferred_source_ids,
            ["hk_gov_1823_ical"]
        );
        assert_eq!(settings.source_policies[0].stale_after_hours, 0);
        assert!(settings.manual_overrides[0].sessions.is_empty());
    }
}
