use super::*;

fn seed_metadata_session(port: &ProductionAdkPort, id: &str) {
    port.store
        .upsert_agent(
            "projection-agent",
            r#"{"id":"projection-agent","name":"Projection","status":"ENABLED"}"#,
        )
        .expect("seed agent");
    port.store
        .upsert_session(id, "projection-agent", "{}")
        .expect("seed metadata session");
}

// Parity: go:452dea11:internal/assistant/engine/event_projection_boundaries_test.go:87 TestSessionProjectionTreatsMissingADKSessionAsEmpty
#[test]
fn metadata_session_without_durable_session_projects_empty_timeline_and_preserves_identity() {
    let (port, _directory) = unready_adk_port();
    let id = "projection-missing-session";
    seed_metadata_session(&port, id);
    assert!(
        port.session_store
            .get_session_by_id(id)
            .expect("read absent durable session")
            .is_none()
    );
    assert!(
        port.session_store
            .list_events(id)
            .expect("read empty transcript")
            .is_empty()
    );
    let result = port
        .read(&format!("/api/v1/adk/sessions/{id}"), "")
        .expect("empty projection succeeds");
    let AdkReadSnapshot::Json(result) = result else {
        panic!("expected JSON session projection")
    };
    assert_eq!(result["session"]["id"], id);
    assert_eq!(result["timeline"], json!([]));
    assert_eq!(result["runs"], json!([]));
    assert_eq!(result["artifacts"], json!([]));
    assert!(
        port.session_store
            .get_session_by_id(id)
            .expect("projection does not create durable session")
            .is_none()
    );
}

// Parity: go:452dea11:internal/assistant/engine/event_projection_boundaries_test.go:72 TestSessionProjectionPropagatesADKSessionReadFailures
#[test]
fn broken_durable_events_fail_session_projection_without_hiding_metadata_session() {
    let (port, directory) = unready_adk_port();
    let id = "projection-read-error";
    seed_metadata_session(&port, id);
    let connection = rusqlite::Connection::open(directory.path().join("adk-session.db"))
        .expect("open fixture database");
    connection
        .execute_batch("DROP TABLE events;")
        .expect("break event storage");
    drop(connection);
    assert!(matches!(
        port.session_store.list_events(id),
        Err(jftrade_store_sqlite::AdkSessionStoreError::Query(_))
    ));
    let failure = port
        .read(&format!("/api/v1/adk/sessions/{id}"), "")
        .expect_err("projection must propagate read failure");
    let AdkReadSnapshotError::Failed {
        status,
        code,
        message,
        ..
    } = failure
    else {
        panic!("expected classified projection failure")
    };
    assert_eq!(status, 500);
    assert_eq!(code, "ADK_MESSAGES_GET_FAILED");
    assert!(message.contains("no such table: events"), "{message}");
    assert!(
        port.store
            .get_session(id)
            .expect("metadata remains readable")
            .is_some()
    );
    assert!(
        port.session_store
            .get_session_by_id(id)
            .expect("absent durable session remains absent")
            .is_none()
    );
}
