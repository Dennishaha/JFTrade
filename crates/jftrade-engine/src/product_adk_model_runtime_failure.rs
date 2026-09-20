// Terminal-failure persistence for the production ADK chat runtime.
//
// Textually included by `product_adk_model_runtime_stream.rs` so the runtime
// impl stays in one module scope while each fragment respects the workspace
// 800-line production limit.

impl ProductionAdkChatRuntime {
    pub(super) fn persist_failure(
        &self,
        chat: &ChatExecution,
        error: &AdkChatPortError,
        run_lease: &RunLeaseGuard,
    ) -> Result<(), AdkChatPortError> {
        // Go's `markFailedChatRun` writes `adkErr.Error()` to both `Message` and
        // `FailureReason`; the classified code lives on `ErrorCode`.  The
        // frozen provider-failure fixture shows the raw provider text in both
        // fields with no `CODE: ` prefix.
        let (error_status, message) = match error {
            AdkChatPortError::Unavailable(message) => (503, message.clone()),
            AdkChatPortError::Conflict(message) => (409, message.clone()),
            AdkChatPortError::Failed { status, message, .. } => (*status, message.clone()),
        };
        let error_message = message.clone();
        let (status, error_code) = run_terminal_state(error);
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        if run.status.eq_ignore_ascii_case("CANCELLED") {
            return Err(run_cancelled());
        }
        if !run.status.eq_ignore_ascii_case("RUNNING") {
            if matches!(
                run.status.to_ascii_uppercase().as_str(),
                "COMPLETED" | "FAILED" | "TIMED_OUT"
            ) {
                return Ok(());
            }
            return Err(unavailable(
                "assistant chat run state changed before failure",
            ));
        }
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        payload["id"] = Value::String(chat.run_id.clone());
        payload["status"] = Value::String(status.to_owned());
        // Go's `markFailedChatRun` keeps the raw error text on `Message` and
        // `FailureReason`, and the classified run-level code on `ErrorCode`.
        payload["message"] = Value::String(message.clone());
        payload["failureReason"] = Value::String(message.clone());
        payload["errorStatus"] = Value::from(error_status);
        payload["errorCode"] = Value::String(error_code.to_owned());
        payload["errorMessage"] = Value::String(error_message.clone());
        // Go's `MarkFailedChatRun` stamps `CompletedAt` on every terminal
        // failure and additionally sets `CancelledAt` for a cancellation, so
        // the console can distinguish an interrupted run from a clean one.
        payload["completedAt"] = Value::String(run.updated_at.clone());
        // Go's `MarkFailedChatRun` sets `Degraded = true` on every terminal
        // failure.
        payload["degraded"] = Value::Bool(true);
        // Go's `failInputContinuation` overwrites `resumeState` with
        // `input_resume_failed` after `markFailedChatRun`, so the console can
        // tell a dead input continuation from a plain provider outage.
        if chat.resumed_from_input {
            payload["resumeState"] = Value::String("input_resume_failed".to_owned());
        }
        if let Some(object) = payload.as_object_mut() {
            object.remove("providerRetry");
        }
        let mut stream_event_id = None;
        let mut stream_event_content = None;
        if chat.route == AdkChatRoute::Stream {
            let has_terminal = payload
                .get("streamEvents")
                .and_then(Value::as_array)
                .and_then(|events| events.last())
                .and_then(|event| event.get("type"))
                .and_then(Value::as_str)
                .is_some_and(|kind| matches!(kind, "final" | "error"));
            if !has_terminal {
                let sequence = payload
                    .get("streamEvents")
                    .and_then(Value::as_array)
                    .map_or(1, |events| events.len() as u64 + 1);
                let mut event = json!({"type":"error","message":message});
                if let Some(object) = event.as_object_mut() {
                    object.insert("streamId".to_owned(), Value::String(chat.run_id.clone()));
                    object.insert("sequence".to_owned(), Value::from(sequence));
                    object.insert("runId".to_owned(), Value::String(chat.run_id.clone()));
                }
                payload
                    .get_mut("streamEvents")
                    .and_then(Value::as_array_mut)
                    .ok_or_else(|| unavailable("persisted ADK run has no stream event list"))?
                    .push(event);
                stream_event_id = Some(format!("{}:stream:{}", chat.run_id, sequence));
                stream_event_content = Some(message.clone());
            }
        }
        payload["status"] = Value::String(status.to_owned());
        payload["message"] = Value::String(message.clone());
        let updated = match (stream_event_id.as_ref(), stream_event_content.as_ref()) {
            (Some(event_id), Some(content)) => self
                .store()
                .update_run_state_if_status_and_revision_with_events_with_lease(
                    &chat.run_id,
                    "RUNNING",
                    &run.updated_at,
                    status,
                    &payload.to_string(),
                    self.session_store.as_ref(),
                    &[AdkRunEvent {
                        id: event_id,
                        session_id: &chat.session_id,
                        invocation_id: &chat.run_id,
                        author: "assistant.stream",
                        content,
                    }],
                    run_lease.owner_id(),
                    run_lease.token(),
                ),
            _ => self
                .store()
                .update_run_state_if_status_and_revision_with_lease(
                    &chat.run_id,
                    "RUNNING",
                    &run.updated_at,
                    status,
                    &payload.to_string(),
                    run_lease.owner_id(),
                    run_lease.token(),
                ),
        }
        .map_err(storage_unavailable)?;
        if !updated {
            let current = self
                .store()
                .get_run(&chat.run_id)
                .map_err(storage_unavailable)?
                .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
            if current.status.eq_ignore_ascii_case("CANCELLED") {
                return Err(run_cancelled());
            }
            if current.status.eq_ignore_ascii_case(status) {
                return Ok(());
            }
            return Err(unavailable(
                "assistant chat run or execution lease changed before failure",
            ));
        }
        // Go's `PersistRunTerminalState` audits the status-specific lifecycle
        // kind, carrying the failure reason so the audit row explains the
        // terminal state without a second run read.
        self.record_run_audit(&RunAuditEvent {
            id: format!("{}:audit:{}", chat.run_id, lifecycle_audit_kind(status)),
            subject_id: chat.run_id.clone(),
            kind: lifecycle_audit_kind(status),
            detail: terminal_audit_message(status),
            // Go's `markFailedChatRun` keeps the raw error text on
            // `FailureReason` and the classified code on `ErrorCode`.
            metadata: terminal_audit_fields(
                &chat.run_id,
                &chat.agent_id,
                status,
                error_code,
                &error_message,
            ),
        });
        // Go answers `200` with a FAILED run plus a synthetic reply, so the
        // terminal row is immediately followed by
        // `AttachFinalAssistantMessage` + `ProjectedChatResponse`.  Persisting
        // that projection here keeps every replay path identical.
        self.attach_terminal_failure_projection(chat, &mut payload, error, run_lease)?;
        Ok(())
    }

    /// Attach the synthetic assistant message Go writes for a terminal
    /// failure and store the resulting projection on the run payload.
    ///
    /// Go's `CompleteChatRun` does not stop at `markFailedChatRun` +
    /// `PersistRunTerminalState`: it rewrites `replyResult` to the
    /// `userFacingADKError(adkErr)` text with `SyntheticKind="provider_error"`,
    /// calls `AttachFinalAssistantMessage` (which appends a session transcript
    /// entry and links `run.FinalMessageID` at it), and finally returns
    /// `ProjectedChatResponse`.  The chat handler therefore answers
    /// `200 ok=true` with a FAILED run plus `reply`, and the frozen
    /// `chat-provider-failure` fixture pins that shape.  Persisting the same
    /// projection here is what lets every replay path
    /// (`prepare_existing_run`, `persisted_turn_response`, retained stream
    /// frames) serve the identical envelope instead of inventing a 5xx.
    pub(super) fn attach_terminal_failure_projection(
        &self,
        chat: &ChatExecution,
        payload: &mut Value,
        error: &AdkChatPortError,
        run_lease: &RunLeaseGuard,
    ) -> Result<(), AdkChatPortError> {
        let reply = user_facing_adk_error(error);
        let message_id = synthetic_assistant_message_id(&chat.run_id, "provider_error", "", &reply);
        let payload_object = payload
            .as_object_mut()
            .ok_or_else(|| unavailable("stored ADK run payload must be an object"))?;
        // Go links the run to the transcript entry it just appended, so the
        // console can render the failure text from the timeline instead of
        // only from `reply`.
        payload_object.insert(
            "finalMessageId".to_owned(),
            Value::String(message_id.clone()),
        );
        let stored = self
            .store()
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        // The durable payload also carries runtime bookkeeping
        // (`streamEvents`, `providerEvents`, `route`, `toolResults`, ...) that
        // Go's `Run` JSON contract never exposes.  The frozen
        // `chat-provider-failure` fixture pins the exposed field set, so the
        // wire projection keeps only those keys, drops the zero-valued fields
        // Go tags `omitempty`, and republishes the projected tool activity.
        let tool_calls: Vec<Value> = payload_object
            .get("toolCalls")
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();
        let completed_at = payload_object
            .get("completedAt")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_owned();
        let mut overrides: Vec<(&str, Value)> = vec![
            ("createdAt", Value::String(stored.created_at.clone())),
            ("updatedAt", Value::String(stored.updated_at.clone())),
            (
                "usage",
                runtime_projection::usage_wire_value(
                    &Value::Object(payload_object.clone()),
                    &tool_calls,
                    &completed_at,
                ),
            ),
        ];
        overrides.extend(runtime_projection::tool_projection_fields(&tool_calls));
        let run_value = runtime_projection::go_run_wire(
            &Value::Object(payload_object.clone()),
            GO_RUN_PROJECTION_FIELDS,
            overrides,
        );
        let session = self
            .store()
            .get_session(&chat.session_id)
            .map_err(storage_unavailable)?
            .map(|session| {
                let title = serde_json::from_str::<Value>(&session.payload_json)
                    .ok()
                    .and_then(|payload| {
                        payload
                            .get("title")
                            .and_then(Value::as_str)
                            .map(str::to_owned)
                    })
                    .unwrap_or_default();
                json!({
                    "id": session.id,
                    "agentId": chat.agent_id,
                    "title": title,
                    "createdAt": session.created_at,
                    "updatedAt": session.updated_at,
                })
            })
            .unwrap_or_else(|| json!({"id": chat.session_id, "agentId": chat.agent_id}));
        let timeline = self.session_timeline_value(
            &chat.session_id,
            Some(runtime_projection::PendingTimelineEntry {
                id: &message_id,
                session_id: &chat.session_id,
                run_id: &chat.run_id,
                text: &reply,
                created_at: &completed_at,
            }),
        )?;
        let response = json!({
            "reply": reply,
            "session": session,
            "run": run_value,
            "pendingApprovals": [],
            "timeline": timeline,
        });
        let mut committed = payload.clone();
        committed["response"] = response.clone();
        // A terminal failure is served to reconnecting stream clients through
        // the retained history, so the projection is the last frame the way
        // Go's `publishTerminalError` -> `RecoverTerminalChatResponse` path
        // publishes `final` instead of a bare `error` event.
        if chat.route == AdkChatRoute::Stream {
            let events = committed
                .get_mut("streamEvents")
                .and_then(Value::as_array_mut)
                .ok_or_else(|| unavailable("persisted ADK run has no stream event list"))?;
            let has_terminal = events
                .last()
                .and_then(|event| event.get("type"))
                .and_then(Value::as_str)
                .is_some_and(|kind| matches!(kind, "final" | "error"));
            if !has_terminal {
                let sequence = events.len() as u64 + 1;
                let mut final_event = json!({"type": "final", "response": response.clone()});
                if let Some(object) = final_event.as_object_mut() {
                    object.insert("streamId".to_owned(), Value::String(chat.run_id.clone()));
                    object.insert("sequence".to_owned(), Value::from(sequence));
                    object.insert("runId".to_owned(), Value::String(chat.run_id.clone()));
                }
                events.push(final_event);
            }
        }
        let updated = self
            .store()
            .update_run_payload_if_status_and_revision_with_events_with_lease(
                &chat.run_id,
                &stored.status,
                &stored.updated_at,
                &committed.to_string(),
                self.session_store.as_ref(),
                &[AdkRunEvent {
                    id: &message_id,
                    session_id: &chat.session_id,
                    invocation_id: &chat.run_id,
                    author: &chat.agent_id,
                    content: &reply,
                }],
                run_lease.owner_id(),
                run_lease.token(),
            )
            .map_err(storage_unavailable)?;
        if !updated {
            return Err(unavailable(
                "assistant chat run state changed before the final message was attached",
            ));
        }
        *payload = committed;
        Ok(())
    }
}

/// The run fields Go's `assistantmodel.Run` exposes on the JSON wire.
///
/// The frozen `api-transport/adk-chat-stream.json` `chat-provider-failure`
/// case enumerates this set, so the terminal-failure projection must not leak
/// the durable payload's internal bookkeeping keys alongside them.
pub(super) const GO_RUN_PROJECTION_FIELDS: &[&str] = &[
    "agentId",
    "cancelledAt",
    "completedAt",
    "createdAt",
    "degraded",
    "errorCode",
    "failureReason",
    "finalMessageId",
    "id",
    "maxDurationMs",
    "message",
    "model",
    "objective",
    "parentRunId",
    "pendingApprovals",
    "permissionMode",
    "preToolContent",
    "preToolReasoning",
    "providerId",
    "providerName",
    "resumeState",
    "sessionId",
    "startedAt",
    "status",
    "toolCalls",
    "toolSummaries",
    "updatedAt",
    "usage",
    "userMessage",
    "workMode",
];

/// Derive the deterministic assistant-message id Go writes for a synthetic
/// reply.
///
/// Go's `syntheticAssistantMessageID` hashes `kind \0 reasoning \0 reply` with
/// SHA-256 and keeps the first eight bytes as lowercase hex, so a retried
/// terminal projection reuses the same transcript entry instead of appending a
/// duplicate.  The `kind` falls back to `local` when the caller does not
/// classify the reply; a provider outage always passes `provider_error`,
/// exactly like `CompleteChatRun`.
pub(super) fn synthetic_assistant_message_id(
    run_id: &str,
    kind: &str,
    reasoning: &str,
    reply: &str,
) -> String {
    use sha2::{Digest, Sha256};

    let kind = if kind.trim().is_empty() {
        "local"
    } else {
        kind.trim()
    };
    let mut digest = Sha256::new();
    digest.update(kind.as_bytes());
    digest.update([0_u8]);
    digest.update(reasoning.trim().as_bytes());
    digest.update([0_u8]);
    digest.update(reply.trim().as_bytes());
    let mut suffix = String::with_capacity(16);
    for byte in digest.finalize().iter().take(8) {
        suffix.push_str(&format!("{byte:02x}"));
    }
    format!("jftrade-{}-{kind}-{suffix}", run_id.trim())
}

/// Go's `userFacingADKError`: map the two error shapes the console can explain
/// to an operator, and fall back to the original provider text.
pub(super) fn user_facing_adk_error(error: &AdkChatPortError) -> String {
    let text = match error {
        AdkChatPortError::Unavailable(message) | AdkChatPortError::Conflict(message) => {
            message.clone()
        }
        AdkChatPortError::Failed { message, .. } => message.clone(),
    };
    let lower = text.to_ascii_lowercase();
    if lower.contains("wrote more than the declared content-length") {
        return "模型服务响应异常，请检查模型服务配置或稍后重试。".to_owned();
    }
    if lower.contains("database is locked") || lower.contains("sqlite_busy") {
        return "数据库繁忙，请稍后重试。".to_owned();
    }
    text
}

/// Go's `RunStatusForContext` + `RunErrorCode` table for a terminal chat run.
///
/// The surrounding context decides the terminal status and the matching
/// *run-level* code, so a model timeout is `TIMED_OUT`/`RUN_TIMED_OUT` (never
/// the provider's own `MODEL_CALL_TIMEOUT`), an unsupported GO-ADK input
/// request is `FAILED`/`ADK_INPUT_UNSUPPORTED` (checked before the status
/// switch, exactly like the reference), and every other failure is
/// `FAILED`/`MODEL_CALL_FAILED`.
pub(super) fn run_terminal_state(error: &AdkChatPortError) -> (&'static str, &'static str) {
    if matches!(
        error,
        AdkChatPortError::Failed { code, .. } if code == "ADK_INPUT_UNSUPPORTED"
    ) {
        return ("FAILED", "ADK_INPUT_UNSUPPORTED");
    }
    if matches!(
        error,
        AdkChatPortError::Failed { code, .. } if code == "MODEL_CALL_TIMEOUT"
    ) {
        return ("TIMED_OUT", "RUN_TIMED_OUT");
    }
    ("FAILED", "MODEL_CALL_FAILED")
}
