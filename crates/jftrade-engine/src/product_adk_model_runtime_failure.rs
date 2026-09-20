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
        Ok(())
    }
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
