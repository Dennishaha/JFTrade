impl ProductionAdkChatRuntime {
    pub(crate) fn new(
        store: Arc<AdkStore>,
        session_store: Arc<AdkSessionStore>,
        settings_path: &Path,
        cancellation_registry: Arc<RunCancellationRegistry>,
        tool_catalog: Arc<crate::product::product_production_ports::ProductionToolCatalog>,
    ) -> Self {
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
        let runtime = Self {
            store,
            session_store,
            secrets_path,
            cancellation_registry,
            tool_catalog,
        };
        runtime.recover_approval_continuations();
        runtime
    }

    fn recover_approval_continuations(&self) {
        let Ok(runs) = self.store.list_runs() else {
            return;
        };
        for run in runs {
            let Ok(payload) = serde_json::from_str::<Value>(&run.payload_json) else {
                continue;
            };
            let recovering = run.status.eq_ignore_ascii_case("RUNNING")
                && payload
                    .get("resumeState")
                    .and_then(Value::as_str)
                    .is_some_and(|state| state.eq_ignore_ascii_case("approval_resuming"));
            if recovering {
                let _ = self.resume_approval(&run.id);
            }
        }
    }

    fn dispatch_inner(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        let prepared = self.prepare_chat(route, input)?;
        match prepared {
            PreparedChat::Existing(output) => Ok(output),
            PreparedChat::New(chat) => self.execute_chat(chat),
        }
    }

    fn prepare_chat(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<PreparedChat, AdkChatPortError> {
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
        let message = text_field(object, "message").ok_or_else(|| AdkChatPortError::Failed {
            status: 400,
            code: "BAD_REQUEST".to_owned(),
            message: "message is required".to_owned(),
        })?;
        let provider = self.resolve_provider(object)?;
        let agent_id = provider.agent_id.clone();
        let model = text_field(object, "model")
            .or_else(|| provider.agent_model.clone())
            .unwrap_or(provider.model.clone());
        if model.is_empty() {
            return Err(unavailable("assistant model is not configured"));
        }
        let fingerprint = fingerprint(&input.body);
        if let Some(existing) = self
            .store
            .list_runs()
            .map_err(storage_unavailable)?
            .into_iter()
            .find(|run| run.client_request_id == input.client_request_id)
        {
            if existing.request_fingerprint != fingerprint {
                return Err(AdkChatPortError::Conflict(
                    "clientRequestId was already used with a different request".to_owned(),
                ));
            }
            if let Some(response) = persisted_response(&existing.payload_json)? {
                return Ok(PreparedChat::Existing(match route {
                    AdkChatRoute::Chat => AdkChatPortOutput::Json(response),
                    AdkChatRoute::Stream => stream_from_payload(&existing.payload_json)?,
                }));
            }
        }
        let session_payload = json!({
            "id": session_id,
            "agentId": agent_id,
            "title": message.chars().take(28).collect::<String>(),
        });
        self.store
            .upsert_session(&session_id, &agent_id, &session_payload.to_string())
            .map_err(storage_unavailable)?;
        // The ADK session projection and the ADK event journal live in
        // separate compatibility databases.  Ensure the referenced session
        // exists before recording the first user event (the event schema has
        // an FK back to this row).
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
            "requestMessage": message.clone(),
            "providerId": provider.id.clone(),
            "model": model.clone(),
        });
        self.store
            .create_run(CreateAdkRunParams {
                id: &run_id,
                session_id: &session_id,
                agent_id: &agent_id,
                status: "RUNNING",
                client_request_id: &input.client_request_id,
                request_fingerprint: &fingerprint,
                payload_json: &initial_payload.to_string(),
            })
            .map_err(|error| {
                if error
                    .to_string()
                    .to_ascii_lowercase()
                    .contains("constraint")
                {
                    AdkChatPortError::Conflict("chat request is already running".to_owned())
                } else {
                    storage_unavailable(error)
                }
            })?;
        self.record_event(&run_id, &session_id, "user", &message)?;
        Ok(PreparedChat::New(ChatExecution {
            route,
            run_id,
            session_id,
            agent_id,
            request: ModelRequest {
                endpoint: provider.endpoint,
                api_key: provider.api_key,
                model,
                instruction: provider.instruction,
                message: message.clone(),
                timeout: provider.timeout,
                tools: self.tool_catalog.openai_tools(),
            },
        }))
    }

    /// Resume a run released from approval.  The durable approval CAS is the
    /// gate; this method only schedules the actual model continuation after
    /// validating the persisted request and current provider configuration.
    pub(crate) fn resume_approval(&self, run_id: &str) -> Result<(), AdkChatPortError> {
        let run = self
            .store
            .get_run(run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        if run.status.eq_ignore_ascii_case("CANCELLED") {
            return Ok(());
        }
        if !run.status.eq_ignore_ascii_case("RUNNING") {
            return Err(unavailable(format!(
                "assistant chat run is already {}",
                run.status
            )));
        }
        let payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
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
            denied_payload["status"] = Value::String("DENIED".to_owned());
            denied_payload["message"] = Value::String("assistant tool call was denied".to_owned());
            denied_payload["completedAt"] = Value::String(now.clone());
            let denied_event_id = format!("{run_id}:denied");
            let event = AdkRunEvent {
                id: &denied_event_id,
                session_id: &run.session_id,
                invocation_id: &run.id,
                author: &run.agent_id,
                content: "assistant tool call was denied",
            };
            self.store
                .update_run_state_if_status_and_revision_with_events(
                    run_id,
                    "RUNNING",
                    &run.updated_at,
                    "DENIED",
                    &denied_payload.to_string(),
                    self.session_store.path(),
                    &[event],
                )
                .map_err(storage_unavailable)?;
            return Ok(());
        }
        let message = object
            .get("requestMessage")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(str::to_owned)
            .or_else(|| {
                self.session_store
                    .list_events(&run.session_id)
                    .ok()?
                    .into_iter()
                    .find(|event| event.invocation_id == run.id && event.author == "user")
                    .map(|event| event.content)
            })
            .ok_or_else(|| unavailable("persisted ADK run has no resumable request"))?;
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
        let chat = ChatExecution {
            route: AdkChatRoute::Chat,
            run_id: run.id,
            session_id: run.session_id,
            agent_id: run.agent_id,
            request: ModelRequest {
                endpoint: provider.endpoint,
                api_key: provider.api_key,
                model,
                instruction: provider.instruction,
                message,
                timeout: provider.timeout,
                tools: self.tool_catalog.openai_tools(),
            },
        };
        let store = Arc::clone(&self.store);
        let session_store = Arc::clone(&self.session_store);
        let secrets_path = self.secrets_path.clone();
        let cancellation_registry = Arc::clone(&self.cancellation_registry);
        let tool_catalog = Arc::clone(&self.tool_catalog);
        std::thread::Builder::new()
            .name("jftrade-adk-approval-resume".to_owned())
            .spawn(move || {
                let runtime = ProductionAdkChatRuntime {
                    store,
                    session_store,
                    secrets_path,
                    cancellation_registry,
                    tool_catalog,
                };
                let cancellation = runtime.cancellation_registry.register(&chat.run_id);
                let _guard = CancellationGuard {
                    registry: Arc::clone(&runtime.cancellation_registry),
                    run_id: chat.run_id.clone(),
                };
                if runtime.run_is_cancelled(&chat.run_id) {
                    return;
                }
                let result = execute_model(chat.request.clone(), Arc::clone(&cancellation));
                if cancellation.load(Ordering::Acquire) {
                    let error = cancellation_error();
                    let _ = runtime.persist_cancelled(&chat, &error);
                    return;
                }
                match result {
                    Ok(response) if !response.tool_calls.is_empty() => {
                        let _ = runtime.persist_tool_calls(&chat, &response);
                    }
                    Ok(response) => {
                        let _ = runtime.persist_success(&chat, response);
                    }
                    Err(error) => {
                        let _ = runtime.persist_failure(&chat, &error);
                    }
                }
            })
            .map_err(|error| unavailable(format!("assistant continuation unavailable: {error}")))?;
        Ok(())
    }

    fn execute_chat(&self, chat: ChatExecution) -> Result<AdkChatPortOutput, AdkChatPortError> {
        if chat.route == AdkChatRoute::Stream {
            let cancellation = self.cancellation_registry.register(&chat.run_id);
            let _guard = CancellationGuard {
                registry: Arc::clone(&self.cancellation_registry),
                run_id: chat.run_id.clone(),
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
            return self.finish_chat(&chat, result);
        }
        // Register synchronous model calls as well as streams.  The cancel
        // mutation runs on another request thread and flips this token before
        // fencing the persisted run state, so the in-flight HTTP call can be
        // interrupted without waiting for the provider timeout.
        let cancellation = self.cancellation_registry.register(&chat.run_id);
        let _guard = CancellationGuard {
            registry: Arc::clone(&self.cancellation_registry),
            run_id: chat.run_id.clone(),
        };
        if self.run_is_cancelled(&chat.run_id) {
            let error = cancellation_error();
            let _ = self.persist_cancelled(&chat, &error);
            return Err(error);
        }
        let result = execute_model(chat.request.clone(), Arc::clone(&cancellation));
        if cancellation.load(Ordering::Acquire) {
            let error = match result {
                Err(error) if is_cancellation_error(&error) => error,
                _ => cancellation_error(),
            };
            let _ = self.persist_cancelled(&chat, &error);
            return Err(error);
        }
        if let Ok(ref response) = result {
            if !response.tool_calls.is_empty() {
                return self.persist_tool_calls(&chat, response);
            }
        }
        self.finish_chat(&chat, result)
    }

    fn persist_tool_calls(
        &self,
        chat: &ChatExecution,
        response: &ModelResponse,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        let run = self
            .store
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        if !run.status.eq_ignore_ascii_case("RUNNING") {
            return Err(unavailable(format!(
                "assistant chat run is already {}",
                run.status
            )));
        }
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        if !payload.is_object() {
            return Err(AdkChatPortError::Failed {
                status: 500,
                code: "ADK_STORAGE_CORRUPT".to_owned(),
                message: "stored ADK run payload must be a JSON object".to_owned(),
            });
        }
        let available_tools = self.tool_catalog.openai_tools();
        let known = response.tool_calls.iter().all(|call| {
            available_tools.iter().any(|schema| {
                schema.get("name").and_then(Value::as_str) == Some(call.name.as_str())
            })
        });
        let status = if known { "PENDING" } else { "FAILED" };
        let mut pending = Vec::new();
        let mut tool_calls = Vec::with_capacity(response.tool_calls.len());
        for (index, call) in response.tool_calls.iter().enumerate() {
            let call_status = if known { "PENDING_APPROVAL" } else { "FAILED" };
            let requires_user = known;
            let approval_id = format!("{}:approval:{}", chat.run_id, index + 1);
            let call_value = json!({
                "id": call.id,
                "name": call.name,
                "arguments": call.arguments,
                "status": call_status,
                "requiresUser": requires_user,
                "approvalId": if known { Value::String(approval_id.clone()) } else { Value::Null },
                "errorCode": if known { Value::Null } else { Value::String("ADK_TOOL_UNAVAILABLE".to_owned()) },
            });
            tool_calls.push(call_value);
            if known {
                let approval = json!({
                    "id": approval_id,
                    "runId": chat.run_id,
                    "agentId": chat.agent_id,
                    "status": "PENDING",
                    "toolName": call.name,
                    "toolCallId": call.id,
                    "arguments": call.arguments,
                    "requiresUser": true,
                });
                self.store
                    .create_approval(
                        &approval_id,
                        &chat.run_id,
                        &chat.agent_id,
                        "PENDING",
                        &approval.to_string(),
                    )
                    .map_err(storage_unavailable)?;
                pending.push(approval);
            }
        }
        {
            let object = payload.as_object_mut().expect("payload object checked");
            object.insert("toolCalls".to_owned(), Value::Array(tool_calls));
            object.insert("pendingApprovals".to_owned(), Value::Array(pending.clone()));
            object.insert("status".to_owned(), Value::String(status.to_owned()));
            object.insert(
                "message".to_owned(),
                Value::String(if known {
                    "assistant tool call requires approval".to_owned()
                } else {
                    "assistant requested an unavailable tool".to_owned()
                }),
            );
        }
        let payload_json = payload.to_string();
        let updated = self
            .store
            .update_run_state_if_status(&chat.run_id, "RUNNING", status, &payload_json)
            .map_err(storage_unavailable)?;
        if !updated {
            return Err(unavailable(
                "assistant chat run state changed before tool call staging",
            ));
        }
        if !known {
            return Err(AdkChatPortError::Failed {
                status: 503,
                code: "ADK_TOOL_UNAVAILABLE".to_owned(),
                message: "assistant requested an unavailable tool".to_owned(),
            });
        }
        let session = self
            .store
            .get_session(&chat.session_id)
            .map_err(storage_unavailable)?
            .map(|session| {
                json!({"id": session.id, "agentId": chat.agent_id, "createdAt": session.created_at, "updatedAt": session.updated_at})
            })
            .unwrap_or_else(|| json!({"id": chat.session_id, "agentId": chat.agent_id}));
        Ok(AdkChatPortOutput::Json(json!({
            "reply": "",
            "session": session,
            "run": payload,
            "pendingApprovals": pending,
            "timeline": [],
        })))
    }

    fn resolve_provider(
        &self,
        request: &serde_json::Map<String, Value>,
    ) -> Result<ResolvedProvider, AdkChatPortError> {
        let requested = request
            .get("providerId")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty());
        let requested_agent_id = text_field(request, "agentId");
        let providers = self.store.list_providers().map_err(storage_unavailable)?;
        let (agent_id, agent_payload) = self.resolve_agent(requested_agent_id)?;
        let agent_provider = agent_payload
            .get("providerId")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty());
        let parsed_providers = providers
            .iter()
            .map(|provider| {
                serde_json::from_str::<Value>(&provider.payload_json)
                    .map(|value| (provider, value))
                    .map_err(|error| AdkChatPortError::Failed {
                        status: 500,
                        code: "ADK_STORAGE_CORRUPT".to_owned(),
                        message: format!("stored ADK provider payload is invalid JSON: {error}"),
                    })
            })
            .collect::<Result<Vec<_>, _>>()?;
        let selected = if let Some(id) = requested.or(agent_provider) {
            parsed_providers
                .iter()
                .find(|(provider, _)| provider.id == id)
                .ok_or_else(|| unavailable("assistant model provider is unavailable"))?
        } else {
            parsed_providers
                .iter()
                .find(|(_, value)| {
                    value
                        .get("default")
                        .and_then(Value::as_bool)
                        .unwrap_or(false)
                })
                .ok_or_else(|| unavailable("no assistant model provider is configured"))?
        };
        let (selected, value) = selected;
        let enabled = value
            .get("enabled")
            .and_then(Value::as_bool)
            .unwrap_or(true);
        if !enabled {
            return Err(unavailable("assistant model provider is disabled"));
        }
        let endpoint = value
            .get("baseUrl")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| unavailable("assistant model provider baseUrl is not configured"))?;
        let endpoint = responses_endpoint(endpoint).map_err(|error| AdkChatPortError::Failed {
            status: 502,
            code: "MODEL_PROVIDER_UNAVAILABLE".to_owned(),
            message: error,
        })?;
        let secrets = read_secrets(&self.secrets_path)?;
        let api_key = secrets
            .get(&selected.id)
            .cloned()
            .or_else(|| {
                value
                    .get("apiKey")
                    .and_then(Value::as_str)
                    .map(str::to_owned)
            })
            .map(|key| key.trim().to_owned())
            .filter(|key| !key.is_empty())
            .ok_or_else(|| unavailable("assistant model provider API key is not configured"))?;
        let instruction = agent_payload
            .get("instruction")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(str::to_owned)
            .or_else(|| Some(DEFAULT_BUILTIN_AGENT_INSTRUCTION.to_owned()));
        let timeout_ms = value
            .get("requestTimeoutMs")
            .and_then(Value::as_u64)
            .unwrap_or(DEFAULT_TIMEOUT_MS)
            .clamp(15_000, 600_000);
        Ok(ResolvedProvider {
            id: selected.id.clone(),
            agent_id,
            endpoint,
            api_key,
            model: value
                .get("model")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .trim()
                .to_owned(),
            agent_model: agent_payload
                .get("model")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(str::to_owned),
            instruction,
            timeout: Duration::from_millis(timeout_ms),
        })
    }

    /// Resolve an explicit agent strictly, or mirror Go DefaultAgent's
    /// enabled-primary -> first-enabled -> builtin-template ordering.
    fn resolve_agent(
        &self,
        requested_agent_id: Option<String>,
    ) -> Result<(String, Value), AdkChatPortError> {
        if let Some(agent_id) = requested_agent_id {
            let entity = self
                .store
                .get_agent(&agent_id)
                .map_err(storage_unavailable)?
                .ok_or_else(|| bad_agent("agent not found"))?;
            let payload = parse_agent_payload(&entity.payload_json)?;
            if !agent_enabled(&payload) {
                return Err(bad_agent("agent is unavailable"));
            }
            return Ok((entity.id, payload));
        }

        let mut agents = self.store.list_agents().map_err(storage_unavailable)?;
        // The Go store orders by updated_at DESC, id ASC and then promotes the
        // primary builtin.  Reapply that ordering because the SQLite adapter
        // exposes the persisted entities without the Go normalization layer.
        agents.sort_by(|left, right| {
            let left_primary = left.id.eq_ignore_ascii_case(DEFAULT_BUILTIN_AGENT_ID);
            let right_primary = right.id.eq_ignore_ascii_case(DEFAULT_BUILTIN_AGENT_ID);
            right_primary
                .cmp(&left_primary)
                .then_with(|| right.updated_at.cmp(&left.updated_at))
                .then_with(|| left.id.cmp(&right.id))
        });
        let mut parsed = Vec::with_capacity(agents.len());
        for agent in agents {
            parsed.push((agent.id, parse_agent_payload(&agent.payload_json)?));
        }
        if let Some((id, payload)) = parsed.iter().find(|(id, payload)| {
            id.eq_ignore_ascii_case(DEFAULT_BUILTIN_AGENT_ID) && agent_enabled(payload)
        }) {
            return Ok((id.clone(), payload.clone()));
        }
        if let Some((id, payload)) = parsed.iter().find(|(_, payload)| agent_enabled(payload)) {
            return Ok((id.clone(), payload.clone()));
        }
        Ok((
            DEFAULT_BUILTIN_AGENT_ID.to_owned(),
            json!({
                "id": DEFAULT_BUILTIN_AGENT_ID,
                "status": "ENABLED",
                "instruction": DEFAULT_BUILTIN_AGENT_INSTRUCTION,
            }),
        ))
    }

    fn record_event(
        &self,
        run_id: &str,
        session_id: &str,
        author: &str,
        content: &str,
    ) -> Result<(), AdkChatPortError> {
        let id = format!("{run_id}:{author}");
        self.record_event_with_id(&id, run_id, session_id, author, content)
    }

    fn record_event_with_id(
        &self,
        id: &str,
        run_id: &str,
        session_id: &str,
        author: &str,
        content: &str,
    ) -> Result<(), AdkChatPortError> {
        let exists = self
            .session_store
            .list_events(session_id)
            .map(|events| events.into_iter().any(|event| event.id == id))
            .map_err(storage_unavailable)?;
        if !exists {
            self.session_store
                .record_event(RecordAdkEventParams {
                    id: &id,
                    app_name: "jftrade",
                    user_id: "local",
                    session_id,
                    invocation_id: run_id,
                    author,
                    content,
                })
                .map_err(storage_unavailable)?;
        }
        Ok(())
    }
}

