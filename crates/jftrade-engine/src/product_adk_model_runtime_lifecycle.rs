impl ProductionAdkChatRuntime {
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
        payload["status"] = Value::String("CANCELLED".to_owned());
        payload["message"] = Value::String(format_adk_error(error));
        payload["errorStatus"] = Value::from(499);
        payload["errorCode"] = Value::String("RUN_CANCELLED".to_owned());
        payload["errorMessage"] = Value::String(format_adk_error(error));
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

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_store_sqlite::RecordAdkEventParams;
    use jftrade_store_sqlite::initialize_current;
    use rusqlite::Connection;
    use std::fs::File;
    use std::sync::Barrier;
    use std::thread;
    use tempfile::tempdir;

    fn initialized_stores() -> (tempfile::TempDir, Arc<AdkStore>, Arc<AdkSessionStore>) {
        let directory = tempdir().expect("temporary directory");
        let adk_path = directory.path().join("adk.db");
        let session_path = directory.path().join("adk-session.db");
        File::create(&adk_path).expect("create ADK database");
        File::create(&session_path).expect("create ADK session database");
        initialize_current(
            &Connection::open(&adk_path).expect("initialize ADK database"),
            "adk",
        )
        .expect("initialize ADK schema");
        initialize_current(
            &Connection::open(&session_path).expect("initialize ADK session database"),
            "adk-session",
        )
        .expect("initialize ADK session schema");
        (
            directory,
            Arc::new(AdkStore::open(&adk_path).expect("open ADK store")),
            Arc::new(AdkSessionStore::open(&session_path).expect("open session store")),
        )
    }

    #[test]
    fn concurrent_first_delivery_creates_one_durable_run_and_event() {
        let (_directory, store, session_store) = initialized_stores();
        session_store
            .upsert_session("jftrade", "local", "session-race", "{}")
            .expect("seed session");
        let barrier = Arc::new(Barrier::new(2));
        let workers = ["run-race-a", "run-race-b"].map(|run_id| {
            let store = Arc::clone(&store);
            let session_store = Arc::clone(&session_store);
            let barrier = Arc::clone(&barrier);
            thread::spawn(move || {
                barrier.wait();
                let event_id = format!("{run_id}:user");
                store
                    .create_run_with_event_idempotent(
                        CreateAdkRunParams {
                            id: run_id,
                            session_id: "session-race",
                            agent_id: "agent-race",
                            status: "RUNNING",
                            client_request_id: "request-race",
                            request_fingerprint: "fingerprint-race",
                            payload_json: "{\"status\":\"RUNNING\"}",
                        },
                        session_store.as_ref(),
                        &AdkRunEvent {
                            id: &event_id,
                            session_id: "session-race",
                            invocation_id: run_id,
                            author: "user",
                            content: "hello",
                        },
                        run_id,
                        Duration::from_secs(1),
                    )
                    .expect("create or load run")
            })
        });
        let outcomes = workers.map(|worker| worker.join().expect("join first delivery"));
        assert_eq!(
            outcomes.iter().filter(|(_, lease)| lease.is_some()).count(),
            1
        );
        assert_eq!(outcomes[0].0.id, outcomes[1].0.id);
        assert_eq!(
            session_store
                .list_events("session-race")
                .expect("list initial events")
                .len(),
            1
        );
    }

    #[test]
    fn tool_claim_heartbeat_is_live_then_becomes_fenced_takeover() {
        let (_directory, store, _session_store) = initialized_stores();
        let run = store
            .create_run(CreateAdkRunParams {
                id: "run-claim",
                session_id: "session-claim",
                agent_id: "agent-claim",
                status: "RUNNING",
                client_request_id: "request-claim",
                request_fingerprint: "fingerprint-claim",
                payload_json: "{\"status\":\"RUNNING\"}",
            })
            .expect("create claim run");
        let first_lease = store
            .claim_run_lease("run-claim", "owner-first", Duration::from_secs(1))
            .expect("claim first run lease");
        let first_claim = match store
            .claim_tool_invocation_if_status_and_revision(
                "run-claim",
                "call-claim",
                "tools.search",
                "{}",
                "RUNNING",
                &run.updated_at,
                "owner-first",
                first_lease.fencing_token,
                Duration::from_millis(100),
            )
            .expect("claim first tool invocation")
        {
            AdkToolInvocationClaim::Execute(invocation) => invocation,
            other => panic!("unexpected first claim: {other:?}"),
        };
        let first_claim = store
            .heartbeat_tool_invocation(&first_claim, Duration::from_millis(250))
            .expect("heartbeat tool claim");
        assert!(
            store
                .release_run_lease(&first_lease)
                .expect("release first lease")
        );
        let second_lease = store
            .claim_run_lease("run-claim", "owner-second", Duration::from_secs(1))
            .expect("claim second run lease");
        assert!(matches!(
            store
                .claim_tool_invocation_if_status_and_revision(
                    "run-claim",
                    "call-claim",
                    "tools.search",
                    "{}",
                    "RUNNING",
                    &run.updated_at,
                    "owner-second",
                    second_lease.fencing_token,
                    Duration::from_millis(100),
                )
                .expect("observe live claim"),
            AdkToolInvocationClaim::Live(_)
        ));
        let remaining = first_claim
            .lease_expires_at_unix_ms
            .saturating_sub(unix_now_ms())
            .max(0) as u64;
        thread::sleep(Duration::from_millis(remaining.saturating_add(25)));
        let takeover = store
            .claim_tool_invocation_if_status_and_revision(
                "run-claim",
                "call-claim",
                "tools.search",
                "{}",
                "RUNNING",
                &run.updated_at,
                "owner-second",
                second_lease.fencing_token,
                Duration::from_millis(100),
            )
            .expect("take over expired claim");
        let AdkToolInvocationClaim::Execute(takeover) = takeover else {
            panic!("expired claim was not executable");
        };
        assert!(takeover.fencing_token > first_claim.fencing_token);
        assert_eq!(takeover.run_lease_token, second_lease.fencing_token);
    }

    #[test]
    fn durable_error_classification_keeps_invariants_fatal() {
        assert_eq!(
            classify_durable_store_error(&AdkStoreError::LeaseLost("lost".to_owned())),
            DurableErrorClass::LeaseHeldOrLost
        );
        assert_eq!(
            classify_durable_store_error(&AdkStoreError::Invariant("mismatch".to_owned())),
            DurableErrorClass::InvariantViolation
        );
        assert!(matches!(
            runtime_store_error(AdkStoreError::Invariant("mismatch".to_owned())),
            AdkChatPortError::Failed { ref code, .. } if code == "ADK_STORAGE_CORRUPT"
        ));
    }

    #[test]
    fn cancellation_registry_fans_out_and_unregisters_exact_token() {
        let registry = RunCancellationRegistry::default();
        let first = registry.register("run-fanout");
        let second = registry.register("run-fanout");

        registry.unregister("run-fanout", &first);
        assert!(registry.cancel("run-fanout"));
        assert!(!first.load(Ordering::Acquire));
        assert!(second.load(Ordering::Acquire));

        registry.unregister("run-fanout", &second);
        assert!(!registry.cancel("run-fanout"));
    }

    #[test]
    fn compacted_context_survives_restart_and_precedes_current_user_message() {
        let directory = tempdir().expect("temporary directory");
        let adk_path = directory.path().join("adk.db");
        let session_path = directory.path().join("adk-session.db");
        File::create(&adk_path).expect("create ADK database");
        File::create(&session_path).expect("create ADK session database");
        initialize_current(
            &Connection::open(&adk_path).expect("initialize ADK database"),
            "adk",
        )
        .expect("initialize ADK schema");
        initialize_current(
            &Connection::open(&session_path).expect("initialize ADK session database"),
            "adk-session",
        )
        .expect("initialize ADK session schema");

        {
            let store = AdkStore::open(&adk_path).expect("open ADK store");
            let session_store = AdkSessionStore::open(&session_path).expect("open session store");
            session_store
                .upsert_session("jftrade", "local", "session-1", "{}")
                .expect("seed session");
            for (id, invocation_id, author, content) in [
                ("event-01", "run-old-1", "user", "first question"),
                ("event-02", "run-old-1", "assistant", "first answer"),
                ("event-03", "run-old-2", "user", "latest durable question"),
                ("event-04", "run-current", "user", "current request"),
            ] {
                session_store
                    .record_event(RecordAdkEventParams {
                        id,
                        app_name: "jftrade",
                        user_id: "local",
                        session_id: "session-1",
                        invocation_id,
                        author,
                        content,
                    })
                    .expect("seed session event");
            }
            store
                .upsert_session_context(
                    "session-1",
                    r#"{"contextRevisionId":"revision-1","compactedEventCount":2,"summaryPreview":"compacted summary"}"#,
                )
                .expect("persist compacted context");
            store
                .save_handoff_segment(
                    "session-1",
                    "handoff-1",
                    1,
                    r#"{"endEventIndex":2,"summary":"handoff summary"}"#,
                )
                .expect("persist handoff segment");
        }

        // Reopen both stores to prove the model payload is rebuilt from the
        // durable compaction rows rather than process-local state.
        let store = AdkStore::open(&adk_path).expect("reopen ADK store");
        let session_store = AdkSessionStore::open(&session_path).expect("reopen session store");
        let context =
            durable_context_items(&store, &session_store, "session-1", Some("run-current"))
                .expect("build durable context");
        let request = ModelRequest {
            endpoint: Url::parse("https://example.test/responses").expect("endpoint"),
            api_key: "secret".to_owned(),
            model: "fixture-model".to_owned(),
            instruction: Some("system instruction".to_owned()),
            message: "current request".to_owned(),
            durable_context: context,
            tool_context: Vec::new(),
            timeout: Duration::from_secs(1),
            tools: Vec::new(),
        };

        let input = model_input(&request);
        assert_eq!(
            input[0],
            json!({"role":"system","content":"system instruction"})
        );
        assert_eq!(
            input[1],
            json!({
                "role":"system",
                "content":"Durable session context:\nhandoff summary\n\ncompacted summary"
            })
        );
        assert_eq!(
            input[2],
            json!({"role":"user","content":"latest durable question"})
        );
        assert_eq!(input[3], json!({"role":"user","content":"current request"}));
        assert_eq!(input.len(), 4, "current event must not be duplicated");
    }
}
