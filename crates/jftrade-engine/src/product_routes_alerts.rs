fn product_alert_routes() -> Vec<RouteSpec> {
    [
        "/api/v1/alerts/option-events",
        "/api/v1/alerts/price",
    ]
    .into_iter()
    .map(|path| route("GET", path))
    .collect()
}
