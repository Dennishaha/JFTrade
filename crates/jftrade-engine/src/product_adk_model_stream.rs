//! OpenAI Responses SSE adapter used by the production ADK chat runtime.

use eventsource_stream::{EventStreamError, Eventsource};
use reqwest::{Client, StatusCode};
use serde_json::{Value, json};
use std::time::Duration;
use tokio_stream::StreamExt;

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

        let mut stream = response.bytes_stream().eventsource();
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
            let event = tokio::select! {
                item = stream.next() => match item {
                    Some(Ok(event)) => Some(event),
                    Some(Err(err)) => return Err(match err {
                        EventStreamError::Transport(error) => {
                            model_request_error(error, request.timeout)
                        }
                        other => upstream_error(format!("decode model stream event: {other}")),
                    }),
                    None => None,
                },
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
            let Some(event) = event else { break };
            bytes_seen = bytes_seen.saturating_add(event.data.len());
            if bytes_seen > MAX_RESPONSE_BYTES {
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
            match value
                .get("type")
                .and_then(Value::as_str)
                .unwrap_or_default()
            {
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
            if completed {
                break;
            }
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn provider_auth_rejections_are_external_and_not_retryable() {
        for (status, code, mapped_status) in [
            (StatusCode::UNAUTHORIZED, "MODEL_PROVIDER_UNAUTHORIZED", 502),
            (StatusCode::FORBIDDEN, "MODEL_PROVIDER_FORBIDDEN", 503),
        ] {
            let error = provider_rejection(
                status,
                None,
                br#"{"error":{"message":"provider rejected credentials"}}"#,
            );
            assert!(matches!(
                &error,
                AdkChatPortError::Failed { status, code: actual, .. }
                    if *status == mapped_status && actual == code
            ));
            assert!(!super::super::is_provider_retryable_error(&error));
        }
    }
}
