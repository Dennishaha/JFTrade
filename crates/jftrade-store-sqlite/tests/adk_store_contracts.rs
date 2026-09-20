use std::fs;
use std::path::Path;
use std::sync::{Arc, Barrier};

#[cfg(unix)]
use std::os::unix::fs::symlink;

use jftrade_store_sqlite::{
    ADK_TEST_CUTOVER_PROFILE, AdkApprovalStage, AdkSessionStore, AdkStore, AdkStoreError,
    AdkTestCutoverStore, CreateAdkRunParams, initialize_current,
};
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

#[test]
fn adk_store_workflow_scheduler_due_and_threshold_triggers() {
    let directory = tempdir().expect("temp dir");
    let database_path = directory.path().join("adk.db");
    seed_valid_go_adk_database(&database_path);

    let store = AdkTestCutoverStore::open_existing(&database_path, ADK_TEST_CUTOVER_PROFILE)
        .expect("open store");

    store
        .upsert_workflow("wf-sched", "ENABLED", r#"{"name":"scheduler wf"}"#)
        .expect("upsert workflow");

    // Seed schedule triggers: 2 due, 1 future, 1 disabled, 1 soft-deleted
    store
        .upsert_workflow_trigger(
            "trig-due-1",
            "wf-sched",
            "schedule",
            "ENABLED",
            "2026-09-08T10:00:00Z",
            r#"{"id":"trig-due-1","workflowId":"wf-sched","config":{"cron":"0 * * * *"}}"#,
        )
        .expect("upsert trig-due-1");
    store
        .upsert_workflow_trigger(
            "trig-due-2",
            "wf-sched",
            "schedule",
            "ENABLED",
            "2026-09-08T11:00:00Z",
            r#"{"id":"trig-due-2","workflowId":"wf-sched","config":{"cron":"0 * * * *"}}"#,
        )
        .expect("upsert trig-due-2");
    store
        .upsert_workflow_trigger(
            "trig-future",
            "wf-sched",
            "schedule",
            "ENABLED",
            "2026-09-08T15:00:00Z",
            r#"{"id":"trig-future","workflowId":"wf-sched","config":{"cron":"0 * * * *"}}"#,
        )
        .expect("upsert trig-future");
    store
        .upsert_workflow_trigger(
            "trig-disabled",
            "wf-sched",
            "schedule",
            "DISABLED",
            "2026-09-08T09:00:00Z",
            r#"{"id":"trig-disabled","workflowId":"wf-sched","config":{"cron":"0 * * * *"}}"#,
        )
        .expect("upsert trig-disabled");
    store
        .upsert_workflow_trigger(
            "trig-deleted",
            "wf-sched",
            "schedule",
            "ENABLED",
            "2026-09-08T09:00:00Z",
            r#"{"id":"trig-deleted","workflowId":"wf-sched","deletedAt":"2026-09-08T09:30:00Z"}"#,
        )
        .expect("upsert trig-deleted");

    // Seed market threshold triggers: 1 enabled, 1 disabled
    store
        .upsert_workflow_trigger(
            "trig-thresh-1",
            "wf-sched",
            "market_threshold",
            "ENABLED",
            "",
            r#"{"id":"trig-thresh-1","workflowId":"wf-sched","config":{"instrumentIds":["US.AAPL"]}}"#,
        )
        .expect("upsert trig-thresh-1");
    store
        .upsert_workflow_trigger(
            "trig-thresh-disabled",
            "wf-sched",
            "market_threshold",
            "DISABLED",
            "",
            r#"{"id":"trig-thresh-disabled","workflowId":"wf-sched","config":{"instrumentIds":["US.MSFT"]}}"#,
        )
        .expect("upsert trig-thresh-disabled");

    // 1. Query due triggers with now = 12:00:00Z
    let due = store
        .list_due_workflow_schedule_triggers("2026-09-08T12:00:00Z", 10)
        .expect("list due triggers");
    assert_eq!(due.len(), 2);
    assert_eq!(due[0].id, "trig-due-1");
    assert_eq!(due[0].next_run_at, "2026-09-08T10:00:00Z");
    assert_eq!(due[1].id, "trig-due-2");
    assert_eq!(due[1].next_run_at, "2026-09-08T11:00:00Z");

    // 2. Query with limit 1
    let due_limited = store
        .list_due_workflow_schedule_triggers("2026-09-08T12:00:00Z", 1)
        .expect("list due triggers limit 1");
    assert_eq!(due_limited.len(), 1);
    assert_eq!(due_limited[0].id, "trig-due-1");

    // 3. Query enabled market threshold triggers
    let thresholds = store
        .list_enabled_workflow_triggers_by_type("market_threshold")
        .expect("list enabled threshold triggers");
    assert_eq!(thresholds.len(), 1);
    assert_eq!(thresholds[0].id, "trig-thresh-1");

    // 4. Update trigger run state for trig-due-1
    store
        .update_workflow_trigger_run_state(
            "trig-due-1",
            "2026-09-08T12:00:00Z",
            "2026-09-08T13:00:00Z",
            None,
        )
        .expect("update trigger run state");

    let updated = store
        .get_workflow_trigger("trig-due-1")
        .expect("get trigger")
        .expect("trigger exists");
    assert_eq!(updated.next_run_at, "2026-09-08T13:00:00Z");
    assert!(
        updated
            .payload_json
            .contains(r#""lastRunAt":"2026-09-08T12:00:00Z""#)
    );
    assert!(
        updated
            .payload_json
            .contains(r#""nextRunAt":"2026-09-08T13:00:00Z""#)
    );

    // After updating trig-due-1's next_run_at to 13:00:00Z, querying for <= 12:00:00Z returns only trig-due-2
    let due_after = store
        .list_due_workflow_schedule_triggers("2026-09-08T12:00:00Z", 10)
        .expect("list due triggers after update");
    assert_eq!(due_after.len(), 1);
    assert_eq!(due_after[0].id, "trig-due-2");

    // 5. Update with error
    store
        .update_workflow_trigger_run_state(
            "trig-due-2",
            "2026-09-08T12:00:00Z",
            "2026-09-08T13:00:00Z",
            Some("simulated failure"),
        )
        .expect("update with error");
    let updated_err = store
        .get_workflow_trigger("trig-due-2")
        .expect("get trigger")
        .expect("trigger exists");
    assert!(
        updated_err
            .payload_json
            .contains(r#""lastError":"simulated failure""#)
    );

    // 6. Update payload directly
    store
        .update_workflow_trigger_payload(
            "trig-thresh-1",
            r#"{"id":"trig-thresh-1","workflowId":"wf-sched","state":{"lastValues":{"US.AAPL":185.5}}}"#,
        )
        .expect("update payload");
    let updated_thresh = store
        .get_workflow_trigger("trig-thresh-1")
        .expect("get thresh trigger")
        .expect("thresh trigger exists");
    assert!(updated_thresh.payload_json.contains("185.5"));
}

#[test]
fn adk_approval_resolution_stages_continuation_and_denial_cas() {
    // Parity: go:452dea11:internal/assistant/engine/approval_retry_sibling_cancellation_test.go:8 TestSynchronousApprovalDenialCancelsSiblingActions
    let dir = tempdir().expect("tempdir");
    let path = dir.path().join("adk.db");
    seed_valid_go_adk_database(&path);

    let store = jftrade_store_sqlite::AdkStore::open_existing(&path, ADK_TEST_CUTOVER_PROFILE)
        .expect("store");

    // 1. Seed approval and associated run
    let run_payload = r#"{"id":"run-appr-1","sessionId":"sess-1","agentId":"agent-1","status":"PENDING","workflowStatus":"PENDING_APPROVAL","pendingApprovals":[{"id":"appr-1","status":"PENDING"}],"toolCalls":[{"id":"call-1","status":"PENDING_APPROVAL"}]}"#;
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-appr-1",
            session_id: "sess-1",
            agent_id: "agent-1",
            status: "PENDING",
            client_request_id: "req-appr-1",
            request_fingerprint: "fp-appr-1",
            payload_json: run_payload,
        })
        .expect("create run");

    let appr_payload =
        r#"{"id":"appr-1","runId":"run-appr-1","agentId":"agent-1","status":"PENDING"}"#;
    store
        .create_approval("appr-1", "run-appr-1", "agent-1", "PENDING", appr_payload)
        .expect("create approval");

    // 2. Resolve approval as DENIED
    let resolution = store
        .resolve_and_stage_approval("appr-1", "DENIED")
        .expect("resolve approval")
        .expect("resolution returned");

    assert!(resolution.changed);
    assert_eq!(resolution.approval.status, "DENIED");
    assert!(resolution.should_continue); // Background runner resumes to process termination
    let run = resolution.run.as_ref().expect("run returned");
    assert_eq!(run.status, "RUNNING");
    assert_eq!(
        resolution
            .run
            .as_ref()
            .and_then(|r| serde_json::from_str::<serde_json::Value>(&r.payload_json).ok())
            .and_then(|v| v.get("message").and_then(|m| m.as_str()).map(str::to_owned)),
        Some("审批已拒绝，正在后台结束运行。".to_owned())
    );

    let _run = resolution.run.expect("run returned");

    // 3. Repeated resolution is idempotent
    let second = store
        .resolve_and_stage_approval("appr-1", "DENIED")
        .expect("idempotent resolve")
        .expect("resolution returned");
    assert!(!second.changed);
    assert_eq!(second.approval.status, "DENIED");
}

/// Parity: go:452dea11:internal/assistant/engine/persistence/provider_selection_test.go:8
/// TestNormalizeDefaultProviderSelection and
/// go:452dea11:internal/assistant/engine/persistence/provider_selection_test.go:29
/// TestSortProvidersDefaultFirst.
///
/// Go's `StoreCore.ListProviders` loads rows `created_at ASC, id ASC`, repairs
/// the default selection so a non-empty table always has exactly one default,
/// persists that repair, and then lists the default provider first.  The Rust
/// port originally returned `created_at DESC` and never repaired anything, so a
/// provider table written without the `default` flag left chat reporting
/// "default agent provider is not configured" instead of falling back to the
/// first provider.
#[test]
fn provider_list_normalizes_default_selection_and_orders_default_first() {
    let directory = tempdir().expect("temp dir");
    let db_path = directory.path().join("adk.db");
    seed_valid_go_adk_database(&db_path);
    let store = AdkTestCutoverStore::open_existing(&db_path, ADK_TEST_CUTOVER_PROFILE)
        .expect("open valid store");

    store
        .upsert_provider(
            "provider-a",
            r#"{"displayName":"A","baseUrl":"https://a.example/v1","model":"m-a"}"#,
        )
        .expect("upsert provider-a");
    store
        .upsert_provider(
            "provider-b",
            r#"{"displayName":"B","baseUrl":"https://b.example/v1","model":"m-b"}"#,
        )
        .expect("upsert provider-b");

    // A legacy/corrupt table: no row carries the flag at all.
    let connection = Connection::open(&db_path).expect("open sqlite");
    connection
        .execute(
            "UPDATE adk_providers SET payload_json = json_remove(payload_json, '$.default')",
            [],
        )
        .expect("strip default flags");

    let listed = store.list_providers().expect("list providers");
    let ids = listed.iter().map(|row| row.id.as_str()).collect::<Vec<_>>();
    assert_eq!(
        ids,
        vec!["provider-a", "provider-b"],
        "Go lists created_at ASC and puts the repaired default first"
    );
    let defaults = listed
        .iter()
        .map(|row| {
            let value: serde_json::Value =
                serde_json::from_str(&row.payload_json).expect("provider payload");
            (
                row.id.clone(),
                value
                    .get("default")
                    .and_then(serde_json::Value::as_bool)
                    .unwrap_or(false),
            )
        })
        .collect::<Vec<_>>();
    assert_eq!(
        defaults,
        vec![
            ("provider-a".to_owned(), true),
            ("provider-b".to_owned(), false)
        ],
        "exactly one provider is repaired as the default"
    );

    // The repair is durable, so a fresh handle observes the same selection and
    // a second read does not flip it again.
    drop(store);
    let reopened = AdkTestCutoverStore::open_existing(&db_path, ADK_TEST_CUTOVER_PROFILE)
        .expect("reopen store");
    let again = reopened.list_providers().expect("list providers again");
    assert_eq!(again[0].id, "provider-a");
    let payload: serde_json::Value =
        serde_json::from_str(&again[0].payload_json).expect("provider payload");
    assert_eq!(payload["default"], serde_json::json!(true));

    // A duplicate-default table collapses to the first default, matching
    // `NormalizeDefaultProviderSelection`.
    connection
        .execute(
            "UPDATE adk_providers SET payload_json = json_set(payload_json, '$.default', 1)",
            [],
        )
        .expect("set every provider default");
    let collapsed = reopened
        .list_providers()
        .expect("list after duplicate default");
    let collapsed_defaults = collapsed
        .iter()
        .map(|row| {
            let value: serde_json::Value =
                serde_json::from_str(&row.payload_json).expect("provider payload");
            (
                row.id.clone(),
                value
                    .get("default")
                    .and_then(serde_json::Value::as_bool)
                    .unwrap_or(false),
            )
        })
        .collect::<Vec<_>>();
    assert_eq!(
        collapsed_defaults,
        vec![
            ("provider-a".to_owned(), true),
            ("provider-b".to_owned(), false)
        ],
        "only the first default survives normalization"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/store_lifecycle_test.go:45
/// TestProvidersMaintainDefaultSelectionAndCreatedOrder.
///
/// The reference lists providers with the selected default first, promotes a
/// replacement atomically when the default is deleted, and keeps exactly one
/// default row afterwards.
#[test]
fn provider_delete_promotes_the_replacement_and_keeps_one_default() {
    let directory = tempdir().expect("temp dir");
    let db_path = directory.path().join("adk.db");
    seed_valid_go_adk_database(&db_path);
    let store = AdkTestCutoverStore::open_existing(&db_path, ADK_TEST_CUTOVER_PROFILE)
        .expect("open valid store");

    store
        .upsert_provider(
            "provider-oldest",
            r#"{"displayName":"Oldest","baseUrl":"https://a.example/v1","model":"m-a","enabled":true,"default":false}"#,
        )
        .expect("upsert provider-oldest");
    store
        .upsert_provider(
            "provider-middle",
            r#"{"displayName":"Middle","baseUrl":"https://m.example/v1","model":"m-m","enabled":true,"default":false}"#,
        )
        .expect("upsert provider-middle");
    store
        .upsert_provider(
            "provider-newer",
            r#"{"displayName":"Newer","baseUrl":"https://b.example/v1","model":"m-b","enabled":true,"default":true}"#,
        )
        .expect("upsert provider-newer");

    let listed = store.list_providers().expect("list providers");
    let ids = listed.iter().map(|row| row.id.as_str()).collect::<Vec<_>>();
    assert_eq!(
        ids,
        vec!["provider-newer", "provider-oldest", "provider-middle"],
        "the selected default is listed before the created_at order"
    );

    let replacement = r#"{"displayName":"Middle Promoted","baseUrl":"https://m.example/v1","model":"m-m","enabled":true,"default":true}"#;
    assert!(
        store
            .delete_provider_with_replacement_atomic(
                "provider-newer",
                Some(("provider-middle", replacement)),
            )
            .expect("delete the default provider"),
        "the deleted provider existed"
    );

    let promoted = store.list_providers().expect("list providers after delete");
    assert_eq!(promoted.len(), 2, "the deleted default is gone");
    assert_eq!(
        promoted[0].id, "provider-middle",
        "the requested replacement is promoted and listed first"
    );
    let promoted_default: serde_json::Value =
        serde_json::from_str(&promoted[0].payload_json).expect("provider payload");
    assert_eq!(
        promoted_default["displayName"], "Middle Promoted",
        "the replacement payload is written atomically: {promoted_default}"
    );
    assert_eq!(
        promoted_default["default"],
        serde_json::Value::Bool(true),
        "the replacement carries the sole default flag: {promoted_default}"
    );
    let other: serde_json::Value =
        serde_json::from_str(&promoted[1].payload_json).expect("provider payload");
    assert_eq!(
        other["default"],
        serde_json::Value::Bool(false),
        "the untouched provider keeps its flag cleared: {other}"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:452
/// TestStoreResolvePendingApprovalMissingAndIdempotent.
///
/// The reference store answers a missing approval id with the zero-value
/// approval plus `changed=false` instead of an error, and a resolve against a
/// row that left `PENDING` is a pure no-op: the first terminal verdict wins and
/// the stored status is not overwritten.  The Rust store already modelled the
/// first half after the CAS rewrite; this test freezes both halves so a future
/// "resolve always overwrites" regression fails here.
#[test]
fn adk_approval_resolution_missing_and_non_pending_rows_are_idempotent() {
    let dir = tempdir().expect("tempdir");
    let path = dir.path().join("adk.db");
    seed_valid_go_adk_database(&path);
    let store = jftrade_store_sqlite::AdkStore::open_existing(&path, ADK_TEST_CUTOVER_PROFILE)
        .expect("store");

    // A missing approval is not an error and reports no change.
    let missing = store
        .resolve_and_stage_approval("approval-missing", "APPROVED")
        .expect("missing approval resolve");
    assert!(
        missing.is_none(),
        "a missing approval resolves to no row instead of an error: {missing:?}"
    );

    // Seed an approval that is already terminal.  Resolving it in the other
    // direction must neither flip the status nor report a change.
    // `create_approval` fences on the owning run, so the terminal fixture needs
    // its run row first.
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-1",
            session_id: "sess-1",
            agent_id: "agent-1",
            status: "RUNNING",
            client_request_id: "req-1",
            request_fingerprint: "fp-1",
            payload_json:
                r#"{"id":"run-1","sessionId":"sess-1","agentId":"agent-1","status":"RUNNING"}"#,
        })
        .expect("create run-1");
    store
        .create_approval(
            "approval-approved",
            "run-1",
            "agent-1",
            "APPROVED",
            r#"{"id":"approval-approved","runId":"run-1","agentId":"agent-1","status":"APPROVED"}"#,
        )
        .expect("seed approved approval");
    let resolved = store
        .resolve_and_stage_approval("approval-approved", "DENIED")
        .expect("resolve approved approval")
        .expect("existing approval resolves");
    assert!(
        !resolved.changed,
        "a non-pending approval never reports changed=true"
    );
    assert_eq!(
        resolved.approval.status, "APPROVED",
        "the first terminal verdict stays authoritative"
    );
    let reread = store
        .list_approvals()
        .expect("list approvals")
        .into_iter()
        .find(|row| row.id == "approval-approved")
        .expect("approval row");
    assert_eq!(
        reread.status, "APPROVED",
        "the durable row keeps the original verdict"
    );

    // An approval whose run already reached `PENDING` only stages the run once;
    // a second resolution of the same row stays a no-op.
    let run_payload = r#"{"id":"run-pending","sessionId":"sess-pending","agentId":"agent-1","status":"PENDING","pendingApprovals":[{"id":"approval-pending","status":"PENDING","functionCallId":"call-pending","confirmationCallId":"call-pending:confirmation"}],"toolCalls":[{"id":"call-pending","status":"PENDING_APPROVAL","requiresUser":true}]}"#;
    store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-pending",
            session_id: "sess-pending",
            agent_id: "agent-1",
            status: "PENDING",
            client_request_id: "req-pending",
            request_fingerprint: "fp-pending",
            payload_json: run_payload,
        })
        .expect("create pending run");
    store
        .create_approval(
            "approval-pending",
            "run-pending",
            "agent-1",
            "PENDING",
            r#"{"id":"approval-pending","runId":"run-pending","agentId":"agent-1","status":"PENDING","functionCallId":"call-pending","confirmationCallId":"call-pending:confirmation"}"#,
        )
        .expect("seed pending approval");

    let first = store
        .resolve_and_stage_approval("approval-pending", "APPROVED")
        .expect("first resolve")
        .expect("pending approval resolves");
    assert!(first.changed, "the first resolution changes the row");
    assert!(
        first.should_continue,
        "the released tool call asks the runtime to continue"
    );
    let second = store
        .resolve_and_stage_approval("approval-pending", "DENIED")
        .expect("duplicate resolve")
        .expect("duplicate resolution returns the row");
    assert!(
        !second.changed,
        "a duplicate resolution is a no-op: {second:?}"
    );
    assert_eq!(second.approval.status, "APPROVED");
    assert!(
        !second.should_continue,
        "a no-op resolution must not stage a second continuation"
    );
}

/// Go's `ResolveAndStageApproval` is idempotent against a *stale embedded*
/// approval: when the durable approval row was already resolved but the run
/// payload still embeds it as `PENDING`, the retry must not flip the verdict
/// and must re-stage the run so the released call can continue.
#[test]
fn adk_approval_resolution_restages_a_stale_embedded_approval() {
    // Parity: go:452dea11:internal/assistant/engine/store_test.go:538
    // TestIdempotentApprovalRecoversPendingRunWithStaleEmbeddedApproval
    let directory = tempdir().expect("temp dir");
    let db_path = directory.path().join("adk.db");
    seed_valid_go_adk_database(&db_path);
    let store = AdkStore::open_existing(&db_path, ADK_TEST_CUTOVER_PROFILE).expect("open store");
    store
        .create_run(CreateAdkRunParams {
            id: "run-stale-approval",
            session_id: "session-stale-approval",
            agent_id: "agent-stale-approval",
            status: "PENDING",
            client_request_id: "request-stale-approval",
            request_fingerprint: "fingerprint-stale-approval",
            payload_json: r#"{"id":"run-stale-approval","sessionId":"session-stale-approval","agentId":"agent-stale-approval","status":"PENDING","pendingApprovals":[{"id":"approval-stale","status":"PENDING","toolName":"strategy.research_backtest"}],"toolCalls":[{"id":"call-stale","name":"strategy.research_backtest","status":"PENDING_APPROVAL","requiresUser":true}]}"#,
        })
        .expect("create pending run");
    // The durable verdict landed without staging the run (the state a crashed
    // or externally repaired database can hold).
    store
        .create_approval(
            "approval-stale",
            "run-stale-approval",
            "agent-stale-approval",
            "APPROVED",
            r#"{"id":"approval-stale","runId":"run-stale-approval","agentId":"agent-stale-approval","toolName":"strategy.research_backtest","status":"APPROVED"}"#,
        )
        .expect("seed stale resolved approval");

    let resolution = store
        .resolve_and_stage_approval("approval-stale", "APPROVED")
        .expect("idempotent retry")
        .expect("the approval row resolves");
    assert!(
        !resolution.changed,
        "an already-resolved approval never reports changed=true"
    );
    assert_eq!(resolution.approval.status, "APPROVED");
    assert!(
        resolution.should_continue,
        "the retry must re-stage the run for the released call: {resolution:?}"
    );
    let run = resolution.run.as_ref().expect("staged run");
    assert_eq!(
        run.status, "RUNNING",
        "the re-staged run leaves the approval wait"
    );
    let payload: serde_json::Value = serde_json::from_str(&run.payload_json).expect("run payload");
    assert_eq!(payload["pendingApprovals"][0]["status"], "APPROVED");
    let stored = store
        .get_run("run-stale-approval")
        .expect("read run")
        .expect("run row");
    assert_eq!(stored.status, "RUNNING");
}

/// Go opens two SQLite handles per ADK store: an eight-connection read pool
/// plus a single-connection write pool (`Store.DB().Stats()` /
/// `Store.DB().WriteStats()`).  The Rust port keeps one mutex-guarded
/// connection behind a process-wide `WriterLease`; the invariant that matters
/// is unchanged: exactly one writer owns the file, and concurrent readers
/// never observe a torn or lost write.
#[test]
fn adk_store_fences_a_second_writer_and_serializes_concurrent_access() {
    // Parity: go:452dea11:internal/assistant/engine/store_test.go:59
    // TestNewStoreUsesSeparatedConcurrentReadAndSingleWritePools
    let directory = tempdir().expect("temp dir");
    let db_path = directory.path().join("adk.db");
    seed_valid_go_adk_database(&db_path);
    let store = Arc::new(
        AdkStore::open_existing(&db_path, ADK_TEST_CUTOVER_PROFILE).expect("open valid store"),
    );

    // A second owner for the same database fails closed instead of joining the
    // write path.
    let second = AdkStore::open_existing(&db_path, ADK_TEST_CUTOVER_PROFILE)
        .expect_err("a second writer must be fenced");
    let message = second.to_string();
    assert!(
        message.contains("writer lease is already held"),
        "unexpected second-writer error: {message}"
    );

    // Concurrent readers/writers all commit: the guarded connection serializes
    // them instead of interleaving partial writes.
    const WORKERS: usize = 8;
    let barrier = Arc::new(Barrier::new(WORKERS));
    let mut handles = Vec::new();
    for index in 0..WORKERS {
        let store = Arc::clone(&store);
        let barrier = Arc::clone(&barrier);
        handles.push(std::thread::spawn(move || {
            let id = format!("session-concurrent-{index}");
            let payload = format!(r#"{{"id":"{id}","agentId":"agent-1"}}"#);
            barrier.wait();
            store
                .upsert_session(&id, "agent-1", &payload)
                .expect("concurrent upsert");
            store
                .get_session(&id)
                .expect("concurrent read")
                .expect("session row")
                .id
        }));
    }
    let mut written: Vec<String> = handles
        .into_iter()
        .map(|handle| handle.join().expect("join concurrent worker"))
        .collect();
    written.sort();
    assert_eq!(written.len(), WORKERS);
    assert_eq!(written[0], "session-concurrent-0");
    assert_eq!(written[WORKERS - 1], "session-concurrent-7");

    let connection = Connection::open(&db_path).expect("open sqlite");
    let stored: i64 = connection
        .query_row(
            "SELECT COUNT(*) FROM adk_sessions WHERE id LIKE 'session-concurrent-%'",
            [],
            |row| row.get(0),
        )
        .expect("count concurrent sessions");
    assert_eq!(stored, WORKERS as i64, "no concurrent write may be lost");
}

/// Go's `NewStore` refuses a database without ADK schema metadata and leaves
/// the file untouched so the operator can inspect or repair it.
#[test]
fn adk_store_rejects_a_legacy_database_without_rewriting_it() {
    // Parity: go:452dea11:internal/assistant/engine/store_test.go:254
    // TestNewStoreRejectsLegacyDatabaseWithoutMutatingIt
    let directory = tempdir().expect("temp dir");
    let db_path = directory.path().join("legacy.db");
    let connection = Connection::open(&db_path).expect("create legacy db");
    connection
        .execute_batch(
            "CREATE TABLE legacy_data (id TEXT PRIMARY KEY, value TEXT NOT NULL);
             INSERT INTO legacy_data (id, value) VALUES ('keep', 'untouched');",
        )
        .expect("seed legacy database");
    drop(connection);
    let before = fs::read(&db_path).expect("read legacy bytes");

    let error = AdkStore::open_existing(&db_path, ADK_TEST_CUTOVER_PROFILE)
        .expect_err("a legacy database must be rejected");
    assert!(
        error.to_string().contains("schema metadata is missing"),
        "unexpected legacy error: {error}"
    );

    assert_eq!(
        fs::read(&db_path).expect("re-read legacy bytes"),
        before,
        "a rejected legacy database must not be modified"
    );
    let connection = Connection::open(&db_path).expect("reopen legacy db");
    let value: String = connection
        .query_row(
            "SELECT value FROM legacy_data WHERE id = 'keep'",
            [],
            |row| row.get(0),
        )
        .expect("legacy row survives");
    assert_eq!(value, "untouched");
    let metadata_tables: i64 = connection
        .query_row(
            "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='jftrade_schema_meta'",
            [],
            |row| row.get(0),
        )
        .expect("inspect metadata table");
    assert_eq!(
        metadata_tables, 0,
        "the rejected database must not gain schema metadata"
    );
}

/// Go's ADK store dropped the pre-ADK `adk_messages` / `adk_transcript_entries`
/// tables; transcripts live in the ADK session database, so opening a store
/// must never recreate them.
#[test]
fn adk_store_schema_has_no_legacy_message_tables() {
    // Parity: go:452dea11:internal/assistant/engine/store_test.go:373
    // TestNewStoreDropsLegacyMessageTables
    let directory = tempdir().expect("temp dir");
    let db_path = directory.path().join("adk.db");
    seed_valid_go_adk_database(&db_path);
    let _store =
        AdkStore::open_existing(&db_path, ADK_TEST_CUTOVER_PROFILE).expect("open valid store");

    let connection = Connection::open(&db_path).expect("open sqlite");
    let legacy: i64 = connection
        .query_row(
            "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'
             AND name IN ('adk_messages', 'adk_transcript_entries')",
            [],
            |row| row.get(0),
        )
        .expect("look up legacy tables");
    assert_eq!(legacy, 0, "legacy message tables must stay dropped");
}

/// Go's `SaveApprovalIfConfirmationAbsent` lets 24 racing callers share one
/// confirmation: exactly one caller creates the approval row and the rest reuse
/// it.  Rust stages approvals inside the run's status/revision CAS, so the same
/// guarantee holds through a different mechanism: one staging attempt wins the
/// revision, the remaining attempts observe the stale revision and stage
/// nothing, and the confirmation index still rejects any duplicate row.
#[test]
fn adk_tool_call_staging_admits_one_confirmation_winner_under_concurrency() {
    // Parity: go:452dea11:internal/assistant/engine/store_test.go:295
    // TestSaveApprovalIfConfirmationAbsentIsConcurrentIdempotent
    const WORKERS: usize = 24;
    let directory = tempdir().expect("temp dir");
    let db_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    seed_valid_go_adk_database(&db_path);
    initialize_current(
        &Connection::open(&session_path).expect("create session db"),
        "adk-session",
    )
    .expect("initialize session schema");
    let store = Arc::new(
        AdkStore::open_existing(&db_path, ADK_TEST_CUTOVER_PROFILE).expect("open valid store"),
    );
    let session_store = Arc::new(AdkSessionStore::open(&session_path).expect("open session store"));
    store
        .upsert_session("session-concurrent", "agent", "{}")
        .expect("seed session");
    store
        .create_run(CreateAdkRunParams {
            id: "run-concurrent-owner",
            session_id: "session-concurrent",
            agent_id: "agent",
            status: "RUNNING",
            client_request_id: "request-concurrent-owner",
            request_fingerprint: "fingerprint-concurrent-owner",
            payload_json: r#"{"id":"run-concurrent-owner","sessionId":"session-concurrent","agentId":"agent","status":"RUNNING","pendingApprovals":[],"toolCalls":[]}"#,
        })
        .expect("seed running run");
    let revision = store
        .get_run("run-concurrent-owner")
        .expect("read seeded run")
        .expect("run row")
        .updated_at;

    let barrier = Arc::new(Barrier::new(WORKERS));
    let mut handles = Vec::new();
    for index in 0..WORKERS {
        let store = Arc::clone(&store);
        let session_store = Arc::clone(&session_store);
        let barrier = Arc::clone(&barrier);
        let revision = revision.clone();
        handles.push(std::thread::spawn(move || {
            let approval_id = format!("approval-concurrent-{index}");
            let approval_payload = format!(
                r#"{{"id":"{approval_id}","runId":"run-concurrent-owner","agentId":"agent","toolName":"strategy.research_backtest","status":"PENDING","functionCallId":"function-concurrent","confirmationCallId":"confirmation-concurrent"}}"#
            );
            let run_payload = format!(
                r#"{{"id":"run-concurrent-owner","sessionId":"session-concurrent","agentId":"agent","status":"PENDING","pendingApprovals":[{{"id":"{approval_id}","toolName":"strategy.research_backtest","status":"PENDING","confirmationCallId":"confirmation-concurrent"}}]}}"#
            );
            let approval = AdkApprovalStage {
                id: &approval_id,
                run_id: "run-concurrent-owner",
                agent_id: "agent",
                payload_json: &approval_payload,
            };
            barrier.wait();
            store
                .stage_tool_calls_if_status_and_revision_with_events(
                    "run-concurrent-owner",
                    "RUNNING",
                    &revision,
                    "PENDING",
                    &run_payload,
                    &[approval],
                    session_store.as_ref(),
                    &[],
                )
                .expect("a losing staging attempt must report the stale revision")
        }));
    }
    let staged: Vec<bool> = handles
        .into_iter()
        .map(|handle| handle.join().expect("join staging worker"))
        .collect();
    assert_eq!(
        staged.iter().filter(|won| **won).count(),
        1,
        "exactly one staging attempt may create the approval: {staged:?}"
    );

    let connection = Connection::open(&db_path).expect("open sqlite");
    let approvals: i64 = connection
        .query_row("SELECT COUNT(*) FROM adk_approvals", [], |row| row.get(0))
        .expect("count approvals");
    assert_eq!(approvals, 1, "the confirmation must own exactly one row");
    let confirmations: i64 = connection
        .query_row(
            "SELECT COUNT(*) FROM adk_approvals
             WHERE json_extract(payload_json, '$.confirmationCallId') = 'confirmation-concurrent'",
            [],
            |row| row.get(0),
        )
        .expect("count confirmations");
    assert_eq!(confirmations, 1);

    // The canonical row is the one the runtime resumes: the store lists exactly
    // that approval and resolving it still stages the continuation.
    let listed = store.list_approvals().expect("list approvals");
    assert_eq!(listed.len(), 1, "{listed:?}");
    let resolved = store
        .resolve_and_stage_approval(&listed[0].id, "APPROVED")
        .expect("resolve the canonical approval")
        .expect("resolution returned");
    assert!(resolved.changed);
}

/// Parity: go:452dea11:internal/assistant/engine/adk_edges_test.go:209
/// `TestSQLiteGormPoolBoundaryBranches`: Rust has no gorm pool wrapper — the
/// store owns its single connection and transaction — so the reachable contract
/// is that a run and its initial session event commit together, a rejected
/// event rolls the run back, a stale CAS update is a no-op, and a missing table
/// surfaces as a store error instead of a panic.
#[test]
fn adk_transaction_boundaries_commit_roll_back_and_report_missing_tables() {
    let directory = tempdir().expect("temp dir");
    let db_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    let adk_connection = Connection::open(&db_path).expect("open adk sqlite");
    initialize_current(&adk_connection, "adk").expect("initialize adk schema");
    drop(adk_connection);
    let session_connection = Connection::open(&session_path).expect("open session sqlite");
    initialize_current(&session_connection, "adk-session").expect("initialize session schema");
    drop(session_connection);

    let store = AdkStore::open(&db_path).expect("open store");
    let session_store = AdkSessionStore::open(&session_path).expect("open session store");
    session_store
        .upsert_session("jftrade", "local", "session-tx", "{}")
        .expect("seed session");

    fn run_params(id: &str) -> CreateAdkRunParams<'_> {
        CreateAdkRunParams {
            id,
            session_id: "session-tx",
            agent_id: "agent-tx",
            status: "RUNNING",
            client_request_id: "",
            request_fingerprint: "",
            payload_json: r#"{"id":"run-tx","status":"RUNNING"}"#,
        }
    }
    // The store requires the event's invocation to be the owning run.
    fn run_event<'a>(id: &'a str, run_id: &'a str) -> jftrade_store_sqlite::AdkRunEvent<'a> {
        jftrade_store_sqlite::AdkRunEvent {
            id,
            session_id: "session-tx",
            invocation_id: run_id,
            author: "user",
            content: "hello",
        }
    }

    store
        .create_run_with_event(
            run_params("run-tx-committed"),
            &session_store,
            &run_event("event-tx-shared", "run-tx-committed"),
        )
        .expect("the run and its first event commit together");
    assert!(
        store
            .get_run("run-tx-committed")
            .expect("read run")
            .is_some()
    );

    // The event insert fails on the primary key, so the run insert must roll
    // back with it.
    let rolled_back = store.create_run_with_event(
        run_params("run-tx-rolled-back"),
        &session_store,
        &run_event("event-tx-shared", "run-tx-rolled-back"),
    );
    assert!(
        rolled_back.is_err(),
        "a duplicate event must reject the transaction"
    );
    assert!(
        store
            .get_run("run-tx-rolled-back")
            .expect("read rolled back run")
            .is_none(),
        "the run insert must roll back with the rejected event"
    );

    // A stale revision is a no-op, not a partial write.
    assert!(
        !store
            .update_run_state_if_status_and_revision(
                "run-tx-committed",
                "RUNNING",
                "stale-revision",
                "PAUSED",
                r#"{"id":"run-tx-committed","status":"PAUSED"}"#,
            )
            .expect("stale CAS update")
    );
    assert_eq!(
        store
            .get_run("run-tx-committed")
            .expect("read run")
            .expect("row")
            .status,
        "RUNNING"
    );

    // A schema that disappeared is reported, never panicked.
    let connection = Connection::open(&db_path).expect("reopen adk sqlite");
    connection
        .execute("DROP TABLE adk_runs", [])
        .expect("drop runs table");
    drop(connection);
    assert!(store.get_run("run-tx-committed").is_err());
    assert!(
        store
            .create_run_with_event(
                run_params("run-tx-missing-table"),
                &session_store,
                &run_event("event-tx-second", "run-tx-missing-table")
            )
            .is_err()
    );
}
