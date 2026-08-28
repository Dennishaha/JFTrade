//! Production market-data adapters bundle.
//!
//! Connects catalog reads, quote reads, subscription mutations, and provider
//! actions to real production state without mock fixtures or dummy arrays.

#[path = "product_production_ports_market_data_actions.rs"]
mod product_production_ports_market_data_actions;
#[path = "product_production_ports_market_data_catalog.rs"]
mod product_production_ports_market_data_catalog;
#[path = "product_production_ports_market_data_projection.rs"]
pub(crate) mod product_production_ports_market_data_projection;
#[path = "product_production_ports_market_data_quote.rs"]
mod product_production_ports_market_data_quote;
#[path = "product_production_ports_market_data_subscription.rs"]
mod product_production_ports_market_data_subscription;

pub(crate) use product_production_ports_market_data_actions::ProductionMarketDataProviderActionsPort;
pub(crate) use product_production_ports_market_data_catalog::ProductionMarketDataCatalogPort;
pub(crate) use product_production_ports_market_data_quote::ProductionMarketDataQuotePort;
pub(crate) use product_production_ports_market_data_subscription::ProductionMarketDataSubscriptionMutationPort;

use std::sync::Arc;
use serde_json::Value;
use crate::product::product_active_provider_state::ActiveProviderState;
use crate::product::{
    MarketDataDerivativeReadSnapshotError, MarketDataDerivativeReadSnapshotPort,
    MarketDataNewsActionsReadSnapshotError, MarketDataNewsActionsReadSnapshotPort,
    MarketDataNewsSearchReadSnapshotError, MarketDataNewsSearchReadSnapshotPort,
    MarketDataOptionsReadSnapshotError, MarketDataOptionsReadSnapshotPort,
    MarketDataPredictionReadSnapshotError, MarketDataPredictionReadSnapshotPort,
};

#[derive(Debug)]
pub(crate) struct ProductionMarketDataDerivativePort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
}

impl MarketDataDerivativeReadSnapshotPort for ProductionMarketDataDerivativePort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, MarketDataDerivativeReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(MarketDataDerivativeReadSnapshotError::Unavailable(
                "derivative market-data provider is not configured".to_owned(),
            ));
        }
        Err(MarketDataDerivativeReadSnapshotError::Unavailable(
            "derivative market-data provider is not configured".to_owned(),
        ))
    }
}

#[derive(Debug)]
pub(crate) struct ProductionMarketDataOptionsPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
}

impl MarketDataOptionsReadSnapshotPort for ProductionMarketDataOptionsPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, MarketDataOptionsReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || !snapshot.opend_ready {
            return Err(MarketDataOptionsReadSnapshotError::Unavailable(
                "options market-data provider is not configured".to_owned(),
            ));
        }
        Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "options market-data provider is not configured".to_owned(),
        ))
    }
}

#[derive(Debug)]
pub(crate) struct ProductionMarketDataNewsPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
}

impl MarketDataNewsActionsReadSnapshotPort for ProductionMarketDataNewsPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, MarketDataNewsActionsReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || (!snapshot.helper_ready && !snapshot.opend_ready) {
            return Err(MarketDataNewsActionsReadSnapshotError::Unavailable(
                "news provider is not configured".to_owned(),
            ));
        }
        Err(MarketDataNewsActionsReadSnapshotError::Unavailable(
            "news provider is not configured".to_owned(),
        ))
    }
}

impl MarketDataNewsSearchReadSnapshotPort for ProductionMarketDataNewsPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, MarketDataNewsSearchReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() || (!snapshot.helper_ready && !snapshot.opend_ready) {
            return Err(MarketDataNewsSearchReadSnapshotError::Unavailable(
                "news provider is not configured".to_owned(),
            ));
        }
        Err(MarketDataNewsSearchReadSnapshotError::Unavailable(
            "news provider is not configured".to_owned(),
        ))
    }
}

#[derive(Debug)]
pub(crate) struct ProductionMarketDataPredictionPort {
    pub(crate) active_provider_state: Arc<ActiveProviderState>,
}

impl MarketDataPredictionReadSnapshotPort for ProductionMarketDataPredictionPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, MarketDataPredictionReadSnapshotError> {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider.is_none() {
            return Err(MarketDataPredictionReadSnapshotError::Unavailable(
                "prediction market-data provider is not configured".to_owned(),
            ));
        }
        Err(MarketDataPredictionReadSnapshotError::Unavailable(
            "prediction market-data provider is not configured".to_owned(),
        ))
    }
}

