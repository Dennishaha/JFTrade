use std::path::Path;
use std::sync::Arc;

use jftrade_store_sqlite::{
    STRATEGY_DEFINITION_PRODUCTION_PROFILE, StoredStrategyDefinition, StrategyDefinitionStore,
    StrategyDefinitionStoreError, StrategyRuntimeStore,
};
use rusqlite::Connection;
use serde_json::json;

const CREATED: &str = "2026-07-25T08:00:00Z";
const DELETED: &str = "2026-07-25T09:00:00Z";

fn open(path: &Path) -> Arc<StrategyDefinitionStore> {
    Arc::new(
        StrategyDefinitionStore::open_existing(path, STRATEGY_DEFINITION_PRODUCTION_PROFILE)
            .expect("production writer"),
    )
}

fn initialize(path: &Path) {
    let connection = Connection::open(path).expect("create database");
    jftrade_store_sqlite::initialize_current(&connection, "strategy").expect("strategy schema");
}

fn definition(id: &str) -> StoredStrategyDefinition {
    StoredStrategyDefinition {
        id: id.to_owned(),
        name: id.to_owned(),
        version: "0.1.0".to_owned(),
        description: String::new(),
        runtime: "pine-pinets".to_owned(),
        source_format: "pine-v6".to_owned(),
        symbol: "US.AAPL".to_owned(),
        interval: "1m".to_owned(),
        script: "//@version=6\nstrategy(\"guard\")".to_owned(),
        visual_model_json: "{}".to_owned(),
        created_at: CREATED.to_owned(),
        updated_at: CREATED.to_owned(),
        deleted_at: None,
    }
}

#[test]
fn definition_delete_guard_tracks_metadata_and_binding_links_until_soft_delete() {
    let directory = tempfile::tempdir().expect("directory");
    let path = directory.path().join("strategy.db");
    initialize(&path);
    let store = open(&path);
    for id in ["guarded", "sibling"] {
        store
            .save_definition(definition(id), CREATED)
            .expect("definition");
    }
    let runtime = StrategyRuntimeStore::from_definition_store(&store);
    for (instance_id, definition_id) in [("native", "guarded"), ("sibling-instance", "sibling")] {
        runtime
            .seed_instance_with_definition(
                instance_id,
                "STOPPED",
                json!({}),
                definition_id,
                definition_id,
                "0.1.0",
                CREATED,
            )
            .expect("native link");
    }
    runtime
        .seed_instance_with_binding("bound", "STOPPED", json!({"strategyId":"guarded"}), CREATED)
        .expect("binding link");
    assert!(matches!(store.delete_definition("guarded", DELETED),
        Err(StrategyDefinitionStoreError::DeleteGuard(message)) if message.contains("2 active instances")));
    runtime
        .delete_instance("native", DELETED)
        .expect("soft delete native");
    assert!(matches!(store.delete_definition("guarded", DELETED),
        Err(StrategyDefinitionStoreError::DeleteGuard(message)) if message.contains("1 active instances")));
    assert!(
        store
            .get_definition("guarded", false)
            .expect("retained definition")
            .is_some()
    );
    runtime
        .delete_instance("bound", DELETED)
        .expect("soft delete bound");
    let deleted = store
        .delete_definition("guarded", DELETED)
        .expect("guard released");
    assert_eq!(deleted.deleted_at.as_deref(), Some(DELETED));
    assert!(matches!(
        store.delete_definition("sibling", DELETED),
        Err(StrategyDefinitionStoreError::DeleteGuard(_))
    ));
}

#[test]
fn definition_delete_rejects_malformed_catalog_payload_without_mutation() {
    let directory = tempfile::tempdir().expect("directory");
    let path = directory.path().join("strategy.db");
    initialize(&path);
    let store = open(&path);
    store
        .save_definition(definition("guarded"), CREATED)
        .expect("definition");
    drop(store);
    // Corrupt the fixture only after releasing the production writer lease.
    let connection = Connection::open(&path).expect("fixture writer");
    connection.execute(
        "INSERT INTO strategy_catalog_operations (operation_id, plugin_id, status, updated_at, payload_json)
         VALUES ('corrupt', '', 'STOPPED', ?1, '{invalid')", [CREATED],
    ).expect("corrupt fixture row");
    drop(connection);
    let store = open(&path);
    assert!(matches!(
        store.delete_definition("guarded", DELETED),
        Err(StrategyDefinitionStoreError::Incompatible(_))
    ));
    let retained = store
        .get_definition("guarded", false)
        .expect("definition read")
        .expect("retained");
    assert_eq!(retained.updated_at, CREATED);
    assert_eq!(retained.deleted_at, None);
    assert_eq!(
        store
            .list_versions("guarded")
            .expect("version history")
            .len(),
        1
    );
}
