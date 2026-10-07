use std::collections::BTreeMap;
use std::future::Future;
use std::io;
use std::pin::Pin;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use serde::{Deserialize, Serialize};
use serde_json::Value;
use tokio::sync::mpsc;
use tokio_stream::{Stream, wrappers::ReceiverStream};

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
pub type ApiStreamBody = Pin<Box<dyn Stream<Item = Result<Vec<u8>, io::Error>> + Send>>;
type ApiStreamReceiver = Arc<Mutex<Option<ApiStreamBody>>>;

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
                receiver: Arc::new(Mutex::new(Some(Box::pin(ReceiverStream::new(receiver))))),
            },
            ApiStreamSender { sender },
        )
    }

    /// Produce chunks only when the HTTP consumer polls, without a worker or
    /// an eagerly encoded body. Dropping the body drops the remaining iterator.
    pub fn from_chunks(
        chunks: impl Iterator<Item = Result<Vec<u8>, io::Error>> + Send + 'static,
    ) -> Self {
        Self::from_stream(Box::pin(tokio_stream::iter(chunks)))
    }

    /// Keep one consumer for a pull-driven body without starting a producer task.
    pub fn from_stream(body: ApiStreamBody) -> Self {
        Self {
            receiver: Arc::new(Mutex::new(Some(body))),
        }
    }

    /// Acquire the single body consumer. A clone cannot acquire it a second
    /// time; channel and pull-based bodies share the same ownership rule.
    pub fn take_body(&self) -> Option<ApiStreamBody> {
        self.receiver.lock().ok()?.take()
    }
}

#[cfg(test)]
mod pull_body_tests {
    use super::*;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use tokio_stream::StreamExt;

    #[tokio::test]
    async fn pull_body_advances_only_on_demand_and_releases_remaining_iterator() {
        struct ReleaseProbe(Arc<AtomicUsize>);
        impl Drop for ReleaseProbe {
            fn drop(&mut self) {
                self.0.fetch_add(1, Ordering::SeqCst);
            }
        }
        let produced = Arc::new(AtomicUsize::new(0));
        let released = Arc::new(AtomicUsize::new(0));
        let probe = ReleaseProbe(released.clone());
        let count = produced.clone();
        let chunks = (0_u64..1_000_000).map(move |index| {
            let _ = &probe;
            count.fetch_add(1, Ordering::SeqCst);
            Ok(index.to_le_bytes().to_vec())
        });
        let source = ApiStream::from_chunks(chunks);
        assert_eq!(produced.load(Ordering::SeqCst), 0);
        let mut body = source.take_body().expect("body consumer");
        assert!(
            source.clone().take_body().is_none(),
            "one consumer across clones"
        );
        assert_eq!(
            body.next().await.expect("first chunk").expect("chunk"),
            0_u64.to_le_bytes()
        );
        assert_eq!(produced.load(Ordering::SeqCst), 1);
        drop(body);
        assert_eq!(
            produced.load(Ordering::SeqCst),
            1,
            "no background encoding or retry"
        );
        assert_eq!(
            released.load(Ordering::SeqCst),
            1,
            "iterator ownership released"
        );
    }
}

/// Upper bound for a producer that cannot block the calling thread because it
/// runs inside a Tokio runtime.  The consumer normally drains continuously, so
/// this only expires when the HTTP client stopped reading; reporting `Err` then
/// cancels the producing run instead of hanging it.
const STREAM_SEND_BACKPRESSURE_TIMEOUT: Duration = Duration::from_secs(30);

impl ApiStreamSender {
    /// Send one body chunk, applying backpressure without panicking.
    ///
    /// Outside a Tokio runtime (dedicated producer threads) this blocks, which
    /// is the documented contract: a slow HTTP client slows the upstream
    /// reader.  Inside a runtime `blocking_send` would panic, which silently
    /// killed successful live streams before their terminal frame, so there we
    /// hand the chunk to the consumer through the channel and yield until it
    /// makes room.
    #[allow(clippy::result_unit_err)]
    pub fn send(&self, chunk: Vec<u8>) -> Result<(), ()> {
        match self.sender.try_send(Ok(chunk)) {
            Ok(()) => Ok(()),
            Err(mpsc::error::TrySendError::Closed(_)) => Err(()),
            Err(mpsc::error::TrySendError::Full(chunk)) => {
                if tokio::runtime::Handle::try_current().is_err() {
                    return self.sender.blocking_send(chunk).map_err(|_| ());
                }
                self.send_from_runtime(chunk)
            }
        }
    }

    #[allow(clippy::result_unit_err)]
    pub fn send_error(&self, error: io::Error) -> Result<(), ()> {
        match self.sender.try_send(Err(error)) {
            Ok(()) => Ok(()),
            Err(mpsc::error::TrySendError::Closed(_)) => Err(()),
            Err(mpsc::error::TrySendError::Full(error)) => {
                if tokio::runtime::Handle::try_current().is_err() {
                    return self.sender.blocking_send(error).map_err(|_| ());
                }
                self.send_from_runtime(error)
            }
        }
    }

    /// Retry a full channel from inside a runtime.
    ///
    /// The consumer lives on another task/thread, so yielding here lets it
    /// drain and preserves the ordering of the produced frames.
    fn send_from_runtime(&self, mut message: Result<Vec<u8>, io::Error>) -> Result<(), ()> {
        let deadline = std::time::Instant::now() + STREAM_SEND_BACKPRESSURE_TIMEOUT;
        loop {
            if std::time::Instant::now() >= deadline {
                return Err(());
            }
            match self.sender.try_send(message) {
                Ok(()) => return Ok(()),
                Err(mpsc::error::TrySendError::Closed(_)) => return Err(()),
                Err(mpsc::error::TrySendError::Full(returned)) => {
                    message = returned;
                    std::thread::sleep(Duration::from_millis(1));
                }
            }
        }
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
