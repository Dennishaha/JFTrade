#[derive(Clone, Debug, Error)]
pub enum AlertSnapshotError {
    #[error("alert snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("alert snapshot capability is unavailable: {0}")]
    CapabilityUnavailable(String),
    #[error("alert snapshot provider failed: {message}")]
    Provider {
        status: Option<u16>,
        message: String,
    },
}

#[derive(Clone, Debug, Error)]
pub enum StrategyDefinitionSnapshotError {
    #[error("strategy definition snapshot is unavailable: {0}")]
    Unavailable(String),
}

#[derive(Clone, Debug, Error)]
pub enum WatchlistMembershipSnapshotError {
    #[error("watchlist membership snapshot is unavailable: {0}")]
    Unavailable(String),
}

#[derive(Clone, Debug, Error)]
pub enum WatchlistReadSnapshotError {
    #[error("watchlist read snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("watchlist read snapshot rejected request: {0}")]
    Invalid(String),
    #[error("watchlist read snapshot resource was not found")]
    NotFound,
}

#[derive(Clone, Debug, Error)]
pub enum PortfolioSnapshotError {
    #[error("portfolio snapshot is unavailable: {0}")]
    Unavailable(String),
}

#[derive(Clone, Debug, Error)]
pub enum ResearchReadSnapshotError {
    #[error("research read snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("research read snapshot request is invalid: {0}")]
    Invalid(String),
    #[error("research read snapshot failed with {status}: [{code}] {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
    },
}

#[derive(Clone, Debug, Error)]
pub enum BrokerReadSnapshotError {
    #[error("broker read snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("broker read snapshot request is invalid: {0}")]
    Invalid(String),
}

#[derive(Clone, Debug, Error)]
pub enum PluginUninstallGuidanceSnapshotError {
    #[error("plugin uninstall guidance snapshot is unavailable: {0}")]
    Unavailable(String),
}

#[derive(Clone, Debug, Error)]
pub enum PluginSnapshotError {
    #[error("plugin snapshot is unavailable: {0}")]
    Unavailable(String),
}
