impl ProductionAdkChatRuntime {
    fn recover_approval_continuations(&self) {
        let Ok(runs) = self.store.list_runs() else {
            return;
        };
        for run in runs {
            if !run.status.eq_ignore_ascii_case("RUNNING") {
                continue;
            }
            let Ok(payload) = serde_json::from_str::<Value>(&run.payload_json) else {
                let message = "stored ADK run payload is invalid JSON";
                let failed_payload = json!({
                    "status": "FAILED",
                    "errorCode": "ADK_STORAGE_CORRUPT",
                    "message": message,
                    "resumeState": "failed",
                });
                let owner_id = lease_owner_id(&run.id);
                match RunLeaseGuard::acquire(Arc::clone(&self.store), &run.id, &owner_id) {
                    Ok(lease) => match self.store.update_run_state_if_status_and_revision_with_lease(
                        &run.id,
                        "RUNNING",
                        &run.updated_at,
                        "FAILED",
                        &failed_payload.to_string(),
                        lease.owner_id(),
                        lease.token(),
                    ) {
                        Ok(true) => eprintln!(
                            "ADK run {} had corrupt payload and was marked FAILED ({message})",
                            run.id
                        ),
                        Ok(false) => eprintln!(
                            "ADK run {} had corrupt payload but changed before failure marking",
                            run.id
                        ),
                        Err(error) => eprintln!(
                            "ADK run {} had corrupt payload; failed to persist failure state: {error}",
                            run.id
                        ),
                    },
                    Err(AdkChatPortError::Conflict(error)) => eprintln!(
                        "ADK run {} had corrupt payload but its execution lease is held: {error}",
                        run.id
                    ),
                    Err(error) => eprintln!(
                        "ADK run {} had corrupt payload; failed to acquire recovery lease: {error:?}",
                        run.id
                    ),
                }
                continue;
            };
            let recovering = payload
                    .get("resumeState")
                    .and_then(Value::as_str)
                    .is_some_and(|state| {
                        state.eq_ignore_ascii_case("approval_resuming")
                            || state.eq_ignore_ascii_case("tool_executing")
                    })
                    || payload
                        .get("toolCalls")
                        .and_then(Value::as_array)
                        .is_some_and(|calls| {
                            calls.iter().any(|call| {
                                call.get("status")
                                    .and_then(Value::as_str)
                                    .is_some_and(|status| status.eq_ignore_ascii_case("RUNNING"))
                            })
                        });
            if recovering {
                let _ = self.resume_approval(&run.id);
            }
        }
    }

    #[allow(dead_code)]
    pub(crate) fn shutdown(&self) {
        self.cancellation_registry.cancel_all();
        self.continuation_supervisor.shutdown();
    }

    fn persist_cancelled(
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
        if run.status.eq_ignore_ascii_case("CANCELLED")
            || !run.status.eq_ignore_ascii_case("RUNNING")
        {
            return Ok(());
        }
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        let has_terminal = payload
            .get("streamEvents")
            .and_then(Value::as_array)
            .and_then(|events| events.last())
            .and_then(|event| event.get("type"))
            .and_then(Value::as_str)
            .is_some_and(|kind| matches!(kind, "final" | "error"));
        let mut stream_event_id = None;
        let mut stream_event_content = None;
        if !has_terminal && chat.route == AdkChatRoute::Stream {
            let sequence = payload
                .get("streamEvents")
                .and_then(Value::as_array)
                .map_or(1, |events| events.len() as u64 + 1);
            let mut event = json!({
                "type": "error",
                "message": format_adk_error(error),
            });
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
            stream_event_content = Some(format_adk_error(error));
        }
        payload["status"] = Value::String("CANCELLED".to_owned());
        payload["message"] = Value::String(format_adk_error(error));
        let updated = match (stream_event_id.as_ref(), stream_event_content.as_ref()) {
            (Some(event_id), Some(content)) => self
                .store
                .update_run_state_if_status_and_revision_with_events_with_lease(
                    &chat.run_id,
                    "RUNNING",
                    &run.updated_at,
                    "CANCELLED",
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
                .store
                .update_run_state_if_status_and_revision_with_lease(
                    &chat.run_id,
                    "RUNNING",
                    &run.updated_at,
                    "CANCELLED",
                    &payload.to_string(),
                    run_lease.owner_id(),
                    run_lease.token(),
                ),
        }
        .map_err(storage_unavailable)?;
        if !updated {
            let current = self
                .store
                .get_run(&chat.run_id)
                .map_err(storage_unavailable)?
                .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
            if !current.status.eq_ignore_ascii_case("CANCELLED") {
                return Err(unavailable(
                    "assistant chat run or execution lease changed before cancellation",
                ));
            }
        }
        Ok(())
    }
}

fn run_cancelled() -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 499,
        code: "RUN_CANCELLED".to_owned(),
        message: "assistant chat run was cancelled".to_owned(),
    }
}
