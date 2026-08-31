//! Durable session-context compaction for the production ADK adapter.
//!
//! A compaction is a projection change, not a successful no-op response.  The
//! handoff segment is written before the new context snapshot so a restart can
//! reconstruct the same active boundary from SQLite alone.

use std::sync::atomic::Ordering;

use jftrade_store_sqlite::StoredAdkHandoffSegment;
use serde_json::{Value, json};

use super::*;

pub(super) fn compact_session_context(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    let session_id = required_identifier(input, "sessionId")?;
    let session = port
        .store
        .get_session(&session_id)
        .map_err(session_mutation_failed)?
        .ok_or_else(|| not_found_mutation("ADK_SESSION_NOT_FOUND", "session not found"))?;
    if port
        .store
        .list_runs()
        .map_err(session_mutation_failed)?
        .into_iter()
        .any(|run| run.session_id == session_id && run.status.eq_ignore_ascii_case("RUNNING"))
    {
        return Err(AdkMutationPortError::Failed {
            status: 409,
            code: "ADK_SESSION_ACTIVE_RUN".to_owned(),
            message: "session has an active run".to_owned(),
        });
    }

    let session_payload = decode_mutation_payload(&session.payload_json, "session")?;
    let context_window = session_payload
        .get("contextWindowTokens")
        .and_then(Value::as_u64)
        .and_then(|value| usize::try_from(value).ok())
        .filter(|value| *value > 0)
        .unwrap_or(0);
    let events = port
        .session_store
        .list_events(&session_id)
        .map_err(session_mutation_failed)?;
    let mode = normalize_context_mode(input.body.get("mode"))?;
    let trigger = input
        .body
        .get("trigger")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or("manual")
        .to_owned();
    let reason = input
        .body
        .get("reason")
        .and_then(Value::as_str)
        .map(str::trim)
        .unwrap_or_default()
        .to_owned();
    let recent_window = parse_recent_user_window(&input.body)?;

    let stored_segments = port
        .store
        .list_handoff_segments(&session_id, false)
        .map_err(session_mutation_failed)?;
    let current_state = port
        .store
        .get_session_context(&session_id)
        .map_err(session_mutation_failed)?
        .map(|stored| decode_mutation_payload(&stored.payload_json, "session context"))
        .transpose()?;
    let stored_revision = current_state
        .as_ref()
        .and_then(|value| value.get("contextRevisionId"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or_default()
        .to_owned();
    // A crash between the old projection and a restart may leave durable
    // handoff rows without a context-state row.  Re-anchor the CAS token to
    // the newest active segment instead of inventing a revision that would
    // hide those rows from the rebuilt context.
    let inferred_revision = if stored_revision.is_empty() {
        let mut revision = String::new();
        for segment in &stored_segments {
            if !segment.active {
                continue;
            }
            let payload = decode_mutation_payload(&segment.payload_json, "handoff segment")?;
            if let Some(value) = payload
                .get("contextRevisionId")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
            {
                revision = value.to_owned();
            }
        }
        revision
    } else {
        String::new()
    };
    let current_revision = if stored_revision.is_empty() {
        inferred_revision
    } else {
        stored_revision.clone()
    };
    let active_segments = current_revision_segments(&stored_segments, &current_revision)?;

    let recent_start = recent_user_event_start(&events, recent_window);
    let protected_start = protected_tail_start(&events);
    let compaction_cutoff = recent_start.min(protected_start);
    let mut active_end = 0usize;
    for segment in &active_segments {
        let payload = decode_mutation_payload(&segment.payload_json, "handoff segment")?;
        let end = payload
            .get("endEventIndex")
            .and_then(Value::as_u64)
            .and_then(|value| usize::try_from(value).ok())
            .ok_or_else(|| AdkMutationPortError::Failed {
                status: 500,
                code: "ADK_STORAGE_CORRUPT".to_owned(),
                message: "handoff segment endEventIndex is invalid".to_owned(),
            })?;
        active_end = active_end.max(end.min(events.len()));
    }
    let should_write = if mode == "aggressive" {
        compaction_cutoff > 0 || !active_segments.is_empty()
    } else {
        compaction_cutoff > active_end
    };

    let now = now_rfc3339();
    // `current_revision` may be recovered from an active handoff segment when
    // the context-state row is missing after an interrupted compaction.  It is
    // still the projection's previous revision, but the SQLite CAS expected
    // token must remain the actual context-row revision (the empty token for a
    // missing row).  Passing the recovered segment token as the expected value
    // would make the first re-anchor fail with a false conflict.
    let expected_revision = stored_revision.clone();
    let previous_revision = current_revision.clone();
    let mut pending_segment: Option<(String, i64, String, bool)> = None;
    let (revision, revision_created_at, compacted_cutoff, active_segments) = if should_write {
        let revision = format!(
            "ctx-{}",
            SESSION_ID_SEQUENCE.fetch_add(1, Ordering::Relaxed)
        );
        let compacted_cutoff = compaction_cutoff.min(events.len());
        let summary_events = if mode == "aggressive" {
            &events[..compacted_cutoff]
        } else {
            &events[active_end.min(compacted_cutoff)..compacted_cutoff]
        };
        let prior_summaries = active_segments
            .iter()
            .map(|segment| handoff_summary(&segment.payload_json))
            .collect::<Result<Vec<_>, _>>()?;
        let summary = build_handoff_summary(&prior_summaries, summary_events, &mode, &reason);
        let sequence = stored_segments
            .iter()
            .filter_map(|segment| usize::try_from(segment.sequence).ok())
            .max()
            .unwrap_or(0)
            .saturating_add(1);
        let id = format!(
            "handoff-{}-{}",
            normalize_id(&session_id),
            SESSION_ID_SEQUENCE.fetch_add(1, Ordering::Relaxed)
        );
        let payload = json!({
            "id": id,
            "sessionId": session_id,
            "contextRevisionId": revision,
            "sequence": sequence,
            "startEventIndex": 0,
            "endEventIndex": compacted_cutoff,
            "summary": summary,
            "mode": if mode == "aggressive" { "aggressive" } else { "manual" },
            "reason": reason,
            "estimatedTokens": estimate_text_tokens(&summary),
            "active": true,
            "createdAt": now,
            "updatedAt": now,
        });
        let payload_json = payload.to_string();
        let segment_id = payload["id"]
            .as_str()
            .ok_or_else(|| AdkMutationPortError::Failed {
                status: 500,
                code: "ADK_CONTEXT_SEGMENT_INVALID".to_owned(),
                message: "generated handoff segment id is invalid".to_owned(),
            })?
            .to_owned();
        let sequence = i64::try_from(sequence).map_err(|_| AdkMutationPortError::Failed {
            status: 500,
            code: "ADK_CONTEXT_SEGMENT_INVALID".to_owned(),
            message: "handoff segment sequence is out of range".to_owned(),
        })?;
        pending_segment = Some((
            segment_id.clone(),
            sequence,
            payload_json.clone(),
            mode == "aggressive",
        ));
        let synthetic = StoredAdkHandoffSegment {
            id: segment_id,
            session_id: session_id.clone(),
            active: true,
            sequence,
            payload_json,
            created_at: now.clone(),
            updated_at: now.clone(),
        };
        // An aggressive compaction replaces the active chain; manual
        // compaction appends a new segment but the new revision is the only
        // chain visible to the projection.
        (revision, now.clone(), compacted_cutoff, vec![synthetic])
    } else {
        let revision = if current_revision.is_empty() {
            format!(
                "ctx-{}",
                SESSION_ID_SEQUENCE.fetch_add(1, Ordering::Relaxed)
            )
        } else {
            current_revision
        };
        let created_at = current_state
            .as_ref()
            .and_then(|value| value.get("contextRevisionCreatedAt"))
            .and_then(Value::as_str)
            .filter(|value| !value.trim().is_empty())
            .unwrap_or(&now)
            .to_owned();
        let cutoff = active_end.min(events.len());
        (revision, created_at, cutoff, active_segments)
    };

    let summary = active_segments
        .last()
        .map(|segment| handoff_summary(&segment.payload_json))
        .transpose()?
        .unwrap_or_default();
    let active_handoff_tokens = estimate_text_tokens(&format!(
        "Session handoff summaries:\n{}",
        active_segments
            .iter()
            .map(|segment| handoff_summary(&segment.payload_json))
            .collect::<Result<Vec<_>, _>>()?
            .into_iter()
            .filter(|value| !value.is_empty())
            .collect::<Vec<_>>()
            .join("\n\n")
    ));
    let raw_tokens = events
        .iter()
        .map(|event| estimate_text_tokens(&event.content))
        .sum::<usize>();
    let effective_event_tokens = events
        .iter()
        .skip(compacted_cutoff)
        .map(|event| estimate_text_tokens(&event.content))
        .sum::<usize>();
    let handoff_tokens = active_handoff_tokens;
    let current_tokens = handoff_tokens.saturating_add(effective_event_tokens);
    let usage_ratio = if context_window == 0 {
        0.0
    } else {
        current_tokens as f64 / context_window as f64
    };
    let status = context_status(usage_ratio, context_window);
    let protected_start = protected_start.min(events.len());
    let recent_start = recent_start.min(events.len());
    let retained_start = compacted_cutoff.max(recent_start).min(events.len());
    let retained_end = protected_start.max(retained_start).min(events.len());
    let retained_recent_count = events[retained_start..retained_end]
        .iter()
        .filter(|event| is_user_event(event))
        .count();
    let protected_recent_count = events[protected_start..]
        .iter()
        .filter(|event| is_user_event(event))
        .count();
    let recent_user_tokens = events[retained_start..retained_end]
        .iter()
        .map(|event| estimate_text_tokens(&event.content))
        .sum::<usize>();
    let protected_tail_tokens = events[protected_start..]
        .iter()
        .map(|event| estimate_text_tokens(&event.content))
        .sum::<usize>();
    let other_visible_tokens = events[compacted_cutoff.min(recent_start)..recent_start]
        .iter()
        .map(|event| estimate_text_tokens(&event.content))
        .sum::<usize>();
    let breakdown = json!({
        "instructionTokens": 0,
        "handoffTokens": handoff_tokens,
        "recentUserTokens": recent_user_tokens,
        "protectedTailTokens": protected_tail_tokens,
        "otherVisibleTokens": other_visible_tokens,
        "pendingUserTokens": 0,
        "toolDeclarationTokens": 0,
    });
    let raw_breakdown = json!({
        "instructionTokens": 0,
        "handoffTokens": 0,
        "recentUserTokens": raw_tokens,
        "protectedTailTokens": 0,
        "otherVisibleTokens": 0,
        "pendingUserTokens": 0,
        "toolDeclarationTokens": 0,
    });
    let last_mode = if trigger.eq_ignore_ascii_case("auto") && mode != "aggressive" {
        "auto"
    } else if mode == "aggressive" {
        "aggressive"
    } else {
        "manual"
    };
    let snapshot = json!({
        "sessionId": session_id,
        "contextRevisionId": revision,
        "previousContextRevisionId": previous_revision,
        "contextRevisionCreatedAt": revision_created_at,
        "currentInputTokens": current_tokens,
        "projectedNextTurnTokens": current_tokens,
        "estimatedInputTokens": current_tokens,
        "rawCurrentInputTokens": raw_tokens,
        "rawProjectedNextTurnTokens": raw_tokens,
        "contextWindowTokens": context_window,
        "usageRatio": usage_ratio,
        "status": status,
        "recentUserWindow": recent_window,
        "retainedRecentUserCount": retained_recent_count,
        "protectedRecentCount": protected_recent_count,
        "activeHandoffCount": active_segments.len(),
        "latestHandoffPreview": summary,
        "summaryPreview": summary,
        "rawEventCount": events.len(),
        "compactedEventCount": compacted_cutoff,
        "summaryBoundaryEventIndex": compacted_cutoff,
        "breakdown": breakdown,
        "rawBreakdown": raw_breakdown,
        "lastCompactedAt": now,
        "lastCompactionMode": last_mode,
        "lastCompactionTrigger": trigger,
        "lastCompactionReason": reason,
        "autoCompacted": last_mode == "auto",
        "degradedSummary": false,
    });
    let snapshot_json = snapshot.to_string();
    port.store
        .commit_session_context_compaction(
            &session_id,
            &expected_revision,
            pending_segment
                .as_ref()
                .map(|(id, sequence, payload, replace)| {
                    (id.as_str(), *sequence, payload.as_str(), *replace)
                }),
            &snapshot_json,
        )
        .map_err(|error| match error {
            jftrade_store_sqlite::AdkStoreError::Conflict(message) => {
                AdkMutationPortError::Failed {
                    status: 409,
                    code: "ADK_CONTEXT_REVISION_CONFLICT".to_owned(),
                    message,
                }
            }
            other => session_mutation_failed(other),
        })?;
    Ok(snapshot)
}

fn normalize_context_mode(value: Option<&Value>) -> Result<String, AdkMutationPortError> {
    let Some(value) = value else {
        return Ok("normal".to_owned());
    };
    let mode = value
        .as_str()
        .ok_or_else(|| invalid_mutation_input("context compaction mode must be a string"))?
        .trim()
        .to_ascii_lowercase();
    if mode.is_empty() || matches!(mode.as_str(), "normal" | "manual" | "summary" | "balanced") {
        return Ok("normal".to_owned());
    }
    if mode == "aggressive" {
        return Ok(mode);
    }
    Err(invalid_mutation_input("invalid context compaction mode"))
}

fn parse_recent_user_window(body: &Value) -> Result<usize, AdkMutationPortError> {
    let Some(value) = body.get("recentUserWindow") else {
        return Ok(10);
    };
    let window = value
        .as_u64()
        .and_then(|value| usize::try_from(value).ok())
        .filter(|value| *value > 0)
        .ok_or_else(|| invalid_mutation_input("recentUserWindow must be a positive integer"))?;
    Ok(window.min(100))
}

fn current_revision_segments(
    segments: &[StoredAdkHandoffSegment],
    revision: &str,
) -> Result<Vec<StoredAdkHandoffSegment>, AdkMutationPortError> {
    let mut selected = Vec::new();
    for segment in segments {
        let payload = decode_mutation_payload(&segment.payload_json, "handoff segment")?;
        let payload_revision = payload
            .get("contextRevisionId")
            .and_then(Value::as_str)
            .map(str::trim)
            .unwrap_or_default();
        if (revision.is_empty() || payload_revision == revision) && segment.active {
            selected.push(segment.clone());
        }
    }
    selected.sort_by_key(|segment| {
        (
            segment.sequence,
            segment.created_at.clone(),
            segment.id.clone(),
        )
    });
    Ok(selected)
}

fn handoff_summary(payload: &str) -> Result<String, AdkMutationPortError> {
    let value = decode_mutation_payload(payload, "handoff segment")?;
    Ok(value
        .get("summary")
        .and_then(Value::as_str)
        .map(str::trim)
        .unwrap_or_default()
        .to_owned())
}

fn build_handoff_summary(
    prior: &[String],
    events: &[jftrade_store_sqlite::StoredAdkEvent],
    mode: &str,
    reason: &str,
) -> String {
    let max_line = if mode == "aggressive" { 140 } else { 220 };
    let max_lines = if mode == "aggressive" { 12 } else { 24 };
    let mut lines = Vec::new();
    for summary in prior {
        let summary = clip_summary_line(summary, max_line);
        if !summary.is_empty() {
            lines.push(format!("Prior handoff: {summary}"));
        }
    }
    if !events.is_empty() {
        lines.push("Conversation material:".to_owned());
    }
    for event in events {
        let content = clip_summary_line(&event.content, max_line);
        if content.is_empty() {
            continue;
        }
        let role = if is_user_event(event) {
            "User"
        } else {
            "Assistant"
        };
        lines.push(format!("- {role}: {content}"));
        if lines.len() >= max_lines {
            break;
        }
    }
    if lines.is_empty() && !reason.trim().is_empty() {
        lines.push(format!(
            "Compaction reason: {}",
            clip_summary_line(reason, max_line)
        ));
    }
    lines.join("\n").trim().to_owned()
}

fn clip_summary_line(value: &str, max_len: usize) -> String {
    let normalized = value.split_whitespace().collect::<Vec<_>>().join(" ");
    if normalized.chars().count() <= max_len {
        return normalized;
    }
    let clipped = normalized.chars().take(max_len).collect::<String>();
    format!("{clipped}...")
}

fn estimate_text_tokens(value: &str) -> usize {
    let bytes = value.trim().len();
    if bytes == 0 {
        0
    } else {
        bytes.saturating_add(3) / 4
    }
}

fn context_status(ratio: f64, window: usize) -> &'static str {
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

fn is_user_event(event: &jftrade_store_sqlite::StoredAdkEvent) -> bool {
    event.author.trim().eq_ignore_ascii_case("user")
        || event.author.to_ascii_lowercase().contains("user")
}

fn recent_user_event_start(
    events: &[jftrade_store_sqlite::StoredAdkEvent],
    window: usize,
) -> usize {
    if events.is_empty() {
        return 0;
    }
    let mut hits = 0;
    for index in (0..events.len()).rev() {
        if !is_user_event(&events[index]) {
            continue;
        }
        hits += 1;
        if hits >= window {
            return index;
        }
    }
    0
}

fn protected_tail_start(events: &[jftrade_store_sqlite::StoredAdkEvent]) -> usize {
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
