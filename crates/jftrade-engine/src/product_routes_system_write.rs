fn product_system_write_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.system_write || !capabilities.contains(ProductCapability::SystemWrite) {
        return Vec::new();
    }
    system_write_routes()
        .iter()
        .map(|(method, path)| route(method, path))
        .collect()
}
