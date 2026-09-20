impl ProductionAdkChatRuntime {
    /// Test constructor that swaps the production tool executor for a fixture.
    ///
    /// The approval-policy regressions only need the staging decision plus the
    /// released-call execution path, so they inject a recording executor
    /// instead of the MCP-backed production one.
    #[cfg(test)]
    pub(super) fn with_tool_executor_for_test(
        store: Arc<AdkStore>,
        session_store: Arc<AdkSessionStore>,
        settings_path: &Path,
        cancellation_registry: Arc<RunCancellationRegistry>,
        tool_catalog: Arc<crate::product::product_production_ports::ProductionToolCatalog>,
        tool_executor: Arc<dyn AdkToolExecutor>,
    ) -> Self {
        let secrets_path = settings_path
            .parent()
            .filter(|parent| !parent.as_os_str().is_empty())
            .map_or_else(
                || PathBuf::from("[FUNC]/adk-[FUNC].json"),
                |parent| parent.join("[FUNC]/adk-[FUNC].json"),
            );
        Self {
            store,
            session_store,
            secrets_path,
            cancellation_registry,
            tool_catalog,
            tool_executor,
            continuation_supervisor: Arc::new(ContinuationSupervisor::default()),
            run_gate: Arc::new(RunGate::default()),
            recovery_supervisor: None,
        }
    }

    pub(crate) fn new(
        store: Arc<AdkStore>,
        session_store: Arc<AdkSessionStore>,
        settings_path: &Path,
        cancellation_registry: Arc<RunCancellationRegistry>,
        tool_catalog: Arc<crate::product::product_production_ports::ProductionToolCatalog>,
    ) -> Arc<Self> {
        let secrets_path = std::env::var_os("JFTRADE_ADK_SECRETS")
            .filter(|value| !value.is_empty())
            .map(PathBuf::from)
            .unwrap_or_else(|| {
                settings_path
                    .parent()
                    .filter(|parent| !parent.as_os_str().is_empty())
                    .map_or_else(
                        || PathBuf::from("secrets/adk-secrets.json"),
                        |parent| parent.join("secrets/adk-secrets.json"),
                    )
            });
        let runtime = Arc::new_cyclic(|runtime_weak| {
            let tool_executor = Arc::new(ProductionAdkToolExecutor::new(
                Arc::clone(&tool_catalog),
                Arc::clone(&store),
            ));
            let continuation_supervisor = Arc::new(ContinuationSupervisor::default());
            let recovery_supervisor =
                runtime_recovery::DurableRunRecoverySupervisor::start(runtime_weak.clone());
            Self {
                store: Arc::clone(&store),
                session_store: Arc::clone(&session_store),
                secrets_path: secrets_path.clone(),
                cancellation_registry: Arc::clone(&cancellation_registry),
                tool_executor,
                tool_catalog: Arc::clone(&tool_catalog),
                continuation_supervisor,
                run_gate: Arc::new(RunGate::default()),
                recovery_supervisor: Some(recovery_supervisor),
            }
        });
        // Any queued/running workflow invocation persisted by a previous
        // process has no live executor after restart. Fence it before serving
        // new requests; pending-approval runs are intentionally left for the
        // continuation recovery path below.
        if let Err(error) = runtime.store.recover_orphaned_workflow_trigger_logs() {
            eprintln!("failed to recover orphaned ADK workflow invocations: {error}");
            // A partially recovered durable log is unsafe to serve: a second
            // runtime could replay the same invocation.  Fence all new work
            // until an operator repairs the store and restarts the process.
            runtime
                .continuation_supervisor
                .stopping
                .store(true, std::sync::atomic::Ordering::Release);
        }
        runtime.reconcile_orphaned_pending_runs();
        runtime.recover_approval_continuations();
        runtime
    }

    fn dispatch_inner(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        let prepared = self.prepare_chat(route, input)?;
        match prepared {
            PreparedChat::Existing(output) => Ok(output),
            PreparedChat::New(chat, run_lease, _run_slot) => {
                self.execute_chat(chat, run_lease)
            }
        }
    }

    fn prepare_chat(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<PreparedChat, AdkChatPortError> {
        if self.continuation_supervisor.stopping.load(Ordering::Acquire) {
            return Err(unavailable("assistant runtime is stopping"));
        }
        let request: Value =
            serde_json::from_slice(&input.body).map_err(|error| AdkChatPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: format!("invalid chat payload: {error}"),
            })?;
        let object = request
            .as_object()
            .ok_or_else(|| AdkChatPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid chat payload".to_owned(),
            })?;
        let session_id = text_field(object, "sessionId")
            .unwrap_or_else(|| format!("session-{}", input.client_request_id));
        // Go's chat handler classifies a blank user message as
        // `400 ADK_CHAT_FAILED`; `BAD_REQUEST` is only for a payload that cannot
        // be decoded or carries an invalid `clientRequestId`.
        let message =
            text_field(object, "message").ok_or_else(|| chat_failed("message is required"))?;
        // Go counts runes, not bytes, so a multibyte message at the boundary is
        // accepted.  The check runs before provider/agent resolution, matching
        // `Runtime.prepareChatRequest`.


        if message.chars().count() > MAX_MESSAGE_LENGTH {
            return Err(chat_failed(format!(
                "message exceeds maximum length of {MAX_MESSAGE_LENGTH} characters"
            )));
        }
        // Go's `runChat` validates every per-request override right after
        // `prepareChatRequest` and *before* the agent definition is resolved,
        // so an invalid override is reported even when the agent itself would
        // not resolve.  The message strings are the reference wording.
        let permission_override = validate_permission_mode_override(object)?;
        validate_work_mode_override(object)?;
        validate_reasoning_effort_override(object)?;
        let fingerprint = fingerprint(&input.body);
        if let Some(existing) = self
            .store
            .get_run_by_client_request_id(&input.client_request_id)
            .map_err(storage_unavailable)?
        {
            return self.prepare_existing_run(existing, route, &fingerprint);
        }
        // Go's `runChat` admits the run here: after the idempotent replay check
        // (a reused `clientRequestId` never consumes a slot) and before agent or
        // provider resolution.  The guard is released when the run finishes,
        // including every later error branch.
        let run_slot = self.run_gate.try_acquire()?;
        // `ValidateChatOverrides` returns the trimmed permission mode, and Go
        // writes it onto the agent before the run snapshot is taken; carrying
        // the normalized value keeps `resolve_provider` free of re-validation.
        let mut request_object = object.clone();
        if let Some(permission_override) = permission_override {
            request_object.insert(
                PERMISSION_MODE_OVERRIDE_FIELD.to_owned(),
                Value::String(permission_override),
            );
        } else {
            request_object.remove(PERMISSION_MODE_OVERRIDE_FIELD);
        }
        let provider = self.resolve_provider(&request_object)?;
        let agent_id = provider.agent_id.clone();
        let model = text_field(object, "model")
            .or_else(|| provider.agent_model.clone())
            .unwrap_or(provider.model.clone());
        if model.is_empty() {
            return Err(unavailable("assistant model is not configured"));
        }
        // Go's `Runtime.resolveSession` reuses an explicit session id only when
        // the stored row exists and already belongs to the resolved agent; a
        // provided-but-missing session is "session not found" and a row owned by
        // another agent is "session belongs to a different agent".  Both stay
        // `400 ADK_CHAT_FAILED` at the handler.  The Rust port previously
        // upserted the session unconditionally, silently rebinding an existing
        // session to the new agent.
        let explicit_session = text_field(object, "sessionId");
        if let Some(existing_agent_id) = self
            .store
            .get_session_agent_id(&session_id)
            .map_err(storage_unavailable)?
        {
            if !existing_agent_id.trim().is_empty() && existing_agent_id.trim() != agent_id {
                return Err(chat_failed("session belongs to a different agent"));
            }
        } else if explicit_session.is_some() {
            return Err(chat_failed("session not found"));
        }
        let session_payload = json!({
            "id": session_id,
            "agentId": agent_id,
            "title": message.chars().take(28).collect::<String>(),
        });
        self.store
            .upsert_session(&session_id, &agent_id, &session_payload.to_string())
            .map_err(storage_unavailable)?;
        self.session_store
            .upsert_session(
                "jftrade",
                "local",
                &session_id,
                &session_payload.to_string(),
            )
            .map_err(storage_unavailable)?;
        let run_id = format!("run-{}", input.client_request_id);
        let initial_payload = json!({
            "id": run_id,
            "sessionId": session_id,
            "agentId": agent_id,
            "status": "RUNNING",
            "message": "",
            "reply": "",
            "pendingApprovals": [],
            "streamId": run_id,
            "streamEvents": [],
            "providerEvents": [],
            "resumeState": "provider_executing",
            "requestMessage": message.clone(),
            "providerId": provider.id.clone(),
            // Go's `startRun` captures the resolved provider display name
            // and the effective permission mode on the run, so a later
            // provider rename or agent edit cannot rewrite an existing
            // run's history.
            "providerName": provider.name.clone(),
            "model": model.clone(),
            "permissionMode": provider.permission_mode.clone(),
            "route": match route { AdkChatRoute::Chat => "chat", AdkChatRoute::Stream => "stream" },
            "toolResults": [],
        });
        let initial_payload_json = initial_payload.to_string();
        let initial_event_id = format!("{run_id}:user");
        let initial_event = AdkRunEvent {
            id: &initial_event_id,
            session_id: &session_id,
            invocation_id: &run_id,
            author: "user",
            content: &message,
        };
        let lease_owner = lease_owner_id(&run_id);
        let (existing_or_created, stored_lease) = self
            .store
            .create_run_with_event_idempotent(
                CreateAdkRunParams {
                    id: &run_id,
                    session_id: &session_id,
                    agent_id: &agent_id,
                    status: "RUNNING",
                    client_request_id: &input.client_request_id,
                    request_fingerprint: &fingerprint,
                    payload_json: &initial_payload_json,
                },
                self.session_store.as_ref(),
                &initial_event,
                &lease_owner,
                RUN_LEASE_TTL,
            )
            .map_err(storage_unavailable)?;
        let Some(stored_lease) = stored_lease else {
            return self.prepare_existing_run(existing_or_created, route, &fingerprint);
        };
        let run_lease = RunLeaseGuard::from_lease(Arc::clone(&self.store), stored_lease)?;
        // Go's `ToolDescriptorsForAgent` scopes the model-visible tool list to
        // the resolved agent; an unrestricted agent keeps the whole catalog.
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
        let tool_context = durable_context_items(
            self.store.as_ref(),
            self.session_store.as_ref(),
            &session_id,
            Some(&run_id),
        )?;
        Ok(PreparedChat::New(
            ChatExecution {
                route,
                run_id,
                session_id,
                agent_id,
                resumed: false,
                permission_mode: Self::agent_permission_mode(&provider.agent_payload),
                request: ModelRequest {
                    endpoint: provider.endpoint,
                    api_key: provider.api_key,
                    model,
                    instruction: provider.instruction,
                    message: message.clone(),
                    durable_context: tool_context,
                    tool_context: Vec::new(),
                    timeout: provider.timeout,
                    tools,
                },
            },
            run_lease,
            run_slot,
        ))
    }

    fn prepare_existing_run(
        &self,
        existing: StoredAdkRun,
        route: AdkChatRoute,
        fingerprint: &str,
    ) -> Result<PreparedChat, AdkChatPortError> {
        if existing.request_fingerprint != fingerprint {
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
                content: "approval denied",
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
        let provider = self.resolve_provider(&request)?;
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
        let chat = ChatExecution {
            route,
            run_id: resumed_run_id.clone(),
            session_id: resumed_session_id.clone(),
            agent_id: run.agent_id,
            resumed: true,
            permission_mode: Self::agent_permission_mode(&provider.agent_payload),
            request: ModelRequest {
                endpoint: provider.endpoint,
                api_key: provider.api_key,
                model,
                instruction: provider.instruction,
                message,
                durable_context: durable_context_items(
                    self.store.as_ref(),
                    self.session_store.as_ref(),
                    &resumed_session_id,
                    Some(&resumed_run_id),
                )?,
                tool_context: tool_context_from_payload(object),
                timeout: provider.timeout,
                tools,
            },
        };
        let store = Arc::clone(&self.store);
        let session_store = Arc::clone(&self.session_store);
        let secrets_path = self.secrets_path.clone();
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

    fn execute_chat(
        &self,
        chat: ChatExecution,
        run_lease: RunLeaseGuard,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        if chat.route == AdkChatRoute::Stream {
            let cancellation = self.cancellation_registry.register(&chat.run_id);
            let _guard = CancellationGuard {
                registry: Arc::clone(&self.cancellation_registry),
                run_id: chat.run_id.clone(),
                token: Arc::clone(&cancellation),
            };
            let cancellation_for_stream = Arc::clone(&cancellation);
            let run_id = chat.run_id.clone();
            let result = execute_model_stream(
                chat.request.clone(),
                |_| Ok(()),
                || {
                    cancellation_for_stream.load(Ordering::Acquire)
                        || self.run_is_cancelled(&run_id)
                },
            );
            if run_lease.is_lost() {
                return Err(unavailable("assistant run execution lease was lost"));
            }
            return self.finish_chat(&chat, result, &run_lease);
        }
        let cancellation = self.cancellation_registry.register(&chat.run_id);
        let _guard = CancellationGuard {
            registry: Arc::clone(&self.cancellation_registry),
            run_id: chat.run_id.clone(),
            token: Arc::clone(&cancellation),
        };
        if self.run_is_cancelled(&chat.run_id) {
            let error = cancellation_error();
            let _ = self.persist_cancelled(&chat, &error, &run_lease);
            return Err(error);
        }
        let result = execute_model(chat.request.clone(), Arc::clone(&cancellation));
        if run_lease.is_lost() {
            return Err(unavailable("assistant run execution lease was lost"));
        }
        if cancellation.load(Ordering::Acquire) {
            let error = match result {
                Err(error) if is_cancellation_error(&error) => error,
                _ => cancellation_error(),
            };
            let _ = self.persist_cancelled(&chat, &error, &run_lease);
            return Err(error);
        }
        if let Ok(ref response) = result
            && !response.tool_calls.is_empty()
        {
            return match self.persist_tool_calls(&chat, response, &run_lease)? {
                ToolCallStaging::Pending(output) => Ok(output),
                // No call needed confirmation, so the run keeps its lease and
                // executes the released calls in the same turn.  Go's ADK loop
                // only parks the run when a `RequireConfirmation` wrapper
                // actually asks the operator.
                ToolCallStaging::Released => {
                    let run_id = chat.run_id.clone();
                    self.run_tool_loop(chat, Arc::clone(&cancellation), &run_lease);
                    Ok(AdkChatPortOutput::Json(
                        self.persisted_turn_response(&run_id)?,
                    ))
                }
            };
        }
        self.finish_chat(&chat, result, &run_lease)
    }

    pub(crate) fn attach_ports(&self, ports: Arc<crate::product::product_production_ports::ProductionPortBundle>) {
        self.tool_executor.attach_ports(ports);
    }
}

include!("product_adk_model_runtime_tool_persistence.rs");
include!("product_adk_model_runtime_input_call.rs");
