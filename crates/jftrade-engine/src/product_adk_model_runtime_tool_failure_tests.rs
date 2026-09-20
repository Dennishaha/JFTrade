//! Regression coverage for Go's "a failed tool does not fail the run" contract.
//!
//! Go's `googleADKExecution.afterToolCallback` records the failure on the
//! `ToolCall` and swallows the error, so the model keeps its turn.  The run
//! then terminates `COMPLETED` through `MarkCompletedChatRun`, which clears
//! `FailureReason`/`ErrorCode` and sets `Degraded = FirstToolCallFailure(run)
//! != ""`.  The Rust port instead failed the whole run on the first tool
//! error, which made the reply and the remaining tool rounds unreachable.

use std::fs::File;
use std::sync::Arc;
use std::time::Duration;

use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

use jftrade_store_sqlite::{AdkSessionStore, AdkStore, CreateAdkRunParams, initialize_current};

use super::{
    ProductionAdkChatRuntime, RunCancellationRegistry, classify_tool_failure,
    first_tool_call_failure, tool_error_text, tool_failed, tool_result_error_code,
    tool_result_error_message,
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

/// Go persists the tool failure as a visible `ToolCall` instead of a run-level
/// error, and `FirstToolCallFailure` is what later sets `degraded`.  The call
/// carries the raw error text (`disk full`), not the model-facing envelope
/// prefix or the code.
#[test]
fn a_failed_tool_result_is_persisted_on_the_call_not_the_run() {
    let (_directory, store, session_store) = initialized_stores();
    let settings_path = _directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        session_store,
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );
    store
        .create_run(CreateAdkRunParams {
            id: "run-tool-failure",
            session_id: "session-tool-failure",
            agent_id: "agent-tool-failure",
            status: "RUNNING",
            client_request_id: "request-tool-failure",
            request_fingerprint: "fp-tool-failure",
            payload_json: &json!({
                "id": "run-tool-failure",
                "status": "RUNNING",
                "route": "chat",
                "resumeState": "provider_executing",
                "toolCalls": [{
                    "id": "call-save-draft",
                    "name": "strategy.save_draft",
                    "arguments": {"name": "draft"},
                    "status": "RUNNING",
                    "requiresUser": false,
                }],
                "toolResults": [],
                "pendingApprovals": [],
            })
            .to_string(),
        })
        .expect("create run");

    // Drive the durable tool-result projection exactly as the continuation
    // loop does when a tool returns an error.
    let run = store
        .get_run("run-tool-failure")
        .expect("read run")
        .expect("run row");
    let lease = store
        .claim_run_lease(
            "run-tool-failure",
            "owner-tool-failure",
            Duration::from_secs(30),
        )
        .expect("claim run lease");
    let claim = store
        .claim_tool_invocation_if_status_and_revision(
            "run-tool-failure",
            "call-save-draft",
            "strategy.save_draft",
            r#"{"name":"draft"}"#,
            "RUNNING",
            &run.updated_at,
            "owner-tool-failure",
            lease.fencing_token,
            Duration::from_secs(30),
            false,
        )
        .expect("claim tool invocation");
    let super::AdkToolInvocationClaim::Execute(invocation) = claim else {
        panic!("a fresh tool invocation must be executable, got {claim:?}");
    };
    // The envelope shape Go's `toolErrorEnvelope` produces: the model-facing
    // `message` carries the verb prefix, nested `error.message` is the raw
    // text, and classification lands on the envelope codes.
    let failure = json!({
        "success": false,
        "message": "工具 strategy.save_draft 执行失败: disk full",
        "error": {
            "code": classify_tool_failure(&tool_failed("disk full")).0,
            "message": tool_error_text(&tool_failed("disk full")),
            "retryable": classify_tool_failure(&tool_failed("disk full")).1,
        },
        "errorCode": classify_tool_failure(&tool_failed("disk full")).0,
        "retryable": classify_tool_failure(&tool_failed("disk full")).1,
    });
    let event = jftrade_store_sqlite::AdkRunEvent {
        id: "run-tool-failure:tool:call-save-draft",
        session_id: "session-tool-failure",
        invocation_id: "run-tool-failure",
        author: "assistant.tool",
        content: "tool failed",
    };
    store
        .commit_tool_result_if_status_and_revision_with_event(
            "run-tool-failure",
            "RUNNING",
            &run.updated_at,
            &json!({
                "id": "run-tool-failure",
                "status": "RUNNING",
                "toolCalls": [{
                    "id": "call-save-draft",
                    "name": "strategy.save_draft",
                    "status": "FAILED",
                    "error": tool_result_error_message(&failure),
                    "errorCode": tool_result_error_code(&failure),
                }],
                "toolResults": [{
                    "callId": "call-save-draft",
                    "status": "FAILED",
                    "output": failure,
                }],
            })
            .to_string(),
            "call-save-draft",
            "strategy.save_draft",
            r#"{"name":"draft"}"#,
            &failure.to_string(),
            "FAILED",
            "owner-tool-failure",
            invocation.fencing_token,
            lease.fencing_token,
            runtime.session_store.as_ref(),
            &event,
        )
        .expect("persist the failed tool result");

    let stored = store
        .get_run("run-tool-failure")
        .expect("read run")
        .expect("run row");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("run payload");
    // The run itself is still RUNNING: only the tool call carries the failure.
    assert_eq!(stored.status, "RUNNING");
    assert_eq!(payload["toolCalls"][0]["status"], "FAILED");
    assert_eq!(payload["toolCalls"][0]["error"], "disk full");
    assert_eq!(
        payload["toolCalls"][0]["errorCode"],
        "TOOL_EXECUTION_FAILED"
    );
    assert!(
        payload.get("failureReason").is_none(),
        "a tool failure never writes a run-level failureReason: {payload}"
    );
    assert!(
        payload.get("errorCode").is_none(),
        "a tool failure never writes a run-level errorCode: {payload}"
    );

    // `FirstToolCallFailure` is what the completed projection turns into
    // `degraded: true`.
    assert_eq!(
        first_tool_call_failure(&stored.payload_json),
        Some("disk full".to_owned())
    );
}

/// Port of Go's `TestADKChatReturnsCompletedEnvelopeWithVisibleToolFailure`.
///
/// The durable shape is a run whose only tool call is `FAILED` with a
/// `disk full` error and a run projection that keeps
/// `failureReason`/`errorCode` empty while `degraded: true` is derived from
/// `FirstToolCallFailure`.  The chat route replays that stored envelope as-is.
#[test]
fn a_completed_run_with_a_failed_tool_replays_as_the_chat_envelope() {
    let (_directory, store, session_store) = initialized_stores();
    let settings_path = _directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );

    use crate::product::product_adk_chat_stream_port::{
        AdkChatInput, AdkChatPortOutput, AdkChatRoute, AdkChatStreamPort as _,
    };

    let run_id = "run-completed-tool-failure-chat";
    let client_request_id = "33333333-3333-4333-8333-333333333331";
    let route = "chat";
    let response = completed_tool_failure_response(run_id, client_request_id, route);
    let body = json!({
        "clientRequestId": client_request_id,
        "agentId": "agent-completed-tool-failure",
        "message": "@strategy.save_draft 保存失败草稿",
    })
    .to_string()
    .into_bytes();
    let fingerprint = super::fingerprint(&body);
    let payload = completed_tool_failure_payload(run_id, client_request_id, route, &response, true);
    store
        .create_run(CreateAdkRunParams {
            id: run_id,
            session_id: "session-completed-tool-failure",
            agent_id: "agent-completed-tool-failure",
            status: "COMPLETED",
            client_request_id,
            request_fingerprint: &fingerprint,
            payload_json: &payload.to_string(),
        })
        .expect("create completed run");

    let run_value = &response["run"];
    assert_eq!(run_value["status"], "COMPLETED");
    assert_eq!(
        run_value["message"], "completed",
        "MarkCompletedChatRun writes the literal `completed`"
    );
    assert_eq!(run_value["degraded"], json!(true));
    assert_eq!(
        run_value["failureReason"], "",
        "a tool failure never becomes a run failure"
    );
    assert_eq!(run_value["errorCode"], "");
    assert_eq!(run_value["toolCalls"][0]["status"], "FAILED");
    assert!(
        run_value["toolCalls"][0]["error"]
            .as_str()
            .is_some_and(|error| error.contains("disk full"))
    );

    let input = AdkChatInput {
        body,
        client_request_id: client_request_id.to_owned(),
    };
    match runtime.dispatch(AdkChatRoute::Chat, &input) {
        Ok(AdkChatPortOutput::Json(replayed)) => {
            assert_eq!(
                replayed, response,
                "the chat route must replay the stored completed envelope"
            );
        }
        outcome => panic!("unexpected chat outcome: {outcome:?}"),
    }
}

/// Port of Go's `TestADKChatStreamReturnsFinalEventForCompletedRunWithToolFailure`.
///
/// The stream route must publish the persisted completed run as the terminal
/// `final` frame carrying the stored response, and it must never fabricate an
/// `error` frame for a run whose only failure was a tool call.
#[test]
fn a_completed_run_with_a_failed_tool_replays_as_a_stream_final_frame() {
    let (_directory, store, session_store) = initialized_stores();
    let settings_path = _directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );

    use crate::product::product_adk_chat_stream_port::{
        AdkChatInput, AdkChatPortOutput, AdkChatRoute, AdkChatStreamFrame, AdkChatStreamPort as _,
    };

    let run_id = "run-completed-tool-failure-stream";
    let client_request_id = "33333333-3333-4333-8333-333333333332";
    let route = "stream";
    let response = completed_tool_failure_response(run_id, client_request_id, route);
    let body = json!({
        "clientRequestId": client_request_id,
        "agentId": "agent-completed-tool-failure",
        "message": "@strategy.save_draft 保存失败草稿",
    })
    .to_string()
    .into_bytes();
    let fingerprint = super::fingerprint(&body);
    let payload = completed_tool_failure_payload(run_id, client_request_id, route, &response, true);
    store
        .create_run(CreateAdkRunParams {
            id: run_id,
            session_id: "session-completed-tool-failure",
            agent_id: "agent-completed-tool-failure",
            status: "COMPLETED",
            client_request_id,
            request_fingerprint: &fingerprint,
            payload_json: &payload.to_string(),
        })
        .expect("create completed run");

    let input = AdkChatInput {
        body,
        client_request_id: client_request_id.to_owned(),
    };
    match runtime.dispatch(AdkChatRoute::Stream, &input) {
        Ok(AdkChatPortOutput::Stream(snapshot)) => {
            assert!(snapshot.terminal, "a completed stream run is terminal");
            assert_eq!(snapshot.headers["X-ADK-Stream-ID"], run_id);
            let Some(AdkChatStreamFrame::Event { data, .. }) = snapshot.frames.last() else {
                panic!("stream snapshot must end with an event frame");
            };
            assert_eq!(
                data["type"], "final",
                "a failed tool must end with final, never with error"
            );
            assert_eq!(data["response"], response);
            assert!(
                !snapshot.frames.iter().any(|frame| matches!(
                    frame,
                    AdkChatStreamFrame::Event { data, .. } if data["type"] == "error"
                )),
                "no error frame may be fabricated for a completed run"
            );
        }
        outcome => panic!("unexpected stream outcome: {outcome:?}"),
    }
}

/// Port of Go's
/// `TestADKChatStreamRecoversCompletedRunAsFinalEventWhenFinalMessageAppendFails`.
///
/// Go's recovery path rebuilds the terminal projection from the durable run
/// (`RecoverTerminalChatResponse`) whenever the stream event list never
/// received its `final` frame.  Rust has no ADK-session `AppendEvent` injection
/// point, so the equivalent durable shape is a terminal payload whose
/// `streamEvents` stop at the run frame: `stream_from_payload` must synthesize
/// the missing `final` frame from the persisted response.
#[test]
fn a_terminal_run_without_a_stream_final_frame_recovers_a_final_frame() {
    let (_directory, store, session_store) = initialized_stores();
    let settings_path = _directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );

    use crate::product::product_adk_chat_stream_port::{
        AdkChatInput, AdkChatPortOutput, AdkChatRoute, AdkChatStreamFrame, AdkChatStreamPort as _,
    };

    let run_id = "run-completed-tool-failure-recovery";
    let client_request_id = "33333333-3333-4333-8333-333333333333";
    let route = "stream";
    let response = completed_tool_failure_response(run_id, client_request_id, route);
    let body = json!({
        "clientRequestId": client_request_id,
        "agentId": "agent-completed-tool-failure",
        "message": "@strategy.save_draft 保存失败草稿",
    })
    .to_string()
    .into_bytes();
    let fingerprint = super::fingerprint(&body);
    // The terminal append failed in Go, so the persisted run has only the
    // `run` frame; the durable `response` is still there to recover from.
    let payload =
        completed_tool_failure_payload(run_id, client_request_id, route, &response, false);
    store
        .create_run(CreateAdkRunParams {
            id: run_id,
            session_id: "session-completed-tool-failure",
            agent_id: "agent-completed-tool-failure",
            status: "COMPLETED",
            client_request_id,
            request_fingerprint: &fingerprint,
            payload_json: &payload.to_string(),
        })
        .expect("create completed run");

    let input = AdkChatInput {
        body,
        client_request_id: client_request_id.to_owned(),
    };
    match runtime.dispatch(AdkChatRoute::Stream, &input) {
        Ok(AdkChatPortOutput::Stream(snapshot)) => {
            assert!(snapshot.terminal, "a completed stream run is terminal");
            assert_eq!(snapshot.headers["X-ADK-Stream-ID"], run_id);
            assert_eq!(
                snapshot.frames.len(),
                2,
                "the run frame plus the recovered final frame"
            );
            let Some(AdkChatStreamFrame::Event { data, .. }) = snapshot.frames.last() else {
                panic!("stream snapshot must end with an event frame");
            };
            assert_eq!(data["type"], "final");
            assert_eq!(data["sequence"], 2);
            assert_eq!(data["response"], response);
            assert!(
                !snapshot.frames.iter().any(|frame| matches!(
                    frame,
                    AdkChatStreamFrame::Event { data, .. } if data["type"] == "error"
                )),
                "recovery must publish final, never synthesize an error frame"
            );
        }
        outcome => panic!("unexpected stream outcome: {outcome:?}"),
    }
}

/// The durable run payload for the completed-with-tool-failure family.
///
/// `include_final_frame` controls whether the persisted `streamEvents` already
/// carry the terminal `final` frame (the normal path) or stop right after the
/// `run` frame (Go's failed final-append recovery path).
fn completed_tool_failure_payload(
    run_id: &str,
    client_request_id: &str,
    route: &str,
    response: &Value,
    include_final_frame: bool,
) -> Value {
    let mut stream_events =
        vec![json!({"type": "run", "streamId": run_id, "sequence": 1, "status": "RUNNING"})];
    if include_final_frame {
        stream_events.push(json!({
            "type": "final",
            "streamId": run_id,
            "sequence": 2,
            "response": response.clone(),
        }));
    }
    json!({
        "id": run_id,
        "sessionId": "session-completed-tool-failure",
        "agentId": "agent-completed-tool-failure",
        "status": "COMPLETED",
        "message": "completed",
        "reply": response["reply"],
        "degraded": true,
        "failureReason": "",
        "errorCode": "",
        "pendingApprovals": [],
        "route": route,
        "clientRequestId": client_request_id,
        "streamId": run_id,
        "toolCalls": [{
            "id": "call-save-draft",
            "name": "strategy.save_draft",
            "status": "FAILED",
            "error": "disk full",
            "errorCode": "TOOL_EXECUTION_FAILED",
            "requiresUser": false,
        }],
        "streamEvents": stream_events,
        "response": response.clone(),
    })
}

/// The durable chat envelope Go's `MarkCompletedChatRun` +
/// `ProjectedChatResponse` produce for a run whose only tool call failed.
fn completed_tool_failure_response(run_id: &str, client_request_id: &str, route: &str) -> Value {
    json!({
        "reply": "保存失败，请检查磁盘空间。",
        "session": {
            "id": "session-completed-tool-failure",
            "agentId": "agent-completed-tool-failure",
        },
        "run": {
            "id": run_id,
            "sessionId": "session-completed-tool-failure",
            "agentId": "agent-completed-tool-failure",
            "clientRequestId": client_request_id,
            "status": "COMPLETED",
            "message": "completed",
            "reply": "保存失败，请检查磁盘空间。",
            "degraded": true,
            "failureReason": "",
            "errorCode": "",
            "pendingApprovals": [],
            "route": route,
            "toolCalls": [{
                "id": "call-save-draft",
                "name": "strategy.save_draft",
                "status": "FAILED",
                "error": "disk full",
                "errorCode": "TOOL_EXECUTION_FAILED",
                "requiresUser": false,
            }],
        },
        "pendingApprovals": [],
        "timeline": [],
    })
}

/// `persist_success` is the Rust port of Go's `MarkCompletedChatRun`.  It must
/// derive `degraded` from the durable tool calls, clear the run-level failure
/// projection, and keep the failed `ToolCall` visible on the completed run.
#[test]
fn persist_success_marks_a_run_degraded_from_its_failed_tool_calls() {
    let (_directory, store, session_store) = initialized_stores();
    let settings_path = _directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );
    store
        .create_run(CreateAdkRunParams {
            id: "run-persist-success",
            session_id: "session-persist-success",
            agent_id: "agent-persist-success",
            status: "RUNNING",
            client_request_id: "request-persist-success",
            request_fingerprint: "fp-persist-success",
            payload_json: &json!({
                "id": "run-persist-success",
                "status": "RUNNING",
                "message": "RUNNING",
                "failureReason": "stale run-level failure",
                "errorCode": "STALE_ERROR_CODE",
                "errorMessage": "stale",
                "errorStatus": 500,
                "pendingApprovals": [{"id": "approval-stale"}],
                "providerRetry": {"attempt": 1},
                "toolCalls": [{
                    "id": "call-save-draft",
                    "name": "strategy.save_draft",
                    "status": "FAILED",
                    "error": "disk full",
                    "errorCode": "TOOL_EXECUTION_FAILED",
                    "requiresUser": false,
                }],
            })
            .to_string(),
        })
        .expect("create running run");
    let run_lease = super::RunLeaseGuard::acquire(
        Arc::clone(&store),
        "run-persist-success",
        "owner-persist-success",
    )
    .expect("acquire run lease");
    let chat = super::ChatExecution {
        route: crate::product::product_adk_chat_stream_port::AdkChatRoute::Chat,
        run_id: "run-persist-success".to_owned(),
        session_id: "session-persist-success".to_owned(),
        agent_id: "agent-persist-success".to_owned(),
        resumed: false,
        permission_mode: "approval".to_owned(),
        context_deltas: Vec::new(),
        request: super::ModelRequest {
            endpoint: reqwest::Url::parse("http://127.0.0.1:1/v1/responses").expect("fixture url"),
            api_key: "sk-fixture".to_owned(),
            model: "fixture-model".to_owned(),
            instruction: None,
            message: "保存策略草稿".to_owned(),
            durable_context: Vec::new(),
            tool_context: Vec::new(),
            timeout: Duration::from_secs(1),
            tools: Vec::new(),
        },
    };
    let response = runtime
        .persist_success(
            &chat,
            super::ModelResponse {
                text: "保存失败，请检查磁盘空间。".to_owned(),
                tool_calls: Vec::new(),
            },
            &run_lease,
        )
        .expect("a failed tool must not stop the completed projection");

    let run_value = &response["run"];
    assert_eq!(run_value["status"], "COMPLETED");
    assert_eq!(run_value["message"], "completed");
    // Go's `assistantmodel.Run` has no `reply` field: the assistant text lives
    // on the response envelope, and the run links it through `finalMessageId`.
    assert_eq!(response["reply"], "保存失败，请检查磁盘空间。");
    assert!(
        run_value.get("reply").is_none(),
        "the run wire shape carries no reply field: {run_value}"
    );
    assert_eq!(
        run_value["degraded"],
        json!(true),
        "a failed tool call marks the completed run degraded"
    );
    // `MarkCompletedChatRun` clears the run-level failure projection and Go's
    // `omitempty` then drops the empty strings from the wire.
    assert!(
        run_value.get("failureReason").is_none(),
        "a cleared failure reason is omitted: {run_value}"
    );
    assert!(
        run_value.get("errorCode").is_none(),
        "a cleared error code is omitted: {run_value}"
    );
    assert_eq!(run_value["pendingApprovals"], json!([]));
    assert_eq!(run_value["toolCalls"][0]["status"], "FAILED");
    assert_eq!(run_value["toolCalls"][0]["error"], "disk full");
    // `ToolSummariesForRun` renders the failure as Go's
    // `SummarizeToolOutput` line so the console can show it beside the call.
    assert_eq!(
        run_value["toolSummaries"],
        json!(["strategy.save_draft failed: disk full"])
    );

    let stored = store
        .get_run("run-persist-success")
        .expect("read run")
        .expect("run row");
    assert_eq!(stored.status, "COMPLETED");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("run payload");
    assert_eq!(payload["degraded"], json!(true));
    assert_eq!(payload["message"], "completed");
    assert_eq!(payload["failureReason"], "");
    assert_eq!(payload["errorCode"], "");
    assert!(
        payload.get("errorStatus").is_none(),
        "the stale failure status must be cleared: {payload}"
    );
    assert!(
        payload.get("providerRetry").is_none(),
        "a completed run drops the provider retry marker: {payload}"
    );
}

/// Go's `TestChatContinuesAfterToolFailure` only ever swallows the tool error:
/// `afterToolCallback` records it and the model keeps its turn.  The Rust port
/// must therefore never convert a tool failure into a run-level abort, and the
/// classification must match Go's `classifyToolError` table.
#[test]
fn tool_failure_classification_matches_the_reference_table() {
    for (error, expected_code, expected_retryable) in [
        (tool_failed("disk full"), "TOOL_EXECUTION_FAILED", false),
        (
            super::AdkChatPortError::Failed {
                status: 504,
                code: "MODEL_CALL_TIMEOUT".to_owned(),
                message: "assistant model request timed out".to_owned(),
            },
            "TIMEOUT",
            true,
        ),
        (
            super::AdkChatPortError::Failed {
                status: 499,
                code: "RUN_CANCELLED".to_owned(),
                message: "assistant chat run was cancelled".to_owned(),
            },
            "CANCELLED",
            false,
        ),
        (
            super::AdkChatPortError::Failed {
                status: 500,
                code: "ADK_TOOL_OUTCOME_UNKNOWN".to_owned(),
                message: "tool outcome is unknown".to_owned(),
            },
            "SUBMISSION_UNKNOWN",
            false,
        ),
        (
            super::AdkChatPortError::Failed {
                status: 409,
                code: "ADK_RUN_LEASE_LOST".to_owned(),
                message: "run lease lost".to_owned(),
            },
            "RUN_LEASE_LOST",
            true,
        ),
    ] {
        assert_eq!(
            classify_tool_failure(&error),
            (expected_code, expected_retryable)
        );
    }
    // `ToolCall.Error` carries the raw text with no code prefix.
    assert_eq!(tool_error_text(&tool_failed("disk full")), "disk full");
    assert_eq!(
        tool_error_text(&super::AdkChatPortError::Unavailable(
            "runtime down".to_owned()
        )),
        "runtime down"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/store_ops_test.go:565
/// TestPendingApprovalResumesThroughGoogleADKAfterRuntimeRestart.
///
/// Go persists the resolved approval, rehydrates the run on the restarted
/// runtime, executes the released tool exactly once, and finishes the run
/// `COMPLETED` with `resumeState=adk_confirmation_resolved`.  The Rust terminal
/// projection dropped that resume state, so a resumed run looked like a plain
/// chat run in the console and in the metrics `resumed` counter.
#[test]
fn a_resumed_approval_run_completes_with_the_confirmation_resolved_state() {
    let (_directory, store, session_store) = initialized_stores();
    let settings_path = _directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    store
        .create_run(CreateAdkRunParams {
            id: "run-resumed-projection",
            session_id: "session-resumed-projection",
            agent_id: "agent-resumed-projection",
            status: "RUNNING",
            client_request_id: "request-resumed-projection",
            request_fingerprint: "fingerprint-resumed-projection",
            payload_json: &json!({
                "id": "run-resumed-projection",
                "sessionId": "session-resumed-projection",
                "agentId": "agent-resumed-projection",
                "status": "RUNNING",
                "route": "chat",
                "resumeState": "approval_resuming",
                "requestMessage": "resume the gated tool",
                "toolCalls": [{
                    "id": "call-resumed",
                    "name": "strategy.save_draft",
                    "arguments": {"name": "draft"},
                    "status": "RUNNING",
                    "requiresUser": false,
                }],
                "toolResults": [],
                "pendingApprovals": [],
            })
            .to_string(),
        })
        .expect("create resumed run");

    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        session_store,
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );
    let lease = store
        .claim_run_lease(
            "run-resumed-projection",
            "owner-resumed-projection",
            Duration::from_secs(30),
        )
        .expect("claim run lease");
    let chat = super::ChatExecution {
        route: crate::product::product_adk_chat_stream_port::AdkChatRoute::Chat,
        run_id: "run-resumed-projection".to_owned(),
        session_id: "session-resumed-projection".to_owned(),
        agent_id: "agent-resumed-projection".to_owned(),
        resumed: true,
        permission_mode: "approval".to_owned(),
        context_deltas: Vec::new(),
        request: super::ModelRequest {
            endpoint: reqwest::Url::parse("http://127.0.0.1:1/v1/responses").expect("endpoint"),
            api_key: "sk-fixture".to_owned(),
            model: "fixture-model".to_owned(),
            instruction: None,
            message: "resume the gated tool".to_owned(),
            durable_context: Vec::new(),
            tool_context: Vec::new(),
            timeout: Duration::from_secs(1),
            tools: Vec::new(),
        },
    };
    let run_lease =
        super::RunLeaseGuard::from_lease(Arc::clone(&store), lease).expect("wrap run lease");
    let response = runtime
        .persist_success(
            &chat,
            super::ModelResponse {
                text: "resumed reply".to_owned(),
                tool_calls: Vec::new(),
            },
            &run_lease,
        )
        .expect("persist resumed success");
    assert_eq!(
        response["run"]["status"], "COMPLETED",
        "a resumed run still completes: {response}"
    );
    assert_eq!(
        response["run"]["resumeState"], "adk_confirmation_resolved",
        "the terminal envelope records the confirmation resume: {response}"
    );
    assert!(
        response["run"]["completedAt"].is_string(),
        "the terminal projection stamps completedAt: {response}"
    );

    let stored = store
        .get_run("run-resumed-projection")
        .expect("read completed run")
        .expect("run row");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("run payload");
    assert_eq!(payload["status"], "COMPLETED");
    assert_eq!(
        payload["resumeState"], "adk_confirmation_resolved",
        "the durable payload keeps the confirmation state: {payload}"
    );
    assert!(payload["completedAt"].is_string());

    // Go's `auditResumedRun` runs before the terminal persistence step, so an
    // approved continuation records `run.resumed` with the resolved
    // `resumeState` ahead of `run.completed`.
    let rows = store.list_audit_events().expect("list audit events");
    let resumed = rows
        .iter()
        .find(|row| row.kind == "run.resumed")
        .unwrap_or_else(|| panic!("a resumed run must audit run.resumed: {rows:?}"));
    assert_eq!(resumed.subject_id, "run-resumed-projection");
    let resumed_payload: Value =
        serde_json::from_str(&resumed.payload_json).expect("resumed audit payload");
    assert_eq!(
        resumed_payload["metadata"]["resumeState"], "adk_confirmation_resolved",
        "the resume audit records the resolved state: {resumed_payload}"
    );
    assert!(
        rows.iter().any(|row| row.kind == "run.completed"),
        "the resumed run still audits its terminal kind: {rows:?}"
    );

    drop(run_lease);
    let _ = runtime;
}
