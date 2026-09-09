#![forbid(unsafe_code)]

use std::collections::BTreeMap;
use std::fs::File;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};

use jftrade_engine::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute, AdkChatStreamPort,
};
use jftrade_engine::product::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationOperation, AdkMutationPort, AdkMutationPortError,
};
use jftrade_engine::product::product_production_ports::product_production_ports_adk::ProductionAdkPort;
use jftrade_store_sqlite::{AdkArtifactStore, AdkSessionStore, AdkStore, initialize_current};
use rusqlite::Connection;
use serde_json::{Value, json};
use tempfile::tempdir;

#[derive(Debug, Default)]
struct MockChatRuntime {
    panic_on_keyword: Mutex<Option<String>>,
    pending_status: Mutex<Option<String>>,
    request_ids: Mutex<Vec<String>>,
    recorded_messages: Arc<Mutex<Vec<String>>>,
    fail_on_keyword: Arc<Mutex<Option<String>>>,
}

impl MockChatRuntime {
    fn new() -> Self {
        Self::default()
    }
}

impl AdkChatStreamPort for MockChatRuntime {
    fn dispatch(
        &self,
        _route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        let parsed: Value = serde_json::from_slice(&input.body).unwrap_or(Value::Null);
        let message = parsed
            .get("message")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_owned();

        self.recorded_messages.lock().unwrap().push(message.clone());
        self.request_ids
            .lock()
            .unwrap()
            .push(input.client_request_id.clone());
        let status = self
            .pending_status
            .lock()
            .unwrap()
            .clone()
            .unwrap_or_else(|| "SUCCEEDED".to_owned());
        let panic_keyword = self.panic_on_keyword.lock().unwrap().clone();
        assert!(
            !panic_keyword.is_some_and(|keyword| message.contains(&keyword)),
            "injected process-boundary failure"
        );

        if let Some(ref keyword) = *self.fail_on_keyword.lock().unwrap()
            && message.contains(keyword)
        {
            return Err(AdkChatPortError::Failed {
                status: 500,
                code: "SIMULATED_FAILURE".to_owned(),
                message: format!("Simulated node failure on keyword: {keyword}"),
            });
        }

        static COUNTER: AtomicU64 = AtomicU64::new(1);
        let count = COUNTER.fetch_add(1, Ordering::Relaxed);
        let reply = format!("Processed: {message}");
        Ok(AdkChatPortOutput::Json(json!({
            "run": {
                "id": format!("run-mock-{count}"),
                "status": status,
                "outputSummary": reply,
            },
            "reply": reply,
            "session": {
                "id": format!("session-mock-{count}"),
            }
        })))
    }
}

fn initialize_db(path: &Path, component: &str) {
    File::create(path).expect("create database file");
    let connection = Connection::open(path).expect("open sqlite");
    initialize_current(&connection, component).expect("initialize schema");
}

struct EngineTestCluster {
    _dir: tempfile::TempDir,
    port: Arc<ProductionAdkPort>,
    mock_chat: Arc<MockChatRuntime>,
    adk_path: PathBuf,
}

impl EngineTestCluster {
    fn new() -> Self {
        let dir = tempdir().expect("tempdir");
        let adk_path = dir.path().join("adk.db");
        let session_path = dir.path().join("adk-session.db");
        let artifact_path = dir.path().join("adk-artifact.db");
        let settings_path = dir.path().join("settings.json");

        initialize_db(&adk_path, "adk");
        initialize_db(&session_path, "adk-session");
        initialize_db(&artifact_path, "adk-artifact");

        let store = Arc::new(AdkStore::open(&adk_path).expect("open adk store"));
        let session_store =
            Arc::new(AdkSessionStore::open(&session_path).expect("open session store"));
        let artifact_store =
            Arc::new(AdkArtifactStore::open(&artifact_path).expect("open artifact store"));

        let mut port =
            ProductionAdkPort::new_for_test(store, session_store, artifact_store, settings_path);
        let mock_chat = Arc::new(MockChatRuntime::new());
        port.chat_runtime = Some(Arc::clone(&mock_chat) as Arc<dyn AdkChatStreamPort>);

        Self {
            _dir: dir,
            port: Arc::new(port),
            mock_chat,
            adk_path,
        }
    }

    fn create_agent(&self, name: &str) -> String {
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateAgent,
            identifiers: BTreeMap::new(),
            body: json!({
                "name": name,
                "description": "Test Agent",
                "instruction": "You are a test agent",
                "model": "gpt-4o",
            }),
            webhook_secret: None,
        };
        let res = self.port.mutate(&input).expect("create agent");
        res["id"].as_str().expect("agent id").to_owned()
    }

    fn create_canvas_workflow(&self, agent_id: &str, name: &str, canvas: Value) -> String {
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateWorkflow,
            identifiers: BTreeMap::new(),
            body: json!({
                "name": name,
                "agentId": agent_id,
                "promptTemplate": "canvas workflow template",
                "canvasGraph": canvas,
            }),
            webhook_secret: None,
        };
        let res = self.port.mutate(&input).expect("create workflow");
        res["id"].as_str().expect("workflow id").to_owned()
    }

    fn run_workflow(
        &self,
        workflow_id: &str,
        inputs: Value,
    ) -> Result<Value, AdkMutationPortError> {
        let mut identifiers = BTreeMap::new();
        identifiers.insert("workflowId".to_owned(), workflow_id.to_owned());
        let input = AdkMutationInput {
            operation: AdkMutationOperation::RunWorkflow,
            identifiers,
            body: json!({ "inputs": inputs }),
            webhook_secret: None,
        };
        self.port.mutate(&input)
    }
}

#[test]
fn test_canvas_workflow_multi_node_execution_and_context_propagation() {
    let cluster = EngineTestCluster::new();
    let agent_id = cluster.create_agent("AlphaTrader");

    let canvas_graph = json!({
        "nodes": [
            {
                "id": "start-1",
                "type": "start",
                "title": "Start Trigger",
                "data": {}
            },
            {
                "id": "agent-step-1",
                "type": "agent",
                "title": "Step 1 Technical Analysis",
                "data": {
                    "promptTemplate": "Analyze symbol {{input.symbol}}",
                    "agentId": agent_id
                }
            },
            {
                "id": "agent-step-2",
                "type": "agent",
                "title": "Step 2 Risk Assessment",
                "data": {
                    "promptTemplate": "Evaluate risk given analysis: {{agent-step-1.reply}} with market {{input.market}}",
                    "agentId": agent_id
                }
            },
            {
                "id": "monitor-end",
                "type": "monitor",
                "title": "Monitoring Check",
                "data": {
                    "metric": "pnl"
                }
            }
        ],
        "edges": [
            {
                "id": "e1",
                "source": "start-1",
                "target": "agent-step-1"
            },
            {
                "id": "e2",
                "source": "agent-step-1",
                "target": "agent-step-2"
            },
            {
                "id": "e3",
                "source": "agent-step-2",
                "target": "monitor-end"
            }
        ]
    });

    let workflow_id = cluster.create_canvas_workflow(&agent_id, "MultiNodeDAG", canvas_graph);

    let run_res = cluster
        .run_workflow(&workflow_id, json!({ "symbol": "NVDA", "market": "US" }))
        .expect("run workflow should succeed");

    assert_eq!(
        run_res["log"]["status"], "SUCCEEDED",
        "Overall workflow status should be SUCCEEDED"
    );

    // 1. Verify context propagation:
    // agent-step-1 received: "Analyze symbol NVDA"
    // agent-step-2 received: "Evaluate risk given analysis: Processed: Analyze symbol NVDA with market US"
    let recorded = cluster.mock_chat.recorded_messages.lock().unwrap().clone();
    assert_eq!(
        recorded.len(),
        2,
        "Two agent nodes should have dispatched to chat runtime"
    );
    assert_eq!(recorded[0], "Analyze symbol NVDA");
    assert_eq!(
        recorded[1], "Evaluate risk given analysis: Processed: Analyze symbol NVDA with market US",
        "Context interpolation from upstream agent-step-1.reply and input.market must be accurate"
    );

    // 2. Verify SQLite nodeRuns persistence conforming to OpenAPI adk.WorkflowNodeRun
    let conn = Connection::open(&cluster.adk_path).expect("open sqlite");
    let mut stmt = conn
        .prepare("SELECT payload_json FROM adk_workflow_trigger_logs WHERE workflow_id = ?1")
        .expect("prepare stmt");
    let mut rows = stmt.query([&workflow_id]).expect("query rows");
    let row = rows.next().expect("next row").expect("has trigger log");
    let payload_str: String = row.get(0).expect("payload_json col");
    let payload: Value = serde_json::from_str(&payload_str).expect("valid json payload");

    let node_runs = payload["nodeRuns"].as_array().expect("nodeRuns array");
    assert_eq!(node_runs.len(), 4, "Must record node runs for all 4 nodes");

    // Node 0: start-1
    assert_eq!(node_runs[0]["nodeId"], "start-1");
    assert_eq!(node_runs[0]["nodeType"], "start");
    assert_eq!(node_runs[0]["status"], "SUCCEEDED");
    assert!(node_runs[0]["startedAt"].is_string());
    assert!(node_runs[0]["finishedAt"].is_string());

    // Node 1: agent-step-1
    assert_eq!(node_runs[1]["nodeId"], "agent-step-1");
    assert_eq!(node_runs[1]["nodeType"], "agent");
    assert_eq!(node_runs[1]["status"], "SUCCEEDED");
    assert_eq!(
        node_runs[1]["outputs"]["reply"],
        "Processed: Analyze symbol NVDA"
    );

    // Node 2: agent-step-2
    assert_eq!(node_runs[2]["nodeId"], "agent-step-2");
    assert_eq!(node_runs[2]["nodeType"], "agent");
    assert_eq!(node_runs[2]["status"], "SUCCEEDED");
    assert_eq!(
        node_runs[2]["outputs"]["reply"],
        "Processed: Evaluate risk given analysis: Processed: Analyze symbol NVDA with market US"
    );

    // Node 3: monitor-end
    assert_eq!(node_runs[3]["nodeId"], "monitor-end");
    assert_eq!(node_runs[3]["nodeType"], "monitor");
    assert_eq!(node_runs[3]["status"], "SUCCEEDED");
}

#[test]
fn approval_and_running_nodes_suspend_then_resume_the_same_durable_request() {
    for pending in ["PENDING_APPROVAL", "PENDING_INPUT", "RUNNING"] {
        let cluster = EngineTestCluster::new();
        let agent = cluster.create_agent("resume-agent");
        let graph = json!({"nodes":[{"id":"start","type":"start"},{"id":"a","type":"agent","data":{"message":"first"}},{"id":"b","type":"agent","data":{"message":"second {{a.reply}}"}}],"edges":[{"source":"start","target":"a"},{"source":"a","target":"b"}]});
        let workflow = cluster.create_canvas_workflow(&agent, "resume", graph);
        *cluster.mock_chat.pending_status.lock().unwrap() = Some(pending.to_owned());
        let result = cluster.run_workflow(&workflow, json!({})).unwrap();
        assert_eq!(cluster.mock_chat.recorded_messages.lock().unwrap().len(), 1);
        assert_ne!(result["log"]["status"], "SUCCEEDED");
        assert!(
            result["log"].get("canvasExecution").is_none(),
            "private recovery state must not leak to HTTP"
        );
        assert!(result["log"]["nodeRuns"][1].get("finishedAt").is_none());
        let id = result["log"]["id"].as_str().unwrap();
        cluster
            .port
            .store
            .recover_orphaned_workflow_trigger_logs()
            .unwrap();
        *cluster.mock_chat.pending_status.lock().unwrap() = None;
        cluster.port.resume_workflow(id).unwrap();
        let row = cluster
            .port
            .store
            .get_workflow_trigger_log(id)
            .unwrap()
            .unwrap();
        assert_eq!(row.status, "SUCCEEDED");
        let calls = cluster.mock_chat.request_ids.lock().unwrap();
        assert_eq!(calls.len(), 3);
        assert_eq!(
            calls[0], calls[1],
            "the pending node must reuse its idempotency key"
        );
        assert_ne!(calls[1], calls[2]);
        drop(calls);
        cluster.port.resume_workflow(id).unwrap();
        assert_eq!(cluster.mock_chat.request_ids.lock().unwrap().len(), 3);
    }
}

#[test]
fn unknown_model_status_is_not_a_successful_canvas_node() {
    let cluster = EngineTestCluster::new();
    let agent = cluster.create_agent("unknown-status");
    let workflow = cluster.create_canvas_workflow(&agent, "unknown", json!({"nodes":[{"id":"start","type":"start"},{"id":"a","type":"agent"}],"edges":[{"source":"start","target":"a"}]}));
    *cluster.mock_chat.pending_status.lock().unwrap() = Some("BROKEN".to_owned());
    let result = cluster.run_workflow(&workflow, json!({})).unwrap();
    assert_eq!(result["log"]["status"], "FAILED");
}

#[test]
fn restart_keeps_completed_nodes_and_the_inflight_request_identity() {
    let cluster = EngineTestCluster::new();
    let agent = cluster.create_agent("crash-agent");
    let workflow = cluster.create_canvas_workflow(&agent, "crash", json!({
        "nodes":[{"id":"start","type":"start"},{"id":"a","type":"agent","data":{"message":"first"}},{"id":"b","type":"agent","data":{"message":"second {{a.reply}}"}}],
        "edges":[{"source":"start","target":"a"},{"source":"a","target":"b"}]
    }));
    *cluster.mock_chat.panic_on_keyword.lock().unwrap() = Some("second".to_owned());
    let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        cluster.run_workflow(&workflow, json!({}))
    }));
    assert!(result.is_err());
    let rows = cluster.port.store.list_workflow_trigger_logs().unwrap();
    assert_eq!(rows.len(), 1);
    let payload: Value = serde_json::from_str(&rows[0].payload_json).unwrap();
    assert_eq!(payload["nodeRuns"][1]["status"], "SUCCEEDED");
    assert_eq!(payload["nodeRuns"][2]["status"], "RUNNING");
    let EngineTestCluster {
        _dir,
        port,
        mock_chat,
        adk_path,
    } = cluster;
    drop(port);
    let store = Arc::new(AdkStore::open(&adk_path).unwrap());
    assert_eq!(store.recover_orphaned_workflow_trigger_logs().unwrap(), 0);
    let mut restarted = ProductionAdkPort::new_for_test(
        store.clone(),
        Arc::new(AdkSessionStore::open(_dir.path().join("adk-session.db")).unwrap()),
        Arc::new(AdkArtifactStore::open(_dir.path().join("adk-artifact.db")).unwrap()),
        _dir.path().join("settings.json"),
    );
    *mock_chat.panic_on_keyword.lock().unwrap() = None;
    restarted.chat_runtime = Some(mock_chat.clone());
    restarted.resume_workflow(&rows[0].id).unwrap();
    assert_eq!(
        store
            .get_workflow_trigger_log(&rows[0].id)
            .unwrap()
            .unwrap()
            .status,
        "SUCCEEDED"
    );
    let messages = mock_chat.recorded_messages.lock().unwrap();
    assert_eq!(messages.iter().filter(|m| m.as_str() == "first").count(), 1);
    let ids = mock_chat.request_ids.lock().unwrap();
    assert_eq!(ids.len(), 3);
    assert_eq!(ids[1], ids[2]);
}

#[test]
fn test_canvas_workflow_cycle_detection_rejects_with_400() {
    let cluster = EngineTestCluster::new();
    let agent_id = cluster.create_agent("CycleAgent");

    // Canvas with cycle: node-a -> node-b -> node-a
    let cyclic_canvas = json!({
        "nodes": [
            {
                "id": "node-a",
                "type": "agent",
                "title": "Node A",
                "data": { "promptTemplate": "Loop A", "agentId": agent_id }
            },
            {
                "id": "node-b",
                "type": "agent",
                "title": "Node B",
                "data": { "promptTemplate": "Loop B", "agentId": agent_id }
            }
        ],
        "edges": [
            {
                "id": "e1",
                "source": "node-a",
                "target": "node-b"
            },
            {
                "id": "e2",
                "source": "node-b",
                "target": "node-a"
            }
        ]
    });

    let workflow_id = cluster.create_canvas_workflow(&agent_id, "CyclicWorkflow", cyclic_canvas);

    let err = cluster
        .run_workflow(&workflow_id, json!({}))
        .expect_err("cyclic workflow execution must be rejected");

    match err {
        AdkMutationPortError::Failed {
            status, message, ..
        } => {
            assert_eq!(status, 400, "Must return HTTP 400 Bad Request");
            assert!(
                message.contains("workflow canvas contains a cycle"),
                "Error message must indicate cycle, got: {message}"
            );
        }
        other => panic!("Expected AdkMutationPortError::Failed with 400, got {other:?}"),
    }
}

#[test]
fn test_canvas_workflow_self_loop_and_invalid_edges_rejected() {
    let cluster = EngineTestCluster::new();
    let agent_id = cluster.create_agent("ValidationAgent");

    // 1. Self loop
    let self_loop_canvas = json!({
        "nodes": [
            {
                "id": "node-x",
                "type": "agent",
                "title": "Self Looping Node",
                "data": { "agentId": agent_id }
            }
        ],
        "edges": [
            {
                "id": "self-edge",
                "source": "node-x",
                "target": "node-x"
            }
        ]
    });

    let loop_workflow_id =
        cluster.create_canvas_workflow(&agent_id, "SelfLoopWorkflow", self_loop_canvas);
    let loop_err = cluster
        .run_workflow(&loop_workflow_id, json!({}))
        .expect_err("self loop must be rejected");

    match loop_err {
        AdkMutationPortError::Failed {
            status, message, ..
        } => {
            assert_eq!(status, 400);
            assert!(
                message.contains("connects node to itself"),
                "Expected self loop error message, got: {message}"
            );
        }
        other => panic!("Expected 400 error, got {other:?}"),
    }

    // 2. Dangling edge referencing unknown node
    let dangling_edge_canvas = json!({
        "nodes": [
            {
                "id": "node-valid",
                "type": "start",
                "title": "Valid Start",
                "data": {}
            }
        ],
        "edges": [
            {
                "id": "broken-edge",
                "source": "node-valid",
                "target": "node-nonexistent"
            }
        ]
    });

    let dangling_workflow_id =
        cluster.create_canvas_workflow(&agent_id, "DanglingWorkflow", dangling_edge_canvas);
    let dangling_err = cluster
        .run_workflow(&dangling_workflow_id, json!({}))
        .expect_err("dangling edge must be rejected");

    match dangling_err {
        AdkMutationPortError::Failed {
            status, message, ..
        } => {
            assert_eq!(status, 400);
            assert!(
                message.contains("references unknown"),
                "Expected unknown node error message, got: {message}"
            );
        }
        other => panic!("Expected 400 error, got {other:?}"),
    }
}

#[test]
fn test_legacy_workflow_synthesizes_noderuns_for_backward_compatibility() {
    let cluster = EngineTestCluster::new();
    let agent_id = cluster.create_agent("LegacyAgent");

    // Legacy workflow without canvasGraph
    let input = AdkMutationInput {
        operation: AdkMutationOperation::CreateWorkflow,
        identifiers: BTreeMap::new(),
        body: json!({
            "name": "LegacySinglePromptWorkflow",
            "agentId": agent_id,
            "promptTemplate": "Analyze legacy ticker {{input.ticker}}",
        }),
        webhook_secret: None,
    };
    let res = cluster.port.mutate(&input).expect("create legacy workflow");
    let workflow_id = res["id"].as_str().expect("workflow id");

    let run_res = cluster
        .run_workflow(workflow_id, json!({ "ticker": "TSLA" }))
        .expect("legacy workflow execution should succeed");

    assert_eq!(run_res["log"]["status"], "SUCCEEDED");

    let recorded = cluster.mock_chat.recorded_messages.lock().unwrap().clone();
    assert_eq!(recorded.len(), 1);
    assert_eq!(recorded[0], "Analyze legacy ticker TSLA");

    // Verify synthesized nodeRuns in SQLite trigger log
    let conn = Connection::open(&cluster.adk_path).expect("open sqlite");
    let mut stmt = conn
        .prepare("SELECT payload_json FROM adk_workflow_trigger_logs WHERE workflow_id = ?1")
        .expect("prepare stmt");
    let mut rows = stmt.query([workflow_id]).expect("query rows");
    let row = rows.next().expect("next row").expect("has trigger log");
    let payload_str: String = row.get(0).expect("payload_json col");
    let payload: Value = serde_json::from_str(&payload_str).expect("valid json payload");

    let node_runs = payload["nodeRuns"].as_array().expect("nodeRuns array");
    assert_eq!(
        node_runs.len(),
        4,
        "Legacy workflow must synthesize 4 node runs"
    );

    assert_eq!(node_runs[0]["nodeId"], "trigger");
    assert_eq!(node_runs[0]["nodeType"], "trigger");
    assert_eq!(node_runs[0]["status"], "SUCCEEDED");

    assert_eq!(node_runs[1]["nodeId"], "start");
    assert_eq!(node_runs[1]["nodeType"], "start");
    assert_eq!(node_runs[1]["status"], "SUCCEEDED");

    assert_eq!(node_runs[2]["nodeId"], "agent");
    assert_eq!(node_runs[2]["nodeType"], "agent");
    assert_eq!(node_runs[2]["status"], "SUCCEEDED");

    assert_eq!(node_runs[3]["nodeId"], "monitor");
    assert_eq!(node_runs[3]["nodeType"], "monitor");
    assert_eq!(node_runs[3]["status"], "SUCCEEDED");
}

#[test]
fn test_canvas_workflow_error_skips_downstream_nodes() {
    let cluster = EngineTestCluster::new();
    let agent_id = cluster.create_agent("FaultyAgent");

    // Set mock to fail when encountering keyword "FAIL_HERE"
    *cluster.mock_chat.fail_on_keyword.lock().unwrap() = Some("FAIL_HERE".to_owned());

    let canvas_graph = json!({
        "nodes": [
            {
                "id": "start-node",
                "type": "start",
                "title": "Start",
                "data": {}
            },
            {
                "id": "failing-agent",
                "type": "agent",
                "title": "Failing Step",
                "data": {
                    "promptTemplate": "Execution FAIL_HERE please",
                    "agentId": agent_id
                }
            },
            {
                "id": "downstream-agent",
                "type": "agent",
                "title": "Downstream Step",
                "data": {
                    "promptTemplate": "Should be skipped: {{failing-agent.reply}}",
                    "agentId": agent_id
                }
            }
        ],
        "edges": [
            {
                "id": "e1",
                "source": "start-node",
                "target": "failing-agent"
            },
            {
                "id": "e2",
                "source": "failing-agent",
                "target": "downstream-agent"
            }
        ]
    });

    let workflow_id = cluster.create_canvas_workflow(&agent_id, "FaultyWorkflow", canvas_graph);

    let run_res = cluster
        .run_workflow(&workflow_id, json!({}))
        .expect("run workflow handles failures gracefully");

    assert_eq!(
        run_res["log"]["status"], "FAILED",
        "Overall status should be FAILED"
    );

    let node_runs = run_res["log"]["nodeRuns"]
        .as_array()
        .expect("nodeRuns array");
    assert_eq!(node_runs.len(), 3);

    // start-node SUCCEEDED
    assert_eq!(node_runs[0]["nodeId"], "start-node");
    assert_eq!(node_runs[0]["status"], "SUCCEEDED");

    // failing-agent FAILED
    assert_eq!(node_runs[1]["nodeId"], "failing-agent");
    assert_eq!(node_runs[1]["status"], "FAILED");
    assert!(node_runs[1]["error"].is_string());

    // downstream-agent SKIPPED
    assert_eq!(node_runs[2]["nodeId"], "downstream-agent");
    assert_eq!(node_runs[2]["status"], "SKIPPED");
    assert_eq!(
        node_runs[2]["error"].as_str(),
        Some("skipped due to predecessor failure")
    );
}
