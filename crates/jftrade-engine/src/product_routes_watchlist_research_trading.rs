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
