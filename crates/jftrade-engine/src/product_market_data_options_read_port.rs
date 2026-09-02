/// Consumer-owned option projections.
pub trait MarketDataOptionsReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<serde_json::Value, MarketDataOptionsReadSnapshotError>;
}

#[derive(Clone, Debug, Error)]
pub enum MarketDataOptionsReadSnapshotError {
    #[error("market-data options snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("market-data options snapshot failed: {code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
    },
}
