fn product_adk_mutation_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.adk_mutation || !capabilities.contains(ProductCapability::AdkMutations) {
        return Vec::new();
    }
    adk_mutation_routes()
        .iter()
        .map(|(method, path)| route(method, path))
        .collect()
}
