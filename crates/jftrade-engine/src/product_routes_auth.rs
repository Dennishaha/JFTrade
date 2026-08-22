fn product_auth_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if ports.auth_session && capabilities.contains(ProductCapability::AuthSession) {
        vec![route("GET", "/api/v1/auth/session")]
    } else {
        Vec::new()
    }
}
