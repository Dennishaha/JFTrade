#![forbid(unsafe_code)]

use std::path::Path;
use std::sync::Arc;
use std::thread;
use std::time::Duration;

use jftrade_store_sqlite::{
    AdkArtifactStore, AdkSessionStore, AdkStore, CreateAdkRunParams, PutAdkArtifactParams,
    RecordAdkEventParams, initialize_current,
};
use rusqlite::{Connection, params};
use tempfile::tempdir;

fn initialize_database(path: &Path, component: &str) {
    let connection = Connection::open(path).expect("open sqlite");
    initialize_current(&connection, component).expect("initialize schema");
}

struct ChallengeCluster {
    _dir: tempfile::TempDir,
    adk_store: Arc<AdkStore>,
    session_store: Arc<AdkSessionStore>,
    artifact_store: Arc<AdkArtifactStore>,
    adk_path: std::path::PathBuf,
    session_path: std::path::PathBuf,
    artifact_path: std::path::PathBuf,
}

impl ChallengeCluster {
    fn new() -> Self {
        let dir = tempdir().expect("tempdir");
        let adk_path = dir.path().join("adk.db");
        let session_path = dir.path().join("adk-session.db");
        let artifact_path = dir.path().join("adk-artifact.db");

        initialize_database(&adk_path, "adk");
        initialize_database(&session_path, "adk-session");
        initialize_database(&artifact_path, "adk-artifact");

        let adk_store = Arc::new(AdkStore::open(&adk_path).expect("open adk store"));
        let session_store =
            Arc::new(AdkSessionStore::open(&session_path).expect("open session store"));
        let artifact_store =
            Arc::new(AdkArtifactStore::open(&artifact_path).expect("open artifact store"));

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

#[derive(Debug, Default, PartialEq, Eq)]
struct SessionRecordAudit {
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

impl SessionRecordAudit {
    #[allow(dead_code)]
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

fn audit_records(cluster: &ChallengeCluster, session_id: &str) -> SessionRecordAudit {
    let mut audit = SessionRecordAudit::default();

    let conn_adk = Connection::open(&cluster.adk_path).expect("open adk");
    audit.sessions = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_sessions WHERE id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    audit.runs = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_runs WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    audit.approvals = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_approvals WHERE run_id IN (SELECT id FROM adk_runs WHERE session_id = ?1)",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    audit.tasks = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_tasks WHERE run_id IN (SELECT id FROM adk_runs WHERE session_id = ?1)",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    audit.run_leases = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_run_leases WHERE run_id IN (SELECT id FROM adk_runs WHERE session_id = ?1)",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    audit.tool_invocations = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_tool_invocations WHERE run_id IN (SELECT id FROM adk_runs WHERE session_id = ?1)",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    audit.session_contexts = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_session_contexts WHERE id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    audit.session_context_state = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_session_context_state WHERE id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    audit.handoff_segments = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_handoff_segments WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    audit.session_notices = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_session_notices WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    audit.session_composer_state = conn_adk
        .query_row(
            "SELECT count(*) FROM adk_session_composer_state WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();

    let conn_session = Connection::open(&cluster.session_path).expect("open session");
    audit.adk_session_sessions = conn_session
        .query_row(
            "SELECT count(*) FROM sessions WHERE id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();
    audit.events = conn_session
        .query_row(
            "SELECT count(*) FROM events WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();

    let conn_artifact = Connection::open(&cluster.artifact_path).expect("open artifact");
    audit.artifacts = conn_artifact
        .query_row(
            "SELECT count(*) FROM artifacts WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap();

    audit
}

/// CHALLENGE 1A:
/// Inject orphan events into `adk-session.db` without corresponding rows in `adk.db`.
/// Verify deletion eliminates all orphan rows cleanly without panicking.
#[test]
fn challenge_orphan_events_in_session_db_without_adk_db_rows() {
    let cluster = ChallengeCluster::new();
    let session_id = "sess-orphan-events-only";

    // Seed session and events into adk-session.db ONLY
    cluster
        .session_store
        .upsert_session("jftrade", "user-1", session_id, "ACTIVE")
        .expect("upsert session in adk-session");

    for i in 1..=5 {
        cluster
            .session_store
            .record_event(RecordAdkEventParams {
                id: &format!("evt-{session_id}-{i}"),
                app_name: "jftrade",
                user_id: "user-1",
                session_id,
                invocation_id: "inv-orph",
                author: if i % 2 == 0 { "assistant" } else { "user" },
                content: &format!("orphan content {i}"),
            })
            .expect("record event");
    }

    // Verify adk.db and adk-artifact.db have zero rows for this session
    let initial_audit = audit_records(&cluster, session_id);
    assert_eq!(initial_audit.sessions, 0, "adk.db must have no session");
    assert_eq!(initial_audit.runs, 0, "adk.db must have no runs");
    assert_eq!(
        initial_audit.artifacts, 0,
        "adk-artifact.db must have no artifacts"
    );
    assert_eq!(initial_audit.adk_session_sessions, 1);
    assert_eq!(initial_audit.events, 5);

    // Trigger cascade deletion
    let deleted = cluster
        .adk_store
        .delete_session_cascade(&cluster.session_store, &cluster.artifact_store, session_id)
        .expect("delete_session_cascade must not panic or error");

    assert!(
        deleted,
        "Deletion must return true because orphan events were eliminated"
    );

    // Verify clean elimination of all orphan rows
    let post_audit = audit_records(&cluster, session_id);
    assert_eq!(
        post_audit,
        SessionRecordAudit::default(),
        "All tables must have 0 rows after deleting orphan events"
    );
}

/// CHALLENGE 1B:
/// Raw orphan events in `adk-session.db` where even `sessions` table in `adk-session.db`
/// is missing (inserted directly with PRAGMA foreign_keys = OFF).
#[test]
fn challenge_raw_orphan_events_without_parent_session_in_session_db() {
    let cluster = ChallengeCluster::new();
    let session_id = "sess-raw-orphan-events";

    // Direct insert with foreign keys disabled
    let conn = Connection::open(&cluster.session_path).expect("open session db");
    conn.execute_batch(&format!(
        "PRAGMA foreign_keys = OFF;
         INSERT INTO events (id, app_name, user_id, session_id, invocation_id, author, content)
         VALUES ('evt-raw-1', 'app-x', 'usr-x', '{session_id}', 'inv-1', 'user', 'lost event');
         PRAGMA foreign_keys = ON;"
    ))
    .expect("insert raw orphan event");

    let audit_before = audit_records(&cluster, session_id);
    assert_eq!(audit_before.events, 1);
    assert_eq!(audit_before.adk_session_sessions, 0);
    assert_eq!(audit_before.sessions, 0);

    let deleted = cluster
        .adk_store
        .delete_session_cascade(&cluster.session_store, &cluster.artifact_store, session_id)
        .expect("delete cascade must handle raw orphan events");

    assert!(deleted);

    let audit_after = audit_records(&cluster, session_id);
    assert_eq!(audit_after, SessionRecordAudit::default());
}

/// CHALLENGE 2:
/// Inject pre-existing orphan rows into `adk-artifact.db` without corresponding rows in `adk.db`.
/// Multiple files, multiple versions, multiple app_names.
/// Shared user artifacts must NOT be deleted.
#[test]
fn challenge_orphan_artifacts_in_artifact_db_without_adk_db_rows() {
    let cluster = ChallengeCluster::new();
    let session_id = "sess-orphan-artifacts-only";

    // Seed multiple orphan artifacts
    for version in 1..=3 {
        cluster
            .artifact_store
            .put_artifact(PutAdkArtifactParams {
                app_name: "app-alpha",
                user_id: "user-1",
                session_id,
                file_name: "analysis.json",
                version,
                part_json: r#"{"status":"orphan"}"#,
                mime_type: "application/json",
                custom_metadata_json: None,
            })
            .expect("put artifact");
    }

    cluster
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "app-beta",
            user_id: "user-2",
            session_id,
            file_name: "chart.png",
            version: 1,
            part_json: r#"{"type":"image"}"#,
            mime_type: "image/png",
            custom_metadata_json: None,
        })
        .expect("put artifact");

    // Also put shared user artifact
    cluster
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "app-alpha",
            user_id: "user-1",
            session_id: "user",
            file_name: "shared_config.json",
            version: 1,
            part_json: r#"{"shared":true}"#,
            mime_type: "application/json",
            custom_metadata_json: None,
        })
        .expect("put shared artifact");

    // Verify adk.db and adk-session.db have zero rows
    let audit_before = audit_records(&cluster, session_id);
    assert_eq!(audit_before.sessions, 0);
    assert_eq!(audit_before.runs, 0);
    assert_eq!(audit_before.events, 0);
    assert_eq!(audit_before.artifacts, 4);

    let deleted = cluster
        .adk_store
        .delete_session_cascade(&cluster.session_store, &cluster.artifact_store, session_id)
        .expect("delete cascade must succeed");

    assert!(deleted);

    let audit_after = audit_records(&cluster, session_id);
    assert_eq!(audit_after, SessionRecordAudit::default());

    // Verify shared user artifact is untouched
    let conn_artifact = Connection::open(&cluster.artifact_path).expect("open artifact");
    let shared_count: i64 = conn_artifact
        .query_row(
            "SELECT count(*) FROM artifacts WHERE session_id = 'user'",
            [],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(shared_count, 1, "Shared user artifact must be preserved");
}

/// CHALLENGE 3:
/// Inject orphan runs, approvals, tasks, leases, invocations, contexts, notices in `adk.db`
/// WITHOUT any `adk_sessions` row in `adk.db`.
#[test]
fn challenge_orphan_runs_and_approvals_in_adk_db_without_session_row() {
    let cluster = ChallengeCluster::new();
    let session_id = "sess-orphan-runs-no-master-session";

    let run_id = "run-orphaned-1";
    let run = cluster
        .adk_store
        .create_run(CreateAdkRunParams {
            id: run_id,
            session_id,
            agent_id: "agent-test",
            status: "RUNNING",
            client_request_id: "req-orph-1",
            request_fingerprint: "fp-orph-1",
            payload_json: "{}",
        })
        .expect("create run");

    let lease = cluster
        .adk_store
        .claim_run_lease(run_id, "worker-x", Duration::from_secs(60))
        .expect("claim lease");

    cluster
        .adk_store
        .claim_tool_invocation_if_status_and_revision(
            run_id,
            "call-orph-1",
            "search",
            "{}",
            "RUNNING",
            &run.updated_at,
            "worker-x",
            lease.fencing_token,
            Duration::from_secs(30),
            false,
        )
        .expect("claim invocation");

    // Inject approvals, tasks, context, notice, handoff directly
    let conn_adk = Connection::open(&cluster.adk_path).expect("open adk");
    conn_adk
        .execute(
            "INSERT INTO adk_approvals (id, run_id, agent_id, status, payload_json, created_at, updated_at)
             VALUES ('appr-orph-1', ?1, 'agent-test', 'PENDING', '{}', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')",
            params![run_id],
        )
        .expect("insert approval");
    conn_adk
        .execute(
            "INSERT INTO adk_tasks (id, status, agent_id, run_id, payload_json, created_at, updated_at)
             VALUES ('task-orph-1', 'RUNNING', 'agent-test', ?1, '{}', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')",
            params![run_id],
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
            "INSERT INTO adk_handoff_segments (id, session_id, active, sequence_no, created_at, updated_at, payload_json)
             VALUES ('ho-orph-1', ?1, 1, 1, '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z', '{}')",
            params![session_id],
        )
        .expect("insert handoff");
    conn_adk
        .execute(
            "INSERT INTO adk_session_notices (id, session_id, run_id, kind, status, payload_json, created_at, updated_at)
             VALUES ('not-orph-1', ?1, ?2, 'INFO', 'ACTIVE', '{}', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')",
            params![session_id, run_id],
        )
        .expect("insert notice");
    conn_adk
        .execute(
            "INSERT INTO adk_session_composer_state (id, session_id, payload_json, created_at, updated_at)
             VALUES (?1, ?1, '{}', '2026-09-01T00:00:00Z', '2026-09-01T00:00:00Z')",
            params![session_id],
        )
        .expect("insert composer state");

    // Crucial check: verify adk_sessions row does NOT exist
    let audit_before = audit_records(&cluster, session_id);
    assert_eq!(audit_before.sessions, 0, "adk_sessions row must NOT exist");
    assert_eq!(audit_before.runs, 1);
    assert_eq!(audit_before.approvals, 1);
    assert_eq!(audit_before.tasks, 1);
    assert_eq!(audit_before.run_leases, 1);
    assert_eq!(audit_before.tool_invocations, 1);
    assert_eq!(audit_before.session_contexts, 1);
    assert_eq!(audit_before.handoff_segments, 1);
    assert_eq!(audit_before.session_notices, 1);
    assert_eq!(audit_before.session_composer_state, 1);

    // Delete cascade
    let deleted = cluster
        .adk_store
        .delete_session_cascade(&cluster.session_store, &cluster.artifact_store, session_id)
        .expect("delete cascade must succeed");

    assert!(
        deleted,
        "Must return true because orphan runs/approvals were eliminated"
    );

    let audit_after = audit_records(&cluster, session_id);
    assert_eq!(
        audit_after,
        SessionRecordAudit::default(),
        "All 11 adk.db tables must have 0 rows"
    );
}

/// CHALLENGE 4:
/// Rapid sequential back-to-back deletions of the same session ID.
/// Call 1 -> true.
/// Call 2..N -> false without error.
#[test]
fn challenge_rapid_sequential_duplicate_deletions() {
    let cluster = ChallengeCluster::new();
    let session_id = "sess-sequential-duplicates";

    // Seed session in adk.db and adk-session.db and adk-artifact.db
    cluster
        .adk_store
        .upsert_session(session_id, "agent-1", "{}")
        .expect("upsert session");
    cluster
        .session_store
        .upsert_session("jftrade", "local", session_id, "ACTIVE")
        .expect("upsert session");
    cluster
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "local",
            session_id,
            file_name: "doc.txt",
            version: 1,
            part_json: "{}",
            mime_type: "text/plain",
            custom_metadata_json: None,
        })
        .expect("put artifact");

    // First call: must succeed and report true
    let first = cluster
        .adk_store
        .delete_session_cascade(&cluster.session_store, &cluster.artifact_store, session_id)
        .expect("call 1");
    assert!(first, "First deletion must return true");

    // Subsequent rapid duplicate calls: must return false cleanly
    for i in 2..=10 {
        let result = cluster
            .adk_store
            .delete_session_cascade(&cluster.session_store, &cluster.artifact_store, session_id)
            .unwrap_or_else(|e| panic!("Call {i} failed with error: {e:?}"));
        assert!(!result, "Duplicate call {i} must return false");
    }

    assert_eq!(
        audit_records(&cluster, session_id),
        SessionRecordAudit::default()
    );
}

/// CHALLENGE 5:
/// Concurrent back-to-back deletions of the same session ID across 20 threads.
/// No deadlocks, no SQLite busy errors, clean final state of 0 records.
#[test]
fn challenge_concurrent_back_to_back_deletions() {
    let cluster = Arc::new(ChallengeCluster::new());
    let session_id = "sess-concurrent-race";

    // Seed data across all three databases
    cluster
        .adk_store
        .upsert_session(session_id, "agent-conc", "{}")
        .expect("upsert session");
    cluster
        .adk_store
        .create_run(CreateAdkRunParams {
            id: "run-conc-1",
            session_id,
            agent_id: "agent-conc",
            status: "SUCCEEDED",
            client_request_id: "req-c",
            request_fingerprint: "fp-c",
            payload_json: "{}",
        })
        .expect("create run");
    cluster
        .session_store
        .upsert_session("jftrade", "local", session_id, "ACTIVE")
        .expect("upsert session");
    cluster
        .session_store
        .record_event(RecordAdkEventParams {
            id: "evt-conc-1",
            app_name: "jftrade",
            user_id: "local",
            session_id,
            invocation_id: "run-conc-1",
            author: "user",
            content: "race test",
        })
        .expect("record event");
    cluster
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "local",
            session_id,
            file_name: "race.json",
            version: 1,
            part_json: "{}",
            mime_type: "application/json",
            custom_metadata_json: None,
        })
        .expect("put artifact");

    let thread_count = 20;
    let mut handles = Vec::with_capacity(thread_count);

    for _ in 0..thread_count {
        let cluster_clone = Arc::clone(&cluster);
        handles.push(thread::spawn(move || {
            cluster_clone.adk_store.delete_session_cascade(
                &cluster_clone.session_store,
                &cluster_clone.artifact_store,
                session_id,
            )
        }));
    }

    let mut true_count = 0;
    let mut false_count = 0;

    for handle in handles {
        let res = handle.join().expect("thread join failed");
        match res {
            Ok(true) => true_count += 1,
            Ok(false) => false_count += 1,
            Err(e) => panic!("Concurrent deletion error: {e:?}"),
        }
    }

    assert!(
        true_count >= 1,
        "At least one thread must report true deletion, got true={true_count}, false={false_count}"
    );
    assert_eq!(
        true_count + false_count,
        thread_count,
        "All threads must complete successfully without errors"
    );

    // Verify all records are 0
    let final_audit = audit_records(&cluster, session_id);
    assert_eq!(
        final_audit,
        SessionRecordAudit::default(),
        "All tables must be completely empty after concurrent deletions"
    );
}

/// CHALLENGE 6:
/// Multi-stage crash and partial failure recovery simulation.
/// Crash after artifact deletion -> recover and delete.
/// Crash after session DB deletion -> recover and delete.
#[test]
fn challenge_multi_stage_crash_recovery_resilience() {
    let cluster = ChallengeCluster::new();
    let session_id = "sess-crash-recovery";

    // Helper to populate all 3 databases
    let populate = || {
        cluster
            .adk_store
            .upsert_session(session_id, "agent-cr", "{}")
            .expect("upsert session");
        cluster
            .session_store
            .upsert_session("jftrade", "local", session_id, "ACTIVE")
            .expect("upsert session");
        cluster
            .artifact_store
            .put_artifact(PutAdkArtifactParams {
                app_name: "jftrade",
                user_id: "local",
                session_id,
                file_name: "crash_test.json",
                version: 1,
                part_json: "{}",
                mime_type: "application/json",
                custom_metadata_json: None,
            })
            .expect("put artifact");
    };

    // Stage 1: Artifacts deleted, process crashes before session & adk deletion
    populate();
    let conn_artifact = Connection::open(&cluster.artifact_path).expect("open artifact");
    conn_artifact
        .execute(
            "DELETE FROM artifacts WHERE session_id = ?1",
            params![session_id],
        )
        .expect("simulate crash 1");

    // Recover via delete_session_cascade
    let recovered_1 = cluster
        .adk_store
        .delete_session_cascade(&cluster.session_store, &cluster.artifact_store, session_id)
        .expect("crash recovery 1");
    assert!(recovered_1);
    assert_eq!(
        audit_records(&cluster, session_id),
        SessionRecordAudit::default()
    );

    // Stage 2: Artifacts AND session DB deleted, process crashes before adk.db deletion
    populate();
    conn_artifact
        .execute(
            "DELETE FROM artifacts WHERE session_id = ?1",
            params![session_id],
        )
        .expect("simulate crash 2a");
    let conn_session = Connection::open(&cluster.session_path).expect("open session");
    conn_session
        .execute("DELETE FROM sessions WHERE id = ?1", params![session_id])
        .expect("simulate crash 2b");

    // Recover via delete_session_cascade
    let recovered_2 = cluster
        .adk_store
        .delete_session_cascade(&cluster.session_store, &cluster.artifact_store, session_id)
        .expect("crash recovery 2");
    assert!(recovered_2);
    assert_eq!(
        audit_records(&cluster, session_id),
        SessionRecordAudit::default()
    );
}
