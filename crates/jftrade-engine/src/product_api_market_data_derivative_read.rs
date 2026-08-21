impl ProductApi {
    fn market_data_derivative_read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<ApiOutput, ApiFailure> {
        let port = self
            .market_data_derivative_read_snapshot_port
            .as_ref()
            .ok_or_else(|| {
                ApiFailure::new(
                    503,
                    "MARKET_DATA_DERIVATIVE_UNAVAILABLE",
                    "market-data derivative snapshot is not configured",
                )
            })?;
        port.read(path, query)
            .map(ApiOutput::Json)
            .map_err(market_data_derivative_read_snapshot_failure)
    }
}

fn is_market_data_derivative_read_path(path: &str) -> bool {
    matches!(
        path,
        "/api/v1/market-data/warrants" | "/api/v1/market-data/futures"
    )
}

fn market_data_derivative_read_snapshot_failure(
    error: MarketDataDerivativeReadSnapshotError,
) -> ApiFailure {
    match error {
        MarketDataDerivativeReadSnapshotError::Unavailable(message) => ApiFailure::new(
            503,
            "MARKET_DATA_DERIVATIVE_UNAVAILABLE",
            message,
        ),
        MarketDataDerivativeReadSnapshotError::Failed {
            status,
            code,
            message,
        } => ApiFailure::new(status, code, message),
    }
}
