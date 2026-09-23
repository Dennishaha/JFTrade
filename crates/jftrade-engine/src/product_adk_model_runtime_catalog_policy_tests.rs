//! Regression coverage for Go's `ToolRequiresApproval` / ADK loop contract.
//!
//! Go builds every product `FunctionTool` with
//! `RequireConfirmation: ToolRequiresApproval(descriptor, agent.PermissionMode)`.
//! A call whose descriptor does not need confirmation is executed inline by the
//! ADK loop in the same turn, and only confirmation-gated calls park the run as
//! `PENDING` with `pendingApprovals`.  The Rust runtime used to ignore the
//! policy completely and stage every supported call for approval, which made
//! read tools wait for an operator that the reference never asks.

use std::collections::BTreeMap;
use std::fs::File;
use std::sync::Arc;
use std::time::Duration;

use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

use jftrade_store_sqlite::{AdkSessionStore, AdkStore, CreateAdkRunParams, initialize_current};

use super::{
    AdkToolExecutor, ChatExecution, ModelRequest, ModelResponse, ModelToolCall,
    ProductionAdkChatRuntime, RunCancellationRegistry, RunLeaseGuard, ToolCallStaging,
};

/// A tool executor that records every capability the runtime tries to run.
#[derive(Debug)]
struct RecordingToolExecutor {
    supported: Vec<&'static str>,
    executed: Arc<std::sync::Mutex<Vec<String>>>,
}

impl RecordingToolExecutor {
    fn new(supported: Vec<&'static str>) -> Self {
        Self {
            supported,
            executed: Arc::new(std::sync::Mutex::new(Vec::new())),
        }
    }

    fn executed(&self) -> Vec<String> {
        self.executed
            .lock()
            .expect("recording executor lock")
            .clone()
    }
}

impl AdkToolExecutor for RecordingToolExecutor {
    fn supports(&self, name: &str) -> bool {
        self.supported.contains(&name)
    }

    fn execute(&self, name: &str, arguments: &Value) -> Result<Value, String> {
        self.executed
            .lock()
            .expect("recording executor lock")
            .push(name.to_owned());
        Ok(json!({"tool": name, "arguments": arguments}))
    }
}

fn initialized_stores() -> (tempfile::TempDir, Arc<AdkStore>, Arc<AdkSessionStore>) {
    let directory = tempdir().expect("temporary directory");
    let adk_path = directory.path().join("adk.db");
    let session_path = directory.path().join("adk-session.db");
    File::create(&adk_path).expect("create ADK database");
    File::create(&session_path).expect("create ADK session database");
    initialize_current(
        &Connection::open(&adk_path).expect("initialize ADK database"),
        "adk",
    )
    .expect("initialize ADK schema");
    initialize_current(
        &Connection::open(&session_path).expect("initialize ADK session database"),
        "adk-session",
    )
    .expect("initialize ADK session schema");
    (
        directory,
        Arc::new(AdkStore::open(&adk_path).expect("open ADK store")),
        Arc::new(AdkSessionStore::open(&session_path).expect("open ADK session store")),
    )
}

/// A runtime whose model provider is never called: the tests stage a model
/// response directly and only exercise the staging/execution decision.
fn runtime_with_executor(
    store: &Arc<AdkStore>,
    session_store: &Arc<AdkSessionStore>,
    directory: &tempfile::TempDir,
    executor: Arc<RecordingToolExecutor>,
) -> ProductionAdkChatRuntime {
    runtime_with_catalog(
        store,
        session_store,
        directory,
        executor,
        production_catalog(),
    )
}

fn production_catalog() -> crate::product::product_production_ports::ProductionToolCatalog {
    let bindings = crate::product::product_production_ports::product_production_ports_adk::PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| {
            (
                definition.adapter,
                crate::product::product_production_ports::ProductionAdapterBinding::Ready,
            )
        })
        .collect::<BTreeMap<_, _>>();
    crate::product::product_production_ports::ProductionToolCatalog::from_bindings(&bindings)
        .expect("complete tool bindings")
}

fn runtime_with_catalog(
    store: &Arc<AdkStore>,
    session_store: &Arc<AdkSessionStore>,
    directory: &tempfile::TempDir,
    executor: Arc<RecordingToolExecutor>,
    catalog: crate::product::product_production_ports::ProductionToolCatalog,
) -> ProductionAdkChatRuntime {
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    ProductionAdkChatRuntime::with_tool_executor_for_test(
        Arc::clone(store),
        Arc::clone(session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(catalog),
        executor,
    )
}

fn seed_run(run_id: &str, permission_mode: &str, tool_name: &str) -> ChatExecution {
    ChatExecution::for_test(
        run_id.to_owned(),
        format!("session-{run_id}"),
        format!("agent-{run_id}"),
        permission_mode.to_owned(),
        ModelRequest {
            endpoint: "http://127.0.0.1:1/v1/responses"
                .parse()
                .expect("loopback endpoint"),
            api_key: "test-key".to_owned(),
            model: "fixture-model".to_owned(),
            instruction: None,
            message: "hello".to_owned(),
            durable_context: Vec::new(),
            tool_context: Vec::new(),
            timeout: Duration::from_secs(5),
            tools: vec![json!({"type": "function", "name": tool_name})],
        },
    )
}

fn create_run(store: &AdkStore, run_id: &str) {
    store
        .create_run(CreateAdkRunParams {
            id: run_id,
            session_id: &format!("session-{run_id}"),
            agent_id: &format!("agent-{run_id}"),
            status: "RUNNING",
            client_request_id: &format!("request-{run_id}"),
            request_fingerprint: &format!("fingerprint-{run_id}"),
            payload_json: &json!({
                "id": run_id,
                "sessionId": format!("session-{run_id}"),
                "agentId": format!("agent-{run_id}"),
                "status": "RUNNING",
                "route": "chat",
                "resumeState": "provider_executing",
                "toolCalls": [],
                "toolResults": [],
                "pendingApprovals": [],
            })
            .to_string(),
        })
        .expect("create run");
}

fn response_with_call(call_id: &str, name: &str) -> ModelResponse {
    ModelResponse {
        text: String::new(),
        tool_calls: vec![ModelToolCall {
            id: call_id.to_owned(),
            name: name.to_owned(),
            arguments: json!({}),
        }],
    }
}

fn run_payload(store: &AdkStore, run_id: &str) -> Value {
    let run = store
        .get_run(run_id)
        .expect("read run")
        .expect("run exists");
    serde_json::from_str(&run.payload_json).expect("decode payload")
}

/// Go's `ToolRequiresApproval` treats `read_internal`/low descriptors as
/// automatically executable in `approval` mode, so a `system.status` call runs
/// immediately instead of producing `pendingApprovals`.
/// Parity: go:452dea11:internal/assistant/engine/tools_test.go:106
/// `TestLowRiskWriteToolsCanSkipApproval`.
#[test]
fn read_tool_runs_without_approval_in_approval_mode() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_with_executor(
        &store,
        &session_store,
        &directory,
        Arc::new(RecordingToolExecutor::new(vec!["system.status"])),
    );
    create_run(&store, "run-read-tool");
    let chat = seed_run("run-read-tool", "approval", "system.status");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-read-tool", "owner-policy")
        .expect("acquire run lease");

    let staged = runtime
        .persist_tool_calls(
            &chat,
            &response_with_call("call-status", "system.status"),
            &lease,
        )
        .expect("stage tool calls");
    assert!(
        matches!(staged, ToolCallStaging::Released),
        "a low-risk read must be released without approval: {staged:?}"
    );
    let run = store
        .get_run("run-read-tool")
        .expect("read run")
        .expect("run exists");
    let payload: Value = serde_json::from_str(&run.payload_json).expect("decode payload");
    assert_eq!(
        run.status, "RUNNING",
        "the run stays live for the tool loop"
    );
    assert_eq!(
        payload["pendingApprovals"].as_array().map(Vec::len),
        Some(0),
        "no operator approval is requested: {payload}"
    );
    assert_eq!(payload["toolCalls"][0]["status"], "RUNNING");
    assert_eq!(payload["toolCalls"][0]["requiresUser"], false);
}

/// The default `approval` mode is the one Go ships with, so an agent payload
/// that omits `permissionMode` must not be treated as `all`: the descriptor
/// policy still decides, and low-risk reads stay automatic.
#[test]
fn blank_permission_mode_still_releases_reads() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_with_executor(
        &store,
        &session_store,
        &directory,
        Arc::new(RecordingToolExecutor::new(vec!["strategy.validate_pine"])),
    );
    create_run(&store, "run-blank-mode");
    let chat = seed_run("run-blank-mode", "", "strategy.validate_pine");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-blank-mode", "owner-policy")
        .expect("acquire run lease");
    let staged = runtime
        .persist_tool_calls(
            &chat,
            &response_with_call("call-pine", "strategy.validate_pine"),
            &lease,
        )
        .expect("stage tool calls");
    assert!(
        matches!(staged, ToolCallStaging::Released),
        "a blank mode normalizes to approval, which does not gate reads: {staged:?}"
    );
    let payload = run_payload(&store, "run-blank-mode");
    assert_eq!(
        payload["pendingApprovals"].as_array().map(Vec::len),
        Some(0)
    );
}

/// Go's `NormalizePermissionMode` maps an unknown mode onto `approval`; the
/// runtime must do the same instead of silently treating garbage as `all`.
#[test]
fn unknown_permission_mode_normalizes_to_approval() {
    assert_eq!(
        ProductionAdkChatRuntime::agent_permission_mode(&json!({"permissionMode": "nope"})),
        "approval"
    );
    assert_eq!(
        ProductionAdkChatRuntime::agent_permission_mode(&json!({})),
        "approval"
    );
    assert_eq!(
        ProductionAdkChatRuntime::agent_permission_mode(&json!({"permissionMode": "all"})),
        "all"
    );
    assert_eq!(
        ProductionAdkChatRuntime::agent_permission_mode(
            &json!({"permissionMode": "less_approval"})
        ),
        "less_approval"
    );
}

/// A descriptor the catalog does not know about must stay fail-closed: the
/// runtime never guesses that an unknown name is safe to run.
#[test]
fn unknown_tool_stays_unavailable() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_with_executor(
        &store,
        &session_store,
        &directory,
        Arc::new(RecordingToolExecutor::new(Vec::new())),
    );
    create_run(&store, "run-unknown-tool");
    let chat = seed_run("run-unknown-tool", "all", "not.a.tool");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-unknown-tool", "owner-policy")
        .expect("acquire run lease");
    let error = runtime
        .persist_tool_calls(
            &chat,
            &response_with_call("call-unknown", "not.a.tool"),
            &lease,
        )
        .expect_err("an unsupported tool must fail closed");
    match error {
        super::AdkChatPortError::Failed { status, code, .. } => {
            assert_eq!(status, 503);
            assert_eq!(code, "ADK_TOOL_UNAVAILABLE");
        }
        other => panic!("unexpected error for an unsupported tool: {other:?}"),
    }
    let run = store
        .get_run("run-unknown-tool")
        .expect("read run")
        .expect("run exists");
    assert_eq!(run.status, "FAILED");
}

/// The released-call path really executes the tool: the loop must claim and run
/// every `RUNNING` call and persist its durable result.
/// Parity: go:452dea11:internal/assistant/engine/store_test.go:471
/// `TestApprovalModeCreatesPendingApprovalForWriteTool`.
#[test]
fn released_calls_are_executed_by_the_tool_loop() {
    let (directory, store, session_store) = initialized_stores();
    let executor = Arc::new(RecordingToolExecutor::new(vec!["system.status"]));
    let runtime = runtime_with_executor(&store, &session_store, &directory, Arc::clone(&executor));
    create_run(&store, "run-execute-released");
    let chat = seed_run("run-execute-released", "approval", "system.status");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-execute-released", "owner-policy")
        .expect("acquire run lease");
    let staged = runtime
        .persist_tool_calls(
            &chat,
            &response_with_call("call-status", "system.status"),
            &lease,
        )
        .expect("stage tool calls");
    assert!(matches!(staged, ToolCallStaging::Released));

    // The provider is unreachable, so the loop stops after executing and
    // persisting the released call; the tool itself must have run exactly once.
    runtime.run_tool_loop(
        chat,
        Arc::new(std::sync::atomic::AtomicBool::new(false)),
        &lease,
    );
    assert_eq!(
        executor.executed(),
        vec!["system.status".to_owned()],
        "the released call is executed by the tool loop"
    );
    let payload = run_payload(&store, "run-execute-released");
    let results = payload["toolResults"].as_array().expect("tool results");
    assert_eq!(results.len(), 1, "the tool result is durable: {payload}");
    assert_eq!(results[0]["callId"], "call-status");
    assert_eq!(payload["toolCalls"][0]["status"], "SUCCEEDED");
}

/// Go's `finishPendingApprovalRun` records the approval wait explicitly:
/// `resumeState=waiting_approval` and the fixed user-facing message.  A gated
/// call has to produce the same durable projection so the console can tell an
/// approval wait from a provider retry.
/// Parity: go:452dea11:internal/assistant/engine/store_test.go:471
/// `TestApprovalModeCreatesPendingApprovalForWriteTool`.
#[test]
fn gated_call_persists_the_go_approval_projection() {
    let (directory, store, session_store) = initialized_stores();
    let catalog =
        crate::product::product_production_ports::ProductionToolCatalog::from_tool_rows(vec![
            json!({
                "id": "alerts.price.set",
                "name": "alerts.price.set",
                "permission": "write_external",
                "riskLevel": "high",
                "requiresApprovalIn": ["approval", "less_approval", "all"],
                "allowedModes": ["approval", "less_approval", "all"],
                "idempotencyMode": "replay_safe",
            }),
        ]);
    let runtime = runtime_with_catalog(
        &store,
        &session_store,
        &directory,
        Arc::new(RecordingToolExecutor::new(vec!["alerts.price.set"])),
        catalog,
    );
    create_run(&store, "run-gated-projection");
    let chat = seed_run("run-gated-projection", "less_approval", "alerts.price.set");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-gated-projection", "owner-policy")
        .expect("acquire run lease");
    let staged = runtime
        .persist_tool_calls(
            &chat,
            &response_with_call("call-alert", "alerts.price.set"),
            &lease,
        )
        .expect("stage tool calls");
    let ToolCallStaging::Pending(super::AdkChatPortOutput::Json(response)) = staged else {
        panic!("an external alert write stays gated even in less_approval: {staged:?}");
    };
    assert!(
        response["reply"]
            .as_str()
            .is_some_and(|reply| reply.contains("审批队列")),
        "the approval wait must answer with the Go approval prompt: {response}"
    );
    let run = store
        .get_run("run-gated-projection")
        .expect("read run")
        .expect("run exists");
    let payload: Value = serde_json::from_str(&run.payload_json).expect("decode payload");
    assert_eq!(run.status, "PENDING");
    assert_eq!(payload["resumeState"], "waiting_approval");
    assert_eq!(payload["message"], "等待用户审批后继续执行。");
    assert_eq!(
        payload["pendingApprovals"].as_array().map(Vec::len),
        Some(1)
    );
    assert_eq!(payload["toolCalls"][0]["status"], "PENDING_APPROVAL");
    assert_eq!(payload["toolCalls"][0]["requiresUser"], true);
}

/// Go's `TestLiveTradingToolsAreAvailableInAllModesWithApproval`: a
/// `live_trading` descriptor stays selectable in every permission mode and
/// still requires approval even in `all`.  The runtime must therefore park such
/// a call instead of executing it or rejecting it as unavailable.
#[test]
fn live_trading_call_is_gated_in_every_mode() {
    let catalog =
        crate::product::product_production_ports::ProductionToolCatalog::from_tool_rows(vec![
            json!({
                "id": "orders.place",
                "name": "orders.place",
                "permission": "live_trading",
                "riskLevel": "critical",
                "requiresApprovalIn": ["approval", "less_approval", "all"],
                "allowedModes": ["approval", "less_approval", "all"],
                "idempotencyMode": "replay_safe",
            }),
        ]);
    for mode in ["approval", "less_approval", "all"] {
        assert!(
            catalog.requires_approval("orders.place", mode),
            "live trading must require approval in {mode}"
        );
    }
    assert!(
        catalog.requires_approval("missing.tool", "all"),
        "a missing descriptor fails closed: unknown tools are never auto-released"
    );

    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_with_catalog(
        &store,
        &session_store,
        &directory,
        Arc::new(RecordingToolExecutor::new(vec!["orders.place"])),
        catalog,
    );
    create_run(&store, "run-live-trading");
    let chat = seed_run("run-live-trading", "all", "orders.place");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-live-trading", "owner-policy")
        .expect("acquire run lease");
    let staged = runtime
        .persist_tool_calls(
            &chat,
            &response_with_call("call-order", "orders.place"),
            &lease,
        )
        .expect("stage tool calls");
    assert!(
        matches!(staged, ToolCallStaging::Pending(_)),
        "live trading must wait for the operator even in all mode: {staged:?}"
    );
    let payload = run_payload(&store, "run-live-trading");
    assert_eq!(payload["toolCalls"][0]["status"], "PENDING_APPROVAL");
    assert_eq!(payload["toolCalls"][0]["requiresUser"], true);
    assert_eq!(
        payload["pendingApprovals"].as_array().map(Vec::len),
        Some(1)
    );
}
