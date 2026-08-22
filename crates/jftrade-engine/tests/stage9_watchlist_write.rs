#[path = "../src/product_watchlist_write_port.rs"]
mod product_watchlist_write_port;

use std::collections::{BTreeMap, VecDeque};
use std::sync::Mutex;

use product_watchlist_write_port::{
    WATCHLIST_WRITE_ROUTES, WatchlistWriteMutation, WatchlistWritePort, WatchlistWritePortError,
    WatchlistWriteRequest, dispatch_watchlist_write, watchlist_write_routes,
};
use serde::Deserialize;
use serde_json::Value;

const FIXTURE_TIMESTAMP: &str = "2026-08-23T00:00:00Z";

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
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
    expected: Vec<FixtureExpected>,
    #[serde(default)]
    calls: Vec<Value>,
    #[serde(default)]
    port_mode: String,
    #[serde(default)]
    concurrent: bool,
    expected_observation: Value,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureRequest {
    method: String,
    path: String,
    body: Option<String>,
    #[serde(default)]
    context: String,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureExpected {
    status: u16,
    headers: BTreeMap<String, String>,
    envelope: Value,
    port_call: bool,
}

#[derive(Debug)]
struct FixturePort {
    responses: Mutex<VecDeque<Result<Value, WatchlistWritePortError>>>,
    calls: Mutex<Vec<Value>>,
}

impl FixturePort {
    fn from_case(case: &FixtureCase) -> Self {
        let responses = case
            .expected
            .iter()
            .filter(|expected| expected.port_call)
            .map(|expected| {
                if expected.envelope["ok"] == true {
                    Ok(expected.envelope["data"].clone())
                } else {
                    Err(error_from_envelope(expected))
                }
            })
            .collect();
        Self {
            responses: Mutex::new(responses),
            calls: Mutex::new(Vec::new()),
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

impl WatchlistWritePort for FixturePort {
    fn mutate(&self, mutation: &WatchlistWriteMutation) -> Result<Value, WatchlistWritePortError> {
        self.calls
            .lock()
            .expect("watchlist write calls lock")
            .push(mutation.value.clone());
        self.responses
            .lock()
            .expect("watchlist write response lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(WatchlistWritePortError {
                    status: 500,
                    code: "WATCHLIST_FAILED".to_owned(),
                    message: "fixture response missing".to_owned(),
                })
            })
    }
}

#[test]
fn watchlist_write_fixture_matches_go_owner_for_all_eight_routes() {
    let fixture = fixture();
    assert_eq!(fixture.version, "stage9.watchlist-write.v1");
    assert_eq!(fixture.timestamp, FIXTURE_TIMESTAMP);
    assert_eq!(fixture.cases.len(), 45);
    assert_eq!(watchlist_write_routes(), &WATCHLIST_WRITE_ROUTES);

    for case in &fixture.cases {
        assert_eq!(
            case.requests.len(),
            case.expected.len(),
            "case {}",
            case.name
        );
        assert!(
            !case.expected_observation.is_null(),
            "case {} lacks rollback/state observation",
            case.name
        );
        let port = FixturePort::from_case(case);
        let port_ref = if case.port_mode == "no-port" {
            None
        } else {
            Some(&port as &dyn WatchlistWritePort)
        };

        for (request, expected) in case.requests.iter().zip(&case.expected) {
            let _ = &request.context;
            let response = dispatch_watchlist_write(
                &WatchlistWriteRequest {
                    method: request.method.clone(),
                    path: request.path.clone(),
                    body: request.body.as_ref().map(|body| body.as_bytes().to_vec()),
                },
                port_ref,
                FIXTURE_TIMESTAMP,
            );
            assert_eq!(response.status, expected.status, "case {}", case.name);
            assert_eq!(response.headers, expected.headers, "case {}", case.name);
            assert_eq!(response.body, expected.envelope, "case {}", case.name);
        }

        let calls = port.calls.lock().expect("watchlist write calls lock");
        assert_eq!(
            calls.as_slice(),
            case.calls.as_slice(),
            "case {} calls",
            case.name
        );
        assert_eq!(
            calls.len(),
            case.expected
                .iter()
                .filter(|expected| expected.port_call)
                .count(),
            "case {} port-call count",
            case.name
        );
        drop(calls);
        port.assert_drained(&case.name);
    }
}

#[test]
fn watchlist_write_fixture_covers_concurrency_and_every_route() {
    let fixture = fixture();
    assert!(
        fixture.cases.iter().any(|case| case.concurrent),
        "revision-fence fixture case is missing"
    );
    let covered = fixture
        .cases
        .iter()
        .flat_map(|case| {
            case.requests
                .iter()
                .map(|request| (request.method.as_str(), request.path.as_str()))
        })
        .collect::<Vec<_>>();
    for (method, path) in WATCHLIST_WRITE_ROUTES {
        let found = covered.iter().any(|(covered_method, covered_path)| {
            *covered_method == method && route_template_matches(covered_path, path)
        });
        assert!(found, "fixture does not cover {method} {path}");
    }
}

fn route_template_matches(path: &str, template: &str) -> bool {
    let path = path.split('?').next().unwrap_or(path);
    let path_parts = path.split('/').collect::<Vec<_>>();
    let template_parts = template.split('/').collect::<Vec<_>>();
    path_parts.len() == template_parts.len()
        && path_parts
            .iter()
            .zip(template_parts)
            .all(|(actual, expected)| expected.starts_with('{') || actual == &expected)
}

fn fixture() -> Fixture {
    serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/watchlist-write.json"
    ))
    .expect("watchlist-write fixture")
}

fn error_from_envelope(expected: &FixtureExpected) -> WatchlistWritePortError {
    WatchlistWritePortError {
        status: expected.status,
        code: expected.envelope["error"]["code"]
            .as_str()
            .unwrap_or("WATCHLIST_FAILED")
            .to_owned(),
        message: expected.envelope["error"]["message"]
            .as_str()
            .unwrap_or_default()
            .to_owned(),
    }
}
