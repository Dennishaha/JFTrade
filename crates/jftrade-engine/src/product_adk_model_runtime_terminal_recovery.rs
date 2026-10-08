// Read-only terminal response recovery shared by live and reconnect owners.
// Textually included by the runtime replay fragment.

impl ProductionAdkChatRuntime {
    pub(super) fn recovered_stream_output(
        &self,
        run: &StoredAdkRun,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        let Some(event) = recover_terminal_stream_event(&self.store, &self.session_store, &run.id)?
        else {
            return stream_from_payload(&run.payload_json);
        };
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        payload["status"] = json!(run.status);
        let events = payload
            .get_mut("streamEvents")
            .and_then(Value::as_array_mut)
            .ok_or_else(|| storage_unavailable("stored stream events must be an array"))?;
        if let Some(current) = events
            .iter_mut()
            .find(|current| current["sequence"] == event["sequence"])
        {
            if current["type"] == "error" {
                *current = event;
            }
        } else {
            events.push(event);
        }
        stream_from_payload(&payload.to_string())
    }
}

pub(crate) fn recover_terminal_stream_event(
    store: &AdkStore,
    sessions: &AdkSessionStore,
    run_id: &str,
) -> Result<Option<Value>, AdkChatPortError> {
    let run_id = run_id.trim();
    if run_id.is_empty() {
        return Ok(None);
    }
    let Some(projection) = store
        .read_stream_projection(run_id)
        .map_err(storage_unavailable)?
    else {
        return Ok(None);
    };
    if let Some((_, event)) = &projection.last_event
        && event["type"] == "final"
    {
        return Ok(Some(event.clone()));
    }
    let Some(response) = recover_terminal_response(store, sessions, &projection.run)? else {
        return Ok(None);
    };
    let sequence = projection
        .last_event
        .as_ref()
        .map_or(1, |(sequence, event)| {
            sequence.saturating_add(u64::from(event["type"] != "error"))
        });
    let payload: Value =
        serde_json::from_str(&projection.run.payload_json).map_err(storage_unavailable)?;
    let stream_id = payload["streamId"].as_str().unwrap_or(&projection.run.id);
    Ok(Some(
        json!({"type":"final", "streamId":stream_id, "runId":projection.run.id,
        "sequence":sequence, "response":response}),
    ))
}

fn recover_terminal_response(
    store: &AdkStore,
    sessions: &AdkSessionStore,
    run: &StoredAdkRun,
) -> Result<Option<Value>, AdkChatPortError> {
    if !matches!(
        run.status.trim().to_ascii_uppercase().as_str(),
        "COMPLETED" | "FAILED" | "TIMED_OUT" | "CANCELLED" | "DENIED"
    ) {
        return Ok(None);
    }
    if let Some(response) = persisted_response(&run.payload_json)? {
        return Ok(Some(response));
    }
    let Some(session) = store
        .get_session(&run.session_id)
        .map_err(storage_unavailable)?
    else {
        return Ok(None);
    };
    let payload: Value = serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
    let events = sessions
        .list_events(&run.session_id)
        .map_err(storage_unavailable)?;
    let (reply, reasoning) = recover_transcript_reply(&events, payload["finalMessageId"].as_str());
    let mut session_value: Value =
        serde_json::from_str(&session.payload_json).map_err(storage_unavailable)?;
    let session_object = session_value
        .as_object_mut()
        .ok_or_else(|| storage_unavailable("stored session payload must be an object"))?;
    session_object.insert("id".to_owned(), json!(session.id));
    session_object.insert("createdAt".to_owned(), json!(session.created_at));
    session_object.insert("updatedAt".to_owned(), json!(session.updated_at));
    let pending = payload
        .get("pendingApprovals")
        .filter(|v| v.is_array())
        .cloned()
        .unwrap_or_else(|| json!([]));
    let run_value = recovered_run_value(run, &payload, pending.clone());
    let mut response = json!({"reply":reply, "session":session_value,
        "run":run_value, "pendingApprovals":pending, "timeline":recovered_timeline(&events)});
    if !reasoning.is_empty() {
        response["reasoningContent"] = json!(reasoning);
    }
    if let Some(context) = recover_terminal_context(store, &session, &events) {
        response["context"] = context;
    }
    Ok(Some(response))
}

fn recovered_run_value(run: &StoredAdkRun, payload: &Value, pending: Value) -> Value {
    let tool_calls = payload
        .get("toolCalls")
        .filter(|v| v.is_array())
        .cloned()
        .unwrap_or_else(|| json!([]));
    runtime_projection::go_run_wire(
        payload,
        &recovery_run_fields(),
        vec![
            ("id", json!(run.id)),
            ("sessionId", json!(run.session_id)),
            ("agentId", json!(run.agent_id)),
            ("status", json!(run.status)),
            (
                "workMode",
                json!(normalize_work_mode(
                    payload["workMode"].as_str().unwrap_or_default()
                )),
            ),
            ("createdAt", json!(run.created_at)),
            ("updatedAt", json!(run.updated_at)),
            (
                "message",
                payload.get("message").cloned().unwrap_or_else(|| json!("")),
            ),
            (
                "maxDurationMs",
                payload
                    .get("maxDurationMs")
                    .cloned()
                    .unwrap_or_else(|| json!(0)),
            ),
            ("toolCalls", tool_calls),
            ("pendingApprovals", pending.clone()),
        ],
    )
}

fn recover_transcript_reply(
    events: &[jftrade_store_sqlite::StoredAdkEvent],
    final_id: Option<&str>,
) -> (String, String) {
    let candidates = events
        .iter()
        .filter(|event| {
            !matches!(
                event.author.trim().to_ascii_lowercase().as_str(),
                "user" | "assistant.stream" | "assistant.tool"
            )
        })
        .filter(|event| {
            serde_json::from_str::<Value>(&event.content)
                .ok()
                .is_none_or(|content| content["role"] != "user")
        })
        .filter_map(|event| {
            let text = transcript_text(&event.content);
            (!text.0.is_empty() || !text.1.is_empty()).then_some((event, text))
        })
        .collect::<Vec<_>>();
    final_id
        .and_then(|id| candidates.iter().find(|(event, _)| event.id == id.trim()))
        .or_else(|| candidates.last())
        .map(|(_, text)| text.clone())
        .unwrap_or_default()
}

fn transcript_text(raw: &str) -> (String, String) {
    let Ok(content) = serde_json::from_str::<Value>(raw) else {
        return (raw.to_owned(), String::new());
    };
    if !matches!(
        content["role"].as_str(),
        Some("model" | "assistant" | "user")
    ) {
        return (raw.to_owned(), String::new());
    }
    let Some(parts) = content.get("parts").and_then(Value::as_array) else {
        return (raw.to_owned(), String::new());
    };
    let mut reply = String::new();
    let mut reasoning = String::new();
    for part in parts {
        if let Some(text) = part.get("text").and_then(Value::as_str) {
            if part.get("thought").and_then(Value::as_bool) == Some(true) {
                reasoning.push_str(text);
            } else {
                reply.push_str(text);
            }
        }
    }
    (reply.trim().to_owned(), reasoning.trim().to_owned())
}

fn recovered_timeline(events: &[jftrade_store_sqlite::StoredAdkEvent]) -> Vec<Value> {
    let decoded = events
        .iter()
        .filter_map(|event| {
            let mut event = event.clone();
            let (text, reasoning) = transcript_text(&event.content);
            if text.is_empty() && reasoning.is_empty() {
                return None;
            }
            event.content = text;
            Some(event)
        })
        .collect::<Vec<_>>();
    runtime_projection::session_timeline(&decoded, None)
}

fn recovery_run_fields() -> Vec<&'static str> {
    let mut fields = runtime_stream::GO_RUN_PROJECTION_FIELDS.to_vec();
    fields.extend([
        "optimizationTaskId",
        "childRunIds",
        "iteration",
        "workflowStatus",
        "workflowEngine",
        "workflowCursor",
        "workflowPlan",
        "inputRequest",
        "inputRequests",
        "pauseRequestedAt",
        "pausedAt",
        "pausedReason",
    ]);
    fields
}

fn recover_terminal_context(
    store: &AdkStore,
    session: &jftrade_store_sqlite::StoredAdkEntity,
    events: &[jftrade_store_sqlite::StoredAdkEvent],
) -> Option<Value> {
    use crate::product::product_production_ports::product_production_ports_adk::read::{
        context_projection, context_window,
    };
    let state = store.get_session_context(&session.id).ok()?;
    let stored = state
        .map(|row| serde_json::from_str::<Value>(&row.payload_json))
        .transpose()
        .ok()?
        .unwrap_or_else(|| json!({}));
    let segments = store.list_handoff_segments(&session.id, true).ok()?;
    let window = context_window::resolve_session_context_window_tokens(
        store,
        &session.id,
        &session.payload_json,
    );
    let recent = context_window::resolve_session_context_recent_window(
        store,
        &session.id,
        &session.payload_json,
    );
    context_projection::refresh_context_snapshot(
        &session.id,
        &session.payload_json,
        &stored,
        events,
        &segments,
        window,
        recent,
    )
    .ok()
}
