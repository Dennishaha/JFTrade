use std::fs;
use std::path::Path;

use jftrade_store_sqlite::{
    ADK_ARTIFACT_TEST_CUTOVER_PROFILE, AdkArtifactStoreError, AdkArtifactTestCutoverStore,
};
use rusqlite::Connection;
use tempfile::tempdir;

fn seed_valid_go_adk_artifact_database(path: &Path) {
    let connection = Connection::open(path).expect("open sqlite");
    connection
        .execute_batch(
            "CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
            VALUES ('adk-artifact', 1, '2026-08-20T00:00:00Z');

            CREATE TABLE artifacts (
                app_name TEXT NOT NULL,
                user_id TEXT NOT NULL,
                session_id TEXT NOT NULL,
                file_name TEXT NOT NULL,
                version INTEGER NOT NULL,
                part_json TEXT NOT NULL,
                mime_type TEXT NOT NULL,
                custom_metadata_json TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                PRIMARY KEY (app_name, user_id, session_id, file_name, version)
            );",
        )
        .expect("seed schema");
}

#[test]
fn adk_artifact_store_rejects_missing_drifted_and_corrupted_go_databases() {
    let directory = tempdir().expect("temp dir");
    let missing_path = directory.path().join("missing.db");

    let err = AdkArtifactTestCutoverStore::open_existing(
        &missing_path,
        ADK_ARTIFACT_TEST_CUTOVER_PROFILE,
    )
    .expect_err("missing DB must fail closed");
    assert!(matches!(err, AdkArtifactStoreError::NotRegularFile(_)));

    let empty_path = directory.path().join("empty.db");
    fs::write(&empty_path, b"").expect("write empty");
    let err =
        AdkArtifactTestCutoverStore::open_existing(&empty_path, ADK_ARTIFACT_TEST_CUTOVER_PROFILE)
            .expect_err("empty DB must fail closed");
    assert!(matches!(err, AdkArtifactStoreError::Schema(_)));

    let drifted_path = directory.path().join("drifted.db");
    seed_valid_go_adk_artifact_database(&drifted_path);
    let connection = Connection::open(&drifted_path).expect("open sqlite");
    connection
        .execute_batch("CREATE TABLE rogue_table (id TEXT PRIMARY KEY);")
        .expect("create rogue");
    drop(connection);

    let err = AdkArtifactTestCutoverStore::open_existing(
        &drifted_path,
        ADK_ARTIFACT_TEST_CUTOVER_PROFILE,
    )
    .expect_err("drifted DB must fail closed");
    assert!(matches!(err, AdkArtifactStoreError::Schema(_)));
}

#[test]
fn adk_artifact_store_lifecycle_and_restart_durability() {
    let directory = tempdir().expect("temp dir");
    let db_path = directory.path().join("adk-artifact.db");
    seed_valid_go_adk_artifact_database(&db_path);

    let store =
        AdkArtifactTestCutoverStore::open_existing(&db_path, ADK_ARTIFACT_TEST_CUTOVER_PROFILE)
            .expect("open valid store");

    let artifact = store
        .put_artifact(jftrade_store_sqlite::PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "user-1",
            session_id: "sess-100",
            file_name: "report.md",
            version: 1,
            part_json: r##"{"text":"# Analysis Report\nMarket: HK"}"##,
            mime_type: "text/markdown",
            custom_metadata_json: Some(r#"{"author":"analyst"}"#),
        })
        .expect("put artifact");
    assert_eq!(artifact.file_name, "report.md");
    assert_eq!(artifact.version, 1);

    let retrieved = store
        .get_artifact("jftrade", "user-1", "sess-100", "report.md", 1)
        .expect("get artifact")
        .expect("found");
    assert_eq!(retrieved.mime_type, "text/markdown");

    // Second owner rejection
    let err =
        AdkArtifactTestCutoverStore::open_existing(&db_path, ADK_ARTIFACT_TEST_CUTOVER_PROFILE)
            .expect_err("second writer lease must fail closed");
    assert!(matches!(err, AdkArtifactStoreError::WriterLease(_)));

    drop(store);

    // Restart durability
    let store2 =
        AdkArtifactTestCutoverStore::open_existing(&db_path, ADK_ARTIFACT_TEST_CUTOVER_PROFILE)
            .expect("reopen store");
    let retrieved2 = store2
        .get_artifact("jftrade", "user-1", "sess-100", "report.md", 1)
        .expect("get artifact")
        .expect("found");
    assert_eq!(retrieved2.version, 1);
}
