use std::sync::Arc;

use axum::body::Body;
use axum::http::header::{CONTENT_TYPE, RETRY_AFTER};
use axum::http::{HeaderValue, Response, StatusCode};
use serde_json::{Value, json};
use time::OffsetDateTime;
use time::format_description::well_known::Rfc3339;

pub trait Clock: Send + Sync {
    fn now_rfc3339(&self) -> String;
}

#[derive(Clone, Debug, Default)]
pub struct SystemClock;

impl Clock for SystemClock {
    fn now_rfc3339(&self) -> String {
        OffsetDateTime::now_utc()
            .format(&Rfc3339)
            .unwrap_or_else(|_| "1970-01-01T00:00:00Z".to_owned())
    }
}

#[derive(Clone, Debug)]
pub struct FixedClock(pub String);

impl Clock for FixedClock {
    fn now_rfc3339(&self) -> String {
        self.0.clone()
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ApiFailure {
    pub status: u16,
    pub code: String,
    pub message: String,
    pub retry_after_seconds: Option<u64>,
}

impl ApiFailure {
    pub fn new(status: u16, code: impl Into<String>, message: impl Into<String>) -> Self {
        Self {
            status,
            code: code.into(),
            message: message.into(),
            retry_after_seconds: None,
        }
    }

    pub fn with_retry_after(mut self, seconds: u64) -> Self {
        self.retry_after_seconds = Some(seconds);
        self
    }
}

pub(crate) fn success_response(clock: &Arc<dyn Clock>, data: Value) -> Response<Body> {
    json_response(
        StatusCode::OK,
        json!({
            "ok": true,
            "data": data,
            "timestamp": clock.now_rfc3339(),
        }),
    )
}

pub(crate) fn error_response(clock: &Arc<dyn Clock>, failure: ApiFailure) -> Response<Body> {
    let status = StatusCode::from_u16(failure.status).unwrap_or(StatusCode::INTERNAL_SERVER_ERROR);
    let retry_after = failure
        .retry_after_seconds
        .map(|seconds| seconds.to_string());
    let mut response = json_response(
        status,
        json!({
            "ok": false,
            "error": {
                "code": failure.code,
                "message": failure.message,
            },
            "timestamp": clock.now_rfc3339(),
        }),
    );
    if let Some(retry_after) = retry_after
        && let Ok(value) = HeaderValue::from_str(&retry_after)
    {
        response.headers_mut().insert(RETRY_AFTER, value);
    }
    response
}

pub(crate) fn empty_response(status: StatusCode) -> Response<Body> {
    Response::builder()
        .status(status)
        .body(Body::empty())
        .expect("status-only response")
}

pub(crate) fn body_response(
    status: StatusCode,
    content_type: &str,
    body: impl Into<Body>,
) -> Response<Body> {
    let mut response = Response::builder()
        .status(status)
        .body(body.into())
        .expect("body response");
    if let Ok(content_type) = HeaderValue::from_str(content_type) {
        response.headers_mut().insert(CONTENT_TYPE, content_type);
    }
    response
}

fn json_response(status: StatusCode, value: Value) -> Response<Body> {
    let body = serde_json::to_vec(&value).unwrap_or_else(|_| b"{}".to_vec());
    body_response(status, "application/json; charset=utf-8", body)
}
