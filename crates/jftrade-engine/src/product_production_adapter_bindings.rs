//! Explicit production route-adapter binding states.

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
    /// The route is backed by a production-owned adapter, but the
    /// composition root did not install that adapter.  This is distinct from
    /// an unavailable external dependency: silently treating an internal
    /// omission as unavailable would let the route reach a fallback handler
    /// (or a static fixture) and report a misleading success.
    MissingInternalAdapter,
}

impl ProductionPortBundle {
    /// Resolve readiness for execution routes that require more than the
    /// generic order writer.  The public execution adapter also owns plain
    /// order placement/cancellation, but buying-power and combo previews have
    /// stricter product-rule readers and must not inherit that writer status.
    pub(crate) fn execution_operation_binding(
        &self,
        path: &str,
    ) -> Option<ProductionAdapterBinding> {
        if !self
            .installed_adapters
            .contains(&ProductionRouteAdapter::ExecutionWrite)
        {
            return None;
        }
        if !self
            .bound_adapters
            .contains_key(&ProductionRouteAdapter::ExecutionWrite)
        {
            return None;
        }
        if self.bound_adapters.get(&ProductionRouteAdapter::ExecutionWrite)
            == Some(&ProductionAdapterBinding::MissingInternalAdapter)
        {
            return Some(ProductionAdapterBinding::MissingInternalAdapter);
        }
        match path {
            // The Rust product-rule port is intentionally fail-closed until a
            // real broker implementation is installed; local parsing alone
            // must never project `allowed: true` as a capability.
            "/api/v1/execution/buying-power" => {
                Some(ProductionAdapterBinding::ExternalUnavailable)
            }
            "/api/v1/execution/combos/previews" => {
                let snapshot = self.active_provider_state.snapshot();
                let option_reader = self.trade_runtime.as_ref().is_some_and(|runtime| {
                    // The handler accepts both generic and spread strategy
                    // payloads, and combo_preview always projects
                    // optionAnalysis. All three readers are therefore needed
                    // for the route-wide Ready claim.
                    runtime.option_strategy_available()
                        && runtime.option_strategy_spread_available()
                        && runtime.option_strategy_analysis_available()
                });
                let trade_reader = if let Some(runtime) = self.trade_runtime.as_ref() {
                    runtime.snapshot().is_ready()
                } else {
                    self.trade_read_port
                        .as_ref()
                        .is_some_and(|_| self.trade_logged_in == Some(true))
                };
                Some(if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                    && snapshot.opend_ready
                    && option_reader
                    && trade_reader
                {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                })
            }
            _ => None,
        }
    }

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
        if !self.installed_adapters.contains(&adapter) {
            return None;
        }
        // Presence in the installation set is the composition-root proof;
        // this readiness map then supplies the startup/status projection.
        // Dynamic readiness below may downgrade an installed adapter when an
        // external provider/runtime is absent, but it must never manufacture
        // a status for an adapter that was not wired at all.
        if !self.bound_adapters.contains_key(&adapter) {
            return None;
        }
        if self.bound_adapters.get(&adapter)
            == Some(&ProductionAdapterBinding::MissingInternalAdapter)
        {
            return Some(ProductionAdapterBinding::MissingInternalAdapter);
        }
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
        if matches!(
            adapter,
            ProductionRouteAdapter::ExecutionWrite | ProductionRouteAdapter::BrokerWrite
        ) {
            let snapshot = self.active_provider_state.snapshot();
            // A runtime handle is authoritative once it has been installed.
            // Never retain a startup writer/login bit after a reconnect or
            // teardown: doing so advertises a ready route while dispatch will
            // fail (or, worse, sends a command through a stale session).
            let trade_ready = if let Some(runtime) = self.trade_runtime.as_ref() {
                runtime.snapshot().is_ready() && runtime.writer_snapshot().is_some()
            } else {
                self.trade_logged_in == Some(true) && self.trade_write_port.is_some()
            };
            let ready = snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                && snapshot.opend_ready
                && trade_ready;
            return Some(if ready {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            });
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

    /// Resolve a binding for composition-root validation.  Every internal
    /// production adapter must be present in the bundle; an absent entry is
    /// not an external outage and must not be downgraded to
    /// `ExternalUnavailable`.
    pub(crate) fn adapter_binding_or_missing(
        &self,
        adapter: ProductionRouteAdapter,
    ) -> ProductionAdapterBinding {
        self.adapter_binding(adapter)
            .unwrap_or(ProductionAdapterBinding::MissingInternalAdapter)
    }

    /// Resolve readiness for one `operation=` value on the shared options
    /// analysis route.  The route remains registered when at least one
    /// operation is available, while callers can distinguish unsupported
    /// operations and receive the normal external-unavailable response.
    pub(crate) fn option_analysis_operation_binding(
        &self,
        operation: &str,
    ) -> Option<ProductionAdapterBinding> {
        if !self
            .installed_adapters
            .contains(&ProductionRouteAdapter::MarketDataOptionsAnalysisRead)
        {
            return None;
        }
        if !self
            .bound_adapters
            .contains_key(&ProductionRouteAdapter::MarketDataOptionsAnalysisRead)
        {
            return None;
        }
        if self
            .bound_adapters
            .get(&ProductionRouteAdapter::MarketDataOptionsAnalysisRead)
            == Some(&ProductionAdapterBinding::MissingInternalAdapter)
        {
            return Some(ProductionAdapterBinding::MissingInternalAdapter);
        }
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
        if !self
            .installed_adapters
            .contains(&ProductionRouteAdapter::ResearchRead)
        {
            return None;
        }
        if !self
            .bound_adapters
            .contains_key(&ProductionRouteAdapter::ResearchRead)
        {
            return None;
        }
        if self.bound_adapters.get(&ProductionRouteAdapter::ResearchRead)
            == Some(&ProductionAdapterBinding::MissingInternalAdapter)
        {
            return Some(ProductionAdapterBinding::MissingInternalAdapter);
        }
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

/// Runtime-scoped adapters must not fall back to their startup readiness when
/// the live capability reader no longer returns a binding (for example after
/// teardown or a failed reconnect).
pub(crate) fn runtime_scoped_adapter(adapter: ProductionRouteAdapter) -> bool {
    is_dynamic_market_data_adapter(adapter)
        || matches!(
            adapter,
            ProductionRouteAdapter::ResearchRead
                | ProductionRouteAdapter::ExecutionWrite
                | ProductionRouteAdapter::BrokerRead
                | ProductionRouteAdapter::BrokerWrite
                | ProductionRouteAdapter::PortfolioRead
                | ProductionRouteAdapter::RemoteWatchlistRead
                | ProductionRouteAdapter::RemoteWatchlistWrite
                | ProductionRouteAdapter::AlertsRead
                | ProductionRouteAdapter::AlertsWrite
                | ProductionRouteAdapter::MarketDataOptionsExpirationsRead
                | ProductionRouteAdapter::MarketDataFuturesRead
                | ProductionRouteAdapter::MarketDataOptionsChainRead
                | ProductionRouteAdapter::MarketDataOptionsScreenRead
                | ProductionRouteAdapter::MarketDataOptionsAnalysisRead
                | ProductionRouteAdapter::MarketDataOptionsEventsRead
                | ProductionRouteAdapter::MarketDataOptionsUnusualRead
                | ProductionRouteAdapter::MarketDataOptionsZeroDteRead
                | ProductionRouteAdapter::MarketDataOptionsZeroDteContractRead
                | ProductionRouteAdapter::MarketDataOptionsEarningsRead
                | ProductionRouteAdapter::MarketDataOptionsSellerRead
                | ProductionRouteAdapter::MarketDataZeroDteWrite
                | ProductionRouteAdapter::WebSocketLive
        )
}

#[path = "product_production_adapter_bindings_matrix.rs"]
mod matrix;
pub(crate) use matrix::{
    MarketDataCapabilityMatrix, production_adapter_bindings,
};

#[cfg(test)]
#[path = "product_production_adapter_bindings_tests.rs"]
mod tests;
