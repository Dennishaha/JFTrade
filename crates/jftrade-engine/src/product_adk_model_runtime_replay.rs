// Durable ADK chat projection replay: tool-failure degradation and
// stream-frame recovery, textually included by
// `product_adk_model_runtime_adapters.rs` so the items stay in the same
// module scope as the rest of the runtime.

/// The first TIMED_OUT/FAILED/CANCELLED tool call message, mirroring Go's
/// `FirstToolCallFailure` (`FirstToolCallByStatus` + `ToolCallFailureMessage`).
///
/// Go keeps a chat run `COMPLETED` when only a tool failed and reports that
/// shape through `Run.degraded = toolFailure != ""`, so the failure text itself
/// never fails the run.  A `RUNNING` or `PENDING_APPROVAL` call is not a
/// failure and must not mark the run degraded.
fn first_tool_call_failure(payload_json: &str) -> Option<String> {
    let payload: Value = serde_json::from_str(payload_json).ok()?;
    payload
        .get("toolCalls")
        .and_then(Value::as_array)?
        .iter()
        .find(|call| {
            call.get("status")
                .and_then(Value::as_str)
                .map(str::trim)
                .is_some_and(|status| {
                    status.eq_ignore_ascii_case("TIMED_OUT")
                        || status.eq_ignore_ascii_case("FAILED")
                        || status.eq_ignore_ascii_case("CANCELLED")
                })
        })
        .map(|call| {
            let error = call
                .get("error")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty());
            if let Some(error) = error {
                return error.to_owned();
            }
            let status = call
                .get("status")
                .and_then(Value::as_str)
                .map(str::trim)
                .unwrap_or_default();
            if status.eq_ignore_ascii_case("TIMED_OUT") {
                "tool execution timed out".to_owned()
            } else if status.eq_ignore_ascii_case("CANCELLED") {
                "tool execution cancelled".to_owned()
            } else {
                "tool execution failed".to_owned()
            }
        })
}

fn persisted_response(raw: &str) -> Result<Option<Value>, AdkChatPortError> {
    let value: Value = serde_json::from_str(raw).map_err(storage_unavailable)?;
    Ok(value.get("response").cloned())
}

fn replayed_run_error(payload: &Value, status: &str) -> AdkChatPortError {
    let default_status = if status.eq_ignore_ascii_case("TIMED_OUT") {
        504
    } else if status.eq_ignore_ascii_case("CANCELLED") {
        499
    } else {
        500
    };
    let http_status = payload
        .get("errorStatus")
        .and_then(Value::as_u64)
        .and_then(|value| u16::try_from(value).ok())
        .unwrap_or(default_status);
    let default_code = if status.eq_ignore_ascii_case("TIMED_OUT") {
        "MODEL_CALL_TIMEOUT"
    } else if status.eq_ignore_ascii_case("CANCELLED") {
        "RUN_CANCELLED"
    } else {
        "ADK_CHAT_FAILED"
    };
    let code = payload
        .get("errorCode")
        .and_then(Value::as_str)
        .unwrap_or(default_code)
        .to_owned();
    let message = payload
        .get("errorMessage")
        .or_else(|| payload.get("message"))
        .and_then(Value::as_str)
        .unwrap_or("assistant chat run failed")
        .to_owned();
    AdkChatPortError::Failed {
        status: http_status,
        code,
        message,
    }
}

fn existing_run_output(
    run: &StoredAdkRun,
    route: AdkChatRoute,
) -> Result<AdkChatPortOutput, AdkChatPortError> {
    let payload: Value = serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
    if let Some(response) = payload.get("response").cloned() {
        return match route {
            AdkChatRoute::Chat => Ok(AdkChatPortOutput::Json(response)),
            AdkChatRoute::Stream => stream_from_payload(&run.payload_json),
        };
    }
    let reply = payload
        .get("reply")
        .or_else(|| payload.get("message"))
        .and_then(Value::as_str)
        .unwrap_or_default();
    let pending = payload
        .get("pendingApprovals")
        .cloned()
        .unwrap_or_else(|| Value::Array(Vec::new()));
    let session = json!({
        "id": run.session_id,
        "agentId": run.agent_id,
        "createdAt": run.created_at,
        "updatedAt": run.updated_at,
    });
    let projection = json!({
        "reply": reply,
        "session": session,
        "run": payload,
        "pendingApprovals": pending,
        "timeline": [],
    });
    match route {
        AdkChatRoute::Chat => Ok(AdkChatPortOutput::Json(projection)),
        AdkChatRoute::Stream => stream_from_payload(&run.payload_json),
    }
}

fn stream_from_payload(raw: &str) -> Result<AdkChatPortOutput, AdkChatPortError> {
    let value: Value = serde_json::from_str(raw).map_err(storage_unavailable)?;
    let stream_id = value
        .get("streamId")
        .and_then(Value::as_str)
        .filter(|id| !id.trim().is_empty())
        .ok_or_else(|| unavailable("persisted ADK run has no stream id"))?
        .to_owned();
    let events = value
        .get("streamEvents")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let terminal = value
        .get("status")
        .and_then(Value::as_str)
        .is_some_and(|status| {
            matches!(
                status.to_ascii_uppercase().as_str(),
                "COMPLETED" | "FAILED" | "TIMED_OUT" | "CANCELLED" | "DENIED" | "PENDING"
            )
        });
    let mut frames: Vec<AdkChatStreamFrame> = events
        .iter()
        .map(|event| {
            let id = event
                .get("sequence")
                .and_then(Value::as_u64)
                .map(|sequence| format!("{stream_id}:{sequence}"));
            AdkChatStreamFrame::Event {
                id,
                data: event.clone(),
            }
        })
        .collect();
    // Go's `publishTerminalError` recovers a persisted terminal run through
    // `RecoverTerminalChatResponse` and publishes the result as the `final`
    // event instead of a second `error`.  A durable terminal run whose stream
    // event list never received that frame (a Go-authored row, or a crash
    // between the run commit and the append) must therefore replay as a
    // synthesized `final` frame built from the persisted response.
    let has_terminal_frame = events.last().is_some_and(|event| {
        event
            .get("type")
            .and_then(Value::as_str)
            .is_some_and(|kind| matches!(kind, "final" | "error"))
    });
    if terminal
        && !has_terminal_frame
        && let Some(response) = value.get("response")
    {
        let sequence = events.len() as u64 + 1;
        frames.push(AdkChatStreamFrame::Event {
            id: Some(format!("{stream_id}:{sequence}")),
            data: json!({
                "type": "final",
                "streamId": stream_id,
                "sequence": sequence,
                "response": response,
            }),
        });
    }
    Ok(AdkChatPortOutput::Stream(AdkChatStreamSnapshot {
        headers: BTreeMap::from([(String::from("X-ADK-Stream-ID"), stream_id)]),
        frames,
        terminal,
    }))
}

