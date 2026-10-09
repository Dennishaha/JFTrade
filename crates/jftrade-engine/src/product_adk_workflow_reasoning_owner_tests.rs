use super::*;
use crate::product::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationOperation, AdkMutationPort,
};
use crate::product::product_production_ports::ProductionAdkPort;
use jftrade_store_sqlite::AdkArtifactStore;

fn workflow_port(
    directory: &tempfile::TempDir,
    runtime: &Arc<ProductionAdkChatRuntime>,
) -> ProductionAdkPort {
    let artifact = directory.path().join("adk-artifact.db");
    File::create(&artifact).unwrap();
    initialize_current(&Connection::open(&artifact).unwrap(), "adk-artifact").unwrap();
    let mut port = ProductionAdkPort::new_for_test(
        runtime.store.clone(),
        runtime.session_store.clone(),
        Arc::new(AdkArtifactStore::open(artifact).unwrap()),
        directory.path().join("settings.json"),
    );
    port.chat_runtime = Some(runtime.clone());
    port
}

// Parity: go:452dea11:internal/assistant/engine/workflow_canvas_test.go:133 TestRunCanvasWorkflowExecutesAReachableAgentGraph
// This real-runtime control covers model execution and session admission only;
// the Go parent workflow plan/status projections remain separately reviewed.
#[test]
fn production_workflow_real_model_creates_its_session_and_replays_without_execution() {
    let provider = SnapshotProvider::new();
    let (directory, runtime) = snapshot_runtime(
        &provider.endpoint,
        "max",
        json!([{"effort":"max", "value":"MAX_V1"}]),
    );
    let port = workflow_port(&directory, &runtime);
    port.store.upsert_workflow("workflow-reasoning-owner", "ENABLED", &json!({
        "id":"workflow-reasoning-owner", "agentId":"agent-snapshot", "model":"snapshot-model",
        "canvasGraph":{"nodes":[{"id":"start","type":"start"},
            {"id":"research","type":"agent","data":{"message":"Research snapshot"}}],
            "edges":[{"source":"start","target":"research"}]},
    }).to_string()).unwrap();
    let result = port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::RunWorkflow,
            identifiers: BTreeMap::from([(
                "workflowId".to_owned(),
                "workflow-reasoning-owner".to_owned(),
            )]),
            body: json!({"inputs":{}}),
            webhook_secret: None,
        })
        .unwrap();
    runtime.shutdown_with_error().unwrap();
    assert_eq!(result["log"]["status"], "SUCCEEDED", "{result}");
    assert_eq!(result["response"]["run"]["status"], "COMPLETED");
    assert_eq!(result["response"]["reply"], "snapshot resumed");
    let run_id = result["response"]["run"]["id"].as_str().unwrap();
    let session_id = result["response"]["session"]["id"].as_str().unwrap();
    let run = port.store.get_run(run_id).unwrap().unwrap();
    let payload: Value = serde_json::from_str(&run.payload_json).unwrap();
    assert_eq!(payload["reasoningEffort"], "max");
    assert_eq!(payload["reasoningEffortValue"], "MAX_V1");
    assert_eq!(
        port.store
            .get_session_agent_id(session_id)
            .unwrap()
            .as_deref(),
        Some("agent-snapshot")
    );
    let events = port.session_store.list_events(session_id).unwrap();
    assert!(
        events
            .iter()
            .any(|event| event.content == "snapshot resumed")
    );
    let audit = port.store.list_audit_events().unwrap();
    let log_id = result["log"]["id"].as_str().unwrap();
    let log = port
        .store
        .get_workflow_trigger_log(log_id)
        .unwrap()
        .unwrap();
    port.resume_workflow(log_id).unwrap();
    assert_eq!(port.store.get_run(run_id).unwrap().unwrap(), run);
    assert_eq!(
        port.store
            .get_workflow_trigger_log(log_id)
            .unwrap()
            .unwrap(),
        log
    );
    assert_eq!(port.store.list_audit_events().unwrap(), audit);
    assert_eq!(port.session_store.list_events(session_id).unwrap(), events);
    let requests = provider.requests.lock().unwrap();
    assert_eq!(requests.len(), 1);
    assert_eq!(requests[0]["model"], "snapshot-model");
    assert_eq!(requests[0]["vendor"]["current"], "MAX_V1");
}

#[test]
fn production_workflow_legacy_checkpoint_keeps_explicit_session_identity_after_provider_drift() {
    let provider = SnapshotProvider::new();
    let (directory, runtime) = snapshot_runtime(
        &provider.endpoint,
        "max",
        json!([{"effort":"max", "value":"MAX_V1"}]),
    );
    let port = workflow_port(&directory, &runtime);
    let uuid = "11111111-1111-4111-8111-111111111111";
    let workflow_id = "workflow-legacy-snapshot";
    let request_id = format!("workflow-{workflow_id}-research-{uuid}");
    let session_id = format!("workflow-session-research-{uuid}");
    port.store
        .upsert_session(
            &session_id,
            "agent-snapshot",
            &json!({
                "id":session_id, "agentId":"agent-snapshot", "title":"Existing workflow session",
            })
            .to_string(),
        )
        .unwrap();
    let body = json!({"clientRequestId":request_id, "sessionId":session_id,
        "agentId":"agent-snapshot", "providerId":"", "model":"snapshot-model",
        "message":"Research snapshot", "objective":""});
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
        panic!("legacy fixture must create one run")
    };
    let response = runtime
        .persist_success(
            &chat,
            ModelResponse {
                usage_metadata: None,
                text: "old workflow completed".to_owned(),
                tool_calls: Vec::new(),
            },
            &lease,
        )
        .unwrap();
    drop(lease);
    drop(slot);
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
    let run = port.store.get_run(&chat.run_id).unwrap().unwrap();
    let session = port.store.get_session(&session_id).unwrap().unwrap();
    let native = port
        .session_store
        .get_session_by_id(&session_id)
        .unwrap()
        .unwrap();
    let events = port.session_store.list_events(&session_id).unwrap();
    let audit = port.store.list_audit_events().unwrap();
    let workflow = json!({"agentId":"agent-snapshot", "model":"snapshot-model"});
    let graph = json!({"nodes":[{"id":"start", "type":"start"},
        {"id":"research", "type":"agent", "data":{"message":"Research snapshot"}}],
        "edges":[{"source":"start", "target":"research"}]});
    for (index, saved_inputs) in [body.clone(), json!({"message":"Research snapshot", "agentId":"agent-snapshot", "providerId":"", "model":"snapshot-model"})].into_iter().enumerate() {
        let log_id = format!("workflow-legacy-log-{index}");
        let payload = json!({"inputs":{}, "runId":chat.run_id, "sessionId":session_id, "result":response,
            "canvasExecution":{"workflow":workflow, "graph":graph, "invocationUuid":uuid, "trigger":null},
            "nodeRuns":[{"nodeId":"start", "nodeType":"start", "status":"SUCCEEDED"},
                {"nodeId":"research", "nodeType":"agent", "status":"RUNNING", "inputs":saved_inputs,
                "outputs":{"sessionId":session_id, "runId":chat.run_id}}]});
        port.store.create_workflow_trigger_log(&log_id, workflow_id, "", "manual", "RUNNING", &chat.run_id, &payload.to_string()).unwrap();
        port.resume_workflow(&log_id).unwrap();
        let completed = port.store.get_workflow_trigger_log(&log_id).unwrap().unwrap();
        assert_eq!(completed.status, "SUCCEEDED");
        let projection: Value = serde_json::from_str(&completed.payload_json).unwrap();
        assert_eq!(projection["result"]["reply"], "old workflow completed");
        assert_eq!(projection["nodeRuns"].as_array().unwrap().len(), 2);
        assert_eq!(projection["nodeRuns"][1]["nodeId"], "research");
        assert_eq!(projection["nodeRuns"][1]["status"], "SUCCEEDED");
        assert_eq!(projection["result"]["run"]["id"], chat.run_id);
        port.resume_workflow(&log_id).unwrap();
        assert_eq!(port.store.get_workflow_trigger_log(&log_id).unwrap().unwrap(), completed);
        assert_eq!(port.store.get_run(&chat.run_id).unwrap().unwrap(), run);
        assert_eq!(port.store.get_session(&session_id).unwrap().unwrap(), session);
        assert_eq!(port.session_store.get_session_by_id(&session_id).unwrap().unwrap(), native);
        assert_eq!(port.session_store.list_events(&session_id).unwrap(), events);
        assert_eq!(port.store.list_audit_events().unwrap(), audit);
    }
    runtime.shutdown_with_error().unwrap();
    assert!(
        provider.requests.lock().unwrap().is_empty(),
        "existing child must replay before current provider validation"
    );
}

#[path = "product_adk_workflow_pending_reasoning_owner_tests.rs"]
mod pending_reasoning;
