//! Restore a run checkpoint from a terminal invocation without rewriting it.

use super::*;

pub(super) struct TerminalReplay<'a> {
    pub expected_status: &'a str,
    pub expected_updated_at: &'a str,
    pub payload_json: &'a str,
    pub owner_id: &'a str,
    pub run_lease_token: i64,
    pub event: &'a AdkRunEvent<'a>,
}

pub(super) fn restore_projection(
    transaction: &rusqlite::Transaction<'_>,
    invocation: &StoredAdkToolInvocation,
    replay: TerminalReplay<'_>,
    now: &str,
) -> Result<bool, AdkStoreError> {
    let output: Value = serde_json::from_str(&invocation.output_json).map_err(|error| {
        AdkStoreError::Invariant(format!("invalid durable tool output: {error}"))
    })?;
    let proposed: Value = serde_json::from_str(replay.payload_json).map_err(|error| {
        AdkStoreError::Validation(format!("invalid replay projection: {error}"))
    })?;
    if !contains_result(&proposed, &invocation.idempotency_key, &output) {
        return Ok(false);
    }
    let current_json: String = transaction
        .query_row(
            "SELECT payload_json FROM adk_runs WHERE id=?1",
            [&invocation.run_id],
            |row| row.get(0),
        )
        .map_err(AdkStoreError::Query)?;
    let current: Value = serde_json::from_str(&current_json).map_err(|error| {
        AdkStoreError::Invariant(format!("invalid current run projection: {error}"))
    })?;
    if contains_result(&current, &invocation.idempotency_key, &output) {
        return Ok(false);
    }
    ensure_current_run_lease(
        transaction,
        &invocation.run_id,
        replay.owner_id,
        replay.run_lease_token,
    )?;
    let affected = transaction
        .execute(
            "UPDATE adk_runs SET payload_json=?1, updated_at=?2
         WHERE id=?3 AND status=?4 AND updated_at=?5",
            params![
                replay.payload_json,
                now,
                invocation.run_id,
                replay.expected_status,
                replay.expected_updated_at
            ],
        )
        .map_err(AdkStoreError::Query)?;
    if affected != 1 {
        return Err(AdkStoreError::RevisionChanged(format!(
            "run {} changed before tool replay",
            invocation.run_id
        )));
    }
    append_adk_session_events(
        transaction,
        now,
        &invocation.run_id,
        std::slice::from_ref(replay.event),
    )?;
    Ok(true)
}

fn contains_result(payload: &Value, call_id: &str, output: &Value) -> bool {
    payload
        .get("toolResults")
        .and_then(Value::as_array)
        .is_some_and(|results| {
            results.iter().any(|result| {
                result["callId"].as_str() == Some(call_id) && &result["output"] == output
            })
        })
}
