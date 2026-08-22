/// Consumer-owned projections for provider-backed news and corporate-action
/// reads. Go remains authoritative for Provider/OpenD lifecycle, capability
/// checks, query validation, normalization, retries, and response shaping.
pub trait MarketDataNewsActionsReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<serde_json::Value, MarketDataNewsActionsReadSnapshotError>;
}

#[derive(Clone, Debug, Error)]
pub enum MarketDataNewsActionsReadSnapshotError {
    #[error("market-data news/actions snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("market-data news/actions snapshot failed: {code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
        retry_after_seconds: Option<u64>,
    },
}
