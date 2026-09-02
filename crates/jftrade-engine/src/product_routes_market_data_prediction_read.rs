fn product_market_data_prediction_read_routes(
    capabilities: &ProductCapabilities,
    ports: ProductRoutePorts,
) -> Vec<RouteSpec> {
    if !ports.market_data_prediction_read
        || !capabilities.contains(ProductCapability::MarketDataPredictionRead)
    {
        return Vec::new();
    }
    market_data_prediction_read_routes()
        .iter()
        .map(|(method, path)| route(method, path))
        .collect()
}
