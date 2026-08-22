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

fn product_backtest_sync_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.backtest_sync_read || !capabilities.contains(ProductCapability::BacktestSyncRead) {
        return Vec::new();
    }
    vec![route("GET", "/api/v1/backtests/sync/{taskId}")]
}

fn product_backtests_write_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.backtests_write || !capabilities.contains(ProductCapability::BacktestsWrite) {
        return Vec::new();
    }
    backtests_write_routes()
        .iter()
        .map(|(method, path)| route(method, path))
        .collect()
}
