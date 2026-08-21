fn product_watchlist_research_trading_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    let mut routes = vec![
        route("GET", "/api/v1/adk/agent-templates"),
        route("GET", "/api/v1/research/screens/catalog"),
    ];
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
