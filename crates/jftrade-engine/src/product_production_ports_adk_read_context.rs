use super::*;

/// Rebuild a context snapshot solely from durable session events and active handoff rows.
/// This path is used when a context-state projection predates the production store (or was interrupted before it could be written), so every persisted boundary is validated instead of being replaced by an empty/synthetic summary.
pub(super) fn rebuild_context_snapshot(
    session_id: &str,
    session_payload_json: &str,
    events: &[jftrade_store_sqlite::StoredAdkEvent],
    segments: &[jftrade_store_sqlite::StoredAdkHandoffSegment],
) -> Result<Value, AdkReadSnapshotError> {
    let session_payload: Value = serde_json::from_str(session_payload_json)
        .map_err(|error| invalid_payload("session", error))?;
    let context_window_tokens = session_payload
        .get("contextWindowTokens")
        .and_then(Value::as_u64)
        .and_then(|value| usize::try_from(value).ok())
        .filter(|value| *value > 0)
        .unwrap_or(0);

    let mut current_revision = String::new();
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
    let mut active_segments = Vec::new();
    for segment in segments {
        let payload: Value = serde_json::from_str(&segment.payload_json)
            .map_err(|error| invalid_payload("handoff segment", error))?;
        let revision = payload
            .get("contextRevisionId")
            .and_then(Value::as_str)
            .map(str::trim)
            .unwrap_or_default();
        if current_revision.is_empty() || revision == current_revision {
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
    let recent_start = recent_context_event_start(events, 10);
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
        "status": context_status_for_read(usage_ratio, context_window_tokens),
        "recentUserWindow": 10,
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
