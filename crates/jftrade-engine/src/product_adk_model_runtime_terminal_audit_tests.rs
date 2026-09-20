//! Regression coverage for Go's chat terminal-state, audit and final-message
//! contracts (`internal/assistant/engine/runner_chat_test.go`).
//!
//! Go's `Runtime.CompleteChatRun` has a strict order: mark the terminal state,
//! persist it together with an audit row, then attach the final assistant
//! message and project the run.  The Rust runtime persisted the run but wrote
//! none of the audit rows, so `GET /api/v1/adk/audit` could never show
//! `run.completed` / `run.failed` / `run.cancelled` / `run.awaiting_approval`
//! even though the store and the read route were fully wired.

use std::fs::File;
use std::sync::Arc;
use std::time::Duration;

use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

use jftrade_store_sqlite::{AdkSessionStore, AdkStore, CreateAdkRunParams, initialize_current};

use super::{
    ChatExecution, ModelRequest, ProductionAdkChatRuntime, RunCancellationRegistry, RunLeaseGuard,
    lifecycle_audit_kind, terminal_audit_fields, terminal_audit_message,
};

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
        Arc::new(AdkSessionStore::open(&session_path).expect("open session store")),
    )
}

fn runtime_for(
    directory: &tempfile::TempDir,
    store: &Arc<AdkStore>,
    session_store: &Arc<AdkSessionStore>,
) -> Arc<ProductionAdkChatRuntime> {
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    ProductionAdkChatRuntime::new(
        Arc::clone(store),
        Arc::clone(session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    )
}

fn create_running_run(store: &AdkStore, run_id: &str, tool_calls: Value) {
    store
        .create_run(CreateAdkRunParams {
            id: run_id,
            session_id: Box::leak(format!("session-{run_id}").into_boxed_str()),
            agent_id: "agent-terminal",
            status: "RUNNING",
            client_request_id: Box::leak(format!("request-{run_id}").into_boxed_str()),
            request_fingerprint: "fingerprint-terminal",
            payload_json: &json!({
                "id": run_id,
                "sessionId": format!("session-{run_id}"),
                "agentId": "agent-terminal",
                "status": "RUNNING",
                "route": "chat",
                "resumeState": "provider_executing",
                "toolCalls": tool_calls,
                "toolResults": [],
                "pendingApprovals": [],
            })
            .to_string(),
        })
        .expect("create run");
}

fn chat_for(run_id: &str) -> ChatExecution {
    ChatExecution::for_test(
        run_id.to_owned(),
        format!("session-{run_id}"),
        "agent-terminal".to_owned(),
        "approval".to_owned(),
        ModelRequest {
            endpoint: "http://127.0.0.1:1/v1/responses"
                .parse()
                .expect("loopback endpoint"),
            api_key: "sk-fixture".to_owned(),
            model: "fixture-model".to_owned(),
            instruction: None,
            message: "hello".to_owned(),
            durable_context: Vec::new(),
            tool_context: Vec::new(),
            timeout: Duration::from_secs(2),
            tools: Vec::new(),
        },
    )
}

fn audit_rows(store: &AdkStore) -> Vec<(String, String, Value)> {
    store
        .list_audit_events()
        .expect("list audit events")
        .into_iter()
        .map(|row| {
            let payload: Value =
                serde_json::from_str(&row.payload_json).expect("decode audit payload");
            (row.kind, row.subject_id, payload)
        })
        .collect()
}

fn audit_row<'a>(rows: &'a [(String, String, Value)], kind: &str) -> &'a (String, String, Value) {
    rows.iter()
        .find(|(row_kind, _, _)| row_kind == kind)
        .unwrap_or_else(|| panic!("audit rows must contain {kind}: {rows:?}"))
}

/// Go's `markFailedChatRun` maps the context error onto
/// `CANCELLED`/`RUN_CANCELLED`, `TIMED_OUT`/`RUN_TIMED_OUT`, or
/// `FAILED`/`MODEL_CALL_FAILED`, and `RunLifecycleAuditKind` derives the
/// matching audit kind from the terminal status.
#[test]
fn lifecycle_audit_helpers_match_the_reference_table() {
    assert_eq!(lifecycle_audit_kind("COMPLETED"), "run.completed");
    assert_eq!(lifecycle_audit_kind("FAILED"), "run.failed");
    assert_eq!(lifecycle_audit_kind("TIMED_OUT"), "run.timed_out");
    assert_eq!(lifecycle_audit_kind("CANCELLED"), "run.cancelled");
    assert_eq!(lifecycle_audit_kind("DENIED"), "run.denied");
    // Go's `TerminalAuditMessage` is "Agent run completed." only for COMPLETED.
    assert_eq!(terminal_audit_message("COMPLETED"), "Agent run completed.");
    assert_eq!(
        terminal_audit_message("FAILED"),
        "Agent run finished with a terminal status."
    );
    assert_eq!(
        terminal_audit_message("TIMED_OUT"),
        "Agent run finished with a terminal status."
    );
    // A run without errorCode/failureReason omits both keys (Go's `omitempty`).
    assert_eq!(
        terminal_audit_fields("run", "agent", "COMPLETED", "", ""),
        json!({"runId": "run", "agentId": "agent", "status": "COMPLETED"})
    );
    assert_eq!(
        terminal_audit_fields("run", "agent", "FAILED", "MODEL_CALL_FAILED", "boom"),
        json!({
            "runId": "run",
            "agentId": "agent",
            "status": "FAILED",
            "errorCode": "MODEL_CALL_FAILED",
            "failureReason": "boom",
        })
    );
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:181
/// TestPersistRunTerminalStateWritesRunAndAudit.
///
/// Go saves the failed run and audits `run.failed` with the error code and
/// failure reason.  Rust now records the same terminal audit row with the
/// `RunID`/`AgentID`/`Status` fields plus the non-empty failure details.
#[test]
fn a_failed_run_persists_its_terminal_state_and_audit_row() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    create_running_run(&store, "run-terminal-audit", json!([]));
    let chat = chat_for("run-terminal-audit");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-terminal-audit", "owner-terminal")
        .expect("acquire run lease");

    let failure = super::AdkChatPortError::Failed {
        status: 502,
        code: "MODEL_CALL_FAILED".to_owned(),
        message: "boom".to_owned(),
    };
    runtime
        .persist_failure(&chat, &failure, &lease)
        .expect("persist failure");

    let run = store
        .get_run("run-terminal-audit")
        .expect("read run")
        .expect("run row");
    assert_eq!(run.status, "FAILED");
    let payload: Value = serde_json::from_str(&run.payload_json).expect("decode payload");
    assert_eq!(payload["errorCode"], "MODEL_CALL_FAILED");
    assert_eq!(payload["degraded"], json!(true));

    let rows = audit_rows(&store);
    let (kind, subject_id, audit) = audit_row(&rows, "run.failed");
    assert_eq!(kind, "run.failed");
    assert_eq!(subject_id, "run-terminal-audit");
    assert_eq!(
        audit["detail"],
        "Agent run finished with a terminal status."
    );
    assert_eq!(audit["metadata"]["runId"], "run-terminal-audit");
    assert_eq!(audit["metadata"]["agentId"], "agent-terminal");
    assert_eq!(audit["metadata"]["status"], "FAILED");
    assert_eq!(audit["metadata"]["errorCode"], "MODEL_CALL_FAILED");
    assert_eq!(audit["metadata"]["failureReason"], "boom");
}

/// A model timeout terminates as `TIMED_OUT` and audits `run.timed_out`, which
/// is the status Go derives from `context.DeadlineExceeded`.
#[test]
fn a_timed_out_run_audits_the_timed_out_lifecycle_kind() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    create_running_run(&store, "run-timeout-audit", json!([]));
    let chat = chat_for("run-timeout-audit");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-timeout-audit", "owner-timeout")
        .expect("acquire run lease");

    runtime
        .persist_failure(
            &chat,
            &super::AdkChatPortError::Failed {
                status: 504,
                code: "MODEL_CALL_TIMEOUT".to_owned(),
                message: "assistant model request timed out".to_owned(),
            },
            &lease,
        )
        .expect("persist timeout");

    let run = store
        .get_run("run-timeout-audit")
        .expect("read run")
        .expect("run row");
    assert_eq!(run.status, "TIMED_OUT");
    let rows = audit_rows(&store);
    let (_, _, audit) = audit_row(&rows, "run.timed_out");
    assert_eq!(audit["metadata"]["status"], "TIMED_OUT");
    assert_eq!(audit["metadata"]["errorCode"], "MODEL_CALL_TIMEOUT");
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:1124
/// TestCancelRunOnTerminalStateIsNoop plus the cancelled audit contract.
///
/// Go audits `run.cancelled` with `RUN_CANCELLED`, and a cancel request on an
/// already-terminal run leaves it untouched.
#[test]
fn a_cancelled_run_audits_run_cancelled_and_terminates_once() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    create_running_run(&store, "run-cancel-audit", json!([]));
    let chat = chat_for("run-cancel-audit");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-cancel-audit", "owner-cancel")
        .expect("acquire run lease");

    let cancellation = super::AdkChatPortError::Failed {
        status: 499,
        code: "CLIENT_DISCONNECTED".to_owned(),
        message: "assistant chat client disconnected".to_owned(),
    };
    runtime
        .persist_cancelled(&chat, &cancellation, &lease)
        .expect("persist cancellation");
    // A second cancellation is a no-op for the terminal run.
    runtime
        .persist_cancelled(&chat, &cancellation, &lease)
        .expect("cancelling a terminal run stays a no-op");

    let run = store
        .get_run("run-cancel-audit")
        .expect("read run")
        .expect("run row");
    assert_eq!(run.status, "CANCELLED");
    let payload: Value = serde_json::from_str(&run.payload_json).expect("decode payload");
    assert_eq!(payload["errorCode"], "RUN_CANCELLED");
    assert_eq!(payload["errorStatus"], 499);

    let rows = audit_rows(&store);
    let cancelled: Vec<_> = rows
        .iter()
        .filter(|(kind, _, _)| kind == "run.cancelled")
        .collect();
    assert_eq!(
        cancelled.len(),
        1,
        "a terminal run audits run.cancelled exactly once: {rows:?}"
    );
    let (_, subject_id, audit) = cancelled[0];
    assert_eq!(subject_id, "run-cancel-audit");
    assert_eq!(audit["metadata"]["status"], "CANCELLED");
    assert_eq!(audit["metadata"]["errorCode"], "RUN_CANCELLED");
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:267
/// TestFinishPendingApprovalRunPersistsPendingStateAndAssistantPrompt.
///
/// Go parks the run as `PENDING` with `resumeState=waiting_approval`, the fixed
/// message, the approval prompt reply, and a `run.awaiting_approval` audit row
/// carrying the pending count — and never writes an assistant placeholder.
#[test]
fn a_gated_call_parks_the_run_and_audits_awaiting_approval() {
    use crate::product::product_adk_model_runtime::AdkToolExecutor;

    let (directory, store, session_store) = initialized_stores();
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
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

    #[derive(Debug)]
    struct RecordingExecutor;

    impl AdkToolExecutor for RecordingExecutor {
        fn supports(&self, name: &str) -> bool {
            name == "alerts.price.set"
        }

        fn execute(&self, _name: &str, _arguments: &Value) -> Result<Value, String> {
            panic!("a gated call must never execute without approval");
        }
    }

    let runtime = ProductionAdkChatRuntime::with_tool_executor_for_test(
        Arc::clone(&store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(catalog),
        Arc::new(RecordingExecutor),
    );
    create_running_run(&store, "run-awaiting-audit", json!([]));
    let chat = chat_for("run-awaiting-audit");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-awaiting-audit", "owner-awaiting")
        .expect("acquire run lease");

    let response = super::ModelResponse {
        text: String::new(),
        tool_calls: vec![super::ModelToolCall {
            id: "call-alert".to_owned(),
            name: "alerts.price.set".to_owned(),
            arguments: json!({}),
        }],
    };
    let staged = runtime
        .persist_tool_calls(&chat, &response, &lease)
        .expect("stage gated call");
    let super::ToolCallStaging::Pending(super::AdkChatPortOutput::Json(output)) = staged else {
        panic!("a high-risk write must park the run: {staged:?}");
    };
    assert!(
        output["reply"]
            .as_str()
            .is_some_and(|reply| reply.contains("审批队列")),
        "the approval wait must answer with the Go prompt: {output}"
    );

    let run = store
        .get_run("run-awaiting-audit")
        .expect("read run")
        .expect("run row");
    assert_eq!(run.status, "PENDING");
    let payload: Value = serde_json::from_str(&run.payload_json).expect("decode payload");
    assert_eq!(payload["resumeState"], "waiting_approval");
    assert_eq!(payload["message"], "等待用户审批后继续执行。");
    assert_eq!(
        payload["pendingApprovals"].as_array().map(Vec::len),
        Some(1)
    );

    let rows = audit_rows(&store);
    let (kind, subject_id, audit) = audit_row(&rows, "run.awaiting_approval");
    assert_eq!(kind, "run.awaiting_approval");
    assert_eq!(subject_id, "run-awaiting-audit");
    assert_eq!(audit["detail"], "Agent run is waiting for approval.");
    assert_eq!(audit["metadata"]["runId"], "run-awaiting-audit");
    assert_eq!(audit["metadata"]["status"], "PENDING");
    assert_eq!(audit["metadata"]["pendingApprovals"], 1);
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:376
/// TestCompleteChatRunSuccessPersistsCompletedRunAndAssistantReply.
///
/// A successful model reply must terminate the run `COMPLETED` with
/// `message="completed"`, persist the assistant text, and audit
/// `run.completed` with the terminal field set (no errorCode/failureReason).
#[test]
fn a_completed_run_persists_the_reply_and_audits_run_completed() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    create_running_run(&store, "run-success-audit", json!([]));
    let chat = chat_for("run-success-audit");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-success-audit", "owner-success")
        .expect("acquire run lease");

    let response = runtime
        .persist_success(
            &chat,
            super::ModelResponse {
                text: "final answer".to_owned(),
                tool_calls: Vec::new(),
            },
            &lease,
        )
        .expect("persist success");

    assert_eq!(response["run"]["status"], "COMPLETED");
    assert_eq!(response["run"]["message"], "completed");
    assert_eq!(response["reply"], "final answer");
    assert_eq!(response["run"]["degraded"], json!(false));

    let run = store
        .get_run("run-success-audit")
        .expect("read run")
        .expect("run row");
    let payload: Value = serde_json::from_str(&run.payload_json).expect("decode payload");
    assert_eq!(payload["status"], "COMPLETED");
    assert_eq!(payload["reply"], "final answer");
    assert_eq!(payload["message"], "completed");

    let rows = audit_rows(&store);
    let (kind, subject_id, audit) = audit_row(&rows, "run.completed");
    assert_eq!(kind, "run.completed");
    assert_eq!(subject_id, "run-success-audit");
    assert_eq!(audit["detail"], "Agent run completed.");
    assert_eq!(audit["metadata"]["status"], "COMPLETED");
    assert!(
        audit["metadata"].get("errorCode").is_none(),
        "a clean completion carries no errorCode: {audit}"
    );
    assert!(
        audit["metadata"].get("failureReason").is_none(),
        "a clean completion carries no failureReason: {audit}"
    );
}

/// Go's `CompleteChatRun` first runs `markFailedChatRun` + terminal
/// persistence for a provider failure: `status=FAILED`,
/// `errorCode=MODEL_CALL_FAILED`, `degraded=true`, the raw provider text on
/// `failureReason`, and a `run.failed` audit row that repeats the same code and
/// reason.
///
/// The wire half of Go's contract (`reply = userFacingADKError(adkErr)`, a
/// `finalMessageId` on the run, and a `200` failed-run projection) is tracked
/// as an open P0 decision: the Rust product deliberately keeps a retryable
/// provider outage durable (`providerRetry` + `502`) so the desktop can
/// recover, while the Go reference answers `200` with the failed run.  See the
/// batch scope note for the reproduction condition and the fix location.
#[test]
fn a_failed_run_persists_the_provider_error_and_audit_row() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    create_running_run(&store, "run-failure-reply", json!([]));
    let chat = chat_for("run-failure-reply");
    let lease = RunLeaseGuard::acquire(
        Arc::clone(&store),
        "run-failure-reply",
        "owner-failure-reply",
    )
    .expect("acquire run lease");

    let failure = super::AdkChatPortError::Failed {
        // `persist_failure` is the non-retryable branch; retryable provider
        // outages go through `persist_provider_retry` and never reach it (see
        // `is_provider_retryable_error`).
        status: 400,
        code: "MODEL_CALL_FAILED".to_owned(),
        message: "provider down".to_owned(),
    };
    runtime
        .persist_failure(&chat, &failure, &lease)
        .expect("persist provider failure");

    let run = store
        .get_run("run-failure-reply")
        .expect("read run")
        .expect("run row");
    assert_eq!(run.status, "FAILED");
    let payload: Value = serde_json::from_str(&run.payload_json).expect("decode payload");
    assert_eq!(payload["status"], "FAILED");
    assert_eq!(payload["errorCode"], "MODEL_CALL_FAILED");
    assert_eq!(payload["errorMessage"], "provider down");
    assert_eq!(payload["degraded"], json!(true));

    let rows = audit_rows(&store);
    let (kind, subject_id, audit) = audit_row(&rows, "run.failed");
    assert_eq!(kind, "run.failed");
    assert_eq!(subject_id, "run-failure-reply");
    assert_eq!(audit["metadata"]["status"], "FAILED");
    assert_eq!(audit["metadata"]["errorCode"], "MODEL_CALL_FAILED");
    assert_eq!(audit["metadata"]["failureReason"], "provider down");
}

/// Go's `FinishPendingInputRun` parks the run as `PENDING_INPUT` with
/// `resumeState=waiting_input`, the fixed message and the input request, then
/// audits `run.awaiting_input` with `runId`/`agentId`/`status`/`requestId`
/// plus `internal/assistant/engine/google_runner_resume.go` `markDeniedResumedRun`.
/// Go's `FinishPendingInputRun` parks the run as `PENDING_INPUT` with
/// `resumeState=waiting_input`, the fixed message and the input request, then
/// audits `run.awaiting_input` with `runId`/`agentId`/`status`/`requestId`
/// plus the originating tool call's `decisionKind` — and deliberately does not
/// leak `blockingReason` into the audit metadata.
///
/// Reference: go:452dea11:internal/assistant/engine/input_request_test.go:584
/// `TestRequestUserToolPausesAndResumesChatRun` (the `run.awaiting_input`
/// assertion at `:605`), plus `runner_chat.go:267` `FinishPendingInputRun`.
#[test]
fn a_pending_input_run_audits_awaiting_input_with_the_decision_kind() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    create_running_run(&store, "run-input-audit", json!([]));
    let chat = chat_for("run-input-audit");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-input-audit", "owner-input")
        .expect("acquire run lease");

    let response = super::ModelResponse {
        text: String::new(),
        tool_calls: vec![super::ModelToolCall {
            id: "call-input".to_owned(),
            name: "interaction.request_user".to_owned(),
            arguments: json!({
                "title": "需要用户决策",
                "decisionKind": "material_tradeoff",
                "blockingReason": "两种方案无法合并",
                "questions": [{
                    "question": "选择哪套参数？",
                    "allowOther": true,
                    "options": [
                        {"label": "保守", "description": "", "recommended": true},
                        {"label": "激进", "description": "", "recommended": false},
                    ],
                }],
            }),
        }],
    };
    let staged = runtime
        .persist_tool_calls(&chat, &response, &lease)
        .expect("stage the pending input call");
    let super::ToolCallStaging::Pending(super::AdkChatPortOutput::Json(output)) = staged else {
        panic!("an input request must park the run: {staged:?}");
    };
    assert_eq!(
        output["reply"], "我需要你确认几个选择，回答后会继续执行。",
        "the park response carries the Go input prompt: {output}"
    );
    assert_eq!(output["run"]["status"], "PENDING_INPUT");

    let run = store
        .get_run("run-input-audit")
        .expect("read run")
        .expect("run row");
    assert_eq!(run.status, "PENDING_INPUT");
    let payload: Value = serde_json::from_str(&run.payload_json).expect("decode payload");
    assert_eq!(payload["resumeState"], "waiting_input");
    assert_eq!(payload["message"], "等待用户回答后继续执行。");

    let rows = audit_rows(&store);
    let (kind, subject_id, audit) = audit_row(&rows, "run.awaiting_input");
    assert_eq!(kind, "run.awaiting_input");
    assert_eq!(subject_id, "run-input-audit");
    assert_eq!(audit["detail"], "Agent run is waiting for user input.");
    assert_eq!(audit["metadata"]["runId"], "run-input-audit");
    assert_eq!(audit["metadata"]["agentId"], "agent-terminal");
    assert_eq!(audit["metadata"]["status"], "PENDING_INPUT");
    assert_eq!(audit["metadata"]["requestId"], output["inputRequest"]["id"]);
    assert_eq!(audit["metadata"]["decisionKind"], "material_tradeoff");
    assert!(
        audit["metadata"].get("blockingReason").is_none(),
        "the audit must not leak the blocking reason: {audit}"
    );
}

#[derive(Debug)]
struct NoopToolExecutor;

impl crate::product::product_adk_model_runtime::AdkToolExecutor for NoopToolExecutor {
    fn supports(&self, _name: &str) -> bool {
        false
    }

    fn execute(&self, name: &str, _arguments: &Value) -> Result<Value, String> {
        Err(format!("noop executor must not run {name}"))
    }
}

#[test]
fn a_denied_approval_audits_run_resumed_and_run_denied_with_the_denied_state() {
    let (directory, store, session_store) = initialized_stores();
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");

    // `with_tool_executor_for_test` does not start the durable recovery
    // scanner, so the regression drives `resume_approval` deterministically
    // instead of racing the startup supervisor.
    let runtime = ProductionAdkChatRuntime::with_tool_executor_for_test(
        Arc::clone(&store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
        Arc::new(NoopToolExecutor),
    );

    // `Runtime.resume_approval` only continues a RUNNING run whose tool call
    // was already resolved to DENIED by the mutation port.
    store
        .create_run(CreateAdkRunParams {
            id: "run-denied-audit",
            session_id: "session-run-denied-audit",
            agent_id: "agent-denied",
            status: "RUNNING",
            client_request_id: "request-run-denied-audit",
            request_fingerprint: "fingerprint-run-denied-audit",
            payload_json: &json!({
                "id": "run-denied-audit",
                "sessionId": "session-run-denied-audit",
                "agentId": "agent-denied",
                "status": "RUNNING",
                "route": "chat",
                "resumeState": "approval_resuming",
                "requestMessage": "denied tool",
                "toolCalls": [{
                    "id": "call-denied",
                    "name": "strategy.save_draft",
                    "arguments": {},
                    "status": "DENIED",
                    "requiresUser": false,
                }],
                "toolResults": [],
                "pendingApprovals": [],
            })
            .to_string(),
        })
        .expect("create denied run");

    runtime
        .resume_approval("run-denied-audit")
        .expect("resume the denied continuation");

    let run = store
        .get_run("run-denied-audit")
        .expect("read run")
        .expect("run row");
    assert_eq!(run.status, "DENIED");
    let payload: Value = serde_json::from_str(&run.payload_json).expect("decode payload");
    assert_eq!(payload["status"], "DENIED");
    assert_eq!(
        payload["resumeState"], "approval_denied",
        "Go stamps the denial resume state: {payload}"
    );
    assert_eq!(payload["message"], "approval denied");
    assert_eq!(payload["errorCode"], "");
    assert_eq!(payload["failureReason"], "");
    assert!(payload["completedAt"].is_string());

    let rows = audit_rows(&store);
    let (kind, subject_id, resumed) = audit_row(&rows, "run.resumed");
    assert_eq!(kind, "run.resumed");
    assert_eq!(subject_id, "run-denied-audit");
    assert_eq!(
        resumed["detail"],
        "Agent run resumed after approval resolution."
    );
    assert_eq!(resumed["metadata"]["resumeState"], "approval_denied");
    assert_eq!(resumed["metadata"]["status"], "DENIED");

    let (kind, subject_id, denied) = audit_row(&rows, "run.denied");
    assert_eq!(kind, "run.denied");
    assert_eq!(subject_id, "run-denied-audit");
    assert_eq!(
        denied["detail"],
        "Agent run reached a terminal state after approval resolution."
    );
    assert_eq!(denied["metadata"]["resumeState"], "approval_denied");
    assert_eq!(denied["metadata"]["status"], "DENIED");
}
