//! Truthful MCP readiness and production-adapter mappings.

use crate::product::product_production_ports::{
    ProductionAdapterBinding, ProductionPortBundle, ProductionToolCatalog,
};
use crate::product::product_production_route_registry::ProductionRouteAdapter;

use super::PRODUCTION_MCP_EXECUTABLE_TOOLS;

/// Return the truthful readiness projection for one reviewed MCP tool.
/// Unimplemented Go names remain explicitly fail-closed even when their
/// broader production adapter happens to be installed.
pub(crate) fn mcp_tool_availability(
    catalog: &ProductionToolCatalog,
    ports: Option<&ProductionPortBundle>,
    name: &str,
) -> &'static str {
    if !PRODUCTION_MCP_EXECUTABLE_TOOLS.contains(&name) {
        return "fail-closed";
    }
    let binding = match ports {
        Some(ports) if name == "execution.buying_power" => {
            ports.execution_operation_binding("/api/v1/execution/buying-power")
        }
        Some(ports) => mcp_port_binding(ports, name),
        None => catalog.binding_for_mcp_tool(name),
    };
    match binding {
        Some(ProductionAdapterBinding::Ready) => "ready",
        Some(ProductionAdapterBinding::ExternalUnavailable) => "unavailable",
        // A missing internal adapter is a composition bug, not an external
        // outage; keep the descriptor fail-closed until startup is repaired.
        Some(ProductionAdapterBinding::MissingInternalAdapter) | None => "fail-closed",
    }
}

/// MCP research names share a small number of production route adapters, but
/// their readiness is not interchangeable. Keep operation-level research
/// checks here so a healthy helper cannot make an unsupported market feed (or
/// Pine/technical tool) appear callable.
fn mcp_port_binding(ports: &ProductionPortBundle, name: &str) -> Option<ProductionAdapterBinding> {
    match name {
        "alerts.price.list" | "alerts.option_event.list" => {
            ports.adapter_binding(ProductionRouteAdapter::AlertsRead)
        }
        "research.screen" => ports.adapter_binding(ProductionRouteAdapter::ResearchScreenWrite),
        "research.screen_catalog" => ports.adapter_binding(ProductionRouteAdapter::ResearchCatalog),
        "research.news" => ports.adapter_binding(ProductionRouteAdapter::MarketDataNewsSearchRead),
        "research.instrument" => {
            ports.research_operation_binding("/api/v1/research/instruments/US.MCP_READINESS")
        }
        "research.financials" => {
            ports.research_operation_binding("/api/v1/research/financials/US.MCP_READINESS")
        }
        "research.analyst" => {
            ports.research_operation_binding("/api/v1/research/analyst/US.MCP_READINESS")
        }
        "research.ownership" => {
            ports.research_operation_binding("/api/v1/research/ownership/US.MCP_READINESS")
        }
        "research.corporate_actions" => {
            ports.research_operation_binding("/api/v1/research/corporate-actions/US.MCP_READINESS")
        }
        "research.valuation" => {
            ports.research_operation_binding("/api/v1/research/valuation/US.MCP_READINESS")
        }
        "research.rankings" => ports.adapter_binding(ProductionRouteAdapter::ResearchRankingsRead),
        "research.industry" => {
            ports.adapter_binding(ProductionRouteAdapter::ResearchIndustriesRead)
        }
        "research.calendar" => ports.adapter_binding(ProductionRouteAdapter::ResearchCalendarRead),
        "research.macro" => ports.adapter_binding(ProductionRouteAdapter::ResearchMacroRead),
        _ => mcp_tool_adapter(name).and_then(|adapter| ports.adapter_binding(adapter)),
    }
}

pub(crate) fn mcp_tool_adapter(name: &str) -> Option<ProductionRouteAdapter> {
    Some(match name {
        "system.status" => ProductionRouteAdapter::SystemCore,
        "system.futu_opend" | "system.runtime_dependencies" => ProductionRouteAdapter::SystemRead,
        "plugins.catalog" => ProductionRouteAdapter::PluginsRead,
        "market.providers" | "market.capabilities" => {
            ProductionRouteAdapter::MarketDataProviderRead
        }
        "market.search" => ProductionRouteAdapter::MarketDataSearchRead,
        "market.instrument_profile" => ProductionRouteAdapter::MarketDataProfileRead,
        "market.snapshot" => ProductionRouteAdapter::MarketDataSnapshotsRead,
        "market.candles" => ProductionRouteAdapter::MarketDataCandlesRead,
        "market.intraday" => ProductionRouteAdapter::MarketDataIntradayRead,
        "market.ticks" => ProductionRouteAdapter::MarketDataTicksRead,
        "market.depth" => ProductionRouteAdapter::MarketDataDepthRead,
        "market.broker_queue" => ProductionRouteAdapter::MarketDataBrokerQueueRead,
        "market.capital_flow" => ProductionRouteAdapter::MarketDataCapitalFlowRead,
        "market.snapshots" => ProductionRouteAdapter::MarketDataBatchSnapshotsWrite,
        "market.subscriptions" => ProductionRouteAdapter::MarketDataSubscriptionRead,
        "derivatives.warrants" => ProductionRouteAdapter::MarketDataDerivativeRead,
        "derivatives.futures" => ProductionRouteAdapter::MarketDataFuturesRead,
        "derivatives.option_chain" => ProductionRouteAdapter::MarketDataOptionsChainRead,
        "derivatives.option_screen" => ProductionRouteAdapter::MarketDataOptionsScreenRead,
        "derivatives.option_analysis" => ProductionRouteAdapter::MarketDataOptionsAnalysisRead,
        "derivatives.option_events" => ProductionRouteAdapter::MarketDataOptionsEventsRead,
        "prediction.discover"
        | "prediction.snapshot"
        | "prediction.depth"
        | "prediction.history"
        | "prediction.combo_eligible" => ProductionRouteAdapter::MarketDataPredictionRead,
        "prediction.combo_quote" => ProductionRouteAdapter::MarketDataPredictionCombosWrite,
        "broker.cash_flows" | "broker.fees" | "broker.margin_ratios" => {
            ProductionRouteAdapter::BrokerRead
        }
        "execution.order_events" => ProductionRouteAdapter::ExecutionRead,
        "execution.buying_power" => ProductionRouteAdapter::ExecutionWrite,
        "watchlist.list" => ProductionRouteAdapter::WatchlistRead,
        "watchlist.remote.list" => ProductionRouteAdapter::RemoteWatchlistRead,
        "portfolio.summary" => ProductionRouteAdapter::PortfolioRead,
        "account.orders" => ProductionRouteAdapter::ExecutionRead,
        "broker.orders" | "broker.fills" => ProductionRouteAdapter::BrokerRead,
        "strategy.definitions"
        | "strategy.definition_versions.list"
        | "strategy.definition_versions.get" => ProductionRouteAdapter::StrategyDefinitionRead,
        "strategy.instance_activity" => ProductionRouteAdapter::StrategyRuntimeRead,
        "backtest.runs" | "backtest.result_view" => ProductionRouteAdapter::BacktestRead,
        "backtest.kline_sync_status" => ProductionRouteAdapter::BacktestSyncRead,
        "risk.state" | "risk.events" => ProductionRouteAdapter::SystemRead,
        "alerts.price.list" | "alerts.option_event.list" => ProductionRouteAdapter::AlertsRead,
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
        _ => return None,
    })
}
