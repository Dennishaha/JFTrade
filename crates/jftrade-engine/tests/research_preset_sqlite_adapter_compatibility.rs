#[path = "../src/product_research_preset_write_port.rs"]
mod product_research_preset_write_port;

use jftrade_store_sqlite::RESEARCH_PRESET_TEST_CUTOVER_PROFILE;
use product_research_preset_write_port::{
    ResearchPresetSqliteTestCutoverPort, ResearchPresetWriteMutation, ResearchPresetWritePort,
    ResearchPresetWritePortError,
};
use rusqlite::Connection;
use serde_json::json;

fn seed_schema(path: &std::path::Path) {
    let connection = Connection::open(path).expect("open fixture database");
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
        .expect("seed schema");
}

fn definition() -> serde_json::Value {
    json!({
        "brokerId": "futu",
        "market": "US",
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
fn explicit_test_cutover_adapter_persists_normalized_mutations() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("research.db");
    seed_schema(&path);
    let port = ResearchPresetSqliteTestCutoverPort::open(&path).expect("open test-cutover port");

    let created = port
        .mutate(&ResearchPresetWriteMutation::Create {
            payload: json!({"name": " Value ", "definition": definition()}),
        })
        .expect("create preset");
    assert_eq!(created["name"], "Value");
    let preset_id = created["presetId"].as_str().expect("preset id").to_owned();
    assert!(preset_id.starts_with("rsp_"));
    assert_eq!(created["revision"], 1);

    let updated = port
        .mutate(&ResearchPresetWriteMutation::Update {
            preset_id: preset_id.clone(),
            payload: json!({"name": " Updated ", "expectedRevision": 1}),
        })
        .expect("update preset");
    assert_eq!(updated["name"], "Updated");
    assert_eq!(updated["revision"], 2);

    let stale = port.mutate(&ResearchPresetWriteMutation::Update {
        preset_id: preset_id.clone(),
        payload: json!({"name": "Stale", "expectedRevision": 1}),
    });
    assert!(matches!(
        stale,
        Err(ResearchPresetWritePortError::Conflict(_))
    ));

    let invalid = port.mutate(&ResearchPresetWriteMutation::Create {
        payload: json!({"name": "Bad", "definition": {"market": "US"}}),
    });
    assert!(matches!(
        invalid,
        Err(ResearchPresetWritePortError::Invalid(message)) if message.contains("querySchemaVersion")
    ));

    let deleted = port
        .mutate(&ResearchPresetWriteMutation::Delete { preset_id })
        .expect("delete preset");
    assert_eq!(deleted, json!({"deleted": true}));
    assert_eq!(RESEARCH_PRESET_TEST_CUTOVER_PROFILE, "cutover-test-only.v1");
}
