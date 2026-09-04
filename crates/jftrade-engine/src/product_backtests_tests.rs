use serde::Deserialize;
use serde_json::Value;
use std::collections::BTreeMap;
use std::sync::Arc;
use tempfile::tempdir;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BacktestsReadFixture {
    version: String,
    cases: Vec<BacktestsReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BacktestsReadCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug)]
struct FixtureBacktestReadPort {
    data: BTreeMap<String, Value>,
    list_case: String,
}

impl FixtureBacktestReadPort {
    fn from_fixture(fixture: &BacktestsReadFixture, list_case: &str) -> Self {
        Self {
            data: fixture
                .cases
                .iter()
                .filter_map(|case| case.data.clone().map(|data| (case.name.clone(), data)))
                .collect(),
            list_case: list_case.to_owned(),
        }
    }
}

impl BacktestReadSnapshotPort for FixtureBacktestReadPort {
    fn list(&self) -> Result<Value, BacktestReadSnapshotError> {
        self.data.get(&self.list_case).cloned().ok_or_else(|| {
            BacktestReadSnapshotError::Unavailable("list fixture is missing".to_owned())
        })
    }

    fn status(&self, run_id: &str) -> Result<Option<Value>, BacktestReadSnapshotError> {
        if run_id == "fixture-run" {
            return Ok(self.data.get("status-existing").cloned());
        }
        Ok(None)
    }

    fn result(&self, run_id: &str) -> Result<Option<Value>, BacktestReadSnapshotError> {
        if run_id == "store-failure" {
            return Err(BacktestReadSnapshotError::Unavailable(
                "load backtest result failed".to_owned(),
            ));
        }
        if run_id == "fixture-run" {
            return Ok(self.data.get("result-existing").cloned());
        }
        Ok(None)
    }

    fn result_view(
        &self,
        request: &BacktestResultViewRequest,
    ) -> Result<Option<BacktestResultViewSnapshot>, BacktestResultViewError> {
        if request.run_id == "store-failure" {
            return Err(BacktestResultViewError::Unavailable(
                "load backtest result failed".to_owned(),
            ));
        }
        if request.run_id == "fixture-run" {
            let res = self.data.get("result-existing").cloned().unwrap_or(Value::Null);
            return Ok(Some(BacktestResultViewSnapshot { data: res }));
        }
        Ok(None)
    }
}

#[derive(Debug)]
struct FailingBacktestReadPort;

impl BacktestReadSnapshotPort for FailingBacktestReadPort {
    fn list(&self) -> Result<Value, BacktestReadSnapshotError> {
        Err(BacktestReadSnapshotError::Unavailable(
            "Go backtest run store unavailable".to_owned(),
        ))
    }

    fn status(&self, _run_id: &str) -> Result<Option<Value>, BacktestReadSnapshotError> {
        Err(BacktestReadSnapshotError::Unavailable(
            "Go backtest run store unavailable".to_owned(),
        ))
    }

    fn result(&self, _run_id: &str) -> Result<Option<Value>, BacktestReadSnapshotError> {
        Err(BacktestReadSnapshotError::Unavailable(
            "Go backtest run store unavailable".to_owned(),
        ))
    }

    fn result_view(
        &self,
        _request: &BacktestResultViewRequest,
    ) -> Result<Option<BacktestResultViewSnapshot>, BacktestResultViewError> {
        Err(BacktestResultViewError::Unavailable(
            "Go backtest run store unavailable".to_owned(),
        ))
    }
}

fn backtests_read_fixture() -> BacktestsReadFixture {
    let fixture: BacktestsReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/backtests-read.json"
    ))
    .expect("backtests read fixture");
    assert_eq!(fixture.version, "stage9.backtests-read.v1");
    fixture
}

#[tokio::test]
async fn backtests_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = backtests_read_fixture();
    for case in &fixture.cases {
        let directory = tempdir().expect("temporary directory");
        let settings_path = directory.path().join("settings.json");
        let list_case = if case.name == "list-empty" {
            "list-empty"
        } else {
            "list"
        };
        let config =
            ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
                .expect("config")
                .with_backtest_read_snapshot_port(Arc::new(FixtureBacktestReadPort::from_fixture(
                    &fixture, list_case,
                )));
        let handle = start_product(config).await.expect("start product");
        assert_eq!(
            handle.startup_record().owned_routes,
            51,
            "case {}",
            case.name
        );
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
        handle.shutdown().await.expect("shutdown product");
    }
}

#[tokio::test]
async fn backtests_read_routes_fail_closed_when_snapshot_port_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_backtest_read_snapshot_port(Arc::new(FailingBacktestReadPort));
    let handle = start_product(config).await.expect("start product");
    for path in [
        "/api/v1/backtests",
        "/api/v1/backtests/fixture-run/status",
        "/api/v1/backtests/fixture-run",
    ] {
        let (status, response) =
            request_json_with_status(handle.startup_record().address, "GET", path, None, &[]).await;
        assert_eq!(status, 500, "path {path}");
        assert_eq!(response["ok"], false, "path {path}");
        assert_eq!(
            response["error"]["code"], "BACKTEST_RUN_STORE_FAILED",
            "path {path}"
        );
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn backtests_read_routes_are_not_registered_without_snapshot_port() {
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
        "/api/v1/backtests",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}
