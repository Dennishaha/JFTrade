use std::io;
use std::panic::{AssertUnwindSafe, catch_unwind};
use std::sync::{Arc, Mutex};

use serde::{Deserialize, Serialize};
use serde_json::Value;
use tokio::sync::watch;

pub const SSE_RETRY_MILLIS: u64 = 3000;

#[derive(Clone, Debug, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct SseEvent {
    pub id: Option<String>,
    pub data: Value,
}

pub fn encode_retry(milliseconds: u64) -> String {
    if milliseconds == 0 {
        String::new()
    } else {
        format!("retry: {milliseconds}\n\n")
    }
}

pub fn encode_event(event: &SseEvent) -> Result<String, serde_json::Error> {
    let mut frame = String::new();
    if let Some(id) = event.id.as_deref() {
        frame.push_str("id: ");
        frame.push_str(id);
        frame.push('\n');
    }
    frame.push_str("data: ");
    frame.push_str(&serde_json::to_string(&event.data)?);
    frame.push_str("\n\n");
    Ok(frame)
}

pub fn encode_comment(comment: &str) -> String {
    format!(": {comment}\n\n")
}

/// Response sink for [`SseWriter`].
///
/// Mirrors Go's `http.ResponseWriter` plus the optional `http.Flusher` type
/// assertion performed by `PrepareSSEWriter`.
pub trait SseSink: Send + 'static {
    fn write_frame(&mut self, frame: &str) -> io::Result<()>;

    fn flush(&mut self) -> io::Result<()>;

    /// Returns `false` for writers that cannot flush, mirroring the Go
    /// `PrepareSSEWriter` type assertion against `http.Flusher`.
    fn supports_flush(&self) -> bool {
        true
    }
}

/// In-memory sink used by the buffered `ApiOutput::Sse` router path and by
/// tests that assert the exact wire body.
#[derive(Clone, Debug, Default)]
pub struct BufferedSseSink {
    body: Arc<Mutex<Vec<u8>>>,
}

impl BufferedSseSink {
    pub fn new() -> Self {
        Self::default()
    }

    pub fn body(&self) -> String {
        String::from_utf8_lossy(&self.body.lock().expect("sse buffer")).into_owned()
    }
}

impl SseSink for BufferedSseSink {
    fn write_frame(&mut self, frame: &str) -> io::Result<()> {
        use std::io::Write;

        self.body
            .lock()
            .expect("sse buffer")
            .write_all(frame.as_bytes())
    }

    fn flush(&mut self) -> io::Result<()> {
        Ok(())
    }
}

/// SSE writer error.
///
/// Mirrors Go's `SSEWriter` error contract: serialization and I/O failures
/// are returned as-is, while a panic raised by the response writer or flusher
/// is converted into an observable `sse write failed: ...` error instead of
/// unwinding the connection owner.
#[derive(Debug)]
pub enum SseError {
    Serialization(serde_json::Error),
    Io(io::Error),
    Panic(String),
}

impl std::fmt::Display for SseError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Serialization(error) => write!(formatter, "{error}"),
            Self::Io(error) => write!(formatter, "{error}"),
            Self::Panic(message) => write!(formatter, "sse write failed: {message}"),
        }
    }
}

impl std::error::Error for SseError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::Serialization(error) => Some(error),
            Self::Io(error) => Some(error),
            Self::Panic(_) => None,
        }
    }
}

impl From<serde_json::Error> for SseError {
    fn from(error: serde_json::Error) -> Self {
        Self::Serialization(error)
    }
}

/// Thread-safe SSE frame writer.
///
/// Mirrors Go's `SSEWriter`: every frame passes through one mutex so
/// concurrent producers cannot interleave frames, and sink panics become
/// errors rather than aborting the streaming owner.
#[derive(Clone)]
pub struct SseWriter {
    sink: Arc<Mutex<Box<dyn SseSink>>>,
    retry_millis: u64,
}

impl std::fmt::Debug for SseWriter {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("SseWriter")
            .field("retry_millis", &self.retry_millis)
            .finish_non_exhaustive()
    }
}

impl SseWriter {
    pub fn new(sink: impl SseSink, retry_millis: u64) -> Self {
        Self {
            sink: Arc::new(Mutex::new(Box::new(sink))),
            retry_millis,
        }
    }

    /// Mirrors Go's `PrepareSSEWriter`: returns `None` when the sink cannot
    /// flush, before any retry directive or frame is emitted.
    pub fn prepare(sink: impl SseSink) -> Option<Self> {
        sink.supports_flush()
            .then(|| Self::new(sink, SSE_RETRY_MILLIS))
    }

    pub fn write_retry_directive(&self) -> Result<(), SseError> {
        if self.retry_millis == 0 {
            return Ok(());
        }
        self.write_frame(encode_retry(self.retry_millis))
    }

    pub fn write_event<T: Serialize + ?Sized>(&self, value: &T) -> Result<(), SseError> {
        let data = serde_json::to_string(value)?;
        self.write_frame(format!("data: {data}\n\n"))
    }

    pub fn write_event_id<T: Serialize + ?Sized>(
        &self,
        id: &str,
        value: &T,
    ) -> Result<(), SseError> {
        let data = serde_json::to_string(value)?;
        self.write_frame(format!("id: {id}\ndata: {data}\n\n"))
    }

    pub fn write_sse_event(&self, event: &SseEvent) -> Result<(), SseError> {
        self.write_frame(encode_event(event)?)
    }

    pub fn write_comment(&self, comment: &str) -> Result<(), SseError> {
        self.write_frame(encode_comment(comment))
    }

    fn write_frame(&self, frame: String) -> Result<(), SseError> {
        let mut sink = self.sink.lock().expect("sse sink");
        match catch_unwind(AssertUnwindSafe(|| {
            sink.write_frame(&frame)?;
            sink.flush()
        })) {
            Ok(Ok(())) => Ok(()),
            Ok(Err(error)) => Err(SseError::Io(error)),
            Err(payload) => Err(SseError::Panic(panic_message(payload.as_ref()))),
        }
    }
}

fn panic_message(payload: &(dyn std::any::Any + Send)) -> String {
    if let Some(message) = payload.downcast_ref::<&str>() {
        (*message).to_owned()
    } else if let Some(message) = payload.downcast_ref::<String>() {
        message.clone()
    } else {
        "unknown panic".to_owned()
    }
}

/// Options for [`run_sse_stream_loop`].
///
/// Mirrors Go's `SSEStreamLoopOptions`: `initial` runs once before the loop,
/// `write_interval` enables the periodic heartbeat, and `trigger` fires an
/// immediate tick.
#[derive(Default)]
pub struct SseStreamLoopOptions {
    pub initial: Option<Box<dyn FnOnce() -> SseLoopFuture + Send>>,
    pub write_interval: Option<std::time::Duration>,
    pub on_tick: Option<Box<dyn FnMut() -> SseLoopFuture + Send>>,
    pub trigger: Option<tokio::sync::mpsc::Receiver<()>>,
}

impl std::fmt::Debug for SseStreamLoopOptions {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter
            .debug_struct("SseStreamLoopOptions")
            .field("initial", &self.initial.is_some())
            .field("write_interval", &self.write_interval)
            .field("on_tick", &self.on_tick.is_some())
            .field("trigger", &self.trigger.is_some())
            .finish()
    }
}

/// Error returned by SSE loop callbacks.
pub type SseLoopError = Box<dyn std::error::Error + Send + Sync>;

/// Boxed future returned by SSE loop callbacks.
pub type SseLoopFuture =
    std::pin::Pin<Box<dyn std::future::Future<Output = Result<(), SseLoopError>> + Send>>;

/// Runs the SSE stream loop until cancellation.
///
/// Semantics mirror Go's `RunSSEStreamLoop`: `initial` errors abort before
/// any tick; cancellation resolves to `Ok(())`; a `trigger` send with
/// `on_tick == None` is ignored; and no ticker is armed unless both
/// `write_interval` and `on_tick` are present.
pub async fn run_sse_stream_loop(
    mut cancel: watch::Receiver<bool>,
    mut options: SseStreamLoopOptions,
) -> Result<(), SseLoopError> {
    if let Some(initial) = options.initial.take() {
        initial().await?;
    }

    let mut interval = match (options.write_interval, options.on_tick.is_some()) {
        (Some(interval), true) => {
            let mut ticker = tokio::time::interval(interval);
            ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Skip);
            Some(ticker)
        }
        _ => None,
    };

    loop {
        tokio::select! {
            cancelled = cancel.changed() => {
                if cancelled.is_err() || *cancel.borrow() {
                    return Ok(());
                }
            }
            Some(()) = async {
                match options.trigger.as_mut() {
                    Some(trigger) => trigger.recv().await,
                    None => std::future::pending().await,
                }
            } => {
                let Some(on_tick) = options.on_tick.as_mut() else {
                    continue;
                };
                on_tick().await?;
            }
            Some(_) = async {
                match interval.as_mut() {
                    Some(interval) => Some(interval.tick().await),
                    None => std::future::pending().await,
                }
            } => {
                let Some(on_tick) = options.on_tick.as_mut() else {
                    continue;
                };
                on_tick().await?;
            }
            else => return Ok(()),
        }
    }
}

#[cfg(test)]
mod tests {
    use std::sync::atomic::{AtomicBool, AtomicUsize, Ordering};

    use serde_json::json;

    use super::*;

    #[derive(Default)]
    struct PanicSink {
        body: Arc<Mutex<Vec<u8>>>,
        panic_on_write: AtomicBool,
        panic_on_flush: AtomicBool,
    }

    impl PanicSink {
        fn panicking_on_write() -> Self {
            let sink = Self::default();
            sink.panic_on_write.store(true, Ordering::SeqCst);
            sink
        }

        fn panicking_on_flush() -> Self {
            let sink = Self::default();
            sink.panic_on_flush.store(true, Ordering::SeqCst);
            sink
        }
    }

    impl SseSink for PanicSink {
        fn write_frame(&mut self, frame: &str) -> io::Result<()> {
            assert!(!self.panic_on_write.load(Ordering::SeqCst), "write failed");
            use std::io::Write;
            self.body.lock().expect("body").write_all(frame.as_bytes())
        }

        fn flush(&mut self) -> io::Result<()> {
            assert!(!self.panic_on_flush.load(Ordering::SeqCst), "flush failed");
            Ok(())
        }
    }

    struct NoFlushSink;

    impl SseSink for NoFlushSink {
        fn write_frame(&mut self, _frame: &str) -> io::Result<()> {
            Ok(())
        }

        fn flush(&mut self) -> io::Result<()> {
            Ok(())
        }

        fn supports_flush(&self) -> bool {
            false
        }
    }

    struct FailingSink(io::Error);

    impl SseSink for FailingSink {
        fn write_frame(&mut self, _frame: &str) -> io::Result<()> {
            Err(io::Error::new(self.0.kind(), self.0.to_string()))
        }

        fn flush(&mut self) -> io::Result<()> {
            Ok(())
        }
    }

    #[test]
    fn frames_preserve_go_retry_id_data_and_comment_shape() {
        assert_eq!(encode_retry(3000), "retry: 3000\n\n");
        assert_eq!(encode_retry(0), "");
        assert_eq!(
            encode_event(&SseEvent {
                id: Some("7".into()),
                data: json!({"type": "progress"}),
            })
            .expect("json"),
            "id: 7\ndata: {\"type\":\"progress\"}\n\n"
        );
        assert_eq!(encode_comment("heartbeat"), ": heartbeat\n\n");
    }

    #[test]
    fn prepare_writes_retry_then_id_event_and_comment_frames() {
        // Parity: internal/api/httpserver/sse_test.go:87 TestPrepareSSEWriterAndFrameFormatting
        let sink = BufferedSseSink::new();
        let writer = SseWriter::prepare(sink.clone()).expect("flushable sink");
        writer.write_retry_directive().expect("retry");
        writer
            .write_event_id("evt-1", &json!({"status": "ready"}))
            .expect("event id");
        writer.write_comment("heartbeat").expect("comment");

        assert_eq!(
            sink.body(),
            "retry: 3000\n\nid: evt-1\ndata: {\"status\":\"ready\"}\n\n: heartbeat\n\n"
        );
    }

    #[test]
    fn no_retry_writer_keeps_the_existing_body() {
        // Parity: internal/api/httpserver/sse_test.go:87 TestPrepareSSEWriterAndFrameFormatting
        let sink = BufferedSseSink::new();
        let writer = SseWriter::new(sink.clone(), 0);
        writer.write_retry_directive().expect("no retry");
        assert_eq!(sink.body(), "");
    }

    #[test]
    fn prepare_rejects_a_sink_without_flush_support() {
        // Parity: internal/api/httpserver/sse_test.go:129 TestPrepareSSEWriterRejectsWriterWithoutFlusher
        assert!(SseWriter::prepare(NoFlushSink).is_none());
    }

    #[test]
    fn write_event_returns_flush_panics_as_errors() {
        // Parity: internal/api/httpserver/sse_test.go:41 TestSSEWriterReturnsFlushPanicAsError
        let writer = SseWriter::prepare(PanicSink::panicking_on_flush()).expect("flushable sink");
        let error = writer
            .write_event(&json!({"type": "delta"}))
            .expect_err("flush panic");
        assert!(
            error.to_string().contains("flush failed"),
            "unexpected error: {error}"
        );
    }

    #[test]
    fn write_retry_returns_write_panics_as_errors() {
        // Parity: internal/api/httpserver/sse_test.go:55 TestSSEWriterReturnsWritePanicAsError
        let writer = SseWriter::prepare(PanicSink::panicking_on_write()).expect("flushable sink");
        let error = writer.write_retry_directive().expect_err("write panic");
        assert!(
            error.to_string().contains("write failed"),
            "unexpected error: {error}"
        );
    }

    #[test]
    fn write_failures_are_reported_with_their_source_message() {
        // Parity: internal/api/httpserver/sse_boundaries_test.go:86 TestFailingSSEWriterIncludesReadableError
        let writer = SseWriter::new(FailingSink(io::Error::other("network closed")), 3000);
        let error = writer.write_retry_directive().expect_err("write error");
        assert!(
            error.to_string().contains("network closed"),
            "unexpected error: {error}"
        );
    }

    #[test]
    fn concurrent_writers_serialize_frames() {
        // Parity: internal/api/httpserver/sse_concurrent_test.go:42 TestSSEWriterSerializesConcurrentWrites
        let sink = BufferedSseSink::new();
        let writer = SseWriter::prepare(sink.clone()).expect("flushable sink");
        let writes = Arc::new(AtomicUsize::new(0));
        let mut handles = Vec::new();
        for index in 0..8 {
            let writer = writer.clone();
            let writes = Arc::clone(&writes);
            handles.push(std::thread::spawn(move || {
                writer
                    .write_event(&json!({"type": "delta", "toolProgress": index}))
                    .expect("event");
                writes.fetch_add(1, Ordering::SeqCst);
            }));
        }
        for handle in handles {
            handle.join().expect("writer thread");
        }

        assert_eq!(writes.load(Ordering::SeqCst), 8);
        let body = sink.body();
        assert_eq!(body.matches("data: ").count(), 8, "body={body}");
        assert_eq!(body.matches("\n\n").count(), 8, "body={body}");
    }

    fn loop_error(message: &'static str) -> Box<dyn std::error::Error + Send + Sync> {
        Box::from(message)
    }

    #[tokio::test]
    async fn stream_loop_propagates_initial_error_before_any_tick() {
        // Parity: internal/api/httpserver/sse_test.go:136 TestRunSSEStreamLoopPropagatesInitialError
        let (_cancel_tx, cancel_rx) = watch::channel(false);
        let error = run_sse_stream_loop(
            cancel_rx,
            SseStreamLoopOptions {
                initial: Some(Box::new(|| {
                    Box::pin(async { Err(loop_error("initial failed")) }) as SseLoopFuture
                })),
                ..Default::default()
            },
        )
        .await
        .expect_err("initial error");
        assert_eq!(error.to_string(), "initial failed");
    }

    #[tokio::test]
    async fn stream_loop_runs_trigger_and_ticker_ticks() {
        // Parity: internal/api/httpserver/sse_test.go:146 TestRunSSEStreamLoopRunsTriggerAndTickerTicks
        let (cancel_tx, cancel_rx) = watch::channel(false);
        let (trigger_tx, trigger_rx) = tokio::sync::mpsc::channel(2);
        trigger_tx.send(()).await.expect("trigger");
        trigger_tx.send(()).await.expect("trigger");
        let ticks = Arc::new(AtomicUsize::new(0));
        let observed = Arc::clone(&ticks);
        run_sse_stream_loop(
            cancel_rx,
            SseStreamLoopOptions {
                on_tick: Some(Box::new(move || {
                    let observed = Arc::clone(&observed);
                    let cancel_tx = cancel_tx.clone();
                    Box::pin(async move {
                        if observed.fetch_add(1, Ordering::SeqCst) == 1 {
                            cancel_tx.send(true).expect("cancel");
                        }
                        Ok(())
                    }) as SseLoopFuture
                })),
                trigger: Some(trigger_rx),
                ..Default::default()
            },
        )
        .await
        .expect("trigger stream loop");
        assert_eq!(ticks.load(Ordering::SeqCst), 2);

        let (cancel_tx, cancel_rx) = watch::channel(false);
        let ticks = Arc::new(AtomicUsize::new(0));
        let observed = Arc::clone(&ticks);
        run_sse_stream_loop(
            cancel_rx,
            SseStreamLoopOptions {
                write_interval: Some(std::time::Duration::from_millis(5)),
                on_tick: Some(Box::new(move || {
                    let observed = Arc::clone(&observed);
                    let cancel_tx = cancel_tx.clone();
                    Box::pin(async move {
                        observed.fetch_add(1, Ordering::SeqCst);
                        cancel_tx.send(true).expect("cancel");
                        Ok(())
                    }) as SseLoopFuture
                })),
                ..Default::default()
            },
        )
        .await
        .expect("ticker stream loop");
        assert_eq!(ticks.load(Ordering::SeqCst), 1);
    }

    #[tokio::test]
    async fn stream_loop_ignores_trigger_without_callback() {
        // Parity: internal/api/httpserver/sse_boundaries_test.go:46 TestRunSSEStreamLoopHandlesTriggerWithoutCallback
        let (cancel_tx, cancel_rx) = watch::channel(false);
        let (trigger_tx, trigger_rx) = tokio::sync::mpsc::channel(1);
        trigger_tx.send(()).await.expect("trigger");
        let canceller = tokio::spawn(async move {
            tokio::time::sleep(std::time::Duration::from_millis(5)).await;
            cancel_tx.send(true).expect("cancel");
        });
        run_sse_stream_loop(
            cancel_rx,
            SseStreamLoopOptions {
                trigger: Some(trigger_rx),
                ..Default::default()
            },
        )
        .await
        .expect("stream loop");
        canceller.await.expect("canceller");
    }

    #[tokio::test]
    async fn stream_loop_propagates_trigger_and_ticker_failures() {
        // Parity: internal/api/httpserver/sse_boundaries_test.go:60 TestRunSSEStreamLoopPropagatesTriggerAndTickerFailures
        let (_cancel_tx, cancel_rx) = watch::channel(false);
        let (trigger_tx, trigger_rx) = tokio::sync::mpsc::channel(1);
        trigger_tx.send(()).await.expect("trigger");
        let error = run_sse_stream_loop(
            cancel_rx,
            SseStreamLoopOptions {
                on_tick: Some(Box::new(|| {
                    Box::pin(async { Err(loop_error("stream write failed")) }) as SseLoopFuture
                })),
                trigger: Some(trigger_rx),
                ..Default::default()
            },
        )
        .await
        .expect_err("trigger failure");
        assert_eq!(error.to_string(), "stream write failed");

        let (_cancel_tx, cancel_rx) = watch::channel(false);
        let error = run_sse_stream_loop(
            cancel_rx,
            SseStreamLoopOptions {
                write_interval: Some(std::time::Duration::from_millis(1)),
                on_tick: Some(Box::new(|| {
                    Box::pin(async { Err(loop_error("stream write failed")) }) as SseLoopFuture
                })),
                ..Default::default()
            },
        )
        .await
        .expect_err("ticker failure");
        assert_eq!(error.to_string(), "stream write failed");
    }

    #[tokio::test]
    async fn stream_loop_handles_nil_ticker_channel() {
        // Parity: internal/api/httpserver/sse_test.go:196 TestTickerCHandlesNilTicker
        // A loop without trigger/interval must never tick; only cancellation resolves it.
        let (cancel_tx, cancel_rx) = watch::channel(false);
        let ticks = Arc::new(AtomicUsize::new(0));
        let observed = Arc::clone(&ticks);
        let canceller = tokio::spawn(async move {
            tokio::time::sleep(std::time::Duration::from_millis(5)).await;
            cancel_tx.send(true).expect("cancel");
        });
        run_sse_stream_loop(
            cancel_rx,
            SseStreamLoopOptions {
                on_tick: Some(Box::new(move || {
                    observed.fetch_add(1, Ordering::SeqCst);
                    Box::pin(async { Ok(()) }) as SseLoopFuture
                })),
                ..Default::default()
            },
        )
        .await
        .expect("nil ticker loop");
        canceller.await.expect("canceller");
        assert_eq!(ticks.load(Ordering::SeqCst), 0);

        // Go only arms a ticker when WriteInterval > 0 AND OnTick != nil.
        let (cancel_tx, cancel_rx) = watch::channel(false);
        let canceller = tokio::spawn(async move {
            tokio::time::sleep(std::time::Duration::from_millis(5)).await;
            cancel_tx.send(true).expect("cancel");
        });
        run_sse_stream_loop(
            cancel_rx,
            SseStreamLoopOptions {
                write_interval: Some(std::time::Duration::from_millis(1)),
                ..Default::default()
            },
        )
        .await
        .expect("interval without callback");
        canceller.await.expect("canceller");
    }

    #[tokio::test]
    async fn write_event_propagates_serialization_and_write_failures() {
        // Parity: internal/api/httpserver/sse_boundaries_test.go:30 TestSSEWriterPropagatesSerializationAndWriteFailures
        struct Unserializable;

        impl Serialize for Unserializable {
            fn serialize<S: serde::Serializer>(&self, _serializer: S) -> Result<S::Ok, S::Error> {
                Err(serde::ser::Error::custom("unsupported value"))
            }
        }

        let writer = SseWriter::new(BufferedSseSink::new(), SSE_RETRY_MILLIS);
        assert!(writer.write_event(&Unserializable).is_err());
        assert!(writer.write_event_id("evt-1", &Unserializable).is_err());

        let writer = SseWriter::new(FailingSink(io::Error::other("client disconnected")), 3000);
        let error = writer.write_comment("heartbeat").expect_err("write error");
        assert_eq!(error.to_string(), "client disconnected");
    }
}
