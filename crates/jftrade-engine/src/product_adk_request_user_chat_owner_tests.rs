use super::*;

struct InputProvider {
    endpoint: String,
    requests: Arc<Mutex<Vec<Value>>>,
    stop: Arc<AtomicBool>,
    worker: Option<thread::JoinHandle<()>>,
}

impl InputProvider {
    fn new(arguments: Value, terminal_status: u16) -> Self {
        let _ = rustls::crypto::ring::default_provider().install_default();
        let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        listener.set_nonblocking(true).unwrap();
        let endpoint = format!("http://{}/v1", listener.local_addr().unwrap());
        let requests = Arc::new(Mutex::new(Vec::new()));
        let stop = Arc::new(AtomicBool::new(false));
        let captured = requests.clone();
        let stopped = stop.clone();
        let first = json!({"output":[{"type":"function_call", "call_id":"call-input-owner",
            "name":"interaction.request_user", "arguments":arguments.to_string()}]});
        let worker = thread::spawn(move || {
            while !stopped.load(Ordering::Acquire) {
                let mut socket = match listener.accept() {
                    Ok((socket, _)) => socket,
                    Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                        thread::sleep(Duration::from_millis(5));
                        continue;
                    }
                    Err(error) => panic!("accept input provider: {error}"),
                };
                socket.set_nonblocking(false).unwrap();
                socket
                    .set_read_timeout(Some(Duration::from_secs(3)))
                    .unwrap();
                socket
                    .set_write_timeout(Some(Duration::from_secs(3)))
                    .unwrap();
                let request = read_http_json_body(&mut socket);
                let mut requests = captured.lock().unwrap();
                requests.push(request);
                let (status, body) = if requests.len() == 1 {
                    (200, first.clone())
                } else if terminal_status == 200 {
                    (200, json!({"output_text":"input resumed"}))
                } else {
                    (
                        terminal_status,
                        json!({"error":{"message":"stop after capturing resume content"}}),
                    )
                };
                drop(requests);
                let body = body.to_string();
                let response = format!(
                    "HTTP/1.1 {status} Fixture\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
                    body.len()
                );
                std::io::Write::write_all(&mut socket, response.as_bytes()).unwrap();
            }
        });
        Self {
            endpoint,
            requests,
            stop,
            worker: Some(worker),
        }
    }
}

impl Drop for InputProvider {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::Release);
        let result = self.worker.take().unwrap().join();
        if !thread::panicking() {
            result.unwrap();
        }
    }
}

fn input_runtime(
    endpoint: &str,
) -> (
    tempfile::TempDir,
    Arc<ProductionAdkChatRuntime>,
    ProductionAdkPort,
) {
    let (directory, store, sessions) = initialized_stores();
    let settings = directory.path().join("settings.json");
    fs::write(&settings, "{}").unwrap();
    store
        .upsert_provider(
            "provider-input-owner",
            &json!({"baseUrl":endpoint,
        "model":"input-model", "apiKey":"fixture-key", "enabled":true})
            .to_string(),
        )
        .unwrap();
    store
        .upsert_agent(
            "agent-input-owner",
            &json!({"id":"agent-input-owner",
        "providerId":"provider-input-owner", "status":"ENABLED", "permissionMode":"all",
        "workMode":"chat", "tools":["interaction.request_user"]})
            .to_string(),
        )
        .unwrap();
    let catalog = crate::product::product_production_ports::ProductionToolCatalog::from_tool_rows(
        vec![
            json!({"id":"interaction.request_user", "allowedModes":["all", "approval", "less_approval"]}),
        ],
    );
    let runtime = ProductionAdkChatRuntime::new(
        store,
        sessions,
        &settings,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(catalog),
    );
    let scanner = runtime.recovery_supervisor.as_ref().unwrap();
    scanner.shutdown();
    let health = scanner.health_snapshot();
    assert_eq!(health.status.as_str(), "shutdown");
    assert!(!health.running);
    let port = workflow_port(&directory, &runtime);
    (directory, runtime, port)
}

fn input_arguments(questions: Value) -> Value {
    json!({"title":"Choose input", "decisionKind":"material_tradeoff",
        "blockingReason":"The selected profile changes the requested result", "questions":questions})
}

fn start_input_chat(runtime: &ProductionAdkChatRuntime, request: &str, message: &str) -> Value {
    let output = runtime
        .dispatch(
            AdkChatRoute::Chat,
            &AdkChatInput {
                client_request_id: request.to_owned(),
                body: serde_json::to_vec(&json!({
            "clientRequestId":request, "agentId":"agent-input-owner", "message":message}))
                .unwrap(),
            },
        )
        .unwrap();
    let AdkChatPortOutput::Json(response) = output else {
        panic!("input chat must return JSON")
    };
    response
}

fn answer_input(port: &ProductionAdkPort, run_id: &str, input_id: &str, answers: Value) -> Value {
    port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: BTreeMap::from([("runId".to_owned(), run_id.to_owned())]),
        body: json!({"requestId":input_id,"answers":answers}),
        webhook_secret: None,
    })
    .unwrap()
}

fn wait_input_terminal(
    runtime: &ProductionAdkChatRuntime,
    run_id: &str,
    status: &str,
) -> StoredAdkRun {
    let deadline = Instant::now() + Duration::from_secs(5);
    loop {
        let run = runtime.store.get_run(run_id).unwrap().unwrap();
        if run.status == status {
            break;
        }
        assert!(
            Instant::now() < deadline,
            "input run did not reach {status}: {run:?}"
        );
        thread::sleep(Duration::from_millis(5));
    }
    assert!(
        runtime
            .continuation_supervisor
            .barrier
            .wait_timeout(Duration::from_secs(5))
    );
    // FAILED can precede the same owner's response/final-message attachment.
    // Capture the full row only after that owner has finished all its writes.
    let terminal = runtime.store.get_run(run_id).unwrap().unwrap();
    assert_eq!(terminal.status, status);
    terminal
}

// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:584 TestRequestUserToolPausesAndResumesChatRun
#[test]
fn production_request_user_chat_parks_two_questions_and_resumes_both_answers() {
    let provider = InputProvider::new(
        input_arguments(json!([
            {"question":"Choose a risk profile", "allowOther":true, "options":[
                {"label":"Conservative", "recommended":true}, {"label":"Active", "recommended":false}]},
            {"question":"Choose an output format", "allowOther":false, "options":[
                {"label":"Markdown", "recommended":true}, {"label":"JSON", "recommended":false}]},
        ])),
        200,
    );
    let (_directory, runtime, port) = input_runtime(&provider.endpoint);
    let pending = start_input_chat(
        &runtime,
        "33333333-3333-4333-8333-333333333311",
        "@input.required decide",
    );
    assert_eq!(pending["run"]["status"], "PENDING_INPUT");
    let questions = pending["inputRequest"]["questions"].as_array().unwrap();
    assert_eq!(questions.len(), 2);
    assert_eq!(questions[0]["options"][0]["id"], "q1-o1");
    assert_eq!(questions[0]["options"][0]["recommended"], true);
    let run_id = pending["run"]["id"].as_str().unwrap();
    let input_id = pending["inputRequest"]["id"].as_str().unwrap();
    let session_id = pending["session"]["id"].as_str().unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let awaiting = audit
        .iter()
        .find(|event| event.kind == "run.awaiting_input" && event.subject_id == run_id)
        .unwrap();
    let audit_payload: Value = serde_json::from_str(&awaiting.payload_json).unwrap();
    assert_eq!(
        audit_payload["metadata"]["decisionKind"],
        "material_tradeoff"
    );
    assert!(audit_payload["metadata"].get("blockingReason").is_none());
    assert_eq!(
        provider.requests.lock().unwrap().len(),
        1,
        "a pending input must not automatically continue"
    );
    let answers = json!([{"questionId":"q1", "otherText":"Balanced"}, {"questionId":"q2", "optionId":"q2-o1"}]);
    let submitted = answer_input(&port, run_id, input_id, answers.clone());
    assert_eq!(submitted["request"]["status"], "ANSWERED");
    let completed = wait_input_terminal(&runtime, run_id, "COMPLETED");
    let payload: Value = serde_json::from_str(&completed.payload_json).unwrap();
    assert_eq!(payload["inputRequest"]["id"], input_id);
    assert_eq!(payload["inputRequest"]["status"], "ANSWERED");
    assert_eq!(payload["inputRequest"]["answers"], answers);
    assert_eq!(
        payload["inputRequest"]["answers"].as_array().unwrap().len(),
        2
    );
    let audit = port.store.list_audit_events().unwrap();
    let events = port.session_store.list_events(session_id).unwrap();
    assert!(events.iter().any(|event| event.content == "input resumed"));
    answer_input(&port, run_id, input_id, answers);
    runtime.resume_approval(run_id).unwrap();
    runtime.shutdown_with_error().unwrap();
    assert_eq!(port.store.get_run(run_id).unwrap().unwrap(), completed);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(port.session_store.list_events(session_id).unwrap(), events);
    let requests = provider.requests.lock().unwrap();
    assert_eq!(
        requests.len(),
        2,
        "chat plus one continuation, with no execution on retry"
    );
    assert!(
        requests[0]["tools"]
            .as_array()
            .unwrap()
            .iter()
            .any(|tool| tool["name"] == "interaction.request_user")
    );
}

// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:773 TestInputResponsePayloadAnchorsResumedRunToOriginalRequest
// Parity: go:452dea11:internal/assistant/engine/input_request_test.go:802 TestResumedInputRunInjectsOriginalRequestAnchor
#[test]
fn production_input_resume_carries_one_original_request_anchor_and_persists_execution_failure() {
    let provider = InputProvider::new(
        input_arguments(json!([
            {"question":"Choose the execution mode", "options":[{"label":"Conservative"}, {"label":"Active"}]},
        ])),
        400,
    );
    let (_directory, runtime, port) = input_runtime(&provider.endpoint);
    let message = "请帮我看看目前交易记录，并执行操作计划。";
    let pending = start_input_chat(&runtime, "33333333-3333-4333-8333-333333333312", message);
    assert_eq!(pending["run"]["status"], "PENDING_INPUT");
    let run_id = pending["run"]["id"].as_str().unwrap();
    let input_id = pending["inputRequest"]["id"].as_str().unwrap();
    let submitted = answer_input(
        &port,
        run_id,
        input_id,
        json!([{"questionId":"q1","optionId":"q1-o1"}]),
    );
    let response = &submitted["run"]["inputResponse"];
    assert_eq!(response["requestId"], input_id);
    assert_eq!(response["originalRequest"], message);
    assert!(
        response["continuationInstruction"]
            .as_str()
            .is_some_and(|value| !value.trim().is_empty())
    );
    assert_eq!(response["answers"].as_array().unwrap().len(), 1);
    assert_eq!(response["answers"][0]["answer"], "Conservative");
    let failed = wait_input_terminal(&runtime, run_id, "FAILED");
    let audit = port.store.list_audit_events().unwrap();
    let session_id = pending["session"]["id"].as_str().unwrap();
    let events = port.session_store.list_events(session_id).unwrap();
    answer_input(
        &port,
        run_id,
        input_id,
        json!([{"questionId":"q1","optionId":"q1-o1"}]),
    );
    runtime.resume_approval(run_id).unwrap();
    runtime.shutdown_with_error().unwrap();
    assert_eq!(port.store.get_run(run_id).unwrap().unwrap(), failed);
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(port.session_store.list_events(session_id).unwrap(), events);
    let requests = provider.requests.lock().unwrap();
    assert_eq!(requests.len(), 2);
    let outputs: Vec<_> = requests[1]["input"]
        .as_array()
        .unwrap()
        .iter()
        .filter(|item| item["type"] == "function_call_output")
        .collect();
    assert_eq!(outputs.len(), 1);
    assert_eq!(
        outputs[0]["call_id"],
        pending["inputRequest"]["functionCallId"]
    );
    let call = requests[1]["input"]
        .as_array()
        .unwrap()
        .iter()
        .find(|item| item["type"] == "function_call" && item["call_id"] == outputs[0]["call_id"])
        .unwrap();
    assert_eq!(call["name"], "interaction.request_user");
    let output: Value = serde_json::from_str(outputs[0]["output"].as_str().unwrap()).unwrap();
    assert_eq!(output["requestId"], input_id);
    assert_eq!(output["originalRequest"], message);
    assert!(
        output["continuationInstruction"]
            .as_str()
            .is_some_and(|value| !value.trim().is_empty())
    );
}
