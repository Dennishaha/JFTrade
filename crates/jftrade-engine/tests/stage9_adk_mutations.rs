#[path = "../src/product_adk_mutation_port.rs"]
mod product_adk_mutation_port;

use std::collections::{BTreeMap, VecDeque};
use std::sync::Mutex;

use product_adk_mutation_port::{
    ADK_MUTATION_ROUTES, AdkMutationInput, AdkMutationPort, AdkMutationPortError,
    AdkMutationRequest, adk_mutation_routes, dispatch_adk_mutation,
};
use serde::Deserialize;
use serde_json::{Value, json};

const FIXTURE_TIMESTAMP: &str = "2026-08-23T08:00:00Z";

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
    method: String,
    request_path: String,
    body: Option<String>,
    #[serde(default)]
    headers: BTreeMap<String, String>,
    expected: FixtureExpected,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureExpected {
    status: u16,
    headers: BTreeMap<String, String>,
    port_call: bool,
    envelope: Value,
}

#[derive(Debug)]
struct FixturePort {
    responses: Mutex<VecDeque<Result<Value, AdkMutationPortError>>>,
    calls: Mutex<Vec<AdkMutationInput>>,
}

impl FixturePort {
    fn from_case(case: &FixtureCase) -> Self {
        let responses = if case.expected.port_call {
            let envelope = &case.expected.envelope;
            let response = if envelope["ok"] == true {
                Ok(envelope["data"].clone())
            } else {
                Err(error_from_envelope(envelope, case.expected.status))
            };
            VecDeque::from([response])
        } else {
            VecDeque::new()
        };
        Self {
            responses: Mutex::new(responses),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn call_count(&self) -> usize {
        self.calls.lock().expect("ADK mutation calls lock").len()
    }

    fn response_count(&self) -> usize {
        self.responses
            .lock()
            .expect("ADK mutation responses lock")
            .len()
    }
}

impl AdkMutationPort for FixturePort {
    fn mutate(&self, input: &AdkMutationInput) -> Result<Value, AdkMutationPortError> {
        self.calls
            .lock()
            .expect("ADK mutation calls lock")
            .push(input.clone());
        self.responses
            .lock()
            .expect("ADK mutation responses lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(AdkMutationPortError::Unavailable(
                    "fixture response missing".to_owned(),
                ))
            })
    }
}

#[derive(Debug)]
struct RecordingPort;

impl AdkMutationPort for RecordingPort {
    fn mutate(&self, input: &AdkMutationInput) -> Result<Value, AdkMutationPortError> {
        Ok(json!({
            "operation": input.operation.name(),
            "identifiers": input.identifiers,
            "body": input.body,
            "webhookSecret": input.webhook_secret,
        }))
    }
}

#[test]
fn adk_mutation_fixture_replays_go_wire_for_all_remaining_operations() {
    let fixture = fixture();
    assert_eq!(fixture.version, "stage9.adk-mutations.v1");
    assert_eq!(fixture.timestamp, FIXTURE_TIMESTAMP);
    assert_eq!(fixture.cases.len(), 40);

    for case in &fixture.cases {
        let port = FixturePort::from_case(case);
        let response = dispatch_adk_mutation(&request(case), Some(&port), FIXTURE_TIMESTAMP);
        assert_eq!(response.status, case.expected.status, "case {}", case.name);
        assert_eq!(
            response.headers, case.expected.headers,
            "case {}",
            case.name
        );
        assert_eq!(response.body, case.expected.envelope, "case {}", case.name);
        assert_eq!(
            port.call_count(),
            usize::from(case.expected.port_call),
            "case {}",
            case.name
        );
        assert_eq!(
            port.response_count(),
            0,
            "case {} fixture responses",
            case.name
        );
    }
}

#[test]
fn adk_mutation_fixture_covers_exact_route_inventory_and_isolates_reads() {
    let fixture = fixture();
    assert_eq!(adk_mutation_routes(), &ADK_MUTATION_ROUTES);
    assert_eq!(adk_mutation_routes().len(), 37);

    let valid_cases = fixture
        .cases
        .iter()
        .filter(|case| case.expected.port_call)
        .collect::<Vec<_>>();
    assert_eq!(valid_cases.len(), 37);

    for (method, template) in adk_mutation_routes() {
        assert!(
            valid_cases.iter().any(|case| {
                case.method == *method && route_template_matches(&case.request_path, template)
            }),
            "fixture does not cover {method} {template}"
        );
    }

    let read_request = AdkMutationRequest {
        method: "GET".to_owned(),
        path: "/api/v1/adk/agents".to_owned(),
        body: None,
        headers: BTreeMap::new(),
    };
    let response = dispatch_adk_mutation(&read_request, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 404);
    assert_eq!(response.body["error"]["code"], "NOT_FOUND");
}

#[test]
fn adk_mutation_leaf_fails_closed_after_shape_validation() {
    let fixture = fixture();
    let valid_case = fixture
        .cases
        .iter()
        .find(|case| case.name == "agent-create")
        .expect("valid ADK mutation fixture case");
    let unavailable = dispatch_adk_mutation(&request(valid_case), None, FIXTURE_TIMESTAMP);
    assert_eq!(unavailable.status, 503);
    assert_eq!(
        unavailable.body["error"]["code"],
        "ADK_MUTATIONS_UNAVAILABLE"
    );

    for case in fixture.cases.iter().filter(|case| !case.expected.port_call) {
        let response = dispatch_adk_mutation(&request(case), None, FIXTURE_TIMESTAMP);
        assert_eq!(response.status, case.expected.status, "case {}", case.name);
        assert_eq!(
            response.headers, case.expected.headers,
            "case {}",
            case.name
        );
        assert_eq!(response.body, case.expected.envelope, "case {}", case.name);
    }
}

#[test]
fn adk_mutation_leaf_preserves_trailing_json_and_webhook_secret_precedence() {
    let malformed = dispatch_adk_mutation(
        &request_with_body("POST", "/api/v1/adk/agents", br#"{"#),
        None,
        "fixture-time",
    );
    assert_eq!(malformed.status, 400);
    assert_eq!(malformed.body["error"]["message"], "invalid agent payload");

    let trailing = dispatch_adk_mutation(
        &request_with_body(
            "POST",
            "/api/v1/adk/agents",
            br#"{"id":"a"} {"ignored":true}"#,
        ),
        Some(&RecordingPort),
        "fixture-time",
    );
    assert_eq!(trailing.status, 200);
    assert_eq!(trailing.body["data"]["body"]["id"], "a");

    let mut webhook = request_with_body(
        "POST",
        "/api/v1/adk/workflow-webhooks/trigger",
        br#"{"inputs":{"source":"fixture"}} {"ignored":true}"#,
    );
    webhook.headers.insert(
        "Authorization".to_owned(),
        "Bearer bearer-secret".to_owned(),
    );
    let response = dispatch_adk_mutation(&webhook, Some(&RecordingPort), "fixture-time");
    assert_eq!(response.status, 200);
    assert_eq!(response.body["data"]["body"]["source"], "fixture");
    assert_eq!(response.body["data"]["webhookSecret"], "bearer-secret");

    webhook.headers.clear();
    webhook.headers.insert(
        "X-JFTrade-Workflow-Secret".to_owned(),
        "legacy-secret".to_owned(),
    );
    let response = dispatch_adk_mutation(&webhook, Some(&RecordingPort), "fixture-time");
    assert_eq!(response.status, 200);
    assert_eq!(response.body["data"]["webhookSecret"], "legacy-secret");
}

fn fixture() -> Fixture {
    let fixture: Fixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/adk-mutations.json"
    ))
    .expect("ADK mutation fixture");
    fixture
}

fn request(case: &FixtureCase) -> AdkMutationRequest {
    AdkMutationRequest {
        method: case.method.clone(),
        path: case.request_path.clone(),
        body: case.body.as_ref().map(|body| body.as_bytes().to_vec()),
        headers: case.headers.clone(),
    }
}

fn request_with_body(method: &str, path: &str, body: &[u8]) -> AdkMutationRequest {
    AdkMutationRequest {
        method: method.to_owned(),
        path: path.to_owned(),
        body: Some(body.to_vec()),
        headers: BTreeMap::new(),
    }
}

fn error_from_envelope(envelope: &Value, status: u16) -> AdkMutationPortError {
    AdkMutationPortError::Failed {
        status,
        code: envelope["error"]["code"]
            .as_str()
            .unwrap_or("ADK_MUTATION_FAILED")
            .to_owned(),
        message: envelope["error"]["message"]
            .as_str()
            .unwrap_or_default()
            .to_owned(),
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
