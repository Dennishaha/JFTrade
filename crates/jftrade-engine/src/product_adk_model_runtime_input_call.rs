impl ProductionAdkChatRuntime {
    fn persist_pending_input_call(
        &self,
        chat: &ChatExecution,
        call: &ModelToolCall,
        run: &StoredAdkRun,
        mut payload: Value,
        run_lease: &RunLeaseGuard,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        let timestamp = run.updated_at.clone();
        let args = &call.arguments;
        let title = args
            .get("title")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|v| !v.is_empty())
            .unwrap_or("需要用户决策");
        let decision_kind = args
            .get("decisionKind")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|v| !v.is_empty())
            .unwrap_or("missing_required_context");
        let blocking_reason = args
            .get("blockingReason")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|v| !v.is_empty())
            .unwrap_or("请根据提示确认后继续执行。");

        let questions_val = args.get("questions").and_then(Value::as_array);
        let questions: Vec<Value> = match questions_val {
            Some(raw_questions) if !raw_questions.is_empty() => raw_questions
                .iter()
                .enumerate()
                .map(|(q_idx, q)| {
                    let q_id = format!("q{}", q_idx + 1);
                    let q_text = q.get("question").and_then(Value::as_str).unwrap_or("请选择");
                    let allow_other = q.get("allowOther").and_then(Value::as_bool).unwrap_or(true);
                    let options = q.get("options").and_then(Value::as_array).map(|opts| {
                        opts.iter().enumerate().map(|(o_idx, opt)| {
                            let label = opt.get("label").and_then(Value::as_str).unwrap_or("选项");
                            let desc = opt.get("description").and_then(Value::as_str).unwrap_or("");
                            let rec = opt.get("recommended").and_then(Value::as_bool).unwrap_or(false);
                            json!({
                                "id": format!("{q_id}-o{}", o_idx + 1),
                                "label": label,
                                "description": desc,
                                "recommended": rec,
                            })
                        }).collect::<Vec<_>>()
                    }).unwrap_or_default();
                    json!({
                        "id": q_id,
                        "question": q_text,
                        "options": options,
                        "allowOther": allow_other,
                    })
                })
                .collect(),
            _ => vec![json!({
                "id": "q1",
                "question": blocking_reason,
                "options": [
                    {"id": "q1-o1", "label": "确认", "description": "", "recommended": true},
                    {"id": "q1-o2", "label": "取消", "description": "", "recommended": false}
                ],
                "allowOther": true,
            })],
        };

        static INPUT_REQUEST_SEQUENCE: std::sync::atomic::AtomicU64 =
            std::sync::atomic::AtomicU64::new(1);
        let millis = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .ok()
            .and_then(|d| u64::try_from(d.as_millis()).ok())
            .unwrap_or_default();
        let seq = INPUT_REQUEST_SEQUENCE.fetch_add(1, std::sync::atomic::Ordering::Relaxed);
        let request_id = format!("input-{millis}-{seq}");

        let input_request = json!({
            "id": request_id,
            "runId": chat.run_id,
            "agentId": chat.agent_id,
            "functionCallId": call.id,
            "title": title,
            "status": "PENDING",
            "decisionKind": decision_kind,
            "blockingReason": blocking_reason,
            "questions": questions,
            "answers": [],
            "createdAt": timestamp.clone(),
            "updatedAt": timestamp.clone(),
            "answeredAt": Value::Null,
        });

        let prior_calls = payload
            .get("toolCalls")
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();
        let mut tool_calls = prior_calls;
        let prior_round = tool_calls
            .iter()
            .filter_map(|c| c.get("round").and_then(Value::as_u64))
            .max()
            .unwrap_or_default();
        let round = prior_round.saturating_add(1);

        let call_value = json!({
            "id": call.id,
            "runId": chat.run_id,
            "functionCallId": call.id,
            "confirmationCallId": Value::Null,
            "name": "interaction.request_user",
            "toolName": "interaction.request_user",
            "arguments": call.arguments,
            "input": call.arguments,
            "status": "PENDING_INPUT",
            "requiresUser": true,
            "approvalId": Value::Null,
            "idempotencyKey": call.id,
            "error": Value::Null,
            "errorCode": Value::Null,
            "round": round,
            "permission": "approval",
            "reason": blocking_reason,
            "createdAt": timestamp.clone(),
            "updatedAt": timestamp.clone(),
        });
        tool_calls.push(call_value);

        let object = payload.as_object_mut().expect("payload object checked");
        object.insert("toolCalls".to_owned(), Value::Array(tool_calls));
        object.insert("pendingApprovals".to_owned(), Value::Array(Vec::new()));
        object.insert("status".to_owned(), Value::String("PENDING_INPUT".to_owned()));
        object.insert("resumeState".to_owned(), Value::String("waiting_input".to_owned()));
        object.insert("message".to_owned(), Value::String("等待用户回答后继续执行。".to_owned()));
        object.insert("inputRequest".to_owned(), input_request.clone());
        let mut input_requests = object
            .get("inputRequests")
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default();
        input_requests.push(input_request.clone());
        object.insert("inputRequests".to_owned(), Value::Array(input_requests));

        let payload_json = payload.to_string();
        let updated = self
            .store
            .stage_tool_calls_if_status_and_revision_with_events_with_lease(
                &chat.run_id,
                "RUNNING",
                &run.updated_at,
                "PENDING_INPUT",
                &payload_json,
                &[],
                self.session_store.as_ref(),
                &[],
                run_lease.owner_id(),
                run_lease.token(),
            )
            .map_err(runtime_store_error)?;

        if !updated {
            return Err(AdkChatPortError::Conflict(
                "assistant chat run state changed before input request staging".to_owned(),
            ));
        }

        let session = self
            .store
            .get_session(&chat.session_id)
            .map_err(storage_unavailable)?
            .map(|session| {
                json!({"id": session.id, "agentId": chat.agent_id, "createdAt": session.created_at, "updatedAt": session.updated_at})
            })
            .unwrap_or_else(|| json!({"id": chat.session_id, "agentId": chat.agent_id}));

        // Go's `FinishPendingInputRun` audits `run.awaiting_input` with the
        // run/agent/status/requestId fields (plus `decisionKind` when the
        // originating tool call declared one).
        let mut metadata = serde_json::Map::new();
        metadata.insert("runId".to_owned(), Value::String(chat.run_id.clone()));
        metadata.insert("agentId".to_owned(), Value::String(chat.agent_id.clone()));
        metadata.insert(
            "status".to_owned(),
            Value::String("PENDING_INPUT".to_owned()),
        );
        if let Some(request_id) = input_request.get("id").and_then(Value::as_str) {
            metadata.insert(
                "requestId".to_owned(),
                Value::String(request_id.to_owned()),
            );
        }
        if let Some(decision_kind) = call
            .arguments
            .get("decisionKind")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
        {
            metadata.insert(
                "decisionKind".to_owned(),
                Value::String(decision_kind.to_owned()),
            );
        }
        self.record_run_audit(&RunAuditEvent {
            id: format!("{}:audit:run.awaiting_input", chat.run_id),
            subject_id: chat.run_id.clone(),
            kind: "run.awaiting_input",
            detail: "Agent run is waiting for user input.",
            metadata: Value::Object(metadata),
        });

        Ok(AdkChatPortOutput::Json(json!({
            "reply": "我需要你确认几个选择，回答后会继续执行。",
            "session": session,
            "run": payload,
            "inputRequest": input_request,
            "pendingApprovals": [],
            "timeline": [],
        })))
    }

}
