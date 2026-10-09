use super::*;

#[path = "product_adk_workflow_reasoning_owner_tests.rs"]
mod workflow_reasoning;

struct SnapshotProvider {
    endpoint: String,
    requests: Arc<Mutex<Vec<Value>>>,
    stop: Arc<AtomicBool>,
    worker: Option<thread::JoinHandle<()>>,
}

impl SnapshotProvider {
    fn new() -> Self {
        let _ = rustls::crypto::ring::default_provider().install_default();
        let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        listener.set_nonblocking(true).unwrap();
        let endpoint = format!("http://{}/v1", listener.local_addr().unwrap());
        let requests = Arc::new(Mutex::new(Vec::new()));
        let stop = Arc::new(AtomicBool::new(false));
        let captured = requests.clone();
        let stopped = stop.clone();
        let worker = thread::spawn(move || {
            while !stopped.load(Ordering::Acquire) {
                let mut socket = match listener.accept() {
                    Ok((socket, _)) => socket,
                    Err(error) if error.kind() == std::io::ErrorKind::WouldBlock => {
                        thread::sleep(Duration::from_millis(5));
                        continue;
                    }
                    Err(error) => panic!("accept snapshot provider: {error}"),
                };
                socket.set_nonblocking(false).unwrap();
                socket.set_read_timeout(Some(Duration::from_secs(3))).unwrap();
                socket.set_write_timeout(Some(Duration::from_secs(3))).unwrap();
                captured.lock().unwrap().push(read_http_json_body(&mut socket));
                let body = r#"{"output_text":"snapshot resumed"}"#;
                let response = format!("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}", body.len());
                std::io::Write::write_all(&mut socket, response.as_bytes()).unwrap();
            }
        });
        Self { endpoint, requests, stop, worker: Some(worker) }
    }
}

impl Drop for SnapshotProvider {
    fn drop(&mut self) {
        self.stop.store(true, Ordering::Release);
        let result = self.worker.take().unwrap().join();
        if !thread::panicking() { result.unwrap(); }
    }
}

fn snapshot_runtime(endpoint: &str, effort: &str, mappings: Value) -> (
    tempfile::TempDir, Arc<ProductionAdkChatRuntime>,
) {
    let (directory, store, sessions) = initialized_stores();
    let settings = directory.path().join("settings.json");
    fs::write(&settings, "{}").unwrap();
    store.upsert_provider("provider-snapshot", &json!({
        "baseUrl": endpoint, "model":"current-model", "apiKey":"fixture-key", "enabled":true,
        "reasoningConfig":{"requestField":"vendor.current", "mappings":mappings},
    }).to_string()).unwrap();
    store.upsert_agent("agent-snapshot", &json!({
        "id":"agent-snapshot", "providerId":"provider-snapshot", "status":"ENABLED",
        "reasoningEffort":effort,
    }).to_string()).unwrap();
    sessions.upsert_session("jftrade", "local", "session-snapshot", "{}").unwrap();
    let runtime = ProductionAdkChatRuntime::new(store, sessions, &settings,
        Arc::new(RunCancellationRegistry::default()),
        Arc::new(crate::product::product_production_ports::ProductionToolCatalog::empty_for_test()));
    // These tests drive the production continuation directly. Join the other
    // recovery owner before seeding any run, so startup timing cannot claim it.
    let scanner = runtime.recovery_supervisor.as_ref().unwrap();
    scanner.shutdown();
    let health = scanner.health_snapshot();
    assert_eq!(health.status.as_str(), "shutdown");
    assert!(!health.running);
    (directory, runtime)
}

// Parity: go:452dea11:internal/assistant/engine/reasoning_effort_lifecycle_test.go:23 TestReasoningEffortResumeUsesRunSnapshot
// Only the chat continuation assertions are closed here; workflow context remains separate.
#[test]
fn production_reasoning_resume_uses_saved_wire_mapping_after_provider_config_drift() {
    let provider = SnapshotProvider::new();
    let (_directory, runtime) = snapshot_runtime(&provider.endpoint, "low",
        json!([{"effort":"low", "value":"LOW_V2"}]));
    runtime.store.create_run(CreateAdkRunParams {
        id:"run-snapshot", session_id:"session-snapshot", agent_id:"agent-snapshot",
        status:"RUNNING", client_request_id:"snapshot-request", request_fingerprint:"snapshot-fingerprint",
        payload_json:&json!({"id":"run-snapshot", "sessionId":"session-snapshot",
            "agentId":"agent-snapshot", "providerId":"provider-snapshot", "model":"snapshot-model",
            "reasoningEffort":"max", "reasoningEffortField":"reasoning_effort", "reasoningEffortValue":"MAX_V1",
            "status":"RUNNING", "route":"chat", "resumeState":"input_resuming", "requestMessage":"continue",
        }).to_string(),
    }).unwrap();
    let resumed = runtime.resume_approval("run-snapshot");
    if resumed.is_err() { runtime.shutdown_with_error().unwrap(); }
    resumed.expect("saved max mapping must be used before current low-only validation");
    let deadline = Instant::now() + Duration::from_secs(5);
    let completed = loop {
        let run = runtime.store.get_run("run-snapshot").unwrap().unwrap();
        if run.status == "COMPLETED" { break run; }
        assert!(Instant::now() < deadline, "resume did not finish: {run:?}");
        thread::sleep(Duration::from_millis(5));
    };
    runtime.shutdown_with_error().unwrap();
    let payload: Value = serde_json::from_str(&completed.payload_json).unwrap();
    assert_eq!(payload["reasoningEffort"], "max");
    assert_eq!(payload["reasoningEffortField"], "reasoning_effort");
    assert_eq!(payload["reasoningEffortValue"], "MAX_V1");
    let audit = runtime.store.list_audit_events().unwrap();
    let events = runtime.session_store.list_events("session-snapshot").unwrap();
    runtime.resume_approval("run-snapshot").unwrap();
    assert_eq!(runtime.store.get_run("run-snapshot").unwrap().unwrap(), completed);
    assert_eq!(runtime.store.list_audit_events().unwrap(), audit);
    assert_eq!(runtime.session_store.list_events("session-snapshot").unwrap(), events);
    let requests = provider.requests.lock().unwrap();
    assert_eq!(requests.len(), 1, "terminal retry cannot re-execute provider");
    assert_eq!(requests[0]["model"], "snapshot-model");
    assert_eq!(requests[0]["reasoning_effort"], "MAX_V1");
    assert!(requests[0].get("vendor").is_none(), "current field must not replace saved field");
}

// Parity: go:452dea11:internal/assistant/engine/reasoning_effort_lifecycle_test.go:10 TestReasoningEffortOverridePriority
#[test]
fn production_reasoning_request_override_wins_over_agent_and_rejects_unknown_effort() {
    let provider = SnapshotProvider::new();
    let (_directory, runtime) = snapshot_runtime(&provider.endpoint, "medium",
        json!([{"effort":"medium","value":"MEDIUM"}, {"effort":"max","value":"MAX"}]));
    let mut request = serde_json::Map::from_iter([("agentId".to_owned(), json!("agent-snapshot"))]);
    let inherited = runtime.resolve_provider(&request).unwrap();
    assert_eq!(inherited.reasoning_effort.as_deref(), Some("medium"));
    assert_eq!(inherited.reasoning, Some(("vendor.current".to_owned(), "MEDIUM".to_owned())));
    request.insert("reasoningEffortOverride".to_owned(), json!("max"));
    let overridden = runtime.resolve_provider(&request).unwrap();
    assert_eq!(overridden.reasoning_effort.as_deref(), Some("max"));
    assert_eq!(overridden.reasoning, Some(("vendor.current".to_owned(), "MAX".to_owned())));
    request.insert("reasoningEffortOverride".to_owned(), json!("extreme"));
    assert!(matches!(runtime.resolve_provider(&request), Err(AdkChatPortError::Failed {status:400, ..})));
    runtime.shutdown_with_error().unwrap();
    assert!(runtime.store.list_runs().unwrap().is_empty());
    assert!(provider.requests.lock().unwrap().is_empty());
}

#[test]
fn production_reasoning_resume_without_complete_snapshot_keeps_current_provider_validation() {
    let provider = SnapshotProvider::new();
    let (_directory, runtime) = snapshot_runtime(&provider.endpoint, "low",
        json!([{"effort":"low", "value":"LOW_V2"}]));
    for (index, snapshot) in [json!({}), json!({"reasoningEffortField":"reasoning_effort"}),
        json!({"reasoningEffortValue":"MAX_V1"}),
        json!({"reasoningEffortField":" ","reasoningEffortValue":"MAX_V1"}),
        json!({"reasoningEffortField":"reasoning_effort","reasoningEffortValue":" "}),
    ].into_iter().enumerate() {
        let id = format!("run-incomplete-{index}");
        let mut payload = json!({"id":id,"agentId":"agent-snapshot", "providerId":"provider-snapshot",
            "model":"snapshot-model","reasoningEffort":"max","status":"RUNNING",
            "requestMessage":"continue","resumeState":"input_resuming"});
        payload.as_object_mut().unwrap().extend(snapshot.as_object().unwrap().clone());
        runtime.store.create_run(CreateAdkRunParams {
            id:&id, session_id:"session-snapshot", agent_id:"agent-snapshot",status:"RUNNING",
            client_request_id:&id, request_fingerprint:&id,payload_json:&payload.to_string(),
        }).unwrap();
        let row = runtime.store.get_run(&id).unwrap().unwrap();
        let audit = runtime.store.list_audit_events().unwrap();
        assert!(matches!(runtime.resume_approval(&id), Err(AdkChatPortError::Failed {status:400, ..})));
        assert_eq!(runtime.store.get_run(&id).unwrap().unwrap(), row);
        assert_eq!(runtime.store.list_audit_events().unwrap(), audit);
    }
    runtime.shutdown_with_error().unwrap();
    assert!(provider.requests.lock().unwrap().is_empty());
}

#[test]
fn production_reasoning_new_request_cannot_supply_private_snapshot_to_bypass_provider_mapping() {
    let provider = SnapshotProvider::new();
    let (_directory, runtime) = snapshot_runtime(&provider.endpoint, "low",
        json!([{"effort":"low", "value":"LOW_V2"}]));
    let input = AdkChatInput {
        client_request_id:"request-private-bypass".to_owned(),
        body:serde_json::to_vec(&json!({"agentId":"agent-snapshot","message":"hello",
            "reasoningEffortOverride":"max","reasoningEffortField":"reasoning_effort",
            "reasoningEffortValue":"MAX_V1"})).unwrap(),
    };
    assert!(matches!(runtime.prepare_chat(AdkChatRoute::Chat, &input),
        Err(AdkChatPortError::Failed {status:400, ..})));
    runtime.shutdown_with_error().unwrap();
    assert!(runtime.store.list_runs().unwrap().is_empty());
    assert!(provider.requests.lock().unwrap().is_empty());
}
