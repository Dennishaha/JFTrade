fn product_watchlist_research_trading_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    let mut routes = vec![
        route("GET", "/api/v1/adk/agent-templates"),
        route("GET", "/api/v1/research/screens/catalog"),
    ];
    if ports.strategy_definitions && capabilities.contains(ProductCapability::StrategyDefinitions) {
        routes.extend([
            route("GET", "/api/v1/strategy-definitions"),
            route("GET", "/api/v1/strategy-definitions/{definitionId}"),
            route(
                "GET",
                "/api/v1/strategy-definitions/{definitionId}/versions",
            ),
            route(
                "GET",
                "/api/v1/strategy-definitions/{definitionId}/versions/{version}",
            ),
        ]);
    }
    if ports.watchlist_memberships
        && capabilities.contains(ProductCapability::WatchlistMemberships)
    {
        routes.push(route(
            "GET",
            "/api/v1/watchlist/instruments/{market}/{symbol}/memberships",
        ));
    }
    if ports.watchlist_read && capabilities.contains(ProductCapability::WatchlistRead) {
        routes.extend([
            route("GET", "/api/v1/watchlist/groups"),
            route("GET", "/api/v1/watchlist/items"),
            route("GET", "/api/v1/watchlist/sources"),
            route("GET", "/api/v1/watchlist/sources/{sourceId}/groups"),
            route("GET", "/api/v1/watchlist/bindings"),
            route("GET", "/api/v1/watchlist/import-runs"),
        ]);
    }
    if ports.portfolio && capabilities.contains(ProductCapability::Portfolio) {
        routes.extend([
            route("GET", "/api/v1/portfolio/{brokerId}/cash-balances"),
            route("GET", "/api/v1/portfolio/{brokerId}/positions"),
        ]);
    }
    if ports.research_read && capabilities.contains(ProductCapability::ResearchRead) {
        routes.extend([
            route("GET", "/api/v1/research/instruments/{instrumentId}"),
            route("GET", "/api/v1/research/financials/{instrumentId}"),
            route("GET", "/api/v1/research/valuation/{instrumentId}"),
            route("GET", "/api/v1/research/analyst/{instrumentId}"),
            route("GET", "/api/v1/research/ownership/{instrumentId}"),
            route("GET", "/api/v1/research/corporate-actions/{instrumentId}"),
            route("GET", "/api/v1/research/short-interest/{instrumentId}"),
            route("GET", "/api/v1/research/technical-indicators/{instrumentId}"),
            route("GET", "/api/v1/research/screens"),
            route("GET", "/api/v1/research/calendars"),
            route("GET", "/api/v1/research/macro"),
            route("GET", "/api/v1/research/rankings"),
            route("GET", "/api/v1/research/institutions"),
            route("GET", "/api/v1/research/industries"),
        ]);
    }
    if ports.broker_read && capabilities.contains(ProductCapability::BrokerRead) {
        routes.extend([
            route("GET", "/api/v1/brokers/capabilities"),
            route("GET", "/api/v1/brokers/{brokerId}/runtime"),
            route("GET", "/api/v1/brokers/{brokerId}/funds"),
            route("GET", "/api/v1/brokers/{brokerId}/positions"),
            route("GET", "/api/v1/brokers/{brokerId}/orders"),
            route("GET", "/api/v1/brokers/{brokerId}/fills"),
            route("GET", "/api/v1/brokers/{brokerId}/cash-flows"),
            route("GET", "/api/v1/brokers/{brokerId}/order-fees"),
            route("GET", "/api/v1/brokers/{brokerId}/margin-ratios"),
            route("GET", "/api/v1/brokers/{brokerId}/max-trade-qtys"),
            route("GET", "/api/v1/brokers/{brokerId}/quote"),
            route("GET", "/api/v1/brokers/{brokerId}/klines"),
            route("GET", "/api/v1/brokers/{brokerId}/securities"),
        ]);
    }
    if ports.remote_watchlist && capabilities.contains(ProductCapability::RemoteWatchlistRead) {
        routes.push(route("GET", "/api/v1/watchlists/remote"));
    }
    if ports.plugins && capabilities.contains(ProductCapability::Plugins) {
        routes.extend([
            route("GET", "/api/v1/plugins"),
            route("GET", "/api/v1/plugins/operations/{operationId}"),
        ]);
    }
    if ports.plugin_uninstall_guidance
        && capabilities.contains(ProductCapability::PluginUninstallGuidance)
    {
        routes.push(route(
            "GET",
            "/api/v1/plugins/{pluginId}/uninstall-guidance",
        ));
    }
    routes
}
