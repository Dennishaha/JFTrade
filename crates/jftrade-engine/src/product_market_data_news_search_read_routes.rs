fn product_market_data_news_search_read_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.market_data_news_search_read
        || !capabilities.contains(ProductCapability::MarketDataNewsSearchRead)
    {
        return Vec::new();
    }
    vec![route("GET", "/api/v1/market-data/news")]
}
