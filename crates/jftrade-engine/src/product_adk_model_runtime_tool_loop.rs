impl ProductionAdkChatRuntime {
    /// Execute calls released by approval, persist every outcome, and feed
    /// durable function_call/function_call_output pairs back to Responses.
    /// A bounded loop prevents a provider from spinning forever.
    #[allow(clippy::never_loop)]
    fn run_approval_continuation(&self, chat: ChatExecution, cancellation: Arc<AtomicBool>) {
        let owner_id = lease_owner_id(&chat.run_id);
        let run_lease =
            match self.acquire_run_lease_with_retry(&chat.run_id, &owner_id, &cancellation) {
                Ok(lease) => lease,
                Err(AdkChatPortError::Conflict(_)) => return,
                Err(_) => return,
            };
        self.run_tool_loop(chat, cancellation, &run_lease);
    }

    /// Execute the durable `RUNNING` tool calls of one chat run and feed their
    /// results back to the provider until the model answers or asks for
    /// approval again.
    ///
    /// Callers that already own the run lease (the chat route and the live
    /// stream route) borrow it here so the fencing token never changes while
    /// the released calls execute.  The approval-continuation worker acquires
    /// its own lease first.
    #[allow(clippy::never_loop)]
    fn run_tool_loop(
        &self,
        mut chat: ChatExecution,
        cancellation: Arc<AtomicBool>,
        run_lease: &RunLeaseGuard,
    ) {
        const MAX_TOOL_ROUNDS: usize = 8;
        let owner_id = run_lease.owner_id().to_owned();
        if self
            .mark_provider_attempt_started(&chat, run_lease)
            .is_err()
        {
            return;
        }
        for _round in 0..MAX_TOOL_ROUNDS {
            if run_lease.is_lost()
                || cancellation.load(Ordering::Acquire)
                || self.run_is_cancelled(&chat.run_id)
            {
                if run_lease.is_lost() {
                    return;
                }
                let error = cancellation_error();
                let _ = self.persist_cancelled(&chat, &error, run_lease);
                return;
            }
            let run = match self.store.get_run(&chat.run_id) {
                Ok(Some(run)) => run,
                Ok(None) => return,
                Err(error) => {
                    let _ = self.persist_failure(&chat, &storage_unavailable(error), run_lease);
                    return;
                }
            };
            if !run.status.eq_ignore_ascii_case("RUNNING") {
                return;
            }
            let payload: Value = match serde_json::from_str(&run.payload_json) {
                Ok(payload) => payload,
                Err(error) => {
                    let failure = storage_unavailable(error);
                    let _ = self.persist_failure(&chat, &failure, run_lease);
                    return;
                }
            };
            let calls = executable_tool_calls(&payload);
            for call in calls {
                if run_lease.is_lost() {
                    return;
                }
                if cancellation.load(Ordering::Acquire) || self.run_is_cancelled(&chat.run_id) {
                    let error = cancellation_error();
                    let _ = self.persist_cancelled(&chat, &error, run_lease);
                    return;
                }
                let call_id = call
                    .get("id")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|id| !id.is_empty())
                    .unwrap_or_default()
                    .to_owned();
                let name = call
                    .get("name")
                    .and_then(Value::as_str)
                    .map(str::trim)
                    .filter(|name| !name.is_empty())
                    .unwrap_or_default()
                    .to_owned();
                let arguments = call.get("arguments").cloned().unwrap_or_else(|| json!({}));
                if call_id.is_empty() || name.is_empty() {
                    let error = tool_unavailable("malformed assistant tool call");
                    let _ = self.persist_failure(&chat, &error, run_lease);
                    return;
                }
                if !self.tool_executor.supports(&name) {
                    let error = tool_unavailable(format!(
                        "tool {name} is not supported by the production runtime"
                    ));
                    let _ = self.persist_failure(&chat, &error, run_lease);
                    return;
                }
                let fail_closed = !replay_safe_tool(&name);
                let idempotency_key = call_id.clone();
                let input_json = match serde_json::to_string(&arguments) {
                    Ok(input_json) => input_json,
                    Err(error) => {
                        let failure = storage_unavailable(error);
                        let _ = self.persist_failure(&chat, &failure, run_lease);
                        return;
                    }
                };
                let claim = self.claim_tool_invocation_with_retry(
                    &chat,
                    &idempotency_key,
                    &name,
                    &input_json,
                    &owner_id,
                    run_lease,
                    &cancellation,
                    fail_closed,
                );
                let (outcome, claim_owner, claim_fencing_token) = match claim {
                    Ok(Some(AdkToolInvocationClaim::Replay(invocation))) => (
                        serde_json::from_str::<Value>(&invocation.output_json)
                            .map(|value| {
                                if invocation.status.eq_ignore_ascii_case("SUCCEEDED") {
                                    Ok(value)
                                } else {
                                    // Go replays a persisted terminal result
                                    // verbatim, so a previously failed tool
                                    // reports the same failure again instead
                                    // of being retried.
                                    Err(tool_failed(format!(
                                        "tool {} returned a persisted failed outcome",
                                        invocation.tool_name
                                    )))
                                }
                            })
                            .unwrap_or_else(|error| Err(storage_unavailable(error))),
                        invocation.owner_id,
                        invocation.fencing_token,
                    ),
                    Ok(Some(AdkToolInvocationClaim::Execute(invocation))) => {
                        if run_lease.is_lost() {
                            return;
                        }
                        let mut heartbeat = match ToolClaimHeartbeat::start(
                            Arc::clone(&self.store),
                            invocation.clone(),
                        ) {
                            Ok(heartbeat) => heartbeat,
                            Err(error) => {
                                let _ = self.persist_failure(&chat, &error, run_lease);
                                return;
                            }
                        };
                        // Go maps an executor error onto the tool call with
                        // `ToolCallFailureMessage`, so the plain error text is
                        // what lands in `ToolCall.Error`; only the caller's
                        // unavailable/conflict wrappers keep a code prefix.
                        // `workflow.wait` spends its whole runtime sleeping, so
                        // the adapter is driven with the cancellation signal
                        // the reference observes through `ctx.Done()`.
                        let cancellation_handle = Arc::clone(&cancellation);
                        let run_id = chat.run_id.clone();
                        let store = Arc::clone(&self.store);
                        let cancellation_probe = Arc::new(move || {
                            cancellation_handle.load(Ordering::Acquire)
                                || store
                                    .get_run(&run_id)
                                    .ok()
                                    .flatten()
                                    .is_some_and(|run| {
                                        run.status.eq_ignore_ascii_case("CANCELLED")
                                    })
                        });
                        // Go's `executeRegisteredTool` bounds every handler
                        // with a 30s context and recovers panics into
                        // `tool panic: %v`; the runtime must never let a
                        // hanging or panicking tool wedge the run.
                        let outcome = execute_tool_with_timeout(
                            &self.tool_executor,
                            &name,
                            &arguments,
                            cancellation_probe,
                            self.tool_executor.execution_deadline(),
                        );
                        if heartbeat.stop() || run_lease.is_lost() {
                            return;
                        }
                        (outcome, invocation.owner_id, invocation.fencing_token)
                    }
                    Ok(Some(AdkToolInvocationClaim::Unknown(_))) => {
                        let error = AdkChatPortError::Failed {
                            status: 500,
                            code: "ADK_TOOL_OUTCOME_UNKNOWN".to_owned(),
                            message: format!(
                                "tool invocation {idempotency_key} expired while in flight; outcome unknown"
                            ),
                        };
                        let _ = self.persist_failure(&chat, &error, run_lease);
                        return;
                    }
                    Ok(Some(AdkToolInvocationClaim::Live(_))) => {
                        unreachable!("live tool claims are consumed by the retry helper")
                    }
                    Ok(None) => return,
                    Err(error) => {
                        let _ = self.persist_failure(&chat, &error, run_lease);
                        return;
                    }
                };
                match outcome {
                    Ok(output) => {
                        // Go's `googleADKTool.executeAndMap` owns this
                        // projection: a handler result that is not an object
                        // becomes `{"result": …}`, and a structured failure
                        // response becomes `structuredToolErrorEnvelope` and a
                        // failed call instead of a successful one.
                        let mapped = map_tool_output(&name, output);
                        if let Err(error) = self.persist_tool_result(
                            &chat,
                            &call,
                            &idempotency_key,
                            mapped.output,
                            &mapped.status,
                            mapped.error_text.as_deref(),
                            &claim_owner,
                            claim_fencing_token,
                            run_lease.token(),
                        ) {
                            if !is_nonfatal_durable_error(&error) {
                                let _ = self.persist_failure(&chat, &error, run_lease);
                            }
                            return;
                        }
                    }
                    Err(error) => {
                        // Go keeps the run alive when only a tool failed.
                        // `afterToolCallback` records the FAILED call and
                        // swallows the error, so the model keeps its turn and
                        // the run still terminates COMPLETED with
                        // `degraded: true` derived from `FirstToolCallFailure`.
                        // Persist the visible failure, then let the loop
                        // continue instead of failing the run.
                        // Go's `toolErrorEnvelope` keeps the raw error text on
                        // `error.message` and classifies it through
                        // `classifyToolError`, while the model-facing
                        // `message` carries the verb prefix.  `ToolCall.Error`
                        // is the raw text on both tool-failure paths.
                        let result = tool_error_envelope(&name, &error);
                        if let Err(commit_error) = self.persist_tool_result(
                            &chat,
                            &call,
                            &idempotency_key,
                            result,
                            "FAILED",
                            None,
                            &claim_owner,
                            claim_fencing_token,
                            run_lease.token(),
                        ) {
                            if !is_nonfatal_durable_error(&commit_error) && !run_lease.is_lost() {
                                let _ = self.persist_failure(&chat, &commit_error, run_lease);
                            }
                            return;
                        }
                    }
                }
            }

            let latest = match self.store.get_run(&chat.run_id) {
                Ok(Some(run)) => run,
                Ok(None) => return,
                Err(error) => {
                    let failure = storage_unavailable(error);
                    let _ = self.persist_failure(&chat, &failure, run_lease);
                    return;
                }
            };
            if !latest.status.eq_ignore_ascii_case("RUNNING") {
                return;
            }
            let latest_payload: Value = match serde_json::from_str(&latest.payload_json) {
                Ok(payload) => payload,
                Err(error) => {
                    let failure = storage_unavailable(error);
                    let _ = self.persist_failure(&chat, &failure, run_lease);
                    return;
                }
            };
            let empty = serde_json::Map::new();
            chat.request.tool_context =
                tool_context_from_payload(latest_payload.as_object().unwrap_or(&empty));
            let result = execute_model(chat.request.clone(), Arc::clone(&cancellation));
            if run_lease.is_lost() {
                return;
            }
            if cancellation.load(Ordering::Acquire) {
                let error = cancellation_error();
                let _ = self.persist_cancelled(&chat, &error, run_lease);
                return;
            }
            match result {
                Ok(response) if !response.tool_calls.is_empty() => {
                    match self.persist_tool_calls(&chat, &response, run_lease) {
                        // The model asked for a call that needs the operator:
                        // the run is parked and this worker stops.
                        Ok(ToolCallStaging::Pending(_)) => return,
                        // Released calls keep the run RUNNING, so the next loop
                        // round executes them.
                        Ok(ToolCallStaging::Released) => continue,
                        Err(error) => {
                            if !is_nonfatal_durable_error(&error) && !run_lease.is_lost() {
                                let _ = self.persist_failure(&chat, &error, run_lease);
                            }
                            return;
                        }
                    }
                }
                Ok(response) => {
                    if let Err(error) = self.persist_success(&chat, response, run_lease)
                        && !run_lease.is_lost()
                        && !is_nonfatal_durable_error(&error)
                    {
                        let _ = self.persist_failure(&chat, &error, run_lease);
                    }
                    return;
                }
                Err(error) => {
                    // A provider failure inside the tool loop reaches Go's
                    // `CompleteChatRun` as `adkErr`, so it is terminal: the run
                    // is marked FAILED and the synthetic reply is attached
                    // instead of the outage staying durable.
                    if !run_lease.is_lost() {
                        let _ = self.persist_failure(&chat, &error, run_lease);
                    }
                    return;
                }
            }
        }
        let error = AdkChatPortError::Failed {
            status: 503,
            code: "ADK_TOOL_LOOP_LIMIT".to_owned(),
            message: "assistant tool loop exceeded the production limit".to_owned(),
        };
        if !run_lease.is_lost() {
            let _ = self.persist_failure(&chat, &error, run_lease);
        }
    }

    #[allow(clippy::too_many_arguments)]
    fn claim_tool_invocation_with_retry(
        &self,
        chat: &ChatExecution,
        idempotency_key: &str,
        name: &str,
        input_json: &str,
        owner_id: &str,
        run_lease: &RunLeaseGuard,
        cancellation: &Arc<AtomicBool>,
        fail_closed: bool,
    ) -> Result<Option<AdkToolInvocationClaim>, AdkChatPortError> {
        loop {
            if run_lease.is_lost()
                || cancellation.load(Ordering::Acquire)
                || self.run_is_cancelled(&chat.run_id)
            {
                return Ok(None);
            }
            let run = self
                .store
                .get_run(&chat.run_id)
                .map_err(storage_unavailable)?
                .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
            let claim = self.store.claim_tool_invocation_if_status_and_revision(
                &chat.run_id,
                idempotency_key,
                name,
                input_json,
                "RUNNING",
                &run.updated_at,
                owner_id,
                run_lease.token(),
                TOOL_CLAIM_TTL,
                fail_closed,
            );
            match claim {
                Ok(AdkToolInvocationClaim::Live(invocation)) => {
                    while unix_now_ms() < invocation.lease_expires_at_unix_ms {
                        if run_lease.is_lost()
                            || cancellation.load(Ordering::Acquire)
                            || self.run_is_cancelled(&chat.run_id)
                        {
                            return Ok(None);
                        }
                        thread::sleep(Duration::from_millis(100));
                    }
                }
                Ok(claim) => return Ok(Some(claim)),
                Err(error) => match classify_durable_store_error(&error) {
                    DurableErrorClass::LeaseHeldOrLost | DurableErrorClass::RevisionConflict => {
                        return Ok(None);
                    }
                    DurableErrorClass::InvariantViolation | DurableErrorClass::StorageFailure => {
                        return Err(runtime_store_error(error));
                    }
                },
            }
        }
    }

    #[allow(clippy::too_many_arguments)]
    fn persist_tool_result(
        &self,
        chat: &ChatExecution,
        call: &Value,
        idempotency_key: &str,
        output: Value,
        status: &str,
        error_text: Option<&str>,
        owner_id: &str,
        fencing_token: i64,
        run_lease_token: i64,
    ) -> Result<(), AdkChatPortError> {
        let run = self
            .store
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        if !run.status.eq_ignore_ascii_case("RUNNING") {
            return Err(AdkChatPortError::Conflict(format!(
                "assistant chat run is already {}",
                run.status
            )));
        }
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        let object = payload
            .as_object_mut()
            .ok_or_else(|| unavailable("persisted ADK run payload must be an object"))?;
        let call_id = call
            .get("id")
            .and_then(Value::as_str)
            .unwrap_or(idempotency_key);
        let tool_name = call
            .get("name")
            .and_then(Value::as_str)
            .unwrap_or("unknown");
        let input = call.get("arguments").cloned().unwrap_or_else(|| json!({}));
        let output_json = serde_json::to_string(&output)
            .map_err(|error| unavailable(format!("encode tool result: {error}")))?;
        let input_json = serde_json::to_string(&input)
            .map_err(|error| unavailable(format!("encode tool arguments: {error}")))?;
        let results = object
            .entry("toolResults")
            .or_insert_with(|| Value::Array(Vec::new()))
            .as_array_mut()
            .ok_or_else(|| unavailable("persisted ADK tool result list is invalid"))?;
        if !results.iter().any(|result| {
            result
                .get("callId")
                .and_then(Value::as_str)
                .is_some_and(|id| id == call_id)
        }) {
            results.push(json!({
                "runId": chat.run_id,
                "callId": call_id,
                "functionCallId": call_id,
                "name": tool_name,
                "toolName": tool_name,
                "arguments": input,
                "status": status,
                "output": output,
                "createdAt": run.created_at,
                "updatedAt": run.updated_at,
            }));
        }
        if let Some(Value::Array(tool_calls)) = object.get_mut("toolCalls") {
            for item in tool_calls {
                if item.get("id").and_then(Value::as_str) != Some(call_id) {
                    continue;
                }
                if let Some(item) = item.as_object_mut() {
                    item.insert("status".to_owned(), Value::String(status.to_owned()));
                    item.insert("requiresUser".to_owned(), Value::Bool(false));
                    item.insert("output".to_owned(), output.clone());
                    item.insert(
                        "completedAt".to_owned(),
                        Value::String(run.updated_at.clone()),
                    );
                    if status.eq_ignore_ascii_case("SUCCEEDED") {
                        // A later successful attempt clears the previous
                        // failure so the projection never mixes both.
                        item.remove("error");
                        item.remove("errorCode");
                    } else {
                        // Go surfaces the tool failure on the run itself:
                        // `ToolCall.Error` carries the message and
                        // `FailureReason`/`ErrorCode` stay empty at run level.
                        // A structured failure response records the envelope's
                        // model-facing text, while an executor error records
                        // the envelope's nested raw text.
                        let error_text = error_text
                            .map(str::to_owned)
                            .unwrap_or_else(|| tool_result_error_message(&output));
                        item.insert(
                            "error".to_owned(),
                            Value::String(error_text),
                        );
                        item.insert(
                            "errorCode".to_owned(),
                            Value::String(if status == "UNKNOWN" {
                                "ADK_TOOL_OUTCOME_UNKNOWN".to_owned()
                            } else {
                                tool_result_error_code(&output)
                            }),
                        );
                    }
                }
            }
        }
        object.insert(
            "resumeState".to_owned(),
            Value::String("tool_result_persisted".to_owned()),
        );
        let event_id = format!("{}:tool:{}", chat.run_id, idempotency_key);
        let event_content = serde_json::to_string(&json!({
            "callId": call_id,
            "name": tool_name,
            "status": status,
            "output": output,
        }))
        .map_err(|error| unavailable(format!("encode tool event: {error}")))?;
        let event = AdkRunEvent {
            id: &event_id,
            session_id: &chat.session_id,
            invocation_id: &chat.run_id,
            author: "assistant.tool",
            content: &event_content,
        };
        let commit = self
            .store
            .commit_tool_result_if_status_and_revision_with_event(
                &chat.run_id,
                "RUNNING",
                &run.updated_at,
                &payload.to_string(),
                idempotency_key,
                tool_name,
                &input_json,
                &output_json,
                status,
                owner_id,
                fencing_token,
                run_lease_token,
                self.session_store.as_ref(),
                &event,
            )
            .map_err(runtime_store_error)?;
        if commit.changed {
            return Ok(());
        }
        // A concurrent continuation already committed this result. Treat it
        // as idempotent success; the next loop reads the durable projection.
        Ok(())
    }
}

include!("product_adk_model_runtime_tool_timeout.rs");

/// Select calls released by approval that do not yet have a durable result.
fn executable_tool_calls(payload: &Value) -> Vec<Value> {
    let Some(calls) = payload.get("toolCalls").and_then(Value::as_array) else {
        return Vec::new();
    };
    let results = payload
        .get("toolResults")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    calls
        .iter()
        .filter(|call| {
            let is_interaction = call
                .get("name")
                .and_then(Value::as_str)
                .is_some_and(|n| n == "interaction.request_user");
            if is_interaction {
                return false;
            }
            let running = call
                .get("status")
                .and_then(Value::as_str)
                .is_some_and(|status| status.eq_ignore_ascii_case("RUNNING"));
            let id = call.get("id").and_then(Value::as_str).unwrap_or_default();
            running
                && !results.iter().any(|result| {
                    result
                        .get("callId")
                        .and_then(Value::as_str)
                        .is_some_and(|result_id| result_id == id)
                })
        })
        .cloned()
        .collect()
}

/// Reconstruct Responses API function-call context from the durable run.
fn tool_context_from_payload(object: &serde_json::Map<String, Value>) -> Vec<Value> {
    let calls = object
        .get("toolCalls")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let results = object
        .get("toolResults")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let mut context = Vec::new();
    for call in calls {
        let Some(call_id) = call.get("id").and_then(Value::as_str) else {
            continue;
        };
        let Some(name) = call.get("name").and_then(Value::as_str) else {
            continue;
        };
        let arguments = call.get("arguments").cloned().unwrap_or_else(|| json!({}));
        let arguments = serde_json::to_string(&arguments).unwrap_or_else(|_| "{}".to_owned());
        context.push(json!({
            "type": "function_call",
            "call_id": call_id,
            "name": name,
            "arguments": arguments,
        }));
        if let Some(result) = results.iter().find(|result| {
            result
                .get("callId")
                .and_then(Value::as_str)
                .is_some_and(|result_id| result_id == call_id)
        }) {
            let output = result.get("output").cloned().unwrap_or(Value::Null);
            let output = serde_json::to_string(&output).unwrap_or_else(|_| "null".to_owned());
            context.push(json!({
                "type": "function_call_output",
                "call_id": call_id,
                "output": output,
            }));
        } else if name == "interaction.request_user"
            && let Some(input_response) = object.get("inputResponse")
        {
            let output = serde_json::to_string(input_response).unwrap_or_else(|_| "null".to_owned());
            context.push(json!({
                "type": "function_call_output",
                "call_id": call_id,
                "output": output,
            }));
        }
    }
    context
}

fn tool_unavailable(message: impl Into<String>) -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 503,
        code: "ADK_TOOL_UNAVAILABLE".to_owned(),
        message: message.into(),
    }
}

/// A tool-level failure that must stay visible on the run without failing it.
/// Go classifies these through `ClassifyToolError`/`ClassifyToolErrorText` and
/// records them on the `ToolCall`, so callers read the message from the run's
/// tool list instead of a run-level error envelope.
fn tool_failed(message: impl Into<String>) -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 503,
        code: "TOOL_EXECUTION_FAILED".to_owned(),
        message: message.into(),
    }
}

/// The raw text Go persists on the tool call for any `AdkChatPortError`
/// variant.  `Unavailable`/`Conflict` carry no code prefix, matching their
/// `error.Error()` rendering.
fn tool_error_text(error: &AdkChatPortError) -> String {
    match error {
        AdkChatPortError::Unavailable(message) | AdkChatPortError::Conflict(message) => {
            message.clone()
        }
        AdkChatPortError::Failed { message, .. } => message.clone(),
    }
}

/// Go's `classifyToolError`: execution failures default to
/// `TOOL_EXECUTION_FAILED`, while timeouts, cancellations, unknown outcomes
/// and lost run leases carry their own code and retryability.
fn classify_tool_failure(error: &AdkChatPortError) -> (&'static str, bool) {
    match error {
        AdkChatPortError::Failed { code, .. } => match code.as_str() {
            "MODEL_CALL_TIMEOUT" | "TOOL_EXECUTION_TIMEOUT" => ("TIMEOUT", true),
            "TOOL_EXECUTION_CANCELLED" => ("CANCELLED", false),
            "RUN_CANCELLED" | "CLIENT_DISCONNECTED" => ("CANCELLED", false),
            "ADK_TOOL_OUTCOME_UNKNOWN" => ("SUBMISSION_UNKNOWN", false),
            "ADK_RUN_LEASE_LOST" => ("RUN_LEASE_LOST", true),
            _ => ("TOOL_EXECUTION_FAILED", false),
        },
        _ => ("TOOL_EXECUTION_FAILED", false),
    }
}

/// The failure text Go records on `ToolCall.Error`.
///
/// Go has two tool-failure paths and both end at the raw error text: a tool
/// executor error is classified through `classifyToolExecutionError`, and a
/// structured `{"success":false}` response is classified through
/// `structuredToolErrorEnvelope`, whose nested `error.message` is that same
/// raw text.  The envelope's top-level `message` is only the model-facing
/// description (`工具 <name> 执行失败: <raw>`), so the nested value wins here.
fn tool_result_error_message(output: &Value) -> String {
    output
        .pointer("/error/message")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|message| !message.is_empty())
        .or_else(|| {
            output
                .get("message")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|message| !message.is_empty())
        })
        .map(str::to_owned)
        .unwrap_or_else(|| {
            output
                .pointer("/error")
                .filter(|error| error.is_string())
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|message| !message.is_empty())
                .unwrap_or("tool execution failed")
                .to_owned()
        })
}

/// The persisted tool-call `errorCode`, mirroring Go's `classifyToolError` /
/// `structuredToolErrorMetadata` projection.
fn tool_result_error_code(output: &Value) -> String {
    output
        .pointer("/error/code")
        .or_else(|| output.get("errorCode"))
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|code| !code.is_empty())
        .map(|code| code.to_ascii_uppercase())
        .unwrap_or_else(|| "TOOL_EXECUTION_FAILED".to_owned())
}
