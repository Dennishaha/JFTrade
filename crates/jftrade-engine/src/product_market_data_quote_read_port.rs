pub type MarketDataQuoteReadFuture<'a> =
    Pin<Box<dyn Future<Output = Result<serde_json::Value, MarketDataQuoteReadSnapshotError>> + Send + 'a>>;

/// Consumer-owned quote projections for subscriptions, securities, snapshots, candles, and depth.
pub trait MarketDataQuoteReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read<'a>(
        &'a self,
        path: &'a str,
        query: &'a str,
    ) -> MarketDataQuoteReadFuture<'a>;
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
