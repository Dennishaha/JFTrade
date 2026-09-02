use std::fmt::Debug;

/// Consumer-owned projection for the product-feature news search route.
///
/// Go remains responsible for provider selection, broker fallback, query
/// normalization, pagination, and all Provider/OpenD lifecycle. Rust only
/// replays the complete captured wire value in explicit test-cutover mode.
pub trait MarketDataNewsSearchReadSnapshotPort: Send + Sync + Debug {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<Value, MarketDataNewsSearchReadSnapshotError>;
}

#[derive(Clone, Debug, Error, PartialEq, Eq)]
pub enum MarketDataNewsSearchReadSnapshotError {
    #[error("market-data news search snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("market-data news search snapshot failed: {code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
        retry_after_seconds: Option<u64>,
    },
}
