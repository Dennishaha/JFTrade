impl ProductionAdkChatRuntime {
    /// Test constructor that swaps the production tool executor for a fixture.
    ///
    /// The approval-policy regressions only need the staging decision plus the
    /// released-call execution path, so they inject a recording executor
    /// instead of the MCP-backed production one.
    #[cfg(test)]
    pub(crate) fn with_tool_executor_for_test(
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
            settings_path: settings_path.to_path_buf(),
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
                settings_path: settings_path.to_path_buf(),
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

    /// Go `Runtime.runtimeLimits().RunTimeout`, re-read from the settings
    /// document on every use: an operator edit applies to the next run instead
    /// of requiring a restart.
    fn run_timeout_ms(&self) -> i64 {
        crate::product::product_adk_run_timeout::assistant_run_timeout_ms(&self.settings_path)
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
        let existing_agent_id = self
            .store
            .get_session_agent_id(&session_id)
            .map_err(storage_unavailable)?;
        if let Some(existing_agent_id) = existing_agent_id.as_ref() {
            if !existing_agent_id.trim().is_empty() && existing_agent_id.trim() != agent_id {
                return Err(chat_failed("session belongs to a different agent"));
            }
        } else if explicit_session.is_some() {
            return Err(chat_failed("session not found"));
        }
        // Go's `resolveSession` only *creates* the session here: the title is
        // the first 28 runes of the message that opened it, and reusing an
        // explicit session id returns the stored row untouched.  Rust upserted
        // the title on every chat, so the second message of a session renamed
        // that session to itself.
        let title: String = message.chars().take(SESSION_TITLE_LIMIT).collect();
        if existing_agent_id.is_none() {
            let session_payload = json!({
                "id": session_id,
                "agentId": agent_id,
                "title": title.clone(),
            });
            self.store
                .upsert_session(&session_id, &agent_id, &session_payload.to_string())
                .map_err(storage_unavailable)?;
        }
        // The Google-ADK session row is created on demand and keeps its state
        // once it exists, the way the ADK session service behaves; rewriting it
        // would reset the transcript metadata on every turn.
        if self
            .session_store
            .get_session_by_id(&session_id)
            .map_err(storage_unavailable)?
            .is_none()
        {
            let session_state = json!({
                "id": session_id,
                "agentId": agent_id,
                "title": title,
            });
            self.session_store
                .upsert_session("jftrade", "local", &session_id, &session_state.to_string())
                .map_err(storage_unavailable)?;
        }
        // Go `RunChat` publishes auto-compaction deltas before the run exists.
        let context_deltas = self.collect_auto_compaction_deltas(&session_id, &message, false)?;
        let run_id = format!("run-{}", input.client_request_id);
        // Go's `startRun` freezes the resolved provider/model snapshot, run
        // budget, work mode and user message before the first model call, and
        // `ProjectedChatResponse` serves them from the run afterwards.
        let work_mode = text_field(object, "workModeOverride")
            .map(|value| normalize_work_mode(&value))
            .unwrap_or_else(|| {
                normalize_work_mode(
                    provider
                        .agent_payload
                        .get("workMode")
                        .and_then(Value::as_str)
                        .unwrap_or_default(),
                )
            });
        let started_at = runtime_projection::now_timestamp();
        let initial_payload = json!({
            "id": run_id,
            "sessionId": session_id,
            "agentId": agent_id,
            "status": "RUNNING",
            // Go's `startRun` writes the literal "running" before the first
            // provider call and the user text onto `run.UserMessage`.
            "message": "running",
            "userMessage": message.clone(),
            "reply": "",
            "pendingApprovals": [],
            "toolCalls": [],
            "toolSummaries": [],
            "workMode": work_mode,
            "maxDurationMs": self.run_timeout_ms(),
            "startedAt": started_at.clone(),
            "usage": {"modelCalls": 0, "toolCallsTotal": 0},
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
        // Go `AutoCompactForModelContext` runs before the provider payload is
        // assembled, so the model sees the compacted projection.
        self.auto_compact_for_model_context(&session_id, &message)?;
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
                resumed_from_input: false,
                permission_mode: Self::agent_permission_mode(&provider.agent_payload),
                context_deltas,
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
            let staging = self.persist_tool_calls(&chat, response, &run_lease);
            if let Err(error) = &staging
                && matches!(
                    error,
                    AdkChatPortError::Failed { code, .. }
                        if code == "ADK_INPUT_REQUEST_CONFLICT"
                )
            {
                // Go's `CompleteChatRun` receives the conflict from
                // `ExecuteGoogleADK` and answers the terminal projection
                // (`200` with a FAILED run and the error text as the reply)
                // instead of a transport error, so the console can show why
                // the turn stopped.  The terminal row already carries that
                // projection, so it is served from the stored payload rather
                // than replayed as an error.
                let error = error.clone();
                self.persist_failure(&chat, &error, &run_lease)?;
                let run = self
                    .store
                    .get_run(&chat.run_id)
                    .map_err(storage_unavailable)?
                    .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
                let stored: Value =
                    serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
                if let Some(response) = stored.get("response").cloned() {
                    return Ok(AdkChatPortOutput::Json(response));
                }
                return Err(error);
            }
            return match staging? {
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

include!("product_adk_model_runtime_resume.rs");
include!("product_adk_model_runtime_tool_persistence.rs");
include!("product_adk_model_runtime_input_call.rs");
include!("product_adk_model_runtime_expiry.rs");

/// Go `assistantmodel.SessionTitleLimit` (`Runtime.resolveSession`): the title
/// of a newly created session is the first 28 runes of its opening message.
const SESSION_TITLE_LIMIT: usize = 28;

include!("product_adk_model_runtime_denial.rs");

/// Go's `model.NormalizeWorkMode`: anything that is not `loop` is `chat`.
fn normalize_work_mode(value: &str) -> &'static str {
    match value.trim().to_ascii_lowercase().as_str() {
        "loop" => "loop",
        _ => "chat",
    }
}
