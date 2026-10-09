use super::*;

// Parity: go:452dea11:internal/assistant/engine/reasoning_effort_lifecycle_test.go:23 TestReasoningEffortResumeUsesRunSnapshot
#[test]
fn production_workflow_input_resume_uses_saved_reasoning_mapping_after_provider_drift() {
    let provider = SnapshotProvider::new();
    let (directory, runtime) = snapshot_runtime(
        &provider.endpoint,
        "max",
        json!([{"effort":"max", "value":"MAX_V1"}]),
    );
    runtime.store.upsert_provider("provider-snapshot", &json!({
        "baseUrl":provider.endpoint, "model":"current-model", "apiKey":"fixture-key", "enabled":true,
        "reasoningConfig":{"requestField":"reasoning_effort", "mappings":[{"effort":"max", "value":"MAX_V1"}]},
    }).to_string()).unwrap();
    let port = workflow_port(&directory, &runtime);
    let uuid = "22222222-2222-4222-8222-222222222222";
    let workflow_id = "workflow-pending-snapshot";
    let request_id = format!("workflow-{workflow_id}-research-{uuid}");
    let body = json!({"clientRequestId":request_id, "agentId":"agent-snapshot",
        "providerId":"", "model":"snapshot-model", "message":"Research snapshot", "objective":""});
    let PreparedChat::New(chat, lease, slot) = runtime
        .prepare_chat(
            AdkChatRoute::Chat,
            &AdkChatInput {
                client_request_id: request_id.clone(),
                body: serde_json::to_vec(&body).unwrap(),
            },
        )
        .unwrap()
    else {
        panic!("workflow fixture must create one new run")
    };
    let staging = runtime.persist_tool_calls(&chat, &ModelResponse {
        usage_metadata: None, text: String::new(), tool_calls: vec![ModelToolCall {
            id: "call-workflow-input".to_owned(), name: "interaction.request_user".to_owned(),
            arguments: json!({"title":"Choose workflow input", "decisionKind":"material_tradeoff",
                "blockingReason":"A choice is required before continuing",
                "questions":[{"question":"Choose a profile", "allowOther":false,
                    "options":[{"label":"Conservative"}, {"label":"Active"}]}]}),
        }],
    }, &lease).unwrap();
    let ToolCallStaging::Pending(AdkChatPortOutput::Json(parked)) = staging else {
        panic!("workflow child must park for input")
    };
    assert_eq!(parked["run"]["status"], "PENDING_INPUT");
    let input_id = parked["inputRequest"]["id"].as_str().unwrap().to_owned();
    drop(lease);
    drop(slot);
    let workflow = json!({"agentId":"agent-snapshot", "model":"snapshot-model"});
    let graph = json!({"nodes":[{"id":"start", "type":"start"},
        {"id":"research", "type":"agent", "data":{"message":"Research snapshot"}}],
        "edges":[{"source":"start", "target":"research"}]});
    let log_id = "workflow-pending-snapshot-log";
    port.store.create_workflow_trigger_log(log_id, workflow_id, "", "manual", "PENDING_INPUT", &chat.run_id,
        &json!({"inputs":{}, "runId":chat.run_id, "sessionId":chat.session_id, "result":parked,
            "canvasExecution":{"workflow":workflow, "graph":graph, "invocationUuid":uuid, "trigger":null},
            "nodeRuns":[{"nodeId":"start", "nodeType":"start", "status":"SUCCEEDED"},
                {"nodeId":"research", "nodeType":"agent", "status":"PENDING_APPROVAL", "inputs":body,
                    "outputs":{"runId":chat.run_id, "sessionId":chat.session_id}}]}).to_string()).unwrap();
    port.store.upsert_provider("provider-snapshot", &json!({
        "baseUrl":provider.endpoint, "model":"current-model", "apiKey":"fixture-key", "enabled":true,
        "reasoningConfig":{"requestField":"vendor.current", "mappings":[{"effort":"low", "value":"LOW_V2"}]},
    }).to_string()).unwrap();
    port.store
        .upsert_agent(
            "agent-snapshot",
            &json!({"id":"agent-snapshot", "providerId":"provider-snapshot",
        "status":"ENABLED", "reasoningEffort":"low"})
            .to_string(),
        )
        .unwrap();
    // Enter the Canvas owner while its child is still blocked. Replaying the
    // saved request must preserve that child before the input owner continues.
    let pending = port.store.get_run(&chat.run_id).unwrap().unwrap();
    let pending_payload: Value = serde_json::from_str(&pending.payload_json).unwrap();
    assert_eq!(pending_payload["reasoningEffort"], "max");
    assert_eq!(pending_payload["reasoningEffortField"], "reasoning_effort");
    assert_eq!(pending_payload["reasoningEffortValue"], "MAX_V1");
    let pending_audit = port.store.list_audit_events().unwrap();
    let pending_events = port.session_store.list_events(&chat.session_id).unwrap();
    port.resume_workflow(log_id).unwrap();
    let pending_log = port
        .store
        .get_workflow_trigger_log(log_id)
        .unwrap()
        .unwrap();
    assert_eq!(pending_log.status, "PENDING_APPROVAL");
    let pending_projection: Value = serde_json::from_str(&pending_log.payload_json).unwrap();
    assert_eq!(pending_projection["result"]["run"]["id"], chat.run_id);
    assert_eq!(
        pending_projection["result"]["run"]["status"],
        "PENDING_INPUT"
    );
    assert_eq!(
        pending_projection["result"]["run"]["reasoningEffort"],
        "max"
    );
    // Private wire mapping stays in the durable owner, never the public result.
    assert!(
        pending_projection["result"]["run"]
            .get("reasoningEffortField")
            .is_none()
    );
    assert!(
        pending_projection["result"]["run"]
            .get("reasoningEffortValue")
            .is_none()
    );
    assert_eq!(port.store.get_run(&chat.run_id).unwrap().unwrap(), pending);
    assert_eq!(port.store.list_audit_events().unwrap(), pending_audit);
    assert_eq!(
        port.session_store.list_events(&chat.session_id).unwrap(),
        pending_events
    );
    assert!(provider.requests.lock().unwrap().is_empty());
    let submitted = port.mutate(&AdkMutationInput {
        operation: AdkMutationOperation::RespondToInput,
        identifiers: BTreeMap::from([("runId".to_owned(), chat.run_id.clone())]),
        body: json!({"requestId":input_id, "answers":[{"questionId":"q1", "optionId":"q1-o1"}]}),
        webhook_secret: None,
    }).unwrap();
    assert_eq!(submitted["request"]["status"], "ANSWERED");
    let deadline = Instant::now() + Duration::from_secs(5);
    let completed = loop {
        let run = port.store.get_run(&chat.run_id).unwrap().unwrap();
        if run.status == "COMPLETED" {
            break run;
        }
        assert!(
            Instant::now() < deadline,
            "workflow child did not complete: {run:?}"
        );
        thread::sleep(Duration::from_millis(5));
    };
    let payload: Value = serde_json::from_str(&completed.payload_json).unwrap();
    assert_eq!(payload["reasoningEffort"], "max");
    assert_eq!(payload["reasoningEffortField"], "reasoning_effort");
    assert_eq!(payload["reasoningEffortValue"], "MAX_V1");
    // COMPLETED is stored before the continuation's final audit. Wait for its
    // existing completion barrier before capturing immutable terminal state.
    assert!(
        runtime
            .continuation_supervisor
            .barrier
            .wait_timeout(Duration::from_secs(5))
    );
    let audit = port.store.list_audit_events().unwrap();
    let events = port.session_store.list_events(&chat.session_id).unwrap();
    port.resume_workflow(log_id).unwrap();
    let log = port
        .store
        .get_workflow_trigger_log(log_id)
        .unwrap()
        .unwrap();
    assert_eq!(log.status, "SUCCEEDED");
    let projection: Value = serde_json::from_str(&log.payload_json).unwrap();
    assert_eq!(projection["result"]["run"]["id"], chat.run_id);
    assert_eq!(projection["result"]["reply"], "snapshot resumed");
    port.resume_workflow(log_id).unwrap();
    runtime.resume_approval(&chat.run_id).unwrap();
    runtime.shutdown_with_error().unwrap();
    assert_eq!(
        port.store.get_run(&chat.run_id).unwrap().unwrap(),
        completed
    );
    assert_eq!(
        port.store
            .get_workflow_trigger_log(log_id)
            .unwrap()
            .unwrap(),
        log
    );
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(
        port.session_store.list_events(&chat.session_id).unwrap(),
        events
    );
    let requests = provider.requests.lock().unwrap();
    assert_eq!(
        requests.len(),
        1,
        "input and workflow retry must not re-execute the child"
    );
    assert_eq!(requests[0]["model"], "snapshot-model");
    assert_eq!(requests[0]["reasoning_effort"], "MAX_V1");
    assert!(requests[0].get("vendor").is_none());
}
