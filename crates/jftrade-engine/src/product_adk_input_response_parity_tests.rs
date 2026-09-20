//! Input-response behavior tests for Go
//! `internal/assistant/engine/input_request_test.go` rows that run through the
//! production ADK mutation port: canonical answer ordering, answer validation,
//! run cancellation of a pending input request and late-answer rejection.

use std::collections::BTreeMap;
use std::sync::Arc;

use serde_json::{Value, json};

use jftrade_store_sqlite::{AdkStore, CreateAdkRunParams};

use crate::product::product_adk_model_runtime::{
    AdkToolExecutor, ProductionAdkChatRuntime, RunCancellationRegistry,
};

use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute, AdkChatStreamPort,
};

use super::*;

/// A loopback Responses endpoint that answers one scripted body and returns the
/// decoded request bodies it received, so a resumed run can be checked against
/// the exact provider payload it produced.
fn spawn_scripted_model_provider(
    rounds: Vec<Value>,
) -> (String, std::thread::JoinHandle<Vec<Value>>) {
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
            }
            let headers_end = request
                .windows(4)
                .position(|w| w == b"\r\n\r\n")
                .map(|index| index + 4)
                .expect("model request headers");
            captured.push(
                serde_json::from_slice::<Value>(&request[headers_end..])
                    .expect("decode model request body"),
            );
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

fn input_question_arguments(question: &str) -> Value {
    json!({
        "decisionKind": "material_tradeoff",
        "blockingReason": "The execution mode changes the requested result.",
        "questions": [{
            "question": question,
            "allowOther": true,
            "options": [{"label": "Conservative"}, {"label": "Active"}],
        }],
    })
}

/// A tool executor that owns no production adapters.  The resumed input run
/// only needs the model turn, so every executable tool stays unavailable while
/// `interaction.request_user` keeps its dedicated runtime path.
#[derive(Debug)]
struct EmptyToolExecutor;

impl AdkToolExecutor for EmptyToolExecutor {
    fn supports(&self, _: &str) -> bool {
        false
    }

    fn execute(&self, name: &str, _: &Value) -> Result<Value, String> {
        Err(format!("tool adapter unavailable: {name}"))
    }
}

/// A fixture runtime that accepts the durable input continuation.  The
/// production resume worker is exercised elsewhere; these rows only need the
/// port to take the resume branch instead of reporting the continuation
/// unavailable.
#[derive(Debug)]
struct AcceptingContinuationRuntime;

impl AdkChatStreamPort for AcceptingContinuationRuntime {
    fn dispatch(
        &self,
        _: AdkChatRoute,
        _: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        Ok(AdkChatPortOutput::Json(json!({"synthetic": true})))
    }

    fn resume_approval(&self, _: &str) -> Result<(), AdkChatPortError> {
        Ok(())
    }

    fn runtime_ready(&self) -> bool {
        true
    }
}

fn mutate(
    port: &ProductionAdkPort,
    operation: AdkMutationOperation,
    identifiers: &[(&str, &str)],
    body: Value,
) -> Value {
    port.mutate(&AdkMutationInput {
        operation,
        identifiers: identifiers
            .iter()
            .map(|(key, value)| ((*key).to_owned(), (*value).to_owned()))
            .collect::<BTreeMap<_, _>>(),
        body,
        webhook_secret: None,
    })
    .expect("mutation succeeds")
}

fn mutate_error(
    port: &ProductionAdkPort,
    operation: AdkMutationOperation,
    identifiers: &[(&str, &str)],
    body: Value,
) -> AdkMutationPortError {
    port.mutate(&AdkMutationInput {
        operation,
        identifiers: identifiers
            .iter()
            .map(|(key, value)| ((*key).to_owned(), (*value).to_owned()))
            .collect::<BTreeMap<_, _>>(),
        body,
        webhook_secret: None,
    })
    .expect_err("mutation must be rejected")
}

fn pending_input_payload(run_id: &str, request_id: &str) -> Value {
    json!({
        "id": run_id,
        "sessionId": "session-input-parity",
        "agentId": "agent-input-parity",
        "status": "PENDING_INPUT",
        "resumeState": "waiting_input",
        "requestMessage": "请帮我看看目前交易记录，并执行操作计划。",
        "inputRequest": {
            "id": request_id,
            "runId": run_id,
            "agentId": "agent-input-parity",
            "functionCallId": "call-input-parity",
            "title": "参数确认",
            "status": "PENDING",
            "decisionKind": "material_tradeoff",
            "blockingReason": "The execution mode changes the requested result.",
            "questions": [
                {
                    "id": "q1",
                    "question": "Choose the execution mode",
                    "allowOther": true,
                    "options": [
                        {"id": "q1-o1", "label": "Conservative", "description": "", "recommended": true},
                        {"id": "q1-o2", "label": "Active", "description": "", "recommended": false},
                    ],
                },
                {
                    "id": "q2",
                    "question": "Output format?",
                    "allowOther": false,
                    "options": [
                        {"id": "q2-o1", "label": "Markdown", "description": "", "recommended": false},
                        {"id": "q2-o2", "label": "JSON", "description": "", "recommended": false},
                    ],
                },
            ],
            "answers": [],
            "createdAt": "2026-09-20T00:00:00Z",
            "updatedAt": "2026-09-20T00:00:00Z",
        },
        "inputRequests": [],
        "toolCalls": [{
            "id": "call-input-parity",
            "runId": run_id,
            "name": "interaction.request_user",
            "toolName": "interaction.request_user",
            "status": "PENDING_INPUT",
            "requiresUser": true,
        }],
        "pendingApprovals": [],
    })
}

fn seed_pending_input_run(
    store: &AdkStore,
    run_id: &str,
    request_id: &str,
) -> Value {
    let mut payload = pending_input_payload(run_id, request_id);
    payload["inputRequests"] = json!([payload["inputRequest"].clone()]);
    store
        .create_run(CreateAdkRunParams {
            id: run_id,
            session_id: "session-input-parity",
            agent_id: "agent-input-parity",
            status: "PENDING_INPUT",
            client_request_id: &format!("client-{run_id}"),
            request_fingerprint: &format!("fingerprint-{run_id}"),
            payload_json: &payload.to_string(),
        })
        .expect("seed pending input run");
    payload
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:16
/// `TestBuildInputRequestAndValidateAnswers` — the answer half: submitted
/// answers are re-ordered into question order, `otherText` is only accepted
/// when the question allows it, and every invalid submission is rejected.
#[test]
fn input_answers_are_canonicalized_by_question_order_and_invalid_answers_are_rejected() {
    let runtime: Arc<dyn AdkChatStreamPort> = Arc::new(AcceptingContinuationRuntime);
    let (port, store, _directory) = setup_test_adk_mutation_port(Some(runtime));
    seed_pending_input_run(&store, "run-input-canonical", "request-input-canonical");

    let response = mutate(
        &port,
        AdkMutationOperation::RespondToInput,
        &[("runId", "run-input-canonical")],
        json!({
            "requestId": "request-input-canonical",
            "answers": [
                {"questionId": "q2", "optionId": "q2-o2"},
                {"questionId": "q1", "otherText": "Balanced"},
            ],
        }),
    );
    assert_eq!(response["request"]["status"], "ANSWERED");
    assert_eq!(response["run"]["status"], "RUNNING");
    assert_eq!(response["run"]["resumeState"], "input_resuming");
    let answers = response["run"]["inputResponse"]["answers"]
        .as_array()
        .expect("inputResponse answers");
    assert_eq!(answers.len(), 2);
    assert_eq!(answers[0]["questionId"], "q1");
    assert_eq!(answers[0]["otherText"], "Balanced");
    assert_eq!(answers[1]["questionId"], "q2");
    assert_eq!(answers[1]["optionId"], "q2-o2");
    assert_eq!(answers[1]["answer"], "JSON");
    assert_eq!(
        response["run"]["inputResponse"]["originalRequest"],
        "请帮我看看目前交易记录，并执行操作计划。"
    );

    let cases: Vec<(Vec<Value>, &str)> = vec![
        (
            vec![json!({"questionId": "q1", "optionId": "q1-o1"})],
            "submitted 1 answers but request has 2 questions",
        ),
        (
            vec![
                json!({"questionId": "q1", "optionId": "q1-o1", "otherText": "both"}),
                json!({"questionId": "q2", "optionId": "q2-o1"}),
            ],
            "q1 must use exactly one answer type",
        ),
        (
            vec![
                json!({"questionId": "q1", "optionId": "q1-o1"}),
                json!({"questionId": "q2", "otherText": "not allowed"}),
            ],
            "q2 does not allow other text",
        ),
        (
            vec![
                json!({"questionId": "q1", "optionId": "missing"}),
                json!({"questionId": "q2", "optionId": "q2-o1"}),
            ],
            "invalid option for q1",
        ),
        (
            vec![
                json!({"questionId": "", "optionId": "q1-o1"}),
                json!({"questionId": "q2", "optionId": "q2-o1"}),
            ],
            "questionId is required",
        ),
        (
            vec![
                json!({"questionId": "unknown", "optionId": "q1-o1"}),
                json!({"questionId": "q2", "optionId": "q2-o1"}),
            ],
            "missing answer for q1",
        ),
    ];
    for (index, (answers, expected)) in cases.iter().enumerate() {
        let run_id = format!("run-input-invalid-{index}");
        let request_id = format!("request-input-invalid-{index}");
        seed_pending_input_run(&store, &run_id, &request_id);
        let error = mutate_error(
            &port,
            AdkMutationOperation::RespondToInput,
            &[("runId", &run_id)],
            json!({"requestId": request_id, "answers": answers}),
        );
        match error {
            AdkMutationPortError::Failed { status, message, .. } => {
                assert_eq!(status, 400, "invalid answer status for {expected}");
                assert!(
                    message.contains(expected),
                    "error {message:?} must report {expected:?}"
                );
            }
            other => panic!("expected 400 for {expected}, got {other:?}"),
        }
    }
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:16
/// `TestBuildInputRequestAndValidateAnswers` — the build half: Go's
/// `buildInputRequest` numbers questions `q1..` and options `q1-o1..`, copies
/// `recommended`/`allowOther` verbatim, and keeps the trimmed title,
/// decision kind and blocking reason on the staged request.
#[test]
fn input_request_questions_publish_the_reference_ids_labels_and_defaults() {
    let cluster = restarted_cluster(vec![scripted_tool_call(
        "call-input-build",
        "interaction.request_user",
        json!({
            "decisionKind": "scope_boundary",
            "blockingReason": "The requested action crosses the agreed scope.",
            "title": "  Scope confirmation  ",
            "questions": [
                {
                    "question": "Continue outside the scope?",
                    "options": [
                        {"label": "Stop", "description": "Keep the current scope", "recommended": true},
                        {"label": "Continue"},
                    ],
                },
                {
                    "question": "Which output format?",
                    "allowOther": true,
                    "options": [{"label": "Markdown"}, {"label": "JSON"}],
                },
            ],
        }),
    )]);
    let chat = chat_input_for(
        "33333333-3333-4333-8333-333333333304",
        "agent-input-parity",
        "@input.build confirm",
    );
    let output = cluster
        .runtime
        .dispatch(AdkChatRoute::Chat, &chat)
        .expect("chat turn parks on the built question");
    let AdkChatPortOutput::Json(response) = output else {
        panic!("chat must answer with the projected JSON envelope");
    };
    assert_eq!(response["run"]["status"], "PENDING_INPUT");
    assert_eq!(response["run"]["resumeState"], "waiting_input");
    let request = &response["inputRequest"];
    assert!(
        request["id"]
            .as_str()
            .is_some_and(|id| id.starts_with("input-")),
        "the request keeps the generated input- id: {request}"
    );
    assert_eq!(
        request["title"], "Scope confirmation",
        "Go trims the model-supplied title"
    );
    assert_eq!(request["decisionKind"], "scope_boundary");
    assert_eq!(request["status"], "PENDING");
    assert_eq!(request["answers"], json!([]));
    let questions = request["questions"].as_array().expect("questions");
    assert_eq!(questions.len(), 2);
    assert_eq!(questions[0]["id"], "q1");
    assert_eq!(questions[0]["question"], "Continue outside the scope?");
    assert_eq!(questions[0]["options"][0]["id"], "q1-o1");
    assert_eq!(questions[0]["options"][0]["label"], "Stop");
    assert_eq!(
        questions[0]["options"][0]["description"],
        "Keep the current scope"
    );
    assert_eq!(questions[0]["options"][0]["recommended"], true);
    assert_eq!(questions[0]["options"][1]["id"], "q1-o2");
    assert_eq!(
        questions[0]["options"][1]["recommended"], false,
        "Go copies the option default when the model omits recommended"
    );
    assert_eq!(
        questions[0]["allowOther"], false,
        "Go copies allowOther verbatim, so an omitted flag stays false"
    );
    assert_eq!(questions[1]["id"], "q2");
    assert_eq!(questions[1]["options"][1]["id"], "q2-o2");
    assert_eq!(questions[1]["allowOther"], true);
    // The staged tool call waits for the operator with the request attached.
    assert_eq!(response["run"]["toolCalls"][0]["status"], "PENDING_INPUT");
    assert_eq!(response["run"]["toolCalls"][0]["requiresUser"], true);
    assert_eq!(
        response["run"]["toolCalls"][0]["functionCallId"], "call-input-build",
        "the tool call keeps the model function-call id"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:773
/// `TestInputResponsePayloadAnchorsResumedRunToOriginalRequest`: the durable
/// input response carries the request id, the operator's original request, a
/// non-empty continuation instruction and the labels the model will read.
#[test]
fn input_response_payload_anchors_the_resumed_run_to_the_original_request() {
    let runtime: Arc<dyn AdkChatStreamPort> = Arc::new(AcceptingContinuationRuntime);
    let (port, store, _directory) = setup_test_adk_mutation_port(Some(runtime));
    seed_pending_input_run(
        &store,
        "run-input-payload-anchor",
        "request-input-payload-anchor",
    );

    let response = mutate(
        &port,
        AdkMutationOperation::RespondToInput,
        &[("runId", "run-input-payload-anchor")],
        json!({
            "requestId": "request-input-payload-anchor",
            "answers": [
                {"questionId": "q1", "optionId": "q1-o1"},
                {"questionId": "q2", "optionId": "q2-o1"},
            ],
        }),
    );
    let payload = &response["run"]["inputResponse"];
    assert_eq!(payload["requestId"], "request-input-payload-anchor");
    assert_eq!(
        payload["originalRequest"],
        "请帮我看看目前交易记录，并执行操作计划。"
    );
    assert!(
        payload["continuationInstruction"]
            .as_str()
            .is_some_and(|instruction| !instruction.trim().is_empty()),
        "the resumed model needs the continuation instruction: {payload}"
    );
    let answers = payload["answers"].as_array().expect("answers");
    assert_eq!(answers.len(), 2);
    assert_eq!(answers[0]["questionId"], "q1");
    assert_eq!(answers[0]["optionId"], "q1-o1");
    assert_eq!(answers[0]["question"], "Choose the execution mode");
    assert_eq!(answers[0]["answer"], "Conservative");
    assert_eq!(answers[1]["questionId"], "q2");
    assert_eq!(answers[1]["optionId"], "q2-o1");
    assert_eq!(answers[1]["answer"], "Markdown");
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:490
/// `TestCancelPendingInputRunCancelsRequestAndRejectsLateAnswer`.
#[test]
fn cancelling_a_pending_input_run_cancels_the_request_and_rejects_a_late_answer() {
    let (port, store, _directory) = setup_test_adk_mutation_port(None);
    seed_pending_input_run(&store, "run-input-cancel", "request-input-cancel");

    let cancelled = mutate(
        &port,
        AdkMutationOperation::CancelRun,
        &[("runId", "run-input-cancel")],
        Value::Null,
    );
    assert_eq!(cancelled["status"], "CANCELLED");
    assert_eq!(cancelled["inputRequest"]["status"], "CANCELLED");
    assert_eq!(cancelled["inputRequests"][0]["status"], "CANCELLED");
    assert_eq!(cancelled["toolCalls"][0]["status"], "CANCELLED");

    let stored = store
        .get_run("run-input-cancel")
        .expect("read cancelled run")
        .expect("run exists");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("run payload");
    assert_eq!(payload["status"], "CANCELLED");
    assert_eq!(payload["inputRequests"][0]["status"], "CANCELLED");

    let error = mutate_error(
        &port,
        AdkMutationOperation::RespondToInput,
        &[("runId", "run-input-cancel")],
        json!({
            "requestId": "request-input-cancel",
            "answers": [
                {"questionId": "q1", "optionId": "q1-o1"},
                {"questionId": "q2", "optionId": "q2-o1"},
            ],
        }),
    );
    match error {
        AdkMutationPortError::Failed { status, code, .. } => {
            assert_eq!(status, 409);
            assert_eq!(code, "ADK_INPUT_RESPONSE_CONFLICT");
        }
        other => panic!("expected 409 ADK_INPUT_RESPONSE_CONFLICT, got {other:?}"),
    }
    let stored = store
        .get_run("run-input-cancel")
        .expect("read run after late answer")
        .expect("run exists");
    assert_eq!(stored.status, "CANCELLED");
}

/// A cluster whose ADK port resumes through a restarted runtime: the runtime is
/// built after the fixture stores exist and owns the scripted provider, so the
/// durable input-continuation path is exercised rather than an in-memory
/// execution object.
struct RestartedCluster {
    port: Arc<ProductionAdkPort>,
    runtime: Arc<ProductionAdkChatRuntime>,
    store: Arc<AdkStore>,
    provider: std::thread::JoinHandle<Vec<Value>>,
    _directory: tempfile::TempDir,
}

fn restarted_cluster(rounds: Vec<Value>) -> RestartedCluster {
    let (endpoint, provider) = spawn_scripted_model_provider(rounds);
    restarted_cluster_with_provider(&endpoint, provider, "all", Arc::new(EmptyToolExecutor))
}

/// A restarted cluster whose provider endpoint, permission mode and tool
/// executor are supplied by the caller.  The input->approval row needs a
/// gated tool the resumed worker can really execute, and the continuation
/// failure row needs an endpoint that is already closed.
fn restarted_cluster_with_provider(
    endpoint: &str,
    provider: std::thread::JoinHandle<Vec<Value>>,
    permission_mode: &str,
    tool_executor: Arc<dyn AdkToolExecutor>,
) -> RestartedCluster {
    let (fixture_port, store, _directory) = setup_test_adk_mutation_port(None);
    store
        .upsert_provider(
            "provider-input-restart",
            &json!({
                "id": "provider-input-restart",
                "displayName": "Restart Provider",
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
            "agent-input-parity",
            &json!({
                "id": "agent-input-parity",
                "name": "Input Parity",
                "providerId": "provider-input-restart",
                "permissionMode": permission_mode,
                "status": "ENABLED",
                "tools": ["interaction.request_user", "strategy.optimize"],
            })
            .to_string(),
        )
        .expect("persist agent");
    let runtime = Arc::new(
        ProductionAdkChatRuntime::with_tool_executor_for_test(
            Arc::clone(&store),
            Arc::clone(&fixture_port.session_store),
            &fixture_port.settings_path,
            Arc::new(RunCancellationRegistry::default()),
            Arc::clone(&fixture_port.tool_catalog),
            tool_executor,
        ),
    );
    let port = Arc::new(ProductionAdkPort {
        store: Arc::clone(&store),
        session_store: Arc::clone(&fixture_port.session_store),
        artifact_store: Arc::clone(&fixture_port.artifact_store),
        tool_catalog: Arc::clone(&fixture_port.tool_catalog),
        settings_path: fixture_port.settings_path.clone(),
        chat_runtime: Some(Arc::clone(&runtime) as Arc<dyn AdkChatStreamPort>),
    });
    RestartedCluster {
        port,
        runtime,
        store,
        provider,
        _directory,
    }
}

/// A model endpoint that is already closed, so the resumed provider call fails
/// fast instead of waiting for a timeout.
fn closed_model_endpoint() -> String {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").expect("bind probe port");
    let address = listener.local_addr().expect("probe address");
    drop(listener);
    format!("http://{}:{}/v1", address.ip(), address.port())
}

/// A tool executor that really runs `strategy.optimize`, so the input ->
/// approval transition can assert the gated call executes exactly once.
#[derive(Debug)]
struct GatedToolExecutor {
    executions: Arc<std::sync::atomic::AtomicUsize>,
}

impl GatedToolExecutor {
    fn new() -> Self {
        Self {
            executions: Arc::new(std::sync::atomic::AtomicUsize::new(0)),
        }
    }

    fn executions(&self) -> usize {
        self.executions.load(std::sync::atomic::Ordering::Acquire)
    }
}

impl AdkToolExecutor for GatedToolExecutor {
    fn supports(&self, name: &str) -> bool {
        name == "strategy.optimize"
    }

    fn execute(&self, name: &str, _: &Value) -> Result<Value, String> {
        self.executions
            .fetch_add(1, std::sync::atomic::Ordering::AcqRel);
        Ok(json!({"status": "ready", "tool": name}))
    }
}

fn chat_input_for(client_request_id: &str, agent_id: &str, message: &str) -> AdkChatInput {
    AdkChatInput {
        body: serde_json::to_vec(&json!({"agentId": agent_id, "message": message}))
            .expect("encode chat body"),
        client_request_id: client_request_id.to_owned(),
    }
}

fn wait_for_run_status(store: &AdkStore, run_id: &str, expected: &str) {
    let deadline = std::time::Instant::now() + std::time::Duration::from_secs(5);
    loop {
        let run = store
            .get_run(run_id)
            .expect("read run")
            .expect("run exists");
        if run.status == expected {
            return;
        }
        assert!(
            std::time::Instant::now() < deadline,
            "run {run_id} never reached {expected}: {}",
            run.status
        );
        std::thread::sleep(std::time::Duration::from_millis(20));
    }
}

fn resume_pending_input_run(cluster: &RestartedCluster, run_id: &str, request_id: &str) -> Value {
    mutate(
        &cluster.port,
        AdkMutationOperation::RespondToInput,
        &[("runId", run_id)],
        json!({
            "requestId": request_id,
            "answers": [
                {"questionId": "q1", "optionId": "q1-o1"},
                {"questionId": "q2", "optionId": "q2-o1"},
            ],
        }),
    )
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:738
/// `TestRequestUserToolResumesAfterRuntimeRestart`.
#[test]
fn a_restarted_runtime_resumes_a_pending_input_run() {
    let cluster = restarted_cluster(vec![scripted_text("已完成原始请求")]);
    seed_pending_input_run(&cluster.store, "run-input-restart", "request-input-restart");

    let response = resume_pending_input_run(&cluster, "run-input-restart", "request-input-restart");
    assert_eq!(response["run"]["status"], "RUNNING");
    assert_eq!(
        cluster.provider.join().expect("scripted provider thread").len(),
        1,
        "the resumed run must call the provider exactly once"
    );
    wait_for_run_status(&cluster.store, "run-input-restart", "COMPLETED");
    // Go's `completeInputContinuation` finishes an answered input request with
    // `resumeState=input_resolved`, which is how the console tells that
    // continuation kind apart from an approval resume.
    let stored = cluster
        .store
        .get_run("run-input-restart")
        .expect("read run")
        .expect("run exists");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("run payload");
    assert_eq!(payload["resumeState"], "input_resolved");
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:802
/// `TestResumedInputRunInjectsOriginalRequestAnchor`.
#[test]
fn a_resumed_input_run_replays_the_original_request_anchor_to_the_provider() {
    let cluster = restarted_cluster(vec![scripted_text("已完成原始请求")]);
    seed_pending_input_run(
        &cluster.store,
        "run-input-anchor",
        "request-input-anchor",
    );

    resume_pending_input_run(&cluster, "run-input-anchor", "request-input-anchor");
    let requests = cluster.provider.join().expect("scripted provider thread");
    assert_eq!(requests.len(), 1, "the resumed run must call the provider");
    let function_response = requests[0]["input"]
        .as_array()
        .and_then(|input| {
            input.iter().find(|item| {
                item.get("type").and_then(Value::as_str) == Some("function_call_output")
                    && item.get("call_id").and_then(Value::as_str) == Some("call-input-parity")
            })
        })
        .expect("the resumed request must replay the input tool result");
    let output = function_response["output"].as_str().unwrap_or_default();
    assert!(
        output.contains("originalRequest")
            && output.contains("请帮我看看目前交易记录，并执行操作计划。"),
        "the resumed function response must anchor the original request: {output}"
    );
    assert!(
        output.contains("continuationInstruction"),
        "the resumed function response must carry the continuation instruction: {output}"
    );
    wait_for_run_status(&cluster.store, "run-input-anchor", "COMPLETED");
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:647
/// `TestRequestUserToolSupportsSequentialQuestionsInOneRun`.
///
/// One run parks twice: the first answer resumes the model, the model asks a
/// second blocking question, and the second answer finishes the run with both
/// requests kept in the durable history as `ANSWERED`.
#[test]
fn sequential_questions_in_one_run_keep_both_answered_requests() {
    let cluster = restarted_cluster(vec![
        scripted_tool_call(
            "call-input-first",
            "interaction.request_user",
            input_question_arguments("First decision?"),
        ),
        scripted_tool_call(
            "call-input-second",
            "interaction.request_user",
            input_question_arguments("Second decision?"),
        ),
        scripted_text("两个问题都已确认，继续执行。"),
    ]);
    let chat = chat_input_for(
        "33333333-3333-4333-8333-333333333301",
        "agent-input-parity",
        "@input.twice decide",
    );
    let output = cluster
        .runtime
        .dispatch(AdkChatRoute::Chat, &chat)
        .expect("first chat turn");
    let AdkChatPortOutput::Json(first) = output else {
        panic!("chat must answer with the projected JSON envelope");
    };
    assert_eq!(first["run"]["status"], "PENDING_INPUT");
    let run_id = first["run"]["id"].as_str().expect("first run id").to_owned();
    let first_request = first["inputRequest"]["id"]
        .as_str()
        .expect("first input request id")
        .to_owned();

    mutate(
        &cluster.port,
        AdkMutationOperation::RespondToInput,
        &[("runId", run_id.as_str())],
        json!({
            "requestId": first_request,
            "answers": [{"questionId": "q1", "optionId": "q1-o1"}],
        }),
    );

    let deadline = std::time::Instant::now() + std::time::Duration::from_secs(5);
    let second_request = loop {
        let payload: Value = serde_json::from_str(
            &cluster
                .store
                .get_run(&run_id)
                .expect("read run")
                .expect("run exists")
                .payload_json,
        )
        .expect("run payload");
        let candidate = payload["inputRequest"]["id"].as_str().unwrap_or_default().to_owned();
        if !candidate.is_empty() && candidate != first_request {
            break candidate;
        }
        assert!(
            std::time::Instant::now() < deadline,
            "the run never staged a second question: {payload}"
        );
        std::thread::sleep(std::time::Duration::from_millis(20));
    };

    mutate(
        &cluster.port,
        AdkMutationOperation::RespondToInput,
        &[("runId", run_id.as_str())],
        json!({
            "requestId": second_request,
            "answers": [{"questionId": "q1", "otherText": "custom"}],
        }),
    );
    wait_for_run_status(&cluster.store, &run_id, "COMPLETED");
    let payload: Value = serde_json::from_str(
        &cluster
            .store
            .get_run(&run_id)
            .expect("read run")
            .expect("run exists")
            .payload_json,
    )
    .expect("run payload");
    let requests = payload["inputRequests"]
        .as_array()
        .expect("input request history");
    assert_eq!(requests.len(), 2, "both questions stay in the history");
    assert!(
        requests
            .iter()
            .all(|request| request["status"] == "ANSWERED"),
        "every answered request keeps its answered status: {requests:?}"
    );
    assert_eq!(requests[0]["answers"][0]["optionId"], "q1-o1");
    assert_eq!(requests[1]["answers"][0]["otherText"], "custom");
    assert_eq!(
        cluster.provider.join().expect("scripted provider thread").len(),
        3,
        "the run must reach the provider for the initial turn and both resumes"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:305
/// `TestPendingInputRequestConflictEdges` — the simultaneous-question branch:
/// Go's `PendingInputRequests` refuses a run with two blocking questions in
/// flight ("simultaneous input requests are not supported for run <id>") and
/// `CompleteChatRun` projects that error as a terminal FAILED run.
///
/// Rust used to park on the first call and silently drop the second, which
/// stranded the model's second function call.  The run must fail instead of
/// publishing a question the operator can answer.
#[test]
fn simultaneous_input_request_calls_fail_the_run_instead_of_parking() {
    let cluster = restarted_cluster(vec![json!({
        "output": [
            {
                "type": "function_call",
                "call_id": "call-input-a",
                "name": "interaction.request_user",
                "arguments": input_question_arguments("First decision?").to_string(),
            },
            {
                "type": "function_call",
                "call_id": "call-input-b",
                "name": "interaction.request_user",
                "arguments": input_question_arguments("Second decision?").to_string(),
            },
        ],
    })]);
    let chat = chat_input_for(
        "33333333-3333-4333-8333-333333333302",
        "agent-input-parity",
        "@input.parallel decide",
    );
    let output = cluster
        .runtime
        .dispatch(AdkChatRoute::Chat, &chat)
        .expect("the conflict is projected as a terminal chat answer");
    let AdkChatPortOutput::Json(response) = output else {
        panic!("chat must answer with the projected JSON envelope");
    };
    assert_eq!(
        response["run"]["status"], "FAILED",
        "two blocking questions can never park the same run: {response}"
    );
    assert_eq!(response["run"]["errorCode"], "MODEL_CALL_FAILED");
    assert_eq!(response["run"]["degraded"], true);
    assert!(
        response["inputRequest"].is_null(),
        "a conflicted run must not publish an input request: {response}"
    );
    let message = response["run"]["message"].as_str().unwrap_or_default();
    assert!(
        message.contains("simultaneous input requests are not supported for run"),
        "the run keeps the reference conflict text: {response}"
    );
    assert!(
        response["reply"]
            .as_str()
            .is_some_and(|reply| reply.contains("simultaneous input requests")),
        "the failure becomes the turn reply: {response}"
    );
    let run_id = response["run"]["id"].as_str().expect("run id").to_owned();
    let stored = cluster
        .store
        .get_run(&run_id)
        .expect("read run")
        .expect("run exists");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("run payload");
    assert!(
        payload.get("inputRequest").is_none() || payload["inputRequest"].is_null(),
        "no question is staged durably: {payload}"
    );
    assert_eq!(
        payload["toolCalls"].as_array().map(Vec::len).unwrap_or_default(),
        0,
        "neither conflicting call is staged as an executable tool call: {payload}"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:413
/// `TestInputContinuationFailureIsPersisted`: Go's `failInputContinuation`
/// marks an unrecoverable answered-input continuation as `FAILED` with
/// `resumeState=input_resume_failed` and a classified non-empty error code.
#[test]
fn an_unrecoverable_input_continuation_fails_the_run_with_the_reference_resume_state() {
    let cluster = restarted_cluster_with_provider(
        &closed_model_endpoint(),
        std::thread::spawn(Vec::<Value>::new),
        "all",
        Arc::new(EmptyToolExecutor),
    );
    seed_pending_input_run(
        &cluster.store,
        "run-input-continuation-failure",
        "request-input-continuation-failure",
    );

    resume_pending_input_run(
        &cluster,
        "run-input-continuation-failure",
        "request-input-continuation-failure",
    );
    wait_for_run_status(&cluster.store, "run-input-continuation-failure", "FAILED");
    let stored = cluster
        .store
        .get_run("run-input-continuation-failure")
        .expect("read run")
        .expect("run exists");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("run payload");
    assert_eq!(
        payload["resumeState"], "input_resume_failed",
        "Go pins the unrecoverable continuation state: {payload}"
    );
    assert!(
        payload["errorCode"]
            .as_str()
            .is_some_and(|code| !code.is_empty()),
        "a terminal continuation failure keeps a classified error code: {payload}"
    );
    assert_eq!(payload["degraded"], true);
    assert!(
        payload["completedAt"].is_string(),
        "the failed continuation stamps completedAt: {payload}"
    );
    assert_eq!(
        payload["inputRequest"]["status"], "ANSWERED",
        "the answered request keeps its recorded answer: {payload}"
    );
}

/// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:694
/// `TestRequestUserToolCanTransitionToApproval`: answering a blocking question
/// resumes the model, the resumed turn asks for a gated tool, the run parks on
/// the operator with the input request still `ANSWERED`, and the approved call
/// executes exactly once before the run completes.
#[test]
fn an_answered_input_request_can_transition_into_an_approval_wait() {
    let executor = Arc::new(GatedToolExecutor::new());
    let (endpoint, provider) = spawn_scripted_model_provider(vec![
        scripted_tool_call(
            "call-input-before-approval",
            "interaction.request_user",
            input_question_arguments("Run the optimization?"),
        ),
        scripted_tool_call(
            "call-optimize-gated",
            "strategy.optimize",
            json!({"objective": "reduce drawdown"}),
        ),
        scripted_text("已按审批执行优化。"),
    ]);
    let cluster = restarted_cluster_with_provider(
        &endpoint,
        provider,
        "approval",
        Arc::clone(&executor) as Arc<dyn AdkToolExecutor>,
    );
    let chat = chat_input_for(
        "33333333-3333-4333-8333-333333333303",
        "agent-input-parity",
        "@input.approval decide",
    );
    let output = cluster
        .runtime
        .dispatch(AdkChatRoute::Chat, &chat)
        .expect("first chat turn");
    let AdkChatPortOutput::Json(first) = output else {
        panic!("chat must answer with the projected JSON envelope");
    };
    assert_eq!(first["run"]["status"], "PENDING_INPUT");
    let run_id = first["run"]["id"].as_str().expect("run id").to_owned();
    let request_id = first["inputRequest"]["id"]
        .as_str()
        .expect("input request id")
        .to_owned();

    mutate(
        &cluster.port,
        AdkMutationOperation::RespondToInput,
        &[("runId", run_id.as_str())],
        json!({
            "requestId": request_id,
            "answers": [{"questionId": "q1", "optionId": "q1-o1"}],
        }),
    );

    let deadline = std::time::Instant::now() + std::time::Duration::from_secs(5);
    let pending = loop {
        let stored = cluster
            .store
            .get_run(&run_id)
            .expect("read run")
            .expect("run exists");
        let payload: Value = serde_json::from_str(&stored.payload_json).expect("run payload");
        if stored.status == "PENDING"
            && payload["pendingApprovals"].as_array().is_some_and(|rows| rows.len() == 1)
        {
            break payload;
        }
        assert!(
            std::time::Instant::now() < deadline,
            "the resumed turn never parked on the gated call: {payload}"
        );
        std::thread::sleep(std::time::Duration::from_millis(20));
    };
    assert_eq!(pending["resumeState"], "waiting_approval");
    assert_eq!(
        pending["inputRequest"]["status"], "ANSWERED",
        "the waiting run keeps the answered question: {pending}"
    );
    assert_eq!(
        executor.executions(),
        0,
        "a gated call must not run before the operator approves"
    );
    let approval_id = pending["pendingApprovals"][0]["id"]
        .as_str()
        .expect("approval id")
        .to_owned();

    mutate(
        &cluster.port,
        AdkMutationOperation::Approve,
        &[("approvalId", approval_id.as_str())],
        json!({}),
    );
    wait_for_run_status(&cluster.store, &run_id, "COMPLETED");
    assert_eq!(
        executor.executions(),
        1,
        "the approved call executes exactly once"
    );
    let stored = cluster
        .store
        .get_run(&run_id)
        .expect("read run")
        .expect("run exists");
    let payload: Value = serde_json::from_str(&stored.payload_json).expect("run payload");
    assert_eq!(payload["inputRequest"]["status"], "ANSWERED");
    // The approved continuation is an approval resume, so Go finalizes it with
    // `adk_confirmation_resolved` even though it started from an answer.
    assert_eq!(payload["resumeState"], "adk_confirmation_resolved");
    assert_eq!(
        cluster.provider.join().expect("scripted provider thread").len(),
        3,
        "the run reaches the provider for the initial turn, the resumed turn and the approved call"
    );
}
