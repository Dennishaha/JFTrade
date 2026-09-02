use std::fs;
use std::path::Path;

#[cfg(unix)]
use std::os::unix::fs::symlink;

use jftrade_store_sqlite::{
    ADK_SESSION_TEST_CUTOVER_PROFILE, AdkSessionStoreError, AdkSessionTestCutoverStore,
};
use rusqlite::Connection;
use tempfile::tempdir;

fn seed_valid_go_adk_session_database(path: &Path) {
    let connection = Connection::open(path).expect("open sqlite");
    connection
        .execute_batch(
            "CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
            VALUES ('adk-session', 4, '2026-08-20T00:00:00Z');

            CREATE TABLE sessions (
                app_name TEXT,
                user_id TEXT,
                id TEXT,
                state TEXT,
                create_time TIMESTAMP,
                update_time TIMESTAMP,
                PRIMARY KEY (app_name, user_id, id)
            );
            CREATE TABLE events (
                id TEXT,
                app_name TEXT,
                user_id TEXT,
                session_id TEXT,
                invocation_id TEXT,
                author TEXT,
                actions BLOB,
                long_running_tool_ids_json TEXT,
                routes_json TEXT,
                output_json TEXT,
                node_info_json TEXT,
                requested_input_json TEXT,
                branch TEXT,
                isolation_scope TEXT,
                timestamp TIMESTAMP,
                content TEXT,
                grounding_metadata TEXT,
                custom_metadata TEXT,
                usage_metadata TEXT,
                citation_metadata TEXT,
                partial NUMERIC,
                turn_complete NUMERIC,
                error_code TEXT,
                error_message TEXT,
                interrupted NUMERIC,
                PRIMARY KEY (id, app_name, user_id, session_id),
                FOREIGN KEY (app_name, user_id, session_id) REFERENCES sessions(app_name, user_id, id) ON DELETE CASCADE
            );
            CREATE TABLE app_states (app_name TEXT PRIMARY KEY, state TEXT, update_time TIMESTAMP);
            CREATE TABLE user_states (app_name TEXT, user_id TEXT, state TEXT, update_time TIMESTAMP, PRIMARY KEY (app_name, user_id));",
        )
        .expect("seed schema");
}

#[test]
fn adk_session_store_rejects_missing_drifted_and_corrupted_go_databases() {
    let directory = tempdir().expect("temp dir");
    let missing_path = directory.path().join("missing.db");

    let err =
        AdkSessionTestCutoverStore::open_existing(&missing_path, ADK_SESSION_TEST_CUTOVER_PROFILE)
            .expect_err("missing DB must fail closed");
    assert!(matches!(err, AdkSessionStoreError::NotRegularFile(_)));

    let empty_path = directory.path().join("empty.db");
    fs::write(&empty_path, b"").expect("write empty");
    let err =
        AdkSessionTestCutoverStore::open_existing(&empty_path, ADK_SESSION_TEST_CUTOVER_PROFILE)
            .expect_err("empty DB must fail closed");
    assert!(matches!(err, AdkSessionStoreError::Schema(_)));

    let drifted_path = directory.path().join("drifted.db");
    seed_valid_go_adk_session_database(&drifted_path);
    let connection = Connection::open(&drifted_path).expect("open sqlite");
    connection
        .execute_batch("CREATE TABLE rogue_table (id TEXT PRIMARY KEY);")
        .expect("create rogue");
    drop(connection);

    let err =
        AdkSessionTestCutoverStore::open_existing(&drifted_path, ADK_SESSION_TEST_CUTOVER_PROFILE)
            .expect_err("drifted DB must fail closed");
    assert!(matches!(err, AdkSessionStoreError::Schema(_)));
}

#[cfg(unix)]
#[test]
fn adk_session_store_rejects_symlink_aliases() {
    let directory = tempdir().expect("temp dir");
    let target = directory.path().join("canonical.db");
    let alias = directory.path().join("alias.db");
    seed_valid_go_adk_session_database(&target);
    symlink(&target, &alias).expect("create database symlink");

    let err = AdkSessionTestCutoverStore::open_existing(&alias, ADK_SESSION_TEST_CUTOVER_PROFILE)
        .expect_err("symlink aliases must fail closed");
    assert!(matches!(err, AdkSessionStoreError::NotRegularFile(_)));
}

#[test]
fn adk_session_store_lifecycle_and_restart_durability() {
    let directory = tempdir().expect("temp dir");
    let db_path = directory.path().join("adk-session.db");
    seed_valid_go_adk_session_database(&db_path);

    let store =
        AdkSessionTestCutoverStore::open_existing(&db_path, ADK_SESSION_TEST_CUTOVER_PROFILE)
            .expect("open valid store");

    let session = store
        .upsert_session("jftrade", "user-1", "sess-100", r#"{"mode":"auto"}"#)
        .expect("upsert session");
    assert_eq!(session.id, "sess-100");

    let event = store
        .record_event(jftrade_store_sqlite::RecordAdkEventParams {
            id: "evt-1",
            app_name: "jftrade",
            user_id: "user-1",
            session_id: "sess-100",
            invocation_id: "inv-1",
            author: "user",
            content: "Hello Assistant",
        })
        .expect("record event");
    assert_eq!(event.id, "evt-1");

    store
        .upsert_app_state("jftrade", r#"{"activeAgents":["analyst"]}"#)
        .expect("upsert app state");

    store
        .upsert_user_state("jftrade", "user-1", r#"{"theme":"dark"}"#)
        .expect("upsert user state");

    // Second owner rejection
    let err = AdkSessionTestCutoverStore::open_existing(&db_path, ADK_SESSION_TEST_CUTOVER_PROFILE)
        .expect_err("second writer lease must fail closed");
    assert!(matches!(err, AdkSessionStoreError::WriterLease(_)));

    drop(store);

    // Restart durability
    let store2 =
        AdkSessionTestCutoverStore::open_existing(&db_path, ADK_SESSION_TEST_CUTOVER_PROFILE)
            .expect("reopen store");
    let session2 = store2
        .upsert_session("jftrade", "user-1", "sess-100", r#"{"mode":"manual"}"#)
        .expect("update session");
    assert_eq!(session2.state, r#"{"mode":"manual"}"#);
}
