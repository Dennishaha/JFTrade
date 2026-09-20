//! Projection helpers for the production ADK read port (context sizing and
//! provider credential redaction).

use std::fs;

use super::*;

/// Map a durable catalog listing fault onto the per-resource error code the
/// original handlers publish.  The transport-level `Unavailable` variant stays
/// reserved for a missing or unready port; a live store that fails to answer a
/// listing must never collapse into it.
pub(super) fn resource_list_failed(
    status: u16,
    code: &str,
    error: AdkStoreError,
) -> AdkReadSnapshotError {
    AdkReadSnapshotError::Failed {
        status,
        code: code.to_owned(),
        message: error.to_string(),
        retry_after_seconds: None,
    }
}

pub(crate) fn estimate_context_tokens(value: &str) -> usize {
    let bytes = value.trim().len();
    if bytes == 0 {
        0
    } else {
        bytes.saturating_add(3) / 4
    }
}

pub(super) fn is_context_user_event(event: &jftrade_store_sqlite::StoredAdkEvent) -> bool {
    event.author.trim().eq_ignore_ascii_case("user")
        || event.author.to_ascii_lowercase().contains("user")
}

pub(super) fn recent_context_event_start(
    events: &[jftrade_store_sqlite::StoredAdkEvent],
    window: usize,
) -> usize {
    let mut hits = 0;
    for index in (0..events.len()).rev() {
        if !is_context_user_event(&events[index]) {
            continue;
        }
        hits += 1;
        if hits >= window {
            return index;
        }
    }
    0
}

pub(super) fn protected_context_event_start(events: &[jftrade_store_sqlite::StoredAdkEvent]) -> usize {
    events
        .iter()
        .position(|event| {
            let content = event.content.to_ascii_lowercase();
            content.contains("approval")
                || content.contains("pending_input")
                || content.contains("pending approval")
                || content.contains("awaiting_input")
        })
        .unwrap_or(events.len())
}

pub(super) fn sanitize_provider(
    value: &mut Value,
    provider_id: &str,
    settings_path: &std::path::Path,
) -> Result<(), AdkReadSnapshotError> {
    let object = value.as_object_mut().ok_or_else(|| {
        invalid_payload(
            "provider",
            "stored ADK provider payload must be a JSON object",
        )
    })?;
    let payload_has_key = object
        .get("apiKey")
        .and_then(Value::as_str)
        .is_some_and(|key| !key.trim().is_empty());
    object.remove("apiKey");
    let secret_has_key = read_secret_presence(settings_path, provider_id)?;
    object.insert(
        "hasApiKey".to_owned(),
        Value::Bool(payload_has_key || secret_has_key),
    );
    Ok(())
}

pub(super) fn read_secret_presence(
    settings_path: &std::path::Path,
    provider_id: &str,
) -> Result<bool, AdkReadSnapshotError> {
    let path = std::env::var_os("JFTRADE_ADK_SECRETS")
        .filter(|value| !value.is_empty())
        .map(std::path::PathBuf::from)
        .unwrap_or_else(|| {
            settings_path
                .parent()
                .filter(|parent| !parent.as_os_str().is_empty())
                .map_or_else(
                    || std::path::PathBuf::from("secrets/adk-secrets.json"),
                    |parent| parent.join("secrets/adk-secrets.json"),
                )
        });
    let bytes = match fs::read(path) {
        Ok(bytes) => bytes,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(false),
        Err(error) => return Err(AdkReadSnapshotError::Unavailable(error.to_string())),
    };
    if bytes.iter().all(u8::is_ascii_whitespace) {
        return Ok(false);
    }
    let secrets: std::collections::BTreeMap<String, String> = serde_json::from_slice(&bytes)
        .map_err(|error| AdkReadSnapshotError::Unavailable(error.to_string()))?;
    Ok(secrets
        .get(provider_id)
        .is_some_and(|key| !key.trim().is_empty()))
}
