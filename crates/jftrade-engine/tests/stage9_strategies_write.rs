#[path = "../src/product_strategy_runtime_write_port.rs"]
mod product_strategy_runtime_write_port;

use std::collections::{BTreeMap, VecDeque};
use std::sync::{Arc, Mutex};

use jftrade_api::ApiRequest;
use product_strategy_runtime_write_port::{
    StrategyRuntimeWriteInput, StrategyRuntimeWriteOperation, StrategyRuntimeWritePort,
    StrategyRuntimeWritePortError, dispatch_strategy_runtime_write, strategy_runtime_write_routes,
};
use serde::Deserialize;
use serde_json::Value;

const FIXTURE_TIMESTAMP: &str = "2026-08-23T06:00:00Z";

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
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
    responses: Vec<Value>,
    expected_observation: Value,
}

#[derive(Debug)]
struct FixturePort {
    responses: Mutex<VecDeque<Result<Value, StrategyRuntimeWritePortError>>>,
    calls: Mutex<Vec<StrategyRuntimeWriteInput>>,
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

    fn calls(&self) -> Vec<StrategyRuntimeWriteInput> {
        self.calls
            .lock()
            .expect("strategy runtime write calls lock")
            .clone()
    }
}

impl StrategyRuntimeWritePort for FixturePort {
    fn mutate(
        &self,
        input: &StrategyRuntimeWriteInput,
    ) -> Result<Value, StrategyRuntimeWritePortError> {
        self.calls
            .lock()
            .expect("strategy runtime write calls lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("strategy runtime write responses lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(StrategyRuntimeWritePortError::Unavailable(
                    "fixture response missing".to_owned(),
                ))
            })
    }
}

fn fixture() -> Fixture {
    let fixture: Fixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/strategies-write.json"
    ))
    .expect("strategies-write fixture");
    assert_eq!(fixture.version, "stage9.strategies-write.v1");
    fixture
}

fn request(case: &FixtureCase, index: usize) -> ApiRequest {
    ApiRequest {
        method: case.method.clone(),
        path: case.request_paths[index].clone(),
        query: String::new(),
        body: case.request_bodies[index].as_bytes().to_vec(),
        request_id: "stage9-strategies-write".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
        csrf_valid: false,
        session_cookie: None,
    }
}

fn error_from_response(response: &Value, status: u16) -> StrategyRuntimeWritePortError {
    let error = &response["error"];
    StrategyRuntimeWritePortError::Failed {
        status,
        code: error["code"]
            .as_str()
            .unwrap_or("STRATEGY_FAILED")
            .to_owned(),
        message: error["message"].as_str().unwrap_or_default().to_owned(),
    }
}

#[test]
fn strategies_runtime_write_fixture_matches_go_owner_for_all_seven_routes() {
    let fixture = fixture();
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
            case.responses.len(),
            "{}",
            case.name
        );

        let port = Arc::new(FixturePort::from_case(case));
        let mut previous_calls = 0;
        for index in 0..case.request_paths.len() {
            let response = dispatch_strategy_runtime_write(
                &request(case, index),
                Some(port.as_ref()),
                FIXTURE_TIMESTAMP,
            );
            assert_eq!(
                response.status, case.expected_statuses[index],
                "{}",
                case.name
            );
            assert_eq!(
                response.headers,
                BTreeMap::from([(
                    "Content-Type".to_owned(),
                    "application/json; charset=utf-8".to_owned(),
                )]),
                "{}",
                case.name
            );
            let mut expected = case.responses[index].clone();
            expected["timestamp"] = Value::String(FIXTURE_TIMESTAMP.to_owned());
            assert_eq!(response.body, expected, "{}", case.name);

            let calls = port.calls();
            let actual_delta = calls.len() - previous_calls;
            assert_eq!(
                actual_delta,
                usize::from(case.port_calls[index]),
                "{}",
                case.name
            );
            if case.port_calls[index] {
                assert_runtime_write_input(case, &calls[previous_calls], index);
            }
            previous_calls = calls.len();
        }
        assert_eq!(
            port.calls().len(),
            case.port_calls.iter().filter(|called| **called).count()
        );
        assert_boundary_observation(case, &port.calls());
    }
}

fn assert_runtime_write_input(case: &FixtureCase, input: &StrategyRuntimeWriteInput, index: usize) {
    let path = &case.request_paths[index];
    let suffix = path
        .strip_prefix("/api/v1/strategies/")
        .expect("strategy route prefix");
    let mut parts = suffix.split('/');
    let instance_id = parts.next().expect("instance id");
    assert_eq!(input.instance_id, instance_id, "{}", case.name);
    let operation = match parts.next() {
        None => match case.method.as_str() {
            "PUT" => StrategyRuntimeWriteOperation::Update,
            "DELETE" => StrategyRuntimeWriteOperation::Delete,
            method => panic!("unexpected base method {method} in {}", case.name),
        },
        Some("runtime-risk") => StrategyRuntimeWriteOperation::UpdateRuntimeRisk,
        Some("pause") => StrategyRuntimeWriteOperation::Pause,
        Some("stop") => StrategyRuntimeWriteOperation::Stop,
        Some("start") => StrategyRuntimeWriteOperation::Start,
        Some("refresh-definition") => StrategyRuntimeWriteOperation::RefreshDefinition,
        Some(action) => panic!("unexpected action {action} in {}", case.name),
    };
    assert_eq!(input.operation, operation, "{}", case.name);

    if operation == StrategyRuntimeWriteOperation::Update {
        let expected = case.expected_observation["updateCalls"]
            .as_array()
            .expect("update observation")
            .first()
            .map(|call| call["binding"].clone())
            .expect("update binding observation");
        assert_eq!(input.binding.as_ref(), Some(&expected), "{}", case.name);
    }
    if operation == StrategyRuntimeWriteOperation::UpdateRuntimeRisk {
        let expected = case.expected_observation["runtimeRiskCalls"]
            .as_array()
            .expect("risk observation")
            .first()
            .map(|call| call["risk"].clone())
            .expect("risk observation value");
        assert_eq!(
            input.runtime_risk.as_ref(),
            Some(&expected),
            "{}",
            case.name
        );
    }
}

fn assert_boundary_observation(case: &FixtureCase, calls: &[StrategyRuntimeWriteInput]) {
    let expected = &case.expected_observation;
    let expected_update_count = expected["updateCalls"].as_array().map_or(0, Vec::len);
    let expected_risk_count = expected["runtimeRiskCalls"].as_array().map_or(0, Vec::len);
    let expected_delete_count = expected["deleteCalls"].as_array().map_or(0, Vec::len);
    let expected_refresh_count = expected["refreshCalls"].as_array().map_or(0, Vec::len);
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation == StrategyRuntimeWriteOperation::Update)
            .count(),
        expected_update_count,
        "{}",
        case.name
    );
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation == StrategyRuntimeWriteOperation::UpdateRuntimeRisk)
            .count(),
        expected_risk_count,
        "{}",
        case.name
    );
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation == StrategyRuntimeWriteOperation::Delete)
            .count(),
        expected_delete_count,
        "{}",
        case.name
    );
    assert_eq!(
        calls
            .iter()
            .filter(|call| call.operation == StrategyRuntimeWriteOperation::RefreshDefinition)
            .count(),
        expected_refresh_count,
        "{}",
        case.name
    );
}

#[test]
fn strategies_runtime_write_leaf_has_exact_route_inventory_and_isolates_reads() {
    assert_eq!(strategy_runtime_write_routes().len(), 7);
    assert!(
        strategy_runtime_write_routes()
            .contains(&("POST", "/api/v1/strategies/{instanceId}/refresh-definition"))
    );
    let read = ApiRequest {
        method: "GET".to_owned(),
        path: "/api/v1/strategies".to_owned(),
        query: String::new(),
        body: Vec::new(),
        request_id: "strategies-runtime-write-read-isolation".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
        csrf_valid: false,
        session_cookie: None,
    };
    let response = dispatch_strategy_runtime_write(&read, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 404);
    assert_eq!(response.body["error"]["code"], "NOT_FOUND");
}

#[test]
fn strategies_runtime_write_leaf_fails_closed_without_test_port() {
    let request = ApiRequest {
        method: "POST".to_owned(),
        path: "/api/v1/strategies/fixture-instance/start".to_owned(),
        query: String::new(),
        body: b"malformed body is ignored by Go control routes".to_vec(),
        request_id: "strategies-runtime-write-unavailable".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
        csrf_valid: false,
        session_cookie: None,
    };
    let response = dispatch_strategy_runtime_write(&request, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(response.body["error"]["code"], "STRATEGY_UNAVAILABLE");
}
