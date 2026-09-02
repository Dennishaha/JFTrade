fn route(method: &str, path: &str) -> RouteSpec {
    RouteSpec {
        method: method.into(),
        path: path.into(),
    }
}

fn product_system_routes(capabilities: &ProductCapabilities, ports: ProductRoutePorts) -> Vec<RouteSpec> {
    let mut routes = [
        "/api/v1/system/status",
        "/api/v1/system/runtime-dependencies",
        "/api/v1/system/futu-opend/install-guide",
        "/api/v1/system/storage/overview",
        "/api/v1/system/real-trade-approvals",
        "/api/v1/system/real-trade-hard-stops",
        "/api/v1/system/real-trade-hard-stop-events",
        "/api/v1/system/real-trade-kill-switch",
        "/api/v1/system/real-trade-kill-switch-events",
        "/api/v1/system/real-trade-risk-limits",
        "/api/v1/system/real-trade-risk-events",
    ]
    .into_iter()
    .map(|path| route("GET", path))
    .collect::<Vec<_>>();
    if ports.system_read && capabilities.contains(ProductCapability::SystemRead) {
        routes.extend([
            route("GET", "/api/v1/system/futu-opend"),
            route("GET", "/api/v1/system/worker/broker-order-updates"),
        ]);
    }
    routes
}
