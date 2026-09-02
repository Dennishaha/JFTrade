use std::path::Path;
use std::sync::{Arc, Barrier};

use jftrade_owner_lock::WriterLeaseError;
use jftrade_store_sqlite::{
    RESEARCH_PRESET_TEST_CUTOVER_PROFILE, ResearchPresetMutation, ResearchPresetStoreError,
    ResearchPresetTestCutoverStore,
};
use rusqlite::{Connection, params};
use serde_json::json;

const CREATED_AT: &str = "2026-08-24T04:00:00Z";
const UPDATED_AT: &str = "2026-08-24T04:01:00Z";

#[test]
fn preset_mutations_are_revision_fenced_and_survive_restart() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("research.db");
    seed_go_research_schema(&path);

    let store = open_store(&path);
    assert_eq!(store.path(), path);
    let conflict =
        ResearchPresetTestCutoverStore::open_existing(&path, RESEARCH_PRESET_TEST_CUTOVER_PROFILE)
            .expect_err("second writer must fail");
    assert!(matches!(
        conflict,
        ResearchPresetStoreError::WriterLease(WriterLeaseError::Held { .. })
    ));

    let created = preset("rsp-rust", "Value", "US", 1);
    let mut unnormalized = created.clone();
    unnormalized.name = " Value ".to_owned();
    assert!(matches!(
        store.insert(&unnormalized, CREATED_AT),
        Err(ResearchPresetStoreError::Incompatible(_))
    ));
    assert!(matches!(
        store.insert(&created, "not-a-timestamp"),
        Err(ResearchPresetStoreError::Incompatible(_))
    ));
    assert!(store.list().expect("list after rejected writes").is_empty());
    let stored = store.insert(&created, CREATED_AT).expect("insert preset");
    assert_eq!(stored.preset, created);
    assert_eq!(stored.created_at, CREATED_AT);

    let duplicate = preset("rsp-duplicate", "Value", "HK", 1);
    assert!(matches!(
        store.insert(&duplicate, CREATED_AT),
        Err(ResearchPresetStoreError::Conflict)
    ));
    assert_eq!(store.list().expect("list after conflict").len(), 1);

    let updated = preset("rsp-rust", "Hong Kong value", "HK", 2);
    let stored = store
        .replace_revision(&updated, 1, UPDATED_AT)
        .expect("replace revision");
    assert_eq!(stored.preset, updated);
    assert_eq!(stored.created_at, CREATED_AT);
    assert_eq!(stored.updated_at, UPDATED_AT);
    assert!(matches!(
        store.replace_revision(&updated, 1, UPDATED_AT),
        Err(ResearchPresetStoreError::Conflict)
    ));

    drop(store);
    let reopened = open_store(&path);
    let items = reopened.list().expect("list after restart");
    assert_eq!(items.len(), 1);
    assert_eq!(items[0].preset, updated);
    reopened.delete(" rsp-rust ").expect("delete preset");
    assert!(matches!(
        reopened.get("rsp-rust"),
        Err(ResearchPresetStoreError::NotFound)
    ));
    assert!(matches!(
        reopened.delete("rsp-rust"),
        Err(ResearchPresetStoreError::NotFound)
    ));
}

#[test]
fn concurrent_revision_updates_commit_exactly_once() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let path = directory.path().join("research.db");
    seed_go_research_schema(&path);
    let store = Arc::new(open_store(&path));
    let current = preset("rsp-race", "Original", "US", 1);
    store.insert(&current, CREATED_AT).expect("seed preset");

    let barrier = Arc::new(Barrier::new(3));
    let handles = ["First", "Second"].map(|name| {
        let store = Arc::clone(&store);
        let barrier = Arc::clone(&barrier);
        let update = preset("rsp-race", name, "US", 2);
        std::thread::spawn(move || {
            barrier.wait();
            store.replace_revision(&update, 1, UPDATED_AT)
        })
    });
    barrier.wait();
    let results = handles.map(|handle| handle.join().expect("join update"));
    assert_eq!(results.iter().filter(|result| result.is_ok()).count(), 1);
    assert_eq!(
        results
            .iter()
            .filter(|result| matches!(result, Err(ResearchPresetStoreError::Conflict)))
            .count(),
        1
    );
    let stored = store.get("rsp-race").expect("read winner");
    assert_eq!(stored.preset.revision, 2);
    assert!(stored.preset.name == "First" || stored.preset.name == "Second");
}

#[test]
fn store_rejects_missing_drifted_and_corrupted_go_databases() {
    let directory = tempfile::tempdir().expect("temporary directory");
    let missing = directory.path().join("missing.db");
    assert!(matches!(
        ResearchPresetTestCutoverStore::open_existing(&missing, "production"),
        Err(ResearchPresetStoreError::UnsupportedProfile(profile)) if profile == "production"
    ));
    assert!(matches!(
        ResearchPresetTestCutoverStore::open_existing(
            &missing,
            RESEARCH_PRESET_TEST_CUTOVER_PROFILE
        ),
        Err(ResearchPresetStoreError::NotRegularFile(_))
    ));
    assert!(
        !missing.exists(),
        "open_existing must not create a database"
    );

    let path = directory.path().join("research.db");
    seed_go_research_schema(&path);
    let connection = Connection::open(&path).expect("open fixture database");
    connection
        .execute_batch("CREATE TABLE rogue_table (id TEXT PRIMARY KEY)")
        .expect("drift schema");
    drop(connection);
    assert!(matches!(
        ResearchPresetTestCutoverStore::open_existing(
            &path,
            RESEARCH_PRESET_TEST_CUTOVER_PROFILE
        ),
        Err(ResearchPresetStoreError::Schema(error)) if error.is_incompatible()
    ));

    let corrupt = directory.path().join("corrupt.db");
    seed_go_research_schema(&corrupt);
    let connection = Connection::open(&corrupt).expect("open corrupt fixture");
    connection
        .execute(
            "INSERT INTO research_screen_presets \
             (preset_id, name, name_key, query_schema_version, query_json, revision, created_at, updated_at) \
             VALUES (?1, ?2, ?3, 2, ?4, 1, ?5, ?5)",
            params![" bad-id ", "Bad", "bad", "[]", "not-a-timestamp"],
        )
        .expect("insert corrupt row");
    drop(connection);
    let store = open_store(&corrupt);
    assert!(matches!(
        store.list(),
        Err(ResearchPresetStoreError::Incompatible(_))
    ));
}

fn definition(market: &str) -> serde_json::Value {
    json!({
        "brokerId": "futu",
        "market": market,
        "pool": {},
        "columns": [{
            "columnId": "price",
            "factor": {
                "instanceId": "price",
                "factorKey": "simple.price",
                "params": {}
            }
        }],
        "catalogVersion": "futu-stock-screen-v1",
        "querySchemaVersion": 2
    })
}

fn preset(id: &str, name: &str, market: &str, revision: u64) -> ResearchPresetMutation {
    ResearchPresetMutation {
        preset_id: id.to_owned(),
        name: name.to_owned(),
        query_schema_version: 2,
        definition: definition(market),
        revision,
    }
}

fn open_store(path: &Path) -> ResearchPresetTestCutoverStore {
    ResearchPresetTestCutoverStore::open_existing(path, RESEARCH_PRESET_TEST_CUTOVER_PROFILE)
        .expect("open research preset test-cutover store")
}

fn seed_go_research_schema(path: &Path) {
    let connection = Connection::open(path).expect("create research fixture");
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
        .expect("seed Go-compatible research schema");
}
