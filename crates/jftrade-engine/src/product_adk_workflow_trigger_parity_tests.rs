//! Durable workflow resources and original trigger configuration boundaries.
use super::*;

fn result(
    port: &ProductionAdkPort,
    operation: AdkMutationOperation,
    identifiers: &[(&str, &str)],
    body: Value,
) -> Result<Value, AdkMutationPortError> {
    port.mutate(&AdkMutationInput {
        operation,
        identifiers: identifiers
            .iter()
            .map(|(k, v)| (k.to_string(), v.to_string()))
            .collect(),
        body,
        webhook_secret: None,
    })
}

fn workflow(port: &ProductionAdkPort) -> (String, String) {
    let agent = mutate(
        port,
        AdkMutationOperation::CreateAgent,
        &[],
        json!({"name":"workflow-resource"}),
    );
    let agent_id = agent["id"].as_str().expect("agent id").to_owned();
    let workflow = mutate(
        port,
        AdkMutationOperation::CreateWorkflow,
        &[],
        json!({"name":"workflow-resource","agentId":agent_id,"status":"enabled","promptTemplate":"run workflow"}),
    );
    (
        agent_id,
        workflow["id"].as_str().expect("workflow id").to_owned(),
    )
}

// Parity: go:452dea11:internal/assistant/workflow_crud_test.go:14 TestWorkflowResourceCrudPaginationAndLogs
#[test]
fn workflow_original_crud_pages_failed_logs_and_soft_deletes_resources() {
    let (port, _directory) = agent_validation_port();
    let (agent_id, id) = workflow(&port);
    let updated = mutate(
        &port,
        AdkMutationOperation::UpdateWorkflow,
        &[("workflowId", &id)],
        json!({"name":" Updated Workflow ","status":"disabled","agentId":agent_id,
            "workMode":"loop","permissionMode":"all","promptTemplate":" run updated ",
            "defaultInputs":{"symbol":"US.AAPL"},"tags":[" daily ","daily","","risk"]}),
    );
    assert_eq!(updated["name"], "Updated Workflow");
    assert_eq!(updated["status"], "DISABLED");
    assert_eq!(updated["workMode"], "loop");
    assert_eq!(updated["tags"], json!(["daily", "risk"]));
    let page = read_json(
        &port,
        "/api/v1/adk/workflows",
        "status=DISABLED&limit=200&offset=-10",
    );
    assert_eq!(page["page"]["limit"], 100);
    assert_eq!(page["page"]["offset"], 0);
    assert!(!page["workflows"].as_array().expect("workflows").is_empty());
    let trigger = mutate(
        &port,
        AdkMutationOperation::CreateWorkflowTrigger,
        &[("workflowId", &id)],
        json!({"id":"workflow-resource-trigger","type":"","title":"","status":"error","config":{"custom":true}}),
    );
    let trigger_id = trigger["trigger"]["id"].as_str().expect("trigger id");
    assert_eq!(trigger["trigger"]["type"], "manual");
    assert_eq!(trigger["trigger"]["title"], "手动触发");
    assert_eq!(trigger["trigger"]["status"], "ERROR");
    let triggers = read_json(&port, &format!("/api/v1/adk/workflows/{id}/triggers"), "");
    assert_eq!(triggers["triggers"].as_array().expect("triggers").len(), 1);
    assert!(triggers["triggers"][0].get("secretHash").is_none());
    let saved = port
        .store
        .create_workflow_trigger_log(
            "original-failed-log",
            &id,
            trigger_id,
            "manual",
            "FAILED",
            "",
            r#"{"id":"original-failed-log","status":"FAILED","error":"unit failure"}"#,
        )
        .expect("failed log");
    let logs = read_json(
        &port,
        "/api/v1/adk/workflow-trigger-logs",
        &format!("workflowId={id}&triggerId={trigger_id}&status=FAILED&limit=0&offset=-1"),
    );
    assert_eq!(logs["page"]["limit"], 20);
    assert_eq!(logs["page"]["offset"], 0);
    assert_eq!(logs["logs"].as_array().expect("logs").len(), 1);
    assert_eq!(logs["logs"][0]["id"], saved.id);
    let deleted = mutate(
        &port,
        AdkMutationOperation::DeleteWorkflowTrigger,
        &[("workflowId", &id), ("triggerId", trigger_id)],
        json!({}),
    );
    assert_eq!(deleted["trigger"]["status"], "DISABLED");
    assert!(
        !deleted["trigger"]["deletedAt"]
            .as_str()
            .expect("deletedAt")
            .is_empty()
    );
    assert!(
        result(
            &port,
            AdkMutationOperation::DeleteWorkflowTrigger,
            &[("workflowId", &id), ("triggerId", trigger_id)],
            json!({})
        )
        .is_err()
    );
    let deleted = mutate(
        &port,
        AdkMutationOperation::DeleteWorkflow,
        &[("workflowId", &id)],
        json!({}),
    );
    assert_eq!(deleted["workflow"]["status"], "DISABLED");
    assert!(
        !deleted["workflow"]["deletedAt"]
            .as_str()
            .expect("deletedAt")
            .is_empty()
    );
    assert!(
        port.read(&format!("/api/v1/adk/workflows/{id}"), "")
            .is_err()
    );
    assert_eq!(
        port.store
            .get_workflow(&id)
            .expect("stored deleted workflow")
            .expect("retained row")
            .status,
        "DISABLED"
    );
    assert_eq!(
        port.store
            .get_workflow_trigger(trigger_id)
            .expect("stored deleted trigger")
            .expect("retained row")
            .status,
        "DISABLED"
    );
}

// Parity: go:452dea11:internal/assistant/workflow/rules_test.go:135 TestRuleHelpersAndValidationEdges
#[test]
fn workflow_trigger_original_validation_errors_preserve_their_field_classification() {
    let (port, _directory) = agent_validation_port();
    let (_, id) = workflow(&port);
    let missing = result(
        &port,
        AdkMutationOperation::CreateWorkflowTrigger,
        &[],
        json!({"type":"manual"}),
    )
    .expect_err("missing workflowId");
    assert!(missing.to_string().contains("workflowId"), "{missing}");
    for (body, expected) in [
        (json!({"type":"schedule","config":{}}), "cron"),
        (
            json!({"type":"market_threshold","config":{"value":1}}),
            "instrumentIds",
        ),
        (
            json!({"type":"market_threshold","config":{"instrumentIds":["US.AAPL"]}}),
            "numeric value",
        ),
        (
            json!({"type":"market_threshold","config":{"instrumentIds":[" ",null],"value":100}}),
            "instrumentIds",
        ),
        (
            json!({"type":"market_threshold","config":{"instrumentIds":["US.AAPL"],"value":"bad"}}),
            "numeric value",
        ),
    ] {
        let before = port
            .store
            .list_workflow_triggers(&id)
            .expect("before triggers");
        let error = result(
            &port,
            AdkMutationOperation::CreateWorkflowTrigger,
            &[("workflowId", &id)],
            body,
        )
        .expect_err("rejected config");
        assert!(error.to_string().contains(expected), "{error}");
        assert_eq!(
            port.store
                .list_workflow_triggers(&id)
                .expect("after triggers"),
            before
        );
    }
}

// Parity: go:452dea11:internal/assistant/workflow/rules_test.go:207 TestRuleFallbacksMismatchesAndValidVariants
#[test]
fn workflow_trigger_original_string_instrument_config_reaches_the_durable_owner() {
    let (port, _directory) = agent_validation_port();
    let (_, id) = workflow(&port);
    for body in [
        json!({"type":"schedule","config":{"cron":"0 8 * * *"}}),
        json!({"type":"manual"}),
        json!({"type":"webhook"}),
        json!({"type":"event"}),
        json!({"type":"market_threshold","config":{"instrumentIds":"US.AAPL","value":100}}),
        json!({"type":"market_threshold","config":{"instrumentIds":" US.AAPL,700,US.AAPL ","value":"100"}}),
    ] {
        let created = result(
            &port,
            AdkMutationOperation::CreateWorkflowTrigger,
            &[("workflowId", &id)],
            body.clone(),
        )
        .expect("valid original trigger config");
        assert_eq!(created["trigger"]["type"], body["type"]);
        let stored = port
            .store
            .get_workflow_trigger(created["trigger"]["id"].as_str().expect("id"))
            .expect("stored trigger")
            .expect("row");
        assert_eq!(stored.trigger_type, body["type"].as_str().expect("type"));
    }
    let fallback = mutate(
        &port,
        AdkMutationOperation::CreateWorkflowTrigger,
        &[("workflowId", &id)],
        json!({"type":"unsupported"}),
    );
    assert_eq!(
        fallback["trigger"]["type"], "manual",
        "production normalization differs from direct ValidateTrigger rejection"
    );
}

// Parity: go:452dea11:internal/assistant/workflows_extended_test.go:14 TestWorkflowTriggerValidationAndBoundaryHelpers
#[test]
fn workflow_trigger_original_invalid_schedule_and_market_configs_do_not_write_rows() {
    let (port, _directory) = agent_validation_port();
    let (_, id) = workflow(&port);
    let missing = result(
        &port,
        AdkMutationOperation::CreateWorkflowTrigger,
        &[],
        json!({"type":"manual"}),
    )
    .expect_err("missing workflowId");
    assert!(missing.to_string().contains("workflowId"), "{missing}");
    for (body, expected) in [
        (json!({"type":"schedule","config":{}}), "cron"),
        (
            json!({"type":"schedule","config":{"cron":"0 0 8 * * 1"}}),
            "5 fields",
        ),
        (
            json!({"type":"schedule","config":{"cron":"0 8 * * 1-5","timezone":"Mars/Base"}}),
            "timezone",
        ),
        (
            json!({"type":"market_threshold","config":{"value":100}}),
            "instrumentIds",
        ),
    ] {
        let before = port.store.list_workflow_triggers(&id).expect("before");
        let error = result(
            &port,
            AdkMutationOperation::CreateWorkflowTrigger,
            &[("workflowId", &id)],
            body,
        )
        .expect_err("invalid config");
        assert!(error.to_string().contains(expected), "{error}");
        assert_eq!(
            port.store.list_workflow_triggers(&id).expect("after"),
            before
        );
    }
}
