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
            PreparedChat::New(chat, run_lease) => self.execute_chat(chat, run_lease),
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
        let fingerprint = fingerprint(&input.body);
        if let Some(existing) = self
            .store
            .get_run_by_client_request_id(&input.client_request_id)
            .map_err(storage_unavailable)?
        {
            return self.prepare_existing_run(existing, route, &fingerprint);
        }
        let provider = self.resolve_provider(object)?;
        let agent_id = provider.agent_id.clone();
        let model = text_field(object, "model")
            .or_else(|| provider.agent_model.clone())
            .unwrap_or(provider.model.clone());
        if model.is_empty() {
            return Err(unavailable("assistant model is not configured"));
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
        let tools = self
            .tool_catalog
            .openai_tools()
            .into_iter()
            .filter(|schema| {
                schema
                    .get("name")
                    .and_then(Value::as_str)
                    .is_some_and(|name| replay_safe_tool(name) && self.tool_executor.supports(name))
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
        if existing.status.eq_ignore_ascii_case("RUNNING") {
            match self.resume_approval(&existing.id) {
                Ok(()) | Err(AdkChatPortError::Conflict(_)) => {}
                Err(AdkChatPortError::Unavailable(_)) => {}
                Err(error) => return Err(error),
            }
        }
        existing_run_output(&existing, route).map(PreparedChat::Existing)
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
        let resumed_session_id = run.session_id.clone();
        let resumed_run_id = run.id.clone();
        let tools = self
            .tool_catalog
            .openai_tools()
            .into_iter()
            .filter(|schema| {
                schema
                    .get("name")
                    .and_then(Value::as_str)
                    .is_some_and(|name| replay_safe_tool(name) && self.tool_executor.supports(name))
            })
            .collect();
        let chat = ChatExecution {
            route,
            run_id: resumed_run_id.clone(),
            session_id: resumed_session_id.clone(),
            agent_id: run.agent_id,
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
        let continuation_supervisor = Arc::clone(&self.continuation_supervisor);
        let continuation_run_id = chat.run_id.clone();
        let supervisor_for_task = Arc::clone(&continuation_supervisor);
        continuation_supervisor.clone().spawn(
            &continuation_run_id,
            move |continuation_cancel| {
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
            .all(|call| replay_safe_tool(&call.name) && self.tool_executor.supports(&call.name));
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
            if !known {
                object.insert("errorStatus".to_owned(), Value::from(503));
                object.insert(
                    "errorCode".to_owned(),
                    Value::String("ADK_TOOL_UNAVAILABLE".to_owned()),
                );
                object.insert(
                    "errorMessage".to_owned(),
                    Value::String("assistant requested an unavailable tool".to_owned()),
                );
            }
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
            .map_err(runtime_store_error)?;
        if !updated {
            return Err(AdkChatPortError::Conflict(
                "assistant chat run state changed before tool call staging".to_owned(),
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
}
