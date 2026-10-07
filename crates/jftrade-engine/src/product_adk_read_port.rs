/// Assistant read projections and optional reconnect bodies supplied by the
/// runtime owner. Fixture adapters may serve a finite snapshot; the production
/// adapter supplies a cursor reader owned by the HTTP body consumer.
pub trait AdkReadSnapshotPort: Send + Sync + Debug {
    fn read(&self, path: &str, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError>;

    fn open_stream(
        &self,
        _path: &str,
        _query: &str,
    ) -> Result<Option<AdkReadLiveStream>, AdkReadSnapshotError> {
        Ok(None)
    }
}

#[derive(Clone, Debug, PartialEq)]
pub struct AdkReadLiveStream {
    pub headers: Vec<(String, String)>,
    pub body: jftrade_api::ApiStream,
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
