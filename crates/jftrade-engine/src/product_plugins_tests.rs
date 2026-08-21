use std::collections::BTreeMap;

use serde::Deserialize;
use serde_json::Value;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct PluginsReadFixture {
    version: String,
    cases: Vec<PluginsReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct PluginsReadCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug)]
struct FixturePluginSnapshotPort {
    catalog: Value,
    operations: BTreeMap<String, Value>,
}

impl FixturePluginSnapshotPort {
    fn from_fixture(fixture: &PluginsReadFixture) -> Self {
        let catalog = fixture
            .cases
            .iter()
            .find(|case| case.name == "catalog")
            .and_then(|case| case.data.clone())
            .expect("plugins catalog fixture case");
        let operations = fixture
            .cases
            .iter()
            .filter_map(|case| {
                let value = case.data.clone()?;
                let operation_id = value.get("operationId")?.as_str()?.to_owned();
                Some((operation_id, value))
            })
            .collect();
        Self {
            catalog,
            operations,
        }
    }
}

impl PluginSnapshotPort for FixturePluginSnapshotPort {
    fn catalog(&self) -> Result<Value, PluginSnapshotError> {
        Ok(self.catalog.clone())
    }

    fn operation(&self, operation_id: &str) -> Result<Option<Value>, PluginSnapshotError> {
        Ok(self.operations.get(operation_id).cloned())
    }
}

#[derive(Debug)]
struct FailingPluginSnapshotPort;

impl PluginSnapshotPort for FailingPluginSnapshotPort {
    fn catalog(&self) -> Result<Value, PluginSnapshotError> {
        Err(PluginSnapshotError::Unavailable(
            "Go plugin catalog fixture unavailable".to_owned(),
        ))
    }

    fn operation(&self, _operation_id: &str) -> Result<Option<Value>, PluginSnapshotError> {
        Err(PluginSnapshotError::Unavailable(
            "Go plugin catalog fixture unavailable".to_owned(),
        ))
    }
}

fn plugins_read_fixture() -> PluginsReadFixture {
    let fixture: PluginsReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/plugins-read.json"
    ))
    .expect("plugins read fixture");
    assert_eq!(fixture.version, "stage9.plugins-read.v1");
    fixture
}

#[tokio::test]
async fn plugins_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = plugins_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_plugin_snapshot_port(Arc::new(FixturePluginSnapshotPort::from_fixture(&fixture)));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 50);
    let address = handle.startup_record().address;
    for case in &fixture.cases {
        assert_eq!(case.method, "GET", "case {}", case.name);
        let (status, response) =
            request_json_with_status(address, &case.method, &case.request_path, None, &[]).await;
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
async fn plugins_read_routes_fail_closed_when_snapshot_port_is_unavailable() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_plugin_snapshot_port(Arc::new(FailingPluginSnapshotPort));
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    for path in ["/api/v1/plugins", "/api/v1/plugins/operations/op-alpha"] {
        let response = request_json(address, "GET", path, None).await;
        assert_eq!(response["ok"], false, "path {path}");
        assert_eq!(
            response["error"]["code"], "PLUGINS_UNAVAILABLE",
            "path {path}"
        );
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn plugins_read_routes_are_not_registered_without_snapshot_port() {
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
        "/api/v1/plugins",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}
