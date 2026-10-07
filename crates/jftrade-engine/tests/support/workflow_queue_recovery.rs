use super::*;
use jftrade_engine::product_workflow_scheduler::WorkflowScheduler;
use std::time::Duration;

fn queued_invocation(cluster: &EngineTestCluster, workflow: &str, id: &str) -> String {
    let created = cluster
        .port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::CreateWorkflowTrigger,
            identifiers: BTreeMap::from([("workflowId".to_owned(), workflow.to_owned())]),
            body: json!({"type": "manual", "status": "ENABLED"}),
            webhook_secret: None,
        })
        .unwrap();
    let trigger_id = created["trigger"]["id"].as_str().unwrap().to_owned();
    let trigger = cluster
        .port
        .store
        .get_workflow_trigger(&trigger_id)
        .unwrap()
        .unwrap();
    let payload: Value = serde_json::from_str(&trigger.payload_json).unwrap();
    assert!(
        cluster
            .port
            .store
            .enqueue_workflow_trigger_invocations(
                &trigger,
                &payload,
                "",
                &[(id.to_owned(), json!({"inputs": {"symbol": "US.AAPL"}}))],
            )
            .unwrap()
    );
    let queued = cluster
        .port
        .store
        .get_workflow_trigger_log(id)
        .unwrap()
        .unwrap();
    assert_eq!(queued.id, id);
    assert_eq!(queued.status, "QUEUED");
    trigger_id
}

fn single_node(agent: &str, prompt: &str) -> Value {
    json!({"nodes": [{"id": "start", "type": "start", "data": {}},
        {"id": "agent", "type": "agent",
        "data": {"agentId": agent, "promptTemplate": prompt}}],
        "edges": [{"id": "start-agent", "source": "start", "target": "agent"}]})
}

async fn finish_background(cluster: &EngineTestCluster, id: &str, status: &str) -> Value {
    let scheduler = WorkflowScheduler::start(
        cluster.port.store.clone(),
        cluster.port.clone(),
        None,
        Duration::from_secs(3600),
    );
    let result = tokio::time::timeout(Duration::from_secs(2), async {
        loop {
            let row = cluster
                .port
                .store
                .get_workflow_trigger_log(id)
                .unwrap()
                .unwrap();
            if row.status == status {
                break serde_json::from_str::<Value>(&row.payload_json).unwrap();
            }
            tokio::time::sleep(Duration::from_millis(5)).await;
        }
    })
    .await;
    scheduler.stop();
    assert!(scheduler.join_shutdown(Duration::from_secs(2)).await);
    result.unwrap_or_else(|error| {
        panic!(
            "queued invocation must reach {status}: {error:?}; current {:?}",
            cluster.port.store.get_workflow_trigger_log(id).unwrap()
        )
    });
    // Terminal checkpoints can precede the final log projection; join before reading it.
    let row = cluster
        .port
        .store
        .get_workflow_trigger_log(id)
        .unwrap()
        .unwrap();
    assert_eq!(row.status, status);
    serde_json::from_str(&row.payload_json).unwrap()
}

// Parity: go:452dea11:internal/assistant/workflow_async_tools_test.go:12 TestStartWorkflowQueuesAndCompletesInBackground
// Parity: go:452dea11:internal/assistant/workflows_resource_recovery_test.go:52 TestWorkflowAsyncTriggerAndBackgroundRecovery
#[tokio::test]
async fn queued_canvas_invocation_completes_with_durable_run_session_and_result() {
    let cluster = EngineTestCluster::new();
    let agent = cluster.create_agent("async-canvas");
    let workflow = cluster.create_canvas_workflow(
        &agent,
        "async-canvas",
        single_node(&agent, "run {{input.symbol}}"),
    );
    let id = "workflow-async-canvas";
    let trigger = queued_invocation(&cluster, &workflow, id);
    let finished = finish_background(&cluster, id, "SUCCEEDED").await;
    assert_eq!(finished["id"], id);
    assert_eq!(finished["triggerId"], trigger);
    assert!(!finished["runId"].as_str().unwrap().is_empty());
    assert!(!finished["sessionId"].as_str().unwrap().is_empty());
    assert!(!finished["result"].is_null());
    assert!(!finished["finishedAt"].as_str().unwrap().is_empty());
    assert_eq!(
        cluster
            .mock_chat
            .recorded_messages
            .lock()
            .unwrap()
            .as_slice(),
        ["run US.AAPL"]
    );
    let fetched = cluster
        .port
        .store
        .get_workflow_trigger_log(id)
        .unwrap()
        .unwrap();
    assert_eq!(fetched.status, "SUCCEEDED");
    assert_eq!(
        serde_json::from_str::<Value>(&fetched.payload_json).unwrap(),
        finished
    );
}

// Parity: go:452dea11:internal/assistant/workflow_async_tools_test.go:86 TestStartWorkflowBackgroundFailureTerminatesLog
#[tokio::test]
async fn queued_model_failure_persists_finished_log_and_legacy_prompt_still_runs() {
    let cluster = EngineTestCluster::new();
    let agent = cluster.create_agent("async-failure");
    *cluster.mock_chat.fail_on_keyword.lock().unwrap() = Some("injected failure".to_owned());
    let workflow = cluster.create_canvas_workflow(
        &agent,
        "async-failure",
        single_node(&agent, "injected failure"),
    );
    queued_invocation(&cluster, &workflow, "workflow-async-failure");
    let failed = finish_background(&cluster, "workflow-async-failure", "FAILED").await;
    assert_eq!(failed["id"], "workflow-async-failure");
    assert!(
        failed["error"]
            .as_str()
            .unwrap()
            .contains("Simulated node failure on keyword: injected failure"),
        "{failed}"
    );
    assert!(!failed["finishedAt"].as_str().unwrap().is_empty());
    // The production legacy single-prompt path intentionally accepts absent canvasGraph.
    let legacy = cluster.create_canvas_workflow(&agent, "legacy-prompt", Value::Null);
    queued_invocation(&cluster, &legacy, "workflow-legacy-prompt");
    assert_eq!(
        finish_background(&cluster, "workflow-legacy-prompt", "SUCCEEDED").await["status"],
        "SUCCEEDED"
    );
}

// Parity: go:452dea11:internal/assistant/workflows_resource_recovery_test.go:105 TestWorkflowBackgroundPersistsFailureAfterRunningTransitionWriteFails
#[tokio::test]
async fn queued_running_write_failure_is_recovered_as_durable_failed_with_original_identity() {
    let cluster = EngineTestCluster::new();
    let agent = cluster.create_agent("running-write-failure");
    let workflow = cluster.create_canvas_workflow(
        &agent,
        "running-write-failure",
        single_node(&agent, "review {{input.symbol}}"),
    );
    let id = "workflow-running-write-failure";
    queued_invocation(&cluster, &workflow, id);
    let connection = Connection::open(&cluster.adk_path).unwrap();
    connection.execute_batch("CREATE TRIGGER reject_running_transition BEFORE UPDATE ON adk_workflow_trigger_logs
        WHEN NEW.status = 'RUNNING' BEGIN SELECT RAISE(ABORT, 'injected RUNNING transition write failure'); END;").unwrap();
    let failed = finish_background(&cluster, id, "FAILED").await;
    assert_eq!(failed["id"], id);
    assert!(
        failed["error"]
            .as_str()
            .unwrap()
            .contains("injected RUNNING transition write failure"),
        "{failed}"
    );
    assert!(!failed["finishedAt"].as_str().unwrap().is_empty());
    assert!(
        cluster
            .mock_chat
            .recorded_messages
            .lock()
            .unwrap()
            .is_empty()
    );
    let logs = cluster.port.store.list_workflow_trigger_logs().unwrap();
    assert_eq!(logs.len(), 1);
    assert_eq!(logs[0].id, id);
    assert_eq!(logs[0].status, "FAILED");
}
