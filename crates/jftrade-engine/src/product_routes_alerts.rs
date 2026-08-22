fn product_alert_routes(include_reads: bool, include_writes: bool) -> Vec<RouteSpec> {
    let mut routes = if include_reads {
        [
            "/api/v1/alerts/option-events",
            "/api/v1/alerts/price",
        ]
        .into_iter()
        .map(|path| route("GET", path))
        .collect::<Vec<_>>()
    } else {
        Vec::new()
    };
    if include_writes {
        routes.extend([
            route("POST", "/api/v1/alerts/option-events"),
            route("POST", "/api/v1/alerts/price"),
        ]);
    }
    routes
}
