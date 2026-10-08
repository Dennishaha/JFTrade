//! One Responses SSE reader shared by synchronous and live model calls.

use eventsource_stream::{EventStreamError, Eventsource};
use serde_json::Value;
use std::time::Duration;
use tokio_stream::StreamExt;

use super::super::{
    MAX_RESPONSE_BYTES, ModelResponse, extract_text, extract_tool_calls, upstream_error,
};
use super::{client_disconnected, model_request_error};
use crate::product::product_adk_chat_stream_port::AdkChatPortError;

pub(in super::super) async fn read_response_events(
    response: reqwest::Response,
    timeout: Duration,
    mut on_event: impl FnMut(&Value) -> Result<(), AdkChatPortError>,
    is_cancelled: &impl Fn() -> bool,
) -> Result<ModelResponse, AdkChatPortError> {
    let mut stream = response.bytes_stream().eventsource();
    let mut state = ResponseEvents {
        bytes_seen: 0,
        completed: false,
        response: ModelResponse {
            text: String::new(),
            tool_calls: Vec::new(),
            usage_metadata: None,
        },
    };
    loop {
        if is_cancelled() {
            return Err(client_disconnected());
        }
        let event = tokio::select! {
            item = stream.next() => match item {
                Some(Ok(event)) => Some(event),
                Some(Err(error)) => return Err(match error {
                    EventStreamError::Transport(error) => model_request_error(error,timeout),
                    other => upstream_error(format!("decode model stream event: {other}")),
                }),
                None => None,
            },
            _ = tokio::time::sleep(Duration::from_millis(250)) => {
                if is_cancelled() { return Err(client_disconnected()); }
                continue;
            }
        };
        let Some(event) = event else { break };
        state.bytes_seen = state.bytes_seen.saturating_add(event.data.len());
        if state.bytes_seen > MAX_RESPONSE_BYTES {
            return Err(upstream_error(
                "assistant model response exceeded size limit",
            ));
        }
        let data = event.data.trim();
        if data.is_empty() || data == "[DONE]" {
            continue;
        }
        let value: Value = serde_json::from_str(data)
            .map_err(|error| upstream_error(format!("decode model stream event: {error}")))?;
        on_event(&value)?;
        state.consume(&value)?;
        if state.completed {
            break;
        }
    }
    state.finish()
}

struct ResponseEvents {
    bytes_seen: usize,
    completed: bool,
    response: ModelResponse,
}

impl ResponseEvents {
    fn consume(&mut self, value: &Value) -> Result<(), AdkChatPortError> {
        match value
            .get("type")
            .and_then(Value::as_str)
            .unwrap_or_default()
        {
            "response.output_text.delta" => {
                if let Some(delta) = value.get("delta").and_then(Value::as_str) {
                    self.response.text.push_str(delta);
                }
            }
            "response.completed" => {
                if let Some(response) = value.get("response") {
                    self.response.usage_metadata = response
                        .get("usage")
                        .filter(|value| value.is_object())
                        .cloned();
                    let text = extract_text(response);
                    if !text.trim().is_empty() {
                        self.response.text = text;
                    }
                    self.response.tool_calls = extract_tool_calls(response)?;
                }
                self.completed = true;
            }
            "response.failed" | "error" => {
                let message = value
                    .pointer("/error/message")
                    .or_else(|| value.pointer("/response/error/message"))
                    .and_then(Value::as_str)
                    .unwrap_or("assistant model stream failed");
                return Err(upstream_error(message));
            }
            _ => {}
        }
        Ok(())
    }

    fn finish(mut self) -> Result<ModelResponse, AdkChatPortError> {
        if !self.completed {
            return Err(upstream_error(
                "assistant model stream ended before response.completed",
            ));
        }
        self.response.text = self.response.text.trim().to_owned();
        if self.response.text.is_empty() && self.response.tool_calls.is_empty() {
            return Err(upstream_error("assistant model returned an empty response"));
        }
        Ok(self.response)
    }
}
