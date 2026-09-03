use std::collections::BTreeMap;
use std::sync::Arc;

use serde::Deserialize;
use serde_json::Value;

use super::*;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ResearchPresetReadFixture {
    version: String,
    cases: Vec<ResearchPresetReadCase>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ResearchPresetReadCase {
    name: String,
    method: String,
    request_path: String,
    expected_status: u16,
    data: Option<Value>,
    error_code: Option<String>,
    error_message: Option<String>,
}

#[derive(Debug)]
struct FixtureResearchPresetReadPort {
    responses: BTreeMap<String, Result<Value, ResearchPresetReadSnapshotError>>,
}

impl FixtureResearchPresetReadPort {
    fn from_fixture(fixture: &ResearchPresetReadFixture) -> Self {
        let mut responses = BTreeMap::new();
        for case in &fixture.cases {
            let response = match (&case.data, &case.error_code) {
                (Some(data), _) => Ok(data.clone()),
                (None, Some(code)) if code == "RESEARCH_PRESET_NOT_FOUND" => {
                    Err(ResearchPresetReadSnapshotError::NotFound)
                }
                _ => Err(ResearchPresetReadSnapshotError::Unavailable(
                    "fixture response missing".to_owned(),
                )),
            };
            responses.insert(case.request_path.clone(), response);
        }
        Self { responses }
    }
}

impl ResearchPresetReadSnapshotPort for FixtureResearchPresetReadPort {
    fn read(&self, path: &str, query: &str) -> Result<Value, ResearchPresetReadSnapshotError> {
        let key = if query.is_empty() {
            path.to_owned()
        } else {
            format!("{path}?{query}")
        };
        self.responses.get(&key).cloned().unwrap_or_else(|| {
            Err(ResearchPresetReadSnapshotError::Unavailable(
                "fixture response missing".to_owned(),
            ))
        })
    }
}

#[derive(Debug)]
struct FailingResearchPresetReadPort;

impl ResearchPresetReadSnapshotPort for FailingResearchPresetReadPort {
    fn read(&self, _path: &str, _query: &str) -> Result<Value, ResearchPresetReadSnapshotError> {
        Err(ResearchPresetReadSnapshotError::Unavailable(
            "Go research preset store unavailable".to_owned(),
        ))
    }
}

fn research_preset_read_fixture() -> ResearchPresetReadFixture {
    let fixture: ResearchPresetReadFixture = serde_json::from_str(include_str!(
        "../../../tests/fixtures/compatibility/api-transport/research-preset-read.json"
    ))
    .expect("research preset read fixture");
    assert_eq!(fixture.version, "stage9.research-preset-read.v1");
    fixture
}

#[tokio::test]
async fn research_preset_read_routes_match_group_fixture_in_cutover_only() {
    let fixture = research_preset_read_fixture();
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_research_preset_read_snapshot_port(Arc::new(
                FixtureResearchPresetReadPort::from_fixture(&fixture),
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
async fn research_preset_read_routes_fail_closed_and_keep_mutations_unregistered() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_research_preset_read_snapshot_port(Arc::new(FailingResearchPresetReadPort));
    let handle = start_product(config).await.expect("start product");
    let response = request_json(
        handle.startup_record().address,
        "GET",
        "/api/v1/research/screens/presets",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "RESEARCH_PRESET_UNAVAILABLE");
    for (method, path) in [
        ("POST", "/api/v1/research/screens/presets"),
        ("PATCH", "/api/v1/research/screens/presets/preset-value"),
        ("DELETE", "/api/v1/research/screens/presets/preset-value"),
    ] {
        let response = request_json(handle.startup_record().address, method, path, None).await;
        assert_eq!(response["ok"], false, "{method} {path}");
        assert_eq!(response["error"]["code"], "NOT_FOUND", "{method} {path}");
    }
    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn research_preset_read_routes_are_not_registered_without_snapshot_port() {
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
        "/api/v1/research/screens/presets",
        None,
    )
    .await;
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown product");
}
