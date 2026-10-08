use super::*;

// Parity: go:452dea11:internal/assistant/workflows_extended_test.go:208 TestWorkflowSchedulerTickAndMarketPollingStablePaths
#[tokio::test]
async fn disabled_due_schedule_advances_without_recording_an_execution() {
    assert_disabled_due_schedule_preserves_execution_time("").await;
}

// Parity: go:452dea11:internal/assistant/workflows_extended_test.go:208 TestWorkflowSchedulerTickAndMarketPollingStablePaths
#[tokio::test]
async fn disabled_due_schedule_preserves_its_previous_execution_time() {
    assert_disabled_due_schedule_preserves_execution_time("2025-12-30T00:00:00Z").await;
}

async fn assert_disabled_due_schedule_preserves_execution_time(last_run_at: &str) {
    let cluster = TestCluster::new();
    let agent = cluster.create_agent("disabled schedule agent");
    let workflow = cluster.create_workflow(&agent, "workflow-scheduler-disabled");
    let trigger = cluster.create_trigger(
        &workflow,
        json!({
            "type":"schedule", "status":"ENABLED", "title":"Due schedule",
            "config":{"cron":"0 8 * * 1-5", "timezone":"Asia/Shanghai"}
        }),
    );
    let mut identifiers = BTreeMap::new();
    identifiers.insert("workflowId".to_owned(), workflow.clone());
    cluster
        .port
        .mutate(&AdkMutationInput {
            operation: AdkMutationOperation::UpdateWorkflow,
            identifiers,
            body: json!({"status":"DISABLED"}),
            webhook_secret: None,
        })
        .unwrap();
    cluster
        .store
        .update_workflow_trigger_run_state(&trigger, last_run_at, "2026-01-01T00:00:00Z", None)
        .unwrap();
    let scheduler =
        WorkflowScheduler::new_for_test(cluster.store.clone(), cluster.port.clone(), None);
    let now = OffsetDateTime::parse("2026-07-01T00:00:05Z", &Rfc3339).unwrap();
    let tick = scheduler.tick(now).await;
    assert_eq!(tick.schedule_triggers_evaluated, 1);
    assert_eq!(tick.schedule_triggers_fired, 0);
    assert!(tick.errors.is_empty(), "{:?}", tick.errors);
    assert!(
        cluster
            .store
            .list_workflow_trigger_logs()
            .unwrap()
            .is_empty()
    );
    let row = cluster
        .store
        .get_workflow_trigger(&trigger)
        .unwrap()
        .unwrap();
    let payload: Value = serde_json::from_str(&row.payload_json).unwrap();
    assert_eq!(
        payload["lastRunAt"].as_str().unwrap_or_default(),
        last_run_at
    );
    assert_eq!(row.next_run_at, "2026-07-02T00:00:00Z");
    assert_eq!(payload["nextRunAt"], row.next_run_at);
    assert!(scheduler.join_invocations(Duration::from_secs(1)));
}
