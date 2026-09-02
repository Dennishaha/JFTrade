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
    assert!(payload.len() < 126, "test payload must use the short frame");
    let mask = [0x12_u8, 0x34, 0x56, 0x78];
    let mut frame = Vec::with_capacity(payload.len() + 6);
    frame.push(0x81);
    frame.push(0x80 | payload.len() as u8);
    frame.extend_from_slice(&mask);
    frame.extend(
        payload
            .iter()
            .enumerate()
            .map(|(index, byte)| byte ^ mask[index % mask.len()]),
    );
    frame
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
