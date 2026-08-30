//! Runtime-owned ADK model adapter.

use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use reqwest::{Client, StatusCode, Url};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};

use jftrade_api::ApiStream;
use jftrade_store_sqlite::{AdkSessionStore, AdkStore, CreateAdkRunParams, RecordAdkEventParams};

use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute, AdkChatStreamFrame,
    AdkChatStreamPort, AdkChatStreamSnapshot,
};

#[path = "product_adk_model_stream.rs"]
mod stream_adapter;
use stream_adapter::execute_model_stream;

#[path = "product_adk_model_runtime_stream.rs"]
mod runtime_stream;

const MAX_RESPONSE_BYTES: usize = 4 << 20;
const DEFAULT_TIMEOUT_MS: u64 = 120_000;
const DEFAULT_BUILTIN_AGENT_ID: &str = "jftrade-default";
const DEFAULT_BUILTIN_AGENT_INSTRUCTION: &str = "你是 JFTrade 投资分析 agent。优先使用内部行情、账户、策略和回测工具；涉及安装 skill、保存策略、运行优化或改变自动化状态时遵守当前审批等级。输出必须说明使用了哪些数据来源，不提供保证收益承诺。\n\n对目标明确的任务，要在当前运行中连续完成诊断、结论以及直接相关的可执行方案。安全、只读且能从现有上下文合理推断的下一步，必须直接完成；不得用‘你想先做哪项’、‘你更想看哪部分’、‘是否继续’或‘如果需要我可以继续’把它留给用户。多个安全分支都直接服务原始意图时，采用推荐默认值或合并覆盖，不得仅为减少工作量要求用户选择。\n\n只有三类真正阻塞情况可以调用 interaction.request_user：缺少只有用户才能提供的必要信息、存在无法合并的重大取舍，或继续会越过权限/任务范围边界。提问时必须如实填写 decisionKind 和 blockingReason。实际写操作仍走审批流程，不得用提问工具替代授权。\n\n收到 interaction.request_user 的回答后，回答只是解除阻塞，必须继续完成原始请求，而不是总结或复述计划后结束运行。";

#[derive(Debug)]
pub(crate) struct ProductionAdkChatRuntime {
    store: Arc<AdkStore>,
    session_store: Arc<AdkSessionStore>,
    secrets_path: PathBuf,
    cancellation_registry: Arc<RunCancellationRegistry>,
}

/// Process-local cancellation fan-out for active provider calls.
#[derive(Debug, Default)]
pub(crate) struct RunCancellationRegistry {
    active: Mutex<BTreeMap<String, Arc<AtomicBool>>>,
}

impl RunCancellationRegistry {
    fn register(&self, run_id: &str) -> Arc<AtomicBool> {
        let token = Arc::new(AtomicBool::new(false));
        if let Ok(mut active) = self.active.lock() {
            active.insert(run_id.to_owned(), Arc::clone(&token));
        }
        token
    }

    fn unregister(&self, run_id: &str) {
        if let Ok(mut active) = self.active.lock() {
            active.remove(run_id);
        }
    }

    pub(crate) fn cancel(&self, run_id: &str) -> bool {
        let token = self
            .active
            .lock()
            .ok()
            .and_then(|active| active.get(run_id).cloned());
        if let Some(token) = token {
            token.store(true, Ordering::Release);
            true
        } else {
            false
        }
    }
}

#[derive(Debug)]
enum PreparedChat {
    Existing(AdkChatPortOutput),
    New(ChatExecution),
}

#[derive(Clone, Debug)]
struct ChatExecution {
    route: AdkChatRoute,
    run_id: String,
    session_id: String,
    agent_id: String,
    request: ModelRequest,
}

fn text_field(object: &serde_json::Map<String, Value>, field: &str) -> Option<String> {
    object
        .get(field)
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
}

impl ProductionAdkChatRuntime {
    pub(crate) fn new(
        store: Arc<AdkStore>,
        session_store: Arc<AdkSessionStore>,
        settings_path: &Path,
        cancellation_registry: Arc<RunCancellationRegistry>,
    ) -> Self {
        let secrets_path = std::env::var_os("JFTRADE_ADK_SECRETS")
            .filter(|value| !value.is_empty())
            .map(PathBuf::from)
            .unwrap_or_else(|| {
                settings_path
                    .parent()
                    .filter(|parent| !parent.as_os_str().is_empty())
                    .map_or_else(
                        || PathBuf::from("secrets/adk-secrets.json"),
                        |parent| parent.join("secrets/adk-secrets.json"),
                    )
            });
        Self {
            store,
            session_store,
            secrets_path,
            cancellation_registry,
        }
    }

    fn dispatch_inner(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
        let prepared = self.prepare_chat(route, input)?;
        match prepared {
            PreparedChat::Existing(output) => Ok(output),
            PreparedChat::New(chat) => self.execute_chat(chat),
        }
    }

    fn prepare_chat(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<PreparedChat, AdkChatPortError> {
        let request: Value =
            serde_json::from_slice(&input.body).map_err(|error| AdkChatPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: format!("invalid chat payload: {error}"),
            })?;
        let object = request
            .as_object()
            .ok_or_else(|| AdkChatPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "invalid chat payload".to_owned(),
            })?;
        let session_id = text_field(object, "sessionId")
            .unwrap_or_else(|| format!("session-{}", input.client_request_id));
        let message = text_field(object, "message").ok_or_else(|| AdkChatPortError::Failed {
            status: 400,
            code: "BAD_REQUEST".to_owned(),
            message: "message is required".to_owned(),
        })?;
        let provider = self.resolve_provider(object)?;
        let agent_id = provider.agent_id.clone();
        let model = text_field(object, "model")
            .or_else(|| provider.agent_model.clone())
            .unwrap_or(provider.model.clone());
        if model.is_empty() {
            return Err(unavailable("assistant model is not configured"));
        }
        let fingerprint = fingerprint(&input.body);
        if let Some(existing) = self
            .store
            .list_runs()
            .map_err(storage_unavailable)?
            .into_iter()
            .find(|run| run.client_request_id == input.client_request_id)
        {
            if existing.request_fingerprint != fingerprint {
                return Err(AdkChatPortError::Conflict(
                    "clientRequestId was already used with a different request".to_owned(),
                ));
            }
            if let Some(response) = persisted_response(&existing.payload_json)? {
                return Ok(PreparedChat::Existing(match route {
                    AdkChatRoute::Chat => AdkChatPortOutput::Json(response),
                    AdkChatRoute::Stream => stream_from_payload(&existing.payload_json)?,
                }));
            }
        }
        let session_payload = json!({
            "id": session_id,
            "agentId": agent_id,
            "title": message.chars().take(28).collect::<String>(),
        });
        self.store
            .upsert_session(&session_id, &agent_id, &session_payload.to_string())
            .map_err(storage_unavailable)?;
        let run_id = format!("run-{}", input.client_request_id);
        let initial_payload = json!({
            "id": run_id,
            "sessionId": session_id,
            "agentId": agent_id,
            "status": "RUNNING",
            "message": "",
            "reply": "",
            "pendingApprovals": [],
            "streamId": run_id,
            "streamEvents": [],
            "providerEvents": [],
        });
        self.store
            .create_run(CreateAdkRunParams {
                id: &run_id,
                session_id: &session_id,
                agent_id: &agent_id,
                status: "RUNNING",
                client_request_id: &input.client_request_id,
                request_fingerprint: &fingerprint,
                payload_json: &initial_payload.to_string(),
            })
            .map_err(|error| {
                if error
                    .to_string()
                    .to_ascii_lowercase()
                    .contains("constraint")
                {
                    AdkChatPortError::Conflict("chat request is already running".to_owned())
                } else {
                    storage_unavailable(error)
                }
            })?;
        self.record_event(&run_id, &session_id, "user", &message)?;
        Ok(PreparedChat::New(ChatExecution {
            route,
            run_id,
            session_id,
            agent_id,
            request: ModelRequest {
                endpoint: provider.endpoint,
                api_key: provider.api_key,
                model,
                instruction: provider.instruction,
                message: message.clone(),
                timeout: provider.timeout,
            },
        }))
    }

    fn execute_chat(&self, chat: ChatExecution) -> Result<AdkChatPortOutput, AdkChatPortError> {
        if chat.route == AdkChatRoute::Stream {
            let result = execute_model_stream(chat.request.clone(), |_| Ok(()), || false);
            return self.finish_chat(&chat, result);
        }
        // Register synchronous model calls as well as streams.  The cancel
        // mutation runs on another request thread and flips this token before
        // fencing the persisted run state, so the in-flight HTTP call can be
        // interrupted without waiting for the provider timeout.
        let cancellation = self.cancellation_registry.register(&chat.run_id);
        let _guard = CancellationGuard {
            registry: Arc::clone(&self.cancellation_registry),
            run_id: chat.run_id.clone(),
        };
        if self.run_is_cancelled(&chat.run_id) {
            let error = cancellation_error();
            let _ = self.persist_cancelled(&chat, &error);
            return Err(error);
        }
        let result = execute_model(chat.request.clone(), Arc::clone(&cancellation));
        if cancellation.load(Ordering::Acquire) {
            let error = match result {
                Err(error) if is_cancellation_error(&error) => error,
                _ => cancellation_error(),
            };
            let _ = self.persist_cancelled(&chat, &error);
            return Err(error);
        }
        self.finish_chat(&chat, result)
    }

    fn resolve_provider(
        &self,
        request: &serde_json::Map<String, Value>,
    ) -> Result<ResolvedProvider, AdkChatPortError> {
        let requested = request
            .get("providerId")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty());
        let requested_agent_id = text_field(request, "agentId");
        let providers = self.store.list_providers().map_err(storage_unavailable)?;
        let (agent_id, agent_payload) = self.resolve_agent(requested_agent_id)?;
        let agent_provider = agent_payload
            .get("providerId")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty());
        let parsed_providers = providers
            .iter()
            .map(|provider| {
                serde_json::from_str::<Value>(&provider.payload_json)
                    .map(|value| (provider, value))
                    .map_err(|error| AdkChatPortError::Failed {
                        status: 500,
                        code: "ADK_STORAGE_CORRUPT".to_owned(),
                        message: format!("stored ADK provider payload is invalid JSON: {error}"),
                    })
            })
            .collect::<Result<Vec<_>, _>>()?;
        let selected = if let Some(id) = requested.or(agent_provider) {
            parsed_providers
                .iter()
                .find(|(provider, _)| provider.id == id)
                .ok_or_else(|| unavailable("assistant model provider is unavailable"))?
        } else {
            parsed_providers
                .iter()
                .find(|(_, value)| {
                    value
                        .get("default")
                        .and_then(Value::as_bool)
                        .unwrap_or(false)
                })
                .ok_or_else(|| unavailable("no assistant model provider is configured"))?
        };
        let (selected, value) = selected;
        let enabled = value
            .get("enabled")
            .and_then(Value::as_bool)
            .unwrap_or(true);
        if !enabled {
            return Err(unavailable("assistant model provider is disabled"));
        }
        let endpoint = value
            .get("baseUrl")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| unavailable("assistant model provider baseUrl is not configured"))?;
        let endpoint = responses_endpoint(endpoint).map_err(|error| AdkChatPortError::Failed {
            status: 502,
            code: "MODEL_PROVIDER_UNAVAILABLE".to_owned(),
            message: error,
        })?;
        let secrets = read_secrets(&self.secrets_path)?;
        let api_key = secrets
            .get(&selected.id)
            .cloned()
            .or_else(|| {
                value
                    .get("apiKey")
                    .and_then(Value::as_str)
                    .map(str::to_owned)
            })
            .map(|key| key.trim().to_owned())
            .filter(|key| !key.is_empty())
            .ok_or_else(|| unavailable("assistant model provider API key is not configured"))?;
        let instruction = agent_payload
            .get("instruction")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(str::to_owned)
            .or_else(|| Some(DEFAULT_BUILTIN_AGENT_INSTRUCTION.to_owned()));
        let timeout_ms = value
            .get("requestTimeoutMs")
            .and_then(Value::as_u64)
            .unwrap_or(DEFAULT_TIMEOUT_MS)
            .clamp(15_000, 600_000);
        Ok(ResolvedProvider {
            agent_id,
            endpoint,
            api_key,
            model: value
                .get("model")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .trim()
                .to_owned(),
            agent_model: agent_payload
                .get("model")
                .and_then(Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(str::to_owned),
            instruction,
            timeout: Duration::from_millis(timeout_ms),
        })
    }

    /// Resolve an explicit agent strictly, or mirror Go DefaultAgent's
    /// enabled-primary -> first-enabled -> builtin-template ordering.
    fn resolve_agent(
        &self,
        requested_agent_id: Option<String>,
    ) -> Result<(String, Value), AdkChatPortError> {
        if let Some(agent_id) = requested_agent_id {
            let entity = self
                .store
                .get_agent(&agent_id)
                .map_err(storage_unavailable)?
                .ok_or_else(|| bad_agent("agent not found"))?;
            let payload = parse_agent_payload(&entity.payload_json)?;
            if !agent_enabled(&payload) {
                return Err(bad_agent("agent is unavailable"));
            }
            return Ok((entity.id, payload));
        }

        let mut agents = self.store.list_agents().map_err(storage_unavailable)?;
        // The Go store orders by updated_at DESC, id ASC and then promotes the
        // primary builtin.  Reapply that ordering because the SQLite adapter
        // exposes the persisted entities without the Go normalization layer.
        agents.sort_by(|left, right| {
            let left_primary = left.id.eq_ignore_ascii_case(DEFAULT_BUILTIN_AGENT_ID);
            let right_primary = right.id.eq_ignore_ascii_case(DEFAULT_BUILTIN_AGENT_ID);
            right_primary
                .cmp(&left_primary)
                .then_with(|| right.updated_at.cmp(&left.updated_at))
                .then_with(|| left.id.cmp(&right.id))
        });
        let mut parsed = Vec::with_capacity(agents.len());
        for agent in agents {
            parsed.push((agent.id, parse_agent_payload(&agent.payload_json)?));
        }
        if let Some((id, payload)) = parsed.iter().find(|(id, payload)| {
            id.eq_ignore_ascii_case(DEFAULT_BUILTIN_AGENT_ID) && agent_enabled(payload)
        }) {
            return Ok((id.clone(), payload.clone()));
        }
        if let Some((id, payload)) = parsed.iter().find(|(_, payload)| agent_enabled(payload)) {
            return Ok((id.clone(), payload.clone()));
        }
        Ok((
            DEFAULT_BUILTIN_AGENT_ID.to_owned(),
            json!({
                "id": DEFAULT_BUILTIN_AGENT_ID,
                "status": "ENABLED",
                "instruction": DEFAULT_BUILTIN_AGENT_INSTRUCTION,
            }),
        ))
    }

    fn record_event(
        &self,
        run_id: &str,
        session_id: &str,
        author: &str,
        content: &str,
    ) -> Result<(), AdkChatPortError> {
        let id = format!("{run_id}:{author}");
        self.record_event_with_id(&id, run_id, session_id, author, content)
    }

    fn record_event_with_id(
        &self,
        id: &str,
        run_id: &str,
        session_id: &str,
        author: &str,
        content: &str,
    ) -> Result<(), AdkChatPortError> {
        let exists = self
            .session_store
            .list_events(session_id)
            .map(|events| events.into_iter().any(|event| event.id == id))
            .map_err(storage_unavailable)?;
        if !exists {
            self.session_store
                .record_event(RecordAdkEventParams {
                    id: &id,
                    app_name: "jftrade",
                    user_id: "local",
                    session_id,
                    invocation_id: run_id,
                    author,
                    content,
                })
                .map_err(storage_unavailable)?;
        }
        Ok(())
    }
}

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
                    let runtime = ProductionAdkChatRuntime {
                        store: stream_store,
                        session_store: stream_session_store,
                        secrets_path: stream_secrets_path,
                        cancellation_registry: Arc::clone(&cancellation_registry),
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
                let runtime = ProductionAdkChatRuntime {
                    store,
                    session_store,
                    secrets_path,
                    cancellation_registry,
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
}

#[derive(Debug)]
struct ResolvedProvider {
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
    timeout: Duration,
}

#[derive(Debug)]
struct ModelResponse {
    text: String,
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
        let body = json!({"model":request.model,"input":input,"stream":false});
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
        if text.is_empty() {
            return Err(upstream_error("assistant model returned an empty response"));
        }
        Ok(ModelResponse { text })
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
