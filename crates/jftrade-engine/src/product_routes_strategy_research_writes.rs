fn product_strategy_research_write_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    let mut routes = Vec::new();
    if ports.research_preset_write && capabilities.contains(ProductCapability::ResearchPresetWrite)
    {
        routes.extend(
            research_preset_write_routes()
                .iter()
                .map(|(method, path)| route(method, path)),
        );
    }
    if ports.strategy_definition_write
        && capabilities.contains(ProductCapability::StrategyDefinitionWrite)
    {
        routes.extend(
            strategy_definition_write_routes()
                .iter()
                .map(|(method, path)| route(method, path)),
        );
    }
    routes
}
