#[path = "../src/product_auth_session_write_port.rs"]
mod product_auth_session_write_port;

use std::collections::{BTreeMap, VecDeque};
use std::sync::Mutex;

use product_auth_session_write_port::{
    AUTH_LOGIN_PATH, AUTH_LOGOUT_PATH, AuthSessionWriteInput, AuthSessionWritePort,
    AuthSessionWritePortError, AuthSessionWritePortResult, AuthSessionWriteRequest,
    auth_session_write_routes, dispatch_auth_session_write,
};
use serde::Deserialize;
use serde_json::{Value, json};

const FIXTURE_TIMESTAMP: &str = "2026-08-22T04:00:00Z";

#[derive(Debug, Deserialize)]
struct Fixture {
    version: String,
    cases: Vec<FixtureCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureCase {
    name: String,
    requests: Vec<FixtureRequest>,
    expected: Vec<FixtureExpected>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureRequest {
    method: String,
    path: String,
    body: String,
    #[serde(default)]
    headers: BTreeMap<String, String>,
    request_context: FixtureRequestContext,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureExpected {
    status: u16,
    response_headers: BTreeMap<String, String>,
    envelope: Value,
    port_call: bool,
    port_error: Option<String>,
}

#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureRequestContext {
    desktop_trusted: bool,
    browser_authenticated: bool,
    origin_provided: bool,
    origin_allowed: bool,
    csrf_valid: bool,
    web_access_enabled: bool,
    web_auth_available: bool,
}

#[derive(Debug)]
struct FixturePort {
    responses: Mutex<VecDeque<Result<AuthSessionWritePortResult, AuthSessionWritePortError>>>,
    calls: Mutex<Vec<AuthSessionWriteInput>>,
}

impl FixturePort {
    fn from_case(case: &FixtureCase) -> Self {
        let responses = case
            .expected
            .iter()
            .filter(|expected| expected.port_call)
            .map(port_response)
            .collect();
        Self {
            responses: Mutex::new(responses),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn call_count(&self) -> usize {
        self.calls.lock().expect("auth write calls lock").len()
    }

    fn assert_drained(&self, case_name: &str) {
        assert!(
            self.responses
                .lock()
                .expect("auth write responses lock")
                .is_empty(),
            "fixture port responses remain for {case_name}"
        );
    }
}

impl AuthSessionWritePort for FixturePort {
    fn login_rate_limit(&self) -> Option<AuthSessionWritePortError> {
        (self.calls.lock().expect("auth write calls lock").len() >= 8).then_some(
            AuthSessionWritePortError::RateLimited {
                retry_after: 300,
                message: "too many failed login attempts".to_owned(),
            },
        )
    }

    fn mutate(
        &self,
        input: &AuthSessionWriteInput,
    ) -> Result<AuthSessionWritePortResult, AuthSessionWritePortError> {
        self.calls
            .lock()
            .expect("auth write calls lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("auth write responses lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(AuthSessionWritePortError::Failed(
                    "fixture response missing".to_owned(),
                ))
            })
    }
}

#[test]
fn auth_session_write_fixture_matches_go_owner_for_both_routes() {
    let fixture = auth_session_write_fixture();
    assert_eq!(fixture.version, "stage9.auth-session-write.v1");
    assert_eq!(fixture.cases.len(), 18);

    for case in &fixture.cases {
        assert_eq!(
            case.requests.len(),
            case.expected.len(),
            "case {}",
            case.name
        );
        let port = FixturePort::from_case(case);
        for (request, expected) in case.requests.iter().zip(&case.expected) {
            let response =
                dispatch_auth_session_write(&to_request(request), Some(&port), FIXTURE_TIMESTAMP);
            assert_eq!(response.status, expected.status, "case {}", case.name);
            assert_eq!(
                owned_headers(&response.headers),
                owned_expected_headers(expected),
                "case {}",
                case.name
            );
            assert_eq!(response.body, expected.envelope, "case {}", case.name);
        }
        assert_eq!(
            port.call_count(),
            case.expected
                .iter()
                .filter(|expected| expected.port_call)
                .count(),
            "case {} port calls",
            case.name
        );
        port.assert_drained(&case.name);
    }
}

#[test]
fn auth_session_write_leaf_has_exact_route_inventory_and_isolation() {
    assert_eq!(auth_session_write_routes().len(), 2);
    assert!(auth_session_write_routes().contains(&("POST", AUTH_LOGIN_PATH)));
    assert!(auth_session_write_routes().contains(&("POST", AUTH_LOGOUT_PATH)));

    let request = AuthSessionWriteRequest {
        method: "GET".to_owned(),
        path: AUTH_LOGIN_PATH.to_owned(),
        body: Some(Vec::new()),
        desktop_trusted: false,
        browser_authenticated: false,
        origin_provided: false,
        origin_allowed: true,
        csrf_valid: false,
        web_access_enabled: true,
        web_auth_available: true,
        session_cookie: None,
    };
    let response = dispatch_auth_session_write(&request, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 404);
    assert_eq!(response.body["error"]["code"], "NOT_FOUND");

    let extra_segment = AuthSessionWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/auth/logout/extra".to_owned(),
        ..request
    };
    assert_eq!(
        dispatch_auth_session_write(&extra_segment, None, FIXTURE_TIMESTAMP).status,
        404
    );
}

#[test]
fn auth_session_write_leaf_fails_closed_without_state_port() {
    let login = AuthSessionWriteRequest {
        method: "POST".to_owned(),
        path: AUTH_LOGIN_PATH.to_owned(),
        body: Some(br#"{"password":"fixture-password"}"#.to_vec()),
        desktop_trusted: false,
        browser_authenticated: false,
        origin_provided: true,
        origin_allowed: true,
        csrf_valid: false,
        web_access_enabled: true,
        web_auth_available: true,
        session_cookie: None,
    };
    let response = dispatch_auth_session_write(&login, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(
        response.body["error"]["code"],
        "AUTH_SESSION_WRITE_UNAVAILABLE"
    );
    assert!(!response.headers.contains_key("Set-Cookie"));

    let logout = AuthSessionWriteRequest {
        method: "POST".to_owned(),
        path: AUTH_LOGOUT_PATH.to_owned(),
        body: Some(b"not-json".to_vec()),
        browser_authenticated: true,
        origin_provided: true,
        origin_allowed: true,
        csrf_valid: true,
        web_access_enabled: true,
        web_auth_available: true,
        ..login
    };
    let response = dispatch_auth_session_write(&logout, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(
        response.body["error"]["code"],
        "AUTH_SESSION_WRITE_UNAVAILABLE"
    );
}

#[test]
fn auth_session_write_leaf_replays_failure_then_recovery() {
    let port = FixturePort {
        responses: Mutex::new(VecDeque::from([
            Err(AuthSessionWritePortError::Canceled(
                "login request was canceled".to_owned(),
            )),
            Ok(AuthSessionWritePortResult {
                data: json!({
                    "authenticated": true,
                    "csrfToken": "fixture-csrf-token",
                    "expiresAt": "fixture-time",
                }),
                set_cookie: Some(
                    "jftrade_web_session=fixture-session-token; Path=/; Expires=Sat, 22 Aug 2026 16:00:00 GMT; Max-Age=43200; HttpOnly; SameSite=Strict".to_owned(),
                ),
            }),
        ])),
        calls: Mutex::new(Vec::new()),
    };
    let request = AuthSessionWriteRequest {
        method: "POST".to_owned(),
        path: AUTH_LOGIN_PATH.to_owned(),
        body: Some(br#"{"password":"fixture-password"}"#.to_vec()),
        origin_provided: true,
        origin_allowed: true,
        web_access_enabled: true,
        web_auth_available: true,
        ..AuthSessionWriteRequest {
            method: String::new(),
            path: String::new(),
            body: None,
            desktop_trusted: false,
            browser_authenticated: false,
            origin_provided: false,
            origin_allowed: false,
            csrf_valid: false,
            web_access_enabled: false,
            web_auth_available: false,
            session_cookie: None,
        }
    };
    let first = dispatch_auth_session_write(&request, Some(&port), FIXTURE_TIMESTAMP);
    let second = dispatch_auth_session_write(&request, Some(&port), FIXTURE_TIMESTAMP);
    assert_eq!(first.status, 408);
    assert_eq!(first.body["error"]["code"], "REQUEST_CANCELED");
    assert_eq!(second.status, 200);
    assert_eq!(second.body["data"]["authenticated"], true);
    assert_eq!(port.call_count(), 2);
}

fn auth_session_write_fixture() -> Fixture {
    serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/auth-session-write.json"
    ))
    .expect("auth-session-write fixture")
}

fn to_request(request: &FixtureRequest) -> AuthSessionWriteRequest {
    AuthSessionWriteRequest {
        method: request.method.clone(),
        path: request.path.clone(),
        body: Some(request.body.as_bytes().to_vec()),
        desktop_trusted: request.request_context.desktop_trusted,
        browser_authenticated: request.request_context.browser_authenticated,
        origin_provided: request.request_context.origin_provided,
        origin_allowed: request.request_context.origin_allowed,
        csrf_valid: request.request_context.csrf_valid,
        web_access_enabled: request.request_context.web_access_enabled,
        web_auth_available: request.request_context.web_auth_available,
        session_cookie: request
            .headers
            .get("Cookie")
            .and_then(|value| value.strip_prefix("jftrade_web_session="))
            .map(ToOwned::to_owned),
    }
}

fn port_response(
    expected: &FixtureExpected,
) -> Result<AuthSessionWritePortResult, AuthSessionWritePortError> {
    let error_message = || {
        expected.envelope["error"]["message"]
            .as_str()
            .unwrap_or("fixture auth-session-write error")
            .to_owned()
    };
    match expected.port_error.as_deref() {
        None => Ok(AuthSessionWritePortResult {
            data: expected.envelope["data"].clone(),
            set_cookie: expected.response_headers.get("set-cookie").cloned(),
        }),
        Some("unavailable") => Err(AuthSessionWritePortError::Unavailable(error_message())),
        Some("invalid-password") => {
            Err(AuthSessionWritePortError::InvalidPassword(error_message()))
        }
        Some("canceled") => Err(AuthSessionWritePortError::Canceled(error_message())),
        Some("configuration-changed") => Err(AuthSessionWritePortError::ConfigurationChanged(
            error_message(),
        )),
        Some("failed") => Err(AuthSessionWritePortError::Failed(error_message())),
        Some(other) => panic!("unexpected auth-session-write port error {other}"),
    }
}

fn owned_headers(headers: &BTreeMap<String, String>) -> BTreeMap<String, String> {
    headers
        .iter()
        .map(|(name, value)| (name.to_ascii_lowercase(), value.clone()))
        .collect()
}

fn owned_expected_headers(expected: &FixtureExpected) -> BTreeMap<String, String> {
    ["cache-control", "content-type", "retry-after", "set-cookie"]
        .into_iter()
        .filter_map(|name| {
            expected
                .response_headers
                .get(name)
                .map(|value| (name.to_owned(), value.clone()))
        })
        .collect()
}
