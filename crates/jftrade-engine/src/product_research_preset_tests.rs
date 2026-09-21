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

use crate::product::product_production_ports::ProductionResearchPresetPort;
use crate::product::product_research_preset_write_port::{
    ResearchPresetWriteMutation, ResearchPresetWritePort, ResearchPresetWritePortError,
};
use jftrade_store_sqlite::ResearchPresetStore;

fn seed_research_preset_schema(path: &std::path::Path) {
    let connection = rusqlite::Connection::open(path).expect("open fixture database");
    connection
        .execute_batch(
            "CREATE TABLE research_screen_presets (
                preset_id TEXT PRIMARY KEY,
                name TEXT NOT NULL,
                name_key TEXT NOT NULL UNIQUE,
                query_schema_version INTEGER NOT NULL,
                query_json TEXT NOT NULL,
                revision INTEGER NOT NULL DEFAULT 1,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            );
            CREATE INDEX research_screen_presets_updated_at
                ON research_screen_presets(updated_at DESC, preset_id);
            CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
                VALUES ('research', 1, '2026-08-24T04:00:00Z');",
        )
        .expect("seed research schema");
}

fn production_preset_port(path: &std::path::Path) -> ProductionResearchPresetPort {
    ProductionResearchPresetPort {
        store: Arc::new(ResearchPresetStore::open(path).expect("open research preset store")),
    }
}

fn preset_definition(market: &str) -> Value {
    serde_json::json!({
        "brokerId": "futu",
        "market": market,
        "pool": {},
        "columns": [{
            "columnId": "price",
            "factor": {"instanceId": "price", "factorKey": "simple.price", "params": {}}
        }],
        "catalogVersion": "futu-stock-screen-v1",
        "querySchemaVersion": 2
    })
}

#[test]
// Parity: go:452dea11:internal/research/presets_test.go:85 TestServiceCreateListGetAndDeletePreset
fn preset_ports_round_trip_create_list_get_and_delete_with_trimmed_ids() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("research.db");
    seed_research_preset_schema(&path);
    let port = production_preset_port(&path);

    let created = port
        .mutate(&ResearchPresetWriteMutation::Create {
            payload: serde_json::json!({
                "name": "  美股价值  ",
                "definition": preset_definition("US"),
            }),
        })
        .expect("create preset");
    assert_eq!(created["name"], "美股价值");
    assert_eq!(created["querySchemaVersion"], 2);
    assert_eq!(created["revision"], 1);
    let preset_id = created["presetId"].as_str().expect("preset id").to_owned();

    let listed = port
        .read("/api/v1/research/screens/presets", "")
        .expect("list presets");
    assert_eq!(listed["presets"][0]["presetId"], preset_id);

    let fetched = port
        .read(
            &format!("/api/v1/research/screens/presets/%20{preset_id}%20"),
            "",
        )
        .expect("get preset by percent-encoded padded id");
    assert_eq!(fetched["presetId"], preset_id);

    port.mutate(&ResearchPresetWriteMutation::Delete {
        preset_id: format!(" {preset_id} "),
    })
    .expect("delete preset");
    assert!(matches!(
        port.read(&format!("/api/v1/research/screens/presets/{preset_id}"), ""),
        Err(ResearchPresetReadSnapshotError::NotFound)
    ));
}

#[test]
// Parity: go:452dea11:internal/research/presets_test.go:111 TestServiceUpdatePresetMergesFieldsAndEnforcesRevision
fn definition_only_updates_keep_the_stored_name_and_enforce_revision() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("research.db");
    seed_research_preset_schema(&path);
    let port = production_preset_port(&path);

    let created = port
        .mutate(&ResearchPresetWriteMutation::Create {
            payload: serde_json::json!({
                "name": "旧名称",
                "definition": preset_definition("US"),
            }),
        })
        .expect("create preset");
    let preset_id = created["presetId"].as_str().expect("preset id").to_owned();

    let definition_only = port
        .mutate(&ResearchPresetWriteMutation::Update {
            preset_id: preset_id.clone(),
            payload: serde_json::json!({
                "definition": preset_definition("HK"),
                "expectedRevision": 1,
            }),
        })
        .expect("definition-only update");
    assert_eq!(definition_only["name"], "旧名称");
    assert_eq!(definition_only["definition"]["market"], "HK");
    assert_eq!(definition_only["querySchemaVersion"], 2);
    assert_eq!(definition_only["revision"], 2);

    assert!(matches!(
        port.mutate(&ResearchPresetWriteMutation::Update {
            preset_id: preset_id.clone(),
            payload: serde_json::json!({"name": "Stale", "expectedRevision": 1}),
        }),
        Err(ResearchPresetWritePortError::Conflict(_))
    ));

    let merged = port
        .mutate(&ResearchPresetWriteMutation::Update {
            preset_id,
            payload: serde_json::json!({
                "name": "  新名称 ",
                "definition": preset_definition("SH"),
                "expectedRevision": 2,
            }),
        })
        .expect("merged update");
    assert_eq!(merged["name"], "新名称");
    assert_eq!(merged["definition"]["market"], "SH");
    assert_eq!(merged["revision"], 3);
}

#[test]
// Parity: go:452dea11:internal/research/presets_test.go:146 TestServiceRejectsUnavailableAndInvalidPresetOperations
fn invalid_preset_operations_fail_closed_before_the_store_is_touched() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("research.db");
    seed_research_preset_schema(&path);
    let port = production_preset_port(&path);
    let invalid_definition = {
        let mut definition = preset_definition("US");
        definition["querySchemaVersion"] = serde_json::json!(1);
        definition
    };

    for (mutation, needle) in [
        (
            ResearchPresetWriteMutation::Create {
                payload: serde_json::json!({"name": "   ", "definition": preset_definition("US")}),
            },
            "name is required",
        ),
        (
            ResearchPresetWriteMutation::Create {
                payload: serde_json::json!({
                    "name": "界".repeat(81),
                    "definition": preset_definition("US"),
                }),
            },
            "name must not exceed 80 characters",
        ),
        (
            ResearchPresetWriteMutation::Create {
                payload: serde_json::json!({"name": "invalid", "definition": invalid_definition}),
            },
            "querySchemaVersion",
        ),
        (
            ResearchPresetWriteMutation::Update {
                preset_id: "preset".to_owned(),
                payload: serde_json::json!({"expectedRevision": 1}),
            },
            "name or definition is required",
        ),
        (
            ResearchPresetWriteMutation::Update {
                preset_id: "preset".to_owned(),
                payload: serde_json::json!({"name": "name", "expectedRevision": 0}),
            },
            "expectedRevision must be positive",
        ),
        (
            ResearchPresetWriteMutation::Update {
                preset_id: "   ".to_owned(),
                payload: serde_json::json!({"name": "name", "expectedRevision": 1}),
            },
            "preset id is required",
        ),
        (
            ResearchPresetWriteMutation::Delete {
                preset_id: "   ".to_owned(),
            },
            "preset id is required",
        ),
    ] {
        match port.mutate(&mutation) {
            Err(ResearchPresetWritePortError::Invalid(message)) => assert!(
                message.contains(needle),
                "unexpected message for {needle:?}: {message}"
            ),
            other => panic!("expected invalid preset rejection for {needle:?}, got {other:?}"),
        }
    }

    let listed = port
        .read("/api/v1/research/screens/presets", "")
        .expect("list presets");
    assert!(listed["presets"].as_array().expect("presets").is_empty());
}

#[test]
// Parity: go:452dea11:internal/research/presets_test.go:202 TestServicePropagatesRepositoryFailures
fn store_failures_surface_as_unavailable_instead_of_silent_success() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("research.db");
    seed_research_preset_schema(&path);
    let port = production_preset_port(&path);
    let created = port
        .mutate(&ResearchPresetWriteMutation::Create {
            payload: serde_json::json!({
                "name": "Value",
                "definition": preset_definition("US"),
            }),
        })
        .expect("create preset");
    let preset_id = created["presetId"].as_str().expect("preset id").to_owned();

    rusqlite::Connection::open(&path)
        .expect("open corrupting connection")
        .execute_batch("DROP TABLE research_screen_presets;")
        .expect("drop preset table");

    assert!(matches!(
        port.read("/api/v1/research/screens/presets", ""),
        Err(ResearchPresetReadSnapshotError::Unavailable(message)) if !message.is_empty()
    ));
    assert!(matches!(
        port.read(&format!("/api/v1/research/screens/presets/{preset_id}"), ""),
        Err(ResearchPresetReadSnapshotError::Unavailable(message)) if !message.is_empty()
    ));
    assert!(matches!(
        port.mutate(&ResearchPresetWriteMutation::Create {
            payload: serde_json::json!({
                "name": "Value",
                "definition": preset_definition("US"),
            }),
        }),
        Err(ResearchPresetWritePortError::Unavailable)
    ));
    assert!(matches!(
        port.mutate(&ResearchPresetWriteMutation::Update {
            preset_id: preset_id.clone(),
            payload: serde_json::json!({"name": "Value", "expectedRevision": 1}),
        }),
        Err(ResearchPresetWritePortError::Unavailable)
    ));
    assert!(matches!(
        port.mutate(&ResearchPresetWriteMutation::Delete {
            preset_id: preset_id.clone(),
        }),
        Err(ResearchPresetWritePortError::Unavailable)
    ));
}
