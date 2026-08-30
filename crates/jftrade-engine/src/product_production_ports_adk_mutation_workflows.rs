//! Production ADK workflows mutation dispatch.

use super::*;

pub(super) fn handles(operation: AdkMutationOperation) -> bool {
    matches!(
        operation,
        AdkMutationOperation::CreateWorkflow
            | AdkMutationOperation::UpdateWorkflow
            | AdkMutationOperation::DeleteWorkflow
            | AdkMutationOperation::CreateWorkflowTrigger
            | AdkMutationOperation::UpdateWorkflowTrigger
            | AdkMutationOperation::DeleteWorkflowTrigger
    )
}

pub(super) fn dispatch(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    debug_assert!(handles(input.operation));
    match input.operation {
        AdkMutationOperation::CreateWorkflow | AdkMutationOperation::UpdateWorkflow => {
            let is_update = input.operation == AdkMutationOperation::UpdateWorkflow;
            let body = object_body(&input.body, "workflow")?;
            let id = input
                .identifiers
                .get("workflowId")
                .cloned()
                .or_else(|| body.get("id").and_then(Value::as_str).map(str::to_owned))
                .map(|value| normalize_id(&value))
                .filter(|value| !value.is_empty())
                .unwrap_or_else(|| next_id("workflow", &WORKFLOW_ID_SEQUENCE));
            let existing = port
                .store
                .get_workflow(&id)
                .map_err(storage_mutation_failed)?;
            if is_update {
                let Some(existing) = existing.as_ref() else {
                    return Err(not_found_mutation(
                        "ADK_WORKFLOW_NOT_FOUND",
                        "workflow not found",
                    ));
                };
                if is_deleted_payload(&existing.payload_json)? {
                    return Err(not_found_mutation(
                        "ADK_WORKFLOW_NOT_FOUND",
                        "workflow not found",
                    ));
                }
            }
            let mut payload = match existing.as_ref() {
                Some(existing) => decode_mutation_payload(&existing.payload_json, "workflow")?,
                None => Value::Object(Map::new()),
            };
            let object = payload
                .as_object_mut()
                .ok_or_else(|| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: "stored ADK workflow payload must be a JSON object".to_owned(),
                })?;
            for key in [
                "name",
                "description",
                "agentId",
                "workMode",
                "providerId",
                "model",
                "permissionMode",
                "promptTemplate",
                "objectiveTemplate",
                "defaultInputs",
                "canvasGraph",
                "tags",
                "builtinTemplate",
            ] {
                if let Some(value) = body.get(key) {
                    object.insert(key.to_owned(), value.clone());
                }
            }
            let name = object
                .get("name")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .ok_or_else(|| invalid_mutation_input("workflow name is required"))?;
            object.insert("name".to_owned(), Value::String(name.to_owned()));
            let agent_id = object
                .get("agentId")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .ok_or_else(|| invalid_mutation_input("workflow agentId is required"))?
                .to_owned();
            validate_session_agent(port, &agent_id)?;
            object.insert("agentId".to_owned(), Value::String(agent_id));
            let work_mode = normalize_workflow_mode(object.get("workMode"), "loop");
            object.insert("workMode".to_owned(), Value::String(work_mode));
            let prompt = object
                .get("promptTemplate")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .ok_or_else(|| invalid_mutation_input("workflow promptTemplate is required"))?;
            object.insert(
                "promptTemplate".to_owned(),
                Value::String(prompt.to_owned()),
            );
            let status = normalize_workflow_status(
                body.get("status"),
                existing
                    .as_ref()
                    .map(|workflow| workflow.status.as_str())
                    .unwrap_or("ENABLED"),
            );
            object.insert("id".to_owned(), Value::String(id.clone()));
            object.insert("status".to_owned(), Value::String(status.clone()));
            let payload_json = payload.to_string();
            let stored = if let Some(existing) = existing.as_ref() {
                let expected = expected_updated_at(&body, &existing.updated_at)?;
                if !port
                    .store
                    .update_workflow_if_revision(
                        &id,
                        &expected,
                        &status,
                        &payload_json,
                    )
                    .map_err(storage_mutation_failed)?
                {
                    let current = port
                        .store
                        .get_workflow(&id)
                        .map_err(storage_mutation_failed)?
                        .ok_or_else(|| {
                            not_found_mutation("ADK_WORKFLOW_NOT_FOUND", "workflow not found")
                        })?;
                    if current.updated_at != expected {
                        return Err(revision_conflict("WORKFLOW"));
                    }
                    return Err(revision_conflict("WORKFLOW"));
                }
                port.store
                    .get_workflow(&id)
                    .map_err(storage_mutation_failed)?
                    .ok_or_else(|| {
                        not_found_mutation("ADK_WORKFLOW_NOT_FOUND", "workflow not found")
                    })?
            } else {
                port.store
                    .upsert_workflow(&id, &status, &payload_json)
                    .map_err(storage_mutation_failed)?
            };
            workflow_payload(&stored)
        }
        AdkMutationOperation::DeleteWorkflow => {
            let id = required_identifier(input, "workflowId")?;
            let Some(existing) = port
                .store
                .get_workflow(&id)
                .map_err(storage_mutation_failed)?
            else {
                return Err(not_found_mutation(
                    "ADK_WORKFLOW_NOT_FOUND",
                    "workflow not found",
                ));
            };
            if is_deleted_payload(&existing.payload_json)? {
                return Err(not_found_mutation(
                    "ADK_WORKFLOW_NOT_FOUND",
                    "workflow not found",
                ));
            }
            let mut payload = decode_mutation_payload(&existing.payload_json, "workflow")?;
            let object = payload
                .as_object_mut()
                .ok_or_else(|| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: "stored ADK workflow payload must be a JSON object".to_owned(),
                })?;
            object.insert("id".to_owned(), Value::String(id.clone()));
            object.insert("status".to_owned(), Value::String("DISABLED".to_owned()));
            let deleted_at = now_rfc3339();
            object.insert(
                "deletedAt".to_owned(),
                Value::String(deleted_at.clone()),
            );
            let expected = expected_updated_at(
                input.body.as_object().unwrap_or(&Map::new()),
                &existing.updated_at,
            )?;
            let changed = port
                .store
                .soft_delete_workflow_if_revision(
                    &id,
                    &expected,
                    &payload.to_string(),
                    &deleted_at,
                )
                .map_err(storage_mutation_failed)?;
            if !changed {
                let current = port
                    .store
                    .get_workflow(&id)
                    .map_err(storage_mutation_failed)?
                    .ok_or_else(|| {
                        not_found_mutation("ADK_WORKFLOW_NOT_FOUND", "workflow not found")
                    })?;
                if is_deleted_payload(&current.payload_json)? {
                    return Err(not_found_mutation(
                        "ADK_WORKFLOW_NOT_FOUND",
                        "workflow not found",
                    ));
                }
                return Err(revision_conflict("WORKFLOW"));
            }
            let stored = port
                .store
                .get_workflow(&id)
                .map_err(storage_mutation_failed)?
                .ok_or_else(|| {
                    not_found_mutation("ADK_WORKFLOW_NOT_FOUND", "workflow not found")
                })?;
            Ok(json!({"deleted": true, "workflow": workflow_payload(&stored)?}))
        }
        AdkMutationOperation::CreateWorkflowTrigger => {
            let workflow_id = required_identifier(input, "workflowId")?;
            let Some(workflow) = port
                .store
                .get_workflow(&workflow_id)
                .map_err(storage_mutation_failed)?
            else {
                return Err(not_found_mutation(
                    "ADK_WORKFLOW_NOT_FOUND",
                    "workflow not found",
                ));
            };
            if is_deleted_payload(&workflow.payload_json)? {
                return Err(not_found_mutation(
                    "ADK_WORKFLOW_NOT_FOUND",
                    "workflow not found",
                ));
            }
            let body = object_body(&input.body, "workflow trigger")?;
            let trigger_type = normalize_trigger_type(body.get("type"), "manual");
            let config = body
                .get("config")
                .filter(|value| !value.is_null())
                .cloned()
                .unwrap_or_else(|| json!({}));
            validate_trigger_config(&trigger_type, &config)?;
            let status = normalize_trigger_status(body.get("status"), "ENABLED");
            let title = body
                .get("title")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(|value| value.chars().take(120).collect::<String>())
                .unwrap_or_else(|| default_trigger_title(&trigger_type).to_owned());
            let id = normalize_id(&normalized_string(body.get("id")));
            let id = if id.is_empty() {
                next_id("workflow-trigger", &TRIGGER_ID_SEQUENCE)
            } else {
                id
            };
            let mut payload = Value::Object(body);
            let object = payload.as_object_mut().expect("object payload");
            object.insert("id".to_owned(), Value::String(id.clone()));
            object.insert("workflowId".to_owned(), Value::String(workflow_id.clone()));
            object.insert("type".to_owned(), Value::String(trigger_type.clone()));
            object.insert("title".to_owned(), Value::String(title));
            object.insert("status".to_owned(), Value::String(status.clone()));
            object.insert("config".to_owned(), config);
            object.insert("nextRunAt".to_owned(), Value::String(String::new()));
            let secret = if trigger_type == "webhook" {
                let (secret, hash) = generate_webhook_secret()?;
                object.insert("secretHash".to_owned(), Value::String(hash));
                object.insert("hasSecret".to_owned(), Value::Bool(true));
                secret
            } else {
                object.remove("secretHash");
                object.insert("hasSecret".to_owned(), Value::Bool(false));
                String::new()
            };
            let stored = port
                .store
                .upsert_workflow_trigger(
                    &id,
                    &workflow_id,
                    &trigger_type,
                    &status,
                    "",
                    &payload.to_string(),
                )
                .map_err(storage_mutation_failed)?;
            trigger_result(&stored, secret)
        }
        AdkMutationOperation::UpdateWorkflowTrigger => {
            let workflow_id = required_identifier(input, "workflowId")?;
            let trigger_id = required_identifier(input, "triggerId")?;
            let Some(workflow) = port
                .store
                .get_workflow(&workflow_id)
                .map_err(storage_mutation_failed)?
            else {
                return Err(not_found_mutation(
                    "ADK_WORKFLOW_NOT_FOUND",
                    "workflow not found",
                ));
            };
            if is_deleted_payload(&workflow.payload_json)? {
                return Err(not_found_mutation(
                    "ADK_WORKFLOW_NOT_FOUND",
                    "workflow not found",
                ));
            }
            let Some(existing) = port
                .store
                .get_workflow_trigger(&trigger_id)
                .map_err(storage_mutation_failed)?
            else {
                return Err(not_found_mutation(
                    "ADK_WORKFLOW_TRIGGER_NOT_FOUND",
                    "workflow trigger not found",
                ));
            };
            if existing.workflow_id != workflow_id || is_deleted_payload(&existing.payload_json)? {
                return Err(not_found_mutation(
                    "ADK_WORKFLOW_TRIGGER_NOT_FOUND",
                    "workflow trigger not found",
                ));
            }
            let body = object_body(&input.body, "workflow trigger")?;
            let mut payload = decode_mutation_payload(&existing.payload_json, "workflow trigger")?;
            let object = payload
                .as_object_mut()
                .ok_or_else(|| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: "stored ADK workflow trigger payload must be a JSON object".to_owned(),
                })?;
            let trigger_type = normalize_trigger_type(body.get("type"), &existing.trigger_type);
            let config = body
                .get("config")
                .filter(|value| !value.is_null())
                .cloned()
                .or_else(|| object.get("config").cloned())
                .unwrap_or_else(|| json!({}));
            validate_trigger_config(&trigger_type, &config)?;
            let status = normalize_trigger_status(body.get("status"), &existing.status);
            let title = body
                .get("title")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(|value| value.chars().take(120).collect::<String>())
                .or_else(|| {
                    object
                        .get("title")
                        .and_then(Value::as_str)
                        .map(str::trim)
                        .filter(|value| !value.is_empty())
                        .map(str::to_owned)
                })
                .unwrap_or_else(|| default_trigger_title(&trigger_type).to_owned());
            let reset_secret = body
                .get("resetSecret")
                .and_then(Value::as_bool)
                .unwrap_or(false);
            object.insert("id".to_owned(), Value::String(trigger_id.clone()));
            object.insert("workflowId".to_owned(), Value::String(workflow_id.clone()));
            object.insert("type".to_owned(), Value::String(trigger_type.clone()));
            object.insert("title".to_owned(), Value::String(title));
            object.insert("status".to_owned(), Value::String(status.clone()));
            object.insert("config".to_owned(), config);
            let secret = if trigger_type == "webhook"
                && (reset_secret
                    || object
                        .get("secretHash")
                        .and_then(Value::as_str)
                        .is_none_or(|value| value.trim().is_empty()))
            {
                let (secret, hash) = generate_webhook_secret()?;
                object.insert("secretHash".to_owned(), Value::String(hash));
                object.insert("hasSecret".to_owned(), Value::Bool(true));
                secret
            } else if trigger_type == "webhook" {
                object.insert("hasSecret".to_owned(), Value::Bool(true));
                String::new()
            } else {
                object.remove("secretHash");
                object.insert("hasSecret".to_owned(), Value::Bool(false));
                String::new()
            };
            let expected = expected_updated_at(&body, &existing.updated_at)?;
            let payload_json = payload.to_string();
            let changed = port
                .store
                .update_workflow_trigger_if_revision(
                    &trigger_id,
                    &expected,
                    &workflow_id,
                    &trigger_type,
                    &status,
                    &existing.next_run_at,
                    &payload_json,
                )
                .map_err(storage_mutation_failed)?;
            if !changed {
                let current = port
                    .store
                    .get_workflow_trigger(&trigger_id)
                    .map_err(storage_mutation_failed)?
                    .ok_or_else(|| {
                        not_found_mutation(
                            "ADK_WORKFLOW_TRIGGER_NOT_FOUND",
                            "workflow trigger not found",
                        )
                    })?;
                if is_deleted_payload(&current.payload_json)? {
                    return Err(not_found_mutation(
                        "ADK_WORKFLOW_TRIGGER_NOT_FOUND",
                        "workflow trigger not found",
                    ));
                }
                return Err(revision_conflict("WORKFLOW_TRIGGER"));
            }
            let stored = port
                .store
                .get_workflow_trigger(&trigger_id)
                .map_err(storage_mutation_failed)?
                .ok_or_else(|| {
                    not_found_mutation(
                        "ADK_WORKFLOW_TRIGGER_NOT_FOUND",
                        "workflow trigger not found",
                    )
                })?;
            trigger_result(&stored, secret)
        }
        AdkMutationOperation::DeleteWorkflowTrigger => {
            let workflow_id = required_identifier(input, "workflowId")?;
            let trigger_id = required_identifier(input, "triggerId")?;
            let Some(workflow) = port
                .store
                .get_workflow(&workflow_id)
                .map_err(storage_mutation_failed)?
            else {
                return Err(not_found_mutation(
                    "ADK_WORKFLOW_NOT_FOUND",
                    "workflow not found",
                ));
            };
            if is_deleted_payload(&workflow.payload_json)? {
                return Err(not_found_mutation(
                    "ADK_WORKFLOW_NOT_FOUND",
                    "workflow not found",
                ));
            }
            let Some(existing) = port
                .store
                .get_workflow_trigger(&trigger_id)
                .map_err(storage_mutation_failed)?
            else {
                return Err(not_found_mutation(
                    "ADK_WORKFLOW_TRIGGER_NOT_FOUND",
                    "workflow trigger not found",
                ));
            };
            if existing.workflow_id != workflow_id || is_deleted_payload(&existing.payload_json)? {
                return Err(not_found_mutation(
                    "ADK_WORKFLOW_TRIGGER_NOT_FOUND",
                    "workflow trigger not found",
                ));
            }
            let mut payload = decode_mutation_payload(&existing.payload_json, "workflow trigger")?;
            let object = payload
                .as_object_mut()
                .ok_or_else(|| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: "stored ADK workflow trigger payload must be a JSON object".to_owned(),
                })?;
            object.insert("status".to_owned(), Value::String("DISABLED".to_owned()));
            let deleted_at = now_rfc3339();
            object.insert("deletedAt".to_owned(), Value::String(deleted_at));
            let expected = expected_updated_at(
                input.body.as_object().unwrap_or(&Map::new()),
                &existing.updated_at,
            )?;
            let changed = port
                .store
                .soft_delete_workflow_trigger_if_revision(
                    &trigger_id,
                    &expected,
                    &workflow_id,
                    &payload.to_string(),
                )
                .map_err(storage_mutation_failed)?;
            if !changed {
                let current = port
                    .store
                    .get_workflow_trigger(&trigger_id)
                    .map_err(storage_mutation_failed)?
                    .ok_or_else(|| {
                        not_found_mutation(
                            "ADK_WORKFLOW_TRIGGER_NOT_FOUND",
                            "workflow trigger not found",
                        )
                    })?;
                if is_deleted_payload(&current.payload_json)? {
                    return Err(not_found_mutation(
                        "ADK_WORKFLOW_TRIGGER_NOT_FOUND",
                        "workflow trigger not found",
                    ));
                }
                return Err(revision_conflict("WORKFLOW_TRIGGER"));
            }
            let stored = port
                .store
                .get_workflow_trigger(&trigger_id)
                .map_err(storage_mutation_failed)?
                .ok_or_else(|| {
                    not_found_mutation(
                        "ADK_WORKFLOW_TRIGGER_NOT_FOUND",
                        "workflow trigger not found",
                    )
                })?;
            Ok(json!({
                "deleted": true,
                "trigger": workflow_trigger_payload(&stored)?,
            }))
        }
        _ => unreachable!("operation group checked before dispatch"),
    }
}
