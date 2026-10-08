// Durable ADK chat projection replay: tool-failure degradation and
// stream-frame recovery, textually included by
// `product_adk_model_runtime_adapters.rs` so the items stay in the same
// module scope as the rest of the runtime.

include!("product_adk_model_runtime_terminal_recovery.rs");

fn check_durable_request_identity(
    store: &AdkStore,
    input: &AdkChatInput,
) -> Result<(), AdkChatPortError> {
    let failure = |message: String| AdkChatPortError::Failed {
        status: 500,
        code: "ADK_CHAT_FAILED".to_owned(),
        message,
    };
    let Some(existing) = store
        .get_run_by_client_request_id(&input.client_request_id)
        .map_err(|error| failure(error.to_string()))?
    else {
        return Ok(());
    };
    // Go's ChatRunByClientRequestID decodes the persisted run before comparing
    // fingerprints. A corrupt row must not be hidden by a transport record.
    let _: Option<serde_json::Map<String, Value>> =
        serde_json::from_str(&existing.payload_json).map_err(|error| failure(error.to_string()))?;
    if !crate::product::product_adk_chat_identity::ChatRequestIdentity::decode(&input.body)?
        .matches(&existing.request_fingerprint)
    {
        return Err(AdkChatPortError::Conflict(
            "clientRequestId was already used with a different request".to_owned(),
        ));
    }
    Ok(())
}

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

/// Build the terminal chat envelope for a run whose released tool calls just
/// finished, or raise the persisted failure when the loop did not complete it.
///
/// Go's `CompleteChatRun` returns `ProjectedChatResponse` after
/// `PersistRunTerminalState`, so the caller reads whatever terminal projection
/// the run loop persisted instead of inventing a response from the release
/// decision.
impl ProductionAdkChatRuntime {
    fn prepare_existing_run(
        &self,
        existing: StoredAdkRun,
        route: AdkChatRoute,
        identity: &crate::product::product_adk_chat_identity::ChatRequestIdentity,
    ) -> Result<PreparedChat, AdkChatPortError> {
        if !identity.matches(&existing.request_fingerprint) {
            return Err(AdkChatPortError::Conflict(
                "clientRequestId was already used with a different request".to_owned(),
            ));
        }
        let payload: Value =
            serde_json::from_str(&existing.payload_json).map_err(storage_unavailable)?;
        let persisted_route = payload
            .get("route")
            .and_then(Value::as_str)
            .unwrap_or("chat");
        let requested_route = match route {
            AdkChatRoute::Chat => "chat",
            AdkChatRoute::Stream => "stream",
        };
        if persisted_route != requested_route {
            return Err(AdkChatPortError::Conflict(
                "clientRequestId was already used on a different chat route".to_owned(),
            ));
        }
        if route == AdkChatRoute::Stream
            && matches!(
                existing.status.to_ascii_uppercase().as_str(),
                "COMPLETED" | "FAILED" | "CANCELLED" | "TIMED_OUT" | "DENIED"
            )
        {
            return self
                .recovered_stream_output(&existing)
                .map(PreparedChat::Existing);
        }
        if let Some(response) = persisted_response(&existing.payload_json)? {
            return Ok(PreparedChat::Existing(match route {
                AdkChatRoute::Chat => AdkChatPortOutput::Json(response),
                AdkChatRoute::Stream => stream_from_payload(&existing.payload_json)?,
            }));
        }
        if matches!(
            existing.status.to_ascii_uppercase().as_str(),
            "FAILED" | "TIMED_OUT" | "CANCELLED"
        ) && route == AdkChatRoute::Chat
        {
            return Err(replayed_run_error(&payload, &existing.status));
        }
        if existing.status.eq_ignore_ascii_case("RUNNING")
            && runtime_recovery::retry_is_due(&payload)
        {
            match self.resume_approval(&existing.id) {
                Ok(()) | Err(AdkChatPortError::Conflict(_)) => {}
                Err(AdkChatPortError::Unavailable(_)) => {}
                Err(error) => return Err(error),
            }
        }
        existing_run_output(&existing, route).map(PreparedChat::Existing)
    }

    pub(super) fn persisted_turn_response(&self, run_id: &str) -> Result<Value, AdkChatPortError> {
        let run = self
            .store
            .get_run(run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        if run.status.eq_ignore_ascii_case("COMPLETED")
            && let Some(response) = persisted_response(&run.payload_json)?
        {
            return Ok(response);
        }
        match run.status.to_ascii_uppercase().as_str() {
            "COMPLETED" => persisted_response(&run.payload_json)?
                .ok_or_else(|| unavailable("completed ADK run has no persisted response")),
            "FAILED" | "TIMED_OUT" | "CANCELLED" | "DENIED" => Err(replayed_run_error(
                &serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?,
                &run.status,
            )),
            _ => existing_run_output(&run, AdkChatRoute::Chat).and_then(|output| match output {
                AdkChatPortOutput::Json(response) => Ok(response),
                _ => Err(unavailable("persisted ADK run has no JSON projection")),
            }),
        }
    }
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

/// Go's hub marks every frame a reconnecting client receives as
/// `replay: true` (see `streamADKChatRecord`: events with a sequence at or
/// below the replay watermark carry the flag).  The durable Rust projection
/// answers the whole retained history on a `GET /streams/{id}` or
/// `GET /runs/{id}/stream` reconnect, so each frame is replayed history by
/// definition and the marker must be present for the wire contract.
fn mark_replayed(event: &Value) -> Value {
    let mut event = event.clone();
    if let Some(object) = event.as_object_mut() {
        object.insert("replay".to_owned(), Value::Bool(true));
    }
    event
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
                data: mark_replayed(event),
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
            data: mark_replayed(&json!({
                "type": "final",
                "streamId": stream_id,
                "sequence": sequence,
                "response": response,
            })),
        });
    }
    Ok(AdkChatPortOutput::Stream(AdkChatStreamSnapshot {
        headers: BTreeMap::from([(String::from("X-ADK-Stream-ID"), stream_id)]),
        frames,
        terminal,
    }))
}
