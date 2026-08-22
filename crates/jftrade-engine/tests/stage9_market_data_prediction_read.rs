use std::collections::BTreeMap;
use std::sync::Arc;

use jftrade_api::{ApiOutput, ApiRequest};
use serde::Deserialize;
use serde_json::Value;

mod prediction_read {
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/src/product_market_data_prediction_read_port.rs"
    ));
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/src/product_market_data_prediction_read_routes.rs"
    ));
    include!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/src/product_market_data_prediction_read_api.rs"
    ));
}

use prediction_read::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct MarketDataPredictionReadFixture {
    version: String,
    cases: Vec<MarketDataPredictionReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct MarketDataPredictionReadCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    #[serde(default)]
    headers: BTreeMap<String, String>,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug)]
struct FixtureMarketDataPredictionReadPort {
    responses: BTreeMap<String, Result<Value, MarketDataPredictionReadSnapshotError>>,
}

impl FixtureMarketDataPredictionReadPort {
    fn from_fixture(fixture: &MarketDataPredictionReadFixture) -> Self {
        let responses = fixture
            .cases
            .iter()
            .map(|case| {
                let response = match &case.data {
                    Some(data) => Ok(data.clone()),
                    None => Err(MarketDataPredictionReadSnapshotError::Failed {
                        status: case.expected_status,
                        code: case.error_code.clone().unwrap_or_default(),
                        message: case.error_message.clone().unwrap_or_default(),
                        retry_after_seconds: case
                            .headers
                            .get("Retry-After")
                            .and_then(|value| value.parse().ok()),
                    }),
                };
                (case.request_path.clone(), response)
            })
            .collect();
        Self { responses }
    }
}

impl MarketDataPredictionReadSnapshotPort for FixtureMarketDataPredictionReadPort {
    fn read(
        &self,
        path: &str,
        query: &str,
    ) -> Result<Value, MarketDataPredictionReadSnapshotError> {
        let key = if query.is_empty() {
            path.to_owned()
        } else {
            format!("{path}?{query}")
        };
        self.responses.get(&key).cloned().unwrap_or_else(|| {
            Err(MarketDataPredictionReadSnapshotError::Unavailable(
                "fixture response missing".to_owned(),
            ))
        })
    }
}

fn fixture() -> MarketDataPredictionReadFixture {
    let fixture: MarketDataPredictionReadFixture = serde_json::from_str(include_str!(concat!(
        env!("CARGO_MANIFEST_DIR"),
        "/../../tests/fixtures/rust-migration/stage9/market-data-prediction-read.json"
    )))
    .expect("market-data prediction read fixture");
    assert_eq!(fixture.version, "stage9.market-data-prediction-read.v1");
    fixture
}

fn request_for(request_path: &str, method: &str) -> ApiRequest {
    let (path, query) = request_path.split_once('?').unwrap_or((request_path, ""));
    ApiRequest {
        method: method.to_owned(),
        path: path.to_owned(),
        query: query.to_owned(),
        body: Vec::new(),
        request_id: "prediction-read-fixture".to_owned(),
        desktop_trusted: true,
        origin_provided: false,
        origin_allowed: true,
        browser_authenticated: true,
    }
}

#[test]
fn prediction_read_routes_cover_the_complete_group() {
    assert_eq!(market_data_prediction_read_routes().len(), 12);
    assert!(
        market_data_prediction_read_routes()
            .iter()
            .all(|(method, _)| *method == "GET")
    );
    assert!(is_market_data_prediction_read_path(
        "/api/v1/market-data/prediction/contracts/US.EC-42/candles/history"
    ));
    assert!(!is_market_data_prediction_read_path(
        "/api/v1/market-data/prediction/contracts/US.EC-42/subscriptions"
    ));
}

#[test]
fn prediction_read_replays_go_fixture_data_errors_and_retry_metadata() {
    let fixture = fixture();
    let port = Arc::new(FixtureMarketDataPredictionReadPort::from_fixture(&fixture));
    let api = MarketDataPredictionReadApi::new(Some(port));
    for case in &fixture.cases {
        let request = request_for(&case.request_path, &case.method);
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
}

#[test]
fn prediction_read_fails_closed_without_a_snapshot_port() {
    let api = MarketDataPredictionReadApi::new(None);
    let request = request_for("/api/v1/market-data/prediction/categories?market=US", "GET");
    let error = api
        .dispatch(&request)
        .expect_err("missing port must fail closed");
    assert_eq!(error.status, 503);
    assert_eq!(error.code, "MARKET_DATA_PREDICTION_READ_UNAVAILABLE");
}

#[test]
fn prediction_read_rejects_unknown_paths_and_non_get_methods() {
    let fixture = fixture();
    let port = Arc::new(FixtureMarketDataPredictionReadPort::from_fixture(&fixture));
    let api = MarketDataPredictionReadApi::new(Some(port));
    let unknown = request_for(
        "/api/v1/market-data/prediction/contracts/US.EC-42/subscriptions",
        "GET",
    );
    let error = api
        .dispatch(&unknown)
        .expect_err("subscription route is outside read group");
    assert_eq!(error.status, 404);
    assert_eq!(error.code, "NOT_FOUND");
    let post = request_for("/api/v1/market-data/prediction/categories", "POST");
    let error = api
        .dispatch(&post)
        .expect_err("POST route is outside read group");
    assert_eq!(error.status, 404);
    assert_eq!(error.code, "NOT_FOUND");
}
