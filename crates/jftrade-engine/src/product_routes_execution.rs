fn product_execution_read_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.execution_read || !capabilities.contains(ProductCapability::ExecutionRead) {
        return Vec::new();
    }
    vec![
        route("GET", "/api/v1/execution/orders"),
        route("GET", "/api/v1/execution/orders/{internalOrderId}"),
        route("GET", "/api/v1/execution/orders/{internalOrderId}/events"),
    ]
}
