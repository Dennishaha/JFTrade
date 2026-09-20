impl ProductionAdkChatRuntime {
    /// Marks durable `PENDING` runs that lost their resumable approval context
    /// as `FAILED/RUN_ORPHANED` on startup.
    ///
    /// The reference runtime only keeps a pending run resumable while its
    /// approval rows still carry the ADK confirmation identifiers.  A pending
    /// run without those identifiers (for example after a partial restore)
    /// can never be woken again, so the startup reconcile marks it orphaned
    /// instead of leaving it pending forever.
    fn reconcile_orphaned_pending_runs(&self) {
        let Ok(runs) = self.store.list_runs() else {
            return;
        };
        for run in runs {
            if !run.status.eq_ignore_ascii_case("PENDING") {
                continue;
            }
            let Ok(mut payload) = serde_json::from_str::<Value>(&run.payload_json) else {
                continue;
            };
            let Some(pending) = payload
                .get("pendingApprovals")
                .and_then(Value::as_array)
                .filter(|approvals| {
                    approvals.iter().any(|approval| {
                        approval
                            .get("status")
                            .and_then(Value::as_str)
                            .is_some_and(|status| status.eq_ignore_ascii_case("PENDING"))
                    })
                })
            else {
                continue;
            };
            let resumable = pending.iter().any(|approval| {
                approval
                    .get("functionCallId")
                    .and_then(Value::as_str)
                    .is_some_and(|value| !value.trim().is_empty())
                    && approval
                        .get("confirmationCallId")
                        .and_then(Value::as_str)
                        .is_some_and(|value| !value.trim().is_empty())
            });
            if resumable {
                continue;
            }
            let message = "pending approval run lost its resumable approval context";
            payload["status"] = Value::String("FAILED".to_owned());
            payload["errorCode"] = Value::String("RUN_ORPHANED".to_owned());
            payload["errorMessage"] = Value::String(message.to_owned());
            payload["message"] = Value::String(message.to_owned());
            payload["resumeState"] = Value::String("approval_context_missing".to_owned());
            match self.store.update_run_state_if_status_and_revision(
                &run.id,
                "PENDING",
                &run.updated_at,
                "FAILED",
                &payload.to_string(),
            ) {
                Ok(true) => eprintln!(
                    "ADK run {} had no resumable approval context and was marked FAILED ({message})",
                    run.id
                ),
                Ok(false) => eprintln!(
                    "ADK run {} had no resumable approval context but changed before failure marking",
                    run.id
                ),
                Err(error) => eprintln!(
                    "ADK run {} had no resumable approval context; failed to persist failure state: {error}",
                    run.id
                ),
            }
        }
    }

    fn recover_approval_continuations(&self) {
        let Ok(runs) = self.store.list_runs() else {
            return;
        };
        for run in runs {
            let is_running = run.status.eq_ignore_ascii_case("RUNNING");
            let is_pending_input = run.status.eq_ignore_ascii_case("PENDING_INPUT");
            if !is_running && !is_pending_input {
                continue;
            }
            let Ok(payload) = serde_json::from_str::<Value>(&run.payload_json) else {
                if !is_running {
                    continue;
                }
                let message = "stored ADK run payload is invalid JSON";
                let failed_payload = json!({
                    "status": "FAILED",
                    "errorCode": "ADK_STORAGE_CORRUPT",
                    "message": message,
                    "resumeState": "failed",
                });
                let owner_id = lease_owner_id(&run.id);
                match RunLeaseGuard::acquire(Arc::clone(&self.store), &run.id, &owner_id) {
                    Ok(lease) => match self
                        .store
                        .update_run_state_if_status_and_revision_with_lease(
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
                        || state.eq_ignore_ascii_case("input_resuming")
                        || state.eq_ignore_ascii_case("input_resume_pending")
                        || state.eq_ignore_ascii_case("tool_executing")
                        || state.eq_ignore_ascii_case("tool_result_persisted")
                        || state.eq_ignore_ascii_case("provider_executing")
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
            if recovering && runtime_recovery::retry_is_due(&payload) {
                let _ = self.resume_approval(&run.id);
            }
        }
    }

    #[allow(dead_code)]
    pub(crate) fn shutdown(&self) {
        // Stop the scanner before cancelling continuations.  Otherwise a
        // final poll can enqueue a fresh worker while the runtime is already
        // tearing down its leases.
        if let Some(supervisor) = self.recovery_supervisor.as_ref() {
            supervisor.shutdown();
        }
        self.cancellation_registry.cancel_all();
        self.continuation_supervisor.shutdown();
        self.tool_executor.detach_ports();
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
        // Go's `markFailedChatRun` projects a cancellation through the same
        // table as every other terminal failure: the raw error text lands on
        // `Message` and `FailureReason`, `ErrorCode` is the classified
        // `RUN_CANCELLED`, and both `CancelledAt` and `CompletedAt` are stamped
        // with `Degraded = true`.
        let message = format_adk_error(error);
        payload["status"] = Value::String("CANCELLED".to_owned());
        payload["message"] = Value::String(message.clone());
        payload["failureReason"] = Value::String(message.clone());
        payload["degraded"] = Value::Bool(true);
        payload["completedAt"] = Value::String(run.updated_at.clone());
        payload["cancelledAt"] = Value::String(run.updated_at.clone());
        payload["errorStatus"] = Value::from(499);
        payload["errorCode"] = Value::String("RUN_CANCELLED".to_owned());
        payload["errorMessage"] = Value::String(message);
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
        // Go's `markFailedChatRun` + `PersistRunTerminalState` audit
        // `run.cancelled` with the run/agent/status fields, so a cancelled run
        // is visible in the audit view next to the run itself.
        self.record_run_audit(&RunAuditEvent {
            id: format!("{}:audit:run.cancelled", chat.run_id),
            subject_id: chat.run_id.clone(),
            kind: "run.cancelled",
            detail: terminal_audit_message("CANCELLED"),
            metadata: terminal_audit_fields(
                &chat.run_id,
                &chat.agent_id,
                "CANCELLED",
                "RUN_CANCELLED",
                &format_adk_error(error),
            ),
        });
        Ok(())
    }
}

/// Request field carrying the per-request permission-mode override.
///
/// Go's `ChatRequest.PermissionModeOverride` is validated by
/// `ValidateChatOverrides` before the agent is resolved, and the returned
/// (normalized) value overrides the agent's own `permissionMode` when the run
/// snapshot is taken.
pub(super) const PERMISSION_MODE_OVERRIDE_FIELD: &str = "permissionModeOverride";

/// Go's `model.ValidPermissionMode`: `approval`, `less_approval`, `all`.
///
/// `ValidateChatOverrides` trims the value, treats a blank override as absent,
/// and otherwise rejects the request with `invalid permission mode %q` before
/// any agent or provider lookup runs.
fn validate_permission_mode_override(
    request: &serde_json::Map<String, Value>,
) -> Result<Option<String>, AdkChatPortError> {
    let Some(raw) = text_field(request, "permissionModeOverride") else {
        return Ok(None);
    };
    match raw.trim().to_ascii_lowercase().as_str() {
        "approval" | "less_approval" | "all" => Ok(Some(raw.trim().to_ascii_lowercase())),
        _ => Err(chat_failed(format!(
            "invalid permission mode {raw:?}"
        ))),
    }
}

/// Go's `model.ValidWorkMode`: blank, `chat` or `loop`.
fn validate_work_mode_override(
    request: &serde_json::Map<String, Value>,
) -> Result<(), AdkChatPortError> {
    let Some(raw) = text_field(request, "workModeOverride") else {
        return Ok(());
    };
    match raw.trim().to_ascii_lowercase().as_str() {
        "" | "chat" | "loop" => Ok(()),
        _ => Err(chat_failed(format!("invalid work mode {raw:?}"))),
    }
}

/// Go's `model.ValidateOptionalReasoningEffort`: blank, `low`, `medium`,
/// `high`, `xhigh` or `max`, compared case-insensitively after trimming.
fn validate_reasoning_effort_override(
    request: &serde_json::Map<String, Value>,
) -> Result<(), AdkChatPortError> {
    let Some(raw) = text_field(request, "reasoningEffortOverride") else {
        return Ok(());
    };
    match raw.trim().to_ascii_lowercase().as_str() {
        "" | "low" | "medium" | "high" | "xhigh" | "max" => Ok(()),
        _ => Err(chat_failed(format!("invalid reasoning effort {raw:?}"))),
    }
}

fn run_cancelled() -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 499,
        code: "RUN_CANCELLED".to_owned(),
        message: "assistant chat run was cancelled".to_owned(),
    }
}

impl ProductionAdkChatRuntime {
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
                .ok_or_else(|| unavailable("agent provider is unavailable"))?
        } else {
            parsed_providers
                .iter()
                .find(|(_, value)| {
                    value
                        .get("default")
                        .and_then(Value::as_bool)
                        .unwrap_or(false)
                })
                .ok_or_else(|| unavailable("default agent provider is not configured"))?
        };
        let (selected, value) = selected;
        let enabled = value
            .get("enabled")
            .and_then(Value::as_bool)
            .unwrap_or(true);
        if !enabled {
            return Err(unavailable("agent provider is unavailable"));
        }
        let endpoint = value
            .get("baseUrl")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| unavailable("agent provider is unavailable"))?;
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
            .ok_or_else(|| unavailable("agent provider API keys is not configured"))?;
        let mut instruction = agent_payload
            .get("instruction")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(str::to_owned)
            .or_else(|| Some(DEFAULT_BUILTIN_AGENT_INSTRUCTION.to_owned()));
        // Go's `Runtime.prepareAgent` appends the durable memory prompt only
        // when the agent opted into memory (`memoryEnabled`), reading the
        // workspace rows plus the agent's own rows.  The injected text is the
        // exact `JFTrade memory:` block the reference implementation builds.
        if let Some(instruction_value) = instruction.as_mut()
            && agent_payload
                .get("memoryEnabled")
                .and_then(Value::as_bool)
                .unwrap_or(false)
        {
            let memory_prompt = self.agent_memory_prompt(&agent_id)?;
            if !memory_prompt.is_empty() {
                *instruction_value = format!(
                    "{}\n\nJFTrade memory:\n{memory_prompt}",
                    instruction_value.trim()
                );
            }
        }
        let timeout_ms = value
            .get("requestTimeoutMs")
            .and_then(Value::as_u64)
            .unwrap_or(DEFAULT_TIMEOUT_MS)
            .clamp(15_000, 600_000);
        // Go's `validateChatOverrides` ran before the agent was resolved and
        // its permission result wins over the agent's own `permissionMode`
        // when the run snapshot is taken.  The override string is carried on
        // the request by `prepare_chat`, which performs that validation.
        let permission_mode = text_field(request, PERMISSION_MODE_OVERRIDE_FIELD)
            .unwrap_or_else(|| Self::agent_permission_mode(&agent_payload));
        Ok(ResolvedProvider {
            id: selected.id.clone(),
            name: value
                .get("displayName")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .trim()
                .to_owned(),
            agent_id,
            agent_payload: agent_payload.clone(),
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
            permission_mode,
            instruction,
            timeout: Duration::from_millis(timeout_ms),
        })
    }

    /// Tool visibility scope for one resolved agent payload, following Go's
    /// `ToolDescriptorsForAgent` normalization: an explicit `toolAccessMode`
    /// wins, otherwise a non-empty `tools` list is an allowlist and an empty
    /// list exposes every registered tool.
    /// The agent's resolved permission mode, normalized like the reference
    /// `NormalizePermissionMode`: an unknown or blank value falls back to
    /// `approval`, so a malformed payload is gated rather than silently
    /// unrestricted.
    fn agent_permission_mode(payload: &Value) -> String {
        let raw = payload
            .get("permissionMode")
            .and_then(Value::as_str)
            .unwrap_or_default();
        jftrade_assistant::normalize_permission_mode(raw).to_owned()
    }

    fn agent_tool_scope(payload: &Value) -> AgentToolScope {
        let tools = payload
            .get("tools")
            .and_then(Value::as_array)
            .map(|values| {
                values
                    .iter()
                    .filter_map(Value::as_str)
                    .map(str::trim)
                    .filter(|value| !value.is_empty())
                    .map(str::to_owned)
                    .collect::<std::collections::BTreeSet<_>>()
            })
            .unwrap_or_default();
        let mode = payload
            .get("toolAccessMode")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(str::to_ascii_lowercase);
        match mode.as_deref() {
            Some("all") => AgentToolScope::All,
            Some("none") => AgentToolScope::None,
            Some("selected") => AgentToolScope::Selected(tools),
            _ if tools.is_empty() => AgentToolScope::All,
            _ => AgentToolScope::Selected(tools),
        }
    }

    /// Go's `Runtime.agentMemoryPrompt`: workspace rows plus the agent's own
    /// rows, ordered by the store (`updated_at DESC, id ASC`), rendered as
    /// `- [scope] key: value` lines and bounded to 4000 runes.
    fn agent_memory_prompt(&self, agent_id: &str) -> Result<String, AdkChatPortError> {
        let rows = self.store.list_memories().map_err(storage_unavailable)?;
        let mut entries = rows
            .into_iter()
            .filter(|row| {
                let scope = row.scope.trim();
                (scope.eq_ignore_ascii_case("workspace") || row.agent_id.trim() == agent_id)
                    && !(scope.eq_ignore_ascii_case("agent") && row.agent_id.trim() != agent_id)
            })
            .collect::<Vec<_>>();
        entries.sort_by(|left, right| {
            right
                .updated_at
                .cmp(&left.updated_at)
                .then_with(|| left.id.cmp(&right.id))
        });
        let mut lines = Vec::new();
        let mut remaining = 4000_usize;
        for entry in entries {
            let value: Value = serde_json::from_str(&entry.payload_json).map_err(|error| {
                AdkChatPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: format!("stored ADK memory payload is invalid JSON: {error}"),
                }
            })?;
            let text = value
                .get("value")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .trim();
            let line = format!("- [{}] {}: {}", entry.scope.trim(), entry.memory_key, text);
            let mut line = line.chars().take(remaining).collect::<String>();
            remaining = remaining.saturating_sub(line.chars().count());
            lines.push(std::mem::take(&mut line));
            if remaining == 0 {
                break;
            }
        }
        Ok(lines.join("\n"))
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
                .ok_or_else(|| chat_failed("agent not found"))?;
            let payload = parse_agent_payload(&entity.payload_json)?;
            if !agent_enabled(&payload) {
                // Go distinguishes a soft-deleted agent ("agent is deleted")
                // from a disabled one ("agent is disabled"); both stay under
                // the chat handler's `400 ADK_CHAT_FAILED` classification.
                return Err(chat_failed(agent_unavailable_reason(&payload)));
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

#[cfg(test)]
#[path = "product_adk_model_runtime_lifecycle_tests.rs"]
mod lifecycle_tests;

include!("product_adk_model_runtime_auto_compaction.rs");
