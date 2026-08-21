use std::collections::BTreeMap;

use serde::Deserialize;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct StrategyDefinitionFixture {
    version: String,
    cases: Vec<StrategyDefinitionFixtureCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct StrategyDefinitionFixtureCase {
    name: String,
    method: String,
    path: String,
    expected_status: u16,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug)]
struct FixtureStrategyDefinitionSnapshotPort {
    list: Vec<Value>,
    details: BTreeMap<String, Value>,
    query_details: BTreeMap<String, Value>,
    versions: BTreeMap<String, Vec<Value>>,
    snapshots: BTreeMap<(String, String), Value>,
}

impl FixtureStrategyDefinitionSnapshotPort {
    fn from_fixture(fixture: &StrategyDefinitionFixture) -> Self {
        let case = |name: &str| {
            fixture
                .cases
                .iter()
                .find(|case| case.name == name)
                .and_then(|case| case.data.clone())
                .unwrap_or_else(|| panic!("missing strategy definition fixture case {name}"))
        };
        let mut details = BTreeMap::new();
        details.insert(
            "fixture-current".to_owned(),
            case("detail-current-default-preview"),
        );
        let mut query_details = BTreeMap::new();
        query_details.insert(
            "fixture-current".to_owned(),
            case("detail-current-query-preview"),
        );
        let mut versions = BTreeMap::new();
        for name in ["versions-current", "versions-soft-deleted"] {
            let value = case(name);
            let definition_id = value
                .as_array()
                .and_then(|items| items.first())
                .and_then(|item| item.get("definitionId"))
                .and_then(Value::as_str)
                .unwrap_or_else(|| panic!("fixture case {name} has no definition id"));
            versions.insert(
                definition_id.to_owned(),
                value
                    .as_array()
                    .cloned()
                    .unwrap_or_else(|| panic!("fixture case {name} is not an array")),
            );
        }
        let snapshot = case("version-current-history");
        let definition_id = snapshot["definitionId"]
            .as_str()
            .expect("historical version definition id")
            .to_owned();
        let version = snapshot["version"]
            .as_str()
            .expect("historical version")
            .to_owned();
        let mut snapshots = BTreeMap::new();
        snapshots.insert((definition_id, version), snapshot);
        Self {
            list: case("list-current-only")
                .as_array()
                .cloned()
                .expect("strategy definition list fixture"),
            details,
            query_details,
            versions,
            snapshots,
        }
    }
}

impl StrategyDefinitionSnapshotPort for FixtureStrategyDefinitionSnapshotPort {
    fn list(&self) -> Result<Vec<Value>, StrategyDefinitionSnapshotError> {
        Ok(self.list.clone())
    }

    fn get(
        &self,
        definition_id: &str,
        preview: &StrategyDefinitionPreview,
    ) -> Result<Option<Value>, StrategyDefinitionSnapshotError> {
        let source =
            if preview.interval.is_some() || preview.symbol.is_some() || preview.use_extended_hours
            {
                self.query_details.get(definition_id)
            } else {
                self.details.get(definition_id)
            };
        Ok(source.cloned())
    }

    fn versions(
        &self,
        definition_id: &str,
    ) -> Result<Option<Vec<Value>>, StrategyDefinitionSnapshotError> {
        Ok(self.versions.get(definition_id).cloned())
    }

    fn version(
        &self,
        definition_id: &str,
        version: &str,
    ) -> Result<Option<Value>, StrategyDefinitionSnapshotError> {
        Ok(self
            .snapshots
            .get(&(definition_id.to_owned(), version.to_owned()))
            .cloned())
    }
}

fn strategy_definition_fixture() -> StrategyDefinitionFixture {
    let fixture: StrategyDefinitionFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/rust-migration/stage9/strategy-definitions.json"
    ))
    .expect("strategy definition fixture");
    assert_eq!(fixture.version, "stage9.strategy-definitions.v1");
    fixture
}

#[tokio::test]
async fn strategy_definition_routes_match_group_fixture_in_cutover_only() {
    let fixture = strategy_definition_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_strategy_definition_snapshot_port(Arc::new(
                FixtureStrategyDefinitionSnapshotPort::from_fixture(&fixture),
            ));
    let handle = start_product(config).await.expect("start product");
    assert_eq!(handle.startup_record().owned_routes, 52);
    let address = handle.startup_record().address;
    for case in &fixture.cases {
        assert_eq!(case.method, "GET", "case {}", case.name);
        let (status, response) =
            request_json_with_status(address, &case.method, &case.path, None, &[]).await;
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
async fn strategy_definition_routes_fail_closed_without_snapshot_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config");
    let handle = start_product(config).await.expect("start product");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "GET",
        "/api/v1/strategy-definitions",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["ok"], false);
    assert_eq!(
        response["error"]["code"],
        "NOT_FOUND"
    );
    handle.shutdown().await.expect("shutdown product");
}
