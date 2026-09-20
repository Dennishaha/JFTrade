//! Shared ADK mutation decoding and entity payload helpers.

use serde_json::{Map, Value};

use crate::product::product_adk_mutation_port::{AdkMutationInput, AdkMutationPortError};
use crate::product_id::generate_prefixed_id;

pub(super) fn decode_mutation_payload(
    raw: &str,
    resource: &str,
) -> Result<Value, AdkMutationPortError> {
    serde_json::from_str(raw).map_err(|error| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_STORAGE_CORRUPT".to_owned(),
        message: format!("stored ADK {resource} payload is invalid JSON: {error}"),
    })
}

/// `400 BAD_REQUEST`, the code every Go ADK handler uses for payload, route
/// identifier and business-rule rejections that are not routed through a
/// dedicated `ADK_*_FAILED` wrapper.
pub(super) fn invalid_mutation_input(message: &str) -> AdkMutationPortError {
    invalid_mutation_with_code("BAD_REQUEST", message)
}

pub(super) fn invalid_mutation_with_code(code: &str, message: &str) -> AdkMutationPortError {
    AdkMutationPortError::Failed {
        status: 400,
        code: code.to_owned(),
        message: message.to_owned(),
    }
}

pub(super) fn input_response_invalid(message: impl Into<String>) -> AdkMutationPortError {
    AdkMutationPortError::Failed {
        status: 400,
        code: "ADK_INPUT_RESPONSE_INVALID".to_owned(),
        message: message.into(),
    }
}

pub(super) fn input_response_conflict(message: impl Into<String>) -> AdkMutationPortError {
    AdkMutationPortError::Failed {
        status: 409,
        code: "ADK_INPUT_RESPONSE_CONFLICT".to_owned(),
        message: message.into(),
    }
}

pub(super) fn required_body_string(
    body: &Value,
    field: &str,
) -> Result<String, AdkMutationPortError> {
    body.get(field)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
        .ok_or_else(|| invalid_mutation_input(&format!("{field} is required")))
}

pub(super) fn required_identifier(
    input: &AdkMutationInput,
    field: &str,
) -> Result<String, AdkMutationPortError> {
    input
        .identifiers
        .get(field)
        .map(String::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
        .ok_or_else(|| invalid_mutation_input(&format!("{field} is required")))
}

pub(super) fn next_session_id() -> String {
    generate_prefixed_id("session")
}

pub(super) fn next_id(prefix: &str) -> String {
    generate_prefixed_id(prefix)
}

pub(crate) fn normalize_id(value: &str) -> String {
    let mut normalized = String::new();
    let mut last_dash = false;
    for character in value.trim().to_ascii_lowercase().chars() {
        if character.is_ascii_alphanumeric() || character == '_' || character == '-' {
            normalized.push(character);
            last_dash = false;
        } else if !last_dash {
            normalized.push('-');
            last_dash = true;
        }
    }
    normalized
        .trim_matches(|character| character == '-' || character == '_')
        .to_owned()
}

pub(super) fn normalize_memory_key(value: &str) -> String {
    normalize_id(value)
}

pub(super) fn normalized_string(value: Option<&Value>) -> String {
    value
        .and_then(Value::as_str)
        .map(str::trim)
        .unwrap_or_default()
        .to_owned()
}

pub(super) fn storage_mutation_failed(error: impl std::fmt::Display) -> AdkMutationPortError {
    AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_MUTATION_FAILED".to_owned(),
        message: error.to_string(),
    }
}

pub(super) fn not_found_mutation(code: &str, message: &str) -> AdkMutationPortError {
    AdkMutationPortError::Failed {
        status: 404,
        code: code.to_owned(),
        message: message.to_owned(),
    }
}

pub(super) fn object_body(
    value: &Value,
    resource: &str,
) -> Result<Map<String, Value>, AdkMutationPortError> {
    value
        .as_object()
        .cloned()
        .ok_or_else(|| invalid_mutation_input(&format!("invalid {resource} payload")))
}

/// Apply a partial update to a persisted ADK entity without discarding fields
/// that were not present in the request body.
///
/// The Go store accepts the same write DTO for create and update.  Its update
/// path preserves server-managed fields (id, timestamps and capabilities) and
/// normalizes the omitted values from the existing record.  Production Rust
/// must therefore merge object members before writing instead of replacing the
/// JSON document with a sparse PUT payload.
pub(super) fn merged_entity_payload(
    stored: &jftrade_store_sqlite::StoredAdkEntity,
    incoming: &Value,
    resource: &str,
) -> Result<Value, AdkMutationPortError> {
    let mut value = decode_mutation_payload(&stored.payload_json, resource)?;
    let object = value
        .as_object_mut()
        .ok_or_else(|| AdkMutationPortError::Failed {
            status: 500,
            code: "ADK_STORAGE_CORRUPT".to_owned(),
            message: format!("stored ADK {resource} payload must be a JSON object"),
        })?;
    let incoming = object_body(incoming, resource)?;
    for (key, member) in incoming {
        if key != "id" && key != "createdAt" && key != "updatedAt" {
            object.insert(key, member);
        }
    }
    object.insert("id".to_owned(), Value::String(stored.id.clone()));
    Ok(value)
}

pub(super) fn new_entity_payload(
    body: &Value,
    resource: &str,
    id: &str,
) -> Result<Value, AdkMutationPortError> {
    let mut object = object_body(body, resource)?;
    object.insert("id".to_owned(), Value::String(id.to_owned()));
    Ok(Value::Object(object))
}

pub(super) fn is_deleted_payload(raw: &str) -> Result<bool, AdkMutationPortError> {
    let value = decode_mutation_payload(raw, "resource")?;
    Ok(value.get("deletedAt").is_some_and(|deleted| {
        !deleted.is_null()
            && deleted
                .as_str()
                .map(str::trim)
                .is_none_or(|value| !value.is_empty())
    }))
}

/// Trim, drop blanks, deduplicate and sort a string list exactly like the
/// reference `NormalizeStringSlice`.  Task dependencies and planner warnings
/// are persisted in that canonical order, so the stored payload is stable no
/// matter how the caller ordered or repeated its input.
pub(super) fn normalized_string_slice(values: Vec<String>) -> Vec<String> {
    let mut values = values
        .into_iter()
        .map(|value| value.trim().to_owned())
        .filter(|value| !value.is_empty())
        .collect::<Vec<_>>();
    values.sort();
    values.dedup();
    values
}

pub(super) fn string_slice(value: Option<&Value>, field: &str) -> Result<Vec<String>, AdkMutationPortError> {
    let Some(value) = value else {
        return Ok(Vec::new());
    };
    let Some(values) = value.as_array() else {
        return Err(invalid_mutation_input(&format!("{field} must be an array")));
    };
    values
        .iter()
        .map(|item| {
            // A blank entry is not an error: the reference
            // `NormalizeStringSlice` silently drops it.  Only a non-string
            // member is a malformed payload.
            item.as_str()
                .map(str::trim)
                .map(str::to_owned)
                .ok_or_else(|| invalid_mutation_input(&format!("{field} must contain strings")))
        })
        .collect()
}

pub(super) fn reject_self_dependency(id: &str, depends_on: &[String]) -> Result<(), AdkMutationPortError> {
    if depends_on
        .iter()
        .any(|dependency| normalize_id(dependency) == id)
    {
        Err(invalid_mutation_input("task cannot depend on itself"))
    } else {
        Ok(())
    }
}
