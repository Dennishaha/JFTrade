#![forbid(unsafe_code)]

use std::io::{Read, Write};
use std::net::TcpListener;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, Ordering};
use std::thread;
use std::time::{Duration, Instant};

use reqwest::Url;
use serde_json::{Value, json};

#[path = "../src/product_adk_chat_stream_port.rs"]
pub mod product_adk_chat_stream_port;

pub mod product {
    pub use super::product_adk_chat_stream_port;
}

use product_adk_chat_stream_port::AdkChatPortError;

pub const MAX_RESPONSE_BYTES: usize = 16 * 1024 * 1024;

#[derive(Clone, Debug)]
pub struct ModelRequest {
    pub endpoint: Url,
    pub api_key: String,
    pub model: String,
    pub instruction: Option<String>,
    pub message: String,
    pub durable_context: Vec<Value>,
    pub tool_context: Vec<Value>,
    pub timeout: Duration,
    pub tools: Vec<Value>,
}

#[derive(Debug)]
pub struct ModelToolCall {
    pub id: String,
    pub name: String,
    pub arguments: Value,
}

#[derive(Debug)]
pub struct ModelResponse {
    pub text: String,
    pub tool_calls: Vec<ModelToolCall>,
}

pub fn extract_text(response: &Value) -> String {
    response
        .get("text")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_owned()
}

pub fn extract_tool_calls(_response: &Value) -> Result<Vec<ModelToolCall>, AdkChatPortError> {
    Ok(Vec::new())
}

pub fn model_input(_request: &ModelRequest) -> Vec<Value> {
    vec![json!({"role": "user", "content": "hello"})]
}

pub fn unavailable(message: impl Into<String>) -> AdkChatPortError {
    AdkChatPortError::Unavailable(message.into())
}

pub fn upstream_error(message: impl Into<String>) -> AdkChatPortError {
    AdkChatPortError::Failed {
        status: 502,
        code: "UPSTREAM_ERROR".to_owned(),
        message: message.into(),
    }
}

pub fn is_provider_retryable_error(_: &AdkChatPortError) -> bool {
    false
}

#[path = "../src/product_adk_model_stream.rs"]
mod stream_adapter;

use stream_adapter::execute_model_stream;

fn run_mock_sse_server(
    response_headers_and_body: &'static str,
    delay_before_body: Duration,
) -> (String, thread::JoinHandle<()>) {
    let listener = TcpListener::bind(("127.0.0.1", 0)).expect("bind listener");
    let addr = listener.local_addr().expect("local addr");
    let handle = thread::spawn(move || {
        let (mut stream, _) = listener.accept().expect("accept connection");
        let mut buf = [0u8; 1024];
        let _ = stream.read(&mut buf);

        if !delay_before_body.is_zero() {
            thread::sleep(delay_before_body);
        }

        let _ = stream.write_all(response_headers_and_body.as_bytes());
        let _ = stream.flush();

        // Keep connection alive for a while so client can observe stream or disconnect
        thread::sleep(Duration::from_millis(1500));
    });

    (
        format!("http://{}:{}/v1/responses", addr.ip(), addr.port()),
        handle,
    )
}

#[test]
fn test_client_disconnect_returns_499_within_250ms() {
    // Upstream sends 200 OK text/event-stream headers, then stays silent (simulating slow LLM generation)
    let sse_response =
        "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: keep-alive\r\n\r\n";
    let (url, server_thread) = run_mock_sse_server(sse_response, Duration::ZERO);

    let cancelled = Arc::new(AtomicBool::new(false));
    let cancelled_clone = Arc::clone(&cancelled);

    // Schedule cancellation after 100ms
    let cancel_trigger = thread::spawn(move || {
        thread::sleep(Duration::from_millis(100));
        cancelled_clone.store(true, Ordering::SeqCst);
    });

    let request = ModelRequest {
        endpoint: Url::parse(&url).unwrap(),
        api_key: "test-key".to_owned(),
        model: "test-model".to_owned(),
        instruction: None,
        message: "hi".to_owned(),
        durable_context: Vec::new(),
        tool_context: Vec::new(),
        timeout: Duration::from_secs(5),
        tools: Vec::new(),
    };

    let start_wait = Instant::now();
    let result = execute_model_stream(request, |_| Ok(()), || cancelled.load(Ordering::SeqCst));

    // Cancellation was triggered at ~100ms.
    // The heartbeat sleep is 250ms.
    // Therefore, total elapsed time should be between 100ms and ~350ms (at most 250ms after cancellation).
    let elapsed = start_wait.elapsed();

    cancel_trigger.join().unwrap();
    server_thread.join().unwrap();

    assert!(
        result.is_err(),
        "execute_model_stream should fail on client disconnect"
    );

    match result.unwrap_err() {
        AdkChatPortError::Failed {
            status,
            code,
            message,
        } => {
            assert_eq!(status, 499, "Status must be HTTP 499");
            assert_eq!(
                code, "CLIENT_DISCONNECTED",
                "Code must be CLIENT_DISCONNECTED"
            );
            assert_eq!(message, "assistant chat client disconnected");
        }
        other => panic!("Expected HTTP 499 CLIENT_DISCONNECTED, got: {:?}", other),
    }

    // Since cancellation fired at ~100ms, the select! 250ms heartbeat detects it within 250ms.
    // Total time should not exceed 450ms (100ms + 250ms + margin).
    assert!(
        elapsed < Duration::from_millis(450),
        "Cancellation should be detected within 250ms of cancellation token firing, took: {:?}",
        elapsed
    );
}

#[test]
fn test_immediate_cancellation_returns_instantly() {
    let sse_response =
        "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: keep-alive\r\n\r\n";
    let (url, server_thread) = run_mock_sse_server(sse_response, Duration::ZERO);

    let request = ModelRequest {
        endpoint: Url::parse(&url).unwrap(),
        api_key: "test-key".to_owned(),
        model: "test-model".to_owned(),
        instruction: None,
        message: "hi".to_owned(),
        durable_context: Vec::new(),
        tool_context: Vec::new(),
        timeout: Duration::from_secs(5),
        tools: Vec::new(),
    };

    let start = Instant::now();
    let result = execute_model_stream(
        request,
        |_| Ok(()),
        || true, // Pre-cancelled!
    );
    let elapsed = start.elapsed();

    server_thread.join().unwrap();

    assert!(result.is_err());
    match result.unwrap_err() {
        AdkChatPortError::Failed { status, code, .. } => {
            assert_eq!(status, 499);
            assert_eq!(code, "CLIENT_DISCONNECTED");
        }
        other => panic!("Expected HTTP 499 CLIENT_DISCONNECTED, got: {:?}", other),
    }

    assert!(
        elapsed < Duration::from_millis(200),
        "Pre-cancelled stream should return immediately without waiting for 250ms timeout, took: {:?}",
        elapsed
    );
}

#[test]
fn test_stream_without_response_completed_fails_closed() {
    // Upstream sends deltas but abruptly closes without response.completed
    let sse_response = concat!(
        "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n",
        "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello world\"}\n\n"
    );
    let (url, server_thread) = run_mock_sse_server(sse_response, Duration::ZERO);

    let request = ModelRequest {
        endpoint: Url::parse(&url).unwrap(),
        api_key: "test-key".to_owned(),
        model: "test-model".to_owned(),
        instruction: None,
        message: "hi".to_owned(),
        durable_context: Vec::new(),
        tool_context: Vec::new(),
        timeout: Duration::from_secs(5),
        tools: Vec::new(),
    };

    let result = execute_model_stream(request, |_| Ok(()), || false);

    server_thread.join().unwrap();

    assert!(result.is_err());
    match result.unwrap_err() {
        AdkChatPortError::Failed {
            status, message, ..
        } => {
            assert_eq!(status, 502);
            assert!(
                message.contains("assistant model stream ended before response.completed"),
                "Must fail closed if stream ends without response.completed, got: {}",
                message
            );
        }
        other => panic!("Expected 502 with uncompleted message, got: {:?}", other),
    }
}

#[test]
fn test_complete_stream_with_response_completed_succeeds() {
    let sse_response = concat!(
        "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n",
        "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n",
        "data: {\"type\":\"response.completed\",\"response\":{\"text\":\"hello completed\"}}\n\n"
    );
    let (url, server_thread) = run_mock_sse_server(sse_response, Duration::ZERO);

    let request = ModelRequest {
        endpoint: Url::parse(&url).unwrap(),
        api_key: "test-key".to_owned(),
        model: "test-model".to_owned(),
        instruction: None,
        message: "hi".to_owned(),
        durable_context: Vec::new(),
        tool_context: Vec::new(),
        timeout: Duration::from_secs(5),
        tools: Vec::new(),
    };

    let mut events_seen = Vec::new();
    let result = execute_model_stream(
        request,
        |ev| {
            events_seen.push(ev.clone());
            Ok(())
        },
        || false,
    );

    server_thread.join().unwrap();

    let response = result.expect("complete stream should succeed");
    assert_eq!(response.text, "hello completed");
    assert_eq!(events_seen.len(), 2);
}
