#![forbid(unsafe_code)]

use std::collections::BTreeMap;
use std::path::Path;
use std::sync::{Arc, Mutex};

use jftrade_engine::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute, AdkChatStreamPort,
};
use jftrade_engine::product::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationOperation, AdkMutationPort,
};
use jftrade_engine::product::product_production_ports::product_production_ports_adk::ProductionAdkPort;
use jftrade_engine::product::{AdkReadSnapshot, AdkReadSnapshotPort};
use jftrade_store_sqlite::{AdkArtifactStore, AdkSessionStore, AdkStore, initialize_current};
use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

#[derive(Debug, Default)]
struct RecordingChat {
    requests: Mutex<Vec<Value>>,
    reply: Mutex<Option<String>>,
}

impl AdkChatStreamPort for RecordingChat {
    fn dispatch(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        assert_eq!(route, AdkChatRoute::Chat);
        let body: Value = serde_json::from_slice(&input.body).expect("chat body");
        assert_eq!(body["clientRequestId"], input.client_request_id);
        let mut requests = self.requests.lock().expect("requests");
        requests.push(body);
        let sequence = requests.len();
        let reply = self
            .reply
            .lock()
            .expect("reply")
            .clone()
            .unwrap_or_else(|| "ok".to_owned());
        Ok(AdkChatPortOutput::Json(json!({
            "run": {"id": format!("child-{sequence}"), "status": "COMPLETED", "outputSummary": reply},
            "session": {"id": format!("session-{sequence}")},
            "reply": reply,
        })))
    }
}

fn open_port(root: &Path, chat: &Arc<RecordingChat>) -> ProductionAdkPort {
    let mut port = ProductionAdkPort::new_for_test(
        Arc::new(AdkStore::open(root.join("adk.db")).expect("ADK store")),
        Arc::new(AdkSessionStore::open(root.join("adk-session.db")).expect("sessions")),
        Arc::new(AdkArtifactStore::open(root.join("adk-artifact.db")).expect("artifacts")),
        root.join("settings.json"),
    );
    port.chat_runtime = Some(Arc::clone(chat) as Arc<dyn AdkChatStreamPort>);
    port
}

fn fixture(root: &Path, chat: &Arc<RecordingChat>) -> ProductionAdkPort {
    for component in ["adk", "adk-session", "adk-artifact"] {
        let connection = Connection::open(root.join(format!("{component}.db"))).expect("database");
        initialize_current(&connection, component).expect("schema");
    }
    open_port(root, chat)
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
            .map(|(k, v)| ((*k).to_owned(), (*v).to_owned()))
            .collect::<BTreeMap<_, _>>(),
        body,
        webhook_secret: None,
    })
    .expect("production mutation")
}

fn create_agent(port: &ProductionAdkPort, id: &str) -> String {
    mutate(
        port,
        AdkMutationOperation::CreateAgent,
        &[],
        json!({
            "id": id, "name": id, "instruction": "workflow behavior fixture", "model": "test-model",
        }),
    )["id"]
        .as_str()
        .expect("agent id")
        .to_owned()
}

fn create_workflow(port: &ProductionAdkPort, agent_id: &str, graph: Option<Value>) -> String {
    let mut body = json!({
        "name": "Workflow trace", "agentId": agent_id, "workMode": "chat",
        "promptTemplate": "run {{ .symbol }}", "defaultInputs": {"symbol": "US.AAPL"},
    });
    if let Some(graph) = graph {
        body["canvasGraph"] = graph;
    }
    mutate(port, AdkMutationOperation::CreateWorkflow, &[], body)["id"]
        .as_str()
        .expect("workflow id")
        .to_owned()
}

fn run(port: &ProductionAdkPort, workflow_id: &str, inputs: Value) -> Value {
    mutate(
        port,
        AdkMutationOperation::RunWorkflow,
        &[("workflowId", workflow_id)],
        json!({"inputs": inputs}),
    )
}

fn read(port: &ProductionAdkPort, path: &str) -> Value {
    match port.read(path, "").expect("production read") {
        AdkReadSnapshot::Json(value) => value,
        _ => panic!("expected JSON"),
    }
}

fn stored_log(port: &ProductionAdkPort, result: &Value) -> Value {
    let id = result["log"]["id"].as_str().expect("log id");
    let row = port
        .store
        .get_workflow_trigger_log(id)
        .expect("read log")
        .expect("durable log");
    assert_eq!(row.status, "SUCCEEDED");
    serde_json::from_str(&row.payload_json).expect("log payload")
}

fn trace_graph() -> Value {
    json!({
        "version": "adk-workflow-canvas/v1",
        "nodes": [
            {"id": "start", "type": "start", "position": {"x": 0, "y": 0}},
            {"id": "agent:primary", "type": "agent", "position": {"x": 100, "y": 0}},
            {"id": "monitor", "type": "monitor", "position": {"x": 200, "y": 0}},
        ],
        "edges": [
            {"id": "start-agent", "source": "start", "target": "agent:primary"},
            {"id": "agent-monitor", "source": "agent:primary", "target": "monitor"},
        ],
    })
}

// Parity: go:452dea11:internal/assistant/workflows_test.go:157 TestSaveWorkflowRoundTripsCanvasGraph
#[test]
fn workflow_canvas_save_read_and_restart_preserve_the_submitted_graph() {
    let directory = tempdir().expect("directory");
    let chat = Arc::new(RecordingChat::default());
    let port = fixture(directory.path(), &chat);
    let agent_id = create_agent(&port, "workflow-canvas-agent");
    let graph = json!({
        "version": "adk-workflow-canvas/v1",
        "nodes": [
            {"id": "start", "type": "start", "position": {"x": 80, "y": 250}},
            {"id": "agent", "type": "agent", "position": {"x": 385, "y": 250}},
        ],
        "edges": [{"id": "start->agent", "source": "start", "target": "agent", "type": "smoothstep"}],
    });
    let created = mutate(
        &port,
        AdkMutationOperation::CreateWorkflow,
        &[],
        json!({
            "id": "workflow-canvas-roundtrip", "name": "Canvas Round Trip",
            "status": "DISABLED", "agentId": agent_id, "workMode": "loop",
            "promptTemplate": "run canvas", "canvasGraph": graph,
        }),
    );
    let id = created["id"].as_str().expect("workflow id");
    assert_eq!(created["canvasGraph"], graph);
    assert_eq!(created["status"], "DISABLED");
    let path = format!("/api/v1/adk/workflows/{id}");
    assert_eq!(read(&port, &path)["canvasGraph"], graph);
    assert!(chat.requests.lock().expect("requests").is_empty());
    drop(port);
    let reopened = open_port(directory.path(), &chat);
    assert_eq!(read(&reopened, &path)["canvasGraph"], graph);
    let row = reopened
        .store
        .get_workflow(id)
        .expect("workflow read")
        .expect("workflow");
    let payload: Value = serde_json::from_str(&row.payload_json).expect("workflow payload");
    assert_eq!(payload["canvasGraph"], graph);
}

// Parity: go:452dea11:internal/assistant/workflows_test.go:198 TestRunWorkflowStoresResultAndNodeTrace
#[test]
fn workflow_run_persists_ordered_start_agent_monitor_outputs() {
    let directory = tempdir().expect("directory");
    let chat = Arc::new(RecordingChat::default());
    let port = fixture(directory.path(), &chat);
    let agent_id = create_agent(&port, "workflow-trace-agent");
    let id = create_workflow(&port, &agent_id, Some(trace_graph()));
    let result = run(&port, &id, json!({"symbol": "US.MSFT"}));
    assert_eq!(result["log"]["status"], "SUCCEEDED");
    let nodes = result["log"]["nodeRuns"].as_array().expect("node runs");
    assert_eq!(
        nodes
            .iter()
            .map(|n| n["nodeId"].as_str().expect("node id"))
            .collect::<Vec<_>>(),
        ["start", "agent:primary", "monitor"]
    );
    assert_eq!(nodes[1]["outputs"]["reply"], "ok");
    assert_eq!(nodes[2]["outputs"]["lastReply"], "ok");
    assert_eq!(result["log"]["result"]["reply"], "ok");
    let stored = stored_log(&port, &result);
    assert_eq!(stored["nodeRuns"], result["log"]["nodeRuns"]);
    assert_eq!(stored["result"], result["log"]["result"]);
    // Rust stores the model response object; Go's Result.Markdown is absent.
    assert!(stored["result"].get("markdown").is_none());
}

// Parity: go:452dea11:internal/assistant/workflows_test.go:276 TestRunWorkflowCanvasCompilesAndStoresNodeOutputs
#[test]
fn workflow_child_nodes_receive_rendered_inputs_and_persist_their_outputs() {
    let directory = tempdir().expect("directory");
    let chat = Arc::new(RecordingChat::default());
    let port = fixture(directory.path(), &chat);
    let parent = create_agent(&port, "workflow-canvas-agent");
    let child = create_agent(&port, "workflow-canvas-child");
    let graph = json!({
        "version": "adk-workflow-canvas/v1",
        "nodes": [
            {"id": "start", "type": "start"},
            {"id": "research", "type": "agent", "data": {"title": "Research", "agentId": child, "promptTemplate": "research {{ .symbol }}"}},
            {"id": "report", "type": "agent", "data": {"title": "Report", "promptTemplate": "report {{ .symbol }}"}},
            {"id": "monitor", "type": "monitor"},
        ],
        "edges": [
            {"id": "start-research", "source": "start", "target": "research"},
            {"id": "research-report", "source": "research", "target": "report"},
            {"id": "report-monitor", "source": "report", "target": "monitor"},
        ],
    });
    let id = create_workflow(&port, &parent, Some(graph));
    let result = run(&port, &id, json!({"symbol": "US.MSFT"}));
    let requests = chat.requests.lock().expect("requests");
    assert_eq!(requests.len(), 2);
    assert_eq!(requests[0]["agentId"], child);
    assert_eq!(requests[0]["message"], "research US.MSFT");
    assert_eq!(requests[1]["agentId"], parent);
    assert_eq!(requests[1]["message"], "report US.MSFT");
    let stored = stored_log(&port, &result);
    let nodes = stored["nodeRuns"].as_array().expect("nodes");
    assert_eq!(
        nodes
            .iter()
            .map(|n| n["nodeId"].as_str().expect("node id"))
            .collect::<Vec<_>>(),
        ["start", "research", "report", "monitor"]
    );
    for (index, request) in requests.iter().enumerate() {
        assert_eq!(nodes[index + 1]["inputs"]["agentId"], request["agentId"]);
        assert_eq!(nodes[index + 1]["inputs"]["message"], request["message"]);
        assert_eq!(nodes[index + 1]["outputs"]["reply"], "ok");
    }
    // The response remains the last child, not Go's aggregate parent run.
    assert!(result["response"]["run"].get("workflowEngine").is_none());
    assert!(result["response"]["run"].get("childRunIDs").is_none());
}

// Parity: go:452dea11:internal/assistant/workflows_test.go:248 TestRunWorkflowWithoutCanvasGraphFailsInsteadOfChatFallback
#[test]
fn workflow_without_canvas_uses_the_existing_durable_single_agent_path() {
    let directory = tempdir().expect("directory");
    let chat = Arc::new(RecordingChat::default());
    let port = fixture(directory.path(), &chat);
    let agent = create_agent(&port, "workflow-no-canvas-agent");
    let id = create_workflow(&port, &agent, None);
    let result = run(&port, &id, json!({}));
    assert_eq!(result["log"]["status"], "SUCCEEDED");
    assert_eq!(result["response"]["reply"], "ok");
    assert_eq!(chat.requests.lock().expect("requests").len(), 1);
    let stored = stored_log(&port, &result);
    assert_eq!(stored["nodeRuns"].as_array().expect("nodes").len(), 4);
    assert_eq!(stored["nodeRuns"][2]["nodeId"], "agent");
    // Frozen Go rejects this request instead of synthesizing a graph.
    assert_eq!(stored["nodeRuns"][2]["outputs"]["reply"], "ok");
}

// Parity: go:452dea11:internal/assistant/engine/workflow_canvas_test.go:133 TestRunCanvasWorkflowExecutesAReachableAgentGraph
#[test]
fn reachable_workflow_graph_executes_once_and_records_terminal_node_states() {
    let directory = tempdir().expect("directory");
    let chat = Arc::new(RecordingChat::default());
    let port = fixture(directory.path(), &chat);
    let agent = create_agent(&port, "canvas-run-agent");
    let mut graph = trace_graph();
    graph["nodes"][1]["data"] =
        json!({"title": "Research", "message": "Summarize the requested market signal."});
    let id = create_workflow(&port, &agent, Some(graph));
    let result = run(&port, &id, json!({}));
    assert_eq!(
        chat.requests.lock().expect("requests")[0]["message"],
        "Summarize the requested market signal."
    );
    assert_eq!(chat.requests.lock().expect("requests").len(), 1);
    assert_eq!(result["response"]["run"]["status"], "COMPLETED");
    assert_eq!(result["response"]["reply"], "ok");
    let stored = stored_log(&port, &result);
    for node in stored["nodeRuns"].as_array().expect("nodes") {
        assert_eq!(node["status"], "SUCCEEDED");
    }
    assert!(result["response"]["run"].get("workflowPlan").is_none());
    assert!(result["response"]["run"].get("workflowStatus").is_none());
}

#[test]
fn workflow_template_input_values_are_not_evaluated_as_other_placeholders() {
    let directory = tempdir().expect("directory");
    let chat = Arc::new(RecordingChat::default());
    let port = fixture(directory.path(), &chat);
    let agent = create_agent(&port, "template-data-agent");
    let mut graph = trace_graph();
    graph["nodes"][1]["data"] = json!({
        "promptTemplate": "数据 symbol={{input.symbol}}; go={{ .symbol }}; alias={{inputs.symbol}}; json={{payload}}; missing={{unknown}}; {{unfinished",
    });
    let id = create_workflow(&port, &agent, Some(graph));
    let result = run(
        &port,
        &id,
        json!({
        "symbol": "{{input.zeta}}", "zeta": "must remain data", "payload": {"text": "引号\"和换行\n"},
        }),
    );
    assert_eq!(
        chat.requests.lock().expect("requests")[0]["message"],
        "数据 symbol={{input.zeta}}; go={{input.zeta}}; alias={{input.zeta}}; json={\"text\":\"引号\\\"和换行\\n\"}; missing={{unknown}}; {{unfinished"
    );
    assert_eq!(
        stored_log(&port, &result)["nodeRuns"][1]["outputs"]["reply"],
        "ok"
    );
}

#[test]
fn workflow_node_output_placeholders_remain_literal_downstream_data() {
    let directory = tempdir().expect("directory");
    let chat = Arc::new(RecordingChat::default());
    *chat.reply.lock().expect("reply") = Some("{{input.zeta}} {{research.reply}}".to_owned());
    let port = fixture(directory.path(), &chat);
    let agent = create_agent(&port, "node-output-data-agent");
    let graph = json!({
        "nodes": [
            {"id": "start", "type": "start"},
            {"id": "research", "type": "agent", "data": {"message": "research"}},
            {"id": "report", "type": "agent", "data": {"message": "received={{research.reply}}; shorthand={{research}}; z={{input.zeta}}"}},
        ],
        "edges": [
            {"source": "start", "target": "research"},
            {"source": "research", "target": "report"},
        ],
    });
    let id = create_workflow(&port, &agent, Some(graph));
    let result = run(&port, &id, json!({"zeta": "replacement"}));
    let requests = chat.requests.lock().expect("requests");
    assert_eq!(requests.len(), 2);
    assert_eq!(
        requests[1]["message"],
        "received={{input.zeta}} {{research.reply}}; shorthand={{input.zeta}} {{research.reply}}; z=replacement"
    );
    assert_eq!(
        stored_log(&port, &result)["nodeRuns"][2]["inputs"]["message"],
        requests[1]["message"]
    );
}
