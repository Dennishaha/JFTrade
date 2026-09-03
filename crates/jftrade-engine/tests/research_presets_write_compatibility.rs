#[path = "../src/product_research_preset_write_port.rs"]
mod product_research_preset_write_port;

use std::collections::{BTreeMap, VecDeque};
use std::sync::{Arc, Mutex};
use std::thread;

use product_research_preset_write_port::{
    CREATE_PRESET_PATH, ResearchPresetWriteMutation, ResearchPresetWritePort,
    ResearchPresetWritePortError, ResearchPresetWriteRequest, dispatch_research_preset_write,
    research_preset_write_routes,
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
    expected: Vec<FixtureExpected>,
    expected_observation: Value,
    #[serde(default)]
    concurrent: bool,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureRequest {
    method: String,
    path: String,
    body: Option<String>,
    #[serde(default)]
    context: String,
    port_call: bool,
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
    responses: Mutex<VecDeque<Result<Value, ResearchPresetWritePortError>>>,
    calls: Mutex<Vec<ResearchPresetWriteMutation>>,
}

impl FixturePort {
    fn from_case(case: &FixtureCase) -> Self {
        let mut responses = VecDeque::new();
        for expected in &case.expected {
            if !expected.port_call {
                continue;
            }
            if expected.envelope["ok"] == true {
                responses.push_back(Ok(expected.envelope["data"].clone()));
            } else {
                responses.push_back(Err(error_from_envelope(&expected.envelope)));
            }
        }
        Self {
            responses: Mutex::new(responses),
            calls: Mutex::new(Vec::new()),
        }
    }

    fn call_count(&self) -> usize {
        self.calls.lock().expect("fixture port calls lock").len()
    }

    fn assert_drained(&self, case_name: &str) {
        assert!(
            self.responses
                .lock()
                .expect("fixture port responses lock")
                .is_empty(),
            "fixture port responses remain for {case_name}"
        );
    }
}

impl ResearchPresetWritePort for FixturePort {
    fn mutate(
        &self,
        mutation: &ResearchPresetWriteMutation,
    ) -> Result<Value, ResearchPresetWritePortError> {
        self.calls
            .lock()
            .expect("fixture port calls lock")
            .push(mutation.clone());
        self.responses
            .lock()
            .expect("fixture port responses lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(ResearchPresetWritePortError::Failed(
                    "fixture response missing".to_owned(),
                ))
            })
    }
}

#[test]
fn research_preset_write_fixture_matches_go_owner_for_all_three_routes() {
    let fixture = research_preset_write_fixture();
    assert_eq!(fixture.timestamp, FIXTURE_TIMESTAMP);
    assert_eq!(fixture.cases.len(), 22);
    for case in &fixture.cases {
        assert!(
            !case.concurrent || case.requests.len() > 1,
            "case {}",
            case.name
        );
        assert_eq!(
            case.requests.len(),
            case.expected.len(),
            "case {}",
            case.name
        );
        let port = FixturePort::from_case(case);
        for (request, expected) in case.requests.iter().zip(&case.expected) {
            let _ = &request.context;
            assert_eq!(request.port_call, expected.port_call, "case {}", case.name);
            let response = dispatch_research_preset_write(
                &ResearchPresetWriteRequest {
                    method: request.method.clone(),
                    path: request.path.clone(),
                    body: request.body.as_ref().map(|body| body.as_bytes().to_vec()),
                },
                Some(&port),
                FIXTURE_TIMESTAMP,
            );
            assert_eq!(response.status, expected.status, "case {}", case.name);
            assert_eq!(response.headers, expected.headers, "case {}", case.name);
            assert_eq!(response.body, expected.envelope, "case {}", case.name);
        }
        assert_eq!(
            port.call_count(),
            case.expected.iter().filter(|item| item.port_call).count(),
            "case {}",
            case.name
        );
        port.assert_drained(&case.name);
        assert_fixture_observation_is_present(case);
    }
}

#[test]
fn research_preset_write_leaf_fails_closed_without_a_test_port() {
    let valid_body = br#"{"name":"Value","definition":{"brokerId":"futu","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2,"columns":[]}}"#;
    let valid = ResearchPresetWriteRequest {
        method: "POST".to_owned(),
        path: CREATE_PRESET_PATH.to_owned(),
        body: Some(valid_body.to_vec()),
    };
    let response = dispatch_research_preset_write(&valid, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
    assert_eq!(
        response.body["error"]["code"],
        "RESEARCH_PRESET_UNAVAILABLE"
    );

    let invalid = ResearchPresetWriteRequest {
        body: Some(br#"{"name":"Value","unknown":true}"#.to_vec()),
        ..valid
    };
    let response = dispatch_research_preset_write(&invalid, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 400);
    assert_eq!(response.body["error"]["code"], "RESEARCH_PRESET_INVALID");

    let delete = ResearchPresetWriteRequest {
        method: "DELETE".to_owned(),
        path: "/api/v1/research/screens/presets/value".to_owned(),
        body: None,
    };
    let response = dispatch_research_preset_write(&delete, None, FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 503);
}

#[test]
fn research_preset_write_leaf_has_exact_three_route_inventory_and_isolated_paths() {
    assert_eq!(research_preset_write_routes().len(), 3);
    assert!(research_preset_write_routes().contains(&("POST", CREATE_PRESET_PATH)));
    assert!(
        research_preset_write_routes()
            .contains(&("PATCH", "/api/v1/research/screens/presets/{presetId}"))
    );
    assert!(
        research_preset_write_routes()
            .contains(&("DELETE", "/api/v1/research/screens/presets/{presetId}"))
    );
    let wrong_method = ResearchPresetWriteRequest {
        method: "GET".to_owned(),
        path: CREATE_PRESET_PATH.to_owned(),
        body: None,
    };
    assert_eq!(
        dispatch_research_preset_write(&wrong_method, None, FIXTURE_TIMESTAMP).status,
        404
    );
    let extra_segment = ResearchPresetWriteRequest {
        method: "DELETE".to_owned(),
        path: "/api/v1/research/screens/presets/value/extra".to_owned(),
        body: None,
    };
    assert_eq!(
        dispatch_research_preset_write(&extra_segment, None, FIXTURE_TIMESTAMP).status,
        404
    );
}

#[test]
fn research_preset_write_leaf_replays_concurrent_fencing_and_recovery() {
    let port = Arc::new(SequencePort::new([
        Err(ResearchPresetWritePortError::Conflict(
            "research screen preset conflict".to_owned(),
        )),
        Ok(json!({"presetId": "rsp-recovered", "revision": 2})),
    ]));
    let request = ResearchPresetWriteRequest {
        method: "PATCH".to_owned(),
        path: "/api/v1/research/screens/presets/rsp-recovered".to_owned(),
        body: Some(br#"{"name":"Retry","expectedRevision":1}"#.to_vec()),
    };
    let first = dispatch_research_preset_write(&request, Some(port.as_ref()), FIXTURE_TIMESTAMP);
    let second = dispatch_research_preset_write(&request, Some(port.as_ref()), FIXTURE_TIMESTAMP);
    assert_eq!(first.status, 409);
    assert_eq!(second.status, 200);
    assert_eq!(second.body["data"]["revision"], 2);

    let concurrent_port = Arc::new(ConcurrentFencePort::default());
    let mut statuses = Vec::new();
    thread::scope(|scope| {
        let mut handles = Vec::new();
        for _ in 0..8 {
            let port = Arc::clone(&concurrent_port);
            let request = request.clone();
            handles.push(scope.spawn(move || {
                dispatch_research_preset_write(&request, Some(port.as_ref()), FIXTURE_TIMESTAMP)
                    .status
            }));
        }
        for handle in handles {
            statuses.push(handle.join().expect("concurrent write thread"));
        }
    });
    statuses.sort_unstable();
    assert_eq!(statuses, vec![200, 409, 409, 409, 409, 409, 409, 409]);
    assert_eq!(concurrent_port.calls(), 8);
}

fn research_preset_write_fixture() -> Fixture {
    let fixture: Fixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/research-presets-write.json"
    ))
    .expect("research presets write fixture");
    assert_eq!(fixture.version, "stage9.research-presets-write.v1");
    fixture
}

fn error_from_envelope(envelope: &Value) -> ResearchPresetWritePortError {
    let code = envelope["error"]["code"]
        .as_str()
        .expect("fixture error code");
    let message = envelope["error"]["message"]
        .as_str()
        .expect("fixture error message")
        .to_owned();
    match code {
        "RESEARCH_PRESET_UNAVAILABLE" => ResearchPresetWritePortError::Unavailable,
        "RESEARCH_PRESET_NOT_FOUND" => ResearchPresetWritePortError::NotFound(message),
        "RESEARCH_PRESET_CONFLICT" => ResearchPresetWritePortError::Conflict(message),
        "RESEARCH_PRESET_INVALID" => ResearchPresetWritePortError::Invalid(message),
        "RESEARCH_PRESET_FAILED" => ResearchPresetWritePortError::Failed(message),
        other => panic!("unexpected research preset fixture error {other}"),
    }
}

fn assert_fixture_observation_is_present(case: &FixtureCase) {
    assert!(case.expected_observation.is_object(), "case {}", case.name);
    for key in [
        "presets",
        "createCalls",
        "getCalls",
        "updateCalls",
        "deleteCalls",
    ] {
        assert!(
            case.expected_observation.get(key).is_some(),
            "case {} missing {key}",
            case.name
        );
    }
}

#[derive(Debug)]
struct SequencePort {
    responses: Mutex<VecDeque<Result<Value, ResearchPresetWritePortError>>>,
}

impl SequencePort {
    fn new<const N: usize>(responses: [Result<Value, ResearchPresetWritePortError>; N]) -> Self {
        Self {
            responses: Mutex::new(VecDeque::from(responses)),
        }
    }
}

impl ResearchPresetWritePort for SequencePort {
    fn mutate(
        &self,
        _mutation: &ResearchPresetWriteMutation,
    ) -> Result<Value, ResearchPresetWritePortError> {
        self.responses
            .lock()
            .expect("sequence port lock")
            .pop_front()
            .expect("sequence port response")
    }
}

#[derive(Debug, Default)]
struct ConcurrentFencePort {
    calls: Mutex<usize>,
}

impl ConcurrentFencePort {
    fn calls(&self) -> usize {
        *self.calls.lock().expect("concurrent port lock")
    }
}

impl ResearchPresetWritePort for ConcurrentFencePort {
    fn mutate(
        &self,
        _mutation: &ResearchPresetWriteMutation,
    ) -> Result<Value, ResearchPresetWritePortError> {
        let mut calls = self.calls.lock().expect("concurrent port lock");
        *calls += 1;
        if *calls == 1 {
            Ok(json!({"presetId": "rsp-concurrent", "revision": 2}))
        } else {
            Err(ResearchPresetWritePortError::Conflict(
                "research screen preset conflict".to_owned(),
            ))
        }
    }
}
