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

/// The production catalog, so a tool id resolves to its real approval policy.
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
        runtime_with_production_catalog(&directory, &store, &session_store, Arc::clone(&executor));

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
        runtime_with_production_catalog(&directory, &store, &session_store, Arc::clone(&executor));

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
