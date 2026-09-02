use std::sync::Arc;

use serde::{Deserialize, Serialize};

use crate::SettingsStoreError;

const DEFAULT_BROKER_ORDER_HISTORY_LOOKBACK_DAYS: i32 = 30;
const DEFAULT_SEEN_FILL_RETENTION_DAYS: i32 = 90;

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub struct ExecutionSettings {
    pub default_trading_environment: String,
    pub broker_order_history_lookback_days: i32,
    pub seen_fill_retention_days: i32,
}

impl Default for ExecutionSettings {
    fn default() -> Self {
        Self {
            default_trading_environment: "SIMULATE".to_owned(),
            broker_order_history_lookback_days: DEFAULT_BROKER_ORDER_HISTORY_LOOKBACK_DAYS,
            seen_fill_retention_days: DEFAULT_SEEN_FILL_RETENTION_DAYS,
        }
    }
}

pub trait ExecutionSettingsStorePort: Send + Sync {
    fn load_execution(&self) -> Result<Option<ExecutionSettings>, SettingsStoreError>;
    fn save_execution(&self, settings: &ExecutionSettings) -> Result<(), SettingsStoreError>;
}

#[derive(Clone)]
pub struct ExecutionService {
    store: Arc<dyn ExecutionSettingsStorePort>,
}

impl ExecutionService {
    pub fn new(store: Arc<dyn ExecutionSettingsStorePort>) -> Self {
        Self { store }
    }

    pub fn settings(&self) -> Result<ExecutionSettings, SettingsStoreError> {
        Ok(self
            .store
            .load_execution()?
            .map(|settings| normalize_execution_settings(&settings))
            .unwrap_or_default())
    }

    pub fn save(&self, input: &ExecutionSettings) -> Result<ExecutionSettings, SettingsStoreError> {
        let normalized = normalize_execution_settings(input);
        self.store.save_execution(&normalized)?;
        Ok(normalized)
    }
}

pub fn normalize_execution_settings(input: &ExecutionSettings) -> ExecutionSettings {
    ExecutionSettings {
        default_trading_environment: match input
            .default_trading_environment
            .trim()
            .to_ascii_uppercase()
            .as_str()
        {
            "SIMULATE" => "SIMULATE".to_owned(),
            "REAL" => "REAL".to_owned(),
            _ => "SIMULATE".to_owned(),
        },
        broker_order_history_lookback_days: clamp_or_default(
            input.broker_order_history_lookback_days,
            DEFAULT_BROKER_ORDER_HISTORY_LOOKBACK_DAYS,
            1,
            365,
        ),
        seen_fill_retention_days: clamp_or_default(
            input.seen_fill_retention_days,
            DEFAULT_SEEN_FILL_RETENTION_DAYS,
            1,
            3_650,
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
    struct MemoryStore(RwLock<Option<ExecutionSettings>>);

    impl ExecutionSettingsStorePort for MemoryStore {
        fn load_execution(&self) -> Result<Option<ExecutionSettings>, SettingsStoreError> {
            Ok(self.0.read().expect("read execution settings").clone())
        }

        fn save_execution(&self, settings: &ExecutionSettings) -> Result<(), SettingsStoreError> {
            *self.0.write().expect("write execution settings") = Some(settings.clone());
            Ok(())
        }
    }

    #[test]
    fn execution_defaults_bounds_and_round_trip_match_go() {
        let service = ExecutionService::new(Arc::new(MemoryStore::default()));
        assert_eq!(
            service.settings().expect("defaults"),
            ExecutionSettings::default()
        );
        let saved = service
            .save(&ExecutionSettings {
                default_trading_environment: " real ".to_owned(),
                broker_order_history_lookback_days: 999,
                seen_fill_retention_days: -1,
            })
            .expect("save");
        assert_eq!(
            saved,
            ExecutionSettings {
                default_trading_environment: "REAL".to_owned(),
                broker_order_history_lookback_days: 365,
                seen_fill_retention_days: DEFAULT_SEEN_FILL_RETENTION_DAYS,
            }
        );
        assert_eq!(service.settings().expect("reload"), saved);
    }
}
