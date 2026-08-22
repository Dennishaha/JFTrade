use std::collections::{BTreeMap, HashMap};
use std::sync::{Arc, Mutex};

use jftrade_api::{ApiOutput, ApiRequest};
use serde::Deserialize;
use serde_json::Value;

mod product_market_data_subscription_mutation_port {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/src/product_market_data_subscription_mutation_port.rs"
    ));
}

mod product_market_data_subscription_mutation_api {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/src/product_market_data_subscription_mutation_api.rs"
    ));
}

mod product_market_data_subscription_mutation_routes {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/src/product_market_data_subscription_mutation_routes.rs"
    ));
}

use product_market_data_subscription_mutation_api::MarketDataSubscriptionMutationApi;
use product_market_data_subscription_mutation_port::{
    MarketDataSubscriptionMutationPort, MarketDataSubscriptionMutationPortError,
    MarketDataSubscriptionMutationRequest,
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
    #[serde(default)]
    context_error: String,
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
    calls: Mutex<Vec<MarketDataSubscriptionMutationRequest>>,
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

    fn calls(&self) -> Vec<MarketDataSubscriptionMutationRequest> {
        self.calls
            .lock()
            .expect("subscription mutation call lock")
            .clone()
    }
}

impl MarketDataSubscriptionMutationPort for FixturePort {
    fn dispatch(
        &self,
        request: &MarketDataSubscriptionMutationRequest,
    ) -> Result<Value, MarketDataSubscriptionMutationPortError> {
        self.calls
            .lock()
            .expect("subscription mutation call lock")
            .push(request.clone());
        let request_path = if request.query.is_empty() {
            request.path.clone()
        } else {
            format!("{}?{}", request.path, request.query)
        };
        let key = fixture_key(&request.method, &request_path, &request.body);
        let offset = {
            let mut offsets = self
                .offsets
                .lock()
                .expect("subscription mutation offset lock");
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
                MarketDataSubscriptionMutationPortError::Unavailable(format!(
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
            } => Err(MarketDataSubscriptionMutationPortError::Failed {
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
        "/../../tests/fixtures/rust-migration/stage9/market-data-subscription-mutation.json"
    )))
    .expect("market-data subscription mutation fixture");
    assert_eq!(
        fixture.version,
        "stage9.market-data-subscription-mutation.v1"
    );
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
        request_id: "stage9-market-data-subscription-mutation".to_owned(),
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
fn subscription_mutation_routes_cover_the_complete_group() {
    let routes =
        product_market_data_subscription_mutation_port::market_data_subscription_mutation_routes();
    assert_eq!(routes.len(), 6);
    assert_eq!(
        routes
            .iter()
            .filter(|(method, _)| *method == "POST")
            .count(),
        4
    );
    assert_eq!(
        product_market_data_subscription_mutation_routes::market_data_subscription_mutation_route_specs(),
        routes.to_vec()
    );
    assert!(
        product_market_data_subscription_mutation_port::is_market_data_subscription_mutation_path(
            "POST",
            "/api/v1/market-data/prediction/contracts/US.EC-42/subscriptions"
        )
    );
    assert!(
        product_market_data_subscription_mutation_port::is_market_data_subscription_mutation_path(
            "DELETE",
            "/api/v1/market-data/prediction/contracts/US.EC-42/subscriptions/fixture-lease"
        )
    );
    assert!(
        !product_market_data_subscription_mutation_port::is_market_data_subscription_mutation_path(
            "GET",
            "/api/v1/market-data/subscriptions"
        )
    );
}

#[test]
fn subscription_mutation_replay_matches_go_fixture_data_errors_and_retry_metadata() {
    let fixture = fixture();
    let port = Arc::new(FixturePort::from_fixture(&fixture));
    let api = MarketDataSubscriptionMutationApi::new(Some(port.clone()));
    for case in &fixture.cases {
        let request = request_for(case);
        match api.dispatch(&request) {
            Ok(ApiOutput::Json(data)) => {
                assert_eq!(case.expected_status, 200, "case {}", case.name);
                assert_eq!(case.data.as_ref(), Some(&data), "case {}", case.name);
            }
            Ok(output) => panic!("case {} returned {output:?}", case.name),
            Err(error) => {
                assert_eq!(error.status, case.expected_status, "case {}", case.name);
                assert_eq!(
                    error.code,
                    case.error_code.clone().unwrap_or_default(),
                    "case {}",
                    case.name
                );
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
        }
    }

    let expected_calls = fixture
        .cases
        .iter()
        .filter(|case| {
            case.method != "POST"
                || serde_json::Deserializer::from_str(&case.body)
                    .into_iter::<Value>()
                    .next()
                    .is_some_and(|result| result.is_ok())
        })
        .count();
    assert_eq!(port.calls().len(), expected_calls);
    for call in port.calls() {
        assert!(
            product_market_data_subscription_mutation_port::is_market_data_subscription_mutation_path(
                &call.method,
                &call.path,
            ),
            "unexpected forwarded path {} {}",
            call.method,
            call.path
        );
    }
}

#[test]
fn subscription_mutation_fails_closed_without_port_and_rejects_unknown_routes() {
    let fixture = fixture();
    let api = MarketDataSubscriptionMutationApi::new(None);
    let valid_case = fixture
        .cases
        .iter()
        .find(|case| case.name == "acquire-success-normalizes-instruments")
        .expect("valid subscription mutation case");
    let error = api
        .dispatch(&request_for(valid_case))
        .expect_err("missing port must fail closed");
    assert_eq!(error.status, 503);
    assert_eq!(
        error.code,
        product_market_data_subscription_mutation_api::MARKET_DATA_SUBSCRIPTION_MUTATION_UNAVAILABLE_CODE
    );

    let port = Arc::new(FixturePort::from_fixture(&fixture));
    let api = MarketDataSubscriptionMutationApi::new(Some(port));
    let unknown = ApiRequest {
        method: "GET".to_owned(),
        path: "/api/v1/market-data/subscriptions".to_owned(),
        query: String::new(),
        body: Vec::new(),
        request_id: "stage9-unknown".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
    };
    let error = api
        .dispatch(&unknown)
        .expect_err("GET snapshot must stay outside mutation group");
    assert_eq!(error.status, 404);
    assert_eq!(error.code, "NOT_FOUND");
}

#[test]
fn subscription_mutation_rejects_malformed_post_bodies_at_the_leaf_boundary() {
    let fixture = fixture();
    let port = Arc::new(FixturePort::from_fixture(&fixture));
    let api = MarketDataSubscriptionMutationApi::new(Some(port));
    for (path, message) in [
        (
            "/api/v1/market-data/subscriptions",
            "invalid subscription request",
        ),
        (
            "/api/v1/market-data/subscriptions/release",
            "invalid release request",
        ),
        (
            "/api/v1/market-data/subscriptions/heartbeat",
            "invalid heartbeat request",
        ),
        (
            "/api/v1/market-data/prediction/contracts/EC-42/subscriptions",
            "invalid prediction subscription payload",
        ),
    ] {
        let request = ApiRequest {
            method: "POST".to_owned(),
            path: path.to_owned(),
            query: String::new(),
            body: b"{".to_vec(),
            request_id: "stage9-malformed".to_owned(),
            desktop_trusted: true,
            origin_provided: false,
            origin_allowed: true,
            browser_authenticated: true,
        };
        let error = api.dispatch(&request).expect_err("malformed body");
        assert_eq!(error.status, 400, "path {path}");
        assert_eq!(error.code, "BAD_REQUEST", "path {path}");
        assert_eq!(error.message, message, "path {path}");
    }
}

#[test]
fn subscription_mutation_fixture_preserves_non_wire_provider_observations() {
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
            .any(|case| case.context_error == "canceled")
    );
}
