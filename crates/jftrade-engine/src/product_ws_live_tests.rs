use std::sync::Arc;

use tempfile::tempdir;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;

use super::*;

#[derive(Debug)]
struct EnabledWsLiveSnapshotPort;

impl WsLiveSnapshotPort for EnabledWsLiveSnapshotPort {
    fn enabled(&self) -> bool {
        true
    }
}

#[tokio::test]
async fn ws_live_route_is_registered_only_with_explicit_snapshot_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let without_port =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(without_port).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    assert!(
        !handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| route == "GET /api/v1/ws/live")
    );
    handle.shutdown().await.expect("shutdown product");

    let with_port =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_ws_live_snapshot_port(Arc::new(EnabledWsLiveSnapshotPort));
    let handle = start_product(with_port).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 49);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| route == "GET /api/v1/ws/live")
    );
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn ws_live_subscription_registry_drives_status_and_releases_on_disconnect() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_ws_live_snapshot_port(Arc::new(EnabledWsLiveSnapshotPort));
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    let mut websocket = websocket_upgrade(address).await;

    let subscription = br#"{"type":"subscribe","subscriptions":{"providerBrokerId":"futu","activeInstruments":[" us.aapl "]}}"#;
    websocket
        .write_all(&masked_text_frame(subscription))
        .await
        .expect("send subscription frame");
    wait_for_live_projection(address, 1, &["US.AAPL"]).await;

    drop(websocket);
    wait_for_live_projection(address, 0, &[]).await;
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/app/apiserver/servercore/live_runtime_test.go:13 TestLiveStreamDiagnosticsUseConfiguredLimit
// Parity: go:452dea11:internal/api/live/handler_test.go:342 TestHandlerConnectionLimitAndCloseLifecycle
#[tokio::test]
async fn ws_live_transport_rejects_origin_and_limit_without_leaking_permits() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(
        &settings_path,
        br#"{"interfaces":{"liveWebSocketConnectionLimit":1}}
"#,
    )
    .expect("seed websocket settings");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_ws_live_snapshot_port(Arc::new(EnabledWsLiveSnapshotPort));
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    let first = websocket_upgrade(address).await;

    let limited = websocket_handshake(address, &[]).await;
    assert_eq!(limited.status, 503);
    assert_eq!(
        limited.content_type(),
        Some("application/json; charset=utf-8")
    );
    assert!(limited.body.contains("LIVE_WS_LIMIT_REACHED"));

    drop(first);
    wait_for_live_projection(address, 0, &[]).await;
    let recovered = websocket_upgrade(address).await;
    drop(recovered);

    let forbidden = websocket_handshake(address, &[("Origin", "http://evil.example")]).await;
    assert_eq!(forbidden.status, 403);
    assert_eq!(forbidden.content_type(), Some("text/plain; charset=utf-8"));
    assert_eq!(forbidden.body, "Forbidden\n");
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/api/live/handler_test.go:342 TestHandlerConnectionLimitAndCloseLifecycle
#[tokio::test]
async fn ws_live_shutdown_closes_active_connection_and_releases_depth_subscription() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_ws_live_snapshot_port(Arc::new(EnabledWsLiveSnapshotPort));
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    let hub = handle.live_hub();
    let mut websocket = websocket_upgrade(address).await;
    let heartbeat = read_server_text_frame(&mut websocket).await;
    let heartbeat: serde_json::Value = serde_json::from_str(&heartbeat).expect("heartbeat JSON");
    assert_eq!(heartbeat["type"], "heartbeat");

    let subscription = br#"{"type":"subscribe","subscriptions":{"providerBrokerId":"futu","activeInstruments":[],"depth":[{"market":"US","symbol":"AAPL","instrumentId":"US.AAPL","num":50}]}}"#;
    websocket
        .write_all(&masked_text_frame(subscription))
        .await
        .expect("send depth subscription");
    wait_for_live_projection(address, 1, &["US.AAPL"]).await;

    let shutdown = tokio::spawn(async move { handle.shutdown().await.expect("shutdown product") });
    let (code, reason) = tokio::time::timeout(
        std::time::Duration::from_secs(5),
        read_server_close_frame(&mut websocket),
    )
    .await
    .expect("shutdown close frame");
    assert_eq!(code, 1001, "server shutdown close code");
    assert_eq!(reason, "server shutting down");
    shutdown.await.expect("shutdown task");

    let snapshot = hub.snapshot();
    assert_eq!(
        snapshot.connected, 0,
        "shutdown must clear live connections"
    );
    assert!(
        snapshot.active_instruments.is_empty(),
        "shutdown must release depth demand: {snapshot:?}"
    );
}

#[tokio::test]
async fn ws_live_transport_rejects_untrusted_origin_before_upgrade() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_ws_live_snapshot_port(Arc::new(EnabledWsLiveSnapshotPort));
    let handle = start_product(config).await.expect("start product");
    // Parity: internal/api/live/handler_test.go:397 TestHandlerRejectsUntrustedWebSocketOrigin
    // Go iterates both a foreign origin and the malformed "null" origin; neither may upgrade.
    for origin in ["http://evil.example", "null"] {
        let response =
            websocket_handshake(handle.startup_record().address, &[("Origin", origin)]).await;
        assert_eq!(response.status, 403, "origin {origin}");
        assert_eq!(
            response.content_type(),
            Some("text/plain; charset=utf-8"),
            "origin {origin}"
        );
        assert_eq!(response.body, "Forbidden\n", "origin {origin}");
    }
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/api/live/handler_test.go:421 TestHandlerAcceptsSameOriginWebSocket
// Parity: go:452dea11:internal/api/live/handler_test.go:113 TestHandlerHeartbeatSubscribeNormalizationAndPayloads
// Parity: go:452dea11:internal/app/apiserver/servercore/ws_events_test.go:45 TestLiveWebSocketSendsSystemNotification
// The reference owner dials the page origin that served the console and reads
// the first event; the Rust transport keeps that guarantee through the
// desktop origin allow-list and answers with the initial heartbeat frame.
#[tokio::test]
async fn ws_live_transport_accepts_trusted_origin_and_streams_heartbeat_first() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_ws_live_snapshot_port(Arc::new(EnabledWsLiveSnapshotPort));
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    let response = websocket_handshake(address, &[("Origin", "http://127.0.0.1:3008")]).await;
    assert_eq!(response.status, 101, "trusted origin must upgrade");
    let mut stream = response.upgraded_stream.expect("upgraded websocket stream");
    let heartbeat: serde_json::Value =
        serde_json::from_str(&read_server_text_frame(&mut stream).await).expect("heartbeat json");
    assert_eq!(heartbeat["type"], "heartbeat");
    assert_eq!(heartbeat["source"], "system");
    assert_eq!(heartbeat["entityId"], "live-websocket");
    assert_eq!(heartbeat["payload"]["type"], "heartbeat");
    assert_eq!(heartbeat["payload"]["liveClients"]["connected"], 1);
    assert_eq!(heartbeat["payload"]["liveClients"]["limit"], 20);
    assert_eq!(heartbeat["payload"]["liveClients"]["atLimit"], false);
    drop(stream);
    wait_for_live_projection(address, 0, &[]).await;
    handle.shutdown().await.expect("shutdown product");
}

// Parity: go:452dea11:internal/app/apiserver/webaccess/security_integration_test.go:490 TestWebSocketUsesCookieSessionWithoutDesktopToken
/// A browser that only holds the web session cookie upgrades the live
/// websocket while the same handshake without credentials is refused.
#[tokio::test]
async fn ws_live_transport_upgrades_with_a_browser_session_cookie_alone() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_ws_live_snapshot_port(Arc::new(EnabledWsLiveSnapshotPort));
    config.access = AccessPolicy {
        session_token: Some("fixture-browser-session".to_owned()),
        enforce_access: true,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://jftrade.local".to_owned()]);
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;

    let anonymous = websocket_handshake(address, &[("Origin", "https://jftrade.local")]).await;
    assert_eq!(anonymous.status, 401);
    assert!(anonymous.body.contains("WEB_AUTH_REQUIRED"));

    let session = websocket_handshake(
        address,
        &[
            ("Origin", "https://jftrade.local"),
            ("Cookie", "jftrade_web_session=fixture-browser-session"),
        ],
    )
    .await;
    assert_eq!(session.status, 101, "cookie session must upgrade");
    drop(session.upgraded_stream);
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn ws_live_route_unavailable_is_plain_text_and_does_not_register_without_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    assert!(
        !handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| route == "GET /api/v1/ws/live")
    );
    let response = websocket_handshake(handle.startup_record().address, &[]).await;
    assert_eq!(response.status, 404);
    assert_eq!(response.content_type(), Some("text/plain; charset=utf-8"));
    assert_eq!(response.body, "404 page not found\n");
    handle.shutdown().await.expect("shutdown product");
}

async fn websocket_upgrade(address: std::net::SocketAddr) -> TcpStream {
    let response = websocket_handshake(address, &[]).await;
    assert_eq!(response.status, 101, "websocket handshake: {response:?}");
    response.upgraded_stream.expect("upgraded websocket stream")
}

#[derive(Debug)]
struct WebsocketHandshakeResponse {
    status: u16,
    headers: Vec<(String, String)>,
    body: String,
    upgraded_stream: Option<TcpStream>,
}

impl WebsocketHandshakeResponse {
    fn content_type(&self) -> Option<&str> {
        self.headers
            .iter()
            .find(|(name, _)| name.eq_ignore_ascii_case("content-type"))
            .map(|(_, value)| value.as_str())
    }
}

async fn websocket_handshake(
    address: std::net::SocketAddr,
    headers: &[(&str, &str)],
) -> WebsocketHandshakeResponse {
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect websocket");
    let extra_headers = headers
        .iter()
        .map(|(name, value)| format!("{name}: {value}\r\n"))
        .collect::<String>();
    let request = format!(
        "GET /api/v1/ws/live HTTP/1.1\r\nHost: {address}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n{extra_headers}\r\n"
    );
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write websocket handshake");
    let mut response = Vec::new();
    while !response.ends_with(b"\r\n\r\n") {
        let mut byte = [0_u8; 1];
        stream
            .read_exact(&mut byte)
            .await
            .expect("read websocket handshake");
        response.push(byte[0]);
        assert!(response.len() < 16 * 1024, "websocket handshake too large");
    }
    let header_text =
        String::from_utf8(response[..response.len() - 4].to_vec()).expect("handshake headers utf8");
    let mut lines = header_text.lines();
    let status = lines
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse().ok())
        .expect("HTTP handshake status");
    let headers: Vec<(String, String)> = lines
        .filter_map(|line| line.split_once(':'))
        .map(|(name, value)| (name.to_owned(), value.trim().to_owned()))
        .collect();
    let body = if status == 101 {
        String::new()
    } else if let Some(length) = headers
        .iter()
        .find(|(name, _)| name.eq_ignore_ascii_case("content-length"))
        .and_then(|(_, value)| value.parse::<usize>().ok())
    {
        let mut body = vec![0_u8; length];
        stream
            .read_exact(&mut body)
            .await
            .expect("read handshake rejection body");
        String::from_utf8(body).expect("handshake body utf8")
    } else {
        let mut body = Vec::new();
        stream
            .read_to_end(&mut body)
            .await
            .expect("read handshake rejection body");
        String::from_utf8(body).expect("handshake body utf8")
    };
    WebsocketHandshakeResponse {
        status,
        headers,
        body,
        upgraded_stream: (status == 101).then_some(stream),
    }
}

fn masked_text_frame(payload: &[u8]) -> Vec<u8> {
    let mask = [0x12_u8, 0x34, 0x56, 0x78];
    let length_bytes = if payload.len() < 126 {
        1
    } else if payload.len() <= u16::MAX as usize {
        3
    } else {
        9
    };
    let mut frame = Vec::with_capacity(payload.len() + length_bytes + 5);
    frame.push(0x81);
    if payload.len() < 126 {
        frame.push(0x80 | payload.len() as u8);
    } else if payload.len() <= u16::MAX as usize {
        frame.push(0xFE);
        frame.extend_from_slice(&(payload.len() as u16).to_be_bytes());
    } else {
        frame.push(0xFF);
        frame.extend_from_slice(&(payload.len() as u64).to_be_bytes());
    }
    frame.extend_from_slice(&mask);
    frame.extend(
        payload
            .iter()
            .enumerate()
            .map(|(index, byte)| byte ^ mask[index % mask.len()]),
    );
    frame
}

async fn read_server_text_frame(stream: &mut TcpStream) -> String {
    let (opcode, payload) = read_server_frame(stream).await;
    assert_eq!(opcode, 0x1, "expected a text frame");
    String::from_utf8(payload).expect("websocket frame utf8")
}

async fn read_server_frame(stream: &mut TcpStream) -> (u8, Vec<u8>) {
    let mut header = [0_u8; 2];
    stream
        .read_exact(&mut header)
        .await
        .expect("read websocket frame header");
    assert_eq!(header[1] & 0x80, 0, "server frames must not be masked");
    let length = match header[1] & 0x7f {
        126 => {
            let mut extended = [0_u8; 2];
            stream
                .read_exact(&mut extended)
                .await
                .expect("read websocket frame length");
            usize::from(u16::from_be_bytes(extended))
        }
        127 => {
            let mut extended = [0_u8; 8];
            stream
                .read_exact(&mut extended)
                .await
                .expect("read websocket frame length");
            usize::try_from(u64::from_be_bytes(extended)).expect("frame length fits usize")
        }
        short => usize::from(short),
    };
    let mut payload = vec![0_u8; length];
    stream
        .read_exact(&mut payload)
        .await
        .expect("read websocket frame payload");
    (header[0] & 0x0f, payload)
}

async fn read_server_close_frame(stream: &mut TcpStream) -> (u16, String) {
    for _ in 0..8 {
        let (opcode, payload) = read_server_frame(stream).await;
        if opcode == 0x1 {
            continue;
        }
        assert_eq!(opcode, 0x8, "expected a close frame");
        let code = u16::from_be_bytes(
            payload
                .get(..2)
                .expect("close frame status code")
                .try_into()
                .expect("close frame status bytes"),
        );
        let reason = String::from_utf8(payload[2..].to_vec()).expect("close frame reason utf8");
        return (code, reason);
    }
    panic!("server did not send a close frame after draining text frames");
}

async fn wait_for_live_projection(
    address: std::net::SocketAddr,
    connected: u64,
    active_instruments: &[&str],
) {
    for _ in 0..100 {
        let (status, response) =
            request_json_with_status(address, "GET", "/api/v1/system/status", None, &[]).await;
        assert_eq!(status, 200, "system status response: {response}");
        let live = &response["data"]["observability"]["live"];
        if live["connected"] == connected
            && live["activeInstruments"] == serde_json::json!(active_instruments)
        {
            return;
        }
        tokio::time::sleep(std::time::Duration::from_millis(10)).await;
    }
    panic!(
        "live projection did not reach connected={connected}, activeInstruments={active_instruments:?}"
    );
}
