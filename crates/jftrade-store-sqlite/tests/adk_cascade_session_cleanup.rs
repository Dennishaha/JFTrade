#![forbid(unsafe_code)]

use std::path::Path;
use std::time::Duration;

use jftrade_store_sqlite::{
    AdkArtifactStore, AdkSessionStore, AdkStore, AdkStoreError, CreateAdkRunParams,
    PutAdkArtifactParams, RecordAdkEventParams, initialize_current,
};
use rusqlite::{Connection, params};
use tempfile::tempdir;

fn initialize_database(path: &Path, component: &str) {
    let connection = Connection::open(path).expect("open sqlite");
    initialize_current(&connection, component).expect("initialize schema");
}

struct TestCluster {
    _dir: tempfile::TempDir,
    adk_store: AdkStore,
    session_store: AdkSessionStore,
    artifact_store: AdkArtifactStore,
    adk_path: std::path::PathBuf,
    session_path: std::path::PathBuf,
    artifact_path: std::path::PathBuf,
}

impl TestCluster {
    fn new() -> Self {
        let dir = tempdir().expect("tempdir");
        let adk_path = dir.path().join("adk.db");
        let session_path = dir.path().join("adk-session.db");
        let artifact_path = dir.path().join("adk-artifact.db");

        initialize_database(&adk_path, "adk");
        initialize_database(&session_path, "adk-session");
        initialize_database(&artifact_path, "adk-artifact");

        let adk_store = AdkStore::open(&adk_path).expect("open adk store");
        let session_store = AdkSessionStore::open(&session_path).expect("open session store");
        let artifact_store = AdkArtifactStore::open(&artifact_path).expect("open artifact store");

        Self {
            _dir: dir,
            adk_store,
            session_store,
            artifact_store,
            adk_path,
            session_path,
            artifact_path,
        }
    }
}

/// Comprehensive count across all 14 tables for a given session ID.
#[derive(Debug, Default, PartialEq, Eq)]
struct SessionRecordCounts {
    // adk.db (11 tables)
    sessions: i64,
    runs: i64,
    approvals: i64,
    tasks: i64,
    run_leases: i64,
    tool_invocations: i64,
    session_contexts: i64,
    session_context_state: i64,
    handoff_segments: i64,
    session_notices: i64,
    session_composer_state: i64,
    // adk-session.db (2 tables)
    adk_session_sessions: i64,
    events: i64,
    // adk-artifact.db (1 table)
    artifacts: i64,
}

impl SessionRecordCounts {
    fn total(&self) -> i64 {
        self.sessions
            + self.runs
            + self.approvals
            + self.tasks
            + self.run_leases
            + self.tool_invocations
            + self.session_contexts
            + self.session_context_state
            + self.handoff_segments
            + self.session_notices
            + self.session_composer_state
            + self.adk_session_sessions
            + self.events
            + self.artifacts
    }
}

fn count_session_records(cluster: &TestCluster, session_id: &str) -> SessionRecordCounts {
    let mut counts = SessionRecordCounts::default();

    // 1. adk.db
    let conn_adk = Connection::open(&cluster.adk_path).expect("open adk for count");
    counts.sessions = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_sessions WHERE id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    counts.runs = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_runs WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    counts.approvals = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_approvals WHERE run_id IN (SELECT id FROM adk_runs WHERE session_id = ?1)",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    counts.tasks = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_tasks WHERE run_id IN (SELECT id FROM adk_runs WHERE session_id = ?1)",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    counts.run_leases = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_run_leases WHERE run_id IN (SELECT id FROM adk_runs WHERE session_id = ?1)",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    counts.tool_invocations = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_tool_invocations WHERE run_id IN (SELECT id FROM adk_runs WHERE session_id = ?1)",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    counts.session_contexts = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_session_contexts WHERE id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    counts.session_context_state = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_session_context_state WHERE id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    counts.handoff_segments = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_handoff_segments WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    counts.session_notices = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_session_notices WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    counts.session_composer_state = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_session_composer_state WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();

    // 2. adk-session.db
    let conn_session = Connection::open(&cluster.session_path).expect("open session for count");
    counts.adk_session_sessions = conn_session
        .query_row(
            "SELECT count(*) FROM sessions WHERE id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    counts.events = conn_session
        .query_row(
            "SELECT count(*) FROM events WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();

    // 3. adk-artifact.db
    let conn_artifact = Connection::open(&cluster.artifact_path).expect("open artifact for count");
    counts.artifacts = conn_artifact
        .query_row(
            "SELECT count(*) FROM artifacts WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();

    counts
}

/// Seed complete session entities across all 14 tables in 3 databases.
fn seed_full_session(cluster: &TestCluster, session_id: &str, agent_id: &str, run_prefix: &str) {
    // 1. adk.db
    cluster
        .adk_store
        .upsert_session(
            session_id,
            agent_id,
            &format!(r#"{{"id":"{session_id}","agentId":"{agent_id}"}}"#),
        )
        .expect("upsert session");

    let run_id_1 = format!("{run_prefix}-1");
    let run_id_2 = format!("{run_prefix}-2");

    let run_1 = cluster
        .adk_store
        .create_run(CreateAdkRunParams {
            id: &run_id_1,
            session_id,
            agent_id,
            status: "RUNNING",
            client_request_id: &format!("req-{run_id_1}"),
            request_fingerprint: "fp-1",
            payload_json: "{}",
        })
        .expect("create run 1");

    cluster
        .adk_store
        .create_run(CreateAdkRunParams {
            id: &run_id_2,
            session_id,
            agent_id,
            status: "SUCCEEDED",
            client_request_id: &format!("req-{run_id_2}"),
            request_fingerprint: "fp-2",
            payload_json: "{}",
        })
        .expect("create run 2");

    let lease = cluster
        .adk_store
        .claim_run_lease(&run_id_1, "worker-1", Duration::from_secs(60))
        .expect("claim lease");

    cluster
        .adk_store
        .claim_tool_invocation_if_status_and_revision(
            &run_id_1,
            "call-1",
            "market.search",
            "{}",
            "RUNNING",
            &run_1.updated_at,
            "worker-1",
            lease.fencing_token,
            Duration::from_secs(30),
            false,
        )
        .expect("claim invocation");

    let conn_adk = Connection::open(&cluster.adk_path).expect("open adk");
    conn_adk
        .execute(
            "INSERT INTO adk_approvals (id, run_id, agent_id, status, payload_json, created_at, updated_at)
             VALUES (?1, ?2, ?3, 'PENDING', '{}', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')",
            params![format!("appr-{run_id_1}"), run_id_1, agent_id],
        )
        .expect("insert approval");
    conn_adk
        .execute(
            "INSERT INTO adk_tasks (id, status, agent_id, run_id, payload_json, created_at, updated_at)
             VALUES (?1, 'RUNNING', ?2, ?3, '{}', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')",
            params![format!("task-{run_id_1}"), agent_id, run_id_1],
        )
        .expect("insert task");
    conn_adk
        .execute(
            "INSERT INTO adk_session_contexts (id, payload_json, created_at, updated_at)
             VALUES (?1, '{}', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')",
            params![session_id],
        )
        .expect("insert context");
    conn_adk
        .execute(
            "INSERT INTO adk_session_context_state (id, payload_json, created_at, updated_at)
             VALUES (?1, '{}', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')",
            params![session_id],
        )
        .expect("insert context state");
    conn_adk
        .execute(
            "INSERT INTO adk_handoff_segments (id, session_id, active, sequence_no, created_at, updated_at, payload_json)
             VALUES (?1, ?2, 1, 1, '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z', '{}')",
            params![format!("ho-{session_id}"), session_id],
        )
        .expect("insert handoff");
    conn_adk
        .execute(
            "INSERT INTO adk_session_notices (id, session_id, run_id, kind, status, payload_json, created_at, updated_at)
             VALUES (?1, ?2, ?3, 'INFO', 'ACTIVE', '{}', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')",
            params![format!("not-{session_id}"), session_id, run_id_1],
        )
        .expect("insert notice");
    conn_adk
        .execute(
            "INSERT INTO adk_session_composer_state (id, session_id, payload_json, created_at, updated_at)
             VALUES (?1, ?2, '{}', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')",
            params![session_id, session_id],
        )
        .expect("insert composer state");

    // 2. adk-session.db
    cluster
        .session_store
        .upsert_session("jftrade", "local", session_id, "ACTIVE")
        .expect("upsert session in session store");
    cluster
        .session_store
        .record_event(RecordAdkEventParams {
            id: &format!("evt-{session_id}-1"),
            app_name: "jftrade",
            user_id: "local",
            session_id,
            invocation_id: &run_id_1,
            author: "user",
            content: "hello",
        })
        .expect("record event 1");
    cluster
        .session_store
        .record_event(RecordAdkEventParams {
            id: &format!("evt-{session_id}-2"),
            app_name: "jftrade",
            user_id: "local",
            session_id,
            invocation_id: &run_id_1,
            author: "assistant",
            content: "hi there",
        })
        .expect("record event 2");

    // 3. adk-artifact.db
    cluster
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "local",
            session_id,
            file_name: "report.md",
            version: 1,
            part_json: r#"{"text":"daily report"}"#,
            mime_type: "text/markdown",
            custom_metadata_json: None,
        })
        .expect("put artifact 1");
    cluster
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "local",
            session_id,
            file_name: "chart.png",
            version: 1,
            part_json: r#"{"data":"base64..."}"#,
            mime_type: "image/png",
            custom_metadata_json: None,
        })
        .expect("put artifact 2");
}

#[test]
fn test_adk_cascade_cleanup_removes_all_entities_across_three_databases() {
    let cluster = TestCluster::new();

    // Seed target session and control session
    seed_full_session(&cluster, "sess-target", "agent-alpha", "run-target");
    seed_full_session(&cluster, "sess-control", "agent-beta", "run-ctrl");

    let target_before = count_session_records(&cluster, "sess-target");
    let control_before = count_session_records(&cluster, "sess-control");

    assert!(
        target_before.total() >= 14,
        "Target session must have entries in all tables"
    );
    assert!(
        control_before.total() >= 14,
        "Control session must have entries in all tables"
    );

    // Execute cascade deletion
    let deleted = cluster
        .adk_store
        .delete_session_cascade(
            &cluster.session_store,
            &cluster.artifact_store,
            "sess-target",
        )
        .expect("delete_session_cascade");
    assert!(deleted, "delete_session_cascade must report true");

    // Assert zero orphan rows for target
    let target_after = count_session_records(&cluster, "sess-target");
    assert_eq!(
        target_after,
        SessionRecordCounts::default(),
        "All 14 tables must have 0 rows for sess-target"
    );

    // Assert control session is completely untouched
    let control_after = count_session_records(&cluster, "sess-control");
    assert_eq!(
        control_after, control_before,
        "Control session records must not be affected"
    );
}

#[test]
fn test_shared_artifacts_preserved_and_user_session_deletion_rejected() {
    let cluster = TestCluster::new();

    seed_full_session(&cluster, "sess-regular", "agent-alpha", "run-reg");

    // Put shared user artifacts under session_id = "user"
    cluster
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "local",
            session_id: "user",
            file_name: "user:preferences.json",
            version: 1,
            part_json: r#"{"theme":"dark"}"#,
            mime_type: "application/json",
            custom_metadata_json: None,
        })
        .expect("put shared artifact 1");
    cluster
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "local",
            session_id: "user",
            file_name: "user:prompt_template.txt",
            version: 1,
            part_json: r#"{"template":"default"}"#,
            mime_type: "text/plain",
            custom_metadata_json: None,
        })
        .expect("put shared artifact 2");

    // 1. Regular session deletion preserves shared artifacts
    cluster
        .adk_store
        .delete_session_cascade(
            &cluster.session_store,
            &cluster.artifact_store,
            "sess-regular",
        )
        .expect("delete regular session");

    let regular_counts = count_session_records(&cluster, "sess-regular");
    assert_eq!(regular_counts.total(), 0, "Regular session must be cleaned");

    let conn_artifact = Connection::open(&cluster.artifact_path).expect("open artifact");
    let shared_count: i64 = conn_artifact
        .query_row(
            "SELECT count(*) FROM artifacts WHERE session_id = 'user'",
            [],
            |r| r.get(0),
        )
        .expect("count shared artifacts");
    assert_eq!(
        shared_count, 2,
        "Shared user artifacts must remain untouched"
    );

    // 2. Direct deletion of "user" session is strictly rejected
    let err = cluster
        .adk_store
        .delete_session_cascade(&cluster.session_store, &cluster.artifact_store, "user")
        .expect_err("deleting 'user' session must fail");

    assert!(
        matches!(err, AdkStoreError::Validation(_)),
        "Error must be a validation error rejecting protected user session: {err:?}"
    );

    let shared_count_after: i64 = conn_artifact
        .query_row(
            "SELECT count(*) FROM artifacts WHERE session_id = 'user'",
            [],
            |r| r.get(0),
        )
        .expect("count shared artifacts after reject");
    assert_eq!(
        shared_count_after, 2,
        "Shared artifacts must still be intact"
    );
}

#[test]
fn test_idempotent_retry_and_partial_failure_crash_recovery() {
    let cluster = TestCluster::new();

    // Part A: Repeated deletion on already-deleted session
    seed_full_session(&cluster, "sess-repeat", "agent-alpha", "run-rep");
    let first = cluster
        .adk_store
        .delete_session_cascade(
            &cluster.session_store,
            &cluster.artifact_store,
            "sess-repeat",
        )
        .expect("first delete");
    assert!(first);

    let second = cluster
        .adk_store
        .delete_session_cascade(
            &cluster.session_store,
            &cluster.artifact_store,
            "sess-repeat",
        )
        .expect("second delete must not fail");
    assert!(!second, "Second delete on clean session must return false");

    // Part B: Crash after artifact deletion (artifacts already gone, events/runs remain)
    seed_full_session(&cluster, "sess-crash-1", "agent-alpha", "run-c1");
    let conn_artifact = Connection::open(&cluster.artifact_path).expect("open artifact");
    conn_artifact
        .execute(
            "DELETE FROM artifacts WHERE session_id = 'sess-crash-1'",
            [],
        )
        .expect("simulate artifact crash");

    // Now resume cascade deletion
    let resumed = cluster
        .adk_store
        .delete_session_cascade(
            &cluster.session_store,
            &cluster.artifact_store,
            "sess-crash-1",
        )
        .expect("resumed cascade delete");
    assert!(resumed);
    assert_eq!(
        count_session_records(&cluster, "sess-crash-1"),
        SessionRecordCounts::default(),
        "All tables must reach zero orphans after resumed deletion"
    );

    // Part C: Orphan sweep where adk_sessions was already lost
    let conn_adk = Connection::open(&cluster.adk_path).expect("open adk");
    conn_adk
        .execute(
            "INSERT INTO adk_runs (id, session_id, agent_id, status, client_request_id, request_fingerprint, payload_json, created_at, updated_at)
             VALUES ('run-orph-1', 'sess-orphan', 'agent-1', 'RUNNING', 'req-1', 'fp-1', '{}', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')",
            [],
        )
        .expect("insert orphan run");

    cluster
        .session_store
        .upsert_session("jftrade", "local", "sess-orphan", "ACTIVE")
        .expect("upsert session");
    cluster
        .session_store
        .record_event(RecordAdkEventParams {
            id: "evt-orph-1",
            app_name: "jftrade",
            user_id: "local",
            session_id: "sess-orphan",
            invocation_id: "inv-1",
            author: "user",
            content: "lost",
        })
        .expect("record orphan event");

    conn_artifact
        .execute(
            "INSERT INTO artifacts (app_name, user_id, session_id, file_name, version, part_json, mime_type, created_at, updated_at)
             VALUES ('jftrade', 'local', 'sess-orphan', 'orphan.txt', 1, '{}', 'text/plain', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')",
            [],
        )
        .expect("insert orphan artifact");

    // Verify adk_sessions row is absent
    let adk_sess_count: i64 = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_sessions WHERE id = 'sess-orphan'",
            [],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(adk_sess_count, 0, "adk_sessions row must not exist");

    let sweep = cluster
        .adk_store
        .delete_session_cascade(
            &cluster.session_store,
            &cluster.artifact_store,
            "sess-orphan",
        )
        .expect("orphan sweep");
    assert!(sweep, "Orphan sweep must clean orphans and report true");
    assert_eq!(
        count_session_records(&cluster, "sess-orphan"),
        SessionRecordCounts::default(),
        "Orphans must be completely cleared"
    );
}

#[test]
fn test_app_name_agnostic_cleanup_across_databases() {
    let cluster = TestCluster::new();
    let session_id = "sess-app-agnostic";

    // Insert session into adk.db
    cluster
        .adk_store
        .upsert_session(
            session_id,
            "agent-app-test",
            &format!(r#"{{"id":"{session_id}","agentId":"agent-app-test"}}"#),
        )
        .expect("upsert session");

    // Upsert sessions under different app_names in adk-session.db
    cluster
        .session_store
        .upsert_session("jftrade", "local", session_id, "ACTIVE")
        .expect("upsert session jftrade");
    cluster
        .session_store
        .upsert_session("google-adk:agent-app-test", "local", session_id, "ACTIVE")
        .expect("upsert session google");

    // Insert events with different app_names in adk-session.db
    cluster
        .session_store
        .record_event(RecordAdkEventParams {
            id: "evt-app-1",
            app_name: "jftrade",
            user_id: "local",
            session_id,
            invocation_id: "inv-1",
            author: "user",
            content: "event under jftrade",
        })
        .expect("record event 1");
    cluster
        .session_store
        .record_event(RecordAdkEventParams {
            id: "evt-app-2",
            app_name: "google-adk:agent-app-test",
            user_id: "local",
            session_id,
            invocation_id: "inv-2",
            author: "assistant",
            content: "event under google-adk",
        })
        .expect("record event 2");

    // Insert artifacts with different app_names in adk-artifact.db
    cluster
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "local",
            session_id,
            file_name: "art-jftrade.txt",
            version: 1,
            part_json: "{}",
            mime_type: "text/plain",
            custom_metadata_json: None,
        })
        .expect("put artifact jftrade");
    cluster
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "google-adk:agent-app-test",
            user_id: "local",
            session_id,
            file_name: "art-google.txt",
            version: 1,
            part_json: "{}",
            mime_type: "text/plain",
            custom_metadata_json: None,
        })
        .expect("put artifact google");

    // Verify before deletion
    let before = count_session_records(&cluster, session_id);
    assert_eq!(before.events, 2);
    assert_eq!(before.artifacts, 2);
    assert_eq!(before.sessions, 1);

    // Delete session cascade
    let deleted = cluster
        .adk_store
        .delete_session_cascade(&cluster.session_store, &cluster.artifact_store, session_id)
        .expect("delete cascade");
    assert!(deleted);

    // Verify all records cleared regardless of app_name
    let after = count_session_records(&cluster, session_id);
    assert_eq!(
        after,
        SessionRecordCounts::default(),
        "All records across all app_names must be wiped"
    );
}
