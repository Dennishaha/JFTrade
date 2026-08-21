/// Consumer-owned read-only projections for research screen presets. Go keeps
/// SQLite and definition normalization; Rust only exposes the captured wire
/// projection in explicit test-cutover wiring.
pub trait ResearchPresetReadSnapshotPort: Send + Sync + std::fmt::Debug {
    fn read(&self, path: &str, query: &str)
        -> Result<serde_json::Value, ResearchPresetReadSnapshotError>;
}

#[derive(Clone, Debug, Error)]
pub enum ResearchPresetReadSnapshotError {
    #[error("research preset read snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("research preset read snapshot request is invalid: {0}")]
    Invalid(String),
    #[error("research preset read snapshot resource was not found")]
    NotFound,
}
