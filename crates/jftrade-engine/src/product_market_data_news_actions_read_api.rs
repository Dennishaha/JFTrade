impl ProductApi {
    fn market_data_news_actions_read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<ApiOutput, ApiFailure> {
        let port = self
            .market_data_news_actions_read_snapshot_port
            .as_ref()
            .ok_or_else(|| {
                ApiFailure::new(
                    503,
                    "MARKET_DATA_NEWS_ACTIONS_UNAVAILABLE",
                    "market-data news/actions snapshot is not configured",
                )
            })?;
        port.read(path, query)
            .map(ApiOutput::Json)
            .map_err(market_data_news_actions_read_snapshot_failure)
    }
}

fn is_market_data_news_actions_read_path(path: &str) -> bool {
    [
        "/api/v1/market-data/news/",
        "/api/v1/market-data/corporate-actions/",
    ]
    .iter()
    .any(|prefix| {
        path.strip_prefix(prefix).is_some_and(|suffix| {
            let mut segments = suffix.split('/');
            segments.next().is_some_and(|segment| !segment.is_empty())
                && segments.next().is_some_and(|segment| !segment.is_empty())
                && segments.next().is_none()
        })
    })
}

fn market_data_news_actions_read_snapshot_failure(
    error: MarketDataNewsActionsReadSnapshotError,
) -> ApiFailure {
    match error {
        MarketDataNewsActionsReadSnapshotError::Unavailable(message) => {
            ApiFailure::new(503, "MARKET_DATA_NEWS_ACTIONS_UNAVAILABLE", message)
        }
        MarketDataNewsActionsReadSnapshotError::Failed {
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
