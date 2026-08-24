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

async fn websocket_upgrade(address: std::net::SocketAddr) -> TcpStream {
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect websocket");
    let request = format!(
        "GET /api/v1/ws/live HTTP/1.1\r\nHost: {address}\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\n\r\n"
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
    let response = String::from_utf8(response).expect("handshake response utf8");
    assert!(
        response.starts_with("HTTP/1.1 101 "),
        "websocket handshake: {response}"
    );
    stream
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
