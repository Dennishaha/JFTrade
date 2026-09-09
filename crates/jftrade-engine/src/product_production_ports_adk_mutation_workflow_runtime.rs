//! Durable workflow invocation and recovery helpers.

use std::collections::BTreeMap;

use jftrade_assistant::{CanvasCompiler, WorkflowCanvasGraph};
use jftrade_store_sqlite::AdkStore;
use serde_json::{Value, json};
use sha2::{Digest, Sha256};

use super::*;

#[path = "product_production_ports_adk_mutation_workflow_canvas.rs"]
mod canvas;
#[path = "product_workflow_resume.rs"]
mod resume;
#[path = "product_workflow_queue_dispatch.rs"]
mod queue_dispatch;

impl ProductionAdkPort {
    pub fn run_queued_workflow(&self, id: &str) -> Result<(), AdkMutationPortError> {
        queue_dispatch::run(self, id)
    }
    pub fn resume_workflow(&self, log_id: &str) -> Result<(), AdkMutationPortError> {
        resume::resume(self, log_id)
    }
}

use canvas::{
    CanvasExecutionContext, execute_canvas_workflow,
};

pub(super) fn run_workflow(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    run_workflow_with_checkpoint(port, input, None)
}

fn run_workflow_with_checkpoint(port: &ProductionAdkPort, input: &AdkMutationInput,
    queued: Option<jftrade_store_sqlite::StoredAdkWorkflowTriggerLog>) -> Result<Value, AdkMutationPortError> {
    let (workflow_id, trigger) = match input.operation {
        AdkMutationOperation::RunWorkflow => {
            let id = required_identifier(input, "workflowId")?;
            (id, None)
        }
        AdkMutationOperation::RunWorkflowTrigger | AdkMutationOperation::RunWorkflowWebhook => {
            let trigger_id = required_identifier(input, "triggerId")?;
            let trigger = port
                .store
                .get_workflow_trigger(&trigger_id)
                .map_err(storage_mutation_failed)?
                .ok_or_else(|| {
                    not_found_mutation(
                        "ADK_WORKFLOW_TRIGGER_NOT_FOUND",
                        "workflow trigger not found",
                    )
                })?;
            if input.operation == AdkMutationOperation::RunWorkflowWebhook {
                if !trigger.trigger_type.eq_ignore_ascii_case("WEBHOOK") {
                    return Err(invalid_mutation_input(
                        "workflow trigger is not a webhook trigger",
                    ));
                }
                if !trigger.status.eq_ignore_ascii_case("ENABLED") {
                    return Err(invalid_mutation_input("workflow webhook is disabled"));
                }
                let secret = input.webhook_secret.as_deref().unwrap_or_default().trim();
                let trigger_payload =
                    decode_mutation_payload(&trigger.payload_json, "workflow trigger")?;
                let expected = trigger_payload
                    .get("secretHash")
                    .and_then(Value::as_str)
                    .unwrap_or_default();
                let mut digest = Sha256::new();
                digest.update(secret.as_bytes());
                if secret.is_empty() || encode_hex(&digest.finalize()) != expected {
                    return Err(invalid_mutation_input("invalid workflow webhook secret"));
                }
            } else if !trigger.status.eq_ignore_ascii_case("ENABLED") {
                return Err(invalid_mutation_input("workflow trigger is disabled"));
            }
            (trigger.workflow_id.clone(), Some(trigger))
        }
        _ => unreachable!(),
    };
    let workflow = port
        .store
        .get_workflow(&workflow_id)
        .map_err(storage_mutation_failed)?
        .ok_or_else(|| not_found_mutation("ADK_WORKFLOW_NOT_FOUND", "workflow not found"))?;
    if !workflow.status.eq_ignore_ascii_case("ENABLED")
        || is_deleted_payload(&workflow.payload_json)?
    {
        return Err(invalid_mutation_input("workflow is disabled"));
    }
    let workflow_value = decode_mutation_payload(&workflow.payload_json, "workflow")?;
    let mut inputs = workflow_value
        .get("defaultInputs")
        .cloned()
        .filter(Value::is_object)
        .unwrap_or_else(|| json!({}));
    if let Some(object) = inputs.as_object_mut() {
        if let Some(explicit_inputs) = input.body.get("inputs").and_then(Value::as_object) {
            object.extend(explicit_inputs.clone());
        }
        object.extend(input.body.as_object().cloned().unwrap_or_default());
    }
    let canvas_graph_opt = workflow_value
        .get("canvasGraph")
        .filter(|v| !v.is_null())
        .filter(|v| v.as_object().is_some_and(|o| !o.is_empty()));

    let canvas_graph = match canvas_graph_opt {
        Some(value) => serde_json::from_value::<WorkflowCanvasGraph>(value.clone())
            .map_err(|e| invalid_mutation_input(&format!("invalid workflow canvasGraph: {e}")))?,
        None => WorkflowCanvasGraph::single_agent(),
    };
    let topological_order = CanvasCompiler::new(&canvas_graph)
        .and_then(|c| c.compile()).map_err(|e| invalid_mutation_input(&e.to_string()))?;

    let invocation_uuid = crate::product_id::generate_uuid_v4();
    let started_at = now_rfc3339();
    let log_id = match queued.as_ref() { Some(row) => row.id.clone(), None => generate_workflow_log_id()? };
    let trigger_id = trigger
        .as_ref()
        .map(|value| value.id.as_str())
        .unwrap_or_default();
    let trigger_type = trigger
        .as_ref()
        .map(|value| value.trigger_type.as_str())
        .unwrap_or("manual");
    let mut invocation = json!({
        "id": log_id.clone(),
        "workflowId": workflow_id,
        "triggerId": trigger_id,
        "triggerType": trigger_type,
        "status": "RUNNING",
        "runId": "",
        "sessionId": "",
        "inputs": inputs.clone(),
        "result": Value::Null,
        "startedAt": started_at.clone(),
    });
    invocation["canvasExecution"] = json!({
        "workflow": workflow_value, "graph": canvas_graph, "invocationUuid": invocation_uuid,
        "trigger": trigger,
    });
    let _owner = port.store.try_claim_workflow_execution(&log_id).map_err(storage_mutation_failed)?;
    let stored_invocation = match queued {
        Some(row) => port.store.update_workflow_trigger_log_if_revision(
            &log_id, &row.updated_at, "RUNNING", "", &invocation.to_string(),
        ).map_err(storage_mutation_failed)?.ok_or_else(|| invalid_mutation_input("workflow queue claim changed"))?,
        None => port.store.create_workflow_trigger_log(
            &log_id, &workflow_id, trigger_id, trigger_type, "RUNNING", "", &invocation.to_string(),
        ).map_err(storage_mutation_failed)?,
    };
    let checkpoint = crate::product_workflow_checkpoint::WorkflowCheckpoint::new(&port.store, stored_invocation);
    let invocation_revision = checkpoint.revision();
    let Some(runtime) = port.chat_runtime.as_deref() else {
        return Err(finalize_workflow_failure(
            port,
            &log_id,
            &invocation_revision,
            &invocation,
            "assistant model runtime is unavailable",
            "ADK_WORKFLOW_RUNTIME_UNAVAILABLE",
        ));
    };

    let outcome_result = execute_canvas_workflow(&CanvasExecutionContext {
        runtime, workflow_id: &workflow_id, workflow_value: &workflow_value,
        trigger: trigger.as_ref(), inputs: &inputs, graph: &canvas_graph,
        topological_order: &topological_order, invocation_uuid: &invocation_uuid,
        checkpoint: &checkpoint,
    });
    let outcome = match outcome_result {
        Ok(outcome) => outcome,
        Err((message, code)) => {
            let durable = checkpoint.payload().unwrap_or_else(|_| invocation.clone());
            return Err(finalize_workflow_failure(
                port,
                &log_id,
                &checkpoint.revision(),
                &durable,
                &message,
                &code,
            ));
        }
    };

    let invocation_revision = checkpoint.revision();
    let mut log = json!({
        "id": log_id,
        "workflowId": workflow_id,
        "triggerId": trigger_id,
        "triggerType": trigger_type,
        "status": outcome.status,
        "runId": outcome.run_id,
        "sessionId": outcome.session_id,
        "inputs": inputs,
        "nodeRuns": outcome.node_runs,
        "result": outcome.response.clone(),
        "startedAt": started_at,
    });
    if let Some(execution) = invocation.get("canvasExecution") {
        log["canvasExecution"] = execution.clone();
    }
    if !matches!(outcome.status.as_str(), "RUNNING" | "PENDING_APPROVAL")
        && let Some(object) = log.as_object_mut()
    {
        object.insert("finishedAt".to_owned(), Value::String(now_rfc3339()));
    }
    let stored = match port
        .store
        .update_workflow_trigger_log_if_revision(
            &log_id,
            &invocation_revision,
            &outcome.status,
            &outcome.run_id,
            &log.to_string(),
        )
        .map_err(storage_mutation_failed)?
    {
        Some(stored) => stored,
        None => {
            let winner = port
                .store
                .get_workflow_trigger_log(&log_id)
                .map_err(storage_mutation_failed)?
                .ok_or_else(|| AdkMutationPortError::Failed {
                    status: 409,
                    code: "ADK_WORKFLOW_CONFLICT".to_owned(),
                    message: "workflow invocation disappeared before completion".to_owned(),
                })?;
            let mut winner_log =
                decode_mutation_payload(&winner.payload_json, "workflow trigger log")?;
            if let Some(object) = winner_log.as_object_mut() {
                object.insert("createdAt".to_owned(), Value::String(winner.created_at));
                object.insert("updatedAt".to_owned(), Value::String(winner.updated_at));
            }
            let winner_response = winner_log.get("result").cloned().unwrap_or(Value::Null);
            super::super::projection::public_workflow_log(&mut winner_log);
            return Ok(json!({
                "workflow": workflow_payload(&workflow)?,
                "trigger": trigger.as_ref().map(workflow_trigger_payload).transpose()?,
                "log": winner_log,
                "response": winner_response,
            }));
        }
    };
    log = decode_mutation_payload(&stored.payload_json, "workflow trigger log")?;
    if let Some(object) = log.as_object_mut() {
        object.insert("createdAt".to_owned(), Value::String(stored.created_at));
        object.insert("updatedAt".to_owned(), Value::String(stored.updated_at));
    }
    super::super::projection::public_workflow_log(&mut log);
    Ok(json!({
        "workflow": workflow_payload(&workflow)?,
        "trigger": trigger.as_ref().map(workflow_trigger_payload).transpose()?,
        "log": log,
        "response": outcome.response,
    }))
}

fn generate_workflow_log_id() -> Result<String, AdkMutationPortError> {
    Ok(crate::product_id::generate_prefixed_id("workflow-log"))
}

fn finalize_workflow_failure(
    port: &ProductionAdkPort,
    log_id: &str,
    expected_revision: &str,
    invocation: &Value,
    message: &str,
    code: &str,
) -> AdkMutationPortError {
    finalize_workflow_failure_with_store(
        port.store.as_ref(),
        log_id,
        expected_revision,
        invocation,
        message,
        code,
    )
}

fn finalize_workflow_failure_with_store(
    store: &AdkStore,
    log_id: &str,
    expected_revision: &str,
    invocation: &Value,
    message: &str,
    code: &str,
) -> AdkMutationPortError {
    let mut payload = invocation.clone();
    if let Some(object) = payload.as_object_mut() {
        object.insert("status".to_owned(), Value::String("FAILED".to_owned()));
        object.insert("errorCode".to_owned(), Value::String(code.to_owned()));
        object.insert("error".to_owned(), Value::String(message.to_owned()));
        object.insert("finishedAt".to_owned(), Value::String(now_rfc3339()));
    }
    let fallback = workflow_failure_error(code, message);
    match store.update_workflow_trigger_log_if_revision(
        log_id,
        expected_revision,
        "FAILED",
        "",
        &payload.to_string(),
    ) {
        Ok(Some(_)) => fallback,
        Ok(None) => match store.get_workflow_trigger_log(log_id) {
            Ok(Some(winner)) => workflow_failure_from_winner(&winner),
            Ok(None) => AdkMutationPortError::Failed {
                status: 409,
                code: "ADK_WORKFLOW_CONFLICT".to_owned(),
                message: "workflow invocation disappeared before failure could be persisted"
                    .to_owned(),
            },
            Err(error) => AdkMutationPortError::Failed {
                status: 500,
                code: "ADK_WORKFLOW_PERSIST_FAILED".to_owned(),
                message: format!("{message}; failed to read durable workflow winner: {error}"),
            },
        },
        Err(error) => AdkMutationPortError::Failed {
            status: 500,
            code: "ADK_WORKFLOW_PERSIST_FAILED".to_owned(),
            message: format!("{message}; failed to persist terminal workflow state: {error}"),
        },
    }
}

fn workflow_failure_error(code: &str, message: &str) -> AdkMutationPortError {
    AdkMutationPortError::Failed {
        status: if code.to_ascii_uppercase().contains("UNAVAILABLE") {
            503
        } else {
            502
        },
        code: code.to_owned(),
        message: message.to_owned(),
    }
}

fn workflow_failure_from_winner(
    winner: &jftrade_store_sqlite::StoredAdkWorkflowTriggerLog,
) -> AdkMutationPortError {
    if winner.status.eq_ignore_ascii_case("FAILED") {
        let payload = match decode_mutation_payload(&winner.payload_json, "workflow trigger log") {
            Ok(payload) => payload,
            Err(error) => return error,
        };
        let code = payload
            .get("errorCode")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .unwrap_or("ADK_WORKFLOW_FAILED");
        let message = payload
            .get("error")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .unwrap_or("workflow invocation failed");
        return workflow_failure_error(code, message);
    }
    AdkMutationPortError::Failed {
        status: 409,
        code: "ADK_WORKFLOW_CONFLICT".to_owned(),
        message: format!(
            "workflow invocation was finalized by another executor ({})",
            winner.status
        ),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_store_sqlite::initialize_current;
    use rusqlite::Connection;
    use std::fs::File;
    use std::time::Duration;
    use tempfile::tempdir;

    #[test]
    fn failure_cas_loser_returns_durable_failed_winner_without_overwriting_it() {
        let directory = tempdir().expect("temporary directory");
        let path = directory.path().join("adk.db");
        File::create(&path).expect("create ADK database");
        initialize_current(
            &Connection::open(&path).expect("initialize ADK database"),
            "adk",
        )
        .expect("initialize ADK schema");
        let store = AdkStore::open(&path).expect("open ADK store");
        let initial = store
            .create_workflow_trigger_log(
                "workflow-log-1",
                "workflow-1",
                "",
                "manual",
                "RUNNING",
                "",
                r#"{"status":"RUNNING"}"#,
            )
            .expect("create invocation");
        std::thread::sleep(Duration::from_millis(2));
        let winner_payload = r#"{"status":"FAILED","errorCode":"ADK_WORKFLOW_RUNTIME_UNAVAILABLE","error":"winner failure"}"#;
        let winner = store
            .update_workflow_trigger_log_if_revision(
                &initial.id,
                &initial.updated_at,
                "FAILED",
                "",
                winner_payload,
            )
            .expect("persist winner")
            .expect("winner row");
        assert_ne!(
            winner.updated_at, initial.updated_at,
            "CAS revision must advance"
        );

        let error = finalize_workflow_failure_with_store(
            &store,
            &initial.id,
            &initial.updated_at,
            &json!({"id":"workflow-log-1","status":"RUNNING"}),
            "loser failure",
            "ADK_WORKFLOW_FAILED",
        );
        assert_eq!(
            error,
            AdkMutationPortError::Failed {
                status: 503,
                code: "ADK_WORKFLOW_RUNTIME_UNAVAILABLE".to_owned(),
                message: "winner failure".to_owned(),
            }
        );
        let durable = store
            .get_workflow_trigger_log(&initial.id)
            .expect("read durable winner")
            .expect("winner remains present");
        assert_eq!(durable.status, "FAILED");
        assert_eq!(durable.payload_json, winner_payload);
    }

    #[test]
    fn failure_cas_loser_distinguishes_missing_durable_winner() {
        let directory = tempdir().expect("temporary directory");
        let path = directory.path().join("adk.db");
        File::create(&path).expect("create ADK database");
        initialize_current(
            &Connection::open(&path).expect("initialize ADK database"),
            "adk",
        )
        .expect("initialize ADK schema");
        let store = AdkStore::open(&path).expect("open ADK store");
        let error = finalize_workflow_failure_with_store(
            &store,
            "missing-workflow-log",
            "stale-revision",
            &json!({"id":"missing-workflow-log","status":"RUNNING"}),
            "loser failure",
            "ADK_WORKFLOW_FAILED",
        );
        assert_eq!(
            error,
            AdkMutationPortError::Failed {
                status: 409,
                code: "ADK_WORKFLOW_CONFLICT".to_owned(),
                message: "workflow invocation disappeared before failure could be persisted"
                    .to_owned(),
            }
        );
    }
}
