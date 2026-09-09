#![forbid(unsafe_code)]

use std::collections::{BTreeMap, HashSet};
use std::fs::File;
use std::path::Path;
use std::sync::Arc;
use std::thread;

use jftrade_engine::product::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationOperation, AdkMutationPort,
};
use jftrade_engine::product::product_production_ports::product_production_ports_adk::ProductionAdkPort;
use jftrade_engine::product_id::{
    extract_uuid_from_prefixed_id, generate_prefixed_id, generate_uuid_v4, is_valid_prefixed_id,
    is_valid_uuid_v4,
};
use jftrade_store_sqlite::{
    AdkArtifactStore, AdkSessionStore, AdkStore, CreateAdkRunParams, initialize_current,
};
use rusqlite::Connection;
use serde_json::json;
use tempfile::tempdir;

fn initialize_db(path: &Path, component: &str) {
    File::create(path).expect("create database file");
    let connection = Connection::open(path).expect("open sqlite");
    initialize_current(&connection, component).expect("initialize schema");
}

struct EngineTestCluster {
    _dir: tempfile::TempDir,
    port: Option<Arc<ProductionAdkPort>>,
    adk_path: std::path::PathBuf,
    session_path: std::path::PathBuf,
    artifact_path: std::path::PathBuf,
    settings_path: std::path::PathBuf,
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

        let port = Some(Arc::new(ProductionAdkPort::new_for_test(
            store,
            session_store,
            artifact_store,
            settings_path.clone(),
        )));

        Self {
            _dir: dir,
            port,
            adk_path,
            session_path,
            artifact_path,
            settings_path,
        }
    }

    fn port(&self) -> &ProductionAdkPort {
        self.port.as_ref().expect("port must be active")
    }

    /// Simulates restarting the engine by dropping the previous `ProductionAdkPort`
    /// (releasing WriterLease locks) and opening fresh stores over the persistent SQLite databases.
    fn restart(&mut self) {
        self.port = None;

        let store = Arc::new(AdkStore::open(&self.adk_path).expect("open adk store"));
        let session_store =
            Arc::new(AdkSessionStore::open(&self.session_path).expect("open session store"));
        let artifact_store =
            Arc::new(AdkArtifactStore::open(&self.artifact_path).expect("open artifact store"));

        self.port = Some(Arc::new(ProductionAdkPort::new_for_test(
            store,
            session_store,
            artifact_store,
            self.settings_path.clone(),
        )));
    }

    fn create_agent_named(&self, port: &ProductionAdkPort, name: &str) -> String {
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateAgent,
            identifiers: BTreeMap::new(),
            body: json!({ "name": name }),
            webhook_secret: None,
        };
        let res = port.mutate(&input).expect("create agent");
        res["id"].as_str().expect("agent id").to_owned()
    }

    fn create_agent_auto_id(&self, port: &ProductionAdkPort) -> String {
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateAgent,
            identifiers: BTreeMap::new(),
            body: json!({ "description": "unnamed autonomous agent" }),
            webhook_secret: None,
        };
        let res = port.mutate(&input).expect("create agent");
        res["id"].as_str().expect("agent id").to_owned()
    }

    fn create_session(&self, port: &ProductionAdkPort, agent_id: &str, title: &str) -> String {
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateSession,
            identifiers: BTreeMap::new(),
            body: json!({ "agentId": agent_id, "title": title }),
            webhook_secret: None,
        };
        let res = port.mutate(&input).expect("create session");
        res["id"].as_str().expect("session id").to_owned()
    }

    fn create_workflow(&self, port: &ProductionAdkPort, agent_id: &str, name: &str) -> String {
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateWorkflow,
            identifiers: BTreeMap::new(),
            body: json!({
                "name": name,
                "agentId": agent_id,
                "promptTemplate": "hello {{name}}"
            }),
            webhook_secret: None,
        };
        let res = port.mutate(&input).expect("create workflow");
        res["id"].as_str().expect("workflow id").to_owned()
    }

    fn create_trigger(&self, port: &ProductionAdkPort, workflow_id: &str) -> String {
        let mut identifiers = BTreeMap::new();
        identifiers.insert("workflowId".to_owned(), workflow_id.to_owned());
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateWorkflowTrigger,
            identifiers,
            body: json!({
                "type": "manual",
                "title": "manual run",
                "config": {}
            }),
            webhook_secret: None,
        };
        let res = port.mutate(&input).expect("create trigger");
        res.get("trigger")
            .and_then(|t| t.get("id"))
            .or_else(|| res.get("id"))
            .and_then(|v| v.as_str())
            .expect("trigger id")
            .to_owned()
    }

    fn create_task(&self, port: &ProductionAdkPort, title: &str) -> String {
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateTask,
            identifiers: BTreeMap::new(),
            body: json!({ "title": title }),
            webhook_secret: None,
        };
        let res = port.mutate(&input).expect("create task");
        res["id"].as_str().expect("task id").to_owned()
    }

    fn run_workflow(&self, port: &ProductionAdkPort, workflow_id: &str) {
        let mut identifiers = BTreeMap::new();
        identifiers.insert("workflowId".to_owned(), workflow_id.to_owned());
        let input = AdkMutationInput {
            operation: AdkMutationOperation::RunWorkflow,
            identifiers,
            body: json!({ "name": "auto-runner" }),
            webhook_secret: None,
        };
        // Without chat_runtime, this returns Err(ADK_WORKFLOW_RUNTIME_UNAVAILABLE),
        // but it still durably records the invocation in adk_workflow_trigger_logs.
        let _ = port.mutate(&input);
    }

    fn close_port(&mut self) {
        self.port = None;
    }
}

#[test]
fn test_adk_mutations_generate_collision_proof_uuids_for_all_entities() {
    let cluster = EngineTestCluster::new();
    let port = cluster.port();

    // 1. Agent with auto-generated ID
    let agent_id = cluster.create_agent_auto_id(port);
    assert!(
        is_valid_prefixed_id(&agent_id, "agent"),
        "Agent ID must be valid prefixed UUID v4, got {agent_id}"
    );

    // 2. Session with auto-generated ID
    let session_id = cluster.create_session(port, &agent_id, "trading session");
    assert!(
        is_valid_prefixed_id(&session_id, "session"),
        "Session ID must be valid prefixed UUID v4, got {session_id}"
    );

    // 3. Workflow with auto-generated ID
    let workflow_id = cluster.create_workflow(port, &agent_id, "alpha strategy");
    assert!(
        is_valid_prefixed_id(&workflow_id, "workflow"),
        "Workflow ID must be valid prefixed UUID v4, got {workflow_id}"
    );

    // 4. Trigger with auto-generated ID
    let trigger_id = cluster.create_trigger(port, &workflow_id);
    assert!(
        is_valid_prefixed_id(&trigger_id, "workflow-trigger"),
        "Trigger ID must be valid prefixed UUID v4, got {trigger_id}"
    );

    // 5. Task with auto-generated ID
    let task_id = cluster.create_task(port, "analyze order book");
    assert!(
        is_valid_prefixed_id(&task_id, "task"),
        "Task ID must be valid prefixed UUID v4, got {task_id}"
    );

    // Verify all 5 generated IDs have distinct UUIDs
    let uuids: HashSet<&str> = [
        extract_uuid_from_prefixed_id(&agent_id, "agent").unwrap(),
        extract_uuid_from_prefixed_id(&session_id, "session").unwrap(),
        extract_uuid_from_prefixed_id(&workflow_id, "workflow").unwrap(),
        extract_uuid_from_prefixed_id(&trigger_id, "workflow-trigger").unwrap(),
        extract_uuid_from_prefixed_id(&task_id, "task").unwrap(),
    ]
    .into_iter()
    .collect();
    assert_eq!(uuids.len(), 5, "All entity UUIDs must be distinct");
}

#[test]
fn test_cross_restart_simulation_produces_zero_collision_in_sqlite() {
    let mut cluster = EngineTestCluster::new();
    const RESTARTS: usize = 20;

    let mut session_ids = HashSet::new();
    let mut workflow_ids = HashSet::new();
    let mut trigger_ids = HashSet::new();
    let mut task_ids = HashSet::new();

    // Create a base agent first
    let agent_id = cluster.create_agent_named(cluster.port(), "base-agent");

    for i in 0..RESTARTS {
        // Simulate service restart: drops old port and acquires fresh WriterLease
        cluster.restart();
        let port = cluster.port();

        let s_id = cluster.create_session(port, &agent_id, &format!("session-run-{i}"));
        assert!(
            session_ids.insert(s_id.clone()),
            "Session collision detected on restart {i}: {s_id}"
        );

        let w_id = cluster.create_workflow(port, &agent_id, &format!("workflow-run-{i}"));
        assert!(
            workflow_ids.insert(w_id.clone()),
            "Workflow collision detected on restart {i}: {w_id}"
        );

        let t_id = cluster.create_trigger(port, &w_id);
        assert!(
            trigger_ids.insert(t_id.clone()),
            "Trigger collision detected on restart {i}: {t_id}"
        );

        let task_id = cluster.create_task(port, &format!("task-run-{i}"));
        assert!(
            task_ids.insert(task_id.clone()),
            "Task collision detected on restart {i}: {task_id}"
        );
    }

    assert_eq!(session_ids.len(), RESTARTS);
    assert_eq!(workflow_ids.len(), RESTARTS);
    assert_eq!(trigger_ids.len(), RESTARTS);
    assert_eq!(task_ids.len(), RESTARTS);
}

#[test]
fn test_workflow_invocation_request_and_session_id_entropy() {
    // Tests that invocation IDs and session IDs generated for workflow runs
    // never collide across simulated restarts.
    let mut seen_request_ids = HashSet::new();
    let mut seen_session_ids = HashSet::new();

    for restart in 0..100 {
        let workflow_id = "wf-test-id";
        let invocation_uuid = generate_uuid_v4();
        let request_id = format!("workflow-{workflow_id}-{invocation_uuid}");
        let session_id = format!("workflow-session-{}", generate_uuid_v4());

        assert!(
            seen_request_ids.insert(request_id.clone()),
            "Duplicate workflow invocation request ID on simulated restart {restart}: {request_id}"
        );
        assert!(
            seen_session_ids.insert(session_id.clone()),
            "Duplicate workflow session ID on simulated restart {restart}: {session_id}"
        );
    }
}

#[test]
fn test_prefixed_id_generator_and_rfc4122_spec() {
    let id = generate_prefixed_id("workflow-log");
    assert!(is_valid_prefixed_id(&id, "workflow-log"));
    let uuid = extract_uuid_from_prefixed_id(&id, "workflow-log").unwrap();
    assert!(is_valid_uuid_v4(uuid));

    // Verify 1000 generated UUIDs all conform to RFC 4122 v4
    for _ in 0..1000 {
        let u = generate_uuid_v4();
        assert_eq!(u.len(), 36);
        assert_eq!(u.as_bytes()[14], b'4'); // version 4
        assert!(matches!(u.as_bytes()[19], b'8' | b'9' | b'a' | b'b')); // variant 1
    }
}

#[test]
fn test_cross_restart_all_adk_mutations_persist_cleanly_in_sqlite() {
    let mut cluster = EngineTestCluster::new();
    const RESTARTS: usize = 30;

    let base_agent = cluster.create_agent_named(cluster.port(), "base-coordinator");

    for i in 0..RESTARTS {
        cluster.restart();
        let port = cluster.port();

        // 1. CreateAgent
        let agent_id = cluster.create_agent_auto_id(port);
        assert!(is_valid_prefixed_id(&agent_id, "agent"));

        // 2. CreateSession
        let session_id = cluster.create_session(port, &base_agent, &format!("session-{i}"));
        assert!(is_valid_prefixed_id(&session_id, "session"));

        // 3. CreateWorkflow
        let workflow_id = cluster.create_workflow(port, &base_agent, &format!("workflow-{i}"));
        assert!(is_valid_prefixed_id(&workflow_id, "workflow"));

        // 4. CreateWorkflowTrigger
        let trigger_id = cluster.create_trigger(port, &workflow_id);
        assert!(is_valid_prefixed_id(&trigger_id, "workflow-trigger"));

        // 5. RunWorkflow (records invocation log in adk_workflow_trigger_logs)
        cluster.run_workflow(port, &workflow_id);

        // 6. CreateTask
        let task_id = cluster.create_task(port, &format!("task-{i}"));
        assert!(is_valid_prefixed_id(&task_id, "task"));

        // 7. Insert run directly into adk_runs using the workflow invocation pattern
        let invocation_uuid = generate_uuid_v4();
        let request_id = format!("workflow-{workflow_id}-{invocation_uuid}");
        let run_id = format!("run-{request_id}");
        let run_session_id = format!("workflow-session-{}", generate_uuid_v4());
        port.store
            .create_run(CreateAdkRunParams {
                id: &run_id,
                session_id: &run_session_id,
                agent_id: &base_agent,
                status: "COMPLETED",
                client_request_id: &request_id,
                request_fingerprint: "fp-test",
                payload_json: "{}",
            })
            .expect("insert run into adk_runs");
    }

    // Drop port to release all writer locks before direct inspection
    let adk_db_path = cluster.adk_path.clone();
    cluster.close_port();

    // Query SQLite database directly to verify zero primary key collisions across restarts
    let conn = Connection::open(&adk_db_path).expect("open sqlite for inspection");

    let count_distinct = |table: &str, col: &str| -> (i64, i64) {
        let sql = format!("SELECT count({col}), count(DISTINCT {col}) FROM {table}");
        conn.query_row(&sql, [], |row| Ok((row.get(0)?, row.get(1)?)))
            .expect("query counts")
    };

    // 1. adk_agents: 1 base + 30 restarts = 31
    let (total, distinct) = count_distinct("adk_agents", "id");
    assert_eq!(total, (RESTARTS + 1) as i64);
    assert_eq!(total, distinct, "Zero PK collisions in adk_agents");

    // 2. adk_sessions: 30
    let (total, distinct) = count_distinct("adk_sessions", "id");
    assert_eq!(total, RESTARTS as i64);
    assert_eq!(total, distinct, "Zero PK collisions in adk_sessions");

    // 3. adk_workflows: 30
    let (total, distinct) = count_distinct("adk_workflows", "id");
    assert_eq!(total, RESTARTS as i64);
    assert_eq!(total, distinct, "Zero PK collisions in adk_workflows");

    // 4. adk_workflow_triggers: 30
    let (total, distinct) = count_distinct("adk_workflow_triggers", "id");
    assert_eq!(total, RESTARTS as i64);
    assert_eq!(
        total, distinct,
        "Zero PK collisions in adk_workflow_triggers"
    );

    // 5. adk_workflow_trigger_logs: 30
    let (total, distinct) = count_distinct("adk_workflow_trigger_logs", "id");
    assert_eq!(total, RESTARTS as i64);
    assert_eq!(
        total, distinct,
        "Zero PK collisions in adk_workflow_trigger_logs"
    );

    // 6. adk_tasks: 30
    let (total, distinct) = count_distinct("adk_tasks", "id");
    assert_eq!(total, RESTARTS as i64);
    assert_eq!(total, distinct, "Zero PK collisions in adk_tasks");

    // 7. adk_runs: 30, both id and client_request_id must be strictly unique
    let (total_runs, distinct_runs) = count_distinct("adk_runs", "id");
    assert_eq!(total_runs, RESTARTS as i64);
    assert_eq!(total_runs, distinct_runs, "Zero PK collisions in adk_runs");

    let (total_reqs, distinct_reqs) = count_distinct("adk_runs", "client_request_id");
    assert_eq!(total_reqs, RESTARTS as i64);
    assert_eq!(
        total_reqs, distinct_reqs,
        "Zero collisions in client_request_id unique index"
    );
}

#[test]
fn test_invalid_and_blank_identifier_fallback_behavior() {
    let cluster = EngineTestCluster::new();
    let port = cluster.port();
    let base_agent = cluster.create_agent_named(port, "valid-agent");

    let weird_inputs = ["   ", "@@@###$$$", "中文测试", ""];

    for raw_id in weird_inputs {
        // Agent creation with invalid id
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateAgent,
            identifiers: BTreeMap::new(),
            body: json!({ "id": raw_id, "description": "test" }),
            webhook_secret: None,
        };
        let res = port.mutate(&input).expect("create agent with raw id");
        let agent_id = res["id"].as_str().expect("agent id");
        assert!(
            is_valid_prefixed_id(agent_id, "agent"),
            "Fallback must produce valid prefixed UUID v4: {agent_id}"
        );

        // Workflow creation with invalid id
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateWorkflow,
            identifiers: BTreeMap::new(),
            body: json!({
                "id": raw_id,
                "name": "wf-test",
                "agentId": &base_agent,
                "promptTemplate": "test"
            }),
            webhook_secret: None,
        };
        let res = port.mutate(&input).expect("create workflow with raw id");
        let wf_id = res["id"].as_str().expect("wf id");
        assert!(
            is_valid_prefixed_id(wf_id, "workflow"),
            "Fallback must produce valid prefixed UUID v4: {wf_id}"
        );

        // Trigger creation with invalid id
        let mut identifiers = BTreeMap::new();
        identifiers.insert("workflowId".to_owned(), wf_id.to_owned());
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateWorkflowTrigger,
            identifiers,
            body: json!({ "id": raw_id, "type": "manual" }),
            webhook_secret: None,
        };
        let res = port.mutate(&input).expect("create trigger with raw id");
        let trigger_id = res
            .get("trigger")
            .and_then(|t| t.get("id"))
            .or_else(|| res.get("id"))
            .and_then(|v| v.as_str())
            .expect("trigger id");
        assert!(
            is_valid_prefixed_id(trigger_id, "workflow-trigger"),
            "Fallback must produce valid prefixed UUID v4: {trigger_id}"
        );

        // Task creation with invalid id
        let input = AdkMutationInput {
            operation: AdkMutationOperation::CreateTask,
            identifiers: BTreeMap::new(),
            body: json!({ "id": raw_id, "title": "test task" }),
            webhook_secret: None,
        };
        let res = port.mutate(&input).expect("create task with raw id");
        let task_id = res["id"].as_str().expect("task id");
        assert!(
            is_valid_prefixed_id(task_id, "task"),
            "Fallback must produce valid prefixed UUID v4: {task_id}"
        );
    }
}

#[test]
fn test_concurrent_multi_entity_sqlite_stress() {
    let cluster = Arc::new(EngineTestCluster::new());
    let base_agent = cluster.create_agent_named(cluster.port(), "stress-base-agent");

    const THREADS: usize = 8;
    const ITERS: usize = 10;

    let mut handles = Vec::new();

    for t in 0..THREADS {
        let cluster = Arc::clone(&cluster);
        let base_agent = base_agent.clone();
        handles.push(thread::spawn(move || {
            let port = cluster.port();
            for i in 0..ITERS {
                let s_id =
                    cluster.create_session(port, &base_agent, &format!("stress-session-{t}-{i}"));
                assert!(is_valid_prefixed_id(&s_id, "session"));

                let wf_id =
                    cluster.create_workflow(port, &base_agent, &format!("stress-wf-{t}-{i}"));
                assert!(is_valid_prefixed_id(&wf_id, "workflow"));

                let tr_id = cluster.create_trigger(port, &wf_id);
                assert!(is_valid_prefixed_id(&tr_id, "workflow-trigger"));

                cluster.run_workflow(port, &wf_id);

                let task_id = cluster.create_task(port, &format!("stress-task-{t}-{i}"));
                assert!(is_valid_prefixed_id(&task_id, "task"));
            }
        }));
    }

    for h in handles {
        h.join().expect("thread join");
    }

    // Verify all rows in SQLite
    let conn = Connection::open(&cluster.adk_path).expect("open sqlite");
    let session_count: i64 = conn
        .query_row(
            "SELECT count(DISTINCT id) FROM adk_sessions WHERE id LIKE 'session-%'",
            [],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(session_count, (THREADS * ITERS) as i64);

    let wf_count: i64 = conn
        .query_row(
            "SELECT count(DISTINCT id) FROM adk_workflows WHERE id LIKE 'workflow-%'",
            [],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(wf_count, (THREADS * ITERS) as i64);

    let tr_count: i64 = conn
        .query_row(
            "SELECT count(DISTINCT id) FROM adk_workflow_triggers WHERE id LIKE 'workflow-trigger-%'",
            [],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(tr_count, (THREADS * ITERS) as i64);

    let log_count: i64 = conn
        .query_row(
            "SELECT count(DISTINCT id) FROM adk_workflow_trigger_logs WHERE id LIKE 'workflow-log-%'",
            [],
            |r| r.get(0),
        )
        .unwrap();
    assert_eq!(log_count, (THREADS * ITERS) as i64);
}
