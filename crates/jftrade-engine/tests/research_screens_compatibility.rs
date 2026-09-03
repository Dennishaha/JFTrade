#[path = "../src/product_research_screen_write_port.rs"]
mod product_research_screen_write_port;

use std::collections::{BTreeMap, VecDeque};
use std::sync::Mutex;

use product_research_screen_write_port::{
    RESEARCH_SCREEN_PATH, ResearchScreenWritePort, ResearchScreenWritePortError,
    ResearchScreenWriteQuery, ResearchScreenWriteRequest, dispatch_research_screen_write,
    research_screen_write_routes,
};
use serde::Deserialize;
use serde_json::{Value, json};

const FIXTURE_TIMESTAMP: &str = "2026-08-23T12:00:00Z";

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
    expected_observation: FixtureObservation,
    #[serde(default)]
    concurrent: bool,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureRequest {
    method: String,
    path: String,
    body: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureExpected {
    status: u16,
    headers: BTreeMap<String, String>,
    envelope: Value,
    port_call: bool,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct FixtureObservation {
    call_count: usize,
    #[serde(default)]
    calls: Vec<FixtureCall>,
}

#[derive(Debug, Deserialize, Eq, PartialEq)]
#[serde(rename_all = "camelCase")]
struct FixtureCall {
    broker_id: String,
    market: String,
    cursor: String,
    page_size: i64,
    operation: String,
    page_from: i64,
}

#[derive(Debug)]
struct FixturePort {
    responses: Mutex<VecDeque<Result<Value, ResearchScreenWritePortError>>>,
    calls: Mutex<Vec<ResearchScreenWriteQuery>>,
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

    fn calls(&self) -> Vec<ResearchScreenWriteQuery> {
        self.calls
            .lock()
            .expect("research screen calls lock")
            .clone()
    }
}

impl ResearchScreenWritePort for FixturePort {
    fn query(
        &self,
        request: &ResearchScreenWriteQuery,
    ) -> Result<Value, ResearchScreenWritePortError> {
        self.calls
            .lock()
            .expect("research screen calls lock")
            .push(request.clone());
        self.responses
            .lock()
            .expect("research screen responses lock")
            .pop_front()
            .unwrap_or_else(|| {
                Err(ResearchScreenWritePortError::Failed(
                    "fixture response missing".to_owned(),
                ))
            })
    }
}

#[test]
fn research_screen_fixture_replays_go_wire_for_the_post_route() {
    let fixture = fixture();
    assert_eq!(fixture.version, "stage9.research-screens.v1");
    assert_eq!(fixture.timestamp, FIXTURE_TIMESTAMP);
    assert_eq!(fixture.cases.len(), 22);
    assert_eq!(
        research_screen_write_routes(),
        &[("POST", RESEARCH_SCREEN_PATH)]
    );

    for case in &fixture.cases {
        assert_eq!(
            case.requests.len(),
            case.expected.len(),
            "case {}",
            case.name
        );
        assert_eq!(case.concurrent, case.name == "concurrent-distinct-pages");
        let port = FixturePort::from_case(case);
        for (index, (request, expected)) in case.requests.iter().zip(&case.expected).enumerate() {
            let response = dispatch_research_screen_write(
                &ResearchScreenWriteRequest {
                    method: request.method.clone(),
                    path: request.path.clone(),
                    body: request.body.as_ref().map(|body| body.as_bytes().to_vec()),
                },
                Some(&port),
                FIXTURE_TIMESTAMP,
            );
            assert_eq!(
                response.status, expected.status,
                "case {} request {index}: response={:?} expected={:?}",
                case.name, response.body, expected.envelope
            );
            assert_eq!(
                response.headers, expected.headers,
                "case {} request {index}",
                case.name
            );
            assert_eq!(
                response.body, expected.envelope,
                "case {} request {index}",
                case.name
            );
        }

        let calls = port.calls();
        let expected_calls = case
            .expected
            .iter()
            .filter(|expected| expected.port_call)
            .count();
        assert_eq!(
            calls.len(),
            expected_calls,
            "case {} port-call count",
            case.name
        );
        if case.name == "repeated-identical-request" {
            assert_eq!(
                case.expected_observation.call_count, 1,
                "Go cache observation"
            );
            assert_eq!(calls.len(), 2, "Rust leaf receives both replay calls");
            assert_eq!(calls[0], calls[1], "repeated request query shape");
        } else {
            assert_eq!(
                calls.len(),
                case.expected_observation.call_count,
                "case {}",
                case.name
            );
            for (call, expected) in calls.iter().zip(&case.expected_observation.calls) {
                assert_eq!(call.broker_id, expected.broker_id, "case {}", case.name);
                assert_eq!(call.market, expected.market, "case {}", case.name);
                assert_eq!(
                    call.offset.to_string(),
                    expected.cursor,
                    "case {}",
                    case.name
                );
                assert_eq!(call.limit, expected.page_size, "case {}", case.name);
                assert_eq!(call.offset, expected.page_from, "case {}", case.name);
            }
        }
    }
}

#[test]
fn research_screen_leaf_fails_closed_and_keeps_route_isolation() {
    let valid = ResearchScreenWriteRequest {
        method: "POST".to_owned(),
        path: RESEARCH_SCREEN_PATH.to_owned(),
        body: Some(
            br#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}"#.to_vec(),
        ),
    };
    let unavailable = dispatch_research_screen_write(&valid, None, FIXTURE_TIMESTAMP);
    assert_eq!(unavailable.status, 503);
    assert_eq!(
        unavailable.body["error"]["code"],
        "RESEARCH_SCREEN_UNAVAILABLE"
    );

    let wrong_method = ResearchScreenWriteRequest {
        method: "GET".to_owned(),
        ..valid.clone()
    };
    let wrong_method_response =
        dispatch_research_screen_write(&wrong_method, None, FIXTURE_TIMESTAMP);
    assert_eq!(wrong_method_response.status, 404);
    assert_eq!(wrong_method_response.body["error"]["code"], "NOT_FOUND");

    let wrong_path = ResearchScreenWriteRequest {
        path: "/api/v1/research/screens/extra".to_owned(),
        ..valid
    };
    let wrong_path_response = dispatch_research_screen_write(&wrong_path, None, FIXTURE_TIMESTAMP);
    assert_eq!(wrong_path_response.status, 404);
    assert_eq!(wrong_path_response.body["error"]["code"], "NOT_FOUND");
}

#[derive(Debug)]
struct InvalidResultPort {
    result: Value,
}

impl ResearchScreenWritePort for InvalidResultPort {
    fn query(
        &self,
        _request: &ResearchScreenWriteQuery,
    ) -> Result<Value, ResearchScreenWritePortError> {
        Ok(self.result.clone())
    }
}

#[test]
fn research_screen_leaf_maps_invalid_provider_rows_to_bad_gateway() {
    let port = InvalidResultPort {
        result: json!({"entries": [{"cells": "invalid"}], "hasMore": false}),
    };
    let request = ResearchScreenWriteRequest {
        method: "POST".to_owned(),
        path: RESEARCH_SCREEN_PATH.to_owned(),
        body: Some(
            br#"{"brokerId":"api-test","market":"US","catalogVersion":"futu-stock-screen-v1","querySchemaVersion":2}"#.to_vec(),
        ),
    };
    let response = dispatch_research_screen_write(&request, Some(&port), FIXTURE_TIMESTAMP);
    assert_eq!(response.status, 502);
    assert_eq!(response.body["error"]["code"], "BROKER_FEATURE_FAILED");
    assert_eq!(
        response.body["error"]["message"],
        "broker returned an invalid stock-screen row"
    );
}

fn fixture() -> Fixture {
    serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/research-screens.json"
    ))
    .expect("research screens fixture")
}

fn error_from_envelope(expected: &FixtureExpected) -> ResearchScreenWritePortError {
    let code = expected.envelope["error"]["code"]
        .as_str()
        .expect("research screen error code");
    let message = expected.envelope["error"]["message"]
        .as_str()
        .expect("research screen error message")
        .to_owned();
    match code {
        "RESEARCH_SCREEN_RATE_LIMITED" => ResearchScreenWritePortError::RateLimited {
            message,
            retry_after: expected.headers["Retry-After"]
                .parse()
                .expect("retry-after seconds"),
        },
        "BROKER_CAPABILITY_UNAVAILABLE" => ResearchScreenWritePortError::Capability(message),
        "MARKET_DATA_PROVIDER_WARMING" => ResearchScreenWritePortError::ProviderWarming,
        "MARKET_DATA_PROVIDER_BUSY" => ResearchScreenWritePortError::ProviderBusy,
        "RESEARCH_SCREEN_UNAVAILABLE" => ResearchScreenWritePortError::Unavailable,
        "BROKER_FEATURE_FAILED" => ResearchScreenWritePortError::Failed(message),
        other => panic!("unexpected research screen fixture error {other}"),
    }
}
