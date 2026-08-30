//! Explicit production route-adapter binding states.

use std::collections::BTreeMap;

use super::{ProductionPortBundle, ProductionRouteAdapter};

/// Query operations accepted by the shared `/options/analysis` endpoint.
/// Keep this list in lock-step with `parse_operation` in the production
/// options reader so readiness can be inspected per operation instead of
/// treating the whole endpoint as ready when only one reader exists.
pub(crate) const OPTION_ANALYSIS_OPERATIONS: &[&str] = &[
    "quote",
    "volatility",
    "exercise_probability",
    "underlying_overview",
    "market_statistics",
    "historical_statistics",
    "historical_volatility",
    "strategy_spread",
    "strategy",
    "strategy_analysis",
    "underlying_rank",
    "contract_rank",
];

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
        if let Some(operation) = option_event_operation(adapter) {
            let snapshot = self.active_provider_state.snapshot();
            let ready = snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                && snapshot.opend_ready
                && self
                    .trade_runtime
                    .as_ref()
                    .is_some_and(|runtime| match operation {
                        OptionEventOperation::Unusual => runtime.option_events_available(),
                        OptionEventOperation::ZeroDte => {
                            runtime.option_zero_dte_screener_available()
                        }
                        OptionEventOperation::ZeroDteContract => {
                            runtime.option_zero_dte_contract_available()
                        }
                        OptionEventOperation::Earnings => {
                            runtime.option_earnings_screener_available()
                        }
                        OptionEventOperation::Seller => runtime.option_seller_screener_available(),
                    });
            return Some(if ready {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            });
        }
        if matches!(
            adapter,
            ProductionRouteAdapter::ResearchRankingsRead
                | ProductionRouteAdapter::ResearchIndustriesRead
                | ProductionRouteAdapter::ResearchCalendarRead
                | ProductionRouteAdapter::ResearchMacroRead
        ) {
            let snapshot = self.active_provider_state.snapshot();
            let ready = snapshot.helper_ready
                && match adapter {
                    ProductionRouteAdapter::ResearchRankingsRead => matches!(
                        snapshot.provider,
                        Some(jftrade_settings::MarketDataProvider::Yfinance)
                            | Some(jftrade_settings::MarketDataProvider::Akshare)
                    ),
                    ProductionRouteAdapter::ResearchIndustriesRead => {
                        snapshot.provider == Some(jftrade_settings::MarketDataProvider::Akshare)
                    }
                    ProductionRouteAdapter::ResearchCalendarRead
                    | ProductionRouteAdapter::ResearchMacroRead => {
                        snapshot.provider == Some(jftrade_settings::MarketDataProvider::Akshare)
                    }
                    _ => false,
                };
            return Some(if ready {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            });
        }
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
        if adapter == ProductionRouteAdapter::MarketDataFuturesRead {
            let snapshot = self.active_provider_state.snapshot();
            return Some(
                if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.future_info_available())
                {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                },
            );
        }
        if matches!(
            adapter,
            ProductionRouteAdapter::MarketDataNewsSearchRead
                | ProductionRouteAdapter::MarketDataNewsActionsRead
        ) {
            let snapshot = self.active_provider_state.snapshot();
            let ready = match (adapter, snapshot.provider) {
                (
                    ProductionRouteAdapter::MarketDataNewsSearchRead,
                    Some(jftrade_settings::MarketDataProvider::Futu),
                ) => {
                    snapshot.opend_ready
                        && self
                            .trade_runtime
                            .as_ref()
                            .is_some_and(|runtime| runtime.news_reader_available())
                }
                (
                    ProductionRouteAdapter::MarketDataNewsActionsRead,
                    Some(jftrade_settings::MarketDataProvider::Futu),
                ) => {
                    snapshot.opend_ready
                        && self
                            .trade_runtime
                            .as_ref()
                            .is_some_and(|runtime| runtime.corporate_actions_reader_available())
                }
                (_, Some(jftrade_settings::MarketDataProvider::Yfinance))
                | (_, Some(jftrade_settings::MarketDataProvider::Akshare)) => snapshot.helper_ready,
                _ => false,
            };
            return Some(if ready {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            });
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
            let operation_ready = OPTION_EVENT_OPERATION_ADAPTERS
                .iter()
                .any(|(_, operation)| {
                    self.adapter_binding(*operation) == Some(ProductionAdapterBinding::Ready)
                });
            return Some(
                if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    // The public route is shared by five query operations;
                    // it is ready only when at least one concrete operation
                    // adapter is installed. Each operation remains exposed
                    // independently through operation_binding().
                    && operation_ready
                {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                },
            );
        }
        if adapter == ProductionRouteAdapter::MarketDataZeroDteWrite {
            let snapshot = self.active_provider_state.snapshot();
            return Some(
                if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.option_zero_dte_contract_available())
                {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                },
            );
        }
        if matches!(
            adapter,
            ProductionRouteAdapter::RemoteWatchlistRead
                | ProductionRouteAdapter::RemoteWatchlistWrite
                | ProductionRouteAdapter::AlertsRead
                | ProductionRouteAdapter::AlertsWrite
        ) {
            let snapshot = self.active_provider_state.snapshot();
            let ready = snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                && snapshot.opend_ready
                && self
                    .trade_runtime
                    .as_ref()
                    .is_some_and(|runtime| match adapter {
                        ProductionRouteAdapter::RemoteWatchlistRead => {
                            runtime.remote_watchlist_reader().is_some()
                        }
                        ProductionRouteAdapter::RemoteWatchlistWrite => {
                            runtime.remote_watchlist_writer().is_some()
                        }
                        ProductionRouteAdapter::AlertsRead => runtime.alert_reader().is_some(),
                        ProductionRouteAdapter::AlertsWrite => runtime.alert_writer().is_some(),
                        _ => false,
                    });
            return Some(if ready {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            });
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
            let binding = production_adapter_bindings(&MarketDataCapabilityMatrix::new(
                provider,
                snapshot.helper_ready,
                // Router readiness is an independent capability from the
                // OpenD transport.  An OpenD session without the shared
                // ProviderRouter must not make snapshot/subscription routes
                // appear ready: those handlers have no demand/cache owner to
                // serve the request.  External OpenD health is evaluated by
                // the operation-specific adapter when it is required.
                snapshot.router_ready,
            ))
            .get(&adapter)
            .copied();
            // A logical ProviderRouter can exist before OpenD has completed
            // its connection handshake. Futu snapshot/subscription adapters
            // depend on both pieces: without a live OpenD session the router
            // has no physical feed/cache owner and must remain unavailable.
            return if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                && !snapshot.opend_ready
                && matches!(
                    adapter,
                    ProductionRouteAdapter::MarketDataSnapshotsRead
                        | ProductionRouteAdapter::MarketDataBatchSnapshotsWrite
                        | ProductionRouteAdapter::MarketDataSubscriptionRead
                        | ProductionRouteAdapter::MarketDataSubscriptionAcquireWrite
                        | ProductionRouteAdapter::MarketDataSubscriptionReleaseWrite
                        | ProductionRouteAdapter::MarketDataSubscriptionClearWrite
                        | ProductionRouteAdapter::MarketDataSubscriptionHeartbeatWrite
                ) {
                Some(ProductionAdapterBinding::ExternalUnavailable)
            } else {
                binding
            };
        }
        self.bound_adapters.get(&adapter).copied()
    }

    /// Resolve readiness for one `operation=` value on the shared options
    /// analysis route.  The route remains registered when at least one
    /// operation is available, while callers can distinguish unsupported
    /// operations and receive the normal external-unavailable response.
    pub(crate) fn option_analysis_operation_binding(
        &self,
        operation: &str,
    ) -> Option<ProductionAdapterBinding> {
        let snapshot = self.active_provider_state.snapshot();
        let ready = snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
            && snapshot.opend_ready
            && self
                .trade_runtime
                .as_ref()
                .is_some_and(|runtime| match operation {
                    "quote" => runtime.option_quotes_available(),
                    "volatility" => runtime.option_volatility_available(),
                    "exercise_probability" => runtime.option_exercise_probability_available(),
                    "underlying_overview" => runtime.option_underlying_overview_available(),
                    "market_statistics" => runtime.option_market_statistic_available(),
                    "historical_statistics" => runtime.option_underlying_his_statistic_available(),
                    "historical_volatility" => runtime.option_underlying_his_volatility_available(),
                    "strategy_spread" => runtime.option_strategy_spread_available(),
                    "strategy" => runtime.option_strategy_available(),
                    "strategy_analysis" => runtime.option_strategy_analysis_available(),
                    "underlying_rank" => runtime.option_underlying_rank_available(),
                    "contract_rank" => runtime.option_contract_rank_available(),
                    _ => false,
                });
        Some(if ready {
            ProductionAdapterBinding::Ready
        } else {
            ProductionAdapterBinding::ExternalUnavailable
        })
    }

    /// Resolve readiness for an individual research operation.  The public
    /// research surface is intentionally one `ResearchRead` adapter, but its
    /// implementations are not uniform: helper-backed instrument routes and
    /// the Futu valuation reader have independent prerequisites while the
    /// remaining baseline routes are deliberately unavailable.  Keeping this
    /// decision path-specific prevents a ready helper/OpenD runtime from
    /// advertising unsupported research operations as ready.
    pub(crate) fn research_operation_binding(
        &self,
        path: &str,
    ) -> Option<ProductionAdapterBinding> {
        let snapshot = self.active_provider_state.snapshot();
        let corporate_actions_route = path
            .strip_prefix("/api/v1/research/corporate-actions/")
            .is_some_and(|suffix| !suffix.is_empty() && !suffix.contains('/'));
        let helper_route = [
            "/api/v1/research/instruments/",
            "/api/v1/research/financials/",
            "/api/v1/research/analyst/",
            "/api/v1/research/ownership/",
        ]
        .iter()
        .any(|prefix| {
            path.strip_prefix(prefix)
                .is_some_and(|suffix| !suffix.is_empty() && !suffix.contains('/'))
        });
        let valuation_route = path
            .strip_prefix("/api/v1/research/valuation/")
            .is_some_and(|suffix| !suffix.is_empty() && !suffix.contains('/'));
        let ready = if corporate_actions_route {
            match snapshot.provider {
                Some(jftrade_settings::MarketDataProvider::Futu) => {
                    snapshot.opend_ready
                        && self
                            .trade_runtime
                            .as_ref()
                            .is_some_and(|runtime| runtime.corporate_actions_reader_available())
                }
                Some(jftrade_settings::MarketDataProvider::Yfinance)
                | Some(jftrade_settings::MarketDataProvider::Akshare) => snapshot.helper_ready,
                None => false,
            }
        } else if helper_route {
            snapshot.helper_ready
                && matches!(
                    snapshot.provider,
                    Some(jftrade_settings::MarketDataProvider::Yfinance)
                        | Some(jftrade_settings::MarketDataProvider::Akshare)
                )
        } else if valuation_route {
            snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                && snapshot.opend_ready
                && self
                    .trade_runtime
                    .as_ref()
                    .is_some_and(|runtime| runtime.valuation_detail_available())
        } else {
            false
        };
        Some(if ready {
            ProductionAdapterBinding::Ready
        } else {
            ProductionAdapterBinding::ExternalUnavailable
        })
    }
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) enum OptionEventOperation {
    Unusual,
    ZeroDte,
    ZeroDteContract,
    Earnings,
    Seller,
}

pub(crate) const OPTION_EVENT_OPERATION_ADAPTERS: &[(&str, ProductionRouteAdapter)] = &[
    (
        "unusual",
        ProductionRouteAdapter::MarketDataOptionsUnusualRead,
    ),
    (
        "zero_dte",
        ProductionRouteAdapter::MarketDataOptionsZeroDteRead,
    ),
    (
        "zero_dte_contract",
        ProductionRouteAdapter::MarketDataOptionsZeroDteContractRead,
    ),
    (
        "earnings",
        ProductionRouteAdapter::MarketDataOptionsEarningsRead,
    ),
    (
        "seller",
        ProductionRouteAdapter::MarketDataOptionsSellerRead,
    ),
];

fn option_event_operation(adapter: ProductionRouteAdapter) -> Option<OptionEventOperation> {
    Some(match adapter {
        ProductionRouteAdapter::MarketDataOptionsUnusualRead => OptionEventOperation::Unusual,
        ProductionRouteAdapter::MarketDataOptionsZeroDteRead => OptionEventOperation::ZeroDte,
        ProductionRouteAdapter::MarketDataOptionsZeroDteContractRead => {
            OptionEventOperation::ZeroDteContract
        }
        ProductionRouteAdapter::MarketDataOptionsEarningsRead => OptionEventOperation::Earnings,
        ProductionRouteAdapter::MarketDataOptionsSellerRead => OptionEventOperation::Seller,
        _ => return None,
    })
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
            | MarketDataNewsSearchRead
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
            Some(ActiveMarketDataProvider::Yfinance) | Some(ActiveMarketDataProvider::Akshare)
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
        Adapter::ResearchRankingsRead,
        Adapter::ResearchIndustriesRead,
        Adapter::ResearchCalendarRead,
        Adapter::ResearchMacroRead,
        Adapter::ResearchScreenWrite,
        Adapter::BacktestStart,
        Adapter::ExecutionWrite,
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
    ];

    if matrix.can_search() {
        ready.push(Adapter::MarketDataSearchRead);
        ready.push(Adapter::MarketDataNewsSearchRead);
    } else {
        unavailable.push(Adapter::MarketDataSearchRead);
        unavailable.push(Adapter::MarketDataNewsSearchRead);
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
#[path = "product_production_adapter_bindings_tests.rs"]
mod tests;
