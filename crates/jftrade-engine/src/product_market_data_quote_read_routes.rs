fn product_market_data_quote_read_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.market_data_quote_read
        || !capabilities.contains(ProductCapability::MarketDataQuoteRead)
    {
        return Vec::new();
    }
    vec![
        route("GET", "/api/v1/market-data/broker-queue/{instrumentId}"),
        route("GET", "/api/v1/market-data/candles/{market}/{symbol}"),
        route("GET", "/api/v1/market-data/capital-flow/{instrumentId}"),
        route("GET", "/api/v1/market-data/depth/{market}/{symbol}"),
        route("GET", "/api/v1/market-data/instruments/{instrumentId}/profile"),
        route("GET", "/api/v1/market-data/intraday/{instrumentId}"),
        route("GET", "/api/v1/market-data/securities/{market}/{symbol}"),
        route("GET", "/api/v1/market-data/snapshots/{market}/{symbol}"),
        route("GET", "/api/v1/market-data/subscriptions"),
        route("GET", "/api/v1/market-data/ticks/{instrumentId}"),
    ]
}
