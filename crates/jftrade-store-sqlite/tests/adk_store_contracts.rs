use std::fs;
use std::path::Path;

#[cfg(unix)]
use std::os::unix::fs::symlink;

use jftrade_store_sqlite::{ADK_TEST_CUTOVER_PROFILE, AdkStoreError, AdkTestCutoverStore};
use rusqlite::Connection;
use tempfile::tempdir;

fn seed_valid_go_adk_database(path: &Path) {
    let connection = Connection::open(path).expect("open sqlite");
    connection
        .execute_batch(
            "CREATE TABLE jftrade_schema_meta (
                component_id TEXT PRIMARY KEY,
                version INTEGER NOT NULL,
                created_at TEXT NOT NULL
            );
            INSERT INTO jftrade_schema_meta (component_id, version, created_at)
            VALUES ('adk', 4, '2026-08-20T00:00:00Z');

            CREATE TABLE adk_providers (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_agents (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_sessions (id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_runs (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, agent_id TEXT NOT NULL, status TEXT NOT NULL, client_request_id TEXT NOT NULL DEFAULT '', request_fingerprint TEXT NOT NULL DEFAULT '', payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_approvals (id TEXT PRIMARY KEY, run_id TEXT NOT NULL, agent_id TEXT NOT NULL, status TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_skills (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_audit_events (id TEXT PRIMARY KEY, kind TEXT NOT NULL, subject_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL);
            CREATE TABLE adk_optimization_tasks (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_tasks (id TEXT PRIMARY KEY, status TEXT NOT NULL, agent_id TEXT NOT NULL, run_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_memory (id TEXT PRIMARY KEY, agent_id TEXT NOT NULL, scope TEXT NOT NULL, memory_key TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_session_contexts (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_handoff_segments (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, active INTEGER NOT NULL, sequence_no INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, payload_json TEXT NOT NULL);
            CREATE TABLE adk_session_context_state (id TEXT PRIMARY KEY, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_session_notices (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, run_id TEXT NOT NULL, kind TEXT NOT NULL, status TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_session_composer_state (id TEXT PRIMARY KEY, session_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_workflows (id TEXT PRIMARY KEY, status TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_workflow_triggers (id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL, trigger_type TEXT NOT NULL, status TEXT NOT NULL, next_run_at TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_workflow_trigger_logs (id TEXT PRIMARY KEY, workflow_id TEXT NOT NULL, trigger_id TEXT NOT NULL, trigger_type TEXT NOT NULL, status TEXT NOT NULL, run_id TEXT NOT NULL, payload_json TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_run_leases (run_id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, fencing_token INTEGER NOT NULL, heartbeat_at_unix_ms INTEGER NOT NULL, expires_at_unix_ms INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
            CREATE TABLE adk_tool_invocations (run_id TEXT NOT NULL, idempotency_key TEXT NOT NULL, tool_name TEXT NOT NULL, status TEXT NOT NULL, owner_id TEXT NOT NULL, fencing_token INTEGER NOT NULL, run_lease_token INTEGER NOT NULL, input_json TEXT NOT NULL, output_json TEXT NOT NULL, lease_expires_at_unix_ms INTEGER NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY (run_id, idempotency_key));

            CREATE INDEX idx_adk_sessions_agent ON adk_sessions (agent_id, updated_at DESC);
            CREATE INDEX idx_adk_runs_session ON adk_runs (session_id, created_at DESC);
            CREATE UNIQUE INDEX idx_adk_runs_client_request ON adk_runs (client_request_id) WHERE client_request_id <> '';
            CREATE INDEX idx_adk_approvals_status ON adk_approvals (status, updated_at DESC);
            CREATE UNIQUE INDEX idx_adk_approvals_confirmation_call ON adk_approvals (json_extract(payload_json, '$.confirmationCallId')) WHERE COALESCE(json_extract(payload_json, '$.confirmationCallId'), '') <> '';
            CREATE INDEX idx_adk_audit_kind ON adk_audit_events (kind, created_at DESC);
            CREATE INDEX idx_adk_tasks_status ON adk_tasks (status, updated_at DESC);
            CREATE INDEX idx_adk_tasks_agent ON adk_tasks (agent_id, updated_at DESC);
            CREATE UNIQUE INDEX idx_adk_memory_agent_scope_key ON adk_memory (agent_id, scope, memory_key);
            CREATE INDEX idx_adk_session_contexts_updated ON adk_session_contexts (updated_at DESC);
            CREATE INDEX idx_adk_handoff_segments_session ON adk_handoff_segments (session_id, sequence_no ASC);
            CREATE INDEX idx_adk_session_context_state_updated ON adk_session_context_state (updated_at DESC);
            CREATE INDEX idx_adk_session_notices_session ON adk_session_notices (session_id, created_at ASC);
            CREATE INDEX idx_adk_workflows_status ON adk_workflows (status, updated_at DESC);
            CREATE INDEX idx_adk_workflow_triggers_workflow ON adk_workflow_triggers (workflow_id, updated_at DESC);
            CREATE INDEX idx_adk_workflow_triggers_due ON adk_workflow_triggers (trigger_type, status, next_run_at ASC);
            CREATE INDEX idx_adk_workflow_trigger_logs_workflow ON adk_workflow_trigger_logs (workflow_id, created_at DESC);
            CREATE INDEX idx_adk_workflow_trigger_logs_trigger ON adk_workflow_trigger_logs (trigger_id, created_at DESC);
            CREATE INDEX idx_adk_workflow_trigger_logs_status ON adk_workflow_trigger_logs (status, updated_at DESC);
            CREATE INDEX idx_adk_run_leases_expires ON adk_run_leases (expires_at_unix_ms ASC);
            CREATE INDEX idx_adk_tool_invocations_status ON adk_tool_invocations (status, lease_expires_at_unix_ms ASC);",
        )
        .expect("seed schema");
}

#[test]
fn adk_store_rejects_missing_drifted_and_corrupted_go_databases() {
    let directory = tempdir().expect("temp dir");
    let missing_path = directory.path().join("missing.db");

    let err = AdkTestCutoverStore::open_existing(&missing_path, ADK_TEST_CUTOVER_PROFILE)
        .expect_err("missing DB must fail closed");
    assert!(matches!(err, AdkStoreError::NotRegularFile(_)));

    let empty_path = directory.path().join("empty.db");
    fs::write(&empty_path, b"").expect("write empty");
    let err = AdkTestCutoverStore::open_existing(&empty_path, ADK_TEST_CUTOVER_PROFILE)
        .expect_err("empty DB must fail closed");
    assert!(matches!(err, AdkStoreError::Schema(_)));

    let drifted_path = directory.path().join("drifted.db");
    seed_valid_go_adk_database(&drifted_path);
    let connection = Connection::open(&drifted_path).expect("open sqlite");
    connection
        .execute_batch("CREATE TABLE rogue_table (id TEXT PRIMARY KEY);")
        .expect("create rogue");
    drop(connection);

    let err = AdkTestCutoverStore::open_existing(&drifted_path, ADK_TEST_CUTOVER_PROFILE)
        .expect_err("drifted DB must fail closed");
    assert!(matches!(err, AdkStoreError::Schema(_)));
}

#[cfg(unix)]
#[test]
fn adk_store_rejects_symlink_aliases() {
    let directory = tempdir().expect("temp dir");
    let target = directory.path().join("canonical.db");
    let alias = directory.path().join("alias.db");
    seed_valid_go_adk_database(&target);
    symlink(&target, &alias).expect("create database symlink");

    let err = AdkTestCutoverStore::open_existing(&alias, ADK_TEST_CUTOVER_PROFILE)
        .expect_err("symlink aliases must fail closed");
    assert!(matches!(err, AdkStoreError::NotRegularFile(_)));
}

#[test]
fn adk_store_lifecycle_and_restart_durability() {
    let directory = tempdir().expect("temp dir");
    let db_path = directory.path().join("adk.db");
    seed_valid_go_adk_database(&db_path);

    let store = AdkTestCutoverStore::open_existing(&db_path, ADK_TEST_CUTOVER_PROFILE)
        .expect("open valid store");

    // Provider lifecycle
    let provider = store
        .upsert_provider(
            "anthropic",
            r#"{"name":"Anthropic","model":"claude-3-7-sonnet"}"#,
        )
        .expect("upsert provider");
    assert_eq!(provider.id, "anthropic");

    let retrieved_provider = store
        .get_provider("anthropic")
        .expect("get provider")
        .expect("found");
    assert_eq!(retrieved_provider.id, "anthropic");

    // Agent lifecycle
    let agent = store
        .upsert_agent("analyst", r#"{"name":"Market Analyst","role":"analyst"}"#)
        .expect("upsert agent");
    assert_eq!(agent.id, "analyst");

    // Session lifecycle
    let session = store
        .upsert_session("sess-1", "analyst", r#"{"title":"Session 1"}"#)
        .expect("upsert session");
    assert_eq!(session.id, "sess-1");

    // Run lifecycle
    let run = store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-101",
            session_id: "sess-1",
            agent_id: "analyst",
            status: "running",
            client_request_id: "req-1",
            request_fingerprint: "fp-abc",
            payload_json: r#"{"goal":"Analyze HK.00700"}"#,
        })
        .expect("create run");
    assert_eq!(run.id, "run-101");
    assert_eq!(run.status, "running");

    let updated_run = store
        .update_run_status("run-101", "completed")
        .expect("update run");
    assert!(updated_run);

    // Approval lifecycle
    let approval = store
        .create_approval(
            "app-1",
            "run-101",
            "analyst",
            "pending",
            r#"{"action":"place_order","confirmationCallId":"call-1"}"#,
        )
        .expect("create approval");
    assert_eq!(approval.id, "app-1");
    assert_eq!(approval.status, "pending");

    let updated_app = store
        .update_approval_status("app-1", "approved")
        .expect("update approval");
    assert!(updated_app);

    // Memory lifecycle
    let mem = store
        .upsert_memory(
            "mem-1",
            "analyst",
            "session",
            "preferred_market",
            r#"{"value":"HK"}"#,
        )
        .expect("upsert memory");
    assert_eq!(mem.id, "mem-1");

    // Workflow lifecycle
    let wf = store
        .upsert_workflow("wf-1", "active", r#"{"name":"Daily Scan"}"#)
        .expect("upsert workflow");
    assert_eq!(wf.id, "wf-1");

    // Audit event
    store
        .record_audit_event("aud-1", "run_created", "run-101", r#"{"agent":"analyst"}"#)
        .expect("record audit");

    // Test second owner rejection while first store is open
    let err = AdkTestCutoverStore::open_existing(&db_path, ADK_TEST_CUTOVER_PROFILE)
        .expect_err("second writer lease must fail closed");
    assert!(matches!(err, AdkStoreError::WriterLease(_)));

    // Drop store to release writer lease
    drop(store);

    // Reopen store to verify restart durability
    let store2 = AdkTestCutoverStore::open_existing(&db_path, ADK_TEST_CUTOVER_PROFILE)
        .expect("reopen store");

    let provider2 = store2
        .get_provider("anthropic")
        .expect("get provider")
        .expect("found");
    assert_eq!(provider2.id, "anthropic");

    let agent2 = store2
        .get_agent("analyst")
        .expect("get agent")
        .expect("found");
    assert_eq!(agent2.id, "analyst");
}

#[test]
fn adk_workflow_and_trigger_mutations_use_timestamp_cas_and_atomic_delete() {
    let directory = tempdir().expect("temp dir");
    let db_path = directory.path().join("adk.db");
    seed_valid_go_adk_database(&db_path);
    let store = AdkTestCutoverStore::open_existing(&db_path, ADK_TEST_CUTOVER_PROFILE)
        .expect("open valid store");

    let workflow = store
        .upsert_workflow("wf-cas", "ENABLED", r#"{"name":"CAS workflow"}"#)
        .expect("create workflow");
    let trigger = store
        .upsert_workflow_trigger(
            "trigger-cas",
            "wf-cas",
            "manual",
            "ENABLED",
            "",
            r#"{"id":"trigger-cas","workflowId":"wf-cas","type":"manual","status":"ENABLED"}"#,
        )
        .expect("create trigger");

    assert!(
        !store
            .update_workflow_if_revision("wf-cas", "stale", "ENABLED", r#"{"name":"stale"}"#,)
            .expect("stale workflow CAS")
    );
    assert!(
        store
            .update_workflow_if_revision(
                "wf-cas",
                &workflow.updated_at,
                "ENABLED",
                r#"{"name":"updated"}"#,
            )
            .expect("workflow CAS")
    );
    let updated_workflow = store
        .get_workflow("wf-cas")
        .expect("get workflow")
        .expect("workflow exists");
    assert_eq!(updated_workflow.payload_json, r#"{"name":"updated"}"#);

    assert!(
        !store
            .update_workflow_trigger_if_revision(
                "trigger-cas",
                "stale",
                "wf-cas",
                "manual",
                "ENABLED",
                "",
                r#"{"status":"stale"}"#,
            )
            .expect("stale trigger CAS")
    );
    assert!(
        store
            .update_workflow_trigger_if_revision(
                "trigger-cas",
                &trigger.updated_at,
                "wf-cas",
                "manual",
                "ENABLED",
                "",
                r#"{"status":"updated"}"#,
            )
            .expect("trigger CAS")
    );

    let deleted_at = "2026-08-30T00:00:00Z";
    assert!(
        store
            .soft_delete_workflow_if_revision(
                "wf-cas",
                &updated_workflow.updated_at,
                r#"{"name":"updated","status":"DISABLED","deletedAt":"2026-08-30T00:00:00Z"}"#,
                deleted_at,
            )
            .expect("atomic workflow delete")
    );
    let deleted_workflow = store
        .get_workflow("wf-cas")
        .expect("get deleted workflow")
        .expect("deleted workflow exists");
    let deleted_trigger = store
        .get_workflow_trigger("trigger-cas")
        .expect("get deleted trigger")
        .expect("deleted trigger exists");
    assert_eq!(deleted_workflow.status, "DISABLED");
    assert_eq!(deleted_trigger.status, "DISABLED");
    assert!(deleted_trigger.payload_json.contains("deletedAt"));
}
