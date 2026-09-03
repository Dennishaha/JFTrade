use std::collections::BTreeMap;
use std::sync::Arc;

use serde::Deserialize;
use serde_json::Value;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct PortfolioReadFixture {
    version: String,
    cases: Vec<PortfolioReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct PortfolioReadCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    data: Option<Value>,
}

#[derive(Debug)]
struct FixturePortfolioSnapshotPort {
    responses: BTreeMap<String, Value>,
}

impl FixturePortfolioSnapshotPort {
    fn from_fixture(fixture: &PortfolioReadFixture) -> Self {
        Self {
            responses: fixture
                .cases
                .iter()
                .filter_map(|case| {
                    case.data
                        .clone()
                        .map(|data| (case.request_path.clone(), data))
                })
                .collect(),
        }
    }
}

impl PortfolioSnapshotPort for FixturePortfolioSnapshotPort {
    fn read(&self, path: &str, _query: &str) -> Result<Value, PortfolioSnapshotError> {
        self.responses.get(path).cloned().ok_or_else(|| {
            PortfolioSnapshotError::Unavailable("fixture response missing".to_owned())
        })
    }
}

#[derive(Debug)]
struct FailingPortfolioSnapshotPort;

impl PortfolioSnapshotPort for FailingPortfolioSnapshotPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, PortfolioSnapshotError> {
        Err(PortfolioSnapshotError::Unavailable(
            "Go portfolio snapshot unavailable".to_owned(),
        ))
    }
}

fn portfolio_read_fixture() -> PortfolioReadFixture {
    let fixture: PortfolioReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/portfolio-read.json"
    ))
    .expect("portfolio read fixture");
    assert_eq!(fixture.version, "stage9.portfolio-read.v1");
    fixture
}

#[tokio::test]
async fn portfolio_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = portfolio_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_portfolio_snapshot_port(Arc::new(FixturePortfolioSnapshotPort::from_fixture(
                &fixture,
            )));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 50);
    let address = handle.startup_record().address;
    for case in &fixture.cases {
        let (status, response) =
            request_json_with_status(address, &case.method, &case.request_path, None, &[]).await;
        assert_eq!(status, case.expected_status, "case {}", case.name);
        assert_eq!(response["ok"], true, "case {}", case.name);
        assert_eq!(response["data"], case.data.clone().expect("fixture data"));
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn portfolio_read_routes_fail_closed_when_snapshot_port_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_portfolio_snapshot_port(Arc::new(FailingPortfolioSnapshotPort));
    let handle = start_product(config).await.expect("start product");
    for path in [
        "/api/v1/portfolio/fixture/cash-balances",
        "/api/v1/portfolio/fixture/positions",
    ] {
        let response = request_json(handle.startup_record().address, "GET", path, None).await;
        assert_eq!(response["ok"], false, "path {path}");
        assert_eq!(
            response["error"]["code"], "PORTFOLIO_UNAVAILABLE",
            "path {path}"
        );
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn portfolio_read_routes_are_not_registered_without_snapshot_port() {
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
        "/api/v1/portfolio/fixture/positions",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}
