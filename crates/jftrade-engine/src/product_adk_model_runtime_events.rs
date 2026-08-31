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
        let tool_executor = Arc::new(ProductionAdkToolExecutor::new(
            Arc::clone(&tool_catalog),
            Arc::clone(&store),
        ));
        let continuation_supervisor = Arc::new(ContinuationSupervisor::default());
        let runtime = Self {
            store,
            session_store,
            secrets_path,
            cancellation_registry,
            tool_executor,
            tool_catalog,
            continuation_supervisor,
        };
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
        self.store
            .create_run_with_event(
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
            )
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
        let tools = self
            .tool_catalog
            .openai_tools()
            .into_iter()
            .filter(|schema| {
                schema
                    .get("name")
                    .and_then(Value::as_str)
                    .is_some_and(|name| self.tool_executor.supports(name))
            })
            .collect();
        let tool_context = durable_context_items(
            self.store.as_ref(),
            self.session_store.as_ref(),
            &session_id,
            Some(&run_id),
        )?;
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
                durable_context: tool_context,
                tool_context: Vec::new(),
                timeout: provider.timeout,
                tools,
            },
        }))
    }

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
        let tools = self
            .tool_catalog
            .openai_tools()
            .into_iter()
            .filter(|schema| {
                schema
                    .get("name")
                    .and_then(Value::as_str)
                    .is_some_and(|name| self.tool_executor.supports(name))
            })
            .collect();
        let chat = ChatExecution {
            route,
            run_id: run.id,
            session_id: run.session_id,
            agent_id: run.agent_id,
            request: ModelRequest {
                endpoint: provider.endpoint,
                api_key: provider.api_key,
                model,
                instruction: provider.instruction,
                message,
                durable_context: Vec::new(),
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
        let continuation_supervisor = Arc::clone(&self.continuation_supervisor);
        let continuation_run_id = chat.run_id.clone();
        let supervisor_for_task = Arc::clone(&continuation_supervisor);
        continuation_supervisor.clone().spawn(&continuation_run_id, move |continuation_cancel| {
                let continuation_supervisor = supervisor_for_task;
                let tool_executor = Arc::new(ProductionAdkToolExecutor::new(
                    Arc::clone(&tool_catalog),
                    Arc::clone(&store),
                ));
                let runtime = ProductionAdkChatRuntime {
                    store,
                    session_store,
                    secrets_path,
                    cancellation_registry,
                    tool_catalog,
                    tool_executor,
                    continuation_supervisor,
                };
                let cancellation = runtime
                    .cancellation_registry
                    .register_token(&chat.run_id, continuation_cancel);
                let _guard = CancellationGuard {
                    registry: Arc::clone(&runtime.cancellation_registry),
                    run_id: chat.run_id.clone(),
                };
                if runtime.run_is_cancelled(&chat.run_id) {
                    return;
                }
                runtime.run_approval_continuation(chat, cancellation);
            })?;
        Ok(())
    }

    fn execute_chat(&self, chat: ChatExecution) -> Result<AdkChatPortOutput, AdkChatPortError> {
        let owner_id = lease_owner_id(&chat.run_id);
        let run_lease = RunLeaseGuard::acquire(
            Arc::clone(&self.store),
            &chat.run_id,
            &owner_id,
        )?;
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
            if run_lease.is_lost() {
                return Err(unavailable("assistant run execution lease was lost"));
            }
            return self.finish_chat(&chat, result, &run_lease);
        }
        let cancellation = self.cancellation_registry.register(&chat.run_id);
        let _guard = CancellationGuard {
            registry: Arc::clone(&self.cancellation_registry),
            run_id: chat.run_id.clone(),
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
            return self.persist_tool_calls(&chat, response, &run_lease);
        }
        self.finish_chat(&chat, result, &run_lease)
    }

    fn persist_tool_calls(
        &self,
        chat: &ChatExecution,
        response: &ModelResponse,
        run_lease: &RunLeaseGuard,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        if run_lease.is_lost() {
            return Err(unavailable("assistant run execution lease was lost"));
        }
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
        let known = response
            .tool_calls
            .iter()
            .all(|call| self.tool_executor.supports(&call.name));
        let status = if known { "PENDING" } else { "FAILED" };
        let mut pending = Vec::new();
        let mut approval_rows: Vec<(String, String)> = Vec::new();
        let prior_calls = payload
            .get("toolCalls")
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();
        let mut tool_calls = prior_calls;
        let prior_round = tool_calls
            .iter()
            .filter_map(|call| call.get("round").and_then(Value::as_u64))
            .max()
            .unwrap_or_default();
        let round = prior_round.saturating_add(1);
        let timestamp = run.updated_at.clone();
        for (index, call) in response.tool_calls.iter().enumerate() {
            let call_status = if known { "PENDING_APPROVAL" } else { "FAILED" };
            let requires_user = known;
            let approval_id = format!("{}:approval:r{}:{}", chat.run_id, round, index + 1);
            let confirmation_call_id = format!("{approval_id}:confirmation");
            let call_value = json!({
                "id": call.id,
                "runId": chat.run_id,
                "functionCallId": call.id,
                "confirmationCallId": if known { Value::String(confirmation_call_id.clone()) } else { Value::Null },
                "name": call.name,
                "toolName": call.name,
                "arguments": call.arguments,
                "input": call.arguments,
                "status": call_status,
                "requiresUser": requires_user,
                "approvalId": if known { Value::String(approval_id.clone()) } else { Value::Null },
                "idempotencyKey": call.id,
                "error": if known { Value::Null } else { Value::String("tool adapter unavailable".to_owned()) },
                "errorCode": if known { Value::Null } else { Value::String("ADK_TOOL_UNAVAILABLE".to_owned()) },
                "round": round,
                "permission": "approval",
                "reason": if known { "assistant requested tool execution" } else { "tool adapter unavailable" },
                "createdAt": timestamp.clone(),
                "updatedAt": timestamp.clone(),
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
                    "functionCallId": call.id,
                    "confirmationCallId": confirmation_call_id,
                    "arguments": call.arguments,
                    "input": call.arguments,
                    "requiresUser": true,
                    "permission": "approval",
                    "reason": "assistant requested tool execution",
                    "createdAt": timestamp.clone(),
                    "updatedAt": timestamp.clone(),
                });
                approval_rows.push((approval_id.clone(), approval.to_string()));
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
        let approval_stages = approval_rows
            .iter()
            .map(|(id, approval_payload)| AdkApprovalStage {
                id,
                run_id: &chat.run_id,
                agent_id: &chat.agent_id,
                payload_json: approval_payload,
            })
            .collect::<Vec<_>>();
        let mut event_rows = Vec::with_capacity(response.tool_calls.len());
        for (index, call) in response.tool_calls.iter().enumerate() {
            let event_id = format!("{}:tool-call:{}:{}", chat.run_id, round, index + 1);
            let content = serde_json::to_string(&json!({
                "id": call.id,
                "name": call.name,
                "arguments": call.arguments,
                "status": if known { "PENDING_APPROVAL" } else { "FAILED" },
            }))
            .map_err(|error| unavailable(format!("encode tool call event: {error}")))?;
            event_rows.push((event_id, content));
        }
        let events = event_rows
            .iter()
            .map(|(id, content)| AdkRunEvent {
                id,
                session_id: &chat.session_id,
                invocation_id: &chat.run_id,
                author: "assistant.tool_call",
                content,
            })
            .collect::<Vec<_>>();
        let updated = self
            .store
            .stage_tool_calls_if_status_and_revision_with_events_with_lease(
                &chat.run_id,
                "RUNNING",
                &run.updated_at,
                status,
                &payload_json,
                &approval_stages,
                self.session_store.as_ref(),
                &events,
                run_lease.owner_id(),
                run_lease.token(),
            )
            .map_err(storage_unavailable)?;
        if !updated {
            return Err(unavailable("assistant chat run state changed before tool call staging"));
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

}
