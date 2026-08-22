use std::collections::{BTreeMap, HashMap};
use std::sync::{Arc, Mutex};

use jftrade_api::{ApiOutput, ApiRequest};
use serde::Deserialize;
use serde_json::Value;

mod product_market_data_provider_actions_port {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/src/product_market_data_provider_actions_port.rs"
    ));
}

mod product_market_data_provider_actions_api {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/src/product_market_data_provider_actions_api.rs"
    ));
}

mod product_market_data_provider_actions_routes {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/src/product_market_data_provider_actions_routes.rs"
    ));
}

use product_market_data_provider_actions_api::MarketDataProviderActionsApi;
use product_market_data_provider_actions_port::{
    MarketDataProviderActionsPort, MarketDataProviderActionsPortError,
    MarketDataProviderActionsRequest,
};

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
    request_path: String,
    #[serde(default)]
    body: String,
    expected_status: u16,
    #[serde(default)]
    headers: BTreeMap<String, String>,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
    provider_call: Option<Value>,
}

#[derive(Clone, Debug)]
enum FixtureResponse {
    Data(Value),
    Error {
        status: u16,
        code: String,
        message: String,
        retry_after_seconds: Option<u64>,
    },
}

#[derive(Debug, Default)]
struct FixturePort {
    responses: BTreeMap<String, Vec<FixtureResponse>>,
    offsets: Mutex<HashMap<String, usize>>,
    calls: Mutex<Vec<MarketDataProviderActionsRequest>>,
}

impl FixturePort {
    fn from_fixture(fixture: &Fixture) -> Self {
        let mut responses = BTreeMap::<String, Vec<FixtureResponse>>::new();
        for case in &fixture.cases {
            let response = match (&case.data, &case.error_code) {
                (Some(data), _) => FixtureResponse::Data(data.clone()),
                (None, Some(code)) => FixtureResponse::Error {
                    status: case.expected_status,
                    code: code.clone(),
                    message: case.error_message.clone().unwrap_or_default(),
                    retry_after_seconds: case
                        .headers
                        .get("Retry-After")
                        .and_then(|value| value.parse().ok()),
                },
                (None, None) => FixtureResponse::Error {
                    status: 503,
                    code: "FIXTURE_RESPONSE_MISSING".to_owned(),
                    message: "fixture response is missing".to_owned(),
                    retry_after_seconds: None,
                },
            };
            responses
                .entry(fixture_key(
                    &case.method,
                    &case.request_path,
                    case.body.as_bytes(),
                ))
                .or_default()
                .push(response);
        }
        Self {
            responses,
            ..Self::default()
        }
    }

    fn calls(&self) -> Vec<MarketDataProviderActionsRequest> {
        self.calls
            .lock()
            .expect("provider-actions call lock")
            .clone()
    }
}

impl MarketDataProviderActionsPort for FixturePort {
    fn dispatch(
        &self,
        request: &MarketDataProviderActionsRequest,
    ) -> Result<Value, MarketDataProviderActionsPortError> {
        self.calls
            .lock()
            .expect("provider-actions call lock")
            .push(request.clone());
        let request_path = if request.query.is_empty() {
            request.path.clone()
        } else {
            format!("{}?{}", request.path, request.query)
        };
        let key = fixture_key(&request.method, &request_path, &request.body);
        let offset = {
            let mut offsets = self.offsets.lock().expect("provider-actions offset lock");
            let current = offsets.entry(key.clone()).or_default();
            let offset = *current;
            *current += 1;
            offset
        };
        let response = self
            .responses
            .get(&key)
            .and_then(|responses| responses.get(offset))
            .cloned()
            .ok_or_else(|| {
                MarketDataProviderActionsPortError::Unavailable(format!(
                    "fixture response missing for {key} occurrence {offset}"
                ))
            })?;
        match response {
            FixtureResponse::Data(data) => Ok(data),
            FixtureResponse::Error {
                status,
                code,
                message,
                retry_after_seconds,
            } => Err(MarketDataProviderActionsPortError::Failed {
                status,
                code,
                message,
                retry_after_seconds,
            }),
        }
    }
}

fn fixture() -> Fixture {
    let fixture: Fixture = serde_json::from_str(include_str!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../../tests/fixtures/rust-migration/stage9/market-data-provider-actions.json"
    )))
    .expect("market-data provider-actions fixture");
    assert_eq!(fixture.version, "stage9.market-data-provider-actions.v1");
    fixture
}

fn request_for(case: &FixtureCase) -> ApiRequest {
    let (path, query) = case
        .request_path
        .split_once('?')
        .unwrap_or((&case.request_path, ""));
    ApiRequest {
        method: case.method.clone(),
        path: path.to_owned(),
        query: query.to_owned(),
        body: case.body.as_bytes().to_vec(),
        request_id: "stage9-market-data-provider-actions".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
    }
}

fn fixture_key(method: &str, request_path: &str, body: &[u8]) -> String {
    format!("{method} {request_path} {}", String::from_utf8_lossy(body))
}

#[test]
fn provider_actions_routes_cover_exactly_the_five_unique_operations() {
    assert_eq!(
        product_market_data_provider_actions_routes::market_data_provider_actions_route_specs()
            .len(),
        5
    );
    assert_eq!(
        product_market_data_provider_actions_port::market_data_provider_actions_routes().len(),
        5
    );
    assert!(
        product_market_data_provider_actions_routes::market_data_provider_actions_route_specs()
            .iter()
            .all(|(method, _)| *method == "POST")
    );
    assert!(
        product_market_data_provider_actions_port::is_market_data_provider_action_path(
            "/api/v1/market-data/options/analysis/US.AAPL"
        )
    );
    assert!(
        !product_market_data_provider_actions_port::is_market_data_provider_action_path(
            "/api/v1/market-data/prediction/contracts/US.EC-1/subscriptions"
        )
    );
}

#[test]
fn provider_actions_replay_matches_go_fixture_and_forwards_raw_requests() {
    let fixture = fixture();
    let port = Arc::new(FixturePort::from_fixture(&fixture));
    let api = MarketDataProviderActionsApi::new(Some(port.clone()));

    for case in &fixture.cases {
        let request = request_for(case);
        let result = api.dispatch(&request);
        match (&case.data, result) {
            (Some(expected), Ok(ApiOutput::Json(actual))) => {
                assert_eq!(case.expected_status, 200, "case {}", case.name);
                assert_eq!(actual, *expected, "case {}", case.name);
            }
            (None, Err(error)) => {
                assert_eq!(error.status, case.expected_status, "case {}", case.name);
                assert_eq!(error.code, case.error_code.clone().unwrap_or_default());
                assert_eq!(
                    error.message,
                    case.error_message.clone().unwrap_or_default(),
                    "case {}",
                    case.name
                );
                assert_eq!(
                    error.retry_after_seconds.map(|seconds| seconds.to_string()),
                    case.headers.get("Retry-After").cloned(),
                    "case {} retry metadata",
                    case.name
                );
            }
            (Some(_), Ok(output)) => panic!("case {} returned {output:?}", case.name),
            (Some(_), Err(error)) => panic!("case {} failed: {error:?}", case.name),
            (None, Ok(output)) => panic!("case {} unexpectedly succeeded: {output:?}", case.name),
        }
    }

    let calls = port.calls();
    let valid_cases = fixture
        .cases
        .iter()
        .filter(|case| serde_json::from_str::<Value>(&case.body).is_ok())
        .count();
    assert_eq!(calls.len(), valid_cases);
    for call in calls {
        assert_eq!(call.method, "POST");
        assert!(
            product_market_data_provider_actions_port::is_market_data_provider_action_path(
                &call.path
            )
        );
    }
}

#[test]
fn provider_actions_fail_closed_without_port_and_reject_unknown_routes() {
    let fixture = fixture();
    let api = MarketDataProviderActionsApi::new(None);
    let error = api
        .dispatch(&request_for(
            fixture
                .cases
                .iter()
                .find(|case| case.name == "normalize-empty-object")
                .expect("normalize fixture case"),
        ))
        .expect_err("missing port must fail closed");
    assert_eq!(error.status, 503);
    assert_eq!(error.code, "MARKET_DATA_PROVIDER_ACTIONS_UNAVAILABLE");

    let port = Arc::new(FixturePort::from_fixture(&fixture));
    let api = MarketDataProviderActionsApi::new(Some(port));
    let unknown = ApiRequest {
        method: "POST".to_owned(),
        path: "/api/v1/market-data/prediction/contracts/US.EC-1/subscriptions".to_owned(),
        query: String::new(),
        body: b"{}".to_vec(),
        request_id: "stage9-unknown".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
    };
    let error = api
        .dispatch(&unknown)
        .expect_err("subscription mutation must stay outside the group");
    assert_eq!(error.status, 404);
    assert_eq!(error.code, "NOT_FOUND");
}

#[test]
fn provider_actions_preserve_fixture_provider_call_evidence_as_non_wire_metadata() {
    let fixture = fixture();
    assert!(
        fixture
            .cases
            .iter()
            .any(|case| case.provider_call.is_some())
    );
    assert!(
        fixture
            .cases
            .iter()
            .any(|case| case.provider_call.is_none())
    );
    assert!(
        fixture
            .cases
            .iter()
            .any(|case| case.headers.contains_key("Retry-After"))
    );
}
