use std::collections::BTreeMap;
use std::sync::Arc;

use serde::Deserialize;
use serde_json::Value;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct MarketDataOptionsReadFixture {
    version: String,
    cases: Vec<MarketDataOptionsReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct MarketDataOptionsReadCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug)]
struct FixtureMarketDataOptionsReadPort {
    responses: BTreeMap<String, Result<Value, MarketDataOptionsReadSnapshotError>>,
}

impl FixtureMarketDataOptionsReadPort {
    fn from_fixture(fixture: &MarketDataOptionsReadFixture) -> Self {
        let responses = fixture
            .cases
            .iter()
            .map(|case| {
                let response = match &case.data {
                    Some(data) => Ok(data.clone()),
                    None => Err(MarketDataOptionsReadSnapshotError::Failed {
                        status: case.expected_status,
                        code: case.error_code.clone().unwrap_or_default(),
                        message: case.error_message.clone().unwrap_or_default(),
                    }),
                };
                (case.request_path.clone(), response)
            })
            .collect();
        Self { responses }
    }
}

impl MarketDataOptionsReadSnapshotPort for FixtureMarketDataOptionsReadPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, MarketDataOptionsReadSnapshotError> {
        let key = if query.is_empty() {
            path.to_owned()
        } else {
            format!("{path}?{query}")
        };
        self.responses.get(&key).cloned().unwrap_or_else(|| {
            Err(MarketDataOptionsReadSnapshotError::Unavailable(
                "fixture response missing".to_owned(),
            ))
        })
    }
}

#[derive(Debug)]
struct FailingMarketDataOptionsReadPort;

impl MarketDataOptionsReadSnapshotPort for FailingMarketDataOptionsReadPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, MarketDataOptionsReadSnapshotError> {
        Err(MarketDataOptionsReadSnapshotError::Unavailable(
            "Go market-data options owner unavailable".to_owned(),
        ))
    }
}

fn market_data_options_read_fixture() -> MarketDataOptionsReadFixture {
    let fixture: MarketDataOptionsReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/market-data-options-read.json"
    ))
    .expect("market-data options fixture");
    assert_eq!(fixture.version, "stage9.market-data-options-read.v1");
    fixture
}

#[tokio::test]
async fn market_data_options_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = market_data_options_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_options_read_snapshot_port(Arc::new(
                FixtureMarketDataOptionsReadPort::from_fixture(&fixture),
            ));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 53);
    for case in &fixture.cases {
        let (status, response) = request_json_with_status(
            handle.startup_record().address,
            &case.method,
            &case.request_path,
            None,
            &[],
        )
        .await;
        assert_eq!(status, case.expected_status, "case {}", case.name);
        if let Some(expected) = &case.data {
            assert_eq!(response["ok"], true, "case {}", case.name);
            assert_eq!(response["data"], *expected, "case {}", case.name);
        } else {
            assert_eq!(response["ok"], false, "case {}", case.name);
            assert_eq!(
                response["error"]["code"].as_str(),
                case.error_code.as_deref()
            );
            assert_eq!(
                response["error"]["message"].as_str(),
                case.error_message.as_deref()
            );
        }
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn market_data_options_read_routes_fail_closed_when_snapshot_port_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_options_read_snapshot_port(Arc::new(
                FailingMarketDataOptionsReadPort,
            ));
    let handle = start_product(config).await.expect("start product");
    for path in [
        "/api/v1/market-data/options/chains/US.AAPL",
        "/api/v1/market-data/options/events",
    ] {
        let response = request_json(handle.startup_record().address, "GET", path, None).await;
        assert_eq!(response["ok"], false, "path {path}");
        assert_eq!(response["error"]["code"], "MARKET_DATA_OPTIONS_UNAVAILABLE");
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn market_data_options_read_routes_are_not_registered_without_snapshot_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let response = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/market-data/options/events",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}
