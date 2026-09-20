// Tool-call staging and the pending-approval projection for one chat run.
//
// Extracted from `product_adk_model_runtime_events.rs` to keep the production
// file under the 800-line architecture limit; it is a single included `impl`
// block so module-scope helpers and visibility are unchanged.

/// Outcome of staging one model response's tool calls.
///
/// The reference builds every `FunctionTool` with
/// `RequireConfirmation: ToolRequiresApproval(descriptor, permissionMode)`.
/// Calls that do not require confirmation are executed by the agent loop in
/// the same turn, so staging must tell the caller whether the run is parked on
/// the operator (`Pending`) or whether the released calls still have to run
/// (`Released`).
#[derive(Debug)]
enum ToolCallStaging {
    /// At least one call is waiting for the operator; the JSON envelope is the
    /// terminal projection for this turn.
    Pending(AdkChatPortOutput),
    /// Every call was released for immediate execution.  The caller owns the
    /// run lease and must continue with the tool loop.
    Released,
}

impl ProductionAdkChatRuntime {
    fn persist_tool_calls(
        &self,
        chat: &ChatExecution,
        response: &ModelResponse,
        run_lease: &RunLeaseGuard,
    ) -> Result<ToolCallStaging, AdkChatPortError> {
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
        if let Some(input_call) = response
            .tool_calls
            .iter()
            .find(|call| call.name == "interaction.request_user")
        {
            return self
                .persist_pending_input_call(chat, input_call, &run, payload, run_lease)
                .map(ToolCallStaging::Pending);
        }
        // Availability and replay safety are separate decisions.  A call is
        // "known" when this process owns a concrete adapter for it; replay
        // safety only controls whether an expired claim may be retried
        // (`fail_closed` in the tool loop).  Folding the two together used to
        // reject every non-replayable production tool (for example the
        // `live_trading` class) with 503 instead of asking the operator.
        let known = response
            .tool_calls
            .iter()
            .all(|call| self.tool_executor.supports(&call.name));
        // The reference builds each `FunctionTool` with
        // `RequireConfirmation: ToolRequiresApproval(descriptor, permissionMode)`.
        // A call that does not require confirmation runs immediately and the
        // chat response already carries its result; only the gated ones become
        // `PENDING_APPROVAL` and park the run.  Staging everything for approval
        // made every read tool wait for an operator that the reference never
        // asks.
        let gated = response
            .tool_calls
            .iter()
            .map(|call| self.tool_catalog.requires_approval(&call.name, &chat.permission_mode))
            .collect::<Vec<_>>();
        let any_gated = gated.iter().any(|value| *value);
        let status = if !known {
            "FAILED"
        } else if any_gated {
            "PENDING"
        } else {
            "RUNNING"
        };
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
            let call_gated = known && gated[index];
            let call_status = if !known {
                "FAILED"
            } else if call_gated {
                "PENDING_APPROVAL"
            } else {
                // Released immediately: the tool loop below picks `RUNNING`
                // calls up through `executable_tool_calls`.
                "RUNNING"
            };
            let approval_id = format!("{}:approval:r{}:{}", chat.run_id, round, index + 1);
            let confirmation_call_id = format!("{approval_id}:confirmation");
            let call_value = json!({
                "id": call.id,
                "runId": chat.run_id,
                "functionCallId": call.id,
                "confirmationCallId": if call_gated { Value::String(confirmation_call_id.clone()) } else { Value::Null },
                "name": call.name,
                "toolName": call.name,
                "arguments": call.arguments,
                "input": call.arguments,
                "status": call_status,
                "requiresUser": call_gated,
                "approvalId": if call_gated { Value::String(approval_id.clone()) } else { Value::Null },
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
            if call_gated {
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
            object.remove("providerRetry");
            if known && any_gated {
                // Go's `finishPendingApprovalRun` records the resumable reason
                // next to the parked run so a reconnect can tell an approval
                // wait from a provider retry.
                object.insert(
                    "resumeState".to_owned(),
                    Value::String("waiting_approval".to_owned()),
                );
            }
            let message = if !known {
                "assistant requested an unavailable tool"
            } else if any_gated {
                "等待用户审批后继续执行。"
            } else {
                "assistant tool call released for execution"
            };
            object.insert("message".to_owned(), Value::String(message.to_owned()));
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
                "status": if !known {
                    "FAILED"
                } else if gated[index] {
                    "PENDING_APPROVAL"
                } else {
                    "RUNNING"
                },
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
        if !any_gated {
            // Every requested call runs without confirmation, so the run stays
            // RUNNING with `RUNNING` calls for the tool loop to pick up.
            return Ok(ToolCallStaging::Released);
        }
        let session = self
            .store
            .get_session(&chat.session_id)
            .map_err(storage_unavailable)?
            .map(|session| {
                json!({"id": session.id, "agentId": chat.agent_id, "createdAt": session.created_at, "updatedAt": session.updated_at})
            })
            .unwrap_or_else(|| json!({"id": chat.session_id, "agentId": chat.agent_id}));
        // Go's `finishPendingApprovalRun` answers with the approval prompt
        // (and no assistant placeholder message), so the console can show the
        // operator exactly what to do next.
        Ok(ToolCallStaging::Pending(AdkChatPortOutput::Json(json!({
            "reply": "我已经准备好执行需要授权的操作，请先在 ADK 审批队列里确认或拒绝。",
            "session": session,
            "run": payload,
            "pendingApprovals": pending,
            "timeline": [],
        }))))
    }
}
