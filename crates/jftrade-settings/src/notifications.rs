use std::collections::BTreeSet;
use std::sync::Arc;

use serde::{Deserialize, Serialize};

use crate::SettingsStoreError;

const DEFAULT_LEVELS: &[&str] = &["warn", "error"];
const DEFAULT_CATEGORIES: &[&str] = &[
    "broker.connection",
    "strategy.order.signal",
    "execution.order",
    "execution.fill",
];

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SystemNotificationSettings {
    #[serde(default)]
    pub enabled: bool,
    #[serde(default)]
    pub mode: String,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub levels: Vec<String>,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub categories: Vec<String>,
    #[serde(default)]
    pub sound_enabled: bool,
}

impl Default for SystemNotificationSettings {
    fn default() -> Self {
        Self {
            enabled: true,
            mode: "important".to_owned(),
            levels: DEFAULT_LEVELS.iter().map(ToString::to_string).collect(),
            categories: DEFAULT_CATEGORIES.iter().map(ToString::to_string).collect(),
            sound_enabled: true,
        }
    }
}

pub trait SystemNotificationSettingsStorePort: Send + Sync {
    fn load_system_notifications(
        &self,
    ) -> Result<Option<SystemNotificationSettings>, SettingsStoreError>;
    fn save_system_notifications(
        &self,
        settings: &SystemNotificationSettings,
    ) -> Result<(), SettingsStoreError>;
}

#[derive(Clone)]
pub struct SystemNotificationService {
    store: Arc<dyn SystemNotificationSettingsStorePort>,
}

impl SystemNotificationService {
    pub fn new(store: Arc<dyn SystemNotificationSettingsStorePort>) -> Self {
        Self { store }
    }

    pub fn settings(&self) -> Result<SystemNotificationSettings, SettingsStoreError> {
        Ok(self
            .store
            .load_system_notifications()?
            .map(|settings| normalize_system_notification_settings(&settings))
            .unwrap_or_default())
    }

    pub fn save(
        &self,
        input: &SystemNotificationSettings,
    ) -> Result<SystemNotificationSettings, SettingsStoreError> {
        let normalized = normalize_system_notification_settings(input);
        self.store.save_system_notifications(&normalized)?;
        Ok(normalized)
    }
}

pub fn normalize_system_notification_settings(
    input: &SystemNotificationSettings,
) -> SystemNotificationSettings {
    let mode = match input.mode.trim().to_ascii_lowercase().as_str() {
        "all" => "all",
        "custom" => "custom",
        _ => "important",
    };
    let (levels, categories) = match mode {
        "important" => (
            DEFAULT_LEVELS.iter().map(ToString::to_string).collect(),
            DEFAULT_CATEGORIES.iter().map(ToString::to_string).collect(),
        ),
        "all" => (Vec::new(), Vec::new()),
        _ => (
            normalize_list(&input.levels, true),
            normalize_list(&input.categories, false),
        ),
    };
    SystemNotificationSettings {
        enabled: input.enabled,
        mode: mode.to_owned(),
        levels,
        categories,
        sound_enabled: input.sound_enabled,
    }
}

pub fn should_forward_system_notification(
    settings: &SystemNotificationSettings,
    level: &str,
    category: &str,
) -> bool {
    if !settings.enabled {
        return false;
    }
    match settings.mode.trim().to_ascii_lowercase().as_str() {
        "all" => true,
        "custom" | "important" => {
            matches_value(level, &settings.levels) || matches_value(category, &settings.categories)
        }
        _ => false,
    }
}

fn matches_value(value: &str, candidates: &[String]) -> bool {
    let value = value.trim();
    !value.is_empty()
        && candidates
            .iter()
            .any(|candidate| value.eq_ignore_ascii_case(candidate.trim()))
}

fn normalize_list(values: &[String], lowercase: bool) -> Vec<String> {
    let mut seen = BTreeSet::new();
    values
        .iter()
        .filter_map(|value| {
            let value = value.trim();
            if value.is_empty() {
                return None;
            }
            let value = if lowercase {
                value.to_ascii_lowercase()
            } else {
                value.to_owned()
            };
            seen.insert(value.clone()).then_some(value)
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn notification_modes_and_ordered_deduplication_match_go() {
        let custom = normalize_system_notification_settings(&SystemNotificationSettings {
            enabled: true,
            mode: " CUSTOM ".to_owned(),
            levels: vec![" WARN ".to_owned(), "warn".to_owned(), "Error".to_owned()],
            categories: vec!["a".to_owned(), " a ".to_owned(), "B".to_owned()],
            sound_enabled: false,
        });
        assert_eq!(custom.mode, "custom");
        assert_eq!(custom.levels, ["warn", "error"]);
        assert_eq!(custom.categories, ["a", "B"]);

        let all = normalize_system_notification_settings(&SystemNotificationSettings {
            mode: "all".to_owned(),
            ..SystemNotificationSettings::default()
        });
        assert!(all.levels.is_empty() && all.categories.is_empty());

        assert!(should_forward_system_notification(
            &custom,
            "WARN",
            "unmatched"
        ));
        assert!(!should_forward_system_notification(
            &SystemNotificationSettings {
                enabled: false,
                ..custom
            },
            "error",
            "a"
        ));
    }
}
