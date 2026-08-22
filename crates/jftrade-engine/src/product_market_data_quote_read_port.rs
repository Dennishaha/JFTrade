/// Consumer-owned quote projections. Go remains authoritative for Provider/
/// OpenD lifecycle, cache freshness, subscription demand and query semantics.
pub trait MarketDataQuoteReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<serde_json::Value, MarketDataQuoteReadSnapshotError>;
}

#[derive(Clone, Debug, Error)]
pub enum MarketDataQuoteReadSnapshotError {
    #[error("market-data quote-read snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("market-data quote-read snapshot failed: {code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
        retry_after_seconds: Option<u64>,
    },
}
