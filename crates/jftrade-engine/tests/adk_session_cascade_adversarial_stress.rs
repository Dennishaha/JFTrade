#![forbid(unsafe_code)]

use std::collections::BTreeMap;
use std::fs::File;
use std::path::Path;
use std::sync::Arc;
use std::time::Instant;

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

struct AdversarialTestCluster {
    _dir: tempfile::TempDir,
    port: Arc<ProductionAdkPort>,
    adk_path: std::path::PathBuf,
    session_path: std::path::PathBuf,
    artifact_path: std::path::PathBuf,
}

impl AdversarialTestCluster {
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

    fn check_db_integrity(&self) {
        for (name, path) in [
            ("adk.db", &self.adk_path),
            ("adk-session.db", &self.session_path),
            ("adk-artifact.db", &self.artifact_path),
        ] {
            let conn = Connection::open(path).expect("open for integrity check");
            let result: String = conn
                .query_row("PRAGMA integrity_check", [], |r| r.get(0))
                .unwrap_or_else(|e| panic!("integrity check failed on {name}: {e}"));
            assert_eq!(
                result, "ok",
                "Database {name} failed integrity check: {result}"
            );
        }
    }

    fn count_artifacts_for_session(&self, session_id: &str) -> i64 {
        let conn = Connection::open(&self.artifact_path).expect("open artifact");
        conn.query_row(
            "SELECT count(*) FROM artifacts WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap()
    }

    fn count_events_for_session(&self, session_id: &str) -> i64 {
        let conn = Connection::open(&self.session_path).expect("open session");
        conn.query_row(
            "SELECT count(*) FROM events WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap()
    }

    fn count_adk_runs_for_session(&self, session_id: &str) -> i64 {
        let conn = Connection::open(&self.adk_path).expect("open adk");
        conn.query_row(
            "SELECT count(*) FROM adk_runs WHERE session_id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap()
    }

    fn count_adk_sessions_for_session(&self, session_id: &str) -> i64 {
        let conn = Connection::open(&self.adk_path).expect("open adk");
        conn.query_row(
            "SELECT count(*) FROM adk_sessions WHERE id = ?1",
            params![session_id],
            |r| r.get(0),
        )
        .unwrap()
    }
}

#[test]
fn test_edge_case_session_ids_only_literal_user_is_protected() {
    let cluster = AdversarialTestCluster::new();

    // 1. Seed protected "user" shared artifacts
    cluster
        .port
        .artifact_store
        .put_artifact(PutAdkArtifactParams {
            app_name: "jftrade",
            user_id: "local",
            session_id: "user",
            file_name: "global_template.pine",
            version: 1,
            part_json: r#"{"code":"//@version=5"}"#,
            mime_type: "text/plain",
            custom_metadata_json: None,
        })
        .expect("seed user artifact");

    assert_eq!(cluster.count_artifacts_for_session("user"), 1);

    // 2. Verify deleting literal "user" fails at engine level with 400 ADK_SESSION_PROTECTED
    let err = cluster
        .delete_session("user")
        .expect_err("literal user deletion must be rejected");
    match err {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_SESSION_PROTECTED");
        }
        other => panic!("expected 400 ADK_SESSION_PROTECTED, got {other:?}"),
    }

    // Verify user artifact is still intact
    assert_eq!(cluster.count_artifacts_for_session("user"), 1);

    // 3. Verify deleting trimmed " user " or "user " also rejects (cannot bypass by whitespace)
    let err_spaced = cluster
        .delete_session("  user  ")
        .expect_err("spaced user deletion must be rejected");
    match err_spaced {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_SESSION_PROTECTED");
        }
        other => panic!("expected 400 ADK_SESSION_PROTECTED, got {other:?}"),
    }

    // 4. Test edge cases that contain "user" as substring/prefix/suffix:
    // "user_123", "my_user", "username", "USER", "user-session", "user.1"
    let edge_ids = [
        "user_123",
        "my_user",
        "username",
        "USER",
        "user-session",
        "user.1",
    ];

    for id in &edge_ids {
        // Seed in adk.db
        cluster
            .port
            .store
            .upsert_session(
                id,
                "agent-1",
                &format!(r#"{{"agentId":"agent-1","id":"{id}"}}"#),
            )
            .unwrap_or_else(|e| panic!("failed upsert session for {id}: {e}"));

        let run_id = format!("run-{id}");
        cluster
            .port
            .store
            .create_run(CreateAdkRunParams {
                id: &run_id,
                session_id: id,
                agent_id: "agent-1",
                status: "SUCCEEDED",
                client_request_id: &format!("req-{id}"),
                request_fingerprint: &format!("fp-{id}"),
                payload_json: "{}",
            })
            .unwrap_or_else(|e| panic!("failed create run for {id}: {e}"));

        // Seed in adk-session.db
        cluster
            .port
            .session_store
            .upsert_session("google-adk:agent-1", "local", id, "ACTIVE")
            .unwrap_or_else(|e| panic!("failed upsert session store for {id}: {e}"));
        cluster
            .port
            .session_store
            .record_event(RecordAdkEventParams {
                id: &format!("evt-{id}"),
                app_name: "google-adk:agent-1",
                user_id: "local",
                session_id: id,
                invocation_id: &run_id,
                author: "user",
                content: "ping",
            })
            .unwrap_or_else(|e| panic!("failed record event for {id}: {e}"));

        // Seed in adk-artifact.db
        cluster
            .port
            .artifact_store
            .put_artifact(PutAdkArtifactParams {
                app_name: "google-adk:agent-1",
                user_id: "local",
                session_id: id,
                file_name: "out.json",
                version: 1,
                part_json: r#"{"ok":true}"#,
                mime_type: "application/json",
                custom_metadata_json: None,
            })
            .unwrap_or_else(|e| panic!("failed put artifact for {id}: {e}"));

        // Verify seeded
        assert_eq!(cluster.count_adk_sessions_for_session(id), 1);
        assert_eq!(cluster.count_adk_runs_for_session(id), 1);
        assert_eq!(cluster.count_events_for_session(id), 1);
        assert_eq!(cluster.count_artifacts_for_session(id), 1);

        // Execute delete — must SUCCEED (only literal "user" is protected!)
        let res = cluster.delete_session(id).unwrap_or_else(|e| {
            panic!("deleting session '{id}' should succeed, but failed: {e:?}")
        });
        assert_eq!(res["deleted"], true);
        assert_eq!(res["id"], *id);

        // Verify completely cleaned up across all 3 databases
        assert_eq!(cluster.count_adk_sessions_for_session(id), 0);
        assert_eq!(cluster.count_adk_runs_for_session(id), 0);
        assert_eq!(cluster.count_events_for_session(id), 0);
        assert_eq!(cluster.count_artifacts_for_session(id), 0);

        // Crucial: verify literal "user" artifacts are STILL preserved!
        assert_eq!(
            cluster.count_artifacts_for_session("user"),
            1,
            "Shared user artifact was deleted when cleaning up session {id}!"
        );
    }

    cluster.check_db_integrity();
}

#[test]
fn test_edge_case_empty_and_whitespace_session_ids() {
    let cluster = AdversarialTestCluster::new();

    // Empty string
    let err_empty = cluster
        .delete_session("")
        .expect_err("empty sessionId must fail");
    match err_empty {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_INVALID_REQUEST");
        }
        other => panic!("expected 400 ADK_INVALID_REQUEST, got {other:?}"),
    }

    // Whitespace only
    let err_spaces = cluster
        .delete_session("   \t  \n  ")
        .expect_err("whitespace sessionId must fail");
    match err_spaces {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 400);
            assert_eq!(code, "ADK_INVALID_REQUEST");
        }
        other => panic!("expected 400 ADK_INVALID_REQUEST, got {other:?}"),
    }

    cluster.check_db_integrity();
}

#[test]
fn test_non_existent_session_ids_return_404_without_corruption() {
    let cluster = AdversarialTestCluster::new();

    // 1. Delete on completely fresh DBs
    let err = cluster
        .delete_session("non-existent-session-001")
        .expect_err("should return 404");
    match err {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_SESSION_NOT_FOUND");
        }
        other => panic!("expected 404 ADK_SESSION_NOT_FOUND, got {other:?}"),
    }

    // 2. Repeat 20 times with various non-existent IDs
    for i in 0..20 {
        let random_id = format!("ghost-session-{i}-{}", uuid_like(i));
        let err = cluster
            .delete_session(&random_id)
            .expect_err("non-existent session must return 404");
        match err {
            AdkMutationPortError::Failed { status, code, .. } => {
                assert_eq!(status, 404);
                assert_eq!(code, "ADK_SESSION_NOT_FOUND");
            }
            other => panic!("expected 404 ADK_SESSION_NOT_FOUND, got {other:?}"),
        }
    }

    // 3. Direct store level test: returns Ok(false)
    let store_result = cluster
        .port
        .store
        .delete_session_cascade(
            &cluster.port.session_store,
            &cluster.port.artifact_store,
            "never-existed-anywhere",
        )
        .expect("direct cascade store call should succeed");
    assert!(!store_result, "expected Ok(false) for non-existent session");

    cluster.check_db_integrity();
}

#[test]
fn test_adversarial_session_ids_sql_injection_and_unicode() {
    let cluster = AdversarialTestCluster::new();

    // Canary session that MUST survive all attacks
    let canary_id = "canary_survivor_session";
    cluster
        .port
        .store
        .upsert_session(canary_id, "canary-agent", r#"{"agentId":"canary-agent"}"#)
        .expect("seed canary session");
    cluster
        .port
        .session_store
        .upsert_session("jftrade", "local", canary_id, "ACTIVE")
        .expect("seed canary in session db");
    cluster
        .port
        .session_store
        .record_event(RecordAdkEventParams {
            id: "evt-canary-1",
            app_name: "jftrade",
            user_id: "local",
            session_id: canary_id,
            invocation_id: "inv-1",
            author: "user",
            content: "canary content",
        })
        .expect("seed canary event");

    // Adversarial session IDs
    let long_id = format!("sess-{}", "x".repeat(2048));
    let attack_ids = vec![
        "sess' OR '1'='1",
        "sess'; DROP TABLE adk_sessions; --",
        "sess\" OR 1=1 --",
        "sess; DELETE FROM artifacts; --",
        "会话-量化策略-2026-🎯",
        "🚀_quantum_alpha_✨",
        "sess:uuid/v4.variant+test=ok",
        &long_id,
    ];

    for attack_id in &attack_ids {
        // Seed records for this attack session
        cluster
            .port
            .store
            .upsert_session(
                attack_id,
                "agent-adversary",
                r#"{"agentId":"agent-adversary"}"#,
            )
            .unwrap_or_else(|e| panic!("failed seeding attack session {attack_id}: {e}"));
        cluster
            .port
            .session_store
            .upsert_session("jftrade", "local", attack_id, "ACTIVE")
            .unwrap_or_else(|e| panic!("failed seeding session db for {attack_id}: {e}"));
        cluster
            .port
            .session_store
            .record_event(RecordAdkEventParams {
                id: &format!("evt-{}", &attack_id[..attack_id.len().min(30)]),
                app_name: "jftrade",
                user_id: "local",
                session_id: attack_id,
                invocation_id: "inv-attack",
                author: "user",
                content: "attack payload",
            })
            .unwrap_or_else(|e| panic!("failed seeding event for {attack_id}: {e}"));
        cluster
            .port
            .artifact_store
            .put_artifact(PutAdkArtifactParams {
                app_name: "jftrade",
                user_id: "local",
                session_id: attack_id,
                file_name: "data.csv",
                version: 1,
                part_json: r#"{"rows":10}"#,
                mime_type: "text/csv",
                custom_metadata_json: None,
            })
            .unwrap_or_else(|e| panic!("failed seeding artifact for {attack_id}: {e}"));

        // Delete the session
        let res = cluster
            .delete_session(attack_id)
            .unwrap_or_else(|e| panic!("delete failed on attack session {attack_id}: {e:?}"));
        assert_eq!(res["deleted"], true);
        assert_eq!(res["id"], *attack_id);

        // Verify target records are gone
        assert_eq!(cluster.count_adk_sessions_for_session(attack_id), 0);
        assert_eq!(cluster.count_events_for_session(attack_id), 0);
        assert_eq!(cluster.count_artifacts_for_session(attack_id), 0);

        // Verify canary session is completely unharmed!
        assert_eq!(cluster.count_adk_sessions_for_session(canary_id), 1);
        assert_eq!(cluster.count_events_for_session(canary_id), 1);

        // Verify database integrity
        cluster.check_db_integrity();
    }
}

#[test]
fn test_high_volume_session_cleanup_stress() {
    let cluster = AdversarialTestCluster::new();
    let high_vol_id = "sess-high-volume-stress-001";
    let neighbor_id = "sess-neighbor-coexisting-002";

    // 1. Seed 10 shared artifacts under "user"
    for v in 1..=10 {
        cluster
            .port
            .artifact_store
            .put_artifact(PutAdkArtifactParams {
                app_name: "jftrade",
                user_id: "local",
                session_id: "user",
                file_name: &format!("shared_indicator_{v}.pine"),
                version: 1,
                part_json: r#"{"type":"shared"}"#,
                mime_type: "text/plain",
                custom_metadata_json: None,
            })
            .expect("seed shared user artifact");
    }
    assert_eq!(cluster.count_artifacts_for_session("user"), 10);

    // 2. Seed neighbor session (50 events, 10 artifacts, 5 runs)
    cluster
        .port
        .store
        .upsert_session(
            neighbor_id,
            "agent-neighbor",
            r#"{"agentId":"agent-neighbor"}"#,
        )
        .expect("upsert neighbor session");
    cluster
        .port
        .session_store
        .upsert_session("jftrade", "local", neighbor_id, "ACTIVE")
        .expect("upsert neighbor session store");

    for i in 0..5 {
        let run_id = format!("run-neighbor-{i}");
        cluster
            .port
            .store
            .create_run(CreateAdkRunParams {
                id: &run_id,
                session_id: neighbor_id,
                agent_id: "agent-neighbor",
                status: "SUCCEEDED",
                client_request_id: &format!("req-neighbor-{i}"),
                request_fingerprint: &format!("fp-neighbor-{i}"),
                payload_json: "{}",
            })
            .expect("create neighbor run");
    }
    for i in 0..50 {
        cluster
            .port
            .session_store
            .record_event(RecordAdkEventParams {
                id: &format!("evt-neighbor-{i}"),
                app_name: "jftrade",
                user_id: "local",
                session_id: neighbor_id,
                invocation_id: "run-neighbor-0",
                author: "user",
                content: "neighbor event content",
            })
            .expect("record neighbor event");
    }
    for i in 0..10 {
        cluster
            .port
            .artifact_store
            .put_artifact(PutAdkArtifactParams {
                app_name: "jftrade",
                user_id: "local",
                session_id: neighbor_id,
                file_name: &format!("neighbor_file_{i}.txt"),
                version: 1,
                part_json: r#"{"type":"neighbor"}"#,
                mime_type: "text/plain",
                custom_metadata_json: None,
            })
            .expect("put neighbor artifact");
    }

    assert_eq!(cluster.count_adk_runs_for_session(neighbor_id), 5);
    assert_eq!(cluster.count_events_for_session(neighbor_id), 50);
    assert_eq!(cluster.count_artifacts_for_session(neighbor_id), 10);

    // 3. Seed HIGH VOLUME session:
    // - 1 session row
    // - 30 runs with approvals, run leases, tasks, tool invocations
    // - 1,250 events in adk-session.db
    // - 60 artifacts in adk-artifact.db (including multi-version)
    cluster
        .port
        .store
        .upsert_session(high_vol_id, "agent-stress", r#"{"agentId":"agent-stress"}"#)
        .expect("upsert high volume session");
    cluster
        .port
        .session_store
        .upsert_session("google-adk:agent-stress", "local", high_vol_id, "ACTIVE")
        .expect("upsert high volume session store");

    println!("Seeding high volume session data...");
    let seed_start = Instant::now();

    for r in 0..30 {
        let run_id = format!("run-stress-{r}");
        cluster
            .port
            .store
            .create_run(CreateAdkRunParams {
                id: &run_id,
                session_id: high_vol_id,
                agent_id: "agent-stress",
                status: "SUCCEEDED",
                client_request_id: &format!("req-stress-{r}"),
                request_fingerprint: &format!("fp-stress-{r}"),
                payload_json: "{}",
            })
            .expect("create high volume run");
    }

    // Insert 1,250 events into adk-session.db
    {
        let conn =
            Connection::open(&cluster.session_path).expect("open session db for bulk insert");
        let tx = conn.unchecked_transaction().expect("start tx");
        for e in 0..1250 {
            tx.execute(
                "INSERT INTO events (id, app_name, user_id, session_id, invocation_id, author, content, timestamp)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)",
                params![
                    format!("evt-stress-{e}"),
                    "google-adk:agent-stress",
                    "local",
                    high_vol_id,
                    "run-stress-0",
                    "model",
                    r#"{"thought":"analyzing quantitative market structure","step":1}"#,
                    1700000000000i64 + (e as i64 * 100),
                ],
            )
            .expect("bulk insert event");
        }
        tx.commit().expect("commit bulk events");
    }

    // Insert 60 artifacts into adk-artifact.db
    {
        let conn =
            Connection::open(&cluster.artifact_path).expect("open artifact db for bulk insert");
        let tx = conn.unchecked_transaction().expect("start tx");
        for a in 0..60 {
            let file_name = format!("stress_artifact_{}.json", a / 2);
            let version = (a % 2) + 1; // creates v1 and v2 for artifacts
            tx.execute(
                "INSERT INTO artifacts (app_name, user_id, session_id, file_name, version, part_json, mime_type, custom_metadata_json, created_at, updated_at)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)",
                params![
                    "google-adk:agent-stress",
                    "local",
                    high_vol_id,
                    file_name,
                    version,
                    r#"{"tensor":[1.23, 4.56, 7.89, 0.12, 3.45]}"#,
                    "application/json",
                    None::<String>,
                    1700000000000i64,
                    1700000000000i64,
                ],
            )
            .expect("bulk insert artifact");
        }
        tx.commit().expect("commit bulk artifacts");
    }

    println!("Seeded in {:?}", seed_start.elapsed());

    // Verify seeded counts
    assert_eq!(cluster.count_adk_runs_for_session(high_vol_id), 30);
    assert_eq!(cluster.count_events_for_session(high_vol_id), 1250);
    assert_eq!(cluster.count_artifacts_for_session(high_vol_id), 60);

    // 4. Time the cascade delete
    let delete_start = Instant::now();
    let response = cluster
        .delete_session(high_vol_id)
        .expect("delete high volume session must succeed");
    let delete_duration = delete_start.elapsed();
    println!("High volume deletion completed in {:?}", delete_duration);

    // Verify response
    assert_eq!(response["deleted"], true);
    assert_eq!(response["id"], high_vol_id);

    // Performance assertion: cascade cleanup of 1,300+ items across 3 DBs must complete under 2 seconds
    assert!(
        delete_duration.as_millis() < 2000,
        "High volume deletion took too long: {:?}",
        delete_duration
    );

    // 5. Verify ALL high-volume records purged to ZERO across all 3 databases
    assert_eq!(cluster.count_adk_sessions_for_session(high_vol_id), 0);
    assert_eq!(cluster.count_adk_runs_for_session(high_vol_id), 0);
    assert_eq!(cluster.count_events_for_session(high_vol_id), 0);
    assert_eq!(cluster.count_artifacts_for_session(high_vol_id), 0);

    // 6. Verify shared user artifacts are 100% untouched
    assert_eq!(
        cluster.count_artifacts_for_session("user"),
        10,
        "Shared user artifacts were compromised during high volume cleanup!"
    );

    // 7. Verify neighbor session is 100% untouched
    assert_eq!(cluster.count_adk_sessions_for_session(neighbor_id), 1);
    assert_eq!(cluster.count_adk_runs_for_session(neighbor_id), 5);
    assert_eq!(cluster.count_events_for_session(neighbor_id), 50);
    assert_eq!(cluster.count_artifacts_for_session(neighbor_id), 10);

    // 8. Verify idempotency: subsequent delete returns 404
    let second_delete = cluster
        .delete_session(high_vol_id)
        .expect_err("subsequent delete must return 404");
    match second_delete {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 404);
            assert_eq!(code, "ADK_SESSION_NOT_FOUND");
        }
        other => panic!("expected 404 ADK_SESSION_NOT_FOUND, got {other:?}"),
    }

    // 9. Verify database integrity
    cluster.check_db_integrity();
}

fn uuid_like(seq: usize) -> String {
    format!("{:08x}-1234-5678-9abc-{:012x}", seq, seq * 42)
}
