use std::collections::BTreeMap;
use std::sync::Arc;

use serde::Deserialize;
use serde_json::Value;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct MarketDataCatalogReadFixture {
    version: String,
    cases: Vec<MarketDataCatalogReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct MarketDataCatalogReadCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    data: Option<Value>,
    error_status: Option<u16>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug)]
struct FixtureMarketDataCatalogReadPort {
    responses: BTreeMap<String, Result<Value, MarketDataCatalogReadSnapshotError>>,
}

impl FixtureMarketDataCatalogReadPort {
    fn from_fixture(fixture: &MarketDataCatalogReadFixture) -> Self {
        let responses = fixture
            .cases
            .iter()
            .map(|case| {
                let response = match (&case.data, case.error_status, &case.error_code) {
                    (Some(data), _, _) => Ok(data.clone()),
                    (None, Some(400), Some(code)) => {
                        Err(MarketDataCatalogReadSnapshotError::Invalid {
                            code: code.clone(),
                            message: case.error_message.clone().unwrap_or_default(),
                        })
                    }
                    (None, Some(status), Some(code)) => {
                        Err(MarketDataCatalogReadSnapshotError::Failed {
                            status,
                            code: code.clone(),
                            message: case.error_message.clone().unwrap_or_default(),
                        })
                    }
                    _ => Err(MarketDataCatalogReadSnapshotError::Unavailable(
                        "fixture response missing".to_owned(),
                    )),
                };
                (case.request_path.clone(), response)
            })
            .collect();
        Self { responses }
    }
}

impl MarketDataCatalogReadSnapshotPort for FixtureMarketDataCatalogReadPort {
    fn read<'a>(&'a self, path: &'a str, query: &'a str) -> MarketDataCatalogReadFuture<'a> {
        let key = if query.is_empty() {
            path.to_owned()
        } else {
            format!("{path}?{query}")
        };
        let res = self.responses.get(&key).cloned().unwrap_or_else(|| {
            Err(MarketDataCatalogReadSnapshotError::Unavailable(
                "fixture response missing".to_owned(),
            ))
        });
        Box::pin(std::future::ready(res))
    }
}

#[derive(Debug)]
struct FailingMarketDataCatalogReadPort;

impl MarketDataCatalogReadSnapshotPort for FailingMarketDataCatalogReadPort {
    fn read<'a>(&'a self, _path: &'a str, _query: &'a str) -> MarketDataCatalogReadFuture<'a> {
        Box::pin(std::future::ready(Err(
            MarketDataCatalogReadSnapshotError::Unavailable(
                "Go market-data catalog owner unavailable".to_owned(),
            ),
        )))
    }
}

fn market_data_catalog_read_fixture() -> MarketDataCatalogReadFixture {
    let fixture: MarketDataCatalogReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/market-data-catalog-read.json"
    ))
    .expect("market-data catalog read fixture");
    assert_eq!(fixture.version, "stage9.market-data-catalog-read.v1");
    fixture
}

#[tokio::test]
async fn market_data_catalog_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = market_data_catalog_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_catalog_read_snapshot_port(Arc::new(
                FixtureMarketDataCatalogReadPort::from_fixture(&fixture),
            ));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 50);
    for case in &fixture.cases {
        assert_eq!(case.method, "GET", "case {}", case.name);
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
                case.error_code.as_deref(),
                "case {}",
                case.name
            );
            assert_eq!(
                response["error"]["message"].as_str(),
                case.error_message.as_deref(),
                "case {}",
                case.name
            );
        }
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn market_data_catalog_read_routes_fail_closed_when_snapshot_port_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_market_data_catalog_read_snapshot_port(Arc::new(
                FailingMarketDataCatalogReadPort,
            ));
    let handle = start_product(config).await.expect("start product");
    let response = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/market-data/markets",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "MARKET_DATA_CATALOG_UNAVAILABLE");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn market_data_catalog_read_routes_are_not_registered_without_snapshot_port() {
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
        "/api/v1/market-data/markets",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}
