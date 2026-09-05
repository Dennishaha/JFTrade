#![forbid(unsafe_code)]

use std::collections::BTreeMap;
use std::fs::File;
use std::path::Path;
use std::sync::Arc;

use jftrade_engine::product::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationOperation, AdkMutationPort, AdkMutationPortError,
};
use jftrade_engine::product::product_production_ports::product_production_ports_adk::ProductionAdkPort;
use jftrade_store_sqlite::{
    AdkArtifactStore, AdkSessionStore, AdkStore, CreateAdkRunParams, PutAdkArtifactParams,
    RecordAdkEventParams, initialize_current,
};
use rusqlite::{Connection, params};
use serde_json::Value;
use tempfile::tempdir;

fn initialize_db(path: &Path, component: &str) {
    File::create(path).expect("create database file");
    let connection = Connection::open(path).expect("open sqlite");
    initialize_current(&connection, component).expect("initialize schema");
}

struct EngineTestCluster {
    _dir: tempfile::TempDir,
    port: Arc<ProductionAdkPort>,
    adk_path: std::path::PathBuf,
    session_path: std::path::PathBuf,
    artifact_path: std::path::PathBuf,
}

impl EngineTestCluster {
    fn new() -> Self {
        let dir = tempdir().expect("tempdir");
        let adk_path = dir.path().join("adk.db");
        let session_path = dir.path().join("adk-session.db");
        let artifact_path = dir.path().join("adk-artifact.db");

        initialize_db(&adk_path, "adk");
        initialize_db(&session_path, "adk-session");
        initialize_db(&artifact_path, "adk-artifact");

        let store = Arc::new(AdkStore::open(&adk_path).expect("open adk store"));
        let session_store =
            Arc::new(AdkSessionStore::open(&session_path).expect("open session store"));
        let artifact_store =
            Arc::new(AdkArtifactStore::open(&artifact_path).expect("open artifact store"));

        let port = Arc::new(ProductionAdkPort::new_for_test(
            store,
            session_store,
            artifact_store,
            dir.path().join("settings.json"),
        ));

        Self {
            _dir: dir,
            port,
            adk_path,
            session_path,
            artifact_path,
        }
    }

    fn delete_session(&self, session_id: &str) -> Result<Value, AdkMutationPortError> {
        let mut identifiers = BTreeMap::new();
        identifiers.insert("sessionId".to_owned(), session_id.to_owned());
        let input = AdkMutationInput {
            operation: AdkMutationOperation::DeleteSession,
            identifiers,
            body: Value::Null,
            webhook_secret: None,
        };
        self.port.mutate(&input)
    }
}

#[test]
fn test_delete_session_cascades_across_all_three_databases_end_to_end() {
    let cluster = EngineTestCluster::new();
    let session_id = "sess-e2e-1";

    // 1. Seed session in adk.db
    cluster
        .port
        .store
        .upsert_session(
            session_id,
            "agent-default",
            r#"{"agentId":"agent-default"}"#,
        )
        .expect("upsert session");
    cluster
        .port
        .store
        .create_run(CreateAdkRunParams {
            id: "run-e2e-1",
            session_id,
            agent_id: "agent-default",
            status: "SUCCEEDED",
            client_request_id: "req-e2e-1",
            request_fingerprint: "fp-e2e-1",
            payload_json: "{}",
        })
        .expect("create run");

    // 2. Seed events in adk-session.db
    cluster
        .port
        .session_store
        .upsert_session("jftrade", "local", session_id, "ACTIVE")
        .expect("upsert session in session store");
    cluster
        .port
        .session_store
        .record_event(RecordAdkEventParams {
            id: "evt-e2e-1",
            app_name: "jftrade",
            user_id: "local",
            session_id,
            invocation_id: "run-e2e-1",
            author: "user",
            content: "e2e query",
        })
        .expect("record event");

    // 3. Seed artifacts in adk-artifact.db
    cluster
        .port
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "local",
            session_id,
            file_name: "analysis.txt",
            version: 1,
            part_json: r#"{"text":"done"}"#,
            mime_type: "text/plain",
            custom_metadata_json: None,
        })
        .expect("put artifact");

    // Execute DELETE via Port
    let response = cluster
        .delete_session(session_id)
        .expect("mutation delete_session");
    assert_eq!(response["deleted"], true);
    assert_eq!(response["id"], session_id);

    // Verify raw SQLite state across all 3 databases
    let conn_adk = Connection::open(&cluster.adk_path).expect("open adk");
    let adk_sess_cnt: i64 = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_sessions WHERE id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    let adk_run_cnt: i64 = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_runs WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(adk_sess_cnt, 0, "adk_sessions must be 0");
    assert_eq!(adk_run_cnt, 0, "adk_runs must be 0");

    let conn_sess = Connection::open(&cluster.session_path).expect("open session");
    let sess_evt_cnt: i64 = conn_sess
        .query_row(
            "SELECT count(*) FROM events WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(sess_evt_cnt, 0, "events must be 0");

    let conn_art = Connection::open(&cluster.artifact_path).expect("open artifact");
    let art_cnt: i64 = conn_art
        .query_row(
            "SELECT count(*) FROM artifacts WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(art_cnt, 0, "artifacts must be 0");
}

#[test]
fn test_delete_session_handles_missing_or_corrupt_agent_id_in_payload_json() {
    let cluster = EngineTestCluster::new();
    let session_id = "sess-corrupt-payload";

    // Insert session with empty payload JSON (no agentId inside JSON)
    cluster
        .port
        .store
        .upsert_session(session_id, "agent-from-column", "{}")
        .expect("upsert session with empty JSON");

    // Seed an event
    cluster
        .port
        .session_store
        .upsert_session("jftrade", "local", session_id, "ACTIVE")
        .expect("upsert session");
    cluster
        .port
        .session_store
        .record_event(RecordAdkEventParams {
            id: "evt-corrupt-1",
            app_name: "jftrade",
            user_id: "local",
            session_id,
            invocation_id: "inv-corrupt",
            author: "user",
            content: "query",
        })
        .expect("record event");

    // Attempt deletion — must succeed and not fail with premature 404
    let response = cluster
        .delete_session(session_id)
        .expect("deletion must succeed despite empty payload JSON");
    assert_eq!(response["deleted"], true);
    assert_eq!(response["id"], session_id);
}

#[test]
fn test_delete_session_safeguards_reserved_user_scope_from_http_api() {
    let cluster = EngineTestCluster::new();

    // Put shared artifact under session_id = "user"
    cluster
        .port
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "local",
            session_id: "user",
            file_name: "user:global_settings.json",
            version: 1,
            part_json: r#"{"key":"val"}"#,
            mime_type: "application/json",
            custom_metadata_json: None,
        })
        .expect("put shared artifact");

    // Attempt to delete reserved "user" session
    let err = cluster
        .delete_session("user")
        .expect_err("deleting 'user' session must fail");

    match err {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_SESSION_PROTECTED");
        }
        other => panic!("Expected Failed status 400 ADK_SESSION_PROTECTED, got {other:?}"),
    }

    // Verify shared artifact still exists
    let conn_art = Connection::open(&cluster.artifact_path).expect("open artifact");
    let count: i64 = conn_art
        .query_row(
            "SELECT count(*) FROM artifacts WHERE session_id = 'user'",
            [],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(count, 1, "Shared user artifact must remain untouched");
}

#[test]
fn test_delete_session_app_name_agnostic_cleanup_defeats_mismatch_bug() {
    let cluster = EngineTestCluster::new();
    let session_id = "sess-heterogeneous";

    cluster
        .port
        .store
        .upsert_session(session_id, "agent-xyz", r#"{"agentId":"agent-xyz"}"#)
        .expect("upsert session");

    // Event written with app_name = "jftrade" (the bug used "google-adk:agent-xyz")
    cluster
        .port
        .session_store
        .upsert_session("jftrade", "local", session_id, "ACTIVE")
        .expect("upsert session");
    cluster
        .port
        .session_store
        .record_event(RecordAdkEventParams {
            id: "evt-jftrade-1",
            app_name: "jftrade",
            user_id: "local",
            session_id,
            invocation_id: "inv-1",
            author: "user",
            content: "jftrade event",
        })
        .expect("record event");

    // Artifact written with app_name = "jftrade"
    cluster
        .port
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "local",
            session_id,
            file_name: "doc.txt",
            version: 1,
            part_json: r#"{"text":"data"}"#,
            mime_type: "text/plain",
            custom_metadata_json: None,
        })
        .expect("put artifact");

    let response = cluster.delete_session(session_id).expect("delete session");
    assert_eq!(response["deleted"], true);
    assert_eq!(response["id"], session_id);

    // Verify both are completely purged despite app_name being "jftrade"
    let conn_sess = Connection::open(&cluster.session_path).expect("open session");
    let sess_evt_cnt: i64 = conn_sess
        .query_row(
            "SELECT count(*) FROM events WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(
        sess_evt_cnt, 0,
        "Event must be cleaned regardless of app_name"
    );

    let conn_art = Connection::open(&cluster.artifact_path).expect("open artifact");
    let art_cnt: i64 = conn_art
        .query_row(
            "SELECT count(*) FROM artifacts WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(
        art_cnt, 0,
        "Artifact must be cleaned regardless of app_name"
    );
}

#[test]
fn test_delete_session_not_found_on_clean_state() {
    let cluster = EngineTestCluster::new();
    let err = cluster
        .delete_session("sess-nonexistent")
        .expect_err("nonexistent session must fail");

    match err {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_SESSION_NOT_FOUND");
        }
        other => panic!("Expected Failed status 404 ADK_SESSION_NOT_FOUND, got {other:?}"),
    }
}
