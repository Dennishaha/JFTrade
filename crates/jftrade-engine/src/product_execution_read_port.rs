/// Consumer-owned execution read projections. Go remains the order ledger,
/// update worker, broker refresh, and permission owner during rehearsal.
pub trait ExecutionReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<serde_json::Value, ExecutionReadSnapshotError>;
}

#[derive(Clone, Debug, Error)]
pub enum ExecutionReadSnapshotError {
    #[error("execution read snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("execution read snapshot request is invalid: {0}")]
    Invalid(String),
    #[error("execution order was not found")]
    NotFound,
    #[error("execution read failed: {code}: {message}")]
    Failed { code: String, message: String },
}
