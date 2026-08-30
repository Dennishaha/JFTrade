use std::sync::Arc;

use serde::{Deserialize, Serialize};
use thiserror::Error;

use crate::SettingsStoreError;

#[derive(Clone, Copy, Debug, Default, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum MarketDataProvider {
    Futu,
    Yfinance,
    #[default]
    Akshare,
}

pub trait MarketDataProviderSettingsStorePort: Send + Sync {
    fn load_active_market_data_provider(&self) -> Result<Option<String>, SettingsStoreError>;

    fn save_active_market_data_provider(
        &self,
        provider: MarketDataProvider,
    ) -> Result<(), SettingsStoreError>;
}

pub trait BacktestMarketDataProviderSettingsStorePort: Send + Sync {
    fn load_backtest_market_data_provider(&self) -> Result<Option<String>, SettingsStoreError>;

    fn save_backtest_market_data_provider(
        &self,
        provider: MarketDataProvider,
    ) -> Result<(), SettingsStoreError>;
}

pub trait MarketDataProviderRuntimePort: Send + Sync {
    fn needs_activation(&self, provider: MarketDataProvider) -> bool;
    fn activate(&self, provider: MarketDataProvider) -> Result<(), String>;
    fn prepare_backtest(&self, provider: MarketDataProvider) -> Result<(), String>;
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum MarketDataProviderSettingsError {
    #[error("active market-data provider must be futu, yfinance, or akshare")]
    Invalid,
    #[error("could not apply market-data provider settings: {0}")]
    Runtime(String),
    #[error(transparent)]
    Store(#[from] SettingsStoreError),
}

#[derive(Clone)]
pub struct MarketDataProviderSettingsService {
    store: Arc<dyn MarketDataProviderSettingsStorePort>,
    runtime: Option<Arc<dyn MarketDataProviderRuntimePort>>,
}

#[derive(Clone)]
pub struct BacktestMarketDataProviderSettingsService {
    store: Arc<dyn BacktestMarketDataProviderSettingsStorePort>,
    runtime: Option<Arc<dyn MarketDataProviderRuntimePort>>,
}

impl BacktestMarketDataProviderSettingsService {
    pub fn new(store: Arc<dyn BacktestMarketDataProviderSettingsStorePort>) -> Self {
        Self {
            store,
            runtime: None,
        }
    }

    pub fn with_runtime(mut self, runtime: Arc<dyn MarketDataProviderRuntimePort>) -> Self {
        self.runtime = Some(runtime);
        self
    }

    pub fn active_provider(&self) -> Result<MarketDataProvider, SettingsStoreError> {
        Ok(self
            .store
            .load_backtest_market_data_provider()?
            .as_deref()
            .map(normalize_market_data_provider)
            .unwrap_or_default())
    }

    pub fn save(&self, input: &str) -> Result<MarketDataProvider, MarketDataProviderSettingsError> {
        let current = self.active_provider()?;
        let next = parse_market_data_provider(input)?;
        if next == current {
            return Ok(next);
        }
        // Persist first so a failed settings write can never leave the
        // process-local runtime pointing at a provider that will be lost on
        // restart.  If runtime preparation fails, restore the old durable
        // value before returning the error.
        self.store.save_backtest_market_data_provider(next)?;
        if let Some(runtime) = &self.runtime
            && let Err(error) = runtime.prepare_backtest(next)
        {
            if let Err(rollback_error) = self.store.save_backtest_market_data_provider(current) {
                return Err(MarketDataProviderSettingsError::Runtime(format!(
                    "{error}; settings rollback failed: {rollback_error}"
                )));
            }
            return Err(MarketDataProviderSettingsError::Runtime(error));
        }
        Ok(next)
    }
}

impl MarketDataProviderSettingsService {
    pub fn new(store: Arc<dyn MarketDataProviderSettingsStorePort>) -> Self {
        Self {
            store,
            runtime: None,
        }
    }

    pub fn with_runtime(mut self, runtime: Arc<dyn MarketDataProviderRuntimePort>) -> Self {
        self.runtime = Some(runtime);
        self
    }

    pub fn active_provider(&self) -> Result<MarketDataProvider, SettingsStoreError> {
        Ok(self
            .store
            .load_active_market_data_provider()?
            .as_deref()
            .map(normalize_market_data_provider)
            .unwrap_or_default())
    }

    pub fn save(&self, input: &str) -> Result<MarketDataProvider, MarketDataProviderSettingsError> {
        let current = self.active_provider()?;
        let next = parse_market_data_provider(input)?;
        self.store.save_active_market_data_provider(next)?;
        let Some(runtime) = &self.runtime else {
            return Ok(next);
        };
        if next == current && !runtime.needs_activation(next) {
            return Ok(next);
        }
        if let Err(error) = runtime.activate(next) {
            if next != current
                && let Err(rollback_error) = self.store.save_active_market_data_provider(current)
            {
                return Err(MarketDataProviderSettingsError::Runtime(format!(
                    "{error}; settings rollback failed: {rollback_error}"
                )));
            }
            return Err(MarketDataProviderSettingsError::Runtime(error));
        }
        Ok(next)
    }
}

pub fn normalize_market_data_provider(input: &str) -> MarketDataProvider {
    match input.trim().to_ascii_lowercase().as_str() {
        "futu" => MarketDataProvider::Futu,
        "yfinance" => MarketDataProvider::Yfinance,
        "akshare" => MarketDataProvider::Akshare,
        _ => MarketDataProvider::default(),
    }
}

pub fn parse_market_data_provider(
    input: &str,
) -> Result<MarketDataProvider, MarketDataProviderSettingsError> {
    match input.trim().to_ascii_lowercase().as_str() {
        "futu" => Ok(MarketDataProvider::Futu),
        "yfinance" => Ok(MarketDataProvider::Yfinance),
        "akshare" => Ok(MarketDataProvider::Akshare),
        _ => Err(MarketDataProviderSettingsError::Invalid),
    }
}

pub const fn provider_id(provider: MarketDataProvider) -> &'static str {
    match provider {
        MarketDataProvider::Futu => "futu",
        MarketDataProvider::Yfinance => "yfinance",
        MarketDataProvider::Akshare => "akshare",
    }
}

#[cfg(test)]
mod tests {
    use std::sync::{Mutex, RwLock};

    use super::*;

    struct Store(RwLock<Option<String>>);

    struct BacktestStore(RwLock<Option<String>>);

    struct Runtime {
        calls: Mutex<Vec<(String, MarketDataProvider)>>,
        fail: bool,
    }

    impl MarketDataProviderSettingsStorePort for Store {
        fn load_active_market_data_provider(&self) -> Result<Option<String>, SettingsStoreError> {
            self.0
                .read()
                .map(|value| value.clone())
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }

        fn save_active_market_data_provider(
            &self,
            provider: MarketDataProvider,
        ) -> Result<(), SettingsStoreError> {
            self.0
                .write()
                .map(|mut value| *value = Some(provider_id(provider).to_owned()))
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }
    }

    impl BacktestMarketDataProviderSettingsStorePort for BacktestStore {
        fn load_backtest_market_data_provider(&self) -> Result<Option<String>, SettingsStoreError> {
            self.0
                .read()
                .map(|value| value.clone())
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }

        fn save_backtest_market_data_provider(
            &self,
            provider: MarketDataProvider,
        ) -> Result<(), SettingsStoreError> {
            self.0
                .write()
                .map(|mut value| *value = Some(provider_id(provider).to_owned()))
                .map_err(|_| SettingsStoreError::new("poisoned"))
        }
    }

    impl MarketDataProviderRuntimePort for Runtime {
        fn needs_activation(&self, _provider: MarketDataProvider) -> bool {
            true
        }

        fn activate(&self, provider: MarketDataProvider) -> Result<(), String> {
            self.calls
                .lock()
                .expect("runtime calls")
                .push(("activate".to_owned(), provider));
            if self.fail {
                Err("activation failed".to_owned())
            } else {
                Ok(())
            }
        }

        fn prepare_backtest(&self, provider: MarketDataProvider) -> Result<(), String> {
            self.calls
                .lock()
                .expect("runtime calls")
                .push(("prepare".to_owned(), provider));
            if self.fail {
                Err("preparation failed".to_owned())
            } else {
                Ok(())
            }
        }
    }

    #[test]
    fn provider_normalization_matches_current_go_defaults() {
        assert_eq!(
            normalize_market_data_provider(" FUTU "),
            MarketDataProvider::Futu
        );
        assert_eq!(
            normalize_market_data_provider(" yfinance "),
            MarketDataProvider::Yfinance
        );
        assert_eq!(
            normalize_market_data_provider("unknown"),
            MarketDataProvider::Akshare
        );
        let service = MarketDataProviderSettingsService::new(Arc::new(Store(RwLock::new(None))));
        assert_eq!(
            service.active_provider().expect("active provider"),
            MarketDataProvider::Akshare
        );
        assert_eq!(
            service.save(" YFINANCE ").expect("save provider"),
            MarketDataProvider::Yfinance
        );
        assert_eq!(
            service.save("invalid"),
            Err(MarketDataProviderSettingsError::Invalid)
        );
    }

    #[test]
    fn active_failure_rolls_back_but_backtest_failure_never_persists() {
        let active_store = Arc::new(Store(RwLock::new(Some("yfinance".to_owned()))));
        let runtime = Arc::new(Runtime {
            calls: Mutex::new(Vec::new()),
            fail: true,
        });
        let active = MarketDataProviderSettingsService::new(active_store.clone())
            .with_runtime(runtime.clone());
        assert!(matches!(
            active.save("futu"),
            Err(MarketDataProviderSettingsError::Runtime(_))
        ));
        assert_eq!(
            active_store.0.read().expect("active store").as_deref(),
            Some("yfinance")
        );

        let backtest_store = Arc::new(BacktestStore(RwLock::new(Some("akshare".to_owned()))));
        let backtest = BacktestMarketDataProviderSettingsService::new(backtest_store.clone())
            .with_runtime(runtime);
        assert!(matches!(
            backtest.save("futu"),
            Err(MarketDataProviderSettingsError::Runtime(_))
        ));
        assert_eq!(
            backtest_store.0.read().expect("backtest store").as_deref(),
            Some("akshare")
        );
    }
}
