fn product_market_data_news_actions_read_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.market_data_news_actions_read
        || !capabilities.contains(ProductCapability::MarketDataNewsActionsRead)
    {
        return Vec::new();
    }
    vec![
        route("GET", "/api/v1/market-data/news/{market}/{symbol}"),
        route(
            "GET",
            "/api/v1/market-data/corporate-actions/{market}/{symbol}",
        ),
    ]
}
