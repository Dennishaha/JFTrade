//! ADK mutation production adapter.

use std::sync::atomic::AtomicU64;

use serde_json::{Map, Value, json};
use sha2::{Digest, Sha256};
use time::{OffsetDateTime, format_description::well_known::Rfc3339};

use crate::product::product_adk_mutation_port::{
    AdkMutationInput, AdkMutationOperation, AdkMutationPort, AdkMutationPortError,
};
use super::ProductionAdkPort;

#[path = "product_production_ports_adk_mutation_helpers.rs"]
mod helpers;
#[path = "product_production_ports_adk_mutation_entities.rs"]
mod entities;
#[path = "product_production_ports_adk_mutation_workflows.rs"]
mod workflows;
#[path = "product_production_ports_adk_mutation_runs.rs"]
mod runs;
#[path = "product_production_ports_adk_mutation_tasks.rs"]
mod tasks;

use helpers::*;

static SESSION_ID_SEQUENCE: AtomicU64 = AtomicU64::new(1);
static AGENT_ID_SEQUENCE: AtomicU64 = AtomicU64::new(1);
static TASK_ID_SEQUENCE: AtomicU64 = AtomicU64::new(1);
static TRIGGER_ID_SEQUENCE: AtomicU64 = AtomicU64::new(1);
static WORKFLOW_ID_SEQUENCE: AtomicU64 = AtomicU64::new(1);


fn object_payload(
    stored: &jftrade_store_sqlite::StoredAdkEntity,
    resource: &str,
) -> Result<Value, AdkMutationPortError> {
    let mut value = decode_mutation_payload(&stored.payload_json, resource)?;
    let object = value.as_object_mut().ok_or_else(|| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_STORAGE_CORRUPT".to_owned(),
        message: format!("stored ADK {resource} payload must be a JSON object"),
    })?;
    object.insert("id".to_owned(), Value::String(stored.id.clone()));
    object.insert("createdAt".to_owned(), Value::String(stored.created_at.clone()));
    object.insert("updatedAt".to_owned(), Value::String(stored.updated_at.clone()));
    Ok(value)
}

fn memory_payload(
    stored: &jftrade_store_sqlite::StoredAdkMemory,
) -> Result<Value, AdkMutationPortError> {
    let mut value = decode_mutation_payload(&stored.payload_json, "memory")?;
    let object = value.as_object_mut().ok_or_else(|| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_STORAGE_CORRUPT".to_owned(),
        message: "stored ADK memory payload must be a JSON object".to_owned(),
    })?;
    object.insert("id".to_owned(), Value::String(stored.id.clone()));
    object.insert("agentId".to_owned(), Value::String(stored.agent_id.clone()));
    object.insert("scope".to_owned(), Value::String(stored.scope.clone()));
    object.insert("key".to_owned(), Value::String(stored.memory_key.clone()));
    object.insert("createdAt".to_owned(), Value::String(stored.created_at.clone()));
    object.insert("updatedAt".to_owned(), Value::String(stored.updated_at.clone()));
    Ok(value)
}

fn task_payload(
    stored: &jftrade_store_sqlite::StoredAdkTask,
) -> Result<Value, AdkMutationPortError> {
    let mut value = decode_mutation_payload(&stored.payload_json, "task")?;
    let object = value.as_object_mut().ok_or_else(|| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_STORAGE_CORRUPT".to_owned(),
        message: "stored ADK task payload must be a JSON object".to_owned(),
    })?;
    object.insert("id".to_owned(), Value::String(stored.id.clone()));
    object.insert("status".to_owned(), Value::String(stored.status.clone()));
    object.insert("agentId".to_owned(), Value::String(stored.agent_id.clone()));
    object.insert("runId".to_owned(), Value::String(stored.run_id.clone()));
    object.insert("createdAt".to_owned(), Value::String(stored.created_at.clone()));
    object.insert("updatedAt".to_owned(), Value::String(stored.updated_at.clone()));
    Ok(value)
}

fn optimization_payload(
    stored: &jftrade_store_sqlite::StoredAdkEntity,
) -> Result<Value, AdkMutationPortError> {
    object_payload(stored, "optimization task")
}

fn workflow_trigger_payload(
    stored: &jftrade_store_sqlite::StoredAdkWorkflowTrigger,
) -> Result<Value, AdkMutationPortError> {
    let mut value = decode_mutation_payload(&stored.payload_json, "workflow trigger")?;
    let object = value.as_object_mut().ok_or_else(|| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_STORAGE_CORRUPT".to_owned(),
        message: "stored ADK workflow trigger payload must be a JSON object".to_owned(),
    })?;
    object.insert("id".to_owned(), Value::String(stored.id.clone()));
    object.insert("workflowId".to_owned(), Value::String(stored.workflow_id.clone()));
    object.insert("type".to_owned(), Value::String(stored.trigger_type.clone()));
    object.insert("status".to_owned(), Value::String(stored.status.clone()));
    object.insert("nextRunAt".to_owned(), Value::String(stored.next_run_at.clone()));
    object.insert("createdAt".to_owned(), Value::String(stored.created_at.clone()));
    object.insert("updatedAt".to_owned(), Value::String(stored.updated_at.clone()));
    let has_secret = object
        .get("hasSecret")
        .and_then(Value::as_bool)
        .unwrap_or(false)
        || object
            .get("secretHash")
            .and_then(Value::as_str)
            .is_some_and(|value| !value.trim().is_empty());
    object.remove("secretHash");
    object.insert("hasSecret".to_owned(), Value::Bool(has_secret));
    Ok(value)
}

fn workflow_payload(
    stored: &jftrade_store_sqlite::StoredAdkWorkflow,
) -> Result<Value, AdkMutationPortError> {
    let mut value = decode_mutation_payload(&stored.payload_json, "workflow")?;
    let object = value.as_object_mut().ok_or_else(|| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_STORAGE_CORRUPT".to_owned(),
        message: "stored ADK workflow payload must be a JSON object".to_owned(),
    })?;
    object.insert("id".to_owned(), Value::String(stored.id.clone()));
    object.insert("status".to_owned(), Value::String(stored.status.clone()));
    object.insert("createdAt".to_owned(), Value::String(stored.created_at.clone()));
    object.insert("updatedAt".to_owned(), Value::String(stored.updated_at.clone()));
    Ok(value)
}

fn now_rfc3339() -> String {
    OffsetDateTime::now_utc()
        .format(&Rfc3339)
        .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
}

fn run_entity_value(
    stored: &jftrade_store_sqlite::StoredAdkRun,
) -> Result<Value, AdkMutationPortError> {
    let mut value = decode_mutation_payload(&stored.payload_json, "run")?;
    let object = value.as_object_mut().ok_or_else(|| {
        AdkMutationPortError::Failed {
            status: 500,
            code: "ADK_STORAGE_CORRUPT".to_owned(),
            message: "stored ADK run payload must be a JSON object".to_owned(),
        }
    })?;
    object.insert("id".to_owned(), Value::String(stored.id.clone()));
    object.insert("status".to_owned(), Value::String(stored.status.clone()));
    object.insert(
        "sessionId".to_owned(),
        Value::String(stored.session_id.clone()),
    );
    object.insert("agentId".to_owned(), Value::String(stored.agent_id.clone()));
    object.insert(
        "createdAt".to_owned(),
        Value::String(stored.created_at.clone()),
    );
    object.insert(
        "updatedAt".to_owned(),
        Value::String(stored.updated_at.clone()),
    );
    Ok(value)
}

fn approval_entity_value(
    stored: &jftrade_store_sqlite::StoredAdkApproval,
) -> Result<Value, AdkMutationPortError> {
    let mut value = decode_mutation_payload(&stored.payload_json, "approval")?;
    let object = value.as_object_mut().ok_or_else(|| {
        AdkMutationPortError::Failed {
            status: 500,
            code: "ADK_STORAGE_CORRUPT".to_owned(),
            message: "stored ADK approval payload must be a JSON object".to_owned(),
        }
    })?;
    object.insert("id".to_owned(), Value::String(stored.id.clone()));
    object.insert("runId".to_owned(), Value::String(stored.run_id.clone()));
    object.insert("agentId".to_owned(), Value::String(stored.agent_id.clone()));
    object.insert("status".to_owned(), Value::String(stored.status.clone()));
    object.insert(
        "createdAt".to_owned(),
        Value::String(stored.created_at.clone()),
    );
    object.insert(
        "updatedAt".to_owned(),
        Value::String(stored.updated_at.clone()),
    );
    Ok(value)
}

fn approval_resolution_value(
    resolution: &jftrade_store_sqlite::AdkApprovalResolution,
) -> Result<Value, AdkMutationPortError> {
    let mut value = Map::new();
    value.insert(
        "approval".to_owned(),
        approval_entity_value(&resolution.approval)?,
    );
    if let Some(run) = resolution.run.as_ref() {
        value.insert("run".to_owned(), run_entity_value(run)?);
    }
    Ok(Value::Object(value))
}

fn run_state_result(
    port: &ProductionAdkPort,
    id: &str,
    status: &str,
    payload: &Value,
) -> Result<Value, AdkMutationPortError> {
    if !port
        .store
        .update_run_state(id, status, &payload.to_string())
        .map_err(storage_mutation_failed)?
    {
        return Err(not_found_mutation("ADK_RUN_NOT_FOUND", "run not found"));
    }
    let updated = port
        .store
        .get_run(id)
        .map_err(storage_mutation_failed)?
        .ok_or_else(|| not_found_mutation("ADK_RUN_NOT_FOUND", "run not found"))?;
    run_entity_value(&updated)
}

fn validate_goal_run(value: &Value, action: &str) -> Result<(), AdkMutationPortError> {
    let object = value.as_object().ok_or_else(|| {
        AdkMutationPortError::Failed {
            status: 500,
            code: "ADK_STORAGE_CORRUPT".to_owned(),
            message: "stored ADK run payload must be a JSON object".to_owned(),
        }
    })?;
    let work_mode = object
        .get("workMode")
        .and_then(Value::as_str)
        .map(str::trim)
        .unwrap_or_default();
    if !work_mode.eq_ignore_ascii_case("loop") {
        return Err(invalid_mutation_input(&format!(
            "only loop goal runs can be {action}"
        )));
    }
    if object
        .get("parentRunId")
        .and_then(Value::as_str)
        .is_some_and(|parent| !parent.trim().is_empty())
    {
        return Err(invalid_mutation_input(&format!(
            "only root goal runs can be {action}"
        )));
    }
    let workflow_status = object
        .get("workflowStatus")
        .and_then(Value::as_str)
        .map(str::trim)
        .unwrap_or_default();
    if workflow_status.is_empty() {
        return Err(invalid_mutation_input(&format!(
            "only loop goal runs can be {action}"
        )));
    }
    Ok(())
}

fn terminal_run_status(status: &str) -> bool {
    matches!(
        status.trim().to_ascii_uppercase().as_str(),
        "COMPLETED" | "FAILED" | "DENIED" | "CANCELLED" | "TIMED_OUT"
    )
}

fn default_trigger_title(trigger_type: &str) -> &'static str {
    match trigger_type {
        "schedule" => "定时触发",
        "webhook" => "Webhook",
        "event" => "事件触发",
        "market_threshold" => "行情阈值",
        _ => "手动触发",
    }
}

fn normalize_workflow_status(value: Option<&Value>, fallback: &str) -> String {
    let candidate = value
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or(fallback)
        .to_ascii_uppercase();
    if candidate == "DISABLED" {
        "DISABLED".to_owned()
    } else {
        "ENABLED".to_owned()
    }
}

fn normalize_workflow_mode(value: Option<&Value>, fallback: &str) -> String {
    let candidate = value
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or(fallback)
        .to_ascii_lowercase();
    if candidate == "chat" {
        "chat".to_owned()
    } else {
        "loop".to_owned()
    }
}

fn bounded_title(value: Option<&Value>) -> String {
    let title = value
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|title| !title.is_empty())
        .unwrap_or("新的 ADK 会话");
    title.chars().take(80).collect()
}

fn session_mutation_failed(error: impl std::fmt::Display) -> AdkMutationPortError {
    AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_SESSION_MUTATION_FAILED".to_owned(),
        message: error.to_string(),
    }
}

fn validate_session_agent(
    port: &ProductionAdkPort,
    agent_id: &str,
) -> Result<(), AdkMutationPortError> {
    if agent_id == "jftrade-default" {
        return Ok(());
    }
    let Some(agent) = port
        .store
        .get_agent(agent_id)
        .map_err(session_mutation_failed)?
    else {
        return Err(invalid_mutation_input("enabled agent is required"));
    };
    let value = decode_mutation_payload(&agent.payload_json, "agent")?;
    let status = value
        .get("status")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|status| !status.is_empty())
        .unwrap_or("ENABLED");
    let deleted = value.get("deletedAt").is_some_and(|deleted_at| {
        !deleted_at.is_null()
            && deleted_at
                .as_str()
                .map(str::trim)
                .is_none_or(|value| !value.is_empty())
    });
    if !status.eq_ignore_ascii_case("ENABLED") || deleted {
        return Err(invalid_mutation_input("enabled agent is required"));
    }
    Ok(())
}

fn session_entity_value(
    stored: &jftrade_store_sqlite::StoredAdkEntity,
) -> Result<Value, AdkMutationPortError> {
    let mut value = decode_mutation_payload(&stored.payload_json, "session")?;
    let object = value.as_object_mut().ok_or_else(|| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_STORAGE_CORRUPT".to_owned(),
        message: "stored ADK session payload must be a JSON object".to_owned(),
    })?;
    object.insert("id".to_owned(), Value::String(stored.id.clone()));
    object.insert(
        "createdAt".to_owned(),
        Value::String(stored.created_at.clone()),
    );
    object.insert(
        "updatedAt".to_owned(),
        Value::String(stored.updated_at.clone()),
    );
    Ok(value)
}

fn default_composer_state(session_id: &str) -> Value {
    json!({
        "sessionId": session_id,
        "chatDraft": "",
        "providerIdOverride": "",
        "modelOverride": "",
        "reasoningEffortOverride": "",
        "workModeOverride": "",
        "permissionModeOverride": "",
        "goalObjectiveDraft": "",
        "goalObjectiveTouched": false,
        "updatedAt": "",
    })
}

fn composer_state_value(
    session_id: &str,
    stored: Option<jftrade_store_sqlite::StoredAdkEntity>,
) -> Result<Value, AdkMutationPortError> {
    let updated_at = stored
        .as_ref()
        .map(|state| state.updated_at.clone())
        .unwrap_or_default();
    let mut value = match stored {
        Some(stored) => decode_mutation_payload(&stored.payload_json, "composer state")?,
        None => default_composer_state(session_id),
    };
    let object = value.as_object_mut().ok_or_else(|| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_STORAGE_CORRUPT".to_owned(),
        message: "stored ADK composer state must be a JSON object".to_owned(),
    })?;
    object.insert(
        "sessionId".to_owned(),
        Value::String(session_id.to_owned()),
    );
    for (key, default) in [
        ("chatDraft", Value::String(String::new())),
        ("providerIdOverride", Value::String(String::new())),
        ("modelOverride", Value::String(String::new())),
        ("reasoningEffortOverride", Value::String(String::new())),
        ("workModeOverride", Value::String(String::new())),
        ("permissionModeOverride", Value::String(String::new())),
        ("goalObjectiveDraft", Value::String(String::new())),
        ("goalObjectiveTouched", Value::Bool(false)),
        ("updatedAt", Value::String(updated_at.clone())),
    ] {
        object.entry(key.to_owned()).or_insert(default);
    }
    object.insert("updatedAt".to_owned(), Value::String(updated_at));
    Ok(value)
}

fn bounded_text(value: &str) -> String {
    value.chars().take(50_000).collect()
}

fn composer_string(
    body: &Value,
    key: &str,
) -> Result<Option<String>, AdkMutationPortError> {
    body.get(key)
        .map(|value| {
            value
                .as_str()
                .map(str::to_owned)
                .ok_or_else(|| invalid_mutation_input("invalid composer state payload"))
        })
        .transpose()
}

fn composer_bool(body: &Value, key: &str) -> Result<Option<bool>, AdkMutationPortError> {
    body.get(key)
        .map(|value| {
            value
                .as_bool()
                .ok_or_else(|| invalid_mutation_input("invalid composer state payload"))
        })
        .transpose()
}

fn validate_optional_composer_mode(
    value: &str,
    allowed: &[&str],
) -> Result<String, AdkMutationPortError> {
    let normalized = value.trim().to_lowercase();
    if normalized.is_empty() || allowed.iter().any(|candidate| *candidate == normalized) {
        Ok(normalized)
    } else {
        Err(invalid_mutation_input("invalid composer state payload"))
    }
}

fn task_status(value: Option<&Value>) -> Result<String, AdkMutationPortError> {
    let status = value
        .and_then(Value::as_str)
        .map(str::trim)
        .unwrap_or_default()
        .to_ascii_uppercase();
    let status = if status.is_empty() { "TODO" } else { status.as_str() };
    if matches!(status, "TODO" | "IN_PROGRESS" | "BLOCKED" | "DONE" | "CANCELLED") {
        Ok(status.to_owned())
    } else {
        Err(invalid_mutation_input("invalid task status"))
    }
}

fn string_slice(
    value: Option<&Value>,
    field: &str,
) -> Result<Vec<String>, AdkMutationPortError> {
    let Some(value) = value else {
        return Ok(Vec::new());
    };
    let Some(values) = value.as_array() else {
        return Err(invalid_mutation_input(&format!("{field} must be an array")));
    };
    values
        .iter()
        .map(|item| {
            item.as_str()
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(str::to_owned)
                .ok_or_else(|| invalid_mutation_input(&format!("{field} must contain strings")))
        })
        .collect()
}

fn reject_self_dependency(id: &str, depends_on: &[String]) -> Result<(), AdkMutationPortError> {
    if depends_on.iter().any(|dependency| normalize_id(dependency) == id) {
        Err(invalid_mutation_input("task cannot depend on itself"))
    } else {
        Ok(())
    }
}

fn normalize_trigger_type(value: Option<&Value>, fallback: &str) -> String {
    let candidate = value
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or(fallback)
        .to_ascii_lowercase();
    match candidate.as_str() {
        "schedule" | "webhook" | "event" | "market_threshold" => candidate,
        _ => "manual".to_owned(),
    }
}

fn normalize_trigger_status(value: Option<&Value>, fallback: &str) -> String {
    let candidate = value
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or(fallback)
        .to_ascii_uppercase();
    match candidate.as_str() {
        "DISABLED" | "ERROR" => candidate,
        _ => "ENABLED".to_owned(),
    }
}

fn validate_trigger_config(
    trigger_type: &str,
    config: &Value,
) -> Result<(), AdkMutationPortError> {
    let config = config
        .as_object()
        .ok_or_else(|| invalid_mutation_input("workflow trigger config must be an object"))?;
    match trigger_type {
        "schedule" if normalized_string(config.get("cron")).is_empty() => {
            Err(invalid_mutation_input("schedule trigger requires cron"))
        }
        "market_threshold" => {
            let instruments = config
                .get("instrumentIds")
                .and_then(Value::as_array)
                .filter(|values| !values.is_empty());
            if instruments.is_none() || !config.get("value").is_some_and(Value::is_number) {
                Err(invalid_mutation_input(
                    "market threshold trigger requires instrumentIds and numeric value",
                ))
            } else {
                Ok(())
            }
        }
        _ => Ok(()),
    }
}

fn generate_webhook_secret() -> Result<(String, String), AdkMutationPortError> {
    let mut bytes = [0_u8; 32];
    getrandom::fill(&mut bytes).map_err(|error| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_SECRET_GENERATION_FAILED".to_owned(),
        message: error.to_string(),
    })?;
    let secret = encode_hex(&bytes);
    let mut digest = Sha256::new();
    digest.update(secret.as_bytes());
    Ok((secret, encode_hex(&digest.finalize())))
}

fn encode_hex(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut output = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        output.push(HEX[(byte >> 4) as usize] as char);
        output.push(HEX[(byte & 0x0f) as usize] as char);
    }
    output
}

fn trigger_result(
    stored: &jftrade_store_sqlite::StoredAdkWorkflowTrigger,
    secret: String,
) -> Result<Value, AdkMutationPortError> {
    let trigger = workflow_trigger_payload(stored)?;
    let mut result = Map::new();
    result.insert("trigger".to_owned(), trigger);
    if !secret.is_empty() {
        result.insert("secret".to_owned(), Value::String(secret));
    }
    Ok(Value::Object(result))
}

fn update_string_field(object: &mut Map<String, Value>, body: &Map<String, Value>, key: &str) {
    if let Some(value) = body.get(key).and_then(Value::as_str) {
        object.insert(key.to_owned(), Value::String(value.trim().to_owned()));
    }
}

fn dispatch_mutation(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    if entities::handles(input.operation) {
        return entities::dispatch(port, input);
    }
    if workflows::handles(input.operation) {
        return workflows::dispatch(port, input);
    }
    if runs::handles(input.operation) {
        return runs::dispatch(port, input);
    }
    if tasks::handles(input.operation) {
        return tasks::dispatch(port, input);
    }
    Err(AdkMutationPortError::Unavailable(format!(
        "ADK operation {} is not supported in production without external assistant runtime",
        input.operation.name()
    )))
}

impl AdkMutationPort for ProductionAdkPort {
    fn mutate(&self, input: &AdkMutationInput) -> Result<Value, AdkMutationPortError> {
        dispatch_mutation(self, input)
    }
}
