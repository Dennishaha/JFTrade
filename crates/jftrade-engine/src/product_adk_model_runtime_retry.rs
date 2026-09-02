impl ProductionAdkChatRuntime {
    /// Keep a provider outage durable without turning it into a terminal run.
    /// The caller still receives the original 502/503/504 response, while a
    /// later supervisor pass can acquire a fresh fenced lease and continue.
    fn persist_provider_retry(
        &self,
        chat: &ChatExecution,
        error: &AdkChatPortError,
        run_lease: &RunLeaseGuard,
    ) -> Result<(), AdkChatPortError> {
        let run = self
            .store
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        if !run.status.eq_ignore_ascii_case("RUNNING") {
            return Ok(());
        }
        let payload: Value = serde_json::from_str(&run.payload_json)
            .map_err(storage_unavailable)?;
        self.persist_provider_retry_with_lease(
            &chat.run_id,
            &chat.session_id,
            &run,
            &payload,
            error,
            run_lease,
            true,
        )
    }

    #[allow(clippy::too_many_arguments)]
    fn persist_provider_retry_with_lease(
        &self,
        run_id: &str,
        session_id: &str,
        run: &StoredAdkRun,
        payload: &Value,
        error: &AdkChatPortError,
        run_lease: &RunLeaseGuard,
        retryable: bool,
    ) -> Result<(), AdkChatPortError> {
        if !run.status.eq_ignore_ascii_case("RUNNING") {
            return Ok(());
        }
        let mut payload = payload.clone();
        let object = payload
            .as_object_mut()
            .ok_or_else(|| AdkChatPortError::Failed {
                status: 500,
                code: "ADK_STORAGE_CORRUPT".to_owned(),
                message: "stored ADK run payload must be an object".to_owned(),
            })?;
        let previous_attempt = object
            .get("providerRetry")
            .and_then(Value::as_object)
            .and_then(|retry| retry.get("attempt"))
            .and_then(Value::as_u64)
            .unwrap_or_default();
        let attempt = previous_attempt.saturating_add(1);
        // A non-retryable provider-resolution failure is only probed again
        // after the maximum interval. This preserves a durable RUNNING
        // record for a later settings fix without polling every second.
        let delay_ms = if retryable {
            runtime_recovery::retry_delay_ms(attempt)
        } else {
            runtime_recovery::max_retry_delay_ms()
        };
        let next_retry_at_unix_ms = runtime_recovery::unix_now_ms().saturating_add(delay_ms);
        let next_retry_at = runtime_recovery::retry_timestamp(next_retry_at_unix_ms);
        let (status, code, message) = runtime_recovery::retry_details(error);
        object.insert(
            "resumeState".to_owned(),
            Value::String("provider_waiting".to_owned()),
        );
        object.insert(
            "providerRetry".to_owned(),
            json!({
                "attempt": attempt,
                "retryable": retryable,
                "nextRetryAt": next_retry_at,
                "nextRetryAtUnixMs": next_retry_at_unix_ms,
                "lastError": {
                    "status": status,
                    "code": code,
                    "message": message,
                },
            }),
        );
        let mut event_row = None;
        if object
            .get("route")
            .and_then(Value::as_str)
            .is_some_and(|route| route.eq_ignore_ascii_case("stream"))
        {
            let events = object
                .get_mut("streamEvents")
                .and_then(Value::as_array_mut)
                .ok_or_else(|| unavailable("persisted ADK run has no stream event list"))?;
            let sequence = events.len() as u64 + 1;
            let event = json!({
                "type": "error",
                "message": format_adk_error(error),
                "retryable": retryable,
                "terminal": false,
                "retryAt": next_retry_at,
                "streamId": run_id,
                "sequence": sequence,
                "runId": run_id,
            });
            events.push(event);
            let event_id = format!("{run_id}:stream:{sequence}");
            let content = format_adk_error(error);
            event_row = Some((event_id, content));
        }
        let updated = match event_row.as_ref() {
            Some((event_id, content)) => self.store.update_run_payload_if_status_and_revision_with_events_with_lease(
                run_id,
                "RUNNING",
                &run.updated_at,
                &payload.to_string(),
                self.session_store.as_ref(),
                &[AdkRunEvent {
                    id: event_id,
                    session_id,
                    invocation_id: run_id,
                    author: "assistant.provider",
                    content,
                }],
                run_lease.owner_id(),
                run_lease.token(),
            ),
            None => self.store.update_run_payload_if_status_and_revision_with_lease(
                run_id,
                "RUNNING",
                &run.updated_at,
                &payload.to_string(),
                run_lease.owner_id(),
                run_lease.token(),
            ),
        }
        .map_err(runtime_store_error)?;
        if !updated {
            return Err(AdkChatPortError::Conflict(
                "assistant chat run changed before provider retry scheduling".to_owned(),
            ));
        }
        Ok(())
    }

    /// Mark the start of a fresh provider attempt and discard the previous
    /// retry marker. This mutation is fenced so a takeover cannot clear a
    /// newer retry scheduled by another worker.
    fn mark_provider_attempt_started(
        &self,
        chat: &ChatExecution,
        run_lease: &RunLeaseGuard,
    ) -> Result<(), AdkChatPortError> {
        let run = self
            .store
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        if !run.status.eq_ignore_ascii_case("RUNNING") {
            return Ok(());
        }
        let mut payload: Value = serde_json::from_str(&run.payload_json)
            .map_err(storage_unavailable)?;
        if let Some(object) = payload.as_object_mut() {
            object.insert(
                "resumeState".to_owned(),
                Value::String("provider_executing".to_owned()),
            );
            object.remove("providerRetry");
        }
        let updated = self
            .store
            .update_run_payload_if_status_and_revision_with_lease(
                &chat.run_id,
                "RUNNING",
                &run.updated_at,
                &payload.to_string(),
                run_lease.owner_id(),
                run_lease.token(),
            )
            .map_err(runtime_store_error)?;
        if !updated {
            return Err(AdkChatPortError::Conflict(
                "assistant chat run changed before provider attempt".to_owned(),
            ));
        }
        Ok(())
    }
}

/// Classify model-provider failures. Storage corruption, cancellation, lease
/// fencing and malformed requests remain coordination/terminal outcomes and
/// are never retried by the background worker.
pub(super) fn is_provider_retryable_error(error: &AdkChatPortError) -> bool {
    match error {
        // Provider resolution/configuration failures are represented as
        // Unavailable. Keep them durable and back off so a later settings
        // change can recover the run, but never let storage/lease failures
        // enter the provider retry loop.
        AdkChatPortError::Unavailable(message) => {
            let message = message.to_ascii_lowercase();
            !message.contains("storage")
                && !message.contains("lease")
                && !message.contains("cancel")
                && (message.contains("provider") || message.contains("model"))
        }
        AdkChatPortError::Conflict(_) => false,
        AdkChatPortError::Failed {
            status,
            code,
            ..
        } => match code.as_str() {
            "MODEL_CALL_TIMEOUT" | "MODEL_PROVIDER_RATE_LIMITED" => true,
            "MODEL_CALL_FAILED" => matches!(*status, 408 | 425 | 429 | 500 | 502 | 503 | 504),
            "MODEL_PROVIDER_UNAUTHORIZED"
            | "MODEL_PROVIDER_FORBIDDEN"
            | "MODEL_PROVIDER_UNAVAILABLE"
            | "CLIENT_DISCONNECTED"
            | "RUN_CANCELLED"
            | "ADK_STORAGE_CORRUPT"
            | "ADK_TOOL_UNAVAILABLE"
            | "ADK_TOOL_LOOP_LIMIT" => false,
            _ => false,
        },
    }
}
