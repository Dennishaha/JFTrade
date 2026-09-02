use std::sync::Arc;

use serde::{Deserialize, Serialize};

use crate::SettingsStoreError;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PineWorkerSettings {
    pub backtest_worker_limit: i32,
    pub instance_worker_limit: i32,
    pub node_binary_path: String,
}

impl Default for PineWorkerSettings {
    fn default() -> Self {
        Self {
            backtest_worker_limit: 2,
            instance_worker_limit: 10,
            node_binary_path: String::new(),
        }
    }
}

pub trait PineWorkerSettingsStorePort: Send + Sync {
    fn load_pine_worker(&self) -> Result<Option<PineWorkerSettings>, SettingsStoreError>;
    fn save_pine_worker(&self, settings: &PineWorkerSettings) -> Result<(), SettingsStoreError>;
}

#[derive(Clone)]
pub struct PineWorkerSettingsService {
    store: Arc<dyn PineWorkerSettingsStorePort>,
}

impl PineWorkerSettingsService {
    pub fn new(store: Arc<dyn PineWorkerSettingsStorePort>) -> Self {
        Self { store }
    }

    pub fn settings(&self) -> Result<PineWorkerSettings, SettingsStoreError> {
        Ok(self
            .store
            .load_pine_worker()?
            .map(|value| normalize_pine_worker_settings(&value))
            .unwrap_or_default())
    }

    pub fn save(
        &self,
        input: &PineWorkerSettings,
    ) -> Result<PineWorkerSettings, SettingsStoreError> {
        let normalized = normalize_pine_worker_settings(input);
        self.store.save_pine_worker(&normalized)?;
        Ok(normalized)
    }
}

pub fn normalize_pine_worker_settings(input: &PineWorkerSettings) -> PineWorkerSettings {
    PineWorkerSettings {
        backtest_worker_limit: input.backtest_worker_limit.clamp(1, 1_000),
        instance_worker_limit: input.instance_worker_limit.clamp(1, 1_000),
        node_binary_path: normalize_executable_path(&input.node_binary_path),
    }
}

pub fn normalize_executable_path(input: &str) -> String {
    let mut value = input.trim();
    loop {
        let bytes = value.as_bytes();
        if bytes.len() < 2 {
            break;
        }
        let first = bytes[0];
        let last = bytes[bytes.len() - 1];
        if !matches!(first, b'"' | b'\'') || first != last {
            break;
        }
        value = value[1..value.len() - 1].trim();
    }
    value.to_owned()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn worker_limits_and_nested_quotes_match_go_settings_owner() {
        assert_eq!(
            normalize_pine_worker_settings(&PineWorkerSettings {
                backtest_worker_limit: 0,
                instance_worker_limit: 1_001,
                node_binary_path: " \' \"/opt/node\" \' ".to_owned(),
            }),
            PineWorkerSettings {
                backtest_worker_limit: 1,
                instance_worker_limit: 1_000,
                node_binary_path: "/opt/node".to_owned(),
            }
        );
        assert_eq!(PineWorkerSettings::default().backtest_worker_limit, 2);
    }
}
