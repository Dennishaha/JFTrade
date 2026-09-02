use std::collections::BTreeMap;
use std::fs;
use std::net::SocketAddr;
use std::sync::{Arc, Mutex};

use serde::Deserialize;
use serde_json::Value;
use tempfile::tempdir;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpStream;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct AlertsReadFixture {
    version: String,
    cases: Vec<AlertsReadCase>,
    wire_cases: Vec<AlertsReadWireCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct AlertsReadCase {
    name: String,
    method: String,
    request_path: String,
    response: Value,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct AlertsReadWireCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    envelope: Value,
}

#[derive(Debug)]
struct FixtureAlertReadPort {
    responses: BTreeMap<String, Value>,
    errors: BTreeMap<String, AlertSnapshotError>,
    calls: Mutex<Vec<(AlertKind, String)>>,
}

impl AlertSnapshotPort for FixtureAlertReadPort {
    fn snapshot(&self, kind: AlertKind, raw_query: &str) -> Result<Value, AlertSnapshotError> {
        self.calls
            .lock()
            .expect("alerts fixture call lock")
            .push((kind, raw_query.to_owned()));
        if let Some(error) = self.errors.get(raw_query) {
            return Err(error.clone());
        }
        self.responses.get(raw_query).cloned().ok_or_else(|| {
            AlertSnapshotError::Unavailable(format!("fixture response missing for {raw_query}"))
        })
    }
}

#[derive(Debug)]
struct UnavailableAlertReadPort;

impl AlertSnapshotPort for UnavailableAlertReadPort {
    fn snapshot(&self, _kind: AlertKind, _raw_query: &str) -> Result<Value, AlertSnapshotError> {
        Err(AlertSnapshotError::Unavailable(
            "Go alerts owner snapshot is unavailable".to_owned(),
        ))
    }
}

fn alerts_read_fixture() -> AlertsReadFixture {
    let fixture: AlertsReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/alerts-read.json"
    ))
    .expect("alerts read fixture");
    assert_eq!(fixture.version, "stage9.alerts-read.v1");
    assert_eq!(fixture.cases.len(), 2);
    assert_eq!(fixture.wire_cases.len(), 3);
    fixture
}

fn query_from_path(path: &str) -> &str {
    path.split_once('?').map_or("", |(_, query)| query)
}

fn fixture_responses(fixture: &AlertsReadFixture) -> BTreeMap<String, Value> {
    let mut responses = BTreeMap::new();
    for case in &fixture.cases {
        responses.insert(
            query_from_path(&case.request_path).to_owned(),
            case.response.clone(),
        );
    }
    for case in &fixture.wire_cases {
        if case.expected_status == 200 {
            let data = case.envelope["data"].clone();
            responses.insert(query_from_path(&case.request_path).to_owned(), data);
        }
    }
    responses
}

fn fixture_errors(fixture: &AlertsReadFixture) -> BTreeMap<String, AlertSnapshotError> {
    let mut errors = BTreeMap::new();
    for case in &fixture.wire_cases {
        let error = match case.expected_status {
            409 => AlertSnapshotError::CapabilityUnavailable(
                case.envelope["error"]["message"]
                    .as_str()
                    .expect("capability error message")
                    .to_owned(),
            ),
            502 => AlertSnapshotError::Provider {
                status: None,
                message: case.envelope["error"]["message"]
                    .as_str()
                    .expect("provider error message")
                    .to_owned(),
            },
            _ => continue,
        };
        errors.insert(query_from_path(&case.request_path).to_owned(), error);
    }
    errors
}

#[tokio::test]
async fn alerts_read_routes_replay_go_fixture_and_preserve_read_only_state() {
    let fixture = alerts_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let seed = br#"{"sentinel":"alerts-read"}
"#;
    fs::write(&settings_path, seed).expect("seed settings");
    let port = Arc::new(FixtureAlertReadPort {
        responses: fixture_responses(&fixture),
        errors: fixture_errors(&fixture),
        calls: Mutex::new(Vec::new()),
    });
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_alert_snapshot_port(port.clone());
    let handle = start_product(config).await.expect("start product");
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "GET /api/v1/alerts/price" })
    );
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "GET /api/v1/alerts/option-events" })
    );
    assert!(
        !handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| route.starts_with("POST /api/v1/alerts/"))
    );

    let mut requests = fixture
        .cases
        .iter()
        .map(|case| (&case.name, &case.method, &case.request_path, &case.response))
        .collect::<Vec<_>>();
    let empty_case = fixture
        .wire_cases
        .iter()
        .find(|case| case.name == "price-empty-result")
        .expect("empty alert case");
    requests.push((
        &empty_case.name,
        &empty_case.method,
        &empty_case.request_path,
        &empty_case.envelope["data"],
    ));

    for (name, method, request_path, expected_data) in requests.iter().copied() {
        let response = request_alert_http(
            handle.startup_record().address,
            method,
            request_path,
            Some(name),
            None,
        )
        .await;
        assert_eq!(response.status, 200, "case {name}");
        assert_eq!(
            response.headers["content-type"],
            "application/json; charset=utf-8"
        );
        assert_eq!(response.headers["x-request-id"], *name, "case {name}");
        assert_eq!(response.body["ok"], true, "case {name}");
        assert_eq!(response.body["data"], *expected_data, "case {name}");
    }

    for case in &fixture.wire_cases {
        if case.expected_status == 200 {
            continue;
        }
        let response = request_alert_http(
            handle.startup_record().address,
            &case.method,
            &case.request_path,
            Some(&case.name),
            None,
        )
        .await;
        assert_eq!(response.status, case.expected_status, "case {}", case.name);
        assert!(
            !response.body["timestamp"]
                .as_str()
                .unwrap_or_default()
                .is_empty()
        );
        let mut normalized_body = response.body;
        normalized_body["timestamp"] = case.envelope["timestamp"].clone();
        assert_eq!(normalized_body, case.envelope, "case {}", case.name);
    }

    {
        let calls = port.calls.lock().expect("alerts fixture call lock");
        assert_eq!(calls.len(), requests.len() + 2);
        for (_, _, request_path, _) in requests.iter().copied() {
            assert!(
                calls
                    .iter()
                    .any(|(_, query)| query == query_from_path(request_path))
            );
        }
        for case in &fixture.wire_cases {
            assert!(
                calls
                    .iter()
                    .any(|(_, query)| query == query_from_path(&case.request_path))
            );
        }
    }
    handle.shutdown().await.expect("shutdown product");
    assert_eq!(fs::read(&settings_path).expect("read settings"), seed);
}

#[tokio::test]
async fn alerts_read_routes_fail_closed_without_snapshot_port_or_with_unavailable_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_alert_snapshot_port(Arc::new(UnavailableAlertReadPort));
    let handle = start_product(config).await.expect("start product");
    for path in [
        "/api/v1/alerts/price?brokerId=futu&market=US",
        "/api/v1/alerts/option-events?brokerId=futu&market=US",
    ] {
        let response = request_alert_http(
            handle.startup_record().address,
            "GET",
            path,
            Some("alerts-unavailable"),
            None,
        )
        .await;
        assert_eq!(response.status, 503, "path {path}");
        assert_eq!(response.body["error"]["code"], "ALERTS_UNAVAILABLE");
    }
    handle.shutdown().await.expect("shutdown product");

    let unavailable_directory = tempdir().expect("temporary directory");
    let unavailable_settings = unavailable_directory.path().join("settings.json");
    let config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("address"),
        &unavailable_settings,
    )
    .expect("config");
    let handle = start_product(config).await.expect("start product");
    let response = request_alert_http(
        handle.startup_record().address,
        "GET",
        "/api/v1/alerts/price?brokerId=futu&market=US",
        Some("alerts-not-registered"),
        None,
    )
    .await;
    assert_eq!(response.status, 404);
    assert_eq!(response.body["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn alerts_read_routes_require_the_authenticated_desktop_token() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let token = "alerts-read-auth-token-012345678901234567890";
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_alert_snapshot_port(Arc::new(FixtureAlertReadPort {
                responses: BTreeMap::from([(
                    "brokerId=futu&market=US".to_owned(),
                    serde_json::json!({"entries": [], "provider": {"brokerId": "futu"}}),
                )]),
                errors: BTreeMap::new(),
                calls: Mutex::new(Vec::new()),
            }));
    config.access = AccessPolicy {
        desktop_token: Some(token.to_owned()),
        enforce_access: true,
        desktop_mode: true,
        ..config.access
    };
    let handle = start_product(config).await.expect("start product");
    let unauthorized = request_alert_http(
        handle.startup_record().address,
        "GET",
        "/api/v1/alerts/price?brokerId=futu&market=US",
        Some("alerts-auth"),
        None,
    )
    .await;
    assert_eq!(unauthorized.status, 401);
    assert_eq!(unauthorized.body["error"]["code"], "WEB_AUTH_REQUIRED");

    let authorization = format!("Bearer {token}");
    let authorized = request_alert_http(
        handle.startup_record().address,
        "GET",
        "/api/v1/alerts/price?brokerId=futu&market=US",
        Some("alerts-auth-ok"),
        Some(("Authorization", authorization.as_str())),
    )
    .await;
    assert_eq!(authorized.status, 200);
    assert_eq!(authorized.body["ok"], true);
    handle.shutdown().await.expect("shutdown product");
}

#[derive(Debug)]
struct AlertHttpResponse {
    status: u16,
    headers: BTreeMap<String, String>,
    body: Value,
}

async fn request_alert_http(
    address: SocketAddr,
    method: &str,
    path: &str,
    request_id: Option<&str>,
    authorization: Option<(&str, &str)>,
) -> AlertHttpResponse {
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect alerts product API");
    let request_id = request_id.map_or(String::new(), |value| format!("X-Request-ID: {value}\r\n"));
    let authorization = authorization.map_or(String::new(), |(name, value)| {
        format!("{name}: {value}\r\n")
    });
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Length: 0\r\n{request_id}{authorization}Connection: close\r\n\r\n"
    );
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write alerts request");
    let mut raw_response = Vec::new();
    stream
        .read_to_end(&mut raw_response)
        .await
        .expect("read alerts response");
    let raw_response = String::from_utf8(raw_response).expect("alerts response UTF-8");
    let (raw_headers, raw_body) = raw_response.split_once("\r\n\r\n").expect("alerts body");
    let status = raw_headers
        .lines()
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse().ok())
        .expect("alerts status");
    let headers = raw_headers
        .lines()
        .skip(1)
        .filter_map(|line| line.split_once(':'))
        .map(|(name, value)| (name.to_ascii_lowercase(), value.trim().to_owned()))
        .collect();
    AlertHttpResponse {
        status,
        headers,
        body: serde_json::from_str(raw_body).expect("alerts JSON response"),
    }
}
