/// Captured Assistant projections supplied by the current Go owner.
///
/// The port deliberately transports complete wire values and replay events.
/// Rust does not open the ADK database, run a Provider, reconcile state, or
/// create a second lifecycle owner before the composition-root cutover.
pub trait AdkReadSnapshotPort: Send + Sync + Debug {
    fn read(&self, path: &str, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError>;
}

#[derive(Clone, Debug, PartialEq)]
pub enum AdkReadSnapshot {
    Json(Value),
    Stream(AdkReadStream),
}

#[derive(Clone, Debug, Default, PartialEq)]
pub struct AdkReadStream {
    pub headers: Vec<(String, String)>,
    pub events: Vec<AdkReadEvent>,
}

#[derive(Clone, Debug, PartialEq)]
pub struct AdkReadEvent {
    pub id: Option<String>,
    pub data: Value,
}

#[derive(Clone, Debug, Error, PartialEq, Eq)]
pub enum AdkReadSnapshotError {
    #[error("ADK read snapshot is unavailable: {0}")]
    Unavailable(String),
    #[error("ADK read snapshot failed: {code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
        retry_after_seconds: Option<u64>,
    },
}
