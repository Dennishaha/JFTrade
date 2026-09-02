impl ProductApi {
    fn market_data_news_search_read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<ApiOutput, ApiFailure> {
        let port = self
            .market_data_news_search_read_snapshot_port
            .as_ref()
            .ok_or_else(|| {
                ApiFailure::new(
                    503,
                    "MARKET_DATA_NEWS_SEARCH_READ_UNAVAILABLE",
                    "market-data news search snapshot is not configured",
                )
            })?;
        port.read(path, query)
            .map(ApiOutput::Json)
            .map_err(market_data_news_search_read_snapshot_failure)
    }
}

fn is_market_data_news_search_read_path(path: &str) -> bool {
    path == "/api/v1/market-data/news"
}

fn market_data_news_search_read_snapshot_failure(
    error: MarketDataNewsSearchReadSnapshotError,
) -> ApiFailure {
    match error {
        MarketDataNewsSearchReadSnapshotError::Unavailable(message) => ApiFailure::new(
            503,
            "MARKET_DATA_NEWS_SEARCH_READ_UNAVAILABLE",
            message,
        ),
        MarketDataNewsSearchReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => {
            let failure = ApiFailure::new(status, code, message);
            match retry_after_seconds {
                Some(seconds) => failure.with_retry_after(seconds),
                None => failure,
            }
        }
    }
}
