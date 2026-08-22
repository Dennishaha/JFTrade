impl ProductApi {
    fn market_data_options_read(&self, path: &str, query: &str) -> Result<ApiOutput, ApiFailure> {
        let port = self
            .market_data_options_read_snapshot_port
            .as_ref()
            .ok_or_else(|| {
                ApiFailure::new(
                    503,
                    "MARKET_DATA_OPTIONS_UNAVAILABLE",
                    "market-data options snapshot is not configured",
                )
            })?;
        port.read(path, query)
            .map(ApiOutput::Json)
            .map_err(market_data_options_read_snapshot_failure)
    }
}

fn is_market_data_options_read_path(path: &str) -> bool {
    if matches!(
        path,
        "/api/v1/market-data/options/screens" | "/api/v1/market-data/options/events"
    ) {
        return true;
    }
    [
        "/api/v1/market-data/options/chains/",
        "/api/v1/market-data/options/expirations/",
        "/api/v1/market-data/options/analysis/",
    ]
    .iter()
    .any(|prefix| {
        path.strip_prefix(prefix)
            .is_some_and(|suffix| !suffix.is_empty() && !suffix.contains('/'))
    })
}

fn market_data_options_read_snapshot_failure(
    error: MarketDataOptionsReadSnapshotError,
) -> ApiFailure {
    match error {
        MarketDataOptionsReadSnapshotError::Unavailable(message) => ApiFailure::new(
            503,
            "MARKET_DATA_OPTIONS_UNAVAILABLE",
            message,
        ),
        MarketDataOptionsReadSnapshotError::Failed {
            status,
            code,
            message,
        } => ApiFailure::new(status, code, message),
    }
}
