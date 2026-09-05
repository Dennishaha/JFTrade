#![forbid(unsafe_code)]

use std::collections::BTreeMap;
use std::fs::File;
use std::path::Path;
use std::sync::Arc;
use std::thread;

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

struct EngineChallengeCluster {
    _dir: tempfile::TempDir,
    port: Arc<ProductionAdkPort>,
    adk_path: std::path::PathBuf,
    session_path: std::path::PathBuf,
    artifact_path: std::path::PathBuf,
}

impl EngineChallengeCluster {
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

/// CHALLENGE: Engine layer handling of pre-existing orphan rows in adk-session.db
#[test]
fn challenge_engine_deletes_orphan_session_events_and_reports_contract() {
    let cluster = EngineChallengeCluster::new();
    let session_id = "sess-orphan-eng-session";

    // Insert session and event directly into adk-session.db only
    cluster
        .port
        .session_store
        .upsert_session("jftrade", "local", session_id, "ACTIVE")
        .expect("upsert session in session store");
    cluster
        .port
        .session_store
        .record_event(RecordAdkEventParams {
            id: "evt-eng-1",
            app_name: "jftrade",
            user_id: "local",
            session_id,
            invocation_id: "inv-eng-1",
            author: "user",
            content: "orphan in session db",
        })
        .expect("record event");

    // First call: must succeed, returning 200 contract
    let result = cluster
        .delete_session(session_id)
        .expect("engine delete must succeed on orphan events");
    assert_eq!(result["id"], session_id);
    assert_eq!(result["deleted"], true);

    // Second call: must report 404 ADK_SESSION_NOT_FOUND
    let second_err = cluster
        .delete_session(session_id)
        .expect_err("subsequent delete must 404");
    match second_err {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_SESSION_NOT_FOUND");
        }
        other => panic!("Unexpected error: {other:?}"),
    }
}

/// CHALLENGE: Engine layer handling of pre-existing orphan rows in adk-artifact.db
#[test]
fn challenge_engine_deletes_orphan_artifacts_and_reports_contract() {
    let cluster = EngineChallengeCluster::new();
    let session_id = "sess-orphan-eng-artifacts";

    cluster
        .port
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "local",
            session_id,
            file_name: "orphan_file.txt",
            version: 1,
            part_json: r#"{"orphan":true}"#,
            mime_type: "text/plain",
            custom_metadata_json: None,
        })
        .expect("put artifact");

    let result = cluster
        .delete_session(session_id)
        .expect("engine delete must succeed on orphan artifacts");
    assert_eq!(result["id"], session_id);
    assert_eq!(result["deleted"], true);

    let second_err = cluster
        .delete_session(session_id)
        .expect_err("subsequent delete must 404");
    match second_err {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_SESSION_NOT_FOUND");
        }
        other => panic!("Unexpected error: {other:?}"),
    }
}

/// CHALLENGE: Engine layer handling of orphan runs and approvals in adk.db
#[test]
fn challenge_engine_deletes_orphan_runs_and_approvals_without_master_session() {
    let cluster = EngineChallengeCluster::new();
    let session_id = "sess-orphan-eng-runs";

    let run_id = "run-eng-orph-1";
    cluster
        .port
        .store
        .create_run(CreateAdkRunParams {
            id: run_id,
            session_id,
            agent_id: "agent-eng",
            status: "RUNNING",
            client_request_id: "req-eng-1",
            request_fingerprint: "fp-eng-1",
            payload_json: "{}",
        })
        .expect("create run");

    let conn_adk = Connection::open(&cluster.adk_path).expect("open adk");
    conn_adk
        .execute(
            "INSERT INTO adk_approvals (id, run_id, agent_id, status, payload_json, created_at, updated_at)
             VALUES ('appr-eng-1', ?1, 'agent-eng', 'PENDING', '{}', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')",
            params![run_id],
        )
        .expect("insert approval");

    let result = cluster
        .delete_session(session_id)
        .expect("engine delete must succeed on orphan runs/approvals");
    assert_eq!(result["id"], session_id);
    assert_eq!(result["deleted"], true);

    let second_err = cluster
        .delete_session(session_id)
        .expect_err("subsequent delete must 404");
    match second_err {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_SESSION_NOT_FOUND");
        }
        other => panic!("Unexpected error: {other:?}"),
    }
}

/// CHALLENGE: Engine layer concurrent deletion from 20 threads
#[test]
fn challenge_engine_concurrent_deletion_across_threads() {
    let cluster = Arc::new(EngineChallengeCluster::new());
    let session_id = "sess-eng-concurrent";

    cluster
        .port
        .store
        .upsert_session(session_id, "agent-conc", "{}")
        .expect("upsert session");
    cluster
        .port
        .session_store
        .upsert_session("jftrade", "local", session_id, "ACTIVE")
        .expect("upsert session");
    cluster
        .port
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "local",
            session_id,
            file_name: "data.bin",
            version: 1,
            part_json: "{}",
            mime_type: "application/octet-stream",
            custom_metadata_json: None,
        })
        .expect("put artifact");

    let thread_count = 20;
    let mut handles = Vec::with_capacity(thread_count);

    for _ in 0..thread_count {
        let cluster_clone = Arc::clone(&cluster);
        handles.push(thread::spawn(move || {
            cluster_clone.delete_session(session_id)
        }));
    }

    let mut success_200 = 0;
    let mut not_found_404 = 0;

    for handle in handles {
        match handle.join().expect("thread join") {
            Ok(val) => {
                assert_eq!(val["deleted"], true);
                success_200 += 1;
            }
            Err(AdkMutationPortError::Failed { status, code, .. }) => {
                assert_eq!(status, 404);
                assert_eq!(code, "ADK_SESSION_NOT_FOUND");
                not_found_404 += 1;
            }
            Err(other) => panic!("Unexpected error from concurrent deletion: {other:?}"),
        }
    }

    assert!(
        success_200 >= 1,
        "At least one thread must receive 200 OK, got {success_200} ok, {not_found_404} not found"
    );
    assert_eq!(success_200 + not_found_404, thread_count);

    // Verify all databases are completely empty of this session
    let conn_adk = Connection::open(&cluster.adk_path).expect("open adk");
    let adk_count: i64 = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_sessions WHERE id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(adk_count, 0);

    let conn_session = Connection::open(&cluster.session_path).expect("open session");
    let session_count: i64 = conn_session
        .query_row(
            "SELECT count(*) FROM sessions WHERE id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(session_count, 0);

    let conn_artifact = Connection::open(&cluster.artifact_path).expect("open artifact");
    let artifact_count: i64 = conn_artifact
        .query_row(
            "SELECT count(*) FROM artifacts WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(artifact_count, 0);
}
