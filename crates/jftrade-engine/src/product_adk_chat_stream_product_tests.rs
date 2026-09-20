use super::*;
use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute, AdkChatStreamFrame,
    AdkChatStreamPort, AdkChatStreamSnapshot,
};
use serde_json::{Value, json};
use std::collections::{BTreeMap, VecDeque};
use std::net::SocketAddr;
use std::sync::{Arc, Mutex};
use tempfile::tempdir;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;

#[derive(Debug)]
struct FixtureAdkChatPort;

impl AdkChatStreamPort for FixtureAdkChatPort {
    fn dispatch(
        &self,
        route: AdkChatRoute,
        _input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        match route {
            AdkChatRoute::Chat => Ok(AdkChatPortOutput::Json(json!({
                "run": {"id": "run-fixture"},
                "message": "fixture response"
            }))),
            AdkChatRoute::Stream => Ok(AdkChatPortOutput::Stream(AdkChatStreamSnapshot {
                headers: BTreeMap::from([(
                    "X-ADK-Stream-ID".to_owned(),
                    "stream-fixture".to_owned(),
                )]),
                frames: vec![AdkChatStreamFrame::Event {
                    id: Some("1".to_owned()),
                    data: json!({"type": "final", "message": "fixture response"}),
                }],
                terminal: true,
            })),
        }
    }
}

#[derive(Debug)]
struct SequencedAdkChatPort {
    responses: Mutex<VecDeque<Result<AdkChatPortOutput, AdkChatPortError>>>,
}

impl SequencedAdkChatPort {
    fn new(
        responses: impl IntoIterator<Item = Result<AdkChatPortOutput, AdkChatPortError>>,
    ) -> Self {
        Self {
            responses: Mutex::new(responses.into_iter().collect()),
        }
    }
}

impl AdkChatStreamPort for SequencedAdkChatPort {
    fn dispatch(
        &self,
        _route: AdkChatRoute,
        _input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        self.responses
            .lock()
            .expect("ADK chat sequence lock")
            .pop_front()
            .expect("ADK chat fixture response")
    }
}

#[derive(Debug)]
struct RetainedAdkReplayPort {
    events: Mutex<Vec<AdkReadEvent>>,
}

impl RetainedAdkReplayPort {
    fn new() -> Self {
        Self {
            events: Mutex::new(Vec::new()),
        }
    }

    fn terminal_events(&self) -> Vec<AdkReadEvent> {
        vec![
            AdkReadEvent {
                id: Some("stream-fixture:1".to_owned()),
                data: json!({"type": "run", "status": "RUNNING"}),
            },
            AdkReadEvent {
                id: Some("stream-fixture:2".to_owned()),
                data: json!({"type": "timeline", "text": "ok"}),
            },
            AdkReadEvent {
                id: Some("stream-fixture:3".to_owned()),
                data: json!({"type": "final", "status": "COMPLETED"}),
            },
        ]
    }

    fn retained_event_count(&self) -> usize {
        self.events.lock().expect("ADK replay event lock").len()
    }
}

impl AdkChatStreamPort for RetainedAdkReplayPort {
    fn dispatch(
        &self,
        route: AdkChatRoute,
        _input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        match route {
            AdkChatRoute::Chat => Ok(AdkChatPortOutput::Json(json!({
                "run": {"id": "run-fixture"},
                "message": "fixture response"
            }))),
            AdkChatRoute::Stream => {
                let events = self.terminal_events();
                *self.events.lock().expect("ADK replay event lock") = events.clone();
                std::thread::sleep(std::time::Duration::from_millis(100));
                Ok(AdkChatPortOutput::Stream(AdkChatStreamSnapshot {
                    headers: BTreeMap::from([(
                        "X-ADK-Stream-ID".to_owned(),
                        "stream-fixture".to_owned(),
                    )]),
                    frames: events
                        .into_iter()
                        .map(|event| AdkChatStreamFrame::Event {
                            id: event.id,
                            data: event.data,
                        })
                        .collect(),
                    terminal: true,
                }))
            }
        }
    }
}

impl AdkReadSnapshotPort for RetainedAdkReplayPort {
    fn read(&self, path: &str, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        if path != "/api/v1/adk/streams/stream-fixture" {
            return Err(AdkReadSnapshotError::Unavailable(
                "retained replay route mismatch".to_owned(),
            ));
        }
        let after = query
            .strip_prefix("after=")
            .and_then(|value| value.parse::<u64>().ok())
            .unwrap_or_default();
        let events = self
            .events
            .lock()
            .expect("ADK replay event lock")
            .iter()
            .filter(|event| {
                event
                    .id
                    .as_deref()
                    .and_then(|id| id.rsplit_once(':'))
                    .and_then(|(_, sequence)| sequence.parse::<u64>().ok())
                    .is_some_and(|sequence| sequence > after)
            })
            .cloned()
            .collect();
        Ok(AdkReadSnapshot::Stream(AdkReadStream {
            headers: vec![("X-ADK-Stream-ID".to_owned(), "stream-fixture".to_owned())],
            events,
        }))
    }
}

#[derive(Debug)]
struct NonTerminalAdkReplayPort;

impl AdkChatStreamPort for NonTerminalAdkReplayPort {
    fn dispatch(
        &self,
        route: AdkChatRoute,
        _input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        match route {
            AdkChatRoute::Chat => Ok(AdkChatPortOutput::Json(json!({
                "run": {"id": "run-nonterminal"},
                "message": "fixture response"
            }))),
            AdkChatRoute::Stream => Ok(AdkChatPortOutput::Stream(AdkChatStreamSnapshot {
                headers: BTreeMap::from([(
                    "X-ADK-Stream-ID".to_owned(),
                    "stream-nonterminal".to_owned(),
                )]),
                frames: vec![AdkChatStreamFrame::Event {
                    id: Some("stream-nonterminal:1".to_owned()),
                    data: json!({"type": "timeline", "status": "streaming", "text": "partial"}),
                }],
                terminal: false,
            })),
        }
    }
}

#[tokio::test]
async fn adk_chat_stream_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_chat_stream_port(Arc::new(FixtureAdkChatPort));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 50);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/adk/chat/stream" })
    );

    let chat = request_raw(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/chat",
        br#"{"clientRequestId":"11111111-1111-4111-8111-111111111111","message":"hello"}"#,
    )
    .await;
    assert_eq!(chat.status, 200);
    assert_eq!(
        chat.headers["content-type"],
        "application/json; charset=utf-8"
    );
    let chat_body: Value = serde_json::from_slice(&chat.body).expect("chat JSON");
    assert_eq!(chat_body["ok"], true);
    assert_eq!(chat_body["data"]["run"]["id"], "run-fixture");

    let stream = request_raw(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/chat/stream",
        br#"{"clientRequestId":"11111111-1111-4111-8111-111111111111","message":"hello"}"#,
    )
    .await;
    assert_eq!(stream.status, 200);
    assert_eq!(stream.headers["content-type"], "text/event-stream");
    assert_eq!(stream.headers["x-adk-stream-id"], "stream-fixture");
    assert_eq!(stream.headers["x-adk-stream-idle-timeout-ms"], "300000");
    let stream_body = String::from_utf8(stream.body).expect("SSE body");
    assert!(stream_body.starts_with("retry: 3000\n\n"));
    assert!(stream_body.contains("id: 1\n"));
    assert!(stream_body.contains("data: "));
    assert!(stream_body.contains("\"type\":\"final\""));
    assert!(stream_body.contains("\"message\":\"fixture response\""));
    assert_eq!(stream.headers["x-adk-stream-id"], "stream-fixture");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn adk_chat_stream_routes_are_isolated_without_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    assert!(!handle.startup_record().capabilities.iter().any(|route| {
        route == "POST /api/v1/adk/chat" || route == "POST /api/v1/adk/chat/stream"
    }));
    let response = request_raw(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/chat",
        br#"{"clientRequestId":"11111111-1111-4111-8111-111111111111"}"#,
    )
    .await;
    assert_eq!(response.status, 404);
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn adk_chat_stream_replays_retained_terminal_events_through_adk_read_after_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{\"seed\":\"adk-replay\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read settings");
    let replay_port = Arc::new(RetainedAdkReplayPort::new());
    let chat_port: Arc<dyn AdkChatStreamPort> = replay_port.clone();
    let read_port: Arc<dyn AdkReadSnapshotPort> = replay_port.clone();
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_chat_stream_port(Arc::clone(&chat_port))
            .with_adk_read_snapshot_port(Arc::clone(&read_port));
    let handle = start_product(config).await.expect("start product");
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/adk/chat/stream" })
    );
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "GET /api/v1/adk/streams/{streamId}" })
    );

    // Parity: go:452dea11:internal/api/assistant/chat_transport_disconnect_test.go:55 TestChatStreamTransportHandlesDisconnectedClients
    // Parity: go:452dea11:internal/api/assistant/chat_transport_disconnect_test.go:110 TestChatStreamReconnectAndReplayRespectClientDisconnect
    // Verifies valid chat stream keeps terminal execution state after client disconnect and respects reconnection/replay filtering
    let disconnected_client = send_request_without_reading(
        handle.startup_record().address,
        "POST",
        ADK_CHAT_STREAM_PATH,
        br#"{"clientRequestId":"11111111-1111-4111-8111-111111111111","message":"disconnect"}"#,
    )
    .await;
    for _ in 0..50 {
        if replay_port.retained_event_count() == 3 {
            break;
        }
        tokio::time::sleep(std::time::Duration::from_millis(10)).await;
    }
    assert_eq!(replay_port.retained_event_count(), 3);
    drop(disconnected_client);

    let replay = request_raw(
        handle.startup_record().address,
        "GET",
        "/api/v1/adk/streams/stream-fixture?after=1",
        &[],
    )
    .await;
    assert_eq!(replay.status, 200);
    assert_eq!(replay.headers["content-type"], "text/event-stream");
    let replay_body = String::from_utf8(replay.body).expect("replay body");
    assert!(!replay_body.contains("id: stream-fixture:1\n"));
    assert!(replay_body.contains("id: stream-fixture:2\n"));
    assert!(replay_body.contains("id: stream-fixture:3\n"));

    handle.shutdown().await.expect("shutdown product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after shutdown"),
        settings_before
    );

    let restarted = start_product(
        ProductConfig::test_cutover(
            "127.0.0.1:0".parse().expect("restarted address"),
            &settings_path,
        )
        .expect("restarted config")
        .with_adk_chat_stream_port(chat_port)
        .with_adk_read_snapshot_port(read_port),
    )
    .await
    .expect("restart product");
    let replay_after_restart = request_raw(
        restarted.startup_record().address,
        "GET",
        "/api/v1/adk/streams/stream-fixture?after=2",
        &[],
    )
    .await;
    assert_eq!(replay_after_restart.status, 200);
    let replay_after_restart_body =
        String::from_utf8(replay_after_restart.body).expect("replay after restart body");
    assert!(!replay_after_restart_body.contains("id: stream-fixture:2\n"));
    assert!(replay_after_restart_body.contains("id: stream-fixture:3\n"));
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after restart"),
        settings_before
    );
}

#[tokio::test]
async fn adk_chat_stream_replays_nonterminal_snapshot_without_fabricating_terminal_event() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_chat_stream_port(Arc::new(NonTerminalAdkReplayPort));
    let handle = start_product(config).await.expect("start product");
    let response = request_raw(
        handle.startup_record().address,
        "POST",
        ADK_CHAT_STREAM_PATH,
        br#"{"clientRequestId":"11111111-1111-4111-8111-111111111111","message":"replay"}"#,
    )
    .await;
    assert_eq!(response.status, 200);
    assert_eq!(response.headers["content-type"], "text/event-stream");
    assert_eq!(response.headers["x-adk-stream-id"], "stream-nonterminal");
    let body = String::from_utf8(response.body).expect("SSE body");
    assert!(body.starts_with("retry: 3000\n\n"));
    assert!(body.contains("streaming"));
    assert!(body.contains("partial"));
    assert!(!body.contains("\"type\":\"final\""));
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn adk_chat_stream_product_replays_browser_boundary_failure_recovery_and_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{\"seed\":\"adk-chat-stream\"}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read settings");
    let port = Arc::new(SequencedAdkChatPort::new([
        Err(AdkChatPortError::Unavailable(
            "fixture runtime unavailable".to_owned(),
        )),
        Ok(AdkChatPortOutput::Json(json!({
            "run": {"id": "run-fixture"},
            "message": "fixture response"
        }))),
        Err(AdkChatPortError::Failed {
            status: 502,
            code: "MODEL_CALL_FAILED".to_owned(),
            message: "fixture provider failed".to_owned(),
        }),
        Ok(AdkChatPortOutput::Stream(AdkChatStreamSnapshot {
            headers: BTreeMap::from([("X-ADK-Stream-ID".to_owned(), "stream-fixture".to_owned())]),
            frames: vec![AdkChatStreamFrame::Event {
                id: Some("1".to_owned()),
                data: json!({"type": "final", "message": "fixture response"}),
            }],
            terminal: true,
        })),
    ]));
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_chat_stream_port(port);
    config.access = AccessPolicy {
        session_token: Some("fixture-browser-session".to_owned()),
        csrf_token: Some("fixture-csrf".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://fixture.jftrade.local".to_owned()]);
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    let body = br#"{"clientRequestId":"11111111-1111-4111-8111-111111111111","message":"hello"}"#;
    let browser_headers = [
        ("Cookie", "jftrade_web_session=fixture-browser-session"),
        ("Origin", "https://fixture.jftrade.local"),
        ("Referer", "https://fixture.jftrade.local/adk"),
        ("X-CSRF-Token", "fixture-csrf"),
        ("X-Request-ID", "adk-chat-stream-fixture"),
    ];

    let unauthorized = request_raw_with_headers(address, "POST", ADK_CHAT_PATH, body, &[]).await;
    assert_eq!(unauthorized.status, 401);

    let csrf_missing = request_raw_with_headers(
        address,
        "POST",
        ADK_CHAT_PATH,
        body,
        &[
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
        ],
    )
    .await;
    assert_eq!(csrf_missing.status, 403);

    let unavailable =
        request_raw_with_headers(address, "POST", ADK_CHAT_PATH, body, &browser_headers).await;
    assert_eq!(unavailable.status, 503);
    let unavailable_body: Value =
        serde_json::from_slice(&unavailable.body).expect("unavailable JSON");
    assert_eq!(unavailable_body["error"]["code"], "ADK_UNAVAILABLE");

    let chat =
        request_raw_with_headers(address, "POST", ADK_CHAT_PATH, body, &browser_headers).await;
    assert_eq!(chat.status, 200);
    assert_eq!(
        chat.headers["content-type"],
        "application/json; charset=utf-8"
    );
    let chat_body: Value = serde_json::from_slice(&chat.body).expect("chat JSON");
    assert_eq!(chat_body["data"]["run"]["id"], "run-fixture");

    let stream_failure = request_raw_with_headers(
        address,
        "POST",
        ADK_CHAT_STREAM_PATH,
        body,
        &[
            ("Accept", "text/event-stream"),
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
            ("Referer", "https://fixture.jftrade.local/adk"),
            ("X-CSRF-Token", "fixture-csrf"),
            ("X-Request-ID", "adk-stream-failure"),
        ],
    )
    .await;
    assert_eq!(stream_failure.status, 502);
    assert_eq!(
        stream_failure.headers["x-adk-stream-idle-timeout-ms"],
        "300000"
    );
    let stream_failure_body: Value =
        serde_json::from_slice(&stream_failure.body).expect("stream failure JSON");
    assert_eq!(stream_failure_body["error"]["code"], "MODEL_CALL_FAILED");

    let stream = request_raw_with_headers(
        address,
        "POST",
        ADK_CHAT_STREAM_PATH,
        body,
        &[
            ("Accept", "text/event-stream"),
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
            ("Origin", "https://fixture.jftrade.local"),
            ("Referer", "https://fixture.jftrade.local/adk"),
            ("X-CSRF-Token", "fixture-csrf"),
            ("X-Request-ID", "adk-stream-success"),
        ],
    )
    .await;
    assert_eq!(stream.status, 200);
    assert_eq!(stream.headers["content-type"], "text/event-stream");
    assert_eq!(stream.headers["cache-control"], "no-cache");
    assert_eq!(stream.headers["connection"], "keep-alive");
    assert_eq!(stream.headers["x-adk-stream-id"], "stream-fixture");
    assert_eq!(stream.headers["x-adk-stream-idle-timeout-ms"], "300000");
    assert_eq!(
        String::from_utf8(stream.body).expect("stream body"),
        "retry: 3000\n\nid: 1\ndata: {\"message\":\"fixture response\",\"type\":\"final\"}\n\n"
    );

    handle.shutdown().await.expect("shutdown product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after shutdown"),
        settings_before
    );

    let restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restarted address"),
        &settings_path,
    )
    .expect("restarted config")
    .with_adk_chat_stream_port(Arc::new(FixtureAdkChatPort));
    let restarted = start_product(restarted_config)
        .await
        .expect("restart product");
    let restarted_stream = request_raw_with_headers(
        restarted.startup_record().address,
        "POST",
        ADK_CHAT_STREAM_PATH,
        body,
        &[("Accept", "text/event-stream")],
    )
    .await;
    assert_eq!(restarted_stream.status, 200);
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after restart"),
        settings_before
    );
}

struct RawResponse {
    status: u16,
    headers: BTreeMap<String, String>,
    body: Vec<u8>,
}

async fn request_raw(address: SocketAddr, method: &str, path: &str, body: &[u8]) -> RawResponse {
    request_raw_with_headers(address, method, path, body, &[]).await
}

async fn request_raw_with_headers(
    address: SocketAddr,
    method: &str,
    path: &str,
    body: &[u8],
    extra_headers: &[(&str, &str)],
) -> RawResponse {
    let extra_headers = extra_headers
        .iter()
        .map(|(name, value)| format!("{name}: {value}\r\n"))
        .collect::<String>();
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect ADK product API");
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\n{extra_headers}Content-Length: {}\r\nConnection: keep-alive\r\n\r\n",
        body.len(),
    );
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write ADK request headers");
    stream
        .write_all(body)
        .await
        .expect("write ADK request body");
    let mut response = Vec::new();
    let separator = loop {
        if let Some(separator) = response.windows(4).position(|window| window == b"\r\n\r\n") {
            break separator;
        }
        let mut chunk = [0_u8; 4096];
        let count = stream
            .read(&mut chunk)
            .await
            .expect("read ADK response headers");
        assert!(count > 0, "ADK response ended before headers");
        response.extend_from_slice(&chunk[..count]);
    };
    let header_bytes = &response[..separator];
    let mut lines = header_bytes.split(|byte| *byte == b'\n');
    let status_line = lines.next().expect("status line");
    let status = String::from_utf8_lossy(status_line)
        .split_whitespace()
        .nth(1)
        .expect("status code")
        .parse()
        .expect("numeric status");
    let headers: BTreeMap<String, String> = lines
        .filter_map(|line| {
            let line = String::from_utf8_lossy(line);
            let (name, value) = line.trim().split_once(':')?;
            Some((name.trim().to_ascii_lowercase(), value.trim().to_owned()))
        })
        .collect();
    let content_length = headers
        .get("content-length")
        .expect("ADK response content length")
        .parse::<usize>()
        .expect("numeric ADK response content length");
    let body_start = separator + 4;
    let body_end = body_start + content_length;
    while response.len() < body_end {
        let mut chunk = [0_u8; 4096];
        let count = stream
            .read(&mut chunk)
            .await
            .expect("read ADK response body");
        assert!(count > 0, "ADK response ended before body");
        response.extend_from_slice(&chunk[..count]);
    }
    RawResponse {
        status,
        headers,
        body: response[body_start..body_end].to_vec(),
    }
}

async fn send_request_without_reading(
    address: SocketAddr,
    method: &str,
    path: &str,
    body: &[u8],
) -> TcpStream {
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect ADK product API");
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
        body.len(),
    );
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write disconnected ADK request headers");
    stream
        .write_all(body)
        .await
        .expect("write disconnected ADK request body");
    stream
        .flush()
        .await
        .expect("flush disconnected ADK request");
    stream
}

/// Read an SSE response until `predicate` accepts the accumulated body (or the
/// deadline expires) and return both the response headers (lower-cased) and the
/// raw text.  Stream tests that must compare `X-ADK-Stream-ID` across a reconnect
/// need the header block, which the plain text variant discards.
async fn request_sse_raw_until(
    address: SocketAddr,
    method: &str,
    path: &str,
    body: &[u8],
    predicate: impl Fn(&str) -> bool,
) -> (BTreeMap<String, String>, String) {
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect ADK product API");
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nAccept: text/event-stream\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: keep-alive\r\n\r\n",
        body.len(),
    );
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write SSE request headers");
    stream
        .write_all(body)
        .await
        .expect("write SSE request body");
    let mut response = Vec::new();
    let deadline = tokio::time::Instant::now() + std::time::Duration::from_secs(15);
    loop {
        let text = String::from_utf8_lossy(&response).to_string();
        if predicate(&text) {
            return (parse_sse_headers(&text), text);
        }
        let remaining = deadline.saturating_duration_since(tokio::time::Instant::now());
        if remaining.is_zero() {
            return (parse_sse_headers(&text), text);
        }
        let mut chunk = [0_u8; 4096];
        match tokio::time::timeout(remaining, stream.read(&mut chunk)).await {
            Ok(Ok(0)) => {
                let text = String::from_utf8_lossy(&response).to_string();
                return (parse_sse_headers(&text), text);
            }
            Ok(Ok(count)) => response.extend_from_slice(&chunk[..count]),
            Ok(Err(error)) => panic!("read SSE body: {error}"),
            Err(_) => {
                let text = String::from_utf8_lossy(&response).to_string();
                return (parse_sse_headers(&text), text);
            }
        }
    }
}

/// Extract the lower-cased header map from a raw HTTP response prefix.
fn parse_sse_headers(response: &str) -> BTreeMap<String, String> {
    let head = response.split("\r\n\r\n").next().unwrap_or(response);
    head.lines()
        .skip(1)
        .filter_map(|line| line.split_once(':'))
        .map(|(name, value)| (name.trim().to_ascii_lowercase(), value.trim().to_owned()))
        .collect()
}

/// Read an SSE response until `predicate` accepts the accumulated body (or the
/// deadline expires), then return it.  Chunked-encoding bodies have no
/// `Content-Length`, so the raw helper above cannot be used here.
async fn request_sse_until(
    address: SocketAddr,
    method: &str,
    path: &str,
    body: &[u8],
    predicate: impl Fn(&str) -> bool,
) -> String {
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect ADK product API");
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: keep-alive\r\n\r\n",
        body.len(),
    );
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write SSE request headers");
    stream
        .write_all(body)
        .await
        .expect("write SSE request body");
    let mut response = Vec::new();
    let deadline = tokio::time::Instant::now() + std::time::Duration::from_secs(15);
    loop {
        let text = String::from_utf8_lossy(&response).to_string();
        if predicate(&text) {
            return text;
        }
        let remaining = deadline.saturating_duration_since(tokio::time::Instant::now());
        if remaining.is_zero() {
            return text;
        }
        let mut chunk = [0_u8; 4096];
        match tokio::time::timeout(remaining, stream.read(&mut chunk)).await {
            Ok(Ok(0)) => return String::from_utf8_lossy(&response).to_string(),
            Ok(Ok(count)) => response.extend_from_slice(&chunk[..count]),
            Ok(Err(error)) => panic!("read SSE body: {error}"),
            Err(_) => return String::from_utf8_lossy(&response).to_string(),
        }
    }
}

/// Parity: go:452dea11:internal/api/assistant/routes_test.go:301
/// TestChatRequestIdempotencyContracts.
///
/// Go requires the UUID identity rules to run before the runtime is touched
/// (missing/invalid `clientRequestId` -> `400 BAD_REQUEST` / "clientRequestId
/// must be a valid UUID"), a repeated identical body to replay the same durable
/// outcome without a second run, and a reused id whose payload changed to
/// answer `409 ADK_CHAT_IDEMPOTENCY_CONFLICT`.
#[tokio::test]
async fn adk_chat_idempotency_contract_matches_the_go_routes() {
    let endpoint = closed_model_endpoint();
    let (_directory, handle) = start_adk_product_with_loopback_provider(&endpoint).await;
    let address = handle.startup_record().address;
    let request_id = "44444444-4444-4444-8444-444444444444";

    // Missing and malformed identities are rejected before any run is created.
    for (body, label) in [
        (
            br#"{"agentId":"agent-live","message":"hello"}"#.to_vec(),
            "missing",
        ),
        (
            br#"{"clientRequestId":"invalid","agentId":"agent-live","message":"hello"}"#.to_vec(),
            "invalid",
        ),
    ] {
        let response = request_raw(address, "POST", ADK_CHAT_PATH, &body).await;
        assert_eq!(response.status, 400, "{label} clientRequestId status");
        let value: Value = serde_json::from_slice(&response.body).expect("identity error JSON");
        assert_eq!(value["error"]["code"], "BAD_REQUEST", "{label}");
        assert_eq!(
            value["error"]["message"], "clientRequestId must be a valid UUID",
            "{label}"
        );
    }

    // The same body replays the durable run; a changed message conflicts.
    let body = format!(
        r#"{{"clientRequestId":"{request_id}","agentId":"agent-live","message":"idempotent once"}}"#
    )
    .into_bytes();
    let first = request_raw(address, "POST", ADK_CHAT_PATH, &body).await;
    let replayed = request_raw(address, "POST", ADK_CHAT_PATH, &body).await;
    let conflict_body = format!(
        r#"{{"clientRequestId":"{request_id}","agentId":"agent-live","message":"changed"}}"#
    )
    .into_bytes();
    let conflict = request_raw(address, "POST", ADK_CHAT_PATH, &conflict_body).await;
    // The loopback model endpoint is closed, so the first call fails fast.
    // Go's `CompleteChatRun` treats that provider failure as a terminal run and
    // returns `ProjectedChatResponse`, so the handler already answers `200` with
    // a FAILED run.  A reused request is then served by `reusedChatResponse` ->
    // `ChatResponseForExistingRun`, which projects that same durable run instead
    // of creating a second one.
    assert_eq!(
        first.status, 200,
        "Go answers 200 with the failed-run projection for a provider failure"
    );
    let first_value: Value = serde_json::from_slice(&first.body).expect("first call JSON");
    assert_eq!(first_value["ok"], true);
    assert_eq!(first_value["data"]["run"]["status"], "FAILED");
    assert_eq!(first_value["data"]["run"]["errorCode"], "MODEL_CALL_FAILED");
    assert_eq!(replayed.status, 200, "an identical repeat replays the run");
    let first_final_message_id = first_value["data"]["run"]["finalMessageId"]
        .as_str()
        .expect("the failure projection links its synthetic reply")
        .to_owned();
    let replayed_final_message_id = {
        let value: Value = serde_json::from_slice(&replayed.body).expect("replay JSON");
        value["data"]["run"]["finalMessageId"]
            .as_str()
            .unwrap_or_default()
            .to_owned()
    };
    assert_eq!(
        replayed_final_message_id, first_final_message_id,
        "the replay must project the same terminal run, not a second one"
    );
    let replayed_value: Value = serde_json::from_slice(&replayed.body).expect("replay JSON");
    assert_eq!(replayed_value["ok"], true);
    assert_eq!(
        replayed_value["data"]["run"]["id"],
        format!("run-{request_id}"),
        "the replay must project the original run instead of creating a new one"
    );
    assert_eq!(conflict.status, 409, "changed payload on a reused id");
    let conflict_value: Value = serde_json::from_slice(&conflict.body).expect("conflict JSON");
    assert_eq!(
        conflict_value["error"]["code"],
        "ADK_CHAT_IDEMPOTENCY_CONFLICT"
    );

    // The stream route applies the same identity rules and reuses the same
    // stream id for a repeated body.
    let stream_request_id = "55555555-5555-4555-8555-555555555555";
    let stream_body = format!(
        r#"{{"clientRequestId":"{stream_request_id}","agentId":"agent-live","message":"stream once"}}"#
    )
    .into_bytes();
    let (stream_headers, _) = request_sse_raw_until(
        address,
        "POST",
        ADK_CHAT_STREAM_PATH,
        &stream_body,
        |text| text.contains("data: "),
    )
    .await;
    let stream_id = stream_headers
        .get("x-adk-stream-id")
        .cloned()
        .expect("first stream must publish X-ADK-Stream-ID");
    assert!(!stream_id.is_empty());
    let (replay_headers, _) = request_sse_raw_until(
        address,
        "POST",
        ADK_CHAT_STREAM_PATH,
        &stream_body,
        |text| text.contains("data: "),
    )
    .await;
    assert_eq!(
        replay_headers.get("x-adk-stream-id"),
        Some(&stream_id),
        "a reused stream request must keep the original stream id"
    );
    let stream_conflict_body = format!(
        r#"{{"clientRequestId":"{stream_request_id}","agentId":"agent-live","message":"changed"}}"#
    )
    .into_bytes();
    let stream_conflict =
        request_raw(address, "POST", ADK_CHAT_STREAM_PATH, &stream_conflict_body).await;
    assert_eq!(stream_conflict.status, 409, "stream conflict status");
    let stream_conflict_value: Value =
        serde_json::from_slice(&stream_conflict.body).expect("stream conflict JSON");
    assert_eq!(
        stream_conflict_value["error"]["code"],
        "ADK_CHAT_IDEMPOTENCY_CONFLICT"
    );

    handle.shutdown().await.expect("shutdown product");
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:325
/// `TestCompleteChatRunFailurePersistsUserFacingErrorReply` plus the frozen
/// `chat-provider-failure` case of
/// `tests/fixtures/compatibility/api-transport/adk-chat-stream.json`.
///
/// Go's `CompleteChatRun` treats a provider failure as a terminal run: it marks
/// the run FAILED, persists that state with an audit row, then attaches a
/// synthetic assistant message whose text is `userFacingADKError(adkErr)` and
/// links it through `run.finalMessageId`. The handler therefore answers
/// `200 ok=true` with a failed run plus `reply`, exactly as the frozen fixture
/// records for `chat-provider-failure`.
///
/// Rust used to keep a retryable provider outage durable (`RUNNING` +
/// `providerRetry`/`resumeState=provider_waiting`) and answer `502`. Go has no
/// `providerRetry`/`provider_waiting` concept at all, the OpenAPI contract for
/// `/api/v1/adk/chat` only declares `200`/`400`, and the route ledger's quirk
/// disposition is "Reproduce the 200 projection; do not fix the Go error
/// precedence". This regression pins the converged behaviour against a real
/// runtime and a real (closed) loopback provider.
#[tokio::test]
async fn production_chat_provider_failure_projects_go_failed_run_with_reply() {
    let endpoint = closed_model_endpoint();
    let (_directory, handle) = start_adk_product_with_loopback_provider(&endpoint).await;
    let address = handle.startup_record().address;
    let body = br#"{"clientRequestId":"11111111-1111-4111-8111-111111111111","agentId":"agent-live","message":"provider failure"}"#;

    let chat = request_raw(address, "POST", ADK_CHAT_PATH, body).await;
    assert_eq!(
        chat.status, 200,
        "Go answers 200 with a failed-run projection for a provider failure"
    );
    let chat_body: Value = serde_json::from_slice(&chat.body).expect("chat JSON");
    assert_eq!(chat_body["ok"], true);
    let run = &chat_body["data"]["run"];
    assert_eq!(
        run["status"], "FAILED",
        "provider failure is terminal: {run}"
    );
    assert_eq!(run["errorCode"], "MODEL_CALL_FAILED");
    assert_eq!(run["degraded"], true);
    assert!(
        run["completedAt"].is_string(),
        "completedAt must be stamped"
    );
    let final_message_id = run["finalMessageId"]
        .as_str()
        .filter(|value| !value.trim().is_empty())
        .unwrap_or_else(|| panic!("Go links the synthetic reply via finalMessageId: {run}"));
    let reply = chat_body["data"]["reply"]
        .as_str()
        .filter(|value| !value.trim().is_empty())
        .unwrap_or_else(|| panic!("Go projects the failure text as reply: {chat_body}"));
    assert!(
        run.get("providerRetry").is_none(),
        "a terminal failure must not keep a durable retry marker: {run}"
    );

    // The synthetic assistant message is a real transcript entry the timeline
    // can render, which is what `finalMessageId` points at.
    let timeline = chat_body["data"]["timeline"]
        .as_array()
        .expect("timeline array");
    assert!(
        timeline.iter().any(|entry| {
            entry["kind"] == "assistant_message"
                && entry["status"] == "final"
                && entry["text"].as_str() == Some(reply)
                && entry["id"].as_str() == Some(final_message_id)
        }),
        "the linked final message must appear on the timeline: {timeline:?}"
    );

    handle.shutdown().await.expect("shutdown product");
}

/// A real `ProductionAdkChatRuntime` backed by temporary ADK stores and a
/// loopback OpenAI-compatible model provider.
async fn start_adk_product_with_loopback_provider(
    endpoint: &str,
) -> (tempfile::TempDir, super::ProductHandle) {
    use crate::product::product_adk_model_runtime::{
        ProductionAdkChatRuntime, RunCancellationRegistry,
    };
    use jftrade_store_sqlite::initialize_current;

    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{}\n").expect("seed settings");
    for (path, component) in [
        (directory.path().join("adk.db"), "adk"),
        (directory.path().join("adk-session.db"), "adk-session"),
    ] {
        let connection = rusqlite::Connection::open(&path).expect("create ADK database");
        initialize_current(&connection, component).expect("initialize ADK schema");
    }
    let store = Arc::new(
        jftrade_store_sqlite::AdkStore::open(directory.path().join("adk.db"))
            .expect("open ADK store"),
    );
    let session_store = Arc::new(
        jftrade_store_sqlite::AdkSessionStore::open(directory.path().join("adk-session.db"))
            .expect("open ADK session store"),
    );
    store
        .upsert_provider(
            "provider-live",
            &json!({
                "id": "provider-live",
                "displayName": "Live Provider",
                "baseUrl": endpoint,
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "agent-live",
            &json!({
                "id": "agent-live",
                "name": "Live Agent",
                "providerId": "provider-live",
                "permissionMode": "all",
                "status": "ENABLED",
            })
            .to_string(),
        )
        .expect("persist agent");
    // `ProductionAdkChatRuntime::new` derives the provider secret store from
    // the settings path (`<settings dir>/[FUNC]/adk-[FUNC].json`).
    std::fs::create_dir_all(directory.path().join("secrets")).expect("create secrets directory");
    std::fs::write(
        directory.path().join("secrets/adk-secrets.json"),
        r#"{"provider-live":"sk-fixture"}"#,
    )
    .expect("write provider secrets");
    let runtime: Arc<dyn AdkChatStreamPort> = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        session_store,
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_chat_stream_port(runtime);
    let handle = start_product(config).await.expect("start product");
    (directory, handle)
}

/// A loopback AI Platform-compatible model endpoint that answers one
/// `response.output_text.delta` plus `response.completed`.  Serves exactly one
/// connection, then returns; the caller joins the thread.
fn spawn_loopback_model_provider() -> (String, std::thread::JoinHandle<()>) {
    let listener =
        std::net::TcpListener::bind("127.0.0.1:0").expect("bind loopback model provider");
    let address = listener.local_addr().expect("model provider address");
    let handle = std::thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept model request");
        // Drain the whole request before answering.  The production prompt
        // plus tool schemas exceed one TCP read, and closing early would make
        // the client observe a broken pipe instead of the SSE body.
        let mut request = Vec::new();
        let mut chunk = [0_u8; 4096];
        let mut expected = None;
        loop {
            let count = std::io::Read::read(&mut stream, &mut chunk).expect("read model request");
            if count == 0 {
                break;
            }
            request.extend_from_slice(&chunk[..count]);
            if expected.is_none()
                && let Some(headers_end) = request.windows(4).position(|w| w == b"\r\n\r\n")
            {
                let headers_end = headers_end + 4;
                let headers = String::from_utf8_lossy(&request[..headers_end]).to_ascii_lowercase();
                let length = headers
                    .lines()
                    .find_map(|line| line.strip_prefix("content-length:"))
                    .and_then(|value| value.trim().parse::<usize>().ok())
                    .unwrap_or_default();
                expected = Some(headers_end + length);
            }
            if expected.is_some_and(|expected| request.len() >= expected) {
                break;
            }
        }
        let body = concat!(
            "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello from loopback\"}\n\n",
            "data: {\"type\":\"response.completed\",\"response\":{\"text\":\"hello from loopback\"}}\n\n"
        );
        let response = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            body.len(),
            body
        );
        let _ = std::io::Write::write_all(&mut stream, response.as_bytes());
        let _ = std::io::Write::flush(&mut stream);
        // Give the client a moment to consume the SSE body before the socket
        // closes, then return so the caller can join this thread.
        std::thread::sleep(std::time::Duration::from_millis(200));
    });
    (
        format!("http://{}:{}/v1/responses", address.ip(), address.port()),
        handle,
    )
}

/// Parity: go:452dea11:internal/api/assistant/adk_routes_test.go:245
/// TestADKChatStreamEmitsSessionRunAndFinalEvents.
///
/// Go runs a real chat against a saved provider and requires the live SSE
/// response to carry `session`, `run` and `final` frames, with `final.response`
/// carrying the resolved agent and the configured `maxDurationMs`.  This drives
/// the real production runtime against a loopback model provider so the whole
/// provider call and terminal projection are exercised, not a fixture port.
#[tokio::test]
async fn production_live_chat_stream_emits_session_run_and_final_events() {
    let (endpoint, provider) = spawn_loopback_model_provider();
    let (_directory, handle) = start_adk_product_with_loopback_provider(&endpoint).await;
    let address = handle.startup_record().address;
    let body = br#"{"clientRequestId":"66666666-6666-4666-8666-666666666666","agentId":"agent-live","message":"hello"}"#;

    let text = request_sse_until(address, "POST", ADK_CHAT_STREAM_PATH, body, |text| {
        text.contains("\"type\":\"final\"") || text.contains("\"type\":\"error\"")
    })
    .await;
    provider.join().expect("loopback provider thread");

    for frame in [
        "\"type\":\"session\"",
        "\"type\":\"run\"",
        "\"type\":\"final\"",
    ] {
        assert!(
            text.contains(frame),
            "live stream must publish {frame}: {text}"
        );
    }
    let session_at = text.find("\"type\":\"session\"").expect("session frame");
    let run_at = text.find("\"type\":\"run\"").expect("run frame");
    let final_at = text.find("\"type\":\"final\"").expect("final frame");
    assert!(
        session_at < run_at && run_at < final_at,
        "session must precede run which must precede final: {text}"
    );
    // The terminal frame carries the projected response for the resolved agent.
    let final_frame = text
        .split("data: ")
        .map(|rest| rest.split('\n').next().unwrap_or_default())
        .filter_map(|data| serde_json::from_str::<Value>(data).ok())
        .find(|value| value["type"] == "final")
        .expect("final frame JSON");
    assert_eq!(
        final_frame["response"]["run"]["agentId"], "agent-live",
        "final frame must project the resolved agent"
    );
    assert!(
        final_frame["response"]["reply"]
            .as_str()
            .is_some_and(|reply| reply.contains("hello from loopback")),
        "final frame must carry the model reply: {final_frame}"
    );

    handle.shutdown().await.expect("shutdown product");
}

/// A closed loopback port used as the model endpoint.  The provider refuses the
/// request immediately, which keeps the assertion deterministic while still
/// driving the real `start_live_stream` path (no fixture port, no network).
fn closed_model_endpoint() -> String {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind probe port");
    let address = listener.local_addr().expect("probe address");
    drop(listener);
    format!("http://{}:{}/v1/responses", address.ip(), address.port())
}

/// Parity: go:452dea11:internal/api/assistant/routes_test.go:265 TestChatAndSSEContracts
///
/// Go's live `/api/v1/adk/chat/stream` publishes the preview `session` frame
/// before the `run` snapshot and before the model call.  The console binds the
/// transcript to the session id carried by frame one, so the ordering is a wire
/// contract.  This drives the real production runtime: the model endpoint is a
/// closed loopback port, so the run fails fast while the preview and run frames
/// have already been published.
#[tokio::test]
async fn production_live_chat_stream_emits_session_before_run_and_terminal_frame() {
    let endpoint = closed_model_endpoint();
    let (_directory, handle) = start_adk_product_with_loopback_provider(&endpoint).await;
    let address = handle.startup_record().address;
    let body = br#"{"clientRequestId":"11111111-1111-4111-8111-111111111111","agentId":"agent-live","message":"hello"}"#;

    let text = request_sse_until(address, "POST", "/api/v1/adk/chat/stream", body, |text| {
        text.contains("\"type\":\"final\"") || text.contains("\"type\":\"error\"")
    })
    .await;

    let session_at = text
        .find("\"type\":\"session\"")
        .unwrap_or_else(|| panic!("missing session frame: {text}"));
    let run_at = text
        .find("\"type\":\"run\"")
        .unwrap_or_else(|| panic!("missing run frame: {text}"));
    assert!(
        session_at < run_at,
        "the preview session frame must precede the run snapshot: {text}"
    );
    let session_frame = text
        .split("data: ")
        .nth(1)
        .and_then(|rest| rest.split('\n').next())
        .expect("first SSE data frame");
    let payload: Value = serde_json::from_str(session_frame).expect("first frame JSON");
    assert_eq!(payload["type"], "session");
    assert_eq!(
        payload["session"]["id"], "session-11111111-1111-4111-8111-111111111111",
        "the preview frame must carry the durable session id"
    );

    handle.shutdown().await.expect("shutdown product");
}
