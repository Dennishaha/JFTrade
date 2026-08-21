fn product_market_data_catalog_read_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.market_data_catalog_read
        || !capabilities.contains(ProductCapability::MarketDataCatalogRead)
    {
        return Vec::new();
    }
    vec![
        route("GET", "/api/v1/market-data/markets"),
        route("GET", "/api/v1/market-data/instruments"),
    ]
}
