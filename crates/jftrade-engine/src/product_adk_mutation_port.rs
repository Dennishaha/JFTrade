//! Stage 9 test-cutover leaf for Assistant mutation and control routes.
//!
//! Go remains the only owner of Assistant runtime, provider, session/task
//! stores, approvals, notifications and workflow execution.  This module only
//! parses the public route boundary and delegates a structured request to an
//! explicitly injected test port.

use std::collections::BTreeMap;

use percent_encoding::percent_decode_str;
use serde::de::Deserialize;
use serde_json::{Map, Value, json};
use thiserror::Error;

pub const ADK_MUTATION_ROUTES: [(&str, &str); 37] = [
    ("DELETE", "/api/v1/adk/agents/{agentId}"),
    ("DELETE", "/api/v1/adk/memory/{memoryId}"),
    ("DELETE", "/api/v1/adk/providers/{providerId}"),
    ("DELETE", "/api/v1/adk/sessions/{sessionId}"),
    ("DELETE", "/api/v1/adk/skills/{skillId}"),
    ("DELETE", "/api/v1/adk/tasks/{taskId}"),
    ("DELETE", "/api/v1/adk/workflows/{workflowId}"),
    (
        "DELETE",
        "/api/v1/adk/workflows/{workflowId}/triggers/{triggerId}",
    ),
    ("PATCH", "/api/v1/adk/runs/{runId}/objective"),
    ("PATCH", "/api/v1/adk/sessions/{sessionId}/composer-state"),
    ("POST", "/api/v1/adk/agents"),
    ("POST", "/api/v1/adk/approvals/{approvalId}/approve"),
    ("POST", "/api/v1/adk/approvals/{approvalId}/deny"),
    ("POST", "/api/v1/adk/memory"),
    ("POST", "/api/v1/adk/optimization-tasks/{taskId}/cancel"),
    ("POST", "/api/v1/adk/providers"),
    ("POST", "/api/v1/adk/providers/{providerId}/default"),
    ("POST", "/api/v1/adk/providers/{providerId}/test"),
    ("POST", "/api/v1/adk/runs/{runId}/cancel"),
    ("POST", "/api/v1/adk/runs/{runId}/input-response"),
    ("POST", "/api/v1/adk/runs/{runId}/pause"),
    ("POST", "/api/v1/adk/runs/{runId}/resume"),
    ("POST", "/api/v1/adk/sessions"),
    ("POST", "/api/v1/adk/sessions/{sessionId}/context/compact"),
    ("POST", "/api/v1/adk/skills"),
    ("POST", "/api/v1/adk/tasks"),
    ("POST", "/api/v1/adk/workflow-triggers/{triggerId}/run"),
    ("POST", "/api/v1/adk/workflow-webhooks/{triggerId}"),
    ("POST", "/api/v1/adk/workflows"),
    ("POST", "/api/v1/adk/workflows/{workflowId}/run"),
    ("POST", "/api/v1/adk/workflows/{workflowId}/triggers"),
    ("PUT", "/api/v1/adk/agents/{agentId}"),
    ("PUT", "/api/v1/adk/providers/{providerId}"),
    ("PUT", "/api/v1/adk/sessions/{sessionId}"),
    ("PUT", "/api/v1/adk/tasks/{taskId}"),
    ("PUT", "/api/v1/adk/workflows/{workflowId}"),
    (
        "PUT",
        "/api/v1/adk/workflows/{workflowId}/triggers/{triggerId}",
    ),
];

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum AdkMutationOperation {
    DeleteAgent,
    DeleteMemory,
    DeleteProvider,
    DeleteSession,
    DeleteSkill,
    DeleteTask,
    DeleteWorkflow,
    DeleteWorkflowTrigger,
    UpdateRunObjective,
    UpdateSessionComposerState,
    CreateAgent,
    Approve,
    Deny,
    CreateMemory,
    CancelOptimizationTask,
    CreateProvider,
    SetDefaultProvider,
    TestProvider,
    CancelRun,
    RespondToInput,
    PauseRun,
    ResumeRun,
    CreateSession,
    CompactSessionContext,
    InstallSkill,
    CreateTask,
    RunWorkflowTrigger,
    RunWorkflowWebhook,
    CreateWorkflow,
    RunWorkflow,
    CreateWorkflowTrigger,
    UpdateAgent,
    UpdateProvider,
    RenameSession,
    UpdateTask,
    UpdateWorkflow,
    UpdateWorkflowTrigger,
}

include!("product_adk_mutation_operation.rs");

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AdkMutationRequest {
    pub method: String,
    pub path: String,
    pub body: Option<Vec<u8>>,
    pub headers: BTreeMap<String, String>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AdkMutationInput {
    pub operation: AdkMutationOperation,
    pub identifiers: BTreeMap<String, String>,
    pub body: Value,
    pub webhook_secret: Option<String>,
}

#[derive(Clone, Debug, Eq, PartialEq, Error)]
pub enum AdkMutationPortError {
    #[error("ADK mutation port is unavailable: {0}")]
    Unavailable(String),
    #[error("{code}: {message}")]
    Failed {
        status: u16,
        code: String,
        message: String,
    },
}

/// Consumer-owned mutation boundary. The injected adapter may call the Go
/// owner during explicit rehearsal, but this port has no state or side-effect
/// capability of its own.
pub trait AdkMutationPort: Send + Sync + std::fmt::Debug {
    fn mutate(&self, input: &AdkMutationInput) -> Result<Value, AdkMutationPortError>;
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AdkMutationResponse {
    pub status: u16,
    pub headers: BTreeMap<String, String>,
    pub body: Value,
}

pub fn adk_mutation_routes() -> &'static [(&'static str, &'static str); 37] {
    &ADK_MUTATION_ROUTES
}

pub fn dispatch_adk_mutation(
    request: &AdkMutationRequest,
    port: Option<&dyn AdkMutationPort>,
    timestamp: &str,
) -> AdkMutationResponse {
    let (path, _) = split_path_query(&request.path);
    let input = match parse_input(
        request.method.as_str(),
        path,
        request.body.as_deref(),
        &request.headers,
    ) {
        Ok(input) => input,
        Err(error) => return error_response(error, timestamp),
    };
    let Some(port) = port else {
        return error_response(
            ErrorSpec {
                status: 503,
                code: "ADK_MUTATIONS_UNAVAILABLE".to_owned(),
                message: "ADK mutation port is unavailable".to_owned(),
            },
            timestamp,
        );
    };
    match port.mutate(&input) {
        Ok(data) => success_response(data, timestamp),
        Err(AdkMutationPortError::Unavailable(message)) => error_response(
            ErrorSpec {
                status: 503,
                code: "ADK_MUTATIONS_UNAVAILABLE".to_owned(),
                message,
            },
            timestamp,
        ),
        Err(AdkMutationPortError::Failed {
            status,
            code,
            message,
        }) => error_response(
            ErrorSpec {
                status,
                code,
                message,
            },
            timestamp,
        ),
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
struct ErrorSpec {
    status: u16,
    code: String,
    message: String,
}

fn parse_input(
    method: &str,
    path: &str,
    body: Option<&[u8]>,
    headers: &BTreeMap<String, String>,
) -> Result<AdkMutationInput, ErrorSpec> {
    let (operation, identifiers) = parse_route(method, path)?;
    let body = if accepts_workflow_inputs(operation) {
        parse_workflow_inputs(body)?
    } else if ignores_body(operation) {
        Value::Object(Map::new())
    } else {
        parse_object_body(
            body,
            body_required(operation),
            body_error_message(operation),
        )?
    };
    let webhook_secret = (operation == AdkMutationOperation::RunWorkflowWebhook)
        .then(|| webhook_secret(headers))
        .flatten();
    Ok(AdkMutationInput {
        operation,
        identifiers,
        body,
        webhook_secret,
    })
}

fn parse_route(
    method: &str,
    path: &str,
) -> Result<(AdkMutationOperation, BTreeMap<String, String>), ErrorSpec> {
    let exact = [
        (
            "POST",
            "/api/v1/adk/agents",
            AdkMutationOperation::CreateAgent,
        ),
        (
            "POST",
            "/api/v1/adk/memory",
            AdkMutationOperation::CreateMemory,
        ),
        (
            "POST",
            "/api/v1/adk/providers",
            AdkMutationOperation::CreateProvider,
        ),
        (
            "POST",
            "/api/v1/adk/sessions",
            AdkMutationOperation::CreateSession,
        ),
        (
            "POST",
            "/api/v1/adk/skills",
            AdkMutationOperation::InstallSkill,
        ),
        (
            "POST",
            "/api/v1/adk/tasks",
            AdkMutationOperation::CreateTask,
        ),
        (
            "POST",
            "/api/v1/adk/workflows",
            AdkMutationOperation::CreateWorkflow,
        ),
    ];
    for (expected_method, expected_path, operation) in exact {
        if path == expected_path {
            return if method == expected_method {
                Ok((operation, BTreeMap::new()))
            } else {
                Err(not_found_spec(path))
            };
        }
    }

    let dynamic = [
        (
            "DELETE",
            "/api/v1/adk/agents/",
            "",
            "agentId",
            AdkMutationOperation::DeleteAgent,
            "agentId is invalid",
        ),
        (
            "DELETE",
            "/api/v1/adk/memory/",
            "",
            "memoryId",
            AdkMutationOperation::DeleteMemory,
            "memoryId is invalid",
        ),
        (
            "DELETE",
            "/api/v1/adk/providers/",
            "",
            "providerId",
            AdkMutationOperation::DeleteProvider,
            "providerId is invalid",
        ),
        (
            "DELETE",
            "/api/v1/adk/sessions/",
            "",
            "sessionId",
            AdkMutationOperation::DeleteSession,
            "sessionId is invalid",
        ),
        (
            "DELETE",
            "/api/v1/adk/skills/",
            "",
            "skillId",
            AdkMutationOperation::DeleteSkill,
            "skillId is invalid",
        ),
        (
            "DELETE",
            "/api/v1/adk/tasks/",
            "",
            "taskId",
            AdkMutationOperation::DeleteTask,
            "taskId is invalid",
        ),
        (
            "DELETE",
            "/api/v1/adk/workflows/",
            "",
            "workflowId",
            AdkMutationOperation::DeleteWorkflow,
            "workflowId is invalid",
        ),
        (
            "PATCH",
            "/api/v1/adk/runs/",
            "/objective",
            "runId",
            AdkMutationOperation::UpdateRunObjective,
            "runId is invalid",
        ),
        (
            "PATCH",
            "/api/v1/adk/sessions/",
            "/composer-state",
            "sessionId",
            AdkMutationOperation::UpdateSessionComposerState,
            "sessionId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/optimization-tasks/",
            "/cancel",
            "taskId",
            AdkMutationOperation::CancelOptimizationTask,
            "taskId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/approvals/",
            "/approve",
            "approvalId",
            AdkMutationOperation::Approve,
            "approvalId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/approvals/",
            "/deny",
            "approvalId",
            AdkMutationOperation::Deny,
            "approvalId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/providers/",
            "/default",
            "providerId",
            AdkMutationOperation::SetDefaultProvider,
            "providerId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/providers/",
            "/test",
            "providerId",
            AdkMutationOperation::TestProvider,
            "providerId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/runs/",
            "/cancel",
            "runId",
            AdkMutationOperation::CancelRun,
            "runId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/runs/",
            "/input-response",
            "runId",
            AdkMutationOperation::RespondToInput,
            "runId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/runs/",
            "/pause",
            "runId",
            AdkMutationOperation::PauseRun,
            "runId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/runs/",
            "/resume",
            "runId",
            AdkMutationOperation::ResumeRun,
            "runId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/sessions/",
            "/context/compact",
            "sessionId",
            AdkMutationOperation::CompactSessionContext,
            "sessionId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/workflow-triggers/",
            "/run",
            "triggerId",
            AdkMutationOperation::RunWorkflowTrigger,
            "triggerId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/workflow-webhooks/",
            "",
            "triggerId",
            AdkMutationOperation::RunWorkflowWebhook,
            "triggerId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/workflows/",
            "/run",
            "workflowId",
            AdkMutationOperation::RunWorkflow,
            "workflowId is invalid",
        ),
        (
            "POST",
            "/api/v1/adk/workflows/",
            "/triggers",
            "workflowId",
            AdkMutationOperation::CreateWorkflowTrigger,
            "workflowId is invalid",
        ),
        (
            "PUT",
            "/api/v1/adk/agents/",
            "",
            "agentId",
            AdkMutationOperation::UpdateAgent,
            "agentId is invalid",
        ),
        (
            "PUT",
            "/api/v1/adk/providers/",
            "",
            "providerId",
            AdkMutationOperation::UpdateProvider,
            "providerId is invalid",
        ),
        (
            "PUT",
            "/api/v1/adk/sessions/",
            "",
            "sessionId",
            AdkMutationOperation::RenameSession,
            "sessionId is invalid",
        ),
        (
            "PUT",
            "/api/v1/adk/tasks/",
            "",
            "taskId",
            AdkMutationOperation::UpdateTask,
            "taskId is invalid",
        ),
        (
            "PUT",
            "/api/v1/adk/workflows/",
            "",
            "workflowId",
            AdkMutationOperation::UpdateWorkflow,
            "workflowId is invalid",
        ),
    ];
    if let Some((workflow_id, trigger_id)) =
        two_identifiers(path, "/api/v1/adk/workflows/", "/triggers/")
    {
        if method != "DELETE" && method != "PUT" {
            return Err(not_found_spec(path));
        }
        let workflow_id = decode_identifier(
            workflow_id,
            "workflowId",
            "workflowId or triggerId is invalid",
        )?;
        let trigger_id = decode_identifier(
            trigger_id,
            "triggerId",
            "workflowId or triggerId is invalid",
        )?;
        let operation = if method == "DELETE" {
            AdkMutationOperation::DeleteWorkflowTrigger
        } else {
            AdkMutationOperation::UpdateWorkflowTrigger
        };
        let mut identifiers = BTreeMap::new();
        identifiers.insert("workflowId".to_owned(), workflow_id);
        identifiers.insert("triggerId".to_owned(), trigger_id);
        return Ok((operation, identifiers));
    }

    for (expected_method, prefix, suffix, label, operation, invalid_message) in dynamic {
        if method != expected_method {
            continue;
        }
        if let Some(raw_id) = path
            .strip_prefix(prefix)
            .and_then(|value| value.strip_suffix(suffix))
        {
            if raw_id.contains('/') {
                continue;
            }
            let id = decode_identifier(raw_id, label, invalid_message)?;
            return Ok((operation, one_identifier(label, id)));
        }
    }

    Err(not_found_spec(path))
}

fn one_identifier(name: &str, value: String) -> BTreeMap<String, String> {
    BTreeMap::from([(name.to_owned(), value)])
}

fn two_identifiers<'a>(path: &'a str, prefix: &str, separator: &str) -> Option<(&'a str, &'a str)> {
    let rest = path.strip_prefix(prefix)?;
    let (first, second) = rest.split_once(separator)?;
    Some((first, second))
}

fn decode_identifier(raw: &str, label: &str, message: &str) -> Result<String, ErrorSpec> {
    if raw.is_empty() || raw.contains('/') || !valid_percent_escapes(raw) {
        return Err(bad_request_spec(message));
    }
    let decoded = percent_decode_str(raw)
        .decode_utf8()
        .map_err(|_| bad_request_spec(message))?;
    if decoded.trim().is_empty() || decoded.contains('/') {
        return Err(bad_request_spec(message));
    }
    let _ = label;
    Ok(decoded.into_owned())
}

fn valid_percent_escapes(value: &str) -> bool {
    let bytes = value.as_bytes();
    let mut index = 0;
    while index < bytes.len() {
        if bytes[index] != b'%' {
            index += 1;
            continue;
        }
        if index + 2 >= bytes.len()
            || !bytes[index + 1].is_ascii_hexdigit()
            || !bytes[index + 2].is_ascii_hexdigit()
        {
            return false;
        }
        index += 3;
    }
    true
}

fn accepts_workflow_inputs(operation: AdkMutationOperation) -> bool {
    matches!(
        operation,
        AdkMutationOperation::RunWorkflowTrigger
            | AdkMutationOperation::RunWorkflowWebhook
            | AdkMutationOperation::RunWorkflow
    )
}

fn ignores_body(operation: AdkMutationOperation) -> bool {
    matches!(
        operation,
        AdkMutationOperation::DeleteAgent
            | AdkMutationOperation::DeleteMemory
            | AdkMutationOperation::DeleteProvider
            | AdkMutationOperation::DeleteSession
            | AdkMutationOperation::DeleteSkill
            | AdkMutationOperation::DeleteTask
            | AdkMutationOperation::DeleteWorkflow
            | AdkMutationOperation::DeleteWorkflowTrigger
            | AdkMutationOperation::Approve
            | AdkMutationOperation::Deny
            | AdkMutationOperation::CancelOptimizationTask
            | AdkMutationOperation::SetDefaultProvider
            | AdkMutationOperation::CancelRun
            | AdkMutationOperation::PauseRun
            | AdkMutationOperation::ResumeRun
    )
}

fn body_required(operation: AdkMutationOperation) -> bool {
    matches!(
        operation,
        AdkMutationOperation::RespondToInput
            | AdkMutationOperation::CreateSession
            | AdkMutationOperation::CompactSessionContext
            | AdkMutationOperation::InstallSkill
            | AdkMutationOperation::UpdateSessionComposerState
            | AdkMutationOperation::RenameSession
            | AdkMutationOperation::UpdateRunObjective
    )
}

fn body_error_message(operation: AdkMutationOperation) -> &'static str {
    match operation {
        AdkMutationOperation::CreateAgent | AdkMutationOperation::UpdateAgent => {
            "invalid agent payload"
        }
        AdkMutationOperation::CreateProvider | AdkMutationOperation::UpdateProvider => {
            "invalid provider payload"
        }
        AdkMutationOperation::CreateTask | AdkMutationOperation::UpdateTask => {
            "invalid task payload"
        }
        AdkMutationOperation::CreateMemory => "invalid memory payload",
        AdkMutationOperation::TestProvider => "invalid provider test payload",
        AdkMutationOperation::CreateWorkflow | AdkMutationOperation::UpdateWorkflow => {
            "invalid workflow payload"
        }
        AdkMutationOperation::CreateWorkflowTrigger
        | AdkMutationOperation::UpdateWorkflowTrigger => "invalid workflow trigger payload",
        AdkMutationOperation::RespondToInput => "input response payload is invalid",
        AdkMutationOperation::CreateSession | AdkMutationOperation::RenameSession => {
            "invalid session payload"
        }
        AdkMutationOperation::UpdateSessionComposerState => "invalid composer state payload",
        AdkMutationOperation::CompactSessionContext => "invalid context compaction payload",
        AdkMutationOperation::UpdateRunObjective => "invalid objective payload",
        AdkMutationOperation::InstallSkill => "invalid skill install payload",
        _ => "invalid ADK mutation payload",
    }
}

fn parse_object_body(
    body: Option<&[u8]>,
    required: bool,
    message: &'static str,
) -> Result<Value, ErrorSpec> {
    let Some(body) = body.filter(|body| !body.is_empty()) else {
        return if required {
            Err(bad_request_spec(message))
        } else {
            Ok(Value::Object(Map::new()))
        };
    };
    let mut decoder = serde_json::Deserializer::from_slice(body);
    let value = Value::deserialize(&mut decoder).map_err(|_| bad_request_spec(message))?;
    match value {
        Value::Null => Ok(Value::Object(Map::new())),
        Value::Object(object) => Ok(Value::Object(object)),
        _ => Err(bad_request_spec(message)),
    }
}

fn parse_workflow_inputs(body: Option<&[u8]>) -> Result<Value, ErrorSpec> {
    let Some(body) = body.filter(|body| !body.is_empty()) else {
        return Ok(Value::Object(Map::new()));
    };
    let mut decoder = serde_json::Deserializer::from_slice(body);
    let value = Value::deserialize(&mut decoder)
        .map_err(|_| bad_request_spec("invalid workflow inputs"))?;
    let Value::Object(object) = value else {
        return if value.is_null() {
            Ok(Value::Object(Map::new()))
        } else {
            Err(bad_request_spec("invalid workflow inputs"))
        };
    };
    if let Some(Value::Object(inputs)) = object.get("inputs") {
        return Ok(Value::Object(inputs.clone()));
    }
    Ok(Value::Object(object))
}

fn webhook_secret(headers: &BTreeMap<String, String>) -> Option<String> {
    let authorization = header_value(headers, "authorization");
    if let Some(value) = authorization {
        let trimmed = value.trim();
        if trimmed.len() >= 7 && trimmed[..7].eq_ignore_ascii_case("bearer ") {
            let token = trimmed[7..].trim();
            if !token.is_empty() {
                return Some(token.to_owned());
            }
        }
    }
    header_value(headers, "x-jftrade-workflow-secret")
}

fn header_value(headers: &BTreeMap<String, String>, wanted: &str) -> Option<String> {
    headers
        .iter()
        .find(|(key, _)| key.eq_ignore_ascii_case(wanted))
        .map(|(_, value)| value.clone())
}

fn split_path_query(path: &str) -> (&str, &str) {
    path.split_once('?').unwrap_or((path, ""))
}

fn bad_request_spec(message: &str) -> ErrorSpec {
    ErrorSpec {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

fn not_found_spec(path: &str) -> ErrorSpec {
    ErrorSpec {
        status: 404,
        code: "NOT_FOUND".to_owned(),
        message: format!("unknown endpoint {path}"),
    }
}

fn success_response(data: Value, timestamp: &str) -> AdkMutationResponse {
    AdkMutationResponse {
        status: 200,
        headers: json_headers(),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn error_response(spec: ErrorSpec, timestamp: &str) -> AdkMutationResponse {
    AdkMutationResponse {
        status: spec.status,
        headers: json_headers(),
        body: json!({
            "ok": false,
            "error": {"code": spec.code, "message": spec.message},
            "timestamp": timestamp,
        }),
    }
}

fn json_headers() -> BTreeMap<String, String> {
    BTreeMap::from([(
        "Content-Type".to_owned(),
        "application/json; charset=utf-8".to_owned(),
    )])
}
