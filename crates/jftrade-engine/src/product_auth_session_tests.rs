use std::collections::VecDeque;
use std::sync::{Arc, Mutex};

use serde::Deserialize;
use serde_json::Value;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct AuthSessionFixture {
    version: String,
    cases: Vec<AuthSessionCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct AuthSessionCase {
    name: String,
    method: String,
    request_path: String,
    request_context: AuthSessionRequestContext,
    expected_status: u16,
    response_headers: std::collections::BTreeMap<String, String>,
    #[serde(default)]
    absent_headers: Vec<String>,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

const AUTH_SESSION_ALLOWED_ORIGIN: &str = "https://fixture.jftrade.local";
const AUTH_SESSION_REQUEST_ID: &str = "fixture-auth-session-id";
const AUTH_SESSION_CONTRACT_HEADER_NAMES: &[&str] = &[
    "access-control-allow-credentials",
    "access-control-allow-headers",
    "access-control-allow-methods",
    "access-control-allow-origin",
    "access-control-expose-headers",
    "cache-control",
    "content-type",
    "vary",
    "x-request-id",
];

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct AuthSessionRequestContext {
    desktop_trusted: bool,
    browser_authenticated: bool,
    origin_provided: bool,
    origin_allowed: bool,
}

#[derive(Debug)]
struct FixtureAuthSessionSnapshotPort {
    sessions: std::collections::BTreeMap<(bool, bool), Value>,
}

impl FixtureAuthSessionSnapshotPort {
    fn from_fixture(fixture: &AuthSessionFixture) -> Self {
        let sessions = fixture
            .cases
            .iter()
            .filter_map(|case| {
                let data = case.data.clone()?;
                Some((
                    (
                        case.request_context.desktop_trusted,
                        case.request_context.browser_authenticated,
                    ),
                    data,
                ))
            })
            .collect();
        Self { sessions }
    }
}

impl AuthSessionSnapshotPort for FixtureAuthSessionSnapshotPort {
    fn session(
        &self,
        request: AuthSessionSnapshotRequest,
    ) -> Result<Value, AuthSessionSnapshotError> {
        self.sessions
            .get(&(request.desktop_trusted, request.browser_authenticated))
            .cloned()
            .ok_or_else(|| {
                AuthSessionSnapshotError::Unavailable(
                    "Go auth-session fixture has no matching request context".to_owned(),
                )
            })
    }
}

#[derive(Debug)]
struct FailingAuthSessionSnapshotPort;

impl AuthSessionSnapshotPort for FailingAuthSessionSnapshotPort {
    fn session(
        &self,
        _request: AuthSessionSnapshotRequest,
    ) -> Result<Value, AuthSessionSnapshotError> {
        Err(AuthSessionSnapshotError::Unavailable(
            "Go auth-session fixture unavailable".to_owned(),
        ))
    }
}

#[derive(Debug)]
struct RecordingAuthSessionSnapshotPort {
    responses: Mutex<VecDeque<Result<Value, AuthSessionSnapshotError>>>,
    calls: Mutex<Vec<AuthSessionSnapshotRequest>>,
}

impl RecordingAuthSessionSnapshotPort {
    fn new(responses: impl IntoIterator<Item = Result<Value, AuthSessionSnapshotError>>) -> Self {
        Self {
            responses: Mutex::new(responses.into_iter().collect()),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn calls(&self) -> Vec<AuthSessionSnapshotRequest> {
        self.calls.lock().expect("auth-session calls lock").clone()
    }
}

impl AuthSessionSnapshotPort for RecordingAuthSessionSnapshotPort {
    fn session(
        &self,
        request: AuthSessionSnapshotRequest,
    ) -> Result<Value, AuthSessionSnapshotError> {
        self.calls
            .lock()
            .expect("auth-session calls lock")
            .push(request);
        self.responses
            .lock()
            .expect("auth-session responses lock")
            .pop_front()
            .expect("auth-session product response")
    }
}

fn auth_session_fixture() -> AuthSessionFixture {
    let fixture: AuthSessionFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/auth-session.json"
    ))
    .expect("auth-session fixture");
    assert_eq!(fixture.version, "stage9.auth-session.v1");
    fixture
}

#[tokio::test]
async fn auth_session_route_matches_go_fixture_in_cutover_only() {
    let fixture = auth_session_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_auth_session_snapshot_port(Arc::new(
                FixtureAuthSessionSnapshotPort::from_fixture(&fixture),
            ));
    config.access = AccessPolicy {
        desktop_token: Some("fixture-desktop-token".to_owned()),
        session_token: Some("fixture-browser-session".to_owned()),
        enforce_access: true,
        desktop_mode: true,
        ..AccessPolicy::default()
    }
    .with_allowed_origins([AUTH_SESSION_ALLOWED_ORIGIN.to_owned()]);
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 49);
    let address = handle.startup_record().address;
    for case in &fixture.cases {
        assert_eq!(case.method, "GET", "case {}", case.name);
        let headers = auth_session_headers(&case.request_context);
        let (status, headers, response) =
            request_json_response(address, &case.method, &case.request_path, &headers).await;
        assert_eq!(status, case.expected_status, "case {}", case.name);
        assert_eq!(
            auth_session_contract_headers(&headers),
            case.response_headers,
            "case {} contract headers",
            case.name
        );
        for (name, expected) in &case.response_headers {
            assert_eq!(
                headers.get(name).map(String::as_str),
                Some(expected.as_str()),
                "case {} header {name}",
                case.name
            );
        }
        for name in &case.absent_headers {
            assert!(
                !headers.contains_key(name),
                "case {} unexpectedly included header {name}",
                case.name
            );
        }
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
async fn auth_session_route_fails_closed_when_snapshot_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_auth_session_snapshot_port(Arc::new(FailingAuthSessionSnapshotPort));
    let handle = start_product(config).await.expect("start product");
    let (status, headers, response) = request_json_response(
        handle.startup_record().address,
        "GET",
        "/api/v1/auth/session",
        &[],
    )
    .await;
    assert_eq!(status, 503);
    assert_eq!(
        headers.get("cache-control").map(String::as_str),
        Some("no-store")
    );
    assert_eq!(
        headers.get("x-request-id").map(String::as_str),
        Some(AUTH_SESSION_REQUEST_ID)
    );
    assert_eq!(response["error"]["code"], "AUTH_SESSION_UNAVAILABLE");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn auth_session_route_is_not_registered_without_snapshot_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, headers, response) = request_json_response(
        handle.startup_record().address,
        "GET",
        "/api/v1/auth/session",
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert!(!headers.contains_key("cache-control"));
    assert_eq!(
        headers.get("x-request-id").map(String::as_str),
        Some(AUTH_SESSION_REQUEST_ID)
    );
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn auth_session_product_recovers_after_snapshot_failure_and_restart_without_side_effects() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{}\n").expect("seed settings");
    let initial_settings = std::fs::read(&settings_path).expect("read initial settings");
    let port = Arc::new(RecordingAuthSessionSnapshotPort::new([
        Err(AuthSessionSnapshotError::Unavailable(
            "fixture snapshot failure".to_owned(),
        )),
        Ok(json!({
            "authenticated": true,
            "csrfToken": "fixture-csrf",
            "expiresAt": "fixture-time"
        })),
    ]));
    let mut config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_auth_session_snapshot_port(port.clone());
    config.access = AccessPolicy {
        session_token: Some("fixture-session".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://fixture.jftrade.local".to_owned()]);
    let headers = [
        ("Cookie", "jftrade_web_session=fixture-session"),
        ("Origin", "https://fixture.jftrade.local"),
    ];
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    let (status, _, response) =
        request_json_response(address, "GET", "/api/v1/auth/session", &headers).await;
    assert_eq!(status, 503);
    assert_eq!(response["error"]["code"], "AUTH_SESSION_UNAVAILABLE");
    let (status, _, response) =
        request_json_response(address, "GET", "/api/v1/auth/session", &headers).await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["authenticated"], true);
    assert_eq!(
        port.calls(),
        vec![
            AuthSessionSnapshotRequest {
                desktop_trusted: false,
                browser_authenticated: true,
                origin_provided: true,
                origin_allowed: true,
            },
            AuthSessionSnapshotRequest {
                desktop_trusted: false,
                browser_authenticated: true,
                origin_provided: true,
                origin_allowed: true,
            },
        ]
    );
    handle.shutdown().await.expect("shutdown product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after recovery"),
        initial_settings
    );

    let restarted_port = Arc::new(RecordingAuthSessionSnapshotPort::new([Ok(json!({
        "authenticated": false
    }))]));
    let mut restarted_config = ProductConfig::test_cutover(
        "127.0.0.1:0".parse().expect("restarted address"),
        &settings_path,
    )
    .expect("restarted config")
    .with_auth_session_snapshot_port(restarted_port);
    restarted_config.access = AccessPolicy {
        session_token: Some("fixture-session".to_owned()),
        enforce_access: true,
        desktop_mode: false,
        ..AccessPolicy::default()
    }
    .with_allowed_origins(["https://fixture.jftrade.local".to_owned()]);
    let restarted = start_product(restarted_config)
        .await
        .expect("restart product");
    let (status, _, response) = request_json_response(
        restarted.startup_record().address,
        "GET",
        "/api/v1/auth/session",
        &headers,
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["authenticated"], false);
    restarted
        .shutdown()
        .await
        .expect("shutdown restarted product");
    assert_eq!(
        std::fs::read(&settings_path).expect("read settings after restart"),
        initial_settings
    );
}

fn auth_session_headers(context: &AuthSessionRequestContext) -> Vec<(&'static str, &'static str)> {
    let mut headers = Vec::new();
    if context.desktop_trusted {
        headers.push(("Authorization", "Bearer fixture-desktop-token"));
    }
    if context.browser_authenticated {
        headers.push(("Cookie", "jftrade_web_session=fixture-browser-session"));
    }
    if context.origin_provided {
        let origin = if context.origin_allowed {
            AUTH_SESSION_ALLOWED_ORIGIN
        } else {
            "https://forbidden.example.test"
        };
        headers.push(("Origin", origin));
    }
    headers
}

fn auth_session_contract_headers(
    headers: &std::collections::BTreeMap<String, String>,
) -> std::collections::BTreeMap<String, String> {
    AUTH_SESSION_CONTRACT_HEADER_NAMES
        .iter()
        .filter_map(|name| {
            headers
                .get(*name)
                .map(|value| ((*name).to_owned(), value.clone()))
        })
        .collect()
}

async fn request_json_response(
    address: SocketAddr,
    method: &str,
    path: &str,
    headers: &[(&str, &str)],
) -> (u16, std::collections::BTreeMap<String, String>, Value) {
    let mut stream = TcpStream::connect(address)
        .await
        .expect("connect product API");
    let extra_headers = headers
        .iter()
        .map(|(name, value)| format!("{name}: {value}\r\n"))
        .collect::<String>();
    let request = format!(
        "{method} {path} HTTP/1.1\r\nHost: {address}\r\nContent-Type: application/json\r\nX-Request-ID: {AUTH_SESSION_REQUEST_ID}\r\n{extra_headers}Content-Length: 0\r\nConnection: close\r\n\r\n"
    );
    stream
        .write_all(request.as_bytes())
        .await
        .expect("write request");
    let mut response = Vec::new();
    stream
        .read_to_end(&mut response)
        .await
        .expect("read response");
    let response = String::from_utf8(response).expect("UTF-8 response");
    let (head, body) = response.split_once("\r\n\r\n").expect("HTTP body");
    let mut lines = head.lines();
    let status = lines
        .next()
        .and_then(|line| line.split_whitespace().nth(1))
        .and_then(|value| value.parse().ok())
        .expect("HTTP status");
    let headers = lines
        .filter_map(|line| line.split_once(':'))
        .map(|(name, value)| (name.trim().to_ascii_lowercase(), value.trim().to_owned()))
        .collect();
    let value = serde_json::from_str(body).expect("JSON response");
    (status, headers, value)
}
