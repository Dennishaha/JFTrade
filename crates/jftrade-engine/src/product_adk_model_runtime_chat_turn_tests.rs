//! Regression coverage for Go's chat turn contracts
//! (`internal/assistant/engine/runner_chat_test.go`).
//!
//! Every test here drives the real runtime against a scripted loopback model
//! provider, so the provider request, the tool round and the terminal
//! projection are exercised end to end instead of being asserted against a
//! fixture port.

use std::fs::File;
use std::sync::Arc;

use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

use jftrade_store_sqlite::{AdkSessionStore, AdkStore, initialize_current};

use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortOutput, AdkChatRoute, AdkChatStreamPort,
};

use super::{AdkToolExecutor, ProductionAdkChatRuntime, RunCancellationRegistry};

#[path = "product_adk_provider_context_prefix_parity_tests.rs"]
mod context_prefix_parity;

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

/// Answers every supported capability with the injected workflow failure, so
/// the loop chat path can be pinned to surface it instead of reporting a
/// fabricated success.
#[derive(Debug)]
struct FailingToolExecutor {
    supported: Vec<&'static str>,
    failure: &'static str,
    executed: Arc<std::sync::Mutex<Vec<String>>>,
}

impl FailingToolExecutor {
    fn new(supported: Vec<&'static str>, failure: &'static str) -> Self {
        Self {
            supported,
            failure,
            executed: Arc::new(std::sync::Mutex::new(Vec::new())),
        }
    }

    fn executed(&self) -> Vec<String> {
        self.executed.lock().expect("failing executor lock").clone()
    }
}

impl AdkToolExecutor for FailingToolExecutor {
    fn supports(&self, name: &str) -> bool {
        self.supported.contains(&name)
    }

    fn execute(&self, name: &str, _arguments: &Value) -> Result<Value, String> {
        self.executed
            .lock()
            .expect("failing executor lock")
            .push(name.to_owned());
        Err(self.failure.to_owned())
    }
}

/// The production catalog, so a tool id resolves to its real approval policy.
fn runtime_with_production_catalog(
    directory: &tempfile::TempDir,
    store: &Arc<AdkStore>,
    session_store: &Arc<AdkSessionStore>,
    executor: Arc<dyn AdkToolExecutor>,
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

/// A loopback Responses endpoint that answers one scripted body per round and
/// returns the decoded request bodies in the order they were received.
fn spawn_scripted_model_provider(
    rounds: Vec<Value>,
) -> (String, std::thread::JoinHandle<Vec<Value>>) {
    const MAX_REQUEST_BYTES: usize = 8 * 1024 * 1024;
    let listener =
        std::net::TcpListener::bind("127.0.0.1:0").expect("bind scripted model provider");
    let address = listener.local_addr().expect("scripted provider address");
    let handle = std::thread::spawn(move || {
        let mut captured = Vec::new();
        for body in rounds {
            let (mut stream, _) = listener.accept().expect("accept model request");
            let mut request = Vec::new();
            let mut chunk = [0_u8; 4096];
            let mut expected = None;
            loop {
                let count =
                    std::io::Read::read(&mut stream, &mut chunk).expect("read model request");
                if count == 0 {
                    break;
                }
                request.extend_from_slice(&chunk[..count]);
                if expected.is_none()
                    && let Some(headers_end) = request.windows(4).position(|w| w == b"\r\n\r\n")
                {
                    let headers_end = headers_end + 4;
                    let headers =
                        String::from_utf8_lossy(&request[..headers_end]).to_ascii_lowercase();
                    let length = headers
                        .lines()
                        .find_map(|line| line.strip_prefix("content-length:"))
                        .and_then(|value| value.trim().parse::<usize>().ok())
                        .unwrap_or_default();
                    expected = Some(headers_end + length);
                }
                if expected.is_some_and(|expected| request.len() >= expected) {
                    break;
                }
                assert!(
                    request.len() <= MAX_REQUEST_BYTES,
                    "model request exceeded the fixture budget"
                );
            }
            let headers_end = request
                .windows(4)
                .position(|w| w == b"\r\n\r\n")
                .map(|index| index + 4)
                .expect("model request headers");
            let request_body = serde_json::from_slice::<Value>(&request[headers_end..])
                .expect("decode model request body");
            captured.push(request_body);
            let body = body.to_string();
            let response = format!(
                "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                body.len(),
                body
            );
            let _ = std::io::Write::write_all(&mut stream, response.as_bytes());
            let _ = std::io::Write::flush(&mut stream);
        }
        captured
    });
    (
        format!("http://{}:{}/v1", address.ip(), address.port()),
        handle,
    )
}

fn scripted_text(text: &str) -> Value {
    json!({
        "output": [{
            "type": "message",
            "content": [{"type": "output_text", "text": text}],
        }],
    })
}

fn scripted_tool_call(call_id: &str, name: &str, arguments: Value) -> Value {
    json!({
        "output": [{
            "type": "function_call",
            "call_id": call_id,
            "name": name,
            "arguments": arguments.to_string(),
        }],
    })
}

fn stored_payload(row: &jftrade_store_sqlite::StoredAdkEntity) -> Value {
    serde_json::from_str(&row.payload_json).expect("decode stored payload")
}

fn chat_input(client_request_id: &str, body: Value) -> AdkChatInput {
    AdkChatInput {
        body: body.to_string().into_bytes(),
        client_request_id: client_request_id.to_owned(),
    }
}

/// Parity: go:452dea11:internal/assistant/engine/context_cache_test.go:187
/// `TestProviderPayloadSortsToolsByNameIndependentOfAgentInputOrder`.
#[test]
fn provider_payload_sorts_tools_by_name_independent_of_agent_input_order() {
    let (directory, store, session_store) = initialized_stores();
    let (endpoint, provider) =
        spawn_scripted_model_provider(vec![scripted_text("first"), scripted_text("second")]);
    store
        .upsert_provider(
            "provider-cache-tools",
            &json!({
                "id": "provider-cache-tools",
                "displayName": "Cache Tools Provider",
                "baseUrl": endpoint,
                "model": "cache-tools-model",
                "apiKey": "sk-cache-tools",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    for (id, tools) in [
        ("agent-cache-tools-a", json!(["tools.search", "http.fetch"])),
        ("agent-cache-tools-b", json!(["http.fetch", "tools.search"])),
    ] {
        store
            .upsert_agent(
                id,
                &json!({
                    "id": id,
                    "name": id,
                    "providerId": "provider-cache-tools",
                    "permissionMode": "approval",
                    "status": "ENABLED",
                    "tools": tools,
                })
                .to_string(),
            )
            .expect("persist agent");
    }
    let runtime = runtime_with_production_catalog(
        &directory,
        &store,
        &session_store,
        Arc::new(RecordingToolExecutor::new(vec![
            "http.fetch",
            "tools.search",
        ])),
    );

    for (request_id, agent_id) in [
        (
            "11111111-1111-4111-8111-111111111187",
            "agent-cache-tools-a",
        ),
        (
            "11111111-1111-4111-8111-111111111188",
            "agent-cache-tools-b",
        ),
    ] {
        runtime
            .dispatch(
                AdkChatRoute::Chat,
                &chat_input(
                    request_id,
                    json!({"agentId": agent_id, "message": "list tools"}),
                ),
            )
            .expect("chat with ordered tool payload");
    }

    let requests = provider.join().expect("scripted provider thread");
    assert_eq!(requests.len(), 2);
    let tool_names = |request: &Value| {
        request["tools"]
            .as_array()
            .expect("tools in provider request")
            .iter()
            .map(|tool| tool["name"].as_str().expect("tool name").to_owned())
            .collect::<Vec<_>>()
    };
    assert_eq!(tool_names(&requests[0]), vec!["http.fetch", "tools.search"]);
    assert_eq!(tool_names(&requests[1]), tool_names(&requests[0]));
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:609
/// `TestChatRequestProviderOverrideRunsWithoutEditingAgent`.
///
/// A per-request `providerId`/`model` override only shapes the run: the
/// provider request uses the override model, the run snapshot freezes the
/// override provider and the stored agent row keeps its own provider.
#[test]
fn a_provider_override_runs_the_turn_without_editing_the_stored_agent() {
    let (directory, store, session_store) = initialized_stores();
    let (endpoint, provider) = spawn_scripted_model_provider(vec![scripted_text("override ok")]);
    store
        .upsert_provider(
            "provider-default",
            &json!({
                "id": "provider-default",
                "displayName": "Default Provider",
                "baseUrl": "http://127.0.0.1:1/v1",
                "model": "default-model",
                "apiKey": "sk-default",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist default provider");
    store
        .upsert_provider(
            "override-provider",
            &json!({
                "id": "override-provider",
                "displayName": "Override Provider",
                "baseUrl": endpoint,
                "model": "provider-default-model",
                "apiKey": "sk-override",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist override provider");
    store
        .upsert_agent(
            "agent-runtime-provider-override",
            &json!({
                "id": "agent-runtime-provider-override",
                "name": "Runtime Provider Override",
                "providerId": "provider-default",
                "model": "agent-model",
                "permissionMode": "approval",
                "status": "ENABLED",
            })
            .to_string(),
        )
        .expect("persist agent");
    let runtime = runtime_with_production_catalog(
        &directory,
        &store,
        &session_store,
        Arc::new(RecordingToolExecutor::new(Vec::new())),
    );

    let output = runtime
        .dispatch(
            AdkChatRoute::Chat,
            &chat_input(
                "11111111-1111-4111-8111-111111111101",
                json!({
                    "agentId": "agent-runtime-provider-override",
                    "message": "使用临时模型运行",
                    "providerId": "override-provider",
                    "model": "override-model",
                }),
            ),
        )
        .expect("chat with provider override");
    let requests = provider.join().expect("scripted provider thread");
    assert_eq!(
        requests[0]["model"], "override-model",
        "the provider request must carry the override model"
    );
    assert!(
        requests[0]["input"]
            .as_array()
            .is_some_and(|input| input.iter().any(|item| item
                .get("content")
                .and_then(Value::as_str)
                .is_some_and(|content| content.contains("使用临时模型运行")))),
        "the override turn still sends the user message: {}",
        requests[0]
    );

    let AdkChatPortOutput::Json(response) = output else {
        panic!("chat must answer with the projected JSON envelope");
    };
    assert_eq!(response["run"]["providerId"], "override-provider");
    assert_eq!(response["run"]["providerName"], "Override Provider");
    assert_eq!(response["run"]["model"], "override-model");

    let stored = store
        .get_agent("agent-runtime-provider-override")
        .expect("read stored agent")
        .expect("stored agent row");
    let agent = stored_payload(&stored);
    assert_eq!(
        agent["providerId"], "provider-default",
        "a per-request override must not rewrite the stored agent provider"
    );
    assert_eq!(agent["model"], "agent-model");
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:127
/// `TestChatToolOnlyADKRunSynthesizesFinalReply`.
///
/// A turn whose only model output is a function call still ends with an
/// assistant reply: the tool result is fed back to the provider and the second
/// answer names the executed tool.
#[test]
fn a_tool_only_turn_returns_the_second_round_reply_that_names_the_tool() {
    let (directory, store, session_store) = initialized_stores();
    let (endpoint, provider) = spawn_scripted_model_provider(vec![
        scripted_tool_call("call-status", "system.status", json!({})),
        scripted_text("已完成 ADK 分析：system.status"),
    ]);
    store
        .upsert_provider(
            "provider-tool-turn",
            &json!({
                "id": "provider-tool-turn",
                "displayName": "Tool Turn Provider",
                "baseUrl": endpoint,
                "model": "fixture-model",
                "apiKey": "sk-fixture",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "tool-final-agent",
            &json!({
                "id": "tool-final-agent",
                "name": "Tool Final",
                "providerId": "provider-tool-turn",
                "permissionMode": "less_approval",
                "status": "ENABLED",
                "tools": ["system.status"],
            })
            .to_string(),
        )
        .expect("persist agent");
    let executor = Arc::new(RecordingToolExecutor::new(vec!["system.status"]));
    let runtime =
        runtime_with_production_catalog(&directory, &store, &session_store, executor.clone());

    let output = runtime
        .dispatch(
            AdkChatRoute::Chat,
            &chat_input(
                "11111111-1111-4111-8111-111111111102",
                json!({
                    "agentId": "tool-final-agent",
                    "message": "@system.status 查询系统状态",
                }),
            ),
        )
        .expect("chat on a tool-only turn");
    let requests = provider.join().expect("scripted provider thread");
    assert_eq!(
        requests.len(),
        2,
        "the tool result must be fed back to the provider"
    );
    assert!(
        requests[1]
            .get("input")
            .and_then(Value::as_array)
            .is_some_and(|input| input.iter().any(|item| {
                item.get("type").and_then(Value::as_str) == Some("function_call_output")
                    && item.get("call_id").and_then(Value::as_str) == Some("call-status")
            })),
        "the second round must carry the durable tool result: {}",
        requests[1]
    );
    assert_eq!(executor.executed(), vec!["system.status".to_owned()]);

    let AdkChatPortOutput::Json(response) = output else {
        panic!("chat must answer with the projected JSON envelope");
    };
    assert_eq!(response["run"]["status"], "COMPLETED");
    assert_eq!(response["run"]["toolCalls"][0]["status"], "SUCCEEDED");
    let reply = response["reply"].as_str().unwrap_or_default();
    assert!(
        !reply.trim().is_empty(),
        "a tool-only turn must still answer with a reply"
    );
    assert!(
        reply.contains("system.status"),
        "the reply must carry the tool result summary: {reply}"
    );
}

/// Go registers `strategy.optimize` with `RequiresApprovalIn=[approval]`, so
/// `less_approval` releases it to the model and to the executor.
#[test]
fn a_released_optimizer_call_reaches_the_production_executor() {
    let (directory, store, session_store) = initialized_stores();
    let (endpoint, provider) = spawn_scripted_model_provider(vec![
        scripted_tool_call(
            "call-optimize",
            "strategy.optimize",
            json!({"definitionIds": ["def-a", "def-b"], "market": "US", "symbol": "US.AAPL"}),
        ),
        scripted_text("已完成 ADK 分析：strategy.optimize"),
    ]);
    store
        .upsert_provider(
            "provider-optimize",
            &json!({
                "id": "provider-optimize",
                "displayName": "Optimize Provider",
                "baseUrl": endpoint,
                "model": "fixture-model",
                "apiKey": "sk-fixture",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "agent-optimize",
            &json!({
                "id": "agent-optimize",
                "name": "Optimize Agent",
                "providerId": "provider-optimize",
                "permissionMode": "less_approval",
                "status": "ENABLED",
                "tools": ["strategy.optimize"],
            })
            .to_string(),
        )
        .expect("persist agent");
    let executor = Arc::new(RecordingToolExecutor::new(vec!["strategy.optimize"]));
    let runtime =
        runtime_with_production_catalog(&directory, &store, &session_store, executor.clone());

    let output = runtime
        .dispatch(
            AdkChatRoute::Chat,
            &chat_input(
                "11111111-1111-4111-8111-111111111104",
                json!({
                    "agentId": "agent-optimize",
                    "message": "@strategy.optimize 优化两个候选策略",
                }),
            ),
        )
        .expect("chat with the optimizer");
    let requests = provider.join().expect("scripted provider thread");
    assert!(
        requests[0]["tools"]
            .as_array()
            .is_some_and(|tools| tools.iter().any(|tool| tool["name"] == "strategy.optimize")),
        "the released optimizer must be exposed to the model: {}",
        requests[0]["tools"]
    );
    assert_eq!(executor.executed(), vec!["strategy.optimize".to_owned()]);
    let AdkChatPortOutput::Json(response) = output else {
        panic!("chat must answer with the projected JSON envelope");
    };
    assert_eq!(response["run"]["status"], "COMPLETED");
    assert_eq!(response["run"]["toolCalls"][0]["status"], "SUCCEEDED");
    assert_eq!(response["run"]["toolCalls"][0]["name"], "strategy.optimize");
}

/// The same call in `approval` mode parks the run instead of executing it.
#[test]
fn the_optimizer_is_gated_in_approval_mode() {
    let (directory, store, session_store) = initialized_stores();
    let (endpoint, provider) = spawn_scripted_model_provider(vec![scripted_tool_call(
        "call-optimize",
        "strategy.optimize",
        json!({"definitionIds": ["def-a"], "market": "US", "symbol": "US.AAPL"}),
    )]);
    store
        .upsert_provider(
            "provider-optimize-gated",
            &json!({
                "id": "provider-optimize-gated",
                "displayName": "Gated Optimize Provider",
                "baseUrl": endpoint,
                "model": "fixture-model",
                "apiKey": "sk-fixture",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "agent-optimize-gated",
            &json!({
                "id": "agent-optimize-gated",
                "name": "Gated Optimize Agent",
                "providerId": "provider-optimize-gated",
                "permissionMode": "approval",
                "status": "ENABLED",
                "tools": ["strategy.optimize"],
            })
            .to_string(),
        )
        .expect("persist agent");
    let executor = Arc::new(RecordingToolExecutor::new(vec!["strategy.optimize"]));
    let runtime =
        runtime_with_production_catalog(&directory, &store, &session_store, executor.clone());

    let output = runtime
        .dispatch(
            AdkChatRoute::Chat,
            &chat_input(
                "11111111-1111-4111-8111-111111111106",
                json!({
                    "agentId": "agent-optimize-gated",
                    "message": "@strategy.optimize 优化一个候选策略",
                }),
            ),
        )
        .expect("chat parked on the optimizer approval");
    provider.join().expect("scripted provider thread");
    assert!(
        executor.executed().is_empty(),
        "an approval-gated optimizer must not enqueue candidates before the operator answers"
    );
    let AdkChatPortOutput::Json(response) = output else {
        panic!("chat must answer with the projected JSON envelope");
    };
    assert_eq!(response["run"]["status"], "PENDING");
    assert_eq!(
        response["run"]["toolCalls"][0]["status"],
        "PENDING_APPROVAL"
    );
    assert_eq!(
        response["pendingApprovals"][0]["toolName"],
        "strategy.optimize"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/runner_chat_test.go:737
/// `TestRunnerChatProjectionPersistenceAndAssistantBoundaries`.
///
/// The parked-approval branch of that boundary contract: a gated call answers
/// with the approval prompt, the run is `PENDING` and every published approval
/// is still awaiting the operator.
#[test]
fn a_gated_tool_call_parks_the_run_with_only_pending_approvals() {
    let (directory, store, session_store) = initialized_stores();
    let (endpoint, provider) = spawn_scripted_model_provider(vec![scripted_tool_call(
        "call-fetch",
        "http.fetch",
        json!({"url": "https://example.invalid"}),
    )]);
    store
        .upsert_provider(
            "provider-approval",
            &json!({
                "id": "provider-approval",
                "displayName": "Approval Provider",
                "baseUrl": endpoint,
                "model": "fixture-model",
                "apiKey": "sk-fixture",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "agent-approval-gate",
            &json!({
                "id": "agent-approval-gate",
                "name": "Approval Gate",
                "providerId": "provider-approval",
                "permissionMode": "approval",
                "status": "ENABLED",
                "tools": ["http.fetch"],
            })
            .to_string(),
        )
        .expect("persist agent");
    let executor = Arc::new(RecordingToolExecutor::new(vec!["http.fetch"]));
    let runtime =
        runtime_with_production_catalog(&directory, &store, &session_store, executor.clone());

    let output = runtime
        .dispatch(
            AdkChatRoute::Chat,
            &chat_input(
                "11111111-1111-4111-8111-111111111103",
                json!({
                    "agentId": "agent-approval-gate",
                    "message": "@http.fetch https://example.invalid",
                }),
            ),
        )
        .expect("chat parked on approval");
    provider.join().expect("scripted provider thread");
    assert!(
        executor.executed().is_empty(),
        "a gated call must not reach the tool executor before the operator answers"
    );

    let AdkChatPortOutput::Json(response) = output else {
        panic!("chat must answer with the projected JSON envelope");
    };
    assert_eq!(response["run"]["status"], "PENDING");
    let approvals = response["pendingApprovals"]
        .as_array()
        .expect("pending approvals array");
    assert_eq!(approvals.len(), 1, "the gated call parks exactly once");
    assert_eq!(approvals[0]["toolName"], "http.fetch");
    assert_eq!(approvals[0]["status"], "PENDING");
    let run_approvals = response["run"]["pendingApprovals"]
        .as_array()
        .expect("run pending approvals array");
    assert!(
        run_approvals
            .iter()
            .all(|approval| approval["status"] == "PENDING"),
        "a parked run only publishes approvals that still await the operator: {run_approvals:?}"
    );
    assert_eq!(
        response["run"]["toolCalls"][0]["status"],
        "PENDING_APPROVAL"
    );
    assert!(
        response["reply"]
            .as_str()
            .is_some_and(|reply| !reply.trim().is_empty()),
        "the parked run answers with the approval prompt"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:72
/// `TestInputRequestToolRunReturnsCorrectableFeedbackForInvalidArgs`.
///
/// A malformed `interaction.request_user` call is a model-correctable slip:
/// Go returns an `invalid_arguments` result instead of parking the run, so the
/// model retries inside the same run.  The provider request for round two must
/// carry that feedback and the run must reach its normal terminal state.
#[test]
fn an_invalid_request_user_call_returns_correctable_feedback_before_parking() {
    let (directory, store, session_store) = initialized_stores();
    let (endpoint, provider) = spawn_scripted_model_provider(vec![
        scripted_tool_call(
            "call-input-invalid",
            "interaction.request_user",
            json!({
                "decisionKind": "material_tradeoff",
                "blockingReason": "The selected option changes the result.",
                "questions": [{
                    "question": "Too many choices?",
                    "options": [
                        {"label": "A"},
                        {"label": "B"},
                        {"label": "C"},
                        {"label": "D"},
                    ],
                }],
            }),
        ),
        scripted_text("参数已修正，继续执行。"),
    ]);
    store
        .upsert_provider(
            "provider-input-feedback",
            &json!({
                "id": "provider-input-feedback",
                "displayName": "Input Feedback Provider",
                "baseUrl": endpoint,
                "model": "fixture-model",
                "apiKey": "sk-fixture",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "agent-input-feedback",
            &json!({
                "id": "agent-input-feedback",
                "name": "Input Feedback",
                "providerId": "provider-input-feedback",
                "permissionMode": "all",
                "status": "ENABLED",
                "tools": ["interaction.request_user"],
            })
            .to_string(),
        )
        .expect("persist agent");
    let executor = Arc::new(RecordingToolExecutor::new(Vec::new()));
    let runtime =
        runtime_with_production_catalog(&directory, &store, &session_store, executor.clone());

    let output = runtime
        .dispatch(
            AdkChatRoute::Chat,
            &chat_input(
                "11111111-1111-4111-8111-111111111103",
                json!({
                    "agentId": "agent-input-feedback",
                    "message": "@input.required 先确认参数",
                }),
            ),
        )
        .expect("chat with malformed interaction arguments");
    let AdkChatPortOutput::Json(response) = output else {
        panic!("chat must answer with the projected JSON envelope");
    };
    assert_eq!(
        response["run"]["status"], "COMPLETED",
        "the corrected turn must finish instead of parking: {response}"
    );
    assert!(
        response["inputRequest"].is_null(),
        "an invalid call must not publish an input request: {response}"
    );
    assert_eq!(response["run"]["toolCalls"][0]["status"], "SUCCEEDED");
    assert_eq!(
        response["run"]["toolCalls"][0]["output"]["status"],
        "invalid_arguments"
    );

    let requests = provider.join().expect("scripted provider thread");
    assert_eq!(
        requests.len(),
        2,
        "correctable feedback must be fed back to the provider"
    );
    let feedback = requests[1]
        .get("input")
        .and_then(Value::as_array)
        .and_then(|input| {
            input.iter().find(|item| {
                item.get("type").and_then(Value::as_str) == Some("function_call_output")
                    && item.get("call_id").and_then(Value::as_str) == Some("call-input-invalid")
            })
        })
        .expect("the second round carries the invalid-arguments feedback");
    let feedback = feedback["output"].as_str().unwrap_or_default();
    assert!(
        feedback.contains("invalid_arguments") && feedback.contains("requires two to 3 options"),
        "the feedback must name the correctable slip: {feedback}"
    );
    assert!(
        executor.executed().is_empty(),
        "a malformed interaction call never reaches the tool executor"
    );
}

/// Go's `TestRuntimeUsesInjectedWorkflowExecutionForLoopChat` installs a
/// workflow executor on the runtime and requires its failure to surface on the
/// loop-mode chat path.  Rust has no `SetWorkflowExecutor` seam - the
/// composition root wires the workflow ports at startup - so the pin is the
/// workflow-family tool boundary on the same loop path: the owner's failure is
/// persisted on the call and the run never claims the workflow executed.
#[test]
fn a_failed_workflow_execution_surfaces_on_the_loop_chat_path() {
    const INJECTED_FAILURE: &str = "injected workflow executor ran";

    let (directory, store, session_store) = initialized_stores();
    let (endpoint, provider) = spawn_scripted_model_provider(vec![
        scripted_tool_call(
            "call-workflow-wait",
            "workflow.wait",
            json!({"workflowId": "workflow-injected"}),
        ),
        scripted_text("已完成 ADK 分析：workflow.wait"),
    ]);
    store
        .upsert_provider(
            "provider-workflow",
            &json!({
                "id": "provider-workflow",
                "displayName": "Workflow Provider",
                "baseUrl": endpoint,
                "model": "fixture-model",
                "apiKey": "sk-fixture",
                "enabled": true,
            })
            .to_string(),
        )
        .expect("persist provider");
    store
        .upsert_agent(
            "agent-workflow",
            &json!({
                "id": "agent-workflow",
                "name": "Workflow Agent",
                "providerId": "provider-workflow",
                "status": "ENABLED",
                "tools": ["workflow.wait"],
            })
            .to_string(),
        )
        .expect("persist agent");
    let executor = Arc::new(FailingToolExecutor::new(
        vec!["workflow.wait"],
        INJECTED_FAILURE,
    ));
    let runtime =
        runtime_with_production_catalog(&directory, &store, &session_store, executor.clone());

    let output = runtime
        .dispatch(
            AdkChatRoute::Chat,
            &chat_input(
                "11111111-1111-4111-8111-111111111108",
                json!({
                    "agentId": "agent-workflow",
                    "message": "@workflow.wait 等待工作流",
                    "workModeOverride": "loop",
                }),
            ),
        )
        .expect("the loop chat must answer the run envelope");
    let _ = provider.join().expect("scripted provider thread");

    assert_eq!(
        executor.executed(),
        vec!["workflow.wait".to_owned()],
        "the loop chat must reach the workflow capability"
    );
    let AdkChatPortOutput::Json(response) = output else {
        panic!("chat must answer with the projected JSON envelope");
    };
    let call = &response["run"]["toolCalls"][0];
    assert_eq!(call["name"], "workflow.wait", "{response}");
    assert_eq!(
        call["status"], "FAILED",
        "the workflow failure must be visible on the call: {response}"
    );
    assert_eq!(
        call["error"], INJECTED_FAILURE,
        "the owner failure text must survive: {response}"
    );
    assert_eq!(
        call["errorCode"], "TOOL_EXECUTION_FAILED",
        "the failure classification must survive: {response}"
    );
}
