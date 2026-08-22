fn product_auth_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    let mut routes = Vec::new();
    if ports.auth_session && capabilities.contains(ProductCapability::AuthSession) {
        routes.push(route("GET", "/api/v1/auth/session"));
    }
    if ports.auth_session_write && capabilities.contains(ProductCapability::AuthSessionWrite) {
        routes.extend(
            auth_session_write_routes()
                .iter()
                .map(|(method, path)| route(method, path)),
        );
    }
    routes
}
