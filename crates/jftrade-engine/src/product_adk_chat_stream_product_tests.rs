use super::*;
use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute, AdkChatStreamFrame,
    AdkChatStreamPort, AdkChatStreamSnapshot,
};
use serde_json::{Value, json};
use std::collections::BTreeMap;
use std::net::SocketAddr;
use std::sync::Arc;
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

struct RawResponse {
    status: u16,
    headers: BTreeMap<String, String>,
    body: Vec<u8>,
}

async fn request_raw(address: SocketAddr, method: &str, path: &str, body: &[u8]) -> RawResponse {
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect ADK product API");
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n",
        body.len()
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
    stream
        .read_to_end(&mut response)
        .await
        .expect("read ADK response");
    let separator = response
        .windows(4)
        .position(|window| window == b"\r\n\r\n")
        .expect("response header separator");
    let header_bytes = &response[..separator];
    let body = response[separator + 4..].to_vec();
    let mut lines = header_bytes.split(|byte| *byte == b'\n');
    let status_line = lines.next().expect("status line");
    let status = String::from_utf8_lossy(status_line)
        .split_whitespace()
        .nth(1)
        .expect("status code")
        .parse()
        .expect("numeric status");
    let headers = lines
        .filter_map(|line| {
            let line = String::from_utf8_lossy(line);
            let (name, value) = line.trim().split_once(':')?;
            Some((name.trim().to_ascii_lowercase(), value.trim().to_owned()))
        })
        .collect();
    RawResponse {
        status,
        headers,
        body,
    }
}
