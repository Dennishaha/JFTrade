/// Consumer-owned remote watchlist projection. Mutation remains Go-owned.
pub trait RemoteWatchlistSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(&self, query: &str) -> Result<serde_json::Value, RemoteWatchlistSnapshotError>;
}

#[derive(Clone, Debug, Error)]
pub enum RemoteWatchlistSnapshotError {
    #[error("remote watchlist snapshot is unavailable: {0}")]
    Unavailable(String),
}

impl ProductApi {
    fn remote_watchlist_read(&self, query: &str) -> Result<ApiOutput, ApiFailure> {
        let port = self.remote_watchlist_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(503, "WATCHLIST_UNAVAILABLE", "remote watchlist snapshot is not configured")
        })?;
        port.read(query).map(ApiOutput::Json).map_err(|error| {
            ApiFailure::new(503, "WATCHLIST_UNAVAILABLE", error.to_string())
        })
    }
}
