#[path = "../src/strategy_pine.rs"]
mod strategy_pine;

use std::collections::{BTreeMap, HashMap};
use std::sync::{Arc, Mutex};

use serde::Deserialize;
use serde_json::{Value, json};

use strategy_pine::{
    JSON_CONTENT_TYPE, PINE_V6_SOURCE_FORMAT, StrategyPineAnalyzeInput,
    StrategyPineAnalyzeSnapshotError, StrategyPineAnalyzeSnapshotPort,
    dispatch_strategy_pine_analyze,
};

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct StrategyPineFixture {
    version: String,
    cases: Vec<StrategyPineFixtureCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct StrategyPineFixtureCase {
    name: String,
    method: String,
    body: String,
    expected_status: u16,
    headers: BTreeMap<String, String>,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug, Default)]
struct FixtureStrategyPinePort {
    projections: HashMap<String, Value>,
    calls: Mutex<Vec<StrategyPineAnalyzeInput>>,
}

impl FixtureStrategyPinePort {
    fn from_fixture(fixture: &StrategyPineFixture) -> Self {
        let projections = fixture
            .cases
            .iter()
            .filter_map(|case| {
                let data = case.data.clone()?;
                let body: Value = serde_json::from_str(&case.body).ok()?;
                let script = body
                    .get("script")
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_owned();
                Some((script, data))
            })
            .collect();
        Self {
            projections,
            calls: Mutex::new(Vec::new()),
        }
    }
}

impl StrategyPineAnalyzeSnapshotPort for FixtureStrategyPinePort {
    fn analyze(
        &self,
        input: &StrategyPineAnalyzeInput,
    ) -> Result<Value, StrategyPineAnalyzeSnapshotError> {
        self.calls
            .lock()
            .expect("strategy-pine fixture call lock")
            .push(input.clone());
        self.projections.get(&input.script).cloned().ok_or_else(|| {
            StrategyPineAnalyzeSnapshotError::Unavailable("fixture projection missing".to_owned())
        })
    }
}

#[derive(Debug)]
struct RecordingPort {
    response: Result<Value, StrategyPineAnalyzeSnapshotError>,
    calls: Mutex<Vec<StrategyPineAnalyzeInput>>,
}

impl RecordingPort {
    fn new(response: Result<Value, StrategyPineAnalyzeSnapshotError>) -> Self {
        Self {
            response,
            calls: Mutex::new(Vec::new()),
        }
    }
}

impl StrategyPineAnalyzeSnapshotPort for RecordingPort {
    fn analyze(
        &self,
        input: &StrategyPineAnalyzeInput,
    ) -> Result<Value, StrategyPineAnalyzeSnapshotError> {
        self.calls
            .lock()
            .expect("strategy-pine recording call lock")
            .push(input.clone());
        self.response.clone()
    }
}

fn fixture() -> StrategyPineFixture {
    let fixture: StrategyPineFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/strategy-pine.json"
    ))
    .expect("strategy-pine fixture");
    assert_eq!(fixture.version, "stage9.strategy-pine.v1");
    fixture
}

#[test]
fn strategy_pine_replays_go_fixture_projection_status_and_headers() {
    let fixture = fixture();
    let port = FixtureStrategyPinePort::from_fixture(&fixture);
    for case in &fixture.cases {
        let response = dispatch_strategy_pine_analyze(
            Some(&port),
            &case.method,
            "/api/v1/strategy-pine/analyze",
            case.body.as_bytes(),
        );
        assert_eq!(response.status, case.expected_status, "case {}", case.name);
        assert_eq!(response.headers, case.headers, "case {} headers", case.name);
        match (&response.data, &case.data, &response.error) {
            (Some(actual), Some(expected), None) => {
                assert_eq!(actual, expected, "case {} data", case.name);
            }
            (None, None, Some(error)) => {
                assert_eq!(error.code, case.error_code.clone().unwrap_or_default());
                assert_eq!(
                    error.message,
                    case.error_message.clone().unwrap_or_default()
                );
                assert_eq!(error.retry_after_seconds, None);
            }
            _ => panic!("case {} returned the wrong envelope shape", case.name),
        }
    }

    let calls = port.calls.lock().expect("strategy-pine fixture call lock");
    assert!(
        calls
            .iter()
            .any(|input| { input.include_ast && input.source_format == PINE_V6_SOURCE_FORMAT })
    );
    assert!(calls.iter().any(|input| input.script.is_empty()));
}

#[test]
fn strategy_pine_preserves_worker_shadow_errors_as_successful_projections() {
    let fixture = fixture();
    let port = FixtureStrategyPinePort::from_fixture(&fixture);
    for name in [
        "worker-unavailable-projection",
        "worker-timeout-projection",
        "worker-cancel-projection",
        "worker-crash-projection",
    ] {
        let case = fixture
            .cases
            .iter()
            .find(|case| case.name == name)
            .expect("worker fixture case");
        let response = dispatch_strategy_pine_analyze(
            Some(&port),
            "POST",
            "/api/v1/strategy-pine/analyze",
            case.body.as_bytes(),
        );
        assert_eq!(response.status, 200, "case {name}");
        assert_eq!(response.error, None, "case {name}");
        assert_eq!(
            response
                .data
                .as_ref()
                .and_then(|data| data["externalEngine"]["status"].as_str()),
            Some("shadow_error"),
            "case {name}"
        );
    }
}

#[test]
fn strategy_pine_applies_input_validation_and_error_precedence_before_the_port() {
    let port = Arc::new(RecordingPort::new(Err(
        StrategyPineAnalyzeSnapshotError::Unavailable("owner unavailable".to_owned()),
    )));
    let cases = [
        (b"{".as_slice(), 400, "BAD_REQUEST"),
        (
            br#"{"script":123,"sourceFormat":"legacy"}"#.as_slice(),
            400,
            "BAD_REQUEST",
        ),
        (
            br#"{"sourceFormat":"legacy","script":"strategy(\"x\")"}"#.as_slice(),
            400,
            "BAD_REQUEST",
        ),
    ];
    for (body, status, code) in cases {
        let response = dispatch_strategy_pine_analyze(
            Some(port.as_ref()),
            "POST",
            "/api/v1/strategy-pine/analyze",
            body,
        );
        assert_eq!(response.status, status);
        assert_eq!(
            response.error.as_ref().map(|error| error.code.as_str()),
            Some(code)
        );
    }
    assert!(port.calls.lock().expect("recording call lock").is_empty());

    let valid = dispatch_strategy_pine_analyze(
        Some(port.as_ref()),
        "POST",
        "/api/v1/strategy-pine/analyze",
        br#"{"script":"","sourceFormat":" PINE-V6 ","includeAst":false}"#,
    );
    assert_eq!(valid.status, 503);
    assert_eq!(
        valid.error.as_ref().map(|error| error.code.as_str()),
        Some("STRATEGY_PINE_ANALYZE_UNAVAILABLE")
    );
    let calls = port.calls.lock().expect("recording call lock");
    assert_eq!(calls.len(), 1);
    assert_eq!(calls[0].source_format, PINE_V6_SOURCE_FORMAT);
}

#[test]
fn strategy_pine_accepts_go_zero_values_and_opaque_empty_or_abnormal_projections() {
    let null_port = RecordingPort::new(Ok(Value::Null));
    let null_response = dispatch_strategy_pine_analyze(
        Some(&null_port),
        "POST",
        "/api/v1/strategy-pine/analyze",
        b"null",
    );
    assert_eq!(null_response.status, 200);
    assert_eq!(null_response.data, Some(Value::Null));
    assert_eq!(
        null_port.calls.lock().expect("null call lock")[0],
        StrategyPineAnalyzeInput {
            script: String::new(),
            source_format: PINE_V6_SOURCE_FORMAT.to_owned(),
            include_ast: false,
        }
    );

    for projection in [json!({}), json!([]), json!("projection"), json!(null)] {
        let port = RecordingPort::new(Ok(projection.clone()));
        let response = dispatch_strategy_pine_analyze(
            Some(&port),
            "POST",
            "/api/v1/strategy-pine/analyze",
            br#"{"script":"strategy(\"x\")"}"#,
        );
        assert_eq!(response.status, 200);
        assert_eq!(response.data, Some(projection));
    }
}

#[test]
fn strategy_pine_preserves_snapshot_failures_and_wire_headers() {
    let unavailable = RecordingPort::new(Err(StrategyPineAnalyzeSnapshotError::Unavailable(
        "Go owner unavailable".to_owned(),
    )));
    let response = dispatch_strategy_pine_analyze(
        Some(&unavailable),
        "POST",
        "/api/v1/strategy-pine/analyze",
        br#"{"script":"strategy(\"x\")"}"#,
    );
    assert_eq!(response.status, 503);
    assert_eq!(response.headers["Content-Type"], JSON_CONTENT_TYPE);
    assert_eq!(
        response.error.as_ref().map(|error| error.code.as_str()),
        Some("STRATEGY_PINE_ANALYZE_UNAVAILABLE")
    );

    let failed = RecordingPort::new(Err(StrategyPineAnalyzeSnapshotError::Failed {
        status: 504,
        code: "STRATEGY_PINE_ANALYZE_TIMEOUT".to_owned(),
        message: "analysis snapshot timed out".to_owned(),
        retry_after_seconds: Some(3),
    }));
    let response = dispatch_strategy_pine_analyze(
        Some(&failed),
        "POST",
        "/api/v1/strategy-pine/analyze",
        br#"{"script":"strategy(\"x\")"}"#,
    );
    assert_eq!(response.status, 504);
    assert_eq!(response.headers["Content-Type"], JSON_CONTENT_TYPE);
    assert_eq!(response.headers["Retry-After"], "3");
    assert_eq!(
        response.error.as_ref().map(|error| error.message.as_str()),
        Some("analysis snapshot timed out")
    );
}

#[test]
fn strategy_pine_rejects_non_route_requests_before_body_parsing() {
    let port = RecordingPort::new(Ok(json!({"unexpected": true})));
    for (method, path) in [
        ("GET", "/api/v1/strategy-pine/analyze"),
        ("POST", "/api/v1/strategy-pine/unknown"),
    ] {
        let response = dispatch_strategy_pine_analyze(Some(&port), method, path, b"{");
        assert_eq!(response.status, 404, "{method} {path}");
        assert_eq!(
            response.error.as_ref().map(|error| error.code.as_str()),
            Some("NOT_FOUND")
        );
    }
    assert!(port.calls.lock().expect("route call lock").is_empty());
}
