//! Production ADK entities mutation dispatch.

use super::*;

pub(super) fn handles(operation: AdkMutationOperation) -> bool {
    matches!(
        operation,
        AdkMutationOperation::CreateAgent
            | AdkMutationOperation::UpdateAgent
            | AdkMutationOperation::DeleteAgent
            | AdkMutationOperation::CreateProvider
            | AdkMutationOperation::UpdateProvider
            | AdkMutationOperation::DeleteProvider
            | AdkMutationOperation::SetDefaultProvider
            | AdkMutationOperation::CreateMemory
            | AdkMutationOperation::DeleteMemory
            | AdkMutationOperation::DeleteSkill
            | AdkMutationOperation::CreateSession
            | AdkMutationOperation::DeleteSession
            | AdkMutationOperation::UpdateSessionComposerState
    )
}

pub(super) fn dispatch(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    debug_assert!(handles(input.operation));
    match input.operation {
            AdkMutationOperation::CreateAgent => {
                let id = input
                    .body
                    .get("id")
                    .and_then(Value::as_str)
                    .or_else(|| input.body.get("name").and_then(Value::as_str))
                    .map(normalize_id)
                    .filter(|value| !value.is_empty())
                    .unwrap_or_else(|| next_id("agent", &AGENT_ID_SEQUENCE));
                let mut payload = new_entity_payload(&input.body, "agent", &id)?;
                let object = payload.as_object_mut().ok_or_else(|| {
                    invalid_mutation_input("invalid agent payload")
                })?;
                object
                    .entry("name".to_owned())
                    .or_insert_with(|| Value::String(id.clone()));
                object
                    .entry("status".to_owned())
                    .or_insert_with(|| Value::String("ENABLED".to_owned()));
                let stored = port.store
                    .upsert_agent(&id, &payload.to_string())
                    .map_err(|e| AdkMutationPortError::Failed {
                        status: 400,
                        code: "ADK_MUTATION_FAILED".to_owned(),
                        message: e.to_string(),
                    })?;
                decode_mutation_payload(&stored.payload_json, "agent")
            }
            AdkMutationOperation::UpdateAgent => {
                let id = required_identifier(input, "agentId")?;
                if id == "jftrade-default" {
                    return Err(AdkMutationPortError::Failed {
                        status: 409,
                        code: "ADK_AGENT_PROTECTED".to_owned(),
                        message: "the built-in agent cannot be modified".to_owned(),
                    });
                }
                if port
                    .store
                    .get_agent(&id)
                    .map_err(storage_mutation_failed)?
                    .is_none()
                {
                    return Err(not_found_mutation(
                        "ADK_AGENT_NOT_FOUND",
                        "agent not found",
                    ));
                }
                let existing = port
                    .store
                    .get_agent(&id)
                    .map_err(storage_mutation_failed)?
                    .ok_or_else(|| {
                        not_found_mutation("ADK_AGENT_NOT_FOUND", "agent not found")
                    })?;
                let payload = merged_entity_payload(&existing, &input.body, "agent")?;
                let stored = port.store
                    .upsert_agent(&id, &payload.to_string())
                    .map_err(|e| AdkMutationPortError::Failed {
                        status: 400,
                        code: "ADK_MUTATION_FAILED".to_owned(),
                        message: e.to_string(),
                    })?;
                decode_mutation_payload(&stored.payload_json, "agent")
            }
            AdkMutationOperation::DeleteAgent => {
                let id = required_identifier(input, "agentId")?;
                if id == "jftrade-default" {
                    return Err(AdkMutationPortError::Failed {
                        status: 409,
                        code: "ADK_AGENT_PROTECTED".to_owned(),
                        message: "the built-in agent cannot be deleted".to_owned(),
                    });
                }
                let deleted = port.store
                    .delete_agent(&id)
                    .map_err(|e| AdkMutationPortError::Failed {
                        status: 400,
                        code: "ADK_MUTATION_FAILED".to_owned(),
                        message: e.to_string(),
                    })?;
                if !deleted {
                    return Err(AdkMutationPortError::Failed {
                        status: 404,
                        code: "ADK_AGENT_NOT_FOUND".to_owned(),
                        message: "agent not found".to_owned(),
                    });
                }
                Ok(json!({"deleted": true}))
            }
            AdkMutationOperation::CreateProvider | AdkMutationOperation::UpdateProvider => {
                let is_update = input.operation == AdkMutationOperation::UpdateProvider;
                let id = input
                    .identifiers
                    .get("providerId")
                    .map(String::as_str)
                    .or_else(|| input.body.get("id").and_then(Value::as_str))
                    .or_else(|| input.body.get("displayName").and_then(Value::as_str))
                    .map(normalize_id)
                    .filter(|value| !value.is_empty())
                    .ok_or_else(|| invalid_mutation_input("providerId or displayName is required"))?;
                let existing = port
                    .store
                    .get_provider(&id)
                    .map_err(storage_mutation_failed)?;
                if is_update && existing.is_none() {
                    return Err(not_found_mutation("ADK_PROVIDER_NOT_FOUND", "provider not found"));
                }
                let payload = match existing.as_ref() {
                    Some(existing) => merged_entity_payload(existing, &input.body, "provider")?,
                    None => new_entity_payload(&input.body, "provider", &id)?,
                };
                let stored = port.store
                    .upsert_provider(&id, &payload.to_string())
                    .map_err(|e| AdkMutationPortError::Failed {
                        status: 400,
                        code: "ADK_MUTATION_FAILED".to_owned(),
                        message: e.to_string(),
                    })?;
                decode_mutation_payload(&stored.payload_json, "provider")
            }
            AdkMutationOperation::DeleteProvider => {
                let id = required_identifier(input, "providerId")?;
                let deleted = port.store
                    .delete_provider(&id)
                    .map_err(|e| AdkMutationPortError::Failed {
                        status: 400,
                        code: "ADK_MUTATION_FAILED".to_owned(),
                        message: e.to_string(),
                    })?;
                if !deleted {
                    return Err(AdkMutationPortError::Failed {
                        status: 404,
                        code: "ADK_PROVIDER_NOT_FOUND".to_owned(),
                        message: "provider not found".to_owned(),
                    });
                }
                Ok(json!({"deleted": true}))
            }
            AdkMutationOperation::SetDefaultProvider => {
                let id = required_identifier(input, "providerId")?;
                if port
                    .store
                    .get_provider(&id)
                    .map_err(storage_mutation_failed)?
                    .is_none()
                {
                    return Err(not_found_mutation(
                        "ADK_PROVIDER_NOT_FOUND",
                        "provider not found",
                    ));
                }
                let providers = port
                    .store
                    .list_providers()
                    .map_err(storage_mutation_failed)?;
                let mut selected = None;
                for provider in providers {
                    let mut value = decode_mutation_payload(&provider.payload_json, "provider")?;
                    let object = value.as_object_mut().ok_or_else(|| {
                        AdkMutationPortError::Failed {
                            status: 500,
                            code: "ADK_STORAGE_CORRUPT".to_owned(),
                            message: "stored ADK provider payload must be a JSON object".to_owned(),
                        }
                    })?;
                    let is_default = provider.id == id;
                    object.insert("id".to_owned(), Value::String(provider.id.clone()));
                    object.insert("default".to_owned(), Value::Bool(is_default));
                    let stored = port
                        .store
                        .upsert_provider(&provider.id, &value.to_string())
                        .map_err(storage_mutation_failed)?;
                    if is_default {
                        selected = Some(stored);
                    }
                }
                let selected = selected.ok_or_else(|| {
                    not_found_mutation("ADK_PROVIDER_NOT_FOUND", "provider not found")
                })?;
                object_payload(&selected, "provider")
            }
            AdkMutationOperation::CreateMemory => {
                let body = object_body(&input.body, "memory")?;
                let scope = body
                    .get("scope")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|value| !value.is_empty())
                    .unwrap_or("workspace")
                    .to_ascii_lowercase();
                if scope != "workspace" && scope != "agent" {
                    return Err(invalid_mutation_input(
                        "memory scope must be workspace or agent",
                    ));
                }
                let agent_id = normalized_string(body.get("agentId"));
                if scope == "agent" {
                    if agent_id.is_empty() {
                        return Err(invalid_mutation_input("agent memory requires agentId"));
                    }
                    if port
                        .store
                        .get_agent(&agent_id)
                        .map_err(storage_mutation_failed)?
                        .is_none()
                    {
                        return Err(not_found_mutation(
                            "ADK_AGENT_NOT_FOUND",
                            "agent not found",
                        ));
                    }
                }
                let key = normalize_memory_key(&normalized_string(body.get("key")));
                if key.is_empty() {
                    return Err(invalid_mutation_input("memory key is required"));
                }
                let value = bounded_text(&normalized_string(body.get("value")));
                let id = normalize_id(&format!("{scope}-{agent_id}-{key}"));
                let payload = json!({
                    "id": id,
                    "agentId": agent_id,
                    "key": key,
                    "value": value,
                    "scope": scope,
                });
                let stored = port
                    .store
                    .upsert_memory(&id, &agent_id, &scope, &key, &payload.to_string())
                    .map_err(storage_mutation_failed)?;
                memory_payload(&stored)
            }
            AdkMutationOperation::DeleteMemory => {
                let id = required_identifier(input, "memoryId")?;
                let deleted = port
                    .store
                    .delete_memory(&id)
                    .map_err(storage_mutation_failed)?;
                if !deleted {
                    return Err(not_found_mutation(
                        "ADK_MEMORY_NOT_FOUND",
                        "memory not found",
                    ));
                }
                Ok(json!({"id": id, "deleted": true}))
            }
            AdkMutationOperation::DeleteSkill => {
                let id = required_identifier(input, "skillId")?;
                let deleted = port
                    .store
                    .delete_skill(&id)
                    .map_err(storage_mutation_failed)?;
                if !deleted {
                    return Err(not_found_mutation(
                        "ADK_SKILL_NOT_FOUND",
                        "skill not found",
                    ));
                }
                Ok(json!({"id": id, "deleted": true}))
            }
            AdkMutationOperation::CreateSession => {
                let agent_id = required_body_string(&input.body, "agentId")?;
                validate_session_agent(port, &agent_id)?;
                let id = next_session_id();
                let title = bounded_title(input.body.get("title"));
                let payload = json!({
                    "id": id,
                    "agentId": agent_id,
                    "title": title,
                });
                let stored = port
                    .store
                    .upsert_session(&id, &agent_id, &payload.to_string())
                    .map_err(session_mutation_failed)?;
                session_entity_value(&stored)
            }
            AdkMutationOperation::DeleteSession => {
                let id = required_identifier(input, "sessionId")?;
                let deleted = port.store.delete_session(&id).map_err(session_mutation_failed)?;
                if !deleted {
                    return Err(AdkMutationPortError::Failed {
                        status: 404,
                        code: "ADK_SESSION_NOT_FOUND".to_owned(),
                        message: "session not found".to_owned(),
                    });
                }
                Ok(json!({"deleted": true}))
            }
            AdkMutationOperation::UpdateSessionComposerState => {
                let id = required_identifier(input, "sessionId")?;
                if port
                    .store
                    .get_session(&id)
                    .map_err(session_mutation_failed)?
                    .is_none()
                {
                    return Err(AdkMutationPortError::Failed {
                        status: 404,
                        code: "NOT_FOUND".to_owned(),
                        message: "session not found".to_owned(),
                    });
                }
                let mut state = composer_state_value(
                    &id,
                    port.store
                        .get_session_composer_state(&id)
                        .map_err(session_mutation_failed)?,
                )?;
                let object = state.as_object_mut().ok_or_else(|| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: "stored ADK composer state must be a JSON object".to_owned(),
                })?;
                if let Some(value) = composer_string(&input.body, "chatDraft")? {
                    object.insert("chatDraft".to_owned(), Value::String(bounded_text(&value)));
                }
                for key in ["providerIdOverride", "modelOverride"] {
                    if let Some(value) = composer_string(&input.body, key)? {
                        object.insert(
                            key.to_owned(),
                            Value::String(value.trim().to_owned()),
                        );
                    }
                }
                if let Some(value) = composer_string(&input.body, "reasoningEffortOverride")? {
                    let value = validate_optional_composer_mode(
                        &value,
                        &["low", "medium", "high", "xhigh", "max"],
                    )?;
                    object.insert("reasoningEffortOverride".to_owned(), Value::String(value));
                }
                if let Some(value) = composer_string(&input.body, "workModeOverride")? {
                    let value = validate_optional_composer_mode(&value, &["chat", "loop"])?;
                    object.insert("workModeOverride".to_owned(), Value::String(value));
                }
                if let Some(value) = composer_string(&input.body, "permissionModeOverride")? {
                    let value = validate_optional_composer_mode(
                        &value,
                        &["approval", "less_approval", "all"],
                    )?;
                    object.insert("permissionModeOverride".to_owned(), Value::String(value));
                }
                if let Some(value) = composer_string(&input.body, "goalObjectiveDraft")? {
                    object.insert(
                        "goalObjectiveDraft".to_owned(),
                        Value::String(bounded_text(&value)),
                    );
                }
                if let Some(value) = composer_bool(&input.body, "goalObjectiveTouched")? {
                    object.insert("goalObjectiveTouched".to_owned(), Value::Bool(value));
                }
                let stored = port
                    .store
                    .upsert_session_composer_state(&id, &state.to_string())
                    .map_err(session_mutation_failed)?;
                composer_state_value(&id, Some(stored))
            }
        _ => unreachable!("operation group checked before dispatch"),
    }
}
