#[path = "../src/product_adk_chat_stream_port.rs"]
mod product_adk_chat_stream_port;

use std::collections::BTreeMap;

use product_adk_chat_stream_port::{
    AdkChatPortError, AdkChatPortOutput, AdkChatRequest, AdkChatRoute, AdkChatStreamFrame,
    AdkChatStreamPort, AdkChatStreamSnapshot, dispatch_adk_chat,
};
use serde::Deserialize;
use serde_json::Value;

const FIXTURE_TIMESTAMP: &str = "fixture-time";
const STREAM_IDLE_TIMEOUT_MS: u64 = 420_000;

#[derive(Debug, Deserialize)]
struct Fixture {
    version: String,
    cases: Vec<FixtureCase>,
}

#[derive(Debug, Deserialize)]
struct FixtureCase {
    name: String,
    method: String,
    #[serde(rename = "requestPath")]
    request_path: String,
    body: Option<String>,
    #[serde(rename = "portMode")]
    port_mode: String,
    expected: Expected,
}

#[derive(Debug, Deserialize)]
struct Expected {
    status: u16,
    headers: BTreeMap<String, String>,
    body: Value,
    #[serde(default)]
    frames: Vec<FixtureFrame>,
}

#[derive(Debug, Deserialize)]
struct FixtureFrame {
    kind: String,
    #[serde(default)]
    id: String,
    #[serde(default)]
    milliseconds: u64,
    #[serde(default)]
    comment: String,
    data: Option<Value>,
}

#[derive(Debug)]
struct FixturePort {
    output: Result<AdkChatPortOutput, AdkChatPortError>,
}

impl AdkChatStreamPort for FixturePort {
    fn dispatch(
        &self,
        _route: AdkChatRoute,
        _input: &product_adk_chat_stream_port::AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        match &self.output {
            Ok(output) => Ok(output.clone()),
            Err(error) => Err(error.clone()),
        }
    }
}

fn fixture() -> Fixture {
    let fixture: Fixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/adk-chat-stream.json"
    ))
    .expect("ADK chat-stream fixture");
    assert_eq!(fixture.version, "stage9.adk-chat-stream.v1");
    fixture
}

#[test]
fn adk_chat_stream_replays_go_wire_fixture_for_leaf_owned_cases() {
    let fixture = fixture();
    assert_eq!(fixture.cases.len(), 17);
    let mut middleware_cases = 0;

    for case in &fixture.cases {
        if matches!(
            case.name.as_str(),
            "chat-auth-required" | "chat-csrf-forbidden"
        ) {
            middleware_cases += 1;
            continue;
        }

        let request = AdkChatRequest {
            method: case.method.clone(),
            path: case.request_path.clone(),
            body: case.body.as_deref().unwrap_or_default().as_bytes().to_vec(),
        };
        let port = fixture_port(case);
        let response = dispatch_adk_chat(
            &request,
            port.as_ref().map(|port| port as &dyn AdkChatStreamPort),
            FIXTURE_TIMESTAMP,
            STREAM_IDLE_TIMEOUT_MS,
        );

        assert_eq!(
            response.status(),
            case.expected.status,
            "case {}",
            case.name
        );
        assert_eq!(
            response.headers(),
            &case.expected.headers,
            "case {}",
            case.name
        );

        if case.name == "stream-client-disconnect" {
            assert!(matches!(
                response,
                product_adk_chat_stream_port::AdkChatWireResponse::Sse { terminal: true, .. }
            ));
            continue;
        }

        if case.expected.body.is_string() {
            assert_eq!(
                response.body(),
                case.expected.body.as_str().unwrap(),
                "case {}",
                case.name
            );
            assert_sse_frames(&response, &case.expected, &case.name);
        } else {
            let expected_body = serde_json::to_string(&case.expected.body).expect("JSON fixture");
            assert_eq!(response.body(), expected_body, "case {}", case.name);
        }
    }

    assert_eq!(middleware_cases, 2);
}

#[test]
fn adk_chat_stream_rejects_unknown_routes_and_keeps_port_optional() {
    let fixture = fixture();
    let method_not_found = fixture
        .cases
        .iter()
        .find(|case| case.name == "chat-method-not-found")
        .expect("method-not-found fixture case");
    let request = AdkChatRequest {
        method: method_not_found.method.clone(),
        path: method_not_found.request_path.clone(),
        body: Vec::new(),
    };
    let response = dispatch_adk_chat(&request, None, FIXTURE_TIMESTAMP, STREAM_IDLE_TIMEOUT_MS);
    assert_eq!(response.status(), 404);
    assert_eq!(response.headers(), &method_not_found.expected.headers);
    let actual: Value = serde_json::from_str(&response.body()).expect("JSON not-found response");
    assert_eq!(actual, method_not_found.expected.body);

    let runtime_unavailable = fixture
        .cases
        .iter()
        .find(|case| case.name == "stream-runtime-unavailable")
        .expect("runtime-unavailable fixture case");
    let request = AdkChatRequest {
        method: runtime_unavailable.method.clone(),
        path: runtime_unavailable.request_path.clone(),
        body: runtime_unavailable
            .body
            .as_deref()
            .unwrap_or_default()
            .as_bytes()
            .to_vec(),
    };
    let response = dispatch_adk_chat(&request, None, FIXTURE_TIMESTAMP, STREAM_IDLE_TIMEOUT_MS);
    assert_eq!(response.status(), runtime_unavailable.expected.status);
    assert_eq!(response.headers(), &runtime_unavailable.expected.headers);
}

#[test]
fn adk_chat_stream_maps_snapshot_errors_without_runtime_side_effects() {
    let request = AdkChatRequest {
        method: "POST".to_owned(),
        path: "/api/v1/adk/chat".to_owned(),
        body: br#"{"clientRequestId":"11111111-1111-4111-8111-111111111111"}"#.to_vec(),
    };
    for error in [
        AdkChatPortError::Unavailable("fixture runtime unavailable".to_owned()),
        AdkChatPortError::Conflict("fixture conflict".to_owned()),
        AdkChatPortError::Failed {
            status: 502,
            code: "FIXTURE_FAILURE".to_owned(),
            message: "fixture failed".to_owned(),
        },
    ] {
        let port = FixturePort { output: Err(error) };
        let response = dispatch_adk_chat(
            &request,
            Some(&port),
            FIXTURE_TIMESTAMP,
            STREAM_IDLE_TIMEOUT_MS,
        );
        assert_eq!(
            response.headers()["Content-Type"],
            "application/json; charset=utf-8"
        );
        let body: Value = serde_json::from_str(&response.body()).expect("JSON error response");
        assert_eq!(body["ok"], false);
        assert!(body["error"]["code"].is_string());
    }
}

fn fixture_port(case: &FixtureCase) -> Option<FixturePort> {
    if matches!(
        case.name.as_str(),
        "chat-invalid-json" | "stream-invalid-json" | "stream-invalid-client-request-id"
    ) || case.port_mode == "runtime-unavailable"
    {
        return None;
    }
    if case.port_mode == "idempotency-conflict" {
        return Some(FixturePort {
            output: Err(AdkChatPortError::Conflict(
                case.expected.body["error"]["message"]
                    .as_str()
                    .expect("conflict message")
                    .to_owned(),
            )),
        });
    }
    if case.name == "chat-method-not-found" {
        return None;
    }
    if case.name == "stream-client-disconnect" {
        return Some(FixturePort {
            output: Ok(AdkChatPortOutput::Stream(AdkChatStreamSnapshot {
                headers: case.expected.headers.clone(),
                frames: Vec::new(),
                terminal: true,
            })),
        });
    }
    if case.request_path.ends_with("/stream") {
        let frames = case
            .expected
            .frames
            .iter()
            .skip_while(|frame| frame.kind == "retry")
            .map(fixture_frame)
            .collect();
        return Some(FixturePort {
            output: Ok(AdkChatPortOutput::Stream(AdkChatStreamSnapshot {
                headers: case.expected.headers.clone(),
                frames,
                terminal: true,
            })),
        });
    }
    if let Some(data) = case.expected.body.get("data") {
        return Some(FixturePort {
            output: Ok(AdkChatPortOutput::Json(data.clone())),
        });
    }
    let error = &case.expected.body["error"];
    Some(FixturePort {
        output: Err(AdkChatPortError::Failed {
            status: case.expected.status,
            code: error["code"].as_str().expect("error code").to_owned(),
            message: error["message"].as_str().expect("error message").to_owned(),
        }),
    })
}

fn fixture_frame(frame: &FixtureFrame) -> AdkChatStreamFrame {
    match frame.kind.as_str() {
        "retry" => AdkChatStreamFrame::Comment(format!("retry: {}", frame.milliseconds)),
        "comment" => AdkChatStreamFrame::Comment(frame.comment.clone()),
        "event" => AdkChatStreamFrame::Event {
            id: (!frame.id.is_empty()).then(|| frame.id.clone()),
            data: frame.data.clone().expect("event data"),
        },
        other => panic!("unknown fixture SSE frame kind {other}"),
    }
}

fn assert_sse_frames(
    response: &product_adk_chat_stream_port::AdkChatWireResponse,
    expected: &Expected,
    case_name: &str,
) {
    let product_adk_chat_stream_port::AdkChatWireResponse::Sse {
        frames, terminal, ..
    } = response
    else {
        panic!("case {case_name} did not return SSE response");
    };
    assert!(*terminal, "case {case_name} did not terminate");
    assert_eq!(
        frames.len(),
        expected.frames.len(),
        "case {case_name} frame count"
    );
    for (actual, expected) in frames.iter().zip(&expected.frames) {
        assert_eq!(actual, &fixture_frame(expected), "case {case_name} frame");
    }
}
