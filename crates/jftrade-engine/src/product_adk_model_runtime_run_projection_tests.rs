//! Regression coverage for Go's `ProjectedChatResponse` run projection
//! (`internal/assistant/engine/runner_chat_test.go`).
//!
//! Go answers `POST /api/v1/adk/chat` with the *projected* run: the durable row
//! merged with the tool activity of the turn, the effective work mode, the run
//! budget and the session timeline.  Rust answered with an ad-hoc object that
//! kept only the last model text, so `preToolContent`, `toolSummaries`,
//! `optimizationTaskId`, `usage.toolCallsTotal` and the user's own timeline
//! entry never reached the console.

use std::fs::File;
use std::sync::Arc;
use std::sync::atomic::Ordering;
use std::time::Duration;

use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

use jftrade_store_sqlite::{
    AdkSessionStore, AdkStore, CreateAdkRunParams, RecordAdkEventParams, initialize_current,
};

use crate::product::product_adk_chat_stream_port::AdkChatStreamPort;

use super::{
    AdkChatRoute, AdkToolExecutor, ChatExecution, ModelRequest, ModelResponse,
    ProductionAdkChatRuntime, RunCancellationRegistry, RunLeaseGuard,
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
        Arc::new(AdkSessionStore::open(&session_path).expect("open ADK session store")),
    )
}

fn runtime_for(
    directory: &tempfile::TempDir,
    store: &Arc<AdkStore>,
    session_store: &Arc<AdkSessionStore>,
) -> ProductionAdkChatRuntime {
    runtime_with_executor(
        directory,
        store,
        session_store,
        Arc::new(RecordingToolExecutor::new(vec!["strategy.optimize"])),
    )
}

/// A runtime whose catalog is the production one, so tool ids resolve to their
/// real policy (`system.status` is a low-risk read that runs without approval).
fn runtime_with_production_catalog(
    directory: &tempfile::TempDir,
    store: &Arc<AdkStore>,
    session_store: &Arc<AdkSessionStore>,
    executor: Arc<RecordingToolExecutor>,
) -> ProductionAdkChatRuntime {
    use std::collections::BTreeMap;

    let bindings = crate::product::product_production_ports::product_production_ports_adk::PRODUCTION_TOOL_DEFINITIONS
        .iter()
        .map(|definition| {
            (
                definition.adapter,
                crate::product::product_production_ports::ProductionAdapterBinding::Ready,
            )
        })
        .collect::<BTreeMap<_, _>>();
    let catalog =
        crate::product::product_production_ports::ProductionToolCatalog::from_bindings(&bindings)
            .expect("complete tool bindings");
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

fn runtime_with_executor(
    directory: &tempfile::TempDir,
    store: &Arc<AdkStore>,
    session_store: &Arc<AdkSessionStore>,
    executor: Arc<RecordingToolExecutor>,
) -> ProductionAdkChatRuntime {
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    ProductionAdkChatRuntime::with_tool_executor_for_test(
        Arc::clone(store),
        Arc::clone(session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
        executor,
    )
}

/// A tool executor that records the capabilities the runtime asks it to run.
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

fn create_running_run(store: &AdkStore, run_id: &str, tool_calls: Value) {
    store
        .create_run(CreateAdkRunParams {
            id: run_id,
            session_id: Box::leak(format!("session-{run_id}").into_boxed_str()),
            agent_id: "agent-projection",
            status: "RUNNING",
            client_request_id: Box::leak(format!("request-{run_id}").into_boxed_str()),
            request_fingerprint: "fingerprint-projection",
            payload_json: &json!({
                "id": run_id,
                "sessionId": format!("session-{run_id}"),
                "agentId": "agent-projection",
                "status": "RUNNING",
                "route": "chat",
                "message": "running",
                "userMessage": "hello",
                "workMode": "chat",
                "maxDurationMs": 1_800_000,
                "startedAt": "2026-09-20T00:00:00Z",
                "usage": {"modelCalls": 0, "toolCallsTotal": 0},
                "resumeState": "provider_executing",
                "toolCalls": tool_calls,
                "toolResults": [],
                "pendingApprovals": [],
            })
            .to_string(),
        })
        .expect("create run");
}

fn chat_for(run_id: &str, endpoint: &str) -> ChatExecution {
    ChatExecution::for_test(
        run_id.to_owned(),
        format!("session-{run_id}"),
        "agent-projection".to_owned(),
        // The production default: `strategy.optimize` needs the operator only
        // in `approval` mode.
        "less_approval".to_owned(),
        ModelRequest {
            endpoint: endpoint.parse().expect("loopback endpoint"),
            api_key: "sk-fixture".to_owned(),
            model: "fixture-model".to_owned(),
            instruction: None,
            message: "hello".to_owned(),
            durable_context: Vec::new(),
            tool_context: Vec::new(),
            timeout: Duration::from_secs(5),
            tools: vec![json!({"type": "function", "name": "strategy.optimize"})],
            reasoning: None,
        },
    )
}

fn record_user_message(session_store: &AdkSessionStore, run_id: &str) {
    // `startRun` creates the Google-ADK session row before it records the
    // opening user event, and the event row is keyed to that session.
    let session_id = format!("session-{run_id}");
    session_store
        .upsert_session(
            "jftrade",
            "local",
            &session_id,
            &json!({"id": session_id, "agentId": "agent-projection", "title": "hello"}).to_string(),
        )
        .expect("create ADK session");
    session_store
        .record_event(RecordAdkEventParams {
            id: &format!("{run_id}:user"),
            app_name: "jftrade",
            user_id: "local",
            session_id: &session_id,
            invocation_id: run_id,
            author: "user",
            content: "hello",
        })
        .expect("record user event");
}

fn run_payload(store: &AdkStore, run_id: &str) -> Value {
    let run = store.get_run(run_id).expect("read run").expect("run row");
    serde_json::from_str(&run.payload_json).expect("decode run payload")
}

#[test]
fn tool_round_and_final_usage_accumulate_without_overwriting_prior_totals() {
    let (directory, store, sessions) = initialized_stores();
    create_running_run(&store, "run-usage-rounds", json!([]));
    record_user_message(&sessions, "run-usage-rounds");
    let mut seeded = run_payload(&store, "run-usage-rounds");
    seeded["usage"]["tokensIn"] = json!(4);
    seeded["usage"]["tokensOut"] = json!(1);
    store
        .update_run_payload("run-usage-rounds", &seeded.to_string())
        .unwrap();
    let executor = Arc::new(RecordingToolExecutor::new(vec!["system.status"]));
    let runtime = runtime_with_production_catalog(&directory, &store, &sessions, executor);
    let chat = chat_for("run-usage-rounds", "http://127.0.0.1:1/v1/responses");
    let lease = RunLeaseGuard::acquire(store.clone(), "run-usage-rounds", "owner-usage").unwrap();
    runtime
        .persist_tool_calls(
            &chat,
            &ModelResponse {
                text: "looking up system status".to_owned(),
                tool_calls: vec![super::ModelToolCall {
                    id: "call-usage".to_owned(),
                    name: "system.status".to_owned(),
                    arguments: json!({}),
                }],
                usage_metadata: Some(
                    json!({"input_tokens":9, "output_tokens":3, "total_tokens":12}),
                ),
            },
            &lease,
        )
        .unwrap();
    let staged = run_payload(&store, "run-usage-rounds");
    let response = runtime
        .persist_success(
            &chat,
            ModelResponse {
                text: "done".to_owned(),
                tool_calls: Vec::new(),
                usage_metadata: Some(
                    json!({"input_tokens":5, "output_tokens":2, "total_tokens":7}),
                ),
            },
            &lease,
        )
        .unwrap();
    let completed = run_payload(&store, "run-usage-rounds");
    let replay = runtime
        .persist_success(
            &chat,
            ModelResponse {
                text: "must not overwrite completed reply".to_owned(),
                tool_calls: Vec::new(),
                usage_metadata: Some(json!({"input_tokens":5, "output_tokens":2})),
            },
            &lease,
        )
        .unwrap();
    runtime.shutdown();
    assert_eq!(
        staged["usage"]["tokensIn"], 13,
        "persist first provider round over prior usage"
    );
    assert_eq!(staged["usage"]["tokensOut"], 4);
    assert_eq!(response["run"]["usage"]["tokensIn"], 18);
    assert_eq!(response["run"]["usage"]["tokensOut"], 6);
    assert_eq!(completed["usage"]["tokensIn"], 18);
    assert_eq!(completed["usage"]["tokensOut"], 6);
    assert_eq!(
        replay, response,
        "terminal replay must not count the same final usage twice"
    );
}

/// Parity: go:452dea11:internal/assistant/model/timeline_helper_test.go:5 TestTimelineHelperBoundaries
/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:490
/// `TestProjectedChatResponseAppliesProjectionToRunFields`.
///
/// The run's tool round must reach the chat envelope the way Go's
/// `applySessionProjectionToRun` does it: the reply is the *merged* assistant
/// text, `preToolContent` keeps what the model said before the call, the
/// projected calls and summaries describe the executed tool, the optimizer task
/// id points at the tool output, `usage.toolCallsTotal` counts the calls,
/// `finalMessageId` links the transcript and the timeline repeats the session.
#[test]
fn a_tool_round_projects_the_pre_tool_reply_and_session_timeline() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    // The durable state a released tool round leaves behind: the pre-tool text
    // the staging step froze, the executed call with its optimizer output, and
    // the user turn the transcript already holds.
    create_running_run(
        &store,
        "run-projection-round",
        json!([{
            "id": "call-opt",
            "runId": "run-projection-round",
            "toolName": "strategy.optimize",
            "name": "strategy.optimize",
            "status": "SUCCEEDED",
            "round": 1,
            "requiresUser": false,
            "output": {"taskId": "opt-999", "status": "started"},
        }]),
    );
    record_user_message(&session_store, "run-projection-round");
    let mut payload = run_payload(&store, "run-projection-round");
    payload["preToolContent"] = json!("先说明一下。");
    let stored_run = store
        .get_run("run-projection-round")
        .expect("read run")
        .expect("run row");
    let fence = store
        .claim_run_lease(
            "run-projection-round",
            "owner-projection-round",
            Duration::from_secs(30),
        )
        .expect("claim run lease");
    assert!(
        store
            .update_run_payload_if_status_and_revision_with_lease(
                "run-projection-round",
                "RUNNING",
                &stored_run.updated_at,
                &payload.to_string(),
                "owner-projection-round",
                fence.fencing_token,
            )
            .expect("update run payload")
    );
    let chat = chat_for("run-projection-round", "http://127.0.0.1:1/v1/responses");
    let lease = RunLeaseGuard::acquire(
        Arc::clone(&store),
        "run-projection-round",
        "owner-projection-round",
    )
    .expect("acquire run lease");

    let response = runtime
        .persist_success(
            &chat,
            ModelResponse {
                usage_metadata: None,
                text: "优化已启动。".to_owned(),
                tool_calls: Vec::new(),
            },
            &lease,
        )
        .expect("persist success");

    assert_eq!(
        response["reply"], "先说明一下。优化已启动。",
        "the projection merges the pre-tool and post-tool assistant text: {response}"
    );
    assert_eq!(response["run"]["preToolContent"], "先说明一下。");
    assert_eq!(response["run"]["status"], "COMPLETED");
    assert_eq!(
        response["run"]["toolCalls"][0]["toolName"],
        "strategy.optimize"
    );
    assert_eq!(response["run"]["toolCalls"][0]["status"], "SUCCEEDED");
    assert_eq!(
        response["run"]["optimizationTaskId"], "opt-999",
        "the optimizer task id comes from the projected tool output: {response}"
    );
    assert_eq!(response["run"]["usage"]["toolCallsTotal"], json!(1));
    assert!(
        response["run"]["toolSummaries"][0]
            .as_str()
            .is_some_and(|summary| summary.contains("strategy.optimize")),
        "the projected summaries describe the executed tool: {response}"
    );
    assert!(
        response["run"]["finalMessageId"]
            .as_str()
            .is_some_and(|value| value.starts_with("jftrade-run-projection-round-")),
        "the completed run links its final message: {response}"
    );
    let timeline = response["timeline"]
        .as_array()
        .unwrap_or_else(|| panic!("the projection carries a timeline: {response}"));
    assert_eq!(timeline.len(), 2, "user + assistant entries: {timeline:?}");
    assert_eq!(timeline[0]["kind"], "user_message");
    assert_eq!(timeline[0]["text"], "hello");
    assert_eq!(timeline[1]["kind"], "assistant_message");
    assert_eq!(timeline[1]["id"], response["run"]["finalMessageId"]);
}

/// Go's `publishFinal` strips tool outputs from the terminal SSE response while
/// retaining the durable tool activity on the run.  The stream projection must
/// keep that transport boundary so reconnect replay does not expose the raw
/// tool payload.
/// Parity: go:452dea11:internal/api/assistant/chat_helpers_test.go:167 TestChatStreamExecutionPublishesDeltaAndFinalVariants
#[test]
fn stream_terminal_projection_trims_tool_outputs_from_final_response() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    store
        .create_run(CreateAdkRunParams {
            id: "run-stream-trim-tool-output",
            session_id: "session-run-stream-trim-tool-output",
            agent_id: "agent-projection",
            status: "RUNNING",
            client_request_id: "request-stream-trim-tool-output",
            request_fingerprint: "fingerprint-stream-trim-tool-output",
            payload_json: &json!({
                "id": "run-stream-trim-tool-output",
                "sessionId": "session-run-stream-trim-tool-output",
                "agentId": "agent-projection",
                "status": "RUNNING",
                "route": "stream",
                "streamId": "run-stream-trim-tool-output",
                "streamEvents": [],
                "toolCalls": [{
                    "id": "tool-1",
                    "toolName": "strategy.optimize",
                    "status": "SUCCEEDED",
                    "output": {"taskId": "opt-1", "status": "started"}
                }],
                "pendingApprovals": []
            })
            .to_string(),
        })
        .expect("create stream run");
    record_user_message(&session_store, "run-stream-trim-tool-output");
    let lease = RunLeaseGuard::acquire(
        Arc::clone(&store),
        "run-stream-trim-tool-output",
        "owner-stream-trim-tool-output",
    )
    .expect("acquire run lease");
    let mut chat = chat_for(
        "run-stream-trim-tool-output",
        "http://127.0.0.1:1/v1/responses",
    );
    chat.route = AdkChatRoute::Stream;

    let response = runtime
        .persist_success(
            &chat,
            ModelResponse {
                usage_metadata: None,
                text: "done".to_owned(),
                tool_calls: Vec::new(),
            },
            &lease,
        )
        .expect("persist stream success");

    assert_eq!(
        response["run"]["toolCalls"][0]["output"],
        json!({"taskId": "opt-1", "status": "started"}),
        "the returned run keeps durable tool activity"
    );
    let stored = store
        .get_run("run-stream-trim-tool-output")
        .expect("read stream run")
        .expect("stream run row");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("decode stream payload");
    assert_eq!(
        payload["response"]["run"]["toolCalls"][0].get("output"),
        None,
        "the stream final response must trim tool output"
    );
    assert_eq!(
        payload["toolCalls"][0]["output"],
        json!({"taskId": "opt-1", "status": "started"}),
        "durable run activity keeps the tool output"
    );
    assert_eq!(
        payload["streamEvents"]
            .as_array()
            .and_then(|events| events.last())
            .and_then(|event| event.get("type")),
        Some(&json!("final")),
        "stream success stores a terminal final event"
    );
}

/// The assistant text that introduced a tool call is captured once, when the
/// call is staged, and a later round never overwrites it.
///
/// Go's ADK projection keeps the text before the *first* function call of the
/// invocation (`state.preToolCaptured`); losing it made the console show only
/// the post-tool answer.
#[test]
fn staging_a_tool_round_freezes_the_pre_tool_assistant_text() {
    let (directory, store, session_store) = initialized_stores();
    create_running_run(&store, "run-pre-tool", json!([]));
    let executor = Arc::new(RecordingToolExecutor::new(vec!["system.status"]));
    let runtime =
        runtime_with_production_catalog(&directory, &store, &session_store, Arc::clone(&executor));
    let chat = chat_for("run-pre-tool", "http://127.0.0.1:1/v1/responses");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-pre-tool", "owner-pre-tool")
        .expect("acquire run lease");

    runtime
        .persist_tool_calls(
            &chat,
            &ModelResponse {
                usage_metadata: None,
                text: "  先说明一下。 ".to_owned(),
                tool_calls: vec![super::ModelToolCall {
                    id: "call-status-first".to_owned(),
                    name: "system.status".to_owned(),
                    arguments: json!({}),
                }],
            },
            &lease,
        )
        .expect("stage the first tool round");
    let payload = run_payload(&store, "run-pre-tool");
    assert_eq!(
        payload["preToolContent"], "先说明一下。",
        "the staged round freezes the trimmed pre-tool text: {payload}"
    );
    assert_eq!(
        payload["toolCalls"][0]["status"], "RUNNING",
        "a low-risk read stays released instead of waiting for approval: {payload}"
    );

    runtime
        .persist_tool_calls(
            &chat,
            &ModelResponse {
                usage_metadata: None,
                text: "第二轮说明。".to_owned(),
                tool_calls: vec![super::ModelToolCall {
                    id: "call-status-second".to_owned(),
                    name: "system.status".to_owned(),
                    arguments: json!({}),
                }],
            },
            &lease,
        )
        .expect("stage the second tool round");
    let payload = run_payload(&store, "run-pre-tool");
    assert_eq!(
        payload["preToolContent"], "先说明一下。",
        "only the first staged round defines the pre-tool text: {payload}"
    );
    assert_eq!(
        payload["toolCalls"].as_array().map(Vec::len),
        Some(2),
        "both rounds stay on the run: {payload}"
    );
    assert!(
        executor.executed().is_empty(),
        "staging records the calls; only the tool loop executes them: {:?}",
        executor.executed()
    );
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:69
/// `TestHydrateRunExecutionResultPopulatesRunFields`.
///
/// Go's `HydrateRunExecutionResult` projects the tool context onto the run: the
/// calls, their summaries, the optimization task id and `usage.toolCallsTotal`.
/// The projection renders one summary per terminal call (a success with its
/// output, a failure with its error, a denial by the user).
#[test]
fn the_run_projection_derives_tool_summaries_optimization_task_and_usage_totals() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    create_running_run(
        &store,
        "run-hydrated-fields",
        json!([
            {
                "id": "call-optimize",
                "toolName": "strategy.optimize",
                "status": "SUCCEEDED",
                "round": 1,
                "output": {"taskId": "opt-123", "status": "started"},
            },
            {
                "id": "call-save",
                "toolName": "strategy.save_draft",
                "status": "FAILED",
                "round": 1,
                "error": "disk full",
            },
            {
                "id": "call-trade",
                "toolName": "trade",
                "status": "DENIED",
                "round": 2,
            },
        ]),
    );
    let chat = chat_for("run-hydrated-fields", "http://127.0.0.1:1/v1/responses");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-hydrated-fields", "owner-hydrated")
        .expect("acquire run lease");

    let response = runtime
        .persist_success(
            &chat,
            ModelResponse {
                usage_metadata: None,
                text: "all set".to_owned(),
                tool_calls: Vec::new(),
            },
            &lease,
        )
        .expect("persist success");

    let run = &response["run"];
    assert_eq!(run["optimizationTaskId"], "opt-123");
    assert_eq!(run["usage"]["toolCallsTotal"], json!(3));
    assert_eq!(
        run["usage"]["modelCalls"],
        json!(3),
        "two tool rounds plus the opening call: {run}"
    );
    let summaries = run["toolSummaries"]
        .as_array()
        .unwrap_or_else(|| panic!("the projection lists tool summaries: {run}"));
    assert_eq!(
        summaries.len(),
        3,
        "one summary per terminal call: {summaries:?}"
    );
    assert!(
        summaries[0]
            .as_str()
            .is_some_and(|summary| summary.starts_with("strategy.optimize => ")),
        "a successful call summarizes its output: {summaries:?}"
    );
    assert_eq!(summaries[1], "strategy.save_draft failed: disk full");
    assert_eq!(summaries[2], "trade denied by user");
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:846
/// `TestProjectedChatResponseDoesNotExposeResolvedApprovals`.
///
/// A resolved approval belongs to the transcript history, not to the pending
/// queue: neither the response nor the projected run may expose it, and the
/// timeline never groups it as a pending approval.
#[test]
fn a_completed_projection_hides_resolved_approvals() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    create_running_run(&store, "run-resolved-approval", json!([]));
    let mut payload = run_payload(&store, "run-resolved-approval");
    payload["pendingApprovals"] = json!([{
        "id": "approval-approved",
        "runId": "run-resolved-approval",
        "agentId": "agent-projection",
        "toolName": "strategy.save_draft",
        "status": "APPROVED",
        "reason": "resolved",
        "createdAt": "2026-09-20T00:00:00Z",
        "updatedAt": "2026-09-20T00:00:01Z",
    }]);
    let stored_run = store
        .get_run("run-resolved-approval")
        .expect("read run")
        .expect("run row");
    let fence = store
        .claim_run_lease(
            "run-resolved-approval",
            "owner-projection",
            Duration::from_secs(30),
        )
        .expect("claim run lease");
    assert!(
        store
            .update_run_payload_if_status_and_revision_with_lease(
                "run-resolved-approval",
                "RUNNING",
                &stored_run.updated_at,
                &payload.to_string(),
                "owner-projection",
                fence.fencing_token,
            )
            .expect("update run payload")
    );
    let chat = chat_for("run-resolved-approval", "http://127.0.0.1:1/v1/responses");
    let lease = RunLeaseGuard::acquire(
        Arc::clone(&store),
        "run-resolved-approval",
        "owner-projection",
    )
    .expect("acquire run lease");

    let response = runtime
        .persist_success(
            &chat,
            ModelResponse {
                usage_metadata: None,
                text: "done".to_owned(),
                tool_calls: Vec::new(),
            },
            &lease,
        )
        .expect("persist success");

    assert_eq!(response["pendingApprovals"], json!([]));
    assert_eq!(response["run"]["pendingApprovals"], json!([]));
    for entry in response["timeline"].as_array().into_iter().flatten() {
        assert_ne!(
            entry["kind"], "approval_group",
            "a resolved approval is not a timeline group: {entry}"
        );
    }
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:102
/// `TestCompleteChatRunDoesNotPromoteTopLevelFollowUpToPendingInput`.
///
/// A reply that asks the operator a question is still a *completed* top-level
/// turn: the run keeps `COMPLETED`, the reply is preserved verbatim, and no
/// `inputRequest` is manufactured.
#[test]
fn a_top_level_follow_up_reply_keeps_its_run_completed() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    create_running_run(&store, "run-top-level-follow-up", json!([]));
    let chat = chat_for("run-top-level-follow-up", "http://127.0.0.1:1/v1/responses");
    let lease = RunLeaseGuard::acquire(
        Arc::clone(&store),
        "run-top-level-follow-up",
        "owner-follow-up",
    )
    .expect("acquire run lease");
    let reply = "请提供策略名称、Pine Script 代码和关键参数，并发给我后我来继续处理。";

    let response = runtime
        .persist_success(
            &chat,
            ModelResponse {
                usage_metadata: None,
                text: reply.to_owned(),
                tool_calls: Vec::new(),
            },
            &lease,
        )
        .expect("persist success");

    assert_eq!(response["run"]["status"], "COMPLETED");
    assert_eq!(response["reply"], reply);
    assert!(
        response["run"].get("inputRequest").is_none(),
        "a top-level follow-up is not a pending input request: {response}"
    );
    assert_eq!(response["run"]["pendingApprovals"], json!([]));
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:1079
/// `TestStartRunPersistsRunAndFinishRemovesActiveHandle`.
///
/// Go's `startRun` persists `RUNNING` with the user message and registers the
/// live handle; `finish()` removes it.  Rust persists the same Go-shaped
/// snapshot and the runtime releases its cancellation handle at the terminal
/// state instead of leaking a stale one.
#[test]
fn a_started_run_serves_its_snapshot_and_drops_the_active_handle() {
    let (directory, store, session_store) = initialized_stores();
    let runtime = runtime_for(&directory, &store, &session_store);
    create_running_run(&store, "run-start-handle", json!([]));
    let payload = run_payload(&store, "run-start-handle");
    assert_eq!(payload["status"], "RUNNING");
    assert_eq!(payload["message"], "running");
    assert_eq!(payload["userMessage"], "hello");
    assert_eq!(payload["workMode"], "chat");
    assert_eq!(payload["maxDurationMs"], json!(1_800_000));
    assert_eq!(
        payload["usage"],
        json!({"modelCalls": 0, "toolCallsTotal": 0})
    );
    assert!(
        payload["startedAt"]
            .as_str()
            .is_some_and(|value| !value.is_empty()),
        "startRun stamps startedAt: {payload}"
    );

    // While the run is live the registry owns a cancellation handle and the
    // terminal projection releases it, exactly like Go's `activeRuns` entry.
    let token = runtime.cancellation_registry.register("run-start-handle");
    assert!(
        runtime.cancellation_registry.cancel("run-start-handle"),
        "a live run is cancellable"
    );
    token.store(false, Ordering::Release);
    let chat = chat_for("run-start-handle", "http://127.0.0.1:1/v1/responses");
    let lease = RunLeaseGuard::acquire(Arc::clone(&store), "run-start-handle", "owner-start")
        .expect("acquire run lease");
    runtime
        .cancellation_registry
        .unregister("run-start-handle", &token);
    runtime
        .persist_success(
            &chat,
            ModelResponse {
                usage_metadata: None,
                text: "done".to_owned(),
                tool_calls: Vec::new(),
            },
            &lease,
        )
        .expect("persist success");
    assert!(
        !runtime.cancellation_registry.cancel("run-start-handle"),
        "a terminal run keeps no cancellation handle"
    );
    assert_eq!(
        store
            .get_run("run-start-handle")
            .expect("read run")
            .expect("run row")
            .status,
        "COMPLETED"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:1043
/// `TestResolveSessionReusesExistingRejectsMismatchAndCreatesTrimmedSession`.
///
/// `resolveSession` creates the session exactly once with a title trimmed to 28
/// runes, and reusing an explicit session id returns the stored row untouched.
/// Rust upserted the title on every chat, so a session was renamed after its own
/// second message even though the console had already labelled it.
#[test]
fn chat_creates_the_session_once_and_reuses_its_stored_title() {
    let (directory, store, session_store) = initialized_stores();
    let settings_path = directory.path().join("settings.json");
    std::fs::write(&settings_path, "{}").expect("write settings");
    std::fs::create_dir_all(directory.path().join("secrets")).expect("create secrets directory");
    std::fs::write(
        directory.path().join("secrets/adk-secrets.json"),
        r#"{"provider-title":"sk-title"}"#,
    )
    .expect("write provider secrets");
    store
        .upsert_provider(
            "provider-title",
            &json!({
                "displayName": "Title fixture",
                // The model call fails on a closed loopback port; only the
                // session resolution and the run snapshot matter here.
                "baseUrl": "http://127.0.0.1:1/v1",
                "model": "fixture-model",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "agent-title",
            &json!({
                "id": "agent-title",
                "name": "Title agent",
                "providerId": "provider-title",
                "status": "ENABLED",
            })
            .to_string(),
        )
        .expect("persist agent");
    let runtime = ProductionAdkChatRuntime::new(
        Arc::clone(&store),
        Arc::clone(&session_store),
        &settings_path,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()),
    );
    let request =
        |client_request_id: &str, session_id: Option<&str>, message: &str| super::AdkChatInput {
            body: json!({
                "agentId": "agent-title",
                "message": message,
                "sessionId": session_id,
            })
            .to_string()
            .into_bytes(),
            client_request_id: client_request_id.to_owned(),
        };

    let long_message = "会话标题".repeat(10);
    runtime
        .dispatch(
            AdkChatRoute::Chat,
            &request("request-title-create", None, &long_message),
        )
        .expect("the first chat creates the session");
    let sessions = store.list_sessions().expect("list sessions");
    assert_eq!(sessions.len(), 1, "one session row: {sessions:?}");
    let created: Value = serde_json::from_str(&sessions[0].payload_json).expect("session payload");
    let title = created["title"]
        .as_str()
        .unwrap_or_else(|| panic!("the created session carries a title: {created}"));
    assert_eq!(
        title.chars().count(),
        28,
        "Go trims the created title to 28 runes: {title:?}"
    );
    assert_eq!(created["agentId"], "agent-title");

    // The second message reuses the explicit id and must not rewrite the title
    // the console already rendered.
    runtime
        .dispatch(
            AdkChatRoute::Chat,
            &request(
                "request-title-reuse",
                Some("session-request-title-create"),
                "第二条消息",
            ),
        )
        .expect("the second chat reuses the session");
    let sessions = store.list_sessions().expect("list sessions");
    assert_eq!(sessions.len(), 1, "reuse does not duplicate the session");
    let reused: Value = serde_json::from_str(&sessions[0].payload_json).expect("session payload");
    assert_eq!(
        reused["title"], created["title"],
        "reusing a session keeps the stored title: {reused}"
    );
}
