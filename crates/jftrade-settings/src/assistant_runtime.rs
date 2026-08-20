use std::sync::Arc;

use serde::{Deserialize, Serialize};

use crate::SettingsStoreError;

const DEFAULT_RUN_TIMEOUT_MS: i32 = 1_800_000;
const DEFAULT_STREAM_IDLE_TIMEOUT_MS: i32 = 300_000;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct AssistantRuntimeSettings {
    pub run_timeout_ms: i32,
    pub stream_idle_timeout_ms: i32,
}

impl Default for AssistantRuntimeSettings {
    fn default() -> Self {
        Self {
            run_timeout_ms: DEFAULT_RUN_TIMEOUT_MS,
            stream_idle_timeout_ms: DEFAULT_STREAM_IDLE_TIMEOUT_MS,
        }
    }
}

pub trait AssistantRuntimeSettingsStorePort: Send + Sync {
    fn load_assistant_runtime(
        &self,
    ) -> Result<Option<AssistantRuntimeSettings>, SettingsStoreError>;
    fn save_assistant_runtime(
        &self,
        settings: &AssistantRuntimeSettings,
    ) -> Result<(), SettingsStoreError>;
}

#[derive(Clone)]
pub struct AssistantRuntimeService {
    store: Arc<dyn AssistantRuntimeSettingsStorePort>,
}

impl AssistantRuntimeService {
    pub fn new(store: Arc<dyn AssistantRuntimeSettingsStorePort>) -> Self {
        Self { store }
    }

    pub fn settings(&self) -> Result<AssistantRuntimeSettings, SettingsStoreError> {
        Ok(self
            .store
            .load_assistant_runtime()?
            .map(|settings| normalize_assistant_runtime_settings(&settings))
            .unwrap_or_default())
    }

    pub fn save(
        &self,
        input: &AssistantRuntimeSettings,
    ) -> Result<AssistantRuntimeSettings, SettingsStoreError> {
        let normalized = normalize_assistant_runtime_settings(input);
        self.store.save_assistant_runtime(&normalized)?;
        Ok(normalized)
    }
}

pub fn normalize_assistant_runtime_settings(
    input: &AssistantRuntimeSettings,
) -> AssistantRuntimeSettings {
    AssistantRuntimeSettings {
        run_timeout_ms: clamp_or_default(
            input.run_timeout_ms,
            DEFAULT_RUN_TIMEOUT_MS,
            60_000,
            43_200_000,
        ),
        stream_idle_timeout_ms: clamp_or_default(
            input.stream_idle_timeout_ms,
            DEFAULT_STREAM_IDLE_TIMEOUT_MS,
            30_000,
            900_000,
        ),
    }
}

fn clamp_or_default(value: i32, fallback: i32, minimum: i32, maximum: i32) -> i32 {
    if value <= 0 {
        fallback
    } else {
        value.clamp(minimum, maximum)
    }
}

#[cfg(test)]
mod tests {
    use std::sync::RwLock;

    use super::*;

    #[derive(Default)]
    struct MemoryStore(RwLock<Option<AssistantRuntimeSettings>>);

    impl AssistantRuntimeSettingsStorePort for MemoryStore {
        fn load_assistant_runtime(
            &self,
        ) -> Result<Option<AssistantRuntimeSettings>, SettingsStoreError> {
            Ok(self.0.read().expect("read assistant settings").clone())
        }

        fn save_assistant_runtime(
            &self,
            settings: &AssistantRuntimeSettings,
        ) -> Result<(), SettingsStoreError> {
            *self.0.write().expect("write assistant settings") = Some(settings.clone());
            Ok(())
        }
    }

    #[test]
    fn assistant_runtime_defaults_bounds_and_round_trip_match_go() {
        let service = AssistantRuntimeService::new(Arc::new(MemoryStore::default()));
        assert_eq!(
            service.settings().expect("defaults"),
            AssistantRuntimeSettings::default()
        );
        let saved = service
            .save(&AssistantRuntimeSettings {
                run_timeout_ms: 1,
                stream_idle_timeout_ms: i32::MAX,
            })
            .expect("save");
        assert_eq!(
            saved,
            AssistantRuntimeSettings {
                run_timeout_ms: 60_000,
                stream_idle_timeout_ms: 900_000,
            }
        );
        assert_eq!(service.settings().expect("reload"), saved);
    }
}
