#[path = "../src/product_strategy_definition_write_port.rs"]
mod product_strategy_definition_write_port;

use std::collections::{BTreeMap, VecDeque};
use std::sync::{Arc, Mutex};

use jftrade_api::ApiRequest;
use product_strategy_definition_write_port::{
    StrategyDefinitionWriteInput, StrategyDefinitionWritePort, StrategyDefinitionWritePortError,
    dispatch_strategy_definition_write, strategy_definition_write_routes,
};
use serde::Deserialize;
use serde_json::Value;

const FIXTURE_TIMESTAMP: &str = "2026-08-22T06:00:00Z";

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
    expected_statuses: Vec<u16>,
    responses: Vec<Value>,
}

#[derive(Debug)]
struct FixturePort {
    responses: Mutex<VecDeque<Result<Value, StrategyDefinitionWritePortError>>>,
    calls: Mutex<Vec<StrategyDefinitionWriteInput>>,
}

impl FixturePort {
    fn from_case(case: &FixtureCase) -> Self {
        let responses = case
            .responses
            .iter()
            .enumerate()
            .filter_map(|(index, response)| {
                if !case_port_call(case, index) {
                    return None;
                }
                Some(if response["ok"] == true {
                    Ok(response["data"].clone())
                } else {
                    Err(error_from_response(response, case.expected_statuses[index]))
                })
            })
            .collect::<VecDeque<_>>();
        Self {
            responses: Mutex::new(responses),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn call_count(&self) -> usize {
        self.calls.lock().expect("strategy write calls lock").len()
    }
}

impl StrategyDefinitionWritePort for FixturePort {
    fn mutate(
        &self,
        input: &StrategyDefinitionWriteInput,
    ) -> Result<Value, StrategyDefinitionWritePortError> {
        self.calls
            .lock()
            .expect("strategy write calls lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("strategy write responses lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(StrategyDefinitionWritePortError::Unavailable(
                    "fixture response missing".to_owned(),
                ))
            })
    }
}

fn fixture() -> Fixture {
    let fixture: Fixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/strategy-definitions-write.json"
    ))
    .expect("strategy-definitions-write fixture");
    assert_eq!(fixture.version, "stage9.strategy-definitions-write.v1");
    fixture
}

fn request(case: &FixtureCase, index: usize) -> ApiRequest {
    ApiRequest {
        method: case.method.clone(),
        path: case.request_paths[index].clone(),
        query: String::new(),
        body: case.request_bodies[index].as_bytes().to_vec(),
        request_id: "stage9-strategy-definitions-write".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
        csrf_valid: false,
        session_cookie: None,
    }
}

fn case_port_call(case: &FixtureCase, _index: usize) -> bool {
    !matches!(
        case.name.as_str(),
        "create-malformed-body"
            | "create-malformed-body-before-unavailable"
            | "update-malformed-body"
    )
}

fn error_from_response(response: &Value, status: u16) -> StrategyDefinitionWritePortError {
    let error = &response["error"];
    StrategyDefinitionWritePortError::Failed {
        status,
        code: error["code"]
            .as_str()
            .unwrap_or("STRATEGY_FAILED")
            .to_owned(),
        message: error["message"].as_str().unwrap_or_default().to_owned(),
    }
}

#[test]
fn strategy_definition_write_fixture_matches_go_owner_for_all_five_routes() {
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
            case.responses.len(),
            "{}",
            case.name
        );
        let port = Arc::new(FixturePort::from_case(case));
        for index in 0..case.request_paths.len() {
            let response = dispatch_strategy_definition_write(
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
        }
        assert_eq!(
            port.call_count(),
            case_port_call_count(case),
            "{}",
            case.name
        );
    }
}

#[test]
fn strategy_definition_write_leaf_has_exact_route_inventory_and_isolates_reads() {
    assert_eq!(strategy_definition_write_routes().len(), 5);
    assert!(strategy_definition_write_routes().contains(&(
        "POST",
        "/api/v1/strategy-definitions/{definitionId}/apply-linked-instances"
    )));
    let read = ApiRequest {
        method: "GET".to_owned(),
        path: "/api/v1/strategy-definitions".to_owned(),
        query: String::new(),
        body: Vec::new(),
        request_id: "strategy-definition-write-read-isolation".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
        csrf_valid: false,
        session_cookie: None,
    };
    let response = dispatch_strategy_definition_write(&read, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 404);
    assert_eq!(response.body["error"]["code"], "NOT_FOUND");
}

#[test]
fn strategy_definition_write_leaf_fails_closed_without_test_port() {
    let request = ApiRequest {
        method: "POST".to_owned(),
        path: "/api/v1/strategy-definitions".to_owned(),
        query: String::new(),
        body: br#"{"name":"Draft"}"#.to_vec(),
        request_id: "strategy-definition-write-unavailable".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
        csrf_valid: false,
        session_cookie: None,
    };
    let response = dispatch_strategy_definition_write(&request, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(
        response.body["error"]["code"],
        "STRATEGY_DEFINITIONS_UNAVAILABLE"
    );
}

fn case_port_call_count(case: &FixtureCase) -> usize {
    (0..case.request_paths.len())
        .filter(|index| case_port_call(case, *index))
        .count()
}
