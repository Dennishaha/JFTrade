/// Read-only projections for persisted backtest runs. Production composition
/// binds this port to the leased Rust SQLite store; test profiles may inject a
/// snapshot implementation for compatibility replay.
pub trait BacktestReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn list(&self) -> Result<serde_json::Value, BacktestReadSnapshotError>;
    fn status(&self, run_id: &str) -> Result<Option<serde_json::Value>, BacktestReadSnapshotError>;
    fn result(&self, run_id: &str) -> Result<Option<serde_json::Value>, BacktestReadSnapshotError>;
    fn result_view(
        &self,
        request: &BacktestResultViewRequest,
    ) -> Result<Option<BacktestResultViewSnapshot>, BacktestResultViewError>;
}

#[derive(Clone, Debug, Default, PartialEq, Eq)]
pub struct BacktestResultViewRequest {
    pub run_id: String,
    pub view: Option<String>,
    pub include: Option<Vec<String>>,
    pub start_time: Option<String>,
    pub end_time: Option<String>,
    pub cursor: Option<String>,
    pub limit: Option<usize>,
    pub resolution: Option<String>,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
pub struct BacktestResultViewSnapshot {
    pub data: serde_json::Value,
}

#[derive(Clone, Debug, Error)]
pub enum BacktestResultViewError {
    #[error("invalid argument: {0}")]
    Invalid(String),
    #[error("backtest run was not found: {0}")]
    NotFound(String),
    #[error("backtest result view unavailable: {0}")]
    Unavailable(String),
    #[error("backtest result view failed: {0}")]
    Failed(String),
}

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct BacktestDataCoverageRequest {
    pub provider: String,
    pub symbol: String,
    pub interval: String,
    pub rehab_type: String,
    pub session_scope: String,
    pub start_time_ms: i64,
    pub end_time_ms: i64,
    pub warmup_bars: usize,
}

/// Persisted projection for the mutable backtest sync-task state. The
/// production implementation uses the backtest-runs WriterLease; a worker is
/// still required before new sync requests can be accepted.
pub trait BacktestSyncReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn progress(
        &self,
        task_id: &str,
    ) -> Result<Option<serde_json::Value>, BacktestSyncReadSnapshotError>;
    fn active_tasks(&self) -> Result<Vec<serde_json::Value>, BacktestSyncReadSnapshotError>;
    fn check_coverage(
        &self,
        _request: &BacktestDataCoverageRequest,
    ) -> Result<bool, BacktestSyncReadSnapshotError> {
        Ok(false)
    }
}

#[derive(Clone, Debug, Error)]
pub enum BacktestReadSnapshotError {
    #[error("backtest run snapshot is unavailable: {0}")]
    Unavailable(String),
}

#[derive(Clone, Debug, Error)]
pub enum BacktestSyncReadSnapshotError {
    #[error("backtest sync task snapshot is unavailable: {0}")]
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

    #[cfg(test)]
    fn with_backtest_sync_read_snapshot_port(
        mut self,
        port: Arc<dyn BacktestSyncReadSnapshotPort>,
    ) -> Self {
        self.backtest_sync_read_snapshot_port = Some(port);
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

    fn backtest_sync_progress(&self, path: &str) -> Result<ApiOutput, ApiFailure> {
        let task_id = backtest_sync_task_id(path)?;
        self.backtest_sync_read_port()?
            .progress(&task_id)
            .map_err(backtest_sync_read_snapshot_failure)?
            .map(ApiOutput::Json)
            .ok_or_else(|| ApiFailure::new(404, "NOT_FOUND", "sync task not found"))
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

    fn backtest_sync_read_port(
        &self,
    ) -> Result<&Arc<dyn BacktestSyncReadSnapshotPort>, ApiFailure> {
        self.backtest_sync_read_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                500,
                "BACKTEST_SYNC_TASK_STORE_FAILED",
                "sync task store not configured",
            )
        })
    }
}

fn backtest_read_snapshot_failure(error: BacktestReadSnapshotError) -> ApiFailure {
    let BacktestReadSnapshotError::Unavailable(message) = error;
    ApiFailure::new(500, "BACKTEST_RUN_STORE_FAILED", message)
}

fn backtest_sync_read_snapshot_failure(error: BacktestSyncReadSnapshotError) -> ApiFailure {
    let BacktestSyncReadSnapshotError::Unavailable(message) = error;
    ApiFailure::new(500, "BACKTEST_SYNC_TASK_STORE_FAILED", message)
}

fn backtest_run_id(path: &str) -> Result<String, ApiFailure> {
    let suffix = path
        .strip_prefix("/api/v1/backtests/")
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "backtest run id is invalid"))?;
    let encoded = suffix
        .strip_suffix("/status")
        .or_else(|| suffix.strip_suffix('/'))
        .unwrap_or(suffix);
    if has_invalid_backtest_percent_escape(encoded) {
        return Err(ApiFailure::new(
            400,
            "BAD_REQUEST",
            "backtest run id is invalid",
        ));
    }
    let decoded = percent_decode_str(encoded)
        .decode_utf8()
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "backtest run id is invalid"))?;
    let run_id = decoded.trim();
    if run_id.contains('/') {
        return Err(ApiFailure::new(
            400,
            "BAD_REQUEST",
            "backtest run id is invalid",
        ));
    }
    Ok(run_id.to_owned())
}

fn has_invalid_backtest_percent_escape(value: &str) -> bool {
    let bytes = value.as_bytes();
    let mut index = 0;
    while index < bytes.len() {
        if bytes[index] != b'%' {
            index += 1;
            continue;
        }
        if index + 2 >= bytes.len()
            || !bytes[index + 1].is_ascii_hexdigit()
            || !bytes[index + 2].is_ascii_hexdigit()
        {
            return true;
        }
        index += 3;
    }
    false
}

fn backtest_sync_task_id(path: &str) -> Result<String, ApiFailure> {
    let encoded = path
        .strip_prefix("/api/v1/backtests/sync/")
        .filter(|value| !value.is_empty() && !value.contains('/'))
        .ok_or_else(|| ApiFailure::new(400, "BAD_REQUEST", "taskId is invalid"))?;
    let decoded = percent_decode_str(encoded)
        .decode_utf8()
        .map_err(|_| ApiFailure::new(400, "BAD_REQUEST", "taskId is invalid"))?;
    let task_id = decoded.trim();
    if task_id.is_empty() {
        return Err(ApiFailure::new(400, "BAD_REQUEST", "taskId is required"));
    }
    Ok(task_id.to_owned())
}
