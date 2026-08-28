impl ProductApi {
    async fn market_data_quote_read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<ApiOutput, ApiFailure> {
        let port = self
            .market_data_quote_read_snapshot_port
            .as_ref()
            .ok_or_else(|| {
                ApiFailure::new(
                    503,
                    "MARKET_DATA_QUOTE_READ_UNAVAILABLE",
                    "market-data quote-read snapshot is not configured",
                )
            })?;
        port.read(path, query)
            .await
            .map(ApiOutput::Json)
            .map_err(market_data_quote_read_snapshot_failure)
    }
}

fn is_market_data_quote_read_path(path: &str) -> bool {
    if path == "/api/v1/market-data/subscriptions" {
        return true;
    }
    [
        "/api/v1/market-data/broker-queue/",
        "/api/v1/market-data/capital-flow/",
        "/api/v1/market-data/intraday/",
        "/api/v1/market-data/ticks/",
    ]
    .iter()
    .any(|prefix| has_one_segment_suffix(path, prefix))
        || [
            "/api/v1/market-data/candles/",
            "/api/v1/market-data/depth/",
            "/api/v1/market-data/securities/",
            "/api/v1/market-data/snapshots/",
        ]
        .iter()
        .any(|prefix| has_two_segment_suffix(path, prefix))
        || has_instrument_profile_suffix(path)
}

fn has_one_segment_suffix(path: &str, prefix: &str) -> bool {
    path.strip_prefix(prefix)
        .is_some_and(|suffix| !suffix.is_empty() && !suffix.contains('/'))
}

fn has_two_segment_suffix(path: &str, prefix: &str) -> bool {
    path.strip_prefix(prefix).is_some_and(|suffix| {
        let mut segments = suffix.split('/');
        segments.next().is_some_and(|segment| !segment.is_empty())
            && segments.next().is_some_and(|segment| !segment.is_empty())
            && segments.next().is_none()
    })
}

fn has_instrument_profile_suffix(path: &str) -> bool {
    has_two_segment_suffix(path, "/api/v1/market-data/instruments/")
        && path.ends_with("/profile")
}

fn market_data_quote_read_snapshot_failure(
    error: MarketDataQuoteReadSnapshotError,
) -> ApiFailure {
    match error {
        MarketDataQuoteReadSnapshotError::Unavailable(message) => ApiFailure::new(
            503,
            "MARKET_DATA_QUOTE_READ_UNAVAILABLE",
            message,
        ),
        MarketDataQuoteReadSnapshotError::Failed {
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
