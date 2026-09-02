use serde::Deserialize;
use serde_json::Value;
use std::collections::BTreeMap;
use std::sync::Arc;
use tempfile::tempdir;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BacktestsSyncReadFixture {
    version: String,
    cases: Vec<BacktestsSyncReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BacktestsSyncReadCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug)]
struct FixtureBacktestSyncReadPort {
    data: BTreeMap<String, Value>,
}

impl FixtureBacktestSyncReadPort {
    fn from_fixture(fixture: &BacktestsSyncReadFixture) -> Self {
        Self {
            data: fixture
                .cases
                .iter()
                .filter_map(|case| case.data.clone().map(|data| (case.name.clone(), data)))
                .collect(),
        }
    }
}

impl BacktestSyncReadSnapshotPort for FixtureBacktestSyncReadPort {
    fn progress(&self, task_id: &str) -> Result<Option<Value>, BacktestSyncReadSnapshotError> {
        let case = match task_id {
            "fixture-queued" => "queued",
            "fixture-running" => "running",
            "fixture-failed" => "failed",
            _ => return Ok(None),
        };
        Ok(self.data.get(case).cloned())
    }
}

#[derive(Debug)]
struct FailingBacktestSyncReadPort;

impl BacktestSyncReadSnapshotPort for FailingBacktestSyncReadPort {
    fn progress(&self, _task_id: &str) -> Result<Option<Value>, BacktestSyncReadSnapshotError> {
        Err(BacktestSyncReadSnapshotError::Unavailable(
            "Go backtest sync task store unavailable".to_owned(),
        ))
    }
}

fn backtests_sync_read_fixture() -> BacktestsSyncReadFixture {
    let fixture: BacktestsSyncReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/backtests-sync-read.json"
    ))
    .expect("backtests sync read fixture");
    assert_eq!(fixture.version, "stage9.backtests-sync-read.v1");
    fixture
}

#[tokio::test]
async fn backtests_sync_read_route_matches_group_fixture_in_cutover_only() {
    let fixture = backtests_sync_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_backtest_sync_read_snapshot_port(Arc::new(
                FixtureBacktestSyncReadPort::from_fixture(&fixture),
            ));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 49);
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
async fn backtests_sync_read_route_fails_closed_when_snapshot_port_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_backtest_sync_read_snapshot_port(Arc::new(FailingBacktestSyncReadPort));
    let handle = start_product(config).await.expect("start product");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/backtests/sync/fixture-running",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 500);
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "BACKTEST_SYNC_TASK_STORE_FAILED");
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn backtests_sync_read_route_is_not_registered_without_snapshot_port() {
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
        "/api/v1/backtests/sync/fixture-running",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}
