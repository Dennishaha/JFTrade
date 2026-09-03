#[path = "../src/product_watchlist_remote_write_port.rs"]
mod product_watchlist_remote_write_port;

use std::collections::{BTreeMap, VecDeque};
use std::sync::Mutex;

use product_watchlist_remote_write_port::{
    REMOTE_WATCHLIST_WRITE_PATH, RemoteWatchlistWriteAction, RemoteWatchlistWritePayloadState,
    RemoteWatchlistWritePort, RemoteWatchlistWritePortError, RemoteWatchlistWriteRequest,
    RemoteWatchlistWriteResolution, dispatch_remote_watchlist_write, remote_watchlist_write_routes,
};
use serde::Deserialize;
use serde_json::{Value, json};

const FIXTURE_TIMESTAMP: &str = "2026-08-22T04:00:00Z";

#[derive(Debug, Deserialize)]
struct Fixture {
    version: String,
    timestamp: String,
    cases: Vec<FixtureCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureCase {
    name: String,
    requests: Vec<FixtureRequest>,
    feature_id: String,
    action: String,
    port_mode: String,
    expected: Vec<FixtureExpected>,
    calls: FixtureCalls,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureRequest {
    method: String,
    request_path: String,
    body: Option<String>,
    #[serde(default)]
    context: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureExpected {
    status: u16,
    headers: BTreeMap<String, String>,
    port_call: bool,
    envelope: Value,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureCalls {
    apply: usize,
    #[serde(default)]
    actions: Vec<Value>,
    #[serde(default)]
    payload_state: Vec<String>,
}

#[derive(Clone, Debug)]
enum FixtureResponse {
    Data(Value),
    Error(RemoteWatchlistWritePortError),
}

#[derive(Debug)]
struct FixturePort {
    mode: String,
    responses: Mutex<VecDeque<FixtureResponse>>,
    actions: Mutex<Vec<RemoteWatchlistWriteAction>>,
}

impl FixturePort {
    fn from_case(case: &FixtureCase) -> Self {
        let mut responses = VecDeque::new();
        for expected in &case.expected {
            if !expected.port_call {
                continue;
            }
            if expected.envelope["ok"] == true {
                responses.push_back(FixtureResponse::Data(expected.envelope["data"].clone()));
            } else {
                responses.push_back(FixtureResponse::Error(error_from_envelope(expected)));
            }
        }
        Self {
            mode: case.port_mode.clone(),
            responses: Mutex::new(responses),
            actions: Mutex::new(Vec::new()),
        }
    }

    fn assert_drained(&self, case_name: &str) {
        assert!(
            self.responses
                .lock()
                .expect("watchlist write response lock")
                .is_empty(),
            "fixture port responses remain for {case_name}"
        );
    }
}

impl RemoteWatchlistWritePort for FixturePort {
    fn resolve(
        &self,
        broker_id: Option<&str>,
        _account_id: Option<&str>,
    ) -> Result<RemoteWatchlistWriteResolution, RemoteWatchlistWritePortError> {
        match self.mode.as_str() {
            "unavailable" => Err(RemoteWatchlistWritePortError::Unavailable(
                "remote watchlist write port is unavailable".to_owned(),
            )),
            "missing-broker" => Err(RemoteWatchlistWritePortError::CapabilityUnavailable(
                "broker feature capability is unavailable: broker \"missing\" is not registered"
                    .to_owned(),
            )),
            "capability-unavailable" => Err(RemoteWatchlistWritePortError::CapabilityUnavailable(
                "broker feature capability is unavailable: broker \"futu\" feature \"watchlist.remote.modify\" is unavailable: "
                    .to_owned(),
            )),
            "adapter-unavailable" => Err(RemoteWatchlistWritePortError::CapabilityUnavailable(
                "broker feature capability is unavailable: broker \"futu\" feature \"watchlist.remote.modify\" is unavailable: adapter interface CustomizationService is not implemented"
                    .to_owned(),
            )),
            _ => Ok(RemoteWatchlistWriteResolution {
                broker_id: "futu".to_owned(),
                security_firm: "Futu/Moomoo via OpenD".to_owned(),
                capability: "available".to_owned(),
                selection_reason: if broker_id.is_some() {
                    "explicit_broker".to_owned()
                } else {
                    "default_broker".to_owned()
                },
            }),
        }
    }

    fn apply(
        &self,
        _resolution: &RemoteWatchlistWriteResolution,
        action: &RemoteWatchlistWriteAction,
    ) -> Result<Option<Value>, RemoteWatchlistWritePortError> {
        self.actions
            .lock()
            .expect("watchlist write action lock")
            .push(action.clone());
        match self
            .responses
            .lock()
            .expect("watchlist write response lock")
            .pop_front()
            .unwrap_or_else(|| {
                FixtureResponse::Error(RemoteWatchlistWritePortError::Unavailable(
                    "fixture response missing".to_owned(),
                ))
            }) {
            FixtureResponse::Data(data) => Ok(Some(data)),
            FixtureResponse::Error(error) => Err(error),
        }
    }
}

#[test]
fn remote_watchlist_write_fixture_matches_go_owner_for_the_single_route() {
    let fixture = fixture();
    assert_eq!(fixture.timestamp, FIXTURE_TIMESTAMP);
    assert_eq!(fixture.cases.len(), 19);

    for case in &fixture.cases {
        assert_eq!(case.feature_id, "watchlist.remote.modify");
        assert_eq!(case.action, "modify");
        assert_eq!(
            case.requests.len(),
            case.expected.len(),
            "case {}",
            case.name
        );
        let port = FixturePort::from_case(case);
        for (request, expected) in case.requests.iter().zip(&case.expected) {
            let _ = &request.context;
            let response = dispatch_remote_watchlist_write(
                &RemoteWatchlistWriteRequest {
                    method: request.method.clone(),
                    path: request.request_path.clone(),
                    body: request.body.as_ref().map(|body| body.as_bytes().to_vec()),
                },
                Some(&port),
                FIXTURE_TIMESTAMP,
            );
            assert_eq!(response.status, expected.status, "case {}", case.name);
            assert_eq!(response.headers, expected.headers, "case {}", case.name);
            assert_eq!(response.body, expected.envelope, "case {}", case.name);
        }
        let actions = port.actions.lock().expect("watchlist write action lock");
        assert_eq!(
            actions.len(),
            case.calls.apply,
            "case {} apply calls",
            case.name
        );
        assert_eq!(
            actions.iter().map(action_json).collect::<Vec<_>>(),
            case.calls.actions,
            "case {} action trace",
            case.name
        );
        assert_eq!(
            actions.iter().map(payload_state_name).collect::<Vec<_>>(),
            case.calls.payload_state,
            "case {} payload states",
            case.name
        );
        drop(actions);
        port.assert_drained(&case.name);
    }
}

#[test]
fn remote_watchlist_write_leaf_fails_closed_without_a_test_port() {
    let request = RemoteWatchlistWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/watchlists/remote?brokerId=futu".to_owned(),
        body: Some(br#"{"groupName":"Favorites","op":1}"#.to_vec()),
    };
    let response = dispatch_remote_watchlist_write(&request, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(
        response.body["error"]["code"],
        "WATCHLIST_REMOTE_WRITE_UNAVAILABLE"
    );
}

#[test]
fn remote_watchlist_write_leaf_has_exact_route_and_error_precedence() {
    assert_eq!(
        remote_watchlist_write_routes(),
        &[("POST", REMOTE_WATCHLIST_WRITE_PATH)]
    );
    let malformed = RemoteWatchlistWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/watchlists/remote?brokerId=missing".to_owned(),
        body: Some(b"{".to_vec()),
    };
    let response = dispatch_remote_watchlist_write(&malformed, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 400);
    assert_eq!(response.body["error"]["code"], "BAD_REQUEST");

    let wrong_method = RemoteWatchlistWriteRequest {
        method: "GET".to_owned(),
        path: REMOTE_WATCHLIST_WRITE_PATH.to_owned(),
        body: None,
    };
    assert_eq!(
        dispatch_remote_watchlist_write(&wrong_method, None, FIXTURE_TIMESTAMP).status,
        404
    );
}

#[test]
fn remote_watchlist_write_leaf_replays_failure_recovery_and_duplicate_forwarding() {
    let fixture = fixture();
    for case_name in [
        "repeated-write-is-forwarded-twice",
        "failed-write-recovers-on-next-request",
    ] {
        let case = fixture
            .cases
            .iter()
            .find(|case| case.name == case_name)
            .expect("recovery fixture case");
        let port = FixturePort::from_case(case);
        for (request, expected) in case.requests.iter().zip(&case.expected) {
            let response = dispatch_remote_watchlist_write(
                &RemoteWatchlistWriteRequest {
                    method: request.method.clone(),
                    path: request.request_path.clone(),
                    body: request.body.as_ref().map(|body| body.as_bytes().to_vec()),
                },
                Some(&port),
                FIXTURE_TIMESTAMP,
            );
            assert_eq!(response.status, expected.status, "case {case_name}");
            assert_eq!(response.body, expected.envelope, "case {case_name}");
        }
        assert_eq!(
            port.actions.lock().expect("watchlist write actions").len(),
            case.calls.apply,
            "case {case_name} apply count"
        );
        port.assert_drained(case_name);
    }
}

fn fixture() -> Fixture {
    let fixture: Fixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/watchlists-remote-write.json"
    ))
    .expect("watchlists-remote-write fixture");
    assert_eq!(fixture.version, "stage9.watchlists-remote-write.v1");
    fixture
}

fn error_from_envelope(expected: &FixtureExpected) -> RemoteWatchlistWritePortError {
    let code = expected.envelope["error"]["code"]
        .as_str()
        .expect("fixture error code");
    let message = expected.envelope["error"]["message"]
        .as_str()
        .expect("fixture error message")
        .to_owned();
    match code {
        "PROVIDER_REQUEST_FAILED" => RemoteWatchlistWritePortError::Provider {
            status: Some(expected.status),
            message,
        },
        "BROKER_FEATURE_FAILED" => RemoteWatchlistWritePortError::Internal(message),
        "MARKET_SNAPSHOT_RATE_LIMITED" => RemoteWatchlistWritePortError::RateLimited {
            retry_after: expected.headers["Retry-After"]
                .parse()
                .expect("retry after"),
            message,
        },
        other => panic!("unexpected remote watchlist write fixture error {other}"),
    }
}

fn action_json(action: &RemoteWatchlistWriteAction) -> Value {
    let mut result = json!({
        "featureId": action.feature_id,
        "brokerId": action.broker_id,
        "action": action.action,
    });
    if let Some(account_id) = action.account_id.as_deref() {
        result["accountId"] = json!(account_id);
    }
    if let Some(Value::Object(payload)) = action.payload.as_ref()
        && !payload.is_empty()
    {
        result["payload"] = Value::Object(payload.clone());
    }
    result
}

fn payload_state_name(action: &RemoteWatchlistWriteAction) -> String {
    match action.payload_state {
        RemoteWatchlistWritePayloadState::Nil => "nil".to_owned(),
        RemoteWatchlistWritePayloadState::EmptyObject => "empty_object".to_owned(),
        RemoteWatchlistWritePayloadState::Object => "object".to_owned(),
    }
}
