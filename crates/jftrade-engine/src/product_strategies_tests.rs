use std::collections::BTreeMap;
use std::sync::Arc;

use serde::Deserialize;
use serde_json::Value;
use tempfile::tempdir;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct StrategyReadFixture {
    version: String,
    cases: Vec<StrategyReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct StrategyReadCase {
    name: String,
    request_path: String,
    expected_status: u16,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug)]
struct FixtureStrategyReadPort {
    responses: BTreeMap<(String, String), Value>,
}

impl FixtureStrategyReadPort {
    fn from_fixture(fixture: &StrategyReadFixture) -> Self {
        let responses = fixture
            .cases
            .iter()
            .filter_map(|case| {
                let data = case.data.clone()?;
                let (path, query) = split_strategy_request_path(&case.request_path);
                Some(((path.to_owned(), query.to_owned()), data))
            })
            .collect();
        Self { responses }
    }
}

impl StrategyReadSnapshotPort for FixtureStrategyReadPort {
    fn read(&self, path: &str, query: &str) -> Result<Option<Value>, StrategyReadSnapshotError> {
        if path.ends_with("/logs") && query.contains("limit=bad") {
            return Err(StrategyReadSnapshotError::Invalid(
                "invalid logs query".to_owned(),
            ));
        }
        if path.ends_with("/audit") && query.contains("toTime=not-a-time") {
            return Err(StrategyReadSnapshotError::Invalid(
                "invalid audit query".to_owned(),
            ));
        }
        Ok(self
            .responses
            .get(&(path.to_owned(), query.to_owned()))
            .cloned())
    }
}

#[derive(Debug)]
struct FailingStrategyReadPort;

impl StrategyReadSnapshotPort for FailingStrategyReadPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Option<Value>, StrategyReadSnapshotError> {
        Err(StrategyReadSnapshotError::Unavailable(
            "Go strategy catalog/activity snapshot unavailable".to_owned(),
        ))
    }
}

fn strategy_read_fixture() -> StrategyReadFixture {
    let fixture: StrategyReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/strategy-instance-read.json"
    ))
    .expect("strategy instance read fixture");
    assert_eq!(fixture.version, "stage9.strategy-instance-read.v1");
    fixture
}

fn split_strategy_request_path(request_path: &str) -> (&str, &str) {
    request_path.split_once('?').unwrap_or((request_path, ""))
}

#[tokio::test]
async fn strategy_instance_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = strategy_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_read_snapshot_port(Arc::new(FixtureStrategyReadPort::from_fixture(
                &fixture,
            )));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 51);
    let address = handle.startup_record().address;
    for case in &fixture.cases {
        let (path, _) = split_strategy_request_path(&case.request_path);
        let (status, response) =
            request_json_with_status(address, "GET", &case.request_path, None, &[]).await;
        assert_eq!(status, case.expected_status, "case {}", case.name);
        if let Some(expected) = &case.data {
            assert_eq!(response["ok"], true, "case {}", case.name);
            assert_eq!(response["data"], *expected, "case {}", case.name);
        } else {
            assert_eq!(response["ok"], false, "case {}", case.name);
            assert_eq!(
                response["error"]["code"].as_str(),
                case.error_code.as_deref(),
                "case {} ({path})",
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
async fn strategy_instance_read_routes_fail_closed_when_snapshot_port_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_read_snapshot_port(Arc::new(FailingStrategyReadPort));
    let handle = start_product(config).await.expect("start product");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/strategies/fixture-running/logs",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 500);
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "STRATEGY_FAILED");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn strategy_instance_read_routes_are_not_registered_without_snapshot_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/strategies",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}
