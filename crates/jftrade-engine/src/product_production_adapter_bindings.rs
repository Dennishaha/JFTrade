//! Explicit production route-adapter binding states.

use super::{ProductionPortBundle, ProductionRouteAdapter};
use crate::product::product_adk_mutation_port::AdkMutationOperation;

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
    /// Resolve readiness for one ADK mutation operation. The public route
    /// registry keeps a single compatibility adapter, while this method
    /// projects each operation from its actual local/runtime dependency.
    pub(crate) fn adk_mutation_operation_binding(
        &self,
        operation: AdkMutationOperation,
    ) -> Option<ProductionAdapterBinding> {
        let adapter = ProductionRouteAdapter::AdkMutation;
        if !self.installed_adapters.contains(&adapter)
            || !self.bound_adapters.contains_key(&adapter)
        {
            return None;
        }
        if self.bound_adapters.get(&adapter)
            == Some(&ProductionAdapterBinding::MissingInternalAdapter)
        {
            return Some(ProductionAdapterBinding::MissingInternalAdapter);
        }
        let model_runtime_operation = matches!(
            operation,
            AdkMutationOperation::TestProvider
                | AdkMutationOperation::RespondToInput
                | AdkMutationOperation::RunWorkflowTrigger
                | AdkMutationOperation::RunWorkflowWebhook
                | AdkMutationOperation::RunWorkflow
        );
        Some(if model_runtime_operation {
            if self.adk_chat_stream.runtime_ready() {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            }
        } else {
            ProductionAdapterBinding::Ready
        })
    }

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
            "/api/v1/execution/buying-power" => {
                // ProductRuleProvider is implemented by the production
                // execution adapter, but it is only useful when the active
                // Futu trade session exposes the typed max-trade-quantity
                // reader.  Do not advertise this route from the generic
                // ExecutionWrite binding alone: that would make the local
                // parser look like a broker capability and could project a
                // synthetic `allowed: true` response while OpenD is absent.
                Some(if self.futu_trade_read_capability_ready() {
                    ProductionAdapterBinding::Ready
                } else {
                    ProductionAdapterBinding::ExternalUnavailable
                })
            }
            "/api/v1/execution/combos/previews" => {
                let snapshot = self.active_provider_state.snapshot();
                // `combo_preview` has two independent production paths:
                // option_combo validates option strategy legality and reads
                // combo buying power, while event_parlay validates active
                // prediction contracts and persists the quote without any
                // option readers.  A route-level binding cannot inspect the
                // request body, so Ready means at least one complete path is
                // installed; the handler keeps the other path's precise
                // 503/error mapping when its external dependency is absent.
                let futu_opend_ready =
                    snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                        && snapshot.opend_ready;
                let event_parlay_ready = futu_opend_ready
                    && self
                        .trade_runtime
                        .as_ref()
                        .is_some_and(|runtime| runtime.prediction_reader_available());
                let option_combo_ready = futu_opend_ready
                    && self.futu_trade_read_capability_ready()
                    && self.trade_runtime.as_ref().is_some_and(|runtime| {
                        // Strategy legality selects either the generic
                        // strategy reader or the spread reader depending on
                        // optionStrategy.  Analysis is invoked after the
                        // legality/max-quantity reads by the handler, so it
                        // remains part of a complete option path.
                        (runtime.option_strategy_available()
                            || runtime.option_strategy_spread_available())
                            && runtime.option_strategy_analysis_available()
                    });
                Some(if event_parlay_ready || option_combo_ready {
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
        // Prediction routes share one OpenD protocol family but have
        // operation-specific ports. Do not let the generic unavailable
        // matrix entry hide a reader that was actually installed, and do not
        // advertise combo/subscription writes when only the read adapter is
        // present.
        if matches!(
            adapter,
            ProductionRouteAdapter::MarketDataPredictionRead
                | ProductionRouteAdapter::MarketDataPredictionSubscriptionAcquireWrite
                | ProductionRouteAdapter::MarketDataPredictionSubscriptionReleaseWrite
                | ProductionRouteAdapter::MarketDataPredictionCombosWrite
        ) {
            let snapshot = self.active_provider_state.snapshot();
            let ready = snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                && snapshot.opend_ready
                && self.trade_runtime.as_ref().is_some_and(|runtime| match adapter {
                    ProductionRouteAdapter::MarketDataPredictionRead => {
                        runtime.prediction_reader_available()
                    }
                    ProductionRouteAdapter::MarketDataPredictionSubscriptionAcquireWrite
                    | ProductionRouteAdapter::MarketDataPredictionSubscriptionReleaseWrite => {
                        runtime.prediction_subscription_available()
                    }
                    ProductionRouteAdapter::MarketDataPredictionCombosWrite => {
                        runtime.prediction_combo_quote_available()
                    }
                    _ => false,
                });
            return Some(if ready {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            });
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
        if adapter == ProductionRouteAdapter::BacktestStart {
            // BacktestStart is backed by the verified PineTS execution worker
            // and the local historical-candle store.  It does not call the
            // live helper/OpenD/router on the request path; missing candles
            // remain a request-level BACKTESTS_WRITE_UNAVAILABLE response.
            // Keep only the provider-selection guard here so a corrupted or
            // unset active provider cannot produce a synthetic run.
            return Some(if self.backtest_execution_ready
                && self.active_provider_state.get().is_some()
            {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            });
        }
        if adapter == ProductionRouteAdapter::WebSocketLive {
            // The hub is constructed before the HTTP listener is exposed, so
            // both `Accepting` (composed, pre-exposure) and `Serving` are
            // route-ready.  Only shutdown states reject new websocket calls;
            // treating `Accepting` as unavailable would leave startup counts
            // stale after the listener is exposed.
            return Some(if self.ws_live.enabled() {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            });
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
        // Microstructure readers are installed on the shared trade runtime
        // even while the initial capability matrix is being built.  Keep the
        // route registered when OpenD is absent, but derive its live binding
        // from the concrete reader rather than the matrix's conservative
        // startup `ExternalUnavailable` entry.  This lets a connected Futu
        // session reach the real depth/ticks/queue/flow/profile handlers and
        // still maps a dead session to the normal 502/503 boundary there.
        if is_microstructure_adapter(adapter) {
            let snapshot = self.active_provider_state.snapshot();
            let ready = snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                && snapshot.opend_ready
                && self
                    .trade_runtime
                    .as_ref()
                    .is_some_and(|runtime| runtime.market_microstructure_available());
            return Some(if ready {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            });
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

    /// Return whether the concrete Futu/OpenD trade reader used by the
    /// production execution adapter is currently available.  The trait's
    /// `read_max_trade_quantity` method is the ProductRule capability; the
    /// route registry can only evaluate readiness without a request payload,
    /// so it checks the live typed reader/session handle rather than a static
    /// `ExecutionWrite` installation bit.
    fn futu_trade_read_capability_ready(&self) -> bool {
        let snapshot = self.active_provider_state.snapshot();
        if snapshot.provider != Some(jftrade_settings::MarketDataProvider::Futu)
            || !snapshot.opend_ready
        {
            return false;
        }
        if let Some(runtime) = self.trade_runtime.as_ref() {
            return runtime.snapshot().is_ready();
        }
        self.trade_logged_in == Some(true) && self.trade_read_port.is_some()
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
            | ResearchScreenWrite
            | MarketDataPredictionRead
            | MarketDataPredictionSubscriptionAcquireWrite
            | MarketDataPredictionSubscriptionReleaseWrite
            | MarketDataPredictionCombosWrite
            | BacktestSyncStart
            | MarketDataDepthRead
            | MarketDataTicksRead
            | MarketDataBrokerQueueRead
            | MarketDataCapitalFlowRead
            | MarketDataIntradayRead
            | MarketDataProfileRead
    )
}

fn is_microstructure_adapter(adapter: ProductionRouteAdapter) -> bool {
    matches!(
        adapter,
        ProductionRouteAdapter::MarketDataDepthRead
            | ProductionRouteAdapter::MarketDataTicksRead
            | ProductionRouteAdapter::MarketDataBrokerQueueRead
            | ProductionRouteAdapter::MarketDataCapitalFlowRead
            | ProductionRouteAdapter::MarketDataIntradayRead
            | ProductionRouteAdapter::MarketDataProfileRead
    )
}

#[path = "product_production_adapter_bindings_readiness.rs"]
mod readiness;

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
