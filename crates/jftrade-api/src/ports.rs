use std::collections::BTreeMap;
use std::future::Future;
use std::io;
use std::pin::Pin;
use std::sync::{Arc, Mutex};

use serde::{Deserialize, Serialize};
use serde_json::Value;
use tokio::sync::mpsc;

use crate::{ApiFailure, SseEvent};

#[derive(Clone, Debug, Eq, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ApiRequest {
    pub method: String,
    pub path: String,
    pub query: String,
    pub body: Vec<u8>,
    pub request_id: String,
    #[serde(default)]
    pub desktop_trusted: bool,
    #[serde(default)]
    pub origin_provided: bool,
    #[serde(default)]
    pub origin_allowed: bool,
    #[serde(default)]
    pub browser_authenticated: bool,
    #[serde(default)]
    pub csrf_valid: bool,
    #[serde(default)]
    pub session_cookie: Option<String>,
}

#[derive(Clone, Debug, PartialEq)]
pub enum ApiOutput {
    Json(Value),
    Sse(Vec<SseEvent>),
    NoContent,
    Raw {
        status: u16,
        content_type: String,
        body: Vec<u8>,
        headers: BTreeMap<String, String>,
    },
    /// A response body that is produced incrementally.  The receiver is
    /// single-consumer; dropping it propagates cancellation to the producer
    /// through [`ApiStreamSender::send`].
    RawStream {
        status: u16,
        content_type: String,
        stream: ApiStream,
        headers: BTreeMap<String, String>,
    },
}

/// Single-consumer body stream used by long-lived HTTP responses.
///
/// Keeping the channel behind a small transport-owned type lets domain ports
/// return a cancellable body without depending on Axum's `Body` type.
type ApiStreamReceiver = Arc<Mutex<Option<mpsc::Receiver<Result<Vec<u8>, io::Error>>>>>;

#[derive(Clone)]
pub struct ApiStream {
    receiver: ApiStreamReceiver,
}

impl std::fmt::Debug for ApiStream {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.write_str("ApiStream(..)")
    }
}

impl PartialEq for ApiStream {
    fn eq(&self, other: &Self) -> bool {
        Arc::ptr_eq(&self.receiver, &other.receiver)
    }
}

impl Eq for ApiStream {}

pub struct ApiStreamSender {
    sender: mpsc::Sender<Result<Vec<u8>, io::Error>>,
}

impl std::fmt::Debug for ApiStreamSender {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.write_str("ApiStreamSender(..)")
    }
}

impl ApiStream {
    pub fn channel(capacity: usize) -> (Self, ApiStreamSender) {
        let (sender, receiver) = mpsc::channel(capacity.max(1));
        (
            Self {
                receiver: Arc::new(Mutex::new(Some(receiver))),
            },
            ApiStreamSender { sender },
        )
    }

    pub(crate) fn take_receiver(&self) -> Option<mpsc::Receiver<Result<Vec<u8>, io::Error>>> {
        self.receiver.lock().ok()?.take()
    }
}

impl ApiStreamSender {
    /// Blocking send is intentional: production model adapters run their
    /// provider reader on a dedicated thread and must apply backpressure when
    /// the HTTP client is slower than the upstream stream.
    #[allow(clippy::result_unit_err)]
    pub fn send(&self, chunk: Vec<u8>) -> Result<(), ()> {
        self.sender.blocking_send(Ok(chunk)).map_err(|_| ())
    }

    #[allow(clippy::result_unit_err)]
    pub fn send_error(&self, error: io::Error) -> Result<(), ()> {
        self.sender.blocking_send(Err(error)).map_err(|_| ())
    }

    pub fn is_closed(&self) -> bool {
        self.sender.is_closed()
    }
}

pub type PortFuture<'a> = Pin<Box<dyn Future<Output = Result<ApiOutput, ApiFailure>> + Send + 'a>>;

pub trait ApiPort: Send + Sync {
    fn dispatch(&self, request: ApiRequest) -> PortFuture<'_>;
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Asset {
    pub content_type: String,
    pub bytes: Vec<u8>,
}

#[derive(Clone, Debug, Default, Eq, PartialEq)]
pub struct AssetBundle {
    assets: BTreeMap<String, Asset>,
}

impl AssetBundle {
    pub fn new(assets: impl IntoIterator<Item = (String, Asset)>) -> Self {
        Self {
            assets: assets.into_iter().collect(),
        }
    }

    pub fn get(&self, path: &str) -> Option<&Asset> {
        self.assets.get(path.trim_start_matches('/'))
    }

    pub fn spa_index(&self) -> Option<&Asset> {
        self.assets.get("index.html")
    }
}
