//! Stage 4 composition boundary. Product traffic remains owned by Go until the
//! later API cutover; this type proves that capability and integration owners
//! can be assembled without creating reverse dependencies.

use jftrade_integration_futu::SubscriptionReconciler;
use jftrade_integration_marketdata_helper::{HelperClient, HelperClientConfig, HttpAdapterError};
use jftrade_integration_pine::{PoolError, WorkerPool};
use jftrade_marketdata::{HealthStatus, MarketDataError, ProviderDescriptor, ProviderRouter};
use thiserror::Error;

pub struct Stage4Assembly {
    pub marketdata: ProviderRouter,
    pub helper: HelperClient,
    pub pine: WorkerPool,
    pub futu_subscriptions: SubscriptionReconciler,
}

#[derive(Debug, Error)]
pub enum Stage4AssemblyError {
    #[error(transparent)]
    MarketData(#[from] MarketDataError),
    #[error(transparent)]
    Helper(#[from] HttpAdapterError),
    #[error(transparent)]
    Pine(#[from] PoolError),
}

impl Stage4Assembly {
    pub fn new(
        providers: impl IntoIterator<Item = (ProviderDescriptor, HealthStatus)>,
        helper_config: HelperClientConfig,
        pine_workers: impl IntoIterator<Item = (String, String)>,
    ) -> Result<Self, Stage4AssemblyError> {
        let mut marketdata = ProviderRouter::new(512);
        for (descriptor, health) in providers {
            marketdata.register(descriptor, health)?;
        }
        Ok(Self {
            marketdata,
            helper: HelperClient::new(helper_config)?,
            pine: WorkerPool::new(pine_workers)?,
            futu_subscriptions: SubscriptionReconciler::new(60_000),
        })
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use jftrade_marketdata::{ProviderCapabilities, ProviderConstraints, ProviderReadiness};

    use super::*;

    #[test]
    fn assembly_keeps_provider_and_integration_owners_explicit() {
        let descriptor = ProviderDescriptor {
            selection_id: "futu".to_owned(),
            provider_id: "futu".to_owned(),
            display_name: "Futu OpenD".to_owned(),
            broker_id: Some("futu".to_owned()),
            source: "futu".to_owned(),
            default_market: "HK".to_owned(),
            supported_markets: vec!["HK".to_owned(), "US".to_owned()],
            transports: vec!["stream".to_owned()],
            capabilities: ProviderCapabilities {
                snapshots: true,
                streaming_quotes: true,
                ..ProviderCapabilities::default()
            },
            constraints: ProviderConstraints {
                requires_open_d: true,
                requires_market_data_right: true,
                uses_subscription_quota: true,
            },
            notes: Vec::new(),
        };
        let health = HealthStatus {
            connected: true,
            readiness: ProviderReadiness::Ready,
            stream_mode: "streaming".to_owned(),
            ..HealthStatus::default()
        };
        let assembly = Stage4Assembly::new(
            [(descriptor, health)],
            HelperClientConfig {
                base_url: "http://127.0.0.1:7788".to_owned(),
                bearer_token: None,
                request_timeout: Duration::from_secs(1),
                max_attempts: 1,
                retry_delay: Duration::ZERO,
            },
            [("pineworker-1".to_owned(), "127.0.0.1:50051".to_owned())],
        )
        .expect("assembly");
        assert_eq!(assembly.marketdata.runtime().generation, 0);
        assert_eq!(assembly.pine.snapshot().len(), 1);
    }
}
