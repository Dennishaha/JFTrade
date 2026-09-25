// Resolved-run continuation for the production ADK chat runtime.
//
// Textually included by `product_adk_model_runtime_events.rs` so the runtime
// impl stays in one module scope while each fragment respects the workspace
// 800-line production limit.

impl ProductionAdkChatRuntime {
    pub(crate) fn resume_approval(&self, run_id: &str) -> Result<(), AdkChatPortError> {
        let Some(run) = self.store.get_run(run_id).map_err(storage_unavailable)? else {
            // Go's `continueResolvedApprovalRun`/`continueResolvedInput` read
            // the run first and return the store's `nil` error when the row is
            // gone, so `ResolveApprovalAsync` still answers the resolution
            // envelope.  Reporting a fabricated failure here would make the
            // approval and input routes roll back an already-staged resolution
            // and answer `503 ADK_CONTINUATION_UNAVAILABLE` for a run that
            // simply no longer exists.
            return Ok(());
        };
        if run.status.eq_ignore_ascii_case("CANCELLED") {
            return Ok(());
        }
        let payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        let resume_state = payload
            .get("resumeState")
            .and_then(Value::as_str)
            .unwrap_or_default();
        // The input route advances the run to `RUNNING` + `input_resuming`
        // before it asks for the continuation, and the recovery scanner
        // retries `RUNNING` + `input_resume_pending` runs, so both states
        // identify an answered-input continuation.  An approval resume carries
        // `approval_resuming` and finalizes as a confirmation.
        let resumed_from_input = resume_state.eq_ignore_ascii_case("input_resuming")
            || resume_state.eq_ignore_ascii_case("input_resume_pending");
        let is_running = run.status.eq_ignore_ascii_case("RUNNING");
        let is_resumable_input = run.status.eq_ignore_ascii_case("PENDING_INPUT")
            && resume_state.eq_ignore_ascii_case("input_resume_pending");
        if !is_running && !is_resumable_input {
            // Go's `runCanContinueResolvedApproval` and `continueResolvedInput`
            // treat a run that is no longer resumable as a silent no-op: the
            // route, a browser retry and the durable recovery scanner all race
            // for the same continuation, and the owner that arrives late must
            // not report the winner's terminal state as its own failure.  The
            // approval route depends on this to answer the resolution envelope
            // (and to leave the winner's terminal write untouched) instead of
            // rolling the staged resolution back with a fabricated 503.
            return Ok(());
        }
        let payload = if is_resumable_input {
            let mut updated_payload = payload;
            if let Some(obj) = updated_payload.as_object_mut() {
                obj.insert("status".to_owned(), Value::String("RUNNING".to_owned()));
                obj.insert(
                    "resumeState".to_owned(),
                    Value::String("input_resuming".to_owned()),
                );
            }
            let updated = self
                .store
                .update_run_state_if_status_and_revision(
                    run_id,
                    "PENDING_INPUT",
                    &run.updated_at,
                    "RUNNING",
                    &updated_payload.to_string(),
                )
                .map_err(storage_unavailable)?;
            if !updated {
                return Err(unavailable(format!(
                    "assistant chat run {run_id} transition from PENDING_INPUT to RUNNING was superseded by concurrent state modification"
                )));
            }
            updated_payload
        } else {
            payload
        };
        if !runtime_recovery::retry_is_due(&payload) {
            // A caller reconnecting or resolving an approval must not bypass
            // a persisted provider backoff (including a non-retryable
            // configuration probe marker).  The scanner will revisit it at
            // the durable deadline.
            return Ok(());
        }
        let object = payload
            .as_object()
            .ok_or_else(|| unavailable("persisted ADK run payload must be an object"))?;
        let denied = object
            .get("toolCalls")
            .and_then(Value::as_array)
            .is_some_and(|calls| {
                calls.iter().any(|call| {
                    call.get("status")
                        .and_then(Value::as_str)
                        .is_some_and(|status| status.eq_ignore_ascii_case("DENIED"))
                })
            });
        if denied {
            let now = run.updated_at.clone();
            // Go's `finalizeResumedResult` replaces the reply with
            // `model.ApprovalResolutionSummary`, so the transcript records the
            // denial instead of the fixed run message.
            let denial_reply = denied_approval_summary(&payload);
            let mut denied_payload = payload;
            // Go's `markDeniedResumedRun` projects the denial the same way as
            // every other resumed terminal state: `resumeState` becomes
            // `approval_denied` and the run keeps the fixed "approval denied"
            // message with empty errorCode/failureReason.
            denied_payload["status"] = Value::String("DENIED".to_owned());
            denied_payload["resumeState"] = Value::String("approval_denied".to_owned());
            denied_payload["message"] = Value::String("approval denied".to_owned());
            denied_payload["errorCode"] = Value::String(String::new());
            denied_payload["failureReason"] = Value::String(String::new());
            denied_payload["completedAt"] = Value::String(now.clone());
            let denied_event_id = format!("{run_id}:denied");
            let event = AdkRunEvent {
                id: &denied_event_id,
                session_id: &run.session_id,
                invocation_id: &run.id,
                author: &run.agent_id,
                content: &denial_reply,
            };
            let owner_id = lease_owner_id(run_id);
            let run_lease = RunLeaseGuard::acquire(Arc::clone(&self.store), run_id, &owner_id)?;
            self.store
                .update_run_state_if_status_and_revision_with_events_with_lease(
                    run_id,
                    "RUNNING",
                    &run.updated_at,
                    "DENIED",
                    &denied_payload.to_string(),
                    self.session_store.as_ref(),
                    &[event],
                    run_lease.owner_id(),
                    run_lease.token(),
                )
                .map_err(storage_unavailable)?;
            // Go's `auditResumedRun` writes `run.resumed` followed by the
            // lifecycle kind of the status the continuation reached, both
            // carrying `resumeState=approval_denied` so the console can label
            // the denial as an approval outcome rather than a plain failure.
            record_resumed_run_audit(
                self.store.as_ref(),
                run_id,
                &run.agent_id,
                "DENIED",
                "approval_denied",
                "",
            );
            return Ok(());
        }
        let message = object
            .get("requestMessage")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(str::to_owned);
        let message = match message {
            Some(message) => message,
            None => self
                .session_store
                .list_events(&run.session_id)
                .map_err(storage_unavailable)?
                .into_iter()
                .find(|event| event.invocation_id == run.id && event.author == "user")
                .map(|event| event.content)
                .ok_or_else(|| unavailable("persisted ADK run has no resumable request"))?,
        };
        let mut request = serde_json::Map::new();
        if let Some(agent_id) = object.get("agentId").and_then(Value::as_str) {
            request.insert("agentId".to_owned(), Value::String(agent_id.to_owned()));
        }
        if let Some(provider_id) = object.get("providerId").and_then(Value::as_str) {
            request.insert(
                "providerId".to_owned(),
                Value::String(provider_id.to_owned()),
            );
        }
        if let Some(model) = object.get("model").and_then(Value::as_str) {
            request.insert("model".to_owned(), Value::String(model.to_owned()));
        }
        if let Some(effort) = object.get("reasoningEffort").and_then(Value::as_str) {
            request.insert(
                "reasoningEffortOverride".to_owned(),
                Value::String(effort.to_owned()),
            );
        }
        let mut provider = self.resolve_provider(&request)?;
        if let (Some(field), Some(value)) = (
            object.get("reasoningEffortField").and_then(Value::as_str),
            object.get("reasoningEffortValue").and_then(Value::as_str),
        ) && !field.trim().is_empty()
            && !value.trim().is_empty()
        {
            provider.reasoning = Some((field.trim().to_owned(), value.trim().to_owned()));
        }
        let model = object
            .get("model")
            .and_then(Value::as_str)
            .filter(|value| !value.trim().is_empty())
            .map(str::to_owned)
            .or_else(|| provider.agent_model.clone())
            .unwrap_or(provider.model.clone());
        if model.trim().is_empty() {
            return Err(unavailable("assistant model is not configured"));
        }
        let route = match object.get("route").and_then(Value::as_str) {
            Some("stream") => AdkChatRoute::Stream,
            _ => AdkChatRoute::Chat,
        };
        let resumed_session_id = run.session_id.clone();
        let resumed_run_id = run.id.clone();
        let tool_scope = Self::agent_tool_scope(&provider.agent_payload);
        let tools = self
            .tool_catalog
            .openai_tools()
            .into_iter()
            .filter(|schema| {
                schema
                    .get("name")
                    .and_then(Value::as_str)
                    .is_some_and(|name| {
                        name == "interaction.request_user"
                            || (tool_scope.exposes(name)
                                && model_exposed_tool(name)
                                && self.tool_executor.supports(name))
                    })
            })
            .collect();
        self.auto_compact_for_model_context(&resumed_session_id, &message)?;
        let chat = ChatExecution {
            route,
            run_id: resumed_run_id.clone(),
            session_id: resumed_session_id.clone(),
            agent_id: run.agent_id,
            resumed: true,
            resumed_from_input,
            permission_mode: Self::agent_permission_mode(&provider.agent_payload),
            context_deltas: Vec::new(),
            request: ModelRequest {
                endpoint: provider.endpoint,
                api_key: provider.api_key,
                model,
                instruction: provider.instruction,
                message,
                durable_context: {
                    durable_context_items(
                        self.store.as_ref(),
                        self.session_store.as_ref(),
                        &resumed_session_id,
                        Some(&resumed_run_id),
                    )?
                },
                tool_context: tool_context_from_payload(object),
                timeout: provider.timeout,
                tools,
                reasoning: provider.reasoning.clone(),
            },
        };
        let store = Arc::clone(&self.store);
        let session_store = Arc::clone(&self.session_store);
        let secrets_path = self.secrets_path.clone();
        let settings_path = self.settings_path.clone();
        let cancellation_registry = Arc::clone(&self.cancellation_registry);
        let tool_catalog = Arc::clone(&self.tool_catalog);
        let tool_executor = Arc::clone(&self.tool_executor);
        let continuation_supervisor = Arc::clone(&self.continuation_supervisor);
        let run_gate = Arc::clone(&self.run_gate);
        let continuation_run_id = chat.run_id.clone();
        let supervisor_for_task = Arc::clone(&continuation_supervisor);
        continuation_supervisor.clone().spawn(
            &continuation_run_id,
            move |continuation_cancel| {
                let continuation_supervisor = supervisor_for_task;
                let runtime = ProductionAdkChatRuntime {
                    store,
                    session_store,
                    secrets_path,
                    settings_path,
                    cancellation_registry,
                    tool_catalog,
                    tool_executor,
                    continuation_supervisor,
                    run_gate,
                    recovery_supervisor: None,
                };
                let cancellation = runtime
                    .cancellation_registry
                    .register_token(&chat.run_id, continuation_cancel);
                let _guard = CancellationGuard {
                    registry: Arc::clone(&runtime.cancellation_registry),
                    run_id: chat.run_id.clone(),
                    token: Arc::clone(&cancellation),
                };
                if runtime.run_is_cancelled(&chat.run_id) {
                    return;
                }
                runtime.run_approval_continuation(chat, cancellation);
            },
        )?;
        Ok(())
    }

}
