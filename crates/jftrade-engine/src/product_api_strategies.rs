/// Consumer-owned projections for strategy instances and their activity.
/// Go remains the owner of catalog/runtime/activity stores; Rust only receives
/// complete wire snapshots in explicit test-cutover wiring.
pub trait StrategyReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<Option<serde_json::Value>, StrategyReadSnapshotError>;
}

#[derive(Clone, Debug, Error)]
pub enum StrategyReadSnapshotError {
    #[error("strategy read snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("invalid strategy read query: {0}")]
    Invalid(String),
}

impl ProductConfig {
    #[cfg(test)]
    fn with_strategy_read_snapshot_port(
        mut self,
        port: Arc<dyn StrategyReadSnapshotPort>,
    ) -> Self {
        self.strategy_read_snapshot_port = Some(port);
        self
    }

    #[cfg(test)]
    fn with_strategy_runtime_write_port(
        mut self,
        port: Arc<dyn StrategyRuntimeWritePort>,
    ) -> Self {
        self.strategy_runtime_write_port = Some(port);
        self
    }
}

impl ProductApi {
    fn strategy_read(&self, path: &str, query: &str) -> Result<ApiOutput, ApiFailure> {
        let port = self.strategy_read_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "STRATEGY_UNAVAILABLE",
                "strategy read snapshot is not configured",
            )
        })?;
        port.read(path, query)
            .map_err(strategy_read_snapshot_failure)?
            .map(ApiOutput::Json)
            .ok_or_else(|| ApiFailure::new(404, "NOT_FOUND", "resource not found"))
    }
}

fn strategy_read_snapshot_failure(error: StrategyReadSnapshotError) -> ApiFailure {
    match error {
        StrategyReadSnapshotError::Unavailable(message) => {
            ApiFailure::new(500, "STRATEGY_FAILED", message)
        }
        StrategyReadSnapshotError::Invalid(message) => {
            ApiFailure::new(400, "BAD_REQUEST", message)
        }
    }
}

fn is_strategy_read_path(path: &str) -> bool {
    if path == "/api/v1/strategies" {
        return true;
    }
    let Some(suffix) = path.strip_prefix("/api/v1/strategies/") else {
        return false;
    };
    let mut parts = suffix.split('/');
    parts.next().is_some_and(|id| !id.is_empty() && !id.contains('/'))
        && matches!(parts.next(), Some("logs") | Some("audit"))
        && parts.next().is_none()
}
