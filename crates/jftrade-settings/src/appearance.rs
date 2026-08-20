use std::sync::Arc;

use serde::{Deserialize, Serialize};
use thiserror::Error;

const DEFAULT_UP_COLOR: &str = "#16c784";
const DEFAULT_DOWN_COLOR: &str = "#ea3943";

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct UiAppearanceSettings {
    pub up_color: String,
    pub down_color: String,
}

impl Default for UiAppearanceSettings {
    fn default() -> Self {
        Self {
            up_color: DEFAULT_UP_COLOR.to_owned(),
            down_color: DEFAULT_DOWN_COLOR.to_owned(),
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
#[error("settings store failed: {message}")]
pub struct SettingsStoreError {
    message: String,
}

impl SettingsStoreError {
    pub fn new(message: impl Into<String>) -> Self {
        Self {
            message: message.into(),
        }
    }

    pub fn message(&self) -> &str {
        &self.message
    }
}

pub trait SettingsStorePort: Send + Sync {
    fn load_appearance(&self) -> Result<Option<UiAppearanceSettings>, SettingsStoreError>;
    fn save_appearance(&self, appearance: &UiAppearanceSettings) -> Result<(), SettingsStoreError>;
}

#[derive(Clone)]
pub struct AppearanceService {
    store: Arc<dyn SettingsStorePort>,
}

impl AppearanceService {
    pub fn new(store: Arc<dyn SettingsStorePort>) -> Self {
        Self { store }
    }

    pub fn appearance(&self) -> Result<UiAppearanceSettings, SettingsStoreError> {
        Ok(self
            .store
            .load_appearance()?
            .map(|value| normalize_appearance(&value))
            .unwrap_or_default())
    }

    pub fn save_appearance(
        &self,
        input: &UiAppearanceSettings,
    ) -> Result<UiAppearanceSettings, SettingsStoreError> {
        let normalized = normalize_appearance(input);
        self.store.save_appearance(&normalized)?;
        Ok(normalized)
    }
}

pub fn normalize_appearance(input: &UiAppearanceSettings) -> UiAppearanceSettings {
    UiAppearanceSettings {
        up_color: normalize_hex_color(&input.up_color, DEFAULT_UP_COLOR),
        down_color: normalize_hex_color(&input.down_color, DEFAULT_DOWN_COLOR),
    }
}

fn normalize_hex_color(value: &str, fallback: &str) -> String {
    let trimmed = value.trim();
    let valid = trimmed.len() == 7
        && trimmed.starts_with('#')
        && trimmed[1..].bytes().all(|byte| byte.is_ascii_hexdigit());
    if valid {
        trimmed.to_ascii_lowercase()
    } else {
        fallback.to_owned()
    }
}

#[cfg(test)]
mod tests {
    use std::sync::RwLock;

    use super::*;

    #[derive(Default)]
    struct MemoryStore(RwLock<Option<UiAppearanceSettings>>);

    impl SettingsStorePort for MemoryStore {
        fn load_appearance(&self) -> Result<Option<UiAppearanceSettings>, SettingsStoreError> {
            Ok(self.0.read().expect("read appearance").clone())
        }

        fn save_appearance(
            &self,
            appearance: &UiAppearanceSettings,
        ) -> Result<(), SettingsStoreError> {
            *self.0.write().expect("write appearance") = Some(appearance.clone());
            Ok(())
        }
    }

    #[test]
    fn appearance_defaults_and_normalization_match_the_go_owner() {
        let service = AppearanceService::new(Arc::new(MemoryStore::default()));
        assert_eq!(
            service.appearance().expect("default"),
            UiAppearanceSettings::default()
        );

        let saved = service
            .save_appearance(&UiAppearanceSettings {
                up_color: " #ABCDEF ".into(),
                down_color: "not-a-color".into(),
            })
            .expect("save");
        assert_eq!(
            saved,
            UiAppearanceSettings {
                up_color: "#abcdef".into(),
                down_color: DEFAULT_DOWN_COLOR.into(),
            }
        );
        assert_eq!(service.appearance().expect("reload"), saved);
    }

    #[test]
    fn normalization_rejects_non_ascii_and_wrong_length_colors() {
        for invalid in ["#１２３４５６", "#12345", "123456", "#12345g", ""] {
            assert_eq!(
                normalize_hex_color(invalid, DEFAULT_UP_COLOR),
                DEFAULT_UP_COLOR
            );
        }
    }
}
