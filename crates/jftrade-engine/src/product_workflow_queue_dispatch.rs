//! Dispatch a durable scheduler row without creating another invocation identity.
use super::*;
pub(super) fn run(port: &ProductionAdkPort, id: &str) -> Result<(), AdkMutationPortError> {
    let Some(_guard) = port.store.try_claim_workflow_execution(id).map_err(storage_mutation_failed)? else { return Ok(()) };
    let Some(row) = port.store.get_workflow_trigger_log(id).map_err(storage_mutation_failed)? else { return Ok(()) };
    if row.status != "QUEUED" { return Ok(()); }
    let mut payload = decode_mutation_payload(&row.payload_json, "workflow queue")?;
    let invocation = &payload["schedulerInvocation"];
    let trigger_id = invocation["triggerId"].as_str().ok_or_else(|| invalid_mutation_input("queued trigger ID missing"))?;
    let input = AdkMutationInput { operation: AdkMutationOperation::RunWorkflowTrigger,
        identifiers: BTreeMap::from([("triggerId".to_owned(), trigger_id.to_owned())]),
        body: invocation["body"].clone(), webhook_secret: None };
    match run_workflow_with_checkpoint(port, &input, Some(row.clone())) {
        Ok(_) => Ok(()),
        Err(error) => {
            payload["status"] = json!("FAILED"); payload["error"] = json!(error.to_string());
            payload["finishedAt"] = json!(now_rfc3339());
            port.store.update_workflow_trigger_log_if_revision(id, &row.updated_at, "FAILED", "", &payload.to_string()).map_err(storage_mutation_failed)?;
            Err(error)
        }
    }
}
