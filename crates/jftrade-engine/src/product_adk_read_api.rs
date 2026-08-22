use std::fmt;


#[derive(Clone, Debug, PartialEq)]
pub enum AdkReadOutput {
    Json(Value),
    Stream(AdkReadStream),
}

#[derive(Clone, Debug, Eq, Error, PartialEq)]
#[error("{status} {code}: {message}")]
pub struct AdkReadFailure {
    pub status: u16,
    pub code: String,
    pub message: String,
    pub retry_after_seconds: Option<u64>,
}

impl AdkReadFailure {
    fn new(status: u16, code: impl Into<String>, message: impl Into<String>) -> Self {
        Self {
            status,
            code: code.into(),
            message: message.into(),
            retry_after_seconds: None,
        }
    }
}

pub fn dispatch_adk_read(
    port: Option<&dyn AdkReadSnapshotPort>,
    method: &str,
    path: &str,
    query: &str,
) -> Result<AdkReadOutput, AdkReadFailure> {
    if method != "GET" {
        return Err(unknown_endpoint(path));
    }
    let Some(route) = route_for(path) else {
        return Err(unknown_endpoint(path));
    };
    validate_path(route, path)?;
    validate_query(route, query)?;
    let Some(port) = port else {
        return Err(AdkReadFailure::new(
            503,
            "ADK_READ_UNAVAILABLE",
            "ADK read snapshot port is not configured",
        ));
    };
    match port.read(path, query).map_err(snapshot_failure)? {
        AdkReadSnapshot::Json(value) if !route_is_stream(route) => Ok(AdkReadOutput::Json(value)),
        AdkReadSnapshot::Stream(stream) if route_is_stream(route) => {
            Ok(AdkReadOutput::Stream(stream))
        }
        AdkReadSnapshot::Json(_) | AdkReadSnapshot::Stream(_) => Err(AdkReadFailure::new(
            500,
            "ADK_READ_INVALID_SNAPSHOT",
            "ADK read snapshot kind does not match the route",
        )),
    }
}

impl ProductApi {
    fn adk_read(&self, request: &ApiRequest) -> Result<ApiOutput, ApiFailure> {
        dispatch_adk_read(
            self.adk_read_snapshot_port.as_deref(),
            &request.method,
            &request.path,
            &request.query,
        )
        .map(adk_read_output)
        .map_err(adk_read_failure)
    }
}

fn adk_read_output(output: AdkReadOutput) -> ApiOutput {
    match output {
        AdkReadOutput::Json(value) => ApiOutput::Json(value),
        AdkReadOutput::Stream(stream) => {
            // The product transport owns the standard SSE headers and framing.
            // The raw snapshot port still carries source headers as evidence.
            ApiOutput::Sse(
                stream
                    .events
                    .into_iter()
                    .map(|event| SseEvent {
                        id: event.id,
                        data: event.data,
                    })
                    .collect(),
            )
        }
    }
}

fn adk_read_failure(error: AdkReadFailure) -> ApiFailure {
    ApiFailure {
        status: error.status,
        code: error.code,
        message: error.message,
        retry_after_seconds: error.retry_after_seconds,
    }
}

fn snapshot_failure(error: AdkReadSnapshotError) -> AdkReadFailure {
    match error {
        AdkReadSnapshotError::Unavailable(message) => {
            AdkReadFailure::new(503, "ADK_READ_UNAVAILABLE", message)
        }
        AdkReadSnapshotError::Failed {
            status,
            code,
            message,
            retry_after_seconds,
        } => AdkReadFailure {
            status,
            code,
            message,
            retry_after_seconds,
        },
    }
}

fn unknown_endpoint(path: &str) -> AdkReadFailure {
    AdkReadFailure::new(404, "NOT_FOUND", format!("unknown endpoint {path}"))
}

fn route_is_stream(route: AdkReadRoute) -> bool {
    matches!(route, AdkReadRoute::RunStream | AdkReadRoute::Stream)
}

fn validate_path(route: AdkReadRoute, path: &str) -> Result<(), AdkReadFailure> {
    let (prefix, suffix, label) = match route {
        AdkReadRoute::OptimizationTask => ("/api/v1/adk/optimization-tasks/", "", "taskId"),
        AdkReadRoute::Run => ("/api/v1/adk/runs/", "", "runId"),
        AdkReadRoute::RunStream => ("/api/v1/adk/runs/", "/stream", "runId"),
        AdkReadRoute::Session => ("/api/v1/adk/sessions/", "", "sessionId"),
        AdkReadRoute::SessionContext => ("/api/v1/adk/sessions/", "/context", "sessionId"),
        AdkReadRoute::Stream => ("/api/v1/adk/streams/", "", "streamId"),
        AdkReadRoute::Task => ("/api/v1/adk/tasks/", "", "taskId"),
        AdkReadRoute::Workflow => ("/api/v1/adk/workflows/", "", "workflowId"),
        AdkReadRoute::WorkflowTriggers => ("/api/v1/adk/workflows/", "/triggers", "workflowId"),
        _ => return Ok(()),
    };
    let encoded = path
        .strip_prefix(prefix)
        .and_then(|value| value.strip_suffix(suffix))
        .unwrap_or_default();
    if has_invalid_percent_encoding(encoded) {
        return Err(invalid_identifier(label));
    }
    let decoded = percent_decode_str(encoded)
        .decode_utf8()
        .map_err(|_| invalid_identifier(label))?;
    if decoded.trim().is_empty() || decoded.contains('/') {
        return Err(invalid_identifier(label));
    }
    Ok(())
}

fn invalid_identifier(label: &str) -> AdkReadFailure {
    AdkReadFailure::new(400, "BAD_REQUEST", format!("{label} is invalid"))
}

fn validate_query(route: AdkReadRoute, query: &str) -> Result<(), AdkReadFailure> {
    let requires_query_binding = matches!(
        route,
        AdkReadRoute::Agents
            | AdkReadRoute::Approvals
            | AdkReadRoute::Audit
            | AdkReadRoute::Memory
            | AdkReadRoute::OptimizationTasks
            | AdkReadRoute::Runs
            | AdkReadRoute::Sessions
            | AdkReadRoute::Tasks
            | AdkReadRoute::WorkflowTriggerLogs
            | AdkReadRoute::Workflows
    );
    if !requires_query_binding && !route_is_stream(route) {
        return Ok(());
    }
    let pairs = decode_query(query).map_err(|_| query_failure(route))?;
    if route_is_stream(route) {
        if pairs
            .iter()
            .find_map(|(key, value)| (key == "after").then_some(value))
            .is_some_and(|value| {
                !value.trim().is_empty()
                    && value.parse::<i64>().map_or(true, |parsed| parsed < 0)
            })
        {
            return Err(AdkReadFailure::new(400, "BAD_REQUEST", "after is invalid"));
        }
        return Ok(());
    }
    for (key, value) in pairs {
        if matches!(key.as_str(), "limit" | "offset") && value.parse::<i64>().is_err() {
            return Err(query_failure(route));
        }
    }
    Ok(())
}

fn query_failure(route: AdkReadRoute) -> AdkReadFailure {
    let resource = match route {
        AdkReadRoute::Agents => "agents",
        AdkReadRoute::Approvals => "approvals",
        AdkReadRoute::Audit => "audit",
        AdkReadRoute::Memory => "memory",
        AdkReadRoute::OptimizationTasks => "optimization tasks",
        AdkReadRoute::Runs => "runs",
        AdkReadRoute::Sessions => "sessions",
        AdkReadRoute::Tasks => "tasks",
        AdkReadRoute::WorkflowTriggerLogs => "workflow trigger logs",
        AdkReadRoute::Workflows => "workflows",
        _ => "ADK",
    };
    AdkReadFailure::new(
        400,
        "BAD_REQUEST",
        format!("invalid {resource} query"),
    )
}

fn decode_query(query: &str) -> Result<Vec<(String, String)>, ()> {
    if query.is_empty() {
        return Ok(Vec::new());
    }
    query
        .split('&')
        .filter(|pair| !pair.is_empty())
        .map(|pair| {
            let (key, value) = pair.split_once('=').unwrap_or((pair, ""));
            Ok((decode_component(key)?, decode_component(value)?))
        })
        .collect()
}

fn decode_component(value: &str) -> Result<String, ()> {
    if has_invalid_percent_encoding(value) {
        return Err(());
    }
    percent_decode_str(&value.replace('+', " "))
        .decode_utf8()
        .map(|value| value.into_owned())
        .map_err(|_| ())
}

fn has_invalid_percent_encoding(value: &str) -> bool {
    let bytes = value.as_bytes();
    bytes.iter().enumerate().any(|(index, byte)| {
        *byte == b'%'
            && (index + 2 >= bytes.len()
                || !bytes[index + 1].is_ascii_hexdigit()
                || !bytes[index + 2].is_ascii_hexdigit())
    })
}

impl fmt::Display for AdkReadOutput {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Json(_) => formatter.write_str("json"),
            Self::Stream(_) => formatter.write_str("stream"),
        }
    }
}
