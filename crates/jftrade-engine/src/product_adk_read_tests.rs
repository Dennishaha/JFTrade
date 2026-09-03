use std::collections::BTreeMap;
use std::net::SocketAddr;
use std::sync::Arc;

use serde::Deserialize;
use serde_json::{Value, json};
use tempfile::tempdir;
use tokio::io::{AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader};
use tokio::net::TcpStream;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct AdkReadFixture {
    version: String,
    cases: Vec<AdkReadFixtureCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct AdkReadFixtureCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct AdkReadSseFixture {
    version: String,
    cases: Vec<AdkReadSseFixtureCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct AdkReadSseFixtureCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    headers: BTreeMap<String, String>,
    body: String,
}

#[derive(Debug)]
struct FixtureAdkReadPort {
    responses: BTreeMap<String, Result<AdkReadSnapshot, AdkReadSnapshotError>>,
}

impl FixtureAdkReadPort {
    fn from_fixture(fixture: &AdkReadFixture) -> Self {
        let responses = fixture
            .cases
            .iter()
            .map(|case| {
                let response = case.data.clone().map_or_else(
                    || {
                        Err(AdkReadSnapshotError::Failed {
                            status: case.expected_status,
                            code: case.error_code.clone().unwrap_or_default(),
                            message: case.error_message.clone().unwrap_or_default(),
                            retry_after_seconds: None,
                        })
                    },
                    |data| Ok(AdkReadSnapshot::Json(data)),
                );
                (case.request_path.clone(), response)
            })
            .collect();
        Self { responses }
    }
}

impl AdkReadSnapshotPort for FixtureAdkReadPort {
    fn read(&self, path: &str, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let key = if query.is_empty() {
            path.to_owned()
        } else {
            format!("{path}?{query}")
        };
        self.responses.get(&key).cloned().unwrap_or_else(|| {
            Err(AdkReadSnapshotError::Unavailable(
                "fixture response missing".to_owned(),
            ))
        })
    }
}

#[derive(Debug)]
struct FailingAdkReadPort;

impl AdkReadSnapshotPort for FailingAdkReadPort {
    fn read(&self, _path: &str, _query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        Err(AdkReadSnapshotError::Unavailable(
            "Go ADK read owner unavailable".to_owned(),
        ))
    }
}

#[derive(Debug)]
struct StreamAdkReadPort;

impl AdkReadSnapshotPort for StreamAdkReadPort {
    fn read(&self, _path: &str, _query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        Ok(AdkReadSnapshot::Stream(AdkReadStream {
            headers: vec![("X-ADK-Stream-ID".to_owned(), "stream-fixture".to_owned())],
            events: vec![
                AdkReadEvent {
                    id: Some("7".to_owned()),
                    data: json!({"type": "progress"}),
                },
                AdkReadEvent {
                    id: None,
                    data: json!({"type": "done"}),
                },
            ],
        }))
    }
}

#[derive(Debug)]
struct FixtureAdkReadSsePort {
    responses: BTreeMap<String, Result<AdkReadSnapshot, AdkReadSnapshotError>>,
}

impl FixtureAdkReadSsePort {
    fn from_fixture(fixture: &AdkReadSseFixture) -> Self {
        let responses = fixture
            .cases
            .iter()
            .map(|case| {
                let path = case
                    .request_path
                    .split('?')
                    .next()
                    .unwrap_or(&case.request_path);
                let query = case
                    .request_path
                    .split_once('?')
                    .map_or("", |(_, query)| query);
                let key = if query.is_empty() {
                    path.to_owned()
                } else {
                    format!("{path}?{query}")
                };
                let headers = case
                    .headers
                    .iter()
                    .filter(|(name, _)| {
                        !matches!(
                            name.as_str(),
                            "content-type" | "cache-control" | "connection"
                        )
                    })
                    .map(|(name, value)| (name.clone(), value.clone()))
                    .collect();
                let events = parse_sse_events(&case.body);
                (
                    key,
                    Ok(AdkReadSnapshot::Stream(AdkReadStream { headers, events })),
                )
            })
            .collect();
        Self { responses }
    }
}

impl AdkReadSnapshotPort for FixtureAdkReadSsePort {
    fn read(&self, path: &str, query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        let key = if query.is_empty() {
            path.to_owned()
        } else {
            format!("{path}?{query}")
        };
        self.responses.get(&key).cloned().unwrap_or_else(|| {
            Err(AdkReadSnapshotError::Unavailable(
                "fixture SSE response missing".to_owned(),
            ))
        })
    }
}

fn adk_read_fixture() -> AdkReadFixture {
    let fixture: AdkReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/adk-read.json"
    ))
    .expect("ADK read fixture");
    assert_eq!(fixture.version, "stage9.adk-read.v1");
    fixture
}

fn adk_read_sse_fixture() -> AdkReadSseFixture {
    let fixture: AdkReadSseFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/adk-read-sse.json"
    ))
    .expect("ADK read SSE fixture");
    assert_eq!(fixture.version, "stage9.adk-read-sse.v1");
    fixture
}

#[tokio::test]
async fn adk_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = adk_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_read_snapshot_port(Arc::new(FixtureAdkReadPort::from_fixture(&fixture)));
    let handle = start_product(config).await.expect("start product");
    let route_count = ADK_READ_ROUTES.len();
    assert!(handle.startup_record().owned_routes >= route_count);
    for case in &fixture.cases {
        let (status, response) = request_adk_json_response(
            handle.startup_record().address,
            &case.method,
            &case.request_path,
        )
        .await;
        assert_eq!(status, case.expected_status, "case {}", case.name);
        if let Some(expected) = &case.data {
            assert_eq!(response["ok"], true, "case {}", case.name);
            assert_eq!(response["data"], *expected, "case {}", case.name);
        } else {
            assert_eq!(response["ok"], false, "case {}", case.name);
            assert_eq!(
                response["error"]["code"].as_str(),
                case.error_code.as_deref(),
                "case {}",
                case.name
            );
            assert_eq!(
                response["error"]["message"].as_str(),
                case.error_message.as_deref(),
                "case {}",
                case.name
            );
        }
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn adk_read_routes_fail_closed_without_snapshot_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    let (status, response) =
        request_adk_json_response(handle.startup_record().address, "GET", "/api/v1/adk").await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}

#[test]
fn adk_read_dynamic_routes_validate_suffixes_and_identifiers() {
    assert_eq!(
        route_for("/api/v1/adk/runs/run-1/stream"),
        Some(AdkReadRoute::RunStream)
    );
    assert_eq!(
        route_for("/api/v1/adk/sessions/session-1/context"),
        Some(AdkReadRoute::SessionContext)
    );
    assert_eq!(
        route_for("/api/v1/adk/workflows/workflow-1/triggers"),
        Some(AdkReadRoute::WorkflowTriggers)
    );
    assert_eq!(route_for("/api/v1/adk/runs//stream"), None);
    assert_eq!(route_for("/api/v1/adk/sessions//context"), None);

    for (path, message) in [
        ("/api/v1/adk/runs/%20/stream", "runId is invalid"),
        ("/api/v1/adk/sessions/%20/context", "sessionId is invalid"),
        (
            "/api/v1/adk/workflows/%20/triggers",
            "workflowId is invalid",
        ),
        ("/api/v1/adk/tasks/a%2Fb", "taskId is invalid"),
    ] {
        let failure = dispatch_adk_read(Some(&FailingAdkReadPort), "GET", path, "")
            .expect_err("invalid identifier");
        assert_eq!(failure.status, 400, "path {path}");
        assert_eq!(failure.message, message, "path {path}");
    }
}

#[test]
fn adk_read_streams_preserve_event_ids_and_payloads() {
    let output = dispatch_adk_read(
        Some(&StreamAdkReadPort),
        "GET",
        "/api/v1/adk/streams/stream-fixture",
        "after=0",
    )
    .expect("stream snapshot");
    let ApiOutput::Raw {
        status,
        content_type,
        body,
        headers,
    } = adk_read_output(output)
    else {
        panic!("ADK stream did not convert to raw SSE output");
    };
    assert_eq!(status, 200);
    assert_eq!(content_type, "text/event-stream");
    assert_eq!(headers["cache-control"], "no-cache");
    assert_eq!(headers["connection"], "keep-alive");
    assert_eq!(headers["X-ADK-Stream-ID"], "stream-fixture");
    assert_eq!(
        String::from_utf8(body).expect("SSE body"),
        "retry: 3000\n\nid: 7\ndata: {\"type\":\"progress\"}\n\ndata: {\"type\":\"done\"}\n\n"
    );
}

#[tokio::test]
async fn adk_read_success_sse_fixture_matches_go_wire_in_cutover_only() {
    let fixture = adk_read_sse_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_read_snapshot_port(Arc::new(FixtureAdkReadSsePort::from_fixture(&fixture)));
    let handle = start_product(config).await.expect("start product");
    for case in &fixture.cases {
        let response = request_adk_raw_response(
            handle.startup_record().address,
            &case.method,
            &case.request_path,
        )
        .await;
        assert_eq!(response.status, case.expected_status, "case {}", case.name);
        assert_eq!(
            response.body,
            case.body.as_bytes(),
            "case {} body",
            case.name
        );
        for (name, expected) in &case.headers {
            assert_eq!(
                response.headers.get(name),
                Some(expected),
                "case {} header {}",
                case.name,
                name
            );
        }
    }
    handle.shutdown().await.expect("shutdown product");
}

async fn request_adk_json_response(address: SocketAddr, method: &str, path: &str) -> (u16, Value) {
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect product API");
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\nX-Request-ID: fixture-adk-read\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"
    );
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write request");
    let mut raw_response = Vec::new();
    stream
        .read_to_end(&mut raw_response)
        .await
        .expect("read response");
    let response = String::from_utf8(raw_response).expect("UTF-8 response");
    let (head, body) = response.split_once("\r\n\r\n").expect("HTTP body");
    let status = head
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse().ok())
        .expect("HTTP status");
    (status, serde_json::from_str(body).expect("JSON response"))
}

#[derive(Debug)]
struct AdkRawResponse {
    status: u16,
    headers: BTreeMap<String, String>,
    body: Vec<u8>,
}

async fn request_adk_raw_response(address: SocketAddr, method: &str, path: &str) -> AdkRawResponse {
    let stream = TcpStream::connect(address)
        .await
        .expect("connect product API");
    let (mut reader, mut writer) = tokio::io::split(stream);
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nX-Request-ID: fixture-adk-read-sse\r\nContent-Length: 0\r\nConnection: keep-alive\r\n\r\n"
    );
    writer
        .write_all(request.as_bytes())
        .await
        .expect("write request");
    let mut buffered = BufReader::new(&mut reader);
    let mut head = Vec::new();
    buffered
        .read_until(b'\n', &mut head)
        .await
        .expect("read response status");
    let status_line = String::from_utf8(head).expect("UTF-8 status");
    let status = status_line
        .split_whitespace()
        .next()
        .and_then(|_| status_line.split_whitespace().nth(1))
        .and_then(|value| value.parse().ok())
        .expect("HTTP status");
    let mut headers = BTreeMap::new();
    let mut content_length = None;
    loop {
        let mut line = Vec::new();
        buffered
            .read_until(b'\n', &mut line)
            .await
            .expect("read response header");
        if line == b"\r\n" || line == b"\n" {
            break;
        }
        let line = String::from_utf8(line).expect("UTF-8 header");
        if let Some((name, value)) = line.trim_end().split_once(": ") {
            let name = name.to_ascii_lowercase();
            let value = value.to_owned();
            if name == "content-length" {
                content_length = value.parse::<usize>().ok();
            }
            headers.insert(name, value);
        }
    }
    let mut body = vec![0; content_length.expect("content length")];
    buffered
        .read_exact(&mut body)
        .await
        .expect("read response body");
    AdkRawResponse {
        status,
        headers,
        body,
    }
}

fn parse_sse_events(body: &str) -> Vec<AdkReadEvent> {
    body.split("\n\n")
        .filter_map(|block| {
            let mut id = None;
            let mut data = None;
            for line in block.lines() {
                if let Some(value) = line.strip_prefix("id: ") {
                    id = Some(value.to_owned());
                } else if let Some(value) = line.strip_prefix("data: ") {
                    data = Some(serde_json::from_str(value).expect("SSE event JSON"));
                }
            }
            data.map(|data| AdkReadEvent { id, data })
        })
        .collect()
}
