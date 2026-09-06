//! Production handlers for ADK operations that cross the model/runtime
//! boundary.  The durable SQLite projection remains authoritative; this
//! module only stages a state transition and delegates provider execution to
//! the runtime already attached to [`ProductionAdkPort`].

use std::fs;
use std::io::{Cursor, Read, Write};
use std::net::{IpAddr, SocketAddr, ToSocketAddrs};
use std::path::PathBuf;
use std::sync::atomic::Ordering;
use std::time::Duration;

use reqwest::Url;
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use zip::ZipArchive;

use super::*;
use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute,
};
use crate::product::product_adk_input_canonical::{
    CanonicalInputAnswers, InputResumeCheckpoint,
};

#[path = "product_production_ports_adk_mutation_skill_helpers.rs"]
mod skill_helpers;

use skill_helpers::{parsed_for_download_host, skill_frontmatter, unsafe_skill_ip};

pub(super) fn handles(operation: AdkMutationOperation) -> bool {
    matches!(
        operation,
        AdkMutationOperation::TestProvider
            | AdkMutationOperation::RespondToInput
            | AdkMutationOperation::CompactSessionContext
            | AdkMutationOperation::InstallSkill
            | AdkMutationOperation::RunWorkflowTrigger
            | AdkMutationOperation::RunWorkflowWebhook
            | AdkMutationOperation::RunWorkflow
    )
}

pub(super) fn dispatch(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    match input.operation {
        AdkMutationOperation::TestProvider => test_provider(port, input),
        AdkMutationOperation::RespondToInput => respond_to_input(port, input),
        AdkMutationOperation::CompactSessionContext => {
            super::context::compact_session_context(port, input)
        }
        AdkMutationOperation::InstallSkill => install_skill(port, input),
        AdkMutationOperation::RunWorkflowTrigger
        | AdkMutationOperation::RunWorkflowWebhook
        | AdkMutationOperation::RunWorkflow => super::workflow::run_workflow(port, input),
        _ => unreachable!("operation group checked before dispatch"),
    }
}

pub(super) fn runtime_error(
    error: AdkChatPortError,
    status: u16,
    code: &str,
) -> AdkMutationPortError {
    match error {
        AdkChatPortError::Unavailable(message) => AdkMutationPortError::Failed {
            status,
            code: code.to_owned(),
            message,
        },
        AdkChatPortError::Conflict(message) => AdkMutationPortError::Failed {
            status: 409,
            code: "ADK_RUN_CONFLICT".to_owned(),
            message,
        },
        AdkChatPortError::Failed {
            status,
            code,
            message,
        } => AdkMutationPortError::Failed {
            status,
            code,
            message,
        },
    }
}

fn test_provider(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    let id = required_identifier(input, "providerId")?;
    let provider = port
        .store
        .get_provider(&id)
        .map_err(storage_mutation_failed)?
        .ok_or_else(|| not_found_mutation("ADK_PROVIDER_NOT_FOUND", "provider not found"))?;
    let provider_value = decode_mutation_payload(&provider.payload_json, "provider")?;
    let mode = input
        .body
        .get("mode")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or("quick")
        .to_ascii_lowercase();
    if !matches!(mode.as_str(), "quick" | "full") {
        return Err(invalid_mutation_input(
            "provider test mode must be quick or full",
        ));
    }
    let Some(runtime) = port.chat_runtime.as_deref() else {
        return Err(AdkMutationPortError::Failed {
            status: 503,
            code: "ADK_PROVIDER_TEST_UNAVAILABLE".to_owned(),
            message: "assistant model runtime is unavailable".to_owned(),
        });
    };
    let sequence = SESSION_ID_SEQUENCE.fetch_add(1, Ordering::Relaxed);
    let request_id = format!("provider-test-{id}-{sequence}");
    let body = json!({
        "clientRequestId": request_id,
        "providerId": id,
        "agentId": "jftrade-default",
        "message": "Respond with a short connectivity check.",
        "model": provider_value.get("model").and_then(Value::as_str).unwrap_or_default(),
    });
    let output = runtime
        .dispatch(
            AdkChatRoute::Chat,
            &AdkChatInput {
                body: serde_json::to_vec(&body).map_err(|error| AdkMutationPortError::Failed {
                    status: 500,
                    code: "ADK_PROVIDER_TEST_FAILED".to_owned(),
                    message: error.to_string(),
                })?,
                client_request_id: request_id,
            },
        )
        .map_err(|error| runtime_error(error, 502, "ADK_PROVIDER_TEST_FAILED"))?;
    let response = match output {
        AdkChatPortOutput::Json(value) => value,
        AdkChatPortOutput::Stream(_) | AdkChatPortOutput::LiveStream(_) => {
            return Err(AdkMutationPortError::Failed {
                status: 502,
                code: "ADK_PROVIDER_TEST_FAILED".to_owned(),
                message: "provider test returned a stream instead of a response".to_owned(),
            });
        }
    };
    let reply = response
        .get("reply")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_owned();
    let reasoning_config = provider_value
        .get("reasoningConfig")
        .cloned()
        .unwrap_or_else(|| json!({}));
    let request_field = reasoning_config
        .get("requestField")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let results = reasoning_config
        .get("mappings")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default()
        .into_iter()
        .filter_map(|mapping| {
            let effort = mapping.get("effort")?.as_str()?.to_owned();
            let value = mapping.get("value")?.as_str()?.to_owned();
            Some(json!({"effort": effort, "value": value, "ok": true}))
        })
        .collect::<Vec<_>>();
    Ok(json!({
        "ok": true,
        "reply": reply,
        "capabilities": provider_value.get("capabilities").cloned().unwrap_or_else(|| json!({"chat": true})),
        "reasoning": {
            "mode": mode,
            "requestField": request_field,
            "ok": true,
            "results": results,
        },
        "checkedAt": now_rfc3339(),
    }))
}

fn respond_to_input(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    let run_id = required_identifier(input, "runId")?;
    let request_id = input
        .body
        .get("requestId")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| input_response_invalid("requestId is required"))?;
    let answers = input
        .body
        .get("answers")
        .and_then(Value::as_array)
        .ok_or_else(|| input_response_invalid("answers must be an array"))?;

    let Some(existing) = port
        .store
        .get_run(&run_id)
        .map_err(storage_mutation_failed)?
    else {
        return Err(not_found_mutation("ADK_RUN_NOT_FOUND", "run not found"));
    };
    let mut payload = decode_mutation_payload(&existing.payload_json, "run")?;
    let object = payload
        .as_object_mut()
        .ok_or_else(|| invalid_mutation_input("stored ADK run payload must be an object"))?;
    let mut request = object
        .get("inputRequest")
        .cloned()
        .filter(|request| request.get("id").and_then(Value::as_str) == Some(request_id));
    if request.is_none() {
        request = object
            .get("inputRequests")
            .and_then(Value::as_array)
            .and_then(|requests| {
                requests
                    .iter()
                    .find(|candidate| {
                        candidate.get("id").and_then(Value::as_str) == Some(request_id)
                    })
                    .cloned()
            });
    }
    let mut request = request.ok_or_else(|| {
        not_found_mutation("ADK_INPUT_REQUEST_NOT_FOUND", "input request not found")
    })?;

    let canonical_answers = validate_input_answers(&request, answers)?;

    let current_status = request
        .get("status")
        .and_then(Value::as_str)
        .unwrap_or("PENDING");
    let resume_state = object.get("resumeState").and_then(Value::as_str).unwrap_or("");
    if current_status.eq_ignore_ascii_case("ANSWERED") || resume_state == "input_resume_pending" {
        let existing_answers = request
            .get("answers")
            .and_then(Value::as_array)
            .map(|a| a.as_slice())
            .unwrap_or(&[]);
        if input_answers_equal(existing_answers, &canonical_answers) {
            if resume_state == "input_resume_pending"
                && let Some(runtime) = port.chat_runtime.as_deref()
            {
                runtime
                    .resume_approval(&run_id)
                    .map_err(|e| runtime_error(e, 503, "ADK_CONTINUATION_UNAVAILABLE"))?;
            }
            return Ok(json!({"request": request, "run": run_entity_value(&existing)?}));
        }
        return Err(input_response_conflict("run already has a different answer"));
    }
    if !current_status.eq_ignore_ascii_case("PENDING") {
        return Err(input_response_conflict("request is no longer pending"));
    }
    let Some(runtime) = port.chat_runtime.as_deref() else {
        return Err(AdkMutationPortError::Failed {
            status: 503,
            code: "ADK_CONTINUATION_UNAVAILABLE".to_owned(),
            message: "assistant input continuation is unavailable".to_owned(),
        });
    };
    let now = now_rfc3339();
    if let Some(request_object) = request.as_object_mut() {
        request_object.insert("status".to_owned(), Value::String("ANSWERED".to_owned()));
        request_object.insert("answers".to_owned(), Value::Array(canonical_answers.clone()));
        request_object.insert("answeredAt".to_owned(), Value::String(now.clone()));
        request_object.insert("updatedAt".to_owned(), Value::String(now.clone()));
    }
    if object
        .get("inputRequest")
        .and_then(Value::as_object)
        .is_some_and(|candidate| candidate.get("id").and_then(Value::as_str) == Some(request_id))
    {
        object.insert("inputRequest".to_owned(), request.clone());
    }
    if let Some(Value::Array(requests)) = object.get_mut("inputRequests") {
        for candidate in requests {
            if candidate.get("id").and_then(Value::as_str) == Some(request_id) {
                *candidate = request.clone();
            }
        }
    }
    let original_request = object
        .get("requestMessage")
        .and_then(Value::as_str)
        .map(str::trim)
        .unwrap_or_default();
    const INPUT_CONTINUATION_INSTRUCTION: &str =
        "用户已回答以上问题。回答只是解除阻塞，不代表原始请求已完成：必须基于回答继续完成 originalRequest 中的原始请求。安全、只读的下一步直接执行；需要写操作时调用相应工具并走审批流程。不得只总结、复述计划或询问是否继续后就结束运行。";

    let questions_list = request.get("questions").and_then(Value::as_array);
    let enriched_answers: Vec<Value> = canonical_answers
        .iter()
        .map(|answer| {
            let mut enriched = answer.clone();
            let Some(obj) = enriched.as_object_mut() else {
                return enriched;
            };
            let q_id = obj.get("questionId").and_then(Value::as_str).unwrap_or_default();
            let Some(questions) = questions_list else {
                return enriched;
            };
            let Some(q) = questions.iter().find(|q| q.get("id").and_then(Value::as_str) == Some(q_id)) else {
                return enriched;
            };
            if let Some(q_text) = q.get("question").and_then(Value::as_str) {
                obj.insert("question".to_owned(), Value::String(q_text.to_owned()));
            }
            let Some(opt_id) = obj.get("optionId").and_then(Value::as_str) else {
                return enriched;
            };
            let Some(opts) = q.get("options").and_then(Value::as_array) else {
                return enriched;
            };
            if let Some(opt) = opts.iter().find(|o| o.get("id").and_then(Value::as_str) == Some(opt_id))
                && let Some(label) = opt.get("label").and_then(Value::as_str)
            {
                obj.insert("answer".to_owned(), Value::String(label.to_owned()));
            }
            enriched
        })
        .collect();

    let input_response_payload = json!({
        "requestId": request_id,
        "answers": enriched_answers,
        "originalRequest": original_request,
        "continuationInstruction": INPUT_CONTINUATION_INSTRUCTION,
    });
    object.insert(
        "inputResponse".to_owned(),
        input_response_payload.clone(),
    );

    let target_call_id = request
        .get("functionCallId")
        .or_else(|| request.get("callId"))
        .and_then(Value::as_str)
        .unwrap_or_default();

    if let Some(Value::Array(tool_calls)) = object.get_mut("toolCalls") {
        for tool_call in tool_calls {
            let matches = if !target_call_id.is_empty() {
                tool_call.get("id").and_then(Value::as_str) == Some(target_call_id)
                    || tool_call.get("functionCallId").and_then(Value::as_str) == Some(target_call_id)
            } else {
                tool_call.get("name").and_then(Value::as_str) == Some("interaction.request_user")
                    && tool_call.get("status").and_then(Value::as_str) == Some("PENDING_INPUT")
            };
            if matches
                && let Some(call_obj) = tool_call.as_object_mut()
            {
                call_obj.insert("status".to_owned(), Value::String("COMPLETED".to_owned()));
                call_obj.insert("updatedAt".to_owned(), Value::String(now.clone()));
            }
        }
    }

    let tool_results = object
        .entry("toolResults".to_owned())
        .or_insert_with(|| Value::Array(Vec::new()))
        .as_array_mut();
    if let Some(results) = tool_results
        && !results.iter().any(|r| {
            r.get("callId").and_then(Value::as_str) == Some(target_call_id)
                && !target_call_id.is_empty()
        })
    {
        results.push(json!({
            "runId": run_id,
            "callId": target_call_id,
            "functionCallId": target_call_id,
            "name": "interaction.request_user",
            "toolName": "interaction.request_user",
            "status": "COMPLETED",
            "output": input_response_payload,
            "createdAt": now.clone(),
            "updatedAt": now.clone(),
        }));
    }

    let checkpoint = InputResumeCheckpoint {
        request_id: request_id.to_owned(),
        answers: CanonicalInputAnswers::from_values(&canonical_answers).answers,
        tool_results: object
            .get("toolResults")
            .cloned()
            .unwrap_or_else(|| Value::Array(Vec::new())),
        resume_state: "input_resuming".to_owned(),
        checkpoint_time: now.clone(),
    };
    object.insert(
        "inputResumeCheckpoint".to_owned(),
        serde_json::to_value(&checkpoint).unwrap_or(Value::Null),
    );

    object.insert("status".to_owned(), Value::String("RUNNING".to_owned()));
    object.insert(
        "resumeState".to_owned(),
        Value::String("input_resuming".to_owned()),
    );
    object.insert("updatedAt".to_owned(), Value::String(now));
    let run_json = serde_json::to_string(object).map_err(storage_mutation_failed)?;
    if !port
        .store
        .update_run_state_if_status_and_revision(
            &run_id,
            "PENDING_INPUT",
            &existing.updated_at,
            "RUNNING",
            &run_json,
        )
        .map_err(storage_mutation_failed)?
    {
        let current = port
            .store
            .get_run(&run_id)
            .map_err(storage_mutation_failed)?
            .ok_or_else(|| not_found_mutation("ADK_RUN_NOT_FOUND", "run not found"))?;
        let current_payload: Value = serde_json::from_str(&current.payload_json)
            .map_err(|e| storage_mutation_failed(format!("failed to parse current run payload: {e}")))?;
        if let Some(winner_resp) = current_payload.get("inputResponse") {
            let empty_answers = Vec::new();
            let winner_answers = winner_resp
                .get("answers")
                .and_then(Value::as_array)
                .unwrap_or(&empty_answers);
            if input_answers_equal(winner_answers, &canonical_answers) {
                return Ok(json!({"request": request, "run": run_entity_value(&current)?}));
            }
            return Err(input_response_conflict(
                "input response has already been submitted with different answers",
            ));
        }
        return Err(AdkMutationPortError::Failed {
            status: 409,
            code: "ADK_RUN_CONFLICT".to_owned(),
            message: "concurrent modification detected for run state".to_owned(),
        });
    }
    match runtime.resume_approval(&run_id) {
        Ok(()) | Err(AdkChatPortError::Conflict(_)) => {}
        Err(error) => {
            // Keep the accepted input response and checkpoint, but mark the state as
            // input_resume_pending so it can be retried or recovered by the startup recovery loop.
            object.insert("status".to_owned(), Value::String("RUNNING".to_owned()));
            object.insert(
                "resumeState".to_owned(),
                Value::String("input_resume_pending".to_owned()),
            );
            let pending_payload = serde_json::to_string(object).map_err(storage_mutation_failed)?;
            let current = port
                .store
                .get_run(&run_id)
                .map_err(storage_mutation_failed)?
                .ok_or_else(|| not_found_mutation("ADK_RUN_NOT_FOUND", "run not found"))?;
            if !port
                .store
                .update_run_state_if_status_and_revision(
                    &run_id,
                    "RUNNING",
                    &current.updated_at,
                    "RUNNING",
                    &pending_payload,
                )
                .map_err(storage_mutation_failed)?
            {
                tracing::error!(
                    run_id = %run_id,
                    "failed to transition run to input_resume_pending after continuation spawn failure"
                );
                return Err(AdkMutationPortError::Failed {
                    status: 409,
                    code: "ADK_RUN_CONFLICT".to_owned(),
                    message: "concurrent state modification while persisting resume checkpoint".to_owned(),
                });
            }
            return Err(runtime_error(error, 503, "ADK_CONTINUATION_UNAVAILABLE"));
        }
    }
    let updated = port
        .store
        .get_run(&run_id)
        .map_err(storage_mutation_failed)?
        .ok_or_else(|| not_found_mutation("ADK_RUN_NOT_FOUND", "run not found"))?;
    Ok(json!({"request": request, "run": run_entity_value(&updated)?}))
}

fn validate_input_answers(
    request: &Value,
    submitted: &[Value],
) -> Result<Vec<Value>, AdkMutationPortError> {
    let empty_vec = Vec::new();
    let questions = request
        .get("questions")
        .and_then(Value::as_array)
        .unwrap_or(&empty_vec);

    if submitted.len() != questions.len() {
        return Err(input_response_invalid(format!(
            "submitted {} answers but request has {} questions",
            submitted.len(),
            questions.len()
        )));
    }

    if questions.is_empty() {
        return Ok(Vec::new());
    }

    let mut by_question = std::collections::BTreeMap::new();
    for answer in submitted {
        if !answer.is_object() {
            return Err(input_response_invalid("answer must be an object"));
        }
        let question_id = answer
            .get("questionId")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|q| !q.is_empty())
            .ok_or_else(|| input_response_invalid("questionId is required"))?;

        if by_question.contains_key(question_id) {
            return Err(input_response_invalid(format!(
                "duplicate answer for {question_id}"
            )));
        }

        let option_id = answer
            .get("optionId")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|s| !s.is_empty());
        let other_text = answer
            .get("otherText")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|s| !s.is_empty());

        by_question.insert(question_id, (option_id, other_text));
    }

    let mut canonical = Vec::with_capacity(submitted.len());
    for question in questions {
        let question_id = question
            .get("id")
            .and_then(Value::as_str)
            .map(str::trim)
            .unwrap_or_default();
        if question_id.is_empty() {
            continue;
        }

        let Some((option_id, other_text)) = by_question.get(question_id).copied() else {
            return Err(input_response_invalid(format!(
                "missing answer for {question_id}"
            )));
        };

        let uses_option = option_id.is_some();
        let uses_other = other_text.is_some();
        if uses_option == uses_other {
            return Err(input_response_invalid(format!(
                "{question_id} must use exactly one answer type"
            )));
        }

        let allow_other = question
            .get("allowOther")
            .and_then(Value::as_bool)
            .unwrap_or(false);

        if let Some(other) = other_text {
            if !allow_other {
                return Err(input_response_invalid(format!(
                    "{question_id} does not allow other text"
                )));
            }
            canonical.push(json!({
                "questionId": question_id,
                "otherText": other,
            }));
            continue;
        }

        let selected_option = option_id.unwrap();
        let valid_option = question
            .get("options")
            .and_then(Value::as_array)
            .map(|opts| {
                opts.iter().any(|opt| {
                    opt.get("id").and_then(Value::as_str) == Some(selected_option)
                })
            })
            .unwrap_or(false);

        if !valid_option {
            return Err(input_response_invalid(format!(
                "invalid option for {question_id}"
            )));
        }

        canonical.push(json!({
            "questionId": question_id,
            "optionId": selected_option,
        }));
    }

    Ok(canonical)
}

fn input_answers_equal(left: &[Value], right: &[Value]) -> bool {
    let l = CanonicalInputAnswers::from_values(left);
    let r = CanonicalInputAnswers::from_values(right);
    l.matches(&r)
}

include!("product_production_ports_adk_mutation_skill.rs");
