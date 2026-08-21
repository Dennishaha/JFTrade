/// Consumer-owned derivative catalog projections. Go retains broker
/// resolution, Provider/OpenD lifecycle and the derivative query service.
pub trait MarketDataDerivativeReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<serde_json::Value, MarketDataDerivativeReadSnapshotError>;
}

#[derive(Clone, Debug, Error)]
pub enum MarketDataDerivativeReadSnapshotError {
    #[error("market-data derivative snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("market-data derivative snapshot failed: {code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
    },
}
