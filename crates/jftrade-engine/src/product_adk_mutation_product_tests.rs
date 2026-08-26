use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationPort, AdkMutationPortError,
};
use super::*;

#[derive(Debug)]
struct FixtureAdkMutationPort;

impl AdkMutationPort for FixtureAdkMutationPort {
    fn mutate(&self, input: &AdkMutationInput) -> Result<Value, AdkMutationPortError> {
        Ok(json!({
            "accepted": true,
            "operation": input.operation.name(),
        }))
    }
}

#[derive(Debug)]
struct UnavailableAdkMutationPort {
    calls: AtomicUsize,
}

impl UnavailableAdkMutationPort {
    fn new() -> Self {
        Self {
            calls: AtomicUsize::new(0),
        }
    }
}

impl AdkMutationPort for UnavailableAdkMutationPort {
    fn mutate(&self, _input: &AdkMutationInput) -> Result<Value, AdkMutationPortError> {
        self.calls.fetch_add(1, Ordering::SeqCst);
        Err(AdkMutationPortError::Unavailable(
            "fixture Rust mutation port crashed".to_owned(),
        ))
    }
}

#[tokio::test]
async fn adk_mutation_routes_register_only_with_explicit_test_port() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let base = ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
        .expect("config");
    let handle = start_product(base).await.expect("start base product");
    assert_eq!(handle.startup_record().owned_routes, 48);
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/agents",
        Some(r#"{"name":"Fixture agent"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    handle.shutdown().await.expect("shutdown base product");

    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_mutation_port(Arc::new(FixtureAdkMutationPort));
    let handle = start_product(config).await.expect("start ADK product");
    assert_eq!(handle.startup_record().owned_routes, 85);
    assert!(
        handle
            .startup_record()
            .capabilities
            .iter()
            .any(|route| { route == "POST /api/v1/adk/agents" })
    );
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/agents",
        Some(r#"{"name":"Fixture agent"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    assert_eq!(response["data"]["operation"], "create-agent");
    handle.shutdown().await.expect("shutdown ADK product");
}

#[tokio::test]
async fn adk_mutation_product_fails_closed_and_recovers_after_restart_without_settings_write() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{}\n").expect("seed settings");
    let settings_before = std::fs::read(&settings_path).expect("read initial settings");
    let unavailable_port = Arc::new(UnavailableAdkMutationPort::new());
    let failing_config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_mutation_port(unavailable_port.clone());
    let handle = start_product(failing_config)
        .await
        .expect("start failing product");

    let (status, malformed) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/agents",
        Some("{"),
        &[],
    )
    .await;
    assert_eq!(status, 400);
    assert_eq!(malformed["error"]["code"], "BAD_REQUEST");
    assert_eq!(unavailable_port.calls.load(Ordering::SeqCst), 0);

    let (status, unavailable) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/agents",
        Some(r#"{"name":"fixture agent"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 503);
    assert_eq!(unavailable["error"]["code"], "ADK_MUTATIONS_UNAVAILABLE");
    assert_eq!(unavailable_port.calls.load(Ordering::SeqCst), 1);
    handle.shutdown().await.expect("shutdown failing product");

    let recovered_config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("recovered config")
            .with_adk_mutation_port(Arc::new(FixtureAdkMutationPort));
    let recovered = start_product(recovered_config)
        .await
        .expect("start recovered product");
    let (status, response) = request_json_with_status(
        recovered.startup_record().address,
        "POST",
        "/api/v1/adk/agents",
        Some(r#"{"name":"fixture agent"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["ok"], true);
    assert_eq!(response["data"]["accepted"], true);
    recovered
        .shutdown()
        .await
        .expect("shutdown recovered product");

    let settings_after = std::fs::read(&settings_path).expect("read final settings");
    assert_eq!(settings_after, settings_before);
}

#[path = "product_adk_mutation_test_cutover.rs"]
mod product_adk_mutation_test_cutover;
use product_adk_mutation_test_cutover::AdkMutationSqliteTestCutoverPort;

fn seed_valid_go_adk_db(path: &Path) {
    let connection = rusqlite::Connection::open(path).expect("open sqlite");
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

#[tokio::test]
async fn adk_sqlite_test_cutover_replays_mutations_and_recovers_across_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{}\n").expect("seed settings");
    let db_path = directory.path().join("adk.db");
    seed_valid_go_adk_db(&db_path);

    let cutover_port =
        Arc::new(AdkMutationSqliteTestCutoverPort::open(&db_path).expect("open port"));
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_mutation_port(cutover_port.clone());
    let handle = start_product(config).await.expect("start product");

    let (status, resp) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/agents",
        Some(r#"{"id":"analyst-1","name":"Senior Analyst"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(resp["ok"], true);
    assert_eq!(resp["data"]["id"], "analyst-1");

    handle.shutdown().await.expect("shutdown product");
    drop(cutover_port);

    // Reopen and verify persistence
    let cutover_port2 =
        Arc::new(AdkMutationSqliteTestCutoverPort::open(&db_path).expect("reopen port"));
    let retrieved = cutover_port2
        .store()
        .get_agent("analyst-1")
        .expect("get agent")
        .expect("found");
    assert_eq!(retrieved.id, "analyst-1");
}
