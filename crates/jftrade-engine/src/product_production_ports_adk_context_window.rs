//! Session-context sizing shared by the ADK read and compaction routes.
//!
//! Go resolves both numbers from the session's effective agent: its
//! provider supplies the context window and the agent's normalized
//! `RecentUserWindow` supplies the retained-turn boundary.  Keeping the
//! lookup in one place is what stops a read and a compaction from
//! disagreeing about the window they project against.

use serde_json::Value;

/// Go `contextStatus`: the ratio decides the band, and a session without a
/// resolved provider window reports `unknown` instead of a fabricated band.
pub(crate) fn context_status_for_read(ratio: f64, window: usize) -> &'static str {
    if window == 0 {
        "unknown"
    } else if ratio >= 0.93 {
        "critical"
    } else if ratio >= 0.85 {
        "near_limit"
    } else if ratio >= 0.70 {
        "warning"
    } else {
        "healthy"
    }
}

/// Go `NormalizeRecentUserWindow`: an agent stores a clamped window and an
/// absent value falls back to the built-in default of six recent user turns.
fn normalize_recent_user_window(value: i64) -> usize {
    if value <= 0 {
        6
    } else if value < 2 {
        2
    } else if value > 100 {
        100
    } else {
        usize::try_from(value).unwrap_or(6)
    }
}

/// The agent a session projects its context through (Go
/// `Runtime.resolveSessionContextAgent`): the composer provider/model override
/// wins over the session's own agent, and a missing row stays unresolved.
fn session_context_agent_payload(
    store: &jftrade_store_sqlite::AdkStore,
    session_id: &str,
    session_payload_json: &str,
) -> Option<Value> {
    let session_payload: Value = serde_json::from_str(session_payload_json).ok()?;
    let agent_id = session_payload
        .get("agentId")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())?;
    let agent = store.get_agent(agent_id).ok().flatten()?;
    let mut payload = serde_json::from_str::<Value>(&agent.payload_json).ok()?;
    let object = payload.as_object_mut()?;
    let composer = store
        .get_session_composer_state(session_id)
        .ok()
        .flatten()
        .and_then(|row| serde_json::from_str::<Value>(&row.payload_json).ok());
    if let Some(composer) = composer.as_ref().and_then(Value::as_object) {
        for key in ["providerIdOverride", "modelOverride"] {
            let Some(value) = composer
                .get(key)
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
            else {
                continue;
            };
            object.insert(
                key.trim_end_matches("Override").to_owned(),
                Value::String(value.to_owned()),
            );
        }
    }
    Some(payload)
}

/// Go `SessionContextManager.contextWindowTokens`: the window comes from the
/// session's effective provider, never from the session row itself.  A missing
/// agent or provider reports zero, which the snapshot surfaces as `unknown`.
pub(crate) fn resolve_session_context_window_tokens(
    store: &jftrade_store_sqlite::AdkStore,
    session_id: &str,
    session_payload_json: &str,
) -> usize {
    let Some(agent) = session_context_agent_payload(store, session_id, session_payload_json) else {
        return 0;
    };
    let Some(provider_id) = agent
        .get("providerId")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
    else {
        return 0;
    };
    let Some(provider) = store.get_provider(provider_id).ok().flatten() else {
        return 0;
    };
    let tokens = serde_json::from_str::<Value>(&provider.payload_json)
        .ok()
        .and_then(|payload| payload.get("contextWindowTokens").and_then(Value::as_i64))
        .unwrap_or(0);
    usize::try_from(tokens).unwrap_or(0)
}

/// The recent user window the session's effective agent projects with.
pub(crate) fn resolve_session_context_recent_window(
    store: &jftrade_store_sqlite::AdkStore,
    session_id: &str,
    session_payload_json: &str,
) -> usize {
    session_context_agent_payload(store, session_id, session_payload_json)
        .and_then(|agent| agent.get("recentUserWindow").and_then(Value::as_i64))
        .map_or(6, normalize_recent_user_window)
}

/// Apply the live window and recent-user window to an already durable context
/// projection without rewriting the stored rows.
pub(crate) fn patch_context_window(
    snapshot: &mut Value,
    context_window_tokens: usize,
    recent_user_window: usize,
) {
    let Some(object) = snapshot.as_object_mut() else {
        return;
    };
    let current = object
        .get("projectedNextTurnTokens")
        .or_else(|| object.get("currentInputTokens"))
        .and_then(Value::as_i64)
        .unwrap_or(0)
        .max(0);
    let current = usize::try_from(current).unwrap_or(0);
    let ratio = if context_window_tokens == 0 {
        0.0
    } else {
        current as f64 / context_window_tokens as f64
    };
    object.insert(
        "contextWindowTokens".to_owned(),
        Value::from(context_window_tokens),
    );
    object.insert("usageRatio".to_owned(), Value::from(ratio));
    object.insert(
        "status".to_owned(),
        Value::String(context_status_for_read(ratio, context_window_tokens).to_owned()),
    );
    object.insert(
        "recentUserWindow".to_owned(),
        Value::from(recent_user_window),
    );
}
