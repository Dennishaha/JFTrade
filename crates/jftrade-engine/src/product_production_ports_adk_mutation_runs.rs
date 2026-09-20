//! Production ADK runs mutation dispatch.

use super::*;
use crate::product::product_adk_chat_stream_port::AdkChatPortError;

fn continuation_unavailable(error: AdkChatPortError) -> AdkMutationPortError {
    AdkMutationPortError::Failed {
        status: 503,
        code: "ADK_CONTINUATION_UNAVAILABLE".to_owned(),
        message: adk_port_error_message(error),
    }
}

fn adk_port_error_message(error: AdkChatPortError) -> String {
    match error {
        AdkChatPortError::Unavailable(message) | AdkChatPortError::Conflict(message) => message,
        AdkChatPortError::Failed { code, message, .. } => format!("{code}: {message}"),
    }
}

pub(super) fn handles(operation: AdkMutationOperation) -> bool {
    matches!(
        operation,
        AdkMutationOperation::Approve
            | AdkMutationOperation::Deny
            | AdkMutationOperation::CancelRun
            | AdkMutationOperation::PauseRun
            | AdkMutationOperation::ResumeRun
    )
}

pub(super) fn dispatch(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    debug_assert!(handles(input.operation));
    match input.operation {
        AdkMutationOperation::Approve => {
            let id = required_identifier(input, "approvalId")?;
            let resolution = port
                .store
                .resolve_and_stage_approval(&id, "APPROVED")
                .map_err(storage_mutation_failed)?;
            let Some(resolution) = resolution else {
                return Ok(json!({"approval": {"id": ""}}));
            };
            if resolution.should_continue {
                if let Some(runtime) = port.chat_runtime.as_deref() {
                    if let Err(error) = runtime.resume_approval(&resolution.approval.run_id) {
                        // The original runtime claims each continuation before
                        // starting it, so a concurrent claim (a browser retry,
                        // the recovery scanner or a duplicate approve) is a
                        // silent no-op there: the owner in flight rereads the
                        // resolved run and executes the released tool call.
                        // Rolling the staged resolution back here would answer
                        // `503 ADK_CONTINUATION_UNAVAILABLE` for a run that is
                        // in fact continuing, so only real start failures are
                        // compensated.
                        if !matches!(error, AdkChatPortError::Conflict(_)) {
                            rollback_staged_approval(port, &resolution)?;
                            return Err(continuation_unavailable(error));
                        }
                    }
                } else {
                    rollback_staged_approval(port, &resolution)?;
                    return Err(AdkMutationPortError::Failed {
                        status: 503,
                        code: "ADK_CONTINUATION_UNAVAILABLE".to_owned(),
                        message: "assistant approval continuation is unavailable".to_owned(),
                    });
                }
            }
            approval_resolution_value(&resolution)
        }
        AdkMutationOperation::Deny => {
            let id = required_identifier(input, "approvalId")?;
            let resolution = port
                .store
                .resolve_and_stage_approval(&id, "DENIED")
                .map_err(storage_mutation_failed)?;
            let Some(resolution) = resolution else {
                return Ok(json!({"approval": {"id": ""}}));
            };
            if resolution.should_continue {
                if let Some(runtime) = port.chat_runtime.as_deref() {
                    if let Err(error) = runtime.resume_approval(&resolution.approval.run_id) {
                        // See the approve branch: an already-claimed
                        // continuation is Go's idempotent wakeup, not a
                        // failure to start one.
                        if !matches!(error, AdkChatPortError::Conflict(_)) {
                            rollback_staged_approval(port, &resolution)?;
                            return Err(continuation_unavailable(error));
                        }
                    }
                } else {
                    rollback_staged_approval(port, &resolution)?;
                    return Err(AdkMutationPortError::Failed {
                        status: 503,
                        code: "ADK_CONTINUATION_UNAVAILABLE".to_owned(),
                        message: "assistant approval continuation is unavailable".to_owned(),
                    });
                }
            }
            approval_resolution_value(&resolution)
        }
        AdkMutationOperation::CancelRun => {
            let id = required_identifier(input, "runId")?;
            // Go's `Runtime.CancelRun` reconciles expired runs first, so a run
            // that already burned its budget is projected as TIMED_OUT rather
            // than being cancelled as if it were still live.
            if let Some(runtime) = port.chat_runtime.as_deref()
                && let Err(error) = runtime.reconcile_expired_runs()
            {
                return Err(AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_RUN_CANCEL_FAILED".to_owned(),
                    message: adk_port_error_message(error),
                });
            }
            // Wake an in-flight provider request before committing the durable
            // cancellation below.  The runtime's SQLite CAS remains the
            // authority if the request races a terminal completion.
            if let Some(runtime) = port.chat_runtime.as_deref() {
                runtime.cancel_run(&id);
            }
            // Go's `CancelRun` wraps every runtime error with the dedicated
            // cancel code, so a missing run answers `404 ADK_RUN_CANCEL_FAILED`
            // rather than the generic `NOT_FOUND` used by the read route.
            let Some(existing) = port.store.get_run(&id).map_err(storage_mutation_failed)? else {
                return Err(not_found_mutation("ADK_RUN_CANCEL_FAILED", "run not found"));
            };
            let status = existing.status.trim().to_ascii_uppercase();
            // Go's `Runtime.CancelRun` cancels RUNNING, PENDING,
            // PENDING_INPUT and PAUSED runs; every other status is a no-op
            // that returns the unchanged projection.
            if !matches!(
                status.as_str(),
                "RUNNING" | "PENDING" | "PENDING_APPROVAL" | "PENDING_INPUT" | "PAUSED"
            ) {
                return run_entity_value(&existing);
            }
            let mut value = decode_mutation_payload(&existing.payload_json, "run")?;
            let object = value
                .as_object_mut()
                .ok_or_else(|| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: "stored ADK run payload must be a JSON object".to_owned(),
                })?;
            let cancelled_at = now_rfc3339();
            object.insert("status".to_owned(), Value::String("CANCELLED".to_owned()));
            object.insert(
                "cancelledAt".to_owned(),
                Value::String(cancelled_at.clone()),
            );
            object.insert(
                "completedAt".to_owned(),
                Value::String(cancelled_at.clone()),
            );
            object.insert("message".to_owned(), Value::String("cancelled".to_owned()));
            object.insert(
                "failureReason".to_owned(),
                Value::String("run was cancelled by user".to_owned()),
            );
            object.insert(
                "errorCode".to_owned(),
                Value::String("RUN_CANCELLED".to_owned()),
            );
            object.insert("pendingApprovals".to_owned(), Value::Array(Vec::new()));
            if object
                .get("workMode")
                .and_then(Value::as_str)
                .is_some_and(|mode| !mode.eq_ignore_ascii_case("chat"))
                && object
                    .get("workflowStatus")
                    .and_then(Value::as_str)
                    .is_some_and(|workflow| !workflow.trim().is_empty())
            {
                object.insert(
                    "workflowStatus".to_owned(),
                    Value::String("FAILED".to_owned()),
                );
                if let Some(Value::Array(plan)) = object.get_mut("workflowPlan") {
                    for step in plan {
                        if let Value::Object(step) = step
                            && step
                                .get("status")
                                .and_then(Value::as_str)
                                .is_none_or(|step_status| step_status != "DONE")
                        {
                            step.insert("status".to_owned(), Value::String("BLOCKED".to_owned()));
                        }
                    }
                }
            }
            if let Some(Value::Object(request)) = object.get_mut("inputRequest")
                && request
                    .get("status")
                    .and_then(Value::as_str)
                    .is_some_and(|request_status| request_status == "PENDING")
            {
                request.insert("status".to_owned(), Value::String("CANCELLED".to_owned()));
                request.insert("updatedAt".to_owned(), Value::String(cancelled_at.clone()));
            }
            if let Some(Value::Array(input_requests)) = object.get_mut("inputRequests") {
                for request in input_requests {
                    if let Value::Object(request) = request
                        && request
                            .get("status")
                            .and_then(Value::as_str)
                            .is_some_and(|request_status| request_status == "PENDING")
                    {
                        request.insert("status".to_owned(), Value::String("CANCELLED".to_owned()));
                        request.insert("updatedAt".to_owned(), Value::String(cancelled_at.clone()));
                    }
                }
            }
            if let Some(Value::Array(tool_calls)) = object.get_mut("toolCalls") {
                for tool_call in tool_calls {
                    if let Value::Object(tool_call) = tool_call
                        && tool_call.get("status").and_then(Value::as_str).is_some_and(
                            |call_status| {
                                matches!(
                                    call_status,
                                    "RUNNING" | "PENDING" | "PENDING_APPROVAL" | "PENDING_INPUT"
                                )
                            },
                        )
                    {
                        tool_call
                            .insert("status".to_owned(), Value::String("CANCELLED".to_owned()));
                        tool_call.insert("requiresUser".to_owned(), Value::Bool(false));
                        tool_call.insert(
                            "completedAt".to_owned(),
                            Value::String(cancelled_at.clone()),
                        );
                    }
                }
            }
            cancel_run_and_deny_pending_approvals(
                port,
                &id,
                &status,
                &existing.updated_at,
                &value,
            )
        }
        AdkMutationOperation::PauseRun => {
            let id = required_identifier(input, "runId")?;
            let Some(existing) = port.store.get_run(&id).map_err(storage_mutation_failed)? else {
                return Err(not_found_mutation("NOT_FOUND", "run not found"));
            };
            let mut value = decode_mutation_payload(&existing.payload_json, "run")?;
            validate_goal_run(&value, "paused")?;
            let status = existing.status.trim().to_ascii_uppercase();
            let object = value
                .as_object_mut()
                .ok_or_else(|| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: "stored ADK run payload must be a JSON object".to_owned(),
                })?;
            if status == "PAUSED" {
                if object
                    .get("pausedReason")
                    .and_then(Value::as_str)
                    .is_some_and(|reason| reason == "user")
                {
                    return run_entity_value(&existing);
                }
                return Err(invalid_mutation_input(
                    "system-paused runs cannot be paused",
                ));
            }
            if terminal_run_status(&status) {
                return Err(invalid_mutation_input("terminal runs cannot be paused"));
            }
            if status != "RUNNING" {
                return Err(invalid_mutation_input(
                    "only running goal runs can be paused",
                ));
            }
            if object.get("pauseRequestedAt").is_none_or(Value::is_null) {
                object.insert("pauseRequestedAt".to_owned(), Value::String(now_rfc3339()));
            }
            object.insert(
                "resumeState".to_owned(),
                Value::String("user_pause_requested".to_owned()),
            );
            object.insert(
                "message".to_owned(),
                Value::String("目标将在当前轮结束后暂停。".to_owned()),
            );
            object.insert("status".to_owned(), Value::String(status.clone()));
            run_state_result(port, &id, &status, &existing.updated_at, &status, &value)
        }
        AdkMutationOperation::ResumeRun => {
            let id = required_identifier(input, "runId")?;
            let Some(existing) = port.store.get_run(&id).map_err(storage_mutation_failed)? else {
                return Err(not_found_mutation("NOT_FOUND", "run not found"));
            };
            let mut value = decode_mutation_payload(&existing.payload_json, "run")?;
            validate_goal_run(&value, "resumed")?;
            let status = existing.status.trim().to_ascii_uppercase();
            let object = value
                .as_object_mut()
                .ok_or_else(|| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_STORAGE_CORRUPT".to_owned(),
                    message: "stored ADK run payload must be a JSON object".to_owned(),
                })?;
            let paused_reason = object
                .get("pausedReason")
                .and_then(Value::as_str)
                .map(str::trim)
                .unwrap_or_default();
            let timed_out = status == "TIMED_OUT";
            if !timed_out
                && (status != "PAUSED"
                    || !matches!(
                        paused_reason,
                        "user" | "iteration_limit" | "self_reference_recovered"
                    ))
            {
                return Err(invalid_mutation_input(
                    "only resumable paused goal runs can be resumed",
                ));
            }
            let now = now_rfc3339();
            if timed_out {
                object.insert("startedAt".to_owned(), Value::String(now.clone()));
                object.remove("completedAt");
                // Go's `ResumeGoalRun` hands a timed-out goal a fresh
                // `RuntimeLimits().RunTimeout` window instead of reusing the
                // budget the previous attempt burned.
                object.insert(
                    "maxDurationMs".to_owned(),
                    json!(
                        crate::product::product_adk_run_timeout::assistant_run_timeout_ms(
                            &port.settings_path
                        )
                    ),
                );
            }
            object.insert("status".to_owned(), Value::String("RUNNING".to_owned()));
            object.insert(
                "workflowStatus".to_owned(),
                Value::String("RUNNING".to_owned()),
            );
            object.insert(
                "resumeState".to_owned(),
                Value::String("user_resuming".to_owned()),
            );
            object.insert(
                "message".to_owned(),
                Value::String("goal resumed".to_owned()),
            );
            object.remove("errorCode");
            object.remove("failureReason");
            object.remove("pauseRequestedAt");
            object.remove("pausedAt");
            object.remove("pausedReason");
            object.insert("degraded".to_owned(), Value::Bool(false));
            run_state_result(port, &id, &status, &existing.updated_at, "RUNNING", &value)
        }
        _ => unreachable!("operation group checked before dispatch"),
    }
}

fn rollback_staged_approval(
    port: &ProductionAdkPort,
    resolution: &jftrade_store_sqlite::AdkApprovalResolution,
) -> Result<(), AdkMutationPortError> {
    let Some(run) = resolution.run.as_ref() else {
        return Ok(());
    };
    port.store
        .rollback_staged_approval(
            &resolution.approval.id,
            &resolution.approval.run_id,
            &run.updated_at,
        )
        .map_err(storage_mutation_failed)?;
    Ok(())
}
