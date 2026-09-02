fn product_market_data_provider_read_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.market_data_provider_read
        || !capabilities.contains(ProductCapability::MarketDataProviderRead)
    {
        return Vec::new();
    }
    vec![route("GET", "/api/v1/market-data/provider")]
}
