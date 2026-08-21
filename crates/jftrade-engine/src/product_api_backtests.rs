/// Consumer-owned read-only projections for persisted backtest runs. The Go
/// run store remains the sole production owner; Rust only receives snapshots
/// in explicit test-cutover wiring and never opens the backtest database.
pub trait BacktestReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn list(&self) -> Result<serde_json::Value, BacktestReadSnapshotError>;
    fn status(&self, run_id: &str) -> Result<Option<serde_json::Value>, BacktestReadSnapshotError>;
    fn result(&self, run_id: &str) -> Result<Option<serde_json::Value>, BacktestReadSnapshotError>;
}

#[derive(Clone, Debug, Error)]
pub enum BacktestReadSnapshotError {
    #[error("backtest run snapshot is unavailable: {0}")]
    Unavailable(String),
}

impl ProductConfig {
    #[cfg(test)]
    fn with_backtest_read_snapshot_port(
        mut self,
        port: Arc<dyn BacktestReadSnapshotPort>,
    ) -> Self {
        self.backtest_read_snapshot_port = Some(port);
        self
    }
}

impl ProductApi {
    fn backtest_list(&self) -> Result<ApiOutput, ApiFailure> {
        self.backtest_read_port()?
            .list()
            .map(ApiOutput::Json)
            .map_err(backtest_read_snapshot_failure)
    }

    fn backtest_status(&self, path: &str) -> Result<ApiOutput, ApiFailure> {
        let run_id = backtest_run_id(path)?;
        self.backtest_read_port()?
            .status(&run_id)
            .map_err(backtest_read_snapshot_failure)?
            .map(ApiOutput::Json)
            .ok_or_else(|| ApiFailure::new(404, "NOT_FOUND", "backtest run not found"))
    }

    fn backtest_result(&self, path: &str) -> Result<ApiOutput, ApiFailure> {
        let run_id = backtest_run_id(path)?;
        self.backtest_read_port()?
            .result(&run_id)
            .map_err(backtest_read_snapshot_failure)?
            .map(ApiOutput::Json)
            .ok_or_else(|| ApiFailure::new(404, "NOT_FOUND", "backtest run not found"))
    }

    fn backtest_read_port(
        &self,
    ) -> Result<&Arc<dyn BacktestReadSnapshotPort>, ApiFailure> {
        self.backtest_read_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                500,
                "BACKTEST_RUN_STORE_FAILED",
                "run store not configured",
            )
        })
    }
}

fn backtest_read_snapshot_failure(error: BacktestReadSnapshotError) -> ApiFailure {
    let BacktestReadSnapshotError::Unavailable(message) = error;
    ApiFailure::new(500, "BACKTEST_RUN_STORE_FAILED", message)
}

fn backtest_run_id(path: &str) -> Result<String, ApiFailure> {
    let suffix = path
        .strip_prefix("/api/v1/backtests/")
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "backtest run id is invalid"))?;
    let encoded = suffix
        .strip_suffix("/status")
        .or_else(|| suffix.strip_suffix('/'))
        .unwrap_or(suffix);
    let decoded = percent_decode_str(encoded)
        .decode_utf8()
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "backtest run id is invalid"))?;
    let run_id = decoded.trim();
    if run_id.is_empty() || run_id.contains('/') {
        return Err(ApiFailure::new(
            400,
            "BAD_REQUEST",
            "backtest run id is invalid",
        ));
    }
    Ok(run_id.to_owned())
}
