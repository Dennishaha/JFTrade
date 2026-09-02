fn product_market_data_derivative_read_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.market_data_derivative_read
        || !capabilities.contains(ProductCapability::MarketDataDerivativeRead)
    {
        return Vec::new();
    }
    vec![
        route("GET", "/api/v1/market-data/warrants"),
        route("GET", "/api/v1/market-data/futures"),
    ]
}
