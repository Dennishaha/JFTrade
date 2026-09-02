impl ProductApi {
    fn market_data_provider_read(&self, path: &str, query: &str) -> Result<ApiOutput, ApiFailure> {
        let port = self
            .market_data_provider_read_snapshot_port
            .as_ref()
            .ok_or_else(|| {
                ApiFailure::new(
                    503,
                    "MARKET_DATA_PROVIDER_UNAVAILABLE",
                    "market-data provider snapshot is not configured",
                )
            })?;
        port.read(path, query)
            .map(ApiOutput::Json)
            .map_err(market_data_provider_read_snapshot_failure)
    }
}

fn is_market_data_provider_read_path(path: &str) -> bool {
    path == "/api/v1/market-data/provider"
}

fn market_data_provider_read_snapshot_failure(
    error: MarketDataProviderReadSnapshotError,
) -> ApiFailure {
    match error {
        MarketDataProviderReadSnapshotError::Unavailable(message) => ApiFailure::new(
            503,
            "MARKET_DATA_PROVIDER_UNAVAILABLE",
            message,
        ),
        MarketDataProviderReadSnapshotError::Failed { code, message } => {
            ApiFailure::new(502, code, message)
        }
    }
}
