fn product_execution_write_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.execution_write || !capabilities.contains(ProductCapability::ExecutionWrite) {
        return Vec::new();
    }
    execution_write_routes()
        .iter()
        .map(|(method, path)| route(method, path))
        .collect()
}
