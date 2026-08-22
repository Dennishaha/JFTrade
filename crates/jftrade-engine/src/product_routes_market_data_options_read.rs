fn product_market_data_options_read_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.market_data_options_read
        || !capabilities.contains(ProductCapability::MarketDataOptionsRead)
    {
        return Vec::new();
    }
    vec![
        route("GET", "/api/v1/market-data/options/chains/{instrumentId}"),
        route("GET", "/api/v1/market-data/options/expirations/{instrumentId}"),
        route("GET", "/api/v1/market-data/options/screens"),
        route("GET", "/api/v1/market-data/options/analysis/{instrumentId}"),
        route("GET", "/api/v1/market-data/options/events"),
    ]
}
