use super::*;

/// Rebuild a context snapshot solely from durable session events and active handoff rows.
/// This path is used when a context-state projection predates the production store (or was interrupted before it could be written), so every persisted boundary is validated instead of being replaced by an empty/synthetic summary.
pub(crate) fn rebuild_context_snapshot(
    session_id: &str,
    session_payload_json: &str,
    stored_revision: &str,
    events: &[jftrade_store_sqlite::StoredAdkEvent],
    segments: &[jftrade_store_sqlite::StoredAdkHandoffSegment],
    context_window_tokens: usize,
    recent_user_window: usize,
) -> Result<Value, AdkReadSnapshotError> {
    let session_payload: Value = serde_json::from_str(session_payload_json)
        .map_err(|error| invalid_payload("session", error))?;
    // Go resolves the window from the session's effective provider; an older
    // Go-owned database may still carry the projection's own copy.
    let context_window_tokens = if context_window_tokens > 0 {
        context_window_tokens
    } else {
        session_payload
            .get("contextWindowTokens")
            .and_then(Value::as_u64)
            .and_then(|value| usize::try_from(value).ok())
            .filter(|value| *value > 0)
            .unwrap_or(0)
    };

    // Go anchors the projection on the revision stored in the session context
    // state and only then reads the handoff chain for that revision.  A
    // database without a context-state row is recovered from the newest
    // revisioned segment (an interrupted compaction), but handoff rows that
    // never carried a revision belong to the legacy layout and stay invisible:
    // adopting them would resurrect a summary the console already compacted
    // past.
    let mut current_revision = stored_revision.trim().to_owned();
    if current_revision.is_empty() {
        for segment in segments {
            let payload: Value = serde_json::from_str(&segment.payload_json)
                .map_err(|error| invalid_payload("handoff segment", error))?;
            if let Some(revision) = payload
                .get("contextRevisionId")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|revision| !revision.is_empty())
            {
                current_revision = revision.to_owned();
            }
        }
    }
    let mut active_segments = Vec::new();
    for segment in segments {
        let payload: Value = serde_json::from_str(&segment.payload_json)
            .map_err(|error| invalid_payload("handoff segment", error))?;
        let revision = payload
            .get("contextRevisionId")
            .and_then(Value::as_str)
            .map(str::trim)
            .unwrap_or_default();
        if !current_revision.is_empty() && revision == current_revision {
            active_segments.push((segment, payload));
        }
    }
    active_segments.sort_by_key(|(segment, _)| {
        (
            segment.sequence,
            segment.created_at.clone(),
            segment.id.clone(),
        )
    });

    let compacted_event_count = active_segments
        .iter()
        .filter_map(|(_, payload)| payload.get("endEventIndex").and_then(Value::as_u64))
        .filter_map(|value| usize::try_from(value).ok())
        .max()
        .unwrap_or(0)
        .min(events.len());
    let summary = active_segments
        .last()
        .and_then(|(_, payload)| payload.get("summary").and_then(Value::as_str))
        .map(str::trim)
        .unwrap_or_default()
        .to_owned();
    let handoff_text = active_segments
        .iter()
        .filter_map(|(_, payload)| payload.get("summary").and_then(Value::as_str))
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .collect::<Vec<_>>()
        .join("\n\n");
    let handoff_tokens =
        estimate_context_tokens(&format!("Session handoff summaries:\n{handoff_text}"));
    let raw_event_tokens = events
        .iter()
        .map(|event| estimate_context_tokens(&event.content))
        .sum::<usize>();
    let effective_event_tokens = events
        .iter()
        .skip(compacted_event_count)
        .map(|event| estimate_context_tokens(&event.content))
        .sum::<usize>();
    let current_input_tokens = handoff_tokens.saturating_add(effective_event_tokens);
    let usage_ratio = if context_window_tokens == 0 {
        0.0
    } else {
        current_input_tokens as f64 / context_window_tokens as f64
    };
    let recent_start = recent_context_event_start(events, recent_user_window.max(1));
    let protected_start = protected_context_event_start(events);
    let retained_start = compacted_event_count.max(recent_start).min(events.len());
    let retained_end = protected_start.max(retained_start).min(events.len());
    let retained_recent_count = events[retained_start..retained_end]
        .iter()
        .filter(|event| is_context_user_event(event))
        .count();
    let protected_recent_count = events[protected_start..]
        .iter()
        .filter(|event| is_context_user_event(event))
        .count();
    let recent_user_tokens = events[retained_start..retained_end]
        .iter()
        .map(|event| estimate_context_tokens(&event.content))
        .sum::<usize>();
    let protected_tail_tokens = events[protected_start..]
        .iter()
        .map(|event| estimate_context_tokens(&event.content))
        .sum::<usize>();
    let other_visible_tokens = events[compacted_event_count.min(recent_start)..recent_start]
        .iter()
        .map(|event| estimate_context_tokens(&event.content))
        .sum::<usize>();
    let revision_created_at = active_segments
        .last()
        .and_then(|(_, payload)| payload.get("createdAt").and_then(Value::as_str))
        .unwrap_or_default();
    let last_compacted_at = active_segments
        .last()
        .and_then(|(_, payload)| payload.get("updatedAt").and_then(Value::as_str))
        .unwrap_or_default();
    let last_mode = active_segments
        .last()
        .and_then(|(_, payload)| payload.get("mode").and_then(Value::as_str))
        .unwrap_or_default();
    let last_reason = active_segments
        .last()
        .and_then(|(_, payload)| payload.get("reason").and_then(Value::as_str))
        .unwrap_or_default();
    Ok(json!({
        "sessionId": session_id,
        "contextRevisionId": current_revision,
        "contextRevisionCreatedAt": revision_created_at,
        "currentInputTokens": current_input_tokens,
        "projectedNextTurnTokens": current_input_tokens,
        "estimatedInputTokens": current_input_tokens,
        "rawCurrentInputTokens": raw_event_tokens,
        "rawProjectedNextTurnTokens": raw_event_tokens,
        "contextWindowTokens": context_window_tokens,
        "usageRatio": usage_ratio,
        "status": super::context_window::context_status_for_read(
            usage_ratio,
            context_window_tokens,
        ),
        "recentUserWindow": recent_user_window,
        "retainedRecentUserCount": retained_recent_count,
        "protectedRecentCount": protected_recent_count,
        "activeHandoffCount": active_segments.len(),
        "latestHandoffPreview": summary,
        "summaryPreview": summary,
        "rawEventCount": events.len(),
        "compactedEventCount": compacted_event_count,
        "summaryBoundaryEventIndex": compacted_event_count,
        "breakdown": {
            "instructionTokens": 0,
            "handoffTokens": handoff_tokens,
            "recentUserTokens": recent_user_tokens,
            "protectedTailTokens": protected_tail_tokens,
            "otherVisibleTokens": other_visible_tokens,
            "pendingUserTokens": 0,
            "toolDeclarationTokens": 0,
        },
        "rawBreakdown": {
            "instructionTokens": 0,
            "handoffTokens": 0,
            "recentUserTokens": raw_event_tokens,
            "protectedTailTokens": 0,
            "otherVisibleTokens": 0,
            "pendingUserTokens": 0,
            "toolDeclarationTokens": 0,
        },
        "lastCompactedAt": last_compacted_at,
        "lastCompactionMode": last_mode,
        "lastCompactionReason": last_reason,
        "autoCompacted": false,
        "degradedSummary": false,
    }))
}

/// Go `SessionContextManager.Snapshot`: the persisted context state is only
/// the compaction boundary.  Every read recomputes the metrics from the
/// transcript, so a turn appended after the last compaction is reflected
/// immediately instead of being frozen behind the stored counters.
///
/// The read path stays side-effect free: Go re-saves the snapshot here, but
/// Rust only persists the state when a compaction commits (CAS-protected), so
/// a read can never race a concurrent compaction into the store.
pub(crate) fn refresh_context_snapshot(
    session_id: &str,
    session_payload_json: &str,
    stored: &Value,
    events: &[jftrade_store_sqlite::StoredAdkEvent],
    segments: &[jftrade_store_sqlite::StoredAdkHandoffSegment],
    context_window_tokens: usize,
    recent_user_window: usize,
) -> Result<Value, AdkReadSnapshotError> {
    let revision = stored
        .get("contextRevisionId")
        .and_then(Value::as_str)
        .map(str::trim)
        .unwrap_or_default();
    let mut snapshot = rebuild_context_snapshot(
        session_id,
        session_payload_json,
        revision,
        events,
        segments,
        context_window_tokens,
        recent_user_window,
    )?;
    // Fields the compaction owns and the transcript cannot derive.
    if let (Some(target), Some(source)) = (snapshot.as_object_mut(), stored.as_object()) {
        for key in [
            "previousContextRevisionId",
            "lastCompactionTrigger",
            "autoCompacted",
        ] {
            if let Some(value) = source.get(key) {
                target.insert(key.to_owned(), value.clone());
            }
        }
    }
    Ok(snapshot)
}

/// `/api/v1/adk/sessions/{id}/context`: the durable state supplies the
/// compaction boundary, while the transcript supplies the live metrics.
pub(crate) fn session_context_snapshot(
    port: &ProductionAdkPort,
    session_id: &str,
) -> Result<AdkReadSnapshot, AdkReadSnapshotError> {
    let Some(session) = port.store.get_session(session_id)? else {
        // Go's `handleADKSessionContext` keeps the route's own code and only
        // escalates "not found" to 404.
        return Err(not_found_with_code(
            "ADK_SESSION_CONTEXT_FAILED",
            "session not found",
        ));
    };
    let window = super::context_window::resolve_session_context_window_tokens(
        &port.store,
        session_id,
        &session.payload_json,
    );
    let recent_window = super::context_window::resolve_session_context_recent_window(
        &port.store,
        session_id,
        &session.payload_json,
    );
    let events = port
        .session_store
        .list_events(session_id)
        .map_err(|error| AdkReadSnapshotError::Unavailable(error.to_string()))?;
    let segments = port
        .store
        .list_handoff_segments(session_id, true)
        .map_err(AdkReadSnapshotError::from)?;
    if let Some(state) = port.store.get_session_context(session_id)? {
        let stored = payload(
            &state.payload_json,
            "session context",
            [("sessionId", session_id.to_owned())],
        )?;
        let mut snapshot = refresh_context_snapshot(
            session_id,
            &session.payload_json,
            &stored,
            &events,
            &segments,
            window,
            recent_window,
        )?;
        super::context_window::patch_context_window(&mut snapshot, window, recent_window);
        return Ok(AdkReadSnapshot::Json(snapshot));
    }
    // A database without a context-state row still gets Go's ensured revision:
    // a legacy handoff row (no `contextRevisionId`) must not reach the
    // projection, and the anchored revision is persisted.
    let revision = format!("ctx-{}", crate::product_id::generate_uuid_v4());
    let snapshot = rebuild_context_snapshot(
        session_id,
        &session.payload_json,
        &revision,
        &events,
        &segments,
        window,
        recent_window,
    )?;
    port.store
        .upsert_session_context(session_id, &snapshot.to_string())?;
    Ok(AdkReadSnapshot::Json(snapshot))
}
