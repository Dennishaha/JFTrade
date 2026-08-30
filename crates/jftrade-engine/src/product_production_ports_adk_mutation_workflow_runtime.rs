//! Durable workflow invocation and recovery helpers.

use std::sync::atomic::Ordering;

use jftrade_store_sqlite::AdkStore;
use serde_json::{Value, json};
use sha2::{Digest, Sha256};

use super::*;
use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortOutput, AdkChatRoute,
};

pub(super) fn run_workflow(
    port: &ProductionAdkPort,
    input: &AdkMutationInput,
) -> Result<Value, AdkMutationPortError> {
    let (workflow_id, trigger) = match input.operation {
        AdkMutationOperation::RunWorkflow => {
            let id = required_identifier(input, "workflowId")?;
            (id, None)
        }
        AdkMutationOperation::RunWorkflowTrigger | AdkMutationOperation::RunWorkflowWebhook => {
            let trigger_id = required_identifier(input, "triggerId")?;
            let trigger = port.store.get_workflow_trigger(&trigger_id).map_err(storage_mutation_failed)?.ok_or_else(|| not_found_mutation("ADK_WORKFLOW_TRIGGER_NOT_FOUND", "workflow trigger not found"))?;
            if input.operation == AdkMutationOperation::RunWorkflowWebhook {
                if !trigger.trigger_type.eq_ignore_ascii_case("webhook") {
                    return Err(not_found_mutation("ADK_WORKFLOW_WEBHOOK_NOT_FOUND", "workflow webhook not found"));
                }
                if !trigger.status.eq_ignore_ascii_case("ENABLED") {
                    return Err(invalid_mutation_input("workflow webhook is disabled"));
                }
                let secret = input.webhook_secret.as_deref().unwrap_or_default().trim();
                let trigger_payload = decode_mutation_payload(&trigger.payload_json, "workflow trigger")?;
                let expected = trigger_payload
                    .get("secretHash")
                    .and_then(Value::as_str)
                    .unwrap_or_default();
                let mut digest = Sha256::new();
                digest.update(secret.as_bytes());
                if secret.is_empty() || encode_hex(&digest.finalize()) != expected {
                    return Err(invalid_mutation_input("invalid workflow webhook secret"));
                }
            } else if !trigger.status.eq_ignore_ascii_case("ENABLED") {
                return Err(invalid_mutation_input("workflow trigger is disabled"));
            }
            (trigger.workflow_id.clone(), Some(trigger))
        }
        _ => unreachable!(),
    };
    let workflow = port.store.get_workflow(&workflow_id).map_err(storage_mutation_failed)?.ok_or_else(|| not_found_mutation("ADK_WORKFLOW_NOT_FOUND", "workflow not found"))?;
    if !workflow.status.eq_ignore_ascii_case("ENABLED") || is_deleted_payload(&workflow.payload_json)? {
        return Err(invalid_mutation_input("workflow is disabled"));
    }
    let workflow_value = decode_mutation_payload(&workflow.payload_json, "workflow")?;
    let mut inputs = workflow_value.get("defaultInputs").cloned().filter(Value::is_object).unwrap_or_else(|| json!({}));
    if let Some(object) = inputs.as_object_mut() {
        object.extend(input.body.as_object().cloned().unwrap_or_default());
    }
    let prompt = workflow_value.get("promptTemplate").and_then(Value::as_str).map(str::trim).filter(|value| !value.is_empty()).ok_or_else(|| invalid_mutation_input("workflow promptTemplate is required"))?;
    let message = render_template(prompt, &inputs);
    let sequence = WORKFLOW_ID_SEQUENCE.fetch_add(1, Ordering::Relaxed);
    let request_id = format!("workflow-{workflow_id}-{sequence}");
    let started_at = now_rfc3339();
    let log_id = generate_workflow_log_id()?;
    // Record the invocation before crossing the external model boundary. A
    // crash or provider timeout therefore leaves a durable RUNNING record that
    // startup recovery can fence and mark orphaned instead of losing the
    // invocation entirely.
    let trigger_id = trigger
        .as_ref()
        .map(|value| value.id.as_str())
        .unwrap_or_default();
    let trigger_type = trigger
        .as_ref()
        .map(|value| value.trigger_type.as_str())
        .unwrap_or("manual");
    let invocation = json!({
        "id": log_id.clone(),
        "workflowId": workflow_id,
        "triggerId": trigger_id,
        "triggerType": trigger_type,
        "status": "RUNNING",
        "runId": "",
        "sessionId": "",
        "inputs": inputs.clone(),
        "result": Value::Null,
        "startedAt": started_at.clone(),
    });
    let stored_invocation = port
        .store
        .create_workflow_trigger_log(
            &log_id,
            &workflow_id,
            trigger_id,
            trigger_type,
            "RUNNING",
            "",
            &invocation.to_string(),
        )
        .map_err(storage_mutation_failed)?;
    let invocation_revision = stored_invocation.updated_at.clone();
    let Some(runtime) = port.chat_runtime.as_deref() else {
        return Err(finalize_workflow_failure(
            port,
            &log_id,
            &invocation_revision,
            &invocation,
            "assistant model runtime is unavailable",
            "ADK_WORKFLOW_RUNTIME_UNAVAILABLE",
        ));
    };
    let body = json!({
        "clientRequestId": request_id,
        "sessionId": format!("workflow-session-{sequence}"),
        "agentId": workflow_value.get("agentId").and_then(Value::as_str).unwrap_or("jftrade-default"),
        "providerId": workflow_value.get("providerId").and_then(Value::as_str).unwrap_or_default(),
        "model": workflow_value.get("model").and_then(Value::as_str).unwrap_or_default(),
        "message": message,
        "objective": workflow_value.get("objectiveTemplate").and_then(Value::as_str).unwrap_or_default(),
    });
    let response = match serde_json::to_vec(&body)
        .map_err(|error| AdkMutationPortError::Failed {
            status: 500,
            code: "ADK_WORKFLOW_FAILED".to_owned(),
            message: error.to_string(),
        })
        .and_then(|body| {
            runtime
                .dispatch(
                    AdkChatRoute::Chat,
                    &AdkChatInput {
                        body,
                        client_request_id: request_id.clone(),
                    },
                )
                .map_err(|error| super::runtime::runtime_error(error, 503, "ADK_WORKFLOW_FAILED"))
        }) {
        Ok(response) => response,
        Err(error) => {
            let message = error.to_string();
            let code = match &error {
                AdkMutationPortError::Failed { code, .. } => code.as_str(),
                _ => "ADK_WORKFLOW_FAILED",
            };
            return Err(finalize_workflow_failure(
                port,
                &log_id,
                &invocation_revision,
                &invocation,
                &message,
                code,
            ));
        }
    };
    let response = match response {
        AdkChatPortOutput::Json(value) => value,
        AdkChatPortOutput::Stream(_) | AdkChatPortOutput::LiveStream(_) => {
            return Err(finalize_workflow_failure(
                port,
                &log_id,
                &invocation_revision,
                &invocation,
                "workflow runtime returned a stream",
                "ADK_WORKFLOW_FAILED",
            ));
        }
    };
    let run = match response.get("run").and_then(Value::as_object) {
        Some(run) => run,
        None => {
            return Err(finalize_workflow_failure(
                port,
                &log_id,
                &invocation_revision,
                &invocation,
                "workflow runtime response did not include a run",
                "ADK_WORKFLOW_FAILED",
            ));
        }
    };
    let run_id = match run
        .get("id")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
    {
        Some(value) => value,
        None => {
            return Err(finalize_workflow_failure(
                port,
                &log_id,
                &invocation_revision,
                &invocation,
                "workflow runtime response did not include a run id",
                "ADK_WORKFLOW_FAILED",
            ));
        }
    };
    let run_status = match run
        .get("status")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
    {
        Some(value) => value,
        None => {
            return Err(finalize_workflow_failure(
                port,
                &log_id,
                &invocation_revision,
                &invocation,
                "workflow runtime response did not include a run status",
                "ADK_WORKFLOW_FAILED",
            ));
        }
    };
    let status = match run_status.to_ascii_uppercase().as_str() {
        "COMPLETED" | "SUCCEEDED" => "SUCCEEDED",
        "PENDING" | "PENDING_APPROVAL" | "PENDING_INPUT" => "PENDING_APPROVAL",
        "RUNNING" => "RUNNING",
        "FAILED" => "FAILED",
        "CANCELLED" | "DENIED" | "TIMED_OUT" => "CANCELLED",
        _ => {
            let message = format!("workflow runtime returned unknown run status {run_status}");
            return Err(finalize_workflow_failure(
                port,
                &log_id,
                &invocation_revision,
                &invocation,
                &message,
                "ADK_WORKFLOW_FAILED",
            ));
        }
    };
    let mut log = json!({"id": log_id, "workflowId": workflow_id, "triggerId": trigger.as_ref().map(|value| value.id.clone()).unwrap_or_default(), "triggerType": trigger.as_ref().map(|value| value.trigger_type.clone()).unwrap_or_else(|| "manual".to_owned()), "status": status, "runId": run_id, "sessionId": response.get("session").and_then(|session| session.get("id")).and_then(Value::as_str).unwrap_or_default(), "inputs": inputs, "result": response.clone(), "startedAt": started_at});
    if !matches!(status, "RUNNING" | "PENDING_APPROVAL") {
        if let Some(object) = log.as_object_mut() {
            object.insert("finishedAt".to_owned(), Value::String(now_rfc3339()));
        }
    }
    let stored = match port
        .store
        .update_workflow_trigger_log_if_revision(
            &log_id,
            &invocation_revision,
            status,
            run_id,
            &log.to_string(),
        )
        .map_err(storage_mutation_failed)?
    {
        Some(stored) => stored,
        None => {
            // Another executor won the terminal CAS.  Re-read that durable
            // winner and return it instead of manufacturing a conflict from
            // a benign duplicate completion.
            let winner = port
                .store
                .get_workflow_trigger_log(&log_id)
                .map_err(storage_mutation_failed)?
                .ok_or_else(|| AdkMutationPortError::Failed {
                    status: 409,
                    code: "ADK_WORKFLOW_CONFLICT".to_owned(),
                    message: "workflow invocation disappeared before completion".to_owned(),
                })?;
            let mut winner_log = decode_mutation_payload(&winner.payload_json, "workflow trigger log")?;
            if let Some(object) = winner_log.as_object_mut() {
                object.insert("createdAt".to_owned(), Value::String(winner.created_at));
                object.insert("updatedAt".to_owned(), Value::String(winner.updated_at));
            }
            let winner_response = winner_log
                .get("result")
                .cloned()
                .unwrap_or(Value::Null);
            return Ok(json!({
                "workflow": workflow_payload(&workflow)?,
                "trigger": trigger.as_ref().map(workflow_trigger_payload).transpose()?,
                "log": winner_log,
                "response": winner_response,
            }));
        }
    };
    // Return the durable timestamps/status from SQLite instead of claiming a
    // successful invocation solely from the in-memory response projection.
    log = decode_mutation_payload(&stored.payload_json, "workflow trigger log")?;
    if let Some(object) = log.as_object_mut() {
        object.insert("createdAt".to_owned(), Value::String(stored.created_at));
        object.insert("updatedAt".to_owned(), Value::String(stored.updated_at));
    }
    Ok(json!({"workflow": workflow_payload(&workflow)?, "trigger": trigger.as_ref().map(workflow_trigger_payload).transpose()?, "log": log, "response": response}))
}

fn generate_workflow_log_id() -> Result<String, AdkMutationPortError> {
    let mut bytes = [0_u8; 32];
    getrandom::fill(&mut bytes).map_err(|error| AdkMutationPortError::Failed {
        status: 500,
        code: "ADK_WORKFLOW_LOG_ID_GENERATION_FAILED".to_owned(),
        message: error.to_string(),
    })?;
    Ok(format!("workflow-log-{}", encode_hex(&bytes)))
}

fn finalize_workflow_failure(
    port: &ProductionAdkPort,
    log_id: &str,
    expected_revision: &str,
    invocation: &Value,
    message: &str,
    code: &str,
) -> AdkMutationPortError {
    finalize_workflow_failure_with_store(
        port.store.as_ref(),
        log_id,
        expected_revision,
        invocation,
        message,
        code,
    )
}

fn finalize_workflow_failure_with_store(
    store: &AdkStore,
    log_id: &str,
    expected_revision: &str,
    invocation: &Value,
    message: &str,
    code: &str,
) -> AdkMutationPortError {
    let mut payload = invocation.clone();
    if let Some(object) = payload.as_object_mut() {
        object.insert("status".to_owned(), Value::String("FAILED".to_owned()));
        object.insert("errorCode".to_owned(), Value::String(code.to_owned()));
        object.insert("error".to_owned(), Value::String(message.to_owned()));
        object.insert("finishedAt".to_owned(), Value::String(now_rfc3339()));
    }
    let fallback = workflow_failure_error(code, message);
    match store.update_workflow_trigger_log_if_revision(
        log_id,
        expected_revision,
        "FAILED",
        "",
        &payload.to_string(),
    ) {
        Ok(Some(_)) => fallback,
        Ok(None) => match store.get_workflow_trigger_log(log_id) {
            Ok(Some(winner)) => workflow_failure_from_winner(&winner),
            Ok(None) => AdkMutationPortError::Failed {
                status: 409,
                code: "ADK_WORKFLOW_CONFLICT".to_owned(),
                message: "workflow invocation disappeared before failure could be persisted"
                    .to_owned(),
            },
            Err(error) => AdkMutationPortError::Failed {
                status: 500,
                code: "ADK_WORKFLOW_PERSIST_FAILED".to_owned(),
                message: format!(
                    "{message}; failed to read durable workflow winner: {error}"
                ),
            },
        },
        Err(error) => AdkMutationPortError::Failed {
            status: 500,
            code: "ADK_WORKFLOW_PERSIST_FAILED".to_owned(),
            message: format!("{message}; failed to persist terminal workflow state: {error}"),
        },
    }
}

fn workflow_failure_error(code: &str, message: &str) -> AdkMutationPortError {
    AdkMutationPortError::Failed {
        status: if code.to_ascii_uppercase().contains("UNAVAILABLE") {
            503
        } else {
            502
        },
        code: code.to_owned(),
        message: message.to_owned(),
    }
}

fn workflow_failure_from_winner(
    winner: &jftrade_store_sqlite::StoredAdkWorkflowTriggerLog,
) -> AdkMutationPortError {
    if winner.status.eq_ignore_ascii_case("FAILED") {
        let payload = match decode_mutation_payload(&winner.payload_json, "workflow trigger log") {
            Ok(payload) => payload,
            Err(error) => return error,
        };
        let code = payload
            .get("errorCode")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .unwrap_or("ADK_WORKFLOW_FAILED");
        let message = payload
            .get("error")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .unwrap_or("workflow invocation failed");
        return workflow_failure_error(code, message);
    }
    AdkMutationPortError::Failed {
        status: 409,
        code: "ADK_WORKFLOW_CONFLICT".to_owned(),
        message: format!(
            "workflow invocation was finalized by another executor ({})",
            winner.status
        ),
    }
}

fn render_template(template: &str, inputs: &Value) -> String {
    let mut output = template.to_owned();
    if let Some(object) = inputs.as_object() {
        for (key, value) in object {
            let rendered = value.as_str().map(str::to_owned).unwrap_or_else(|| value.to_string());
            output = output.replace(&format!("{{{{{key}}}}}"), &rendered);
        }
    }
    output
}

#[cfg(test)]
mod tests {
    use super::*;
    use jftrade_store_sqlite::initialize_current;
    use rusqlite::Connection;
    use std::fs::File;
    use std::time::Duration;
    use tempfile::tempdir;

    #[test]
    fn failure_cas_loser_returns_durable_failed_winner_without_overwriting_it() {
        let directory = tempdir().expect("temporary directory");
        let path = directory.path().join("adk.db");
        File::create(&path).expect("create ADK database");
        initialize_current(&Connection::open(&path).expect("initialize ADK database"), "adk")
            .expect("initialize ADK schema");
        let store = AdkStore::open(&path).expect("open ADK store");
        let initial = store
            .create_workflow_trigger_log(
                "workflow-log-1",
                "workflow-1",
                "",
                "manual",
                "RUNNING",
                "",
                r#"{"status":"RUNNING"}"#,
            )
            .expect("create invocation");
        std::thread::sleep(Duration::from_millis(2));
        let winner_payload =
            r#"{"status":"FAILED","errorCode":"ADK_WORKFLOW_RUNTIME_UNAVAILABLE","error":"winner failure"}"#;
        let winner = store
            .update_workflow_trigger_log_if_revision(
                &initial.id,
                &initial.updated_at,
                "FAILED",
                "",
                winner_payload,
            )
            .expect("persist winner")
            .expect("winner row");
        assert_ne!(winner.updated_at, initial.updated_at, "CAS revision must advance");

        let error = finalize_workflow_failure_with_store(
            &store,
            &initial.id,
            &initial.updated_at,
            &json!({"id":"workflow-log-1","status":"RUNNING"}),
            "loser failure",
            "ADK_WORKFLOW_FAILED",
        );
        assert_eq!(
            error,
            AdkMutationPortError::Failed {
                status: 503,
                code: "ADK_WORKFLOW_RUNTIME_UNAVAILABLE".to_owned(),
                message: "winner failure".to_owned(),
            }
        );
        let durable = store
            .get_workflow_trigger_log(&initial.id)
            .expect("read durable winner")
            .expect("winner remains present");
        assert_eq!(durable.status, "FAILED");
        assert_eq!(durable.payload_json, winner_payload);
    }

    #[test]
    fn failure_cas_loser_distinguishes_missing_durable_winner() {
        let directory = tempdir().expect("temporary directory");
        let path = directory.path().join("adk.db");
        File::create(&path).expect("create ADK database");
        initialize_current(&Connection::open(&path).expect("initialize ADK database"), "adk")
            .expect("initialize ADK schema");
        let store = AdkStore::open(&path).expect("open ADK store");
        let error = finalize_workflow_failure_with_store(
            &store,
            "missing-workflow-log",
            "stale-revision",
            &json!({"id":"missing-workflow-log","status":"RUNNING"}),
            "loser failure",
            "ADK_WORKFLOW_FAILED",
        );
        assert_eq!(
            error,
            AdkMutationPortError::Failed {
                status: 409,
                code: "ADK_WORKFLOW_CONFLICT".to_owned(),
                message: "workflow invocation disappeared before failure could be persisted"
                    .to_owned(),
            }
        );
    }
}
