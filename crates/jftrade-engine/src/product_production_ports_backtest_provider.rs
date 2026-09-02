//! Process-local backtest market-data provider override.
//!
//! The settings service owns persistence; this port owns the atomic runtime
//! snapshot consumed by backtest start/sync.  Composition should initialize it
//! from the persisted settings value and install it as the backtest settings
//! runtime before serving requests.

use jftrade_settings::{MarketDataProvider, MarketDataProviderRuntimePort};
use std::sync::{Arc, RwLock};

#[allow(dead_code)]
#[derive(Clone)]
pub(crate) struct BacktestMarketDataProviderState {
    provider: Arc<RwLock<MarketDataProvider>>,
}

impl std::fmt::Debug for BacktestMarketDataProviderState {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("BacktestMarketDataProviderState")
            .field("provider", &self.get())
            .finish()
    }
}

#[allow(dead_code)]
impl BacktestMarketDataProviderState {
    pub(crate) fn new(initial: MarketDataProvider) -> Self {
        Self {
            provider: Arc::new(RwLock::new(initial)),
        }
    }

    pub(crate) fn get(&self) -> MarketDataProvider {
        *self
            .provider
            .read()
            .unwrap_or_else(|error| error.into_inner())
    }

    /// Atomically publish the provider used by newly accepted backtests.
    pub(crate) fn set(&self, provider: MarketDataProvider) -> MarketDataProvider {
        let mut current = self
            .provider
            .write()
            .unwrap_or_else(|error| error.into_inner());
        let previous = *current;
        *current = provider;
        previous
    }
}

impl MarketDataProviderRuntimePort for BacktestMarketDataProviderState {
    fn needs_activation(&self, provider: MarketDataProvider) -> bool {
        self.get() != provider
    }

    fn activate(&self, provider: MarketDataProvider) -> Result<(), String> {
        self.set(provider);
        Ok(())
    }

    fn prepare_backtest(&self, provider: MarketDataProvider) -> Result<(), String> {
        self.set(provider);
        Ok(())
    }
}
