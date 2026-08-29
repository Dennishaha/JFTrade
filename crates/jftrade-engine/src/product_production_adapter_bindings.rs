//! Explicit production route-adapter binding states.

use std::collections::BTreeMap;

use super::{ProductionPortBundle, ProductionRouteAdapter};

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum ProductionAdapterBinding {
    Ready,
    ExternalUnavailable,
}

impl ProductionPortBundle {
    #[cfg(test)]
    pub(crate) fn database_leases(
        &self,
    ) -> &crate::product::product_production_ports::ProductionDatabaseLeaseSnapshot {
        &self.database_leases
    }

    /// Return the concrete binding installed by the production composition
    /// root. A missing adapter is a startup error; an external-unavailable
    /// adapter is an intentional fail-closed boundary that keeps the public
    /// route available while its process/provider is absent.
    pub(crate) fn adapter_binding(
        &self,
        adapter: ProductionRouteAdapter,
    ) -> Option<ProductionAdapterBinding> {
        if matches!(
            adapter,
            ProductionRouteAdapter::BrokerRead | ProductionRouteAdapter::PortfolioRead
        ) {
            let snapshot = self.active_provider_state.snapshot();
            return Some(
                if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && (self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.snapshot().is_ready())
                        || (self.trade_runtime.is_none()
                            && self.trade_logged_in == Some(true)
                            && self.trade_read_port.is_some()))
                {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                },
            );
        }
        if adapter == ProductionRouteAdapter::WebSocketLive && !self.ws_live.enabled() {
            return None;
        }
        if adapter == ProductionRouteAdapter::MarketDataOptionsExpirationsRead {
            let snapshot = self.active_provider_state.snapshot();
            return Some(
                if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.option_expirations_available())
                {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                },
            );
        }
        if adapter == ProductionRouteAdapter::MarketDataOptionsChainRead {
            let snapshot = self.active_provider_state.snapshot();
            return Some(
                if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.option_chains_available())
                {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                },
            );
        }
        if adapter == ProductionRouteAdapter::MarketDataOptionsScreenRead {
            let snapshot = self.active_provider_state.snapshot();
            return Some(
                if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.option_screens_available())
                {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                },
            );
        }
        if adapter == ProductionRouteAdapter::MarketDataOptionsAnalysisRead {
            let snapshot = self.active_provider_state.snapshot();
            return Some(
                if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && self.trade_runtime.as_ref().is_some_and(|runtime| {
                        runtime.option_quotes_available()
                            || runtime.option_volatility_available()
                            || runtime.option_exercise_probability_available()
                            || runtime.option_underlying_overview_available()
                            || runtime.option_underlying_his_volatility_available()
                            || runtime.option_market_statistic_available()
                            || runtime.option_underlying_his_statistic_available()
                            || runtime.option_strategy_spread_available()
                            || runtime.option_strategy_analysis_available()
                            || runtime.option_underlying_rank_available()
                            || runtime.option_contract_rank_available()
                    })
                {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                },
            );
        }
        if adapter == ProductionRouteAdapter::MarketDataOptionsEventsRead {
            let snapshot = self.active_provider_state.snapshot();
            return Some(
                if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| {
                            runtime.option_events_available()
                                || runtime.option_zero_dte_screener_available()
                                || runtime.option_earnings_screener_available()
                                || runtime.option_zero_dte_contract_available()
                                || runtime.option_seller_screener_available()
                        })
                {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                },
            );
        }
        // Market-data capability is provider-dependent and can change at
        // runtime. Recompute those bindings from the shared snapshot instead
        // of exposing the startup matrix after a provider transition.
        if is_dynamic_market_data_adapter(adapter) {
            let snapshot = self.active_provider_state.snapshot();
            let provider = snapshot.provider.map(|provider| match provider {
                jftrade_settings::MarketDataProvider::Futu => "futu",
                jftrade_settings::MarketDataProvider::Yfinance => "yfinance",
                jftrade_settings::MarketDataProvider::Akshare => "akshare",
            });
            return production_adapter_bindings(&MarketDataCapabilityMatrix::new(
                provider,
                snapshot.helper_ready,
                snapshot.opend_ready || snapshot.router_ready,
            ))
            .get(&adapter)
            .copied();
        }
        self.bound_adapters.get(&adapter).copied()
    }
}

fn is_dynamic_market_data_adapter(adapter: ProductionRouteAdapter) -> bool {
    use ProductionRouteAdapter::*;
    matches!(
        adapter,
        MarketDataSearchRead
            | MarketDataCandlesRead
            | MarketDataSecuritiesRead
            | MarketDataMarketsRead
            | MarketDataSnapshotsRead
            | MarketDataBatchSnapshotsWrite
            | MarketDataSubscriptionRead
            | MarketDataSubscriptionAcquireWrite
            | MarketDataSubscriptionReleaseWrite
            | MarketDataSubscriptionClearWrite
            | MarketDataSubscriptionHeartbeatWrite
            | MarketDataNewsActionsRead
            | BacktestSyncStart
    )
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
        match self.active_provider {
            Some(ActiveMarketDataProvider::Yfinance) | Some(ActiveMarketDataProvider::Akshare) => {
                self.helper_ready
            }
            _ => false,
        }
    }

    pub(crate) fn can_read_candles(&self) -> bool {
        match self.active_provider {
            Some(ActiveMarketDataProvider::Yfinance) | Some(ActiveMarketDataProvider::Akshare) => {
                self.helper_ready
            }
            _ => false,
        }
    }

    pub(crate) fn can_read_securities(&self) -> bool {
        match self.active_provider {
            Some(ActiveMarketDataProvider::Yfinance) | Some(ActiveMarketDataProvider::Akshare) => {
                self.helper_ready
            }
            _ => false,
        }
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
            Some(ActiveMarketDataProvider::Yfinance)
        ) && self.helper_ready
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
        Adapter::AdkMutation,
        Adapter::WebSocketLive,
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
        Adapter::ResearchScreenWrite,
        Adapter::BacktestStart,
        Adapter::ExecutionWrite,
        Adapter::BrokerRead,
        Adapter::BrokerWrite,
        Adapter::PortfolioRead,
        Adapter::MarketDataDerivativeRead,
        Adapter::MarketDataOptionsRead,
        Adapter::MarketDataOptionsChainRead,
        Adapter::MarketDataOptionsExpirationsRead,
        Adapter::MarketDataOptionsScreenRead,
        Adapter::MarketDataOptionsAnalysisRead,
        Adapter::MarketDataOptionsEventsRead,
        Adapter::MarketDataNewsSearchRead,
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
    ];

    if matrix.can_search() {
        ready.push(Adapter::MarketDataSearchRead);
    } else {
        unavailable.push(Adapter::MarketDataSearchRead);
    }

    if matrix.can_read_candles() {
        ready.push(Adapter::MarketDataCandlesRead);
        ready.push(Adapter::BacktestSyncStart);
    } else {
        unavailable.push(Adapter::MarketDataCandlesRead);
        unavailable.push(Adapter::BacktestSyncStart);
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
        ready.push(Adapter::MarketDataSnapshotsRead);
        ready.push(Adapter::MarketDataBatchSnapshotsWrite);
    } else {
        unavailable.push(Adapter::MarketDataSnapshotsRead);
        unavailable.push(Adapter::MarketDataBatchSnapshotsWrite);
    }

    if matrix.can_mutate_subscriptions() {
        ready.push(Adapter::MarketDataSubscriptionAcquireWrite);
        ready.push(Adapter::MarketDataSubscriptionReleaseWrite);
        ready.push(Adapter::MarketDataSubscriptionClearWrite);
        ready.push(Adapter::MarketDataSubscriptionHeartbeatWrite);
    } else {
        unavailable.push(Adapter::MarketDataSubscriptionAcquireWrite);
        unavailable.push(Adapter::MarketDataSubscriptionReleaseWrite);
        unavailable.push(Adapter::MarketDataSubscriptionClearWrite);
        unavailable.push(Adapter::MarketDataSubscriptionHeartbeatWrite);
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn news_actions_binding_requires_yfinance_helper_readiness() {
        let ready = production_adapter_bindings(&MarketDataCapabilityMatrix::new(
            Some("yfinance"),
            true,
            false,
        ));
        assert_eq!(
            ready.get(&ProductionRouteAdapter::MarketDataNewsActionsRead),
            Some(&ProductionAdapterBinding::Ready)
        );

        for matrix in [
            MarketDataCapabilityMatrix::new(Some("yfinance"), false, false),
            MarketDataCapabilityMatrix::new(Some("akshare"), true, false),
            MarketDataCapabilityMatrix::new(Some("futu"), false, true),
        ] {
            let bindings = production_adapter_bindings(&matrix);
            assert_eq!(
                bindings.get(&ProductionRouteAdapter::MarketDataNewsActionsRead),
                Some(&ProductionAdapterBinding::ExternalUnavailable)
            );
        }
    }

    #[test]
    fn option_chain_binding_defaults_to_external_unavailable() {
        let bindings = production_adapter_bindings(&MarketDataCapabilityMatrix::new(
            Some("futu"),
            false,
            true,
        ));
        assert_eq!(
            bindings.get(&ProductionRouteAdapter::MarketDataOptionsChainRead),
            Some(&ProductionAdapterBinding::ExternalUnavailable)
        );
        assert_eq!(
            bindings.get(&ProductionRouteAdapter::MarketDataOptionsScreenRead),
            Some(&ProductionAdapterBinding::ExternalUnavailable)
        );
        assert_eq!(
            bindings.get(&ProductionRouteAdapter::MarketDataOptionsAnalysisRead),
            Some(&ProductionAdapterBinding::ExternalUnavailable)
        );
        assert_eq!(
            bindings.get(&ProductionRouteAdapter::MarketDataOptionsEventsRead),
            Some(&ProductionAdapterBinding::ExternalUnavailable)
        );
    }
}
