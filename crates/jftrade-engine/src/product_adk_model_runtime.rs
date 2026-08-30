//! Runtime-owned ADK model adapter.
//!
//! The public ADK transport is intentionally synchronous (the HTTP layer turns
//! the returned snapshot into JSON/SSE).  Provider calls therefore execute on
//! a short-lived Tokio thread so an in-flight request never blocks the Axum
//! worker.  The adapter speaks the OpenAI Responses wire format used by the Go
//! runtime and keeps all persisted state in the Rust ADK stores.

use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::time::Duration;

use reqwest::{Client, Url};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};

use jftrade_store_sqlite::{AdkSessionStore, AdkStore, CreateAdkRunParams, RecordAdkEventParams};

use crate::product::product_adk_chat_stream_port::{
    AdkChatInput, AdkChatPortError, AdkChatPortOutput, AdkChatRoute, AdkChatStreamFrame,
    AdkChatStreamPort, AdkChatStreamSnapshot,
};

const MAX_RESPONSE_BYTES: usize = 4 << 20;
const DEFAULT_TIMEOUT_MS: u64 = 120_000;

#[derive(Debug)]
pub(crate) struct ProductionAdkChatRuntime {
    store: Arc<AdkStore>,
    session_store: Arc<AdkSessionStore>,
    secrets_path: PathBuf,
}

impl ProductionAdkChatRuntime {
    pub(crate) fn new(
        store: Arc<AdkStore>,
        session_store: Arc<AdkSessionStore>,
        settings_path: &Path,
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
        }
    }

    fn dispatch_inner(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError> {
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
        let agent_id = object
            .get("agentId")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .unwrap_or("jftrade-default")
            .to_owned();
        let session_id = object
            .get("sessionId")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(str::to_owned)
            .unwrap_or_else(|| format!("session-{}", input.client_request_id));
        let message = object
            .get("message")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| AdkChatPortError::Failed {
                status: 400,
                code: "BAD_REQUEST".to_owned(),
                message: "message is required".to_owned(),
            })?
            .to_owned();
        let provider = self.resolve_provider(object)?;
        let model = object
            .get("model")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(str::to_owned)
            .or_else(|| provider.agent_model.clone())
            .unwrap_or_else(|| provider.model.clone());
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
                return Ok(match route {
                    AdkChatRoute::Chat => AdkChatPortOutput::Json(response),
                    AdkChatRoute::Stream => stream_from_payload(&existing.payload_json)?,
                });
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

        let request = ModelRequest {
            endpoint: provider.endpoint,
            api_key: provider.api_key,
            model,
            instruction: provider.instruction,
            message,
            timeout: provider.timeout,
        };
        let result = execute_model(request);
        match result {
            Ok(model_response) => {
                let response = self.persist_success(
                    &run_id,
                    &session_id,
                    &agent_id,
                    &input.client_request_id,
                    model_response,
                )?;
                if route == AdkChatRoute::Chat {
                    Ok(AdkChatPortOutput::Json(response))
                } else {
                    let run = self
                        .store
                        .get_run(&run_id)
                        .map_err(storage_unavailable)?
                        .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
                    stream_from_payload(&run.payload_json)
                }
            }
            Err(error) => {
                let _ = self.persist_failure(&run_id, &error);
                Err(error)
            }
        }
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
        let agent_id = request
            .get("agentId")
            .and_then(Value::as_str)
            .map(str::trim)
            .filter(|value| !value.is_empty());
        let providers = self.store.list_providers().map_err(storage_unavailable)?;
        let agent = match agent_id {
            Some(id) => self.store.get_agent(id).map_err(storage_unavailable)?,
            None => None,
        };
        let agent_payload = agent
            .as_ref()
            .map(|agent| {
                serde_json::from_str::<Value>(&agent.payload_json).map_err(|error| {
                    AdkChatPortError::Failed {
                        status: 500,
                        code: "ADK_STORAGE_CORRUPT".to_owned(),
                        message: format!("stored ADK agent payload is invalid JSON: {error}"),
                    }
                })
            })
            .transpose()?;
        let agent_provider = agent_payload
            .as_ref()
            .and_then(|agent| agent.get("providerId").and_then(Value::as_str));
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
        let selected = requested
            .or(agent_provider)
            .and_then(|id| {
                parsed_providers
                    .iter()
                    .find(|(provider, _)| provider.id == id)
            })
            .or_else(|| {
                parsed_providers.iter().find(|(_, value)| {
                    value
                        .get("default")
                        .and_then(Value::as_bool)
                        .unwrap_or(false)
                })
            })
            .or_else(|| {
                parsed_providers
                    .iter()
                    .find(|(provider, _)| !provider.payload_json.is_empty())
            });
        let Some((selected, value)) = selected else {
            return Err(unavailable("no assistant model provider is configured"));
        };
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
        let instruction = agent_payload.as_ref().and_then(|agent| {
            agent
                .get("instruction")
                .and_then(Value::as_str)
                .map(str::to_owned)
        });
        let timeout_ms = value
            .get("requestTimeoutMs")
            .and_then(Value::as_u64)
            .unwrap_or(DEFAULT_TIMEOUT_MS)
            .clamp(15_000, 600_000);
        Ok(ResolvedProvider {
            endpoint,
            api_key,
            model: value
                .get("model")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .trim()
                .to_owned(),
            agent_model: agent_payload.as_ref().and_then(|agent| {
                agent
                    .get("model")
                    .and_then(Value::as_str)
                    .map(str::to_owned)
            }),
            instruction,
            timeout: Duration::from_millis(timeout_ms),
        })
    }

    fn record_event(
        &self,
        run_id: &str,
        session_id: &str,
        author: &str,
        content: &str,
    ) -> Result<(), AdkChatPortError> {
        let id = format!("{run_id}:{author}");
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

    fn persist_success(
        &self,
        run_id: &str,
        session_id: &str,
        agent_id: &str,
        _client_request_id: &str,
        model_response: ModelResponse,
    ) -> Result<Value, AdkChatPortError> {
        let run = self
            .store
            .get_run(run_id)
            .map_err(storage_unavailable)?
            .ok_or_else(|| unavailable("persisted ADK run disappeared"))?;
        let now = run.updated_at.clone();
        let session = self
            .store
            .get_session(session_id)
            .map_err(storage_unavailable)?
            .map(|session| json!({"id": session.id, "agentId": agent_id, "createdAt": session.created_at, "updatedAt": session.updated_at}))
            .unwrap_or_else(|| json!({"id": session_id, "agentId": agent_id}));
        let text = model_response.text;
        self.record_event(run_id, session_id, agent_id, &text)?;
        let run_value = json!({
            "id": run_id,
            "sessionId": session_id,
            "agentId": agent_id,
            "status": "COMPLETED",
            "message": text.clone(),
            "reply": text.clone(),
            "pendingApprovals": [],
            "createdAt": run.created_at,
            "updatedAt": now,
        });
        let timeline = json!({
            "id": format!("{run_id}:assistant"),
            "kind": "assistant_message",
            "status": "final",
            "text": text.clone(),
        });
        let response = json!({
            "reply": text.clone(),
            "session": session,
            "run": run_value,
            "pendingApprovals": [],
            "timeline": [timeline],
        });
        let stream_events = vec![
            json!({"type":"run","streamId":run_id,"sequence":1,"runId":run_id,"run":{"id":run_id,"status":"RUNNING"}}),
            json!({"type":"timeline","streamId":run_id,"sequence":2,"runId":run_id,"timeline":timeline}),
            json!({"type":"final","streamId":run_id,"sequence":3,"runId":run_id,"response":response}),
        ];
        let payload = json!({
            "id": run_id,
            "sessionId": session_id,
            "agentId": agent_id,
            "status": "COMPLETED",
            "reply": text,
            "response": response,
            "streamId": run_id,
            "streamEvents": stream_events,
        });
        self.store
            .update_run_payload(run_id, &payload.to_string())
            .map_err(storage_unavailable)?;
        self.store
            .update_run_status(run_id, "COMPLETED")
            .map_err(storage_unavailable)?;
        Ok(response)
    }

    fn persist_failure(
        &self,
        run_id: &str,
        error: &AdkChatPortError,
    ) -> Result<(), AdkChatPortError> {
        let message = match error {
            AdkChatPortError::Unavailable(message) | AdkChatPortError::Conflict(message) => {
                message.clone()
            }
            AdkChatPortError::Failed { code, message, .. } => format!("{code}: {message}"),
        };
        self.store
            .update_run_payload(
                run_id,
                &json!({"id":run_id,"status":"FAILED","message":message}).to_string(),
            )
            .map_err(storage_unavailable)?;
        self.store
            .update_run_status(run_id, "FAILED")
            .map_err(storage_unavailable)?;
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
        let input = input.clone();
        std::thread::Builder::new()
            .name("jftrade-adk-model".to_owned())
            .spawn(move || {
                let runtime = ProductionAdkChatRuntime {
                    store,
                    session_store,
                    secrets_path,
                };
                runtime.dispatch_inner(route, &input)
            })
            .map_err(|error| AdkChatPortError::Unavailable(error.to_string()))?
            .join()
            .map_err(|_| unavailable("assistant model runtime panicked"))?
    }
}

#[derive(Debug)]
struct ResolvedProvider {
    endpoint: Url,
    api_key: String,
    model: String,
    agent_model: Option<String>,
    instruction: Option<String>,
    timeout: Duration,
}

#[derive(Debug)]
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

fn execute_model(request: ModelRequest) -> Result<ModelResponse, AdkChatPortError> {
    // reqwest is built with rustls-no-provider so the engine can use the same
    // ring-backed crypto provider as the desktop runtime without relying on a
    // process-global provider installed by an unrelated integration crate.
    let _ = rustls::crypto::ring::default_provider().install_default();
    let runtime = tokio::runtime::Builder::new_current_thread()
        .enable_all()
        .build()
        .map_err(|error| unavailable(format!("assistant model runtime unavailable: {error}")))?;
    runtime.block_on(async move {
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
        let response = client
            .post(request.endpoint)
            .bearer_auth(request.api_key)
            .header("content-type", "application/json")
            .json(&body)
            .send()
            .await
            .map_err(|error| {
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
        let status = response.status();
        let bytes = response
            .bytes()
            .await
            .map_err(|error| upstream_error(error.to_string()))?;
        if bytes.len() > MAX_RESPONSE_BYTES {
            return Err(upstream_error(
                "assistant model response exceeded size limit",
            ));
        }
        let value: Value = serde_json::from_slice(&bytes)
            .map_err(|error| upstream_error(format!("decode model response: {error}")))?;
        if !status.is_success() {
            let message = value
                .pointer("/error/message")
                .and_then(Value::as_str)
                .unwrap_or("assistant model provider rejected the request");
            return Err(AdkChatPortError::Failed {
                status: 502,
                code: "MODEL_CALL_FAILED".to_owned(),
                message: message.to_owned(),
            });
        }
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

fn storage_unavailable(error: impl std::fmt::Display) -> AdkChatPortError {
    unavailable(format!("ADK storage unavailable: {error}"))
}
