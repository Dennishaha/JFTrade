use super::*;
use serde_json::Value;

pub(super) fn open_retained_stream(
    port: &ProductionAdkPort,
    path: &str,
    query: &str,
) -> Result<Option<AdkReadLiveStream>, AdkReadSnapshotError> {
    let id = dynamic_id(path, "/api/v1/adk/streams/", "")
        .or_else(|| dynamic_id(path, "/api/v1/adk/runs/", "/stream"));
    let Some(id) = id else { return Ok(None) };
    let after = query_param(query, "after")
        .and_then(|value| value.parse::<u64>().ok())
        .unwrap_or(0);
    if path.starts_with("/api/v1/adk/streams/")
        && let Some(stream) = port.unavailable_streams.open(&id, after).map_err(|_| {
            AdkReadSnapshotError::Unavailable(unavailable_stream::UNAVAILABLE.to_owned())
        })?
    {
        return Ok(Some(stream));
    }
    let cursor = port
        .store
        .open_stream_cursor(&id)?
        .ok_or_else(|| not_found("stream not found"))?;
    if active_stream_expired(&port.store, &cursor.run_id) {
        return Err(not_found("stream not found"));
    }
    Ok(Some(AdkReadLiveStream {
        headers: vec![("X-ADK-Stream-ID".to_owned(), cursor.stream_id.clone())],
        body: reconnect_stream::body(
            port.store.clone(),
            port.session_store.clone(),
            cursor,
            after,
        ),
    }))
}

/// Go cleanup treats a failed GetRun as a missing lookup. Reconnect still uses
/// its existing cursor/history errors; this best-effort read gains no writer.
pub(super) fn active_stream_expired(store: &AdkStore, run_id: &str) -> bool {
    let Ok(Some(run)) = store.read_stream_run_metadata(run_id) else {
        return false;
    };
    if matches!(
        run.status.as_str(),
        "COMPLETED" | "FAILED" | "CANCELLED" | "TIMED_OUT" | "DENIED"
    ) || jftrade_assistant::validate_persisted_run_payload(&run.payload_json).is_err()
    {
        return false;
    }
    let Ok(payload) = serde_json::from_str::<Value>(&run.payload_json) else {
        return false;
    };
    let started =
        timestamp_ns(&payload, "startedAt").or_else(|| timestamp_ns(&payload, "createdAt"));
    let Some(started) = started else { return false };
    jftrade_assistant::active_run_stream_retention_expired(
        time::OffsetDateTime::now_utc().unix_timestamp_nanos(),
        started,
        payload
            .get("maxDurationMs")
            .and_then(Value::as_i64)
            .unwrap_or(0),
    )
}

fn timestamp_ns(payload: &Value, field: &str) -> Option<i128> {
    time::OffsetDateTime::parse(
        payload.get(field)?.as_str()?.trim(),
        &time::format_description::well_known::Rfc3339,
    )
    .ok()
    .map(|value| value.unix_timestamp_nanos())
}
