//! OpenAI Responses SSE adapter used by the production ADK chat runtime.

use reqwest::{Client, StatusCode};
use serde_json::{Value, json};
use std::time::Duration;

use super::{
    MAX_RESPONSE_BYTES, ModelRequest, ModelResponse, extract_text, extract_tool_calls, model_input,
    unavailable, upstream_error,
};
use crate::product::product_adk_chat_stream_port::AdkChatPortError;

/// Read one Responses request and invoke `on_event` for each complete SSE
/// event before returning.  The callback is deliberately synchronous so the
/// runtime can durably append each event before reading the next chunk.
pub(super) fn execute_model_stream<F>(
    request: ModelRequest,
    mut on_event: F,
    is_cancelled: impl Fn() -> bool,
) -> Result<ModelResponse, AdkChatPortError>
where
    F: FnMut(&Value) -> Result<(), AdkChatPortError>,
{
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
        let input = model_input(&request);
        let mut body = json!({"model":request.model,"input":input,"stream":true});
        if !request.tools.is_empty() {
            body["tools"] = Value::Array(request.tools.clone());
        }
        let response = client
            .post(request.endpoint)
            .bearer_auth(request.api_key)
            .header("content-type", "application/json")
            .json(&body)
            .send()
            .await
            .map_err(|error| model_request_error(error, request.timeout))?;
        let status = response.status();
        let retry_after = response
            .headers()
            .get(reqwest::header::RETRY_AFTER)
            .and_then(|value| value.to_str().ok())
            .map(str::to_owned);
        let content_type = response
            .headers()
            .get(reqwest::header::CONTENT_TYPE)
            .and_then(|value| value.to_str().ok())
            .unwrap_or_default()
            .to_ascii_lowercase();
        if !status.is_success() {
            let bytes = bounded_response(response, MAX_RESPONSE_BYTES).await?;
            return Err(provider_rejection(status, retry_after.as_deref(), &bytes));
        }
        if !content_type.contains("text/event-stream") {
            let bytes = bounded_response(response, MAX_RESPONSE_BYTES).await?;
            let value: Value = serde_json::from_slice(&bytes)
                .map_err(|error| upstream_error(format!("decode model response: {error}")))?;
            on_event(&value)?;
            let text = extract_text(&value).trim().to_owned();
            let tool_calls = extract_tool_calls(&value)?;
            if text.is_empty() && tool_calls.is_empty() {
                return Err(upstream_error("assistant model returned an empty response"));
            }
            return Ok(ModelResponse { text, tool_calls });
        }

        let mut response = response;
        let mut decoder = SseDecoder::default();
        let mut text = String::new();
        let mut tool_calls = Vec::new();
        let mut completed = false;
        let mut bytes_seen = 0usize;
        loop {
            if is_cancelled() {
                return Err(AdkChatPortError::Failed {
                    status: 499,
                    code: "CLIENT_DISCONNECTED".to_owned(),
                    message: "assistant chat client disconnected".to_owned(),
                });
            }
            let chunk = tokio::select! {
                result = response.chunk() => result.map_err(|error| model_request_error(error, request.timeout))?,
                _ = tokio::time::sleep(Duration::from_millis(250)) => {
                    if is_cancelled() {
                        return Err(AdkChatPortError::Failed {
                            status: 499,
                            code: "CLIENT_DISCONNECTED".to_owned(),
                            message: "assistant chat client disconnected".to_owned(),
                        });
                    }
                    continue;
                }
            };
            let Some(chunk) = chunk else { break };
            bytes_seen = bytes_seen.saturating_add(chunk.len());
            if bytes_seen > MAX_RESPONSE_BYTES {
                return Err(upstream_error("assistant model response exceeded size limit"));
            }
            let chunk = std::str::from_utf8(&chunk)
                .map_err(|error| upstream_error(format!("decode model stream bytes: {error}")))?;
            decoder.push(chunk, |value| {
                on_event(value)?;
                match value.get("type").and_then(Value::as_str).unwrap_or_default() {
                    "response.output_text.delta" => {
                        if let Some(delta) = value.get("delta").and_then(Value::as_str) {
                            text.push_str(delta);
                        }
                    }
                    "response.completed" => {
                        if let Some(response) = value.get("response") {
                            let completed_text = extract_text(response);
                            if !completed_text.trim().is_empty() {
                                text = completed_text;
                            }
                            tool_calls = extract_tool_calls(response)?;
                        }
                        completed = true;
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
            })?;
            if completed {
                break;
            }
        }
        if !completed {
            decoder.finish(|value| {
                on_event(value)?;
                if value.get("type").and_then(Value::as_str) == Some("response.completed") {
                    completed = true;
                    let completed_text = value
                        .get("response")
                        .map(extract_text)
                        .unwrap_or_default();
                    if !completed_text.trim().is_empty() {
                        text = completed_text;
                    }
                    if let Some(response) = value.get("response") {
                        tool_calls = extract_tool_calls(response)?;
                    }
                }
                Ok(())
            })?;
        }
        if !completed {
            return Err(upstream_error(
                "assistant model stream ended before response.completed",
            ));
        }
        let text = text.trim().to_owned();
        if text.is_empty() && tool_calls.is_empty() {
            return Err(upstream_error("assistant model returned an empty response"));
        }
        Ok(ModelResponse { text, tool_calls })
    })
}

fn model_request_error(error: reqwest::Error, timeout: Duration) -> AdkChatPortError {
    if error.is_timeout() {
        return AdkChatPortError::Failed {
            status: 504,
            code: "MODEL_CALL_TIMEOUT".to_owned(),
            message: format!(
                "assistant model request timed out after {} ms",
                timeout.as_millis()
            ),
        };
    }
    upstream_error(error.to_string())
}

async fn bounded_response(
    response: reqwest::Response,
    limit: usize,
) -> Result<Vec<u8>, AdkChatPortError> {
    let bytes = response
        .bytes()
        .await
        .map_err(|error| upstream_error(error.to_string()))?;
    if bytes.len() > limit {
        return Err(upstream_error(
            "assistant model response exceeded size limit",
        ));
    }
    Ok(bytes.to_vec())
}

fn provider_rejection(
    status: StatusCode,
    retry_after: Option<&str>,
    bytes: &[u8],
) -> AdkChatPortError {
    let value = serde_json::from_slice::<Value>(bytes).unwrap_or(Value::Null);
    let message = value
        .pointer("/error/message")
        .and_then(Value::as_str)
        .unwrap_or("assistant model provider rejected the request");
    let (code, status_code) = match status {
        // Keep upstream auth/permission failures outside the local browser
        // auth boundary; sync and SSE use the same external-dependency map.
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

#[derive(Default)]
struct SseDecoder {
    buffer: String,
}

impl SseDecoder {
    fn push<F>(&mut self, chunk: &str, mut on_event: F) -> Result<(), AdkChatPortError>
    where
        F: FnMut(&Value) -> Result<(), AdkChatPortError>,
    {
        self.buffer.push_str(chunk);
        while let Some((index, separator_len)) = event_boundary(&self.buffer) {
            let frame: String = self.buffer.drain(..index + separator_len).collect();
            decode_frame(&frame, &mut on_event)?;
        }
        Ok(())
    }

    fn finish<F>(&mut self, mut on_event: F) -> Result<(), AdkChatPortError>
    where
        F: FnMut(&Value) -> Result<(), AdkChatPortError>,
    {
        if self.buffer.trim().is_empty() {
            return Ok(());
        }
        let frame = std::mem::take(&mut self.buffer);
        decode_frame(&frame, &mut on_event)
    }
}

fn event_boundary(buffer: &str) -> Option<(usize, usize)> {
    let lf = buffer.find("\n\n").map(|index| (index, 2));
    let crlf = buffer.find("\r\n\r\n").map(|index| (index, 4));
    match (lf, crlf) {
        (Some(left), Some(right)) => Some(if left.0 < right.0 { left } else { right }),
        (Some(boundary), None) | (None, Some(boundary)) => Some(boundary),
        (None, None) => None,
    }
}

fn decode_frame<F>(frame: &str, on_event: &mut F) -> Result<(), AdkChatPortError>
where
    F: FnMut(&Value) -> Result<(), AdkChatPortError>,
{
    let mut data = String::new();
    for line in frame.lines() {
        let Some(value) = line.strip_prefix("data:") else {
            continue;
        };
        if !data.is_empty() {
            data.push('\n');
        }
        data.push_str(value.trim_start());
    }
    if data.trim().is_empty() || data.trim() == "[DONE]" {
        return Ok(());
    }
    let value: Value = serde_json::from_str(data.trim())
        .map_err(|error| upstream_error(format!("decode model stream event: {error}")))?;
    on_event(&value)
}
