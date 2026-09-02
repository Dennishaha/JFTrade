/// Consumer-owned lifecycle projections for system read routes. Go remains
/// the OpenD and order-update worker owner; Rust only serves captured wire
/// values in explicit test-cutover wiring.
pub trait SystemReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(&self, path: &str) -> Result<serde_json::Value, SystemReadSnapshotError>;
}

#[derive(Clone, Debug, Error)]
pub enum SystemReadSnapshotError {
    #[error("system read snapshot is unavailable: {0}")]
    Unavailable(String),
}

impl ProductConfig {
    #[cfg(test)]
    fn with_system_read_snapshot_port(mut self, port: Arc<dyn SystemReadSnapshotPort>) -> Self {
        self.system_read_snapshot_port = Some(port);
        self
    }
}

impl ProductApi {
    fn system_read(&self, path: &str) -> Result<ApiOutput, ApiFailure> {
        let port = self.system_read_snapshot_port.as_ref().ok_or_else(|| {
            ApiFailure::new(
                503,
                "SYSTEM_READ_UNAVAILABLE",
                "system read snapshot is not configured",
            )
        })?;
        port.read(path)
            .map(ApiOutput::Json)
            .map_err(|error| ApiFailure::new(503, "SYSTEM_READ_UNAVAILABLE", error.to_string()))
    }
}
