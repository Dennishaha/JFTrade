//! Builtin agents keep their behavior while accepting provider configuration.

use serde_json::{Value, json};
use thiserror::Error;

const CONFIGURATION_FIELDS: [&str; 3] = ["providerId", "model", "reasoningEffort"];

#[derive(Debug, Error)]
pub enum BuiltinAgentConfigurationError {
    #[error(
        "the built-in agent is protected: only provider, model, and reasoning effort can be edited"
    )]
    Protected,
    #[error("invalid agent payload")]
    InvalidPayload,
    #[error("stored ADK agent payload must be a JSON object")]
    InvalidCurrentPayload,
}

/// Validate `validation_payload` against the provider catalog before saving `payload`.
pub struct BuiltinAgentConfiguration {
    pub payload: Value,
    pub validation_payload: Value,
}

/// Prepare a configuration change without publishing caller-supplied behavior fields.
pub fn prepare_builtin_agent_configuration(
    current: &Value,
    incoming: &Value,
) -> Result<BuiltinAgentConfiguration, BuiltinAgentConfigurationError> {
    let incoming_object = incoming
        .as_object()
        .ok_or(BuiltinAgentConfigurationError::InvalidPayload)?;
    let requested_configuration = CONFIGURATION_FIELDS
        .iter()
        .any(|key| incoming_object.contains_key(*key));
    let protected_change = incoming_object.keys().any(|key| {
        !CONFIGURATION_FIELDS.contains(&key.as_str())
            && !matches!(key.as_str(), "createdAt" | "updatedAt")
            && protected_value(incoming, key) != protected_value(current, key)
    });
    if !requested_configuration || protected_change {
        return Err(BuiltinAgentConfigurationError::Protected);
    }
    let mut payload = current.clone();
    let object = payload
        .as_object_mut()
        .ok_or(BuiltinAgentConfigurationError::InvalidCurrentPayload)?;
    let mut validation_object = object.clone();
    for (key, value) in incoming_object {
        if CONFIGURATION_FIELDS.contains(&key.as_str()) {
            if !value.is_null() && !value.is_string() {
                return Err(BuiltinAgentConfigurationError::InvalidPayload);
            }
            object.insert(key.clone(), value.clone());
        }
        if !matches!(key.as_str(), "id" | "createdAt" | "updatedAt") {
            validation_object.insert(key.clone(), value.clone());
        }
    }
    Ok(BuiltinAgentConfiguration {
        payload,
        validation_payload: Value::Object(validation_object),
    })
}

fn normalized_names(values: Vec<String>) -> Vec<String> {
    values
        .into_iter()
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
        .collect::<std::collections::BTreeSet<_>>()
        .into_iter()
        .collect()
}

fn protected_value(payload: &Value, key: &str) -> Value {
    let value = payload.get(key).unwrap_or(&Value::Null);
    match key {
        "tools" | "skills" => {
            if value.is_null() {
                return json!([]);
            }
            let Some(values) = value
                .as_array()
                .filter(|items| items.iter().all(Value::is_string))
            else {
                return value.clone();
            };
            json!(normalized_names(
                values
                    .iter()
                    .filter_map(Value::as_str)
                    .map(str::to_owned)
                    .collect()
            ))
        }
        "name" | "instruction" | "status" | "permissionMode" | "workMode" | "toolAccessMode" => {
            if !value.is_null() && !value.is_string() {
                return value.clone();
            }
            json!(protected_text(payload, key))
        }
        "memoryEnabled" => {
            if value.is_null() {
                json!(false)
            } else {
                value.clone()
            }
        }
        "recentUserWindow" | "loopMaxIterations" => {
            let count = if value.is_null() {
                0
            } else if let Some(count) = value.as_i64() {
                count
            } else {
                return value.clone();
            };
            let count = if key == "recentUserWindow" {
                if count <= 0 { 6 } else { count.clamp(2, 100) }
            } else if count <= 0 {
                5
            } else {
                count.min(20)
            };
            json!(count)
        }
        _ => value.clone(),
    }
}

fn protected_text(payload: &Value, key: &str) -> String {
    let text = payload
        .get(key)
        .and_then(Value::as_str)
        .unwrap_or_default()
        .trim();
    match key {
        "status" => {
            if text.is_empty() {
                "ENABLED".to_owned()
            } else {
                text.to_ascii_uppercase()
            }
        }
        "permissionMode" => match text.to_ascii_lowercase().as_str() {
            "less_approval" => "less_approval".to_owned(),
            "all" => "all".to_owned(),
            _ => "approval".to_owned(),
        },
        "workMode" => {
            if text.eq_ignore_ascii_case("loop") {
                "loop".to_owned()
            } else {
                "chat".to_owned()
            }
        }
        "toolAccessMode" => match text.to_ascii_lowercase().as_str() {
            "all" => "all".to_owned(),
            "selected" => "selected".to_owned(),
            "none" => "none".to_owned(),
            _ => {
                if payload
                    .get("tools")
                    .and_then(Value::as_array)
                    .is_some_and(|items| {
                        items
                            .iter()
                            .any(|item| item.as_str().is_some_and(|name| !name.trim().is_empty()))
                    })
                {
                    "selected".to_owned()
                } else {
                    "all".to_owned()
                }
            }
        },
        _ => text.to_owned(),
    }
}
