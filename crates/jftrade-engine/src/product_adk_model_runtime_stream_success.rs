// Successful-turn persistence for the production ADK chat runtime.
//
// Textually included by `product_adk_model_runtime_stream.rs` so the runtime
// impl stays in one module scope while each fragment respects the workspace
// 800-line production limit.

impl ProductionAdkChatRuntime {
    pub(super) fn persist_success(
        &self,
        chat: &ChatExecution,
        model_response: ModelResponse,
        run_lease: &RunLeaseGuard,
    ) -> Result<Value, AdkChatPortError> {
        let run = self
            .store()
            .get_run(&chat.run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        if run.status.eq_ignore_ascii_case("CANCELLED") {
            return Err(run_cancelled());
        }
        if !run.status.eq_ignore_ascii_case("RUNNING") {
            if let Some(response) = super::persisted_response(&run.payload_json)? {
                return Ok(response);
            }
            return Err(unavailable(format!("run is already {}", run.status)));
        }
        let now = run.updated_at.clone();
        let session = self
            .store()
            .get_session(&chat.session_id)
            .map_err(storage_unavailable)?
            .map(|session| {
                // Go serves the stored session entity, whose title the console
                // renders as the transcript header.
                let title = serde_json::from_str::<Value>(&session.payload_json)
                    .ok()
                    .and_then(|payload| {
                        payload
                            .get("title")
                            .and_then(Value::as_str)
                            .map(str::to_owned)
                    })
                    .unwrap_or_default();
                json!({
                    "id": session.id,
                    "agentId": chat.agent_id,
                    "title": title,
                    "createdAt": session.created_at,
                    "updatedAt": session.updated_at,
                })
            })
            .unwrap_or_else(|| json!({"id": chat.session_id, "agentId": chat.agent_id}));
        let text = model_response.text;
        // Go's `MarkCompletedChatRun` keeps a run COMPLETED even when a tool
        // call failed, and records that shape with `degraded: true` derived
        // from the first TIMED_OUT/FAILED/CANCELLED tool call.  The flag is
        // what tells the console the reply is usable but partial.
        let degraded = super::first_tool_call_failure(&run.payload_json).is_some();
        // Go projects the stored run (including its tool calls) into the chat
        // envelope, so a failed tool stays visible on the returned run while
        // the run-level `failureReason`/`errorCode` stay empty.
        let stored: Value = serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        // `MarkCompletedChatRun` clears the run-level failure projection before
        // Go projects the run onto the wire, so a stale `errorCode` or
        // `failureReason` from an earlier attempt never reaches the console.
        let mut projection_payload = stored.clone();
        if let Some(object) = projection_payload.as_object_mut() {
            for key in [
                "errorCode",
                "errorMessage",
                "errorStatus",
                "failureReason",
                "providerRetry",
            ] {
                object.remove(key);
            }
        }
        let tool_calls = stored
            .get("toolCalls")
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();
        // Go serves the *merged* assistant text of the run, not just the last
        // model answer: the text that preceded the first tool call and the text
        // the model produced after the tool results are concatenated
        // (`mergeProjectedText` in the ADK projection), which the frozen
        // `chat-success` fixture pins for a plain turn.
        let pre_tool_content = stored
            .get("preToolContent")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .trim()
            .to_owned();
        let reply = runtime_projection::merge_projected_text(&pre_tool_content, text.trim(), false);
        let reply = reply.trim().to_owned();
        // Go's `hydrateResumedRun` finishes an approval continuation with
        // `resumeState=adk_confirmation_resolved` and a `completedAt` stamp, so
        // the console can tell a resumed run from a plain chat run.  The
        // reference writes the state before the run becomes terminal, so the
        // projection must carry both fields on the terminal envelope too.
        // Go's `AttachFinalAssistantMessage` derives the transcript id from
        // `syntheticAssistantMessageID(runID, replyResult)` and writes
        // `run.FinalMessageID = message.ID`, so the timeline entry, the stored
        // transcript event and the run link all share one id.  A successful
        // turn carries no synthetic kind, which Go's helper defaults to
        // `local`.  The reasoning slot stays empty: Rust's `ModelResponse`
        // carries only the visible text, so there is no reasoning buffer to
        // fold into the digest the way Go's `googleADKExecution.result()` does.
        let final_message_id = synthetic_assistant_message_id(&chat.run_id, "local", "", &reply);
        let completed_at = now.clone();
        // Go distinguishes the two resume kinds: `hydrateResumedRun` finishes
        // an approval continuation with `adk_confirmation_resolved`, while
        // `completeInputContinuation` finishes an answered input request with
        // `input_resolved`.
        let resume_state = Value::String(if chat.resumed_from_input {
            "input_resolved".to_owned()
        } else if chat.resumed {
            "adk_confirmation_resolved".to_owned()
        } else {
            String::new()
        });
        // Go answers with `ProjectedChatResponse`: the durable run projected
        // over `assistantmodel.Run`'s wire field set (including the tool
        // activity and usage the projection derives) plus the session
        // timeline.  Rust answered with an ad-hoc object, so the completed
        // envelope lost `userMessage`, `usage`, `workMode`, `startedAt`,
        // `toolSummaries`, `optimizationTaskId` and the user's own timeline
        // entry even though the durable row knew all of them.
        let mut overrides: Vec<(&str, Value)> = vec![
            ("id", Value::String(chat.run_id.clone())),
            ("sessionId", Value::String(chat.session_id.clone())),
            ("agentId", Value::String(chat.agent_id.clone())),
            ("status", Value::String("COMPLETED".to_owned())),
            // Go's `MarkCompletedChatRun` writes the fixed literal
            // "completed" to `Run.Message`; the assistant text lives in the
            // response-level `reply`.
            ("message", Value::String("completed".to_owned())),
            ("pendingApprovals", Value::Array(Vec::new())),
            ("degraded", Value::Bool(degraded)),
            ("resumeState", resume_state),
            ("completedAt", Value::String(completed_at.clone())),
            ("finalMessageId", Value::String(final_message_id.clone())),
            ("createdAt", Value::String(run.created_at.clone())),
            ("updatedAt", Value::String(now.clone())),
            (
                "usage",
                runtime_projection::usage_wire_value(&stored, &tool_calls, &completed_at),
            ),
        ];
        overrides.extend(runtime_projection::tool_projection_fields(&tool_calls));
        if !pre_tool_content.is_empty() {
            overrides.push(("preToolContent", Value::String(pre_tool_content)));
        }
        let run_value = runtime_projection::go_run_wire(
            &projection_payload,
            GO_RUN_PROJECTION_FIELDS,
            overrides,
        );
        let timeline = self.session_timeline_value(
            &chat.session_id,
            Some(runtime_projection::PendingTimelineEntry {
                id: &final_message_id,
                session_id: &chat.session_id,
                run_id: &chat.run_id,
                text: &reply,
                created_at: &completed_at,
            }),
        )?;
        let response = json!({
            "reply": reply.clone(),
            "session": session,
            "run": run_value,
            "pendingApprovals": [],
            "timeline": timeline,
        });
        // Go's `publishFinal` clears each ToolCall.Output before emitting the
        // terminal SSE frame.  Keep the full activity on the durable run, but
        // make the stored response used by reconnect replay match that wire
        // boundary for Stream requests.
        let terminal_response = if chat.route == AdkChatRoute::Stream {
            let mut trimmed = response.clone();
            trim_stream_tool_outputs(&mut trimmed);
            trimmed
        } else {
            response.clone()
        };
        let mut payload: Value =
            serde_json::from_str(&run.payload_json).map_err(storage_unavailable)?;
        payload["id"] = Value::String(chat.run_id.clone());
        payload["sessionId"] = Value::String(chat.session_id.clone());
        payload["agentId"] = Value::String(chat.agent_id.clone());
        payload["status"] = Value::String("COMPLETED".to_owned());
        payload["reply"] = Value::String(reply.clone());
        payload["message"] = Value::String("completed".to_owned());
        payload["degraded"] = Value::Bool(degraded);
        payload["completedAt"] = Value::String(completed_at.clone());
        // The projection fields the envelope served are durable too, so a
        // reconnect or a `clientRequestId` replay rebuilds the identical
        // response instead of re-deriving the tool activity.
        payload["toolCalls"] = Value::Array(tool_calls.clone());
        payload["toolSummaries"] = json!(runtime_projection::tool_summaries_for_run(&tool_calls));
        let optimization_task_id = runtime_projection::optimization_task_id(&tool_calls);
        if !optimization_task_id.is_empty() {
            payload["optimizationTaskId"] = Value::String(optimization_task_id);
        }
        payload["usage"] =
            runtime_projection::usage_wire_value(&stored, &tool_calls, &completed_at);
        if chat.resumed {
            payload["resumeState"] = Value::String(if chat.resumed_from_input {
                "input_resolved".to_owned()
            } else {
                "adk_confirmation_resolved".to_owned()
            });
        }
        if let Some(object) = payload.as_object_mut() {
            object.remove("providerRetry");
            // `MarkCompletedChatRun` clears the run-level failure projection;
            // the tool failure remains visible on `toolCalls` only.
            object.insert("failureReason".to_owned(), Value::String(String::new()));
            object.insert("errorCode".to_owned(), Value::String(String::new()));
            object.insert("errorMessage".to_owned(), Value::String(String::new()));
            object.remove("errorStatus");
            object.insert("pendingApprovals".to_owned(), Value::Array(Vec::new()));
        }
        payload["response"] = terminal_response.clone();
        let mut final_sequence = None;
        if chat.route == AdkChatRoute::Stream {
            let events = payload
                .get_mut("streamEvents")
                .and_then(Value::as_array_mut)
                .ok_or_else(|| unavailable("persisted ADK run has no stream event list"))?;
            let sequence = events.len() as u64 + 1;
            let mut final_event = json!({"type":"final","response":terminal_response.clone()});
            if let Some(object) = final_event.as_object_mut() {
                object.insert("streamId".to_owned(), Value::String(chat.run_id.clone()));
                object.insert("sequence".to_owned(), Value::from(sequence));
                object.insert("runId".to_owned(), Value::String(chat.run_id.clone()));
            }
            events.push(final_event);
            final_sequence = Some(sequence);
        }
        payload["finalMessageId"] = Value::String(final_message_id.clone());
        let assistant_event = AdkRunEvent {
            id: &final_message_id,
            session_id: &chat.session_id,
            invocation_id: &chat.run_id,
            author: &chat.agent_id,
            content: &reply,
        };
        let stream_event_id =
            final_sequence.map(|sequence| format!("{}:stream:{}", chat.run_id, sequence));
        let stream_event = stream_event_id.as_ref().map(|id| AdkRunEvent {
            id,
            session_id: &chat.session_id,
            invocation_id: &chat.run_id,
            author: "assistant.stream",
            content: response
                .get("reply")
                .and_then(Value::as_str)
                .unwrap_or_default(),
        });
        let mut events = vec![assistant_event];
        if let Some(stream_event) = stream_event.as_ref() {
            events.push(stream_event.clone());
        }
        let updated = self
            .store()
            .update_run_state_if_status_and_revision_with_events_with_lease(
                &chat.run_id,
                "RUNNING",
                &run.updated_at,
                "COMPLETED",
                &payload.to_string(),
                self.session_store.as_ref(),
                &events,
                run_lease.owner_id(),
                run_lease.token(),
            )
            .map_err(storage_unavailable)?;
        if !updated {
            let current = self
                .store()
                .get_run(&chat.run_id)
                .map_err(storage_unavailable)?;
            let Some(current) = current else {
                return Err(unavailable("persisted ADK run disappeared"));
            };
            if current.status.eq_ignore_ascii_case("CANCELLED") {
                return Err(run_cancelled());
            }
            if !current.status.eq_ignore_ascii_case("COMPLETED") {
                return Err(unavailable(
                    "assistant chat run state changed before completion",
                ));
            }
            return super::persisted_response(&current.payload_json)?
                .ok_or_else(|| unavailable("completed ADK run has no persisted response"));
        }
        // Go's `auditResumedRun` runs before `PersistRunTerminalState`, so an
        // approval continuation writes `run.resumed` (carrying the resolved
        // resume state) ahead of the terminal lifecycle row.
        if chat.resumed_from_input {
            // Go's `completeInputContinuation` audits the answered-input
            // completion as `run.input_resolved` with only the run id, instead
            // of the `run.resumed` row an approval continuation writes.
            self.record_run_audit(&RunAuditEvent {
                id: format!("{}:audit:run.input_resolved", chat.run_id),
                subject_id: chat.run_id.clone(),
                kind: "run.input_resolved",
                detail: "Agent run completed after user input.",
                metadata: json!({"runId": chat.run_id}),
            });
        } else if chat.resumed {
            record_resumed_run_audit(
                self.store.as_ref(),
                &chat.run_id,
                &chat.agent_id,
                "COMPLETED",
                "adk_confirmation_resolved",
                "",
            );
        }
        // Go's `PersistRunTerminalState` audits `run.completed` (or the
        // status-specific kind) with `RunID`/`AgentID`/`Status` and the
        // non-empty `ErrorCode`/`FailureReason`.
        self.record_run_audit(&RunAuditEvent {
            id: format!("{}:audit:run.completed", chat.run_id),
            subject_id: chat.run_id.clone(),
            kind: "run.completed",
            detail: terminal_audit_message("COMPLETED"),
            metadata: terminal_audit_fields(&chat.run_id, &chat.agent_id, "COMPLETED", "", ""),
        });
        Ok(response)
    }

}

fn trim_stream_tool_outputs(response: &mut Value) {
    let Some(tool_calls) = response.pointer_mut("/run/toolCalls").and_then(Value::as_array_mut)
    else {
        return;
    };
    for tool_call in tool_calls {
        if let Some(object) = tool_call.as_object_mut() {
            object.remove("output");
        }
    }
}
