/// Consumer-owned projection for CN index constituent listings.
///
/// The reference exposes `market.index_constituents` only through the ADK tool
/// registry (it is not part of `LocalMCPReadOnlyToolNames` and has no public
/// HTTP route), so this port is consumed by the production MCP executor
/// instead of a wire handler.
pub trait MarketIndexConstituentsReadPort: Send + Sync + std::fmt::Debug {
    /// Read one CN index member list.
    ///
    /// `market` is the normalized exchange label (`SH`/`SZ`) and `limit` is the
    /// already validated page size. The payload keeps the reference contract:
    /// `market`, `symbol`, `instrumentId`, `constituents` and `source`.
    fn read(
        &self,
        market: &str,
        symbol: &str,
        limit: usize,
    ) -> Result<serde_json::Value, MarketIndexConstituentsReadError>;
}

#[derive(Clone, Debug, Error)]
pub enum MarketIndexConstituentsReadError {
    #[error("index constituents read is unavailable: {0}")]
    Unavailable(String),
    #[error("index constituents read failed: {code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
        retry_after_seconds: Option<u64>,
    },
}
