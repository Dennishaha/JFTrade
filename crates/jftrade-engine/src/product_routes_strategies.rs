fn product_strategy_read_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.strategy_read || !capabilities.contains(ProductCapability::StrategyRead) {
        return Vec::new();
    }
    vec![
        route("GET", "/api/v1/strategies"),
        route("GET", "/api/v1/strategies/{instanceId}/logs"),
        route("GET", "/api/v1/strategies/{instanceId}/audit"),
    ]
}
