use std::collections::BTreeMap;
use std::sync::Arc;

use serde::Deserialize;
use serde_json::Value;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SystemReadFixture {
    version: String,
    cases: Vec<SystemReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct SystemReadCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    data: Value,
}

#[derive(Debug)]
struct FixtureSystemReadPort {
    responses: BTreeMap<String, Value>,
}

impl FixtureSystemReadPort {
    fn from_fixture(fixture: &SystemReadFixture) -> Self {
        Self {
            responses: fixture
                .cases
                .iter()
                .map(|case| (case.request_path.clone(), case.data.clone()))
                .collect(),
        }
    }
}

impl SystemReadSnapshotPort for FixtureSystemReadPort {
    fn read(&self, path: &str) -> Result<Value, SystemReadSnapshotError> {
        self.responses.get(path).cloned().ok_or_else(|| {
            SystemReadSnapshotError::Unavailable("fixture response missing".to_owned())
        })
    }
}

#[derive(Debug)]
struct FailingSystemReadPort;

impl SystemReadSnapshotPort for FailingSystemReadPort {
    fn read(&self, _path: &str) -> Result<Value, SystemReadSnapshotError> {
        Err(SystemReadSnapshotError::Unavailable(
            "Go system read snapshot unavailable".to_owned(),
        ))
    }
}

fn system_read_fixture() -> SystemReadFixture {
    let fixture: SystemReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/system-read.json"
    ))
    .expect("system read fixture");
    assert_eq!(fixture.version, "stage9.system-read.v1");
    fixture
}

#[tokio::test]
async fn system_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = system_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_system_read_snapshot_port(Arc::new(FixtureSystemReadPort::from_fixture(
                &fixture,
            )));
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
        assert_eq!(response["ok"], true, "case {}", case.name);
        assert_eq!(response["data"], case.data, "case {}", case.name);
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn system_read_routes_fail_closed_when_snapshot_port_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_system_read_snapshot_port(Arc::new(FailingSystemReadPort));
    let handle = start_product(config).await.expect("start product");
    for path in [
        "/api/v1/system/futu-opend",
        "/api/v1/system/worker/broker-order-updates",
    ] {
        let response = request_json(handle.startup_record().address, "GET", path, None).await;
        assert_eq!(response["ok"], false, "path {path}");
        assert_eq!(
            response["error"]["code"], "SYSTEM_READ_UNAVAILABLE",
            "path {path}"
        );
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn system_read_routes_are_not_registered_without_snapshot_port() {
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
        "/api/v1/system/futu-opend",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}
