/// Consumer-owned provider status projection. The Go market-data service keeps
/// provider health, runtime, and subscriptions authoritative during rehearsal.
pub trait MarketDataProviderReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<serde_json::Value, MarketDataProviderReadSnapshotError>;
}

#[derive(Clone, Debug, Error)]
pub enum MarketDataProviderReadSnapshotError {
    #[error("market-data provider snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("market-data provider snapshot failed: {code}: {message}")]
    Failed { code: String, message: String },
}
