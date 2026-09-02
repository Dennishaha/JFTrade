pub type MarketDataCatalogReadFuture<'a> =
    Pin<Box<dyn Future<Output = Result<serde_json::Value, MarketDataCatalogReadSnapshotError>> + Send + 'a>>;

/// Consumer-owned market-data catalog projections for markets and instrument search.
pub trait MarketDataCatalogReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read<'a>(
        &'a self,
        path: &'a str,
        query: &'a str,
    ) -> MarketDataCatalogReadFuture<'a>;
}

#[derive(Clone, Debug, Error)]
pub enum MarketDataCatalogReadSnapshotError {
    #[error("market-data catalog snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("market-data catalog request is invalid: {code}: {message}")]
    Invalid { code: String, message: String },
    #[error("market-data catalog snapshot failed: {code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
    },
}
