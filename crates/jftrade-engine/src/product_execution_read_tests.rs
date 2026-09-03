use std::collections::BTreeMap;
use std::sync::Arc;

use serde::Deserialize;
use serde_json::Value;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ExecutionReadFixture {
    version: String,
    cases: Vec<ExecutionReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ExecutionReadCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug)]
struct FixtureExecutionReadPort {
    responses: BTreeMap<String, Result<Value, ExecutionReadSnapshotError>>,
}

impl FixtureExecutionReadPort {
    fn from_fixture(fixture: &ExecutionReadFixture) -> Self {
        let mut responses = BTreeMap::new();
        for case in &fixture.cases {
            let response = match (&case.data, case.error_code.as_deref()) {
                (Some(data), _) => Ok(data.clone()),
                (None, Some("ORDER_NOT_FOUND")) => Err(ExecutionReadSnapshotError::NotFound),
                (None, Some(code)) => Err(ExecutionReadSnapshotError::Failed {
                    code: code.to_owned(),
                    message: case.error_message.clone().unwrap_or_default(),
                }),
                _ => Err(ExecutionReadSnapshotError::Unavailable(
                    "fixture response missing".to_owned(),
                )),
            };
            responses.insert(case.request_path.clone(), response);
        }
        Self { responses }
    }
}

impl ExecutionReadSnapshotPort for FixtureExecutionReadPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, ExecutionReadSnapshotError> {
        let key = if query.is_empty() {
            path.to_owned()
        } else {
            format!("{path}?{query}")
        };
        self.responses.get(&key).cloned().unwrap_or_else(|| {
            Err(ExecutionReadSnapshotError::Unavailable(
                "fixture response missing".to_owned(),
            ))
        })
    }
}

#[derive(Debug)]
struct FailingExecutionReadPort;

impl ExecutionReadSnapshotPort for FailingExecutionReadPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, ExecutionReadSnapshotError> {
        Err(ExecutionReadSnapshotError::Unavailable(
            "Go execution owner unavailable".to_owned(),
        ))
    }
}

fn execution_read_fixture() -> ExecutionReadFixture {
    let fixture: ExecutionReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/execution-read.json"
    ))
    .expect("execution read fixture");
    assert_eq!(fixture.version, "stage9.execution-read.v1");
    fixture
}

#[tokio::test]
async fn execution_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = execution_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_execution_read_snapshot_port(Arc::new(FixtureExecutionReadPort::from_fixture(
                &fixture,
            )));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 51);
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
async fn execution_read_routes_fail_closed_without_snapshot_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_execution_read_snapshot_port(Arc::new(FailingExecutionReadPort));
    let handle = start_product(config).await.expect("start product");
    for path in [
        "/api/v1/execution/orders",
        "/api/v1/execution/orders/order-1",
        "/api/v1/execution/orders/order-1/events",
    ] {
        let response = request_json(handle.startup_record().address, "GET", path, None).await;
        assert_eq!(response["ok"], false, "path {path}");
        assert_eq!(
            response["error"]["code"], "EXECUTION_UNAVAILABLE",
            "path {path}"
        );
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn execution_read_routes_are_not_registered_without_snapshot_port() {
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
        "/api/v1/execution/orders",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}
