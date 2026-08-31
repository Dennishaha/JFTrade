//! Route-specific readiness predicates kept separate from the binding table.

use super::{ProductionAdapterBinding, ProductionPortBundle, ProductionRouteAdapter};

impl ProductionPortBundle {
    /// Resolve readiness for one `operation=` value on the shared options
    /// analysis route. The route remains registered when at least one
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

    /// Resolve readiness for an individual research operation. The public
    /// research surface is intentionally one `ResearchRead` adapter, but its
    /// implementations are not uniform: helper-backed instrument routes and
    /// the Futu valuation reader have independent prerequisites while the
    /// remaining baseline routes are deliberately unavailable.
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
        if self
            .bound_adapters
            .get(&ProductionRouteAdapter::ResearchRead)
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
