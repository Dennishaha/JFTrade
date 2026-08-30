//! Explicit unavailable adapters for optional external integrations.

use serde_json::Value;
use std::sync::Arc;

use crate::product::product_market_data_provider_actions_port::{
    MarketDataProviderActionsPort, MarketDataProviderActionsPortError,
    MarketDataProviderActionsRequest,
};
use crate::product::product_market_data_subscription_mutation_port::{
    MarketDataSubscriptionMutationPort, MarketDataSubscriptionMutationPortError,
    MarketDataSubscriptionMutationRequest,
};
use crate::product::product_research_screen_write_port::{
    ResearchScreenWritePort, ResearchScreenWritePortError, ResearchScreenWriteQuery,
};
use crate::product::product_system_write_port::{
    SystemWriteInput, SystemWritePort, SystemWritePortError,
};
use crate::product::product_watchlist_remote_write_port::{
    RemoteWatchlistWriteAction, RemoteWatchlistWritePort, RemoteWatchlistWritePortError,
    RemoteWatchlistWriteResolution,
};
use crate::product::strategy_pine::{
    StrategyPineAnalyzeInput, StrategyPineAnalyzeSnapshotError, StrategyPineAnalyzeSnapshotPort,
};
use crate::product::{
    BrokerReadSnapshotError, BrokerReadSnapshotPort, MarketDataCatalogReadSnapshotError,
    MarketDataCatalogReadSnapshotPort, MarketDataDerivativeReadSnapshotError,
    MarketDataDerivativeReadSnapshotPort, MarketDataNewsActionsReadSnapshotError,
    MarketDataNewsActionsReadSnapshotPort, MarketDataNewsSearchReadSnapshotError,
    MarketDataNewsSearchReadSnapshotPort, MarketDataOptionsReadSnapshotError,
    MarketDataOptionsReadSnapshotPort, MarketDataPredictionReadSnapshotError,
    MarketDataPredictionReadSnapshotPort, MarketDataQuoteReadSnapshotError,
    MarketDataQuoteReadSnapshotPort, PortfolioSnapshotError, PortfolioSnapshotPort,
    RemoteWatchlistSnapshotError, RemoteWatchlistSnapshotPort, ResearchReadSnapshotError,
    ResearchReadSnapshotPort, WsLiveSnapshotPort,
};
use jftrade_api::{LiveHub, LiveHubLifecycle};

#[allow(dead_code)]
#[derive(Debug)]
pub(crate) struct ProductionUnavailablePort {
    reason: &'static str,
}

#[allow(dead_code)]
impl ProductionUnavailablePort {
    pub(super) fn new(reason: &'static str) -> Self {
        Self { reason }
    }
}

macro_rules! impl_unavailable_read_port {
    ($trait_name:ident, $error_name:ident) => {
        impl $trait_name for ProductionUnavailablePort {
            fn read(&self, _path: &str, _query: &str) -> Result<Value, $error_name> {
                Err($error_name::Unavailable(self.reason.to_owned()))
            }
        }
    };
}

impl_unavailable_read_port!(PortfolioSnapshotPort, PortfolioSnapshotError);
impl_unavailable_read_port!(ResearchReadSnapshotPort, ResearchReadSnapshotError);
impl_unavailable_read_port!(BrokerReadSnapshotPort, BrokerReadSnapshotError);
impl_unavailable_read_port!(
    MarketDataDerivativeReadSnapshotPort,
    MarketDataDerivativeReadSnapshotError
);
impl_unavailable_read_port!(
    MarketDataOptionsReadSnapshotPort,
    MarketDataOptionsReadSnapshotError
);
impl_unavailable_read_port!(
    MarketDataNewsActionsReadSnapshotPort,
    MarketDataNewsActionsReadSnapshotError
);
impl_unavailable_read_port!(
    MarketDataNewsSearchReadSnapshotPort,
    MarketDataNewsSearchReadSnapshotError
);
impl_unavailable_read_port!(
    MarketDataPredictionReadSnapshotPort,
    MarketDataPredictionReadSnapshotError
);

impl MarketDataCatalogReadSnapshotPort for ProductionUnavailablePort {
    fn read<'a>(
        &'a self,
        _path: &'a str,
        _query: &'a str,
    ) -> crate::product::MarketDataCatalogReadFuture<'a> {
        let reason = self.reason.to_owned();
        Box::pin(async move { Err(MarketDataCatalogReadSnapshotError::Unavailable(reason)) })
    }
}

impl MarketDataQuoteReadSnapshotPort for ProductionUnavailablePort {
    fn read<'a>(
        &'a self,
        _path: &'a str,
        _query: &'a str,
    ) -> crate::product::MarketDataQuoteReadFuture<'a> {
        let reason = self.reason.to_owned();
        Box::pin(async move { Err(MarketDataQuoteReadSnapshotError::Unavailable(reason)) })
    }
}

impl RemoteWatchlistSnapshotPort for ProductionUnavailablePort {
    fn read(&self, _query: &str) -> Result<Value, RemoteWatchlistSnapshotError> {
        Err(RemoteWatchlistSnapshotError::Unavailable(
            self.reason.to_owned(),
        ))
    }
}

impl MarketDataProviderActionsPort for ProductionUnavailablePort {
    fn dispatch<'a>(
        &'a self,
        _request: &'a MarketDataProviderActionsRequest,
    ) -> crate::product::product_market_data_provider_actions_port::MarketDataProviderActionsFuture<'a>{
        let reason = self.reason.to_owned();
        Box::pin(async move { Err(MarketDataProviderActionsPortError::Unavailable(reason)) })
    }
}

impl MarketDataSubscriptionMutationPort for ProductionUnavailablePort {
    fn dispatch(
        &self,
        _request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        Err(MarketDataSubscriptionMutationPortError::Unavailable(
            self.reason.to_owned(),
        ))
    }
}

impl ResearchScreenWritePort for ProductionUnavailablePort {
    fn query(
        &self,
        _request: &ResearchScreenWriteQuery,
    ) -> Result<Value, ResearchScreenWritePortError> {
        let _ = self.reason;
        Err(ResearchScreenWritePortError::Unavailable)
    }
}

impl SystemWritePort for ProductionUnavailablePort {
    fn mutate(&self, _input: &SystemWriteInput) -> Result<Value, SystemWritePortError> {
        Err(SystemWritePortError::Unavailable(self.reason.to_owned()))
    }
}

impl RemoteWatchlistWritePort for ProductionUnavailablePort {
    fn resolve(
        &self,
        _broker_id: Option<&str>,
        _account_id: Option<&str>,
    ) -> Result<RemoteWatchlistWriteResolution, RemoteWatchlistWritePortError> {
        Err(RemoteWatchlistWritePortError::Unavailable(
            self.reason.to_owned(),
        ))
    }

    fn apply(
        &self,
        _resolution: &RemoteWatchlistWriteResolution,
        _action: &RemoteWatchlistWriteAction,
    ) -> Result<Option<Value>, RemoteWatchlistWritePortError> {
        Err(RemoteWatchlistWritePortError::Unavailable(
            self.reason.to_owned(),
        ))
    }
}

impl StrategyPineAnalyzeSnapshotPort for ProductionUnavailablePort {
    fn analyze(
        &self,
        _input: &StrategyPineAnalyzeInput,
    ) -> Result<Value, StrategyPineAnalyzeSnapshotError> {
        Err(StrategyPineAnalyzeSnapshotError::Unavailable(
            self.reason.to_owned(),
        ))
    }
}

#[derive(Debug, Default)]
pub(crate) struct ProductionWsLivePort {
    live_hub: Option<Arc<LiveHub>>,
}

impl ProductionWsLivePort {
    pub(crate) fn new(live_hub: Option<Arc<LiveHub>>) -> Self {
        Self { live_hub }
    }
}

impl WsLiveSnapshotPort for ProductionWsLivePort {
    fn enabled(&self) -> bool {
        self.live_hub
            .as_ref()
            .is_some_and(|hub| hub.lifecycle() == LiveHubLifecycle::Serving)
    }
}
