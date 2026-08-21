/// Consumer-owned market-data catalog projections. The Go market-data
/// service remains authoritative for provider lifecycle and catalog queries.
pub trait MarketDataCatalogReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<serde_json::Value, MarketDataCatalogReadSnapshotError>;
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
