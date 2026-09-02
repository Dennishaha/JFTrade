impl ProductApi {
    async fn market_data_catalog_read(&self, path: &str, query: &str) -> Result<ApiOutput, ApiFailure> {
        let port = self
            .market_data_catalog_read_snapshot_port
            .as_ref()
            .ok_or_else(|| {
                ApiFailure::new(
                    503,
                    "MARKET_DATA_CATALOG_UNAVAILABLE",
                    "market-data catalog snapshot is not configured",
                )
            })?;
        port.read(path, query)
            .await
            .map(ApiOutput::Json)
            .map_err(market_data_catalog_read_snapshot_failure)
    }
}

fn is_market_data_catalog_read_path(path: &str) -> bool {
    matches!(
        path,
        "/api/v1/market-data/markets" | "/api/v1/market-data/instruments"
    )
}

fn market_data_catalog_read_snapshot_failure(
    error: MarketDataCatalogReadSnapshotError,
) -> ApiFailure {
    match error {
        MarketDataCatalogReadSnapshotError::Unavailable(message) => ApiFailure::new(
            503,
            "MARKET_DATA_CATALOG_UNAVAILABLE",
            message,
        ),
        MarketDataCatalogReadSnapshotError::Invalid { code, message } => {
            ApiFailure::new(400, code, message)
        }
        MarketDataCatalogReadSnapshotError::Failed {
            status,
            code,
            message,
        } => ApiFailure::new(status, code, message),
    }
}
