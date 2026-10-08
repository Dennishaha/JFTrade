use std::fs;

use jftrade_store_sqlite::{AdkSessionStore, AdkSessionStoreError};
use rusqlite::{Connection, OpenFlags};
use tempfile::tempdir;

// Parity: go:452dea11:internal/assistant/engine/persistence/session_sqlite_schema_test.go:82 TestSQLiteSessionServiceRejectsV1SchemaWithoutMutatingEvents
#[test]
fn production_session_store_rejects_v1_without_mutating_schema_or_events() {
    let directory = tempdir().expect("temporary directory");
    let path = directory.path().join("adk-session-v1.db");
    let connection = Connection::open(&path).expect("create V1 database");
    connection
        .execute_batch(
            "CREATE TABLE sessions (app_name TEXT, user_id TEXT, id TEXT, state TEXT, create_time TIMESTAMP, update_time TIMESTAMP, PRIMARY KEY (app_name,user_id,id));
             CREATE TABLE events (id TEXT, app_name TEXT, user_id TEXT, session_id TEXT, invocation_id TEXT, author TEXT, actions BLOB, long_running_tool_ids_json TEXT, branch TEXT, timestamp TIMESTAMP, content TEXT, grounding_metadata TEXT, custom_metadata TEXT, usage_metadata TEXT, citation_metadata TEXT, partial NUMERIC, turn_complete NUMERIC, error_code TEXT, error_message TEXT, interrupted NUMERIC, PRIMARY KEY (id,app_name,user_id,session_id), FOREIGN KEY (app_name,user_id,session_id) REFERENCES sessions(app_name,user_id,id) ON DELETE CASCADE);
             CREATE TABLE app_states (app_name TEXT PRIMARY KEY, state TEXT, update_time TIMESTAMP);
             CREATE TABLE user_states (app_name TEXT, user_id TEXT, state TEXT, update_time TIMESTAMP, PRIMARY KEY (app_name,user_id));
             CREATE TABLE jftrade_schema_meta (component_id TEXT PRIMARY KEY, version INTEGER NOT NULL, created_at TEXT NOT NULL);
             INSERT INTO jftrade_schema_meta VALUES ('adk-session', 1, 'now');
             INSERT INTO sessions (app_name,user_id,id,state) VALUES ('app','user','session','{}');
             INSERT INTO events (id,app_name,user_id,session_id,invocation_id,author,branch,content) VALUES ('event','app','user','session','invocation','agent','branch','{}');",
        )
        .expect("seed original V1 schema and event");
    drop(connection);
    let original = fs::read(&path).expect("original database bytes");

    for _ in 0..2 {
        let error = AdkSessionStore::open(&path).expect_err("V1 must fail closed");
        match error {
            AdkSessionStoreError::Schema(error) => {
                assert!(error.is_incompatible(), "{error}");
                assert!(error.to_string().contains("adk-session"));
            }
            other => panic!("expected incompatible schema, got {other:?}"),
        }
        assert_eq!(fs::read(&path).expect("rejected database bytes"), original);
    }

    let connection = Connection::open_with_flags(&path, OpenFlags::SQLITE_OPEN_READ_ONLY)
        .expect("reopen without writing");
    let version: i64 = connection
        .query_row(
            "SELECT version FROM jftrade_schema_meta WHERE component_id='adk-session'",
            [],
            |row| row.get(0),
        )
        .expect("preserved schema version");
    assert_eq!(version, 1);
    let count: i64 = connection
        .query_row("SELECT COUNT(*) FROM events", [], |row| row.get(0))
        .expect("preserved event count");
    assert_eq!(count, 1);
    let event: (String, String, String, String) = connection
        .query_row(
            "SELECT id,session_id,branch,content FROM events",
            [],
            |row| Ok((row.get(0)?, row.get(1)?, row.get(2)?, row.get(3)?)),
        )
        .expect("preserved original event");
    assert_eq!(
        event,
        (
            "event".into(),
            "session".into(),
            "branch".into(),
            "{}".into()
        )
    );
}
