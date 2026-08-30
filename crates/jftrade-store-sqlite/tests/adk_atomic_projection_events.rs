use std::fs;
use std::path::Path;

use jftrade_store_sqlite::{
    AdkRunEvent, AdkStore, AdkStoreError, CreateAdkRunParams, StoredAdkRun, initialize_current,
};
use rusqlite::Connection;
use tempfile::tempdir;

fn initialize_database(path: &Path, component: &str) {
    let connection = Connection::open(path).expect("open sqlite");
    initialize_current(&connection, component).expect("initialize schema");
}

fn create_running_run(store: &AdkStore, id: &str, session_id: &str) -> StoredAdkRun {
    store
        .create_run(CreateAdkRunParams {
            id,
            session_id,
            agent_id: "agent",
            status: "RUNNING",
            client_request_id: &format!("request-{id}"),
            request_fingerprint: "fingerprint",
            payload_json: r#"{"streamEvents":[],"providerEvents":[]}"#,
        })
        .expect("create run")
}

#[test]
fn payload_and_session_event_commit_or_rollback_together() {
    let directory = tempdir().expect("temp dir");
    let run_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    initialize_database(&run_path, "adk");

    let store = AdkStore::open(&run_path).expect("open adk store");
    let run = create_running_run(&store, "run-atomic", "session-atomic");
    let lease = store
        .claim_run_lease(
            "run-atomic",
            "owner-atomic",
            std::time::Duration::from_secs(30),
        )
        .expect("claim lease");

    // An empty attached database makes the event insert fail.  The run
    // update must be rolled back rather than exposing a payload without its
    // corresponding session event.
    fs::write(&session_path, b"").expect("create invalid session db");
    let event = AdkRunEvent {
        id: "run-atomic:stream:1",
        session_id: "session-atomic",
        invocation_id: "run-atomic",
        author: "assistant.stream",
        content: "hello",
    };
    let result = store.update_run_payload_if_status_and_revision_with_events_with_lease(
        "run-atomic",
        "RUNNING",
        &run.updated_at,
        r#"{"streamEvents":[{"type":"timeline"}],"providerEvents":[]}"#,
        &session_path,
        &[event],
        lease.owner_id.as_str(),
        lease.fencing_token,
    );
    assert!(result.is_err(), "missing session schema must fail closed");

    let current = store
        .get_run("run-atomic")
        .expect("read run")
        .expect("run exists");
    assert_eq!(current.payload_json, run.payload_json);
    assert_eq!(current.updated_at, run.updated_at);
}

#[test]
fn missing_session_database_is_not_created_and_run_is_unchanged() {
    let directory = tempdir().expect("temp dir");
    let run_path = directory.path().join("adk.db");
    let session_path = directory.path().join("missing-adk-session.db");
    initialize_database(&run_path, "adk");

    let store = AdkStore::open(&run_path).expect("open adk store");
    let run = create_running_run(&store, "run-missing-session", "session-missing");
    let lease = store
        .claim_run_lease(
            &run.id,
            "owner-missing-session",
            std::time::Duration::from_secs(30),
        )
        .expect("claim lease");
    let event = AdkRunEvent {
        id: "run-missing-session:stream:1",
        session_id: &run.session_id,
        invocation_id: &run.id,
        author: "assistant.stream",
        content: "hello",
    };

    let result = store.update_run_payload_if_status_and_revision_with_events_with_lease(
        &run.id,
        &run.status,
        &run.updated_at,
        r#"{"streamEvents":[{"type":"timeline"}],"providerEvents":[]}"#,
        &session_path,
        &[event],
        &lease.owner_id,
        lease.fencing_token,
    );
    assert!(
        matches!(result, Err(AdkStoreError::NotRegularFile(_))),
        "missing session database must fail before ATTACH: {result:?}"
    );
    assert!(
        !session_path.exists(),
        "ATTACH must not create the database"
    );

    let current = store
        .get_run(&run.id)
        .expect("read run")
        .expect("run exists");
    assert_eq!(current.payload_json, run.payload_json);
    assert_eq!(current.updated_at, run.updated_at);
}

#[test]
fn duplicate_event_key_with_different_content_rolls_back_projection() {
    let directory = tempdir().expect("temp dir");
    let run_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    initialize_database(&run_path, "adk");
    initialize_database(&session_path, "adk-session");

    let store = AdkStore::open(&run_path).expect("open adk store");
    let initial_event = AdkRunEvent {
        id: "run-duplicate:user",
        session_id: "session-duplicate",
        invocation_id: "run-duplicate",
        author: "user",
        content: "original",
    };
    let run = store
        .create_run_with_event(
            CreateAdkRunParams {
                id: "run-duplicate",
                session_id: "session-duplicate",
                agent_id: "agent",
                status: "RUNNING",
                client_request_id: "request-duplicate",
                request_fingerprint: "fingerprint",
                payload_json: r#"{"streamEvents":[],"providerEvents":[]}"#,
            },
            &session_path,
            &initial_event,
        )
        .expect("create run with event");
    let lease = store
        .claim_run_lease(
            &run.id,
            "owner-duplicate",
            std::time::Duration::from_secs(30),
        )
        .expect("claim lease");
    let conflicting_event = AdkRunEvent {
        content: "different",
        ..initial_event
    };

    let result = store.update_run_payload_if_status_and_revision_with_events_with_lease(
        &run.id,
        &run.status,
        &run.updated_at,
        r#"{"streamEvents":[{"type":"timeline"}],"providerEvents":[]}"#,
        &session_path,
        &[conflicting_event],
        &lease.owner_id,
        lease.fencing_token,
    );
    assert!(
        matches!(result, Err(AdkStoreError::Conflict(_))),
        "mismatched replay must conflict: {result:?}"
    );

    let current = store
        .get_run(&run.id)
        .expect("read run")
        .expect("run exists");
    assert_eq!(current.payload_json, run.payload_json);
    assert_eq!(current.updated_at, run.updated_at);
    let event_content = Connection::open(&session_path)
        .expect("open session database")
        .query_row(
            "SELECT content FROM events WHERE id = ?1 AND session_id = ?2",
            (&initial_event.id, &initial_event.session_id),
            |row| row.get::<_, String>(0),
        )
        .expect("read persisted event");
    assert_eq!(event_content, "original");
}

#[test]
fn event_must_match_the_run_and_session_before_projection_changes() {
    let directory = tempdir().expect("temp dir");
    let run_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    initialize_database(&run_path, "adk");
    initialize_database(&session_path, "adk-session");

    let store = AdkStore::open(&run_path).expect("open adk store");
    let run = create_running_run(&store, "run-identity", "session-identity");
    let lease = store
        .claim_run_lease(
            &run.id,
            "owner-identity",
            std::time::Duration::from_secs(30),
        )
        .expect("claim lease");
    let cases = [
        AdkRunEvent {
            id: "run-identity:wrong-session",
            session_id: "another-session",
            invocation_id: &run.id,
            author: "assistant.stream",
            content: "hello",
        },
        AdkRunEvent {
            id: "run-identity:wrong-run",
            session_id: &run.session_id,
            invocation_id: "another-run",
            author: "assistant.stream",
            content: "hello",
        },
    ];

    for event in cases {
        let result = store.update_run_payload_if_status_and_revision_with_events_with_lease(
            &run.id,
            &run.status,
            &run.updated_at,
            r#"{"streamEvents":[{"type":"timeline"}],"providerEvents":[]}"#,
            &session_path,
            &[event],
            &lease.owner_id,
            lease.fencing_token,
        );
        assert!(
            matches!(result, Err(AdkStoreError::Conflict(_))),
            "foreign event identity must conflict: {result:?}"
        );
        let current = store
            .get_run(&run.id)
            .expect("read run")
            .expect("run exists");
        assert_eq!(current.payload_json, run.payload_json);
        assert_eq!(current.updated_at, run.updated_at);
    }
}
