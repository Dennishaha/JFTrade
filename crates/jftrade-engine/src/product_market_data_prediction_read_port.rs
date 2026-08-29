/// Consumer-owned projections for prediction-market discovery and contract
/// reads.
pub trait MarketDataPredictionReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<serde_json::Value, MarketDataPredictionReadSnapshotError>;
}

#[derive(Clone, Debug, thiserror::Error)]
pub enum MarketDataPredictionReadSnapshotError {
    #[allow(dead_code)]
    #[error("market-data prediction snapshot request is invalid: {0}")]
    Invalid(String),
    #[error("market-data prediction snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("market-data prediction snapshot failed: {code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
        retry_after_seconds: Option<u64>,
    },
}
