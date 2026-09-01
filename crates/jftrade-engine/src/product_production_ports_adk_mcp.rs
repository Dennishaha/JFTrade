//! MCP-specific readiness aliases for the production ADK catalog.

use super::{
    PRODUCTION_TOOL_DEFINITIONS, ProductionAdapterBinding, ProductionRouteAdapter,
    ProductionToolCatalog,
};

impl ProductionToolCatalog {
    /// Resolve readiness for the native MCP surface. Most MCP names map
    /// directly to an ADK descriptor, while compatibility aliases use the
    /// same startup binding map so `tools/list` and `tools/call` cannot drift.
    pub(crate) fn binding_for_mcp_tool(&self, name: &str) -> Option<ProductionAdapterBinding> {
        let snapshot = self
            .active_provider_state
            .as_ref()
            .map(|state| state.snapshot());
        if let Some(definition) = PRODUCTION_TOOL_DEFINITIONS
            .iter()
            .find(|definition| definition.id == name)
        {
            return Some(snapshot.as_ref().map_or_else(
                || {
                    self.bindings
                        .get(&definition.adapter)
                        .copied()
                        .unwrap_or(ProductionAdapterBinding::ExternalUnavailable)
                },
                |snapshot| self.binding_for(definition, snapshot),
            ));
        }
        let adapter = match name {
            "plugins.catalog" => ProductionRouteAdapter::PluginsRead,
            "market.providers" | "market.capabilities" => {
                ProductionRouteAdapter::MarketDataProviderRead
            }
            "market.instrument_profile" => ProductionRouteAdapter::MarketDataProfileRead,
            "market.intraday" => ProductionRouteAdapter::MarketDataIntradayRead,
            "market.ticks" => ProductionRouteAdapter::MarketDataTicksRead,
            "market.depth" => ProductionRouteAdapter::MarketDataDepthRead,
            "market.broker_queue" => ProductionRouteAdapter::MarketDataBrokerQueueRead,
            "market.capital_flow" => ProductionRouteAdapter::MarketDataCapitalFlowRead,
            "derivatives.warrants" => ProductionRouteAdapter::MarketDataDerivativeRead,
            "derivatives.futures" => ProductionRouteAdapter::MarketDataFuturesRead,
            "derivatives.option_chain" => ProductionRouteAdapter::MarketDataOptionsChainRead,
            "derivatives.option_screen" => ProductionRouteAdapter::MarketDataOptionsScreenRead,
            "derivatives.option_analysis" => {
                ProductionRouteAdapter::MarketDataOptionsAnalysisRead
            }
            "derivatives.option_events" => ProductionRouteAdapter::MarketDataOptionsEventsRead,
            "prediction.discover"
            | "prediction.snapshot"
            | "prediction.depth"
            | "prediction.history"
            | "prediction.combo_eligible" => ProductionRouteAdapter::MarketDataPredictionRead,
            "prediction.combo_quote" => ProductionRouteAdapter::MarketDataPredictionCombosWrite,
            "system.runtime_dependencies" => ProductionRouteAdapter::SystemRead,
            "watchlist.remote.list" => ProductionRouteAdapter::RemoteWatchlistRead,
            "portfolio.summary" => ProductionRouteAdapter::PortfolioRead,
            "account.orders" => ProductionRouteAdapter::ExecutionRead,
            "broker.orders" | "broker.fills" => ProductionRouteAdapter::BrokerRead,
            "broker.cash_flows" | "broker.fees" | "broker.margin_ratios" => {
                ProductionRouteAdapter::BrokerRead
            }
            "execution.order_events" => ProductionRouteAdapter::ExecutionRead,
            "execution.buying_power" => ProductionRouteAdapter::ExecutionWrite,
            "alerts.price.list" | "alerts.option_event.list" => {
                ProductionRouteAdapter::AlertsRead
            }
            "research.instrument"
            | "research.financials"
            | "research.analyst"
            | "research.ownership"
            | "research.corporate_actions"
            | "research.valuation"
            | "research.rankings"
            | "research.industry"
            | "research.calendar"
            | "research.macro" => ProductionRouteAdapter::ResearchRead,
            "research.news" => ProductionRouteAdapter::MarketDataNewsSearchRead,
            "research.screen" => ProductionRouteAdapter::ResearchScreenWrite,
            "research.screen_catalog" => ProductionRouteAdapter::ResearchCatalog,
            "strategy.definition_versions.list" | "strategy.definition_versions.get" => {
                ProductionRouteAdapter::StrategyDefinitionRead
            }
            "strategy.instance_activity" => ProductionRouteAdapter::StrategyRuntimeRead,
            "risk.state" | "risk.events" => ProductionRouteAdapter::SystemRead,
            _ => return None,
        };
        let startup_binding = self
            .bindings
            .get(&adapter)
            .copied()
            .unwrap_or(ProductionAdapterBinding::ExternalUnavailable);
        if adapter != ProductionRouteAdapter::PortfolioRead {
            return Some(startup_binding);
        }
        let Some(snapshot) = snapshot else {
            return Some(startup_binding);
        };
        let trade_ready = self
            .trade_runtime
            .as_ref()
            .is_some_and(|runtime| runtime.snapshot().is_ready());
        Some(
            if snapshot.provider == Some(jftrade_settings::MarketDataProvider::Futu)
                && snapshot.opend_ready
                && trade_ready
            {
                ProductionAdapterBinding::Ready
            } else {
                ProductionAdapterBinding::ExternalUnavailable
            },
        )
    }
}
