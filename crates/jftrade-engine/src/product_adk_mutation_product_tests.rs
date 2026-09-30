use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use serde_json::{Value, json};
use tempfile::tempdir;

use super::super::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortOutput, AdkChatRoute, AdkChatStreamPort,
};
use super::super::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationPort, AdkMutationPortError,
};
use super::*;

#[derive(Debug)]
struct ProviderProbeWireRuntime;

impl AdkChatStreamPort for ProviderProbeWireRuntime {
    fn dispatch(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, super::super::product_adk_chat_stream_port::AdkChatPortError>
    {
        assert_eq!(route, AdkChatRoute::Chat);
        let request: Value = serde_json::from_slice(&input.body).expect("decode probe request");
        let mode = request["providerTestMode"]
            .as_str()
            .expect("provider test mode");
        Ok(AdkChatPortOutput::Json(json!({
            "ok": mode == "quick",
            "capabilities": {
                "streaming": true,
                "tools": mode == "quick",
                "reasoning": mode == "quick",
            },
            "reasoning": {
                "mode": mode,
                "requestField": "reasoning.effort",
                "ok": mode == "quick",
            },
            "checkedAt": "2026-01-01T00:00:00Z",
        })))
    }

    fn runtime_ready(&self) -> bool {
        true
    }
}

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

/// Parity: go:452dea11:internal/api/assistant/routes_resource_contracts_test.go:159
/// `TestProviderAndAgentValidationContracts`: the public provider probe route
/// defaults to quick mode, preserves the full-mode probe projection, rejects
/// unsupported modes before dispatch, and maps a missing provider to the Go
/// 404 not-found envelope.
#[tokio::test]
async fn provider_test_routes_match_go_wire_mode_and_missing_provider_contract() {
    use jftrade_store_sqlite::initialize_current;

    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{}\n").expect("seed settings");
    for (component, filename) in [
        ("adk", "adk.db"),
        ("adk-session", "adk-session.db"),
        ("adk-artifact", "adk-artifact.db"),
    ] {
        let connection = rusqlite::Connection::open(directory.path().join(filename))
            .expect("temporary database");
        initialize_current(&connection, component).expect("current schema");
    }
    let mut port = production_optimization_port(directory.path(), false);
    port.store
        .upsert_provider(
            "provider-wire",
            &json!({
                "id": "provider-wire",
                "displayName": "Wire Provider",
                "baseUrl": "https://provider.example/v1",
                "model": "probe-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    Arc::get_mut(&mut port)
        .expect("port is uniquely owned before composition")
        .chat_runtime = Some(Arc::new(ProviderProbeWireRuntime));
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_mutation_port(port.clone());
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;
    let route = "/api/v1/adk/providers/provider-wire/test";

    for (body, mode, ok) in [
        (None, "quick", true),
        (Some(r#"{"mode":"full"}"#), "full", false),
    ] {
        let (status, response) = request_json_with_status(address, "POST", route, body, &[]).await;
        assert_eq!(status, 200, "provider test {mode}: {response}");
        assert_eq!(response["ok"], true);
        assert_eq!(response["data"]["ok"], ok);
        assert_eq!(response["data"]["reasoning"]["mode"], mode);
        assert_eq!(
            response["data"]["reasoning"]["requestField"],
            "reasoning.effort"
        );
    }

    let (status, response) =
        request_json_with_status(address, "POST", route, Some(r#"{"mode":"slow"}"#), &[]).await;
    assert_eq!(status, 400);
    assert_eq!(response["error"]["code"], "BAD_REQUEST");
    assert_eq!(
        response["error"]["message"],
        "provider test mode must be quick or full"
    );

    let (status, response) = request_json_with_status(
        address,
        "POST",
        "/api/v1/adk/providers/missing-provider/test",
        Some(r#"{"mode":"quick"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 404);
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "ADK_PROVIDER_NOT_FOUND");
    assert_eq!(response["error"]["message"], "provider not found");

    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn optimization_task_cancel_route_preserves_go_operation_identity() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_mutation_port(Arc::new(FixtureAdkMutationPort));
    let handle = start_product(config).await.expect("start product");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/optimization-tasks/opt-test/cancel",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["operation"], "cancel-optimization-task");
    handle.shutdown().await.expect("shutdown product");
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

fn production_optimization_port(
    root: &Path,
    initialize: bool,
) -> Arc<super::super::product_production_ports::ProductionAdkPort> {
    use jftrade_store_sqlite::{AdkArtifactStore, AdkSessionStore, AdkStore, initialize_current};
    if initialize {
        for component in ["adk", "adk-session", "adk-artifact"] {
            let connection = rusqlite::Connection::open(root.join(format!("{component}.db")))
                .expect("temporary database");
            initialize_current(&connection, component).expect("current schema");
        }
    }
    Arc::new(
        super::super::product_production_ports::ProductionAdkPort::new_for_test(
            Arc::new(AdkStore::open(root.join("adk.db")).expect("ADK writer")),
            Arc::new(AdkSessionStore::open(root.join("adk-session.db")).expect("session writer")),
            Arc::new(
                AdkArtifactStore::open(root.join("adk-artifact.db")).expect("artifact writer"),
            ),
            root.join("settings.json"),
        ),
    )
}

// Parity: go:452dea11:internal/api/assistant/adk_ops_test.go:247 TestADKOptimizationTaskCanBeQueriedAndCancelled
#[tokio::test]
async fn optimization_task_http_cancellation_persists_through_production_port_restart() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    let port = production_optimization_port(directory.path(), true);
    let original = json!({"status":"queued", "objective":"return",
        "runs":[{"definitionId":"definition-1", "runId":"missing-run"}]});
    port.store
        .upsert_optimization_task("opt-persist", &original.to_string())
        .expect("seed task");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_read_snapshot_port(port.clone())
            .with_adk_mutation_port(port.clone());
    let handle = start_product(config).await.expect("start product");
    let path = "/api/v1/adk/optimization-tasks/opt-persist";
    let (status, before) =
        request_json_with_status(handle.startup_record().address, "GET", path, None, &[]).await;
    assert_eq!(status, 200);
    assert_eq!(before["data"]["status"], "queued");
    let (status, response) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        "/api/v1/adk/optimization-tasks/opt-persist/cancel",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(response["data"]["status"], "cancelled");
    assert_eq!(response["data"]["runs"], original["runs"]);
    let stored = port
        .store
        .get_optimization_task("opt-persist")
        .unwrap()
        .unwrap();
    let (status, repeated) = request_json_with_status(
        handle.startup_record().address,
        "POST",
        &format!("{path}/cancel"),
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200);
    assert_eq!(repeated["ok"], true);
    assert_eq!(repeated["data"], response["data"]);
    assert_eq!(
        port.store
            .get_optimization_task("opt-persist")
            .unwrap()
            .unwrap()
            .updated_at,
        stored.updated_at,
        "repeat cancellation must not rewrite terminal state"
    );
    handle.shutdown().await.expect("shutdown product");
    drop(port);
    let reopened = production_optimization_port(directory.path(), false);
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("restart config")
            .with_adk_read_snapshot_port(reopened);
    let handle = start_product(config).await.expect("restart product");
    let (status, restored) =
        request_json_with_status(handle.startup_record().address, "GET", path, None, &[]).await;
    assert_eq!(status, 200);
    assert_eq!(restored["data"], response["data"]);
    handle.shutdown().await.expect("shutdown restarted product");
}

/// Parity: go:452dea11:internal/api/assistant/adk_ops_test.go:394 TestADKOptimizationTaskNegativeRoutes
/// TestADKOptimizationTaskNegativeRoutes.
///
/// Go answers the optimization-task negative matrix through two handlers that
/// both normalise a "not found" service error to `404 NOT_FOUND` /
/// "optimization task not found", and reject an undecodable `:taskId` with
/// `400 BAD_REQUEST` / "taskId is invalid" before the service is consulted.
/// The read and cancel surfaces must agree on all four responses.
#[tokio::test]
async fn optimization_task_negative_routes_match_the_reference_matrix() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{}\n").expect("seed settings");
    let port = production_optimization_port(directory.path(), true);
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_read_snapshot_port(port.clone())
            .with_adk_mutation_port(port);
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;

    // `%zz` is not a valid percent escape, so the identifier never reaches the
    // store on either surface.
    for (method, path) in [
        ("GET", "/api/v1/adk/optimization-tasks/%zz"),
        ("POST", "/api/v1/adk/optimization-tasks/%zz/cancel"),
    ] {
        let (status, response) = request_json_with_status(address, method, path, None, &[]).await;
        assert_eq!(status, 400, "route {method} {path}: {response}");
        assert_eq!(response["ok"], false, "route {method} {path}");
        assert_eq!(
            response["error"]["code"], "BAD_REQUEST",
            "route {method} {path}"
        );
        assert_eq!(
            response["error"]["message"], "taskId is invalid",
            "route {method} {path}"
        );
    }

    for (method, path) in [
        ("GET", "/api/v1/adk/optimization-tasks/missing-task"),
        ("POST", "/api/v1/adk/optimization-tasks/missing-task/cancel"),
    ] {
        let (status, response) = request_json_with_status(address, method, path, None, &[]).await;
        assert_eq!(status, 404, "route {method} {path}: {response}");
        assert_eq!(response["ok"], false, "route {method} {path}");
        assert_eq!(
            response["error"]["code"], "NOT_FOUND",
            "route {method} {path}"
        );
        assert_eq!(
            response["error"]["message"], "optimization task not found",
            "route {method} {path}"
        );
    }

    handle.shutdown().await.expect("shutdown product");
}

/// Parity: go:452dea11:internal/api/assistant/adk_approval_test.go:450 TestADKRunPauseResumeRoutesRejectInvalidRuns
/// TestADKRunPauseResumeRoutesRejectInvalidRuns.
///
/// Go's pause handler validates the run before mutating it: a loop-mode child
/// run (`parentRunId` set) is rejected with `400`, while resume of a missing
/// run is `404 NOT_FOUND`.  The child branch is what stops a workflow child
/// from pausing itself out of its parent's control.
/// Parity: go:452dea11:internal/api/assistant/routes_payload_pagination_test.go:80 TestAssistantRunMutationRoutesEnforceGoalLifecycleRules
/// TestAssistantRunMutationRoutesEnforceGoalLifecycleRules.
///
/// The same production port accepts a goal pause (`RUNNING` loop run returns
/// `resumeState=user_pause_requested`) while rejecting chat runs on all three
/// lifecycle mutations with their own route codes.  This witness keeps the Go
/// test's exact assertion set in one place, separate from the child-run
/// rejection witness used by `adk_approval_test.go:450`.
#[tokio::test]
async fn assistant_run_mutation_routes_enforce_goal_lifecycle_rules() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{}\n").expect("seed settings");
    let port = production_optimization_port(directory.path(), true);
    port.store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-route-pause",
            session_id: "session-1",
            agent_id: "agent-1",
            status: "RUNNING",
            client_request_id: "goal-pause-request",
            request_fingerprint: "goal-pause-fingerprint",
            payload_json: r#"{
                "id":"run-route-pause",
                "sessionId":"session-1",
                "agentId":"agent-1",
                "status":"RUNNING",
                "workMode":"loop",
                "workflowStatus":"running",
                "toolCalls":[],
                "pendingApprovals":[]
            }"#,
        })
        .expect("seed goal run");
    port.store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-route-chat",
            session_id: "session-1",
            agent_id: "agent-1",
            status: "RUNNING",
            client_request_id: "goal-chat-request",
            request_fingerprint: "goal-chat-fingerprint",
            payload_json: r#"{
                "id":"run-route-chat",
                "sessionId":"session-1",
                "agentId":"agent-1",
                "status":"RUNNING",
                "workMode":"chat",
                "toolCalls":[],
                "pendingApprovals":[]
            }"#,
        })
        .expect("seed chat run");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_read_snapshot_port(port.clone())
            .with_adk_mutation_port(port);
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;

    let (status, paused) = request_json_with_status(
        address,
        "POST",
        "/api/v1/adk/runs/run-route-pause/pause",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 200, "goal pause: {paused}");
    assert_eq!(paused["data"]["resumeState"], "user_pause_requested");

    for (path, code) in [
        (
            "/api/v1/adk/runs/run-route-chat/pause",
            "ADK_RUN_PAUSE_FAILED",
        ),
        (
            "/api/v1/adk/runs/run-route-chat/resume",
            "ADK_RUN_RESUME_FAILED",
        ),
    ] {
        let (status, response) = request_json_with_status(address, "POST", path, None, &[]).await;
        assert_eq!(status, 400, "{path}: {response}");
        assert_eq!(response["error"]["code"], code, "{path}");
    }
    let (status, response) = request_json_with_status(
        address,
        "PATCH",
        "/api/v1/adk/runs/run-route-chat/objective",
        Some(r#"{"objective":"should be rejected"}"#),
        &[],
    )
    .await;
    assert_eq!(status, 400, "chat objective: {response}");
    assert_eq!(response["error"]["code"], "ADK_RUN_OBJECTIVE_UPDATE_FAILED");

    handle.shutdown().await.expect("shutdown product");
}

#[tokio::test]
async fn goal_pause_rejects_child_runs_and_resume_reports_missing_runs() {
    let directory = tempdir().expect("temporary directory");
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, b"{}\n").expect("seed settings");
    let port = production_optimization_port(directory.path(), true);
    port.store
        .create_run(jftrade_store_sqlite::CreateAdkRunParams {
            id: "run-child-pause",
            session_id: "session-1",
            agent_id: "agent-1",
            status: "RUNNING",
            client_request_id: "child-pause-request",
            request_fingerprint: "child-pause-fingerprint",
            payload_json: r#"{
                "id":"run-child-pause",
                "sessionId":"session-1",
                "agentId":"agent-1",
                "status":"RUNNING",
                "workMode":"loop",
                "parentRunId":"run-parent",
                "workflowStatus":"RUNNING",
                "toolCalls":[],
                "pendingApprovals":[]
            }"#,
        })
        .expect("seed child run");
    let config =
        ProductConfig::test_cutover("127.0.0.1:0".parse().expect("address"), &settings_path)
            .expect("config")
            .with_adk_read_snapshot_port(port.clone())
            .with_adk_mutation_port(port);
    let handle = start_product(config).await.expect("start product");
    let address = handle.startup_record().address;

    let (status, response) = request_json_with_status(
        address,
        "POST",
        "/api/v1/adk/runs/run-child-pause/pause",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 400, "child pause response: {response}");
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "ADK_RUN_PAUSE_FAILED");
    assert_eq!(
        response["error"]["message"], "only root goal runs can be paused",
        "child run pause must not reach the mutation"
    );

    let (status, response) = request_json_with_status(
        address,
        "POST",
        "/api/v1/adk/runs/missing-run/resume",
        None,
        &[],
    )
    .await;
    assert_eq!(status, 404, "missing resume response: {response}");
    assert_eq!(response["ok"], false);
    assert_eq!(response["error"]["code"], "NOT_FOUND");
    assert_eq!(response["error"]["message"], "run not found");

    handle.shutdown().await.expect("shutdown product");
}
