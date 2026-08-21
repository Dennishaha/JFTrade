fn product_backtest_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.backtest_read || !capabilities.contains(ProductCapability::BacktestRead) {
        return Vec::new();
    }
    vec![
        route("GET", "/api/v1/backtests"),
        route("GET", "/api/v1/backtests/{runId}/status"),
        route("GET", "/api/v1/backtests/{runId}"),
    ]
}
