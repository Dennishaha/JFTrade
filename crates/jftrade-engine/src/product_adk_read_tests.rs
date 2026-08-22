use std::collections::BTreeMap;
use std::net::SocketAddr;
use std::sync::Arc;

use serde::Deserialize;
use serde_json::{Value, json};
use tempfile::tempdir;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
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

fn adk_read_fixture() -> AdkReadFixture {
    let fixture: AdkReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/adk-read.json"
    ))
    .expect("ADK read fixture");
    assert_eq!(fixture.version, "stage9.adk-read.v1");
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
    let ApiOutput::Sse(events) = adk_read_output(output) else {
        panic!("ADK stream did not convert to SSE output");
    };
    assert_eq!(
        events,
        vec![
            SseEvent {
                id: Some("7".to_owned()),
                data: json!({"type": "progress"}),
            },
            SseEvent {
                id: None,
                data: json!({"type": "done"}),
            },
        ]
    );
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
