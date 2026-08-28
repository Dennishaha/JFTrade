//! Production ADK tasks mutation dispatch.

use super::*;

pub(super) fn handles(operation: AdkMutationOperation) -> bool {
    matches!(
        operation,
        AdkMutationOperation::UpdateRunObjective
            | AdkMutationOperation::CreateTask
            | AdkMutationOperation::UpdateTask
            | AdkMutationOperation::DeleteTask
            | AdkMutationOperation::CancelOptimizationTask
            | AdkMutationOperation::RenameSession
    )
}

pub(super) fn dispatch(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    debug_assert!(handles(input.operation));
    match input.operation {
            AdkMutationOperation::UpdateRunObjective => {
                let id = required_identifier(input, "runId")?;
                let objective = required_body_string(&input.body, "objective")?;
                let Some(existing) = port.store.get_run(&id).map_err(storage_mutation_failed)? else {
                    return Err(not_found_mutation("ADK_RUN_NOT_FOUND", "run not found"));
                };
                let mut value = decode_mutation_payload(&existing.payload_json, "run")?;
                let object = value.as_object_mut().ok_or_else(|| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: "stored ADK run payload must be a JSON object".to_owned(),
                })?;
                let work_mode = object
                    .get("workMode")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .unwrap_or_default();
                if !work_mode.eq_ignore_ascii_case("loop") {
                    return Err(invalid_mutation_input(
                        "objective can only be updated for goal runs",
                    ));
                }
                if object
                    .get("parentRunId")
                    .and_then(Value::as_str)
                    .is_some_and(|parent| !parent.trim().is_empty())
                {
                    return Err(invalid_mutation_input(
                        "child run objective cannot be updated",
                    ));
                }
                if !matches!(
                    existing.status.trim().to_ascii_uppercase().as_str(),
                    "RUNNING" | "PENDING_APPROVAL"
                ) {
                    return Err(invalid_mutation_input(
                        "objective cannot be updated for terminal run",
                    ));
                }
                object.insert("objective".to_owned(), Value::String(bounded_text(&objective)));
                object.insert("id".to_owned(), Value::String(existing.id.clone()));
                object.insert("status".to_owned(), Value::String(existing.status.clone()));
                if !port
                    .store
                    .update_run_payload(&id, &value.to_string())
                    .map_err(storage_mutation_failed)?
                {
                    return Err(not_found_mutation("ADK_RUN_NOT_FOUND", "run not found"));
                }
                let updated = port
                    .store
                    .get_run(&id)
                    .map_err(storage_mutation_failed)?
                    .ok_or_else(|| not_found_mutation("ADK_RUN_NOT_FOUND", "run not found"))?;
                let mut result = decode_mutation_payload(&updated.payload_json, "run")?;
                let result_object = result.as_object_mut().ok_or_else(|| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: "stored ADK run payload must be a JSON object".to_owned(),
                })?;
                result_object.insert("id".to_owned(), Value::String(updated.id));
                result_object.insert("status".to_owned(), Value::String(updated.status));
                result_object.insert("sessionId".to_owned(), Value::String(updated.session_id));
                result_object.insert("agentId".to_owned(), Value::String(updated.agent_id));
                result_object.insert("createdAt".to_owned(), Value::String(updated.created_at));
                result_object.insert("updatedAt".to_owned(), Value::String(updated.updated_at));
                Ok(result)
            }
            AdkMutationOperation::CreateTask => {
                let body = object_body(&input.body, "task")?;
                let id = normalize_id(&normalized_string(body.get("id")));
                let id = if id.is_empty() {
                    next_id("task", &TASK_ID_SEQUENCE)
                } else {
                    id
                };
                let title = normalized_string(body.get("title"));
                if title.is_empty() {
                    return Err(invalid_mutation_input("task title is required"));
                }
                let status = task_status(body.get("status"))?;
                let depends_on = string_slice(body.get("dependsOn"), "dependsOn")?;
                reject_self_dependency(&id, &depends_on)?;
                let agent_id = normalized_string(body.get("agentId"));
                let run_id = normalized_string(body.get("runId"));
                let mut payload = Value::Object(body);
                let object = payload.as_object_mut().expect("object payload");
                object.insert("id".to_owned(), Value::String(id.clone()));
                object.insert("title".to_owned(), Value::String(title));
                object.insert("status".to_owned(), Value::String(status.clone()));
                object.insert("agentId".to_owned(), Value::String(agent_id.clone()));
                object.insert("runId".to_owned(), Value::String(run_id.clone()));
                object.insert(
                    "dependsOn".to_owned(),
                    Value::Array(depends_on.into_iter().map(Value::String).collect()),
                );
                let stored = port
                    .store
                    .upsert_task(&id, &status, &agent_id, &run_id, &payload.to_string())
                    .map_err(storage_mutation_failed)?;
                task_payload(&stored)
            }
            AdkMutationOperation::UpdateTask => {
                let id = required_identifier(input, "taskId")?;
                let Some(existing) = port.store.get_task(&id).map_err(storage_mutation_failed)? else {
                    return Err(not_found_mutation("ADK_TASK_NOT_FOUND", "task not found"));
                };
                let mut payload = decode_mutation_payload(&existing.payload_json, "task")?;
                let object = payload.as_object_mut().ok_or_else(|| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: "stored ADK task payload must be a JSON object".to_owned(),
                })?;
                let body = object_body(&input.body, "task")?;
                if let Some(title) = body.get("title").and_then(Value::as_str)
                    && title.trim().is_empty()
                {
                    return Err(invalid_mutation_input("task title is required"));
                }
                if body.contains_key("status") {
                    object.insert("status".to_owned(), Value::String(task_status(body.get("status"))?));
                }
                if let Some(depends_on) = body.get("dependsOn")
                    && !depends_on.is_null()
                {
                    let values = string_slice(Some(depends_on), "dependsOn")?;
                    reject_self_dependency(&id, &values)?;
                    object.insert(
                        "dependsOn".to_owned(),
                        Value::Array(values.into_iter().map(Value::String).collect()),
                    );
                }
                for key in [
                    "title", "description", "agentId", "runId", "modeHint", "agentRole",
                    "plannerStepId", "planSource", "workflowMode", "objective", "message",
                    "executor", "childAgentId", "childProviderId", "childModel",
                    "childPermissionMode", "resultSummary",
                ] {
                    update_string_field(object, &body, key);
                }
                if let Some(value) = body.get("order").filter(|value| !value.is_null()) {
                    if !value.is_i64() {
                        return Err(invalid_mutation_input("task order must be an integer"));
                    }
                    object.insert("order".to_owned(), value.clone());
                }
                if let Some(warnings) = body.get("plannerWarnings")
                    && !warnings.is_null()
                {
                    let values = string_slice(Some(warnings), "plannerWarnings")?;
                    object.insert(
                        "plannerWarnings".to_owned(),
                        Value::Array(values.into_iter().map(Value::String).collect()),
                    );
                }
                object.insert("id".to_owned(), Value::String(id.clone()));
                let status = object
                    .get("status")
                    .and_then(Value::as_str)
                    .map(str::to_owned)
                    .unwrap_or(existing.status);
                let agent_id = normalized_string(object.get("agentId"));
                let run_id = normalized_string(object.get("runId"));
                let stored = port
                    .store
                    .upsert_task(&id, &status, &agent_id, &run_id, &payload.to_string())
                    .map_err(storage_mutation_failed)?;
                task_payload(&stored)
            }
            AdkMutationOperation::DeleteTask => {
                let id = required_identifier(input, "taskId")?;
                let deleted = port
                    .store
                    .delete_task(&id)
                    .map_err(storage_mutation_failed)?;
                if !deleted {
                    return Err(not_found_mutation("ADK_TASK_NOT_FOUND", "task not found"));
                }
                Ok(json!({"id": id, "deleted": true}))
            }
            AdkMutationOperation::CancelOptimizationTask => {
                let id = required_identifier(input, "taskId")?;
                let Some(stored) = port
                    .store
                    .get_optimization_task(&id)
                    .map_err(storage_mutation_failed)?
                else {
                    return Err(not_found_mutation(
                        "ADK_OPTIMIZATION_TASK_NOT_FOUND",
                        "optimization task not found",
                    ));
                };
                let mut payload = decode_mutation_payload(&stored.payload_json, "optimization task")?;
                let object = payload.as_object_mut().ok_or_else(|| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: "stored ADK optimization task payload must be a JSON object".to_owned(),
                })?;
                object.insert("status".to_owned(), Value::String("cancelled".to_owned()));
                let updated = port
                    .store
                    .upsert_optimization_task(&id, &payload.to_string())
                    .map_err(storage_mutation_failed)?;
                optimization_payload(&updated)
            }
            AdkMutationOperation::RenameSession => {
                let id = required_identifier(input, "sessionId")?;
                let Some(existing) = port.store.get_session(&id).map_err(session_mutation_failed)? else {
                    return Err(AdkMutationPortError::Failed {
                        status: 404,
                        code: "NOT_FOUND".to_owned(),
                        message: "session not found".to_owned(),
                    });
                };
                let mut value = decode_mutation_payload(&existing.payload_json, "session")?;
                let object = value.as_object_mut().ok_or_else(|| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: "stored ADK session payload must be a JSON object".to_owned(),
                })?;
                let title = input
                    .body
                    .get("title")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|title| !title.is_empty())
                    .ok_or_else(|| invalid_mutation_input("session title is required"))?;
                object.insert(
                    "title".to_owned(),
                    Value::String(title.chars().take(80).collect()),
                );
                let agent_id = object
                    .get("agentId")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|agent_id| !agent_id.is_empty())
                    .ok_or_else(|| AdkMutationPortError::Failed {
                        status: 500,
                        code: "ADK_STORAGE_CORRUPT".to_owned(),
                        message: "stored ADK session is missing agentId".to_owned(),
                    })?
                    .to_owned();
                let stored = port
                    .store
                    .upsert_session(&id, &agent_id, &value.to_string())
                    .map_err(session_mutation_failed)?;
                session_entity_value(&stored)
            }
        _ => unreachable!("operation group checked before dispatch"),
    }
}
