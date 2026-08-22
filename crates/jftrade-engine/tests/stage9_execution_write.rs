#[path = "../src/product_execution_write_port.rs"]
mod product_execution_write_port;

use std::collections::{BTreeMap, VecDeque};
use std::sync::Mutex;

use product_execution_write_port::{
    EXECUTION_WRITE_ROUTES, ExecutionWriteContext, ExecutionWriteInput, ExecutionWriteOperation,
    ExecutionWritePort, ExecutionWritePortError, ExecutionWriteRequest, dispatch_execution_write,
    execution_write_routes,
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
    port_mode: String,
    requests: Vec<FixtureRequest>,
    expected: Vec<FixtureExpected>,
    #[serde(default)]
    go_calls: Vec<Value>,
    observation: FixtureObservation,
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
struct FixtureObservation {
    buying_power_calls: usize,
    combo_preview_calls: usize,
    combo_place_calls: usize,
    combo_cancel_calls: usize,
    order_preview_calls: usize,
    order_place_calls: usize,
    order_cancel_calls: usize,
}

#[derive(Debug)]
struct FixturePort {
    responses: Mutex<VecDeque<Result<Value, ExecutionWritePortError>>>,
    calls: Mutex<Vec<ExecutionWriteInput>>,
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
                .expect("execution write response lock")
                .is_empty(),
            "fixture responses remain for {case_name}"
        );
    }
}

impl ExecutionWritePort for FixturePort {
    fn mutate(&self, input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        self.calls
            .lock()
            .expect("execution write calls lock")
            .push(input.clone());
        let mut responses = self
            .responses
            .lock()
            .expect("execution write response lock");
        responses.pop_front().unwrap_or_else(|| {
            Err(ExecutionWritePortError::Failed {
                status: 500,
                code: "EXECUTION_WRITE_FAILED".to_owned(),
                message: "fixture response missing".to_owned(),
            })
        })
    }
}

#[test]
fn execution_write_fixture_replays_all_seven_go_owned_mutations() {
    let fixture = fixture();
    assert_eq!(fixture.version, "stage9.execution-write.v1");
    assert_eq!(fixture.cases.len(), 57);
    assert_eq!(execution_write_routes(), &EXECUTION_WRITE_ROUTES);

    for case in &fixture.cases {
        assert_eq!(
            case.requests.len(),
            case.expected.len(),
            "case {}",
            case.name
        );
        assert!(
            !case.port_mode.is_empty(),
            "case {} has no port mode",
            case.name
        );
        assert_eq!(
            case.go_calls.len(),
            observation_total(&case.observation),
            "case {}",
            case.name
        );
        let port = FixturePort::from_case(case);
        assert_eq!(
            port.responses.lock().expect("fixture response count").len(),
            case.expected
                .iter()
                .filter(|expected| expected.port_call)
                .count(),
            "case {} fixture response queue",
            case.name
        );
        for (request, expected) in case.requests.iter().zip(&case.expected) {
            let response =
                dispatch_execution_write(&to_request(request), Some(&port), FIXTURE_TIMESTAMP);
            let call_count = port.calls.lock().expect("execution call count").len();
            assert_eq!(
                response.status, expected.status,
                "case {} response {:?}, calls {}",
                case.name, response.body, call_count
            );
            assert_eq!(response.headers, expected.headers, "case {}", case.name);
            assert_eq!(response.body, expected.envelope, "case {}", case.name);
        }
        let calls = port.calls.lock().expect("execution write call list");
        assert_eq!(
            calls.len(),
            case.expected
                .iter()
                .filter(|expected| expected.port_call)
                .count(),
            "case {} port-call count",
            case.name
        );
        assert_input_shape(case, &calls);
        drop(calls);
        port.assert_drained(&case.name);
    }
}

#[test]
fn execution_write_fixture_covers_each_route_and_context_boundary() {
    let fixture = fixture();
    for (method, template) in EXECUTION_WRITE_ROUTES {
        assert!(
            fixture
                .cases
                .iter()
                .flat_map(|case| case.requests.iter())
                .any(|request| {
                    request.method == method
                        && route_template_matches(&request.request_path, template)
                }),
            "fixture does not cover {method} {template}"
        );
    }
    assert!(fixture.cases.iter().any(|case| {
        case.requests
            .iter()
            .any(|request| request.context == "canceled")
    }));
    assert!(fixture.cases.iter().any(|case| {
        case.requests
            .iter()
            .any(|request| request.body.as_deref() == Some("null"))
    }));
    assert!(fixture.cases.iter().any(|case| {
        case.requests.iter().any(|request| {
            request
                .body
                .as_deref()
                .is_some_and(|body| body.contains("{\"ignored\":true}"))
        })
    }));
}

#[test]
fn execution_write_leaf_fails_closed_without_test_port_after_shape_validation() {
    let valid = ExecutionWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/execution/orders".to_owned(),
        body: Some(
            br#"{"market":"US","symbol":"AAPL","side":"BUY","quantity":1,"price":100}"#.to_vec(),
        ),
        context: ExecutionWriteContext::Normal,
    };
    let response = dispatch_execution_write(&valid, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(
        response.body["error"]["code"],
        "EXECUTION_WRITE_UNAVAILABLE"
    );

    let malformed = ExecutionWriteRequest {
        body: Some(b"{".to_vec()),
        ..valid
    };
    let response = dispatch_execution_write(&malformed, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 400);
    assert_eq!(response.body["error"]["code"], "BAD_REQUEST");
}

#[test]
fn execution_write_leaf_preserves_null_trailing_json_and_percent_id_trim() {
    let port = RecordingPort::new();
    let trailing = ExecutionWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/execution/previews".to_owned(),
        body: Some(br#"{"market":"US","symbol":"AAPL","side":"BUY","quantity":1,"price":100}{"ignored":true}"#.to_vec()),
        context: ExecutionWriteContext::Normal,
    };
    let response = dispatch_execution_write(&trailing, Some(&port), FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 200);
    assert_eq!(response.body["data"], json!({"accepted": true}));
    assert_eq!(
        port.last_operation(),
        Some(ExecutionWriteOperation::OrderPreview)
    );

    let null = ExecutionWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/execution/combos/previews".to_owned(),
        body: Some(b"null".to_vec()),
        context: ExecutionWriteContext::Normal,
    };
    let response = dispatch_execution_write(&null, Some(&port), FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 200);
    assert_eq!(port.last_payload(), Some(Value::Null));

    let cancel = ExecutionWriteRequest {
        method: "POST".to_owned(),
        path: "/api/v1/execution/orders/%20order-1%20/cancel".to_owned(),
        body: None,
        context: ExecutionWriteContext::Canceled,
    };
    let response = dispatch_execution_write(&cancel, Some(&port), FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 200);
    assert_eq!(port.last_internal_order_id(), Some("order-1".to_owned()));
    assert_eq!(port.last_context(), Some(ExecutionWriteContext::Canceled));
}

#[derive(Debug)]
struct RecordingPort {
    inputs: Mutex<Vec<ExecutionWriteInput>>,
}

impl RecordingPort {
    const fn new() -> Self {
        Self {
            inputs: Mutex::new(Vec::new()),
        }
    }

    fn last(&self) -> ExecutionWriteInput {
        self.inputs
            .lock()
            .expect("recording execution input lock")
            .last()
            .cloned()
            .expect("recorded execution input")
    }

    fn last_operation(&self) -> Option<ExecutionWriteOperation> {
        Some(self.last().operation)
    }

    fn last_payload(&self) -> Option<Value> {
        Some(self.last().payload)
    }

    fn last_internal_order_id(&self) -> Option<String> {
        self.last().internal_order_id
    }

    fn last_context(&self) -> Option<ExecutionWriteContext> {
        Some(self.last().context)
    }
}

impl Default for RecordingPort {
    fn default() -> Self {
        Self::new()
    }
}

impl ExecutionWritePort for RecordingPort {
    fn mutate(&self, input: &ExecutionWriteInput) -> Result<Value, ExecutionWritePortError> {
        self.inputs
            .lock()
            .expect("recording execution input lock")
            .push(input.clone());
        Ok(json!({"accepted": true}))
    }
}

fn assert_input_shape(case: &FixtureCase, calls: &[ExecutionWriteInput]) {
    let mut index = 0;
    for (request, expected) in case.requests.iter().zip(&case.expected) {
        if !expected.port_call {
            continue;
        }
        let input = &calls[index];
        index += 1;
        assert_eq!(
            input.context,
            request_context(request),
            "case {}",
            case.name
        );
        assert!(
            input.operation.name().contains("order")
                || input.operation.name().contains("combo")
                || input.operation == ExecutionWriteOperation::BuyingPower,
            "case {} operation {:?}",
            case.name,
            input.operation
        );
        if input.internal_order_id.is_none() {
            assert!(
                input.payload.is_null() || input.payload.is_object(),
                "case {}",
                case.name
            );
        }
    }
}

fn observation_total(observation: &FixtureObservation) -> usize {
    observation.buying_power_calls
        + observation.combo_preview_calls
        + observation.combo_place_calls
        + observation.combo_cancel_calls
        + observation.order_preview_calls
        + observation.order_place_calls
        + observation.order_cancel_calls
}

fn request_context(request: &FixtureRequest) -> ExecutionWriteContext {
    match request.context.as_str() {
        "canceled" => ExecutionWriteContext::Canceled,
        "deadline" => ExecutionWriteContext::Deadline,
        _ => ExecutionWriteContext::Normal,
    }
}

fn to_request(request: &FixtureRequest) -> ExecutionWriteRequest {
    ExecutionWriteRequest {
        method: request.method.clone(),
        path: request.request_path.clone(),
        body: request.body.as_ref().map(|body| body.as_bytes().to_vec()),
        context: request_context(request),
    }
}

fn route_template_matches(path: &str, template: &str) -> bool {
    let path = path.split('?').next().unwrap_or(path);
    let actual = path.split('/').collect::<Vec<_>>();
    let expected = template.split('/').collect::<Vec<_>>();
    actual.len() == expected.len()
        && actual
            .iter()
            .zip(expected)
            .all(|(actual, expected)| expected.starts_with('{') || actual == &expected)
}

fn fixture() -> Fixture {
    serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/execution-write.json"
    ))
    .expect("execution-write fixture")
}

fn error_from_envelope(expected: &FixtureExpected) -> ExecutionWritePortError {
    ExecutionWritePortError::Failed {
        status: expected.status,
        code: expected.envelope["error"]["code"]
            .as_str()
            .unwrap_or("EXECUTION_WRITE_FAILED")
            .to_owned(),
        message: expected.envelope["error"]["message"]
            .as_str()
            .unwrap_or_default()
            .to_owned(),
    }
}
