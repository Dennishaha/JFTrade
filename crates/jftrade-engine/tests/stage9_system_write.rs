#[path = "../src/product_system_write_port.rs"]
mod product_system_write_port;

use std::collections::VecDeque;
use std::sync::Mutex;

use product_system_write_port::{
    SYSTEM_WRITE_ROUTES, SystemWriteInput, SystemWriteOperation, SystemWritePort,
    SystemWritePortError, SystemWriteRequest, dispatch_system_write, system_write_routes,
};
use serde::Deserialize;
use serde_json::{Value, json};

const FIXTURE_TIMESTAMP: &str = "2026-08-23T10:00:00Z";

#[derive(Debug, Deserialize)]
struct Fixture {
    version: String,
    cases: Vec<FixtureCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureCase {
    name: String,
    method: String,
    request_paths: Vec<String>,
    request_bodies: Vec<String>,
    #[serde(default)]
    expected_statuses: Vec<u16>,
    port_calls: Vec<bool>,
    response_headers: Vec<std::collections::BTreeMap<String, String>>,
    responses: Vec<Value>,
    expected_observation: Value,
}

#[derive(Debug)]
struct FixturePort {
    responses: Mutex<VecDeque<Result<Value, SystemWritePortError>>>,
    calls: Mutex<Vec<SystemWriteInput>>,
}

impl FixturePort {
    fn from_case(case: &FixtureCase) -> Self {
        let responses = case
            .responses
            .iter()
            .enumerate()
            .filter_map(|(index, response)| {
                if !case.port_calls[index] {
                    return None;
                }
                Some(if response["ok"] == true {
                    Ok(response["data"].clone())
                } else {
                    Err(error_from_response(response, case.expected_statuses[index]))
                })
            })
            .collect();
        Self {
            responses: Mutex::new(responses),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn calls(&self) -> Vec<SystemWriteInput> {
        self.calls.lock().expect("system write calls lock").clone()
    }
}

impl SystemWritePort for FixturePort {
    fn mutate(&self, input: &SystemWriteInput) -> Result<Value, SystemWritePortError> {
        self.calls
            .lock()
            .expect("system write calls lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("system write responses lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(SystemWritePortError::Unavailable(
                    "fixture response missing".to_owned(),
                ))
            })
    }
}

fn fixture() -> Fixture {
    let fixture: Fixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/system-write.json"
    ))
    .expect("system-write fixture");
    assert_eq!(fixture.version, "stage9.system-write.v1");
    fixture
}

fn request(case: &FixtureCase, index: usize) -> SystemWriteRequest {
    SystemWriteRequest {
        method: case.method.clone(),
        path: case.request_paths[index].clone(),
        body: case.request_bodies[index].as_bytes().to_vec(),
    }
}

fn error_from_response(response: &Value, status: u16) -> SystemWritePortError {
    SystemWritePortError::Failed {
        status,
        code: response["error"]["code"]
            .as_str()
            .unwrap_or("SYSTEM_WRITE_FAILED")
            .to_owned(),
        message: response["error"]["message"]
            .as_str()
            .unwrap_or_default()
            .to_owned(),
    }
}

#[test]
fn system_write_fixture_replays_go_wire_for_all_seven_routes() {
    let fixture = fixture();
    assert_eq!(fixture.cases.len(), 47);
    let mut requests = 0;
    for case in &fixture.cases {
        assert_eq!(
            case.request_paths.len(),
            case.request_bodies.len(),
            "{}",
            case.name
        );
        assert_eq!(
            case.request_paths.len(),
            case.expected_statuses.len(),
            "{}",
            case.name
        );
        assert_eq!(
            case.request_paths.len(),
            case.port_calls.len(),
            "{}",
            case.name
        );
        assert_eq!(
            case.request_paths.len(),
            case.response_headers.len(),
            "{}",
            case.name
        );
        assert_eq!(
            case.request_paths.len(),
            case.responses.len(),
            "{}",
            case.name
        );
        requests += case.request_paths.len();

        let port = FixturePort::from_case(case);
        for index in 0..case.request_paths.len() {
            let response =
                dispatch_system_write(&request(case, index), Some(&port), FIXTURE_TIMESTAMP);
            assert_eq!(
                response.status, case.expected_statuses[index],
                "case {} request {index}",
                case.name
            );
            assert_eq!(
                response.headers, case.response_headers[index],
                "case {} request {index}",
                case.name
            );
            assert_eq!(
                response.body, case.responses[index],
                "case {} request {index}",
                case.name
            );
        }
        assert_eq!(
            port.calls().len(),
            case.port_calls.iter().filter(|called| **called).count(),
            "case {} calls",
            case.name
        );
        assert_eq!(
            system_write_observation(&port.calls()),
            case.expected_observation,
            "case {} observation",
            case.name
        );
    }
    assert_eq!(requests, 68);
}

#[test]
fn system_write_fixture_covers_exact_route_inventory_and_isolates_reads() {
    let fixture = fixture();
    assert_eq!(system_write_routes(), &SYSTEM_WRITE_ROUTES);
    assert_eq!(system_write_routes().len(), 7);
    let valid_cases = fixture
        .cases
        .iter()
        .enumerate()
        .flat_map(|(case_index, case)| {
            case.request_paths
                .iter()
                .enumerate()
                .filter(move |(request_index, _)| case.port_calls[*request_index])
                .map(move |(request_index, path)| (&case.method, path, case_index, request_index))
        })
        .collect::<Vec<_>>();
    for (method, template) in system_write_routes() {
        assert!(
            valid_cases.iter().any(|(actual_method, path, _, _)| {
                actual_method == method && route_template_matches(path, template)
            }),
            "fixture does not cover {method} {template}"
        );
    }

    let read_request = SystemWriteRequest {
        method: "GET".to_owned(),
        path: "/api/v1/system/real-trade-risk-limits".to_owned(),
        body: Vec::new(),
    };
    let response = dispatch_system_write(&read_request, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 404);
    assert_eq!(response.body["error"]["code"], "NOT_FOUND");
}

#[test]
fn system_write_leaf_fails_closed_after_shape_validation() {
    let fixture = fixture();
    let valid_case = fixture
        .cases
        .iter()
        .find(|case| case.name == "risk-update-success-unknown-field")
        .expect("valid system write fixture case");
    let unavailable = dispatch_system_write(&request(valid_case, 0), None, FIXTURE_TIMESTAMP);
    assert_eq!(unavailable.status, 503);
    assert_eq!(
        unavailable.body["error"]["code"],
        "SYSTEM_WRITE_UNAVAILABLE"
    );

    let malformed = SystemWriteRequest {
        method: "PUT".to_owned(),
        path: "/api/v1/system/real-trade-risk-limits".to_owned(),
        body: b"{".to_vec(),
    };
    let response = dispatch_system_write(&malformed, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 400);
    assert_eq!(response.body["error"]["code"], "BAD_REQUEST");
}

#[test]
fn system_write_port_unavailable_maps_to_fail_closed_503() {
    let request = SystemWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/system/futu-opend/manual-retry".to_owned(),
        body: b"malformed body is ignored by Go".to_vec(),
    };
    let port = UnavailablePort;
    let response = dispatch_system_write(&request, Some(&port), FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(response.body["error"]["code"], "SYSTEM_WRITE_UNAVAILABLE");
}

#[derive(Debug)]
struct UnavailablePort;

impl SystemWritePort for UnavailablePort {
    fn mutate(&self, _input: &SystemWriteInput) -> Result<Value, SystemWritePortError> {
        Err(SystemWritePortError::Unavailable(
            "injected OpenD/control port unavailable".to_owned(),
        ))
    }
}

fn system_write_observation(calls: &[SystemWriteInput]) -> Value {
    let mut observation = json!({
        "resetCalls": [],
        "riskUpdateCalls": [],
        "riskDisableCalls": [],
        "killActivateCalls": [],
        "killReleaseCalls": [],
        "hardActivateCalls": [],
        "hardReleaseCalls": [],
    });
    for input in calls {
        match input.operation {
            SystemWriteOperation::ManualRetry => {
                observation["resetCalls"]
                    .as_array_mut()
                    .expect("reset calls array")
                    .push(Value::String("manual-retry".to_owned()));
            }
            SystemWriteOperation::UpdateRisk => observation["riskUpdateCalls"]
                .as_array_mut()
                .expect("risk update calls array")
                .push(json!({"command": input.risk})),
            SystemWriteOperation::DisableRisk => observation["riskDisableCalls"]
                .as_array_mut()
                .expect("risk disable calls array")
                .push(json!({"command": input.risk})),
            SystemWriteOperation::ActivateKillSwitch => observation["killActivateCalls"]
                .as_array_mut()
                .expect("kill activate calls array")
                .push(json!({"command": input.kill_switch})),
            SystemWriteOperation::ReleaseKillSwitch => observation["killReleaseCalls"]
                .as_array_mut()
                .expect("kill release calls array")
                .push(json!({"command": input.kill_switch})),
            SystemWriteOperation::ActivateHardStop => observation["hardActivateCalls"]
                .as_array_mut()
                .expect("hard activate calls array")
                .push(json!({"command": input.hard_stop})),
            SystemWriteOperation::ReleaseHardStop => observation["hardReleaseCalls"]
                .as_array_mut()
                .expect("hard release calls array")
                .push(json!({
                    "hardStopId": input.hard_stop_id,
                    "command": input.hard_stop,
                })),
        }
    }
    observation
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
