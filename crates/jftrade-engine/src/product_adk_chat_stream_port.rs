use std::collections::BTreeMap;

use jftrade_api::ApiStream;
use serde::Deserialize;
use serde_json::{Value, json};

pub const ADK_CHAT_PATH: &str = "/api/v1/adk/chat";
pub const ADK_CHAT_STREAM_PATH: &str = "/api/v1/adk/chat/stream";
pub const ADK_STREAM_RETRY_MILLIS: u64 = 3000;

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum AdkChatRoute {
    Chat,
    Stream,
}

impl AdkChatRoute {
    fn from_request(method: &str, path: &str) -> Option<Self> {
        if method != "POST" {
            return None;
        }
        match path {
            ADK_CHAT_PATH => Some(Self::Chat),
            ADK_CHAT_STREAM_PATH => Some(Self::Stream),
            _ => None,
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AdkChatRequest {
    pub method: String,
    pub path: String,
    pub body: Vec<u8>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AdkChatInput {
    pub body: Vec<u8>,
    pub client_request_id: String,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum AdkChatStreamFrame {
    Event { id: Option<String>, data: Value },
    Comment(String),
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AdkChatStreamSnapshot {
    pub headers: BTreeMap<String, String>,
    pub frames: Vec<AdkChatStreamFrame>,
    pub terminal: bool,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AdkChatLiveStream {
    pub headers: BTreeMap<String, String>,
    pub stream: ApiStream,
}

#[allow(dead_code)]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum AdkChatPortOutput {
    Json(Value),
    Stream(AdkChatStreamSnapshot),
    LiveStream(AdkChatLiveStream),
}

#[allow(dead_code)]
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum AdkChatPortError {
    Unavailable(String),
    Conflict(String),
    Failed {
        status: u16,
        code: String,
        message: String,
    },
}

pub trait AdkChatStreamPort: Send + Sync + std::fmt::Debug {
    fn dispatch(
        &self,
        route: AdkChatRoute,
        input: &AdkChatInput,
    ) -> Result<AdkChatPortOutput, AdkChatPortError>;

    /// Signals an active provider call to stop.  Implementations that do not
    /// own a live runtime may keep the default no-op behavior.
    fn cancel_run(&self, _run_id: &str) -> bool {
        false
    }

    /// Schedules a durable approval continuation.  Production runtimes must
    /// override this; the default keeps fixture ports source-compatible.
    fn resume_approval(&self, _run_id: &str) -> Result<(), AdkChatPortError> {
        Err(AdkChatPortError::Unavailable(
            "assistant approval continuation is unavailable".to_owned(),
        ))
    }

    /// Reports whether a concrete model/runtime configuration is available
    /// for model-backed mutations without dispatching a request.
    fn runtime_ready(&self) -> bool {
        false
    }

    /// Stops provider calls and joins any continuation workers owned by this
    /// port.  The default is intentionally a no-op for stateless rehearsal
    /// ports; production adapters override it so their SQLite stores are not
    /// released while a background continuation still holds an `Arc`.
    fn shutdown(&self) {}
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub enum AdkChatWireResponse {
    Json {
        status: u16,
        headers: BTreeMap<String, String>,
        body: Value,
    },
    Sse {
        status: u16,
        headers: BTreeMap<String, String>,
        frames: Vec<AdkChatStreamFrame>,
        terminal: bool,
    },
    LiveSse {
        status: u16,
        headers: BTreeMap<String, String>,
        stream: ApiStream,
    },
}

impl AdkChatWireResponse {
    pub fn status(&self) -> u16 {
        match self {
            Self::Json { status, .. } | Self::Sse { status, .. } | Self::LiveSse { status, .. } => {
                *status
            }
        }
    }

    pub fn headers(&self) -> &BTreeMap<String, String> {
        match self {
            Self::Json { headers, .. }
            | Self::Sse { headers, .. }
            | Self::LiveSse { headers, .. } => headers,
        }
    }

    pub fn body(&self) -> String {
        match self {
            Self::Json { body, .. } => {
                serde_json::to_string(body).expect("chat JSON envelope is serializable")
            }
            Self::Sse { frames, .. } => encode_sse_frames(frames),
            Self::LiveSse { .. } => String::new(),
        }
    }
}

pub fn dispatch_adk_chat(
    request: &AdkChatRequest,
    port: Option<&dyn AdkChatStreamPort>,
    timestamp: &str,
    stream_idle_timeout_ms: u64,
) -> AdkChatWireResponse {
    let Some(route) = AdkChatRoute::from_request(&request.method, &request.path) else {
        return json_error(
            404,
            "NOT_FOUND",
            &format!("unknown endpoint {}", request.path),
            timestamp,
        );
    };
    let input = match decode_input(&request.body) {
        Ok(input) => input,
        Err(InputDecodeError::Payload(message)) => {
            if route == AdkChatRoute::Stream {
                return invalid_stream_response(message, stream_idle_timeout_ms);
            }
            return json_error(400, "BAD_REQUEST", "invalid chat payload", timestamp);
        }
        Err(InputDecodeError::Identity(message)) => {
            let response = json_error(400, "BAD_REQUEST", &message, timestamp);
            return add_stream_idle_header(route, response, stream_idle_timeout_ms);
        }
    };
    let Some(port) = port else {
        return json_error(
            503,
            "ADK_UNAVAILABLE",
            "ADK runtime is unavailable",
            timestamp,
        );
    };
    let output = match port.dispatch(route, &input) {
        Ok(output) => output,
        Err(error) => {
            let response = port_error_response(error, timestamp);
            return add_stream_idle_header(route, response, stream_idle_timeout_ms);
        }
    };
    match (route, output) {
        (AdkChatRoute::Chat, AdkChatPortOutput::Json(data)) => json_success(data, timestamp),
        (AdkChatRoute::Stream, AdkChatPortOutput::Stream(snapshot)) => {
            stream_success(snapshot, stream_idle_timeout_ms)
        }
        (AdkChatRoute::Stream, AdkChatPortOutput::LiveStream(stream)) => {
            live_stream_success(stream, stream_idle_timeout_ms)
        }
        (_, _) => json_error(
            500,
            "ADK_CHAT_FAILED",
            "ADK chat port returned an invalid response",
            timestamp,
        ),
    }
}

enum InputDecodeError {
    Payload(String),
    Identity(String),
}

fn decode_input(body: &[u8]) -> Result<AdkChatInput, InputDecodeError> {
    let mut deserializer = serde_json::Deserializer::from_slice(body);
    let value = Value::deserialize(&mut deserializer).map_err(|error| {
        if body.is_empty() {
            InputDecodeError::Payload("EOF".to_owned())
        } else if body == b"{" {
            InputDecodeError::Payload("unexpected EOF".to_owned())
        } else if body.first() == Some(&b'[') {
            InputDecodeError::Payload(
                "json: cannot unmarshal array into Go value of type assistant.ADKChatRequest"
                    .to_owned(),
            )
        } else {
            InputDecodeError::Payload(error.to_string())
        }
    })?;
    let object = match value {
        Value::Null => serde_json::Map::new(),
        Value::Object(object) => object,
        _ => {
            return Err(InputDecodeError::Payload(
                "json: cannot unmarshal value into Go value of type assistant.ADKChatRequest"
                    .to_owned(),
            ));
        }
    };
    if object
        .get("clientRequestId")
        .is_some_and(|value| !value.is_null() && !value.is_string())
    {
        return Err(InputDecodeError::Payload(
            "json: cannot unmarshal value into Go struct field ADKChatRequest.clientRequestId of type string"
                .to_owned(),
        ));
    }
    let client_request_id = object
        .get("clientRequestId")
        .and_then(Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| {
            InputDecodeError::Identity("clientRequestId must be a valid UUID".to_owned())
        })?;
    let client_request_id = canonical_uuid(client_request_id).ok_or_else(|| {
        InputDecodeError::Identity("clientRequestId must be a valid UUID".to_owned())
    })?;
    Ok(AdkChatInput {
        body: body.to_vec(),
        client_request_id,
    })
}

fn canonical_uuid(value: &str) -> Option<String> {
    let value = value
        .strip_prefix("urn:uuid:")
        .or_else(|| value.strip_prefix("URN:UUID:"))
        .unwrap_or(value)
        .trim_matches(['{', '}']);
    let compact = value.replace('-', "");
    if compact.len() != 32 || !compact.bytes().all(|byte| byte.is_ascii_hexdigit()) {
        return None;
    }
    let lower = compact.to_ascii_lowercase();
    Some(format!(
        "{}-{}-{}-{}-{}",
        &lower[0..8],
        &lower[8..12],
        &lower[12..16],
        &lower[16..20],
        &lower[20..32]
    ))
}

fn json_success(data: Value, timestamp: &str) -> AdkChatWireResponse {
    AdkChatWireResponse::Json {
        status: 200,
        headers: json_headers(),
        body: json!({"ok": true, "data": data, "timestamp": timestamp}),
    }
}

fn json_error(status: u16, code: &str, message: &str, timestamp: &str) -> AdkChatWireResponse {
    AdkChatWireResponse::Json {
        status,
        headers: json_headers(),
        body: json!({
            "ok": false,
            "error": {"code": code, "message": message},
            "timestamp": timestamp,
        }),
    }
}

fn port_error_response(error: AdkChatPortError, timestamp: &str) -> AdkChatWireResponse {
    match error {
        AdkChatPortError::Unavailable(message) => {
            json_error(503, "ADK_UNAVAILABLE", &message, timestamp)
        }
        AdkChatPortError::Conflict(message) => {
            json_error(409, "ADK_CHAT_IDEMPOTENCY_CONFLICT", &message, timestamp)
        }
        AdkChatPortError::Failed {
            status,
            code,
            message,
        } => json_error(status, &code, &message, timestamp),
    }
}

fn add_stream_idle_header(
    route: AdkChatRoute,
    mut response: AdkChatWireResponse,
    stream_idle_timeout_ms: u64,
) -> AdkChatWireResponse {
    if route == AdkChatRoute::Stream {
        match &mut response {
            AdkChatWireResponse::Json { headers, .. }
            | AdkChatWireResponse::Sse { headers, .. }
            | AdkChatWireResponse::LiveSse { headers, .. } => {
                headers.insert(
                    "X-ADK-Stream-Idle-Timeout-Ms".to_owned(),
                    stream_idle_timeout_ms.to_string(),
                );
            }
        }
    }
    response
}

fn invalid_stream_response(message: String, stream_idle_timeout_ms: u64) -> AdkChatWireResponse {
    let mut headers = sse_headers(stream_idle_timeout_ms);
    headers.remove("X-ADK-Stream-ID");
    AdkChatWireResponse::Sse {
        status: 200,
        headers,
        frames: vec![
            AdkChatStreamFrame::Comment(format!("retry: {}", ADK_STREAM_RETRY_MILLIS)),
            AdkChatStreamFrame::Event {
                id: None,
                data: json!({
                    "type": "error",
                    "message": format!("invalid chat payload: {message}"),
                }),
            },
        ],
        terminal: true,
    }
}

fn stream_success(
    snapshot: AdkChatStreamSnapshot,
    stream_idle_timeout_ms: u64,
) -> AdkChatWireResponse {
    let mut headers = sse_headers(stream_idle_timeout_ms);
    headers.extend(snapshot.headers);
    let mut frames = vec![AdkChatStreamFrame::Comment(format!(
        "retry: {}",
        ADK_STREAM_RETRY_MILLIS
    ))];
    frames.extend(snapshot.frames);
    AdkChatWireResponse::Sse {
        status: 200,
        headers,
        frames,
        terminal: snapshot.terminal,
    }
}

fn live_stream_success(
    stream: AdkChatLiveStream,
    stream_idle_timeout_ms: u64,
) -> AdkChatWireResponse {
    let mut headers = sse_headers(stream_idle_timeout_ms);
    headers.extend(stream.headers);
    AdkChatWireResponse::LiveSse {
        status: 200,
        headers,
        stream: stream.stream,
    }
}

fn json_headers() -> BTreeMap<String, String> {
    BTreeMap::from([(
        "Content-Type".to_owned(),
        "application/json; charset=utf-8".to_owned(),
    )])
}

fn sse_headers(stream_idle_timeout_ms: u64) -> BTreeMap<String, String> {
    BTreeMap::from([
        ("Cache-Control".to_owned(), "no-cache".to_owned()),
        ("Connection".to_owned(), "keep-alive".to_owned()),
        ("Content-Type".to_owned(), "text/event-stream".to_owned()),
        (
            "X-ADK-Stream-Idle-Timeout-Ms".to_owned(),
            stream_idle_timeout_ms.to_string(),
        ),
    ])
}

fn encode_sse_frames(frames: &[AdkChatStreamFrame]) -> String {
    let mut body = String::new();
    for frame in frames {
        match frame {
            AdkChatStreamFrame::Event { id, data } => {
                if let Some(id) = id {
                    body.push_str("id: ");
                    body.push_str(id);
                    body.push('\n');
                }
                body.push_str("data: ");
                body.push_str(
                    &serde_json::to_string(data).expect("chat SSE event is serializable"),
                );
                body.push_str("\n\n");
            }
            AdkChatStreamFrame::Comment(comment) => {
                if let Some(retry) = comment.strip_prefix("retry: ") {
                    body.push_str("retry: ");
                    body.push_str(retry);
                    body.push_str("\n\n");
                } else {
                    body.push_str(": ");
                    body.push_str(comment);
                    body.push_str("\n\n");
                }
            }
        }
    }
    body
}

#[cfg(test)]
mod tests {
    use super::*;

    #[derive(Debug)]
    struct FixedPort(Result<AdkChatPortOutput, AdkChatPortError>);

    impl AdkChatStreamPort for FixedPort {
        fn dispatch(
            &self,
            _route: AdkChatRoute,
            _input: &AdkChatInput,
        ) -> Result<AdkChatPortOutput, AdkChatPortError> {
            self.0.clone()
        }
    }

    fn request(path: &str) -> AdkChatRequest {
        AdkChatRequest {
            method: "POST".to_owned(),
            path: path.to_owned(),
            body: br#"{"clientRequestId":"11111111-1111-4111-8111-111111111111"}"#.to_vec(),
        }
    }

    #[test]
    fn adk_port_outputs_and_errors_keep_route_wire_mapping() {
        let chat_port = FixedPort(Ok(AdkChatPortOutput::Json(json!({
            "reply": "ok"
        }))));
        let chat = dispatch_adk_chat(
            &request(ADK_CHAT_PATH),
            Some(&chat_port),
            "fixture-time",
            420_000,
        );
        assert_eq!(chat.status(), 200);
        assert!(chat.body().contains("\"reply\":\"ok\""));

        let stream_port = FixedPort(Ok(AdkChatPortOutput::Stream(AdkChatStreamSnapshot {
            headers: BTreeMap::new(),
            frames: vec![AdkChatStreamFrame::Event {
                id: Some("1".to_owned()),
                data: json!({"type": "final"}),
            }],
            terminal: true,
        })));
        let stream = dispatch_adk_chat(
            &request(ADK_CHAT_STREAM_PATH),
            Some(&stream_port),
            "fixture-time",
            420_000,
        );
        assert_eq!(stream.status(), 200);
        assert!(stream.body().contains("id: 1\n"));

        let unavailable_port = FixedPort(Err(AdkChatPortError::Unavailable(
            "runtime unavailable".to_owned(),
        )));
        let unavailable = dispatch_adk_chat(
            &request(ADK_CHAT_PATH),
            Some(&unavailable_port),
            "fixture-time",
            420_000,
        );
        assert_eq!(unavailable.status(), 503);

        let conflict_port = FixedPort(Err(AdkChatPortError::Conflict(
            "duplicate request".to_owned(),
        )));
        let conflict = dispatch_adk_chat(
            &request(ADK_CHAT_PATH),
            Some(&conflict_port),
            "fixture-time",
            420_000,
        );
        assert_eq!(conflict.status(), 409);

        let failed_port = FixedPort(Err(AdkChatPortError::Failed {
            status: 502,
            code: "MODEL_CALL_FAILED".to_owned(),
            message: "provider failed".to_owned(),
        }));
        let failed = dispatch_adk_chat(
            &request(ADK_CHAT_PATH),
            Some(&failed_port),
            "fixture-time",
            420_000,
        );
        assert_eq!(failed.status(), 502);
    }
}
