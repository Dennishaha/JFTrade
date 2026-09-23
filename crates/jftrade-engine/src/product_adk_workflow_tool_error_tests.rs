//! Behavior tests for the Go `internal/assistant/assembly/
//! workflow_tools_error_boundaries_test.go` rows.
//!
//! Go's workflow tools answer with whatever the workflow manager returned, and
//! they refuse a request whose `canvasGraph` cannot be decoded before anything
//! is written. Rust has no model-tool registry: the console routes reach the
//! manager through the ADK read and mutation ports, so the equivalent
//! assertions are that a typed manager failure crosses both ports unchanged
//! and that a structurally broken canvas payload is answered `400 BAD_REQUEST`
//! instead of being stored for the canvas runtime to choke on later.

use std::collections::BTreeMap;

use serde_json::Value;

use crate::product::product_adk_mutation_port::{
    AdkMutationPort, AdkMutationPortError, AdkMutationRequest, dispatch_adk_mutation,
};
use crate::product::{
    AdkReadSnapshot, AdkReadSnapshotError, AdkReadSnapshotPort, dispatch_adk_read,
};

use super::*;

const MANAGER_FAILURE_CODE: &str = "WORKFLOW_MANAGER_FAILED";
const MANAGER_FAILURE_MESSAGE: &str = "workflow manager failed";
const READ_FAILURE_STATUS: u16 = 502;
const MUTATION_FAILURE_STATUS: u16 = 422;
const READ_FAILURE_RETRY_AFTER_SECONDS: u64 = 3;

#[derive(Debug)]
struct FailingWorkflowReadPort;

impl AdkReadSnapshotPort for FailingWorkflowReadPort {
    fn read(&self, _path: &str, _query: &str) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
        Err(AdkReadSnapshotError::Failed {
            status: READ_FAILURE_STATUS,
            code: MANAGER_FAILURE_CODE.to_owned(),
            message: MANAGER_FAILURE_MESSAGE.to_owned(),
            retry_after_seconds: Some(READ_FAILURE_RETRY_AFTER_SECONDS),
        })
    }
}

#[derive(Debug)]
struct FailingWorkflowMutationPort;

impl AdkMutationPort for FailingWorkflowMutationPort {
    fn mutate(&self, _input: &AdkMutationInput) -> Result<Value, AdkMutationPortError> {
        Err(AdkMutationPortError::Failed {
            status: MUTATION_FAILURE_STATUS,
            code: MANAGER_FAILURE_CODE.to_owned(),
            message: MANAGER_FAILURE_MESSAGE.to_owned(),
        })
    }
}

fn mutate_result(
    port: &ProductionAdkPort,
    operation: AdkMutationOperation,
    identifiers: &[(&str, &str)],
    body: Value,
) -> Result<Value, AdkMutationPortError> {
    port.mutate(&AdkMutationInput {
        operation,
        identifiers: identifiers
            .iter()
            .map(|(key, value)| ((*key).to_owned(), (*value).to_owned()))
            .collect::<BTreeMap<_, _>>(),
        body,
        webhook_secret: None,
    })
}

fn mutate(
    port: &ProductionAdkPort,
    operation: AdkMutationOperation,
    identifiers: &[(&str, &str)],
    body: Value,
) -> Value {
    mutate_result(port, operation, identifiers, body).expect("mutation succeeds")
}

fn created_workflow_id(port: &ProductionAdkPort, agent_id: &str, name: &str) -> String {
    mutate(
        port,
        AdkMutationOperation::CreateWorkflow,
        &[],
        json!({"name": name, "agentId": agent_id, "promptTemplate": "prompt"}),
    )["id"]
        .as_str()
        .expect("workflow id")
        .to_owned()
}

fn read_json(port: &ProductionAdkPort, path: &str, query: &str) -> Value {
    match port.read(path, query).expect("read snapshot") {
        AdkReadSnapshot::Json(value) => value,
        other => panic!("expected a JSON snapshot for {path}, got {other:?}"),
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/workflow_tools_error_boundaries_test.go:103
/// `TestWorkflowToolsRemainingSessionAndPayloadErrors` — the
/// `applyWorkflowWriteFields` half.
///
/// Go copies name, description, status, agentId, workMode, providerId, model,
/// permissionMode, promptTemplate, objectiveTemplate, defaultInputs and tags
/// from the tool input into the write request. The Rust REST body carries the
/// same fields, so a create with the documented values stores every one of
/// them and the read projection returns them.
#[test]
fn workflow_writes_apply_the_documented_write_fields() {
    let (port, _directory) = agent_validation_port();
    let agent_id = mutate(
        &port,
        AdkMutationOperation::CreateAgent,
        &[],
        json!({"name": "Field Agent", "instruction": "fields", "model": "gpt-4o"}),
    )["id"]
        .as_str()
        .expect("agent id")
        .to_owned();

    let created = mutate(
        &port,
        AdkMutationOperation::CreateWorkflow,
        &[],
        json!({
            "name": "Field Workflow",
            "description": "description",
            "status": "DISABLED",
            "agentId": agent_id,
            "workMode": "chat",
            "providerId": "provider-enabled",
            "model": "fixture-model",
            "permissionMode": "approval",
            "promptTemplate": "prompt",
            "objectiveTemplate": "objective",
            "defaultInputs": {"symbol": "US.AAPL"},
            "tags": ["one"],
        }),
    );
    for (field, expected) in [
        ("name", json!("Field Workflow")),
        ("description", json!("description")),
        ("status", json!("DISABLED")),
        ("workMode", json!("chat")),
        ("providerId", json!("provider-enabled")),
        ("model", json!("fixture-model")),
        ("permissionMode", json!("approval")),
        ("promptTemplate", json!("prompt")),
        ("objectiveTemplate", json!("objective")),
        ("defaultInputs", json!({"symbol": "US.AAPL"})),
        ("tags", json!(["one"])),
        ("agentId", json!(agent_id)),
    ] {
        assert_eq!(created[field], expected, "{field}: {created}");
    }

    let workflow_id = created["id"].as_str().expect("workflow id");
    let listed = read_json(&port, "/api/v1/adk/workflows", "");
    let stored = listed["workflows"]
        .as_array()
        .expect("workflows array")
        .iter()
        .find(|workflow| workflow["id"] == workflow_id)
        .unwrap_or_else(|| panic!("the created workflow must stay listed: {listed}"));
    assert_eq!(stored["workMode"], "chat");
    assert_eq!(stored["providerId"], "provider-enabled");
    assert_eq!(stored["model"], "fixture-model");
    assert_eq!(stored["permissionMode"], "approval");
    assert_eq!(stored["objectiveTemplate"], "objective");
    assert_eq!(stored["defaultInputs"]["symbol"], "US.AAPL");
    assert_eq!(stored["tags"], json!(["one"]));
}

/// Parity: go:452dea11:internal/assistant/assembly/workflow_tools_error_boundaries_test.go:52
/// `TestWorkflowToolsRemainingManagerErrorPropagation`.
///
/// Every Go workflow tool returned the manager's own error unchanged, and the
/// nil manager failed closed. Rust's ports carry typed failures, so the
/// assertion here is that neither dispatch entry point rewrites them: the four
/// workflow read routes answer the read port's status, code, message and
/// `retry_after_seconds`, and all eight workflow mutation routes answer the
/// mutation port's status, code and message. The nil-port half lives in
/// `workflow_bridge_operations_fail_closed_without_their_ports`.
#[test]
fn workflow_manager_failures_surface_verbatim_on_every_workflow_route() {
    let read_port = FailingWorkflowReadPort;
    for path in [
        "/api/v1/adk/workflows",
        "/api/v1/adk/workflows/workflow-1",
        "/api/v1/adk/workflows/workflow-1/triggers",
        "/api/v1/adk/workflow-trigger-logs",
    ] {
        let failure = dispatch_adk_read(Some(&read_port), "GET", path, "")
            .expect_err("a failing read port must surface its own failure");
        assert_eq!(failure.status, READ_FAILURE_STATUS, "{path}");
        assert_eq!(failure.code, MANAGER_FAILURE_CODE, "{path}");
        assert_eq!(failure.message, MANAGER_FAILURE_MESSAGE, "{path}");
        assert_eq!(
            failure.retry_after_seconds,
            Some(READ_FAILURE_RETRY_AFTER_SECONDS),
            "{path}"
        );
    }

    let mutation_port = FailingWorkflowMutationPort;
    let routes: [(&str, &str, Value); 8] = [
        ("POST", "/api/v1/adk/workflows", json!({"name": "Failing"})),
        (
            "PUT",
            "/api/v1/adk/workflows/workflow-1",
            json!({"name": "Failing"}),
        ),
        ("DELETE", "/api/v1/adk/workflows/workflow-1", json!({})),
        (
            "POST",
            "/api/v1/adk/workflows/workflow-1/triggers",
            json!({"type": "manual", "title": "Failing"}),
        ),
        (
            "PUT",
            "/api/v1/adk/workflows/workflow-1/triggers/trigger-1",
            json!({"title": "Failing"}),
        ),
        (
            "DELETE",
            "/api/v1/adk/workflows/workflow-1/triggers/trigger-1",
            json!({}),
        ),
        (
            "POST",
            "/api/v1/adk/workflows/workflow-1/run",
            json!({"inputs": {"symbol": "US.AAPL"}}),
        ),
        (
            "POST",
            "/api/v1/adk/workflow-triggers/trigger-1/run",
            json!({}),
        ),
    ];
    for (method, path, body) in routes {
        let request = AdkMutationRequest {
            method: method.to_owned(),
            path: path.to_owned(),
            body: Some(body.to_string().into_bytes()),
            headers: BTreeMap::new(),
        };
        let response =
            dispatch_adk_mutation(&request, Some(&mutation_port), "2026-01-01T00:00:00Z");
        assert_eq!(
            response.status, MUTATION_FAILURE_STATUS,
            "{method} {path} must keep the port status: {response:?}"
        );
        assert_eq!(response.body["ok"], false, "{method} {path}");
        assert_eq!(
            response.body["error"]["code"], MANAGER_FAILURE_CODE,
            "{method} {path}: {response:?}"
        );
        assert_eq!(
            response.body["error"]["message"], MANAGER_FAILURE_MESSAGE,
            "{method} {path}: {response:?}"
        );
    }
}

/// Parity: go:452dea11:internal/assistant/assembly/workflow_tools_error_boundaries_test.go:103
/// `TestWorkflowToolsRemainingSessionAndPayloadErrors`.
///
/// Go decodes `canvasGraph` before it writes a definition, so both the console
/// tool (`decodeWorkflowCanvasGraph`) and the REST binding (`*WorkflowCanvas-
/// Graph`) reject a value that is not a graph document: `"invalid"` never
/// reaches the store, while an omitted key leaves the stored graph alone and a
/// well-formed document is kept. Rust stores the body as JSON, so the same
/// request is validated against the graph type the canvas runtime reads back.
#[test]
fn workflow_writes_reject_malformed_canvas_graphs_before_storing() {
    let (port, _directory) = agent_validation_port();
    let agent_id = mutate(
        &port,
        AdkMutationOperation::CreateAgent,
        &[],
        json!({"name": "Canvas Agent", "instruction": "canvas", "model": "gpt-4o"}),
    )["id"]
        .as_str()
        .expect("agent id")
        .to_owned();

    for malformed in [
        json!("invalid"),
        json!({"version": "v1", "nodes": "invalid"}),
        json!([{"id": "start", "type": "start"}]),
    ] {
        let failure = mutate_result(
            &port,
            AdkMutationOperation::CreateWorkflow,
            &[],
            json!({
                "name": "Broken Canvas",
                "agentId": agent_id,
                "promptTemplate": "prompt",
                "canvasGraph": malformed,
            }),
        )
        .expect_err("a malformed canvas graph must not be stored");
        let AdkMutationPortError::Failed {
            status,
            code,
            message,
        } = &failure
        else {
            panic!("a malformed canvas graph must answer a typed 400: {failure:?}");
        };
        assert_eq!(*status, 400, "{malformed}");
        assert_eq!(code, "BAD_REQUEST", "{malformed}");
        assert!(
            message.contains("canvasGraph"),
            "the canvas failure names the offending field: {malformed} -> {message}"
        );
    }

    let workflow_id = created_workflow_id(&port, &agent_id, "Omitted Canvas");
    let omitted = mutate(
        &port,
        AdkMutationOperation::UpdateWorkflow,
        &[("workflowId", workflow_id.as_str())],
        json!({"description": "kept"}),
    );
    assert!(
        omitted.get("canvasGraph").is_none_or(Value::is_null),
        "a workflow saved without a canvas graph stays without one: {omitted}"
    );

    let graph_workflow_id = mutate(
        &port,
        AdkMutationOperation::CreateWorkflow,
        &[],
        json!({
            "name": "Valid Canvas",
            "agentId": agent_id,
            "promptTemplate": "prompt",
            "canvasGraph": {"version": "v1"},
        }),
    )["id"]
        .as_str()
        .expect("workflow id")
        .to_owned();
    let failure = mutate_result(
        &port,
        AdkMutationOperation::UpdateWorkflow,
        &[("workflowId", graph_workflow_id.as_str())],
        json!({"canvasGraph": "invalid"}),
    )
    .expect_err("a malformed canvas graph must not overwrite a stored one");
    let AdkMutationPortError::Failed { status, code, .. } = &failure else {
        panic!("a malformed canvas graph must answer a typed 400: {failure:?}");
    };
    assert_eq!(*status, 400);
    assert_eq!(code, "BAD_REQUEST");

    let listed = read_json(&port, "/api/v1/adk/workflows", "");
    let stored = listed["workflows"]
        .as_array()
        .expect("workflows array")
        .iter()
        .find(|workflow| workflow["id"] == graph_workflow_id.as_str())
        .unwrap_or_else(|| panic!("the canvas workflow must stay listed: {listed}"));
    assert_eq!(
        stored["canvasGraph"]["version"], "v1",
        "the rejected update left the stored graph untouched: {stored}"
    );
    assert!(
        !listed["workflows"]
            .as_array()
            .expect("workflows array")
            .iter()
            .any(|workflow| workflow["name"] == "Broken Canvas"),
        "no rejected canvas write may leave a workflow behind: {listed}"
    );
}

/// Parity: go:452dea11:internal/assistant/assembly/workflow_tools_error_boundaries_test.go:103
/// `TestWorkflowToolsRemainingSessionAndPayloadErrors` — the non-webhook
/// trigger-update branch.
///
/// `workflowTriggerUpdateRequest` applied a new `type` and `config` when the
/// trigger was not webhook related, so a manual trigger can become a schedule
/// trigger in one update. The Rust route keeps the same contract and also
/// recomputes `nextRunAt` for the enabled schedule.
#[test]
fn workflow_trigger_update_switches_a_manual_trigger_to_a_schedule() {
    let (port, _directory) = agent_validation_port();
    let agent_id = mutate(
        &port,
        AdkMutationOperation::CreateAgent,
        &[],
        json!({"name": "Trigger Agent", "instruction": "trigger", "model": "gpt-4o"}),
    )["id"]
        .as_str()
        .expect("agent id")
        .to_owned();
    let workflow_id = created_workflow_id(&port, &agent_id, "Trigger Workflow");
    let trigger_id = mutate(
        &port,
        AdkMutationOperation::CreateWorkflowTrigger,
        &[("workflowId", workflow_id.as_str())],
        json!({"type": "manual", "title": "Manual Trigger"}),
    )["trigger"]["id"]
        .as_str()
        .expect("trigger id")
        .to_owned();

    let updated = mutate(
        &port,
        AdkMutationOperation::UpdateWorkflowTrigger,
        &[
            ("workflowId", workflow_id.as_str()),
            ("triggerId", trigger_id.as_str()),
        ],
        json!({"type": "schedule", "config": {"cron": "* * * * *"}}),
    );
    assert_eq!(updated["trigger"]["type"], "schedule");
    assert_eq!(updated["trigger"]["config"]["cron"], "* * * * *");
    assert_eq!(
        updated["trigger"]["title"], "Manual Trigger",
        "an omitted title keeps its stored value: {updated}"
    );
    assert!(
        updated["trigger"]["nextRunAt"]
            .as_str()
            .is_some_and(|value| !value.is_empty()),
        "an enabled schedule trigger computes its next run: {updated}"
    );
}
