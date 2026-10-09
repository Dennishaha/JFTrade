//! Canvas Workflow DAG execution engine.

use std::collections::BTreeMap;

use jftrade_assistant::{CanvasCompiler, WorkflowCanvasGraph, WorkflowNodeRun};
use serde_json::{Value, json};

use super::*;
use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortOutput, AdkChatRoute, AdkChatStreamPort,
};

#[derive(Clone, Debug)]
pub(super) struct WorkflowExecutionOutcome {
    pub(super) status: String,
    pub(super) run_id: String,
    pub(super) session_id: String,
    pub(super) response: Value,
    pub(super) node_runs: Vec<WorkflowNodeRun>,
}

pub(super) fn render_canvas_template(
    template: &str,
    inputs: &Value,
    node_outputs: &BTreeMap<String, Value>,
) -> String {
    let mut output = String::with_capacity(template.len());
    let mut remaining = template;
    // Only scan the submitted template. Inserted values remain data even when
    // they contain another placeholder or a node-output reference.
    while let Some(start) = remaining.find("{{") {
        output.push_str(&remaining[..start]);
        remaining = &remaining[start..];
        let Some(end) = remaining[2..].find("}}") else {
            break;
        };
        let end = end + 2;
        let key = remaining[2..end].trim();
        match canvas_template_value(key, inputs, node_outputs) {
            Some(value) => output.push_str(&value),
            None => output.push_str(&remaining[..end + 2]),
        }
        remaining = &remaining[end + 2..];
    }
    output.push_str(remaining);
    output
}

fn canvas_template_value(
    key: &str,
    inputs: &Value,
    node_outputs: &BTreeMap<String, Value>,
) -> Option<String> {
    let input_key = key
        .strip_prefix("input.")
        .or_else(|| key.strip_prefix("inputs."))
        .or_else(|| key.strip_prefix('.'))
        .unwrap_or(key);
    let value = inputs.get(input_key).or_else(|| {
        node_outputs.iter().find_map(|(node_id, output)| {
            if key == node_id {
                return output.get("reply").filter(|value| value.is_string());
            }
            key.strip_prefix(node_id)
                .and_then(|suffix| suffix.strip_prefix('.'))
                .and_then(|field| output.get(field))
        })
    })?;
    Some(value.as_str().map(str::to_owned).unwrap_or_else(|| value.to_string()))
}

pub(super) struct CanvasExecutionContext<'a> {
    pub runtime: &'a dyn AdkChatStreamPort,
    pub workflow_id: &'a str,
    pub workflow_value: &'a Value,
    pub trigger: Option<&'a jftrade_store_sqlite::StoredAdkWorkflowTrigger>,
    pub inputs: &'a Value,
    pub graph: &'a WorkflowCanvasGraph,
    pub topological_order: &'a [String],
    pub invocation_uuid: &'a str,
    pub checkpoint: &'a crate::product_workflow_checkpoint::WorkflowCheckpoint<'a>,
}

pub(super) fn execute_canvas_workflow(
    ctx: &CanvasExecutionContext<'_>,
) -> Result<WorkflowExecutionOutcome, (String, String)> {
    let compiler = CanvasCompiler::new(ctx.graph)
        .map_err(|e| (e.to_string(), "ADK_WORKFLOW_FAILED".to_owned()))?;
    let mut node_outputs: BTreeMap<String, Value> = BTreeMap::new();
    let mut node_runs = ctx.checkpoint.node_runs()?;
    for node in &node_runs {
        if let Some(outputs) = &node.outputs { node_outputs.insert(node.node_id.clone(), outputs.clone()); }
    }
    let previous = ctx.checkpoint.payload()?;
    let mut last_run_id = previous["runId"].as_str().unwrap_or_default().to_owned();
    let mut last_session_id = previous["sessionId"].as_str().unwrap_or_default().to_owned();
    let mut last_response = previous["result"].clone();
    let mut last_reply = String::new();
    let mut overall_status = "SUCCEEDED".to_owned();
    let mut execution_error: Option<String> = None;

    for node_id in ctx.topological_order {
        ctx.checkpoint.save(&node_runs, "RUNNING", &last_run_id, &last_session_id, &last_response)?;
        if let Some(previous) = node_runs.iter().find(|node| node.node_id == *node_id)
            && matches!(previous.status.as_str(), "FAILED" | "CANCELLED" | "SKIPPED") {
            overall_status = "FAILED".to_owned();
            continue;
        }
        if node_runs.iter().any(|node| node.node_id == *node_id && node.status == "SUCCEEDED") {
            continue;
        }
        let previous_node = node_runs.iter().find(|node| node.node_id == *node_id).cloned();
        node_runs.retain(|node| node.node_id != *node_id);
        let node = compiler.node(node_id).expect("node must exist");
        let node_type = compiler.node_type(node_id).unwrap_or("agent");
        let node_started = now_rfc3339();

        match node_type {
            "trigger" => {
                let matched_event = ctx.inputs.get("event").cloned().unwrap_or_else(|| json!({}));
                let out = json!({
                    "inputs": ctx.inputs.clone(),
                    "matchedEvent": matched_event,
                    "triggerId": ctx.trigger.as_ref().map(|t| t.id.clone()),
                });
                node_outputs.insert(node_id.clone(), out.clone());
                node_runs.push(WorkflowNodeRun {
                    node_id: node_id.clone(),
                    node_type: "trigger".to_owned(),
                    title: Some(node.title()),
                    status: "SUCCEEDED".to_owned(),
                    started_at: Some(node_started),
                    finished_at: Some(now_rfc3339()),
                    inputs: Some(ctx.inputs.clone()),
                    outputs: Some(out),
                    error: None,
                });
            }
            "start" => {
                let objective = ctx
                    .workflow_value
                    .get("objectiveTemplate")
                    .and_then(Value::as_str)
                    .unwrap_or_default();
                let out = json!({
                    "inputs": ctx.inputs.clone(),
                    "objective": objective,
                });
                node_outputs.insert(node_id.clone(), out.clone());
                node_runs.push(WorkflowNodeRun {
                    node_id: node_id.clone(),
                    node_type: "start".to_owned(),
                    title: Some(node.title()),
                    status: "SUCCEEDED".to_owned(),
                    started_at: Some(node_started),
                    finished_at: Some(now_rfc3339()),
                    inputs: Some(ctx.inputs.clone()),
                    outputs: Some(out),
                    error: None,
                });
            }
            "agent" => {
                let has_failed_predecessor = compiler.incoming_edges(node_id).iter().any(|pred_id| {
                    node_runs.iter().any(|r| &r.node_id == pred_id && (r.status == "FAILED" || r.status == "SKIPPED"))
                });

                if has_failed_predecessor || overall_status == "FAILED" {
                    node_runs.push(WorkflowNodeRun {
                        node_id: node_id.clone(),
                        node_type: "agent".to_owned(),
                        title: Some(node.title()),
                        status: "SKIPPED".to_owned(),
                        started_at: Some(node_started),
                        finished_at: Some(now_rfc3339()),
                        inputs: None,
                        outputs: None,
                        error: Some("skipped due to predecessor failure".to_owned()),
                    });
                    continue;
                }

                let template = node
                    .get_data_str("promptTemplate")
                    .or_else(|| node.get_data_str("message"))
                    .or_else(|| ctx.workflow_value.get("promptTemplate").and_then(Value::as_str))
                    .unwrap_or_default();
                let mut message = render_canvas_template(template, ctx.inputs, &node_outputs);
                if message.trim().is_empty() {
                    message = node.title();
                }

                let agent_id = node
                    .get_data_str("agentId")
                    .or_else(|| ctx.workflow_value.get("agentId").and_then(Value::as_str))
                    .unwrap_or("jftrade-default");
                let provider_id = node
                    .get_data_str("providerId")
                    .or_else(|| ctx.workflow_value.get("providerId").and_then(Value::as_str))
                    .unwrap_or_default();
                let model = node
                    .get_data_str("model")
                    .or_else(|| ctx.workflow_value.get("model").and_then(Value::as_str))
                    .unwrap_or_default();
                let objective = node
                    .get_data_str("objective")
                    .or_else(|| node.get_data_str("objectiveTemplate"))
                    .or_else(|| ctx.workflow_value.get("objectiveTemplate").and_then(Value::as_str))
                    .unwrap_or_default();

                let request_id = format!("workflow-{}-{node_id}-{}", ctx.workflow_id, ctx.invocation_uuid);
                let session_id = format!("workflow-session-{node_id}-{}", ctx.invocation_uuid);
                let body = workflow_chat_request(previous_node.as_ref(), &session_id, json!({
                    "clientRequestId": request_id,
                    "agentId": agent_id,
                    "providerId": provider_id,
                    "model": model,
                    "message": message,
                    "objective": objective,
                }));

                // Save the stable request identity before the external boundary.
                // Replays use that same clientRequestId after process restart.
                node_runs.push(WorkflowNodeRun {
                    node_id: node_id.clone(), node_type: "agent".to_owned(),
                    title: Some(node.title()), status: "RUNNING".to_owned(),
                    started_at: Some(node_started.clone()), finished_at: None,
                    inputs: Some(body.clone()), outputs: None, error: None,
                });
                ctx.checkpoint.save(&node_runs, "RUNNING", &last_run_id, &last_session_id, &last_response)?;
                node_runs.pop();
                let dispatch_result = serde_json::to_vec(&body)
                    .map_err(|e| (e.to_string(), "ADK_WORKFLOW_FAILED".to_owned()))
                    .and_then(|body_bytes| {
                        ctx.runtime
                            .dispatch(
                                AdkChatRoute::Chat,
                                &AdkChatInput {
                                    body: body_bytes,
                                    client_request_id: request_id.clone(),
                                },
                            )
                            .map_err(|e| {
                                let err = super::runtime::runtime_error(e, 503, "ADK_WORKFLOW_FAILED");
                                let code = match &err {
                                    AdkMutationPortError::Failed { code, .. } => code.clone(),
                                    _ => "ADK_WORKFLOW_FAILED".to_owned(),
                                };
                                (err.to_string(), code)
                            })
                    });

                let response_val = match dispatch_result {
                    Ok(AdkChatPortOutput::Json(val)) => val,
                    Ok(_) => {
                        let err_msg = "workflow runtime returned a stream".to_owned();
                        overall_status = "FAILED".to_owned();
                        execution_error = Some(err_msg.clone());
                        node_runs.push(WorkflowNodeRun {
                            node_id: node_id.clone(),
                            node_type: "agent".to_owned(),
                            title: Some(node.title()),
                            status: "FAILED".to_owned(),
                            started_at: Some(node_started),
                            finished_at: Some(now_rfc3339()),
                            inputs: Some(json!({"message": message})),
                            outputs: None,
                            error: Some(err_msg),
                        });
                        continue;
                    }
                    Err((err_msg, _)) => {
                        overall_status = "FAILED".to_owned();
                        execution_error = Some(err_msg.clone());
                        node_runs.push(WorkflowNodeRun {
                            node_id: node_id.clone(),
                            node_type: "agent".to_owned(),
                            title: Some(node.title()),
                            status: "FAILED".to_owned(),
                            started_at: Some(node_started),
                            finished_at: Some(now_rfc3339()),
                            inputs: Some(json!({"message": message})),
                            outputs: None,
                            error: Some(err_msg),
                        });
                        continue;
                    }
                };

                let run_id = response_val
                    .get("run")
                    .and_then(|r| r.get("id"))
                    .and_then(Value::as_str)
                    .unwrap_or_default()
                    .to_owned();
                if run_id.is_empty() {
                    return Err(("workflow model response omitted run id".to_owned(), "ADK_WORKFLOW_FAILED".to_owned()));
                }
                let run_status = response_val
                    .get("run")
                    .and_then(|r| r.get("status"))
                    .and_then(Value::as_str)
                    .unwrap_or("UNKNOWN")
                    .to_ascii_uppercase();
                let reply = response_val
                    .get("reply")
                    .and_then(Value::as_str)
                    .or_else(|| response_val.get("message").and_then(Value::as_str))
                    .or_else(|| response_val.get("run").and_then(|r| r.get("outputSummary")).and_then(Value::as_str))
                    .unwrap_or_default()
                    .to_owned();
                let node_session = response_val
                    .get("session")
                    .and_then(|s| s.get("id"))
                    .and_then(Value::as_str)
                    .unwrap_or(&session_id)
                    .to_owned();

                let node_status = match run_status.as_str() {
                    "COMPLETED" | "SUCCEEDED" => "SUCCEEDED",
                    "PENDING" | "PENDING_APPROVAL" | "PENDING_INPUT" => "PENDING_APPROVAL",
                    "RUNNING" => "RUNNING",
                    "FAILED" => "FAILED",
                    "CANCELLED" | "DENIED" | "TIMED_OUT" => "CANCELLED",
                    _ => "FAILED",
                };

                if node_status == "PENDING_APPROVAL" || node_status == "RUNNING" {
                    overall_status = node_status.to_owned();
                    execution_error = None;
                } else if node_status != "SUCCEEDED" {
                    overall_status = "FAILED".to_owned();
                    execution_error = Some(format!("node {node_id} returned run status {run_status}"));
                }

                last_run_id = run_id.clone();
                last_session_id = node_session.clone();
                last_reply = reply.clone();
                last_response = response_val.clone();

                let agent_outputs = json!({
                    "runId": run_id,
                    "sessionId": node_session,
                    "reply": reply,
                    "status": node_status,
                });
                node_outputs.insert(node_id.clone(), agent_outputs.clone());

                node_runs.push(WorkflowNodeRun {
                    node_id: node_id.clone(),
                    node_type: "agent".to_owned(),
                    title: Some(node.title()),
                    status: node_status.to_owned(),
                    started_at: Some(node_started),
                    finished_at: Some(now_rfc3339()),
                    inputs: Some(json!({
                        "message": message,
                        "agentId": agent_id,
                        "providerId": provider_id,
                        "model": model,
                    })),
                    outputs: Some(agent_outputs),
                    error: if node_status == "FAILED" { execution_error.clone() } else { None },
                });
                if node_status == "PENDING_APPROVAL" || node_status == "RUNNING" {
                    if let Some(node) = node_runs.last_mut() { node.finished_at = None; }
                    ctx.checkpoint.save(&node_runs, &overall_status, &last_run_id, &last_session_id, &last_response)?;
                    // A non-terminal model run owns the continuation. Do not
                    // execute downstream nodes against a provisional output.
                    break;
                }
            }
            "monitor" => {
                let mut predecessor_outputs = json!({});
                if let Some(obj) = predecessor_outputs.as_object_mut() {
                    for pred in compiler.incoming_edges(node_id) {
                        if let Some(out) = node_outputs.get(pred) {
                            obj.insert(pred.clone(), out.clone());
                        }
                    }
                }
                let monitor_outputs = json!({
                    "status": overall_status,
                    "lastReply": last_reply,
                    "predecessors": predecessor_outputs,
                });
                node_outputs.insert(node_id.clone(), monitor_outputs.clone());
                node_runs.push(WorkflowNodeRun {
                    node_id: node_id.clone(),
                    node_type: "monitor".to_owned(),
                    title: Some(node.title()),
                    status: overall_status.clone(),
                    started_at: Some(node_started),
                    finished_at: Some(now_rfc3339()),
                    inputs: Some(json!({
                        "predecessors": compiler.incoming_edges(node_id),
                    })),
                    outputs: Some(monitor_outputs),
                    error: execution_error.clone(),
                });
            }
            _ => {}
        }
        ctx.checkpoint.save(&node_runs, "RUNNING", &last_run_id, &last_session_id, &last_response)?;
    }
    ctx.checkpoint.save(&node_runs, &overall_status, &last_run_id, &last_session_id, &last_response)?;

    let response = if last_response.is_object() && last_response.get("run").is_some() {
        last_response
    } else {
        json!({
            "run": {
                "id": last_run_id,
                "status": overall_status,
            },
            "session": {
                "id": last_session_id,
            },
            "reply": last_reply,
        })
    };

    Ok(WorkflowExecutionOutcome {
        status: overall_status,
        run_id: last_run_id,
        session_id: last_session_id,
        response,
        node_runs,
    })
}

fn workflow_chat_request(previous: Option<&WorkflowNodeRun>, legacy_session: &str, mut body: Value) -> Value {
    // An unstarted node lets the chat owner resolve/create its session. Passing
    // a fabricated explicit id would require a row which does not yet exist.
    // A checkpoint from before dispatch owns the complete original request.
    if let Some(saved) = previous.and_then(|node| node.inputs.as_ref())
        && saved.get("clientRequestId") == body.get("clientRequestId") {
        return saved.clone();
    }
    // Older paused nodes saved only the projected inputs and outputs after
    // dispatch. Their deterministic session id identifies the original body;
    // keep it explicit so the existing run's canonical identity still matches.
    if previous.and_then(|node| node.outputs.as_ref())
        .and_then(|output| output.get("sessionId")).and_then(Value::as_str) == Some(legacy_session) {
        body["sessionId"] = json!(legacy_session);
    }
    body
}
