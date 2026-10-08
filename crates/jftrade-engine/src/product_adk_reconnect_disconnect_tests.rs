use super::*;
use crate::product::AdkReadSnapshotPort;
use std::io;
use std::net::SocketAddr;
use std::pin::Pin;
use std::sync::Mutex;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::task::{Context, Poll};
use tokio::io::{AsyncRead, AsyncReadExt, AsyncWrite, AsyncWriteExt, ReadBuf};
use tokio::net::{TcpListener, TcpStream};
use tokio::sync::oneshot;

#[path = "product_adk_retained_stream_readiness_tests.rs"]
mod retained_readiness;

#[path = "product_adk_pre_run_stream_tests.rs"]
mod pre_run;

#[path = "product_adk_durable_identity_preflight_tests.rs"]
mod durable_identity;

#[path = "product_adk_canonical_identity_owner_tests.rs"]
mod canonical_identity;

// Inject only the socket failure. Routes, cursor projection, body polling and
// connection cleanup all run through the prepared production router/Hyper.
struct FailingListener {
    socket: TcpListener,
    needle: &'static [u8],
    failures: Arc<AtomicUsize>,
    written: Arc<Mutex<Vec<u8>>>,
    dropped: Option<oneshot::Sender<()>>,
}

impl axum::serve::Listener for FailingListener {
    type Io = FailingConnection;
    type Addr = SocketAddr;

    async fn accept(&mut self) -> (Self::Io, Self::Addr) {
        let (socket, address) = self.socket.accept().await.unwrap();
        (
            FailingConnection {
                socket,
                needle: self.needle,
                failures: self.failures.clone(),
                written: self.written.clone(),
                dropped: self.dropped.take(),
            },
            address,
        )
    }

    fn local_addr(&self) -> io::Result<Self::Addr> {
        self.socket.local_addr()
    }
}

struct FailingConnection {
    socket: TcpStream,
    needle: &'static [u8],
    failures: Arc<AtomicUsize>,
    written: Arc<Mutex<Vec<u8>>>,
    dropped: Option<oneshot::Sender<()>>,
}

impl Drop for FailingConnection {
    fn drop(&mut self) {
        if let Some(dropped) = self.dropped.take() {
            let _ = dropped.send(());
        }
    }
}

impl AsyncRead for FailingConnection {
    fn poll_read(
        mut self: Pin<&mut Self>,
        context: &mut Context<'_>,
        buffer: &mut ReadBuf<'_>,
    ) -> Poll<io::Result<()>> {
        Pin::new(&mut self.socket).poll_read(context, buffer)
    }
}

impl AsyncWrite for FailingConnection {
    fn poll_write(
        mut self: Pin<&mut Self>,
        context: &mut Context<'_>,
        buffer: &[u8],
    ) -> Poll<io::Result<usize>> {
        let failure_at = buffer
            .windows(self.needle.len())
            .position(|part| part == self.needle);
        if failure_at == Some(0) {
            self.failures.fetch_add(1, Ordering::SeqCst);
            return Poll::Ready(Err(io::Error::new(
                io::ErrorKind::BrokenPipe,
                "client disconnected",
            )));
        }
        // Headers and any earlier successful frame reach the client first,
        // even when Hyper coalesces them with the frame that must fail.
        let prefix = &buffer[..failure_at.unwrap_or(buffer.len())];
        match Pin::new(&mut self.socket).poll_write(context, prefix) {
            Poll::Ready(Ok(count)) => {
                self.written
                    .lock()
                    .unwrap()
                    .extend_from_slice(&prefix[..count]);
                Poll::Ready(Ok(count))
            }
            result => result,
        }
    }

    fn poll_flush(mut self: Pin<&mut Self>, context: &mut Context<'_>) -> Poll<io::Result<()>> {
        Pin::new(&mut self.socket).poll_flush(context)
    }

    fn poll_shutdown(mut self: Pin<&mut Self>, context: &mut Context<'_>) -> Poll<io::Result<()>> {
        Pin::new(&mut self.socket).poll_shutdown(context)
    }
}

async fn prepared_router(port: Arc<ProductionAdkPort>) -> crate::product::PreparedProduct {
    prepared_router_with_chat(port, false).await
}

async fn prepared_router_with_chat(
    port: Arc<ProductionAdkPort>,
    include_chat: bool,
) -> crate::product::PreparedProduct {
    let mut config = crate::product::ProductConfig::test_cutover(
        "127.0.0.1:0".parse().unwrap(),
        &port.settings_path,
    )
    .unwrap()
    .with_adk_read_snapshot_port(port.clone());
    if include_chat {
        config = config.with_adk_chat_stream_port(port);
    }
    let runtime = crate::product_runtime::ProductRuntimeState::product_only(&config);
    crate::product::prepare_product_with_runtime_state(config, runtime, None)
        .await
        .unwrap()
}

async fn fail_reconnect_write(port: Arc<ProductionAdkPort>, path: &str, needle: &'static [u8]) {
    fail_stream_write(port, path, None, needle, Some("stream-cursor")).await;
}

async fn fail_stream_write(
    port: Arc<ProductionAdkPort>,
    path: &str,
    body: Option<&[u8]>,
    needle: &'static [u8],
    expected_stream_id: Option<&str>,
) -> Option<String> {
    let prepared = prepared_router_with_chat(port.clone(), body.is_some()).await;
    let socket = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let address = socket.local_addr().unwrap();
    let failures = Arc::new(AtomicUsize::new(0));
    let written = Arc::new(Mutex::new(Vec::new()));
    let (dropped_tx, dropped_rx) = oneshot::channel();
    let (stop_tx, stop_rx) = oneshot::channel();
    let baseline = Arc::strong_count(&port.store);
    let listener = FailingListener {
        socket,
        needle,
        failures: failures.clone(),
        written: written.clone(),
        dropped: Some(dropped_tx),
    };
    let server = tokio::spawn(
        axum::serve(listener, prepared.router.clone())
            .with_graceful_shutdown(async {
                let _ = stop_rx.await;
            })
            .into_future(),
    );
    let mut client = TcpStream::connect(address).await.unwrap();
    let method = if body.is_some() { "POST" } else { "GET" };
    let payload = body.unwrap_or_default();
    client
        .write_all(
            format!("{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n", payload.len())
                .as_bytes(),
        )
        .await
        .unwrap();
    client.write_all(payload).await.unwrap();
    let mut response = Vec::new();
    tokio::time::timeout(Duration::from_secs(3), client.read_to_end(&mut response))
        .await
        .unwrap()
        .unwrap();
    tokio::time::timeout(Duration::from_secs(3), dropped_rx)
        .await
        .unwrap()
        .unwrap();
    stop_tx.send(()).unwrap();
    tokio::time::timeout(Duration::from_secs(3), server)
        .await
        .unwrap()
        .unwrap()
        .unwrap();
    assert_eq!(
        failures.load(Ordering::SeqCst),
        1,
        "a failed frame is never retried"
    );
    assert_eq!(response, *written.lock().unwrap());
    let wire = String::from_utf8(response).unwrap();
    let (headers, body) = wire.split_once("\r\n\r\n").unwrap();
    assert!(headers.starts_with("HTTP/1.1 200"), "{headers}");
    assert!(
        headers.contains("content-type: text/event-stream"),
        "{headers}"
    );
    match expected_stream_id {
        Some(stream_id) => assert!(
            headers.contains(&format!("x-adk-stream-id: {stream_id}")),
            "{headers}"
        ),
        None => assert!(!headers.contains("x-adk-stream-id:"), "{headers}"),
    }
    assert!(
        !body.contains(std::str::from_utf8(needle).unwrap()),
        "failed frame reached client: {body}"
    );
    if needle == b"data: " {
        assert!(
            body.contains("retry: 3000\n\n"),
            "retry must precede failed replay: {body}"
        );
    } else {
        assert!(
            !body.contains("data: "),
            "a frame followed the failed retry: {body}"
        );
    }
    assert_eq!(
        Arc::strong_count(&port.store),
        baseline,
        "failed HTTP body must release the production reader"
    );
    headers.lines().find_map(|line| {
        line.strip_prefix("x-adk-stream-id: ")
            .map(|value| value.trim().to_owned())
    })
}

// Parity: go:452dea11:internal/api/assistant/chat_transport_disconnect_test.go:55 TestChatStreamTransportHandlesDisconnectedClients
#[tokio::test]
async fn production_unavailable_chat_retains_terminal_error_after_retry_disconnect() {
    let (_directory, port) = reconnect_port();
    assert!(port.chat_runtime.is_none());
    let runs = port.store.list_runs().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let payload =
        br#"{"clientRequestId":"11111111-1111-4111-8111-111111111111","message":"hello"}"#;
    let stream_id = fail_stream_write(
        port.clone(),
        "/api/v1/adk/chat/stream",
        Some(payload),
        b"retry: 3000",
        Some("stream-"),
    )
    .await
    .expect("stream identity assigned before failed retry");
    let path = format!("/api/v1/adk/streams/{stream_id}");
    let replay = port.open_stream(&path, "").unwrap().unwrap();
    assert_eq!(
        replay.headers,
        vec![("X-ADK-Stream-ID".to_owned(), stream_id.clone())]
    );
    let mut reader = replay.body.take_body().unwrap();
    assert_eq!(reader.next().await.unwrap().unwrap(), b"retry: 3000\n\n");
    let frame = String::from_utf8(reader.next().await.unwrap().unwrap()).unwrap();
    assert!(frame.contains(&format!("id: {stream_id}:1\n")), "{frame}");
    let data: Value = serde_json::from_str(
        frame
            .lines()
            .find_map(|line| line.strip_prefix("data: "))
            .unwrap(),
    )
    .unwrap();
    assert_eq!(data["type"], "error");
    assert_eq!(data["streamId"], stream_id);
    assert_eq!(data["sequence"], 1);
    assert_eq!(data["replay"], true);
    assert!(!data["message"].as_str().unwrap().is_empty());
    assert!(data.get("runId").is_none());
    assert!(reader.next().await.is_none());
    let after = port.open_stream(&path, "after=1").unwrap().unwrap();
    let mut reader = after.body.take_body().unwrap();
    assert_eq!(reader.next().await.unwrap().unwrap(), b"retry: 3000\n\n");
    assert!(reader.next().await.is_none());
    assert_eq!(port.store.list_runs().unwrap(), runs);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
}

#[tokio::test]
async fn production_unavailable_chat_reuses_retained_error_over_http_and_rejects_conflicts() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port) = reconnect_port();
    let runs = port.store.list_runs().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let prepared = prepared_router_with_chat(port.clone(), true).await;
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let base = format!("http://{}", handle.startup_record().address);
    let client = reqwest::Client::new();
    let payload =
        json!({"clientRequestId":"11111111-1111-4111-8111-111111111111", "message":"hello"});
    let first = client
        .post(format!("{base}/api/v1/adk/chat/stream"))
        .json(&payload)
        .send()
        .await
        .unwrap();
    assert_eq!(first.status(), 200);
    assert_eq!(first.headers()["content-type"], "text/event-stream");
    let id = first.headers()["x-adk-stream-id"]
        .to_str()
        .unwrap()
        .to_owned();
    let initial = first.text().await.unwrap();
    assert!(initial.starts_with("retry: 3000\n\n"));
    assert!(initial.contains(&format!("id: {id}:1\n")));
    let mut event: Value = serde_json::from_str(
        initial
            .lines()
            .find_map(|line| line.strip_prefix("data: "))
            .unwrap(),
    )
    .unwrap();
    assert_eq!(event["type"], "error");
    assert!(!event["message"].as_str().unwrap().is_empty());
    assert!(event.get("replay").is_none());
    event["replay"] = json!(true);
    for response in [
        client
            .post(format!("{base}/api/v1/adk/chat/stream"))
            .json(&payload)
            .send()
            .await
            .unwrap(),
        client
            .get(format!("{base}/api/v1/adk/streams/{id}"))
            .send()
            .await
            .unwrap(),
    ] {
        assert_eq!(response.status(), 200);
        assert_eq!(response.headers()["x-adk-stream-id"], id);
        let body = response.text().await.unwrap();
        assert!(body.starts_with("retry: 3000\n\n"));
        let replay: Value = serde_json::from_str(
            body.lines()
                .find_map(|line| line.strip_prefix("data: "))
                .unwrap(),
        )
        .unwrap();
        assert_eq!(replay, event);
        assert_eq!(
            body.lines()
                .filter(|line| line.starts_with("data: "))
                .count(),
            1
        );
    }
    let after = client
        .get(format!("{base}/api/v1/adk/streams/{id}?after=1"))
        .send()
        .await
        .unwrap();
    assert_eq!(after.status(), 200);
    assert_eq!(after.text().await.unwrap(), "retry: 3000\n\n");
    let conflict = client
        .post(format!("{base}/api/v1/adk/chat/stream"))
        .json(&json!({"clientRequestId":payload["clientRequestId"], "message":"different"}))
        .send()
        .await
        .unwrap();
    assert_eq!(conflict.status(), 409);
    assert_eq!(
        conflict.json::<Value>().await.unwrap()["error"]["code"],
        "ADK_CHAT_IDEMPOTENCY_CONFLICT"
    );
    let chat = client
        .post(format!("{base}/api/v1/adk/chat"))
        .json(&payload)
        .send()
        .await
        .unwrap();
    assert_eq!(chat.status(), 503);
    assert_eq!(
        chat.json::<Value>().await.unwrap()["error"]["code"],
        "ADK_UNAVAILABLE"
    );
    let missing = client
        .get(format!("{base}/api/v1/adk/runs/{id}/stream"))
        .send()
        .await
        .unwrap();
    assert_eq!(missing.status(), 404);
    handle.shutdown().await.unwrap();
    assert_eq!(port.store.list_runs().unwrap(), runs);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
}

// Parity: go:452dea11:internal/api/assistant/chat_transport_disconnect_test.go:55 TestChatStreamTransportHandlesDisconnectedClients
#[tokio::test]
async fn production_malformed_chat_stops_after_the_first_retry_write_failure() {
    let (_directory, port) = reconnect_port();
    assert!(port.chat_runtime.is_none());
    let before = port.store.list_runs().unwrap();
    fail_stream_write(
        port.clone(),
        "/api/v1/adk/chat/stream",
        Some(br#"{"message":"#),
        b"retry: 3000",
        None,
    )
    .await;
    assert_eq!(port.store.list_runs().unwrap(), before);
}

// Parity: go:452dea11:internal/api/assistant/chat_transport_disconnect_test.go:110 TestChatStreamReconnectAndReplayRespectClientDisconnect
#[tokio::test]
async fn production_reconnect_retry_and_replay_write_failures_preserve_terminal_history() {
    let (_directory, port) = reconnect_port();
    seed_reconnect(
        &port,
        "COMPLETED",
        vec![json!({"type":"final", "sequence":1, "runId":"run-cursor"})],
    );
    let before = port.store.get_run("run-cursor").unwrap().unwrap();
    for path in [
        "/api/v1/adk/streams/stream-cursor",
        "/api/v1/adk/runs/run-cursor/stream",
    ] {
        fail_reconnect_write(port.clone(), path, b"retry: 3000").await;
        fail_reconnect_write(port.clone(), path, b"data: ").await;
        assert_eq!(port.store.get_run("run-cursor").unwrap().unwrap(), before);
        let replay = crate::product::AdkReadSnapshotPort::open_stream(port.as_ref(), path, "")
            .unwrap()
            .unwrap();
        let mut body = replay.body.take_body().unwrap();
        assert_eq!(body.next().await.unwrap().unwrap(), b"retry: 3000\n\n");
        let final_frame = String::from_utf8(body.next().await.unwrap().unwrap()).unwrap();
        assert!(final_frame.contains("\"type\":\"final\""), "{final_frame}");
        assert!(final_frame.contains("\"replay\":true"), "{final_frame}");
        assert!(body.next().await.is_none());
    }
}

// Parity: go:452dea11:internal/api/assistant/chat_transport_disconnect_test.go:110 TestChatStreamReconnectAndReplayRespectClientDisconnect
#[tokio::test]
async fn production_reconnect_precancelled_listener_writes_no_body_or_run_changes() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port) = reconnect_port();
    seed_reconnect(&port, "RUNNING", Vec::new());
    let before = port.store.get_run("run-cursor").unwrap().unwrap();
    let prepared = prepared_router(port.clone()).await;
    let (cancel, cancelled) = tokio::sync::watch::channel(false);
    cancel.send(true).unwrap();
    let router = jftrade_api::with_listener_shutdown(prepared.router.clone(), cancelled);
    let socket = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let address = socket.local_addr().unwrap();
    let (stop, stopped) = oneshot::channel();
    let server = tokio::spawn(
        axum::serve(socket, router)
            .with_graceful_shutdown(async {
                let _ = stopped.await;
            })
            .into_future(),
    );
    for path in [
        "/api/v1/adk/streams/stream-cursor",
        "/api/v1/adk/runs/run-cursor/stream",
    ] {
        let response = tokio::time::timeout(
            Duration::from_secs(3),
            reqwest::get(format!("http://{address}{path}")),
        )
        .await
        .unwrap()
        .unwrap();
        assert_eq!(response.status(), 503);
        assert!(response.bytes().await.unwrap().is_empty());
    }
    stop.send(()).unwrap();
    tokio::time::timeout(Duration::from_secs(3), server)
        .await
        .unwrap()
        .unwrap()
        .unwrap();
    assert_eq!(port.store.get_run("run-cursor").unwrap().unwrap(), before);
}

// Parity: go:452dea11:internal/assistant/service_test.go:49 TestAgentTemplatesAvailableWithoutRuntime
#[tokio::test]
async fn production_agent_templates_remain_available_without_chat_runtime() {
    let _ = rustls::crypto::ring::default_provider().install_default();
    let (_directory, port) = reconnect_port();
    assert!(
        port.chat_runtime.is_none(),
        "exercise the absent runtime branch"
    );
    let prepared = prepared_router(port).await;
    // Call the production projection owner: rehearsal HTTP dispatch alone
    // would only prove availability of the static compatibility template.
    assert!(prepared.api.adk_chat_stream_port.is_none());
    let jftrade_api::ApiOutput::Json(templates) =
        prepared.api.production_agent_templates().unwrap()
    else {
        panic!("production template owner must return JSON")
    };
    assert!(
        templates["templates"].is_array(),
        "nonnull templates: {templates}"
    );
    let handle = crate::product::expose_prepared_product(prepared).unwrap();
    let response = reqwest::get(format!(
        "http://{}/api/v1/adk/agent-templates",
        handle.startup_record().address
    ))
    .await
    .unwrap();
    assert_eq!(response.status(), 200);
    let envelope: Value = response.json().await.unwrap();
    assert_eq!(envelope["ok"], true);
    assert!(
        envelope["data"]["templates"].is_array(),
        "templates must be nonnull: {envelope}"
    );
    handle.shutdown().await.unwrap();
}
