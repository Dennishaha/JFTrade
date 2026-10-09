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
    error: impl std::fmt::Display,
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

/// Call id of a durable `assistant.tool_call` envelope that is still waiting
/// for an operator decision, plus the original call it belongs to when the
/// envelope names one.
///
/// Go records the same state as a `toolconfirmation` function call that has no
/// matching function response, so the anchor has to survive compaction until
/// the operator answers it.
fn pending_approval_anchor(
    event: &jftrade_store_sqlite::StoredAdkEvent,
) -> Option<(String, Option<String>)> {
    if !event.author.trim().eq_ignore_ascii_case("assistant.tool_call") {
        return None;
    }
    let value: Value = serde_json::from_str(event.content.trim()).ok()?;
    let object = value.as_object()?;
    let status = object.get("status").and_then(Value::as_str)?.trim();
    if !status.eq_ignore_ascii_case("PENDING_APPROVAL") {
        return None;
    }
    let call_id = object
        .get("id")
        .or_else(|| object.get("callId"))
        .and_then(Value::as_str)?
        .trim()
        .to_owned();
    if call_id.is_empty() {
        return None;
    }
    let original = object
        .get("functionCallId")
        .or_else(|| object.get("originalCallId"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty() && *value != call_id)
        .map(str::to_owned);
    Some((call_id, original))
}

/// Call id carried by a durable envelope (`callId` on tool results, `id` on
/// staged calls).
fn envelope_call_id(event: &jftrade_store_sqlite::StoredAdkEvent) -> Option<String> {
    let value: Value = serde_json::from_str(event.content.trim()).ok()?;
    let object = value.as_object()?;
    let call_id = object
        .get("callId")
        .or_else(|| object.get("id"))
        .and_then(Value::as_str)?
        .trim();
    (!call_id.is_empty()).then(|| call_id.to_owned())
}

/// Approval call ids that already have a durable tool outcome.  Go derives the
/// same set from the `toolconfirmation` function responses.
fn resolved_tool_call_ids(
    events: &[jftrade_store_sqlite::StoredAdkEvent],
) -> std::collections::BTreeSet<String> {
    events
        .iter()
        .filter(|event| event.author.trim().eq_ignore_ascii_case("assistant.tool"))
        .filter_map(envelope_call_id)
        .collect()
}

/// Runs that ended through the denial path (`{runId}:denied`).  Every pending
/// approval of such a run was denied together, so none of them anchors the
/// protected tail any more.
fn denied_run_ids(
    events: &[jftrade_store_sqlite::StoredAdkEvent],
) -> std::collections::BTreeSet<String> {
    events
        .iter()
        .filter(|event| event.id.trim().ends_with(":denied"))
        .map(|event| event.invocation_id.trim().to_owned())
        .filter(|run_id| !run_id.is_empty())
        .collect()
}

/// Index of the durable call envelope that carries `call_id`, matching Go's
/// `functionCallEventIndex` lookup for an approval's original call.
fn call_envelope_index(
    events: &[jftrade_store_sqlite::StoredAdkEvent],
    call_id: &str,
) -> Option<usize> {
    events
        .iter()
        .position(|event| envelope_call_id(event).is_some_and(|id| id == call_id))
}

/// Go `protectedTailStart`: the earliest event a compaction must keep because
/// an approval is still unresolved.
///
/// Go ignores approvals that already have a function response and rewinds an
/// unresolved approval to the function call it confirms.  The Rust transcript
/// keeps approvals as `assistant.tool_call` envelopes whose `status` stays
/// `PENDING_APPROVAL` until a matching `assistant.tool` result (or a denied
/// run terminal event) records the outcome, so the same skip and rewind apply
/// to those envelopes.
pub(crate) fn protected_context_event_start(
    events: &[jftrade_store_sqlite::StoredAdkEvent],
) -> usize {
    let resolved = resolved_tool_call_ids(events);
    let denied_runs = denied_run_ids(events);
    let mut start = events.len();
    for (index, event) in events.iter().enumerate() {
        let Some((call_id, original_id)) = pending_approval_anchor(event) else {
            continue;
        };
        if resolved.contains(&call_id) || denied_runs.contains(event.invocation_id.trim()) {
            continue;
        }
        let candidate = original_id
            .as_deref()
            .and_then(|original| call_envelope_index(events, original))
            .unwrap_or(index);
        start = start.min(candidate);
    }
    start
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
