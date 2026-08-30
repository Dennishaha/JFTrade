impl AdkChatStreamPort for ProductionAdkChatRuntime {
    fn dispatch(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        let store = Arc::clone(&self.store);
        let session_store = Arc::clone(&self.session_store);
        let secrets_path = self.secrets_path.clone();
        let cancellation_registry = Arc::clone(&self.cancellation_registry);
        let tool_catalog = Arc::clone(&self.tool_catalog);
        let input = input.clone();
        if route == AdkChatRoute::Stream {
            let (stream, sender) = ApiStream::channel(32);
            let (started, ready) = std::sync::mpsc::sync_channel(1);
            let stream_store = Arc::clone(&store);
            let stream_session_store = Arc::clone(&session_store);
            let stream_secrets_path = secrets_path.clone();
            let stream_input = input.clone();
            std::thread::Builder::new()
                .name("jftrade-adk-stream".to_owned())
                .spawn(move || {
                    let tool_executor = Arc::new(ProductionAdkToolExecutor::new(
                        Arc::clone(&tool_catalog),
                        Arc::clone(&stream_store),
                    ));
                    let runtime = ProductionAdkChatRuntime {
                        store: stream_store,
                        session_store: stream_session_store,
                        secrets_path: stream_secrets_path,
                        cancellation_registry: Arc::clone(&cancellation_registry),
                        tool_catalog: Arc::clone(&tool_catalog),
                        tool_executor,
                    };
                    runtime.start_live_stream(stream_input, stream, sender, started);
                })
                .map_err(|error| AdkChatPortError::Unavailable(error.to_string()))?;
            return ready
                .recv()
                .map_err(|_| unavailable("assistant model stream failed to start"))?;
        }
        std::thread::Builder::new()
            .name("jftrade-adk-model".to_owned())
            .spawn(move || {
                let tool_executor = Arc::new(ProductionAdkToolExecutor::new(
                    Arc::clone(&tool_catalog),
                    Arc::clone(&store),
                ));
                let runtime = ProductionAdkChatRuntime {
                    store,
                    session_store,
                    secrets_path,
                    cancellation_registry,
                    tool_catalog,
                    tool_executor,
                };
                runtime.dispatch_inner(route, &input)
            })
            .map_err(|error| AdkChatPortError::Unavailable(error.to_string()))?
            .join()
            .map_err(|_| unavailable("assistant model runtime panicked"))?
    }

    fn cancel_run(&self, run_id: &str) -> bool {
        self.cancellation_registry.cancel(run_id)
    }

    fn resume_approval(&self, run_id: &str) -> Result<(), AdkChatPortError> {
        ProductionAdkChatRuntime::resume_approval(self, run_id)
    }
}

#[derive(Debug)]
struct ResolvedProvider {
    id: String,
    agent_id: String,
    endpoint: Url,
    api_key: String,
    model: String,
    agent_model: Option<String>,
    instruction: Option<String>,
    timeout: Duration,
}

fn parse_agent_payload(raw: &str) -> Result<Value, AdkChatPortError> {
    serde_json::from_str::<Value>(raw).map_err(|error| AdkChatPortError::Failed {
        status: 500,
        code: "ADK_STORAGE_CORRUPT".to_owned(),
        message: format!("stored ADK agent payload is invalid JSON: {error}"),
    })
}

fn agent_enabled(payload: &Value) -> bool {
    let status = payload
        .get("status")
        .and_then(Value::as_str)
        .unwrap_or("ENABLED");
    status.eq_ignore_ascii_case("ENABLED")
        && !payload
            .get("deletedAt")
            .is_some_and(|value| !value.is_null())
}

fn bad_agent(message: &str) -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 400,
        code: "BAD_REQUEST".to_owned(),
        message: message.to_owned(),
    }
}

#[derive(Clone, Debug)]
struct ModelRequest {
    endpoint: Url,
    api_key: String,
    model: String,
    instruction: Option<String>,
    message: String,
    /// Responses API input items produced by prior tool-call rounds. The
    /// initial request leaves this empty; approval continuations append the
    /// original function calls and durable function_call_output items.
    tool_context: Vec<Value>,
    timeout: Duration,
    tools: Vec<Value>,
}

#[derive(Debug)]
struct ModelResponse {
    text: String,
    tool_calls: Vec<ModelToolCall>,
}

#[derive(Clone, Debug)]
struct ModelToolCall {
    id: String,
    name: String,
    arguments: Value,
}

struct CancellationGuard {
    registry: Arc<RunCancellationRegistry>,
    run_id: String,
}

impl Drop for CancellationGuard {
    fn drop(&mut self) {
        self.registry.unregister(&self.run_id);
    }
}

fn execute_model(
    request: ModelRequest,
    cancellation: Arc<AtomicBool>,
) -> Result<ModelResponse, AdkChatPortError> {
    // reqwest is built with rustls-no-provider so the engine can use the same
    // ring-backed crypto provider as the desktop runtime without relying on a
    // process-global provider installed by an unrelated integration crate.
    let _ = rustls::crypto::ring::default_provider().install_default();
    let runtime = tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .map_err(|error| unavailable(format!("assistant model runtime unavailable: {error}")))?;
    runtime.block_on(async move {
        if cancellation.load(Ordering::Acquire) {
            return Err(cancellation_error());
        }
        let client = Client::builder()
            .connect_timeout(request.timeout)
            .timeout(request.timeout)
            .redirect(reqwest::redirect::Policy::none())
            .build()
            .map_err(|error| upstream_error(format!("create model client: {error}")))?;
        let mut input = Vec::new();
        if let Some(instruction) = request.instruction.filter(|value| !value.trim().is_empty()) {
            input.push(json!({"role":"system","content":instruction}));
        }
        input.push(json!({"role":"user","content":request.message}));
        input.extend(request.tool_context.clone());
        let mut body = json!({"model":request.model,"input":input,"stream":false});
        if !request.tools.is_empty() {
            body["tools"] = Value::Array(request.tools.clone());
        }
        let send = client
            .post(request.endpoint)
            .bearer_auth(request.api_key)
            .header("content-type", "application/json")
            .json(&body)
            .send();
        tokio::pin!(send);
        let response = loop {
            tokio::select! {
                result = &mut send => {
                    break result.map_err(|error| {
                        if error.is_timeout() {
                            AdkChatPortError::Failed {
                                status: 504,
                                code: "MODEL_CALL_TIMEOUT".to_owned(),
                                message: "assistant model request timed out".to_owned(),
                            }
                        } else {
                            upstream_error(error.to_string())
                        }
                    })?;
                }
                _ = tokio::time::sleep(Duration::from_millis(100)) => {
                    if cancellation.load(Ordering::Acquire) {
                        return Err(cancellation_error());
                    }
                }
            }
        };
        let status = response.status();
        let retry_after = response
            .headers()
            .get(reqwest::header::RETRY_AFTER)
            .and_then(|value| value.to_str().ok())
            .map(str::to_owned);
        let bytes_future = response.bytes();
        tokio::pin!(bytes_future);
        let bytes = loop {
            tokio::select! {
                result = &mut bytes_future => {
                    break result.map_err(|error| upstream_error(error.to_string()))?;
                }
                _ = tokio::time::sleep(Duration::from_millis(100)) => {
                    if cancellation.load(Ordering::Acquire) {
                        return Err(cancellation_error());
                    }
                }
            }
        };
        if bytes.len() > MAX_RESPONSE_BYTES {
            return Err(upstream_error(
                "assistant model response exceeded size limit",
            ));
        }
        if !status.is_success() {
            let value = serde_json::from_slice::<Value>(&bytes).unwrap_or(Value::Null);
            return Err(provider_rejection(status, retry_after.as_deref(), &value));
        }
        let value: Value = serde_json::from_slice(&bytes)
            .map_err(|error| upstream_error(format!("decode model response: {error}")))?;
        let text = extract_text(&value).trim().to_owned();
        let tool_calls = extract_tool_calls(&value)?;
        if !tool_calls.is_empty() {
            return Ok(ModelResponse { text, tool_calls });
        }
        if text.is_empty() {
            return Err(upstream_error("assistant model returned an empty response"));
        }
        Ok(ModelResponse { text, tool_calls })
    })
}

fn extract_text(value: &Value) -> String {
    if let Some(text) = value.get("output_text").and_then(Value::as_str) {
        return text.to_owned();
    }
    if let Some(text) = value
        .pointer("/choices/0/message/content")
        .and_then(Value::as_str)
    {
        return text.to_owned();
    }
    let mut output = String::new();
    if let Some(items) = value.get("output").and_then(Value::as_array) {
        for item in items {
            if let Some(content) = item.get("content").and_then(Value::as_array) {
                for part in content {
                    if let Some(text) = part.get("text").and_then(Value::as_str) {
                        output.push_str(text);
                    }
                }
            }
        }
    }
    output
}

fn extract_tool_calls(value: &Value) -> Result<Vec<ModelToolCall>, AdkChatPortError> {
    let Some(output) = value.get("output").and_then(Value::as_array) else {
        return Ok(Vec::new());
    };
    let mut calls = Vec::new();
    for item in output {
        if item.get("type").and_then(Value::as_str) != Some("function_call") {
            continue;
        }
        let name = item
            .get("name")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| upstream_error("assistant model returned a tool call without a name"))?
            .to_owned();
        let id = item
            .get("call_id")
            .or_else(|| item.get("id"))
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| upstream_error("assistant model returned a tool call without an id"))?
            .to_owned();
        let arguments = match item.get("arguments") {
            Some(Value::String(raw)) => serde_json::from_str(raw)
                .map_err(|error| upstream_error(format!("invalid tool call arguments: {error}")))?,
            Some(value) if value.is_object() => value.clone(),
            Some(_) | None => {
                return Err(upstream_error(
                    "assistant model returned invalid tool arguments",
                ));
            }
        };
        calls.push(ModelToolCall {
            id,
            name,
            arguments,
        });
    }
    Ok(calls)
}

fn encode_sse_event(value: &Value) -> Vec<u8> {
    let mut body = String::new();
    if let (Some(stream_id), Some(sequence)) = (
        value.get("streamId").and_then(Value::as_str),
        value.get("sequence").and_then(Value::as_u64),
    ) {
        body.push_str("id: ");
        body.push_str(stream_id);
        body.push(':');
        body.push_str(&sequence.to_string());
        body.push('\n');
    }
    body.push_str("data: ");
    body.push_str(&serde_json::to_string(value).unwrap_or_else(|_| "{}".to_owned()));
    body.push_str("\n\n");
    body.into_bytes()
}

fn format_adk_error(error: &AdkChatPortError) -> String {
    match error {
        AdkChatPortError::Unavailable(message) | AdkChatPortError::Conflict(message) => {
            message.clone()
        }
        AdkChatPortError::Failed { code, message, .. } => format!("{code}: {message}"),
    }
}

fn is_client_disconnect(error: &AdkChatPortError) -> bool {
    matches!(
        error,
        AdkChatPortError::Failed { code, .. } if code == "CLIENT_DISCONNECTED"
    )
}

fn is_run_cancelled(error: &AdkChatPortError) -> bool {
    matches!(
        error,
        AdkChatPortError::Failed { code, .. } if code == "RUN_CANCELLED"
    )
}

fn is_cancellation_error(error: &AdkChatPortError) -> bool {
    matches!(
        error,
        AdkChatPortError::Failed { status: 499, code, .. }
            if code == "CLIENT_DISCONNECTED" || code == "RUN_CANCELLED"
    )
}

fn cancellation_error() -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 499,
        code: "CLIENT_DISCONNECTED".to_owned(),
        message: "assistant chat client disconnected".to_owned(),
    }
}

fn responses_endpoint(base_url: &str) -> Result<Url, String> {
    let mut url = Url::parse(base_url.trim()).map_err(|error| error.to_string())?;
    if !matches!(url.scheme(), "http" | "https") || url.host_str().is_none() {
        return Err("assistant model provider baseUrl must use http or https".to_owned());
    }
    let path = url.path().trim_end_matches('/');
    if !path.ends_with("/responses") {
        url.set_path(&format!("{path}/responses"));
    }
    Ok(url)
}

fn read_secrets(path: &Path) -> Result<BTreeMap<String, String>, AdkChatPortError> {
    let bytes = match fs::read(path) {
        Ok(bytes) => bytes,
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => return Ok(BTreeMap::new()),
        Err(error) => {
            return Err(AdkChatPortError::Failed {
                status: 500,
                code: "ADK_SECRET_STORE_FAILED".to_owned(),
                message: error.to_string(),
            });
        }
    };
    if bytes.iter().all(u8::is_ascii_whitespace) {
        return Ok(BTreeMap::new());
    }
    serde_json::from_slice(&bytes).map_err(|error| AdkChatPortError::Failed {
        status: 500,
        code: "ADK_SECRET_STORE_FAILED".to_owned(),
        message: error.to_string(),
    })
}

fn persisted_response(raw: &str) -> Result<Option<Value>, AdkChatPortError> {
    let value: Value = serde_json::from_str(raw).map_err(storage_unavailable)?;
    Ok(value.get("response").cloned())
}

fn stream_from_payload(raw: &str) -> Result<AdkChatPortOutput, AdkChatPortError> {
    let value: Value = serde_json::from_str(raw).map_err(storage_unavailable)?;
    let stream_id = value
        .get("streamId")
        .and_then(Value::as_str)
        .filter(|id| !id.trim().is_empty())
        .ok_or_else(|| unavailable("persisted ADK run has no stream id"))?
        .to_owned();
    let events = value
        .get("streamEvents")
        .and_then(Value::as_array)
        .ok_or_else(|| unavailable("persisted ADK run has no stream events"))?;
    let frames = value
        .get("streamEvents")
        .and_then(Value::as_array)
        .unwrap_or(events)
        .iter()
        .map(|event| {
            let id = event
                .get("sequence")
                .and_then(Value::as_u64)
                .map(|sequence| format!("{stream_id}:{sequence}"));
            AdkChatStreamFrame::Event {
                id,
                data: event.clone(),
            }
        })
        .collect();
    Ok(AdkChatPortOutput::Stream(AdkChatStreamSnapshot {
        headers: BTreeMap::from([(String::from("X-ADK-Stream-ID"), stream_id)]),
        frames,
        terminal: true,
    }))
}

fn fingerprint(bytes: &[u8]) -> String {
    let digest = Sha256::digest(bytes);
    digest.iter().map(|byte| format!("{byte:02x}")).collect()
}

fn unavailable(message: impl Into<String>) -> AdkChatPortError {
    AdkChatPortError::Unavailable(message.into())
}

fn upstream_error(message: impl Into<String>) -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 502,
        code: "MODEL_CALL_FAILED".to_owned(),
        message: message.into(),
    }
}

fn provider_rejection(
    status: StatusCode,
    retry_after: Option<&str>,
    value: &Value,
) -> AdkChatPortError {
    let message = value
        .pointer("/error/message")
        .and_then(Value::as_str)
        .unwrap_or("assistant model provider rejected the request");
    let (code, status_code) = match status {
        // Provider auth failures are external dependency failures.  Do not
        // leak 401/403 through the local session boundary, where they would
        // be interpreted as a browser-auth failure.
        StatusCode::UNAUTHORIZED => ("MODEL_PROVIDER_UNAUTHORIZED", 502),
        StatusCode::FORBIDDEN => ("MODEL_PROVIDER_FORBIDDEN", 503),
        StatusCode::TOO_MANY_REQUESTS => ("MODEL_PROVIDER_RATE_LIMITED", 429),
        _ => ("MODEL_CALL_FAILED", status.as_u16()),
    };
    let message = match retry_after.filter(|value| !value.trim().is_empty()) {
        Some(retry_after) => format!("{message} (Retry-After: {retry_after})"),
        None => message.to_owned(),
    };
    AdkChatPortError::Failed {
        status: status_code,
        code: code.to_owned(),
        message,
    }
}

fn storage_unavailable(error: impl std::fmt::Display) -> AdkChatPortError {
    unavailable(format!("ADK storage unavailable: {error}"))
}
