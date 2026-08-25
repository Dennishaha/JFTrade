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
