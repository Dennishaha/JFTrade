fn product_strategy_runtime_write_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.strategy_runtime_write
        || !capabilities.contains(ProductCapability::StrategyRuntimeWrite)
    {
        return Vec::new();
    }
    strategy_runtime_write_routes()
        .iter()
        .map(|(method, path)| route(method, path))
        .collect()
}
