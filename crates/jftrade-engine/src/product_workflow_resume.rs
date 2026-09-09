//! Resume only durable Canvas invocations. Child model runs retain their own leases.
use jftrade_assistant::{CanvasCompiler, WorkflowCanvasGraph};
use serde_json::Value;

use super::{ProductionAdkPort, AdkMutationPortError, storage_mutation_failed, invalid_mutation_input};
use super::canvas::{CanvasExecutionContext, execute_canvas_workflow};
use crate::product_workflow_checkpoint::WorkflowCheckpoint;

pub(super) fn resume(port: &ProductionAdkPort, id: &str) -> Result<(), AdkMutationPortError> {
    let Some(_owner) = port.store.try_claim_workflow_execution(id).map_err(storage_mutation_failed)? else {
        return Ok(());
    };
    let Some(row) = port.store.get_workflow_trigger_log(id).map_err(storage_mutation_failed)? else { return Ok(()) };
    if !matches!(row.status.as_str(), "RUNNING" | "PENDING_APPROVAL" | "PENDING_INPUT" | "QUEUED") { return Ok(()) }
    let checkpoint = WorkflowCheckpoint::new(&port.store, row.clone());
    let payload = checkpoint.payload().map_err(checkpoint_error)?;
    let Some(execution) = payload.get("canvasExecution").filter(|v| v.is_object()) else { return Ok(()) };
    let graph: WorkflowCanvasGraph = serde_json::from_value(execution["graph"].clone())
        .map_err(|e| invalid_mutation_input(&e.to_string()))?;
    let order = CanvasCompiler::new(&graph).and_then(|c| c.compile())
        .map_err(|e| invalid_mutation_input(&e.to_string()))?;
    let trigger: Option<jftrade_store_sqlite::StoredAdkWorkflowTrigger> = serde_json::from_value(execution["trigger"].clone())
        .map_err(|e| invalid_mutation_input(&e.to_string()))?;
    let Some(runtime) = port.chat_runtime.as_deref() else { return Ok(()) };
    let uuid = execution["invocationUuid"].as_str().ok_or_else(|| invalid_mutation_input("missing invocation UUID"))?;
    let outcome = execute_canvas_workflow(&CanvasExecutionContext {
        runtime, workflow_id: &row.workflow_id, workflow_value: &execution["workflow"],
        trigger: trigger.as_ref(), inputs: &payload["inputs"], graph: &graph,
        topological_order: &order, invocation_uuid: uuid, checkpoint: &checkpoint,
    }).map_err(checkpoint_error)?;
    let mut payload = checkpoint.payload().map_err(checkpoint_error)?;
    if !matches!(outcome.status.as_str(), "RUNNING" | "PENDING_APPROVAL" | "PENDING_INPUT") {
        payload["finishedAt"] = Value::String(super::now_rfc3339());
    }
    port.store.update_workflow_trigger_log_if_revision(id, &checkpoint.revision(),
        &outcome.status, &outcome.run_id, &payload.to_string()).map_err(storage_mutation_failed)?;
    Ok(())
}

fn checkpoint_error((message, code): (String, String)) -> AdkMutationPortError {
    AdkMutationPortError::Failed { status: 500, code, message }
}
