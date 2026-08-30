use std::collections::BTreeMap;

use super::{ProductionAdapterBinding, ProductionRouteAdapter};

#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum ActiveMarketDataProvider {
    Futu,
    Yfinance,
    Akshare,
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub(crate) struct MarketDataCapabilityMatrix {
    pub active_provider: Option<ActiveMarketDataProvider>,
    pub helper_ready: bool,
    pub router_ready: bool,
}

impl MarketDataCapabilityMatrix {
    pub(crate) fn new(
        active_provider: Option<&str>,
        helper_ready: bool,
        router_ready: bool,
    ) -> Self {
        let active_provider = match active_provider
            .map(str::trim)
            .map(str::to_ascii_lowercase)
            .as_deref()
        {
            Some("futu") => Some(ActiveMarketDataProvider::Futu),
            Some("yfinance") => Some(ActiveMarketDataProvider::Yfinance),
            Some("akshare") => Some(ActiveMarketDataProvider::Akshare),
            _ => None,
        };
        Self {
            active_provider,
            helper_ready,
            router_ready,
        }
    }

    pub(crate) fn can_search(&self) -> bool {
        matches!(
            self.active_provider,
            Some(ActiveMarketDataProvider::Yfinance)
                | Some(ActiveMarketDataProvider::Akshare)
        ) && self.helper_ready
    }

    pub(crate) fn can_read_candles(&self) -> bool {
        self.can_search()
    }

    pub(crate) fn can_read_securities(&self) -> bool {
        self.can_search()
    }

    pub(crate) fn can_read_snapshots(&self) -> bool {
        match self.active_provider {
            Some(ActiveMarketDataProvider::Yfinance) | Some(ActiveMarketDataProvider::Akshare) => {
                self.helper_ready
            }
            Some(ActiveMarketDataProvider::Futu) => self.router_ready,
            None => false,
        }
    }

    pub(crate) fn can_read_markets(&self) -> bool {
        match self.active_provider {
            Some(ActiveMarketDataProvider::Yfinance) | Some(ActiveMarketDataProvider::Akshare) => {
                self.helper_ready
            }
            Some(ActiveMarketDataProvider::Futu) => true,
            None => false,
        }
    }

    pub(crate) fn can_mutate_subscriptions(&self) -> bool {
        match self.active_provider {
            Some(ActiveMarketDataProvider::Futu) => self.router_ready,
            Some(ActiveMarketDataProvider::Yfinance) | Some(ActiveMarketDataProvider::Akshare) => {
                self.helper_ready
            }
            None => false,
        }
    }

    pub(crate) fn can_read_news_actions(&self) -> bool {
        matches!(
            self.active_provider,
            Some(ActiveMarketDataProvider::Yfinance) | Some(ActiveMarketDataProvider::Akshare)
        ) && self.helper_ready
    }
}

fn bind_adapters(
    bindings: &mut BTreeMap<ProductionRouteAdapter, ProductionAdapterBinding>,
    status: ProductionAdapterBinding,
    adapters: &[ProductionRouteAdapter],
) {
    for adapter in adapters {
        let replaced = bindings.insert(*adapter, status);
        debug_assert!(replaced.is_none(), "duplicate production adapter binding");
    }
}

pub(crate) fn production_adapter_bindings(
    matrix: &MarketDataCapabilityMatrix,
) -> BTreeMap<ProductionRouteAdapter, ProductionAdapterBinding> {
    use ProductionAdapterBinding::{ExternalUnavailable, Ready};
    use ProductionRouteAdapter as Adapter;

    let mut bindings = BTreeMap::new();
    let mut ready = vec![
        Adapter::AuthSessionRead,
        Adapter::AuthSessionWrite,
        Adapter::Settings,
        Adapter::DataManagement,
        Adapter::SystemCore,
        Adapter::SystemRead,
        Adapter::RealTradeControlWrite,
        Adapter::Calendar,
        Adapter::WatchlistMemberships,
        Adapter::WatchlistRead,
        Adapter::WatchlistWrite,
        Adapter::StrategyDefinitionRead,
        Adapter::StrategyDefinitionWrite,
        Adapter::StrategyRuntimeRead,
        Adapter::StrategyRuntimeWrite,
        Adapter::ResearchCatalog,
        Adapter::ResearchPresetRead,
        Adapter::ResearchPresetWrite,
        Adapter::BacktestRead,
        Adapter::BacktestDelete,
        Adapter::BacktestSyncRead,
        Adapter::BacktestSyncCancel,
        Adapter::ExecutionRead,
        Adapter::PluginsRead,
        Adapter::PluginGuidanceRead,
        Adapter::AdkTemplatesRead,
        Adapter::AdkRead,
        Adapter::WebSocketLive,
        // The execution adapter is always installed. Its handlers perform
        // live Futu/OpenD readiness checks and fail closed when unavailable;
        // route registration must not hide those operations at startup.
        Adapter::ExecutionWrite,
        Adapter::MarketDataProviderRead,
        Adapter::MarketDataSubscriptionRead,
        Adapter::MarketDataInstrumentsNormalizeWrite,
        Adapter::PluginsWrite,
    ];
    let mut unavailable = vec![
        Adapter::SystemOpenDWrite,
        Adapter::RemoteWatchlistRead,
        Adapter::RemoteWatchlistWrite,
        Adapter::StrategyPine,
        Adapter::ResearchRead,
        Adapter::ResearchRankingsRead,
        Adapter::ResearchIndustriesRead,
        Adapter::ResearchCalendarRead,
        Adapter::ResearchMacroRead,
        Adapter::ResearchScreenWrite,
        Adapter::BacktestStart,
        Adapter::BrokerRead,
        Adapter::BrokerWrite,
        Adapter::PortfolioRead,
        Adapter::MarketDataDerivativeRead,
        Adapter::MarketDataFuturesRead,
        Adapter::MarketDataOptionsRead,
        Adapter::MarketDataOptionsChainRead,
        Adapter::MarketDataOptionsExpirationsRead,
        Adapter::MarketDataOptionsScreenRead,
        Adapter::MarketDataOptionsAnalysisRead,
        Adapter::MarketDataOptionsEventsRead,
        Adapter::MarketDataOptionsUnusualRead,
        Adapter::MarketDataOptionsZeroDteRead,
        Adapter::MarketDataOptionsZeroDteContractRead,
        Adapter::MarketDataOptionsEarningsRead,
        Adapter::MarketDataOptionsSellerRead,
        Adapter::MarketDataPredictionRead,
        Adapter::MarketDataDepthRead,
        Adapter::MarketDataTicksRead,
        Adapter::MarketDataBrokerQueueRead,
        Adapter::MarketDataCapitalFlowRead,
        Adapter::MarketDataIntradayRead,
        Adapter::MarketDataProfileRead,
        Adapter::MarketDataOptionsAnalysisWrite,
        Adapter::MarketDataZeroDteWrite,
        Adapter::MarketDataPredictionCombosWrite,
        Adapter::MarketDataPredictionSubscriptionAcquireWrite,
        Adapter::MarketDataPredictionSubscriptionReleaseWrite,
        Adapter::AlertsRead,
        Adapter::AlertsWrite,
        Adapter::AdkChat,
        // The ADK mutation adapter currently contains a mixed surface:
        // durable entity/workflow CRUD is local, while continuation, skill
        // install and workflow-run operations still require an external
        // assistant runtime. Keep the umbrella binding fail-closed so an
        // unimplemented operation can never be advertised as Ready.
        Adapter::AdkMutation,
    ];

    if matrix.can_search() {
        ready.extend([Adapter::MarketDataSearchRead, Adapter::MarketDataNewsSearchRead]);
    } else {
        unavailable.extend([Adapter::MarketDataSearchRead, Adapter::MarketDataNewsSearchRead]);
    }
    if matrix.can_read_candles() {
        ready.extend([Adapter::MarketDataCandlesRead, Adapter::BacktestSyncStart]);
    } else {
        unavailable.extend([Adapter::MarketDataCandlesRead, Adapter::BacktestSyncStart]);
    }
    if matrix.can_read_securities() {
        ready.push(Adapter::MarketDataSecuritiesRead);
    } else {
        unavailable.push(Adapter::MarketDataSecuritiesRead);
    }
    if matrix.can_read_markets() {
        ready.push(Adapter::MarketDataMarketsRead);
    } else {
        unavailable.push(Adapter::MarketDataMarketsRead);
    }
    if matrix.can_read_snapshots() {
        ready.extend([
            Adapter::MarketDataSnapshotsRead,
            Adapter::MarketDataBatchSnapshotsWrite,
        ]);
    } else {
        unavailable.extend([
            Adapter::MarketDataSnapshotsRead,
            Adapter::MarketDataBatchSnapshotsWrite,
        ]);
    }
    if matrix.can_mutate_subscriptions() {
        ready.extend([
            Adapter::MarketDataSubscriptionAcquireWrite,
            Adapter::MarketDataSubscriptionReleaseWrite,
            Adapter::MarketDataSubscriptionClearWrite,
            Adapter::MarketDataSubscriptionHeartbeatWrite,
        ]);
    } else {
        unavailable.extend([
            Adapter::MarketDataSubscriptionAcquireWrite,
            Adapter::MarketDataSubscriptionReleaseWrite,
            Adapter::MarketDataSubscriptionClearWrite,
            Adapter::MarketDataSubscriptionHeartbeatWrite,
        ]);
    }
    if matrix.can_read_news_actions() {
        ready.push(Adapter::MarketDataNewsActionsRead);
    } else {
        unavailable.push(Adapter::MarketDataNewsActionsRead);
    }

    bind_adapters(&mut bindings, Ready, &ready);
    bind_adapters(&mut bindings, ExternalUnavailable, &unavailable);
    bindings
}
