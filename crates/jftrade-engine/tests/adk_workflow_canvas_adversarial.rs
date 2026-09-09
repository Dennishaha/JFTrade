#![forbid(unsafe_code)]

use std::collections::BTreeMap;
use std::fs::File;
use std::path::{Path, PathBuf};
use std::str::FromStr;
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
struct MockAdversarialChatRuntime {
    recorded_messages: Arc<Mutex<Vec<String>>>,
    fail_on_keyword: Arc<Mutex<Option<String>>>,
    custom_reply_map: Arc<Mutex<BTreeMap<String, String>>>,
}

impl MockAdversarialChatRuntime {
    fn new() -> Self {
        Self::default()
    }
}

impl AdkChatStreamPort for MockAdversarialChatRuntime {
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

        if let Some(ref keyword) = *self.fail_on_keyword.lock().unwrap()
            && message.contains(keyword)
        {
            return Err(AdkChatPortError::Failed {
                status: 500,
                code: "ADVERSARIAL_SIMULATED_FAILURE".to_owned(),
                message: format!("Simulated node failure on keyword: {keyword}"),
            });
        }

        static COUNTER: AtomicU64 = AtomicU64::new(100);
        let count = COUNTER.fetch_add(1, Ordering::Relaxed);

        let custom_reply = self
            .custom_reply_map
            .lock()
            .unwrap()
            .iter()
            .find_map(|(k, v)| {
                if message.contains(k) {
                    Some(v.clone())
                } else {
                    None
                }
            });

        let reply = custom_reply.unwrap_or_else(|| format!("Processed: {message}"));

        Ok(AdkChatPortOutput::Json(json!({
            "run": {
                "id": format!("run-adversarial-{count}"),
                "status": "SUCCEEDED",
                "outputSummary": reply,
            },
            "reply": reply,
            "session": {
                "id": format!("session-adversarial-{count}"),
            }
        })))
    }
}

fn initialize_test_db(path: &Path, component: &str) {
    File::create(path).expect("create database file");
    let connection = Connection::open(path).expect("open sqlite");
    initialize_current(&connection, component).expect("initialize schema");
}

struct AdversarialTestCluster {
    _dir: tempfile::TempDir,
    port: Arc<ProductionAdkPort>,
    mock_chat: Arc<MockAdversarialChatRuntime>,
    adk_path: PathBuf,
}

impl AdversarialTestCluster {
    fn new() -> Self {
        let dir = tempdir().expect("tempdir");
        let adk_path = dir.path().join("adk.db");
        let session_path = dir.path().join("adk-session.db");
        let artifact_path = dir.path().join("adk-artifact.db");
        let settings_path = dir.path().join("settings.json");

        initialize_test_db(&adk_path, "adk");
        initialize_test_db(&session_path, "adk-session");
        initialize_test_db(&artifact_path, "adk-artifact");

        let store = Arc::new(AdkStore::open(&adk_path).expect("open adk store"));
        let session_store =
            Arc::new(AdkSessionStore::open(&session_path).expect("open session store"));
        let artifact_store =
            Arc::new(AdkArtifactStore::open(&artifact_path).expect("open artifact store"));

        let mut port =
            ProductionAdkPort::new_for_test(store, session_store, artifact_store, settings_path);
        let mock_chat = Arc::new(MockAdversarialChatRuntime::new());
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
                "instruction": "Adversarial test agent",
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
                "promptTemplate": "canvas template",
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
fn test_context_interpolation_multi_node_pipeline_and_diamond_join() {
    let cluster = AdversarialTestCluster::new();
    let agent_id = cluster.create_agent("PipelineTrader");

    // Canvas DAG:
    // start-root -> step-a
    // step-a -> step-b
    // step-a -> step-c
    // step-b -> step-c
    // step-c -> monitor-end
    let canvas_graph = json!({
        "nodes": [
            {
                "id": "start-root",
                "type": "start",
                "title": "Start Root",
                "data": {}
            },
            {
                "id": "step-a",
                "type": "agent",
                "title": "Step A Alpha",
                "data": {
                    "promptTemplate": "Alpha symbol={{input.symbol}}",
                    "agentId": agent_id
                }
            },
            {
                "id": "step-b",
                "type": "agent",
                "title": "Step B Beta",
                "data": {
                    "promptTemplate": "Beta upstream={{step-a.reply}} and mult={{input.multiplier}}",
                    "agentId": agent_id
                }
            },
            {
                "id": "step-c",
                "type": "agent",
                "title": "Step C Gamma",
                "data": {
                    "promptTemplate": "Gamma joined A=[{{step-a.reply}}] B=[{{step-b.reply}}]",
                    "agentId": agent_id
                }
            },
            {
                "id": "monitor-end",
                "type": "monitor",
                "title": "Monitor End",
                "data": {}
            }
        ],
        "edges": [
            { "id": "e1", "source": "start-root", "target": "step-a" },
            { "id": "e2", "source": "step-a", "target": "step-b" },
            { "id": "e3", "source": "step-a", "target": "step-c" },
            { "id": "e4", "source": "step-b", "target": "step-c" },
            { "id": "e5", "source": "step-c", "target": "monitor-end" }
        ]
    });

    let workflow_id = cluster.create_canvas_workflow(&agent_id, "DiamondDAG", canvas_graph);

    let run_res = cluster
        .run_workflow(&workflow_id, json!({ "symbol": "NVDA", "multiplier": "5" }))
        .expect("workflow run must succeed");

    assert_eq!(run_res["log"]["status"], "SUCCEEDED");

    let messages = cluster.mock_chat.recorded_messages.lock().unwrap().clone();
    assert_eq!(messages.len(), 3, "Expected 3 agent node executions");

    // 1. Verify step-a interpolation
    assert_eq!(messages[0], "Alpha symbol=NVDA");

    // 2. Verify step-b interpolation from step-a
    assert_eq!(
        messages[1],
        "Beta upstream=Processed: Alpha symbol=NVDA and mult=5"
    );

    // 3. Verify step-c diamond join interpolation receiving both step-a and step-b replies
    assert_eq!(
        messages[2],
        "Gamma joined A=[Processed: Alpha symbol=NVDA] B=[Processed: Beta upstream=Processed: Alpha symbol=NVDA and mult=5]"
    );
}

#[test]
fn test_context_interpolation_preserves_special_characters_and_json_payloads() {
    let cluster = AdversarialTestCluster::new();
    let agent_id = cluster.create_agent("JsonTrader");

    let complex_reply =
        r#"{"signal":"BUY","qty":150,"tags":["AI","NVDA"],"notes":"target $180 {{escaped}}"}"#;
    cluster
        .mock_chat
        .custom_reply_map
        .lock()
        .unwrap()
        .insert("ExtractJson".to_owned(), complex_reply.to_owned());

    let canvas_graph = json!({
        "nodes": [
            {
                "id": "start-node",
                "type": "start",
                "title": "Start",
                "data": {}
            },
            {
                "id": "extractor",
                "type": "agent",
                "title": "Json Extractor",
                "data": {
                    "promptTemplate": "ExtractJson for {{input.symbol}}",
                    "agentId": agent_id
                }
            },
            {
                "id": "consumer",
                "type": "agent",
                "title": "Json Consumer",
                "data": {
                    "promptTemplate": "Received JSON: {{extractor.reply}} with market {{input.market}}",
                    "agentId": agent_id
                }
            }
        ],
        "edges": [
            { "id": "e1", "source": "start-node", "target": "extractor" },
            { "id": "e2", "source": "extractor", "target": "consumer" }
        ]
    });

    let workflow_id = cluster.create_canvas_workflow(&agent_id, "SpecialCharsDAG", canvas_graph);

    let run_res = cluster
        .run_workflow(
            &workflow_id,
            json!({ "symbol": "NVDA", "market": "NASDAQ" }),
        )
        .expect("run workflow should succeed");

    assert_eq!(run_res["log"]["status"], "SUCCEEDED");

    let recorded = cluster.mock_chat.recorded_messages.lock().unwrap().clone();
    assert_eq!(recorded.len(), 2);
    assert_eq!(recorded[0], "ExtractJson for NVDA");
    assert_eq!(
        recorded[1],
        format!("Received JSON: {complex_reply} with market NASDAQ"),
        "Context interpolation must preserve raw JSON characters, braces, and quotes faithfully"
    );
}

#[test]
fn test_sqlite_persistence_noderuns_strict_openapi_schema_compliance() {
    let cluster = AdversarialTestCluster::new();
    let agent_id = cluster.create_agent("SchemaAgent");

    let canvas_graph = json!({
        "nodes": [
            {
                "id": "start-node",
                "type": "start",
                "title": "Start Node Title",
                "data": {}
            },
            {
                "id": "agent-step",
                "type": "agent",
                "title": "Agent Node Title",
                "data": {
                    "promptTemplate": "Execute agent for {{input.ticker}}",
                    "agentId": agent_id
                }
            },
            {
                "id": "monitor-step",
                "type": "monitor",
                "title": "Monitor Node Title",
                "data": {}
            }
        ],
        "edges": [
            { "id": "e1", "source": "start-node", "target": "agent-step" },
            { "id": "e2", "source": "agent-step", "target": "monitor-step" }
        ]
    });

    let workflow_id = cluster.create_canvas_workflow(&agent_id, "SchemaCompliance", canvas_graph);

    let run_res = cluster
        .run_workflow(&workflow_id, json!({ "ticker": "AAPL" }))
        .expect("run workflow must succeed");

    assert_eq!(run_res["log"]["status"], "SUCCEEDED");

    // Connect to SQLite directly and query adk_workflow_trigger_logs
    let conn = Connection::open(&cluster.adk_path).expect("open adk sqlite");
    let mut stmt = conn
        .prepare("SELECT id, workflow_id, trigger_type, status, payload_json FROM adk_workflow_trigger_logs WHERE workflow_id = ?1")
        .expect("prepare select");
    let mut rows = stmt.query([&workflow_id]).expect("query logs");
    let row = rows
        .next()
        .expect("next row")
        .expect("must have trigger log row");

    let log_id: String = row.get(0).expect("id col");
    let db_workflow_id: String = row.get(1).expect("workflow_id col");
    let trigger_type: String = row.get(2).expect("trigger_type col");
    let db_status: String = row.get(3).expect("status col");
    let payload_str: String = row.get(4).expect("payload_json col");

    assert!(!log_id.is_empty());
    assert_eq!(db_workflow_id, workflow_id);
    assert_eq!(trigger_type, "manual");
    assert_eq!(db_status, "SUCCEEDED");

    let payload: Value = serde_json::from_str(&payload_str).expect("valid json payload_json");
    let node_runs = payload["nodeRuns"]
        .as_array()
        .expect("nodeRuns is a JSON array");
    assert_eq!(node_runs.len(), 3, "Expected 3 node runs in payload_json");

    let valid_types = ["trigger", "start", "agent", "monitor"];
    let valid_statuses = [
        "SUCCEEDED",
        "FAILED",
        "SKIPPED",
        "PENDING_APPROVAL",
        "RUNNING",
        "CANCELLED",
    ];

    for nr in node_runs {
        // Required OpenAPI fields: nodeId, nodeType, status
        let node_id = nr["nodeId"]
            .as_str()
            .expect("nodeId must be non-empty string");
        assert!(!node_id.is_empty(), "nodeId cannot be empty");

        let node_type = nr["nodeType"].as_str().expect("nodeType must be string");
        assert!(
            valid_types.contains(&node_type),
            "nodeType {node_type} must be a known type"
        );

        let status = nr["status"].as_str().expect("status must be string");
        assert!(
            valid_statuses.contains(&status),
            "status {status} must be a valid status"
        );

        // startedAt and finishedAt must be valid RFC3339 timestamps
        let started_at_str = nr["startedAt"].as_str().expect("startedAt must be string");
        let finished_at_str = nr["finishedAt"]
            .as_str()
            .expect("finishedAt must be string");
        let started_at =
            jiff::Timestamp::from_str(started_at_str).expect("startedAt is valid RFC3339");
        let finished_at =
            jiff::Timestamp::from_str(finished_at_str).expect("finishedAt is valid RFC3339");
        assert!(started_at <= finished_at, "startedAt must be <= finishedAt");

        // inputs and outputs must be JSON objects for successful runs
        assert!(nr["inputs"].is_object(), "inputs must be a JSON object");
        assert!(nr["outputs"].is_object(), "outputs must be a JSON object");
    }

    // Specific node assertions
    // Node 0: start-node
    assert_eq!(node_runs[0]["nodeId"], "start-node");
    assert_eq!(node_runs[0]["nodeType"], "start");
    assert_eq!(node_runs[0]["outputs"]["inputs"]["ticker"], "AAPL");

    // Node 1: agent-step
    assert_eq!(node_runs[1]["nodeId"], "agent-step");
    assert_eq!(node_runs[1]["nodeType"], "agent");
    assert_eq!(node_runs[1]["inputs"]["message"], "Execute agent for AAPL");
    assert_eq!(
        node_runs[1]["outputs"]["reply"],
        "Processed: Execute agent for AAPL"
    );
    assert_eq!(node_runs[1]["outputs"]["status"], "SUCCEEDED");
    assert!(node_runs[1]["outputs"]["runId"].is_string());
    assert!(node_runs[1]["outputs"]["sessionId"].is_string());

    // Node 2: monitor-step
    assert_eq!(node_runs[2]["nodeId"], "monitor-step");
    assert_eq!(node_runs[2]["nodeType"], "monitor");
    assert_eq!(node_runs[2]["outputs"]["status"], "SUCCEEDED");
    assert_eq!(
        node_runs[2]["outputs"]["lastReply"],
        "Processed: Execute agent for AAPL"
    );
    assert!(node_runs[2]["outputs"]["predecessors"]["agent-step"].is_object());
}

#[test]
fn test_cycle_detection_and_invalid_graph_permutations_reject_with_clean_400() {
    let cluster = AdversarialTestCluster::new();
    let agent_id = cluster.create_agent("CycleAdversary");

    // Permutation 1: 3-cycle: A -> B -> C -> A (with start -> A)
    let cycle_3_nodes = json!({
        "nodes": [
            { "id": "s", "type": "start", "data": {} },
            { "id": "a", "type": "agent", "data": { "agentId": agent_id } },
            { "id": "b", "type": "agent", "data": { "agentId": agent_id } },
            { "id": "c", "type": "agent", "data": { "agentId": agent_id } }
        ],
        "edges": [
            { "id": "e0", "source": "s", "target": "a" },
            { "id": "e1", "source": "a", "target": "b" },
            { "id": "e2", "source": "b", "target": "c" },
            { "id": "e3", "source": "c", "target": "a" }
        ]
    });
    let wf1 = cluster.create_canvas_workflow(&agent_id, "Cycle3", cycle_3_nodes);
    let err1 = cluster
        .run_workflow(&wf1, json!({}))
        .expect_err("3-cycle must be rejected");
    match err1 {
        AdkMutationPortError::Failed {
            status, message, ..
        } => {
            assert_eq!(status, 400);
            assert!(
                message.contains("workflow canvas contains a cycle"),
                "Got: {message}"
            );
        }
        other => panic!("Expected 400 error, got {other:?}"),
    }

    // Permutation 2: Disjoint cycle: s -> a (valid), plus disconnected x -> y -> x
    let disjoint_cycle = json!({
        "nodes": [
            { "id": "s", "type": "start", "data": {} },
            { "id": "a", "type": "agent", "data": { "agentId": agent_id } },
            { "id": "x", "type": "agent", "data": { "agentId": agent_id } },
            { "id": "y", "type": "agent", "data": { "agentId": agent_id } }
        ],
        "edges": [
            { "id": "e0", "source": "s", "target": "a" },
            { "id": "ex", "source": "x", "target": "y" },
            { "id": "ey", "source": "y", "target": "x" }
        ]
    });
    let wf2 = cluster.create_canvas_workflow(&agent_id, "DisjointCycle", disjoint_cycle);
    let err2 = cluster
        .run_workflow(&wf2, json!({}))
        .expect_err("disjoint cycle must be rejected");
    match err2 {
        AdkMutationPortError::Failed {
            status, message, ..
        } => {
            assert_eq!(status, 400);
            assert!(
                message.contains("workflow canvas contains a cycle"),
                "Got: {message}"
            );
        }
        other => panic!("Expected 400 error, got {other:?}"),
    }

    // Permutation 3: No executable agent nodes: start -> monitor
    let no_agent = json!({
        "nodes": [
            { "id": "s", "type": "start", "data": {} },
            { "id": "m", "type": "monitor", "data": {} }
        ],
        "edges": [
            { "id": "e1", "source": "s", "target": "m" }
        ]
    });
    let wf3 = cluster.create_canvas_workflow(&agent_id, "NoAgent", no_agent);
    let err3 = cluster
        .run_workflow(&wf3, json!({}))
        .expect_err("no agent must be rejected");
    match err3 {
        AdkMutationPortError::Failed {
            status, message, ..
        } => {
            assert_eq!(status, 400);
            assert!(
                message.contains("no executable agent nodes"),
                "Got: {message}"
            );
        }
        other => panic!("Expected 400 error, got {other:?}"),
    }

    // Permutation 4: Unreachable agent node: s -> a, orphan agent b
    let unreachable_agent = json!({
        "nodes": [
            { "id": "s", "type": "start", "data": {} },
            { "id": "a", "type": "agent", "data": { "agentId": agent_id } },
            { "id": "b-orphan", "type": "agent", "data": { "agentId": agent_id } }
        ],
        "edges": [
            { "id": "e1", "source": "s", "target": "a" }
        ]
    });
    let wf4 = cluster.create_canvas_workflow(&agent_id, "Unreachable", unreachable_agent);
    let err4 = cluster
        .run_workflow(&wf4, json!({}))
        .expect_err("unreachable agent must be rejected");
    match err4 {
        AdkMutationPortError::Failed {
            status, message, ..
        } => {
            assert_eq!(status, 400);
            assert!(
                message.contains("is not reachable from start or trigger"),
                "Got: {message}"
            );
        }
        other => panic!("Expected 400 error, got {other:?}"),
    }
}

#[test]
fn test_partial_failure_cascade_records_skipped_nodes_in_sqlite() {
    let cluster = AdversarialTestCluster::new();
    let agent_id = cluster.create_agent("FailureAgent");

    *cluster.mock_chat.fail_on_keyword.lock().unwrap() = Some("CRASH_NOW".to_owned());

    // Start -> A (crashes) -> B (should skip) -> Monitor
    let canvas_graph = json!({
        "nodes": [
            { "id": "start-node", "type": "start", "data": {} },
            {
                "id": "node-a",
                "type": "agent",
                "title": "Crashing Node",
                "data": {
                    "promptTemplate": "Execute CRASH_NOW here",
                    "agentId": agent_id
                }
            },
            {
                "id": "node-b",
                "type": "agent",
                "title": "Downstream Node",
                "data": {
                    "promptTemplate": "After A: {{node-a.reply}}",
                    "agentId": agent_id
                }
            },
            { "id": "monitor-node", "type": "monitor", "data": {} }
        ],
        "edges": [
            { "id": "e1", "source": "start-node", "target": "node-a" },
            { "id": "e2", "source": "node-a", "target": "node-b" },
            { "id": "e3", "source": "node-b", "target": "monitor-node" }
        ]
    });

    let workflow_id = cluster.create_canvas_workflow(&agent_id, "CascadeFailure", canvas_graph);
    let run_res = cluster
        .run_workflow(&workflow_id, json!({}))
        .expect("run workflow handles failures");

    assert_eq!(run_res["log"]["status"], "FAILED");

    // Inspect SQLite trigger log payload_json
    let conn = Connection::open(&cluster.adk_path).expect("open adk sqlite");
    let mut stmt = conn
        .prepare("SELECT payload_json FROM adk_workflow_trigger_logs WHERE workflow_id = ?1")
        .expect("prepare");
    let mut rows = stmt.query([&workflow_id]).expect("query");
    let row = rows.next().expect("row").expect("exists");
    let payload_str: String = row.get(0).expect("payload_json");
    let payload: Value = serde_json::from_str(&payload_str).expect("valid json");

    let node_runs = payload["nodeRuns"].as_array().expect("nodeRuns array");
    assert_eq!(node_runs.len(), 4);

    // start-node: SUCCEEDED
    assert_eq!(node_runs[0]["nodeId"], "start-node");
    assert_eq!(node_runs[0]["status"], "SUCCEEDED");

    // node-a: FAILED
    assert_eq!(node_runs[1]["nodeId"], "node-a");
    assert_eq!(node_runs[1]["status"], "FAILED");
    assert!(node_runs[1]["error"].is_string());

    // node-b: SKIPPED
    assert_eq!(node_runs[2]["nodeId"], "node-b");
    assert_eq!(node_runs[2]["status"], "SKIPPED");
    assert_eq!(
        node_runs[2]["error"].as_str(),
        Some("skipped due to predecessor failure")
    );

    // monitor-node: FAILED (inherits overall failure)
    assert_eq!(node_runs[3]["nodeId"], "monitor-node");
    assert_eq!(node_runs[3]["status"], "FAILED");
}
